package proxy

import (
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedJournalObservationAndDeliveryRemainAtomic(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { return nil }
	request, err := database.admitJournalRequest(t.Context(), intent("observed"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := newJournalWorkerClaim(request.ID, request.OwnerToken, request.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-observed", reserve)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.dispatchJournalAttempt(t.Context(), claim, attempt.ID); err != nil {
		t.Fatal(err)
	}
	evidence, err := newJournalUsageEvidence(journalUsageEvidenceInput{
		AttemptID: attempt.ID, AdapterRevision: "openai-responses-1", ProviderRequestID: "private-provider-request",
		Quantities:   []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "9007199254740993"}, {Dimension: "output_tokens", Unit: "token", Value: "0"}},
		SourceFields: []journalSourceField{{Path: "usage.input_tokens", Value: "9007199254740993"}, {Path: "usage.output_tokens", Value: "0"}},
		Outcome:      journalOutcomeComplete, ObservedAt: claim.now,
	}, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// A delivery write failure must roll back the observation and execution result.
	if err := database.database.Exec("CREATE TRIGGER reject_journal_delivery BEFORE INSERT ON managed_journal_delivery_records BEGIN SELECT RAISE(ABORT, 'controlled_delivery_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := database.observeJournalAttempt(t.Context(), claim, evidence); err == nil {
		t.Fatal("delivery failure was ignored")
	}
	if read("/" + request.ID)["state"] != "executing" {
		t.Fatal("failed observation changed execution state")
	}
	var count int64
	if err := database.database.Model(&managedJournalObservationRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial observation count=%d error=%v", count, err)
	}
	if err := database.database.Exec("DROP TRIGGER reject_journal_delivery").Error; err != nil {
		t.Fatal(err)
	}
	observation, err := database.observeJournalAttempt(t.Context(), claim, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if replay, err := database.observeJournalAttempt(t.Context(), claim, evidence); err != nil || replay.ID != observation.ID {
		t.Fatalf("observation replay: %+v %v", replay, err)
	}
	if response := read("/" + request.ID); response["state"] != "completed" || response["usage_state"] != "complete" {
		t.Fatalf("observed request: %v", response)
	}
	connection, err := database.database.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := openJournalTransactionInstance(t, database)
	database.database = restarted.database
	pending, err := restarted.pendingJournalDeliveries(t.Context(), 10)
	if err != nil || len(pending) != 1 || pending[0].ID != observation.ID {
		t.Fatalf("pending evidence: %+v %v", pending, err)
	}
	if string(pending[0].Quantities) != `[{"dimension":"input_tokens","unit":"token","value":"9007199254740993"},{"dimension":"output_tokens","unit":"token","value":"0"}]` {
		t.Fatalf("quantity precision lost: %s", pending[0].Quantities)
	}
	if err := database.database.Exec("CREATE TABLE controlled_accounting_effects (observation_id TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	failure := errors.New("controlled accounting failure")
	if err := restarted.deliverJournalObservation(t.Context(), observation.ID, claim.now, func(tx *gorm.DB, observation managedJournalObservationRecord) error {
		if err := tx.Exec("INSERT INTO controlled_accounting_effects VALUES (?)", observation.ID).Error; err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatalf("accounting failure: %v", err)
	}
	apply := func(tx *gorm.DB, observation managedJournalObservationRecord) error {
		return tx.Exec("INSERT INTO controlled_accounting_effects VALUES (?)", observation.ID).Error
	}
	for range 2 {
		if err := restarted.deliverJournalObservation(t.Context(), observation.ID, claim.now, apply); err != nil {
			t.Fatal(err)
		}
	}
	if pending, err := restarted.pendingJournalDeliveries(t.Context(), 10); err != nil || len(pending) != 0 {
		t.Fatalf("delivered evidence remains pending: %+v %v", pending, err)
	}
	if err := database.database.Table("controlled_accounting_effects").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("accounting effects=%d error=%v", count, err)
	}
}

func TestHostedJournalUnknownUsageCannotAuthorizeContinuation(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { return nil }
	request, err := database.admitJournalRequest(t.Context(), intent("unknown"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := newJournalWorkerClaim(request.ID, request.OwnerToken, request.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-unknown", reserve)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.dispatchJournalAttempt(t.Context(), claim, attempt.ID); err != nil {
		t.Fatal(err)
	}
	evidence, err := newJournalUsageEvidence(journalUsageEvidenceInput{AttemptID: attempt.ID, AdapterRevision: "test-1", Quantities: []journalQuantity{{Dimension: "input_tokens", Unit: "token", UnknownReason: journalQuantityNotReported}}, Outcome: journalOutcomeContinue, ObservedAt: claim.now}, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.observeJournalAttempt(t.Context(), claim, evidence); err != nil {
		t.Fatal(err)
	}
	if response := read("/" + request.ID); response["state"] != "executing" || response["usage_state"] != "unknown" {
		t.Fatalf("unknown usage changed to zero: %v", response)
	}
	if _, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-after-unknown", reserve); !errors.Is(err, errUsageJournalConflict) {
		t.Fatalf("unknown usage authorized continuation: %v", err)
	}
	var cases int64
	if err := database.database.Model(&managedJournalCaseRecord{}).Where("request_id = ?", request.ID).Count(&cases).Error; err != nil || cases != 1 {
		t.Fatalf("unknown usage reconciliation cases=%d error=%v", cases, err)
	}
}
