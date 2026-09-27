package proxy

import (
	"net/http"
	"reflect"
	"testing"
)

func TestHostedFinancialSummaryRejectsChargesFromAnotherRequest(t *testing.T) {
	database, management, charges := newFundsCreditFixture(t)
	fixture := fundsStartupFixture{database: database, management: management}
	before := fixture.state(t)
	path := "/billing-accounts/billing-journal/requests/" + charges[0].RequestID + "/charge-summary"
	original := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK)
	writeRequest := func(requestID string) {
		t.Helper()
		if err := database.database.Model(&managedChargeRecord{}).Where("id = ?", charges[1].ID).UpdateColumn("request_id", requestID).Error; err != nil {
			t.Fatal(err)
		}
	}
	writeRequest(charges[0].RequestID)
	for range 2 {
		failure := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusInternalServerError)
		if !reflect.DeepEqual(failure, map[string]any{"error": map[string]any{"code": "usage_journal_unavailable"}}) {
			t.Fatalf("invalid summary exposed partial or private data: %v", failure)
		}
		for _, resource := range []string{"balance", "ledger-entries", "reservations"} {
			current := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/"+resource, "", http.StatusOK)
			if !reflect.DeepEqual(before[resource], current) {
				t.Fatalf("invalid summary changed %s", resource)
			}
		}
		assertFundsCreditRemainder(t, database, "23", "25000")
	}
	writeRequest(charges[1].RequestID)
	for range 2 {
		reopened := openJournalTransactionInstance(t, database)
		server, cookie := newFundsManagementHTTPFixture(t, reopened)
		current := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, path, "", http.StatusOK)
		if !reflect.DeepEqual(original, current) || !reflect.DeepEqual(before, fixture.state(t)) {
			t.Fatal("restored summary changed financial resources")
		}
		server.Close()
	}
}

func TestHostedFinancialReadsRejectCreditsOnUnresolvedCharge(t *testing.T) {
	fixture := newFinancialReadFixture(t)
	var charge managedChargeRecord
	if err := fixture.database.database.First(&charge).Error; err != nil {
		t.Fatal(err)
	}
	command, err := newCustomerChargeAdjustment(charge.BillingAccountID, charge.ID, "credited-charge", "customer_credit", ExactMoney{Numerator: "3", Denominator: "1000"}, ratingTestAcceptanceTime())
	if err != nil {
		t.Fatal(err)
	}
	if err := applyFundsFixtureCredit(t, fixture.database, command); err != nil {
		t.Fatal(err)
	}
	before := fixture.snapshot(t)
	if err := fixture.database.database.Model(&managedChargeRecord{}).Where("id = ?", charge.ID).UpdateColumn("state", chargeLimitUnresolved).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		for _, name := range []string{"charges", "charge", "summary", "reservation"} {
			fixture.read(t, fixture.resources[name], fixture.cookie("owner"), http.StatusInternalServerError)
		}
		for _, name := range []string{"balance", "entries", "tenant", "reservations"} {
			if !reflect.DeepEqual(before[name], fixture.read(t, fixture.resources[name], fixture.cookie("owner"), http.StatusOK)) {
				t.Fatalf("inconsistent charge read changed %s", name)
			}
		}
		assertFundsCreditRemainder(t, fixture.database, "0", "1")
	}
	if err := fixture.database.database.Model(&managedChargeRecord{}).Where("id = ?", charge.ID).UpdateColumn("state", charge.State).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		reopened := openJournalTransactionInstance(t, fixture.database)
		server, cookie := newFundsManagementHTTPFixture(t, reopened)
		restored := fixture
		restored.server, restored.cookie = server, cookie
		if !reflect.DeepEqual(before, restored.snapshot(t)) || fixture.calls.Load() != 1 {
			t.Fatalf("restored charge changed financial resources or provider calls: calls=%d", fixture.calls.Load())
		}
		server.Close()
	}
}
