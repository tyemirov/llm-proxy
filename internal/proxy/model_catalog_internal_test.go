package proxy

import "slices"

func internalTestModelCatalog(offerings ...ProviderOffering) ModelCatalog {
	providers := []CatalogProvider{
		{ID: ProviderNameOpenAI, Label: "OpenAI", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameDeepSeek, Label: "DeepSeek", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameDashScope, Label: "DashScope", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameMoonshot, Label: "Moonshot", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameMiniMax, Label: "MiniMax", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameSiliconFlow, Label: "SiliconFlow", CredentialKinds: []string{CatalogCredentialAPIKey}},
		{ID: ProviderNameZhipu, Label: "Zhipu", CredentialKinds: []string{CatalogCredentialAPIKey}},
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
		Families:   []ModelFamily{{ID: "test", Publisher: "test", Label: "Test"}},
		Models:     models,
		Offerings:  offerings,
		Prices:     prices,
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
	if offering.WireContract != "" {
		return
	}
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
