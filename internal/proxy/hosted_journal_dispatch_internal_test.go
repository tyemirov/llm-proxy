package proxy

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedJournalDispatchRequiresClaimAndCurrentAuthority(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	accepted, err := database.admitJournalRequest(t.Context(), intent("dispatch"), func(*gorm.DB, managedJournalRequestRecord) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	claim, err := newJournalWorkerClaim(accepted.ID, accepted.OwnerToken, accepted.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	var reservations atomic.Int64
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { reservations.Add(1); return nil }
	attempt, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-first", reserve)
	if err != nil || attempt.State != journalAttemptPrepared || attempt.Number != 1 {
		t.Fatalf("prepare: attempt=%+v error=%v", attempt, err)
	}
	if replay, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-first", reserve); err != nil || replay.ID != attempt.ID || reservations.Load() != 1 {
		t.Fatalf("prepare replay: attempt=%+v reservations=%d error=%v", replay, reservations.Load(), err)
	}
	if replay, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-second", reserve); err != nil || replay.ID != attempt.ID || reservations.Load() != 1 {
		t.Fatalf("retained prepared attempt: attempt=%+v reservations=%d error=%v", replay, reservations.Load(), err)
	}
	stale, err := newJournalWorkerClaim(accepted.ID, "other-worker", claim.now)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.dispatchJournalAttempt(t.Context(), stale, attempt.ID); !errors.Is(err, errJournalClaimLost) {
		t.Fatalf("stale worker dispatch: %v", err)
	}
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", accepted.GrantID).Update("state", hostedGrantSuspended).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.dispatchJournalAttempt(t.Context(), claim, attempt.ID); !errors.Is(err, errHostedAuthorityDenied) {
		t.Fatalf("suspended grant dispatch: %v", err)
	}
	if read("/" + accepted.ID)["state"] != "accepted" {
		t.Fatal("rejected dispatch changed the request state")
	}
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", accepted.GrantID).Update("state", hostedGrantActive).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.dispatchJournalAttempt(t.Context(), claim, attempt.ID); err != nil {
		t.Fatal(err)
	}
	if read("/" + accepted.ID)["state"] != "executing" {
		t.Fatal("dispatch did not persist execution state")
	}
	if err := database.dispatchJournalAttempt(t.Context(), claim, attempt.ID); !errors.Is(err, errUsageJournalConflict) {
		t.Fatalf("duplicate dispatch authorized provider work again: %v", err)
	}
}

func TestHostedJournalExpiredDispatchRetainsUncertainty(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { return nil }
	accepted, err := database.admitJournalRequest(t.Context(), intent("crash"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := newJournalWorkerClaim(accepted.ID, accepted.OwnerToken, accepted.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-crash", reserve)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.dispatchJournalAttempt(t.Context(), claim, attempt.ID); err != nil {
		t.Fatal(err)
	}
	// A different service instance must leave the live worker alone.
	restarted := openJournalTransactionInstance(t, database)
	if err := restarted.recoverJournalDispatches(t.Context(), journalExecutionText, accepted.ClaimExpiresAt.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if read("/" + accepted.ID)["state"] != "executing" {
		t.Fatal("recovery stole a live worker claim")
	}
	for range 2 {
		if err := restarted.recoverJournalDispatches(t.Context(), journalExecutionText, accepted.ClaimExpiresAt); err != nil {
			t.Fatal(err)
		}
	}
	response := read("/" + accepted.ID)
	if response["state"] != "uncertain" || response["usage_state"] != "unknown" {
		t.Fatalf("recovery discarded uncertainty: %v", response)
	}
	var cases int64
	if err := database.database.Model(&managedJournalCaseRecord{}).Where("request_id = ?", accepted.ID).Count(&cases).Error; err != nil || cases != 1 {
		t.Fatalf("recovery cases=%d error=%v", cases, err)
	}
	if err := database.dispatchJournalAttempt(t.Context(), claim, attempt.ID); !errors.Is(err, errJournalClaimLost) {
		t.Fatalf("recovered worker dispatch: %v", err)
	}
	if _, err := restarted.prepareJournalAttempt(t.Context(), claim, "attempt-retry", reserve); !errors.Is(err, errJournalClaimLost) {
		t.Fatalf("uncertain work authorized a new paid attempt: %v", err)
	}
}
