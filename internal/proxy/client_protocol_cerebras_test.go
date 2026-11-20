package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tyemirov/tauth/pkg/sessionvalidator"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
)

const cerebrasQualifiedModel = "qwen3.8-27b"
const cerebrasQualifiedSchema = `{"type":"object","properties":{"answer":{"type":"string","enum":["yes"]},"optional":{"type":"number","minimum":0}},"required":["answer"],"additionalProperties":false}`

func cerebrasCandidateCatalog(t *testing.T, provider string) *proxy.ProviderCatalog {
	t.Helper()
	canonical := testfixtures.ProviderCatalog(t)
	if provider == "cerebras" {
		return canonical
	}
	schema := canonical.Schema()
	for index := range schema.Providers {
		if schema.Providers[index].ID == "cerebras" {
			schema.Providers[index].ID = provider
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func cerebrasHTTPServer(t *testing.T, upstream, provider string) *httptest.Server {
	t.Helper()
	tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenant.Defaults = proxy.TenantDefaults{Provider: provider, Model: cerebrasQualifiedModel}
	tenant.ProviderKeys = map[string]string{provider: "provider-fixture"}
	tenant.ProviderTextModels = map[string]string{provider: cerebrasQualifiedModel}
	router, err := buildRouterWithManagedTenant(t, proxy.Configuration{AssetStorePath: t.TempDir(), ProviderCatalog: cerebrasCandidateCatalog(t, provider), Endpoints: providerEndpointOverrides(map[string]string{provider: upstream}, nil)}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server
}

func cerebrasPost(t *testing.T, server *httptest.Server, path, body, key string) (int, http.Header, []byte) {
	t.Helper()
	if strings.HasPrefix(path, "/v2?") {
		path += "&key=" + TestSecret
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+TestSecret)
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header, data
}

func TestClientProtocolsCerebrasOfficialClientAndArbitraryProvider(t *testing.T) {
	for _, provider := range []string{"cerebras", "independent-inference"} {
		t.Run(provider, func(t *testing.T) {
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "POST" || r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer provider-fixture" {
					t.Errorf("provider boundary %s %s", r.Method, r.URL.Path)
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				format, _ := payload["response_format"].(map[string]any)
				schema, _ := format["json_schema"].(map[string]any)
				if payload["model"] != "qwen-3.8-27b" || payload["reasoning_effort"] != "medium" || payload["max_completion_tokens"] != float64(128) || payload["max_tokens"] != nil || format["type"] != "json_schema" || schema["strict"] != true || schema["name"] != "llm_proxy_structured_output" || len(schema) != 3 {
					t.Errorf("wire payload=%v", payload)
				}
				observed, _ := json.Marshal(schema["schema"])
				if !jsonBodiesEqual(observed, []byte(cerebrasQualifiedSchema)) {
					t.Errorf("schema changed %s", observed)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"choices":[{"message":{"content":" {\"answer\" : \"yes\"} "},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14,"completion_tokens_details":{"reasoning_tokens":3}}}`)
			}))
			defer upstream.Close()
			server := cerebrasHTTPServer(t, upstream.URL, provider)
			config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: TestSecret, Provider: provider})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(config, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			cap, err := client.GetPublicCapabilities(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, offering := range cap.Offerings {
				if offering.Provider == provider && offering.Model == cerebrasQualifiedModel {
					found = slices.Contains(offering.Capabilities, "structured_output")
				}
			}
			if !found {
				t.Fatal("offering schema capability absent")
			}
			effort, limit := "medium", 128
			input := llmproxyclient.MessagesRequestInput{Model: cerebrasQualifiedModel, MaxTokens: &limit, ReasoningEffort: &effort, Messages: []llmproxyclient.MessageInput{{Role: "system", Content: "Return JSON"}, {Role: "user", Content: "Answer yes"}}, StructuredOutput: &llmproxyclient.StructuredOutputInput{JSONSchema: []byte(cerebrasQualifiedSchema)}, IdempotencyKey: "qualification:schema"}
			request, err := llmproxyclient.NewMessagesRequest(input)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				output, err := client.PostMessages(context.Background(), request)
				if err != nil || !jsonBodiesEqual([]byte(output), []byte(`{"answer":"yes"}`)) {
					t.Fatalf("output=%s err=%v", output, err)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("replay provider calls=%d", calls.Load())
			}
			input.Messages = []llmproxyclient.MessageInput{{Role: "user", Content: "Changed"}}
			conflict, err := llmproxyclient.NewMessagesRequest(input)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.PostMessages(context.Background(), conflict); err == nil || calls.Load() != 1 {
				t.Fatalf("conflict err=%v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestClientProtocolsCerebrasAdmissionAndFailureUsage(t *testing.T) {
	for _, scenario := range []struct {
		name, finish, output, usage string
		status                      int
	}{
		{"success", "stop", `{"answer":"yes"}`, `,"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14}`, 200},
		{"reasoning-included-in-output", "stop", `{"answer":"yes"}`, `,"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14,"completion_tokens_details":{"reasoning_tokens":3}}`, 200},
		{"failed-reasoning-included-in-output", "length", `{"answer":"yes"}`, `,"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14,"completion_tokens_details":{"reasoning_tokens":3}}`, 502},
		{"truncated", "length", `{"answer":"yes"}`, `,"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14}`, 502},
		{"invalid-schema", "stop", `{"answer":"no"}`, `,"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14}`, 502},
		{"missing-usage", "stop", `{"answer":"yes"}`, ``, 200},
		{"unknown-finish", "unknown", `{"answer":"yes"}`, `,"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14}`, 502},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				encoded, _ := json.Marshal(scenario.output)
				io.WriteString(w, `{"choices":[{"message":{"content":`+string(encoded)+`},"finish_reason":"`+scenario.finish+`"}]`+scenario.usage+`}`)
			}))
			defer upstream.Close()
			server := cerebrasHTTPServer(t, upstream.URL, "cerebras")
			body := `{"model":"qwen3.8-27b","messages":[{"role":"user","content":"JSON"}],"max_tokens":128,"structured_output":{"schema":` + cerebrasQualifiedSchema + `}}`
			status, headers, data := cerebrasPost(t, server, "/v2?provider=cerebras", body, "cerebras:"+scenario.name)
			if status != scenario.status || calls.Load() != 1 {
				t.Fatalf("status=%d body=%s calls=%d", status, data, calls.Load())
			}
			if scenario.usage == "" {
				if headers.Get("X-LLM-Proxy-Total-Tokens") != "" {
					t.Fatal("missing usage became measured")
				}
			} else if scenario.status == 200 && headers.Get("X-LLM-Proxy-Total-Tokens") != "14" {
				t.Fatalf("usage missing: %v", headers)
			}
			management := proxy.ManagedRouterTestManagementConfiguration()
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, &sessionvalidator.Claims{TenantID: management.TAuthTenantID, UserID: "managed-router-test-user-1", UserEmail: "owner@example.com", UserDisplayName: "owner", RegisteredClaims: jwt.RegisteredClaims{Issuer: proxy.DefaultManagementJWTIssuer, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
			signed, err := token.SignedString([]byte(management.JWTSigningKey))
			if err != nil {
				t.Fatal(err)
			}
			owner := &http.Cookie{Name: management.SessionCookieName, Value: signed}
			usage := waitForManagementValue(t, func() managementUsageTestResponse {
				return requestManagementUsage(t, server.Config.Handler, owner, "all")
			}, func(value managementUsageTestResponse) bool { return value.Totals.Requests == 1 })
			expected := 14
			if scenario.usage == "" {
				expected = 0
			}
			if usage.Totals.FailedRequests != map[bool]int{true: 0, false: 1}[scenario.status == http.StatusOK] || usage.Totals.TotalTokens != expected || usage.Totals.ResponseTokens != map[bool]int{true: 5, false: 0}[scenario.usage != ""] {
				t.Fatalf("operational usage=%+v", usage.Totals)
			}

		})
	}
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer upstream.Close()
	server := cerebrasHTTPServer(t, upstream.URL, "cerebras")
	for _, extra := range []string{`,"reasoning_effort":"xhigh"`, `,"max_tokens":65537`, `,"web_search":true`, `,"structured_output":{"schema":{"type":"object","properties":{"a":{"type":"string","pattern":"x"}},"additionalProperties":false}}`} {
		body := `{"model":"qwen3.8-27b","messages":[{"role":"user","content":"hello"}]` + extra + `}`
		status, _, data := cerebrasPost(t, server, "/v2?provider=cerebras", body, "cerebras:negative")
		if status != 400 || calls.Load() != 0 {
			t.Fatalf("rejection status=%d body=%s calls=%d", status, data, calls.Load())
		}
	}
	status, _, data := cerebrasPost(t, server, "/v1/chat/completions", `{"model":"cerebras/qwen3.8-27b","messages":[{"role":"user","content":"hello"}],"temperature":0.2}`, "")
	if status != 400 || calls.Load() != 0 {
		t.Fatalf("sampling status=%d body=%s calls=%d", status, data, calls.Load())
	}
	request, _ := http.NewRequest("GET", server.URL+"/v1/models", nil)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 401 || bytes.Contains(data, []byte("provider-fixture")) {
		t.Fatalf("auth status=%d body=%s", response.StatusCode, data)
	}
}

func TestClientProtocolsCerebrasUnchangedStrictToolsAdmission(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output_text":"done"}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"openai": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, scenario := range []struct {
		name, required string
		status         int
	}{{"closed-required", `,"required":["value"]`, 200}, {"optional-property", ``, 400}} {
		t.Run(scenario.name, func(t *testing.T) {
			body := `{"model":"openai/gpt-5.6","messages":[{"role":"user","content":"Read value"}],"tools":[{"type":"function","function":{"name":"read","strict":true,"parameters":{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string"}}` + scenario.required + `}}}]}`
			status, _, data := cerebrasPost(t, server, "/v1/chat/completions", body, "")
			if status != scenario.status {
				t.Fatalf("strict tools status=%d body=%s", status, data)
			}
		})
	}
	if calls.Load() != 1 {
		t.Fatalf("strict tool parameter admission provider calls=%d", calls.Load())
	}
}
