package proxy

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

type requestTelemetryContextKey struct{}

type requestTelemetryPhase uint8

const (
	requestTelemetryPhaseAuthentication requestTelemetryPhase = iota
	requestTelemetryPhaseUpstreamAdmission
	requestTelemetryPhaseUpstreamRateLimit
	requestTelemetryPhaseProviderHTTP
	requestTelemetryPhaseProviderPollWait
	requestTelemetryPhaseContinuationWait
	requestTelemetryPhaseResponseFormatting
	requestTelemetryPhaseManagedUsageEnqueue
)

type requestTelemetry struct {
	mutex sync.Mutex

	requestID string
	endpoint  string
	startedAt time.Time

	route                 managedUsageRoute
	requestTimeoutSeconds int
	routeBound            bool

	authentication      time.Duration
	upstreamAdmission   time.Duration
	upstreamRateLimit   time.Duration
	providerHTTP        time.Duration
	providerPollWait    time.Duration
	continuationWait    time.Duration
	responseFormatting  time.Duration
	managedUsageEnqueue time.Duration

	openAICreateCount        int
	openAIPollCount          int
	geminiCreateCount        int
	geminiPollCount          int
	geminiCancelCount        int
	geminiDeleteCount        int
	continuationAttemptCount int
	completedOutputBytes     int
}

type requestTelemetrySnapshot struct {
	requestID             string
	endpoint              string
	provider              string
	model                 string
	requestTimeoutSeconds int
	totalLatency          time.Duration
	authentication        time.Duration
	upstreamAdmission     time.Duration
	upstreamRateLimit     time.Duration
	providerHTTP          time.Duration
	providerPollWait      time.Duration
	continuationWait      time.Duration
	responseFormatting    time.Duration
	managedUsageEnqueue   time.Duration
}

func newRequestTelemetry(requestID string, endpoint string) *requestTelemetry {
	return &requestTelemetry{
		requestID: requestID,
		endpoint:  endpoint,
		startedAt: time.Now(),
	}
}

func requestContextWithTelemetry(requestContext context.Context, telemetry *requestTelemetry) context.Context {
	return context.WithValue(requestContext, requestTelemetryContextKey{}, telemetry)
}

func requestTelemetryFromContext(requestContext context.Context) *requestTelemetry {
	telemetry, _ := requestContext.Value(requestTelemetryContextKey{}).(*requestTelemetry)
	return telemetry
}

func (telemetry *requestTelemetry) bindRoute(route managedUsageRoute, requestTimeoutSeconds int) {
	telemetry.mutex.Lock()
	defer telemetry.mutex.Unlock()
	telemetry.route = route
	telemetry.requestTimeoutSeconds = requestTimeoutSeconds
	telemetry.routeBound = true
}

func (telemetry *requestTelemetry) usageRoute() *managedUsageRoute {
	telemetry.mutex.Lock()
	defer telemetry.mutex.Unlock()
	if !telemetry.routeBound {
		return nil
	}
	route := telemetry.route
	return &route
}

func (telemetry *requestTelemetry) addPhase(phase requestTelemetryPhase, elapsed time.Duration) {
	telemetry.mutex.Lock()
	defer telemetry.mutex.Unlock()
	switch phase {
	case requestTelemetryPhaseAuthentication:
		telemetry.authentication += elapsed
	case requestTelemetryPhaseUpstreamAdmission:
		telemetry.upstreamAdmission += elapsed
	case requestTelemetryPhaseUpstreamRateLimit:
		telemetry.upstreamRateLimit += elapsed
	case requestTelemetryPhaseProviderHTTP:
		telemetry.providerHTTP += elapsed
	case requestTelemetryPhaseProviderPollWait:
		telemetry.providerPollWait += elapsed
	case requestTelemetryPhaseContinuationWait:
		telemetry.continuationWait += elapsed
	case requestTelemetryPhaseResponseFormatting:
		telemetry.responseFormatting += elapsed
	case requestTelemetryPhaseManagedUsageEnqueue:
		telemetry.managedUsageEnqueue += elapsed
	}
}

func addRequestTelemetryPhase(requestContext context.Context, phase requestTelemetryPhase, startedAt time.Time) {
	if telemetry := requestTelemetryFromContext(requestContext); telemetry != nil {
		telemetry.addPhase(phase, time.Since(startedAt))
	}
}

func waitForRequestTelemetryPhase(requestContext context.Context, waitDuration time.Duration, phase requestTelemetryPhase) error {
	waitStartedAt := time.Now()
	waitTimer := time.NewTimer(waitDuration)
	defer waitTimer.Stop()
	select {
	case <-waitTimer.C:
		addRequestTelemetryPhase(requestContext, phase, waitStartedAt)
		return nil
	case <-requestContext.Done():
		addRequestTelemetryPhase(requestContext, phase, waitStartedAt)
		return requestContext.Err()
	}
}

func (telemetry *requestTelemetry) recordOpenAIProgress(structuredLogger *zap.SugaredLogger, progressKind string, state string, completionSignal string, currentOutputBytes int) {
	telemetry.mutex.Lock()
	if progressKind == telemetryProgressKindOpenAICreate {
		telemetry.openAICreateCount++
	} else {
		telemetry.openAIPollCount++
	}
	createCount := telemetry.openAICreateCount
	pollCount := telemetry.openAIPollCount
	requestID := telemetry.requestID
	provider := telemetry.route.providerIdentifier.string()
	model := telemetry.route.modelIdentifier.string()
	elapsedMilliseconds := time.Since(telemetry.startedAt).Milliseconds()
	accumulatedOutputBytes := telemetry.completedOutputBytes + currentOutputBytes
	telemetry.mutex.Unlock()

	fields := []any{
		logFieldRequestID, requestID,
		logFieldProvider, provider,
		logFieldModel, model,
		logFieldProgressKind, progressKind,
		logFieldProviderState, state,
		logFieldCompletionSignal, completionSignal,
		logFieldElapsedMilliseconds, elapsedMilliseconds,
		logFieldCurrentOutputBytes, currentOutputBytes,
		logFieldAccumulatedOutputBytes, accumulatedOutputBytes,
	}
	if progressKind == telemetryProgressKindOpenAICreate {
		fields = append(fields, logFieldAttemptCount, createCount)
	} else {
		fields = append(fields, logFieldPollCount, pollCount)
	}
	structuredLogger.Infow(logEventProviderProgress, fields...)
}

func recordOpenAIProgress(requestContext context.Context, structuredLogger *zap.SugaredLogger, progressKind string, snapshot openAIResponseSnapshot, progressError error) {
	recordOpenAIProgressWithRetryDecision(requestContext, structuredLogger, progressKind, snapshot, progressError, pollableResourceDoNotRetry)
}

func recordOpenAIProgressWithRetryDecision(requestContext context.Context, structuredLogger *zap.SugaredLogger, progressKind string, snapshot openAIResponseSnapshot, progressError error, retryDecision pollableResourceRetryDecision) {
	telemetry := requestTelemetryFromContext(requestContext)
	telemetry.recordOpenAIProgress(
		structuredLogger,
		progressKind,
		normalizedOpenAIState(snapshot.status),
		openAICompletionSignal(snapshot, progressError, retryDecision),
		len([]byte(snapshot.text)),
	)
}

func (telemetry *requestTelemetry) recordGeminiProgress(structuredLogger *zap.SugaredLogger, progressKind string, state string, completionSignal string, providerErrorCodes []string) {
	telemetry.mutex.Lock()
	progressCount := 0
	switch progressKind {
	case telemetryProgressKindGeminiCreate:
		telemetry.geminiCreateCount++
		progressCount = telemetry.geminiCreateCount
	case telemetryProgressKindGeminiPoll:
		telemetry.geminiPollCount++
		progressCount = telemetry.geminiPollCount
	case telemetryProgressKindGeminiCancel:
		telemetry.geminiCancelCount++
		progressCount = telemetry.geminiCancelCount
	case telemetryProgressKindGeminiDelete:
		telemetry.geminiDeleteCount++
		progressCount = telemetry.geminiDeleteCount
	}
	requestID := telemetry.requestID
	provider := telemetry.route.providerIdentifier.string()
	model := telemetry.route.modelIdentifier.string()
	elapsedMilliseconds := time.Since(telemetry.startedAt).Milliseconds()
	telemetry.mutex.Unlock()

	fields := []any{
		logFieldRequestID, requestID,
		logFieldProvider, provider,
		logFieldModel, model,
		logFieldProgressKind, progressKind,
		logFieldProviderState, state,
		logFieldCompletionSignal, completionSignal,
		logFieldElapsedMilliseconds, elapsedMilliseconds,
	}
	if progressKind == telemetryProgressKindGeminiPoll {
		fields = append(fields, logFieldPollCount, progressCount)
	} else {
		fields = append(fields, logFieldAttemptCount, progressCount)
	}
	if len(providerErrorCodes) > 0 {
		fields = append(fields, logFieldProviderErrorCodes, providerErrorCodes)
	}
	structuredLogger.Infow(logEventProviderProgress, fields...)
}

func recordGeminiProgress(requestContext context.Context, structuredLogger *zap.SugaredLogger, progressKind string, snapshot geminiInteractionSnapshot, progressError error, retryDecision pollableResourceRetryDecision) {
	telemetry := requestTelemetryFromContext(requestContext)
	if telemetry == nil || structuredLogger == nil {
		return
	}
	providerErrorCodes := snapshot.providerFailureCodes()
	telemetry.recordGeminiProgress(
		structuredLogger,
		progressKind,
		normalizedGeminiState(snapshot.status),
		geminiCompletionSignal(snapshot, progressError, retryDecision),
		providerErrorCodes,
	)
}

func normalizedGeminiState(state string) string {
	switch state {
	case statusQueued, statusInProgress, statusCompleted, statusCancelled, statusFailed, statusIncomplete, geminiInteractionStatusRequiresAction, telemetryProviderStateDeleted:
		return state
	default:
		return telemetryProviderStateUnknown
	}
}

func geminiCompletionSignal(snapshot geminiInteractionSnapshot, progressError error, retryDecision pollableResourceRetryDecision) string {
	if errors.Is(progressError, context.Canceled) || errors.Is(progressError, context.DeadlineExceeded) {
		return telemetryCompletionCanceled
	}
	if retryDecision == pollableResourceRetryVisibility {
		return telemetryCompletionPending
	}
	if progressError != nil {
		return telemetryCompletionFailure
	}
	switch snapshot.status {
	case statusQueued, statusInProgress:
		return telemetryCompletionPending
	case statusCompleted, telemetryProviderStateDeleted:
		return telemetryCompletionComplete
	case statusIncomplete:
		return telemetryCompletionOutputLimit
	case statusCancelled:
		return telemetryCompletionCanceled
	default:
		return telemetryCompletionFailure
	}
}

func normalizedOpenAIState(state string) string {
	switch state {
	case statusQueued, statusInProgress, statusCompleted, statusCancelled, statusFailed, statusIncomplete:
		return state
	default:
		return telemetryProviderStateUnknown
	}
}

func openAICompletionSignal(snapshot openAIResponseSnapshot, progressError error, retryDecision pollableResourceRetryDecision) string {
	if errors.Is(progressError, context.Canceled) || errors.Is(progressError, context.DeadlineExceeded) {
		return telemetryCompletionCanceled
	}
	if retryDecision == pollableResourceRetryVisibility {
		return telemetryCompletionPending
	}
	if progressError != nil {
		return telemetryCompletionFailure
	}
	switch snapshot.status {
	case statusQueued, statusInProgress:
		return telemetryCompletionPending
	case statusCompleted:
		return telemetryCompletionComplete
	case statusIncomplete:
		if snapshot.incompleteReason == "max_output_tokens" {
			return telemetryCompletionOutputLimit
		}
		return telemetryCompletionFailure
	case statusCancelled:
		return telemetryCompletionCanceled
	default:
		return telemetryCompletionFailure
	}
}

func recordContinuationProgress(requestContext context.Context, structuredLogger *zap.SugaredLogger, generation textGenerationResult, accumulatedOutputBytes int, generationError error) {
	telemetry := requestTelemetryFromContext(requestContext)
	telemetry.mutex.Lock()
	telemetry.continuationAttemptCount++
	attemptCount := telemetry.continuationAttemptCount
	telemetry.completedOutputBytes = accumulatedOutputBytes
	requestID := telemetry.requestID
	provider := telemetry.route.providerIdentifier.string()
	model := telemetry.route.modelIdentifier.string()
	elapsedMilliseconds := time.Since(telemetry.startedAt).Milliseconds()
	telemetry.mutex.Unlock()

	completionSignal := telemetryCompletionComplete
	switch {
	case errors.Is(generationError, errProviderOutputLimitReached):
		completionSignal = telemetryCompletionOutputLimit
	case errors.Is(generationError, context.Canceled), errors.Is(generationError, context.DeadlineExceeded):
		completionSignal = telemetryCompletionCanceled
	case generationError != nil:
		completionSignal = telemetryCompletionFailure
	}
	structuredLogger.Infow(
		logEventProviderProgress,
		logFieldRequestID, requestID,
		logFieldProvider, provider,
		logFieldModel, model,
		logFieldProgressKind, telemetryProgressKindContinuationAttempt,
		logFieldAttemptCount, attemptCount,
		logFieldCompletionSignal, completionSignal,
		logFieldElapsedMilliseconds, elapsedMilliseconds,
		logFieldCurrentOutputBytes, len([]byte(generation.text)),
		logFieldAccumulatedOutputBytes, accumulatedOutputBytes,
	)
}

func (telemetry *requestTelemetry) snapshot() (requestTelemetrySnapshot, bool) {
	telemetry.mutex.Lock()
	defer telemetry.mutex.Unlock()
	if !telemetry.routeBound {
		return requestTelemetrySnapshot{}, false
	}
	return requestTelemetrySnapshot{
		requestID:             telemetry.requestID,
		endpoint:              telemetry.endpoint,
		provider:              telemetry.route.providerIdentifier.string(),
		model:                 telemetry.route.modelIdentifier.string(),
		requestTimeoutSeconds: telemetry.requestTimeoutSeconds,
		totalLatency:          time.Since(telemetry.startedAt),
		authentication:        telemetry.authentication,
		upstreamAdmission:     telemetry.upstreamAdmission,
		upstreamRateLimit:     telemetry.upstreamRateLimit,
		providerHTTP:          telemetry.providerHTTP,
		providerPollWait:      telemetry.providerPollWait,
		continuationWait:      telemetry.continuationWait,
		responseFormatting:    telemetry.responseFormatting,
		managedUsageEnqueue:   telemetry.managedUsageEnqueue,
	}, true
}

func emitRequestTelemetrySummary(requestContext context.Context, structuredLogger *zap.SugaredLogger, statusCode int, outcome string) {
	telemetry := requestTelemetryFromContext(requestContext)
	snapshot, routed := telemetry.snapshot()
	if !routed {
		return
	}
	structuredLogger.Infow(
		logEventRequestPhaseSummary,
		logFieldRequestID, snapshot.requestID,
		logFieldEndpoint, snapshot.endpoint,
		logFieldProvider, snapshot.provider,
		logFieldModel, snapshot.model,
		logFieldRequestTimeoutSeconds, snapshot.requestTimeoutSeconds,
		logFieldStatus, statusCode,
		logFieldOutcome, outcome,
		logFieldTotalLatencyMilliseconds, snapshot.totalLatency.Milliseconds(),
		logFieldAuthenticationMilliseconds, snapshot.authentication.Milliseconds(),
		logFieldUpstreamAdmissionMilliseconds, snapshot.upstreamAdmission.Milliseconds(),
		logFieldUpstreamRateLimitMilliseconds, snapshot.upstreamRateLimit.Milliseconds(),
		logFieldProviderHTTPMilliseconds, snapshot.providerHTTP.Milliseconds(),
		logFieldProviderPollWaitMilliseconds, snapshot.providerPollWait.Milliseconds(),
		logFieldContinuationWaitMilliseconds, snapshot.continuationWait.Milliseconds(),
		logFieldResponseFormattingMilliseconds, snapshot.responseFormatting.Milliseconds(),
		logFieldManagedUsageEnqueueMilliseconds, snapshot.managedUsageEnqueue.Milliseconds(),
	)
}
