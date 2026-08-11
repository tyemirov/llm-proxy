package proxy

import "slices"

func internalTestModelCatalog(offerings ...ProviderOffering) ModelCatalog {
	providers := []CatalogProvider{
		{ID: ProviderNameOpenAI, Label: "OpenAI"},
		{ID: ProviderNameDeepSeek, Label: "DeepSeek"},
		{ID: ProviderNameDashScope, Label: "DashScope"},
		{ID: ProviderNameMoonshot, Label: "Moonshot"},
		{ID: ProviderNameMiniMax, Label: "MiniMax"},
		{ID: ProviderNameSiliconFlow, Label: "SiliconFlow"},
		{ID: ProviderNameZhipu, Label: "Zhipu"},
		{ID: ProviderNameGemini, Label: "Gemini"},
		{ID: ProviderNameAnthropic, Label: "Anthropic"},
		{ID: ProviderNameMeta, Label: "Meta"},
		{ID: ProviderNameGrok, Label: "Grok"},
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
	return ModelCatalog{
		Providers:  providers,
		Publishers: []ModelPublisher{{ID: "test", Label: "Test"}},
		Families:   []ModelFamily{{ID: "test", Publisher: "test", Label: "Test"}},
		Models:     models,
		Offerings:  offerings,
	}
}

func internalTestOffering(provider string, model string, operations []string, defaults []string) ProviderOffering {
	offering := ProviderOffering{
		Provider: provider, Model: model, ProviderModel: model,
		Operations: append([]string(nil), operations...), DefaultOperations: append([]string(nil), defaults...),
	}
	if slices.Contains(operations, ModelOperationText) {
		internalConfigureTextOffering(&offering)
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
