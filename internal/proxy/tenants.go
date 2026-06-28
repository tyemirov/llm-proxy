package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

var (
	ErrMissingTenants                 = errors.New("tenants must include at least one tenant")
	ErrInvalidTenant                  = errors.New("invalid tenant")
	ErrInvalidManagementConfiguration = errors.New("invalid management configuration")
)

// TenantConfiguration is the config-file shape for one authenticated tenant.
type TenantConfiguration struct {
	ID       string
	Secret   string
	Defaults TenantDefaults
}

// TenantDefaults holds default request values selected by an authenticated tenant.
type TenantDefaults struct {
	Provider          string
	Model             string
	DictationProvider string
	DictationModel    string
	SystemPrompt      string
}

type tenantID string

func newTenantID(rawIdentifier string) (tenantID, error) {
	normalizedIdentifier := strings.TrimSpace(rawIdentifier)
	if normalizedIdentifier == constants.EmptyString {
		return tenantID(""), fmt.Errorf("%w: id must be set", ErrInvalidTenant)
	}
	return tenantID(normalizedIdentifier), nil
}

func (identifier tenantID) string() string {
	return string(identifier)
}

type tenantDefaults struct {
	provider          string
	model             string
	dictationProvider string
	dictationModel    string
	systemPrompt      string
}

func newTenantDefaults(rawDefaults TenantDefaults) tenantDefaults {
	defaults := tenantDefaults{
		provider:          strings.TrimSpace(rawDefaults.Provider),
		model:             strings.TrimSpace(rawDefaults.Model),
		dictationProvider: strings.TrimSpace(rawDefaults.DictationProvider),
		dictationModel:    strings.TrimSpace(rawDefaults.DictationModel),
		systemPrompt:      rawDefaults.SystemPrompt,
	}
	if defaults.provider == constants.EmptyString {
		defaults.provider = DefaultProvider
	}
	defaults.provider = strings.ToLower(defaults.provider)
	if defaults.dictationProvider == constants.EmptyString {
		defaults.dictationProvider = DefaultDictationProvider
	}
	defaults.dictationProvider = strings.ToLower(defaults.dictationProvider)
	return defaults
}

type tenant struct {
	identifier      tenantID
	secretDigest    [sha256.Size]byte
	defaults        tenantDefaults
	managed         bool
	providerAPIKeys map[providerID]string
}

func newTenant(rawTenant TenantConfiguration) (tenant, error) {
	identifier, identifierError := newTenantID(rawTenant.ID)
	if identifierError != nil {
		return tenant{}, identifierError
	}
	normalizedSecret := strings.TrimSpace(rawTenant.Secret)
	if normalizedSecret == constants.EmptyString {
		return tenant{}, fmt.Errorf("%w: id=%s secret must be set", ErrInvalidTenant, identifier.string())
	}
	return tenant{
		identifier:   identifier,
		secretDigest: sha256.Sum256([]byte(normalizedSecret)),
		defaults:     newTenantDefaults(rawTenant.Defaults),
	}, nil
}

type tenantRegistry struct {
	tenants []tenant
}

func newTenantRegistry(rawTenants []TenantConfiguration, allowEmpty bool) (tenantRegistry, error) {
	if len(rawTenants) == 0 {
		if allowEmpty {
			return tenantRegistry{}, nil
		}
		return tenantRegistry{}, ErrMissingTenants
	}
	seenIdentifiers := map[string]struct{}{}
	seenSecretDigests := map[string]tenantID{}
	tenants := make([]tenant, 0, len(rawTenants))
	for _, rawTenant := range rawTenants {
		currentTenant, tenantError := newTenant(rawTenant)
		if tenantError != nil {
			return tenantRegistry{}, tenantError
		}
		identifier := currentTenant.identifier.string()
		if _, exists := seenIdentifiers[identifier]; exists {
			return tenantRegistry{}, fmt.Errorf("%w: duplicate id=%s", ErrInvalidTenant, identifier)
		}
		seenIdentifiers[identifier] = struct{}{}
		secretDigest := hex.EncodeToString(currentTenant.secretDigest[:])
		if existingIdentifier, exists := seenSecretDigests[secretDigest]; exists {
			return tenantRegistry{}, fmt.Errorf("%w: duplicate secret tenant=%s existing_tenant=%s", ErrInvalidTenant, identifier, existingIdentifier.string())
		}
		seenSecretDigests[secretDigest] = currentTenant.identifier
		tenants = append(tenants, currentTenant)
	}
	return tenantRegistry{tenants: tenants}, nil
}

func (registry tenantRegistry) authenticate(rawSecret string) (tenant, bool) {
	presentedSecret := strings.TrimSpace(rawSecret)
	presentedDigest := sha256.Sum256([]byte(presentedSecret))
	var matchedTenant tenant
	matched := false
	for _, candidate := range registry.tenants {
		if constantTimeDigestEquals(candidate.secretDigest, presentedDigest) {
			matchedTenant = candidate
			matched = true
		}
	}
	return matchedTenant, matched
}

func (registry tenantRegistry) containsSecretDigest(secretDigest [sha256.Size]byte) bool {
	for _, candidate := range registry.tenants {
		if constantTimeDigestEquals(candidate.secretDigest, secretDigest) {
			return true
		}
	}
	return false
}

// DefaultTenantDefaults returns the built-in request defaults for a single tenant.
func DefaultTenantDefaults() TenantDefaults {
	return TenantDefaults{
		Provider:          DefaultProvider,
		Model:             DefaultModel,
		DictationProvider: DefaultDictationProvider,
		DictationModel:    DefaultDictationModel,
		SystemPrompt:      constants.EmptyString,
	}
}

// DefaultTenantConfiguration returns one tenant using the built-in request defaults.
func DefaultTenantConfiguration(identifier string, secret string) TenantConfiguration {
	return TenantConfiguration{
		ID:       identifier,
		Secret:   secret,
		Defaults: DefaultTenantDefaults(),
	}
}

// SingleTenantConfigurations returns a tenant slice for tests and small deployments.
func SingleTenantConfigurations(identifier string, secret string) []TenantConfiguration {
	return []TenantConfiguration{DefaultTenantConfiguration(identifier, secret)}
}

// SingleTenantConfigurationsWithDefaults returns one tenant with caller-specified request defaults.
func SingleTenantConfigurationsWithDefaults(identifier string, secret string, defaults TenantDefaults) []TenantConfiguration {
	return []TenantConfiguration{{
		ID:       identifier,
		Secret:   secret,
		Defaults: defaults,
	}}
}
