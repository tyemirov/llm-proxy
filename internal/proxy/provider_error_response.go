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
		ginContext.String(statusCodeForError(requestError), requestError.Error())
		return
	}
	writeProviderErrorResponse(ginContext, providerIdentifier, requestError, structuredLogger)
}

func writeProviderErrorResponse(ginContext *gin.Context, providerIdentifier string, requestError error, structuredLogger *zap.SugaredLogger) {
	errorCode := llmproxycontract.ErrorCodeProviderError
	if errors.Is(requestError, ErrProviderRateLimited) {
		errorCode = llmproxycontract.ErrorCodeProviderRateLimited
	}

	upstreamStatus, retryAfterValue, retryable, hasUpstreamStatus := providerHTTPMetadata(requestError)
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
		structuredLogger.Warnw(logEventProviderFailure, logFields...)
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
