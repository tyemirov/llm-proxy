package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"time"
)

type journalResponseMeter interface {
	observe([]byte, int, string, time.Time) (journalUsageEvidenceInput, bool)
}

func newCompletionJournalMeter(codec, operation string) (journalResponseMeter, error) {
	if operation == ModelOperationDictation && (codec == CatalogProtocolMultipartTranscription || codec == CatalogProtocolMetaTranscription) {
		return dictationJournalMeter{codec: codec}, nil
	}
	return newTextJournalMeter(codec)
}

type dictationJournalMeter struct{ codec string }

func (profile dictationJournalMeter) observe(body []byte, status int, attemptID string, now time.Time) (journalUsageEvidenceInput, bool) {
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	valid := decoder.Decode(&payload) == nil && decoder.Decode(new(any)) == io.EOF
	if !valid {
		payload = nil
	}
	providerID, _ := payload["id"].(string)
	input := journalUsageEvidenceInput{AttemptID: attemptID, AdapterRevision: profile.codec + ":1", ProviderRequestID: providerID, Outcome: journalOutcomeContinue, ObservedAt: now}
	quantity := journalQuantity{Dimension: "audio_seconds", Unit: "second", UnknownReason: journalQuantityNotReported}
	if profile.codec == CatalogProtocolMetaTranscription {
		quantity.UnknownReason = journalQuantityUnsupported
	} else if !valid {
		quantity.UnknownReason = journalQuantityInvalid
	} else {
		kind, present := journalMeterPath(payload, "usage.type")
		switch kind {
		case "tokens":
			meter := journalTokenMeter{codec: profile.codec, fields: []journalMeterField{
				{"total_tokens", "usage.total_tokens", ""},
				{"input_tokens", "usage.input_tokens", "total_tokens"},
				{"output_tokens", "usage.output_tokens", "total_tokens"},
				{"input_audio_tokens", "usage.input_token_details.audio_tokens", "input_tokens"},
				{"input_text_tokens", "usage.input_token_details.text_tokens", "input_tokens"},
			}}
			return meter.observe(body, status, attemptID, now)
		case "duration":
			value, present := journalMeterPath(payload, "usage.seconds")
			if present {
				number, valid := value.(json.Number)
				if valid && journalDecimalPattern.MatchString(number.String()) {
					quantity.Value, quantity.UnknownReason = number.String(), ""
					input.SourceFields = []journalSourceField{{Path: "usage.seconds", Value: number.String()}}
				} else {
					quantity.UnknownReason = journalQuantityInvalid
				}
			}
		default:
			if present {
				quantity.UnknownReason = journalQuantityUnsupported
			}
		}
	}
	input.Quantities = []journalQuantity{quantity}
	return input, true
}
