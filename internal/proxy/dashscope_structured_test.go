package proxy_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

const dashScopeFlashStructuredModel = "qwen3.8-flash"

func dashScopeStructuredBody(t *testing.T, model string, attachments []map[string]any) string {
	t.Helper()
	message := map[string]any{"role": "user", "content": "Return JSON with decision pass."}
	if attachments != nil {
		message["attachments"] = attachments
	}
	encoded, err := json.Marshal(map[string]any{
		"model": model, "reasoning_effort": "none", "max_tokens": 4096,
		"messages": []any{message}, "structured_output": map[string]any{"schema": json.RawMessage(structuredDecisionSchema)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func dashScopeStructuredRouter(t *testing.T, upstream string) http.Handler {
	t.Helper()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:      providerEndpointOverrides(map[string]string{proxy.ProviderNameDashScope: upstream}, nil),
		AssetStorePath: t.TempDir(), AssetRetentionSeconds: 3600,
	}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestDashScopeResponsesStructuredOutputConverges(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		text, _ := payload["text"].(map[string]any)
		format, _ := text["format"].(map[string]any)
		reasoning, _ := payload["reasoning"].(map[string]any)
		if r.Method != http.MethodPost || r.URL.Path != "/responses" || payload["model"] != dashScopeFlashStructuredModel || payload["store"] != false || payload["background"] != nil || payload["previous_response_id"] != nil || payload["max_output_tokens"] != float64(4096) || reasoning["effort"] != "none" || format["type"] != "json_schema" || format["strict"] != true || format["name"] != "llm_proxy_structured_output" {
			t.Errorf("unexpected provider request %s %s payload=%v", r.Method, r.URL.Path, payload)
		}
		encodedSchema, _ := json.Marshal(format["schema"])
		if !jsonBodiesEqual(encodedSchema, []byte(structuredDecisionSchema)) {
			t.Errorf("schema changed: %s", encodedSchema)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"decision\":\"pass\"}"}]}],"usage":{"input_tokens":9,"output_tokens":5,"total_tokens":14}}`)
	}))
	defer upstream.Close()
	router := dashScopeStructuredRouter(t, upstream.URL)
	body := dashScopeStructuredBody(t, dashScopeFlashStructuredModel, nil)
	first := performStructuredRequestForProvider(t, router, body, "flash:success", proxy.ProviderNameDashScope)
	if first.Code != http.StatusOK || !jsonBodiesEqual(first.Body.Bytes(), []byte(`{"decision":"pass"}`)) || first.Header().Get(llmproxycontract.HeaderStructuredRequestState) != "succeeded" || first.Header().Get("X-LLM-Proxy-Total-Tokens") != "14" {
		t.Fatalf("status=%d body=%s headers=%v", first.Code, first.Body.String(), first.Header())
	}
	replay := performStructuredRequestForProvider(t, router, body, "flash:success", proxy.ProviderNameDashScope)
	if replay.Code != http.StatusOK || !jsonBodiesEqual(first.Body.Bytes(), replay.Body.Bytes()) || calls.Load() != 1 {
		t.Fatalf("replay status=%d body=%s calls=%d", replay.Code, replay.Body.String(), calls.Load())
	}
	reconcile := httptest.NewRequest(http.MethodGet, "/v2/requests?key="+TestSecret, nil)
	reconcile.Header.Set(llmproxycontract.HeaderIdempotencyKey, "flash:success")
	result := httptest.NewRecorder()
	router.ServeHTTP(result, reconcile)
	if result.Code != http.StatusOK || !jsonBodiesEqual(first.Body.Bytes(), result.Body.Bytes()) || calls.Load() != 1 {
		t.Fatalf("reconciliation status=%d body=%s calls=%d", result.Code, result.Body.String(), calls.Load())
	}
	public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: currentQwenCatalog(t)})
	if err != nil {
		t.Fatal(err)
	}
	for _, offering := range public.Offerings {
		if offering.Provider == proxy.ProviderNameDashScope && slices.Contains(offering.Capabilities, "structured_output") != (offering.Model == dashScopeFlashStructuredModel) {
			t.Errorf("unexpected DashScope structured capability for %s: %v", offering.Model, offering.Capabilities)
		}
	}
	for _, model := range public.Models {
		if model.Identifier == dashScopeFlashStructuredModel && !slices.Contains(model.Capabilities, proxy.PublicModelCapabilityStructuredOutput) {
			t.Errorf("Flash model capability union omits structured output: %v", model.Capabilities)
		}
	}
}

func TestDashScopeResponsesStructuredOutputRejectsInvalidResults(t *testing.T) {
	for _, scenario := range []struct{ name, status, output string }{
		{"not-json", "completed", "not JSON"},
		{"invalid-schema", "completed", `{"decision":"private-invalid"}`},
		{"incomplete-json", "incomplete", `{"decision":"pass"}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"status": scenario.status, "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": scenario.output}}}}, "usage": map[string]int{"input_tokens": 9, "output_tokens": 5, "total_tokens": 14}})
			}))
			defer upstream.Close()
			router := dashScopeStructuredRouter(t, upstream.URL)
			identity := "flash:failure:" + scenario.name
			response := performStructuredRequestForProvider(t, router, dashScopeStructuredBody(t, dashScopeFlashStructuredModel, nil), identity, proxy.ProviderNameDashScope)
			if response.Code != http.StatusBadGateway || calls.Load() != 1 || strings.Contains(response.Body.String(), "private-invalid") {
				t.Fatalf("status=%d body=%s calls=%d", response.Code, response.Body.String(), calls.Load())
			}
			statusRequest := httptest.NewRequest(http.MethodGet, "/v2/requests?key="+TestSecret, nil)
			statusRequest.Header.Set(llmproxycontract.HeaderIdempotencyKey, identity)
			status := httptest.NewRecorder()
			router.ServeHTTP(status, statusRequest)
			if status.Code != http.StatusBadGateway || status.Header().Get(llmproxycontract.HeaderStructuredRequestState) != "failed" || calls.Load() != 1 {
				t.Fatalf("failure status=%d body=%s calls=%d", status.Code, status.Body.String(), calls.Load())
			}
		})
	}
}

func TestDashScopeResponsesStructuredOutputRejectsMediaAndOtherModels(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	router := dashScopeStructuredRouter(t, upstream.URL)
	asset := uploadTestAsset(t, router, TestSecret, "image/png", []byte("image"))
	for index, attachments := range [][]map[string]any{
		{messageMediaPayload("image", "image/png", []byte("image"))},
		{{"type": "image", "mime_type": "image/png", "asset_id": asset.AssetID}},
		{{"type": "image", "mime_type": "image/png", "url": "https://example.test/image.png"}},
	} {
		response := performStructuredRequestForProvider(t, router, dashScopeStructuredBody(t, dashScopeFlashStructuredModel, attachments), fmt.Sprintf("flash:media:%d", index), proxy.ProviderNameDashScope)
		if response.Code != http.StatusBadRequest || calls.Load() != 0 {
			t.Fatalf("media=%d status=%d body=%s calls=%d", index, response.Code, response.Body.String(), calls.Load())
		}
	}
	for _, model := range []string{"qwen-plus", "qwen3.7-plus"} {
		body := strings.Replace(dashScopeStructuredBody(t, model, nil), `,"reasoning_effort":"none"`, "", 1)
		response := performStructuredRequestForProvider(t, router, body, "flash:other:"+model, proxy.ProviderNameDashScope)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "structured_output is unsupported") || calls.Load() != 0 {
			t.Fatalf("model=%s status=%d body=%s calls=%d", model, response.Code, response.Body.String(), calls.Load())
		}
	}
}

func TestDashScopeResponsesStructuredOutputClientProtocols(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		text, _ := payload["text"].(map[string]any)
		format, _ := text["format"].(map[string]any)
		if format["type"] != "json_schema" || format["strict"] != true {
			t.Errorf("missing strict schema: %v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"decision\":\"pass\"}"}]}]}`)
	}))
	defer upstream.Close()
	router := dashScopeStructuredRouter(t, upstream.URL)
	for _, path := range []string{"/v1/chat/completions", "/v1/responses"} {
		for _, image := range []string{"", "data:image/png;base64,aW1hZ2U=", "https://example.test/image.png"} {
			payload := map[string]any{"model": "dashscope/" + dashScopeFlashStructuredModel}
			if path == "/v1/chat/completions" {
				content := []any{map[string]string{"type": "text", "text": "Return JSON decision pass."}}
				if image != "" {
					content = append(content, map[string]any{"type": "image_url", "image_url": map[string]string{"url": image}})
				}
				payload["messages"] = []any{map[string]any{"role": "user", "content": content}}
				payload["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "decision", "schema": json.RawMessage(structuredDecisionSchema), "strict": true}}
			} else {
				content := []any{map[string]string{"type": "input_text", "text": "Return JSON decision pass."}}
				if image != "" {
					content = append(content, map[string]string{"type": "input_image", "image_url": image})
				}
				payload["input"] = []any{map[string]any{"role": "user", "content": content}}
				payload["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": "decision", "schema": json.RawMessage(structuredDecisionSchema), "strict": true}}
			}
			body, _ := json.Marshal(payload)
			request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+TestSecret)
			response := httptest.NewRecorder()
			before := calls.Load()
			router.ServeHTTP(response, request)
			if image == "" {
				if response.Code != http.StatusOK || calls.Load() != before+1 || !strings.Contains(response.Body.String(), "decision") {
					t.Fatalf("%s status=%d body=%s calls=%d", path, response.Code, response.Body.String(), calls.Load())
				}
			} else if response.Code != http.StatusBadRequest || calls.Load() != before {
				t.Fatalf("%s image status=%d body=%s calls=%d", path, response.Code, response.Body.String(), calls.Load())
			}
		}
	}
}

func TestDashScopeResponsesStructuredOutputCatalogAdmission(t *testing.T) {
	for _, model := range []string{proxy.ModelNameGPT55, dashScopeFlashStructuredModel} {
		schema := testfixtures.ProviderCatalog(t).Schema()
		for providerIndex := range schema.Providers {
			for offeringIndex := range schema.Providers[providerIndex].Offerings {
				offering := &schema.Providers[providerIndex].Offerings[offeringIndex]
				if offering.Model == model {
					offering.StructuredOutput = model == proxy.ModelNameGPT55
				}
			}
		}
		catalog, err := proxy.NewProviderCatalog(schema)
		if model == proxy.ModelNameGPT55 {
			if err == nil || !strings.Contains(err.Error(), ".structured_output") {
				t.Fatalf("unqualified offering should reject structured declaration: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var calls atomic.Int64
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameDashScope: upstream.URL}, nil)}, zap.NewNop().Sugar())
		if err != nil {
			upstream.Close()
			t.Fatal(err)
		}
		response := performStructuredRequestForProvider(t, router, dashScopeStructuredBody(t, dashScopeFlashStructuredModel, nil), "flash:disabled-capability", proxy.ProviderNameDashScope)
		upstream.Close()
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "structured_output is unsupported") || calls.Load() != 0 {
			t.Fatalf("disabled declaration status=%d body=%s calls=%d", response.Code, response.Body.String(), calls.Load())
		}
	}
}

func TestDashScopeResponsesStructuredOutputRejectsUnsupportedSchemas(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	router := dashScopeStructuredRouter(t, upstream.URL)
	for index, schema := range []string{
		`{"type":"array","items":{"type":"string"},"uniqueItems":true}`,
		`{"type":"number","multipleOf":2}`,
		`{"oneOf":[{"type":"string"},{"type":"number"}]}`,
		`{"type":"string","format":"unsupported-format"}`,
		`{"type":"object","properties":{"name":true}}`,
		`{"$ref":"https://example.test/remote-schema"}`,
	} {
		body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"Return JSON"}],"structured_output":{"schema":%s}}`, dashScopeFlashStructuredModel, schema)
		response := performStructuredRequestForProvider(t, router, body, fmt.Sprintf("flash:unsupported-schema:%d", index), proxy.ProviderNameDashScope)
		if response.Code != http.StatusBadRequest || calls.Load() != 0 {
			t.Fatalf("schema=%s status=%d body=%s calls=%d", schema, response.Code, response.Body.String(), calls.Load())
		}
	}
}

func TestDashScopeResponsesStructuredOutputPreservesSupportedConstraints(t *testing.T) {
	const schema = `{"$defs":{"label":{"type":"string","enum":["pass"],"pattern":"^[a-z]+$","title":"Decision label","description":"A short label"}},"type":"object","additionalProperties":false,"properties":{"decision":{"$ref":"#/$defs/label"},"score":{"type":"number","minimum":0,"maximum":10,"exclusiveMinimum":0,"exclusiveMaximum":10},"values":{"type":"array","minItems":1,"maxItems":2,"items":{"const":3}},"email":{"type":"string","format":"email"},"date":{"type":"string","format":"date"},"optional":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["decision","score","values","email","date"]}`
	const output = `{"decision":"pass","score":3,"values":[3],"email":"fiction@example.test","date":"2026-11-13","optional":null}`
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		text, _ := payload["text"].(map[string]any)
		format, _ := text["format"].(map[string]any)
		encodedSchema, _ := json.Marshal(format["schema"])
		if !jsonBodiesEqual(encodedSchema, []byte(schema)) {
			t.Errorf("supported constraints changed: %s", encodedSchema)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": output}}}}})
	}))
	defer upstream.Close()
	router := dashScopeStructuredRouter(t, upstream.URL)
	body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"Return JSON"}],"structured_output":{"schema":%s}}`, dashScopeFlashStructuredModel, schema)
	response := performStructuredRequestForProvider(t, router, body, "flash:supported-schema", proxy.ProviderNameDashScope)
	if response.Code != http.StatusOK || !jsonBodiesEqual(response.Body.Bytes(), []byte(output)) || calls.Load() != 1 {
		t.Fatalf("supported schema status=%d body=%s calls=%d", response.Code, response.Body.String(), calls.Load())
	}
}
