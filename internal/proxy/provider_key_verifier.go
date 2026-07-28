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
	openAIVerificationAcceptedStatuses    = map[string]struct{}{
		statusQueued:     {},
		statusInProgress: {},
		statusCompleted:  {},
		statusIncomplete: {},
	}
	providerKeyVerificationRequestBuilders = map[providerTextTransport]providerKeyVerificationRequestBuilder{
		textTransportOpenAIResponses:      buildOpenAIProviderKeyVerificationRequest,
		textTransportOpenAICompatibleChat: buildChatProviderKeyVerificationRequest,
		textTransportGeminiGenerate:       buildGeminiProviderKeyVerificationRequest,
		textTransportAnthropicMessages:    buildAnthropicProviderKeyVerificationRequest,
	}
	providerKeyVerificationResponseValidators = map[providerTextTransport]providerKeyVerificationResponseValidator{
		textTransportOpenAIResponses:      validOpenAIProviderKeyVerificationResponse,
		textTransportOpenAICompatibleChat: validChatProviderKeyVerificationResponse,
		textTransportGeminiGenerate:       validGeminiProviderKeyVerificationResponse,
		textTransportAnthropicMessages:    validAnthropicProviderKeyVerificationResponse,
	}
)

type providerKeyVerifier interface {
	verify(context.Context, providerDefinition, textModelDefinition, string) error
}

type operationalProviderKeyVerifier struct {
	httpClient HTTPDoer
	endpoints  *Endpoints
	timeout    time.Duration
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
	Candidates []json.RawMessage `json:"candidates"`
}

type anthropicProviderKeyVerificationResponse struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Role string `json:"role"`
}

func newOperationalProviderKeyVerifier(httpClient HTTPDoer, endpoints *Endpoints, timeout time.Duration) *operationalProviderKeyVerifier {
	return &operationalProviderKeyVerifier{
		httpClient: httpClient,
		endpoints:  endpoints,
		timeout:    timeout,
	}
}

func (verifier *operationalProviderKeyVerifier) verify(parentContext context.Context, provider providerDefinition, model textModelDefinition, apiKey string) error {
	verificationContext, cancelVerification := context.WithTimeout(parentContext, verifier.timeout)
	defer cancelVerification()

	requestBuilder := providerKeyVerificationRequestBuilders[provider.textTransport]
	httpRequest, buildError := requestBuilder(verificationContext, verifier.endpoints, provider, model, strings.TrimSpace(apiKey))
	if buildError != nil {
		return errProviderKeyVerificationUnavailable
	}
	httpResponse, requestError := verifier.httpClient.Do(httpRequest)
	if requestError != nil {
		return providerKeyVerificationTransportError(verificationContext, requestError)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		return providerKeyVerificationStatusError(httpResponse.StatusCode)
	}
	responseBytes, readError := io.ReadAll(io.LimitReader(httpResponse.Body, providerKeyVerificationResponseLimit+1))
	if readError != nil || int64(len(responseBytes)) > providerKeyVerificationResponseLimit {
		return errProviderKeyVerificationUnavailable
	}
	responseValidator := providerKeyVerificationResponseValidators[provider.textTransport]
	if !responseValidator(responseBytes) {
		return errProviderKeyVerificationUnavailable
	}
	return nil
}

func buildOpenAIProviderKeyVerificationRequest(requestContext context.Context, endpoints *Endpoints, _ providerDefinition, model textModelDefinition, apiKey string) (*http.Request, error) {
	maxTokens := providerKeyVerificationMaxTokens
	payload := buildRequestPayload(
		model.string(),
		model.requestProfile.string(),
		providerKeyVerificationPrompt,
		false,
		&maxTokens,
		"",
		false,
		false,
	)
	payloadBytes, _ := json.Marshal(payload)
	return buildAuthorizedJSONRequest(requestContext, http.MethodPost, endpoints.GetResponsesURL(), apiKey, bytes.NewReader(payloadBytes))
}

func buildChatProviderKeyVerificationRequest(requestContext context.Context, _ *Endpoints, provider providerDefinition, model textModelDefinition, apiKey string) (*http.Request, error) {
	maxTokens := providerKeyVerificationMaxTokens
	payload := chatCompletionRequest{
		Model: model.string(),
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
	requestURL := strings.TrimRight(provider.textBaseURL, "/") + "/chat/completions"
	return buildAuthorizedJSONRequest(requestContext, http.MethodPost, requestURL, apiKey, bytes.NewReader(payloadBytes))
}

func buildGeminiProviderKeyVerificationRequest(requestContext context.Context, _ *Endpoints, provider providerDefinition, model textModelDefinition, apiKey string) (*http.Request, error) {
	payload := geminiGenerateContentRequest{
		Contents: []geminiRequestContent{{
			Role:  string(chatRoleUser),
			Parts: []geminiRequestPart{{Text: providerKeyVerificationPrompt}},
		}},
		GenerationConfig: &geminiGenerationConfig{MaxOutputTokens: providerKeyVerificationMaxTokens},
	}
	payloadBytes, _ := json.Marshal(payload)
	return buildProviderKeyVerificationRequestWithHeaders(
		requestContext,
		geminiGenerateContentURL(provider.textBaseURL, model),
		payloadBytes,
		map[string]string{
			headerContentType:  mimeApplicationJSON,
			geminiAPIKeyHeader: apiKey,
		},
	)
}

func buildAnthropicProviderKeyVerificationRequest(requestContext context.Context, _ *Endpoints, provider providerDefinition, model textModelDefinition, apiKey string) (*http.Request, error) {
	payload := anthropicMessagesRequest{
		Model:     model.string(),
		MaxTokens: providerKeyVerificationMaxTokens,
		Messages: []anthropicMessage{{
			Role:    string(chatRoleUser),
			Content: providerKeyVerificationPrompt,
		}},
	}
	payloadBytes, _ := json.Marshal(payload)
	requestURL := strings.TrimRight(provider.textBaseURL, "/") + "/v1/messages"
	return buildProviderKeyVerificationRequestWithHeaders(
		requestContext,
		requestURL,
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

func validOpenAIProviderKeyVerificationResponse(responseBytes []byte) bool {
	var response openAIProviderKeyVerificationResponse
	decodeError := json.Unmarshal(responseBytes, &response)
	_, acceptedStatus := openAIVerificationAcceptedStatuses[strings.TrimSpace(response.Status)]
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
	return decodeError == nil && len(response.Candidates) > 0
}

func validAnthropicProviderKeyVerificationResponse(responseBytes []byte) bool {
	var response anthropicProviderKeyVerificationResponse
	decodeError := json.Unmarshal(responseBytes, &response)
	return decodeError == nil &&
		strings.TrimSpace(response.ID) != "" &&
		response.Type == anthropicVerificationResponseType &&
		response.Role == anthropicVerificationResponseRole
}
