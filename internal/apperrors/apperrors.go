// Package apperrors provides shared application error values.
package apperrors

import "errors"

const (
	configurationFieldServiceSecret = "server.service_secret"
	configurationFieldOpenAIAPIKey  = "providers.openai.api_key"
	messageSuffixMustBeSet          = " must be set"
	messageMissingServiceSecret     = configurationFieldServiceSecret + messageSuffixMustBeSet
	messageMissingOpenAIKey         = configurationFieldOpenAIAPIKey + messageSuffixMustBeSet
)

var (
	// ErrMissingServiceSecret is returned when the service secret configuration field is empty.
	ErrMissingServiceSecret = errors.New(messageMissingServiceSecret)
	// ErrMissingOpenAIKey is returned when the OpenAI API key configuration field is empty.
	ErrMissingOpenAIKey = errors.New(messageMissingOpenAIKey)
)
