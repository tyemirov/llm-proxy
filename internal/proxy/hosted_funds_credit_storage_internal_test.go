package proxy

import (
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

func TestHostedFundsCreditRejectsUnusableRetainedCharge(t *testing.T) {
	for _, scenario := range []string{"missing-charge", "unresolved-charge", "rating-digest", "rating-json", "released-reservation"} {
		t.Run(scenario, func(t *testing.T) {
			database, management, charges := newFundsCreditFixture(t)
			financial := fundsStartupFixture{database: database, management: management}
			before := financial.state(t)
			charge := charges[0]
			command := fundsCreditCommand(t, charge, "retained-credit")
			table := "managed_charge_records"
			query := "id = ?"
			identifier := charge.ID
			updates := map[string]any{}
			restore := map[string]any{}
			switch scenario {
			case "missing-charge":
				var err error
				command, err = newCustomerChargeAdjustment(charge.BillingAccountID, "charge-absent", "retained-credit", "customer_credit", ExactMoney{Numerator: "1", Denominator: "1000"}, ratingTestAcceptanceTime())
				if err != nil {
					t.Fatal(err)
				}
			case "unresolved-charge":
				updates["state"], restore["state"] = chargeUsageUnresolved, charge.State
			case "rating-digest":
				updates["rating_digest"], restore["rating_digest"] = "invalid", charge.RatingDigest
			case "rating-json":
				updates["rating"], restore["rating"] = []byte("{"), charge.Rating
				updates["rating_digest"], restore["rating_digest"] = sha256Hex("{"), charge.RatingDigest
			case "released-reservation":
				table, query, identifier = "managed_funds_reservation_records", "request_id = ?", charge.RequestID
				updates["state"], restore["state"] = fundsReservationReleased, fundsReservationSettled
			}
			if len(updates) != 0 {
				if err := database.database.Table(table).Where(query, identifier).Updates(updates).Error; err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if err := applyFundsFixtureCredit(t, database, command); err == nil {
					t.Fatal("unusable retained evidence permitted a credit")
				}
				for _, resource := range []string{"balance", "ledger-entries"} {
					current := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/"+resource, "", http.StatusOK)
					if !reflect.DeepEqual(before[resource], current) {
						t.Fatalf("rejected credit changed %s", resource)
					}
				}
				assertFundsCreditRemainder(t, database, "23", "25000")
				for _, model := range []any{&managedChargeAdjustmentRecord{}, &managedFundsCreditRecord{}} {
					var count int64
					if err := database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
						t.Fatalf("rejected credit retained partial records: %T count=%d error=%v", model, count, err)
					}
				}
			}
			if len(restore) != 0 {
				if err := database.database.Table(table).Where(query, identifier).Updates(restore).Error; err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(before, financial.state(t)) {
				t.Fatal("rejected credit changed restored financial resources")
			}
			command = fundsCreditCommand(t, charge, "retained-credit")
			var credited map[string]any
			for iteration := range 2 {
				if err := applyFundsFixtureCredit(t, openJournalTransactionInstance(t, database), command); err != nil {
					t.Fatal(err)
				}
				assertHostedFundsBalance(t, database, 5, 5)
				assertFundsCreditRemainder(t, database, "91", "12500")
				current := financial.state(t)
				if iteration == 0 {
					credited = current
				} else if !reflect.DeepEqual(credited, current) {
					t.Fatal("credit replay changed financial resources")
				}
			}
		})
	}
}

func TestHostedFundsCreditStorageFailuresPreserveExactFinancialState(t *testing.T) {
	for _, scenario := range []struct {
		name, table, trigger string
		ordinal              int64
	}{
		{name: "charge-read", table: "managed_charge_records", ordinal: 1},
		{name: "settlement-charge-read", table: "managed_charge_records", ordinal: 2},
		{name: "existing-credit-read", table: "managed_charge_adjustment_records", ordinal: 1},
		{name: "charge-credits-read", table: "managed_charge_adjustment_records", ordinal: 2},
		{name: "settled-credits-read", table: "managed_charge_adjustment_records", ordinal: 3},
		{name: "reservation-read", table: "managed_funds_reservation_records", ordinal: 1},
		{name: "financial-account-read", table: "managed_funds_account_records", ordinal: 1},
		{name: "settlement-read", table: "managed_funds_settlement_records", ordinal: 1},
		{name: "corrections-read", table: "managed_funds_correction_records", ordinal: 1},
		{name: "ledger-account-read", table: "ledger_accounts", ordinal: 1},
		{name: "account-lock", trigger: "BEFORE UPDATE OF id ON managed_billing_account_records"},
		{name: "credit-write", trigger: "BEFORE INSERT ON managed_charge_adjustment_records"},
		{name: "credit-receipt-write", trigger: "BEFORE INSERT ON managed_funds_credit_records"},
		{name: "ledger-credit-write", trigger: "BEFORE INSERT ON ledger_entries"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, management, charges := newFundsCreditFixture(t)
			financial := fundsStartupFixture{database: database, management: management}
			before := financial.state(t)
			path := "/billing-accounts/billing-journal/charges/" + charges[0].ID
			original := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK)
			command := fundsCreditCommand(t, charges[0], "storage-credit")
			failure := errors.New("controlled_credit_storage_failure")
			var armed atomic.Bool
			var reads, failures atomic.Int64
			if scenario.table != "" {
				callback := database.database.Callback().Query()
				const name = "test:credit_storage_read"
				if err := callback.Before("gorm:query").Register(name, func(tx *gorm.DB) {
					if armed.Load() && !tx.DryRun && tx.Statement.Table == scenario.table && reads.Add(1) == scenario.ordinal {
						failures.Add(1)
						tx.AddError(failure)
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := callback.Remove(name); err != nil {
						t.Error(err)
					}
				})
			} else if err := database.database.Exec("CREATE TRIGGER reject_credit_storage " + scenario.trigger + " BEGIN SELECT RAISE(ABORT, 'controlled_credit_storage_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			for range 2 {
				reads.Store(0)
				armed.Store(true)
				err := applyFundsFixtureCredit(t, database, command)
				armed.Store(false)
				if err == nil || scenario.table != "" && !errors.Is(err, failure) {
					t.Fatalf("credit storage error=%v", err)
				}
				if !reflect.DeepEqual(before, financial.state(t)) {
					t.Fatal("failed credit changed public financial resources")
				}
				assertHostedFundsBalance(t, database, 4, 4)
				assertFundsCreditRemainder(t, database, "23", "25000")
				for _, model := range []any{&managedChargeAdjustmentRecord{}, &managedFundsCreditRecord{}} {
					var count int64
					if err := database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
						t.Fatalf("failed credit retained partial records: %T count=%d error=%v", model, count, err)
					}
				}
			}
			if scenario.table != "" && failures.Load() != 2 {
				t.Fatalf("credit read failures=%d want=2", failures.Load())
			}
			if scenario.trigger != "" {
				if err := database.database.Exec("DROP TRIGGER reject_credit_storage").Error; err != nil {
					t.Fatal(err)
				}
			}
			var recovered map[string]any
			for iteration := range 2 {
				if err := applyFundsFixtureCredit(t, openJournalTransactionInstance(t, database), command); err != nil {
					t.Fatal(err)
				}
				assertHostedFundsBalance(t, database, 5, 5)
				assertFundsCreditRemainder(t, database, "91", "12500")
				current := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK)
				if !reflect.DeepEqual(current["rating"], original["rating"]) || !reflect.DeepEqual(current["customer_charge"], original["customer_charge"]) || len(current["customer_adjustments"].([]any)) != 1 || !reflect.DeepEqual(current["net_customer_charge"], map[string]any{"numerator": "0", "denominator": "1"}) {
					t.Fatalf("recovered credit changed original cost or repeated: %v", current)
				}
				state := financial.state(t)
				if iteration > 0 && !reflect.DeepEqual(recovered, state) {
					t.Fatal("reopened credit replay changed financial effects")
				}
				recovered = state
			}
		})
	}
}
