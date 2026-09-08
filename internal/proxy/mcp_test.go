package proxy_test

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/tauth/pkg/oauthvalidator"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

func TestMCPRequiresOAuth(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	server := httptest.NewServer(router)
	defer server.Close()
	response, err := http.Post(server.URL+"/mcp", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("MCP unauthenticated status=%d, want 401", response.StatusCode)
	}
	if !strings.Contains(response.Header.Get("WWW-Authenticate"), "/.well-known/oauth-protected-resource/mcp") {
		t.Fatal("missing OAuth protected-resource challenge")
	}
}

type mcpFixture struct {
	server       *httptest.Server
	issuer       *httptest.Server
	key          *ecdsa.PrivateKey
	router       http.Handler
	databasePath string
	logs         *observer.ObservedLogs
}

func newMCPFixture(t *testing.T, upstream http.Handler) mcpFixture {
	return newMCPFixtureWithConfig(t, upstream, proxy.Configuration{})
}

func newMCPFixtureWithConfig(t *testing.T, upstream http.Handler, configuration proxy.Configuration) mcpFixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public, err := key.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/jwks" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(oauthvalidator.JWKSet{Keys: []oauthvalidator.JWK{{KeyType: "EC", Use: "sig", KeyID: "fixture", Algorithm: "ES256", Curve: "P-256", X: base64.RawURLEncoding.EncodeToString(public[1:33]), Y: base64.RawURLEncoding.EncodeToString(public[33:])}}})
	}))
	t.Cleanup(issuer.Close)
	server := httptest.NewUnstartedServer(nil)
	selectedTimeout := configuration.RequestTimeoutSeconds
	configuration = managementConfigurationWithDatabasePath(configuration, filepath.Join(t.TempDir(), "tenants.db"))
	if selectedTimeout != 0 {
		configuration.RequestTimeoutSeconds = selectedTimeout
	}
	configuration.Management.TAuthURL = issuer.URL
	configuration.Management.ProxyOrigin = "http://" + server.Listener.Addr().String()
	if upstream != nil {
		provider := httptest.NewServer(upstream)
		t.Cleanup(provider.Close)
		configuration.Endpoints = providerEndpoints(provider.URL, proxy.ProviderNameOpenAI, proxy.ProviderNameDeepSeek, proxy.ProviderNameGemini)
	}
	previous := proxy.HTTPClient
	proxy.HTTPClient = managementProviderKeyVerificationHTTPDoer{next: previous}
	logCore, logs := observer.New(zap.DebugLevel)
	router, err := buildRouterWithCatalogs(t, configuration, zap.New(logCore).Sugar())
	proxy.HTTPClient = previous
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = router
	server.Start()
	t.Cleanup(server.Close)
	return mcpFixture{server: server, issuer: issuer, key: key, router: router, databasePath: configuration.Management.DatabasePath, logs: logs}
}

func (fixture mcpFixture) token(t *testing.T, subject string, change func(*oauthvalidator.Claims)) string {
	t.Helper()
	claims := oauthvalidator.Claims{ClientID: "fixture-client", Scope: "llm-proxy:use", TenantID: testManagementTenantID, GrantID: "fixture-grant", RegisteredClaims: jwt.RegisteredClaims{Issuer: fixture.issuer.URL, Subject: subject, Audience: jwt.ClaimStrings{fixture.server.URL}, IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Second)), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
	if change != nil {
		change(&claims)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"], token.Header["typ"] = "fixture", "at+jwt"
	signed, err := token.SignedString(fixture.key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

type mcpBearerTransport struct{ token string }

func (fixture mcpFixture) waitUsageEvents(t *testing.T, count int64) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(fixture.databasePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	waitForPersistedManagedUsageEventCount(t, database, count)
}

func (transport mcpBearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(request)
}

func (fixture mcpFixture) client(t *testing.T, subject string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "F021 acceptance", Version: "1.0.0"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: fixture.server.URL + "/mcp", HTTPClient: &http.Client{Transport: mcpBearerTransport{token: fixture.token(t, subject, nil)}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func mcpCall(t *testing.T, client *mcp.ClientSession, name string, arguments any) *mcp.CallToolResult {
	t.Helper()
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mcpTenants(t *testing.T, client *mcp.ClientSession) managementAccountTestResponse {
	t.Helper()
	result := mcpCall(t, client, "llm_proxy.list_tenants", struct{}{})
	if result.IsError {
		t.Fatalf("tenant discovery failed: %+v", result)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var account managementAccountTestResponse
	if err := json.Unmarshal(data, &account); err != nil {
		t.Fatal(err)
	}
	return account
}

func TestMCPTenantDiscoveryAndGeneration(t *testing.T) {
	fixture := newMCPFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answer := "first-response"
		if r.Header.Get("Authorization") == "Bearer second-fixture-key" {
			answer = "second-response"
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"response-fixture","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"`+answer+`"}]}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`)
	}))
	owner := managementSessionCookie(t, "mcp-owner")
	first := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
	second := createManagementTenant(t, fixture.router, owner, "Second").Tenant.ID
	foreign := requestManagementAccount(t, fixture.router, managementSessionCookie(t, "foreign-owner")).Tenants[0].ID
	client := fixture.client(t, "mcp-owner")
	if got, want := mcpTenants(t, client).Tenants, requestManagementAccount(t, fixture.router, owner).Tenants; !reflect.DeepEqual(got, want) {
		t.Fatalf("tenants=%+v want=%+v", got, want)
	}
	if tenants := mcpTenants(t, fixture.client(t, "unprovisioned")).Tenants; len(tenants) != 0 {
		t.Fatal("discovery created a tenant")
	}
	tools, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 2 {
		t.Fatalf("tools=%+v", tools.Tools)
	}
	for _, tool := range tools.Tools {
		if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
			t.Fatal("incorrect annotations")
		}
	}
	for _, tenant := range []struct{ id, key, answer string }{{first, "first-fixture-key", "first-response"}, {second, "second-fixture-key", "second-response"}} {
		saveManagementProviderKey(t, fixture.router, owner, tenant.id, tenant.key, proxy.ModelNameGPT41, "")
		result := mcpCall(t, client, "llm_proxy.generate_text", map[string]any{"tenant_id": tenant.id, "messages": []map[string]string{{"role": "user", "content": "hello"}}, "request_timeout_seconds": 10})
		if result.IsError {
			t.Fatalf("generation=%+v", result)
		}
		data, _ := json.Marshal(result.StructuredContent)
		var output struct {
			Text, Provider, Model string
			RequestID             string `json:"request_id"`
			Timeout               int    `json:"request_timeout_seconds"`
			Usage                 struct {
				Total int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(data, &output); err != nil {
			t.Fatal(err)
		}
		if output.Text != tenant.answer || output.Provider != "openai" || output.Model != proxy.ModelNameGPT41 || output.RequestID == "" || output.Timeout != 10 || output.Usage.Total != 5 {
			t.Fatalf("output=%s", data)
		}
		resource, err := client.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "llm-proxy://tenants/" + tenant.id + "/routes"})
		if err != nil {
			t.Fatal(err)
		}
		if len(resource.Contents) != 1 || !strings.Contains(resource.Contents[0].Text, `"provider":"openai"`) {
			t.Fatalf("routes=%+v", resource)
		}
		usage := waitForManagementValue(t, func() managementTenantUsageTestResponse {
			return requestManagementTenantUsage(t, fixture.router, owner, tenant.id)
		}, func(v managementTenantUsageTestResponse) bool { return v.Totals.Requests == 1 })
		if usage.Totals.Requests != 1 {
			t.Fatal("generation usage missing")
		}
	}
	reopened := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, fixture.databasePath)
	if requestManagementTenantUsage(t, reopened, owner, first).Totals.Requests != 1 || requestManagementTenantUsage(t, reopened, owner, second).Totals.Requests != 1 {
		t.Fatal("MCP usage lost after reopen")
	}

	for _, id := range []string{foreign, "missing", ""} {
		result := mcpCall(t, client, "llm_proxy.generate_text", map[string]any{"tenant_id": id, "messages": []map[string]string{{"role": "user", "content": "hello"}}})
		if !result.IsError || result.Content[0].(*mcp.TextContent).Text != "not_found" {
			t.Fatalf("foreign access=%+v", result)
		}
		if _, err := fixture.client(t, "mcp-owner").ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "llm-proxy://tenants/" + id + "/routes"}); err == nil {
			t.Fatal("foreign routes accepted")
		}
	}
	for _, input := range []any{map[string]any{}, map[string]any{"tenant_id": first, "messages": []any{}, "key": "forbidden"}} {
		result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "llm_proxy.generate_text", Arguments: input})
		if err == nil && !result.IsError {
			t.Fatal("invalid schema accepted")
		}
	}
}

func TestMCPOAuthBoundary(t *testing.T) {
	fixture := newMCPFixture(t, nil)
	for _, test := range []struct {
		name   string
		change func(*oauthvalidator.Claims)
		status int
	}{
		{"expired", func(c *oauthvalidator.Claims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute)) }, 401},
		{"issuer", func(c *oauthvalidator.Claims) { c.Issuer = "https://wrong.example" }, 401},
		{"audience", func(c *oauthvalidator.Claims) { c.Audience = jwt.ClaimStrings{"https://wrong.example"} }, 401},
		{"scope", func(c *oauthvalidator.Claims) { c.Scope = "wrong:scope" }, 403},
		{"tenant", func(c *oauthvalidator.Claims) { c.TenantID = "other-app" }, 401},
		{"subject", func(c *oauthvalidator.Claims) { c.Subject = "" }, 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, _ := http.NewRequestWithContext(t.Context(), "POST", fixture.server.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`))
			request.Header.Set("Authorization", "Bearer "+fixture.token(t, "owner", test.change))
			response, err := fixture.server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.status {
				t.Fatalf("status=%d want=%d", response.StatusCode, test.status)
			}
		})
	}
}

func TestMCPDiscoveryTracksManagementChanges(t *testing.T) {
	fixture := newMCPFixture(t, nil)
	owner := managementSessionCookie(t, "changing-owner")
	client := fixture.client(t, "changing-owner")
	if len(mcpTenants(t, client).Tenants) != 0 {
		t.Fatal("unexpected initial tenant")
	}
	first := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
	second := createManagementTenant(t, fixture.router, owner, "Second").Tenant.ID
	if len(mcpTenants(t, client).Tenants) != 2 {
		t.Fatal("creation absent")
	}
	for _, operation := range []struct {
		method, body string
		status       int
	}{{"PUT", `{"name":"Renamed"}`, 200}, {"DELETE", `{}`, 204}} {
		request := authenticatedJSONRequest(operation.method, "/api/management/tenants/"+second, operation.body, owner)
		response := httptest.NewRecorder()
		fixture.router.ServeHTTP(response, request)
		if response.Code != operation.status {
			t.Fatalf("management status=%d body=%s", response.Code, response.Body)
		}
		if got, want := mcpTenants(t, client).Tenants, requestManagementAccount(t, fixture.router, owner).Tenants; !reflect.DeepEqual(got, want) {
			t.Fatalf("discovery=%+v want=%+v", got, want)
		}
	}
	result := mcpCall(t, client, "llm_proxy.generate_text", map[string]any{"tenant_id": second, "messages": []any{}})
	if !result.IsError || result.Content[0].(*mcp.TextContent).Text != "not_found" {
		t.Fatal("deleted tenant authorized")
	}
	resource, err := client.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "llm-proxy://tenants/" + first + "/routes"})
	if err != nil || !strings.Contains(resource.Contents[0].Text, `"routes":[]`) {
		t.Fatalf("unconfigured routes=%+v err=%v", resource, err)
	}
}

func TestMCPHTTPContract(t *testing.T) {
	fixture := newMCPFixture(t, nil)
	response, err := fixture.server.Client().Get(fixture.server.URL + "/.well-known/oauth-protected-resource/mcp")
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Resource             string
		AuthorizationServers []string `json:"authorization_servers"`
	}
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || metadata.Resource != fixture.server.URL || !reflect.DeepEqual(metadata.AuthorizationServers, []string{fixture.issuer.URL}) || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("incorrect protected resource metadata")
	}
	for _, test := range []struct {
		name, path, method, body string
		headers                  map[string]string
		status                   int
	}{
		{"discovery", "/mcp", "POST", `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"http-test","version":"1.0.0"}}}}`, nil, 200},
		{"old version header", "/mcp", "POST", `{}`, map[string]string{"MCP-Protocol-Version": "2025-11-25"}, 400},
		{"session", "/mcp", "POST", `{}`, map[string]string{"Mcp-Session-Id": "obsolete"}, 400},
		{"query secret", "/mcp?key=fixture", "POST", `{}`, nil, 400},
		{"foreign origin", "/mcp", "POST", `{}`, map[string]string{"Origin": "https://foreign.example"}, 403},
		{"malformed bearer", "/mcp", "POST", `{}`, map[string]string{"Authorization": "Bearer invalid"}, 401},
		{"old transport", "/mcp", "GET", ``, nil, 405},
		{"tenant path", "/mcp/tenant", "POST", `{}`, nil, 404},
		{"missing version", "/mcp", "POST", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, nil, 400},
		{"old version", "/mcp", "POST", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2025-11-25"}}}`, nil, 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, _ := http.NewRequestWithContext(t.Context(), test.method, fixture.server.URL+test.path, strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer "+fixture.token(t, "owner", nil))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("MCP-Protocol-Version", "2026-07-28")
			var rpc struct {
				Method string `json:"method"`
			}
			_ = json.Unmarshal([]byte(test.body), &rpc)
			request.Header.Set("Mcp-Method", rpc.Method)
			request.Header.Set("Accept", "application/json, text/event-stream")
			for key, value := range test.headers {
				request.Header.Set(key, value)
			}
			response, err := fixture.server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.StatusCode, test.status, body)
			}
			if test.name == "discovery" && (!strings.Contains(string(body), `"supportedVersions":["2026-07-28"]`) || response.Header.Get("Mcp-Session-Id") != "") {
				t.Fatalf("unexpected discovery: %s", body)
			}
		})
	}
}

func TestMCPGenerationFailuresAndUsage(t *testing.T) {
	fixture := newMCPFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "private provider detail", http.StatusTooManyRequests)
	}))
	owner := managementSessionCookie(t, "failure-owner")
	tenant := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
	client := fixture.client(t, "failure-owner")
	input := map[string]any{"tenant_id": tenant, "provider": "openai", "messages": []map[string]string{{"role": "user", "content": "private prompt"}}}
	result := mcpCall(t, client, "llm_proxy.generate_text", input)
	if !result.IsError {
		t.Fatal("unconfigured route succeeded")
	}
	saveManagementProviderKey(t, fixture.router, owner, tenant, "private-fixture-key", proxy.ModelNameGPT41, "")
	for _, change := range []map[string]any{{"request_timeout_seconds": 0}, {"max_tokens": 0}, {"messages": []any{}}, {"provider": "unknown"}, {"reasoning_effort": "invalid"}, {}} {
		arguments := map[string]any{}
		for k, v := range input {
			arguments[k] = v
		}
		for k, v := range change {
			arguments[k] = v
		}
		result := mcpCall(t, client, "llm_proxy.generate_text", arguments)
		data, _ := json.Marshal(result)
		if !result.IsError || strings.Contains(string(data), "private") {
			t.Fatalf("unsafe failure=%s", data)
		}
	}
	failures := waitForManagementValue(t, func() managementUsageFailuresTestResponse {
		return requestManagementUsageFailures(t, fixture.router, owner, "/api/management/tenants/"+tenant, "30d", 25, "")
	}, func(v managementUsageFailuresTestResponse) bool { return len(v.Failures) == 1 })
	if failures.Failures[0].Endpoint != "mcp" || failures.Failures[0].StatusCode != 504 {
		t.Fatalf("failures=%+v", failures)
	}
	rejections := waitForManagementValue(t, func() managementUsageRejectionsTestResponse {
		return requestManagementUsageRejections(t, fixture.router, owner, "/api/management/tenants/"+tenant, "30d", 25, "")
	}, func(v managementUsageRejectionsTestResponse) bool { return len(v.Rejections) == 6 })
	for _, failure := range rejections.Rejections {
		if failure.Endpoint != "mcp" {
			t.Fatal("incorrect rejection endpoint")
		}
	}

}

func TestMCPCancellationAndTimeout(t *testing.T) {
	for _, cancelCaller := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "cancellation"}[cancelCaller], func(t *testing.T) {
			started, stopped := make(chan struct{}), make(chan struct{})
			fixture := newMCPFixtureWithConfig(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				close(started)
				<-r.Context().Done()
				close(stopped)
			}), proxy.Configuration{RequestTimeoutSeconds: 1, MaxRequestTimeoutSeconds: 2})
			owner := managementSessionCookie(t, "timeout-owner")
			tenant := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
			saveManagementProviderKey(t, fixture.router, owner, tenant, "timeout-key", proxy.ModelNameGPT41, "")
			client := fixture.client(t, "timeout-owner")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "llm_proxy.generate_text", Arguments: map[string]any{"tenant_id": tenant, "messages": []map[string]string{{"role": "user", "content": "hi"}}}})
				if !cancelCaller && (err != nil || !result.IsError || result.Content[0].(*mcp.TextContent).Text != "request_timeout") {
					t.Errorf("timeout result=%+v err=%v", result, err)
				}
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("upstream did not start")
			}
			if cancelCaller {
				cancel()
			}
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("upstream was not cancelled")
			}
			<-done
			usage := waitForManagementValue(t, func() managementTenantUsageTestResponse {
				return requestManagementTenantUsage(t, fixture.router, owner, tenant)
			}, func(v managementTenantUsageTestResponse) bool { return v.Totals.Requests == 1 })
			if usage.Totals.Requests != 1 {
				t.Fatal("timeout usage missing")
			}
		})
	}
}

func TestMCPCapacityAndConcurrentTenants(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 3)
	fixture := newMCPFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		<-release
		answer := "first"
		if r.Header.Get("Authorization") == "Bearer second-key" {
			answer = "second"
		}
		io.WriteString(w, `{"status":"completed","output_text":"`+answer+`"}`)
	}))
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	owner := managementSessionCookie(t, "concurrent-owner")
	first := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
	second := createManagementTenant(t, fixture.router, owner, "Second").Tenant.ID
	saveManagementProviderKey(t, fixture.router, owner, first, "first-key", proxy.ModelNameGPT41, "")
	saveManagementProviderKey(t, fixture.router, owner, second, "second-key", proxy.ModelNameGPT41, "")
	client := fixture.client(t, "concurrent-owner")
	type answer struct {
		tenant string
		result *mcp.CallToolResult
		err    error
	}
	results := make(chan answer, 3)
	call := func(tenant string) {
		result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "llm_proxy.generate_text", Arguments: map[string]any{"tenant_id": tenant, "messages": []map[string]string{{"role": "user", "content": "hello"}}}})
		results <- answer{tenant, result, err}
	}
	go call(first)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first call did not start")
	}
	go call(second)
	go call(second)
	select {
	case result := <-results:
		if result.err != nil || !result.result.IsError || result.result.Content[0].(*mcp.TextContent).Text != "service_unavailable" {
			t.Fatalf("capacity=%+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queue did not reject excess work")
	}
	close(release)
	for range 2 {
		result := <-results
		if result.err != nil || result.result.IsError {
			t.Fatalf("accepted generation=%+v", result)
		}
		expected := "first"
		if result.tenant == second {
			expected = "second"
		}
		if result.result.Content[0].(*mcp.TextContent).Text != expected {
			t.Fatal("cross-tenant credential selection")
		}
	}
	fixture.waitUsageEvents(t, 3)
}

func TestMCPMediaAndSchemaPrivacy(t *testing.T) {
	var received string
	fixture := newMCPFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = string(body)
		io.WriteString(w, `{"status":"completed","output_text":"private generated answer"}`)
	}))
	owner := managementSessionCookie(t, "media-owner")
	tenant := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
	saveManagementProviderKey(t, fixture.router, owner, tenant, "private-media-key", proxy.ModelNameGPT41, "")
	client := fixture.client(t, "media-owner")
	image := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII="
	input := map[string]any{"tenant_id": tenant, "messages": []any{map[string]any{"role": "user", "content": "private media prompt", "order": 0, "attachments": []map[string]string{{"type": "image", "mime_type": "image/png", "data": image}}}}, "provider": "openai", "model": proxy.ModelNameGPT41, "max_tokens": 100}
	result := mcpCall(t, client, "llm_proxy.generate_text", input)
	if result.IsError || !strings.Contains(received, image) {
		t.Fatalf("media result=%+v received media=%t", result, strings.Contains(received, image))
	}
	for _, name := range []string{"reasoning_effort", "max_tokens", "request_timeout_seconds"} {
		input[name] = nil
		result := mcpCall(t, client, "llm_proxy.generate_text", input)
		if !result.IsError || result.Content[0].(*mcp.TextContent).Text != "invalid_request" {
			t.Fatalf("null control accepted: %s", name)
		}
		delete(input, name)
	}
	input["messages"] = []any{map[string]any{"role": "user", "content": "private media prompt", "attachments": []map[string]string{{"type": "image", "mime_type": "image/png", "data": "invalid"}}}}
	if !mcpCall(t, client, "llm_proxy.generate_text", input).IsError {
		t.Fatal("invalid media accepted")
	}
	logs, _ := json.Marshal(fixture.logs.AllUntimed())
	for _, secret := range []string{"private media prompt", "private generated answer", "private-media-key", image} {
		if strings.Contains(string(logs), secret) {
			t.Fatal("request data in logs")
		}
	}
	fixture.waitUsageEvents(t, 2)
}

func TestMCPDatabaseErrorsRemainSanitized(t *testing.T) {
	fixture := newMCPFixture(t, nil)
	owner := managementSessionCookie(t, "db-owner")
	tenant := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
	client := fixture.client(t, "db-owner")
	database, err := gorm.Open(sqlite.Open(fixture.databasePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := database.Exec("DROP TABLE managed_user_records").Error; err != nil {
		t.Fatal(err)
	}
	result := mcpCall(t, client, "llm_proxy.list_tenants", struct{}{})
	encoded, _ := json.Marshal(result.StructuredContent)
	if !result.IsError || string(encoded) != `{"code":"proxy_error"}` {
		t.Fatalf("database discovery error=%s", encoded)
	}
	if err := database.Exec("DROP TABLE managed_tenant_records").Error; err != nil {
		t.Fatal(err)
	}
	result = mcpCall(t, client, "llm_proxy.generate_text", map[string]any{"tenant_id": tenant, "messages": []any{}})
	if !result.IsError || result.Content[0].(*mcp.TextContent).Text != "proxy_error" {
		t.Fatal("database failure exposed")
	}
	_, err = client.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "llm-proxy://tenants/" + tenant + "/routes"})
	if err == nil || strings.Contains(err.Error(), "SQL") {
		t.Fatalf("resource error=%v", err)
	}
}

func TestMCPProviderRateLimit(t *testing.T) {
	fixture := newMCPFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		http.Error(w, `{"error":{"message":"private upstream detail"}}`, http.StatusTooManyRequests)
	}))
	owner := managementSessionCookie(t, "rate-owner")
	tenant := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, authenticatedJSONRequest("PUT", "/api/management/tenants/"+tenant+"/provider-connections/deepseek", `{"fields":{"api_key":"rate-fixture-key"},"text_model":"deepseek-v4-flash","system_prompt":""}`, owner))
	if response.Code != 200 {
		t.Fatalf("configure rate provider=%d %s", response.Code, response.Body)
	}
	result := mcpCall(t, fixture.client(t, "rate-owner"), "llm_proxy.generate_text", map[string]any{"tenant_id": tenant, "provider": "deepseek", "model": "deepseek-v4-flash", "messages": []map[string]string{{"role": "user", "content": "rate fixture"}}})
	if !result.IsError || result.Content[0].(*mcp.TextContent).Text != "rate_limited" {
		t.Fatalf("rate result=%+v", result)
	}
	failures := waitForManagementValue(t, func() managementUsageFailuresTestResponse {
		return requestManagementUsageFailures(t, fixture.router, owner, "/api/management/tenants/"+tenant, "30d", 25, "")
	}, func(v managementUsageFailuresTestResponse) bool { return len(v.Failures) == 1 })
	if failures.Failures[0].StatusCode != 429 || failures.Failures[0].OutcomeCode != "rate_limited" || failures.Failures[0].Endpoint != "mcp" {
		t.Fatalf("rate usage=%+v", failures)
	}
}

func TestMCPOrderedImageAndAudio(t *testing.T) {
	var captured map[string]any
	fixture := newMCPFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = decodeGeminiInteractionRequest(t, r)
		writeGeminiInteractionSnapshot(t, w, "", "completed", "media accepted", nil)
	}))
	owner := managementSessionCookie(t, "audio-owner")
	tenant := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, authenticatedJSONRequest("PUT", "/api/management/tenants/"+tenant+"/provider-connections/gemini", `{"fields":{"api_key":"audio-fixture-key"},"text_model":"gemini-3.5-flash","system_prompt":""}`, owner))
	if response.Code != 200 {
		t.Fatalf("configure audio provider=%d %s", response.Code, response.Body)
	}
	image, audio := []byte("first-image"), []byte("voice-track")
	result := mcpCall(t, fixture.client(t, "audio-owner"), "llm_proxy.generate_text", map[string]any{"tenant_id": tenant, "provider": "gemini", "model": proxy.ModelNameGemini35Flash, "messages": []any{map[string]any{"role": "user", "content": "Inspect in order.", "attachments": []map[string]any{messageMediaPayload("image", "image/png", image), messageMediaPayload("audio", "audio/m4a", audio)}}}})
	if result.IsError {
		t.Fatalf("audio generation=%+v", result)
	}
	input := captured["input"].([]any)
	content := input[0].(map[string]any)["content"].([]any)
	if len(content) != 3 {
		t.Fatalf("media count=%d", len(content))
	}
	assertGeminiInteractionMediaContent(t, content[1], "image", "image/png", image)
	assertGeminiInteractionMediaContent(t, content[2], "audio", "audio/m4a", audio)
	saveManagementProviderKey(t, fixture.router, owner, tenant, "openai-fixture-key", proxy.ModelNameGPT41, "")
	routes, err := fixture.client(t, "audio-owner").ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "llm-proxy://tenants/" + tenant + "/routes"})
	if err != nil || !strings.Contains(routes.Contents[0].Text, `"provider":"gemini"`) || !strings.Contains(routes.Contents[0].Text, `"provider":"openai"`) {
		t.Fatalf("configured routes=%+v err=%v", routes, err)
	}
	fixture.waitUsageEvents(t, 1)
}

func TestMCPAssetStoreFailure(t *testing.T) {
	root := t.TempDir()
	fixture := newMCPFixtureWithConfig(t, nil, proxy.Configuration{AssetStorePath: root})
	owner := managementSessionCookie(t, "asset-owner")
	tenant := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
	saveManagementProviderKey(t, fixture.router, owner, tenant, "asset-fixture-key", proxy.ModelNameGPT41, "")
	secret := generateManagementTenantSecret(t, fixture.router, owner, tenant)
	asset := uploadTestAsset(t, fixture.router, secret, "image/png", []byte("image"))
	corrupted, err := os.Create(filepath.Join(root, asset.AssetID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := corrupted.WriteString("invalid metadata"); err != nil {
		t.Fatal(err)
	}
	if err := corrupted.Close(); err != nil {
		t.Fatal(err)
	}
	result := mcpCall(t, fixture.client(t, "asset-owner"), "llm_proxy.generate_text", map[string]any{"tenant_id": tenant, "messages": []any{map[string]any{"role": "user", "content": "inspect", "attachments": []map[string]string{{"type": "image", "mime_type": "image/png", "asset_id": asset.AssetID}}}}})
	if !result.IsError || result.Content[0].(*mcp.TextContent).Text != "proxy_error" {
		t.Fatalf("asset failure=%+v", result)
	}
	failures := waitForManagementValue(t, func() managementUsageFailuresTestResponse {
		return requestManagementUsageFailures(t, fixture.router, owner, "/api/management/tenants/"+tenant, "30d", 25, "")
	}, func(v managementUsageFailuresTestResponse) bool { return len(v.Failures) == 1 })
	if failures.Failures[0].StatusCode != 500 || failures.Failures[0].Endpoint != "mcp" {
		t.Fatalf("asset usage=%+v", failures)
	}
}

func TestMCPBodyBound(t *testing.T) {
	fixture := newMCPFixture(t, nil)
	request, _ := http.NewRequestWithContext(t.Context(), "POST", fixture.server.URL+"/mcp", strings.NewReader(strings.Repeat(" ", proxy.MaxV2RequestBytes+1)))
	request.Header.Set("Authorization", "Bearer "+fixture.token(t, "body-owner", nil))
	request.Header.Set("MCP-Protocol-Version", "2026-07-28")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response, err := fixture.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 413 {
		t.Fatalf("body bound status=%d", response.StatusCode)
	}
}

func TestMCPStalledUploadDeadline(t *testing.T) {
	fixture := newMCPFixtureWithConfig(t, nil, proxy.Configuration{RequestTimeoutSeconds: 1})
	connection, err := net.Dial("tcp", fixture.server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(connection, "POST /mcp HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nMCP-Protocol-Version: 2026-07-28\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\nContent-Length: 4096\r\nConnection: close\r\n\r\n{", fixture.server.Listener.Addr().String(), fixture.token(t, "stalled-owner", nil))
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatalf("stalled MCP upload did not terminate within the server budget: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusRequestTimeout {
		t.Fatalf("stalled upload status=%d want=408", response.StatusCode)
	}
	fixture.waitUsageEvents(t, 0)
}

type mcpDeadlineWriter struct {
	http.ResponseWriter
	setDeadline func(time.Time) error
}

func TestMCPStalledUploadCancellation(t *testing.T) {
	fixture := newMCPFixtureWithConfig(t, nil, proxy.Configuration{RequestTimeoutSeconds: 30})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reading, finished := make(chan struct{}, 1), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = &observedClientBody{ReadCloser: r.Body, reading: reading}
		fixture.router.ServeHTTP(w, r.WithContext(ctx))
		close(finished)
	}))
	defer server.Close()
	connection, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(connection, "POST /mcp HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nMCP-Protocol-Version: 2026-07-28\r\nContent-Type: application/json\r\nContent-Length: 4096\r\n\r\n{", server.Listener.Addr().String(), fixture.token(t, "canceled-owner", nil))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-reading:
	case <-time.After(3 * time.Second):
		t.Fatal("upload read did not start")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not release the HTTP handler")
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusRequestTimeout {
		t.Fatalf("canceled upload status=%d want=408", response.StatusCode)
	}
	fixture.waitUsageEvents(t, 0)
}

func (writer mcpDeadlineWriter) SetReadDeadline(deadline time.Time) error {
	return writer.setDeadline(deadline)
}

func TestMCPUploadBoundaryFailures(t *testing.T) {
	fixture := newMCPFixture(t, nil)
	for _, scenario := range []struct {
		name         string
		failDeadline int
		cancel       bool
		status       int
	}{
		{name: "initial deadline unavailable", failDeadline: 1, status: 500},
		{name: "deadline cleanup fails", failDeadline: 2, status: 500},
		{name: "canceled upload", cancel: true, status: 408},
		{name: "cancellation deadline fails", failDeadline: 2, cancel: true, status: 500},
		{name: "incomplete body", status: 400},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithCancel(r.Context())
				defer cancel()
				calls := 0
				cancellationApplied := make(chan struct{})
				writer := mcpDeadlineWriter{ResponseWriter: w, setDeadline: func(deadline time.Time) error {
					calls++
					if scenario.cancel && calls == 2 {
						defer close(cancellationApplied)
					}
					if calls == scenario.failDeadline {
						return errors.New("injected deadline failure")
					}
					return http.NewResponseController(w).SetReadDeadline(deadline)
				}}
				if scenario.cancel {
					r.Body = io.NopCloser(mcpUploadReader(func([]byte) (int, error) {
						cancel()
						<-cancellationApplied
						return 0, io.ErrUnexpectedEOF
					}))
				} else if scenario.failDeadline == 0 {
					r.Body = io.NopCloser(mcpUploadReader(func([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }))
				}
				fixture.router.ServeHTTP(writer, r.WithContext(ctx))
			}))
			defer server.Close()
			request, _ := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/mcp", strings.NewReader("{}"))
			request.Header.Set("Authorization", "Bearer "+fixture.token(t, "boundary-owner", nil))
			request.Header.Set("MCP-Protocol-Version", "2026-07-28")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != scenario.status {
				t.Fatalf("upload status=%d want=%d", response.StatusCode, scenario.status)
			}
		})
	}
	fixture.waitUsageEvents(t, 0)
}

type mcpUploadReader func([]byte) (int, error)

func (read mcpUploadReader) Read(data []byte) (int, error) { return read(data) }

func TestMCPUploadDeadlineClearedBeforeGeneration(t *testing.T) {
	fixture := newMCPFixtureWithConfig(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		select {
		case <-time.After(1200 * time.Millisecond):
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"response-fixture","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"generation finished"}]}]}`)
		case <-r.Context().Done():
			t.Error("upload deadline canceled generation")
		}
	}), proxy.Configuration{RequestTimeoutSeconds: 1, MaxRequestTimeoutSeconds: 3})
	owner := managementSessionCookie(t, "upload-owner")
	tenant := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
	saveManagementProviderKey(t, fixture.router, owner, tenant, "upload-key", proxy.ModelNameGPT41, "")
	client := fixture.client(t, "upload-owner")
	result := mcpCall(t, client, "llm_proxy.generate_text", map[string]any{"tenant_id": tenant, "messages": []map[string]string{{"role": "user", "content": "hi"}}, "request_timeout_seconds": 3})
	if result.IsError {
		t.Fatalf("generation failed after upload deadline: %+v", result)
	}
	mcpTenants(t, client)
	fixture.waitUsageEvents(t, 1)
}
