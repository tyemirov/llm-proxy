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
	Provider              string
	Model                 string
	TranscriptionProvider string
	TranscriptionModel    string
	SpeechProvider        string
	SpeechModel           string
	SystemPrompt          string
	ReasoningEffort       string
}

// DefaultTenantDefaults returns the canonical routing defaults assigned to a new managed tenant.
func DefaultTenantDefaults() TenantDefaults {
	return TenantDefaults{
		Provider:              DefaultProvider,
		Model:                 DefaultModel,
		TranscriptionProvider: DefaultTranscriptionProvider,
		TranscriptionModel:    DefaultTranscriptionModel,
	}
}

type tenantID string

func (identifier tenantID) string() string {
	return string(identifier)
}

type tenantDefaults struct {
	provider              string
	model                 string
	transcriptionProvider string
	transcriptionModel    string
	speechProvider        string
	speechModel           string
	systemPrompt          string
	reasoningEffort       string
}

func newTenantDefaults(rawDefaults TenantDefaults) tenantDefaults {
	return normalizedTenantDefaults(rawDefaults)
}

func normalizedTenantDefaults(rawDefaults TenantDefaults) tenantDefaults {
	defaults := tenantDefaults{
		provider:              strings.TrimSpace(rawDefaults.Provider),
		model:                 strings.TrimSpace(rawDefaults.Model),
		transcriptionProvider: strings.TrimSpace(rawDefaults.TranscriptionProvider),
		transcriptionModel:    strings.TrimSpace(rawDefaults.TranscriptionModel),
		speechProvider:        strings.TrimSpace(rawDefaults.SpeechProvider),
		speechModel:           strings.TrimSpace(rawDefaults.SpeechModel),
		systemPrompt:          rawDefaults.SystemPrompt,
		reasoningEffort:       rawDefaults.ReasoningEffort,
	}
	defaults.provider = strings.ToLower(defaults.provider)
	defaults.transcriptionProvider = strings.ToLower(defaults.transcriptionProvider)
	defaults.speechProvider = strings.ToLower(defaults.speechProvider)
	return defaults
}

type managedProviderSettings struct {
	connectionID     string
	hostedGrantID    string
	hostedOfferings  []hostedGrantOffering
	connectionValues map[string]string
	configuredFields map[string]bool
	textModel        string
	systemPrompt     string
}

func (settings managedProviderSettings) connectionValue(fieldIdentifier string) string {
	return strings.TrimSpace(settings.connectionValues[fieldIdentifier])
}

func (settings managedProviderSettings) fieldConfigured(fieldIdentifier string) bool {
	return settings.configuredFields[fieldIdentifier]
}

func (settings managedProviderSettings) hasRequiredConnectionFields(definition providerDefinition) bool {
	// Hosted credentials are supplied only by the funds admission boundary.
	if settings.hostedGrantID != "" {
		return false
	}
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
