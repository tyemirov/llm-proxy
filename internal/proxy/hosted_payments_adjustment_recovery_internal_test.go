package proxy

import (
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestHostedPaymentsAdjustmentWritesRollBackTogetherAndRecover(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		previous  string
		next      string
		statement string
		posted    string
		available string
	}{
		{"hold", "", "pending_approval", "BEFORE INSERT ON reservations", "500", "300"},
		{"hold-projection", "", "pending_approval", "BEFORE UPDATE ON managed_payment_adjustment_records", "500", "300"},
		{"hold-revision", "", "pending_approval", "BEFORE INSERT ON managed_payment_adjustment_revision_records", "500", "300"},
		{"hold-event", "", "pending_approval", "BEFORE UPDATE ON managed_payment_inbox_records WHEN NEW.state = 'applied'", "500", "300"},
		{"release", "pending_approval", "approved", "BEFORE UPDATE ON reservations", "300", "300"},
		{"refund-entry", "pending_approval", "approved", "BEFORE INSERT ON ledger_entries WHEN NEW.amount_cents < 0", "300", "300"},
		{"refund-order", "pending_approval", "approved", "BEFORE UPDATE ON managed_funding_order_records", "300", "300"},
		{"refund-projection", "pending_approval", "approved", "BEFORE UPDATE ON managed_payment_adjustment_records", "300", "300"},
		{"refund-revision", "pending_approval", "approved", "BEFORE INSERT ON managed_payment_adjustment_revision_records", "300", "300"},
		{"refund-event", "pending_approval", "approved", "BEFORE UPDATE ON managed_payment_inbox_records WHEN NEW.state = 'applied'", "300", "300"},
		{"restored-credit", "approved", "reversed", "BEFORE INSERT ON ledger_entries", "500", "500"},
		{"restored-projection", "approved", "reversed", "BEFORE UPDATE ON managed_payment_adjustment_records", "500", "500"},
		{"restored-revision", "approved", "reversed", "BEFORE INSERT ON managed_payment_adjustment_revision_records", "500", "500"},
		{"restored-event", "approved", "reversed", "BEFORE UPDATE ON managed_payment_inbox_records WHEN NEW.state = 'applied'", "500", "500"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "adjustment-write-recovery", `{"offer_code":"five"}`, http.StatusCreated)
			checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
			if err := checkout.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			sendPaymentEventFixture(t, database, completedPaymentFixture(t, processor), "transaction.completed", 1)
			worker := paymentProcessorFixture(t, checkout, database)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			if scenario.previous != "" {
				previous := paymentAdjustmentFixture(processor, scenario.previous, 200, 1)
				sendPaymentEventFixture(t, database, previous, "adjustment.created", 2)
				if err := worker.reconcile(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			orderPath := paymentOrdersTestPath + "/" + order["id"].(string)
			paths := []string{orderPath, orderPath + "/receipt", fundsBalanceTestPath, "/billing-accounts/billing-journal/ledger-entries"}
			before := make([]map[string]any, len(paths))
			for index, path := range paths {
				before[index] = paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
			}
			next := paymentAdjustmentFixture(processor, scenario.next, 200, 2)
			sendPaymentEventFixture(t, database, next, "adjustment.updated", 3)
			if err := database.database.Exec("CREATE TRIGGER reject_refund_write " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_refund_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			if err := worker.reconcile(t.Context()); err == nil {
				t.Fatalf("refund failure not reported: %v", err)
			}
			for index, path := range paths {
				after := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
				if !reflect.DeepEqual(before[index], after) {
					t.Fatalf("failed adjustment changed %s: before=%v after=%v", path, before[index], after)
				}
			}
			var event managedPaymentInboxRecord
			if err := database.database.Where("event_id = ?", "evt_00000000000000000000000003").First(&event).Error; err != nil || event.State != paymentInboxPending {
				t.Fatalf("failed adjustment acknowledged: %+v error=%v", event, err)
			}
			if err := database.database.Exec("DROP TRIGGER reject_refund_write").Error; err != nil {
				t.Fatal(err)
			}
			restarted := paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database))
			if err := restarted.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			balance := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
			if balance["posted_cents"] != scenario.posted || balance["available_cents"] != scenario.available {
				t.Fatalf("recovered balance=%v", balance)
			}
			for index, path := range paths {
				before[index] = paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
			}
			// A separate notification for the same processor state has no new effect.
			sendPaymentEventFixture(t, database, next, "adjustment.updated", 4)
			if err := restarted.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			for index, path := range paths {
				after := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
				if !reflect.DeepEqual(before[index], after) {
					t.Fatalf("replayed adjustment changed %s: before=%v after=%v", path, before[index], after)
				}
			}
		})
	}
}

func TestHostedPaymentsAdjustmentTaxInclusiveRoundingConservesFunding(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "tax-inclusive-refund", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	transaction := completedPaymentFixture(t, processor)
	processor.mutex.Lock()
	details := transaction["details"].(map[string]any)
	totals := details["totals"].(map[string]any)
	totals["subtotal"], totals["tax"], totals["total"], totals["grand_total"], totals["grand_total_tax"] = "454", "46", "500", "500", "46"
	line := details["line_items"].([]any)[0].(map[string]any)["totals"].(map[string]any)
	line["subtotal"], line["tax"], line["total"] = "454", "46", "500"
	transaction["payments"].([]any)[0].(map[string]any)["amount"] = "500"
	details["adjusted_totals"] = map[string]any{"subtotal": "454", "tax": "46", "total": "500", "grand_total": "500", "grand_total_tax": "46", "fee": "30", "retained_fee": "0", "currency_code": "USD"}
	processor.mutex.Unlock()
	sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
	worker := paymentProcessorFixture(t, checkout, database)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	for index, step := range []struct {
		status    string
		principal int
		remaining string
		tax       string
		total     string
		posted    string
		available string
		reversed  string
		pending   string
	}{
		{"pending_approval", 1, "454", "46", "500", "500", "498", "0", "2"},
		{"approved", 1, "453", "46", "499", "499", "499", "1", "0"},
		{"approved", 454, "0", "0", "0", "0", "0", "500", "0"},
	} {
		adjustment := paymentAdjustmentFixture(processor, step.status, step.principal, index+1)
		processor.mutex.Lock()
		adjusted := transaction["details"].(map[string]any)["adjusted_totals"].(map[string]any)
		adjusted["subtotal"], adjusted["tax"], adjusted["total"], adjusted["grand_total"], adjusted["grand_total_tax"] = step.remaining, step.tax, step.total, step.total, step.tax
		if step.principal == 454 {
			adjustment["totals"].(map[string]any)["tax"] = "46"
			adjustment["totals"].(map[string]any)["total"] = "500"
			item := adjustment["items"].([]any)[0].(map[string]any)["totals"].(map[string]any)
			item["tax"], item["total"] = "46", "500"
		}
		processor.mutex.Unlock()
		sendPaymentEventFixture(t, database, adjustment, "adjustment.updated", index+2)
		if err := worker.reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		balance := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
		if balance["posted_cents"] != step.posted || balance["available_cents"] != step.available {
			t.Fatalf("step %d rounded balance=%v", index, balance)
		}
		receipt := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string)+"/receipt", "", "", http.StatusOK)
		if receipt["reversed_cents"] != step.reversed || receipt["pending_refund_cents"] != step.pending {
			t.Fatalf("step %d rounded refund=%v", index, receipt)
		}
		worker = paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database))
	}
}
