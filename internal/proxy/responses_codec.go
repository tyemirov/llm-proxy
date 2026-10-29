package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

// responsesVariation selects one qualified Responses wire contract.
type responsesVariation uint8

const (
	responsesXAI responsesVariation = iota
	responsesDashScope
	responsesOpenAI
)

var responsesVariationLabels = map[responsesVariation]string{
	responsesXAI:       "xAI",
	responsesDashScope: "DashScope",
	responsesOpenAI:    "OpenAI",
}

func (variation responsesVariation) label() string { return responsesVariationLabels[variation] }

type responsesCodec struct {
	httpClient HTTPDoer
	variation  responsesVariation
}

type responsesEnvelope struct {
	Status            string                `json:"status"`
	Error             json.RawMessage       `json:"error"`
	Output            []responsesOutputItem `json:"output"`
	IncompleteDetails struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Usage *upstreamTokenUsage `json:"usage"`
}

type responsesToolCallItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (item responsesToolCallItem) functionCall() functionCall {
	return functionCall{ID: item.CallID, Name: item.Name, Arguments: item.Arguments}
}

type responsesOutputItem struct {
	responsesToolCallItem
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func (client responsesCodec) generateText(ctx context.Context, apiKey, endpointURL string, model textModelDefinition, messages chatMessages, maxTokens *int, reasoningEffort string, schema *structuredOutputSchema, tools *callerTools, logger *zap.SugaredLogger) (textGenerationResult, error) {
	encoded, err := client.encodeRequest(model, messages, false, maxTokens, reasoningEffort, schema, tools)
	if err != nil {
		return textGenerationResult{}, err
	}
	request, err := buildAuthorizedJSONRequest(ctx, http.MethodPost, endpointURL, apiKey, bytes.NewReader(encoded))
	if err != nil {
		return textGenerationResult{}, err
	}
	status, body, headers, _, err := utils.PerformHTTPRequest(client.httpClient.Do, request, logger, logEventProviderRequestError)
	if err := providerResponseError(status, headers, err); err != nil {
		return textGenerationResult{}, err
	}
	return client.parse(body)
}

func (client responsesCodec) parse(body []byte) (textGenerationResult, error) {
	var response responsesEnvelope
	if err := json.Unmarshal(body, &response); err != nil {
		return textGenerationResult{}, fmt.Errorf("%w: decode %s response: %w", ErrProviderAPI, client.variation.label(), err)
	}
	generation := textGenerationResult{}
	if response.Usage != nil {
		usage, err := normalizeTokenUsage(response.Usage.InputTokens, response.Usage.OutputTokens, response.Usage.TotalTokens)
		if err != nil {
			return generation, err
		}
		generation.usage = usage
	}
	fail := func(reason string) (textGenerationResult, error) {
		return textGenerationResult{usage: generation.usage}, fmt.Errorf("%w: %s response %s", ErrProviderAPI, client.variation.label(), reason)
	}
	if len(response.Error) > 0 && string(response.Error) != "null" {
		return fail("contains an error")
	}
	var text strings.Builder
	for _, item := range response.Output {
		switch item.Type {
		case "reasoning": // Reasoning text and encrypted state are provider-private.
		case "function_call":
			if client.variation == responsesDashScope {
				return fail("contains unsupported output")
			}
			generation.toolCalls = append(generation.toolCalls, item.functionCall())
		case "message":
			if item.Role != "assistant" {
				return fail("contains a non-assistant message")
			}
			for _, content := range item.Content {
				if content.Type != "output_text" {
					return fail("contains unsupported message content")
				}
				text.WriteString(content.Text)
			}
		default:
			return fail("contains unsupported output")
		}
	}
	if err := validateFunctionCalls(generation.toolCalls); err != nil {
		return fail("contains invalid function calls")
	}
	generation.text = text.String()
	switch response.Status {
	case "completed":
		if strings.TrimSpace(generation.text) == "" && len(generation.toolCalls) == 0 {
			return fail("has no visible result")
		}
		return generation, nil
	case "incomplete":
		if client.variation == responsesDashScope || (response.IncompleteDetails.Reason == "max_output_tokens" && len(generation.toolCalls) == 0) {
			return generation, errProviderOutputLimitReached
		}
		return fail("is incomplete")
	default:
		return fail("has no synchronous terminal result")
	}
}

// encodeRequest owns request serialization while execution remains a transport concern.
func (client responsesCodec) encodeRequest(model textModelDefinition, messages chatMessages, webSearch bool, maxTokens *int, reasoningEffort string, schema *structuredOutputSchema, tools *callerTools) ([]byte, error) {
	if err := validateInlineMessageMediaBeforeSerialization(model, messages); err != nil {
		return nil, err
	}
	imageDetail := "auto"
	if client.variation == responsesXAI {
		imageDetail = "high"
	}
	if client.variation == responsesDashScope {
		imageDetail = ""
	}
	input, err := messages.openAIResponsesInput(imageDetail, client.variation != responsesOpenAI || tools != nil)
	if err != nil {
		return nil, err
	}
	var requestPayload any
	if client.variation == responsesOpenAI {
		requestPayload = buildRequestPayload(model.providerString(), model.requestProfile.string(), input, webSearch, maxTokens, reasoningEffort, true, true, schema, tools)
	} else {
		payload := map[string]any{"model": model.providerString(), "input": input, "store": false}
		if maxTokens != nil {
			payload["max_output_tokens"] = *maxTokens
		}
		if reasoningEffort != "" {
			payload["reasoning"] = map[string]any{"effort": reasoningEffort}
		}
		if schema != nil {
			payload["text"] = openAIStructuredTextFor(schema)
		}
		tools.applyResponses(payload)
		requestPayload = payload
	}
	encoded, _ := json.Marshal(requestPayload)
	if err := validateInlineMessageMediaRequestLimit(model, messages, encoded); err != nil {
		return nil, err
	}

	return encoded, nil
}

func (client responsesCodec) decodeOpenAI(responseBytes []byte) (openAIResponseSnapshot, error) {
	var decodedObject map[string]any
	decodeError := json.Unmarshal(responseBytes, &decodedObject)
	incompleteReason := constants.EmptyString
	if incompleteDetails, ok := decodedObject["incomplete_details"].(map[string]any); ok {
		incompleteReason = utils.GetString(incompleteDetails, "reason")
	}
	usage, usageError := parseResponsesTokenUsage(responseBytes)
	responseSnapshot := openAIResponseSnapshot{
		identifier:            utils.GetString(decodedObject, jsonFieldID),
		status:                utils.GetString(decodedObject, jsonFieldStatus),
		incompleteReason:      incompleteReason,
		text:                  extractTextFromAny(responseBytes),
		usage:                 usage,
		hasFinalMessage:       hasFinalMessage(responseBytes),
		hasTopLevelOutputText: !utils.IsBlank(utils.GetString(decodedObject, jsonFieldOutputText)),
	}
	if decodeError != nil {
		return responseSnapshot, decodeError
	}
	if usageError != nil {
		return responseSnapshot, usageError
	}
	var toolEnvelope struct {
		Output []responsesToolCallItem `json:"output"`
	}
	if err := json.Unmarshal(responseBytes, &toolEnvelope); err != nil {
		return responseSnapshot, err
	}
	for _, item := range toolEnvelope.Output {
		if item.Type == "function_call" {
			responseSnapshot.toolCalls = append(responseSnapshot.toolCalls, item.functionCall())
		}
	}
	// Pending snapshots can contain function arguments that are still being generated.
	if !responseSnapshot.isPending() {
		if err := validateFunctionCalls(responseSnapshot.toolCalls); err != nil {
			return responseSnapshot, err
		}
	}
	return responseSnapshot, nil
}
