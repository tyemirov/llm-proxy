package proxy_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

const currentGrokModel = "grok-4.6"

func currentGrokCatalog(t *testing.T) *proxy.ProviderCatalog {
	t.Helper()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for index := range schema.Models {
		if schema.Models[index].ID == currentGrokModel {
			schema.Models[index].Enabled = proxy.ModelEnabled
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestGrokCurrentHTTP(t *testing.T) {
	expectedEffort := ""
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if r.Method != "POST" || r.URL.Path != "/responses" || payload["model"] != currentGrokModel || payload["store"] != false || payload["background"] != nil || payload["previous_response_id"] != nil || payload["max_output_tokens"] != float64(4096) {
			t.Errorf("Grok payload=%v", payload)
		}
		if expectedEffort == "" {
			if payload["reasoning"] != nil {
				t.Error("omitted effort must remain omitted")
			}
		} else {
			reasoning, _ := payload["reasoning"].(map[string]any)
			if reasoning["effort"] != expectedEffort {
				t.Errorf("Grok reasoning=%v", reasoning)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"private"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"visible"}]}],"usage":{"input_tokens":2,"output_tokens":7,"total_tokens":9,"output_tokens_details":{"reasoning_tokens":5}}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentGrokCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"xai": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, effort := range []string{"", "low", "medium", "high", "xhigh"} {
		expectedEffort = effort
		requestURL := server.URL + "/?key=" + TestSecret + "&provider=xai&model=" + currentGrokModel + "&prompt=test&max_tokens=4096"
		if effort != "" {
			requestURL += "&reasoning_effort=" + effort
		}
		response, err := http.Get(requestURL)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 || string(body) != "visible" || response.Header.Get("X-LLM-Proxy-Total-Tokens") != "9" {
			t.Fatalf("effort=%s status=%d body=%s", effort, response.StatusCode, body)
		}
	}
	for _, effort := range []string{"none", "minimal", "max"} {
		response, err := http.Get(server.URL + "/?key=" + TestSecret + "&provider=xai&model=" + currentGrokModel + "&prompt=test&reasoning_effort=" + effort)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 400 || calls != 5 {
			t.Fatalf("invalid effort=%s status=%d calls=%d", effort, response.StatusCode, calls)
		}
	}
}

func TestGrokCurrentCatalog(t *testing.T) {
	public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: currentGrokCatalog(t)})
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(public.Offerings, func(offering proxy.PublicProviderOffering) bool { return offering.Model == currentGrokModel })
	if index < 0 {
		t.Fatal("missing Grok 4.6 offering")
	}
	offering := public.Offerings[index]
	if !reflect.DeepEqual(offering.ReasoningEfforts, []string{"low", "medium", "high", "xhigh"}) || !slices.Contains(offering.Capabilities, "image_input") {
		t.Fatalf("offering=%+v", offering)
	}
	if len(offering.Limits) != 1 || offering.Limits[0].Value == nil || *offering.Limits[0].Value != 500000 {
		t.Fatalf("context=%v", offering.Limits)
	}
	priceIndex := slices.IndexFunc(public.Prices, func(price proxy.CatalogPriceDescriptor) bool { return price.Model == currentGrokModel })
	if priceIndex < 0 {
		t.Fatal("missing Grok price")
	}
	rates := public.Prices[priceIndex].Rates
	if len(rates) != 6 {
		t.Fatalf("rates=%v", rates)
	}
	for index, expected := range []float64{2, 6, 0.5, 4, 12, 1} {
		if rates[index].Rate != expected || rates[index].Currency != "USD" || rates[index].Unit != "USD/1M_tokens" || rates[index].Conditions.BillingMode != "standard" || rates[index].Conditions.Mode != []string{"input_tokens_below_200000", "input_tokens_at_least_200000"}[index/3] {
			t.Fatalf("rate=%v", rates[index])
		}
	}
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", proxy.PublicCapabilitiesPath, nil))
	if strings.Contains(response.Body.String(), currentGrokModel) {
		t.Fatal("unqualified Grok is public")
	}
}

func TestGrokCurrentMediaStructuredOutput(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		content := payload["input"].([]any)[0].(map[string]any)["content"].([]any)
		for index, mime := range []string{"image/jpeg", "image/png"} {
			image := content[index].(map[string]any)
			if image["type"] != "input_image" || image["image_url"] != "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString([]byte(mime)) {
				t.Error("image order or bytes differ")
			}
		}
		if payload["text"].(map[string]any)["format"].(map[string]any)["type"] != "json_schema" || payload["reasoning"].(map[string]any)["effort"] != "xhigh" {
			t.Error("structured reasoning payload differs")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"answer\":\"yes\"}"}]}]}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentGrokCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"xai": upstream.URL}, nil), AssetStorePath: t.TempDir()}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	attachments := []any{}
	for _, mime := range []string{"image/jpeg", "image/png"} {
		attachments = append(attachments, map[string]string{"type": "image", "mime_type": mime, "data": base64.StdEncoding.EncodeToString([]byte(mime))})
	}
	body, _ := json.Marshal(map[string]any{"model": currentGrokModel, "reasoning_effort": "xhigh", "messages": []any{map[string]any{"role": "user", "content": "inspect", "attachments": attachments}}, "structured_output": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]string{"type": "string"}}, "required": []string{"answer"}, "additionalProperties": false}}})
	response := performStructuredRequestForProvider(t, router, string(body), "grok-current-media", "xai")
	if response.Code != 200 || response.Body.String() != `{"answer":"yes"}` || calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
}

func TestGrokCurrentFunctionCalls(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["model"] != currentGrokModel || payload["store"] != false || payload["reasoning"].(map[string]any)["effort"] != "high" {
			t.Error("tool request lost model, storage, or effort")
		}
		tools := payload["tools"].([]any)
		if tools[0].(map[string]any)["name"] != "read" {
			t.Error("tool declaration differs")
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			io.WriteString(w, `{"status":"completed","output":[{"type":"function_call","call_id":"read_1","name":"read","arguments":"{}"}]}`)
			return
		}
		input := payload["input"].([]any)
		last := input[len(input)-1].(map[string]any)
		if last["type"] != "function_call_output" || last["call_id"] != "read_1" || last["output"] != "file contents" {
			t.Errorf("tool output=%v", last)
		}
		io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"read complete"}]}]}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentGrokCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"xai": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	for index, input := range []string{`"read"`, `[{"role":"user","content":"read"},{"type":"function_call","call_id":"read_1","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"read_1","output":"file contents"}]`} {
		request := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"xai/grok-4.6","reasoning":{"effort":"high"},"input":`+input+`,"tools":[{"type":"function","name":"read","parameters":{"type":"object","additionalProperties":false},"strict":true}]}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+TestSecret)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		expected := []string{`"call_id":"read_1"`, `read complete`}[index]
		if response.Code != 200 || !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("tool turn=%d status=%d body=%s", index, response.Code, response.Body.String())
		}
	}
	if calls != 2 {
		t.Fatalf("tool calls=%d", calls)
	}
}

func TestGrokCurrentRejectMismatchedReasoningAdapter(t *testing.T) {
	schema := currentGrokCatalog(t).Schema()
	for p := range schema.Providers {
		if schema.Providers[p].ID == proxy.ProviderNameGemini {
			schema.Providers[p].Offerings[0].ReasoningEffort = &proxy.ReasoningEffortCapability{Adapter: "xai_responses", Efforts: []string{"high"}}
		}
	}
	_, err := proxy.NewProviderCatalog(schema)
	if err == nil || !strings.Contains(err.Error(), "adapter=xai_responses") {
		t.Fatalf("expected xAI reasoning route error, got %v", err)
	}
}
