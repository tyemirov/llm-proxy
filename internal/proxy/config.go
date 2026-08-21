package proxy

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"path/filepath"
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

	// DefaultRequestTimeoutSeconds is the request work budget used when the client omits one.
	DefaultRequestTimeoutSeconds = 360
	// DefaultMaxRequestTimeoutSeconds is the one-hour operator capacity ceiling for a request.
	DefaultMaxRequestTimeoutSeconds = 60 * 60
	// DefaultMaxPromptBytes limits JSON LLM request bodies accepted by POST /.
	DefaultMaxPromptBytes = 4 * 1024 * 1024
	// MaxV2RequestBytes limits each buffered canonical message request.
	MaxV2RequestBytes = 8 * 1024 * 1024
	// DefaultMaxAssetBytes bounds one tenant asset upload at the largest supported provider file size.
	DefaultMaxAssetBytes int64 = 2_000_000_000
	// DefaultAssetRetentionSeconds keeps uploaded tenant assets for 48 hours.
	DefaultAssetRetentionSeconds = 48 * 60 * 60
	// DefaultAssetStorePath is the persistent filesystem location for tenant assets.
	DefaultAssetStorePath      = "/data/assets"
	DefaultDictationModel      = "gpt-4o-mini-transcribe"
	DefaultMaxInputAudioBytes  = 25 * 1024 * 1024
	DefaultManagementJWTIssuer = "tauth"
	// DefaultManagementUsageQueueSize is the number of managed usage events retained for asynchronous persistence.
	DefaultManagementUsageQueueSize = 1024
	managedProviderKeyBytes         = 32
)

// Configuration holds runtime settings.
type Configuration struct {
	Management                 ManagementConfiguration
	Port                       int
	LogLevel                   string
	WorkerCount                int
	QueueSize                  int
	RequestTimeoutSeconds      int
	MaxRequestTimeoutSeconds   int
	MaxPromptBytes             int64
	MaxAssetBytes              int64
	AssetRetentionSeconds      int
	AssetStorePath             string
	MaxInputAudioBytes         int64
	UpstreamRateLimits         []UpstreamRateLimitConfiguration
	Endpoints                  *Endpoints
	ProviderCatalog            *ProviderCatalog
	ProviderConnectionValues   map[string]map[string]string
	ModelCatalog               ModelCatalog
	upstreamRateLimits         upstreamRateLimits
	managementSessionValidator *managementSessionValidator
	requestTimeoutPolicy       requestTimeoutPolicy
	validated                  bool
}

// ManagementConfiguration holds authenticated browser UI and self-service tenant settings.
type ManagementConfiguration struct {
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
	SessionPath              string
	JWTSigningKey            string
	JWTIssuer                string
	SessionCookieName        string
	DatabasePath             string
	UsageQueueSize           int
	ProviderKeyEncryptionKey string
	ManagementAPIOrigin      string
	ProxyOrigin              string
	DatabaseDialector        gorm.Dialector
}

// NewConfiguration returns a normalized runtime configuration after validating startup invariants.
func NewConfiguration(configuration Configuration) (Configuration, error) {
	if configuration.ProviderCatalog != nil {
		configuration.ModelCatalog = configuration.ProviderCatalog.ModelCatalog()
		connectionValues, connectionValuesError := configuration.ProviderCatalog.validatedConnectionValues(configuration.ProviderConnectionValues)
		if connectionValuesError != nil {
			return Configuration{}, connectionValuesError
		}
		configuration.ProviderConnectionValues = connectionValues
	}
	configuration.ApplyTunables()
	timeoutPolicy, timeoutPolicyError := newRequestTimeoutPolicy(configuration.RequestTimeoutSeconds, configuration.MaxRequestTimeoutSeconds)
	if timeoutPolicyError != nil {
		return Configuration{}, timeoutPolicyError
	}
	upstreamRateLimits, rateLimitError := newUpstreamRateLimits(configuration.UpstreamRateLimits)
	if rateLimitError != nil {
		return Configuration{}, rateLimitError
	}
	if validationError := validateConfig(configuration); validationError != nil {
		return Configuration{}, validationError
	}
	sessionValidator, sessionValidationError := newManagementSessionValidator(configuration.Management)
	if sessionValidationError != nil {
		return Configuration{}, sessionValidationError
	}
	configuration.upstreamRateLimits = upstreamRateLimits
	configuration.managementSessionValidator = sessionValidator
	configuration.requestTimeoutPolicy = timeoutPolicy
	configuration.validated = true
	return configuration, nil
}

func ensureValidatedConfiguration(configuration Configuration) (Configuration, error) {
	if configuration.validated {
		return configuration, nil
	}
	return NewConfiguration(configuration)
}

func validateConfig(configuration Configuration) error {
	if !filepath.IsAbs(configuration.AssetStorePath) || filepath.Clean(configuration.AssetStorePath) != configuration.AssetStorePath {
		return fmt.Errorf("invalid asset_store_path")
	}
	if managementValidationError := validateManagementConfiguration(configuration.Management); managementValidationError != nil {
		return managementValidationError
	}
	if configuration.ProviderCatalog == nil {
		return fmt.Errorf("%w: field=provider_catalog", ErrInvalidModelCatalog)
	}
	return nil
}

var errProviderOutputLimitReached = errors.New("provider output limit reached")
var errQueueFull = errors.New(errorQueueFull)

// ApplyTunables ensures tunable configuration values have sensible defaults.
func (configuration *Configuration) ApplyTunables() {
	configuration.Management.ApplyTunables()
	if configuration.WorkerCount <= 0 {
		configuration.WorkerCount = DefaultWorkers
	}
	if configuration.QueueSize <= 0 {
		configuration.QueueSize = DefaultQueueSize
	}
	if configuration.RequestTimeoutSeconds == 0 {
		configuration.RequestTimeoutSeconds = DefaultRequestTimeoutSeconds
	}
	if configuration.MaxRequestTimeoutSeconds == 0 {
		configuration.MaxRequestTimeoutSeconds = DefaultMaxRequestTimeoutSeconds
	}
	if configuration.MaxPromptBytes <= 0 {
		configuration.MaxPromptBytes = DefaultMaxPromptBytes
	}
	if configuration.MaxAssetBytes <= 0 {
		configuration.MaxAssetBytes = DefaultMaxAssetBytes
	}
	if configuration.AssetRetentionSeconds <= 0 {
		configuration.AssetRetentionSeconds = DefaultAssetRetentionSeconds
	}
	configuration.AssetStorePath = strings.TrimSpace(configuration.AssetStorePath)
	if configuration.AssetStorePath == constants.EmptyString {
		configuration.AssetStorePath = DefaultAssetStorePath
	}
	if configuration.MaxInputAudioBytes <= 0 {
		configuration.MaxInputAudioBytes = DefaultMaxInputAudioBytes
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
	configuration.SessionPath = strings.TrimSpace(configuration.SessionPath)
	configuration.JWTSigningKey = strings.TrimSpace(configuration.JWTSigningKey)
	configuration.JWTIssuer = strings.TrimSpace(configuration.JWTIssuer)
	if configuration.JWTIssuer == constants.EmptyString {
		configuration.JWTIssuer = DefaultManagementJWTIssuer
	}
	configuration.SessionCookieName = strings.TrimSpace(configuration.SessionCookieName)
	configuration.DatabasePath = strings.TrimSpace(configuration.DatabasePath)
	if configuration.UsageQueueSize == 0 {
		configuration.UsageQueueSize = DefaultManagementUsageQueueSize
	}
	configuration.ProviderKeyEncryptionKey = strings.TrimSpace(configuration.ProviderKeyEncryptionKey)
	configuration.ManagementAPIOrigin = strings.TrimSpace(configuration.ManagementAPIOrigin)
	configuration.ProxyOrigin = strings.TrimSpace(configuration.ProxyOrigin)
}

func validateManagementConfiguration(configuration ManagementConfiguration) error {
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
		{fieldName: "management.session_path", fieldValue: configuration.SessionPath},
		{fieldName: "management.database_path", fieldValue: configuration.DatabasePath},
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
	if configuration.UsageQueueSize <= 0 {
		return fmt.Errorf("%w: field=management.usage_queue_size", ErrInvalidManagementConfiguration)
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
