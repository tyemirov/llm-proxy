package proxy

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/apperrors"
	"github.com/tyemirov/llm-proxy/internal/constants"
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
	MaxPromptBytes               int64
	DictationModel               string
	MaxInputAudioBytes           int64
	Endpoints                    *Endpoints
	validated                    bool
}

// NewConfiguration returns a normalized runtime configuration after validating startup invariants.
func NewConfiguration(configuration Configuration) (Configuration, error) {
	configuration.ApplyTunables()
	if validationError := validateConfig(configuration); validationError != nil {
		return Configuration{}, validationError
	}
	configuration.validated = true
	return configuration, nil
}

func ensureValidatedConfiguration(configuration Configuration) (Configuration, error) {
	if configuration.validated {
		return configuration, nil
	}
	return NewConfiguration(configuration)
}

func validateConfig(configuration Configuration) error {
	if strings.TrimSpace(configuration.ServiceSecret) == constants.EmptyString {
		return apperrors.ErrMissingServiceSecret
	}
	if credentialError := validateDefaultProviderCredential(configuration.DefaultProvider, configuration); credentialError != nil {
		return credentialError
	}
	if credentialError := validateDefaultDictationProviderCredential(configuration.DefaultDictationProvider, configuration); credentialError != nil {
		return credentialError
	}
	return nil
}

func validateDefaultProviderCredential(providerIdentifier string, configuration Configuration) error {
	switch strings.ToLower(strings.TrimSpace(providerIdentifier)) {
	case ProviderNameOpenAI:
		if strings.TrimSpace(configuration.OpenAIKey) == constants.EmptyString {
			return apperrors.ErrMissingOpenAIKey
		}
	case ProviderNameDeepSeek:
		if strings.TrimSpace(configuration.DeepSeekKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s", ErrProviderNotConfigured, ProviderNameDeepSeek)
		}
	case ProviderNameDashScope, providerAliasQwen:
		if strings.TrimSpace(configuration.DashScopeKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s", ErrProviderNotConfigured, ProviderNameDashScope)
		}
	case ProviderNameMoonshot, providerAliasKimi:
		if strings.TrimSpace(configuration.MoonshotKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s", ErrProviderNotConfigured, ProviderNameMoonshot)
		}
	case ProviderNameSiliconFlow:
		if strings.TrimSpace(configuration.SiliconFlowKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s", ErrProviderNotConfigured, ProviderNameSiliconFlow)
		}
	case ProviderNameZhipu, providerAliasGLM:
		if strings.TrimSpace(configuration.ZhipuKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s", ErrProviderNotConfigured, ProviderNameZhipu)
		}
	case ProviderNameGemini:
		if strings.TrimSpace(configuration.GeminiKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s", ErrProviderNotConfigured, ProviderNameGemini)
		}
	default:
		return fmt.Errorf("%w: %s", ErrUnknownProvider, providerIdentifier)
	}
	return nil
}

func validateDefaultDictationProviderCredential(providerIdentifier string, configuration Configuration) error {
	switch strings.ToLower(strings.TrimSpace(providerIdentifier)) {
	case ProviderNameOpenAI:
		if strings.TrimSpace(configuration.OpenAIKey) == constants.EmptyString {
			return apperrors.ErrMissingOpenAIKey
		}
	case ProviderNameSiliconFlow:
		if strings.TrimSpace(configuration.SiliconFlowKey) == constants.EmptyString {
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
	configuration.ServiceSecret = strings.TrimSpace(configuration.ServiceSecret)
	configuration.OpenAIKey = strings.TrimSpace(configuration.OpenAIKey)
	configuration.DeepSeekKey = strings.TrimSpace(configuration.DeepSeekKey)
	configuration.DashScopeKey = strings.TrimSpace(configuration.DashScopeKey)
	configuration.MoonshotKey = strings.TrimSpace(configuration.MoonshotKey)
	configuration.SiliconFlowKey = strings.TrimSpace(configuration.SiliconFlowKey)
	configuration.ZhipuKey = strings.TrimSpace(configuration.ZhipuKey)
	configuration.GeminiKey = strings.TrimSpace(configuration.GeminiKey)
	if configuration.RequestTimeoutSeconds <= 0 {
		configuration.RequestTimeoutSeconds = DefaultRequestTimeoutSeconds
	}
	if configuration.UpstreamPollTimeoutSeconds <= 0 {
		configuration.UpstreamPollTimeoutSeconds = DefaultUpstreamPollTimeoutSeconds
	}
	if strings.TrimSpace(configuration.DefaultProvider) == constants.EmptyString {
		configuration.DefaultProvider = DefaultProvider
	}
	configuration.DefaultProvider = strings.ToLower(strings.TrimSpace(configuration.DefaultProvider))
	if strings.TrimSpace(configuration.DefaultModel) == constants.EmptyString {
		configuration.DefaultModel = DefaultModel
	}
	configuration.DefaultModel = strings.TrimSpace(configuration.DefaultModel)
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
	configuration.DictationModel = strings.TrimSpace(configuration.DictationModel)
	if configuration.MaxInputAudioBytes <= 0 {
		configuration.MaxInputAudioBytes = DefaultMaxInputAudioBytes
	}
	configuration.DeepSeekBaseURL = strings.TrimSpace(configuration.DeepSeekBaseURL)
	if strings.TrimSpace(configuration.DeepSeekBaseURL) == constants.EmptyString {
		configuration.DeepSeekBaseURL = defaultDeepSeekBaseURL
	}
	configuration.DashScopeBaseURL = strings.TrimSpace(configuration.DashScopeBaseURL)
	if strings.TrimSpace(configuration.DashScopeBaseURL) == constants.EmptyString {
		configuration.DashScopeBaseURL = defaultDashScopeBaseURL
	}
	configuration.MoonshotBaseURL = strings.TrimSpace(configuration.MoonshotBaseURL)
	if strings.TrimSpace(configuration.MoonshotBaseURL) == constants.EmptyString {
		configuration.MoonshotBaseURL = defaultMoonshotBaseURL
	}
	configuration.SiliconFlowBaseURL = strings.TrimSpace(configuration.SiliconFlowBaseURL)
	if strings.TrimSpace(configuration.SiliconFlowBaseURL) == constants.EmptyString {
		configuration.SiliconFlowBaseURL = defaultSiliconFlowBaseURL
	}
	configuration.SiliconFlowTranscriptionsURL = strings.TrimSpace(configuration.SiliconFlowTranscriptionsURL)
	if strings.TrimSpace(configuration.SiliconFlowTranscriptionsURL) == constants.EmptyString {
		configuration.SiliconFlowTranscriptionsURL = strings.TrimRight(configuration.SiliconFlowBaseURL, "/") + "/audio/transcriptions"
	}
	configuration.ZhipuBaseURL = strings.TrimSpace(configuration.ZhipuBaseURL)
	if strings.TrimSpace(configuration.ZhipuBaseURL) == constants.EmptyString {
		configuration.ZhipuBaseURL = defaultZhipuBaseURL
	}
	configuration.GeminiBaseURL = strings.TrimSpace(configuration.GeminiBaseURL)
	if strings.TrimSpace(configuration.GeminiBaseURL) == constants.EmptyString {
		configuration.GeminiBaseURL = defaultGeminiBaseURL
	}
}
