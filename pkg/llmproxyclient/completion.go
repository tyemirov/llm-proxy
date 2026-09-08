package llmproxyclient

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

// CompletionResult contains a response and safe provider-dispatch measurements.
// On failure, only metadata received from the proxy is available.
type CompletionResult struct {
	text          string
	resolvedModel string
	usage         *TokenUsage
}

// Text returns the response body on success.
func (result CompletionResult) Text() string { return result.text }

// ResolvedModel returns the exact selected catalog model, or empty if unavailable.
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

// PostMessagesCompletion returns text and metadata from one native messages call.
// The caller must inspect the result even when an error is returned.
// Structured output requests use their durable reconciliation contract instead.
func (client Client) PostMessagesCompletion(ctx context.Context, request MessagesRequest) (CompletionResult, error) {
	if request.structuredOutput != nil {
		return CompletionResult{}, fmt.Errorf("%w: completion measurements require ordinary messages", ErrInvalidClientRequest)
	}
	requestURL, body, err := client.messagesPostRequest(request)
	if err != nil {
		return CompletionResult{}, err
	}
	result, err := client.postCompletionPayload(ctx, requestURL, body, request.requestTimeoutSeconds, request.idempotencyKey)
	if err == nil && result.resolvedModel == "" {
		return CompletionResult{}, fmt.Errorf("%w: missing resolved model", ErrClientHTTPFailure)
	}
	return result, err
}

func completionMetadata(header http.Header) (CompletionResult, error) {
	result := CompletionResult{resolvedModel: strings.TrimSpace(header.Get(llmproxycontract.HeaderResolvedModel))}
	names := []string{llmproxycontract.HeaderRequestTokens, llmproxycontract.HeaderResponseTokens, llmproxycontract.HeaderTotalTokens}
	counts := [3]int{}
	present := 0
	for index, name := range names {
		values := header.Values(name)
		if len(values) == 0 {
			continue
		}
		if len(values) != 1 {
			return CompletionResult{}, fmt.Errorf("%w: duplicate token metadata", ErrClientHTTPFailure)
		}
		count, err := strconv.Atoi(values[0])
		if err != nil || count < 0 {
			return CompletionResult{}, fmt.Errorf("%w: invalid token metadata", ErrClientHTTPFailure)
		}
		counts[index] = count
		present++
	}
	if present != 0 && present != len(names) {
		return CompletionResult{}, fmt.Errorf("%w: incomplete token metadata", ErrClientHTTPFailure)
	}
	if present == len(names) {
		result.usage = &TokenUsage{inputTokens: counts[0], outputTokens: counts[1], totalTokens: counts[2]}
	}
	return result, nil
}
