package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	providerKeyVerificationPrompt           = "Verify this provider credential."
	providerKeyVerificationMaxTokens        = 16
	providerKeyVerificationResponseLimit    = int64(1 << 20)
	providerKeyRejectedError                = "provider_key_rejected"
	providerKeyVerificationRateLimitedError = "provider_key_verification_rate_limited"
	providerKeyVerificationTimedOutError    = "provider_key_verification_timed_out"
	providerKeyVerificationUnavailableError = "provider_key_verification_unavailable"
	anthropicVerificationResponseType       = "message"
	anthropicVerificationResponseRole       = "assistant"
)

var (
	errProviderKeyRejected                = errors.New(providerKeyRejectedError)
	errProviderKeyVerificationRateLimited = errors.New(providerKeyVerificationRateLimitedError)
	errProviderKeyVerificationTimedOut    = errors.New(providerKeyVerificationTimedOutError)
	errProviderKeyVerificationUnavailable = errors.New(providerKeyVerificationUnavailableError)
	pollableVerificationAcceptedStatuses  = map[string]struct{}{
		statusQueued:     {},
		statusInProgress: {},
		statusCompleted:  {},
		statusIncomplete: {},
	}
	providerKeyVerificationRequestBuilders = map[textRouteCapabilities]providerKeyVerificationRequestBuilder{
		openAIResponsesPollableRouteCapabilities:          buildOpenAIProviderKeyVerificationRequest,
		openAIResponsesSynchronousRouteCapabilities:       buildSynchronousResponsesProviderKeyVerificationRequest,
		openAIChatCompletionsSynchronousRouteCapabilities: buildChatProviderKeyVerificationRequest,
		geminiInteractionsSynchronousRouteCapabilities:    buildGeminiProviderKeyVerificationRequest,
		anthropicMessagesSynchronousRouteCapabilities:     buildAnthropicProviderKeyVerificationRequest,
	}
	providerKeyVerificationResponseValidators = map[textWireContract]providerKeyVerificationResponseValidator{
		textWireContractOpenAIResponses:       validOpenAIProviderKeyVerificationResponse,
		textWireContractOpenAIChatCompletions: validChatProviderKeyVerificationResponse,
		textWireContractGeminiInteractions:    validGeminiProviderKeyVerificationResponse,
		textWireContractAnthropicMessages:     validAnthropicProviderKeyVerificationResponse,
	}
)

type providerKeyVerifier interface {
	verify(context.Context, providerDefinition, textModelDefinition, string) error
}

type operationalProviderKeyVerifier struct {
	httpClient HTTPDoer
	endpoints  *Endpoints
	timeout    time.Duration
	logger     *zap.SugaredLogger
}

type providerKeyVerificationRequestBuilder func(context.Context, *Endpoints, providerDefinition, textModelDefinition, string) (*http.Request, error)
type providerKeyVerificationResponseValidator func([]byte) bool

type openAIProviderKeyVerificationResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type chatProviderKeyVerificationResponse struct {
	Choices []json.RawMessage `json:"choices"`
}

type geminiProviderKeyVerificationResponse struct {
	Status string `json:"status"`
}

type anthropicProviderKeyVerificationResponse struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Role string `json:"role"`
}

func newOperationalProviderKeyVerifier(httpClient HTTPDoer, endpoints *Endpoints, timeout time.Duration, logger *zap.SugaredLogger) *operationalProviderKeyVerifier {
	return &operationalProviderKeyVerifier{
		httpClient: httpClient,
		endpoints:  endpoints,
		timeout:    timeout,
		logger:     logger,
	}
}

func (verifier *operationalProviderKeyVerifier) verify(parentContext context.Context, provider providerDefinition, model textModelDefinition, apiKey string) error {
	verificationContext, cancelVerification := context.WithTimeout(parentContext, verifier.timeout)
	defer cancelVerification()

	routeCapabilities := textRouteCapabilities{
		wireContract:       model.wireContract,
		executionLifecycle: model.executionLifecycle,
	}
	trimmedAPIKey := strings.TrimSpace(apiKey)
	if routeCapabilities == geminiInteractionsPollableRouteCapabilities {
		return verifier.verifyPollableGeminiCredential(verificationContext, provider, model, trimmedAPIKey)
	}

	requestBuilder := providerKeyVerificationRequestBuilders[routeCapabilities]
	httpRequest, buildError := requestBuilder(verificationContext, verifier.endpoints, provider, model, "")
	if buildError != nil {
		return errProviderKeyVerificationUnavailable
	}
	httpClient := newProviderTransportHTTPDoer(verifier.httpClient, provider, trimmedAPIKey)
	httpResponse, requestError := httpClient.Do(httpRequest)
	if requestError != nil {
		return providerKeyVerificationTransportError(verificationContext, requestError)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		return providerKeyVerificationStatusError(httpResponse.StatusCode)
	}
	responseBytes, responseError := readProviderKeyVerificationResponse(httpResponse.Body)
	if responseError != nil {
		return responseError
	}
	responseValidator := providerKeyVerificationResponseValidators[model.wireContract]
	if !responseValidator(responseBytes) {
		return errProviderKeyVerificationUnavailable
	}
	return nil
}

func (verifier *operationalProviderKeyVerifier) verifyPollableGeminiCredential(verificationContext context.Context, provider providerDefinition, model textModelDefinition, apiKey string) (verificationError error) {
	httpClient := newProviderTransportHTTPDoer(verifier.httpClient, provider, apiKey)
	geminiClient := newGeminiInteractionsClientWithHTTPPerformer(httpClient, performProviderKeyVerificationGeminiInteractionHTTP)
	payload := geminiProviderKeyVerificationPayload(model, true)
	createdSnapshot, createError := geminiClient.createInteraction(verificationContext, "", provider.textBaseURL, payload, verifier.logger)
	if strings.TrimSpace(createdSnapshot.identifier) == "" {
		if createError != nil {
			return providerKeyVerificationProviderError(verificationContext, createError)
		}
		return errProviderKeyVerificationUnavailable
	}

	cleanupMode := createdSnapshot.cleanupMode()
	defer func() {
		cleanupError := geminiClient.releaseInteraction(verificationContext, "", provider.textBaseURL, createdSnapshot.identifier, cleanupMode, verifier.logger)
		if verificationError == nil && cleanupError != nil {
			verificationError = providerKeyVerificationProviderError(verificationContext, cleanupError)
		}
	}()
	if createError != nil {
		return providerKeyVerificationProviderError(verificationContext, createError)
	}

	lifecycle := pollableResourceLifecycle[geminiInteractionSnapshot]{
		observe: func(observationContext context.Context) (geminiInteractionSnapshot, error) {
			return geminiClient.getInteraction(observationContext, "", provider.textBaseURL, createdSnapshot.identifier, verifier.logger)
		},
		isPending:         geminiInteractionSnapshot.isPending,
		recordObservation: func(geminiInteractionSnapshot, error, pollableResourceRetryDecision) {},
	}
	retrievedSnapshot, retrieveError := lifecycle.observeCreated(verificationContext)
	if retrieveError != nil {
		return providerKeyVerificationProviderError(verificationContext, retrieveError)
	}
	cleanupMode = retrievedSnapshot.cleanupMode()
	if !validPollableGeminiProviderKeyVerificationStatus(retrievedSnapshot.status) {
		return errProviderKeyVerificationUnavailable
	}
	return nil
}

func performProviderKeyVerificationGeminiInteractionHTTP(httpClient HTTPDoer, httpRequest *http.Request, _ *zap.SugaredLogger) (int, []byte, http.Header, error) {
	httpResponse, requestError := httpClient.Do(httpRequest)
	if requestError != nil {
		return 0, nil, nil, requestError
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		return httpResponse.StatusCode, nil, httpResponse.Header, nil
	}
	responseBytes, responseError := readProviderKeyVerificationResponse(httpResponse.Body)
	return httpResponse.StatusCode, responseBytes, httpResponse.Header, responseError
}

func readProviderKeyVerificationResponse(responseBody io.Reader) ([]byte, error) {
	responseBytes, readError := io.ReadAll(io.LimitReader(responseBody, providerKeyVerificationResponseLimit+1))
	if readError != nil || int64(len(responseBytes)) > providerKeyVerificationResponseLimit {
		return nil, errProviderKeyVerificationUnavailable
	}
	return responseBytes, nil
}

func buildOpenAIProviderKeyVerificationRequest(requestContext context.Context, _ *Endpoints, provider providerDefinition, model textModelDefinition, apiKey string) (*http.Request, error) {
	maxTokens := providerKeyVerificationMaxTokens
	payload := buildRequestPayload(
		model.providerString(),
		model.requestProfile.string(),
		providerKeyVerificationPrompt,
		false,
		&maxTokens,
		"",
		false,
		false,
		nil,
	)
	payloadBytes, _ := json.Marshal(payload)
	return buildAuthorizedJSONRequest(requestContext, http.MethodPost, provider.textEndpointURL, apiKey, bytes.NewReader(payloadBytes))
}

func buildSynchronousResponsesProviderKeyVerificationRequest(requestContext context.Context, _ *Endpoints, provider providerDefinition, model textModelDefinition, apiKey string) (*http.Request, error) {
	payload := struct {
		Model           string `json:"model"`
		Input           string `json:"input"`
		MaxOutputTokens int    `json:"max_output_tokens"`
		Store           bool   `json:"store"`
	}{
		Model:           model.providerString(),
		Input:           providerKeyVerificationPrompt,
		MaxOutputTokens: providerKeyVerificationMaxTokens,
		Store:           false,
	}
	payloadBytes, _ := json.Marshal(payload)
	return buildAuthorizedJSONRequest(requestContext, http.MethodPost, provider.textEndpointURL, apiKey, bytes.NewReader(payloadBytes))
}

func buildChatProviderKeyVerificationRequest(requestContext context.Context, _ *Endpoints, provider providerDefinition, model textModelDefinition, apiKey string) (*http.Request, error) {
	maxTokens := providerKeyVerificationMaxTokens
	payload := chatCompletionRequest{
		Model: model.providerString(),
		Messages: []chatCompletionMessage{{
			Role:    string(chatRoleUser),
			Content: providerKeyVerificationPrompt,
		}},
	}
	switch provider.chatTokenLimitParameter {
	case chatCompletionTokenLimitMaxTokens:
		payload.MaxTokens = &maxTokens
	case chatCompletionTokenLimitMaxCompletionTokens:
		payload.MaxCompletionTokens = &maxTokens
	}
	payloadBytes, _ := json.Marshal(payload)
	return buildAuthorizedJSONRequest(requestContext, http.MethodPost, provider.textEndpointURL, apiKey, bytes.NewReader(payloadBytes))
}

func buildGeminiProviderKeyVerificationRequest(requestContext context.Context, _ *Endpoints, provider providerDefinition, model textModelDefinition, apiKey string) (*http.Request, error) {
	payload := geminiProviderKeyVerificationPayload(model, false)
	payloadBytes, _ := json.Marshal(payload)
	return buildProviderKeyVerificationRequestWithHeaders(
		requestContext,
		geminiInteractionsURL(provider.textBaseURL),
		payloadBytes,
		map[string]string{
			headerContentType:       mimeApplicationJSON,
			geminiAPIKeyHeader:      apiKey,
			geminiAPIRevisionHeader: geminiAPIRevisionValue,
		},
	)
}

func geminiProviderKeyVerificationPayload(model textModelDefinition, background bool) geminiInteractionRequest {
	return geminiInteractionRequest{
		Model: model.providerString(),
		Input: []geminiInteractionStep{{
			Type: geminiInteractionStepUserInput,
			Content: []geminiInteractionContent{{
				Type: geminiInteractionContentText,
				Text: providerKeyVerificationPrompt,
			}},
		}},
		GenerationConfig: &geminiInteractionGeneration{MaxOutputTokens: providerKeyVerificationMaxTokens},
		Background:       background,
		Store:            background,
	}
}

func buildAnthropicProviderKeyVerificationRequest(requestContext context.Context, _ *Endpoints, provider providerDefinition, model textModelDefinition, apiKey string) (*http.Request, error) {
	payload := anthropicMessagesRequest{
		Model:     model.providerString(),
		MaxTokens: providerKeyVerificationMaxTokens,
		Messages: []anthropicMessage{{
			Role:    string(chatRoleUser),
			Content: providerKeyVerificationPrompt,
		}},
	}
	payloadBytes, _ := json.Marshal(payload)
	return buildProviderKeyVerificationRequestWithHeaders(
		requestContext,
		provider.textEndpointURL,
		payloadBytes,
		map[string]string{
			headerContentType:      mimeApplicationJSON,
			anthropicAPIKeyHeader:  apiKey,
			anthropicVersionHeader: anthropicVersionValue,
		},
	)
}

func buildProviderKeyVerificationRequestWithHeaders(requestContext context.Context, requestURL string, payloadBytes []byte, headers map[string]string) (*http.Request, error) {
	httpRequest, buildError := http.NewRequestWithContext(requestContext, http.MethodPost, requestURL, bytes.NewReader(payloadBytes))
	if buildError != nil {
		return nil, buildError
	}
	for headerName, headerValue := range headers {
		httpRequest.Header.Set(headerName, headerValue)
	}
	return httpRequest, nil
}

func providerKeyVerificationTransportError(requestContext context.Context, requestError error) error {
	if errors.Is(requestError, context.Canceled) || errors.Is(requestError, context.DeadlineExceeded) || requestContext.Err() != nil {
		return errProviderKeyVerificationTimedOut
	}
	return errProviderKeyVerificationUnavailable
}

func providerKeyVerificationStatusError(statusCode int) error {
	switch {
	case statusCode == http.StatusRequestTimeout || statusCode == http.StatusGatewayTimeout:
		return errProviderKeyVerificationTimedOut
	case statusCode == http.StatusTooManyRequests:
		return errProviderKeyVerificationRateLimited
	case statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError:
		return errProviderKeyRejected
	default:
		return errProviderKeyVerificationUnavailable
	}
}

func providerKeyVerificationProviderError(requestContext context.Context, requestError error) error {
	statusCode, _, _, hasProviderStatus := providerHTTPMetadata(requestError)
	if hasProviderStatus {
		return providerKeyVerificationStatusError(statusCode)
	}
	return providerKeyVerificationTransportError(requestContext, requestError)
}

func validOpenAIProviderKeyVerificationResponse(responseBytes []byte) bool {
	var response openAIProviderKeyVerificationResponse
	decodeError := json.Unmarshal(responseBytes, &response)
	_, acceptedStatus := pollableVerificationAcceptedStatuses[strings.TrimSpace(response.Status)]
	return decodeError == nil && strings.TrimSpace(response.ID) != "" && acceptedStatus
}

func validChatProviderKeyVerificationResponse(responseBytes []byte) bool {
	var response chatProviderKeyVerificationResponse
	decodeError := json.Unmarshal(responseBytes, &response)
	return decodeError == nil && len(response.Choices) > 0
}

func validGeminiProviderKeyVerificationResponse(responseBytes []byte) bool {
	var response geminiProviderKeyVerificationResponse
	decodeError := json.Unmarshal(responseBytes, &response)
	return decodeError == nil &&
		(response.Status == statusCompleted || response.Status == statusIncomplete)
}

func validPollableGeminiProviderKeyVerificationStatus(status string) bool {
	_, acceptedStatus := pollableVerificationAcceptedStatuses[strings.TrimSpace(status)]
	return acceptedStatus
}

func validAnthropicProviderKeyVerificationResponse(responseBytes []byte) bool {
	var response anthropicProviderKeyVerificationResponse
	decodeError := json.Unmarshal(responseBytes, &response)
	return decodeError == nil &&
		strings.TrimSpace(response.ID) != "" &&
		response.Type == anthropicVerificationResponseType &&
		response.Role == anthropicVerificationResponseRole
}
