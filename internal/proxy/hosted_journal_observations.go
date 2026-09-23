package proxy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (database *gormManagedTenantDatabase) observeJournalAttempt(ctx context.Context, claim journalWorkerClaim, evidence journalUsageEvidence) (managedJournalObservationRecord, error) {
	var observation managedJournalObservationRecord
	err := database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		// Retained observations can be replayed after completion or claim expiry.
		lock := transaction.Model(&managedJournalRequestRecord{}).Where("id = ? AND owner_token = ?", claim.requestID, claim.owner).UpdateColumn("state", gorm.Expr("state"))
		if lock.Error != nil {
			return fmt.Errorf("lock observation request %s: %w", claim.requestID, lock.Error)
		}
		if lock.RowsAffected != 1 {
			return errJournalClaimLost
		}
		var attempt managedJournalAttemptRecord
		if err := transaction.Where("request_id = ? AND id = ?", claim.requestID, evidence.record.AttemptID).First(&attempt).Error; err != nil {
			return fmt.Errorf("read observation attempt %s: %w", evidence.record.AttemptID, err)
		}
		err := transaction.Where("attempt_id = ? AND evidence_digest = ?", attempt.ID, evidence.record.EvidenceDigest).First(&observation).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read observation for attempt %s: %w", attempt.ID, err)
		}
		request, err := lockJournalClaim(transaction, claim)
		if err != nil {
			return err
		}
		if attempt.State != journalAttemptDispatched {
			return errUsageJournalConflict
		}
		observation, err = persistJournalObservation(transaction, request, attempt, evidence, claim.now)
		return err
	})
	if err != nil {
		return managedJournalObservationRecord{}, err
	}
	return observation, nil
}

// The caller has locked and authorized its execution owner. Cancellation can
// use the same evidence transaction without borrowing an expired worker claim.
func persistJournalObservation(transaction *gorm.DB, request managedJournalRequestRecord, attempt managedJournalAttemptRecord, evidence journalUsageEvidence, now time.Time) (managedJournalObservationRecord, error) {
	observation := evidence.record
	if err := transaction.Omit(clause.Associations).Create(&observation).Error; err != nil {
		return managedJournalObservationRecord{}, fmt.Errorf("record observation %s: %w", observation.ID, err)
	}
	delivery := managedJournalDeliveryRecord{ObservationID: observation.ID, CreatedAt: now}
	if err := transaction.Omit(clause.Associations).Create(&delivery).Error; err != nil {
		return managedJournalObservationRecord{}, fmt.Errorf("record observation delivery %s: %w", observation.ID, err)
	}
	if err := transaction.Model(&managedJournalAttemptRecord{}).Where("id = ?", attempt.ID).
		Updates(map[string]any{"state": journalAttemptObserved, "observed_at": observation.ObservedAt, "provider_request_id": evidence.providerRequestID, "updated_at": now}).Error; err != nil {
		return managedJournalObservationRecord{}, fmt.Errorf("record observed attempt %s: %w", attempt.ID, err)
	}
	state := journalRequestExecuting
	switch observation.Outcome {
	case journalOutcomeComplete:
		state = journalRequestCompleted
	case journalOutcomeFail:
		state = journalRequestFailed
	}
	if err := transaction.Model(&managedJournalRequestRecord{}).Where("id = ?", request.ID).
		Updates(map[string]any{"state": state, "usage_state": observation.Completeness, "failure_code": observation.FailureCode, "updated_at": now}).Error; err != nil {
		return managedJournalObservationRecord{}, fmt.Errorf("record observed request %s: %w", request.ID, err)
	}
	if observation.Completeness == journalUsageUnknown {
		record := managedJournalCaseRecord{ID: journalCaseIDPrefix + sha256Hex(request.ID + ":" + journalCaseUsageUnknown)[:32], RequestID: request.ID, Reason: journalCaseUsageUnknown, CreatedAt: now}
		if err := transaction.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
			return managedJournalObservationRecord{}, fmt.Errorf("record unknown usage for request %s: %w", request.ID, err)
		}
	}
	return observation, nil
}

func (database *gormManagedTenantDatabase) pendingJournalDeliveries(ctx context.Context, limit int) ([]managedJournalObservationRecord, error) {
	if limit < 1 || limit > 100 {
		return nil, errUsageJournalInvalid
	}
	observations := []managedJournalObservationRecord{}
	err := database.database.WithContext(ctx).Where("id IN (?)", database.database.Model(&managedJournalDeliveryRecord{}).Select("observation_id").Where("delivered_at IS NULL")).
		Order("created_at, id").Limit(limit).Find(&observations).Error
	if err != nil {
		return nil, fmt.Errorf("read pending usage delivery: %w", err)
	}
	return observations, nil
}

type journalAccountingDelivery func(*gorm.DB, managedJournalObservationRecord) error

func (database *gormManagedTenantDatabase) deliverJournalObservation(ctx context.Context, observationID string, now time.Time, apply journalAccountingDelivery) error {
	return database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		// Delivery and destination effects share the transaction. A failed destination
		// leaves the delivery pending, including when it wrote before returning an error.
		lock := transaction.Model(&managedJournalDeliveryRecord{}).Where("observation_id = ?", observationID).UpdateColumn("observation_id", gorm.Expr("observation_id"))
		if lock.Error != nil {
			return fmt.Errorf("lock usage delivery %s: %w", observationID, lock.Error)
		}
		if lock.RowsAffected != 1 {
			return errUsageJournalNotFound
		}
		var delivery managedJournalDeliveryRecord
		if err := transaction.Where("observation_id = ?", observationID).First(&delivery).Error; err != nil {
			return fmt.Errorf("read usage delivery %s: %w", observationID, err)
		}
		if delivery.DeliveredAt != nil {
			return nil
		}
		var observation managedJournalObservationRecord
		if err := transaction.Where("id = ?", observationID).First(&observation).Error; err != nil {
			return fmt.Errorf("read usage evidence %s: %w", observationID, err)
		}
		if err := apply(transaction, observation); err != nil {
			return fmt.Errorf("apply usage delivery %s: %w", observationID, err)
		}
		if err := transaction.Model(&managedJournalDeliveryRecord{}).Where("observation_id = ?", observationID).UpdateColumn("delivered_at", now).Error; err != nil {
			return fmt.Errorf("complete usage delivery %s: %w", observationID, err)
		}
		return nil
	})
}
