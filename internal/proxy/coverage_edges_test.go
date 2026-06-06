package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

type coverageRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper coverageRoundTripper) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	return roundTripper(httpRequest)
}

type coverageHTTPDoer func(*http.Request) (*http.Response, error)

const (
	testHeaderLLMProxyRequestTokens  = "X-LLM-Proxy-Request-Tokens"
	testHeaderLLMProxyResponseTokens = "X-LLM-Proxy-Response-Tokens"
	testHeaderLLMProxyTotalTokens    = "X-LLM-Proxy-Total-Tokens"
	coverageShortRequestTimeout      = 25 * time.Millisecond
)

func (httpDoer coverageHTTPDoer) Do(httpRequest *http.Request) (*http.Response, error) {
	return httpDoer(httpRequest)
}

func coverageHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func coverageLogger() *zap.SugaredLogger {
	return zap.NewNop().Sugar()
}

func requireUpstreamFailureStatus(t *testing.T, statusCode int) {
	t.Helper()
	if statusCode != http.StatusBadGateway && statusCode != http.StatusGatewayTimeout {
		t.Fatalf("status=%d want one of %d,%d", statusCode, http.StatusBadGateway, http.StatusGatewayTimeout)
	}
}

func coverageRouter(t *testing.T, configuration proxy.Configuration) *gin.Engine {
	t.Helper()
	router, buildError := proxy.BuildRouter(configuration, coverageLogger())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}
	return router
}

func textRouterWithResponsesHandler(t *testing.T, handler http.HandlerFunc) *gin.Engine {
	t.Helper()
	upstreamServer := httptest.NewServer(handler)
	t.Cleanup(upstreamServer.Close)
	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(upstreamServer.URL)
	return coverageRouter(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  2,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
		Endpoints:                  endpoints,
	})
}

func performCoverageTextRequest(t *testing.T, router http.Handler, queryParameters url.Values, acceptHeader string) (int, string, string) {
	t.Helper()
	if queryParameters.Get("key") == "" {
		queryParameters.Set("key", TestSecret)
	}
	if queryParameters.Get("prompt") == "" {
		queryParameters.Set("prompt", TestPrompt)
	}
	request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
	if acceptHeader != "" {
		request.Header.Set("Accept", acceptHeader)
	}
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, request)
	return responseRecorder.Code, responseRecorder.Body.String(), responseRecorder.Header().Get("Content-Type")
}

func performCoverageTextRequestWithTimeout(t *testing.T, router http.Handler, queryParameters url.Values, acceptHeader string, timeoutDuration time.Duration) (int, string, string) {
	t.Helper()
	if queryParameters.Get("key") == "" {
		queryParameters.Set("key", TestSecret)
	}
	if queryParameters.Get("prompt") == "" {
		queryParameters.Set("prompt", TestPrompt)
	}
	request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
	if acceptHeader != "" {
		request.Header.Set("Accept", acceptHeader)
	}
	requestContext, cancelRequest := context.WithTimeout(request.Context(), timeoutDuration)
	defer cancelRequest()
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, request.WithContext(requestContext))
	return responseRecorder.Code, responseRecorder.Body.String(), responseRecorder.Header().Get("Content-Type")
}

func TestCoverageFormatsAndRequestEdges(t *testing.T) {
	router := textRouterWithResponsesHandler(t, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"output_text":"formatted \"answer\""}`))
	})

	t.Run("json format query overrides accept header", func(subTest *testing.T) {
		queryParameters := url.Values{}
		queryParameters.Set("format", " application/json ")
		statusCode, body, contentType := performCoverageTextRequest(subTest, router, queryParameters, "text/csv")
		if statusCode != http.StatusOK {
			subTest.Fatalf("status=%d body=%s", statusCode, body)
		}
		if contentType != "application/json" {
			subTest.Fatalf("content-type=%q", contentType)
		}
		var response map[string]string
		if decodeError := json.Unmarshal([]byte(body), &response); decodeError != nil {
			subTest.Fatalf("decode json: %v", decodeError)
		}
		if response["response"] != `formatted "answer"` {
			subTest.Fatalf("response=%q", response["response"])
		}
	})

	t.Run("xml accept header", func(subTest *testing.T) {
		queryParameters := url.Values{}
		statusCode, body, contentType := performCoverageTextRequest(subTest, router, queryParameters, "text/xml")
		if statusCode != http.StatusOK {
			subTest.Fatalf("status=%d body=%s", statusCode, body)
		}
		if contentType != "application/xml" {
			subTest.Fatalf("content-type=%q", contentType)
		}
		if !strings.Contains(body, `request="hello"`) || !strings.Contains(body, `formatted &#34;answer&#34;`) {
			subTest.Fatalf("body=%q", body)
		}
	})

	t.Run("default text response", func(subTest *testing.T) {
		queryParameters := url.Values{}
		statusCode, body, contentType := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK {
			subTest.Fatalf("status=%d body=%s", statusCode, body)
		}
		if contentType != "text/plain; charset=utf-8" {
			subTest.Fatalf("content-type=%q", contentType)
		}
		if body != `formatted "answer"` {
			subTest.Fatalf("body=%q", body)
		}
	})

	t.Run("json format includes OpenAI token usage and headers", func(subTest *testing.T) {
		usageRouter := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"output_text":"token answer","usage":{"input_tokens":7,"output_tokens":5,"total_tokens":12}}`))
		})
		queryParameters := url.Values{}
		queryParameters.Set("format", "application/json")
		queryParameters.Set("key", TestSecret)
		queryParameters.Set("prompt", TestPrompt)
		request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
		responseRecorder := httptest.NewRecorder()
		usageRouter.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusOK {
			subTest.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens) != "7" {
			subTest.Fatalf("request tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens))
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens) != "5" {
			subTest.Fatalf("response tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens))
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens) != "12" {
			subTest.Fatalf("total tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens))
		}
		var response struct {
			Response string `json:"response"`
			Usage    struct {
				RequestTokens  int `json:"request_tokens"`
				ResponseTokens int `json:"response_tokens"`
				TotalTokens    int `json:"total_tokens"`
			} `json:"usage"`
		}
		if decodeError := json.Unmarshal(responseRecorder.Body.Bytes(), &response); decodeError != nil {
			subTest.Fatalf("decode json: %v", decodeError)
		}
		if response.Response != "token answer" || response.Usage.RequestTokens != 7 || response.Usage.ResponseTokens != 5 || response.Usage.TotalTokens != 12 {
			subTest.Fatalf("response=%+v", response)
		}
	})

	t.Run("json format omits empty OpenAI token usage", func(subTest *testing.T) {
		usageRouter := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"output_text":"empty usage answer","usage":{}}`))
		})
		queryParameters := url.Values{}
		queryParameters.Set("format", "application/json")
		queryParameters.Set("key", TestSecret)
		queryParameters.Set("prompt", TestPrompt)
		request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
		responseRecorder := httptest.NewRecorder()
		usageRouter.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusOK {
			subTest.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens) != "" {
			subTest.Fatalf("request tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens))
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens) != "" {
			subTest.Fatalf("response tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens))
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens) != "" {
			subTest.Fatalf("total tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens))
		}
		var response map[string]any
		if decodeError := json.Unmarshal(responseRecorder.Body.Bytes(), &response); decodeError != nil {
			subTest.Fatalf("decode json: %v", decodeError)
		}
		if _, found := response["usage"]; found {
			subTest.Fatalf("usage must be omitted: %v", response)
		}
	})

	t.Run("invalid web search value is ignored as false", func(subTest *testing.T) {
		queryParameters := url.Values{}
		queryParameters.Set("web_search", "maybe")
		statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK {
			subTest.Fatalf("status=%d body=%s", statusCode, body)
		}
	})

	t.Run("false web search value is accepted", func(subTest *testing.T) {
		queryParameters := url.Values{}
		queryParameters.Set("web_search", "0")
		statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK {
			subTest.Fatalf("status=%d body=%s", statusCode, body)
		}
	})

	t.Run("json body validates malformed and missing prompt requests", func(subTest *testing.T) {
		for _, requestBody := range []string{`{`, `{"model":"` + proxy.ModelNameGPT41 + `"}`} {
			request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret, strings.NewReader(requestBody))
			request.Header.Set("Content-Type", "application/json")
			responseRecorder := httptest.NewRecorder()
			router.ServeHTTP(responseRecorder, request)
			if responseRecorder.Code != http.StatusBadRequest {
				subTest.Fatalf("body=%s status=%d", requestBody, responseRecorder.Code)
			}
		}
	})

	t.Run("json body accepts query model override and default system prompt", func(subTest *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret+"&model="+proxy.ModelNameGPT41, strings.NewReader(`{"prompt":"hello json"}`))
		request.Header.Set("Content-Type", "application/json")
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusOK {
			subTest.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
		}
	})

	t.Run("json body reports provider validation errors", func(subTest *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret+"&provider=unknown", strings.NewReader(`{"prompt":"hello json"}`))
		request.Header.Set("Content-Type", "application/json")
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusBadRequest {
			subTest.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
		}
	})

	t.Run("request context deadline adjusts enqueue timeout", func(subTest *testing.T) {
		queryParameters := url.Values{}
		queryParameters.Set("key", TestSecret)
		queryParameters.Set("prompt", TestPrompt)
		request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
		deadlineContext, cancel := context.WithDeadline(request.Context(), time.Now().Add(time.Second))
		defer cancel()
		request = request.WithContext(deadlineContext)
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusOK {
			subTest.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
		}
	})
}

func TestCoverageOpenAIResponsesMaxTokensContract(t *testing.T) {
	t.Run("default request omits max_output_tokens", func(subTest *testing.T) {
		var capturedPayload map[string]any
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			requestBytes, _ := io.ReadAll(httpRequest.Body)
			if decodeError := json.Unmarshal(requestBytes, &capturedPayload); decodeError != nil {
				subTest.Fatalf("decode upstream request: %v", decodeError)
			}
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"output_text":"no cap"}`))
		})
		queryParameters := url.Values{}
		statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK || body != "no cap" {
			subTest.Fatalf("status=%d body=%q", statusCode, body)
		}
		if _, exists := capturedPayload["max_output_tokens"]; exists {
			subTest.Fatalf("max_output_tokens must be omitted by default: %v", capturedPayload)
		}
	})

	t.Run("query max_tokens maps to max_output_tokens", func(subTest *testing.T) {
		var capturedPayload map[string]any
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			requestBytes, _ := io.ReadAll(httpRequest.Body)
			if decodeError := json.Unmarshal(requestBytes, &capturedPayload); decodeError != nil {
				subTest.Fatalf("decode upstream request: %v", decodeError)
			}
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"output_text":"query cap"}`))
		})
		queryParameters := url.Values{}
		queryParameters.Set("max_tokens", "777")
		statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK || body != "query cap" {
			subTest.Fatalf("status=%d body=%q", statusCode, body)
		}
		if capturedPayload["max_output_tokens"] != float64(777) {
			subTest.Fatalf("max_output_tokens=%v payload=%v", capturedPayload["max_output_tokens"], capturedPayload)
		}
	})

	t.Run("json body max_tokens maps to max_output_tokens", func(subTest *testing.T) {
		var capturedPayload map[string]any
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			requestBytes, _ := io.ReadAll(httpRequest.Body)
			if decodeError := json.Unmarshal(requestBytes, &capturedPayload); decodeError != nil {
				subTest.Fatalf("decode upstream request: %v", decodeError)
			}
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"output_text":"json cap"}`))
		})
		request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret, strings.NewReader(`{"prompt":"hello json","max_tokens":333}`))
		request.Header.Set("Content-Type", "application/json")
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusOK || responseRecorder.Body.String() != "json cap" {
			subTest.Fatalf("status=%d body=%q", responseRecorder.Code, responseRecorder.Body.String())
		}
		if capturedPayload["max_output_tokens"] != float64(333) {
			subTest.Fatalf("max_output_tokens=%v payload=%v", capturedPayload["max_output_tokens"], capturedPayload)
		}
	})

	t.Run("invalid query max_tokens is rejected before upstream", func(subTest *testing.T) {
		upstreamCalled := false
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			upstreamCalled = true
			responseWriter.WriteHeader(http.StatusInternalServerError)
		})
		for _, rawValue := range []string{"0", "-1", "abc"} {
			queryParameters := url.Values{}
			queryParameters.Set("max_tokens", rawValue)
			statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
			if statusCode != http.StatusBadRequest || !strings.Contains(body, "invalid max_tokens parameter") {
				subTest.Fatalf("max_tokens=%q status=%d body=%q", rawValue, statusCode, body)
			}
		}
		if upstreamCalled {
			subTest.Fatal("upstream must not be called for invalid query max_tokens")
		}
	})

	t.Run("invalid json max_tokens is rejected before upstream", func(subTest *testing.T) {
		upstreamCalled := false
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			upstreamCalled = true
			responseWriter.WriteHeader(http.StatusInternalServerError)
		})
		request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret, strings.NewReader(`{"prompt":"hello json","max_tokens":0}`))
		request.Header.Set("Content-Type", "application/json")
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusBadRequest || !strings.Contains(responseRecorder.Body.String(), "invalid max_tokens parameter") {
			subTest.Fatalf("status=%d body=%q", responseRecorder.Code, responseRecorder.Body.String())
		}
		if upstreamCalled {
			subTest.Fatal("upstream must not be called for invalid json max_tokens")
		}
	})
}

func TestCoverageConfigurationValidationMatrix(t *testing.T) {
	testCases := []struct {
		name          string
		configuration proxy.Configuration
		expectedError string
	}{
		{
			name:          "missing tenants",
			configuration: proxy.Configuration{OpenAIKey: TestAPIKey},
			expectedError: "tenants must include at least one tenant",
		},
		{
			name:          "missing openai text credential",
			configuration: proxy.Configuration{Tenants: proxy.SingleTenantConfigurations("test", TestSecret)},
			expectedError: "provider not configured: provider=openai endpoint=text",
		},
		{
			name: "duplicate tenant id",
			configuration: proxy.Configuration{
				Tenants: []proxy.TenantConfiguration{
					proxy.DefaultTenantConfiguration("duplicate", "secret-one"),
					proxy.DefaultTenantConfiguration("duplicate", "secret-two"),
				},
				OpenAIKey: TestAPIKey,
			},
			expectedError: "duplicate id=duplicate",
		},
		{
			name: "missing tenant id",
			configuration: proxy.Configuration{
				Tenants: []proxy.TenantConfiguration{
					proxy.DefaultTenantConfiguration("", "tenant-secret"),
				},
				OpenAIKey: TestAPIKey,
			},
			expectedError: "id must be set",
		},
		{
			name: "missing tenant secret",
			configuration: proxy.Configuration{
				Tenants: []proxy.TenantConfiguration{
					proxy.DefaultTenantConfiguration("tenant", ""),
				},
				OpenAIKey: TestAPIKey,
			},
			expectedError: "secret must be set",
		},
		{
			name: "duplicate tenant secret",
			configuration: proxy.Configuration{
				Tenants: []proxy.TenantConfiguration{
					proxy.DefaultTenantConfiguration("tenant-a", "shared-secret"),
					proxy.DefaultTenantConfiguration("tenant-b", "shared-secret"),
				},
				OpenAIKey: TestAPIKey,
			},
			expectedError: "duplicate secret tenant=tenant-b existing_tenant=tenant-a",
		},
		{
			name: "missing deepseek text credential",
			configuration: proxy.Configuration{
				Tenants:   proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameDeepSeek, Model: proxy.ModelNameDeepSeekV4Flash, DictationProvider: proxy.ProviderNameOpenAI, DictationModel: proxy.DefaultDictationModel}),
				OpenAIKey: TestAPIKey,
			},
			expectedError: "provider not configured: provider=deepseek",
		},
		{
			name: "missing dashscope text credential from alias",
			configuration: proxy.Configuration{
				Tenants:   proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: "qwen", Model: proxy.ModelNameDashScopeQwenPlus, DictationProvider: proxy.ProviderNameOpenAI, DictationModel: proxy.DefaultDictationModel}),
				OpenAIKey: TestAPIKey,
			},
			expectedError: "provider not configured: provider=dashscope",
		},
		{
			name: "missing moonshot text credential from alias",
			configuration: proxy.Configuration{
				Tenants:   proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: "kimi", Model: proxy.ModelNameMoonshotKimi, DictationProvider: proxy.ProviderNameOpenAI, DictationModel: proxy.DefaultDictationModel}),
				OpenAIKey: TestAPIKey,
			},
			expectedError: "provider not configured: provider=moonshot",
		},
		{
			name: "missing siliconflow text credential",
			configuration: proxy.Configuration{
				Tenants:   proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameSiliconFlow, Model: proxy.ModelNameSiliconFlowDeepSeek, DictationProvider: proxy.ProviderNameOpenAI, DictationModel: proxy.DefaultDictationModel}),
				OpenAIKey: TestAPIKey,
			},
			expectedError: "provider not configured: provider=siliconflow",
		},
		{
			name: "missing zhipu text credential from alias",
			configuration: proxy.Configuration{
				Tenants:   proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: "glm", Model: proxy.ModelNameZhipuGLM, DictationProvider: proxy.ProviderNameOpenAI, DictationModel: proxy.DefaultDictationModel}),
				OpenAIKey: TestAPIKey,
			},
			expectedError: "provider not configured: provider=zhipu",
		},
		{
			name: "unknown default text provider",
			configuration: proxy.Configuration{
				Tenants:   proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: "unknown", Model: proxy.DefaultModel, DictationProvider: proxy.ProviderNameOpenAI, DictationModel: proxy.DefaultDictationModel}),
				OpenAIKey: TestAPIKey,
			},
			expectedError: "unknown provider: unknown",
		},
		{
			name: "missing openai dictation credential",
			configuration: proxy.Configuration{
				Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameDeepSeek, Model: proxy.ModelNameDeepSeekV4Flash, DictationProvider: proxy.ProviderNameOpenAI, DictationModel: proxy.DefaultDictationModel}),
				DeepSeekKey:                testDeepSeekKey,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			},
			expectedError: "provider not configured: provider=openai endpoint=dictation",
		},
		{
			name: "unsupported qwen default dictation",
			configuration: proxy.Configuration{
				Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.DefaultModel, DictationProvider: "qwen"}),
				OpenAIKey:                  TestAPIKey,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			},
			expectedError: "unsupported provider endpoint: provider=dashscope endpoint=dictation",
		},
		{
			name: "unsupported kimi default dictation",
			configuration: proxy.Configuration{
				Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.DefaultModel, DictationProvider: "kimi"}),
				OpenAIKey:                  TestAPIKey,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			},
			expectedError: "unsupported provider endpoint: provider=moonshot endpoint=dictation",
		},
		{
			name: "unsupported glm default dictation",
			configuration: proxy.Configuration{
				Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.DefaultModel, DictationProvider: "glm"}),
				OpenAIKey:                  TestAPIKey,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			},
			expectedError: "unsupported provider endpoint: provider=zhipu endpoint=dictation",
		},
		{
			name: "unknown default dictation provider",
			configuration: proxy.Configuration{
				Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.DefaultModel, DictationProvider: "unknown"}),
				OpenAIKey:                  TestAPIKey,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			},
			expectedError: "unknown provider: unknown",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			_, buildError := proxy.BuildRouter(testCase.configuration, coverageLogger())
			if buildError == nil || !strings.Contains(buildError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want contains %q", buildError, testCase.expectedError)
			}
		})
	}
}

func TestCoverageOpenAILifecycleBranches(t *testing.T) {
	t.Run("terminal message returns without polling", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"id":"done","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"text","text":"direct final"}]}]}`))
		})
		queryParameters := url.Values{}
		statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK || body != "direct final" {
			subTest.Fatalf("status=%d body=%q", statusCode, body)
		}
	})

	t.Run("terminal response negative token usage maps to bad gateway", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"id":"done","status":"completed","output_text":"bad usage","usage":{"input_tokens":-1,"output_tokens":1,"total_tokens":0}}`))
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("terminal message joins multiple text parts", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"id":"done","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"text","text":"first"},{"type":"output_text","text":"second"}]}]}`))
		})
		queryParameters := url.Values{}
		statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK || body != "first\nsecond" {
			subTest.Fatalf("status=%d body=%q", statusCode, body)
		}
	})

	t.Run("initial incomplete response with text returns text", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"id":"partial","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial answer"}]}]}`))
		})
		queryParameters := url.Values{}
		statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK || body != "partial answer" {
			subTest.Fatalf("status=%d body=%q", statusCode, body)
		}
	})

	t.Run("initial incomplete response without details maps to bad gateway", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"id":"partial","status":"incomplete","output":[]}`))
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("initial incomplete response with unsupported reason maps to bad gateway", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"id":"partial","status":"incomplete","incomplete_details":{"reason":"content_filter"},"output":[]}`))
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("initial incomplete continuation failure maps to bad gateway", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				requestBytes, _ := io.ReadAll(httpRequest.Body)
				var requestPayload map[string]any
				_ = json.Unmarshal(requestBytes, &requestPayload)
				if requestPayload["previous_response_id"] == nil {
					_, _ = responseWriter.Write([]byte(`{"id":"partial","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`))
					return
				}
				responseWriter.WriteHeader(http.StatusBadRequest)
				_, _ = responseWriter.Write([]byte(`{"error":"continuation rejected"}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("initial incomplete continuation poll failure maps to bad gateway", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				requestBytes, _ := io.ReadAll(httpRequest.Body)
				var requestPayload map[string]any
				_ = json.Unmarshal(requestBytes, &requestPayload)
				if requestPayload["previous_response_id"] == nil {
					_, _ = responseWriter.Write([]byte(`{"id":"partial","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`))
					return
				}
				_, _ = responseWriter.Write([]byte(`{"id":"continued","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/continued":
				_, _ = responseWriter.Write([]byte(`{"status":"failed"}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("initial incomplete continuation aggregates token usage", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				requestBytes, _ := io.ReadAll(httpRequest.Body)
				var requestPayload map[string]any
				_ = json.Unmarshal(requestBytes, &requestPayload)
				if requestPayload["previous_response_id"] == nil {
					_, _ = responseWriter.Write([]byte(`{"id":"partial","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`))
					return
				}
				_, _ = responseWriter.Write([]byte(`{"id":"continued","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/continued":
				_, _ = responseWriter.Write([]byte(`{"status":"completed","output_text":"usage continued","usage":{"input_tokens":4,"output_tokens":5,"total_tokens":9}}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		queryParameters.Set("key", TestSecret)
		queryParameters.Set("prompt", TestPrompt)
		request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusOK || responseRecorder.Body.String() != "usage continued" {
			subTest.Fatalf("status=%d body=%q", responseRecorder.Code, responseRecorder.Body.String())
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens) != "5" {
			subTest.Fatalf("request tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens))
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens) != "7" {
			subTest.Fatalf("response tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens))
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens) != "12" {
			subTest.Fatalf("total tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens))
		}
	})

	t.Run("initial incomplete continuation keeps initial token usage without final usage", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				requestBytes, _ := io.ReadAll(httpRequest.Body)
				var requestPayload map[string]any
				_ = json.Unmarshal(requestBytes, &requestPayload)
				if requestPayload["previous_response_id"] == nil {
					_, _ = responseWriter.Write([]byte(`{"id":"partial","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[],"usage":{"input_tokens":6,"output_tokens":1,"total_tokens":7}}`))
					return
				}
				_, _ = responseWriter.Write([]byte(`{"id":"continued","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/continued":
				_, _ = responseWriter.Write([]byte(`{"status":"completed","output_text":"usage initial"}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		queryParameters.Set("key", TestSecret)
		queryParameters.Set("prompt", TestPrompt)
		request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusOK || responseRecorder.Body.String() != "usage initial" {
			subTest.Fatalf("status=%d body=%q", responseRecorder.Code, responseRecorder.Body.String())
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens) != "6" {
			subTest.Fatalf("request tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens))
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens) != "1" {
			subTest.Fatalf("response tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens))
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens) != "7" {
			subTest.Fatalf("total tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens))
		}
	})

	t.Run("completed without final message starts synthesis continuation", func(subTest *testing.T) {
		var synthesisPayload map[string]any
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				requestBytes, _ := io.ReadAll(httpRequest.Body)
				var requestPayload map[string]any
				_ = json.Unmarshal(requestBytes, &requestPayload)
				if requestPayload["previous_response_id"] == nil {
					_, _ = responseWriter.Write([]byte(`{"id":"needs_synthesis","status":"completed","output":[{"type":"web_search_call","action":{"query":"weather"}}]}`))
					return
				}
				synthesisPayload = requestPayload
				_, _ = responseWriter.Write([]byte(`{"id":"synthesis","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/synthesis":
				_, _ = responseWriter.Write([]byte(`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"synthesized"}]}]}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		queryParameters.Set("max_tokens", "222")
		statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK || body != "synthesized" {
			subTest.Fatalf("status=%d body=%q", statusCode, body)
		}
		if synthesisPayload["max_output_tokens"] != float64(222) {
			subTest.Fatalf("synthesis max_output_tokens=%v payload=%v", synthesisPayload["max_output_tokens"], synthesisPayload)
		}
	})

	t.Run("completed output object still starts synthesis continuation", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				requestBytes, _ := io.ReadAll(httpRequest.Body)
				var requestPayload map[string]any
				_ = json.Unmarshal(requestBytes, &requestPayload)
				if requestPayload["previous_response_id"] == nil {
					_, _ = responseWriter.Write([]byte(`{"id":"object_output","status":"completed","output":{}}`))
					return
				}
				_, _ = responseWriter.Write([]byte(`{"id":"object_synthesis","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/object_synthesis":
				_, _ = responseWriter.Write([]byte(`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"text","text":"object synthesized"}]}]}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK || body != "object synthesized" {
			subTest.Fatalf("status=%d body=%q", statusCode, body)
		}
	})

	t.Run("queued response poll failure maps to bad gateway", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				_, _ = responseWriter.Write([]byte(`{"id":"queued","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/queued":
				responseWriter.WriteHeader(http.StatusBadRequest)
				_, _ = responseWriter.Write([]byte(`{"error":"poll rejected"}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("queued response surfaces final poll token usage", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				_, _ = responseWriter.Write([]byte(`{"id":"queued","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/queued":
				_, _ = responseWriter.Write([]byte(`{"status":"completed","output_text":"poll usage","usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11}}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		queryParameters.Set("key", TestSecret)
		queryParameters.Set("prompt", TestPrompt)
		request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusOK || responseRecorder.Body.String() != "poll usage" {
			subTest.Fatalf("status=%d body=%q", responseRecorder.Code, responseRecorder.Body.String())
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens) != "8" {
			subTest.Fatalf("request tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens))
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens) != "3" {
			subTest.Fatalf("response tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens))
		}
		if responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens) != "11" {
			subTest.Fatalf("total tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens))
		}
	})

	t.Run("queued response negative poll token usage maps to bad gateway", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				_, _ = responseWriter.Write([]byte(`{"id":"queued","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/queued":
				_, _ = responseWriter.Write([]byte(`{"status":"completed","output_text":"bad usage","usage":{"input_tokens":1,"output_tokens":-1,"total_tokens":0}}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("queued response malformed identifier fails poll construction", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"id":"bad\nid","status":"queued"}`))
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("queued response poll transport error reports upstream failure", func(subTest *testing.T) {
		previousClient := proxy.HTTPClient
		requestCount := 0
		proxy.HTTPClient = coverageHTTPDoer(func(httpRequest *http.Request) (*http.Response, error) {
			requestCount++
			if requestCount == 1 {
				return coverageHTTPResponse(http.StatusOK, `{"id":"queued","status":"queued"}`), nil
			}
			return nil, backoff.Permanent(errors.New("poll transport failed"))
		})
		subTest.Cleanup(func() { proxy.HTTPClient = previousClient })
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      1,
			UpstreamPollTimeoutSeconds: TestTimeout,
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		requireUpstreamFailureStatus(subTest, statusCode)
	})

	t.Run("poll failed status maps to bad gateway", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				_, _ = responseWriter.Write([]byte(`{"id":"failed","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/failed":
				_, _ = responseWriter.Write([]byte(`{"status":"failed"}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("poll incomplete response with text returns text", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				_, _ = responseWriter.Write([]byte(`{"id":"partial","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/partial":
				_, _ = responseWriter.Write([]byte(`{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial poll answer"}]}]}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK || body != "partial poll answer" {
			subTest.Fatalf("status=%d body=%q", statusCode, body)
		}
	})

	t.Run("poll incomplete response without text maps to bad gateway", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				_, _ = responseWriter.Write([]byte(`{"id":"partial","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/partial":
				_, _ = responseWriter.Write([]byte(`{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("synthesis transport error reports upstream failure", func(subTest *testing.T) {
		previousClient := proxy.HTTPClient
		requestCount := 0
		proxy.HTTPClient = coverageHTTPDoer(func(httpRequest *http.Request) (*http.Response, error) {
			requestCount++
			if requestCount == 1 {
				return coverageHTTPResponse(http.StatusOK, `{"id":"needs_synthesis","status":"completed","output":[]}`), nil
			}
			return nil, backoff.Permanent(errors.New("synthesis transport failed"))
		})
		subTest.Cleanup(func() { proxy.HTTPClient = previousClient })
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      1,
			UpstreamPollTimeoutSeconds: TestTimeout,
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		requireUpstreamFailureStatus(subTest, statusCode)
	})

	t.Run("synthesis malformed poll identifier fails request construction", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				requestBytes, _ := io.ReadAll(httpRequest.Body)
				var requestPayload map[string]any
				_ = json.Unmarshal(requestBytes, &requestPayload)
				if requestPayload["previous_response_id"] == nil {
					_, _ = responseWriter.Write([]byte(`{"id":"needs_synthesis","status":"completed","output":[]}`))
					return
				}
				_, _ = responseWriter.Write([]byte(`{"id":"bad\nid","status":"queued"}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("poll timeout maps to bad gateway after upstream incomplete", func(subTest *testing.T) {
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				_, _ = responseWriter.Write([]byte(`{"id":"slow","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/slow":
				_, _ = responseWriter.Write([]byte(`{"status":"in_progress"}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		}))
		subTest.Cleanup(upstreamServer.Close)
		endpoints := proxy.NewEndpoints()
		endpoints.SetResponsesURL(upstreamServer.URL)
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      2,
			UpstreamPollTimeoutSeconds: 1,
			Endpoints:                  endpoints,
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("poll fetch timeout maps to bad gateway", func(subTest *testing.T) {
		endpoints := proxy.NewEndpoints()
		endpoints.SetResponsesURL("https://openai.invalid/v1/responses")
		previousClient := proxy.HTTPClient
		proxy.HTTPClient = &http.Client{Transport: coverageRoundTripper(func(httpRequest *http.Request) (*http.Response, error) {
			switch httpRequest.URL.String() {
			case endpoints.GetResponsesURL():
				return coverageHTTPResponse(http.StatusOK, `{"id":"slow","status":"queued"}`), nil
			case endpoints.GetResponsesURL() + "/slow":
				<-httpRequest.Context().Done()
				return nil, httpRequest.Context().Err()
			default:
				subTest.Fatalf("unexpected request to %s", httpRequest.URL.String())
				return nil, nil
			}
		})}
		subTest.Cleanup(func() { proxy.HTTPClient = previousClient })
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      2,
			UpstreamPollTimeoutSeconds: 1,
			Endpoints:                  endpoints,
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("poll sleep timeout maps to bad gateway", func(subTest *testing.T) {
		endpoints := proxy.NewEndpoints()
		endpoints.SetResponsesURL("https://openai.invalid/v1/responses")
		previousClient := proxy.HTTPClient
		proxy.HTTPClient = &http.Client{Transport: coverageRoundTripper(func(httpRequest *http.Request) (*http.Response, error) {
			switch httpRequest.URL.String() {
			case endpoints.GetResponsesURL():
				return coverageHTTPResponse(http.StatusOK, `{"id":"slow","status":"queued"}`), nil
			case endpoints.GetResponsesURL() + "/slow":
				time.Sleep(750 * time.Millisecond)
				return coverageHTTPResponse(http.StatusOK, `{"status":"in_progress"}`), nil
			default:
				subTest.Fatalf("unexpected request to %s", httpRequest.URL.String())
				return nil, nil
			}
		})}
		subTest.Cleanup(func() { proxy.HTTPClient = previousClient })
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      2,
			UpstreamPollTimeoutSeconds: 1,
			Endpoints:                  endpoints,
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("request context cancellation during poll sleep maps to gateway timeout", func(subTest *testing.T) {
		endpoints := proxy.NewEndpoints()
		endpoints.SetResponsesURL("https://openai.invalid/v1/responses")
		previousClient := proxy.HTTPClient
		proxy.HTTPClient = &http.Client{Transport: coverageRoundTripper(func(httpRequest *http.Request) (*http.Response, error) {
			switch httpRequest.URL.String() {
			case endpoints.GetResponsesURL():
				return coverageHTTPResponse(http.StatusOK, `{"id":"slow","status":"queued"}`), nil
			case endpoints.GetResponsesURL() + "/slow":
				return coverageHTTPResponse(http.StatusOK, `{"status":"in_progress"}`), nil
			default:
				subTest.Fatalf("unexpected request to %s", httpRequest.URL.String())
				return nil, nil
			}
		})}
		subTest.Cleanup(func() { proxy.HTTPClient = previousClient })
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      2,
			UpstreamPollTimeoutSeconds: 2,
			Endpoints:                  endpoints,
		})
		queryParameters := url.Values{}
		queryParameters.Set("key", TestSecret)
		queryParameters.Set("prompt", TestPrompt)
		request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
		requestContext, cancelRequest := context.WithTimeout(request.Context(), 100*time.Millisecond)
		defer cancelRequest()
		request = request.WithContext(requestContext)
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusGatewayTimeout {
			subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusGatewayTimeout, responseRecorder.Body.String())
		}
	})

	t.Run("poll completed without text maps to bad gateway", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				_, _ = responseWriter.Write([]byte(`{"id":"blank","status":"queued"}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/blank":
				_, _ = responseWriter.Write([]byte(`{"status":"completed","output":[]}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("non terminal poll transport error reports upstream failure", func(subTest *testing.T) {
		previousClient := proxy.HTTPClient
		requestCount := 0
		proxy.HTTPClient = coverageHTTPDoer(func(httpRequest *http.Request) (*http.Response, error) {
			requestCount++
			switch requestCount {
			case 1:
				return coverageHTTPResponse(http.StatusOK, `{"id":"queued","status":"queued"}`), nil
			case 2:
				return coverageHTTPResponse(http.StatusOK, `{"status":"in_progress"}`), nil
			default:
				return nil, backoff.Permanent(errors.New("poll transport failed"))
			}
		})
		subTest.Cleanup(func() { proxy.HTTPClient = previousClient })
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      1,
			UpstreamPollTimeoutSeconds: TestTimeout,
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		requireUpstreamFailureStatus(subTest, statusCode)
	})

	t.Run("initial bad request status maps to bad gateway", func(subTest *testing.T) {
		router := textRouterWithResponsesHandler(subTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.WriteHeader(http.StatusBadRequest)
			_, _ = responseWriter.Write([]byte(`{"error":"bad request"}`))
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("invalid responses URL fails request construction", func(subTest *testing.T) {
		endpoints := proxy.NewEndpoints()
		endpoints.SetResponsesURL("http://[::1")
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      TestTimeout,
			UpstreamPollTimeoutSeconds: TestTimeout,
			Endpoints:                  endpoints,
		})
		queryParameters := url.Values{}
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("synthesis continuation malformed responses fail", func(subTest *testing.T) {
		testCases := []struct {
			name          string
			synthesisBody string
			synthesisCode int
		}{
			{name: "bad status", synthesisCode: http.StatusBadRequest, synthesisBody: `{}`},
			{name: "malformed json", synthesisCode: http.StatusOK, synthesisBody: `{`},
			{name: "missing identifier", synthesisCode: http.StatusOK, synthesisBody: `{"status":"queued"}`},
		}
		for _, testCase := range testCases {
			subTest.Run(testCase.name, func(caseTest *testing.T) {
				router := textRouterWithResponsesHandler(caseTest, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
					responseWriter.Header().Set("Content-Type", "application/json")
					switch {
					case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
						requestBytes, _ := io.ReadAll(httpRequest.Body)
						var requestPayload map[string]any
						_ = json.Unmarshal(requestBytes, &requestPayload)
						if requestPayload["previous_response_id"] == nil {
							_, _ = responseWriter.Write([]byte(`{"id":"needs_synthesis","status":"completed","output":[]}`))
							return
						}
						responseWriter.WriteHeader(testCase.synthesisCode)
						_, _ = responseWriter.Write([]byte(testCase.synthesisBody))
					default:
						http.NotFound(responseWriter, httpRequest)
					}
				})
				queryParameters := url.Values{}
				statusCode, _, _ := performCoverageTextRequest(caseTest, router, queryParameters, "")
				if statusCode != http.StatusBadGateway {
					caseTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
				}
			})
		}
	})
}

func TestCoverageProviderRoutingEdges(t *testing.T) {
	t.Run("alias provider resolves text request", func(subTest *testing.T) {
		var capturedPayload map[string]any
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			requestBytes, _ := io.ReadAll(httpRequest.Body)
			_ = json.Unmarshal(requestBytes, &capturedPayload)
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"reasoning_content":"reasoned answer"}}]}`))
		}))
		subTest.Cleanup(upstreamServer.Close)
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			DashScopeKey:               "sk-qwen",
			DashScopeBaseURL:           upstreamServer.URL,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      TestTimeout,
			UpstreamPollTimeoutSeconds: TestTimeout,
		})
		queryParameters := url.Values{}
		queryParameters.Set("provider", "qwen")
		queryParameters.Set("system_prompt", "system text")
		statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK || body != "reasoned answer" {
			subTest.Fatalf("status=%d body=%q", statusCode, body)
		}
		messages, ok := capturedPayload["messages"].([]any)
		if !ok || len(messages) != 2 {
			subTest.Fatalf("messages=%v", capturedPayload["messages"])
		}
	})

	t.Run("provider status and parse errors map to responses", func(subTest *testing.T) {
		testCases := []struct {
			name       string
			statusCode int
			body       string
			wantStatus int
		}{
			{name: "rate limited", statusCode: http.StatusTooManyRequests, body: `{}`, wantStatus: http.StatusTooManyRequests},
			{name: "provider api failure", statusCode: http.StatusBadRequest, body: `{}`, wantStatus: http.StatusBadGateway},
			{name: "malformed json", statusCode: http.StatusOK, body: `{`, wantStatus: http.StatusBadGateway},
			{name: "negative usage", statusCode: http.StatusOK, body: `{"choices":[{"message":{"content":"bad usage"}}],"usage":{"prompt_tokens":1,"completion_tokens":-1}}`, wantStatus: http.StatusBadGateway},
			{name: "missing text", statusCode: http.StatusOK, body: `{"choices":[{"message":{}}]}`, wantStatus: http.StatusBadGateway},
		}
		for _, testCase := range testCases {
			subTest.Run(testCase.name, func(caseTest *testing.T) {
				upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
					responseWriter.Header().Set("Content-Type", "application/json")
					responseWriter.WriteHeader(testCase.statusCode)
					_, _ = responseWriter.Write([]byte(testCase.body))
				}))
				caseTest.Cleanup(upstreamServer.Close)
				router := coverageRouter(caseTest, proxy.Configuration{
					Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
					OpenAIKey:                  TestAPIKey,
					DeepSeekKey:                testDeepSeekKey,
					DeepSeekBaseURL:            upstreamServer.URL,
					LogLevel:                   proxy.LogLevelInfo,
					WorkerCount:                1,
					QueueSize:                  1,
					RequestTimeoutSeconds:      TestTimeout,
					UpstreamPollTimeoutSeconds: TestTimeout,
				})
				queryParameters := url.Values{}
				queryParameters.Set("provider", proxy.ProviderNameDeepSeek)
				statusCode, _, _ := performCoverageTextRequest(caseTest, router, queryParameters, "")
				if statusCode != testCase.wantStatus {
					caseTest.Fatalf("status=%d want=%d", statusCode, testCase.wantStatus)
				}
			})
		}
	})

	t.Run("transport deadline maps to gateway timeout", func(subTest *testing.T) {
		previousClient := proxy.HTTPClient
		proxy.HTTPClient = &http.Client{Transport: coverageRoundTripper(func(httpRequest *http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		})}
		subTest.Cleanup(func() { proxy.HTTPClient = previousClient })
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			DeepSeekKey:                testDeepSeekKey,
			DeepSeekBaseURL:            "https://deepseek.invalid",
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      1,
			UpstreamPollTimeoutSeconds: TestTimeout,
		})
		queryParameters := url.Values{}
		queryParameters.Set("provider", proxy.ProviderNameDeepSeek)
		statusCode, _, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusGatewayTimeout {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusGatewayTimeout)
		}
	})

	t.Run("provider model and URL validation failures", func(subTest *testing.T) {
		testCases := []struct {
			name       string
			baseURL    string
			model      string
			wantStatus int
		}{
			{name: "unknown model", baseURL: "https://deepseek.invalid", model: "unknown-model", wantStatus: http.StatusBadRequest},
			{name: "invalid provider base URL", baseURL: "http://[::1", model: proxy.ModelNameDeepSeekV4Flash, wantStatus: http.StatusBadGateway},
		}
		for _, testCase := range testCases {
			subTest.Run(testCase.name, func(caseTest *testing.T) {
				router := coverageRouter(caseTest, proxy.Configuration{
					Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
					OpenAIKey:                  TestAPIKey,
					DeepSeekKey:                testDeepSeekKey,
					DeepSeekBaseURL:            testCase.baseURL,
					LogLevel:                   proxy.LogLevelInfo,
					WorkerCount:                1,
					QueueSize:                  1,
					RequestTimeoutSeconds:      1,
					UpstreamPollTimeoutSeconds: TestTimeout,
				})
				queryParameters := url.Values{}
				queryParameters.Set("provider", proxy.ProviderNameDeepSeek)
				queryParameters.Set("model", testCase.model)
				statusCode, _, _ := performCoverageTextRequest(caseTest, router, queryParameters, "")
				if statusCode != testCase.wantStatus {
					caseTest.Fatalf("status=%d want=%d", statusCode, testCase.wantStatus)
				}
			})
		}
	})

	t.Run("blank model uses provider default", func(subTest *testing.T) {
		var capturedPayload map[string]any
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			bodyBytes, readError := io.ReadAll(httpRequest.Body)
			if readError != nil {
				subTest.Fatalf("read body: %v", readError)
			}
			if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
				subTest.Fatalf("unmarshal body: %v", unmarshalError)
			}
			_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"default ok"}}]}`))
		}))
		subTest.Cleanup(upstreamServer.Close)
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			DeepSeekKey:                testDeepSeekKey,
			DeepSeekBaseURL:            upstreamServer.URL,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      TestTimeout,
			UpstreamPollTimeoutSeconds: TestTimeout,
		})
		queryParameters := url.Values{}
		queryParameters.Set("provider", proxy.ProviderNameDeepSeek)
		queryParameters.Set("model", " ")
		statusCode, body, _ := performCoverageTextRequest(subTest, router, queryParameters, "")
		if statusCode != http.StatusOK {
			subTest.Fatalf("status=%d want=%d body=%s", statusCode, http.StatusOK, body)
		}
		if capturedPayload["model"] != proxy.ModelNameDeepSeekV4Flash {
			subTest.Fatalf("model=%v want=%s", capturedPayload["model"], proxy.ModelNameDeepSeekV4Flash)
		}
	})

	t.Run("provider non deadline transport error maps to bad gateway", func(subTest *testing.T) {
		previousClient := proxy.HTTPClient
		proxy.HTTPClient = &http.Client{Transport: coverageRoundTripper(func(httpRequest *http.Request) (*http.Response, error) {
			return nil, errors.New("transport failed")
		})}
		subTest.Cleanup(func() { proxy.HTTPClient = previousClient })
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			DeepSeekKey:                testDeepSeekKey,
			DeepSeekBaseURL:            "https://deepseek.invalid",
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      1,
			UpstreamPollTimeoutSeconds: TestTimeout,
		})
		queryParameters := url.Values{}
		queryParameters.Set("provider", proxy.ProviderNameDeepSeek)
		statusCode, _, _ := performCoverageTextRequestWithTimeout(subTest, router, queryParameters, "", coverageShortRequestTimeout)
		if statusCode != http.StatusGatewayTimeout {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusGatewayTimeout)
		}
	})
}

func TestCoverageDictationEdges(t *testing.T) {
	t.Run("invalid and missing audio forms", func(subTest *testing.T) {
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      1,
			UpstreamPollTimeoutSeconds: TestTimeout,
			MaxInputAudioBytes:         1024,
		})
		invalidRequest := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, strings.NewReader("not multipart"))
		invalidRecorder := httptest.NewRecorder()
		router.ServeHTTP(invalidRecorder, invalidRequest)
		if invalidRecorder.Code != http.StatusBadRequest {
			subTest.Fatalf("invalid form status=%d", invalidRecorder.Code)
		}

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		if closeError := writer.Close(); closeError != nil {
			subTest.Fatalf("close multipart writer: %v", closeError)
		}
		missingRequest := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, body)
		missingRequest.Header.Set("Content-Type", writer.FormDataContentType())
		missingRecorder := httptest.NewRecorder()
		router.ServeHTTP(missingRecorder, missingRequest)
		if missingRecorder.Code != http.StatusBadRequest {
			subTest.Fatalf("missing file status=%d", missingRecorder.Code)
		}
	})

	t.Run("unsupported and unknown dictation requests", func(subTest *testing.T) {
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			DeepSeekKey:                testDeepSeekKey,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      TestTimeout,
			UpstreamPollTimeoutSeconds: TestTimeout,
			MaxInputAudioBytes:         1024,
		})
		for _, requestURL := range []string{
			"/dictate?key=" + TestSecret + "&provider=deepseek",
			"/dictate?key=" + TestSecret + "&provider=unknown",
		} {
			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			filePart, createError := writer.CreateFormFile("audio", "audio.webm")
			if createError != nil {
				subTest.Fatalf("create form file: %v", createError)
			}
			_, _ = filePart.Write([]byte(testAudioPayload))
			_ = writer.Close()
			request := httptest.NewRequest(http.MethodPost, requestURL, body)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			responseRecorder := httptest.NewRecorder()
			router.ServeHTTP(responseRecorder, request)
			if responseRecorder.Code != http.StatusBadRequest {
				subTest.Fatalf("url=%s status=%d", requestURL, responseRecorder.Code)
			}
		}
	})

	t.Run("dictation provider credential and model validation failures", func(subTest *testing.T) {
		testCases := []struct {
			name          string
			configuration proxy.Configuration
			requestURL    string
			wantStatus    int
		}{
			{
				name: "siliconflow missing credential",
				configuration: proxy.Configuration{
					Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
					OpenAIKey:                  TestAPIKey,
					LogLevel:                   proxy.LogLevelInfo,
					WorkerCount:                1,
					QueueSize:                  1,
					RequestTimeoutSeconds:      TestTimeout,
					UpstreamPollTimeoutSeconds: TestTimeout,
					MaxInputAudioBytes:         1024,
				},
				requestURL: "/dictate?key=" + TestSecret + "&provider=siliconflow",
				wantStatus: http.StatusServiceUnavailable,
			},
			{
				name: "siliconflow unknown model",
				configuration: proxy.Configuration{
					Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
					OpenAIKey:                  TestAPIKey,
					SiliconFlowKey:             testSiliconFlowKey,
					LogLevel:                   proxy.LogLevelInfo,
					WorkerCount:                1,
					QueueSize:                  1,
					RequestTimeoutSeconds:      TestTimeout,
					UpstreamPollTimeoutSeconds: TestTimeout,
					MaxInputAudioBytes:         1024,
				},
				requestURL: "/dictate?key=" + TestSecret + "&provider=siliconflow&model=unknown",
				wantStatus: http.StatusBadRequest,
			},
			{
				name: "openai missing credential when non openai defaults are configured",
				configuration: proxy.Configuration{
					Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameDeepSeek, Model: proxy.ModelNameDeepSeekV4Flash, DictationProvider: proxy.ProviderNameSiliconFlow}),
					DeepSeekKey:                testDeepSeekKey,
					SiliconFlowKey:             testSiliconFlowKey,
					LogLevel:                   proxy.LogLevelInfo,
					WorkerCount:                1,
					QueueSize:                  1,
					RequestTimeoutSeconds:      TestTimeout,
					UpstreamPollTimeoutSeconds: TestTimeout,
					MaxInputAudioBytes:         1024,
				},
				requestURL: "/dictate?key=" + TestSecret + "&provider=openai",
				wantStatus: http.StatusServiceUnavailable,
			},
		}
		for _, testCase := range testCases {
			subTest.Run(testCase.name, func(caseTest *testing.T) {
				router := coverageRouter(caseTest, testCase.configuration)
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)
				filePart, createError := writer.CreateFormFile("audio", "audio.webm")
				if createError != nil {
					caseTest.Fatalf("create form file: %v", createError)
				}
				_, _ = filePart.Write([]byte(testAudioPayload))
				_ = writer.Close()
				request := httptest.NewRequest(http.MethodPost, testCase.requestURL, body)
				request.Header.Set("Content-Type", writer.FormDataContentType())
				responseRecorder := httptest.NewRecorder()
				router.ServeHTTP(responseRecorder, request)
				if responseRecorder.Code != testCase.wantStatus {
					caseTest.Fatalf("status=%d want=%d", responseRecorder.Code, testCase.wantStatus)
				}
			})
		}
	})

	t.Run("upstream dictation status and transport errors", func(subTest *testing.T) {
		testCases := []struct {
			name       string
			statusCode int
			body       string
			wantStatus int
		}{
			{name: "upstream status", statusCode: http.StatusBadRequest, body: `{}`, wantStatus: http.StatusBadGateway},
			{name: "blank transcript", statusCode: http.StatusOK, body: `   `, wantStatus: http.StatusBadGateway},
		}
		for _, testCase := range testCases {
			subTest.Run(testCase.name, func(caseTest *testing.T) {
				upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
					responseWriter.WriteHeader(testCase.statusCode)
					_, _ = responseWriter.Write([]byte(testCase.body))
				}))
				caseTest.Cleanup(upstreamServer.Close)
				endpoints := proxy.NewEndpoints()
				endpoints.SetTranscriptionsURL(upstreamServer.URL)
				router := coverageRouter(caseTest, proxy.Configuration{
					Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
					OpenAIKey:                  TestAPIKey,
					LogLevel:                   proxy.LogLevelInfo,
					WorkerCount:                1,
					QueueSize:                  1,
					RequestTimeoutSeconds:      TestTimeout,
					UpstreamPollTimeoutSeconds: TestTimeout,
					MaxInputAudioBytes:         1024,
					Endpoints:                  endpoints,
				})
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)
				filePart, createError := writer.CreateFormFile("audio", "   ")
				if createError != nil {
					caseTest.Fatalf("create form file: %v", createError)
				}
				_, _ = filePart.Write([]byte(testAudioPayload))
				_ = writer.Close()
				request := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, body)
				request.Header.Set("Content-Type", writer.FormDataContentType())
				responseRecorder := httptest.NewRecorder()
				router.ServeHTTP(responseRecorder, request)
				if responseRecorder.Code != testCase.wantStatus {
					caseTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, testCase.wantStatus, responseRecorder.Body.String())
				}
			})
		}
	})

	t.Run("invalid dictation endpoint fails request construction", func(subTest *testing.T) {
		endpoints := proxy.NewEndpoints()
		endpoints.SetTranscriptionsURL("http://[::1")
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      TestTimeout,
			UpstreamPollTimeoutSeconds: TestTimeout,
			MaxInputAudioBytes:         1024,
			Endpoints:                  endpoints,
		})
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		filePart, createError := writer.CreateFormFile("audio", "audio.webm")
		if createError != nil {
			subTest.Fatalf("create form file: %v", createError)
		}
		_, _ = filePart.Write([]byte(testAudioPayload))
		_ = writer.Close()
		request := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", responseRecorder.Code, http.StatusBadGateway)
		}
	})

	t.Run("dictation transport deadline maps to timeout", func(subTest *testing.T) {
		previousClient := proxy.HTTPClient
		proxy.HTTPClient = &http.Client{Transport: coverageRoundTripper(func(httpRequest *http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		})}
		subTest.Cleanup(func() { proxy.HTTPClient = previousClient })
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      TestTimeout,
			UpstreamPollTimeoutSeconds: TestTimeout,
			MaxInputAudioBytes:         1024,
		})
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		filePart, createError := writer.CreateFormFile("audio", "audio.webm")
		if createError != nil {
			subTest.Fatalf("create form file: %v", createError)
		}
		_, _ = filePart.Write([]byte(testAudioPayload))
		_ = writer.Close()
		request := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusGatewayTimeout {
			subTest.Fatalf("status=%d want=%d", responseRecorder.Code, http.StatusGatewayTimeout)
		}
	})
}

func TestCoverageServeAndEndpointReset(t *testing.T) {
	endpoints := proxy.NewEndpoints()
	endpoints.SetTranscriptionsURL("http://transcriptions.example")
	endpoints.ResetTranscriptionsURL()
	if transcriptionsURL := endpoints.GetTranscriptionsURL(); !strings.Contains(transcriptionsURL, "/audio/transcriptions") {
		t.Fatalf("transcriptionsURL=%q", transcriptionsURL)
	}

	buildError := proxy.Serve(proxy.Configuration{
		OpenAIKey:                  TestAPIKey,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, coverageLogger())
	if buildError == nil {
		t.Fatalf("Serve buildError=nil want non-nil")
	}

	serveError := proxy.Serve(proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		Port:                       -1,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, coverageLogger())
	if serveError == nil {
		t.Fatalf("Serve error=nil want non-nil")
	}
}

func TestCoverageHTTPUtilityReadFailure(t *testing.T) {
	previousClient := proxy.HTTPClient
	proxy.HTTPClient = &http.Client{Transport: coverageRoundTripper(func(httpRequest *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(failingReader{}),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { proxy.HTTPClient = previousClient })
	router := coverageRouter(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      1,
		UpstreamPollTimeoutSeconds: TestTimeout,
	})
	queryParameters := url.Values{}
	statusCode, _, _ := performCoverageTextRequestWithTimeout(t, router, queryParameters, "", coverageShortRequestTimeout)
	if statusCode != http.StatusGatewayTimeout {
		t.Fatalf("status=%d want=%d", statusCode, http.StatusGatewayTimeout)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}
