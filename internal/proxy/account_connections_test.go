package proxy_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func accountConnectionExchange(t *testing.T, router http.Handler, cookie *http.Cookie, method, path string, body any, status int) map[string]any {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, authenticatedJSONRequest(method, "/api/management"+path, string(payload), cookie))
	if response.Code != status {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.Code, status, response.Body.String())
	}
	var value map[string]any
	if response.Body.Len() > 0 {
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
	}
	return value
}

func TestAccountConnectionInventoryPagination(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	owner := managementSessionCookie(t, "connection-page-owner")
	other := managementSessionCookie(t, "connection-page-other")
	exchange := func(method, path string, body any, status int) map[string]any {
		t.Helper()
		return accountConnectionExchange(t, router, owner, method, path, body, status)
	}
	for index := 0; index < 3; index++ {
		exchange(http.MethodPost, "/connections", map[string]any{"name": fmt.Sprintf("Connection %d", index), "provider": "openai", "fields": map[string]string{"api_key": testManagementOpenAIKey}}, http.StatusCreated)
	}
	accountConnectionExchange(t, router, other, http.MethodPost, "/connections", map[string]any{"name": "Other account", "provider": "openai", "fields": map[string]string{"api_key": testManagementOpenAIKey}}, http.StatusCreated)
	page := exchange(http.MethodGet, "/connections?limit=2", nil, http.StatusOK)
	connections := page["connections"].([]any)
	if len(connections) != 2 {
		t.Fatalf("page contains %d connections, want 2", len(connections))
	}
	cursor, valid := page["next_cursor"].(string)
	if !valid || cursor == "" {
		t.Fatal("page omits continuation cursor")
	}
	firstID := connections[0].(map[string]any)["id"].(string)
	secondID := connections[1].(map[string]any)["id"].(string)
	if firstID >= secondID || cursor != secondID {
		t.Fatal("inventory is not ordered by stable connection ID")
	}
	// Removing the cursor resource must not invalidate the next page.
	exchange(http.MethodDelete, "/connections/"+cursor, nil, http.StatusNoContent)
	next := exchange(http.MethodGet, "/connections?limit=2&cursor="+url.QueryEscape(cursor), nil, http.StatusOK)
	remaining := next["connections"].([]any)
	if len(remaining) != 1 || next["next_cursor"] != "" || remaining[0].(map[string]any)["id"].(string) <= cursor {
		t.Fatalf("invalid continuation: %v", next)
	}
	for _, query := range []string{"limit=0", "limit=101", "limit=two", "limit=2&limit=3", "cursor=invalid", "cursor=", "unexpected=1", "cursor=%zz", "limit=2;cursor=x"} {
		exchange(http.MethodGet, "/connections?"+query, nil, http.StatusBadRequest)
	}
}

func TestAccountConnectionResponsesAreNotCached(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	owner := managementSessionCookie(t, "connection-cache-owner")
	for _, scenario := range []struct {
		method, path, body string
		cookie             *http.Cookie
		status             int
	}{
		{http.MethodGet, "/connections", "", owner, http.StatusOK},
		{http.MethodGet, "/connections/missing", "", owner, http.StatusNotFound},
		{http.MethodPost, "/connections", `{}`, owner, http.StatusBadRequest},
		{http.MethodDelete, "/connections/missing", "", owner, http.StatusNoContent},
		{http.MethodGet, "/connections", "", nil, http.StatusUnauthorized},
	} {
		request := httptest.NewRequest(scenario.method, "/api/management"+scenario.path, strings.NewReader(scenario.body))
		request.Header.Set("Content-Type", "application/json")
		if scenario.cookie != nil {
			request.AddCookie(scenario.cookie)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != scenario.status || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s %s status=%d cache=%q", scenario.method, scenario.path, response.Code, response.Header().Get("Cache-Control"))
		}
	}
}

func TestAccountConnectionEditsRequireCurrentAssignmentPreview(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	owner := managementSessionCookie(t, "connection-edit-owner")
	tenantID := requestManagementAccount(t, router, owner).Tenants[0].ID
	secondTenant := createManagementTenant(t, router, owner, "Social Threader").Tenant.ID
	exchange := func(method, path string, body any, status int) map[string]any {
		t.Helper()
		return accountConnectionExchange(t, router, owner, method, path, body, status)
	}
	connection := exchange(http.MethodPost, "/connections", map[string]any{"name": "Production", "provider": "openai", "fields": map[string]string{"api_key": testManagementOpenAIKey}}, http.StatusCreated)
	id := connection["id"].(string)
	path := "/connections/" + id
	mutations := []struct {
		method string
		path   string
		body   any
		status int
	}{
		{http.MethodPut, "/tenants/" + tenantID + "/connections/openai", map[string]string{"connection_id": id}, http.StatusOK},
		{http.MethodPut, "/tenants/" + secondTenant + "/connections/openai", map[string]string{"connection_id": id}, http.StatusOK},
		{http.MethodDelete, "/tenants/" + tenantID + "/connections/openai", nil, http.StatusNoContent},
		{http.MethodDelete, "/tenants/" + secondTenant, nil, http.StatusNoContent},
	}
	for index, mutation := range mutations {
		before := exchange(http.MethodGet, path, nil, http.StatusOK)
		exchange(mutation.method, mutation.path, mutation.body, mutation.status)
		edit := map[string]any{"name": fmt.Sprintf("Edit %d", index), "provider": "openai", "version": before["version"], "fields": map[string]string{"api_key": ""}}
		exchange(http.MethodPut, path, edit, http.StatusConflict)
		after := exchange(http.MethodGet, path, nil, http.StatusOK)
		if after["name"] != before["name"] {
			t.Fatal("stale edit changed the connection")
		}
		edit["version"] = after["version"]
		saved := exchange(http.MethodPut, path, edit, http.StatusOK)
		if saved["version"].(float64) <= after["version"].(float64) {
			t.Fatal("successful edit did not advance the connection version")
		}
	}
}

func TestAccountConnectionSharedRoutingAndTenantUsage(t *testing.T) {
	type upstreamCall struct {
		authorization string
		model         string
	}
	var mutex sync.Mutex
	var calls []upstreamCall
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		mutex.Lock()
		calls = append(calls, upstreamCall{request.Header.Get("Authorization"), body.Model})
		mutex.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"shared-response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"tenant response"}]}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`))
	}))
	t.Cleanup(upstream.Close)
	router := newManagementRouter(t, proxy.Configuration{Endpoints: providerEndpoints(upstream.URL, proxy.ProviderNameOpenAI)})
	owner := managementSessionCookie(t, "shared-route-owner")
	first := requestManagementAccount(t, router, owner).Tenants[0].ID
	second := createManagementTenant(t, router, owner, "FamilyHome").Tenant.ID
	exchange := func(method, path string, body any, status int) map[string]any {
		t.Helper()
		return accountConnectionExchange(t, router, owner, method, path, body, status)
	}
	connection := exchange(http.MethodPost, "/connections", map[string]any{"name": "Production", "provider": "openai", "fields": map[string]string{"api_key": "shared-original-key"}}, http.StatusCreated)
	id := connection["id"].(string)
	for _, tenantID := range []string{first, second} {
		exchange(http.MethodPut, "/tenants/"+tenantID+"/connections/openai", map[string]string{"connection_id": id}, http.StatusOK)
	}
	saveManagementDefaults(t, router, owner, first, proxy.ModelNameGPT41, "First tenant prompt")
	saveManagementDefaults(t, router, owner, second, proxy.ModelNameGPT55, "Second tenant prompt")
	secrets := []string{generateManagementTenantSecret(t, router, owner, first), generateManagementTenantSecret(t, router, owner, second)}
	for _, credential := range []string{"shared-original-key", "shared-rotated-key"} {
		current := exchange(http.MethodGet, "/connections/"+id, nil, http.StatusOK)
		exchange(http.MethodPut, "/connections/"+id, map[string]any{"name": "Production", "provider": "openai", "version": current["version"], "fields": map[string]string{"api_key": credential}}, http.StatusOK)
		for _, secret := range secrets {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?"+url.Values{"key": {secret}, "prompt": {"hello"}}.Encode(), nil))
			if response.Code != http.StatusOK {
				t.Fatalf("shared route status=%d body=%s", response.Code, response.Body.String())
			}
		}
	}
	mutex.Lock()
	observedCalls := append([]upstreamCall(nil), calls...)
	mutex.Unlock()
	expected := []upstreamCall{{"Bearer shared-original-key", proxy.ModelNameGPT41}, {"Bearer shared-original-key", proxy.ModelNameGPT55}, {"Bearer shared-rotated-key", proxy.ModelNameGPT41}, {"Bearer shared-rotated-key", proxy.ModelNameGPT55}}
	if fmt.Sprint(observedCalls) != fmt.Sprint(expected) {
		t.Fatalf("shared credential routing=%v want=%v", observedCalls, expected)
	}
	for _, tenantID := range []string{first, second} {
		usage := waitForManagementValue(t, func() managementTenantUsageTestResponse {
			return requestManagementTenantUsage(t, router, owner, tenantID)
		}, func(value managementTenantUsageTestResponse) bool { return value.Totals.Requests == 2 })
		if usage.Totals.Requests != 2 {
			t.Fatalf("tenant %s requests=%d want=2", tenantID, usage.Totals.Requests)
		}
	}
	if usage := requestManagementAccountUsage(t, router, owner); usage.Totals.Requests != 4 {
		t.Fatalf("account requests=%d want=4", usage.Totals.Requests)
	}
}

func TestAccountConnectionStartupValidatesUnassignedConnections(t *testing.T) {
	for _, scenario := range []struct {
		name, table, column string
		value               any
	}{
		{"provider", "managed_account_connection_records", "provider_id", "unknown-provider"},
		{"name", "managed_account_connection_records", "name", ""},
		{"version", "managed_account_connection_records", "version", 0},
		{"field", "managed_connection_field_records", "field_id", "unknown-field"},
		{"ciphertext", "managed_connection_field_records", "value", "invalid"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "connections.db")
			router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
			owner := managementSessionCookie(t, "startup-connection-owner")
			connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Unassigned", "provider": "openai", "fields": map[string]string{"api_key": testManagementOpenAIKey}}, http.StatusCreated)
			database := openManagedFixtureDatabase(t, databasePath)
			identifier := "id"
			if scenario.table == "managed_connection_field_records" {
				identifier = "connection_id"
			}
			if err := database.Table(scenario.table).Where(identifier+" = ?", connection["id"]).Update(scenario.column, scenario.value).Error; err != nil {
				t.Fatal(err)
			}
			_, err := buildRouterWithCatalogs(t, managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath), zap.NewNop().Sugar())
			if err == nil || !strings.Contains(err.Error(), "connection="+connection["id"].(string)) {
				t.Fatalf("startup must reject invalid unassigned %s with connection context; error=%v", scenario.name, err)
			}
		})
	}
}

func TestAccountConnectionConcurrentVerifiedEdits(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseVerification := func() { releaseOnce.Do(func() { close(release) }) }
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") == "Bearer first-candidate" {
			close(started)
			select {
			case <-release:
			case <-request.Context().Done():
				return
			}
		}
		writeProviderKeyVerificationSuccess(writer, verificationTransportChat)
	}))
	t.Cleanup(func() { releaseVerification(); upstream.Close() })
	configuration := managementConfigurationWithDatabasePath(providerKeyVerificationConfiguration(upstream.URL), filepath.Join(t.TempDir(), "connections.db"))
	configuration.WorkerCount = 2
	router, buildError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatal(buildError)
	}
	owner := managementSessionCookie(t, "concurrent-connection-owner")
	created := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Production", "provider": "deepseek", "fields": map[string]string{"api_key": "initial-credential"}}, http.StatusCreated)
	path := "/connections/" + created["id"].(string)
	payload, err := json.Marshal(map[string]any{"name": "First edit", "provider": "deepseek", "version": created["version"], "fields": map[string]string{"api_key": "first-candidate"}})
	if err != nil {
		t.Fatal(err)
	}
	firstRequest := authenticatedJSONRequest(http.MethodPut, "/api/management"+path, string(payload), owner)
	complete := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, firstRequest)
		complete <- response
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first credential verification did not start")
	}
	second := accountConnectionExchange(t, router, owner, http.MethodPut, path, map[string]any{"name": "Second edit", "provider": "deepseek", "version": created["version"], "fields": map[string]string{"api_key": "second-candidate"}}, http.StatusOK)
	releaseVerification()
	select {
	case response := <-complete:
		if response.Code != http.StatusConflict {
			t.Fatalf("stale verified edit status=%d body=%s", response.Code, response.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("stale edit did not complete")
	}
	current := accountConnectionExchange(t, router, owner, http.MethodGet, path, nil, http.StatusOK)
	if current["version"] != second["version"] || current["name"] != "Second edit" || fmt.Sprint(current["fields"]) != fmt.Sprint(second["fields"]) {
		t.Fatal("stale verified edit replaced the later saved connection")
	}
}

func TestAccountConnectionLifecycle(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	owner := managementSessionCookie(t, "connections-owner")
	other := managementSessionCookie(t, "connections-other")
	account := requestManagementAccount(t, router, owner)
	tenantID := account.Tenants[0].ID
	request := func(method, path, body string, cookie *http.Cookie, status int) map[string]any {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, authenticatedJSONRequest(method, "/api/management"+path, body, cookie))
		if response.Code != status {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.Code, status, response.Body.String())
		}
		if strings.Contains(response.Body.String(), testManagementOpenAIKey) {
			t.Fatal("connection response exposes upstream credential")
		}
		var value map[string]any
		if response.Body.Len() > 0 && status < 300 {
			if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
				t.Fatal(err)
			}
		}
		return value
	}
	created := request(http.MethodPost, "/connections", `{"name":"Production","provider":"openai","fields":{"api_key":"`+testManagementOpenAIKey+`"}}`, owner, http.StatusCreated)
	id := created["id"].(string)
	if len(created["tenant_ids"].([]any)) != 0 {
		t.Fatal("new connection was implicitly assigned")
	}
	assignment := "/tenants/" + tenantID + "/connections/openai"
	request(http.MethodPut, assignment, `{"connection_id":"`+id+`"}`, owner, http.StatusOK)
	request(http.MethodPut, assignment, `{"connection_id":"`+id+`"}`, owner, http.StatusOK)
	profile := request(http.MethodGet, "/tenants/"+tenantID, "", owner, http.StatusOK)
	defaults := profile["tenant"].(map[string]any)["defaults"].(map[string]any)
	if defaults["provider"] != "" || defaults["model"] != "" {
		t.Fatalf("attachment selected defaults: %v", defaults)
	}
	request(http.MethodGet, "/connections/"+id, "", other, http.StatusNotFound)
	request(http.MethodPut, assignment, `{"connection_id":"`+id+`"}`, other, http.StatusNotFound)
	request(http.MethodDelete, "/connections/"+id, `{}`, owner, http.StatusConflict)
	second := request(http.MethodPost, "/connections", `{"name":"Other","provider":"openai","fields":{"api_key":"`+testManagementOpenAIKey+`"}}`, owner, http.StatusCreated)
	request(http.MethodPut, assignment, `{"connection_id":"`+second["id"].(string)+`"}`, owner, http.StatusConflict)
	tenant := request(http.MethodPost, "/tenants", `{"name":"Social Threader"}`, owner, http.StatusCreated)
	secondTenantID := tenant["tenant"].(map[string]any)["id"].(string)
	request(http.MethodPut, "/tenants/"+secondTenantID+"/connections/openai", `{"connection_id":"`+id+`"}`, owner, http.StatusOK)
	shared := request(http.MethodGet, "/connections/"+id, "", owner, http.StatusOK)
	if len(shared["tenant_ids"].([]any)) != 2 {
		t.Fatalf("assignments=%v", shared)
	}
	request(http.MethodDelete, assignment, `{}`, owner, http.StatusNoContent)
	shared = request(http.MethodGet, "/connections/"+id, "", owner, http.StatusOK)
	if len(shared["tenant_ids"].([]any)) != 1 {
		t.Fatal("detach changed another tenant assignment")
	}
	request(http.MethodDelete, "/tenants/"+secondTenantID+"/connections/openai", `{}`, owner, http.StatusNoContent)
	request(http.MethodDelete, "/connections/"+id, `{}`, owner, http.StatusNoContent)
	request(http.MethodGet, "/connections/"+id, "", owner, http.StatusNotFound)
}
