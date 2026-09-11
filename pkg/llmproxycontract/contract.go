// Package llmproxycontract exposes canonical llm-proxy wire-contract literals.
package llmproxycontract

const (
	// AssetPath is the authenticated tenant asset upload endpoint.
	AssetPath = "/model/v1/assets"
	// MediaCapabilitiesPath is the authenticated tenant media capability resource.
	MediaCapabilitiesPath = "/model/v1/capabilities"
	// MediaOperationsPath is the authenticated tenant media operation collection.
	MediaOperationsPath = "/model/v1/operations"
	// MediaCapabilityVideoGenerate identifies durable video generation.
	MediaCapabilityVideoGenerate = "video.generate"
	// MediaOperationStateQueued identifies accepted work awaiting a worker.
	MediaOperationStateQueued = "queued"
	// MediaOperationStateRunning identifies claimed work with current execution authority.
	MediaOperationStateRunning = "running"
	// MediaOperationStateSucceeded identifies a terminal operation with published outputs.
	MediaOperationStateSucceeded = "succeeded"
	// MediaOperationStateFailed identifies a terminal operation with caller-safe error evidence.
	MediaOperationStateFailed = "failed"
	// MediaOperationStateCancelled identifies provider-confirmed or pre-dispatch cancellation.
	MediaOperationStateCancelled = "cancelled"
	// MediaOperationStateUncertain identifies a provider outcome that cannot be proved.
	MediaOperationStateUncertain = "uncertain"
	// MediaCancellationNotRequested identifies an operation without a cancellation request.
	MediaCancellationNotRequested = "not_requested"
	// MediaCancellationRequested identifies a persisted cancellation request awaiting observation.
	MediaCancellationRequested = "requested"
	// MediaCancellationConfirmed identifies proved cancellation.
	MediaCancellationConfirmed = "confirmed"
	// MediaCancellationUnsupported identifies a provider that cannot confirm cancellation.
	MediaCancellationUnsupported = "unsupported"
	// HeaderRequestID carries the proxy-owned identifier used to correlate one public request with structured logs.
	HeaderRequestID = "X-LLM-Proxy-Request-ID"
	// HeaderIdempotencyKey binds one structured request intent to one durable provider submission.
	HeaderIdempotencyKey = "Idempotency-Key"
	// HeaderStructuredRequestState reports the durable state of a structured request.
	HeaderStructuredRequestState = "X-LLM-Proxy-Structured-Request-State"
	// StructuredRequestPath is the authenticated reconciliation endpoint for structured v2 requests.
	StructuredRequestPath = "/v2/requests"
	// TenantIdentityPath is the authenticated tenant identity endpoint.
	TenantIdentityPath = "/v2/identity"
	// HeaderRequestTimeoutSeconds carries the accepted proxy work budget for one upstream request.
	HeaderRequestTimeoutSeconds = "X-LLM-Proxy-Request-Timeout-Seconds"
	// ErrorCodeInvalidRequestTimeout identifies a rejected request-timeout header.
	ErrorCodeInvalidRequestTimeout = "invalid_request_timeout"
	// ErrorCodeProviderError identifies a sanitized upstream provider failure.
	ErrorCodeProviderError = "provider_error"
	// ErrorCodeProviderRateLimited identifies a sanitized upstream provider rate-limit response.
	ErrorCodeProviderRateLimited = "provider_rate_limited"
	// ErrorCodeProviderMediaLimitExceeded identifies media that exceeds the selected provider offering limit.
	ErrorCodeProviderMediaLimitExceeded = "provider_media_limit_exceeded"
	// ErrorCodeRequestTimeout identifies expiration of the accepted proxy work budget.
	ErrorCodeRequestTimeout = "request_timeout"
	// ErrorCodeInvalidIdempotencyKey identifies a missing or malformed idempotency key.
	ErrorCodeInvalidIdempotencyKey = "invalid_idempotency_key"
	// ErrorCodeStructuredRequestNotFound identifies a missing tenant-bound durable request.
	ErrorCodeStructuredRequestNotFound = "structured_request_not_found"
	// ErrorCodeStructuredRequestIntentConflict identifies reuse of one key for a different request intent.
	ErrorCodeStructuredRequestIntentConflict = "structured_request_intent_conflict"
	// ErrorCodeStructuredRequestOutcomeUnknown identifies a dispatched request whose provider outcome cannot be proved.
	ErrorCodeStructuredRequestOutcomeUnknown = "structured_request_outcome_unknown"
	// ErrorCodeStructuredRequestFailed identifies a terminal provider failure recorded for a structured request.
	ErrorCodeStructuredRequestFailed = "structured_request_failed"
	// ErrorCodeStructuredRequestInvalid identifies invalid structured-request state at submission.
	ErrorCodeStructuredRequestInvalid = "structured_request_invalid"
	// ErrorCodeStructuredRequestStore identifies a durable request-store failure.
	ErrorCodeStructuredRequestStore = "structured_request_store_error"
	// ErrorCodeMediaOperationInvalid identifies an invalid media operation request.
	ErrorCodeMediaOperationInvalid = "media_operation_invalid"
	// ErrorCodeMediaOperationUnavailable identifies a route without an available execution adapter or credential.
	ErrorCodeMediaOperationUnavailable = "media_operation_unavailable"
	// ErrorCodeMediaOperationNotFound identifies a missing tenant-owned operation.
	ErrorCodeMediaOperationNotFound = "media_operation_not_found"
	// ErrorCodeMediaOperationIntentConflict identifies changed intent under an accepted key.
	ErrorCodeMediaOperationIntentConflict = "media_operation_intent_conflict"
	// ErrorCodeMediaOperationCapacity identifies exhausted accepted-operation capacity.
	ErrorCodeMediaOperationCapacity = "media_operation_capacity_exceeded"
	// ErrorCodeMediaOperationStore identifies a durable media store failure.
	ErrorCodeMediaOperationStore = "media_operation_store_error"
	// ErrorCodeMediaOperationExpired identifies a retained idempotency tombstone after operation detail expiry.
	ErrorCodeMediaOperationExpired = "media_operation_expired"
	// ErrorCodeAssetInUse identifies an asset protected by an active durable operation reference.
	ErrorCodeAssetInUse = "asset_in_use"
)
