package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"time"
)

type journalUnknownReason string
type journalObservationOutcome string

const (
	journalQuantityNotReported journalUnknownReason      = "not_reported"
	journalQuantityInvalid     journalUnknownReason      = "invalid_quantity"
	journalQuantityUnsupported journalUnknownReason      = "unsupported_meter"
	journalOutcomeContinue     journalObservationOutcome = "continue"
	journalOutcomeComplete     journalObservationOutcome = "complete"
	journalOutcomeFail         journalObservationOutcome = "fail"
)

var (
	journalDimensionPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	journalDecimalPattern    = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)
	journalSourcePathPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.\[\]]{0,255}$`)
)

// Values are decimal strings, never floating-point projections of provider usage.
// IncludedIn names an inclusive parent quantity; it must not be charged twice.
type journalQuantity struct {
	Dimension     string               `json:"dimension"`
	Unit          string               `json:"unit"`
	Value         string               `json:"value,omitempty"`
	UnknownReason journalUnknownReason `json:"unknown_reason,omitempty"`
	IncludedIn    string               `json:"included_in,omitempty"`
}

// Only numeric metering fields are retained, not arbitrary provider response data.
type journalSourceField struct {
	Path  string `json:"path"`
	Value string `json:"value"`
}

type journalUsageEvidenceInput struct {
	AttemptID         string
	AdapterRevision   string
	ProviderRequestID string
	Quantities        []journalQuantity
	SourceFields      []journalSourceField
	Outcome           journalObservationOutcome
	FailureCode       string
	ObservedAt        time.Time
}

type journalUsageEvidence struct {
	record            managedJournalObservationRecord
	providerRequestID string
}

func newJournalUsageEvidence(input journalUsageEvidenceInput, entropy io.Reader) (journalUsageEvidence, error) {
	if !strings.HasPrefix(input.AttemptID, journalAttemptIDPrefix) || input.AdapterRevision == "" || input.ObservedAt.IsZero() || len(input.Quantities) == 0 {
		return journalUsageEvidence{}, errUsageJournalInvalid
	}
	switch input.Outcome {
	case journalOutcomeComplete, journalOutcomeContinue:
		if input.FailureCode != "" {
			return journalUsageEvidence{}, errUsageJournalInvalid
		}
	case journalOutcomeFail:
		if !journalDimensionPattern.MatchString(input.FailureCode) {
			return journalUsageEvidence{}, errUsageJournalInvalid
		}
	default:
		return journalUsageEvidence{}, errUsageJournalInvalid
	}
	quantities := slices.Clone(input.Quantities)
	completeness, err := normalizeJournalQuantities(quantities)
	if err != nil {
		return journalUsageEvidence{}, err
	}
	fields := append([]journalSourceField{}, input.SourceFields...)
	slices.SortFunc(fields, func(left, right journalSourceField) int { return strings.Compare(left.Path, right.Path) })
	for index := range fields {
		field := &fields[index]
		if !journalSourcePathPattern.MatchString(field.Path) || !journalDecimalPattern.MatchString(field.Value) || (index > 0 && field.Path == fields[index-1].Path) {
			return journalUsageEvidence{}, errUsageJournalInvalid
		}
		field.Value = normalizeJournalDecimal(field.Value)
	}
	quantityJSON, err := json.Marshal(quantities)
	if err != nil {
		return journalUsageEvidence{}, fmt.Errorf("encode usage quantities: %w", err)
	}
	fieldJSON, err := json.Marshal(fields)
	if err != nil {
		return journalUsageEvidence{}, fmt.Errorf("encode usage source fields: %w", err)
	}
	identity, err := json.Marshal([]string{input.AdapterRevision, input.ProviderRequestID, string(quantityJSON), string(fieldJSON), string(input.Outcome), input.FailureCode})
	if err != nil {
		return journalUsageEvidence{}, fmt.Errorf("encode usage evidence identity: %w", err)
	}
	identifier, err := newHostedResourceID(journalObservationIDPrefix, entropy)
	if err != nil {
		return journalUsageEvidence{}, err
	}
	return journalUsageEvidence{record: managedJournalObservationRecord{
		ID: identifier, AttemptID: input.AttemptID, EvidenceDigest: sha256Hex(string(identity)), AdapterRevision: input.AdapterRevision,
		Quantities: quantityJSON, SourceFields: fieldJSON, Completeness: completeness, Outcome: input.Outcome, FailureCode: input.FailureCode,
		ObservedAt: input.ObservedAt.UTC(), CreatedAt: input.ObservedAt.UTC(),
	}, providerRequestID: input.ProviderRequestID}, nil
}

func normalizeJournalDecimal(value string) string {
	if strings.Contains(value, ".") {
		return strings.TrimSuffix(strings.TrimRight(value, "0"), ".")
	}
	return value
}

func normalizeJournalQuantities(quantities []journalQuantity) (journalUsageState, error) {
	state := journalUsageComplete
	slices.SortFunc(quantities, func(left, right journalQuantity) int { return strings.Compare(left.Dimension, right.Dimension) })
	byDimension := make(map[string]journalQuantity, len(quantities))
	for index := range quantities {
		quantity := &quantities[index]
		if !journalDimensionPattern.MatchString(quantity.Dimension) || !journalDimensionPattern.MatchString(quantity.Unit) {
			return "", errUsageJournalInvalid
		}
		if _, exists := byDimension[quantity.Dimension]; exists {
			return "", errUsageJournalInvalid
		}
		if quantity.Value != "" {
			if quantity.UnknownReason != "" || !journalDecimalPattern.MatchString(quantity.Value) {
				return "", errUsageJournalInvalid
			}
			quantity.Value = normalizeJournalDecimal(quantity.Value)
		} else {
			switch quantity.UnknownReason {
			case journalQuantityNotReported, journalQuantityInvalid, journalQuantityUnsupported:
				state = journalUsageUnknown
			default:
				return "", errUsageJournalInvalid
			}
		}
		byDimension[quantity.Dimension] = *quantity
	}
	for _, quantity := range quantities {
		seen := map[string]bool{quantity.Dimension: true}
		for parent := quantity.IncludedIn; parent != ""; {
			inclusive, exists := byDimension[parent]
			if !exists || seen[parent] || inclusive.Unit != quantity.Unit {
				return "", errUsageJournalInvalid
			}
			if quantity.Value != "" && inclusive.Value != "" {
				childValue, _ := new(big.Rat).SetString(quantity.Value)
				parentValue, _ := new(big.Rat).SetString(inclusive.Value)
				if childValue.Cmp(parentValue) > 0 {
					return "", errUsageJournalInvalid
				}
			}
			seen[parent] = true
			parent = inclusive.IncludedIn
		}
	}
	return state, nil
}
