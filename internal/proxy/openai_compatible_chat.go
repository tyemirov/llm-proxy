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
	Role             string  `json:"role"`
	Content          any     `json:"content"`
	ReasoningContent *string `json:"reasoning_content,omitempty"`
}

type chatCompletionContinuation struct {
	messages []chatCompletionMessage
}

type chatCompletionRequest struct {
	Model               string                  `json:"model"`
	Messages            []chatCompletionMessage `json:"messages"`
	MaxTokens           *int                    `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                    `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     string                  `json:"reasoning_effort,omitempty"`
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
	Content          string  `json:"content"`
	ReasoningContent *string `json:"reasoning_content"`
}

func newOpenAICompatibleChatClient(httpClient HTTPDoer) *openAICompatibleChatClient {
	return &openAICompatibleChatClient{
		httpClient: httpClient,
	}
}

func (client *openAICompatibleChatClient) generateText(parentContext context.Context, apiKey string, baseURL string, modelIdentifier textModelDefinition, messages chatMessages, maxTokens *int, tokenLimitParameter chatCompletionTokenLimitParameter, reasoningEffort string, continuation *chatCompletionContinuation, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	if mediaLimitError := validateInlineMessageMediaBeforeSerialization(modelIdentifier, messages); mediaLimitError != nil {
		return textGenerationResult{}, mediaLimitError
	}
	var providerMessages []chatCompletionMessage
	if continuation == nil {
		var messagesError error
		providerMessages, messagesError = messages.chatCompletionMessages()
		if messagesError != nil {
			return textGenerationResult{}, messagesError
		}
	} else {
		providerMessages = continuation.messages
	}
	payload := chatCompletionRequest{
		Model:    modelIdentifier.providerString(),
		Messages: providerMessages,
	}
	if modelIdentifier.reasoningEffort != nil && modelIdentifier.reasoningEffort.adapter == reasoningEffortAdapterMoonshotChatCompletions {
		payload.ReasoningEffort = reasoningEffort
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
	if mediaLimitError := validateInlineMessageMediaRequestLimit(modelIdentifier, messages, payloadBytes); mediaLimitError != nil {
		return textGenerationResult{}, mediaLimitError
	}

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
	if errors.Is(parseError, errProviderOutputLimitReached) && generation.chatCompletionReasoningContent != nil {
		generation.chatCompletionContinuation = newChatCompletionContinuation(providerMessages, generation.text, generation.chatCompletionReasoningContent)
	}
	if parseError != nil {
		return generation, parseError
	}
	return generation, nil
}

func newChatCompletionContinuation(providerMessages []chatCompletionMessage, visibleContent string, reasoningContent *string) *chatCompletionContinuation {
	continuationMessages := append([]chatCompletionMessage(nil), providerMessages...)
	continuationMessages = append(continuationMessages,
		chatCompletionMessage{Role: string(chatRoleAssistant), Content: visibleContent, ReasoningContent: reasoningContent},
		chatCompletionMessage{Role: string(chatRoleUser), Content: completionContinuationInstruction},
	)
	return &chatCompletionContinuation{messages: continuationMessages}
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
		choiceGeneration := textGenerationResult{text: visibleText, usage: usage, chatCompletionReasoningContent: choice.Message.ReasoningContent}
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
