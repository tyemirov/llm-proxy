package proxy

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"

	"gorm.io/gorm"
)

type hostedMediaReservation func(*gorm.DB, managedJournalRequestRecord, mediaOperationRecord) error

// Validation can depend on a private credential reference, for example when a
// request names a prior provider result. Admission rechecks this reference under
// the database writer lock before reserving funds or creating an operation.
func (service *mediaOperationService) hostedMediaValidationContext(ctx context.Context, requestTenant tenant, payload mediaOperationCreatePayload) (context.Context, string, error) {
	request := managedJournalRequestRecord{
		TenantID: requestTenant.identifier.string(), Provider: payload.Provider, Model: payload.Model,
		Operation: catalogOperationForMediaCapability(payload.Capability), CatalogRevision: service.catalog.Revision(), CreatedAt: service.store.now(),
	}
	if err := bindJournalAuthority(service.store.database.WithContext(ctx), requestTenant.userID, &request); err != nil {
		return ctx, "", fmt.Errorf("%w: resolve media grant: %w", errMediaOperationUnavailable, err)
	}
	return context.WithValue(ctx, hostedMediaRequestContextKey{}, request), mediaJournalCredentialReference(request), nil
}

func mediaJournalCredentialReference(request managedJournalRequestRecord) string {
	return request.PlatformConnectionID + ":v" + strconv.FormatUint(request.CredentialVersion, 10)
}

func (service *mediaOperationService) admitHostedMedia(transaction *gorm.DB, requestTenant tenant, key string, canonicalIntent []byte, operation mediaOperationRecord) error {
	intent, err := newJournalAdmissionIntent(journalAdmissionInput{
		OwnerUserID: requestTenant.userID, TenantID: operation.TenantID, Provider: operation.Provider,
		Model: operation.Model, Operation: operation.CatalogOperation, CatalogRevision: operation.CatalogRevision,
		IdempotencyKey: key, CanonicalIntent: canonicalIntent, ExecutionKind: journalExecutionMedia, ExecutionID: operation.OperationID,
		OwnerToken: operation.OperationID, Now: operation.AcceptedAt, ClaimExpiresAt: operation.DeadlineAt,
	}, rand.Reader)
	if err != nil {
		return fmt.Errorf("%w: create media journal intent: %w", errMediaOperationStore, err)
	}
	accepted, err := admitJournalRequest(transaction, intent, func(transaction *gorm.DB, request managedJournalRequestRecord) error {
		if mediaJournalCredentialReference(request) != operation.CredentialReference {
			return errHostedAuthorityDenied
		}
		return service.hostedAdmission(transaction, request, operation)
	})
	if errors.Is(err, errUsageJournalConflict) {
		return errMediaOperationIntentConflict
	}
	if errors.Is(err, errInsufficientFunds) || errors.Is(err, errFinancialAdmissionUnavailable) || errors.Is(err, errFinancialAccountSuspended) {
		return err
	}
	if errors.Is(err, errHostedAuthorityDenied) || errors.Is(err, ErrCatalogRatingUnavailable) {
		return errMediaOperationUnavailable
	}
	if err != nil {
		return fmt.Errorf("%w: admit media journal request: %w", errMediaOperationStore, err)
	}
	if accepted.ExecutionID != operation.OperationID {
		return fmt.Errorf("%w: media operation missing for retained journal request %s", errMediaOperationStore, accepted.ID)
	}
	return nil
}
