package proxy

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

type paymentAuditFixture struct {
	database   *gormManagedTenantDatabase
	server     *httptest.Server
	cookie     func(string) *http.Cookie
	processor  *checkoutProtocolFixture
	checkout   *paddleCheckoutDelivery
	orderID    string
	management ManagementConfiguration
	payments   *PaymentConfiguration
}

func newPaymentAuditFixture(t *testing.T) paymentAuditFixture {
	t.Helper()
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "audit-evidence", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	sendPaymentEventFixture(t, database, completedPaymentFixture(t, processor), "transaction.completed", 1)
	if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	management, payments := paymentReconciliationConfiguration(t, database, processor)
	return paymentAuditFixture{database, server, cookie, processor, checkout, order["id"].(string), management, payments}
}

func (fixture paymentAuditFixture) balance(t *testing.T) map[string]any {
	t.Helper()
	return paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
}

func (fixture paymentAuditFixture) applyAdjustment(t *testing.T, state string, principal int) {
	t.Helper()
	adjustment := paymentAdjustmentFixture(fixture.processor, state, principal, 1)
	sendPaymentEventFixture(t, fixture.database, adjustment, "adjustment.created", 2)
	if err := paymentProcessorFixture(t, fixture.checkout, fixture.database).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestHostedPaymentsReconciliationIdentifiesExactFinancialDifferences(t *testing.T) {
	for _, scenario := range []struct{ name, category, code string }{
		{"older-snapshot", "timing", "processor_snapshot_conflict"},
		{"conflicting-snapshot", "timing", "processor_snapshot_conflict"},
		{"payment-not-completed", "timing", "receipt_without_completion"},
		{"canceled-payment", "timing", "order_state"},
		{"discounted-total", "discount", "payment_discount_amount"},
		{"missing-completed-totals", "amount", "completed_evidence_invalid"},
		{"unknown-fee", "fee", "processor_fee"},
		{"invalid-adjustments", "adjustment", "adjustment_evidence_invalid"},
		{"approved-refund", "adjustment", "reversed_credit"},
		{"full-refund", "timing", "order_state"},
		{"chargeback", "timing", "order_state"},
		{"missing-projection", "adjustment", "adjustment_projection_missing"},
		{"receipt-currency", "currency", "receipt_currency"},
		{"receipt-identity", "identity", "receipt_identity"},
		{"missing-hold", "ledger", "refund_hold"},
		{"short-hold", "adjustment", "refund_hold_shortfall"},
		{"reversal-amount", "ledger", "funding_reversal"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			processor := fixture.processor
			processor.mutex.Lock()
			transaction := processor.transactions[0]
			transaction["updated_at"] = "2026-09-23T12:00:02Z"
			details := transaction["details"].(map[string]any)
			switch scenario.name {
			case "older-snapshot":
				transaction["updated_at"] = "2026-09-23T12:00:00Z"
			case "conflicting-snapshot":
				transaction["updated_at"] = "2026-09-23T12:00:01Z"
				transaction["invoice_number"] = "changed-at-same-revision"
			case "payment-not-completed":
				transaction["status"] = "ready"
			case "canceled-payment":
				transaction["status"] = "canceled"
			case "discounted-total":
				details["totals"].(map[string]any)["discount"] = "10"
			case "missing-completed-totals":
				details["totals"] = nil
			case "unknown-fee":
				delete(details["totals"].(map[string]any), "fee")
			case "invalid-adjustments":
				details["adjusted_totals"] = nil
			}
			processor.mutex.Unlock()
			statement := ""
			switch scenario.name {
			case "approved-refund":
				paymentAdjustmentFixture(processor, "approved", 200, 1)
			case "full-refund":
				paymentAdjustmentFixture(processor, "approved", 500, 1)
			case "chargeback":
				adjustment := paymentAdjustmentFixture(processor, "approved", 500, 1)
				adjustment["action"] = "chargeback"
			case "missing-projection":
				statement = "DELETE FROM managed_payment_adjustment_records"
			case "receipt-currency":
				statement = "UPDATE managed_payment_receipt_records SET currency = 'EUR'"
			case "receipt-identity":
				statement = "UPDATE managed_payment_receipt_records SET customer_id = 'ctm_other'"
			case "missing-hold":
				fixture.applyAdjustment(t, "pending_approval", 200)
				statement = "DELETE FROM reservations"
			case "short-hold":
				fixture.applyAdjustment(t, "pending_approval", 200)
				statement = "UPDATE managed_payment_adjustment_records SET held_cents = 100"
			case "reversal-amount":
				fixture.applyAdjustment(t, "approved", 200)
				statement = "UPDATE ledger_entries SET amount_cents = -199 WHERE type = 'spend'"
			}
			if statement != "" {
				if err := fixture.database.database.Exec(statement).Error; err != nil {
					t.Fatal(err)
				}
			}
			before := fixture.balance(t)
			report, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "compare-"+scenario.name)
			if err != nil || report.State != paymentReconciliationCompleted || report.CompletedOrders != 1 || len(report.Items) != 1 {
				t.Fatalf("financial comparison: report=%+v error=%v", report, err)
			}
			found := false
			for _, difference := range report.Items[0].Differences {
				if difference.Category == scenario.category && difference.Code == scenario.code {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing %s/%s difference: %+v", scenario.category, scenario.code, report.Items[0].Differences)
			}
			// Completed reports retain their evidence after the processor changes again.
			processor.mutex.Lock()
			processor.transactions = nil
			processor.mutex.Unlock()
			replayed, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "compare-"+scenario.name)
			if err != nil || !reflect.DeepEqual(report, replayed) {
				t.Fatalf("completed report changed on replay: report=%+v error=%v", replayed, err)
			}
			if !reflect.DeepEqual(before, fixture.balance(t)) {
				t.Fatal("financial comparison changed customer funds")
			}
		})
	}
}

func TestHostedPaymentsReconciliationNeverAppendsOffsettingLedgerEffects(t *testing.T) {
	fixture := newPaymentAuditFixture(t)
	fixture.applyAdjustment(t, "approved", 200)
	// The processor now reports a larger refund. Only the payment worker may apply it.
	paymentAdjustmentFixture(fixture.processor, "approved", 300, 2)
	balance := fixture.balance(t)
	readEntries := func() map[string]any {
		t.Helper()
		return paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/ledger-entries?limit=100", "", "", http.StatusOK)
	}
	entries := readEntries()
	if len(entries["entries"].([]any)) != 2 {
		t.Fatalf("fixture must contain one funding credit and one refund: %v", entries)
	}
	fixture.processor.mutex.Lock()
	transactions := fixture.processor.transactions
	fixture.processor.transactions = nil
	fixture.processor.mutex.Unlock()
	if _, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "read-only-audit"); err == nil {
		t.Fatal("missing processor evidence was accepted")
	}
	if !reflect.DeepEqual(entries, readEntries()) || !reflect.DeepEqual(balance, fixture.balance(t)) {
		t.Fatal("failed comparison changed Ledger effects")
	}
	fixture.processor.mutex.Lock()
	fixture.processor.transactions = transactions
	fixture.processor.mutex.Unlock()
	report, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "read-only-audit")
	if err != nil || report.CompletedOrders != 1 || len(report.Items) != 1 {
		t.Fatalf("resumed comparison failed: report=%+v error=%v", report, err)
	}
	found := false
	for _, difference := range report.Items[0].Differences {
		found = found || difference.Code == "reversed_credit"
	}
	if !found {
		t.Fatal("comparison did not report the unapplied processor refund")
	}
	for _, runID := range []string{"read-only-audit", "new-read-only-audit"} {
		if _, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, runID); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(entries, readEntries()) || !reflect.DeepEqual(balance, fixture.balance(t)) {
			t.Fatal("comparison appended offsetting entries or changed customer funds")
		}
	}
}
