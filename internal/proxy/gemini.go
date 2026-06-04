package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/temirov/llm-proxy/internal/constants"
	"github.com/temirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

const geminiAPIKeyHeader = "x-goog-api-key"

type geminiGenerateContentClient struct {
	httpClient     HTTPDoer
	requestTimeout time.Duration
}

type geminiGenerateContentRequest struct {
	Contents          []geminiContent         `json:"contents"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerateContentResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     *int `json:"promptTokenCount"`
	CandidatesTokenCount *int `json:"candidatesTokenCount"`
	TotalTokenCount      *int `json:"totalTokenCount"`
}

func newGeminiGenerateContentClient(httpClient HTTPDoer, requestTimeout time.Duration) *geminiGenerateContentClient {
	return &geminiGenerateContentClient{
		httpClient:     httpClient,
		requestTimeout: requestTimeout,
	}
}

func (client *geminiGenerateContentClient) generateText(parentContext context.Context, apiKey string, baseURL string, modelIdentifier modelID, userPrompt string, systemPrompt string, maxTokens *int, structuredLogger *zap.SugaredLogger) (textGenerationResult, error) {
	payload := geminiGenerateContentRequest{
		Contents: []geminiContent{{
			Role:  "user",
			Parts: []geminiPart{{Text: userPrompt}},
		}},
	}
	if !utils.IsBlank(systemPrompt) {
		payload.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: systemPrompt}}}
	}
	if maxTokens != nil {
		payload.GenerationConfig = &geminiGenerationConfig{MaxOutputTokens: *maxTokens}
	}
	payloadBytes, _ := json.Marshal(payload)

	requestURL := geminiGenerateContentURL(baseURL, modelIdentifier)
	requestContext, cancelRequest := context.WithTimeout(parentContext, client.requestTimeout)
	defer cancelRequest()
	httpRequest, buildError := http.NewRequestWithContext(requestContext, http.MethodPost, requestURL, bytes.NewReader(payloadBytes))
	if buildError != nil {
		structuredLogger.Errorw(logEventBuildHTTPRequest, constants.LogFieldError, buildError)
		return textGenerationResult{}, buildError
	}
	httpRequest.Header.Set(headerContentType, mimeApplicationJSON)
	httpRequest.Header.Set(geminiAPIKeyHeader, strings.TrimSpace(apiKey))

	statusCode, responseBytes, _, requestError := utils.PerformHTTPRequest(client.httpClient.Do, httpRequest, structuredLogger, logEventProviderRequestError)
	if requestError != nil {
		return textGenerationResult{}, requestError
	}
	if statusCode == http.StatusTooManyRequests {
		return textGenerationResult{}, fmt.Errorf("%w: gemini generateContent", ErrProviderRateLimited)
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return textGenerationResult{}, fmt.Errorf("%w: status=%d", ErrProviderAPI, statusCode)
	}
	generation, parseError := parseGeminiGenerateContentResponse(responseBytes)
	if parseError != nil {
		return textGenerationResult{}, parseError
	}
	return generation, nil
}

func geminiGenerateContentURL(baseURL string, modelIdentifier modelID) string {
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
	for _, candidate := range response.Candidates {
		var textBuilder strings.Builder
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != constants.EmptyString {
				textBuilder.WriteString(part.Text)
			}
		}
		trimmedText := strings.TrimSpace(textBuilder.String())
		if trimmedText != constants.EmptyString {
			return textGenerationResult{text: trimmedText, usage: usage}, nil
		}
	}
	return textGenerationResult{}, fmt.Errorf("%w: gemini generateContent returned no text", ErrProviderAPI)
}

func parseGeminiTokenUsage(usage *geminiUsageMetadata) (*tokenUsage, error) {
	if usage == nil {
		return nil, nil
	}
	return normalizeTokenUsage(usage.PromptTokenCount, usage.CandidatesTokenCount, usage.TotalTokenCount)
}
