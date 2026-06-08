package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

const (
	anthropicAPIKeyHeader  = "x-api-key"
	anthropicVersionHeader = "anthropic-version"
	anthropicVersionValue  = "2023-06-01"
)

type anthropicMessagesClient struct {
	httpClient     HTTPDoer
	requestTimeout time.Duration
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
	Content []anthropicContentBlock `json:"content"`
	Usage   *upstreamTokenUsage     `json:"usage"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func newAnthropicMessagesClient(httpClient HTTPDoer, requestTimeout time.Duration) *anthropicMessagesClient {
	return &anthropicMessagesClient{
		httpClient:     httpClient,
		requestTimeout: requestTimeout,
	}
}

func (client *anthropicMessagesClient) generateText(parentContext context.Context, apiKey string, baseURL string, modelIdentifier modelID, messages chatMessages, maxTokens *int, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	providerMessages, systemPrompt := messages.anthropicMessages()
	payload := anthropicMessagesRequest{
		Model:     modelIdentifier.string(),
		MaxTokens: anthropicMaxTokens(modelIdentifier, maxTokens),
		System:    systemPrompt,
		Messages:  providerMessages,
	}
	payloadBytes, _ := json.Marshal(payload)

	requestContext, cancelRequest := context.WithTimeout(parentContext, client.requestTimeout)
	defer cancelRequest()
	requestURL := strings.TrimRight(baseURL, "/") + "/v1/messages"
	httpRequest, buildError := http.NewRequestWithContext(requestContext, http.MethodPost, requestURL, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return textGenerationResult{}, buildError
	}
	httpRequest.Header.Set(headerContentType, mimeApplicationJSON)
	httpRequest.Header.Set(anthropicAPIKeyHeader, strings.TrimSpace(apiKey))
	httpRequest.Header.Set(anthropicVersionHeader, anthropicVersionValue)

	statusCode, responseBytes, _, requestError := utils.PerformHTTPRequest(client.httpClient.Do, httpRequest, structuredLogger, logEventProviderRequestError)
	if requestError != nil {
		return textGenerationResult{}, requestError
	}
	if statusCode == http.StatusTooManyRequests {
		return textGenerationResult{}, fmt.Errorf("%w: anthropic Messages", ErrProviderRateLimited)
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return textGenerationResult{}, fmt.Errorf("%w: status=%d", ErrProviderAPI, statusCode)
	}
	generation, parseError := parseAnthropicMessagesResponse(responseBytes)
	if parseError != nil {
		return textGenerationResult{}, parseError
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

func anthropicMaxTokens(modelIdentifier modelID, maxTokens *int) int {
	if maxTokens != nil {
		return *maxTokens
	}
	switch strings.ToLower(modelIdentifier.string()) {
	case strings.ToLower(ModelNameClaudeOpus48):
		return anthropicOpusOutputTokenLimit
	case strings.ToLower(ModelNameClaudeOpus41), strings.ToLower(ModelNameClaudeOpus41Alias):
		return anthropicLegacyOpusOutputTokenLimit
	default:
		return anthropicOutputTokenLimit
	}
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
	var textBuilder strings.Builder
	for _, contentBlock := range response.Content {
		if contentBlock.Type == textPartType && strings.TrimSpace(contentBlock.Text) != constants.EmptyString {
			textBuilder.WriteString(contentBlock.Text)
		}
	}
	trimmedText := strings.TrimSpace(textBuilder.String())
	if trimmedText == constants.EmptyString {
		return textGenerationResult{}, fmt.Errorf("%w: anthropic Messages returned no text", ErrProviderAPI)
	}
	return textGenerationResult{text: trimmedText, usage: usage}, nil
}

func parseAnthropicTokenUsage(usage *upstreamTokenUsage) (*tokenUsage, error) {
	if usage == nil {
		return nil, nil
	}
	return normalizeTokenUsage(usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
}
