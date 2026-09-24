package proxy

import (
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

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
