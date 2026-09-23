package proxy

import (
	"net/http"
	"testing"
	"time"
)

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
