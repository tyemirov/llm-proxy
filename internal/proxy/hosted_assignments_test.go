package proxy_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
)

func TestHostedAssignmentExplicitChoiceIsolationAndRestart(t *testing.T) {
	var dispatches atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		dispatches.Add(1)
		writer.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()
	catalog := testfixtures.ProviderCatalog(t)
	configuration := proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpoints(upstream.URL, proxy.ProviderNameOpenAI)}
	databasePath := filepath.Join(t.TempDir(), "assignments.db")
	router := newManagementRouterWithDatabasePath(t, configuration, databasePath)
	server := httptest.NewServer(router)
	defer server.Close()
	operator := managementSessionCookieWithEmail(t, "assignment-operator", testManagementAdminEmail)
	owner := managementSessionCookie(t, "assignment-owner")
	other := managementSessionCookie(t, "assignment-other")
	tenant := requestManagementAccount(t, router, owner).Tenants[0].ID
	otherTenant := requestManagementAccount(t, router, other).Tenants[0].ID
	account := hostedResourceID(t, requireHostedHTTP(t, server, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "billing", http.StatusCreated))
	platform := hostedResourceID(t, requireHostedHTTP(t, server, operator, http.MethodPost, "/platform-connections", `{"name":"Hosted","provider":"openai","fields":{"api_key":"sk-user-openai"}}`, "platform", http.StatusCreated))
	grantIntent := fmt.Sprintf(`{"billing_account_id":%q,"tenant_id":%q,"platform_connection_id":%q,"catalog_revision":%q,"offerings":[{"model":"gpt-6-astra","operations":["text"]}],"reason":"Assignment test"}`, account, tenant, platform, catalog.ModelCatalog().Revision)
	grant := hostedResourceID(t, requireHostedHTTP(t, server, operator, http.MethodPost, "/hosted-access-grants", grantIntent, "grant", http.StatusCreated))
	connection := hostedResourceID(t, requireHostedHTTP(t, server, owner, http.MethodPost, "/connections", `{"name":"Customer","provider":"openai","version":0,"fields":{"api_key":"sk-user-customer"}}`, "connection", http.StatusCreated))
	collection := "/tenants/" + tenant + "/connections"
	path := collection + "/openai"
	before := requireHostedHTTP(t, server, owner, http.MethodGet, collection, "", "", http.StatusOK)
	if string(before.body) != `{"assignments":[]}` {
		t.Fatalf("unexpected initial assignments: %s", before.body)
	}
	for _, invalid := range []string{fmt.Sprintf(`{"connection_id":%q}`, connection), `{"kind":"automatic","resource_id":"unknown"}`, `{"kind":"hosted_access_grant","resource_id":""}`} {
		requireHostedHTTP(t, server, owner, http.MethodPut, path, invalid, "", http.StatusBadRequest)
	}
	hosted := fmt.Sprintf(`{"kind":"hosted_access_grant","resource_id":%q}`, grant)
	owned := fmt.Sprintf(`{"kind":"account_connection","resource_id":%q}`, connection)
	requireHostedHTTP(t, server, other, http.MethodPut, path, hosted, "", http.StatusNotFound)
	requireHostedHTTP(t, server, other, http.MethodGet, collection, "", "", http.StatusNotFound)
	requireHostedHTTP(t, server, other, http.MethodPut, "/tenants/"+otherTenant+"/connections/openai", hosted, "", http.StatusNotFound)
	requireHostedHTTP(t, server, owner, http.MethodPut, collection+"/gemini", hosted, "", http.StatusNotFound)
	assigned := requireHostedHTTP(t, server, owner, http.MethodPut, path, hosted, "", http.StatusOK)
	if strings.Contains(string(assigned.body), platform) || strings.Contains(string(assigned.body), "sk-user-") {
		t.Fatalf("hosted assignment exposed private fields: %s", assigned.body)
	}
	requireHostedHTTP(t, server, owner, http.MethodPut, path, hosted, "", http.StatusOK)
	profilePath := "/tenants/" + tenant + "/provider-profiles/openai"
	profile := `{"text_model":"gpt-6-astra","system_prompt":"Customer instructions"}`
	requireHostedHTTP(t, server, other, http.MethodPut, profilePath, profile, "", http.StatusNotFound)
	requireHostedHTTP(t, server, owner, http.MethodPut, profilePath, profile, "", http.StatusOK)
	requireHostedHTTP(t, server, owner, http.MethodPut, path, owned, "", http.StatusConflict)
	listing := requireHostedHTTP(t, server, owner, http.MethodGet, collection, "", "", http.StatusOK)
	database := openManagedFixtureDatabase(t, databasePath)
	if err := database.Exec("INSERT INTO managed_tenant_connection_records (tenant_id,provider_id,connection_id) VALUES (?,?,?)", tenant, "openai", connection).Error; err == nil {
		t.Fatal("database allowed account credentials beside a hosted assignment")
	}
	var assignments struct {
		Assignments []struct {
			Kind       string `json:"kind"`
			ResourceID string `json:"resource_id"`
			Provider   string `json:"provider"`
		} `json:"assignments"`
	}
	if err := json.Unmarshal(listing.body, &assignments); err != nil {
		t.Fatal(err)
	}
	if len(assignments.Assignments) != 1 || assignments.Assignments[0].Kind != "hosted_access_grant" || assignments.Assignments[0].ResourceID != grant || assignments.Assignments[0].Provider != "openai" {
		t.Fatalf("invalid assignment representation: %s", listing.body)
	}
	secret := generateManagementTenantSecret(t, router, owner, tenant)
	request, err := http.NewRequest(http.MethodGet, server.URL+"/?key="+secret+"&provider=openai&model=gpt-6-astra&prompt=test", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Idempotency-Key", "disabled-hosted-execution")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden || dispatches.Load() != 0 {
		t.Fatalf("hosted execution bypassed admission: status=%d body=%s calls=%d", response.StatusCode, body, dispatches.Load())
	}
	server.Close()
	restarted := httptest.NewServer(newManagementRouterWithDatabasePath(t, configuration, databasePath))
	defer restarted.Close()
	after := requireHostedHTTP(t, restarted, owner, http.MethodGet, collection, "", "", http.StatusOK)
	if string(after.body) != string(listing.body) {
		t.Fatal("restart changed the assignment")
	}
	retained := requireHostedHTTP(t, restarted, owner, http.MethodGet, "/tenants/"+tenant, "", "", http.StatusOK)
	if !strings.Contains(string(retained.body), "Customer instructions") {
		t.Fatalf("restart lost the hosted provider profile: %s", retained.body)
	}
	requireHostedHTTP(t, restarted, owner, http.MethodDelete, path, "", "", http.StatusNoContent)
	requireHostedHTTP(t, restarted, owner, http.MethodPut, path, owned, "", http.StatusOK)
	if err := database.Exec("INSERT INTO managed_hosted_tenant_assignment_records (tenant_id,provider_id,grant_id) VALUES (?,?,?)", tenant, "openai", grant).Error; err == nil {
		t.Fatal("database allowed a hosted grant beside account credentials")
	}
	requireHostedHTTP(t, restarted, owner, http.MethodPut, path, hosted, "", http.StatusConflict)
	ownedListing := requireHostedHTTP(t, restarted, owner, http.MethodGet, collection, "", "", http.StatusOK)
	if !strings.Contains(string(ownedListing.body), `"kind":"account_connection"`) || !strings.Contains(string(ownedListing.body), connection) {
		t.Fatalf("missing customer credential selection: %s", ownedListing.body)
	}
	requireHostedHTTP(t, restarted, owner, http.MethodDelete, path, "", "", http.StatusNoContent)
	requireHostedHTTP(t, restarted, operator, http.MethodPatch, "/hosted-access-grants/"+grant, `{"revision":1,"state":"suspended","reason":"Stop new assignments"}`, "", http.StatusOK)
	requireHostedHTTP(t, restarted, owner, http.MethodPut, path, hosted, "", http.StatusConflict)
}

func TestHostedAssignmentConcurrentCredentialChoices(t *testing.T) {
	catalog := testfixtures.ProviderCatalog(t)
	configuration := proxy.Configuration{ProviderCatalog: catalog}
	databasePath := filepath.Join(t.TempDir(), "assignments.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	router := newManagementRouterWithDatabasePath(t, configuration, databasePath)
	first := httptest.NewServer(router)
	defer first.Close()
	operator := managementSessionCookieWithEmail(t, "choices-operator", testManagementAdminEmail)
	owner := managementSessionCookie(t, "choices-owner")
	tenant := requestManagementAccount(t, router, owner).Tenants[0].ID
	account := hostedResourceID(t, requireHostedHTTP(t, first, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "billing", http.StatusCreated))
	platform := hostedResourceID(t, requireHostedHTTP(t, first, operator, http.MethodPost, "/platform-connections", `{"name":"Hosted","provider":"openai","fields":{"api_key":"sk-user-openai"}}`, "platform", http.StatusCreated))
	grantIntent := fmt.Sprintf(`{"billing_account_id":%q,"tenant_id":%q,"platform_connection_id":%q,"catalog_revision":%q,"offerings":[{"model":"gpt-6-astra","operations":["text"]}],"reason":"Concurrent choice"}`, account, tenant, platform, catalog.ModelCatalog().Revision)
	grant := hostedResourceID(t, requireHostedHTTP(t, first, operator, http.MethodPost, "/hosted-access-grants", grantIntent, "grant", http.StatusCreated))
	connection := hostedResourceID(t, requireHostedHTTP(t, first, owner, http.MethodPost, "/connections", `{"name":"Customer","provider":"openai","version":0,"fields":{"api_key":"sk-user-customer"}}`, "connection", http.StatusCreated))
	second := httptest.NewServer(newManagementRouterWithDatabasePath(t, configuration, databasePath))
	defer second.Close()
	servers := []*httptest.Server{first, second}
	choices := []string{fmt.Sprintf(`{"kind":"account_connection","resource_id":%q}`, connection), fmt.Sprintf(`{"kind":"hosted_access_grant","resource_id":%q}`, grant)}
	path := "/tenants/" + tenant + "/connections/openai"
	for attempt := 0; attempt < 6; attempt++ {
		responses := make([]hostedHTTPResponse, 2)
		failures := make([]error, 2)
		var group sync.WaitGroup
		start := make(chan struct{})
		for index := range 2 {
			group.Add(1)
			go func() {
				defer group.Done()
				<-start
				server := servers[index]
				responses[index], failures[index] = hostedHTTPExchange(server.Client(), server.URL, owner, http.MethodPut, path, choices[index], "")
			}()
		}
		close(start)
		group.Wait()
		successes, conflicts := 0, 0
		for index, response := range responses {
			if failures[index] != nil {
				t.Fatal(failures[index])
			}
			switch response.status {
			case http.StatusOK:
				successes++
			case http.StatusConflict:
				conflicts++
			default:
				t.Fatalf("concurrent credential choice: status=%d body=%s", response.status, response.body)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf("choice arbitration: successes=%d conflicts=%d", successes, conflicts)
		}
		requireHostedHTTP(t, first, owner, http.MethodDelete, path, "", "", http.StatusNoContent)
	}
}
