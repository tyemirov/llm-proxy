package proxy_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func TestXAIResponsesRoutes(t *testing.T) {
	calls := 0
	var upstreamModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/responses" {
			t.Errorf("unexpected upstream %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["store"] != false || body["max_output_tokens"] != float64(32) {
			t.Errorf("storage/limit payload=%v", body)
		}
		for _, field := range []string{"background", "previous_response_id", "messages", "max_tokens", "include"} {
			if _, present := body[field]; present {
				t.Errorf("unexpected field=%s", field)
			}
		}
		upstreamModel, _ = body["model"].(string)
		input, ok := body["input"].([]any)
		if !ok || len(input) != 3 {
			t.Errorf("input=%v", body["input"])
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output":[{"type":"reasoning","encrypted_content":"private","summary":[{"type":"summary_text","text":"private"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"visible"}]}],"usage":{"input_tokens":2,"output_tokens":7,"total_tokens":9,"output_tokens_details":{"reasoning_tokens":5}}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"xai": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	models := []string{"grok-4.3", "grok-4.3-latest", "grok-4.5", "grok-4.20-0309-reasoning", "grok-4.20-0309-non-reasoning", "grok-latest", "grok-build-0.1", "grok-code-fast", "grok-code-fast-1", "grok-code-fast-1-0825"}
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			body := fmt.Sprintf(`{"model":%q,"max_tokens":32,"messages":[{"role":"system","content":"Be concise"},{"role":"assistant","content":"Prior answer"},{"role":"user","content":"Next"}]}`, model)
			response, err := http.Post(server.URL+"/v2?provider=xai&key="+TestSecret, "application/json", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			data, _ := io.ReadAll(response.Body)
			if response.StatusCode != 200 || string(data) != "visible" {
				t.Fatalf("status=%d body=%s", response.StatusCode, data)
			}
			if upstreamModel != model {
				t.Fatalf("upstream model=%s want=%s", upstreamModel, model)
			}
			if response.Header.Get("X-LLM-Proxy-Total-Tokens") != "9" {
				t.Fatalf("usage headers=%v", response.Header)
			}
		})
	}
	if calls != len(models) {
		t.Fatalf("upstream calls=%d", calls)
	}
	for _, offering := range testfixtures.ProviderCatalog(t).ModelCatalog().Offerings {
		if offering.Provider == "xai" && len(offering.Operations) == 1 && offering.Operations[0] == "text" && offering.WireContract != "xai_responses" {
			t.Errorf("model=%s protocol=%s", offering.Model, offering.WireContract)
		}
	}
}

func TestXAIResponsesRejectNoncanonicalResults(t *testing.T) {
	for _, raw := range []string{
		`{"status":"completed","output_text":"private top-level text"}`,
		`{"status":"completed","output":[{"type":"message","role":"user","content":[{"type":"output_text","text":"private user text"}]}]}`,
		`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"private refusal"}]}]}`,
		`{"status":"in_progress","output":[]}`,
		`{"status":"incomplete","incomplete_details":{"reason":"content_filter"}}`,
		`{"status":"failed","error":{"message":"private error"}}`,
		`{"status":"completed","error":{"message":"private error"},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"visible"}]}]}`,
		`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"visible"}]}],"usage":{"input_tokens":-1,"output_tokens":2,"total_tokens":1}}`,
		`{"status":"completed","output":[{"type":"function_call","call_id":"bad","name":"bad name","arguments":"{}"}]}`,
		`{"status":"completed","output":[{"type":"unknown"}]}`,
		`{`,
	} {
		t.Run(raw, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, raw)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"xai": upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", "/?key="+TestSecret+"&provider=xai&model=grok-4.5&prompt=test", nil))
			if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), "private") || calls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
			}
		})
	}
}

func TestXAIResponsesContinuation(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/responses" {
			t.Errorf("unexpected upstream %s %s", r.Method, r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["store"] != false || payload["background"] != nil || payload["previous_response_id"] != nil {
			t.Errorf("stateful payload=%v", payload)
		}
		if payload["model"] != "grok-4.3" {
			t.Errorf("default model=%v", payload["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			io.WriteString(w, `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"first "}]}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`)
			return
		}
		input := payload["input"].([]any)
		if input[len(input)-2].(map[string]any)["content"] != "first " {
			t.Errorf("continuation input=%v", input)
		}
		io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"second"}]}],"usage":{"input_tokens":5,"output_tokens":4,"total_tokens":9}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"xai": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/?key="+TestSecret+"&provider=xai&prompt=test", nil))
	if response.Code != 200 || response.Body.String() != "first second" || calls != 2 || response.Header().Get("X-LLM-Proxy-Total-Tokens") != "14" {
		t.Fatalf("status=%d calls=%d body=%s headers=%v", response.Code, calls, response.Body.String(), response.Header())
	}
}

func TestXAIResponsesHTTPErrorMetadata(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"message":"private provider failure"}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"xai": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/?key="+TestSecret+"&provider=xai&prompt=test", nil))
	if response.Code != 429 || !strings.Contains(response.Body.String(), `"upstream_status":429`) || strings.Contains(response.Body.String(), "private") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestXAIResponsesFunctionCalls(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output":[{"type":"function_call","call_id":"read_1","name":"read","arguments":"{}"}]}`)
	}))
	defer upstream.Close()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for p := range schema.Providers {
		if schema.Providers[p].ID == "xai" {
			for o := range schema.Providers[p].Offerings {
				if schema.Providers[p].Offerings[o].Model == "grok-4.5" {
					schema.Providers[p].Offerings[o].CallerTools = true
				}
			}
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpointOverrides(map[string]string{"xai": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"xai/grok-4.5","input":"read","tools":[{"type":"function","name":"read","parameters":{"type":"object","additionalProperties":false},"strict":true}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+TestSecret)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"call_id":"read_1"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
