package proxy

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

type hostedMediaRequestContextKey struct{}

func (store *mediaOperationStore) hostedMediaRequest(transaction *gorm.DB, operationID string) (*managedJournalRequestRecord, error) {
	if store.journal == nil {
		return nil, nil
	}
	var record managedJournalRequestRecord
	err := transaction.Where("execution_kind = ? AND execution_id = ?", journalExecutionMedia, operationID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read media journal for operation %s: %w", operationID, err)
	}
	return &record, nil
}

func mediaJournalWorkerToken(operationID string, generation uint64) string {
	return operationID + ":" + strconv.FormatUint(generation, 10)
}

// Media worker generation and journal authority have one transaction owner.
func (store *mediaOperationStore) bindMediaJournalClaim(transaction *gorm.DB, claim mediaOperationClaimRecord, now time.Time) error {
	request, err := store.hostedMediaRequest(transaction, claim.OperationID)
	if err != nil || request == nil {
		return err
	}
	result := transaction.Model(&managedJournalRequestRecord{}).
		Where("id = ? AND state IN ?", request.ID, []journalRequestState{journalRequestAccepted, journalRequestExecuting}).
		Updates(map[string]any{"owner_token": mediaJournalWorkerToken(claim.OperationID, claim.Generation), "claim_expires_at": claim.ExpiresAt, "updated_at": now})
	if result.Error != nil {
		return fmt.Errorf("bind media claim for request %s: %w", request.ID, result.Error)
	}
	if result.RowsAffected != 1 {
		return errJournalClaimLost
	}
	return nil
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

func (service *mediaOperationService) renewMediaClaim(operationID string, generation uint64) error {
	now := service.store.now()
	return service.store.database.Transaction(func(transaction *gorm.DB) error {
		if err := lockMediaOperationClaim(transaction, operationID, generation, now); err != nil {
			return err
		}
		expiresAt := now.Add(service.claimLifetime)
		if err := transaction.Model(&mediaOperationClaimRecord{}).Where("operation_id = ? AND generation = ?", operationID, generation).Update("expires_at", expiresAt).Error; err != nil {
			return fmt.Errorf("renew media claim for operation %s: %w", operationID, err)
		}
		return service.store.bindMediaJournalClaim(transaction, mediaOperationClaimRecord{OperationID: operationID, Generation: generation, ExpiresAt: expiresAt}, now)
	})
}

func (service *mediaOperationService) dispatchHostedMedia(transaction *gorm.DB, operationID string, generation uint64, now time.Time) error {
	request, err := service.store.hostedMediaRequest(transaction, operationID)
	if err != nil || request == nil {
		return err
	}
	if service.hostedAdmission == nil {
		return errHostedAuthorityDenied
	}
	claim := journalWorkerClaim{requestID: request.ID, owner: mediaJournalWorkerToken(operationID, generation), now: now}
	identifier, err := newHostedResourceID(journalAttemptIDPrefix, rand.Reader)
	if err != nil {
		return err
	}
	// GORM uses savepoints for these nested transactions. The outer media
	// transaction owns the final commit of authorization and dispatch intent.
	journal := &gormManagedTenantDatabase{database: transaction}
	attempt, err := journal.prepareJournalAttempt(transaction.Statement.Context, claim, identifier, service.hostedAdmission)
	if err != nil {
		return err
	}
	return journal.dispatchJournalAttempt(transaction.Statement.Context, claim, attempt.ID)
}

func (store *mediaOperationStore) finishHostedMedia(transaction *gorm.DB, operation mediaOperationRecord, now time.Time) error {
	request, err := store.hostedMediaRequest(transaction, operation.OperationID)
	if err != nil || request == nil {
		return err
	}
	state, failure := journalRequestFailed, operation.PublicErrorCode
	switch operation.PublicState {
	case MediaOperationStateSucceeded:
		state, failure = journalRequestCompleted, ""
	case MediaOperationStateCancelled:
		failure = "operation_cancelled"
	case MediaOperationStateUncertain:
		if err := retainInterruptedJournalExecution(transaction, request, now); err != nil {
			return err
		}
		state = journalRequestUncertain
	}
	if state != journalRequestUncertain {
		var attempts []managedJournalAttemptRecord
		if err := transaction.Where("request_id = ? AND state = ?", request.ID, journalAttemptDispatched).Find(&attempts).Error; err != nil {
			return fmt.Errorf("read media attempts for request %s: %w", request.ID, err)
		}
		for _, attempt := range attempts {
			evidence, err := newJournalUsageEvidence(journalUsageEvidenceInput{
				AttemptID: attempt.ID, AdapterRevision: "media-execution:1", ProviderRequestID: attempt.ProviderRequestID,
				Quantities: []journalQuantity{{Dimension: "provider_usage", Unit: "units", UnknownReason: journalQuantityUnsupported}},
				Outcome:    journalOutcomeContinue, ObservedAt: now,
			}, rand.Reader)
			if err != nil {
				return err
			}
			if _, err := persistJournalObservation(transaction, *request, attempt, evidence, now); err != nil {
				return err
			}
		}
	}
	updates := map[string]any{"state": state, "failure_code": failure, "updated_at": now}
	if state == journalRequestCompleted {
		updates["result_published_at"] = now
	}
	if err := transaction.Model(&managedJournalRequestRecord{}).Where("id = ?", request.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("finish media journal request %s: %w", request.ID, err)
	}
	return nil
}

type hostedMediaAuthorizationContextKey struct{}
type hostedMediaAuthorize func(context.Context, hostedProviderRole) error

type hostedMediaDispatchAuthority struct {
	service        *mediaOperationService
	operationID    string
	generation     uint64
	submissionRole hostedProviderRole
	submitted      atomic.Bool
}

func (authority *hostedMediaDispatchAuthority) authorize(ctx context.Context, role hostedProviderRole) error {
	switch role {
	case hostedProviderGeneration:
		if authority.submissionRole != hostedProviderGeneration || !authority.submitted.CompareAndSwap(false, true) {
			return errHostedAuthorityDenied
		}
	case hostedProviderStaging:
		if authority.submissionRole != hostedProviderGeneration {
			return errHostedAuthorityDenied
		}
	case hostedProviderObservation, hostedProviderAuxiliary:
	default:
		return errHostedAuthorityDenied
	}
	now := authority.service.store.now()
	return authority.service.store.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := lockMediaOperationClaim(transaction, authority.operationID, authority.generation, now); err != nil {
			return err
		}
		journal, err := authority.service.store.hostedMediaRequest(transaction, authority.operationID)
		if err != nil {
			return err
		}
		if journal == nil || journal.OwnerToken != mediaJournalWorkerToken(authority.operationID, authority.generation) {
			return errJournalClaimLost
		}
		if role == hostedProviderGeneration || role == hostedProviderStaging {
			return requireJournalAuthority(transaction, *journal)
		}
		return nil
	})
}

type hostedMediaHTTPDoer struct {
	next      HTTPDoer
	authorize hostedMediaAuthorize
	role      hostedProviderRole
}

func (doer *hostedMediaHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	if doer.authorize == nil || (doer.role == hostedProviderGeneration && request.Method != http.MethodPost) || (doer.role == hostedProviderMetadata && request.Method != http.MethodGet) {
		return nil, errHostedAuthorityDenied
	}
	if err := doer.authorize(request.Context(), doer.role); err != nil {
		return nil, err
	}
	return doer.next.Do(request)
}

func (service *mediaOperationService) hostedMediaCancellationAuthority(operationID string) hostedMediaAuthorize {
	return func(ctx context.Context, role hostedProviderRole) error {
		if role != hostedProviderCancellation {
			return errHostedAuthorityDenied
		}
		var count int64
		if err := service.store.database.WithContext(ctx).Model(&mediaOperationRecord{}).Where("operation_id = ? AND public_state = ? AND cancellation_state = ?", operationID, MediaOperationStateRunning, MediaCancellationRequested).Count(&count).Error; err != nil {
			return fmt.Errorf("authorize cancellation for operation %s: %w", operationID, err)
		}
		if count != 1 {
			return errJournalClaimLost
		}
		return nil
	}
}

// Provider adapters use one connection resolver for accepted hosted authority
// and explicit customer-owned assignments. Hosted reads keep the pinned version.
func mediaConnectionSettings(ctx context.Context, tenantID, providerName, reference string, tenants *managedTenantStore, store *mediaOperationStore) (managedProviderSettings, string, error) {
	if request, hosted := ctx.Value(hostedMediaRequestContextKey{}).(managedJournalRequestRecord); hosted {
		if request.TenantID != tenantID || request.Provider != providerName || mediaJournalCredentialReference(request) != reference {
			return managedProviderSettings{}, "", errHostedAuthorityDenied
		}
		definition := tenants.routingDefaults.definitions[providerID(providerName)]
		provider, err := loadJournalProvider(ctx, store.database, tenants.providerKeyCipher, request, definition)
		if err != nil {
			return managedProviderSettings{}, "", err
		}
		return managedProviderSettings{connectionValues: provider.connectionValues}, request.PlatformConnectionID, nil
	}
	var assignment managedTenantConnectionRecord
	err := store.database.WithContext(ctx).Preload("Connection.Fields").Where("tenant_id = ? AND provider_id = ?", tenantID, providerName).First(&assignment).Error
	if err != nil {
		return managedProviderSettings{}, "", errMediaOperationUnavailable
	}
	if reference != "" && assignment.ConnectionID+":v"+strconv.FormatUint(assignment.Connection.Version, 10) != reference {
		return managedProviderSettings{}, "", errMediaOperationUnavailable
	}
	settings, err := tenants.accountConnectionSettings(assignment.Connection)
	return settings, assignment.ConnectionID, err
}
