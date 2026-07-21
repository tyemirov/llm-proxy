package proxy

import (
	"strings"
)

const (
	// ProviderNameOpenAI identifies the OpenAI provider.
	ProviderNameOpenAI = "openai"
	// ProviderNameDeepSeek identifies the DeepSeek provider.
	ProviderNameDeepSeek = "deepseek"
	// ProviderNameDashScope identifies Alibaba Cloud Model Studio DashScope-compatible routing.
	ProviderNameDashScope = "dashscope"
	// ProviderNameQwenCloud identifies Qwen Cloud Token Plan routing.
	ProviderNameQwenCloud = "qwencloud"
	// ProviderNameMoonshot identifies Moonshot/Kimi routing.
	ProviderNameMoonshot = "moonshot"
	// ProviderNameMiniMax identifies MiniMax routing.
	ProviderNameMiniMax = "minimax"
	// ProviderNameSiliconFlow identifies SiliconFlow routing.
	ProviderNameSiliconFlow = "siliconflow"
	// ProviderNameZhipu identifies Zhipu/GLM routing.
	ProviderNameZhipu = "zhipu"
	// ProviderNameGemini identifies Google Gemini routing.
	ProviderNameGemini = "gemini"
	// ProviderNameAnthropic identifies Anthropic Claude routing.
	ProviderNameAnthropic = "anthropic"
	// ProviderNameMeta identifies Meta Model API routing.
	ProviderNameMeta = "meta"
	// ProviderNameGrok identifies xAI Grok routing.
	ProviderNameGrok = "grok"
)

const (
	providerAliasQwen   = "qwen"
	providerAliasKimi   = "kimi"
	providerAliasGLM    = "glm"
	providerAliasClaude = "claude"
	providerAliasXAI    = "xai"
)

const (
	defaultDeepSeekBaseURL        = "https://api.deepseek.com"
	defaultDashScopeBaseURL       = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	defaultQwenCloudBaseURL       = "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1"
	defaultMoonshotBaseURL        = "https://api.moonshot.ai/v1"
	defaultMiniMaxBaseURL         = "https://api.minimax.io/v1"
	defaultSiliconFlowBaseURL     = "https://api.siliconflow.com/v1"
	defaultZhipuBaseURL           = "https://open.bigmodel.cn/api/paas/v4"
	defaultZhipuTranscriptionsURL = "https://api.z.ai/api/paas/v4/audio/transcriptions"
	defaultGeminiBaseURL          = "https://generativelanguage.googleapis.com/v1"
	defaultAnthropicBaseURL       = "https://api.anthropic.com"
	defaultMetaBaseURL            = "https://api.meta.ai/v1"
	defaultGrokBaseURL            = "https://api.x.ai/v1"
	defaultGrokTranscriptionsURL  = "https://api.x.ai/v1/stt"
)

const (
	// ModelNameDeepSeekV4Flash identifies the low-cost DeepSeek V4 flash model.
	ModelNameDeepSeekV4Flash = "deepseek-v4-flash"
	// ModelNameDeepSeekV4Pro identifies the higher-capability DeepSeek V4 pro model.
	ModelNameDeepSeekV4Pro = "deepseek-v4-pro"
	// ModelNameDeepSeekChat identifies the legacy DeepSeek chat model name.
	ModelNameDeepSeekChat = "deepseek-chat"
	// ModelNameDeepSeekReasoner identifies the legacy DeepSeek reasoner model name.
	ModelNameDeepSeekReasoner = "deepseek-reasoner"
	// ModelNameDashScopeQwenPlus identifies DashScope Qwen Plus.
	ModelNameDashScopeQwenPlus = "qwen-plus"
	// ModelNameQwenCloudQwen38MaxPreview identifies Qwen Cloud Token Plan's Qwen 3.8 Max preview.
	ModelNameQwenCloudQwen38MaxPreview = "qwen3.8-max-preview"
	// ModelNameMoonshotKimi identifies the Kimi K2 preview model.
	ModelNameMoonshotKimi = "kimi-k2-0905-preview"
	// ModelNameMiniMaxM27 identifies MiniMax M2.7.
	ModelNameMiniMaxM27 = "MiniMax-M2.7"
	// ModelNameSiliconFlowDeepSeek identifies SiliconFlow-hosted DeepSeek R1.
	ModelNameSiliconFlowDeepSeek = "deepseek-ai/DeepSeek-R1"
	// ModelNameZhipuGLM identifies the GLM 5.1 model.
	ModelNameZhipuGLM = "glm-5.1"
	// ModelNameGemini35Flash identifies Gemini 3.5 Flash.
	ModelNameGemini35Flash = "gemini-3.5-flash"
	// ModelNameGemini31FlashLite identifies Gemini 3.1 Flash-Lite.
	ModelNameGemini31FlashLite = "gemini-3.1-flash-lite"
	// ModelNameGemini25Flash identifies Gemini 2.5 Flash.
	ModelNameGemini25Flash = "gemini-2.5-flash"
	// ModelNameGemini25FlashLite identifies Gemini 2.5 Flash-Lite.
	ModelNameGemini25FlashLite = "gemini-2.5-flash-lite"
	// ModelNameGemini25Pro identifies Gemini 2.5 Pro.
	ModelNameGemini25Pro = "gemini-2.5-pro"
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
	// ModelNameClaudeOpus41 identifies Claude Opus 4.1.
	ModelNameClaudeOpus41 = "claude-opus-4-1-20250805"
	// ModelNameClaudeOpus41Alias identifies the Claude Opus 4.1 convenience alias.
	ModelNameClaudeOpus41Alias = "claude-opus-4-1"
	// ModelNameMuseSpark11 identifies Meta Muse Spark 1.1.
	ModelNameMuseSpark11 = "muse-spark-1.1"
	// ModelNameGrok43 identifies the current Grok 4.3 model.
	ModelNameGrok43 = "grok-4.3"
	// ModelNameGrok43Latest identifies the Grok 4.3 latest alias.
	ModelNameGrok43Latest = "grok-4.3-latest"
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

type providerTextTransport string

const (
	textTransportOpenAIResponses      providerTextTransport = "openai_responses"
	textTransportOpenAICompatibleChat providerTextTransport = "openai_compatible_chat"
	textTransportGeminiGenerate       providerTextTransport = "gemini_generate"
	textTransportAnthropicMessages    providerTextTransport = "anthropic_messages"
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
	requestProfileOpenAIResponsesBase             modelRequestProfile = "openai_responses_base"
	requestProfileOpenAIResponsesTemperature      modelRequestProfile = "openai_responses_temperature"
	requestProfileOpenAIResponsesTemperatureTools modelRequestProfile = "openai_responses_temperature_tools"
	requestProfileOpenAIResponsesReasoningTools   modelRequestProfile = "openai_responses_reasoning_tools"
)

type reasoningEffortAdapter string

const (
	reasoningEffortAdapterNone            reasoningEffortAdapter = ""
	reasoningEffortAdapterOpenAIResponses reasoningEffortAdapter = "openai_responses"
)

var canonicalReasoningEfforts = []string{
	"minimal",
	"low",
	"medium",
	"high",
}

type reasoningEffortCapability struct {
	adapter reasoningEffortAdapter
}

func knownReasoningEffortAdapter(adapter reasoningEffortAdapter) bool {
	return adapter == reasoningEffortAdapterOpenAIResponses
}

func canonicalReasoningEffortOptions() []string {
	return append([]string(nil), canonicalReasoningEfforts...)
}

func isCanonicalReasoningEffort(rawEffort string) bool {
	for _, effort := range canonicalReasoningEfforts {
		if rawEffort == effort {
			return true
		}
	}
	return false
}

type textModelDefinition struct {
	identifier          modelID
	requestProfile      modelRequestProfile
	supportsWebSearch   bool
	outputTokenLimit    int
	hasOutputTokenLimit bool
	reasoningEffort     *reasoningEffortCapability
}

func (definition textModelDefinition) string() string {
	return definition.identifier.string()
}

type providerDefinition struct {
	identifier                providerID
	aliases                   []string
	textAPIKey                string
	textBaseURL               string
	transcriptionAPIKey       string
	transcriptionsURL         string
	defaultTextModel          modelID
	defaultTranscriptionModel modelID
	transcriptionModelField   string
	textModels                map[string]textModelDefinition
	textReasoningEffort       *reasoningEffortCapability
	transcriptionModels       map[string]modelID
	supportsDictation         bool
	textTransport             providerTextTransport
	chatTokenLimitParameter   chatCompletionTokenLimitParameter
}

func (definition providerDefinition) effectiveReasoningEffort(model textModelDefinition) *reasoningEffortCapability {
	if definition.textReasoningEffort != nil {
		return definition.textReasoningEffort
	}
	return model.reasoningEffort
}

func (definition providerDefinition) credentialFor(endpoint endpointKind) string {
	if endpoint == endpointKindDictation {
		return strings.TrimSpace(definition.transcriptionAPIKey)
	}
	return strings.TrimSpace(definition.textAPIKey)
}
