package proxy

import (
	"fmt"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

type providerRequestCodecDefinition struct {
	modelField              string
	tokenField              string
	mediaExecutionLifecycle textExecutionLifecycle
	requiredHeaders         []ProviderCatalogHeader
}

type providerResponseCodecDefinition struct {
	responsePolicy    string
	outputFields      []string
	finishRules       providerProtocolFinishRules
	continuationRules []string
	errorRules        []string
	usageFields       providerProtocolUsageFields
}

type providerTransportComposition struct {
	requestCodec       string
	responseCodec      string
	authentication     ProviderCatalogAuthentication
	lifecycle          textExecutionLifecycle
	resourceVisibility pollableResourceVisibilityPolicy
	parameters         providerProtocolParameters
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

func composeProviderTransport(transport ProviderCatalogTransport, field string) (providerTransportComposition, error) {
	request, requestError := providerRequestCodecDefinitionFor(transport.Components.RequestCodec, field+".components.request_codec")
	if requestError != nil {
		return providerTransportComposition{}, requestError
	}
	response, responseError := providerResponseCodecDefinitionFor(transport.Components.ResponseCodec, field+".components.response_codec")
	if responseError != nil {
		return providerTransportComposition{}, responseError
	}
	if transport.Components.RequestCodec.ID != transport.Components.ResponseCodec.ID {
		return providerTransportComposition{}, unsupportedTransportComponentCombination(transport, field, "codec_pair")
	}
	if authenticationError := validateProviderCatalogAuthentication(transport.Components.Authentication, field+".components.authentication"); authenticationError != nil {
		return providerTransportComposition{}, authenticationError
	}
	if !providerCatalogHeadersEqual(transport.Headers, request.requiredHeaders) {
		return providerTransportComposition{}, unsupportedTransportComponentCombination(transport, field, "required_headers")
	}
	lifecycle := textExecutionLifecycle(transport.Components.Execution.ID)
	if !knownTextExecutionLifecycle(lifecycle) {
		return providerTransportComposition{}, fmt.Errorf("%w: field=%s.components.execution.id reason=unsupported_execution_lifecycle lifecycle=%s", ErrInvalidModelCatalog, field, transport.Components.Execution.ID)
	}
	if !requestCodecSupportsLifecycle(transport.Components.RequestCodec.ID, lifecycle) {
		return providerTransportComposition{}, unsupportedTransportComponentCombination(transport, field, "execution_lifecycle")
	}
	visibility, visibilityError := validatedProviderCatalogResourceVisibility(transport.Components.Execution, transport.Components.RequestCodec.ID, field+".components.execution.resource_visibility")
	if visibilityError != nil {
		return providerTransportComposition{}, visibilityError
	}
	return providerTransportComposition{
		requestCodec:       transport.Components.RequestCodec.ID,
		responseCodec:      transport.Components.ResponseCodec.ID,
		authentication:     transport.Components.Authentication,
		lifecycle:          lifecycle,
		resourceVisibility: visibility,
		parameters: providerProtocolParameters{
			ResponsePolicy:          response.responsePolicy,
			ModelField:              request.modelField,
			TokenField:              request.tokenField,
			MediaExecutionLifecycle: string(request.mediaExecutionLifecycle),
			OutputFields:            append([]string(nil), response.outputFields...),
			FinishRules: providerProtocolFinishRules{
				Complete: append([]string(nil), response.finishRules.Complete...),
				Continue: append([]string(nil), response.finishRules.Continue...),
			},
			ContinuationRules: append([]string(nil), response.continuationRules...),
			ErrorRules:        append([]string(nil), response.errorRules...),
			UsageFields:       response.usageFields,
		},
	}, nil
}

func providerRequestCodecDefinitionFor(reference ProviderCatalogCodecReference, field string) (providerRequestCodecDefinition, error) {
	definition := providerRequestCodecDefinition{}
	switch reference.ID {
	case CatalogProtocolOpenAIResponses:
		definition.modelField = "model"
		definition.tokenField = "max_output_tokens"
		definition.mediaExecutionLifecycle = textExecutionLifecyclePollableResource
	case CatalogProtocolDashScopeResponses, CatalogProtocolXAIResponses:
		definition.modelField = "model"
		definition.tokenField = "max_output_tokens"
		definition.mediaExecutionLifecycle = textExecutionLifecycleSynchronousCompletion
	case CatalogProtocolOpenAIChatCompletions:
		definition.modelField = "model"
		definition.mediaExecutionLifecycle = textExecutionLifecycleSynchronousCompletion
		switch reference.Variation {
		case CatalogProtocolVariationMaxTokens:
			definition.tokenField = string(chatCompletionTokenLimitMaxTokens)
		case CatalogProtocolVariationMaxCompletionTokens:
			definition.tokenField = string(chatCompletionTokenLimitMaxCompletionTokens)
		default:
			return providerRequestCodecDefinition{}, unsupportedProviderCodecVariation(reference, field)
		}
		return definition, nil
	case CatalogProtocolAnthropicMessages:
		definition.modelField = "model"
		definition.tokenField = "max_tokens"
		definition.mediaExecutionLifecycle = textExecutionLifecycleSynchronousCompletion
		definition.requiredHeaders = []ProviderCatalogHeader{{Name: "anthropic-version", Value: "2023-06-01"}}
	case CatalogProtocolVertexGenerateContent:
		definition.modelField = "path.model"
		definition.tokenField = "generationConfig.maxOutputTokens"
		definition.mediaExecutionLifecycle = textExecutionLifecycleSynchronousCompletion
	case CatalogProtocolGeminiInteractions:
		definition.modelField = "model"
		definition.tokenField = "generation_config.max_output_tokens"
		definition.mediaExecutionLifecycle = textExecutionLifecycleSynchronousCompletion
		definition.requiredHeaders = []ProviderCatalogHeader{{Name: "Api-Revision", Value: "2026-05-20"}}
	case CatalogProtocolMetaTranscription:
		definition.modelField = "request.model"
	case CatalogProtocolMultipartTranscription:
		switch reference.Variation {
		case CatalogProtocolVariationTranscriptionModel:
			definition.modelField = "model"
		case CatalogProtocolVariationTranscriptionModelOmitted:
		default:
			return providerRequestCodecDefinition{}, unsupportedProviderCodecVariation(reference, field)
		}
		return definition, nil
	case CatalogProtocolXAIVideosGenerations:
		definition.modelField = "model"
	default:
		return providerRequestCodecDefinition{}, unsupportedProviderCodec(reference, field)
	}
	if reference.Variation != constants.EmptyString {
		return providerRequestCodecDefinition{}, unsupportedProviderCodecVariation(reference, field)
	}
	return definition, nil
}

func providerResponseCodecDefinitionFor(reference ProviderCatalogCodecReference, field string) (providerResponseCodecDefinition, error) {
	definition := providerResponseCodecDefinition{}
	switch reference.ID {
	case CatalogProtocolDashScopeResponses:
		definition.outputFields = []string{"output[].content[].text"}
		definition.finishRules = providerProtocolFinishRules{Complete: []string{"completed"}, Continue: []string{"incomplete"}}
		definition.continuationRules = []string{"append_visible_assistant_output", "request_missing_suffix"}
		definition.errorRules = []string{"cancelled", "failed", "unknown_status"}
		definition.usageFields = providerProtocolUsageFields{Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "usage.total_tokens"}
	case CatalogProtocolOpenAIResponses, CatalogProtocolXAIResponses:
		definition.outputFields = []string{"output[].content[].text", "output[].type", "output[].call_id", "output[].name", "output[].arguments"}
		definition.finishRules = providerProtocolFinishRules{Complete: []string{"completed"}, Continue: []string{"incomplete:max_output_tokens"}}
		definition.continuationRules = []string{"append_visible_assistant_output", "request_missing_suffix"}
		definition.errorRules = []string{"cancelled", "failed", "refusal", "unknown_status"}
		definition.usageFields = providerProtocolUsageFields{Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "usage.total_tokens"}
	case CatalogProtocolOpenAIChatCompletions:
		definition.outputFields = []string{"choices[].message.content", "choices[].message.tool_calls"}
		definition.finishRules = providerProtocolFinishRules{Complete: []string{"stop", "tool_calls"}, Continue: []string{"length"}}
		definition.continuationRules = []string{"append_visible_assistant_output", "request_missing_suffix"}
		definition.errorRules = []string{"content_filter", "unknown_finish_reason"}
		definition.usageFields = providerProtocolUsageFields{Input: "usage.prompt_tokens", Output: "usage.completion_tokens", Total: "usage.total_tokens"}
		if reference.Variation == CatalogProtocolVariationQianfan {
			definition.responsePolicy = string(chatCompletionResponsePolicyQianfan)
			definition.outputFields = []string{"choices[].message.content"}
			definition.finishRules.Complete = []string{"stop"}
			definition.errorRules = []string{"content_filter", "tool_calls", "unknown_finish_reason", "blocked_flag"}
			return definition, nil
		}
	case CatalogProtocolAnthropicMessages:
		definition.outputFields = []string{"content[].text"}
		definition.finishRules = providerProtocolFinishRules{Complete: []string{"end_turn", "stop_sequence"}, Continue: []string{"max_tokens"}}
		definition.continuationRules = []string{"append_visible_assistant_output", "request_missing_suffix"}
		definition.errorRules = []string{"pause_turn", "refusal", "tool_use", "unknown_stop_reason"}
		definition.usageFields = providerProtocolUsageFields{Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "derived_input_plus_output"}
	case CatalogProtocolVertexGenerateContent:
		definition.outputFields = []string{"candidates[].content.parts[].text"}
		definition.finishRules = providerProtocolFinishRules{Complete: []string{"STOP"}}
		definition.errorRules = []string{"MAX_TOKENS", "blocked", "unknown_finish_reason"}
		definition.usageFields = providerProtocolUsageFields{Input: "usageMetadata.promptTokenCount", Output: "usageMetadata.candidatesTokenCount+thoughtsTokenCount", Total: "usageMetadata.totalTokenCount"}
	case CatalogProtocolGeminiInteractions:
		definition.outputFields = []string{"outputs[].text"}
		definition.finishRules = providerProtocolFinishRules{Complete: []string{"completed"}, Continue: []string{"incomplete"}}
		definition.continuationRules = []string{}
		definition.errorRules = []string{"blocked", "cancelled", "failed", "unknown_status"}
		definition.usageFields = providerProtocolUsageFields{Input: "usage.input_tokens", Output: "usage.output_tokens", Total: "usage.total_tokens"}
	case CatalogProtocolMetaTranscription:
		definition.outputFields = []string{"transcript"}
		definition.finishRules = providerProtocolFinishRules{Complete: []string{"http_2xx"}}
		definition.errorRules = []string{"malformed_response", "provider_error"}
	case CatalogProtocolMultipartTranscription:
		definition.outputFields = []string{"text"}
		definition.finishRules = providerProtocolFinishRules{Complete: []string{"http_2xx"}, Continue: []string{}}
		definition.continuationRules = []string{}
		definition.errorRules = []string{"malformed_response", "provider_error"}
	case CatalogProtocolXAIVideosGenerations:
		definition.outputFields = []string{"data[].url"}
		definition.finishRules = providerProtocolFinishRules{Complete: []string{"completed"}, Continue: []string{"pending"}}
		definition.continuationRules = []string{}
		definition.errorRules = []string{"failed", "unknown_status"}
	default:
		return providerResponseCodecDefinition{}, unsupportedProviderCodec(reference, field)
	}
	if reference.Variation != constants.EmptyString {
		return providerResponseCodecDefinition{}, unsupportedProviderCodecVariation(reference, field)
	}
	return definition, nil
}

func requestCodecSupportsLifecycle(codec string, lifecycle textExecutionLifecycle) bool {
	switch codec {
	case CatalogProtocolOpenAIResponses, CatalogProtocolXAIVideosGenerations:
		return lifecycle == textExecutionLifecyclePollableResource
	case CatalogProtocolGeminiInteractions:
		return lifecycle == textExecutionLifecyclePollableResource || lifecycle == textExecutionLifecycleSynchronousCompletion
	case CatalogProtocolDashScopeResponses, CatalogProtocolXAIResponses, CatalogProtocolOpenAIChatCompletions,
		CatalogProtocolAnthropicMessages, CatalogProtocolVertexGenerateContent,
		CatalogProtocolMetaTranscription, CatalogProtocolMultipartTranscription:
		return lifecycle == textExecutionLifecycleSynchronousCompletion
	default:
		return false
	}
}

func unsupportedProviderCodec(reference ProviderCatalogCodecReference, field string) error {
	return fmt.Errorf("%w: field=%s.id reason=unsupported_codec codec=%s", ErrInvalidModelCatalog, field, reference.ID)
}

func unsupportedProviderCodecVariation(reference ProviderCatalogCodecReference, field string) error {
	return fmt.Errorf("%w: field=%s.variation reason=unsupported_codec_variation codec=%s variation=%s", ErrInvalidModelCatalog, field, reference.ID, reference.Variation)
}

func unsupportedTransportComponentCombination(transport ProviderCatalogTransport, field string, component string) error {
	return fmt.Errorf("%w: field=%s.components reason=unsupported_component_combination component=%s request_codec=%s response_codec=%s authentication=%s execution=%s", ErrInvalidModelCatalog, field, component, transport.Components.RequestCodec.ID, transport.Components.ResponseCodec.ID, transport.Components.Authentication.Kind, transport.Components.Execution.ID)
}
