package proxy

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const fundsRecoveryBatchSize = 100
const fundsUndispatchedExpired = "undispatched_request_expired"

// Each effect has one database transaction. A later run resumes retained
// deliveries and reservations without a process-local recovery checkpoint.
func (database *gormManagedTenantDatabase) reconcileHostedFunds(ctx context.Context, now time.Time) error {
	for {
		var observations []managedJournalObservationRecord
		fundedAttempts := database.database.Model(&managedJournalAttemptRecord{}).Select("id").
			Where("request_id IN (?)", database.database.Model(&managedFundsReservationRecord{}).Select("request_id"))
		pending := database.database.Model(&managedJournalDeliveryRecord{}).Select("observation_id").Where("delivered_at IS NULL")
		if err := database.database.WithContext(ctx).Where("attempt_id IN (?) AND id IN (?)", fundedAttempts, pending).
			Order("created_at, id").Limit(fundsRecoveryBatchSize).Find(&observations).Error; err != nil {
			return fmt.Errorf("read funded usage deliveries: %w", err)
		}
		for _, observation := range observations {
			if err := database.deliverJournalObservation(ctx, observation.ID, now, deliverHostedFunds); err != nil {
				return fmt.Errorf("recover funded usage %s: %w", observation.ID, err)
			}
		}
		if len(observations) < fundsRecoveryBatchSize {
			break
		}
	}
	cursor := ""
	for {
		var reservations []managedFundsReservationRecord
		if err := database.database.WithContext(ctx).Where("state = ? AND request_id > ?", fundsReservationHeld, cursor).
			Order("request_id").Limit(fundsRecoveryBatchSize).Find(&reservations).Error; err != nil {
			return fmt.Errorf("read held funds for recovery: %w", err)
		}
		for _, reservation := range reservations {
			if err := database.reconcileHostedReservation(ctx, reservation.RequestID, now); err != nil {
				return err
			}
			cursor = reservation.RequestID
		}
		if len(reservations) < fundsRecoveryBatchSize {
			return nil
		}
	}
}

func (database *gormManagedTenantDatabase) reconcileHostedReservation(ctx context.Context, requestID string, now time.Time) error {
	return database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		// Fence dispatch and claim renewal before examining retained evidence.
		lock := transaction.Model(&managedJournalRequestRecord{}).Where("id = ?", requestID).UpdateColumn("state", gorm.Expr("state"))
		if lock.Error != nil {
			return fmt.Errorf("lock funds recovery request %s: %w", requestID, lock.Error)
		}
		var request managedJournalRequestRecord
		if err := transaction.Where("id = ?", requestID).First(&request).Error; err != nil {
			return fmt.Errorf("read funds recovery request %s: %w", requestID, err)
		}
		var reservation managedFundsReservationRecord
		if err := transaction.Where("request_id = ? AND billing_account_id = ?", request.ID, request.BillingAccountID).First(&reservation).Error; err != nil {
			return fmt.Errorf("read recovery reservation %s: %w", requestID, err)
		}
		if reservation.State != fundsReservationHeld {
			return nil
		}
		if (request.State == journalRequestAccepted || request.State == journalRequestExecuting) && request.ClaimExpiresAt.After(now) {
			return nil
		}
		var dispatched int64
		if err := transaction.Model(&managedJournalAttemptRecord{}).Where("request_id = ? AND (state <> ? OR dispatch_at IS NOT NULL)", request.ID, journalAttemptPrepared).Count(&dispatched).Error; err != nil {
			return fmt.Errorf("read dispatch evidence for reservation %s: %w", requestID, err)
		}
		if dispatched == 0 {
			// Close expired admission before releasing its hold. A stale worker
			// cannot reacquire this request or dispatch with released funds.
			if request.State == journalRequestAccepted || request.State == journalRequestExecuting {
				if err := transaction.Model(&managedJournalRequestRecord{}).Where("id = ?", request.ID).
					Updates(map[string]any{"state": journalRequestFailed, "failure_code": fundsUndispatchedExpired, "updated_at": now}).Error; err != nil {
					return fmt.Errorf("close undispatched request %s: %w", requestID, err)
				}
			}
			if err := postHostedFundsSettlement(transaction, reservation, 0, now); err != nil {
				return err
			}
			return updateHostedFundsReservation(transaction, reservation, fundsReservationReleased, now)
		}
		switch request.State {
		case journalRequestCompleted:
			return settleHostedFunds(transaction, request.ID, now)
		case journalRequestAccepted, journalRequestExecuting:
			if err := retainInterruptedJournalExecution(transaction, &request, now); err != nil {
				return err
			}
			return retainHostedFundsForReconciliation(transaction, reservation, journalCaseDispatchUnknown, now)
		case journalRequestUncertain:
			reason := journalCaseExecutionUnknown
			if request.UsageState == journalUsageUnknown {
				reason = journalCaseDispatchUnknown
			}
			return retainHostedFundsForReconciliation(transaction, reservation, reason, now)
		case journalRequestFailed:
			return retainHostedFundsForReconciliation(transaction, reservation, chargePolicyUnresolved, now)
		default:
			return fmt.Errorf("invalid request state for funds recovery %s", requestID)
		}
	})
}
