package proxy_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

const astraModel = "gpt-6-astra"

func astraCatalog(t *testing.T) *proxy.ProviderCatalog {
	t.Helper()
	return testfixtures.ProviderCatalog(t)
}

func TestAstraHTTP(t *testing.T) {
	expectedEffort := ""
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if r.Method != "POST" || r.URL.Path != "/responses" || payload["model"] != astraModel || payload["store"] != true || payload["background"] != true || payload["previous_response_id"] != nil || payload["max_output_tokens"] != float64(4096) {
			t.Errorf("Astra payload=%v", payload)
		}
		if expectedEffort == "" {
			if payload["reasoning"] != nil {
				t.Error("omitted effort must remain omitted")
			}
		} else {
			reasoning, _ := payload["reasoning"].(map[string]any)
			if reasoning["effort"] != expectedEffort {
				t.Errorf("Astra reasoning=%v", reasoning)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"private"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"visible"}]}],"usage":{"input_tokens":2,"output_tokens":7,"total_tokens":9,"output_tokens_details":{"reasoning_tokens":5}}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: astraCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"openai": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max"} {
		expectedEffort = effort
		requestURL := server.URL + "/?key=" + TestSecret + "&provider=openai&model=" + astraModel + "&prompt=test&max_tokens=4096"
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
	for _, effort := range []string{"none", "minimal"} {
		response, err := http.Get(server.URL + "/?key=" + TestSecret + "&provider=openai&model=" + astraModel + "&prompt=test&reasoning_effort=" + effort)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 400 || calls != 6 {
			t.Fatalf("invalid effort=%s status=%d calls=%d", effort, response.StatusCode, calls)
		}
	}
	response, err := http.Get(server.URL + "/?key=" + TestSecret + "&provider=openai&model=" + astraModel + "&prompt=test&max_tokens=128001")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 400 || calls != 6 {
		t.Fatalf("output bound status=%d calls=%d", response.StatusCode, calls)
	}

}

func TestAstraCatalog(t *testing.T) {
	public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: astraCatalog(t)})
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(public.Offerings, func(offering proxy.PublicProviderOffering) bool { return offering.Model == astraModel })
	if index < 0 {
		t.Fatal("missing Astra offering")
	}
	offering := public.Offerings[index]
	if !reflect.DeepEqual(offering.ReasoningEfforts, []string{"low", "medium", "high", "xhigh", "max"}) || !slices.Contains(offering.Capabilities, "image_input") {
		t.Fatalf("offering=%+v", offering)
	}
	if len(offering.Limits) != 3 || offering.Limits[0].Value == nil || *offering.Limits[0].Value != 1050000 {
		t.Fatalf("Astra limits=%v", offering.Limits)
	}
	priceIndex := slices.IndexFunc(public.Prices, func(price proxy.CatalogPriceDescriptor) bool { return price.Model == astraModel })
	if priceIndex < 0 {
		t.Fatal("missing Astra price")
	}
	rates := public.Prices[priceIndex].Rates
	if len(rates) != 8 {
		t.Fatalf("rates=%v", rates)
	}
	for index, expected := range []float64{10, 50, 1, 12.5, 20, 75, 2, 25} {
		if rates[index].Rate != expected || rates[index].Conditions.BillingMode != "standard" {
			t.Fatalf("rate=%v", rates[index])
		}
	}
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", proxy.PublicCapabilitiesPath, nil))
	if !strings.Contains(response.Body.String(), astraModel) {
		t.Fatal("qualified Astra is absent from public discovery")
	}
}

func TestAstraMediaStructuredOutput(t *testing.T) {
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
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: astraCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"openai": upstream.URL}, nil), AssetStorePath: t.TempDir()}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	attachments := []any{}
	for _, mime := range []string{"image/jpeg", "image/png"} {
		attachments = append(attachments, map[string]string{"type": "image", "mime_type": mime, "data": base64.StdEncoding.EncodeToString([]byte(mime))})
	}
	body, _ := json.Marshal(map[string]any{"model": astraModel, "reasoning_effort": "xhigh", "messages": []any{map[string]any{"role": "user", "content": "inspect", "attachments": attachments}}, "structured_output": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]string{"type": "string"}}, "required": []string{"answer"}, "additionalProperties": false}}})
	response := performStructuredRequestForProvider(t, router, string(body), "astra-media", "openai")
	if response.Code != 200 || response.Body.String() != `{"answer":"yes"}` || calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
}

func TestAstraFunctionCalls(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["model"] != astraModel || payload["store"] != true || payload["reasoning"].(map[string]any)["effort"] != "high" {
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
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: astraCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"openai": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	for index, input := range []string{`"read"`, `[{"role":"user","content":"read"},{"type":"function_call","call_id":"read_1","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"read_1","output":"file contents"}]`} {
		request := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"openai/gpt-6-astra","reasoning":{"effort":"high"},"input":`+input+`,"tools":[{"type":"function","name":"read","parameters":{"type":"object","additionalProperties":false},"strict":true}]}`))
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

func TestAstraLive(t *testing.T) {
	if os.Getenv("LLM_PROXY_LIVE_ASTRA") != "true" {
		t.Skip("explicit live acceptance required")
	}
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Fatal("OPENAI_API_KEY is required")
	}
	tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenant.ProviderKeys["openai"] = key
	router, err := buildRouterWithManagedTenant(t, proxy.Configuration{ProviderCatalog: astraCatalog(t), RequestTimeoutSeconds: 60}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal("build Astra qualification proxy failed")
	}
	server := httptest.NewServer(router)
	defer server.Close()
	client := &http.Client{Timeout: 65 * time.Second}
	for _, scenario := range []string{"structured", "function"} {
		payload := `{"model":"openai/gpt-6-astra","reasoning":{"effort":"low"},"max_output_tokens":4096,"input":"Return the answer OK.",`
		if scenario == "structured" {
			payload += `"text":{"format":{"type":"json_schema","name":"answer","schema":{"type":"object","properties":{"answer":{"type":"string","enum":["OK"]}},"required":["answer"],"additionalProperties":false}}}}`
		} else {
			payload += `"tools":[{"type":"function","name":"answer","parameters":{"type":"object","properties":{"answer":{"type":"string","enum":["OK"]}},"required":["answer"],"additionalProperties":false},"strict":true}],"tool_choice":{"type":"function","name":"answer"}}`
		}
		request, err := http.NewRequest("POST", server.URL+"/v1/responses", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+TestSecret)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "astra-live-"+scenario)
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("Astra %s transport failed", scenario)
		}
		var result struct {
			Output []struct {
				Type      string
				Name      string
				Arguments string
				Content   []struct{ Text string }
			}
		}
		decodeError := json.NewDecoder(response.Body).Decode(&result)
		response.Body.Close()
		if response.StatusCode != 200 || decodeError != nil {
			t.Fatalf("Astra %s status=%d invalid_response=%t", scenario, response.StatusCode, decodeError != nil)
		}
		accepted := false
		for _, output := range result.Output {
			var answer struct{ Answer string }
			if scenario == "function" && output.Type == "function_call" && output.Name == "answer" {
				accepted = json.Unmarshal([]byte(output.Arguments), &answer) == nil && answer.Answer == "OK"
			}
			for _, content := range output.Content {
				if scenario == "structured" {
					accepted = json.Unmarshal([]byte(content.Text), &answer) == nil && answer.Answer == "OK"
				}
			}
		}
		if !accepted {
			t.Fatalf("Astra %s did not return the expected typed result", scenario)
		}
		t.Logf("Astra live %s passed", scenario)
	}
	response, err := client.Get(server.URL + "/?key=" + TestSecret + "&provider=openai&model=gpt-6-astra&reasoning_effort=low&max_tokens=4096&web_search=true&prompt=Find+the+official+OpenAI+API+documentation+homepage+and+return+its+URL.")
	if err != nil {
		t.Fatal("Astra web search transport failed")
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || !strings.Contains(string(body), "openai.com") {
		t.Fatalf("Astra web search status=%d", response.StatusCode)
	}
	t.Log("Astra live web search passed")
}
