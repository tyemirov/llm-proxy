package proxy

import (
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestHostedDictatorInputFinancialAcceptance(t *testing.T) {
	for _, scenario := range hostedDictatorInputScenarios {
		for _, mode := range []string{"success", "running", "absent", "zero_input", "missing_input", "invalid_rate", "fractional_rate", "over_limit", "failed", "cancelled", "invalid_state", "artifact_loss", "observation_failure", "delivery_failure"} {
			if mode == "absent" && scenario.capability != "audio.voice.extract" {
				continue
			}
			if mode == "artifact_loss" && scenario.capability != "audio.align" && scenario.capability != "subtitles.create" {
				continue
			}
			t.Run(scenario.name+"/"+mode, func(t *testing.T) {
				database, _, management, _ := newHostedRatingFixture(t)
				upstream := &hostedDictatorOperationsUpstream{mode: mode}
				server, worker, intent := newHostedDictatorInputFixture(t, database, ModelNameDictatorWhisperBase, upstream, scenario)
				settings := hostedDictatorInputFinancialSettings(t, scenario)
				worker.catalog = settings.catalog
				worker.hostedAdmission = settings.mediaAdmission(worker.providers)
				hostedSpeechHTTP(t, server, "unfunded-input", intent, http.StatusPaymentRequired)
				if upstream.submissions.Load() != 0 {
					t.Fatal("unfunded input operation dispatched")
				}
				seedHostedFunds(t, database, 500)
				id := hostedSpeechHTTP(t, server, "funded-input", intent, http.StatusAccepted)["operation_id"].(string)
				assertHostedFundsBalance(t, database, 500, 448)
				failureTable := ""
				switch mode {
				case "observation_failure":
					failureTable = "managed_journal_observation_records"
				case "delivery_failure":
					failureTable = "managed_journal_delivery_records"
				}
				if failureTable != "" {
					if err := database.database.Exec("CREATE TRIGGER reject_input_financial BEFORE INSERT ON " + failureTable + " BEGIN SELECT RAISE(ABORT, 'controlled input financial failure'); END").Error; err != nil {
						t.Fatal(err)
					}
				}
				worker.runOperation("funded-input-worker", id)
				result := hostedMediaWorkerStatus(t, server, id)
				wantState := MediaOperationStateSucceeded
				switch mode {
				case "failed":
					wantState = MediaOperationStateFailed
				case "cancelled":
					wantState = MediaOperationStateCancelled
				case "artifact_loss", "observation_failure", "delivery_failure", "invalid_state":
					wantState = MediaOperationStateUncertain
				}
				if result["state"] != wantState {
					t.Fatalf("input financial operation=%v", result)
				}
				if failureTable != "" {
					if upstream.downloads.Load() != 0 {
						t.Fatal("artifact transfer preceded durable input evidence")
					}
					if err := database.database.Exec("DROP TRIGGER reject_input_financial").Error; err != nil {
						t.Fatal(err)
					}
				}
				downloads := upstream.downloads.Load()
				if replay := hostedSpeechHTTP(t, server, "funded-input", intent, http.StatusOK); replay["operation_id"] != id {
					t.Fatalf("input replay changed operation: %v", replay)
				}
				worker.runOperation("replayed-input-worker", id)
				if upstream.submissions.Load() != 1 || upstream.downloads.Load() != downloads {
					t.Fatal("input replay repeated provider work")
				}
				var previous map[string]any
				for range 2 {
					restarted := openJournalTransactionInstance(t, database)
					if err := restarted.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
						t.Fatal(err)
					}
					balance := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
					if previous != nil && !reflect.DeepEqual(balance, previous) {
						t.Fatalf("input settlement changed after restart: %v -> %v", previous, balance)
					}
					previous = balance
				}
				charges := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)["charges"].([]any)
				if failureTable != "" || mode == "invalid_state" {
					if len(charges) != 0 || previous["pending_cents"] != "52" || upstream.downloads.Load() != 0 {
						t.Fatalf("failed input evidence settled funds: charges=%v balance=%v", charges, previous)
					}
					assertHostedFundsBalance(t, database, 500, 448)
					return
				}
				if len(charges) != 1 {
					t.Fatalf("input charges=%v", charges)
				}
				charge := charges[0].(map[string]any)
				summary := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+charge["request_id"].(string)+"/charge-summary", "", http.StatusOK)
				var providerCost any = map[string]any{"numerator": "32001", "denominator": "400000"}
				switch mode {
				case "success", "running", "absent":
					assertHostedFundsBalance(t, database, 490, 490)
					assertFundsCreditRemainder(t, database, "16013", "4000000")
					if summary["state"] != string(requestChargeRated) || !reflect.DeepEqual(summary["customer_charge"], map[string]any{"numerator": "416013", "denominator": "4000000"}) {
						t.Fatalf("input settlement=%v", summary)
					}
				case "zero_input":
					providerCost = map[string]any{"numerator": "0", "denominator": "1"}
					assertHostedFundsBalance(t, database, 500, 500)
					assertFundsCreditRemainder(t, database, "0", "1")
					if summary["state"] != string(requestChargeRated) || !reflect.DeepEqual(summary["customer_charge"], providerCost) {
						t.Fatalf("measured zero input was not settled: %v", summary)
					}
				default:
					assertHostedFundsBalance(t, database, 500, 448)
					assertFundsCreditRemainder(t, database, "0", "1")
					if summary["state"] != string(requestChargeUnresolved) || summary["customer_charge"] != nil || previous["pending_cents"] != "52" {
						t.Fatalf("unresolved input released funds: summary=%v balance=%v", summary, previous)
					}
					switch mode {
					case "missing_input", "invalid_rate", "fractional_rate":
						providerCost = nil
						if charge["state"] != chargeUsageUnresolved {
							t.Fatalf("unknown input charge=%v", charge)
						}
					case "over_limit":
						providerCost = map[string]any{"numerator": "160001", "denominator": "400000"}
						if charge["state"] != chargeLimitUnresolved {
							t.Fatalf("excess input charge=%v", charge)
						}
					}
				}
				if !reflect.DeepEqual(summary["provider_cost"], providerCost) {
					t.Fatalf("input provider cost=%v", summary)
				}
			})
		}
	}
}

func hostedDictatorInputFinancialSettings(t *testing.T, scenario hostedDictatorInputScenario) *hostedRuntimeSettings {
	t.Helper()
	catalog := internalCanonicalProviderCatalog().ModelCatalog()
	model := ModelNameDictatorWhisperBase
	for index := range catalog.Offerings {
		offering := &catalog.Offerings[index]
		if offering.Provider == ProviderNameDictator && offering.Model == model {
			maximum := 10
			offering.Limits = append(offering.Limits, CatalogLimit{ID: "input_audio_seconds", Unit: "seconds", Value: &maximum})
		}
	}
	var rate CatalogDecimal = "0.04"
	unit := "USD/second"
	switch scenario.name {
	case "diarize", "subtitles-alignment":
		rate, unit = "2.4", "USD/minute"
	case "align", "subtitles-transcription":
		rate, unit = "144", "USD/hour"
	}
	for index := range catalog.Prices {
		price := &catalog.Prices[index]
		if price.Provider == ProviderNameDictator && price.Model == model && price.Operation == scenario.operation {
			*price = CatalogPriceDescriptor{Provider: price.Provider, Model: price.Model, Operation: price.Operation, Available: true, Source: "https://example.com/controlled-input-prices", LastVerified: "2026-09-25", Rates: []CatalogPriceRate{{Component: "input_audio", Currency: "USD", Rate: rate, Unit: unit, Conditions: CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}}}}
		}
	}
	settings, err := newHostedRuntimeSettings(&HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: ProviderNameDictator, Model: model, Operation: scenario.operation, MaximumAttempts: 1}}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return settings
}
