package proxy

import (
	"context"

	"go.uber.org/zap"
)

type providerRouter struct {
	openAIClient    *OpenAIClient
	chatClient      *openAICompatibleChatClient
	geminiClient    *geminiGenerateContentClient
	anthropicClient *anthropicMessagesClient
}

func newProviderRouter(openAIClient *OpenAIClient, chatClient *openAICompatibleChatClient, geminiClient *geminiGenerateContentClient, anthropicClient *anthropicMessagesClient) *providerRouter {
	return &providerRouter{
		openAIClient:    openAIClient,
		chatClient:      chatClient,
		geminiClient:    geminiClient,
		anthropicClient: anthropicClient,
	}
}

func (router *providerRouter) generateText(requestContext context.Context, request chatRequestParameters, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	if request.provider.textTransport == textTransportOpenAIResponses {
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
	if request.provider.textTransport == textTransportGeminiGenerate {
		return router.geminiClient.generateText(
			requestContext,
			request.provider.credentialFor(endpointKindText),
			request.provider.textBaseURL,
			request.model,
			request.messages,
			request.maxTokens,
			structuredLogger,
		)
	}
	if request.provider.textTransport == textTransportAnthropicMessages {
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
	return router.chatClient.generateText(
		requestContext,
		request.provider.credentialFor(endpointKindText),
		request.provider.textBaseURL,
		request.model,
		request.messages,
		request.maxTokens,
		request.provider.chatTokenLimitParameter,
		structuredLogger,
	)
}

func (router *providerRouter) transcribeAudio(requestContext context.Context, request dictationRequestParameters, structuredLogger *zap.SugaredLogger) (string, error) {
	transcriptionsURL := request.provider.transcriptionsURL
	if request.provider.identifier == providerID(ProviderNameOpenAI) {
		transcriptionsURL = router.openAIClient.endpoints.GetTranscriptionsURL()
	}
	return router.openAIClient.transcribeAudioWithURL(
		requestContext,
		request.provider.credentialFor(endpointKindDictation),
		transcriptionsURL,
		request.provider.transcriptionModelField,
		request.model.string(),
		request.fileName,
		request.audioReader,
		structuredLogger,
	)
}
