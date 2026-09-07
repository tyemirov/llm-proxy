package proxy_test

import (
	"bytes"
	"context"
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

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

var currentClaudeModels = []string{"claude-fable-5-1", "claude-opus-5"}
var currentClaudeEfforts = []string{"low", "medium", "high", "xhigh", "max"}

func currentClaudeCandidateCatalog(t *testing.T) *proxy.ProviderCatalog {
	t.Helper()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for index := range schema.Models {
		if slices.Contains(currentClaudeModels, schema.Models[index].ID) {
			schema.Models[index].Enabled = proxy.ModelEnabled
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestClaudeCurrentModelsHTTP(t *testing.T) {
	for _, model := range currentClaudeModels {
		t.Run(model, func(t *testing.T) {
			var expectedEffort string
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if r.URL.Path != "/v1/messages" || r.Method != http.MethodPost || body["model"] != model || body["max_tokens"] != float64(128000) {
					t.Errorf("Claude request=%s %s %v", r.Method, r.URL.Path, body)
				}
				if _, exists := body["thinking"]; exists {
					t.Error("request changed the model's adaptive thinking default")
				}
				if expectedEffort == "" {
					if _, exists := body["output_config"]; exists {
						t.Error("omitted effort must omit output_config")
					}
				} else if !reflect.DeepEqual(body["output_config"], map[string]any{"effort": expectedEffort}) {
					t.Errorf("effort payload=%v expected=%s", body["output_config"], expectedEffort)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"content":[{"type":"thinking","thinking":"private Claude reasoning","signature":"private signature"},{"type":"text","text":"visible answer"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`)
			}))
			defer upstream.Close()
			core, logs := observer.New(zap.DebugLevel)
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentClaudeCandidateCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameAnthropic: upstream.URL}, nil)}, zap.New(core).Sugar())
			if err != nil {
				t.Fatal(err)
			}
			for _, effort := range append([]string{""}, currentClaudeEfforts...) {
				expectedEffort = effort
				query := "/?key=" + TestSecret + "&provider=anthropic&model=" + model + "&prompt=test"
				if effort != "" {
					query += "&reasoning_effort=" + effort
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, query, nil))
				if response.Code != http.StatusOK || response.Body.String() != "visible answer" || response.Header().Get("X-LLM-Proxy-Total-Tokens") != "5" {
					t.Fatalf("Claude %s effort=%s status=%d body=%s", model, effort, response.Code, response.Body.String())
				}
			}
			for _, parameters := range []string{"model=" + model + "&reasoning_effort=none", "model=" + model + "&max_tokens=128001", "model=claude-sonnet-4-6&reasoning_effort=high"} {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&provider=anthropic&prompt=test&"+parameters, nil))
				if response.Code != http.StatusBadRequest {
					t.Fatalf("invalid Claude input status=%d body=%s", response.Code, response.Body.String())
				}
			}
			if calls != 6 || strings.Contains(fmt.Sprint(logs.All()), "private Claude reasoning") || strings.Contains(fmt.Sprint(logs.All()), "private signature") {
				t.Fatalf("calls=%d or private reasoning was logged", calls)
			}
		})
	}
}

func TestClaudeCurrentModelsImagesAndStructuredOutput(t *testing.T) {
	for _, model := range currentClaudeModels {
		t.Run(model, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				output := body["output_config"].(map[string]any)
				format := output["format"].(map[string]any)
				if body["model"] != model || output["effort"] != "high" || format["type"] != "json_schema" || format["schema"] == nil {
					t.Errorf("structured Claude request=%v", body)
				}
				content := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
				for index, mime := range []string{"image/jpeg", "image/png", "image/webp"} {
					source := content[index].(map[string]any)["source"].(map[string]any)
					if source["type"] != "base64" || source["media_type"] != mime || source["data"] != base64.StdEncoding.EncodeToString([]byte(mime)) {
						t.Errorf("image order/data=%v", content)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"content":[{"type":"thinking","thinking":"private"},{"type":"text","text":"{\"answer\":\"yes\"}"}],"stop_reason":"end_turn"}`)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentClaudeCandidateCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameAnthropic: upstream.URL}, nil), AssetStorePath: t.TempDir()}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			var attachments []any
			for _, mime := range []string{"image/jpeg", "image/png", "image/webp"} {
				attachments = append(attachments, map[string]string{"type": "image", "mime_type": mime, "data": base64.StdEncoding.EncodeToString([]byte(mime))})
			}
			body, _ := json.Marshal(map[string]any{
				"model": model, "reasoning_effort": "high", "messages": []any{map[string]any{"role": "user", "content": "inspect", "attachments": attachments}},
				"structured_output": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]string{"type": "string"}}, "required": []string{"answer"}, "additionalProperties": false}},
			})
			response := performStructuredRequestForProvider(t, router, string(body), "claude-current:"+model, "anthropic")
			if response.Code != http.StatusOK || response.Body.String() != `{"answer":"yes"}` || calls != 1 {
				t.Fatalf("Claude image/structured status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
			}
		})
	}
}

func TestClaudeCurrentModelsManagedVerification(t *testing.T) {
	for _, model := range currentClaudeModels {
		t.Run(model, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["model"] != model || body["max_tokens"] != float64(16) || body["thinking"] != nil || body["output_config"] != nil {
					t.Errorf("Claude verification=%v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"id":"claude-key-verification","type":"message","role":"assistant","content":[{"type":"text","text":"verified"}],"stop_reason":"end_turn"}`)
			}))
			defer upstream.Close()
			configuration := providerKeyVerificationConfiguration(upstream.URL)
			configuration.ProviderCatalog = currentClaudeCandidateCatalog(t)
			router := newOperationalProviderKeyVerificationRouter(t, configuration, zap.NewNop().Sugar(), t.TempDir()+"/managed.db", TestTimeout)
			cookie := managementSessionCookie(t, "claude-current-verification")
			tenant := managementDefaultTenantTestID(t, router, cookie)
			response := putManagementProviderKey(t, router, cookie, tenant, "anthropic", "claude-candidate-key", model, "", context.Background())
			if response.Code != http.StatusOK || calls != 1 {
				t.Fatalf("verification status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
			}
		})
	}
}

func TestClaudeCurrentModelsContinuation(t *testing.T) {
	for _, model := range currentClaudeModels {
		t.Run(model, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["model"] != model || body["output_config"].(map[string]any)["effort"] != "max" {
					t.Errorf("continuation model/effort=%v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				if calls == 1 {
					io.WriteString(w, `{"content":[{"type":"thinking","thinking":"private continuation"},{"type":"text","text":"visible"}],"stop_reason":"max_tokens","usage":{"input_tokens":2,"output_tokens":3}}`)
					return
				}
				messages := body["messages"].([]any)
				if len(messages) != 3 || messages[1].(map[string]any)["content"] != "visible" || messages[2].(map[string]any)["role"] != "user" || strings.Contains(fmt.Sprint(body), "private continuation") {
					t.Errorf("continuation history=%v", messages)
				}
				io.WriteString(w, `{"content":[{"type":"text","text":" suffix"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentClaudeCandidateCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameAnthropic: upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&provider=anthropic&model="+model+"&prompt=test&max_tokens=32&reasoning_effort=max", nil))
			if response.Code != http.StatusOK || response.Body.String() != "visible suffix" || calls != 2 || response.Header().Get("X-LLM-Proxy-Total-Tokens") != "10" {
				t.Fatalf("continuation status=%d calls=%d body=%s usage=%s", response.Code, calls, response.Body.String(), response.Header().Get("X-LLM-Proxy-Total-Tokens"))
			}
		})
	}
}

func TestClaudeCurrentModelsCatalog(t *testing.T) {
	catalog := currentClaudeCandidateCatalog(t)
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/public/capabilities", nil))
	var public proxy.PublicCapabilityCatalog
	if err := json.Unmarshal(response.Body.Bytes(), &public); err != nil {
		t.Fatal(err)
	}
	for _, model := range currentClaudeModels {
		foundOffering, foundPrice := false, false
		for _, offering := range public.Offerings {
			if offering.Model != model {
				continue
			}
			foundOffering = true
			if offering.Provider != "anthropic" || offering.OutputTokenLimit != 128000 || !reflect.DeepEqual(offering.ReasoningEfforts, currentClaudeEfforts) || !slices.Contains(offering.Capabilities, "image_input") || len(offering.Limits) != 1 || (offering.Limits[0].Value == nil || *offering.Limits[0].Value != 1000000) {
				t.Errorf("Claude public offering=%+v", offering)
			}
		}
		for _, price := range public.Prices {
			if price.Model != model {
				continue
			}
			foundPrice = true
			expected := []float64{10, 50, .25, 12.5, 20}
			if model == "claude-opus-5" {
				expected = []float64{5, 25, .5, 6.25, 10}
			}
			if len(price.Rates) != len(expected) {
				t.Fatalf("Claude price rates=%v", price.Rates)
			}
			for index, rate := range price.Rates {
				if rate.Rate != expected[index] || rate.Conditions.BillingMode != "standard" {
					t.Errorf("Claude rate=%v", rate)
				}
			}
			if price.Rates[3].Conditions.Duration != "5m" || price.Rates[4].Conditions.Duration != "1h" {
				t.Errorf("Claude cache durations=%v", price.Rates)
			}
		}
		if !foundOffering || !foundPrice {
			t.Fatalf("Claude model=%s offering=%t price=%t", model, foundOffering, foundPrice)
		}
	}
	schema := catalog.Schema()
	for providerIndex := range schema.Providers {
		provider := &schema.Providers[providerIndex]
		if provider.ID == "openai" {
			provider.Offerings[0].ReasoningEffort = &proxy.ReasoningEffortCapability{Adapter: "anthropic_messages", Efforts: currentClaudeEfforts}
		}
	}
	if _, err := proxy.NewProviderCatalog(schema); err == nil || !strings.Contains(err.Error(), "adapter=anthropic_messages") {
		t.Fatalf("Anthropic effort on wrong protocol error=%v", err)
	}
}

func TestClaudeCurrentModelsImageRejection(t *testing.T) {
	for _, model := range currentClaudeModels {
		t.Run(model, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: currentClaudeCandidateCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameAnthropic: upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			for _, scenario := range []string{"encoded image bytes", "image count", "unsupported MIME"} {
				attachments := []map[string]string{{"type": "image", "mime_type": "image/png", "data": "YQ=="}}
				expected := http.StatusRequestEntityTooLarge
				switch scenario {
				case "encoded image bytes":
					attachments[0]["data"] = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("a"), 7500001))
				case "image count":
					for len(attachments) < 601 {
						attachments = append(attachments, attachments[0])
					}
				case "unsupported MIME":
					attachments[0]["mime_type"] = "image/gif"
					expected = http.StatusBadRequest
				}
				body, _ := json.Marshal(map[string]any{"model": model, "messages": []any{map[string]any{"role": "user", "content": "inspect", "attachments": attachments}}})
				request := httptest.NewRequest(http.MethodPost, "/v2?provider=anthropic&key="+TestSecret, bytes.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != expected {
					t.Fatalf("%s status=%d body=%s", scenario, response.Code, response.Body.String())
				}
			}
			if calls != 0 {
				t.Fatalf("invalid images reached provider: calls=%d", calls)
			}
		})
	}
}
