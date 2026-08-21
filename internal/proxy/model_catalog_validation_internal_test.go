package proxy

import (
	"strings"
	"testing"
)

func TestModelCatalogValidationRejectsEveryIdentityBoundary(t *testing.T) {
	testCases := []struct {
		name     string
		mutate   func(*ModelCatalog)
		expected string
	}{
		{name: "providers missing", mutate: func(catalog *ModelCatalog) { catalog.Providers = nil }, expected: "catalog.providers"},
		{name: "provider identifier", mutate: func(catalog *ModelCatalog) { catalog.Providers[0].ID = "OpenAI" }, expected: "reason=not_canonical"},
		{name: "provider duplicate", mutate: func(catalog *ModelCatalog) { catalog.Providers[1].ID = catalog.Providers[0].ID }, expected: "duplicate_identifier="},
		{name: "provider label", mutate: func(catalog *ModelCatalog) { catalog.Providers[0].Label = " " }, expected: ".label"},
		{name: "provider credentials", mutate: func(catalog *ModelCatalog) { catalog.Providers[0].CredentialKinds = nil }, expected: ".credential_kinds"},
		{name: "publishers missing", mutate: func(catalog *ModelCatalog) { catalog.Publishers = nil }, expected: "catalog.publishers"},
		{name: "publisher identifier", mutate: func(catalog *ModelCatalog) { catalog.Publishers[0].ID = "Test" }, expected: "reason=not_canonical"},
		{name: "publisher duplicate", mutate: func(catalog *ModelCatalog) { catalog.Publishers = append(catalog.Publishers, catalog.Publishers[0]) }, expected: "duplicate_identifier="},
		{name: "publisher label", mutate: func(catalog *ModelCatalog) { catalog.Publishers[0].Label = " Test" }, expected: ".label"},
		{name: "families missing", mutate: func(catalog *ModelCatalog) { catalog.Families = nil }, expected: "catalog.families"},
		{name: "family identifier", mutate: func(catalog *ModelCatalog) { catalog.Families[0].ID = "Test" }, expected: "reason=not_canonical"},
		{name: "family duplicate", mutate: func(catalog *ModelCatalog) { catalog.Families = append(catalog.Families, catalog.Families[0]) }, expected: "duplicate_identifier="},
		{name: "family publisher", mutate: func(catalog *ModelCatalog) { catalog.Families[0].Publisher = "missing" }, expected: "reason=dangling_reference"},
		{name: "family label", mutate: func(catalog *ModelCatalog) { catalog.Families[0].Label = " " }, expected: ".label"},
		{name: "family weight access", mutate: func(catalog *ModelCatalog) { catalog.Families[0].WeightAccess = "future" }, expected: ".weight_access"},
		{name: "models missing", mutate: func(catalog *ModelCatalog) { catalog.Models = nil }, expected: "catalog.models"},
		{name: "model identifier", mutate: func(catalog *ModelCatalog) { catalog.Models[0].ID = "Model One" }, expected: "reason=not_canonical"},
		{name: "model duplicate", mutate: func(catalog *ModelCatalog) { catalog.Models = append(catalog.Models, catalog.Models[0]) }, expected: "duplicate_identifier="},
		{name: "model publisher", mutate: func(catalog *ModelCatalog) { catalog.Models[0].Publisher = "missing" }, expected: "reason=dangling_reference"},
		{name: "model family", mutate: func(catalog *ModelCatalog) { catalog.Models[0].Family = "missing" }, expected: "reason=dangling_reference"},
		{name: "model version", mutate: func(catalog *ModelCatalog) { catalog.Models[0].Version = " " }, expected: ".version"},
		{name: "model operations", mutate: func(catalog *ModelCatalog) { catalog.Models[0].Operations = nil }, expected: ".operations"},
		{name: "model media", mutate: func(catalog *ModelCatalog) {
			catalog.Models[0].MediaInputs = []string{CatalogArtifactImage, CatalogArtifactImage}
		}, expected: "duplicate=image"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			catalog := internalSingleTextModelCatalog()
			testCase.mutate(&catalog)
			_, catalogError := NewCatalogService(catalog)
			assertInvalidProviderCatalogError(t, catalogError, testCase.expected)
		})
	}
}

func TestModelCatalogValidationRejectsEveryOfferingBoundary(t *testing.T) {
	testCases := []struct {
		name     string
		mutate   func(*ModelCatalog)
		expected string
	}{
		{name: "offerings missing", mutate: func(catalog *ModelCatalog) { catalog.Offerings = nil }, expected: "catalog.offerings"},
		{name: "provider reference", mutate: func(catalog *ModelCatalog) { catalog.Offerings[0].Provider = "missing" }, expected: "reason=dangling_reference"},
		{name: "model reference", mutate: func(catalog *ModelCatalog) { catalog.Offerings[0].Model = "missing" }, expected: "reason=dangling_reference"},
		{name: "provider model", mutate: func(catalog *ModelCatalog) { catalog.Offerings[0].ProviderModel = " " }, expected: ".provider_model"},
		{name: "route conflict", mutate: func(catalog *ModelCatalog) { catalog.Offerings = append(catalog.Offerings, catalog.Offerings[0]) }, expected: "route_conflict="},
		{name: "provider native model conflict", mutate: func(catalog *ModelCatalog) {
			model := catalog.Models[0]
			model.ID = "second-model"
			model.Version = "second-model"
			catalog.Models = append(catalog.Models, model)
			offering := catalog.Offerings[0]
			offering.Model = model.ID
			offering.DefaultOperations = nil
			catalog.Offerings = append(catalog.Offerings, offering)
		}, expected: "provider_native_model_conflict="},
		{name: "operations", mutate: func(catalog *ModelCatalog) { catalog.Offerings[0].Operations = nil }, expected: ".operations"},
		{name: "operation unsupported by model", mutate: func(catalog *ModelCatalog) {
			catalog.Offerings[0].Operations = append(catalog.Offerings[0].Operations, ModelOperationDictation)
		}, expected: "unsupported_by_model"},
		{name: "default operation invalid", mutate: func(catalog *ModelCatalog) { catalog.Offerings[0].DefaultOperations = []string{"future"} }, expected: "operation=future"},
		{name: "default operation unsupported", mutate: func(catalog *ModelCatalog) {
			catalog.Offerings[0].DefaultOperations = []string{ModelOperationDictation}
		}, expected: "unsupported_by_offering"},
		{name: "output token limit", mutate: func(catalog *ModelCatalog) { catalog.Offerings[0].OutputTokenLimit = -1 }, expected: ".output_token_limit"},
		{name: "media unsupported by model", mutate: func(catalog *ModelCatalog) { catalog.Offerings[0].MediaInputs = []string{CatalogArtifactImage} }, expected: "unsupported_by_model"},
		{name: "media duplicate", mutate: func(catalog *ModelCatalog) {
			catalog.Models[0].MediaInputs = []string{CatalogArtifactImage}
			catalog.Offerings[0].MediaInputs = []string{CatalogArtifactImage, CatalogArtifactImage}
		}, expected: "duplicate=image"},
		{name: "media limit", mutate: func(catalog *ModelCatalog) {
			catalog.Models[0].MediaInputs = []string{CatalogArtifactImage}
			catalog.Offerings[0].MediaInputs = []string{CatalogArtifactImage}
			catalog.Offerings[0].MediaLimits = []CatalogMediaLimit{{ID: "future", MediaType: CatalogArtifactImage, Transport: CatalogMediaTransportInline, Status: CatalogMediaLimitStatusBounded}}
		}, expected: ".media_limits"},
		{name: "exact model without offering", mutate: func(catalog *ModelCatalog) {
			model := catalog.Models[0]
			model.ID = "unoffered-model"
			model.Version = "unoffered-model"
			catalog.Models = append(catalog.Models, model)
		}, expected: "missing_provider_offering"},
		{name: "price operation", mutate: func(catalog *ModelCatalog) { catalog.Prices[0].Operation = ModelOperationDictation }, expected: "dangling_reference"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			catalog := internalSingleTextModelCatalog()
			testCase.mutate(&catalog)
			_, catalogError := NewCatalogService(catalog)
			assertInvalidProviderCatalogError(t, catalogError, testCase.expected)
		})
	}
}

func TestTextOfferingValidationRejectsEveryAdapterBoundary(t *testing.T) {
	baseOffering := internalTestOffering(ProviderNameOpenAI, ModelNameGPT41, []string{ModelOperationText}, []string{ModelOperationText})
	testCases := []struct {
		name     string
		mutate   func(*ProviderOffering)
		expected string
	}{
		{name: "wire contract shape", mutate: func(offering *ProviderOffering) { offering.WireContract = " " }, expected: ".wire_contract"},
		{name: "lifecycle shape", mutate: func(offering *ProviderOffering) { offering.ExecutionLifecycle = " " }, expected: ".execution_lifecycle"},
		{name: "unknown wire contract", mutate: func(offering *ProviderOffering) { offering.WireContract = "future" }, expected: "wire_contract=future"},
		{name: "unknown lifecycle", mutate: func(offering *ProviderOffering) { offering.ExecutionLifecycle = "future" }, expected: "execution_lifecycle=future"},
		{name: "adapter pair", mutate: func(offering *ProviderOffering) {
			offering.WireContract = string(textWireContractAnthropicMessages)
			offering.ExecutionLifecycle = string(textExecutionLifecyclePollableResource)
			offering.RequestProfile = ""
		}, expected: "wire_contract=anthropic_messages"},
		{name: "web search", mutate: func(offering *ProviderOffering) {
			offering.WireContract = string(textWireContractOpenAIChatCompletions)
			offering.ExecutionLifecycle = string(textExecutionLifecycleSynchronousCompletion)
			offering.RequestProfile = ""
			offering.WebSearch = true
		}, expected: ".web_search"},
		{name: "anthropic output limit", mutate: func(offering *ProviderOffering) {
			offering.WireContract = string(textWireContractAnthropicMessages)
			offering.ExecutionLifecycle = string(textExecutionLifecycleSynchronousCompletion)
			offering.RequestProfile = ""
			offering.OutputTokenLimit = 0
		}, expected: ".output_token_limit"},
		{name: "request profile", mutate: func(offering *ProviderOffering) { offering.RequestProfile = "future" }, expected: ".request_profile"},
		{name: "reasoning capability", mutate: func(offering *ProviderOffering) {
			offering.ReasoningEffort = &ReasoningEffortCapability{Adapter: "future", Efforts: []string{"high"}}
		}, expected: ".reasoning_effort.adapter"},
		{name: "responses reasoning adapter", mutate: func(offering *ProviderOffering) {
			offering.ReasoningEffort = &ReasoningEffortCapability{Adapter: string(reasoningEffortAdapterOpenAIResponses), Efforts: []string{"high"}}
		}, expected: "adapter=openai_responses"},
		{name: "chat reasoning adapter", mutate: func(offering *ProviderOffering) {
			offering.ReasoningEffort = &ReasoningEffortCapability{Adapter: string(reasoningEffortAdapterOpenAIChatCompletions), Efforts: []string{"high"}}
		}, expected: "adapter=openai_chat_completions"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			offering := baseOffering
			testCase.mutate(&offering)
			assertInvalidProviderCatalogError(t, validateTextOffering(offering, "offering"), testCase.expected)
		})
	}
}

func TestModelCatalogOperationAndMediaSetEdges(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		operations []string
		expected   string
	}{
		{name: "missing", operations: nil, expected: "field=operations"},
		{name: "unknown", operations: []string{"future"}, expected: "operation=future"},
		{name: "duplicate", operations: []string{ModelOperationText, ModelOperationText}, expected: "duplicate=text"},
	} {
		t.Run("operation "+testCase.name, func(t *testing.T) {
			_, operationError := validatedOperationSet(testCase.operations, "operations")
			assertInvalidProviderCatalogError(t, operationError, testCase.expected)
		})
	}
	if _, mediaError := validatedExactModelMediaInputs([]string{"future"}, "media"); mediaError == nil {
		t.Fatal("unknown exact-model media input was accepted")
	}
	if _, mediaError := validatedExactModelMediaInputs([]string{CatalogArtifactImage, CatalogArtifactImage}, "media"); mediaError == nil {
		t.Fatal("duplicate exact-model media input was accepted")
	}
	if knownTextWireContract("future") || knownTextExecutionLifecycle("future") {
		t.Fatal("unknown text route capability was accepted")
	}
	if strings.TrimSpace(modelRequestProfile("profile").string()) != "profile" {
		t.Fatal("request profile string projection changed")
	}
}

func internalSingleTextModelCatalog() ModelCatalog {
	return internalTestModelCatalog(internalTestOffering(
		ProviderNameOpenAI,
		ModelNameGPT41,
		[]string{ModelOperationText},
		[]string{ModelOperationText},
	))
}
