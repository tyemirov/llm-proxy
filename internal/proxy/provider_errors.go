package proxy

import "errors"

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
)
