package proxy

import (
	"errors"
	"testing"
)

func TestManagedRoutingDefaultsRejectNonCanonicalPairs(t *testing.T) {
	const (
		openAITextModel       = "openai-text"
		openAIDictationModel  = "openai-stt"
		deepSeekTextModel     = "deepseek-text"
		siliconDictationModel = "silicon-stt"
	)
	providers := newProviderRegistry(Configuration{
		ModelCatalog: internalTestModelCatalog(
			internalTestOffering(ProviderNameOpenAI, openAITextModel, []string{ModelOperationText}, []string{ModelOperationText}),
			internalTestOffering(ProviderNameOpenAI, openAIDictationModel, []string{ModelOperationDictation}, []string{ModelOperationDictation}),
			internalTestOffering(ProviderNameDeepSeek, deepSeekTextModel, []string{ModelOperationText}, []string{ModelOperationText}),
			internalTestOffering(ProviderNameSiliconFlow, "silicon-text", []string{ModelOperationText}, []string{ModelOperationText}),
			internalTestOffering(ProviderNameSiliconFlow, siliconDictationModel, []string{ModelOperationDictation}, []string{ModelOperationDictation}),
		),
	})
	canonical := TenantDefaults{
		Provider:          ProviderNameOpenAI,
		Model:             openAITextModel,
		DictationProvider: ProviderNameOpenAI,
		DictationModel:    openAIDictationModel,
	}
	providerSettings := map[providerID]managedProviderSettings{
		newProviderID(ProviderNameOpenAI): {apiKey: "sk-openai", textModel: openAITextModel},
	}
	if _, validationError := validatePersistedManagedRoutingDefaults(providers, providerSettings, TenantDefaults{
		Provider:          "OPENAI",
		Model:             openAITextModel,
		DictationProvider: ProviderNameOpenAI,
		DictationModel:    openAIDictationModel,
	}); !errors.Is(validationError, errManagedRoutingDefaultsInvalid) {
		t.Fatalf("non-canonical text error=%v", validationError)
	}
	if _, validationError := validatePersistedManagedRoutingDefaults(providers, providerSettings, TenantDefaults{
		Provider:          ProviderNameOpenAI,
		Model:             openAITextModel,
		DictationProvider: "OPENAI",
		DictationModel:    openAIDictationModel,
	}); !errors.Is(validationError, errManagedRoutingDefaultsInvalid) {
		t.Fatalf("non-canonical dictation error=%v", validationError)
	}
	validated, validationError := validatePersistedManagedRoutingDefaults(providers, providerSettings, canonical)
	if validationError != nil || validated.value() != canonical {
		t.Fatalf("canonical defaults=%+v error=%v", validated.value(), validationError)
	}
	if _, validationError := newManagedRoutingDefaults(providers, TenantDefaults{ReasoningEffort: "high"}); !errors.Is(validationError, errManagedRoutingDefaultsInvalid) {
		t.Fatalf("reasoning without text default error=%v", validationError)
	}
	if _, validationError := validatePersistedManagedRoutingDefaults(providers, map[providerID]managedProviderSettings{
		newProviderID("missing"): {apiKey: "sk-missing", textModel: "missing-model"},
	}, TenantDefaults{}); !errors.Is(validationError, errManagedRoutingDefaultsInvalid) {
		t.Fatalf("unknown keyed provider error=%v", validationError)
	}
	if _, validationError := validatePersistedManagedRoutingDefaults(providers, map[providerID]managedProviderSettings{
		newProviderID(ProviderNameDeepSeek): {apiKey: "sk-deepseek", textModel: deepSeekTextModel},
	}, canonical); !errors.Is(validationError, errManagedRoutingDefaultsInvalid) {
		t.Fatalf("unkeyed default provider error=%v", validationError)
	}

	reconciled, reconciliationError := reconcileManagedRoutingDefaults(providers, map[providerID]managedProviderSettings{
		newProviderID(ProviderNameOpenAI):      {apiKey: "sk-openai", textModel: openAITextModel},
		newProviderID(ProviderNameDeepSeek):    {apiKey: "sk-deepseek", textModel: deepSeekTextModel},
		newProviderID(ProviderNameSiliconFlow): {apiKey: " ", textModel: "silicon-text"},
	}, defaultManagedRoutingDefaults())
	if reconciliationError != nil {
		t.Fatalf("reconcile keyed providers: %v", reconciliationError)
	}
	expectedReconciled := TenantDefaults{
		Provider:          ProviderNameDeepSeek,
		Model:             deepSeekTextModel,
		DictationProvider: ProviderNameOpenAI,
		DictationModel:    openAIDictationModel,
	}
	if reconciled.value() != expectedReconciled {
		t.Fatalf("reconciled defaults=%+v want=%+v", reconciled.value(), expectedReconciled)
	}

	textOnly, reconciliationError := reconcileManagedRoutingDefaults(providers, map[providerID]managedProviderSettings{
		newProviderID(ProviderNameDeepSeek): {apiKey: "sk-deepseek", textModel: deepSeekTextModel},
	}, defaultManagedRoutingDefaults())
	if reconciliationError != nil {
		t.Fatalf("reconcile text-only provider: %v", reconciliationError)
	}
	if textOnly.value() != (TenantDefaults{Provider: ProviderNameDeepSeek, Model: deepSeekTextModel}) {
		t.Fatalf("text-only defaults=%+v", textOnly.value())
	}
}
