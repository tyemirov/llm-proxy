package proxy_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func TestOpenAPIContractMatchesRegisteredOwnedRoutes(t *testing.T) {
	contract, loadError := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if loadError != nil {
		t.Fatalf("load canonical OpenAPI contract: %v", loadError)
	}
	serverURLs, serverURLError := contract.ServerURLs()
	if serverURLError != nil {
		t.Fatalf("OpenAPI server URLs: %v", serverURLError)
	}
	if !reflect.DeepEqual(serverURLs, []string{"https://llm-proxy-api.mprlab.com"}) {
		t.Fatalf("OpenAPI server URLs=%v", serverURLs)
	}
	documentedProtocolMethods, protocolMethodsError := contract.ProtocolMethods()
	if protocolMethodsError != nil {
		t.Fatalf("OpenAPI protocol methods: %v", protocolMethodsError)
	}

	handler := newManagementRouter(t, proxy.Configuration{})
	router, ok := handler.(*gin.Engine)
	if !ok {
		t.Fatalf("management router type=%T want *gin.Engine", handler)
	}
	actualOperations := make([]openapitest.OperationKey, 0)
	for _, route := range router.Routes() {
		if _, protocolOnly := documentedProtocolMethods[route.Method]; protocolOnly {
			continue
		}
		actualOperations = append(actualOperations, openapitest.OperationKey{
			Method: route.Method,
			Path:   openAPIPath(route.Path),
		})
	}
	sort.Slice(actualOperations, func(leftIndex int, rightIndex int) bool {
		if actualOperations[leftIndex].Path == actualOperations[rightIndex].Path {
			return actualOperations[leftIndex].Method < actualOperations[rightIndex].Method
		}
		return actualOperations[leftIndex].Path < actualOperations[rightIndex].Path
	})
	documentedOperations, operationsError := contract.Operations()
	if operationsError != nil {
		t.Fatalf("OpenAPI operations: %v", operationsError)
	}
	if !reflect.DeepEqual(actualOperations, documentedOperations) {
		t.Fatalf("registered operations=%v documented operations=%v", actualOperations, documentedOperations)
	}

	if _, optionsDocumentedAsOperation := documentedProtocolMethods[http.MethodOptions]; !optionsDocumentedAsOperation {
		t.Fatalf("registered OPTIONS handlers are not documented as protocol handling")
	}
}

func TestOpenAPIContractDocumentsActualAuthenticationBoundaries(t *testing.T) {
	contract, loadError := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if loadError != nil {
		t.Fatalf("load canonical OpenAPI contract: %v", loadError)
	}

	tenantClientKey, tenantClientKeyError := contract.SecurityScheme("TenantClientKey")
	if tenantClientKeyError != nil {
		t.Fatalf("TenantClientKey security scheme: %v", tenantClientKeyError)
	}
	if tenantClientKey != (openapitest.SecurityScheme{Type: "apiKey", In: "query", Name: "key"}) {
		t.Fatalf("TenantClientKey=%+v", tenantClientKey)
	}

	tauthSession, tauthSessionError := contract.SecurityScheme("TAuthSession")
	if tauthSessionError != nil {
		t.Fatalf("TAuthSession security scheme: %v", tauthSessionError)
	}
	if tauthSession != (openapitest.SecurityScheme{Type: "apiKey", In: "cookie", Name: "app_session_llm_proxy"}) {
		t.Fatalf("TAuthSession=%+v", tauthSession)
	}

	operations, operationsError := contract.Operations()
	if operationsError != nil {
		t.Fatalf("OpenAPI operations: %v", operationsError)
	}
	for _, operation := range operations {
		expectedSecurity := [][]string{{"TAuthSession"}}
		switch operation.Path {
		case "/", "/v2", "/v2/analyze", "/dictate":
			expectedSecurity = [][]string{{"TenantClientKey"}}
		case proxy.ManagementConfigUIPath:
			expectedSecurity = [][]string{}
		}
		actualSecurity, securityError := contract.SecurityRequirements(operation.Path, operation.Method)
		if securityError != nil {
			t.Fatalf("%s %s security: %v", operation.Method, operation.Path, securityError)
		}
		if !reflect.DeepEqual(actualSecurity, expectedSecurity) {
			t.Fatalf("%s %s security=%v want=%v", operation.Method, operation.Path, actualSecurity, expectedSecurity)
		}
	}
}

func TestOpenAPIContractValidatesRepresentativeRealHTTPExchanges(t *testing.T) {
	contract, loadError := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if loadError != nil {
		t.Fatalf("load canonical OpenAPI contract: %v", loadError)
	}
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			t.Fatalf("upstream path=%s want=/chat/completions", request.URL.Path)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"contract response"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	}))
	t.Cleanup(upstreamServer.Close)
	router := newManagementRouter(t, proxy.Configuration{DeepSeekBaseURL: upstreamServer.URL})
	sessionCookie := managementSessionCookie(t, "openapi-contract-user")

	configRequest := httptest.NewRequest(http.MethodGet, proxy.ManagementConfigUIPath, nil)
	configRequest.Header.Set("Origin", "http://localhost:8080")
	assertOpenAPIRequest(t, contract, proxy.ManagementConfigUIPath, configRequest, nil)
	configResponse := httptest.NewRecorder()
	router.ServeHTTP(configResponse, configRequest)
	assertOpenAPIResponse(t, contract, proxy.ManagementConfigUIPath, http.MethodGet, configResponse)

	unauthorizedRequest := httptest.NewRequest(http.MethodGet, "/api/management/account", nil)
	assertOpenAPIRequest(t, contract, "/api/management/account", unauthorizedRequest, nil)
	unauthorizedResponse := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedResponse, unauthorizedRequest)
	assertOpenAPIResponse(t, contract, "/api/management/account", http.MethodGet, unauthorizedResponse)

	accountRequest := httptest.NewRequest(http.MethodGet, "/api/management/account", nil)
	accountRequest.AddCookie(sessionCookie)
	assertOpenAPIRequest(t, contract, "/api/management/account", accountRequest, nil)
	accountResponse := httptest.NewRecorder()
	router.ServeHTTP(accountResponse, accountRequest)
	assertOpenAPIResponse(t, contract, "/api/management/account", http.MethodGet, accountResponse)
	var account managementAccountTestResponse
	if decodeError := json.Unmarshal(accountResponse.Body.Bytes(), &account); decodeError != nil {
		t.Fatalf("decode management account: %v", decodeError)
	}
	if len(account.Tenants) != 1 {
		t.Fatalf("management account tenants=%d want=1", len(account.Tenants))
	}
	tenantID := account.Tenants[0].ID
	tenantPath := "/api/management/tenants/" + url.PathEscape(tenantID)

	providerKeyBody := []byte(managementProviderKeyRequestBody(t, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""))
	providerKeyRequest := authenticatedJSONRequest(http.MethodPut, tenantPath+"/provider-keys/deepseek", string(providerKeyBody), sessionCookie)
	assertOpenAPIRequest(t, contract, "/api/management/tenants/{tenant_id}/provider-keys/{provider}", providerKeyRequest, providerKeyBody)
	providerKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(providerKeyResponse, providerKeyRequest)
	assertOpenAPIResponse(t, contract, "/api/management/tenants/{tenant_id}/provider-keys/{provider}", http.MethodPut, providerKeyResponse)

	retainedProviderKeyBody := []byte(managementProviderKeyRequestBody(t, "", proxy.ModelNameDeepSeekV4Flash, "Retain the existing key."))
	retainedProviderKeyRequest := authenticatedJSONRequest(
		http.MethodPut,
		tenantPath+"/provider-keys/deepseek",
		string(retainedProviderKeyBody),
		sessionCookie,
	)
	assertOpenAPIRequest(t, contract, "/api/management/tenants/{tenant_id}/provider-keys/{provider}", retainedProviderKeyRequest, retainedProviderKeyBody)
	retainedProviderKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(retainedProviderKeyResponse, retainedProviderKeyRequest)
	assertOpenAPIResponse(t, contract, "/api/management/tenants/{tenant_id}/provider-keys/{provider}", http.MethodPut, retainedProviderKeyResponse)

	secretBody := []byte(`{}`)
	secretRequest := authenticatedJSONRequest(http.MethodPost, tenantPath+"/secrets", string(secretBody), sessionCookie)
	assertOpenAPIRequest(t, contract, "/api/management/tenants/{tenant_id}/secrets", secretRequest, secretBody)
	secretResponse := httptest.NewRecorder()
	router.ServeHTTP(secretResponse, secretRequest)
	assertOpenAPIResponse(t, contract, "/api/management/tenants/{tenant_id}/secrets", http.MethodPost, secretResponse)
	var secret managementTenantSecretTestResponse
	if decodeError := json.Unmarshal(secretResponse.Body.Bytes(), &secret); decodeError != nil {
		t.Fatalf("decode tenant secret: %v", decodeError)
	}

	v2Body := []byte(`{"messages":[{"role":"user","content":"Validate this exchange."}],"model":"deepseek-v4-flash","web_search":false,"max_tokens":5}`)
	v2Request := httptest.NewRequest(
		http.MethodPost,
		"/v2?key="+url.QueryEscape(secret.Secret)+"&provider=deepseek&format=application/json",
		bytes.NewReader(v2Body),
	)
	v2Request.Header.Set("Content-Type", "application/json")
	assertOpenAPIRequest(t, contract, "/v2", v2Request, v2Body)
	v2Response := httptest.NewRecorder()
	router.ServeHTTP(v2Response, v2Request)
	assertOpenAPIResponse(t, contract, "/v2", http.MethodPost, v2Response)

	analyzerUpstream := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{
			"id":"openapi-analyzer",
			"status":"completed",
			"output_text":"{\"outcome\":\"pass\"}",
			"output":[{
				"type":"message",
				"role":"assistant",
				"content":[{"type":"output_text","text":"{\"outcome\":\"pass\"}"}]
			}]
		}`))
	}))
	t.Cleanup(analyzerUpstream.Close)
	analyzerRouter := NewTestRouter(t, analyzerUpstream.URL)
	analyzerBody := analyzerRequestBody(t, []byte("openapi-first-image"), []byte("openapi-second-image"))
	analyzerRequest := httptest.NewRequest(
		http.MethodPost,
		"/v2/analyze?key="+url.QueryEscape(TestSecret)+"&provider=openai",
		bytes.NewReader(analyzerBody),
	)
	analyzerRequest.Header.Set("Content-Type", "application/json")
	assertOpenAPIRequest(t, contract, "/v2/analyze", analyzerRequest, analyzerBody)
	analyzerResponse := httptest.NewRecorder()
	analyzerRouter.ServeHTTP(analyzerResponse, analyzerRequest)
	assertOpenAPIResponse(t, contract, "/v2/analyze", http.MethodPost, analyzerResponse)

	unsupportedReasoningBody := []byte(`{"messages":[{"role":"user","content":"Validate the route error."}],"model":"deepseek-v4-flash","reasoning_effort":"high"}`)
	unsupportedReasoningRequest := httptest.NewRequest(
		http.MethodPost,
		"/v2?key="+url.QueryEscape(secret.Secret)+"&provider=deepseek",
		bytes.NewReader(unsupportedReasoningBody),
	)
	unsupportedReasoningRequest.Header.Set("Content-Type", "application/json")
	assertOpenAPIRequest(t, contract, "/v2", unsupportedReasoningRequest, unsupportedReasoningBody)
	unsupportedReasoningResponse := httptest.NewRecorder()
	router.ServeHTTP(unsupportedReasoningResponse, unsupportedReasoningRequest)
	assertOpenAPIResponse(t, contract, "/v2", http.MethodPost, unsupportedReasoningResponse)

	invalidTimeoutRequest := httptest.NewRequest(
		http.MethodPost,
		"/v2?key="+url.QueryEscape(secret.Secret)+"&provider=deepseek",
		bytes.NewReader(v2Body),
	)
	invalidTimeoutRequest.Header.Set("Content-Type", "application/json")
	invalidTimeoutRequest.Header.Set("X-LLM-Proxy-Request-Timeout-Seconds", "0")
	invalidTimeoutResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidTimeoutResponse, invalidTimeoutRequest)
	assertOpenAPIResponse(t, contract, "/v2", http.MethodPost, invalidTimeoutResponse)
	waitForManagementRequestCount(t, router, sessionCookie, 2)

	failuresRequest := httptest.NewRequest(http.MethodGet, tenantPath+"/usage/failures?interval=30d&limit=25", nil)
	failuresRequest.AddCookie(sessionCookie)
	assertOpenAPIRequest(t, contract, "/api/management/tenants/{tenant_id}/usage/failures", failuresRequest, nil)
	failuresResponse := httptest.NewRecorder()
	router.ServeHTTP(failuresResponse, failuresRequest)
	assertOpenAPIResponse(t, contract, "/api/management/tenants/{tenant_id}/usage/failures", http.MethodGet, failuresResponse)

	overLimitFailuresRequest := httptest.NewRequest(http.MethodGet, tenantPath+"/usage/failures?interval=30d&limit=101", nil)
	overLimitFailuresRequest.AddCookie(sessionCookie)
	validationError := contract.ValidateRequest(
		"/api/management/tenants/{tenant_id}/usage/failures",
		overLimitFailuresRequest.Method,
		overLimitFailuresRequest,
		nil,
	)
	if validationError == nil || !strings.Contains(validationError.Error(), "must be at most 100") {
		t.Fatalf("over-limit OpenAPI validation error=%v", validationError)
	}
	overLimitFailuresResponse := httptest.NewRecorder()
	router.ServeHTTP(overLimitFailuresResponse, overLimitFailuresRequest)
	if overLimitFailuresResponse.Code != http.StatusBadRequest {
		t.Fatalf("over-limit status=%d body=%q", overLimitFailuresResponse.Code, overLimitFailuresResponse.Body.String())
	}
	assertOpenAPIResponse(
		t,
		contract,
		"/api/management/tenants/{tenant_id}/usage/failures",
		http.MethodGet,
		overLimitFailuresResponse,
	)
}

func assertOpenAPIRequest(t *testing.T, contract *openapitest.Contract, path string, request *http.Request, body []byte) {
	t.Helper()
	if validationError := contract.ValidateRequest(path, request.Method, request, body); validationError != nil {
		t.Fatalf("validate OpenAPI request %s %s: %v", request.Method, path, validationError)
	}
}

func assertOpenAPIResponse(t *testing.T, contract *openapitest.Contract, path string, method string, response *httptest.ResponseRecorder) {
	t.Helper()
	if validationError := contract.ValidateResponse(path, method, response.Code, response.Header(), response.Body.Bytes()); validationError != nil {
		t.Fatalf("validate OpenAPI response %s %s status=%d body=%q: %v", method, path, response.Code, response.Body.String(), validationError)
	}
}

func openAPIPath(ginPath string) string {
	pathSegments := strings.Split(ginPath, "/")
	for segmentIndex, segment := range pathSegments {
		if strings.HasPrefix(segment, ":") {
			pathSegments[segmentIndex] = "{" + strings.TrimPrefix(segment, ":") + "}"
		}
	}
	return strings.Join(pathSegments, "/")
}
