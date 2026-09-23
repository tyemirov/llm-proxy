package proxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

func TestHostedRatingGeminiBillsSeparateThoughtsWithoutExplicitStorage(t *testing.T) {
	for _, mode := range []string{"reported", "missing_thoughts", "unexpected_tool"} {
		t.Run(mode, func(t *testing.T) { testHostedGeminiRating(t, mode) })
	}
}

func testHostedGeminiRating(t *testing.T, mode string) {
	database, _, management, _ := newHostedRatingFixture(t)
	catalog := internalTestModelCatalog(internalTestOffering("gemini", "gemini-3.5-flash", []string{"text"}, []string{"text"}))
	catalog.Revision = "journal-catalog"
	inputLimit := 1000
	catalog.Offerings[0].OutputTokenLimit = 1000
	catalog.Offerings[0].Limits = []CatalogLimit{{ID: "input_tokens", Value: &inputLimit, Unit: "tokens"}}
	conditions := CatalogPriceConditions{ServiceTier: "standard", EffectiveFrom: "2026-09-01T00:00:00Z"}
	cache, storage := conditions, conditions
	cache.CacheClass, storage.CacheClass = "read", "storage"
	catalog.Prices[0] = CatalogPriceDescriptor{Provider: "gemini", Model: "gemini-3.5-flash", Operation: "text", Available: true, Source: "https://example.com/prices", LastVerified: "2026-09-22", Rates: []CatalogPriceRate{
		{Component: "input_text", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: conditions},
		{Component: "output_text", Currency: "USD", Rate: "8", Unit: "USD/1M_tokens", Conditions: conditions},
		{Component: "cache_read", Currency: "USD", Rate: "0.5", Unit: "USD/1M_tokens", Conditions: cache},
		{Component: "cache_storage", Currency: "USD", Rate: "2", Unit: "USD/1M_token_hours", Conditions: storage},
	}}
	prices, err := NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	request := chatRequestParameters{provider: providerDefinition{identifier: providerID("gemini"), activeTransport: providerTransportDefinition{responseCodec: CatalogProtocolGeminiInteractions}}, model: textModelDefinition{identifier: newModelID("gemini-3.5-flash")}}
	reserve, err := newHostedTextPriceAdmission(prices, request, ratingTestAcceptanceTime(), categoricalPriceConditions(conditions), 1)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"id":"private-thinking","status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"rated thought"}]}],"usage":{"total_input_tokens":1000,"total_output_tokens":100,"total_cached_tokens":200,"total_thought_tokens":40,"total_tool_use_tokens":0,"input_tokens_by_modality":[{"modality":"text","tokens":1000}],"output_tokens_by_modality":[{"modality":"text","tokens":100}],"cached_tokens_by_modality":[{"modality":"text","tokens":200}]}}`
	if mode == "missing_thoughts" {
		payload = strings.Replace(payload, `,"total_thought_tokens":40`, "", 1)
	}
	if mode == "unexpected_tool" {
		payload = strings.Replace(payload, `"total_tool_use_tokens":0`, `"total_tool_use_tokens":1`, 1)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, payload)
	}))
	t.Cleanup(upstream.Close)
	server := newHostedTextProviderFixture(t, database, upstream.URL, "gemini", "gemini-3.5-flash", func(dependencies *hostedTextRequestDependencies) {
		dependencies.now = func() time.Time { return ratingTestAcceptanceTime() }
		dependencies.authorize = reserve
	})
	if err := database.database.Model(&managedPlatformCredentialRecord{}).Where("connection_id = ?", "platform-text").Update("qualified_at", ratingTestAcceptanceTime().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	httpRequest, err := http.NewRequest(http.MethodPost, server.URL+"/?provider=gemini&model=gemini-3.5-flash", strings.NewReader(`{"prompt":"private prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set(llmproxycontract.HeaderIdempotencyKey, "gemini-billed-thoughts")
	response, err := server.Client().Do(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || string(body) != "rated thought" {
		t.Fatalf("Gemini response=%d %s %v", response.StatusCode, body, err)
	}
	pending, err := database.pendingJournalDeliveries(t.Context(), 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("Gemini observations=%v error=%v", pending, err)
	}
	if err := database.deliverJournalObservation(t.Context(), pending[0].ID, pending[0].ObservedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error {
		if mode != "reported" {
			return errors.New("unresolved Gemini usage reached settlement")
		}
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	collection := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
	charge := collection["charges"].([]any)[0].(map[string]any)
	if mode == "reported" {
		if charge["state"] != chargeRated || !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": "1833", "denominator": "500000"}) {
			t.Fatalf("Gemini usage was not rated exactly: %v", charge)
		}
	} else if charge["state"] != chargeUsageUnresolved || charge["customer_charge"] != nil {
		t.Fatalf("incomplete Gemini evidence became a charge: %v", charge)
	}
	retained := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/price-snapshots/"+charge["price_snapshot_id"].(string), "", http.StatusOK)
	document := retained["snapshot"].(map[string]any)
	if !reflect.DeepEqual(document["excluded_components"], []any{"cache_storage"}) || !reflect.DeepEqual(document["zero_dimensions"], []any{"tool_input_tokens"}) {
		t.Fatalf("operation conditions were not retained: %v", document)
	}
}
