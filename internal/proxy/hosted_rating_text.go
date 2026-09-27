package proxy

import (
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"time"
)

type textPriceComponent struct{ component, cacheClass string }

// These names join the shared price catalog to the native protocol meters.
// Rates and provider identities remain in the catalog.
var textPriceDimensions = map[textPriceComponent]string{
	{"input_tokens", ""}:                      "input_tokens",
	{"input_text", ""}:                        "input_tokens",
	{"output_tokens", ""}:                     "output_tokens",
	{"output_text", ""}:                       "output_tokens",
	{"cache_read", "read"}:                    "cache_read_tokens",
	{"prompt_cache_read_tokens", "read"}:      "cache_read_tokens",
	{"prompt_cache_write_tokens", "write"}:    "cache_write_tokens",
	{"prompt_cache_write_tokens", "write_5m"}: "cache_write_5m_tokens",
	{"prompt_cache_write_tokens", "write_1h"}: "cache_write_1h_tokens",
}

// Configuration constructs this scope after it resolves the offering and
// validates the selected service conditions and attempt limit.
type hostedTextPriceScope struct {
	catalog         CatalogService
	offering        ProviderOffering
	conditions      CatalogPriceConditions
	maximumAttempts uint32
}

// The model ceilings cover each continuation, including a larger output limit
// after an empty response. Cache upper bounds never assume a cache discount.
func (scope *hostedTextPriceScope) admission(codec string, acceptedAt time.Time, webSearch bool) (journalReservation, error) {
	snapshot, bounds, err := scope.pricing(codec, acceptedAt, webSearch)
	if err != nil {
		return nil, err
	}
	return newHostedPriceAdmission(snapshot, bounds, scope.maximumAttempts)
}

func (scope *hostedTextPriceScope) pricing(codec string, acceptedAt time.Time, webSearch bool) (*CatalogRatingSnapshot, []CatalogUsageBound, error) {
	offering := scope.offering
	inputCeiling := 0
	searchCeiling := 0
	for _, limit := range offering.Limits {
		if limit.ID == "web_search_calls" && limit.Unit == "calls" && !limit.AccountDependent && limit.Value != nil {
			searchCeiling = *limit.Value
		}
		// Input is contained in the total context. Every fixed ceiling applies,
		// so the smallest input or context limit bounds its cost.
		if (limit.ID == "input_tokens" || limit.ID == "context_tokens") && limit.Unit == "tokens" && !limit.AccountDependent && limit.Value != nil {
			if inputCeiling == 0 || *limit.Value < inputCeiling {
				inputCeiling = *limit.Value
			}
		}
	}
	if inputCeiling <= 0 || offering.OutputTokenLimit <= 0 {
		return nil, nil, fmt.Errorf("%w: catalog input and output token ceilings required", ErrCatalogRatingUnavailable)
	}
	profile := newTextJournalMeter(codec)
	if webSearch && (profile.codec != CatalogProtocolOpenAIResponses || searchCeiling <= 0) {
		return nil, nil, fmt.Errorf("%w: provider tool cost requires an enforced call bound", ErrCatalogRatingUnavailable)
	}
	snapshot, err := profile.ratingSnapshot(scope, acceptedAt, webSearch)
	if err != nil {
		return nil, nil, err
	}
	bounds := make([]CatalogUsageBound, 0, len(snapshot.components))
	for _, component := range snapshot.components {
		for _, dimension := range []string{component.binding.Dimension, component.binding.AdditionalDimension} {
			if dimension == "" {
				continue
			}
			var ceiling int
			unit := "token"
			// ratingSnapshot binds only textPriceDimensions, separate reasoning,
			// and the explicitly priced search-call dimension.
			switch dimension {
			case "input_tokens", "cache_read_tokens", "cache_write_tokens", "cache_write_5m_tokens", "cache_write_1h_tokens":
				ceiling = inputCeiling
			case "output_tokens", "reasoning_tokens":
				ceiling = offering.OutputTokenLimit
			case "web_search_calls":
				ceiling, unit = searchCeiling, "call"
			}
			maximum := strconv.Itoa(ceiling)
			if webSearch && (dimension == "input_tokens" || dimension == "cache_read_tokens") {
				passes := new(big.Int).Add(big.NewInt(int64(searchCeiling)), big.NewInt(1))
				maximum = passes.Mul(passes, big.NewInt(int64(ceiling))).String()
			}
			bounds = append(bounds, CatalogUsageBound{Dimension: dimension, Unit: unit, Maximum: maximum})
		}
	}
	for _, dimension := range snapshot.zeroDimensions {
		bounds = append(bounds, CatalogUsageBound{Dimension: dimension, Unit: "token", Maximum: "0"})
	}
	return snapshot, bounds, nil
}

func (profile journalTokenMeter) ratingSnapshot(scope *hostedTextPriceScope, acceptedAt time.Time, webSearch bool) (*CatalogRatingSnapshot, error) {
	provider, model := scope.offering.Provider, scope.offering.Model
	descriptor, found := scope.catalog.catalog.prices[catalogPriceIdentifier(provider, model, ModelOperationText)]
	if !found || !descriptor.Available {
		return nil, fmt.Errorf("%w: text price unavailable", ErrCatalogRatingUnavailable)
	}
	dimensions := make(map[string]bool, len(profile.fields))
	separateReasoning := false
	zeroDimensions := []string{}
	for _, field := range profile.fields {
		if field.includedIn == "" && field.dimension == "reasoning_tokens" {
			separateReasoning = true
		}
		if field.includedIn == "" && field.dimension == "tool_input_tokens" {
			zeroDimensions = append(zeroDimensions, field.dimension)
		}
		dimensions[field.dimension] = true
	}
	bindings := []CatalogRateBinding{}
	selected := map[CatalogRateBinding]bool{}
	excludedComponents := []string{}
	searchPriced := false
	for _, rate := range descriptor.Rates {
		if rate.Component == "web_search_calls" {
			if !webSearch {
				if !searchPriced {
					excludedComponents = append(excludedComponents, rate.Component)
				}
				searchPriced = true
				continue
			}
			if profile.codec != CatalogProtocolOpenAIResponses || rate.Unit != "USD/call" {
				return nil, fmt.Errorf("%w: unsupported search price", ErrCatalogRatingUnavailable)
			}
		}
		// These adapters do not allocate or reference explicit cache resources.
		if rate.Component == "cache_storage" && (profile.codec == CatalogProtocolGeminiInteractions || profile.codec == CatalogProtocolVertexGenerateContent) {
			if !slices.Contains(excludedComponents, rate.Component) {
				excludedComponents = append(excludedComponents, rate.Component)
			}
			continue
		}
		categorical := categoricalPriceConditions(rate.Conditions)
		base := categorical
		base.CacheClass = ""
		if base != scope.conditions || !catalogPriceActiveAt(rate.Conditions, acceptedAt) {
			continue
		}
		dimension := textPriceDimensions[textPriceComponent{rate.Component, categorical.CacheClass}]
		if rate.Component == "web_search_calls" && webSearch {
			dimension, searchPriced = "web_search_calls", true
		} else if !dimensions[dimension] || rate.Unit != "USD/1M_tokens" {
			return nil, fmt.Errorf("%w: protocol=%s component=%s has no native billing quantity", ErrCatalogRatingUnavailable, profile.codec, rate.Component)
		}
		binding := CatalogRateBinding{Dimension: dimension, Component: rate.Component, Conditions: categorical}
		if dimension == "output_tokens" && separateReasoning {
			binding.AdditionalDimension = "reasoning_tokens"
		}
		if !selected[binding] {
			bindings = append(bindings, binding)
			selected[binding] = true
		}
	}
	if webSearch && !searchPriced {
		return nil, fmt.Errorf("%w: search call rate required", ErrCatalogRatingUnavailable)
	}
	return scope.catalog.newRatingSnapshot(provider, model, ModelOperationText, acceptedAt, bindings, excludedComponents, zeroDimensions)
}
