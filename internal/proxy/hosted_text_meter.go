package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
)

type journalMeterField struct {
	dimension  string
	path       string
	includedIn string
}

type journalTokenMeter struct {
	codec      string
	fields     []journalMeterField
	pollable   bool
	modalities []journalModalityField
}

// Profiles follow response protocols, not provider names. A missing measurement
// remains unknown even when operational telemetry represents it as zero.
func newTextJournalMeter(codec string) (journalTokenMeter, error) {
	profile := journalTokenMeter{codec: codec}
	switch codec {
	case CatalogProtocolOpenAIResponses, CatalogProtocolDashScopeResponses, CatalogProtocolXAIResponses:
		profile.pollable = true
		profile.fields = []journalMeterField{{"input_tokens", "usage.input_tokens", ""}, {"output_tokens", "usage.output_tokens", ""}, {"cache_read_tokens", "usage.input_tokens_details.cached_tokens", "input_tokens"}, {"reasoning_tokens", "usage.output_tokens_details.reasoning_tokens", "output_tokens"}}
	case CatalogProtocolOpenAIChatCompletions:
		profile.fields = []journalMeterField{{"input_tokens", "usage.prompt_tokens", ""}, {"output_tokens", "usage.completion_tokens", ""}, {"cache_read_tokens", "usage.prompt_tokens_details.cached_tokens", "input_tokens"}, {"reasoning_tokens", "usage.completion_tokens_details.reasoning_tokens", "output_tokens"}}
	case CatalogProtocolAnthropicMessages:
		profile.fields = []journalMeterField{
			{"input_tokens", "usage.input_tokens", ""},
			{"output_tokens", "usage.output_tokens", ""},
			{"cache_read_tokens", "usage.cache_read_input_tokens", ""},
			{"cache_write_tokens", "usage.cache_creation_input_tokens", ""},
			{"cache_write_5m_tokens", "usage.cache_creation.ephemeral_5m_input_tokens", "cache_write_tokens"},
			{"cache_write_1h_tokens", "usage.cache_creation.ephemeral_1h_input_tokens", "cache_write_tokens"},
		}
	case CatalogProtocolGeminiInteractions:
		profile.pollable = true
		profile.fields = []journalMeterField{{"input_tokens", "usage.total_input_tokens", ""}, {"output_tokens", "usage.total_output_tokens", ""}, {"cache_read_tokens", "usage.total_cached_tokens", "input_tokens"}, {"reasoning_tokens", "usage.total_thought_tokens", ""}}
		profile.fields = append(profile.fields, journalMeterField{"tool_input_tokens", "usage.total_tool_use_tokens", ""})
		profile.modalities = []journalModalityField{
			{"input_", "input_tokens", "usage.input_tokens_by_modality", "tokens", interactionModalityNames},
			{"output_", "output_tokens", "usage.output_tokens_by_modality", "tokens", interactionModalityNames},
			{"cache_read_", "cache_read_tokens", "usage.cached_tokens_by_modality", "tokens", interactionModalityNames},
			{"tool_input_", "tool_input_tokens", "usage.tool_use_tokens_by_modality", "tokens", interactionModalityNames},
		}
	case CatalogProtocolVertexGenerateContent:
		profile.fields = []journalMeterField{{"input_tokens", "usageMetadata.promptTokenCount", ""}, {"output_tokens", "usageMetadata.candidatesTokenCount", ""}, {"cache_read_tokens", "usageMetadata.cachedContentTokenCount", "input_tokens"}, {"reasoning_tokens", "usageMetadata.thoughtsTokenCount", ""}}
		profile.fields = append(profile.fields, journalMeterField{"tool_input_tokens", "usageMetadata.toolUsePromptTokenCount", ""})
		profile.modalities = []journalModalityField{
			{"input_", "input_tokens", "usageMetadata.promptTokensDetails", "tokenCount", vertexModalityNames},
			{"output_", "output_tokens", "usageMetadata.candidatesTokensDetails", "tokenCount", vertexModalityNames},
			{"cache_read_", "cache_read_tokens", "usageMetadata.cacheTokensDetails", "tokenCount", vertexModalityNames},
			{"tool_input_", "tool_input_tokens", "usageMetadata.toolUsePromptTokensDetails", "tokenCount", vertexModalityNames},
		}
	default:
		return journalTokenMeter{}, fmt.Errorf("%w: metering unavailable for codec %s", errHostedAuthorityDenied, codec)
	}
	return profile, nil
}

func (profile journalTokenMeter) observe(body []byte, status int, attemptID string, now time.Time) (journalUsageEvidenceInput, bool) {
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	validPayload := decoder.Decode(&payload) == nil && decoder.Decode(new(any)) == io.EOF
	if !validPayload {
		payload = nil
	}
	providerID, _ := payload["id"].(string)
	input := journalUsageEvidenceInput{AttemptID: attemptID, AdapterRevision: profile.codec + ":1", ProviderRequestID: providerID, Outcome: journalOutcomeContinue, ObservedAt: now}
	if validPayload && profile.pollable && status >= http.StatusOK && status < http.StatusMultipleChoices {
		state, _ := payload["status"].(string)
		switch state {
		case "queued", "in_progress", "pending":
			return input, false
		}
	}
	for _, field := range profile.fields {
		field.observeInteger(payload, validPayload, "token", &input)
	}
	validateJournalTokenPartitions(input.Quantities)
	for _, field := range profile.modalities {
		field.observe(payload, &input)
	}
	validateJournalModalityCache(input.Quantities)
	if profile.codec == CatalogProtocolXAIResponses {
		field := journalMeterField{dimension: "provider_cost", path: "usage.cost_in_usd_ticks"}
		field.observeInteger(payload, validPayload, "usd_tick", &input)
	}
	return input, true
}

func (field journalMeterField) observeInteger(payload map[string]any, validPayload bool, unit string, input *journalUsageEvidenceInput) {
	quantity := journalQuantity{Dimension: field.dimension, Unit: unit, IncludedIn: field.includedIn, UnknownReason: journalQuantityNotReported}
	value, present := journalMeterPath(payload, field.path)
	if !validPayload {
		quantity.UnknownReason = journalQuantityInvalid
	} else if present {
		number, valid := value.(json.Number)
		if valid && journalDecimalPattern.MatchString(number.String()) && !strings.Contains(number.String(), ".") {
			quantity.Value, quantity.UnknownReason = number.String(), ""
			input.SourceFields = append(input.SourceFields, journalSourceField{Path: field.path, Value: number.String()})
		} else {
			quantity.UnknownReason = journalQuantityInvalid
		}
	}
	input.Quantities = append(input.Quantities, quantity)
}

// Token subdivisions are disjoint and exhaust their reported parent.
// Contradictory numbers remain in source evidence, but cannot become charges.
func validateJournalTokenPartitions(quantities []journalQuantity) {
	values := make(map[string]*big.Int, len(quantities))
	parents := make(map[string]string, len(quantities))
	for _, quantity := range quantities {
		parents[quantity.Dimension] = quantity.IncludedIn
		if quantity.Value != "" {
			values[quantity.Dimension], _ = new(big.Int).SetString(quantity.Value, 10)
		}
	}
	consistent := true
	for dimension, value := range values {
		for parent := parents[dimension]; parent != ""; parent = parents[parent] {
			if inclusive := values[parent]; inclusive != nil && value.Cmp(inclusive) > 0 {
				consistent = false
			}
		}
	}
	for _, partition := range [][3]string{
		{"total_tokens", "input_tokens", "output_tokens"},
		{"input_tokens", "input_text_tokens", "input_image_tokens"},
		{"input_tokens", "input_text_tokens", "input_audio_tokens"},
		{"output_tokens", "output_text_tokens", "output_image_tokens"},
		{"cache_write_tokens", "cache_write_5m_tokens", "cache_write_1h_tokens"},
	} {
		parent, left, right := values[partition[0]], values[partition[1]], values[partition[2]]
		if parent != nil && left != nil && right != nil && new(big.Int).Add(left, right).Cmp(parent) != 0 {
			consistent = false
		}
	}
	if !consistent {
		for index := range quantities {
			if quantities[index].Value != "" {
				quantities[index].Value, quantities[index].UnknownReason = "", journalQuantityInvalid
			}
		}
	}
}

func journalMeterPath(payload map[string]any, path string) (any, bool) {
	var value any = payload
	for _, part := range strings.Split(path, ".") {
		object, valid := value.(map[string]any)
		if !valid {
			return nil, false
		}
		var present bool
		value, present = object[part]
		if !present {
			return nil, false
		}
	}
	return value, true
}
