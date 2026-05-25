package proxy

import (
	"fmt"
	"strings"

	"github.com/temirov/llm-proxy/internal/constants"
)

type providerRegistry struct {
	definitions map[providerID]providerDefinition
	aliases     map[string]providerID
}

func newProviderRegistry(configuration Configuration) (*providerRegistry, error) {
	openAIProviderID := providerID(ProviderNameOpenAI)
	deepSeekProviderID := providerID(ProviderNameDeepSeek)
	dashScopeProviderID := providerID(ProviderNameDashScope)
	moonshotProviderID := providerID(ProviderNameMoonshot)
	siliconFlowProviderID := providerID(ProviderNameSiliconFlow)
	zhipuProviderID := providerID(ProviderNameZhipu)

	definitions := map[providerID]providerDefinition{
		openAIProviderID: {
			identifier:                openAIProviderID,
			textAPIKey:                configuration.OpenAIKey,
			transcriptionAPIKey:       configuration.OpenAIKey,
			defaultTextModel:          modelID(configuration.DefaultModel),
			defaultTranscriptionModel: modelID(configuration.DictationModel),
			textModels:                modelSet(ModelNameGPT4oMini, ModelNameGPT4o, ModelNameGPT41, ModelNameGPT5Mini, ModelNameGPT5, ModelNameGPT55, ModelNameGPT55Pro),
			transcriptionModels:       modelSet(configuration.DictationModel, DefaultDictationModel, "gpt-4o-transcribe"),
			supportsText:              true,
			supportsDictation:         true,
			supportsWebSearch:         true,
			usesOpenAIResponses:       true,
		},
		deepSeekProviderID: {
			identifier:          deepSeekProviderID,
			textAPIKey:          configuration.DeepSeekKey,
			textBaseURL:         configuration.DeepSeekBaseURL,
			defaultTextModel:    modelID(ModelNameDeepSeekV4Flash),
			textModels:          modelSet(ModelNameDeepSeekV4Flash, ModelNameDeepSeekV4Pro, ModelNameDeepSeekChat, ModelNameDeepSeekReasoner),
			transcriptionModels: map[string]modelID{},
			supportsText:        true,
		},
		dashScopeProviderID: {
			identifier:          dashScopeProviderID,
			aliases:             []string{providerAliasQwen},
			textAPIKey:          configuration.DashScopeKey,
			textBaseURL:         configuration.DashScopeBaseURL,
			defaultTextModel:    modelID(ModelNameDashScopeQwenPlus),
			textModels:          modelSet(ModelNameDashScopeQwenPlus),
			transcriptionModels: map[string]modelID{},
			supportsText:        true,
		},
		moonshotProviderID: {
			identifier:          moonshotProviderID,
			aliases:             []string{providerAliasKimi},
			textAPIKey:          configuration.MoonshotKey,
			textBaseURL:         configuration.MoonshotBaseURL,
			defaultTextModel:    modelID(ModelNameMoonshotKimi),
			textModels:          modelSet(ModelNameMoonshotKimi),
			transcriptionModels: map[string]modelID{},
			supportsText:        true,
		},
		siliconFlowProviderID: {
			identifier:                siliconFlowProviderID,
			textAPIKey:                configuration.SiliconFlowKey,
			textBaseURL:               configuration.SiliconFlowBaseURL,
			transcriptionAPIKey:       configuration.SiliconFlowKey,
			transcriptionsURL:         configuration.SiliconFlowTranscriptionsURL,
			defaultTextModel:          modelID(ModelNameSiliconFlowDeepSeek),
			defaultTranscriptionModel: modelID(defaultSiliconFlowSTTModel),
			textModels:                modelSet(ModelNameSiliconFlowDeepSeek),
			transcriptionModels:       modelSet(defaultSiliconFlowSTTModel),
			supportsText:              true,
			supportsDictation:         true,
		},
		zhipuProviderID: {
			identifier:          zhipuProviderID,
			aliases:             []string{providerAliasGLM},
			textAPIKey:          configuration.ZhipuKey,
			textBaseURL:         configuration.ZhipuBaseURL,
			defaultTextModel:    modelID(ModelNameZhipuGLM),
			textModels:          modelSet(ModelNameZhipuGLM),
			transcriptionModels: map[string]modelID{},
			supportsText:        true,
		},
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
	return registry, nil
}

func modelSet(modelIdentifiers ...string) map[string]modelID {
	models := map[string]modelID{}
	for _, modelIdentifier := range modelIdentifiers {
		trimmedModelIdentifier := strings.TrimSpace(modelIdentifier)
		if trimmedModelIdentifier != constants.EmptyString {
			models[strings.ToLower(trimmedModelIdentifier)] = modelID(trimmedModelIdentifier)
		}
	}
	return models
}

func (registry *providerRegistry) resolveProvider(rawProvider string, defaultProvider string) (providerDefinition, error) {
	providerCandidate := strings.TrimSpace(rawProvider)
	if providerCandidate == constants.EmptyString {
		providerCandidate = defaultProvider
	}
	normalizedProvider, providerError := newProviderID(providerCandidate)
	if providerError != nil {
		return providerDefinition{}, providerError
	}
	canonicalIdentifier, foundAlias := registry.aliases[normalizedProvider.string()]
	if !foundAlias {
		return providerDefinition{}, fmt.Errorf("%w: %s", ErrUnknownProvider, normalizedProvider.string())
	}
	definition, foundDefinition := registry.definitions[canonicalIdentifier]
	if !foundDefinition {
		return providerDefinition{}, fmt.Errorf("%w: %s", ErrUnknownProvider, normalizedProvider.string())
	}
	return definition, nil
}

func (registry *providerRegistry) resolveTextRequest(rawProvider string, rawModel string, defaultProvider string, defaultModel string, webSearchEnabled bool) (providerDefinition, modelID, error) {
	definition, providerError := registry.resolveProvider(rawProvider, defaultProvider)
	if providerError != nil {
		return providerDefinition{}, modelID(""), providerError
	}
	if !definition.supports(endpointKindText) {
		return providerDefinition{}, modelID(""), fmt.Errorf("%w: provider=%s endpoint=%s", ErrUnsupportedEndpoint, definition.identifier.string(), endpointKindText)
	}
	if webSearchEnabled && !definition.supportsWebSearch {
		return providerDefinition{}, modelID(""), fmt.Errorf("%w: provider=%s capability=web_search", ErrUnsupportedCapability, definition.identifier.string())
	}
	modelIdentifier := strings.TrimSpace(rawModel)
	if modelIdentifier == constants.EmptyString {
		if definition.identifier == providerID(ProviderNameOpenAI) {
			modelIdentifier = defaultModel
		} else {
			modelIdentifier = definition.defaultTextModel.string()
		}
	}
	resolvedModel, modelError := resolveModelFromSet(definition.textModels, modelIdentifier)
	if modelError != nil {
		return providerDefinition{}, modelID(""), modelError
	}
	if definition.credentialFor(endpointKindText) == constants.EmptyString {
		return providerDefinition{}, modelID(""), fmt.Errorf("%w: provider=%s endpoint=%s", ErrProviderNotConfigured, definition.identifier.string(), endpointKindText)
	}
	return definition, resolvedModel, nil
}

func (registry *providerRegistry) resolveDictationRequest(rawProvider string, rawModel string, defaultProvider string, defaultModel string) (providerDefinition, modelID, error) {
	definition, providerError := registry.resolveProvider(rawProvider, defaultProvider)
	if providerError != nil {
		return providerDefinition{}, modelID(""), providerError
	}
	if !definition.supports(endpointKindDictation) {
		return providerDefinition{}, modelID(""), fmt.Errorf("%w: provider=%s endpoint=%s", ErrUnsupportedEndpoint, definition.identifier.string(), endpointKindDictation)
	}
	modelIdentifier := strings.TrimSpace(rawModel)
	if modelIdentifier == constants.EmptyString {
		if definition.identifier == providerID(ProviderNameOpenAI) {
			modelIdentifier = defaultModel
		} else {
			modelIdentifier = definition.defaultTranscriptionModel.string()
		}
	}
	if definition.identifier == providerID(ProviderNameOpenAI) {
		resolvedModel, constructorError := newModelID(modelIdentifier)
		if constructorError != nil {
			return providerDefinition{}, modelID(""), constructorError
		}
		if definition.credentialFor(endpointKindDictation) == constants.EmptyString {
			return providerDefinition{}, modelID(""), fmt.Errorf("%w: provider=%s endpoint=%s", ErrProviderNotConfigured, definition.identifier.string(), endpointKindDictation)
		}
		return definition, resolvedModel, nil
	}
	resolvedModel, modelError := resolveModelFromSet(definition.transcriptionModels, modelIdentifier)
	if modelError != nil {
		return providerDefinition{}, modelID(""), modelError
	}
	if definition.credentialFor(endpointKindDictation) == constants.EmptyString {
		return providerDefinition{}, modelID(""), fmt.Errorf("%w: provider=%s endpoint=%s", ErrProviderNotConfigured, definition.identifier.string(), endpointKindDictation)
	}
	return definition, resolvedModel, nil
}

func resolveModelFromSet(modelIdentifiers map[string]modelID, rawModel string) (modelID, error) {
	resolvedModel, constructorError := newModelID(rawModel)
	if constructorError != nil {
		return modelID(""), constructorError
	}
	if modelIdentifier, known := modelIdentifiers[strings.ToLower(resolvedModel.string())]; known {
		return modelIdentifier, nil
	}
	return modelID(""), fmt.Errorf("%w: %s", ErrUnknownModel, resolvedModel.string())
}
