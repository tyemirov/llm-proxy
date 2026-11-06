package proxy

import (
	"errors"
	"sort"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type structuredOutputFailureReason string

const (
	structuredOutputNotJSON                structuredOutputFailureReason = "structured_output.not_json"
	structuredOutputMultipleJSON           structuredOutputFailureReason = "structured_output.multiple_json_values"
	structuredOutputSchemaViolation        structuredOutputFailureReason = "structured_output.schema_violation"
	structuredOutputDiagnosticNodeLimit                                  = 64
	structuredOutputDiagnosticDepthLimit                                 = 16
	structuredOutputDiagnosticKeywordLimit                               = 8
	structuredOutputKeywordCodePrefix                                    = "json_schema."
)

var structuredOutputFailureMessages = map[structuredOutputFailureReason]string{
	structuredOutputNotJSON:         "provider output is not JSON",
	structuredOutputMultipleJSON:    "provider output contains multiple JSON values",
	structuredOutputSchemaViolation: "provider output violates structured schema",
}

// Only standard constraint keywords can enter diagnostic metadata. Schema and
// instance paths, names, values, and validator messages are never retained.
var structuredOutputDiagnosticKeywords = map[string]struct{}{
	"type": {}, "enum": {}, "const": {}, "required": {}, "additionalProperties": {},
	"minimum": {}, "maximum": {}, "exclusiveMinimum": {}, "exclusiveMaximum": {}, "multipleOf": {},
	"minLength": {}, "maxLength": {}, "pattern": {}, "format": {},
	"minItems": {}, "maxItems": {}, "uniqueItems": {}, "contains": {}, "minContains": {}, "maxContains": {},
	"minProperties": {}, "maxProperties": {}, "dependentRequired": {}, "propertyNames": {},
	"unevaluatedProperties": {}, "unevaluatedItems": {}, "not": {}, "allOf": {}, "anyOf": {}, "oneOf": {},
}

type structuredOutputResponseFailure struct {
	reason   structuredOutputFailureReason
	keywords []string
}

func newStructuredOutputResponseFailure(reason structuredOutputFailureReason, validationError error) error {
	return &structuredOutputResponseFailure{reason: reason, keywords: structuredOutputFailureKeywords(validationError)}
}

func (failure *structuredOutputResponseFailure) Error() string {
	return ErrProviderAPI.Error() + ": " + structuredOutputFailureMessages[failure.reason]
}
func (failure *structuredOutputResponseFailure) Unwrap() error                  { return ErrProviderAPI }
func (failure *structuredOutputResponseFailure) providerFailureRetryable() bool { return false }
func (failure *structuredOutputResponseFailure) providerFailureCodes() []string {
	return append([]string{string(failure.reason)}, failure.keywords...)
}

func structuredOutputFailureKeywords(validationError error) []string {
	var root *jsonschema.ValidationError
	if !errors.As(validationError, &root) {
		return nil
	}
	visited := 0
	keywords := map[string]struct{}{}
	var visit func(*jsonschema.ValidationError, int)
	visit = func(node *jsonschema.ValidationError, depth int) {
		visited++
		if node == nil || depth > structuredOutputDiagnosticDepthLimit {
			return
		}
		if node.ErrorKind != nil {
			path := node.ErrorKind.KeywordPath()
			if len(path) > 0 {
				if _, allowed := structuredOutputDiagnosticKeywords[path[0]]; allowed && len(keywords) < structuredOutputDiagnosticKeywordLimit {
					keywords[path[0]] = struct{}{}
				}
			}
		}
		for _, cause := range node.Causes {
			if visited >= structuredOutputDiagnosticNodeLimit {
				break
			}
			visit(cause, depth+1)
		}
	}
	visit(root, 0)
	result := make([]string, 0, len(keywords))
	for keyword := range keywords {
		result = append(result, structuredOutputKeywordCodePrefix+keyword)
	}
	sort.Strings(result)
	return result
}
