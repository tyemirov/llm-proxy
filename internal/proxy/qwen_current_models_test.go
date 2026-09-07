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

var currentQwenModels = []string{"qwen3.8-max", "qwen3.8-max-0902", "qwen3.8-flash", "qwen3.8-2.4t-a95b", "qwen3.8-27b"}

func currentQwenCatalog(t *testing.T) *proxy.ProviderCatalog {
	t.Helper()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for index := range schema.Models {
		if slices.Contains(currentQwenModels, schema.Models[index].ID) {
			schema.Models[index].Enabled = proxy.ModelEnabled
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestQwenCurrentHTTP(t *testing.T) {
	for _, model := range currentQwenModels {
		t.Run(model, func(t *testing.T) {
			calls := 0
			expectedEffort := ""
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if r.Method != "POST" || r.URL.Path != "/responses" || payload["model"] != model || payload["store"] != false || payload["max_output_tokens"] != float64(131072) {
					t.Errorf("request=%s %s payload=%v", r.Method, r.URL.Path, payload)
				}
				for _, key := range []string{"enable_thinking", "reasoning_effort", "background", "previous_response_id", "conversation", "include", "tools", "text"} {
					if payload[key] != nil {
						t.Errorf("unexpected %s", key)
					}
				}
				if expectedEffort == "" {
					if payload["reasoning"] != nil {
						t.Error("omitted effort changed")
					}
				} else if reasoning, ok := payload["reasoning"].(map[string]any); !ok || reasoning["effort"] != expectedEffort {
					t.Errorf("reasoning=%v want=%s", payload["reasoning"], expectedEffort)
				}
				input := payload["input"].([]any)
				w.Header().Set("Content-Type", "application/json")
				if calls%2 == 1 {
					if len(input) != 1 || input[0].(map[string]any)["content"] != "test" {
						t.Errorf("input=%v", input)
					}
					io.WriteString(w, `{"status":"incomplete","output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"private"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"visible "}]}],"usage":{"input_tokens":2,"output_tokens":7,"total_tokens":9,"output_tokens_details":{"reasoning_tokens":5}}}`)
					return
				}
				if len(input) != 3 || input[1].(map[string]any)["content"] != "visible " {
					t.Errorf("continuation=%v", input)
				}
				encoded, _ := json.Marshal(input)
				if strings.Contains(string(encoded), "private") {
					t.Error("private reasoning entered continuation")
				}
				io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":3,"output_tokens":8,"total_tokens":11}}`)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentQwenCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"dashscope": upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			target := server.URL + "/?key=" + TestSecret + "&provider=dashscope&model=" + model + "&prompt=test"
			for _, effort := range []string{"", "none", "low", "medium", "xhigh"} {
				expectedEffort = effort
				requestURL := target + "&max_tokens=131072"
				if effort != "" {
					requestURL += "&reasoning_effort=" + effort
				}
				response, err := http.Get(requestURL)
				if err != nil {
					t.Fatal(err)
				}
				data, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if response.StatusCode != 200 || string(data) != "visible answer" || response.Header.Get("X-LLM-Proxy-Total-Tokens") != "20" {
					t.Fatalf("effort=%s status=%d body=%s usage=%s", effort, response.StatusCode, data, response.Header.Get("X-LLM-Proxy-Total-Tokens"))
				}
			}
			for _, query := range []string{"&reasoning_effort=minimal", "&reasoning_effort=high", "&reasoning_effort=max", "&max_tokens=15", "&max_tokens=131073"} {
				response, err := http.Get(target + query)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 400 || calls != 10 {
					t.Fatalf("query=%s status=%d calls=%d", query, response.StatusCode, calls)
				}
			}
		})
	}
}

func TestQwenCurrentImages(t *testing.T) {
	for _, model := range currentQwenModels {
		t.Run(model, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if payload["model"] != model || payload["reasoning"].(map[string]any)["effort"] != "medium" {
					t.Errorf("request=%v", payload)
				}
				content := payload["input"].([]any)[0].(map[string]any)["content"].([]any)
				if len(content) != 3 {
					t.Fatalf("content=%v", content)
				}
				for index, mime := range []string{"image/png", "image/jpeg"} {
					part := content[index].(map[string]any)
					if part["type"] != "input_image" || part["image_url"] != "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString([]byte(mime)) || part["detail"] != nil {
						t.Errorf("image=%v", part)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"images accepted"}]}]}`)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentQwenCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"dashscope": upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			attachments := []map[string]any{messageMediaPayload("image", "image/png", []byte("image/png")), messageMediaPayload("image", "image/jpeg", []byte("image/jpeg"))}
			body := mediaV2RequestBody(t, model, "inspect", attachments)
			body = strings.TrimSuffix(body, "}") + `,"reasoning_effort":"medium"}`
			response, err := http.Post(server.URL+"/v2?provider=dashscope&key="+TestSecret, "application/json", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if model == "qwen3.8-2.4t-a95b" {
				if response.StatusCode != 400 || calls != 0 {
					t.Fatalf("text-only model status=%d calls=%d body=%s", response.StatusCode, calls, data)
				}
				return
			}
			if response.StatusCode != 200 || calls != 1 || string(data) != "images accepted" {
				t.Fatalf("image status=%d calls=%d body=%s", response.StatusCode, calls, data)
			}
			excessive := make([]map[string]any, 251)
			for index := range excessive {
				excessive[index] = attachments[0]
			}
			response, err = http.Post(server.URL+"/v2?provider=dashscope&key="+TestSecret, "application/json", strings.NewReader(mediaV2RequestBody(t, model, "inspect", excessive)))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 413 || calls != 1 {
				t.Fatalf("image count status=%d calls=%d", response.StatusCode, calls)
			}
		})
	}
}

func TestQwenCurrentCatalog(t *testing.T) {
	catalog := currentQwenCatalog(t)
	public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: catalog})
	if err != nil {
		t.Fatal(err)
	}
	for index, model := range currentQwenModels {
		offeringIndex := slices.IndexFunc(public.Offerings, func(offering proxy.PublicProviderOffering) bool { return offering.Model == model })
		if offeringIndex < 0 {
			t.Fatalf("missing %s", model)
		}
		offering := public.Offerings[offeringIndex]
		if !reflect.DeepEqual(offering.ReasoningEfforts, []string{"none", "low", "medium", "xhigh"}) {
			t.Fatalf("efforts=%v", offering.ReasoningEfforts)
		}
		if len(offering.Limits) != 1 || offering.Limits[0].Value == nil || *offering.Limits[0].Value != 1000000 {
			t.Fatalf("context=%v", offering.Limits)
		}
		if len(offering.Controls) != 1 || offering.Controls[0].Minimum == nil || *offering.Controls[0].Minimum != 16 || offering.Controls[0].Maximum == nil || *offering.Controls[0].Maximum != 131072 {
			t.Fatalf("controls=%v", offering.Controls)
		}
		image := model != "qwen3.8-2.4t-a95b"
		if slices.Contains(offering.Capabilities, "image_input") != image || (len(offering.MediaLimits) == 3) != image {
			t.Fatalf("media offering=%+v", offering)
		}
		for _, limit := range offering.MediaLimits {
			if limit.ID == proxy.CatalogMediaLimitIDImageCount && (limit.Value == nil || *limit.Value != 250) {
				t.Errorf("count=%+v", limit)
			}
			if limit.ID == proxy.CatalogMediaLimitIDImageInlineBytes && (limit.Value == nil || *limit.Value != 20000000 || limit.Scope != "attachment_data_uri_bytes") {
				t.Errorf("URI=%+v", limit)
			}
		}
		priceIndex := slices.IndexFunc(public.Prices, func(price proxy.CatalogPriceDescriptor) bool { return price.Model == model })
		if priceIndex < 0 {
			t.Fatalf("missing price %s", model)
		}
		price := public.Prices[priceIndex]
		expected := [][2]float64{{2, 6}, {2, 6}, {.15, .47}, {2, 6}, {.5, 3}}[index]
		if !price.Available || len(price.Rates) != 2 || price.Source != "https://www.alibabacloud.com/help/en/model-studio/model-pricing" || price.LastVerified != "2026-09-05" {
			t.Fatalf("price=%+v", price)
		}
		for index, rate := range price.Rates {
			if rate.Rate != expected[index] || rate.Currency != "USD" || rate.Unit != "USD/1M_tokens" || rate.Conditions.BillingMode != "pay_as_you_go_list" || rate.Conditions.Mode != "singapore_international;thinking_or_non_thinking;input_tokens_0_to_1000000" || rate.Component != []string{"input_tokens", "output_tokens"}[index] {
				t.Fatalf("rate=%+v", rate)
			}
		}
	}
	familyIndex := slices.IndexFunc(catalog.Schema().Families, func(family proxy.ModelFamily) bool { return family.ID == "qwen3-8" })
	if familyIndex < 0 || catalog.Schema().Families[familyIndex].WeightAccess != "open_weights" {
		t.Fatal("open-weight Qwen family is missing")
	}
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", proxy.PublicCapabilitiesPath, nil))
	for _, model := range currentQwenModels {
		if strings.Contains(response.Body.String(), model) {
			t.Errorf("unqualified model is public: %s", model)
		}
	}
}

func TestQwenCurrentRejectMismatchedReasoningAdapter(t *testing.T) {
	schema := currentQwenCatalog(t).Schema()
	for index := range schema.Providers {
		if schema.Providers[index].ID == "gemini" {
			schema.Providers[index].Offerings[0].ReasoningEffort = &proxy.ReasoningEffortCapability{Adapter: "dashscope_responses", Efforts: []string{"low"}}
		}
	}
	_, err := proxy.NewProviderCatalog(schema)
	if err == nil || !strings.Contains(err.Error(), "adapter=dashscope_responses") {
		t.Fatalf("expected DashScope route error, got %v", err)
	}
}
