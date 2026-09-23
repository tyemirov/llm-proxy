package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

func completedPaymentFixture(t *testing.T, processor *checkoutProtocolFixture) map[string]any {
	t.Helper()
	processor.mutex.Lock()
	defer processor.mutex.Unlock()
	transaction := processor.transactions[0]
	transaction["status"] = "completed"
	transaction["completed_at"] = "2026-09-23T12:00:00Z"
	transaction["updated_at"] = "2026-09-23T12:00:01Z"
	transaction["invoice_number"] = "INV-FIXTURE-1"
	transaction["payments"] = []any{map[string]any{"payment_attempt_id": "pay-fixture-1", "amount": "550", "status": "captured", "captured_at": "2026-09-23T12:00:00Z"}}
	transaction["details"] = map[string]any{
		"adjusted_totals": map[string]any{"subtotal": "500", "tax": "50", "total": "550", "grand_total": "550", "grand_total_tax": "50", "fee": "30", "retained_fee": "0", "earnings": "470", "currency_code": "USD"},
		"totals":          map[string]any{"subtotal": "500", "discount": "0", "tax": "50", "total": "550", "credit": "0", "credit_to_balance": "0", "balance": "0", "grand_total": "550", "grand_total_tax": "50", "fee": "30", "earnings": "470", "currency_code": "USD"},
		"payout_totals":   map[string]any{"subtotal": "450", "discount": "0", "tax": "45", "total": "495", "credit": "0", "credit_to_balance": "0", "balance": "0", "grand_total": "495", "grand_total_tax": "45", "fee": "27", "earnings": "423", "currency_code": "EUR"},
		"line_items":      []any{map[string]any{"id": "txnitm_01hv8x2axb33yr5y238zfwcn5p", "price_id": "pri_01hv8x2axb33yr5y238zfwcn5p", "quantity": 1, "totals": map[string]any{"subtotal": "500", "discount": "0", "tax": "50", "total": "550"}}},
	}
	return transaction
}

func sendPaymentEventFixture(t *testing.T, database *gormManagedTenantDatabase, data map[string]any, eventType string, sequence int) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"event_id": fmt.Sprintf("evt_%026d", sequence), "event_type": eventType, "occurred_at": "2026-09-23T12:00:00Z", "data": data})
	if err != nil {
		t.Fatal(err)
	}
	server := paymentInboxTestServer(t, database, "sandbox", "processor-fixture")
	paymentInboxHTTP(t, server, string(payload), paymentInboxSignature(string(payload), paymentInboxTestSecret, time.Now()), http.StatusOK)
}

func paymentProcessorFixture(t *testing.T, checkout *paddleCheckoutDelivery, database *gormManagedTenantDatabase) *paddlePaymentProcessor {
	t.Helper()
	worker, err := newPaddlePaymentProcessor(database, checkout.catalog, checkout.client)
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func TestHostedPaymentsCompletedCreditCommitsOnceAcrossEventsAndWorkers(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "completed-credit", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	transaction := completedPaymentFixture(t, processor)
	for sequence := 1; sequence <= 3; sequence++ {
		sendPaymentEventFixture(t, database, transaction, "transaction.completed", sequence)
	}
	assertHostedFundsBalance(t, database, 0, 0)
	workers := []*paddlePaymentProcessor{paymentProcessorFixture(t, checkout, database), paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database))}
	var group sync.WaitGroup
	for _, worker := range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := worker.reconcile(t.Context()); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	for _, worker := range workers {
		if err := worker.reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	paid := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
	if paid["state"] != "paid" {
		t.Fatalf("order=%v", paid)
	}
	balance := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
	if balance["posted_cents"] != "500" || balance["available_cents"] != "500" {
		t.Fatalf("balance=%v", balance)
	}
	history := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/ledger-entries", "", "", http.StatusOK)
	if len(history["entries"].([]any)) != 1 {
		t.Fatalf("duplicate credits: %v", history)
	}
	var receipts []managedPaymentReceiptRecord
	if err := database.database.Find(&receipts).Error; err != nil || len(receipts) != 1 {
		t.Fatalf("receipts=%v error=%v", receipts, err)
	}
	receipt := receipts[0]
	if receipt.OrderID != order["id"] || receipt.CreditCents != 500 || receipt.GrossCents != "550" || receipt.TaxCents != "50" || receipt.FeeCents == nil || *receipt.FeeCents != "30" || receipt.PayoutCurrency != "EUR" || receipt.PayoutEarningsCents == nil || *receipt.PayoutEarningsCents != "423" {
		t.Fatalf("lost financial evidence: %+v", receipt)
	}
}

func TestHostedPaymentsCompletedRejectsMismatchedEvidence(t *testing.T) {
	for _, scenario := range []string{"foreign-account", "currency", "amount", "discount", "uncaptured", "paid-only", "event-api-disagreement"} {
		t.Run(scenario, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "mismatch", `{"offer_code":"five"}`, http.StatusCreated)
			checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
			if err := checkout.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			transaction := completedPaymentFixture(t, processor)
			processor.mutex.Lock()
			switch scenario {
			case "foreign-account":
				transaction["custom_data"].(map[string]string)["billing_account_id"] = "billing-other"
			case "currency":
				transaction["currency_code"] = "EUR"
			case "amount":
				transaction["details"].(map[string]any)["totals"].(map[string]any)["total"] = "5"
			case "discount":
				transaction["details"].(map[string]any)["totals"].(map[string]any)["discount"] = "100"
			case "uncaptured":
				transaction["payments"].([]any)[0].(map[string]any)["status"] = "authorized"
			case "paid-only":
				transaction["status"] = "paid"
			}
			processor.mutex.Unlock()
			sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
			if scenario == "event-api-disagreement" {
				processor.mutex.Lock()
				transaction["payments"].([]any)[0].(map[string]any)["amount"] = "500"
				processor.mutex.Unlock()
			}
			if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			assertHostedFundsBalance(t, database, 0, 0)
			pending := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
			if pending["state"] != "pending" {
				t.Fatalf("unverified payment changed order: %v", pending)
			}
			var event managedPaymentInboxRecord
			if err := database.database.First(&event).Error; err != nil || event.State != paymentInboxReconciliation || event.Reason == "" {
				t.Fatalf("event=%+v error=%v", event, err)
			}
		})
	}
}

func TestHostedPaymentsCompletedRollsBackAndRecoversFinancialWrites(t *testing.T) {
	for _, boundary := range []struct{ name, statement string }{
		{"receipt", "BEFORE INSERT ON managed_payment_receipt_records"},
		{"order", "BEFORE UPDATE ON managed_funding_order_records WHEN NEW.state = 'paid'"},
		{"event", "BEFORE UPDATE ON managed_payment_inbox_records WHEN NEW.state = 'applied'"},
	} {
		t.Run(boundary.name, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "credit-rollback", `{"offer_code":"five"}`, http.StatusCreated)
			checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
			if err := checkout.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			transaction := completedPaymentFixture(t, processor)
			sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
			if err := database.database.Exec("CREATE TRIGGER reject_funding " + boundary.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_payment_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err == nil {
				t.Fatal("financial write failure was ignored")
			}
			assertHostedFundsBalance(t, database, 0, 0)
			pending := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
			if pending["state"] != "pending" {
				t.Fatalf("partial commit: %v", pending)
			}
			var count int64
			if err := database.database.Model(&managedPaymentReceiptRecord{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("partial receipt count=%d error=%v", count, err)
			}
			if err := database.database.Exec("DROP TRIGGER reject_funding").Error; err != nil {
				t.Fatal(err)
			}
			restarted := paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database))
			if err := restarted.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			assertHostedFundsBalance(t, database, 500, 500)
			// A delayed payment collection event cannot repeat the credit or
			// regress the order after transaction completion.
			transaction["status"] = "paid"
			sendPaymentEventFixture(t, database, transaction, "transaction.paid", 2)
			if err := restarted.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			paid := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
			if paid["state"] != "paid" {
				t.Fatalf("delayed event regressed order: %v", paid)
			}
			assertHostedFundsBalance(t, database, 500, 500)
		})
	}
}
