package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
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
	// Block the real storage boundary while HTTP fills the bounded queue.
	started, release := make(chan struct{}), make(chan struct{})
	var blockOnce sync.Once
	if err := database.database.Callback().Create().Before("gorm:create").Register("test:telemetry_saturation", func(tx *gorm.DB) {
		if tx.Statement.Table == managedUsageEventTable {
			blockOnce.Do(func() {
				close(started)
				select {
				case <-release:
				case <-tx.Statement.Context.Done():
					tx.AddError(tx.Statement.Context.Err())
				}
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { close(release) })
	writer := newManagedUsageWriter(service.store, 1)
	service.store.usageWriter = writer
	server := httptest.NewServer(router)
	server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
	t.Cleanup(server.Close)
	for index := range 3 {
		if body := hostedIdentityHTTP(t, server, "telemetry-full", "private prompt", http.StatusOK); body != "saved answer" {
			t.Fatalf("hosted result=%q", body)
		}
		if index == 0 {
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("telemetry did not reach the storage boundary")
			}
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
