package llmproxyclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestMessagesRequestCompletion(t *testing.T) {
	for _, fixture := range []struct {
		name, body, model    string
		status               int
		wantUsage, wantError bool
	}{
		{"measured", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"private response"}]}],"model":"exact-model","usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`, "exact-model", 200, true, false},
		{"measured zero", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"private response"}]}],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`, "", 200, true, false},
		{"unavailable", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"private response"}]}]}`, "", 200, false, false},
		{"null metadata", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"private response"}]}],"model":null,"usage":null}`, "", 200, false, false},
		{"partial usage", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"private response"}]}],"model":"exact-model","usage":{"input_tokens":10}}`, "exact-model", 200, false, false},
		{"negative usage", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"private response"}]}],"usage":{"input_tokens":-1,"output_tokens":3,"total_tokens":2}}`, "", 200, false, false},
		{"invalid usage", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"private response"}]}],"usage":"private invalid data"}`, "", 200, false, false},
		{"invalid model", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"private response"}]}],"model":123}`, "", 200, false, false},
		{"incomplete", `{"object":"response","status":"incomplete","output":[]}`, "", 200, false, true},
		{"tool result", `{"object":"response","status":"completed","output":[{"type":"function_call"}]}`, "", 200, false, true},
		{"refusal", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"refusal","refusal":"private"}]}]}`, "", 200, false, true},
		{"empty output", `{"object":"response","status":"completed","output":[]}`, "", 200, false, true},
		{"reasoning and text", `{"object":"response","status":"completed","output":[{"type":"reasoning","summary":[]},{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"private response"}]}]}`, "", 200, false, false},
		{"failed", `{"error":"private failure"}`, "", 502, false, true},
		{"malformed response", `not JSON`, "", 200, false, true},
		{"missing text", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":null}]}],"model":"exact-model"}`, "", 200, false, true},
		{"wrong text type", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":123}]}]}`, "", 200, false, true},
		{"truncated response", `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"private response"}]}]}`, "", 200, false, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/responses" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Accept") != "application/json" {
					t.Error("wrong representation")
				}
				if fixture.name == "truncated response" {
					w.Header().Set("Content-Length", "1000")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(fixture.status)
				io.WriteString(w, fixture.body)
			}))
			defer server.Close()
			config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{Protocol: llmproxyclient.ProtocolOpenAIResponses, BaseURL: server.URL, Secret: "secret", Provider: "openai"})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(config, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Model: "requested-alias", Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "private prompt"}}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.PostMessagesCompletion(context.Background(), request)
			if (err != nil) != fixture.wantError {
				t.Fatalf("error=%v", err)
			}
			if err != nil && !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
				t.Fatalf("untyped error=%v", err)
			}
			if (result.Usage() != nil) != fixture.wantUsage {
				t.Fatalf("usage=%v", result.Usage())
			}
			invalidMetadata := strings.Contains(fixture.name, "invalid") || fixture.name == "partial usage" || fixture.name == "negative usage"
			if (result.MetadataError() != nil) != invalidMetadata {
				t.Fatalf("metadata error=%v", result.MetadataError())
			}
			if result.MetadataError() != nil && strings.Contains(result.MetadataError().Error(), "private") {
				t.Fatal("metadata diagnostic exposed content")
			}
			if result.ResolvedModel() != fixture.model {
				t.Fatalf("model=%q", result.ResolvedModel())
			}
			if fixture.wantUsage && result.Usage().InputTokens()+result.Usage().OutputTokens() != result.Usage().TotalTokens() {
				t.Fatal("token counts changed")
			}
			if fixture.wantError && result.Text() != "" {
				t.Fatal("failed response exposed as completion")
			}
			if !fixture.wantError && result.Text() != "private response" {
				t.Fatalf("text=%q", result.Text())
			}
		})
	}
}

func TestMessagesRequestCompletionRejectsUnsupportedInputs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid request reached server") }))
	defer server.Close()
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{Protocol: llmproxyclient.ProtocolOpenAIResponses, BaseURL: server.URL, Secret: "secret", ModelProfilePath: filepath.Join(t.TempDir(), "missing.json"), ModelProfileReader: os.ReadFile})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.PostMessagesCompletion(context.Background(), request); !errors.Is(err, llmproxyclient.ErrInvalidModelProfile) {
		t.Fatalf("profile error=%v", err)
	}
	request, err = llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "hello"}}, StructuredOutput: &llmproxyclient.StructuredOutputInput{JSONSchema: []byte(`{"type":"object","properties":{"decision":{"type":"string"}},"required":["decision"],"additionalProperties":false}`)}, IdempotencyKey: "measurement-test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.PostMessagesCompletion(context.Background(), request); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
		t.Fatalf("structured error=%v", err)
	}
}

func TestMessagesRequestCompletionProtocolSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2" {
			if r.URL.Query().Get("key") != "secret" || r.URL.Query().Get("format") != "text/plain" {
				t.Error("native contract changed")
			}
			io.WriteString(w, "native text")
			return
		}
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer secret" || r.URL.RawQuery != "" {
			t.Error("Responses protocol not selected")
		}
		var body struct {
			Model     string                                 `json:"model"`
			Input     []struct{ Type, Role, Content string } `json:"input"`
			Max       int                                    `json:"max_output_tokens"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
			Store bool `json:"store"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "openai/exact-model" || len(body.Input) != 1 || body.Input[0].Content != "hello" || body.Input[0].Type != "message" || body.Max != 200 || body.Reasoning.Effort != "low" || body.Store {
			t.Fatalf("request=%+v", body)
		}
		if r.Header.Get("X-LLM-Proxy-Request-Timeout-Seconds") != "300" {
			t.Error("work budget missing")
		}
		io.WriteString(w, `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"standard text"}]}]}`)
	}))
	defer server.Close()
	max, timeout, effort := 200, 300, "low"
	for _, protocol := range []llmproxyclient.Protocol{llmproxyclient.ProtocolNative, llmproxyclient.ProtocolOpenAIResponses} {
		config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{Protocol: protocol, BaseURL: server.URL, Secret: "secret", Provider: "openai"})
		if err != nil {
			t.Fatal(err)
		}
		address, err := config.MessagesPostURL()
		if err != nil {
			t.Fatal(err)
		}
		if protocol == llmproxyclient.ProtocolOpenAIResponses && address != server.URL+"/v1/responses" {
			t.Fatalf("endpoint=%s", address)
		}
		client, err := llmproxyclient.NewClient(config, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Model: "exact-model", Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "hello"}}, MaxTokens: &max, ReasoningEffort: &effort, RequestTimeoutSeconds: &timeout})
		if err != nil {
			t.Fatal(err)
		}
		result, err := client.PostMessagesCompletion(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		want := "native text"
		if protocol == llmproxyclient.ProtocolOpenAIResponses {
			want = "standard text"
		}
		if result.Text() != want || result.Usage() != nil {
			t.Fatalf("result=%+v", result)
		}
		text, err := client.PostMessages(t.Context(), request)
		if err != nil || text != want {
			t.Fatalf("text=%q error=%v", text, err)
		}
	}
	for _, input := range []llmproxyclient.ConfigInput{
		{Protocol: "unknown", BaseURL: server.URL, Secret: "secret"},
		{Protocol: llmproxyclient.ProtocolOpenAIResponses, BaseURL: server.URL + "?key=secret", Secret: "secret"},
	} {
		if _, err := llmproxyclient.NewConfig(input); !errors.Is(err, llmproxyclient.ErrInvalidClientConfig) {
			t.Fatalf("config error=%v", err)
		}
	}
}

func TestMessagesRequestCompletionProfilesAndUnsupportedRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "openai/exact-model" {
			t.Fatalf("profile model=%q", body.Model)
		}
		io.WriteString(w, `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"profile text"}]}]}`)
	}))
	defer server.Close()
	input := llmproxyclient.ConfigInput{Protocol: llmproxyclient.ProtocolOpenAIResponses, BaseURL: server.URL, Secret: "secret", ModelProfilePath: "selected.json", ModelProfileReader: func(string) ([]byte, error) { return []byte(`{"provider":"openai","model":"exact-model"}`), nil }}
	config, err := llmproxyclient.NewConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	messages := []llmproxyclient.MessageInput{{Role: "user", Content: "hello"}}
	request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.PostMessagesCompletion(t.Context(), request)
	if err != nil || result.Text() != "profile text" {
		t.Fatalf("profile result=%+v error=%v", result, err)
	}
	conflict, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: messages, Model: "conflict"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.PostMessagesCompletion(t.Context(), conflict); !errors.Is(err, llmproxyclient.ErrInvalidModelProfile) {
		t.Fatalf("conflict error=%v", err)
	}
	input.ModelProfilePath = ""
	input.ModelProfileReader = nil
	input.Provider = "openai"
	config, err = llmproxyclient.NewConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	client, err = llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []llmproxyclient.MessagesRequestInput{
		{Messages: messages},
		{Messages: messages, Model: "exact-model", WebSearch: true},
		{Messages: append(messages, llmproxyclient.MessageInput{Role: "assistant", ToolCalls: []llmproxyclient.FunctionCall{{ID: "call", Name: "lookup", Arguments: "{}"}}}, llmproxyclient.MessageInput{Role: "tool", Content: "tool text", ToolCallID: "call"}), Model: "exact-model"},
	} {
		request, err := llmproxyclient.NewMessagesRequest(invalid)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = client.PostMessagesCompletion(t.Context(), request); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
			t.Fatalf("unsupported error=%v", err)
		}
	}
}
