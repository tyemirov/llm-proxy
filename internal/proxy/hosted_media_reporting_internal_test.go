package proxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHostedMediaWorkerReportsAdapterPersistenceFailures(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
	for _, phase := range []string{"persist_provider_handle", "bind_provider_request", "publish_preview", "renew_claim"} {
		t.Run(phase, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			core, logs := observer.New(zap.InfoLevel)
			var calls atomic.Int64
			releaseProvider := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				switch phase {
				case "persist_provider_handle", "bind_provider_request":
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprint(writer, `{"id":"resp_callback_failure","status":"queued"}`)
				case "publish_preview":
					writer.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(writer, "event: image_generation.partial_image\ndata: {\"type\":\"image_generation.partial_image\",\"partial_image_index\":0,\"output_format\":\"png\",\"b64_json\":%q}\n\n", encoded)
				case "renew_claim":
					if err := database.database.Exec("CREATE TRIGGER reject_media_callback BEFORE UPDATE OF expires_at ON media_operation_claim_records BEGIN SELECT RAISE(ABORT, 'controlled media callback failure'); END").Error; err != nil {
						t.Error(err)
						return
					}
					select {
					case <-request.Context().Done():
					case <-releaseProvider:
					}
				}
			}))
			t.Cleanup(upstream.Close)
			t.Cleanup(func() { close(releaseProvider) })
			server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL), func(service *mediaOperationService) {
				service.logger = zap.New(core).Sugar()
				service.claimRenewal = 20 * time.Millisecond
			})
			controls := json.RawMessage(`{"surface":"images","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1}`)
			var trigger string
			switch phase {
			case "persist_provider_handle", "bind_provider_request":
				controls = json.RawMessage(`{"surface":"responses","responses_model":"gpt-5","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1}`)
				trigger = "BEFORE UPDATE OF provider_handle ON media_operation_records WHEN NEW.provider_handle != '' AND NEW.public_state = 'running'"
				if phase == "bind_provider_request" {
					trigger = "BEFORE UPDATE OF provider_request_id ON managed_journal_attempt_records WHEN NEW.provider_request_id != ''"
				}
			case "publish_preview":
				controls = json.RawMessage(`{"surface":"images","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1,"stream":true,"partial_images":1}`)
				trigger = "BEFORE INSERT ON media_operation_partial_reference_records"
			}
			accepted := hostedMediaAdmissionHTTP(t, server, "callback-failure", "private image prompt", http.StatusAccepted, controls)
			id := accepted["operation_id"].(string)
			if trigger != "" {
				if err := database.database.Exec("CREATE TRIGGER reject_media_callback " + trigger + " BEGIN SELECT RAISE(ABORT, 'controlled media callback failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			service.runOperation("callback-worker", id)
			reports := logs.FilterMessage("media operation persistence failed").All()
			if len(reports) != 1 {
				t.Fatalf("callback reports=%v", reports)
			}
			fields := reports[0].ContextMap()
			wantPhase := phase
			if phase == "bind_provider_request" {
				wantPhase = "persist_provider_handle"
				var operation mediaOperationRecord
				if err := database.database.Where("operation_id = ?", id).First(&operation).Error; err != nil {
					t.Fatal(err)
				}
				var attempt managedJournalAttemptRecord
				if err := database.database.First(&attempt).Error; err != nil {
					t.Fatal(err)
				}
				if operation.ProviderHandle != "" || attempt.ProviderRequestID != "" {
					t.Fatalf("provider receipt transaction did not roll back: handle=%q id=%q", operation.ProviderHandle, attempt.ProviderRequestID)
				}
			}
			if fields["phase"] != wantPhase || fields["operation_id"] != id || !strings.Contains(fields["error"].(string), "controlled media callback failure") {
				t.Fatalf("callback report=%v", fields)
			}
			result := hostedMediaWorkerStatus(t, server, id)
			entry := read("")["requests"].([]any)[0].(map[string]any)
			if result["state"] != MediaOperationStateUncertain || entry["state"] != string(journalRequestUncertain) || calls.Load() != 1 {
				t.Fatalf("callback outcome=%v journal=%v calls=%d", result, entry, calls.Load())
			}
			service.runOperation("duplicate-worker", id)
			hostedMediaAdmissionHTTP(t, server, "callback-failure", "private image prompt", http.StatusOK, controls)
			if calls.Load() != 1 {
				t.Fatalf("callback failure repeated provider work: %d", calls.Load())
			}
		})
	}
}
