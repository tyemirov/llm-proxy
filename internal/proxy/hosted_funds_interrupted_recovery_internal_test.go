package proxy

import (
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestHostedFundsStartupInterruptedTextPreservesFundsAcrossWriteFailures(t *testing.T) {
	for _, scenario := range []struct{ name, statement string }{
		{"attempt", "BEFORE UPDATE OF state ON managed_journal_attempt_records WHEN NEW.state = 'uncertain'"},
		{"request", "BEFORE UPDATE OF state ON managed_journal_request_records WHEN NEW.state = 'uncertain'"},
		{"case", "BEFORE INSERT ON managed_journal_case_records"},
		{"reservation", "BEFORE UPDATE ON managed_funds_reservation_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newJournalInterruptionFixture(t, "dispatched")
			before := fixture.resources(t)
			now := func() time.Time { return time.Unix(0, fixture.now.Load()) }
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_interrupted_funds " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_interrupted_funds_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			for range 2 {
				failFundsApplicationAt(t, fixture.database, fixture.management, now)
				if !reflect.DeepEqual(before, fixture.resources(t)) {
					t.Fatal("failed funds recovery changed the journal or financial resources")
				}
				if fixture.calls.Load() != 1 {
					t.Fatal("failed funds recovery repeated provider work")
				}
				assertFundsCreditRemainder(t, fixture.database, "0", "1")
			}
			if err := fixture.database.database.Exec("DROP TRIGGER reject_interrupted_funds").Error; err != nil {
				t.Fatal(err)
			}
			var recovered map[string]any
			for iteration := range 2 {
				restartFundsApplication(t, fixture.database, fixture.management, now)
				hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", http.StatusConflict)
				assertHostedFundsBalance(t, fixture.database, 5, 2)
				assertFundsCreditRemainder(t, fixture.database, "0", "1")
				current := fixture.resources(t)
				for _, resource := range []string{"ledger-entries", "charges"} {
					if !reflect.DeepEqual(before[resource], current[resource]) {
						t.Fatalf("uncertain execution changed %s", resource)
					}
				}
				var request managedJournalRequestRecord
				if err := fixture.database.database.Where("id = ?", fixture.request.ID).First(&request).Error; err != nil {
					t.Fatal(err)
				}
				var reservation managedFundsReservationRecord
				if err := fixture.database.database.Where("request_id = ?", request.ID).First(&reservation).Error; err != nil {
					t.Fatal(err)
				}
				if request.State != journalRequestUncertain || request.UsageState != journalUsageUnknown || reservation.State != fundsReservationReconciliation {
					t.Fatalf("recovery lost uncertainty: request=%s usage=%s reservation=%s", request.State, request.UsageState, reservation.State)
				}
				if iteration == 0 {
					recovered = current
				} else if !reflect.DeepEqual(recovered, current) {
					t.Fatal("repeated recovery changed the journal or financial resources")
				}
				if fixture.calls.Load() != 1 {
					t.Fatal("recovery or replay repeated provider work")
				}
			}
		})
	}
}
