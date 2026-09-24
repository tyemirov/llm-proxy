package proxy

import (
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedPaymentsReadsFailWithoutPartialAmountsAndRecover(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "read-failure", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	service.paymentPortal = checkout.client
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	sendPaymentEventFixture(t, database, completedPaymentFixture(t, processor), "transaction.completed", 1)
	if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	orderPath := paymentOrdersTestPath + "/" + order["id"].(string)
	var failedTable atomic.Pointer[string]
	var failures atomic.Int64
	const callback = "test:payment_resource_read_failure"
	if err := database.database.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if table := failedTable.Load(); table != nil && tx.Statement.Table == *table {
			failures.Add(1)
			tx.AddError(errors.New("controlled_private_payment_read_failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.database.Callback().Query().Remove(callback); err != nil {
			t.Error(err)
		}
	})
	for _, scenario := range []struct{ name, path, table string }{
		{"orders", paymentOrdersTestPath, "managed_funding_order_records"},
		{"order", orderPath, "managed_funding_order_records"},
		{"checkout", orderPath + "/checkout", "managed_payment_checkout_records"},
		{"receipt", orderPath + "/receipt", "managed_payment_receipt_records"},
		{"receipt-order", orderPath + "/receipt", "managed_funding_order_records"},
		{"receipt-adjustment", orderPath + "/receipt", "managed_payment_adjustment_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, scenario.path, "", "", http.StatusOK)
			failures.Store(0)
			failedTable.Store(&scenario.table)
			defer failedTable.Store(nil)
			response := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, scenario.path, "", "", http.StatusServiceUnavailable)
			failedTable.Store(nil)
			if failures.Load() == 0 || !reflect.DeepEqual(response, map[string]any{"error": map[string]any{"code": errFundingUnavailable.Error()}}) {
				t.Fatalf("payment read failure exposed a partial or private response: %v", response)
			}
			after := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, scenario.path, "", "", http.StatusOK)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("payment read recovery changed evidence: before=%v after=%v", before, after)
			}
			assertHostedFundsBalance(t, database, 500, 500)
		})
	}
	t.Run("portal-customer", func(t *testing.T) {
		table := "managed_payment_customer_records"
		failedTable.Store(&table)
		defer failedTable.Store(nil)
		paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, "/billing-accounts/billing-journal/payment-portal-sessions", "", `{}`, http.StatusServiceUnavailable)
		if processor.portalCalls.Load() != 0 {
			t.Fatal("customer read failure created a portal session")
		}
		failedTable.Store(nil)
		paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, "/billing-accounts/billing-journal/payment-portal-sessions", "", `{}`, http.StatusCreated)
	})
}

func TestHostedPaymentsReceiptSeparatesCustomerAmountsAndPrivateEvidence(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "receipt", `{"offer_code":"five"}`, http.StatusCreated)
	path := paymentOrdersTestPath + "/" + order["id"].(string) + "/receipt"
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusNotFound)
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
	receipt := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
	if receipt["funding_order_id"] != order["id"] || receipt["credit_cents"] != "500" || receipt["gross_cents"] != "550" || receipt["tax_cents"] != "50" || receipt["invoice_number"] != "INV-FIXTURE-1" || receipt["reversed_cents"] != "0" || receipt["pending_refund_cents"] != "0" {
		t.Fatalf("receipt=%v", receipt)
	}
	for _, field := range []string{"financial_evidence", "evidence_digest", "fee_cents", "earnings_cents", "customer_id", "processor_account_id", "ledger_key", "inbox_id", "payout_currency"} {
		if _, present := receipt[field]; present {
			t.Fatalf("private receipt field: %s", field)
		}
	}
	paymentOrderHTTP(t, server, cookie("other"), http.MethodGet, path, "", "", http.StatusNotFound)
	paymentOrderHTTP(t, server, nil, http.MethodGet, path, "", "", http.StatusUnauthorized)
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path+"?account=other", "", "", http.StatusBadRequest)
	adjustment := paymentAdjustmentFixture(processor, "pending_approval", 200, 1)
	sendPaymentEventFixture(t, database, adjustment, "adjustment.created", 2)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	receipt = paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
	if receipt["pending_refund_cents"] != "200" || receipt["reversed_cents"] != "0" {
		t.Fatalf("pending receipt=%v", receipt)
	}
	adjustment = paymentAdjustmentFixture(processor, "approved", 200, 2)
	sendPaymentEventFixture(t, database, adjustment, "adjustment.updated", 3)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	service.store.database = openJournalTransactionInstance(t, database)
	receipt = paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
	if receipt["pending_refund_cents"] != "0" || receipt["reversed_cents"] != "200" || receipt["state"] != "partially_refunded" {
		t.Fatalf("refunded receipt=%v", receipt)
	}
	if receipt["adjusted_gross_cents"] != "330" || receipt["adjusted_tax_cents"] != "30" {
		t.Fatalf("adjusted processor amounts=%v", receipt)
	}
}

func TestHostedPaymentsPortalUsesOwnedCustomerAndRejectsFinancialInputs(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	service.paymentPortal = checkout.client
	path := "/billing-accounts/billing-journal/payment-portal-sessions"
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, path, "", `{}`, http.StatusNotFound)
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "portal", `{"offer_code":"five"}`, http.StatusCreated)
	checkout.now = time.Now
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"customer_id":"ctm_other"}`, `{"environment":"production"}`, `{"return_url":"https://evil.test"}`, `{"amount":500}`, `null`, `[]`, `{} {}`} {
		paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, path, "", body, http.StatusBadRequest)
	}
	paymentOrderHTTP(t, server, cookie("other"), http.MethodPost, path, "", `{}`, http.StatusNotFound)
	paymentOrderHTTP(t, server, nil, http.MethodPost, path, "", `{}`, http.StatusUnauthorized)
	if processor.portalCalls.Load() != 0 {
		t.Fatal("unowned portal session created")
	}
	for range 2 {
		session := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, path, "", `{}`, http.StatusCreated)
		if session["provider"] != "paddle" || session["environment"] != "sandbox" || session["url"] != processor.portalURL {
			t.Fatalf("portal session=%v", session)
		}
	}
	if processor.portalCalls.Load() != 2 {
		t.Fatal("temporary portal session was cached")
	}
	assertHostedFundsBalance(t, database, 0, 0)
	service.funding.processorAccountID = "another-processor"
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, path, "", `{}`, http.StatusNotFound)
	service.funding.processorAccountID = "processor-fixture"
	processor.mutex.Lock()
	processor.portalURL = "javascript:alert(1)"
	processor.mutex.Unlock()
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, path, "", `{}`, http.StatusServiceUnavailable)
	service.paymentPortal = nil
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, path, "", `{}`, http.StatusServiceUnavailable)
}
