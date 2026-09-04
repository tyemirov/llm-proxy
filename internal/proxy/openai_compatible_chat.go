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
	ToolCalls        []chatFunctionCall `json:"tool_calls,omitempty"`
	ToolCallID       string             `json:"tool_call_id,omitempty"`
	Role             string             `json:"role"`
	Content          any                `json:"content"`
	ReasoningContent *string            `json:"reasoning_content,omitempty"`
}

type chatCompletionContinuation struct {
	messages []chatCompletionMessage
}

type chatCompletionRequest struct {
	Tools               []map[string]any        `json:"tools,omitempty"`
	ToolChoice          any                     `json:"tool_choice,omitempty"`
	ParallelToolCalls   *bool                   `json:"parallel_tool_calls,omitempty"`
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
	ToolCalls        []chatFunctionCall `json:"tool_calls"`
	Content          string             `json:"content"`
	ReasoningContent *string            `json:"reasoning_content"`
}

func newOpenAICompatibleChatClient(httpClient HTTPDoer) *openAICompatibleChatClient {
	return &openAICompatibleChatClient{
		httpClient: httpClient,
	}
}

func (client *openAICompatibleChatClient) generateText(parentContext context.Context, apiKey string, endpointURL string, modelIdentifier textModelDefinition, messages chatMessages, maxTokens *int, tokenLimitParameter chatCompletionTokenLimitParameter, reasoningEffort string, continuation *chatCompletionContinuation, tools *callerTools, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
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
	if modelIdentifier.reasoningEffort != nil && modelIdentifier.reasoningEffort.adapter == reasoningEffortAdapterOpenAIChatCompletions {
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
	if tools != nil && len(tools.declarations) > 0 {
		for _, decl := range tools.declarations {
			payload.Tools = append(payload.Tools, map[string]any{"type": "function", "function": decl})
		}
		payload.ToolChoice = tools.selection.Mode
		if tools.selection.Mode == "function" {
			payload.ToolChoice = map[string]any{"type": "function", "function": map[string]string{"name": tools.selection.Name}}
		}
		payload.ParallelToolCalls = tools.parallel
	}
	payloadBytes, _ := json.Marshal(payload)
	if mediaLimitError := validateInlineMessageMediaRequestLimit(modelIdentifier, messages, payloadBytes); mediaLimitError != nil {
		return textGenerationResult{}, mediaLimitError
	}

	httpRequest, buildError := buildAuthorizedJSONRequest(parentContext, http.MethodPost, endpointURL, apiKey, bytes.NewReader(payloadBytes))
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
		if finishReason == "tool_calls" {
			calls, err := canonicalChatFunctionCalls(choice.Message.ToolCalls)
			if err != nil || len(calls) == 0 {
				return generation, fmt.Errorf("%w: invalid tool calls", ErrProviderAPI)
			}
			choiceGeneration.toolCalls = calls
			return choiceGeneration, nil
		}
		if len(choice.Message.ToolCalls) > 0 {
			return generation, fmt.Errorf("%w: tool calls without tool stop", ErrProviderAPI)
		}
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
