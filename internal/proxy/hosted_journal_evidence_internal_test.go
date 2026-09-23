package proxy

import (
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

func TestHostedJournalEvidencePreservesInclusiveExactQuantities(t *testing.T) {
	input := journalUsageEvidenceInput{AttemptID: "attempt-evidence", AdapterRevision: "meter-1", Outcome: journalOutcomeComplete, ObservedAt: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC),
		Quantities:   []journalQuantity{{Dimension: "cached_tokens", Unit: "token", Value: "9007199254740992.00", IncludedIn: "input_tokens"}, {Dimension: "input_tokens", Unit: "token", Value: "9007199254740993"}},
		SourceFields: []journalSourceField{{Path: "usage.cached_tokens", Value: "9007199254740992.00"}, {Path: "usage.input_tokens", Value: "9007199254740993"}},
	}
	first, err := newJournalUsageEvidence(input, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	input.Quantities[0].Value = "9007199254740992"
	input.SourceFields[0].Value = "9007199254740992"
	input.ObservedAt = input.ObservedAt.Add(time.Second)
	replay, err := newJournalUsageEvidence(input, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if replay.record.EvidenceDigest != first.record.EvidenceDigest {
		t.Fatal("equivalent evidence has different identity")
	}
	input.Quantities[0].Value = "9007199254740994"
	if _, err := newJournalUsageEvidence(input, rand.Reader); !errors.Is(err, errUsageJournalInvalid) {
		t.Fatalf("inclusive child exceeds its parent: %v", err)
	}
}

func TestHostedJournalEvidenceRejectsAmbiguousOrUnmeasuredZero(t *testing.T) {
	for name, quantities := range map[string][]journalQuantity{
		"absent quantities":   {},
		"absent measurement":  {{Dimension: "input_tokens", Unit: "token"}},
		"zero and unknown":    {{Dimension: "input_tokens", Unit: "token", Value: "0", UnknownReason: journalQuantityNotReported}},
		"negative quantity":   {{Dimension: "input_tokens", Unit: "token", Value: "-1"}},
		"nonfinite quantity":  {{Dimension: "input_tokens", Unit: "token", Value: "NaN"}},
		"duplicate dimension": {{Dimension: "input_tokens", Unit: "token", Value: "1"}, {Dimension: "input_tokens", Unit: "token", Value: "2"}},
		"missing parent":      {{Dimension: "cached_tokens", Unit: "token", Value: "1", IncludedIn: "input_tokens"}},
		"cyclic inclusion":    {{Dimension: "input_tokens", Unit: "token", Value: "1", IncludedIn: "cached_tokens"}, {Dimension: "cached_tokens", Unit: "token", Value: "1", IncludedIn: "input_tokens"}},
		"different units":     {{Dimension: "input_tokens", Unit: "token", Value: "1"}, {Dimension: "audio_seconds", Unit: "second", Value: "1", IncludedIn: "input_tokens"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := newJournalUsageEvidence(journalUsageEvidenceInput{AttemptID: "attempt-invalid", AdapterRevision: "meter-1", Quantities: quantities, Outcome: journalOutcomeComplete, ObservedAt: time.Now()}, rand.Reader)
			if !errors.Is(err, errUsageJournalInvalid) {
				t.Fatalf("invalid evidence accepted: %v", err)
			}
		})
	}
}
