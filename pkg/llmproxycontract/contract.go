// Package llmproxycontract exposes canonical llm-proxy wire-contract literals.
package llmproxycontract

const (
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
	// ErrorCodeRequestTimeout identifies expiration of the accepted proxy work budget.
	ErrorCodeRequestTimeout = "request_timeout"
)
