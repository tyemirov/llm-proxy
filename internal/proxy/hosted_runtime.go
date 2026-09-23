package proxy

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"gorm.io/gorm"
)

// HostedConfiguration explicitly selects offerings for funded execution.
// Omission keeps hosted dispatch disabled. Rates remain in the provider catalog.
type HostedConfiguration struct {
	Offerings []HostedOfferingConfiguration `mapstructure:"offerings"`
}

func (settings *hostedRuntimeSettings) authorizeCompletion(transaction *gorm.DB, request managedJournalRequestRecord, intent hostedCompletionIntent) error {
	scope, exists := settings.offerings[hostedOfferingKey{request.Provider, request.Model, request.Operation}]
	if !exists {
		return errHostedAuthorityDenied
	}
	var reserve journalReservation
	var err error
	if request.Operation == ModelOperationText {
		input := chatRequestParameters{provider: intent.provider, model: textModelDefinition{identifier: intent.model}, maxTokens: intent.maxTokens, webSearchEnabled: intent.webSearch}
		reserve, err = newHostedTextPriceAdmission(settings.catalog, input, request.CreatedAt, scope.Conditions, scope.MaximumAttempts)
	} else {
		reserve, err = newHostedMediaPriceAdmission(settings.catalog, request, intent.provider.activeTransport.requestCodec, scope.Conditions, scope.MaximumAttempts)
	}
	if err != nil {
		return fmt.Errorf("%w: select hosted request price: %w", errFinancialAdmissionUnavailable, err)
	}
	return newHostedFundsAdmission(reserve)(transaction, request)
}

// HostedOfferingConfiguration declares the operator-selected financial scope.
type HostedOfferingConfiguration struct {
	Provider        string                 `mapstructure:"provider"`
	Model           string                 `mapstructure:"model"`
	Operation       string                 `mapstructure:"operation"`
	MaximumAttempts uint32                 `mapstructure:"maximum_attempts"`
	Conditions      CatalogPriceConditions `mapstructure:"conditions"`
}

type hostedOfferingKey struct{ provider, model, operation string }

type hostedRuntimeSettings struct {
	catalog   CatalogService
	offerings map[hostedOfferingKey]HostedOfferingConfiguration
}

func newHostedRuntimeSettings(input *HostedConfiguration, catalog ModelCatalog) (*hostedRuntimeSettings, error) {
	if input == nil {
		return nil, nil
	}
	if len(input.Offerings) == 0 {
		return nil, fmt.Errorf("configure hosted execution: explicit offering scopes are required")
	}
	prices, err := NewCatalogService(catalog)
	if err != nil {
		return nil, fmt.Errorf("configure hosted price catalog: %w", err)
	}
	settings := &hostedRuntimeSettings{catalog: prices, offerings: make(map[hostedOfferingKey]HostedOfferingConfiguration, len(input.Offerings))}
	for _, offering := range input.Offerings {
		key := hostedOfferingKey{offering.Provider, offering.Model, offering.Operation}
		if offering.Provider != strings.TrimSpace(offering.Provider) || offering.Model != strings.TrimSpace(offering.Model) {
			return nil, fmt.Errorf("configure hosted execution: scope %v requires canonical identifiers", key)
		}
		if _, exists := settings.offerings[key]; exists {
			return nil, fmt.Errorf("configure hosted execution: duplicate offering scope %v", key)
		}
		if offering.MaximumAttempts == 0 || offering.Conditions != categoricalPriceConditions(offering.Conditions) || offering.Conditions.CacheClass != "" {
			return nil, fmt.Errorf("configure hosted execution: scope %v requires positive attempts and categorical service conditions", key)
		}
		if err := validateCatalogPriceConditions(offering.Conditions); err != nil {
			return nil, fmt.Errorf("configure hosted conditions for scope %v: %w", key, err)
		}
		if offering.Model == "" {
			if _, err := prices.ResolveService(offering.Provider, offering.Operation); err != nil {
				return nil, fmt.Errorf("configure hosted service scope: %w", err)
			}
		} else {
			resolved, err := prices.ResolveOffering(offering.Provider, offering.Model)
			if err != nil {
				return nil, fmt.Errorf("configure hosted offering scope: %w", err)
			}
			if !slices.Contains(resolved.Operations, offering.Operation) {
				return nil, fmt.Errorf("configure hosted execution: unsupported operation in scope %v", key)
			}
		}
		settings.offerings[key] = offering
	}
	return settings, nil
}

func (settings *hostedRuntimeSettings) mediaAdmission(providers *providerRegistry) hostedMediaReservation {
	return func(transaction *gorm.DB, request managedJournalRequestRecord, operation mediaOperationRecord) error {
		scope, exists := settings.offerings[hostedOfferingKey{request.Provider, request.Model, request.Operation}]
		if !exists {
			return errHostedAuthorityDenied
		}
		var transport string
		if request.Model == "" {
			route, err := settings.catalog.ResolveService(request.Provider, request.Operation)
			if err != nil {
				return fmt.Errorf("%w: resolve hosted service: %w", errFinancialAdmissionUnavailable, err)
			}
			transport = route.Transport
		} else {
			route, err := settings.catalog.ResolveOffering(request.Provider, request.Model)
			if err != nil {
				return fmt.Errorf("%w: resolve hosted media offering: %w", errFinancialAdmissionUnavailable, err)
			}
			transport = route.Transport
		}
		codec := providers.definitions[providerID(request.Provider)].transports[transport].requestCodec
		if err := matchHostedMediaPriceConditions(codec, scope.Conditions, operation); err != nil {
			return fmt.Errorf("%w: select hosted media conditions: %w", errFinancialAdmissionUnavailable, err)
		}
		reserve, err := newHostedMediaPriceAdmission(settings.catalog, request, codec, scope.Conditions, scope.MaximumAttempts)
		if err != nil {
			return fmt.Errorf("%w: select hosted media price: %w", errFinancialAdmissionUnavailable, err)
		}
		return newHostedFundsAdmission(reserve)(transaction, request)
	}
}

func matchHostedMediaPriceConditions(codec string, conditions CatalogPriceConditions, operation mediaOperationRecord) error {
	// Deployment conditions describe the selected provider account. Request
	// conditions must match the normalized intent before a price can be used.
	conditions.BillingMode, conditions.ServiceTier, conditions.Region = "", "", ""
	if codec == CatalogProtocolOpenAIImages {
		var controls struct {
			Quality string `json:"quality"`
			Size    string `json:"size"`
		}
		if err := json.Unmarshal(operation.NormalizedControls, &controls); err != nil {
			return fmt.Errorf("decode retained media controls: %w", err)
		}
		if (conditions.Quality != "" && conditions.Quality != controls.Quality) || (conditions.Resolution != "" && conditions.Resolution != controls.Size) {
			return fmt.Errorf("%w: request differs from selected image price conditions", ErrCatalogRatingUnavailable)
		}
		conditions.Quality, conditions.Resolution = "", ""
	}
	if conditions != (CatalogPriceConditions{}) {
		return fmt.Errorf("%w: request condition mapping unavailable for codec=%s", ErrCatalogRatingUnavailable, codec)
	}
	return nil
}
