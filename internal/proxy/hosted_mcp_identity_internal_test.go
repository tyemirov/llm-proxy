package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"github.com/tyemirov/tauth/pkg/oauthvalidator"
)

func TestHostedMCPIdentitySharesNativeExecution(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("Authorization") != "Bearer sk-platform-pinned" {
			t.Error("MCP did not use the pinned platform credential")
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-mcp-result","status":"completed","output_text":"shared result","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	var responses *structuredRequestStore
	server, clientFor := newHostedMCPIdentityServer(t, database, upstream.URL, t.TempDir(), func(dependencies *hostedTextRequestDependencies) { responses = dependencies.responses })
	owner := clientFor("owner")
	input := map[string]any{"tenant_id": "managed-first", "idempotency_key": "shared-mcp", "provider": "openai", "model": "gpt-4.1", "messages": []map[string]string{{"role": "user", "content": "shared prompt"}}}
	first := hostedMCPCall(t, owner, input, false)
	if first["text"] != "shared result" || first["request_id"] == "" {
		t.Fatalf("MCP result: %v", first)
	}
	replay := hostedMCPCall(t, owner, input, false)
	if replay["request_id"] != first["request_id"] || replay["text"] != first["text"] {
		t.Fatalf("MCP replay changed the execution receipt: first=%v replay=%v", first, replay)
	}
	hostedIdentityHTTP(t, server, "shared-mcp", "shared prompt", http.StatusOK)
	input["messages"] = []map[string]string{{"role": "user", "content": "changed prompt"}}
	if result := hostedMCPCall(t, owner, input, true); result["code"] != errUsageJournalConflict.Error() {
		t.Fatalf("changed MCP intent: %v", result)
	}
	if result := hostedMCPCall(t, clientFor("foreign-owner"), input, true); result["code"] != mcpNotFound {
		t.Fatalf("foreign MCP account disclosed the request: %v", result)
	}
	input["messages"] = []map[string]string{{"role": "user", "content": "shared prompt"}}
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("state", hostedGrantRevoked).Error; err != nil {
		t.Fatal(err)
	}
	if saved := hostedMCPCall(t, owner, input, false); saved["request_id"] != first["request_id"] {
		t.Fatalf("grant revocation removed the receipt: %v", saved)
	}
	input["idempotency_key"] = "new-after-revocation"
	if denied := hostedMCPCall(t, owner, input, true); denied["code"] != errHostedAuthorityDenied.Error() {
		t.Fatalf("revoked grant allowed new work: %v", denied)
	}
	input["idempotency_key"] = "shared-mcp"
	responses.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if expired := hostedMCPCall(t, owner, input, true); expired["code"] != errHostedResultExpired.Error() {
		t.Fatalf("MCP result expiry: %v", expired)
	}
	if calls.Load() != 1 {
		t.Fatalf("MCP replay dispatched provider work %d times", calls.Load())
	}
}

func TestHostedMCPIdentityRejectsMissingOrInvalidKeys(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)
	_, clientFor := newHostedMCPIdentityServer(t, database, upstream.URL, t.TempDir())
	client := clientFor("owner")
	input := map[string]any{"tenant_id": "managed-first", "provider": "openai", "model": "gpt-4.1", "messages": []map[string]string{{"role": "user", "content": "invalid key"}}}
	for _, key := range []any{"missing", nil, "", " ", strings.Repeat("k", 129), 123} {
		if key != "missing" {
			input["idempotency_key"] = key
		}
		if result := hostedMCPCall(t, client, input, true); result["code"] != string(managedUsageOutcomeInvalidRequest) {
			t.Fatalf("invalid key %v: %v", key, result)
		}
	}
	if calls.Load() != 0 || len(read("")["requests"].([]any)) != 0 {
		t.Fatal("invalid MCP key created provider work or a journal request")
	}
}

func TestHostedMCPIdentityConcurrentNativeRequest(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"private-mcp-concurrent","status":"completed","output_text":"shared result","usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	server, clientFor := newHostedMCPIdentityServer(t, database, upstream.URL, t.TempDir())
	client := clientFor("owner")
	t.Cleanup(unblock)
	input := map[string]any{"tenant_id": "managed-first", "idempotency_key": "concurrent-mcp", "provider": "openai", "model": "gpt-4.1", "messages": []map[string]string{{"role": "user", "content": "concurrent prompt"}}}
	type outcome struct {
		result *mcp.CallToolResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: mcpGenerateTextTool, Arguments: input})
		finished <- outcome{result, err}
	}()
	select {
	case <-started:
	case result := <-finished:
		t.Fatalf("MCP did not dispatch: %+v", result)
	case <-time.After(5 * time.Second):
		t.Fatal("MCP did not dispatch")
	}
	pending := hostedMCPCall(t, client, input, false)
	if pending["state"] != structuredRequestStateDispatched || pending["request_id"] == "" {
		t.Fatalf("concurrent MCP result: %v", pending)
	}
	native := hostedIdentityHTTP(t, server, "concurrent-mcp", "concurrent prompt", http.StatusAccepted)
	if !strings.Contains(native, fmt.Sprintf(`"proxy_request_id":%q`, pending["request_id"])) {
		t.Fatalf("native duplicate changed the execution: %s MCP=%v", native, pending)
	}
	unblock()
	completed := <-finished
	if completed.err != nil || completed.result.IsError {
		t.Fatalf("first MCP request failed: %+v", completed)
	}
	replay := hostedMCPCall(t, client, input, false)
	if replay["request_id"] != pending["request_id"] || replay["text"] != "shared result" || calls.Load() != 1 {
		t.Fatalf("MCP concurrent request changed: %v calls=%d", replay, calls.Load())
	}
}

func TestHostedMCPIdentityRetainsUnknownOutcome(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if _, err := io.Copy(io.Discard, request.Body); err != nil {
			t.Error(err)
		}
		connection, _, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		connection.Close()
	}))
	t.Cleanup(upstream.Close)
	server, clientFor := newHostedMCPIdentityServer(t, database, upstream.URL, t.TempDir())
	client := clientFor("owner")
	input := map[string]any{"tenant_id": "managed-first", "idempotency_key": "unknown-mcp", "provider": "openai", "model": "gpt-4.1", "messages": []map[string]string{{"role": "user", "content": "uncertain prompt"}}}
	hostedMCPCall(t, client, input, true)
	result := hostedMCPCall(t, client, input, true)
	journal := read("")["requests"].([]any)[0].(map[string]any)
	if result["code"] != llmproxycontract.ErrorCodeStructuredRequestOutcomeUnknown || result["state"] != structuredRequestStateUncertain || result["request_id"] != journal["execution_id"] {
		t.Fatalf("MCP uncertainty lost its receipt: result=%v journal=%v", result, journal)
	}
	hostedIdentityHTTP(t, server, "unknown-mcp", "uncertain prompt", http.StatusConflict)
	if calls.Load() != 1 || journal["usage_state"] != "unknown" {
		t.Fatalf("unknown MCP outcome dispatched again or lost usage: calls=%d journal=%v", calls.Load(), journal)
	}
}

func hostedMCPCall(t *testing.T, client *mcp.ClientSession, arguments any, wantError bool) map[string]any {
	t.Helper()
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: mcpGenerateTextTool, Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError != wantError {
		t.Fatalf("MCP isError=%v want=%v result=%+v", result.IsError, wantError, result)
	}
	body, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func newHostedMCPIdentityServer(t *testing.T, database *gormManagedTenantDatabase, upstreamURL, root string, configure ...func(*hostedTextRequestDependencies)) (*httptest.Server, func(string) *mcp.ClientSession) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public, err := key.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	issuer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/oauth/jwks" {
			http.NotFound(writer, request)
			return
		}
		if err := json.NewEncoder(writer).Encode(oauthvalidator.JWKSet{Keys: []oauthvalidator.JWK{{KeyType: "EC", Use: "sig", KeyID: "hosted-fixture", Algorithm: "ES256", Curve: "P-256", X: base64.RawURLEncoding.EncodeToString(public[1:33]), Y: base64.RawURLEncoding.EncodeToString(public[33:])}}}); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(issuer.Close)
	router, management, providers := newHostedIdentityHTTPHandler(t, database, upstreamURL, root, configure...)
	server := httptest.NewUnstartedServer(router)
	configuration := Configuration{MaxPromptBytes: 4096, Management: management.configuration}
	configuration.Management.TAuthURL = issuer.URL
	configuration.Management.ProxyOrigin = "http://" + server.Listener.Addr().String()
	configuration.requestTimeoutPolicy, err = newRequestTimeoutPolicy(30, 30)
	if err != nil {
		t.Fatal(err)
	}
	if err := registerMCPRoutes(router, configuration, management, providers, newTenantAssetStore(root, 4096, 60), nil); err != nil {
		t.Fatal(err)
	}
	server.Start()
	server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
	t.Cleanup(server.Close)
	clientFor := func(subject string) *mcp.ClientSession {
		t.Helper()
		claims := oauthvalidator.Claims{ClientID: "hosted-fixture", Scope: mcpUseScope, TenantID: configuration.Management.TAuthTenantID, GrantID: "hosted-fixture-grant", RegisteredClaims: jwt.RegisteredClaims{Issuer: issuer.URL, Subject: subject, Audience: jwt.ClaimStrings{server.URL}, IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Second)), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
		token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
		token.Header["kid"], token.Header["typ"] = "hosted-fixture", "at+jwt"
		signed, err := token.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		client := mcp.NewClient(&mcp.Implementation{Name: "Hosted identity acceptance", Version: "1.0.0"}, nil)
		session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: server.URL + mcpPath, HTTPClient: &http.Client{Transport: hostedMCPBearerTransport{token: signed}}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { session.Close() })
		return session
	}
	return server, clientFor
}

type hostedMCPBearerTransport struct{ token string }

func (transport hostedMCPBearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(request)
}
