package main

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/spf13/viper"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

var errPublicCapabilityConfig = errors.New("public_capability_config_failed")

var newPublicCapabilityCatalog = proxy.NewPublicCapabilityCatalog

type publicCapabilityAPIConfiguration struct {
	Catalog  proxy.PublicCapabilityCatalog
	Port     int
	LogLevel string
}

func loadPublicCapabilityAPIConfiguration(rawConfigPath string) (publicCapabilityAPIConfiguration, error) {
	configPath := normalizedConfigPath(rawConfigPath)
	providerCatalog, catalogLoadError := loadProviderCatalog(configPath)
	if catalogLoadError != nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: %v", errPublicCapabilityConfig, catalogLoadError)
	}
	configBytes, readError := readConfigBytes(configPath)
	if readError != nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s: %v", errPublicCapabilityConfig, configPath, readError)
	}
	configReader := viper.New()
	configReader.SetConfigType(configFileType)
	if readConfigError := configReader.ReadConfig(bytes.NewReader(configBytes)); readConfigError != nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s: %v", errPublicCapabilityConfig, configPath, readConfigError)
	}
	serverReader := configReader.Sub("server")
	if serverReader == nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s field=server", errPublicCapabilityConfig, configPath)
	}
	var serverConfig serverConfiguration
	if unmarshalError := serverReader.UnmarshalExact(&serverConfig); unmarshalError != nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s field=server: %v", errPublicCapabilityConfig, configPath, unmarshalError)
	}
	if serverConfig.Port <= 0 {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s: server.port must be positive", errPublicCapabilityConfig, configPath)
	}
	maxRequestTimeoutSeconds, timeoutError := configuredPositiveInteger(serverConfig.MaxRequestTimeoutSeconds, proxy.DefaultMaxRequestTimeoutSeconds, "server.max_request_timeout_seconds")
	if timeoutError != nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s: %v", errPublicCapabilityConfig, configPath, timeoutError)
	}
	capabilityCatalog, catalogError := newPublicCapabilityCatalog(proxy.Configuration{
		ProviderCatalog:          providerCatalog,
		MaxPromptBytes:           serverConfig.MaxPromptBytes,
		MaxInputAudioBytes:       serverConfig.MaxInputAudioBytes,
		MaxRequestTimeoutSeconds: maxRequestTimeoutSeconds,
	})
	if catalogError != nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s: %v", errPublicCapabilityConfig, configPath, catalogError)
	}
	return publicCapabilityAPIConfiguration{
		Catalog:  capabilityCatalog,
		Port:     serverConfig.Port,
		LogLevel: serverConfig.LogLevel,
	}, nil
}
