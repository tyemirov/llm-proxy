package proxy

import (
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

func TestHostedFundsSettlementLaterReadFailuresPreservePendingDelivery(t *testing.T) {
	for _, scenario := range []struct {
		table string
		read  int64
	}{
		{"managed_journal_observation_records", 2},
		{"managed_price_snapshot_records", 2},
		{"managed_journal_request_records", 2},
		{"managed_journal_request_records", 3},
		{"managed_journal_request_records", 4},
		{"managed_charge_records", 2},
		{"managed_journal_attempt_records", 2},
		{"managed_journal_attempt_records", 3},
		{"managed_charge_adjustment_records", 2},
		{"ledger_accounts", 2},
	} {
		t.Run(scenario.table+"/"+strconv.FormatInt(scenario.read, 10), func(t *testing.T) {
			fixture := newFundsStartupFixture(t, startupCompleteUsage)
			before := fixture.state(t)
			callback := fixture.database.database.Callback().Query()
			var reads, failures atomic.Int64
			if err := callback.Before("gorm:query").Register("test:settlement_read", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == scenario.table && reads.Add(1) == scenario.read {
					failures.Add(1)
					tx.AddError(errors.New("controlled_settlement_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			fixture.failStartup(t)
			if err := callback.Remove("test:settlement_read"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 {
				t.Fatalf("settlement read failures=%d", failures.Load())
			}
			fixture.assertPending(t, before)
			fixture.assertSettledOnce(t, before)
		})
	}
}

func TestHostedFundsSettlementLedgerReadFailureRollsBackHoldRelease(t *testing.T) {
	for _, table := range []string{"ledger_entries", "reservations"} {
		t.Run(table, func(t *testing.T) {
			fixture := newFundsStartupFixture(t, startupCompleteUsage)
			before := fixture.state(t)
			callback := fixture.database.database.Callback().Row()
			var failures atomic.Int64
			if err := callback.Before("gorm:row").Register("test:settlement_balance", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == table {
					failures.Add(1)
					tx.AddError(errors.New("controlled_settlement_balance_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			fixture.failStartup(t)
			if err := callback.Remove("test:settlement_balance"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() == 0 {
				t.Fatal("settlement balance failure was not exercised")
			}
			fixture.assertPending(t, before)
			fixture.assertSettledOnce(t, before)
		})
	}
}

func TestHostedFundsSettlementCorruptAmountsPreserveFunds(t *testing.T) {
	for _, scenario := range []struct{ table, column, original, corrupt string }{
		{"managed_funds_account_records", "remainder_denominator", "1", "0"},
		{"managed_funds_account_records", "remainder_numerator", "0", "invalid"},
		{"managed_funds_account_records", "remainder_numerator", "0", "-1"},
		{"managed_funds_account_records", "remainder_numerator", "0", "1"},
		{"managed_funds_tenant_records", "spent_denominator", "1", "0"},
		{"managed_funds_tenant_records", "spent_numerator", "0", "invalid"},
		{"managed_funds_tenant_records", "spent_numerator", "0", "-1"},
	} {
		t.Run(scenario.table+"/"+scenario.column+"/"+scenario.corrupt, func(t *testing.T) {
			fixture := newFundsStartupFixture(t, startupCompleteUsage)
			before := fixture.state(t)
			server, cookie := newFundsManagementHTTPFixture(t, fixture.database)
			limitPath := "/billing-accounts/billing-journal/tenant-limits/managed-first"
			limit := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, limitPath, "", http.StatusOK)
			update := func(value string) {
				t.Helper()
				result := fixture.database.database.Table(scenario.table).Where("billing_account_id = ?", "billing-journal").UpdateColumn(scenario.column, value)
				if result.Error != nil || result.RowsAffected != 1 {
					t.Fatalf("corrupt amount update rows=%d error=%v", result.RowsAffected, result.Error)
				}
			}
			update(scenario.corrupt)
			if scenario.table == "managed_funds_account_records" {
				failure := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", http.StatusInternalServerError)
				if !reflect.DeepEqual(failure, map[string]any{"error": map[string]any{"code": "billing_account_store_failed"}}) {
					t.Fatalf("corrupt balance response=%v", failure)
				}
			} else {
				fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, limitPath, "", http.StatusInternalServerError)
			}
			fixture.failStartup(t)
			update(scenario.original)
			fixture.assertPending(t, before)
			if !reflect.DeepEqual(limit, fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, limitPath, "", http.StatusOK)) {
				t.Fatal("failed settlement changed tenant usage")
			}
			fixture.assertSettledOnce(t, before)
		})
	}
}

func TestHostedFundsSettlementReservationConflictRollsBackDebit(t *testing.T) {
	fixture := newFundsStartupFixture(t, startupCompleteUsage)
	before := fixture.state(t)
	if err := fixture.database.database.Exec("CREATE TRIGGER ignore_settlement_reservation BEFORE UPDATE ON managed_funds_reservation_records BEGIN SELECT RAISE(IGNORE); END").Error; err != nil {
		t.Fatal(err)
	}
	fixture.failStartup(t)
	fixture.assertPending(t, before)
	if err := fixture.database.database.Exec("DROP TRIGGER ignore_settlement_reservation").Error; err != nil {
		t.Fatal(err)
	}
	fixture.assertSettledOnce(t, before)
}
