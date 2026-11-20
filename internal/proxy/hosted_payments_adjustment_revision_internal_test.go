package proxy

import (
	"net/http"
	"strings"
	"testing"
)

func TestHostedPaymentsRefundUsesIndependentAdjustmentRevision(t *testing.T) {
	fixture := newPaymentAuditFixture(t)
	for index, state := range []string{"pending_approval", "approved"} {
		adjustment := paymentAdjustmentFixture(fixture.processor, state, 200, index+1)
		fixture.processor.mutex.Lock()
		fixture.processor.transactions[0]["updated_at"] = "2026-09-23T12:00:01Z"
		fixture.processor.mutex.Unlock()
		sendPaymentEventFixture(t, fixture.database, adjustment, "adjustment.updated", index+2)
		if err := paymentProcessorFixture(t, fixture.checkout, fixture.database).reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		balance := fixture.balance(t)
		posted, reserved, reversed, expectedState := "500", "200", "0", "paid"
		if state == "approved" {
			posted, reserved, reversed, expectedState = "300", "0", "200", "partially_refunded"
		}
		if balance["posted_cents"] != posted || balance["available_cents"] != "300" || balance["reserved_cents"] != reserved {
			t.Fatalf("refund with unchanged transaction revision: state=%s balance=%v", state, balance)
		}
		receipt := paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+fixture.orderID+"/receipt", "", "", http.StatusOK)
		if receipt["state"] != expectedState || receipt["reversed_cents"] != reversed || receipt["pending_refund_cents"] != reserved {
			t.Fatalf("receipt=%v", receipt)
		}
		sendPaymentEventFixture(t, fixture.database, adjustment, "adjustment.updated", index+2)
		if err := paymentProcessorFixture(t, fixture.checkout, openJournalTransactionInstance(t, fixture.database)).reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		report, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "independent-adjustment-"+state)
		if err != nil || len(report.Items) != 1 || len(report.Items[0].Differences) != 0 {
			t.Fatalf("refund reconciliation: %+v error=%v", report, err)
		}
	}
	entries := paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/ledger-entries", "", "", http.StatusOK)
	grants, reversals := 0, 0
	for _, raw := range entries["entries"].([]any) {
		entry := raw.(map[string]any)
		if entry["type"] == "grant" {
			grants++
		}
		if entry["type"] == "spend" {
			reversals++
		}
	}
	if grants != 1 || reversals != 1 {
		t.Fatalf("duplicate refund effects: %v", entries)
	}
}

func TestHostedPaymentsFixedRevisionRejectsUnverifiedChanges(t *testing.T) {
	for _, scenario := range []string{"status", "identity", "original-money", "capture", "checkout-presence", "uncorroborated-total", "missing-adjustment", "older-adjustment", "conflicting-adjustment"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			adjustment := paymentAdjustmentFixture(fixture.processor, "approved", 200, 2)
			fixture.processor.mutex.Lock()
			transaction := fixture.processor.transactions[0]
			transaction["updated_at"] = "2026-09-23T12:00:01Z"
			fixture.processor.mutex.Unlock()
			sendPaymentEventFixture(t, fixture.database, adjustment, "adjustment.updated", 2)
			worker := paymentProcessorFixture(t, fixture.checkout, fixture.database)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			assertHostedFundsBalance(t, fixture.database, 300, 300)
			fixture.processor.mutex.Lock()
			details := transaction["details"].(map[string]any)
			transaction["checkout"] = map[string]any{"url": "https://changed-checkout.example.test/?_ptxn=" + transaction["id"].(string)}
			switch scenario {
			case "status":
				transaction["status"] = "canceled"
			case "identity":
				transaction["customer_id"] = "ctm_00000000000000000000000002"
			case "original-money":
				details["totals"].(map[string]any)["fee"] = "31"
			case "capture":
				transaction["payments"].([]any)[0].(map[string]any)["captured_at"] = "2026-09-23T11:59:59Z"
			case "checkout-presence":
				transaction["checkout"] = nil
			case "uncorroborated-total":
				adjusted := details["adjusted_totals"].(map[string]any)
				adjusted["subtotal"], adjusted["tax"], adjusted["total"], adjusted["grand_total"], adjusted["grand_total_tax"] = "250", "25", "275", "275", "25"
			case "missing-adjustment":
				fixture.processor.adjustments = nil
			case "older-adjustment":
				adjustment["updated_at"] = "2026-09-23T12:01:30Z"
			case "conflicting-adjustment":
				adjustment["totals"].(map[string]any)["retained_fee"] = "1"
			}
			fixture.processor.mutex.Unlock()
			if scenario == "status" {
				sendPaymentEventFixture(t, fixture.database, paymentStateFixture(t, fixture.processor, "canceled", "2026-09-23T12:00:01Z"), "transaction.canceled", 3)
			} else {
				sendPaymentEventFixture(t, fixture.database, adjustment, "adjustment.updated", 3)
			}
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			assertHostedFundsBalance(t, fixture.database, 300, 300)
			report, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "unverified-fixed-revision")
			if err != nil || len(report.Items) != 1 || len(report.Items[0].Differences) == 0 {
				t.Fatalf("unverified evidence passed reconciliation: %+v error=%v", report, err)
			}
		})
	}
}

func TestHostedPaymentsCorruptRetainedTransactionRevision(t *testing.T) {
	for _, scenario := range []string{"integrity", "decode"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			changes := map[string]any{"evidence": "{"}
			message := "integrity failed"
			if scenario == "decode" {
				changes["evidence_digest"] = sha256Hex("{")
				message = "decode retained payment state"
			}
			if err := fixture.database.database.Model(&managedPaymentStateObservationRecord{}).Where("order_id = ?", fixture.orderID).Updates(changes).Error; err != nil {
				t.Fatal(err)
			}
			sendPaymentEventFixture(t, fixture.database, paymentStateFixture(t, fixture.processor, "completed", "2026-09-23T12:00:01Z"), "transaction.updated", 2)
			if err := paymentProcessorFixture(t, fixture.checkout, fixture.database).reconcile(t.Context()); err == nil || !strings.Contains(err.Error(), message) {
				t.Fatalf("corrupt retained revision error=%v; want %s", err, message)
			}
			if _, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "corrupt-revision"); err == nil || !strings.Contains(err.Error(), message) {
				t.Fatalf("corrupt audit revision error=%v; want %s", err, message)
			}
			assertHostedFundsBalance(t, fixture.database, 500, 500)
		})
	}
}

func TestHostedPaymentsReconciliationRejectsCorruptRefundEvidence(t *testing.T) {
	fixture := newPaymentAuditFixture(t)
	adjustment := paymentAdjustmentFixture(fixture.processor, "approved", 200, 2)
	sendPaymentEventFixture(t, fixture.database, adjustment, "adjustment.updated", 2)
	if err := paymentProcessorFixture(t, fixture.checkout, fixture.database).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, fixture.database, 300, 300)
	if err := fixture.database.database.Model(&managedPaymentAdjustmentRecord{}).Where("order_id = ?", fixture.orderID).UpdateColumn("evidence", "{").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "corrupt-refund-evidence"); err == nil || !strings.Contains(err.Error(), "decode retained adjustment evidence") {
		t.Fatalf("corrupt refund reconciliation error=%v", err)
	}
	assertHostedFundsBalance(t, fixture.database, 300, 300)
}
