package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

const (
	geminiAPIKeyHeader          = "x-goog-api-key"
	geminiFinishReasonStop      = geminiFinishReason("STOP")
	geminiFinishReasonMaxTokens = geminiFinishReason("MAX_TOKENS")
)

type geminiFinishReason string

type geminiGenerateContentClient struct {
	httpClient HTTPDoer
}

type geminiGenerateContentRequest struct {
	Contents          []geminiRequestContent  `json:"contents"`
	SystemInstruction *geminiRequestContent   `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

type geminiRequestContent struct {
	Role  string              `json:"role,omitempty"`
	Parts []geminiRequestPart `json:"parts"`
}

type geminiRequestPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
}

type geminiInlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiResponseContent struct {
	Parts []geminiResponsePart `json:"parts"`
}

type geminiResponsePart struct {
	Text    string `json:"text"`
	Thought bool   `json:"thought"`
}

type geminiGenerateContentResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content      geminiResponseContent `json:"content"`
	FinishReason geminiFinishReason    `json:"finishReason"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     *int `json:"promptTokenCount"`
	CandidatesTokenCount *int `json:"candidatesTokenCount"`
	TotalTokenCount      *int `json:"totalTokenCount"`
}

func newGeminiGenerateContentClient(httpClient HTTPDoer) *geminiGenerateContentClient {
	return &geminiGenerateContentClient{
		httpClient: httpClient,
	}
}

func (client *geminiGenerateContentClient) generateText(parentContext context.Context, apiKey string, baseURL string, modelIdentifier textModelDefinition, messages chatMessages, maxTokens *int, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	contents, systemInstruction := messages.geminiContents()
	payload := geminiGenerateContentRequest{
		Contents:          contents,
		SystemInstruction: systemInstruction,
	}
	if maxTokens != nil {
		payload.GenerationConfig = &geminiGenerationConfig{MaxOutputTokens: *maxTokens}
	}
	payloadBytes, _ := json.Marshal(payload)

	requestURL := geminiGenerateContentURL(baseURL, modelIdentifier)
	httpRequest, buildError := http.NewRequestWithContext(parentContext, http.MethodPost, requestURL, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return textGenerationResult{}, buildError
	}
	httpRequest.Header.Set(headerContentType, mimeApplicationJSON)
	httpRequest.Header.Set(geminiAPIKeyHeader, strings.TrimSpace(apiKey))

	statusCode, responseBytes, responseHeader, _, requestError := utils.PerformHTTPRequest(client.httpClient.Do, httpRequest, structuredLogger, logEventProviderRequestError)
	if responseError := providerResponseError(statusCode, responseHeader, requestError); responseError != nil {
		return textGenerationResult{}, responseError
	}
	generation, parseError := parseGeminiGenerateContentResponse(responseBytes)
	if parseError != nil {
		return generation, parseError
	}
	return generation, nil
}

func (messages chatMessages) geminiContents() ([]geminiRequestContent, *geminiRequestContent) {
	contents := []geminiRequestContent{}
	var systemInstructionParts []geminiRequestPart
	for _, message := range messages {
		if message.role == chatRoleSystem {
			systemInstructionParts = append(systemInstructionParts, geminiRequestPart{Text: message.content})
			continue
		}
		role := "user"
		if message.role == chatRoleAssistant {
			role = "model"
		}
		parts := []geminiRequestPart{{Text: message.content}}
		for _, attachment := range message.attachments {
			parts = append(parts, geminiRequestPart{InlineData: &geminiInlineData{
				MIMEType: attachment.mimeType,
				Data:     base64.StdEncoding.EncodeToString(attachment.data),
			}})
		}
		contents = append(contents, geminiRequestContent{
			Role:  role,
			Parts: parts,
		})
	}
	if len(systemInstructionParts) == 0 {
		return contents, nil
	}
	return contents, &geminiRequestContent{Parts: systemInstructionParts}
}

func geminiGenerateContentURL(baseURL string, modelIdentifier textModelDefinition) string {
	trimmedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return trimmedBaseURL + "/models/" + url.PathEscape(modelIdentifier.string()) + ":generateContent"
}

func parseGeminiGenerateContentResponse(responseBytes []byte) (textGenerationResult, error) {
	var response geminiGenerateContentResponse
	if decodeError := json.Unmarshal(responseBytes, &response); decodeError != nil {
		return textGenerationResult{}, decodeError
	}
	usage, usageError := parseGeminiTokenUsage(response.UsageMetadata)
	if usageError != nil {
		return textGenerationResult{}, usageError
	}
	generation := textGenerationResult{usage: usage}
	for _, candidate := range response.Candidates {
		visibleText := visibleGeminiCandidateText(candidate)
		if finishReasonError := validateGeminiFinishReason(candidate.FinishReason); finishReasonError != nil {
			return textGenerationResult{text: visibleText, usage: usage}, finishReasonError
		}
		if !utils.IsBlank(visibleText) {
			return textGenerationResult{text: visibleText, usage: usage}, nil
		}
	}
	return generation, fmt.Errorf("%w: gemini generateContent returned no text", ErrProviderAPI)
}

func validateGeminiFinishReason(reason geminiFinishReason) error {
	if reason == geminiFinishReasonStop {
		return nil
	}
	if reason == geminiFinishReasonMaxTokens {
		return errProviderOutputLimitReached
	}
	normalizedReason := strings.TrimSpace(string(reason))
	if normalizedReason == constants.EmptyString {
		return fmt.Errorf("%w: gemini generateContent missing finishReason", ErrProviderAPI)
	}
	return fmt.Errorf("%w: gemini generateContent finishReason=%s", ErrProviderAPI, normalizedReason)
}

func visibleGeminiCandidateText(candidate geminiCandidate) string {
	var textBuilder strings.Builder
	for _, part := range candidate.Content.Parts {
		if !part.Thought && strings.TrimSpace(part.Text) != constants.EmptyString {
			textBuilder.WriteString(part.Text)
		}
	}
	return textBuilder.String()
}

func parseGeminiTokenUsage(usage *geminiUsageMetadata) (*tokenUsage, error) {
	if usage == nil {
		return nil, nil
	}
	return normalizeTokenUsage(usage.PromptTokenCount, usage.CandidatesTokenCount, usage.TotalTokenCount)
}
