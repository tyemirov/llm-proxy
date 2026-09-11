package proxy

import (
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

const (
	// ProviderNameOpenAI identifies the OpenAI provider.
	ProviderNameOpenAI = "openai"
	// ProviderNameBaidu identifies international Baidu Qianfan routing.
	ProviderNameBaidu = "baidu"
	// ProviderNameDeepSeek identifies the DeepSeek provider.
	ProviderNameDeepSeek = "deepseek"
	// ProviderNameDashScope identifies Alibaba Cloud Model Studio DashScope-compatible routing.
	ProviderNameDashScope = "dashscope"
	// ProviderNameMoonshot identifies Moonshot/Kimi routing.
	ProviderNameMoonshot = "moonshot"
	// ProviderNameMiniMax identifies MiniMax routing.
	ProviderNameMiniMax = "minimax"
	// ProviderNameSiliconFlow identifies SiliconFlow routing.
	ProviderNameSiliconFlow = "siliconflow"
	// ProviderNameZAI identifies international Z.AI routing.
	ProviderNameZAI = "zai"
	// ProviderNameGemini identifies Google Gemini routing.
	ProviderNameGemini = "gemini"
	// ProviderNameAnthropic identifies Anthropic Claude routing.
	ProviderNameAnthropic = "anthropic"
	// ProviderNameMeta identifies Meta Model API routing.
	ProviderNameMeta = "meta"
	// ProviderNameXAI identifies the xAI credential and routing boundary.
	ProviderNameXAI = "xai"
)

const (
	providerAliasQwen   = "qwen"
	providerAliasKimi   = "kimi"
	providerAliasClaude = "claude"
)

const (
	// ModelNameDeepSeekV4Flash identifies the low-cost DeepSeek V4 flash model.
	ModelNameDeepSeekV4Flash = "deepseek-v4-flash"
	// ModelNameDeepSeekV4Pro identifies the higher-capability DeepSeek V4 pro model.
	ModelNameDeepSeekV4Pro = "deepseek-v4-pro"
	// ModelNameDashScopeQwenPlus identifies DashScope Qwen Plus.
	ModelNameDashScopeQwenPlus = "qwen-plus"
	// ModelNameDashScopeQwen37Max identifies DashScope Qwen 3.7 Max.
	ModelNameDashScopeQwen37Max = "qwen3.7-max"
	// ModelNameDashScopeQwen37Plus identifies DashScope Qwen 3.7 Plus.
	ModelNameDashScopeQwen37Plus = "qwen3.7-plus"
	// ModelNameDashScopeQwen36Flash identifies DashScope Qwen 3.6 Flash.
	ModelNameDashScopeQwen36Flash = "qwen3.6-flash"
	// ModelNameMoonshotKimiK26 identifies Moonshot Kimi K2.6.
	ModelNameMoonshotKimiK26 = "kimi-k2.6"
	// ModelNameMoonshotKimiK3 identifies Moonshot Kimi K3.
	ModelNameMoonshotKimiK3 = "kimi-k3"
	// ModelNameMoonshotKimiK27Code identifies Moonshot Kimi K2.7 Code.
	ModelNameMoonshotKimiK27Code = "kimi-k2.7-code"
	// ModelNameMoonshotKimiK27CodeHighSpeed identifies Moonshot Kimi K2.7 Code Highspeed.
	ModelNameMoonshotKimiK27CodeHighSpeed = "kimi-k2.7-code-highspeed"
	// ModelNameMiniMaxM3 identifies MiniMax M3.
	ModelNameMiniMaxM3 = "minimax-m3"
	// ModelNameMiniMaxM27 identifies MiniMax M2.7.
	ModelNameMiniMaxM27 = "minimax-m2.7"
	// ModelNameMiniMaxM27HighSpeed identifies MiniMax M2.7 Highspeed.
	ModelNameMiniMaxM27HighSpeed = "minimax-m2.7-highspeed"
	// ModelNameMiniMaxM25 identifies MiniMax M2.5.
	ModelNameMiniMaxM25 = "minimax-m2.5"
	// ModelNameMiniMaxM25HighSpeed identifies MiniMax M2.5 Highspeed.
	ModelNameMiniMaxM25HighSpeed = "minimax-m2.5-highspeed"
	// ModelNameMiniMaxM21 identifies MiniMax M2.1.
	ModelNameMiniMaxM21 = "minimax-m2.1"
	// ModelNameMiniMaxM21HighSpeed identifies MiniMax M2.1 Highspeed.
	ModelNameMiniMaxM21HighSpeed = "minimax-m2.1-highspeed"
	// ModelNameMiniMaxM2 identifies MiniMax M2.
	ModelNameMiniMaxM2 = "minimax-m2"
	// ModelNameSiliconFlowDeepSeek identifies SiliconFlow-hosted DeepSeek R1.
	ModelNameSiliconFlowDeepSeek = "deepseek-reasoner"
	// ModelNameZAIGLM identifies the GLM 5.1 model.
	ModelNameZAIGLM = "glm-5.1"
	// ModelNameGemini36Flash identifies Gemini 3.6 Flash.
	ModelNameGemini36Flash = "gemini-3.6-flash"
	// ModelNameGemini37Flash identifies Gemini 3.7 Flash.
	ModelNameGemini37Flash = "gemini-3.7-flash"
	// ModelNameGemini35Flash identifies Gemini 3.5 Flash.
	ModelNameGemini35Flash = "gemini-3.5-flash"
	// ModelNameClaudeOpus48 identifies Claude Opus 4.8.
	ModelNameClaudeOpus48 = "claude-opus-4-8"
	// ModelNameClaudeSonnet46 identifies Claude Sonnet 4.6.
	ModelNameClaudeSonnet46 = "claude-sonnet-4-6"
	// ModelNameClaudeHaiku45 identifies Claude Haiku 4.5.
	ModelNameClaudeHaiku45 = "claude-haiku-4-5-20251001"
	// ModelNameClaudeHaiku45Alias identifies the Claude Haiku 4.5 convenience alias.
	ModelNameClaudeHaiku45Alias = "claude-haiku-4-5"
	// ModelNameClaudeSonnet45 identifies Claude Sonnet 4.5.
	ModelNameClaudeSonnet45 = "claude-sonnet-4-5-20250929"
	// ModelNameClaudeSonnet45Alias identifies the Claude Sonnet 4.5 convenience alias.
	ModelNameClaudeSonnet45Alias = "claude-sonnet-4-5"
	// ModelNameMuseSpark11 identifies Meta Muse Spark 1.1.
	ModelNameMuseSpark11 = "muse-spark-1.1"
	// ModelNameMuseSpark12 identifies Meta Muse Spark 1.2.
	ModelNameMuseSpark12 = "muse-spark-1.2"
	// ModelNameGrok43 identifies the current Grok 4.3 model.
	ModelNameGrok43 = "grok-4.3"
	// ModelNameGrok43Latest identifies the Grok 4.3 latest alias.
	ModelNameGrok43Latest = "grok-4.3-latest"
	// ModelNameGrok45 identifies the Grok 4.5 model.
	ModelNameGrok45 = "grok-4.5"
	// ModelNameGrokLatest identifies the current Grok latest alias.
	ModelNameGrokLatest = "grok-latest"
	// ModelNameGrokBuild01 identifies the Grok Build coding model.
	ModelNameGrokBuild01 = "grok-build-0.1"
	// ModelNameGrokCodeFast identifies the Grok code fast alias.
	ModelNameGrokCodeFast = "grok-code-fast"
	// ModelNameGrokCodeFast1 identifies the Grok code fast 1 alias.
	ModelNameGrokCodeFast1 = "grok-code-fast-1"
	// ModelNameGrokCodeFast10825 identifies the dated Grok code fast 1 model.
	ModelNameGrokCodeFast10825 = "grok-code-fast-1-0825"
)

type endpointKind string

const (
	endpointKindText      endpointKind = "text"
	endpointKindDictation endpointKind = "dictation"
)

type textWireContract string

const (
	textWireContractOpenAIResponses       textWireContract = "openai_responses"
	textWireContractDashScopeResponses    textWireContract = "dashscope_responses"
	textWireContractXAIResponses          textWireContract = "xai_responses"
	textWireContractOpenAIChatCompletions textWireContract = "openai_chat_completions"
	textWireContractVertexGenerateContent textWireContract = "vertex_generate_content"
	textWireContractGeminiInteractions    textWireContract = "gemini_interactions"
	textWireContractAnthropicMessages     textWireContract = "anthropic_messages"
)

type textExecutionLifecycle string

const (
	textExecutionLifecycleSynchronousCompletion textExecutionLifecycle = "synchronous_completion"
	textExecutionLifecyclePollableResource      textExecutionLifecycle = "pollable_resource"
)

type textRouteCapabilities struct {
	wireContract       textWireContract
	executionLifecycle textExecutionLifecycle
}

var (
	vertexGenerateContentRouteCapabilities   = textRouteCapabilities{wireContract: textWireContractVertexGenerateContent, executionLifecycle: textExecutionLifecycleSynchronousCompletion}
	openAIResponsesPollableRouteCapabilities = textRouteCapabilities{
		wireContract:       textWireContractOpenAIResponses,
		executionLifecycle: textExecutionLifecyclePollableResource,
	}
	dashScopeResponsesSynchronousRouteCapabilities = textRouteCapabilities{
		wireContract:       textWireContractDashScopeResponses,
		executionLifecycle: textExecutionLifecycleSynchronousCompletion,
	}
	xaiResponsesSynchronousRouteCapabilities = textRouteCapabilities{
		wireContract:       textWireContractXAIResponses,
		executionLifecycle: textExecutionLifecycleSynchronousCompletion,
	}
	openAIChatCompletionsSynchronousRouteCapabilities = textRouteCapabilities{
		wireContract:       textWireContractOpenAIChatCompletions,
		executionLifecycle: textExecutionLifecycleSynchronousCompletion,
	}
	geminiInteractionsPollableRouteCapabilities = textRouteCapabilities{
		wireContract:       textWireContractGeminiInteractions,
		executionLifecycle: textExecutionLifecyclePollableResource,
	}
	geminiInteractionsSynchronousRouteCapabilities = textRouteCapabilities{
		wireContract:       textWireContractGeminiInteractions,
		executionLifecycle: textExecutionLifecycleSynchronousCompletion,
	}
	anthropicMessagesSynchronousRouteCapabilities = textRouteCapabilities{
		wireContract:       textWireContractAnthropicMessages,
		executionLifecycle: textExecutionLifecycleSynchronousCompletion,
	}
)

type chatCompletionTokenLimitParameter string

const (
	chatCompletionTokenLimitMaxTokens           chatCompletionTokenLimitParameter = "max_tokens"
	chatCompletionTokenLimitMaxCompletionTokens chatCompletionTokenLimitParameter = "max_completion_tokens"
)

type providerID string

func newProviderID(rawIdentifier string) providerID {
	normalizedIdentifier := strings.ToLower(strings.TrimSpace(rawIdentifier))
	return providerID(normalizedIdentifier)
}

func (identifier providerID) string() string {
	return string(identifier)
}

type modelID string

func newModelID(rawIdentifier string) modelID {
	normalizedIdentifier := strings.TrimSpace(rawIdentifier)
	return modelID(normalizedIdentifier)
}

func (identifier modelID) string() string {
	return string(identifier)
}

type modelRequestProfile string

const (
	requestProfileMiniMaxChatCompletions          modelRequestProfile = "minimax_chat_completions"
	requestProfileOpenAIResponsesTemperature      modelRequestProfile = "openai_responses_temperature"
	requestProfileOpenAIResponsesTemperatureTools modelRequestProfile = "openai_responses_temperature_tools"
	requestProfileOpenAIResponsesReasoningTools   modelRequestProfile = "openai_responses_reasoning_tools"
)

type reasoningEffortAdapter string

const (
	reasoningEffortAdapterVertexGenerateContent   reasoningEffortAdapter = "vertex_generate_content"
	reasoningEffortAdapterNone                    reasoningEffortAdapter = ""
	reasoningEffortAdapterDashScopeResponses      reasoningEffortAdapter = "dashscope_responses"
	reasoningEffortAdapterXAIResponses            reasoningEffortAdapter = "xai_responses"
	reasoningEffortAdapterOpenAIResponses         reasoningEffortAdapter = "openai_responses"
	reasoningEffortAdapterOpenAIChatCompletions   reasoningEffortAdapter = "openai_chat_completions"
	reasoningEffortAdapterChatCompletionsThinking reasoningEffortAdapter = "chat_completions_thinking"
	reasoningEffortAdapterGeminiInteractions      reasoningEffortAdapter = "gemini_interactions"
	reasoningEffortAdapterAnthropicMessages       reasoningEffortAdapter = "anthropic_messages"
)

type reasoningEffortCapability struct {
	adapter reasoningEffortAdapter
	efforts []string
}

// reasoningEffortAdapterSupportedValues is the adapter's accepted vocabulary;
// each configured text route owns the ordered subset it exposes.
var reasoningEffortAdapterSupportedValues = map[reasoningEffortAdapter]map[string]struct{}{
	reasoningEffortAdapterVertexGenerateContent: {"minimal": {}, "low": {}, "medium": {}, "high": {}},
	reasoningEffortAdapterDashScopeResponses:    {"none": {}, "low": {}, "medium": {}, "xhigh": {}},
	reasoningEffortAdapterXAIResponses:          {"low": {}, "medium": {}, "high": {}, "xhigh": {}},
	reasoningEffortAdapterOpenAIResponses: {
		"none":    {},
		"minimal": {},
		"low":     {},
		"medium":  {},
		"high":    {},
		"xhigh":   {},
		"max":     {},
	},
	reasoningEffortAdapterOpenAIChatCompletions: {
		"minimal": {},
		"low":     {},
		"medium":  {},
		"high":    {},
		"xhigh":   {},
		"max":     {},
	},
	reasoningEffortAdapterChatCompletionsThinking: {
		"none": {}, "low": {}, "high": {}, "max": {},
	},
	reasoningEffortAdapterGeminiInteractions: {
		"minimal": {},
		"low":     {},
		"medium":  {},
		"high":    {},
	},
	reasoningEffortAdapterAnthropicMessages: {
		"low": {}, "medium": {}, "high": {}, "xhigh": {}, "max": {},
	},
}

func knownReasoningEffortAdapter(adapter reasoningEffortAdapter) bool {
	_, known := reasoningEffortAdapterSupportedValues[adapter]
	return known
}

func reasoningEffortAdapterSupports(adapter reasoningEffortAdapter, effort string) bool {
	_, supported := reasoningEffortAdapterSupportedValues[adapter][effort]
	return supported
}

func (capability *reasoningEffortCapability) supports(effort string) bool {
	if capability == nil {
		return false
	}
	for _, configuredEffort := range capability.efforts {
		if configuredEffort == effort {
			return true
		}
	}
	return false
}

type textModelDefinition struct {
	callerTools bool

	identifier              modelID
	providerIdentifier      modelID
	transportIdentifier     string
	wireContract            textWireContract
	executionLifecycle      textExecutionLifecycle
	mediaExecutionLifecycle textExecutionLifecycle
	routeAdapter            textRouteAdapter
	requestProfile          modelRequestProfile
	supportsWebSearch       bool
	outputTokenLimit        int
	hasOutputTokenLimit     bool
	reasoningEffort         *reasoningEffortCapability
	chatResponsePolicy      chatCompletionResponsePolicy
	imageMIMETypes          []string
	mediaInputs             map[messageMediaType]struct{}
	mediaLimits             []CatalogMediaLimit
}

func (definition textModelDefinition) string() string {
	return definition.identifier.string()
}

func (definition textModelDefinition) providerString() string {
	return definition.providerIdentifier.string()
}

func (definition textModelDefinition) supportsMediaInput(mediaInput messageMediaType) bool {
	_, supported := definition.mediaInputs[mediaInput]
	return supported
}

type providerDefinition struct {
	identifier                providerID
	label                     string
	apiServiceLabel           string
	keyAcquisitionURL         string
	aliases                   []string
	capabilities              []string
	modelFamilies             []ModelFamily
	fields                    map[string]ProviderCatalogField
	fieldOrder                []string
	connectionValues          map[string]string
	transports                map[string]providerTransportDefinition
	activeTransport           providerTransportDefinition
	textAPIKey                string
	textBaseURL               string
	textEndpointURL           string
	transcriptionAPIKey       string
	transcriptionsURL         string
	defaultTextModel          modelID
	defaultTranscriptionModel modelID
	transcriptionModelField   string
	textModels                map[string]textModelDefinition
	transcriptionModels       map[string]dictationModelDefinition
	supportsDictation         bool
	chatTokenLimitParameter   chatCompletionTokenLimitParameter
}

type dictationModelDefinition struct {
	identifier          modelID
	providerIdentifier  modelID
	transportIdentifier string
}

type providerTransportDefinition struct {
	identifier          string
	endpoint            ProviderCatalogEndpoint
	authentication      ProviderCatalogAuthentication
	headers             []ProviderCatalogHeader
	requestCodec        string
	responseCodec       string
	lifecycle           textExecutionLifecycle
	resourceVisibility  pollableResourceVisibilityPolicy
	protocolParameters  providerProtocolParameters
	endpointURLOverride string
}

func (definition providerDefinition) resolvedTransport(transportIdentifier string) (providerDefinition, bool) {
	transport, found := definition.transports[transportIdentifier]
	if !found {
		return providerDefinition{}, false
	}
	baseURL := transport.endpoint.DefaultBaseURL
	if transport.endpoint.SettingField != constants.EmptyString {
		baseURL = definition.connectionValues[transport.endpoint.SettingField]
	}
	definition.activeTransport = transport
	definition.textBaseURL = strings.TrimRight(baseURL, "/")
	definition.textEndpointURL = definition.textBaseURL + transport.endpoint.Path
	if transport.endpointURLOverride != constants.EmptyString {
		definition.textEndpointURL = transport.endpointURLOverride
		definition.textBaseURL = strings.TrimSuffix(transport.endpointURLOverride, transport.endpoint.Path)
	}
	definition.textAPIKey = definition.connectionValues[transport.authentication.Field]
	definition.transcriptionAPIKey = definition.textAPIKey
	definition.transcriptionsURL = definition.textEndpointURL
	definition.transcriptionModelField = transport.protocolParameters.ModelField
	definition.chatTokenLimitParameter = chatCompletionTokenLimitParameter(transport.protocolParameters.TokenField)
	return definition, true
}

func (definition providerDefinition) credentialFor(endpoint endpointKind) string {
	if endpoint == endpointKindDictation {
		return strings.TrimSpace(definition.transcriptionAPIKey)
	}
	return strings.TrimSpace(definition.textAPIKey)
}
