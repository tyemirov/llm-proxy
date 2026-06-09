package proxy

import (
	"fmt"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

// ProviderModelCatalogs maps canonical provider identifiers to configured model catalogs.
type ProviderModelCatalogs map[string]ProviderModelCatalog

// ProviderModelCatalog declares text and dictation model support for one provider.
type ProviderModelCatalog struct {
	Text      ModelEndpointCatalog
	Dictation ModelEndpointCatalog
}

// ModelEndpointCatalog declares allowed models and the endpoint default model.
type ModelEndpointCatalog struct {
	DefaultModel string
	Models       []ModelConfiguration
}

// ModelConfiguration declares runtime metadata for one configured model.
type ModelConfiguration struct {
	ID               string
	RequestProfile   string
	WebSearch        bool
	OutputTokenLimit int
}

func defaultProviderModelCatalogs() ProviderModelCatalogs {
	return ProviderModelCatalogs{
		ProviderNameOpenAI: {
			Text: ModelEndpointCatalog{
				DefaultModel: ModelNameGPT41,
				Models: []ModelConfiguration{
					{ID: ModelNameGPT4oMini, RequestProfile: requestProfileOpenAIResponsesTemperature.string()},
					{ID: ModelNameGPT4o, RequestProfile: requestProfileOpenAIResponsesTemperatureTools.string(), WebSearch: true},
					{ID: ModelNameGPT41, RequestProfile: requestProfileOpenAIResponsesTemperatureTools.string(), WebSearch: true},
					{ID: ModelNameGPT5Mini, RequestProfile: requestProfileOpenAIResponsesBase.string()},
					{ID: ModelNameGPT5, RequestProfile: requestProfileOpenAIResponsesReasoningTools.string(), WebSearch: true},
					{ID: ModelNameGPT55, RequestProfile: requestProfileOpenAIResponsesReasoningTools.string(), WebSearch: true},
					{ID: ModelNameGPT55Pro, RequestProfile: requestProfileOpenAIResponsesReasoningTools.string(), WebSearch: true},
				},
			},
			Dictation: ModelEndpointCatalog{
				DefaultModel: DefaultDictationModel,
				Models: []ModelConfiguration{
					{ID: DefaultDictationModel},
					{ID: "gpt-4o-transcribe"},
				},
			},
		},
		ProviderNameDeepSeek: {
			Text: ModelEndpointCatalog{
				DefaultModel: ModelNameDeepSeekV4Flash,
				Models: []ModelConfiguration{
					{ID: ModelNameDeepSeekV4Flash},
					{ID: ModelNameDeepSeekV4Pro},
					{ID: ModelNameDeepSeekChat},
					{ID: ModelNameDeepSeekReasoner},
				},
			},
		},
		ProviderNameDashScope: {
			Text: ModelEndpointCatalog{
				DefaultModel: ModelNameDashScopeQwenPlus,
				Models: []ModelConfiguration{
					{ID: ModelNameDashScopeQwenPlus},
				},
			},
		},
		ProviderNameMoonshot: {
			Text: ModelEndpointCatalog{
				DefaultModel: ModelNameMoonshotKimi,
				Models: []ModelConfiguration{
					{ID: ModelNameMoonshotKimi},
				},
			},
		},
		ProviderNameSiliconFlow: {
			Text: ModelEndpointCatalog{
				DefaultModel: ModelNameSiliconFlowDeepSeek,
				Models: []ModelConfiguration{
					{ID: ModelNameSiliconFlowDeepSeek},
				},
			},
			Dictation: ModelEndpointCatalog{
				DefaultModel: defaultSiliconFlowSTTModel,
				Models: []ModelConfiguration{
					{ID: defaultSiliconFlowSTTModel},
				},
			},
		},
		ProviderNameZhipu: {
			Text: ModelEndpointCatalog{
				DefaultModel: ModelNameZhipuGLM,
				Models: []ModelConfiguration{
					{ID: ModelNameZhipuGLM},
				},
			},
			Dictation: ModelEndpointCatalog{
				DefaultModel: defaultZhipuSTTModel,
				Models: []ModelConfiguration{
					{ID: defaultZhipuSTTModel},
				},
			},
		},
		ProviderNameGemini: {
			Text: ModelEndpointCatalog{
				DefaultModel: ModelNameGemini35Flash,
				Models: []ModelConfiguration{
					{ID: ModelNameGemini35Flash, OutputTokenLimit: geminiOutputTokenLimit},
					{ID: ModelNameGemini31FlashLite, OutputTokenLimit: geminiOutputTokenLimit},
					{ID: ModelNameGemini25Flash, OutputTokenLimit: geminiOutputTokenLimit},
					{ID: ModelNameGemini25FlashLite, OutputTokenLimit: geminiOutputTokenLimit},
					{ID: ModelNameGemini25Pro, OutputTokenLimit: geminiOutputTokenLimit},
				},
			},
		},
		ProviderNameAnthropic: {
			Text: ModelEndpointCatalog{
				DefaultModel: ModelNameClaudeSonnet46,
				Models: []ModelConfiguration{
					{ID: ModelNameClaudeOpus48, OutputTokenLimit: anthropicOpusOutputTokenLimit},
					{ID: ModelNameClaudeSonnet46, OutputTokenLimit: anthropicOutputTokenLimit},
					{ID: ModelNameClaudeHaiku45, OutputTokenLimit: anthropicOutputTokenLimit},
					{ID: ModelNameClaudeHaiku45Alias, OutputTokenLimit: anthropicOutputTokenLimit},
					{ID: ModelNameClaudeSonnet45, OutputTokenLimit: anthropicOutputTokenLimit},
					{ID: ModelNameClaudeSonnet45Alias, OutputTokenLimit: anthropicOutputTokenLimit},
					{ID: ModelNameClaudeOpus41, OutputTokenLimit: anthropicLegacyOpusOutputTokenLimit},
					{ID: ModelNameClaudeOpus41Alias, OutputTokenLimit: anthropicLegacyOpusOutputTokenLimit},
				},
			},
		},
		ProviderNameGrok: {
			Text: ModelEndpointCatalog{
				DefaultModel: ModelNameGrok43,
				Models: []ModelConfiguration{
					{ID: ModelNameGrok43},
					{ID: ModelNameGrok43Latest},
					{ID: ModelNameGrokLatest},
					{ID: ModelNameGrokBuild01},
					{ID: ModelNameGrokCodeFast},
					{ID: ModelNameGrokCodeFast1},
					{ID: ModelNameGrokCodeFast10825},
				},
			},
			Dictation: ModelEndpointCatalog{
				DefaultModel: defaultGrokSTTModel,
				Models: []ModelConfiguration{
					{ID: defaultGrokSTTModel},
				},
			},
		},
	}
}

func validateProviderModelCatalogs(catalogs ProviderModelCatalogs) error {
	for _, providerName := range []string{
		ProviderNameOpenAI,
		ProviderNameDeepSeek,
		ProviderNameDashScope,
		ProviderNameMoonshot,
		ProviderNameSiliconFlow,
		ProviderNameZhipu,
		ProviderNameGemini,
		ProviderNameAnthropic,
		ProviderNameGrok,
	} {
		catalog, found := catalogs[providerName]
		if !found {
			return fmt.Errorf("%w: provider=%s field=providers.%s.text", ErrInvalidModelCatalog, providerName, providerName)
		}
		if catalogError := validateModelEndpointCatalog(providerName, endpointKindText, catalog.Text); catalogError != nil {
			return catalogError
		}
	}
	for _, providerName := range []string{ProviderNameOpenAI, ProviderNameSiliconFlow, ProviderNameZhipu, ProviderNameGrok} {
		if catalogError := validateModelEndpointCatalog(providerName, endpointKindDictation, catalogs[providerName].Dictation); catalogError != nil {
			return catalogError
		}
	}
	return nil
}

func validateModelEndpointCatalog(providerName string, endpoint endpointKind, catalog ModelEndpointCatalog) error {
	endpointName := string(endpoint)
	fieldPrefix := fmt.Sprintf("providers.%s.%s", providerName, endpointName)
	defaultModel := strings.TrimSpace(catalog.DefaultModel)
	if defaultModel == constants.EmptyString {
		return fmt.Errorf("%w: provider=%s endpoint=%s field=%s.default_model", ErrInvalidModelCatalog, providerName, endpointName, fieldPrefix)
	}
	if len(catalog.Models) == 0 {
		return fmt.Errorf("%w: provider=%s endpoint=%s field=%s.models", ErrInvalidModelCatalog, providerName, endpointName, fieldPrefix)
	}
	seenModelIdentifiers := map[string]struct{}{}
	defaultModelFound := false
	for modelIndex, modelConfiguration := range catalog.Models {
		modelIdentifier := strings.TrimSpace(modelConfiguration.ID)
		if modelIdentifier == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s endpoint=%s field=%s.models[%d].id", ErrInvalidModelCatalog, providerName, endpointName, fieldPrefix, modelIndex)
		}
		normalizedModelIdentifier := strings.ToLower(modelIdentifier)
		if _, duplicate := seenModelIdentifiers[normalizedModelIdentifier]; duplicate {
			return fmt.Errorf("%w: provider=%s endpoint=%s duplicate_model=%s", ErrInvalidModelCatalog, providerName, endpointName, modelIdentifier)
		}
		seenModelIdentifiers[normalizedModelIdentifier] = struct{}{}
		if strings.EqualFold(modelIdentifier, defaultModel) {
			defaultModelFound = true
		}
		if modelConfiguration.OutputTokenLimit < 0 {
			return fmt.Errorf("%w: provider=%s endpoint=%s field=%s.models[%d].output_token_limit", ErrInvalidModelCatalog, providerName, endpointName, fieldPrefix, modelIndex)
		}
		if providerName == ProviderNameAnthropic && endpoint == endpointKindText && modelConfiguration.OutputTokenLimit <= 0 {
			return fmt.Errorf("%w: provider=%s endpoint=%s field=%s.models[%d].output_token_limit", ErrInvalidModelCatalog, providerName, endpointName, fieldPrefix, modelIndex)
		}
		if profileError := validateModelRequestProfile(providerName, endpoint, modelConfiguration); profileError != nil {
			return fmt.Errorf("%w: field=%s.models[%d].request_profile", profileError, fieldPrefix, modelIndex)
		}
	}
	if !defaultModelFound {
		return fmt.Errorf("%w: provider=%s endpoint=%s default_model=%s", ErrInvalidModelCatalog, providerName, endpointName, defaultModel)
	}
	return nil
}

func validateModelRequestProfile(providerName string, endpoint endpointKind, modelConfiguration ModelConfiguration) error {
	requestProfile := strings.TrimSpace(modelConfiguration.RequestProfile)
	if providerName != ProviderNameOpenAI || endpoint != endpointKindText {
		if requestProfile != constants.EmptyString {
			return fmt.Errorf("%w: provider=%s endpoint=%s profile=%s", ErrInvalidModelCatalog, providerName, endpoint, requestProfile)
		}
		return nil
	}
	if requestProfile == constants.EmptyString {
		return fmt.Errorf("%w: provider=%s endpoint=%s", ErrInvalidModelCatalog, providerName, endpoint)
	}
	if !knownModelRequestProfile(modelRequestProfile(requestProfile)) {
		return fmt.Errorf("%w: provider=%s endpoint=%s profile=%s", ErrInvalidModelCatalog, providerName, endpoint, requestProfile)
	}
	return nil
}

func knownModelRequestProfile(requestProfile modelRequestProfile) bool {
	switch requestProfile {
	case requestProfileOpenAIResponsesBase,
		requestProfileOpenAIResponsesTemperature,
		requestProfileOpenAIResponsesTemperatureTools,
		requestProfileOpenAIResponsesReasoningTools:
		return true
	default:
		return false
	}
}

func textModelSet(catalog ModelEndpointCatalog) map[string]textModelDefinition {
	models := map[string]textModelDefinition{}
	for _, modelConfiguration := range catalog.Models {
		trimmedModelIdentifier := strings.TrimSpace(modelConfiguration.ID)
		if trimmedModelIdentifier != constants.EmptyString {
			models[strings.ToLower(trimmedModelIdentifier)] = textModelDefinition{
				identifier:          modelID(trimmedModelIdentifier),
				requestProfile:      modelRequestProfile(strings.TrimSpace(modelConfiguration.RequestProfile)),
				supportsWebSearch:   modelConfiguration.WebSearch,
				outputTokenLimit:    modelConfiguration.OutputTokenLimit,
				hasOutputTokenLimit: modelConfiguration.OutputTokenLimit > 0,
			}
		}
	}
	return models
}

func dictationModelSet(catalog ModelEndpointCatalog) map[string]modelID {
	models := map[string]modelID{}
	for _, modelConfiguration := range catalog.Models {
		trimmedModelIdentifier := strings.TrimSpace(modelConfiguration.ID)
		if trimmedModelIdentifier != constants.EmptyString {
			models[strings.ToLower(trimmedModelIdentifier)] = modelID(trimmedModelIdentifier)
		}
	}
	return models
}

func (requestProfile modelRequestProfile) string() string {
	return string(requestProfile)
}
