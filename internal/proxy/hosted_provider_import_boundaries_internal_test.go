package proxy

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const financialUTCOverflowTimestamp = "9999-12-31T23:59:59-01:00"

func TestHostedPaymentsProviderImportRouteFailuresPreserveEvidence(t *testing.T) {
	for _, field := range []string{"model", "operation"} {
		for _, value := range []string{strings.Repeat("a", 257), " leading", "trailing ", "embedded\x00value", "embedded\rvalue", "embedded\nvalue"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				fixture := newProviderAuditFixture(t)
				input, _ := providerCostEvidenceFixture()
				input[field] = value
				assertRejectedProviderImport(t, fixture, input, "invalid provider evidence route")
			})
		}
	}
}

func TestHostedPaymentsProviderImportUTCRangeFailurePreservesEvidence(t *testing.T) {
	for _, field := range []string{"period_end", "reported_at"} {
		t.Run(field, func(t *testing.T) {
			fixture := newProviderAuditFixture(t)
			input, _ := providerCostEvidenceFixture()
			// RFC 3339 permits this local year. Its UTC year is outside the JSON
			// timestamp range, so the imported evidence cannot be retained.
			input[field] = financialUTCOverflowTimestamp
			assertRejectedProviderImport(t, fixture, input, "year outside of range")
		})
	}
}

func assertRejectedProviderImport(t *testing.T, fixture providerAuditFixture, input map[string]any, reason string) {
	t.Helper()
	before := fixture.charges(t)
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	report, err := ReconcileProviderCosts(t.Context(), fixture.configuration, "provider-recovery", strings.NewReader(string(encoded)), strings.NewReader(fixture.source))
	if err == nil || !strings.Contains(err.Error(), reason) || !reflect.DeepEqual(report, ProviderCostReconciliationReport{}) {
		t.Fatalf("invalid import report=%+v error=%v want=%s", report, err, reason)
	}
	fixture.assertNoImport(t)
	if !reflect.DeepEqual(before, fixture.charges(t)) {
		t.Fatal("invalid provider import changed charges")
	}
	fixture.assertRecovery(t, before)
}

func TestHostedPaymentsProviderImportRunIdentityCannotChangeSource(t *testing.T) {
	fixture := newProviderAuditFixture(t)
	before := fixture.charges(t)
	original, err := fixture.reconcile(t, fixture.configuration)
	if err != nil {
		t.Fatal(err)
	}
	var sourceBefore managedProviderCostEvidenceRecord
	var runBefore managedProviderReconciliationRunRecord
	if err := fixture.database.database.First(&sourceBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.database.First(&runBefore).Error; err != nil {
		t.Fatal(err)
	}
	input, _ := providerCostEvidenceFixture()
	input["source_reference"] = "another-provider-source"
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		report, err := ReconcileProviderCosts(t.Context(), fixture.configuration, "provider-recovery", strings.NewReader(string(encoded)), strings.NewReader(fixture.source))
		if err == nil || !strings.Contains(err.Error(), "provider reconciliation run identity conflict") || !reflect.DeepEqual(report, ProviderCostReconciliationReport{}) {
			t.Fatalf("source change report=%+v error=%v", report, err)
		}
		var sources []managedProviderCostEvidenceRecord
		var runs []managedProviderReconciliationRunRecord
		if err := fixture.database.database.Find(&sources).Error; err != nil {
			t.Fatal(err)
		}
		if err := fixture.database.database.Find(&runs).Error; err != nil {
			t.Fatal(err)
		}
		if len(sources) != 1 || len(runs) != 1 || !reflect.DeepEqual(sourceBefore, sources[0]) || !reflect.DeepEqual(runBefore, runs[0]) || !reflect.DeepEqual(before, fixture.charges(t)) {
			t.Fatal("rejected source change altered retained evidence, reports, or charges")
		}
	}
	replayed, err := fixture.reconcile(t, fixture.configuration)
	if err != nil || !reflect.DeepEqual(original, replayed) {
		t.Fatalf("source conflict changed original replay: report=%+v error=%v", replayed, err)
	}
	fixture.assertRecovery(t, before)
}
