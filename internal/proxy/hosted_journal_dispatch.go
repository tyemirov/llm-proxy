package proxy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errJournalClaimLost = errors.New(llmproxycontract.ErrorCodeUsageJournalClaimLost)

type journalWorkerClaim struct {
	requestID string
	owner     string
	now       time.Time
}

func newJournalWorkerClaim(requestID, owner string, now time.Time) (journalWorkerClaim, error) {
	if !strings.HasPrefix(requestID, journalRequestIDPrefix) || owner == "" || now.IsZero() {
		return journalWorkerClaim{}, errUsageJournalInvalid
	}
	return journalWorkerClaim{requestID: requestID, owner: owner, now: now.UTC()}, nil
}

func lockJournalClaim(transaction *gorm.DB, claim journalWorkerClaim) (managedJournalRequestRecord, error) {
	result := transaction.Model(&managedJournalRequestRecord{}).
		Where("id = ? AND owner_token = ? AND claim_expires_at > ? AND state IN ?", claim.requestID, claim.owner, claim.now, []journalRequestState{journalRequestAccepted, journalRequestExecuting}).
		UpdateColumn("state", gorm.Expr("state"))
	if result.Error != nil {
		return managedJournalRequestRecord{}, fmt.Errorf("lock journal request %s: %w", claim.requestID, result.Error)
	}
	if result.RowsAffected != 1 {
		return managedJournalRequestRecord{}, errJournalClaimLost
	}
	var record managedJournalRequestRecord
	if err := transaction.Where("id = ?", claim.requestID).First(&record).Error; err != nil {
		return record, fmt.Errorf("read claimed request %s: %w", claim.requestID, err)
	}
	return record, nil
}

func requireJournalAuthority(transaction *gorm.DB, request managedJournalRequestRecord) error {
	var count int64
	err := transaction.Model(&managedHostedGrantRecord{}).
		Where("id = ? AND tenant_id = ? AND provider = ? AND billing_account_id = ? AND platform_connection_id = ? AND state = ?", request.GrantID, request.TenantID, request.Provider, request.BillingAccountID, request.PlatformConnectionID, hostedGrantActive).
		Where("id IN (?)", transaction.Model(&managedHostedTenantAssignmentRecord{}).Select("grant_id").Where("tenant_id = ? AND provider_id = ?", request.TenantID, request.Provider)).Count(&count).Error
	if err != nil {
		return fmt.Errorf("read dispatch authority for request %s: %w", request.ID, err)
	}
	if count != 1 {
		return errHostedAuthorityDenied
	}
	return nil
}

func (database *gormManagedTenantDatabase) prepareJournalAttempt(ctx context.Context, claim journalWorkerClaim, attemptID string, reserve journalReservation) (managedJournalAttemptRecord, error) {
	var attempt managedJournalAttemptRecord
	err := database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		request, err := lockJournalClaim(transaction, claim)
		if err != nil {
			return err
		}
		if err := requireJournalAuthority(transaction, request); err != nil {
			return err
		}
		err = transaction.Where("request_id = ? AND id = ?", request.ID, attemptID).First(&attempt).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read attempt %s: %w", attemptID, err)
		}
		var previous managedJournalAttemptRecord
		err = transaction.Where("request_id = ?", request.ID).Order("number DESC").First(&previous).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read prior attempt for request %s: %w", request.ID, err)
		}
		if previous.State == journalAttemptPrepared && request.State == journalRequestAccepted {
			attempt = previous
			return nil
		}
		if previous.ID != "" && (previous.State != journalAttemptObserved || request.UsageState != journalUsageComplete) {
			return errUsageJournalConflict
		}
		attempt = managedJournalAttemptRecord{ID: attemptID, RequestID: request.ID, Number: previous.Number + 1, State: journalAttemptPrepared, CreatedAt: claim.now, UpdatedAt: claim.now}
		if err := transaction.Omit(clause.Associations).Create(&attempt).Error; err != nil {
			return fmt.Errorf("prepare attempt %s: %w", attemptID, err)
		}
		if err := reserve(transaction, request); err != nil {
			return fmt.Errorf("authorize funds for attempt %s: %w", attemptID, err)
		}
		return nil
	})
	if err != nil {
		return managedJournalAttemptRecord{}, err
	}
	return attempt, nil
}

// A successful return permits exactly one provider dispatch. A repeated call is
// a conflict, because a lost response cannot establish whether the provider ran.
func (database *gormManagedTenantDatabase) dispatchJournalAttempt(ctx context.Context, claim journalWorkerClaim, attemptID string) error {
	return database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		request, err := lockJournalClaim(transaction, claim)
		if err != nil {
			return err
		}
		if err := requireJournalAuthority(transaction, request); err != nil {
			return err
		}
		result := transaction.Model(&managedJournalAttemptRecord{}).Where("request_id = ? AND id = ? AND state = ?", request.ID, attemptID, journalAttemptPrepared).
			Updates(map[string]any{"state": journalAttemptDispatched, "dispatch_at": claim.now, "updated_at": claim.now})
		if result.Error != nil {
			return fmt.Errorf("record dispatch for attempt %s: %w", attemptID, result.Error)
		}
		if result.RowsAffected != 1 {
			return errUsageJournalConflict
		}
		if err := transaction.Model(&managedJournalRequestRecord{}).Where("id = ?", request.ID).
			Updates(map[string]any{"state": journalRequestExecuting, "usage_state": journalUsagePending, "updated_at": claim.now}).Error; err != nil {
			return fmt.Errorf("record execution for request %s: %w", request.ID, err)
		}
		return nil
	})
}

func (database *gormManagedTenantDatabase) recoverJournalDispatches(ctx context.Context, kind journalExecutionKind, now time.Time) error {
	return database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		// Serialize recovery with worker transitions before reading the expired set.
		query := transaction.Model(&managedJournalRequestRecord{}).
			Where("execution_kind = ? AND claim_expires_at <= ? AND state = ?", kind, now, journalRequestExecuting)
		if err := query.UpdateColumn("state", gorm.Expr("state")).Error; err != nil {
			return fmt.Errorf("lock expired journal dispatches: %w", err)
		}
		var requests []managedJournalRequestRecord
		if err := query.Find(&requests).Error; err != nil {
			return fmt.Errorf("read expired journal dispatches: %w", err)
		}
		for _, request := range requests {
			if err := retainInterruptedJournalExecution(transaction, &request, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func recoverJournalAdmission(transaction *gorm.DB, accepted *managedJournalRequestRecord, proposal managedJournalRequestRecord) error {
	if accepted.ClaimExpiresAt.After(proposal.CreatedAt) {
		return nil
	}
	switch accepted.State {
	case journalRequestAccepted:
		if err := requireJournalAuthority(transaction, *accepted); err != nil {
			return err
		}
		var dispatched int64
		if err := transaction.Model(&managedJournalAttemptRecord{}).Where("request_id = ? AND state <> ?", accepted.ID, journalAttemptPrepared).Count(&dispatched).Error; err != nil {
			return fmt.Errorf("read accepted attempts for request %s: %w", accepted.ID, err)
		}
		if dispatched != 0 {
			return errUsageJournalConflict
		}
		if err := transaction.Model(&managedJournalRequestRecord{}).Where("id = ?", accepted.ID).Updates(map[string]any{"owner_token": proposal.OwnerToken, "claim_expires_at": proposal.ClaimExpiresAt, "updated_at": proposal.CreatedAt}).Error; err != nil {
			return fmt.Errorf("reclaim accepted request %s: %w", accepted.ID, err)
		}
		accepted.OwnerToken, accepted.ClaimExpiresAt, accepted.UpdatedAt = proposal.OwnerToken, proposal.ClaimExpiresAt, proposal.CreatedAt
	case journalRequestExecuting:
		return retainInterruptedJournalExecution(transaction, accepted, proposal.CreatedAt)
	}
	return nil
}

func retainInterruptedJournalExecution(transaction *gorm.DB, request *managedJournalRequestRecord, now time.Time) error {
	reason := journalCaseExecutionUnknown
	result := transaction.Model(&managedJournalAttemptRecord{}).Where("request_id = ? AND state = ?", request.ID, journalAttemptDispatched).
		Updates(map[string]any{"state": journalAttemptUncertain, "updated_at": now})
	if result.Error != nil {
		return fmt.Errorf("retain interrupted attempts for request %s: %w", request.ID, result.Error)
	}
	if result.RowsAffected > 0 {
		request.UsageState, reason = journalUsageUnknown, journalCaseDispatchUnknown
	}
	request.State, request.UpdatedAt = journalRequestUncertain, now
	if err := transaction.Model(&managedJournalRequestRecord{}).Where("id = ?", request.ID).
		Updates(map[string]any{"state": request.State, "usage_state": request.UsageState, "updated_at": now}).Error; err != nil {
		return fmt.Errorf("retain interrupted request %s: %w", request.ID, err)
	}
	record := managedJournalCaseRecord{ID: journalCaseIDPrefix + sha256Hex(request.ID + ":" + reason)[:32], RequestID: request.ID, Reason: reason, CreatedAt: now}
	if err := transaction.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
		return fmt.Errorf("create execution reconciliation for request %s: %w", request.ID, err)
	}
	return nil
}
