package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

type fundsCorrectionFixture struct {
	fundsDecisionFixture
	creditPath string
	creditBody string
}

func newFundsCorrectionFixture(t *testing.T) fundsCorrectionFixture {
	t.Helper()
	decision := newFundsDecisionFixture(t)
	fundsResolutionHTTP(t, decision.server, decision.cookie("operator"), http.MethodPut, decision.path, decision.body, http.StatusOK)
	path := "/billing-accounts/billing-journal/requests/" + decision.reservation.RequestID + "/funds-credits/recover-credit"
	body := `{"credit":{"numerator":"7","denominator":"1000"},"reason":"approved_correction","evidence_reference":"credit-review"}`
	return fundsCorrectionFixture{decision, path, body}
}

func (fixture fundsCorrectionFixture) resources(t *testing.T) map[string]any {
	t.Helper()
	state := fixture.state(t)
	state["decision"] = fundsResolutionHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, fixture.path, "", http.StatusOK)
	state["tenant-limit"] = fundsResolutionHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/tenant-limits/managed-first", "", http.StatusOK)
	return state
}

func (fixture fundsCorrectionFixture) reject(t *testing.T, status int) {
	t.Helper()
	failure := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.creditPath, fixture.creditBody, status)
	if strings.Contains(fmt.Sprint(failure), "controlled_") {
		t.Fatal("private credit failure exposed")
	}
}

func (fixture fundsCorrectionFixture) assertUnchanged(t *testing.T, before map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(before, fixture.resources(t)) {
		t.Fatal("failed credit changed financial resources or original decision")
	}
	assertFundsCreditRemainder(t, fixture.database, "1", "200")
	fundsResolutionHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, fixture.creditPath, "", http.StatusNotFound)
	var count int64
	if err := fixture.database.database.Model(&managedFundsCorrectionRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial credit receipt count=%d error=%v", count, err)
	}
}

func (fixture fundsCorrectionFixture) recoverCredit(t *testing.T, before map[string]any) {
	t.Helper()
	restarted, cookie := newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, fixture.database))
	result := fundsResolutionHTTP(t, restarted, cookie("operator"), http.MethodPut, fixture.creditPath, fixture.creditBody, http.StatusOK)
	if result["credited_cents"] != "1" || !reflect.DeepEqual(result["credit"], map[string]any{"numerator": "7", "denominator": "1000"}) {
		t.Fatalf("recovered credit=%v", result)
	}
	assertHostedFundsBalance(t, fixture.database, 5, 5)
	assertFundsCreditRemainder(t, fixture.database, "1", "125")
	after := fixture.resources(t)
	if !reflect.DeepEqual(before["decision"], after["decision"]) || !reflect.DeepEqual(before["reservation"], after["reservation"]) {
		t.Fatal("credit rewrote the original settlement")
	}
	beforeEntries := before["entries"].(map[string]any)["entries"].([]any)
	afterEntries := after["entries"].(map[string]any)["entries"].([]any)
	if len(afterEntries) != len(beforeEntries)+1 {
		t.Fatalf("credit ledger entry count before=%d after=%d", len(beforeEntries), len(afterEntries))
	}
	for range 2 {
		replayed := fundsResolutionHTTP(t, restarted, cookie("operator"), http.MethodPut, fixture.creditPath, fixture.creditBody, http.StatusOK)
		read := fundsResolutionHTTP(t, restarted, cookie("owner"), http.MethodGet, fixture.creditPath, "", http.StatusOK)
		if !reflect.DeepEqual(result, replayed) || !reflect.DeepEqual(result, read) || !reflect.DeepEqual(after, fixture.resources(t)) {
			t.Fatal("credit replay changed receipt or financial effects")
		}
	}
}

func TestHostedFundsCorrectionWriteFailuresPreserveCreditState(t *testing.T) {
	for _, scenario := range []struct{ name, statement string }{
		{"account-lock", "BEFORE UPDATE ON managed_billing_account_records"},
		{"ledger-credit", "BEFORE INSERT ON ledger_entries"},
		{"remainder", "BEFORE UPDATE ON managed_funds_account_records"},
		{"tenant-usage", "BEFORE UPDATE ON managed_funds_tenant_records"},
		{"audit-receipt", "BEFORE INSERT ON managed_funds_correction_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsCorrectionFixture(t)
			before := fixture.resources(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_credit_write " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_credit_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			fixture.reject(t, http.StatusInternalServerError)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_credit_write").Error; err != nil {
				t.Fatal(err)
			}
			fixture.assertUnchanged(t, before)
			fixture.recoverCredit(t, before)
		})
	}
}

func TestHostedFundsCorrectionReadFailuresPreserveCreditState(t *testing.T) {
	for _, scenario := range []struct {
		table string
		read  int64
	}{
		{"managed_funds_correction_records", 1},
		{"managed_funds_correction_records", 2},
		{"managed_funds_correction_records", 3},
		{"managed_funds_reservation_records", 1},
		{"managed_funds_settlement_records", 1},
		{"managed_charge_adjustment_records", 1},
		{"managed_funds_account_records", 1},
		{"ledger_accounts", 1},
		{"managed_journal_request_records", 1},
		{"managed_funds_tenant_records", 1},
	} {
		t.Run(scenario.table+"/"+strconv.FormatInt(scenario.read, 10), func(t *testing.T) {
			fixture := newFundsCorrectionFixture(t)
			before := fixture.resources(t)
			var reads, failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:credit_read", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == scenario.table && reads.Add(1) == scenario.read {
					failures.Add(1)
					tx.AddError(errors.New("controlled_credit_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			fixture.reject(t, http.StatusInternalServerError)
			if err := callback.Remove("test:credit_read"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 {
				t.Fatalf("credit read failures=%d", failures.Load())
			}
			fixture.assertUnchanged(t, before)
			fixture.recoverCredit(t, before)
		})
	}
}

func TestHostedFundsCorrectionCorruptAmountsCannotCreateCredit(t *testing.T) {
	for _, scenario := range []struct{ table, column, value string }{
		{"managed_funds_settlement_records", "charge_denominator", "200"},
		{"managed_funds_account_records", "remainder_denominator", "200"},
		{"managed_funds_tenant_records", "spent_denominator", "200"},
	} {
		t.Run(scenario.table, func(t *testing.T) {
			fixture := newFundsCorrectionFixture(t)
			before := fixture.resources(t)
			update := func(value string) {
				t.Helper()
				if err := fixture.database.database.Table(scenario.table).Where("billing_account_id = ?", "billing-journal").UpdateColumn(scenario.column, value).Error; err != nil {
					t.Fatal(err)
				}
			}
			update("0")
			fixture.reject(t, http.StatusInternalServerError)
			update(scenario.value)
			fixture.assertUnchanged(t, before)
			fixture.recoverCredit(t, before)
		})
	}
}

func TestHostedFundsCorrectionRejectsInvalidCommandsWithoutCredit(t *testing.T) {
	for _, scenario := range []struct{ name, old, replacement string }{
		{"zero", `"7"`, `"0"`},
		{"negative", `"7"`, `"-7"`},
		{"denominator", `"1000"`, `"0"`},
		{"non-numeric", `"7"`, `"invalid"`},
		{"reason", `"approved_correction"`, `"invalid reason"`},
		{"empty-evidence", `"credit-review"`, `""`},
		{"padded-evidence", `"credit-review"`, `" credit-review"`},
		{"multiline-evidence", `"credit-review"`, `"credit\nreview"`},
		{"long-evidence", `"credit-review"`, strconv.Quote(strings.Repeat("x", 257))},
		{"unknown-field", `"reason":`, `"unrecognized":true,"reason":`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsCorrectionFixture(t)
			before := fixture.resources(t)
			original := fixture.creditBody
			fixture.creditBody = strings.Replace(original, scenario.old, scenario.replacement, 1)
			fixture.reject(t, http.StatusBadRequest)
			fixture.assertUnchanged(t, before)
			fixture.creditBody = original
			fixture.recoverCredit(t, before)
		})
	}
}
