package proxy

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func hostedSearchRatingAuthorization(t *testing.T) func(*hostedTextRequestDependencies) {
	t.Helper()
	catalog, conditions := hostedRatingTextCatalog()
	maximum := 2
	catalog.Offerings[0].Limits = append(catalog.Offerings[0].Limits, CatalogLimit{ID: "web_search_calls", Unit: "calls", Value: &maximum})
	catalog.Prices[0].Rates = append(catalog.Prices[0].Rates, CatalogPriceRate{Component: "web_search_calls", Currency: "USD", Rate: "0.01", Unit: "USD/call", Conditions: conditions})
	prices, err := NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	return func(dependencies *hostedTextRequestDependencies) {
		dependencies.authorize = func(transaction *gorm.DB, request managedJournalRequestRecord, _ hostedCompletionIntent) error {
			input := chatRequestParameters{provider: providerDefinition{identifier: providerID(request.Provider), activeTransport: providerTransportDefinition{responseCodec: CatalogProtocolOpenAIResponses}}, model: textModelDefinition{identifier: newModelID(request.Model)}, webSearchEnabled: true}
			reserve, err := newHostedTextPriceAdmission(prices, input, request.CreatedAt, categoricalPriceConditions(conditions), 2)
			if err != nil {
				return err
			}
			return reserve(transaction, request)
		}
	}
}

func TestHostedRatingSearchReservesAndEnforcesAcceptedToolBound(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		enabled bool
		calls   int
		state   string
	}{
		{"priced search", true, 2, chargeRated},
		{"exceeded search bound", true, 3, chargeLimitUnresolved},
		{"missing search evidence", true, -1, chargeUsageUnresolved},
		{"search disabled", false, 0, chargeRated},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, intent, management, _ := newHostedRatingFixture(t)
			catalog, conditions := hostedRatingTextCatalog()
			callLimit := 2
			catalog.Offerings[0].Limits = append(catalog.Offerings[0].Limits, CatalogLimit{ID: "web_search_calls", Unit: "calls", Value: &callLimit})
			catalog.Prices[0].Rates = append(catalog.Prices[0].Rates, CatalogPriceRate{Component: "web_search_calls", Currency: "USD", Rate: "0.01", Unit: "USD/call", Conditions: conditions})
			prices, err := NewCatalogService(catalog)
			if err != nil {
				t.Fatal(err)
			}
			priceRequest := chatRequestParameters{provider: providerDefinition{identifier: providerID("openai"), activeTransport: providerTransportDefinition{responseCodec: CatalogProtocolOpenAIResponses}}, model: textModelDefinition{identifier: newModelID("gpt-4.1")}, webSearchEnabled: scenario.enabled}
			reserve, err := newHostedTextPriceAdmission(prices, priceRequest, ratingTestAcceptanceTime(), categoricalPriceConditions(conditions), 1)
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := database.admitJournalRequest(t.Context(), intent("bounded-search"), reserve)
			if err != nil {
				t.Fatal(err)
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodDelete {
					writer.WriteHeader(http.StatusNoContent)
					return
				}
				var payload map[string]json.RawMessage
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if scenario.enabled && string(payload["max_tool_calls"]) != "2" {
					t.Errorf("accepted tool bound not sent: %s", payload["max_tool_calls"])
				}
				if !scenario.enabled && payload["max_tool_calls"] != nil {
					t.Errorf("unexpected search cap: %s", payload["max_tool_calls"])
				}
				output := []map[string]any{}
				for index := 0; index < scenario.calls; index++ {
					output = append(output, map[string]any{"type": "web_search_call", "id": fmt.Sprintf("search-%d", index), "status": "completed", "action": map[string]string{"type": "search"}})
				}
				response := map[string]any{"id": "private-rated-search", "status": "completed", "output_text": "rated answer", "usage": map[string]any{"input_tokens": 1000, "output_tokens": 100, "input_tokens_details": map[string]int{"cached_tokens": 200}, "output_tokens_details": map[string]int{"reasoning_tokens": 40}}}
				if scenario.calls >= 0 {
					response["output"] = output
				}
				writer.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(writer).Encode(response); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(upstream.Close)
			execution := newHostedTextExecution(database, accepted, reserve, func() time.Time { return accepted.CreatedAt.Add(time.Second) }, rand.Reader)
			execution.webSearch = scenario.enabled
			server := newJournalTextHTTPServer(t, database, upstream.URL, execution)
			body := fmt.Sprintf(`{"prompt":"private prompt","web_search":%t}`, scenario.enabled)
			response, err := server.Client().Post(server.URL+"/?provider=openai&model=gpt-4.1", "application/json", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("search status=%d", response.StatusCode)
			}
			pending, err := database.pendingJournalDeliveries(t.Context(), 10)
			if err != nil || len(pending) != 1 {
				t.Fatalf("search observations=%v error=%v", pending, err)
			}
			settled := 0
			if err := database.deliverJournalObservation(t.Context(), pending[0].ID, pending[0].ObservedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { settled++; return nil })); err != nil {
				t.Fatal(err)
			}
			collection := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
			charge := collection["charges"].([]any)[0].(map[string]any)
			retained := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/price-snapshots/"+charge["price_snapshot_id"].(string), "", http.StatusOK)
			snapshot := retained["snapshot"].(map[string]any)
			if scenario.enabled {
				bounds := map[string]string{}
				for _, item := range snapshot["bounds"].([]any) {
					bound := item.(map[string]any)
					bounds[bound["dimension"].(string)] = bound["maximum"].(string)
				}
				if bounds["input_tokens"] != "3000" || bounds["web_search_calls"] != "2" || snapshot["maximum"].(map[string]any)["reserved_cents"] != "5" {
					t.Fatalf("search costs missing from reservation: %v", snapshot)
				}
			} else if !reflect.DeepEqual(snapshot["excluded_components"], []any{"web_search_calls"}) {
				t.Fatalf("disabled search price was not excluded: %v", snapshot)
			}
			if charge["state"] != scenario.state {
				t.Fatalf("search charge=%v", charge)
			}
			if scenario.state == chargeRated {
				want := map[string]any{"numerator": "117", "denominator": "4000"}
				if !scenario.enabled {
					want["numerator"] = "13"
				}
				if settled != 1 || !reflect.DeepEqual(charge["customer_charge"], want) {
					t.Fatalf("search settlement=%d charge=%v", settled, charge)
				}
			} else if settled != 0 || charge["customer_charge"] != nil {
				t.Fatalf("unresolved search settled: %v", charge)
			}
		})
	}
}
