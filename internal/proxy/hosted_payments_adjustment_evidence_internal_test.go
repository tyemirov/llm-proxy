package proxy

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestHostedPaymentsAdjustmentInvalidEvidencePreservesFundsAndRecovers(t *testing.T) {
	for _, scenario := range []string{
		"missing-adjusted-totals", "adjusted-currency", "invalid-adjusted-amount", "inconsistent-adjusted-total", "excess-remaining",
		"foreign-transaction", "foreign-customer", "subscription", "currency", "duplicate", "missing-item",
		"invalid-created-time", "creation-before-payment", "update-before-creation", "foreign-line", "line-total-mismatch",
		"invalid-fee", "inconsistent-adjustment-total", "unsupported-action", "pending-reversal", "unsupported-status",
		"uncorroborated-refund", "missing-adjustment", "processor-unavailable", "changed-original-payment",
	} {
		t.Run(scenario, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "invalid-adjustment", `{"offer_code":"five"}`, http.StatusCreated)
			checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
			if err := checkout.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			transaction := completedPaymentFixture(t, processor)
			sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
			worker := paymentProcessorFixture(t, checkout, database)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			orderPath := paymentOrdersTestPath + "/" + order["id"].(string)
			before := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, orderPath+"/receipt", "", "", http.StatusOK)
			adjustment := paymentAdjustmentFixture(processor, "approved", 200, 1)
			// Deliver an authentic notification, then vary the independent API evidence.
			sendPaymentEventFixture(t, database, adjustment, "adjustment.created", 2)
			processor.mutex.Lock()
			details := transaction["details"].(map[string]any)
			adjusted := details["adjusted_totals"].(map[string]any)
			totals := adjustment["totals"].(map[string]any)
			item := adjustment["items"].([]any)[0].(map[string]any)
			switch scenario {
			case "missing-adjusted-totals":
				delete(details, "adjusted_totals")
			case "adjusted-currency":
				adjusted["currency_code"] = "EUR"
			case "invalid-adjusted-amount":
				adjusted["retained_fee"] = "0.30"
			case "inconsistent-adjusted-total":
				adjusted["total"] = "329"
			case "excess-remaining":
				adjusted["subtotal"], adjusted["tax"], adjusted["total"], adjusted["grand_total"], adjusted["grand_total_tax"] = "600", "60", "660", "660", "60"
			case "foreign-transaction":
				adjustment["transaction_id"] = "txn_00000000000000000000000002"
			case "foreign-customer":
				adjustment["customer_id"] = "ctm_00000000000000000000000002"
			case "subscription":
				adjustment["subscription_id"] = "sub_00000000000000000000000001"
			case "currency":
				adjustment["currency_code"] = "EUR"
			case "duplicate":
				processor.adjustments = append(processor.adjustments, adjustment)
			case "missing-item":
				adjustment["items"] = []any{}
			case "invalid-created-time":
				adjustment["created_at"] = "not-a-timestamp"
			case "creation-before-payment":
				adjustment["created_at"] = "2026-09-23T11:59:59Z"
			case "update-before-creation":
				adjustment["updated_at"] = "2026-09-23T12:00:59Z"
			case "foreign-line":
				item["item_id"] = "txnitm_00000000000000000000000002"
			case "line-total-mismatch":
				item["totals"].(map[string]any)["total"] = "200"
			case "invalid-fee":
				totals["fee"] = "-1"
			case "inconsistent-adjustment-total":
				totals["total"] = "200"
				item["totals"].(map[string]any)["total"] = "200"
			case "unsupported-action":
				adjustment["action"] = "unknown_action"
			case "pending-reversal":
				adjustment["action"], adjustment["status"] = "chargeback_reverse", "pending_approval"
			case "unsupported-status":
				adjustment["status"] = "unknown_status"
			case "uncorroborated-refund":
				adjusted["subtotal"], adjusted["tax"], adjusted["total"], adjusted["grand_total"], adjusted["grand_total_tax"] = "500", "50", "550", "550", "50"
			case "missing-adjustment":
				adjustment["id"] = "adj_00000000000000000000000002"
			case "processor-unavailable":
				processor.transactions = nil
			case "changed-original-payment":
				transaction["invoice_number"] = "INV-CHANGED"
			}
			processor.mutex.Unlock()
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			assertHostedFundsBalance(t, database, 500, 500)
			after := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, orderPath+"/receipt", "", "", http.StatusOK)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("unverified refund changed receipt: before=%v after=%v", before, after)
			}
			var event managedPaymentInboxRecord
			if err := database.database.Where("event_id = ?", "evt_00000000000000000000000002").First(&event).Error; err != nil || event.State != paymentInboxReconciliation || event.Reason == "" {
				t.Fatalf("invalid adjustment not retained for reconciliation: %+v error=%v", event, err)
			}
			processor.mutex.Lock()
			processor.transactions = []map[string]any{transaction}
			transaction["invoice_number"] = "INV-FIXTURE-1"
			processor.mutex.Unlock()
			corrected := paymentAdjustmentFixture(processor, "approved", 200, 2)
			sendPaymentEventFixture(t, database, corrected, "adjustment.updated", 3)
			restarted := paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database))
			for range 2 {
				if err := restarted.reconcile(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			balance := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
			if balance["posted_cents"] != "300" || balance["available_cents"] != "300" {
				t.Fatalf("corrected refund did not apply exactly once: %v", balance)
			}
			receipt := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, orderPath+"/receipt", "", "", http.StatusOK)
			if receipt["reversed_cents"] != "200" || receipt["pending_refund_cents"] != "0" {
				t.Fatalf("corrected refund receipt=%v", receipt)
			}
			history := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/ledger-entries", "", "", http.StatusOK)
			if len(history["entries"].([]any)) != 2 || processor.creates.Load() != 1 {
				t.Fatalf("refund recovery duplicated an effect: %v", history)
			}
		})
	}
}

func TestHostedPaymentsAdjustmentRejectsIncompleteOrRewrittenHistory(t *testing.T) {
	for _, scenario := range []string{"missing-prior-adjustment", "older-adjustment", "rewritten-adjustment"} {
		t.Run(scenario, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "refund-history", `{"offer_code":"five"}`, http.StatusCreated)
			checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
			if err := checkout.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			sendPaymentEventFixture(t, database, completedPaymentFixture(t, processor), "transaction.completed", 1)
			worker := paymentProcessorFixture(t, checkout, database)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			approved := paymentAdjustmentFixture(processor, "approved", 200, 2)
			sendPaymentEventFixture(t, database, approved, "adjustment.created", 2)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			previous, err := json.Marshal(approved)
			if err != nil {
				t.Fatal(err)
			}
			processor.mutex.Lock()
			processor.transactions[0]["updated_at"] = "2026-09-23T12:03:00Z"
			switch scenario {
			case "missing-prior-adjustment":
				approved["id"] = "adj_00000000000000000000000002"
			case "older-adjustment":
				approved["updated_at"] = "2026-09-23T12:01:00Z"
			case "rewritten-adjustment":
				approved["reason"] = "changed without a new revision"
			}
			processor.mutex.Unlock()
			sendPaymentEventFixture(t, database, approved, "adjustment.updated", 3)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			assertHostedFundsBalance(t, database, 300, 300)
			var event managedPaymentInboxRecord
			if err := database.database.Where("event_id = ?", "evt_00000000000000000000000003").First(&event).Error; err != nil || event.State != paymentInboxReconciliation {
				t.Fatalf("changed history not retained: %+v error=%v", event, err)
			}
			var restored map[string]any
			if err := json.Unmarshal(previous, &restored); err != nil {
				t.Fatal(err)
			}
			processor.mutex.Lock()
			restored["updated_at"] = "2026-09-23T12:04:00Z"
			processor.transactions[0]["updated_at"] = restored["updated_at"]
			processor.adjustments = []map[string]any{restored}
			processor.mutex.Unlock()
			sendPaymentEventFixture(t, database, restored, "adjustment.updated", 4)
			if err := paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database)).reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			receipt := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string)+"/receipt", "", "", http.StatusOK)
			if receipt["reversed_cents"] != "200" {
				t.Fatalf("history recovery changed refund: %v", receipt)
			}
			history := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/ledger-entries", "", "", http.StatusOK)
			if len(history["entries"].([]any)) != 2 {
				t.Fatalf("history recovery repeated refund: %v", history)
			}
		})
	}
}
