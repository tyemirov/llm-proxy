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

var (
	errConfigFileRead           = errors.New("config_file_read_failed")
	errConfigFileParse          = errors.New("config_file_parse_failed")
	errConfigEnvironmentRead    = errors.New("config_environment_read_failed")
	errConfigPlaceholderMissing = errors.New("config_placeholder_missing")
	errConfigInvalid            = errors.New("config_invalid")
	placeholderPattern          = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	readConfigBytes             = os.ReadFile
	readDotEnvFile              = gotenv.Read
	processEnvironment          = os.Environ
)

type fileConfiguration struct {
	Server    serverConfiguration    `mapstructure:"server"`
	Tenants   []tenantConfiguration  `mapstructure:"tenants"`
	Providers providersConfiguration `mapstructure:"providers"`
}

type serverConfiguration struct {
	Port                       int    `mapstructure:"port"`
	LogLevel                   string `mapstructure:"log_level"`
	Workers                    int    `mapstructure:"workers"`
	QueueSize                  int    `mapstructure:"queue_size"`
	RequestTimeoutSeconds      int    `mapstructure:"request_timeout_seconds"`
	UpstreamPollTimeoutSeconds int    `mapstructure:"upstream_poll_timeout_seconds"`
	MaxPromptBytes             int64  `mapstructure:"max_prompt_bytes"`
	MaxInputAudioBytes         int64  `mapstructure:"max_input_audio_bytes"`
}

type tenantConfiguration struct {
	ID       string               `mapstructure:"id"`
	Secret   string               `mapstructure:"secret"`
	Defaults tenantDefaultsConfig `mapstructure:"defaults"`
}

type tenantDefaultsConfig struct {
	Provider          string `mapstructure:"provider"`
	Model             string `mapstructure:"model"`
	DictationProvider string `mapstructure:"dictation_provider"`
	DictationModel    string `mapstructure:"dictation_model"`
	SystemPrompt      string `mapstructure:"system_prompt"`
}

type providersConfiguration struct {
	OpenAI      providerConfiguration    `mapstructure:"openai"`
	DeepSeek    providerConfiguration    `mapstructure:"deepseek"`
	DashScope   providerConfiguration    `mapstructure:"dashscope"`
	Moonshot    providerConfiguration    `mapstructure:"moonshot"`
	SiliconFlow siliconFlowConfiguration `mapstructure:"siliconflow"`
	Zhipu       providerConfiguration    `mapstructure:"zhipu"`
	Gemini      providerConfiguration    `mapstructure:"gemini"`
	Anthropic   providerConfiguration    `mapstructure:"anthropic"`
	Grok        providerConfiguration    `mapstructure:"grok"`
}

type providerConfiguration struct {
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
}

type siliconFlowConfiguration struct {
	APIKey            string `mapstructure:"api_key"`
	BaseURL           string `mapstructure:"base_url"`
	TranscriptionsURL string `mapstructure:"transcriptions_url"`
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
	expandedConfig := placeholderPattern.ReplaceAllStringFunc(configContent, func(placeholder string) string {
		placeholderMatches := placeholderPattern.FindStringSubmatch(placeholder)
		placeholderName := placeholderMatches[1]
		placeholderValue, foundValue := expansionEnvironment[placeholderName]
		if !foundValue {
			missingPlaceholders[placeholderName] = struct{}{}
			return placeholder
		}
		return placeholderValue
	})
	if len(missingPlaceholders) > 0 {
		missingNames := make([]string, 0, len(missingPlaceholders))
		for placeholderName := range missingPlaceholders {
			missingNames = append(missingNames, placeholderName)
		}
		sort.Strings(missingNames)
		return constants.EmptyString, fmt.Errorf("%w: names=%s", errConfigPlaceholderMissing, strings.Join(missingNames, ","))
	}
	return expandedConfig, nil
}

func (configuration fileConfiguration) toProxyConfiguration() (proxy.Configuration, error) {
	return proxy.NewConfiguration(proxy.Configuration{
		Tenants:                      tenantConfigurations(configuration.Tenants),
		OpenAIKey:                    configuration.Providers.OpenAI.APIKey,
		DeepSeekKey:                  configuration.Providers.DeepSeek.APIKey,
		DashScopeKey:                 configuration.Providers.DashScope.APIKey,
		MoonshotKey:                  configuration.Providers.Moonshot.APIKey,
		SiliconFlowKey:               configuration.Providers.SiliconFlow.APIKey,
		ZhipuKey:                     configuration.Providers.Zhipu.APIKey,
		GeminiKey:                    configuration.Providers.Gemini.APIKey,
		AnthropicKey:                 configuration.Providers.Anthropic.APIKey,
		GrokKey:                      configuration.Providers.Grok.APIKey,
		DeepSeekBaseURL:              configuration.Providers.DeepSeek.BaseURL,
		DashScopeBaseURL:             configuration.Providers.DashScope.BaseURL,
		MoonshotBaseURL:              configuration.Providers.Moonshot.BaseURL,
		SiliconFlowBaseURL:           configuration.Providers.SiliconFlow.BaseURL,
		SiliconFlowTranscriptionsURL: configuration.Providers.SiliconFlow.TranscriptionsURL,
		ZhipuBaseURL:                 configuration.Providers.Zhipu.BaseURL,
		GeminiBaseURL:                configuration.Providers.Gemini.BaseURL,
		AnthropicBaseURL:             configuration.Providers.Anthropic.BaseURL,
		GrokBaseURL:                  configuration.Providers.Grok.BaseURL,
		Port:                         configuration.Server.Port,
		LogLevel:                     configuration.Server.LogLevel,
		WorkerCount:                  configuration.Server.Workers,
		QueueSize:                    configuration.Server.QueueSize,
		RequestTimeoutSeconds:        configuration.Server.RequestTimeoutSeconds,
		UpstreamPollTimeoutSeconds:   configuration.Server.UpstreamPollTimeoutSeconds,
		MaxPromptBytes:               configuration.Server.MaxPromptBytes,
		MaxInputAudioBytes:           configuration.Server.MaxInputAudioBytes,
	})
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
