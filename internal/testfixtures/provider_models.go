package testfixtures

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
)

// ProviderCatalog loads the repository provider catalog for tests.
func ProviderCatalog(testingInstance testing.TB) *proxy.ProviderCatalog {
	testingInstance.Helper()
	_, currentFile, _, callerOK := runtime.Caller(0)
	if !callerOK {
		testingInstance.Fatal("locate test fixture file")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	catalogPath := filepath.Join(repositoryRoot, "configs", "providers.yml")
	document, readError := os.ReadFile(catalogPath)
	if readError != nil {
		testingInstance.Fatalf("read provider catalog: %v", readError)
	}
	catalog, parseError := proxy.ParseProviderCatalog(document)
	if parseError != nil {
		testingInstance.Fatalf("parse provider catalog: %v", parseError)
	}
	return catalog
}

// ProviderCatalogWithResourceVisibilityInterval returns a catalog with one provider's pollable text interval changed for timing-independent lifecycle tests.
func ProviderCatalogWithResourceVisibilityInterval(testingInstance testing.TB, providerIdentifier string, retryIntervalMilliseconds int) *proxy.ProviderCatalog {
	testingInstance.Helper()
	schema := ProviderCatalog(testingInstance).Schema()
	matchCount := 0
	for providerIndex := range schema.Providers {
		provider := &schema.Providers[providerIndex]
		if provider.ID != providerIdentifier {
			continue
		}
		for transportIndex := range provider.Transports {
			transport := &provider.Transports[transportIndex]
			if transport.ResourceVisibility.RetryLimit == 0 {
				continue
			}
			transport.ResourceVisibility.RetryIntervalMilliseconds = retryIntervalMilliseconds
			matchCount++
		}
	}
	if matchCount != 1 {
		testingInstance.Fatalf("provider %s pollable text transports=%d want=1", providerIdentifier, matchCount)
	}
	catalog, catalogError := proxy.NewProviderCatalog(schema)
	if catalogError != nil {
		testingInstance.Fatalf("compile provider catalog with test visibility interval: %v", catalogError)
	}
	return catalog
}

// ModelCatalog returns the normalized projection used by catalog assertions.
func ModelCatalog(testingInstance testing.TB) proxy.ModelCatalog {
	testingInstance.Helper()
	return ProviderCatalog(testingInstance).ModelCatalog()
}

// WithModelCatalog returns a configuration with the explicit provider catalog.
func WithModelCatalog(testingInstance testing.TB, configuration proxy.Configuration) proxy.Configuration {
	testingInstance.Helper()
	if configuration.ProviderCatalog == nil {
		configuration.ProviderCatalog = ProviderCatalog(testingInstance)
	}
	return configuration
}

// ProviderCatalogFromModelCatalog compiles a strict test catalog from a normalized projection.
func ProviderCatalogFromModelCatalog(testingInstance testing.TB, modelCatalog proxy.ModelCatalog) *proxy.ProviderCatalog {
	testingInstance.Helper()
	catalog, catalogError := NewProviderCatalogFromModelCatalog(modelCatalog)
	if catalogError != nil {
		testingInstance.Fatalf("compile test provider catalog: %v", catalogError)
	}
	return catalog
}

// NewProviderCatalogFromModelCatalog returns strict construction errors to rejection tests.
func NewProviderCatalogFromModelCatalog(modelCatalog proxy.ModelCatalog) (*proxy.ProviderCatalog, error) {
	providerLabels := map[string]string{}
	for _, provider := range modelCatalog.Providers {
		providerLabels[provider.ID] = provider.Label
	}
	prices := map[string][]proxy.ProviderCatalogPrice{}
	for _, price := range modelCatalog.Prices {
		key := price.Provider + "\x00" + price.Model
		prices[key] = append(prices[key], proxy.ProviderCatalogPrice{
			Operation: price.Operation, Available: price.Available, Rates: price.Rates,
			MinimumCharge: price.MinimumCharge, Source: price.Source,
			LastVerified: price.LastVerified, UnavailableReason: price.UnavailableReason,
		})
	}
	providerIndexes := map[string]int{}
	transportIDs := map[string]string{}
	schema := proxy.ProviderCatalogSchema{
		SchemaVersion: proxy.ProviderCatalogSchemaVersion,
		Operations:    modelCatalog.Operations, Publishers: modelCatalog.Publishers,
		Families: modelCatalog.Families, Models: modelCatalog.Models,
	}
	empty := ""
	for _, offering := range modelCatalog.Offerings {
		providerIndex, found := providerIndexes[offering.Provider]
		if !found {
			providerIndex = len(schema.Providers)
			providerIndexes[offering.Provider] = providerIndex
			schema.Providers = append(schema.Providers, proxy.ProviderCatalogProvider{
				ID: offering.Provider, Label: providerLabels[offering.Provider], KeyAcquisitionURL: "https://provider.example/keys",
				Fields: []proxy.ProviderCatalogField{{
					ID: proxy.CatalogCredentialAPIKey, Label: "Test API key", Kind: proxy.CatalogProviderFieldKindCredential,
					Type: proxy.CatalogProviderFieldTypeOpaque, Required: true, Default: &empty, Secret: true,
					Validation: proxy.ProviderCatalogFieldValidation{MinimumLength: 1},
				}},
			})
		}
		provider := &schema.Providers[providerIndex]
		transportKey := offering.Provider + "\x00" + offering.WireContract + "\x00" + offering.ExecutionLifecycle
		transportID, transportFound := transportIDs[transportKey]
		if !transportFound {
			transportID = fmt.Sprintf("transport-%d", len(provider.Transports)+1)
			transportIDs[transportKey] = transportID
			provider.Transports = append(provider.Transports, testProviderTransport(transportID, offering))
		}
		provider.Offerings = append(provider.Offerings, proxy.ProviderCatalogOffering{
			Model: offering.Model, UpstreamModel: offering.ProviderModel, Transport: transportID,
			Operations: offering.Operations, DefaultOperations: offering.DefaultOperations,
			RequestProfile: offering.RequestProfile, WebSearch: offering.WebSearch,
			OutputTokenLimit: offering.OutputTokenLimit, ReasoningEffort: offering.ReasoningEffort,
			MediaInputs: offering.MediaInputs, MediaLimits: offering.MediaLimits,
			Controls: offering.Controls, Limits: offering.Limits,
			Prices: prices[offering.Provider+"\x00"+offering.Model],
		})
	}
	return proxy.NewProviderCatalog(schema)
}

func testProviderTransport(identifier string, offering proxy.ProviderOffering) proxy.ProviderCatalogTransport {
	parameters := testProviderProtocolParameters(offering)
	transport := proxy.ProviderCatalogTransport{
		ID:              identifier,
		Endpoint:        proxy.ProviderCatalogEndpoint{Method: proxy.CatalogEndpointMethodPost, DefaultBaseURL: "https://provider.example", Path: testProviderProtocolPath(offering.WireContract)},
		Authentication:  proxy.ProviderCatalogAuthentication{Kind: proxy.CatalogAuthenticationBearer, Field: proxy.CatalogCredentialAPIKey, Header: "Authorization", Prefix: "Bearer "},
		RequestProtocol: offering.WireContract, ResponseProtocol: offering.WireContract,
		UsageMapping: offering.WireContract, Lifecycle: offering.ExecutionLifecycle,
		ProtocolParameters: parameters,
	}
	if offering.ExecutionLifecycle == "pollable_resource" {
		switch offering.Provider {
		case proxy.ProviderNameOpenAI:
			transport.ResourceVisibility = proxy.ProviderCatalogResourceVisibility{
				RetryIntervalMilliseconds: 2000,
				RetryLimit:                1,
				RetryStatusCodes:          []int{403, 404},
			}
		case proxy.ProviderNameGemini:
			transport.ResourceVisibility = proxy.ProviderCatalogResourceVisibility{
				RetryIntervalMilliseconds: 5000,
				RetryLimit:                6,
				RetryStatusCodes:          []int{400, 403, 404},
			}
		}
	}
	if offering.WireContract == proxy.CatalogProtocolGeminiInteractions {
		transport.Authentication = proxy.ProviderCatalogAuthentication{Kind: proxy.CatalogAuthenticationHeader, Field: proxy.CatalogCredentialAPIKey, Header: "x-goog-api-key"}
		transport.Headers = []proxy.ProviderCatalogHeader{{Name: "Api-Revision", Value: "2026-05-20"}}
	}
	if offering.WireContract == proxy.CatalogProtocolAnthropicMessages {
		transport.Authentication = proxy.ProviderCatalogAuthentication{Kind: proxy.CatalogAuthenticationHeader, Field: proxy.CatalogCredentialAPIKey, Header: "x-api-key"}
		transport.Headers = []proxy.ProviderCatalogHeader{{Name: "anthropic-version", Value: "2023-06-01"}}
	}
	return transport
}

func testProviderProtocolParameters(offering proxy.ProviderOffering) proxy.ProviderCatalogProtocolParameters {
	switch offering.WireContract {
	case proxy.CatalogProtocolOpenAIResponses:
		return proxy.ProviderCatalogProtocolParameters{
			ModelField: "model", TokenField: "max_output_tokens", MediaExecutionLifecycle: offering.ExecutionLifecycle,
			OutputFields:      []string{"output[].content[].text"},
			FinishRules:       proxy.ProviderCatalogFinishRules{Complete: []string{"completed"}, Continue: []string{"incomplete:max_output_tokens"}},
			ContinuationRules: []string{"append_visible_assistant_output", "request_missing_suffix"},
			ErrorRules:        []string{"cancelled", "failed", "refusal", "unknown_status"},
			UsageFields:       proxy.ProviderCatalogUsageFields{Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "usage.total_tokens"},
		}
	case proxy.CatalogProtocolOpenAIChatCompletions:
		return proxy.ProviderCatalogProtocolParameters{
			ModelField: "model", TokenField: "max_tokens", MediaExecutionLifecycle: "synchronous_completion",
			OutputFields:      []string{"choices[].message.content"},
			FinishRules:       proxy.ProviderCatalogFinishRules{Complete: []string{"stop"}, Continue: []string{"length"}},
			ContinuationRules: []string{"append_visible_assistant_output", "request_missing_suffix"},
			ErrorRules:        []string{"content_filter", "tool_calls", "unknown_finish_reason"},
			UsageFields:       proxy.ProviderCatalogUsageFields{Input: "usage.prompt_tokens", Output: "usage.completion_tokens", Total: "usage.total_tokens"},
		}
	case proxy.CatalogProtocolAnthropicMessages:
		return proxy.ProviderCatalogProtocolParameters{
			ModelField: "model", TokenField: "max_tokens", MediaExecutionLifecycle: "synchronous_completion",
			OutputFields:      []string{"content[].text"},
			FinishRules:       proxy.ProviderCatalogFinishRules{Complete: []string{"end_turn", "stop_sequence"}, Continue: []string{"max_tokens"}},
			ContinuationRules: []string{"append_visible_assistant_output", "request_missing_suffix"},
			ErrorRules:        []string{"pause_turn", "refusal", "tool_use", "unknown_stop_reason"},
			UsageFields:       proxy.ProviderCatalogUsageFields{Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "derived_input_plus_output"},
		}
	case proxy.CatalogProtocolGeminiInteractions:
		return proxy.ProviderCatalogProtocolParameters{
			ModelField: "model", TokenField: "generation_config.max_output_tokens", MediaExecutionLifecycle: "synchronous_completion",
			OutputFields:      []string{"outputs[].text"},
			FinishRules:       proxy.ProviderCatalogFinishRules{Complete: []string{"completed"}, Continue: []string{"incomplete"}},
			ContinuationRules: []string{},
			ErrorRules:        []string{"blocked", "cancelled", "failed", "unknown_status"},
			UsageFields:       proxy.ProviderCatalogUsageFields{Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "usage.total_tokens"},
		}
	case proxy.CatalogProtocolMultipartTranscription:
		return proxy.ProviderCatalogProtocolParameters{
			ModelField: "model", OutputFields: []string{"text"},
			FinishRules:       proxy.ProviderCatalogFinishRules{Complete: []string{"http_2xx"}, Continue: []string{}},
			ContinuationRules: []string{}, ErrorRules: []string{"malformed_response", "provider_error"},
		}
	case proxy.CatalogProtocolXAIVideosGenerations:
		return proxy.ProviderCatalogProtocolParameters{
			ModelField: "model", OutputFields: []string{"data[].url"},
			FinishRules:       proxy.ProviderCatalogFinishRules{Complete: []string{"completed"}, Continue: []string{"pending"}},
			ContinuationRules: []string{}, ErrorRules: []string{"failed", "unknown_status"},
		}
	default:
		return proxy.ProviderCatalogProtocolParameters{}
	}
}

func testProviderProtocolPath(protocol string) string {
	switch protocol {
	case proxy.CatalogProtocolOpenAIResponses:
		return "/responses"
	case proxy.CatalogProtocolOpenAIChatCompletions:
		return "/chat/completions"
	case proxy.CatalogProtocolAnthropicMessages:
		return "/v1/messages"
	case proxy.CatalogProtocolGeminiInteractions:
		return "/interactions"
	case proxy.CatalogProtocolMultipartTranscription:
		return "/audio/transcriptions"
	case proxy.CatalogProtocolXAIVideosGenerations:
		return "/videos/generations"
	default:
		return "/unsupported"
	}
}
