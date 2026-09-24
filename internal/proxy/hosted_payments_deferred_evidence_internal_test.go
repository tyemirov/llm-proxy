package proxy

import (
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func deferredPaymentEvent(t *testing.T, fixture checkoutRecoveryFixture, sequence int, reason string) {
	t.Helper()
	var event managedPaymentInboxRecord
	if err := fixture.database.database.Where("event_id = ?", fmt.Sprintf("evt_%026d", sequence)).First(&event).Error; err != nil {
		t.Fatal(err)
	}
	if event.State != paymentInboxReconciliation || event.Reason != reason {
		t.Fatalf("deferred event state=%s reason=%s want=%s", event.State, event.Reason, reason)
	}
}

func assertDeferredFundingReplay(t *testing.T, fixture checkoutRecoveryFixture, completed map[string]any, want string, entries int) {
	t.Helper()
	var before map[string]any
	for sequence := 2; sequence <= 3; sequence++ {
		sendPaymentEventFixture(t, fixture.database, completed, paymentTransactionCompleted, sequence)
		worker := paymentProcessorFixture(t, fixture.worker, openJournalTransactionInstance(t, fixture.database))
		now := time.Now().Add(2 * paymentCheckoutRetry)
		worker.now = func() time.Time { return now }
		if err := worker.reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		current := fixture.funds(t)
		current["receipt"] = paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, fixture.path+"/receipt", "", "", http.StatusOK)
		balance := current[fundsBalanceTestPath].(map[string]any)
		history := current["/billing-accounts/billing-journal/ledger-entries?limit=100"].(map[string]any)
		if balance["posted_cents"] != want || balance["available_cents"] != want || len(history["entries"].([]any)) != entries || fixture.processor.creates.Load() != 1 {
			t.Fatalf("deferred funding effects: balance=%v history=%v creates=%d", balance, history, fixture.processor.creates.Load())
		}
		if sequence == 2 {
			before = current
		} else if !reflect.DeepEqual(before, current) {
			t.Fatal("replayed funding repeated a credit or changed its receipt")
		}
	}
}

func TestHostedPaymentsDeferredMalformedEventsPreserveFunds(t *testing.T) {
	for _, scenario := range []struct{ name, eventType, reason string }{
		{"adjustment-without-transaction", "adjustment.created", "adjustment_event_invalid"},
		{"adjustment-invalid-transaction", "adjustment.created", "adjustment_event_invalid"},
		{"transaction-invalid-payments", paymentTransactionCompleted, "transaction_event_invalid"},
		{"unknown-checkout", paymentTransactionCompleted, "checkout_unresolved"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newCheckoutRecoveryFixture(t)
			if err := fixture.worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			completed := completedPaymentFixture(t, fixture.processor)
			data := maps.Clone(completed)
			switch scenario.name {
			case "adjustment-without-transaction":
				data = map[string]any{"id": "adj_00000000000000000000000001"}
			case "adjustment-invalid-transaction":
				data = map[string]any{"id": "adj_00000000000000000000000001", "transaction_id": 42}
			case "transaction-invalid-payments":
				data["payments"] = "unreadable"
			case "unknown-checkout":
				data["id"] = "txn_00000000000000000000000002"
			}
			before := fixture.funds(t)
			sendPaymentEventFixture(t, fixture.database, data, scenario.eventType, 1)
			if err := paymentProcessorFixture(t, fixture.worker, fixture.database).reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			deferredPaymentEvent(t, fixture, 1, scenario.reason)
			if !reflect.DeepEqual(before, fixture.funds(t)) {
				t.Fatal("unverified event changed customer funds")
			}
			paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, fixture.path+"/receipt", "", "", http.StatusNotFound)
			assertDeferredFundingReplay(t, fixture, completed, "500", 1)
		})
	}
}

func TestHostedPaymentsDeferredSupplierMismatchPreservesFunds(t *testing.T) {
	fixture := newCheckoutRecoveryFixture(t)
	if err := fixture.worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	completed := completedPaymentFixture(t, fixture.processor)
	before := fixture.funds(t)
	sendPaymentEventFixture(t, fixture.database, completed, paymentTransactionCompleted, 1)
	var offers []fundingOfferInput
	for _, offer := range fixture.worker.catalog.offers {
		offers = append(offers, offer)
	}
	catalog, err := newFundingCatalog("sandbox", "processor-fixture", "different-supplier", offers)
	if err != nil {
		t.Fatal(err)
	}
	worker := newPaddlePaymentProcessor(fixture.database, catalog, fixture.worker.client)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	deferredPaymentEvent(t, fixture, 1, "supplier_mismatch")
	if !reflect.DeepEqual(before, fixture.funds(t)) {
		t.Fatal("supplier mismatch credited customer funds")
	}
	assertDeferredFundingReplay(t, fixture, completed, "500", 1)
}

func TestHostedPaymentsDeferredAdjustmentBeforeFundingPreservesFunds(t *testing.T) {
	fixture := newCheckoutRecoveryFixture(t)
	if err := fixture.worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	completed := completedPaymentFixture(t, fixture.processor)
	adjustment := paymentAdjustmentFixture(fixture.processor, "approved", 200, 1)
	before := fixture.funds(t)
	sendPaymentEventFixture(t, fixture.database, adjustment, "adjustment.created", 1)
	if err := paymentProcessorFixture(t, fixture.worker, fixture.database).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	deferredPaymentEvent(t, fixture, 1, "funding_receipt_unresolved")
	if !reflect.DeepEqual(before, fixture.funds(t)) {
		t.Fatal("adjustment before funding changed customer funds")
	}
	assertDeferredFundingReplay(t, fixture, completed, "300", 2)
}

func TestHostedPaymentsDeferredAdjustmentEvidencePreservesFunds(t *testing.T) {
	fixture := newCheckoutRecoveryFixture(t)
	if err := fixture.worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	completed := completedPaymentFixture(t, fixture.processor)
	before := fixture.funds(t)
	sendPaymentEventFixture(t, fixture.database, completed, paymentTransactionCompleted, 1)
	fixture.processor.mutex.Lock()
	details := fixture.processor.transactions[0]["details"].(map[string]any)
	original := details["adjusted_totals"]
	details["adjusted_totals"] = nil
	fixture.processor.mutex.Unlock()
	if err := paymentProcessorFixture(t, fixture.worker, fixture.database).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	deferredPaymentEvent(t, fixture, 1, "adjustment_evidence_unavailable")
	if !reflect.DeepEqual(before, fixture.funds(t)) {
		t.Fatal("missing adjustment evidence credited customer funds")
	}
	fixture.processor.mutex.Lock()
	details["adjusted_totals"] = original
	fixture.processor.mutex.Unlock()
	assertDeferredFundingReplay(t, fixture, completed, "500", 1)
}

func TestHostedPaymentsPendingAdjustmentsBoundRemainingPrincipal(t *testing.T) {
	fixture := newPaymentAuditFixture(t)
	fixture.applyAdjustment(t, "approved", 200)
	fixture.processor.mutex.Lock()
	approved := fixture.processor.adjustments[0]
	remaining := fixture.processor.transactions[0]["details"].(map[string]any)["adjusted_totals"]
	fixture.processor.mutex.Unlock()
	pending := paymentAdjustmentFixture(fixture.processor, "pending_approval", 500, 2)
	fixture.processor.mutex.Lock()
	pending["id"] = "adj_00000000000000000000000002"
	fixture.processor.adjustments = []map[string]any{approved, pending}
	fixture.processor.transactions[0]["details"].(map[string]any)["adjusted_totals"] = remaining
	fixture.processor.mutex.Unlock()
	sendPaymentEventFixture(t, fixture.database, pending, "adjustment.created", 3)
	worker := paymentProcessorFixture(t, fixture.checkout, fixture.database)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	balance := fixture.balance(t)
	if balance["posted_cents"] != "300" || balance["available_cents"] != "0" || balance["state"] != "active" {
		t.Fatalf("pending adjustment exceeded remaining principal: %v", balance)
	}
	var projection managedPaymentAdjustmentRecord
	if err := fixture.database.database.First(&projection, "order_id = ?", fixture.orderID).Error; err != nil || projection.PendingCents != 300 || projection.HeldCents != 300 {
		t.Fatalf("unbounded pending hold=%+v error=%v", projection, err)
	}
	fixture.processor.mutex.Lock()
	pending["status"], pending["updated_at"] = "rejected", "2026-09-23T12:03:00Z"
	fixture.processor.transactions[0]["updated_at"] = "2026-09-23T12:03:00Z"
	fixture.processor.mutex.Unlock()
	var before map[string]any
	for sequence := 4; sequence <= 5; sequence++ {
		sendPaymentEventFixture(t, fixture.database, pending, "adjustment.updated", sequence)
		worker = paymentProcessorFixture(t, fixture.checkout, openJournalTransactionInstance(t, fixture.database))
		if err := worker.reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		balance = fixture.balance(t)
		if balance["posted_cents"] != "300" || balance["available_cents"] != "300" {
			t.Fatalf("rejected pending adjustment changed approved refund: %v", balance)
		}
		current := paymentAdjustmentResources(t, fixture)
		if sequence == 4 {
			before = current
		} else if !reflect.DeepEqual(before, current) {
			t.Fatal("pending adjustment replay repeated financial effects")
		}
	}
}
