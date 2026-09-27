package proxy

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedFinancialCreditReadsRejectInvalidQueriesWithoutEffects(t *testing.T) {
	fixture := newFundsCorrectionFixture(t)
	original := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.creditPath, fixture.creditBody, http.StatusOK)
	before := fixture.resources(t)
	for _, owner := range []string{"owner", "operator"} {
		for _, path := range []string{
			fixture.creditPath + "?unexpected=1",
			fixture.creditPath[:strings.LastIndex(fixture.creditPath, "/")+1] + "%20",
		} {
			fundsResolutionHTTP(t, fixture.server, fixture.cookie(owner), http.MethodGet, path, "", http.StatusBadRequest)
			read := fundsResolutionHTTP(t, fixture.server, fixture.cookie(owner), http.MethodGet, fixture.creditPath, "", http.StatusOK)
			if !reflect.DeepEqual(original, read) || !reflect.DeepEqual(before, fixture.resources(t)) {
				t.Fatal("rejected credit query changed receipts or funds")
			}
			assertFundsCreditRemainder(t, fixture.database, "1", "125")
		}
	}
}

func TestHostedFinancialCreditReadsRejectCorruptReceipts(t *testing.T) {
	fixture := newFundsCorrectionFixture(t)
	original := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.creditPath, fixture.creditBody, http.StatusOK)
	before := fixture.resources(t)
	var record managedFundsCorrectionRecord
	if err := fixture.database.database.First(&record).Error; err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name, column    string
		value, original any
	}{
		{"invalid-numerator", "credit_numerator", "invalid", record.CreditNumerator},
		{"negative-credit", "credit_numerator", "-7", record.CreditNumerator},
		{"zero-credit", "credit_numerator", "0", record.CreditNumerator},
		{"invalid-denominator", "credit_denominator", "invalid", record.CreditDenominator},
		{"zero-denominator", "credit_denominator", "0", record.CreditDenominator},
		{"negative-denominator", "credit_denominator", "-1000", record.CreditDenominator},
		{"invalid-reason", "reason", "invalid reason", record.Reason},
		{"missing-timestamp", "created_at", time.Time{}, record.CreatedAt},
		{"changed-credit", "credit_numerator", "1", record.CreditNumerator},
		{"changed-cents", "credited_cents", int64(2), record.Effect.CreditedCents},
		{"invalid-before", "remainder_before_denominator", "0", record.Effect.RemainderBeforeDenominator},
		{"excess-before", "remainder_before_numerator", "200", record.Effect.RemainderBeforeNumerator},
		{"changed-before", "remainder_before_numerator", "0", record.Effect.RemainderBeforeNumerator},
		{"invalid-after", "remainder_after_denominator", "0", record.Effect.RemainderAfterDenominator},
		{"excess-after", "remainder_after_numerator", "125", record.Effect.RemainderAfterNumerator},
		{"changed-after", "remainder_after_numerator", "0", record.Effect.RemainderAfterNumerator},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			write := func(value any) {
				t.Helper()
				result := fixture.database.database.Model(&managedFundsCorrectionRecord{}).Where("id = ?", record.ID).UpdateColumn(scenario.column, value)
				if result.Error != nil || result.RowsAffected != 1 {
					t.Fatalf("credit mutation rows=%d error=%v", result.RowsAffected, result.Error)
				}
			}
			write(scenario.value)
			t.Cleanup(func() { write(scenario.original) })
			for _, owner := range []string{"owner", "operator"} {
				failure := fundsResolutionHTTP(t, fixture.server, fixture.cookie(owner), http.MethodGet, fixture.creditPath, "", http.StatusInternalServerError)
				if !reflect.DeepEqual(failure, map[string]any{"error": map[string]any{"code": "billing_account_store_failed"}}) {
					t.Fatalf("credit read returned private or partial data: %v", failure)
				}
			}
			if !reflect.DeepEqual(before, fixture.resources(t)) {
				t.Fatal("corrupt credit read changed financial resources")
			}
			assertFundsCreditRemainder(t, fixture.database, "1", "125")
		})
		read := fundsResolutionHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, fixture.creditPath, "", http.StatusOK)
		replayed := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.creditPath, fixture.creditBody, http.StatusOK)
		if !reflect.DeepEqual(original, read) || !reflect.DeepEqual(original, replayed) || !reflect.DeepEqual(before, fixture.resources(t)) {
			t.Fatal("restored credit changed its receipt or financial effects")
		}
	}
}

func TestHostedFinancialCreditReadsRecoverStorageWithoutNewEffects(t *testing.T) {
	fixture := newFundsCorrectionFixture(t)
	original := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.creditPath, fixture.creditBody, http.StatusOK)
	before := fixture.resources(t)
	for _, scenario := range []struct{ owner, table string }{
		{"owner", "managed_billing_account_records"},
		{"owner", "managed_funds_correction_records"},
		{"operator", "managed_funds_correction_records"},
	} {
		t.Run(scenario.owner+"/"+scenario.table, func(t *testing.T) {
			var failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:credit_read", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == scenario.table {
					failures.Add(1)
					tx.AddError(errors.New("controlled_credit_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := callback.Remove("test:credit_read"); err != nil {
					t.Error(err)
				}
			})
			failure := fundsResolutionHTTP(t, fixture.server, fixture.cookie(scenario.owner), http.MethodGet, fixture.creditPath, "", http.StatusInternalServerError)
			if failures.Load() != 1 || !reflect.DeepEqual(failure, map[string]any{"error": map[string]any{"code": "billing_account_store_failed"}}) {
				t.Fatalf("credit read failures=%d response=%v", failures.Load(), failure)
			}
		})
		read := fundsResolutionHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, fixture.creditPath, "", http.StatusOK)
		if !reflect.DeepEqual(original, read) || !reflect.DeepEqual(before, fixture.resources(t)) {
			t.Fatal("failed credit read changed receipt or financial effects")
		}
		assertFundsCreditRemainder(t, fixture.database, "1", "125")
	}
}

func TestHostedFinancialCreditReadsRejectCorruptChargeAdjustments(t *testing.T) {
	fixture := newFinancialReadFixture(t)
	var charge managedChargeRecord
	if err := fixture.database.database.First(&charge).Error; err != nil {
		t.Fatal(err)
	}
	command, err := newCustomerChargeAdjustment(charge.BillingAccountID, charge.ID, "credit-read", "customer_credit", ExactMoney{Numerator: "3", Denominator: "1000"}, ratingTestAcceptanceTime())
	if err != nil {
		t.Fatal(err)
	}
	if err := applyFundsFixtureCredit(t, fixture.database, command); err != nil {
		t.Fatal(err)
	}
	before := fixture.snapshot(t)
	record := command.record
	for _, scenario := range []struct {
		name, column    string
		value, original any
	}{
		{"invalid-numerator", "credit_numerator", "invalid", record.CreditNumerator},
		{"negative-credit", "credit_numerator", "-3", record.CreditNumerator},
		{"zero-credit", "credit_numerator", "0", record.CreditNumerator},
		{"excess-credit", "credit_numerator", "1000", record.CreditNumerator},
		{"zero-denominator", "credit_denominator", "0", record.CreditDenominator},
		{"invalid-reason", "reason", "invalid reason", record.Reason},
		{"missing-timestamp", "created_at", time.Time{}, record.CreatedAt},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			write := func(value any) {
				t.Helper()
				result := fixture.database.database.Model(&managedChargeAdjustmentRecord{}).Where("id = ?", record.ID).UpdateColumn(scenario.column, value)
				if result.Error != nil || result.RowsAffected != 1 {
					t.Fatalf("adjustment mutation rows=%d error=%v", result.RowsAffected, result.Error)
				}
			}
			write(scenario.value)
			t.Cleanup(func() { write(scenario.original) })
			for _, name := range []string{"charges", "charge", "summary", "reservation"} {
				fixture.read(t, fixture.resources[name], fixture.cookie("owner"), http.StatusInternalServerError)
			}
			for _, name := range []string{"balance", "entries", "tenant"} {
				if !reflect.DeepEqual(before[name], fixture.read(t, fixture.resources[name], fixture.cookie("owner"), http.StatusOK)) {
					t.Fatalf("corrupt adjustment changed %s", name)
				}
			}
		})
		if !reflect.DeepEqual(before, fixture.snapshot(t)) {
			t.Fatal("restored adjustment changed financial resources")
		}
		assertFundsCreditRemainder(t, fixture.database, "0", "1")
		if fixture.calls.Load() != 1 {
			t.Fatalf("credit read repeated provider work: calls=%d", fixture.calls.Load())
		}
	}
}

func TestHostedFinancialCreditEffectsRejectLaterCreditAndReplay(t *testing.T) {
	for _, scenario := range []struct {
		name, column string
		value        any
	}{
		{"amount", "credit_numerator", "1"},
		{"cents", "credited_cents", int64(2)},
		{"before", "remainder_before_numerator", "0"},
		{"after", "remainder_after_numerator", "0"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsCorrectionFixture(t)
			original := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.creditPath, fixture.creditBody, http.StatusOK)
			before := fixture.resources(t)
			var record managedFundsCorrectionRecord
			if err := fixture.database.database.First(&record).Error; err != nil {
				t.Fatal(err)
			}
			if err := fixture.database.database.Model(&managedFundsCorrectionRecord{}).Where("id = ?", record.ID).UpdateColumn(scenario.column, scenario.value).Error; err != nil {
				t.Fatal(err)
			}
			nextPath := fixture.creditPath + "-next"
			nextBody := `{"credit":{"numerator":"1","denominator":"1000"},"reason":"approved_correction","evidence_reference":"next-review"}`
			fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, nextPath, nextBody, http.StatusInternalServerError)
			// A changed amount conflicts with the original idempotent request.
			replayStatus := http.StatusInternalServerError
			if scenario.name == "amount" {
				replayStatus = http.StatusConflict
			}
			fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.creditPath, fixture.creditBody, replayStatus)
			if !reflect.DeepEqual(before, fixture.resources(t)) {
				t.Fatal("inconsistent retained credit changed financial resources")
			}
			fundsResolutionHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, nextPath, "", http.StatusNotFound)
			if err := fixture.database.database.Model(&record).Select("credit_numerator", "credited_cents", "remainder_before_numerator", "remainder_after_numerator").Updates(record).Error; err != nil {
				t.Fatal(err)
			}
			restarted, cookie := newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, fixture.database))
			read := fundsResolutionHTTP(t, restarted, cookie("owner"), http.MethodGet, fixture.creditPath, "", http.StatusOK)
			if !reflect.DeepEqual(original, read) {
				t.Fatal("restored evidence changed the original credit")
			}
			created := fundsResolutionHTTP(t, restarted, cookie("operator"), http.MethodPut, nextPath, nextBody, http.StatusOK)
			if created["credited_cents"] != "0" {
				t.Fatalf("fractional credit created an unexpected Ledger effect: %v", created)
			}
			after := fixture.resources(t)
			for range 2 {
				replayed := fundsResolutionHTTP(t, restarted, cookie("operator"), http.MethodPut, nextPath, nextBody, http.StatusOK)
				if !reflect.DeepEqual(created, replayed) || !reflect.DeepEqual(after, fixture.resources(t)) {
					t.Fatal("restored credit replay changed financial effects")
				}
			}
			assertHostedFundsBalance(t, fixture.database, 5, 5)
			assertFundsCreditRemainder(t, fixture.database, "7", "1000")
		})
	}
}
