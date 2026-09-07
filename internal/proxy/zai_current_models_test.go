package proxy_test

import (
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

var currentZAIModels = []string{"glm-5.3", "glm-5.3-flash"}

func currentZAICatalog(t *testing.T) *proxy.ProviderCatalog {
	t.Helper()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for index := range schema.Models {
		if slices.Contains(currentZAIModels, schema.Models[index].ID) {
			schema.Models[index].Enabled = proxy.ModelEnabled
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func zaiCurrentJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestZAICurrentHTTP(t *testing.T) {
	for _, model := range currentZAIModels {
		t.Run(model, func(t *testing.T) {
			expectedEffort := ""
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if r.Method != "POST" || r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer "+testZAIKey || payload["model"] != model || payload["max_tokens"] != float64(131072) {
					t.Errorf("Z.AI request=%s %s payload=%v", r.Method, r.URL.Path, payload)
				}
				for _, field := range []string{"max_completion_tokens", "max_output_tokens", "reasoning_split", "response_format", "background", "store"} {
					if payload[field] != nil {
						t.Errorf("unexpected field %s", field)
					}
				}
				if !reflect.DeepEqual(payload["thinking"], map[string]any{"type": "enabled"}) {
					t.Errorf("thinking=%v", payload["thinking"])
				}
				if expectedEffort == "" {
					if payload["reasoning_effort"] != nil {
						t.Error("omitted effort must remain omitted")
					}
				} else if payload["reasoning_effort"] != expectedEffort {
					t.Errorf("effort=%v want=%s", payload["reasoning_effort"], expectedEffort)
				}
				messages := payload["messages"].([]any)
				w.Header().Set("Content-Type", "application/json")
				if calls%2 == 1 {
					if len(messages) != 1 || messages[0].(map[string]any)["content"] != "test" {
						t.Errorf("initial messages=%v", messages)
					}
					io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"visible ","reasoning_content":"private reasoning"},"finish_reason":"length"}],"usage":{"prompt_tokens":2,"completion_tokens":7,"total_tokens":9}}`)
					return
				}
				if len(messages) != 3 || messages[1].(map[string]any)["content"] != "visible " || messages[1].(map[string]any)["reasoning_content"] != "private reasoning" {
					t.Errorf("continuation=%v", messages)
				}
				io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"answer","reasoning_content":"private final reasoning"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":8,"total_tokens":11,"prompt_tokens_details":{"cached_tokens":1}}}`)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentZAICatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"zai": upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			for _, method := range []string{"GET", "POST", "v2"} {
				for _, effort := range []string{"", "low", "high", "max"} {
					expectedEffort = effort
					requestURL := server.URL + "/?key=" + TestSecret + "&provider=zai"
					var request *http.Request
					if method == "GET" {
						requestURL += "&model=" + model + "&prompt=test&max_tokens=131072"
						if effort != "" {
							requestURL += "&reasoning_effort=" + effort
						}
						request, err = http.NewRequest("GET", requestURL, nil)
					} else {
						payload := map[string]any{"model": model, "max_tokens": 131072}
						if method == "v2" {
							requestURL = server.URL + "/v2?key=" + TestSecret + "&provider=zai"
							payload["messages"] = []any{map[string]string{"role": "user", "content": "test"}}
						} else {
							payload["prompt"] = "test"
						}
						if effort != "" {
							payload["reasoning_effort"] = effort
						}
						request, err = http.NewRequest("POST", requestURL, strings.NewReader(string(zaiCurrentJSON(t, payload))))
						if err == nil {
							request.Header.Set("Content-Type", "application/json")
						}
					}
					if err != nil {
						t.Fatal(err)
					}
					response, err := http.DefaultClient.Do(request)
					if err != nil {
						t.Fatal(err)
					}
					body, _ := io.ReadAll(response.Body)
					response.Body.Close()
					if response.StatusCode != 200 || string(body) != "visible answer" || response.Header.Get("X-LLM-Proxy-Total-Tokens") != "20" {
						t.Fatalf("method=%s effort=%s status=%d body=%s headers=%v", method, effort, response.StatusCode, body, response.Header)
					}
				}
			}
			for _, suffix := range []string{"&reasoning_effort=none", "&reasoning_effort=minimal", "&reasoning_effort=medium", "&reasoning_effort=xhigh", "&reasoning_effort=invalid", "&max_tokens=131073", "&max_tokens=0"} {
				response, err := http.Get(server.URL + "/?key=" + TestSecret + "&provider=zai&model=" + model + "&prompt=test" + suffix)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 400 || calls != 24 {
					t.Fatalf("suffix=%s status=%d calls=%d", suffix, response.StatusCode, calls)
				}
			}
		})
	}
}

func TestZAICurrentCatalog(t *testing.T) {
	catalog := currentZAICatalog(t)
	public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: catalog})
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range currentZAIModels {
		index := slices.IndexFunc(public.Offerings, func(o proxy.PublicProviderOffering) bool { return o.Model == model })
		if index < 0 {
			t.Fatalf("missing offering %s", model)
		}
		offering := public.Offerings[index]
		if !reflect.DeepEqual(offering.ReasoningEfforts, []string{"low", "high", "max"}) || slices.Contains(offering.Capabilities, "image_input") != (model == "glm-5.3-flash") {
			t.Fatalf("offering=%+v", offering)
		}
		if len(offering.Limits) != 1 || offering.Limits[0].Value == nil || *offering.Limits[0].Value != 1000000 {
			t.Fatalf("limits=%v", offering.Limits)
		}
		index = slices.IndexFunc(public.Prices, func(p proxy.CatalogPriceDescriptor) bool { return p.Model == model })
		if index < 0 {
			t.Fatalf("missing price %s", model)
		}
		price := public.Prices[index]
		expected := []float64{1.4, 4.4, 0.26}
		if model == "glm-5.3-flash" {
			expected = []float64{0.15, 0.5, 0.03, 0.075, 0.25, 0.015}
		}
		if !price.Available || price.Source != "https://docs.z.ai/guides/overview/pricing" || price.LastVerified != "2026-09-05" || len(price.Rates) != len(expected) {
			t.Fatalf("price=%+v", price)
		}
		for i, amount := range expected {
			rate := price.Rates[i]
			mode := "pay_as_you_go_list"
			if i >= 3 {
				mode = "promotion_until_2026-09-09T16:00:00Z_exclusive"
			}
			if rate.Rate != amount || rate.Currency != "USD" || rate.Unit != "USD/1M_tokens" || rate.Component != []string{"input_tokens", "output_tokens", "cache_read"}[i%3] || rate.Conditions.BillingMode != mode {
				t.Fatalf("rate=%+v", rate)
			}
		}
	}
	schema := catalog.Schema()
	for _, model := range schema.Models {
		if slices.Contains(currentZAIModels, model.ID) && (model.Publisher != "zai" || model.Family != "glm-5") {
			t.Fatalf("model=%+v", model)
		}
	}
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", proxy.PublicCapabilitiesPath, nil))
	if response.Code != 200 {
		t.Fatalf("catalog status=%d", response.Code)
	}
	for _, model := range currentZAIModels {
		if strings.Contains(response.Body.String(), model) {
			t.Fatalf("candidate %s in discovery", model)
		}
	}
}

func TestZAICurrentRejectUnsupportedInput(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Error("unsupported input reached Z.AI")
		w.WriteHeader(500)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentZAICatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"zai": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range currentZAIModels {
		structured := map[string]any{"model": model, "reasoning_effort": "max", "messages": []any{map[string]string{"role": "user", "content": "answer"}}, "structured_output": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]string{"type": "string"}}, "required": []string{"answer"}, "additionalProperties": false}}}
		response := performStructuredRequestForProvider(t, router, string(zaiCurrentJSON(t, structured)), model+"-structured", "zai")
		if response.Code != 400 || !strings.Contains(response.Body.String(), "structured_output is unsupported") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		for _, media := range []map[string]any{messageMediaPayload("audio", "audio/wav", []byte("audio"))} {
			body := mediaV2RequestBody(t, model, "test", []map[string]any{media})
			request := httptest.NewRequest("POST", "/v2?key="+TestSecret+"&provider=zai", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != 400 {
				t.Fatalf("model=%s status=%d body=%s", model, response.Code, response.Body.String())
			}
		}
	}
	if calls != 0 {
		t.Fatalf("calls=%d", calls)
	}
}
