package proxy

import (
	"fmt"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

type providerProtocolDefinition struct {
	allowedLifecycles []textExecutionLifecycle
	authentication    ProviderCatalogAuthentication
	headers           []ProviderCatalogHeader
	parameters        providerProtocolParameters
}

type providerProtocolParameters struct {
	ResponsePolicy          string
	ModelField              string
	TokenField              string
	MediaExecutionLifecycle string
	OutputFields            []string
	FinishRules             providerProtocolFinishRules
	ContinuationRules       []string
	ErrorRules              []string
	UsageFields             providerProtocolUsageFields
}

type providerProtocolFinishRules struct {
	Complete []string
	Continue []string
}

type providerProtocolUsageFields struct {
	Input  string
	Output string
	Total  string
}

func providerProtocolDefinitionFor(reference ProviderCatalogProtocolReference, field string) (providerProtocolDefinition, error) {
	bearerAuthentication := ProviderCatalogAuthentication{
		Kind: CatalogAuthenticationBearer, Header: "Authorization", Prefix: "Bearer ",
	}
	definition := providerProtocolDefinition{authentication: bearerAuthentication}
	synchronous := []textExecutionLifecycle{textExecutionLifecycleSynchronousCompletion}
	switch reference.ID {
	case CatalogProtocolDashScopeResponses:
		if reference.Variation != constants.EmptyString {
			return providerProtocolDefinition{}, unsupportedProviderProtocolVariation(reference, field)
		}
		definition.allowedLifecycles = synchronous
		definition.parameters = providerProtocolParameters{
			ModelField: "model", TokenField: "max_output_tokens", MediaExecutionLifecycle: string(textExecutionLifecycleSynchronousCompletion),
			OutputFields:      []string{"output[].content[].text"},
			FinishRules:       providerProtocolFinishRules{Complete: []string{"completed"}, Continue: []string{"incomplete"}},
			ContinuationRules: []string{"append_visible_assistant_output", "request_missing_suffix"},
			ErrorRules:        []string{"cancelled", "failed", "unknown_status"},
			UsageFields:       providerProtocolUsageFields{Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "usage.total_tokens"},
		}
	case CatalogProtocolOpenAIResponses:
		if reference.Variation != constants.EmptyString {
			return providerProtocolDefinition{}, unsupportedProviderProtocolVariation(reference, field)
		}
		definition.allowedLifecycles = []textExecutionLifecycle{textExecutionLifecyclePollableResource}
		definition.parameters = responsesProtocolParameters(textExecutionLifecyclePollableResource)
	case CatalogProtocolXAIResponses:
		if reference.Variation != constants.EmptyString {
			return providerProtocolDefinition{}, unsupportedProviderProtocolVariation(reference, field)
		}
		definition.allowedLifecycles = synchronous
		definition.parameters = responsesProtocolParameters(textExecutionLifecycleSynchronousCompletion)
	case CatalogProtocolOpenAIChatCompletions:
		definition.allowedLifecycles = synchronous
		definition.parameters = chatCompletionsProtocolParameters(reference.Variation)
		if definition.parameters.TokenField == constants.EmptyString {
			return providerProtocolDefinition{}, unsupportedProviderProtocolVariation(reference, field)
		}
	case CatalogProtocolAnthropicMessages:
		if reference.Variation != constants.EmptyString {
			return providerProtocolDefinition{}, unsupportedProviderProtocolVariation(reference, field)
		}
		definition.allowedLifecycles = synchronous
		definition.authentication = ProviderCatalogAuthentication{Kind: CatalogAuthenticationHeader, Header: "x-api-key"}
		definition.headers = []ProviderCatalogHeader{{Name: "anthropic-version", Value: "2023-06-01"}}
		definition.parameters = providerProtocolParameters{
			ModelField: "model", TokenField: "max_tokens", MediaExecutionLifecycle: string(textExecutionLifecycleSynchronousCompletion),
			OutputFields: []string{"content[].text"},
			FinishRules: providerProtocolFinishRules{
				Complete: []string{"end_turn", "stop_sequence"}, Continue: []string{"max_tokens"},
			},
			ContinuationRules: []string{"append_visible_assistant_output", "request_missing_suffix"},
			ErrorRules:        []string{"pause_turn", "refusal", "tool_use", "unknown_stop_reason"},
			UsageFields: providerProtocolUsageFields{
				Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "derived_input_plus_output",
			},
		}
	case CatalogProtocolVertexGenerateContent:
		if reference.Variation != constants.EmptyString {
			return providerProtocolDefinition{}, unsupportedProviderProtocolVariation(reference, field)
		}
		definition.allowedLifecycles = synchronous
		definition.authentication = ProviderCatalogAuthentication{Kind: CatalogAuthenticationHeader, Header: "x-goog-api-key"}
		definition.parameters = providerProtocolParameters{
			ModelField: "path.model", TokenField: "generationConfig.maxOutputTokens", MediaExecutionLifecycle: string(textExecutionLifecycleSynchronousCompletion),
			OutputFields: []string{"candidates[].content.parts[].text"},
			FinishRules:  providerProtocolFinishRules{Complete: []string{"STOP"}},
			ErrorRules:   []string{"MAX_TOKENS", "blocked", "unknown_finish_reason"},
			UsageFields:  providerProtocolUsageFields{Input: "usageMetadata.promptTokenCount", Output: "usageMetadata.candidatesTokenCount+thoughtsTokenCount", Total: "usageMetadata.totalTokenCount"},
		}
	case CatalogProtocolGeminiInteractions:
		if reference.Variation != constants.EmptyString {
			return providerProtocolDefinition{}, unsupportedProviderProtocolVariation(reference, field)
		}
		definition.allowedLifecycles = []textExecutionLifecycle{textExecutionLifecyclePollableResource, textExecutionLifecycleSynchronousCompletion}
		definition.authentication = ProviderCatalogAuthentication{Kind: CatalogAuthenticationHeader, Header: "x-goog-api-key"}
		definition.headers = []ProviderCatalogHeader{{Name: "Api-Revision", Value: "2026-05-20"}}
		definition.parameters = providerProtocolParameters{
			ModelField: "model", TokenField: "generation_config.max_output_tokens", MediaExecutionLifecycle: string(textExecutionLifecycleSynchronousCompletion),
			OutputFields:      []string{"outputs[].text"},
			FinishRules:       providerProtocolFinishRules{Complete: []string{"completed"}, Continue: []string{"incomplete"}},
			ContinuationRules: []string{},
			ErrorRules:        []string{"blocked", "cancelled", "failed", "unknown_status"},
			UsageFields:       providerProtocolUsageFields{Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "usage.total_tokens"},
		}
	case CatalogProtocolMetaTranscription:
		if reference.Variation != constants.EmptyString {
			return providerProtocolDefinition{}, unsupportedProviderProtocolVariation(reference, field)
		}
		definition.allowedLifecycles = synchronous
		definition.parameters = providerProtocolParameters{
			ModelField: "request.model", OutputFields: []string{"transcript"},
			FinishRules: providerProtocolFinishRules{Complete: []string{"http_2xx"}},
			ErrorRules:  []string{"malformed_response", "provider_error"},
		}
	case CatalogProtocolMultipartTranscription:
		definition.allowedLifecycles = synchronous
		modelField := constants.EmptyString
		switch reference.Variation {
		case CatalogProtocolVariationTranscriptionModel:
			modelField = "model"
		case CatalogProtocolVariationTranscriptionModelOmitted:
		default:
			return providerProtocolDefinition{}, unsupportedProviderProtocolVariation(reference, field)
		}
		definition.parameters = providerProtocolParameters{
			ModelField: modelField, OutputFields: []string{"text"},
			FinishRules:       providerProtocolFinishRules{Complete: []string{"http_2xx"}, Continue: []string{}},
			ContinuationRules: []string{}, ErrorRules: []string{"malformed_response", "provider_error"},
			UsageFields: providerProtocolUsageFields{},
		}
	case CatalogProtocolXAIVideosGenerations:
		if reference.Variation != constants.EmptyString {
			return providerProtocolDefinition{}, unsupportedProviderProtocolVariation(reference, field)
		}
		definition.allowedLifecycles = []textExecutionLifecycle{textExecutionLifecyclePollableResource}
		definition.parameters = providerProtocolParameters{
			ModelField: "model", OutputFields: []string{"data[].url"},
			FinishRules:       providerProtocolFinishRules{Complete: []string{"completed"}, Continue: []string{"pending"}},
			ContinuationRules: []string{}, ErrorRules: []string{"failed", "unknown_status"},
			UsageFields: providerProtocolUsageFields{},
		}
	default:
		return providerProtocolDefinition{}, fmt.Errorf("%w: field=%s.id reason=unsupported_protocol protocol=%s", ErrInvalidModelCatalog, field, reference.ID)
	}
	return definition, nil
}

func responsesProtocolParameters(mediaLifecycle textExecutionLifecycle) providerProtocolParameters {
	return providerProtocolParameters{
		ModelField: "model", TokenField: "max_output_tokens", MediaExecutionLifecycle: string(mediaLifecycle),
		OutputFields: []string{"output[].content[].text", "output[].type", "output[].call_id", "output[].name", "output[].arguments"},
		FinishRules: providerProtocolFinishRules{
			Complete: []string{"completed"}, Continue: []string{"incomplete:max_output_tokens"},
		},
		ContinuationRules: []string{"append_visible_assistant_output", "request_missing_suffix"},
		ErrorRules:        []string{"cancelled", "failed", "refusal", "unknown_status"},
		UsageFields:       providerProtocolUsageFields{Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "usage.total_tokens"},
	}
}

func chatCompletionsProtocolParameters(variation string) providerProtocolParameters {
	parameters := providerProtocolParameters{
		ModelField: "model", MediaExecutionLifecycle: string(textExecutionLifecycleSynchronousCompletion),
		OutputFields: []string{"choices[].message.content", "choices[].message.tool_calls"},
		FinishRules: providerProtocolFinishRules{
			Complete: []string{"stop", "tool_calls"}, Continue: []string{"length"},
		},
		ContinuationRules: []string{"append_visible_assistant_output", "request_missing_suffix"},
		ErrorRules:        []string{"content_filter", "unknown_finish_reason"},
		UsageFields:       providerProtocolUsageFields{Input: "usage.prompt_tokens", Output: "usage.completion_tokens", Total: "usage.total_tokens"},
	}
	switch variation {
	case CatalogProtocolVariationMaxTokens:
		parameters.TokenField = string(chatCompletionTokenLimitMaxTokens)
	case CatalogProtocolVariationMaxCompletionTokens:
		parameters.TokenField = string(chatCompletionTokenLimitMaxCompletionTokens)
	case CatalogProtocolVariationQianfanMaxTokens:
		parameters.TokenField = string(chatCompletionTokenLimitMaxTokens)
		parameters.ResponsePolicy = string(chatCompletionResponsePolicyQianfan)
		parameters.OutputFields = []string{"choices[].message.content"}
		parameters.FinishRules.Complete = []string{"stop"}
		parameters.ErrorRules = []string{"content_filter", "tool_calls", "unknown_finish_reason", "blocked_flag"}
	}
	return parameters
}

func unsupportedProviderProtocolVariation(reference ProviderCatalogProtocolReference, field string) error {
	return fmt.Errorf("%w: field=%s.variation reason=unsupported_protocol_variation protocol=%s variation=%s", ErrInvalidModelCatalog, field, reference.ID, reference.Variation)
}
