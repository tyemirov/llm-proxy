package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

type fundsDecisionFixture struct {
	database    *gormManagedTenantDatabase
	server      *httptest.Server
	cookie      func(string) *http.Cookie
	reservation managedFundsReservationRecord
	path        string
	body        string
}

func newFundsDecisionFixture(t *testing.T) fundsDecisionFixture {
	t.Helper()
	database, server, cookie, reservation := newFundsResolutionFixture(t)
	return fundsDecisionFixture{database, server, cookie, reservation,
		"/billing-accounts/billing-journal/requests/" + reservation.RequestID + "/funds-resolution",
		fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":"3","denominator":"200"},"reason":"approved_exception","evidence_reference":"review-recovery"}`, reservation.Revision)}
}

func (fixture fundsDecisionFixture) state(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{
		"balance":     paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK),
		"entries":     paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/ledger-entries?limit=100", "", "", http.StatusOK),
		"reservation": fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodGet, "/billing-accounts/billing-journal/reservations/"+fixture.reservation.RequestID, "", http.StatusOK),
	}
}

func (fixture fundsDecisionFixture) assertUnchanged(t *testing.T, before map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(before, fixture.state(t)) {
		t.Fatal("failed decision changed funds or Ledger history")
	}
	assertFundsCreditRemainder(t, fixture.database, "0", "1")
	for _, model := range []any{&managedFundsSettlementRecord{}, &managedFundsResolutionRecord{}} {
		var count int64
		if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("partial decision %T: count=%d error=%v", model, count, err)
		}
	}
}

func (fixture fundsDecisionFixture) recover(t *testing.T, before map[string]any) {
	t.Helper()
	result := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.path, fixture.body+" \n\t", http.StatusOK)
	if result["settled_cents"] != "1" {
		t.Fatalf("incorrect settlement: %v", result)
	}
	assertHostedFundsBalance(t, fixture.database, 4, 4)
	assertFundsCreditRemainder(t, fixture.database, "1", "200")
	after := fixture.state(t)
	beforeEntries := before["entries"].(map[string]any)["entries"].([]any)
	afterEntries := after["entries"].(map[string]any)["entries"].([]any)
	if len(afterEntries) != len(beforeEntries)+2 {
		t.Fatalf("expected one hold release and one debit: before=%v after=%v", beforeEntries, afterEntries)
	}
	restarted, cookie := newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, fixture.database))
	replayed := fundsResolutionHTTP(t, restarted, cookie("operator"), http.MethodPut, fixture.path, fixture.body, http.StatusOK)
	read := fundsResolutionHTTP(t, restarted, cookie("owner"), http.MethodGet, fixture.path, "", http.StatusOK)
	if !reflect.DeepEqual(result, replayed) || !reflect.DeepEqual(result, read) || !reflect.DeepEqual(after, fixture.state(t)) {
		t.Fatal("decision replay changed report or financial effects")
	}
}

func TestHostedFundsResolutionWriteFailuresPreserveCompleteFinancialState(t *testing.T) {
	for _, scenario := range []struct{ name, statement string }{
		{"request-lock", "BEFORE UPDATE ON managed_journal_request_records"},
		{"account-lock", "BEFORE UPDATE ON managed_billing_account_records"},
		{"hold-release", "BEFORE UPDATE ON reservations"},
		{"release-entry", "BEFORE INSERT ON ledger_entries"},
		{"debit-entry", "BEFORE INSERT ON ledger_entries WHEN NEW.amount_cents = -1"},
		{"settlement", "BEFORE INSERT ON managed_funds_settlement_records"},
		{"remainder", "BEFORE UPDATE ON managed_funds_account_records"},
		{"tenant-usage", "BEFORE UPDATE ON managed_funds_tenant_records"},
		{"reservation", "BEFORE UPDATE ON managed_funds_reservation_records"},
		{"decision", "BEFORE INSERT ON managed_funds_resolution_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsDecisionFixture(t)
			before := fixture.state(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_funds_decision " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_funds_decision_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			failure := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.path, fixture.body, http.StatusInternalServerError)
			if strings.Contains(fmt.Sprint(failure), "controlled_") {
				t.Fatal("private database error exposed")
			}
			fixture.assertUnchanged(t, before)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_funds_decision").Error; err != nil {
				t.Fatal(err)
			}
			fixture.recover(t, before)
		})
	}
}

func TestHostedFundsResolutionReadFailuresPreserveCompleteFinancialState(t *testing.T) {
	// Capture the successful operation's real reads so later reads of the same
	// table receive the same rollback checks as its first read.
	baseline := newFundsDecisionFixture(t)
	var readsMutex sync.Mutex
	var tables []string
	const callbackName = "test:funds_decision_read"
	observe := func(tx *gorm.DB) {
		if !tx.DryRun {
			readsMutex.Lock()
			tables = append(tables, tx.Statement.Table)
			readsMutex.Unlock()
		}
	}
	queries := baseline.database.database.Callback().Query()
	rows := baseline.database.database.Callback().Row()
	if err := queries.Before("gorm:query").Register(callbackName, observe); err != nil {
		t.Fatal(err)
	}
	if err := rows.Before("gorm:row").Register(callbackName, observe); err != nil {
		t.Fatal(err)
	}
	fundsResolutionHTTP(t, baseline.server, baseline.cookie("operator"), http.MethodPut, baseline.path, baseline.body, http.StatusOK)
	if err := queries.Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	if err := rows.Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	readsMutex.Lock()
	readTables := append([]string(nil), tables...)
	readsMutex.Unlock()
	if len(readTables) == 0 {
		t.Fatal("resolution made no database reads")
	}
	for index, table := range readTables {
		t.Run(fmt.Sprintf("%02d-%s", index+1, table), func(t *testing.T) {
			fixture := newFundsDecisionFixture(t)
			before := fixture.state(t)
			var reads, failures atomic.Int64
			var enabled atomic.Bool
			reject := func(tx *gorm.DB) {
				if enabled.Load() && !tx.DryRun && reads.Add(1) == int64(index+1) {
					failures.Add(1)
					tx.AddError(errors.New("controlled_funds_decision_read_failure"))
				}
			}
			queries := fixture.database.database.Callback().Query()
			rows := fixture.database.database.Callback().Row()
			if err := queries.Before("gorm:query").Register(callbackName, reject); err != nil {
				t.Fatal(err)
			}
			if err := rows.Before("gorm:row").Register(callbackName, reject); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := queries.Remove(callbackName); err != nil {
					t.Error(err)
				}
				if err := rows.Remove(callbackName); err != nil {
					t.Error(err)
				}
			})
			for range 2 {
				reads.Store(0)
				enabled.Store(true)
				failure := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.path, fixture.body, http.StatusInternalServerError)
				enabled.Store(false)
				if strings.Contains(fmt.Sprint(failure), "controlled_") {
					t.Fatal("resolution exposed its database failure")
				}
				fixture.assertUnchanged(t, before)
			}
			if failures.Load() != 2 {
				t.Fatalf("resolution read failures=%d", failures.Load())
			}
			fixture.recover(t, before)
		})
	}
}

func TestHostedFundsResolutionBoundaryFailuresPreserveFinancialState(t *testing.T) {
	for _, scenario := range []struct {
		name, table string
		read        int64
		failure     error
		status      int
	}{
		{"missing-reservation", "managed_funds_reservation_records", 1, gorm.ErrRecordNotFound, http.StatusNotFound},
		{"later-credit-list", "managed_charge_adjustment_records", 2, errors.New("controlled_resolution_credit_list_failure"), http.StatusInternalServerError},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsDecisionFixture(t)
			before := fixture.state(t)
			var reads, failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			const callbackName = "test:resolution_boundary"
			if err := callback.Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement.Table == scenario.table && reads.Add(1) == scenario.read {
					failures.Add(1)
					tx.AddError(scenario.failure)
				}
			}); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				reads.Store(0)
				response := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.path, fixture.body, scenario.status)
				if strings.Contains(fmt.Sprint(response), "controlled_") {
					t.Fatal("resolution exposed its database failure")
				}
			}
			if err := callback.Remove(callbackName); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 2 {
				t.Fatalf("resolution boundary failures=%d", failures.Load())
			}
			fixture.assertUnchanged(t, before)
			fixture.recover(t, before)
		})
	}
	t.Run("corrupt-retained-price", func(t *testing.T) {
		fixture := newFundsDecisionFixture(t)
		before := fixture.state(t)
		var price managedPriceSnapshotRecord
		if err := fixture.database.database.First(&price).Error; err != nil {
			t.Fatal(err)
		}
		if err := fixture.database.database.Model(&price).UpdateColumn("digest", "invalid-price-digest").Error; err != nil {
			t.Fatal(err)
		}
		for range 2 {
			response := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.path, fixture.body, http.StatusInternalServerError)
			if !reflect.DeepEqual(response, map[string]any{"error": map[string]any{"code": "billing_account_store_failed"}}) {
				t.Fatalf("corrupt price response=%v", response)
			}
		}
		// Restore the price before reading resources whose exposure includes it.
		if err := fixture.database.database.Model(&price).UpdateColumn("digest", sha256Hex(string(price.Document))).Error; err != nil {
			t.Fatal(err)
		}
		fixture.assertUnchanged(t, before)
		fixture.recover(t, before)
	})
}

func TestHostedFundsResolutionRejectsInvalidCommandsWithoutFinancialChanges(t *testing.T) {
	for _, scenario := range []string{"malformed", "unknown-field", "multiple-json", "trailing-garbage", "trailing-null", "zero-revision", "invalid-reason", "long-reference", "reference-newline", "reference-padding", "query", "missing-request", "missing-decision"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newFundsDecisionFixture(t)
			before := fixture.state(t)
			path, body, method, status := fixture.path, fixture.body, http.MethodPut, http.StatusBadRequest
			switch scenario {
			case "malformed":
				body = "{"
			case "unknown-field":
				body = strings.TrimSuffix(body, "}") + `,"extra":true}`
			case "multiple-json":
				body += " {}"
			case "trailing-garbage":
				body += " invalid"
			case "trailing-null":
				body += " null"
			case "zero-revision":
				body = strings.Replace(body, fmt.Sprintf(`"revision":%d`, fixture.reservation.Revision), `"revision":0`, 1)
			case "invalid-reason":
				body = strings.ReplaceAll(body, "approved_exception", "invalid reason")
			case "long-reference", "reference-newline", "reference-padding":
				values := map[string]string{"long-reference": strings.Repeat("x", 257), "reference-newline": "review\nprivate", "reference-padding": " review "}
				encoded, err := json.Marshal(values[scenario])
				if err != nil {
					t.Fatal(err)
				}
				body = strings.Replace(body, `"review-recovery"`, string(encoded), 1)
			case "query":
				path += "?unexpected=true"
			case "missing-request":
				path = strings.Replace(path, fixture.reservation.RequestID, "absent-request", 1)
				status = http.StatusNotFound
			case "missing-decision":
				method, body, status = http.MethodGet, "", http.StatusNotFound
			}
			fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), method, path, body, status)
			fixture.assertUnchanged(t, before)
			fixture.recover(t, before)
		})
	}
}

func TestHostedFundsResolutionCompletedReadsFailWithoutPartialReports(t *testing.T) {
	for _, table := range []string{"managed_funds_resolution_records", "managed_funds_settlement_records"} {
		t.Run(table, func(t *testing.T) {
			fixture := newFundsDecisionFixture(t)
			expected := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.path, fixture.body, http.StatusOK)
			before := fixture.state(t)
			callback := fixture.database.database.Callback().Query()
			var failures atomic.Int64
			if err := callback.Before("gorm:query").Register("test:completed_decision_read", func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					failures.Add(1)
					tx.AddError(errors.New("controlled_completed_decision_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			response := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodGet, fixture.path, "", http.StatusInternalServerError)
			if err := callback.Remove("test:completed_decision_read"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() == 0 || response["customer_charge"] != nil || strings.Contains(fmt.Sprint(response), "controlled_") {
				t.Fatalf("partial or private report: %v", response)
			}
			recovered := fundsResolutionHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, fixture.path, "", http.StatusOK)
			if !reflect.DeepEqual(expected, recovered) || !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatal("report recovery changed financial records")
			}
		})
	}
}

func TestHostedFundsResolutionRejectsInconsistentRetainedReceipt(t *testing.T) {
	fixture := newFundsDecisionFixture(t)
	expected := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.path, fixture.body, http.StatusOK)
	before := fixture.state(t)
	var original managedFundsResolutionRecord
	if err := fixture.database.database.First(&original).Error; err != nil {
		t.Fatal(err)
	}
	var settlement managedFundsSettlementRecord
	if err := fixture.database.database.First(&settlement).Error; err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name, table, column string
		value, original     any
	}{
		{"different-charge", "managed_funds_resolution_records", "charge_numerator", "999", original.ChargeNumerator},
		{"invalid-charge", "managed_funds_resolution_records", "charge_numerator", "invalid", original.ChargeNumerator},
		{"negative-charge", "managed_funds_resolution_records", "charge_numerator", "-3", original.ChargeNumerator},
		{"zero-denominator", "managed_funds_resolution_records", "charge_denominator", "0", original.ChargeDenominator},
		{"invalid-reason", "managed_funds_resolution_records", "reason", "invalid reason", original.Reason},
		{"missing-time", "managed_funds_resolution_records", "created_at", time.Time{}, original.CreatedAt},
		{"missing-revision", "managed_funds_resolution_records", "reservation_revision", uint64(0), original.ReservationRevision},
		{"different-settlement", "managed_funds_settlement_records", "charge_numerator", "4", settlement.ChargeNumerator},
		{"different-cents", "managed_funds_settlement_records", "settled_cents", int64(2), settlement.SettledCents},
		{"invalid-before", "managed_funds_settlement_records", "remainder_before_denominator", "0", settlement.RemainderBeforeDenominator},
		{"excess-before", "managed_funds_settlement_records", "remainder_before_numerator", "1", settlement.RemainderBeforeNumerator},
		{"different-after", "managed_funds_settlement_records", "remainder_after_numerator", "2", settlement.RemainderAfterNumerator},
		{"invalid-after", "managed_funds_settlement_records", "remainder_after_denominator", "0", settlement.RemainderAfterDenominator},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			write := func(value any) {
				t.Helper()
				result := fixture.database.database.Table(scenario.table).Where("request_id = ?", original.RequestID).UpdateColumn(scenario.column, value)
				if result.Error != nil || result.RowsAffected != 1 {
					t.Fatalf("retained receipt write rows=%d error=%v", result.RowsAffected, result.Error)
				}
			}
			write(scenario.value)
			t.Cleanup(func() { write(scenario.original) })
			corruptState := fixture.state(t)
			for _, owner := range []string{"owner", "operator"} {
				failure := fundsResolutionHTTP(t, fixture.server, fixture.cookie(owner), http.MethodGet, fixture.path, "", http.StatusInternalServerError)
				if !reflect.DeepEqual(failure, map[string]any{"error": map[string]any{"code": "billing_account_store_failed"}}) {
					t.Fatalf("resolution exposed inconsistent financial evidence: %v", failure)
				}
			}
			fundsResolutionHTTP(t, fixture.server, fixture.cookie("other"), http.MethodGet, fixture.path, "", http.StatusNotFound)
			if !reflect.DeepEqual(corruptState, fixture.state(t)) {
				t.Fatal("inconsistent resolution read changed financial effects")
			}
			assertFundsCreditRemainder(t, fixture.database, "1", "200")
		})
		restarted, cookie := newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, fixture.database))
		for range 2 {
			read := fundsResolutionHTTP(t, restarted, cookie("owner"), http.MethodGet, fixture.path, "", http.StatusOK)
			replayed := fundsResolutionHTTP(t, restarted, cookie("operator"), http.MethodPut, fixture.path, fixture.body, http.StatusOK)
			if !reflect.DeepEqual(expected, read) || !reflect.DeepEqual(expected, replayed) || !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatal("restored resolution changed its receipt or financial effects")
			}
		}
	}
}
