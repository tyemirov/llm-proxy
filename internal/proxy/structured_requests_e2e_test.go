package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const structuredDecisionSchema = `{"type":"object","additionalProperties":false,"required":["decision"],"properties":{"decision":{"type":"string","enum":["pass","return"]}}}`

func TestStructuredRequestConvergesAcrossSubmitReplayAndReconciliation(testingInstance *testing.T) {
	var providerCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		providerCalls.Add(1)
		body, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			testingInstance.Fatal(readError)
		}
		var payload map[string]any
		if decodeError := json.Unmarshal(body, &payload); decodeError != nil {
			testingInstance.Fatalf("provider payload=%s error=%v", body, decodeError)
		}
		text, textOK := payload["text"].(map[string]any)
		format, formatOK := text["format"].(map[string]any)
		if !textOK || !formatOK || format["type"] != "json_schema" || format["strict"] != true || format["schema"] == nil {
			testingInstance.Fatalf("provider payload lacks enforced schema: %s", body)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"id":"structured","status":"completed","output_text":"{\"decision\":\"pass\"}"}`))
	}))
	testingInstance.Cleanup(upstream.Close)
	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(upstream.URL)
	router := coverageRouter(testingInstance, proxy.Configuration{
		Tenants: proxy.SingleTenantConfigurationsWithDefaults("structured", TestSecret, proxy.TenantDefaults{
			Provider: proxy.ProviderNameOpenAI, Model: proxy.ModelNameGPT55,
		}),
		OpenAIKey: "provider-key", WorkerCount: 1, QueueSize: 2, RequestTimeoutSeconds: TestTimeout,
		Endpoints: endpoints, AssetStorePath: testingInstance.TempDir(), AssetRetentionSeconds: 3600,
	})
	body := structuredV2Body("review", false, proxy.ModelNameGPT55)

	first := performStructuredRequest(testingInstance, router, body, "review:one")
	if first.Code != http.StatusOK || first.Body.String() != `{"decision":"pass"}` || first.Header().Get(llmproxycontract.HeaderStructuredRequestState) != "succeeded" {
		testingInstance.Fatalf("first status=%d body=%s headers=%v", first.Code, first.Body.String(), first.Header())
	}
	second := performStructuredRequest(testingInstance, router, body, "review:one")
	if second.Code != http.StatusOK || !jsonBodiesEqual(second.Body.Bytes(), []byte(`{"decision":"pass"}`)) || providerCalls.Load() != 1 {
		testingInstance.Fatalf("replay status=%d body=%s provider_calls=%d", second.Code, second.Body.String(), providerCalls.Load())
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/v2/requests?key="+TestSecret, nil)
	statusRequest.Header.Set(llmproxycontract.HeaderIdempotencyKey, "review:one")
	status := httptest.NewRecorder()
	router.ServeHTTP(status, statusRequest)
	if status.Code != http.StatusOK || !jsonBodiesEqual(status.Body.Bytes(), []byte(`{"decision":"pass"}`)) || providerCalls.Load() != 1 {
		testingInstance.Fatalf("status=%d body=%s provider_calls=%d", status.Code, status.Body.String(), providerCalls.Load())
	}

	conflict := performStructuredRequest(testingInstance, router, structuredV2Body("different", false, proxy.ModelNameGPT55), "review:one")
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "structured_request_intent_conflict") || providerCalls.Load() != 1 {
		testingInstance.Fatalf("conflict status=%d body=%s provider_calls=%d", conflict.Code, conflict.Body.String(), providerCalls.Load())
	}
}

func TestStructuredRequestRejectsInvalidContractsBeforeProviderDispatch(testingInstance *testing.T) {
	var providerCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		responseWriter.WriteHeader(http.StatusInternalServerError)
	}))
	testingInstance.Cleanup(upstream.Close)
	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(upstream.URL)
	router := coverageRouter(testingInstance, proxy.Configuration{
		Tenants:   proxy.SingleTenantConfigurationsWithDefaults("structured-invalid", TestSecret, proxy.TenantDefaults{Provider: "deepseek", Model: "deepseek-v4-flash"}),
		OpenAIKey: "provider-key", DeepSeekKey: "deepseek-key", DeepSeekBaseURL: upstream.URL,
		WorkerCount: 1, QueueSize: 2, RequestTimeoutSeconds: TestTimeout, Endpoints: endpoints,
		AssetStorePath: testingInstance.TempDir(), AssetRetentionSeconds: 3600,
	})

	testCases := []struct {
		name     string
		body     string
		key      string
		want     string
		provider string
	}{
		{name: "invalid schema", body: `{"messages":[{"role":"user","content":"review"}],"structured_output":{"schema":{"type":"missing"}}}`, key: "review:invalid", want: "invalid structured_output"},
		{name: "missing key", body: structuredV2Body("review", false, "deepseek-v4-flash"), want: "invalid Idempotency-Key"},
		{name: "key without schema", body: `{"messages":[{"role":"user","content":"review"}]}`, key: "review:extra", want: "invalid Idempotency-Key"},
		{name: "web search", body: structuredV2Body("review", true, proxy.ModelNameGPT55), key: "review:web", want: "structured_output does not support web_search", provider: proxy.ProviderNameOpenAI},
		{name: "unsupported wire", body: structuredV2Body("review", false, "deepseek-v4-flash"), key: "review:chat", want: "structured_output is unsupported"},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subtest *testing.T) {
			response := performStructuredRequestForProvider(subtest, router, testCase.body, testCase.key, testCase.provider)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), testCase.want) {
				subtest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	if providerCalls.Load() != 0 {
		testingInstance.Fatalf("provider calls=%d", providerCalls.Load())
	}
}

func TestStructuredRequestPersistsProviderSchemaFailure(testingInstance *testing.T) {
	var providerCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"id":"structured-invalid","status":"completed","output_text":"{\"decision\":\"unknown\"}"}`))
	}))
	testingInstance.Cleanup(upstream.Close)
	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(upstream.URL)
	router := coverageRouter(testingInstance, proxy.Configuration{
		Tenants:   proxy.SingleTenantConfigurationsWithDefaults("structured-failure", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.ModelNameGPT55}),
		OpenAIKey: "provider-key", WorkerCount: 1, QueueSize: 2, RequestTimeoutSeconds: TestTimeout,
		Endpoints: endpoints, AssetStorePath: testingInstance.TempDir(), AssetRetentionSeconds: 3600,
	})
	body := structuredV2Body("review", false, proxy.ModelNameGPT55)
	first := performStructuredRequest(testingInstance, router, body, "review:failure")
	if first.Code != http.StatusBadGateway || providerCalls.Load() != 1 {
		testingInstance.Fatalf("first status=%d body=%s calls=%d", first.Code, first.Body.String(), providerCalls.Load())
	}
	statusRequest := httptest.NewRequest(http.MethodGet, "/v2/requests?key="+TestSecret, nil)
	statusRequest.Header.Set(llmproxycontract.HeaderIdempotencyKey, "review:failure")
	status := httptest.NewRecorder()
	router.ServeHTTP(status, statusRequest)
	if status.Code != http.StatusBadGateway || !strings.Contains(status.Body.String(), "structured_request_failed") || providerCalls.Load() != 1 {
		testingInstance.Fatalf("reconciliation status=%d body=%s calls=%d", status.Code, status.Body.String(), providerCalls.Load())
	}
	second := performStructuredRequest(testingInstance, router, body, "review:failure")
	if second.Code != http.StatusBadGateway || providerCalls.Load() != 2 {
		testingInstance.Fatalf("retry status=%d body=%s calls=%d", second.Code, second.Body.String(), providerCalls.Load())
	}
}

func TestBuildRouterRejectsUnsafeStructuredRequestStore(testingInstance *testing.T) {
	assetRoot := testingInstance.TempDir()
	if writeError := os.WriteFile(filepath.Join(assetRoot, "structured-requests"), []byte("unsafe"), 0o600); writeError != nil {
		testingInstance.Fatal(writeError)
	}
	_, buildError := proxy.BuildRouter(withModelCatalog(testingInstance, proxy.Configuration{
		Tenants: proxy.SingleTenantConfigurations("unsafe-structured-store", TestSecret), OpenAIKey: "provider-key",
		WorkerCount: 1, QueueSize: 1, RequestTimeoutSeconds: TestTimeout, AssetStorePath: assetRoot,
	}), coverageLogger())
	if buildError == nil {
		testingInstance.Fatal("unsafe structured request store must fail router construction")
	}
}

func structuredV2Body(content string, webSearch bool, model string) string {
	payload := map[string]any{
		"messages": []map[string]string{{"role": "user", "content": content}},
		"model":    model, "web_search": webSearch,
		"structured_output": map[string]any{"schema": json.RawMessage(structuredDecisionSchema)},
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func performStructuredRequest(testingInstance testing.TB, router http.Handler, body string, idempotencyKey string) *httptest.ResponseRecorder {
	return performStructuredRequestForProvider(testingInstance, router, body, idempotencyKey, "")
}

func performStructuredRequestForProvider(testingInstance testing.TB, router http.Handler, body string, idempotencyKey string, provider string) *httptest.ResponseRecorder {
	testingInstance.Helper()
	requestPath := "/v2?key=" + TestSecret
	if provider != "" {
		requestPath += "&provider=" + provider
	}
	request := httptest.NewRequest(http.MethodPost, requestPath, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, idempotencyKey)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func jsonBodiesEqual(first []byte, second []byte) bool {
	var firstValue any
	var secondValue any
	return json.Unmarshal(first, &firstValue) == nil && json.Unmarshal(second, &secondValue) == nil && reflect.DeepEqual(firstValue, secondValue)
}
