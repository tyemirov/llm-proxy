package testfixtures

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

type modelCatalogFileConfiguration struct {
	Catalog proxy.ModelCatalog `mapstructure:"catalog"`
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
	return parsedConfiguration.Catalog
}

// WithModelCatalog returns a configuration with the explicit model catalog from configs/config.yml.
func WithModelCatalog(testingInstance testing.TB, configuration proxy.Configuration) proxy.Configuration {
	testingInstance.Helper()
	if len(configuration.ModelCatalog.Offerings) == 0 {
		configuration.ModelCatalog = ModelCatalog(testingInstance)
	}
	return configuration
}
