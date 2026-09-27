package proxy_test

import (
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func ratingCatalog(t *testing.T, rates []proxy.CatalogPriceRate, minimum *proxy.CatalogMinimumCharge) (proxy.ModelCatalog, string, string, string) {
	t.Helper()
	document, err := os.ReadFile("../../configs/providers.yml")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := proxy.ParseProviderCatalog(document)
	if err != nil {
		t.Fatal(err)
	}
	model := catalog.ModelCatalog()
	descriptor := &model.Prices[0]
	descriptor.Available = true
	descriptor.UnavailableReason = ""
	descriptor.Rates = rates
	descriptor.MinimumCharge = minimum
	return model, descriptor.Provider, descriptor.Model, descriptor.Operation
}

func TestCatalogRatingExactMarkupAndInclusiveQuantities(t *testing.T) {
	conditions := proxy.CatalogPriceConditions{BillingMode: "standard"}
	rates := []proxy.CatalogPriceRate{
		{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: conditions},
		{Component: "cache_read", Currency: "USD", Rate: "0.5", Unit: "USD/1M_tokens", Conditions: conditions},
		{Component: "output_tokens", Currency: "USD", Rate: "8", Unit: "USD/1M_tokens", Conditions: conditions},
	}
	catalog, provider, model, operation := ratingCatalog(t, rates, nil)
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	bindings := []proxy.CatalogRateBinding{
		{Dimension: "input_tokens", Component: "input_tokens", Conditions: conditions},
		{Dimension: "cached_tokens", Component: "cache_read", Conditions: conditions},
		{Dimension: "output_tokens", Component: "output_tokens", Conditions: conditions},
	}
	snapshot, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), bindings)
	if err != nil {
		t.Fatal(err)
	}
	quantities := []proxy.CatalogUsageQuantity{
		{Dimension: "input_tokens", Unit: "token", Value: "1000"},
		{Dimension: "cached_tokens", Unit: "token", Value: "200", IncludedIn: "input_tokens"},
		{Dimension: "output_tokens", Unit: "token", Value: "100"},
		{Dimension: "reasoning_tokens", Unit: "token", Value: "40", IncludedIn: "output_tokens"},
	}
	result, err := snapshot.Rate(quantities)
	if err != nil {
		t.Fatal(err)
	}
	// (800*2 + 200*0.5 + 100*8)/1M = $0.0025; 30% markup = $0.00325.
	if result.State != proxy.CatalogRatingResolved || result.ProviderCost != (proxy.ExactMoney{Numerator: "1", Denominator: "400"}) || result.CustomerCharge != (proxy.ExactMoney{Numerator: "13", Denominator: "4000"}) {
		t.Fatalf("unexpected exact rating: %+v", result)
	}
	if len(result.Lines) != 3 || result.Lines[0].Quantity != "800" || result.Lines[2].Quantity != "100" {
		t.Fatalf("inclusive quantities: %+v", result.Lines)
	}
	// Mutations outside the snapshot cannot rewrite accepted rates or conditions.
	rates[0].Rate = "999"
	catalog.Prices[0].Rates[0].Rate = "999"
	bindings[0].Component = "changed"
	result.Lines[0].ProviderRate = "999"
	again, err := snapshot.Rate(quantities)
	if err != nil || again.ProviderCost.Numerator != "1" || again.Lines[0].ProviderRate != "2" {
		t.Fatalf("mutable snapshot: %+v %v", again, err)
	}
	replay, err := snapshot.Rate(quantities)
	if err != nil || !reflect.DeepEqual(replay, again) {
		t.Fatalf("unstable rating: %+v %v", replay, err)
	}
	quantities[1].Value = ""
	quantities[1].UnknownReason = "not_reported"
	unresolved, err := snapshot.Rate(quantities)
	if err != nil || unresolved.State != proxy.CatalogRatingUnresolved || len(unresolved.UnresolvedDimensions) != 1 || unresolved.ProviderCost.Numerator != "" {
		t.Fatalf("unknown usage settled: %+v %v", unresolved, err)
	}
}

func TestCatalogRatingUnitsMinimumAndExactFractions(t *testing.T) {
	for _, test := range []struct{ name, unit, quantityUnit, quantity, rate, providerNumerator, providerDenominator string }{
		{"minute", "USD/minute", "second", "1", "1", "1", "60"},
		{"hour", "USD/hour", "second", "1", "1", "1", "3600"},
		{"cache storage", "USD/1M_token_hours", "token_second", "3600", "2", "1", "500000"},
		{"video", "USD/output_second", "second", "1.25", "0.14", "7", "40"},
		{"image", "USD/input_image", "image", "3", "0.02", "3", "50"},
		{"large", "USD/1M_tokens", "token", "9007199254740993", "1", "9007199254740993", "1000000"},
		{"fraction", "USD/1M_tokens", "token", "1", "0.000000000000000001", "1", "1000000000000000000000000"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rate, err := proxy.NewCatalogDecimal(test.rate)
			if err != nil {
				t.Fatal(err)
			}
			catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{{Component: "usage", Currency: "USD", Rate: rate, Unit: test.unit}}, nil)
			service, err := proxy.NewCatalogService(catalog)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{{Component: "usage", Dimension: "usage"}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := snapshot.Rate([]proxy.CatalogUsageQuantity{{Dimension: "usage", Unit: test.quantityUnit, Value: test.quantity}})
			if err != nil || result.ProviderCost != (proxy.ExactMoney{Numerator: test.providerNumerator, Denominator: test.providerDenominator}) {
				t.Fatalf("rating=%+v error=%v", result, err)
			}
		})
	}
	catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{{Component: "usage", Currency: "USD", Rate: "0.01", Unit: "USD/input_image"}}, &proxy.CatalogMinimumCharge{Currency: "USD", Amount: "1", Unit: "USD/request"})
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{{Component: "usage", Dimension: "usage"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := snapshot.Rate([]proxy.CatalogUsageQuantity{{Dimension: "usage", Unit: "image", Value: "1"}})
	if err != nil || result.ProviderCost != (proxy.ExactMoney{Numerator: "1", Denominator: "1"}) || result.CustomerCharge != (proxy.ExactMoney{Numerator: "13", Denominator: "10"}) || result.MinimumAdjustment != (proxy.ExactMoney{Numerator: "99", Denominator: "100"}) {
		t.Fatalf("minimum=%+v error=%v", result, err)
	}
	_, err = service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), nil)
	if !errors.Is(err, proxy.ErrCatalogRatingUnavailable) {
		t.Fatalf("missing component: %v", err)
	}
	_, err = service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{{Component: "usage", Dimension: "usage", Conditions: proxy.CatalogPriceConditions{Mode: "unknown"}}})
	if !errors.Is(err, proxy.ErrCatalogRatingUnavailable) {
		t.Fatalf("unmatched condition: %v", err)
	}
}

func TestCatalogRatingCarriesFractionsAcrossLedgerSettlements(t *testing.T) {
	charge := proxy.ExactMoney{Numerator: "13", Denominator: "4000"}
	carry := proxy.ExactMoney{Numerator: "0", Denominator: "1"}
	var total int64
	for range 400 {
		cents, next, err := proxy.SettleUSDCents(charge, carry)
		if err != nil {
			t.Fatal(err)
		}
		total += cents
		carry = next
	}
	if total != 130 || carry != (proxy.ExactMoney{Numerator: "0", Denominator: "1"}) {
		t.Fatalf("lost fractional charges: cents=%d carry=%+v", total, carry)
	}
	reserved, err := proxy.ReserveUSDCents(charge)
	if err != nil || reserved != 1 {
		t.Fatalf("reservation=%d error=%v", reserved, err)
	}
	for _, value := range []proxy.ExactMoney{
		{Numerator: "-1", Denominator: "1"}, {Numerator: "1", Denominator: "0"},
		{Numerator: "1.2", Denominator: "1"}, {Numerator: "01", Denominator: "1"},
		{Numerator: "9223372036854775808", Denominator: "100"}, {},
	} {
		if _, err := proxy.ReserveUSDCents(value); err == nil {
			t.Fatalf("invalid reservation accepted: %+v", value)
		}
		if _, _, err := proxy.SettleUSDCents(value, proxy.ExactMoney{Numerator: "0", Denominator: "1"}); err == nil {
			t.Fatalf("invalid settlement accepted: %+v", value)
		}
	}
	if _, _, err := proxy.SettleUSDCents(charge, proxy.ExactMoney{Numerator: "1", Denominator: "100"}); err == nil {
		t.Fatal("carry of a full cent accepted")
	}
	maximum := proxy.ExactMoney{Numerator: "9223372036854775807", Denominator: "100"}
	if cents, err := proxy.ReserveUSDCents(maximum); err != nil || cents != 9223372036854775807 {
		t.Fatalf("maximum cents=%d error=%v", cents, err)
	}
}

func TestCatalogRatingRejectsInvalidAndAmbiguousEvidence(t *testing.T) {
	rates := []proxy.CatalogPriceRate{
		{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens"},
		{Component: "cache_read", Currency: "USD", Rate: "1", Unit: "USD/1M_tokens"},
	}
	catalog, provider, model, operation := ratingCatalog(t, rates, nil)
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	bindings := []proxy.CatalogRateBinding{{Component: "input_tokens", Dimension: "input"}, {Component: "cache_read", Dimension: "cache"}}
	snapshot, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), bindings)
	if err != nil {
		t.Fatal(err)
	}
	for _, quantities := range [][]proxy.CatalogUsageQuantity{
		{{Dimension: "input", Unit: "token", Value: "-1"}},
		{{Dimension: "input", Unit: "token", Value: "1e3"}},
		{{Dimension: "input", Unit: "token", Value: "10"}, {Dimension: "input", Unit: "token", Value: "1"}},
		{{Dimension: "input", Unit: "image", Value: "10"}, {Dimension: "cache", Unit: "token", Value: "1"}},
		{{Dimension: "input", Unit: "token", Value: "10"}, {Dimension: "cache", Unit: "token", Value: "11", IncludedIn: "input"}},
		{{Dimension: "input", Unit: "token", Value: "10", IncludedIn: "cache"}, {Dimension: "cache", Unit: "token", Value: "10", IncludedIn: "input"}},
	} {
		if _, err := snapshot.Rate(quantities); !errors.Is(err, proxy.ErrCatalogRatingInvalid) {
			t.Fatalf("invalid quantities accepted: %+v error=%v", quantities, err)
		}
	}
	for _, selections := range [][]proxy.CatalogRateBinding{
		{{Component: "input_tokens", Dimension: "input"}},
		{{Component: "input_tokens", Dimension: "input"}, {Component: "cache_read", Dimension: "input"}},
		{{Component: "input_tokens", Dimension: "input"}, {Component: "input_tokens", Dimension: "another_input"}, {Component: "cache_read", Dimension: "cache"}},
		{{Component: "input_tokens", Dimension: "invalid!"}, {Component: "cache_read", Dimension: "cache"}},
	} {
		if _, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), selections); err == nil {
			t.Fatalf("ambiguous bindings accepted: %+v", selections)
		}
	}
	missing, err := snapshot.Rate([]proxy.CatalogUsageQuantity{{Dimension: "input", Unit: "token", Value: "10"}})
	if err != nil || missing.State != proxy.CatalogRatingUnresolved {
		t.Fatalf("missing quantity was charged: %+v %v", missing, err)
	}
	if _, err := service.NewRatingSnapshot(provider, "absent", operation, ratingTestAcceptanceTime(), bindings); !errors.Is(err, proxy.ErrCatalogRatingUnavailable) {
		t.Fatalf("absent price: %v", err)
	}
	catalog.Prices[0].Rates[0].Unit = "USD/unpriced_measure"
	unsupported, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unsupported.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), bindings); !errors.Is(err, proxy.ErrCatalogRatingUnavailable) {
		t.Fatalf("unsupported unit: %v", err)
	}
}

func TestCatalogRatingMaximumIncludesEachAttemptAndDoesNotDiscountUnknownCache(t *testing.T) {
	catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{
		{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens"},
		{Component: "cache_read", Currency: "USD", Rate: "0.5", Unit: "USD/1M_tokens"},
		{Component: "output_tokens", Currency: "USD", Rate: "8", Unit: "USD/1M_tokens"},
	}, nil)
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{
		{Component: "input_tokens", Dimension: "input"},
		{Component: "cache_read", Dimension: "cache"},
		{Component: "output_tokens", Dimension: "output"},
	})
	if err != nil {
		t.Fatal(err)
	}
	bounds := []proxy.CatalogUsageBound{
		{Dimension: "input", Unit: "token", Maximum: "1000"},
		{Dimension: "cache", Unit: "token", Maximum: "1000"},
		{Dimension: "output", Unit: "token", Maximum: "1000"},
	}
	maximum, err := snapshot.MaximumCharge(bounds, 3)
	// A cache upper bound is not evidence that the input will be cached.
	// (1000*2 + 1000*0.5 + 1000*8)/1M * 1.30 * 3 = $0.04095.
	if err != nil || maximum.CustomerCharge != (proxy.ExactMoney{Numerator: "819", Denominator: "20000"}) || maximum.ReservedCents != 5 || maximum.Attempts != 3 {
		t.Fatalf("maximum=%+v error=%v", maximum, err)
	}
	if _, err := snapshot.MaximumCharge(bounds, 0); err == nil {
		t.Fatal("zero attempt limit accepted")
	}
	if _, err := snapshot.MaximumCharge(bounds[:2], 3); err == nil {
		t.Fatal("unbounded output accepted")
	}
	bounds[2].Maximum = ""
	if _, err := snapshot.MaximumCharge(bounds, 3); err == nil {
		t.Fatal("unknown maximum accepted")
	}
}

func TestCatalogRatingRetainsCompleteTierScheduleAndBoundsEveryTier(t *testing.T) {
	lower := proxy.CatalogPriceConditions{InputTokens: proxy.CatalogTokenRange{MaximumExclusive: 200000}}
	upper := proxy.CatalogPriceConditions{InputTokens: proxy.CatalogTokenRange{Minimum: 200000}}
	catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{
		{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: lower},
		{Component: "input_tokens", Currency: "USD", Rate: "4", Unit: "USD/1M_tokens", Conditions: upper},
		{Component: "output_tokens", Currency: "USD", Rate: "8", Unit: "USD/1M_tokens", Conditions: lower},
		{Component: "output_tokens", Currency: "USD", Rate: "6", Unit: "USD/1M_tokens", Conditions: upper},
	}, nil)
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{{Component: "input_tokens", Dimension: "input_tokens"}, {Component: "output_tokens", Dimension: "output_tokens"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ input, providerRate, outputRate string }{{"199999", "2", "8"}, {"200000", "4", "6"}} {
		rated, err := snapshot.Rate([]proxy.CatalogUsageQuantity{{Dimension: "input_tokens", Unit: "token", Value: test.input}, {Dimension: "output_tokens", Unit: "token", Value: "1000"}})
		if err != nil || rated.State != proxy.CatalogRatingResolved || string(rated.Lines[0].ProviderRate) != test.providerRate || string(rated.Lines[1].ProviderRate) != test.outputRate {
			t.Fatalf("tier result=%+v error=%v", rated, err)
		}
	}
	maximum, err := snapshot.MaximumCharge([]proxy.CatalogUsageBound{{Dimension: "input_tokens", Unit: "token", Maximum: "300000"}, {Dimension: "output_tokens", Unit: "token", Maximum: "1000"}}, 2)
	// Conservative component maxima: (300000*4 + 1000*8)/1M * 1.3 * 2 = 3.1408.
	if err != nil || maximum.CustomerCharge != (proxy.ExactMoney{Numerator: "1963", Denominator: "625"}) || maximum.ReservedCents != 315 {
		t.Fatalf("tier maximum=%+v error=%v", maximum, err)
	}
	lowMaximum, err := snapshot.MaximumCharge([]proxy.CatalogUsageBound{{Dimension: "input_tokens", Unit: "token", Maximum: "1000"}, {Dimension: "output_tokens", Unit: "token", Maximum: "1000"}}, 1)
	if err != nil || lowMaximum.CustomerCharge != (proxy.ExactMoney{Numerator: "13", Denominator: "1000"}) {
		t.Fatalf("inaccessible tier inflated reservation: %+v %v", lowMaximum, err)
	}
}

func ratingTestAcceptanceTime() time.Time { return time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC) }

func TestCatalogRatingCompoundQuantitiesRejectOverlapAndUnknownContributions(t *testing.T) {
	catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{{Component: "output_tokens", Currency: "USD", Rate: "8", Unit: "USD/1M_tokens"}}, nil)
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	binding := proxy.CatalogRateBinding{Component: "output_tokens", Dimension: "output_tokens", AdditionalDimension: "reasoning_tokens"}
	snapshot, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{binding})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		quantity proxy.CatalogUsageQuantity
		invalid  bool
	}{
		{"already included", proxy.CatalogUsageQuantity{Dimension: "reasoning_tokens", Unit: "token", Value: "40", IncludedIn: "output_tokens"}, true},
		{"incompatible unit", proxy.CatalogUsageQuantity{Dimension: "reasoning_tokens", Unit: "second", Value: "40"}, true},
		{"unknown contribution", proxy.CatalogUsageQuantity{Dimension: "reasoning_tokens", Unit: "token", UnknownReason: "not_reported"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := snapshot.Rate([]proxy.CatalogUsageQuantity{{Dimension: "output_tokens", Unit: "token", Value: "100"}, test.quantity})
			if test.invalid {
				if !errors.Is(err, proxy.ErrCatalogRatingInvalid) {
					t.Fatalf("overlapping quantities were charged: %+v %v", result, err)
				}
			} else if err != nil || result.State != proxy.CatalogRatingUnresolved {
				t.Fatalf("unknown contribution became zero: %+v %v", result, err)
			}
		})
	}
	if _, err := snapshot.MaximumCharge([]proxy.CatalogUsageBound{{Dimension: "output_tokens", Unit: "token", Maximum: "100"}}, 1); !errors.Is(err, proxy.ErrCatalogRatingUnavailable) {
		t.Fatalf("unbounded additional quantity accepted: %v", err)
	}
	maximum, err := snapshot.MaximumCharge([]proxy.CatalogUsageBound{{Dimension: "output_tokens", Unit: "token", Maximum: "100"}, {Dimension: "reasoning_tokens", Unit: "token", Maximum: "40"}}, 1)
	if err != nil || maximum.CustomerCharge != (proxy.ExactMoney{Numerator: "91", Denominator: "62500"}) {
		t.Fatalf("compound bound=%+v error=%v", maximum, err)
	}
	binding.AdditionalDimension = binding.Dimension
	if _, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{binding}); !errors.Is(err, proxy.ErrCatalogRatingInvalid) {
		t.Fatalf("repeated source dimension accepted: %v", err)
	}
}

func TestCatalogRatingRejectsUncoveredTierBounds(t *testing.T) {
	for _, test := range []struct {
		name     string
		interval proxy.CatalogTokenRange
		maximum  string
	}{
		{"missing initial tier", proxy.CatalogTokenRange{Minimum: 10}, "20"},
		{"exclusive upper endpoint", proxy.CatalogTokenRange{MaximumExclusive: 10}, "10"},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{{Component: "output_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: proxy.CatalogPriceConditions{InputTokens: test.interval}}}, nil)
			service, err := proxy.NewCatalogService(catalog)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{{Component: "output_tokens", Dimension: "output_tokens"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := snapshot.MaximumCharge([]proxy.CatalogUsageBound{{Dimension: "input_tokens", Unit: "token", Maximum: test.maximum}, {Dimension: "output_tokens", Unit: "token", Maximum: "100"}}, 1); !errors.Is(err, proxy.ErrCatalogRatingUnavailable) {
				t.Fatalf("incomplete schedule admitted: %v", err)
			}
			if _, err := snapshot.MaximumCharge([]proxy.CatalogUsageBound{{Dimension: "output_tokens", Unit: "token", Maximum: "100"}}, 1); !errors.Is(err, proxy.ErrCatalogRatingUnavailable) {
				t.Fatalf("absent tier bound admitted: %v", err)
			}
			result, err := snapshot.Rate([]proxy.CatalogUsageQuantity{{Dimension: "output_tokens", Unit: "token", Value: "100"}})
			if err != nil || result.State != proxy.CatalogRatingUnresolved {
				t.Fatalf("absent tier quantity settled: %+v %v", result, err)
			}
		})
	}
}

func TestCatalogRatingSelectsScheduleAtAcceptanceTime(t *testing.T) {
	previous := proxy.CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z", EffectiveUntil: "2026-09-22T12:00:00Z"}
	current := proxy.CatalogPriceConditions{EffectiveFrom: "2026-09-22T12:00:00Z", EffectiveUntil: "2026-10-01T00:00:00Z"}
	future := proxy.CatalogPriceConditions{EffectiveFrom: "2026-10-01T00:00:00Z"}
	catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{
		{Component: "input_tokens", Currency: "USD", Rate: "1", Unit: "USD/1M_tokens", Conditions: previous},
		{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: current},
		{Component: "input_tokens", Currency: "USD", Rate: "9", Unit: "USD/1M_tokens", Conditions: future},
	}, nil)
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{{Component: "input_tokens", Dimension: "input_tokens"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := snapshot.Rate([]proxy.CatalogUsageQuantity{{Dimension: "input_tokens", Unit: "token", Value: "100"}})
	if err != nil || result.State != proxy.CatalogRatingResolved || result.Lines[0].ProviderRate != "2" {
		t.Fatalf("incorrect effective schedule: %+v %v", result, err)
	}
}
