package proxy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

type providerRegistry struct {
	definitions map[providerID]providerDefinition
	aliases     map[string]providerID
}

type providerSummary struct {
	identifier            string
	label                 string
	aliases               []string
	textDefaultModel      string
	textModels            []textModelSummary
	supportsDictation     bool
	dictationDefaultModel string
	dictationModels       []string
}

type textModelSummary struct {
	identifier      string
	reasoningEffort *reasoningEffortCapability
}

func newProviderRegistry(configuration Configuration) *providerRegistry {
	definitions := configuredProviderDefinitions(configuration)
	for _, provider := range configuration.ModelCatalog.Providers {
		identifier := providerID(provider.ID)
		definition := definitions[identifier]
		definition.label = provider.Label
		definitions[identifier] = definition
	}
	for _, offering := range configuration.ModelCatalog.Offerings {
		identifier := providerID(offering.Provider)
		definition := definitions[identifier]
		if offeringSupportsOperation(offering, ModelOperationText) {
			routeCapabilities := textRouteCapabilities{
				wireContract:       textWireContract(offering.WireContract),
				executionLifecycle: textExecutionLifecycle(offering.ExecutionLifecycle),
			}
			definition.textModels[strings.ToLower(offering.Model)] = textModelDefinition{
				identifier:          modelID(offering.Model),
				providerIdentifier:  modelID(offering.ProviderModel),
				wireContract:        routeCapabilities.wireContract,
				executionLifecycle:  routeCapabilities.executionLifecycle,
				routeAdapter:        textRouteAdapters[routeCapabilities],
				requestProfile:      modelRequestProfile(offering.RequestProfile),
				supportsWebSearch:   offering.WebSearch,
				outputTokenLimit:    offering.OutputTokenLimit,
				hasOutputTokenLimit: offering.OutputTokenLimit > 0,
				reasoningEffort:     configuredReasoningEffortCapability(offering.ReasoningEffort),
				mediaInputs:         configuredMediaInputSet(offering.MediaInputs),
				mediaLimits:         cloneCatalogMediaLimits(offering.MediaLimits),
			}
			if offeringDefaultsOperation(offering, ModelOperationText) {
				definition.defaultTextModel = modelID(offering.Model)
			}
		}
		if offeringSupportsOperation(offering, ModelOperationDictation) {
			definition.transcriptionModels[strings.ToLower(offering.Model)] = dictationModelDefinition{
				identifier:         modelID(offering.Model),
				providerIdentifier: modelID(offering.ProviderModel),
			}
			definition.supportsDictation = true
			if offeringDefaultsOperation(offering, ModelOperationDictation) {
				definition.defaultTranscriptionModel = modelID(offering.Model)
			}
		}
		definitions[identifier] = definition
	}

	registry := &providerRegistry{
		definitions: definitions,
		aliases:     map[string]providerID{},
	}
	for identifier, definition := range definitions {
		registry.aliases[identifier.string()] = identifier
		for _, alias := range definition.aliases {
			normalizedAlias := strings.ToLower(strings.TrimSpace(alias))
			if normalizedAlias != constants.EmptyString {
				registry.aliases[normalizedAlias] = identifier
			}
		}
	}
	return registry
}

func configuredProviderDefinitions(configuration Configuration) map[providerID]providerDefinition {
	return map[providerID]providerDefinition{
		providerID(ProviderNameOpenAI): {
			identifier: providerID(ProviderNameOpenAI), textAPIKey: configuration.OpenAIKey,
			transcriptionAPIKey: configuration.OpenAIKey, transcriptionsURL: configuration.OpenAITranscriptionsURL,
			transcriptionModelField: keyModel, textModels: map[string]textModelDefinition{}, transcriptionModels: map[string]dictationModelDefinition{},
		},
		providerID(ProviderNameDeepSeek): {
			identifier: providerID(ProviderNameDeepSeek), textAPIKey: configuration.DeepSeekKey, textBaseURL: configuration.DeepSeekBaseURL,
			textModels: map[string]textModelDefinition{}, transcriptionModels: map[string]dictationModelDefinition{}, chatTokenLimitParameter: chatCompletionTokenLimitMaxTokens,
		},
		providerID(ProviderNameDashScope): {
			identifier: providerID(ProviderNameDashScope), aliases: []string{providerAliasQwen}, textAPIKey: configuration.DashScopeKey, textBaseURL: configuration.DashScopeBaseURL,
			textModels: map[string]textModelDefinition{}, transcriptionModels: map[string]dictationModelDefinition{}, chatTokenLimitParameter: chatCompletionTokenLimitMaxTokens,
		},
		providerID(ProviderNameMoonshot): {
			identifier: providerID(ProviderNameMoonshot), aliases: []string{providerAliasKimi}, textAPIKey: configuration.MoonshotKey, textBaseURL: configuration.MoonshotBaseURL,
			textModels: map[string]textModelDefinition{}, transcriptionModels: map[string]dictationModelDefinition{}, chatTokenLimitParameter: chatCompletionTokenLimitMaxCompletionTokens,
		},
		providerID(ProviderNameMiniMax): {
			identifier: providerID(ProviderNameMiniMax), textAPIKey: configuration.MiniMaxKey, textBaseURL: configuration.MiniMaxBaseURL,
			textModels: map[string]textModelDefinition{}, transcriptionModels: map[string]dictationModelDefinition{}, chatTokenLimitParameter: chatCompletionTokenLimitMaxCompletionTokens,
		},
		providerID(ProviderNameSiliconFlow): {
			identifier: providerID(ProviderNameSiliconFlow), textAPIKey: configuration.SiliconFlowKey, textBaseURL: configuration.SiliconFlowBaseURL,
			transcriptionAPIKey: configuration.SiliconFlowKey, transcriptionsURL: configuration.SiliconFlowTranscriptionsURL, transcriptionModelField: keyModel,
			textModels: map[string]textModelDefinition{}, transcriptionModels: map[string]dictationModelDefinition{}, chatTokenLimitParameter: chatCompletionTokenLimitMaxTokens,
		},
		providerID(ProviderNameZAI): {
			identifier: providerID(ProviderNameZAI), textAPIKey: configuration.ZAIKey, textBaseURL: configuration.ZAIBaseURL,
			transcriptionAPIKey: configuration.ZAIKey, transcriptionsURL: configuration.ZAITranscriptionsURL, transcriptionModelField: keyModel,
			textModels: map[string]textModelDefinition{}, transcriptionModels: map[string]dictationModelDefinition{}, chatTokenLimitParameter: chatCompletionTokenLimitMaxTokens,
		},
		providerID(ProviderNameGemini): {
			identifier: providerID(ProviderNameGemini), textAPIKey: configuration.GeminiKey, textBaseURL: configuration.GeminiBaseURL,
			textModels: map[string]textModelDefinition{}, transcriptionModels: map[string]dictationModelDefinition{},
		},
		providerID(ProviderNameAnthropic): {
			identifier: providerID(ProviderNameAnthropic), aliases: []string{providerAliasClaude}, textAPIKey: configuration.AnthropicKey, textBaseURL: configuration.AnthropicBaseURL,
			textModels: map[string]textModelDefinition{}, transcriptionModels: map[string]dictationModelDefinition{},
		},
		providerID(ProviderNameMeta): {
			identifier: providerID(ProviderNameMeta), textAPIKey: configuration.MetaKey, textBaseURL: configuration.MetaBaseURL,
			textModels: map[string]textModelDefinition{}, transcriptionModels: map[string]dictationModelDefinition{}, chatTokenLimitParameter: chatCompletionTokenLimitMaxCompletionTokens,
		},
		providerID(ProviderNameXAI): {
			identifier: providerID(ProviderNameXAI), textAPIKey: configuration.XAIKey, textBaseURL: configuration.XAIBaseURL,
			transcriptionAPIKey: configuration.XAIKey, transcriptionsURL: configuration.XAITranscriptionsURL, transcriptionModelField: constants.EmptyString,
			textModels: map[string]textModelDefinition{}, transcriptionModels: map[string]dictationModelDefinition{}, chatTokenLimitParameter: chatCompletionTokenLimitMaxTokens,
		},
	}
}

func configuredProviderAPIKeys(configuration Configuration) map[providerID]string {
	providerAPIKeys := map[providerID]string{}
	configuredProviderAPIKey(configuration.OpenAIKey, ProviderNameOpenAI, providerAPIKeys)
	configuredProviderAPIKey(configuration.DeepSeekKey, ProviderNameDeepSeek, providerAPIKeys)
	configuredProviderAPIKey(configuration.DashScopeKey, ProviderNameDashScope, providerAPIKeys)
	configuredProviderAPIKey(configuration.MoonshotKey, ProviderNameMoonshot, providerAPIKeys)
	configuredProviderAPIKey(configuration.MiniMaxKey, ProviderNameMiniMax, providerAPIKeys)
	configuredProviderAPIKey(configuration.SiliconFlowKey, ProviderNameSiliconFlow, providerAPIKeys)
	configuredProviderAPIKey(configuration.ZAIKey, ProviderNameZAI, providerAPIKeys)
	configuredProviderAPIKey(configuration.GeminiKey, ProviderNameGemini, providerAPIKeys)
	configuredProviderAPIKey(configuration.AnthropicKey, ProviderNameAnthropic, providerAPIKeys)
	configuredProviderAPIKey(configuration.MetaKey, ProviderNameMeta, providerAPIKeys)
	configuredProviderAPIKey(configuration.XAIKey, ProviderNameXAI, providerAPIKeys)
	return providerAPIKeys
}

func configuredProviderAPIKey(rawAPIKey string, rawProvider string, providerAPIKeys map[providerID]string) {
	apiKey := strings.TrimSpace(rawAPIKey)
	if apiKey == constants.EmptyString {
		return
	}
	providerAPIKeys[newProviderID(rawProvider)] = apiKey
}

func (registry *providerRegistry) forTenant(requestTenant tenant) *providerRegistry {
	if !requestTenant.managed {
		return registry
	}
	definitions := make(map[providerID]providerDefinition, len(registry.definitions))
	for identifier, definition := range registry.definitions {
		definition.textAPIKey = constants.EmptyString
		definition.transcriptionAPIKey = constants.EmptyString
		if providerSettings, configured := requestTenant.providerSettings[identifier]; configured {
			definition.textAPIKey = providerSettings.apiKey
			if providerSettings.baseURL != constants.EmptyString {
				definition.textBaseURL = providerSettings.baseURL
			}
			if definition.supportsDictation {
				definition.transcriptionAPIKey = providerSettings.apiKey
			}
		}
		definitions[identifier] = definition
	}
	return &providerRegistry{
		definitions: definitions,
		aliases:     registry.aliases,
	}
}

func (registry *providerRegistry) canonicalProviderID(rawProvider string) (providerID, error) {
	definition, providerError := registry.resolveProvider(rawProvider, constants.EmptyString)
	if providerError != nil {
		return providerID(""), providerError
	}
	return definition.identifier, nil
}

func (registry *providerRegistry) providerSummaries() []providerSummary {
	identifiers := make([]string, 0, len(registry.definitions))
	identifierLookup := map[string]providerID{}
	for identifier := range registry.definitions {
		identifierString := identifier.string()
		identifiers = append(identifiers, identifierString)
		identifierLookup[identifierString] = identifier
	}
	sort.Strings(identifiers)
	summaries := make([]providerSummary, 0, len(identifiers))
	for _, identifierString := range identifiers {
		identifier := identifierLookup[identifierString]
		definition := registry.definitions[identifier]
		aliases := append([]string(nil), definition.aliases...)
		sort.Strings(aliases)
		summaries = append(summaries, providerSummary{
			identifier:            definition.identifier.string(),
			label:                 definition.label,
			aliases:               aliases,
			textDefaultModel:      definition.defaultTextModel.string(),
			textModels:            sortedTextModelSummaries(definition.textModels),
			supportsDictation:     definition.supportsDictation,
			dictationDefaultModel: definition.defaultTranscriptionModel.string(),
			dictationModels:       sortedDictationModels(definition.transcriptionModels),
		})
	}
	return summaries
}

func (registry *providerRegistry) resolveProvider(rawProvider string, defaultProvider string) (providerDefinition, error) {
	providerCandidate := strings.TrimSpace(rawProvider)
	if providerCandidate == constants.EmptyString {
		providerCandidate = defaultProvider
	}
	normalizedProvider := newProviderID(providerCandidate)
	canonicalIdentifier, foundAlias := registry.aliases[normalizedProvider.string()]
	if !foundAlias {
		return providerDefinition{}, fmt.Errorf("%w: %s", ErrUnknownProvider, normalizedProvider.string())
	}
	return registry.definitions[canonicalIdentifier], nil
}

func validateReasoningEffortForResolvedTextRoute(provider providerDefinition, model textModelDefinition, effort string) error {
	if effort == constants.EmptyString {
		return nil
	}
	if !model.reasoningEffort.supports(effort) {
		return unsupportedReasoningEffortError(provider, model, effort)
	}
	return nil
}

func unsupportedReasoningEffortError(provider providerDefinition, model textModelDefinition, effort string) error {
	return fmt.Errorf("%w: provider=%s model=%s capability=reasoning_effort effort=%s", ErrUnsupportedCapability, provider.identifier.string(), model.string(), effort)
}

func (registry *providerRegistry) resolveTextRequest(rawProvider string, rawModel string, defaultProvider string, defaultModel string, webSearchEnabled bool) (providerDefinition, textModelDefinition, error) {
	definition, resolvedModel, resolutionError := registry.resolveTextModel(rawProvider, rawModel, defaultProvider, defaultModel, webSearchEnabled)
	if resolutionError != nil {
		return providerDefinition{}, textModelDefinition{}, resolutionError
	}
	if definition.credentialFor(endpointKindText) == constants.EmptyString {
		return providerDefinition{}, textModelDefinition{}, fmt.Errorf("%w: provider=%s endpoint=%s", ErrProviderNotConfigured, definition.identifier.string(), endpointKindText)
	}
	return definition, resolvedModel, nil
}

func (registry *providerRegistry) resolveTextModel(rawProvider string, rawModel string, defaultProvider string, defaultModel string, webSearchEnabled bool) (providerDefinition, textModelDefinition, error) {
	definition, providerError := registry.resolveProvider(rawProvider, defaultProvider)
	if providerError != nil {
		return providerDefinition{}, textModelDefinition{}, providerError
	}
	modelIdentifier := strings.TrimSpace(rawModel)
	if modelIdentifier == constants.EmptyString {
		if strings.TrimSpace(defaultModel) != constants.EmptyString {
			modelIdentifier = defaultModel
		} else {
			modelIdentifier = definition.defaultTextModel.string()
		}
	}
	resolvedModel, modelError := resolveTextModelFromSet(definition.textModels, modelIdentifier)
	if modelError != nil {
		return providerDefinition{}, textModelDefinition{}, modelError
	}
	if webSearchEnabled && !resolvedModel.supportsWebSearch {
		return providerDefinition{}, textModelDefinition{}, fmt.Errorf("%w: provider=%s model=%s capability=web_search", ErrUnsupportedCapability, definition.identifier.string(), resolvedModel.string())
	}
	return definition, resolvedModel, nil
}

func (registry *providerRegistry) resolveDictationRequest(rawProvider string, rawModel string, defaultProvider string, defaultModel string) (providerDefinition, modelID, error) {
	definition, resolvedModel, resolutionError := registry.resolveDictationModel(rawProvider, rawModel, defaultProvider, defaultModel)
	if resolutionError != nil {
		return providerDefinition{}, modelID(""), resolutionError
	}
	if definition.credentialFor(endpointKindDictation) == constants.EmptyString {
		return providerDefinition{}, modelID(""), fmt.Errorf("%w: provider=%s endpoint=%s", ErrProviderNotConfigured, definition.identifier.string(), endpointKindDictation)
	}
	return definition, resolvedModel, nil
}

func (registry *providerRegistry) resolveDictationModel(rawProvider string, rawModel string, defaultProvider string, defaultModel string) (providerDefinition, modelID, error) {
	definition, providerError := registry.resolveProvider(rawProvider, defaultProvider)
	if providerError != nil {
		return providerDefinition{}, modelID(""), providerError
	}
	if !definition.supportsDictation {
		return providerDefinition{}, modelID(""), fmt.Errorf("%w: provider=%s endpoint=%s", ErrUnsupportedEndpoint, definition.identifier.string(), endpointKindDictation)
	}
	modelIdentifier := strings.TrimSpace(rawModel)
	if modelIdentifier == constants.EmptyString {
		if strings.TrimSpace(rawProvider) == constants.EmptyString && strings.TrimSpace(defaultModel) != constants.EmptyString {
			modelIdentifier = defaultModel
		} else {
			modelIdentifier = definition.defaultTranscriptionModel.string()
		}
	}
	resolvedModel, modelError := resolveModelFromSet(definition.transcriptionModels, modelIdentifier)
	if modelError != nil {
		return providerDefinition{}, modelID(""), modelError
	}
	return definition, resolvedModel, nil
}

func resolveModelFromSet(modelIdentifiers map[string]dictationModelDefinition, rawModel string) (modelID, error) {
	resolvedModel := newModelID(rawModel)
	if modelDefinition, known := modelIdentifiers[strings.ToLower(resolvedModel.string())]; known {
		return modelDefinition.identifier, nil
	}
	return modelID(""), fmt.Errorf("%w: %s", ErrUnknownModel, resolvedModel.string())
}

func resolveTextModelFromSet(modelIdentifiers map[string]textModelDefinition, rawModel string) (textModelDefinition, error) {
	resolvedModel := newModelID(rawModel)
	if modelIdentifier, known := modelIdentifiers[strings.ToLower(resolvedModel.string())]; known {
		return modelIdentifier, nil
	}
	return textModelDefinition{}, fmt.Errorf("%w: %s", ErrUnknownModel, resolvedModel.string())
}

func sortedTextModelSummaries(modelIdentifiers map[string]textModelDefinition) []textModelSummary {
	modelsByIdentifier := map[string]textModelSummary{}
	modelIdentifiersByName := make([]string, 0, len(modelIdentifiers))
	for _, modelDefinition := range modelIdentifiers {
		modelIdentifier := modelDefinition.string()
		if _, seen := modelsByIdentifier[modelIdentifier]; seen {
			continue
		}
		modelsByIdentifier[modelIdentifier] = textModelSummary{
			identifier:      modelIdentifier,
			reasoningEffort: modelDefinition.reasoningEffort,
		}
		modelIdentifiersByName = append(modelIdentifiersByName, modelIdentifier)
	}
	sort.Strings(modelIdentifiersByName)
	models := make([]textModelSummary, 0, len(modelIdentifiersByName))
	for _, modelIdentifier := range modelIdentifiersByName {
		models = append(models, modelsByIdentifier[modelIdentifier])
	}
	return models
}

func sortedDictationModels(modelIdentifiers map[string]dictationModelDefinition) []string {
	models := make([]string, 0, len(modelIdentifiers))
	seenModels := map[string]struct{}{}
	for _, modelDefinition := range modelIdentifiers {
		modelIdentifierString := modelDefinition.identifier.string()
		if _, seen := seenModels[modelIdentifierString]; seen {
			continue
		}
		seenModels[modelIdentifierString] = struct{}{}
		models = append(models, modelIdentifierString)
	}
	sort.Strings(models)
	return models
}
