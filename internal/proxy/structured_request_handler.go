package proxy

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

const structuredJSONContentType = "application/json; charset=utf-8"

type structuredRequestStatusEnvelope struct {
	State          string `json:"state"`
	ProxyRequestID string `json:"proxy_request_id"`
	StartedAt      string `json:"started_at"`
	UpdatedAt      string `json:"updated_at"`
	ElapsedSeconds int64  `json:"elapsed_seconds"`
}

type structuredRequestErrorEnvelope struct {
	Error structuredRequestError `json:"error"`
}

type structuredRequestError struct {
	Code           string `json:"code"`
	State          string `json:"state,omitempty"`
	Cause          string `json:"cause,omitempty"`
	ProxyRequestID string `json:"proxy_request_id,omitempty"`
}

func structuredRequestStatusHandler(store *structuredRequestStore) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		ginContext.Header(headerCacheControl, cacheControlNoStore)
		idempotencyKey, keyError := reconciliationIdempotencyKey(ginContext)
		if keyError != nil {
			writeStructuredRequestError(ginContext, http.StatusBadRequest, llmproxycontract.ErrorCodeInvalidIdempotencyKey, "", "", "")
			return
		}
		record, lookupError := store.lookup(authenticatedTenantFromContext(ginContext), idempotencyKey)
		if errors.Is(lookupError, errStructuredRequestNotFound) {
			writeStructuredRequestError(ginContext, http.StatusNotFound, llmproxycontract.ErrorCodeStructuredRequestNotFound, "", "", "")
			return
		}
		if lookupError != nil {
			writeStructuredRequestError(ginContext, http.StatusInternalServerError, llmproxycontract.ErrorCodeStructuredRequestStore, "", "", "")
			return
		}
		writeStructuredRequestRecord(ginContext, record, store.now().UTC())
	}
}

func reconciliationIdempotencyKey(ginContext *gin.Context) (string, error) {
	values := ginContext.Request.Header.Values(llmproxycontract.HeaderIdempotencyKey)
	if len(values) != 1 || !validIdempotencyKey(values[0]) {
		return "", errStructuredOutputInvalid
	}
	return values[0], nil
}

func writeStructuredRequestRecord(ginContext *gin.Context, record structuredRequestRecord, now time.Time) {
	ginContext.Header(llmproxycontract.HeaderStructuredRequestState, record.State)
	switch record.State {
	case structuredRequestStateSucceeded:
		ginContext.Data(http.StatusOK, structuredJSONContentType, append([]byte(nil), record.Result...))
	case structuredRequestStateNotDispatched, structuredRequestStateDispatched:
		startedAt, _ := time.Parse(time.RFC3339Nano, record.StartedAt)
		elapsed := now.Sub(startedAt)
		if elapsed < 0 {
			elapsed = 0
		}
		ginContext.JSON(http.StatusAccepted, structuredRequestStatusEnvelope{
			State: record.State, ProxyRequestID: record.ProxyRequestID,
			StartedAt: record.StartedAt, UpdatedAt: record.UpdatedAt,
			ElapsedSeconds: int64(elapsed / time.Second),
		})
	case structuredRequestStateFailed:
		statusCode := record.StatusCode
		if statusCode < 400 || statusCode >= 600 {
			statusCode = http.StatusBadGateway
		}
		writeStructuredRequestError(ginContext, statusCode, llmproxycontract.ErrorCodeStructuredRequestFailed, record.State, record.FailureCode, record.ProxyRequestID)
	case structuredRequestStateUncertain:
		writeStructuredRequestError(ginContext, http.StatusConflict, llmproxycontract.ErrorCodeStructuredRequestOutcomeUnknown, record.State, record.FailureCode, record.ProxyRequestID)
	default:
		writeStructuredRequestError(ginContext, http.StatusInternalServerError, llmproxycontract.ErrorCodeStructuredRequestStore, "", "", "")
	}
}

func writeStructuredRequestError(ginContext *gin.Context, statusCode int, code string, state string, cause string, proxyRequestID string) {
	ginContext.JSON(statusCode, structuredRequestErrorEnvelope{Error: structuredRequestError{
		Code: code, State: state, Cause: cause, ProxyRequestID: proxyRequestID,
	}})
}

func submitStructuredChatRequest(
	ginContext *gin.Context,
	upstreamProviders *providerRouter,
	chatRequest chatRequestParameters,
	requestTenant tenant,
	bodyBytes []byte,
	store *structuredRequestStore,
	managedTenants *managedTenantStore,
	structuredLogger *zap.SugaredLogger,
	requestStart time.Time,
) {
	canonicalBody, canonicalError := canonicalJSON(bodyBytes)
	if canonicalError != nil {
		writeStructuredRequestError(ginContext, http.StatusBadRequest, llmproxycontract.ErrorCodeStructuredRequestInvalid, "", "", "")
		return
	}
	intentSHA256 := structuredRequestIntent(chatRequest.provider.identifier.string(), chatRequest.model.string(), canonicalBody)
	_, _, beginError := store.begin(
		requestTenant, chatRequest.idempotencyKey, intentSHA256,
		chatRequest.provider.identifier.string(), chatRequest.model.string(), requestIDFromContext(ginContext),
	)
	if errors.Is(beginError, errStructuredRequestConflict) {
		writeStructuredRequestError(ginContext, http.StatusConflict, llmproxycontract.ErrorCodeStructuredRequestIntentConflict, "", "", "")
		return
	}
	if beginError != nil {
		writeStructuredRequestError(ginContext, http.StatusInternalServerError, llmproxycontract.ErrorCodeStructuredRequestStore, "", "", "")
		return
	}
	claimed, claimError := store.claimDispatch(requestTenant, chatRequest.idempotencyKey, intentSHA256)
	if claimError != nil {
		writeStructuredRequestError(ginContext, http.StatusInternalServerError, llmproxycontract.ErrorCodeStructuredRequestStore, "", "", "")
		return
	}
	if !claimed {
		latest, lookupError := store.lookup(requestTenant, chatRequest.idempotencyKey)
		if lookupError != nil {
			writeStructuredRequestError(ginContext, http.StatusInternalServerError, llmproxycontract.ErrorCodeStructuredRequestStore, "", "", "")
			return
		}
		writeStructuredRequestRecord(ginContext, latest, store.now().UTC())
		return
	}

	bindRequestTelemetryRoute(ginContext, chatRequest.provider.identifier.string(), chatRequest.model.string())
	generation, requestError := upstreamProviders.generateText(ginContext.Request.Context(), chatRequest, structuredLogger)
	if requestError != nil {
		if requestContextEnded(ginContext) {
			_ = store.uncertain(requestTenant, chatRequest.idempotencyKey, intentSHA256)
			recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, chatRequest.provider.identifier.string(), chatRequest.model.string(), ginContext.Writer.Status(), generation.usage, requestStart)
			return
		}
		markRequestOutcome(ginContext, requestFailureOutcome(requestError), managedRequestFailureOutcome(requestError))
		statusCode := statusCodeForError(requestError)
		if failError := store.fail(requestTenant, chatRequest.idempotencyKey, intentSHA256, statusCode, structuredRequestFailureCause(statusCode)); failError != nil {
			writeStructuredRequestError(ginContext, http.StatusInternalServerError, llmproxycontract.ErrorCodeStructuredRequestStore, "", "", "")
			recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, chatRequest.provider.identifier.string(), chatRequest.model.string(), http.StatusInternalServerError, generation.usage, requestStart)
			return
		}
		writeProviderRequestErrorResponse(ginContext, chatRequest.provider.identifier.string(), requestError, structuredLogger)
		recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, chatRequest.provider.identifier.string(), chatRequest.model.string(), statusCode, generation.usage, requestStart)
		return
	}
	if persistError := store.succeed(requestTenant, chatRequest.idempotencyKey, intentSHA256, generation.text); persistError != nil {
		writeStructuredRequestError(ginContext, http.StatusInternalServerError, llmproxycontract.ErrorCodeStructuredRequestStore, "", "", "")
		recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, chatRequest.provider.identifier.string(), chatRequest.model.string(), http.StatusInternalServerError, generation.usage, requestStart)
		return
	}
	if requestContextEnded(ginContext) {
		recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, chatRequest.provider.identifier.string(), chatRequest.model.string(), ginContext.Writer.Status(), generation.usage, requestStart)
		return
	}
	markRequestOutcome(ginContext, requestOutcomeSuccess, managedUsageOutcomeSuccess)
	writeTokenUsageHeaders(ginContext.Writer.Header(), generation.usage)
	ginContext.Header(llmproxycontract.HeaderStructuredRequestState, structuredRequestStateSucceeded)
	ginContext.Data(http.StatusOK, structuredJSONContentType, []byte(generation.text))
	recordManagedUsage(managedTenants, structuredLogger, ginContext, requestTenant, usageEndpointV2, chatRequest.provider.identifier.string(), chatRequest.model.string(), http.StatusOK, generation.usage, requestStart)
}

func structuredRequestFailureCause(statusCode int) string {
	switch statusCode {
	case http.StatusTooManyRequests:
		return llmproxycontract.ErrorCodeProviderRateLimited
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	case http.StatusGatewayTimeout:
		return llmproxycontract.ErrorCodeRequestTimeout
	default:
		return llmproxycontract.ErrorCodeProviderError
	}
}
