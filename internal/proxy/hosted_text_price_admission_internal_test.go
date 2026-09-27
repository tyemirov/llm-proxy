package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

func TestHostedTextPriceAdmissionRejectsInvalidOutputLimitsAtHTTPBoundary(t *testing.T) {
	for _, protocol := range []struct{ name, path, body string }{
		{"native", "/?provider=openai&model=gpt-4.1", `{"prompt":"funded prompt","max_tokens":%d}`},
		{"messages", v2Path + "?provider=openai&model=gpt-4.1", `{"messages":[{"role":"user","content":"funded prompt"}],"max_tokens":%d}`},
		{"chat-completions", chatCompletionsPath, `{"model":"openai/gpt-4.1","messages":[{"role":"user","content":"funded prompt"}],"max_completion_tokens":%d}`},
		{"responses", responsesPath, `{"model":"openai/gpt-4.1","input":"funded prompt","max_output_tokens":%d}`},
	} {
		t.Run(protocol.name, func(t *testing.T) {
			runtime := newHostedRuntimeTextFixture(t)
			seedHostedFunds(t, runtime.database, 5)
			application, err := buildRouterWithStoreForTest(t, runtime.configuration, zap.NewNop().Sugar(), func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
				return runtime.store, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(application)
			defer server.Close()
			server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
			fixture := fundsAdmissionFixture{fundsStartupFixture: runtime.fundsStartupFixture}
			before := fixture.state(t)
			for _, maximum := range []int{-1, 0, 1001} {
				message, err := http.NewRequest(http.MethodPost, server.URL+protocol.path, strings.NewReader(fmt.Sprintf(protocol.body, maximum)))
				if err != nil {
					t.Fatal(err)
				}
				message.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
				message.Header.Set("Content-Type", "application/json")
				message.Header.Set(llmproxycontract.HeaderIdempotencyKey, "invalid-output-limit")
				response, err := server.Client().Do(message)
				if err != nil {
					t.Fatal(err)
				}
				body, readError := io.ReadAll(response.Body)
				closeError := response.Body.Close()
				if readError != nil || closeError != nil || response.StatusCode != http.StatusBadRequest {
					t.Fatalf("output limit=%d status=%d body=%s read=%v close=%v", maximum, response.StatusCode, body, readError, closeError)
				}
				validateHostedIdentityResponse(t, message, response, body)
				fixture.assertRolledBack(t, before)
			}
			if fixture.calls.Load() != 0 {
				t.Fatalf("invalid output limit dispatched %d provider requests", fixture.calls.Load())
			}
		})
	}
}

func TestHostedTextPriceAdmissionRejectsUnqualifiedCatalogAndRecovers(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		search bool
		change func(*ModelCatalog)
	}{
		{"unavailable-price", false, func(catalog *ModelCatalog) {
			catalog.Prices[0].Available = false
			catalog.Prices[0].Rates = nil
			catalog.Prices[0].UnavailableReason = "Controlled price qualification is incomplete."
		}},
		{"reservation-overflow", false, func(catalog *ModelCatalog) { catalog.Prices[0].Rates[0].Rate = "1000000000000000000000000000000" }},
		{"unsupported-token-unit", false, func(catalog *ModelCatalog) { catalog.Prices[0].Rates[0].Unit = "USD/call" }},
		{"unmetered-component", false, func(catalog *ModelCatalog) { catalog.Prices[0].Rates[0].Component = "unmetered_input" }},
		{"missing-input-bound", false, func(catalog *ModelCatalog) { catalog.Offerings[0].Limits = nil }},
		{"account-dependent-input", false, func(catalog *ModelCatalog) {
			catalog.Offerings[0].Limits[0].AccountDependent = true
			catalog.Offerings[0].Limits[0].Value = nil
		}},
		{"missing-output-bound", false, func(catalog *ModelCatalog) { catalog.Offerings[0].OutputTokenLimit = 0 }},
		{"missing-search-bound", true, func(catalog *ModelCatalog) { catalog.Offerings[0].Limits = catalog.Offerings[0].Limits[:1] }},
		{"missing-search-price", true, func(catalog *ModelCatalog) { catalog.Prices[0].Rates = catalog.Prices[0].Rates[:3] }},
		{"unsupported-search-unit", true, func(catalog *ModelCatalog) { catalog.Prices[0].Rates[3].Unit = "USD/1M_tokens" }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			fixture.generation.Close()
			catalogForRequest := func() (ModelCatalog, *HostedConfiguration) {
				catalog, conditions := hostedRatingTextCatalog()
				if scenario.search {
					maximum := 1
					catalog.Offerings[0].Limits = append(catalog.Offerings[0].Limits, CatalogLimit{ID: "web_search_calls", Value: &maximum, Unit: "calls"})
					catalog.Prices[0].Rates = append(catalog.Prices[0].Rates, CatalogPriceRate{Component: "web_search_calls", Currency: "USD", Rate: "0.01", Unit: "USD/call", Conditions: conditions})
				}
				return catalog, &HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: "openai", Model: "gpt-4.1", Operation: ModelOperationText, MaximumAttempts: 1, Conditions: categoricalPriceConditions(conditions)}}}
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodDelete {
					writer.WriteHeader(http.StatusNoContent)
					return
				}
				fixture.calls.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(writer, `{"id":"qualified-price-result","status":"completed","output_text":"funded result","output":[],"usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
			}))
			defer upstream.Close()
			start := func(catalog ModelCatalog, configuration *HostedConfiguration, database *gormManagedTenantDatabase) *httptest.Server {
				settings, err := newHostedRuntimeSettings(configuration, catalog)
				if err != nil {
					t.Fatal(err)
				}
				return newHostedIdentityHTTPServer(t, database, upstream.URL, fixture.responseRoot, func(dependencies *hostedTextRequestDependencies) {
					dependencies.now = ratingTestAcceptanceTime
					dependencies.authorize = settings.authorizeCompletion
				})
			}
			request := func(server *httptest.Server, status int) string {
				message, err := http.NewRequest(http.MethodPost, server.URL+"/?provider=openai&model=gpt-4.1", strings.NewReader(fmt.Sprintf(`{"prompt":"funded prompt","web_search":%t}`, scenario.search)))
				if err != nil {
					t.Fatal(err)
				}
				message.Header.Set("Content-Type", "application/json")
				message.Header.Set(llmproxycontract.HeaderIdempotencyKey, "price-recovery")
				response, err := server.Client().Do(message)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil || response.StatusCode != status {
					t.Fatalf("price admission status=%d want=%d body=%s error=%v", response.StatusCode, status, body, err)
				}
				validateHostedIdentityResponse(t, message, response, body)
				return string(body)
			}
			catalog, configuration := catalogForRequest()
			scenario.change(&catalog)
			denied := start(catalog, configuration, fixture.database)
			before := fixture.state(t)
			for range 2 {
				body := request(denied, http.StatusServiceUnavailable)
				if !strings.Contains(body, `"code":"financial_admission_unavailable"`) || strings.Contains(body, "Controlled") || strings.Contains(body, "unmetered_input") {
					t.Fatalf("price admission lost public failure contract: %s", body)
				}
				fixture.assertRolledBack(t, before)
			}
			if fixture.calls.Load() != 0 {
				t.Fatal("unqualified price dispatched provider work")
			}
			denied.Close()
			catalog, configuration = catalogForRequest()
			qualified := start(catalog, configuration, fixture.database)
			for range 2 {
				if body := request(qualified, http.StatusOK); body != "funded result" {
					t.Fatalf("qualified result=%q", body)
				}
			}
			qualified.Close()
			restarted := start(catalog, configuration, openJournalTransactionInstance(t, fixture.database))
			assertHostedFundsBalance(t, fixture.database, 5, 5)
			assertFundsCreditRemainder(t, fixture.database, "91", "25000")
			before = fixture.state(t)
			if body := request(restarted, http.StatusOK); body != "funded result" {
				t.Fatalf("restarted result=%q", body)
			}
			if fixture.calls.Load() != 1 || !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatalf("price recovery repeated financial effects: calls=%d", fixture.calls.Load())
			}
		})
	}
}
