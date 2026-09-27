package proxy

import (
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MarkoPoloResearchLab/ledger/pkg/gormstore"
	"gorm.io/gorm"
)

func TestHostedFinancialBalanceRejectsUnusableLedgerAccountIdentity(t *testing.T) {
	fixture := newFinancialReadFixture(t)
	before := fixture.snapshot(t)
	for _, value := range []string{"", " \t\n"} {
		t.Run(value, func(t *testing.T) {
			var failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			const name = "test:invalid_ledger_account_identity"
			if err := callback.After("gorm:query").Register(name, func(tx *gorm.DB) {
				account, ok := tx.Statement.Dest.(*gormstore.LedgerAccount)
				if !ok || tx.DryRun || tx.Error != nil || tx.Statement.Table != "ledger_accounts" {
					return
				}
				failures.Add(1)
				account.AccountID = value
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := callback.Remove(name); err != nil {
					t.Error(err)
				}
			})
			for range 2 {
				fixture.read(t, fixture.resources["balance"], fixture.cookie("owner"), http.StatusInternalServerError)
			}
			if failures.Load() != 2 {
				t.Fatalf("invalid account identity reads=%d want=2", failures.Load())
			}
		})
		fixture.assertUnchanged(t, before)
		fixture.restart(t)
		fixture.assertUnchanged(t, before)
	}
}

func TestHostedPaymentsProviderReconciliationRejectsUnencodableRetainedTimestamps(t *testing.T) {
	for _, scenario := range []struct{ name, table, column string }{
		{"attempt", "managed_journal_attempt_records", "updated_at"},
		{"request", "managed_journal_request_records", "updated_at"},
		{"charge", "managed_charge_records", "created_at"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newProviderAuditFixture(t)
			before := fixture.charges(t)
			var original string
			if err := fixture.database.database.Table(scenario.table).Select("CAST(" + scenario.column + " AS TEXT)").Scan(&original).Error; err != nil {
				t.Fatal(err)
			}
			write := func(value string) {
				t.Helper()
				result := fixture.database.database.Table(scenario.table).Where("1 = 1").UpdateColumn(scenario.column, value)
				if result.Error != nil || result.RowsAffected != 1 {
					t.Fatalf("retained timestamp write rows=%d error=%v", result.RowsAffected, result.Error)
				}
			}
			// SQLite accepts this timestamp, but the timezone hour exceeds the
			// RFC 3339 JSON range. The import must not retain a partial snapshot.
			const invalid = "2026-09-22T12:00:00+24:00"
			write(invalid)
			t.Cleanup(func() { write(original) })
			corrupted := fixture.charges(t)
			for range 2 {
				if _, err := fixture.reconcile(t, fixture.configuration); err == nil || !strings.Contains(err.Error(), "timezone hour outside of range") {
					t.Fatalf("unencodable retained timestamp produced a comparison: %v", err)
				}
				fixture.assertNoImport(t)
				var retained string
				if err := fixture.database.database.Table(scenario.table).Select("CAST(" + scenario.column + " AS TEXT)").Scan(&retained).Error; err != nil || retained != invalid {
					t.Fatalf("failed comparison changed retained timestamp: value=%q error=%v", retained, err)
				}
				if !reflect.DeepEqual(corrupted, fixture.charges(t)) {
					t.Fatal("failed timestamp encoding changed charges")
				}
			}
			write(original)
			fixture.assertRecovery(t, before)
		})
	}
}
