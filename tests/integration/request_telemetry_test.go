package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	telemetrySummaryEvent         = "proxy request phase summary"
	telemetryProgressEvent        = "proxy provider progress"
	telemetryOpenAICreateProgress = "openai_create"
	telemetryOpenAIPollProgress   = "openai_poll"
	telemetryContinuationProgress = "continuation_attempt"
	telemetryPollTolerance        = 450 * time.Millisecond
	telemetryProviderDelay        = 25 * time.Millisecond
	telemetryQueueHold            = 100 * time.Millisecond
	telemetryRateLimitInterval    = 140 * time.Millisecond
	telemetryRateLimitTolerance   = 100 * time.Millisecond
	telemetrySafeOutput           = "telemetry-output-sentinel"
	telemetryUnsafePrompt         = "telemetry-prompt-sentinel"
	telemetryUnsafeUpstreamID     = "telemetry-upstream-id-sentinel"
	telemetryUnsafeProviderBody   = "telemetry-provider-body-sentinel"
)

func TestIntegrationRequestTelemetryCorrelatesOpenAIPollingAndTerminalPhases(testingInstance *testing.T) {
	var upstreamCalls atomic.Int64
	observedCore, observedLogs := observer.New(zapcore.InfoLevel)
	router := timeoutContractRouter(
		testingInstance,
		requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
			time.Sleep(telemetryProviderDelay)
			callIndex := upstreamCalls.Add(1)
			switch callIndex {
			case 1:
				return completedTimeoutContractResponse(`{"id":"` + telemetryUnsafeUpstreamID + `","status":"queued"}`), nil
			case 2:
				return completedTimeoutContractResponse(`{"id":"` + telemetryUnsafeUpstreamID + `","status":"in_progress"}`), nil
			default:
				return completedTimeoutContractResponse(`{"id":"` + telemetryUnsafeUpstreamID + `","status":"completed","output_text":"` + telemetrySafeOutput + `"}`), nil
			}
		}),
		timeoutContractConfiguration(2, 3),
		zap.New(observedCore).Sugar(),
	)
	request := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt="+telemetryUnsafePrompt, nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != telemetrySafeOutput {
		testingInstance.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	requestID := response.Header().Get(llmproxycontract.HeaderRequestID)
	if requestID == "" {
		testingInstance.Fatal("response omitted proxy request id")
	}
	progressEntries := observedLogs.FilterMessage(telemetryProgressEvent).All()
	if len(progressEntries) != 4 {
		testingInstance.Fatalf("progress entries=%d want=4: %v", len(progressEntries), observedLogs.All())
	}
	expectedProgress := []struct {
		kind             string
		state            string
		signal           string
		attemptCount     int64
		pollCount        int64
		currentBytes     int64
		accumulatedBytes int64
	}{
		{kind: telemetryOpenAICreateProgress, state: "queued", signal: "pending", attemptCount: 1},
		{kind: telemetryOpenAIPollProgress, state: "in_progress", signal: "pending", pollCount: 1},
		{kind: telemetryOpenAIPollProgress, state: "completed", signal: "complete", pollCount: 2, currentBytes: int64(len(telemetrySafeOutput)), accumulatedBytes: int64(len(telemetrySafeOutput))},
		{kind: telemetryContinuationProgress, signal: "complete", attemptCount: 1, currentBytes: int64(len(telemetrySafeOutput)), accumulatedBytes: int64(len(telemetrySafeOutput))},
	}
	for entryIndex, expected := range expectedProgress {
		fields := progressEntries[entryIndex].ContextMap()
		if fields["request_id"] != requestID || fields["provider"] != proxy.ProviderNameOpenAI || fields["model"] != proxy.ModelNameGPT41 || fields["progress_kind"] != expected.kind || fields["completion_signal"] != expected.signal {
			testingInstance.Fatalf("progress[%d]=%v", entryIndex, fields)
		}
		if expected.state != "" && fields["provider_state"] != expected.state {
			testingInstance.Fatalf("progress[%d] state=%v want=%q", entryIndex, fields["provider_state"], expected.state)
		}
		if telemetryNumericField(fields, "attempt_count") != expected.attemptCount || telemetryNumericField(fields, "poll_count") != expected.pollCount || telemetryNumericField(fields, "current_output_bytes") != expected.currentBytes || telemetryNumericField(fields, "accumulated_output_bytes") != expected.accumulatedBytes {
			testingInstance.Fatalf("progress[%d] counters=%v", entryIndex, fields)
		}
		if telemetryNumericField(fields, "elapsed_ms") < 0 {
			testingInstance.Fatalf("progress[%d] elapsed=%v", entryIndex, fields)
		}
	}
	summary := telemetrySummaryForRequest(testingInstance, observedLogs, requestID)
	assertTelemetrySummaryIdentity(testingInstance, summary, "/", proxy.ProviderNameOpenAI, proxy.ModelNameGPT41, 2, http.StatusOK, "success")
	assertTelemetrySummaryPhases(testingInstance, summary)
	if telemetryNumericField(summary, "provider_http_ms") < (3*telemetryProviderDelay).Milliseconds()-15 || telemetryNumericField(summary, "provider_poll_wait_ms") < telemetryPollTolerance.Milliseconds() {
		testingInstance.Fatalf("polling phase summary=%v", summary)
	}
	if telemetryNumericField(summary, "upstream_rate_limit_wait_ms") != 0 || telemetryNumericField(summary, "continuation_wait_ms") != 0 || telemetryNumericField(summary, "managed_usage_enqueue_ms") != 0 {
		testingInstance.Fatalf("unused phases were not zero: %v", summary)
	}
	assertTelemetryLogsExcludeContent(
		testingInstance,
		observedLogs,
		telemetryUnsafePrompt,
		telemetrySafeOutput,
		telemetryUnsafeUpstreamID,
		serviceSecretValue,
		openAIKeyValue,
	)
}

func TestIntegrationRequestTelemetryMarksOnlyRetriedVisibilityErrorPending(testingInstance *testing.T) {
	testCases := []struct {
		name            string
		pollResponses   []string
		pollStatuses    []int
		expectedSignals []string
	}{
		{
			name: "reconciliation visibility error",
			pollResponses: []string{
				`{"error":"` + telemetryUnsafeProviderBody + `"}`,
				`{"error":"` + telemetryUnsafeProviderBody + `"}`,
			},
			pollStatuses:    []int{http.StatusForbidden, http.StatusForbidden},
			expectedSignals: []string{"pending", "failure"},
		},
		{
			name: "later poll visibility error",
			pollResponses: []string{
				`{"id":"` + telemetryUnsafeUpstreamID + `","status":"in_progress"}`,
				`{"error":"` + telemetryUnsafeProviderBody + `"}`,
			},
			pollStatuses:    []int{http.StatusOK, http.StatusNotFound},
			expectedSignals: []string{"pending", "failure"},
		},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			var upstreamCalls atomic.Int64
			observedCore, observedLogs := observer.New(zapcore.InfoLevel)
			router := timeoutContractRouter(
				subTest,
				requestTimeoutHTTPDoer(func(*http.Request) (*http.Response, error) {
					callIndex := int(upstreamCalls.Add(1))
					if callIndex == 1 {
						return completedTimeoutContractResponse(`{"id":"` + telemetryUnsafeUpstreamID + `","status":"queued"}`), nil
					}
					response := completedTimeoutContractResponse(testCase.pollResponses[callIndex-2])
					response.StatusCode = testCase.pollStatuses[callIndex-2]
					return response, nil
				}),
				timeoutContractConfiguration(5, 5),
				zap.New(observedCore).Sugar(),
			)
			request := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt="+telemetryUnsafePrompt, nil)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != http.StatusBadGateway || upstreamCalls.Load() != 3 {
				subTest.Fatalf("status=%d calls=%d body=%s", response.Code, upstreamCalls.Load(), response.Body.String())
			}
			requestID := response.Header().Get(llmproxycontract.HeaderRequestID)
			pollSignals := make([]string, 0, len(testCase.expectedSignals))
			for _, entry := range observedLogs.FilterMessage(telemetryProgressEvent).All() {
				fields := entry.ContextMap()
				if fields["request_id"] == requestID && fields["progress_kind"] == telemetryOpenAIPollProgress {
					completionSignal, signalIsString := fields["completion_signal"].(string)
					if !signalIsString {
						subTest.Fatalf("poll completion signal=%v", fields["completion_signal"])
					}
					pollSignals = append(pollSignals, completionSignal)
				}
			}
			if len(pollSignals) != len(testCase.expectedSignals) {
				subTest.Fatalf("poll completion signals=%v want=%v", pollSignals, testCase.expectedSignals)
			}
			for signalIndex, expectedSignal := range testCase.expectedSignals {
				if pollSignals[signalIndex] != expectedSignal {
					subTest.Fatalf("poll completion signals=%v want=%v", pollSignals, testCase.expectedSignals)
				}
			}
			summary := telemetrySummaryForRequest(subTest, observedLogs, requestID)
			assertTelemetrySummaryIdentity(subTest, summary, "/", proxy.ProviderNameOpenAI, proxy.ModelNameGPT41, 5, http.StatusBadGateway, "provider_failure")
			assertTelemetryLogsExcludeContent(subTest, observedLogs, telemetryUnsafePrompt, telemetryUnsafeProviderBody, telemetryUnsafeUpstreamID, serviceSecretValue, openAIKeyValue)
		})
	}
}

func TestIntegrationRequestTelemetryTracksProviderNeutralContinuation(testingInstance *testing.T) {
	var upstreamCalls atomic.Int64
	observedCore, observedLogs := observer.New(zapcore.InfoLevel)
	configuration := timeoutContractConfiguration(2, 3)
	configuration.Endpoints = integrationProviderEndpoints("https://meta-telemetry.invalid", proxy.ProviderNameMeta)
	router := timeoutContractRouter(
		testingInstance,
		requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
			time.Sleep(telemetryProviderDelay)
			if upstreamCalls.Add(1) == 1 {
				return completedTimeoutContractResponse(`{"choices":[{"message":{"content":"first-part-"},"finish_reason":"length"}]}`), nil
			}
			return completedTimeoutContractResponse(`{"choices":[{"message":{"content":"second-part"},"finish_reason":"stop"}]}`), nil
		}),
		configuration,
		zap.New(observedCore).Sugar(),
	)
	request := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&provider="+proxy.ProviderNameMeta+"&prompt="+telemetryUnsafePrompt, nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "first-part-second-part" {
		testingInstance.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	requestID := response.Header().Get(llmproxycontract.HeaderRequestID)
	progressEntries := observedLogs.FilterMessage(telemetryProgressEvent).All()
	if len(progressEntries) != 2 {
		testingInstance.Fatalf("progress entries=%d want=2", len(progressEntries))
	}
	firstFields := progressEntries[0].ContextMap()
	secondFields := progressEntries[1].ContextMap()
	if firstFields["request_id"] != requestID || firstFields["progress_kind"] != telemetryContinuationProgress || firstFields["completion_signal"] != "output_limit" || telemetryNumericField(firstFields, "attempt_count") != 1 || telemetryNumericField(firstFields, "current_output_bytes") != int64(len("first-part-")) || telemetryNumericField(firstFields, "accumulated_output_bytes") != int64(len("first-part-")) {
		testingInstance.Fatalf("first continuation progress=%v", firstFields)
	}
	if secondFields["request_id"] != requestID || secondFields["completion_signal"] != "complete" || telemetryNumericField(secondFields, "attempt_count") != 2 || telemetryNumericField(secondFields, "current_output_bytes") != int64(len("second-part")) || telemetryNumericField(secondFields, "accumulated_output_bytes") != int64(len("first-part-second-part")) {
		testingInstance.Fatalf("second continuation progress=%v", secondFields)
	}
	summary := telemetrySummaryForRequest(testingInstance, observedLogs, requestID)
	assertTelemetrySummaryIdentity(testingInstance, summary, "/", proxy.ProviderNameMeta, proxy.ModelNameMuseSpark11, 2, http.StatusOK, "success")
	assertTelemetrySummaryPhases(testingInstance, summary)
	if telemetryNumericField(summary, "continuation_wait_ms") < telemetryPollTolerance.Milliseconds() || telemetryNumericField(summary, "provider_poll_wait_ms") != 0 {
		testingInstance.Fatalf("continuation phase summary=%v", summary)
	}
	assertTelemetryLogsExcludeContent(testingInstance, observedLogs, telemetryUnsafePrompt, "first-part-", "second-part", "sk-meta")
}

func TestIntegrationRequestTelemetryClassifiesFailureCancellationAndBudgetExpiry(testingInstance *testing.T) {
	testCases := []struct {
		name            string
		requestContext  context.Context
		requestBudget   string
		doer            proxy.HTTPDoer
		expectedStatus  int
		expectedOutcome string
		expectedSignal  string
	}{
		{
			name: "provider failure",
			doer: requestTimeoutHTTPDoer(func(*http.Request) (*http.Response, error) {
				response := completedTimeoutContractResponse(`{"error":"` + telemetryUnsafeProviderBody + `"}`)
				response.StatusCode = http.StatusBadRequest
				return response, nil
			}),
			expectedStatus:  http.StatusBadGateway,
			expectedOutcome: "provider_failure",
			expectedSignal:  "failure",
		},
		{
			name: "provider cancellation signal",
			doer: requestTimeoutHTTPDoer(func(*http.Request) (*http.Response, error) {
				return completedTimeoutContractResponse(`{"id":"` + telemetryUnsafeUpstreamID + `","status":"cancelled"}`), nil
			}),
			expectedStatus:  http.StatusBadGateway,
			expectedOutcome: "provider_failure",
			expectedSignal:  "canceled",
		},
		{
			name:           "caller cancellation",
			requestContext: canceledTelemetryContext(),
			doer: requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
				<-request.Context().Done()
				return nil, request.Context().Err()
			}),
			expectedStatus:  499,
			expectedOutcome: "caller_cancelled",
			expectedSignal:  "canceled",
		},
		{
			name:          "proxy budget expiry",
			requestBudget: "1",
			doer: requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
				<-request.Context().Done()
				return nil, request.Context().Err()
			}),
			expectedStatus:  http.StatusGatewayTimeout,
			expectedOutcome: "proxy_timeout",
			expectedSignal:  "canceled",
		},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			observedCore, observedLogs := observer.New(zapcore.InfoLevel)
			router := timeoutContractRouter(subTest, testCase.doer, timeoutContractConfiguration(2, 3), zap.New(observedCore).Sugar())
			request := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt="+telemetryUnsafePrompt, nil)
			if testCase.requestContext != nil {
				request = request.WithContext(testCase.requestContext)
			}
			if testCase.requestBudget != "" {
				request.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, testCase.requestBudget)
			}
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != testCase.expectedStatus {
				subTest.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if testCase.requestContext != nil {
				if summaries := observedLogs.FilterMessage(telemetrySummaryEvent).All(); len(summaries) != 0 {
					subTest.Fatalf("pre-authentication cancellation summaries=%v", summaries)
				}
				if progress := observedLogs.FilterMessage(telemetryProgressEvent).All(); len(progress) != 0 {
					subTest.Fatalf("pre-authentication cancellation progress=%v", progress)
				}
				return
			}
			requestID := response.Header().Get(llmproxycontract.HeaderRequestID)
			summary := telemetrySummaryForRequest(subTest, observedLogs, requestID)
			expectedBudget := int64(2)
			if testCase.requestBudget != "" {
				expectedBudget = 1
			}
			assertTelemetrySummaryIdentity(subTest, summary, "/", proxy.ProviderNameOpenAI, proxy.ModelNameGPT41, expectedBudget, int64(testCase.expectedStatus), testCase.expectedOutcome)
			assertTelemetrySummaryPhases(subTest, summary)
			createProgress := telemetryProgressForKind(subTest, observedLogs, requestID, telemetryOpenAICreateProgress)
			if createProgress["completion_signal"] != testCase.expectedSignal {
				subTest.Fatalf("OpenAI create completion signal=%v want=%q", createProgress, testCase.expectedSignal)
			}
			assertTelemetryLogsExcludeContent(subTest, observedLogs, telemetryUnsafePrompt, telemetryUnsafeProviderBody, serviceSecretValue, openAIKeyValue)
		})
	}
}

func TestIntegrationRequestTelemetryClassifiesCanceledOpenAIPoll(testingInstance *testing.T) {
	requestContext, cancelRequest := context.WithCancel(context.Background())
	testingInstance.Cleanup(cancelRequest)
	var upstreamCalls atomic.Int64
	observedCore, observedLogs := observer.New(zapcore.InfoLevel)
	router := timeoutContractRouter(
		testingInstance,
		requestTimeoutHTTPDoer(func(request *http.Request) (*http.Response, error) {
			if upstreamCalls.Add(1) == 1 {
				return completedTimeoutContractResponse(`{"id":"` + telemetryUnsafeUpstreamID + `","status":"queued"}`), nil
			}
			cancelRequest()
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
		timeoutContractConfiguration(2, 3),
		zap.New(observedCore).Sugar(),
	)
	request := httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt="+telemetryUnsafePrompt, nil).WithContext(requestContext)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != 499 {
		testingInstance.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	requestID := response.Header().Get(llmproxycontract.HeaderRequestID)
	pollProgress := telemetryProgressForKind(testingInstance, observedLogs, requestID, telemetryOpenAIPollProgress)
	if pollProgress["completion_signal"] != "canceled" {
		testingInstance.Fatalf("OpenAI poll completion signal=%v", pollProgress)
	}
	continuationProgress := telemetryProgressForKind(testingInstance, observedLogs, requestID, telemetryContinuationProgress)
	if continuationProgress["completion_signal"] != "canceled" {
		testingInstance.Fatalf("continuation completion signal=%v", continuationProgress)
	}
	summary := telemetrySummaryForRequest(testingInstance, observedLogs, requestID)
	assertTelemetrySummaryIdentity(testingInstance, summary, "/", proxy.ProviderNameOpenAI, proxy.ModelNameGPT41, 2, 499, "caller_cancelled")
	assertTelemetrySummaryPhases(testingInstance, summary)
	assertTelemetryLogsExcludeContent(testingInstance, observedLogs, telemetryUnsafePrompt, telemetryUnsafeUpstreamID, serviceSecretValue, openAIKeyValue)
}

func TestIntegrationRequestTelemetrySeparatesAdmissionAndRateLimitWait(testingInstance *testing.T) {
	testingInstance.Run("queue wait", func(subTest *testing.T) {
		firstUpstreamStarted := make(chan struct{})
		releaseFirstUpstream := make(chan struct{})
		var callCount atomic.Int64
		var releaseOnce sync.Once
		observedCore, observedLogs := observer.New(zapcore.InfoLevel)
		router := timeoutContractRouter(
			subTest,
			requestTimeoutHTTPDoer(func(*http.Request) (*http.Response, error) {
				if callCount.Add(1) == 1 {
					close(firstUpstreamStarted)
					<-releaseFirstUpstream
				}
				return completedTimeoutContractResponse(`{"id":"queue","status":"completed","output_text":"ok"}`), nil
			}),
			timeoutContractConfiguration(2, 3),
			zap.New(observedCore).Sugar(),
		)
		subTest.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirstUpstream) }) })
		firstResult := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			firstResponse := httptest.NewRecorder()
			router.ServeHTTP(firstResponse, httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=first", nil))
			firstResult <- firstResponse
		}()
		select {
		case <-firstUpstreamStarted:
		case <-time.After(time.Second):
			subTest.Fatal("first upstream request did not start")
		}
		secondResult := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			secondResponse := httptest.NewRecorder()
			router.ServeHTTP(secondResponse, httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=second", nil))
			secondResult <- secondResponse
		}()
		time.Sleep(telemetryQueueHold)
		releaseOnce.Do(func() { close(releaseFirstUpstream) })
		firstResponse := <-firstResult
		secondResponse := <-secondResult
		if firstResponse.Code != http.StatusOK || secondResponse.Code != http.StatusOK {
			subTest.Fatalf("statuses=%d,%d", firstResponse.Code, secondResponse.Code)
		}
		secondSummary := telemetrySummaryForRequest(subTest, observedLogs, secondResponse.Header().Get(llmproxycontract.HeaderRequestID))
		if telemetryNumericField(secondSummary, "upstream_admission_ms") < telemetryQueueHold.Milliseconds()-20 || telemetryNumericField(secondSummary, "upstream_rate_limit_wait_ms") != 0 {
			subTest.Fatalf("queue summary=%v", secondSummary)
		}
	})

	testingInstance.Run("rate-limit wait", func(subTest *testing.T) {
		observedCore, observedLogs := observer.New(zapcore.InfoLevel)
		configuration := timeoutContractConfiguration(2, 3)
		configuration.Endpoints = integrationProviderEndpoints("https://telemetry-rate.invalid/v1", proxy.ProviderNameOpenAI)
		configuration.UpstreamRateLimits = []proxy.UpstreamRateLimitConfiguration{{
			Origin:      "https://telemetry-rate.invalid",
			MaxRequests: 1,
			Interval:    telemetryRateLimitInterval.String(),
		}}
		router := timeoutContractRouter(
			subTest,
			requestTimeoutHTTPDoer(func(*http.Request) (*http.Response, error) {
				return completedTimeoutContractResponse(`{"id":"rate","status":"completed","output_text":"ok"}`), nil
			}),
			configuration,
			zap.New(observedCore).Sugar(),
		)
		firstResponse := httptest.NewRecorder()
		router.ServeHTTP(firstResponse, httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=first", nil))
		secondResponse := httptest.NewRecorder()
		router.ServeHTTP(secondResponse, httptest.NewRequest(http.MethodGet, "/?key="+serviceSecretValue+"&prompt=second", nil))
		if firstResponse.Code != http.StatusOK || secondResponse.Code != http.StatusOK {
			subTest.Fatalf("statuses=%d,%d", firstResponse.Code, secondResponse.Code)
		}
		secondSummary := telemetrySummaryForRequest(subTest, observedLogs, secondResponse.Header().Get(llmproxycontract.HeaderRequestID))
		if telemetryNumericField(secondSummary, "upstream_rate_limit_wait_ms") < telemetryRateLimitTolerance.Milliseconds() || telemetryNumericField(secondSummary, "upstream_admission_ms") >= telemetryNumericField(secondSummary, "upstream_rate_limit_wait_ms") {
			subTest.Fatalf("rate-limit summary=%v", secondSummary)
		}
	})
}

func canceledTelemetryContext() context.Context {
	requestContext, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()
	return requestContext
}

func telemetrySummaryForRequest(testingInstance *testing.T, observedLogs *observer.ObservedLogs, requestID string) map[string]any {
	testingInstance.Helper()
	for _, entry := range observedLogs.FilterMessage(telemetrySummaryEvent).All() {
		fields := entry.ContextMap()
		if fields["request_id"] == requestID {
			return fields
		}
	}
	testingInstance.Fatalf("missing terminal telemetry summary for request_id=%q: %v", requestID, observedLogs.All())
	return nil
}

func telemetryProgressForKind(testingInstance *testing.T, observedLogs *observer.ObservedLogs, requestID string, progressKind string) map[string]any {
	testingInstance.Helper()
	for _, entry := range observedLogs.FilterMessage(telemetryProgressEvent).All() {
		fields := entry.ContextMap()
		if fields["request_id"] == requestID && fields["progress_kind"] == progressKind {
			return fields
		}
	}
	testingInstance.Fatalf("missing %s telemetry progress for request_id=%q: %v", progressKind, requestID, observedLogs.All())
	return nil
}

func assertTelemetrySummaryIdentity(testingInstance *testing.T, fields map[string]any, endpoint string, provider string, model string, budget any, status any, outcome string) {
	testingInstance.Helper()
	if fields["endpoint"] != endpoint || fields["provider"] != provider || fields["model"] != model || telemetryNumericField(fields, "request_timeout_seconds") != telemetryIntegerValue(budget) || telemetryNumericField(fields, "status") != telemetryIntegerValue(status) || fields["outcome"] != outcome {
		testingInstance.Fatalf("terminal telemetry identity=%v", fields)
	}
}

func assertTelemetrySummaryPhases(testingInstance *testing.T, fields map[string]any) {
	testingInstance.Helper()
	for _, fieldName := range []string{
		"total_latency_ms",
		"authentication_ms",
		"upstream_admission_ms",
		"upstream_rate_limit_wait_ms",
		"provider_http_ms",
		"provider_poll_wait_ms",
		"continuation_wait_ms",
		"response_formatting_ms",
		"managed_usage_enqueue_ms",
	} {
		if _, present := fields[fieldName]; !present || telemetryNumericField(fields, fieldName) < 0 {
			testingInstance.Fatalf("terminal telemetry missing nonnegative %s: %v", fieldName, fields)
		}
	}
}

func telemetryNumericField(fields map[string]any, fieldName string) int64 {
	value, present := fields[fieldName]
	if !present {
		return 0
	}
	return telemetryIntegerValue(value)
}

func telemetryIntegerValue(value any) int64 {
	switch typedValue := value.(type) {
	case int:
		return int64(typedValue)
	case int64:
		return typedValue
	default:
		panic(fmt.Sprintf("telemetry field has non-integer type %T", value))
	}
}

func assertTelemetryLogsExcludeContent(testingInstance *testing.T, observedLogs *observer.ObservedLogs, forbiddenValues ...string) {
	testingInstance.Helper()
	for _, entry := range observedLogs.All() {
		if entry.Message != telemetrySummaryEvent && entry.Message != telemetryProgressEvent {
			continue
		}
		loggedContent := entry.Message + fmt.Sprint(entry.ContextMap())
		for _, forbiddenValue := range forbiddenValues {
			if strings.Contains(loggedContent, forbiddenValue) {
				testingInstance.Fatalf("telemetry exposed forbidden value in %s", loggedContent)
			}
		}
		for _, forbiddenField := range []string{"prompt", "messages", "response", "response_body", "provider_body", "credential", "cookie", "authorization", "id"} {
			if _, present := entry.ContextMap()[forbiddenField]; present {
				testingInstance.Fatalf("telemetry exposed forbidden field %q in %v", forbiddenField, entry.ContextMap())
			}
		}
	}
}
