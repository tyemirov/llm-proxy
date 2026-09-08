package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

const vertexResponseLimit = 16 << 20
const vertexInlineRequestLimit = 20_000_000

type vertexPart struct {
	Text       string            `json:"text,omitempty"`
	Thought    bool              `json:"thought,omitempty"`
	InlineData *vertexInlineData `json:"inlineData,omitempty"`
}
type vertexInlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}
type vertexContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []vertexPart `json:"parts"`
}
type vertexThinkingConfig struct {
	Level string `json:"thinkingLevel"`
}
type vertexTextFormat struct {
	MIMEType string          `json:"mimeType"`
	Schema   json.RawMessage `json:"schema"`
}
type vertexResponseFormat struct {
	Text vertexTextFormat `json:"text"`
}
type vertexGenerationConfig struct {
	MaxOutputTokens *int                   `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *vertexThinkingConfig  `json:"thinkingConfig,omitempty"`
	ResponseFormat  []vertexResponseFormat `json:"responseFormat,omitempty"`
}
type vertexRequest struct {
	Contents          []vertexContent        `json:"contents"`
	SystemInstruction *vertexContent         `json:"systemInstruction,omitempty"`
	GenerationConfig  vertexGenerationConfig `json:"generationConfig"`
}
type vertexResponse struct {
	Candidates []struct {
		Content      vertexContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	Usage *struct {
		Input    int `json:"promptTokenCount"`
		Output   int `json:"candidatesTokenCount"`
		Thoughts int `json:"thoughtsTokenCount"`
		Total    int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

type vertexTextRouteAdapter struct{}

func (vertexTextRouteAdapter) generateText(ctx context.Context, router *providerRouter, request chatRequestParameters, _ *zap.SugaredLogger) (textGenerationResult, error) {
	if err := validateInlineMessageMediaBeforeSerialization(request.model, request.messages); err != nil {
		return textGenerationResult{}, err
	}
	payload, err := buildVertexPayload(request.messages, request.maxTokens, request.reasoningEffort, request.structuredOutput)
	if err != nil {
		return textGenerationResult{}, err
	}
	encoded, _ := json.Marshal(payload)
	if err := validateInlineMessageMediaRequestLimit(request.model, request.messages, encoded); err != nil {
		return textGenerationResult{}, err
	}
	return generateVertexContent(ctx, router.geminiClient.httpClient, request.provider, request.model.providerString(), payload)
}

func buildVertexPayload(messages chatMessages, maxTokens *int, effort string, schema *structuredOutputSchema) (vertexRequest, error) {
	payload := vertexRequest{GenerationConfig: vertexGenerationConfig{MaxOutputTokens: maxTokens}}
	if effort != "" {
		payload.GenerationConfig.ThinkingConfig = &vertexThinkingConfig{Level: strings.ToUpper(effort)}
	}
	if schema != nil {
		payload.GenerationConfig.ResponseFormat = []vertexResponseFormat{{Text: vertexTextFormat{MIMEType: "APPLICATION_JSON", Schema: schema.canonical}}}
	}
	for _, message := range messages {
		content := vertexContent{Role: string(message.role), Parts: []vertexPart{}}
		if message.content != "" {
			content.Parts = append(content.Parts, vertexPart{Text: message.content})
		}
		for i := range message.attachments {
			data, err := message.attachments[i].bytes()
			if err != nil {
				return vertexRequest{}, fmt.Errorf("read Vertex media: %w", err)
			}
			content.Parts = append(content.Parts, vertexPart{InlineData: &vertexInlineData{MIMEType: message.attachments[i].mimeType, Data: base64.StdEncoding.EncodeToString(data)}})
		}
		if message.role == chatRoleSystem {
			if payload.SystemInstruction == nil {
				payload.SystemInstruction = &vertexContent{Parts: []vertexPart{}}
			}
			payload.SystemInstruction.Parts = append(payload.SystemInstruction.Parts, content.Parts...)
		} else {
			if message.role == chatRoleAssistant {
				content.Role = "model"
			}
			payload.Contents = append(payload.Contents, content)
		}
	}
	return payload, nil
}

func buildVertexRequest(ctx context.Context, provider providerDefinition, model string, payload vertexRequest) (*http.Request, error) {
	encoded, _ := json.Marshal(payload)
	if len(encoded) > vertexInlineRequestLimit {
		return nil, ErrProviderMediaLimit
	}
	endpoint := provider.textEndpointURL + "/" + url.PathEscape(model) + ":generateContent"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("build Vertex request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func generateVertexContent(ctx context.Context, httpClient HTTPDoer, provider providerDefinition, model string, payload vertexRequest) (textGenerationResult, error) {
	request, err := buildVertexRequest(ctx, provider, model, payload)
	if err != nil {
		return textGenerationResult{}, err
	}
	httpClient = newProviderTransportHTTPDoer(httpClient, provider, provider.credentialFor(endpointKindText))
	response, err := httpClient.Do(request)
	if err != nil {
		return textGenerationResult{}, fmt.Errorf("send Vertex request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return textGenerationResult{}, newProviderHTTPError(response.StatusCode, response.Header)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, vertexResponseLimit+1))
	if err != nil {
		return textGenerationResult{}, fmt.Errorf("read Vertex response: %w", err)
	}
	if len(body) > vertexResponseLimit {
		return textGenerationResult{}, fmt.Errorf("%w: Vertex response exceeds limit", ErrProviderAPI)
	}
	return parseVertexResponse(body)
}

func parseVertexResponse(body []byte) (textGenerationResult, error) {
	var response vertexResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return textGenerationResult{}, fmt.Errorf("%w: invalid Vertex response", ErrProviderAPI)
	}
	result := textGenerationResult{}
	if response.Usage != nil {
		if response.Usage.Thoughts < 0 || response.Usage.Output < 0 {
			return result, fmt.Errorf("%w: invalid Vertex usage", ErrProviderAPI)
		}
		usage, err := newTokenUsage(response.Usage.Input, response.Usage.Output+response.Usage.Thoughts, response.Usage.Total)
		if err != nil {
			return result, err
		}
		result.usage = usage
	}
	if len(response.Candidates) != 1 {
		return result, fmt.Errorf("%w: Vertex response must contain one candidate", ErrProviderAPI)
	}
	candidate := response.Candidates[0]
	if candidate.FinishReason == "MAX_TOKENS" {
		return result, errProviderOutputLimitReached
	}
	if candidate.FinishReason != "STOP" {
		return result, fmt.Errorf("%w: Vertex completion failed", ErrProviderAPI)
	}
	var output strings.Builder
	for _, part := range candidate.Content.Parts {
		if !part.Thought {
			output.WriteString(part.Text)
		}
	}
	result.text = output.String()
	if strings.TrimSpace(result.text) == "" {
		return result, fmt.Errorf("%w: empty Vertex completion", ErrProviderAPI)
	}
	return result, nil
}

func transcribeVertexAudio(ctx context.Context, httpClient HTTPDoer, request dictationRequestParameters) (string, error) {
	mimeType, supported := geminiTranscriptionMIMETypes[strings.ToLower(filepath.Ext(request.fileName))]
	if !supported {
		return "", ErrInvalidAudioInput
	}
	data, err := io.ReadAll(io.LimitReader(request.audioReader, vertexInlineRequestLimit+1))
	if err != nil {
		return "", fmt.Errorf("read Vertex audio: %w", err)
	}
	if len(data) == 0 {
		return "", ErrInvalidAudioInput
	}
	if len(data) > vertexInlineRequestLimit {
		return "", ErrProviderMediaLimit
	}
	payload := vertexRequest{Contents: []vertexContent{{Role: "user", Parts: []vertexPart{{InlineData: &vertexInlineData{MIMEType: mimeType, Data: base64.StdEncoding.EncodeToString(data)}}}}}}
	model := request.provider.transcriptionModels[strings.ToLower(request.model.string())].providerIdentifier.string()
	result, err := generateVertexContent(ctx, httpClient, request.provider, model, payload)
	return result.text, err
}
