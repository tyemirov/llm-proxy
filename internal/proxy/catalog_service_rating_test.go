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
	const unavailableReason = "Controlled service price is unavailable."
	const source = "https://example.com/controlled-service-rates"
	const lastVerified = "2026-09-23"
	for providerIndex := range catalog.Providers {
		provider := &catalog.Providers[providerIndex]
		if provider.ID != "elevenlabs" {
			continue
		}
		for serviceIndex := range provider.Services {
			route := &provider.Services[serviceIndex]
			if route.Operation == proxy.ModelOperationAudioAlignment {
				route.Price = proxy.ProviderCatalogPrice{
					Operation: route.Operation, Available: false, Source: source,
					LastVerified: lastVerified, UnavailableReason: unavailableReason,
				}
			}
		}
	}
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	route, err := service.ResolveService("elevenlabs", proxy.ModelOperationAudioAlignment)
	if err != nil {
		t.Fatal(err)
	}
	selection := service.SelectPrice("elevenlabs", "", route.Operation, "input_audio", proxy.CatalogPriceConditions{})
	if selection.Available || selection.UnavailableReason != unavailableReason || selection.Source != source || selection.LastVerified != lastVerified {
		t.Fatalf("unavailable service evidence lost: %+v", selection)
	}
}

func TestCatalogRatingAdvertisedOpenAIImageAndResponsePrices(t *testing.T) {
	service, err := proxy.NewCatalogService(testfixtures.ProviderCatalog(t).ModelCatalog())
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct{ model, operation, component, cache, rate string }{
		{"gpt-5", proxy.ModelOperationText, "input_tokens", "", "1.25"},
		{"gpt-5", proxy.ModelOperationText, "cache_read", "read", "0.125"},
		{"gpt-5", proxy.ModelOperationText, "output_tokens", "", "10"},
		{"gpt-image-2", proxy.ModelOperationImageGeneration, "input_text", "", "2.5"},
		{"gpt-image-2", proxy.ModelOperationImageGeneration, "input_image", "", "4"},
		{"gpt-image-2", proxy.ModelOperationImageGeneration, "output_image", "", "15"},
		{"gpt-image-2", proxy.ModelOperationImageEditing, "input_text", "", "2.5"},
		{"gpt-image-2", proxy.ModelOperationImageEditing, "input_image", "", "4"},
		{"gpt-image-2", proxy.ModelOperationImageEditing, "output_image", "", "15"},
	} {
		selected := service.SelectPrice("openai", scenario.model, scenario.operation, scenario.component, proxy.CatalogPriceConditions{BillingMode: "standard", ServiceTier: "standard", CacheClass: scenario.cache, EffectiveFrom: "2026-09-26T00:00:00Z"})
		if !selected.Available || selected.Rate == nil || string(selected.Rate.Rate) != scenario.rate || selected.Rate.Unit != "USD/1M_tokens" || selected.Source != "https://developers.openai.com/api/docs/pricing" || selected.LastVerified != "2026-09-26" {
			t.Fatalf("advertised %s %s %s price=%+v", scenario.model, scenario.operation, scenario.component, selected)
		}
	}
	offering, err := service.ResolveOffering("openai", "gpt-5")
	if err != nil || offering.OutputTokenLimit != 128000 {
		t.Fatalf("response output bound=%+v error=%v", offering, err)
	}
	found := false
	for _, limit := range offering.Limits {
		if limit.ID == "context_tokens" && limit.Value != nil && *limit.Value == 400000 {
			found = true
		}
	}
	if !found {
		t.Fatal("published response context ceiling absent")
	}
}
