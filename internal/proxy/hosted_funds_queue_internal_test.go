package proxy

import (
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostedFundsQueueImageOutcomes(t *testing.T) {
	for _, mode := range []string{"measured", "zero", "missing", "above_bound", "submit_error", "result_error", "artifact_error"} {
		t.Run(mode, func(t *testing.T) {
			database, _, management, _ := newHostedRatingFixture(t)
			png := imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 16, 16)))
			var submits, results, downloads atomic.Int64
			var upstream *httptest.Server
			headers := func(writer http.ResponseWriter) {
				if mode == "missing" {
					return
				}
				units := "2.125"
				if mode == "zero" {
					units = "0"
				}
				if mode == "above_bound" {
					units = "20"
				}
				writer.Header().Set("X-Fal-Billable-Units", units)
			}
			upstream = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/artifact.png" {
					downloads.Add(1)
					if request.Header.Get("Authorization") != "" {
						t.Error("platform secret reached artifact")
					}
					if mode == "artifact_error" {
						writer.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					writer.Header().Set("Content-Type", "image/png")
					_, _ = writer.Write(png)
					return
				}
				if request.Header.Get("Authorization") != "Key hosted-queue-secret" {
					t.Error("queue lost accepted authority")
				}
				writer.Header().Set("Content-Type", "application/json")
				switch request.Method + " " + request.URL.Path {
				case "POST /reve/2.1/text-to-image":
					submits.Add(1)
					if mode == "submit_error" {
						headers(writer)
						writer.WriteHeader(http.StatusBadRequest)
						fmt.Fprint(writer, `{"error":"controlled rejection"}`)
						return
					}
					fmt.Fprintf(writer, `{"request_id":"financial-queue","status_url":%q,"response_url":%q,"cancel_url":%q}`, upstream.URL+"/reve/requests/financial-queue/status", upstream.URL+"/reve/requests/financial-queue", upstream.URL+"/reve/requests/financial-queue/cancel")
				case "GET /reve/requests/financial-queue/status":
					fmt.Fprint(writer, `{"request_id":"financial-queue","status":"COMPLETED"}`)
				case "GET /reve/requests/financial-queue":
					results.Add(1)
					headers(writer)
					if mode == "result_error" {
						writer.WriteHeader(http.StatusServiceUnavailable)
					}
					fmt.Fprintf(writer, `{"images":[{"url":%q}]}`, upstream.URL+"/artifact.png")
				default:
					t.Errorf("unexpected queue request %s %s", request.Method, request.URL.Path)
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(upstream.Close)
			server, worker, intent := newHostedQueueUsageFixture(t, database, upstream.URL)
			settings := hostedQueueFinancialSettings(t)
			worker.catalog = settings.catalog
			worker.hostedAdmission = settings.mediaAdmission(worker.providers)
			configureHostedQueueUsage(t, worker, upstream.URL)
			hostedSpeechHTTP(t, server, "unfunded-queue", intent, http.StatusPaymentRequired)
			if submits.Load() != 0 || downloads.Load() != 0 || results.Load() != 0 {
				t.Fatal("unfunded queue did provider work")
			}
			seedHostedFunds(t, database, 500)
			id := hostedSpeechHTTP(t, server, "funded-queue", intent, http.StatusAccepted)["operation_id"].(string)
			assertHostedFundsBalance(t, database, 500, 474)
			worker.runOperation("funded-queue", id)
			state := MediaOperationStateSucceeded
			if mode == "submit_error" {
				state = MediaOperationStateFailed
			}
			if mode == "result_error" || mode == "artifact_error" {
				state = MediaOperationStateUncertain
			}
			if result := hostedMediaWorkerStatus(t, server, id); result["state"] != state {
				t.Fatalf("queue outcome=%v", result)
			}
			for range 2 {
				if err := database.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
				worker.runOperation("duplicate-queue", id)
				if replay := hostedSpeechHTTP(t, server, "funded-queue", intent, http.StatusOK); replay["operation_id"] != id {
					t.Fatal("queue replay changed identity")
				}
			}
			if submits.Load() != 1 || results.Load() > 1 || downloads.Load() > 1 {
				t.Fatalf("duplicate queue work: submits=%d results=%d downloads=%d", submits.Load(), results.Load(), downloads.Load())
			}
			charges := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)["charges"].([]any)
			if len(charges) != 1 {
				t.Fatalf("queue charges=%v", charges)
			}
			charge := charges[0].(map[string]any)
			summary := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+charge["request_id"].(string)+"/charge-summary", "", http.StatusOK)
			balance := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
			var expectedCost any = map[string]any{"numerator": "17", "denominator": "400"}
			switch mode {
			case "measured":
				assertHostedFundsBalance(t, database, 495, 495)
				assertFundsCreditRemainder(t, database, "21", "4000")
				if charge["state"] != chargeRated || summary["state"] != string(requestChargeRated) || !reflect.DeepEqual(summary["customer_charge"], map[string]any{"numerator": "221", "denominator": "4000"}) {
					t.Fatalf("queue charge=%v", summary)
				}
			case "zero":
				expectedCost = map[string]any{"numerator": "0", "denominator": "1"}
				assertHostedFundsBalance(t, database, 500, 500)
				assertFundsCreditRemainder(t, database, "0", "1")
				if !reflect.DeepEqual(summary["customer_charge"], expectedCost) {
					t.Fatalf("zero queue=%v", summary)
				}
			default:
				assertHostedFundsBalance(t, database, 500, 474)
				if summary["state"] != string(requestChargeUnresolved) || summary["customer_charge"] != nil || balance["pending_cents"] != "26" {
					t.Fatalf("queue lost unresolved funds: summary=%v balance=%v", summary, balance)
				}
				if mode == "missing" {
					expectedCost = nil
					if charge["state"] != chargeUsageUnresolved {
						t.Fatalf("unknown queue charge=%v", charge)
					}
				}
				if mode == "above_bound" {
					expectedCost = map[string]any{"numerator": "2", "denominator": "5"}
					if charge["state"] != chargeLimitUnresolved {
						t.Fatalf("excess queue charge=%v", charge)
					}
				}
			}
			if !reflect.DeepEqual(summary["provider_cost"], expectedCost) {
				t.Fatalf("lost queue provider cost: %v", summary)
			}
			if mode == "above_bound" {
				reservation := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/reservations/"+charge["request_id"].(string), "", http.StatusOK)
				if !reflect.DeepEqual(reservation["known_platform_exposure"], map[string]any{"numerator": "7", "denominator": "50"}) {
					t.Fatalf("queue exposure=%v", reservation)
				}
			}
		})
	}
}

func hostedQueueFinancialSettings(t *testing.T) *hostedRuntimeSettings {
	t.Helper()
	catalog := internalCanonicalProviderCatalog().ModelCatalog()
	for index := range catalog.Offerings {
		offering := &catalog.Offerings[index]
		if offering.Provider == "fal" && offering.Model == "reve-2.1" {
			maximum := 10
			offering.Limits = append(offering.Limits, CatalogLimit{ID: "billable_units", Unit: "provider_units", Value: &maximum})
		}
	}
	for index := range catalog.Prices {
		price := &catalog.Prices[index]
		if price.Provider == "fal" && price.Model == "reve-2.1" && price.Operation == ModelOperationImageGeneration {
			*price = CatalogPriceDescriptor{Provider: price.Provider, Model: price.Model, Operation: price.Operation, Available: true, Source: "https://example.com/controlled-queue-rates", LastVerified: "2026-09-23", Rates: []CatalogPriceRate{{Component: "billable_units", Currency: "USD", Rate: "0.02", Unit: "USD/provider_unit", Conditions: CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}}}}
		}
	}
	settings, err := newHostedRuntimeSettings(&HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: "fal", Model: "reve-2.1", Operation: ModelOperationImageGeneration, MaximumAttempts: 1}}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return settings
}
