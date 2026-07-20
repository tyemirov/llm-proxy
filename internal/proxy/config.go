package proxy

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
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
	managedProviderKeyBytes         = 32
	tenantValidationErrorFormat     = "%w: tenant=%s"
)

// Configuration holds runtime settings.
type Configuration struct {
	Tenants                      []TenantConfiguration
	Management                   ManagementConfiguration
	OpenAIKey                    string
	DeepSeekKey                  string
	DashScopeKey                 string
	QwenCloudKey                 string
	MoonshotKey                  string
	MiniMaxKey                   string
	SiliconFlowKey               string
	ZhipuKey                     string
	GeminiKey                    string
	AnthropicKey                 string
	MetaKey                      string
	GrokKey                      string
	OpenAIBaseURL                string
	OpenAITranscriptionsURL      string
	DeepSeekBaseURL              string
	DashScopeBaseURL             string
	QwenCloudBaseURL             string
	MoonshotBaseURL              string
	MiniMaxBaseURL               string
	SiliconFlowBaseURL           string
	SiliconFlowTranscriptionsURL string
	ZhipuBaseURL                 string
	ZhipuTranscriptionsURL       string
	GeminiBaseURL                string
	AnthropicBaseURL             string
	MetaBaseURL                  string
	GrokBaseURL                  string
	GrokTranscriptionsURL        string
	Port                         int
	LogLevel                     string
	WorkerCount                  int
	QueueSize                    int
	RequestTimeoutSeconds        int
	MaxPromptBytes               int64
	MaxInputAudioBytes           int64
	UpstreamRateLimits           []UpstreamRateLimitConfiguration
	Endpoints                    *Endpoints
	ProviderModels               ProviderModelCatalogs
	upstreamRateLimits           upstreamRateLimits
	tenants                      tenantRegistry
	managementSessionValidator   *managementSessionValidator
	validated                    bool
}

// ManagementConfiguration holds authenticated browser UI and self-service tenant settings.
type ManagementConfiguration struct {
	Enabled                  bool
	PublicOrigin             string
	UIDescription            string
	UIOrigins                []string
	AdminEmails              []string
	TAuthURL                 string
	TAuthTenantID            string
	GoogleClientID           string
	LoginPath                string
	LogoutPath               string
	NoncePath                string
	JWTSigningKey            string
	JWTIssuer                string
	SessionCookieName        string
	DatabaseDialect          string
	DatabaseDSN              string
	ProviderKeyEncryptionKey string
	ManagementAPIOrigin      string
	ProxyOrigin              string
	LegacyTokenMigration     LegacyTokenMigrationConfiguration
	DatabaseDialector        gorm.Dialector
}

// LegacyTokenMigrationConfiguration identifies the one legacy tenant and verified account email allowed to claim it.
type LegacyTokenMigrationConfiguration struct {
	TenantID   string
	OwnerEmail string
}

// NewConfiguration returns a normalized runtime configuration after validating startup invariants.
func NewConfiguration(configuration Configuration) (Configuration, error) {
	configuration.ApplyTunables()
	upstreamRateLimits, rateLimitError := newUpstreamRateLimits(configuration.UpstreamRateLimits)
	if rateLimitError != nil {
		return Configuration{}, rateLimitError
	}
	tenants, validationError := validateConfig(configuration)
	if validationError != nil {
		return Configuration{}, validationError
	}
	var sessionValidator *managementSessionValidator
	if configuration.Management.Enabled {
		var sessionValidationError error
		sessionValidator, sessionValidationError = newManagementSessionValidator(configuration.Management)
		if sessionValidationError != nil {
			return Configuration{}, sessionValidationError
		}
	}
	configuration.upstreamRateLimits = upstreamRateLimits
	configuration.tenants = tenants
	configuration.managementSessionValidator = sessionValidator
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
	if managementValidationError := validateManagementConfiguration(configuration.Management); managementValidationError != nil {
		return tenantRegistry{}, managementValidationError
	}
	if configuration.Management.Enabled {
		if len(configuration.Tenants) != 0 {
			return tenantRegistry{}, fmt.Errorf("%w: field=tenants unsupported_in_management_mode", ErrInvalidManagementConfiguration)
		}
		if len(configuredProviderAPIKeys(configuration)) != 0 {
			return tenantRegistry{}, fmt.Errorf("%w: field=providers.api_key unsupported_in_management_mode", ErrInvalidManagementConfiguration)
		}
	}
	tenants, tenantError := newTenantRegistry(configuration.Tenants, configuration.Management.Enabled)
	if tenantError != nil {
		return tenantRegistry{}, tenantError
	}
	if modelCatalogError := validateProviderModelCatalogs(configuration.ProviderModels); modelCatalogError != nil {
		return tenantRegistry{}, modelCatalogError
	}
	providers := newProviderRegistry(configuration)
	validator := newModelValidator(providers)
	for _, currentTenant := range tenants.tenants {
		if validationError := validateTenantDefaultRuntime(validator, currentTenant); validationError != nil {
			return tenantRegistry{}, validationError
		}
	}
	return tenants, nil
}

func validateTenantDefaultRuntime(validator *modelValidator, currentTenant tenant) error {
	if _, _, verificationError := validator.ResolveText(constants.EmptyString, constants.EmptyString, currentTenant.defaults.provider, currentTenant.defaults.model, false); verificationError != nil {
		return fmt.Errorf(tenantValidationErrorFormat, verificationError, currentTenant.identifier.string())
	}
	if _, _, verificationError := validator.ResolveDictation(constants.EmptyString, constants.EmptyString, currentTenant.defaults.dictationProvider, currentTenant.defaults.dictationModel); verificationError != nil {
		return fmt.Errorf(tenantValidationErrorFormat, verificationError, currentTenant.identifier.string())
	}
	return nil
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
	configuration.QwenCloudKey = strings.TrimSpace(configuration.QwenCloudKey)
	configuration.MoonshotKey = strings.TrimSpace(configuration.MoonshotKey)
	configuration.MiniMaxKey = strings.TrimSpace(configuration.MiniMaxKey)
	configuration.SiliconFlowKey = strings.TrimSpace(configuration.SiliconFlowKey)
	configuration.ZhipuKey = strings.TrimSpace(configuration.ZhipuKey)
	configuration.GeminiKey = strings.TrimSpace(configuration.GeminiKey)
	configuration.AnthropicKey = strings.TrimSpace(configuration.AnthropicKey)
	configuration.MetaKey = strings.TrimSpace(configuration.MetaKey)
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
	configuration.QwenCloudBaseURL = strings.TrimSpace(configuration.QwenCloudBaseURL)
	if strings.TrimSpace(configuration.QwenCloudBaseURL) == constants.EmptyString {
		configuration.QwenCloudBaseURL = defaultQwenCloudBaseURL
	}
	configuration.MoonshotBaseURL = strings.TrimSpace(configuration.MoonshotBaseURL)
	if strings.TrimSpace(configuration.MoonshotBaseURL) == constants.EmptyString {
		configuration.MoonshotBaseURL = defaultMoonshotBaseURL
	}
	configuration.MiniMaxBaseURL = strings.TrimSpace(configuration.MiniMaxBaseURL)
	if strings.TrimSpace(configuration.MiniMaxBaseURL) == constants.EmptyString {
		configuration.MiniMaxBaseURL = defaultMiniMaxBaseURL
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
	configuration.MetaBaseURL = strings.TrimSpace(configuration.MetaBaseURL)
	if strings.TrimSpace(configuration.MetaBaseURL) == constants.EmptyString {
		configuration.MetaBaseURL = defaultMetaBaseURL
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
	for emailIndex, emailValue := range configuration.AdminEmails {
		configuration.AdminEmails[emailIndex] = strings.ToLower(strings.TrimSpace(emailValue))
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
	configuration.ProviderKeyEncryptionKey = strings.TrimSpace(configuration.ProviderKeyEncryptionKey)
	configuration.ManagementAPIOrigin = strings.TrimSpace(configuration.ManagementAPIOrigin)
	configuration.ProxyOrigin = strings.TrimSpace(configuration.ProxyOrigin)
	configuration.LegacyTokenMigration.TenantID = strings.TrimSpace(configuration.LegacyTokenMigration.TenantID)
	configuration.LegacyTokenMigration.OwnerEmail = strings.ToLower(strings.TrimSpace(configuration.LegacyTokenMigration.OwnerEmail))
}

func validateManagementConfiguration(configuration ManagementConfiguration) error {
	migrationConfigured := configuration.LegacyTokenMigration.TenantID != constants.EmptyString || configuration.LegacyTokenMigration.OwnerEmail != constants.EmptyString
	if !configuration.Enabled {
		if migrationConfigured {
			return fmt.Errorf("%w: field=management.legacy_token_migration requires_management", ErrInvalidManagementConfiguration)
		}
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
		{fieldName: "management.database_dialect", fieldValue: configuration.DatabaseDialect},
		{fieldName: "management.database_dsn", fieldValue: configuration.DatabaseDSN},
		{fieldName: "management.provider_key_encryption_key", fieldValue: configuration.ProviderKeyEncryptionKey},
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
	for _, emailValue := range configuration.AdminEmails {
		if _, emailError := normalizeManagementEmail(emailValue); emailError != nil {
			return fmt.Errorf("%w: field=management.admin_emails value=%s", ErrInvalidManagementConfiguration, emailValue)
		}
	}
	if migrationError := validateLegacyTokenMigrationConfiguration(configuration.LegacyTokenMigration); migrationError != nil {
		return migrationError
	}
	if !supportedManagementDatabaseDialect(configuration.DatabaseDialect) {
		return fmt.Errorf("%w: field=management.database_dialect value=%s", ErrInvalidManagementConfiguration, configuration.DatabaseDialect)
	}
	if _, keyError := newManagedProviderKeyCipher(configuration.ProviderKeyEncryptionKey); keyError != nil {
		return fmt.Errorf("%w: field=management.provider_key_encryption_key: %v", ErrInvalidManagementConfiguration, keyError)
	}
	return nil
}

func decodeManagedProviderKey(rawEncryptionKey string) ([managedProviderKeyBytes]byte, error) {
	decodedKey, decodeError := base64.StdEncoding.DecodeString(strings.TrimSpace(rawEncryptionKey))
	if decodeError != nil {
		return [managedProviderKeyBytes]byte{}, fmt.Errorf("invalid_base64")
	}
	if len(decodedKey) != managedProviderKeyBytes {
		return [managedProviderKeyBytes]byte{}, fmt.Errorf("invalid_length=%d", len(decodedKey))
	}
	var encryptionKey [managedProviderKeyBytes]byte
	copy(encryptionKey[:], decodedKey)
	return encryptionKey, nil
}

func validateLegacyTokenMigrationConfiguration(configuration LegacyTokenMigrationConfiguration) error {
	tenantIdentifier := strings.TrimSpace(configuration.TenantID)
	ownerEmail := strings.TrimSpace(configuration.OwnerEmail)
	if tenantIdentifier == constants.EmptyString && ownerEmail == constants.EmptyString {
		return nil
	}
	if tenantIdentifier == constants.EmptyString {
		return fmt.Errorf("%w: field=management.legacy_token_migration.tenant_id", ErrInvalidManagementConfiguration)
	}
	if ownerEmail == constants.EmptyString {
		return fmt.Errorf("%w: field=management.legacy_token_migration.owner_email", ErrInvalidManagementConfiguration)
	}
	if _, emailError := normalizeManagementEmail(ownerEmail); emailError != nil {
		return fmt.Errorf("%w: field=management.legacy_token_migration.owner_email value=%s", ErrInvalidManagementConfiguration, ownerEmail)
	}
	return nil
}

func normalizeManagementEmail(rawEmail string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(rawEmail))
	if email == constants.EmptyString {
		return constants.EmptyString, ErrInvalidManagementConfiguration
	}
	parsedAddress, parseError := mail.ParseAddress(email)
	if parseError != nil || parsedAddress.Address != email || parsedAddress.Name != constants.EmptyString {
		return constants.EmptyString, ErrInvalidManagementConfiguration
	}
	return email, nil
}

func supportedManagementDatabaseDialect(databaseDialect string) bool {
	supportedDialects := map[string]struct{}{
		ManagementDatabaseDialectPostgres: {},
		ManagementDatabaseDialectSQLite:   {},
	}
	_, supportedDialect := supportedDialects[databaseDialect]
	return supportedDialect
}
