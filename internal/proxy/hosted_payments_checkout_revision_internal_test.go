package proxy

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestHostedPaymentsGeneratedCheckoutLinkIsIndependentOfRevision(t *testing.T) {
	for _, refunded := range []bool{false, true} {
		name := "paid"
		if refunded {
			name = "refunded"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			if refunded {
				fixture.applyAdjustment(t, "approved", 500)
			}
			balance := fixture.balance(t)
			ledgerPath := "/billing-accounts/billing-journal/ledger-entries"
			ledger := paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, ledgerPath, "", "", http.StatusOK)
			path := paymentOrdersTestPath + "/" + fixture.orderID + "/receipt"
			receipt := paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
			fixture.processor.mutex.Lock()
			transaction := fixture.processor.transactions[0]
			checkoutURL := "https://llm-checkout.example.test/?_ptxn=" + transaction["id"].(string)
			transaction["checkout"] = map[string]any{"url": checkoutURL}
			fixture.processor.mutex.Unlock()
			report, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "changed-generated-link")
			if err != nil || len(report.Items) != 1 || len(report.Items[0].Differences) != 0 {
				t.Fatalf("generated URL changed transaction revision: %+v error=%v", report, err)
			}
			var audit managedPaymentReconciliationItemRecord
			if err := fixture.database.database.Where("run_id = ?", report.RunID).First(&audit).Error; err != nil || !strings.Contains(audit.Evidence, checkoutURL) || sha256Hex(audit.Evidence) != audit.EvidenceDigest {
				t.Fatalf("reconciliation lost generated URL evidence: %+v error=%v", audit, err)
			}
			sendPaymentEventFixture(t, fixture.database, transaction, "transaction.updated", 20)
			if err := paymentProcessorFixture(t, fixture.checkout, fixture.database).reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			sendPaymentEventFixture(t, fixture.database, transaction, "transaction.updated", 20)
			if err := paymentProcessorFixture(t, fixture.checkout, openJournalTransactionInstance(t, fixture.database)).reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(balance, fixture.balance(t)) || !reflect.DeepEqual(receipt, paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)) {
				t.Fatal("generated link changed customer funds or receipt")
			}
			if !reflect.DeepEqual(ledger, paymentOrderHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, ledgerPath, "", "", http.StatusOK)) {
				t.Fatal("checkout URL replay added ledger effects")
			}
			var observation managedPaymentStateObservationRecord
			if err := fixture.database.database.Where("order_id = ? AND evidence LIKE ?", fixture.orderID, "%"+checkoutURL+"%").First(&observation).Error; err != nil || sha256Hex(observation.Evidence) != observation.EvidenceDigest {
				t.Fatalf("observation lost generated URL evidence: %+v error=%v", observation, err)
			}
			var event managedPaymentInboxRecord
			if err := fixture.database.database.Where("event_type = ?", "transaction.updated").First(&event).Error; err != nil || event.State != paymentInboxApplied {
				t.Fatalf("matching lifecycle event was not applied: %+v error=%v", event, err)
			}
		})
	}
}
