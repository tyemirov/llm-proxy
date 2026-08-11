package llmproxyclient

import (
	"encoding/json"
	"fmt"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

// HTTPFailure is a completed non-2xx llm-proxy response. It exposes only the
// safe status and recognized stable proxy error code; the raw response body is
// deliberately unavailable.
type HTTPFailure struct {
	statusCode     int
	proxyErrorCode string
}

// Error reports the sanitized completed-response failure.
func (failure *HTTPFailure) Error() string {
	if failure.proxyErrorCode == "" {
		return fmt.Sprintf("%s: status=%d", ErrClientHTTPFailure, failure.statusCode)
	}
	return fmt.Sprintf(
		"%s: status=%d code=%s",
		ErrClientHTTPFailure,
		failure.statusCode,
		failure.proxyErrorCode,
	)
}

// Unwrap preserves errors.Is(error, ErrClientHTTPFailure).
func (failure *HTTPFailure) Unwrap() error {
	return ErrClientHTTPFailure
}

// StatusCode returns the proxy HTTP response status.
func (failure *HTTPFailure) StatusCode() int {
	return failure.statusCode
}

// ProxyErrorCode returns the recognized stable proxy error code, or an empty
// string when the response has no recognized structured error code.
func (failure *HTTPFailure) ProxyErrorCode() string {
	return failure.proxyErrorCode
}

func newHTTPFailure(statusCode int, responseBody []byte) *HTTPFailure {
	return &HTTPFailure{
		statusCode:     statusCode,
		proxyErrorCode: recognizedProxyErrorCode(responseBody),
	}
}

func recognizedProxyErrorCode(responseBody []byte) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if decodeError := json.Unmarshal(responseBody, &envelope); decodeError != nil {
		return ""
	}
	switch envelope.Error.Code {
	case llmproxycontract.ErrorCodeInvalidRequestTimeout,
		llmproxycontract.ErrorCodeProviderError,
		llmproxycontract.ErrorCodeProviderMediaLimitExceeded,
		llmproxycontract.ErrorCodeProviderRateLimited,
		llmproxycontract.ErrorCodeRequestTimeout:
		return envelope.Error.Code
	default:
		return ""
	}
}
