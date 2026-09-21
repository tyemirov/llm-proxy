package proxy

import (
	"slices"
	"testing"
)

func TestPublicCapabilityDomainForOperation(t *testing.T) {
	testCases := []struct {
		operation string
		domain    string
	}{
		{"text", PublicCapabilityDomainText},
		{"dictation", PublicCapabilityDomainTranscription},
		{"audio_transcription", PublicCapabilityDomainTranscription},
		{"audio_diarization", PublicCapabilityDomainTranscription},
		{"audio_alignment", PublicCapabilityDomainTranscription},
		{"subtitle_creation", PublicCapabilityDomainTranscription},
		{"speech_generation", PublicCapabilityDomainSpeech},
		{"voice_extraction", PublicCapabilityDomainSpeech},
		{"image_input", PublicCapabilityDomainImage},
		{"video_generation", PublicCapabilityDomainVideo},
		{"web_search", ""},
		{"unknown_operation", ""},
	}
	for _, testCase := range testCases {
		if domain := PublicCapabilityDomainForOperation(testCase.operation); domain != testCase.domain {
			t.Fatalf("operation=%s domain=%q want=%q", testCase.operation, domain, testCase.domain)
		}
	}
	if domains := PublicCapabilityDomainsForOperations([]string{"speech_generation", "audio_transcription", "text", "web_search"}); !slices.Equal(domains, []string{"speech", "text", "transcription"}) {
		t.Fatalf("domains=%v", domains)
	}
}

func TestPublicCapabilityCatalogProjectsGranularDictatorDomains(t *testing.T) {
	catalog, catalogError := NewPublicCapabilityCatalog(Configuration{ProviderCatalog: internalCanonicalProviderCatalog()})
	if catalogError != nil {
		t.Fatal(catalogError)
	}
	modelDomains := map[string][]string{}
	for _, model := range catalog.Models {
		modelDomains[model.Identifier] = model.Domains
	}
	for _, unexpected := range []string{"dictator-speech-v1"} {
		if _, found := modelDomains[unexpected]; found {
			t.Fatalf("umbrella model %s still projected", unexpected)
		}
	}
	expectedModels := map[string][]string{
		"whisper-base":     {"speech", "transcription"},
		"whisper-large-v3": {"speech", "transcription"},
		"qwen3-tts":        {"speech"},
		"silero-ru":        {"speech"},
		"gpt-transcribe":   {"transcription"},
		"gpt-4.1":          {"image", "text"},
	}
	for model, expected := range expectedModels {
		if domains := modelDomains[model]; !slices.Equal(domains, expected) {
			t.Fatalf("model=%s domains=%v want=%v", model, domains, expected)
		}
	}
	offeringDomains := map[string][]string{}
	for _, offering := range catalog.Offerings {
		if offering.Provider == ProviderNameDictator || offering.Model == "gpt-4.1" {
			offeringDomains[offering.Model] = offering.Domains
		}
	}
	for model, expected := range expectedModels {
		if model == "gpt-transcribe" {
			continue
		}
		if domains := offeringDomains[model]; !slices.Equal(domains, expected) {
			t.Fatalf("offering=%s domains=%v want=%v", model, domains, expected)
		}
	}
}
