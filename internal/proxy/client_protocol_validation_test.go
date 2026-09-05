package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestClientProtocolsRejectBeforeDispatch(t *testing.T) {
	contract, err := openapitest.Load("../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output_text":"unexpected dispatch"}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameOpenAI: upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	user := []any{map[string]any{"role": "user", "content": "hello"}}
	tool := map[string]any{"type": "function", "function": map[string]any{"name": "read", "parameters": map[string]any{"type": "object"}}}
	tests := []struct {
		name, path, body, key, contentType, timeout string
		status                                      int
	}{
		{"missing bearer", "/v1/models", "", "", "", "", 401},
		{"model body", "/v1/models", `{ "key": "ignored" }`, "Bearer " + TestSecret, "application/json", "", 400},
		{"wrong bearer", "/v1/models", "", "Bearer invalid", "", "", 401},
		{"wrong scheme", "/v1/models", "", "Basic value", "", "", 401},
		{"padded bearer", "/v1/models", "", "Bearer  " + TestSecret, "", "", 401},
		{"URL key", "/v1/models?key=" + TestSecret, "", "Bearer " + TestSecret, "", "", 400},
		{"invalid body", "/v1/responses", "{", "Bearer " + TestSecret, "application/json", "", 400},
		{"wrong MIME", "/v1/responses", "{}", "Bearer " + TestSecret, "text/plain", "", 415},
		{"query controls", "/v1/responses?model=openai/gpt-5.6", "{}", "Bearer " + TestSecret, "application/json", "", 400},
		{"invalid timeout", "/v1/responses", "{}", "Bearer " + TestSecret, "application/json", "0", 400},
	}
	add := func(name, path string, payload any) {
		encoded, _ := json.Marshal(payload)
		tests = append(tests, struct {
			name, path, body, key, contentType, timeout string
			status                                      int
		}{name, path, string(encoded), "Bearer " + TestSecret, "application/json", "", 400})
	}
	for _, model := range []string{"", "gpt-5.6", "OpenAI/gpt-5.6", "openai/GPT-5.6", "openai/unknown", "unknown/model", "/gpt-5.6", "openai/", " openai/gpt-5.6"} {
		add("route "+model, "/v1/chat/completions", map[string]any{"model": model, "messages": user})
	}
	for _, mutation := range []map[string]any{
		{"temperature": 0.5}, {"key": "forbidden"}, {"n": 2}, {"store": true}, {"messages": nil}, {"messages": []any{}}, {"messages": []any{map[string]any{"role": "assistant", "content": "hello"}}}, {"messages": []any{map[string]any{"role": "user", "content": true}}}, {"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "image_url", "image_url": "no"}}}}}, {"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "output_text", "text": "wrong"}}}}},
		{"max_tokens": 0}, {"max_completion_tokens": -1}, {"max_tokens": 1, "max_completion_tokens": 1}, {"reasoning_effort": "unsupported"}, {"reasoning_effort": nil},
		{"stream_options": map[string]any{"include_usage": true}},
		{"tool_choice": "required"}, {"parallel_tool_calls": true}, {"tools": []any{tool}, "tool_choice": "invalid"}, {"tools": []any{tool}, "tool_choice": map[string]any{"type": "function", "function": map[string]any{"name": "other"}}}, {"tools": []any{map[string]any{"type": "web_search"}}},
		{"tools": []any{tool, tool}}, {"tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "bad name", "parameters": map[string]any{"type": "object"}}}}},
		{"tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "read", "parameters": map[string]any{"type": "array"}}}}},
		{"tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "read", "parameters": map[string]any{"type": "invalid"}}}}},
		{"response_format": map[string]any{"type": "json_object"}}, {"response_format": map[string]any{"type": "json_schema"}}, {"response_format": map[string]any{"type": "text", "json_schema": map[string]any{}}},
		{"messages": []any{map[string]any{"role": "user", "content": "hello", "refusal": "no"}}},
	} {
		payload := map[string]any{"model": "openai/gpt-5.6", "messages": user}
		for k, v := range mutation {
			payload[k] = v
		}
		add("chat controls", "/v1/chat/completions", payload)
	}
	for _, mutation := range []map[string]any{
		{"previous_response_id": "private"}, {"store": true}, {"include": []string{"reasoning.encrypted_content"}}, {"prompt_cache_key": "cache"}, {"reasoning": map[string]any{"summary": "auto"}}, {"text": map[string]any{"verbosity": "low"}}, {"text": map[string]any{"format": map[string]any{"type": "json_object"}}},
		{"text": map[string]any{"format": map[string]any{"type": "json_schema"}}}, {"text": map[string]any{"format": map[string]any{"type": "text", "schema": map[string]any{}}}},
		{"input": 17}, {"input": []any{map[string]any{"type": "item_reference", "id": "remote"}}}, {"input": []any{map[string]any{"role": "tool", "content": "bad"}}}, {"input": []any{map[string]any{"role": "user", "content": "hi", "call_id": "bad"}}},
		{"input": []any{map[string]any{"type": "function_call", "role": "user"}}}, {"input": []any{map[string]any{"type": "function_call_output", "call_id": "call"}}},
		{"input": []any{map[string]any{"role": "user", "content": "hello"}, map[string]any{"type": "function_call_output", "call_id": "unmatched", "output": "hi"}}},
		{"tools": []any{map[string]any{"type": "web_search"}}}, {"tool_choice": map[string]any{"type": "file_search"}},
	} {
		payload := map[string]any{"model": "openai/gpt-5.6", "input": "hello"}
		for k, v := range mutation {
			payload[k] = v
		}
		add("response controls", "/v1/responses", payload)
	}
	for index, test := range tests {
		t.Run(test.name+strconv.Itoa(index), func(t *testing.T) {
			method := "POST"
			if strings.HasPrefix(test.path, "/v1/models") {
				method = "GET"
			}
			request, _ := http.NewRequest(method, server.URL+test.path, strings.NewReader(test.body))
			request.Header.Set("Authorization", test.key)
			request.Header.Set("Content-Type", test.contentType)
			if test.timeout != "" {
				request.Header.Set("X-LLM-Proxy-Request-Timeout-Seconds", test.timeout)
			}
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != test.status {
				t.Fatalf("status=%d body=%s", response.StatusCode, data)
			}
			path, _, _ := strings.Cut(test.path, "?")
			if err := contract.ValidateResponse(path, method, response.StatusCode, response.Header, data); err != nil {
				t.Fatal(err)
			}
			var body map[string]json.RawMessage
			if json.Unmarshal(data, &body) != nil || !bytes.Contains(body["error"], []byte("request_id")) {
				t.Fatalf("error envelope=%s", data)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("rejected requests dispatched %d provider calls", calls.Load())
	}
}

func TestClientProtocolsHistoryAndSchemaRejections(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid input reached provider")
		w.WriteHeader(500)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"openai": upstream.URL, "gemini": upstream.URL, "deepseek": upstream.URL, "minimax": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, test := range []struct{ path, body string }{
		{"/v1/responses", `{} {}`},
		{"/v1/chat/completions", `{"model":"openai/gpt-5.6","messages":[{"role":"USER","content":"hi"}]}`},
		{"/v1/chat/completions", `{"model":"openai/gpt-5.6","messages":[{"role":"user","content":"hi"}],"tool_choice":[]}`},
		{"/v1/chat/completions", `{"model":"openai/gpt-5.6","messages":[{"role":"user","content":"hi"}],"tool_choice":{"type":"web_search"}}`},
		{"/v1/responses", `{"model":"openai/gpt-5.6","input":"hi","tool_choice":[]}`},
		{"/v1/chat/completions", `{"model":"openai/gpt-5.6","messages":[{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"type":"custom","id":"a","function":{"name":"read","arguments":"{}"}}]}]}`},
		{"/v1/responses", `{"model":"openai/gpt-5.6","input":[{"role":"user","content":[{"type":"image_url"}]}]}`},
		{"/v1/responses", `{"model":"minimax/minimax-m2.7-highspeed","input":"hi","max_output_tokens":999999999}`},
		{"/v1/responses", `{"model":"gemini/gemini-3.5-flash","input":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]}`},
		{"/v1/responses", `{"model":"openai/gpt-5.6","input":"hi","text":{"format":{"type":"json_schema","name":"answer","schema":{"type":"invalid"}}}}`},
		{"/v1/responses", `{"model":"openai/gpt-5.6","input":"hi","text":{"format":{"type":"json_schema","name":"answer","schema":{"type":"object"}}}}`},
		{"/v1/chat/completions", `{"model":"openai/gpt-5.6","messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_schema","json_schema":{"name":"answer"}}}`},
		{"/v1/responses", `{"model":"deepseek/deepseek-chat","input":"hi","tools":[{"type":"function","name":"read","parameters":{"type":"object"}}]}`},
		{"/v1/responses", `{"model":"openai/gpt-5.6","input":"hi","tools":[{"type":"function","name":"read","parameters":{"type":"object"},"strict":true}]}`},
		{"/v1/responses", `{"model":"openai/gpt-5.6","input":"hi","tools":[{"type":"function","name":"read","parameters":{"type":"object"}}],"text":{"format":{"type":"json_schema","name":"answer","schema":{"type":"object"}}}}`},
		{"/v2?key=" + TestSecret, `{"model":"gpt-5.6","messages":[{"role":"unknown","content":"hi"}]}`},
		{"/v2?key=" + TestSecret, `{"model":"gpt-5.6","messages":[{"role":"user","content":"hi"}],"tools":[{"name":"read","parameters":{"type":"object"}}],"tool_choice":{"mode":"auto","name":"read"}}`},
		{"/v2?key=" + TestSecret, `{"model":"gpt-5.6","messages":[{"role":"user","content":"hi","tool_calls":[{"id":"a","name":"read","arguments":"{}"}]}]}`},
		{"/v2?key=" + TestSecret, `{"model":"gpt-5.6","messages":[{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"id":"a","name":"read","arguments":"{}"}]}]}`},
		{"/v2?key=" + TestSecret, `{"model":"gpt-5.6","messages":[{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"id":"a","name":"read","arguments":"{}"}]},{"role":"user","content":"next"}]}`},
		{"/v2?key=" + TestSecret, `{"model":"gpt-5.6","messages":[{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"id":"a","name":"read","arguments":"{}"}]},{"role":"tool","tool_call_id":"a","content":"done"},{"role":"assistant","tool_calls":[{"id":"a","name":"read","arguments":"{}"}]},{"role":"tool","tool_call_id":"a","content":"done"}]}`},
	} {
		req, _ := http.NewRequest("POST", server.URL+test.path, strings.NewReader(test.body))
		req.Header.Set("Authorization", "Bearer "+TestSecret)
		req.Header.Set("Content-Type", "application/json")
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Fatalf("%s %s: status=%d body=%s", test.path, test.body, res.StatusCode, body)
		}
	}
}
