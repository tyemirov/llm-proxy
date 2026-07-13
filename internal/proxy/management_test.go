package proxy_test

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/tauth/pkg/sessionvalidator"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	testManagementSigningKey               = "management-signing-key"
	testManagementTenantID                 = "llm-proxy-test"
	testManagementCookieName               = "llm_proxy_test_session"
	testManagementAdminEmail               = "admin@example.com"
	testManagementProviderKeyEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	testManagementOpenAIKey                = "sk-user-openai"
	testManagementDeepSeekKey              = "sk-user-deepseek"
	testManagementMetaKey                  = "sk-user-meta"
)

func managementProviderKeyRequestBody(t *testing.T, apiKey string, textModel string, systemPrompt string) string {
	t.Helper()
	requestBody, marshalError := json.Marshal(map[string]string{
		"api_key":       apiKey,
		"text_model":    textModel,
		"system_prompt": systemPrompt,
	})
	if marshalError != nil {
		t.Fatalf("marshal provider key request: %v", marshalError)
	}
	return string(requestBody)
}

func TestManagementStaticPagesAndUnauthenticatedAPI(t *testing.T) {
	staticServer := httptest.NewServer(http.FileServer(http.Dir("../../site")))
	defer staticServer.Close()

	staticIndexResponse, indexError := http.Get(staticServer.URL + "/")
	if indexError != nil {
		t.Fatalf("static index request: %v", indexError)
	}
	defer staticIndexResponse.Body.Close()
	if staticIndexResponse.StatusCode != http.StatusOK {
		t.Fatalf("static index status=%d want=%d", staticIndexResponse.StatusCode, http.StatusOK)
	}
	indexBytes, readIndexError := io.ReadAll(staticIndexResponse.Body)
	if readIndexError != nil {
		t.Fatalf("read static index: %v", readIndexError)
	}
	indexHTML := string(indexBytes)
	requiredFragments := []string{
		"mpr-ui-config.js",
		`data-mpr-ui-bundle-src="https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@v3.9.0/mpr-ui.js"`,
		`src="/assets/llm-proxy/js/app.js"`,
		`data-config-url="/config-ui.yaml"`,
		`<mpr-user`,
		`<mpr-footer`,
	}
	for _, requiredFragment := range requiredFragments {
		if !strings.Contains(indexHTML, requiredFragment) {
			t.Fatalf("static index missing %q", requiredFragment)
		}
	}
	forbiddenFragments := []string{"tauth.js", "tauth-login-path", "tauth-logout-path", "tauth-nonce-path", "{{MPR_UI_VERSION}}"}
	for _, forbiddenFragment := range forbiddenFragments {
		if strings.Contains(indexHTML, forbiddenFragment) {
			t.Fatalf("static index must not include %q", forbiddenFragment)
		}
	}

	router := newManagementRouter(t, proxy.Configuration{})

	indexRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	indexResponse := httptest.NewRecorder()
	router.ServeHTTP(indexResponse, indexRequest)
	if indexResponse.Code != http.StatusForbidden {
		t.Fatalf("backend index status=%d want=%d", indexResponse.Code, http.StatusForbidden)
	}

	configRequest := httptest.NewRequest(http.MethodGet, proxy.ManagementConfigUIPath, nil)
	configRequest.Header.Set("Origin", "http://localhost:8080")
	configResponse := httptest.NewRecorder()
	router.ServeHTTP(configResponse, configRequest)
	if configResponse.Code != http.StatusOK {
		t.Fatalf("backend config status=%d want=%d body=%s", configResponse.Code, http.StatusOK, configResponse.Body.String())
	}
	configBody := configResponse.Body.String()
	for _, requiredFragment := range []string{
		`llmProxy:`,
		`managementApiOrigin: "http://localhost:8080"`,
		`proxyOrigin: "http://localhost:8080"`,
		`description: "LLM Proxy"`,
		`- "http://localhost:8080"`,
		`tauthUrl: "http://localhost:8443"`,
		`googleClientId: "google-client-id"`,
		`tenantId: "llm-proxy-test"`,
		`loginPath: "/auth/google"`,
	} {
		if !strings.Contains(configBody, requiredFragment) {
			t.Fatalf("%s missing %q in %s", proxy.ManagementConfigUIFileName, requiredFragment, configBody)
		}
	}
	if configResponse.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8080" || configResponse.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("config headers=%v", configResponse.Header())
	}

	removedRuntimeConfigRequest := httptest.NewRequest(http.MethodGet, "/llm-proxy-config.json", nil)
	removedRuntimeConfigResponse := httptest.NewRecorder()
	router.ServeHTTP(removedRuntimeConfigResponse, removedRuntimeConfigRequest)
	if removedRuntimeConfigResponse.Code != http.StatusNotFound {
		t.Fatalf("removed runtime config status=%d want=%d", removedRuntimeConfigResponse.Code, http.StatusNotFound)
	}

	assetRequest := httptest.NewRequest(http.MethodGet, "/assets/llm-proxy/styles.css", nil)
	assetResponse := httptest.NewRecorder()
	router.ServeHTTP(assetResponse, assetRequest)
	if assetResponse.Code != http.StatusNotFound {
		t.Fatalf("backend asset status=%d want=%d", assetResponse.Code, http.StatusNotFound)
	}

	profileRequest := httptest.NewRequest(http.MethodGet, "/api/management/profile", nil)
	profileResponse := httptest.NewRecorder()
	router.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusUnauthorized {
		t.Fatalf("profile status=%d want=%d", profileResponse.Code, http.StatusUnauthorized)
	}

	corsProfileRequest := httptest.NewRequest(http.MethodGet, "/api/management/profile", nil)
	corsProfileRequest.Header.Set("Origin", "http://localhost:8080")
	corsProfileResponse := httptest.NewRecorder()
	router.ServeHTTP(corsProfileResponse, corsProfileRequest)
	if corsProfileResponse.Code != http.StatusUnauthorized {
		t.Fatalf("cors profile status=%d want=%d", corsProfileResponse.Code, http.StatusUnauthorized)
	}
	if corsProfileResponse.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8080" || corsProfileResponse.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("cors profile headers=%v", corsProfileResponse.Header())
	}

	preflightRequest := httptest.NewRequest(http.MethodOptions, "/api/management/profile", nil)
	preflightRequest.Header.Set("Origin", "http://localhost:8080")
	preflightRequest.Header.Set("Access-Control-Request-Method", http.MethodGet)
	preflightResponse := httptest.NewRecorder()
	router.ServeHTTP(preflightResponse, preflightRequest)
	if preflightResponse.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d want=%d body=%s", preflightResponse.Code, http.StatusNoContent, preflightResponse.Body.String())
	}
	if preflightResponse.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8080" || preflightResponse.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("preflight headers=%v", preflightResponse.Header())
	}

	disallowedPreflightRequest := httptest.NewRequest(http.MethodOptions, "/api/management/profile", nil)
	disallowedPreflightRequest.Header.Set("Origin", "https://other.example")
	disallowedPreflightResponse := httptest.NewRecorder()
	router.ServeHTTP(disallowedPreflightResponse, disallowedPreflightRequest)
	if disallowedPreflightResponse.Code != http.StatusForbidden {
		t.Fatalf("disallowed preflight status=%d want=%d", disallowedPreflightResponse.Code, http.StatusForbidden)
	}

	originSessionCookie := managementSessionCookie(t, "tauth-origin-user")
	disallowedMutationRequest := authenticatedJSONRequest(http.MethodPost, "/api/management/secrets", `{}`, originSessionCookie)
	disallowedMutationRequest.Header.Set("Origin", "https://other.example")
	disallowedMutationResponse := httptest.NewRecorder()
	router.ServeHTTP(disallowedMutationResponse, disallowedMutationRequest)
	if disallowedMutationResponse.Code != http.StatusForbidden {
		t.Fatalf("disallowed mutation status=%d want=%d", disallowedMutationResponse.Code, http.StatusForbidden)
	}

	missingContentTypeMutationRequest := httptest.NewRequest(http.MethodPost, "/api/management/secrets", strings.NewReader(""))
	missingContentTypeMutationRequest.AddCookie(originSessionCookie)
	missingContentTypeMutationResponse := httptest.NewRecorder()
	router.ServeHTTP(missingContentTypeMutationResponse, missingContentTypeMutationRequest)
	if missingContentTypeMutationResponse.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("missing content type mutation status=%d want=%d", missingContentTypeMutationResponse.Code, http.StatusUnsupportedMediaType)
	}

	simpleMutationRequest := httptest.NewRequest(http.MethodPost, "/api/management/secrets", strings.NewReader(""))
	simpleMutationRequest.Header.Set("Content-Type", "text/plain")
	simpleMutationRequest.AddCookie(originSessionCookie)
	simpleMutationResponse := httptest.NewRecorder()
	router.ServeHTTP(simpleMutationResponse, simpleMutationRequest)
	if simpleMutationResponse.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("simple mutation status=%d want=%d", simpleMutationResponse.Code, http.StatusUnsupportedMediaType)
	}

	allowedMutationRequest := authenticatedJSONRequest(http.MethodPost, "/api/management/secrets", `{}`, originSessionCookie)
	allowedMutationRequest.Header.Set("Origin", "http://localhost:8080")
	allowedMutationResponse := httptest.NewRecorder()
	router.ServeHTTP(allowedMutationResponse, allowedMutationRequest)
	if allowedMutationResponse.Code != http.StatusOK {
		t.Fatalf("allowed mutation status=%d want=%d body=%s", allowedMutationResponse.Code, http.StatusOK, allowedMutationResponse.Body.String())
	}
	if allowedMutationResponse.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8080" || allowedMutationResponse.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("allowed mutation headers=%v", allowedMutationResponse.Header())
	}

	missingSecretRequest := httptest.NewRequest(http.MethodGet, "/?key=", nil)
	missingSecretResponse := httptest.NewRecorder()
	router.ServeHTTP(missingSecretResponse, missingSecretRequest)
	if missingSecretResponse.Code != http.StatusForbidden {
		t.Fatalf("empty managed secret status=%d body=%s", missingSecretResponse.Code, missingSecretResponse.Body.String())
	}
}

func TestManagementRejectsInvalidSessionsAndRequests(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	invalidCookies := []*http.Cookie{
		{Name: testManagementCookieName, Value: "not-a-jwt"},
		managementSessionCookieWithoutExpiration(t),
		managementSessionCookieWithClaims(t, jwt.MapClaims{"iss": "tauth", "tenant_id": testManagementTenantID, "user_id": "expired-user", "exp": time.Now().UTC().Add(-time.Hour).Unix()}),
		managementSessionCookieWithClaims(t, jwt.MapClaims{"iss": "wrong", "tenant_id": testManagementTenantID, "user_id": "user"}),
		managementSessionCookieWithClaims(t, jwt.MapClaims{"iss": "tauth", "tenant_id": "wrong-tenant", "user_id": "user"}),
		managementSessionCookieWithClaims(t, jwt.MapClaims{"iss": "tauth", "tenant_id": testManagementTenantID, "user_id": "user", "iat": time.Now().UTC().Add(time.Hour).Unix()}),
		managementSessionCookieWithClaims(t, jwt.MapClaims{"iss": "tauth", "tenant_id": testManagementTenantID}),
	}
	for cookieIndex, invalidCookie := range invalidCookies {
		request := httptest.NewRequest(http.MethodGet, "/api/management/profile", nil)
		request.AddCookie(invalidCookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("invalid cookie %d status=%d want=%d body=%s", cookieIndex, response.Code, http.StatusUnauthorized, response.Body.String())
		}
	}

	sessionCookie := managementSessionCookie(t, "tauth-user-errors")
	badRequests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{method: http.MethodPut, path: "/api/management/provider-keys/unknown", body: managementProviderKeyRequestBody(t, "sk", proxy.ModelNameGPT41, ""), status: http.StatusBadRequest},
		{method: http.MethodPut, path: "/api/management/provider-keys/openai", body: managementProviderKeyRequestBody(t, "", proxy.ModelNameGPT41, ""), status: http.StatusBadRequest},
		{method: http.MethodPut, path: "/api/management/provider-keys/openai", body: `{"api_key":"sk","text_model":"gpt-4.1","system_prompt":"","extra":true}`, status: http.StatusBadRequest},
		{method: http.MethodPut, path: "/api/management/provider-keys/openai", body: `{"api_key":"sk","system_prompt":""}`, status: http.StatusBadRequest},
		{method: http.MethodPut, path: "/api/management/provider-keys/openai", body: managementProviderKeyRequestBody(t, "sk", "missing-model", ""), status: http.StatusBadRequest},
		{method: http.MethodDelete, path: "/api/management/provider-keys/unknown", body: `{}`, status: http.StatusBadRequest},
		{method: http.MethodPut, path: "/api/management/defaults", body: `{"provider":"openai","model":"gpt-4.1","extra":true}`, status: http.StatusBadRequest},
		{method: http.MethodPut, path: "/api/management/defaults", body: `{"provider":"openai","model":"gpt-4.1","dictation_provider":"","dictation_model":"","system_prompt":""}`, status: http.StatusBadRequest},
	}
	for _, badRequest := range badRequests {
		request := authenticatedJSONRequest(badRequest.method, badRequest.path, badRequest.body, sessionCookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != badRequest.status {
			t.Fatalf("%s %s status=%d want=%d body=%s", badRequest.method, badRequest.path, response.Code, badRequest.status, response.Body.String())
		}
	}

	saveRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/openai", managementProviderKeyRequestBody(t, "skhort", proxy.ModelNameGPT41, ""), sessionCookie)
	saveResponse := httptest.NewRecorder()
	router.ServeHTTP(saveResponse, saveRequest)
	if saveResponse.Code != http.StatusOK || !strings.Contains(saveResponse.Body.String(), `"masked_key":"saved"`) {
		t.Fatalf("save short key status=%d body=%s", saveResponse.Code, saveResponse.Body.String())
	}

	dictationDefaults := `{"provider":"openai","model":"gpt-4.1","dictation_provider":"deepseek","dictation_model":"deepseek-v4-flash","system_prompt":""}`
	dictationDefaultsRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/defaults", dictationDefaults, sessionCookie)
	dictationDefaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(dictationDefaultsResponse, dictationDefaultsRequest)
	if dictationDefaultsResponse.Code != http.StatusBadRequest {
		t.Fatalf("dictation defaults status=%d body=%s", dictationDefaultsResponse.Code, dictationDefaultsResponse.Body.String())
	}

	deepSeekOnlyCookie := managementSessionCookie(t, "tauth-deepseek-only")
	saveDeepSeekRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/deepseek", managementProviderKeyRequestBody(t, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""), deepSeekOnlyCookie)
	saveDeepSeekResponse := httptest.NewRecorder()
	router.ServeHTTP(saveDeepSeekResponse, saveDeepSeekRequest)
	if saveDeepSeekResponse.Code != http.StatusOK {
		t.Fatalf("save deepseek key status=%d body=%s", saveDeepSeekResponse.Code, saveDeepSeekResponse.Body.String())
	}
	blankDictationDefaults := `{"provider":"deepseek","model":"` + proxy.ModelNameDeepSeekV4Flash + `","dictation_provider":"","dictation_model":"","system_prompt":""}`
	blankDictationDefaultsRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/defaults", blankDictationDefaults, deepSeekOnlyCookie)
	blankDictationDefaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(blankDictationDefaultsResponse, blankDictationDefaultsRequest)
	if blankDictationDefaultsResponse.Code != http.StatusBadRequest {
		t.Fatalf("blank dictation defaults status=%d want=%d body=%s", blankDictationDefaultsResponse.Code, http.StatusBadRequest, blankDictationDefaultsResponse.Body.String())
	}

	removeRequest := authenticatedJSONRequest(http.MethodDelete, "/api/management/provider-keys/openai", `{}`, sessionCookie)
	removeResponse := httptest.NewRecorder()
	router.ServeHTTP(removeResponse, removeRequest)
	if removeResponse.Code != http.StatusOK || strings.Contains(removeResponse.Body.String(), `"has_key":true`) {
		t.Fatalf("remove status=%d body=%s", removeResponse.Code, removeResponse.Body.String())
	}
}

func TestManagementDatabasePersistenceAndOpenFailures(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "managed-tenants.db")
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"persisted ok"}}]}`))
	}))
	defer upstreamServer.Close()

	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{DeepSeekBaseURL: upstreamServer.URL}, databasePath)
	sessionCookie := managementSessionCookie(t, "tauth-persisted-user")
	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/deepseek", managementProviderKeyRequestBody(t, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""), sessionCookie)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}
	saveOpenAIKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/openai", managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT41, ""), sessionCookie)
	saveOpenAIKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveOpenAIKeyResponse, saveOpenAIKeyRequest)
	if saveOpenAIKeyResponse.Code != http.StatusOK {
		t.Fatalf("save openai key status=%d body=%s", saveOpenAIKeyResponse.Code, saveOpenAIKeyResponse.Body.String())
	}
	defaultsBody := `{"provider":"deepseek","model":"` + proxy.ModelNameDeepSeekV4Flash + `","dictation_provider":"openai","dictation_model":"` + proxy.DefaultDictationModel + `","system_prompt":""}`
	defaultsRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/defaults", defaultsBody, sessionCookie)
	defaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(defaultsResponse, defaultsRequest)
	if defaultsResponse.Code != http.StatusOK {
		t.Fatalf("defaults status=%d body=%s", defaultsResponse.Code, defaultsResponse.Body.String())
	}
	secretRequest := authenticatedJSONRequest(http.MethodPost, "/api/management/secrets", `{}`, sessionCookie)
	secretResponse := httptest.NewRecorder()
	router.ServeHTTP(secretResponse, secretRequest)
	if secretResponse.Code != http.StatusOK {
		t.Fatalf("secret status=%d body=%s", secretResponse.Code, secretResponse.Body.String())
	}
	var secretPayload struct {
		Secret string `json:"secret"`
	}
	if decodeError := json.Unmarshal(secretResponse.Body.Bytes(), &secretPayload); decodeError != nil {
		t.Fatalf("decode secret payload: %v", decodeError)
	}

	reloadedRouter := newManagementRouterWithDatabasePath(t, proxy.Configuration{DeepSeekBaseURL: upstreamServer.URL}, databasePath)
	reloadedRequest := httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secretPayload.Secret)+"&prompt=hello", nil)
	reloadedResponse := httptest.NewRecorder()
	reloadedRouter.ServeHTTP(reloadedResponse, reloadedRequest)
	if reloadedResponse.Code != http.StatusOK || strings.TrimSpace(reloadedResponse.Body.String()) != "persisted ok" {
		t.Fatalf("reloaded status=%d body=%s", reloadedResponse.Code, reloadedResponse.Body.String())
	}

	parentFile := filepath.Join(t.TempDir(), "parent-file")
	if writeError := os.WriteFile(parentFile, []byte("not a directory"), 0o600); writeError != nil {
		t.Fatalf("write parent file: %v", writeError)
	}
	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(parentFile, "store.db"))
	_, openError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if openError == nil {
		t.Fatalf("BuildRouter must reject not-a-directory database path")
	}
}

func TestManagementClaimsLegacyTokenForConfiguredAccount(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "managed-tenants.db")
	legacySecret := "legacy-config-secret"
	legacyDeepSeekKey := "sk-legacy-deepseek"
	legacyTenantID := "legacy"
	legacyOwnerUserID := "tauth-legacy-owner"
	legacyOwnerEmail := "legacy-owner@example.com"
	var capturedAuthorizations []string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		capturedAuthorizations = append(capturedAuthorizations, request.Header.Get("Authorization"))
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"legacy migrated ok"}}]}`))
	}))
	defer upstreamServer.Close()

	newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	seedLegacyManagedTenant(t, databasePath, legacyTenantID, legacySecret, legacyDeepSeekKey)
	unconfigured := managementConfigurationWithDatabasePath(proxy.Configuration{DeepSeekBaseURL: upstreamServer.URL}, databasePath)
	if _, unconfiguredError := buildRouterWithCatalogs(t, unconfigured, zap.NewNop().Sugar()); unconfiguredError == nil || !strings.Contains(unconfiguredError.Error(), "legacy_owner_config_missing") {
		t.Fatalf("unconfigured migration error=%v", unconfiguredError)
	}

	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{DeepSeekBaseURL: upstreamServer.URL}, databasePath)
	configuration.Management.LegacyTokenMigration = proxy.LegacyTokenMigrationConfiguration{
		TenantID:   legacyTenantID,
		OwnerEmail: legacyOwnerEmail,
	}
	router, buildError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	preClaimResponse := requestLegacyConfigSecret(t, router, legacySecret)
	if preClaimResponse.Code != http.StatusForbidden {
		t.Fatalf("pre-claim status=%d body=%s", preClaimResponse.Code, preClaimResponse.Body.String())
	}

	otherUserCookie := managementSessionCookieWithEmail(t, "other-user", "other@example.com")
	otherUsage := requestManagementUsage(t, router, otherUserCookie)
	if otherUsage.Totals.Requests != 0 {
		t.Fatalf("other user usage=%+v", otherUsage.Totals)
	}
	secondPreClaimResponse := requestLegacyConfigSecret(t, router, legacySecret)
	if secondPreClaimResponse.Code != http.StatusForbidden {
		t.Fatalf("non-owner claimed legacy token status=%d", secondPreClaimResponse.Code)
	}

	ownerCookie := managementSessionCookieWithEmail(t, legacyOwnerUserID, legacyOwnerEmail)
	profileRequest := httptest.NewRequest(http.MethodGet, "/api/management/profile", nil)
	profileRequest.AddCookie(ownerCookie)
	profileResponse := httptest.NewRecorder()
	router.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("owner profile status=%d body=%s", profileResponse.Code, profileResponse.Body.String())
	}
	var profilePayload struct {
		Tenant struct {
			ID        string `json:"id"`
			HasSecret bool   `json:"has_secret"`
		} `json:"tenant"`
	}
	if decodeError := json.Unmarshal(profileResponse.Body.Bytes(), &profilePayload); decodeError != nil {
		t.Fatalf("decode owner profile: %v", decodeError)
	}
	if profilePayload.Tenant.ID != legacyTenantID || !profilePayload.Tenant.HasSecret {
		t.Fatalf("owner profile tenant=%+v", profilePayload.Tenant)
	}

	historicalUsage := requestManagementUsage(t, router, ownerCookie)
	if historicalUsage.Totals.Requests != 1 || historicalUsage.Totals.TotalTokens != 7 {
		t.Fatalf("historical usage=%+v", historicalUsage.Totals)
	}

	legacyResponse := requestLegacyConfigSecret(t, router, legacySecret)
	if legacyResponse.Code != http.StatusOK || strings.TrimSpace(legacyResponse.Body.String()) != "legacy migrated ok" {
		t.Fatalf("claimed status=%d body=%s", legacyResponse.Code, legacyResponse.Body.String())
	}
	updatedUsage := requestManagementUsage(t, router, ownerCookie)
	if updatedUsage.Totals.Requests != 2 || updatedUsage.Totals.TotalTokens != 7 {
		t.Fatalf("updated usage=%+v", updatedUsage.Totals)
	}

	reloadedRouter, reloadError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if reloadError != nil {
		t.Fatalf("reload router: %v", reloadError)
	}
	reloadedUsage := requestManagementUsage(t, reloadedRouter, ownerCookie)
	if reloadedUsage.Totals.Requests != 2 {
		t.Fatalf("reloaded usage=%+v", reloadedUsage.Totals)
	}
	reloadedResponse := requestLegacyConfigSecret(t, reloadedRouter, legacySecret)
	if reloadedResponse.Code != http.StatusOK || strings.TrimSpace(reloadedResponse.Body.String()) != "legacy migrated ok" {
		t.Fatalf("reloaded status=%d body=%s", reloadedResponse.Code, reloadedResponse.Body.String())
	}

	legacyUserCount := countManagedTenantFixture(t, databasePath, "static-config:"+legacyTenantID)
	if legacyUserCount != 0 {
		t.Fatalf("legacy user count=%d", legacyUserCount)
	}
	if len(capturedAuthorizations) != 2 {
		t.Fatalf("captured authorizations=%v", capturedAuthorizations)
	}
	for authorizationIndex, authorization := range capturedAuthorizations {
		if authorization != "Bearer "+legacyDeepSeekKey {
			t.Fatalf("authorization %d=%q want=%q", authorizationIndex, authorization, "Bearer "+legacyDeepSeekKey)
		}
	}
}

func TestManagementLegacyTokenClaimRejectsExistingDestination(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "managed-tenants.db")
	legacyTenantID := "legacy-conflict"
	ownerUserID := "existing-owner"
	ownerEmail := "existing-owner@example.com"
	ownerCookie := managementSessionCookieWithEmail(t, ownerUserID, ownerEmail)

	initialRouter := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	profileRequest := httptest.NewRequest(http.MethodGet, "/api/management/profile", nil)
	profileRequest.AddCookie(ownerCookie)
	profileResponse := httptest.NewRecorder()
	initialRouter.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("initial profile status=%d body=%s", profileResponse.Code, profileResponse.Body.String())
	}
	seedLegacyManagedTenant(t, databasePath, legacyTenantID, "legacy-conflict-secret", "sk-conflict")

	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath)
	configuration.Management.LegacyTokenMigration = proxy.LegacyTokenMigrationConfiguration{TenantID: legacyTenantID, OwnerEmail: ownerEmail}
	router, buildError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}
	conflictRequest := httptest.NewRequest(http.MethodGet, "/api/management/profile", nil)
	conflictRequest.AddCookie(ownerCookie)
	conflictResponse := httptest.NewRecorder()
	router.ServeHTTP(conflictResponse, conflictRequest)
	if conflictResponse.Code != http.StatusConflict || !strings.Contains(conflictResponse.Body.String(), "managed_legacy_token_migration_conflict") {
		t.Fatalf("conflict status=%d body=%s", conflictResponse.Code, conflictResponse.Body.String())
	}
	if countManagedTenantFixture(t, databasePath, "static-config:"+legacyTenantID) != 1 || countManagedTenantFixture(t, databasePath, ownerUserID) != 1 {
		t.Fatalf("conflict changed source or destination")
	}
}

func TestManagementRejectsStaticCredentialModel(t *testing.T) {
	testCases := []struct {
		name          string
		configuration proxy.Configuration
		expectedField string
	}{
		{
			name:          "static tenant",
			configuration: proxy.Configuration{Tenants: proxy.SingleTenantConfigurations("legacy", "legacy-secret")},
			expectedField: "field=tenants",
		},
		{
			name:          "static provider key",
			configuration: proxy.Configuration{OpenAIKey: "sk-global"},
			expectedField: "field=providers.api_key",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			configuration := managementConfigurationWithDatabasePath(testCase.configuration, filepath.Join(subTest.TempDir(), "managed-tenants.db"))
			_, buildError := buildRouterWithCatalogs(subTest, configuration, zap.NewNop().Sugar())
			if buildError == nil || !strings.Contains(buildError.Error(), testCase.expectedField) {
				subTest.Fatalf("error=%v want contains %q", buildError, testCase.expectedField)
			}
		})
	}
}

func TestManagementConfigurationValidationRequiresBackendAuthFields(t *testing.T) {
	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "store.db"))
	configuration.Management.TAuthTenantID = " "
	_, buildError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.tauth_tenant_id") {
		t.Fatalf("BuildRouter error=%v want missing management.tauth_tenant_id", buildError)
	}

	configuration = managementConfigurationWithDatabasePath(proxy.Configuration{}, "")
	_, buildError = buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.database_dsn") {
		t.Fatalf("BuildRouter error=%v want missing management.database_dsn", buildError)
	}

	configuration = managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "store.db"))
	configuration.Management.UIOrigins = nil
	_, buildError = buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.ui_origins") {
		t.Fatalf("BuildRouter error=%v want missing management.ui_origins", buildError)
	}

	configuration = managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "store.db"))
	configuration.Management.UIOrigins = []string{"http://localhost:8080", " "}
	_, buildError = buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.ui_origins") {
		t.Fatalf("BuildRouter error=%v want blank management.ui_origins", buildError)
	}

	configuration = managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "store.db"))
	configuration.Management.DatabaseDialect = "mysql"
	_, buildError = buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.database_dialect") {
		t.Fatalf("BuildRouter error=%v want unsupported management.database_dialect", buildError)
	}

	configuration = managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "store.db"))
	configuration.Management.DatabaseDialect = " "
	_, buildError = buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.database_dialect") {
		t.Fatalf("BuildRouter error=%v want missing management.database_dialect", buildError)
	}

	configuration = managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "store.db"))
	configuration.Management.AdminEmails = []string{"not an email"}
	_, buildError = buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.admin_emails") {
		t.Fatalf("BuildRouter error=%v want invalid management.admin_emails", buildError)
	}

	configuration = managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "store.db"))
	configuration.Management.ProviderKeyEncryptionKey = " "
	_, buildError = buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.provider_key_encryption_key") {
		t.Fatalf("BuildRouter error=%v want missing management.provider_key_encryption_key", buildError)
	}

	configuration = managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "store.db"))
	configuration.Management.ProviderKeyEncryptionKey = "not-base64"
	_, buildError = buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.provider_key_encryption_key") {
		t.Fatalf("BuildRouter error=%v want invalid management.provider_key_encryption_key", buildError)
	}

	configuration = managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "store.db"))
	configuration.Management.LegacyTokenMigration = proxy.LegacyTokenMigrationConfiguration{TenantID: "legacy"}
	_, buildError = buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.legacy_token_migration.owner_email") {
		t.Fatalf("BuildRouter error=%v want missing legacy migration owner email", buildError)
	}

	configuration = managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "store.db"))
	configuration.Management.LegacyTokenMigration = proxy.LegacyTokenMigrationConfiguration{OwnerEmail: "owner@example.com"}
	_, buildError = buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.legacy_token_migration.tenant_id") {
		t.Fatalf("BuildRouter error=%v want missing legacy migration tenant id", buildError)
	}

	configuration = managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "store.db"))
	configuration.Management.LegacyTokenMigration = proxy.LegacyTokenMigrationConfiguration{TenantID: "legacy", OwnerEmail: "not an email"}
	_, buildError = buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.legacy_token_migration.owner_email") {
		t.Fatalf("BuildRouter error=%v want invalid legacy migration owner email", buildError)
	}

	disabledManagementConfiguration := proxy.Configuration{
		Tenants: proxy.SingleTenantConfigurations("static", "secret"),
		Management: proxy.ManagementConfiguration{
			LegacyTokenMigration: proxy.LegacyTokenMigrationConfiguration{TenantID: "legacy", OwnerEmail: "owner@example.com"},
		},
	}
	_, buildError = newConfigurationWithCatalogs(t, disabledManagementConfiguration)
	if buildError == nil || !strings.Contains(buildError.Error(), "management.legacy_token_migration requires_management") {
		t.Fatalf("NewConfiguration error=%v want disabled legacy migration rejection", buildError)
	}
}

func TestManagementSQLiteDialectOpensConfiguredDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "managed-tenants.db")
	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath)
	configuration.Management.DatabaseDialect = proxy.ManagementDatabaseDialectSQLite
	configuration.Management.DatabaseDSN = databasePath
	configuration.Management.DatabaseDialector = nil
	router, buildError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	profileRequest := authenticatedJSONRequest(http.MethodGet, "/api/management/profile", "", managementSessionCookie(t, "tauth-sqlite-user"))
	profileResponse := httptest.NewRecorder()
	router.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%s", profileResponse.Code, profileResponse.Body.String())
	}
}

func TestManagementGeneratedSecretSupportsDictationAndRejectsMultipartProviderKeys(t *testing.T) {
	var capturedAuthorization string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		capturedAuthorization = request.Header.Get("Authorization")
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"text":"managed dictation ok"}`))
	}))
	defer upstreamServer.Close()

	router := newManagementRouter(t, proxy.Configuration{
		OpenAITranscriptionsURL: upstreamServer.URL,
	})
	sessionCookie := managementSessionCookie(t, "tauth-dictation-user")
	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/openai", managementProviderKeyRequestBody(t, "sk-user-openai", proxy.ModelNameGPT41, ""), sessionCookie)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}
	secretRequest := authenticatedJSONRequest(http.MethodPost, "/api/management/secrets", `{}`, sessionCookie)
	secretResponse := httptest.NewRecorder()
	router.ServeHTTP(secretResponse, secretRequest)
	if secretResponse.Code != http.StatusOK {
		t.Fatalf("secret status=%d body=%s", secretResponse.Code, secretResponse.Body.String())
	}
	var secretPayload struct {
		Secret string `json:"secret"`
	}
	if decodeError := json.Unmarshal(secretResponse.Body.Bytes(), &secretPayload); decodeError != nil {
		t.Fatalf("decode secret: %v", decodeError)
	}

	for _, includeProviderKeyField := range []bool{true, false} {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		if includeProviderKeyField {
			if writeError := writer.WriteField("openai_api_key", "sk-client"); writeError != nil {
				t.Fatalf("write provider key field: %v", writeError)
			}
		}
		filePart, createError := writer.CreateFormFile("audio", "recording.webm")
		if createError != nil {
			t.Fatalf("CreateFormFile error: %v", createError)
		}
		if _, writeError := filePart.Write([]byte("audio")); writeError != nil {
			t.Fatalf("write audio: %v", writeError)
		}
		if closeError := writer.Close(); closeError != nil {
			t.Fatalf("close multipart: %v", closeError)
		}
		request := httptest.NewRequest(http.MethodPost, "/dictate?key="+url.QueryEscape(secretPayload.Secret), body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if includeProviderKeyField {
			if response.Code != http.StatusBadRequest {
				t.Fatalf("multipart provider key status=%d body=%s", response.Code, response.Body.String())
			}
			continue
		}
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "managed dictation ok") {
			t.Fatalf("dictation status=%d body=%s", response.Code, response.Body.String())
		}
	}
	if capturedAuthorization != "Bearer sk-user-openai" {
		t.Fatalf("authorization=%q want=%q", capturedAuthorization, "Bearer sk-user-openai")
	}
}

func TestManagementUsageSummaryRecordsManagedProxyRequests(t *testing.T) {
	chatServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			t.Fatalf("chat path=%s want=/chat/completions", request.URL.Path)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"managed usage ok"}}],"usage":{"prompt_tokens":4,"completion_tokens":6,"total_tokens":10}}`))
	}))
	defer chatServer.Close()
	dictationServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		http.Error(responseWriter, "dictation unavailable", http.StatusBadGateway)
	}))
	defer dictationServer.Close()

	router := newManagementRouter(t, proxy.Configuration{
		DeepSeekBaseURL:         chatServer.URL,
		OpenAITranscriptionsURL: dictationServer.URL,
	})
	userOneCookie := managementSessionCookie(t, "usage-user-one")
	userTwoCookie := managementSessionCookie(t, "usage-user-two")

	emptyUsage := requestManagementUsage(t, router, userOneCookie)
	if emptyUsage.PeriodDays != 30 || len(emptyUsage.Daily) != 30 || emptyUsage.Totals.Requests != 0 {
		t.Fatalf("empty usage=%+v daily=%d", emptyUsage.Totals, len(emptyUsage.Daily))
	}

	saveDeepSeekKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/deepseek", managementProviderKeyRequestBody(t, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""), userOneCookie)
	saveDeepSeekKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveDeepSeekKeyResponse, saveDeepSeekKeyRequest)
	if saveDeepSeekKeyResponse.Code != http.StatusOK {
		t.Fatalf("save deepseek key status=%d body=%s", saveDeepSeekKeyResponse.Code, saveDeepSeekKeyResponse.Body.String())
	}
	saveOpenAIKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/openai", managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT41, ""), userOneCookie)
	saveOpenAIKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveOpenAIKeyResponse, saveOpenAIKeyRequest)
	if saveOpenAIKeyResponse.Code != http.StatusOK {
		t.Fatalf("save openai key status=%d body=%s", saveOpenAIKeyResponse.Code, saveOpenAIKeyResponse.Body.String())
	}
	defaultsBody := `{"provider":"deepseek","model":"` + proxy.ModelNameDeepSeekV4Flash + `","dictation_provider":"openai","dictation_model":"` + proxy.DefaultDictationModel + `","system_prompt":""}`
	defaultsRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/defaults", defaultsBody, userOneCookie)
	defaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(defaultsResponse, defaultsRequest)
	if defaultsResponse.Code != http.StatusOK {
		t.Fatalf("defaults status=%d body=%s", defaultsResponse.Code, defaultsResponse.Body.String())
	}
	secretRequest := authenticatedJSONRequest(http.MethodPost, "/api/management/secrets", `{}`, userOneCookie)
	secretResponse := httptest.NewRecorder()
	router.ServeHTTP(secretResponse, secretRequest)
	if secretResponse.Code != http.StatusOK {
		t.Fatalf("secret status=%d body=%s", secretResponse.Code, secretResponse.Body.String())
	}
	var secretPayload struct {
		Secret string `json:"secret"`
	}
	if decodeError := json.Unmarshal(secretResponse.Body.Bytes(), &secretPayload); decodeError != nil {
		t.Fatalf("decode secret: %v", decodeError)
	}

	textRequest := httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secretPayload.Secret)+"&prompt=hello", nil)
	textResponse := httptest.NewRecorder()
	router.ServeHTTP(textResponse, textRequest)
	if textResponse.Code != http.StatusOK || strings.TrimSpace(textResponse.Body.String()) != "managed usage ok" {
		t.Fatalf("text status=%d body=%s", textResponse.Code, textResponse.Body.String())
	}

	audioBody := &bytes.Buffer{}
	audioWriter := multipart.NewWriter(audioBody)
	filePart, createError := audioWriter.CreateFormFile("audio", "recording.webm")
	if createError != nil {
		t.Fatalf("CreateFormFile error: %v", createError)
	}
	if _, writeError := filePart.Write([]byte("audio")); writeError != nil {
		t.Fatalf("write audio: %v", writeError)
	}
	if closeError := audioWriter.Close(); closeError != nil {
		t.Fatalf("close multipart: %v", closeError)
	}
	dictationRequest := httptest.NewRequest(http.MethodPost, "/dictate?key="+url.QueryEscape(secretPayload.Secret), audioBody)
	dictationRequest.Header.Set("Content-Type", audioWriter.FormDataContentType())
	dictationResponse := httptest.NewRecorder()
	router.ServeHTTP(dictationResponse, dictationRequest)
	if dictationResponse.Code != http.StatusBadGateway {
		t.Fatalf("dictation status=%d body=%s", dictationResponse.Code, dictationResponse.Body.String())
	}

	invalidTextRequest := httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secretPayload.Secret)+"&prompt=hello&max_tokens=0", nil)
	invalidTextResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidTextResponse, invalidTextRequest)
	if invalidTextResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid text status=%d body=%s", invalidTextResponse.Code, invalidTextResponse.Body.String())
	}

	invalidV2Request := httptest.NewRequest(http.MethodPost, "/v2?key="+url.QueryEscape(secretPayload.Secret), bytes.NewBufferString(`{"messages":[{"role":"user","content":"hello"}],"max_tokens":0}`))
	invalidV2Request.Header.Set("Content-Type", "application/json")
	invalidV2Response := httptest.NewRecorder()
	router.ServeHTTP(invalidV2Response, invalidV2Request)
	if invalidV2Response.Code != http.StatusBadRequest {
		t.Fatalf("invalid v2 status=%d body=%s", invalidV2Response.Code, invalidV2Response.Body.String())
	}

	invalidDictationBody := &bytes.Buffer{}
	invalidDictationWriter := multipart.NewWriter(invalidDictationBody)
	if closeError := invalidDictationWriter.Close(); closeError != nil {
		t.Fatalf("close invalid multipart: %v", closeError)
	}
	invalidDictationRequest := httptest.NewRequest(http.MethodPost, "/dictate?key="+url.QueryEscape(secretPayload.Secret), invalidDictationBody)
	invalidDictationRequest.Header.Set("Content-Type", invalidDictationWriter.FormDataContentType())
	invalidDictationResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidDictationResponse, invalidDictationRequest)
	if invalidDictationResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid dictation status=%d body=%s", invalidDictationResponse.Code, invalidDictationResponse.Body.String())
	}

	usage := requestManagementUsage(t, router, userOneCookie)
	if usage.Totals.Requests != 5 || usage.Totals.SuccessfulRequests != 1 || usage.Totals.FailedRequests != 4 {
		t.Fatalf("usage totals=%+v", usage.Totals)
	}
	if usage.Totals.TextRequests != 3 || usage.Totals.DictationRequests != 2 || usage.Totals.RequestTokens != 4 || usage.Totals.ResponseTokens != 6 || usage.Totals.TotalTokens != 10 {
		t.Fatalf("usage totals=%+v", usage.Totals)
	}
	if len(usage.Providers) != 2 || usage.Providers[0].Provider != proxy.ProviderNameDeepSeek || usage.Providers[0].Data.Requests != 3 || usage.Providers[1].Provider != proxy.ProviderNameOpenAI || usage.Providers[1].Data.Requests != 2 {
		t.Fatalf("providers=%+v", usage.Providers)
	}
	if len(usage.StatusCodes) != 3 || usage.StatusCodes[0].StatusCode != http.StatusOK || usage.StatusCodes[0].Requests != 1 || usage.StatusCodes[1].StatusCode != http.StatusBadRequest || usage.StatusCodes[1].Requests != 3 || usage.StatusCodes[2].StatusCode != http.StatusBadGateway || usage.StatusCodes[2].Requests != 1 {
		t.Fatalf("status codes=%+v", usage.StatusCodes)
	}
	if isolatedUsage := requestManagementUsage(t, router, userTwoCookie); isolatedUsage.Totals.Requests != 0 {
		t.Fatalf("user two usage leaked: %+v", isolatedUsage.Totals)
	}
}

func TestManagementAdminUsersDashboard(t *testing.T) {
	chatServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"admin usage ok"}}],"usage":{"prompt_tokens":7,"completion_tokens":11,"total_tokens":18}}`))
	}))
	defer chatServer.Close()

	router := newManagementRouter(t, proxy.Configuration{DeepSeekBaseURL: chatServer.URL})
	userOneCookie := managementSessionCookie(t, "admin-visible-user-one")
	userTwoCookie := managementSessionCookie(t, "admin-visible-user-two")
	adminCookie := managementSessionCookieWithEmail(t, "admin-user", testManagementAdminEmail)

	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/deepseek", managementProviderKeyRequestBody(t, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""), userOneCookie)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}
	saveOpenAIKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/openai", managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT41, ""), userOneCookie)
	saveOpenAIKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveOpenAIKeyResponse, saveOpenAIKeyRequest)
	if saveOpenAIKeyResponse.Code != http.StatusOK {
		t.Fatalf("save openai key status=%d body=%s", saveOpenAIKeyResponse.Code, saveOpenAIKeyResponse.Body.String())
	}
	defaultsBody := `{"provider":"deepseek","model":"` + proxy.ModelNameDeepSeekV4Flash + `","dictation_provider":"openai","dictation_model":"` + proxy.DefaultDictationModel + `","system_prompt":""}`
	defaultsRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/defaults", defaultsBody, userOneCookie)
	defaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(defaultsResponse, defaultsRequest)
	if defaultsResponse.Code != http.StatusOK {
		t.Fatalf("defaults status=%d body=%s", defaultsResponse.Code, defaultsResponse.Body.String())
	}
	secretRequest := authenticatedJSONRequest(http.MethodPost, "/api/management/secrets", `{}`, userOneCookie)
	secretResponse := httptest.NewRecorder()
	router.ServeHTTP(secretResponse, secretRequest)
	if secretResponse.Code != http.StatusOK {
		t.Fatalf("secret status=%d body=%s", secretResponse.Code, secretResponse.Body.String())
	}
	var secretPayload struct {
		Secret string `json:"secret"`
	}
	if decodeError := json.Unmarshal(secretResponse.Body.Bytes(), &secretPayload); decodeError != nil {
		t.Fatalf("decode secret: %v", decodeError)
	}

	textRequest := httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secretPayload.Secret)+"&prompt=hello", nil)
	textResponse := httptest.NewRecorder()
	router.ServeHTTP(textResponse, textRequest)
	if textResponse.Code != http.StatusOK {
		t.Fatalf("text status=%d body=%s", textResponse.Code, textResponse.Body.String())
	}

	profileRequest := authenticatedJSONRequest(http.MethodGet, "/api/management/profile", "", userTwoCookie)
	profileResponse := httptest.NewRecorder()
	router.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%s", profileResponse.Code, profileResponse.Body.String())
	}

	forbiddenRequest := authenticatedJSONRequest(http.MethodGet, "/api/management/admin/users", "", userOneCookie)
	forbiddenResponse := httptest.NewRecorder()
	router.ServeHTTP(forbiddenResponse, forbiddenRequest)
	if forbiddenResponse.Code != http.StatusForbidden {
		t.Fatalf("admin users non-admin status=%d want=%d body=%s", forbiddenResponse.Code, http.StatusForbidden, forbiddenResponse.Body.String())
	}

	adminRequest := authenticatedJSONRequest(http.MethodGet, "/api/management/admin/users", "", adminCookie)
	adminResponse := httptest.NewRecorder()
	router.ServeHTTP(adminResponse, adminRequest)
	if adminResponse.Code != http.StatusOK {
		t.Fatalf("admin users status=%d body=%s", adminResponse.Code, adminResponse.Body.String())
	}
	adminBody := adminResponse.Body.String()
	forbiddenFragments := []string{testManagementDeepSeekKey, secretPayload.Secret, "masked_key", "SecretDigest"}
	for _, forbiddenFragment := range forbiddenFragments {
		if strings.Contains(adminBody, forbiddenFragment) {
			t.Fatalf("admin response leaked %q: %s", forbiddenFragment, adminBody)
		}
	}
	var adminUsers managementAdminUsersTestResponse
	if decodeError := json.Unmarshal(adminResponse.Body.Bytes(), &adminUsers); decodeError != nil {
		t.Fatalf("decode admin users: %v", decodeError)
	}
	if adminUsers.PeriodDays != 30 || len(adminUsers.Users) != 2 {
		t.Fatalf("admin users=%+v", adminUsers)
	}
	userUsageByID := map[string]int{}
	for _, user := range adminUsers.Users {
		userUsageByID[user.User.ID] = user.Usage.Totals.Requests
		if user.Tenant.ID == "" || user.User.Email == "" {
			t.Fatalf("admin user missing tenant/email: %+v", user)
		}
	}
	if userUsageByID["admin-visible-user-one"] != 1 || userUsageByID["admin-visible-user-two"] != 0 {
		t.Fatalf("admin usage by user=%+v", userUsageByID)
	}
}

func TestManagementMetaProviderRoutesWithEncryptedTenantKey(t *testing.T) {
	var capturedAuthorization string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			t.Fatalf("path=%s want=/chat/completions", request.URL.Path)
		}
		capturedAuthorization = request.Header.Get("Authorization")
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read upstream body: %v", readError)
		}
		var upstreamPayload map[string]any
		if unmarshalError := json.Unmarshal(bodyBytes, &upstreamPayload); unmarshalError != nil {
			t.Fatalf("unmarshal upstream body: %v", unmarshalError)
		}
		if upstreamPayload["model"] != proxy.ModelNameMuseSpark11 {
			t.Fatalf("model=%v want=%s", upstreamPayload["model"], proxy.ModelNameMuseSpark11)
		}
		messages, messagesOK := upstreamPayload["messages"].([]any)
		if !messagesOK || len(messages) != 2 {
			t.Fatalf("messages=%+v", upstreamPayload["messages"])
		}
		systemMessage, systemMessageOK := messages[0].(map[string]any)
		if !systemMessageOK || systemMessage["role"] != "system" || systemMessage["content"] != "meta managed system" {
			t.Fatalf("system message=%+v", messages[0])
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"managed meta ok"}}]}`))
	}))
	defer upstreamServer.Close()

	router := newManagementRouter(t, proxy.Configuration{
		MetaBaseURL: upstreamServer.URL,
	})
	userOneCookie := managementSessionCookie(t, "tauth-user-one")
	userTwoCookie := managementSessionCookie(t, "tauth-user-two")

	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/meta", managementProviderKeyRequestBody(t, testManagementMetaKey, proxy.ModelNameMuseSpark11, "meta managed system"), userOneCookie)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}
	if strings.Contains(saveKeyResponse.Body.String(), testManagementMetaKey) || !strings.Contains(saveKeyResponse.Body.String(), "sk-...meta") {
		t.Fatalf("provider key response leaked or failed to mask key: %s", saveKeyResponse.Body.String())
	}
	for _, expectedFragment := range []string{`"id":"meta"`, `"label":"Meta"`, `"text_model":"muse-spark-1.1"`, `"text_default_model":"muse-spark-1.1"`, `"supports_dictation":false`} {
		if !strings.Contains(saveKeyResponse.Body.String(), expectedFragment) {
			t.Fatalf("provider key response missing %q: %s", expectedFragment, saveKeyResponse.Body.String())
		}
	}

	saveOpenAIKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/openai", managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT41, ""), userOneCookie)
	saveOpenAIKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveOpenAIKeyResponse, saveOpenAIKeyRequest)
	if saveOpenAIKeyResponse.Code != http.StatusOK {
		t.Fatalf("save openai key status=%d body=%s", saveOpenAIKeyResponse.Code, saveOpenAIKeyResponse.Body.String())
	}

	defaultsBody := `{"provider":"meta","model":"` + proxy.ModelNameMuseSpark11 + `","dictation_provider":"openai","dictation_model":"` + proxy.DefaultDictationModel + `","system_prompt":""}`
	defaultsRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/defaults", defaultsBody, userOneCookie)
	defaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(defaultsResponse, defaultsRequest)
	if defaultsResponse.Code != http.StatusOK {
		t.Fatalf("defaults status=%d body=%s", defaultsResponse.Code, defaultsResponse.Body.String())
	}

	userTwoProfileRequest := httptest.NewRequest(http.MethodGet, "/api/management/profile", nil)
	userTwoProfileRequest.AddCookie(userTwoCookie)
	userTwoProfileResponse := httptest.NewRecorder()
	router.ServeHTTP(userTwoProfileResponse, userTwoProfileRequest)
	if userTwoProfileResponse.Code != http.StatusOK {
		t.Fatalf("user2 profile status=%d body=%s", userTwoProfileResponse.Code, userTwoProfileResponse.Body.String())
	}
	if strings.Contains(userTwoProfileResponse.Body.String(), "sk-...meta") {
		t.Fatalf("user2 saw user1 provider key: %s", userTwoProfileResponse.Body.String())
	}

	secretRequest := authenticatedJSONRequest(http.MethodPost, "/api/management/secrets", `{}`, userOneCookie)
	secretResponseRecorder := httptest.NewRecorder()
	router.ServeHTTP(secretResponseRecorder, secretRequest)
	if secretResponseRecorder.Code != http.StatusOK {
		t.Fatalf("secret status=%d body=%s", secretResponseRecorder.Code, secretResponseRecorder.Body.String())
	}
	var secretResponse struct {
		Secret string `json:"secret"`
	}
	if decodeError := json.Unmarshal(secretResponseRecorder.Body.Bytes(), &secretResponse); decodeError != nil {
		t.Fatalf("decode secret response: %v", decodeError)
	}
	if !strings.HasPrefix(secretResponse.Secret, "llmp_") {
		t.Fatalf("secret=%q", secretResponse.Secret)
	}

	proxyRequestValues := url.Values{}
	proxyRequestValues.Set("key", secretResponse.Secret)
	proxyRequestValues.Set("prompt", "hello")
	proxyRequestValues.Set("provider", proxy.ProviderNameMeta)
	proxyRequest := httptest.NewRequest(http.MethodGet, "/?"+proxyRequestValues.Encode(), nil)
	proxyResponse := httptest.NewRecorder()
	router.ServeHTTP(proxyResponse, proxyRequest)
	if proxyResponse.Code != http.StatusOK || strings.TrimSpace(proxyResponse.Body.String()) != "managed meta ok" {
		t.Fatalf("proxy status=%d body=%q", proxyResponse.Code, proxyResponse.Body.String())
	}
	if capturedAuthorization != "Bearer "+testManagementMetaKey {
		t.Fatalf("authorization=%q want=%q", capturedAuthorization, "Bearer "+testManagementMetaKey)
	}

	revokeRequest := authenticatedJSONRequest(http.MethodDelete, "/api/management/secrets", `{}`, userOneCookie)
	revokeResponse := httptest.NewRecorder()
	router.ServeHTTP(revokeResponse, revokeRequest)
	if revokeResponse.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", revokeResponse.Code, revokeResponse.Body.String())
	}
	revokedProxyRequest := httptest.NewRequest(http.MethodGet, "/?"+proxyRequestValues.Encode(), nil)
	revokedProxyResponse := httptest.NewRecorder()
	router.ServeHTTP(revokedProxyResponse, revokedProxyRequest)
	if revokedProxyResponse.Code != http.StatusForbidden {
		t.Fatalf("revoked status=%d want=%d", revokedProxyResponse.Code, http.StatusForbidden)
	}
}

func TestManagementGeneratedSecretOmittedProviderUsesTenantDefaults(t *testing.T) {
	var capturedModels []string
	var capturedInputs []string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" {
			t.Fatalf("path=%s want=/responses", request.URL.Path)
		}
		if authorizationHeader := request.Header.Get("Authorization"); authorizationHeader != "Bearer "+testManagementOpenAIKey {
			t.Fatalf("authorization=%q want=%q", authorizationHeader, "Bearer "+testManagementOpenAIKey)
		}
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read upstream body: %v", readError)
		}
		var upstreamPayload map[string]any
		if unmarshalError := json.Unmarshal(bodyBytes, &upstreamPayload); unmarshalError != nil {
			t.Fatalf("unmarshal upstream body: %v", unmarshalError)
		}
		model, modelOK := upstreamPayload["model"].(string)
		input, inputOK := upstreamPayload["input"].(string)
		if !modelOK || !inputOK {
			t.Fatalf("upstream payload=%+v", upstreamPayload)
		}
		capturedModels = append(capturedModels, model)
		capturedInputs = append(capturedInputs, input)
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"id":"response-id","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"text","text":"managed openai ok"}]}]}`))
	}))
	defer upstreamServer.Close()

	router := newManagementRouter(t, proxy.Configuration{
		OpenAIBaseURL: upstreamServer.URL,
	})
	userCookie := managementSessionCookie(t, "tauth-openai-defaults-user")
	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/openai", managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT55, "provider-owned system"), userCookie)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}

	defaultsBody := `{"provider":"openai","model":"` + proxy.ModelNameGPT41 + `","dictation_provider":"openai","dictation_model":"` + proxy.DefaultDictationModel + `","system_prompt":"tenant default system"}`
	defaultsRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/defaults", defaultsBody, userCookie)
	defaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(defaultsResponse, defaultsRequest)
	if defaultsResponse.Code != http.StatusOK {
		t.Fatalf("defaults status=%d body=%s", defaultsResponse.Code, defaultsResponse.Body.String())
	}

	secretRequest := authenticatedJSONRequest(http.MethodPost, "/api/management/secrets", `{}`, userCookie)
	secretResponseRecorder := httptest.NewRecorder()
	router.ServeHTTP(secretResponseRecorder, secretRequest)
	if secretResponseRecorder.Code != http.StatusOK {
		t.Fatalf("secret status=%d body=%s", secretResponseRecorder.Code, secretResponseRecorder.Body.String())
	}
	var secretResponse struct {
		Secret string `json:"secret"`
	}
	if decodeError := json.Unmarshal(secretResponseRecorder.Body.Bytes(), &secretResponse); decodeError != nil {
		t.Fatalf("decode secret response: %v", decodeError)
	}

	omittedQuery := url.Values{}
	omittedQuery.Set("key", secretResponse.Secret)
	omittedQuery.Set("prompt", "hello omitted")
	omittedResponse := httptest.NewRecorder()
	router.ServeHTTP(omittedResponse, httptest.NewRequest(http.MethodGet, "/?"+omittedQuery.Encode(), nil))
	if omittedResponse.Code != http.StatusOK {
		t.Fatalf("omitted status=%d body=%s", omittedResponse.Code, omittedResponse.Body.String())
	}

	explicitQuery := url.Values{}
	explicitQuery.Set("key", secretResponse.Secret)
	explicitQuery.Set("prompt", "hello explicit")
	explicitQuery.Set("provider", proxy.ProviderNameOpenAI)
	explicitResponse := httptest.NewRecorder()
	router.ServeHTTP(explicitResponse, httptest.NewRequest(http.MethodGet, "/?"+explicitQuery.Encode(), nil))
	if explicitResponse.Code != http.StatusOK {
		t.Fatalf("explicit status=%d body=%s", explicitResponse.Code, explicitResponse.Body.String())
	}

	if len(capturedModels) != 2 || len(capturedInputs) != 2 {
		t.Fatalf("captured models=%v inputs=%v", capturedModels, capturedInputs)
	}
	if capturedModels[0] != proxy.ModelNameGPT41 || capturedInputs[0] != "tenant default system\n\nhello omitted" {
		t.Fatalf("omitted model/input=%q/%q", capturedModels[0], capturedInputs[0])
	}
	if capturedModels[1] != proxy.ModelNameGPT55 || capturedInputs[1] != "provider-owned system\n\nhello explicit" {
		t.Fatalf("explicit model/input=%q/%q", capturedModels[1], capturedInputs[1])
	}
}

func TestProxyRejectsClientSuppliedProviderKeys(t *testing.T) {
	router := NewTestRouter(t, "https://upstream.invalid")

	queryRequest := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&api_key=sk-client", nil)
	queryResponse := httptest.NewRecorder()
	router.ServeHTTP(queryResponse, queryRequest)
	if queryResponse.Code != http.StatusBadRequest || strings.TrimSpace(queryResponse.Body.String()) != "client provider API keys are not accepted" {
		t.Fatalf("query status=%d body=%q", queryResponse.Code, queryResponse.Body.String())
	}

	jsonRequest := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret, bytes.NewBufferString(`{"prompt":"hello","openai_api_key":"sk-client"}`))
	jsonRequest.Header.Set("Content-Type", "application/json")
	jsonResponse := httptest.NewRecorder()
	router.ServeHTTP(jsonResponse, jsonRequest)
	if jsonResponse.Code != http.StatusBadRequest || strings.TrimSpace(jsonResponse.Body.String()) != "client provider API keys are not accepted" {
		t.Fatalf("json status=%d body=%q", jsonResponse.Code, jsonResponse.Body.String())
	}

	jsonQueryRequest := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret+"&provider_api_key=sk-client", bytes.NewBufferString(`{"prompt":"hello"}`))
	jsonQueryRequest.Header.Set("Content-Type", "application/json")
	jsonQueryResponse := httptest.NewRecorder()
	router.ServeHTTP(jsonQueryResponse, jsonQueryRequest)
	if jsonQueryResponse.Code != http.StatusBadRequest {
		t.Fatalf("json query status=%d body=%q", jsonQueryResponse.Code, jsonQueryResponse.Body.String())
	}

	v2QueryRequest := httptest.NewRequest(http.MethodPost, "/v2?key="+TestSecret+"&xai_api_key=sk-client", bytes.NewBufferString(`{"messages":[{"role":"user","content":"hello"}]}`))
	v2QueryRequest.Header.Set("Content-Type", "application/json")
	v2QueryResponse := httptest.NewRecorder()
	router.ServeHTTP(v2QueryResponse, v2QueryRequest)
	if v2QueryResponse.Code != http.StatusBadRequest {
		t.Fatalf("v2 query status=%d body=%q", v2QueryResponse.Code, v2QueryResponse.Body.String())
	}

	v2JSONRequest := httptest.NewRequest(http.MethodPost, "/v2?key="+TestSecret, bytes.NewBufferString(`{"messages":[{"role":"user","content":"hello"}],"anthropic_api_key":"sk-client"}`))
	v2JSONRequest.Header.Set("Content-Type", "application/json")
	v2JSONResponse := httptest.NewRecorder()
	router.ServeHTTP(v2JSONResponse, v2JSONRequest)
	if v2JSONResponse.Code != http.StatusBadRequest {
		t.Fatalf("v2 json status=%d body=%q", v2JSONResponse.Code, v2JSONResponse.Body.String())
	}

	dictationQueryRequest := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret+"&gemini_api_key=sk-client", nil)
	dictationQueryResponse := httptest.NewRecorder()
	router.ServeHTTP(dictationQueryResponse, dictationQueryRequest)
	if dictationQueryResponse.Code != http.StatusBadRequest {
		t.Fatalf("dictation query status=%d body=%q", dictationQueryResponse.Code, dictationQueryResponse.Body.String())
	}
}

func newManagementRouter(t *testing.T, configuration proxy.Configuration) http.Handler {
	t.Helper()
	return newManagementRouterWithDatabasePath(t, configuration, filepath.Join(t.TempDir(), "managed-tenants.db"))
}

func newManagementRouterWithDatabasePath(t *testing.T, configuration proxy.Configuration, databasePath string) http.Handler {
	t.Helper()
	router, buildError := buildRouterWithCatalogs(t, managementConfigurationWithDatabasePath(configuration, databasePath), zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}
	return router
}

func managementConfigurationWithDatabasePath(configuration proxy.Configuration, databasePath string) proxy.Configuration {
	databaseDSN := "sqlite-test-management"
	var databaseDialector gorm.Dialector = sqlite.Open(databasePath)
	if databasePath == "" {
		databaseDSN = ""
		databaseDialector = nil
	}
	configuration.Management = proxy.ManagementConfiguration{
		Enabled:                  true,
		PublicOrigin:             "http://localhost:8080",
		UIDescription:            "LLM Proxy",
		UIOrigins:                []string{"http://localhost:8080", "http://127.0.0.1:4179", "http://localhost:4179"},
		AdminEmails:              []string{testManagementAdminEmail},
		TAuthURL:                 "http://localhost:8443",
		TAuthTenantID:            testManagementTenantID,
		GoogleClientID:           "google-client-id",
		LoginPath:                "/auth/google",
		LogoutPath:               "/auth/logout",
		NoncePath:                "/auth/nonce",
		JWTSigningKey:            testManagementSigningKey,
		SessionCookieName:        testManagementCookieName,
		DatabaseDialect:          proxy.ManagementDatabaseDialectSQLite,
		DatabaseDSN:              databaseDSN,
		ProviderKeyEncryptionKey: testManagementProviderKeyEncryptionKey,
		ManagementAPIOrigin:      "http://localhost:8080",
		ProxyOrigin:              "http://localhost:8080",
		DatabaseDialector:        databaseDialector,
	}
	configuration.LogLevel = proxy.LogLevelInfo
	configuration.WorkerCount = 1
	configuration.QueueSize = 1
	configuration.RequestTimeoutSeconds = TestTimeout
	return configuration
}

func managementSessionCookie(t *testing.T, userID string) *http.Cookie {
	t.Helper()
	return managementSessionCookieWithEmail(t, userID, userID+"@example.com")
}

func managementSessionCookieWithEmail(t *testing.T, userID string, userEmail string) *http.Cookie {
	t.Helper()
	now := time.Now().UTC()
	return signedManagementSessionCookie(t, &sessionvalidator.Claims{
		TenantID:        testManagementTenantID,
		UserID:          userID,
		UserEmail:       userEmail,
		UserDisplayName: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    proxy.DefaultManagementJWTIssuer,
			IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	})
}

func managementSessionCookieWithClaims(t *testing.T, claims jwt.MapClaims) *http.Cookie {
	t.Helper()
	if _, hasExpiry := claims["exp"]; !hasExpiry {
		claims["exp"] = time.Now().UTC().Add(time.Hour).Unix()
	}
	return signedManagementSessionCookie(t, claims)
}

func managementSessionCookieWithoutExpiration(t *testing.T) *http.Cookie {
	t.Helper()
	return signedManagementSessionCookie(t, jwt.MapClaims{
		"iss":       "tauth",
		"tenant_id": testManagementTenantID,
		"user_id":   "user-without-expiration",
	})
}

func signedManagementSessionCookie(t *testing.T, claims jwt.Claims) *http.Cookie {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, signingError := token.SignedString([]byte(testManagementSigningKey))
	if signingError != nil {
		t.Fatalf("sign token: %v", signingError)
	}
	return &http.Cookie{Name: testManagementCookieName, Value: signedToken}
}

func authenticatedJSONRequest(method string, path string, body string, sessionCookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(sessionCookie)
	return request
}

type managementUsageTestResponse struct {
	PeriodDays int `json:"period_days"`
	Totals     struct {
		Requests           int `json:"requests"`
		SuccessfulRequests int `json:"successful_requests"`
		FailedRequests     int `json:"failed_requests"`
		TextRequests       int `json:"text_requests"`
		DictationRequests  int `json:"dictation_requests"`
		RequestTokens      int `json:"request_tokens"`
		ResponseTokens     int `json:"response_tokens"`
		TotalTokens        int `json:"total_tokens"`
	} `json:"totals"`
	Daily []struct {
		Date string `json:"date"`
		Data struct {
			Requests int `json:"requests"`
		} `json:"data"`
	} `json:"daily"`
	Providers []struct {
		Provider string `json:"provider"`
		Data     struct {
			Requests int `json:"requests"`
		} `json:"data"`
	} `json:"providers"`
	StatusCodes []struct {
		StatusCode int `json:"status_code"`
		Requests   int `json:"requests"`
	} `json:"status_codes"`
}

type managementAdminUsersTestResponse struct {
	PeriodDays int `json:"period_days"`
	Users      []struct {
		User struct {
			ID          string `json:"id"`
			Email       string `json:"email"`
			DisplayName string `json:"display_name"`
			IsAdmin     bool   `json:"is_admin"`
		} `json:"user"`
		Tenant struct {
			ID        string `json:"id"`
			HasSecret bool   `json:"has_secret"`
		} `json:"tenant"`
		Usage managementUsageTestResponse `json:"usage"`
	} `json:"users"`
}

func requestManagementUsage(t *testing.T, router http.Handler, sessionCookie *http.Cookie) managementUsageTestResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/management/usage", nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("usage status=%d body=%s", response.Code, response.Body.String())
	}
	var usage managementUsageTestResponse
	if decodeError := json.Unmarshal(response.Body.Bytes(), &usage); decodeError != nil {
		t.Fatalf("decode usage: %v", decodeError)
	}
	return usage
}

type managedTenantFixture struct {
	UserID                   string `gorm:"primaryKey"`
	UserEmail                string
	UserDisplayName          string
	UserAvatarURL            string
	TenantID                 string `gorm:"uniqueIndex"`
	SecretDigest             string `gorm:"index"`
	DefaultProvider          string
	DefaultModel             string
	DefaultDictationProvider string
	DefaultDictationModel    string
	DefaultSystemPrompt      string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

func (managedTenantFixture) TableName() string {
	return "managed_tenant_records"
}

type managedProviderKeyFixture struct {
	UserID          string `gorm:"primaryKey"`
	ProviderID      string `gorm:"primaryKey"`
	APIKey          string `gorm:"column:api_key"`
	EncryptedAPIKey string
	TextModel       string
	SystemPrompt    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (managedProviderKeyFixture) TableName() string {
	return "managed_provider_api_key_records"
}

type managedUsageFixture struct {
	ID                  uint `gorm:"primaryKey"`
	UserID              string
	TenantID            string
	Endpoint            string
	ProviderID          string
	ModelID             string
	StatusCode          int
	Success             bool
	LatencyMilliseconds int64
	RequestTokens       int
	ResponseTokens      int
	TotalTokens         int
	CreatedAt           time.Time
}

func (managedUsageFixture) TableName() string {
	return "managed_usage_event_records"
}

func seedLegacyManagedTenant(t *testing.T, databasePath string, tenantID string, rawSecret string, providerAPIKey string) {
	t.Helper()
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open legacy fixture database: %v", openError)
	}
	legacyUserID := "static-config:" + tenantID
	timestamp := time.Now().UTC().Add(-time.Hour)
	secretDigest := sha256.Sum256([]byte(rawSecret))
	tenantRecord := managedTenantFixture{
		UserID:                   legacyUserID,
		TenantID:                 tenantID,
		SecretDigest:             hex.EncodeToString(secretDigest[:]),
		DefaultProvider:          proxy.ProviderNameDeepSeek,
		DefaultModel:             proxy.ModelNameDeepSeekV4Flash,
		DefaultDictationProvider: proxy.ProviderNameOpenAI,
		DefaultDictationModel:    proxy.DefaultDictationModel,
		CreatedAt:                timestamp,
		UpdatedAt:                timestamp,
	}
	if createError := database.Create(&tenantRecord).Error; createError != nil {
		t.Fatalf("create legacy tenant fixture: %v", createError)
	}
	providerRecord := managedProviderKeyFixture{
		UserID:          legacyUserID,
		ProviderID:      proxy.ProviderNameDeepSeek,
		EncryptedAPIKey: encryptLegacyProviderKey(t, legacyUserID, proxy.ProviderNameDeepSeek, providerAPIKey),
		TextModel:       proxy.ModelNameDeepSeekV4Flash,
		CreatedAt:       timestamp,
		UpdatedAt:       timestamp,
	}
	if createError := database.Create(&providerRecord).Error; createError != nil {
		t.Fatalf("create legacy provider fixture: %v", createError)
	}
	usageRecord := managedUsageFixture{
		UserID:              legacyUserID,
		TenantID:            tenantID,
		Endpoint:            "text",
		ProviderID:          proxy.ProviderNameDeepSeek,
		ModelID:             proxy.ModelNameDeepSeekV4Flash,
		StatusCode:          http.StatusOK,
		Success:             true,
		LatencyMilliseconds: 25,
		RequestTokens:       3,
		ResponseTokens:      4,
		TotalTokens:         7,
		CreatedAt:           timestamp,
	}
	if createError := database.Create(&usageRecord).Error; createError != nil {
		t.Fatalf("create legacy usage fixture: %v", createError)
	}
}

func encryptLegacyProviderKey(t *testing.T, userID string, providerID string, apiKey string) string {
	t.Helper()
	encryptionKey, decodeError := base64.StdEncoding.DecodeString(testManagementProviderKeyEncryptionKey)
	if decodeError != nil {
		t.Fatalf("decode test provider encryption key: %v", decodeError)
	}
	blockCipher, cipherError := aes.NewCipher(encryptionKey)
	if cipherError != nil {
		t.Fatalf("create test provider block cipher: %v", cipherError)
	}
	aeadCipher, aeadError := cipher.NewGCM(blockCipher)
	if aeadError != nil {
		t.Fatalf("create test provider AEAD: %v", aeadError)
	}
	nonce := make([]byte, aeadCipher.NonceSize())
	if _, readError := io.ReadFull(rand.Reader, nonce); readError != nil {
		t.Fatalf("read test provider nonce: %v", readError)
	}
	associatedData := []byte(userID + "\x00" + providerID)
	sealedAPIKey := aeadCipher.Seal(nil, nonce, []byte(apiKey), associatedData)
	return "llmpk1:" + base64.StdEncoding.EncodeToString(append(nonce, sealedAPIKey...))
}

func countManagedTenantFixture(t *testing.T, databasePath string, userID string) int64 {
	t.Helper()
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open managed fixture database: %v", openError)
	}
	var recordCount int64
	if countError := database.Model(&managedTenantFixture{}).
		Where(&managedTenantFixture{UserID: userID}).
		Count(&recordCount).
		Error; countError != nil {
		t.Fatalf("count managed tenant fixture: %v", countError)
	}
	return recordCount
}

func requestLegacyConfigSecret(t *testing.T, router http.Handler, secret string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secret)+"&prompt=hello", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
