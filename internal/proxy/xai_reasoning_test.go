package proxy_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
)

func TestXAIResponsesGrok43ReasoningThroughOfficialClient(t *testing.T) {
	const schema = `{"type":"object","properties":{"answer":{"type":"string","pattern":"^[a-z]+$"}},"required":["answer"],"additionalProperties":false}`
	for _, scenario := range []struct {
		name, model, effort string
		structured          bool
		accepted            bool
	}{
		{name: "explicit none", model: "grok-4.3", effort: "none", accepted: true},
		{name: "exact alias", model: "grok-4.3-latest", effort: "none", accepted: true},
		{name: "structured none", model: "grok-4.3", effort: "none", structured: true, accepted: true},
		{name: "omitted effort", model: "grok-4.3", accepted: true},
		{name: "unsupported effort", model: "grok-4.3", effort: "minimal"},
		{name: "unrelated model", model: "grok-4.5", effort: "none"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.Method != http.MethodPost || request.URL.Path != "/responses" || request.Header.Get("Authorization") != "Bearer sk-xai" {
					t.Error("xAI request lost its method, endpoint, or credential")
				}
				var payload map[string]json.RawMessage
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Error(err)
					writer.WriteHeader(http.StatusBadRequest)
					return
				}
				var model string
				var limit int
				var store bool
				if json.Unmarshal(payload["model"], &model) != nil || model != scenario.model || json.Unmarshal(payload["max_output_tokens"], &limit) != nil || limit != 32 || json.Unmarshal(payload["store"], &store) != nil || store {
					t.Error("xAI request changed the exact model, output limit, or storage control")
				}
				if scenario.effort == "" {
					if _, exists := payload["reasoning"]; exists {
						t.Error("omitted reasoning became an explicit override")
					}
				} else {
					var reasoning map[string]string
					if json.Unmarshal(payload["reasoning"], &reasoning) != nil || len(reasoning) != 1 || reasoning["effort"] != scenario.effort {
						t.Error("explicit reasoning effort did not reach the xAI payload")
					}
				}
				visible := "visible"
				if scenario.structured {
					var text struct {
						Format struct {
							Type   string          `json:"type"`
							Strict bool            `json:"strict"`
							Schema json.RawMessage `json:"schema"`
						} `json:"format"`
					}
					var expectedSchema, observedSchema any
					if json.Unmarshal(payload["text"], &text) != nil || json.Unmarshal([]byte(schema), &expectedSchema) != nil || json.Unmarshal(text.Format.Schema, &observedSchema) != nil || text.Format.Type != "json_schema" || !text.Format.Strict || !reflect.DeepEqual(expectedSchema, observedSchema) {
						t.Error("reasoning route changed the provider-enforced schema")
					}
					visible = `{"answer":"visible"}`
				}
				encoded, err := json.Marshal(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": visible}}}}, "usage": map[string]int{"input_tokens": 2, "output_tokens": 3, "total_tokens": 5}})
				if err != nil {
					t.Error(err)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write(encoded)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"xai": upstream.URL}, nil), AssetStorePath: t.TempDir()}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: TestSecret, Provider: "xai"})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(configuration, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			limit := 32
			input := llmproxyclient.MessagesRequestInput{Model: scenario.model, MaxTokens: &limit, Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Return visible"}}}
			if scenario.effort != "" {
				input.ReasoningEffort = &scenario.effort
			}
			if scenario.structured {
				input.StructuredOutput = &llmproxyclient.StructuredOutputInput{JSONSchema: []byte(schema)}
				input.IdempotencyKey = "grok43-structured-none"
			}
			request, err := llmproxyclient.NewMessagesRequest(input)
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.PostMessages(t.Context(), request)
			if !scenario.accepted {
				var failure *llmproxyclient.HTTPFailure
				if !errors.As(err, &failure) || failure.StatusCode() != http.StatusBadRequest || calls.Load() != 0 {
					t.Fatalf("unsupported route: calls=%d error=%v", calls.Load(), err)
				}
				return
			}
			want := "visible"
			if scenario.structured {
				want = `{"answer":"visible"}`
			}
			if err != nil || result != want || calls.Load() != 1 {
				t.Fatalf("completion: result=%q calls=%d error=%v", result, calls.Load(), err)
			}
		})
	}
}

func TestManagementXAIResponsesVerificationDisablesDeclaredReasoning(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		var payload struct {
			Model           string            `json:"model"`
			MaxOutputTokens int               `json:"max_output_tokens"`
			Store           *bool             `json:"store"`
			Reasoning       map[string]string `json:"reasoning"`
		}
		if json.NewDecoder(request.Body).Decode(&payload) != nil || request.Method != http.MethodPost || request.URL.Path != "/responses" || payload.Model != "grok-4.3" || payload.MaxOutputTokens != 16 || payload.Store == nil || *payload.Store || len(payload.Reasoning) != 1 || payload.Reasoning["effort"] != "none" {
			t.Error("verification did not select the declared nonreasoning mode")
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}]}`)
	}))
	defer upstream.Close()
	router := newOperationalProviderKeyVerificationRouter(t, providerKeyVerificationConfiguration(upstream.URL), zap.NewNop().Sugar(), t.TempDir()+"/managed-tenants.db", TestTimeout)
	server := httptest.NewServer(router)
	defer server.Close()
	cookie := managementSessionCookie(t, "grok43-none-verification")
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/management/connections", strings.NewReader(`{"name":"Bounded xAI","provider":"xai","fields":{"api_key":"candidate-xai"}}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "grok43-none-verification")
	request.AddCookie(cookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated || calls.Load() != 1 {
		t.Fatalf("connection verification: status=%d calls=%d body=%s", response.StatusCode, calls.Load(), body)
	}
}
