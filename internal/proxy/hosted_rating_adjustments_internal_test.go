package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedRatingConcurrentCustomerCreditsCannotExceedCharge(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	_, observation := observeRatedFixture(t, database, intent("concurrent-credit-charge"), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}})
	if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })); err != nil {
		t.Fatal(err)
	}
	collection := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
	chargeID := collection["charges"].([]any)[0].(map[string]any)["id"].(string)
	if err := database.database.Exec("CREATE TABLE concurrent_credit_effects (id TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	second := openJournalTransactionInstance(t, database)
	ready := make(chan struct{})
	results := make(chan error, 2)
	for index, instance := range []*gormManagedTenantDatabase{database, second} {
		command, err := newCustomerChargeAdjustment("billing-journal", chargeID, fmt.Sprintf("concurrent-%d", index), "customer_credit", ExactMoney{Numerator: "91", Denominator: "25000"}, observation.CreatedAt.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			<-ready
			results <- instance.applyCustomerChargeAdjustment(t.Context(), command, func(transaction *gorm.DB, adjustment managedChargeAdjustmentRecord) error {
				return transaction.Exec("INSERT INTO concurrent_credit_effects (id) VALUES (?)", adjustment.ID).Error
			})
		}()
	}
	close(ready)
	accepted, rejected := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, errUsageJournalConflict):
			rejected++
		default:
			t.Fatalf("concurrent adjustment: %v", err)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("accepted=%d rejected=%d", accepted, rejected)
	}
	current := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges/"+chargeID, "", http.StatusOK)
	if len(current["customer_adjustments"].([]any)) != 1 || !reflect.DeepEqual(current["net_customer_charge"], map[string]any{"numerator": "0", "denominator": "1"}) {
		t.Fatalf("concurrent over-credit: %v", current)
	}
	var effects int64
	if err := database.database.Table("concurrent_credit_effects").Count(&effects).Error; err != nil || effects != 1 {
		t.Fatalf("credit effects=%d error=%v", effects, err)
	}
}

func TestHostedRatingCustomerCreditsPreserveProviderCost(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	_, observation := observeRatedFixture(t, database, intent("adjustable-charge"), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}})
	if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })); err != nil {
		t.Fatal(err)
	}
	collection := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
	original := collection["charges"].([]any)[0].(map[string]any)
	chargeID := original["id"].(string)
	path := "/billing-accounts/billing-journal/charges/" + chargeID
	command := func(account, key string, numerator string) customerChargeAdjustment {
		t.Helper()
		input, err := newCustomerChargeAdjustment(account, chargeID, key, "customer_credit", ExactMoney{Numerator: numerator, Denominator: "25000"}, observation.CreatedAt.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		return input
	}
	if err := database.database.Exec("CREATE TABLE adjustment_effects (id TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	apply := func(transaction *gorm.DB, adjustment managedChargeAdjustmentRecord) error {
		return transaction.Exec("INSERT INTO adjustment_effects (id) VALUES (?)", adjustment.ID).Error
	}
	first := command("billing-journal", "first-credit", "40")
	for range 2 {
		if err := database.applyCustomerChargeAdjustment(t.Context(), first, apply); err != nil {
			t.Fatal(err)
		}
	}
	partial := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK)
	if !reflect.DeepEqual(partial["net_customer_charge"], map[string]any{"numerator": "51", "denominator": "25000"}) || len(partial["customer_adjustments"].([]any)) != 1 {
		t.Fatalf("partial credit: %v", partial)
	}
	if err := database.applyCustomerChargeAdjustment(t.Context(), command("billing-journal", "first-credit", "41"), apply); !errors.Is(err, errUsageJournalConflict) {
		t.Fatalf("changed event accepted: %v", err)
	}
	if err := database.applyCustomerChargeAdjustment(t.Context(), command("billing-journal", "excess-credit", "52"), apply); !errors.Is(err, errUsageJournalConflict) {
		t.Fatalf("excess credit accepted: %v", err)
	}
	if err := database.applyCustomerChargeAdjustment(t.Context(), command("billing-foreign", "foreign-credit", "1"), apply); !errors.Is(err, errUsageJournalNotFound) {
		t.Fatalf("foreign credit accepted: %v", err)
	}
	last := command("billing-journal", "last-credit", "51")
	failure := errors.New("controlled adjustment settlement failure")
	if err := database.applyCustomerChargeAdjustment(t.Context(), last, func(transaction *gorm.DB, adjustment managedChargeAdjustmentRecord) error {
		if err := apply(transaction, adjustment); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatalf("settlement failure lost: %v", err)
	}
	if current := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK); !reflect.DeepEqual(current, partial) {
		t.Fatalf("failed credit changed charge: %v", current)
	}
	restarted := openJournalTransactionInstance(t, database)
	if err := restarted.applyCustomerChargeAdjustment(t.Context(), last, apply); err != nil {
		t.Fatal(err)
	}
	if err := database.applyCustomerChargeAdjustment(t.Context(), last, apply); err != nil {
		t.Fatal(err)
	}
	final := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK)
	if !reflect.DeepEqual(final["rating"], original["rating"]) || !reflect.DeepEqual(final["customer_charge"], original["customer_charge"]) || !reflect.DeepEqual(final["net_customer_charge"], map[string]any{"numerator": "0", "denominator": "1"}) || len(final["customer_adjustments"].([]any)) != 2 {
		t.Fatalf("credit rewrote provider cost or failed to remove customer charge: %v", final)
	}
	var effects int64
	if err := database.database.Table("adjustment_effects").Count(&effects).Error; err != nil || effects != 2 {
		t.Fatalf("credit effects=%d error=%v", effects, err)
	}
	listed := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
	if !reflect.DeepEqual(listed["charges"].([]any)[0], final) {
		t.Fatalf("charge collection omitted credits: %v", listed)
	}
}
