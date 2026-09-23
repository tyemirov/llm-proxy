package proxy

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func hostedSignalsDatabasePath(t *testing.T, database *gormManagedTenantDatabase) string {
	t.Helper()
	var row struct{ File string }
	if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row.File
}

func readHostedSignalsFixture(t *testing.T, database *gormManagedTenantDatabase) HostedFinancialSignals {
	t.Helper()
	report, err := ReadHostedFinancialSignals(t.Context(), hostedSignalsDatabasePath(t, database))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"billing-journal", "private-provider-request", "platform-journal", "hosted-service-secret"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("private identity in signals: %s", encoded)
		}
	}
	return report
}

func TestHostedSignalsRetainPendingWorkAndExactBalancesWithoutWrites(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 500)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	hostedIdentityHTTP(t, server, "signals-funded", "funded prompt", http.StatusOK)
	report := readHostedSignalsFixture(t, database)
	if report.PendingUsageDeliveries.Count != 1 || report.PendingUsageDeliveries.OldestAt == nil || report.UnsettledCompletedRequests.Count != 1 || report.UnsettledCompletedRequests.OldestAt == nil || report.PostedCents != "500" || report.ReservedCents != "3" {
		t.Fatalf("pending work hidden: %+v", report)
	}
	again := readHostedSignalsFixture(t, database)
	again.ObservedAt = report.ObservedAt
	if !reflect.DeepEqual(again, report) {
		t.Fatalf("read changed financial state: %+v != %+v", again, report)
	}
	if err := deliverFundsFixture(t, database); err != nil {
		t.Fatal(err)
	}
	settled := readHostedSignalsFixture(t, database)
	if settled.PendingUsageDeliveries.Count != 0 || settled.PendingUsageDeliveries.OldestAt != nil || settled.UnsettledCompletedRequests.Count != 0 || settled.ReservedCents != "0" || settled.PostedCents != "500" || settled.UnsettledFraction != (ExactMoney{Numerator: "91", Denominator: "25000"}) {
		t.Fatalf("settlement signals: %+v", settled)
	}
	// Inject retained uncertainty at the database boundary for the operational read.
	if err := database.database.Model(&managedJournalAttemptRecord{}).Where("1 = 1").Update("state", journalAttemptUncertain).Error; err != nil {
		t.Fatal(err)
	}
	uncertain := readHostedSignalsFixture(t, database)
	if uncertain.UnresolvedAttempts.Count != 1 || uncertain.UnresolvedAttempts.OldestAt == nil {
		t.Fatalf("uncertainty hidden: %+v", uncertain)
	}
	if err := database.database.Model(&managedJournalAttemptRecord{}).Where("1 = 1").Update("state", journalAttemptDispatched).Error; err != nil {
		t.Fatal(err)
	}
	active := readHostedSignalsFixture(t, database)
	if active.ActiveAttempts.Count != 1 || active.ActiveAttempts.OldestAt == nil || active.UnresolvedAttempts.Count != 0 {
		t.Fatalf("active work counted as unresolved: %+v", active)
	}
	if calls.Load() != 1 {
		t.Fatalf("signals dispatched work: %d", calls.Load())
	}
}

func TestHostedSignalsUseLatestPaymentComparison(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "signals-order", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	transaction := completedPaymentFixture(t, processor)
	management, payments := paymentReconciliationConfiguration(t, database, processor)
	if _, err := ReconcilePayments(t.Context(), management, payments, "signals-before"); err != nil {
		t.Fatal(err)
	}
	before := readHostedSignalsFixture(t, database)
	if before.PaymentDifferences["receipt_missing"] != 1 {
		t.Fatalf("payment difference absent: %+v", before)
	}
	sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
	if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcilePayments(t.Context(), management, payments, "signals-after"); err != nil {
		t.Fatal(err)
	}
	after := readHostedSignalsFixture(t, database)
	if len(after.PaymentDifferences) != 0 || after.PostedCents != "500" {
		t.Fatalf("obsolete comparison remained current: %+v", after)
	}
	if after.LatestPaymentComparisons.Count != 1 || after.LatestPaymentComparisons.OldestAt == nil {
		t.Fatalf("comparison coverage absent: %+v", after)
	}
	if err := database.database.Model(&managedPaymentReconciliationRunRecord{}).Where("id = ?", "signals-after").Updates(map[string]any{"state": paymentReconciliationPending, "completed_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	checkpoint := readHostedSignalsFixture(t, database)
	if checkpoint.PendingPaymentComparisons.Count != 1 || checkpoint.PendingPaymentComparisons.OldestAt == nil {
		t.Fatalf("incomplete comparison run hidden: %+v", checkpoint)
	}
	if err := database.database.Model(&managedPaymentReconciliationItemRecord{}).Where("run_id = ?", "signals-after").Update("result", "{").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ReadHostedFinancialSignals(t.Context(), hostedSignalsDatabasePath(t, database)); err == nil {
		t.Fatal("malformed comparison produced a healthy report")
	}
}

func TestHostedSignalsDoNotCreateMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sqlite")
	if _, err := ReadHostedFinancialSignals(t.Context(), path); err == nil {
		t.Fatal("missing database produced a report")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("read created missing database: %v", err)
	}
}

func TestHostedSignalsUseLatestProviderComparisonAndRetainExposure(t *testing.T) {
	database, intent, _, reserve := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 500)
	request, observation := observeRatedFixture(t, database, intent("signals-provider"), newHostedFundsAdmission(reserve), journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "20000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}})
	if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.ObservedAt, deliverHostedFunds); err != nil {
		t.Fatal(err)
	}
	input, source := providerCostEvidenceFixture()
	compare := func(id string) {
		t.Helper()
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ReconcileProviderCosts(t.Context(), ManagementConfiguration{DatabaseDialector: database.database.Dialector}, id, strings.NewReader(string(encoded)), strings.NewReader(source)); err != nil {
			t.Fatal(err)
		}
	}
	compare("signals-provider-before")
	before := readHostedSignalsFixture(t, database)
	if len(before.ProviderDifferences) == 0 || before.LatestProviderComparisons.Count != 1 || before.OpenReconciliationCases.Count == 0 || before.KnownPlatformExposure != (ExactMoney{Numerator: "37", Denominator: "2500"}) || before.ReconciliationHeldCents != "3" {
		t.Fatalf("provider discrepancy or exposure hidden: %+v", before)
	}
	input["usage_amount"] = ExactMoney{Numerator: "51", Denominator: "1250"}
	source = "provider,period,cost\nopenai,2026-09-22,0.0408\n"
	input["source_sha256"] = sha256Hex(source)
	input["source_reference"] = "corrected-invoice-fixture-2026-09-22"
	compare("signals-provider-after")
	after := readHostedSignalsFixture(t, database)
	if len(after.ProviderDifferences) != 0 || after.LatestProviderComparisons.Count != 1 || after.KnownPlatformExposure != before.KnownPlatformExposure {
		t.Fatalf("latest scope or retained exposure changed: %+v", after)
	}
	if err := database.database.Model(&managedFundsExposureRecord{}).Where("request_id = ?", request.ID).Update("provider_cost_complete", false).Error; err != nil {
		t.Fatal(err)
	}
	incomplete := readHostedSignalsFixture(t, database)
	if incomplete.IncompleteProviderCosts != 1 || incomplete.KnownPlatformExposure != before.KnownPlatformExposure {
		t.Fatalf("unknown cost treated as complete: %+v", incomplete)
	}
	if err := database.database.Model(&managedProviderReconciliationRunRecord{}).Where("id = ?", "signals-provider-after").Update("report", "{").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ReadHostedFinancialSignals(t.Context(), hostedSignalsDatabasePath(t, database)); err == nil {
		t.Fatal("malformed provider evidence produced a healthy report")
	}
}
