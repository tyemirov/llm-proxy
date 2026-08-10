package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

const (
	contextKeyRequestTimeoutState = "request_timeout_state"
	logEventUpstreamRequestEnded  = "upstream request ended"
	requestOutcomeValidation      = "validation_failure"
	requestOutcomeSuccess         = "success"
	requestOutcomeProxyTimeout    = "proxy_timeout"
	requestOutcomeProxyOverload   = "proxy_overload"
	requestOutcomeProviderFailure = "provider_failure"
	requestOutcomeCallerCancelled = "caller_cancelled"
	statusClientClosedRequest     = 499
)

var errRequestTimeoutBudgetExpired = errors.New("request timeout budget expired")

type requestTimeoutPolicy struct {
	defaultBudget requestTimeoutBudget
	maximum       int
}

type requestTimeoutBudget struct {
	seconds  int
	duration time.Duration
}

type requestTimeoutState struct {
	budget              requestTimeoutBudget
	outcome             string
	managedUsageOutcome managedUsageOutcomeCode
}

func newRequestTimeoutPolicy(defaultSeconds int, maximumSeconds int) (requestTimeoutPolicy, error) {
	if defaultSeconds <= 0 {
		return requestTimeoutPolicy{}, fmt.Errorf("invalid request timeout configuration: server.request_timeout_seconds must be positive")
	}
	if maximumSeconds <= 0 {
		return requestTimeoutPolicy{}, fmt.Errorf("invalid request timeout configuration: server.max_request_timeout_seconds must be positive")
	}
	if defaultSeconds > maximumSeconds {
		return requestTimeoutPolicy{}, fmt.Errorf("invalid request timeout configuration: server.request_timeout_seconds exceeds server.max_request_timeout_seconds")
	}
	if int64(maximumSeconds) > int64(^uint64(0)>>1)/int64(time.Second) {
		return requestTimeoutPolicy{}, fmt.Errorf("invalid request timeout configuration: server.max_request_timeout_seconds exceeds supported duration")
	}
	return requestTimeoutPolicy{
		defaultBudget: newRequestTimeoutBudget(defaultSeconds),
		maximum:       maximumSeconds,
	}, nil
}

func newRequestTimeoutBudget(seconds int) requestTimeoutBudget {
	return requestTimeoutBudget{
		seconds:  seconds,
		duration: time.Duration(seconds) * time.Second,
	}
}

func (policy requestTimeoutPolicy) resolve(headerValues []string) (requestTimeoutBudget, bool) {
	if len(headerValues) == 0 {
		return policy.defaultBudget, true
	}
	if len(headerValues) != 1 || headerValues[0] == "" {
		return requestTimeoutBudget{}, false
	}
	for _, character := range headerValues[0] {
		if character < '0' || character > '9' {
			return requestTimeoutBudget{}, false
		}
	}
	requestedSeconds, parseError := strconv.Atoi(headerValues[0])
	if parseError != nil || requestedSeconds <= 0 || requestedSeconds > policy.maximum {
		return requestTimeoutBudget{}, false
	}
	return newRequestTimeoutBudget(requestedSeconds), true
}

func requestTimeoutHandler(policy requestTimeoutPolicy, structuredLogger *zap.SugaredLogger, handler gin.HandlerFunc) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		budget, valid := policy.resolve(ginContext.Request.Header.Values(llmproxycontract.HeaderRequestTimeoutSeconds))
		if !valid {
			ginContext.Data(
				http.StatusBadRequest,
				mimeApplicationJSON,
				[]byte(fmt.Sprintf(
					`{"error":{"code":"%s","max_request_timeout_seconds":%d}}`,
					llmproxycontract.ErrorCodeInvalidRequestTimeout,
					policy.maximum,
				)),
			)
			return
		}

		ginContext.Header(llmproxycontract.HeaderRequestTimeoutSeconds, strconv.Itoa(budget.seconds))
		callerContext := ginContext.Request.Context()
		requestContext, cancelRequest := context.WithTimeoutCause(
			callerContext,
			budget.duration,
			errRequestTimeoutBudgetExpired,
		)
		requestBody := ginContext.Request.Body
		stopBodyClose := context.AfterFunc(requestContext, func() {
			if requestBody != nil {
				_ = requestBody.Close()
			}
		})
		state := &requestTimeoutState{
			budget:              budget,
			outcome:             requestOutcomeValidation,
			managedUsageOutcome: managedUsageOutcomeInvalidRequest,
		}
		ginContext.Set(contextKeyRequestTimeoutState, state)
		ginContext.Request = ginContext.Request.WithContext(requestContext)
		if requestBody != nil {
			ginContext.Request.Body = contextReadCloser{
				Reader: contextReader{contextValue: requestContext, reader: requestBody},
				Closer: requestBody,
			}
		}

		handler(ginContext)

		_ = stopBodyClose()
		cancelRequest()
		if structuredLogger != nil {
			emitRequestTelemetrySummary(ginContext.Request.Context(), structuredLogger, ginContext.Writer.Status(), state.outcome)
			requestTenant := authenticatedTenantFromContext(ginContext)
			structuredLogger.Infow(
				logEventUpstreamRequestEnded,
				logFieldMethod, ginContext.Request.Method,
				logFieldEndpoint, requestLogPath(ginContext.Request.URL),
				logFieldTenantID, requestTenant.identifier.string(),
				logFieldRequestTimeoutSeconds, budget.seconds,
				logFieldOutcome, state.outcome,
				logFieldStatus, ginContext.Writer.Status(),
			)
		}
	}
}

func markRequestOutcome(ginContext *gin.Context, outcome string, managedOutcome managedUsageOutcomeCode) {
	state := requestTimeoutStateFromContext(ginContext)
	state.outcome = outcome
	state.managedUsageOutcome = managedOutcome
}

func markManagedUsageOutcome(ginContext *gin.Context, managedOutcome managedUsageOutcomeCode) {
	requestTimeoutStateFromContext(ginContext).managedUsageOutcome = managedOutcome
}

func requestFailureOutcome(requestError error) string {
	if errors.Is(requestError, errQueueFull) {
		return requestOutcomeProxyOverload
	}
	return requestOutcomeProviderFailure
}

func managedRequestFailureOutcome(requestError error) managedUsageOutcomeCode {
	switch {
	case errors.Is(requestError, ErrProviderRateLimited):
		return managedUsageOutcomeRateLimited
	case errors.Is(requestError, ErrProviderNotConfigured), errors.Is(requestError, errQueueFull):
		return managedUsageOutcomeServiceUnavailable
	case errors.Is(requestError, context.DeadlineExceeded), errors.Is(requestError, context.Canceled):
		return managedUsageOutcomeRequestTimeout
	default:
		return managedUsageOutcomeUpstreamError
	}
}

func requestTimeoutStateFromContext(ginContext *gin.Context) *requestTimeoutState {
	return ginContext.MustGet(contextKeyRequestTimeoutState).(*requestTimeoutState)
}

func requestContextEnded(ginContext *gin.Context) bool {
	state := requestTimeoutStateFromContext(ginContext)
	requestCause := context.Cause(ginContext.Request.Context())
	if requestCause == nil {
		return false
	}
	if !errors.Is(requestCause, errRequestTimeoutBudgetExpired) {
		state.outcome = requestOutcomeCallerCancelled
		state.managedUsageOutcome = managedUsageOutcomeRequestTimeout
		formattingStartedAt := time.Now()
		ginContext.Status(statusClientClosedRequest)
		addRequestTelemetryPhase(ginContext.Request.Context(), requestTelemetryPhaseResponseFormatting, formattingStartedAt)
		return true
	}
	state.outcome = requestOutcomeProxyTimeout
	state.managedUsageOutcome = managedUsageOutcomeRequestTimeout
	formattingStartedAt := time.Now()
	ginContext.Data(
		http.StatusGatewayTimeout,
		mimeApplicationJSON,
		[]byte(fmt.Sprintf(
			`{"error":{"code":"%s","request_timeout_seconds":%d}}`,
			llmproxycontract.ErrorCodeRequestTimeout,
			state.budget.seconds,
		)),
	)
	addRequestTelemetryPhase(ginContext.Request.Context(), requestTelemetryPhaseResponseFormatting, formattingStartedAt)
	return true
}

type contextReader struct {
	contextValue context.Context
	reader       io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	readBytes, readError := reader.reader.Read(buffer)
	if contextError := reader.contextValue.Err(); contextError != nil {
		return readBytes, contextError
	}
	return readBytes, readError
}

type contextReadCloser struct {
	io.Reader
	io.Closer
}
