package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

type openAICompatibleChatClient struct {
	httpClient HTTPDoer
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model               string                  `json:"model"`
	Messages            []chatCompletionMessage `json:"messages"`
	MaxTokens           *int                    `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                    `json:"max_completion_tokens,omitempty"`
}

type chatCompletionResponse struct {
	Choices []chatCompletionChoice `json:"choices"`
	Usage   *upstreamTokenUsage    `json:"usage"`
}

type chatCompletionChoice struct {
	Message      chatCompletionResponseMessage `json:"message"`
	FinishReason string                        `json:"finish_reason"`
}

type chatCompletionResponseMessage struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

func newOpenAICompatibleChatClient(httpClient HTTPDoer) *openAICompatibleChatClient {
	return &openAICompatibleChatClient{
		httpClient: httpClient,
	}
}

func (client *openAICompatibleChatClient) generateText(parentContext context.Context, apiKey string, baseURL string, modelIdentifier textModelDefinition, messages chatMessages, maxTokens *int, tokenLimitParameter chatCompletionTokenLimitParameter, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	payload := chatCompletionRequest{
		Model:    modelIdentifier.string(),
		Messages: messages.chatCompletionMessages(),
	}
	if maxTokens != nil {
		switch tokenLimitParameter {
		case chatCompletionTokenLimitMaxTokens:
			payload.MaxTokens = maxTokens
		case chatCompletionTokenLimitMaxCompletionTokens:
			payload.MaxCompletionTokens = maxTokens
		}
	}
	payloadBytes, _ := json.Marshal(payload)

	requestURL := strings.TrimRight(baseURL, "/") + "/chat/completions"
	httpRequest, buildError := buildAuthorizedJSONRequest(parentContext, http.MethodPost, requestURL, apiKey, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return textGenerationResult{}, buildError
	}
	statusCode, responseBytes, responseHeader, _, requestError := utils.PerformHTTPRequest(client.httpClient.Do, httpRequest, structuredLogger, logEventProviderRequestError)
	if responseError := providerResponseError(statusCode, responseHeader, requestError); responseError != nil {
		return textGenerationResult{}, responseError
	}
	generation, parseError := parseChatCompletionResponse(responseBytes)
	if parseError != nil {
		return generation, parseError
	}
	return generation, nil
}

func (messages chatMessages) chatCompletionMessages() []chatCompletionMessage {
	chatMessagesPayload := make([]chatCompletionMessage, 0, len(messages))
	for _, message := range messages {
		chatMessagesPayload = append(chatMessagesPayload, chatCompletionMessage{Role: string(message.role), Content: message.content})
	}
	return chatMessagesPayload
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
	generation := textGenerationResult{usage: usage}
	for _, choice := range response.Choices {
		finishReason := strings.TrimSpace(choice.FinishReason)
		if finishReason == constants.EmptyString {
			return generation, fmt.Errorf("%w: chat completion missing finish_reason", ErrProviderAPI)
		}
		visibleText := choice.Message.Content
		if utils.IsBlank(visibleText) {
			visibleText = choice.Message.ReasoningContent
		}
		choiceGeneration := textGenerationResult{text: visibleText, usage: usage}
		if finishReason == "length" {
			return choiceGeneration, errProviderOutputLimitReached
		}
		if finishReason != finishReasonStop {
			return generation, fmt.Errorf("%w: chat completion finish_reason=%s", ErrProviderAPI, finishReason)
		}
		if !utils.IsBlank(visibleText) {
			return choiceGeneration, nil
		}
	}
	return generation, errors.New(errorProviderNoText)
}
