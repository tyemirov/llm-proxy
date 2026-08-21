package proxy_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
		case "/", "/v2", "/v2/requests", "/dictate", "/model/v1/assets", "/model/v1/assets/{asset_id}":
			expectedSecurity = [][]string{{"TenantClientKey"}}
		case proxy.ManagementConfigUIPath, proxy.PublicCapabilitiesPath:
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

func TestOpenAPIContractEnforcesCanonicalWebSearchQuery(t *testing.T) {
	contract, loadError := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if loadError != nil {
		t.Fatalf("load canonical OpenAPI contract: %v", loadError)
	}

	for _, rawValue := range []string{"true", "false"} {
		request := httptest.NewRequest(
			http.MethodGet,
			"/?key=contract-test-key&prompt=contract&web_search="+url.QueryEscape(rawValue),
			nil,
		)
		if validationError := contract.ValidateRequest("/", request.Method, request, nil); validationError != nil {
			t.Fatalf("web_search=%q rejected: %v", rawValue, validationError)
		}
	}

	for _, rawValue := range []string{"", "1", "0", "t", "f", "y", "n", "yes", "no", "TRUE", "FALSE"} {
		request := httptest.NewRequest(
			http.MethodGet,
			"/?key=contract-test-key&prompt=contract&web_search="+url.QueryEscape(rawValue),
			nil,
		)
		if validationError := contract.ValidateRequest("/", request.Method, request, nil); validationError == nil {
			t.Fatalf("web_search=%q accepted", rawValue)
		}
	}
}

func TestOpenAPIContractEnforcesV2MediaRelationships(t *testing.T) {
	contract, loadError := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if loadError != nil {
		t.Fatalf("load canonical OpenAPI contract: %v", loadError)
	}

	testCases := []struct {
		name      string
		body      string
		wantValid bool
	}{
		{
			name:      "user image attachment",
			body:      `{"messages":[{"role":"user","content":"Describe the image.","attachments":[{"type":"image","mime_type":"image/png","data":"YQ==","sha256":"ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"}]}]}`,
			wantValid: true,
		},
		{
			name:      "user audio attachment",
			body:      `{"messages":[{"role":"user","content":"Transcribe the audio.","attachments":[{"type":"audio","mime_type":"audio/wav","data":"YQ==","sha256":"ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"}]}]}`,
			wantValid: true,
		},
		{
			name:      "user image asset attachment",
			body:      `{"messages":[{"role":"user","content":"Describe the image.","attachments":[{"type":"image","mime_type":"image/png","asset_id":"ast_0123456789abcdef0123456789abcdef","sha256":"ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"}]}]}`,
			wantValid: true,
		},
		{
			name:      "user audio asset attachment",
			body:      `{"messages":[{"role":"user","content":"Transcribe the audio.","attachments":[{"type":"audio","mime_type":"audio/wav","asset_id":"ast_0123456789abcdef0123456789abcdef","sha256":"ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"}]}]}`,
			wantValid: true,
		},
		{
			name:      "assistant text",
			body:      `{"messages":[{"role":"assistant","content":"Text only."}]}`,
			wantValid: true,
		},
		{
			name: "image type with audio MIME",
			body: `{"messages":[{"role":"user","content":"Reject mismatched media.","attachments":[{"type":"image","mime_type":"audio/wav","data":"YQ==","sha256":"ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"}]}]}`,
		},
		{
			name: "system attachment",
			body: `{"messages":[{"role":"system","content":"Reject attached system media.","attachments":[{"type":"image","mime_type":"image/png","data":"YQ==","sha256":"ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"}]}]}`,
		},
		{
			name: "attachment with data and asset id",
			body: `{"messages":[{"role":"user","content":"Reject conflicting media.","attachments":[{"type":"image","mime_type":"image/png","data":"YQ==","asset_id":"ast_0123456789abcdef0123456789abcdef","sha256":"ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"}]}]}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			body := []byte(testCase.body)
			request := httptest.NewRequest(http.MethodPost, "/v2?key=contract-test-key", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			validationError := contract.ValidateRequest("/v2", request.Method, request, body)
			if testCase.wantValid && validationError != nil {
				t.Fatalf("valid request rejected: %v", validationError)
			}
			if !testCase.wantValid && validationError == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
}

func TestOpenAPIContractValidatesTenantAssetUploadAndDeleteExchanges(t *testing.T) {
	contract, loadError := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if loadError != nil {
		t.Fatalf("load canonical OpenAPI contract: %v", loadError)
	}
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		AssetStorePath: t.TempDir(),
	}, nil)
	if buildError != nil {
		t.Fatalf("BuildRouter error: %v", buildError)
	}
	assetBytes := []byte("openapi-asset")
	digest := sha256.Sum256(assetBytes)
	uploadRequest := httptest.NewRequest(http.MethodPost, "/model/v1/assets?key="+TestSecret, bytes.NewReader(assetBytes))
	uploadRequest.Header.Set("Content-Type", "image/png")
	uploadRequest.Header.Set("X-LLM-Proxy-Asset-SHA256", hex.EncodeToString(digest[:]))
	assertOpenAPIRequest(t, contract, "/model/v1/assets", uploadRequest, assetBytes)
	uploadResponse := httptest.NewRecorder()
	router.ServeHTTP(uploadResponse, uploadRequest)
	assertOpenAPIResponse(t, contract, "/model/v1/assets", http.MethodPost, uploadResponse)
	var asset struct {
		AssetID string `json:"asset_id"`
	}
	if decodeError := json.Unmarshal(uploadResponse.Body.Bytes(), &asset); decodeError != nil || asset.AssetID == "" {
		t.Fatalf("asset response=%s error=%v", uploadResponse.Body.String(), decodeError)
	}
	deletePath := "/model/v1/assets/" + asset.AssetID
	deleteRequest := httptest.NewRequest(http.MethodDelete, deletePath+"?key="+TestSecret, nil)
	assertOpenAPIRequest(t, contract, "/model/v1/assets/{asset_id}", deleteRequest, nil)
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	assertOpenAPIResponse(t, contract, "/model/v1/assets/{asset_id}", http.MethodDelete, deleteResponse)
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
	router := newManagementRouter(t, proxy.Configuration{Endpoints: providerEndpoints(upstreamServer.URL, proxy.ProviderNameDeepSeek)})
	sessionCookie := managementSessionCookie(t, "openapi-contract-user")

	capabilitiesRequest := httptest.NewRequest(http.MethodGet, proxy.PublicCapabilitiesPath, nil)
	assertOpenAPIRequest(t, contract, proxy.PublicCapabilitiesPath, capabilitiesRequest, nil)
	capabilitiesResponse := httptest.NewRecorder()
	router.ServeHTTP(capabilitiesResponse, capabilitiesRequest)
	assertOpenAPIResponse(t, contract, proxy.PublicCapabilitiesPath, http.MethodGet, capabilitiesResponse)
	var capabilityCatalog proxy.PublicCapabilityCatalog
	if decodeError := json.Unmarshal(capabilitiesResponse.Body.Bytes(), &capabilityCatalog); decodeError != nil {
		t.Fatalf("decode public capability catalog: %v", decodeError)
	}
	if len(capabilityCatalog.Providers) != 11 {
		t.Fatalf("public capability providers=%d want=11", len(capabilityCatalog.Providers))
	}

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
	providerKeyRequest := authenticatedJSONRequest(http.MethodPut, tenantPath+"/provider-connections/deepseek", string(providerKeyBody), sessionCookie)
	assertOpenAPIRequest(t, contract, "/api/management/tenants/{tenant_id}/provider-connections/{provider}", providerKeyRequest, providerKeyBody)
	providerKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(providerKeyResponse, providerKeyRequest)
	assertOpenAPIResponse(t, contract, "/api/management/tenants/{tenant_id}/provider-connections/{provider}", http.MethodPut, providerKeyResponse)

	retainedProviderKeyBody := []byte(managementProviderKeyRequestBody(t, "", proxy.ModelNameDeepSeekV4Flash, "Retain the existing key."))
	retainedProviderKeyRequest := authenticatedJSONRequest(
		http.MethodPut,
		tenantPath+"/provider-connections/deepseek",
		string(retainedProviderKeyBody),
		sessionCookie,
	)
	assertOpenAPIRequest(t, contract, "/api/management/tenants/{tenant_id}/provider-connections/{provider}", retainedProviderKeyRequest, retainedProviderKeyBody)
	retainedProviderKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(retainedProviderKeyResponse, retainedProviderKeyRequest)
	assertOpenAPIResponse(t, contract, "/api/management/tenants/{tenant_id}/provider-connections/{provider}", http.MethodPut, retainedProviderKeyResponse)

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
