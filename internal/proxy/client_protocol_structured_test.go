package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestClientProtocolsStructuredOutput(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["text"] == nil {
			t.Error("structured control missing")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output_text":"{\"answer\":\"yes\"}"}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameOpenAI: upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	schema := map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]string{"type": "string"}}, "required": []string{"answer"}, "additionalProperties": false}
	for _, path := range []string{"/v1/chat/completions", "/v1/responses"} {
		for _, stream := range []bool{false, true} {
			payload := map[string]any{"model": "openai/gpt-5.6", "stream": stream}
			if path == "/v1/chat/completions" {
				payload["messages"] = []any{map[string]any{"role": "developer", "content": "Return JSON"}, map[string]any{"role": "user", "content": []any{map[string]string{"type": "text", "text": "Answer yes"}}}}
				payload["max_tokens"] = 100
				payload["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "answer", "schema": schema, "strict": true}}
			} else {
				payload["input"] = []any{map[string]any{"role": "user", "content": []any{map[string]string{"type": "input_text", "text": "Answer yes"}}}}
				payload["reasoning"] = map[string]string{"effort": "low"}
				payload["instructions"] = "Return JSON"
				payload["max_output_tokens"] = 100
				payload["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": "answer", "schema": schema, "strict": true}}
			}
			data, _ := json.Marshal(payload)
			req, _ := http.NewRequest("POST", server.URL+path, bytes.NewReader(data))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+TestSecret)
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 200 || !bytes.Contains(body, []byte("answer")) {
				t.Fatalf("structured: %d %s", response.StatusCode, body)
			}
		}
	}
}
