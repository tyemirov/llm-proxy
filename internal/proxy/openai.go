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
	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/utils"
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
	httpClient     HTTPDoer
	endpoints      *Endpoints
	requestTimeout time.Duration
}

// NewOpenAIClient constructs an OpenAIClient initialized with the supplied components.
func NewOpenAIClient(httpClient HTTPDoer, endpoints *Endpoints, requestTimeout time.Duration) *OpenAIClient {
	return &OpenAIClient{
		httpClient:     httpClient,
		endpoints:      endpoints,
		requestTimeout: requestTimeout,
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

// openAIRequest sends messages to the OpenAI responses API and returns the resulting text.
func (client *OpenAIClient) openAIRequest(parentContext context.Context, openAIKey string, modelIdentifier textModelDefinition, messages chatMessages, webSearchEnabled bool, maxTokens *int, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	payload := BuildRequestPayload(modelIdentifier.string(), modelIdentifier.requestProfile.string(), messages.openAIResponsesInput(), webSearchEnabled, maxTokens)
	payloadBytes, _ := json.Marshal(payload)

	requestContext, cancelRequest := context.WithTimeout(parentContext, client.requestTimeout)
	defer cancelRequest()
	httpRequest, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodPost, client.endpoints.GetResponsesURL(), openAIKey, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return textGenerationResult{}, buildError
	}

	statusCode, responseBytes, latencyMillis, requestError := client.performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIRequestError)
	if requestError != nil {
		return textGenerationResult{}, requestError
	}

	structuredLogger.Debugw(logEventOpenAIInitialResponseBody, logFieldResponseBody, string(responseBytes))

	responseSnapshot, snapshotError := newOpenAIResponseSnapshot(responseBytes)

	structuredLogger.Infow(
		logEventOpenAIResponse,
		logFieldHTTPStatus, statusCode,
		logFieldAPIStatus, responseSnapshot.status,
		constants.LogFieldLatencyMilliseconds, latencyMillis,
		logFieldResponseText, responseSnapshot.text,
	)

	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		structuredLogger.Desugar().Error(
			errorOpenAIAPI,
			zap.Int(logFieldStatus, statusCode),
			zap.ByteString(logFieldResponseBody, responseBytes),
		)
		return textGenerationResult{}, errors.New(errorOpenAIAPI)
	}
	if snapshotError != nil {
		return textGenerationResult{}, errors.New(errorOpenAIAPI)
	}

	return client.resolveOpenAIResponse(requestContext, openAIKey, modelIdentifier, webSearchEnabled, maxTokens, responseSnapshot, structuredLogger)
}

type openAIResponseSnapshot struct {
	decodedObject         map[string]any
	identifier            string
	status                string
	text                  string
	usage                 *tokenUsage
	hasFinalMessage       bool
	hasTopLevelOutputText bool
}

func newOpenAIResponseSnapshot(responseBytes []byte) (openAIResponseSnapshot, error) {
	var decodedObject map[string]any
	decodeError := json.Unmarshal(responseBytes, &decodedObject)
	usage, usageError := parseResponsesTokenUsage(responseBytes)
	responseSnapshot := openAIResponseSnapshot{
		decodedObject:         decodedObject,
		identifier:            utils.GetString(decodedObject, jsonFieldID),
		status:                strings.ToLower(utils.GetString(decodedObject, jsonFieldStatus)),
		text:                  extractTextFromAny(responseBytes),
		usage:                 usage,
		hasFinalMessage:       hasFinalMessage(responseBytes),
		hasTopLevelOutputText: !utils.IsBlank(utils.GetString(decodedObject, jsonFieldOutputText)),
	}
	if decodeError != nil {
		return responseSnapshot, decodeError
	}
	if usageError != nil {
		return responseSnapshot, usageError
	}
	return responseSnapshot, nil
}

func (responseSnapshot openAIResponseSnapshot) isTerminal() bool {
	switch responseSnapshot.status {
	case statusCompleted, statusSucceeded, statusDone, statusCancelled, statusFailed, statusErrored, statusIncomplete:
		return true
	default:
		return false
	}
}

func (responseSnapshot openAIResponseSnapshot) needsSynthesis() bool {
	return responseSnapshot.status == statusCompleted && !responseSnapshot.hasFinalMessage && !responseSnapshot.hasTopLevelOutputText
}

func (responseSnapshot openAIResponseSnapshot) generation() textGenerationResult {
	return textGenerationResult{text: responseSnapshot.text, usage: responseSnapshot.usage}
}

func (client *OpenAIClient) resolveOpenAIResponse(parentContext context.Context, openAIKey string, modelIdentifier textModelDefinition, webSearchEnabled bool, maxTokens *int, responseSnapshot openAIResponseSnapshot, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	if !responseSnapshot.isTerminal() && !utils.IsBlank(responseSnapshot.identifier) {
		finalGeneration, pollError := client.pollResponseUntilDone(parentContext, openAIKey, responseSnapshot.identifier, modelIdentifier, webSearchEnabled, maxTokens, structuredLogger)
		if pollError != nil {
			structuredLogger.Errorw(
				logEventOpenAIPollError,
				logFieldID, responseSnapshot.identifier,
				constants.LogFieldError, pollError,
			)
			return textGenerationResult{}, openAIStageError(pollError)
		}
		if !utils.IsBlank(finalGeneration.text) {
			finalGeneration.usage = mergeTokenUsage(responseSnapshot.usage, finalGeneration.usage)
			return finalGeneration, nil
		}
	}
	if !responseSnapshot.isTerminal() {
		if utils.IsBlank(responseSnapshot.text) {
			return textGenerationResult{}, errors.New(errorOpenAIAPI)
		}
		return responseSnapshot.generation(), nil
	}
	return client.resolveTerminalOpenAIResponse(parentContext, openAIKey, modelIdentifier, webSearchEnabled, maxTokens, responseSnapshot, structuredLogger)
}

func (client *OpenAIClient) resolveTerminalOpenAIResponse(parentContext context.Context, openAIKey string, modelIdentifier textModelDefinition, webSearchEnabled bool, maxTokens *int, responseSnapshot openAIResponseSnapshot, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	switch responseSnapshot.status {
	case statusCancelled, statusFailed, statusErrored:
		return textGenerationResult{}, errors.New(errorOpenAIFailedStatus)
	case statusIncomplete:
		return client.resolveIncompleteOpenAIResponse(parentContext, openAIKey, modelIdentifier, webSearchEnabled, maxTokens, responseSnapshot, structuredLogger)
	default:
		return client.resolveCompleteOpenAIResponse(parentContext, openAIKey, modelIdentifier, webSearchEnabled, maxTokens, responseSnapshot, structuredLogger)
	}
}

func (client *OpenAIClient) resolveIncompleteOpenAIResponse(parentContext context.Context, openAIKey string, modelIdentifier textModelDefinition, webSearchEnabled bool, maxTokens *int, responseSnapshot openAIResponseSnapshot, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	if !utils.IsBlank(responseSnapshot.text) {
		return responseSnapshot.generation(), nil
	}
	if !canContinueIncompleteResponse(responseSnapshot.decodedObject) || utils.IsBlank(responseSnapshot.identifier) {
		return textGenerationResult{}, ErrUpstreamIncomplete
	}
	continuedResponseID, continuationError := client.startIncompleteContinuation(parentContext, openAIKey, responseSnapshot.identifier, modelIdentifier, webSearchEnabled, maxTokens, structuredLogger)
	if continuationError != nil {
		structuredLogger.Errorw(
			logEventOpenAIContinueError,
			logFieldID, responseSnapshot.identifier,
			constants.LogFieldError, continuationError,
		)
		return textGenerationResult{}, openAIStageError(continuationError)
	}
	finalGeneration, pollError := client.pollResponseUntilDone(parentContext, openAIKey, continuedResponseID, modelIdentifier, webSearchEnabled, maxTokens, structuredLogger)
	if pollError != nil {
		structuredLogger.Errorw(
			logEventOpenAIPollError,
			logFieldID, continuedResponseID,
			constants.LogFieldError, pollError,
		)
		return textGenerationResult{}, openAIStageError(pollError)
	}
	finalGeneration.usage = mergeTokenUsage(responseSnapshot.usage, finalGeneration.usage)
	return finalGeneration, nil
}

func (client *OpenAIClient) resolveCompleteOpenAIResponse(parentContext context.Context, openAIKey string, modelIdentifier textModelDefinition, webSearchEnabled bool, maxTokens *int, responseSnapshot openAIResponseSnapshot, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	if responseSnapshot.needsSynthesis() && !utils.IsBlank(responseSnapshot.identifier) {
		structuredLogger.Debugw(logEventMissingFinalMessage)
		continuedResponseID, synthErr := client.startSynthesisContinuation(parentContext, openAIKey, responseSnapshot.identifier, modelIdentifier.string(), maxTokens, structuredLogger)
		if synthErr != nil {
			structuredLogger.Errorw(
				logEventOpenAIContinueError,
				logFieldID, responseSnapshot.identifier,
				constants.LogFieldError, synthErr,
			)
			return textGenerationResult{}, openAIStageError(synthErr)
		}
		finalGeneration, pollError := client.pollResponseUntilDone(parentContext, openAIKey, continuedResponseID, modelIdentifier, webSearchEnabled, maxTokens, structuredLogger)
		if pollError != nil {
			structuredLogger.Errorw(
				logEventOpenAIPollError,
				logFieldID, continuedResponseID,
				constants.LogFieldError, pollError,
			)
			return textGenerationResult{}, openAIStageError(pollError)
		}
		finalGeneration.usage = mergeTokenUsage(responseSnapshot.usage, finalGeneration.usage)
		return finalGeneration, nil
	}
	if utils.IsBlank(responseSnapshot.text) {
		return textGenerationResult{}, errors.New(errorOpenAIAPI)
	}
	return responseSnapshot.generation(), nil
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
func (client *OpenAIClient) startSynthesisContinuation(parentContext context.Context, openAIKey string, previousResponseID string, modelIdentifier string, maxTokens *int, structuredLogger *zap.SugaredLogger) (string, error) {
	payload := map[string]any{
		keyModel:              modelIdentifier,
		keyPreviousResponseID: previousResponseID,
		keyBackground:         true,
		keyStore:              true,
		keyToolChoice:         toolChoiceNone,
		keyInput:              synthesisInstructionPrimary,
		keyReasoning: map[string]any{
			keyEffort: reasoningEffortMinimal,
		},
		keyText: map[string]any{
			keyFormat:    map[string]any{keyType: textFormatType},
			keyVerbosity: verbosityLow,
		},
	}
	if maxTokens != nil {
		payload[keyMaxOutputTokens] = *maxTokens
	}
	return client.startContinuationResponse(parentContext, openAIKey, payload, structuredLogger)
}

func (client *OpenAIClient) startIncompleteContinuation(parentContext context.Context, openAIKey string, previousResponseID string, modelIdentifier textModelDefinition, webSearchEnabled bool, maxTokens *int, structuredLogger *zap.SugaredLogger) (string, error) {
	payload := buildStatefulContinuationPayload(modelIdentifier, continuationInstructionPrimary, webSearchEnabled, maxTokens, previousResponseID)
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

func buildStatefulContinuationPayload(modelIdentifier textModelDefinition, inputText string, webSearchEnabled bool, maxTokens *int, previousResponseID string) map[string]any {
	payloadBytes, _ := json.Marshal(BuildRequestPayload(modelIdentifier.string(), modelIdentifier.requestProfile.string(), inputText, webSearchEnabled, maxTokens))
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

// pollResponseUntilDone repeatedly fetches a response until it is complete or the request context expires.
func (client *OpenAIClient) pollResponseUntilDone(parentContext context.Context, openAIKey string, responseIdentifier string, modelIdentifier textModelDefinition, webSearchEnabled bool, maxTokens *int, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	for {
		responseSnapshot, responseComplete, fetchError := client.fetchResponseByID(parentContext, openAIKey, responseIdentifier, structuredLogger)
		if fetchError != nil {
			if parentContext.Err() != nil {
				return textGenerationResult{}, parentContext.Err()
			}
			return textGenerationResult{}, fetchError
		}
		if responseComplete {
			return client.resolveTerminalOpenAIResponse(parentContext, openAIKey, modelIdentifier, webSearchEnabled, maxTokens, responseSnapshot, structuredLogger)
		}
		select {
		case <-time.After(responsePollInterval):
		case <-parentContext.Done():
			return textGenerationResult{}, parentContext.Err()
		}
	}
}

// fetchResponseByID retrieves a response by identifier and reports whether the response is complete.
func (client *OpenAIClient) fetchResponseByID(parentContext context.Context, openAIKey string, responseIdentifier string, structuredLogger *zap.SugaredLogger) (openAIResponseSnapshot, bool, error) {
	resourceURL := client.endpoints.GetResponsesURL() + "/" + responseIdentifier
	requestContext, cancel := context.WithTimeout(parentContext, client.requestTimeout)
	defer cancel()

	httpRequest, buildError := buildAuthorizedJSONRequest(requestContext, http.MethodGet, resourceURL, openAIKey, nil)
	if buildError != nil {
		return openAIResponseSnapshot{}, false, buildError
	}

	statusCode, responseBytes, _, requestError := client.performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIPollError)
	if requestError != nil {
		return openAIResponseSnapshot{}, false, requestError
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		structuredLogger.Desugar().Error(
			errorOpenAIAPI,
			zap.Int(logFieldStatus, statusCode),
			zap.ByteString(logFieldResponseBody, responseBytes),
			zap.String(logFieldID, responseIdentifier),
		)
		return openAIResponseSnapshot{}, false, errors.New(errorOpenAIAPI)
	}

	structuredLogger.Debugw(
		logEventOpenAIPollResponseBody,
		logFieldID, responseIdentifier,
		logFieldResponseBody, string(responseBytes),
	)

	responseSnapshot, snapshotError := newOpenAIResponseSnapshot(responseBytes)
	if snapshotError != nil {
		return openAIResponseSnapshot{}, false, errors.New(errorOpenAIAPI)
	}
	return responseSnapshot, responseSnapshot.isTerminal(), nil
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
			if errors.Is(transportError, context.Canceled) || errors.Is(transportError, context.DeadlineExceeded) || errors.Is(transportError, errQueueFull) {
				return backoff.Permanent(transportError)
			}
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
