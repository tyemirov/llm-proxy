package integration_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	timeoutExpectedStatusCode       = http.StatusGatewayTimeout
	callerCancellationStatusCode    = 499
	timeoutRequestTimeout           = 1
	timeoutUpstreamDelay            = 3 * time.Second
	timeoutHTTPClientTimeout        = timeoutUpstreamDelay + 2*time.Second
	gatewayContextRequestTimeout    = 150 * time.Millisecond
	gatewayContextProxyTimeout      = 2
	gatewayContextLateResponseDelay = 650 * time.Millisecond
	gatewayContextAssertionTimeout  = 2 * time.Second
	openAIAPIResponseLogMessage     = "OpenAI API response"
	lateOpenAIResponseBody          = `{"status":"completed","output_text":"LATE_OPENAI_RESPONSE"}`
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
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":"completed","output_text":"NEVER"}`)), Header: make(http.Header)}, nil
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
	router, buildError := buildIntegrationRouter(testingInstance, proxy.Configuration{
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
		ZAIBaseURL:                   "https://zai.invalid",
	}, loggerInstance.Sugar())
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
	if httpResponse.StatusCode != callerCancellationStatusCode {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf(statusWantBodyFormat, httpResponse.StatusCode, callerCancellationStatusCode, string(responseBody))
	}

	select {
	case <-upstreamRequestCanceled:
	case <-lateUsableOpenAIResponse:
		testingInstance.Fatal("upstream produced a late usable OpenAI API response after caller cancellation")
	case <-time.After(gatewayContextAssertionTimeout):
		testingInstance.Fatal("upstream request context was not canceled after caller cancellation")
	}
	if observedLogs.FilterMessage(openAIAPIResponseLogMessage).Len() != 0 {
		testingInstance.Fatal("observed late OpenAI API response log after caller cancellation")
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
			router, buildError := buildIntegrationRouter(subTest, proxy.Configuration{LogLevel: logLevelDebug, WorkerCount: 1, QueueSize: 8, RequestTimeoutSeconds: timeoutRequestTimeout, Endpoints: endpoints}, newLogger(subTest))
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

type requestTimeoutHTTPDoer func(*http.Request) (*http.Response, error)

func (doer requestTimeoutHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

type closeAwareBlockingBody struct {
	closed chan struct{}
	once   sync.Once
}

func newCloseAwareBlockingBody() *closeAwareBlockingBody {
	return &closeAwareBlockingBody{closed: make(chan struct{})}
}

func (body *closeAwareBlockingBody) Read([]byte) (int, error) {
	<-body.closed
	return 0, errors.New("body closed")
}

func (body *closeAwareBlockingBody) Close() error {
	body.once.Do(func() { close(body.closed) })
	return nil
}

func timeoutContractConfiguration(defaultSeconds int, maximumSeconds int) proxy.Configuration {
	return proxy.Configuration{
		LogLevel:                 proxy.LogLevelInfo,
		WorkerCount:              1,
		QueueSize:                4,
		RequestTimeoutSeconds:    defaultSeconds,
		MaxRequestTimeoutSeconds: maximumSeconds,
	}
}

func timeoutContractRouter(testingInstance *testing.T, doer proxy.HTTPDoer, configuration proxy.Configuration, logger *zap.SugaredLogger) *gin.Engine {
	testingInstance.Helper()
	previousHTTPClient := proxy.HTTPClient
	proxy.HTTPClient = doer
	testingInstance.Cleanup(func() { proxy.HTTPClient = previousHTTPClient })
	router, buildError := buildIntegrationRouter(testingInstance, configuration, logger)
	if buildError != nil {
		testingInstance.Fatalf("BuildRouter failed: %v", buildError)
	}
	return router
}

func completedTimeoutContractResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func timeoutContractMultipartBody(testingInstance *testing.T) (*bytes.Buffer, string) {
	testingInstance.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	filePart, createError := writer.CreateFormFile("audio", "sample.webm")
	if createError != nil {
		testingInstance.Fatalf("create audio form: %v", createError)
	}
	if _, writeError := filePart.Write([]byte("audio")); writeError != nil {
		testingInstance.Fatalf("write audio form: %v", writeError)
	}
	if closeError := writer.Close(); closeError != nil {
		testingInstance.Fatalf("close audio form: %v", closeError)
	}
	return body, writer.FormDataContentType()
}

func TestIntegrationRequestTimeoutHeaderValidationAndEcho(testingInstance *testing.T) {
	var upstreamCalls atomic.Int64
	router := timeoutContractRouter(
		testingInstance,
		requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
			upstreamCalls.Add(1)
			return completedTimeoutContractResponse(`{"id":"complete","status":"completed","output_text":"ok"}`), nil
		}),
		timeoutContractConfiguration(2, 3),
		zap.NewNop().Sugar(),
	)
	invalidHeaders := []struct {
		name   string
		values []string
	}{
		{name: "blank", values: []string{""}},
		{name: "repeated", values: []string{"1", "2"}},
		{name: "signed", values: []string{"+1"}},
		{name: "fractional", values: []string{"1.5"}},
		{name: "nonnumeric", values: []string{"later"}},
		{name: "zero", values: []string{"0"}},
		{name: "negative", values: []string{"-1"}},
		{name: "over limit", values: []string{"4"}},
		{name: "overflow", values: []string{strings.Repeat("9", 64)}},
	}
	for _, testCase := range invalidHeaders {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=hello", nil)
			for _, value := range testCase.values {
				request.Header.Add(llmproxycontract.HeaderRequestTimeoutSeconds, value)
			}
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusBadRequest {
				subTest.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
			}
			if responseRecorder.Header().Get("Content-Type") != contentTypeJSON {
				subTest.Fatalf("content type=%q", responseRecorder.Header().Get("Content-Type"))
			}
			expectedBody := `{"error":{"code":"invalid_request_timeout","max_request_timeout_seconds":3}}`
			if responseRecorder.Body.String() != expectedBody {
				subTest.Fatalf("body=%q want=%q", responseRecorder.Body.String(), expectedBody)
			}
			if responseRecorder.Header().Get(llmproxycontract.HeaderRequestTimeoutSeconds) != "" {
				subTest.Fatalf("rejected response must not echo an effective timeout")
			}
		})
	}
	if upstreamCalls.Load() != 0 {
		testingInstance.Fatalf("invalid headers reached upstream: calls=%d", upstreamCalls.Load())
	}

	acceptedHeaders := []struct {
		name           string
		requestedValue string
		effectiveValue string
	}{
		{name: "omitted uses default", effectiveValue: "2"},
		{name: "explicit value", requestedValue: "3", effectiveValue: "3"},
	}
	for _, testCase := range acceptedHeaders {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=hello", nil)
			if testCase.requestedValue != "" {
				request.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, testCase.requestedValue)
			}
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusOK {
				subTest.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
			}
			if responseRecorder.Header().Get(llmproxycontract.HeaderRequestTimeoutSeconds) != testCase.effectiveValue {
				subTest.Fatalf(
					"effective timeout=%q want=%q",
					responseRecorder.Header().Get(llmproxycontract.HeaderRequestTimeoutSeconds),
					testCase.effectiveValue,
				)
			}
		})
	}
	if upstreamCalls.Load() != int64(len(acceptedHeaders)) {
		testingInstance.Fatalf("accepted upstream calls=%d want=%d", upstreamCalls.Load(), len(acceptedHeaders))
	}
}

func TestIntegrationRequestTimeoutAppliesToEveryUpstreamRoute(testingInstance *testing.T) {
	router := timeoutContractRouter(
		testingInstance,
		requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
			if strings.Contains(request.URL.Path, "transcriptions") {
				return completedTimeoutContractResponse(`{"text":"dictated"}`), nil
			}
			return completedTimeoutContractResponse(`{"id":"complete","status":"completed","output_text":"ok"}`), nil
		}),
		timeoutContractConfiguration(1, 3),
		zap.NewNop().Sugar(),
	)
	multipartBody, multipartContentType := timeoutContractMultipartBody(testingInstance)
	testCases := []struct {
		name        string
		method      string
		path        string
		body        io.Reader
		contentType string
	}{
		{name: "get root", method: http.MethodGet, path: "/?key=" + serviceSecretValue + "&prompt=hello"},
		{name: "post root", method: http.MethodPost, path: "/?key=" + serviceSecretValue, body: strings.NewReader(`{"prompt":"hello"}`), contentType: contentTypeJSON},
		{name: "post v2", method: http.MethodPost, path: "/v2?key=" + serviceSecretValue, body: strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`), contentType: contentTypeJSON},
		{name: "post dictate", method: http.MethodPost, path: "/dictate?key=" + serviceSecretValue, body: multipartBody, contentType: multipartContentType},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			request := httptest.NewRequest(testCase.method, testCase.path, testCase.body)
			request.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "3")
			if testCase.contentType != "" {
				request.Header.Set("Content-Type", testCase.contentType)
			}
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusOK {
				subTest.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
			}
			if responseRecorder.Header().Get(llmproxycontract.HeaderRequestTimeoutSeconds) != "3" {
				subTest.Fatalf("effective timeout=%q", responseRecorder.Header().Get(llmproxycontract.HeaderRequestTimeoutSeconds))
			}
		})
	}
}

func TestIntegrationRequestTimeoutReturnsCanonicalErrorAndSafeOutcome(testingInstance *testing.T) {
	upstreamCanceled := make(chan struct{})
	var cancelOnce sync.Once
	callerContext, cancelCaller := context.WithCancel(context.Background())
	defer cancelCaller()
	observedCore, observedLogs := observer.New(zapcore.InfoLevel)
	router := timeoutContractRouter(
		testingInstance,
		requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			cancelCaller()
			cancelOnce.Do(func() { close(upstreamCanceled) })
			return nil, request.Context().Err()
		}),
		timeoutContractConfiguration(2, 3),
		zap.New(observedCore).Sugar(),
	)
	request := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=secret-prompt", nil).WithContext(callerContext)
	request.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "1")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusGatewayTimeout {
		testingInstance.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if responseRecorder.Header().Get(llmproxycontract.HeaderRequestTimeoutSeconds) != "1" {
		testingInstance.Fatalf("effective timeout=%q", responseRecorder.Header().Get(llmproxycontract.HeaderRequestTimeoutSeconds))
	}
	if responseRecorder.Header().Get("Content-Type") != contentTypeJSON {
		testingInstance.Fatalf("content type=%q", responseRecorder.Header().Get("Content-Type"))
	}
	expectedBody := `{"error":{"code":"request_timeout","request_timeout_seconds":1}}`
	if responseRecorder.Body.String() != expectedBody {
		testingInstance.Fatalf("body=%q want=%q", responseRecorder.Body.String(), expectedBody)
	}
	select {
	case <-upstreamCanceled:
	default:
		testingInstance.Fatal("upstream context was not canceled")
	}
	outcomeEntries := observedLogs.FilterMessage("upstream request ended").All()
	if len(outcomeEntries) != 1 {
		testingInstance.Fatalf("outcome log entries=%d", len(outcomeEntries))
	}
	outcomeFields := outcomeEntries[0].ContextMap()
	if outcomeFields["request_timeout_seconds"] != int64(1) || outcomeFields["outcome"] != "proxy_timeout" {
		testingInstance.Fatalf("outcome fields=%v", outcomeFields)
	}
	for _, forbiddenField := range []string{"prompt", "audio", "credential", "response_body", "error"} {
		if _, exists := outcomeFields[forbiddenField]; exists {
			testingInstance.Fatalf("unsafe outcome field %q=%v", forbiddenField, outcomeFields[forbiddenField])
		}
	}
}

func TestIntegrationExplicitRequestTimeoutIncludesQueueWait(testingInstance *testing.T) {
	upstreamStarted := make(chan struct{})
	releaseUpstream := make(chan struct{})
	var upstreamCalls atomic.Int64
	var startOnce sync.Once
	router := timeoutContractRouter(
		testingInstance,
		requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
			upstreamCalls.Add(1)
			startOnce.Do(func() { close(upstreamStarted) })
			select {
			case <-request.Context().Done():
				return nil, request.Context().Err()
			case <-releaseUpstream:
				return completedTimeoutContractResponse(`{"id":"complete","status":"completed","output_text":"ok"}`), nil
			}
		}),
		timeoutContractConfiguration(2, 3),
		zap.NewNop().Sugar(),
	)

	firstResponse := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		request := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=first", nil)
		request.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "3")
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		firstResponse <- responseRecorder
	}()
	select {
	case <-upstreamStarted:
	case <-time.After(time.Second):
		testingInstance.Fatal("first upstream request did not start")
	}

	queuedRequest := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=queued", nil)
	queuedRequest.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "1")
	queuedResponse := httptest.NewRecorder()
	router.ServeHTTP(queuedResponse, queuedRequest)

	if queuedResponse.Code != http.StatusGatewayTimeout {
		testingInstance.Fatalf("queued status=%d body=%s", queuedResponse.Code, queuedResponse.Body.String())
	}
	if queuedResponse.Body.String() != `{"error":{"code":"request_timeout","request_timeout_seconds":1}}` {
		testingInstance.Fatalf("queued body=%q", queuedResponse.Body.String())
	}
	if queuedResponse.Header().Get(llmproxycontract.HeaderRequestTimeoutSeconds) != "1" {
		testingInstance.Fatalf(
			"queued effective timeout=%q",
			queuedResponse.Header().Get(llmproxycontract.HeaderRequestTimeoutSeconds),
		)
	}
	if upstreamCalls.Load() != 1 {
		testingInstance.Fatalf("queued request reached upstream: calls=%d", upstreamCalls.Load())
	}

	close(releaseUpstream)
	select {
	case responseRecorder := <-firstResponse:
		if responseRecorder.Code != http.StatusOK {
			testingInstance.Fatalf("first status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
		}
	case <-time.After(time.Second):
		testingInstance.Fatal("first request did not finish after upstream release")
	}
}

func TestIntegrationRequestTimeoutLogsTerminalOutcomes(testingInstance *testing.T) {
	var upstreamCall atomic.Int64
	observedCore, observedLogs := observer.New(zapcore.InfoLevel)
	router := timeoutContractRouter(
		testingInstance,
		requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
			if upstreamCall.Add(1) == 1 {
				return completedTimeoutContractResponse(`{"id":"complete","status":"completed","output_text":"ok"}`), nil
			}
			providerFailure := completedTimeoutContractResponse(`{"error":{"message":"provider failed"}}`)
			providerFailure.StatusCode = http.StatusBadRequest
			return providerFailure, nil
		}),
		timeoutContractConfiguration(2, 3),
		zap.New(observedCore).Sugar(),
	)

	successRequest := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=success", nil)
	successResponse := httptest.NewRecorder()
	router.ServeHTTP(successResponse, successRequest)
	if successResponse.Code != http.StatusOK {
		testingInstance.Fatalf("success status=%d body=%s", successResponse.Code, successResponse.Body.String())
	}

	failureRequest := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=failure", nil)
	failureResponse := httptest.NewRecorder()
	router.ServeHTTP(failureResponse, failureRequest)
	if failureResponse.Code != http.StatusBadGateway {
		testingInstance.Fatalf("failure status=%d body=%s", failureResponse.Code, failureResponse.Body.String())
	}

	callerContext, cancelCaller := context.WithCancel(context.Background())
	cancelCaller()
	canceledRequest := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=canceled", nil).WithContext(callerContext)
	canceledResponse := httptest.NewRecorder()
	router.ServeHTTP(canceledResponse, canceledRequest)
	if canceledResponse.Code != callerCancellationStatusCode {
		testingInstance.Fatalf("canceled status=%d body=%s", canceledResponse.Code, canceledResponse.Body.String())
	}

	outcomeEntries := observedLogs.FilterMessage("upstream request ended").All()
	if len(outcomeEntries) != 3 {
		testingInstance.Fatalf("outcome log entries=%d", len(outcomeEntries))
	}
	expectedOutcomes := []string{"success", "provider_failure", "caller_cancelled"}
	for entryIndex, expectedOutcome := range expectedOutcomes {
		outcomeFields := outcomeEntries[entryIndex].ContextMap()
		if outcomeFields["outcome"] != expectedOutcome {
			testingInstance.Fatalf("outcome[%d]=%v want=%q", entryIndex, outcomeFields, expectedOutcome)
		}
	}
}

func TestIntegrationRequestedTimeoutCanOutliveDefault(testingInstance *testing.T) {
	router := timeoutContractRouter(
		testingInstance,
		requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
			time.Sleep(1100 * time.Millisecond)
			return completedTimeoutContractResponse(`{"id":"complete","status":"completed","output_text":"late success"}`), nil
		}),
		timeoutContractConfiguration(1, 2),
		zap.NewNop().Sugar(),
	)

	defaultRequest := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=hello", nil)
	defaultResponse := httptest.NewRecorder()
	router.ServeHTTP(defaultResponse, defaultRequest)
	if defaultResponse.Code != http.StatusGatewayTimeout {
		testingInstance.Fatalf("default status=%d body=%s", defaultResponse.Code, defaultResponse.Body.String())
	}

	extendedRequest := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=hello", nil)
	extendedRequest.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "2")
	extendedResponse := httptest.NewRecorder()
	router.ServeHTTP(extendedResponse, extendedRequest)
	if extendedResponse.Code != http.StatusOK {
		testingInstance.Fatalf("extended status=%d body=%s", extendedResponse.Code, extendedResponse.Body.String())
	}
	if extendedResponse.Header().Get(llmproxycontract.HeaderRequestTimeoutSeconds) != "2" {
		testingInstance.Fatalf("effective timeout=%q", extendedResponse.Header().Get(llmproxycontract.HeaderRequestTimeoutSeconds))
	}
}

func TestIntegrationRequestTimeoutIncludesBodyParsingAndDictation(testingInstance *testing.T) {
	var upstreamCalls atomic.Int64
	waitForDeadline := requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
		upstreamCalls.Add(1)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	router := timeoutContractRouter(testingInstance, waitForDeadline, timeoutContractConfiguration(2, 3), zap.NewNop().Sugar())

	blockingJSONBody := newCloseAwareBlockingBody()
	jsonRequest := httptest.NewRequest(http.MethodPost, "/v2?key="+serviceSecretValue, nil)
	jsonRequest.Body = blockingJSONBody
	jsonRequest.Header.Set("Content-Type", contentTypeJSON)
	jsonRequest.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "1")
	jsonResponse := httptest.NewRecorder()
	router.ServeHTTP(jsonResponse, jsonRequest)
	if jsonResponse.Code != http.StatusGatewayTimeout {
		testingInstance.Fatalf("body parsing status=%d body=%s", jsonResponse.Code, jsonResponse.Body.String())
	}
	if upstreamCalls.Load() != 0 {
		testingInstance.Fatalf("body parsing timeout reached upstream")
	}

	blockingMultipartBody := newCloseAwareBlockingBody()
	dictationParsingRequest := httptest.NewRequest(http.MethodPost, "/dictate?key="+serviceSecretValue, nil)
	dictationParsingRequest.Body = blockingMultipartBody
	dictationParsingRequest.Header.Set("Content-Type", "multipart/form-data; boundary=blocking")
	dictationParsingRequest.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "1")
	dictationParsingResponse := httptest.NewRecorder()
	router.ServeHTTP(dictationParsingResponse, dictationParsingRequest)
	if dictationParsingResponse.Code != http.StatusGatewayTimeout {
		testingInstance.Fatalf("dictation parsing status=%d body=%s", dictationParsingResponse.Code, dictationParsingResponse.Body.String())
	}
	if upstreamCalls.Load() != 0 {
		testingInstance.Fatalf("dictation parsing timeout reached upstream")
	}

	audioBody, contentType := timeoutContractMultipartBody(testingInstance)
	dictationRequest := httptest.NewRequest(http.MethodPost, "/dictate?key="+serviceSecretValue, audioBody)
	dictationRequest.Header.Set("Content-Type", contentType)
	dictationRequest.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "1")
	dictationResponse := httptest.NewRecorder()
	router.ServeHTTP(dictationResponse, dictationRequest)
	if dictationResponse.Code != http.StatusGatewayTimeout {
		testingInstance.Fatalf("dictation provider status=%d body=%s", dictationResponse.Code, dictationResponse.Body.String())
	}
	if upstreamCalls.Load() != 1 {
		testingInstance.Fatalf("dictation upstream calls=%d want=1", upstreamCalls.Load())
	}
}

func TestIntegrationDictationSuccessAfterDeadlineIsRejected(testingInstance *testing.T) {
	router := timeoutContractRouter(
		testingInstance,
		requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
			time.Sleep(1100 * time.Millisecond)
			return completedTimeoutContractResponse(`{"text":"late dictation"}`), nil
		}),
		timeoutContractConfiguration(1, 2),
		zap.NewNop().Sugar(),
	)
	audioBody, contentType := timeoutContractMultipartBody(testingInstance)
	request := httptest.NewRequest(http.MethodPost, "/dictate?key="+serviceSecretValue, audioBody)
	request.Header.Set("Content-Type", contentType)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusGatewayTimeout {
		testingInstance.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
}

func TestIntegrationOpenAIPollingConsumesOneRequestBudget(testingInstance *testing.T) {
	var pollCalls atomic.Int64
	router := timeoutContractRouter(
		testingInstance,
		requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
			if request.Method == http.MethodPost {
				return completedTimeoutContractResponse(`{"id":"background","status":"queued"}`), nil
			}
			pollCalls.Add(1)
			return completedTimeoutContractResponse(`{"id":"background","status":"in_progress"}`), nil
		}),
		timeoutContractConfiguration(2, 3),
		zap.NewNop().Sugar(),
	)
	request := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=hello", nil)
	request.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "1")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusGatewayTimeout {
		testingInstance.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if pollCalls.Load() < 2 {
		testingInstance.Fatalf("poll calls=%d want at least 2", pollCalls.Load())
	}
}

func TestIntegrationRequestTimeoutConfigurationInvariants(testingInstance *testing.T) {
	testCases := []struct {
		name          string
		defaultValue  int
		maximumValue  int
		expectedError string
	}{
		{name: "negative default", defaultValue: -1, maximumValue: 3, expectedError: "request_timeout_seconds must be positive"},
		{name: "negative maximum", defaultValue: 1, maximumValue: -1, expectedError: "max_request_timeout_seconds must be positive"},
		{name: "default exceeds maximum", defaultValue: 3, maximumValue: 2, expectedError: "request_timeout_seconds exceeds"},
		{name: "unsupported duration", defaultValue: 1, maximumValue: int(10_000_000_000), expectedError: "exceeds supported duration"},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			_, buildError := proxy.BuildRouter(
				proxy.Configuration{
					RequestTimeoutSeconds:    testCase.defaultValue,
					MaxRequestTimeoutSeconds: testCase.maximumValue,
				},
				zap.NewNop().Sugar(),
			)
			if buildError == nil || !strings.Contains(buildError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want contains %q", buildError, testCase.expectedError)
			}
		})
	}
}
