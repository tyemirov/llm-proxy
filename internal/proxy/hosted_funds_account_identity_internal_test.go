package proxy

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedPaymentsCompletedRejectsEmptyStoredAccount(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "empty-funding-account", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	transaction := completedPaymentFixture(t, processor)
	processor.mutex.Lock()
	transaction["custom_data"].(map[string]string)[paymentMetadataAccount] = ""
	processor.mutex.Unlock()
	replaceStoredBillingAccountIdentity(t, database.database, "billing-journal", "")
	sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
	worker := paymentProcessorFixture(t, checkout, database)
	if err := worker.reconcile(t.Context()); err == nil || !strings.Contains(err.Error(), "construct ledger account") {
		t.Fatalf("empty stored account permitted funding: %v", err)
	}
	for _, model := range []any{&managedPaymentReceiptRecord{}, &managedPaymentAdjustmentRecord{}} {
		var count int64
		if err := database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("invalid account retained partial payment evidence: %T count=%d error=%v", model, count, err)
		}
	}
	replaceStoredBillingAccountIdentity(t, database.database, "", "billing-journal")
	assertHostedFundsBalance(t, database, 0, 0)
	processor.mutex.Lock()
	transaction["custom_data"].(map[string]string)[paymentMetadataAccount] = "billing-journal"
	processor.mutex.Unlock()
	for sequence := 2; sequence <= 3; sequence++ {
		sendPaymentEventFixture(t, database, transaction, "transaction.completed", sequence)
		worker = paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database))
		worker.now = func() time.Time { return time.Now().Add(3 * paymentCheckoutRetry) }
		if err := worker.reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		assertHostedFundsBalance(t, database, 500, 500)
		history := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/ledger-entries", "", "", http.StatusOK)
		if len(history["entries"].([]any)) != 1 {
			t.Fatalf("restored payment repeated a credit: %v", history)
		}
		paid := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
		if paid["state"] != "paid" {
			t.Fatalf("restored payment did not complete: %v", paid)
		}
	}
}

// Keep every foreign key valid while reproducing a stored empty account ID.
// Ledger entries retain their original identity and must remain unchanged.
func replaceStoredBillingAccountIdentity(t *testing.T, database *gorm.DB, previous, next string) {
	t.Helper()
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("PRAGMA defer_foreign_keys = ON").Error; err != nil {
			return err
		}
		tables, err := tx.Migrator().GetTables()
		if err != nil {
			return err
		}
		for _, table := range tables {
			if tx.Migrator().HasColumn(table, "billing_account_id") {
				if err := tx.Table(table).Where("billing_account_id = ?", previous).UpdateColumn("billing_account_id", next).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(&managedBillingAccountRecord{}).Where("id = ?", previous).UpdateColumn("id", next).Error
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHostedFundsStartupRejectsEmptyStoredSettlementAccount(t *testing.T) {
	fixture := newFundsStartupFixture(t, startupCompleteUsage)
	before := fixture.state(t)
	for range 2 {
		replaceStoredBillingAccountIdentity(t, fixture.database.database, "billing-journal", "")
		fixture.failStartup(t)
		replaceStoredBillingAccountIdentity(t, fixture.database.database, "", "billing-journal")
		fixture.assertPending(t, before)
		assertHostedFundsBalance(t, fixture.database, 5, 2)
	}
	fixture.assertSettledOnce(t, before)
}

func TestHostedPaymentsAdjustmentRejectsEmptyStoredAccount(t *testing.T) {
	fixture := newAdjustmentReadFixture(t)
	before := paymentAdjustmentResources(t, fixture.paymentAuditFixture)
	for iteration := range 2 {
		replaceStoredBillingAccountIdentity(t, fixture.database.database, "billing-journal", "")
		worker := paymentProcessorFixture(t, fixture.checkout, fixture.database)
		worker.now = func() time.Time { return time.Now().Add(time.Duration(iteration) * paymentCheckoutRetry) }
		err := worker.reconcile(t.Context())
		replaceStoredBillingAccountIdentity(t, fixture.database.database, "", "billing-journal")
		if err != nil {
			t.Fatal(err)
		}
		event := fixture.assertUnapplied(t, before, paymentInboxReconciliation)
		if event.Reason != "transaction_evidence_changed" {
			t.Fatalf("empty stored account did not retain the evidence conflict: %+v", event)
		}
	}
	fixture.recover(t)
}
