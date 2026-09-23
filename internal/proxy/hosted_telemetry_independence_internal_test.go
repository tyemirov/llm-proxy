package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHostedUsageSurvivesTelemetrySaturation(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-telemetry","status":"completed","output_text":"saved answer","usage":{"input_tokens":9007199254740993,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	router, service, _ := newHostedIdentityHTTPHandler(t, database, upstream.URL, t.TempDir())
	// Model a full queue whose consumer cannot make progress. The real writer
	// still receives HTTP telemetry and applies its normal overflow behavior.
	writer := newManagedUsageWriter(service.store, 1)
	writer.queue <- managedUsageWrite{}
	writer.startOnce.Do(func() {})
	service.store.usageWriter = writer
	server := httptest.NewServer(router)
	server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
	t.Cleanup(server.Close)
	for range 2 {
		if body := hostedIdentityHTTP(t, server, "telemetry-full", "private prompt", http.StatusOK); body != "saved answer" {
			t.Fatalf("hosted result=%q", body)
		}
	}
	if calls.Load() != 1 || len(writer.queue) != 1 {
		t.Fatalf("calls=%d queue=%d", calls.Load(), len(writer.queue))
	}
	var telemetry int64
	if err := database.database.Model(&managedUsageEventRecord{}).Count(&telemetry).Error; err != nil || telemetry != 0 {
		t.Fatalf("telemetry=%d error=%v", telemetry, err)
	}
	pending, err := database.pendingJournalDeliveries(t.Context(), 100)
	if err != nil || len(pending) != 1 {
		t.Fatalf("financial deliveries=%v error=%v", pending, err)
	}
	if !strings.Contains(string(pending[0].Quantities), `"dimension":"input_tokens","unit":"token","value":"9007199254740993"`) {
		t.Fatalf("financial quantities=%s", pending[0].Quantities)
	}
	entries := read("")["requests"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["state"] != string(journalRequestCompleted) || entries[0].(map[string]any)["usage_state"] != string(journalUsageComplete) {
		t.Fatalf("journal=%v", entries)
	}
}
