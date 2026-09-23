package proxy

import (
	"errors"
	"fmt"
	"math/big"
	"slices"
	"time"
)

const (
	// CatalogRatingResolved means every required quantity has an exact price.
	CatalogRatingResolved = "resolved"
	// CatalogRatingUnresolved prevents settlement of incomplete usage.
	CatalogRatingUnresolved = "unresolved"
)

var (
	// ErrCatalogRatingUnavailable identifies a route that cannot be rated.
	ErrCatalogRatingUnavailable = errors.New("catalog rating unavailable")
	// ErrCatalogRatingInvalid identifies invalid metering input.
	ErrCatalogRatingInvalid = errors.New("invalid catalog rating input")
)

// ExactMoney represents an exact USD amount as a reduced rational number.
// No ledger rounding occurs in a price snapshot.
type ExactMoney struct {
	Numerator   string `json:"numerator"`
	Denominator string `json:"denominator"`
}

// CatalogRateBinding binds a measured dimension to an exact catalog component.
type CatalogRateBinding struct {
	Dimension           string                 `json:"dimension"`
	AdditionalDimension string                 `json:"additional_dimension"`
	Component           string                 `json:"component"`
	Conditions          CatalogPriceConditions `json:"conditions"`
}

// CatalogUsageQuantity uses the usage journal's quantity and inclusion contract.
type CatalogUsageQuantity = journalQuantity

// CatalogRatedLine retains the measured amount and both accepted rates.
type CatalogRatedLine struct {
	Dimension      string                 `json:"dimension"`
	Component      string                 `json:"component"`
	Quantity       string                 `json:"quantity"`
	QuantityUnit   string                 `json:"quantity_unit"`
	ProviderRate   CatalogDecimal         `json:"provider_rate"`
	CustomerRate   ExactMoney             `json:"customer_rate"`
	RateUnit       string                 `json:"rate_unit"`
	Conditions     CatalogPriceConditions `json:"conditions"`
	ProviderCost   ExactMoney             `json:"provider_cost"`
	CustomerCharge ExactMoney             `json:"customer_charge"`
}

// CatalogRatedUsage keeps unresolved evidence distinct from a zero charge.
type CatalogRatedUsage struct {
	State                string             `json:"state"`
	Lines                []CatalogRatedLine `json:"lines"`
	ProviderCost         ExactMoney         `json:"provider_cost"`
	CustomerCharge       ExactMoney         `json:"customer_charge"`
	MinimumAdjustment    ExactMoney         `json:"minimum_adjustment"`
	UnresolvedDimensions []string           `json:"unresolved_dimensions"`
}

type catalogRatingUnit struct {
	quantity string
	divisor  int64
}

var catalogRatingUnits = map[string]catalogRatingUnit{
	"USD/1M_tokens":      {"token", 1000000},
	"USD/minute":         {"second", 60},
	"USD/hour":           {"second", 3600},
	"USD/1M_token_hours": {"token_second", 3600000000},
	"USD/output_second":  {"second", 1},
	"USD/input_image":    {"image", 1},
	"USD/second":         {"second", 1},
	"USD/call":           {"call", 1},
	"USD/provider_unit":  {"provider_unit", 1},
}

type catalogSnapshotComponent struct {
	binding CatalogRateBinding
	rates   []CatalogPriceRate
	unit    catalogRatingUnit
}

type catalogSelectedRate struct {
	binding CatalogRateBinding
	rate    CatalogPriceRate
	unit    catalogRatingUnit
}

// CatalogRatingSnapshot owns immutable copies of the selected catalog rates.
// Durable storage binds this value to an accepted request before dispatch.
type CatalogRatingSnapshot struct {
	markup             *big.Rat
	revision           string
	descriptor         CatalogPriceDescriptor
	acceptedAt         time.Time
	components         []catalogSnapshotComponent
	excludedComponents []string
	zeroDimensions     []string
}

// NewRatingSnapshot selects every required component from one catalog revision.
// Missing quantities and unsupported units cannot create a paid route.
func (service CatalogService) NewRatingSnapshot(provider, model, operation string, acceptedAt time.Time, bindings []CatalogRateBinding) (*CatalogRatingSnapshot, error) {
	return service.newRatingSnapshot(provider, model, operation, acceptedAt, bindings, nil, nil)
}

func (service CatalogService) newRatingSnapshot(provider, model, operation string, acceptedAt time.Time, bindings []CatalogRateBinding, excludedComponents, zeroDimensions []string) (*CatalogRatingSnapshot, error) {
	descriptor, found := service.catalog.prices[catalogPriceIdentifier(provider, model, operation)]
	if !found || !descriptor.Available {
		return nil, fmt.Errorf("%w: price unavailable", ErrCatalogRatingUnavailable)
	}
	if acceptedAt.IsZero() {
		return nil, fmt.Errorf("%w: acceptance time required", ErrCatalogRatingInvalid)
	}
	snapshot := &CatalogRatingSnapshot{revision: service.Revision(), descriptor: cloneCatalogPriceDescriptor(descriptor), markup: big.NewRat(13, 10), acceptedAt: acceptedAt.UTC(), excludedComponents: slices.Clone(excludedComponents), zeroDimensions: slices.Clone(zeroDimensions)}
	dimensions, selected, components := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, binding := range bindings {
		if err := claimRatingDimensions(binding, dimensions); err != nil {
			return nil, err
		}
		if binding.Conditions != categoricalPriceConditions(binding.Conditions) {
			return nil, fmt.Errorf("%w: invalid dimension or categorical conditions", ErrCatalogRatingInvalid)
		}
		identity := fmt.Sprintf("%s\x00%#v", binding.Component, binding.Conditions)
		if selected[identity] {
			return nil, fmt.Errorf("%w: duplicate selected component", ErrCatalogRatingInvalid)
		}
		component := catalogSnapshotComponent{binding: binding}
		for _, rate := range descriptor.Rates {
			if rate.Component != binding.Component || categoricalPriceConditions(rate.Conditions) != binding.Conditions || !catalogPriceActiveAt(rate.Conditions, acceptedAt) {
				continue
			}
			if rate.Conditions.InputTokens.UnresolvedReason != "" {
				return nil, fmt.Errorf("%w: unresolved token boundary", ErrCatalogRatingUnavailable)
			}
			unit, known := catalogRatingUnits[rate.Unit]
			if !known {
				return nil, fmt.Errorf("%w: unit=%s", ErrCatalogRatingUnavailable, rate.Unit)
			}
			if len(component.rates) != 0 && rate.Unit != component.rates[0].Unit {
				return nil, fmt.Errorf("%w: component units differ", ErrCatalogRatingUnavailable)
			}
			component.unit = unit
			component.rates = append(component.rates, rate)
		}
		if len(component.rates) == 0 {
			return nil, fmt.Errorf("%w: component=%s", ErrCatalogRatingUnavailable, binding.Component)
		}
		snapshot.components = append(snapshot.components, component)
		selected[identity], components[binding.Component] = true, true
	}
	for _, rate := range descriptor.Rates {
		if !components[rate.Component] {
			if slices.Contains(excludedComponents, rate.Component) && operation == ModelOperationText &&
				((rate.Component == "cache_storage" && rate.Unit == "USD/1M_token_hours") || (rate.Component == "web_search_calls" && rate.Unit == "USD/call")) {
				continue
			}
			return nil, fmt.Errorf("%w: missing component=%s", ErrCatalogRatingUnavailable, rate.Component)
		}
	}
	if err := validateRatingRules(excludedComponents, zeroDimensions, dimensions, components); err != nil {
		return nil, err
	}
	if descriptor.MinimumCharge != nil && descriptor.MinimumCharge.Unit != "USD/request" {
		return nil, fmt.Errorf("%w: minimum charge unit=%s", ErrCatalogRatingUnavailable, descriptor.MinimumCharge.Unit)
	}
	return snapshot, nil
}

// Rate calculates one attempt with exact arithmetic and the approved 30% markup.
// Priced children replace their portion of an inclusive parent quantity.
func (snapshot *CatalogRatingSnapshot) Rate(input []CatalogUsageQuantity) (CatalogRatedUsage, error) {
	if snapshot == nil || len(snapshot.components) == 0 {
		return CatalogRatedUsage{}, fmt.Errorf("%w: snapshot required", ErrCatalogRatingInvalid)
	}
	measured, err := ratingMeasurements(input)
	if err != nil {
		return CatalogRatedUsage{}, err
	}
	if err := snapshot.combineRatingDimensions(measured); err != nil {
		return CatalogRatedUsage{}, err
	}
	result := CatalogRatedUsage{State: CatalogRatingResolved, Lines: []CatalogRatedLine{}, UnresolvedDimensions: []string{}}
	for _, dimension := range snapshot.zeroDimensions {
		quantity, found := measured[dimension]
		if !found || quantity.UnknownReason != "" || quantity.Value != "0" {
			appendRatingUnresolved(&result, dimension)
		}
	}
	selected := make([]catalogSelectedRate, 0, len(snapshot.components))
	for _, component := range snapshot.components {
		quantity, found := measured[component.binding.Dimension]
		if !found || quantity.UnknownReason != "" {
			appendRatingUnresolved(&result, component.binding.Dimension)
			continue
		}
		if quantity.Unit != component.unit.quantity {
			return CatalogRatedUsage{}, fmt.Errorf("%w: dimension=%s incompatible unit=%s", ErrCatalogRatingInvalid, quantity.Dimension, quantity.Unit)
		}
		rate, found := selectMeasuredCatalogRate(component, measured)
		if !found {
			appendRatingUnresolved(&result, component.binding.Dimension)
			continue
		}
		selected = append(selected, catalogSelectedRate{binding: component.binding, rate: rate, unit: component.unit})
	}
	if len(result.UnresolvedDimensions) != 0 {
		result.State = CatalogRatingUnresolved
		return result, nil
	}
	return snapshot.rateSelectedMeasurements(measured, selected)
}

func ratingMeasurements(input []CatalogUsageQuantity) (map[string]journalQuantity, error) {
	quantities := slices.Clone(input)
	for _, quantity := range quantities {
		if len(quantity.Value) > catalogDecimalMaximumLength {
			return nil, fmt.Errorf("%w: quantity length", ErrCatalogRatingInvalid)
		}
	}
	if _, err := normalizeJournalQuantities(quantities); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCatalogRatingInvalid, err)
	}
	measured := make(map[string]journalQuantity, len(quantities))
	for _, quantity := range quantities {
		if quantity.Unit == "token" && quantity.Value != "" && !ratingRational(quantity.Value).IsInt() {
			return nil, fmt.Errorf("%w: fractional token count", ErrCatalogRatingInvalid)
		}
		measured[quantity.Dimension] = quantity
	}
	return measured, nil
}

func claimRatingDimensions(binding CatalogRateBinding, claimed map[string]bool) error {
	for index, dimension := range []string{binding.Dimension, binding.AdditionalDimension} {
		if index == 1 && dimension == "" {
			continue
		}
		if !journalDimensionPattern.MatchString(dimension) || claimed[dimension] {
			return fmt.Errorf("%w: invalid or repeated billing dimension=%s", ErrCatalogRatingInvalid, dimension)
		}
		claimed[dimension] = true
	}
	return nil
}

func validateRatingRules(excluded, zero []string, dimensions, components map[string]bool) error {
	seen := map[string]bool{}
	for _, component := range excluded {
		if (component != "cache_storage" && component != "web_search_calls") || components[component] || seen[component] {
			return fmt.Errorf("%w: invalid excluded component=%s", ErrCatalogRatingInvalid, component)
		}
		seen[component] = true
	}
	for _, dimension := range zero {
		if !journalDimensionPattern.MatchString(dimension) || dimensions[dimension] {
			return fmt.Errorf("%w: invalid zero quantity condition=%s", ErrCatalogRatingInvalid, dimension)
		}
		dimensions[dimension] = true
	}
	return nil
}

// Combine disjoint native roots in the calculation only. The original journal
// quantities stay unchanged, and the snapshot retains the composition rule.
func (snapshot *CatalogRatingSnapshot) combineRatingDimensions(measured map[string]journalQuantity) error {
	for _, component := range snapshot.components {
		binding := component.binding
		if binding.AdditionalDimension == "" {
			continue
		}
		primary, found := measured[binding.Dimension]
		if !found || primary.UnknownReason != "" {
			continue
		}
		additional, found := measured[binding.AdditionalDimension]
		if !found || additional.UnknownReason != "" {
			primary.Value, primary.UnknownReason = "", journalQuantityNotReported
			if found {
				primary.UnknownReason = additional.UnknownReason
			}
			measured[binding.Dimension] = primary
			continue
		}
		if primary.IncludedIn != "" || additional.IncludedIn != "" || primary.Unit != additional.Unit {
			return fmt.Errorf("%w: compound quantities must be disjoint roots with the same unit", ErrCatalogRatingInvalid)
		}
		primary.Value = ratingDecimal(new(big.Rat).Add(ratingRational(primary.Value), ratingRational(additional.Value)))
		additional.IncludedIn = binding.Dimension
		measured[binding.Dimension], measured[binding.AdditionalDimension] = primary, additional
	}
	return nil
}

func appendRatingUnresolved(result *CatalogRatedUsage, dimension string) {
	if !slices.Contains(result.UnresolvedDimensions, dimension) {
		result.UnresolvedDimensions = append(result.UnresolvedDimensions, dimension)
	}
}

func selectMeasuredCatalogRate(component catalogSnapshotComponent, measured map[string]journalQuantity) (CatalogPriceRate, bool) {
	for _, rate := range component.rates {
		interval := rate.Conditions.InputTokens
		if interval.Minimum == 0 && interval.MaximumExclusive == 0 {
			return rate, true
		}
		input, found := measured["input_tokens"]
		if !found || input.UnknownReason != "" || input.Unit != "token" {
			return CatalogPriceRate{}, false
		}
		value := ratingRational(input.Value)
		if value.Cmp(new(big.Rat).SetInt(new(big.Int).SetUint64(interval.Minimum))) >= 0 && (interval.MaximumExclusive == 0 || value.Cmp(new(big.Rat).SetInt(new(big.Int).SetUint64(interval.MaximumExclusive))) < 0) {
			return rate, true
		}
	}
	return CatalogPriceRate{}, false
}

func (snapshot *CatalogRatingSnapshot) rateSelectedMeasurements(measured map[string]journalQuantity, selected []catalogSelectedRate) (CatalogRatedUsage, error) {
	result := CatalogRatedUsage{State: CatalogRatingResolved, Lines: []CatalogRatedLine{}, UnresolvedDimensions: []string{}}
	priced := make(map[string]bool, len(selected))
	for _, entry := range selected {
		priced[entry.binding.Dimension] = true
	}
	providerTotal := new(big.Rat)
	for _, entry := range selected {
		quantity := measured[entry.binding.Dimension]
		amount := ratingRational(quantity.Value)
		for _, child := range selected {
			childDimension := child.binding.Dimension
			for parent := measured[childDimension].IncludedIn; parent != ""; parent = measured[parent].IncludedIn {
				if parent == quantity.Dimension {
					amount.Sub(amount, ratingRational(measured[childDimension].Value))
					break
				}
				if priced[parent] {
					break
				}
			}
		}
		if amount.Sign() < 0 {
			return CatalogRatedUsage{}, fmt.Errorf("%w: inclusive children exceed dimension=%s", ErrCatalogRatingInvalid, quantity.Dimension)
		}
		rate := ratingRational(string(entry.rate.Rate))
		cost := new(big.Rat).Mul(amount, rate)
		cost.Quo(cost, new(big.Rat).SetInt64(entry.unit.divisor))
		providerTotal.Add(providerTotal, cost)
		result.Lines = append(result.Lines, CatalogRatedLine{
			Dimension: quantity.Dimension, Component: entry.rate.Component, Quantity: ratingDecimal(amount), QuantityUnit: quantity.Unit,
			ProviderRate: entry.rate.Rate, CustomerRate: ratingMoney(snapshot.customerAmount(rate)), RateUnit: entry.rate.Unit, Conditions: entry.rate.Conditions,
			ProviderCost: ratingMoney(cost), CustomerCharge: ratingMoney(snapshot.customerAmount(cost)),
		})
	}
	minimumAdjustment := new(big.Rat)
	if minimum := snapshot.descriptor.MinimumCharge; minimum != nil {
		required := ratingRational(string(minimum.Amount))
		if providerTotal.Cmp(required) < 0 {
			minimumAdjustment.Sub(required, providerTotal)
			providerTotal.Set(required)
		}
	}
	result.ProviderCost = ratingMoney(providerTotal)
	result.CustomerCharge = ratingMoney(snapshot.customerAmount(providerTotal))
	result.MinimumAdjustment = ratingMoney(minimumAdjustment)
	return result, nil
}

func ratingRational(value string) *big.Rat { amount, _ := new(big.Rat).SetString(value); return amount }
func (snapshot *CatalogRatingSnapshot) customerAmount(value *big.Rat) *big.Rat {
	return new(big.Rat).Mul(value, snapshot.markup)
}
func ratingMoney(value *big.Rat) ExactMoney {
	return ExactMoney{Numerator: value.Num().String(), Denominator: value.Denom().String()}
}
func ratingDecimal(value *big.Rat) string {
	// Subtraction of finite decimal quantities remains a finite decimal.
	places, _ := value.FloatPrec()
	return normalizeJournalDecimal(value.FloatString(places))
}

// CatalogUsageBound declares an upper limit for one billable dimension.
type CatalogUsageBound struct {
	Dimension string `json:"dimension"`
	Unit      string `json:"unit"`
	Maximum   string `json:"maximum"`
}

// CatalogAuthorizedMaximum is the monetary limit for a bounded set of attempts.
type CatalogAuthorizedMaximum struct {
	Attempts       uint32     `json:"attempts"`
	CustomerCharge ExactMoney `json:"customer_charge"`
	ReservedCents  int64      `json:"reserved_cents,string"`
}

// MaximumCharge includes every component and each authorized attempt.
// Upper bounds never establish cache hits or other inclusive discounts.
func (snapshot *CatalogRatingSnapshot) MaximumCharge(bounds []CatalogUsageBound, attempts uint32) (CatalogAuthorizedMaximum, error) {
	if attempts == 0 || snapshot == nil || len(snapshot.components) == 0 {
		return CatalogAuthorizedMaximum{}, fmt.Errorf("%w: snapshot and positive attempt limit required", ErrCatalogRatingInvalid)
	}
	quantities := make([]CatalogUsageQuantity, len(bounds))
	for index, bound := range bounds {
		quantities[index] = CatalogUsageQuantity{Dimension: bound.Dimension, Unit: bound.Unit, Value: bound.Maximum}
	}
	measured, err := ratingMeasurements(quantities)
	if err != nil {
		return CatalogAuthorizedMaximum{}, err
	}
	if err := snapshot.combineRatingDimensions(measured); err != nil {
		return CatalogAuthorizedMaximum{}, err
	}
	for _, dimension := range snapshot.zeroDimensions {
		quantity, found := measured[dimension]
		if !found || quantity.Value != "0" {
			return CatalogAuthorizedMaximum{}, fmt.Errorf("%w: dimension=%s must remain zero", ErrCatalogRatingUnavailable, dimension)
		}
	}
	selected := make([]catalogSelectedRate, 0, len(snapshot.components))
	for _, component := range snapshot.components {
		bound, found := measured[component.binding.Dimension]
		if !found || bound.UnknownReason != "" {
			return CatalogAuthorizedMaximum{}, fmt.Errorf("%w: unbounded usage", ErrCatalogRatingUnavailable)
		}
		if bound.Unit != component.unit.quantity {
			return CatalogAuthorizedMaximum{}, fmt.Errorf("%w: incompatible bound unit", ErrCatalogRatingInvalid)
		}
		rate, err := maximumCatalogRate(component, measured)
		if err != nil {
			return CatalogAuthorizedMaximum{}, err
		}
		selected = append(selected, catalogSelectedRate{binding: component.binding, rate: rate, unit: component.unit})
	}
	rated, err := snapshot.rateSelectedMeasurements(measured, selected)
	if err != nil {
		return CatalogAuthorizedMaximum{}, err
	}
	maximum, err := parseExactMoney(rated.CustomerCharge)
	if err != nil {
		return CatalogAuthorizedMaximum{}, err
	}
	maximum.Mul(maximum, new(big.Rat).SetInt64(int64(attempts)))
	exact := ratingMoney(maximum)
	reserved, err := ReserveUSDCents(exact)
	if err != nil {
		return CatalogAuthorizedMaximum{}, err
	}
	return CatalogAuthorizedMaximum{Attempts: attempts, CustomerCharge: exact, ReservedCents: reserved}, nil
}

func maximumCatalogRate(component catalogSnapshotComponent, measured map[string]journalQuantity) (CatalogPriceRate, error) {
	rates := slices.Clone(component.rates)
	slices.SortFunc(rates, func(left, right CatalogPriceRate) int {
		return new(big.Int).SetUint64(left.Conditions.InputTokens.Minimum).Cmp(new(big.Int).SetUint64(right.Conditions.InputTokens.Minimum))
	})
	chosen := rates[0]
	maximumInput := new(big.Rat)
	if len(rates) > 1 || rates[0].Conditions.InputTokens != (CatalogTokenRange{}) {
		input, found := measured["input_tokens"]
		if !found || input.Value == "" || input.Unit != "token" {
			return CatalogPriceRate{}, fmt.Errorf("%w: input tier bound required", ErrCatalogRatingUnavailable)
		}
		maximumInput = ratingRational(input.Value)
	}
	covered := new(big.Int)
	for _, rate := range rates {
		interval := rate.Conditions.InputTokens
		minimum := new(big.Int).SetUint64(interval.Minimum)
		if new(big.Rat).SetInt(minimum).Cmp(maximumInput) > 0 {
			break
		}
		if minimum.Cmp(covered) > 0 {
			return CatalogPriceRate{}, fmt.Errorf("%w: gap in token tier schedule", ErrCatalogRatingUnavailable)
		}
		if ratingRational(string(rate.Rate)).Cmp(ratingRational(string(chosen.Rate))) > 0 {
			chosen = rate
		}
		if interval.MaximumExclusive == 0 {
			return chosen, nil
		}
		covered.SetUint64(interval.MaximumExclusive)
		if new(big.Rat).SetInt(covered).Cmp(maximumInput) > 0 {
			return chosen, nil
		}
	}
	return CatalogPriceRate{}, fmt.Errorf("%w: incomplete token tier schedule", ErrCatalogRatingUnavailable)
}
