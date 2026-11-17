package proxy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
)

func TestClientProtocolsCerebrasPlainTextAndUsage(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/chat/completions" || request.Header.Get("Authorization") != "Bearer provider-fixture" {
			t.Errorf("upstream boundary: method=%s path=%s authorization=%q", request.Method, request.URL.Path, request.Header.Get("Authorization"))
		}
		var payload struct {
			Model               string                        `json:"model"`
			MaxCompletionTokens int                           `json:"max_completion_tokens"`
			ReasoningEffort     string                        `json:"reasoning_effort"`
			Messages            []llmproxyclient.MessageInput `json:"messages"`
			ResponseFormat      json.RawMessage               `json:"response_format"`
			MaxTokens           json.RawMessage               `json:"max_tokens"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.Model != "qwen-3.8-27b" || payload.MaxCompletionTokens != 32768 || payload.ReasoningEffort != "none" || len(payload.ResponseFormat) != 0 || len(payload.MaxTokens) != 0 || len(payload.Messages) != 2 || payload.Messages[0].Role != "system" || payload.Messages[0].Content != "Keep punctuation" || payload.Messages[1].Content != "Answer plainly" {
			t.Errorf("plain wire payload=%+v", payload)
		}
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, `{"id":"private-upstream-id","choices":[{"message":{"role":"assistant","content":"Plain answer — preserved."},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14,"completion_tokens_details":{"reasoning_tokens":3}}}`)
	}))
	defer upstream.Close()
	server := cerebrasHTTPServer(t, upstream.URL, "cerebras")
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: TestSecret, Provider: "cerebras"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	limit, effort := 32768, "none"
	input := llmproxyclient.MessagesRequestInput{Model: cerebrasQualifiedModel, MaxTokens: &limit, ReasoningEffort: &effort, Messages: []llmproxyclient.MessageInput{{Role: "system", Content: "Keep punctuation"}, {Role: "user", Content: "Answer plainly"}}}
	request, err := llmproxyclient.NewMessagesRequest(input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := client.PostMessages(context.Background(), request)
	if err != nil || output != "Plain answer — preserved." {
		t.Fatalf("plain output=%q err=%v", output, err)
	}
	status, headers, body := cerebrasPost(t, server, "/v1/chat/completions", `{"model":"cerebras/qwen3.8-27b","messages":[{"role":"system","content":"Keep punctuation"},{"role":"user","content":"Answer plainly"}],"max_tokens":32768,"reasoning_effort":"none"}`, "")
	var response struct {
		Model string `json:"model"`
		Usage struct {
			Input  int `json:"prompt_tokens"`
			Output int `json:"completion_tokens"`
			Total  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || response.Model != "cerebras/qwen3.8-27b" || response.Usage.Input != 9 || response.Usage.Output != 5 || response.Usage.Total != 14 || headers.Get("X-LLM-Proxy-Total-Tokens") != "14" || strings.Contains(string(body), "private-upstream-id") || calls.Load() != 2 {
		t.Fatalf("public response status=%d body=%s headers=%v calls=%d", status, body, headers, calls.Load())
	}
}

func TestClientProtocolsCerebrasProviderErrors(t *testing.T) {
	for _, providerStatus := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(providerStatus), func(t *testing.T) {
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				if providerStatus == http.StatusTooManyRequests {
					writer.Header().Set("Retry-After", "2")
				}
				writer.WriteHeader(providerStatus)
				io.WriteString(writer, `{"error":{"message":"protected provider body provider-fixture","code":"fixture_error"}}`)
			}))
			defer upstream.Close()
			server := cerebrasHTTPServer(t, upstream.URL, "cerebras")
			status, headers, body := cerebrasPost(t, server, "/v1/chat/completions", `{"model":"cerebras/qwen3.8-27b","messages":[{"role":"user","content":"private user prompt"}]}`, "")
			expected := http.StatusBadGateway
			if providerStatus == http.StatusTooManyRequests {
				expected = http.StatusTooManyRequests
			}
			if status != expected || !json.Valid(body) || calls.Load() != 1 {
				t.Fatalf("status=%d body=%s calls=%d", status, body, calls.Load())
			}
			for _, protected := range []string{"protected provider body", "provider-fixture", "private user prompt", TestSecret} {
				if strings.Contains(string(body), protected) {
					t.Fatalf("public error exposed %q", protected)
				}
			}
			if providerStatus == http.StatusTooManyRequests && headers.Get("Retry-After") != "2" {
				t.Fatalf("retry headers=%v", headers)
			}
		})
	}
}

func TestClientProtocolsCerebrasCanonicalActivationRequired(t *testing.T) {
	for _, disabled := range []string{"provider", "model", "offering"} {
		t.Run(disabled, func(t *testing.T) {
			schema := testfixtures.ProviderCatalog(t).Schema()
			for index := range schema.Models {
				if disabled == "model" && schema.Models[index].ID == cerebrasQualifiedModel {
					schema.Models[index].Enabled = proxy.ModelDisabled
				}
			}
			for index := range schema.Providers {
				provider := &schema.Providers[index]
				if provider.ID != "cerebras" {
					continue
				}
				if disabled == "provider" {
					provider.Enabled = proxy.ModelDisabled
				}
				for offeringIndex := range provider.Offerings {
					if disabled == "offering" && provider.Offerings[offeringIndex].Model == cerebrasQualifiedModel {
						provider.Offerings[offeringIndex].Enabled = proxy.ModelDisabled
						provider.Offerings[offeringIndex].DefaultOperations = nil
					}
				}
			}
			catalog, err := proxy.NewProviderCatalog(schema)
			if disabled == "model" {
				if err == nil || !strings.Contains(err.Error(), "disabled_default") {
					t.Fatalf("disabled default model accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				writer.WriteHeader(http.StatusInternalServerError)
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpointOverrides(map[string]string{"cerebras": upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			status, _, body := cerebrasPost(t, server, "/v1/chat/completions", `{"model":"cerebras/qwen3.8-27b","messages":[{"role":"user","content":"hello"}]}`, "")
			if status != http.StatusBadRequest || calls.Load() != 0 {
				t.Fatalf("disabled %s status=%d body=%s calls=%d", disabled, status, body, calls.Load())
			}
			response, err := server.Client().Get(server.URL + proxy.PublicCapabilitiesPath)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var public proxy.PublicCapabilityCatalog
			if err := json.NewDecoder(response.Body).Decode(&public); err != nil {
				t.Fatal(err)
			}
			for _, offering := range public.Offerings {
				if offering.Provider == "cerebras" && offering.Model == cerebrasQualifiedModel {
					t.Fatalf("disabled %s remains discoverable", disabled)
				}
			}
		})
	}
}
