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
	textModels            []string
	supportsDictation     bool
	dictationDefaultModel string
	dictationModels       []string
}

func newProviderRegistry(configuration Configuration) *providerRegistry {
	openAIProviderID := providerID(ProviderNameOpenAI)
	deepSeekProviderID := providerID(ProviderNameDeepSeek)
	dashScopeProviderID := providerID(ProviderNameDashScope)
	qwenCloudProviderID := providerID(ProviderNameQwenCloud)
	moonshotProviderID := providerID(ProviderNameMoonshot)
	miniMaxProviderID := providerID(ProviderNameMiniMax)
	siliconFlowProviderID := providerID(ProviderNameSiliconFlow)
	zhipuProviderID := providerID(ProviderNameZhipu)
	geminiProviderID := providerID(ProviderNameGemini)
	anthropicProviderID := providerID(ProviderNameAnthropic)
	metaProviderID := providerID(ProviderNameMeta)
	grokProviderID := providerID(ProviderNameGrok)
	openAIModels := configuration.ProviderModels[ProviderNameOpenAI]
	deepSeekModels := configuration.ProviderModels[ProviderNameDeepSeek]
	dashScopeModels := configuration.ProviderModels[ProviderNameDashScope]
	qwenCloudModels := configuration.ProviderModels[ProviderNameQwenCloud]
	moonshotModels := configuration.ProviderModels[ProviderNameMoonshot]
	miniMaxModels := configuration.ProviderModels[ProviderNameMiniMax]
	siliconFlowModels := configuration.ProviderModels[ProviderNameSiliconFlow]
	zhipuModels := configuration.ProviderModels[ProviderNameZhipu]
	geminiModels := configuration.ProviderModels[ProviderNameGemini]
	anthropicModels := configuration.ProviderModels[ProviderNameAnthropic]
	metaModels := configuration.ProviderModels[ProviderNameMeta]
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
			identifier:              deepSeekProviderID,
			textAPIKey:              configuration.DeepSeekKey,
			textBaseURL:             configuration.DeepSeekBaseURL,
			defaultTextModel:        modelID(deepSeekModels.Text.DefaultModel),
			textModels:              textModelSet(deepSeekModels.Text),
			transcriptionModels:     map[string]modelID{},
			textTransport:           textTransportOpenAICompatibleChat,
			chatTokenLimitParameter: chatCompletionTokenLimitMaxTokens,
		},
		dashScopeProviderID: {
			identifier:              dashScopeProviderID,
			aliases:                 []string{providerAliasQwen},
			textAPIKey:              configuration.DashScopeKey,
			textBaseURL:             configuration.DashScopeBaseURL,
			defaultTextModel:        modelID(dashScopeModels.Text.DefaultModel),
			textModels:              textModelSet(dashScopeModels.Text),
			transcriptionModels:     map[string]modelID{},
			textTransport:           textTransportOpenAICompatibleChat,
			chatTokenLimitParameter: chatCompletionTokenLimitMaxTokens,
		},
		qwenCloudProviderID: {
			identifier:              qwenCloudProviderID,
			textAPIKey:              configuration.QwenCloudKey,
			textBaseURL:             configuration.QwenCloudBaseURL,
			defaultTextModel:        modelID(qwenCloudModels.Text.DefaultModel),
			textModels:              textModelSet(qwenCloudModels.Text),
			transcriptionModels:     map[string]modelID{},
			textTransport:           textTransportOpenAICompatibleChat,
			chatTokenLimitParameter: chatCompletionTokenLimitMaxTokens,
		},
		moonshotProviderID: {
			identifier:              moonshotProviderID,
			aliases:                 []string{providerAliasKimi},
			textAPIKey:              configuration.MoonshotKey,
			textBaseURL:             configuration.MoonshotBaseURL,
			defaultTextModel:        modelID(moonshotModels.Text.DefaultModel),
			textModels:              textModelSet(moonshotModels.Text),
			transcriptionModels:     map[string]modelID{},
			textTransport:           textTransportOpenAICompatibleChat,
			chatTokenLimitParameter: chatCompletionTokenLimitMaxCompletionTokens,
		},
		miniMaxProviderID: {
			identifier:              miniMaxProviderID,
			textAPIKey:              configuration.MiniMaxKey,
			textBaseURL:             configuration.MiniMaxBaseURL,
			defaultTextModel:        modelID(miniMaxModels.Text.DefaultModel),
			textModels:              textModelSet(miniMaxModels.Text),
			transcriptionModels:     map[string]modelID{},
			textTransport:           textTransportOpenAICompatibleChat,
			chatTokenLimitParameter: chatCompletionTokenLimitMaxCompletionTokens,
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
			chatTokenLimitParameter:   chatCompletionTokenLimitMaxTokens,
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
			chatTokenLimitParameter:   chatCompletionTokenLimitMaxTokens,
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
		metaProviderID: {
			identifier:              metaProviderID,
			textAPIKey:              configuration.MetaKey,
			textBaseURL:             configuration.MetaBaseURL,
			defaultTextModel:        modelID(metaModels.Text.DefaultModel),
			textModels:              textModelSet(metaModels.Text),
			transcriptionModels:     map[string]modelID{},
			textTransport:           textTransportOpenAICompatibleChat,
			chatTokenLimitParameter: chatCompletionTokenLimitMaxCompletionTokens,
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
			chatTokenLimitParameter:   chatCompletionTokenLimitMaxTokens,
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

func configuredProviderAPIKeys(configuration Configuration) map[providerID]string {
	providerAPIKeys := map[providerID]string{}
	configuredProviderAPIKey(configuration.OpenAIKey, ProviderNameOpenAI, providerAPIKeys)
	configuredProviderAPIKey(configuration.DeepSeekKey, ProviderNameDeepSeek, providerAPIKeys)
	configuredProviderAPIKey(configuration.DashScopeKey, ProviderNameDashScope, providerAPIKeys)
	configuredProviderAPIKey(configuration.QwenCloudKey, ProviderNameQwenCloud, providerAPIKeys)
	configuredProviderAPIKey(configuration.MoonshotKey, ProviderNameMoonshot, providerAPIKeys)
	configuredProviderAPIKey(configuration.MiniMaxKey, ProviderNameMiniMax, providerAPIKeys)
	configuredProviderAPIKey(configuration.SiliconFlowKey, ProviderNameSiliconFlow, providerAPIKeys)
	configuredProviderAPIKey(configuration.ZhipuKey, ProviderNameZhipu, providerAPIKeys)
	configuredProviderAPIKey(configuration.GeminiKey, ProviderNameGemini, providerAPIKeys)
	configuredProviderAPIKey(configuration.AnthropicKey, ProviderNameAnthropic, providerAPIKeys)
	configuredProviderAPIKey(configuration.MetaKey, ProviderNameMeta, providerAPIKeys)
	configuredProviderAPIKey(configuration.GrokKey, ProviderNameGrok, providerAPIKeys)
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
			if definition.supportsDictation {
				definition.transcriptionAPIKey = providerSettings.apiKey
			}
		} else if apiKey, configured := requestTenant.providerAPIKeys[identifier]; configured {
			definition.textAPIKey = apiKey
			if definition.supportsDictation {
				definition.transcriptionAPIKey = apiKey
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
			label:                 providerLabel(definition.identifier),
			aliases:               aliases,
			textDefaultModel:      definition.defaultTextModel.string(),
			textModels:            sortedTextModels(definition.textModels),
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

func sortedTextModels(modelIdentifiers map[string]textModelDefinition) []string {
	models := make([]string, 0, len(modelIdentifiers))
	seenModels := map[string]struct{}{}
	for _, modelDefinition := range modelIdentifiers {
		modelIdentifier := modelDefinition.string()
		if _, seen := seenModels[modelIdentifier]; seen {
			continue
		}
		seenModels[modelIdentifier] = struct{}{}
		models = append(models, modelIdentifier)
	}
	sort.Strings(models)
	return models
}

func sortedDictationModels(modelIdentifiers map[string]modelID) []string {
	models := make([]string, 0, len(modelIdentifiers))
	seenModels := map[string]struct{}{}
	for _, modelIdentifier := range modelIdentifiers {
		modelIdentifierString := modelIdentifier.string()
		if _, seen := seenModels[modelIdentifierString]; seen {
			continue
		}
		seenModels[modelIdentifierString] = struct{}{}
		models = append(models, modelIdentifierString)
	}
	sort.Strings(models)
	return models
}

func providerLabel(identifier providerID) string {
	switch identifier.string() {
	case ProviderNameOpenAI:
		return "OpenAI"
	case ProviderNameDeepSeek:
		return "DeepSeek"
	case ProviderNameDashScope:
		return "DashScope"
	case ProviderNameQwenCloud:
		return "Qwen Cloud"
	case ProviderNameMoonshot:
		return "Moonshot"
	case ProviderNameMiniMax:
		return "MiniMax"
	case ProviderNameSiliconFlow:
		return "SiliconFlow"
	case ProviderNameZhipu:
		return "Zhipu"
	case ProviderNameGemini:
		return "Gemini"
	case ProviderNameAnthropic:
		return "Anthropic"
	case ProviderNameMeta:
		return "Meta"
	case ProviderNameGrok:
		return "Grok"
	default:
		return identifier.string()
	}
}
