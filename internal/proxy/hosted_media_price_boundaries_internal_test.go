package proxy

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestHostedMediaPriceFailurePreservesFundsAndRequestIdentity(t *testing.T) {
	for _, scenario := range []string{"unavailable", "expired", "future", "missing-bound", "unmapped-condition"} {
		t.Run(scenario, func(t *testing.T) {
			database, _, management, _ := newHostedRatingFixture(t)
			seedHostedFunds(t, database, 500)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				writer.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(upstream.Close)
			server, worker := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
			catalog := hostedImageFinancialCatalog(ModelOperationImageGeneration)
			conditions := CatalogPriceConditions{Quality: "low", Resolution: "1024x1024"}
			for index := range catalog.Prices {
				price := &catalog.Prices[index]
				if price.Provider != "openai" || price.Model != "gpt-image-2" || price.Operation != ModelOperationImageGeneration {
					continue
				}
				switch scenario {
				case "unavailable":
					price.Available, price.Rates = false, nil
					price.UnavailableReason = "Controlled price qualification is incomplete."
				case "expired", "future":
					for rate := range price.Rates {
						if scenario == "expired" {
							price.Rates[rate].Conditions.EffectiveFrom = "2000-01-01T00:00:00Z"
							price.Rates[rate].Conditions.EffectiveUntil = "2001-01-01T00:00:00Z"
						} else {
							price.Rates[rate].Conditions.EffectiveFrom = "2099-01-01T00:00:00Z"
						}
					}
				}
			}
			if scenario == "missing-bound" {
				for index := range catalog.Offerings {
					offering := &catalog.Offerings[index]
					if offering.Provider == "openai" && offering.Model == "gpt-image-2" {
						for limit := range offering.Limits {
							if offering.Limits[limit].ID == "output_image_tokens" {
								offering.Limits = append(offering.Limits[:limit], offering.Limits[limit+1:]...)
								break
							}
						}
					}
				}
			}
			if scenario == "unmapped-condition" {
				conditions.Duration = "10"
			}
			settings, err := newHostedRuntimeSettings(&HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: "openai", Model: "gpt-image-2", Operation: ModelOperationImageGeneration, MaximumAttempts: 1, Conditions: conditions}}}, catalog)
			if err != nil {
				t.Fatal(err)
			}
			worker.hostedAdmission = settings.mediaAdmission(worker.providers)
			financial := fundsStartupFixture{database: database, management: management, calls: &calls}
			before := financial.state(t)
			for range 2 {
				hostedMediaAdmissionHTTP(t, server, "price-recovery", "funded image", http.StatusServiceUnavailable)
				if !reflect.DeepEqual(before, financial.state(t)) || calls.Load() != 0 || len(worker.queue) != 0 {
					t.Fatal("rejected price admission changed funds or queued provider work")
				}
				for _, model := range []any{&managedJournalRequestRecord{}, &managedPriceSnapshotRecord{}, &managedFundsReservationRecord{}, &mediaOperationRecord{}} {
					var count int64
					if err := database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
						t.Fatalf("rejected admission retained %T: count=%d error=%v", model, count, err)
					}
				}
			}
			worker.hostedAdmission = hostedImageFinancialSettings(t, ModelOperationImageGeneration).mediaAdmission(worker.providers)
			accepted := hostedMediaAdmissionHTTP(t, server, "price-recovery", "funded image", http.StatusAccepted)
			replayed := hostedMediaAdmissionHTTP(t, server, "price-recovery", "funded image", http.StatusOK)
			if accepted["operation_id"] != replayed["operation_id"] || len(worker.queue) != 1 || calls.Load() != 0 {
				t.Fatalf("restored price duplicated provider work: accepted=%v replay=%v queued=%d calls=%d", accepted, replayed, len(worker.queue), calls.Load())
			}
			assertHostedFundsBalance(t, database, 500, 461)
			assertFundsCreditRemainder(t, database, "0", "1")
		})
	}
}
