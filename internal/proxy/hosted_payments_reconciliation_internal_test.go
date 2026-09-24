package proxy

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func paymentReconciliationConfiguration(t *testing.T, database *gormManagedTenantDatabase, processor *checkoutProtocolFixture) (ManagementConfiguration, *PaymentConfiguration) {
	t.Helper()
	if err := bindPaymentEnvironment(database.database, paymentEnvironmentSandbox); err != nil {
		t.Fatal(err)
	}
	return ManagementConfiguration{DatabaseDialector: database.database.Dialector}, &PaymentConfiguration{
		Environment: "sandbox", ClientToken: "test_fixture", ProcessorAccountID: "processor-fixture", SupplierID: "supplier-fixture", APIKey: "checkout-fixture-key", APIBaseURL: processor.server.URL, WebhookSecret: "fixture-secret",
		Offers: []PaymentOfferConfiguration{{Code: "five", PriceID: "pri_01hv8x2axb33yr5y238zfwcn5p", FundingCents: 500}},
	}
}

func TestHostedPaymentsReconciliationRetainsDifferencesWithoutFinancialEffects(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "audit-order", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	transaction := completedPaymentFixture(t, processor)
	management, payments := paymentReconciliationConfiguration(t, database, processor)
	report, err := ReconcilePayments(t.Context(), management, payments, "audit-before-event")
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "completed" || report.CompletedOrders != 1 || len(report.Items) != 1 || report.Items[0].OrderID != order["id"] {
		t.Fatalf("report=%+v", report)
	}
	found := false
	for _, difference := range report.Items[0].Differences {
		if difference.Code == "receipt_missing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing payment concealed: %+v", report.Items)
	}
	assertHostedFundsBalance(t, database, 0, 0)
	sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
	if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	retained, err := ReconcilePayments(t.Context(), management, payments, "audit-before-event")
	if err != nil || !reflect.DeepEqual(retained, report) {
		t.Fatalf("replay changed report: %+v %v", retained, err)
	}
	current, err := ReconcilePayments(t.Context(), management, payments, "audit-after-event")
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Items) != 1 || len(current.Items[0].Differences) != 0 {
		t.Fatalf("verified credit differs: %+v", current)
	}
	assertHostedFundsBalance(t, database, 500, 500)
}

func TestHostedPaymentsReconciliationCheckpointRollsBackAndResumes(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "audit-recovery", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	paymentStateFixture(t, processor, "ready", "2026-09-23T12:00:00Z")
	management, payments := paymentReconciliationConfiguration(t, database, processor)
	if err := database.database.Exec("CREATE TRIGGER reject_audit_checkpoint BEFORE UPDATE OF completed_orders ON managed_payment_reconciliation_run_records BEGIN SELECT RAISE(ABORT, 'controlled_checkpoint_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcilePayments(t.Context(), management, payments, "audit-resume"); err == nil {
		t.Fatal("checkpoint failure ignored")
	}
	var run managedPaymentReconciliationRunRecord
	if err := database.database.First(&run).Error; err != nil || run.CompletedOrders != 0 {
		t.Fatalf("checkpoint=%+v error=%v", run, err)
	}
	var items []managedPaymentReconciliationItemRecord
	if err := database.database.Find(&items).Error; err != nil || len(items) != 1 || items[0].State != "pending" || items[0].Result != "" {
		t.Fatalf("partial item=%+v error=%v", items, err)
	}
	if err := database.database.Exec("DROP TRIGGER reject_audit_checkpoint").Error; err != nil {
		t.Fatal(err)
	}
	report, err := ReconcilePayments(t.Context(), management, payments, "audit-resume")
	if err != nil || report.CompletedOrders != 1 || report.State != "completed" {
		t.Fatalf("resume=%+v error=%v", report, err)
	}
	payments.ProcessorAccountID = "different-processor"
	if _, err := ReconcilePayments(t.Context(), management, payments, "audit-resume"); err == nil {
		t.Fatal("run identity changed")
	}
	assertHostedFundsBalance(t, database, 0, 0)
}

func TestHostedPaymentsReconciliationReportsSeededFinancialDifferences(t *testing.T) {
	for _, scenario := range []struct{ name, category string }{{"currency", "currency"}, {"fee", "fee"}, {"discount", "discount"}, {"ledger", "ledger"}, {"duplicate", "duplicate_effect"}, {"receipt-credit", "amount"}, {"refund", "adjustment"}} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "audit-seeded", `{"offer_code":"five"}`, http.StatusCreated)
			checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
			if err := checkout.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			transaction := completedPaymentFixture(t, processor)
			sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
			if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			processor.mutex.Lock()
			transaction["updated_at"] = "2026-09-23T12:00:02Z"
			switch scenario.name {
			case "currency":
				transaction["currency_code"] = "EUR"
			case "fee":
				transaction["details"].(map[string]any)["totals"].(map[string]any)["fee"] = "31"
			case "discount":
				transaction["discount_id"] = "dsc_00000000000000000000000001"
			}
			processor.mutex.Unlock()
			if scenario.name == "ledger" {
				if err := database.database.Exec("UPDATE ledger_entries SET amount_cents = 499 WHERE type = 'grant'").Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario.name == "receipt-credit" {
				if err := database.database.Exec("UPDATE managed_payment_receipt_records SET credit_cents = 600").Error; err != nil {
					t.Fatal(err)
				}
				if err := database.database.Exec("UPDATE ledger_entries SET amount_cents = 600 WHERE type = 'grant'").Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario.name == "duplicate" {
				if err := database.database.Exec("INSERT INTO ledger_entries (entry_id, account_id, type, amount_cents, idempotency_key, metadata, created_at) SELECT 'audit-duplicate', account_id, type, amount_cents, 'audit-duplicate', metadata, created_at FROM ledger_entries WHERE type = 'grant'").Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario.name == "refund" {
				paymentAdjustmentFixture(processor, "pending_approval", 200, 1)
			}
			before := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
			management, payments := paymentReconciliationConfiguration(t, database, processor)
			report, err := ReconcilePayments(t.Context(), management, payments, "seeded-"+scenario.name)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, difference := range report.Items[0].Differences {
				if difference.Category == scenario.category {
					found = true
				}
			}
			if !found {
				t.Fatalf("seeded %s concealed: %+v", scenario.name, report.Items)
			}
			after := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("audit changed financial state: before=%v after=%v", before, after)
			}
		})
	}
}

func TestHostedPaymentsReconciliationResumesFixedScopeAcrossConcurrentWorkers(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	first := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "audit-first", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	paymentStateFixture(t, processor, "ready", "2026-09-23T12:00:00Z")
	var unavailable atomic.Bool
	unavailable.Store(true)
	boundary := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if unavailable.Load() {
			http.Error(writer, "controlled outage", http.StatusServiceUnavailable)
			return
		}
		processor.server.Config.Handler.ServeHTTP(writer, request)
	}))
	defer boundary.Close()
	management, payments := paymentReconciliationConfiguration(t, database, processor)
	payments.APIBaseURL = boundary.URL
	if _, err := ReconcilePayments(t.Context(), management, payments, "concurrent-run"); err == nil {
		t.Fatal("processor failure concealed")
	}
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "audit-later", `{"offer_code":"five"}`, http.StatusCreated)
	unavailable.Store(false)
	type outcome struct {
		report PaymentReconciliationReport
		err    error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			report, err := ReconcilePayments(t.Context(), management, payments, "concurrent-run")
			results <- outcome{report, err}
		}()
	}
	firstResult, secondResult := <-results, <-results
	if firstResult.err != nil || secondResult.err != nil {
		t.Fatalf("concurrent runs: %v %v", firstResult.err, secondResult.err)
	}
	if !reflect.DeepEqual(firstResult.report, secondResult.report) {
		t.Fatal("concurrent reports differ")
	}
	report := firstResult.report
	if report.TotalOrders != 1 || report.CompletedOrders != 1 || len(report.Items) != 1 || report.Items[0].OrderID != first["id"] {
		t.Fatalf("scope or checkpoint changed: %+v", report)
	}
	next, err := ReconcilePayments(t.Context(), management, payments, "next-run")
	if err != nil || next.TotalOrders != 2 || next.CompletedOrders != 2 {
		t.Fatalf("next run: %+v %v", next, err)
	}
	assertHostedFundsBalance(t, database, 0, 0)
}
