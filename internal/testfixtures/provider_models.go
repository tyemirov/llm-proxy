package testfixtures

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

type modelCatalogFileConfiguration struct {
	Catalog modelCatalogConfiguration `mapstructure:"catalog"`
}

type modelCatalogConfiguration struct {
	Providers  []catalogProviderConfiguration  `mapstructure:"providers"`
	Publishers []modelPublisherConfiguration   `mapstructure:"publishers"`
	Families   []modelFamilyConfiguration      `mapstructure:"families"`
	Models     []exactModelConfiguration       `mapstructure:"models"`
	Offerings  []providerOfferingConfiguration `mapstructure:"offerings"`
}

type catalogProviderConfiguration struct {
	ID    string `mapstructure:"id"`
	Label string `mapstructure:"label"`
}

type modelPublisherConfiguration struct {
	ID    string `mapstructure:"id"`
	Label string `mapstructure:"label"`
}

type modelFamilyConfiguration struct {
	ID        string `mapstructure:"id"`
	Publisher string `mapstructure:"publisher"`
	Label     string `mapstructure:"label"`
}

type exactModelConfiguration struct {
	ID          string   `mapstructure:"id"`
	Publisher   string   `mapstructure:"publisher"`
	Family      string   `mapstructure:"family"`
	Version     string   `mapstructure:"version"`
	Operations  []string `mapstructure:"operations"`
	MediaInputs []string `mapstructure:"media_inputs"`
}

type providerOfferingConfiguration struct {
	Provider           string                           `mapstructure:"provider"`
	Model              string                           `mapstructure:"model"`
	ProviderModel      string                           `mapstructure:"provider_model"`
	Operations         []string                         `mapstructure:"operations"`
	DefaultOperations  []string                         `mapstructure:"default_operations"`
	WireContract       string                           `mapstructure:"wire_contract"`
	ExecutionLifecycle string                           `mapstructure:"execution_lifecycle"`
	RequestProfile     string                           `mapstructure:"request_profile"`
	WebSearch          bool                             `mapstructure:"web_search"`
	OutputTokenLimit   int                              `mapstructure:"output_token_limit"`
	ReasoningEffort    *reasoningEffortCapabilityConfig `mapstructure:"reasoning_effort"`
	MediaInputs        []string                         `mapstructure:"media_inputs"`
}

type reasoningEffortCapabilityConfig struct {
	Adapter string   `mapstructure:"adapter"`
	Efforts []string `mapstructure:"efforts"`
}

// ModelCatalog loads the repository model catalog for tests that build proxy.Configuration directly.
func ModelCatalog(testingInstance testing.TB) proxy.ModelCatalog {
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
		testingInstance.Fatalf("read model catalog config: %v", readConfigError)
	}
	var parsedConfiguration modelCatalogFileConfiguration
	if unmarshalError := configReader.Unmarshal(&parsedConfiguration); unmarshalError != nil {
		testingInstance.Fatalf("parse model catalog config: %v", unmarshalError)
	}
	return parsedConfiguration.Catalog.proxyCatalog()
}

// WithModelCatalog returns a configuration with the explicit model catalog from configs/config.yml.
func WithModelCatalog(testingInstance testing.TB, configuration proxy.Configuration) proxy.Configuration {
	testingInstance.Helper()
	configuration.ModelCatalog = ModelCatalog(testingInstance)
	return configuration
}

func (configuration modelCatalogConfiguration) proxyCatalog() proxy.ModelCatalog {
	providers := make([]proxy.CatalogProvider, 0, len(configuration.Providers))
	for _, provider := range configuration.Providers {
		providers = append(providers, proxy.CatalogProvider{ID: provider.ID, Label: provider.Label})
	}
	publishers := make([]proxy.ModelPublisher, 0, len(configuration.Publishers))
	for _, publisher := range configuration.Publishers {
		publishers = append(publishers, proxy.ModelPublisher{ID: publisher.ID, Label: publisher.Label})
	}
	families := make([]proxy.ModelFamily, 0, len(configuration.Families))
	for _, family := range configuration.Families {
		families = append(families, proxy.ModelFamily{ID: family.ID, Publisher: family.Publisher, Label: family.Label})
	}
	models := make([]proxy.ExactModel, 0, len(configuration.Models))
	for _, model := range configuration.Models {
		models = append(models, proxy.ExactModel{
			ID: model.ID, Publisher: model.Publisher, Family: model.Family, Version: model.Version,
			Operations: append([]string(nil), model.Operations...), MediaInputs: append([]string(nil), model.MediaInputs...),
		})
	}
	offerings := make([]proxy.ProviderOffering, 0, len(configuration.Offerings))
	for _, offering := range configuration.Offerings {
		offerings = append(offerings, proxy.ProviderOffering{
			Provider: offering.Provider, Model: offering.Model, ProviderModel: offering.ProviderModel,
			Operations: append([]string(nil), offering.Operations...), DefaultOperations: append([]string(nil), offering.DefaultOperations...),
			WireContract: offering.WireContract, ExecutionLifecycle: offering.ExecutionLifecycle, RequestProfile: offering.RequestProfile,
			WebSearch: offering.WebSearch, OutputTokenLimit: offering.OutputTokenLimit,
			ReasoningEffort: reasoningEffortCapability(offering.ReasoningEffort), MediaInputs: append([]string(nil), offering.MediaInputs...),
		})
	}
	return proxy.ModelCatalog{Providers: providers, Publishers: publishers, Families: families, Models: models, Offerings: offerings}
}

func reasoningEffortCapability(configuration *reasoningEffortCapabilityConfig) *proxy.ReasoningEffortCapability {
	if configuration == nil {
		return nil
	}
	return &proxy.ReasoningEffortCapability{
		Adapter: configuration.Adapter,
		Efforts: append([]string(nil), configuration.Efforts...),
	}
}
