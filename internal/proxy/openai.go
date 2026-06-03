package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/temirov/llm-proxy/internal/constants"
	"github.com/temirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

// HTTPDoer executes HTTP requests, allowing the proxy to abstract the underlying HTTP client.
type HTTPDoer interface {
	Do(httpRequest *http.Request) (*http.Response, error)
}

var (
	// HTTPClient is the default HTTPDoer implementation that delegates to http.DefaultClient.
	HTTPClient HTTPDoer = http.DefaultClient
)

// OpenAIClient provides access to the OpenAI responses API with configurable
// endpoints and tunable parameters.
type OpenAIClient struct {
	httpClient          HTTPDoer
	endpoints           *Endpoints
	requestTimeout      time.Duration
	maxOutputTokens     int
	upstreamPollTimeout time.Duration
}

// NewOpenAIClient constructs an OpenAIClient initialized with the supplied components.
func NewOpenAIClient(httpClient HTTPDoer, endpoints *Endpoints, requestTimeout time.Duration, maxTokens int, pollTimeout time.Duration) *OpenAIClient {
	return &OpenAIClient{
		httpClient:          httpClient,
		endpoints:           endpoints,
		requestTimeout:      requestTimeout,
		maxOutputTokens:     maxTokens,
		upstreamPollTimeout: pollTimeout,
	}
}

const (
	synthesisInstructionPrimary    = "Now synthesize the final answer with concise citations."
	continuationInstructionPrimary = "Continue from the previous response and provide the final answer."
	responsePollInterval           = 500 * time.Millisecond
)

// hasFinalMessage checks if the response payload contains the terminal assistant message.
func hasFinalMessage(rawPayload []byte) bool {
	var envelope struct {
		Output []json.RawMessage `json:"output"`
	}
	if json.Unmarshal(rawPayload, &envelope) != nil {
		return false // Can't parse, assume not final.
	}
	if len(envelope.Output) == 0 {
		return false // No output, can't be final.
	}

	for _, rawItem := range envelope.Output {
		var header struct {
			Type string `json:"type"`
			Role string `json:"role"`
		}
		if json.Unmarshal(rawItem, &header) == nil && header.Type == responseTypeMessage && header.Role == responseRoleAssistant {
			// Found the message, so this is a truly final response.
			return true
		}
	}

	// No assistant message found.
	return false
}

// openAIRequest sends a prompt to the OpenAI responses API and returns the resulting text.
func (client *OpenAIClient) openAIRequest(parentContext context.Context, openAIKey string, modelIdentifier string, userPrompt string, systemPrompt string, webSearchEnabled bool, structuredLogger *zap.SugaredLogger) (string, error) {
	// The Responses API expects a single string input. We'll prepend the system prompt to the user prompt.
	var combinedPrompt strings.Builder
	if !utils.IsBlank(systemPrompt) {
		combinedPrompt.WriteString(systemPrompt)
		combinedPrompt.WriteString("\n\n")
	}
	combinedPrompt.WriteString(userPrompt)

	payload := BuildRequestPayload(modelIdentifier, combinedPrompt.String(), webSearchEnabled, client.maxOutputTokens)
	payloadBytes, _ := json.Marshal(payload)

	requestContext, cancelRequest := context.WithTimeout(parentContext, client.requestTimeout)
	defer cancelRequest()
	httpRequest, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodPost, client.endpoints.GetResponsesURL(), openAIKey, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return constants.EmptyString, buildError
	}

	statusCode, responseBytes, latencyMillis, requestError := client.performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIRequestError)
	if requestError != nil {
		return constants.EmptyString, requestError
	}

	structuredLogger.Debugw(logEventOpenAIInitialResponseBody, logFieldResponseBody, string(responseBytes))

	var decodedObject map[string]any
	_ = json.Unmarshal(responseBytes, &decodedObject)

	outputText := extractTextFromAny(responseBytes)
	responseIdentifier := utils.GetString(decodedObject, jsonFieldID)
	apiStatus := utils.GetString(decodedObject, jsonFieldStatus)

	structuredLogger.Infow(
		logEventOpenAIResponse,
		logFieldHTTPStatus, statusCode,
		logFieldAPIStatus, apiStatus,
		constants.LogFieldLatencyMilliseconds, latencyMillis,
		logFieldResponseText, outputText,
	)

	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		structuredLogger.Desugar().Error(
			errorOpenAIAPI,
			zap.Int(logFieldStatus, statusCode),
			zap.ByteString(logFieldResponseBody, responseBytes),
		)
		return constants.EmptyString, errors.New(errorOpenAIAPI)
	}

	isTerminalStatus := false
	switch apiStatus {
	case statusCompleted, statusSucceeded, statusDone, statusCancelled, statusFailed, statusErrored, statusIncomplete:
		isTerminalStatus = true
	}

	forcedSynthesis := false
	if isTerminalStatus && apiStatus == statusCompleted && !hasFinalMessage(responseBytes) {
		forcedSynthesis = true
		structuredLogger.Debugw(logEventMissingFinalMessage)
	}

	if apiStatus == statusIncomplete {
		if !utils.IsBlank(outputText) {
			return outputText, nil
		}
		if canContinueIncompleteResponse(decodedObject) && !utils.IsBlank(responseIdentifier) {
			continuedResponseID, continuationError := client.startIncompleteContinuation(requestContext, openAIKey, responseIdentifier, modelIdentifier, webSearchEnabled, structuredLogger)
			if continuationError != nil {
				structuredLogger.Errorw(
					logEventOpenAIContinueError,
					logFieldID, responseIdentifier,
					constants.LogFieldError, continuationError,
				)
				return constants.EmptyString, openAIStageError(continuationError)
			}
			finalText, pollError := client.pollResponseUntilDone(requestContext, openAIKey, continuedResponseID, structuredLogger)
			if pollError != nil {
				structuredLogger.Errorw(
					logEventOpenAIPollError,
					logFieldID, continuedResponseID,
					constants.LogFieldError, pollError,
				)
				return constants.EmptyString, openAIStageError(pollError)
			}
			if !utils.IsBlank(finalText) {
				return finalText, nil
			}
		}
		return constants.EmptyString, ErrUpstreamIncomplete
	}

	if forcedSynthesis && !utils.IsBlank(responseIdentifier) {
		continuedResponseID, synthErr := client.startSynthesisContinuation(requestContext, openAIKey, responseIdentifier, modelIdentifier, structuredLogger)
		if synthErr != nil {
			structuredLogger.Errorw(
				logEventOpenAIContinueError,
				logFieldID, responseIdentifier,
				constants.LogFieldError, synthErr,
			)
			return constants.EmptyString, openAIStageError(synthErr)
		}
		finalText, pollError := client.pollResponseUntilDone(requestContext, openAIKey, continuedResponseID, structuredLogger)
		if pollError != nil {
			structuredLogger.Errorw(
				logEventOpenAIPollError,
				logFieldID, continuedResponseID,
				constants.LogFieldError, pollError,
			)
			return constants.EmptyString, openAIStageError(pollError)
		}
		if !utils.IsBlank(finalText) {
			return finalText, nil
		}
	}

	if !isTerminalStatus && !utils.IsBlank(responseIdentifier) {
		finalText, pollError := client.pollResponseUntilDone(requestContext, openAIKey, responseIdentifier, structuredLogger)
		if pollError != nil {
			structuredLogger.Errorw(
				logEventOpenAIPollError,
				logFieldID, responseIdentifier,
				constants.LogFieldError, pollError,
			)
			return constants.EmptyString, openAIStageError(pollError)
		}
		if !utils.IsBlank(finalText) {
			return finalText, nil
		}
	}

	// If the initial response is terminal but we couldn't extract text, it's an error.
	if utils.IsBlank(outputText) {
		return constants.EmptyString, errors.New(errorOpenAIAPI)
	}
	return outputText, nil
}

func openAIStageError(stageError error) error {
	if errors.Is(stageError, context.Canceled) || errors.Is(stageError, context.DeadlineExceeded) {
		return stageError
	}
	return errors.New(errorOpenAIAPI)
}

// startSynthesisContinuation begins a synthesis-only pass by POSTing /v1/responses with
// previous_response_id and tool_choice set to "none". It allocates enough output tokens, limits reasoning effort to minimal, and includes a low-verbosity text format hint.
// It returns the identifier of the new response.
func (client *OpenAIClient) startSynthesisContinuation(parentContext context.Context, openAIKey string, previousResponseID string, modelIdentifier string, structuredLogger *zap.SugaredLogger) (string, error) {
	outputTokenLimit := client.maxOutputTokens
	if outputTokenLimit < 1536 {
		outputTokenLimit = 1536
	}

	payload := map[string]any{
		keyModel:              modelIdentifier,
		keyPreviousResponseID: previousResponseID,
		keyToolChoice:         toolChoiceNone,
		keyInput:              synthesisInstructionPrimary,
		keyMaxOutputTokens:    outputTokenLimit,
		keyReasoning: map[string]any{
			keyEffort: reasoningEffortMinimal,
		},
		keyText: map[string]any{
			keyFormat:    map[string]any{keyType: textFormatType},
			keyVerbosity: verbosityLow,
		},
	}
	return client.startContinuationResponse(parentContext, openAIKey, payload, structuredLogger)
}

func (client *OpenAIClient) startIncompleteContinuation(parentContext context.Context, openAIKey string, previousResponseID string, modelIdentifier string, webSearchEnabled bool, structuredLogger *zap.SugaredLogger) (string, error) {
	outputTokenLimit := client.maxOutputTokens
	if outputTokenLimit < 1536 {
		outputTokenLimit = 1536
	}

	payload := buildStatefulContinuationPayload(modelIdentifier, continuationInstructionPrimary, webSearchEnabled, outputTokenLimit, previousResponseID)
	return client.startContinuationResponse(parentContext, openAIKey, payload, structuredLogger)
}

func (client *OpenAIClient) startContinuationResponse(parentContext context.Context, openAIKey string, payload map[string]any, structuredLogger *zap.SugaredLogger) (string, error) {
	payloadBytes, _ := json.Marshal(payload)

	requestContext, cancelRequest := context.WithTimeout(parentContext, client.requestTimeout)
	defer cancelRequest()
	request, _ := buildAuthorizedJSONRequest(requestContext, http.MethodPost, client.endpoints.GetResponsesURL(), openAIKey, bytes.NewReader(payloadBytes))

	statusCode, responseBytes, _, requestError := client.performResponsesRequest(request, structuredLogger, logEventOpenAIRequestError)
	if requestError != nil {
		return constants.EmptyString, requestError
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return constants.EmptyString, errors.New(errorOpenAIAPI)
	}

	var decodedResponse map[string]any
	if json.Unmarshal(responseBytes, &decodedResponse) != nil {
		return constants.EmptyString, errors.New(errorOpenAIAPI)
	}
	newID := utils.GetString(decodedResponse, jsonFieldID)
	if utils.IsBlank(newID) {
		return constants.EmptyString, errors.New(errorOpenAIAPI)
	}
	return newID, nil
}

func buildStatefulContinuationPayload(modelIdentifier string, inputText string, webSearchEnabled bool, maxTokens int, previousResponseID string) map[string]any {
	payloadBytes, _ := json.Marshal(BuildRequestPayload(modelIdentifier, inputText, webSearchEnabled, maxTokens))
	var payload map[string]any
	_ = json.Unmarshal(payloadBytes, &payload)
	payload[keyPreviousResponseID] = previousResponseID
	return payload
}

func canContinueIncompleteResponse(decodedObject map[string]any) bool {
	details, ok := decodedObject[jsonFieldIncompleteDetails].(map[string]any)
	if !ok {
		return false
	}
	reason := utils.GetString(details, jsonFieldReason)
	switch reason {
	case incompleteReasonMaxTokens, incompleteReasonMaxOutputTokens:
		return true
	default:
		return false
	}
}

// pollResponseUntilDone repeatedly fetches a response until it is complete or the poll timeout elapses.
func (client *OpenAIClient) pollResponseUntilDone(parentContext context.Context, openAIKey string, responseIdentifier string, structuredLogger *zap.SugaredLogger) (string, error) {
	pollContext, cancelPoll := context.WithTimeout(parentContext, client.upstreamPollTimeout)
	defer cancelPoll()
	for {
		textCandidate, responseComplete, fetchError := client.fetchResponseByID(pollContext, openAIKey, responseIdentifier, structuredLogger)
		if fetchError != nil {
			if parentContext.Err() != nil {
				return constants.EmptyString, parentContext.Err()
			}
			if pollContext.Err() != nil {
				return constants.EmptyString, ErrUpstreamIncomplete
			}
			return constants.EmptyString, fetchError
		}
		if responseComplete && !utils.IsBlank(textCandidate) {
			return textCandidate, nil
		}
		if responseComplete {
			return constants.EmptyString, errors.New(errorOpenAIAPINoText)
		}
		select {
		case <-time.After(responsePollInterval):
		case <-pollContext.Done():
			if parentContext.Err() != nil {
				return constants.EmptyString, parentContext.Err()
			}
			return constants.EmptyString, ErrUpstreamIncomplete
		}
	}
}

// fetchResponseByID retrieves a response by identifier and reports whether the response is complete.
func (client *OpenAIClient) fetchResponseByID(parentContext context.Context, openAIKey string, responseIdentifier string, structuredLogger *zap.SugaredLogger) (string, bool, error) {
	resourceURL := client.endpoints.GetResponsesURL() + "/" + responseIdentifier
	requestContext, cancel := context.WithTimeout(parentContext, client.requestTimeout)
	defer cancel()

	httpRequest, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodGet, resourceURL, openAIKey, nil)
	if buildError != nil {
		return constants.EmptyString, false, buildError
	}

	statusCode, responseBytes, _, requestError := client.performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIPollError)
	if requestError != nil {
		return constants.EmptyString, false, requestError
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		structuredLogger.Desugar().Error(
			errorOpenAIAPI,
			zap.Int(logFieldStatus, statusCode),
			zap.ByteString(logFieldResponseBody, responseBytes),
			zap.String(logFieldID, responseIdentifier),
		)
		return constants.EmptyString, false, errors.New(errorOpenAIAPI)
	}

	structuredLogger.Debugw(
		logEventOpenAIPollResponseBody,
		logFieldID, responseIdentifier,
		logFieldResponseBody, string(responseBytes),
	)

	var decodedObject map[string]any
	_ = json.Unmarshal(responseBytes, &decodedObject)
	responseStatus := strings.ToLower(utils.GetString(decodedObject, jsonFieldStatus))
	outputText := extractTextFromAny(responseBytes)

	switch responseStatus {
	case statusCompleted, statusSucceeded, statusDone:
		return outputText, true, nil
	case statusCancelled, statusFailed, statusErrored:
		return constants.EmptyString, true, errors.New(errorOpenAIFailedStatus)
	case statusIncomplete:
		if !utils.IsBlank(outputText) {
			return outputText, true, nil
		}
		return constants.EmptyString, true, ErrUpstreamIncomplete
	default:
		return constants.EmptyString, false, nil
	}
}

// --- Final, Corrected Response Parser ---
type outputItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content []contentPart   `json:"content"`
	Action  json.RawMessage `json:"action"`
}
type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type searchAction struct {
	Query string `json:"query"`
}

// joinParts creates a single string by joining the trimmed text from each
// provided content part using a line break when multiple parts contain text.
func joinParts(parts []contentPart) string {
	var builder strings.Builder
	for _, part := range parts {
		if part.Type == outputPartType || part.Type == textPartType {
			text := strings.TrimSpace(part.Text)
			if text != constants.EmptyString {
				if builder.Len() > 0 {
					builder.WriteString(constants.LineBreak)
				}
				builder.WriteString(text)
			}
		}
	}
	return builder.String()
}

// extractTextFromAny parses the final response from OpenAI.
func extractTextFromAny(rawPayload []byte) string {
	var envelope struct {
		OutputText string            `json:"output_text"`
		Output     []json.RawMessage `json:"output"` // Use json.RawMessage for resilience
	}

	if json.Unmarshal(rawPayload, &envelope) != nil {
		return constants.EmptyString
	}

	// 1. Prioritize `output_text` as the most reliable source.
	if !utils.IsBlank(envelope.OutputText) {
		return envelope.OutputText
	}

	// 2. If `output_text` is missing, parse the `output` array for the assistant's message.
	if len(envelope.Output) > 0 {
		for _, rawItem := range envelope.Output {
			var header struct {
				Type string `json:"type"`
				Role string `json:"role"`
			}
			if json.Unmarshal(rawItem, &header) == nil && header.Type == responseTypeMessage && header.Role == responseRoleAssistant {
				var msgItem outputItem
				if json.Unmarshal(rawItem, &msgItem) == nil {
					return joinParts(msgItem.Content)
				}
			}
		}
	}

	// 3. If no message was found, create a fallback from the last tool call.
	if len(envelope.Output) > 0 {
		lastQuery := constants.EmptyString
		for outputIndex := len(envelope.Output) - 1; outputIndex >= 0; outputIndex-- {
			rawItem := envelope.Output[outputIndex]
			var header struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(rawItem, &header) == nil && header.Type == responseTypeWebSearchCall {
				var searchItem struct {
					Action searchAction `json:"action"`
				}
				if json.Unmarshal(rawItem, &searchItem) == nil && !utils.IsBlank(searchItem.Action.Query) {
					lastQuery = searchItem.Action.Query
					break
				}
			}
		}
		if !utils.IsBlank(lastQuery) {
			return fmt.Sprintf(fallbackFinalAnswerFormat, lastQuery)
		}
	}

	return constants.EmptyString
}

// --- HTTP and Helper Functions ---
func (client *OpenAIClient) performResponsesRequest(httpRequest *http.Request, structuredLogger *zap.SugaredLogger, logEvent string) (int, []byte, int64, error) {
	var statusCode int
	var responseBytes []byte
	var latencyMillis int64
	operation := func() error {
		var transportError error
		statusCode, responseBytes, latencyMillis, transportError = utils.PerformHTTPRequest(client.httpClient.Do, httpRequest, structuredLogger, logEvent)
		if transportError != nil {
			return transportError
		}
		// Retry on server errors (5xx) and rate limit errors (429).
		if statusCode >= http.StatusInternalServerError || statusCode == http.StatusTooManyRequests {
			return errors.New(errorOpenAIAPI)
		}
		return nil
	}
	retryStrategy := utils.AcquireExponentialBackoff()
	defer utils.ReleaseExponentialBackoff(retryStrategy)
	retryError := backoff.Retry(operation, backoff.WithContext(retryStrategy, httpRequest.Context()))
	return statusCode, responseBytes, latencyMillis, retryError
}

func buildAuthorizedJSONRequest(contextToUse context.Context, method string, resourceURL string, openAIKey string, body io.Reader) (*http.Request, error) {
	httpReq, httpRequestError := http.NewRequestWithContext(contextToUse, method, resourceURL, body)
	if httpRequestError != nil {
		return nil, httpRequestError
	}
	httpReq.Header.Set(headerAuthorization, headerAuthorizationPrefix+openAIKey)
	if body != nil {
		httpReq.Header.Set(headerContentType, mimeApplicationJSON)
	}
	return httpReq, nil
}
