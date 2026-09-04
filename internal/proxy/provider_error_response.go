package proxy

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

type providerErrorEnvelope struct {
	Error providerErrorDetail `json:"error"`
}

type providerErrorDetail struct {
	Code           string  `json:"code"`
	Provider       string  `json:"provider"`
	UpstreamStatus *int    `json:"upstream_status"`
	Retryable      bool    `json:"retryable"`
	RequestID      string  `json:"request_id"`
	RetryAfter     *string `json:"retry_after"`
}

func writeProviderRequestErrorResponse(ginContext *gin.Context, providerIdentifier string, requestError error, structuredLogger *zap.SugaredLogger) {

	if errors.Is(requestError, errQueueFull) {
		writeClientError(ginContext, statusCodeForError(requestError), "capacity_exceeded", requestError.Error())
		return
	}
	writeProviderErrorResponse(ginContext, providerIdentifier, requestError, structuredLogger)
}

func writeProviderErrorResponse(ginContext *gin.Context, providerIdentifier string, requestError error, structuredLogger *zap.SugaredLogger) {
	errorCode := llmproxycontract.ErrorCodeProviderError
	if errors.Is(requestError, ErrProviderRateLimited) {
		errorCode = llmproxycontract.ErrorCodeProviderRateLimited
	} else if errors.Is(requestError, ErrProviderMediaLimit) {
		errorCode = llmproxycontract.ErrorCodeProviderMediaLimitExceeded
	}

	upstreamStatus, retryAfterValue, retryable, hasUpstreamStatus, providerErrorCodes := providerFailureMetadata(requestError)
	var upstreamStatusValue *int
	if hasUpstreamStatus {
		upstreamStatusValue = &upstreamStatus
	}
	var retryAfter *string
	if retryAfterValue != "" {
		retryAfter = &retryAfterValue
		ginContext.Header("Retry-After", retryAfterValue)
	}

	requestID := requestIDFromContext(ginContext)
	if structuredLogger != nil {
		logFields := []any{
			logFieldProvider, providerIdentifier,
			logFieldRetryable, retryable,
			logFieldRequestID, requestID,
		}
		if hasUpstreamStatus {
			logFields = append(logFields, logFieldUpstreamCode, upstreamStatus)
		}
		if retryAfter != nil {
			logFields = append(logFields, logFieldRetryAfter, *retryAfter)
		}
		if len(providerErrorCodes) > 0 {
			logFields = append(logFields, logFieldProviderErrorCodes, providerErrorCodes)
		}
		structuredLogger.Warnw(logEventProviderFailure, logFields...)
	}

	if _, ok := ginContext.Get(contextKeyClientErrorEncoder); ok {
		writeOpenAIError(ginContext, statusCodeForError(requestError), errorCode, "The provider request failed.")
		return
	}

	ginContext.JSON(
		statusCodeForError(requestError),
		providerErrorEnvelope{
			Error: providerErrorDetail{
				Code:           errorCode,
				Provider:       providerIdentifier,
				UpstreamStatus: upstreamStatusValue,
				Retryable:      retryable,
				RequestID:      requestID,
				RetryAfter:     retryAfter,
			},
		},
	)
}
