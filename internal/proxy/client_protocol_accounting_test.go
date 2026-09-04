package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func TestClientProtocolsTenantDiscoveryAndAccounting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer provider-fixture" {
			t.Error("incorrect provider credential")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output":[{"type":"function_call","call_id":"a","name":"read","arguments":"{}"},{"type":"function_call","call_id":"b","name":"read","arguments":"{}"}],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`)
	}))
	defer upstream.Close()
	router := newManagementRouter(t, proxy.Configuration{Endpoints: providerEndpoints(upstream.URL, proxy.ProviderNameOpenAI)})
	owner := managementSessionCookie(t, "protocol-owner")
	account := requestManagementAccount(t, router, owner)
	first := account.Tenants[0].ID
	second := createManagementTenant(t, router, owner, "No provider").Tenant.ID
	saveManagementProviderKey(t, router, owner, first, "provider-fixture", proxy.ModelNameGPT41, "")
	key := generateManagementTenantSecret(t, router, owner, first)
	emptyKey := generateManagementTenantSecret(t, router, owner, second)
	server := httptest.NewServer(router)
	defer server.Close()
	for _, test := range []struct {
		key   string
		empty bool
	}{{key, false}, {emptyKey, true}} {
		req, _ := http.NewRequest("GET", server.URL+"/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+test.key)
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("models: %d %s", response.StatusCode, body)
		}
		var list struct {
			Data []struct {
				ID      string `json:"id"`
				Created int64  `json:"created"`
				Owner   string `json:"owned_by"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &list); err != nil {
			t.Fatal(err)
		}
		if test.empty != (len(list.Data) == 0) {
			t.Fatalf("tenant discovery empty=%v records=%d", test.empty, len(list.Data))
		}
		previous := ""
		for _, model := range list.Data {
			if !strings.HasPrefix(model.ID, "openai/") || model.ID <= previous || model.Created <= 0 || model.Owner == "" {
				t.Fatalf("invalid model record=%+v", model)
			}
			previous = model.ID
		}
	}
	for _, path := range []string{"/v1/chat/completions", "/v1/responses"} {
		body := `{"model":"openai/gpt-4.1","messages":[{"role":"user","content":"read twice"}],"tools":[{"type":"function","function":{"name":"read","parameters":{"type":"object"}}}],"stream":true,"parallel_tool_calls":true,"stream_options":{"include_usage":true}}`
		if path == "/v1/responses" {
			body = `{"model":"openai/gpt-4.1","input":"read twice","tools":[{"type":"function","name":"read","parameters":{"type":"object"}}],"stream":true,"parallel_tool_calls":true}`
		}
		req, _ := http.NewRequest("POST", server.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("stream: %d %s", response.StatusCode, data)
		}
	}
	usage := waitForManagementValue(t, func() managementTenantUsageTestResponse { return requestManagementTenantUsage(t, router, owner, first) }, func(value managementTenantUsageTestResponse) bool { return value.Totals.Requests == 2 })
	if usage.Totals.Requests != 2 {
		t.Fatalf("usage events=%d want=2", usage.Totals.Requests)
	}
	if other := requestManagementTenantUsage(t, router, owner, second); other.Totals.Requests != 0 {
		t.Fatalf("discovery recorded generation: %d", other.Totals.Requests)
	}
}
