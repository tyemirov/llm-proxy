package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

type paymentAuditReadFailureDialector struct {
	gorm.Dialector
	table    string
	failures *atomic.Int64
	manyOnly bool
}

func (dialector paymentAuditReadFailureDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	return database.Callback().Query().Before("gorm:query").Register("test:payment_audit_read", func(tx *gorm.DB) {
		if tx.Statement.Table == dialector.table && (!dialector.manyOnly || !tx.Statement.RaiseErrorOnNotFound) {
			dialector.failures.Add(1)
			tx.AddError(errors.New("controlled_payment_audit_read_failure"))
		}
	})
}

func TestHostedPaymentsReconciliationReadFailuresResumeWithoutFinancialChanges(t *testing.T) {
	for _, table := range []string{
		"managed_payment_environment_records", "managed_payment_reconciliation_run_records", "managed_payment_reconciliation_item_records",
		"managed_funding_order_records", "managed_payment_checkout_records", "managed_payment_receipt_records",
		"managed_payment_adjustment_records", "managed_payment_state_observation_records", "ledger_accounts", "ledger_entries", "reservations",
	} {
		t.Run(table, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			fixture.applyAdjustment(t, "pending_approval", 200)
			before := fixture.balance(t)
			management := fixture.management
			var failures atomic.Int64
			management.DatabaseDialector = paymentAuditReadFailureDialector{Dialector: management.DatabaseDialector, table: table, failures: &failures}
			_, err := ReconcilePayments(t.Context(), management, fixture.payments, "read-failure")
			if err == nil || !strings.Contains(err.Error(), "controlled_payment_audit_read_failure") || failures.Load() == 0 {
				t.Fatalf("read failure not reported: failures=%d error=%v", failures.Load(), err)
			}
			if !reflect.DeepEqual(before, fixture.balance(t)) {
				t.Fatal("failed reconciliation changed customer funds")
			}
			report, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "read-failure")
			if err != nil || report.State != paymentReconciliationCompleted || report.CompletedOrders != 1 || len(report.Items) != 1 || len(report.Items[0].Differences) != 0 {
				t.Fatalf("reconciliation did not recover: report=%+v error=%v", report, err)
			}
			replayed, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "read-failure")
			if err != nil || !reflect.DeepEqual(report, replayed) || !reflect.DeepEqual(before, fixture.balance(t)) {
				t.Fatalf("reconciliation recovery changed retained results or funds: error=%v", err)
			}
		})
	}
}

func TestHostedPaymentsReconciliationWritesPreserveAtomicCheckpoints(t *testing.T) {
	for _, scenario := range []struct{ name, statement string }{
		{"run", "BEFORE INSERT ON managed_payment_reconciliation_run_records"},
		{"scope", "BEFORE INSERT ON managed_payment_reconciliation_item_records"},
		{"scope-count", "BEFORE UPDATE OF total_orders ON managed_payment_reconciliation_run_records"},
		{"account-lock", "BEFORE UPDATE ON managed_billing_account_records"},
		{"item", "BEFORE UPDATE ON managed_payment_reconciliation_item_records"},
		{"checkpoint", "BEFORE UPDATE OF completed_orders ON managed_payment_reconciliation_run_records"},
		{"completion", "BEFORE UPDATE OF state ON managed_payment_reconciliation_run_records WHEN NEW.state = 'completed'"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			fixture.applyAdjustment(t, "pending_approval", 200)
			before := fixture.balance(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_payment_audit " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_payment_audit_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			if _, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "write-failure"); err == nil {
				t.Fatal("failed reconciliation write was acknowledged")
			}
			var runs []managedPaymentReconciliationRunRecord
			var items []managedPaymentReconciliationItemRecord
			if err := fixture.database.database.Find(&runs).Error; err != nil {
				t.Fatal(err)
			}
			if err := fixture.database.database.Find(&items).Error; err != nil {
				t.Fatal(err)
			}
			if scenario.name == "run" || scenario.name == "scope" || scenario.name == "scope-count" {
				if len(runs) != 0 || len(items) != 0 {
					t.Fatalf("partial reconciliation scope: runs=%+v items=%+v", runs, items)
				}
			} else {
				wantCompleted := int64(0)
				wantItemState := paymentReconciliationPending
				if scenario.name == "completion" {
					wantCompleted, wantItemState = 1, paymentReconciliationCompleted
				}
				if len(runs) != 1 || runs[0].State != paymentReconciliationPending || runs[0].CompletedOrders != wantCompleted || len(items) != 1 || items[0].State != wantItemState {
					t.Fatalf("invalid retained checkpoint: runs=%+v items=%+v", runs, items)
				}
				if wantCompleted == 0 && (items[0].Result != "" || items[0].Evidence != "" || items[0].EvidenceDigest != "") {
					t.Fatal("failed checkpoint retained a partial result")
				}
			}
			if !reflect.DeepEqual(before, fixture.balance(t)) {
				t.Fatal("failed audit write changed funds")
			}
			if err := fixture.database.database.Exec("DROP TRIGGER reject_payment_audit").Error; err != nil {
				t.Fatal(err)
			}
			report, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "write-failure")
			if err != nil || report.State != paymentReconciliationCompleted || report.CompletedOrders != 1 || len(report.Items) != 1 || len(report.Items[0].Differences) != 0 {
				t.Fatalf("checkpoint did not recover: report=%+v error=%v", report, err)
			}
			replayed, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "write-failure")
			if err != nil || !reflect.DeepEqual(report, replayed) || !reflect.DeepEqual(before, fixture.balance(t)) {
				t.Fatalf("checkpoint replay changed report or funds: error=%v", err)
			}
		})
	}
}

func TestHostedPaymentsReconciliationUnreadableEvidenceStopsAndRecovers(t *testing.T) {
	for _, scenario := range []string{"ledger-metadata", "ledger-order-identity", "retained-report"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			before := fixture.balance(t)
			table, column, original := "ledger_entries", "metadata", ""
			value := `{`
			if scenario == "ledger-order-identity" {
				value = `{"funding_order_id":42}`
			}
			if scenario == "retained-report" {
				if _, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "unreadable-evidence"); err != nil {
					t.Fatal(err)
				}
				table, column = "managed_payment_reconciliation_item_records", "result"
			}
			if err := fixture.database.database.Table(table).Select(column).Scan(&original).Error; err != nil {
				t.Fatal(err)
			}
			if err := fixture.database.database.Table(table).Where("1 = 1").Update(column, value).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "unreadable-evidence"); err == nil {
				t.Fatal("unreadable audit evidence was accepted")
			}
			if !reflect.DeepEqual(before, fixture.balance(t)) {
				t.Fatal("unreadable audit evidence changed funds")
			}
			if err := fixture.database.database.Table(table).Where("1 = 1").Update(column, original).Error; err != nil {
				t.Fatal(err)
			}
			report, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "unreadable-evidence")
			if err != nil || report.State != paymentReconciliationCompleted || report.CompletedOrders != 1 || len(report.Items) != 1 || len(report.Items[0].Differences) != 0 || !reflect.DeepEqual(before, fixture.balance(t)) {
				t.Fatalf("restored evidence did not recover safely: report=%+v error=%v", report, err)
			}
		})
	}
}

func TestHostedPaymentsReconciliationInvalidInputsCreateNoRunOrProviderCall(t *testing.T) {
	for _, scenario := range []string{"run-id", "missing-payments", "invalid-payments", "missing-database", "unavailable-database", "uninitialized-schema", "environment"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			before := fixture.balance(t)
			var calls atomic.Int64
			boundary := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				fixture.processor.server.Config.Handler.ServeHTTP(writer, request)
			}))
			defer boundary.Close()
			management, payments, runID := fixture.management, *fixture.payments, "invalid-inputs"
			payments.APIBaseURL = boundary.URL
			selected := &payments
			switch scenario {
			case "run-id":
				runID = ""
			case "missing-payments":
				selected = nil
			case "invalid-payments":
				payments.ClientToken = "live_wrong_environment"
			case "missing-database":
				management = ManagementConfiguration{}
			case "unavailable-database":
				management = ManagementConfiguration{DatabasePath: filepath.Join(t.TempDir(), "missing", "database.sqlite")}
			case "uninitialized-schema":
				management = ManagementConfiguration{DatabasePath: filepath.Join(t.TempDir(), "empty.sqlite")}
			case "environment":
				if err := fixture.database.database.Exec("UPDATE managed_payment_environment_records SET environment = 'production'").Error; err != nil {
					t.Fatal(err)
				}
			}
			if _, err := ReconcilePayments(t.Context(), management, selected, runID); err == nil {
				t.Fatal("invalid reconciliation inputs were accepted")
			}
			var runs int64
			if err := fixture.database.database.Model(&managedPaymentReconciliationRunRecord{}).Count(&runs).Error; err != nil || runs != 0 || calls.Load() != 0 {
				t.Fatalf("invalid input created work: runs=%d calls=%d error=%v", runs, calls.Load(), err)
			}
			if !reflect.DeepEqual(before, fixture.balance(t)) {
				t.Fatal("invalid reconciliation input changed funds")
			}
		})
	}
}

func TestHostedPaymentsReconciliationReportReadFailurePreservesCompletedRun(t *testing.T) {
	for _, table := range []string{"managed_payment_reconciliation_run_records", "managed_payment_reconciliation_item_records"} {
		t.Run(table, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			report, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "completed-report")
			if err != nil {
				t.Fatal(err)
			}
			before := fixture.balance(t)
			management := fixture.management
			var failures atomic.Int64
			management.DatabaseDialector = paymentAuditReadFailureDialector{Dialector: management.DatabaseDialector, table: table, failures: &failures, manyOnly: table == "managed_payment_reconciliation_item_records"}
			if _, err := ReconcilePayments(t.Context(), management, fixture.payments, "completed-report"); err == nil || failures.Load() == 0 {
				t.Fatalf("report read failure concealed: failures=%d error=%v", failures.Load(), err)
			}
			recovered, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, "completed-report")
			if err != nil || !reflect.DeepEqual(report, recovered) || !reflect.DeepEqual(before, fixture.balance(t)) {
				t.Fatalf("report retrieval changed completed evidence or funds: error=%v", err)
			}
		})
	}
}

func TestHostedPaymentsReconciliationConcurrentObservationsShareOneCheckpoint(t *testing.T) {
	fixture := newPaymentAuditFixture(t)
	before := fixture.balance(t)
	var observations atomic.Int64
	release := make(chan struct{})
	boundary := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/transactions/") {
			if observations.Add(1) == 2 {
				close(release)
			}
			select {
			case <-release:
			case <-request.Context().Done():
				return
			}
		}
		fixture.processor.server.Config.Handler.ServeHTTP(writer, request)
	}))
	defer boundary.Close()
	payments := *fixture.payments
	payments.APIBaseURL = boundary.URL
	type outcome struct {
		report PaymentReconciliationReport
		err    error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			report, err := ReconcilePayments(t.Context(), fixture.management, &payments, "simultaneous-evidence")
			results <- outcome{report, err}
		}()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || observations.Load() != 2 {
		t.Fatalf("concurrent observations: first=%v second=%v calls=%d", first.err, second.err, observations.Load())
	}
	if !reflect.DeepEqual(first.report, second.report) || first.report.State != paymentReconciliationCompleted || first.report.CompletedOrders != 1 || len(first.report.Items) != 1 || len(first.report.Items[0].Differences) != 0 {
		t.Fatalf("concurrent reports do not share one result: first=%+v second=%+v", first.report, second.report)
	}
	if !reflect.DeepEqual(before, fixture.balance(t)) {
		t.Fatal("concurrent comparison changed customer funds")
	}
}
