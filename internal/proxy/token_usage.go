package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type textGenerationResult struct {
	text  string
	usage *tokenUsage
}

type tokenUsage struct {
	RequestTokens  int `json:"request_tokens"`
	ResponseTokens int `json:"response_tokens"`
	TotalTokens    int `json:"total_tokens"`
}

type upstreamTokenUsage struct {
	InputTokens      *int `json:"input_tokens"`
	OutputTokens     *int `json:"output_tokens"`
	PromptTokens     *int `json:"prompt_tokens"`
	CompletionTokens *int `json:"completion_tokens"`
	TotalTokens      *int `json:"total_tokens"`
}

func newTokenUsage(requestTokens int, responseTokens int, totalTokens int) (*tokenUsage, error) {
	if requestTokens < 0 || responseTokens < 0 || totalTokens < 0 {
		return nil, fmt.Errorf("%w: token usage cannot be negative", ErrProviderAPI)
	}
	if totalTokens == 0 {
		totalTokens = requestTokens + responseTokens
	}
	return &tokenUsage{RequestTokens: requestTokens, ResponseTokens: responseTokens, TotalTokens: totalTokens}, nil
}

func mergeTokenUsage(primaryUsage *tokenUsage, additionalUsage *tokenUsage) *tokenUsage {
	if primaryUsage == nil {
		return additionalUsage
	}
	if additionalUsage == nil {
		return primaryUsage
	}
	return &tokenUsage{
		RequestTokens:  primaryUsage.RequestTokens + additionalUsage.RequestTokens,
		ResponseTokens: primaryUsage.ResponseTokens + additionalUsage.ResponseTokens,
		TotalTokens:    primaryUsage.TotalTokens + additionalUsage.TotalTokens,
	}
}

func parseResponsesTokenUsage(responseBytes []byte) (*tokenUsage, error) {
	var envelope struct {
		Usage *upstreamTokenUsage `json:"usage"`
	}
	if decodeError := json.Unmarshal(responseBytes, &envelope); decodeError != nil {
		return nil, decodeError
	}
	if envelope.Usage == nil {
		return nil, nil
	}
	return normalizeTokenUsage(envelope.Usage.InputTokens, envelope.Usage.OutputTokens, envelope.Usage.TotalTokens)
}

func parseChatCompletionTokenUsage(usage *upstreamTokenUsage) (*tokenUsage, error) {
	if usage == nil {
		return nil, nil
	}
	return normalizeTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
}

func normalizeTokenUsage(requestTokens *int, responseTokens *int, totalTokens *int) (*tokenUsage, error) {
	if requestTokens == nil && responseTokens == nil && totalTokens == nil {
		return nil, nil
	}
	return newTokenUsage(tokenCountValue(requestTokens), tokenCountValue(responseTokens), tokenCountValue(totalTokens))
}

func tokenCountValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func writeTokenUsageHeaders(responseHeader http.Header, usage *tokenUsage) {
	if usage == nil {
		return
	}
	responseHeader.Set(headerLLMProxyRequestTokens, strconv.Itoa(usage.RequestTokens))
	responseHeader.Set(headerLLMProxyResponseTokens, strconv.Itoa(usage.ResponseTokens))
	responseHeader.Set(headerLLMProxyTotalTokens, strconv.Itoa(usage.TotalTokens))
}
