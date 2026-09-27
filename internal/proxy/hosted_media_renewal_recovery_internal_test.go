package proxy

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

func TestHostedMediaRenewalFailuresRetainFundsAndPreventResubmission(t *testing.T) {
	for _, scenario := range []struct{ name, statement, state string }{
		{"claim-lock", "BEFORE UPDATE OF generation ON media_operation_claim_records", MediaOperationStateRunning},
		{"claim-expiry", "BEFORE UPDATE OF expires_at ON media_operation_claim_records", MediaOperationStateUncertain},
		{"journal-expiry", "BEFORE UPDATE OF claim_expires_at ON managed_journal_request_records", MediaOperationStateUncertain},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			dispatched, release := make(chan struct{}), make(chan struct{})
			var announce, releaseOnce sync.Once
			fixture := newFundedMediaRecoveryFixtureWithResponse(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				announce.Do(func() { close(dispatched) })
				select {
				case <-request.Context().Done():
				case <-release:
					writer.WriteHeader(http.StatusServiceUnavailable)
				}
			}))
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			fixture.worker.claimRenewal = 20 * time.Millisecond
			core, logs := observer.New(zap.ErrorLevel)
			fixture.worker.logger = zap.New(core).Sugar()
			done := make(chan struct{})
			go func() {
				defer close(done)
				fixture.worker.runOperation("renewal-failure-worker", fixture.operationID)
			}()
			select {
			case <-dispatched:
			case <-time.After(5 * time.Second):
				t.Fatal("funded provider request did not start")
			}
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_media_renewal " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_media_renewal_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not stop after failed renewal")
			}
			releaseOnce.Do(func() { close(release) })
			reports := logs.FilterMessage("media operation persistence failed").All()
			renewalFailures := 0
			for _, report := range reports {
				if report.ContextMap()["phase"] == "renew_claim" && report.ContextMap()["operation_id"] == fixture.operationID {
					renewalFailures++
				}
			}
			if renewalFailures != 1 {
				t.Fatalf("renewal failure reports=%d want=1", renewalFailures)
			}
			fixture.assertFailure(t, scenario.state, 1)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_media_renewal").Error; err != nil {
				t.Fatal(err)
			}
			fixture.recover(t, scenario.state)
		})
	}
}

func TestHostedMediaAuthorizationStorageFailuresPreventProviderCalls(t *testing.T) {
	for _, scenario := range []struct{ name, state string }{
		{"claim-lock", MediaOperationStateRunning},
		{"journal-read", MediaOperationStateUncertain},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundedMediaRecoveryFixture(t)
			var dispatched atomic.Bool
			var failures atomic.Int64
			updates := fixture.database.database.Callback().Update()
			queries := fixture.database.database.Callback().Query()
			if scenario.name == "claim-lock" {
				if err := fixture.database.database.Exec(`CREATE TRIGGER reject_media_authorization BEFORE UPDATE OF generation ON media_operation_claim_records
					WHEN EXISTS (SELECT 1 FROM media_operation_records WHERE operation_id = NEW.operation_id AND provider_execution_state = 'dispatched')
					BEGIN SELECT RAISE(ABORT, 'controlled_media_authorization_lock_failure'); END`).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := updates.After("gorm:update").Register("test:media_authorization_dispatch", func(tx *gorm.DB) {
					if tx.Error == nil && tx.Statement.Table == "media_operation_records" {
						if values, ok := tx.Statement.Dest.(map[string]any); ok && values["provider_execution_state"] == MediaProviderExecutionDispatched {
							dispatched.Store(true)
						}
					}
				}); err != nil {
					t.Fatal(err)
				}
				if err := queries.Before("gorm:query").Register("test:media_authorization_read", func(tx *gorm.DB) {
					if !tx.DryRun && tx.Statement.Table == "managed_journal_request_records" && dispatched.CompareAndSwap(true, false) {
						failures.Add(1)
						tx.AddError(errors.New("controlled_media_authorization_read_failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			fixture.worker.runOperation("authorization-failure-worker", fixture.operationID)
			if scenario.name == "claim-lock" {
				if err := fixture.database.database.Exec("DROP TRIGGER reject_media_authorization").Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := queries.Remove("test:media_authorization_read"); err != nil {
					t.Fatal(err)
				}
				if err := updates.Remove("test:media_authorization_dispatch"); err != nil {
					t.Fatal(err)
				}
				if failures.Load() != 1 {
					t.Fatalf("authorization read failures=%d want=1", failures.Load())
				}
			}
			fixture.assertFailure(t, scenario.state, 0)
			fixture.recover(t, scenario.state)
		})
	}
}
