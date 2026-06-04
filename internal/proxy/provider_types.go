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
	// ProviderNameMoonshot identifies Moonshot/Kimi routing.
	ProviderNameMoonshot = "moonshot"
	// ProviderNameSiliconFlow identifies SiliconFlow routing.
	ProviderNameSiliconFlow = "siliconflow"
	// ProviderNameZhipu identifies Zhipu/GLM routing.
	ProviderNameZhipu = "zhipu"
	// ProviderNameGemini identifies Google Gemini routing.
	ProviderNameGemini = "gemini"
)

const (
	providerAliasQwen = "qwen"
	providerAliasKimi = "kimi"
	providerAliasGLM  = "glm"
)

const (
	defaultDeepSeekBaseURL     = "https://api.deepseek.com"
	defaultDashScopeBaseURL    = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	defaultMoonshotBaseURL     = "https://api.moonshot.ai/v1"
	defaultSiliconFlowBaseURL  = "https://api.siliconflow.com/v1"
	defaultZhipuBaseURL        = "https://open.bigmodel.cn/api/paas/v4"
	defaultGeminiBaseURL       = "https://generativelanguage.googleapis.com/v1"
	defaultSiliconFlowSTTModel = "FunAudioLLM/SenseVoiceSmall"
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
	// ModelNameMoonshotKimi identifies the Kimi K2 preview model.
	ModelNameMoonshotKimi = "kimi-k2-0905-preview"
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

type providerDefinition struct {
	identifier                providerID
	aliases                   []string
	textAPIKey                string
	textBaseURL               string
	transcriptionAPIKey       string
	transcriptionsURL         string
	defaultTextModel          modelID
	defaultTranscriptionModel modelID
	textModels                map[string]modelID
	transcriptionModels       map[string]modelID
	supportsDictation         bool
	supportsWebSearch         bool
	textTransport             providerTextTransport
}

func (definition providerDefinition) credentialFor(endpoint endpointKind) string {
	if endpoint == endpointKindDictation {
		return strings.TrimSpace(definition.transcriptionAPIKey)
	}
	return strings.TrimSpace(definition.textAPIKey)
}
