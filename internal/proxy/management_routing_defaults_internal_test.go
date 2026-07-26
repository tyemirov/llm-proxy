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
		ProviderModels: ProviderModelCatalogs{
			ProviderNameOpenAI: {
				Text: ModelEndpointCatalog{
					DefaultModel: openAITextModel,
					Models:       []ModelConfiguration{{ID: openAITextModel}},
				},
				Dictation: ModelEndpointCatalog{
					DefaultModel: openAIDictationModel,
					Models:       []ModelConfiguration{{ID: openAIDictationModel}},
				},
			},
			ProviderNameDeepSeek: {
				Text: ModelEndpointCatalog{
					DefaultModel: deepSeekTextModel,
					Models:       []ModelConfiguration{{ID: deepSeekTextModel}},
				},
			},
			ProviderNameSiliconFlow: {
				Text: ModelEndpointCatalog{
					DefaultModel: "silicon-text",
					Models:       []ModelConfiguration{{ID: "silicon-text"}},
				},
				Dictation: ModelEndpointCatalog{
					DefaultModel: siliconDictationModel,
					Models:       []ModelConfiguration{{ID: siliconDictationModel}},
				},
			},
		},
	})
	canonical := TenantDefaults{
		Provider:          ProviderNameOpenAI,
		Model:             openAITextModel,
		DictationProvider: ProviderNameOpenAI,
		DictationModel:    openAIDictationModel,
	}
	if _, validationError := validatePersistedManagedRoutingDefaults(providers, TenantDefaults{
		Provider:          "OPENAI",
		Model:             openAITextModel,
		DictationProvider: ProviderNameOpenAI,
		DictationModel:    openAIDictationModel,
	}); !errors.Is(validationError, errManagedRoutingDefaultsInvalid) {
		t.Fatalf("non-canonical text error=%v", validationError)
	}
	if _, validationError := validatePersistedManagedRoutingDefaults(providers, TenantDefaults{
		Provider:          ProviderNameOpenAI,
		Model:             openAITextModel,
		DictationProvider: "OPENAI",
		DictationModel:    openAIDictationModel,
	}); !errors.Is(validationError, errManagedRoutingDefaultsInvalid) {
		t.Fatalf("non-canonical dictation error=%v", validationError)
	}
	validated, validationError := validatePersistedManagedRoutingDefaults(providers, canonical)
	if validationError != nil || validated.value() != canonical {
		t.Fatalf("canonical defaults=%+v error=%v", validated.value(), validationError)
	}
}
