package proxy_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/tauth/pkg/oauthvalidator"
)

// Exercise the initialize wire contract used by Codex and other HTTP clients.
func TestMCPHandshakeVersions(t *testing.T) {
	for _, version := range []string{"2025-03-26", "2025-06-18", "2025-11-25"} {
		t.Run(version, func(t *testing.T) {
			fixture := newMCPFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"id":"response-fixture","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"version acceptance"}]}]}`)
			}))
			owner := managementSessionCookie(t, "version-owner")
			tenant := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
			foreign := requestManagementAccount(t, fixture.router, managementSessionCookie(t, "other-owner")).Tenants[0].ID
			saveManagementProviderKey(t, fixture.router, owner, tenant, "fixture-key", proxy.ModelNameGPT41, "")
			token := fixture.token(t, "version-owner", nil)
			call := func(headerVersion, method string, params any, notification bool, bearer string, status int) map[string]any {
				t.Helper()
				message := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
				if !notification {
					message["id"] = 1
				}
				body, err := json.Marshal(message)
				if err != nil {
					t.Fatal(err)
				}
				request, err := http.NewRequestWithContext(t.Context(), "POST", fixture.server.URL+"/mcp", strings.NewReader(string(body)))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Accept", "application/json, text/event-stream")
				if bearer != "" {
					request.Header.Set("Authorization", "Bearer "+bearer)
				}
				if headerVersion != "" {
					request.Header.Set("MCP-Protocol-Version", headerVersion)
				}
				response, err := fixture.server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				data, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != status {
					t.Fatalf("%s version=%q: HTTP %d want %d: %s", method, headerVersion, response.StatusCode, status, data)
				}
				if response.Header.Get("Mcp-Session-Id") != "" || response.Header.Get("Cache-Control") != "no-store" {
					t.Fatal("response created a session or became cacheable")
				}
				if status != http.StatusOK {
					return nil
				}
				var envelope struct {
					Result map[string]any `json:"result"`
					Error  any            `json:"error"`
				}
				if err := json.Unmarshal(data, &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Error != nil {
					t.Fatalf("%s RPC error: %v", method, envelope.Error)
				}
				return envelope.Result
			}
			params := map[string]any{"protocolVersion": version, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "protocol-acceptance", "version": "1"}}
			call("", "initialize", params, false, "", 401)
			initialized := call("", "initialize", params, false, token, 200)
			if initialized["protocolVersion"] != version {
				t.Fatalf("negotiated %v want %s", initialized["protocolVersion"], version)
			}
			for _, offered := range []string{"2024-11-05", "2026-07-28", "2099-01-01"} {
				params["protocolVersion"] = offered
				negotiated := call("", "initialize", params, false, token, 200)
				if negotiated["protocolVersion"] != "2025-11-25" {
					t.Fatalf("unsupported offer %s negotiated %v", offered, negotiated)
				}
			}
			call("2099-01-01", "tools/list", map[string]any{}, false, token, 400)
			call("", "tools/list", map[string]any{}, false, token, 200)
			call(version, "notifications/initialized", map[string]any{}, true, token, 202)
			tools := call(version, "tools/list", map[string]any{}, false, token, 200)
			if len(tools["tools"].([]any)) != 2 {
				t.Fatalf("tools=%v", tools)
			}
			tenants := call(version, "tools/call", map[string]any{"name": "llm_proxy.list_tenants", "arguments": map[string]any{}}, false, token, 200)
			if text := fmt.Sprint(tenants); !strings.Contains(text, tenant) || strings.Contains(text, foreign) {
				t.Fatalf("tenant isolation: %s", text)
			}
			routes := call(version, "resources/read", map[string]any{"uri": "llm-proxy://tenants/" + tenant + "/routes"}, false, token, 200)
			if !strings.Contains(fmt.Sprint(routes), "openai") {
				t.Fatalf("routes=%v", routes)
			}
			for _, id := range []string{tenant, foreign} {
				result := call(version, "tools/call", map[string]any{"name": "llm_proxy.generate_text", "arguments": map[string]any{"tenant_id": id, "messages": []map[string]string{{"role": "user", "content": "hello"}}}}, false, token, 200)
				if id == tenant && !strings.Contains(fmt.Sprint(result), "version acceptance") {
					t.Fatalf("generation=%v", result)
				}
				if id == foreign && (result["isError"] != true || !strings.Contains(fmt.Sprint(result), "not_found")) {
					t.Fatalf("foreign generation=%v", result)
				}
			}
			call(version, "tools/list", map[string]any{}, false, "invalid", 401)
			wrongScope := fixture.token(t, "version-owner", func(claims *oauthvalidator.Claims) { claims.Scope = "other" })
			call(version, "tools/list", map[string]any{}, false, wrongScope, 403)
			fixture.waitUsageEvents(t, 1)
		})
	}
}
