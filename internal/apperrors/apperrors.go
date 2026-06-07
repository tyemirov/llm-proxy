// Package apperrors provides shared application error values.
package apperrors

import "errors"

const (
	configurationFieldOpenAIAPIKey = "providers.openai.api_key"
	messageSuffixMustBeSet         = " must be set"
	messageMissingOpenAIKey        = configurationFieldOpenAIAPIKey + messageSuffixMustBeSet
)

var (
	// ErrMissingOpenAIKey is returned when the OpenAI API key configuration field is empty.
	ErrMissingOpenAIKey = errors.New(messageMissingOpenAIKey)
)
