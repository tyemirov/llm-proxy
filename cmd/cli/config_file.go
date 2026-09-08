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
	errProviderCatalogRead      = errors.New("provider_catalog_read_failed")
	errProviderCatalogInvalid   = errors.New("provider_catalog_invalid")
	errConfigPlaceholderMissing = errors.New("config_placeholder_missing")
	errConfigInvalid            = errors.New("config_invalid")
	placeholderPattern          = regexp.MustCompile(`\$\{([^}]+)\}`)
	placeholderNamePattern      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	readConfigBytes             = os.ReadFile
	readProviderCatalogBytes    = os.ReadFile
	readDotEnvFile              = gotenv.Read
	processEnvironment          = os.Environ
)

type fileConfiguration struct {
	Server     serverConfiguration     `mapstructure:"server"`
	Management managementConfiguration `mapstructure:"management"`
}

type serverConfiguration struct {
	Port                     int                              `mapstructure:"port"`
	LogLevel                 string                           `mapstructure:"log_level"`
	Workers                  int                              `mapstructure:"workers"`
	QueueSize                int                              `mapstructure:"queue_size"`
	RequestTimeoutSeconds    *int                             `mapstructure:"request_timeout_seconds"`
	MaxRequestTimeoutSeconds *int                             `mapstructure:"max_request_timeout_seconds"`
	MaxPromptBytes           int64                            `mapstructure:"max_prompt_bytes"`
	MaxAssetBytes            int64                            `mapstructure:"max_asset_bytes"`
	AssetRetentionSeconds    int                              `mapstructure:"asset_retention_seconds"`
	AssetStorePath           string                           `mapstructure:"asset_store_path"`
	MaxInputAudioBytes       int64                            `mapstructure:"max_input_audio_bytes"`
	UpstreamRateLimits       []upstreamRateLimitConfiguration `mapstructure:"upstream_rate_limits"`
}

type upstreamRateLimitConfiguration struct {
	Origin      string `mapstructure:"origin"`
	MaxRequests int    `mapstructure:"max_requests"`
	Interval    string `mapstructure:"interval"`
}

type managementConfiguration struct {
	PublicOrigin             string   `mapstructure:"public_origin"`
	UIDescription            string   `mapstructure:"ui_description"`
	UIOrigins                []string `mapstructure:"ui_origins"`
	AdminEmails              []string `mapstructure:"admin_emails"`
	TAuthURL                 string   `mapstructure:"tauth_url"`
	TAuthTenantID            string   `mapstructure:"tauth_tenant_id"`
	GoogleClientID           string   `mapstructure:"google_client_id"`
	LoginPath                string   `mapstructure:"login_path"`
	LogoutPath               string   `mapstructure:"logout_path"`
	NoncePath                string   `mapstructure:"nonce_path"`
	SessionPath              string   `mapstructure:"session_path"`
	JWTSigningKey            string   `mapstructure:"jwt_signing_key"`
	JWTIssuer                string   `mapstructure:"jwt_issuer"`
	SessionCookieName        string   `mapstructure:"session_cookie_name"`
	DatabasePath             string   `mapstructure:"database_path"`
	UsageQueueSize           *int     `mapstructure:"usage_queue_size"`
	ProviderKeyEncryptionKey string   `mapstructure:"provider_key_encryption_key"`
	ManagementAPIOrigin      string   `mapstructure:"management_api_origin"`
	ProxyOrigin              string   `mapstructure:"proxy_origin"`
}

func loadRuntimeConfiguration(rawConfigPath string) (proxy.Configuration, error) {
	configPath := normalizedConfigPath(rawConfigPath)
	providerCatalog, catalogError := loadProviderCatalog(configPath)
	if catalogError != nil {
		return proxy.Configuration{}, catalogError
	}
	configBytes, readError := readConfigBytes(configPath)
	if readError != nil {
		return proxy.Configuration{}, fmt.Errorf("%w: path=%s: %v", errConfigFileRead, configPath, readError)
	}
	expansionEnvironment, environmentError := configurationExpansionEnvironment(configPath)
	if environmentError != nil {
		return proxy.Configuration{}, environmentError
	}
	providerConnectionValues, bindingError := providerCatalog.ResolveEnvironmentBindings(expansionEnvironment)
	if bindingError != nil {
		return proxy.Configuration{}, fmt.Errorf("%w: path=%s: %v", errConfigInvalid, configPath, bindingError)
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
	if integerValidationError := validateExplicitPositiveIntegerConfiguration(configReader); integerValidationError != nil {
		return proxy.Configuration{}, fmt.Errorf("%w: path=%s: %v", errConfigInvalid, configPath, integerValidationError)
	}

	var parsedConfiguration fileConfiguration
	if unmarshalError := configReader.UnmarshalExact(&parsedConfiguration); unmarshalError != nil {
		return proxy.Configuration{}, fmt.Errorf("%w: path=%s: %v", errConfigFileParse, configPath, unmarshalError)
	}
	runtimeConfig, configError := parsedConfiguration.toProxyConfiguration(providerCatalog, providerConnectionValues)
	if configError != nil {
		return proxy.Configuration{}, fmt.Errorf("%w: path=%s: %v", errConfigInvalid, configPath, configError)
	}
	return runtimeConfig, nil
}

func loadProviderCatalog(configPath string) (*proxy.ProviderCatalog, error) {
	providerCatalogPath := filepath.Join(filepath.Dir(configPath), "providers.yml")
	document, readError := readProviderCatalogBytes(providerCatalogPath)
	if readError != nil {
		return nil, fmt.Errorf("%w: path=%s: %v", errProviderCatalogRead, providerCatalogPath, readError)
	}
	catalog, parseError := proxy.ParseProviderCatalog(document)
	if parseError != nil {
		return nil, fmt.Errorf("%w: path=%s: %v", errProviderCatalogInvalid, providerCatalogPath, parseError)
	}
	return catalog, nil
}

func validateExplicitPositiveIntegerConfiguration(configReader *viper.Viper) error {
	positiveIntegerFields := map[string]struct{}{
		"server.request_timeout_seconds":     {},
		"server.max_request_timeout_seconds": {},
		"management.usage_queue_size":        {},
	}
	for _, configuredField := range configReader.AllKeys() {
		if _, isPositiveIntegerField := positiveIntegerFields[configuredField]; isPositiveIntegerField && configReader.Get(configuredField) == nil {
			return fmt.Errorf("invalid configuration: %s must be positive", configuredField)
		}
	}
	return nil
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

func (configuration fileConfiguration) toProxyConfiguration(providerCatalog *proxy.ProviderCatalog, providerConnectionValues map[string]map[string]string) (proxy.Configuration, error) {
	requestTimeoutSeconds, timeoutError := configuredPositiveInteger(configuration.Server.RequestTimeoutSeconds, proxy.DefaultRequestTimeoutSeconds, "server.request_timeout_seconds")
	if timeoutError != nil {
		return proxy.Configuration{}, timeoutError
	}
	maxRequestTimeoutSeconds, maxTimeoutError := configuredPositiveInteger(configuration.Server.MaxRequestTimeoutSeconds, proxy.DefaultMaxRequestTimeoutSeconds, "server.max_request_timeout_seconds")
	if maxTimeoutError != nil {
		return proxy.Configuration{}, maxTimeoutError
	}
	usageQueueSize, usageQueueError := configuredPositiveInteger(configuration.Management.UsageQueueSize, proxy.DefaultManagementUsageQueueSize, "management.usage_queue_size")
	if usageQueueError != nil {
		return proxy.Configuration{}, usageQueueError
	}
	return proxy.NewConfiguration(proxy.Configuration{
		Management:               managementProxyConfiguration(configuration.Management, usageQueueSize),
		ProviderCatalog:          providerCatalog,
		ProviderConnectionValues: providerConnectionValues,
		Port:                     configuration.Server.Port,
		LogLevel:                 configuration.Server.LogLevel,
		WorkerCount:              configuration.Server.Workers,
		QueueSize:                configuration.Server.QueueSize,
		RequestTimeoutSeconds:    requestTimeoutSeconds,
		MaxRequestTimeoutSeconds: maxRequestTimeoutSeconds,
		MaxPromptBytes:           configuration.Server.MaxPromptBytes,
		MaxAssetBytes:            configuration.Server.MaxAssetBytes,
		AssetRetentionSeconds:    configuration.Server.AssetRetentionSeconds,
		AssetStorePath:           configuration.Server.AssetStorePath,
		MaxInputAudioBytes:       configuration.Server.MaxInputAudioBytes,
		UpstreamRateLimits:       proxyUpstreamRateLimitConfigurations(configuration.Server.UpstreamRateLimits),
	})
}

func configuredPositiveInteger(configuredValue *int, defaultValue int, fieldName string) (int, error) {
	if configuredValue == nil {
		return defaultValue, nil
	}
	if *configuredValue <= 0 {
		return 0, fmt.Errorf("invalid configuration: %s must be positive", fieldName)
	}
	return *configuredValue, nil
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

func managementProxyConfiguration(configuration managementConfiguration, usageQueueSize int) proxy.ManagementConfiguration {
	return proxy.ManagementConfiguration{
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
		SessionPath:              configuration.SessionPath,
		JWTSigningKey:            configuration.JWTSigningKey,
		JWTIssuer:                configuration.JWTIssuer,
		SessionCookieName:        configuration.SessionCookieName,
		DatabasePath:             configuration.DatabasePath,
		UsageQueueSize:           usageQueueSize,
		ProviderKeyEncryptionKey: configuration.ProviderKeyEncryptionKey,
		ManagementAPIOrigin:      configuration.ManagementAPIOrigin,
		ProxyOrigin:              configuration.ProxyOrigin,
	}
}
