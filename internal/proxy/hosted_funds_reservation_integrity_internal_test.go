package proxy

import (
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

func TestHostedFundsAdmissionRejectsCorruptRetainedPrice(t *testing.T) {
	for _, failure := range []string{"digest", "document"} {
		t.Run(failure, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			before := fixture.state(t)
			callback := fixture.database.database.Callback().Query()
			const name = "test:admission_price_integrity"
			var failures atomic.Int64
			if err := callback.After("gorm:query").Register(name, func(transaction *gorm.DB) {
				if transaction.Error != nil || transaction.Statement.Table != "managed_price_snapshot_records" {
					return
				}
				record := transaction.Statement.Dest.(*managedPriceSnapshotRecord)
				if failure == "document" {
					record.Document = []byte(`{"private":"unreadable price"}`)
					record.Digest = sha256Hex(string(record.Document))
				} else {
					record.Digest = strings.Repeat("0", 64)
				}
				failures.Add(1)
			}); err != nil {
				t.Fatal(err)
			}
			fixture.reject(t)
			if err := callback.Remove(name); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 2 {
				t.Fatalf("corrupt accepted price reads=%d want=2", failures.Load())
			}
			fixture.assertRolledBack(t, before)
			fixture.recoverAdmission(t)
		})
	}
}

func TestHostedFundsAdmissionMissingAccountLockPreservesFunds(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	before := fixture.state(t)
	if err := fixture.database.database.Exec("CREATE TRIGGER ignore_billing_account_lock BEFORE UPDATE OF id ON managed_billing_account_records BEGIN SELECT RAISE(IGNORE); END").Error; err != nil {
		t.Fatal(err)
	}
	rejectFundedJournalAdmission(t, fixture, http.StatusForbidden)
	fixture.assertRolledBack(t, before)
	if err := fixture.database.database.Exec("DROP TRIGGER ignore_billing_account_lock").Error; err != nil {
		t.Fatal(err)
	}
	fixture.recoverAdmission(t)
}

func TestHostedFundsAttemptConflictsPreserveReservation(t *testing.T) {
	for _, scenario := range []struct {
		name, table, code string
		status            int
	}{
		{"price", "managed_price_snapshot_records", "usage_journal_conflict", http.StatusConflict},
		{"account", "managed_funds_reservation_records", "financial_admission_unavailable", http.StatusServiceUnavailable},
		{"maximum", "managed_funds_reservation_records", "financial_admission_unavailable", http.StatusServiceUnavailable},
		{"state", "managed_funds_reservation_records", "financial_admission_unavailable", http.StatusServiceUnavailable},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			callback := fixture.database.database.Callback().Query()
			const name = "test:attempt_reservation_integrity"
			var failures atomic.Int64
			if err := callback.After("gorm:query").Register(name, func(transaction *gorm.DB) {
				if transaction.Error != nil || transaction.Statement.Table != scenario.table || hostedTextExecutionFromContext(transaction.Statement.Context) == nil {
					return
				}
				switch scenario.name {
				case "price":
					transaction.Statement.Dest.(*managedPriceSnapshotRecord).Digest = strings.Repeat("0", 64)
				case "account":
					transaction.Statement.Dest.(*managedFundsReservationRecord).BillingAccountID = "billing-other"
				case "maximum":
					transaction.Statement.Dest.(*managedFundsReservationRecord).MaximumCents++
				case "state":
					transaction.Statement.Dest.(*managedFundsReservationRecord).State = fundsReservationReleased
				}
				failures.Add(1)
			}); err != nil {
				t.Fatal(err)
			}
			body := hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", scenario.status)
			if !strings.Contains(body, `"code":"`+scenario.code+`"`) || strings.Contains(body, "billing-other") {
				t.Fatalf("unsafe reservation conflict response: %s", body)
			}
			if err := callback.Remove(name); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 || fixture.calls.Load() != 0 {
				t.Fatalf("conflict reads=%d provider calls=%d", failures.Load(), fixture.calls.Load())
			}
			assertHostedFundsBalance(t, fixture.database, 5, 2)
			for _, model := range []any{&managedJournalAttemptRecord{}, &managedJournalObservationRecord{}, &managedJournalDeliveryRecord{}, &managedFundsSettlementRecord{}} {
				var count int64
				if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("partial conflicting attempt %T: count=%d error=%v", model, count, err)
				}
			}
			var request managedJournalRequestRecord
			if err := fixture.database.database.Where("key_digest = ?", sha256Hex(textExecutionRecoveryKey)).First(&request).Error; err != nil {
				t.Fatal(err)
			}
			retained := ratingHTTPExchange(t, fixture.management, http.MethodGet, "/billing-accounts/billing-journal/reservations/"+request.ID, "", http.StatusOK)
			if retained["state"] != fundsReservationHeld || retained["maximum_cents"] != "3" {
				t.Fatalf("conflicting read changed accepted reservation: %v", retained)
			}
			before := fixture.state(t)
			hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", http.StatusBadGateway)
			if !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatal("conflict replay changed financial resources")
			}
			recoverHostedTextExecution(t, fixture, request, 0)
		})
	}
}
