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

// PublicCapabilityCatalog is the normalized tenant-safe model discovery contract.
type PublicCapabilityCatalog struct {
	Providers                []PublicProviderCapability   `json:"providers"`
	Publishers               []PublicModelPublisher       `json:"publishers"`
	Families                 []PublicModelFamily          `json:"families"`
	Models                   []PublicExactModelCapability `json:"models"`
	Offerings                []PublicProviderOffering     `json:"offerings"`
	Counts                   PublicCapabilityCounts       `json:"counts"`
	MaxPromptBytes           int64                        `json:"max_prompt_bytes"`
	MaxInputAudioBytes       int64                        `json:"max_input_audio_bytes"`
	MaxRequestTimeoutSeconds int                          `json:"max_request_timeout_seconds"`
}

// PublicCapabilityCounts reports each normalized catalog dimension separately.
type PublicCapabilityCounts struct {
	Providers         int `json:"providers"`
	ModelPublishers   int `json:"model_publishers"`
	ModelFamilies     int `json:"model_families"`
	ExactModels       int `json:"exact_models"`
	ProviderOfferings int `json:"provider_offerings"`
}

// PublicProviderCapability identifies one selectable provider.
type PublicProviderCapability struct {
	Identifier string `json:"identifier"`
	Label      string `json:"label"`
}

// PublicModelPublisher identifies one model publisher and its exact-model count.
type PublicModelPublisher struct {
	Identifier string `json:"identifier"`
	Label      string `json:"label"`
	ModelCount int    `json:"model_count"`
}

// PublicModelFamily identifies one family within a model publisher.
type PublicModelFamily struct {
	Identifier string `json:"identifier"`
	Publisher  string `json:"publisher"`
	Label      string `json:"label"`
}

// PublicExactModelCapability describes one provider-independent exact model.
type PublicExactModelCapability struct {
	Identifier        string   `json:"identifier"`
	Publisher         string   `json:"publisher"`
	Family            string   `json:"family"`
	Version           string   `json:"version"`
	Operations        []string `json:"operations"`
	MediaInputs       []string `json:"media_inputs"`
	Capabilities      []string `json:"capabilities"`
	ProviderOfferings []string `json:"provider_offerings"`
}

// PublicProviderOffering describes one selectable provider and exact-model route.
type PublicProviderOffering struct {
	Identifier       string   `json:"identifier"`
	Provider         string   `json:"provider"`
	Model            string   `json:"model"`
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

// NewPublicCapabilityCatalog validates and projects the runtime catalog into a
// deterministic public representation.
func NewPublicCapabilityCatalog(configuration Configuration) (PublicCapabilityCatalog, error) {
	configuration.ApplyTunables()
	if _, catalogError := validateModelCatalog(configuration.ModelCatalog); catalogError != nil {
		return PublicCapabilityCatalog{}, catalogError
	}
	return newPublicCapabilityCatalog(configuration), nil
}

func newPublicCapabilityCatalog(configuration Configuration) PublicCapabilityCatalog {
	providers := make([]PublicProviderCapability, 0, len(configuration.ModelCatalog.Providers))
	for _, provider := range configuration.ModelCatalog.Providers {
		providers = append(providers, PublicProviderCapability{Identifier: provider.ID, Label: provider.Label})
	}
	sort.Slice(providers, func(first int, second int) bool { return providers[first].Identifier < providers[second].Identifier })

	modelCounts := map[string]int{}
	for _, model := range configuration.ModelCatalog.Models {
		modelCounts[model.Publisher]++
	}
	publishers := make([]PublicModelPublisher, 0, len(configuration.ModelCatalog.Publishers))
	for _, publisher := range configuration.ModelCatalog.Publishers {
		publishers = append(publishers, PublicModelPublisher{Identifier: publisher.ID, Label: publisher.Label, ModelCount: modelCounts[publisher.ID]})
	}
	sort.Slice(publishers, func(first int, second int) bool { return publishers[first].Identifier < publishers[second].Identifier })

	families := make([]PublicModelFamily, 0, len(configuration.ModelCatalog.Families))
	for _, family := range configuration.ModelCatalog.Families {
		families = append(families, PublicModelFamily{Identifier: family.ID, Publisher: family.Publisher, Label: family.Label})
	}
	sort.Slice(families, func(first int, second int) bool { return families[first].Identifier < families[second].Identifier })

	offerings := make([]PublicProviderOffering, 0, len(configuration.ModelCatalog.Offerings))
	offeringIdentifiersByModel := map[string][]string{}
	offeringCapabilitiesByModel := map[string]map[string]struct{}{}
	for _, offering := range configuration.ModelCatalog.Offerings {
		publicOffering := publicProviderOffering(offering)
		offerings = append(offerings, publicOffering)
		offeringIdentifiersByModel[offering.Model] = append(offeringIdentifiersByModel[offering.Model], publicOffering.Identifier)
		if offeringCapabilitiesByModel[offering.Model] == nil {
			offeringCapabilitiesByModel[offering.Model] = map[string]struct{}{}
		}
		for _, capability := range publicOffering.Capabilities {
			offeringCapabilitiesByModel[offering.Model][capability] = struct{}{}
		}
	}
	sort.Slice(offerings, func(first int, second int) bool { return offerings[first].Identifier < offerings[second].Identifier })

	models := make([]PublicExactModelCapability, 0, len(configuration.ModelCatalog.Models))
	for _, model := range configuration.ModelCatalog.Models {
		modelOfferings := append([]string(nil), offeringIdentifiersByModel[model.ID]...)
		sort.Strings(modelOfferings)
		capabilities := sortedPublicCapabilities(offeringCapabilitiesByModel[model.ID])
		models = append(models, PublicExactModelCapability{
			Identifier:        model.ID,
			Publisher:         model.Publisher,
			Family:            model.Family,
			Version:           model.Version,
			Operations:        append([]string{}, model.Operations...),
			MediaInputs:       append([]string{}, model.MediaInputs...),
			Capabilities:      capabilities,
			ProviderOfferings: modelOfferings,
		})
	}
	sort.Slice(models, func(first int, second int) bool { return models[first].Identifier < models[second].Identifier })

	return PublicCapabilityCatalog{
		Providers:  providers,
		Publishers: publishers,
		Families:   families,
		Models:     models,
		Offerings:  offerings,
		Counts: PublicCapabilityCounts{
			Providers:         len(providers),
			ModelPublishers:   len(publishers),
			ModelFamilies:     len(families),
			ExactModels:       len(models),
			ProviderOfferings: len(offerings),
		},
		MaxPromptBytes:           configuration.MaxPromptBytes,
		MaxInputAudioBytes:       configuration.MaxInputAudioBytes,
		MaxRequestTimeoutSeconds: configuration.MaxRequestTimeoutSeconds,
	}
}

func publicProviderOffering(offering ProviderOffering) PublicProviderOffering {
	capabilities := append([]string(nil), offering.Operations...)
	if offering.WebSearch {
		capabilities = append(capabilities, PublicModelCapabilityWebSearch)
	}
	for _, mediaInput := range offering.MediaInputs {
		capabilities = append(capabilities, publicMediaInputCapability(mediaInput))
	}
	reasoningEfforts := publicReasoningEfforts(offering.ReasoningEffort)
	if len(reasoningEfforts) != 0 {
		capabilities = append(capabilities, PublicModelCapabilityReasoning)
	}
	sort.Strings(capabilities)
	return PublicProviderOffering{
		Identifier:       providerOfferingIdentifier(offering.Provider, offering.Model),
		Provider:         offering.Provider,
		Model:            offering.Model,
		Capabilities:     capabilities,
		WireContract:     offering.WireContract,
		OutputTokenLimit: offering.OutputTokenLimit,
		ReasoningEfforts: reasoningEfforts,
	}
}

func sortedPublicCapabilities(capabilities map[string]struct{}) []string {
	result := make([]string, 0, len(capabilities))
	for capability := range capabilities {
		result = append(result, capability)
	}
	sort.Strings(result)
	return result
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

func publicMediaInputCapability(mediaInput string) string {
	if mediaInput == string(messageMediaTypeImage) {
		return PublicModelCapabilityImageInput
	}
	return PublicModelCapabilityAudioInput
}

func publicReasoningEfforts(capability *ReasoningEffortCapability) []string {
	if capability == nil {
		return []string{}
	}
	return append([]string(nil), capability.Efforts...)
}
