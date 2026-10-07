package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type textGenerationResult struct {
	toolCalls                      []functionCall
	text                           string
	usage                          *tokenUsage
	chatCompletionReasoningContent *string
	chatCompletionContinuation     *chatCompletionContinuation
}

type tokenMeasurementEvidence uint8

type tokenMeasurementState uint8

const (
	tokenMeasurementUnknown tokenMeasurementState = iota
	tokenMeasurementPartial
	tokenMeasurementComplete
	tokenRequestShift                                         = 0
	tokenResponseShift                                        = 2
	tokenTotalShift                                           = 4
	tokenMeasurementCompleteEvidence tokenMeasurementEvidence = 42
)

func (evidence tokenMeasurementEvidence) state(shift uint) tokenMeasurementState {
	return tokenMeasurementState((evidence >> shift) & 3)
}

func (evidence tokenMeasurementEvidence) validate() error {
	if evidence > 63 || evidence.state(tokenRequestShift) > tokenMeasurementComplete || evidence.state(tokenResponseShift) > tokenMeasurementComplete || evidence.state(tokenTotalShift) > tokenMeasurementComplete {
		return fmt.Errorf("%w: invalid token measurement evidence=%d", ErrProviderAPI, evidence)
	}
	return nil
}

func measurementEvidence(usage *tokenUsage) tokenMeasurementEvidence {
	if usage == nil || usage.MeasurementEvidence == nil {
		return 0
	}
	return *usage.MeasurementEvidence
}

// completeTokenUsage exposes complete logical-generation counts only.
func completeTokenUsage(usage *tokenUsage) *tokenUsage {
	if measurementEvidence(usage) != tokenMeasurementCompleteEvidence {
		return nil
	}
	return usage
}

type tokenUsage struct {
	RequestTokens       int                       `json:"request_tokens"`
	ResponseTokens      int                       `json:"response_tokens"`
	TotalTokens         int                       `json:"total_tokens"`
	MeasurementEvidence *tokenMeasurementEvidence `json:"measurement_evidence,omitempty"`
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
	evidence := tokenMeasurementCompleteEvidence
	return &tokenUsage{RequestTokens: requestTokens, ResponseTokens: responseTokens, TotalTokens: totalTokens, MeasurementEvidence: &evidence}, nil
}

func mergeTokenUsage(primaryUsage *tokenUsage, additionalUsage *tokenUsage) *tokenUsage {
	if primaryUsage == nil && additionalUsage == nil {
		return nil
	}
	primaryEvidence, additionalEvidence := measurementEvidence(primaryUsage), measurementEvidence(additionalUsage)
	if primaryUsage == nil {
		primaryUsage = &tokenUsage{}
	}
	if additionalUsage == nil {
		additionalUsage = &tokenUsage{}
	}
	var evidence tokenMeasurementEvidence
	for _, shift := range []uint{tokenRequestShift, tokenResponseShift, tokenTotalShift} {
		state := tokenMeasurementUnknown
		primaryState, additionalState := primaryEvidence.state(shift), additionalEvidence.state(shift)
		if primaryState == tokenMeasurementComplete && additionalState == tokenMeasurementComplete {
			state = tokenMeasurementComplete
		} else if primaryState != tokenMeasurementUnknown || additionalState != tokenMeasurementUnknown {
			state = tokenMeasurementPartial
		}
		evidence |= tokenMeasurementEvidence(state) << shift
	}
	return &tokenUsage{RequestTokens: primaryUsage.RequestTokens + additionalUsage.RequestTokens, ResponseTokens: primaryUsage.ResponseTokens + additionalUsage.ResponseTokens, TotalTokens: primaryUsage.TotalTokens + additionalUsage.TotalTokens, MeasurementEvidence: &evidence}
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
	if totalTokens == nil && requestTokens != nil && responseTokens != nil {
		derived := *requestTokens + *responseTokens
		totalTokens = &derived
	}
	usage, err := newTokenUsage(tokenCountValue(requestTokens), tokenCountValue(responseTokens), tokenCountValue(totalTokens))
	if err != nil {
		return nil, err
	}
	var evidence tokenMeasurementEvidence
	for index, value := range []*int{requestTokens, responseTokens, totalTokens} {
		if value != nil {
			evidence |= tokenMeasurementEvidence(tokenMeasurementComplete) << (2 * uint(index))
		}
	}
	usage.MeasurementEvidence = &evidence
	return usage, nil
}

func tokenCountValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func writeTokenUsageHeaders(responseHeader http.Header, usage *tokenUsage) {
	if completeTokenUsage(usage) == nil {
		return
	}
	responseHeader.Set(headerLLMProxyRequestTokens, strconv.Itoa(usage.RequestTokens))
	responseHeader.Set(headerLLMProxyResponseTokens, strconv.Itoa(usage.ResponseTokens))
	responseHeader.Set(headerLLMProxyTotalTokens, strconv.Itoa(usage.TotalTokens))
}

// observeTokenUsage replaces snapshots of one generation without double counting.
// A quantity omitted by the latest snapshot retains only a partial earlier subtotal.
func observeTokenUsage(previous *tokenUsage, observed *tokenUsage) *tokenUsage {
	if previous == nil && observed == nil {
		return nil
	}
	if observed == nil {
		observed = &tokenUsage{}
	}
	if previous == nil {
		return observed
	}
	combined := *observed
	evidence := measurementEvidence(observed)
	previousEvidence := measurementEvidence(previous)
	previousCounts := []int{previous.RequestTokens, previous.ResponseTokens, previous.TotalTokens}
	quantities := []*int{&combined.RequestTokens, &combined.ResponseTokens, &combined.TotalTokens}
	for index, shift := range []uint{tokenRequestShift, tokenResponseShift, tokenTotalShift} {
		if evidence.state(shift) == tokenMeasurementUnknown && previousEvidence.state(shift) != tokenMeasurementUnknown {
			*quantities[index] = previousCounts[index]
			evidence |= tokenMeasurementEvidence(tokenMeasurementPartial) << shift
		}
	}
	combined.MeasurementEvidence = &evidence
	return &combined
}

// completionTokenUsage retains the existing public operational measurement shape.
type completionTokenUsage struct {
	RequestTokens  int `json:"request_tokens"`
	ResponseTokens int `json:"response_tokens"`
	TotalTokens    int `json:"total_tokens"`
}

func publicTokenUsage(usage *tokenUsage) *completionTokenUsage {
	if completeTokenUsage(usage) == nil {
		return nil
	}
	return &completionTokenUsage{RequestTokens: usage.RequestTokens, ResponseTokens: usage.ResponseTokens, TotalTokens: usage.TotalTokens}
}
