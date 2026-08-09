package proxy

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

var errManagedRoutingDefaultsInvalid = errors.New("managed_routing_defaults_invalid")

type managedRoutingDefaults struct {
	tenantDefaults TenantDefaults
}

type managedRoutingProvider struct {
	definition providerDefinition
	textModel  textModelDefinition
}

func newManagedRoutingDefaults(providers *providerRegistry, rawDefaults TenantDefaults) (managedRoutingDefaults, error) {
	textProvider, textModel, hasTextDefault, textError := resolveManagedTextRoutingDefaultPair(providers, rawDefaults.Provider, rawDefaults.Model)
	if textError != nil {
		return managedRoutingDefaults{}, textError
	}
	dictationProvider, dictationModel, hasDictationDefault, dictationError := resolveManagedDictationRoutingDefaultPair(providers, rawDefaults.DictationProvider, rawDefaults.DictationModel)
	if dictationError != nil {
		return managedRoutingDefaults{}, dictationError
	}
	reasoningEffort := rawDefaults.ReasoningEffort
	if !hasTextDefault {
		if reasoningEffort != constants.EmptyString {
			return managedRoutingDefaults{}, fmt.Errorf("%w: field=reasoning_effort effort=%s reason=text_default_unset", errManagedRoutingDefaultsInvalid, reasoningEffort)
		}
	} else if reasoningEffortError := validateReasoningEffortForResolvedTextRoute(textProvider, textModel, reasoningEffort); reasoningEffortError != nil {
		return managedRoutingDefaults{}, fmt.Errorf("%w: field=reasoning_effort effort=%s: %w", errManagedRoutingDefaultsInvalid, reasoningEffort, reasoningEffortError)
	}
	textProviderIdentifier := constants.EmptyString
	textModelIdentifier := constants.EmptyString
	if hasTextDefault {
		textProviderIdentifier = textProvider.identifier.string()
		textModelIdentifier = textModel.string()
	}
	dictationProviderIdentifier := constants.EmptyString
	dictationModelIdentifier := constants.EmptyString
	if hasDictationDefault {
		dictationProviderIdentifier = dictationProvider.string()
		dictationModelIdentifier = dictationModel.string()
	}
	return managedRoutingDefaults{tenantDefaults: TenantDefaults{
		Provider:          textProviderIdentifier,
		Model:             textModelIdentifier,
		DictationProvider: dictationProviderIdentifier,
		DictationModel:    dictationModelIdentifier,
		SystemPrompt:      rawDefaults.SystemPrompt,
		ReasoningEffort:   reasoningEffort,
	}}, nil
}

func defaultManagedRoutingDefaults() managedRoutingDefaults {
	return managedRoutingDefaults{tenantDefaults: TenantDefaults{}}
}

func (defaults managedRoutingDefaults) value() TenantDefaults {
	return defaults.tenantDefaults
}

func validateCanonicalManagedRoutingDefaults(providers *providerRegistry, rawDefaults TenantDefaults) (managedRoutingDefaults, error) {
	defaults, defaultsError := newManagedRoutingDefaults(providers, rawDefaults)
	if defaultsError != nil {
		return managedRoutingDefaults{}, defaultsError
	}
	if strings.TrimSpace(rawDefaults.Provider) != defaults.tenantDefaults.Provider || strings.TrimSpace(rawDefaults.Model) != defaults.tenantDefaults.Model {
		return managedRoutingDefaults{}, managedRoutingDefaultsCanonicalError(endpointKindText, rawDefaults.Provider, rawDefaults.Model)
	}
	if strings.TrimSpace(rawDefaults.DictationProvider) != defaults.tenantDefaults.DictationProvider || strings.TrimSpace(rawDefaults.DictationModel) != defaults.tenantDefaults.DictationModel {
		return managedRoutingDefaults{}, managedRoutingDefaultsCanonicalError(endpointKindDictation, rawDefaults.DictationProvider, rawDefaults.DictationModel)
	}
	return defaults, nil
}

func validatePersistedManagedRoutingDefaults(providers *providerRegistry, providerSettings map[providerID]managedProviderSettings, rawDefaults TenantDefaults) (managedRoutingDefaults, error) {
	defaults, defaultsError := validateCanonicalManagedRoutingDefaults(providers, rawDefaults)
	if defaultsError != nil {
		return managedRoutingDefaults{}, defaultsError
	}
	reconciled, reconciliationError := reconcileManagedRoutingDefaults(providers, providerSettings, defaults)
	if reconciliationError != nil {
		return managedRoutingDefaults{}, reconciliationError
	}
	if defaults.value() != reconciled.value() {
		return managedRoutingDefaults{}, fmt.Errorf("%w: reason=provider_key_ineligible", errManagedRoutingDefaultsInvalid)
	}
	return defaults, nil
}

func reconcileManagedRoutingDefaults(providers *providerRegistry, providerSettings map[providerID]managedProviderSettings, current managedRoutingDefaults) (managedRoutingDefaults, error) {
	routingProviders, routingProvidersError := newManagedRoutingProviders(providers, providerSettings)
	if routingProvidersError != nil {
		return managedRoutingDefaults{}, routingProvidersError
	}
	return reconcileManagedRoutingDefaultsWithProviders(current, routingProviders), nil
}

func newManagedRoutingProviders(providers *providerRegistry, providerSettings map[providerID]managedProviderSettings) ([]managedRoutingProvider, error) {
	keyedProviderIdentifiers := managedKeyedProviderIdentifiers(providerSettings)
	routingProviders := make([]managedRoutingProvider, 0, len(keyedProviderIdentifiers))
	for _, providerIdentifier := range keyedProviderIdentifiers {
		settings := providerSettings[providerIdentifier]
		definition, model, resolutionError := providers.resolveTextModel(providerIdentifier.string(), settings.textModel, constants.EmptyString, constants.EmptyString, false)
		if resolutionError != nil {
			return nil, managedRoutingDefaultsPairError(endpointKindText, providerIdentifier.string(), settings.textModel, resolutionError)
		}
		routingProviders = append(routingProviders, managedRoutingProvider{
			definition: definition,
			textModel:  model,
		})
	}
	return routingProviders, nil
}

func reconcileManagedRoutingDefaultsWithProviders(current managedRoutingDefaults, routingProviders []managedRoutingProvider) managedRoutingDefaults {
	sort.Slice(routingProviders, func(first int, second int) bool {
		return routingProviders[first].definition.identifier.string() < routingProviders[second].definition.identifier.string()
	})
	reconciled := current.value()
	providerIsKeyed := func(rawProvider string) bool {
		providerIdentifier := newProviderID(rawProvider)
		for _, routingProvider := range routingProviders {
			if routingProvider.definition.identifier == providerIdentifier {
				return true
			}
		}
		return false
	}
	if !providerIsKeyed(reconciled.Provider) {
		reconciled.Provider = constants.EmptyString
		reconciled.Model = constants.EmptyString
		reconciled.ReasoningEffort = constants.EmptyString
		if len(routingProviders) != 0 {
			reconciled.Provider = routingProviders[0].definition.identifier.string()
			reconciled.Model = routingProviders[0].textModel.string()
		}
	}
	if !providerIsKeyed(reconciled.DictationProvider) {
		reconciled.DictationProvider = constants.EmptyString
		reconciled.DictationModel = constants.EmptyString
		for _, routingProvider := range routingProviders {
			if !routingProvider.definition.supportsDictation {
				continue
			}
			reconciled.DictationProvider = routingProvider.definition.identifier.string()
			reconciled.DictationModel = routingProvider.definition.defaultTranscriptionModel.string()
			break
		}
	}
	return managedRoutingDefaults{tenantDefaults: reconciled}
}

func reconcileManagedRoutingDefaultsAfterProviderTextModelChange(reconciled managedRoutingDefaults, routingProviders []managedRoutingProvider, changedProviderIdentifier providerID) managedRoutingDefaults {
	if newProviderID(reconciled.tenantDefaults.Provider) != changedProviderIdentifier {
		return reconciled
	}
	routingProvidersByIdentifier := make(map[providerID]managedRoutingProvider, len(routingProviders))
	for _, routingProvider := range routingProviders {
		routingProvidersByIdentifier[routingProvider.definition.identifier] = routingProvider
	}
	changedProvider := routingProvidersByIdentifier[changedProviderIdentifier]
	updated := reconciled.value()
	updated.Model = changedProvider.textModel.string()
	if reasoningEffortError := validateReasoningEffortForResolvedTextRoute(changedProvider.definition, changedProvider.textModel, updated.ReasoningEffort); reasoningEffortError != nil {
		updated.ReasoningEffort = constants.EmptyString
	}
	return managedRoutingDefaults{tenantDefaults: updated}
}

func managedKeyedProviderIdentifiers(providerSettings map[providerID]managedProviderSettings) []providerID {
	identifiers := make([]providerID, 0, len(providerSettings))
	for providerIdentifier, settings := range providerSettings {
		if strings.TrimSpace(settings.apiKey) != constants.EmptyString {
			identifiers = append(identifiers, providerIdentifier)
		}
	}
	sort.Slice(identifiers, func(first int, second int) bool {
		return identifiers[first].string() < identifiers[second].string()
	})
	return identifiers
}

func resolveManagedTextRoutingDefaultPair(providers *providerRegistry, rawProvider string, rawModel string) (providerDefinition, textModelDefinition, bool, error) {
	provider := strings.TrimSpace(rawProvider)
	model := strings.TrimSpace(rawModel)
	if provider == constants.EmptyString && model == constants.EmptyString {
		return providerDefinition{}, textModelDefinition{}, false, nil
	}
	if provider == constants.EmptyString || model == constants.EmptyString {
		return providerDefinition{}, textModelDefinition{}, false, managedRoutingDefaultsPairError(endpointKindText, rawProvider, rawModel, errManagedRoutingDefaultsInvalid)
	}
	definition, resolvedModel, resolutionError := providers.resolveTextModel(provider, model, constants.EmptyString, constants.EmptyString, false)
	if resolutionError != nil {
		return providerDefinition{}, textModelDefinition{}, false, managedRoutingDefaultsPairError(endpointKindText, rawProvider, rawModel, resolutionError)
	}
	return definition, resolvedModel, true, nil
}

func resolveManagedDictationRoutingDefaultPair(providers *providerRegistry, rawProvider string, rawModel string) (providerID, modelID, bool, error) {
	provider := strings.TrimSpace(rawProvider)
	model := strings.TrimSpace(rawModel)
	if provider == constants.EmptyString && model == constants.EmptyString {
		return providerID(""), modelID(""), false, nil
	}
	if provider == constants.EmptyString || model == constants.EmptyString {
		return providerID(""), modelID(""), false, managedRoutingDefaultsPairError(endpointKindDictation, rawProvider, rawModel, errManagedRoutingDefaultsInvalid)
	}
	definition, resolvedModel, resolutionError := providers.resolveDictationModel(provider, model, constants.EmptyString, constants.EmptyString)
	if resolutionError != nil {
		return providerID(""), modelID(""), false, managedRoutingDefaultsPairError(endpointKindDictation, rawProvider, rawModel, resolutionError)
	}
	return definition.identifier, resolvedModel, true, nil
}

func managedRoutingDefaultsPairError(endpoint endpointKind, rawProvider string, rawModel string, cause error) error {
	return fmt.Errorf("%w: endpoint=%s provider=%s model=%s: %w", errManagedRoutingDefaultsInvalid, endpoint, strings.TrimSpace(rawProvider), strings.TrimSpace(rawModel), cause)
}

func managedRoutingDefaultsCanonicalError(endpoint endpointKind, rawProvider string, rawModel string) error {
	return fmt.Errorf("%w: endpoint=%s provider=%s model=%s reason=not_canonical", errManagedRoutingDefaultsInvalid, endpoint, strings.TrimSpace(rawProvider), strings.TrimSpace(rawModel))
}
