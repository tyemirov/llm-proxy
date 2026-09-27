package proxy

import (
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestHostedFundsCorrectionRejectsInvalidPriorUsageCredit(t *testing.T) {
	for _, scenario := range []struct {
		name, column string
		value        any
	}{
		{"invalid-numerator", "credit_numerator", "invalid"},
		{"negative-credit", "credit_numerator", "-3"},
		{"zero-credit", "credit_numerator", "0"},
		{"zero-denominator", "credit_denominator", "0"},
		{"invalid-reason", "reason", "invalid reason"},
		{"missing-timestamp", "created_at", time.Time{}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFinancialReadFixture(t)
			var charge managedChargeRecord
			if err := fixture.database.database.First(&charge).Error; err != nil {
				t.Fatal(err)
			}
			command, err := newCustomerChargeAdjustment(charge.BillingAccountID, charge.ID, "prior-usage-credit", "customer_credit", ExactMoney{Numerator: "3", Denominator: "1000"}, ratingTestAcceptanceTime())
			if err != nil {
				t.Fatal(err)
			}
			if err := applyFundsFixtureCredit(t, fixture.database, command); err != nil {
				t.Fatal(err)
			}
			before := fixture.snapshot(t)
			if err := fixture.database.database.Model(&managedChargeAdjustmentRecord{}).Where("id = ?", command.record.ID).UpdateColumn(scenario.column, scenario.value).Error; err != nil {
				t.Fatal(err)
			}
			path := "/billing-accounts/billing-journal/requests/" + charge.RequestID + "/funds-credits/after-prior-credit"
			body := `{"credit":{"numerator":"1","denominator":"1000"},"reason":"approved_correction","evidence_reference":"prior-credit-review"}`
			for range 2 {
				failure := fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, path, body, http.StatusInternalServerError)
				if !reflect.DeepEqual(failure, map[string]any{"error": map[string]any{"code": "billing_account_store_failed"}}) {
					t.Fatalf("invalid prior credit exposed partial or private data: %v", failure)
				}
				fundsResolutionHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, path, "", http.StatusNotFound)
				for _, name := range []string{"balance", "entries", "tenant"} {
					if !reflect.DeepEqual(before[name], fixture.read(t, fixture.resources[name], fixture.cookie("owner"), http.StatusOK)) {
						t.Fatalf("invalid prior credit changed %s", name)
					}
				}
				assertFundsCreditRemainder(t, fixture.database, "0", "1")
			}
			if err := fixture.database.database.Model(&managedChargeAdjustmentRecord{}).Where("id = ?", command.record.ID).
				Select("credit_numerator", "credit_denominator", "reason", "created_at").Updates(command.record).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, fixture.snapshot(t)) {
				t.Fatal("rejected correction changed retained financial resources")
			}
			var receipt, recovered map[string]any
			for iteration := range 2 {
				reopened := openJournalTransactionInstance(t, fixture.database)
				server, cookie := newFundsManagementHTTPFixture(t, reopened)
				current := fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusOK)
				if current["credited_cents"] != "1" || fixture.calls.Load() != 1 {
					t.Fatalf("restored credit produced incorrect effects: %v calls=%d", current, fixture.calls.Load())
				}
				assertHostedFundsBalance(t, reopened, 5, 5)
				assertFundsCreditRemainder(t, reopened, "9", "1000")
				state := fixture.snapshot(t)
				if iteration > 0 && (!reflect.DeepEqual(receipt, current) || !reflect.DeepEqual(recovered, state)) {
					t.Fatal("replayed correction changed its receipt or financial effects")
				}
				receipt, recovered = current, state
				server.Close()
			}
		})
	}
}
