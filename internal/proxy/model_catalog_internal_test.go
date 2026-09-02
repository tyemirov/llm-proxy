package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
)

func internalCanonicalProviderCatalog() *ProviderCatalog {
	_, sourceFile, _, _ := runtime.Caller(0)
	document, readError := os.ReadFile(filepath.Join(filepath.Dir(sourceFile), "..", "..", "configs", "providers.yml"))
	if readError != nil {
		panic(readError)
	}
	catalog, catalogError := ParseProviderCatalog(document)
	if catalogError != nil {
		panic(catalogError)
	}
	return catalog
}

func internalTestModelCatalog(offerings ...ProviderOffering) ModelCatalog {
	providers := []CatalogProvider{
		{ID: ProviderNameOpenAI, Label: "OpenAI", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameDeepSeek, Label: "DeepSeek", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameDashScope, Label: "DashScope", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameMoonshot, Label: "Moonshot", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameMiniMax, Label: "MiniMax", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameSiliconFlow, Label: "SiliconFlow", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameZAI, Label: "Z.AI", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameGemini, Label: "Gemini", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameAnthropic, Label: "Anthropic", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameMeta, Label: "Meta", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameXAI, Label: "xAI", CredentialKinds: []string{CatalogCredentialAPIKey}},
	}
	models := []ExactModel{}
	modelIndexes := map[string]int{}
	for offeringIndex := range offerings {
		offering := &offerings[offeringIndex]
		if offering.ProviderModel == "" {
			offering.ProviderModel = offering.Model
		}
		if slices.Contains(offering.Operations, ModelOperationText) {
			internalConfigureTextOffering(offering)
		} else if slices.Contains(offering.Operations, ModelOperationDictation) {
			offering.WireContract = CatalogWireContractMultipartTranscription
			offering.ExecutionLifecycle = string(textExecutionLifecycleSynchronousCompletion)
		}
		modelIndex, found := modelIndexes[offering.Model]
		if !found {
			modelIndex = len(models)
			modelIndexes[offering.Model] = modelIndex
			models = append(models, ExactModel{
				ID: offering.Model, Publisher: "test", Family: "test", Version: offering.Model,
			})
		}
		for _, operation := range offering.Operations {
			if !slices.Contains(models[modelIndex].Operations, operation) {
				models[modelIndex].Operations = append(models[modelIndex].Operations, operation)
			}
		}
		for _, mediaInput := range offering.MediaInputs {
			if !slices.Contains(models[modelIndex].MediaInputs, mediaInput) {
				models[modelIndex].MediaInputs = append(models[modelIndex].MediaInputs, mediaInput)
			}
		}
	}
	prices := make([]CatalogPriceDescriptor, 0)
	for _, offering := range offerings {
		for _, operation := range offering.Operations {
			prices = append(prices, CatalogPriceDescriptor{
				Provider: offering.Provider, Model: offering.Model, Operation: operation,
				Source: "https://example.com/pricing", LastVerified: "2026-08-10", UnavailableReason: "Test price is unavailable.",
			})
		}
	}
	return ModelCatalog{
		Revision: "2026-08-10.test.1",
		Operations: []ModelOperationKind{
			{ID: ModelOperationText, InputArtifacts: []string{CatalogArtifactText, CatalogArtifactImage, CatalogArtifactAudio}, OutputArtifacts: []string{CatalogArtifactText}},
			{ID: ModelOperationDictation, InputArtifacts: []string{CatalogArtifactAudio}, OutputArtifacts: []string{CatalogArtifactText}},
			{ID: ModelOperationVideoGeneration, InputArtifacts: []string{CatalogArtifactText, CatalogArtifactImage}, OutputArtifacts: []string{CatalogArtifactVideo}},
		},
		Providers:  providers,
		Publishers: []ModelPublisher{{ID: "test", Label: "Test"}},
		Families: []ModelFamily{{
			ID: "test", Publisher: "test", Label: "Test", WeightAccess: ModelWeightAccessProprietary,
		}},
		Models:    models,
		Offerings: offerings,
		Prices:    prices,
	}
}

func internalTestProviderCatalog(modelCatalog ModelCatalog) *ProviderCatalog {
	providerLabels := map[string]string{}
	for _, provider := range modelCatalog.Providers {
		providerLabels[provider.ID] = provider.Label
	}
	prices := map[string][]ProviderCatalogPrice{}
	for _, price := range modelCatalog.Prices {
		key := price.Provider + "\x00" + price.Model
		prices[key] = append(prices[key], ProviderCatalogPrice{
			Operation: price.Operation, Available: price.Available, Rates: price.Rates,
			MinimumCharge: price.MinimumCharge, Source: price.Source,
			LastVerified: price.LastVerified, UnavailableReason: price.UnavailableReason,
		})
	}
	providerIndexes := map[string]int{}
	transportIDs := map[string]string{}
	schema := ProviderCatalogSchema{
		SchemaVersion: ProviderCatalogSchemaVersion,
		Operations:    modelCatalog.Operations,
		Publishers:    modelCatalog.Publishers,
		Families:      modelCatalog.Families,
		Models:        modelCatalog.Models,
	}
	empty := ""
	for _, offering := range modelCatalog.Offerings {
		providerIndex, found := providerIndexes[offering.Provider]
		if !found {
			providerIndex = len(schema.Providers)
			providerIndexes[offering.Provider] = providerIndex
			schema.Providers = append(schema.Providers, ProviderCatalogProvider{
				ID: offering.Provider, Label: providerLabels[offering.Provider], KeyAcquisitionURL: "https://provider.example/keys",
				Fields: []ProviderCatalogField{{
					ID: CatalogCredentialAPIKey, Label: "Test API key", Kind: CatalogProviderFieldKindCredential,
					Type: CatalogProviderFieldTypeOpaque, Required: true, Default: &empty, Secret: true,
					Validation: ProviderCatalogFieldValidation{MinimumLength: 1},
				}},
			})
		}
		provider := &schema.Providers[providerIndex]
		transportKey := offering.Provider + "\x00" + offering.WireContract + "\x00" + offering.ExecutionLifecycle
		transportID, transportFound := transportIDs[transportKey]
		if !transportFound {
			transportID = fmt.Sprintf("transport-%d", len(provider.Transports)+1)
			transportIDs[transportKey] = transportID
			transport := internalTestProviderTransport(transportID, offering)
			provider.Transports = append(provider.Transports, transport)
		}
		provider.Offerings = append(provider.Offerings, ProviderCatalogOffering{
			Model: offering.Model, UpstreamModel: offering.ProviderModel, Transport: transportID,
			Operations: offering.Operations, DefaultOperations: offering.DefaultOperations,
			RequestProfile: offering.RequestProfile, WebSearch: offering.WebSearch,
			OutputTokenLimit: offering.OutputTokenLimit, ReasoningEffort: offering.ReasoningEffort,
			MediaInputs: offering.MediaInputs, MediaLimits: offering.MediaLimits,
			Controls: offering.Controls, Limits: offering.Limits,
			Prices: prices[offering.Provider+"\x00"+offering.Model],
		})
	}
	catalog, catalogError := NewProviderCatalog(schema)
	if catalogError != nil {
		panic(catalogError)
	}
	return catalog
}

func newInternalTestProviderRegistry(configuration Configuration) *providerRegistry {
	configuration.ProviderCatalog = internalTestProviderCatalog(configuration.ModelCatalog)
	return newProviderRegistry(configuration)
}

func internalTestProviderTransport(identifier string, offering ProviderOffering) ProviderCatalogTransport {
	protocol := offering.WireContract
	for _, provider := range internalCanonicalProviderCatalog().schema.Providers {
		for _, template := range provider.Transports {
			if template.RequestProtocol != protocol || template.Lifecycle != offering.ExecutionLifecycle {
				continue
			}
			template.ID = identifier
			template.Endpoint = ProviderCatalogEndpoint{
				Method: CatalogEndpointMethodPost, DefaultBaseURL: "https://provider.example", Path: internalTestProviderProtocolPath(protocol),
			}
			return template
		}
	}
	return ProviderCatalogTransport{ID: identifier, RequestProtocol: protocol}
}

func internalTestProviderProtocolPath(protocol string) string {
	switch protocol {
	case CatalogProtocolOpenAIResponses:
		return "/responses"
	case CatalogProtocolOpenAIChatCompletions:
		return "/chat/completions"
	case CatalogProtocolAnthropicMessages:
		return "/v1/messages"
	case CatalogProtocolGeminiInteractions:
		return "/interactions"
	case CatalogProtocolMultipartTranscription:
		return "/audio/transcriptions"
	case CatalogProtocolXAIVideosGenerations:
		return "/videos/generations"
	default:
		return "/unsupported"
	}
}

func internalTestOffering(provider string, model string, operations []string, defaults []string) ProviderOffering {
	offering := ProviderOffering{
		Provider: provider, Model: model, ProviderModel: model,
		Operations: append([]string(nil), operations...), DefaultOperations: append([]string(nil), defaults...),
	}
	if slices.Contains(operations, ModelOperationText) {
		internalConfigureTextOffering(&offering)
	} else if slices.Contains(operations, ModelOperationDictation) {
		offering.WireContract = CatalogWireContractMultipartTranscription
		offering.ExecutionLifecycle = string(textExecutionLifecycleSynchronousCompletion)
	}
	return offering
}

func internalConfigureTextOffering(offering *ProviderOffering) {
	if offering.WireContract == "" {
		switch offering.Provider {
		case ProviderNameOpenAI:
			offering.WireContract = string(textWireContractOpenAIResponses)
			offering.ExecutionLifecycle = string(textExecutionLifecyclePollableResource)
			offering.RequestProfile = string(requestProfileOpenAIResponsesTemperatureTools)
		case ProviderNameGemini:
			offering.WireContract = string(textWireContractGeminiInteractions)
			offering.ExecutionLifecycle = string(textExecutionLifecycleSynchronousCompletion)
		case ProviderNameAnthropic:
			offering.WireContract = string(textWireContractAnthropicMessages)
			offering.ExecutionLifecycle = string(textExecutionLifecycleSynchronousCompletion)
			if offering.OutputTokenLimit == 0 {
				offering.OutputTokenLimit = 1024
			}
		default:
			offering.WireContract = string(textWireContractOpenAIChatCompletions)
			offering.ExecutionLifecycle = string(textExecutionLifecycleSynchronousCompletion)
		}
	}
	if offering.MediaExecutionLifecycle == "" && len(offering.MediaInputs) > 0 {
		offering.MediaExecutionLifecycle = offering.ExecutionLifecycle
		if offering.WireContract == string(textWireContractGeminiInteractions) {
			offering.MediaExecutionLifecycle = string(textExecutionLifecycleSynchronousCompletion)
		}
	}
}
