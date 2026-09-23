package proxy

import (
	"net/http"
	"strings"
)

const speechCharacterCostHeader = "character-cost"

// Preserve the provider's reported cost units. Text length, audio duration, and
// output success do not establish this value or its monetary conversion.
type journalHeaderMeter struct {
	codec           string
	quantityHeader  string
	requestIDHeader string
	dimension       string
}

func (request MediaOperationExecutionRequest) recordSpeechUsage(header http.Header, codec string) error {
	return request.recordHeaderUsage(header, journalHeaderMeter{codec: codec, quantityHeader: speechCharacterCostHeader, requestIDHeader: "request-id", dimension: "character_cost"})
}

func (request MediaOperationExecutionRequest) recordQueueUsage(header http.Header) error {
	return request.recordHeaderUsage(header, journalHeaderMeter{codec: CatalogProtocolFALQueueImages, quantityHeader: "x-fal-billable-units", requestIDHeader: "x-fal-request-id", dimension: "billable_units"})
}

func (request MediaOperationExecutionRequest) recordHeaderUsage(header http.Header, meter journalHeaderMeter) error {
	if request.recordUsage == nil {
		return nil
	}
	quantity := journalQuantity{Dimension: meter.dimension, Unit: "provider_unit", UnknownReason: journalQuantityNotReported}
	input := journalUsageEvidenceInput{AdapterRevision: meter.codec + ":1", ProviderRequestID: header.Get(meter.requestIDHeader), Outcome: journalOutcomeContinue}
	values := header.Values(meter.quantityHeader)
	if len(values) != 0 {
		if len(values) == 1 && journalDecimalPattern.MatchString(values[0]) {
			quantity.Value, quantity.UnknownReason = values[0], ""
			input.SourceFields = []journalSourceField{{Path: "headers." + strings.ReplaceAll(meter.quantityHeader, "-", "_"), Value: values[0]}}
		} else {
			quantity.UnknownReason = journalQuantityInvalid
		}
	}
	input.Quantities = []journalQuantity{quantity}
	return request.recordUsage(input)
}
