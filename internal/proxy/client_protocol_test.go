package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
)

func TestClientProtocolsTextAndTools(t *testing.T) {
	contract, err := openapitest.Load("../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+TestAPIKey {
			t.Error("upstream credential boundary")
		}
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		if len(payload["tools"]) > 0 && !bytes.Contains(payload["input"], []byte("function_call_output")) {
			io.WriteString(w, `{"id":"private-id","status":"completed","output":[{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"hello.txt\"}"}],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`)
			return
		}
		io.WriteString(w, `{"id":"private-id","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello client"}]}],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameOpenAI: upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, test := range []struct{ path, body, want string }{
		{"/v1/models", "", `"openai/`},
		{"/v1/chat/completions", `{"model":"openai/gpt-5.6","messages":[{"role":"user","content":"hello"}]}`, `"chat.completion"`},
		{"/v1/responses", `{"model":"openai/gpt-5.6","input":"hello","store":false}`, `"output_text"`},
		{"/v1/chat/completions", `{"model":"openai/gpt-5.6","messages":[{"role":"user","content":"read"}],"tools":[{"type":"function","function":{"name":"read_file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}}}]}`, `"tool_calls"`},
		{"/v1/responses", `{"model":"openai/gpt-5.6","input":"read","tools":[{"type":"function","name":"read_file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}}]}`, `"function_call"`},
		{"/v1/chat/completions", `{"model":"openai/gpt-5.6","messages":[{"role":"user","content":"hello"}],"stream":true}`, "data: [DONE]"},
		{"/v1/responses", `{"model":"openai/gpt-5.6","input":"hello","stream":true}`, "event: response.completed"},
	} {
		t.Run(test.path+test.want, func(t *testing.T) {
			method := http.MethodPost
			if test.body == "" {
				method = http.MethodGet
			}
			req, _ := http.NewRequest(method, server.URL+test.path, strings.NewReader(test.body))
			req.Header.Set("Authorization", "Bearer "+TestSecret)
			req.Header.Set("Content-Type", "application/json")
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if err := contract.ValidateRequest(test.path, method, req, []byte(test.body)); err != nil {
				t.Fatal(err)
			}
			if err := contract.ValidateResponse(test.path, method, response.StatusCode, response.Header, data); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || !strings.Contains(string(data), test.want) {
				t.Fatalf("status=%d body=%s", response.StatusCode, data)
			}
			if bytes.Contains(data, []byte("private-id")) {
				t.Fatal("upstream identifier exposed")
			}
		})
	}
}

func TestClientProtocolsOpenAISDK(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+TestAPIKey {
			t.Error("tenant credential reached provider")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "transcriptions") {
			io.WriteString(w, `{"text":"fixture transcription"}`)
			return
		}
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if len(payload["tools"]) > 0 && !bytes.Contains(payload["input"], []byte("function_call_output")) {
			io.WriteString(w, `{"id":"private-id","status":"completed","output":[{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"hello.txt\"}"}],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`)
			return
		}
		io.WriteString(w, `{"id":"private-id","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello client"}]}],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameOpenAI: upstream.URL}, map[string]map[string]string{proxy.ProviderNameOpenAI: {"dictation": upstream.URL + "/audio/transcriptions"}})}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	script, err := filepath.Abs("../../tests/client-protocols/openai-sdk.mjs")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("node", script)
	command.Env = append(os.Environ(), "PROTOCOL_BASE_URL="+server.URL+"/v1", "PROTOCOL_TENANT_KEY="+TestSecret)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OpenAI SDK: %v\n%s", err, output)
	}
	exerciseNativeClients(t, server)
}

func exerciseNativeClients(t *testing.T, server *httptest.Server) {
	t.Helper()
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: TestSecret, Provider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := client.GetPublicCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foundTools := false
	for _, offering := range catalog.Offerings {
		if offering.Provider == "openai" && offering.Model == "gpt-5.6" {
			for _, capability := range offering.Capabilities {
				foundTools = foundTools || capability == "caller_tools"
			}
		}
	}
	if !foundTools {
		t.Fatal("native client lost the caller tool capability")
	}
	declarations := []llmproxyclient.FunctionDeclaration{{Name: "read_file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`)}}
	messages := []llmproxyclient.MessageInput{{Role: "user", Content: "Read fixture"}}
	parallel := false
	request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: messages, Model: "gpt-5.6", Tools: declarations, ToolChoice: &llmproxyclient.ToolChoice{Mode: "function", Name: "read_file"}, ParallelToolCalls: &parallel})
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.PostMessages(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Type  string                        `json:"type"`
		Calls []llmproxyclient.FunctionCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if result.Type != "tool_calls" || len(result.Calls) != 1 {
		t.Fatalf("result=%s", body)
	}
	messages = append(messages, llmproxyclient.MessageInput{Role: "assistant", ToolCalls: result.Calls}, llmproxyclient.MessageInput{Role: "tool", Content: "fixture read complete", ToolCallID: result.Calls[0].ID})
	request, err = llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Messages: messages, Model: "gpt-5.6", Tools: declarations})
	if err != nil {
		t.Fatal(err)
	}
	body, err = client.PostMessages(context.Background(), request)
	if err != nil || body != "hello client" {
		t.Fatalf("native round trip: %v %s", err, body)
	}
	script, _ := filepath.Abs("../../tests/client-protocols/native-python.py")
	pythonPath, _ := filepath.Abs("../../python")
	command := exec.Command("python3", script)
	command.Env = append(os.Environ(), "PYTHONPATH="+pythonPath, "PROTOCOL_BASE_URL="+server.URL, "PROTOCOL_TENANT_KEY="+TestSecret)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native Python: %v\n%s", err, output)
	}
}

func TestClientProtocolsOpenCode(t *testing.T) {
	for _, protocol := range []struct{ label, pkg, path string }{{"chat", "@ai-sdk/openai-compatible", "/v1/chat/completions"}, {"responses", "@ai-sdk/openai", "/v1/responses"}} {
		t.Run(protocol.label, func(t *testing.T) {
			directory, directoryError := filepath.EvalSymlinks(t.TempDir())
			if directoryError != nil {
				t.Fatal(directoryError)
			}
			fixture := filepath.Join(directory, "fixture.txt")
			if err := os.WriteFile(fixture, []byte("fixture read complete"), 0o644); err != nil {
				t.Fatal(err)
			}
			var observedResult atomic.Bool
			var observedProtocol atomic.Bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+TestAPIKey {
					t.Error("provider authentication")
				}
				var payload struct {
					Input json.RawMessage `json:"input"`
					Tools []struct {
						Name string `json:"name"`
					} `json:"tools"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "application/json")
				if bytes.Contains(payload.Input, []byte("function_call_output")) && bytes.Contains(payload.Input, []byte("fixture read complete")) {
					observedResult.Store(true)
				}
				hasRead := false
				for _, tool := range payload.Tools {
					if tool.Name == "read" {
						hasRead = true
					}
				}
				if hasRead && !observedResult.Load() {
					args, _ := json.Marshal(map[string]string{"filePath": fixture})
					body, _ := json.Marshal(map[string]any{"id": "private-id", "status": "completed", "output": []any{map[string]any{"type": "function_call", "call_id": "call_read", "name": "read", "arguments": string(args)}}, "usage": map[string]int{"input_tokens": 10, "output_tokens": 3, "total_tokens": 13}})
					w.Write(body)
					return
				}
				io.WriteString(w, `{"id":"private-id","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Fixture verified."}]}],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameOpenAI: upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == protocol.path && r.Header.Get("Authorization") == "Bearer "+TestSecret {
					observedProtocol.Store(true)
				}
				router.ServeHTTP(w, r)
			}))
			defer server.Close()

			exampleName := "chat-completions.json"
			if protocol.label == "responses" {
				exampleName = "responses.json"
			}
			example, err := os.ReadFile("../../examples/opencode/" + exampleName)
			if err != nil {
				t.Fatal(err)
			}
			var config map[string]any
			if err := json.Unmarshal(example, &config); err != nil {
				t.Fatal(err)
			}
			config["small_model"] = config["model"]
			config["enabled_providers"] = []string{"llmproxy"}
			config["share"] = "disabled"
			config["autoupdate"] = false
			config["permission"] = map[string]any{"*": "deny", "read": "allow", "external_directory": map[string]string{directory + "/*": "allow"}}
			config["provider"].(map[string]any)["llmproxy"].(map[string]any)["options"].(map[string]any)["baseURL"] = server.URL + "/v1"

			encoded, _ := json.Marshal(config)
			configPath := filepath.Join(directory, "opencode.json")
			if err := os.WriteFile(configPath, encoded, 0o644); err != nil {
				t.Fatal(err)
			}
			platform := runtime.GOOS
			architecture := runtime.GOARCH
			if architecture == "amd64" {
				architecture = "x64"
			}
			binary, err := filepath.Abs("../../node_modules/opencode-" + platform + "-" + architecture + "/bin/opencode")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, "run", "--dir", directory, "--pure", "--format", "json", "--title", "Protocol fixture", "Read fixture.txt using the read tool and report its contents.")
			command.Dir = directory
			command.Env = append(os.Environ(), "PWD="+directory, "LLM_PROXY_CLIENT_KEY="+TestSecret, "OPENCODE_CONFIG="+configPath, "XDG_CONFIG_HOME="+directory+"/config", "XDG_DATA_HOME="+directory+"/data", "XDG_CACHE_HOME="+directory+"/cache", "OPENCODE_DISABLE_AUTOUPDATE=true", "OPENCODE_DISABLE_MODELS_FETCH=true", "OPENCODE_DISABLE_DEFAULT_PLUGINS=true")
			output, err := command.CombinedOutput()
			if err != nil || !observedResult.Load() || !observedProtocol.Load() {
				t.Fatalf("OpenCode err=%v protocol=%v result=%v\n%s", err, observedProtocol.Load(), observedResult.Load(), output)
			}
		})
	}
}
