package proxy

import (
	"errors"
)

// ErrUnknownModel is returned when a model identifier is not recognized.
var ErrUnknownModel = errors.New(errorUnknownModel)

// modelValidator validates model identifiers using the static provider registry.
type modelValidator struct {
	registry *providerRegistry
}

// newModelValidator creates a modelValidator.
func newModelValidator(registry *providerRegistry) *modelValidator {
	return &modelValidator{registry: registry}
}

// ResolveText validates and resolves a provider/model pair for text generation.
func (validator *modelValidator) ResolveText(providerIdentifier string, modelIdentifier string, defaultProvider string, defaultModel string, webSearchEnabled bool) (providerDefinition, textModelDefinition, error) {
	return validator.registry.resolveTextRequest(providerIdentifier, modelIdentifier, defaultProvider, defaultModel, webSearchEnabled)
}

// ResolveDictation validates and resolves a provider/model pair for audio transcription.
func (validator *modelValidator) ResolveDictation(providerIdentifier string, modelIdentifier string, defaultProvider string, defaultModel string) (providerDefinition, modelID, error) {
	return validator.registry.resolveDictationRequest(providerIdentifier, modelIdentifier, defaultProvider, defaultModel)
}
