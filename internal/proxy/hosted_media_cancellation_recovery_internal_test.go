package proxy

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

type mediaCancellationStorageFailure struct {
	name, phase, table string
	occurrence         int64
}

func (failure mediaCancellationStorageFailure) install(t *testing.T, database *gorm.DB) func() {
	t.Helper()
	var register func(string, func(*gorm.DB)) error
	var remove func(string) error
	callbacks := database.Callback()
	switch failure.phase {
	case "query":
		register, remove = callbacks.Query().Before("gorm:query").Register, callbacks.Query().Remove
	case "update":
		register, remove = callbacks.Update().Before("gorm:update").Register, callbacks.Update().Remove
	case "create":
		register, remove = callbacks.Create().Before("gorm:create").Register, callbacks.Create().Remove
	case "delete":
		register, remove = callbacks.Delete().Before("gorm:delete").Register, callbacks.Delete().Remove
	default:
		t.Fatalf("unknown cancellation failure phase %q", failure.phase)
	}
	var calls, failures atomic.Int64
	const callbackName = "test:media_cancellation_failure"
	if err := register(callbackName, func(tx *gorm.DB) {
		if !tx.DryRun && tx.Statement.Table == failure.table && calls.Add(1) == failure.occurrence {
			failures.Add(1)
			tx.AddError(errors.New("controlled_media_cancellation_failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = remove(callbackName) })
	return func() {
		t.Helper()
		if err := remove(callbackName); err != nil {
			t.Fatal(err)
		}
		if failures.Load() != 1 {
			t.Fatalf("cancellation storage failures=%d want=1", failures.Load())
		}
	}
}

func cancelFundedMediaHTTP(t *testing.T, server *httptest.Server, operationID string, want int) map[string]any {
	t.Helper()
	request, err := http.NewRequest(http.MethodPut, server.URL+llmproxycontract.MediaOperationsPath+"/"+operationID+"/cancellation", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want {
		t.Fatalf("cancellation status=%d want=%d body=%v", response.StatusCode, want, body)
	}
	if want == http.StatusInternalServerError && body["error"].(map[string]any)["code"] != errMediaOperationStore.Error() {
		t.Fatalf("cancellation exposed an incorrect storage error: %v", body)
	}
	return body
}

func TestHostedMediaQueuedCancellationFailuresPreserveFundsAndRecoverOnce(t *testing.T) {
	for _, failure := range []mediaCancellationStorageFailure{
		{"operation-lock", "update", "media_operation_records", 1},
		{"operation-terminal", "update", "media_operation_records", 2},
		{"input-release", "update", "media_operation_asset_reference_records", 1},
		{"usage-read", "query", "media_operation_usage_delivery_records", 1},
		{"usage-event", "create", "managed_usage_event_records", 1},
		{"usage-delivery", "create", "media_operation_usage_delivery_records", 1},
		{"journal-read", "query", "managed_journal_request_records", 1},
		{"attempt-read", "query", "managed_journal_attempt_records", 1},
		{"journal-terminal", "update", "managed_journal_request_records", 1},
		{"claim-release", "delete", "media_operation_claim_records", 1},
	} {
		t.Run(failure.name, func(t *testing.T) {
			fixture := newFundedMediaRecoveryFixture(t)
			before := fixture.state(t)
			operation := hostedMediaWorkerStatus(t, fixture.server, fixture.operationID)
			remove := failure.install(t, fixture.database.database)
			cancelFundedMediaHTTP(t, fixture.server, fixture.operationID, http.StatusInternalServerError)
			remove()
			if !reflect.DeepEqual(operation, hostedMediaWorkerStatus(t, fixture.server, fixture.operationID)) || !reflect.DeepEqual(before, fixture.state(t)) || fixture.calls.Load() != 0 {
				t.Fatal("failed cancellation changed queued work or financial resources")
			}
			var previous map[string]any
			for iteration := 0; iteration < 2; iteration++ {
				database := openJournalTransactionInstance(t, fixture.database)
				server, worker := newFundedMediaRecoveryWorker(t, database, fixture.upstreamURL, fixture.worker.assets)
				cancelled := cancelFundedMediaHTTP(t, server, fixture.operationID, http.StatusOK)
				worker.runOperation("cancelled-recovery-worker", fixture.operationID)
				if err := database.reconcileHostedFunds(t.Context(), worker.store.now()); err != nil {
					t.Fatal(err)
				}
				replay := hostedMediaAdmissionHTTP(t, server, mediaRecoveryKey, mediaRecoveryPrompt, http.StatusOK)
				if cancelled["state"] != MediaOperationStateCancelled || replay["state"] != MediaOperationStateCancelled || replay["operation_id"] != fixture.operationID || fixture.calls.Load() != 0 {
					t.Fatalf("cancelled recovery changed accepted work: cancellation=%v replay=%v calls=%d", cancelled, replay, fixture.calls.Load())
				}
				assertHostedFundsBalance(t, database, 500, 500)
				financial := fixture.state(t)
				if iteration > 0 && !reflect.DeepEqual(previous, financial) {
					t.Fatal("cancellation replay repeated financial effects")
				}
				previous = financial
				server.Close()
			}
		})
	}
}

func TestHostedMediaRunningCancellationFailuresRetainUncertainFunds(t *testing.T) {
	for _, scenario := range []struct {
		failure mediaCancellationStorageFailure
		state   string
	}{
		{mediaCancellationStorageFailure{"intent-write", "update", "media_operation_records", 2}, MediaCancellationNotRequested},
		{mediaCancellationStorageFailure{"journal-read", "query", "managed_journal_request_records", 1}, MediaCancellationNotRequested},
		{mediaCancellationStorageFailure{"adapter-read", "query", "media_operation_records", 2}, MediaCancellationRequested},
		{mediaCancellationStorageFailure{"unsupported-write", "update", "media_operation_records", 3}, MediaCancellationRequested},
	} {
		t.Run(scenario.failure.name, func(t *testing.T) {
			started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			releaseProvider := func() { releaseOnce.Do(func() { close(release) }) }
			fixture := newFundedMediaRecoveryFixtureWithResponse(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				close(started)
				select {
				case <-request.Context().Done():
				case <-release:
					writer.WriteHeader(http.StatusServiceUnavailable)
				}
			}))
			t.Cleanup(func() {
				releaseProvider()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("media cancellation fixture worker did not stop")
				}
				fixture.worker.workers.Wait()
			})
			go func() {
				defer close(done)
				fixture.worker.runOperation("running-cancellation-worker", fixture.operationID)
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("funded provider request did not start")
			}
			before := fixture.state(t)
			remove := scenario.failure.install(t, fixture.database.database)
			cancelFundedMediaHTTP(t, fixture.server, fixture.operationID, http.StatusInternalServerError)
			remove()
			current := hostedMediaWorkerStatus(t, fixture.server, fixture.operationID)
			if current["state"] != MediaOperationStateRunning || current["cancellation_state"] != scenario.state || !reflect.DeepEqual(before, fixture.state(t)) || fixture.calls.Load() != 1 {
				t.Fatalf("failed cancellation changed accepted work: operation=%v calls=%d", current, fixture.calls.Load())
			}
			cancelled := cancelFundedMediaHTTP(t, fixture.server, fixture.operationID, http.StatusOK)
			if cancelled["state"] != MediaOperationStateRunning || cancelled["cancellation_state"] != MediaCancellationUnsupported || !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatalf("unsupported cancellation changed funds or execution: %v", cancelled)
			}
			releaseProvider()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("provider failure did not finish media execution")
			}
			fixture.assertFailure(t, MediaOperationStateUncertain, 1)
			fixture.recover(t, MediaOperationStateUncertain)
		})
	}
}
