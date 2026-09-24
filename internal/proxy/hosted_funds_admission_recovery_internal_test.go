package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

type fundsAdmissionFixture struct {
	fundsStartupFixture
	generation   *httptest.Server
	upstreamURL  string
	responseRoot string
	prices       journalReservation
}

func newFundsAdmissionFixture(t *testing.T) fundsAdmissionFixture {
	t.Helper()
	database, _, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	calls := &atomic.Int64{}
	upstream := fundsUpstream(t, calls)
	root := t.TempDir()
	generation := newHostedIdentityHTTPServer(t, database, upstream.URL, root, fundsDependencies(prices))
	return fundsAdmissionFixture{fundsStartupFixture{database, management, calls}, generation, upstream.URL, root, prices}
}

func (fixture fundsAdmissionFixture) reject(t *testing.T) {
	t.Helper()
	for range 2 {
		body := hostedIdentityHTTP(t, fixture.generation, "recover-admission", "funded prompt", http.StatusServiceUnavailable)
		if !strings.Contains(body, `"code":"financial_admission_unavailable"`) || strings.Contains(body, "controlled_") {
			t.Fatalf("unsafe admission error: %s", body)
		}
	}
	if fixture.calls.Load() != 0 {
		t.Fatalf("failed admission dispatched provider work: %d", fixture.calls.Load())
	}
}

func (fixture fundsAdmissionFixture) assertRolledBack(t *testing.T, before map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(before, fixture.state(t)) {
		t.Fatal("failed admission changed financial resources")
	}
	for _, model := range []any{&managedJournalRequestRecord{}, &managedJournalAttemptRecord{}, &managedJournalObservationRecord{}, &managedJournalDeliveryRecord{}, &managedPriceSnapshotRecord{}, &managedFundsReservationRecord{}, &managedFundsAccountRecord{}} {
		var count int64
		if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("partial admission %T: count=%d error=%v", model, count, err)
		}
	}
}

func (fixture fundsAdmissionFixture) recoverAdmission(t *testing.T) {
	t.Helper()
	for range 2 {
		if result := hostedIdentityHTTP(t, fixture.generation, "recover-admission", "funded prompt", http.StatusOK); result != "funded result" {
			t.Fatalf("recovered result=%q", result)
		}
	}
	assertHostedFundsBalance(t, fixture.database, 5, 2)
	for _, model := range []any{&managedJournalRequestRecord{}, &managedPriceSnapshotRecord{}, &managedFundsReservationRecord{}} {
		var count int64
		if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("admission repeated %T: count=%d error=%v", model, count, err)
		}
	}
	restarted := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, fixture.database), fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices))
	assertHostedFundsBalance(t, fixture.database, 5, 5)
	assertFundsCreditRemainder(t, fixture.database, "91", "25000")
	before := fixture.state(t)
	if result := hostedIdentityHTTP(t, restarted, "recover-admission", "funded prompt", http.StatusOK); result != "funded result" {
		t.Fatalf("restarted result=%q", result)
	}
	if fixture.calls.Load() != 1 || !reflect.DeepEqual(before, fixture.state(t)) {
		t.Fatalf("restart replay changed effects: calls=%d", fixture.calls.Load())
	}
}

func TestHostedFundsAdmissionWriteFailuresRollBackBeforeDispatch(t *testing.T) {
	for _, scenario := range []struct{ name, statement string }{
		{"accepted-price", "BEFORE INSERT ON managed_price_snapshot_records"},
		{"account-lock", "BEFORE UPDATE ON managed_billing_account_records"},
		{"financial-account", "BEFORE INSERT ON managed_funds_account_records"},
		{"tenant-account", "BEFORE INSERT ON managed_funds_tenant_records"},
		{"ledger-hold", "BEFORE INSERT ON ledger_entries WHEN NEW.type = 'hold'"},
		{"ledger-reservation", "BEFORE INSERT ON reservations"},
		{"reservation", "BEFORE INSERT ON managed_funds_reservation_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			before := fixture.state(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_funds_admission " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_funds_admission_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			fixture.reject(t)
			fixture.assertRolledBack(t, before)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_funds_admission").Error; err != nil {
				t.Fatal(err)
			}
			fixture.recoverAdmission(t)
		})
	}
}

func TestHostedFundsAdmissionReadFailuresRollBackBeforeDispatch(t *testing.T) {
	for _, table := range []string{"managed_journal_attempt_records", "managed_price_snapshot_records", "managed_funds_account_records", "managed_payment_adjustment_records", "managed_funds_reservation_records", "managed_funds_tenant_records", "ledger_accounts"} {
		t.Run(table, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			before := fixture.state(t)
			callback := fixture.database.database.Callback().Query()
			var failures atomic.Int64
			if err := callback.Before("gorm:query").Register("test:funds_admission_read", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == table {
					failures.Add(1)
					tx.AddError(errors.New("controlled_funds_admission_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			fixture.reject(t)
			if err := callback.Remove("test:funds_admission_read"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() == 0 {
				t.Fatal("database read failure not exercised")
			}
			fixture.assertRolledBack(t, before)
			fixture.recoverAdmission(t)
		})
	}
}

func TestHostedFundsAdmissionBalanceReadFailuresRollBackBeforeDispatch(t *testing.T) {
	for _, table := range []string{"ledger_entries", "reservations", "managed_funds_reservation_records"} {
		t.Run(table, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			ratingHTTPExchange(t, fixture.management, http.MethodPut, tenantFundsLimitTestPath, `{"limit_cents":"5","revision":0}`, http.StatusOK)
			limit := ratingHTTPExchange(t, fixture.management, http.MethodGet, tenantFundsLimitTestPath, "", http.StatusOK)
			before := fixture.state(t)
			callback := fixture.database.database.Callback().Row()
			var failures atomic.Int64
			if err := callback.Before("gorm:row").Register("test:funds_admission_balance_read", func(tx *gorm.DB) {
				if tx.DryRun {
					return
				}
				if tx.Statement.Table == table {
					failures.Add(1)
					tx.AddError(errors.New("controlled_funds_admission_balance_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			fixture.reject(t)
			if err := callback.Remove("test:funds_admission_balance_read"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() == 0 {
				t.Fatal("balance read failure not exercised")
			}
			fixture.assertRolledBack(t, before)
			if !reflect.DeepEqual(limit, ratingHTTPExchange(t, fixture.management, http.MethodGet, tenantFundsLimitTestPath, "", http.StatusOK)) {
				t.Fatal("failed admission changed tenant allowance")
			}
			fixture.recoverAdmission(t)
		})
	}
}

func TestHostedFundsAdmissionRejectsCorruptAccountRemainders(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		remainder ExactMoney
	}{
		{"zero-denominator", ExactMoney{Numerator: "0", Denominator: "0"}},
		{"negative", ExactMoney{Numerator: "-1", Denominator: "1000"}},
		{"invalid-number", ExactMoney{Numerator: "private-invalid-remainder", Denominator: "1000"}},
		{"whole-cent", ExactMoney{Numerator: "1", Denominator: "100"}},
		{"above-cent", ExactMoney{Numerator: "1", Denominator: "1"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			before := fixture.state(t)
			callback := fixture.database.database.Callback().Query()
			const name = "test:funds_admission_remainder"
			var corruptReads atomic.Int64
			if err := callback.After("gorm:query").Register(name, func(transaction *gorm.DB) {
				if transaction.Error != nil || transaction.Statement.Table != "managed_funds_account_records" {
					return
				}
				if record, ok := transaction.Statement.Dest.(*managedFundsAccountRecord); ok {
					corruptReads.Add(1)
					record.RemainderNumerator = scenario.remainder.Numerator
					record.RemainderDenominator = scenario.remainder.Denominator
				}
			}); err != nil {
				t.Fatal(err)
			}
			fixture.reject(t)
			if err := callback.Remove(name); err != nil {
				t.Fatal(err)
			}
			if corruptReads.Load() != 2 {
				t.Fatalf("corrupt financial account reads=%d want=2", corruptReads.Load())
			}
			fixture.assertRolledBack(t, before)
			fixture.recoverAdmission(t)
		})
	}
}
