package proxy

import (
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

type fundingOrderAdmissionRecords struct {
	orders     []managedFundingOrderRecord
	deliveries []managedPaymentDeliveryRecord
}

func retainedFundingAdmissions(t *testing.T, database *gormManagedTenantDatabase) fundingOrderAdmissionRecords {
	t.Helper()
	var records fundingOrderAdmissionRecords
	if err := database.database.Order("id").Find(&records.orders).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.database.Order("order_id").Find(&records.deliveries).Error; err != nil {
		t.Fatal(err)
	}
	return records
}

func TestHostedPaymentsOrderAdmissionFailuresPreserveFundsAndRecover(t *testing.T) {
	for _, scenario := range []struct{ name, statement, key string }{
		{"account-lock", "BEFORE UPDATE OF id ON managed_billing_account_records", "admission-recovery"},
		{"order-insert", "BEFORE INSERT ON managed_funding_order_records", "admission-recovery"},
		{"delivery-insert", "BEFORE INSERT ON managed_payment_delivery_records", "admission-recovery"},
		{"new-order-read", "", "admission-recovery"},
		{"replay-order-read", "", "audit-evidence"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			before := paymentAdjustmentResources(t, fixture)
			admissions := retainedFundingAdmissions(t, fixture.database)
			var rejectRead atomic.Bool
			var rejectedReads atomic.Int64
			const callback = "test:funding_admission_read_failure"
			if scenario.statement == "" {
				rejectRead.Store(true)
				if err := fixture.database.database.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
					if rejectRead.Load() && tx.Statement.Table == "managed_funding_order_records" {
						rejectedReads.Add(1)
						tx.AddError(errors.New("controlled_funding_admission_failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := fixture.database.database.Callback().Query().Remove(callback); err != nil {
						t.Error(err)
					}
				})
			} else if err := fixture.database.database.Exec("CREATE TRIGGER reject_funding_admission " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_funding_admission_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			response := paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodPost, paymentOrdersTestPath, scenario.key, `{"offer_code":"five"}`, http.StatusServiceUnavailable)
			if !reflect.DeepEqual(response, map[string]any{"error": map[string]any{"code": errFundingUnavailable.Error()}}) {
				t.Fatalf("failed admission exposed partial or private data: %v", response)
			}
			rejectRead.Store(false)
			if scenario.statement == "" && rejectedReads.Load() != 1 {
				t.Fatalf("order read failure count=%d", rejectedReads.Load())
			}
			if !reflect.DeepEqual(admissions, retainedFundingAdmissions(t, fixture.database)) || !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) {
				t.Fatal("failed order admission changed retained orders, delivery, or financial resources")
			}
			if scenario.statement != "" {
				if err := fixture.database.database.Exec("DROP TRIGGER reject_funding_admission").Error; err != nil {
					t.Fatal(err)
				}
			}
			var accepted map[string]any
			for iteration := range 2 {
				_, server, cookie := paymentOrdersFixture(t, openJournalTransactionInstance(t, fixture.database))
				order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, scenario.key, `{"offer_code":"five"}`, http.StatusCreated)
				if iteration == 0 {
					accepted = order
				} else if !reflect.DeepEqual(accepted, order) {
					t.Fatal("restarted replay changed the accepted funding order")
				}
				read := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
				if !reflect.DeepEqual(order, read) {
					t.Fatal("accepted funding order differs from its HTTP resource")
				}
			}
			expectedCount := 2
			expectedState := fundingOrderCreated
			if scenario.key == "audit-evidence" {
				expectedCount = 1
				expectedState = "paid"
			}
			after := retainedFundingAdmissions(t, fixture.database)
			if len(after.orders) != expectedCount || len(after.deliveries) != expectedCount || accepted["state"] != expectedState || accepted["funding_cents"] != "500" {
				t.Fatalf("recovery created incorrect funding records: orders=%d deliveries=%d accepted=%v", len(after.orders), len(after.deliveries), accepted)
			}
			if !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) || fixture.processor.creates.Load() != 1 {
				t.Fatal("order recovery changed existing funds or repeated processor work")
			}
		})
	}
}

func TestHostedPaymentsFundingReadRejectionsPreserveFinancialResources(t *testing.T) {
	fixture := newPaymentAuditFixture(t)
	before := paymentAdjustmentResources(t, fixture)
	admissions := retainedFundingAdmissions(t, fixture.database)
	offersPath := "/billing-accounts/billing-journal/funding-offers"
	for _, scenario := range []struct {
		name, path, subject string
		status              int
	}{
		{"unowned-offers", offersPath, "other", http.StatusNotFound},
		{"unauthenticated-offers", offersPath, "", http.StatusUnauthorized},
		{"offer-query", offersPath + "?currency=EUR", "owner", http.StatusBadRequest},
		{"order-query", paymentOrdersTestPath + "/" + fixture.orderID + "?amount=1", "owner", http.StatusBadRequest},
		{"invalid-history-cursor", paymentOrdersTestPath + "?cursor=other-resource", "owner", http.StatusBadRequest},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var cookie *http.Cookie
			if scenario.subject != "" {
				cookie = fixture.cookie(scenario.subject)
			}
			paymentOrderHTTP(t, fixture.server, cookie, http.MethodGet, scenario.path, "", "", scenario.status)
			if !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) || !reflect.DeepEqual(admissions, retainedFundingAdmissions(t, fixture.database)) || fixture.processor.creates.Load() != 1 {
				t.Fatal("rejected funding read changed financial resources or processor work")
			}
		})
	}
}
