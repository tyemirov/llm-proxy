package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestManagementTenantHandlersRejectInvalidAndFailedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	principal := managementPrincipal{userID: "tauth-handler-user", userEmail: "owner@example.com"}
	tenantParams := gin.Params{{Key: "tenant_id", Value: "managed-default"}}
	providerParams := gin.Params{
		{Key: "tenant_id", Value: "managed-default"},
		{Key: "provider", Value: ProviderNameOpenAI},
	}

	newSeededService := func() (*managementService, *fakeManagedTenantDatabase) {
		database := newFakeManagedTenantDatabase()
		fakeUserWithTenant(database, principal, "managed-default", "Default", now)
		return newInternalManagementService(t, database, internalManagementProviderRegistry()), database
	}

	service, database := newSeededService()
	database.saveUserError = errInternalTestDatabase
	response := executeInternalManagementHandler(service.accountHandler(), http.MethodGet, "/api/management/account", "", nil, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("account error status=%d", response.Code)
	}

	service, _ = newSeededService()
	response = executeInternalManagementHandler(service.createTenantHandler(), http.MethodPost, "/api/management/tenants", "{", nil, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("create decode status=%d", response.Code)
	}
	response = executeInternalManagementHandler(service.createTenantHandler(), http.MethodPost, "/api/management/tenants", `{"name":" "}`, nil, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("create name status=%d", response.Code)
	}
	service, database = newSeededService()
	database.tenantNameExistsResults = []bool{true}
	response = executeInternalManagementHandler(service.createTenantHandler(), http.MethodPost, "/api/management/tenants", `{"name":"Project"}`, nil, principal)
	if response.Code != http.StatusConflict {
		t.Fatalf("create conflict status=%d", response.Code)
	}

	service, _ = newSeededService()
	response = executeInternalManagementHandler(service.tenantProfileHandler(), http.MethodGet, "/api/management/tenants/%20", "", gin.Params{{Key: "tenant_id", Value: " "}}, principal)
	if response.Code != http.StatusNotFound {
		t.Fatalf("profile invalid tenant status=%d", response.Code)
	}
	service, database = newSeededService()
	database.tenantByOwnerAndIDErrors = []error{errInternalTestDatabase}
	response = executeInternalManagementHandler(service.tenantProfileHandler(), http.MethodGet, "/api/management/tenants/managed-default", "", tenantParams, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("profile store status=%d", response.Code)
	}

	service, _ = newSeededService()
	response = executeInternalManagementHandler(service.renameTenantHandler(), http.MethodPut, "/api/management/tenants/%20", `{}`, gin.Params{{Key: "tenant_id", Value: " "}}, principal)
	if response.Code != http.StatusNotFound {
		t.Fatalf("rename invalid tenant status=%d", response.Code)
	}
	response = executeInternalManagementHandler(service.renameTenantHandler(), http.MethodPut, "/api/management/tenants/managed-default", "{", tenantParams, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("rename decode status=%d", response.Code)
	}
	response = executeInternalManagementHandler(service.renameTenantHandler(), http.MethodPut, "/api/management/tenants/managed-default", `{"name":" "}`, tenantParams, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("rename name status=%d", response.Code)
	}
	service, database = newSeededService()
	database.tenantNameExistsErrors = []error{errInternalTestDatabase}
	response = executeInternalManagementHandler(service.renameTenantHandler(), http.MethodPut, "/api/management/tenants/managed-default", `{"name":"Renamed"}`, tenantParams, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("rename store status=%d", response.Code)
	}

	service, _ = newSeededService()
	response = executeInternalManagementHandler(service.deleteTenantHandler(), http.MethodDelete, "/api/management/tenants/%20", "", gin.Params{{Key: "tenant_id", Value: " "}}, principal)
	if response.Code != http.StatusNotFound {
		t.Fatalf("delete invalid tenant status=%d", response.Code)
	}
	service, database = newSeededService()
	database.deleteTenantErrors = []error{errInternalTestDatabase}
	response = executeInternalManagementHandler(service.deleteTenantHandler(), http.MethodDelete, "/api/management/tenants/managed-default", "", tenantParams, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("delete store status=%d", response.Code)
	}

	service, _ = newSeededService()
	response = executeInternalManagementHandler(service.usageHandler(), http.MethodGet, "/api/management/tenants/%20/usage?interval=30d", "", gin.Params{{Key: "tenant_id", Value: " "}}, principal)
	if response.Code != http.StatusNotFound {
		t.Fatalf("usage invalid tenant status=%d", response.Code)
	}
	response = executeInternalManagementHandler(service.usageFailuresHandler(), http.MethodGet, "/api/management/tenants/%20/usage/failures?interval=30d", "", gin.Params{{Key: "tenant_id", Value: " "}}, principal)
	if response.Code != http.StatusNotFound {
		t.Fatalf("usage failures invalid tenant status=%d", response.Code)
	}
	for _, path := range []string{
		"/api/management/tenants/managed-default/usage",
		"/api/management/tenants/managed-default/usage?interval=30d&other=x",
		"/api/management/tenants/managed-default/usage?interval=invalid",
	} {
		response = executeInternalManagementHandler(service.usageHandler(), http.MethodGet, path, "", tenantParams, principal)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("usage path=%s status=%d", path, response.Code)
		}
	}
	service, database = newSeededService()
	database.streamUsageEventsError = errInternalTestDatabase
	response = executeInternalManagementHandler(service.usageHandler(), http.MethodGet, "/api/management/tenants/managed-default/usage?interval=30d", "", tenantParams, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("usage store status=%d", response.Code)
	}

	service, _ = newSeededService()
	response = executeInternalManagementHandler(service.adminUsersHandler(), http.MethodGet, "/api/management/admin/users", "", nil, principal)
	if response.Code != http.StatusForbidden {
		t.Fatalf("admin forbidden status=%d", response.Code)
	}
	service, database = newSeededService()
	database.usersError = errInternalTestDatabase
	response = executeInternalManagementHandler(service.adminUsersHandler(), http.MethodGet, "/api/management/admin/users", "", nil, managementPrincipal{userID: "admin", isAdmin: true})
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("admin store status=%d", response.Code)
	}

	service, _ = newSeededService()
	response = executeInternalManagementHandler(service.saveProviderKeyHandler(), http.MethodPut, "/api/management/tenants/%20/provider-keys/openai", `{}`, gin.Params{{Key: "tenant_id", Value: " "}}, principal)
	if response.Code != http.StatusNotFound {
		t.Fatalf("save key invalid tenant status=%d", response.Code)
	}
	invalidProviderParams := gin.Params{
		{Key: "tenant_id", Value: "managed-default"},
		{Key: "provider", Value: "missing"},
	}
	response = executeInternalManagementHandler(service.saveProviderKeyHandler(), http.MethodPut, "/api/management/tenants/managed-default/provider-keys/missing", `{}`, invalidProviderParams, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("save key provider status=%d", response.Code)
	}
	response = executeInternalManagementHandler(service.saveProviderKeyHandler(), http.MethodPut, "/api/management/tenants/managed-default/provider-keys/openai", "{", providerParams, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("save key decode status=%d", response.Code)
	}
	response = executeInternalManagementHandler(service.saveProviderKeyHandler(), http.MethodPut, "/api/management/tenants/managed-default/provider-keys/openai", `{"api_key":"sk","text_model":"","system_prompt":""}`, providerParams, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("save key settings status=%d", response.Code)
	}
	service, database = newSeededService()
	database.saveProviderKeyErrors = []error{errInternalTestDatabase}
	response = executeInternalManagementHandler(service.saveProviderKeyHandler(), http.MethodPut, "/api/management/tenants/managed-default/provider-keys/openai", `{"api_key":"sk","text_model":"`+ModelNameGPT41+`","system_prompt":""}`, providerParams, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("save key store status=%d", response.Code)
	}

	service, _ = newSeededService()
	response = executeInternalManagementHandler(service.removeProviderKeyHandler(), http.MethodDelete, "/api/management/tenants/%20/provider-keys/openai", "", gin.Params{{Key: "tenant_id", Value: " "}}, principal)
	if response.Code != http.StatusNotFound {
		t.Fatalf("remove key invalid tenant status=%d", response.Code)
	}
	response = executeInternalManagementHandler(service.removeProviderKeyHandler(), http.MethodDelete, "/api/management/tenants/managed-default/provider-keys/missing", "", invalidProviderParams, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("remove key provider status=%d", response.Code)
	}
	service, database = newSeededService()
	database.deleteProviderKeyErrors = []error{errInternalTestDatabase}
	response = executeInternalManagementHandler(service.removeProviderKeyHandler(), http.MethodDelete, "/api/management/tenants/managed-default/provider-keys/openai", "", providerParams, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("remove key store status=%d", response.Code)
	}

	service, _ = newSeededService()
	response = executeInternalManagementHandler(service.revealProviderKeyHandler(), http.MethodPost, "/api/management/tenants/%20/provider-keys/openai/reveal", "", gin.Params{{Key: "tenant_id", Value: " "}}, principal)
	if response.Code != http.StatusNotFound {
		t.Fatalf("reveal invalid tenant status=%d", response.Code)
	}
	response = executeInternalManagementHandler(service.revealProviderKeyHandler(), http.MethodPost, "/api/management/tenants/managed-default/provider-keys/missing/reveal", "", invalidProviderParams, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("reveal invalid provider status=%d", response.Code)
	}
	response = executeInternalManagementHandler(service.revealProviderKeyHandler(), http.MethodPost, "/api/management/tenants/managed-default/provider-keys/openai/reveal", "", providerParams, principal)
	if response.Code != http.StatusNotFound {
		t.Fatalf("reveal missing status=%d", response.Code)
	}
	service, database = newSeededService()
	record := database.tenantsByID["managed-default"]
	record.ProviderAPIKeys = []managedProviderAPIKeyRecord{{
		TenantID: "managed-default", ProviderID: ProviderNameOpenAI, EncryptedAPIKey: "invalid",
	}}
	database.tenantsByID["managed-default"] = record
	response = executeInternalManagementHandler(service.revealProviderKeyHandler(), http.MethodPost, "/api/management/tenants/managed-default/provider-keys/openai/reveal", "", providerParams, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("reveal store status=%d", response.Code)
	}

	service, _ = newSeededService()
	response = executeInternalManagementHandler(service.updateDefaultsHandler(), http.MethodPut, "/api/management/tenants/%20/defaults", `{}`, gin.Params{{Key: "tenant_id", Value: " "}}, principal)
	if response.Code != http.StatusNotFound {
		t.Fatalf("defaults invalid tenant status=%d", response.Code)
	}
	response = executeInternalManagementHandler(service.updateDefaultsHandler(), http.MethodPut, "/api/management/tenants/managed-default/defaults", "{", tenantParams, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("defaults decode status=%d", response.Code)
	}
	response = executeInternalManagementHandler(service.updateDefaultsHandler(), http.MethodPut, "/api/management/tenants/managed-default/defaults", `{"provider":"openai","model":"`+ModelNameGPT41+`","dictation_provider":"openai","dictation_model":"`+DefaultDictationModel+`","system_prompt":""}`, tenantParams, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("defaults reasoning status=%d", response.Code)
	}
	response = executeInternalManagementHandler(service.updateDefaultsHandler(), http.MethodPut, "/api/management/tenants/managed-default/defaults", managementDefaultsBody("missing", ModelNameGPT41), tenantParams, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("defaults construction status=%d", response.Code)
	}
	service, database = newSeededService()
	database.tenantByOwnerAndIDErrors = []error{errInternalTestDatabase}
	response = executeInternalManagementHandler(service.updateDefaultsHandler(), http.MethodPut, "/api/management/tenants/managed-default/defaults", managementDefaultsBody(ProviderNameOpenAI, ModelNameGPT41), tenantParams, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("defaults profile status=%d", response.Code)
	}
	noKeyProviders := newProviderRegistry(Configuration{
		ProviderModels: ProviderModelCatalogs{
			ProviderNameOpenAI: {
				Text: ModelEndpointCatalog{
					DefaultModel: ModelNameGPT41,
					Models:       []ModelConfiguration{{ID: ModelNameGPT41}},
				},
				Dictation: ModelEndpointCatalog{
					DefaultModel: DefaultDictationModel,
					Models:       []ModelConfiguration{{ID: DefaultDictationModel}},
				},
			},
		},
	})
	database = newFakeManagedTenantDatabase()
	fakeUserWithTenant(database, principal, "managed-default", "Default", now)
	service = newInternalManagementService(t, database, noKeyProviders)
	response = executeInternalManagementHandler(service.updateDefaultsHandler(), http.MethodPut, "/api/management/tenants/managed-default/defaults", managementDefaultsBody(ProviderNameOpenAI, ModelNameGPT41), tenantParams, principal)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("defaults validation status=%d body=%q", response.Code, response.Body.String())
	}
	service, database = newSeededService()
	service.store.randomReader = strings.NewReader(strings.Repeat("x", 64))
	if _, saveError := service.store.saveProviderKey(principal, "managed-default", newProviderID(ProviderNameOpenAI), "sk-openai", ModelNameGPT41, ""); saveError != nil {
		t.Fatalf("seed defaults provider key: %v", saveError)
	}
	database.saveTenantErrors = []error{errInternalTestDatabase}
	response = executeInternalManagementHandler(service.updateDefaultsHandler(), http.MethodPut, "/api/management/tenants/managed-default/defaults", managementDefaultsBody(ProviderNameOpenAI, ModelNameGPT41), tenantParams, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("defaults store status=%d", response.Code)
	}

	service, _ = newSeededService()
	response = executeInternalManagementHandler(service.generateSecretHandler(), http.MethodPost, "/api/management/tenants/%20/secrets", "", gin.Params{{Key: "tenant_id", Value: " "}}, principal)
	if response.Code != http.StatusNotFound {
		t.Fatalf("generate invalid tenant status=%d", response.Code)
	}
	service, _ = newSeededService()
	service.store.randomReader = strings.NewReader("")
	response = executeInternalManagementHandler(service.generateSecretHandler(), http.MethodPost, "/api/management/tenants/managed-default/secrets", "", tenantParams, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("generate store status=%d", response.Code)
	}
	service, database = newSeededService()
	record = database.tenantsByID["managed-default"]
	record.DefaultProvider = "missing"
	database.tenantsByID["managed-default"] = record
	response = executeInternalManagementHandler(service.generateSecretHandler(), http.MethodPost, "/api/management/tenants/managed-default/secrets", "", tenantParams, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("generate profile status=%d", response.Code)
	}

	service, _ = newSeededService()
	response = executeInternalManagementHandler(service.revokeSecretHandler(), http.MethodDelete, "/api/management/tenants/%20/secrets", "", gin.Params{{Key: "tenant_id", Value: " "}}, principal)
	if response.Code != http.StatusNotFound {
		t.Fatalf("revoke invalid tenant status=%d", response.Code)
	}
	service, database = newSeededService()
	database.saveTenantErrors = []error{errInternalTestDatabase}
	response = executeInternalManagementHandler(service.revokeSecretHandler(), http.MethodDelete, "/api/management/tenants/managed-default/secrets", "", tenantParams, principal)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("revoke store status=%d", response.Code)
	}
}

func TestManagementResponseErrorMappings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		storeError error
		statusCode int
	}{
		{storeError: gorm.ErrRecordNotFound, statusCode: http.StatusInternalServerError},
		{storeError: errManagedTenantNotFound, statusCode: http.StatusNotFound},
		{storeError: errManagedTenantNameConflict, statusCode: http.StatusConflict},
		{storeError: errManagedFinalTenantDeletion, statusCode: http.StatusConflict},
		{storeError: errManagedTenantNameInvalid, statusCode: http.StatusBadRequest},
		{storeError: errManagedProviderKeyInvalid, statusCode: http.StatusBadRequest},
		{storeError: errInternalTestDatabase, statusCode: http.StatusInternalServerError},
	} {
		recorder := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(recorder)
		writeManagementStoreError(ginContext, testCase.storeError)
		if recorder.Code != testCase.statusCode {
			t.Fatalf("error=%v status=%d want=%d", testCase.storeError, recorder.Code, testCase.statusCode)
		}
	}

	service, _ := newSeededInternalManagementService(t)
	invalidSnapshot := managedTenantSnapshot{
		tenantID:   "managed-default",
		tenantName: "Default",
		defaults: TenantDefaults{
			Provider: "missing",
		},
	}
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	service.writeTenantProfileResponse(ginContext, invalidSnapshot, http.StatusOK)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("profile response status=%d", recorder.Code)
	}
	if _, profileError := service.tenantProfileResponse(invalidSnapshot); profileError == nil {
		t.Fatal("invalid profile response succeeded")
	}
}

func newInternalManagementService(t *testing.T, database *fakeManagedTenantDatabase, providers *providerRegistry) *managementService {
	t.Helper()
	configuration := ManagementConfiguration{
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
		JWTSigningKey:            "management-signing-key",
		JWTIssuer:                DefaultManagementJWTIssuer,
		SessionCookieName:        "llm_proxy_test_session",
		ProviderKeyEncryptionKey: testManagedProviderKeyEncryptionKey,
		ManagementAPIOrigin:      "http://localhost:8080",
		ProxyOrigin:              "http://localhost:8080",
	}
	sessionValidator, validationError := newManagementSessionValidator(configuration)
	if validationError != nil {
		t.Fatalf("new session validator: %v", validationError)
	}
	store := newManagedTenantStoreWithDatabase(database)
	store.routingDefaults = providers
	return newManagementService(
		configuration,
		sessionValidator,
		store,
		providers,
		newTenantAuthenticator(tenantRegistry{}, store),
		zap.NewNop().Sugar(),
	)
}

func newSeededInternalManagementService(t *testing.T) (*managementService, *fakeManagedTenantDatabase) {
	t.Helper()
	database := newFakeManagedTenantDatabase()
	principal := managementPrincipal{userID: "tauth-handler-user", userEmail: "owner@example.com"}
	fakeUserWithTenant(database, principal, "managed-default", "Default", time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC))
	return newInternalManagementService(t, database, internalManagementProviderRegistry()), database
}

func executeInternalManagementHandler(handler gin.HandlerFunc, method string, path string, body string, params gin.Params, principal managementPrincipal) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	ginContext.Params = params
	ginContext.Set(contextKeyManagementPrincipal, principal)
	handler(ginContext)
	return response
}

func managementDefaultsBody(provider string, model string) string {
	return `{"provider":"` + provider + `","model":"` + model + `","dictation_provider":"openai","dictation_model":"` + DefaultDictationModel + `","system_prompt":"","reasoning_effort":""}`
}
