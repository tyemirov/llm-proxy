package proxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

func TestGeminiInteractionsSynchronousModelsUseNonstoredImmediateResponses(testingInstance *testing.T) {
	requestCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestCount++
		assertGeminiInteractionHeaders(testingInstance, request, testGeminiKey)
		if request.Method != http.MethodPost || request.URL.Path != testGeminiInteractionsPath {
			testingInstance.Fatalf("unexpected Gemini Interactions request=%s %s", request.Method, request.URL.Path)
		}
		payload := decodeGeminiInteractionRequest(testingInstance, request)
		if payload["model"] != proxy.ModelNameGemini25Flash || payload["background"] != false || payload["store"] != false {
			testingInstance.Fatalf("synchronous Gemini payload=%v", payload)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"synchronous interaction ok"}]}]}`))
	}))
	testingInstance.Cleanup(upstreamServer.Close)

	response := performGeminiInteractionsRequestForModel(
		testingInstance,
		geminiInteractionsTestRouter(testingInstance, upstreamServer.URL),
		context.Background(),
		proxy.ModelNameGemini25Flash,
	)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "synchronous interaction ok") || requestCount != 1 {
		testingInstance.Fatalf("synchronous Gemini status=%d body=%q requests=%d", response.Code, response.Body.String(), requestCount)
	}
}

func TestGeminiInteractionsSynchronousActiveStatusIsSafeProviderError(testingInstance *testing.T) {
	requestCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestCount++
		assertGeminiInteractionHeaders(testingInstance, request, testGeminiKey)
		writeGeminiInteractionSnapshot(testingInstance, responseWriter, "", "in_progress", "private active text", nil)
	}))
	testingInstance.Cleanup(upstreamServer.Close)

	response := performGeminiInteractionsRequestForModel(
		testingInstance,
		geminiInteractionsTestRouter(testingInstance, upstreamServer.URL),
		context.Background(),
		proxy.ModelNameGemini25Flash,
	)
	assertProviderErrorResponse(testingInstance, response, providerErrorExpectation{
		statusCode: http.StatusBadGateway,
		errorCode:  llmproxycontract.ErrorCodeProviderError,
		provider:   proxy.ProviderNameGemini,
		rawBody:    "private active text",
	})
	if requestCount != 1 {
		testingInstance.Fatalf("synchronous active requests=%d", requestCount)
	}
}

func TestGeminiInteractionsTerminalFailuresAreSafeAndDeletedAtPublicBoundary(testingInstance *testing.T) {
	for _, testCase := range []struct {
		name   string
		status string
	}{
		{name: "failed", status: "failed"},
		{name: "cancelled", status: "cancelled"},
		{name: "budget exceeded", status: "budget_exceeded"},
		{name: "requires action", status: "requires_action"},
		{name: "unknown terminal state", status: "provider_private_state"},
	} {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			const interactionIdentifier = "private-terminal-interaction"
			const privateModelText = "private terminal model text"
			deleteCount := 0
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				assertGeminiInteractionHeaders(subTest, request, testGeminiKey)
				switch {
				case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath:
					writeGeminiInteractionSnapshot(subTest, responseWriter, interactionIdentifier, testCase.status, privateModelText, &testGeminiInteractionUsage{Input: 7, Output: 9, Total: 19})
				case request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
					deleteCount++
					writeGeminiInteractionDeleted(subTest, responseWriter)
				default:
					subTest.Fatalf("unexpected Gemini Interactions request=%s %s", request.Method, request.URL.Path)
				}
			}))
			subTest.Cleanup(upstreamServer.Close)

			router := geminiInteractionsTestRouter(subTest, upstreamServer.URL)
			response := performGeminiInteractionsRequest(subTest, router, context.Background())
			assertProviderErrorResponse(subTest, response, providerErrorExpectation{
				statusCode: http.StatusBadGateway,
				errorCode:  llmproxycontract.ErrorCodeProviderError,
				provider:   proxy.ProviderNameGemini,
				rawBody:    privateModelText,
			})
			if deleteCount != 1 ||
				strings.Contains(response.Body.String(), interactionIdentifier) ||
				strings.Contains(response.Body.String(), testCase.status) {
				subTest.Fatalf("unsafe terminal response=%q deletes=%d", response.Body.String(), deleteCount)
			}
		})
	}
}

func TestGeminiInteractionsDeletionFailureSuppressesSuccessfulOutput(testingInstance *testing.T) {
	const interactionIdentifier = "delete-failure-interaction"
	const privateDeleteBody = "private delete failure"
	const completedText = "must not escape after failed deletion"
	deleteCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		assertGeminiInteractionHeaders(testingInstance, request, testGeminiKey)
		switch {
		case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath:
			writeGeminiInteractionSnapshot(testingInstance, responseWriter, interactionIdentifier, "completed", completedText, &testGeminiInteractionUsage{Input: 3, Output: 4, Total: 7})
		case request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			deleteCount++
			http.Error(responseWriter, privateDeleteBody, http.StatusServiceUnavailable)
		default:
			testingInstance.Fatalf("unexpected Gemini Interactions request=%s %s", request.Method, request.URL.Path)
		}
	}))
	testingInstance.Cleanup(upstreamServer.Close)

	response := performGeminiInteractionsRequest(testingInstance, geminiInteractionsTestRouter(testingInstance, upstreamServer.URL), context.Background())
	upstreamStatus := http.StatusServiceUnavailable
	assertProviderErrorResponse(testingInstance, response, providerErrorExpectation{
		statusCode:     http.StatusBadGateway,
		errorCode:      llmproxycontract.ErrorCodeProviderError,
		provider:       proxy.ProviderNameGemini,
		upstreamStatus: &upstreamStatus,
		retryable:      true,
		rawBody:        privateDeleteBody,
	})
	if deleteCount != 1 || strings.Contains(response.Body.String(), completedText) {
		testingInstance.Fatalf("delete failure response=%q deletes=%d", response.Body.String(), deleteCount)
	}
}

func TestGeminiInteractionsCancellationCancelsThenDeletesProviderResource(testingInstance *testing.T) {
	const interactionIdentifier = "cancelled-by-client"
	pollStarted := make(chan struct{})
	var observedRequests []string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		observedRequests = append(observedRequests, request.Method+" "+request.URL.Path)
		assertGeminiInteractionHeaders(testingInstance, request, testGeminiKey)
		switch {
		case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath:
			writeGeminiInteractionSnapshot(testingInstance, responseWriter, interactionIdentifier, "queued", "", nil)
		case request.Method == http.MethodGet && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			close(pollStarted)
			<-request.Context().Done()
		case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier+"/cancel":
			writeGeminiInteractionSnapshot(testingInstance, responseWriter, interactionIdentifier, "cancelled", "", nil)
		case request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			writeGeminiInteractionDeleted(testingInstance, responseWriter)
		default:
			testingInstance.Fatalf("unexpected Gemini Interactions request=%s %s", request.Method, request.URL.Path)
		}
	}))
	testingInstance.Cleanup(upstreamServer.Close)

	requestContext, cancelRequest := context.WithCancel(context.Background())
	cancelComplete := make(chan struct{})
	go func() {
		<-pollStarted
		cancelRequest()
		close(cancelComplete)
	}()
	response := performGeminiInteractionsRequest(testingInstance, geminiInteractionsTestRouter(testingInstance, upstreamServer.URL), requestContext)
	<-cancelComplete
	cancelRequest()
	if response.Code != 499 ||
		strings.Contains(response.Body.String(), interactionIdentifier) ||
		strings.Contains(response.Body.String(), testGeminiKey) {
		testingInstance.Fatalf("cancellation status=%d body=%q", response.Code, response.Body.String())
	}
	expectedRequests := []string{
		http.MethodPost + " " + testGeminiInteractionsPath,
		http.MethodGet + " " + testGeminiInteractionsPath + "/" + interactionIdentifier,
		http.MethodPost + " " + testGeminiInteractionsPath + "/" + interactionIdentifier + "/cancel",
		http.MethodDelete + " " + testGeminiInteractionsPath + "/" + interactionIdentifier,
	}
	if strings.Join(observedRequests, "|") != strings.Join(expectedRequests, "|") {
		testingInstance.Fatalf("cancellation requests=%v want=%v", observedRequests, expectedRequests)
	}
}

func TestGeminiInteractionsAlreadyCancelledContextSkipsPollingAndCleansUp(testingInstance *testing.T) {
	const interactionIdentifier = "cancelled-before-poll"
	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	var observedRequests []string
	previousHTTPClient := proxy.HTTPClient
	testingInstance.Cleanup(func() { proxy.HTTPClient = previousHTTPClient })
	proxy.HTTPClient = coverageHTTPDoer(func(request *http.Request) (*http.Response, error) {
		observedRequests = append(observedRequests, request.Method+" "+request.URL.Path)
		assertGeminiInteractionHeaders(testingInstance, request, testGeminiKey)
		switch {
		case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath:
			cancelRequest()
			return coverageHTTPResponse(http.StatusOK, `{"id":"`+interactionIdentifier+`","status":"queued"}`), nil
		case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier+"/cancel":
			return coverageHTTPResponse(http.StatusOK, `{"id":"`+interactionIdentifier+`","status":"cancelled"}`), nil
		case request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			return coverageHTTPResponse(http.StatusOK, `{}`), nil
		default:
			testingInstance.Fatalf("unexpected Gemini Interactions request=%s %s", request.Method, request.URL.Path)
			return nil, nil
		}
	})
	router := geminiInteractionsTestRouter(testingInstance, "https://gemini.invalid")
	proxy.HTTPClient = previousHTTPClient

	response := performGeminiInteractionsRequest(testingInstance, router, requestContext)
	if response.Code != 499 {
		testingInstance.Fatalf("cancellation status=%d body=%q", response.Code, response.Body.String())
	}
	expectedRequests := []string{
		http.MethodPost + " " + testGeminiInteractionsPath,
		http.MethodPost + " " + testGeminiInteractionsPath + "/" + interactionIdentifier + "/cancel",
		http.MethodDelete + " " + testGeminiInteractionsPath + "/" + interactionIdentifier,
	}
	if strings.Join(observedRequests, "|") != strings.Join(expectedRequests, "|") {
		testingInstance.Fatalf("cancellation requests=%v want=%v", observedRequests, expectedRequests)
	}
}

func TestGeminiInteractionsPollFailureStillDeletesWhenCancellationFails(testingInstance *testing.T) {
	const interactionIdentifier = "poll-failure-interaction"
	const privatePollBody = "private poll failure"
	const privateCancelBody = "private cancel failure"
	var observedRequests []string
	var cancelRequestContext context.Context
	var deleteRequestContext context.Context
	deleteRequestStartedWithLiveContext := false
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		observedRequests = append(observedRequests, request.Method+" "+request.URL.Path)
		assertGeminiInteractionHeaders(testingInstance, request, testGeminiKey)
		switch {
		case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath:
			writeGeminiInteractionSnapshot(testingInstance, responseWriter, interactionIdentifier, "in_progress", "", nil)
		case request.Method == http.MethodGet && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			http.Error(responseWriter, privatePollBody, http.StatusInternalServerError)
		case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier+"/cancel":
			http.Error(responseWriter, privateCancelBody, http.StatusInternalServerError)
		case request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			writeGeminiInteractionDeleted(testingInstance, responseWriter)
		default:
			testingInstance.Fatalf("unexpected Gemini Interactions request=%s %s", request.Method, request.URL.Path)
		}
	}))
	testingInstance.Cleanup(upstreamServer.Close)
	previousHTTPClient := proxy.HTTPClient
	testingInstance.Cleanup(func() { proxy.HTTPClient = previousHTTPClient })
	proxy.HTTPClient = coverageHTTPDoer(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/cancel"):
			cancelRequestContext = request.Context()
		case request.Method == http.MethodDelete:
			deleteRequestContext = request.Context()
			deleteRequestStartedWithLiveContext = deleteRequestContext.Err() == nil
		}
		return upstreamServer.Client().Do(request)
	})
	router := geminiInteractionsTestRouter(testingInstance, upstreamServer.URL)
	proxy.HTTPClient = previousHTTPClient

	response := performGeminiInteractionsRequest(testingInstance, router, context.Background())
	upstreamStatus := http.StatusInternalServerError
	assertProviderErrorResponse(testingInstance, response, providerErrorExpectation{
		statusCode:     http.StatusBadGateway,
		errorCode:      llmproxycontract.ErrorCodeProviderError,
		provider:       proxy.ProviderNameGemini,
		upstreamStatus: &upstreamStatus,
		retryable:      true,
		rawBody:        privatePollBody,
	})
	if strings.Contains(response.Body.String(), privateCancelBody) {
		testingInstance.Fatalf("cancel error escaped response=%q", response.Body.String())
	}
	expectedRequests := []string{
		http.MethodPost + " " + testGeminiInteractionsPath,
		http.MethodGet + " " + testGeminiInteractionsPath + "/" + interactionIdentifier,
		http.MethodPost + " " + testGeminiInteractionsPath + "/" + interactionIdentifier + "/cancel",
		http.MethodDelete + " " + testGeminiInteractionsPath + "/" + interactionIdentifier,
	}
	if strings.Join(observedRequests, "|") != strings.Join(expectedRequests, "|") {
		testingInstance.Fatalf("poll failure requests=%v want=%v", observedRequests, expectedRequests)
	}
	if cancelRequestContext == nil || deleteRequestContext == nil {
		testingInstance.Fatalf("missing cleanup contexts cancel=%v delete=%v", cancelRequestContext, deleteRequestContext)
	}
	if cancelRequestContext == deleteRequestContext {
		testingInstance.Fatal("cancel and delete reused one cleanup context")
	}
	if !deleteRequestStartedWithLiveContext {
		testingInstance.Fatal("delete cleanup started with an exhausted context")
	}
}

func geminiInteractionsTestRouter(testingInstance testing.TB, baseURL string) http.Handler {
	testingInstance.Helper()
	router, buildError := buildRouterWithCatalogs(testingInstance, proxy.Configuration{
		GeminiBaseURL:         baseURL,
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		testingInstance.Fatalf(messageBuildRouterError, buildError)
	}
	return router
}

func performGeminiInteractionsRequest(testingInstance testing.TB, router http.Handler, requestContext context.Context) *httptest.ResponseRecorder {
	return performGeminiInteractionsRequestForModel(testingInstance, router, requestContext, proxy.ModelNameGemini35Flash)
}

func performGeminiInteractionsRequestForModel(testingInstance testing.TB, router http.Handler, requestContext context.Context, model string) *httptest.ResponseRecorder {
	testingInstance.Helper()
	request := httptest.NewRequest(
		http.MethodGet,
		"/?key="+TestSecret+"&prompt=hello&provider="+proxy.ProviderNameGemini+"&model="+model+"&format=application/json",
		nil,
	).WithContext(requestContext)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
