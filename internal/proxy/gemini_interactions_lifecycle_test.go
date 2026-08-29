package proxy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	testGeminiProgressEvent         = "proxy provider progress"
	testGeminiFailureEvent          = "provider failure"
	testGeminiSummaryEvent          = "proxy request phase summary"
	testGeminiCreateProgress        = "gemini_create"
	testGeminiPollProgress          = "gemini_poll"
	testGeminiCancelProgress        = "gemini_cancel"
	testGeminiDeleteProgress        = "gemini_delete"
	testGeminiContinuationProgress  = "continuation_attempt"
	testGeminiProviderErrorCodesKey = "provider_error_codes"
	testGeminiErrorCodeURIBase      = "https://gemini-errors.provider.invalid/interactions/problems/"
)

func TestGeminiInteractionsTerminalCodesDriveSafeRetryAndI045AtPublicBoundary(testingInstance *testing.T) {
	for _, testCase := range []struct {
		name      string
		errorCode string
		retryable bool
	}{
		{name: "retryable URI code", errorCode: testGeminiErrorCodeURIBase + "rate_limit_exceeded", retryable: true},
		{name: "non-retryable URI code", errorCode: testGeminiErrorCodeURIBase + "content_blocked"},
		{name: "unknown URI code fails closed", errorCode: testGeminiErrorCodeURIBase + "future_provider_fault"},
	} {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			const interactionIdentifier = "private-terminal-code-interaction"
			const privateProviderMessage = "private provider diagnostic must not escape"
			var observedRequests []string
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				observedRequests = append(observedRequests, request.Method+" "+request.URL.Path)
				assertGeminiInteractionHeaders(subTest, request, testGeminiKey)
				switch {
				case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath:
					writeGeminiInteractionSnapshot(subTest, responseWriter, interactionIdentifier, "in_progress", "", nil)
				case request.Method == http.MethodGet && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
					writeGeminiInteractionSnapshotWithErrors(
						subTest,
						responseWriter,
						interactionIdentifier,
						"failed",
						"private failed output",
						&testGeminiInteractionUsage{Input: 4, Output: 5, Total: 9},
						[]testGeminiInteractionError{{Code: testCase.errorCode, Message: privateProviderMessage}},
					)
				case request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
					writeGeminiInteractionDeleted(subTest, responseWriter)
				default:
					subTest.Fatalf("unexpected Gemini Interactions request=%s %s", request.Method, request.URL.Path)
				}
			}))
			subTest.Cleanup(upstreamServer.Close)

			observedCore, observedLogs := observer.New(zapcore.InfoLevel)
			router := geminiInteractionsTestRouterWithLogger(subTest, upstreamServer.URL, zap.New(observedCore).Sugar())
			response := performGeminiInteractionsRequest(subTest, router, context.Background())
			assertProviderErrorResponse(subTest, response, providerErrorExpectation{
				statusCode: http.StatusBadGateway,
				errorCode:  llmproxycontract.ErrorCodeProviderError,
				provider:   proxy.ProviderNameGemini,
				retryable:  testCase.retryable,
				rawBody:    privateProviderMessage,
			})
			assertOpenAPIProviderErrorResponse(subTest, "/", http.MethodGet, response)
			if strings.Contains(response.Body.String(), testCase.errorCode) || strings.Contains(response.Body.String(), interactionIdentifier) {
				subTest.Fatalf("public response exposed provider diagnostic body=%q", response.Body.String())
			}

			requestID := response.Header().Get(llmproxycontract.HeaderRequestID)
			progressEntries := geminiProgressEntriesForRequest(observedLogs, requestID)
			if len(progressEntries) != 4 {
				subTest.Fatalf("Gemini progress entries=%d want=4 logs=%v", len(progressEntries), observedLogs.All())
			}
			assertGeminiProgress(subTest, progressEntries[0].ContextMap(), testGeminiCreateProgress, "in_progress", "pending", nil)
			assertGeminiProgress(subTest, progressEntries[1].ContextMap(), testGeminiPollProgress, "failed", "failure", []string{testCase.errorCode})
			assertGeminiProgress(subTest, progressEntries[2].ContextMap(), testGeminiDeleteProgress, "deleted", "complete", nil)
			assertGeminiProgress(subTest, progressEntries[3].ContextMap(), testGeminiContinuationProgress, "", "failure", nil)

			failureEntries := observedLogs.FilterMessage(testGeminiFailureEvent).All()
			if len(failureEntries) != 1 {
				subTest.Fatalf("provider failure entries=%d want=1 logs=%v", len(failureEntries), observedLogs.All())
			}
			failureFields := failureEntries[0].ContextMap()
			if failureFields["request_id"] != requestID || failureFields["retryable"] != testCase.retryable {
				subTest.Fatalf("provider failure fields=%v", failureFields)
			}
			assertGeminiProviderErrorCodes(subTest, failureFields, []string{testCase.errorCode})
			if _, present := failureFields["upstream_status"]; present {
				subTest.Fatalf("terminal condition invented upstream status fields=%v", failureFields)
			}
			if summaries := geminiSummaryEntriesForRequest(observedLogs, requestID); len(summaries) != 1 {
				subTest.Fatalf("terminal summaries=%d want=1 logs=%v", len(summaries), observedLogs.All())
			}
			expectedRequests := []string{
				http.MethodPost + " " + testGeminiInteractionsPath,
				http.MethodGet + " " + testGeminiInteractionsPath + "/" + interactionIdentifier,
				http.MethodDelete + " " + testGeminiInteractionsPath + "/" + interactionIdentifier,
			}
			if strings.Join(observedRequests, "|") != strings.Join(expectedRequests, "|") {
				subTest.Fatalf("terminal requests=%v want=%v", observedRequests, expectedRequests)
			}
			assertGeminiLogsExcludeValues(subTest, observedLogs, privateProviderMessage, interactionIdentifier, "private failed output", testGeminiKey, TestSecret)
		})
	}
}

func TestGeminiInteractionsRejectMalformedTerminalCodeWithoutLoggingIt(testingInstance *testing.T) {
	const interactionIdentifier = "private-malformed-code-interaction"
	const malformedCode = "rate_limit_exceeded"
	deleteCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath:
			writeGeminiInteractionSnapshotWithErrors(
				testingInstance,
				responseWriter,
				interactionIdentifier,
				"failed",
				"",
				nil,
				[]testGeminiInteractionError{{Code: malformedCode, Message: "private malformed code message"}},
			)
		case request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			deleteCount++
			writeGeminiInteractionDeleted(testingInstance, responseWriter)
		default:
			testingInstance.Fatalf("unexpected Gemini Interactions request=%s %s", request.Method, request.URL.Path)
		}
	}))
	testingInstance.Cleanup(upstreamServer.Close)

	observedCore, observedLogs := observer.New(zapcore.InfoLevel)
	response := performGeminiInteractionsRequest(
		testingInstance,
		geminiInteractionsTestRouterWithLogger(testingInstance, upstreamServer.URL, zap.New(observedCore).Sugar()),
		context.Background(),
	)
	assertProviderErrorResponse(testingInstance, response, providerErrorExpectation{
		statusCode: http.StatusBadGateway,
		errorCode:  llmproxycontract.ErrorCodeProviderError,
		provider:   proxy.ProviderNameGemini,
		rawBody:    malformedCode,
	})
	if deleteCount != 1 {
		testingInstance.Fatalf("malformed code cleanup deletes=%d want=1", deleteCount)
	}
	for _, entry := range observedLogs.All() {
		if _, present := entry.ContextMap()[testGeminiProviderErrorCodesKey]; present {
			testingInstance.Fatalf("malformed provider code reached telemetry entry=%v", entry)
		}
	}
	assertGeminiLogsExcludeValues(testingInstance, observedLogs, malformedCode, interactionIdentifier, "private malformed code message")
}

func TestGeminiInteractionsCleanupFailureHasDistinctI045Operation(testingInstance *testing.T) {
	const interactionIdentifier = "private-cleanup-failure-interaction"
	const completedText = "completed output must not escape"
	const privateDeleteBody = "private deletion failure"
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath:
			writeGeminiInteractionSnapshot(testingInstance, responseWriter, interactionIdentifier, "in_progress", "", nil)
		case request.Method == http.MethodGet && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			writeGeminiInteractionSnapshot(testingInstance, responseWriter, interactionIdentifier, "completed", completedText, nil)
		case request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			http.Error(responseWriter, privateDeleteBody, http.StatusServiceUnavailable)
		default:
			testingInstance.Fatalf("unexpected Gemini Interactions request=%s %s", request.Method, request.URL.Path)
		}
	}))
	testingInstance.Cleanup(upstreamServer.Close)

	observedCore, observedLogs := observer.New(zapcore.InfoLevel)
	response := performGeminiInteractionsRequest(
		testingInstance,
		geminiInteractionsTestRouterWithLogger(testingInstance, upstreamServer.URL, zap.New(observedCore).Sugar()),
		context.Background(),
	)
	upstreamStatus := http.StatusServiceUnavailable
	assertProviderErrorResponse(testingInstance, response, providerErrorExpectation{
		statusCode:     http.StatusBadGateway,
		errorCode:      llmproxycontract.ErrorCodeProviderError,
		provider:       proxy.ProviderNameGemini,
		upstreamStatus: &upstreamStatus,
		retryable:      true,
		rawBody:        privateDeleteBody,
	})
	requestID := response.Header().Get(llmproxycontract.HeaderRequestID)
	progressEntries := geminiProgressEntriesForRequest(observedLogs, requestID)
	if len(progressEntries) != 4 {
		testingInstance.Fatalf("cleanup progress entries=%d want=4 logs=%v", len(progressEntries), observedLogs.All())
	}
	assertGeminiProgress(testingInstance, progressEntries[1].ContextMap(), testGeminiPollProgress, "completed", "complete", nil)
	assertGeminiProgress(testingInstance, progressEntries[2].ContextMap(), testGeminiDeleteProgress, "unknown", "failure", nil)
	assertGeminiLogsExcludeValues(testingInstance, observedLogs, privateDeleteBody, interactionIdentifier, completedText)
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
	observedCore, observedLogs := observer.New(zapcore.InfoLevel)
	response := performGeminiInteractionsRequest(testingInstance, geminiInteractionsTestRouterWithLogger(testingInstance, upstreamServer.URL, zap.New(observedCore).Sugar()), requestContext)
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
	requestID := response.Header().Get(llmproxycontract.HeaderRequestID)
	cancelProgress := geminiProgressForKind(testingInstance, observedLogs, requestID, testGeminiCancelProgress)
	assertGeminiProgress(testingInstance, cancelProgress, testGeminiCancelProgress, "cancelled", "canceled", nil)
	deleteProgress := geminiProgressForKind(testingInstance, observedLogs, requestID, testGeminiDeleteProgress)
	assertGeminiProgress(testingInstance, deleteProgress, testGeminiDeleteProgress, "deleted", "complete", nil)
}

func TestGeminiInteractionsReconcilesTransientVisibilitySequenceAtPublicBoundary(testingInstance *testing.T) {
	const interactionIdentifier = "delayed-visible-interaction"
	const completedText = "visible Gemini response"
	pollCount := 0
	deleteCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		assertGeminiInteractionHeaders(testingInstance, request, testGeminiKey)
		switch {
		case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath:
			writeGeminiInteractionSnapshot(testingInstance, responseWriter, interactionIdentifier, "in_progress", "", nil)
		case request.Method == http.MethodGet && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			pollCount++
			switch pollCount {
			case 1:
				http.Error(responseWriter, "private visibility permission error", http.StatusForbidden)
			case 2:
				http.Error(responseWriter, "private visibility argument error", http.StatusBadRequest)
			case 3:
				http.Error(responseWriter, "private visibility missing error", http.StatusNotFound)
			default:
				writeGeminiInteractionSnapshot(testingInstance, responseWriter, interactionIdentifier, "completed", completedText, nil)
			}
		case request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			deleteCount++
			writeGeminiInteractionDeleted(testingInstance, responseWriter)
		default:
			testingInstance.Fatalf("unexpected Gemini Interactions request=%s %s", request.Method, request.URL.Path)
		}
	}))
	testingInstance.Cleanup(upstreamServer.Close)

	response := performGeminiInteractionsRequest(
		testingInstance,
		geminiInteractionsTestRouterWithVisibilityInterval(testingInstance, upstreamServer.URL),
		context.Background(),
	)
	var responseBody struct {
		Response string `json:"response"`
	}
	decodeError := json.Unmarshal(response.Body.Bytes(), &responseBody)
	if response.Code != http.StatusOK || decodeError != nil || responseBody.Response != completedText || pollCount != 4 || deleteCount != 1 {
		testingInstance.Fatalf(
			"status=%d body=%q decode_error=%v polls=%d deletes=%d",
			response.Code,
			response.Body.String(),
			decodeError,
			pollCount,
			deleteCount,
		)
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
	return geminiInteractionsTestRouterWithLogger(testingInstance, baseURL, zap.NewNop().Sugar())
}

func geminiInteractionsTestRouterWithLogger(testingInstance testing.TB, baseURL string, structuredLogger *zap.SugaredLogger) http.Handler {
	testingInstance.Helper()
	return geminiInteractionsTestRouterWithCatalogAndLogger(testingInstance, baseURL, testfixtures.ProviderCatalog(testingInstance), structuredLogger)
}

func geminiInteractionsTestRouterWithVisibilityInterval(testingInstance testing.TB, baseURL string) http.Handler {
	testingInstance.Helper()
	return geminiInteractionsTestRouterWithCatalog(
		testingInstance,
		baseURL,
		testfixtures.ProviderCatalogWithResourceVisibilityInterval(testingInstance, proxy.ProviderNameGemini, testVisibilityRetryIntervalMS),
	)
}

func geminiInteractionsTestRouterWithCatalog(testingInstance testing.TB, baseURL string, providerCatalog *proxy.ProviderCatalog) http.Handler {
	testingInstance.Helper()
	return geminiInteractionsTestRouterWithCatalogAndLogger(testingInstance, baseURL, providerCatalog, zap.NewNop().Sugar())
}

func geminiInteractionsTestRouterWithCatalogAndLogger(testingInstance testing.TB, baseURL string, providerCatalog *proxy.ProviderCatalog, structuredLogger *zap.SugaredLogger) http.Handler {
	testingInstance.Helper()
	router, buildError := buildRouterWithCatalogs(testingInstance, proxy.Configuration{
		Endpoints:             providerEndpoints(baseURL, proxy.ProviderNameGemini),
		ProviderCatalog:       providerCatalog,
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, structuredLogger)
	if buildError != nil {
		testingInstance.Fatalf(messageBuildRouterError, buildError)
	}
	return router
}

func geminiProgressEntriesForRequest(observedLogs *observer.ObservedLogs, requestID string) []observer.LoggedEntry {
	entries := []observer.LoggedEntry{}
	for _, entry := range observedLogs.FilterMessage(testGeminiProgressEvent).All() {
		if entry.ContextMap()["request_id"] == requestID {
			entries = append(entries, entry)
		}
	}
	return entries
}

func geminiSummaryEntriesForRequest(observedLogs *observer.ObservedLogs, requestID string) []observer.LoggedEntry {
	entries := []observer.LoggedEntry{}
	for _, entry := range observedLogs.FilterMessage(testGeminiSummaryEvent).All() {
		if entry.ContextMap()["request_id"] == requestID {
			entries = append(entries, entry)
		}
	}
	return entries
}

func geminiProgressForKind(testingInstance testing.TB, observedLogs *observer.ObservedLogs, requestID string, progressKind string) map[string]any {
	testingInstance.Helper()
	for _, entry := range geminiProgressEntriesForRequest(observedLogs, requestID) {
		if entry.ContextMap()["progress_kind"] == progressKind {
			return entry.ContextMap()
		}
	}
	testingInstance.Fatalf("missing Gemini progress kind=%q logs=%v", progressKind, observedLogs.All())
	return nil
}

func assertGeminiProgress(testingInstance testing.TB, fields map[string]any, progressKind string, providerState string, completionSignal string, providerErrorCodes []string) {
	testingInstance.Helper()
	if fields["progress_kind"] != progressKind || fields["completion_signal"] != completionSignal {
		testingInstance.Fatalf("Gemini progress fields=%v", fields)
	}
	if providerState != "" && fields["provider_state"] != providerState {
		testingInstance.Fatalf("Gemini progress state=%v want=%q", fields, providerState)
	}
	assertGeminiProviderErrorCodes(testingInstance, fields, providerErrorCodes)
}

func assertGeminiProviderErrorCodes(testingInstance testing.TB, fields map[string]any, expectedCodes []string) {
	testingInstance.Helper()
	rawCodes, present := fields[testGeminiProviderErrorCodesKey]
	if len(expectedCodes) == 0 {
		if present {
			testingInstance.Fatalf("unexpected provider error codes=%v", fields)
		}
		return
	}
	actualCodes := []string{}
	switch typedCodes := rawCodes.(type) {
	case []string:
		actualCodes = typedCodes
	case []any:
		for _, rawCode := range typedCodes {
			errorCode, codeIsString := rawCode.(string)
			if !codeIsString {
				testingInstance.Fatalf("provider error code=%v has type %T", rawCode, rawCode)
			}
			actualCodes = append(actualCodes, errorCode)
		}
	}
	if !present || strings.Join(actualCodes, "|") != strings.Join(expectedCodes, "|") {
		testingInstance.Fatalf("provider error codes=%v want=%v fields=%v", rawCodes, expectedCodes, fields)
	}
}

func assertGeminiLogsExcludeValues(testingInstance testing.TB, observedLogs *observer.ObservedLogs, forbiddenValues ...string) {
	testingInstance.Helper()
	for _, entry := range observedLogs.All() {
		loggedValue := entry.Message + fmt.Sprint(entry.ContextMap())
		for _, forbiddenValue := range forbiddenValues {
			if strings.Contains(loggedValue, forbiddenValue) {
				testingInstance.Fatalf("Gemini telemetry exposed forbidden value=%q entry=%s", forbiddenValue, loggedValue)
			}
		}
	}
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
