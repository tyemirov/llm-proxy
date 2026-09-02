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
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
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
	testProviderKeyVerificationPrompt      = "Verify this provider credential."
)

type managementProviderKeyVerificationHTTPDoer struct {
	next proxy.HTTPDoer
}

func (httpDoer managementProviderKeyVerificationHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	if request.Header.Get("x-goog-api-key") != "" && strings.HasSuffix(request.URL.Path, "/interactions/verification") {
		responseBody := `{}`
		if request.Method == http.MethodGet {
			responseBody = `{"id":"verification","status":"completed"}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(responseBody)),
			Request:    request,
		}, nil
	}
	if request.Body == nil {
		return httpDoer.next.Do(request)
	}
	requestBody, readError := io.ReadAll(request.Body)
	if readError != nil {
		return nil, readError
	}
	request.Body = io.NopCloser(bytes.NewReader(requestBody))
	if !bytes.Contains(requestBody, []byte(testProviderKeyVerificationPrompt)) {
		return httpDoer.next.Do(request)
	}
	responseBody := `{"choices":[{}]}`
	switch {
	case request.Header.Get("x-goog-api-key") != "":
		responseBody = `{"status":"completed"}`
		if bytes.Contains(requestBody, []byte(`"background":true`)) {
			responseBody = `{"id":"verification","status":"queued"}`
		}
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

func managementProviderKeyRequestBody(t *testing.T, apiKey string, textModel string, systemPrompt string) string {
	return managementProviderKeyRequestBodyWithBaseURL(t, apiKey, "", textModel, systemPrompt)
}

func managementProviderKeyRequestBodyWithBaseURL(t *testing.T, apiKey string, baseURL string, textModel string, systemPrompt string) string {
	t.Helper()
	fields := map[string]string{"api_key": apiKey}
	if baseURL != "" {
		fields["base_url"] = baseURL
	}
	requestBody, marshalError := json.Marshal(map[string]any{
		"fields":        fields,
		"text_model":    textModel,
		"system_prompt": systemPrompt,
	})
	if marshalError != nil {
		t.Fatalf("marshal provider key request: %v", marshalError)
	}
	return string(requestBody)
}

func managementDefaultsRequestBody(t *testing.T, provider string, model string, dictationProvider string, dictationModel string, systemPrompt string) string {
	t.Helper()
	requestBody, marshalError := json.Marshal(map[string]string{
		"provider":           provider,
		"model":              model,
		"dictation_provider": dictationProvider,
		"dictation_model":    dictationModel,
		"system_prompt":      systemPrompt,
		"reasoning_effort":   "",
	})
	if marshalError != nil {
		t.Fatalf("marshal defaults request: %v", marshalError)
	}
	return string(requestBody)
}

func managementDefaultsRequestBodyWithReasoningEffort(t *testing.T, provider string, model string, dictationProvider string, dictationModel string, systemPrompt string, reasoningEffort string) string {
	t.Helper()
	requestBody, marshalError := json.Marshal(map[string]string{
		"provider":           provider,
		"model":              model,
		"dictation_provider": dictationProvider,
		"dictation_model":    dictationModel,
		"system_prompt":      systemPrompt,
		"reasoning_effort":   reasoningEffort,
	})
	if marshalError != nil {
		t.Fatalf("marshal defaults request: %v", marshalError)
	}
	return string(requestBody)
}

func TestManagementStaticPagesAndUnauthenticatedAPI(t *testing.T) {
	staticServer := httptest.NewServer(http.FileServer(http.Dir("../../site")))
	defer staticServer.Close()

	landingResponse, landingError := http.Get(staticServer.URL + "/")
	if landingError != nil {
		t.Fatalf("static landing request: %v", landingError)
	}
	defer landingResponse.Body.Close()
	if landingResponse.StatusCode != http.StatusOK {
		t.Fatalf("static landing status=%d want=%d", landingResponse.StatusCode, http.StatusOK)
	}
	landingBytes, readLandingError := io.ReadAll(landingResponse.Body)
	if readLandingError != nil {
		t.Fatalf("read static landing: %v", readLandingError)
	}
	landingHTML := string(landingBytes)
	for _, requiredFragment := range []string{
		`Integrate once. Use the model that fits.`,
		`data-config-url="/config-ui.yaml"`,
		`sign-in-label="Log In"`,
		`data-llm-proxy-authenticated-redirect-url="/app/"`,
		`src="/assets/llm-proxy/js/ui/landingAuthRoute.js?v=20260808b113"`,
		`<!-- llm-proxy-capability-catalog -->`,
		`<mpr-header`,
		`<mpr-footer`,
	} {
		if !strings.Contains(landingHTML, requiredFragment) {
			t.Fatalf("static landing missing %q", requiredFragment)
		}
	}
	if strings.Contains(landingHTML, `sign-in-redirect-url=`) {
		t.Fatal("static landing must use the authenticated route guard as its single redirect owner")
	}
	if strings.Contains(landingHTML, `src="https://tauth.mprlab.com/tauth.js"`) {
		t.Fatal("static landing must delegate browser authentication to MPR UI")
	}

	staticIndexResponse, indexError := http.Get(staticServer.URL + "/app/")
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
		`href="https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.css"`,
		`src="https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui-config.js"`,
		`data-mpr-ui-bundle-src="https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.js"`,
		`src="/assets/llm-proxy/js/startupGuard.js?v=20260902c237"`,
		`src="/assets/llm-proxy/js/app.js?v=20260902c237"`,
		`data-config-url="/config-ui.yaml"`,
		`<mpr-user`,
		`<mpr-footer`,
	}
	for _, requiredFragment := range requiredFragments {
		if !strings.Contains(indexHTML, requiredFragment) {
			t.Fatalf("static index missing %q", requiredFragment)
		}
	}
	forbiddenFragments := []string{"Sign in to manage LLM Proxy keys", "MarcoPoloResearchLab/mpr-ui@v", `src="https://tauth.mprlab.com/tauth.js"`, "tauth-login-path", "tauth-logout-path", "tauth-nonce-path", "{{MPR_UI_VERSION}}"}
	for _, forbiddenFragment := range forbiddenFragments {
		if strings.Contains(indexHTML, forbiddenFragment) {
			t.Fatalf("static index must not include %q", forbiddenFragment)
		}
	}

	removedIndexResponse, removedIndexError := http.Get(staticServer.URL + "/manage/")
	if removedIndexError != nil {
		t.Fatalf("removed static index request: %v", removedIndexError)
	}
	defer removedIndexResponse.Body.Close()
	if removedIndexResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("removed static index status=%d want=%d", removedIndexResponse.StatusCode, http.StatusNotFound)
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
		`sessionPath: "/auth/session"`,
	} {
		if !strings.Contains(configBody, requiredFragment) {
			t.Fatalf("%s missing %q in %s", proxy.ManagementConfigUIFileName, requiredFragment, configBody)
		}
	}
	if strings.Contains(configBody, "authButton") {
		t.Fatalf("%s must keep login-button presentation in static markup: %s", proxy.ManagementConfigUIFileName, configBody)
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

	profileRequest := httptest.NewRequest(http.MethodGet, "/api/management/account", nil)
	profileResponse := httptest.NewRecorder()
	router.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusUnauthorized {
		t.Fatalf("profile status=%d want=%d", profileResponse.Code, http.StatusUnauthorized)
	}

	corsProfileRequest := httptest.NewRequest(http.MethodGet, "/api/management/account", nil)
	corsProfileRequest.Header.Set("Origin", "http://localhost:8080")
	corsProfileResponse := httptest.NewRecorder()
	router.ServeHTTP(corsProfileResponse, corsProfileRequest)
	if corsProfileResponse.Code != http.StatusUnauthorized {
		t.Fatalf("cors profile status=%d want=%d", corsProfileResponse.Code, http.StatusUnauthorized)
	}
	if corsProfileResponse.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8080" || corsProfileResponse.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("cors profile headers=%v", corsProfileResponse.Header())
	}

	preflightRequest := httptest.NewRequest(http.MethodOptions, "/api/management/account", nil)
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

	disallowedPreflightRequest := httptest.NewRequest(http.MethodOptions, "/api/management/account", nil)
	disallowedPreflightRequest.Header.Set("Origin", "https://other.example")
	disallowedPreflightResponse := httptest.NewRecorder()
	router.ServeHTTP(disallowedPreflightResponse, disallowedPreflightRequest)
	if disallowedPreflightResponse.Code != http.StatusForbidden {
		t.Fatalf("disallowed preflight status=%d want=%d", disallowedPreflightResponse.Code, http.StatusForbidden)
	}

	originSessionCookie := managementSessionCookie(t, "tauth-origin-user")
	originSecretsPath := managementDefaultTenantTestPath(t, router, originSessionCookie, "/secrets")
	disallowedMutationRequest := authenticatedJSONRequest(http.MethodPost, originSecretsPath, `{}`, originSessionCookie)
	disallowedMutationRequest.Header.Set("Origin", "https://other.example")
	disallowedMutationResponse := httptest.NewRecorder()
	router.ServeHTTP(disallowedMutationResponse, disallowedMutationRequest)
	if disallowedMutationResponse.Code != http.StatusForbidden {
		t.Fatalf("disallowed mutation status=%d want=%d", disallowedMutationResponse.Code, http.StatusForbidden)
	}

	missingContentTypeMutationRequest := httptest.NewRequest(http.MethodPost, originSecretsPath, strings.NewReader(""))
	missingContentTypeMutationRequest.AddCookie(originSessionCookie)
	missingContentTypeMutationResponse := httptest.NewRecorder()
	router.ServeHTTP(missingContentTypeMutationResponse, missingContentTypeMutationRequest)
	if missingContentTypeMutationResponse.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("missing content type mutation status=%d want=%d", missingContentTypeMutationResponse.Code, http.StatusUnsupportedMediaType)
	}

	simpleMutationRequest := httptest.NewRequest(http.MethodPost, originSecretsPath, strings.NewReader(""))
	simpleMutationRequest.Header.Set("Content-Type", "text/plain")
	simpleMutationRequest.AddCookie(originSessionCookie)
	simpleMutationResponse := httptest.NewRecorder()
	router.ServeHTTP(simpleMutationResponse, simpleMutationRequest)
	if simpleMutationResponse.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("simple mutation status=%d want=%d", simpleMutationResponse.Code, http.StatusUnsupportedMediaType)
	}

	allowedMutationRequest := authenticatedJSONRequest(http.MethodPost, originSecretsPath, `{}`, originSessionCookie)
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
		request := httptest.NewRequest(http.MethodGet, "/api/management/account", nil)
		request.AddCookie(invalidCookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("invalid cookie %d status=%d want=%d body=%s", cookieIndex, response.Code, http.StatusUnauthorized, response.Body.String())
		}
	}

	sessionCookie := managementSessionCookie(t, "tauth-user-errors")
	tenantPath := managementDefaultTenantTestPath(t, router, sessionCookie, "")
	badRequests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{method: http.MethodPut, path: tenantPath + "/provider-connections/unknown", body: managementProviderKeyRequestBody(t, "sk", proxy.ModelNameGPT41, ""), status: http.StatusBadRequest},
		{method: http.MethodPut, path: tenantPath + "/provider-connections/qwencloud", body: managementProviderKeyRequestBody(t, "sk", "qwen3.8-max-preview", "retired"), status: http.StatusBadRequest},
		{method: http.MethodPut, path: tenantPath + "/provider-connections/openai", body: managementProviderKeyRequestBody(t, "", proxy.ModelNameGPT41, ""), status: http.StatusBadRequest},
		{method: http.MethodPut, path: tenantPath + "/provider-connections/openai", body: `{"api_key":"sk","text_model":"gpt-4.1","system_prompt":"","extra":true}`, status: http.StatusBadRequest},
		{method: http.MethodPut, path: tenantPath + "/provider-connections/openai", body: `{"api_key":"sk","system_prompt":""}`, status: http.StatusBadRequest},
		{method: http.MethodPut, path: tenantPath + "/provider-connections/openai", body: managementProviderKeyRequestBody(t, "sk", "missing-model", ""), status: http.StatusBadRequest},
		{method: http.MethodDelete, path: tenantPath + "/provider-connections/unknown", body: `{}`, status: http.StatusBadRequest},
		{method: http.MethodPut, path: tenantPath + "/defaults", body: managementDefaultsRequestBody(t, "qwencloud", "qwen3.8-max-preview", "", "", ""), status: http.StatusBadRequest},
		{method: http.MethodPut, path: tenantPath + "/defaults", body: `{"provider":"openai","model":"gpt-4.1","extra":true}`, status: http.StatusBadRequest},
		{method: http.MethodPut, path: tenantPath + "/defaults", body: `{"provider":"openai","model":"gpt-4.1","dictation_provider":"","dictation_model":"","system_prompt":"","reasoning_effort":""}`, status: http.StatusBadRequest},
	}
	for _, badRequest := range badRequests {
		request := authenticatedJSONRequest(badRequest.method, badRequest.path, badRequest.body, sessionCookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != badRequest.status {
			t.Fatalf("%s %s status=%d want=%d body=%s", badRequest.method, badRequest.path, response.Code, badRequest.status, response.Body.String())
		}
	}

	saveRequest := authenticatedJSONRequest(http.MethodPut, tenantPath+"/provider-connections/openai", managementProviderKeyRequestBody(t, "skhort", proxy.ModelNameGPT41, ""), sessionCookie)
	saveResponse := httptest.NewRecorder()
	router.ServeHTTP(saveResponse, saveRequest)
	if saveResponse.Code != http.StatusOK || !strings.Contains(saveResponse.Body.String(), `"masked_value":"saved"`) {
		t.Fatalf("save short key status=%d body=%s", saveResponse.Code, saveResponse.Body.String())
	}

	dictationDefaults := `{"provider":"openai","model":"gpt-4.1","dictation_provider":"deepseek","dictation_model":"deepseek-v4-flash","system_prompt":"","reasoning_effort":""}`
	dictationDefaultsRequest := authenticatedJSONRequest(http.MethodPut, tenantPath+"/defaults", dictationDefaults, sessionCookie)
	dictationDefaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(dictationDefaultsResponse, dictationDefaultsRequest)
	if dictationDefaultsResponse.Code != http.StatusBadRequest {
		t.Fatalf("dictation defaults status=%d body=%s", dictationDefaultsResponse.Code, dictationDefaultsResponse.Body.String())
	}

	deepSeekOnlyCookie := managementSessionCookie(t, "tauth-deepseek-only")
	deepSeekTenantPath := managementDefaultTenantTestPath(t, router, deepSeekOnlyCookie, "")
	saveDeepSeekRequest := authenticatedJSONRequest(http.MethodPut, deepSeekTenantPath+"/provider-connections/deepseek", managementProviderKeyRequestBody(t, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""), deepSeekOnlyCookie)
	saveDeepSeekResponse := httptest.NewRecorder()
	router.ServeHTTP(saveDeepSeekResponse, saveDeepSeekRequest)
	if saveDeepSeekResponse.Code != http.StatusOK {
		t.Fatalf("save deepseek key status=%d body=%s", saveDeepSeekResponse.Code, saveDeepSeekResponse.Body.String())
	}
	blankDictationDefaults := `{"provider":"deepseek","model":"` + proxy.ModelNameDeepSeekV4Flash + `","dictation_provider":"","dictation_model":"","system_prompt":"","reasoning_effort":""}`
	blankDictationDefaultsRequest := authenticatedJSONRequest(http.MethodPut, deepSeekTenantPath+"/defaults", blankDictationDefaults, deepSeekOnlyCookie)
	blankDictationDefaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(blankDictationDefaultsResponse, blankDictationDefaultsRequest)
	if blankDictationDefaultsResponse.Code != http.StatusOK ||
		!strings.Contains(blankDictationDefaultsResponse.Body.String(), `"dictation_provider":"","dictation_model":""`) {
		t.Fatalf("blank dictation defaults status=%d want=%d body=%s", blankDictationDefaultsResponse.Code, http.StatusOK, blankDictationDefaultsResponse.Body.String())
	}

	removeRequest := authenticatedJSONRequest(http.MethodDelete, tenantPath+"/provider-connections/openai", `{}`, sessionCookie)
	removeResponse := httptest.NewRecorder()
	router.ServeHTTP(removeResponse, removeRequest)
	if removeResponse.Code != http.StatusOK || strings.Contains(removeResponse.Body.String(), `"has_key":true`) {
		t.Fatalf("remove status=%d body=%s", removeResponse.Code, removeResponse.Body.String())
	}
}

func TestManagementProviderKeyRevealIsOwnerScoped(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	ownerCookie := managementSessionCookie(t, "tauth-reveal-owner")
	otherCookie := managementSessionCookie(t, "tauth-reveal-other")
	adminCookie := managementSessionCookieWithEmail(t, "tauth-reveal-admin", testManagementAdminEmail)
	ownerTenantPath := managementDefaultTenantTestPath(t, router, ownerCookie, "")

	saveRequest := authenticatedJSONRequest(http.MethodPut, ownerTenantPath+"/provider-connections/openai", managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT41, ""), ownerCookie)
	saveResponse := httptest.NewRecorder()
	router.ServeHTTP(saveResponse, saveRequest)
	if saveResponse.Code != http.StatusOK {
		t.Fatalf("save provider key status=%d body=%s", saveResponse.Code, saveResponse.Body.String())
	}
	if strings.Contains(saveResponse.Body.String(), testManagementOpenAIKey) {
		t.Fatalf("save provider key response leaked raw key: %s", saveResponse.Body.String())
	}

	unauthenticatedRevealRequest := httptest.NewRequest(http.MethodPost, ownerTenantPath+"/provider-connections/openai/fields/api_key/reveal", strings.NewReader(`{}`))
	unauthenticatedRevealRequest.Header.Set("Content-Type", "application/json")
	unauthenticatedRevealRequest.Header.Set("Origin", "http://localhost:8080")
	unauthenticatedRevealResponse := httptest.NewRecorder()
	router.ServeHTTP(unauthenticatedRevealResponse, unauthenticatedRevealRequest)
	if unauthenticatedRevealResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated reveal status=%d want=%d body=%s", unauthenticatedRevealResponse.Code, http.StatusUnauthorized, unauthenticatedRevealResponse.Body.String())
	}

	for _, rejectedReveal := range []struct {
		name       string
		request    *http.Request
		statusCode int
	}{
		{
			name:       "missing origin",
			request:    authenticatedJSONRequest(http.MethodPost, ownerTenantPath+"/provider-connections/openai/fields/api_key/reveal", `{}`, ownerCookie),
			statusCode: http.StatusForbidden,
		},
		{
			name:       "wrong origin",
			request:    authenticatedProviderKeyRevealRequest(http.MethodPost, ownerTenantPath+"/provider-connections/openai/fields/api_key/reveal", ownerCookie, "https://other.example"),
			statusCode: http.StatusForbidden,
		},
		{
			name:       "missing content type",
			request:    providerKeyRevealRequestWithoutContentType(http.MethodPost, ownerTenantPath+"/provider-connections/openai/fields/api_key/reveal", ownerCookie, "http://localhost:8080"),
			statusCode: http.StatusUnsupportedMediaType,
		},
	} {
		t.Run(rejectedReveal.name, func(subTest *testing.T) {
			rejectedRevealResponse := httptest.NewRecorder()
			router.ServeHTTP(rejectedRevealResponse, rejectedReveal.request)
			if rejectedRevealResponse.Code != rejectedReveal.statusCode {
				subTest.Fatalf("reveal status=%d want=%d body=%s", rejectedRevealResponse.Code, rejectedReveal.statusCode, rejectedRevealResponse.Body.String())
			}
			if strings.Contains(rejectedRevealResponse.Body.String(), testManagementOpenAIKey) {
				subTest.Fatalf("rejected reveal leaked raw key: %s", rejectedRevealResponse.Body.String())
			}
		})
	}

	ownerRevealRequest := authenticatedProviderKeyRevealRequest(http.MethodPost, ownerTenantPath+"/provider-connections/openai/fields/api_key/reveal", ownerCookie, "http://localhost:8080")
	ownerRevealResponse := httptest.NewRecorder()
	router.ServeHTTP(ownerRevealResponse, ownerRevealRequest)
	if ownerRevealResponse.Code != http.StatusOK {
		t.Fatalf("owner reveal status=%d want=%d body=%s", ownerRevealResponse.Code, http.StatusOK, ownerRevealResponse.Body.String())
	}
	if ownerRevealResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("owner reveal cache-control=%q want=no-store", ownerRevealResponse.Header().Get("Cache-Control"))
	}
	var ownerRevealPayload map[string]string
	if decodeError := json.Unmarshal(ownerRevealResponse.Body.Bytes(), &ownerRevealPayload); decodeError != nil {
		t.Fatalf("decode owner reveal: %v", decodeError)
	}
	if len(ownerRevealPayload) != 2 || ownerRevealPayload["field_id"] != "api_key" || ownerRevealPayload["value"] != testManagementOpenAIKey {
		t.Fatalf("owner reveal payload=%+v", ownerRevealPayload)
	}

	for _, unavailableReveal := range []struct {
		name       string
		path       string
		cookie     *http.Cookie
		statusCode int
	}{
		{name: "different owner", path: ownerTenantPath + "/provider-connections/openai/fields/api_key/reveal", cookie: otherCookie, statusCode: http.StatusNotFound},
		{name: "missing provider key", path: ownerTenantPath + "/provider-connections/deepseek/fields/api_key/reveal", cookie: ownerCookie, statusCode: http.StatusNotFound},
		{name: "unknown provider", path: ownerTenantPath + "/provider-connections/unknown/fields/api_key/reveal", cookie: ownerCookie, statusCode: http.StatusBadRequest},
	} {
		t.Run(unavailableReveal.name, func(subTest *testing.T) {
			unavailableRevealRequest := authenticatedProviderKeyRevealRequest(http.MethodPost, unavailableReveal.path, unavailableReveal.cookie, "http://localhost:8080")
			unavailableRevealResponse := httptest.NewRecorder()
			router.ServeHTTP(unavailableRevealResponse, unavailableRevealRequest)
			if unavailableRevealResponse.Code != unavailableReveal.statusCode {
				subTest.Fatalf("reveal status=%d want=%d body=%s", unavailableRevealResponse.Code, unavailableReveal.statusCode, unavailableRevealResponse.Body.String())
			}
			if strings.Contains(unavailableRevealResponse.Body.String(), testManagementOpenAIKey) {
				subTest.Fatalf("unavailable reveal leaked raw key: %s", unavailableRevealResponse.Body.String())
			}
		})
	}

	profileRequest := httptest.NewRequest(http.MethodGet, ownerTenantPath, nil)
	profileRequest.AddCookie(ownerCookie)
	profileResponse := httptest.NewRecorder()
	router.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusOK || strings.Contains(profileResponse.Body.String(), testManagementOpenAIKey) {
		t.Fatalf("profile status=%d body=%s", profileResponse.Code, profileResponse.Body.String())
	}

	adminRequest := httptest.NewRequest(http.MethodGet, "/api/management/admin/users", nil)
	adminRequest.AddCookie(adminCookie)
	adminResponse := httptest.NewRecorder()
	router.ServeHTTP(adminResponse, adminRequest)
	if adminResponse.Code != http.StatusOK || strings.Contains(adminResponse.Body.String(), testManagementOpenAIKey) {
		t.Fatalf("admin status=%d body=%s", adminResponse.Code, adminResponse.Body.String())
	}
}

func TestManagementProviderKeyRevealPersistsUpdatedKey(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "managed-tenants.db")
	updatedProviderKey := "sk-user-deepseek-updated"
	capturedAuthorizations := []string{}
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		capturedAuthorizations = append(capturedAuthorizations, request.Header.Get("Authorization"))
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"updated key ok"},"finish_reason":"stop"}]}`))
	}))
	defer upstreamServer.Close()

	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{Endpoints: providerEndpoints(upstreamServer.URL, proxy.ProviderNameDeepSeek)}, databasePath)
	ownerCookie := managementSessionCookie(t, "tauth-reveal-persistence-owner")
	ownerTenantID := managementDefaultTenantTestID(t, router, ownerCookie)
	ownerTenantPath := "/api/management/tenants/" + url.PathEscape(ownerTenantID)
	saveOriginalRequest := authenticatedJSONRequest(http.MethodPut, ownerTenantPath+"/provider-connections/deepseek", managementProviderKeyRequestBody(t, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""), ownerCookie)
	saveOriginalResponse := httptest.NewRecorder()
	router.ServeHTTP(saveOriginalResponse, saveOriginalRequest)
	if saveOriginalResponse.Code != http.StatusOK {
		t.Fatalf("save original key status=%d body=%s", saveOriginalResponse.Code, saveOriginalResponse.Body.String())
	}

	database := openManagedFixtureDatabase(t, databasePath)
	originalProviderKeyRecord, queryError := loadManagedProviderKeyFixture(database, ownerTenantID, proxy.ProviderNameDeepSeek)
	if queryError != nil {
		t.Fatalf("load original provider key record: %v", queryError)
	}
	if strings.Contains(originalProviderKeyRecord.EncryptedAPIKey, testManagementDeepSeekKey) {
		t.Fatalf("original provider key record=%+v", originalProviderKeyRecord)
	}

	originalRevealRequest := authenticatedProviderKeyRevealRequest(http.MethodPost, ownerTenantPath+"/provider-connections/deepseek/fields/api_key/reveal", ownerCookie, "http://localhost:8080")
	originalRevealResponse := httptest.NewRecorder()
	router.ServeHTTP(originalRevealResponse, originalRevealRequest)
	if originalRevealResponse.Code != http.StatusOK || !strings.Contains(originalRevealResponse.Body.String(), testManagementDeepSeekKey) {
		t.Fatalf("original reveal status=%d body=%s", originalRevealResponse.Code, originalRevealResponse.Body.String())
	}

	saveUpdatedRequest := authenticatedJSONRequest(http.MethodPut, ownerTenantPath+"/provider-connections/deepseek", managementProviderKeyRequestBody(t, updatedProviderKey, proxy.ModelNameDeepSeekV4Flash, ""), ownerCookie)
	saveUpdatedResponse := httptest.NewRecorder()
	router.ServeHTTP(saveUpdatedResponse, saveUpdatedRequest)
	if saveUpdatedResponse.Code != http.StatusOK || strings.Contains(saveUpdatedResponse.Body.String(), updatedProviderKey) {
		t.Fatalf("save updated key status=%d body=%s", saveUpdatedResponse.Code, saveUpdatedResponse.Body.String())
	}

	updatedProviderKeyRecord, queryError := loadManagedProviderKeyFixture(database, ownerTenantID, proxy.ProviderNameDeepSeek)
	if queryError != nil {
		t.Fatalf("load updated provider key record: %v", queryError)
	}
	if updatedProviderKeyRecord.EncryptedAPIKey == originalProviderKeyRecord.EncryptedAPIKey || strings.Contains(updatedProviderKeyRecord.EncryptedAPIKey, updatedProviderKey) {
		t.Fatalf("updated provider key record=%+v", updatedProviderKeyRecord)
	}

	updatedRevealRequest := authenticatedProviderKeyRevealRequest(http.MethodPost, ownerTenantPath+"/provider-connections/deepseek/fields/api_key/reveal", ownerCookie, "http://localhost:8080")
	updatedRevealResponse := httptest.NewRecorder()
	router.ServeHTTP(updatedRevealResponse, updatedRevealRequest)
	if updatedRevealResponse.Code != http.StatusOK || !strings.Contains(updatedRevealResponse.Body.String(), updatedProviderKey) {
		t.Fatalf("updated reveal status=%d body=%s", updatedRevealResponse.Code, updatedRevealResponse.Body.String())
	}

	secretRequest := authenticatedJSONRequest(http.MethodPost, ownerTenantPath+"/secrets", `{}`, ownerCookie)
	secretResponse := httptest.NewRecorder()
	router.ServeHTTP(secretResponse, secretRequest)
	if secretResponse.Code != http.StatusOK {
		t.Fatalf("generate secret status=%d body=%s", secretResponse.Code, secretResponse.Body.String())
	}
	var secretPayload struct {
		Secret string `json:"secret"`
	}
	if decodeError := json.Unmarshal(secretResponse.Body.Bytes(), &secretPayload); decodeError != nil {
		t.Fatalf("decode generated secret: %v", decodeError)
	}

	for _, proxyRouter := range []http.Handler{router, newManagementRouterWithDatabasePath(t, proxy.Configuration{Endpoints: providerEndpoints(upstreamServer.URL, proxy.ProviderNameDeepSeek)}, databasePath)} {
		proxyRequest := httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secretPayload.Secret)+"&provider=deepseek&model="+proxy.ModelNameDeepSeekV4Flash+"&prompt=hello", nil)
		proxyResponse := httptest.NewRecorder()
		proxyRouter.ServeHTTP(proxyResponse, proxyRequest)
		if proxyResponse.Code != http.StatusOK || strings.TrimSpace(proxyResponse.Body.String()) != "updated key ok" {
			t.Fatalf("updated key proxy status=%d body=%s", proxyResponse.Code, proxyResponse.Body.String())
		}
	}
	if len(capturedAuthorizations) != 2 || capturedAuthorizations[0] != "Bearer "+updatedProviderKey || capturedAuthorizations[1] != "Bearer "+updatedProviderKey {
		t.Fatalf("updated key authorizations=%v", capturedAuthorizations)
	}
	waitForManagementRequestCount(t, router, ownerCookie, 2)
	if updateError := database.Model(&managedProviderConnectionFixture{}).Where("tenant_id = ? AND provider_id = ? AND field_id = ?", ownerTenantID, proxy.ProviderNameDeepSeek, proxy.CatalogCredentialAPIKey).Update("value", "invalid").Error; updateError != nil {
		t.Fatalf("corrupt updated provider key record: %v", updateError)
	}
	corruptRevealRequest := authenticatedProviderKeyRevealRequest(http.MethodPost, ownerTenantPath+"/provider-connections/deepseek/fields/api_key/reveal", ownerCookie, "http://localhost:8080")
	corruptRevealResponse := httptest.NewRecorder()
	router.ServeHTTP(corruptRevealResponse, corruptRevealRequest)
	if corruptRevealResponse.Code != http.StatusInternalServerError || strings.Contains(corruptRevealResponse.Body.String(), updatedProviderKey) {
		t.Fatalf("corrupt reveal status=%d body=%s", corruptRevealResponse.Code, corruptRevealResponse.Body.String())
	}
}

func TestManagementRoutingDefaultsRequireCompleteCanonicalPairs(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	sessionCookie := managementSessionCookie(t, "tauth-routing-defaults-user")
	tenantPath := managementDefaultTenantTestPath(t, router, sessionCookie, "")
	providerKeyRequests := []struct {
		provider string
		apiKey   string
		model    string
	}{
		{provider: proxy.ProviderNameDeepSeek, apiKey: testManagementDeepSeekKey, model: proxy.ModelNameDeepSeekV4Flash},
		{provider: proxy.ProviderNameOpenAI, apiKey: testManagementOpenAIKey, model: proxy.ModelNameGPT41},
	}
	for _, providerKeyRequest := range providerKeyRequests {
		request := authenticatedJSONRequest(http.MethodPut, tenantPath+"/provider-connections/"+providerKeyRequest.provider, managementProviderKeyRequestBody(t, providerKeyRequest.apiKey, providerKeyRequest.model, ""), sessionCookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("save provider=%s status=%d body=%s", providerKeyRequest.provider, response.Code, response.Body.String())
		}
	}

	expectedDefaults := managementTenantDefaultsTestResponse{
		Provider:          proxy.ProviderNameDeepSeek,
		Model:             proxy.ModelNameDeepSeekV4Flash,
		DictationProvider: proxy.ProviderNameOpenAI,
		DictationModel:    proxy.DefaultDictationModel,
	}
	validRequest := authenticatedJSONRequest(http.MethodPut, tenantPath+"/defaults", managementDefaultsRequestBody(t, expectedDefaults.Provider, expectedDefaults.Model, expectedDefaults.DictationProvider, expectedDefaults.DictationModel, ""), sessionCookie)
	validResponse := httptest.NewRecorder()
	router.ServeHTTP(validResponse, validRequest)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("save defaults status=%d body=%s", validResponse.Code, validResponse.Body.String())
	}
	if validResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("save defaults cache control=%q want=no-store", validResponse.Header().Get("Cache-Control"))
	}
	assertManagementProfileDefaults(t, router, sessionCookie, expectedDefaults)

	invalidRequests := []struct {
		name              string
		provider          string
		model             string
		dictationProvider string
		dictationModel    string
		expectedPair      string
	}{
		{
			name:              "text model belongs to another provider",
			provider:          proxy.ProviderNameDeepSeek,
			model:             proxy.ModelNameGPT41,
			dictationProvider: proxy.ProviderNameOpenAI,
			dictationModel:    proxy.DefaultDictationModel,
			expectedPair:      "endpoint=text provider=deepseek model=gpt-4.1",
		},
		{
			name:              "blank text model",
			provider:          proxy.ProviderNameDeepSeek,
			model:             "",
			dictationProvider: proxy.ProviderNameOpenAI,
			dictationModel:    proxy.DefaultDictationModel,
			expectedPair:      "endpoint=text provider=deepseek model=",
		},
		{
			name:              "unsupported dictation provider",
			provider:          proxy.ProviderNameDeepSeek,
			model:             proxy.ModelNameDeepSeekV4Flash,
			dictationProvider: proxy.ProviderNameDeepSeek,
			dictationModel:    proxy.ModelNameDeepSeekV4Flash,
			expectedPair:      "endpoint=dictation provider=deepseek model=deepseek-v4-flash",
		},
		{
			name:              "blank dictation model",
			provider:          proxy.ProviderNameDeepSeek,
			model:             proxy.ModelNameDeepSeekV4Flash,
			dictationProvider: proxy.ProviderNameOpenAI,
			dictationModel:    "",
			expectedPair:      "endpoint=dictation provider=openai model=",
		},
	}
	for _, invalidRequest := range invalidRequests {
		t.Run(invalidRequest.name, func(subTest *testing.T) {
			request := authenticatedJSONRequest(http.MethodPut, tenantPath+"/defaults", managementDefaultsRequestBody(subTest, invalidRequest.provider, invalidRequest.model, invalidRequest.dictationProvider, invalidRequest.dictationModel, ""), sessionCookie)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "managed_routing_defaults_invalid") || !strings.Contains(response.Body.String(), invalidRequest.expectedPair) {
				subTest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			assertManagementProfileDefaults(subTest, router, sessionCookie, expectedDefaults)
		})
	}
}

func TestManagementRoutingDefaultsFollowSavedProviderKeys(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if authorizationHeader := request.Header.Get("Authorization"); authorizationHeader != "Bearer "+testManagementDeepSeekKey {
			t.Fatalf("authorization=%q", authorizationHeader)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"keyed default ok"},"finish_reason":"stop"}]}`))
	}))
	defer upstreamServer.Close()

	router := newManagementRouter(t, proxy.Configuration{Endpoints: providerEndpoints(upstreamServer.URL, proxy.ProviderNameDeepSeek)})
	sessionCookie := managementSessionCookie(t, "tauth-keyed-routing-defaults-user")
	tenantPath := managementDefaultTenantTestPath(t, router, sessionCookie, "")
	assertManagementProfileDefaults(t, router, sessionCookie, managementTenantDefaultsTestResponse{})

	saveProviderKey := func(provider string, apiKey string, model string) *httptest.ResponseRecorder {
		request := authenticatedJSONRequest(
			http.MethodPut,
			tenantPath+"/provider-connections/"+provider,
			managementProviderKeyRequestBody(t, apiKey, model, ""),
			sessionCookie,
		)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	if response := saveProviderKey(proxy.ProviderNameDeepSeek, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash); response.Code != http.StatusOK {
		t.Fatalf("save text-only provider status=%d body=%s", response.Code, response.Body.String())
	}
	textOnlyDefaults := managementTenantDefaultsTestResponse{
		Provider: proxy.ProviderNameDeepSeek,
		Model:    proxy.ModelNameDeepSeekV4Flash,
	}
	assertManagementProfileDefaults(t, router, sessionCookie, textOnlyDefaults)

	unkeyedDefaultsRequest := authenticatedJSONRequest(
		http.MethodPut,
		tenantPath+"/defaults",
		managementDefaultsRequestBody(t, proxy.ProviderNameOpenAI, proxy.ModelNameGPT41, "", "", ""),
		sessionCookie,
	)
	unkeyedDefaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(unkeyedDefaultsResponse, unkeyedDefaultsRequest)
	if unkeyedDefaultsResponse.Code != http.StatusBadRequest || !strings.Contains(unkeyedDefaultsResponse.Body.String(), "provider_key_ineligible") {
		t.Fatalf("unkeyed defaults status=%d body=%s", unkeyedDefaultsResponse.Code, unkeyedDefaultsResponse.Body.String())
	}

	secretRequest := authenticatedJSONRequest(http.MethodPost, tenantPath+"/secrets", `{}`, sessionCookie)
	secretResponse := httptest.NewRecorder()
	router.ServeHTTP(secretResponse, secretRequest)
	if secretResponse.Code != http.StatusOK {
		t.Fatalf("generate secret status=%d body=%s", secretResponse.Code, secretResponse.Body.String())
	}
	var secretPayload struct {
		Secret string `json:"secret"`
	}
	if decodeError := json.Unmarshal(secretResponse.Body.Bytes(), &secretPayload); decodeError != nil {
		t.Fatalf("decode generated secret: %v", decodeError)
	}
	proxyRequest := httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secretPayload.Secret)+"&prompt=hello", nil)
	proxyResponse := httptest.NewRecorder()
	router.ServeHTTP(proxyResponse, proxyRequest)
	if proxyResponse.Code != http.StatusOK || strings.TrimSpace(proxyResponse.Body.String()) != "keyed default ok" {
		t.Fatalf("default proxy status=%d body=%s", proxyResponse.Code, proxyResponse.Body.String())
	}
	dictationBody := &bytes.Buffer{}
	dictationWriter := multipart.NewWriter(dictationBody)
	dictationFile, createDictationFileError := dictationWriter.CreateFormFile("audio", "recording.webm")
	if createDictationFileError != nil {
		t.Fatalf("create disabled dictation file: %v", createDictationFileError)
	}
	if _, writeDictationError := dictationFile.Write([]byte("audio")); writeDictationError != nil {
		t.Fatalf("write disabled dictation audio: %v", writeDictationError)
	}
	if closeDictationError := dictationWriter.Close(); closeDictationError != nil {
		t.Fatalf("close disabled dictation body: %v", closeDictationError)
	}
	dictationRequest := httptest.NewRequest(http.MethodPost, "/dictate?key="+url.QueryEscape(secretPayload.Secret), dictationBody)
	dictationRequest.Header.Set("Content-Type", dictationWriter.FormDataContentType())
	dictationResponse := httptest.NewRecorder()
	router.ServeHTTP(dictationResponse, dictationRequest)
	if dictationResponse.Code != http.StatusBadRequest || !strings.Contains(dictationResponse.Body.String(), "unknown provider") {
		t.Fatalf("disabled dictation status=%d body=%s", dictationResponse.Code, dictationResponse.Body.String())
	}

	if response := saveProviderKey(proxy.ProviderNameOpenAI, testManagementOpenAIKey, proxy.ModelNameGPT55); response.Code != http.StatusOK {
		t.Fatalf("save dictation provider status=%d body=%s", response.Code, response.Body.String())
	}
	assertManagementProfileDefaults(t, router, sessionCookie, managementTenantDefaultsTestResponse{
		Provider:          proxy.ProviderNameDeepSeek,
		Model:             proxy.ModelNameDeepSeekV4Flash,
		DictationProvider: proxy.ProviderNameOpenAI,
		DictationModel:    proxy.DefaultDictationModel,
	})
	if response := saveProviderKey(proxy.ProviderNameOpenAI, "", proxy.ModelNameGPT4oMini); response.Code != http.StatusOK {
		t.Fatalf("update inactive provider model status=%d body=%s", response.Code, response.Body.String())
	}
	assertManagementProfileDefaults(t, router, sessionCookie, managementTenantDefaultsTestResponse{
		Provider:          proxy.ProviderNameDeepSeek,
		Model:             proxy.ModelNameDeepSeekV4Flash,
		DictationProvider: proxy.ProviderNameOpenAI,
		DictationModel:    proxy.DefaultDictationModel,
	})

	openAIDefaultsRequest := authenticatedJSONRequest(
		http.MethodPut,
		tenantPath+"/defaults",
		managementDefaultsRequestBody(t, proxy.ProviderNameOpenAI, proxy.ModelNameGPT41, proxy.ProviderNameOpenAI, proxy.DefaultDictationModel, ""),
		sessionCookie,
	)
	openAIDefaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(openAIDefaultsResponse, openAIDefaultsRequest)
	if openAIDefaultsResponse.Code != http.StatusOK {
		t.Fatalf("select OpenAI defaults status=%d body=%s", openAIDefaultsResponse.Code, openAIDefaultsResponse.Body.String())
	}
	if response := saveProviderKey(proxy.ProviderNameOpenAI, "", proxy.ModelNameGPT55); response.Code != http.StatusOK {
		t.Fatalf("update active provider model status=%d body=%s", response.Code, response.Body.String())
	}
	assertManagementProfileDefaults(t, router, sessionCookie, managementTenantDefaultsTestResponse{
		Provider:          proxy.ProviderNameOpenAI,
		Model:             proxy.ModelNameGPT55,
		DictationProvider: proxy.ProviderNameOpenAI,
		DictationModel:    proxy.DefaultDictationModel,
	})
	reasoningDefaultsRequest := authenticatedJSONRequest(
		http.MethodPut,
		tenantPath+"/defaults",
		managementDefaultsRequestBodyWithReasoningEffort(t, proxy.ProviderNameOpenAI, proxy.ModelNameGPT5, proxy.ProviderNameOpenAI, proxy.DefaultDictationModel, "", "high"),
		sessionCookie,
	)
	reasoningDefaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(reasoningDefaultsResponse, reasoningDefaultsRequest)
	if reasoningDefaultsResponse.Code != http.StatusOK {
		t.Fatalf("select reasoning defaults status=%d body=%s", reasoningDefaultsResponse.Code, reasoningDefaultsResponse.Body.String())
	}
	if response := saveProviderKey(proxy.ProviderNameOpenAI, "", proxy.ModelNameGPT5Mini); response.Code != http.StatusOK {
		t.Fatalf("update active provider to reasoning-compatible model status=%d body=%s", response.Code, response.Body.String())
	}
	assertManagementProfileDefaults(t, router, sessionCookie, managementTenantDefaultsTestResponse{
		Provider:          proxy.ProviderNameOpenAI,
		Model:             proxy.ModelNameGPT5Mini,
		DictationProvider: proxy.ProviderNameOpenAI,
		DictationModel:    proxy.DefaultDictationModel,
		ReasoningEffort:   "high",
	})
	if response := saveProviderKey(proxy.ProviderNameOpenAI, "", proxy.ModelNameGPT41); response.Code != http.StatusOK {
		t.Fatalf("update active provider to model without reasoning status=%d body=%s", response.Code, response.Body.String())
	}
	assertManagementProfileDefaults(t, router, sessionCookie, managementTenantDefaultsTestResponse{
		Provider:          proxy.ProviderNameOpenAI,
		Model:             proxy.ModelNameGPT41,
		DictationProvider: proxy.ProviderNameOpenAI,
		DictationModel:    proxy.DefaultDictationModel,
	})
	overriddenDefaultsRequest := authenticatedJSONRequest(
		http.MethodPut,
		tenantPath+"/defaults",
		managementDefaultsRequestBody(t, proxy.ProviderNameOpenAI, proxy.ModelNameGPT4oMini, proxy.ProviderNameOpenAI, proxy.DefaultDictationModel, ""),
		sessionCookie,
	)
	overriddenDefaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(overriddenDefaultsResponse, overriddenDefaultsRequest)
	if overriddenDefaultsResponse.Code != http.StatusOK {
		t.Fatalf("override synchronized routing model status=%d body=%s", overriddenDefaultsResponse.Code, overriddenDefaultsResponse.Body.String())
	}
	unchangedProviderModelRequest := authenticatedJSONRequest(
		http.MethodPut,
		tenantPath+"/provider-connections/openai",
		managementProviderKeyRequestBody(t, "", proxy.ModelNameGPT41, "Use provider guidance."),
		sessionCookie,
	)
	unchangedProviderModelResponse := httptest.NewRecorder()
	router.ServeHTTP(unchangedProviderModelResponse, unchangedProviderModelRequest)
	if unchangedProviderModelResponse.Code != http.StatusOK {
		t.Fatalf("save provider prompt with unchanged model status=%d body=%s", unchangedProviderModelResponse.Code, unchangedProviderModelResponse.Body.String())
	}
	assertManagementProfileDefaults(t, router, sessionCookie, managementTenantDefaultsTestResponse{
		Provider:          proxy.ProviderNameOpenAI,
		Model:             proxy.ModelNameGPT4oMini,
		DictationProvider: proxy.ProviderNameOpenAI,
		DictationModel:    proxy.DefaultDictationModel,
	})
	removeOpenAIRequest := authenticatedJSONRequest(http.MethodDelete, tenantPath+"/provider-connections/openai", `{}`, sessionCookie)
	removeOpenAIResponse := httptest.NewRecorder()
	router.ServeHTTP(removeOpenAIResponse, removeOpenAIRequest)
	if removeOpenAIResponse.Code != http.StatusOK {
		t.Fatalf("remove default provider status=%d body=%s", removeOpenAIResponse.Code, removeOpenAIResponse.Body.String())
	}
	assertManagementProfileDefaults(t, router, sessionCookie, textOnlyDefaults)

	removeDeepSeekRequest := authenticatedJSONRequest(http.MethodDelete, tenantPath+"/provider-connections/deepseek", `{}`, sessionCookie)
	removeDeepSeekResponse := httptest.NewRecorder()
	router.ServeHTTP(removeDeepSeekResponse, removeDeepSeekRequest)
	if removeDeepSeekResponse.Code != http.StatusOK {
		t.Fatalf("remove final provider status=%d body=%s", removeDeepSeekResponse.Code, removeDeepSeekResponse.Body.String())
	}
	assertManagementProfileDefaults(t, router, sessionCookie, managementTenantDefaultsTestResponse{})
	waitForManagementRequestCount(t, router, sessionCookie, 2)
}

func TestManagementRoutingDefaultsRequireAnExactTextRouteReasoningEffort(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	sessionCookie := managementSessionCookie(t, "tauth-reasoning-effort-user")
	tenantPath := managementDefaultTenantTestPath(t, router, sessionCookie, "")
	for _, providerKeyRequest := range []struct {
		provider string
		apiKey   string
		model    string
	}{
		{provider: proxy.ProviderNameOpenAI, apiKey: testManagementOpenAIKey, model: proxy.ModelNameGPT5},
		{provider: proxy.ProviderNameDeepSeek, apiKey: testManagementDeepSeekKey, model: proxy.ModelNameDeepSeekV4Flash},
		{provider: proxy.ProviderNameMoonshot, apiKey: "sk-moonshot", model: proxy.ModelNameMoonshotKimiK3},
	} {
		request := authenticatedJSONRequest(http.MethodPut, tenantPath+"/provider-connections/"+providerKeyRequest.provider, managementProviderKeyRequestBody(t, providerKeyRequest.apiKey, providerKeyRequest.model, ""), sessionCookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("save provider=%s status=%d body=%s", providerKeyRequest.provider, response.Code, response.Body.String())
		}
	}

	saveReasoningDefault := func(provider string, model string, reasoningEffort string) *httptest.ResponseRecorder {
		request := authenticatedJSONRequest(
			http.MethodPut,
			tenantPath+"/defaults",
			managementDefaultsRequestBodyWithReasoningEffort(t, provider, model, proxy.ProviderNameOpenAI, proxy.DefaultDictationModel, "", reasoningEffort),
			sessionCookie,
		)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	gpt5Response := saveReasoningDefault(proxy.ProviderNameOpenAI, proxy.ModelNameGPT5, "high")
	if gpt5Response.Code != http.StatusOK {
		t.Fatalf("save GPT-5 reasoning defaults status=%d body=%s", gpt5Response.Code, gpt5Response.Body.String())
	}
	gpt5MiniResponse := saveReasoningDefault(proxy.ProviderNameOpenAI, proxy.ModelNameGPT5Mini, "high")
	if gpt5MiniResponse.Code != http.StatusOK {
		t.Fatalf("save GPT-5 mini reasoning defaults status=%d body=%s", gpt5MiniResponse.Code, gpt5MiniResponse.Body.String())
	}
	gpt56Response := saveReasoningDefault(proxy.ProviderNameOpenAI, proxy.ModelNameGPT56, "max")
	if gpt56Response.Code != http.StatusOK {
		t.Fatalf("save GPT-5.6 reasoning defaults status=%d body=%s", gpt56Response.Code, gpt56Response.Body.String())
	}
	kimiK3Response := saveReasoningDefault(proxy.ProviderNameMoonshot, proxy.ModelNameMoonshotKimiK3, "high")
	if kimiK3Response.Code != http.StatusOK {
		t.Fatalf("save Kimi K3 reasoning defaults status=%d body=%s", kimiK3Response.Code, kimiK3Response.Body.String())
	}
	profileRequest := httptest.NewRequest(http.MethodGet, tenantPath, nil)
	profileRequest.AddCookie(sessionCookie)
	profileResponse := httptest.NewRecorder()
	router.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%s", profileResponse.Code, profileResponse.Body.String())
	}
	var rawProfile map[string]json.RawMessage
	if decodeError := json.Unmarshal(profileResponse.Body.Bytes(), &rawProfile); decodeError != nil {
		t.Fatalf("decode raw capability profile: %v", decodeError)
	}
	if _, foundGlobalOptions := rawProfile["reasoning_effort_options"]; foundGlobalOptions {
		t.Fatalf("profile retains global reasoning effort options")
	}
	var profile struct {
		Tenant struct {
			Defaults struct {
				Provider        string `json:"provider"`
				Model           string `json:"model"`
				ReasoningEffort string `json:"reasoning_effort"`
			} `json:"defaults"`
		} `json:"tenant"`
		Providers []struct {
			ID              string          `json:"id"`
			ReasoningEffort json.RawMessage `json:"reasoning_effort"`
			TextModels      []struct {
				ID              string `json:"id"`
				ReasoningEffort *struct {
					Adapter string   `json:"adapter"`
					Efforts []string `json:"efforts"`
				} `json:"reasoning_effort"`
			} `json:"text_models"`
		} `json:"providers"`
	}
	if decodeError := json.Unmarshal(profileResponse.Body.Bytes(), &profile); decodeError != nil {
		t.Fatalf("decode capability profile: %v", decodeError)
	}
	if profile.Tenant.Defaults.Provider != proxy.ProviderNameMoonshot || profile.Tenant.Defaults.Model != proxy.ModelNameMoonshotKimiK3 || profile.Tenant.Defaults.ReasoningEffort != "high" {
		t.Fatalf("profile defaults=%+v", profile.Tenant.Defaults)
	}
	expectedModelEfforts := map[string][]string{
		proxy.ModelNameGPT5Mini: {"minimal", "low", "medium", "high"},
		proxy.ModelNameGPT5:     {"minimal", "low", "medium", "high"},
		proxy.ModelNameGPT55:    {"none", "low", "medium", "high", "xhigh"},
		proxy.ModelNameGPT56:    {"none", "low", "medium", "high", "xhigh", "max"},
		proxy.ModelNameGPT55Pro: {"medium", "high", "xhigh"},
	}
	matchedModelEfforts := map[string]bool{}
	matchedKimiK3 := false
	for _, provider := range profile.Providers {
		if provider.ID == proxy.ProviderNameOpenAI {
			if len(provider.ReasoningEffort) != 0 {
				t.Fatalf("OpenAI profile retains provider-level reasoning capability=%s", string(provider.ReasoningEffort))
			}
			for _, model := range provider.TextModels {
				expectedEfforts, required := expectedModelEfforts[model.ID]
				if required {
					if model.ReasoningEffort == nil || model.ReasoningEffort.Adapter != "openai_responses" || !reflect.DeepEqual(model.ReasoningEffort.Efforts, expectedEfforts) {
						t.Fatalf("model=%s reasoning capability=%+v want=%v", model.ID, model.ReasoningEffort, expectedEfforts)
					}
					matchedModelEfforts[model.ID] = true
				}
			}
		}
		if provider.ID == proxy.ProviderNameMoonshot {
			for _, model := range provider.TextModels {
				if model.ID == proxy.ModelNameMoonshotKimiK3 && model.ReasoningEffort != nil && model.ReasoningEffort.Adapter == proxy.CatalogProtocolOpenAIChatCompletions && reflect.DeepEqual(model.ReasoningEffort.Efforts, []string{"low", "high", "max"}) {
					matchedKimiK3 = true
				}
			}
		}
	}
	if len(matchedModelEfforts) != len(expectedModelEfforts) || !matchedKimiK3 {
		t.Fatalf("profile model capabilities=%v Kimi K3=%t", matchedModelEfforts, matchedKimiK3)
	}

	incompatibleResponse := saveReasoningDefault(proxy.ProviderNameOpenAI, proxy.ModelNameGPT5, "max")
	if incompatibleResponse.Code != http.StatusBadRequest || !strings.Contains(incompatibleResponse.Body.String(), "managed_routing_defaults_invalid") {
		t.Fatalf("incompatible GPT-5 reasoning effort status=%d body=%s", incompatibleResponse.Code, incompatibleResponse.Body.String())
	}
	unsupportedResponse := saveReasoningDefault(proxy.ProviderNameDeepSeek, proxy.ModelNameDeepSeekV4Flash, "high")
	if unsupportedResponse.Code != http.StatusBadRequest || !strings.Contains(unsupportedResponse.Body.String(), "managed_routing_defaults_invalid") {
		t.Fatalf("unsupported-route reasoning effort status=%d body=%s", unsupportedResponse.Code, unsupportedResponse.Body.String())
	}
	nonCanonicalResponse := saveReasoningDefault(proxy.ProviderNameOpenAI, proxy.ModelNameGPT56, " max ")
	if nonCanonicalResponse.Code != http.StatusBadRequest || !strings.Contains(nonCanonicalResponse.Body.String(), "managed_routing_defaults_invalid") {
		t.Fatalf("noncanonical reasoning effort status=%d body=%s", nonCanonicalResponse.Code, nonCanonicalResponse.Body.String())
	}
	assertManagementProfileDefaults(t, router, sessionCookie, managementTenantDefaultsTestResponse{
		Provider:          proxy.ProviderNameMoonshot,
		Model:             proxy.ModelNameMoonshotKimiK3,
		DictationProvider: proxy.ProviderNameOpenAI,
		DictationModel:    proxy.DefaultDictationModel,
		ReasoningEffort:   "high",
	})
}

func TestManagementRoutingDefaultsRequireExplicitReasoningEffort(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	sessionCookie := managementSessionCookie(t, "tauth-routing-defaults-missing-reasoning-effort")
	tenantPath := managementDefaultTenantTestPath(t, router, sessionCookie, "")
	request := authenticatedJSONRequest(
		http.MethodPut,
		tenantPath+"/defaults",
		`{"provider":"openai","model":"gpt-4.1","dictation_provider":"openai","dictation_model":"gpt-4o-mini-transcribe","system_prompt":""}`,
		sessionCookie,
	)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "field=reasoning_effort") {
		t.Fatalf("missing reasoning effort status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestManagementProfileOmitsReasoningCapabilitiesWithoutModelDeclarations(t *testing.T) {
	catalogs := testfixtures.ModelCatalog(t)
	for offeringIndex := range catalogs.Offerings {
		if catalogs.Offerings[offeringIndex].Provider == proxy.ProviderNameOpenAI && slices.Contains(catalogs.Offerings[offeringIndex].Operations, proxy.ModelOperationText) {
			catalogs.Offerings[offeringIndex].ReasoningEffort = nil
		}
	}
	router := newManagementRouter(t, proxy.Configuration{ModelCatalog: catalogs})
	sessionCookie := managementSessionCookie(t, "tauth-profile-no-reasoning-effort")
	request := httptest.NewRequest(http.MethodGet, managementDefaultTenantTestPath(t, router, sessionCookie, ""), nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%s", response.Code, response.Body.String())
	}
	var profile map[string]json.RawMessage
	if decodeError := json.Unmarshal(response.Body.Bytes(), &profile); decodeError != nil {
		t.Fatalf("decode profile: %v", decodeError)
	}
	if _, foundGlobalOptions := profile["reasoning_effort_options"]; foundGlobalOptions {
		t.Fatalf("profile retains global reasoning effort options")
	}
	var providers []map[string]json.RawMessage
	if decodeError := json.Unmarshal(profile["providers"], &providers); decodeError != nil {
		t.Fatalf("decode profile providers: %v", decodeError)
	}
	for _, provider := range providers {
		if _, foundProviderCapability := provider["reasoning_effort"]; foundProviderCapability {
			t.Fatalf("provider retains reasoning capability=%s", string(provider["reasoning_effort"]))
		}
	}
}

func TestManagementRoutingDefaultsRequireSavedProviderKeys(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	noKeyCookie := managementSessionCookie(t, "tauth-routing-defaults-no-key")
	noKeyTenantPath := managementDefaultTenantTestPath(t, router, noKeyCookie, "")
	noKeyRequest := authenticatedJSONRequest(http.MethodPut, noKeyTenantPath+"/defaults", managementDefaultsRequestBody(t, proxy.ProviderNameOpenAI, proxy.ModelNameGPT41, proxy.ProviderNameOpenAI, proxy.DefaultDictationModel, ""), noKeyCookie)
	noKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(noKeyResponse, noKeyRequest)
	if noKeyResponse.Code != http.StatusBadRequest || !strings.Contains(noKeyResponse.Body.String(), "management_defaults_invalid") {
		t.Fatalf("missing text provider key status=%d body=%s", noKeyResponse.Code, noKeyResponse.Body.String())
	}

	textOnlyCookie := managementSessionCookie(t, "tauth-routing-defaults-text-only")
	textOnlyTenantPath := managementDefaultTenantTestPath(t, router, textOnlyCookie, "")
	saveDeepSeekKeyRequest := authenticatedJSONRequest(http.MethodPut, textOnlyTenantPath+"/provider-connections/deepseek", managementProviderKeyRequestBody(t, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""), textOnlyCookie)
	saveDeepSeekKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveDeepSeekKeyResponse, saveDeepSeekKeyRequest)
	if saveDeepSeekKeyResponse.Code != http.StatusOK {
		t.Fatalf("save deepseek key status=%d body=%s", saveDeepSeekKeyResponse.Code, saveDeepSeekKeyResponse.Body.String())
	}
	missingDictationKeyRequest := authenticatedJSONRequest(http.MethodPut, textOnlyTenantPath+"/defaults", managementDefaultsRequestBody(t, proxy.ProviderNameDeepSeek, proxy.ModelNameDeepSeekV4Flash, proxy.ProviderNameOpenAI, proxy.DefaultDictationModel, ""), textOnlyCookie)
	missingDictationKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(missingDictationKeyResponse, missingDictationKeyRequest)
	if missingDictationKeyResponse.Code != http.StatusBadRequest || !strings.Contains(missingDictationKeyResponse.Body.String(), "management_defaults_invalid") {
		t.Fatalf("missing dictation provider key status=%d body=%s", missingDictationKeyResponse.Code, missingDictationKeyResponse.Body.String())
	}
}

func TestManagementRoutingDefaultsDoNotRequireConfiguredStaticDefault(t *testing.T) {
	configuration := withModelCatalog(t, managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "managed-tenants.db")))
	for offeringIndex := range configuration.ModelCatalog.Offerings {
		offering := &configuration.ModelCatalog.Offerings[offeringIndex]
		if offering.Provider != proxy.ProviderNameOpenAI || !slices.Contains(offering.Operations, proxy.ModelOperationText) {
			continue
		}
		offering.DefaultOperations = nil
		if offering.Model == proxy.ModelNameGPT4oMini {
			offering.DefaultOperations = []string{proxy.ModelOperationText}
		}
	}
	if _, buildError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar()); buildError != nil {
		t.Fatalf("BuildRouter error=%v", buildError)
	}
}

func TestManagementDatabasePersistenceAndOpenFailures(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "managed-tenants.db")
	requestedModels := []string{}
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var requestPayload struct {
			Model string `json:"model"`
		}
		if decodeError := json.NewDecoder(request.Body).Decode(&requestPayload); decodeError != nil {
			t.Fatalf("decode persisted upstream request: %v", decodeError)
		}
		requestedModels = append(requestedModels, requestPayload.Model)
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"persisted ok"},"finish_reason":"stop"}]}`))
	}))
	defer upstreamServer.Close()

	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{Endpoints: providerEndpoints(upstreamServer.URL, proxy.ProviderNameDeepSeek)}, databasePath)
	sessionCookie := managementSessionCookie(t, "tauth-persisted-user")
	tenantPath := managementDefaultTenantTestPath(t, router, sessionCookie, "")
	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, tenantPath+"/provider-connections/deepseek", managementProviderKeyRequestBody(t, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""), sessionCookie)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}
	saveOpenAIKeyRequest := authenticatedJSONRequest(http.MethodPut, tenantPath+"/provider-connections/openai", managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT41, ""), sessionCookie)
	saveOpenAIKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveOpenAIKeyResponse, saveOpenAIKeyRequest)
	if saveOpenAIKeyResponse.Code != http.StatusOK {
		t.Fatalf("save openai key status=%d body=%s", saveOpenAIKeyResponse.Code, saveOpenAIKeyResponse.Body.String())
	}
	defaultsBody := `{"provider":"deepseek","model":"` + proxy.ModelNameDeepSeekV4Flash + `","dictation_provider":"openai","dictation_model":"` + proxy.DefaultDictationModel + `","system_prompt":"","reasoning_effort":""}`
	defaultsRequest := authenticatedJSONRequest(http.MethodPut, tenantPath+"/defaults", defaultsBody, sessionCookie)
	defaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(defaultsResponse, defaultsRequest)
	if defaultsResponse.Code != http.StatusOK {
		t.Fatalf("defaults status=%d body=%s", defaultsResponse.Code, defaultsResponse.Body.String())
	}
	secretRequest := authenticatedJSONRequest(http.MethodPost, tenantPath+"/secrets", `{}`, sessionCookie)
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

	reloadedRouter := newManagementRouterWithDatabasePath(t, proxy.Configuration{Endpoints: providerEndpoints(upstreamServer.URL, proxy.ProviderNameDeepSeek)}, databasePath)
	reloadedRequest := httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secretPayload.Secret)+"&prompt=hello", nil)
	reloadedResponse := httptest.NewRecorder()
	reloadedRouter.ServeHTTP(reloadedResponse, reloadedRequest)
	if reloadedResponse.Code != http.StatusOK || strings.TrimSpace(reloadedResponse.Body.String()) != "persisted ok" {
		t.Fatalf("reloaded status=%d body=%s", reloadedResponse.Code, reloadedResponse.Body.String())
	}
	if len(requestedModels) != 1 || requestedModels[0] != proxy.ModelNameDeepSeekV4Flash {
		t.Fatalf("persisted default models=%v want=[%s]", requestedModels, proxy.ModelNameDeepSeekV4Flash)
	}
	waitForManagementRequestCount(t, reloadedRouter, sessionCookie, 1)

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

func TestManagementStartupRejectsInvalidPersistedRoutingDefaults(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "managed-tenants.db")
	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath)
	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	sessionCookie := managementSessionCookie(t, "tauth-invalid-persisted-defaults")
	tenantID := managementDefaultTenantTestID(t, router, sessionCookie)
	saveKeyRequest := authenticatedJSONRequest(
		http.MethodPut,
		"/api/management/tenants/"+url.PathEscape(tenantID)+"/provider-connections/openai",
		managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT41, ""),
		sessionCookie,
	)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save provider key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}

	fixtureDatabase := openManagedFixtureDatabase(t, databasePath)
	updateResult := fixtureDatabase.
		Table("managed_tenant_records").
		Where("tenant_id = ?", tenantID).
		Updates(map[string]interface{}{
			"default_provider": proxy.ProviderNameOpenAI,
			"default_model":    "missing-model",
		})
	if updateResult.Error != nil || updateResult.RowsAffected != 1 {
		t.Fatalf("mutate persisted defaults rows=%d error=%v", updateResult.RowsAffected, updateResult.Error)
	}
	sqlDatabase, sqlDatabaseError := fixtureDatabase.DB()
	if sqlDatabaseError != nil {
		t.Fatalf("resolve fixture database: %v", sqlDatabaseError)
	}
	if closeError := sqlDatabase.Close(); closeError != nil {
		t.Fatalf("close fixture database: %v", closeError)
	}

	_, reopenError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	for _, expectedFragment := range []string{
		"operation=validate table=managed_tenant_records",
		"owner=tauth-invalid-persisted-defaults",
		"tenant=" + tenantID,
		"endpoint=text provider=openai model=missing-model",
	} {
		if reopenError == nil || !strings.Contains(reopenError.Error(), expectedFragment) {
			t.Fatalf("BuildRouter error=%v want contains %q", reopenError, expectedFragment)
		}
	}
}

func TestManagementConfigurationValidationRequiresBackendAuthFields(t *testing.T) {
	authFieldTestCases := []struct {
		name          string
		clearField    func(*proxy.ManagementConfiguration)
		expectedError string
	}{
		{
			name: "signing key",
			clearField: func(configuration *proxy.ManagementConfiguration) {
				configuration.JWTSigningKey = " "
			},
			expectedError: "session.validator.missing_signing_key",
		},
		{
			name: "session cookie name",
			clearField: func(configuration *proxy.ManagementConfiguration) {
				configuration.SessionCookieName = " "
			},
			expectedError: "management.session_cookie_name",
		},
		{
			name: "session path",
			clearField: func(configuration *proxy.ManagementConfiguration) {
				configuration.SessionPath = " "
			},
			expectedError: "management.session_path",
		},
	}
	for _, testCase := range authFieldTestCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(subTest.TempDir(), "store.db"))
			testCase.clearField(&configuration.Management)
			_, buildError := buildRouterWithCatalogs(subTest, configuration, zap.NewNop().Sugar())
			if buildError == nil || !strings.Contains(buildError.Error(), testCase.expectedError) {
				subTest.Fatalf("BuildRouter error=%v want contains %q", buildError, testCase.expectedError)
			}
		})
	}

	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "store.db"))
	configuration.Management.TAuthTenantID = " "
	_, buildError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.tauth_tenant_id") {
		t.Fatalf("BuildRouter error=%v want missing management.tauth_tenant_id", buildError)
	}

	configuration = managementConfigurationWithDatabasePath(proxy.Configuration{}, "")
	_, buildError = buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "management.database_path") {
		t.Fatalf("BuildRouter error=%v want missing management.database_path", buildError)
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

}

func TestManagementOpensConfiguredDatabasePath(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "managed-tenants.db")
	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath)
	configuration.Management.DatabaseDialector = nil
	router, buildError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	sessionCookie := managementSessionCookie(t, "tauth-sqlite-user")
	profileRequest := authenticatedJSONRequest(http.MethodGet, managementDefaultTenantTestPath(t, router, sessionCookie, ""), "", sessionCookie)
	profileResponse := httptest.NewRecorder()
	router.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%s", profileResponse.Code, profileResponse.Body.String())
	}
}

func TestManagedAuthenticationReadsThroughConcurrentSQLiteWriter(t *testing.T) {
	upstreamReached := make(chan struct{})
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestBody, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Errorf("read concurrent writer upstream request: %v", readError)
			responseWriter.WriteHeader(http.StatusInternalServerError)
			return
		}
		if bytes.Contains(requestBody, []byte(testProviderKeyVerificationPrompt)) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"id":"verification","status":"completed"}`))
			return
		}
		close(upstreamReached)
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"id":"response-id","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"text","text":"wal reader ok"}]}]}`))
	}))
	defer upstreamServer.Close()

	databasePath := filepath.Join(t.TempDir(), "managed-tenants.db")
	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{
		Endpoints: providerEndpoints(upstreamServer.URL, proxy.ProviderNameOpenAI),
	}, databasePath)
	configuration.Management.DatabaseDialector = nil
	router, buildError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	const ownerID = "wal-reader-owner"
	sessionCookie := managementSessionCookie(t, ownerID)
	account := requestManagementAccount(t, router, sessionCookie)
	tenantID := account.Tenants[0].ID
	saveManagementProviderKey(t, router, sessionCookie, tenantID, testManagementOpenAIKey, proxy.ModelNameGPT41, "")
	saveManagementDefaults(t, router, sessionCookie, tenantID, proxy.ModelNameGPT41, "")
	secret := generateManagementTenantSecret(t, router, sessionCookie, tenantID)

	writerDatabase, writerOpenError := gorm.Open(
		sqlite.Open(databasePath+"?_pragma=busy_timeout(5000)&_txlock=exclusive"),
		&gorm.Config{},
	)
	if writerOpenError != nil {
		t.Fatalf("open concurrent writer: %v", writerOpenError)
	}
	writerSQLDatabase, sqlDatabaseError := writerDatabase.DB()
	if sqlDatabaseError != nil {
		t.Fatalf("resolve concurrent writer: %v", sqlDatabaseError)
	}
	defer writerSQLDatabase.Close()

	writerTransaction := writerDatabase.Begin()
	if writerTransaction.Error != nil {
		t.Fatalf("begin concurrent writer: %v", writerTransaction.Error)
	}
	writerTransactionOpen := true
	defer func() {
		if writerTransactionOpen {
			_ = writerTransaction.Rollback().Error
		}
	}()
	updateResult := writerTransaction.
		Table("managed_user_records").
		Where("user_id = ?", ownerID).
		Update("user_display_name", "Writer in progress")
	if updateResult.Error != nil || updateResult.RowsAffected != 1 {
		t.Fatalf("hold concurrent writer rows=%d error=%v", updateResult.RowsAffected, updateResult.Error)
	}

	responseDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		request := httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secret)+"&prompt=hello", nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		responseDone <- response
	}()

	select {
	case <-upstreamReached:
	case <-time.After(time.Second):
		t.Fatal("managed authentication did not reach the provider while a SQLite writer was active")
	}
	if commitError := writerTransaction.Commit().Error; commitError != nil {
		t.Fatalf("commit concurrent writer: %v", commitError)
	}
	writerTransactionOpen = false

	select {
	case response := <-responseDone:
		if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "wal reader ok" {
			t.Fatalf("proxy status=%d body=%q", response.Code, response.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("proxy request did not finish after the concurrent writer committed")
	}
	waitForManagementRequestCount(t, router, sessionCookie, 1)
}

func TestManagementProfileListsCurrentCatalogModels(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	sessionCookie := managementSessionCookie(t, "tauth-current-catalog-user")
	profileRequest := authenticatedJSONRequest(http.MethodGet, managementDefaultTenantTestPath(t, router, sessionCookie, ""), "", sessionCookie)
	profileResponse := httptest.NewRecorder()
	router.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%s", profileResponse.Code, profileResponse.Body.String())
	}

	var profilePayload struct {
		Providers []struct {
			ID               string   `json:"id"`
			Label            string   `json:"label"`
			Aliases          []string `json:"aliases"`
			TextDefaultModel string   `json:"text_default_model"`
			TextModels       []struct {
				ID string `json:"id"`
			} `json:"text_models"`
		} `json:"providers"`
	}
	if decodeError := json.Unmarshal(profileResponse.Body.Bytes(), &profilePayload); decodeError != nil {
		t.Fatalf("decode profile: %v", decodeError)
	}
	modelsByProvider := map[string][]string{}
	textDefaultsByProvider := map[string]string{}
	labelsByProvider := map[string]string{}
	aliasesByProvider := map[string][]string{}
	for _, provider := range profilePayload.Providers {
		models := make([]string, 0, len(provider.TextModels))
		for _, model := range provider.TextModels {
			models = append(models, model.ID)
		}
		modelsByProvider[provider.ID] = models
		textDefaultsByProvider[provider.ID] = provider.TextDefaultModel
		labelsByProvider[provider.ID] = provider.Label
		aliasesByProvider[provider.ID] = provider.Aliases
	}
	if labelsByProvider[proxy.ProviderNameZAI] != "Z.AI" || len(aliasesByProvider[proxy.ProviderNameZAI]) != 0 {
		t.Fatalf("Z.AI profile label=%q aliases=%v", labelsByProvider[proxy.ProviderNameZAI], aliasesByProvider[proxy.ProviderNameZAI])
	}
	for _, retiredProvider := range []string{"zhipu", "glm"} {
		if _, found := modelsByProvider[retiredProvider]; found {
			t.Fatalf("profile exposes retired provider=%s", retiredProvider)
		}
	}
	expectedModels := map[string][]string{
		proxy.ProviderNameOpenAI:    {"gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"},
		proxy.ProviderNameDashScope: {proxy.ModelNameDashScopeQwenPlus, proxy.ModelNameDashScopeQwen36Flash, proxy.ModelNameDashScopeQwen37Max, proxy.ModelNameDashScopeQwen37Plus},
		proxy.ProviderNameMoonshot:  {proxy.ModelNameMoonshotKimiK26, proxy.ModelNameMoonshotKimiK27Code, proxy.ModelNameMoonshotKimiK27CodeHighSpeed, proxy.ModelNameMoonshotKimiK3},
		proxy.ProviderNameMiniMax: {
			proxy.ModelNameMiniMaxM2,
			proxy.ModelNameMiniMaxM21,
			proxy.ModelNameMiniMaxM21HighSpeed,
			proxy.ModelNameMiniMaxM25,
			proxy.ModelNameMiniMaxM25HighSpeed,
			proxy.ModelNameMiniMaxM27,
			proxy.ModelNameMiniMaxM27HighSpeed,
		},
		proxy.ProviderNameZAI:       {"glm-5.2"},
		proxy.ProviderNameGemini:    {"gemini-3-flash-preview", proxy.ModelNameGemini35Flash},
		proxy.ProviderNameAnthropic: {"claude-fable-5", "claude-sonnet-5"},
		proxy.ProviderNameXAI:       {"grok-4.5", "grok-4.20-0309-reasoning", "grok-4.20-0309-non-reasoning"},
	}
	for providerIdentifier, expectedProviderModels := range expectedModels {
		configuredModels, configured := modelsByProvider[providerIdentifier]
		if !configured {
			t.Fatalf("profile missing provider=%s", providerIdentifier)
		}
		for _, expectedModel := range expectedProviderModels {
			found := false
			for _, configuredModel := range configuredModels {
				if configuredModel == expectedModel {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("profile provider=%s models=%v missing=%s", providerIdentifier, configuredModels, expectedModel)
			}
		}
	}
	configuredMoonshotModels := modelsByProvider[proxy.ProviderNameMoonshot]
	if !reflect.DeepEqual(configuredMoonshotModels, expectedModels[proxy.ProviderNameMoonshot]) {
		t.Fatalf("profile provider=%s models=%v want=%v", proxy.ProviderNameMoonshot, configuredMoonshotModels, expectedModels[proxy.ProviderNameMoonshot])
	}
	if textDefaultsByProvider[proxy.ProviderNameMoonshot] != proxy.ModelNameMoonshotKimiK26 {
		t.Fatalf("profile provider=%s default=%s want=%s", proxy.ProviderNameMoonshot, textDefaultsByProvider[proxy.ProviderNameMoonshot], proxy.ModelNameMoonshotKimiK26)
	}
	if configuredDashScopeModels := modelsByProvider[proxy.ProviderNameDashScope]; !reflect.DeepEqual(configuredDashScopeModels, expectedModels[proxy.ProviderNameDashScope]) {
		t.Fatalf("profile provider=%s models=%v want=%v", proxy.ProviderNameDashScope, configuredDashScopeModels, expectedModels[proxy.ProviderNameDashScope])
	}
	if textDefaultsByProvider[proxy.ProviderNameDashScope] != proxy.ModelNameDashScopeQwenPlus {
		t.Fatalf("profile provider=%s default=%s want=%s", proxy.ProviderNameDashScope, textDefaultsByProvider[proxy.ProviderNameDashScope], proxy.ModelNameDashScopeQwenPlus)
	}
	if configuredGeminiModels := modelsByProvider[proxy.ProviderNameGemini]; !reflect.DeepEqual(configuredGeminiModels, expectedModels[proxy.ProviderNameGemini]) {
		t.Fatalf("profile provider=%s models=%v want=%v", proxy.ProviderNameGemini, configuredGeminiModels, expectedModels[proxy.ProviderNameGemini])
	}
	if configuredMiniMaxModels := modelsByProvider[proxy.ProviderNameMiniMax]; !reflect.DeepEqual(configuredMiniMaxModels, expectedModels[proxy.ProviderNameMiniMax]) {
		t.Fatalf("profile provider=%s models=%v want=%v", proxy.ProviderNameMiniMax, configuredMiniMaxModels, expectedModels[proxy.ProviderNameMiniMax])
	}
	if textDefaultsByProvider[proxy.ProviderNameMiniMax] != proxy.ModelNameMiniMaxM27 {
		t.Fatalf("profile provider=%s default=%s want=%s", proxy.ProviderNameMiniMax, textDefaultsByProvider[proxy.ProviderNameMiniMax], proxy.ModelNameMiniMaxM27)
	}
}

func TestManagementGeneratedSecretSupportsDictationAndRejectsMultipartProviderKeys(t *testing.T) {
	var capturedAuthorization string
	upstreamRequestCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		upstreamRequestCount++
		capturedAuthorization = request.Header.Get("Authorization")
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"text":"managed dictation ok"}`))
	}))
	defer upstreamServer.Close()

	router := newManagementRouter(t, proxy.Configuration{
		Endpoints: providerEndpointOverrides(nil, map[string]map[string]string{
			proxy.ProviderNameOpenAI: {"dictation": upstreamServer.URL},
		}),
	})
	sessionCookie := managementSessionCookie(t, "tauth-dictation-user")
	tenantPath := managementDefaultTenantTestPath(t, router, sessionCookie, "")
	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, tenantPath+"/provider-connections/openai", managementProviderKeyRequestBody(t, "sk-user-openai", proxy.ModelNameGPT41, ""), sessionCookie)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}
	secretRequest := authenticatedJSONRequest(http.MethodPost, tenantPath+"/secrets", `{}`, sessionCookie)
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

	for _, credentialField := range []string{"openai_api_key", "zai_api_key", "zhipu_api_key", "glm_api_key", ""} {
		t.Run(credentialField, func(subTest *testing.T) {
			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			if credentialField != "" {
				if writeError := writer.WriteField(credentialField, "sk-client"); writeError != nil {
					subTest.Fatalf("write provider key field: %v", writeError)
				}
			}
			filePart, createError := writer.CreateFormFile("audio", "recording.webm")
			if createError != nil {
				subTest.Fatalf("CreateFormFile error: %v", createError)
			}
			if _, writeError := filePart.Write([]byte("audio")); writeError != nil {
				subTest.Fatalf("write audio: %v", writeError)
			}
			if closeError := writer.Close(); closeError != nil {
				subTest.Fatalf("close multipart: %v", closeError)
			}
			request := httptest.NewRequest(http.MethodPost, "/dictate?key="+url.QueryEscape(secretPayload.Secret), body)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if credentialField != "" {
				if response.Code != http.StatusBadRequest || strings.TrimSpace(response.Body.String()) != "client provider API keys are not accepted" {
					subTest.Fatalf("field=%s status=%d body=%q", credentialField, response.Code, response.Body.String())
				}
				return
			}
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "managed dictation ok") {
				subTest.Fatalf("dictation status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	if upstreamRequestCount != 1 || capturedAuthorization != "Bearer sk-user-openai" {
		t.Fatalf("upstream requests=%d authorization=%q", upstreamRequestCount, capturedAuthorization)
	}
	waitForManagementRequestCount(t, router, sessionCookie, 5)
}

func TestManagementUsageSummaryRecordsManagedProxyRequests(t *testing.T) {
	chatServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			t.Fatalf("chat path=%s want=/chat/completions", request.URL.Path)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"managed usage ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":6,"total_tokens":10}}`))
	}))
	defer chatServer.Close()
	dictationServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		http.Error(responseWriter, "dictation unavailable", http.StatusBadGateway)
	}))
	defer dictationServer.Close()

	router := newManagementRouter(t, proxy.Configuration{
		Endpoints: providerEndpointOverrides(
			map[string]string{proxy.ProviderNameDeepSeek: chatServer.URL},
			map[string]map[string]string{proxy.ProviderNameOpenAI: {"dictation": dictationServer.URL}},
		),
	})
	userOneCookie := managementSessionCookie(t, "usage-user-one")
	userTwoCookie := managementSessionCookie(t, "usage-user-two")
	userOneTenantPath := managementDefaultTenantTestPath(t, router, userOneCookie, "")

	emptyUsage := requestManagementUsage(t, router, userOneCookie, "30d")
	if emptyUsage.Interval != "30d" || emptyUsage.BucketUnit != "day" || len(emptyUsage.Buckets) != 30 || emptyUsage.Totals.Requests != 0 {
		t.Fatalf("empty usage=%+v buckets=%d", emptyUsage, len(emptyUsage.Buckets))
	}

	saveDeepSeekKeyRequest := authenticatedJSONRequest(http.MethodPut, userOneTenantPath+"/provider-connections/deepseek", managementProviderKeyRequestBody(t, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""), userOneCookie)
	saveDeepSeekKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveDeepSeekKeyResponse, saveDeepSeekKeyRequest)
	if saveDeepSeekKeyResponse.Code != http.StatusOK {
		t.Fatalf("save deepseek key status=%d body=%s", saveDeepSeekKeyResponse.Code, saveDeepSeekKeyResponse.Body.String())
	}
	saveOpenAIKeyRequest := authenticatedJSONRequest(http.MethodPut, userOneTenantPath+"/provider-connections/openai", managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT41, ""), userOneCookie)
	saveOpenAIKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveOpenAIKeyResponse, saveOpenAIKeyRequest)
	if saveOpenAIKeyResponse.Code != http.StatusOK {
		t.Fatalf("save openai key status=%d body=%s", saveOpenAIKeyResponse.Code, saveOpenAIKeyResponse.Body.String())
	}
	defaultsBody := `{"provider":"deepseek","model":"` + proxy.ModelNameDeepSeekV4Flash + `","dictation_provider":"openai","dictation_model":"` + proxy.DefaultDictationModel + `","system_prompt":"","reasoning_effort":""}`
	defaultsRequest := authenticatedJSONRequest(http.MethodPut, userOneTenantPath+"/defaults", defaultsBody, userOneCookie)
	defaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(defaultsResponse, defaultsRequest)
	if defaultsResponse.Code != http.StatusOK {
		t.Fatalf("defaults status=%d body=%s", defaultsResponse.Code, defaultsResponse.Body.String())
	}
	secretRequest := authenticatedJSONRequest(http.MethodPost, userOneTenantPath+"/secrets", `{}`, userOneCookie)
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

	usage := waitForManagementValue(t, func() managementUsageTestResponse {
		return requestManagementUsage(t, router, userOneCookie, "30d")
	}, func(payload managementUsageTestResponse) bool {
		return payload.Totals.Requests == 5
	})
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
	if isolatedUsage := requestManagementUsage(t, router, userTwoCookie, "all"); isolatedUsage.Totals.Requests != 0 || len(isolatedUsage.Buckets) != 0 {
		t.Fatalf("user two usage leaked: %+v", isolatedUsage.Totals)
	}

	for _, intervalExpectation := range []struct {
		interval    string
		bucketUnit  string
		bucketCount int
	}{
		{interval: "all", bucketUnit: "day", bucketCount: 1},
		{interval: "30d", bucketUnit: "day", bucketCount: 30},
		{interval: "7d", bucketUnit: "day", bucketCount: 7},
		{interval: "1d", bucketUnit: "hour", bucketCount: 24},
	} {
		intervalUsage := requestManagementUsage(t, router, userOneCookie, intervalExpectation.interval)
		if intervalUsage.Interval != intervalExpectation.interval || intervalUsage.BucketUnit != intervalExpectation.bucketUnit || len(intervalUsage.Buckets) != intervalExpectation.bucketCount || intervalUsage.Totals.Requests != usage.Totals.Requests {
			t.Fatalf("interval=%s usage=%+v buckets=%d", intervalExpectation.interval, intervalUsage, len(intervalUsage.Buckets))
		}
	}

	for _, invalidPath := range []string{
		"/api/management/usage",
		"/api/management/usage?interval=unknown",
		"/api/management/usage?interval=1d&interval=7d",
		userOneTenantPath + "/usage",
		userOneTenantPath + "/usage?interval=unknown",
		userOneTenantPath + "/usage?interval=1d&interval=7d",
	} {
		invalidRequest := httptest.NewRequest(http.MethodGet, invalidPath, nil)
		invalidRequest.AddCookie(userOneCookie)
		invalidResponse := httptest.NewRecorder()
		router.ServeHTTP(invalidResponse, invalidRequest)
		if invalidResponse.Code != http.StatusBadRequest {
			t.Fatalf("usage path=%s status=%d body=%s", invalidPath, invalidResponse.Code, invalidResponse.Body.String())
		}
	}
}

func TestManagementValidationFailureUsageUsesSelectedProviderDefaultModel(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	sessionCookie := managementSessionCookie(t, "usage-provider-default-user")
	tenantPath := managementDefaultTenantTestPath(t, router, sessionCookie, "")

	for _, providerKey := range []struct {
		provider string
		apiKey   string
		model    string
	}{
		{provider: proxy.ProviderNameOpenAI, apiKey: testManagementOpenAIKey, model: proxy.ModelNameGPT41},
		{provider: proxy.ProviderNameGemini, apiKey: testGeminiKey, model: proxy.ModelNameGemini35Flash},
	} {
		request := authenticatedJSONRequest(
			http.MethodPut,
			tenantPath+"/provider-connections/"+providerKey.provider,
			managementProviderKeyRequestBody(t, providerKey.apiKey, providerKey.model, ""),
			sessionCookie,
		)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("save provider=%s status=%d body=%s", providerKey.provider, response.Code, response.Body.String())
		}
	}

	secretRequest := authenticatedJSONRequest(http.MethodPost, tenantPath+"/secrets", `{}`, sessionCookie)
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

	requestCases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/"},
		{method: http.MethodPost, path: "/", body: `{"prompt":"hello","max_tokens":0}`},
		{method: http.MethodPost, path: "/v2", body: `{"messages":[{"role":"user","content":"hello"}],"max_tokens":0}`},
	}
	for _, requestCase := range requestCases {
		query := url.Values{
			"key":      []string{secretPayload.Secret},
			"provider": []string{proxy.ProviderNameGemini},
		}
		if requestCase.method == http.MethodGet {
			query.Set("prompt", "hello")
			query.Set("max_tokens", "0")
		}
		request := httptest.NewRequest(
			requestCase.method,
			requestCase.path+"?"+query.Encode(),
			strings.NewReader(requestCase.body),
		)
		if requestCase.method == http.MethodPost {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("method=%s path=%s status=%d body=%s", requestCase.method, requestCase.path, response.Code, response.Body.String())
		}
	}

	usage := waitForManagementValue(t, func() managementUsageTestResponse {
		return requestManagementUsage(t, router, sessionCookie, "30d")
	}, func(payload managementUsageTestResponse) bool {
		return payload.Totals.Requests == len(requestCases)
	})
	if len(usage.Providers) != 1 || usage.Providers[0].Provider != proxy.ProviderNameGemini || usage.Providers[0].Data.Requests != len(requestCases) {
		t.Fatalf("providers=%+v", usage.Providers)
	}
	if len(usage.Models) != 1 || usage.Models[0].Provider != proxy.ProviderNameGemini || usage.Models[0].Model != proxy.ModelNameGemini35Flash || usage.Models[0].Data.Requests != len(requestCases) {
		t.Fatalf("models=%+v", usage.Models)
	}
}

func TestManagementUnconfiguredProviderUsageUsesCatalogDefaultModel(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	sessionCookie := managementSessionCookie(t, "usage-catalog-default-user")
	tenantPath := managementDefaultTenantTestPath(t, router, sessionCookie, "")

	providerRequest := authenticatedJSONRequest(
		http.MethodPut,
		tenantPath+"/provider-connections/"+proxy.ProviderNameOpenAI,
		managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT41, ""),
		sessionCookie,
	)
	providerResponse := httptest.NewRecorder()
	router.ServeHTTP(providerResponse, providerRequest)
	if providerResponse.Code != http.StatusOK {
		t.Fatalf("save provider status=%d body=%s", providerResponse.Code, providerResponse.Body.String())
	}

	secretRequest := authenticatedJSONRequest(http.MethodPost, tenantPath+"/secrets", `{}`, sessionCookie)
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

	requestCases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/", body: ""},
		{method: http.MethodPost, path: "/", body: `{"prompt":"hello"}`},
		{method: http.MethodPost, path: "/v2", body: `{"messages":[{"role":"user","content":"hello"}]}`},
	}
	for _, requestCase := range requestCases {
		query := url.Values{
			"key":      []string{secretPayload.Secret},
			"provider": []string{proxy.ProviderNameGemini},
		}
		if requestCase.method == http.MethodGet {
			query.Set("prompt", "hello")
		}
		request := httptest.NewRequest(
			requestCase.method,
			requestCase.path+"?"+query.Encode(),
			strings.NewReader(requestCase.body),
		)
		if requestCase.method == http.MethodPost {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("method=%s path=%s status=%d body=%s", requestCase.method, requestCase.path, response.Code, response.Body.String())
		}
	}

	usage := waitForManagementValue(t, func() managementUsageTestResponse {
		return requestManagementUsage(t, router, sessionCookie, "30d")
	}, func(payload managementUsageTestResponse) bool {
		return payload.Totals.Requests == len(requestCases)
	})
	if len(usage.Providers) != 1 || usage.Providers[0].Provider != proxy.ProviderNameGemini || usage.Providers[0].Data.Requests != len(requestCases) {
		t.Fatalf("providers=%+v", usage.Providers)
	}
	if len(usage.Models) != 1 || usage.Models[0].Provider != proxy.ProviderNameGemini || usage.Models[0].Model != proxy.ModelNameGemini35Flash || usage.Models[0].Data.Requests != len(requestCases) {
		t.Fatalf("models=%+v", usage.Models)
	}
}

func TestManagementAdminUsersDashboard(t *testing.T) {
	chatServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"admin usage ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":11,"total_tokens":18}}`))
	}))
	defer chatServer.Close()

	router := newManagementRouter(t, proxy.Configuration{Endpoints: providerEndpoints(chatServer.URL, proxy.ProviderNameDeepSeek)})
	userOneCookie := managementSessionCookie(t, "admin-visible-user-one")
	userTwoCookie := managementSessionCookie(t, "admin-visible-user-two")
	adminCookie := managementSessionCookieWithEmail(t, "admin-user", testManagementAdminEmail)
	userOneTenantPath := managementDefaultTenantTestPath(t, router, userOneCookie, "")

	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, userOneTenantPath+"/provider-connections/deepseek", managementProviderKeyRequestBody(t, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""), userOneCookie)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}
	saveOpenAIKeyRequest := authenticatedJSONRequest(http.MethodPut, userOneTenantPath+"/provider-connections/openai", managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT41, ""), userOneCookie)
	saveOpenAIKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveOpenAIKeyResponse, saveOpenAIKeyRequest)
	if saveOpenAIKeyResponse.Code != http.StatusOK {
		t.Fatalf("save openai key status=%d body=%s", saveOpenAIKeyResponse.Code, saveOpenAIKeyResponse.Body.String())
	}
	defaultsBody := `{"provider":"deepseek","model":"` + proxy.ModelNameDeepSeekV4Flash + `","dictation_provider":"openai","dictation_model":"` + proxy.DefaultDictationModel + `","system_prompt":"","reasoning_effort":""}`
	defaultsRequest := authenticatedJSONRequest(http.MethodPut, userOneTenantPath+"/defaults", defaultsBody, userOneCookie)
	defaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(defaultsResponse, defaultsRequest)
	if defaultsResponse.Code != http.StatusOK {
		t.Fatalf("defaults status=%d body=%s", defaultsResponse.Code, defaultsResponse.Body.String())
	}
	secretRequest := authenticatedJSONRequest(http.MethodPost, userOneTenantPath+"/secrets", `{}`, userOneCookie)
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

	profileRequest := authenticatedJSONRequest(http.MethodGet, "/api/management/account", "", userTwoCookie)
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
	waitForManagementValue(t, func() managementUsageTestResponse {
		return requestManagementUsage(t, router, userOneCookie, "30d")
	}, func(payload managementUsageTestResponse) bool {
		return payload.Totals.Requests == 1
	})

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
		if user.TenantCount != 1 || len(user.Tenants) != 1 {
			t.Fatalf("admin user tenant count mismatch: %+v", user)
		}
		userUsageByID[user.User.ID] = user.Tenants[0].Usage.Totals.Requests
		if user.Tenants[0].ID == "" || user.User.Email == "" {
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
		if upstreamPayload["model"] != proxy.ModelNameMuseSpark12 {
			t.Fatalf("model=%v want=%s", upstreamPayload["model"], proxy.ModelNameMuseSpark12)
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
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"managed meta ok"},"finish_reason":"stop"}]}`))
	}))
	defer upstreamServer.Close()

	router := newManagementRouter(t, proxy.Configuration{
		Endpoints: providerEndpoints(upstreamServer.URL, proxy.ProviderNameMeta),
	})
	userOneCookie := managementSessionCookie(t, "tauth-user-one")
	userTwoCookie := managementSessionCookie(t, "tauth-user-two")
	userOneTenantPath := managementDefaultTenantTestPath(t, router, userOneCookie, "")

	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, userOneTenantPath+"/provider-connections/meta", managementProviderKeyRequestBody(t, testManagementMetaKey, proxy.ModelNameMuseSpark12, "meta managed system"), userOneCookie)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}
	if strings.Contains(saveKeyResponse.Body.String(), testManagementMetaKey) || !strings.Contains(saveKeyResponse.Body.String(), "sk-...meta") {
		t.Fatalf("provider key response leaked or failed to mask key: %s", saveKeyResponse.Body.String())
	}
	for _, expectedFragment := range []string{
		`"id":"meta"`,
		`"label":"Meta"`,
		`"text_model":"muse-spark-1.2"`,
		`"text_default_model":"muse-spark-1.1"`,
		`"text_models":[{"id":"muse-spark-1.1"},{"id":"muse-spark-1.2"}]`,
		`"supports_dictation":false`,
	} {
		if !strings.Contains(saveKeyResponse.Body.String(), expectedFragment) {
			t.Fatalf("provider key response missing %q: %s", expectedFragment, saveKeyResponse.Body.String())
		}
	}

	saveOpenAIKeyRequest := authenticatedJSONRequest(http.MethodPut, userOneTenantPath+"/provider-connections/openai", managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT41, ""), userOneCookie)
	saveOpenAIKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveOpenAIKeyResponse, saveOpenAIKeyRequest)
	if saveOpenAIKeyResponse.Code != http.StatusOK {
		t.Fatalf("save openai key status=%d body=%s", saveOpenAIKeyResponse.Code, saveOpenAIKeyResponse.Body.String())
	}

	defaultsBody := `{"provider":"meta","model":"` + proxy.ModelNameMuseSpark12 + `","dictation_provider":"openai","dictation_model":"` + proxy.DefaultDictationModel + `","system_prompt":"meta managed system","reasoning_effort":""}`
	defaultsRequest := authenticatedJSONRequest(http.MethodPut, userOneTenantPath+"/defaults", defaultsBody, userOneCookie)
	defaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(defaultsResponse, defaultsRequest)
	if defaultsResponse.Code != http.StatusOK {
		t.Fatalf("defaults status=%d body=%s", defaultsResponse.Code, defaultsResponse.Body.String())
	}

	userTwoProfileRequest := httptest.NewRequest(http.MethodGet, managementDefaultTenantTestPath(t, router, userTwoCookie, ""), nil)
	userTwoProfileRequest.AddCookie(userTwoCookie)
	userTwoProfileResponse := httptest.NewRecorder()
	router.ServeHTTP(userTwoProfileResponse, userTwoProfileRequest)
	if userTwoProfileResponse.Code != http.StatusOK {
		t.Fatalf("user2 profile status=%d body=%s", userTwoProfileResponse.Code, userTwoProfileResponse.Body.String())
	}
	if strings.Contains(userTwoProfileResponse.Body.String(), "sk-...meta") {
		t.Fatalf("user2 saw user1 provider key: %s", userTwoProfileResponse.Body.String())
	}

	secretRequest := authenticatedJSONRequest(http.MethodPost, userOneTenantPath+"/secrets", `{}`, userOneCookie)
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
	if proxyResponse.Code != http.StatusOK || strings.TrimSpace(proxyResponse.Body.String()) != "managed meta ok" {
		t.Fatalf("proxy status=%d body=%q", proxyResponse.Code, proxyResponse.Body.String())
	}
	if capturedAuthorization != "Bearer "+testManagementMetaKey {
		t.Fatalf("authorization=%q want=%q", capturedAuthorization, "Bearer "+testManagementMetaKey)
	}

	replacementSecretRequest := authenticatedJSONRequest(http.MethodPost, userOneTenantPath+"/secrets", `{}`, userOneCookie)
	replacementSecretResponseRecorder := httptest.NewRecorder()
	router.ServeHTTP(replacementSecretResponseRecorder, replacementSecretRequest)
	if replacementSecretResponseRecorder.Code != http.StatusOK {
		t.Fatalf("replacement secret status=%d body=%s", replacementSecretResponseRecorder.Code, replacementSecretResponseRecorder.Body.String())
	}
	var replacementSecretResponse struct {
		Secret string `json:"secret"`
	}
	if decodeError := json.Unmarshal(replacementSecretResponseRecorder.Body.Bytes(), &replacementSecretResponse); decodeError != nil {
		t.Fatalf("decode replacement secret response: %v", decodeError)
	}
	if !strings.HasPrefix(replacementSecretResponse.Secret, "llmp_") || replacementSecretResponse.Secret == secretResponse.Secret {
		t.Fatalf("replacement secret=%q original=%q", replacementSecretResponse.Secret, secretResponse.Secret)
	}
	replacedProxyRequest := httptest.NewRequest(http.MethodGet, "/?"+proxyRequestValues.Encode(), nil)
	replacedProxyResponse := httptest.NewRecorder()
	router.ServeHTTP(replacedProxyResponse, replacedProxyRequest)
	if replacedProxyResponse.Code != http.StatusForbidden {
		t.Fatalf("replaced key status=%d want=%d", replacedProxyResponse.Code, http.StatusForbidden)
	}
	proxyRequestValues.Set("key", replacementSecretResponse.Secret)
	replacementProxyResponse := httptest.NewRecorder()
	router.ServeHTTP(replacementProxyResponse, httptest.NewRequest(http.MethodGet, "/?"+proxyRequestValues.Encode(), nil))
	if replacementProxyResponse.Code != http.StatusOK {
		t.Fatalf("replacement key status=%d body=%q", replacementProxyResponse.Code, replacementProxyResponse.Body.String())
	}
	deleteSecretRequest := authenticatedJSONRequest(http.MethodDelete, userOneTenantPath+"/secrets", `{}`, userOneCookie)
	deleteSecretResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteSecretResponse, deleteSecretRequest)
	if deleteSecretResponse.Code != http.StatusNotFound {
		t.Fatalf("obsolete secret delete status=%d want=%d", deleteSecretResponse.Code, http.StatusNotFound)
	}
	waitForManagementRequestCount(t, router, userOneCookie, 2)
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
		Endpoints: providerEndpoints(upstreamServer.URL, proxy.ProviderNameOpenAI),
	})
	userCookie := managementSessionCookie(t, "tauth-openai-defaults-user")
	tenantPath := managementDefaultTenantTestPath(t, router, userCookie, "")
	saveKeyRequest := authenticatedJSONRequest(http.MethodPut, tenantPath+"/provider-connections/openai", managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT55, "provider-owned system"), userCookie)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%s", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}

	defaultsBody := `{"provider":"openai","model":"` + proxy.ModelNameGPT41 + `","dictation_provider":"openai","dictation_model":"` + proxy.DefaultDictationModel + `","system_prompt":"tenant default system","reasoning_effort":""}`
	defaultsRequest := authenticatedJSONRequest(http.MethodPut, tenantPath+"/defaults", defaultsBody, userCookie)
	defaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(defaultsResponse, defaultsRequest)
	if defaultsResponse.Code != http.StatusOK {
		t.Fatalf("defaults status=%d body=%s", defaultsResponse.Code, defaultsResponse.Body.String())
	}

	secretRequest := authenticatedJSONRequest(http.MethodPost, tenantPath+"/secrets", `{}`, userCookie)
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
	waitForManagementRequestCount(t, router, userCookie, 2)
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

	qwenCloudJSONRequest := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret, bytes.NewBufferString(`{"prompt":"hello","qwen_cloud_token_plan_api_key":"sk-client"}`))
	qwenCloudJSONRequest.Header.Set("Content-Type", "application/json")
	qwenCloudJSONResponse := httptest.NewRecorder()
	router.ServeHTTP(qwenCloudJSONResponse, qwenCloudJSONRequest)
	if qwenCloudJSONResponse.Code != http.StatusBadRequest {
		t.Fatalf("qwen cloud json status=%d body=%q", qwenCloudJSONResponse.Code, qwenCloudJSONResponse.Body.String())
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

	miniMaxQueryRequest := httptest.NewRequest(http.MethodPost, "/v2?key="+TestSecret+"&minimax_api_key=sk-client", bytes.NewBufferString(`{"messages":[{"role":"user","content":"hello"}]}`))
	miniMaxQueryRequest.Header.Set("Content-Type", "application/json")
	miniMaxQueryResponse := httptest.NewRecorder()
	router.ServeHTTP(miniMaxQueryResponse, miniMaxQueryRequest)
	if miniMaxQueryResponse.Code != http.StatusBadRequest {
		t.Fatalf("minimax query status=%d body=%q", miniMaxQueryResponse.Code, miniMaxQueryResponse.Body.String())
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
	previousHTTPClient := proxy.HTTPClient
	proxy.HTTPClient = managementProviderKeyVerificationHTTPDoer{next: previousHTTPClient}
	defer func() {
		proxy.HTTPClient = previousHTTPClient
	}()
	router, buildError := buildRouterWithCatalogs(t, managementConfigurationWithDatabasePath(configuration, databasePath), zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}
	return router
}

func managementConfigurationWithDatabasePath(configuration proxy.Configuration, databasePath string) proxy.Configuration {
	var databaseDialector gorm.Dialector = sqlite.Open(databasePath)
	if databasePath == "" {
		databaseDialector = nil
	}
	configuration.Management = proxy.ManagementConfiguration{
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
		SessionPath:              "/auth/session",
		JWTSigningKey:            testManagementSigningKey,
		SessionCookieName:        testManagementCookieName,
		DatabasePath:             databasePath,
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

func authenticatedProviderKeyRevealRequest(method string, path string, sessionCookie *http.Cookie, origin string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	request.AddCookie(sessionCookie)
	return request
}

func providerKeyRevealRequestWithoutContentType(method string, path string, sessionCookie *http.Cookie, origin string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	request.Header.Set("Origin", origin)
	request.AddCookie(sessionCookie)
	return request
}

type managementUsageTestResponse struct {
	Interval   string `json:"interval"`
	BucketUnit string `json:"bucket_unit"`
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
	Buckets []struct {
		Start string `json:"start"`
		Data  struct {
			Requests int `json:"requests"`
		} `json:"data"`
	} `json:"buckets"`
	Providers []struct {
		Provider string `json:"provider"`
		Data     struct {
			Requests int `json:"requests"`
		} `json:"data"`
	} `json:"providers"`
	Models []struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Data     struct {
			Requests int `json:"requests"`
		} `json:"data"`
	} `json:"models"`
	StatusCodes []struct {
		StatusCode int `json:"status_code"`
		Requests   int `json:"requests"`
	} `json:"status_codes"`
}

type managementAdminUsageTestResponse struct {
	PeriodDays int `json:"period_days"`
	Totals     struct {
		Requests           int `json:"requests"`
		SuccessfulRequests int `json:"successful_requests"`
		TotalTokens        int `json:"total_tokens"`
	} `json:"totals"`
	Daily []struct {
		Date string `json:"date"`
		Data struct {
			Requests int `json:"requests"`
		} `json:"data"`
	} `json:"daily"`
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
		TenantCount int `json:"tenant_count"`
		Tenants     []struct {
			ID        string                           `json:"id"`
			HasSecret bool                             `json:"has_secret"`
			Usage     managementAdminUsageTestResponse `json:"usage"`
		} `json:"tenants"`
	} `json:"users"`
}

func requestManagementUsage(t *testing.T, router http.Handler, sessionCookie *http.Cookie, interval string) managementUsageTestResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/management/usage?interval="+url.QueryEscape(interval), nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("usage status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("usage cache control=%q want=no-store", response.Header().Get("Cache-Control"))
	}
	var contractFields map[string]json.RawMessage
	if decodeError := json.Unmarshal(response.Body.Bytes(), &contractFields); decodeError != nil {
		t.Fatalf("decode usage contract: %v", decodeError)
	}
	for _, obsoleteField := range []string{"period_days", "daily"} {
		if _, exists := contractFields[obsoleteField]; exists {
			t.Fatalf("usage response retained obsolete field %q: %s", obsoleteField, response.Body.String())
		}
	}
	var usage managementUsageTestResponse
	if decodeError := json.Unmarshal(response.Body.Bytes(), &usage); decodeError != nil {
		t.Fatalf("decode usage: %v", decodeError)
	}
	return usage
}

func waitForManagementRequestCount(t *testing.T, router http.Handler, sessionCookie *http.Cookie, expectedRequests int) {
	t.Helper()
	waitForManagementValue(t, func() managementUsageTestResponse {
		return requestManagementUsage(t, router, sessionCookie, "30d")
	}, func(payload managementUsageTestResponse) bool {
		return payload.Totals.Requests == expectedRequests
	})
}

type managementTenantDefaultsTestResponse struct {
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	DictationProvider string `json:"dictation_provider"`
	DictationModel    string `json:"dictation_model"`
	ReasoningEffort   string `json:"reasoning_effort"`
}

func assertManagementProfileDefaults(t *testing.T, router http.Handler, sessionCookie *http.Cookie, expectedDefaults managementTenantDefaultsTestResponse) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, managementDefaultTenantTestPath(t, router, sessionCookie, ""), nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("profile cache control=%q want=no-store", response.Header().Get("Cache-Control"))
	}
	var profile struct {
		Tenant struct {
			Defaults managementTenantDefaultsTestResponse `json:"defaults"`
		} `json:"tenant"`
	}
	if decodeError := json.Unmarshal(response.Body.Bytes(), &profile); decodeError != nil {
		t.Fatalf("decode profile: %v", decodeError)
	}
	if profile.Tenant.Defaults != expectedDefaults {
		t.Fatalf("profile defaults=%+v want=%+v", profile.Tenant.Defaults, expectedDefaults)
	}
}

func managementDefaultTenantTestID(t *testing.T, router http.Handler, sessionCookie *http.Cookie) string {
	t.Helper()
	account := requestManagementAccount(t, router, sessionCookie)
	if len(account.Tenants) == 0 {
		t.Fatal("management account has no tenants")
	}
	return account.Tenants[0].ID
}

func managementDefaultTenantTestPath(t *testing.T, router http.Handler, sessionCookie *http.Cookie, suffix string) string {
	t.Helper()
	return "/api/management/tenants/" + url.PathEscape(managementDefaultTenantTestID(t, router, sessionCookie)) + suffix
}

type managedProviderKeyFixture struct {
	TenantID        string
	ProviderID      string
	EncryptedAPIKey string
	TextModel       string
	SystemPrompt    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type managedProviderConnectionFixture struct {
	TenantID   string `gorm:"primaryKey"`
	ProviderID string `gorm:"primaryKey"`
	FieldID    string `gorm:"primaryKey"`
	Value      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (managedProviderConnectionFixture) TableName() string {
	return "managed_provider_connection_records"
}

type managedProviderProfileFixture struct {
	TenantID     string `gorm:"primaryKey"`
	ProviderID   string `gorm:"primaryKey"`
	TextModel    string
	SystemPrompt string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (managedProviderProfileFixture) TableName() string {
	return "managed_provider_profile_records"
}

func loadManagedProviderKeyFixture(database *gorm.DB, tenantID string, providerID string) (managedProviderKeyFixture, error) {
	var connection managedProviderConnectionFixture
	if queryError := database.Where("tenant_id = ? AND provider_id = ? AND field_id = ?", tenantID, providerID, proxy.CatalogCredentialAPIKey).First(&connection).Error; queryError != nil {
		return managedProviderKeyFixture{}, queryError
	}
	var profile managedProviderProfileFixture
	if queryError := database.Where("tenant_id = ? AND provider_id = ?", tenantID, providerID).First(&profile).Error; queryError != nil {
		return managedProviderKeyFixture{}, queryError
	}
	return managedProviderKeyFixture{
		TenantID: tenantID, ProviderID: providerID, EncryptedAPIKey: connection.Value,
		TextModel: profile.TextModel, SystemPrompt: profile.SystemPrompt,
		CreatedAt: connection.CreatedAt, UpdatedAt: connection.UpdatedAt,
	}, nil
}

func openManagedFixtureDatabase(t *testing.T, databasePath string) *gorm.DB {
	t.Helper()
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open managed fixture database: %v", openError)
	}
	return database
}
