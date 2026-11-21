package proxy_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

type openAIGPT6Fixture struct {
	model       string
	noneAllowed bool
	inputLimit  int
	prices      [6]string
}

var openAIGPT6Fixtures = []openAIGPT6Fixture{
	{"gpt-6-astra", false, 922000, [6]string{"10", "50", "1", "20", "75", "2"}},
	{"gpt-6-sol", true, 0, [6]string{"2", "10", "0.2", "4", "15", "0.4"}},
	{"gpt-6-luna", true, 0, [6]string{"0.1", "0.5", "0.01", "0.2", "0.75", "0.02"}},
	{"gpt-6.1-sol", false, 0, [6]string{"2", "10", "0.1", "4", "15", "0.2"}},
	{"gpt-6.1-terra", true, 0, [6]string{"1", "5", "0.05", "2", "7.5", "0.1"}},
	{"gpt-6.2-astra", false, 922000, [6]string{"8", "40", "0.4", "16", "60", "0.8"}},
	{"gpt-6.2-sol", false, 922000, [6]string{"2", "10", "0.1", "4", "15", "0.2"}},
}

func TestOpenAIGPT6Catalog(t *testing.T) {
	for _, modelFixture := range openAIGPT6Fixtures {
		t.Run(modelFixture.model, func(t *testing.T) {
			model := modelFixture.model
			catalog := testfixtures.ProviderCatalog(t)
			public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: catalog})
			if err != nil {
				t.Fatal(err)
			}
			index := slices.IndexFunc(public.Offerings, func(offering proxy.PublicProviderOffering) bool {
				return offering.Provider == "openai" && offering.Model == model
			})
			if index < 0 {
				t.Fatalf("missing %s offering", model)
			}
			offering := public.Offerings[index]
			efforts := []string{"low", "medium", "high", "xhigh", "max"}
			if modelFixture.noneAllowed {
				efforts = append([]string{"none"}, efforts...)
			}
			if !reflect.DeepEqual(offering.ReasoningEfforts, efforts) || !slices.Contains(offering.Capabilities, "image_input") || !slices.Contains(offering.Capabilities, "structured_output") {
				t.Fatalf("offering=%+v", offering)
			}
			limits := map[string]int{}
			for _, limit := range offering.Limits {
				if limit.Value != nil {
					limits[limit.ID] = *limit.Value
				}
			}
			if limits["context_tokens"] != 1050000 || limits["input_tokens"] != modelFixture.inputLimit || (model != "gpt-6-luna" && limits["output_tokens"] != 128000) {
				t.Fatalf("limits=%v", limits)
			}
			server := httptest.NewServer(proxy.BuildPublicCapabilityRouter(public, "error"))
			t.Cleanup(server.Close)
			response, err := server.Client().Get(server.URL + proxy.PublicCapabilitiesPath)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(body), model) {
				t.Fatalf("public discovery status=%d error=%v", response.StatusCode, err)
			}
			service, err := proxy.NewCatalogService(catalog.ModelCatalog())
			if err != nil {
				t.Fatal(err)
			}
			at := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
			for _, fixture := range []struct{ component, cache, short, long string }{
				{"input_tokens", "", modelFixture.prices[0], modelFixture.prices[3]}, {"output_tokens", "", modelFixture.prices[1], modelFixture.prices[4]}, {"prompt_cache_read_tokens", "read", modelFixture.prices[2], modelFixture.prices[5]},
			} {
				for _, count := range []uint64{272000, 272001} {
					want := fixture.short
					if count > 272000 {
						want = fixture.long
					}
					selected := service.SelectApplicablePrice("openai", model, "text", fixture.component, proxy.CatalogPriceConditions{BillingMode: "standard", ServiceTier: "standard", CacheClass: fixture.cache}, count, at)
					if !selected.Available || selected.Rate == nil || string(selected.Rate.Rate) != want {
						t.Fatalf("%s input=%d selected=%+v want=%s", fixture.component, count, selected, want)
					}
				}
			}
			if selected := service.SelectApplicablePrice("openai", model, "text", "prompt_cache_write_tokens", proxy.CatalogPriceConditions{BillingMode: "standard", ServiceTier: "standard", CacheClass: "write"}, 1, at); model != "gpt-6-astra" && model != "gpt-6-luna" && selected.Available {
				t.Fatal("unspecified cache retention was assigned a price")
			}
		})
	}
}

func TestOpenAIGPT6HTTPStructuredImageAndRejections(t *testing.T) {
	for _, modelFixture := range openAIGPT6Fixtures {
		t.Run(modelFixture.model, func(t *testing.T) {
			model := modelFixture.model
			selectedEffort := "medium"
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls++
				var payload map[string]any
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Error(err)
					writer.WriteHeader(400)
					return
				}
				if request.URL.Path != "/responses" || payload["model"] != model || payload["store"] != true || payload["background"] != true || payload["max_output_tokens"] != float64(4096) {
					t.Errorf("Responses payload=%v", payload)
				}
				if payload["reasoning"].(map[string]any)["effort"] != selectedEffort || payload["temperature"] != nil || payload["top_p"] != nil {
					t.Error("reasoning or sampling policy differs")
				}
				content := payload["input"].([]any)[0].(map[string]any)["content"].([]any)
				image := content[0].(map[string]any)
				if image["type"] != "input_image" || image["image_url"] != "data:image/png;base64,"+base64.StdEncoding.EncodeToString([]byte("image/png")) {
					t.Error("image bytes or placement differ")
				}
				format := payload["text"].(map[string]any)["format"].(map[string]any)
				if format["type"] != "json_schema" || format["strict"] != true {
					t.Error("strict structured schema not serialized")
				}
				writer.Header().Set("Content-Type", "application/json")
				io.WriteString(writer, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"answer\":\"yes\"}"}]}],"usage":{"input_tokens":120,"output_tokens":30,"total_tokens":150}}`)
			}))
			t.Cleanup(upstream.Close)
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"openai": upstream.URL}, nil), AssetStorePath: t.TempDir()}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			discoveryRequest, err := http.NewRequest(http.MethodGet, server.URL+"/v1/models", nil)
			if err != nil {
				t.Fatal(err)
			}
			discoveryRequest.Header.Set("Authorization", "Bearer "+TestSecret)
			discoveryResponse, err := server.Client().Do(discoveryRequest)
			if err != nil {
				t.Fatal(err)
			}
			discoveryBody, err := io.ReadAll(discoveryResponse.Body)
			discoveryResponse.Body.Close()
			if err != nil || discoveryResponse.StatusCode != http.StatusOK || !strings.Contains(string(discoveryBody), "openai/"+model) || calls != 0 {
				t.Fatalf("tenant route discovery status=%d calls=%d error=%v", discoveryResponse.StatusCode, calls, err)
			}
			base := map[string]any{"model": model, "reasoning_effort": "medium", "max_tokens": 4096, "messages": []any{map[string]any{"role": "user", "content": "inspect", "attachments": []any{map[string]string{"type": "image", "mime_type": "image/png", "data": base64.StdEncoding.EncodeToString([]byte("image/png"))}}}}, "structured_output": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]string{"type": "string"}}, "required": []string{"answer"}, "additionalProperties": false}}}
			cases := []struct {
				effort         string
				tokens, status int
			}{}
			for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
				cases = append(cases, struct {
					effort         string
					tokens, status int
				}{effort, 4096, http.StatusOK})
			}
			noneStatus := http.StatusBadRequest
			if modelFixture.noneAllowed {
				noneStatus = http.StatusOK
			}
			cases = append(cases, struct {
				effort         string
				tokens, status int
			}{"none", 4096, noneStatus},
				struct {
					effort         string
					tokens, status int
				}{"minimal", 4096, http.StatusBadRequest},
				struct {
					effort         string
					tokens, status int
				}{"medium", 128001, http.StatusBadRequest})
			expectedCalls := 0
			for index, fixture := range cases {
				selectedEffort = fixture.effort
				base["reasoning_effort"], base["max_tokens"] = fixture.effort, fixture.tokens
				body, err := json.Marshal(base)
				if err != nil {
					t.Fatal(err)
				}
				request, err := http.NewRequest("POST", server.URL+"/v2?key="+TestSecret+"&provider=openai", strings.NewReader(string(body)))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Idempotency-Key", fmt.Sprintf("%s-case-%d", model, index))
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := io.ReadAll(response.Body)
				response.Body.Close()
				if fixture.status == http.StatusOK {
					expectedCalls++
				}
				if err != nil || response.StatusCode != fixture.status || calls != expectedCalls {
					t.Fatalf("case=%d effort=%s status=%d calls=%d expected=%d body=%s error=%v", index, fixture.effort, response.StatusCode, calls, expectedCalls, actual, err)
				}
				if fixture.status == http.StatusOK && (string(actual) != `{"answer":"yes"}` || response.Header.Get("X-LLM-Proxy-Request-Tokens") != "120" || response.Header.Get("X-LLM-Proxy-Response-Tokens") != "30") {
					t.Fatalf("completion or usage differs: %s", actual)
				}
			}
		})
	}
}
