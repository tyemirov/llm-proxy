package proxy

import (
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedRatingTextProtocolBindsNativeCacheUsage(t *testing.T) {
	thousand, twoThousand := 1000, 2000
	for _, test := range []struct {
		name   string
		limits []CatalogLimit
	}{
		{"input ceiling", []CatalogLimit{{ID: "input_tokens", Value: &thousand, Unit: "tokens"}}},
		{"context ceiling", []CatalogLimit{{ID: "context_tokens", Value: &thousand, Unit: "tokens"}}},
		{"input tighter", []CatalogLimit{{ID: "input_tokens", Value: &thousand, Unit: "tokens"}, {ID: "context_tokens", Value: &twoThousand, Unit: "tokens"}}},
		{"context tighter", []CatalogLimit{{ID: "input_tokens", Value: &twoThousand, Unit: "tokens"}, {ID: "context_tokens", Value: &thousand, Unit: "tokens"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, intent, management, _ := newHostedRatingFixture(t)
			catalog, conditions := hostedRatingTextCatalog()
			catalog.Offerings[0].Limits = test.limits
			prices, err := NewCatalogService(catalog)
			if err != nil {
				t.Fatal(err)
			}
			maxTokens := 100
			priceRequest := chatRequestParameters{provider: providerDefinition{identifier: providerID("openai"), activeTransport: providerTransportDefinition{responseCodec: CatalogProtocolOpenAIResponses}}, model: textModelDefinition{identifier: newModelID("gpt-4.1")}, maxTokens: &maxTokens}
			reserve, err := newHostedTextPriceAdmission(prices, priceRequest, ratingTestAcceptanceTime(), categoricalPriceConditions(conditions), 1)
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := database.admitJournalRequest(t.Context(), intent("native-cache-price"), reserve)
			if err != nil {
				t.Fatal(err)
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodDelete {
					writer.WriteHeader(http.StatusNoContent)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(writer, `{"id":"private-rated-response","status":"completed","output_text":"rated answer","usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":200},"output_tokens_details":{"reasoning_tokens":40}}}`)
			}))
			t.Cleanup(upstream.Close)
			execution := newHostedTextExecution(database, accepted, reserve, func() time.Time { return accepted.CreatedAt.Add(time.Second) }, rand.Reader)
			server := newJournalTextHTTPServer(t, database, upstream.URL, execution)
			response, err := server.Client().Post(server.URL+"/?provider=openai&model=gpt-4.1", "application/json", strings.NewReader(`{"prompt":"private prompt"}`))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != http.StatusOK || string(body) != "rated answer" {
				t.Fatalf("native text response=%d %s %v", response.StatusCode, body, err)
			}
			pending, err := database.pendingJournalDeliveries(t.Context(), 10)
			if err != nil || len(pending) != 1 {
				t.Fatalf("native text observations=%v error=%v", pending, err)
			}
			if err := database.deliverJournalObservation(t.Context(), pending[0].ID, pending[0].ObservedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })); err != nil {
				t.Fatal(err)
			}
			collection := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
			charge := collection["charges"].([]any)[0].(map[string]any)
			if charge["state"] != chargeRated || !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": "13", "denominator": "4000"}) {
				t.Fatalf("native usage was not rated exactly: %v", charge)
			}
			retained := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/price-snapshots/"+charge["price_snapshot_id"].(string), "", http.StatusOK)
			document := retained["snapshot"].(map[string]any)
			if document["maximum"].(map[string]any)["reserved_cents"] != "2" {
				t.Fatalf("catalog ceiling was not reserved: %v", document)
			}
			for _, bound := range document["bounds"].([]any) {
				if bound.(map[string]any)["maximum"] != "1000" {
					t.Fatalf("request bound ignores catalog ceiling: %v", bound)
				}
			}
		})
	}
}

func TestHostedRatingTextBindingsRespectProtocolEvidence(t *testing.T) {
	for _, codec := range []string{CatalogProtocolOpenAIResponses, CatalogProtocolDashScopeResponses, CatalogProtocolXAIResponses, CatalogProtocolOpenAIChatCompletions, CatalogProtocolAnthropicMessages} {
		t.Run(codec, func(t *testing.T) {
			catalog := internalTestModelCatalog(internalTestOffering("openai", "gpt-4.1", []string{"text"}, []string{"text"}))
			conditions := CatalogPriceConditions{ServiceTier: "standard", BillingMode: "standard"}
			cache := conditions
			cache.CacheClass = "read"
			rates := []CatalogPriceRate{
				{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: conditions},
				{Component: "output_tokens", Currency: "USD", Rate: "8", Unit: "USD/1M_tokens", Conditions: conditions},
				{Component: "prompt_cache_read_tokens", Currency: "USD", Rate: "0.5", Unit: "USD/1M_tokens", Conditions: cache},
			}
			payload := `{"usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":200},"output_tokens_details":{"reasoning_tokens":40},"cost_in_usd_ticks":25000000}}`
			expected := ExactMoney{Numerator: "13", Denominator: "4000"}
			if codec == CatalogProtocolOpenAIChatCompletions {
				payload = `{"usage":{"prompt_tokens":1000,"completion_tokens":100,"prompt_tokens_details":{"cached_tokens":200},"completion_tokens_details":{"reasoning_tokens":40}}}`
			}
			if codec == CatalogProtocolAnthropicMessages {
				five, hour := conditions, conditions
				five.CacheClass, hour.CacheClass = "write_5m", "write_1h"
				rates = append(rates, CatalogPriceRate{Component: "prompt_cache_write_tokens", Currency: "USD", Rate: "2.5", Unit: "USD/1M_tokens", Conditions: five}, CatalogPriceRate{Component: "prompt_cache_write_tokens", Currency: "USD", Rate: "4", Unit: "USD/1M_tokens", Conditions: hour})
				payload = `{"usage":{"input_tokens":800,"output_tokens":100,"cache_read_input_tokens":200,"cache_creation_input_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":20}}}`
				expected = ExactMoney{Numerator: "6773", Denominator: "2000000"}
			}
			catalog.Prices[0] = CatalogPriceDescriptor{Provider: "openai", Model: "gpt-4.1", Operation: "text", Available: true, Source: "https://example.com/prices", LastVerified: "2026-09-22", Rates: rates}
			prices, err := NewCatalogService(catalog)
			if err != nil {
				t.Fatal(err)
			}
			meter, err := newTextJournalMeter(codec)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := meter.ratingSnapshot(prices, "openai", "gpt-4.1", ratingTestAcceptanceTime(), conditions, false)
			if err != nil {
				t.Fatal(err)
			}
			observation, terminal := meter.observe([]byte(payload), http.StatusOK, "attempt-test", ratingTestAcceptanceTime())
			result, err := snapshot.Rate(observation.Quantities)
			if err != nil || !terminal || result.State != CatalogRatingResolved || result.CustomerCharge != expected {
				t.Fatalf("protocol quantities=%+v result=%+v error=%v", observation.Quantities, result, err)
			}
			unknown, _ := meter.observe([]byte(`{}`), http.StatusOK, "attempt-unknown", ratingTestAcceptanceTime())
			result, err = snapshot.Rate(unknown.Quantities)
			if err != nil || result.State != CatalogRatingUnresolved {
				t.Fatalf("unreported quantities became charges: %+v %v", result, err)
			}
			if _, err := meter.ratingSnapshot(prices, "openai", "gpt-4.1", ratingTestAcceptanceTime(), CatalogPriceConditions{ServiceTier: "priority"}, false); !errors.Is(err, ErrCatalogRatingUnavailable) {
				t.Fatalf("unpriced service tier admitted: %v", err)
			}
		})
	}
}

func hostedRatingTextCatalog() (ModelCatalog, CatalogPriceConditions) {
	catalog := internalTestModelCatalog(internalTestOffering("openai", "gpt-4.1", []string{"text"}, []string{"text"}))
	catalog.Revision = "journal-catalog"
	inputLimit := 1000
	catalog.Offerings[0].OutputTokenLimit = 1000
	catalog.Offerings[0].Limits = []CatalogLimit{{ID: "input_tokens", Value: &inputLimit, Unit: "tokens"}}
	conditions := CatalogPriceConditions{BillingMode: "standard", ServiceTier: "standard", EffectiveFrom: "2026-09-01T00:00:00Z"}
	cache := conditions
	cache.CacheClass = "read"
	catalog.Prices[0] = CatalogPriceDescriptor{Provider: "openai", Model: "gpt-4.1", Operation: "text", Available: true, Source: "https://example.com/prices", LastVerified: "2026-09-22", Rates: []CatalogPriceRate{
		{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: conditions},
		{Component: "output_tokens", Currency: "USD", Rate: "8", Unit: "USD/1M_tokens", Conditions: conditions},
		{Component: "cache_read", Currency: "USD", Rate: "0.5", Unit: "USD/1M_tokens", Conditions: cache},
	}}
	return catalog, conditions
}

func TestHostedRatingTextBoundsStopExcessContinuationBeforeDispatch(t *testing.T) {
	database, intent, _, _ := newHostedRatingFixture(t)
	catalog, conditions := hostedRatingTextCatalog()
	prices, err := NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	maxTokens := 100
	request := chatRequestParameters{provider: providerDefinition{identifier: providerID("openai"), activeTransport: providerTransportDefinition{responseCodec: CatalogProtocolOpenAIResponses}}, model: textModelDefinition{identifier: newModelID("gpt-4.1")}, maxTokens: &maxTokens}
	reserve, err := newHostedTextPriceAdmission(prices, request, ratingTestAcceptanceTime(), categoricalPriceConditions(conditions), 2)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := database.admitJournalRequest(t.Context(), intent("bounded-continuations"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		calls.Add(1)
		var count int64
		if err := database.database.Model(&managedPriceSnapshotRecord{}).Where("request_id = ?", accepted.ID).Count(&count).Error; err != nil || count != 1 {
			t.Errorf("provider dispatch preceded retained price: %d %v", count, err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output_text":"partial","usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	execution := newHostedTextExecution(database, accepted, reserve, func() time.Time { return accepted.CreatedAt.Add(time.Second) }, rand.Reader)
	server := newJournalTextHTTPServer(t, database, upstream.URL, execution)
	response, err := server.Client().Post(server.URL+"/?provider=openai&model=gpt-4.1&max_tokens=100", "application/json", strings.NewReader(`{"prompt":"private prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	validateHostedIdentityResponse(t, response.Request, response, body)
	if response.StatusCode != http.StatusConflict || calls.Load() != 2 || !strings.Contains(string(body), "usage_journal_conflict") {
		t.Fatalf("excess continuation status=%d calls=%d body=%s", response.StatusCode, calls.Load(), body)
	}
	var attempts int64
	if err := database.database.Model(&managedJournalAttemptRecord{}).Where("request_id = ?", accepted.ID).Count(&attempts).Error; err != nil || attempts != 2 {
		t.Fatalf("excess attempt retained: %d %v", attempts, err)
	}
}

func TestHostedRatingTextBoundsRejectUnknownCapacityAndTools(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ModelCatalog, *chatRequestParameters)
	}{
		{"missing input", func(c *ModelCatalog, _ *chatRequestParameters) { c.Offerings[0].Limits = nil }},
		{"account dependent input", func(c *ModelCatalog, _ *chatRequestParameters) {
			c.Offerings[0].Limits[0].AccountDependent = true
			c.Offerings[0].Limits[0].Value = nil
		}},
		{"missing output", func(c *ModelCatalog, _ *chatRequestParameters) { c.Offerings[0].OutputTokenLimit = 0 }},
		{"unbounded search", func(_ *ModelCatalog, r *chatRequestParameters) { r.webSearchEnabled = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog, conditions := hostedRatingTextCatalog()
			request := chatRequestParameters{provider: providerDefinition{identifier: providerID("openai"), activeTransport: providerTransportDefinition{responseCodec: CatalogProtocolOpenAIResponses}}, model: textModelDefinition{identifier: newModelID("gpt-4.1")}}
			test.change(&catalog, &request)
			prices, err := NewCatalogService(catalog)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := newHostedTextPriceAdmission(prices, request, ratingTestAcceptanceTime(), categoricalPriceConditions(conditions), 2); !errors.Is(err, ErrCatalogRatingUnavailable) {
				t.Fatalf("unbounded work authorized: %v", err)
			}
		})
	}
}
