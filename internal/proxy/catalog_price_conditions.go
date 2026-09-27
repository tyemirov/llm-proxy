package proxy

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var catalogPriceRegionPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,127}$`)

// CatalogTokenRange is a half-open interval of exact input token counts.
// A zero upper bound is unbounded. An unresolved boundary is never paid eligible.
type CatalogTokenRange struct {
	Minimum          uint64 `json:"minimum,string" mapstructure:"minimum" yaml:"minimum,omitempty"`
	MaximumExclusive uint64 `json:"maximum_exclusive,string" mapstructure:"maximum_exclusive" yaml:"maximum_exclusive,omitempty"`
	UnresolvedReason string `json:"unresolved_reason" mapstructure:"unresolved_reason" yaml:"unresolved_reason,omitempty"`
}

func validateCatalogPriceConditions(conditions CatalogPriceConditions) error {
	tokens := conditions.InputTokens
	if tokens.MaximumExclusive != 0 && tokens.Minimum >= tokens.MaximumExclusive {
		return fmt.Errorf("%w: empty price token range", ErrInvalidModelCatalog)
	}
	if tokens.UnresolvedReason != strings.TrimSpace(tokens.UnresolvedReason) || (tokens.UnresolvedReason != "" && (tokens.Minimum != 0 || tokens.MaximumExclusive != 0)) {
		return fmt.Errorf("%w: invalid unresolved token range", ErrInvalidModelCatalog)
	}
	switch conditions.CacheClass {
	case "", "read", "write", "write_5m", "write_1h", "storage":
	default:
		return fmt.Errorf("%w: unknown cache class", ErrInvalidModelCatalog)
	}
	switch conditions.ServiceTier {
	case "", "standard", "priority", "batch", "flex", "fast":
	default:
		return fmt.Errorf("%w: unknown service tier", ErrInvalidModelCatalog)
	}
	if conditions.Region != "" && !catalogPriceRegionPattern.MatchString(conditions.Region) {
		return fmt.Errorf("%w: invalid price region", ErrInvalidModelCatalog)
	}
	for _, value := range []string{conditions.EffectiveFrom, conditions.EffectiveUntil} {
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil || !strings.HasSuffix(value, "Z") || parsed.Format(time.RFC3339) != value {
			return fmt.Errorf("%w: invalid effective price time", ErrInvalidModelCatalog)
		}
	}
	if conditions.EffectiveFrom != "" && conditions.EffectiveUntil != "" && !catalogPriceTime(conditions.EffectiveFrom).Before(catalogPriceTime(conditions.EffectiveUntil)) {
		return fmt.Errorf("%w: empty effective price interval", ErrInvalidModelCatalog)
	}
	return nil
}

func catalogPriceTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}

func categoricalPriceConditions(conditions CatalogPriceConditions) CatalogPriceConditions {
	conditions.InputTokens = CatalogTokenRange{}
	conditions.EffectiveFrom = ""
	conditions.EffectiveUntil = ""
	return conditions
}

func catalogPriceConditionsOverlap(left, right CatalogPriceConditions) bool {
	if categoricalPriceConditions(left) != categoricalPriceConditions(right) || left.InputTokens.UnresolvedReason != "" || right.InputTokens.UnresolvedReason != "" {
		return false
	}
	if (left.InputTokens.MaximumExclusive != 0 && left.InputTokens.MaximumExclusive <= right.InputTokens.Minimum) || (right.InputTokens.MaximumExclusive != 0 && right.InputTokens.MaximumExclusive <= left.InputTokens.Minimum) {
		return false
	}
	if (left.EffectiveUntil != "" && right.EffectiveFrom != "" && !catalogPriceTime(left.EffectiveUntil).After(catalogPriceTime(right.EffectiveFrom))) || (right.EffectiveUntil != "" && left.EffectiveFrom != "" && !catalogPriceTime(right.EffectiveUntil).After(catalogPriceTime(left.EffectiveFrom))) {
		return false
	}
	return true
}

func catalogPriceActiveAt(conditions CatalogPriceConditions, at time.Time) bool {
	return !at.IsZero() && (conditions.EffectiveFrom == "" || !at.Before(catalogPriceTime(conditions.EffectiveFrom))) && (conditions.EffectiveUntil == "" || at.Before(catalogPriceTime(conditions.EffectiveUntil)))
}

// SelectApplicablePrice matches typed ranges and times within the shared catalog.
// Categorical conditions must match exactly. Unknown boundaries remain unavailable.
func (service CatalogService) SelectApplicablePrice(provider, model, operation, component string, conditions CatalogPriceConditions, inputTokens uint64, at time.Time) CatalogPriceSelection {
	descriptor, found := service.catalog.prices[catalogPriceIdentifier(provider, model, operation)]
	if !found {
		return CatalogPriceSelection{UnavailableReason: "price_not_cataloged"}
	}
	selection := CatalogPriceSelection{Source: descriptor.Source, LastVerified: descriptor.LastVerified, UnavailableReason: descriptor.UnavailableReason}
	if descriptor.MinimumCharge != nil {
		minimum := *descriptor.MinimumCharge
		selection.MinimumCharge = &minimum
	}
	if !descriptor.Available {
		return selection
	}
	if conditions != categoricalPriceConditions(conditions) || at.IsZero() {
		selection.UnavailableReason = "price_conditions_incomplete"
		return selection
	}
	for _, rate := range descriptor.Rates {
		tokens := rate.Conditions.InputTokens
		if rate.Component != component || categoricalPriceConditions(rate.Conditions) != conditions || tokens.UnresolvedReason != "" || inputTokens < tokens.Minimum || (tokens.MaximumExclusive != 0 && inputTokens >= tokens.MaximumExclusive) || !catalogPriceActiveAt(rate.Conditions, at) {
			continue
		}
		selected := rate
		selection.Rate = &selected
		selection.Available = true
		selection.UnavailableReason = ""
		return selection
	}
	selection.UnavailableReason = "exact_price_unavailable"
	return selection
}
