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
	httpClient HTTPDoer
	endpoints  *Endpoints
}

// NewOpenAIClient constructs an OpenAIClient initialized with the supplied components.
func NewOpenAIClient(httpClient HTTPDoer, endpoints *Endpoints) *OpenAIClient {
	return &OpenAIClient{
		httpClient: httpClient,
		endpoints:  endpoints,
	}
}

const (
	synthesisInstructionPrimary = "Now synthesize the final answer with concise citations."
	responsePollInterval        = 500 * time.Millisecond
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
func (client *OpenAIClient) openAIRequest(parentContext context.Context, openAIKey string, modelIdentifier textModelDefinition, messages chatMessages, webSearchEnabled bool, maxTokens *int, reasoningEffort string, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	payload := BuildRequestPayload(modelIdentifier.string(), modelIdentifier.requestProfile.string(), messages.openAIResponsesInput(), webSearchEnabled, maxTokens, reasoningEffort)
	payloadBytes, _ := json.Marshal(payload)

	httpRequest, buildError := buildAuthorizedJSONRequest(parentContext, http.MethodPost, client.endpoints.GetResponsesURL(), openAIKey, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return textGenerationResult{}, buildError
	}

	statusCode, responseBytes, _, latencyMillis, requestError := client.performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIRequestError)
	if requestError != nil {
		return textGenerationResult{}, requestError
	}

	responseSnapshot, snapshotError := newOpenAIResponseSnapshot(responseBytes)

	structuredLogger.Infow(
		logEventOpenAIResponse,
		logFieldHTTPStatus, statusCode,
		logFieldAPIStatus, responseSnapshot.status,
		constants.LogFieldLatencyMilliseconds, latencyMillis,
	)

	if snapshotError != nil {
		return textGenerationResult{}, errors.New(errorOpenAIAPI)
	}

	return client.resolveOpenAIResponse(parentContext, openAIKey, modelIdentifier, webSearchEnabled, maxTokens, reasoningEffort, responseSnapshot, structuredLogger)
}

type openAIResponseSnapshot struct {
	identifier            string
	status                string
	incompleteReason      string
	text                  string
	usage                 *tokenUsage
	hasFinalMessage       bool
	hasTopLevelOutputText bool
}

func newOpenAIResponseSnapshot(responseBytes []byte) (openAIResponseSnapshot, error) {
	var decodedObject map[string]any
	decodeError := json.Unmarshal(responseBytes, &decodedObject)
	incompleteReason := constants.EmptyString
	if incompleteDetails, ok := decodedObject["incomplete_details"].(map[string]any); ok {
		incompleteReason = utils.GetString(incompleteDetails, "reason")
	}
	usage, usageError := parseResponsesTokenUsage(responseBytes)
	responseSnapshot := openAIResponseSnapshot{
		identifier:            utils.GetString(decodedObject, jsonFieldID),
		status:                utils.GetString(decodedObject, jsonFieldStatus),
		incompleteReason:      incompleteReason,
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
	case statusCompleted, statusCancelled, statusFailed, statusIncomplete:
		return true
	default:
		return false
	}
}

func (responseSnapshot openAIResponseSnapshot) isPending() bool {
	switch responseSnapshot.status {
	case statusQueued, statusInProgress:
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

func (client *OpenAIClient) resolveOpenAIResponse(parentContext context.Context, openAIKey string, modelIdentifier textModelDefinition, webSearchEnabled bool, maxTokens *int, reasoningEffort string, responseSnapshot openAIResponseSnapshot, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	if responseSnapshot.isTerminal() {
		return client.resolveTerminalOpenAIResponse(parentContext, openAIKey, modelIdentifier, webSearchEnabled, maxTokens, reasoningEffort, responseSnapshot, structuredLogger)
	}
	if !responseSnapshot.isPending() {
		return client.resolveTerminalOpenAIResponse(parentContext, openAIKey, modelIdentifier, webSearchEnabled, maxTokens, reasoningEffort, responseSnapshot, structuredLogger)
	}
	if utils.IsBlank(responseSnapshot.identifier) {
		return textGenerationResult{usage: responseSnapshot.usage}, errors.New(errorOpenAIAPI)
	}
	finalGeneration, pollError := client.pollResponseUntilDone(parentContext, openAIKey, responseSnapshot.identifier, responseSnapshot.usage, modelIdentifier, webSearchEnabled, maxTokens, reasoningEffort, structuredLogger)
	if pollError != nil {
		if !errors.Is(pollError, errProviderOutputLimitReached) {
			structuredLogger.Errorw(
				logEventOpenAIPollError,
				logFieldID, responseSnapshot.identifier,
				constants.LogFieldError, pollError,
			)
		}
		return finalGeneration, openAIStageError(pollError)
	}
	return finalGeneration, nil
}

func (client *OpenAIClient) resolveTerminalOpenAIResponse(parentContext context.Context, openAIKey string, modelIdentifier textModelDefinition, webSearchEnabled bool, maxTokens *int, reasoningEffort string, responseSnapshot openAIResponseSnapshot, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	switch responseSnapshot.status {
	case statusCompleted:
		return client.resolveCompleteOpenAIResponse(parentContext, openAIKey, modelIdentifier, webSearchEnabled, maxTokens, reasoningEffort, responseSnapshot, structuredLogger)
	case statusCancelled, statusFailed:
		return textGenerationResult{usage: responseSnapshot.usage}, errors.New(errorOpenAIFailedStatus)
	case statusIncomplete:
		if responseSnapshot.incompleteReason != "max_output_tokens" {
			return textGenerationResult{usage: responseSnapshot.usage}, fmt.Errorf("%w: Responses incomplete reason=%s", ErrProviderAPI, responseSnapshot.incompleteReason)
		}
		return responseSnapshot.generation(), errProviderOutputLimitReached
	}
	return textGenerationResult{usage: responseSnapshot.usage}, errors.New(errorOpenAIAPI)
}

func (client *OpenAIClient) resolveCompleteOpenAIResponse(parentContext context.Context, openAIKey string, modelIdentifier textModelDefinition, webSearchEnabled bool, maxTokens *int, reasoningEffort string, responseSnapshot openAIResponseSnapshot, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	if responseSnapshot.needsSynthesis() && !utils.IsBlank(responseSnapshot.identifier) {
		structuredLogger.Debugw(logEventMissingFinalMessage)
		continuedResponseID, synthErr := client.startSynthesisContinuation(parentContext, openAIKey, responseSnapshot.identifier, modelIdentifier.string(), maxTokens, reasoningEffort, structuredLogger)
		if synthErr != nil {
			structuredLogger.Errorw(
				logEventOpenAIContinueError,
				logFieldID, responseSnapshot.identifier,
				constants.LogFieldError, synthErr,
			)
			return textGenerationResult{}, openAIStageError(synthErr)
		}
		finalGeneration, pollError := client.pollResponseUntilDone(parentContext, openAIKey, continuedResponseID, nil, modelIdentifier, webSearchEnabled, maxTokens, reasoningEffort, structuredLogger)
		finalGeneration.usage = mergeTokenUsage(responseSnapshot.usage, finalGeneration.usage)
		if pollError != nil {
			if !errors.Is(pollError, errProviderOutputLimitReached) {
				structuredLogger.Errorw(
					logEventOpenAIPollError,
					logFieldID, continuedResponseID,
					constants.LogFieldError, pollError,
				)
			}
			return finalGeneration, openAIStageError(pollError)
		}
		return finalGeneration, nil
	}
	if utils.IsBlank(responseSnapshot.text) {
		return textGenerationResult{}, errors.New(errorOpenAIAPI)
	}
	return responseSnapshot.generation(), nil
}

func openAIStageError(stageError error) error {
	if errors.Is(stageError, context.Canceled) || errors.Is(stageError, context.DeadlineExceeded) || errors.Is(stageError, errProviderOutputLimitReached) {
		return stageError
	}
	if _, _, _, hasHTTPMetadata := providerHTTPMetadata(stageError); hasHTTPMetadata {
		return stageError
	}
	return errors.New(errorOpenAIAPI)
}

// startSynthesisContinuation begins a synthesis-only pass by POSTing /v1/responses with
// previous_response_id and tool_choice set to "none". It retains the tenant-selected
// reasoning effort when one is configured and includes a low-verbosity text format hint.
// It returns the identifier of the new response.
func (client *OpenAIClient) startSynthesisContinuation(parentContext context.Context, openAIKey string, previousResponseID string, modelIdentifier string, maxTokens *int, reasoningEffort string, structuredLogger *zap.SugaredLogger) (string, error) {
	payload := map[string]any{
		keyModel:              modelIdentifier,
		keyPreviousResponseID: previousResponseID,
		keyBackground:         true,
		keyStore:              true,
		keyToolChoice:         toolChoiceNone,
		keyInput:              synthesisInstructionPrimary,
		keyText: map[string]any{
			keyFormat:    map[string]any{keyType: textFormatType},
			keyVerbosity: verbosityLow,
		},
	}
	if strings.TrimSpace(reasoningEffort) != constants.EmptyString {
		payload[keyReasoning] = map[string]any{keyEffort: reasoningEffort}
	}
	if maxTokens != nil {
		payload[keyMaxOutputTokens] = *maxTokens
	}
	return client.startContinuationResponse(parentContext, openAIKey, payload, structuredLogger)
}

func (client *OpenAIClient) startContinuationResponse(parentContext context.Context, openAIKey string, payload map[string]any, structuredLogger *zap.SugaredLogger) (string, error) {
	payloadBytes, _ := json.Marshal(payload)

	request, _ := buildAuthorizedJSONRequest(parentContext, http.MethodPost, client.endpoints.GetResponsesURL(), openAIKey, bytes.NewReader(payloadBytes))

	_, responseBytes, _, _, requestError := client.performResponsesRequest(request, structuredLogger, logEventOpenAIRequestError)
	if requestError != nil {
		return constants.EmptyString, requestError
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

// pollResponseUntilDone repeatedly fetches a response until it is complete or the request context expires.
func (client *OpenAIClient) pollResponseUntilDone(parentContext context.Context, openAIKey string, responseIdentifier string, latestUsage *tokenUsage, modelIdentifier textModelDefinition, webSearchEnabled bool, maxTokens *int, reasoningEffort string, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	for {
		responseSnapshot, responseComplete, fetchError := client.fetchResponseByID(parentContext, openAIKey, responseIdentifier, structuredLogger)
		if responseSnapshot.usage != nil {
			latestUsage = responseSnapshot.usage
		}
		if fetchError != nil {
			if parentContext.Err() != nil {
				return textGenerationResult{usage: latestUsage}, parentContext.Err()
			}
			return textGenerationResult{usage: latestUsage}, fetchError
		}
		if responseComplete {
			responseSnapshot.usage = latestUsage
			return client.resolveTerminalOpenAIResponse(parentContext, openAIKey, modelIdentifier, webSearchEnabled, maxTokens, reasoningEffort, responseSnapshot, structuredLogger)
		}
		select {
		case <-time.After(responsePollInterval):
		case <-parentContext.Done():
			return textGenerationResult{usage: latestUsage}, parentContext.Err()
		}
	}
}

// fetchResponseByID retrieves a response by identifier and reports whether the response is complete.
func (client *OpenAIClient) fetchResponseByID(parentContext context.Context, openAIKey string, responseIdentifier string, structuredLogger *zap.SugaredLogger) (openAIResponseSnapshot, bool, error) {
	resourceURL := client.endpoints.GetResponsesURL() + "/" + responseIdentifier
	httpRequest, buildError := buildAuthorizedJSONRequest(parentContext, http.MethodGet, resourceURL, openAIKey, nil)
	if buildError != nil {
		return openAIResponseSnapshot{}, false, buildError
	}

	_, responseBytes, _, _, requestError := client.performResponsesRequest(httpRequest, structuredLogger, logEventOpenAIPollError)
	if requestError != nil {
		return openAIResponseSnapshot{}, false, requestError
	}

	responseSnapshot, snapshotError := newOpenAIResponseSnapshot(responseBytes)
	if snapshotError != nil {
		return responseSnapshot, false, errors.New(errorOpenAIAPI)
	}
	if responseSnapshot.isTerminal() {
		return responseSnapshot, true, nil
	}
	if !responseSnapshot.isPending() {
		return responseSnapshot, false, errors.New(errorOpenAIAPI)
	}
	return responseSnapshot, false, nil
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

// joinParts creates a single string from visible content while preserving each
// part's boundary whitespace for the completion coordinator.
func joinParts(parts []contentPart) string {
	var builder strings.Builder
	for _, part := range parts {
		if part.Type == outputPartType || part.Type == textPartType {
			if !utils.IsBlank(part.Text) {
				if builder.Len() > 0 {
					builder.WriteString(constants.LineBreak)
				}
				builder.WriteString(part.Text)
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
func (client *OpenAIClient) performResponsesRequest(httpRequest *http.Request, structuredLogger *zap.SugaredLogger, logEvent string) (int, []byte, http.Header, int64, error) {
	var statusCode int
	var responseBytes []byte
	var responseHeader http.Header
	var latencyMillis int64
	operation := func() error {
		var transportError error
		statusCode, responseBytes, responseHeader, latencyMillis, transportError = utils.PerformHTTPRequest(client.httpClient.Do, httpRequest, structuredLogger, logEvent)
		responseError := providerResponseError(statusCode, responseHeader, transportError)
		if _, _, _, hasHTTPMetadata := providerHTTPMetadata(responseError); !hasHTTPMetadata {
			if responseError == nil {
				return nil
			}
			if errors.Is(transportError, context.Canceled) || errors.Is(transportError, context.DeadlineExceeded) || errors.Is(transportError, errQueueFull) {
				return backoff.Permanent(responseError)
			}
			return responseError
		}
		if statusCode >= http.StatusInternalServerError || statusCode == http.StatusTooManyRequests {
			return responseError
		}
		return backoff.Permanent(responseError)
	}
	retryStrategy := utils.AcquireExponentialBackoff()
	defer utils.ReleaseExponentialBackoff(retryStrategy)
	retryError := backoff.Retry(operation, backoff.WithContext(retryStrategy, httpRequest.Context()))
	return statusCode, responseBytes, responseHeader, latencyMillis, retryError
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
