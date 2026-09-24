package proxy_test

import (
	"errors"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
)

func TestCatalogRatingProviderServicesShareExactPriceContract(t *testing.T) {
	for _, scenario := range []struct {
		operation, component, rate, unit, dimension, quantityUnit, quantity string
		providerCost, customerCharge                                        proxy.ExactMoney
	}{
		{proxy.ModelOperationAudioAlignment, "input_audio", "4", "USD/hour", "audio_seconds", "second", "1.25", proxy.ExactMoney{Numerator: "1", Denominator: "720"}, proxy.ExactMoney{Numerator: "13", Denominator: "7200"}},
		{proxy.ModelOperationPronunciationDictionaryCreation, "service_calls", "0.5", "USD/call", "service_calls", "call", "1", proxy.ExactMoney{Numerator: "1", Denominator: "2"}, proxy.ExactMoney{Numerator: "13", Denominator: "20"}},
	} {
		t.Run(scenario.operation, func(t *testing.T) {
			catalog := testfixtures.ProviderCatalog(t).ModelCatalog()
			var route *proxy.ProviderCatalogService
			for index := range catalog.Providers {
				if catalog.Providers[index].ID != "elevenlabs" {
					continue
				}
				for serviceIndex := range catalog.Providers[index].Services {
					candidate := &catalog.Providers[index].Services[serviceIndex]
					if candidate.Operation == scenario.operation {
						route = candidate
					}
				}
			}
			if route == nil {
				t.Fatal("canonical service missing")
			}
			conditions := proxy.CatalogPriceConditions{BillingMode: "standard", EffectiveFrom: "2026-09-01T00:00:00Z"}
			rate, err := proxy.NewCatalogDecimal(scenario.rate)
			if err != nil {
				t.Fatal(err)
			}
			// These controlled rates test the financial contract, not supplier prices.
			route.Price = proxy.ProviderCatalogPrice{Operation: scenario.operation, Available: true, Source: "https://example.com/controlled-service-rates", LastVerified: "2026-09-23", Rates: []proxy.CatalogPriceRate{{Component: scenario.component, Currency: "USD", Rate: rate, Unit: scenario.unit, Conditions: conditions}}}
			service, err := proxy.NewCatalogService(catalog)
			if err != nil {
				t.Fatal(err)
			}
			selected := service.SelectPrice("elevenlabs", "", scenario.operation, scenario.component, conditions)
			if !selected.Available || selected.Rate == nil || selected.Rate.Rate != rate || selected.Source != route.Price.Source {
				t.Fatalf("declared service price absent: %+v", selected)
			}
			bindings := []proxy.CatalogRateBinding{{Dimension: scenario.dimension, Component: scenario.component, Conditions: proxy.CatalogPriceConditions{BillingMode: "standard"}}}
			snapshot, err := service.NewRatingSnapshot("elevenlabs", "", scenario.operation, ratingTestAcceptanceTime(), bindings)
			if err != nil {
				t.Fatal(err)
			}
			quantities := []proxy.CatalogUsageQuantity{{Dimension: scenario.dimension, Unit: scenario.quantityUnit, Value: scenario.quantity}}
			assertRating := func() {
				t.Helper()
				rated, err := snapshot.Rate(quantities)
				if err != nil || rated.State != proxy.CatalogRatingResolved || rated.ProviderCost != scenario.providerCost || rated.CustomerCharge != scenario.customerCharge {
					t.Fatalf("service rating=%+v error=%v", rated, err)
				}
			}
			assertRating()
			maximum, err := snapshot.MaximumCharge([]proxy.CatalogUsageBound{{Dimension: scenario.dimension, Unit: scenario.quantityUnit, Maximum: scenario.quantity}}, 1)
			if err != nil || maximum.CustomerCharge != scenario.customerCharge {
				t.Fatalf("service maximum=%+v error=%v", maximum, err)
			}
			route.Price.Rates[0].Rate = "99"
			selected.Rate.Rate = "98"
			resolved, err := service.ResolveService("elevenlabs", scenario.operation)
			if err != nil {
				t.Fatal(err)
			}
			resolved.Price.Rates[0].Rate = "97"
			assertRating()
			if again := service.SelectPrice("elevenlabs", "", scenario.operation, scenario.component, conditions); again.Rate == nil || again.Rate.Rate != rate {
				t.Fatalf("mutable service index: %+v", again)
			}
			unknown, err := snapshot.Rate([]proxy.CatalogUsageQuantity{{Dimension: scenario.dimension, Unit: scenario.quantityUnit, UnknownReason: "not_reported"}})
			if err != nil || unknown.State != proxy.CatalogRatingUnresolved {
				t.Fatalf("unknown service usage charged: %+v %v", unknown, err)
			}
			if _, err := service.NewRatingSnapshot("elevenlabs", "invented-model", scenario.operation, ratingTestAcceptanceTime(), bindings); !errors.Is(err, proxy.ErrCatalogRatingUnavailable) {
				t.Fatalf("service accepted invented model: %v", err)
			}
		})
	}
}

func TestCatalogRatingUnavailableServiceRetainsItsReason(t *testing.T) {
	catalog := testfixtures.ProviderCatalog(t).ModelCatalog()
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	route, err := service.ResolveService("elevenlabs", proxy.ModelOperationAudioAlignment)
	if err != nil {
		t.Fatal(err)
	}
	selection := service.SelectPrice("elevenlabs", "", route.Operation, "input_audio", proxy.CatalogPriceConditions{})
	if selection.Available || selection.UnavailableReason != route.Price.UnavailableReason || selection.Source != route.Price.Source {
		t.Fatalf("unavailable service evidence lost: %+v", selection)
	}
}
