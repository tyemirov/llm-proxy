package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// A detach committed between two database reads must not leave a saved route
// that uses the detached connection. A single locked read can save first.
type detachingDefaultsDatabase struct {
	*fakeManagedTenantDatabase
	reads int
}

func (database *detachingDefaultsDatabase) tenantByOwnerAndID(owner, tenant string) (managedTenantRecord, error) {
	database.reads++
	if database.reads == 2 {
		record := database.tenantsByID[tenant]
		record.ConnectionAssignments = nil
		database.tenantsByID[tenant] = record
	}
	return database.fakeManagedTenantDatabase.tenantByOwnerAndID(owner, tenant)
}

func TestAccountConnectionDefaultSaveUsesCurrentAssignment(t *testing.T) {
	service, database := newSeededInternalManagementService(t)
	principal := managementPrincipal{userID: "tauth-handler-user", userEmail: "owner@example.com"}
	record := database.tenantsByID["managed-default"]
	ciphertext, err := service.store.providerKeyCipher.encryptConnection(strings.NewReader(strings.Repeat("x", 64)), "test-connection", ProviderNameOpenAI, CatalogCredentialAPIKey, "sk-openai")
	if err != nil {
		t.Fatal(err)
	}
	record.ConnectionAssignments = []managedTenantConnectionRecord{{TenantID: record.TenantID, ProviderID: ProviderNameOpenAI, ConnectionID: "test-connection", Connection: managedAccountConnectionRecord{ID: "test-connection", OwnerUserID: principal.userID, ProviderID: ProviderNameOpenAI, Fields: []managedConnectionFieldRecord{{ConnectionID: "test-connection", FieldID: CatalogCredentialAPIKey, Value: ciphertext}}}}}
	record.ProviderProfiles = []managedProviderProfileRecord{{TenantID: record.TenantID, ProviderID: ProviderNameOpenAI, TextModel: ModelNameGPT41}}
	database.tenantsByID[record.TenantID] = record
	service.store.database = &detachingDefaultsDatabase{fakeManagedTenantDatabase: database}
	router := gin.New()
	router.Use(func(ctx *gin.Context) { ctx.Set(contextKeyManagementPrincipal, principal) })
	router.PUT("/api/management/tenants/:tenant_id/defaults", service.updateDefaultsHandler())
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	request, err := http.NewRequest(http.MethodPut, server.URL+"/api/management/tenants/managed-default/defaults", strings.NewReader(managementDefaultsBody(ProviderNameOpenAI, ModelNameGPT41)))
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	persisted := database.tenantsByID[record.TenantID]
	if len(persisted.ConnectionAssignments) == 0 && persisted.DefaultProvider != "" {
		t.Fatalf("default save retained detached provider: status=%d provider=%s", response.StatusCode, persisted.DefaultProvider)
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusBadRequest {
		t.Fatalf("default save status=%d", response.StatusCode)
	}
}
