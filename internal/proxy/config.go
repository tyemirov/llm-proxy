package proxy

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"gorm.io/gorm"
)

const (
	// DefaultPort is the TCP port used by the HTTP server when no explicit port is provided.
	DefaultPort = 8080
	// DefaultWorkers is the maximum number of concurrent upstream HTTP operations.
	DefaultWorkers = 4
	// DefaultQueueSize is the number of upstream HTTP operations that may wait for a worker.
	DefaultQueueSize = 100
	// DefaultModel is the model identifier used when the client does not supply one.
	DefaultModel = ModelNameGPT41
	// DefaultProvider is the provider identifier used when the client does not supply one.
	DefaultProvider = ProviderNameOpenAI
	// DefaultDictationProvider is the provider used when /dictate does not supply one.
	DefaultDictationProvider = ProviderNameOpenAI

	// DefaultRequestTimeoutSeconds is the overall app-side request timeout.
	DefaultRequestTimeoutSeconds = 360
	// DefaultMaxPromptBytes limits JSON LLM request bodies accepted by POST /.
	DefaultMaxPromptBytes      = 4 * 1024 * 1024
	DefaultDictationModel      = "gpt-4o-mini-transcribe"
	DefaultMaxInputAudioBytes  = 25 * 1024 * 1024
	DefaultManagementJWTIssuer = "tauth"
	// ManagementDatabaseDialectPostgres selects the GORM Postgres dialector.
	ManagementDatabaseDialectPostgres = "postgres"
	// ManagementDatabaseDialectSQLite selects the GORM SQLite dialector.
	ManagementDatabaseDialectSQLite = "sqlite"
)

// Configuration holds runtime settings.
type Configuration struct {
	Tenants                      []TenantConfiguration
	Management                   ManagementConfiguration
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

// ManagementConfiguration holds authenticated browser UI and self-service tenant settings.
type ManagementConfiguration struct {
	Enabled             bool
	PublicOrigin        string
	UIDescription       string
	UIOrigins           []string
	TAuthURL            string
	TAuthTenantID       string
	GoogleClientID      string
	LoginPath           string
	LogoutPath          string
	NoncePath           string
	JWTSigningKey       string
	JWTIssuer           string
	SessionCookieName   string
	DatabaseDialect     string
	DatabaseDSN         string
	ManagementAPIOrigin string
	ProxyOrigin         string
	DatabaseDialector   gorm.Dialector
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
	tenants, tenantError := newTenantRegistry(configuration.Tenants, configuration.Management.Enabled)
	if tenantError != nil {
		return tenantRegistry{}, tenantError
	}
	if managementValidationError := validateManagementConfiguration(configuration.Management); managementValidationError != nil {
		return tenantRegistry{}, managementValidationError
	}
	if modelCatalogError := validateProviderModelCatalogs(configuration.ProviderModels); modelCatalogError != nil {
		return tenantRegistry{}, modelCatalogError
	}
	providers := newProviderRegistry(configuration)
	if !configuration.Management.Enabled {
		validator := newModelValidator(providers)
		for _, currentTenant := range tenants.tenants {
			if _, _, verificationError := validator.ResolveText(constants.EmptyString, constants.EmptyString, currentTenant.defaults.provider, currentTenant.defaults.model, false); verificationError != nil {
				return tenantRegistry{}, fmt.Errorf("%w: tenant=%s", verificationError, currentTenant.identifier.string())
			}
			if _, _, verificationError := validator.ResolveDictation(constants.EmptyString, constants.EmptyString, currentTenant.defaults.dictationProvider, currentTenant.defaults.dictationModel); verificationError != nil {
				return tenantRegistry{}, fmt.Errorf("%w: tenant=%s", verificationError, currentTenant.identifier.string())
			}
		}
	}
	return tenants, nil
}

// ErrUpstreamIncomplete indicates that the upstream provider returned an incomplete response before the request deadline.
var ErrUpstreamIncomplete = errors.New(errorUpstreamIncomplete)

var errQueueFull = errors.New(errorQueueFull)

// ApplyTunables ensures tunable configuration values have sensible defaults.
func (configuration *Configuration) ApplyTunables() {
	configuration.Management.ApplyTunables()
	configuration.OpenAIKey = strings.TrimSpace(configuration.OpenAIKey)
	configuration.DeepSeekKey = strings.TrimSpace(configuration.DeepSeekKey)
	configuration.DashScopeKey = strings.TrimSpace(configuration.DashScopeKey)
	configuration.MoonshotKey = strings.TrimSpace(configuration.MoonshotKey)
	configuration.SiliconFlowKey = strings.TrimSpace(configuration.SiliconFlowKey)
	configuration.ZhipuKey = strings.TrimSpace(configuration.ZhipuKey)
	configuration.GeminiKey = strings.TrimSpace(configuration.GeminiKey)
	configuration.AnthropicKey = strings.TrimSpace(configuration.AnthropicKey)
	configuration.GrokKey = strings.TrimSpace(configuration.GrokKey)
	if configuration.WorkerCount <= 0 {
		configuration.WorkerCount = DefaultWorkers
	}
	if configuration.QueueSize <= 0 {
		configuration.QueueSize = DefaultQueueSize
	}
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

// ApplyTunables normalizes optional management settings.
func (configuration *ManagementConfiguration) ApplyTunables() {
	configuration.PublicOrigin = strings.TrimSpace(configuration.PublicOrigin)
	configuration.UIDescription = strings.TrimSpace(configuration.UIDescription)
	for originIndex, originValue := range configuration.UIOrigins {
		configuration.UIOrigins[originIndex] = strings.TrimSpace(originValue)
	}
	configuration.TAuthURL = strings.TrimSpace(configuration.TAuthURL)
	configuration.TAuthTenantID = strings.TrimSpace(configuration.TAuthTenantID)
	configuration.GoogleClientID = strings.TrimSpace(configuration.GoogleClientID)
	configuration.LoginPath = strings.TrimSpace(configuration.LoginPath)
	configuration.LogoutPath = strings.TrimSpace(configuration.LogoutPath)
	configuration.NoncePath = strings.TrimSpace(configuration.NoncePath)
	configuration.JWTSigningKey = strings.TrimSpace(configuration.JWTSigningKey)
	configuration.JWTIssuer = strings.TrimSpace(configuration.JWTIssuer)
	if configuration.JWTIssuer == constants.EmptyString {
		configuration.JWTIssuer = DefaultManagementJWTIssuer
	}
	configuration.SessionCookieName = strings.TrimSpace(configuration.SessionCookieName)
	configuration.DatabaseDialect = strings.ToLower(strings.TrimSpace(configuration.DatabaseDialect))
	configuration.DatabaseDSN = strings.TrimSpace(configuration.DatabaseDSN)
	configuration.ManagementAPIOrigin = strings.TrimSpace(configuration.ManagementAPIOrigin)
	configuration.ProxyOrigin = strings.TrimSpace(configuration.ProxyOrigin)
}

func validateManagementConfiguration(configuration ManagementConfiguration) error {
	if !configuration.Enabled {
		return nil
	}
	requiredFields := []struct {
		fieldName  string
		fieldValue string
	}{
		{fieldName: "management.public_origin", fieldValue: configuration.PublicOrigin},
		{fieldName: "management.ui_description", fieldValue: configuration.UIDescription},
		{fieldName: "management.tauth_url", fieldValue: configuration.TAuthURL},
		{fieldName: "management.tauth_tenant_id", fieldValue: configuration.TAuthTenantID},
		{fieldName: "management.google_client_id", fieldValue: configuration.GoogleClientID},
		{fieldName: "management.login_path", fieldValue: configuration.LoginPath},
		{fieldName: "management.logout_path", fieldValue: configuration.LogoutPath},
		{fieldName: "management.nonce_path", fieldValue: configuration.NoncePath},
		{fieldName: "management.jwt_signing_key", fieldValue: configuration.JWTSigningKey},
		{fieldName: "management.jwt_issuer", fieldValue: configuration.JWTIssuer},
		{fieldName: "management.session_cookie_name", fieldValue: configuration.SessionCookieName},
		{fieldName: "management.database_dialect", fieldValue: configuration.DatabaseDialect},
		{fieldName: "management.database_dsn", fieldValue: configuration.DatabaseDSN},
		{fieldName: "management.management_api_origin", fieldValue: configuration.ManagementAPIOrigin},
		{fieldName: "management.proxy_origin", fieldValue: configuration.ProxyOrigin},
	}
	for _, requiredField := range requiredFields {
		if strings.TrimSpace(requiredField.fieldValue) == constants.EmptyString {
			return fmt.Errorf("%w: field=%s", ErrInvalidManagementConfiguration, requiredField.fieldName)
		}
	}
	if len(configuration.UIOrigins) == 0 {
		return fmt.Errorf("%w: field=management.ui_origins", ErrInvalidManagementConfiguration)
	}
	for _, originValue := range configuration.UIOrigins {
		if strings.TrimSpace(originValue) == constants.EmptyString {
			return fmt.Errorf("%w: field=management.ui_origins", ErrInvalidManagementConfiguration)
		}
	}
	if !supportedManagementDatabaseDialect(configuration.DatabaseDialect) {
		return fmt.Errorf("%w: field=management.database_dialect value=%s", ErrInvalidManagementConfiguration, configuration.DatabaseDialect)
	}
	return nil
}

func supportedManagementDatabaseDialect(databaseDialect string) bool {
	supportedDialects := map[string]struct{}{
		ManagementDatabaseDialectPostgres: {},
		ManagementDatabaseDialectSQLite:   {},
	}
	_, supportedDialect := supportedDialects[databaseDialect]
	return supportedDialect
}
