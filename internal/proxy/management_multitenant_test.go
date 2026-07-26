package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
)

type managementAccountTestResponse struct {
	User struct {
		ID string `json:"id"`
	} `json:"user"`
	Tenants []managementTenantSummaryTestResponse `json:"tenants"`
}

type managementTenantSummaryTestResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HasSecret bool   `json:"has_secret"`
}

type managementTenantProfileTestResponse struct {
	Tenant managementTenantSummaryTestResponse `json:"tenant"`
}

type managementTenantSecretTestResponse struct {
	Secret string `json:"secret"`
}

type managementTenantUsageTestResponse struct {
	Totals struct {
		Requests int `json:"requests"`
	} `json:"totals"`
}

func TestManagementTenantLifecycleAndIsolation(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	ownerCookie := managementSessionCookie(t, "tenant-owner")
	otherOwnerCookie := managementSessionCookie(t, "other-tenant-owner")

	account := requestManagementAccount(t, router, ownerCookie)
	if account.User.ID != "tenant-owner" {
		t.Fatalf("account user=%q want=tenant-owner", account.User.ID)
	}
	if len(account.Tenants) != 1 || account.Tenants[0].Name != "Default" || account.Tenants[0].ID == "" {
		t.Fatalf("initial tenants=%+v", account.Tenants)
	}
	defaultTenantID := account.Tenants[0].ID

	obsoleteProfileRequest := httptest.NewRequest(http.MethodGet, "/api/management/profile", nil)
	obsoleteProfileRequest.AddCookie(ownerCookie)
	obsoleteProfileResponse := httptest.NewRecorder()
	router.ServeHTTP(obsoleteProfileResponse, obsoleteProfileRequest)
	if obsoleteProfileResponse.Code != http.StatusNotFound {
		t.Fatalf("obsolete profile status=%d body=%q", obsoleteProfileResponse.Code, obsoleteProfileResponse.Body.String())
	}

	createRequest := authenticatedJSONRequest(http.MethodPost, "/api/management/tenants", `{"name":"Project Alpha"}`, ownerCookie)
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create tenant status=%d body=%q", createResponse.Code, createResponse.Body.String())
	}
	var createdProfile managementTenantProfileTestResponse
	if decodeError := json.Unmarshal(createResponse.Body.Bytes(), &createdProfile); decodeError != nil {
		t.Fatalf("decode created tenant: %v", decodeError)
	}
	if createdProfile.Tenant.Name != "Project Alpha" || createdProfile.Tenant.ID == "" || createdProfile.Tenant.ID == defaultTenantID {
		t.Fatalf("created tenant=%+v default=%q", createdProfile.Tenant, defaultTenantID)
	}
	createdTenantID := createdProfile.Tenant.ID

	account = requestManagementAccount(t, router, ownerCookie)
	if len(account.Tenants) != 2 || account.Tenants[0].ID != defaultTenantID || account.Tenants[1].ID != createdTenantID {
		t.Fatalf("ordered tenants=%+v", account.Tenants)
	}

	foreignReadRequest := httptest.NewRequest(http.MethodGet, "/api/management/tenants/"+createdTenantID, nil)
	foreignReadRequest.AddCookie(otherOwnerCookie)
	foreignReadResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignReadResponse, foreignReadRequest)
	if foreignReadResponse.Code != http.StatusNotFound {
		t.Fatalf("foreign tenant read status=%d body=%q", foreignReadResponse.Code, foreignReadResponse.Body.String())
	}

	foreignDeleteRequest := authenticatedJSONRequest(http.MethodDelete, "/api/management/tenants/"+createdTenantID, `{}`, otherOwnerCookie)
	foreignDeleteResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignDeleteResponse, foreignDeleteRequest)
	if foreignDeleteResponse.Code != http.StatusNotFound {
		t.Fatalf("foreign tenant delete status=%d body=%q", foreignDeleteResponse.Code, foreignDeleteResponse.Body.String())
	}

	renameRequest := authenticatedJSONRequest(http.MethodPut, "/api/management/tenants/"+createdTenantID, `{"name":"Project Beta"}`, ownerCookie)
	renameResponse := httptest.NewRecorder()
	router.ServeHTTP(renameResponse, renameRequest)
	if renameResponse.Code != http.StatusOK {
		t.Fatalf("rename tenant status=%d body=%q", renameResponse.Code, renameResponse.Body.String())
	}
	var renamedProfile managementTenantProfileTestResponse
	if decodeError := json.Unmarshal(renameResponse.Body.Bytes(), &renamedProfile); decodeError != nil {
		t.Fatalf("decode renamed tenant: %v", decodeError)
	}
	if renamedProfile.Tenant.Name != "Project Beta" {
		t.Fatalf("renamed tenant=%+v", renamedProfile.Tenant)
	}

	duplicateRequest := authenticatedJSONRequest(http.MethodPost, "/api/management/tenants", `{"name":"project beta"}`, ownerCookie)
	duplicateResponse := httptest.NewRecorder()
	router.ServeHTTP(duplicateResponse, duplicateRequest)
	if duplicateResponse.Code != http.StatusConflict {
		t.Fatalf("duplicate tenant status=%d body=%q", duplicateResponse.Code, duplicateResponse.Body.String())
	}

	unscopedUsageRequest := httptest.NewRequest(http.MethodGet, "/api/management/usage?interval=30d", nil)
	unscopedUsageRequest.AddCookie(ownerCookie)
	unscopedUsageResponse := httptest.NewRecorder()
	router.ServeHTTP(unscopedUsageResponse, unscopedUsageRequest)
	if unscopedUsageResponse.Code != http.StatusNotFound {
		t.Fatalf("unscoped usage status=%d body=%q", unscopedUsageResponse.Code, unscopedUsageResponse.Body.String())
	}

	scopedUsageRequest := httptest.NewRequest(http.MethodGet, "/api/management/tenants/"+createdTenantID+"/usage?interval=30d", nil)
	scopedUsageRequest.AddCookie(ownerCookie)
	scopedUsageResponse := httptest.NewRecorder()
	router.ServeHTTP(scopedUsageResponse, scopedUsageRequest)
	if scopedUsageResponse.Code != http.StatusOK {
		t.Fatalf("scoped usage status=%d body=%q", scopedUsageResponse.Code, scopedUsageResponse.Body.String())
	}

	deleteRequest := authenticatedJSONRequest(http.MethodDelete, "/api/management/tenants/"+createdTenantID, `{}`, ownerCookie)
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete tenant status=%d body=%q", deleteResponse.Code, deleteResponse.Body.String())
	}

	deleteFinalRequest := authenticatedJSONRequest(http.MethodDelete, "/api/management/tenants/"+defaultTenantID, `{}`, ownerCookie)
	deleteFinalResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteFinalResponse, deleteFinalRequest)
	if deleteFinalResponse.Code != http.StatusConflict {
		t.Fatalf("delete final tenant status=%d body=%q", deleteFinalResponse.Code, deleteFinalResponse.Body.String())
	}
}

func TestManagementTenantConfigurationSecretAndUsageIsolation(t *testing.T) {
	type upstreamCall struct {
		authorization string
		model         string
	}
	upstreamCalls := []upstreamCall{}
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" {
			t.Fatalf("upstream path=%q", request.URL.Path)
		}
		body, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read upstream body: %v", readError)
		}
		var payload struct {
			Model string `json:"model"`
		}
		if decodeError := json.Unmarshal(body, &payload); decodeError != nil {
			t.Fatalf("decode upstream body: %v", decodeError)
		}
		upstreamCalls = append(upstreamCalls, upstreamCall{
			authorization: request.Header.Get("Authorization"),
			model:         payload.Model,
		})
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"id":"response-id","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"text","text":"tenant ok"}]}]}`))
	}))
	defer upstreamServer.Close()

	router := newManagementRouter(t, proxy.Configuration{OpenAIBaseURL: upstreamServer.URL})
	ownerCookie := managementSessionCookie(t, "configuration-owner")
	otherOwnerCookie := managementSessionCookie(t, "configuration-other-owner")
	account := requestManagementAccount(t, router, ownerCookie)
	firstTenantID := account.Tenants[0].ID
	secondTenant := createManagementTenant(t, router, ownerCookie, "Second")
	secondTenantID := secondTenant.Tenant.ID

	firstKey := "sk-first-tenant"
	secondKey := "sk-second-tenant"
	saveManagementProviderKey(t, router, ownerCookie, firstTenantID, firstKey, proxy.ModelNameGPT41, "first system")
	saveManagementProviderKey(t, router, ownerCookie, secondTenantID, secondKey, proxy.ModelNameGPT55, "second system")
	saveManagementDefaults(t, router, ownerCookie, firstTenantID, proxy.ModelNameGPT41, "first default")
	saveManagementDefaults(t, router, ownerCookie, secondTenantID, proxy.ModelNameGPT55, "second default")

	firstSecret := generateManagementTenantSecret(t, router, ownerCookie, firstTenantID)
	secondSecret := generateManagementTenantSecret(t, router, ownerCookie, secondTenantID)
	for _, secret := range []string{firstSecret, secondSecret} {
		values := url.Values{"key": {secret}, "prompt": {"hello"}}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?"+values.Encode(), nil))
		if response.Code != http.StatusOK {
			t.Fatalf("public proxy status=%d body=%q", response.Code, response.Body.String())
		}
	}
	if len(upstreamCalls) != 2 ||
		upstreamCalls[0].authorization != "Bearer "+firstKey ||
		upstreamCalls[0].model != proxy.ModelNameGPT41 ||
		upstreamCalls[1].authorization != "Bearer "+secondKey ||
		upstreamCalls[1].model != proxy.ModelNameGPT55 {
		t.Fatalf("upstream calls=%+v", upstreamCalls)
	}

	for _, tenantID := range []string{firstTenantID, secondTenantID} {
		usage := requestManagementTenantUsage(t, router, ownerCookie, tenantID)
		if usage.Totals.Requests != 1 {
			t.Fatalf("tenant=%s requests=%d want=1", tenantID, usage.Totals.Requests)
		}
	}

	foreignRevealRequest := authenticatedProviderKeyRevealRequest(
		http.MethodPost,
		managementTenantTestPath(secondTenantID, "/provider-keys/openai/reveal"),
		otherOwnerCookie,
		"http://localhost:8080",
	)
	foreignRevealResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignRevealResponse, foreignRevealRequest)
	if foreignRevealResponse.Code != http.StatusNotFound {
		t.Fatalf("foreign reveal status=%d body=%q", foreignRevealResponse.Code, foreignRevealResponse.Body.String())
	}

	accountResponse := requestManagementAccount(t, router, ownerCookie)
	accountJSON, marshalError := json.Marshal(accountResponse)
	if marshalError != nil {
		t.Fatalf("marshal account: %v", marshalError)
	}
	if strings.Contains(string(accountJSON), firstKey) || strings.Contains(string(accountJSON), secondKey) ||
		strings.Contains(string(accountJSON), firstSecret) || strings.Contains(string(accountJSON), secondSecret) {
		t.Fatalf("account leaked credentials: %s", accountJSON)
	}

	revokeRequest := authenticatedJSONRequest(http.MethodDelete, managementTenantTestPath(firstTenantID, "/secrets"), `{}`, ownerCookie)
	revokeResponse := httptest.NewRecorder()
	router.ServeHTTP(revokeResponse, revokeRequest)
	if revokeResponse.Code != http.StatusOK {
		t.Fatalf("revoke first secret status=%d body=%q", revokeResponse.Code, revokeResponse.Body.String())
	}
	firstProxyResponse := httptest.NewRecorder()
	router.ServeHTTP(firstProxyResponse, httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(firstSecret)+"&prompt=revoked", nil))
	if firstProxyResponse.Code != http.StatusForbidden {
		t.Fatalf("revoked first secret status=%d", firstProxyResponse.Code)
	}
	secondProxyResponse := httptest.NewRecorder()
	router.ServeHTTP(secondProxyResponse, httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secondSecret)+"&prompt=still-active", nil))
	if secondProxyResponse.Code != http.StatusOK {
		t.Fatalf("second secret after first revoke status=%d body=%q", secondProxyResponse.Code, secondProxyResponse.Body.String())
	}

	deleteFirstRequest := authenticatedJSONRequest(http.MethodDelete, managementTenantTestPath(firstTenantID, ""), `{}`, ownerCookie)
	deleteFirstResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteFirstResponse, deleteFirstRequest)
	if deleteFirstResponse.Code != http.StatusNoContent {
		t.Fatalf("delete first tenant status=%d body=%q", deleteFirstResponse.Code, deleteFirstResponse.Body.String())
	}
	secondAfterDeleteResponse := httptest.NewRecorder()
	router.ServeHTTP(secondAfterDeleteResponse, httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secondSecret)+"&prompt=after-delete", nil))
	if secondAfterDeleteResponse.Code != http.StatusOK {
		t.Fatalf("second secret after first delete status=%d body=%q", secondAfterDeleteResponse.Code, secondAfterDeleteResponse.Body.String())
	}
}

func requestManagementAccount(t *testing.T, router http.Handler, sessionCookie *http.Cookie) managementAccountTestResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/management/account", nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("account status=%d body=%q", response.Code, response.Body.String())
	}
	var account managementAccountTestResponse
	if decodeError := json.Unmarshal(response.Body.Bytes(), &account); decodeError != nil {
		t.Fatalf("decode account: %v", decodeError)
	}
	return account
}

func createManagementTenant(t *testing.T, router http.Handler, sessionCookie *http.Cookie, name string) managementTenantProfileTestResponse {
	t.Helper()
	request := authenticatedJSONRequest(http.MethodPost, "/api/management/tenants", `{"name":"`+name+`"}`, sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create tenant status=%d body=%q", response.Code, response.Body.String())
	}
	var profile managementTenantProfileTestResponse
	if decodeError := json.Unmarshal(response.Body.Bytes(), &profile); decodeError != nil {
		t.Fatalf("decode tenant: %v", decodeError)
	}
	return profile
}

func saveManagementProviderKey(t *testing.T, router http.Handler, sessionCookie *http.Cookie, tenantID string, apiKey string, model string, systemPrompt string) {
	t.Helper()
	request := authenticatedJSONRequest(
		http.MethodPut,
		managementTenantTestPath(tenantID, "/provider-keys/openai"),
		managementProviderKeyRequestBody(t, apiKey, model, systemPrompt),
		sessionCookie,
	)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("save provider key tenant=%s status=%d body=%q", tenantID, response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), apiKey) {
		t.Fatalf("save provider key leaked raw key: %q", response.Body.String())
	}
}

func saveManagementDefaults(t *testing.T, router http.Handler, sessionCookie *http.Cookie, tenantID string, model string, systemPrompt string) {
	t.Helper()
	request := authenticatedJSONRequest(
		http.MethodPut,
		managementTenantTestPath(tenantID, "/defaults"),
		managementDefaultsRequestBody(t, proxy.ProviderNameOpenAI, model, proxy.ProviderNameOpenAI, proxy.DefaultDictationModel, systemPrompt),
		sessionCookie,
	)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("save defaults tenant=%s status=%d body=%q", tenantID, response.Code, response.Body.String())
	}
}

func generateManagementTenantSecret(t *testing.T, router http.Handler, sessionCookie *http.Cookie, tenantID string) string {
	t.Helper()
	request := authenticatedJSONRequest(http.MethodPost, managementTenantTestPath(tenantID, "/secrets"), `{}`, sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("generate secret tenant=%s status=%d body=%q", tenantID, response.Code, response.Body.String())
	}
	var payload managementTenantSecretTestResponse
	if decodeError := json.Unmarshal(response.Body.Bytes(), &payload); decodeError != nil {
		t.Fatalf("decode secret: %v", decodeError)
	}
	if !strings.HasPrefix(payload.Secret, "llmp_") {
		t.Fatalf("generated secret=%q", payload.Secret)
	}
	return payload.Secret
}

func requestManagementTenantUsage(t *testing.T, router http.Handler, sessionCookie *http.Cookie, tenantID string) managementTenantUsageTestResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, managementTenantTestPath(tenantID, "/usage")+"?interval=30d", nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("usage tenant=%s status=%d body=%q", tenantID, response.Code, response.Body.String())
	}
	var payload managementTenantUsageTestResponse
	if decodeError := json.Unmarshal(response.Body.Bytes(), &payload); decodeError != nil {
		t.Fatalf("decode usage: %v", decodeError)
	}
	return payload
}

func managementTenantTestPath(tenantID string, suffix string) string {
	return "/api/management/tenants/" + url.PathEscape(tenantID) + suffix
}
