package proxy

import (
	"context"
	"errors"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

const completionContinuationInstruction = "Continue exactly where the previous response stopped. Return only the missing suffix without repeating any completed text."

type providerRouter struct {
	openAIClient    *OpenAIClient
	chatClient      *openAICompatibleChatClient
	geminiClient    *geminiInteractionsClient
	anthropicClient *anthropicMessagesClient
}

type textRouteAdapter interface {
	generateText(context.Context, *providerRouter, chatRequestParameters, *zap.SugaredLogger) (textGenerationResult, error)
}

type openAIResponsesTextRouteAdapter struct{}
type openAIResponsesSynchronousTextRouteAdapter struct{}
type openAIChatCompletionsTextRouteAdapter struct{}
type geminiInteractionsTextRouteAdapter struct{}
type anthropicMessagesTextRouteAdapter struct{}

var textRouteAdapters = map[textRouteCapabilities]textRouteAdapter{
	openAIResponsesPollableRouteCapabilities:          openAIResponsesTextRouteAdapter{},
	openAIResponsesSynchronousRouteCapabilities:       openAIResponsesSynchronousTextRouteAdapter{},
	openAIChatCompletionsSynchronousRouteCapabilities: openAIChatCompletionsTextRouteAdapter{},
	geminiInteractionsPollableRouteCapabilities:       geminiInteractionsTextRouteAdapter{},
	geminiInteractionsSynchronousRouteCapabilities:    geminiInteractionsTextRouteAdapter{},
	anthropicMessagesSynchronousRouteCapabilities:     anthropicMessagesTextRouteAdapter{},
}

func newProviderRouter(openAIClient *OpenAIClient, chatClient *openAICompatibleChatClient, geminiClient *geminiInteractionsClient, anthropicClient *anthropicMessagesClient) *providerRouter {
	return &providerRouter{
		openAIClient:    openAIClient,
		chatClient:      chatClient,
		geminiClient:    geminiClient,
		anthropicClient: anthropicClient,
	}
}

func (router *providerRouter) generateText(requestContext context.Context, request chatRequestParameters, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	originalMessages := request.messages
	accumulatedText := strings.Builder{}
	var accumulatedUsage *tokenUsage
	for {
		generation, generationError := router.generateTextAttempt(requestContext, request, structuredLogger)
		accumulatedText.WriteString(generation.text)
		accumulatedUsage = mergeTokenUsage(accumulatedUsage, generation.usage)
		recordContinuationProgress(requestContext, structuredLogger, generation, len([]byte(accumulatedText.String())), generationError)
		if !errors.Is(generationError, errProviderOutputLimitReached) {
			return textGenerationResult{
				text:  strings.TrimSpace(accumulatedText.String()),
				usage: accumulatedUsage,
			}, generationError
		}

		request.messages = completionContinuationMessages(originalMessages, accumulatedText.String())
		request.maxTokens = continuationMaxTokens(request.maxTokens, request.model, generation.text)
		request.chatCompletionContinuation = generation.chatCompletionContinuation
		if waitError := waitForRequestTelemetryPhase(requestContext, responsePollInterval, requestTelemetryPhaseContinuationWait); waitError != nil {
			return textGenerationResult{
				text:  strings.TrimSpace(accumulatedText.String()),
				usage: accumulatedUsage,
			}, waitError
		}
	}
}

func (router *providerRouter) generateTextAttempt(requestContext context.Context, request chatRequestParameters, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	return request.model.routeAdapter.generateText(requestContext, router, request, structuredLogger)
}

func (openAIResponsesTextRouteAdapter) generateText(requestContext context.Context, router *providerRouter, request chatRequestParameters, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	return router.openAIClient.openAIRequest(
		requestContext,
		request.provider.credentialFor(endpointKindText),
		request.model,
		request.messages,
		request.webSearchEnabled,
		request.maxTokens,
		request.reasoningEffort,
		structuredLogger,
	)
}

func (openAIResponsesSynchronousTextRouteAdapter) generateText(requestContext context.Context, router *providerRouter, request chatRequestParameters, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	return router.openAIClient.xAIResponsesRequest(
		requestContext,
		request.provider.credentialFor(endpointKindText),
		request.provider.textBaseURL,
		request.model,
		request.messages,
		request.maxTokens,
		structuredLogger,
	)
}

func (openAIChatCompletionsTextRouteAdapter) generateText(requestContext context.Context, router *providerRouter, request chatRequestParameters, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	return router.chatClient.generateText(
		requestContext,
		request.provider.credentialFor(endpointKindText),
		request.provider.textBaseURL,
		request.model,
		request.messages,
		request.maxTokens,
		request.provider.chatTokenLimitParameter,
		request.reasoningEffort,
		request.chatCompletionContinuation,
		structuredLogger,
	)
}

func (geminiInteractionsTextRouteAdapter) generateText(requestContext context.Context, router *providerRouter, request chatRequestParameters, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	return router.geminiClient.generateText(
		requestContext,
		request.provider.credentialFor(endpointKindText),
		request.provider.textBaseURL,
		request.model,
		request.messages,
		request.maxTokens,
		request.model.executionLifecycle,
		structuredLogger,
	)
}

func (anthropicMessagesTextRouteAdapter) generateText(requestContext context.Context, router *providerRouter, request chatRequestParameters, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	return router.anthropicClient.generateText(
		requestContext,
		request.provider.credentialFor(endpointKindText),
		request.provider.textBaseURL,
		request.model,
		request.messages,
		request.maxTokens,
		structuredLogger,
	)
}

func completionContinuationMessages(originalMessages chatMessages, accumulatedText string) chatMessages {
	continuationMessages := append(chatMessages(nil), originalMessages...)
	if !utils.IsBlank(accumulatedText) {
		continuationMessages = append(continuationMessages, chatMessage{role: chatRoleAssistant, content: accumulatedText})
	}
	return append(continuationMessages, chatMessage{role: chatRoleUser, content: completionContinuationInstruction})
}

func continuationMaxTokens(currentMaxTokens *int, model textModelDefinition, latestText string) *int {
	if !utils.IsBlank(latestText) || !model.hasOutputTokenLimit {
		return currentMaxTokens
	}
	nextMaxTokens := model.outputTokenLimit
	if currentMaxTokens != nil && *currentMaxTokens < model.outputTokenLimit && *currentMaxTokens <= model.outputTokenLimit/2 {
		nextMaxTokens = *currentMaxTokens * 2
	}
	return &nextMaxTokens
}

func (router *providerRouter) transcribeAudio(requestContext context.Context, request dictationRequestParameters, structuredLogger *zap.SugaredLogger) (string, error) {
	transcriptionsURL := request.provider.transcriptionsURL
	if request.provider.identifier == providerID(ProviderNameOpenAI) {
		transcriptionsURL = router.openAIClient.endpoints.GetTranscriptionsURL()
	}
	providerModel := request.provider.transcriptionModels[strings.ToLower(request.model.string())].providerIdentifier
	return router.openAIClient.transcribeAudioWithURL(
		requestContext,
		request.provider.credentialFor(endpointKindDictation),
		transcriptionsURL,
		request.provider.transcriptionModelField,
		providerModel.string(),
		request.fileName,
		request.audioReader,
		structuredLogger,
	)
}
