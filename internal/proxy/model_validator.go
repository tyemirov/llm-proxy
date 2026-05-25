package proxy

import (
	"errors"
	"fmt"
)

// errUnknownModelFormat specifies the format string for wrapping an unknown model error.
const errUnknownModelFormat = "%w: %s"

// ErrUnknownModel is returned when a model identifier is not recognized.
var ErrUnknownModel = errors.New(errorUnknownModel)

// modelValidator validates model identifiers using the static provider registry.
type modelValidator struct {
	registry *providerRegistry
}

// newModelValidator creates a modelValidator.
func newModelValidator(registry *providerRegistry) (*modelValidator, error) {
	return &modelValidator{registry: registry}, nil
}

// VerifyText checks whether the provided provider/model pair is known.
func (validator *modelValidator) VerifyText(providerIdentifier string, modelIdentifier string, defaultProvider string, defaultModel string, webSearchEnabled bool) error {
	_, _, resolutionError := validator.ResolveText(providerIdentifier, modelIdentifier, defaultProvider, defaultModel, webSearchEnabled)
	if resolutionError != nil {
		return resolutionError
	}
	return nil
}

// ResolveText validates and resolves a provider/model pair for text generation.
func (validator *modelValidator) ResolveText(providerIdentifier string, modelIdentifier string, defaultProvider string, defaultModel string, webSearchEnabled bool) (providerDefinition, modelID, error) {
	return validator.registry.resolveTextRequest(providerIdentifier, modelIdentifier, defaultProvider, defaultModel, webSearchEnabled)
}

// ResolveDictation validates and resolves a provider/model pair for audio transcription.
func (validator *modelValidator) ResolveDictation(providerIdentifier string, modelIdentifier string, defaultProvider string, defaultModel string) (providerDefinition, modelID, error) {
	return validator.registry.resolveDictationRequest(providerIdentifier, modelIdentifier, defaultProvider, defaultModel)
}

// Verify checks whether the provided model identifier is known.
func (validator *modelValidator) Verify(modelIdentifier string) error {
	if _, known := modelPayloadSchemas[modelIdentifier]; !known {
		return fmt.Errorf(errUnknownModelFormat, ErrUnknownModel, modelIdentifier)
	}
	return nil
}
