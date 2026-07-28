package proxy

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

var (
	// ErrUnknownProvider is returned when a request names a provider that is not registered.
	ErrUnknownProvider = errors.New(errorUnknownProvider)
	// ErrProviderNotConfigured is returned when a registered provider lacks a required server-side credential.
	ErrProviderNotConfigured = errors.New(errorProviderNotConfigured)
	// ErrUnsupportedCapability is returned when a request asks a provider for an unsupported capability.
	ErrUnsupportedCapability = errors.New(errorUnsupportedCapability)
	// ErrUnsupportedEndpoint is returned when a provider does not support the requested endpoint.
	ErrUnsupportedEndpoint = errors.New(errorUnsupportedEndpoint)
	// ErrConflictingModelParameters is returned when query and JSON body model values disagree.
	ErrConflictingModelParameters = errors.New(errorConflictingModelParameters)
	// ErrProviderRateLimited is returned when an upstream provider reports rate limiting.
	ErrProviderRateLimited = errors.New(errorProviderRateLimited)
	// ErrProviderAPI is returned when an upstream provider returns an unsuccessful response.
	ErrProviderAPI = errors.New(errorProviderAPI)
	// ErrInvalidChatMessages is returned when a JSON request body contains invalid chat messages.
	ErrInvalidChatMessages = errors.New(errorInvalidChatMessages)
	// ErrInvalidModelCatalog is returned when configured provider model catalogs are incomplete or inconsistent.
	ErrInvalidModelCatalog = errors.New("invalid_model_catalog")
)

type providerHTTPError struct {
	error
	statusCode    int
	retryAfter    string
	retryableHint bool
}

func newProviderHTTPError(statusCode int, responseHeader http.Header) error {
	cause := ErrProviderAPI
	if statusCode == http.StatusTooManyRequests {
		cause = ErrProviderRateLimited
	}
	return &providerHTTPError{
		error:         cause,
		statusCode:    statusCode,
		retryAfter:    sanitizedRetryAfter(responseHeader.Get("Retry-After")),
		retryableHint: retryableProviderStatus(statusCode),
	}
}

func (providerError *providerHTTPError) Unwrap() error {
	return providerError.error
}

func providerHTTPMetadata(requestError error) (int, string, bool, bool) {
	var providerError *providerHTTPError
	if !errors.As(requestError, &providerError) {
		return 0, "", false, false
	}
	return providerError.statusCode, providerError.retryAfter, providerError.retryableHint, true
}

func sanitizedRetryAfter(rawValue string) string {
	trimmedValue := strings.TrimSpace(rawValue)
	if retryAfterSeconds, parseError := strconv.ParseUint(trimmedValue, 10, 63); parseError == nil {
		return strconv.FormatUint(retryAfterSeconds, 10)
	}
	retryAfterTime, parseError := http.ParseTime(trimmedValue)
	if parseError != nil {
		return ""
	}
	return retryAfterTime.UTC().Format(http.TimeFormat)
}

func retryableProviderStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
