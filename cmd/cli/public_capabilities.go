package main

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/spf13/viper"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

var errPublicCapabilityConfig = errors.New("public_capability_config_failed")

type publicCapabilityAPIConfiguration struct {
	Catalog  proxy.PublicCapabilityCatalog
	Port     int
	LogLevel string
}

func loadPublicCapabilityAPIConfiguration(rawConfigPath string) (publicCapabilityAPIConfiguration, error) {
	configPath := normalizedConfigPath(rawConfigPath)
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
	providersReader := configReader.Sub("providers")
	if providersReader == nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s field=providers", errPublicCapabilityConfig, configPath)
	}
	catalogReader := configReader.Sub("catalog")
	if catalogReader == nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s field=catalog", errPublicCapabilityConfig, configPath)
	}
	var serverConfig serverConfiguration
	if unmarshalError := serverReader.UnmarshalExact(&serverConfig); unmarshalError != nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s field=server: %v", errPublicCapabilityConfig, configPath, unmarshalError)
	}
	if serverConfig.Port <= 0 {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s: server.port must be positive", errPublicCapabilityConfig, configPath)
	}
	var providersConfig providersConfiguration
	if unmarshalError := providersReader.UnmarshalExact(&providersConfig); unmarshalError != nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s field=providers: %v", errPublicCapabilityConfig, configPath, unmarshalError)
	}
	var catalogConfig modelCatalogConfiguration
	if unmarshalError := catalogReader.UnmarshalExact(&catalogConfig); unmarshalError != nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s field=catalog: %v", errPublicCapabilityConfig, configPath, unmarshalError)
	}
	maxRequestTimeoutSeconds, timeoutError := configuredPositiveInteger(serverConfig.MaxRequestTimeoutSeconds, proxy.DefaultMaxRequestTimeoutSeconds, "server.max_request_timeout_seconds")
	if timeoutError != nil {
		return publicCapabilityAPIConfiguration{}, fmt.Errorf("%w: path=%s: %v", errPublicCapabilityConfig, configPath, timeoutError)
	}
	capabilityCatalog, catalogError := proxy.NewPublicCapabilityCatalog(proxy.Configuration{
		ModelCatalog:             catalogConfig.proxyCatalog(),
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
