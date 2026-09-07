package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

type dashScopeResponsesClient struct{ httpClient HTTPDoer }

type dashScopeResponse struct {
	Status string                  `json:"status"`
	Error  json.RawMessage         `json:"error"`
	Output []dashScopeResponseItem `json:"output"`
	Usage  *struct {
		InputTokens  *int `json:"input_tokens"`
		OutputTokens *int `json:"output_tokens"`
		TotalTokens  *int `json:"total_tokens"`
	} `json:"usage"`
}

type dashScopeResponseItem struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// dashScopeResponsesInput preserves the DashScope message and image representation.
func dashScopeResponsesInput(messages chatMessages) ([]map[string]any, error) {
	input := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if len(message.attachments) == 0 {
			input = append(input, map[string]any{"role": string(message.role), "content": message.content})
			continue
		}
		content := make([]map[string]any, 0, len(message.attachments)+1)
		for _, attachment := range message.attachments {
			data, err := attachment.bytes()
			if err != nil {
				return nil, err
			}
			content = append(content, map[string]any{"type": "input_image", "image_url": "data:" + attachment.mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)})
		}
		content = append(content, map[string]any{"type": "input_text", "text": message.content})
		input = append(input, map[string]any{"role": string(message.role), "content": content})
	}
	return input, nil
}

func (client dashScopeResponsesClient) generateText(ctx context.Context, apiKey, endpointURL string, model textModelDefinition, messages chatMessages, maxTokens *int, reasoningEffort string, logger *zap.SugaredLogger) (textGenerationResult, error) {
	if err := validateInlineMessageMediaBeforeSerialization(model, messages); err != nil {
		return textGenerationResult{}, err
	}
	input, err := dashScopeResponsesInput(messages)
	if err != nil {
		return textGenerationResult{}, err
	}
	payload := map[string]any{"model": model.providerString(), "input": input, "store": false}
	if reasoningEffort != "" {
		payload["reasoning"] = map[string]string{"effort": reasoningEffort}
	}
	if maxTokens != nil {
		payload["max_output_tokens"] = *maxTokens
	}
	encoded, _ := json.Marshal(payload)
	if err := validateInlineMessageMediaRequestLimit(model, messages, encoded); err != nil {
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
	return parseDashScopeResponse(body)
}

func parseDashScopeResponse(body []byte) (textGenerationResult, error) {
	var response dashScopeResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return textGenerationResult{}, fmt.Errorf("%w: decode DashScope response: %w", ErrProviderAPI, err)
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
		return textGenerationResult{usage: generation.usage}, fmt.Errorf("%w: DashScope response %s", ErrProviderAPI, reason)
	}
	if len(response.Error) > 0 && string(response.Error) != "null" {
		return fail("contains an error")
	}
	var text strings.Builder
	for _, item := range response.Output {
		switch item.Type {
		case "reasoning": // Reasoning text and encrypted state are provider-private.
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
	generation.text = text.String()
	switch response.Status {
	case "completed":
		if strings.TrimSpace(generation.text) == "" {
			return fail("has no visible result")
		}
		return generation, nil
	case "incomplete":
		return generation, errProviderOutputLimitReached
	default:
		return fail("has no synchronous terminal result")
	}
}
