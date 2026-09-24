package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

type providerAuditFixture struct {
	database      *gormManagedTenantDatabase
	server        *httptest.Server
	configuration ManagementConfiguration
	normalized    string
	source        string
}

func TestHostedPaymentsProviderReconciliationRejectsInvalidImportInputs(t *testing.T) {
	for _, scenario := range []string{"run-id", "missing-evidence", "missing-source", "evidence-too-large", "source-too-large", "empty-source", "malformed-json", "multiple-json", "invalid-discount", "invalid-fees", "missing-database", "unavailable-database", "missing-schema", "missing-connection"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newProviderAuditFixture(t)
			before := fixture.charges(t)
			configuration := fixture.configuration
			runID := "provider-recovery"
			var normalized io.Reader = strings.NewReader(fixture.normalized)
			var source io.Reader = strings.NewReader(fixture.source)
			switch scenario {
			case "run-id":
				runID = ""
			case "missing-evidence":
				normalized = nil
			case "missing-source":
				source = nil
			case "evidence-too-large":
				normalized = strings.NewReader(strings.Repeat(" ", paymentWebhookMaximumBytes+1))
			case "source-too-large":
				source = strings.NewReader(strings.Repeat("x", providerCostSourceMaximumBytes+1))
			case "empty-source":
				source = strings.NewReader("")
			case "malformed-json":
				normalized = strings.NewReader("{")
			case "multiple-json":
				normalized = strings.NewReader(fixture.normalized + " {}")
			case "invalid-discount", "invalid-fees", "missing-connection":
				input, _ := providerCostEvidenceFixture()
				if scenario == "missing-connection" {
					input["platform_connection_id"] = "absent-platform"
				} else {
					input[strings.TrimPrefix(scenario, "invalid-")] = ExactMoney{"1", "0"}
				}
				encoded, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				normalized = strings.NewReader(string(encoded))
			case "missing-database":
				configuration = ManagementConfiguration{}
			case "unavailable-database":
				configuration = ManagementConfiguration{DatabasePath: filepath.Join(t.TempDir(), "absent", "management.db")}
			case "missing-schema":
				configuration = ManagementConfiguration{DatabasePath: filepath.Join(t.TempDir(), "empty.db")}
			}
			if _, err := ReconcileProviderCosts(t.Context(), configuration, runID, normalized, source); err == nil {
				t.Fatal("invalid import accepted")
			}
			fixture.assertNoImport(t)
			fixture.assertRecovery(t, before)
		})
	}
}

func newProviderAuditFixture(t *testing.T) providerAuditFixture {
	t.Helper()
	database, intent, server, reserve := newHostedRatingFixture(t)
	_, observation := observeRatedFixture(t, database, intent("provider-audit"), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}})
	if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })); err != nil {
		t.Fatal(err)
	}
	input, source := providerCostEvidenceFixture()
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return providerAuditFixture{database, server, ManagementConfiguration{DatabaseDialector: database.database.Dialector}, string(encoded), source}
}

func (fixture providerAuditFixture) reconcile(t *testing.T, configuration ManagementConfiguration) (ProviderCostReconciliationReport, error) {
	t.Helper()
	return ReconcileProviderCosts(t.Context(), configuration, "provider-recovery", strings.NewReader(fixture.normalized), strings.NewReader(fixture.source))
}

func (fixture providerAuditFixture) charges(t *testing.T) any {
	t.Helper()
	return ratingHTTPExchange(t, fixture.server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
}

func (fixture providerAuditFixture) assertNoImport(t *testing.T) {
	t.Helper()
	for _, model := range []any{&managedProviderCostEvidenceRecord{}, &managedProviderReconciliationRunRecord{}} {
		var count int64
		if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("partial import %T: count=%d error=%v", model, count, err)
		}
	}
}

func (fixture providerAuditFixture) assertRecovery(t *testing.T, before any) {
	t.Helper()
	report, err := fixture.reconcile(t, fixture.configuration)
	if err != nil || report.AttemptCount != 1 || report.IncompleteAttempts != 0 || len(report.Differences) != 0 || report.KnownProviderCost != (ExactMoney{"7", "2500"}) {
		t.Fatalf("provider audit did not recover: report=%+v error=%v", report, err)
	}
	replayed, err := fixture.reconcile(t, fixture.configuration)
	if err != nil || !reflect.DeepEqual(report, replayed) || !reflect.DeepEqual(before, fixture.charges(t)) {
		t.Fatalf("provider audit replay changed report or charges: error=%v", err)
	}
	var retained managedProviderCostEvidenceRecord
	if err := fixture.database.database.First(&retained, "id = ?", report.EvidenceID).Error; err != nil || string(retained.Source) != fixture.source {
		t.Fatalf("source bytes not retained: error=%v", err)
	}
}

func TestHostedPaymentsProviderReconciliationReadFailuresRollBackImport(t *testing.T) {
	for _, table := range []string{"managed_platform_credential_records", "managed_platform_connection_records", "managed_provider_cost_evidence_records", "managed_provider_reconciliation_run_records", "attempts", "managed_journal_request_records", "managed_charge_records"} {
		t.Run(table, func(t *testing.T) {
			fixture := newProviderAuditFixture(t)
			before := fixture.charges(t)
			configuration := fixture.configuration
			var failures atomic.Int64
			configuration.DatabaseDialector = paymentAuditReadFailureDialector{Dialector: configuration.DatabaseDialector, table: table, failures: &failures}
			_, err := fixture.reconcile(t, configuration)
			if err == nil || !strings.Contains(err.Error(), "controlled_payment_audit_read_failure") || failures.Load() == 0 {
				t.Fatalf("read failure not reported: failures=%d error=%v", failures.Load(), err)
			}
			fixture.assertNoImport(t)
			if !reflect.DeepEqual(before, fixture.charges(t)) {
				t.Fatal("failed import changed charges")
			}
			fixture.assertRecovery(t, before)
		})
	}
}

func TestHostedPaymentsProviderReconciliationWriteFailuresRollBackImport(t *testing.T) {
	for _, statement := range []string{"BEFORE UPDATE ON managed_platform_connection_records", "BEFORE INSERT ON managed_provider_cost_evidence_records", "BEFORE INSERT ON managed_provider_reconciliation_run_records"} {
		t.Run(statement, func(t *testing.T) {
			fixture := newProviderAuditFixture(t)
			before := fixture.charges(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_provider_audit " + statement + " BEGIN SELECT RAISE(ABORT, 'controlled_provider_audit_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.reconcile(t, fixture.configuration); err == nil {
				t.Fatal("failed import acknowledged")
			}
			fixture.assertNoImport(t)
			if !reflect.DeepEqual(before, fixture.charges(t)) {
				t.Fatal("failed import changed charges")
			}
			if err := fixture.database.database.Exec("DROP TRIGGER reject_provider_audit").Error; err != nil {
				t.Fatal(err)
			}
			fixture.assertRecovery(t, before)
		})
	}
}

func TestHostedPaymentsProviderReconciliationRejectsCorruptRetainedEvidence(t *testing.T) {
	for _, scenario := range []struct{ name, table, column string }{
		{"source", "managed_provider_cost_evidence_records", "source"},
		{"scope", "managed_provider_cost_evidence_records", "input"},
		{"local-evidence", "managed_provider_reconciliation_run_records", "local_evidence"},
		{"report", "managed_provider_reconciliation_run_records", "report"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newProviderAuditFixture(t)
			before := fixture.charges(t)
			report, err := fixture.reconcile(t, fixture.configuration)
			if err != nil {
				t.Fatal(err)
			}
			var original string
			if err := fixture.database.database.Table(scenario.table).Select(scenario.column).Scan(&original).Error; err != nil {
				t.Fatal(err)
			}
			if err := fixture.database.database.Table(scenario.table).Where("1 = 1").UpdateColumn(scenario.column, "{").Error; err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.reconcile(t, fixture.configuration); err == nil {
				t.Fatal("corrupted import accepted")
			}
			if !reflect.DeepEqual(before, fixture.charges(t)) {
				t.Fatal("corrupted import changed charges")
			}
			if err := fixture.database.database.Table(scenario.table).Where("1 = 1").UpdateColumn(scenario.column, original).Error; err != nil {
				t.Fatal(err)
			}
			replayed, err := fixture.reconcile(t, fixture.configuration)
			if err != nil || !reflect.DeepEqual(report, replayed) {
				t.Fatalf("restored evidence changed report: error=%v", err)
			}
			fixture.assertRecovery(t, before)
		})
	}
}
