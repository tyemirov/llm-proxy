package proxy

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestHostedSignalsRejectCorruptRetainedAmountsWithoutPartialReport(t *testing.T) {
	for _, source := range []string{"remainder", "exposure"} {
		t.Run(source, func(t *testing.T) {
			usage := startupCompleteUsage
			if source == "exposure" {
				usage = `{"input_tokens":20000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`
			}
			fixture := newFundsStartupFixture(t, usage)
			fixture.restart(t)
			before := fixture.state(t)
			originalReport := readHostedSignalsFixture(t, fixture.database)
			table, numerator, denominator := "managed_funds_account_records", "remainder_numerator", "remainder_denominator"
			if source == "exposure" {
				table, numerator, denominator = "managed_funds_exposure_records", "excess_numerator", "excess_denominator"
			}
			var original struct{ Numerator, Denominator string }
			if err := fixture.database.database.Table(table).Select(numerator + " AS numerator, " + denominator + " AS denominator").Take(&original).Error; err != nil {
				t.Fatal(err)
			}
			for _, scenario := range []struct{ name, numerator, denominator string }{
				{"invalid-numerator", "invalid", "1000"},
				{"negative-numerator", "-1", "1000"},
				{"zero-denominator", "1", "0"},
				{"invalid-denominator", "1", "invalid"},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					write := func(amountNumerator, amountDenominator string) {
						t.Helper()
						result := fixture.database.database.Table(table).Where("billing_account_id = ?", "billing-journal").Updates(map[string]any{numerator: amountNumerator, denominator: amountDenominator})
						if result.Error != nil || result.RowsAffected != 1 {
							t.Fatalf("signal source update rows=%d error=%v", result.RowsAffected, result.Error)
						}
					}
					write(scenario.numerator, scenario.denominator)
					t.Cleanup(func() { write(original.Numerator, original.Denominator) })
					report, err := ReadHostedFinancialSignals(t.Context(), hostedSignalsDatabasePath(t, fixture.database))
					if err == nil || !reflect.DeepEqual(report, HostedFinancialSignals{}) {
						t.Fatalf("corrupt financial source returned a partial report: %+v error=%v", report, err)
					}
					var retained struct{ Numerator, Denominator string }
					if err := fixture.database.database.Table(table).Select(numerator + " AS numerator, " + denominator + " AS denominator").Take(&retained).Error; err != nil {
						t.Fatal(err)
					}
					if retained.Numerator != scenario.numerator || retained.Denominator != scenario.denominator {
						t.Fatal("signal reader rewrote corrupt retained evidence")
					}
				})
				report := readHostedSignalsFixture(t, fixture.database)
				report.ObservedAt = originalReport.ObservedAt
				if !reflect.DeepEqual(report, originalReport) || !reflect.DeepEqual(before, fixture.state(t)) || fixture.calls.Load() != 1 {
					t.Fatal("signal recovery changed financial resources or repeated provider work")
				}
			}
		})
	}
}

func TestHostedSignalsRejectInconsistentComparisonEvidence(t *testing.T) {
	for _, source := range []string{"payment", "provider"} {
		t.Run(source, func(t *testing.T) {
			var database *gormManagedTenantDatabase
			var assertState func(*testing.T)
			table, keyColumn, key := "managed_payment_reconciliation_item_records", "run_id", "signals-integrity"
			resultColumn, evidenceColumn := "result", "evidence"
			identityField, timestampField, digestField := "order_id", "observed_at", "evidence_digest"
			if source == "payment" {
				fixture := newPaymentAuditFixture(t)
				if _, err := ReconcilePayments(t.Context(), fixture.management, fixture.payments, key); err != nil {
					t.Fatal(err)
				}
				database = fixture.database
				before := fixture.balance(t)
				assertState = func(t *testing.T) {
					t.Helper()
					if !reflect.DeepEqual(before, fixture.balance(t)) {
						t.Fatal("signal read changed customer funds")
					}
				}
			} else {
				fixture := newProviderAuditFixture(t)
				if _, err := fixture.reconcile(t, fixture.configuration); err != nil {
					t.Fatal(err)
				}
				database = fixture.database
				table, keyColumn, key = "managed_provider_reconciliation_run_records", "id", "provider-recovery"
				resultColumn, evidenceColumn = "report", "local_evidence"
				identityField, timestampField, digestField = "run_id", "created_at", "local_evidence_digest"
				before := fixture.charges(t)
				assertState = func(t *testing.T) {
					t.Helper()
					if !reflect.DeepEqual(before, fixture.charges(t)) {
						t.Fatal("signal read changed customer charges")
					}
				}
			}
			var original struct{ Document, Evidence string }
			if err := database.database.Table(table).Where(keyColumn+" = ?", key).Select(resultColumn + " AS document, " + evidenceColumn + " AS evidence").Take(&original).Error; err != nil {
				t.Fatal(err)
			}
			originalReport := readHostedSignalsFixture(t, database)
			for _, field := range []string{identityField, timestampField, digestField, "raw-evidence"} {
				t.Run(field, func(t *testing.T) {
					var document map[string]any
					if err := json.Unmarshal([]byte(original.Document), &document); err != nil {
						t.Fatal(err)
					}
					evidence := original.Evidence
					if field == "raw-evidence" {
						evidence += " "
					} else if field == timestampField {
						document[field] = "2020-01-01T00:00:00Z"
					} else {
						document[field] = "inconsistent-evidence"
					}
					encoded, err := json.Marshal(document)
					if err != nil {
						t.Fatal(err)
					}
					write := func(document, evidence string) {
						t.Helper()
						result := database.database.Table(table).Where(keyColumn+" = ?", key).Updates(map[string]any{resultColumn: document, evidenceColumn: evidence})
						if result.Error != nil || result.RowsAffected != 1 {
							t.Fatalf("comparison mutation rows=%d error=%v", result.RowsAffected, result.Error)
						}
					}
					write(string(encoded), evidence)
					t.Cleanup(func() { write(original.Document, original.Evidence) })
					report, err := ReadHostedFinancialSignals(t.Context(), hostedSignalsDatabasePath(t, database))
					if err == nil || !reflect.DeepEqual(report, HostedFinancialSignals{}) {
						t.Fatalf("inconsistent evidence produced a partial report: %+v error=%v", report, err)
					}
					assertState(t)
				})
				restored := readHostedSignalsFixture(t, database)
				restored.ObservedAt = originalReport.ObservedAt
				if !reflect.DeepEqual(originalReport, restored) {
					t.Fatal("restored evidence changed the report")
				}
				assertState(t)
			}
		})
	}
}

func TestHostedSignalsRejectUnreadableOldestWorkWithoutDelivery(t *testing.T) {
	fixture := newFundsStartupFixture(t, startupCompleteUsage)
	before := fixture.state(t)
	original := readHostedSignalsFixture(t, fixture.database)
	if original.PendingUsageDeliveries.Count != 1 || original.PendingUsageDeliveries.OldestAt == nil {
		t.Fatalf("pending delivery absent: %+v", original)
	}
	if err := fixture.database.database.Exec("ALTER TABLE managed_journal_delivery_records RENAME COLUMN created_at TO unavailable_created_at").Error; err != nil {
		t.Fatal(err)
	}
	report, err := ReadHostedFinancialSignals(t.Context(), hostedSignalsDatabasePath(t, fixture.database))
	if err == nil || !reflect.DeepEqual(report, HostedFinancialSignals{}) {
		t.Fatalf("unreadable queue returned a partial report: %+v error=%v", report, err)
	}
	if err := fixture.database.database.Exec("ALTER TABLE managed_journal_delivery_records RENAME COLUMN unavailable_created_at TO created_at").Error; err != nil {
		t.Fatal(err)
	}
	restored := readHostedSignalsFixture(t, fixture.database)
	restored.ObservedAt = original.ObservedAt
	if !reflect.DeepEqual(original, restored) {
		t.Fatal("signal read changed the pending queue")
	}
	fixture.assertPending(t, before)
	fixture.restart(t)
	fixture.assertSettledOnce(t, before)
}
