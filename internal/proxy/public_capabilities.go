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
	Identifier            string
	Label                 string
	TextDefaultModel      string
	TextModels            []PublicTextModelCapability
	DictationDefaultModel string
	DictationModels       []string
}

// PublicTextModelCapability describes the public request capabilities for one
// exact provider and text-model route.
type PublicTextModelCapability struct {
	Identifier         string
	Default            bool
	WireContract       string
	ExecutionLifecycle string
	WebSearch          bool
	OutputTokenLimit   int
	ReasoningEfforts   []string
	MediaInputs        []string
}

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
			Identifier:            providerSummary.identifier,
			Label:                 providerSummary.label,
			TextDefaultModel:      providerSummary.textDefaultModel,
			TextModels:            publicTextModelCapabilities(definition),
			DictationDefaultModel: providerSummary.dictationDefaultModel,
			DictationModels:       append([]string(nil), providerSummary.dictationModels...),
		})
	}
	return PublicCapabilityCatalog{
		Providers:                providers,
		MaxPromptBytes:           configuration.MaxPromptBytes,
		MaxInputAudioBytes:       configuration.MaxInputAudioBytes,
		MaxRequestTimeoutSeconds: configuration.MaxRequestTimeoutSeconds,
	}, nil
}

func publicTextModelCapabilities(definition providerDefinition) []PublicTextModelCapability {
	modelIdentifiers := make([]string, 0, len(definition.textModels))
	modelsByIdentifier := make(map[string]textModelDefinition, len(definition.textModels))
	for _, modelDefinition := range definition.textModels {
		modelIdentifier := modelDefinition.identifier.string()
		modelsByIdentifier[modelIdentifier] = modelDefinition
		modelIdentifiers = append(modelIdentifiers, modelIdentifier)
	}
	sort.Strings(modelIdentifiers)

	models := make([]PublicTextModelCapability, 0, len(modelIdentifiers))
	for _, modelIdentifier := range modelIdentifiers {
		modelDefinition := modelsByIdentifier[modelIdentifier]
		models = append(models, PublicTextModelCapability{
			Identifier:         modelIdentifier,
			Default:            modelDefinition.identifier == definition.defaultTextModel,
			WireContract:       string(modelDefinition.wireContract),
			ExecutionLifecycle: string(modelDefinition.executionLifecycle),
			WebSearch:          modelDefinition.supportsWebSearch,
			OutputTokenLimit:   modelDefinition.outputTokenLimit,
			ReasoningEfforts:   publicReasoningEfforts(modelDefinition.reasoningEffort),
			MediaInputs:        publicMediaInputs(modelDefinition.mediaInputs),
		})
	}
	return models
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
