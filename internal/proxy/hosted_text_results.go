package proxy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The database writer lock fences the filesystem projection against claim
// replacement and startup recovery. A filesystem rename and database commit
// are distinct effects; recovery repairs a missing publication receipt.
func (service *hostedTextRequests) publishResponse(ctx context.Context, accepted managedJournalRequestRecord, publish func(managedJournalRequestRecord) error) error {
	persistContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), journalPersistenceTimeout)
	defer cancel()
	now := service.now().UTC()
	return service.database.database.WithContext(persistContext).Transaction(func(transaction *gorm.DB) error {
		result := transaction.Model(&managedJournalRequestRecord{}).
			Where("id = ? AND owner_token = ? AND claim_expires_at > ?", accepted.ID, accepted.OwnerToken, now).
			UpdateColumn("state", gorm.Expr("state"))
		if result.Error != nil {
			return fmt.Errorf("lock result publication for request %s: %w", accepted.ID, result.Error)
		}
		if result.RowsAffected != 1 {
			return errJournalClaimLost
		}
		var current managedJournalRequestRecord
		if err := transaction.Where("id = ?", accepted.ID).First(&current).Error; err != nil {
			return fmt.Errorf("read result publication for request %s: %w", accepted.ID, err)
		}
		if err := publish(current); err != nil {
			return fmt.Errorf("publish response for request %s: %w", accepted.ID, err)
		}
		if current.State == journalRequestCompleted {
			if err := transaction.Model(&managedJournalRequestRecord{}).Where("id = ?", current.ID).UpdateColumn("result_published_at", now).Error; err != nil {
				return fmt.Errorf("record result publication for request %s: %w", current.ID, err)
			}
		}
		return nil
	})
}

func (service *hostedTextRequests) recoverResults(ctx context.Context) error {
	// Bound each database transaction. Successfully recovered rows leave the
	// candidate set, so a restart can continue without an in-memory cursor.
	const batchSize = 100
	for {
		var candidates []managedJournalRequestRecord
		now := service.now().UTC()
		err := service.database.database.WithContext(ctx).
			Where("execution_kind IN ? AND state = ? AND result_published_at IS NULL AND claim_expires_at <= ?", []journalExecutionKind{journalExecutionText, journalExecutionDictation}, journalRequestCompleted, now).
			Order("id").Limit(batchSize).Find(&candidates).Error
		if err != nil {
			return fmt.Errorf("read unpublished text results: %w", err)
		}
		for _, candidate := range candidates {
			if err := service.recoverResult(ctx, candidate.ID, now); err != nil {
				return err
			}
		}
		if len(candidates) < batchSize {
			return nil
		}
	}
}

func (service *hostedTextRequests) recoverResult(ctx context.Context, requestID string, now time.Time) error {
	return service.database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		result := transaction.Model(&managedJournalRequestRecord{}).
			Where("id = ? AND state = ? AND result_published_at IS NULL AND claim_expires_at <= ?", requestID, journalRequestCompleted, now).
			UpdateColumn("state", gorm.Expr("state"))
		if result.Error != nil {
			return fmt.Errorf("lock unpublished request %s: %w", requestID, result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		var request managedJournalRequestRecord
		if err := transaction.Where("id = ?", requestID).First(&request).Error; err != nil {
			return fmt.Errorf("read unpublished request %s: %w", requestID, err)
		}
		service.responses.mu.Lock()
		defer service.responses.mu.Unlock()
		path, err := service.responses.recordPath(sha256Hex(request.TenantID), request.KeyDigest, false)
		if err != nil {
			return err
		}
		record, err := service.responses.read(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read unpublished result %s: %w", requestID, err)
		}
		if err == nil {
			if err := matchHostedResult(request, record); err != nil {
				return err
			}
			if record.State == structuredRequestStateSucceeded {
				var stored hostedStoredCompletion
				if err := decodeStrictJSON(record.Result, &stored); err != nil {
					return fmt.Errorf("validate unpublished result %s: %w", requestID, err)
				}
				if err := transaction.Model(&managedJournalRequestRecord{}).Where("id = ?", requestID).UpdateColumn("result_published_at", now).Error; err != nil {
					return fmt.Errorf("recover result receipt %s: %w", requestID, err)
				}
				return nil
			}
		}
		if err := transaction.Model(&managedJournalRequestRecord{}).Where("id = ?", requestID).
			Updates(map[string]any{"state": journalRequestUncertain, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("retain unpublished result %s: %w", requestID, err)
		}
		reconciliation := managedJournalCaseRecord{ID: journalCaseIDPrefix + sha256Hex(requestID + ":" + journalCaseResultUnknown)[:32], RequestID: requestID, Reason: journalCaseResultUnknown, CreatedAt: now}
		if err := transaction.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&reconciliation).Error; err != nil {
			return fmt.Errorf("create result reconciliation %s: %w", requestID, err)
		}
		return nil
	})
}

func matchHostedResult(request managedJournalRequestRecord, record structuredRequestRecord) error {
	if record.ProxyRequestID != request.ExecutionID || record.IntentSHA256 != request.IntentDigest || record.TenantSHA256 != sha256Hex(request.TenantID) || record.IdempotencySHA256 != request.KeyDigest || record.Provider != request.Provider || record.Model != request.Model {
		return errUsageJournalConflict
	}
	return nil
}
