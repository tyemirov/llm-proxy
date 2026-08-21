package proxy

import (
	"crypto/sha256"
	"errors"
	"strings"
)

var (
	ErrInvalidManagementConfiguration = errors.New("invalid management configuration")
)

// TenantDefaults holds default request values selected by an authenticated tenant.
type TenantDefaults struct {
	Provider          string
	Model             string
	DictationProvider string
	DictationModel    string
	SystemPrompt      string
	ReasoningEffort   string
}

// DefaultTenantDefaults returns the canonical routing defaults assigned to a new managed tenant.
func DefaultTenantDefaults() TenantDefaults {
	return TenantDefaults{
		Provider:          DefaultProvider,
		Model:             DefaultModel,
		DictationProvider: DefaultDictationProvider,
		DictationModel:    DefaultDictationModel,
	}
}

type tenantID string

func (identifier tenantID) string() string {
	return string(identifier)
}

type tenantDefaults struct {
	provider          string
	model             string
	dictationProvider string
	dictationModel    string
	systemPrompt      string
	reasoningEffort   string
}

func newTenantDefaults(rawDefaults TenantDefaults) tenantDefaults {
	return normalizedTenantDefaults(rawDefaults)
}

func normalizedTenantDefaults(rawDefaults TenantDefaults) tenantDefaults {
	defaults := tenantDefaults{
		provider:          strings.TrimSpace(rawDefaults.Provider),
		model:             strings.TrimSpace(rawDefaults.Model),
		dictationProvider: strings.TrimSpace(rawDefaults.DictationProvider),
		dictationModel:    strings.TrimSpace(rawDefaults.DictationModel),
		systemPrompt:      rawDefaults.SystemPrompt,
		reasoningEffort:   rawDefaults.ReasoningEffort,
	}
	defaults.provider = strings.ToLower(defaults.provider)
	defaults.dictationProvider = strings.ToLower(defaults.dictationProvider)
	return defaults
}

type managedProviderSettings struct {
	connectionValues   map[string]string
	connectionVersions map[string]managedProviderConnectionVersion
	configuredFields   map[string]bool
	textModel          string
	systemPrompt       string
}

type managedProviderConnectionVersion [sha256.Size]byte

func (settings managedProviderSettings) connectionValue(fieldIdentifier string) string {
	return strings.TrimSpace(settings.connectionValues[fieldIdentifier])
}

func (settings managedProviderSettings) connectionVersion(fieldIdentifier string) managedProviderConnectionVersion {
	return settings.connectionVersions[fieldIdentifier]
}

func (settings managedProviderSettings) fieldConfigured(fieldIdentifier string) bool {
	return settings.configuredFields[fieldIdentifier]
}

func (settings managedProviderSettings) hasRequiredConnectionFields(definition providerDefinition) bool {
	for fieldIdentifier, field := range definition.fields {
		if field.Required && settings.connectionValue(fieldIdentifier) == "" {
			return false
		}
	}
	return true
}

type tenant struct {
	identifier       tenantID
	userID           string
	secretDigest     [sha256.Size]byte
	defaults         tenantDefaults
	providerSettings map[providerID]managedProviderSettings
}
