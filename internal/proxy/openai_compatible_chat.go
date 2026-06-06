package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

type openAICompatibleChatClient struct {
	httpClient     HTTPDoer
	requestTimeout time.Duration
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model     string                  `json:"model"`
	Messages  []chatCompletionMessage `json:"messages"`
	MaxTokens *int                    `json:"max_tokens,omitempty"`
}

type chatCompletionResponse struct {
	Choices []chatCompletionChoice `json:"choices"`
	Usage   *upstreamTokenUsage    `json:"usage"`
}

type chatCompletionChoice struct {
	Message chatCompletionResponseMessage `json:"message"`
}

type chatCompletionResponseMessage struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

func newOpenAICompatibleChatClient(httpClient HTTPDoer, requestTimeout time.Duration) *openAICompatibleChatClient {
	return &openAICompatibleChatClient{
		httpClient:     httpClient,
		requestTimeout: requestTimeout,
	}
}

func (client *openAICompatibleChatClient) generateText(parentContext context.Context, apiKey string, baseURL string, modelIdentifier modelID, userPrompt string, systemPrompt string, maxTokens *int, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	messages := []chatCompletionMessage{}
	if !utils.IsBlank(systemPrompt) {
		messages = append(messages, chatCompletionMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, chatCompletionMessage{Role: "user", Content: userPrompt})

	payload := chatCompletionRequest{
		Model:     modelIdentifier.string(),
		Messages:  messages,
		MaxTokens: maxTokens,
	}
	payloadBytes, _ := json.Marshal(payload)

	requestContext, cancelRequest := context.WithTimeout(parentContext, client.requestTimeout)
	defer cancelRequest()
	requestURL := strings.TrimRight(baseURL, "/") + "/chat/completions"
	httpRequest, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodPost, requestURL, apiKey, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return textGenerationResult{}, buildError
	}
	statusCode, responseBytes, _, requestError := utils.PerformHTTPRequest(client.httpClient.Do, httpRequest, structuredLogger, logEventProviderRequestError)
	if requestError != nil {
		return textGenerationResult{}, requestError
	}
	if statusCode == http.StatusTooManyRequests {
		return textGenerationResult{}, fmt.Errorf("%w: chat completion", ErrProviderRateLimited)
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return textGenerationResult{}, fmt.Errorf("%w: status=%d", ErrProviderAPI, statusCode)
	}
	generation, parseError := parseChatCompletionResponse(responseBytes)
	if parseError != nil {
		return textGenerationResult{}, parseError
	}
	return generation, nil
}

func parseChatCompletionResponse(responseBytes []byte) (textGenerationResult, error) {
	var response chatCompletionResponse
	if decodeError := json.Unmarshal(responseBytes, &response); decodeError != nil {
		return textGenerationResult{}, decodeError
	}
	usage, usageError := parseChatCompletionTokenUsage(response.Usage)
	if usageError != nil {
		return textGenerationResult{}, usageError
	}
	for _, choice := range response.Choices {
		trimmedContent := strings.TrimSpace(choice.Message.Content)
		if trimmedContent != constants.EmptyString {
			return textGenerationResult{text: trimmedContent, usage: usage}, nil
		}
		trimmedReasoning := strings.TrimSpace(choice.Message.ReasoningContent)
		if trimmedReasoning != constants.EmptyString {
			return textGenerationResult{text: trimmedReasoning, usage: usage}, nil
		}
	}
	return textGenerationResult{}, errors.New(errorProviderNoText)
}
