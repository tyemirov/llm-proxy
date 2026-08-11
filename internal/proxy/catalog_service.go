package proxy

import (
	"fmt"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

const (
	// CatalogControlEnum identifies a control with an exact value vocabulary.
	CatalogControlEnum = "enum"
	// CatalogControlInteger identifies a bounded integer control.
	CatalogControlInteger = "integer"
	// CatalogControlBoolean identifies a Boolean control.
	CatalogControlBoolean = "boolean"
	// CatalogCurrencyUSD identifies published United States dollar prices.
	CatalogCurrencyUSD = "USD"
)

// CatalogControl declares one route-specific request control.
type CatalogControl struct {
	ID               string   `json:"id" mapstructure:"id"`
	Kind             string   `json:"kind" mapstructure:"kind"`
	Values           []string `json:"values" mapstructure:"values"`
	Minimum          *int     `json:"minimum" mapstructure:"minimum"`
	Maximum          *int     `json:"maximum" mapstructure:"maximum"`
	AccountDependent bool     `json:"account_dependent" mapstructure:"account_dependent"`
}

// CatalogLimit declares one fixed or account-dependent route limit.
type CatalogLimit struct {
	ID               string `json:"id" mapstructure:"id"`
	Value            *int   `json:"value" mapstructure:"value"`
	Unit             string `json:"unit" mapstructure:"unit"`
	AccountDependent bool   `json:"account_dependent" mapstructure:"account_dependent"`
}

// CatalogPriceConditions identifies one exact published billing condition set.
type CatalogPriceConditions struct {
	Resolution     string `json:"resolution" mapstructure:"resolution"`
	GeneratedAudio string `json:"generated_audio" mapstructure:"generated_audio"`
	InputMedia     string `json:"input_media" mapstructure:"input_media"`
	OutputMedia    string `json:"output_media" mapstructure:"output_media"`
	Duration       string `json:"duration" mapstructure:"duration"`
	Quantity       string `json:"quantity" mapstructure:"quantity"`
	Quality        string `json:"quality" mapstructure:"quality"`
	Mode           string `json:"mode" mapstructure:"mode"`
	APIVersion     string `json:"api_version" mapstructure:"api_version"`
	AvatarType     string `json:"avatar_type" mapstructure:"avatar_type"`
	BillingMode    string `json:"billing_mode" mapstructure:"billing_mode"`
	BillingOutcome string `json:"billing_outcome" mapstructure:"billing_outcome"`
}

// CatalogPriceRate is one exact published billing component.
type CatalogPriceRate struct {
	Component  string                 `json:"component" mapstructure:"component"`
	Currency   string                 `json:"currency" mapstructure:"currency"`
	Rate       float64                `json:"rate" mapstructure:"rate"`
	Unit       string                 `json:"unit" mapstructure:"unit"`
	Conditions CatalogPriceConditions `json:"conditions" mapstructure:"conditions"`
}

// CatalogMinimumCharge declares one published request minimum.
type CatalogMinimumCharge struct {
	Currency string  `json:"currency" mapstructure:"currency"`
	Amount   float64 `json:"amount" mapstructure:"amount"`
	Unit     string  `json:"unit" mapstructure:"unit"`
}

// CatalogPriceDescriptor owns pricing for one provider, model, and operation.
type CatalogPriceDescriptor struct {
	Provider          string                `json:"provider" mapstructure:"provider"`
	Model             string                `json:"model" mapstructure:"model"`
	Operation         string                `json:"operation" mapstructure:"operation"`
	Available         bool                  `json:"available" mapstructure:"available"`
	Rates             []CatalogPriceRate    `json:"rates" mapstructure:"rates"`
	MinimumCharge     *CatalogMinimumCharge `json:"minimum_charge" mapstructure:"minimum_charge"`
	Source            string                `json:"source" mapstructure:"source"`
	LastVerified      string                `json:"last_verified" mapstructure:"last_verified"`
	UnavailableReason string                `json:"unavailable_reason" mapstructure:"unavailable_reason"`
}

// CatalogPriceSelection is an exact price result. Unavailable selections carry
// a stable reason and never guess a rate.
type CatalogPriceSelection struct {
	Available         bool
	Rate              *CatalogPriceRate
	MinimumCharge     *CatalogMinimumCharge
	Source            string
	LastVerified      string
	UnavailableReason string
}

// CatalogService is the validated model-operation capability and price
// snapshot used by routing, public discovery, management, and later planning.
type CatalogService struct {
	catalog validatedModelCatalog
}

// NewCatalogService validates one complete catalog snapshot.
func NewCatalogService(catalog ModelCatalog) (CatalogService, error) {
	validated, validationError := validateModelCatalog(cloneModelCatalog(catalog))
	if validationError != nil {
		return CatalogService{}, validationError
	}
	return CatalogService{catalog: validated}, nil
}

// Revision returns the exact catalog snapshot identifier.
func (service CatalogService) Revision() string {
	return service.catalog.revision
}

// ResolveOffering resolves one canonical provider and model pair.
func (service CatalogService) ResolveOffering(provider string, model string) (ProviderOffering, error) {
	offeringIdentifier := providerOfferingIdentifier(strings.TrimSpace(provider), strings.TrimSpace(model))
	offering, found := service.catalog.offerings[offeringIdentifier]
	if !found {
		return ProviderOffering{}, fmt.Errorf("%w: route=%s reason=unknown", ErrInvalidModelCatalog, offeringIdentifier)
	}
	return cloneProviderOffering(offering), nil
}

// SelectPrice returns the exact component and condition match. Missing or
// incomplete selections return a typed unavailable result.
func (service CatalogService) SelectPrice(provider string, model string, operation string, component string, conditions CatalogPriceConditions) CatalogPriceSelection {
	descriptor, found := service.catalog.prices[catalogPriceIdentifier(provider, model, operation)]
	if !found {
		return CatalogPriceSelection{UnavailableReason: "price_not_cataloged"}
	}
	selection := CatalogPriceSelection{
		Source:            descriptor.Source,
		LastVerified:      descriptor.LastVerified,
		UnavailableReason: descriptor.UnavailableReason,
	}
	if descriptor.MinimumCharge != nil {
		minimumCharge := *descriptor.MinimumCharge
		selection.MinimumCharge = &minimumCharge
	}
	if !descriptor.Available {
		return selection
	}
	component = strings.TrimSpace(component)
	for rateIndex := range descriptor.Rates {
		rate := descriptor.Rates[rateIndex]
		if rate.Component == component && rate.Conditions == conditions {
			selectedRate := rate
			selection.Available = true
			selection.Rate = &selectedRate
			selection.UnavailableReason = constants.EmptyString
			return selection
		}
	}
	selection.UnavailableReason = "exact_price_unavailable"
	return selection
}

func cloneModelCatalog(catalog ModelCatalog) ModelCatalog {
	cloned := catalog
	cloned.Operations = make([]ModelOperationKind, len(catalog.Operations))
	for operationIndex, operation := range catalog.Operations {
		cloned.Operations[operationIndex] = operation
		cloned.Operations[operationIndex].InputArtifacts = append([]string(nil), operation.InputArtifacts...)
		cloned.Operations[operationIndex].OutputArtifacts = append([]string(nil), operation.OutputArtifacts...)
	}
	cloned.Providers = make([]CatalogProvider, len(catalog.Providers))
	for providerIndex, provider := range catalog.Providers {
		cloned.Providers[providerIndex] = provider
		cloned.Providers[providerIndex].CredentialKinds = append([]string(nil), provider.CredentialKinds...)
	}
	cloned.Publishers = append([]ModelPublisher(nil), catalog.Publishers...)
	cloned.Families = append([]ModelFamily(nil), catalog.Families...)
	cloned.Models = make([]ExactModel, len(catalog.Models))
	for modelIndex, model := range catalog.Models {
		cloned.Models[modelIndex] = model
		cloned.Models[modelIndex].Operations = append([]string(nil), model.Operations...)
		cloned.Models[modelIndex].MediaInputs = append([]string(nil), model.MediaInputs...)
	}
	cloned.Offerings = make([]ProviderOffering, len(catalog.Offerings))
	for offeringIndex, offering := range catalog.Offerings {
		cloned.Offerings[offeringIndex] = cloneProviderOffering(offering)
	}
	cloned.Prices = make([]CatalogPriceDescriptor, len(catalog.Prices))
	for priceIndex, price := range catalog.Prices {
		cloned.Prices[priceIndex] = cloneCatalogPriceDescriptor(price)
	}
	return cloned
}

func cloneProviderOffering(offering ProviderOffering) ProviderOffering {
	cloned := offering
	cloned.Operations = append([]string(nil), offering.Operations...)
	cloned.DefaultOperations = append([]string(nil), offering.DefaultOperations...)
	cloned.MediaInputs = append([]string(nil), offering.MediaInputs...)
	if offering.ReasoningEffort != nil {
		cloned.ReasoningEffort = &ReasoningEffortCapability{
			Adapter: offering.ReasoningEffort.Adapter,
			Efforts: append([]string(nil), offering.ReasoningEffort.Efforts...),
		}
	}
	cloned.Controls = make([]CatalogControl, len(offering.Controls))
	for controlIndex, control := range offering.Controls {
		cloned.Controls[controlIndex] = control
		cloned.Controls[controlIndex].Values = append([]string(nil), control.Values...)
		cloned.Controls[controlIndex].Minimum = cloneInteger(control.Minimum)
		cloned.Controls[controlIndex].Maximum = cloneInteger(control.Maximum)
	}
	cloned.Limits = make([]CatalogLimit, len(offering.Limits))
	for limitIndex, limit := range offering.Limits {
		cloned.Limits[limitIndex] = limit
		cloned.Limits[limitIndex].Value = cloneInteger(limit.Value)
	}
	return cloned
}

func cloneCatalogPriceDescriptor(descriptor CatalogPriceDescriptor) CatalogPriceDescriptor {
	cloned := descriptor
	cloned.Rates = append([]CatalogPriceRate(nil), descriptor.Rates...)
	if descriptor.MinimumCharge != nil {
		minimumCharge := *descriptor.MinimumCharge
		cloned.MinimumCharge = &minimumCharge
	}
	return cloned
}

func cloneInteger(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func validateCatalogRevision(revision string) error {
	if _, revisionError := canonicalCatalogIdentifier(revision, "catalog.revision"); revisionError != nil {
		return revisionError
	}
	return nil
}

func validateModelOperationKinds(operations []ModelOperationKind, validated map[string]ModelOperationKind) error {
	if len(operations) == 0 {
		return fmt.Errorf("%w: field=catalog.operations", ErrInvalidModelCatalog)
	}
	for operationIndex, operation := range operations {
		identifier, identifierError := canonicalCatalogIdentifier(operation.ID, fmt.Sprintf("catalog.operations[%d].id", operationIndex))
		if identifierError != nil {
			return identifierError
		}
		if identifier != ModelOperationText && identifier != ModelOperationDictation && identifier != ModelOperationVideoGeneration {
			return fmt.Errorf("%w: field=catalog.operations[%d].id operation=%s", ErrInvalidModelCatalog, operationIndex, identifier)
		}
		if _, duplicate := validated[identifier]; duplicate {
			return fmt.Errorf("%w: field=catalog.operations[%d].id duplicate_identifier=%s", ErrInvalidModelCatalog, operationIndex, identifier)
		}
		if artifactError := validateArtifactKinds(operation.InputArtifacts, fmt.Sprintf("catalog.operations[%d].input_artifacts", operationIndex)); artifactError != nil {
			return artifactError
		}
		if artifactError := validateArtifactKinds(operation.OutputArtifacts, fmt.Sprintf("catalog.operations[%d].output_artifacts", operationIndex)); artifactError != nil {
			return artifactError
		}
		validated[identifier] = operation
	}
	for _, requiredOperation := range []string{ModelOperationText, ModelOperationDictation, ModelOperationVideoGeneration} {
		if _, found := validated[requiredOperation]; !found {
			return fmt.Errorf("%w: field=catalog.operations operation=%s reason=missing", ErrInvalidModelCatalog, requiredOperation)
		}
	}
	return nil
}

func validateArtifactKinds(artifacts []string, field string) error {
	if len(artifacts) == 0 {
		return fmt.Errorf("%w: field=%s", ErrInvalidModelCatalog, field)
	}
	seen := map[string]struct{}{}
	for artifactIndex, artifact := range artifacts {
		switch artifact {
		case CatalogArtifactText, CatalogArtifactImage, CatalogArtifactAudio, CatalogArtifactVideo:
		default:
			return fmt.Errorf("%w: field=%s[%d] artifact=%s", ErrInvalidModelCatalog, field, artifactIndex, artifact)
		}
		if _, duplicate := seen[artifact]; duplicate {
			return fmt.Errorf("%w: field=%s[%d] duplicate=%s", ErrInvalidModelCatalog, field, artifactIndex, artifact)
		}
		seen[artifact] = struct{}{}
	}
	return nil
}

func validateCredentialKinds(credentials []string, field string) error {
	if len(credentials) != 1 || credentials[0] != CatalogCredentialAPIKey {
		return fmt.Errorf("%w: field=%s", ErrInvalidModelCatalog, field)
	}
	return nil
}

func validateCatalogControls(controls []CatalogControl, field string) error {
	seen := map[string]struct{}{}
	for controlIndex, control := range controls {
		identifier, identifierError := canonicalCatalogIdentifier(control.ID, fmt.Sprintf("%s[%d].id", field, controlIndex))
		if identifierError != nil {
			return identifierError
		}
		if _, duplicate := seen[identifier]; duplicate {
			return fmt.Errorf("%w: field=%s[%d].id duplicate_identifier=%s", ErrInvalidModelCatalog, field, controlIndex, identifier)
		}
		switch control.Kind {
		case CatalogControlEnum:
			if len(control.Values) == 0 || control.Minimum != nil || control.Maximum != nil {
				return fmt.Errorf("%w: field=%s[%d] kind=%s", ErrInvalidModelCatalog, field, controlIndex, control.Kind)
			}
			values := map[string]struct{}{}
			for valueIndex, value := range control.Values {
				if strings.TrimSpace(value) == constants.EmptyString || value != strings.TrimSpace(value) {
					return fmt.Errorf("%w: field=%s[%d].values[%d]", ErrInvalidModelCatalog, field, controlIndex, valueIndex)
				}
				if _, duplicate := values[value]; duplicate {
					return fmt.Errorf("%w: field=%s[%d].values[%d] duplicate=%s", ErrInvalidModelCatalog, field, controlIndex, valueIndex, value)
				}
				values[value] = struct{}{}
			}
		case CatalogControlInteger:
			if len(control.Values) != 0 || control.Minimum == nil || control.Maximum == nil || *control.Minimum < 0 || *control.Maximum < 0 || *control.Minimum > *control.Maximum {
				return fmt.Errorf("%w: field=%s[%d] kind=%s", ErrInvalidModelCatalog, field, controlIndex, control.Kind)
			}
		case CatalogControlBoolean:
			if len(control.Values) != 0 || control.Minimum != nil || control.Maximum != nil {
				return fmt.Errorf("%w: field=%s[%d] kind=%s", ErrInvalidModelCatalog, field, controlIndex, control.Kind)
			}
		default:
			return fmt.Errorf("%w: field=%s[%d].kind kind=%s", ErrInvalidModelCatalog, field, controlIndex, control.Kind)
		}
		seen[identifier] = struct{}{}
	}
	return nil
}

func validateCatalogLimits(limits []CatalogLimit, field string) error {
	seen := map[string]struct{}{}
	for limitIndex, limit := range limits {
		identifier, identifierError := canonicalCatalogIdentifier(limit.ID, fmt.Sprintf("%s[%d].id", field, limitIndex))
		if identifierError != nil {
			return identifierError
		}
		if _, duplicate := seen[identifier]; duplicate {
			return fmt.Errorf("%w: field=%s[%d].id duplicate_identifier=%s", ErrInvalidModelCatalog, field, limitIndex, identifier)
		}
		if strings.TrimSpace(limit.Unit) == constants.EmptyString || limit.Unit != strings.TrimSpace(limit.Unit) {
			return fmt.Errorf("%w: field=%s[%d].unit", ErrInvalidModelCatalog, field, limitIndex)
		}
		if limit.AccountDependent {
			if limit.Value != nil {
				return fmt.Errorf("%w: field=%s[%d].value reason=account_dependent", ErrInvalidModelCatalog, field, limitIndex)
			}
		} else if limit.Value == nil || *limit.Value <= 0 {
			return fmt.Errorf("%w: field=%s[%d].value", ErrInvalidModelCatalog, field, limitIndex)
		}
		seen[identifier] = struct{}{}
	}
	return nil
}

func validateVideoOffering(offering ProviderOffering, field string) error {
	if offering.Provider != ProviderNameXAI || offering.WireContract != "xai_videos_generations" || offering.ExecutionLifecycle != string(textExecutionLifecyclePollableResource) {
		return fmt.Errorf("%w: field=%s reason=unsupported_video_route", ErrInvalidModelCatalog, field)
	}
	if offering.RequestProfile != constants.EmptyString || offering.WebSearch || offering.OutputTokenLimit != 0 || offering.ReasoningEffort != nil || len(offering.MediaInputs) != 0 {
		return fmt.Errorf("%w: field=%s reason=text_capabilities_on_video_route", ErrInvalidModelCatalog, field)
	}
	if len(offering.Controls) == 0 || len(offering.Limits) == 0 {
		return fmt.Errorf("%w: field=%s reason=incomplete_video_capabilities", ErrInvalidModelCatalog, field)
	}
	return nil
}

func validateCatalogPrices(prices []CatalogPriceDescriptor, catalog validatedModelCatalog) error {
	if len(prices) == 0 {
		return fmt.Errorf("%w: field=catalog.prices", ErrInvalidModelCatalog)
	}
	for priceIndex, descriptor := range prices {
		field := fmt.Sprintf("catalog.prices[%d]", priceIndex)
		offering, found := catalog.offerings[providerOfferingIdentifier(descriptor.Provider, descriptor.Model)]
		if !found || !offeringSupportsOperation(offering, descriptor.Operation) {
			return fmt.Errorf("%w: field=%s route=%s:%s operation=%s reason=dangling_reference", ErrInvalidModelCatalog, field, descriptor.Provider, descriptor.Model, descriptor.Operation)
		}
		identifier := catalogPriceIdentifier(descriptor.Provider, descriptor.Model, descriptor.Operation)
		if _, duplicate := catalog.prices[identifier]; duplicate {
			return fmt.Errorf("%w: field=%s price_conflict=%s", ErrInvalidModelCatalog, field, identifier)
		}
		if !strings.HasPrefix(descriptor.Source, "https://") || descriptor.Source != strings.TrimSpace(descriptor.Source) {
			return fmt.Errorf("%w: field=%s.source", ErrInvalidModelCatalog, field)
		}
		if verifiedAt, parseError := time.Parse(time.DateOnly, descriptor.LastVerified); parseError != nil || verifiedAt.Format(time.DateOnly) != descriptor.LastVerified {
			return fmt.Errorf("%w: field=%s.last_verified", ErrInvalidModelCatalog, field)
		}
		if descriptor.Available {
			if len(descriptor.Rates) == 0 || descriptor.UnavailableReason != constants.EmptyString {
				return fmt.Errorf("%w: field=%s reason=incomplete_available_price", ErrInvalidModelCatalog, field)
			}
		} else if len(descriptor.Rates) != 0 || strings.TrimSpace(descriptor.UnavailableReason) == constants.EmptyString || descriptor.UnavailableReason != strings.TrimSpace(descriptor.UnavailableReason) {
			return fmt.Errorf("%w: field=%s reason=incomplete_unavailable_price", ErrInvalidModelCatalog, field)
		}
		seenRates := map[string]struct{}{}
		for rateIndex, rate := range descriptor.Rates {
			if strings.TrimSpace(rate.Component) == constants.EmptyString || rate.Component != strings.TrimSpace(rate.Component) || rate.Currency != CatalogCurrencyUSD || rate.Rate < 0 || strings.TrimSpace(rate.Unit) == constants.EmptyString || rate.Unit != strings.TrimSpace(rate.Unit) {
				return fmt.Errorf("%w: field=%s.rates[%d]", ErrInvalidModelCatalog, field, rateIndex)
			}
			rateIdentifier := fmt.Sprintf("%s\x00%#v", rate.Component, rate.Conditions)
			if _, duplicate := seenRates[rateIdentifier]; duplicate {
				return fmt.Errorf("%w: field=%s.rates[%d] reason=ambiguous", ErrInvalidModelCatalog, field, rateIndex)
			}
			seenRates[rateIdentifier] = struct{}{}
		}
		if descriptor.MinimumCharge != nil && (descriptor.MinimumCharge.Currency != CatalogCurrencyUSD || descriptor.MinimumCharge.Amount < 0 || strings.TrimSpace(descriptor.MinimumCharge.Unit) == constants.EmptyString || descriptor.MinimumCharge.Unit != strings.TrimSpace(descriptor.MinimumCharge.Unit)) {
			return fmt.Errorf("%w: field=%s.minimum_charge", ErrInvalidModelCatalog, field)
		}
		catalog.prices[identifier] = descriptor
	}
	for _, offering := range catalog.offerings {
		for _, operation := range offering.Operations {
			identifier := catalogPriceIdentifier(offering.Provider, offering.Model, operation)
			if _, found := catalog.prices[identifier]; !found {
				return fmt.Errorf("%w: field=catalog.prices route=%s:%s operation=%s reason=missing", ErrInvalidModelCatalog, offering.Provider, offering.Model, operation)
			}
		}
	}
	return nil
}

func catalogPriceIdentifier(provider string, model string, operation string) string {
	return strings.TrimSpace(provider) + "\x00" + strings.TrimSpace(model) + "\x00" + strings.TrimSpace(operation)
}
