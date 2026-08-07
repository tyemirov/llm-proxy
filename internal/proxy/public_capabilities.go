package proxy

import (
	"fmt"
	"sort"
)

// PublicCapabilityCatalog is the sanitized provider, model, and request-limit
// contract published on the public site.
type PublicCapabilityCatalog struct {
	Providers                []PublicProviderCapability
	MaxPromptBytes           int64
	MaxInputAudioBytes       int64
	MaxRequestTimeoutSeconds int
}

// PublicProviderCapability describes the public routing contract for one
// canonical provider.
type PublicProviderCapability struct {
	Identifier string
	Label      string
	Models     []PublicModelCapability
}

// PublicModelCapability describes the public request capabilities for one
// exact provider and model.
type PublicModelCapability struct {
	Identifier       string
	DefaultEndpoints []string
	Capabilities     []string
	WireContract     string
	OutputTokenLimit int
	ReasoningEfforts []string
}

// Public model capability identifiers are the stable filter vocabulary for the
// generated public catalog.
const (
	PublicModelCapabilityText       = "text"
	PublicModelCapabilityDictation  = "dictation"
	PublicModelCapabilityWebSearch  = "web_search"
	PublicModelCapabilityImageInput = "image_input"
	PublicModelCapabilityAudioInput = "audio_input"
	PublicModelCapabilityReasoning  = "reasoning"
)

// NewPublicCapabilityCatalog validates and projects the runtime provider
// registry into a deterministic, secret-free public catalog.
func NewPublicCapabilityCatalog(configuration Configuration) (PublicCapabilityCatalog, error) {
	configuration.ApplyTunables()
	registry := newProviderRegistry(configuration)
	for rawProviderIdentifier := range configuration.ProviderModels {
		providerIdentifier := newProviderID(rawProviderIdentifier)
		if providerIdentifier.string() != rawProviderIdentifier {
			return PublicCapabilityCatalog{}, fmt.Errorf("%w: provider=%s reason=not_canonical", ErrInvalidModelCatalog, rawProviderIdentifier)
		}
		if _, knownProvider := registry.definitions[providerIdentifier]; !knownProvider {
			return PublicCapabilityCatalog{}, fmt.Errorf("%w: provider=%s reason=unknown", ErrInvalidModelCatalog, rawProviderIdentifier)
		}
	}
	if len(configuration.ProviderModels) != len(registry.definitions) {
		return PublicCapabilityCatalog{}, fmt.Errorf(
			"%w: configured_provider_count=%d canonical_provider_count=%d",
			ErrInvalidModelCatalog,
			len(configuration.ProviderModels),
			len(registry.definitions),
		)
	}
	if catalogError := validateProviderModelCatalogs(configuration.ProviderModels); catalogError != nil {
		return PublicCapabilityCatalog{}, catalogError
	}

	providerSummaries := registry.providerSummaries()
	providers := make([]PublicProviderCapability, 0, len(providerSummaries))
	for _, providerSummary := range providerSummaries {
		definition := registry.definitions[providerID(providerSummary.identifier)]
		providers = append(providers, PublicProviderCapability{
			Identifier: providerSummary.identifier,
			Label:      providerSummary.label,
			Models:     publicModelCapabilities(definition),
		})
	}
	return PublicCapabilityCatalog{
		Providers:                providers,
		MaxPromptBytes:           configuration.MaxPromptBytes,
		MaxInputAudioBytes:       configuration.MaxInputAudioBytes,
		MaxRequestTimeoutSeconds: configuration.MaxRequestTimeoutSeconds,
	}, nil
}

func publicModelCapabilities(definition providerDefinition) []PublicModelCapability {
	modelIdentifiers := make([]string, 0, len(definition.textModels)+len(definition.transcriptionModels))
	modelsByIdentifier := make(map[string]PublicModelCapability, cap(modelIdentifiers))
	for _, modelDefinition := range definition.textModels {
		modelIdentifier := modelDefinition.identifier.string()
		capabilities := []string{PublicModelCapabilityText}
		if modelDefinition.supportsWebSearch {
			capabilities = append(capabilities, PublicModelCapabilityWebSearch)
		}
		for _, mediaInput := range publicMediaInputs(modelDefinition.mediaInputs) {
			capabilities = append(capabilities, publicMediaInputCapability(mediaInput))
		}
		reasoningEfforts := publicReasoningEfforts(modelDefinition.reasoningEffort)
		if len(reasoningEfforts) != 0 {
			capabilities = append(capabilities, PublicModelCapabilityReasoning)
		}
		defaultEndpoints := []string{}
		if modelDefinition.identifier == definition.defaultTextModel {
			defaultEndpoints = append(defaultEndpoints, PublicModelCapabilityText)
		}
		modelsByIdentifier[modelIdentifier] = PublicModelCapability{
			Identifier:       modelIdentifier,
			DefaultEndpoints: defaultEndpoints,
			Capabilities:     capabilities,
			WireContract:     string(modelDefinition.wireContract),
			OutputTokenLimit: modelDefinition.outputTokenLimit,
			ReasoningEfforts: reasoningEfforts,
		}
		modelIdentifiers = append(modelIdentifiers, modelIdentifier)
	}
	for _, modelIdentifier := range sortedDictationModels(definition.transcriptionModels) {
		modelCapability, modelExists := modelsByIdentifier[modelIdentifier]
		if !modelExists {
			modelCapability = PublicModelCapability{Identifier: modelIdentifier}
			modelIdentifiers = append(modelIdentifiers, modelIdentifier)
		}
		modelCapability.Capabilities = append(modelCapability.Capabilities, PublicModelCapabilityDictation)
		if newModelID(modelIdentifier) == definition.defaultTranscriptionModel {
			modelCapability.DefaultEndpoints = append(modelCapability.DefaultEndpoints, PublicModelCapabilityDictation)
		}
		modelsByIdentifier[modelIdentifier] = modelCapability
	}
	sort.Strings(modelIdentifiers)

	models := make([]PublicModelCapability, 0, len(modelIdentifiers))
	for _, modelIdentifier := range modelIdentifiers {
		models = append(models, modelsByIdentifier[modelIdentifier])
	}
	return models
}

func publicMediaInputCapability(mediaInput string) string {
	if mediaInput == string(messageMediaTypeImage) {
		return PublicModelCapabilityImageInput
	}
	return PublicModelCapabilityAudioInput
}

func publicReasoningEfforts(capability *reasoningEffortCapability) []string {
	if capability == nil {
		return []string{}
	}
	return append([]string(nil), capability.efforts...)
}

func publicMediaInputs(mediaInputs map[messageMediaType]struct{}) []string {
	publicInputs := make([]string, 0, len(mediaInputs))
	for mediaInput := range mediaInputs {
		publicInputs = append(publicInputs, string(mediaInput))
	}
	sort.Strings(publicInputs)
	return publicInputs
}
