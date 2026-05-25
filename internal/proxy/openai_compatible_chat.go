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

	"github.com/temirov/llm-proxy/internal/constants"
	"github.com/temirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

type openAICompatibleChatClient struct {
	httpClient      HTTPDoer
	requestTimeout  time.Duration
	maxOutputTokens int
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model     string                  `json:"model"`
	Messages  []chatCompletionMessage `json:"messages"`
	MaxTokens int                     `json:"max_tokens,omitempty"`
}

type chatCompletionResponse struct {
	Choices []chatCompletionChoice `json:"choices"`
}

type chatCompletionChoice struct {
	Message chatCompletionResponseMessage `json:"message"`
}

type chatCompletionResponseMessage struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

func newOpenAICompatibleChatClient(httpClient HTTPDoer, requestTimeout time.Duration, maxOutputTokens int) *openAICompatibleChatClient {
	return &openAICompatibleChatClient{
		httpClient:      httpClient,
		requestTimeout:  requestTimeout,
		maxOutputTokens: maxOutputTokens,
	}
}

func (client *openAICompatibleChatClient) generateText(apiKey string, baseURL string, modelIdentifier modelID, userPrompt string, systemPrompt string, structuredLogger *zap.SugaredLogger) (string, error) {
	messages := []chatCompletionMessage{}
	if !utils.IsBlank(systemPrompt) {
		messages = append(messages, chatCompletionMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, chatCompletionMessage{Role: "user", Content: userPrompt})

	payload := chatCompletionRequest{
		Model:     modelIdentifier.string(),
		Messages:  messages,
		MaxTokens: client.maxOutputTokens,
	}
	payloadBytes, _ := json.Marshal(payload)

	requestContext, cancelRequest := context.WithTimeout(context.Background(), client.requestTimeout)
	defer cancelRequest()
	requestURL := strings.TrimRight(baseURL, "/") + "/chat/completions"
	httpRequest, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodPost, requestURL, apiKey, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return constants.EmptyString, buildError
	}
	statusCode, responseBytes, _, requestError := utils.PerformHTTPRequest(client.httpClient.Do, httpRequest, structuredLogger, logEventProviderRequestError)
	if requestError != nil {
		return constants.EmptyString, requestError
	}
	if statusCode == http.StatusTooManyRequests {
		return constants.EmptyString, fmt.Errorf("%w: chat completion", ErrProviderRateLimited)
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return constants.EmptyString, fmt.Errorf("%w: status=%d", ErrProviderAPI, statusCode)
	}
	responseText, parseError := parseChatCompletionText(responseBytes)
	if parseError != nil {
		return constants.EmptyString, parseError
	}
	return responseText, nil
}

func parseChatCompletionText(responseBytes []byte) (string, error) {
	var response chatCompletionResponse
	if decodeError := json.Unmarshal(responseBytes, &response); decodeError != nil {
		return constants.EmptyString, decodeError
	}
	for _, choice := range response.Choices {
		trimmedContent := strings.TrimSpace(choice.Message.Content)
		if trimmedContent != constants.EmptyString {
			return trimmedContent, nil
		}
		trimmedReasoning := strings.TrimSpace(choice.Message.ReasoningContent)
		if trimmedReasoning != constants.EmptyString {
			return trimmedReasoning, nil
		}
	}
	return constants.EmptyString, errors.New(errorProviderNoText)
}
