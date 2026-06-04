package proxy

import (
	"errors"
	"fmt"
	"strings"

	"github.com/temirov/llm-proxy/internal/apperrors"
	"github.com/temirov/llm-proxy/internal/constants"
)

const (
	// DefaultPort is the TCP port used by the HTTP server when no explicit port is provided.
	DefaultPort = 8080
	// DefaultWorkers is the number of worker goroutines that process upstream requests.
	DefaultWorkers = 4
	// DefaultQueueSize is the capacity of the internal request queue.
	DefaultQueueSize = 100
	// DefaultModel is the model identifier used when the client does not supply one.
	DefaultModel = ModelNameGPT41
	// DefaultProvider is the provider identifier used when the client does not supply one.
	DefaultProvider = ProviderNameOpenAI
	// DefaultDictationProvider is the provider used when /dictate does not supply one.
	DefaultDictationProvider = ProviderNameOpenAI

	DefaultRequestTimeoutSeconds      = 180 // overall app-side request timeout
	DefaultUpstreamPollTimeoutSeconds = 60  // poll budget after "incomplete"
	DefaultMaxOutputTokens            = 8192
	// DefaultMaxPromptBytes limits JSON LLM request bodies accepted by POST /.
	DefaultMaxPromptBytes     = 4 * 1024 * 1024
	DefaultDictationModel     = "gpt-4o-mini-transcribe"
	DefaultMaxInputAudioBytes = 25 * 1024 * 1024
)

// Configuration holds runtime settings.
type Configuration struct {
	ServiceSecret                string
	OpenAIKey                    string
	DeepSeekKey                  string
	DashScopeKey                 string
	MoonshotKey                  string
	SiliconFlowKey               string
	ZhipuKey                     string
	GeminiKey                    string
	DefaultProvider              string
	DefaultModel                 string
	DefaultDictationProvider     string
	DeepSeekBaseURL              string
	DashScopeBaseURL             string
	MoonshotBaseURL              string
	SiliconFlowBaseURL           string
	SiliconFlowTranscriptionsURL string
	ZhipuBaseURL                 string
	GeminiBaseURL                string
	Port                         int
	LogLevel                     string
	SystemPrompt                 string
	WorkerCount                  int
	QueueSize                    int
	RequestTimeoutSeconds        int
	UpstreamPollTimeoutSeconds   int
	MaxOutputTokens              int
	MaxPromptBytes               int64
	DictationModel               string
	MaxInputAudioBytes           int64
	Endpoints                    *Endpoints
}

// validateConfig confirms required settings are present.
func validateConfig(config Configuration) error {
	if strings.TrimSpace(config.ServiceSecret) == constants.EmptyString {
		return apperrors.ErrMissingServiceSecret
	}
	if credentialError := validateDefaultProviderCredential(config.DefaultProvider, config); credentialError != nil {
		return credentialError
	}
	if credentialError := validateDefaultDictationProviderCredential(config.DefaultDictationProvider, config); credentialError != nil {
		return credentialError
	}
	return nil
}

func validateDefaultProviderCredential(providerIdentifier string, config Configuration) error {
	switch strings.ToLower(strings.TrimSpace(providerIdentifier)) {
	case ProviderNameOpenAI:
		if strings.TrimSpace(config.OpenAIKey) == constants.EmptyString {
			return apperrors.ErrMissingOpenAIKey
		}
	case ProviderNameDeepSeek:
		if strings.TrimSpace(config.DeepSeekKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s", ErrProviderNotConfigured, ProviderNameDeepSeek)
		}
	case ProviderNameDashScope, providerAliasQwen:
		if strings.TrimSpace(config.DashScopeKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s", ErrProviderNotConfigured, ProviderNameDashScope)
		}
	case ProviderNameMoonshot, providerAliasKimi:
		if strings.TrimSpace(config.MoonshotKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s", ErrProviderNotConfigured, ProviderNameMoonshot)
		}
	case ProviderNameSiliconFlow:
		if strings.TrimSpace(config.SiliconFlowKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s", ErrProviderNotConfigured, ProviderNameSiliconFlow)
		}
	case ProviderNameZhipu, providerAliasGLM:
		if strings.TrimSpace(config.ZhipuKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s", ErrProviderNotConfigured, ProviderNameZhipu)
		}
	case ProviderNameGemini:
		if strings.TrimSpace(config.GeminiKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s", ErrProviderNotConfigured, ProviderNameGemini)
		}
	default:
		return fmt.Errorf("%w: %s", ErrUnknownProvider, providerIdentifier)
	}
	return nil
}

func validateDefaultDictationProviderCredential(providerIdentifier string, config Configuration) error {
	switch strings.ToLower(strings.TrimSpace(providerIdentifier)) {
	case ProviderNameOpenAI:
		if strings.TrimSpace(config.OpenAIKey) == constants.EmptyString {
			return apperrors.ErrMissingOpenAIKey
		}
	case ProviderNameSiliconFlow:
		if strings.TrimSpace(config.SiliconFlowKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s endpoint=%s", ErrProviderNotConfigured, ProviderNameSiliconFlow, endpointKindDictation)
		}
	case ProviderNameDeepSeek:
		return fmt.Errorf("%w: provider=%s endpoint=%s", ErrUnsupportedEndpoint, ProviderNameDeepSeek, endpointKindDictation)
	case ProviderNameDashScope, providerAliasQwen:
		return fmt.Errorf("%w: provider=%s endpoint=%s", ErrUnsupportedEndpoint, ProviderNameDashScope, endpointKindDictation)
	case ProviderNameMoonshot, providerAliasKimi:
		return fmt.Errorf("%w: provider=%s endpoint=%s", ErrUnsupportedEndpoint, ProviderNameMoonshot, endpointKindDictation)
	case ProviderNameZhipu, providerAliasGLM:
		return fmt.Errorf("%w: provider=%s endpoint=%s", ErrUnsupportedEndpoint, ProviderNameZhipu, endpointKindDictation)
	case ProviderNameGemini:
		return fmt.Errorf("%w: provider=%s endpoint=%s", ErrUnsupportedEndpoint, ProviderNameGemini, endpointKindDictation)
	default:
		return fmt.Errorf("%w: %s", ErrUnknownProvider, providerIdentifier)
	}
	return nil
}

// ErrUpstreamIncomplete indicates that the upstream provider returned an incomplete response before the poll deadline.
var ErrUpstreamIncomplete = errors.New(errorUpstreamIncomplete)

// ApplyTunables ensures tunable configuration values have sensible defaults.
func (configuration *Configuration) ApplyTunables() {
	if configuration.RequestTimeoutSeconds <= 0 {
		configuration.RequestTimeoutSeconds = DefaultRequestTimeoutSeconds
	}
	if configuration.UpstreamPollTimeoutSeconds <= 0 {
		configuration.UpstreamPollTimeoutSeconds = DefaultUpstreamPollTimeoutSeconds
	}
	if configuration.MaxOutputTokens <= 0 {
		configuration.MaxOutputTokens = DefaultMaxOutputTokens
	}
	if strings.TrimSpace(configuration.DefaultProvider) == constants.EmptyString {
		configuration.DefaultProvider = DefaultProvider
	}
	configuration.DefaultProvider = strings.ToLower(strings.TrimSpace(configuration.DefaultProvider))
	if strings.TrimSpace(configuration.DefaultModel) == constants.EmptyString {
		configuration.DefaultModel = DefaultModel
	}
	if strings.TrimSpace(configuration.DefaultDictationProvider) == constants.EmptyString {
		configuration.DefaultDictationProvider = DefaultDictationProvider
	}
	configuration.DefaultDictationProvider = strings.ToLower(strings.TrimSpace(configuration.DefaultDictationProvider))
	if configuration.MaxPromptBytes <= 0 {
		configuration.MaxPromptBytes = DefaultMaxPromptBytes
	}
	if strings.TrimSpace(configuration.DictationModel) == constants.EmptyString {
		configuration.DictationModel = DefaultDictationModel
	}
	if configuration.MaxInputAudioBytes <= 0 {
		configuration.MaxInputAudioBytes = DefaultMaxInputAudioBytes
	}
	if strings.TrimSpace(configuration.DeepSeekBaseURL) == constants.EmptyString {
		configuration.DeepSeekBaseURL = defaultDeepSeekBaseURL
	}
	if strings.TrimSpace(configuration.DashScopeBaseURL) == constants.EmptyString {
		configuration.DashScopeBaseURL = defaultDashScopeBaseURL
	}
	if strings.TrimSpace(configuration.MoonshotBaseURL) == constants.EmptyString {
		configuration.MoonshotBaseURL = defaultMoonshotBaseURL
	}
	if strings.TrimSpace(configuration.SiliconFlowBaseURL) == constants.EmptyString {
		configuration.SiliconFlowBaseURL = defaultSiliconFlowBaseURL
	}
	if strings.TrimSpace(configuration.SiliconFlowTranscriptionsURL) == constants.EmptyString {
		configuration.SiliconFlowTranscriptionsURL = strings.TrimRight(configuration.SiliconFlowBaseURL, "/") + "/audio/transcriptions"
	}
	if strings.TrimSpace(configuration.ZhipuBaseURL) == constants.EmptyString {
		configuration.ZhipuBaseURL = defaultZhipuBaseURL
	}
	if strings.TrimSpace(configuration.GeminiBaseURL) == constants.EmptyString {
		configuration.GeminiBaseURL = defaultGeminiBaseURL
	}
}
