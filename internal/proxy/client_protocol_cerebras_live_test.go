package proxy_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

// TestCerebrasLiveProviderConfiguration exercises the canonical catalog through
// a real local Proxy. Only that Proxy's managed connection receives the provider
// credential; the official consumer receives a disposable tenant key.
func TestCerebrasLiveProviderConfiguration(t *testing.T) {
	if os.Getenv("LLM_PROXY_LIVE_CEREBRAS") != "true" {
		t.Skip("set LLM_PROXY_LIVE_CEREBRAS=true for the paid Cerebras test")
	}
	key := os.Getenv("CEREBRAS_API_KEY")
	if key == "" {
		t.Fatal("CEREBRAS_API_KEY is required in the Proxy test environment")
	}
	tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenant.Defaults = proxy.TenantDefaults{Provider: "cerebras", Model: cerebrasQualifiedModel}
	tenant.ProviderKeys = map[string]string{"cerebras": key}
	tenant.ProviderTextModels = map[string]string{"cerebras": cerebrasQualifiedModel}
	router, err := buildRouterWithManagedTenant(t, proxy.Configuration{
		AssetStorePath:  t.TempDir(),
		ProviderCatalog: testfixtures.ProviderCatalog(t),
	}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatalf("construct Proxy with canonical Cerebras configuration: %v", err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	observer := &cerebrasLiveResponseObserver{next: server.Client().Transport}
	consumer := server.Client()
	consumer.Transport = observer
	configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: TestSecret, Provider: "cerebras"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(configuration, consumer)
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := client.GetPublicCapabilities(context.Background())
	if err != nil {
		t.Fatalf("discover Cerebras through Proxy: %v", err)
	}
	found := false
	for _, offering := range capabilities.Offerings {
		if offering.Provider == "cerebras" && offering.Model == cerebrasQualifiedModel {
			found = true
		}
	}
	if !found {
		t.Fatal("canonical catalog does not expose cerebras/qwen3.8-27b")
	}
	for _, scenario := range []struct {
		name, effort, prompt string
		maxTokens            int
		schema               []byte
	}{
		{name: "plain_text", effort: "none", prompt: "Reply with exactly CEREBRAS_OK and nothing else.", maxTokens: 256},
		{name: "strict_json", effort: "low", prompt: "Return a JSON object whose answer is CEREBRAS_OK.", maxTokens: 4096, schema: []byte(`{"type":"object","properties":{"answer":{"type":"string","enum":["CEREBRAS_OK"]}},"required":["answer"],"additionalProperties":false}`)},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			workSeconds := 180
			input := llmproxyclient.MessagesRequestInput{
				Model: cerebrasQualifiedModel, MaxTokens: &scenario.maxTokens, ReasoningEffort: &scenario.effort,
				Messages:              []llmproxyclient.MessageInput{{Role: "user", Content: scenario.prompt}},
				RequestTimeoutSeconds: &workSeconds,
			}
			if scenario.schema != nil {
				input.StructuredOutput = &llmproxyclient.StructuredOutputInput{JSONSchema: scenario.schema}
				input.IdempotencyKey = "cerebras-live:strict-json"
			}
			request, err := llmproxyclient.NewMessagesRequest(input)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 220*time.Second)
			defer cancel()
			started := time.Now()
			answer, err := client.PostMessages(ctx, request)
			if err != nil {
				t.Fatalf("Cerebras generation through Proxy failed: %v", err)
			}
			if scenario.schema == nil {
				if strings.TrimSpace(answer) != "CEREBRAS_OK" {
					t.Fatalf("unexpected plain answer: %q", answer)
				}
			} else {
				var result map[string]string
				if err := json.Unmarshal([]byte(answer), &result); err != nil || len(result) != 1 || result["answer"] != "CEREBRAS_OK" {
					t.Fatalf("strict JSON answer failed its contract: %q", answer)
				}
			}
			headers := observer.headers.Clone()
			requestTokens := cerebrasLivePositiveHeader(t, headers, testHeaderLLMProxyRequestTokens)
			responseTokens := cerebrasLivePositiveHeader(t, headers, testHeaderLLMProxyResponseTokens)
			totalTokens := cerebrasLivePositiveHeader(t, headers, testHeaderLLMProxyTotalTokens)
			if totalTokens != requestTokens+responseTokens {
				t.Fatalf("inconsistent usage: input=%d output=%d total=%d", requestTokens, responseTokens, totalTokens)
			}
			requestID := headers.Get(llmproxycontract.HeaderRequestID)
			if requestID == "" {
				t.Fatal("Proxy request ID is missing")
			}
			t.Logf("provider=cerebras model=%s case=%s input=%d output=%d total=%d elapsed_ms=%d", cerebrasQualifiedModel, scenario.name, requestTokens, responseTokens, totalTokens, time.Since(started).Milliseconds())
			if scenario.schema != nil {
				replayed, replayError := client.PostMessages(ctx, request)
				if replayError != nil {
					t.Fatalf("replay retained structured answer: %v", replayError)
				}
				if !jsonBodiesEqual([]byte(replayed), []byte(answer)) {
					t.Fatalf("structured replay changed the JSON value: first=%q replay=%q", answer, replayed)
				}
				if state := observer.headers.Get(llmproxycontract.HeaderStructuredRequestState); state != "succeeded" {
					t.Fatalf("structured replay state=%q", state)
				}
				if observer.headers.Get(llmproxycontract.HeaderRequestID) == "" {
					t.Fatal("replay HTTP request ID is missing")
				}
			}
		})
	}
}

type cerebrasLiveResponseObserver struct {
	next    http.RoundTripper
	headers http.Header
}

func (observer *cerebrasLiveResponseObserver) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := observer.next.RoundTrip(request)
	if err == nil {
		observer.headers = response.Header.Clone()
	}
	return response, err
}

func cerebrasLivePositiveHeader(t *testing.T, headers http.Header, name string) int {
	t.Helper()
	values := headers.Values(name)
	if len(values) != 1 {
		t.Fatalf("usage header %s has %d values", name, len(values))
	}
	value, err := strconv.Atoi(values[0])
	if err != nil || value <= 0 {
		t.Fatalf("usage header %s is not a positive count", name)
	}
	return value
}
