// Package llmproxycontract exposes canonical llm-proxy wire-contract literals.
package llmproxycontract

const (
	// HeaderRequestTimeoutSeconds carries the accepted proxy work budget for one upstream request.
	HeaderRequestTimeoutSeconds = "X-LLM-Proxy-Request-Timeout-Seconds"
	// ErrorCodeInvalidRequestTimeout identifies a rejected request-timeout header.
	ErrorCodeInvalidRequestTimeout = "invalid_request_timeout"
	// ErrorCodeRequestTimeout identifies expiration of the accepted proxy work budget.
	ErrorCodeRequestTimeout = "request_timeout"
)
