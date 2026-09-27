package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostedFundsSpeechCatalogRetainsUnresolvedOutcomes(t *testing.T) {
	for _, offering := range internalCanonicalProviderCatalog().ModelCatalog().Offerings {
		if offering.WireContract != CatalogProtocolElevenLabsSpeech && offering.WireContract != CatalogProtocolElevenLabsConversion {
			continue
		}
		for _, scenario := range []struct {
			name                                     string
			values                                   []string
			status                                   int
			contentType, operationState, chargeState string
			providerCost                             any
		}{
			{"zero", []string{"0"}, 200, "audio/mpeg", MediaOperationStateSucceeded, chargeRated, map[string]any{"numerator": "0", "denominator": "1"}},
			{"missing", nil, 200, "audio/mpeg", MediaOperationStateSucceeded, chargeUsageUnresolved, nil},
			{"negative", []string{"-1"}, 200, "audio/mpeg", MediaOperationStateSucceeded, chargeUsageUnresolved, nil},
			{"invalid", []string{"private-header"}, 200, "audio/mpeg", MediaOperationStateSucceeded, chargeUsageUnresolved, nil},
			{"duplicate", []string{"1", "2"}, 200, "audio/mpeg", MediaOperationStateSucceeded, chargeUsageUnresolved, nil},
			{"above_bound", []string{"200"}, 200, "audio/mpeg", MediaOperationStateSucceeded, chargeLimitUnresolved, map[string]any{"numerator": "4", "denominator": "5"}},
			{"provider_failure", []string{"12.5"}, 400, "application/json", MediaOperationStateFailed, chargeRated, map[string]any{"numerator": "1", "denominator": "20"}},
			{"provider_unavailable", []string{"12.5"}, 503, "application/json", MediaOperationStateUncertain, chargeRated, map[string]any{"numerator": "1", "denominator": "20"}},
			{"lost_result", []string{"12.5"}, 200, "application/json", MediaOperationStateUncertain, chargeRated, map[string]any{"numerator": "1", "denominator": "20"}},
		} {
			t.Run(offering.Provider+"/"+offering.Model+"/"+scenario.name, func(t *testing.T) {
				database, _, management, _ := newHostedRatingFixture(t)
				var calls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					calls.Add(1)
					for _, value := range scenario.values {
						writer.Header().Add(speechCharacterCostHeader, value)
					}
					writer.Header().Set("Content-Type", scenario.contentType)
					writer.WriteHeader(scenario.status)
					fmt.Fprint(writer, "controlled audio")
				}))
				t.Cleanup(upstream.Close)
				operation := ModelOperationSpeechGeneration
				if offering.WireContract == CatalogProtocolElevenLabsConversion {
					operation = ModelOperationSpeechConversion
				}
				server, worker, intent := newHostedSpeechUsageFixture(t, database, offering.Provider, offering.Model, upstream.URL, hostedSpeechFundsConfiguration(t, offering, operation))
				seedHostedFunds(t, database, 500)
				accepted := hostedSpeechHTTP(t, server, "financial-outcome", intent, http.StatusAccepted)
				id := accepted["operation_id"].(string)
				assertHostedFundsBalance(t, database, 500, 448)
				worker.runOperation("financial-outcome", id)
				if result := hostedMediaWorkerStatus(t, server, id); result["state"] != scenario.operationState {
					t.Fatalf("operation=%v", result)
				}
				// Repeat both delivery and execution after the terminal outcome. Neither
				// may submit another request or create another financial effect.
				for range 2 {
					if err := database.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
						t.Fatal(err)
					}
					worker.runOperation("duplicate-outcome", id)
					if replay := hostedSpeechHTTP(t, server, "financial-outcome", intent, http.StatusOK); replay["operation_id"] != id {
						t.Fatal("replay changed operation identity")
					}
				}
				charges := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)["charges"].([]any)
				if len(charges) != 1 || calls.Load() != 1 {
					t.Fatalf("charges=%d calls=%d", len(charges), calls.Load())
				}
				charge := charges[0].(map[string]any)
				if charge["state"] != scenario.chargeState || !reflect.DeepEqual(charge["rating"].(map[string]any)["provider_cost"], scenario.providerCost) {
					t.Fatalf("charge=%v", charge)
				}
				requestPath := "/billing-accounts/billing-journal/requests/" + charge["request_id"].(string)
				summary := ratingHTTPExchange(t, management, http.MethodGet, requestPath+"/charge-summary", "", http.StatusOK)
				if !reflect.DeepEqual(summary["provider_cost"], scenario.providerCost) {
					t.Fatalf("summary lost provider evidence: %v", summary)
				}
				balance := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
				if balance["posted_cents"] != "500" || balance["spent_cents"] != "0" {
					t.Fatalf("unexpected debit: %v", balance)
				}
				if scenario.name == "zero" {
					if summary["state"] != string(requestChargeRated) || !reflect.DeepEqual(summary["customer_charge"], scenario.providerCost) {
						t.Fatalf("zero summary=%v", summary)
					}
					if balance["available_cents"] != "500" || balance["reserved_cents"] != "0" || balance["pending_cents"] != "0" {
						t.Fatalf("zero hold not released: %v", balance)
					}
				} else {
					if summary["state"] != string(requestChargeUnresolved) || summary["customer_charge"] != nil || summary["net_customer_charge"] != nil {
						t.Fatalf("unresolved final charge=%v", summary)
					}
					if balance["available_cents"] != "448" || balance["reserved_cents"] != "52" || balance["pending_cents"] != "52" {
						t.Fatalf("unresolved hold lost: %v", balance)
					}
					cases := ratingHTTPExchange(t, management, http.MethodGet, requestPath+"/reconciliation-cases", "", http.StatusOK)
					if len(cases["cases"].([]any)) == 0 {
						t.Fatal("unresolved outcome has no reconciliation case")
					}
				}
				reservation := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/reservations/"+charge["request_id"].(string), "", http.StatusOK)
				knownCost := scenario.providerCost
				if knownCost == nil {
					knownCost = map[string]any{"numerator": "0", "denominator": "1"}
				}
				exposure := map[string]any{"numerator": "0", "denominator": "1"}
				if scenario.name == "above_bound" {
					exposure = map[string]any{"numerator": "7", "denominator": "25"}
				}
				if reservation["provider_cost_complete"] != (scenario.providerCost != nil) || !reflect.DeepEqual(reservation["known_provider_cost"], knownCost) || !reflect.DeepEqual(reservation["known_platform_exposure"], exposure) {
					t.Fatalf("lost financial exposure: %v", reservation)
				}
				assertFundsCreditRemainder(t, database, "0", "1")
			})
		}
	}
}
