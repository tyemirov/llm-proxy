package proxy

import (
	"fmt"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

type providerRegistry struct {
	definitions map[providerID]providerDefinition
	aliases     map[string]providerID
}

func newProviderRegistry(configuration Configuration) *providerRegistry {
	openAIProviderID := providerID(ProviderNameOpenAI)
	deepSeekProviderID := providerID(ProviderNameDeepSeek)
	dashScopeProviderID := providerID(ProviderNameDashScope)
	moonshotProviderID := providerID(ProviderNameMoonshot)
	siliconFlowProviderID := providerID(ProviderNameSiliconFlow)
	zhipuProviderID := providerID(ProviderNameZhipu)
	geminiProviderID := providerID(ProviderNameGemini)
	anthropicProviderID := providerID(ProviderNameAnthropic)
	grokProviderID := providerID(ProviderNameGrok)
	openAIModels := configuration.ProviderModels[ProviderNameOpenAI]
	deepSeekModels := configuration.ProviderModels[ProviderNameDeepSeek]
	dashScopeModels := configuration.ProviderModels[ProviderNameDashScope]
	moonshotModels := configuration.ProviderModels[ProviderNameMoonshot]
	siliconFlowModels := configuration.ProviderModels[ProviderNameSiliconFlow]
	zhipuModels := configuration.ProviderModels[ProviderNameZhipu]
	geminiModels := configuration.ProviderModels[ProviderNameGemini]
	anthropicModels := configuration.ProviderModels[ProviderNameAnthropic]
	grokModels := configuration.ProviderModels[ProviderNameGrok]

	definitions := map[providerID]providerDefinition{
		openAIProviderID: {
			identifier:                openAIProviderID,
			textAPIKey:                configuration.OpenAIKey,
			transcriptionAPIKey:       configuration.OpenAIKey,
			transcriptionsURL:         configuration.OpenAITranscriptionsURL,
			defaultTextModel:          modelID(openAIModels.Text.DefaultModel),
			defaultTranscriptionModel: modelID(openAIModels.Dictation.DefaultModel),
			transcriptionModelField:   keyModel,
			textModels:                textModelSet(openAIModels.Text),
			transcriptionModels:       dictationModelSet(openAIModels.Dictation),
			supportsDictation:         true,
			textTransport:             textTransportOpenAIResponses,
		},
		deepSeekProviderID: {
			identifier:          deepSeekProviderID,
			textAPIKey:          configuration.DeepSeekKey,
			textBaseURL:         configuration.DeepSeekBaseURL,
			defaultTextModel:    modelID(deepSeekModels.Text.DefaultModel),
			textModels:          textModelSet(deepSeekModels.Text),
			transcriptionModels: map[string]modelID{},
			textTransport:       textTransportOpenAICompatibleChat,
		},
		dashScopeProviderID: {
			identifier:          dashScopeProviderID,
			aliases:             []string{providerAliasQwen},
			textAPIKey:          configuration.DashScopeKey,
			textBaseURL:         configuration.DashScopeBaseURL,
			defaultTextModel:    modelID(dashScopeModels.Text.DefaultModel),
			textModels:          textModelSet(dashScopeModels.Text),
			transcriptionModels: map[string]modelID{},
			textTransport:       textTransportOpenAICompatibleChat,
		},
		moonshotProviderID: {
			identifier:          moonshotProviderID,
			aliases:             []string{providerAliasKimi},
			textAPIKey:          configuration.MoonshotKey,
			textBaseURL:         configuration.MoonshotBaseURL,
			defaultTextModel:    modelID(moonshotModels.Text.DefaultModel),
			textModels:          textModelSet(moonshotModels.Text),
			transcriptionModels: map[string]modelID{},
			textTransport:       textTransportOpenAICompatibleChat,
		},
		siliconFlowProviderID: {
			identifier:                siliconFlowProviderID,
			textAPIKey:                configuration.SiliconFlowKey,
			textBaseURL:               configuration.SiliconFlowBaseURL,
			transcriptionAPIKey:       configuration.SiliconFlowKey,
			transcriptionsURL:         configuration.SiliconFlowTranscriptionsURL,
			defaultTextModel:          modelID(siliconFlowModels.Text.DefaultModel),
			defaultTranscriptionModel: modelID(siliconFlowModels.Dictation.DefaultModel),
			transcriptionModelField:   keyModel,
			textModels:                textModelSet(siliconFlowModels.Text),
			transcriptionModels:       dictationModelSet(siliconFlowModels.Dictation),
			supportsDictation:         true,
			textTransport:             textTransportOpenAICompatibleChat,
		},
		zhipuProviderID: {
			identifier:                zhipuProviderID,
			aliases:                   []string{providerAliasGLM},
			textAPIKey:                configuration.ZhipuKey,
			textBaseURL:               configuration.ZhipuBaseURL,
			transcriptionAPIKey:       configuration.ZhipuKey,
			transcriptionsURL:         configuration.ZhipuTranscriptionsURL,
			defaultTextModel:          modelID(zhipuModels.Text.DefaultModel),
			defaultTranscriptionModel: modelID(zhipuModels.Dictation.DefaultModel),
			transcriptionModelField:   keyModel,
			textModels:                textModelSet(zhipuModels.Text),
			transcriptionModels:       dictationModelSet(zhipuModels.Dictation),
			supportsDictation:         true,
			textTransport:             textTransportOpenAICompatibleChat,
		},
		geminiProviderID: {
			identifier:          geminiProviderID,
			textAPIKey:          configuration.GeminiKey,
			textBaseURL:         configuration.GeminiBaseURL,
			defaultTextModel:    modelID(geminiModels.Text.DefaultModel),
			textModels:          textModelSet(geminiModels.Text),
			transcriptionModels: map[string]modelID{},
			textTransport:       textTransportGeminiGenerate,
		},
		anthropicProviderID: {
			identifier:          anthropicProviderID,
			aliases:             []string{providerAliasClaude},
			textAPIKey:          configuration.AnthropicKey,
			textBaseURL:         configuration.AnthropicBaseURL,
			defaultTextModel:    modelID(anthropicModels.Text.DefaultModel),
			textModels:          textModelSet(anthropicModels.Text),
			transcriptionModels: map[string]modelID{},
			textTransport:       textTransportAnthropicMessages,
		},
		grokProviderID: {
			identifier:                grokProviderID,
			aliases:                   []string{providerAliasXAI},
			textAPIKey:                configuration.GrokKey,
			textBaseURL:               configuration.GrokBaseURL,
			transcriptionAPIKey:       configuration.GrokKey,
			transcriptionsURL:         configuration.GrokTranscriptionsURL,
			defaultTextModel:          modelID(grokModels.Text.DefaultModel),
			defaultTranscriptionModel: modelID(grokModels.Dictation.DefaultModel),
			transcriptionModelField:   constants.EmptyString,
			textModels:                textModelSet(grokModels.Text),
			transcriptionModels:       dictationModelSet(grokModels.Dictation),
			supportsDictation:         true,
			textTransport:             textTransportOpenAICompatibleChat,
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
	return registry
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

func (registry *providerRegistry) resolveTextRequest(rawProvider string, rawModel string, defaultProvider string, defaultModel string, webSearchEnabled bool) (providerDefinition, textModelDefinition, error) {
	definition, providerError := registry.resolveProvider(rawProvider, defaultProvider)
	if providerError != nil {
		return providerDefinition{}, textModelDefinition{}, providerError
	}
	modelIdentifier := strings.TrimSpace(rawModel)
	if modelIdentifier == constants.EmptyString {
		if strings.TrimSpace(rawProvider) == constants.EmptyString && strings.TrimSpace(defaultModel) != constants.EmptyString {
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
	if definition.credentialFor(endpointKindText) == constants.EmptyString {
		return providerDefinition{}, textModelDefinition{}, fmt.Errorf("%w: provider=%s endpoint=%s", ErrProviderNotConfigured, definition.identifier.string(), endpointKindText)
	}
	return definition, resolvedModel, nil
}

func (registry *providerRegistry) resolveDictationRequest(rawProvider string, rawModel string, defaultProvider string, defaultModel string) (providerDefinition, modelID, error) {
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
	if definition.credentialFor(endpointKindDictation) == constants.EmptyString {
		return providerDefinition{}, modelID(""), fmt.Errorf("%w: provider=%s endpoint=%s", ErrProviderNotConfigured, definition.identifier.string(), endpointKindDictation)
	}
	return definition, resolvedModel, nil
}

func resolveModelFromSet(modelIdentifiers map[string]modelID, rawModel string) (modelID, error) {
	resolvedModel := newModelID(rawModel)
	if modelIdentifier, known := modelIdentifiers[strings.ToLower(resolvedModel.string())]; known {
		return modelIdentifier, nil
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
