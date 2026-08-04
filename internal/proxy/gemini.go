package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

const (
	geminiAPIKeyHeader                    = "x-goog-api-key"
	geminiAPIRevisionHeader               = "Api-Revision"
	geminiAPIRevisionValue                = "2026-05-20"
	geminiInteractionStatusRequiresAction = "requires_action"
	geminiInteractionStatusBudgetExceeded = "budget_exceeded"
	geminiInteractionStepUserInput        = "user_input"
	geminiInteractionStepModelOutput      = "model_output"
	geminiInteractionContentText          = "text"
	geminiInteractionCleanupTimeout       = 5 * time.Second
)

type geminiInteractionsClient struct {
	httpClient HTTPDoer
}

type geminiInteractionCleanupMode uint8

const (
	geminiInteractionDeleteOnly geminiInteractionCleanupMode = iota
	geminiInteractionCancelAndDelete
)

type geminiInteractionRequest struct {
	Model             string                       `json:"model"`
	Input             []geminiInteractionStep      `json:"input"`
	SystemInstruction string                       `json:"system_instruction,omitempty"`
	GenerationConfig  *geminiInteractionGeneration `json:"generation_config,omitempty"`
	Background        bool                         `json:"background"`
	Store             bool                         `json:"store"`
}

type geminiInteractionGeneration struct {
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
}

type geminiInteractionStep struct {
	Type    string                     `json:"type"`
	Content []geminiInteractionContent `json:"content,omitempty"`
}

type geminiInteractionContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

type geminiInteractionResponse struct {
	ID     string                       `json:"id"`
	Status string                       `json:"status"`
	Steps  []geminiInteractionStep      `json:"steps"`
	Usage  *geminiInteractionTokenUsage `json:"usage"`
}

type geminiInteractionTokenUsage struct {
	TotalInputTokens  *int `json:"total_input_tokens"`
	TotalOutputTokens *int `json:"total_output_tokens"`
	TotalTokens       *int `json:"total_tokens"`
}

type geminiInteractionSnapshot struct {
	identifier string
	status     string
	text       string
	usage      *tokenUsage
}

func newGeminiInteractionsClient(httpClient HTTPDoer) *geminiInteractionsClient {
	return &geminiInteractionsClient{httpClient: httpClient}
}

func (client *geminiInteractionsClient) generateText(parentContext context.Context, apiKey string, baseURL string, modelIdentifier textModelDefinition, messages chatMessages, maxTokens *int, executionLifecycle textExecutionLifecycle, structuredLogger *zap.SugaredLogger) (generation textGenerationResult, generationError error) {
	input, systemInstruction := messages.geminiInteractionInput()
	background := executionLifecycle == textExecutionLifecyclePollableResource
	payload := geminiInteractionRequest{
		Model:             modelIdentifier.string(),
		Input:             input,
		SystemInstruction: systemInstruction,
		Background:        background,
		Store:             background,
	}
	if maxTokens != nil {
		payload.GenerationConfig = &geminiInteractionGeneration{MaxOutputTokens: *maxTokens}
	}

	snapshot, createError := client.createInteraction(parentContext, apiKey, baseURL, payload, structuredLogger)
	if createError != nil {
		if background && !utils.IsBlank(snapshot.identifier) {
			cleanupError := client.releaseInteraction(parentContext, apiKey, baseURL, snapshot.identifier, snapshot.cleanupMode(), structuredLogger)
			if cleanupError != nil {
				client.logInteractionCleanupError(cleanupError, snapshot.identifier, structuredLogger)
			}
		}
		return textGenerationResult{usage: snapshot.usage}, createError
	}
	if !background {
		if snapshot.isPending() {
			return textGenerationResult{usage: snapshot.usage}, fmt.Errorf("%w: synchronous Gemini Interaction returned active status=%s", ErrProviderAPI, snapshot.status)
		}
		return snapshot.resolve()
	}
	if utils.IsBlank(snapshot.identifier) {
		return textGenerationResult{usage: snapshot.usage}, fmt.Errorf("%w: Gemini Interactions create response missing id", ErrProviderAPI)
	}

	interactionIdentifier := snapshot.identifier
	latestUsage := snapshot.usage
	cleanupMode := snapshot.cleanupMode()
	defer func() {
		cleanupError := client.releaseInteraction(parentContext, apiKey, baseURL, interactionIdentifier, cleanupMode, structuredLogger)
		if cleanupError == nil {
			return
		}
		client.logInteractionCleanupError(cleanupError, interactionIdentifier, structuredLogger)
		if generationError == nil || errors.Is(generationError, errProviderOutputLimitReached) {
			generation = textGenerationResult{usage: generation.usage}
			generationError = cleanupError
		}
	}()

	for snapshot.isPending() {
		select {
		case <-time.After(responsePollInterval):
		case <-parentContext.Done():
			return textGenerationResult{usage: latestUsage}, parentContext.Err()
		}

		polledSnapshot, pollError := client.getInteraction(parentContext, apiKey, baseURL, interactionIdentifier, structuredLogger)
		if polledSnapshot.usage != nil {
			latestUsage = polledSnapshot.usage
		}
		if pollError != nil {
			if parentContext.Err() != nil {
				return textGenerationResult{usage: latestUsage}, parentContext.Err()
			}
			return textGenerationResult{usage: latestUsage}, pollError
		}
		snapshot = polledSnapshot
		cleanupMode = snapshot.cleanupMode()
	}

	snapshot.usage = latestUsage
	return snapshot.resolve()
}

func (client *geminiInteractionsClient) createInteraction(parentContext context.Context, apiKey string, baseURL string, payload geminiInteractionRequest, structuredLogger *zap.SugaredLogger) (geminiInteractionSnapshot, error) {
	responseBytes, requestError := client.performInteractionRequest(
		parentContext,
		http.MethodPost,
		geminiInteractionsURL(baseURL),
		apiKey,
		payload,
		structuredLogger,
	)
	if requestError != nil {
		return geminiInteractionSnapshot{}, requestError
	}
	return newGeminiInteractionSnapshot(responseBytes)
}

func (client *geminiInteractionsClient) getInteraction(parentContext context.Context, apiKey string, baseURL string, interactionIdentifier string, structuredLogger *zap.SugaredLogger) (geminiInteractionSnapshot, error) {
	responseBytes, requestError := client.performInteractionRequest(
		parentContext,
		http.MethodGet,
		geminiInteractionResourceURL(baseURL, interactionIdentifier),
		apiKey,
		nil,
		structuredLogger,
	)
	if requestError != nil {
		return geminiInteractionSnapshot{}, requestError
	}
	return newGeminiInteractionSnapshot(responseBytes)
}

func (client *geminiInteractionsClient) releaseInteraction(parentContext context.Context, apiKey string, baseURL string, interactionIdentifier string, cleanupMode geminiInteractionCleanupMode, structuredLogger *zap.SugaredLogger) error {
	cleanupContext, cancelCleanup := context.WithTimeout(context.WithoutCancel(parentContext), geminiInteractionCleanupTimeout)
	defer cancelCleanup()

	var cancelError error
	if cleanupMode == geminiInteractionCancelAndDelete {
		_, cancelError = client.performInteractionRequest(
			cleanupContext,
			http.MethodPost,
			geminiInteractionResourceURL(baseURL, interactionIdentifier)+"/cancel",
			apiKey,
			nil,
			structuredLogger,
		)
	}
	_, deleteError := client.performInteractionRequest(
		cleanupContext,
		http.MethodDelete,
		geminiInteractionResourceURL(baseURL, interactionIdentifier),
		apiKey,
		nil,
		structuredLogger,
	)
	if cancelError != nil {
		return cancelError
	}
	return deleteError
}

func (client *geminiInteractionsClient) performInteractionRequest(parentContext context.Context, method string, requestURL string, apiKey string, payload any, structuredLogger *zap.SugaredLogger) ([]byte, error) {
	var requestBody io.Reader
	if payload != nil {
		payloadBytes, _ := json.Marshal(payload)
		requestBody = bytes.NewReader(payloadBytes)
	}
	httpRequest, buildError := http.NewRequestWithContext(parentContext, method, requestURL, requestBody)
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return nil, buildError
	}
	httpRequest.Header.Set(geminiAPIKeyHeader, strings.TrimSpace(apiKey))
	httpRequest.Header.Set(geminiAPIRevisionHeader, geminiAPIRevisionValue)
	if payload != nil {
		httpRequest.Header.Set(headerContentType, mimeApplicationJSON)
	}

	statusCode, responseBytes, responseHeader, _, requestError := utils.PerformHTTPRequest(client.httpClient.Do, httpRequest, structuredLogger, logEventProviderRequestError)
	if responseError := providerResponseError(statusCode, responseHeader, requestError); responseError != nil {
		return nil, responseError
	}
	return responseBytes, nil
}

func (client *geminiInteractionsClient) logInteractionCleanupError(cleanupError error, interactionIdentifier string, structuredLogger *zap.SugaredLogger) {
	structuredLogger.Errorw(
		"Gemini interaction cleanup error",
		logFieldID, interactionIdentifier,
		constants.LogFieldError, cleanupError,
	)
}

func (messages chatMessages) geminiInteractionInput() ([]geminiInteractionStep, string) {
	input := make([]geminiInteractionStep, 0, len(messages))
	systemInstructions := []string{}
	for _, message := range messages {
		if message.role == chatRoleSystem {
			systemInstructions = append(systemInstructions, message.content)
			continue
		}

		stepType := geminiInteractionStepUserInput
		if message.role == chatRoleAssistant {
			stepType = geminiInteractionStepModelOutput
		}
		content := []geminiInteractionContent{{
			Type: geminiInteractionContentText,
			Text: message.content,
		}}
		for _, attachment := range message.attachments {
			content = append(content, geminiInteractionContent{
				Type:     string(attachment.mediaType),
				Data:     base64.StdEncoding.EncodeToString(attachment.data),
				MIMEType: attachment.mimeType,
			})
		}
		input = append(input, geminiInteractionStep{Type: stepType, Content: content})
	}
	return input, strings.Join(systemInstructions, "\n\n")
}

func geminiInteractionsURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/interactions"
}

func geminiInteractionResourceURL(baseURL string, interactionIdentifier string) string {
	return geminiInteractionsURL(baseURL) + "/" + url.PathEscape(interactionIdentifier)
}

func newGeminiInteractionSnapshot(responseBytes []byte) (geminiInteractionSnapshot, error) {
	var response geminiInteractionResponse
	if decodeError := json.Unmarshal(responseBytes, &response); decodeError != nil {
		return geminiInteractionSnapshot{}, decodeError
	}
	usage, usageError := parseGeminiInteractionTokenUsage(response.Usage)
	snapshot := geminiInteractionSnapshot{
		identifier: strings.TrimSpace(response.ID),
		status:     strings.TrimSpace(response.Status),
		text:       visibleGeminiInteractionText(response.Steps),
		usage:      usage,
	}
	if usageError != nil {
		return snapshot, usageError
	}
	return snapshot, nil
}

func (snapshot geminiInteractionSnapshot) isPending() bool {
	return snapshot.status == statusQueued || snapshot.status == statusInProgress
}

func (snapshot geminiInteractionSnapshot) cleanupMode() geminiInteractionCleanupMode {
	if snapshot.isPending() {
		return geminiInteractionCancelAndDelete
	}
	return geminiInteractionDeleteOnly
}

func (snapshot geminiInteractionSnapshot) resolve() (textGenerationResult, error) {
	generation := textGenerationResult{text: snapshot.text, usage: snapshot.usage}
	switch snapshot.status {
	case statusCompleted:
		if utils.IsBlank(snapshot.text) {
			return textGenerationResult{usage: snapshot.usage}, fmt.Errorf("%w: Gemini Interactions completed without model text", ErrProviderAPI)
		}
		return generation, nil
	case statusIncomplete:
		return generation, errProviderOutputLimitReached
	case geminiInteractionStatusRequiresAction:
		return textGenerationResult{usage: snapshot.usage}, fmt.Errorf("%w: Gemini Interactions requires_action is unsupported", ErrProviderAPI)
	case statusFailed, statusCancelled, geminiInteractionStatusBudgetExceeded:
		return textGenerationResult{usage: snapshot.usage}, fmt.Errorf("%w: Gemini Interactions terminal status=%s", ErrProviderAPI, snapshot.status)
	default:
		return textGenerationResult{usage: snapshot.usage}, fmt.Errorf("%w: Gemini Interactions unknown status=%s", ErrProviderAPI, snapshot.status)
	}
}

func visibleGeminiInteractionText(steps []geminiInteractionStep) string {
	visibleText := constants.EmptyString
	for _, step := range steps {
		if step.Type != geminiInteractionStepModelOutput {
			continue
		}
		var textBuilder strings.Builder
		for _, content := range step.Content {
			if content.Type == geminiInteractionContentText && !utils.IsBlank(content.Text) {
				textBuilder.WriteString(content.Text)
			}
		}
		if textBuilder.Len() > 0 {
			visibleText = textBuilder.String()
		}
	}
	return visibleText
}

func parseGeminiInteractionTokenUsage(usage *geminiInteractionTokenUsage) (*tokenUsage, error) {
	if usage == nil {
		return nil, nil
	}
	return normalizeTokenUsage(usage.TotalInputTokens, usage.TotalOutputTokens, usage.TotalTokens)
}
