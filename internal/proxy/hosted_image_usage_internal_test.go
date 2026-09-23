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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHostedImageUsageRetainsExactEvidenceBeforePublication(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
	const exactUsage = `{"total_tokens":9007199254741000,"input_tokens":9007199254740993,"output_tokens":7,"input_tokens_details":{"text_tokens":9007199254740990,"image_tokens":3},"output_tokens_details":{"text_tokens":0,"image_tokens":7}}`
	for _, scenario := range []struct {
		name, usage, output, inputValue string
		stream                          bool
		partials                        int
		assetLimit                      int64
		state                           string
		unknown                         journalUnknownReason
	}{
		{name: "json", usage: exactUsage, output: encoded, inputValue: "9007199254740993", state: MediaOperationStateSucceeded},
		{name: "stream", usage: exactUsage, output: encoded, inputValue: "9007199254740993", stream: true, state: MediaOperationStateSucceeded},
		{name: "stream-preview", usage: exactUsage, output: encoded, inputValue: "9007199254740993", stream: true, partials: 1, state: MediaOperationStateSucceeded},
		{name: "asset-failure", usage: exactUsage, output: encoded, inputValue: "9007199254740993", assetLimit: 1, state: MediaOperationStateFailed},
		{name: "invalid-image", usage: exactUsage, output: "not-an-image", inputValue: "9007199254740993", state: MediaOperationStateFailed},
		{name: "invalid-stream-image", usage: exactUsage, output: "not-an-image", inputValue: "9007199254740993", stream: true, state: MediaOperationStateFailed},
		{name: "missing", usage: `null`, output: encoded, unknown: journalQuantityNotReported, state: MediaOperationStateSucceeded},
		{name: "invalid-number", usage: `{"input_tokens":"12"}`, output: encoded, unknown: journalQuantityInvalid, state: MediaOperationStateSucceeded},
		{name: "inconsistent-total", usage: `{"total_tokens":16,"input_tokens":10,"output_tokens":7}`, output: encoded, unknown: journalQuantityInvalid, state: MediaOperationStateSucceeded},
		{name: "inconsistent-input", usage: `{"total_tokens":17,"input_tokens":10,"output_tokens":7,"input_tokens_details":{"text_tokens":8,"image_tokens":3}}`, output: encoded, unknown: journalQuantityInvalid, state: MediaOperationStateSucceeded},
		{name: "oversized-child", usage: `{"total_tokens":10,"input_tokens":11}`, output: encoded, unknown: journalQuantityInvalid, state: MediaOperationStateSucceeded},
		{name: "zero", usage: `{"total_tokens":0,"input_tokens":0,"output_tokens":0,"input_tokens_details":{"text_tokens":0,"image_tokens":0},"output_tokens_details":{"text_tokens":0,"image_tokens":0}}`, output: encoded, inputValue: "0", state: MediaOperationStateSucceeded},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if scenario.stream {
					writer.Header().Set("Content-Type", "text/event-stream")
					if scenario.partials != 0 {
						fmt.Fprintf(writer, "event: image_generation.partial_image\ndata: {\"type\":\"image_generation.partial_image\",\"partial_image_index\":0,\"b64_json\":%q,\"output_format\":\"png\",\"usage\":{\"input_tokens\":111}}\n\n", encoded)
					}
					fmt.Fprintf(writer, "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"b64_json\":%q,\"output_format\":\"png\",\"usage\":%s}\n\n", scenario.output, scenario.usage)
				} else {
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(writer, `{"data":[{"b64_json":%q,"revised_prompt":"private revised prompt"}],"usage":%s}`, scenario.output, scenario.usage)
				}
			}))
			t.Cleanup(upstream.Close)
			server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
			controls := json.RawMessage(fmt.Sprintf(`{"surface":"images","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1,"stream":%t,"partial_images":%d}`, scenario.stream, scenario.partials))
			accepted := hostedMediaAdmissionHTTP(t, server, "image-usage", "private image prompt", http.StatusAccepted, controls)
			id := accepted["operation_id"].(string)
			if scenario.assetLimit != 0 {
				service.assets.maxAssetBytes = scenario.assetLimit
			}
			service.runOperation("image-meter-worker", id)
			result := hostedMediaWorkerStatus(t, server, id)
			if result["state"] != scenario.state {
				t.Fatalf("image result=%v", result)
			}
			hostedMediaAdmissionHTTP(t, server, "image-usage", "private image prompt", http.StatusOK, controls)
			service.runOperation("duplicate-image-worker", id)
			observations, err := database.pendingJournalDeliveries(context.Background(), 100)
			if err != nil || len(observations) != 1 || calls.Load() != 1 {
				t.Fatalf("observations=%v calls=%d error=%v", observations, calls.Load(), err)
			}
			observation := observations[0]
			if observation.AdapterRevision != CatalogProtocolOpenAIImages+":1" {
				t.Fatalf("image meter not retained: %s", observation.AdapterRevision)
			}
			var quantities []journalQuantity
			if err := json.Unmarshal(observation.Quantities, &quantities); err != nil {
				t.Fatal(err)
			}
			byDimension := make(map[string]journalQuantity)
			for _, quantity := range quantities {
				byDimension[quantity.Dimension] = quantity
			}
			input := byDimension["input_tokens"]
			if input.Value != scenario.inputValue || input.UnknownReason != scenario.unknown || input.IncludedIn != "total_tokens" {
				t.Fatalf("input evidence=%+v", input)
			}
			if byDimension["input_image_tokens"].IncludedIn != "input_tokens" || byDimension["output_image_tokens"].IncludedIn != "output_tokens" {
				t.Fatalf("missing inclusion rules: %s", observation.Quantities)
			}
			for _, dimension := range []string{"cache_read_text_tokens", "cache_read_image_tokens"} {
				if byDimension[dimension].UnknownReason != journalQuantityUnsupported {
					t.Fatalf("unqualified cache measurement: %s", observation.Quantities)
				}
			}
			if strings.Contains(string(observation.SourceFields), "private") || strings.Contains(string(observation.SourceFields), "b64_json") {
				t.Fatalf("content retained in metering evidence: %s", observation.SourceFields)
			}
			if scenario.inputValue != "" && !strings.Contains(string(observation.SourceFields), `"path":"usage.input_tokens","value":"`+scenario.inputValue+`"`) {
				t.Fatalf("source precision lost: %s", observation.SourceFields)
			}
			entry := read("")["requests"].([]any)[0].(map[string]any)
			if entry["usage_state"] != string(journalUsageUnknown) {
				t.Fatalf("unqualified cache usage became complete: %v", entry)
			}
		})
	}
}

func TestHostedImageUsageObsoleteResponseCannotRecordEvidence(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	encoded := base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		close(entered)
		select {
		case <-release:
		case <-request.Context().Done():
			return
		}
		fmt.Fprintf(writer, `{"data":[{"b64_json":%q}],"usage":{"input_tokens":10}}`, encoded)
	}))
	t.Cleanup(upstream.Close)
	server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
	accepted := hostedMediaAdmissionHTTP(t, server, "obsolete-image-observation", "private image prompt", http.StatusAccepted)
	id := accepted["operation_id"].(string)
	go func() { defer close(done); service.runOperation("obsolete-image-worker", id) }()
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }); waitHostedMediaWorker(t, done) })
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("image provider did not receive request")
	}
	if err := database.database.Model(&mediaOperationClaimRecord{}).Where("operation_id = ?", id).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	service.runOperation("replacement-image-worker", id)
	releaseOnce.Do(func() { close(release) })
	waitHostedMediaWorker(t, done)
	result := hostedMediaWorkerStatus(t, server, id)
	entry := read("")["requests"].([]any)[0].(map[string]any)
	observations, err := database.pendingJournalDeliveries(context.Background(), 100)
	if err != nil || len(observations) != 0 || calls.Load() != 1 || result["state"] != MediaOperationStateUncertain || len(result["outputs"].([]any)) != 0 || entry["usage_state"] != string(journalUsageUnknown) {
		t.Fatalf("obsolete response result=%v journal=%v calls=%d observations=%v error=%v", result, entry, calls.Load(), observations, err)
	}
}

func TestHostedImageUsagePersistenceFailurePreventsPublication(t *testing.T) {
	for _, failedTable := range []string{"managed_journal_observation_records", "managed_journal_delivery_records"} {
		t.Run(failedTable, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			encoded := base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				fmt.Fprintf(writer, `{"data":[{"b64_json":%q}],"usage":{"input_tokens":10}}`, encoded)
			}))
			t.Cleanup(upstream.Close)
			core, logs := observer.New(zap.InfoLevel)
			server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL), func(service *mediaOperationService) { service.logger = zap.New(core).Sugar() })
			accepted := hostedMediaAdmissionHTTP(t, server, "failed-observation", "private image prompt", http.StatusAccepted)
			id := accepted["operation_id"].(string)
			if err := database.database.Exec("CREATE TRIGGER reject_image_usage BEFORE INSERT ON " + failedTable + " BEGIN SELECT RAISE(ABORT, 'controlled image evidence failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			service.runOperation("failed-observation-worker", id)
			reports := logs.FilterMessage("media operation persistence failed").All()
			if len(reports) != 1 || reports[0].ContextMap()["operation_id"] != id || reports[0].ContextMap()["phase"] != "observe_usage" || !strings.Contains(reports[0].ContextMap()["error"].(string), "controlled image evidence failure") {
				t.Fatalf("usage write report=%v", reports)
			}
			result := hostedMediaWorkerStatus(t, server, id)
			if result["state"] != MediaOperationStateUncertain || result["error"].(map[string]any)["code"] != "usage_journal_unavailable" || len(result["outputs"].([]any)) != 0 {
				t.Fatalf("published without durable evidence: %v", result)
			}
			var count int64
			if err := database.database.Model(&managedJournalObservationRecord{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("partial observation count=%d error=%v", count, err)
			}
			if err := database.database.Exec("DROP TRIGGER reject_image_usage").Error; err != nil {
				t.Fatal(err)
			}
			service.runOperation("replacement-worker", id)
			hostedMediaAdmissionHTTP(t, server, "failed-observation", "private image prompt", http.StatusOK)
			entry := read("")["requests"].([]any)[0].(map[string]any)
			if calls.Load() != 1 || entry["state"] != string(journalRequestUncertain) || entry["usage_state"] != string(journalUsageUnknown) {
				t.Fatalf("lost evidence replay calls=%d journal=%v", calls.Load(), entry)
			}
		})
	}
}
