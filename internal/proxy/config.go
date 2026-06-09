package proxy

import (
	"errors"
	"fmt"
	"strings"

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

	DefaultRequestTimeoutSeconds = 240 // overall app-side request timeout
	// DefaultMaxPromptBytes limits JSON LLM request bodies accepted by POST /.
	DefaultMaxPromptBytes     = 4 * 1024 * 1024
	DefaultDictationModel     = "gpt-4o-mini-transcribe"
	DefaultMaxInputAudioBytes = 25 * 1024 * 1024
)

// Configuration holds runtime settings.
type Configuration struct {
	Tenants                      []TenantConfiguration
	OpenAIKey                    string
	DeepSeekKey                  string
	DashScopeKey                 string
	MoonshotKey                  string
	SiliconFlowKey               string
	ZhipuKey                     string
	GeminiKey                    string
	AnthropicKey                 string
	GrokKey                      string
	OpenAIBaseURL                string
	OpenAITranscriptionsURL      string
	DeepSeekBaseURL              string
	DashScopeBaseURL             string
	MoonshotBaseURL              string
	SiliconFlowBaseURL           string
	SiliconFlowTranscriptionsURL string
	ZhipuBaseURL                 string
	ZhipuTranscriptionsURL       string
	GeminiBaseURL                string
	AnthropicBaseURL             string
	GrokBaseURL                  string
	GrokTranscriptionsURL        string
	Port                         int
	LogLevel                     string
	WorkerCount                  int
	QueueSize                    int
	RequestTimeoutSeconds        int
	MaxPromptBytes               int64
	MaxInputAudioBytes           int64
	Endpoints                    *Endpoints
	ProviderModels               ProviderModelCatalogs
	tenants                      tenantRegistry
	validated                    bool
}

// NewConfiguration returns a normalized runtime configuration after validating startup invariants.
func NewConfiguration(configuration Configuration) (Configuration, error) {
	configuration.ApplyTunables()
	tenants, validationError := validateConfig(configuration)
	if validationError != nil {
		return Configuration{}, validationError
	}
	configuration.tenants = tenants
	configuration.validated = true
	return configuration, nil
}

func ensureValidatedConfiguration(configuration Configuration) (Configuration, error) {
	if configuration.validated {
		return configuration, nil
	}
	return NewConfiguration(configuration)
}

func validateConfig(configuration Configuration) (tenantRegistry, error) {
	tenants, tenantError := newTenantRegistry(configuration.Tenants)
	if tenantError != nil {
		return tenantRegistry{}, tenantError
	}
	if modelCatalogError := validateProviderModelCatalogs(configuration.ProviderModels); modelCatalogError != nil {
		return tenantRegistry{}, modelCatalogError
	}
	providers := newProviderRegistry(configuration)
	validator := newModelValidator(providers)
	for _, currentTenant := range tenants.tenants {
		if _, _, verificationError := validator.ResolveText(constants.EmptyString, constants.EmptyString, currentTenant.defaults.provider, currentTenant.defaults.model, false); verificationError != nil {
			return tenantRegistry{}, fmt.Errorf("%w: tenant=%s", verificationError, currentTenant.identifier.string())
		}
		if _, _, verificationError := validator.ResolveDictation(constants.EmptyString, constants.EmptyString, currentTenant.defaults.dictationProvider, currentTenant.defaults.dictationModel); verificationError != nil {
			return tenantRegistry{}, fmt.Errorf("%w: tenant=%s", verificationError, currentTenant.identifier.string())
		}
	}
	return tenants, nil
}

// ErrUpstreamIncomplete indicates that the upstream provider returned an incomplete response before the poll deadline.
var ErrUpstreamIncomplete = errors.New(errorUpstreamIncomplete)

// ApplyTunables ensures tunable configuration values have sensible defaults.
func (configuration *Configuration) ApplyTunables() {
	configuration.OpenAIKey = strings.TrimSpace(configuration.OpenAIKey)
	configuration.DeepSeekKey = strings.TrimSpace(configuration.DeepSeekKey)
	configuration.DashScopeKey = strings.TrimSpace(configuration.DashScopeKey)
	configuration.MoonshotKey = strings.TrimSpace(configuration.MoonshotKey)
	configuration.SiliconFlowKey = strings.TrimSpace(configuration.SiliconFlowKey)
	configuration.ZhipuKey = strings.TrimSpace(configuration.ZhipuKey)
	configuration.GeminiKey = strings.TrimSpace(configuration.GeminiKey)
	configuration.AnthropicKey = strings.TrimSpace(configuration.AnthropicKey)
	configuration.GrokKey = strings.TrimSpace(configuration.GrokKey)
	if configuration.RequestTimeoutSeconds <= 0 {
		configuration.RequestTimeoutSeconds = DefaultRequestTimeoutSeconds
	}
	if configuration.MaxPromptBytes <= 0 {
		configuration.MaxPromptBytes = DefaultMaxPromptBytes
	}
	if configuration.MaxInputAudioBytes <= 0 {
		configuration.MaxInputAudioBytes = DefaultMaxInputAudioBytes
	}
	configuration.OpenAIBaseURL = strings.TrimSpace(configuration.OpenAIBaseURL)
	if strings.TrimSpace(configuration.OpenAIBaseURL) == constants.EmptyString {
		configuration.OpenAIBaseURL = defaultOpenAIBaseURL
	}
	configuration.OpenAITranscriptionsURL = strings.TrimSpace(configuration.OpenAITranscriptionsURL)
	if strings.TrimSpace(configuration.OpenAITranscriptionsURL) == constants.EmptyString {
		configuration.OpenAITranscriptionsURL = defaultTranscriptionsURL
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
	configuration.ZhipuTranscriptionsURL = strings.TrimSpace(configuration.ZhipuTranscriptionsURL)
	if strings.TrimSpace(configuration.ZhipuTranscriptionsURL) == constants.EmptyString {
		configuration.ZhipuTranscriptionsURL = defaultZhipuTranscriptionsURL
	}
	configuration.GeminiBaseURL = strings.TrimSpace(configuration.GeminiBaseURL)
	if strings.TrimSpace(configuration.GeminiBaseURL) == constants.EmptyString {
		configuration.GeminiBaseURL = defaultGeminiBaseURL
	}
	configuration.AnthropicBaseURL = strings.TrimSpace(configuration.AnthropicBaseURL)
	if strings.TrimSpace(configuration.AnthropicBaseURL) == constants.EmptyString {
		configuration.AnthropicBaseURL = defaultAnthropicBaseURL
	}
	configuration.GrokBaseURL = strings.TrimSpace(configuration.GrokBaseURL)
	if strings.TrimSpace(configuration.GrokBaseURL) == constants.EmptyString {
		configuration.GrokBaseURL = defaultGrokBaseURL
	}
	configuration.GrokTranscriptionsURL = strings.TrimSpace(configuration.GrokTranscriptionsURL)
	if strings.TrimSpace(configuration.GrokTranscriptionsURL) == constants.EmptyString {
		configuration.GrokTranscriptionsURL = defaultGrokTranscriptionsURL
	}
}
