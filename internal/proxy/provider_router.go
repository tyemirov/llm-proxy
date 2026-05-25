package proxy

import (
	"fmt"

	"github.com/temirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

type providerRouter struct {
	openAIClient *OpenAIClient
	chatClient   *openAICompatibleChatClient
}

func newProviderRouter(openAIClient *OpenAIClient, chatClient *openAICompatibleChatClient) *providerRouter {
	return &providerRouter{
		openAIClient: openAIClient,
		chatClient:   chatClient,
	}
}

func (router *providerRouter) generateText(request chatRequestParameters, structuredLogger *zap.SugaredLogger) (string, error) {
	if request.provider.usesOpenAIResponses {
		return router.openAIClient.openAIRequest(
			request.provider.credentialFor(endpointKindText),
			request.model.string(),
			request.prompt,
			request.systemPrompt,
			request.webSearchEnabled,
			structuredLogger,
		)
	}
	return router.chatClient.generateText(
		request.provider.credentialFor(endpointKindText),
		request.provider.textBaseURL,
		request.model,
		request.prompt,
		request.systemPrompt,
		structuredLogger,
	)
}

func (router *providerRouter) transcribeAudio(request dictationRequestParameters, structuredLogger *zap.SugaredLogger) (string, error) {
	transcriptionsURL := request.provider.transcriptionsURL
	if request.provider.identifier == providerID(ProviderNameOpenAI) {
		transcriptionsURL = router.openAIClient.endpoints.GetTranscriptionsURL()
	}
	if transcriptionsURL == constants.EmptyString {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s endpoint=%s", ErrUnsupportedEndpoint, request.provider.identifier.string(), endpointKindDictation)
	}
	return router.openAIClient.transcribeAudioWithURL(
		request.provider.credentialFor(endpointKindDictation),
		transcriptionsURL,
		request.model.string(),
		request.fileName,
		request.audioReader,
		structuredLogger,
	)
}
