package proxy

import (
	"reflect"
	"testing"
)

func TestHostedPaymentsConflictingRevisionsPreserveRefundHold(t *testing.T) {
	for _, scenario := range []string{"older-transaction", "conflicting-transaction", "older-adjustment", "conflicting-adjustment"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			pending := paymentAdjustmentFixture(fixture.processor, "pending_approval", 200, 2)
			sendPaymentEventFixture(t, fixture.database, pending, "adjustment.created", 2)
			worker := paymentProcessorFixture(t, fixture.checkout, fixture.database)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			before := paymentAdjustmentResources(t, fixture)
			balance := fixture.balance(t)
			if balance["posted_cents"] != "500" || balance["available_cents"] != "300" {
				t.Fatalf("pending refund is not held: %v", balance)
			}
			next := paymentAdjustmentFixture(fixture.processor, "approved", 200, 3)
			fixture.processor.mutex.Lock()
			switch scenario {
			case "older-transaction":
				fixture.processor.transactions[0]["updated_at"] = "2026-09-23T12:01:00Z"
			case "conflicting-transaction":
				fixture.processor.transactions[0]["updated_at"] = "2026-09-23T12:02:00Z"
			case "older-adjustment":
				next["updated_at"] = "2026-09-23T12:01:00Z"
			case "conflicting-adjustment":
				next["updated_at"] = "2026-09-23T12:02:00Z"
			}
			fixture.processor.mutex.Unlock()
			sendPaymentEventFixture(t, fixture.database, next, "adjustment.updated", 3)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			recovery := adjustmentReadFixture{fixture, next}
			event := recovery.assertUnapplied(t, before, paymentInboxReconciliation)
			if event.Reason != "financial_state_conflict" {
				t.Fatalf("conflicting revision reason=%s", event.Reason)
			}
			var retained []managedPaymentStateObservationRecord
			if err := fixture.database.database.Order("processor_updated_at").Find(&retained).Error; err != nil || len(retained) != 2 {
				t.Fatalf("conflicting transaction retained partial observations: count=%d error=%v", len(retained), err)
			}
			var revisions []managedPaymentAdjustmentRevisionRecord
			if err := fixture.database.database.Order("revision").Find(&revisions).Error; err != nil || len(revisions) != 2 {
				t.Fatalf("conflicting adjustment retained a financial revision: count=%d error=%v", len(revisions), err)
			}
			recovery.next = paymentAdjustmentFixture(fixture.processor, "approved", 200, 3)
			recovery.recover(t)
			var after []managedPaymentAdjustmentRevisionRecord
			if err := fixture.database.database.Order("revision").Find(&after).Error; err != nil || len(after) != 3 || !reflect.DeepEqual(revisions, after[:2]) {
				t.Fatalf("refund recovery changed prior evidence: count=%d error=%v", len(after), err)
			}
		})
	}
}
