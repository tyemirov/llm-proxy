package proxy

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

const managedRoutingDefaultsMigrationVersion = 3

var (
	errManagedRoutingDefaultsInvalid   = errors.New("managed_routing_defaults_invalid")
	errManagedRoutingDefaultsMigration = errors.New("managed_routing_defaults_migration_failed")
)

type managedRoutingDefaults struct {
	tenantDefaults TenantDefaults
}

func newManagedRoutingDefaults(providers *providerRegistry, rawDefaults TenantDefaults) (managedRoutingDefaults, error) {
	textProvider, textModel, textError := resolveManagedTextRoutingDefaultPair(providers, rawDefaults.Provider, rawDefaults.Model)
	if textError != nil {
		return managedRoutingDefaults{}, textError
	}
	dictationProvider, dictationModel, dictationError := resolveManagedDictationRoutingDefaultPair(providers, rawDefaults.DictationProvider, rawDefaults.DictationModel)
	if dictationError != nil {
		return managedRoutingDefaults{}, dictationError
	}
	reasoningEffort := rawDefaults.ReasoningEffort
	if reasoningEffortError := validateReasoningEffortForResolvedTextRoute(textProvider, textModel, reasoningEffort); reasoningEffortError != nil {
		return managedRoutingDefaults{}, fmt.Errorf("%w: field=reasoning_effort effort=%s: %w", errManagedRoutingDefaultsInvalid, reasoningEffort, reasoningEffortError)
	}
	return managedRoutingDefaults{tenantDefaults: TenantDefaults{
		Provider:          textProvider.identifier.string(),
		Model:             textModel.string(),
		DictationProvider: dictationProvider.string(),
		DictationModel:    dictationModel.string(),
		SystemPrompt:      rawDefaults.SystemPrompt,
		ReasoningEffort:   reasoningEffort,
	}}, nil
}

func defaultManagedRoutingDefaults() managedRoutingDefaults {
	return managedRoutingDefaults{tenantDefaults: DefaultTenantDefaults()}
}

func (defaults managedRoutingDefaults) value() TenantDefaults {
	return defaults.tenantDefaults
}

func validatePersistedManagedRoutingDefaults(providers *providerRegistry, rawDefaults TenantDefaults) (managedRoutingDefaults, error) {
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

func migrateManagedRoutingDefaults(providers *providerRegistry, rawDefaults TenantDefaults) (managedRoutingDefaults, error) {
	textProvider, textModel, textError := migrateManagedTextRoutingDefaultPair(providers, rawDefaults.Provider, rawDefaults.Model)
	if textError != nil {
		return managedRoutingDefaults{}, textError
	}
	dictationProvider, dictationModel, dictationError := migrateManagedDictationRoutingDefaultPair(providers, rawDefaults.DictationProvider, rawDefaults.DictationModel)
	if dictationError != nil {
		return managedRoutingDefaults{}, dictationError
	}
	reasoningEffort := rawDefaults.ReasoningEffort
	if validateReasoningEffortForResolvedTextRoute(textProvider, textModel, reasoningEffort) != nil {
		reasoningEffort = constants.EmptyString
	}
	return newManagedRoutingDefaults(providers, TenantDefaults{
		Provider:          textProvider.identifier.string(),
		Model:             textModel.string(),
		DictationProvider: dictationProvider.string(),
		DictationModel:    dictationModel.string(),
		SystemPrompt:      rawDefaults.SystemPrompt,
		ReasoningEffort:   reasoningEffort,
	})
}

func resolveManagedTextRoutingDefaultPair(providers *providerRegistry, rawProvider string, rawModel string) (providerDefinition, textModelDefinition, error) {
	provider := strings.TrimSpace(rawProvider)
	model := strings.TrimSpace(rawModel)
	if provider == constants.EmptyString || model == constants.EmptyString {
		return providerDefinition{}, textModelDefinition{}, managedRoutingDefaultsPairError(endpointKindText, rawProvider, rawModel, errManagedRoutingDefaultsInvalid)
	}
	definition, resolvedModel, resolutionError := providers.resolveTextModel(provider, model, constants.EmptyString, constants.EmptyString, false)
	if resolutionError != nil {
		return providerDefinition{}, textModelDefinition{}, managedRoutingDefaultsPairError(endpointKindText, rawProvider, rawModel, resolutionError)
	}
	return definition, resolvedModel, nil
}

func resolveManagedDictationRoutingDefaultPair(providers *providerRegistry, rawProvider string, rawModel string) (providerID, modelID, error) {
	provider := strings.TrimSpace(rawProvider)
	model := strings.TrimSpace(rawModel)
	if provider == constants.EmptyString || model == constants.EmptyString {
		return providerID(""), modelID(""), managedRoutingDefaultsPairError(endpointKindDictation, rawProvider, rawModel, errManagedRoutingDefaultsInvalid)
	}
	definition, resolvedModel, resolutionError := providers.resolveDictationModel(provider, model, constants.EmptyString, constants.EmptyString)
	if resolutionError != nil {
		return providerID(""), modelID(""), managedRoutingDefaultsPairError(endpointKindDictation, rawProvider, rawModel, resolutionError)
	}
	return definition.identifier, resolvedModel, nil
}

func migrateManagedTextRoutingDefaultPair(providers *providerRegistry, rawProvider string, rawModel string) (providerDefinition, textModelDefinition, error) {
	definition, providerError := providers.resolveProvider(rawProvider, constants.EmptyString)
	if providerError != nil {
		return providerDefinition{}, textModelDefinition{}, managedRoutingDefaultsPairError(endpointKindText, rawProvider, rawModel, providerError)
	}
	model := strings.TrimSpace(rawModel)
	if model == constants.EmptyString {
		return definition, definition.textModels[strings.ToLower(definition.defaultTextModel.string())], nil
	}
	_, resolvedModel, resolutionError := providers.resolveTextModel(definition.identifier.string(), model, constants.EmptyString, constants.EmptyString, false)
	if resolutionError == nil {
		return definition, resolvedModel, nil
	}
	if providers.hasConfiguredTextModel(model) {
		return definition, definition.textModels[strings.ToLower(definition.defaultTextModel.string())], nil
	}
	return providerDefinition{}, textModelDefinition{}, managedRoutingDefaultsPairError(endpointKindText, rawProvider, rawModel, resolutionError)
}

func migrateManagedDictationRoutingDefaultPair(providers *providerRegistry, rawProvider string, rawModel string) (providerID, modelID, error) {
	definition, providerError := providers.resolveProvider(rawProvider, constants.EmptyString)
	if providerError != nil {
		return providerID(""), modelID(""), managedRoutingDefaultsPairError(endpointKindDictation, rawProvider, rawModel, providerError)
	}
	if !definition.supportsDictation {
		return providerID(""), modelID(""), managedRoutingDefaultsPairError(endpointKindDictation, rawProvider, rawModel, fmt.Errorf("%w: provider=%s endpoint=%s", ErrUnsupportedEndpoint, definition.identifier.string(), endpointKindDictation))
	}
	model := strings.TrimSpace(rawModel)
	if model == constants.EmptyString {
		return definition.identifier, definition.defaultTranscriptionModel, nil
	}
	_, resolvedModel, resolutionError := providers.resolveDictationModel(definition.identifier.string(), model, constants.EmptyString, constants.EmptyString)
	if resolutionError == nil {
		return definition.identifier, resolvedModel, nil
	}
	if providers.hasConfiguredDictationModel(model) {
		return definition.identifier, definition.defaultTranscriptionModel, nil
	}
	return providerID(""), modelID(""), managedRoutingDefaultsPairError(endpointKindDictation, rawProvider, rawModel, resolutionError)
}

func (registry *providerRegistry) hasConfiguredTextModel(rawModel string) bool {
	model := strings.ToLower(strings.TrimSpace(rawModel))
	for _, definition := range registry.definitions {
		if _, configured := definition.textModels[model]; configured {
			return true
		}
	}
	return false
}

func (registry *providerRegistry) hasConfiguredDictationModel(rawModel string) bool {
	model := strings.ToLower(strings.TrimSpace(rawModel))
	for _, definition := range registry.definitions {
		if _, configured := definition.transcriptionModels[model]; configured {
			return true
		}
	}
	return false
}

func managedRoutingDefaultsPairError(endpoint endpointKind, rawProvider string, rawModel string, cause error) error {
	return fmt.Errorf("%w: endpoint=%s provider=%s model=%s: %w", errManagedRoutingDefaultsInvalid, endpoint, strings.TrimSpace(rawProvider), strings.TrimSpace(rawModel), cause)
}

func managedRoutingDefaultsCanonicalError(endpoint endpointKind, rawProvider string, rawModel string) error {
	return fmt.Errorf("%w: endpoint=%s provider=%s model=%s reason=not_canonical", errManagedRoutingDefaultsInvalid, endpoint, strings.TrimSpace(rawProvider), strings.TrimSpace(rawModel))
}
