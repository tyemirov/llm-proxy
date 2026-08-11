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
	ID                 string
	WireContract       string
	ExecutionLifecycle string
	RequestProfile     string
	WebSearch          bool
	OutputTokenLimit   int
	ReasoningEffort    *ReasoningEffortCapability
	MediaInputs        []string
}

var providerTextRouteCapabilities = map[string]map[textRouteCapabilities]struct{}{
	ProviderNameOpenAI: {
		openAIResponsesPollableRouteCapabilities: {},
	},
	ProviderNameDeepSeek: {
		openAIChatCompletionsSynchronousRouteCapabilities: {},
	},
	ProviderNameDashScope: {
		openAIChatCompletionsSynchronousRouteCapabilities: {},
	},
	ProviderNameMoonshot: {
		openAIChatCompletionsSynchronousRouteCapabilities: {},
	},
	ProviderNameMiniMax: {
		openAIChatCompletionsSynchronousRouteCapabilities: {},
	},
	ProviderNameSiliconFlow: {
		openAIChatCompletionsSynchronousRouteCapabilities: {},
	},
	ProviderNameZhipu: {
		openAIChatCompletionsSynchronousRouteCapabilities: {},
	},
	ProviderNameGemini: {
		geminiInteractionsPollableRouteCapabilities:    {},
		geminiInteractionsSynchronousRouteCapabilities: {},
	},
	ProviderNameAnthropic: {
		anthropicMessagesSynchronousRouteCapabilities: {},
	},
	ProviderNameMeta: {
		openAIChatCompletionsSynchronousRouteCapabilities: {},
	},
	ProviderNameGrok: {
		openAIChatCompletionsSynchronousRouteCapabilities: {},
	},
}

// ReasoningEffortCapability declares the configured upstream mapping for one
// exact text provider/model route.
type ReasoningEffortCapability struct {
	Adapter string
	Efforts []string
}

func validateProviderModelCatalogs(catalogs ProviderModelCatalogs) error {
	for _, providerName := range []string{
		ProviderNameOpenAI,
		ProviderNameDeepSeek,
		ProviderNameDashScope,
		ProviderNameMoonshot,
		ProviderNameMiniMax,
		ProviderNameSiliconFlow,
		ProviderNameZhipu,
		ProviderNameGemini,
		ProviderNameAnthropic,
		ProviderNameMeta,
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
		if modelConfiguration.WebSearch && (providerName != ProviderNameOpenAI || endpoint != endpointKindText) {
			return fmt.Errorf("%w: provider=%s endpoint=%s field=%s.models[%d].web_search", ErrInvalidModelCatalog, providerName, endpointName, fieldPrefix, modelIndex)
		}
		if providerName == ProviderNameAnthropic && endpoint == endpointKindText && modelConfiguration.OutputTokenLimit <= 0 {
			return fmt.Errorf("%w: provider=%s endpoint=%s field=%s.models[%d].output_token_limit", ErrInvalidModelCatalog, providerName, endpointName, fieldPrefix, modelIndex)
		}
		modelFieldPrefix := fmt.Sprintf("%s.models[%d]", fieldPrefix, modelIndex)
		if _, capabilityError := validatedTextRouteCapabilities(providerName, endpoint, modelConfiguration, modelFieldPrefix); capabilityError != nil {
			return capabilityError
		}
		if profileError := validateModelRequestProfile(providerName, endpoint, modelConfiguration); profileError != nil {
			return fmt.Errorf("%w: field=%s.models[%d].request_profile", profileError, fieldPrefix, modelIndex)
		}
		modelReasoningEffort, modelCapabilityError := validatedReasoningEffortCapability(modelConfiguration.ReasoningEffort, modelFieldPrefix+".reasoning_effort")
		if modelCapabilityError != nil {
			return fmt.Errorf("%w: provider=%s endpoint=%s", modelCapabilityError, providerName, endpointName)
		}
		if modelReasoningEffort != nil {
			if capabilityError := validateReasoningEffortAdapterMapping(providerName, endpoint, modelConfiguration, *modelReasoningEffort); capabilityError != nil {
				return fmt.Errorf("%w: field=%s.reasoning_effort", capabilityError, modelFieldPrefix)
			}
		}
		if _, mediaInputError := validatedMediaInputSet(providerName, endpoint, modelConfiguration.MediaInputs, modelFieldPrefix+".media_inputs"); mediaInputError != nil {
			return mediaInputError
		}
	}
	if !defaultModelFound {
		return fmt.Errorf("%w: provider=%s endpoint=%s default_model=%s", ErrInvalidModelCatalog, providerName, endpointName, defaultModel)
	}
	return nil
}

func validatedTextRouteCapabilities(providerName string, endpoint endpointKind, modelConfiguration ModelConfiguration, fieldPrefix string) (textRouteCapabilities, error) {
	rawWireContract := modelConfiguration.WireContract
	rawExecutionLifecycle := modelConfiguration.ExecutionLifecycle
	if endpoint != endpointKindText {
		if rawWireContract != constants.EmptyString {
			return textRouteCapabilities{}, fmt.Errorf("%w: provider=%s endpoint=%s field=%s.wire_contract", ErrInvalidModelCatalog, providerName, endpoint, fieldPrefix)
		}
		if rawExecutionLifecycle != constants.EmptyString {
			return textRouteCapabilities{}, fmt.Errorf("%w: provider=%s endpoint=%s field=%s.execution_lifecycle", ErrInvalidModelCatalog, providerName, endpoint, fieldPrefix)
		}
		return textRouteCapabilities{}, nil
	}
	if rawWireContract == constants.EmptyString || rawWireContract != strings.TrimSpace(rawWireContract) {
		return textRouteCapabilities{}, fmt.Errorf("%w: provider=%s endpoint=%s field=%s.wire_contract", ErrInvalidModelCatalog, providerName, endpoint, fieldPrefix)
	}
	if rawExecutionLifecycle == constants.EmptyString || rawExecutionLifecycle != strings.TrimSpace(rawExecutionLifecycle) {
		return textRouteCapabilities{}, fmt.Errorf("%w: provider=%s endpoint=%s field=%s.execution_lifecycle", ErrInvalidModelCatalog, providerName, endpoint, fieldPrefix)
	}
	capabilities := textRouteCapabilities{
		wireContract:       textWireContract(rawWireContract),
		executionLifecycle: textExecutionLifecycle(rawExecutionLifecycle),
	}
	if !knownTextWireContract(capabilities.wireContract) {
		return textRouteCapabilities{}, fmt.Errorf("%w: provider=%s endpoint=%s field=%s.wire_contract wire_contract=%s", ErrInvalidModelCatalog, providerName, endpoint, fieldPrefix, rawWireContract)
	}
	if !knownTextExecutionLifecycle(capabilities.executionLifecycle) {
		return textRouteCapabilities{}, fmt.Errorf("%w: provider=%s endpoint=%s field=%s.execution_lifecycle execution_lifecycle=%s", ErrInvalidModelCatalog, providerName, endpoint, fieldPrefix, rawExecutionLifecycle)
	}
	if _, allowed := providerTextRouteCapabilities[providerName][capabilities]; !allowed {
		return textRouteCapabilities{}, fmt.Errorf(
			"%w: provider=%s endpoint=%s wire_contract=%s execution_lifecycle=%s",
			ErrInvalidModelCatalog,
			providerName,
			endpoint,
			rawWireContract,
			rawExecutionLifecycle,
		)
	}
	return capabilities, nil
}

func knownTextWireContract(wireContract textWireContract) bool {
	switch wireContract {
	case textWireContractOpenAIResponses,
		textWireContractOpenAIChatCompletions,
		textWireContractGeminiInteractions,
		textWireContractAnthropicMessages:
		return true
	default:
		return false
	}
}

func knownTextExecutionLifecycle(executionLifecycle textExecutionLifecycle) bool {
	switch executionLifecycle {
	case textExecutionLifecycleSynchronousCompletion,
		textExecutionLifecyclePollableResource:
		return true
	default:
		return false
	}
}

func validatedReasoningEffortCapability(rawCapability *ReasoningEffortCapability, fieldPrefix string) (*reasoningEffortCapability, error) {
	if rawCapability == nil {
		return nil, nil
	}
	adapter := reasoningEffortAdapter(strings.TrimSpace(rawCapability.Adapter))
	if adapter == reasoningEffortAdapterNone {
		return nil, fmt.Errorf("%w: field=%s.adapter", ErrInvalidModelCatalog, fieldPrefix)
	}
	if !knownReasoningEffortAdapter(adapter) {
		return nil, fmt.Errorf("%w: field=%s.adapter adapter=%s", ErrInvalidModelCatalog, fieldPrefix, adapter)
	}
	if len(rawCapability.Efforts) == 0 {
		return nil, fmt.Errorf("%w: field=%s.efforts", ErrInvalidModelCatalog, fieldPrefix)
	}
	efforts := make([]string, 0, len(rawCapability.Efforts))
	seenEfforts := map[string]struct{}{}
	for effortIndex, effort := range rawCapability.Efforts {
		if effort == constants.EmptyString || effort != strings.TrimSpace(effort) || !reasoningEffortAdapterSupports(adapter, effort) {
			return nil, fmt.Errorf("%w: field=%s.efforts[%d] effort=%s", ErrInvalidModelCatalog, fieldPrefix, effortIndex, effort)
		}
		if _, duplicate := seenEfforts[effort]; duplicate {
			return nil, fmt.Errorf("%w: field=%s.efforts[%d] effort=%s", ErrInvalidModelCatalog, fieldPrefix, effortIndex, effort)
		}
		seenEfforts[effort] = struct{}{}
		efforts = append(efforts, effort)
	}
	return &reasoningEffortCapability{adapter: adapter, efforts: efforts}, nil
}

func validateReasoningEffortAdapterMapping(providerName string, endpoint endpointKind, modelConfiguration ModelConfiguration, capability reasoningEffortCapability) error {
	if endpoint != endpointKindText {
		return fmt.Errorf("%w: provider=%s endpoint=%s adapter=%s", ErrInvalidModelCatalog, providerName, endpoint, capability.adapter)
	}
	if capability.adapter != reasoningEffortAdapterOpenAIResponses || providerName != ProviderNameOpenAI || modelRequestProfile(strings.TrimSpace(modelConfiguration.RequestProfile)) != requestProfileOpenAIResponsesReasoningTools {
		return fmt.Errorf("%w: provider=%s endpoint=%s adapter=%s profile=%s", ErrInvalidModelCatalog, providerName, endpoint, capability.adapter, strings.TrimSpace(modelConfiguration.RequestProfile))
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
	case requestProfileOpenAIResponsesTemperature,
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
			routeCapabilities := textRouteCapabilities{
				wireContract:       textWireContract(strings.TrimSpace(modelConfiguration.WireContract)),
				executionLifecycle: textExecutionLifecycle(strings.TrimSpace(modelConfiguration.ExecutionLifecycle)),
			}
			models[strings.ToLower(trimmedModelIdentifier)] = textModelDefinition{
				identifier:          modelID(trimmedModelIdentifier),
				wireContract:        routeCapabilities.wireContract,
				executionLifecycle:  routeCapabilities.executionLifecycle,
				routeAdapter:        textRouteAdapters[routeCapabilities],
				requestProfile:      modelRequestProfile(strings.TrimSpace(modelConfiguration.RequestProfile)),
				supportsWebSearch:   modelConfiguration.WebSearch,
				outputTokenLimit:    modelConfiguration.OutputTokenLimit,
				hasOutputTokenLimit: modelConfiguration.OutputTokenLimit > 0,
				reasoningEffort:     configuredReasoningEffortCapability(modelConfiguration.ReasoningEffort),
				mediaInputs:         configuredMediaInputSet(modelConfiguration.MediaInputs),
			}
		}
	}
	return models
}

func validatedMediaInputSet(providerName string, endpoint endpointKind, rawMediaInputs []string, fieldPrefix string) (map[messageMediaType]struct{}, error) {
	mediaInputs := make(map[messageMediaType]struct{}, len(rawMediaInputs))
	for mediaInputIndex, rawMediaInput := range rawMediaInputs {
		mediaInput := messageMediaType(rawMediaInput)
		if rawMediaInput == constants.EmptyString || rawMediaInput != strings.TrimSpace(rawMediaInput) {
			return nil, fmt.Errorf("%w: field=%s[%d]", ErrInvalidModelCatalog, fieldPrefix, mediaInputIndex)
		}
		if !providerSupportsMessageMedia(providerName, endpoint, mediaInput) {
			return nil, fmt.Errorf(
				"%w: field=%s[%d] provider=%s endpoint=%s media_input=%s",
				ErrInvalidModelCatalog,
				fieldPrefix,
				mediaInputIndex,
				providerName,
				endpoint,
				rawMediaInput,
			)
		}
		if _, duplicate := mediaInputs[mediaInput]; duplicate {
			return nil, fmt.Errorf("%w: field=%s[%d] duplicate=%s", ErrInvalidModelCatalog, fieldPrefix, mediaInputIndex, rawMediaInput)
		}
		mediaInputs[mediaInput] = struct{}{}
	}
	return mediaInputs, nil
}

func configuredMediaInputSet(rawMediaInputs []string) map[messageMediaType]struct{} {
	mediaInputs := make(map[messageMediaType]struct{}, len(rawMediaInputs))
	for _, rawMediaInput := range rawMediaInputs {
		mediaInputs[messageMediaType(rawMediaInput)] = struct{}{}
	}
	return mediaInputs
}

func configuredReasoningEffortCapability(configuration *ReasoningEffortCapability) *reasoningEffortCapability {
	if configuration == nil {
		return nil
	}
	return &reasoningEffortCapability{
		adapter: reasoningEffortAdapter(strings.TrimSpace(configuration.Adapter)),
		efforts: append([]string(nil), configuration.Efforts...),
	}
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
