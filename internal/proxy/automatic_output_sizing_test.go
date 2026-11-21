package proxy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
)

func TestAutomaticOutputSizingUsesNativeOfferingLimits(t *testing.T) {
	for _, expected := range []struct {
		provider, model string
		maximum         int
	}{
		{"cerebras", "qwen3.8-27b", 32768},
		{"dashscope", "qwen3.8-27b", 131072},
		{"openai", "gpt-6-luna", 128000},
		{"anthropic", "claude-haiku-5-5", 128000},
	} {
		t.Run(expected.provider, func(t *testing.T) {
			for _, provider := range testfixtures.ProviderCatalog(t).Schema().Providers {
				if provider.ID == expected.provider {
					for _, model := range provider.Offerings {
						if model.Model == expected.model {
							if model.OutputTokenLimit != expected.maximum {
								t.Errorf("catalog output ceiling=%d; want documented native %d", model.OutputTokenLimit, expected.maximum)
							}
							return
						}
					}
				}
			}
			t.Fatal("selected offering missing")
		})
	}
}

func TestAutomaticOutputSizingUsesCatalogCeilings(t *testing.T) {
	for _, route := range []struct{ provider, model, field, effort string }{
		{"openai", "gpt-6-luna", "max_output_tokens", "xhigh"},
		{"dashscope", "qwen3.8-flash", "max_output_tokens", "xhigh"},
		{"cerebras", "qwen3.8-27b", "max_completion_tokens", "high"},
		{"anthropic", "claude-haiku-5-5", "max_tokens", "xhigh"},
	} {
		t.Run(route.provider, func(t *testing.T) {
			schema := testfixtures.ProviderCatalog(t).Schema()
			for p := range schema.Providers {
				if schema.Providers[p].ID != route.provider {
					continue
				}
				for m := range schema.Providers[p].Offerings {
					model := &schema.Providers[p].Offerings[m]
					if model.Model != route.model {
						continue
					}
					model.OutputTokenLimit = 128000
					for c := range model.Controls {
						if model.Controls[c].ID == "max_tokens" {
							maximum := float64(128000)
							model.Controls[c].Maximum = &maximum
						}
					}
				}
			}
			catalog, err := proxy.NewProviderCatalog(schema)
			if err != nil {
				t.Fatal(err)
			}
			var maxima []int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodDelete {
					io.WriteString(w, `{}`)
					return
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				maximum, _ := body[route.field].(float64)
				maxima = append(maxima, int(maximum))
				const result = `{"answer":"complete"}`
				switch route.provider {
				case "anthropic":
					fmt.Fprintf(w, `{"content":[{"type":"text","text":%q}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":70000}}`, result)
				case "cerebras":
					fmt.Fprintf(w, `{"choices":[{"message":{"content":%q},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":70000,"total_tokens":70010}}`, result)
				default:
					fmt.Fprintf(w, `{"id":"resp_automatic","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":%q}]}],"usage":{"input_tokens":10,"output_tokens":70000,"total_tokens":70010}}`, result)
				}
			}))
			defer upstream.Close()
			tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
			tenant.Defaults = proxy.TenantDefaults{Provider: route.provider, Model: route.model}
			tenant.ProviderKeys = map[string]string{route.provider: "provider-fixture"}
			tenant.ProviderTextModels = map[string]string{route.provider: route.model}
			router, err := buildRouterWithManagedTenant(t, proxy.Configuration{ProviderCatalog: catalog, AssetStorePath: t.TempDir(), Endpoints: providerEndpointOverrides(map[string]string{route.provider: upstream.URL}, nil)}, zap.NewNop().Sugar(), tenant)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{Protocol: llmproxyclient.ProtocolOpenAIResponses, BaseURL: server.URL, Secret: TestSecret, Provider: route.provider})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(configuration, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			var total int
			for attempt := 0; attempt < 8; attempt++ {
				input := llmproxyclient.MessagesRequestInput{Model: route.model, ReasoningEffort: &route.effort, Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Return complete JSON."}}}
				request, err := llmproxyclient.NewMessagesRequest(input)
				if err != nil {
					t.Fatal(err)
				}
				result, err := client.PostMessagesCompletion(context.Background(), request)
				if err != nil {
					t.Fatal(err)
				}
				if result.Usage() == nil {
					t.Fatal("completed structured response lost native usage")
				}
				total += result.Usage().OutputTokens()
				if maxima[len(maxima)-1] != 128000 {
					t.Errorf("omitted limit dispatched %d; want catalog ceiling 128000", maxima[len(maxima)-1])
				}
			}
			if total != 560000 {
				t.Fatalf("aggregate output tokens=%d; want 560000", total)
			}
			structured, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Model: route.model, ReasoningEffort: &route.effort, Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Return complete JSON."}}, StructuredOutput: &llmproxyclient.StructuredOutputInput{JSONSchema: []byte(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`)}, IdempotencyKey: "automatic-structured"})
			if err != nil {
				t.Fatal(err)
			}
			nativeConfiguration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{Protocol: llmproxyclient.ProtocolNative, BaseURL: server.URL, Secret: TestSecret, Provider: route.provider})
			if err != nil {
				t.Fatal(err)
			}
			nativeClient, err := llmproxyclient.NewClient(nativeConfiguration, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := nativeClient.PostMessagesCompletion(context.Background(), structured); err != nil {
				t.Fatal(err)
			}
			if maxima[len(maxima)-1] != 128000 {
				t.Errorf("structured request dispatched %d; want catalog ceiling 128000", maxima[len(maxima)-1])
			}
		})
	}
}

func TestAutomaticOutputSizingPreservesUnknownUpstreamDefault(t *testing.T) {
	schema := testfixtures.ProviderCatalog(t).Schema()
	for p := range schema.Providers {
		if schema.Providers[p].ID != "openai" {
			continue
		}
		for m := range schema.Providers[p].Offerings {
			model := &schema.Providers[p].Offerings[m]
			if model.Model == "gpt-6-luna" {
				model.OutputTokenLimit = 0
			}
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			io.WriteString(w, `{}`)
			return
		}
		calls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if _, exists := body["max_output_tokens"]; exists {
			t.Error("Proxy invented an unknown output ceiling")
		}
		io.WriteString(w, `{"id":"resp_default","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Complete."}]}],"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}`)
	}))
	defer upstream.Close()
	tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenant.Defaults = proxy.TenantDefaults{Provider: "openai", Model: "gpt-6-luna"}
	tenant.ProviderKeys = map[string]string{"openai": "fixture-key"}
	tenant.ProviderTextModels = map[string]string{"openai": "gpt-6-luna"}
	router, err := buildRouterWithManagedTenant(t, proxy.Configuration{ProviderCatalog: catalog, Endpoints: providerEndpointOverrides(map[string]string{"openai": upstream.URL}, nil)}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{Protocol: llmproxyclient.ProtocolOpenAIResponses, BaseURL: server.URL, Secret: TestSecret, Provider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Model: "gpt-6-luna", Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Complete this."}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.PostMessagesCompletion(t.Context(), request)
	if err != nil || calls != 1 || result.Text() != "Complete." {
		t.Fatalf("result=%s calls=%d error=%v", result.Text(), calls, err)
	}
}
