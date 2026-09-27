package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"gorm.io/gorm"
)

func newFundsCreditFixture(t *testing.T) (*gormManagedTenantDatabase, *httptest.Server, []managedChargeRecord) {
	t.Helper()
	database, _, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	for index := range 3 {
		hostedIdentityHTTP(t, server, fmt.Sprintf("credit-%d", index), "funded prompt", http.StatusOK)
		if err := deliverFundsFixture(t, database); err != nil {
			t.Fatal(err)
		}
	}
	assertHostedFundsBalance(t, database, 4, 4)
	var charges []managedChargeRecord
	if err := database.database.Order("id").Find(&charges).Error; err != nil || len(charges) != 3 {
		t.Fatalf("charges=%v error=%v", charges, err)
	}
	return database, management, charges
}

func fundsCreditCommand(t *testing.T, charge managedChargeRecord, key string) customerChargeAdjustment {
	t.Helper()
	command, err := newCustomerChargeAdjustment(charge.BillingAccountID, charge.ID, key, "customer_credit", ExactMoney{Numerator: "91", Denominator: "25000"}, ratingTestAcceptanceTime())
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func applyFundsFixtureCredit(t *testing.T, database *gormManagedTenantDatabase, command customerChargeAdjustment) error {
	t.Helper()
	return database.applyCustomerChargeAdjustment(t.Context(), command, settleHostedChargeAdjustment)
}

func assertFundsCreditRemainder(t *testing.T, database *gormManagedTenantDatabase, numerator, denominator string) {
	t.Helper()
	var financial managedFundsAccountRecord
	if err := database.database.Where("billing_account_id = ?", "billing-journal").First(&financial).Error; err != nil {
		t.Fatal(err)
	}
	if financial.RemainderNumerator != numerator || financial.RemainderDenominator != denominator {
		t.Fatalf("credit remainder=%s/%s want=%s/%s", financial.RemainderNumerator, financial.RemainderDenominator, numerator, denominator)
	}
}

func TestHostedFundsCreditsRestoreFractionalChargesWithoutChangingCosts(t *testing.T) {
	database, management, charges := newFundsCreditFixture(t)
	wantRemainders := []ExactMoney{{Numerator: "91", Denominator: "12500"}, {Numerator: "91", Denominator: "25000"}, {Numerator: "0", Denominator: "1"}}
	for index, charge := range charges {
		path := "/billing-accounts/billing-journal/charges/" + charge.ID
		original := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK)
		command := fundsCreditCommand(t, charge, fmt.Sprintf("full-credit-%d", index))
		for range 2 {
			if err := applyFundsFixtureCredit(t, openJournalTransactionInstance(t, database), command); err != nil {
				t.Fatal(err)
			}
		}
		assertHostedFundsBalance(t, database, 5, 5)
		assertFundsCreditRemainder(t, database, wantRemainders[index].Numerator, wantRemainders[index].Denominator)
		updated := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK)
		if !reflect.DeepEqual(updated["rating"], original["rating"]) || !reflect.DeepEqual(updated["customer_charge"], original["customer_charge"]) || !reflect.DeepEqual(updated["net_customer_charge"], map[string]any{"numerator": "0", "denominator": "1"}) || len(updated["customer_adjustments"].([]any)) != 1 {
			t.Fatalf("credit changed original costs or repeated: %v", updated)
		}
	}
	account, err := newHostedLedgerAccount(database.database, "billing-journal", ratingTestAcceptanceTime())
	if err != nil {
		t.Fatal(err)
	}
	entries, err := account.service.ListEntries(t.Context(), account.tenant, account.user, account.namespace, ratingTestAcceptanceTime().Unix()+1, 100, ledger.ListEntriesFilter{})
	if err != nil || len(entries) != 9 {
		t.Fatalf("credit ledger effects=%v error=%v", entries, err)
	}
	var total int64
	for _, entry := range entries {
		total += entry.AmountCents().Int64()
	}
	if total != 5 {
		t.Fatalf("credits did not conserve the funded balance: %d", total)
	}
}

func TestHostedFundsCreditFailureRollsBackLedgerAndAdjustment(t *testing.T) {
	database, management, charges := newFundsCreditFixture(t)
	path := "/billing-accounts/billing-journal/charges/" + charges[0].ID
	original := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK)
	if err := database.database.Exec("CREATE TRIGGER reject_credit_remainder BEFORE UPDATE ON managed_funds_account_records BEGIN SELECT RAISE(ABORT, 'controlled_credit_remainder_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	command := fundsCreditCommand(t, charges[0], "credit-write-failure")
	if err := applyFundsFixtureCredit(t, database, command); err == nil {
		t.Fatal("financial credit write failure was ignored")
	}
	assertHostedFundsBalance(t, database, 4, 4)
	assertFundsCreditRemainder(t, database, "23", "25000")
	if current := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK); !reflect.DeepEqual(current, original) {
		t.Fatalf("failed credit changed retained charge: %v", current)
	}
	if err := database.database.Exec("DROP TRIGGER reject_credit_remainder").Error; err != nil {
		t.Fatal(err)
	}
	if err := applyFundsFixtureCredit(t, openJournalTransactionInstance(t, database), command); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 5, 5)
	assertFundsCreditRemainder(t, database, "91", "12500")
}

func TestHostedFundsConcurrentCreditsApplyOnce(t *testing.T) {
	database, management, charges := newFundsCreditFixture(t)
	command := fundsCreditCommand(t, charges[0], "concurrent-credit")
	start := make(chan struct{})
	var group sync.WaitGroup
	for range 3 {
		worker := openJournalTransactionInstance(t, database)
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			if err := applyFundsFixtureCredit(t, worker, command); err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	group.Wait()
	assertHostedFundsBalance(t, database, 5, 5)
	assertFundsCreditRemainder(t, database, "91", "12500")
	current := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges/"+charges[0].ID, "", http.StatusOK)
	if len(current["customer_adjustments"].([]any)) != 1 {
		t.Fatalf("repeated credit=%v", current)
	}
}

func TestHostedFundsCreditBeforeSettlementDoesNotCreateFunds(t *testing.T) {
	database, _, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	hostedIdentityHTTP(t, server, "credit-before-settlement", "funded prompt", http.StatusOK)
	observations, err := database.pendingJournalDeliveries(t.Context(), 100)
	if err != nil || len(observations) != 1 {
		t.Fatalf("observations=%v error=%v", observations, err)
	}
	if err := database.deliverJournalObservation(t.Context(), observations[0].ID, observations[0].ObservedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })); err != nil {
		t.Fatal(err)
	}
	var charge managedChargeRecord
	if err := database.database.First(&charge).Error; err != nil {
		t.Fatal(err)
	}
	command := fundsCreditCommand(t, charge, "unsettled-credit")
	if err := applyFundsFixtureCredit(t, database, command); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 5, 2)
	if err := database.reconcileHostedFunds(t.Context(), ratingTestAcceptanceTime()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 5, 5)
	assertFundsCreditRemainder(t, database, "0", "1")
	current := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+charge.RequestID+"/charge-summary", "", http.StatusOK)
	if !reflect.DeepEqual(current["net_customer_charge"], map[string]any{"numerator": "0", "denominator": "1"}) {
		t.Fatalf("settlement ignored credit: %v", current)
	}
	var receipt managedFundsSettlementRecord
	if err := database.database.Where("request_id = ?", charge.RequestID).First(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	var creditIDs []string
	if err := json.Unmarshal(receipt.AdjustmentIDs, &creditIDs); err != nil || !reflect.DeepEqual(creditIDs, []string{command.record.ID}) {
		t.Fatalf("settlement lost applied credit: %s error=%v", receipt.AdjustmentIDs, err)
	}
	var financialCredits int64
	if err := database.database.Model(&managedFundsCreditRecord{}).Count(&financialCredits).Error; err != nil || financialCredits != 0 {
		t.Fatalf("unsettled credit created a ledger credit: count=%d error=%v", financialCredits, err)
	}
}

func TestHostedFundsDistinctConcurrentCreditsConserveRemainder(t *testing.T) {
	database, _, charges := newFundsCreditFixture(t)
	start := make(chan struct{})
	var group sync.WaitGroup
	for index, charge := range charges {
		worker := openJournalTransactionInstance(t, database)
		command := fundsCreditCommand(t, charge, fmt.Sprintf("distinct-credit-%d", index))
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			if err := applyFundsFixtureCredit(t, worker, command); err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	group.Wait()
	assertHostedFundsBalance(t, database, 5, 5)
	assertFundsCreditRemainder(t, database, "0", "1")
	var receipts []managedFundsCreditRecord
	if err := database.database.Find(&receipts).Error; err != nil || len(receipts) != 3 {
		t.Fatalf("credit receipts=%v error=%v", receipts, err)
	}
	var cents int64
	for _, receipt := range receipts {
		cents += receipt.Effect.CreditedCents
	}
	if cents != 1 {
		t.Fatalf("concurrent credits posted %d cents", cents)
	}
}
