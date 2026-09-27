package proxy

import (
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"testing/iotest"
)

func TestHostedPaymentsCheckoutIdentifierFailurePreservesPendingOrder(t *testing.T) {
	fixture := newCheckoutRecoveryFixture(t)
	before := fixture.funds(t)
	var original managedPaymentDeliveryRecord
	if err := fixture.database.database.First(&original, "order_id = ?", fixture.order["id"]).Error; err != nil {
		t.Fatal(err)
	}
	func() {
		originalReader := rand.Reader
		rand.Reader = iotest.ErrReader(errors.New("controlled_checkout_identifier_failure"))
		defer func() { rand.Reader = originalReader }()
		fixture.failStartup(t, "controlled_checkout_identifier_failure")
	}()
	fixture.assertUnavailable(t, before, 0)
	var retained managedPaymentDeliveryRecord
	if err := fixture.database.database.First(&retained, "order_id = ?", fixture.order["id"]).Error; err != nil || !reflect.DeepEqual(original, retained) {
		t.Fatalf("identifier failure changed pending checkout: delivery=%+v error=%v", retained, err)
	}
	var customers int64
	if err := fixture.database.database.Model(&managedPaymentCustomerRecord{}).Count(&customers).Error; err != nil || customers != 0 {
		t.Fatalf("identifier failure retained customers=%d error=%v", customers, err)
	}
	fixture.recover(t, before)
}

func TestHostedPaymentsCheckoutConcurrentCustomerConflictPreventsDispatch(t *testing.T) {
	fixture := newCheckoutRecoveryFixture(t)
	before := fixture.funds(t)
	var order managedFundingOrderRecord
	if err := fixture.database.database.First(&order, "id = ?", fixture.order["id"]).Error; err != nil {
		t.Fatal(err)
	}
	competing := managedPaymentCustomerRecord{
		ID: paymentCustomerKey(order), BillingAccountID: order.BillingAccountID,
		Environment: order.Environment, ProcessorAccountID: order.ProcessorAccountID,
		CustomerID: "ctm_00000000000000000000000002", CreatedAt: fixture.now,
	}
	var inserted atomic.Bool
	original := fixture.processor.server
	processor := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/customers" && inserted.CompareAndSwap(false, true) {
			if err := fixture.database.database.Create(&competing).Error; err != nil {
				t.Error(err)
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		original.Config.Handler.ServeHTTP(writer, request)
	}))
	t.Cleanup(processor.Close)
	fixture.processor.server = processor
	fixture.worker = checkoutWorkerFixture(t, fixture.database, fixture.worker.catalog, fixture.processor, fixture.now)
	if err := fixture.worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	fixture.assertUnavailable(t, before, 0)
	var delivery managedPaymentDeliveryRecord
	if err := fixture.database.database.First(&delivery, "order_id = ?", order.ID).Error; err != nil ||
		delivery.State != paymentDeliveryPending || delivery.Reason != "customer_unavailable" || delivery.TransactionDispatchedAt != nil {
		t.Fatalf("customer conflict lost pending checkout: delivery=%+v error=%v", delivery, err)
	}
	var retained managedPaymentCustomerRecord
	if err := fixture.database.database.First(&retained, "id = ?", competing.ID).Error; err != nil || !inserted.Load() || retained.CustomerID != competing.CustomerID {
		t.Fatalf("customer conflict overwrote existing binding: customer=%+v error=%v", retained, err)
	}
	if err := fixture.database.database.Delete(&competing).Error; err != nil {
		t.Fatal(err)
	}
	fixture.recover(t, before)
}

func TestHostedPaymentsReceiptConflictsPreserveRefundHold(t *testing.T) {
	for _, scenario := range []struct{ column, reason string }{
		{"transaction_id", "financial_state_conflict"},
		{"evidence_digest", "transaction_evidence_changed"},
	} {
		t.Run(scenario.column, func(t *testing.T) {
			fixture := newAdjustmentReadFixture(t)
			before := paymentAdjustmentResources(t, fixture.paymentAuditFixture)
			var observations []managedPaymentStateObservationRecord
			if err := fixture.database.database.Order("id").Find(&observations).Error; err != nil {
				t.Fatal(err)
			}
			var receipt managedPaymentReceiptRecord
			if err := fixture.database.database.First(&receipt, "order_id = ?", fixture.orderID).Error; err != nil {
				t.Fatal(err)
			}
			original, replacement := receipt.EvidenceDigest, sha256Hex("conflicting receipt evidence")
			if scenario.column == "transaction_id" {
				original, replacement = receipt.TransactionID, "txn_00000000000000000000000002"
			}
			if err := fixture.database.database.Model(&receipt).UpdateColumn(scenario.column, replacement).Error; err != nil {
				t.Fatal(err)
			}
			if err := paymentProcessorFixture(t, fixture.checkout, fixture.database).reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			var after []managedPaymentStateObservationRecord
			if err := fixture.database.database.Order("id").Find(&after).Error; err != nil || !reflect.DeepEqual(observations, after) {
				t.Fatalf("receipt conflict retained partial observations: error=%v", err)
			}
			if err := fixture.database.database.Model(&receipt).UpdateColumn(scenario.column, original).Error; err != nil {
				t.Fatal(err)
			}
			event := fixture.assertUnapplied(t, before, paymentInboxReconciliation)
			if event.Reason != scenario.reason {
				t.Fatalf("receipt conflict reason=%s", event.Reason)
			}
			fixture.recover(t)
		})
	}
}
