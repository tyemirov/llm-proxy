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
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

func (client *anthropicMessagesClient) generateText(parentContext context.Context, apiKey string, baseURL string, modelIdentifier textModelDefinition, messages chatMessages, maxTokens *int, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	providerMessages, systemPrompt := messages.anthropicMessages()
	payload := anthropicMessagesRequest{
		Model:     modelIdentifier.providerString(),
		MaxTokens: anthropicMaxTokens(modelIdentifier, maxTokens),
		System:    systemPrompt,
		Messages:  providerMessages,
	}
	payloadBytes, _ := json.Marshal(payload)

	requestURL := strings.TrimRight(baseURL, "/") + "/v1/messages"
	httpRequest, buildError := http.NewRequestWithContext(parentContext, http.MethodPost, requestURL, bytes.NewReader(payloadBytes))
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

func (messages chatMessages) anthropicMessages() ([]anthropicMessage, string) {
	providerMessages := make([]anthropicMessage, 0, len(messages))
	systemPrompts := []string{}
	for _, message := range messages {
		if message.role == chatRoleSystem {
			systemPrompts = append(systemPrompts, message.content)
			continue
		}
		providerMessages = append(providerMessages, anthropicMessage{Role: string(message.role), Content: message.content})
	}
	return providerMessages, strings.Join(systemPrompts, "\n\n")
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
