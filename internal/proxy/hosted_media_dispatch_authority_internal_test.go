package proxy

import (
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

func TestHostedMediaDispatchJournalFailuresPreventProviderCalls(t *testing.T) {
	for _, scenario := range []string{"read-error", "missing-journal", "different-owner"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newFundedMediaRecoveryFixture(t)
			before := fixture.state(t)
			var failed atomic.Bool
			queries := fixture.database.database.Callback().Query()
			const callback = "test:media_dispatch_journal"
			if err := queries.After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Error != nil || tx.DryRun || tx.Statement.Table != "managed_journal_request_records" ||
					tx.Statement.Context.Value(hostedMediaAuthorizationContextKey{}) == nil || !failed.CompareAndSwap(false, true) {
					return
				}
				switch scenario {
				case "read-error":
					tx.AddError(errors.New("controlled_dispatch_journal_read_failure"))
				case "missing-journal":
					tx.AddError(gorm.ErrRecordNotFound)
				case "different-owner":
					tx.Statement.Dest.(*managedJournalRequestRecord).OwnerToken = "different-worker"
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := queries.Remove(callback); err != nil {
					t.Error(err)
				}
			})
			fixture.worker.runOperation("dispatch-authority-worker", fixture.operationID)
			if err := queries.Remove(callback); err != nil {
				t.Fatal(err)
			}
			if !failed.Load() {
				t.Fatal("dispatch journal failure was not exercised")
			}
			fixture.assertFailure(t, MediaOperationStateUncertain, 0)
			if !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatal("dispatch journal failure changed financial resources")
			}
			assertFundsCreditRemainder(t, fixture.database, "0", "1")
			fixture.recover(t, MediaOperationStateUncertain)
			assertFundsCreditRemainder(t, fixture.database, "0", "1")
		})
	}
}
