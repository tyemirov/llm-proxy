package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errHostedAuthorityDenied = errors.New(llmproxycontract.ErrorCodeHostedAuthorityDenied)

type journalAdmissionInput struct {
	OwnerUserID     string
	TenantID        string
	Provider        string
	Model           string
	Operation       string
	CatalogRevision string
	IdempotencyKey  string
	CanonicalIntent []byte
	ExecutionKind   journalExecutionKind
	ExecutionID     string
	OwnerToken      string
	Now             time.Time
	ClaimExpiresAt  time.Time
}

type journalAdmissionIntent struct {
	owner  string
	record managedJournalRequestRecord
}

func newJournalAdmissionIntent(input journalAdmissionInput, entropy io.Reader) (journalAdmissionIntent, error) {
	for _, identifier := range []string{input.OwnerUserID, input.TenantID, input.Provider, input.Operation, input.CatalogRevision, input.ExecutionID, input.OwnerToken} {
		if identifier == "" || identifier != strings.TrimSpace(identifier) {
			return journalAdmissionIntent{}, errUsageJournalInvalid
		}
	}
	if input.Model != strings.TrimSpace(input.Model) || (input.Model == "" && input.ExecutionKind != journalExecutionMedia) {
		return journalAdmissionIntent{}, errUsageJournalInvalid
	}
	if !validIdempotencyKey(input.IdempotencyKey) || input.Now.IsZero() || !input.ClaimExpiresAt.After(input.Now) {
		return journalAdmissionIntent{}, errUsageJournalInvalid
	}
	switch input.ExecutionKind {
	case journalExecutionText, journalExecutionMedia, journalExecutionDictation:
	default:
		return journalAdmissionIntent{}, errUsageJournalInvalid
	}
	canonical, err := canonicalJSON(input.CanonicalIntent)
	if err != nil {
		return journalAdmissionIntent{}, fmt.Errorf("%w: normalize request intent: %w", errUsageJournalInvalid, err)
	}
	identifier, err := newHostedResourceID(journalRequestIDPrefix, entropy)
	if err != nil {
		return journalAdmissionIntent{}, err
	}
	// Only caller intent participates in replay identity. A new worker, execution ID,
	// or catalog revision must not change a previously accepted request.
	identity, err := json.Marshal([]string{string(input.ExecutionKind), input.Provider, input.Model, input.Operation, string(canonical)})
	if err != nil {
		return journalAdmissionIntent{}, fmt.Errorf("encode request identity: %w", err)
	}
	return journalAdmissionIntent{owner: input.OwnerUserID, record: managedJournalRequestRecord{
		ID: identifier, TenantID: input.TenantID, KeyDigest: sha256Hex(input.IdempotencyKey), IntentDigest: sha256Hex(string(identity)),
		Provider: input.Provider, Model: input.Model, Operation: input.Operation, CatalogRevision: input.CatalogRevision,
		ExecutionKind: input.ExecutionKind, ExecutionID: input.ExecutionID, State: journalRequestAccepted, UsageState: journalUsagePending,
		OwnerToken: input.OwnerToken, ClaimExpiresAt: input.ClaimExpiresAt.UTC(), CreatedAt: input.Now.UTC(), UpdatedAt: input.Now.UTC(),
	}}, nil
}

// The funds owner supplies its reservation operation on this same transaction.
// No external call or second database can provide atomic request admission.
type journalReservation func(*gorm.DB, managedJournalRequestRecord) error

func (database *gormManagedTenantDatabase) admitJournalRequest(ctx context.Context, intent journalAdmissionIntent, reserve journalReservation) (managedJournalRequestRecord, error) {
	var accepted managedJournalRequestRecord
	err := database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var err error
		accepted, err = admitJournalRequest(transaction, intent, reserve)
		return err
	})
	if err != nil {
		return managedJournalRequestRecord{}, err
	}
	return accepted, nil
}

// The execution owner can include its operation record and asset references in
// this same transaction. The caller owns commit or rollback.
func admitJournalRequest(transaction *gorm.DB, intent journalAdmissionIntent, reserve journalReservation) (managedJournalRequestRecord, error) {
	var accepted managedJournalRequestRecord
	if _, err := lockProviderAssignmentTenant(transaction, intent.owner, intent.record.TenantID); err != nil {
		return managedJournalRequestRecord{}, err
	}
	err := transaction.Where("tenant_id = ? AND key_digest = ?", intent.record.TenantID, intent.record.KeyDigest).First(&accepted).Error
	if err == nil {
		if accepted.IntentDigest != intent.record.IntentDigest {
			return managedJournalRequestRecord{}, errUsageJournalConflict
		}
		if err := recoverJournalAdmission(transaction, &accepted, intent.record); err != nil {
			return managedJournalRequestRecord{}, err
		}
		return accepted, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return managedJournalRequestRecord{}, fmt.Errorf("read request admission for tenant %s: %w", intent.record.TenantID, err)
	}
	accepted = intent.record
	if err := bindJournalAuthority(transaction, intent.owner, &accepted); err != nil {
		return managedJournalRequestRecord{}, err
	}
	if err := transaction.Omit(clause.Associations).Create(&accepted).Error; err != nil {
		return managedJournalRequestRecord{}, fmt.Errorf("create journal request %s: %w", accepted.ID, err)
	}
	if err := reserve(transaction, accepted); err != nil {
		return managedJournalRequestRecord{}, fmt.Errorf("reserve funds for request %s: %w", accepted.ID, err)
	}
	return accepted, nil
}

func bindJournalAuthority(transaction *gorm.DB, owner string, request *managedJournalRequestRecord) error {
	var grant managedHostedGrantRecord
	err := transaction.Where("tenant_id = ? AND provider = ? AND state = ? AND catalog_revision = ?", request.TenantID, request.Provider, hostedGrantActive, request.CatalogRevision).
		Where("id IN (?)", transaction.Model(&managedHostedTenantAssignmentRecord{}).Select("grant_id").Where("tenant_id = ? AND provider_id = ?", request.TenantID, request.Provider)).
		Where("billing_account_id IN (?)", transaction.Model(&managedBillingAccountRecord{}).Select("id").Where("owner_user_id = ?", owner)).First(&grant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errHostedAuthorityDenied
	}
	if err != nil {
		return fmt.Errorf("read request authority for tenant %s: %w", request.TenantID, err)
	}
	var offerings []hostedGrantOffering
	if err := json.Unmarshal(grant.Offerings, &offerings); err != nil {
		return fmt.Errorf("decode offerings for grant %s: %w", grant.ID, err)
	}
	permitted := slices.ContainsFunc(offerings, func(offering hostedGrantOffering) bool {
		return offering.Model == request.Model && slices.Contains(offering.Operations, request.Operation)
	})
	if !permitted {
		return errHostedAuthorityDenied
	}
	var connection managedPlatformConnectionRecord
	if err := transaction.Where("id = ? AND provider = ?", grant.PlatformConnectionID, request.Provider).First(&connection).Error; err != nil {
		return fmt.Errorf("read platform connection for grant %s: %w", grant.ID, err)
	}
	var credential managedPlatformCredentialRecord
	if err := transaction.Where("connection_id = ? AND version = ?", connection.ID, connection.Version).First(&credential).Error; err != nil {
		return fmt.Errorf("read credential version for grant %s: %w", grant.ID, err)
	}
	if credential.QualifiedAt.IsZero() || credential.QualifiedAt.After(request.CreatedAt) {
		return errHostedAuthorityDenied
	}
	request.BillingAccountID, request.GrantID, request.GrantRevision = grant.BillingAccountID, grant.ID, grant.Revision
	request.PlatformConnectionID, request.CredentialVersion = connection.ID, credential.Version
	return nil
}
