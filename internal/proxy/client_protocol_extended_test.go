package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func TestClientProtocolsProviderToolContract(t *testing.T) {
	for _, protocol := range []string{"responses", "chat"} {
		t.Run(protocol, func(t *testing.T) {
			result := `{"status":"completed","output_text":"hello"}`
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, result)
			}))
			defer upstream.Close()
			schema := testfixtures.ProviderCatalog(t).Schema()
			provider, model := "openai", "gpt-5.6"
			if protocol == "chat" {
				provider, model = "deepseek", "deepseek-chat"
				for p := range schema.Providers {
					if schema.Providers[p].ID == provider {
						for o := range schema.Providers[p].Offerings {
							if schema.Providers[p].Offerings[o].Model == model {
								schema.Providers[p].Offerings[o].CallerTools = true
							}
						}
					}
				}
			}
			catalog, err := proxy.NewProviderCatalog(schema)
			if err != nil {
				t.Fatal(err)
			}
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpointOverrides(map[string]string{provider: upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			tests := []struct {
				choice   string
				parallel *bool
				calls    string
				text     string
				status   int
			}{
				{"auto", nil, `[{"id":"a","name":"read","arguments":"{}"}]`, "", 200},
				{"function", nil, `[{"id":"a","name":"read","arguments":"{}"}]`, "visible", 200},
				{"none", nil, `[{"id":"a","name":"read","arguments":"{}"}]`, "", 502},
				{"auto", nil, `[{"id":"a","name":"other","arguments":"{}"}]`, "", 502},
				{"auto", nil, `[{"id":"a","name":"read","arguments":"invalid"}]`, "", 502},
				{"auto", nil, `[{"id":"a","name":"read","arguments":"{}"},{"id":"a","name":"read","arguments":"{}"}]`, "", 502},
				{"required", nil, `[]`, "no call", 502},
				{"auto", boolPointer(false), `[{"id":"a","name":"read","arguments":"{}"},{"id":"b","name":"read","arguments":"{}"}]`, "", 502},
				{"function", boolPointer(true), `[{"id":"a","name":"other","arguments":"{}"}]`, "", 502},
			}
			for index, test := range tests {
				t.Run(strconv.Itoa(index), func(t *testing.T) {
					var calls []map[string]string
					json.Unmarshal([]byte(test.calls), &calls)
					if protocol == "responses" {
						output := []any{}
						if test.text != "" {
							output = append(output, map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": test.text}}})
						}
						for _, call := range calls {
							output = append(output, map[string]string{"type": "function_call", "call_id": call["id"], "name": call["name"], "arguments": call["arguments"]})
						}
						data, _ := json.Marshal(map[string]any{"status": "completed", "output": output})
						result = string(data)
					} else {
						output := []any{}
						for _, call := range calls {
							output = append(output, map[string]any{"id": call["id"], "type": "function", "function": map[string]string{"name": call["name"], "arguments": call["arguments"]}})
						}
						finish := "stop"
						if len(calls) > 0 {
							finish = "tool_calls"
						}
						data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": test.text, "tool_calls": output}, "finish_reason": finish}}})
						result = string(data)
					}
					payload := map[string]any{"model": provider + "/" + model, "input": "read", "tools": []any{map[string]any{"type": "function", "name": "read", "parameters": map[string]string{"type": "object"}}, map[string]any{"type": "function", "name": "other", "parameters": map[string]string{"type": "object"}}}, "tool_choice": test.choice, "stream": index%2 == 0}
					// An undeclared function is distinct from a named-choice mismatch.
					if test.choice == "auto" {
						payload["tools"] = payload["tools"].([]any)[:1]
					}
					if test.choice == "function" {
						payload["tool_choice"] = map[string]string{"type": "function", "name": "read"}
					}
					if test.parallel != nil {
						payload["parallel_tool_calls"] = *test.parallel
					}
					data, _ := json.Marshal(payload)
					req, _ := http.NewRequest("POST", server.URL+"/v1/responses", bytes.NewReader(data))
					req.Header.Set("Authorization", "Bearer "+TestSecret)
					req.Header.Set("Content-Type", "application/json")
					response, err := server.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					body, _ := io.ReadAll(response.Body)
					response.Body.Close()
					if response.StatusCode != test.status {
						t.Fatalf("status=%d want=%d body=%s", response.StatusCode, test.status, body)
					}
				})
			}
			if protocol == "chat" {
				result = `{"choices":[{"message":{"content":"invalid stop","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}}]},"finish_reason":"stop"}]}`
				req, _ := http.NewRequest("POST", server.URL+"/v1/responses", strings.NewReader(`{"model":"deepseek/deepseek-chat","input":"hi"}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+TestSecret)
				response, err := server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				io.Copy(io.Discard, response.Body)
				response.Body.Close()
				if response.StatusCode != 502 {
					t.Fatalf("tool call with text finish status=%d", response.StatusCode)
				}
			}

		})
	}
}
func boolPointer(value bool) *bool { return &value }

func TestClientProtocolsTranscriptionValidation(t *testing.T) {
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{MaxInputAudioBytes: 16, MaxPromptBytes: 128}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, test := range []struct {
		model, extra, format string
		files, size, status  int
	}{
		{"", "", "", 1, 3, 400}, {"gpt-4o-transcribe", "", "", 1, 3, 400}, {"openai/gpt-4.1", "", "", 1, 3, 400}, {"openai/GPT-4O-TRANSCRIBE", "", "", 1, 3, 400}, {"openai/gpt-4o-transcribe", "language", "", 1, 3, 400}, {"openai/gpt-4o-transcribe", "", "text", 1, 3, 400}, {"openai/gpt-4o-transcribe", "", "", 0, 3, 400}, {"openai/gpt-4o-transcribe", "", "", 2, 3, 400}, {"openai/gpt-4o-transcribe", "", "", 1, 0, 413}, {"openai/gpt-4o-transcribe", "", "", 1, 17, 413}, {"openai/gpt-4o-transcribe", "", "", 1, 2 << 20, 413},
	} {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		if test.model != "" {
			form.WriteField("model", test.model)
		}
		if test.extra != "" {
			form.WriteField(test.extra, "unsupported")
		}
		if test.format != "" {
			form.WriteField("response_format", test.format)
		}
		for i := 0; i < test.files; i++ {
			file, _ := form.CreateFormFile("file", "audio.wav")
			io.Copy(file, strings.NewReader(strings.Repeat("x", test.size)))
		}
		form.Close()
		req, _ := http.NewRequest("POST", server.URL+"/v1/audio/transcriptions", &body)
		req.Header.Set("Authorization", "Bearer "+TestSecret)
		req.Header.Set("Content-Type", form.FormDataContentType())
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != test.status {
			t.Fatalf("case=%+v status=%d body=%s", test, response.StatusCode, data)
		}
	}
	for _, test := range []struct {
		method, path, content, body string
		status                      int
	}{{"POST", "/v1/audio/transcriptions", "text/plain", "hi", 415}, {"POST", "/v1/audio/transcriptions?language=en", "multipart/form-data", "", 400}, {"POST", "/v1/audio/transcriptions", "multipart/form-data; boundary=bad", "bad", 400}, {"POST", "/v1/responses", "application/json", strings.Repeat("x", 129), 413}, {"PUT", "/v1/models", "", "", 405}, {"GET", "/missing", "", "", 404}} {
		req, _ := http.NewRequest(test.method, server.URL+test.path, strings.NewReader(test.body))
		req.Header.Set("Authorization", "Bearer "+TestSecret)
		req.Header.Set("Content-Type", test.content)
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != test.status {
			t.Fatalf("status=%d body=%s", response.StatusCode, data)
		}
	}
}

func TestClientProtocolsCatalogMetadata(t *testing.T) {
	for _, kind := range []string{"created", "tools"} {
		schema := testfixtures.ProviderCatalog(t).Schema()
		schema.Providers[0].Offerings[0].Created = 0
		if kind == "tools" {
			schema = testfixtures.ProviderCatalog(t).Schema()
			for p := range schema.Providers {
				if schema.Providers[p].ID == "anthropic" {
					schema.Providers[p].Offerings[0].CallerTools = true
				}
			}
		}
		if _, err := proxy.NewProviderCatalog(schema); err == nil {
			t.Fatalf("accepted invalid %s metadata", kind)
		}
	}
}

func TestClientProtocolsSynchronousResponsesAndNativeHistory(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request["max_output_tokens"] != float64(100) {
			t.Error("output limit missing")
		}
		w.Header().Set("Content-Type", "application/json")
		if request["text"] != nil {
			io.WriteString(w, `{"status":"completed","output_text":"{\"answer\":\"yes\"}"}`)
			return
		}
		io.WriteString(w, `{"status":"completed","output_text":"history complete"}`)
	}))
	defer upstream.Close()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for p := range schema.Providers {
		if schema.Providers[p].ID == "xai" {
			for o := range schema.Providers[p].Offerings {
				if schema.Providers[p].Offerings[o].Model == "grok-4.5" {
					schema.Providers[p].Offerings[o].CallerTools = true
					schema.Providers[p].Offerings[o].RequestProfile = "openai_responses_temperature_tools"
				}
			}
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameXAI: upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, test := range []struct{ path, body string }{
		{"/v1/responses", `{"model":"xai/grok-4.5","max_output_tokens":100,"input":[{"role":"developer","content":"Read twice"},{"role":"user","content":"read"},{"role":"assistant","content":[{"type":"output_text","text":"I will read."}]},{"type":"function_call","call_id":"a","name":"read","arguments":"{}"},{"type":"function_call","call_id":"b","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"a","output":"one"},{"type":"function_call_output","call_id":"b","output":"two"}],"tools":[{"type":"function","name":"read","parameters":{"type":"object","additionalProperties":false},"strict":true}],"parallel_tool_calls":true}`},
		{"/v1/responses", `{"model":"xai/grok-4.5","max_output_tokens":100,"input":"hi","text":{"format":{"type":"json_schema","name":"answer","schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}}}}`},
		{"/v2?key=" + TestSecret + "&provider=xai&format=application/json", `{"model":"grok-4.5","max_tokens":100,"messages":[{"role":"user","content":"read"},{"role":"assistant","tool_calls":[{"id":"a","name":"read","arguments":"{}"}]},{"role":"tool","tool_call_id":"a","content":"done"}]}`},
	} {
		req, _ := http.NewRequest("POST", server.URL+test.path, strings.NewReader(test.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+TestSecret)
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("%s status=%d body=%s", test.path, res.StatusCode, body)
		}
	}
}

func TestClientProtocolsLostMultipartFile(t *testing.T) {
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	form.WriteField("model", "openai/gpt-4o-transcribe")
	file, _ := form.CreateFormFile("file", "audio.wav")
	io.WriteString(file, "fixture audio")
	form.Close()
	req := httptest.NewRequest("POST", "/v1/audio/transcriptions", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+TestSecret)
	if err := req.ParseMultipartForm(0); err != nil {
		t.Fatal(err)
	}
	if err := req.MultipartForm.RemoveAll(); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != 400 {
		t.Fatalf("lost upload status=%d body=%s", response.Code, response.Body)
	}
}
