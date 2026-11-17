package proxy_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func TestProviderCatalogXA1DActivation(t *testing.T) {
	catalog := testfixtures.ProviderCatalog(t)
	service, err := proxy.NewCatalogService(catalog.ModelCatalog())
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range [][2]string{{"openai", "gpt-6-luna"}, {"anthropic", "claude-haiku-5-5"}, {"gemini", "gemini-3.5-flash-lite"}} {
		if _, err := service.ResolveOffering(route[0], route[1]); err != nil {
			t.Fatalf("authorized qualification route unavailable: %v %v", route, err)
		}
	}
	if _, err := service.ResolveOffering("dashscope", "qwen3.8-27b"); err == nil {
		t.Fatal("Cerebras qualification incorrectly activated Alibaba 27B")
	}
	offering, err := service.ResolveOffering("cerebras", "qwen3.8-27b")
	if err != nil || offering.ProviderModel != "qwen-3.8-27b" || !slices.Contains(offering.DefaultOperations, "text") {
		t.Fatalf("qualified Cerebras route: %+v %v", offering, err)
	}
}

func TestProviderCatalogXA1DPriceBoundaries(t *testing.T) {
	service, err := proxy.NewCatalogService(testfixtures.ProviderCatalog(t).ModelCatalog())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	for _, candidate := range []struct {
		provider, model string
		boundary        uint64
		components      []string
		caches          []string
		lower, upper    []string
	}{
		{"openai", "gpt-6-luna", 272000, []string{"input_tokens", "output_tokens", "prompt_cache_read_tokens", "prompt_cache_write_tokens"}, []string{"", "", "read", "write"}, []string{"0.1", "0.5", "0.01", "0.125"}, []string{"0.2", "0.75", "0.02", "0.25"}},
		{"anthropic", "claude-haiku-5-5", 100000, []string{"input_tokens", "output_tokens", "prompt_cache_read_tokens", "prompt_cache_write_tokens", "prompt_cache_write_tokens"}, []string{"", "", "read", "write_5m", "write_1h"}, []string{"0.1", "0.5", "0.01", "0.125", "0.2"}, []string{"0.5", "2.5", "0.05", "0.625", "1"}},
	} {
		for index, component := range candidate.components {
			for _, tokens := range []uint64{candidate.boundary, candidate.boundary + 1} {
				expected := candidate.lower[index]
				if tokens > candidate.boundary {
					expected = candidate.upper[index]
				}
				selected := service.SelectApplicablePrice(candidate.provider, candidate.model, "text", component, proxy.CatalogPriceConditions{BillingMode: "standard", ServiceTier: "standard", CacheClass: candidate.caches[index]}, tokens, at)
				if !selected.Available || selected.Rate == nil || string(selected.Rate.Rate) != expected || selected.LastVerified != "2026-11-16" {
					t.Fatalf("%s %s %s input=%d selection=%+v expected=%s", candidate.provider, candidate.model, component, tokens, selected, expected)
				}
			}
		}
	}
}

func TestProviderCatalogXA1DCandidateProtocol(t *testing.T) {
	for _, candidate := range []struct {
		provider, model, path, response string
		efforts                         []string
	}{
		{"openai", "gpt-6-luna", "/responses", `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"yes"}]}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`, []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{"anthropic", "claude-haiku-5-5", "/v1/messages", `{"content":[{"type":"text","text":"yes"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`, []string{"low", "medium", "high", "xhigh", "max"}},
	} {
		t.Run(candidate.model, func(t *testing.T) {
			expectedEffort := ""
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.Method != http.MethodPost || r.URL.Path != candidate.path || body["model"] != candidate.model {
					t.Errorf("wire route %s %s %v", r.Method, r.URL.Path, body)
				}
				control := "reasoning"
				if candidate.provider == "anthropic" {
					control = "output_config"
					if body["thinking"] != nil || body["temperature"] != nil {
						t.Errorf("unexpected adaptive/sampling override: %v", body)
					}
				}
				if expectedEffort == "" {
					if body[control] != nil {
						t.Errorf("omitted effort sent: %v", body)
					}
				} else if body[control].(map[string]any)["effort"] != expectedEffort {
					t.Errorf("effort: %v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, candidate.response)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{AssetStorePath: t.TempDir(), ProviderCatalog: testfixtures.ProviderCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{candidate.provider: upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			for _, effort := range append([]string{""}, candidate.efforts...) {
				expectedEffort = effort
				requestURL := fmt.Sprintf("%s/?key=%s&provider=%s&model=%s&prompt=test&max_tokens=128", server.URL, TestSecret, candidate.provider, candidate.model)
				if effort != "" {
					requestURL += "&reasoning_effort=" + effort
				}
				response, err := http.Get(requestURL)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if response.StatusCode != http.StatusOK || string(body) != "yes" {
					t.Fatalf("effort=%s status=%d body=%s", effort, response.StatusCode, body)
				}
			}
			if calls != len(candidate.efforts)+1 {
				t.Fatalf("calls=%d", calls)
			}
			if candidate.provider == "anthropic" {
				body := `{"model":"claude-haiku-5-5","messages":[{"role":"user","content":"score"}],"structured_output":{"schema":{"type":"object","properties":{"score":{"type":"integer","minimum":1,"maximum":10}},"required":["score"],"additionalProperties":false}}}`
				response := performStructuredRequestForProvider(t, router, body, "xa1d-haiku-minimum", "anthropic")
				if response.Code != http.StatusBadRequest || calls != len(candidate.efforts)+1 {
					t.Fatalf("native numeric bounds reached provider: status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
				}
			}
		})
	}
}

func TestProviderCatalogXA1DDeclaredControls(t *testing.T) {
	service, err := proxy.NewCatalogService(testfixtures.ProviderCatalog(t).ModelCatalog())
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"openai", "anthropic"} {
		model := "gpt-6-luna"
		expectedContext := 1050000
		expectedEfforts := []string{"none", "low", "medium", "high", "xhigh", "max"}
		if provider == "anthropic" {
			model = "claude-haiku-5-5"
			expectedContext = 1000000
			expectedEfforts = expectedEfforts[1:]
		}
		offering, err := service.ResolveOffering(provider, model)
		if err != nil {
			t.Fatal(err)
		}
		if offering.OutputTokenLimit != 128000 || offering.ReasoningEffort == nil || !slices.Equal(offering.ReasoningEffort.Efforts, expectedEfforts) {
			t.Fatalf("controls: %+v", offering)
		}
		foundContext := false
		for _, limit := range offering.Limits {
			if limit.ID == "context_tokens" && limit.Value != nil && *limit.Value == expectedContext {
				foundContext = true
			}
		}
		if !foundContext {
			t.Fatalf("context: %+v", offering.Limits)
		}
		if provider == "anthropic" {
			expected := []string{"$defs", "$ref", "additionalProperties", "allOf", "anyOf", "const", "default", "definitions", "description", "enum", "format", "items", "minItems", "pattern", "properties", "required", "title", "type"}
			policy := offering.StructuredOutput
			if policy == nil || !slices.Equal(policy.Keywords, expected) || policy.Root != "any" || policy.Objects != "closed" || policy.RequiredProperties != "explicit" || policy.References != "local_nonroot" || policy.ArrayMinimum != "zero_or_one" || policy.ReferenceAllOf != "reject" {
				t.Fatalf("Anthropic schema policy: %+v", policy)
			}
		}
	}
}

func TestProviderCatalogXA1DHaikuEnumAndConstSchema(t *testing.T) {
	const schema = `{"type":"object","properties":{"score":{"type":"integer","enum":[1,2,3,4,5,6,7,8,9,10]},"identity":{"type":"string","const":"fixed-identity"}},"required":["score","identity"],"additionalProperties":false}`
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		format := body["output_config"].(map[string]any)["format"].(map[string]any)
		observed, _ := json.Marshal(format["schema"])
		if r.URL.Path != "/v1/messages" || body["model"] != "claude-haiku-5-5" || format["type"] != "json_schema" || !jsonBodiesEqual(observed, []byte(schema)) {
			t.Errorf("Anthropic schema wire changed: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"content":[{"type":"text","text":"{\"score\":10,\"identity\":\"fixed-identity\"}"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{AssetStorePath: t.TempDir(), ProviderCatalog: testfixtures.ProviderCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"anthropic": upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	body := `{"model":"claude-haiku-5-5","messages":[{"role":"user","content":"score"}],"structured_output":{"schema":` + schema + `}}`
	response := performStructuredRequestForProvider(t, router, body, "xa1d-haiku-enum-const", "anthropic")
	if response.Code != http.StatusOK || calls != 1 || !jsonBodiesEqual(response.Body.Bytes(), []byte(`{"score":10,"identity":"fixed-identity"}`)) {
		t.Fatalf("Haiku schema status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
}
