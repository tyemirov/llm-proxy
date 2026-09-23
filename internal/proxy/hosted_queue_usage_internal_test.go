package proxy

import (
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

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestHostedQueueUsageRetainsBilledUnits(t *testing.T) {
	for _, mode := range []string{"exact", "zero", "missing", "invalid", "duplicate", "submit_error", "status_error", "result_error", "malformed_result", "artifact_error", "observation_failure", "delivery_failure"} {
		t.Run(mode, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			data := imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 16, 16)))
			var submits, downloads atomic.Int64
			var upstream *httptest.Server
			headers := func(writer http.ResponseWriter) {
				writer.Header().Set("x-fal-request-id", "private-queue")
				switch mode {
				case "missing":
				case "invalid":
					writer.Header().Set("X-Fal-Billable-Units", "private invalid units")
				case "zero":
					writer.Header().Set("X-Fal-Billable-Units", "0")
				case "duplicate":
					writer.Header().Add("X-Fal-Billable-Units", "1")
					writer.Header().Add("X-Fal-Billable-Units", "2")
				default:
					writer.Header().Set("X-Fal-Billable-Units", "9007199254740993.125")
				}
			}
			upstream = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/artifact.png" {
					downloads.Add(1)
					if request.Header.Get("Authorization") != "" {
						t.Error("credential sent to artifact")
					}
					if mode == "artifact_error" {
						writer.WriteHeader(503)
						return
					}
					writer.Header().Set("Content-Type", "image/png")
					_, _ = writer.Write(data)
					return
				}
				if request.Header.Get("Authorization") != "Key hosted-queue-secret" {
					t.Error("queue credential changed")
				}
				writer.Header().Set("Content-Type", "application/json")
				switch request.Method + " " + request.URL.Path {
				case "POST /reve/2.1/text-to-image":
					submits.Add(1)
					if mode == "submit_error" {
						headers(writer)
						writer.WriteHeader(400)
						fmt.Fprint(writer, `{"error":"private failure"}`)
						return
					}
					fmt.Fprintf(writer, `{"request_id":"private-queue","status_url":%q,"response_url":%q,"cancel_url":%q}`, upstream.URL+"/reve/requests/private-queue/status", upstream.URL+"/reve/requests/private-queue", upstream.URL+"/reve/requests/private-queue/cancel")
				case "GET /reve/requests/private-queue/status":
					if mode == "status_error" {
						headers(writer)
						fmt.Fprint(writer, `{"request_id":"private-queue","status":"COMPLETED","error":"private failure"}`)
						return
					}
					fmt.Fprint(writer, `{"request_id":"private-queue","status":"COMPLETED"}`)
				case "GET /reve/requests/private-queue":
					headers(writer)
					if mode == "result_error" {
						writer.WriteHeader(503)
					}
					if mode == "malformed_result" {
						fmt.Fprint(writer, "private malformed result")
						return
					}
					fmt.Fprintf(writer, `{"images":[{"url":%q}]}`, upstream.URL+"/artifact.png")
				default:
					t.Errorf("unexpected queue request %s %s", request.Method, request.URL.Path)
					writer.WriteHeader(404)
				}
			}))
			t.Cleanup(upstream.Close)
			server, service, intent := newHostedQueueUsageFixture(t, database, upstream.URL)
			accepted := hostedSpeechHTTP(t, server, "queue-usage", intent, http.StatusAccepted)
			id := accepted["operation_id"].(string)
			failureTable := ""
			if mode == "observation_failure" {
				failureTable = "managed_journal_observation_records"
			}
			if mode == "delivery_failure" {
				failureTable = "managed_journal_delivery_records"
			}
			if failureTable != "" {
				if err := database.database.Exec("CREATE TRIGGER reject_queue_usage BEFORE INSERT ON " + failureTable + " BEGIN SELECT RAISE(ABORT, 'controlled queue evidence failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			service.runOperation("queue-worker", id)
			result := hostedMediaWorkerStatus(t, server, id)
			wantState := MediaOperationStateSucceeded
			switch mode {
			case "submit_error", "status_error":
				wantState = MediaOperationStateFailed
			case "result_error", "malformed_result", "artifact_error", "observation_failure", "delivery_failure":
				wantState = MediaOperationStateUncertain
			}
			if result["state"] != wantState {
				t.Fatalf("queue state=%v", result)
			}
			if failureTable != "" {
				if downloads.Load() != 0 || result["error"].(map[string]any)["code"] != llmproxycontract.ErrorCodeUsageJournalUnavailable {
					t.Fatalf("evidence failure published output=%v downloads=%d", result, downloads.Load())
				}
				if err := database.database.Exec("DROP TRIGGER reject_queue_usage").Error; err != nil {
					t.Fatal(err)
				}
			}
			replay := hostedSpeechHTTP(t, server, "queue-usage", intent, http.StatusOK)
			service.runOperation("duplicate-worker", id)
			if replay["operation_id"] != id || submits.Load() != 1 {
				t.Fatalf("repeated queue submission=%d replay=%v", submits.Load(), replay)
			}
			pending, err := database.pendingJournalDeliveries(t.Context(), 100)
			if err != nil {
				t.Fatal(err)
			}
			if failureTable != "" {
				if len(pending) != 0 {
					t.Fatal("partial evidence committed")
				}
				return
			}
			if len(pending) != 1 {
				t.Fatalf("queue evidence=%v", pending)
			}
			var quantities []journalQuantity
			if err := json.Unmarshal(pending[0].Quantities, &quantities); err != nil {
				t.Fatal(err)
			}
			value, reason := "9007199254740993.125", journalUnknownReason("")
			switch mode {
			case "zero":
				value = "0"
			case "missing":
				value, reason = "", journalQuantityNotReported
			case "invalid", "duplicate":
				value, reason = "", journalQuantityInvalid
			}
			if len(quantities) != 1 || quantities[0].Dimension != "billable_units" || quantities[0].Unit != "provider_unit" || quantities[0].Value != value || quantities[0].UnknownReason != reason || pending[0].AdapterRevision != CatalogProtocolFALQueueImages+":1" {
				t.Fatalf("queue codec=%s quantities=%s", pending[0].AdapterRevision, pending[0].Quantities)
			}
			if strings.Contains(string(pending[0].SourceFields), "private") || strings.Contains(string(pending[0].SourceFields), "http") {
				t.Fatal("private source evidence")
			}
			if value != "" && !strings.Contains(string(pending[0].SourceFields), `"path":"headers.x_fal_billable_units","value":"`+value+`"`) {
				t.Fatalf("queue source=%s", pending[0].SourceFields)
			}
			usage := journalUsageComplete
			if reason != "" {
				usage = journalUsageUnknown
			}
			entry := read("")["requests"].([]any)[0].(map[string]any)
			if entry["usage_state"] != string(usage) {
				t.Fatalf("queue completeness=%v", entry)
			}
		})
	}
}

func newHostedQueueUsageFixture(t *testing.T, database *gormManagedTenantDatabase, endpoint string) (*httptest.Server, *mediaOperationService, string) {
	t.Helper()
	server, service := newHostedVoiceHTTPFixture(t, database, "fal", "reve-2.1", map[string]string{CatalogCredentialAPIKey: "hosted-queue-secret"})
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-voices").Update("offerings", []byte(`[{"model":"reve-2.1","operations":["image_generation"]}]`)).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.database.Create(&managedHostedGrantRevisionRecord{GrantID: "grant-voices", Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Queue acceptance", CreatedAt: service.store.now()}).Error; err != nil {
		t.Fatal(err)
	}
	configureHostedQueueUsage(t, service, endpoint)
	return server, service, `{"capability":"image.generate","provider":"fal","model":"reve-2.1","input":{"prompt":"private prompt"},"controls":{"aspect_ratio":"1:1","output_format":"png","output_count":1}}`
}

func configureHostedQueueUsage(t *testing.T, service *mediaOperationService, endpoint string) {
	t.Helper()
	imageAdapter := service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")].(*imageGenerationAdapter)
	offering, err := service.catalog.ResolveOffering("fal", "reve-2.1")
	if err != nil {
		t.Fatal(err)
	}
	definition := service.providers.definitions[providerID("fal")]
	transport := definition.transports[offering.Transport]
	transport.endpointURLOverride = endpoint + "/reve/2.1/text-to-image"
	transport.artifactOrigins = []string{endpoint}
	definition.transports[offering.Transport] = transport
	service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "fal", "reve-2.1")] = newQueueImageAdapter(offering, definition, imageAdapter.tenants, service.store, service.assets)
}

func TestHostedQueueUsageRecoveryRetainsSingleAttempt(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	data := imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 16, 16)))
	var submits, polls, results atomic.Int64
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.Method + " " + request.URL.Path {
		case "POST /reve/2.1/text-to-image":
			submits.Add(1)
			fmt.Fprintf(writer, `{"request_id":"private-queue","status_url":%q,"response_url":%q,"cancel_url":%q}`, upstream.URL+"/reve/requests/private-queue/status", upstream.URL+"/reve/requests/private-queue", upstream.URL+"/reve/requests/private-queue/cancel")
		case "GET /reve/requests/private-queue/status":
			if polls.Add(1) == 1 {
				close(entered)
				select {
				case <-release:
				case <-request.Context().Done():
					return
				}
			}
			fmt.Fprint(writer, `{"request_id":"private-queue","status":"COMPLETED"}`)
		case "GET /reve/requests/private-queue":
			results.Add(1)
			writer.Header().Set("X-Fal-Billable-Units", "2.125")
			writer.Header().Set("x-fal-request-id", "private-queue")
			fmt.Fprintf(writer, `{"images":[{"url":%q}]}`, upstream.URL+"/artifact.png")
		case "GET /artifact.png":
			writer.Header().Set("Content-Type", "image/png")
			_, _ = writer.Write(data)
		default:
			t.Errorf("unexpected recovery request=%s", request.URL.Path)
			writer.WriteHeader(404)
		}
	}))
	t.Cleanup(upstream.Close)
	first, firstService, intent := newHostedQueueUsageFixture(t, database, upstream.URL)
	second, secondService := newHostedMediaAdmissionHTTPServer(t, openJournalTransactionInstance(t, database))
	secondService.assets = firstService.assets
	configureHostedQueueUsage(t, secondService, upstream.URL)
	id := hostedSpeechHTTP(t, first, "recovered-queue", intent, http.StatusAccepted)["operation_id"].(string)
	go func() { defer close(done); firstService.runOperation("first-queue-worker", id) }()
	t.Cleanup(func() { once.Do(func() { close(release) }); waitHostedMediaWorker(t, done) })
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("queue did not reach polling")
	}
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-voices").Update("state", hostedGrantRevoked).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.database.Model(&mediaOperationClaimRecord{}).Where("operation_id = ?", id).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	secondService.runOperation("replacement-queue-worker", id)
	once.Do(func() { close(release) })
	waitHostedMediaWorker(t, done)
	result := hostedMediaWorkerStatus(t, second, id)
	if result["state"] != MediaOperationStateSucceeded || submits.Load() != 1 || polls.Load() != 2 || results.Load() != 1 {
		t.Fatalf("recovery=%v submissions=%d polls=%d results=%d", result, submits.Load(), polls.Load(), results.Load())
	}
	pending, err := database.pendingJournalDeliveries(t.Context(), 100)
	if err != nil || len(pending) != 1 || !strings.Contains(string(pending[0].Quantities), `"value":"2.125"`) {
		t.Fatalf("recovered evidence count=%d error=%v", len(pending), err)
	}
	var attempts int64
	if err := database.database.Model(&managedJournalAttemptRecord{}).Count(&attempts).Error; err != nil || attempts != 1 {
		t.Fatalf("recovery attempts=%d error=%v", attempts, err)
	}
	entry := read("")["requests"].([]any)[0].(map[string]any)
	if entry["state"] != string(journalRequestCompleted) || entry["usage_state"] != string(journalUsageComplete) {
		t.Fatalf("recovery journal=%v", entry)
	}
}
