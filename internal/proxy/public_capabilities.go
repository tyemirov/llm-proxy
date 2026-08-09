package proxy

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// PublicCapabilitiesPath is the canonical unauthenticated capability catalog
// resource consumed by public frontend builds and external API clients.
const PublicCapabilitiesPath = "/api/public/capabilities"

// PublicCapabilityCatalog is the sanitized provider, model, and request-limit
// contract published on the public site.
type PublicCapabilityCatalog struct {
	Providers                []PublicProviderCapability `json:"providers"`
	MaxPromptBytes           int64                      `json:"max_prompt_bytes"`
	MaxInputAudioBytes       int64                      `json:"max_input_audio_bytes"`
	MaxRequestTimeoutSeconds int                        `json:"max_request_timeout_seconds"`
}

// PublicProviderCapability describes the public routing contract for one
// canonical provider.
type PublicProviderCapability struct {
	Identifier string                  `json:"identifier"`
	Label      string                  `json:"label"`
	Models     []PublicModelCapability `json:"models"`
}

// PublicModelCapability describes the public request capabilities for one
// exact provider and model.
type PublicModelCapability struct {
	Identifier       string   `json:"identifier"`
	DefaultEndpoints []string `json:"default_endpoints"`
	Capabilities     []string `json:"capabilities"`
	WireContract     string   `json:"wire_contract"`
	OutputTokenLimit int      `json:"output_token_limit"`
	ReasoningEfforts []string `json:"reasoning_efforts"`
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
	if catalogError := validateProviderModelCatalogs(configuration.ProviderModels); catalogError != nil {
		return PublicCapabilityCatalog{}, catalogError
	}
	return newPublicCapabilityCatalog(configuration, registry), nil
}

func newPublicCapabilityCatalog(configuration Configuration, registry *providerRegistry) PublicCapabilityCatalog {
	providerIdentifiers := make([]string, 0, len(configuration.ProviderModels))
	for providerIdentifier := range configuration.ProviderModels {
		providerIdentifiers = append(providerIdentifiers, providerIdentifier)
	}
	sort.Strings(providerIdentifiers)
	providers := make([]PublicProviderCapability, 0, len(providerIdentifiers))
	for _, providerIdentifier := range providerIdentifiers {
		definition := registry.definitions[providerID(providerIdentifier)]
		providers = append(providers, PublicProviderCapability{
			Identifier: providerIdentifier,
			Label:      providerLabel(definition.identifier),
			Models:     publicModelCapabilities(definition),
		})
	}
	return PublicCapabilityCatalog{
		Providers:                providers,
		MaxPromptBytes:           configuration.MaxPromptBytes,
		MaxInputAudioBytes:       configuration.MaxInputAudioBytes,
		MaxRequestTimeoutSeconds: configuration.MaxRequestTimeoutSeconds,
	}
}

func registerPublicCapabilityRoutes(router *gin.Engine, capabilityCatalog PublicCapabilityCatalog) {
	router.GET(PublicCapabilitiesPath, func(ginContext *gin.Context) {
		ginContext.Header("Cache-Control", "public, max-age=300")
		ginContext.JSON(http.StatusOK, capabilityCatalog)
	})
}

// BuildPublicCapabilityRouter constructs the minimal public REST surface used
// when frontend tooling needs capability data without private runtime config.
func BuildPublicCapabilityRouter(capabilityCatalog PublicCapabilityCatalog, logLevel string) *gin.Engine {
	if strings.ToLower(logLevel) == LogLevelDebug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	registerPublicCapabilityRoutes(router, capabilityCatalog)
	return router
}

// ServePublicCapabilities starts the minimal public REST surface on port.
func ServePublicCapabilities(capabilityCatalog PublicCapabilityCatalog, port int, logLevel string) error {
	return BuildPublicCapabilityRouter(capabilityCatalog, logLevel).Run(fmt.Sprintf(":%d", port))
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
			modelCapability = PublicModelCapability{
				Identifier:       modelIdentifier,
				DefaultEndpoints: []string{},
				Capabilities:     []string{},
				ReasoningEfforts: []string{},
			}
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
