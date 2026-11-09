package proxy

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Lock queries stay on the caller's transaction. SQLite acquires its writer
// before subsequent reads; the predicates also fence ownership and claim expiry.
// Workflows request domain scopes through the named operations below.
func lockManagedRows(query *gorm.DB, column string) *gorm.DB {
	return query.UpdateColumn(column, gorm.Expr(column))
}

func retainManagedPaymentReconciliationItems(transaction *gorm.DB, run managedPaymentReconciliationRunRecord) *gorm.DB {
	return transaction.Exec("INSERT INTO managed_payment_reconciliation_item_records (run_id, order_id, state, result, evidence, evidence_digest) SELECT ?, id, ?, '', '', '' FROM managed_funding_order_records WHERE environment = ? AND processor_account_id = ? AND supplier_id = ?", run.ID, paymentReconciliationPending, run.Environment, run.ProcessorAccountID, run.SupplierID)
}

func lockManagedBillingAccount(transaction *gorm.DB, accountID string) *gorm.DB {
	return lockManagedRows(transaction.Model(&managedBillingAccountRecord{}).Where("id = ?", accountID), "id")
}

func lockManagedPlatformConnection(transaction *gorm.DB, connectionID string) *gorm.DB {
	return lockManagedRows(transaction.Model(&managedPlatformConnectionRecord{}).Where("id = ?", connectionID), "id")
}

func lockManagedObservationRequest(transaction *gorm.DB, claim journalWorkerClaim) *gorm.DB {
	return lockManagedRows(transaction.Model(&managedJournalRequestRecord{}).Where("id = ? AND owner_token = ?", claim.requestID, claim.owner), "state")
}

func lockManagedReservationRequest(transaction *gorm.DB, requestID string) *gorm.DB {
	return lockManagedRows(transaction.Model(&managedJournalRequestRecord{}).Where("id = ?", requestID), "state")
}

func lockManagedResolutionRequest(transaction *gorm.DB, decision managedFundsResolutionRecord) *gorm.DB {
	return lockManagedRows(transaction.Model(&managedJournalRequestRecord{}).Where("id = ? AND billing_account_id = ?", decision.RequestID, decision.BillingAccountID), "state")
}

func lockManagedPublicationRequest(transaction *gorm.DB, accepted managedJournalRequestRecord, now time.Time) *gorm.DB {
	return lockManagedRows(transaction.Model(&managedJournalRequestRecord{}).
		Where("id = ? AND owner_token = ? AND claim_expires_at > ?", accepted.ID, accepted.OwnerToken, now), "state")
}

func lockManagedUnpublishedRequest(transaction *gorm.DB, requestID string, now time.Time) *gorm.DB {
	return lockManagedRows(transaction.Model(&managedJournalRequestRecord{}).
		Where("id = ? AND state = ? AND result_published_at IS NULL AND claim_expires_at <= ?", requestID, journalRequestCompleted, now), "state")
}

func lockManagedResponseRecovery(transaction *gorm.DB) *gorm.DB {
	return lockManagedRows(transaction.Model(&managedJournalRequestRecord{}).Where("execution_kind = ?", journalExecutionText), "state")
}

func lockManagedJournalDelivery(transaction *gorm.DB, observationID string) *gorm.DB {
	return lockManagedRows(transaction.Model(&managedJournalDeliveryRecord{}).Where("observation_id = ?", observationID), "observation_id")
}

func lockManagedTenantMediaOperation(transaction *gorm.DB, operationID, tenantID string) *gorm.DB {
	return lockManagedRows(transaction.Model(&mediaOperationRecord{}).Where("operation_id = ? AND tenant_id = ?", operationID, tenantID), "public_state")
}

func lockManagedActiveMediaOperation(transaction *gorm.DB, operationID string) *gorm.DB {
	return lockManagedRows(transaction.Model(&mediaOperationRecord{}).
		Where("operation_id = ? AND public_state IN ?", operationID, []string{MediaOperationStateQueued, MediaOperationStateRunning}), "public_state")
}

func readManagedMediaOperationForUpdate(transaction *gorm.DB, operationID string, record *mediaOperationRecord) error {
	return transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(record, "operation_id = ?", operationID).Error
}

func readManagedMediaParent(transaction *gorm.DB, record mediaOperationRecord, parent *mediaOperationRecord) error {
	return transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(parent,
		"operation_id = ? AND tenant_id = ? AND provider = ? AND model = ? AND credential_reference = ? AND public_state = ?",
		record.ParentOperationID, record.TenantID, record.Provider, record.Model, record.CredentialReference, MediaOperationStateSucceeded).Error
}

func readManagedExpiredDispatches(transaction *gorm.DB, kind journalExecutionKind, now time.Time) ([]managedJournalRequestRecord, error) {
	query := transaction.Model(&managedJournalRequestRecord{}).
		Where("execution_kind = ? AND claim_expires_at <= ? AND state = ?", kind, now, journalRequestExecuting)
	if err := lockManagedRows(query, "state").Error; err != nil {
		return nil, fmt.Errorf("lock expired journal dispatches: %w", err)
	}
	var requests []managedJournalRequestRecord
	if err := query.Find(&requests).Error; err != nil {
		return nil, fmt.Errorf("read expired journal dispatches: %w", err)
	}
	return requests, nil
}

func lockProviderAssignmentTenant(transaction *gorm.DB, owner, tenantID string) (managedTenantRecord, error) {
	// Acquire the SQLite writer before reading either assignment subtype.
	// A read-first transaction cannot safely upgrade its snapshot after another writer commits.
	result := transaction.Model(&managedTenantRecord{}).Where("owner_user_id = ? AND tenant_id = ?", owner, tenantID).
		UpdateColumn("tenant_id", gorm.Expr("tenant_id"))
	if result.Error != nil {
		return managedTenantRecord{}, fmt.Errorf("lock provider assignments for tenant %s: %w", tenantID, result.Error)
	}
	if result.RowsAffected != 1 {
		return managedTenantRecord{}, errManagedTenantNotFound
	}
	var record managedTenantRecord
	if err := transaction.Where("owner_user_id = ? AND tenant_id = ?", owner, tenantID).First(&record).Error; err != nil {
		return record, managedTenantQueryError(owner, tenantID, err)
	}
	return record, nil
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

func lockMediaOperationClaim(transaction *gorm.DB, operationID string, generation uint64, now time.Time) error {
	result := transaction.Model(&mediaOperationClaimRecord{}).
		Where("operation_id = ? AND generation = ? AND expires_at > ?", operationID, generation, now).
		UpdateColumn("generation", gorm.Expr("generation"))
	if result.Error != nil {
		return fmt.Errorf("lock media claim for operation %s: %w", operationID, result.Error)
	}
	if result.RowsAffected != 1 {
		return errJournalClaimLost
	}
	return nil
}
