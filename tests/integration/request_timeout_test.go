package integration_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	timeoutExpectedStatusCode       = http.StatusGatewayTimeout
	timeoutRequestTimeout           = 1
	timeoutUpstreamDelay            = 3 * time.Second
	timeoutHTTPClientTimeout        = timeoutUpstreamDelay + 2*time.Second
	gatewayContextRequestTimeout    = 150 * time.Millisecond
	gatewayContextProxyTimeout      = 2
	gatewayContextLateResponseDelay = 650 * time.Millisecond
	gatewayContextAssertionTimeout  = 2 * time.Second
	openAIAPIResponseLogMessage     = "OpenAI API response"
	lateOpenAIResponseBody          = `{"output_text":"LATE_OPENAI_RESPONSE"}`
)

// makeTimeoutHTTPClient returns an HTTP client whose responses delay longer than the request timeout.
func makeTimeoutHTTPClient(testingInstance *testing.T, endpoints *proxy.Endpoints) *http.Client {
	testingInstance.Helper()
	return &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			switch {
			case request.URL.String() == endpoints.GetModelsURL():
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(modelsListBody)), Header: make(http.Header)}, nil
			case strings.HasPrefix(request.URL.String(), endpoints.GetModelsURL()+"/"):
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(metadataTemperatureTools)), Header: make(http.Header)}, nil
			case request.URL.String() == endpoints.GetResponsesURL():
				select {
				case <-request.Context().Done():
					return nil, request.Context().Err()
				case <-time.After(timeoutUpstreamDelay):
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"output_text":"NEVER"}`)), Header: make(http.Header)}, nil
				}
			default:
				testingInstance.Fatalf("unexpected request to %s", request.URL.String())
				return nil, nil
			}
		}),
		Timeout: timeoutHTTPClientTimeout,
	}
}

// TestIntegrationGatewayContextTimeoutCancelsUpstreamRequest verifies gateway-style request cancellation stops upstream OpenAI work before any late response is accepted.
func TestIntegrationGatewayContextTimeoutCancelsUpstreamRequest(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamRequestCanceled := make(chan struct{})
	lateUsableOpenAIResponse := make(chan struct{})
	var upstreamCancelOnce sync.Once
	var lateResponseOnce sync.Once
	endpoints := proxy.NewEndpoints()
	timeoutClient := &http.Client{
		Transport: roundTripperFunc(func(httpRequest *http.Request) (*http.Response, error) {
			switch {
			case httpRequest.URL.String() == endpoints.GetModelsURL():
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(modelsListBody)), Header: make(http.Header)}, nil
			case strings.HasPrefix(httpRequest.URL.String(), endpoints.GetModelsURL()+"/"):
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(metadataTemperatureTools)), Header: make(http.Header)}, nil
			case httpRequest.URL.String() == endpoints.GetResponsesURL():
				responseHeader := make(http.Header)
				responseHeader.Set(contentTypeHeaderKey, contentTypeJSON)
				select {
				case <-httpRequest.Context().Done():
					upstreamCancelOnce.Do(func() { close(upstreamRequestCanceled) })
					return nil, httpRequest.Context().Err()
				case <-time.After(gatewayContextLateResponseDelay):
					lateResponseOnce.Do(func() { close(lateUsableOpenAIResponse) })
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(lateOpenAIResponseBody)), Header: responseHeader}, nil
				}
			default:
				testingInstance.Fatalf(unexpectedRequestFormat, httpRequest.URL.String())
				return nil, nil
			}
		}),
	}

	previousHTTPClient := proxy.HTTPClient
	proxy.HTTPClient = timeoutClient
	testingInstance.Cleanup(func() { proxy.HTTPClient = previousHTTPClient })
	endpoints.SetModelsURL(mockModelsURL)
	endpoints.SetResponsesURL(mockResponsesURL)

	observedCore, observedLogs := observer.New(zapcore.DebugLevel)
	loggerInstance := zap.New(observedCore)
	testingInstance.Cleanup(func() { _ = loggerInstance.Sync() })
	router, buildError := proxy.BuildRouter(integrationConfiguration(testingInstance, proxy.Configuration{
		Tenants:                      proxy.SingleTenantConfigurations("integration", serviceSecretValue),
		OpenAIKey:                    openAIKeyValue,
		LogLevel:                     logLevelDebug,
		WorkerCount:                  1,
		QueueSize:                    4,
		RequestTimeoutSeconds:        gatewayContextProxyTimeout,
		Endpoints:                    endpoints,
		MaxPromptBytes:               proxy.DefaultMaxPromptBytes,
		MaxInputAudioBytes:           proxy.DefaultMaxInputAudioBytes,
		DeepSeekBaseURL:              "https://deepseek.invalid",
		DashScopeBaseURL:             "https://dashscope.invalid",
		MoonshotBaseURL:              "https://moonshot.invalid",
		SiliconFlowBaseURL:           "https://siliconflow.invalid",
		SiliconFlowTranscriptionsURL: "https://siliconflow.invalid/audio/transcriptions",
		ZhipuBaseURL:                 "https://zhipu.invalid",
	}), loggerInstance.Sugar())
	if buildError != nil {
		testingInstance.Fatalf(buildRouterFailedFormat, buildError)
	}

	applicationServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		gatewayContext, cancelGatewayContext := context.WithTimeout(httpRequest.Context(), gatewayContextRequestTimeout)
		defer cancelGatewayContext()
		router.ServeHTTP(responseWriter, httpRequest.WithContext(gatewayContext))
	}))
	testingInstance.Cleanup(applicationServer.Close)

	requestURL, _ := url.Parse(applicationServer.URL)
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	requestURL.RawQuery = queryValues.Encode()
	httpResponse, requestError := applicationServer.Client().Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf(getFailedFormat, requestError)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != timeoutExpectedStatusCode {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf(statusWantBodyFormat, httpResponse.StatusCode, timeoutExpectedStatusCode, string(responseBody))
	}

	select {
	case <-upstreamRequestCanceled:
	case <-lateUsableOpenAIResponse:
		testingInstance.Fatal("upstream produced a late usable OpenAI API response after proxy sent 504")
	case <-time.After(gatewayContextAssertionTimeout):
		testingInstance.Fatal("upstream request context was not canceled after proxy sent 504")
	}
	if observedLogs.FilterMessage(openAIAPIResponseLogMessage).Len() != 0 {
		testingInstance.Fatal("observed late OpenAI API response log after proxy sent 504")
	}
}

// TestIntegrationUpstreamRequestTimeoutTriggersGatewayTimeout verifies upstream timeouts result in a gateway timeout before the upstream delay elapses.
func TestIntegrationUpstreamRequestTimeoutTriggersGatewayTimeout(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct{ name string }{{name: "gateway_timeout"}}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			endpoints := proxy.NewEndpoints()
			configureProxy(subTest, makeTimeoutHTTPClient(subTest, endpoints), endpoints)
			router, buildError := proxy.BuildRouter(integrationConfiguration(subTest, proxy.Configuration{Tenants: proxy.SingleTenantConfigurations("integration", serviceSecretValue), OpenAIKey: openAIKeyValue, LogLevel: logLevelDebug, WorkerCount: 1, QueueSize: 8, RequestTimeoutSeconds: timeoutRequestTimeout, Endpoints: endpoints}), newLogger(subTest))
			if buildError != nil {
				subTest.Fatalf("BuildRouter failed: %v", buildError)
			}
			server := httptest.NewServer(router)
			subTest.Cleanup(server.Close)
			requestURL, _ := url.Parse(server.URL)
			queryValues := requestURL.Query()
			queryValues.Set(promptQueryParameter, promptValue)
			queryValues.Set(keyQueryParameter, serviceSecretValue)
			requestURL.RawQuery = queryValues.Encode()
			startInstant := time.Now()
			httpResponse, requestError := http.Get(requestURL.String())
			elapsedDuration := time.Since(startInstant)
			if requestError != nil {
				subTest.Fatalf("GET failed: %v", requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != timeoutExpectedStatusCode {
				subTest.Fatalf("status=%d want=%d", httpResponse.StatusCode, timeoutExpectedStatusCode)
			}
			if elapsedDuration >= timeoutUpstreamDelay {
				subTest.Fatalf("elapsed=%v exceeds upstream delay %v", elapsedDuration, timeoutUpstreamDelay)
			}
		})
	}
}
