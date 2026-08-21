package testfixtures

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	managedRouterSigningKey    = "managed-router-fixture-signing-key"
	managedRouterTAuthTenantID = "llm-proxy-test"
	managedRouterCookieName    = "managed_router_fixture_session"
	managedRouterEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	managedRouterUserID        = "managed-router-fixture-user"
	managedRouterDashScopeURL  = "https://managed-router-fixture.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1"
)

// ManagedTenant describes one tenant provisioned through the management contract for a router test.
type ManagedTenant struct {
	Secret         string
	Defaults       proxy.TenantDefaults
	ProviderKeys   map[string]string
	ProviderFields map[string]map[string]string
}

// StandardManagedTenant returns the common managed tenant used by black-box tests.
func StandardManagedTenant(secret string) ManagedTenant {
	return ManagedTenant{
		Secret:   secret,
		Defaults: proxy.DefaultTenantDefaults(),
		ProviderKeys: map[string]string{
			proxy.ProviderNameOpenAI:      "sk-test",
			proxy.ProviderNameDeepSeek:    "sk-deepseek",
			proxy.ProviderNameDashScope:   "sk-dashscope",
			proxy.ProviderNameMoonshot:    "sk-moonshot",
			proxy.ProviderNameMiniMax:     "sk-minimax",
			proxy.ProviderNameSiliconFlow: "sk-siliconflow",
			proxy.ProviderNameZAI:         "sk-zai",
			proxy.ProviderNameGemini:      "sk-gemini",
			proxy.ProviderNameAnthropic:   "sk-ant",
			proxy.ProviderNameMeta:        "sk-meta",
			proxy.ProviderNameXAI:         "sk-xai",
		},
		ProviderFields: map[string]map[string]string{
			proxy.ProviderNameDashScope: {"base_url": managedRouterDashScopeURL},
		},
	}
}

// BuildManagedRouter builds and provisions a router through the mandatory management API.
func BuildManagedRouter(testingInstance testing.TB, configuration proxy.Configuration, structuredLogger *zap.SugaredLogger, tenant ManagedTenant) (*gin.Engine, error) {
	testingInstance.Helper()
	databasePath := "file:managed-router-" + rand.Text() + "?mode=memory&cache=shared"
	configuration.Management = managedRouterConfiguration(databasePath)
	originalHTTPClient := proxy.HTTPClient
	proxy.HTTPClient = managedProviderVerificationDoer{next: originalHTTPClient}
	defer func() { proxy.HTTPClient = originalHTTPClient }()
	configuration = WithModelCatalog(testingInstance, configuration)
	bootstrapConfiguration := configuration
	bootstrapConfiguration.UpstreamRateLimits = nil
	bootstrapRouter, buildError := proxy.BuildRouter(bootstrapConfiguration, structuredLogger)
	if buildError != nil {
		proxy.HTTPClient = originalHTTPClient
		return nil, buildError
	}
	sessionCookie, cookieError := managedRouterSessionCookie()
	if cookieError != nil {
		return nil, cookieError
	}
	tenantID, accountError := managedRouterTenantID(bootstrapRouter, sessionCookie)
	if accountError != nil {
		proxy.HTTPClient = originalHTTPClient
		return nil, accountError
	}
	for provider, apiKey := range tenant.ProviderKeys {
		providerError := saveManagedProviderKey(bootstrapRouter, sessionCookie, tenantID, configuration.ProviderCatalog, provider, apiKey, tenant.ProviderFields[provider])
		if providerError != nil {
			proxy.HTTPClient = originalHTTPClient
			return nil, providerError
		}
	}
	defaults := tenant.Defaults
	if defaults == (proxy.TenantDefaults{}) {
		defaults = proxy.DefaultTenantDefaults()
	}
	if defaultsError := saveManagedDefaults(bootstrapRouter, sessionCookie, tenantID, defaults); defaultsError != nil {
		proxy.HTTPClient = originalHTTPClient
		return nil, defaultsError
	}
	if strings.TrimSpace(tenant.Secret) == "" {
		proxy.HTTPClient = originalHTTPClient
		return nil, fmt.Errorf("managed router fixture secret must be set")
	}
	if secretError := setManagedSecret(databasePath, tenantID, tenant.Secret); secretError != nil {
		proxy.HTTPClient = originalHTTPClient
		return nil, secretError
	}
	router, buildError := proxy.BuildRouter(configuration, structuredLogger)
	proxy.HTTPClient = originalHTTPClient
	if buildError != nil {
		return nil, buildError
	}
	return router, nil
}

func managedRouterConfiguration(databasePath string) proxy.ManagementConfiguration {
	return proxy.ManagementConfiguration{
		PublicOrigin:             "http://localhost:8080",
		UIDescription:            "LLM Proxy",
		UIOrigins:                []string{"http://localhost:8080"},
		TAuthURL:                 "http://localhost:8443",
		TAuthTenantID:            managedRouterTAuthTenantID,
		GoogleClientID:           "google-client-id",
		LoginPath:                "/auth/google",
		LogoutPath:               "/auth/logout",
		NoncePath:                "/auth/nonce",
		SessionPath:              "/auth/session",
		JWTSigningKey:            managedRouterSigningKey,
		SessionCookieName:        managedRouterCookieName,
		DatabasePath:             databasePath,
		ProviderKeyEncryptionKey: managedRouterEncryptionKey,
		ManagementAPIOrigin:      "http://localhost:8080",
		ProxyOrigin:              "http://localhost:8080",
		DatabaseDialector:        sqlite.Open(databasePath),
	}
}

func managedRouterSessionCookie() (*http.Cookie, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss":        proxy.DefaultManagementJWTIssuer,
		"tenant_id":  managedRouterTAuthTenantID,
		"user_id":    managedRouterUserID,
		"user_email": "managed-router@example.com",
		"iat":        now.Add(-time.Minute).Unix(),
		"exp":        now.Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, signingError := token.SignedString([]byte(managedRouterSigningKey))
	if signingError != nil {
		return nil, signingError
	}
	return &http.Cookie{Name: managedRouterCookieName, Value: signedToken}, nil
}

func managedRouterTenantID(router http.Handler, sessionCookie *http.Cookie) (string, error) {
	request := httptest.NewRequest(http.MethodGet, "/api/management/account", nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		return "", fmt.Errorf("managed router account status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Tenants []struct {
			ID string `json:"id"`
		} `json:"tenants"`
	}
	if decodeError := json.Unmarshal(response.Body.Bytes(), &payload); decodeError != nil {
		return "", decodeError
	}
	if len(payload.Tenants) != 1 {
		return "", fmt.Errorf("managed router account tenant_count=%d", len(payload.Tenants))
	}
	return payload.Tenants[0].ID, nil
}

func saveManagedProviderKey(router http.Handler, sessionCookie *http.Cookie, tenantID string, catalog *proxy.ProviderCatalog, provider string, apiKey string, configuredFields map[string]string) error {
	fields := map[string]string{}
	for _, providerDefinition := range catalog.Schema().Providers {
		if providerDefinition.ID != provider {
			continue
		}
		for _, field := range providerDefinition.Fields {
			if field.Kind == proxy.CatalogProviderFieldKindCredential {
				fields[field.ID] = apiKey
				continue
			}
			if value := configuredFields[field.ID]; value != "" {
				fields[field.ID] = value
			}
		}
		break
	}
	body := map[string]any{"fields": fields, "text_model": managedProviderModel(provider)}
	encodedBody, encodeError := json.Marshal(body)
	if encodeError != nil {
		return encodeError
	}
	request := httptest.NewRequest(http.MethodPut, "/api/management/tenants/"+tenantID+"/provider-connections/"+provider, bytes.NewReader(encodedBody))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		return fmt.Errorf("managed router provider=%s status=%d body=%s", provider, response.Code, response.Body.String())
	}
	return nil
}

func saveManagedDefaults(router http.Handler, sessionCookie *http.Cookie, tenantID string, defaults proxy.TenantDefaults) error {
	encodedBody, encodeError := json.Marshal(map[string]any{
		"provider": defaults.Provider, "model": defaults.Model,
		"dictation_provider": defaults.DictationProvider, "dictation_model": defaults.DictationModel,
		"system_prompt": defaults.SystemPrompt, "reasoning_effort": defaults.ReasoningEffort,
	})
	if encodeError != nil {
		return encodeError
	}
	request := httptest.NewRequest(http.MethodPut, "/api/management/tenants/"+tenantID+"/defaults", bytes.NewReader(encodedBody))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		return fmt.Errorf("managed router defaults status=%d body=%s", response.Code, response.Body.String())
	}
	return nil
}

func setManagedSecret(databasePath string, tenantID string, secret string) error {
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		return openError
	}
	digest := sha256.Sum256([]byte(secret))
	result := database.Table("managed_tenant_records").Where("tenant_id = ?", tenantID).Update("secret_digest", hex.EncodeToString(digest[:]))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("managed router secret rows=%d", result.RowsAffected)
	}
	sqlDatabase, databaseError := database.DB()
	if databaseError != nil {
		return databaseError
	}
	return sqlDatabase.Close()
}

func managedProviderModel(provider string) string {
	switch provider {
	case proxy.ProviderNameOpenAI:
		return proxy.ModelNameGPT41
	case proxy.ProviderNameDeepSeek:
		return proxy.ModelNameDeepSeekV4Flash
	case proxy.ProviderNameDashScope:
		return proxy.ModelNameDashScopeQwenPlus
	case proxy.ProviderNameMoonshot:
		return proxy.ModelNameMoonshotKimiK26
	case proxy.ProviderNameMiniMax:
		return proxy.ModelNameMiniMaxM27
	case proxy.ProviderNameSiliconFlow:
		return proxy.ModelNameSiliconFlowDeepSeek
	case proxy.ProviderNameZAI:
		return proxy.ModelNameZAIGLM
	case proxy.ProviderNameGemini:
		return proxy.ModelNameGemini25Flash
	case proxy.ProviderNameAnthropic:
		return proxy.ModelNameClaudeSonnet46
	case proxy.ProviderNameMeta:
		return proxy.ModelNameMuseSpark11
	case proxy.ProviderNameXAI:
		return proxy.ModelNameGrok43
	default:
		return ""
	}
}

type managedProviderVerificationDoer struct {
	next proxy.HTTPDoer
}

func (doer managedProviderVerificationDoer) Do(request *http.Request) (*http.Response, error) {
	if request.Body == nil {
		return doer.next.Do(request)
	}
	body, readError := io.ReadAll(request.Body)
	if readError != nil {
		return nil, readError
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	if !bytes.Contains(body, []byte("Verify this provider credential.")) {
		return doer.next.Do(request)
	}
	responseBody := `{"choices":[{}]}`
	switch {
	case request.Header.Get("x-goog-api-key") != "":
		responseBody = `{"status":"completed"}`
	case request.Header.Get("x-api-key") != "":
		responseBody = `{"id":"verification","type":"message","role":"assistant"}`
	case strings.HasSuffix(request.URL.Path, "/responses"):
		responseBody = `{"id":"verification","status":"completed"}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(responseBody)),
		Request:    request,
	}, nil
}
