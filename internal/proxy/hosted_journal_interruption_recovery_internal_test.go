package proxy

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type journalInterruptionFixture struct {
	fundsAdmissionFixture
	request managedJournalRequestRecord
	now     *atomic.Int64
}

func newJournalInterruptionFixture(t *testing.T, checkpoint string) journalInterruptionFixture {
	t.Helper()
	fixture := newFundsAdmissionFixture(t)
	management := newInternalManagementService(t, newFakeManagedTenantDatabase(), internalManagementProviderRegistry())
	management.store.database = fixture.database
	router := fixture.management.Config.Handler.(*gin.Engine)
	router.GET(managementAPIPath+managementJournalAttemptsPath, management.listJournalAttemptsHandler())
	router.GET(managementAPIPath+managementJournalObservationsPath, management.listJournalObservationsHandler())
	fixture.generation.Close()
	now := &atomic.Int64{}
	now.Store(ratingTestAcceptanceTime().UnixNano())
	fixture.generation = newHostedIdentityHTTPServer(t, fixture.database, fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices), func(dependencies *hostedTextRequestDependencies) {
		dependencies.now = func() time.Time { return time.Unix(0, now.Load()) }
	})
	finishState, calls := "completed", int64(1)
	if checkpoint != "observed" {
		table := "managed_journal_observation_records"
		finishState = "uncertain"
		if checkpoint == "accepted" {
			table, finishState, calls = "managed_journal_attempt_records", "failed", 0
		}
		if err := fixture.database.database.Exec("CREATE TRIGGER reject_interruption_evidence BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT, 'controlled_interruption_evidence_failure'); END").Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := fixture.database.database.Exec("CREATE TRIGGER reject_interruption_finish BEFORE UPDATE OF state ON managed_journal_request_records WHEN NEW.state = '" + finishState + "' BEGIN SELECT RAISE(ABORT, 'controlled_interruption_finish_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	request := failHostedTextExecution(t, fixture, calls)
	if err := fixture.database.database.Exec("DROP TRIGGER reject_interruption_finish").Error; err != nil {
		t.Fatal(err)
	}
	if checkpoint != "observed" {
		if err := fixture.database.database.Exec("DROP TRIGGER reject_interruption_evidence").Error; err != nil {
			t.Fatal(err)
		}
	}
	want := journalRequestExecuting
	if checkpoint == "accepted" {
		want = journalRequestAccepted
	}
	if request.State != want {
		t.Fatalf("interrupted state=%s want=%s", request.State, want)
	}
	now.Store(request.ClaimExpiresAt.Add(time.Second).UnixNano())
	return journalInterruptionFixture{fixture, request, now}
}

func (fixture journalInterruptionFixture) resources(t *testing.T) map[string]any {
	t.Helper()
	resources := fixture.state(t)
	for _, suffix := range []string{"", "/attempts", "/observations", "/reconciliation-cases"} {
		resources["journal"+suffix] = accountConnectionHTTPExchange(t, fixture.management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+fixture.request.ID+suffix, "", http.StatusOK)
	}
	return resources
}

func (fixture journalInterruptionFixture) recover(t *testing.T, checkpoint string) {
	t.Helper()
	if checkpoint != "accepted" {
		hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", http.StatusConflict)
		recoverHostedTextExecution(t, fixture.fundsAdmissionFixture, fixture.request, 1)
		return
	}
	if result := hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", http.StatusOK); result != "funded result" {
		t.Fatalf("reclaimed result=%q", result)
	}
	var before map[string]any
	for iteration := range 2 {
		restarted := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, fixture.database), fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices), func(dependencies *hostedTextRequestDependencies) {
			dependencies.now = func() time.Time { return time.Unix(0, fixture.now.Load()) }
		})
		hostedIdentityHTTP(t, restarted, textExecutionRecoveryKey, "funded prompt", http.StatusOK)
		assertHostedFundsBalance(t, fixture.database, 5, 5)
		assertFundsCreditRemainder(t, fixture.database, "91", "25000")
		current := fixture.resources(t)
		if iteration == 0 {
			before = current
		} else if !reflect.DeepEqual(before, current) {
			t.Fatal("reclaimed request repeated financial or journal effects")
		}
		if fixture.calls.Load() != 1 {
			t.Fatalf("reclaimed provider calls=%d", fixture.calls.Load())
		}
		restarted.Close()
	}
}

func TestHostedJournalInterruptionRecoveryWriteFailuresPreserveEvidence(t *testing.T) {
	for _, scenario := range []struct{ checkpoint, name, statement string }{
		{"accepted", "owner", "BEFORE UPDATE OF owner_token ON managed_journal_request_records"},
		{"dispatched", "attempt", "BEFORE UPDATE OF state ON managed_journal_attempt_records WHEN NEW.state = 'uncertain'"},
		{"dispatched", "request", "BEFORE UPDATE OF state ON managed_journal_request_records WHEN NEW.state = 'uncertain'"},
		{"dispatched", "case", "BEFORE INSERT ON managed_journal_case_records"},
		{"observed", "request", "BEFORE UPDATE OF state ON managed_journal_request_records WHEN NEW.state = 'uncertain'"},
		{"observed", "case", "BEFORE INSERT ON managed_journal_case_records"},
	} {
		t.Run(scenario.checkpoint+"/"+scenario.name, func(t *testing.T) {
			fixture := newJournalInterruptionFixture(t, scenario.checkpoint)
			before := fixture.resources(t)
			calls := fixture.calls.Load()
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_interruption_recovery " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_interruption_recovery_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			body := hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", http.StatusBadGateway)
			if strings.Contains(body, "controlled_") {
				t.Fatalf("recovery exposed storage details: %s", body)
			}
			if err := fixture.database.database.Exec("DROP TRIGGER reject_interruption_recovery").Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, fixture.resources(t)) || fixture.calls.Load() != calls {
				t.Fatal("failed recovery changed evidence, funds, or provider calls")
			}
			fixture.recover(t, scenario.checkpoint)
		})
	}
}

func TestHostedJournalInterruptionRecoveryReadFailuresPreserveEvidence(t *testing.T) {
	for _, table := range []string{"managed_hosted_grant_records", "managed_journal_attempt_records"} {
		t.Run(table, func(t *testing.T) {
			fixture := newJournalInterruptionFixture(t, "accepted")
			before := fixture.resources(t)
			var failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:interruption_recovery", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Context.Value(hostedTextIdentityContextKey{}) != nil && tx.Statement.Table == table {
					failures.Add(1)
					tx.AddError(errors.New("controlled_interruption_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", http.StatusBadGateway)
			if err := callback.Remove("test:interruption_recovery"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 || fixture.calls.Load() != 0 || !reflect.DeepEqual(before, fixture.resources(t)) {
				t.Fatalf("failed recovery changed evidence or funds: failures=%d calls=%d", failures.Load(), fixture.calls.Load())
			}
			fixture.recover(t, "accepted")
		})
	}
}
