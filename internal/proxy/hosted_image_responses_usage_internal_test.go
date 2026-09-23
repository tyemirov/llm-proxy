package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHostedImageResponsesUsageRetainsSeparateEvidence(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
	const exactUsage = `{"total_tokens":9007199254741000,"input_tokens":9007199254740993,"output_tokens":7,"input_tokens_details":{"cached_tokens":3,"cache_write_tokens":0},"output_tokens_details":{"reasoning_tokens":2}}`
	for _, scenario := range []struct {
		name, surface, status, usage, value, output, state string
		unknown                                            journalUnknownReason
		failWrite                                          bool
	}{
		{name: "json", surface: "json", status: "completed", usage: exactUsage, value: "9007199254740993", output: encoded, state: MediaOperationStateSucceeded},
		{name: "poll", surface: "poll", status: "completed", usage: exactUsage, value: "9007199254740993", output: encoded, state: MediaOperationStateSucceeded},
		{name: "stream", surface: "stream", status: "completed", usage: exactUsage, value: "9007199254740993", output: encoded, state: MediaOperationStateSucceeded},
		{name: "invalid-image", surface: "json", status: "completed", usage: exactUsage, value: "9007199254740993", output: "invalid", state: MediaOperationStateFailed},
		{name: "invalid-stream-image", surface: "stream", status: "completed", usage: exactUsage, value: "9007199254740993", output: "invalid", state: MediaOperationStateFailed},
		{name: "failed", surface: "poll", status: "failed", usage: exactUsage, value: "9007199254740993", state: MediaOperationStateFailed},
		{name: "incomplete", surface: "stream", status: "incomplete", usage: exactUsage, value: "9007199254740993", state: MediaOperationStateFailed},
		{name: "cancelled", surface: "poll", status: "cancelled", usage: exactUsage, value: "9007199254740993", state: MediaOperationStateCancelled},
		{name: "missing", surface: "json", status: "completed", usage: `null`, unknown: journalQuantityNotReported, output: encoded, state: MediaOperationStateSucceeded},
		{name: "invalid-number", surface: "json", status: "completed", usage: `{"input_tokens":"12"}`, unknown: journalQuantityInvalid, output: encoded, state: MediaOperationStateSucceeded},
		{name: "inconsistent-total", surface: "json", status: "completed", usage: `{"total_tokens":3,"input_tokens":2,"output_tokens":2}`, unknown: journalQuantityInvalid, output: encoded, state: MediaOperationStateSucceeded},
		{name: "zero", surface: "json", status: "completed", usage: `{"total_tokens":0,"input_tokens":0,"output_tokens":0,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`, value: "0", output: encoded, state: MediaOperationStateSucceeded},
		{name: "write-failure", surface: "json", status: "completed", usage: exactUsage, output: encoded, failWrite: true, state: MediaOperationStateUncertain},
		{name: "stream-write-failure", surface: "stream", status: "completed", usage: exactUsage, output: encoded, failWrite: true, state: MediaOperationStateUncertain},
		{name: "poll-write-failure", surface: "poll", status: "completed", usage: exactUsage, output: encoded, failWrite: true, state: MediaOperationStateUncertain},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			var posts, polls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				if request.Method == http.MethodPost {
					posts.Add(1)
				} else {
					polls.Add(1)
				}
				terminal := fmt.Sprintf(`{"id":"resp_usage","status":%q,"usage":%s,"output":[{"id":"image_private","type":"image_generation_call","status":"completed","result":%q,"revised_prompt":"private rewritten prompt","usage":{"input_tokens":999}}]}`, scenario.status, scenario.usage, scenario.output)
				if scenario.surface == "poll" && request.Method == http.MethodPost {
					fmt.Fprint(writer, `{"id":"resp_usage","status":"queued","usage":{"input_tokens":111}}`)
				} else if scenario.surface == "stream" {
					writer.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(writer, "event: response.created\ndata: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_usage\",\"status\":\"queued\",\"usage\":{\"input_tokens\":111}}}\n\n")
					fmt.Fprintf(writer, "event: response.%s\ndata: {\"type\":\"response.%s\",\"sequence_number\":1,\"response\":%s}\n\n", scenario.status, scenario.status, terminal)
				} else {
					fmt.Fprint(writer, terminal)
				}
			}))
			t.Cleanup(upstream.Close)
			server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
			controls := json.RawMessage(fmt.Sprintf(`{"surface":"responses","responses_model":"gpt-5","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1,"stream":%t,"partial_images":0}`, scenario.surface == "stream"))
			accepted := hostedMediaAdmissionHTTP(t, server, "responses-usage", "private image prompt", http.StatusAccepted, controls)
			id := accepted["operation_id"].(string)
			if scenario.failWrite {
				if err := database.database.Exec("CREATE TRIGGER reject_response_usage BEFORE INSERT ON managed_journal_delivery_records BEGIN SELECT RAISE(ABORT, 'controlled response evidence failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			service.runOperation("responses-usage-worker", id)
			result := hostedMediaWorkerStatus(t, server, id)
			if result["state"] != scenario.state {
				t.Fatalf("result=%v", result)
			}
			hostedMediaAdmissionHTTP(t, server, "responses-usage", "private image prompt", http.StatusOK, controls)
			service.runOperation("duplicate-responses-worker", id)
			observations, err := database.pendingJournalDeliveries(context.Background(), 100)
			if err != nil || posts.Load() != 1 {
				t.Fatalf("observations=%v posts=%d error=%v", observations, posts.Load(), err)
			}
			if scenario.surface == "poll" && polls.Load() != 1 {
				t.Fatalf("polls=%d", polls.Load())
			}
			if scenario.failWrite {
				if len(observations) != 0 || len(result["outputs"].([]any)) != 0 || result["error"].(map[string]any)["code"] != "usage_journal_unavailable" {
					t.Fatalf("published without evidence: %v observations=%v", result, observations)
				}
				return
			}
			if len(observations) != 1 {
				t.Fatalf("observations=%v", observations)
			}
			observation := observations[0]
			if observation.AdapterRevision != CatalogProtocolOpenAIResponses+":image:1" {
				t.Fatalf("wrong meter=%s", observation.AdapterRevision)
			}
			var quantities []journalQuantity
			if err := json.Unmarshal(observation.Quantities, &quantities); err != nil {
				t.Fatal(err)
			}
			byDimension := map[string]journalQuantity{}
			for _, quantity := range quantities {
				byDimension[quantity.Dimension] = quantity
			}
			input := byDimension["responses_input_tokens"]
			if input.Value != scenario.value || input.UnknownReason != scenario.unknown || input.IncludedIn != "responses_total_tokens" {
				t.Fatalf("input=%+v", input)
			}
			if byDimension["responses_cache_read_tokens"].IncludedIn != "responses_input_tokens" || byDimension["responses_reasoning_tokens"].IncludedIn != "responses_output_tokens" {
				t.Fatalf("inclusion=%s", observation.Quantities)
			}
			if scenario.usage == exactUsage {
				for dimension, value := range map[string]string{"responses_output_tokens": "7", "responses_cache_read_tokens": "3", "responses_cache_write_tokens": "0", "responses_reasoning_tokens": "2"} {
					if quantity := byDimension[dimension]; quantity.Value != value || quantity.UnknownReason != "" {
						t.Fatalf("lost response quantity %s=%+v", dimension, quantity)
					}
				}
			}
			for _, dimension := range []string{"input_tokens", "output_tokens", "cache_read_text_tokens", "cache_read_image_tokens"} {
				if byDimension[dimension].UnknownReason != journalQuantityUnsupported {
					t.Fatalf("unqualified image usage=%s", observation.Quantities)
				}
			}
			if strings.Contains(string(observation.SourceFields), "private") || strings.Contains(string(observation.SourceFields), "999") || strings.Contains(string(observation.SourceFields), "111") {
				t.Fatalf("nonterminal or private evidence=%s", observation.SourceFields)
			}
			if scenario.value != "" && !strings.Contains(string(observation.SourceFields), `"path":"usage.input_tokens","value":"`+scenario.value+`"`) {
				t.Fatalf("source precision=%s", observation.SourceFields)
			}
			if entry := read("")["requests"].([]any)[0].(map[string]any); entry["usage_state"] != string(journalUsageUnknown) {
				t.Fatalf("image usage became complete=%v", entry)
			}
		})
	}
}
