package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

const (
	anthropicAPIKeyHeader           = "x-api-key"
	anthropicVersionHeader          = "anthropic-version"
	anthropicVersionValue           = "2023-06-01"
	anthropicStopReasonEndTurn      = "end_turn"
	anthropicStopReasonStopSequence = "stop_sequence"
	anthropicStopReasonMaxTokens    = "max_tokens"
)

type anthropicMessagesClient struct {
	httpClient HTTPDoer
}

type anthropicMessagesRequest struct {
	Model        string                           `json:"model"`
	MaxTokens    int                              `json:"max_tokens"`
	System       string                           `json:"system,omitempty"`
	Messages     []anthropicMessage               `json:"messages"`
	OutputConfig *anthropicStructuredOutputConfig `json:"output_config,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicInputContentBlock struct {
	Type   string                `json:"type"`
	Text   string                `json:"text,omitempty"`
	Source *anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicMessagesResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      *upstreamTokenUsage     `json:"usage"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func newAnthropicMessagesClient(httpClient HTTPDoer) *anthropicMessagesClient {
	return &anthropicMessagesClient{
		httpClient: httpClient,
	}
}

func (client *anthropicMessagesClient) generateText(parentContext context.Context, apiKey string, endpointURL string, modelIdentifier textModelDefinition, messages chatMessages, maxTokens *int, structuredOutput *structuredOutputSchema, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	if mediaLimitError := validateInlineMessageMediaBeforeSerialization(modelIdentifier, messages); mediaLimitError != nil {
		return textGenerationResult{}, mediaLimitError
	}
	providerMessages, systemPrompt, messagesError := messages.anthropicMessages()
	if messagesError != nil {
		return textGenerationResult{}, messagesError
	}
	payload := anthropicMessagesRequest{
		Model:        modelIdentifier.providerString(),
		MaxTokens:    anthropicMaxTokens(modelIdentifier, maxTokens),
		System:       systemPrompt,
		Messages:     providerMessages,
		OutputConfig: anthropicStructuredOutputFor(structuredOutput),
	}
	payloadBytes, _ := json.Marshal(payload)
	if mediaLimitError := validateInlineMessageMediaRequestLimit(modelIdentifier, messages, payloadBytes); mediaLimitError != nil {
		return textGenerationResult{}, mediaLimitError
	}

	httpRequest, buildError := http.NewRequestWithContext(parentContext, http.MethodPost, endpointURL, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return textGenerationResult{}, buildError
	}
	httpRequest.Header.Set(headerContentType, mimeApplicationJSON)
	httpRequest.Header.Set(anthropicAPIKeyHeader, strings.TrimSpace(apiKey))
	httpRequest.Header.Set(anthropicVersionHeader, anthropicVersionValue)

	statusCode, responseBytes, responseHeader, _, requestError := utils.PerformHTTPRequest(client.httpClient.Do, httpRequest, structuredLogger, logEventProviderRequestError)
	if responseError := providerResponseError(statusCode, responseHeader, requestError); responseError != nil {
		return textGenerationResult{}, responseError
	}
	generation, parseError := parseAnthropicMessagesResponse(responseBytes)
	if parseError != nil {
		return generation, parseError
	}
	return generation, nil
}

func (messages chatMessages) anthropicMessages() ([]anthropicMessage, string, error) {
	providerMessages := make([]anthropicMessage, 0, len(messages))
	systemPrompts := []string{}
	for messageIndex := range messages {
		message := &messages[messageIndex]
		if message.role == chatRoleSystem {
			systemPrompts = append(systemPrompts, message.content)
			continue
		}
		if len(message.attachments) == 0 {
			providerMessages = append(providerMessages, anthropicMessage{Role: string(message.role), Content: message.content})
			continue
		}
		content := make([]anthropicInputContentBlock, 0, len(message.attachments)+1)
		for attachmentIndex := range message.attachments {
			attachment := &message.attachments[attachmentIndex]
			data, dataError := attachment.bytes()
			if dataError != nil {
				return nil, constants.EmptyString, dataError
			}
			content = append(content, anthropicInputContentBlock{
				Type: "image",
				Source: &anthropicImageSource{
					Type:      "base64",
					MediaType: attachment.mimeType,
					Data:      base64.StdEncoding.EncodeToString(data),
				},
			})
		}
		content = append(content, anthropicInputContentBlock{Type: "text", Text: message.content})
		providerMessages = append(providerMessages, anthropicMessage{Role: string(message.role), Content: content})
	}
	return providerMessages, strings.Join(systemPrompts, "\n\n"), nil
}

func anthropicMaxTokens(modelIdentifier textModelDefinition, maxTokens *int) int {
	if maxTokens != nil {
		return *maxTokens
	}
	return modelIdentifier.outputTokenLimit
}

func parseAnthropicMessagesResponse(responseBytes []byte) (textGenerationResult, error) {
	var response anthropicMessagesResponse
	if decodeError := json.Unmarshal(responseBytes, &response); decodeError != nil {
		return textGenerationResult{}, decodeError
	}
	usage, usageError := parseAnthropicTokenUsage(response.Usage)
	if usageError != nil {
		return textGenerationResult{}, usageError
	}
	generation := textGenerationResult{usage: usage}
	var textBuilder strings.Builder
	for _, contentBlock := range response.Content {
		if contentBlock.Type == textPartType && strings.TrimSpace(contentBlock.Text) != constants.EmptyString {
			textBuilder.WriteString(contentBlock.Text)
		}
	}
	visibleText := textBuilder.String()
	stopReason := strings.TrimSpace(response.StopReason)
	if stopReason == constants.EmptyString {
		return generation, fmt.Errorf("%w: anthropic Messages missing stop_reason", ErrProviderAPI)
	}
	if stopReason == anthropicStopReasonMaxTokens {
		return textGenerationResult{text: visibleText, usage: usage}, errProviderOutputLimitReached
	}
	if stopReason != anthropicStopReasonEndTurn && stopReason != anthropicStopReasonStopSequence {
		return generation, fmt.Errorf("%w: anthropic Messages stop_reason=%s", ErrProviderAPI, stopReason)
	}
	if utils.IsBlank(visibleText) {
		return generation, fmt.Errorf("%w: anthropic Messages returned no text", ErrProviderAPI)
	}
	return textGenerationResult{text: visibleText, usage: usage}, nil
}

func parseAnthropicTokenUsage(usage *upstreamTokenUsage) (*tokenUsage, error) {
	if usage == nil {
		return nil, nil
	}
	return normalizeTokenUsage(usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
}
