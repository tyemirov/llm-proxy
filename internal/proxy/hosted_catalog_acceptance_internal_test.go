package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestHostedTextJournalCoversActiveCatalog(t *testing.T) {
	registry := internalManagementProviderRegistry()
	count := 0
	for providerID, provider := range registry.definitions {
		for modelID, model := range provider.textModels {
			count++
			t.Run(providerID.string()+"/"+modelID, func(t *testing.T) {
				database, _, read := newJournalTransactionFixture(t)
				transport := provider.transports[model.transportIdentifier]
				payload := hostedCatalogTextResponse(t, transport.responseCodec)
				var calls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.Method == http.MethodDelete {
						writer.WriteHeader(http.StatusNoContent)
						return
					}
					calls.Add(1)
					if request.Method != http.MethodPost || request.Header.Get(transport.authentication.Header) != transport.authentication.Prefix+"hosted-text-secret" {
						t.Error("catalog dispatch used incorrect authority")
					}
					writer.Header().Set("Content-Type", "application/json")
					_, _ = writer.Write(payload)
				}))
				t.Cleanup(upstream.Close)
				server := newHostedTextProviderFixture(t, database, upstream.URL, providerID.string(), modelID)
				for range 2 {
					request, err := http.NewRequest(http.MethodPost, server.URL+"/?provider="+providerID.string()+"&model="+modelID, strings.NewReader(`{"prompt":"private catalog prompt"}`))
					if err != nil {
						t.Fatal(err)
					}
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "catalog-usage")
					response, err := server.Client().Do(request)
					if err != nil {
						t.Fatal(err)
					}
					body, err := io.ReadAll(response.Body)
					response.Body.Close()
					if err != nil || response.StatusCode != http.StatusOK || string(body) != "retained answer" {
						t.Fatalf("catalog status=%d body=%s error=%v", response.StatusCode, body, err)
					}
					validateHostedIdentityResponse(t, request, response, body)
				}
				pending, err := database.pendingJournalDeliveries(t.Context(), 100)
				if err != nil || len(pending) != 1 || calls.Load() != 1 {
					t.Fatalf("catalog observations=%v calls=%d error=%v", pending, calls.Load(), err)
				}
				var quantities []journalQuantity
				if err := json.Unmarshal(pending[0].Quantities, &quantities); err != nil {
					t.Fatal(err)
				}
				input := "9007199254740993"
				if transport.responseCodec == CatalogProtocolGeminiInteractions || transport.responseCodec == CatalogProtocolVertexGenerateContent {
					input = "9007199254741005"
				}
				found := false
				for _, quantity := range quantities {
					if quantity.UnknownReason != "" {
						t.Fatalf("catalog quantity is unknown: %+v", quantity)
					}
					if quantity.Dimension == "input_tokens" {
						found = quantity.Value == input && quantity.Unit == "token"
					}
				}
				if !found {
					t.Fatalf("catalog input evidence=%s", pending[0].Quantities)
				}
				entry := read("")["requests"].([]any)[0].(map[string]any)
				if entry["state"] != string(journalRequestCompleted) || entry["usage_state"] != string(journalUsageComplete) {
					t.Fatalf("catalog journal=%v", entry)
				}
			})
		}
	}
	if count == 0 {
		t.Fatal("no active text routes were exercised")
	}
	t.Logf("verified %d active text offerings", count)
}

func hostedCatalogTextResponse(t *testing.T, codec string) []byte {
	t.Helper()
	switch codec {
	case CatalogProtocolGeminiInteractions:
		return hostedModalityResponse(t, "gemini", "exact")
	case CatalogProtocolVertexGenerateContent:
		return hostedModalityResponse(t, "vertex", "exact")
	case CatalogProtocolOpenAIChatCompletions:
		return []byte(`{"id":"private-catalog","choices":[{"index":0,"message":{"role":"assistant","content":"retained answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9007199254740993,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":0},"completion_tokens_details":{"reasoning_tokens":0}}}`)
	case CatalogProtocolAnthropicMessages:
		return []byte(`{"id":"private-catalog","type":"message","role":"assistant","content":[{"type":"text","text":"retained answer"}],"stop_reason":"end_turn","usage":{"input_tokens":9007199254740993,"output_tokens":2,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0}}}`)
	case CatalogProtocolOpenAIResponses, CatalogProtocolDashScopeResponses, CatalogProtocolXAIResponses:
		return []byte(`{"id":"private-catalog","status":"completed","output_text":"retained answer","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"retained answer"}]}],"usage":{"input_tokens":9007199254740993,"output_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0},"cost_in_usd_ticks":13}}`)
	default:
		t.Fatalf("unqualified text usage codec %q", codec)
		return nil
	}
}
