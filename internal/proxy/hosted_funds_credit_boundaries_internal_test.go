package proxy

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
)

func TestHostedFundsCreditRejectsInvalidRawAmounts(t *testing.T) {
	database, management, charges := newFundsCreditFixture(t)
	charge := charges[0]
	path := "/billing-accounts/billing-journal/charges/" + charge.ID
	before := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK)
	for _, amount := range []ExactMoney{
		{Numerator: "-1", Denominator: "1000"},
		{Numerator: "0", Denominator: "1"},
		{Numerator: "1", Denominator: "0"},
		{Numerator: "invalid", Denominator: "1"},
	} {
		_, err := newCustomerChargeAdjustment(charge.BillingAccountID, charge.ID, "validated-credit", "customer_credit", amount, ratingTestAcceptanceTime())
		if !errors.Is(err, ErrCatalogRatingInvalid) {
			t.Fatalf("invalid raw credit %v accepted: %v", amount, err)
		}
		if current := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK); !reflect.DeepEqual(before, current) {
			t.Fatal("rejected credit changed the retained charge")
		}
		assertHostedFundsBalance(t, database, 4, 4)
		assertFundsCreditRemainder(t, database, "23", "25000")
	}
	command := fundsCreditCommand(t, charge, "validated-credit")
	for range 2 {
		if err := applyFundsFixtureCredit(t, openJournalTransactionInstance(t, database), command); err != nil {
			t.Fatal(err)
		}
		assertHostedFundsBalance(t, database, 5, 5)
		assertFundsCreditRemainder(t, database, "91", "12500")
	}
	current := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK)
	if len(current["customer_adjustments"].([]any)) != 1 || !reflect.DeepEqual(current["rating"], before["rating"]) {
		t.Fatal("corrected credit repeated or changed the original cost")
	}
}

func TestHostedFundsExposureNeverReturnsNegativeResolutionLimit(t *testing.T) {
	database, management, charges := newFundsCreditFixture(t)
	charge := charges[0]
	if err := applyFundsFixtureCredit(t, database, fundsCreditCommand(t, charge, "exposure-credit")); err != nil {
		t.Fatal(err)
	}
	path := "/billing-accounts/billing-journal/reservations/" + charge.RequestID
	before := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK)
	var original managedPriceSnapshotRecord
	if err := database.database.Where("request_id = ?", charge.RequestID).First(&original).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, document, err := restoreHostedPriceSnapshot(original)
	if err != nil {
		t.Fatal(err)
	}
	for index := range document.Bounds {
		document.Bounds[index].Maximum = "0"
	}
	document.Maximum, err = snapshot.MaximumCharge(document.Bounds, document.Maximum.Attempts)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(document.hostedPriceSnapshotDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.database.Model(&managedPriceSnapshotRecord{}).Where("id = ?", original.ID).Updates(map[string]any{"document": encoded, "digest": sha256Hex(string(encoded))}).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		current := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK)
		if !reflect.DeepEqual(current["resolution_charge_limit"], map[string]any{"numerator": "0", "denominator": "1"}) {
			t.Fatalf("stored bound produced a negative resolution limit: %v", current)
		}
		assertHostedFundsBalance(t, database, 5, 5)
		assertFundsCreditRemainder(t, database, "91", "12500")
	}
	if err := database.database.Model(&managedPriceSnapshotRecord{}).Where("id = ?", original.ID).Updates(map[string]any{"document": original.Document, "digest": original.Digest}).Error; err != nil {
		t.Fatal(err)
	}
	if current := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK); !reflect.DeepEqual(current, before) {
		t.Fatal("restored price changed financial evidence")
	}
}
