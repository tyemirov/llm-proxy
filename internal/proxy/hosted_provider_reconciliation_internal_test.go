package proxy

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func providerCostEvidenceFixture() (map[string]any, string) {
	source := "provider,period,cost\nopenai,2026-09-22,0.0028\n"
	return map[string]any{
		"source_reference": "invoice-fixture-2026-09-22", "source_sha256": sha256Hex(source),
		"provider": "openai", "platform_connection_id": "platform-journal", "credential_version": 1, "provider_account_reference": "provider-account-fixture",
		"period_start": "2026-09-22T00:00:00Z", "period_end": "2026-09-23T00:00:00Z", "reported_at": "2026-09-23T01:00:00Z",
		"currency": "USD", "usage_amount": ExactMoney{"7", "2500"}, "discount": ExactMoney{"0", "1"}, "fees": ExactMoney{"0", "1"}, "attempt_count": 1,
	}, source
}

func TestHostedPaymentsProviderReconciliationRetainsExactCostsWithoutChargeChanges(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	_, observation := observeRatedFixture(t, database, intent("provider-invoice"), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}})
	if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })); err != nil {
		t.Fatal(err)
	}
	before := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
	input, source := providerCostEvidenceFixture()
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	configuration := ManagementConfiguration{DatabaseDialector: database.database.Dialector}
	report, err := ReconcileProviderCosts(t.Context(), configuration, "provider-run", strings.NewReader(string(encoded)), strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	if report.AttemptCount != 1 || report.IncompleteAttempts != 0 || !reflect.DeepEqual(report.KnownProviderCost, ExactMoney{"7", "2500"}) || len(report.Differences) != 0 {
		t.Fatalf("report=%+v", report)
	}
	replay, err := ReconcileProviderCosts(t.Context(), configuration, "provider-run", strings.NewReader(string(encoded)), strings.NewReader(source))
	if err != nil || !reflect.DeepEqual(replay, report) {
		t.Fatalf("replay=%+v error=%v", replay, err)
	}
	input["usage_amount"] = ExactMoney{"1", "1"}
	encoded, _ = json.Marshal(input)
	if _, err := ReconcileProviderCosts(t.Context(), configuration, "provider-run", strings.NewReader(string(encoded)), strings.NewReader(source)); err == nil {
		t.Fatal("run accepted changed evidence")
	}
	after := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("provider invoice rewrote customer charges")
	}
}

func TestHostedPaymentsProviderReconciliationRejectsUnboundSource(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	configuration := ManagementConfiguration{DatabaseDialector: database.database.Dialector}
	for _, scenario := range []string{"digest", "provider", "credential", "unknown-field", "amount", "period"} {
		t.Run(scenario, func(t *testing.T) {
			input, source := providerCostEvidenceFixture()
			switch scenario {
			case "digest":
				source += "changed"
			case "provider":
				input["provider"] = "anthropic"
			case "credential":
				input["credential_version"] = 2
			case "unknown-field":
				input["override_customer_price"] = true
			case "amount":
				input["usage_amount"] = ExactMoney{"1", "0"}
			case "period":
				input["period_end"] = input["period_start"]
			}
			encoded, _ := json.Marshal(input)
			if _, err := ReconcileProviderCosts(t.Context(), configuration, "invalid-"+scenario, strings.NewReader(string(encoded)), strings.NewReader(source)); err == nil {
				t.Fatal("invalid evidence accepted")
			}
		})
	}
}

func TestHostedPaymentsProviderReconciliationSeparatesDifferencesAndRollsBack(t *testing.T) {
	for _, scenario := range []struct{ name, category string }{{"cost", "amount"}, {"currency", "currency"}, {"discount", "discount"}, {"fee", "fee"}, {"missing-rating", "missing_usage"}, {"count", "missing_usage"}, {"timing", "timing"}} {
		t.Run(scenario.name, func(t *testing.T) {
			database, intent, _, reserve := newHostedRatingFixture(t)
			_, observation := observeRatedFixture(t, database, intent("provider-difference"), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}})
			if scenario.name != "missing-rating" {
				if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })); err != nil {
					t.Fatal(err)
				}
			}
			input, source := providerCostEvidenceFixture()
			switch scenario.name {
			case "cost":
				input["usage_amount"] = ExactMoney{"3", "1000"}
			case "currency":
				input["currency"] = "EUR"
			case "discount":
				input["discount"] = ExactMoney{"1", "1000"}
			case "fee":
				input["fees"] = ExactMoney{"1", "1000"}
			case "count":
				input["attempt_count"] = 2
			case "timing":
				input["reported_at"] = "2026-09-22T20:00:00Z"
			}
			encoded, _ := json.Marshal(input)
			configuration := ManagementConfiguration{DatabaseDialector: database.database.Dialector}
			if err := database.database.Exec("CREATE TRIGGER reject_provider_report BEFORE INSERT ON managed_provider_reconciliation_run_records BEGIN SELECT RAISE(ABORT, 'controlled_provider_report_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			if _, err := ReconcileProviderCosts(t.Context(), configuration, "provider-difference", strings.NewReader(string(encoded)), strings.NewReader(source)); err == nil {
				t.Fatal("report write failure ignored")
			}
			var count int64
			if err := database.database.Model(&managedProviderCostEvidenceRecord{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("partial source count=%d error=%v", count, err)
			}
			if err := database.database.Exec("DROP TRIGGER reject_provider_report").Error; err != nil {
				t.Fatal(err)
			}
			report, err := ReconcileProviderCosts(t.Context(), configuration, "provider-difference", strings.NewReader(string(encoded)), strings.NewReader(source))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, difference := range report.Differences {
				if difference.Category == scenario.category {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing %s difference: %+v", scenario.category, report)
			}
		})
	}
}

func TestHostedPaymentsProviderReconciliationConcurrentRunsRetainOneReport(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	configuration := ManagementConfiguration{DatabaseDialector: database.database.Dialector}
	input, source := providerCostEvidenceFixture()
	input["usage_amount"] = ExactMoney{"0", "1"}
	input["attempt_count"] = 0
	encoded, _ := json.Marshal(input)
	type outcome struct {
		report ProviderCostReconciliationReport
		err    error
	}
	results := make(chan outcome, 8)
	start := make(chan struct{})
	for range 8 {
		go func() {
			<-start
			report, err := ReconcileProviderCosts(t.Context(), configuration, "concurrent-provider-run", strings.NewReader(string(encoded)), strings.NewReader(source))
			results <- outcome{report, err}
		}()
	}
	close(start)
	var expected *ProviderCostReconciliationReport
	for range 8 {
		result := <-results
		if result.err != nil {
			t.Errorf("concurrent provider run: %v", result.err)
			continue
		}
		if expected == nil {
			expected = &result.report
		} else if !reflect.DeepEqual(*expected, result.report) {
			t.Error("concurrent reports differ")
		}
	}
	var count int64
	if err := database.database.Model(&managedProviderReconciliationRunRecord{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("retained report count=%d error=%v", count, err)
	}
}

func TestHostedPaymentsProviderReconciliationScopesModelOperationAndPeriod(t *testing.T) {
	database, intent, _, reserve := newHostedRatingFixture(t)
	_, observation := observeRatedFixture(t, database, intent("provider-scope"), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}})
	if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })); err != nil {
		t.Fatal(err)
	}
	configuration := ManagementConfiguration{DatabaseDialector: database.database.Dialector}
	for _, scenario := range []string{"matching-route", "different-model", "different-operation", "end-exclusive"} {
		t.Run(scenario, func(t *testing.T) {
			input, source := providerCostEvidenceFixture()
			input["source_reference"] = "scope-" + scenario
			switch scenario {
			case "matching-route":
				input["model"] = "gpt-4.1"
				input["operation"] = "text"
			case "different-model":
				input["model"] = "different-model"
			case "different-operation":
				input["operation"] = "speech"
			case "end-exclusive":
				input["period_end"] = observation.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
			}
			expected := int64(0)
			if scenario == "matching-route" {
				expected = 1
			} else {
				input["usage_amount"] = ExactMoney{"0", "1"}
				input["attempt_count"] = 0
			}
			encoded, _ := json.Marshal(input)
			report, err := ReconcileProviderCosts(t.Context(), configuration, "scope-"+scenario, strings.NewReader(string(encoded)), strings.NewReader(source))
			if err != nil {
				t.Fatal(err)
			}
			if report.AttemptCount != expected {
				t.Fatalf("scope=%s attempts=%d want=%d", scenario, report.AttemptCount, expected)
			}
		})
	}
}
