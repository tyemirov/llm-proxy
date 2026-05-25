package proxy

import (
	"fmt"
	"strings"

	"github.com/temirov/llm-proxy/internal/constants"
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
)

type endpointKind string

const (
	endpointKindText      endpointKind = "text"
	endpointKindDictation endpointKind = "dictation"
)

type providerID string

func newProviderID(rawIdentifier string) (providerID, error) {
	normalizedIdentifier := strings.ToLower(strings.TrimSpace(rawIdentifier))
	if normalizedIdentifier == constants.EmptyString {
		return providerID(""), fmt.Errorf("%w: empty provider", ErrUnknownProvider)
	}
	return providerID(normalizedIdentifier), nil
}

func (identifier providerID) string() string {
	return string(identifier)
}

type modelID string

func newModelID(rawIdentifier string) (modelID, error) {
	normalizedIdentifier := strings.TrimSpace(rawIdentifier)
	if normalizedIdentifier == constants.EmptyString {
		return modelID(""), fmt.Errorf("%w: empty model", ErrUnknownModel)
	}
	return modelID(normalizedIdentifier), nil
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
	supportsText              bool
	supportsDictation         bool
	supportsWebSearch         bool
	usesOpenAIResponses       bool
}

func (definition providerDefinition) credentialFor(endpoint endpointKind) string {
	if endpoint == endpointKindDictation {
		return strings.TrimSpace(definition.transcriptionAPIKey)
	}
	return strings.TrimSpace(definition.textAPIKey)
}

func (definition providerDefinition) supports(endpoint endpointKind) bool {
	switch endpoint {
	case endpointKindText:
		return definition.supportsText
	case endpointKindDictation:
		return definition.supportsDictation
	default:
		return false
	}
}
