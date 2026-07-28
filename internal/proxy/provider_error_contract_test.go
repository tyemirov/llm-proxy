package proxy_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type publicProviderErrorEnvelope struct {
	Error struct {
		Code           string  `json:"code"`
		Provider       string  `json:"provider"`
		UpstreamStatus *int    `json:"upstream_status"`
		Retryable      bool    `json:"retryable"`
		RequestID      string  `json:"request_id"`
		RetryAfter     *string `json:"retry_after"`
	} `json:"error"`
}

type providerErrorExpectation struct {
	statusCode     int
	errorCode      string
	provider       string
	upstreamStatus *int
	retryable      bool
	retryAfter     *string
	rawBody        string
}

func TestProviderErrorContractSanitizesEveryTextTransport(t *testing.T) {
	const rawProviderBody = `{"private":"provider-body-must-not-escape"}`
	retryAfterDate := "Sun, 06 Nov 1994 08:49:37 GMT"
	testCases := []struct {
		name               string
		provider           string
		upstreamStatus     int
		upstreamRetryAfter string
		wantStatus         int
		wantCode           string
		wantRetryable      bool
		wantRetryAfter     *string
		truncateBody       bool
		configure          func(*proxy.Configuration, string)
	}{
		{
			name:               "OpenAI Responses truncated error body",
			provider:           proxy.ProviderNameOpenAI,
			upstreamStatus:     http.StatusBadRequest,
			upstreamRetryAfter: "provider-controlled",
			wantStatus:         http.StatusBadGateway,
			wantCode:           llmproxycontract.ErrorCodeProviderError,
			truncateBody:       true,
			configure: func(configuration *proxy.Configuration, baseURL string) {
				configuration.OpenAIBaseURL = baseURL
			},
		},
		{
			name:               "Anthropic Messages",
			provider:           proxy.ProviderNameAnthropic,
			upstreamStatus:     http.StatusServiceUnavailable,
			upstreamRetryAfter: "000120",
			wantStatus:         http.StatusBadGateway,
			wantCode:           llmproxycontract.ErrorCodeProviderError,
			wantRetryable:      true,
			wantRetryAfter:     stringPointer("120"),
			configure: func(configuration *proxy.Configuration, baseURL string) {
				configuration.AnthropicKey = testAnthropicKey
				configuration.AnthropicBaseURL = baseURL
			},
		},
		{
			name:               "Gemini generateContent",
			provider:           proxy.ProviderNameGemini,
			upstreamStatus:     http.StatusTooManyRequests,
			upstreamRetryAfter: "Sunday, 06-Nov-94 08:49:37 GMT",
			wantStatus:         http.StatusTooManyRequests,
			wantCode:           llmproxycontract.ErrorCodeProviderRateLimited,
			wantRetryable:      true,
			wantRetryAfter:     &retryAfterDate,
			configure: func(configuration *proxy.Configuration, baseURL string) {
				configuration.GeminiKey = testGeminiKey
				configuration.GeminiBaseURL = baseURL
			},
		},
		{
			name:               "Meta chat completions truncated error body",
			provider:           proxy.ProviderNameMeta,
			upstreamStatus:     http.StatusInternalServerError,
			upstreamRetryAfter: "11",
			wantStatus:         http.StatusBadGateway,
			wantCode:           llmproxycontract.ErrorCodeProviderError,
			wantRetryable:      true,
			wantRetryAfter:     stringPointer("11"),
			truncateBody:       true,
			configure: func(configuration *proxy.Configuration, baseURL string) {
				configuration.MetaKey = testMetaKey
				configuration.MetaBaseURL = baseURL
			},
		},
		{
			name:               "Moonshot chat completions",
			provider:           proxy.ProviderNameMoonshot,
			upstreamStatus:     http.StatusTooManyRequests,
			upstreamRetryAfter: "7",
			wantStatus:         http.StatusTooManyRequests,
			wantCode:           llmproxycontract.ErrorCodeProviderRateLimited,
			wantRetryable:      true,
			wantRetryAfter:     stringPointer("7"),
			configure: func(configuration *proxy.Configuration, baseURL string) {
				configuration.MoonshotKey = "sk-moonshot"
				configuration.MoonshotBaseURL = baseURL
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
				responseWriter.Header().Set("Content-Type", "application/json")
				responseWriter.Header().Set("Retry-After", testCase.upstreamRetryAfter)
				if testCase.truncateBody {
					responseWriter.Header().Set("Content-Length", strconv.Itoa(len(rawProviderBody)+1))
				}
				responseWriter.WriteHeader(testCase.upstreamStatus)
				_, _ = responseWriter.Write([]byte(rawProviderBody))
			}))
			subTest.Cleanup(upstreamServer.Close)

			configuration := providerErrorTestConfiguration()
			testCase.configure(&configuration, upstreamServer.URL)
			router, buildError := buildRouterWithCatalogs(subTest, configuration, zap.NewNop().Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			queryParameters := url.Values{}
			queryParameters.Set("key", TestSecret)
			queryParameters.Set("prompt", "hello")
			queryParameters.Set("provider", testCase.provider)
			request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
			responseRecorder := httptest.NewRecorder()
			router.ServeHTTP(responseRecorder, request)

			assertProviderErrorResponse(subTest, responseRecorder, providerErrorExpectation{
				statusCode:     testCase.wantStatus,
				errorCode:      testCase.wantCode,
				provider:       testCase.provider,
				upstreamStatus: intPointer(testCase.upstreamStatus),
				retryable:      testCase.wantRetryable,
				retryAfter:     testCase.wantRetryAfter,
				rawBody:        rawProviderBody,
			})
			assertOpenAPIProviderErrorResponse(subTest, "/", http.MethodGet, responseRecorder)
		})
	}
}

func TestProviderErrorContractRepresentsProtocolFailureWithoutInventingUpstreamStatus(t *testing.T) {
	const malformedProviderBody = `{private-provider-body`
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		responseWriter.Header().Set("Retry-After", "9")
		_, _ = responseWriter.Write([]byte(malformedProviderBody))
	}))
	t.Cleanup(upstreamServer.Close)

	configuration := providerErrorTestConfiguration()
	configuration.AnthropicKey = testAnthropicKey
	configuration.AnthropicBaseURL = upstreamServer.URL
	router, buildError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=anthropic", nil)
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, request)

	assertProviderErrorResponse(t, responseRecorder, providerErrorExpectation{
		statusCode: http.StatusBadGateway,
		errorCode:  llmproxycontract.ErrorCodeProviderError,
		provider:   proxy.ProviderNameAnthropic,
		rawBody:    malformedProviderBody,
	})
	assertOpenAPIProviderErrorResponse(t, "/", http.MethodGet, responseRecorder)
}

func TestProviderErrorContractPreservesOpenAIPollFailureMetadata(t *testing.T) {
	const rawProviderBody = `{"private":"poll-provider-body"}`
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/responses":
			_, _ = responseWriter.Write([]byte(`{"id":"poll-response-id","status":"queued"}`))
		case request.Method == http.MethodGet && request.URL.Path == "/responses/poll-response-id":
			responseWriter.Header().Set("Retry-After", "5")
			responseWriter.WriteHeader(http.StatusBadRequest)
			_, _ = responseWriter.Write([]byte(rawProviderBody))
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	t.Cleanup(upstreamServer.Close)

	configuration := providerErrorTestConfiguration()
	configuration.OpenAIBaseURL = upstreamServer.URL
	router, buildError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=openai", nil)
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, request)

	assertProviderErrorResponse(t, responseRecorder, providerErrorExpectation{
		statusCode:     http.StatusBadGateway,
		errorCode:      llmproxycontract.ErrorCodeProviderError,
		provider:       proxy.ProviderNameOpenAI,
		upstreamStatus: intPointer(http.StatusBadRequest),
		retryAfter:     stringPointer("5"),
		rawBody:        rawProviderBody,
	})
	assertOpenAPIProviderErrorResponse(t, "/", http.MethodGet, responseRecorder)
}

func TestProviderErrorContractAppliesToDictation(t *testing.T) {
	const rawProviderBody = `{"private":"dictation-provider-body"}`
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.Header().Set("Retry-After", "3")
		responseWriter.WriteHeader(http.StatusTooManyRequests)
		_, _ = responseWriter.Write([]byte(rawProviderBody))
	}))
	t.Cleanup(upstreamServer.Close)

	configuration := providerErrorTestConfiguration()
	configuration.OpenAITranscriptionsURL = upstreamServer.URL
	configuration.MaxInputAudioBytes = 1024
	router, buildError := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	requestBody := &bytes.Buffer{}
	multipartWriter := multipart.NewWriter(requestBody)
	audioPart, createError := multipartWriter.CreateFormFile("audio", "audio.webm")
	if createError != nil {
		t.Fatalf("create audio part: %v", createError)
	}
	_, _ = audioPart.Write([]byte("audio"))
	_ = multipartWriter.Close()
	request := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, requestBody)
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, request)

	assertProviderErrorResponse(t, responseRecorder, providerErrorExpectation{
		statusCode:     http.StatusTooManyRequests,
		errorCode:      llmproxycontract.ErrorCodeProviderRateLimited,
		provider:       proxy.ProviderNameOpenAI,
		upstreamStatus: intPointer(http.StatusTooManyRequests),
		retryable:      true,
		retryAfter:     stringPointer("3"),
		rawBody:        rawProviderBody,
	})
	assertOpenAPIProviderErrorResponse(t, "/dictate", http.MethodPost, responseRecorder)
}

func TestProviderErrorContractLogsOnlySafeCorrelationMetadata(t *testing.T) {
	const rawProviderBody = `{private-do-not-log`
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(rawProviderBody))
	}))
	t.Cleanup(upstreamServer.Close)

	observedCore, observedLogs := observer.New(zap.DebugLevel)
	configuration := providerErrorTestConfiguration()
	configuration.LogLevel = proxy.LogLevelDebug
	configuration.OpenAIBaseURL = upstreamServer.URL
	router, buildError := buildRouterWithCatalogs(t, configuration, zap.New(observedCore).Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}
	request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=openai", nil)
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, request)

	providerFailureLogs := observedLogs.FilterMessage("provider failure").All()
	if len(providerFailureLogs) != 1 {
		t.Fatalf("provider failure logs=%d want=1 logs=%v", len(providerFailureLogs), observedLogs.All())
	}
	logContext := providerFailureLogs[0].ContextMap()
	if logContext["provider"] != proxy.ProviderNameOpenAI ||
		logContext["retryable"] != false ||
		logContext["request_id"] != responseRecorder.Header().Get(llmproxycontract.HeaderRequestID) {
		t.Fatalf("provider failure log=%v", logContext)
	}
	if _, exposesUpstreamStatus := logContext["upstream_status"]; exposesUpstreamStatus {
		t.Fatalf("provider failure log invented upstream status=%v", logContext)
	}
	if _, exposesRetryAfter := logContext["retry_after"]; exposesRetryAfter {
		t.Fatalf("provider failure log invented retry-after=%v", logContext)
	}
	if _, exposesError := logContext["error"]; exposesError {
		t.Fatalf("provider failure log exposed error=%v", logContext)
	}
	if _, exposesBody := logContext["response_body"]; exposesBody {
		t.Fatalf("provider failure log exposed response body=%v", logContext)
	}
	for _, logEntry := range observedLogs.All() {
		contextBytes, _ := json.Marshal(logEntry.ContextMap())
		if strings.Contains(logEntry.Message, rawProviderBody) || strings.Contains(string(contextBytes), rawProviderBody) {
			t.Fatalf("structured log exposed provider body: message=%q context=%s", logEntry.Message, contextBytes)
		}
	}
}

func providerErrorTestConfiguration() proxy.Configuration {
	return proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:             TestAPIKey,
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}
}

func assertOpenAPIProviderErrorResponse(t *testing.T, path string, method string, responseRecorder *httptest.ResponseRecorder) {
	t.Helper()
	contract, loadError := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if loadError != nil {
		t.Fatalf("load canonical OpenAPI contract: %v", loadError)
	}
	if validationError := contract.ValidateResponse(path, method, responseRecorder.Code, responseRecorder.Header(), responseRecorder.Body.Bytes()); validationError != nil {
		t.Fatalf("provider error violates OpenAPI: %v body=%s", validationError, responseRecorder.Body.String())
	}
}

func assertProviderErrorResponse(t *testing.T, responseRecorder *httptest.ResponseRecorder, expectation providerErrorExpectation) {
	t.Helper()
	if responseRecorder.Code != expectation.statusCode {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, expectation.statusCode, responseRecorder.Body.String())
	}
	if !strings.HasPrefix(responseRecorder.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("content-type=%q want application/json", responseRecorder.Header().Get("Content-Type"))
	}

	responseBytes := responseRecorder.Body.Bytes()
	var response publicProviderErrorEnvelope
	decoder := json.NewDecoder(bytes.NewReader(responseBytes))
	decoder.DisallowUnknownFields()
	if decodeError := decoder.Decode(&response); decodeError != nil {
		t.Fatalf("decode provider error: %v body=%s", decodeError, responseBytes)
	}
	if response.Error.Code != expectation.errorCode ||
		response.Error.Provider != expectation.provider ||
		!equalOptionalInt(response.Error.UpstreamStatus, expectation.upstreamStatus) ||
		response.Error.Retryable != expectation.retryable ||
		!equalOptionalString(response.Error.RetryAfter, expectation.retryAfter) {
		t.Fatalf("provider error=%+v expectation=%+v", response.Error, expectation)
	}
	requestIDHeader := responseRecorder.Header().Get(llmproxycontract.HeaderRequestID)
	if requestIDHeader == "" || response.Error.RequestID != requestIDHeader {
		t.Fatalf("request_id=%q header=%q", response.Error.RequestID, requestIDHeader)
	}
	retryAfterHeader := responseRecorder.Header().Get("Retry-After")
	if expectation.retryAfter == nil && retryAfterHeader != "" {
		t.Fatalf("retry-after header=%q want absent", retryAfterHeader)
	}
	if expectation.retryAfter != nil && retryAfterHeader != *expectation.retryAfter {
		t.Fatalf("retry-after header=%q want=%q", retryAfterHeader, *expectation.retryAfter)
	}
	if strings.Contains(string(responseBytes), expectation.rawBody) {
		t.Fatalf("provider error exposed raw body: %s", responseBytes)
	}
}

func intPointer(value int) *int {
	return &value
}

func stringPointer(value string) *string {
	return &value
}

func equalOptionalInt(actual *int, expected *int) bool {
	if actual == nil || expected == nil {
		return actual == nil && expected == nil
	}
	return *actual == *expected
}

func equalOptionalString(actual *string, expected *string) bool {
	if actual == nil || expected == nil {
		return actual == nil && expected == nil
	}
	return *actual == *expected
}
