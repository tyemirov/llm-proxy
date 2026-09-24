package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

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
	for _, table := range []string{"managed_funds_resolution_records", "managed_journal_request_records", "managed_funds_reservation_records", "managed_price_snapshot_records", "managed_charge_records", "managed_charge_adjustment_records", "managed_funds_account_records", "managed_funds_tenant_records", "ledger_accounts", "reservations", "managed_funds_settlement_records"} {
		t.Run(table, func(t *testing.T) {
			fixture := newFundsDecisionFixture(t)
			before := fixture.state(t)
			var failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:funds_decision_read", func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					failures.Add(1)
					tx.AddError(errors.New("controlled_funds_decision_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			failure := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.path, fixture.body, http.StatusInternalServerError)
			if err := callback.Remove("test:funds_decision_read"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() == 0 || strings.Contains(fmt.Sprint(failure), "controlled_") {
				t.Fatalf("read failure not safely reported: failures=%d response=%v", failures.Load(), failure)
			}
			fixture.assertUnchanged(t, before)
			fixture.recover(t, before)
		})
	}
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
