package proxy

import (
	"fmt"
	"math"
	"math/big"
	"strconv"

	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
)

type hostedMediaUsageContextKey struct{}

// Capture native measurements before the protocol adapter processes results.
func (connection *hostedMediaGRPCConnection) recordResponseUsage(reply any) error {
	if connection.recordUsage == nil {
		return nil
	}
	input := journalUsageEvidenceInput{AdapterRevision: CatalogProtocolDictatorSpeechV1 + ":2", Outcome: journalOutcomeContinue}
	var nativeState int32
	switch response := reply.(type) {
	case *dictator.GetSynthesizeSpeechJobResponse:
		nativeState, input.ProviderRequestID = int32(response.State), response.JobId
		input.Quantities = nil
		recordDictatorOutputDuration(&input, response.AudioDurationSeconds, "audio_duration_seconds")
	case *dictator.GetTranscribeJobResponse:
		nativeState, input.ProviderRequestID = int32(response.State), response.JobId
		recordDictatorInputDuration(&input, response.InputAudioUsage)
	case *dictator.GetDiarizeAudioJobResponse:
		nativeState, input.ProviderRequestID = int32(response.State), response.JobId
		recordDictatorInputDuration(&input, response.InputAudioUsage)
	case *dictator.GetAlignTranscriptJobResponse:
		nativeState, input.ProviderRequestID = int32(response.State), response.JobId
		recordDictatorInputDuration(&input, response.InputAudioUsage)
	case *dictator.GetRenderSubtitlesJobResponse:
		nativeState, input.ProviderRequestID = int32(response.State), response.JobId
		recordDictatorInputDuration(&input, response.InputAudioUsage)
	case *dictator.GetExtractReferenceSampleJobResponse:
		nativeState, input.ProviderRequestID = int32(response.State), response.JobId
		recordDictatorInputDuration(&input, response.InputAudioUsage)
		recordDictatorOutputDuration(&input, response.GetSampleArtifact().GetAudioMetadata().GetDurationSeconds(), "sample_artifact.audio_metadata.duration_seconds")
	default:
		return nil
	}
	state, err := dictatorJobState(nativeState)
	if err != nil {
		return err
	}
	switch state {
	case dictatorProtocolStateSucceeded, dictatorProtocolStateFailed, dictatorProtocolStateCancelled:
	default:
		return nil
	}
	if err := connection.recordUsage(input); err != nil {
		return fmt.Errorf("record Dictator usage: %w: %w", errUsageJournalUnavailable, err)
	}
	return nil
}

func recordDictatorInputDuration(input *journalUsageEvidenceInput, usage *dictator.InputAudioUsage) {
	quantity := journalQuantity{Dimension: "input_audio_seconds", Unit: "second", UnknownReason: journalQuantityNotReported}
	if usage != nil {
		input.SourceFields = append(input.SourceFields,
			journalSourceField{Path: "input_audio_usage.sample_count", Value: strconv.FormatUint(usage.SampleCount, 10)},
			journalSourceField{Path: "input_audio_usage.sample_rate_hz", Value: strconv.FormatUint(uint64(usage.SampleRateHz), 10)})
		if usage.SampleRateHz == 0 {
			quantity.UnknownReason = journalQuantityInvalid
		} else {
			duration := new(big.Rat).SetFrac(new(big.Int).SetUint64(usage.SampleCount), new(big.Int).SetUint64(uint64(usage.SampleRateHz)))
			if _, finite := duration.FloatPrec(); finite {
				quantity.Value, quantity.UnknownReason = ratingDecimal(duration), ""
			} else {
				// The journal requires exact decimal quantities. Never round a repeating fraction.
				quantity.UnknownReason = journalQuantityUnsupported
			}
		}
	}
	input.Quantities = append(input.Quantities, quantity)
}

func recordDictatorOutputDuration(input *journalUsageEvidenceInput, duration float64, path string) {
	quantity := journalQuantity{Dimension: "output_audio_seconds", Unit: "second", UnknownReason: journalQuantityNotReported}
	switch {
	case math.IsNaN(duration), math.IsInf(duration, 0), duration < 0:
		quantity.UnknownReason = journalQuantityInvalid
	case duration > 0:
		quantity.Value, quantity.UnknownReason = strconv.FormatFloat(duration, 'f', -1, 64), ""
		input.SourceFields = append(input.SourceFields, journalSourceField{Path: path, Value: quantity.Value})
	}
	// Proto3 has no presence marker for this scalar. Its zero default is unknown.
	input.Quantities = append(input.Quantities, quantity)
}
