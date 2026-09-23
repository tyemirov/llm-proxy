package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestHostedProviderCostPreservesExactTicks(t *testing.T) {
	for _, scenario := range []struct {
		name, field, value string
		reason             journalUnknownReason
	}{
		{"exact", `,"cost_in_usd_ticks":9007199254740993123`, "9007199254740993123", ""},
		{"invalid_tokens", `,"cost_in_usd_ticks":123`, "123", ""},
		{"provider_failure", `,"cost_in_usd_ticks":456`, "456", ""},
		{"zero", `,"cost_in_usd_ticks":0`, "0", ""},
		{"missing", "", "", journalQuantityNotReported},
		{"negative", `,"cost_in_usd_ticks":-1`, "", journalQuantityInvalid},
		{"fraction", `,"cost_in_usd_ticks":1.5`, "", journalQuantityInvalid},
		{"string", `,"cost_in_usd_ticks":"private cost"`, "", journalQuantityInvalid},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodDelete {
					writer.WriteHeader(http.StatusNoContent)
					return
				}
				calls.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				if scenario.name == "provider_failure" {
					writer.WriteHeader(http.StatusBadRequest)
				}
				cache := 0
				if scenario.name == "invalid_tokens" {
					cache = 11
				}
				fmt.Fprintf(writer, `{"id":"private-cost-response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"retained answer"}]}],"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12,"input_tokens_details":{"cached_tokens":%d},"output_tokens_details":{"reasoning_tokens":0}%s}}`, cache, scenario.field)
			}))
			t.Cleanup(upstream.Close)
			server := newHostedTextProviderFixture(t, database, upstream.URL, "xai", "grok-4.3")
			for range 2 {
				request, err := http.NewRequest(http.MethodPost, server.URL+"/?provider=xai&model=grok-4.3", strings.NewReader(`{"prompt":"private prompt"}`))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "provider-cost")
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				want := http.StatusOK
				if scenario.name == "provider_failure" {
					want = http.StatusBadGateway
				}
				if err != nil || response.StatusCode != want {
					t.Fatalf("cost status=%d body=%s error=%v", response.StatusCode, body, err)
				}
				validateHostedIdentityResponse(t, request, response, body)
			}
			pending, err := database.pendingJournalDeliveries(t.Context(), 100)
			if err != nil || len(pending) != 1 || calls.Load() != 1 {
				t.Fatalf("cost deliveries=%v calls=%d error=%v", pending, calls.Load(), err)
			}
			var quantities []journalQuantity
			if err := json.Unmarshal(pending[0].Quantities, &quantities); err != nil {
				t.Fatal(err)
			}
			var cost journalQuantity
			for _, quantity := range quantities {
				if quantity.Dimension == "provider_cost" {
					cost = quantity
				}
			}
			if cost.Unit != "usd_tick" || cost.Value != scenario.value || cost.UnknownReason != scenario.reason || cost.IncludedIn != "" {
				t.Fatalf("cost=%+v evidence=%s", cost, pending[0].Quantities)
			}
			if strings.Contains(string(pending[0].SourceFields), "private") {
				t.Fatal("private content in cost evidence")
			}
			if scenario.value != "" && !strings.Contains(string(pending[0].SourceFields), `"path":"usage.cost_in_usd_ticks","value":"`+scenario.value+`"`) {
				t.Fatalf("cost source=%s", pending[0].SourceFields)
			}
			want := journalUsageComplete
			if scenario.reason != "" || scenario.name == "invalid_tokens" {
				want = journalUsageUnknown
			}
			entry := read("")["requests"].([]any)[0].(map[string]any)
			if entry["usage_state"] != string(want) {
				t.Fatalf("cost completeness=%v", entry)
			}
		})
	}
}
