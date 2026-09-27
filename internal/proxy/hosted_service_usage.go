package proxy

const journalServiceCallsDimension = "service_calls"

// A validated creation receipt establishes one successful native call. This
// quantity has a monetary value only under an explicit catalog rate per call.
// Recovery uses the same receipt and evidence identity without another call.
func (request MediaOperationExecutionRequest) recordDictionaryUsage() error {
	if request.recordUsage == nil {
		return nil
	}
	return request.recordUsage(journalUsageEvidenceInput{
		AdapterRevision: CatalogProtocolElevenLabsDictionary + ":1",
		Outcome:         journalOutcomeContinue,
		Quantities:      []journalQuantity{{Dimension: journalServiceCallsDimension, Unit: "call", Value: "1"}},
		SourceFields:    []journalSourceField{{Path: "result.dictionary_created", Value: "1"}},
	})
}
