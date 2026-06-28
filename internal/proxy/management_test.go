package proxy_test

import (
	"bytes"
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
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	testManagementSigningKey  = "management-signing-key"
	testManagementTenantID    = "llm-proxy-test"
	testManagementCookieName  = "llm_proxy_test_session"
	testManagementDeepSeekKey = "sk-user-deepseek"
)

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
		`src="/assets/llm-proxy/js/app.js"`,
		`<mpr-header`,
		`<mpr-user`,
		`<mpr-footer`,
	}
	for _, requiredFragment := range requiredFragments {
		if !strings.Contains(indexHTML, requiredFragment) {
			t.Fatalf("static index missing %q", requiredFragment)
		}
	}
	forbiddenFragments := []string{"tauth.js", "data-config-url=\"/config-ui.yaml\"", "data-mpr-ui-bundle-src", "tauth-login-path", "tauth-logout-path", "tauth-nonce-path", "{{MPR_UI_VERSION}}"}
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
		{method: http.MethodPut, path: "/api/management/provider-keys/unknown", body: `{"api_key":"sk"}`, status: http.StatusBadRequest},
		{method: http.MethodPut, path: "/api/management/provider-keys/openai", body: `{"api_key":""}`, status: http.StatusBadRequest},
		{method: http.MethodPut, path: "/api/management/provider-keys/openai", body: `{"api_key":"sk","extra":true}`, status: http.StatusBadRequest},
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

	saveRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/openai", `{"api_key":"skhort"}`, sessionCookie)
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
	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/deepseek", `{"api_key":"`+testManagementDeepSeekKey+`"}`, sessionCookie)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}
	defaultsBody := `{"provider":"deepseek","model":"` + proxy.ModelNameDeepSeekV4Flash + `","dictation_provider":"","dictation_model":"","system_prompt":""}`
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

func TestManagementMigratesLegacyConfigOnceThenUsesDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "managed-tenants.db")
	legacySecret := "legacy-config-secret"
	legacyDeepSeekKey := "sk-legacy-deepseek"
	staleDeepSeekKey := "sk-stale-deepseek"
	var capturedAuthorizations []string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		capturedAuthorizations = append(capturedAuthorizations, request.Header.Get("Authorization"))
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"legacy migrated ok"}}]}`))
	}))
	defer upstreamServer.Close()

	legacyConfiguration := proxy.Configuration{
		Tenants: proxy.SingleTenantConfigurationsWithDefaults("legacy", legacySecret, proxy.TenantDefaults{
			Provider: proxy.ProviderNameDeepSeek,
			Model:    proxy.ModelNameDeepSeekV4Flash,
		}),
		DeepSeekKey:     legacyDeepSeekKey,
		DeepSeekBaseURL: upstreamServer.URL,
	}
	router := newManagementRouterWithDatabasePath(t, legacyConfiguration, databasePath)
	legacyResponse := requestLegacyConfigSecret(t, router, legacySecret)
	if legacyResponse.Code != http.StatusOK || strings.TrimSpace(legacyResponse.Body.String()) != "legacy migrated ok" {
		t.Fatalf("legacy status=%d body=%s", legacyResponse.Code, legacyResponse.Body.String())
	}

	reloadedConfiguration := proxy.Configuration{DeepSeekBaseURL: upstreamServer.URL}
	reloadedRouter := newManagementRouterWithDatabasePath(t, reloadedConfiguration, databasePath)
	reloadedResponse := requestLegacyConfigSecret(t, reloadedRouter, legacySecret)
	if reloadedResponse.Code != http.StatusOK || strings.TrimSpace(reloadedResponse.Body.String()) != "legacy migrated ok" {
		t.Fatalf("reloaded status=%d body=%s", reloadedResponse.Code, reloadedResponse.Body.String())
	}

	staleConfiguration := proxy.Configuration{
		Tenants: proxy.SingleTenantConfigurationsWithDefaults("legacy", legacySecret, proxy.TenantDefaults{
			Provider: proxy.ProviderNameDeepSeek,
			Model:    proxy.ModelNameDeepSeekV4Flash,
		}),
		DeepSeekKey:     staleDeepSeekKey,
		DeepSeekBaseURL: upstreamServer.URL,
	}
	staleRouter := newManagementRouterWithDatabasePath(t, staleConfiguration, databasePath)
	staleResponse := requestLegacyConfigSecret(t, staleRouter, legacySecret)
	if staleResponse.Code != http.StatusOK || strings.TrimSpace(staleResponse.Body.String()) != "legacy migrated ok" {
		t.Fatalf("stale status=%d body=%s", staleResponse.Code, staleResponse.Body.String())
	}

	if len(capturedAuthorizations) != 3 {
		t.Fatalf("captured authorizations=%v", capturedAuthorizations)
	}
	for authorizationIndex, authorization := range capturedAuthorizations {
		if authorization != "Bearer "+legacyDeepSeekKey {
			t.Fatalf("authorization %d=%q want=%q", authorizationIndex, authorization, "Bearer "+legacyDeepSeekKey)
		}
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
	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/openai", `{"api_key":"sk-user-openai"}`, sessionCookie)
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

func TestManagementGeneratedSecretRoutesWithTenantProviderKey(t *testing.T) {
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
		if upstreamPayload["model"] != proxy.ModelNameDeepSeekV4Flash {
			t.Fatalf("model=%v want=%s", upstreamPayload["model"], proxy.ModelNameDeepSeekV4Flash)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"managed deepseek ok"}}]}`))
	}))
	defer upstreamServer.Close()

	router := newManagementRouter(t, proxy.Configuration{
		DeepSeekBaseURL: upstreamServer.URL,
	})
	userOneCookie := managementSessionCookie(t, "tauth-user-one")
	userTwoCookie := managementSessionCookie(t, "tauth-user-two")

	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/provider-keys/deepseek", `{"api_key":"`+testManagementDeepSeekKey+`"}`, userOneCookie)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}
	if strings.Contains(saveKeyResponse.Body.String(), testManagementDeepSeekKey) || !strings.Contains(saveKeyResponse.Body.String(), "sk-...seek") {
		t.Fatalf("provider key response leaked or failed to mask key: %s", saveKeyResponse.Body.String())
	}

	defaultsBody := `{"provider":"deepseek","model":"` + proxy.ModelNameDeepSeekV4Flash + `","dictation_provider":"","dictation_model":"","system_prompt":""}`
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
	if strings.Contains(userTwoProfileResponse.Body.String(), "sk-...seek") {
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
	proxyRequest := httptest.NewRequest(http.MethodGet, "/?"+proxyRequestValues.Encode(), nil)
	proxyResponse := httptest.NewRecorder()
	router.ServeHTTP(proxyResponse, proxyRequest)
	if proxyResponse.Code != http.StatusOK || strings.TrimSpace(proxyResponse.Body.String()) != "managed deepseek ok" {
		t.Fatalf("proxy status=%d body=%q", proxyResponse.Code, proxyResponse.Body.String())
	}
	if capturedAuthorization != "Bearer "+testManagementDeepSeekKey {
		t.Fatalf("authorization=%q want=%q", capturedAuthorization, "Bearer "+testManagementDeepSeekKey)
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
		Enabled:             true,
		PublicOrigin:        "http://localhost:8080",
		UIDescription:       "LLM Proxy",
		UIOrigins:           []string{"http://localhost:8080", "http://127.0.0.1:4179", "http://localhost:4179"},
		TAuthURL:            "http://localhost:8443",
		TAuthTenantID:       testManagementTenantID,
		GoogleClientID:      "google-client-id",
		LoginPath:           "/auth/google",
		LogoutPath:          "/auth/logout",
		NoncePath:           "/auth/nonce",
		JWTSigningKey:       testManagementSigningKey,
		SessionCookieName:   testManagementCookieName,
		DatabaseDialect:     proxy.ManagementDatabaseDialectSQLite,
		DatabaseDSN:         databaseDSN,
		ManagementAPIOrigin: "http://localhost:8080",
		ProxyOrigin:         "http://localhost:8080",
		DatabaseDialector:   databaseDialector,
	}
	configuration.LogLevel = proxy.LogLevelInfo
	configuration.WorkerCount = 1
	configuration.QueueSize = 1
	configuration.RequestTimeoutSeconds = TestTimeout
	return configuration
}

func managementSessionCookie(t *testing.T, userID string) *http.Cookie {
	t.Helper()
	now := time.Now().UTC()
	return managementSessionCookieWithClaims(t, jwt.MapClaims{
		"iss":               "tauth",
		"tenant_id":         testManagementTenantID,
		"user_id":           userID,
		"user_email":        userID + "@example.com",
		"user_display_name": userID,
		"iat":               now.Add(-time.Minute).Unix(),
		"exp":               now.Add(time.Hour).Unix(),
	})
}

func managementSessionCookieWithClaims(t *testing.T, claims jwt.MapClaims) *http.Cookie {
	t.Helper()
	if _, hasExpiry := claims["exp"]; !hasExpiry {
		claims["exp"] = time.Now().UTC().Add(time.Hour).Unix()
	}
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

func requestLegacyConfigSecret(t *testing.T, router http.Handler, secret string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secret)+"&prompt=hello", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
