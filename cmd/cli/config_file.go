package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

const (
	configFileType = "yaml"
	dotEnvFileName = ".env"
)

const (
	providerAliasQwen   = "qwen"
	providerAliasKimi   = "kimi"
	providerAliasGLM    = "glm"
	providerAliasClaude = "claude"
	providerAliasXAI    = "xai"
)

var (
	errConfigFileRead            = errors.New("config_file_read_failed")
	errConfigFileParse           = errors.New("config_file_parse_failed")
	errConfigEnvironmentRead     = errors.New("config_environment_read_failed")
	errConfigPlaceholderMissing  = errors.New("config_placeholder_missing")
	errConfigInvalid             = errors.New("config_invalid")
	errProviderAPIKeyRequired    = errors.New("provider_api_key_required")
	errProviderBaseURLRequired   = errors.New("provider_base_url_required")
	errTranscriptionsURLRequired = errors.New("provider_transcriptions_url_required")
	placeholderPattern           = regexp.MustCompile(`\$\{([^}]+)\}`)
	placeholderNamePattern       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	optionalAPIKeyLinePattern    = regexp.MustCompile(`^\s*api_key:\s*(?:"\$\{([A-Za-z_][A-Za-z0-9_]*)\}"|'\$\{([A-Za-z_][A-Za-z0-9_]*)\}'|\$\{([A-Za-z_][A-Za-z0-9_]*)\})\s*(?:#.*)?$`)
	readConfigBytes              = os.ReadFile
	readDotEnvFile               = gotenv.Read
	processEnvironment           = os.Environ
)

type fileConfiguration struct {
	Server     serverConfiguration     `mapstructure:"server"`
	Management managementConfiguration `mapstructure:"management"`
	Tenants    []tenantConfiguration   `mapstructure:"tenants"`
	Providers  providersConfiguration  `mapstructure:"providers"`
}

type serverConfiguration struct {
	Port                  int                              `mapstructure:"port"`
	LogLevel              string                           `mapstructure:"log_level"`
	Workers               int                              `mapstructure:"workers"`
	QueueSize             int                              `mapstructure:"queue_size"`
	RequestTimeoutSeconds int                              `mapstructure:"request_timeout_seconds"`
	MaxPromptBytes        int64                            `mapstructure:"max_prompt_bytes"`
	MaxInputAudioBytes    int64                            `mapstructure:"max_input_audio_bytes"`
	UpstreamRateLimits    []upstreamRateLimitConfiguration `mapstructure:"upstream_rate_limits"`
}

type upstreamRateLimitConfiguration struct {
	Origin      string `mapstructure:"origin"`
	MaxRequests int    `mapstructure:"max_requests"`
	Interval    string `mapstructure:"interval"`
}

type tenantConfiguration struct {
	ID       string               `mapstructure:"id"`
	Secret   string               `mapstructure:"secret"`
	Defaults tenantDefaultsConfig `mapstructure:"defaults"`
}

type managementConfiguration struct {
	Enabled                  bool                              `mapstructure:"enabled"`
	PublicOrigin             string                            `mapstructure:"public_origin"`
	UIDescription            string                            `mapstructure:"ui_description"`
	UIOrigins                []string                          `mapstructure:"ui_origins"`
	AdminEmails              []string                          `mapstructure:"admin_emails"`
	TAuthURL                 string                            `mapstructure:"tauth_url"`
	TAuthTenantID            string                            `mapstructure:"tauth_tenant_id"`
	GoogleClientID           string                            `mapstructure:"google_client_id"`
	LoginPath                string                            `mapstructure:"login_path"`
	LogoutPath               string                            `mapstructure:"logout_path"`
	NoncePath                string                            `mapstructure:"nonce_path"`
	JWTSigningKey            string                            `mapstructure:"jwt_signing_key"`
	JWTIssuer                string                            `mapstructure:"jwt_issuer"`
	SessionCookieName        string                            `mapstructure:"session_cookie_name"`
	DatabaseDialect          string                            `mapstructure:"database_dialect"`
	DatabaseDSN              string                            `mapstructure:"database_dsn"`
	ProviderKeyEncryptionKey string                            `mapstructure:"provider_key_encryption_key"`
	ManagementAPIOrigin      string                            `mapstructure:"management_api_origin"`
	ProxyOrigin              string                            `mapstructure:"proxy_origin"`
	LegacyTokenMigration     legacyTokenMigrationConfiguration `mapstructure:"legacy_token_migration"`
}

type legacyTokenMigrationConfiguration struct {
	TenantID   string `mapstructure:"tenant_id"`
	OwnerEmail string `mapstructure:"owner_email"`
}

type tenantDefaultsConfig struct {
	Provider          string `mapstructure:"provider"`
	Model             string `mapstructure:"model"`
	DictationProvider string `mapstructure:"dictation_provider"`
	DictationModel    string `mapstructure:"dictation_model"`
	SystemPrompt      string `mapstructure:"system_prompt"`
}

type providersConfiguration struct {
	OpenAI      transcribingProviderConfiguration `mapstructure:"openai"`
	DeepSeek    providerConfiguration             `mapstructure:"deepseek"`
	DashScope   providerConfiguration             `mapstructure:"dashscope"`
	Moonshot    providerConfiguration             `mapstructure:"moonshot"`
	SiliconFlow transcribingProviderConfiguration `mapstructure:"siliconflow"`
	Zhipu       transcribingProviderConfiguration `mapstructure:"zhipu"`
	Gemini      providerConfiguration             `mapstructure:"gemini"`
	Anthropic   providerConfiguration             `mapstructure:"anthropic"`
	Meta        providerConfiguration             `mapstructure:"meta"`
	Grok        transcribingProviderConfiguration `mapstructure:"grok"`
}

type providerConfiguration struct {
	APIKey  string                     `mapstructure:"api_key"`
	BaseURL string                     `mapstructure:"base_url"`
	Text    modelEndpointConfiguration `mapstructure:"text"`
}

type transcribingProviderConfiguration struct {
	APIKey            string                     `mapstructure:"api_key"`
	BaseURL           string                     `mapstructure:"base_url"`
	TranscriptionsURL string                     `mapstructure:"transcriptions_url"`
	Text              modelEndpointConfiguration `mapstructure:"text"`
	Dictation         modelEndpointConfiguration `mapstructure:"dictation"`
}

type modelEndpointConfiguration struct {
	DefaultModel string               `mapstructure:"default_model"`
	Models       []modelConfiguration `mapstructure:"models"`
}

type modelConfiguration struct {
	ID               string `mapstructure:"id"`
	RequestProfile   string `mapstructure:"request_profile"`
	WebSearch        bool   `mapstructure:"web_search"`
	OutputTokenLimit int    `mapstructure:"output_token_limit"`
}

type providerAPIKeyRequirement struct {
	providerName string
	fieldName    string
	apiKey       string
}

func loadRuntimeConfiguration(rawConfigPath string) (proxy.Configuration, error) {
	configPath := normalizedConfigPath(rawConfigPath)
	configBytes, readError := readConfigBytes(configPath)
	if readError != nil {
		return proxy.Configuration{}, fmt.Errorf("%w: path=%s: %v", errConfigFileRead, configPath, readError)
	}
	expansionEnvironment, environmentError := configurationExpansionEnvironment(configPath)
	if environmentError != nil {
		return proxy.Configuration{}, environmentError
	}
	expandedConfig, expansionError := expandConfigPlaceholders(string(configBytes), expansionEnvironment)
	if expansionError != nil {
		return proxy.Configuration{}, fmt.Errorf("%w: path=%s: %v", errConfigFileParse, configPath, expansionError)
	}

	configReader := viper.New()
	configReader.SetConfigType(configFileType)
	if readConfigError := configReader.ReadConfig(strings.NewReader(expandedConfig)); readConfigError != nil {
		return proxy.Configuration{}, fmt.Errorf("%w: path=%s: %v", errConfigFileParse, configPath, readConfigError)
	}

	var parsedConfiguration fileConfiguration
	if unmarshalError := configReader.UnmarshalExact(&parsedConfiguration); unmarshalError != nil {
		return proxy.Configuration{}, fmt.Errorf("%w: path=%s: %v", errConfigFileParse, configPath, unmarshalError)
	}
	runtimeConfig, configError := parsedConfiguration.toProxyConfiguration()
	if configError != nil {
		return proxy.Configuration{}, fmt.Errorf("%w: path=%s: %v", errConfigInvalid, configPath, configError)
	}
	return runtimeConfig, nil
}

func normalizedConfigPath(rawConfigPath string) string {
	configPath := strings.TrimSpace(rawConfigPath)
	if configPath == constants.EmptyString {
		return defaultConfigPath
	}
	return configPath
}

func configurationExpansionEnvironment(configPath string) (map[string]string, error) {
	expansionEnvironment := map[string]string{}
	dotEnvPath := filepath.Join(filepath.Dir(configPath), dotEnvFileName)
	dotEnvValues, dotEnvError := readDotEnvFile(dotEnvPath)
	if dotEnvError != nil && !os.IsNotExist(dotEnvError) {
		return nil, fmt.Errorf("%w: path=%s: %v", errConfigEnvironmentRead, dotEnvPath, dotEnvError)
	}
	for variableName, variableValue := range dotEnvValues {
		expansionEnvironment[variableName] = variableValue
	}
	for _, environmentValue := range processEnvironment() {
		variableName, variableValue, _ := strings.Cut(environmentValue, "=")
		expansionEnvironment[variableName] = variableValue
	}
	return expansionEnvironment, nil
}

func expandConfigPlaceholders(configContent string, expansionEnvironment map[string]string) (string, error) {
	missingPlaceholders := map[string]struct{}{}
	var expandedConfig strings.Builder
	for _, configLine := range strings.SplitAfter(configContent, "\n") {
		expandedLine := placeholderPattern.ReplaceAllStringFunc(configLine, func(placeholder string) string {
			placeholderMatches := placeholderPattern.FindStringSubmatch(placeholder)
			placeholderName := placeholderMatches[1]
			if !placeholderNamePattern.MatchString(placeholderName) {
				missingPlaceholders[placeholderName] = struct{}{}
				return placeholder
			}
			placeholderValue, foundValue := expansionEnvironment[placeholderName]
			if foundValue {
				return placeholderValue
			}
			if optionalMissingAPIKeyPlaceholder(configLine, placeholderName) {
				return constants.EmptyString
			}
			missingPlaceholders[placeholderName] = struct{}{}
			return placeholder
		})
		expandedConfig.WriteString(expandedLine)
	}
	if len(missingPlaceholders) > 0 {
		missingNames := make([]string, 0, len(missingPlaceholders))
		for placeholderName := range missingPlaceholders {
			missingNames = append(missingNames, placeholderName)
		}
		sort.Strings(missingNames)
		return constants.EmptyString, fmt.Errorf("%w: names=%s", errConfigPlaceholderMissing, strings.Join(missingNames, ","))
	}
	return expandedConfig.String(), nil
}

func optionalMissingAPIKeyPlaceholder(configLine string, placeholderName string) bool {
	trimmedLine := strings.TrimRight(configLine, "\r\n")
	placeholderLineMatches := optionalAPIKeyLinePattern.FindStringSubmatch(trimmedLine)
	if len(placeholderLineMatches) == 0 {
		return false
	}
	return placeholderLineMatches[1] == placeholderName ||
		placeholderLineMatches[2] == placeholderName ||
		placeholderLineMatches[3] == placeholderName
}

func (configuration fileConfiguration) toProxyConfiguration() (proxy.Configuration, error) {
	if configurationValidationError := configuration.validateCompleteConfiguration(); configurationValidationError != nil {
		return proxy.Configuration{}, configurationValidationError
	}
	return proxy.NewConfiguration(proxy.Configuration{
		Tenants:                      tenantConfigurations(configuration.Tenants),
		Management:                   managementProxyConfiguration(configuration.Management),
		OpenAIKey:                    configuration.Providers.OpenAI.APIKey,
		DeepSeekKey:                  configuration.Providers.DeepSeek.APIKey,
		DashScopeKey:                 configuration.Providers.DashScope.APIKey,
		MoonshotKey:                  configuration.Providers.Moonshot.APIKey,
		SiliconFlowKey:               configuration.Providers.SiliconFlow.APIKey,
		ZhipuKey:                     configuration.Providers.Zhipu.APIKey,
		GeminiKey:                    configuration.Providers.Gemini.APIKey,
		AnthropicKey:                 configuration.Providers.Anthropic.APIKey,
		MetaKey:                      configuration.Providers.Meta.APIKey,
		GrokKey:                      configuration.Providers.Grok.APIKey,
		OpenAIBaseURL:                configuration.Providers.OpenAI.BaseURL,
		OpenAITranscriptionsURL:      configuration.Providers.OpenAI.TranscriptionsURL,
		DeepSeekBaseURL:              configuration.Providers.DeepSeek.BaseURL,
		DashScopeBaseURL:             configuration.Providers.DashScope.BaseURL,
		MoonshotBaseURL:              configuration.Providers.Moonshot.BaseURL,
		SiliconFlowBaseURL:           configuration.Providers.SiliconFlow.BaseURL,
		SiliconFlowTranscriptionsURL: configuration.Providers.SiliconFlow.TranscriptionsURL,
		ZhipuBaseURL:                 configuration.Providers.Zhipu.BaseURL,
		ZhipuTranscriptionsURL:       configuration.Providers.Zhipu.TranscriptionsURL,
		GeminiBaseURL:                configuration.Providers.Gemini.BaseURL,
		AnthropicBaseURL:             configuration.Providers.Anthropic.BaseURL,
		MetaBaseURL:                  configuration.Providers.Meta.BaseURL,
		GrokBaseURL:                  configuration.Providers.Grok.BaseURL,
		GrokTranscriptionsURL:        configuration.Providers.Grok.TranscriptionsURL,
		Port:                         configuration.Server.Port,
		LogLevel:                     configuration.Server.LogLevel,
		WorkerCount:                  configuration.Server.Workers,
		QueueSize:                    configuration.Server.QueueSize,
		RequestTimeoutSeconds:        configuration.Server.RequestTimeoutSeconds,
		MaxPromptBytes:               configuration.Server.MaxPromptBytes,
		MaxInputAudioBytes:           configuration.Server.MaxInputAudioBytes,
		UpstreamRateLimits:           proxyUpstreamRateLimitConfigurations(configuration.Server.UpstreamRateLimits),
		ProviderModels:               configuration.Providers.providerModelCatalogs(),
	})
}

func proxyUpstreamRateLimitConfigurations(configurations []upstreamRateLimitConfiguration) []proxy.UpstreamRateLimitConfiguration {
	proxyConfigurations := make([]proxy.UpstreamRateLimitConfiguration, 0, len(configurations))
	for _, configuration := range configurations {
		proxyConfigurations = append(proxyConfigurations, proxy.UpstreamRateLimitConfiguration{
			Origin:      configuration.Origin,
			MaxRequests: configuration.MaxRequests,
			Interval:    configuration.Interval,
		})
	}
	return proxyConfigurations
}

func managementProxyConfiguration(configuration managementConfiguration) proxy.ManagementConfiguration {
	return proxy.ManagementConfiguration{
		Enabled:                  configuration.Enabled,
		PublicOrigin:             configuration.PublicOrigin,
		UIDescription:            configuration.UIDescription,
		UIOrigins:                configuration.UIOrigins,
		AdminEmails:              configuration.AdminEmails,
		TAuthURL:                 configuration.TAuthURL,
		TAuthTenantID:            configuration.TAuthTenantID,
		GoogleClientID:           configuration.GoogleClientID,
		LoginPath:                configuration.LoginPath,
		LogoutPath:               configuration.LogoutPath,
		NoncePath:                configuration.NoncePath,
		JWTSigningKey:            configuration.JWTSigningKey,
		JWTIssuer:                configuration.JWTIssuer,
		SessionCookieName:        configuration.SessionCookieName,
		DatabaseDialect:          configuration.DatabaseDialect,
		DatabaseDSN:              configuration.DatabaseDSN,
		ProviderKeyEncryptionKey: configuration.ProviderKeyEncryptionKey,
		ManagementAPIOrigin:      configuration.ManagementAPIOrigin,
		ProxyOrigin:              configuration.ProxyOrigin,
		LegacyTokenMigration: proxy.LegacyTokenMigrationConfiguration{
			TenantID:   configuration.LegacyTokenMigration.TenantID,
			OwnerEmail: configuration.LegacyTokenMigration.OwnerEmail,
		},
	}
}

func (configuration providersConfiguration) providerModelCatalogs() proxy.ProviderModelCatalogs {
	return proxy.ProviderModelCatalogs{
		proxy.ProviderNameOpenAI: {
			Text:      configuration.OpenAI.Text.proxyCatalog(),
			Dictation: configuration.OpenAI.Dictation.proxyCatalog(),
		},
		proxy.ProviderNameDeepSeek: {
			Text: configuration.DeepSeek.Text.proxyCatalog(),
		},
		proxy.ProviderNameDashScope: {
			Text: configuration.DashScope.Text.proxyCatalog(),
		},
		proxy.ProviderNameMoonshot: {
			Text: configuration.Moonshot.Text.proxyCatalog(),
		},
		proxy.ProviderNameSiliconFlow: {
			Text:      configuration.SiliconFlow.Text.proxyCatalog(),
			Dictation: configuration.SiliconFlow.Dictation.proxyCatalog(),
		},
		proxy.ProviderNameZhipu: {
			Text:      configuration.Zhipu.Text.proxyCatalog(),
			Dictation: configuration.Zhipu.Dictation.proxyCatalog(),
		},
		proxy.ProviderNameGemini: {
			Text: configuration.Gemini.Text.proxyCatalog(),
		},
		proxy.ProviderNameAnthropic: {
			Text: configuration.Anthropic.Text.proxyCatalog(),
		},
		proxy.ProviderNameMeta: {
			Text: configuration.Meta.Text.proxyCatalog(),
		},
		proxy.ProviderNameGrok: {
			Text:      configuration.Grok.Text.proxyCatalog(),
			Dictation: configuration.Grok.Dictation.proxyCatalog(),
		},
	}
}

func (configuration fileConfiguration) validateCompleteConfiguration() error {
	if providerValidationError := configuration.Providers.validateCompleteProviderConfiguration(); providerValidationError != nil {
		return providerValidationError
	}
	if configuration.Management.Enabled {
		return nil
	}
	return configuration.validateTenantDefaultProviderCredentials()
}

func (configuration modelEndpointConfiguration) proxyCatalog() proxy.ModelEndpointCatalog {
	models := make([]proxy.ModelConfiguration, 0, len(configuration.Models))
	for _, currentModel := range configuration.Models {
		models = append(models, proxy.ModelConfiguration{
			ID:               currentModel.ID,
			RequestProfile:   currentModel.RequestProfile,
			WebSearch:        currentModel.WebSearch,
			OutputTokenLimit: currentModel.OutputTokenLimit,
		})
	}
	return proxy.ModelEndpointCatalog{
		DefaultModel: configuration.DefaultModel,
		Models:       models,
	}
}

func (configuration providersConfiguration) validateCompleteProviderConfiguration() error {
	requiredBaseURLs := []struct {
		providerName string
		fieldName    string
		baseURL      string
	}{
		{providerName: proxy.ProviderNameOpenAI, fieldName: "providers.openai.base_url", baseURL: configuration.OpenAI.BaseURL},
		{providerName: proxy.ProviderNameDeepSeek, fieldName: "providers.deepseek.base_url", baseURL: configuration.DeepSeek.BaseURL},
		{providerName: proxy.ProviderNameDashScope, fieldName: "providers.dashscope.base_url", baseURL: configuration.DashScope.BaseURL},
		{providerName: proxy.ProviderNameMoonshot, fieldName: "providers.moonshot.base_url", baseURL: configuration.Moonshot.BaseURL},
		{providerName: proxy.ProviderNameSiliconFlow, fieldName: "providers.siliconflow.base_url", baseURL: configuration.SiliconFlow.BaseURL},
		{providerName: proxy.ProviderNameZhipu, fieldName: "providers.zhipu.base_url", baseURL: configuration.Zhipu.BaseURL},
		{providerName: proxy.ProviderNameGemini, fieldName: "providers.gemini.base_url", baseURL: configuration.Gemini.BaseURL},
		{providerName: proxy.ProviderNameAnthropic, fieldName: "providers.anthropic.base_url", baseURL: configuration.Anthropic.BaseURL},
		{providerName: proxy.ProviderNameMeta, fieldName: "providers.meta.base_url", baseURL: configuration.Meta.BaseURL},
		{providerName: proxy.ProviderNameGrok, fieldName: "providers.grok.base_url", baseURL: configuration.Grok.BaseURL},
	}
	for _, requiredBaseURL := range requiredBaseURLs {
		if strings.TrimSpace(requiredBaseURL.baseURL) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s field=%s", errProviderBaseURLRequired, requiredBaseURL.providerName, requiredBaseURL.fieldName)
		}
	}

	requiredTranscriptionsURLs := []struct {
		providerName      string
		fieldName         string
		transcriptionsURL string
	}{
		{providerName: proxy.ProviderNameOpenAI, fieldName: "providers.openai.transcriptions_url", transcriptionsURL: configuration.OpenAI.TranscriptionsURL},
		{providerName: proxy.ProviderNameSiliconFlow, fieldName: "providers.siliconflow.transcriptions_url", transcriptionsURL: configuration.SiliconFlow.TranscriptionsURL},
		{providerName: proxy.ProviderNameZhipu, fieldName: "providers.zhipu.transcriptions_url", transcriptionsURL: configuration.Zhipu.TranscriptionsURL},
		{providerName: proxy.ProviderNameGrok, fieldName: "providers.grok.transcriptions_url", transcriptionsURL: configuration.Grok.TranscriptionsURL},
	}
	for _, requiredTranscriptionsURL := range requiredTranscriptionsURLs {
		if strings.TrimSpace(requiredTranscriptionsURL.transcriptionsURL) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s field=%s", errTranscriptionsURLRequired, requiredTranscriptionsURL.providerName, requiredTranscriptionsURL.fieldName)
		}
	}
	return nil
}

func (configuration fileConfiguration) validateTenantDefaultProviderCredentials() error {
	for _, currentTenant := range configuration.Tenants {
		if requiredAPIKey, knownProvider := configuration.Providers.textAPIKeyRequirement(currentTenant.Defaults.Provider); knownProvider && strings.TrimSpace(requiredAPIKey.apiKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s field=%s tenant=%s endpoint=text", errProviderAPIKeyRequired, requiredAPIKey.providerName, requiredAPIKey.fieldName, currentTenant.ID)
		}
		if requiredAPIKey, knownProvider := configuration.Providers.dictationAPIKeyRequirement(currentTenant.Defaults.DictationProvider); knownProvider && strings.TrimSpace(requiredAPIKey.apiKey) == constants.EmptyString {
			return fmt.Errorf("%w: provider=%s field=%s tenant=%s endpoint=dictation", errProviderAPIKeyRequired, requiredAPIKey.providerName, requiredAPIKey.fieldName, currentTenant.ID)
		}
	}
	return nil
}

func (configuration providersConfiguration) textAPIKeyRequirement(rawProvider string) (providerAPIKeyRequirement, bool) {
	normalizedProvider := strings.ToLower(strings.TrimSpace(rawProvider))
	if normalizedProvider == constants.EmptyString {
		normalizedProvider = proxy.ProviderNameOpenAI
	}
	return configuration.apiKeyRequirement(normalizedProvider)
}

func (configuration providersConfiguration) dictationAPIKeyRequirement(rawProvider string) (providerAPIKeyRequirement, bool) {
	normalizedProvider := strings.ToLower(strings.TrimSpace(rawProvider))
	if normalizedProvider == constants.EmptyString {
		normalizedProvider = proxy.ProviderNameOpenAI
	}
	switch normalizedProvider {
	case proxy.ProviderNameOpenAI, proxy.ProviderNameSiliconFlow, proxy.ProviderNameZhipu, providerAliasGLM, proxy.ProviderNameGrok, providerAliasXAI:
		return configuration.apiKeyRequirement(normalizedProvider)
	default:
		return providerAPIKeyRequirement{}, false
	}
}

func (configuration providersConfiguration) apiKeyRequirement(normalizedProvider string) (providerAPIKeyRequirement, bool) {
	switch normalizedProvider {
	case proxy.ProviderNameOpenAI:
		return providerAPIKeyRequirement{providerName: proxy.ProviderNameOpenAI, fieldName: "providers.openai.api_key", apiKey: configuration.OpenAI.APIKey}, true
	case proxy.ProviderNameDeepSeek:
		return providerAPIKeyRequirement{providerName: proxy.ProviderNameDeepSeek, fieldName: "providers.deepseek.api_key", apiKey: configuration.DeepSeek.APIKey}, true
	case proxy.ProviderNameDashScope, providerAliasQwen:
		return providerAPIKeyRequirement{providerName: proxy.ProviderNameDashScope, fieldName: "providers.dashscope.api_key", apiKey: configuration.DashScope.APIKey}, true
	case proxy.ProviderNameMoonshot, providerAliasKimi:
		return providerAPIKeyRequirement{providerName: proxy.ProviderNameMoonshot, fieldName: "providers.moonshot.api_key", apiKey: configuration.Moonshot.APIKey}, true
	case proxy.ProviderNameSiliconFlow:
		return providerAPIKeyRequirement{providerName: proxy.ProviderNameSiliconFlow, fieldName: "providers.siliconflow.api_key", apiKey: configuration.SiliconFlow.APIKey}, true
	case proxy.ProviderNameZhipu, providerAliasGLM:
		return providerAPIKeyRequirement{providerName: proxy.ProviderNameZhipu, fieldName: "providers.zhipu.api_key", apiKey: configuration.Zhipu.APIKey}, true
	case proxy.ProviderNameGemini:
		return providerAPIKeyRequirement{providerName: proxy.ProviderNameGemini, fieldName: "providers.gemini.api_key", apiKey: configuration.Gemini.APIKey}, true
	case proxy.ProviderNameAnthropic, providerAliasClaude:
		return providerAPIKeyRequirement{providerName: proxy.ProviderNameAnthropic, fieldName: "providers.anthropic.api_key", apiKey: configuration.Anthropic.APIKey}, true
	case proxy.ProviderNameMeta:
		return providerAPIKeyRequirement{providerName: proxy.ProviderNameMeta, fieldName: "providers.meta.api_key", apiKey: configuration.Meta.APIKey}, true
	case proxy.ProviderNameGrok, providerAliasXAI:
		return providerAPIKeyRequirement{providerName: proxy.ProviderNameGrok, fieldName: "providers.grok.api_key", apiKey: configuration.Grok.APIKey}, true
	default:
		return providerAPIKeyRequirement{}, false
	}
}

func tenantConfigurations(rawTenants []tenantConfiguration) []proxy.TenantConfiguration {
	tenants := make([]proxy.TenantConfiguration, 0, len(rawTenants))
	for _, rawTenant := range rawTenants {
		tenants = append(tenants, proxy.TenantConfiguration{
			ID:     rawTenant.ID,
			Secret: rawTenant.Secret,
			Defaults: proxy.TenantDefaults{
				Provider:          rawTenant.Defaults.Provider,
				Model:             rawTenant.Defaults.Model,
				DictationProvider: rawTenant.Defaults.DictationProvider,
				DictationModel:    rawTenant.Defaults.DictationModel,
				SystemPrompt:      rawTenant.Defaults.SystemPrompt,
			},
		})
	}
	return tenants
}
