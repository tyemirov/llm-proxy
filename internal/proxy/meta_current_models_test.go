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

const currentMetaModel = "muse-spark-1.3"

func currentMetaCatalog(t *testing.T) *proxy.ProviderCatalog {
	t.Helper()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for index := range schema.Models {
		if schema.Models[index].ID == currentMetaModel {
			schema.Models[index].Enabled = proxy.ModelEnabled
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestMetaCurrentHTTP(t *testing.T) {
	expectedEffort := ""
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if r.Method != "POST" || r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer "+testMetaKey || payload["model"] != currentMetaModel || payload["max_completion_tokens"] != float64(4096) || payload["max_tokens"] != nil || payload["max_output_tokens"] != nil {
			t.Errorf("Meta request=%s %s payload=%v", r.Method, r.URL.Path, payload)
		}
		if expectedEffort == "" {
			if payload["reasoning_effort"] != nil {
				t.Error("omitted effort must remain omitted")
			}
		} else if payload["reasoning_effort"] != expectedEffort {
			t.Errorf("reasoning_effort=%v want=%s", payload["reasoning_effort"], expectedEffort)
		}
		messages := payload["messages"].([]any)
		w.Header().Set("Content-Type", "application/json")
		if calls%2 == 1 {
			if len(messages) != 1 || messages[0].(map[string]any)["content"] != "test" {
				t.Errorf("initial messages=%v", messages)
			}
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"visible ","reasoning_content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":2,"completion_tokens":7,"total_tokens":9}}`)
			return
		}
		if len(messages) != 3 || messages[1].(map[string]any)["content"] != "visible " || strings.Contains(string(metaCurrentJSON(t, messages)), "private") {
			t.Errorf("continuation=%v", messages)
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"answer","reasoning_content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":8,"total_tokens":11,"completion_tokens_details":{"reasoning_tokens":5}}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentMetaCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"meta": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, method := range []string{"GET", "POST", "v2"} {
		for _, effort := range []string{"", "minimal", "low", "medium", "high", "xhigh", "max"} {
			expectedEffort = effort
			requestURL := server.URL + "/?key=" + TestSecret + "&provider=meta"
			var request *http.Request
			if method == "GET" {
				requestURL += "&model=" + currentMetaModel + "&prompt=test&max_tokens=4096"
				if effort != "" {
					requestURL += "&reasoning_effort=" + effort
				}
				request, err = http.NewRequest("GET", requestURL, nil)
			} else {
				payload := map[string]any{"model": currentMetaModel, "max_tokens": 4096}
				if method == "v2" {
					requestURL = server.URL + "/v2?key=" + TestSecret + "&provider=meta"
					payload["messages"] = []any{map[string]string{"role": "user", "content": "test"}}
				} else {
					payload["prompt"] = "test"
				}
				if effort != "" {
					payload["reasoning_effort"] = effort
				}
				request, err = http.NewRequest("POST", requestURL, strings.NewReader(string(metaCurrentJSON(t, payload))))
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
	for _, effort := range []string{"none", "invalid"} {
		response, err := http.Get(server.URL + "/?key=" + TestSecret + "&provider=meta&model=" + currentMetaModel + "&prompt=test&reasoning_effort=" + effort)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 400 || calls != 42 {
			t.Fatalf("effort=%s status=%d calls=%d", effort, response.StatusCode, calls)
		}
	}
}

func TestMetaCurrentCatalog(t *testing.T) {
	public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: currentMetaCatalog(t)})
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(public.Offerings, func(offering proxy.PublicProviderOffering) bool { return offering.Model == currentMetaModel })
	if index < 0 {
		t.Fatal("missing Muse Spark 1.3 offering")
	}
	offering := public.Offerings[index]
	if !reflect.DeepEqual(offering.ReasoningEfforts, []string{"minimal", "low", "medium", "high", "xhigh", "max"}) || slices.Contains(offering.Capabilities, "image_input") {
		t.Fatalf("offering=%+v", offering)
	}
	if len(offering.Limits) != 1 || offering.Limits[0].Value == nil || *offering.Limits[0].Value != 1048576 {
		t.Fatalf("limits=%v", offering.Limits)
	}
	priceIndex := slices.IndexFunc(public.Prices, func(price proxy.CatalogPriceDescriptor) bool { return price.Model == currentMetaModel })
	if priceIndex < 0 {
		t.Fatal("missing Meta price")
	}
	price := public.Prices[priceIndex]
	if !price.Available || price.Source != "https://dev.meta.ai/docs/pricing-rate-limits" || price.LastVerified != "2026-09-05" || len(price.Rates) != 3 {
		t.Fatalf("price=%+v", price)
	}
	for index, expected := range []float64{1.25, 4.25, 0.15} {
		rate := price.Rates[index]
		if rate.Rate != expected || rate.Currency != "USD" || rate.Unit != "USD/1M_tokens" || rate.Conditions.BillingMode != "standard" || rate.Component != []string{"input_tokens", "output_tokens", "cache_read"}[index] {
			t.Fatalf("rate=%v", rate)
		}
	}
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", proxy.PublicCapabilitiesPath, nil))
	if response.Code != 200 || strings.Contains(response.Body.String(), currentMetaModel) {
		t.Fatalf("candidate discovery status=%d", response.Code)
	}
}

func TestMetaCurrentRejectStructuredOutput(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Error("unsupported structured output reached Meta")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentMetaCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"meta": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	response := performStructuredRequestForProvider(t, router, `{"model":"muse-spark-1.3","reasoning_effort":"max","messages":[{"role":"user","content":"answer"}],"structured_output":{"schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}}}`, "meta-current-structured", "meta")
	if response.Code != 400 || !strings.Contains(response.Body.String(), "structured_output is unsupported") || calls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
}

func metaCurrentJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
