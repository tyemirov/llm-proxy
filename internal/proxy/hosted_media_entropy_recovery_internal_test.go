package proxy

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type mediaWorkerEntropyFailure struct {
	next   io.Reader
	failAt int64
	reads  atomic.Int64
	failed atomic.Bool
}

func (source *mediaWorkerEntropyFailure) Read(buffer []byte) (int, error) {
	if source.reads.Add(1) == source.failAt {
		source.failed.Store(true)
		return 0, errors.New("controlled_media_entropy_failure")
	}
	return source.next.Read(buffer)
}

func TestHostedMediaWorkerEntropyFailuresPreserveFundsAndRecovery(t *testing.T) {
	for _, scenario := range []struct {
		name, state               string
		failAt, calls, deliveries int64
	}{
		{"dispatch-identifier", MediaOperationStateFailed, 1, 0, 1},
		{"terminal-evidence", MediaOperationStateRunning, 2, 1, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundedMediaRecoveryFixtureWithResponse(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusBadRequest)
			}))
			before := fixture.state(t)
			core, logs := observer.New(zap.ErrorLevel)
			fixture.worker.logger = zap.New(core).Sugar()
			source := &mediaWorkerEntropyFailure{next: rand.Reader, failAt: scenario.failAt}
			// All request setup is complete. Restore the external reader only
			// after this worker and its adapter goroutine have both stopped.
			func() {
				rand.Reader = source
				defer func() {
					fixture.worker.workers.Wait()
					rand.Reader = source.next
				}()
				fixture.worker.runOperation("entropy-failure-worker", fixture.operationID)
			}()
			if !source.failed.Load() {
				t.Fatal("worker entropy failure was not exercised")
			}
			fixture.assertFailure(t, scenario.state, scenario.calls)
			if !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatal("entropy failure changed financial resources")
			}
			assertFundsCreditRemainder(t, fixture.database, "0", "1")
			reports := logs.FilterMessage("media operation worker failed").All()
			if len(reports) != 1 || reports[0].ContextMap()["operation_id"] != fixture.operationID ||
				!strings.Contains(fmt.Sprint(reports[0].ContextMap()["error"]), "controlled_media_entropy_failure") {
				t.Fatalf("worker entropy failure reports=%v", reports)
			}
			for _, model := range []any{&managedJournalObservationRecord{}, &managedJournalDeliveryRecord{}, &managedFundsSettlementRecord{}} {
				var count int64
				if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("entropy failure retained partial %T records=%d error=%v", model, count, err)
				}
			}
			var deliveries int64
			if err := fixture.database.database.Model(&mediaOperationUsageDeliveryRecord{}).Count(&deliveries).Error; err != nil || deliveries != scenario.deliveries {
				t.Fatalf("media usage deliveries=%d want=%d error=%v", deliveries, scenario.deliveries, err)
			}
			var attempts int64
			if err := fixture.database.database.Model(&managedJournalAttemptRecord{}).Count(&attempts).Error; err != nil || attempts != scenario.calls {
				t.Fatalf("entropy failure retained attempts=%d want=%d error=%v", attempts, scenario.calls, err)
			}
			fixture.recover(t, scenario.state)
			assertFundsCreditRemainder(t, fixture.database, "0", "1")
		})
	}
}

func TestHostedMediaUsageIdentifierFailurePreventsPublication(t *testing.T) {
	fixture := newFundedMediaRecoveryFixture(t)
	before := fixture.state(t)
	source := &mediaWorkerEntropyFailure{next: rand.Reader, failAt: 2}
	func() {
		rand.Reader = source
		defer func() {
			fixture.worker.workers.Wait()
			rand.Reader = source.next
		}()
		fixture.worker.runOperation("usage-identifier-worker", fixture.operationID)
	}()
	if !source.failed.Load() {
		t.Fatal("usage identifier failure was not exercised")
	}
	fixture.assertFailure(t, MediaOperationStateUncertain, 1)
	current := hostedMediaWorkerStatus(t, fixture.server, fixture.operationID)
	if current["error"].(map[string]any)["code"] != "usage_journal_unavailable" {
		t.Fatalf("usage identifier error=%v", current)
	}
	if !reflect.DeepEqual(before, fixture.state(t)) {
		t.Fatal("usage identifier failure changed financial resources")
	}
	for _, model := range []any{&managedJournalObservationRecord{}, &managedJournalDeliveryRecord{}, &managedChargeRecord{}, &managedFundsSettlementRecord{}} {
		var count int64
		if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("usage identifier failure retained partial %T records=%d error=%v", model, count, err)
		}
	}
	fixture.recover(t, MediaOperationStateUncertain)
	assertFundsCreditRemainder(t, fixture.database, "0", "1")
}
