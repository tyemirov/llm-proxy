package llmproxyclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// CompletionResult contains a response and safe provider-dispatch measurements.
// Measurements are optional and independent of successful text delivery.
type CompletionResult struct {
	text          string
	resolvedModel string
	usage         *TokenUsage
	metadataError error
}

// Text returns the response body on success.
func (result CompletionResult) Text() string { return result.text }

// ResolvedModel returns the response model field, or empty if unavailable.
// It does not claim a provider-reported model revision.
func (result CompletionResult) ResolvedModel() string { return result.resolvedModel }

// TokenUsage is the provider-reported token count for the completion.
type TokenUsage struct{ inputTokens, outputTokens, totalTokens int }

// InputTokens returns the measured input count.
func (usage TokenUsage) InputTokens() int { return usage.inputTokens }

// OutputTokens returns the measured output count.
func (usage TokenUsage) OutputTokens() int { return usage.outputTokens }

// TotalTokens returns the measured total count.
func (usage TokenUsage) TotalTokens() int { return usage.totalTokens }

// Usage returns a copy of measured usage. Nil means usage is unavailable.
func (result CompletionResult) Usage() *TokenUsage {
	if result.usage == nil {
		return nil
	}
	usage := *result.usage
	return &usage
}

// PostMessagesCompletion returns text and optional measurements using the selected protocol.
// Native calls return text without measurements. Responses calls read standard model and usage fields.
func (client Client) PostMessagesCompletion(ctx context.Context, request MessagesRequest) (CompletionResult, error) {
	if client.config.protocol == ProtocolNative {
		text, err := client.PostMessages(ctx, request)
		return CompletionResult{text: text}, err
	}
	body, err := client.responsesPayload(request)
	if err != nil {
		return CompletionResult{}, err
	}
	requestURL := client.config.responsesPostURL()
	httpRequest := (&http.Request{Method: http.MethodPost, URL: &requestURL, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body))}).WithContext(ctx)
	httpRequest.Header.Set(headerAccept, formatQueryValueJSON)
	httpRequest.Header.Set(headerContentType, jsonContentType)
	httpRequest.Header.Set("Authorization", "Bearer "+client.config.secret)
	if request.requestTimeoutSeconds != nil {
		httpRequest.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, strconv.Itoa(*request.requestTimeoutSeconds))
	}
	responseBody, err := client.readCompletionResponse(httpRequest)
	if err != nil {
		return CompletionResult{}, err
	}
	return decodeCompletion(responseBody)
}

func (config Config) responsesPostURL() url.URL {
	requestURL := *config.baseURL
	requestURL.Path = strings.TrimRight(requestURL.Path, "/") + "/v1/responses"
	requestURL.RawPath = ""
	return requestURL
}

func (client Client) responsesPayload(request MessagesRequest) ([]byte, error) {
	if request.structuredOutput != nil || request.webSearch || len(request.tools) > 0 || request.toolChoice != nil || request.parallelToolCalls != nil {
		return nil, fmt.Errorf("%w: Responses completion supports ordinary text messages", ErrInvalidClientRequest)
	}
	provider, model := client.config.provider, request.model
	if client.config.modelProfilePath != "" {
		if model != "" {
			return nil, fmt.Errorf("%w: request model conflicts with model_profile", ErrInvalidModelProfile)
		}
		profile, err := client.config.currentModelProfile()
		if err != nil {
			return nil, err
		}
		provider, model = profile.provider, profile.model
	}
	if provider == "" || model == "" {
		return nil, fmt.Errorf("%w: Responses requires an exact provider and model", ErrInvalidClientRequest)
	}
	type responseMessage struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	messages := make([]responseMessage, 0, len(request.messages))
	for _, message := range request.messages {
		if len(message.attachments) > 0 || len(message.toolCalls) > 0 || message.toolCallID != "" || message.role == "tool" {
			return nil, fmt.Errorf("%w: Responses completion supports text-only message history", ErrInvalidClientRequest)
		}
		messages = append(messages, responseMessage{Type: "message", Role: message.role, Content: message.content})
	}
	type responseReasoning struct {
		Effort string `json:"effort"`
	}
	var reasoning *responseReasoning
	if request.reasoningEffort != nil {
		reasoning = &responseReasoning{Effort: *request.reasoningEffort}
	}
	payload := struct {
		Model           string             `json:"model"`
		Input           []responseMessage  `json:"input"`
		MaxOutputTokens *int               `json:"max_output_tokens,omitempty"`
		Reasoning       *responseReasoning `json:"reasoning,omitempty"`
		Store           bool               `json:"store"`
	}{Model: provider + "/" + model, Input: messages, MaxOutputTokens: request.maxTokens, Store: false}
	if reasoning != nil {
		payload.Reasoning = reasoning
	}
	body, _ := json.Marshal(payload)
	return body, nil
}

// MetadataError reports invalid optional metadata without invalidating the text.
// Nil means metadata is valid or absent. The error excludes response content.
func (result CompletionResult) MetadataError() error { return result.metadataError }

func decodeCompletion(body []byte) (CompletionResult, error) {
	var envelope struct {
		Object string `json:"object"`
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Status  string `json:"status"`
			Content []struct {
				Type string  `json:"type"`
				Text *string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Model json.RawMessage `json:"model"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Object != "response" || envelope.Status != "completed" {
		return CompletionResult{}, fmt.Errorf("%w: invalid completion JSON response", ErrClientHTTPFailure)
	}
	var text strings.Builder
	for _, item := range envelope.Output {
		if item.Type == "reasoning" {
			continue
		}
		if item.Type != "message" || item.Role != "assistant" || item.Status != "completed" {
			return CompletionResult{}, fmt.Errorf("%w: incomplete or unsupported response output", ErrClientHTTPFailure)
		}
		for _, part := range item.Content {
			if part.Type != "output_text" || part.Text == nil {
				return CompletionResult{}, fmt.Errorf("%w: response did not return text", ErrClientHTTPFailure)
			}
			text.WriteString(*part.Text)
		}
	}
	if text.Len() == 0 {
		return CompletionResult{}, fmt.Errorf("%w: response contains no output text", ErrClientHTTPFailure)
	}
	result := CompletionResult{text: text.String()}
	if len(envelope.Model) > 0 {
		if err := json.Unmarshal(envelope.Model, &result.resolvedModel); err != nil {
			result.metadataError = errors.New("invalid completion model metadata")
		}
		result.resolvedModel = strings.TrimSpace(result.resolvedModel)
	}
	if len(envelope.Usage) > 0 {
		var usage *struct {
			Input  *int `json:"input_tokens"`
			Output *int `json:"output_tokens"`
			Total  *int `json:"total_tokens"`
		}
		err := json.Unmarshal(envelope.Usage, &usage)
		if err != nil || (usage != nil && (usage.Input == nil || usage.Output == nil || usage.Total == nil || *usage.Input < 0 || *usage.Output < 0 || *usage.Total < 0)) {
			result.metadataError = errors.Join(result.metadataError, errors.New("invalid completion usage metadata"))
		} else if usage != nil {
			result.usage = &TokenUsage{inputTokens: *usage.Input, outputTokens: *usage.Output, totalTokens: *usage.Total}
		}
	}
	return result, nil
}
