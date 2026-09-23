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

func TestHostedToolUsageCountsSearchActions(t *testing.T) {
	for _, scenario := range []struct {
		name, output, value string
		reason              journalUnknownReason
	}{
		{"searches", `[{"type":"web_search_call","id":"ws1","status":"completed","action":{"type":"search","query":"private query"}},{"type":"web_search_call","id":"ws2","status":"completed","action":{"type":"search","queries":["private query","second query"]}}]`, "2", ""},
		{"other_actions", `[{"type":"web_search_call","id":"ws1","status":"completed","action":{"type":"open_page","url":"https://private.example"}},{"type":"web_search_call","id":"ws2","status":"completed","action":{"type":"find_in_page","pattern":"private pattern"}}]`, "0", ""},
		{"empty", `[]`, "0", ""},
		{"missing", `null`, "", journalQuantityNotReported},
		{"wrong_shape", `{}`, "", journalQuantityInvalid},
		{"unfinished", `[{"type":"web_search_call","id":"ws1","status":"in_progress","action":{"type":"search"}}]`, "", journalQuantityNotReported},
		{"failed", `[{"type":"web_search_call","id":"ws1","status":"failed","action":{"type":"search"}}]`, "", journalQuantityNotReported},
		{"unknown_action", `[{"type":"web_search_call","id":"ws1","status":"completed","action":{"type":"future_action"}}]`, "", journalQuantityUnsupported},
		{"duplicate", `[{"type":"web_search_call","id":"ws1","status":"completed","action":{"type":"search"}},{"type":"web_search_call","id":"ws1","status":"completed","action":{"type":"search"}}]`, "", journalQuantityInvalid},
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
				body, _ := io.ReadAll(request.Body)
				if !strings.Contains(string(body), "web_search") {
					t.Error("request did not enable provider search")
				}
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(writer, `{"id":"private-response","status":"completed","output_text":"retained answer","output":%s,"usage":{"input_tokens":10,"output_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`, scenario.output)
			}))
			t.Cleanup(upstream.Close)
			server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir())
			if scenario.name == "wrong_shape" {
				hostedSearchHTTP(t, server, "tool-usage", http.StatusBadGateway)
			} else {
				for range 2 {
					hostedSearchHTTP(t, server, "tool-usage", http.StatusOK)
				}
			}
			pending, err := database.pendingJournalDeliveries(t.Context(), 100)
			if err != nil || len(pending) != 1 || calls.Load() != 1 {
				t.Fatalf("tool observations=%v calls=%d error=%v", pending, calls.Load(), err)
			}
			var quantities []journalQuantity
			if err := json.Unmarshal(pending[0].Quantities, &quantities); err != nil {
				t.Fatal(err)
			}
			var search journalQuantity
			for _, quantity := range quantities {
				if quantity.Dimension == "web_search_calls" {
					search = quantity
				}
			}
			if search.Unit != "call" || search.Value != scenario.value || search.UnknownReason != scenario.reason {
				t.Fatalf("search usage=%+v", search)
			}
			if strings.Contains(string(pending[0].SourceFields), "private") || strings.Contains(string(pending[0].SourceFields), "https:") {
				t.Fatal("private tool content retained")
			}
			if scenario.value != "" && !strings.Contains(string(pending[0].SourceFields), `"path":"derived.output.web_search.search_count","value":"`+scenario.value+`"`) {
				t.Fatalf("missing tool count evidence=%s", pending[0].SourceFields)
			}
			usage := journalUsageComplete
			if scenario.reason != "" {
				usage = journalUsageUnknown
			}
			entry := read("")["requests"].([]any)[0].(map[string]any)
			if entry["usage_state"] != string(usage) {
				t.Fatalf("tool journal=%v", entry)
			}
		})
	}
}

func TestHostedToolUsagePollingAndContinuations(t *testing.T) {
	for _, mode := range []string{"polling", "continuation", "unknown", "write_failure"} {
		t.Run(mode, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			var posts, polls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodDelete {
					writer.WriteHeader(http.StatusNoContent)
					return
				}
				ordinal := posts.Load()
				if request.Method == http.MethodPost {
					ordinal = posts.Add(1)
				} else {
					polls.Add(1)
				}
				writer.Header().Set("Content-Type", "application/json")
				if mode == "polling" && request.Method == http.MethodPost {
					fmt.Fprint(writer, `{"id":"private-tool-1","status":"in_progress","output":[{"type":"web_search_call","id":"ws1","status":"in_progress","action":{"type":"search"}}]}`)
					return
				}
				status, details := "completed", ""
				output := `[{"type":"web_search_call","id":"ws1","status":"completed","action":{"type":"search"}}]`
				if ordinal == 1 && (mode == "continuation" || mode == "unknown") {
					status, details = "incomplete", `,"incomplete_details":{"reason":"max_output_tokens"}`
				}
				if mode == "unknown" {
					output = "null"
				}
				fmt.Fprintf(writer, `{"id":"private-tool-%d","status":%q,"output_text":"retained answer","output":%s,"usage":{"input_tokens":10,"output_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}%s}`, ordinal, status, output, details)
			}))
			t.Cleanup(upstream.Close)
			server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir())
			want, wantPosts, wantObservations := http.StatusOK, int64(1), 1
			switch mode {
			case "continuation":
				wantPosts, wantObservations = 2, 2
			case "unknown":
				want = http.StatusConflict
			case "write_failure":
				want, wantObservations = http.StatusBadGateway, 0
				if err := database.database.Exec("CREATE TRIGGER reject_tool_usage BEFORE INSERT ON managed_journal_delivery_records BEGIN SELECT RAISE(ABORT, 'controlled tool evidence failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			hostedSearchHTTP(t, server, "tool-flow", want)
			if mode == "unknown" {
				want = http.StatusBadGateway
			}
			if mode == "write_failure" {
				if err := database.database.Exec("DROP TRIGGER reject_tool_usage").Error; err != nil {
					t.Fatal(err)
				}
				want = http.StatusConflict
			}
			hostedSearchHTTP(t, server, "tool-flow", want)
			observations, err := database.pendingJournalDeliveries(t.Context(), 100)
			if err != nil || len(observations) != wantObservations || posts.Load() != wantPosts {
				t.Fatalf("tool flow evidence=%v posts=%d error=%v", observations, posts.Load(), err)
			}
			if mode == "polling" && polls.Load() != 1 {
				t.Fatalf("tool polls=%d", polls.Load())
			}
			for _, observation := range observations {
				var quantities []journalQuantity
				if err := json.Unmarshal(observation.Quantities, &quantities); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, quantity := range quantities {
					if quantity.Dimension != "web_search_calls" {
						continue
					}
					found = true
					if mode == "unknown" {
						if quantity.UnknownReason != journalQuantityNotReported {
							t.Fatalf("missing tool evidence=%+v", quantity)
						}
					} else if quantity.Value != "1" {
						t.Fatalf("tool count=%+v", quantity)
					}
				}
				if !found {
					t.Fatal("tool counter missing after continuation or polling")
				}
			}
			entry := read("")["requests"].([]any)[0].(map[string]any)
			if (mode == "unknown" || mode == "write_failure") && entry["usage_state"] != string(journalUsageUnknown) {
				t.Fatalf("failed evidence became complete: %v", entry)
			}
		})
	}
}

func hostedSearchHTTP(t *testing.T, server *httptest.Server, key string, want int) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/?provider=openai&model=gpt-4.1", strings.NewReader(`{"prompt":"private prompt","web_search":true}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != want {
		t.Fatalf("search status=%d body=%s error=%v", response.StatusCode, body, err)
	}
	validateHostedIdentityResponse(t, request, response, body)
}
