package proxy_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func TestCatalogRatingRejectsInvalidMeasurementBoundaries(t *testing.T) {
	catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{
		{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens"},
	}, nil)
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{{Component: "input_tokens", Dimension: "input_tokens"}})
	if err != nil {
		t.Fatal(err)
	}
	valid := []proxy.CatalogUsageQuantity{{Dimension: "input_tokens", Unit: "token", Value: "10"}}
	before, err := snapshot.Rate(valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct{ name, value, unit string }{
		{"fractional-token", "1.5", "token"},
		{"excessive-quantity", strings.Repeat("9", 1024), "token"},
		{"incompatible-unit", "10", "second"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			usage := []proxy.CatalogUsageQuantity{{Dimension: "input_tokens", Unit: scenario.unit, Value: scenario.value}}
			if rated, err := snapshot.Rate(usage); !errors.Is(err, proxy.ErrCatalogRatingInvalid) || rated.CustomerCharge.Numerator != "" {
				t.Fatalf("invalid usage returned a charge: result=%+v error=%v", rated, err)
			}
			bound := []proxy.CatalogUsageBound{{Dimension: "input_tokens", Unit: scenario.unit, Maximum: scenario.value}}
			if maximum, err := snapshot.MaximumCharge(bound, 1); !errors.Is(err, proxy.ErrCatalogRatingInvalid) || maximum.ReservedCents != 0 {
				t.Fatalf("invalid bound returned a reservation: result=%+v error=%v", maximum, err)
			}
			after, err := snapshot.Rate(valid)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected input changed the accepted snapshot: result=%+v error=%v", after, err)
			}
		})
	}
}

func TestCatalogRatingRejectsIncompletePriceSelection(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		at         time.Time
		conditions proxy.CatalogPriceConditions
		rates      []proxy.CatalogPriceRate
		minimum    *proxy.CatalogMinimumCharge
		want       error
	}{
		{name: "missing-acceptance-time", want: proxy.ErrCatalogRatingInvalid},
		{name: "range-in-categorical-selection", at: ratingTestAcceptanceTime(), conditions: proxy.CatalogPriceConditions{InputTokens: proxy.CatalogTokenRange{Minimum: 10}}, want: proxy.ErrCatalogRatingInvalid},
		{name: "time-in-categorical-selection", at: ratingTestAcceptanceTime(), conditions: proxy.CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}, want: proxy.ErrCatalogRatingInvalid},
		{name: "mixed-tier-units", at: ratingTestAcceptanceTime(), rates: []proxy.CatalogPriceRate{
			{Component: "input_tokens", Currency: "USD", Rate: "1", Unit: "USD/1M_tokens", Conditions: proxy.CatalogPriceConditions{InputTokens: proxy.CatalogTokenRange{MaximumExclusive: 10}}},
			{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/second", Conditions: proxy.CatalogPriceConditions{InputTokens: proxy.CatalogTokenRange{Minimum: 10}}},
		}, want: proxy.ErrCatalogRatingUnavailable},
		{name: "unsupported-minimum-unit", at: ratingTestAcceptanceTime(), minimum: &proxy.CatalogMinimumCharge{Currency: "USD", Amount: "1", Unit: "USD/hour"}, want: proxy.ErrCatalogRatingUnavailable},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			rates := scenario.rates
			if rates == nil {
				rates = []proxy.CatalogPriceRate{{Component: "input_tokens", Currency: "USD", Rate: "1", Unit: "USD/1M_tokens"}}
			}
			catalog, provider, model, operation := ratingCatalog(t, rates, scenario.minimum)
			service, err := proxy.NewCatalogService(catalog)
			if err != nil {
				t.Fatal(err)
			}
			bindings := []proxy.CatalogRateBinding{{Component: "input_tokens", Dimension: "input_tokens", Conditions: scenario.conditions}}
			if snapshot, err := service.NewRatingSnapshot(provider, model, operation, scenario.at, bindings); !errors.Is(err, scenario.want) || snapshot != nil {
				t.Fatalf("incomplete price selection returned a snapshot: snapshot=%+v error=%v", snapshot, err)
			}
		})
	}
}

func TestCatalogRatingLeavesMeasuredTierGapsUnresolved(t *testing.T) {
	catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{
		{Component: "input_tokens", Currency: "USD", Rate: "1", Unit: "USD/1M_tokens", Conditions: proxy.CatalogPriceConditions{InputTokens: proxy.CatalogTokenRange{MaximumExclusive: 10}}},
		{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: proxy.CatalogPriceConditions{InputTokens: proxy.CatalogTokenRange{Minimum: 20}}},
	}, nil)
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{{Component: "input_tokens", Dimension: "input_tokens"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, quantity := range []string{"10", "19"} {
		rated, err := snapshot.Rate([]proxy.CatalogUsageQuantity{{Dimension: "input_tokens", Unit: "token", Value: quantity}})
		if err != nil || rated.State != proxy.CatalogRatingUnresolved || !reflect.DeepEqual(rated.UnresolvedDimensions, []string{"input_tokens"}) || rated.CustomerCharge.Numerator != "" {
			t.Fatalf("unpriced tier received a charge: quantity=%s result=%+v error=%v", quantity, rated, err)
		}
	}
	if maximum, err := snapshot.MaximumCharge([]proxy.CatalogUsageBound{{Dimension: "input_tokens", Unit: "token", Maximum: "20"}}, 1); !errors.Is(err, proxy.ErrCatalogRatingUnavailable) || maximum.ReservedCents != 0 {
		t.Fatalf("bound across a tier gap returned funds: result=%+v error=%v", maximum, err)
	}
	// The same accepted schedule still prices the first quantity after the gap.
	rated, err := snapshot.Rate([]proxy.CatalogUsageQuantity{{Dimension: "input_tokens", Unit: "token", Value: "20"}})
	if err != nil || rated.State != proxy.CatalogRatingResolved || rated.CustomerCharge != (proxy.ExactMoney{Numerator: "13", Denominator: "250000"}) {
		t.Fatalf("valid tier changed after rejected usage: result=%+v error=%v", rated, err)
	}
}

func TestCatalogRatingPriceSelectionRetainsAvailabilityAndMinimum(t *testing.T) {
	catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{
		{Component: "input_tokens", Currency: "USD", Rate: "1", Unit: "USD/1M_tokens"},
	}, &proxy.CatalogMinimumCharge{Currency: "USD", Amount: "0.01", Unit: "USD/request"})
	service, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name, selectedModel, reason string
		conditions                  proxy.CatalogPriceConditions
		at                          time.Time
	}{
		{name: "unknown-model", selectedModel: "absent", at: ratingTestAcceptanceTime(), reason: "price_not_cataloged"},
		{name: "missing-time", selectedModel: model, reason: "price_conditions_incomplete"},
		{name: "range-selection", selectedModel: model, at: ratingTestAcceptanceTime(), conditions: proxy.CatalogPriceConditions{InputTokens: proxy.CatalogTokenRange{Minimum: 1}}, reason: "price_conditions_incomplete"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			selected := service.SelectApplicablePrice(provider, scenario.selectedModel, operation, "input_tokens", scenario.conditions, 1, scenario.at)
			if selected.Available || selected.Rate != nil || selected.UnavailableReason != scenario.reason {
				t.Fatalf("unavailable selection returned a rate: %+v", selected)
			}
		})
	}
	selected := service.SelectApplicablePrice(provider, model, operation, "input_tokens", proxy.CatalogPriceConditions{}, 1, ratingTestAcceptanceTime())
	if !selected.Available || selected.MinimumCharge == nil || selected.MinimumCharge.Amount != "0.01" {
		t.Fatalf("selection lost its minimum: %+v", selected)
	}
	selected.MinimumCharge.Amount = "999"
	again := service.SelectApplicablePrice(provider, model, operation, "input_tokens", proxy.CatalogPriceConditions{}, 1, ratingTestAcceptanceTime())
	if again.MinimumCharge == nil || again.MinimumCharge.Amount != "0.01" {
		t.Fatalf("caller mutation changed the catalog: %+v", again)
	}
	catalog.Prices[0].Available = false
	catalog.Prices[0].UnavailableReason = "Supplier rate pending."
	catalog.Prices[0].Rates = nil
	catalog.Prices[0].MinimumCharge = nil
	unavailable, err := proxy.NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	selected = unavailable.SelectApplicablePrice(provider, model, operation, "input_tokens", proxy.CatalogPriceConditions{}, 1, ratingTestAcceptanceTime())
	if selected.Available || selected.Rate != nil || selected.UnavailableReason != "Supplier rate pending." {
		t.Fatalf("unavailable supplier price acquired a rate: %+v", selected)
	}
}

func TestCatalogRatingRejectsInvalidPriceConditionsAtImport(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		conditions proxy.CatalogPriceConditions
	}{
		{"empty-range", proxy.CatalogPriceConditions{InputTokens: proxy.CatalogTokenRange{Minimum: 10, MaximumExclusive: 10}}},
		{"uncertain-range-with-bound", proxy.CatalogPriceConditions{InputTokens: proxy.CatalogTokenRange{Minimum: 10, UnresolvedReason: "Unknown boundary."}}},
		{"untrimmed-uncertainty", proxy.CatalogPriceConditions{InputTokens: proxy.CatalogTokenRange{UnresolvedReason: " Unknown boundary."}}},
		{"unknown-cache-class", proxy.CatalogPriceConditions{CacheClass: "unspecified"}},
		{"non-UTC-time", proxy.CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00+01:00"}},
		{"invalid-time", proxy.CatalogPriceConditions{EffectiveUntil: "tomorrow"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			catalog, _, _, _ := ratingCatalog(t, []proxy.CatalogPriceRate{{Component: "input_tokens", Currency: "USD", Rate: "1", Unit: "USD/1M_tokens", Conditions: scenario.conditions}}, nil)
			if _, err := proxy.NewCatalogService(catalog); !errors.Is(err, proxy.ErrInvalidModelCatalog) {
				t.Fatalf("invalid external price condition accepted: %v", err)
			}
		})
	}
}

func TestCatalogRatingReservationPreservesExactAmountAcrossRepeatedCalculations(t *testing.T) {
	for _, scenario := range []struct {
		name, rate, quantity string
		minimum              *proxy.CatalogMinimumCharge
		want                 proxy.ExactMoney
		cents                int64
		wantError            error
	}{
		{"minimum-per-attempt", "1", "1", &proxy.CatalogMinimumCharge{Currency: "USD", Amount: "2", Unit: "USD/request"}, proxy.ExactMoney{Numerator: "39", Denominator: "5"}, 780, nil},
		{"fractional-cent", "0.000000000000000001", "1", nil, proxy.ExactMoney{Numerator: "39", Denominator: "10000000000000000000"}, 1, nil},
		{"ledger-overflow", "1", "9223372036854775807", nil, proxy.ExactMoney{}, 0, proxy.ErrCatalogRatingUnavailable},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			rate, err := proxy.NewCatalogDecimal(scenario.rate)
			if err != nil {
				t.Fatal(err)
			}
			catalog, provider, model, operation := ratingCatalog(t, []proxy.CatalogPriceRate{{Component: "service_calls", Currency: "USD", Rate: rate, Unit: "USD/call"}}, scenario.minimum)
			service, err := proxy.NewCatalogService(catalog)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := service.NewRatingSnapshot(provider, model, operation, ratingTestAcceptanceTime(), []proxy.CatalogRateBinding{{Component: "service_calls", Dimension: "calls"}})
			if err != nil {
				t.Fatal(err)
			}
			quantities := []proxy.CatalogUsageQuantity{{Dimension: "calls", Unit: "call", Value: scenario.quantity}}
			before, err := snapshot.Rate(quantities)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				maximum, err := snapshot.MaximumCharge([]proxy.CatalogUsageBound{{Dimension: "calls", Unit: "call", Maximum: scenario.quantity}}, 3)
				if !errors.Is(err, scenario.wantError) || maximum.CustomerCharge != scenario.want || maximum.ReservedCents != scenario.cents {
					t.Fatalf("reservation changed exact units: maximum=%+v error=%v", maximum, err)
				}
				if scenario.wantError == nil {
					reserved, err := proxy.ReserveUSDCents(maximum.CustomerCharge)
					if err != nil || reserved != scenario.cents || maximum.Attempts != 3 {
						t.Fatalf("reservation differs from its reported exact charge: cents=%d maximum=%+v error=%v", reserved, maximum, err)
					}
				}
				after, err := snapshot.Rate(quantities)
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("reservation mutated the accepted rating: before=%+v after=%+v error=%v", before, after, err)
				}
			}
		})
	}
}
