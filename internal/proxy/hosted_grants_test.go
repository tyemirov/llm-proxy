package proxy_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
)

type hostedGrantTestResponse struct {
	ID                   string `json:"id"`
	BillingAccountID     string `json:"billing_account_id"`
	TenantID             string `json:"tenant_id"`
	State                string `json:"state"`
	Revision             uint64 `json:"revision"`
	PlatformConnectionID string `json:"platform_connection_id"`
}

func TestHostedGrantBrowserPreflight(t *testing.T) {
	server := httptest.NewServer(newManagementRouter(t, proxy.Configuration{}))
	defer server.Close()
	request, err := http.NewRequest(http.MethodOptions, server.URL+"/api/management/hosted-access-grants/grant-test", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "http://localhost:8080")
	request.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	request.Header.Set("Access-Control-Request-Headers", "content-type")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent || !slices.Contains(strings.Split(response.Header.Get("Access-Control-Allow-Methods"), ", "), http.MethodPatch) {
		t.Fatalf("grant browser preflight: status=%d allowed=%q", response.StatusCode, response.Header.Get("Access-Control-Allow-Methods"))
	}
}

func hostedResourceID(t *testing.T, response hostedHTTPResponse) string {
	t.Helper()
	var resource struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.body, &resource); err != nil {
		t.Fatal(err)
	}
	if resource.ID == "" {
		t.Fatalf("missing resource identity: %s", response.body)
	}
	return resource.ID
}

func TestHostedGrantAuthorityTransitionsAndRestart(t *testing.T) {
	catalog := testfixtures.ProviderCatalog(t)
	configuration := proxy.Configuration{ProviderCatalog: catalog}
	databasePath := filepath.Join(t.TempDir(), "grants.db")
	router := newManagementRouterWithDatabasePath(t, configuration, databasePath)
	server := httptest.NewServer(router)
	defer server.Close()
	operator := managementSessionCookieWithEmail(t, "grant-operator", testManagementAdminEmail)
	owner := managementSessionCookie(t, "grant-owner")
	other := managementSessionCookie(t, "grant-other")
	tenant := requestManagementAccount(t, router, owner).Tenants[0].ID
	otherTenant := requestManagementAccount(t, router, other).Tenants[0].ID
	account := hostedResourceID(t, requireHostedHTTP(t, server, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "billing", http.StatusCreated))
	connection := hostedResourceID(t, requireHostedHTTP(t, server, operator, http.MethodPost, "/platform-connections", `{"name":"Grant provider","provider":"openai","fields":{"api_key":"sk-user-openai"}}`, "platform", http.StatusCreated))
	const collection = "/hosted-access-grants"
	intent := fmt.Sprintf(`{"billing_account_id":%q,"tenant_id":%q,"platform_connection_id":%q,"catalog_revision":%q,"offerings":[{"model":"gpt-6-astra","operations":["text"]}],"reason":"Customer enrollment"}`, account, tenant, connection, catalog.ModelCatalog().Revision)
	requireHostedHTTP(t, server, nil, http.MethodPost, collection, intent, "grant", http.StatusUnauthorized)
	requireHostedHTTP(t, server, owner, http.MethodPost, collection, intent, "grant", http.StatusForbidden)
	requireHostedHTTP(t, server, operator, http.MethodPost, collection, strings.Replace(intent, tenant, otherTenant, 1), "foreign-tenant", http.StatusNotFound)
	requireHostedHTTP(t, server, operator, http.MethodPost, collection, strings.Replace(intent, "gpt-6-astra", "unknown-model", 1), "bad-offering", http.StatusBadRequest)
	requireHostedHTTP(t, server, operator, http.MethodPost, collection, strings.Replace(intent, `"text"`, `"unknown-operation"`, 1), "bad-operation", http.StatusBadRequest)
	requireHostedHTTP(t, server, operator, http.MethodPost, collection, strings.Replace(intent, catalog.ModelCatalog().Revision, "stale-catalog", 1), "stale-catalog", http.StatusConflict)
	created := requireHostedHTTP(t, server, operator, http.MethodPost, collection, intent, "grant", http.StatusCreated)
	var grant hostedGrantTestResponse
	if err := json.Unmarshal(created.body, &grant); err != nil {
		t.Fatal(err)
	}
	if grant.ID == "" || grant.State != "active" || grant.Revision != 1 || grant.BillingAccountID != account || grant.TenantID != tenant || grant.PlatformConnectionID != connection {
		t.Fatalf("invalid grant: %s", created.body)
	}
	path := collection + "/" + grant.ID
	if created.header.Get("Location") != "/api/management"+path {
		t.Fatal("missing grant location")
	}
	replayed := requireHostedHTTP(t, server, operator, http.MethodPost, collection, intent, "grant", http.StatusCreated)
	if string(replayed.body) != string(created.body) {
		t.Fatal("creation replay changed")
	}
	requireHostedHTTP(t, server, operator, http.MethodPost, collection, strings.Replace(intent, "Customer enrollment", "Changed intent", 1), "grant", http.StatusConflict)
	owned := requireHostedHTTP(t, server, owner, http.MethodGet, path, "", "", http.StatusOK)
	if strings.Contains(string(owned.body), connection) || strings.Contains(string(owned.body), "sk-user-") || strings.Contains(string(owned.body), "Customer enrollment") {
		t.Fatalf("customer read exposed operator data: %s", owned.body)
	}
	requireHostedHTTP(t, server, other, http.MethodGet, path, "", "", http.StatusNotFound)
	foreignList := requireHostedHTTP(t, server, other, http.MethodGet, collection, "", "", http.StatusOK)
	if strings.Contains(string(foreignList.body), grant.ID) {
		t.Fatal("grant list crossed account ownership")
	}
	requireHostedHTTP(t, server, owner, http.MethodPatch, path, `{"state":"suspended","revision":1,"reason":"Review"}`, "", http.StatusForbidden)
	requireHostedHTTP(t, server, operator, http.MethodPatch, path, `{"state":"suspended","revision":0,"reason":"Review"}`, "", http.StatusBadRequest)
	database := openManagedFixtureDatabase(t, databasePath)
	if err := database.Exec("CREATE TRIGGER reject_grant_audit BEFORE INSERT ON managed_hosted_grant_revision_records BEGIN SELECT RAISE(ABORT, 'controlled audit failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	requireHostedHTTP(t, server, operator, http.MethodPatch, path, `{"state":"suspended","revision":1,"reason":"Audit failure"}`, "", http.StatusInternalServerError)
	unchanged := requireHostedHTTP(t, server, operator, http.MethodGet, path, "", "", http.StatusOK)
	if string(unchanged.body) != string(created.body) {
		t.Fatal("failed audit committed a grant transition")
	}
	if err := database.Exec("DROP TRIGGER reject_grant_audit").Error; err != nil {
		t.Fatal(err)
	}
	for index, state := range []string{"suspended", "active", "revoked"} {
		body := fmt.Sprintf(`{"state":%q,"revision":%d,"reason":"Operator review"}`, state, index+1)
		updated := requireHostedHTTP(t, server, operator, http.MethodPatch, path, body, "", http.StatusOK)
		if err := json.Unmarshal(updated.body, &grant); err != nil {
			t.Fatal(err)
		}
		if grant.State != state || grant.Revision != uint64(index+2) {
			t.Fatalf("invalid transition: %s", updated.body)
		}
		requireHostedHTTP(t, server, operator, http.MethodPatch, path, body, "", http.StatusConflict)
	}
	requireHostedHTTP(t, server, operator, http.MethodPatch, path, `{"state":"active","revision":4,"reason":"Revoked is terminal"}`, "", http.StatusConflict)
	audit := requireHostedHTTP(t, server, operator, http.MethodGet, path+"/revisions", "", "", http.StatusOK)
	var history struct {
		Revisions []struct {
			Revision uint64 `json:"revision"`
			State    string `json:"state"`
			Actor    string `json:"actor_user_id"`
			Reason   string `json:"reason"`
		} `json:"revisions"`
	}
	if err := json.Unmarshal(audit.body, &history); err != nil {
		t.Fatal(err)
	}
	if len(history.Revisions) != 4 {
		t.Fatalf("incomplete audit: %s", audit.body)
	}
	for index, entry := range history.Revisions {
		if entry.Revision != uint64(index+1) || entry.Actor != "grant-operator" || entry.Reason == "" {
			t.Fatalf("invalid audit: %s", audit.body)
		}
	}
	requireHostedHTTP(t, server, owner, http.MethodGet, path+"/revisions", "", "", http.StatusForbidden)
	first := requireHostedHTTP(t, server, operator, http.MethodGet, path+"/revisions?limit=2", "", "", http.StatusOK)
	var page struct {
		Revisions []json.RawMessage `json:"revisions"`
		Cursor    string            `json:"next_cursor"`
	}
	if err := json.Unmarshal(first.body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Revisions) != 2 || page.Cursor != "2" {
		t.Fatalf("invalid first audit page: %s", first.body)
	}
	last := requireHostedHTTP(t, server, operator, http.MethodGet, path+"/revisions?limit=2&cursor="+page.Cursor, "", "", http.StatusOK)
	if err := json.Unmarshal(last.body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Revisions) != 2 || page.Cursor != "" {
		t.Fatalf("invalid last audit page: %s", last.body)
	}
	for _, query := range []string{"limit=0", "limit=101", "limit=x", "cursor=0", "cursor=-1", "cursor=01", "cursor=x", "unknown=1", "limit=1&limit=2"} {
		requireHostedHTTP(t, server, operator, http.MethodGet, path+"/revisions?"+query, "", "", http.StatusBadRequest)
	}
	server.Close()
	changedSchema := catalog.Schema()
	changedSchema.Providers[0].Label += " catalog update"
	changedCatalog, err := proxy.NewProviderCatalog(changedSchema)
	if err != nil {
		t.Fatal(err)
	}
	restarted := httptest.NewServer(newManagementRouterWithDatabasePath(t, proxy.Configuration{ProviderCatalog: changedCatalog}, databasePath))
	defer restarted.Close()
	after := requireHostedHTTP(t, restarted, operator, http.MethodGet, path+"/revisions", "", "", http.StatusOK)
	if string(after.body) != string(audit.body) {
		t.Fatal("restart changed grant audit")
	}
	replayAfterRevocation := requireHostedHTTP(t, restarted, operator, http.MethodPost, collection, intent, "grant", http.StatusCreated)
	if string(replayAfterRevocation.body) != string(created.body) {
		t.Fatal("revocation changed original creation receipt")
	}
	retained := requireHostedHTTP(t, restarted, owner, http.MethodGet, path, "", "", http.StatusOK)
	if err := json.Unmarshal(retained.body, &grant); err != nil {
		t.Fatal(err)
	}
	if grant.State != "revoked" || grant.Revision != 4 {
		t.Fatalf("restart lost revocation: %s", retained.body)
	}
	requireHostedHTTP(t, restarted, owner, http.MethodPost, "/tenants", `{"name":"Other tenant"}`, "", http.StatusCreated)
	deletion := requireHostedHTTP(t, restarted, owner, http.MethodDelete, "/tenants/"+tenant, `{}`, "", http.StatusConflict)
	if string(deletion.body) != "managed_tenant_hosted_history" {
		t.Fatalf("missing hosted history conflict: %s", deletion.body)
	}
	requireHostedHTTP(t, restarted, owner, http.MethodGet, "/tenants/"+tenant, "", "", http.StatusOK)
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		path, method string
		response     hostedHTTPResponse
	}{
		{collection, http.MethodPost, created},
		{collection, http.MethodGet, foreignList},
		{collection + "/{grant_id}", http.MethodGet, owned},
		{collection + "/{grant_id}", http.MethodGet, unchanged},
		{collection + "/{grant_id}", http.MethodGet, retained},
		{collection + "/{grant_id}/revisions", http.MethodGet, audit},
		{"/tenants/{tenant_id}", http.MethodDelete, deletion},
	} {
		if err := contract.ValidateResponse("/api/management"+scenario.path, scenario.method, scenario.response.status, scenario.response.header, scenario.response.body); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHostedGrantConcurrentCreationAndRevision(t *testing.T) {
	catalog := testfixtures.ProviderCatalog(t)
	configuration := proxy.Configuration{ProviderCatalog: catalog}
	databasePath := filepath.Join(t.TempDir(), "concurrent.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	router := newManagementRouterWithDatabasePath(t, configuration, databasePath)
	first := httptest.NewServer(router)
	defer first.Close()
	operator := managementSessionCookieWithEmail(t, "concurrent-grant-operator", testManagementAdminEmail)
	owner := managementSessionCookie(t, "concurrent-grant-owner")
	tenant := requestManagementAccount(t, router, owner).Tenants[0].ID
	account := hostedResourceID(t, requireHostedHTTP(t, first, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "billing", http.StatusCreated))
	connection := hostedResourceID(t, requireHostedHTTP(t, first, operator, http.MethodPost, "/platform-connections", `{"name":"Grant provider","provider":"openai","fields":{"api_key":"sk-user-openai"}}`, "platform", http.StatusCreated))
	second := httptest.NewServer(newManagementRouterWithDatabasePath(t, configuration, databasePath))
	defer second.Close()
	servers := []*httptest.Server{first, second}
	const collection = "/hosted-access-grants"
	intent := fmt.Sprintf(`{"billing_account_id":%q,"tenant_id":%q,"platform_connection_id":%q,"catalog_revision":%q,"offerings":[{"model":"gpt-6-astra","operations":["text"]}],"reason":"Concurrent enrollment"}`, account, tenant, connection, catalog.ModelCatalog().Revision)
	const requests = 6
	var group sync.WaitGroup
	responses := make([]hostedHTTPResponse, requests)
	failures := make([]error, requests)
	for index := range requests {
		group.Add(1)
		go func() {
			defer group.Done()
			server := servers[index%len(servers)]
			responses[index], failures[index] = hostedHTTPExchange(server.Client(), server.URL, operator, http.MethodPost, collection, intent, "concurrent-grant")
		}()
	}
	group.Wait()
	for index, response := range responses {
		if failures[index] != nil || response.status != http.StatusCreated {
			t.Fatalf("concurrent creation: err=%v status=%d body=%s", failures[index], response.status, response.body)
		}
		if string(response.body) != string(responses[0].body) {
			t.Fatal("concurrent creation produced different grants")
		}
	}
	grantID := hostedResourceID(t, responses[0])
	path := collection + "/" + grantID
	for index := range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			server := servers[index]
			state := []string{"suspended", "revoked"}[index]
			body := fmt.Sprintf(`{"state":%q,"revision":1,"reason":"Concurrent transition"}`, state)
			responses[index], failures[index] = hostedHTTPExchange(server.Client(), server.URL, operator, http.MethodPatch, path, body, "")
		}()
	}
	group.Wait()
	successes, conflicts := 0, 0
	for index, response := range responses[:2] {
		if failures[index] != nil {
			t.Fatal(failures[index])
		}
		switch response.status {
		case http.StatusOK:
			successes++
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("concurrent update: status=%d body=%s", response.status, response.body)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("revision precondition: successes=%d conflicts=%d", successes, conflicts)
	}
	audit := requireHostedHTTP(t, first, operator, http.MethodGet, path+"/revisions", "", "", http.StatusOK)
	var history struct {
		Revisions []json.RawMessage `json:"revisions"`
	}
	if err := json.Unmarshal(audit.body, &history); err != nil {
		t.Fatal(err)
	}
	if len(history.Revisions) != 2 {
		t.Fatalf("concurrent transition has duplicate audit: %s", audit.body)
	}
	secondGrant := requireHostedHTTP(t, second, operator, http.MethodPost, collection, intent, "another-grant", http.StatusCreated)
	if hostedResourceID(t, secondGrant) == grantID {
		t.Fatal("distinct creation key reused grant")
	}
	firstPage := requireHostedHTTP(t, first, owner, http.MethodGet, collection+"?limit=1", "", "", http.StatusOK)
	var page struct {
		Grants []hostedGrantTestResponse `json:"hosted_access_grants"`
		Cursor string                    `json:"next_cursor"`
	}
	if err := json.Unmarshal(firstPage.body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Grants) != 1 || page.Cursor != page.Grants[0].ID || page.Grants[0].PlatformConnectionID != "" {
		t.Fatalf("invalid grant page: %s", firstPage.body)
	}
	prior := page.Grants[0].ID
	lastPage := requireHostedHTTP(t, second, owner, http.MethodGet, collection+"?limit=1&cursor="+page.Cursor, "", "", http.StatusOK)
	if err := json.Unmarshal(lastPage.body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Grants) != 1 || page.Cursor != "" || page.Grants[0].ID <= prior || page.Grants[0].PlatformConnectionID != "" {
		t.Fatalf("invalid final grant page: %s", lastPage.body)
	}
}
