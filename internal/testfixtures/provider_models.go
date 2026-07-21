package testfixtures

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

type providerModelsFileConfiguration struct {
	Providers map[string]providerModelsProviderConfiguration `mapstructure:"providers"`
}

type providerModelsProviderConfiguration struct {
	Text      providerModelsEndpointConfiguration `mapstructure:"text"`
	Dictation providerModelsEndpointConfiguration `mapstructure:"dictation"`
}

type providerModelsEndpointConfiguration struct {
	DefaultModel    string                               `mapstructure:"default_model"`
	Models          []providerModelsModelConfiguration   `mapstructure:"models"`
	ReasoningEffort *providerModelsReasoningEffortConfig `mapstructure:"reasoning_effort"`
}

type providerModelsModelConfiguration struct {
	ID               string                               `mapstructure:"id"`
	RequestProfile   string                               `mapstructure:"request_profile"`
	WebSearch        bool                                 `mapstructure:"web_search"`
	OutputTokenLimit int                                  `mapstructure:"output_token_limit"`
	ReasoningEffort  *providerModelsReasoningEffortConfig `mapstructure:"reasoning_effort"`
}

type providerModelsReasoningEffortConfig struct {
	Adapter string   `mapstructure:"adapter"`
	Efforts []string `mapstructure:"efforts"`
}

// ProviderModelCatalogs loads the repository config model catalogs for tests that build proxy.Configuration directly.
func ProviderModelCatalogs(testingInstance testing.TB) proxy.ProviderModelCatalogs {
	testingInstance.Helper()
	_, currentFile, _, callerOK := runtime.Caller(0)
	if !callerOK {
		testingInstance.Fatal("locate test fixture file")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	configPath := filepath.Join(repositoryRoot, "configs", "config.yml")
	configReader := viper.New()
	configReader.SetConfigFile(configPath)
	if readConfigError := configReader.ReadInConfig(); readConfigError != nil {
		testingInstance.Fatalf("read provider model config: %v", readConfigError)
	}
	var parsedConfiguration providerModelsFileConfiguration
	if unmarshalError := configReader.Unmarshal(&parsedConfiguration); unmarshalError != nil {
		testingInstance.Fatalf("parse provider model config: %v", unmarshalError)
	}
	catalogs := proxy.ProviderModelCatalogs{}
	for providerName, providerConfiguration := range parsedConfiguration.Providers {
		catalogs[providerName] = proxy.ProviderModelCatalog{
			Text:      providerConfiguration.Text.proxyCatalog(),
			Dictation: providerConfiguration.Dictation.proxyCatalog(),
		}
	}
	return catalogs
}

// WithProviderModelCatalogs returns a configuration with explicit provider model catalogs from configs/config.yml.
func WithProviderModelCatalogs(testingInstance testing.TB, configuration proxy.Configuration) proxy.Configuration {
	testingInstance.Helper()
	configuration.ProviderModels = ProviderModelCatalogs(testingInstance)
	return configuration
}

func (configuration providerModelsEndpointConfiguration) proxyCatalog() proxy.ModelEndpointCatalog {
	models := make([]proxy.ModelConfiguration, 0, len(configuration.Models))
	for _, modelConfiguration := range configuration.Models {
		models = append(models, proxy.ModelConfiguration{
			ID:               modelConfiguration.ID,
			RequestProfile:   modelConfiguration.RequestProfile,
			WebSearch:        modelConfiguration.WebSearch,
			OutputTokenLimit: modelConfiguration.OutputTokenLimit,
			ReasoningEffort:  providerModelsReasoningEffortCapability(modelConfiguration.ReasoningEffort),
		})
	}
	return proxy.ModelEndpointCatalog{
		DefaultModel:    configuration.DefaultModel,
		Models:          models,
		ReasoningEffort: providerModelsReasoningEffortCapability(configuration.ReasoningEffort),
	}
}

func providerModelsReasoningEffortCapability(configuration *providerModelsReasoningEffortConfig) *proxy.ReasoningEffortCapability {
	if configuration == nil {
		return nil
	}
	return &proxy.ReasoningEffortCapability{
		Adapter: configuration.Adapter,
		Efforts: append([]string(nil), configuration.Efforts...),
	}
}
