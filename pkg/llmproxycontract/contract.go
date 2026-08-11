// Package llmproxycontract exposes canonical llm-proxy wire-contract literals.
package llmproxycontract

const (
	// AssetPath is the authenticated tenant asset upload endpoint.
	AssetPath = "/model/v1/assets"
	// HeaderAssetSHA256 carries the canonical lowercase hexadecimal digest for an asset upload.
	HeaderAssetSHA256 = "X-LLM-Proxy-Asset-SHA256"
	// HeaderRequestID carries the proxy-owned identifier used to correlate one public request with structured logs.
	HeaderRequestID = "X-LLM-Proxy-Request-ID"
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
)
