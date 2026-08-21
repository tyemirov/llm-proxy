package proxy

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const managedRouterTestDashScopeBaseURL = "https://managed-router-test.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1"

// ManagedTenantTestConfiguration describes one managed tenant used by router tests.
type ManagedTenantTestConfiguration struct {
	ID                    string
	Secret                string
	Defaults              TenantDefaults
	ProviderKeys          map[string]string
	ProviderTextModels    map[string]string
	ProviderSystemPrompts map[string]string
}

// StandardManagedTenantTestConfiguration returns the common managed tenant used by proxy tests.
func StandardManagedTenantTestConfiguration(secret string) ManagedTenantTestConfiguration {
	return ManagedTenantTestConfiguration{
		ID:       "test",
		Secret:   secret,
		Defaults: DefaultTenantDefaults(),
		ProviderKeys: map[string]string{
			ProviderNameOpenAI:      "sk-test",
			ProviderNameDeepSeek:    "sk-deepseek",
			ProviderNameDashScope:   "sk-dashscope",
			ProviderNameMoonshot:    "sk-moonshot",
			ProviderNameMiniMax:     "sk-minimax",
			ProviderNameSiliconFlow: "sk-siliconflow",
			ProviderNameZAI:         "sk-zai",
			ProviderNameGemini:      "sk-gemini",
			ProviderNameAnthropic:   "sk-ant",
			ProviderNameMeta:        "sk-meta",
			ProviderNameXAI:         "sk-xai",
		},
	}
}

// BuildRouterWithManagedTenantForTest builds a router whose client and provider credentials come from a managed tenant store.
func BuildRouterWithManagedTenantForTest(testingInstance testing.TB, configuration Configuration, structuredLogger *zap.SugaredLogger, tenantConfiguration ManagedTenantTestConfiguration) (*gin.Engine, error) {
	return BuildRouterWithManagedTenantsForTest(testingInstance, configuration, structuredLogger, []ManagedTenantTestConfiguration{tenantConfiguration})
}

// BuildRouterWithManagedTenantsForTest builds a router with explicit managed tenant records.
func BuildRouterWithManagedTenantsForTest(testingInstance testing.TB, configuration Configuration, structuredLogger *zap.SugaredLogger, tenantConfigurations []ManagedTenantTestConfiguration) (*gin.Engine, error) {
	testingInstance.Helper()
	if configuration.ProviderCatalog == nil {
		if len(configuration.ModelCatalog.Offerings) == 0 {
			configuration.ProviderCatalog = internalCanonicalProviderCatalog()
		} else {
			configuration.ProviderCatalog = internalTestProviderCatalog(configuration.ModelCatalog)
		}
	}
	configuration.Endpoints = managedRouterTestEndpointOverrides(configuration)
	configuration.Management = managedRouterTestManagementConfiguration()
	return buildRouter(configuration, structuredLogger, func(_ ManagementConfiguration, providers *providerRegistry) (*managedTenantStore, error) {
		return newManagedRouterTestStore(configuration, providers, tenantConfigurations)
	})
}

func managedRouterTestEndpointOverrides(configuration Configuration) *Endpoints {
	endpoints := configuration.Endpoints
	if endpoints == nil {
		endpoints = NewEndpoints()
	}
	return endpoints
}

func managedRouterTestManagementConfiguration() ManagementConfiguration {
	return ManagementConfiguration{
		PublicOrigin:             "http://localhost:8080",
		UIDescription:            "LLM Proxy",
		UIOrigins:                []string{"http://localhost:8080"},
		TAuthURL:                 "http://localhost:8443",
		TAuthTenantID:            "llm-proxy-test",
		GoogleClientID:           "google-client-id",
		LoginPath:                "/auth/google",
		LogoutPath:               "/auth/logout",
		NoncePath:                "/auth/nonce",
		SessionPath:              "/auth/session",
		JWTSigningKey:            "managed-router-test-signing-key",
		SessionCookieName:        "managed_router_test_session",
		DatabasePath:             "managed-router-test.db",
		ProviderKeyEncryptionKey: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		ManagementAPIOrigin:      "http://localhost:8080",
		ProxyOrigin:              "http://localhost:8080",
	}
}

// ManagedRouterTestManagementConfiguration returns valid mandatory management settings for configuration-only tests.
func ManagedRouterTestManagementConfiguration() ManagementConfiguration {
	return managedRouterTestManagementConfiguration()
}

func newManagedRouterTestStore(configuration Configuration, providers *providerRegistry, tenantConfigurations []ManagedTenantTestConfiguration) (*managedTenantStore, error) {
	database := newFakeManagedTenantDatabase()
	providerKeyCipher := internalManagedProviderKeyCipher()
	now := time.Now().UTC()
	for tenantIndex, tenantConfiguration := range tenantConfigurations {
		identifier := tenantConfiguration.ID
		if identifier == "" {
			identifier = fmt.Sprintf("test-%d", tenantIndex+1)
		}
		if tenantConfiguration.Secret == "" {
			return nil, fmt.Errorf("managed router test secret must be set")
		}
		defaults := tenantConfiguration.Defaults
		if defaults == (TenantDefaults{}) {
			defaults = DefaultTenantDefaults()
		}
		secretDigest := sha256.Sum256([]byte(tenantConfiguration.Secret))
		secretDigestText := hex.EncodeToString(secretDigest[:])
		ownerUserID := fmt.Sprintf("managed-router-test-user-%d", tenantIndex+1)
		record := managedTenantRecord{
			TenantID:                 identifier,
			OwnerUserID:              ownerUserID,
			Name:                     fmt.Sprintf("Test %d", tenantIndex+1),
			NameKey:                  fmt.Sprintf("test-%d", tenantIndex+1),
			SecretDigest:             &secretDigestText,
			DefaultProvider:          defaults.Provider,
			DefaultModel:             defaults.Model,
			DefaultDictationProvider: defaults.DictationProvider,
			DefaultDictationModel:    defaults.DictationModel,
			DefaultSystemPrompt:      defaults.SystemPrompt,
			DefaultReasoningEffort:   defaults.ReasoningEffort,
			CreatedAt:                now,
			UpdatedAt:                now,
		}
		for providerIdentifier, apiKey := range tenantConfiguration.ProviderKeys {
			providerIDValue, providerError := providers.canonicalProviderID(providerIdentifier)
			if providerError != nil {
				return nil, providerError
			}
			definition := providers.definitions[providerIDValue]
			connections := make([]managedProviderConnectionRecord, 0, len(definition.fields))
			connectionComplete := true
			for fieldIdentifier, field := range definition.fields {
				value := *field.Default
				if field.Kind == CatalogProviderFieldKindCredential {
					value = apiKey
				} else if fieldIdentifier == "base_url" {
					value = managedRouterTestDashScopeBaseURL
				}
				if value == "" {
					connectionComplete = connectionComplete && !field.Required
					continue
				}
				storedValue := value
				if field.Secret {
					encryptedValue, encryptionError := providerKeyCipher.encryptConnection(rand.Reader, identifier, providerIDValue.string(), fieldIdentifier, value)
					if encryptionError != nil {
						return nil, encryptionError
					}
					storedValue = encryptedValue
				}
				connections = append(connections, managedProviderConnectionRecord{
					TenantID: identifier, ProviderID: providerIDValue.string(), FieldID: fieldIdentifier,
					Value: storedValue, CreatedAt: now, UpdatedAt: now,
				})
			}
			if !connectionComplete {
				continue
			}
			textModel := tenantConfiguration.ProviderTextModels[providerIDValue.string()]
			if textModel == "" {
				textModel = definition.defaultTextModel.string()
			}
			record.ProviderConnections = append(record.ProviderConnections, connections...)
			record.ProviderProfiles = append(record.ProviderProfiles, managedProviderProfileRecord{
				TenantID: identifier, ProviderID: providerIDValue.string(), TextModel: textModel,
				SystemPrompt: tenantConfiguration.ProviderSystemPrompts[providerIDValue.string()], CreatedAt: now, UpdatedAt: now,
			})
		}
		database.usersByID[record.OwnerUserID] = managedUserRecord{UserID: record.OwnerUserID, CreatedAt: now, UpdatedAt: now}
		database.tenantsByID[record.TenantID] = record
	}
	store := newManagedTenantStoreWithDatabaseAndCipher(database, providerKeyCipher)
	store.routingDefaults = providers
	for _, tenantConfiguration := range tenantConfigurations {
		secretDigest := sha256.Sum256([]byte(tenantConfiguration.Secret))
		record, recordError := database.tenantBySecretDigest(context.Background(), hex.EncodeToString(secretDigest[:]))
		if recordError != nil {
			return nil, fmt.Errorf("managed router test tenant lookup failed: tenant=%s: %w", tenantConfiguration.ID, recordError)
		}
		if _, tenantError := store.tenant(record, secretDigest); tenantError != nil {
			return nil, fmt.Errorf("managed router test tenant construction failed: tenant=%s: %w", tenantConfiguration.ID, tenantError)
		}
	}
	return store, nil
}

func managedTenantForInternalTest(identifier string, secret string) tenant {
	secretDigest := sha256.Sum256([]byte(secret))
	return tenant{
		identifier:   tenantID(identifier),
		secretDigest: secretDigest,
		defaults:     newTenantDefaults(DefaultTenantDefaults()),
	}
}
