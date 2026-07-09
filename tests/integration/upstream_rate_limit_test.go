package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	rateLimitChatPath                 = "/chat/completions"
	rateLimitDictationPath            = "/audio/transcriptions"
	rateLimitResponsesPath            = "/v1/responses"
	rateLimitTextResponse             = `{"choices":[{"message":{"content":"rate limit text ok"}}]}`
	rateLimitDictationResponse        = `{"text":"rate limit dictation ok"}`
	rateLimitOpenAIResponse           = `{"id":"rate-limit-complete","status":"completed","output_text":"rate limit retry ok"}`
	rateLimitShortInterval            = 120 * time.Millisecond
	rateLimitTimingTolerance          = 20 * time.Millisecond
	rateLimitRequestTimeoutSeconds    = 3
	rateLimitConcurrentRequestCount   = 4
	rateLimitConcurrentWindowCapacity = 2
	rateLimitCancellationTimeout      = 40 * time.Millisecond
	rateLimitAssertionTimeout         = time.Second
)

type rateLimitRequestResult struct {
	statusCode   int
	requestError error
}

func TestIntegrationSharedUpstreamRateLimitAppliesToTextAndDictationAndLogsDelay(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	var callMutex sync.Mutex
	callTimes := make([]time.Time, 0, 2)
	callPaths := make([]string, 0, 2)
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		callMutex.Lock()
		callTimes = append(callTimes, time.Now())
		callPaths = append(callPaths, httpRequest.URL.Path)
		callMutex.Unlock()
		writeRateLimitUpstreamResponse(responseWriter, httpRequest.URL.Path)
	}))
	testingInstance.Cleanup(upstreamServer.Close)

	observedCore, observedLogs := observer.New(zapcore.DebugLevel)
	loggerInstance := zap.New(observedCore)
	testingInstance.Cleanup(func() { _ = loggerInstance.Sync() })
	configuration := rateLimitIntegrationConfiguration(upstreamServer.URL)
	configuration.UpstreamRateLimits = []proxy.UpstreamRateLimitConfiguration{{
		Origin:      upstreamServer.URL,
		MaxRequests: 1,
		Interval:    rateLimitShortInterval.String(),
	}}
	router := buildRateLimitIntegrationRouter(testingInstance, configuration, loggerInstance.Sugar())
	applicationServer := httptest.NewServer(router)
	testingInstance.Cleanup(applicationServer.Close)

	textStatus, textError := performRateLimitTextRequest(applicationServer.Client(), applicationServer.URL, proxy.ProviderNameDeepSeek)
	if textError != nil || textStatus != http.StatusOK {
		testingInstance.Fatalf("text status=%d error=%v", textStatus, textError)
	}
	dictationStatus, dictationError := performRateLimitDictationRequest(applicationServer.Client(), applicationServer.URL)
	if dictationError != nil || dictationStatus != http.StatusOK {
		testingInstance.Fatalf("dictation status=%d error=%v", dictationStatus, dictationError)
	}

	callMutex.Lock()
	recordedTimes := append([]time.Time(nil), callTimes...)
	recordedPaths := append([]string(nil), callPaths...)
	callMutex.Unlock()
	if len(recordedTimes) != 2 || recordedPaths[0] != rateLimitChatPath || recordedPaths[1] != rateLimitDictationPath {
		testingInstance.Fatalf("paths=%v times=%v", recordedPaths, recordedTimes)
	}
	minimumGap := rateLimitShortInterval - rateLimitTimingTolerance
	if actualGap := recordedTimes[1].Sub(recordedTimes[0]); actualGap < minimumGap {
		testingInstance.Fatalf("shared text/dictation gap=%s want>=%s", actualGap, minimumGap)
	}

	delayLogs := observedLogs.FilterMessage(constants.LogEventUpstreamRateLimitDelayed).All()
	if len(delayLogs) != 1 {
		testingInstance.Fatalf("delay log count=%d want=1", len(delayLogs))
	}
	logFields := delayLogs[0].ContextMap()
	if logFields[constants.LogFieldUpstreamOrigin] != upstreamServer.URL || fmt.Sprint(logFields[constants.LogFieldRateLimitMaxRequests]) != "1" || logFields[constants.LogFieldRateLimitInterval] != rateLimitShortInterval.String() {
		testingInstance.Fatalf("delay log fields=%v", logFields)
	}
}

func TestIntegrationSharedUpstreamRateLimitIsConcurrencySafe(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	var callMutex sync.Mutex
	callTimes := make([]time.Time, 0, rateLimitConcurrentRequestCount)
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		callMutex.Lock()
		callTimes = append(callTimes, time.Now())
		callMutex.Unlock()
		writeRateLimitUpstreamResponse(responseWriter, httpRequest.URL.Path)
	}))
	testingInstance.Cleanup(upstreamServer.Close)

	configuration := rateLimitIntegrationConfiguration(upstreamServer.URL)
	configuration.UpstreamRateLimits = []proxy.UpstreamRateLimitConfiguration{{
		Origin:      upstreamServer.URL,
		MaxRequests: rateLimitConcurrentWindowCapacity,
		Interval:    rateLimitShortInterval.String(),
	}}
	router := buildRateLimitIntegrationRouter(testingInstance, configuration, newLogger(testingInstance))
	applicationServer := httptest.NewServer(router)
	testingInstance.Cleanup(applicationServer.Close)

	startRequests := make(chan struct{})
	results := make(chan rateLimitRequestResult, rateLimitConcurrentRequestCount)
	var requestWaitGroup sync.WaitGroup
	requestWaitGroup.Add(rateLimitConcurrentRequestCount)
	for requestIndex := 0; requestIndex < rateLimitConcurrentRequestCount; requestIndex++ {
		go func() {
			defer requestWaitGroup.Done()
			<-startRequests
			statusCode, requestError := performRateLimitTextRequest(applicationServer.Client(), applicationServer.URL, proxy.ProviderNameDeepSeek)
			results <- rateLimitRequestResult{statusCode: statusCode, requestError: requestError}
		}()
	}
	close(startRequests)
	requestWaitGroup.Wait()
	close(results)
	for result := range results {
		if result.requestError != nil || result.statusCode != http.StatusOK {
			testingInstance.Fatalf("concurrent status=%d error=%v", result.statusCode, result.requestError)
		}
	}

	callMutex.Lock()
	recordedTimes := append([]time.Time(nil), callTimes...)
	callMutex.Unlock()
	if len(recordedTimes) != rateLimitConcurrentRequestCount {
		testingInstance.Fatalf("call count=%d want=%d", len(recordedTimes), rateLimitConcurrentRequestCount)
	}
	sort.Slice(recordedTimes, func(firstIndex int, secondIndex int) bool {
		return recordedTimes[firstIndex].Before(recordedTimes[secondIndex])
	})
	minimumGap := rateLimitShortInterval - rateLimitTimingTolerance
	if firstWindowGap := recordedTimes[rateLimitConcurrentWindowCapacity].Sub(recordedTimes[0]); firstWindowGap < minimumGap {
		testingInstance.Fatalf("first rolling window gap=%s want>=%s", firstWindowGap, minimumGap)
	}
	if secondWindowGap := recordedTimes[rateLimitConcurrentWindowCapacity+1].Sub(recordedTimes[1]); secondWindowGap < minimumGap {
		testingInstance.Fatalf("second rolling window gap=%s want>=%s", secondWindowGap, minimumGap)
	}
}

func TestIntegrationUpstreamRateLimitsAreIndependentByOrigin(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	firstUpstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		writeRateLimitUpstreamResponse(responseWriter, httpRequest.URL.Path)
	}))
	testingInstance.Cleanup(firstUpstreamServer.Close)
	secondUpstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		writeRateLimitUpstreamResponse(responseWriter, httpRequest.URL.Path)
	}))
	testingInstance.Cleanup(secondUpstreamServer.Close)

	configuration := rateLimitIntegrationConfiguration(firstUpstreamServer.URL)
	configuration.DashScopeBaseURL = secondUpstreamServer.URL
	configuration.UpstreamRateLimits = []proxy.UpstreamRateLimitConfiguration{
		{Origin: firstUpstreamServer.URL, MaxRequests: 1, Interval: "2s"},
		{Origin: secondUpstreamServer.URL, MaxRequests: 1, Interval: "2s"},
	}
	router := buildRateLimitIntegrationRouter(testingInstance, configuration, newLogger(testingInstance))
	applicationServer := httptest.NewServer(router)
	testingInstance.Cleanup(applicationServer.Close)

	firstStatus, firstError := performRateLimitTextRequest(applicationServer.Client(), applicationServer.URL, proxy.ProviderNameDeepSeek)
	if firstError != nil || firstStatus != http.StatusOK {
		testingInstance.Fatalf("first origin status=%d error=%v", firstStatus, firstError)
	}
	independentClient := &http.Client{Timeout: 500 * time.Millisecond}
	secondStatus, secondError := performRateLimitTextRequest(independentClient, applicationServer.URL, proxy.ProviderNameDashScope)
	if secondError != nil || secondStatus != http.StatusOK {
		testingInstance.Fatalf("second origin status=%d error=%v", secondStatus, secondError)
	}
}

func TestIntegrationUpstreamRateLimitCountsOpenAIRetries(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	var callMutex sync.Mutex
	callTimes := make([]time.Time, 0, 2)
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		if httpRequest.URL.Path != rateLimitResponsesPath {
			http.NotFound(responseWriter, httpRequest)
			return
		}
		callMutex.Lock()
		callTimes = append(callTimes, time.Now())
		callCount := len(callTimes)
		callMutex.Unlock()
		if callCount == 1 {
			responseWriter.WriteHeader(http.StatusInternalServerError)
			return
		}
		responseWriter.Header().Set("Content-Type", contentTypeJSON)
		_, _ = io.WriteString(responseWriter, rateLimitOpenAIResponse)
	}))
	testingInstance.Cleanup(upstreamServer.Close)

	configuration := rateLimitIntegrationConfiguration(upstreamServer.URL)
	configuration.UpstreamRateLimits = []proxy.UpstreamRateLimitConfiguration{{
		Origin:      upstreamServer.URL,
		MaxRequests: 1,
		Interval:    "900ms",
	}}
	router := buildRateLimitIntegrationRouter(testingInstance, configuration, newLogger(testingInstance))
	applicationServer := httptest.NewServer(router)
	testingInstance.Cleanup(applicationServer.Close)

	statusCode, requestError := performRateLimitTextRequest(applicationServer.Client(), applicationServer.URL, proxy.ProviderNameOpenAI)
	if requestError != nil || statusCode != http.StatusOK {
		testingInstance.Fatalf("retry status=%d error=%v", statusCode, requestError)
	}
	callMutex.Lock()
	recordedTimes := append([]time.Time(nil), callTimes...)
	callMutex.Unlock()
	if len(recordedTimes) != 2 {
		testingInstance.Fatalf("retry call count=%d want=2", len(recordedTimes))
	}
	if retryGap := recordedTimes[1].Sub(recordedTimes[0]); retryGap < 800*time.Millisecond {
		testingInstance.Fatalf("retry gap=%s want>=800ms", retryGap)
	}
}

func TestIntegrationUpstreamRateLimitDisabledPreservesConcurrentTextAndDictation(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	arrivals := make(chan struct{}, 2)
	releaseUpstream := make(chan struct{})
	var releaseOnce sync.Once
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		arrivals <- struct{}{}
		<-releaseUpstream
		writeRateLimitUpstreamResponse(responseWriter, httpRequest.URL.Path)
	}))
	testingInstance.Cleanup(upstreamServer.Close)
	testingInstance.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseUpstream) })
	})

	configuration := rateLimitIntegrationConfiguration(upstreamServer.URL)
	configuration.WorkerCount = 2
	configuration.QueueSize = 2
	configuration.LogLevel = ""
	router := buildRateLimitIntegrationRouter(testingInstance, configuration, nil)
	applicationServer := httptest.NewServer(router)
	testingInstance.Cleanup(applicationServer.Close)

	results := make(chan rateLimitRequestResult, 2)
	go func() {
		statusCode, requestError := performRateLimitTextRequest(applicationServer.Client(), applicationServer.URL, proxy.ProviderNameDeepSeek)
		results <- rateLimitRequestResult{statusCode: statusCode, requestError: requestError}
	}()
	go func() {
		statusCode, requestError := performRateLimitDictationRequest(applicationServer.Client(), applicationServer.URL)
		results <- rateLimitRequestResult{statusCode: statusCode, requestError: requestError}
	}()

	for arrivalIndex := 0; arrivalIndex < 2; arrivalIndex++ {
		select {
		case <-arrivals:
		case <-time.After(rateLimitAssertionTimeout):
			testingInstance.Fatal("text and dictation did not reach the unrestricted shared HTTP client concurrently")
		}
	}
	releaseOnce.Do(func() { close(releaseUpstream) })
	for resultIndex := 0; resultIndex < 2; resultIndex++ {
		select {
		case result := <-results:
			if result.requestError != nil || result.statusCode != http.StatusOK {
				testingInstance.Fatalf("unrestricted status=%d error=%v", result.statusCode, result.requestError)
			}
		case <-time.After(rateLimitAssertionTimeout):
			testingInstance.Fatal("unrestricted request did not complete after upstream release")
		}
	}
}

func TestIntegrationUpstreamRateLimitCancellationReturnsGatewayTimeoutAndLogs(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	var callMutex sync.Mutex
	upstreamCallCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		callMutex.Lock()
		upstreamCallCount++
		callMutex.Unlock()
		writeRateLimitUpstreamResponse(responseWriter, httpRequest.URL.Path)
	}))
	testingInstance.Cleanup(upstreamServer.Close)

	observedCore, observedLogs := observer.New(zapcore.DebugLevel)
	loggerInstance := zap.New(observedCore)
	testingInstance.Cleanup(func() { _ = loggerInstance.Sync() })
	configuration := rateLimitIntegrationConfiguration(upstreamServer.URL)
	configuration.UpstreamRateLimits = []proxy.UpstreamRateLimitConfiguration{{
		Origin:      upstreamServer.URL,
		MaxRequests: 1,
		Interval:    "2s",
	}}
	router := buildRateLimitIntegrationRouter(testingInstance, configuration, loggerInstance.Sugar())
	applicationServer := httptest.NewServer(router)
	testingInstance.Cleanup(applicationServer.Close)
	firstStatus, firstError := performRateLimitTextRequest(applicationServer.Client(), applicationServer.URL, proxy.ProviderNameDeepSeek)
	if firstError != nil || firstStatus != http.StatusOK {
		testingInstance.Fatalf("first status=%d error=%v", firstStatus, firstError)
	}

	cancelingGateway := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		requestContext, cancelRequest := context.WithTimeout(httpRequest.Context(), rateLimitCancellationTimeout)
		defer cancelRequest()
		router.ServeHTTP(responseWriter, httpRequest.WithContext(requestContext))
	}))
	testingInstance.Cleanup(cancelingGateway.Close)
	secondStatus, secondError := performRateLimitTextRequest(cancelingGateway.Client(), cancelingGateway.URL, proxy.ProviderNameDeepSeek)
	if secondError != nil || secondStatus != http.StatusGatewayTimeout {
		testingInstance.Fatalf("canceled status=%d error=%v", secondStatus, secondError)
	}

	callMutex.Lock()
	actualUpstreamCallCount := upstreamCallCount
	callMutex.Unlock()
	if actualUpstreamCallCount != 1 {
		testingInstance.Fatalf("upstream call count=%d want=1", actualUpstreamCallCount)
	}
	if len(observedLogs.FilterMessage(constants.LogEventUpstreamRateLimitDelayed).All()) != 1 {
		testingInstance.Fatal("expected one delayed rate-limit log")
	}
	if len(observedLogs.FilterMessage(constants.LogEventUpstreamRateLimitCanceled).All()) != 1 {
		testingInstance.Fatal("expected one canceled rate-limit log")
	}
}

func rateLimitIntegrationConfiguration(upstreamURL string) proxy.Configuration {
	return proxy.Configuration{
		Tenants:                 proxy.SingleTenantConfigurations("integration", serviceSecretValue),
		OpenAIKey:               openAIKeyValue,
		DeepSeekKey:             "sk-deepseek",
		DashScopeKey:            "sk-dashscope",
		OpenAIBaseURL:           upstreamURL + "/v1",
		OpenAITranscriptionsURL: upstreamURL + rateLimitDictationPath,
		DeepSeekBaseURL:         upstreamURL,
		DashScopeBaseURL:        upstreamURL,
		LogLevel:                logLevelDebug,
		WorkerCount:             rateLimitConcurrentRequestCount,
		QueueSize:               rateLimitConcurrentRequestCount,
		RequestTimeoutSeconds:   rateLimitRequestTimeoutSeconds,
	}
}

func buildRateLimitIntegrationRouter(testingInstance *testing.T, configuration proxy.Configuration, structuredLogger *zap.SugaredLogger) http.Handler {
	testingInstance.Helper()
	previousHTTPClient := proxy.HTTPClient
	proxy.HTTPClient = &http.Client{Timeout: time.Duration(rateLimitRequestTimeoutSeconds) * time.Second}
	testingInstance.Cleanup(func() { proxy.HTTPClient = previousHTTPClient })
	router, buildError := proxy.BuildRouter(integrationConfiguration(testingInstance, configuration), structuredLogger)
	if buildError != nil {
		testingInstance.Fatalf(buildRouterFailedFormat, buildError)
	}
	return router
}

func performRateLimitTextRequest(httpClient *http.Client, applicationURL string, providerName string) (int, error) {
	requestURL, parseError := url.Parse(applicationURL)
	if parseError != nil {
		return 0, parseError
	}
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	queryValues.Set("provider", providerName)
	requestURL.RawQuery = queryValues.Encode()
	httpResponse, requestError := httpClient.Get(requestURL.String())
	if requestError != nil {
		return 0, requestError
	}
	defer httpResponse.Body.Close()
	_, _ = io.Copy(io.Discard, httpResponse.Body)
	return httpResponse.StatusCode, nil
}

func performRateLimitDictationRequest(httpClient *http.Client, applicationURL string) (int, error) {
	requestBody := &bytes.Buffer{}
	multipartWriter := multipart.NewWriter(requestBody)
	audioPart, createError := multipartWriter.CreateFormFile("audio", "recording.webm")
	if createError != nil {
		return 0, createError
	}
	if _, writeError := audioPart.Write([]byte("audio")); writeError != nil {
		return 0, writeError
	}
	if closeError := multipartWriter.Close(); closeError != nil {
		return 0, closeError
	}
	requestURL := applicationURL + "/dictate?key=" + url.QueryEscape(serviceSecretValue) + "&provider=" + proxy.ProviderNameOpenAI
	httpRequest, requestError := http.NewRequest(http.MethodPost, requestURL, requestBody)
	if requestError != nil {
		return 0, requestError
	}
	httpRequest.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	httpResponse, responseError := httpClient.Do(httpRequest)
	if responseError != nil {
		return 0, responseError
	}
	defer httpResponse.Body.Close()
	_, _ = io.Copy(io.Discard, httpResponse.Body)
	return httpResponse.StatusCode, nil
}

func writeRateLimitUpstreamResponse(responseWriter http.ResponseWriter, requestPath string) {
	responseWriter.Header().Set("Content-Type", contentTypeJSON)
	switch requestPath {
	case rateLimitChatPath:
		_, _ = io.WriteString(responseWriter, rateLimitTextResponse)
	case rateLimitDictationPath:
		_, _ = io.WriteString(responseWriter, rateLimitDictationResponse)
	default:
		http.Error(responseWriter, "unexpected upstream path", http.StatusNotFound)
	}
}
