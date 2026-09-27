package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedFundsStartupAttemptAboveLimitRetainsUnresolvedHold(t *testing.T) {
	fixture := newFundsStartupFixture(t, startupCompleteUsage)
	before := fixture.state(t)
	var attempt managedJournalAttemptRecord
	if err := fixture.database.database.First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.database.Model(&attempt).UpdateColumn("number", 3).Error; err != nil {
		t.Fatal(err)
	}
	var retained map[string]any
	for iteration := range 2 {
		fixture.restart(t)
		current := fixture.state(t)
		if !reflect.DeepEqual(before["ledger-entries"], current["ledger-entries"]) {
			t.Fatal("attempt above its limit changed Ledger entries")
		}
		balance := current["balance"].(map[string]any)
		if balance["pending_cents"] != "3" || balance["reserved_cents"] != "3" {
			t.Fatalf("attempt above its limit lost the reconciliation hold: %v", balance)
		}
		charges := current["charges"].(map[string]any)["charges"].([]any)
		if len(charges) != 1 || charges[0].(map[string]any)["state"] != chargeLimitUnresolved || fixture.calls.Load() != 1 {
			t.Fatalf("attempt limit was not retained: charges=%v calls=%d", charges, fixture.calls.Load())
		}
		if iteration > 0 && !reflect.DeepEqual(retained, current) {
			t.Fatal("attempt limit recovery repeated financial effects")
		}
		retained = current
		assertHostedFundsBalance(t, fixture.database, 5, 2)
		assertFundsCreditRemainder(t, fixture.database, "0", "1")
	}
	var reservation managedFundsReservationRecord
	if err := fixture.database.database.Where("request_id = ?", attempt.RequestID).First(&reservation).Error; err != nil || reservation.State != fundsReservationReconciliation {
		t.Fatalf("attempt limit reservation=%+v error=%v", reservation, err)
	}
	server, cookie := newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, fixture.database))
	path := "/billing-accounts/billing-journal/requests/" + attempt.RequestID + "/funds-resolution"
	body := fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":"13","denominator":"1000"},"reason":"approved_attempt_review","evidence_reference":"retained-attempt-review"}`, reservation.Revision)
	var resolved map[string]any
	for iteration := range 2 {
		fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusOK)
		fixture.restart(t)
		assertHostedFundsBalance(t, fixture.database, 4, 4)
		assertFundsCreditRemainder(t, fixture.database, "3", "1000")
		current := fixture.state(t)
		if fixture.calls.Load() != 1 || (iteration > 0 && !reflect.DeepEqual(resolved, current)) {
			t.Fatal("attempt resolution repeated provider work or settlement")
		}
		resolved = current
	}
}

func TestHostedFundsStartupReducedReservationRetainsFundsForResolution(t *testing.T) {
	fixture := newFundsStartupFixture(t, startupCompleteUsage)
	before := fixture.state(t)
	var reservation managedFundsReservationRecord
	if err := fixture.database.database.First(&reservation).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.database.Model(&managedFundsReservationRecord{}).Where("request_id = ?", reservation.RequestID).UpdateColumn("maximum_cents", 0).Error; err != nil {
		t.Fatal(err)
	}
	var retained map[string]any
	for iteration := range 2 {
		fixture.restart(t)
		current := fixture.state(t)
		for _, resource := range []string{"balance", "ledger-entries"} {
			if !reflect.DeepEqual(before[resource], current[resource]) {
				t.Fatalf("reduced reservation changed %s", resource)
			}
		}
		assertHostedFundsBalance(t, fixture.database, 5, 2)
		assertFundsCreditRemainder(t, fixture.database, "0", "1")
		var count int64
		if err := fixture.database.database.Model(&managedFundsSettlementRecord{}).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("reduced reservation created settlement: count=%d error=%v", count, err)
		}
		if iteration == 0 {
			retained = current
		} else if !reflect.DeepEqual(retained, current) {
			t.Fatal("repeated recovery changed the reconciliation hold")
		}
	}
	var held managedFundsReservationRecord
	if err := fixture.database.database.First(&held, "request_id = ?", reservation.RequestID).Error; err != nil {
		t.Fatal(err)
	}
	var cases []managedJournalCaseRecord
	if err := fixture.database.database.Where("request_id = ?", reservation.RequestID).Find(&cases).Error; err != nil || len(cases) != 1 || cases[0].Reason != chargeLimitUnresolved || held.State != fundsReservationReconciliation {
		t.Fatalf("reservation=%+v cases=%+v error=%v", held, cases, err)
	}
	if err := fixture.database.database.Model(&held).UpdateColumn("maximum_cents", reservation.MaximumCents).Error; err != nil {
		t.Fatal(err)
	}
	server, cookie := newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, fixture.database))
	path := "/billing-accounts/billing-journal/requests/" + reservation.RequestID + "/funds-resolution"
	body := fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":"13","denominator":"1000"},"reason":"approved_corrected_ceiling","evidence_reference":"reservation-review"}`, held.Revision)
	var resolved map[string]any
	for iteration := range 2 {
		response := fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusOK)
		if response["settled_cents"] != "1" {
			t.Fatalf("resolution=%v", response)
		}
		assertHostedFundsBalance(t, fixture.database, 4, 4)
		assertFundsCreditRemainder(t, fixture.database, "3", "1000")
		fixture.restart(t)
		current := fixture.state(t)
		if iteration == 0 {
			resolved = current
		} else if !reflect.DeepEqual(resolved, current) {
			t.Fatal("resolution replay repeated financial effects")
		}
	}
}

func TestHostedFundsStartupReleasedReservationPreventsSettlement(t *testing.T) {
	fixture := newFundsStartupFixture(t, startupCompleteUsage)
	before := fixture.state(t)
	var reservation managedFundsReservationRecord
	if err := fixture.database.database.First(&reservation).Error; err != nil {
		t.Fatal(err)
	}
	writeState := func(state string) {
		t.Helper()
		if err := fixture.database.database.Model(&managedFundsReservationRecord{}).Where("request_id = ?", reservation.RequestID).UpdateColumn("state", state).Error; err != nil {
			t.Fatal(err)
		}
	}
	writeState(fundsReservationReleased)
	invalid := fixture.state(t)
	for range 2 {
		fixture.failStartup(t)
		fixture.assertPending(t, invalid)
		assertHostedFundsBalance(t, fixture.database, 5, 2)
	}
	writeState(reservation.State)
	if !reflect.DeepEqual(before, fixture.state(t)) {
		t.Fatal("rejected settlement changed restored financial resources")
	}
	fixture.assertSettledOnce(t, before)
}

func TestHostedFundsStartupRejectsInvalidReservationIdentityRead(t *testing.T) {
	fixture := newFundsStartupFixture(t, startupCompleteUsage)
	before := fixture.state(t)
	var reads atomic.Int64
	queries := fixture.database.database.Callback().Query()
	const callback = "test:invalid_reservation_identity"
	if err := queries.After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.DryRun || tx.Error != nil || tx.RowsAffected != 1 {
			return
		}
		if reservation, ok := tx.Statement.Dest.(*managedFundsReservationRecord); ok {
			reservation.RequestID = ""
			reads.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		fixture.failStartup(t)
	}
	if err := queries.Remove(callback); err != nil {
		t.Fatal(err)
	}
	if reads.Load() < 2 {
		t.Fatal("startup did not read the invalid reservation identity")
	}
	fixture.assertPending(t, before)
	fixture.assertSettledOnce(t, before)
}

func TestHostedFundsStartupRetainedSettlementFailuresPreserveFunds(t *testing.T) {
	for _, scenario := range []struct {
		name, table, column string
		value               any
	}{
		{"charge-digest", "managed_charge_records", "rating_digest", "invalid"},
		{"charge-json", "managed_charge_records", "rating", []byte("{")},
		{"credit-numerator", "managed_charge_adjustment_records", "credit_numerator", "invalid"},
		{"credit-negative", "managed_charge_adjustment_records", "credit_numerator", "-3"},
		{"credit-zero", "managed_charge_adjustment_records", "credit_numerator", "0"},
		{"credit-exceeds-charge", "managed_charge_adjustment_records", "credit_numerator", "1000"},
		{"credit-denominator", "managed_charge_adjustment_records", "credit_denominator", "0"},
		{"credit-reason", "managed_charge_adjustment_records", "reason", "invalid reason"},
		{"credit-timestamp", "managed_charge_adjustment_records", "created_at", time.Time{}},
		{"credit-read", "managed_charge_adjustment_records", "", nil},
		{"ledger-account-read", "ledger_accounts", "", nil},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsStartupFixture(t, startupCompleteUsage)
			observations, err := fixture.database.pendingJournalDeliveries(t.Context(), 100)
			if err != nil || len(observations) != 1 {
				t.Fatalf("pending observations=%v error=%v", observations, err)
			}
			// Retain rating before the financial worker performs settlement.
			if err := fixture.database.deliverJournalObservation(t.Context(), observations[0].ID, observations[0].ObservedAt,
				newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })); err != nil {
				t.Fatal(err)
			}
			var charge managedChargeRecord
			if err := fixture.database.database.First(&charge).Error; err != nil {
				t.Fatal(err)
			}
			command, err := newCustomerChargeAdjustment(charge.BillingAccountID, charge.ID, "retained-credit", "customer_credit", ExactMoney{Numerator: "3", Denominator: "1000"}, ratingTestAcceptanceTime())
			if err != nil {
				t.Fatal(err)
			}
			if err := applyFundsFixtureCredit(t, fixture.database, command); err != nil {
				t.Fatal(err)
			}
			before := fixture.state(t)
			var armed atomic.Bool
			var failures atomic.Int64
			if scenario.column == "" {
				callback := fixture.database.database.Callback().Query()
				if err := callback.Before("gorm:query").Register("test:retained_settlement_read", func(tx *gorm.DB) {
					if armed.Load() && !tx.DryRun && tx.Statement.Table == scenario.table {
						failures.Add(1)
						tx.AddError(errors.New("controlled_retained_settlement_read_failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := callback.Remove("test:retained_settlement_read"); err != nil {
						t.Error(err)
					}
				})
			} else {
				id := command.record.ID
				if scenario.table == "managed_charge_records" {
					id = charge.ID
				}
				values := map[string]any{scenario.column: scenario.value}
				if scenario.column == "rating" {
					values["rating_digest"] = sha256Hex(string(scenario.value.([]byte)))
				}
				if err := fixture.database.database.Table(scenario.table).Where("id = ?", id).Updates(values).Error; err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				armed.Store(true)
				fixture.failStartup(t)
				armed.Store(false)
				for _, resource := range []string{"balance", "ledger-entries"} {
					current := ratingHTTPExchange(t, fixture.management, http.MethodGet, "/billing-accounts/billing-journal/"+resource, "", http.StatusOK)
					if !reflect.DeepEqual(before[resource], current) {
						t.Fatalf("failed retained settlement changed %s", resource)
					}
				}
				assertHostedFundsBalance(t, fixture.database, 5, 2)
				assertFundsCreditRemainder(t, fixture.database, "0", "1")
				var count int64
				if err := fixture.database.database.Model(&managedFundsSettlementRecord{}).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("failed startup retained settlement: count=%d error=%v", count, err)
				}
			}
			if scenario.column == "" && failures.Load() < 2 {
				t.Fatalf("expected read failures were not exercised: %d", failures.Load())
			}
			if err := fixture.database.database.Model(&managedChargeRecord{}).Where("id = ?", charge.ID).
				Select("rating", "rating_digest").Updates(charge).Error; err != nil {
				t.Fatal(err)
			}
			if err := fixture.database.database.Model(&managedChargeAdjustmentRecord{}).Where("id = ?", command.record.ID).
				Select("credit_numerator", "credit_denominator", "reason", "created_at").Updates(command.record).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatal("failed startup changed restored financial resources")
			}
			fixture.restart(t)
			assertHostedFundsBalance(t, fixture.database, 4, 4)
			assertFundsCreditRemainder(t, fixture.database, "0", "1")
			var settlement managedFundsSettlementRecord
			if err := fixture.database.database.First(&settlement).Error; err != nil {
				t.Fatal(err)
			}
			var credits []string
			if err := json.Unmarshal(settlement.AdjustmentIDs, &credits); err != nil || !reflect.DeepEqual(credits, []string{command.record.ID}) {
				t.Fatalf("settlement lost prior credit: credits=%v error=%v", credits, err)
			}
			after := fixture.state(t)
			previousEntries := before["ledger-entries"].(map[string]any)["entries"].([]any)
			entries := after["ledger-entries"].(map[string]any)["entries"].([]any)
			if len(entries) != len(previousEntries)+2 {
				t.Fatalf("expected one release and one debit: before=%v after=%v", previousEntries, entries)
			}
			fixture.restart(t)
			if !reflect.DeepEqual(after, fixture.state(t)) {
				t.Fatal("restart repeated settlement effects")
			}
		})
	}
}
