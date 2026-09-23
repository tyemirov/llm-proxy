package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

const (
	managementJournalRequestsPath = managementBillingAccountPath + "/requests"
	managementJournalRequestPath  = managementJournalRequestsPath + "/:request_id"
	journalRequestIDPrefix        = "request-"
	journalAttemptIDPrefix        = "attempt-"
	journalObservationIDPrefix    = "observation-"
	journalCaseIDPrefix           = "case-"
)

var (
	errUsageJournalInvalid     = errors.New("usage_journal_invalid")
	errUsageJournalNotFound    = errors.New("usage_journal_not_found")
	errUsageJournalConflict    = errors.New(llmproxycontract.ErrorCodeUsageJournalConflict)
	errUsageJournalUnavailable = errors.New(llmproxycontract.ErrorCodeUsageJournalUnavailable)
)

type journalRequestState string
type journalAttemptState string
type journalUsageState string
type journalExecutionKind string

const (
	journalCaseUsageUnknown     = "usage_unknown"
	journalCaseDispatchUnknown  = "dispatch_outcome_unknown"
	journalCaseExecutionUnknown = "execution_outcome_unknown"
	journalCaseResultUnknown    = "execution_result_unknown"
)

const (
	journalRequestAccepted    journalRequestState  = "accepted"
	journalRequestExecuting   journalRequestState  = "executing"
	journalRequestCompleted   journalRequestState  = "completed"
	journalRequestFailed      journalRequestState  = "failed"
	journalRequestUncertain   journalRequestState  = "uncertain"
	journalAttemptPrepared    journalAttemptState  = "prepared"
	journalAttemptDispatched  journalAttemptState  = "dispatched"
	journalAttemptObserved    journalAttemptState  = "observed"
	journalAttemptUncertain   journalAttemptState  = "uncertain"
	journalUsagePending       journalUsageState    = "pending"
	journalUsageComplete      journalUsageState    = "complete"
	journalUsageUnknown       journalUsageState    = "unknown"
	journalExecutionText      journalExecutionKind = "text_request"
	journalExecutionMedia     journalExecutionKind = "media_operation"
	journalExecutionDictation journalExecutionKind = "dictation_request"
)

// Financial records reference execution identities; they do not own execution payloads.
type managedJournalRequestRecord struct {
	ID                   string                           `gorm:"primaryKey"`
	BillingAccountID     string                           `gorm:"not null;index"`
	BillingAccount       managedBillingAccountRecord      `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	TenantID             string                           `gorm:"not null;uniqueIndex:idx_journal_tenant_intent"`
	Tenant               managedTenantRecord              `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnDelete:RESTRICT"`
	KeyDigest            string                           `gorm:"not null;uniqueIndex:idx_journal_tenant_intent"`
	IntentDigest         string                           `gorm:"not null"`
	GrantID              string                           `gorm:"not null"`
	GrantRevision        uint64                           `gorm:"not null"`
	Grant                managedHostedGrantRevisionRecord `gorm:"foreignKey:GrantID,GrantRevision;references:GrantID,Revision;constraint:OnDelete:RESTRICT"`
	PlatformConnectionID string                           `gorm:"not null"`
	CredentialVersion    uint64                           `gorm:"not null"`
	Credential           managedPlatformCredentialRecord  `gorm:"foreignKey:PlatformConnectionID,CredentialVersion;references:ConnectionID,Version;constraint:OnDelete:RESTRICT"`
	Provider             string                           `gorm:"not null"`
	Model                string                           `gorm:"not null"`
	Operation            string                           `gorm:"not null"`
	CatalogRevision      string                           `gorm:"not null"`
	ExecutionKind        journalExecutionKind             `gorm:"not null;uniqueIndex:idx_journal_execution;check:execution_kind IN ('text_request','media_operation','dictation_request')"`
	ExecutionID          string                           `gorm:"not null;uniqueIndex:idx_journal_execution"`
	State                journalRequestState              `gorm:"not null;check:state IN ('accepted','executing','completed','failed','uncertain')"`
	UsageState           journalUsageState                `gorm:"not null;check:usage_state IN ('pending','complete','unknown')"`
	FailureCode          string
	OwnerToken           string    `gorm:"not null"`
	ClaimExpiresAt       time.Time `gorm:"not null"`
	ResultPublishedAt    *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type managedJournalAttemptRecord struct {
	ID                string                      `gorm:"primaryKey"`
	RequestID         string                      `gorm:"not null;uniqueIndex:idx_journal_attempt_number"`
	Request           managedJournalRequestRecord `gorm:"foreignKey:RequestID;references:ID;constraint:OnDelete:RESTRICT"`
	Number            uint64                      `gorm:"not null;uniqueIndex:idx_journal_attempt_number;check:number > 0"`
	State             journalAttemptState         `gorm:"not null;check:state IN ('prepared','dispatched','observed','uncertain')"`
	ProviderRequestID string
	DispatchAt        *time.Time
	ObservedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type managedJournalObservationRecord struct {
	ID              string                      `gorm:"primaryKey"`
	AttemptID       string                      `gorm:"not null;uniqueIndex:idx_journal_observation_evidence"`
	Attempt         managedJournalAttemptRecord `gorm:"foreignKey:AttemptID;references:ID;constraint:OnDelete:RESTRICT"`
	EvidenceDigest  string                      `gorm:"not null;uniqueIndex:idx_journal_observation_evidence"`
	AdapterRevision string                      `gorm:"not null"`
	Quantities      []byte                      `gorm:"not null"`
	SourceFields    []byte                      `gorm:"not null"`
	Completeness    journalUsageState           `gorm:"not null;check:completeness IN ('complete','unknown')"`
	Outcome         journalObservationOutcome   `gorm:"not null;check:outcome IN ('continue','complete','fail')"`
	FailureCode     string
	ObservedAt      time.Time `gorm:"not null"`
	CreatedAt       time.Time
}

type managedJournalDeliveryRecord struct {
	ObservationID string                          `gorm:"primaryKey"`
	Observation   managedJournalObservationRecord `gorm:"foreignKey:ObservationID;references:ID;constraint:OnDelete:RESTRICT"`
	DeliveredAt   *time.Time
	CreatedAt     time.Time
}

type managedJournalCaseRecord struct {
	ID         string                      `gorm:"primaryKey"`
	RequestID  string                      `gorm:"not null;uniqueIndex:idx_journal_case_reason"`
	Request    managedJournalRequestRecord `gorm:"foreignKey:RequestID;references:ID;constraint:OnDelete:RESTRICT"`
	Reason     string                      `gorm:"not null;uniqueIndex:idx_journal_case_reason"`
	ResolvedAt *time.Time
	Resolution string
	CreatedAt  time.Time
}

func initializeHostedJournalSchema(database *gorm.DB) error {
	models := []any{&managedJournalRequestRecord{}, &managedJournalAttemptRecord{}, &managedJournalObservationRecord{}, &managedJournalDeliveryRecord{}, &managedJournalCaseRecord{}}
	present := 0
	for _, model := range models {
		if database.Migrator().HasTable(model) {
			present++
		}
	}
	if present == 0 {
		if err := database.AutoMigrate(models...); err != nil {
			return fmt.Errorf("%w: operation=create_usage_journal: %w", errManagedTenantSchemaMigration, err)
		}
		return nil
	}
	if present != len(models) {
		return fmt.Errorf("%w: operation=validate_usage_journal reason=partial_schema", errManagedTenantSchemaMigration)
	}
	for _, model := range models {
		if err := validateHostedTable(database, model); err != nil {
			return fmt.Errorf("%w: operation=validate_usage_journal: %w", errManagedTenantSchemaMigration, err)
		}
	}
	return nil
}

type managementJournalRequestResponse struct {
	ID              string               `json:"id"`
	TenantID        string               `json:"tenant_id"`
	GrantID         string               `json:"grant_id"`
	GrantRevision   uint64               `json:"grant_revision"`
	Provider        string               `json:"provider"`
	Model           string               `json:"model,omitempty"`
	Operation       string               `json:"operation"`
	CatalogRevision string               `json:"catalog_revision"`
	ExecutionKind   journalExecutionKind `json:"execution_kind"`
	ExecutionID     string               `json:"execution_id"`
	State           journalRequestState  `json:"state"`
	UsageState      journalUsageState    `json:"usage_state"`
	FailureCode     string               `json:"failure_code,omitempty"`
	CreatedAt       string               `json:"created_at"`
	UpdatedAt       string               `json:"updated_at"`
}

type managementJournalRequestsResponse struct {
	Requests   []managementJournalRequestResponse `json:"requests"`
	NextCursor string                             `json:"next_cursor"`
}

func journalRequestResponse(record managedJournalRequestRecord) managementJournalRequestResponse {
	return managementJournalRequestResponse{ID: record.ID, TenantID: record.TenantID, GrantID: record.GrantID, GrantRevision: record.GrantRevision,
		Provider: record.Provider, Model: record.Model, Operation: record.Operation, CatalogRevision: record.CatalogRevision,
		ExecutionKind: record.ExecutionKind, ExecutionID: record.ExecutionID, State: record.State, UsageState: record.UsageState, FailureCode: record.FailureCode,
		CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: record.UpdatedAt.UTC().Format(time.RFC3339Nano)}
}

func (database *gormManagedTenantDatabase) journalRequests(ctx context.Context, accountID string, page managedConnectionPage) ([]managedJournalRequestRecord, error) {
	records := []managedJournalRequestRecord{}
	if err := database.database.WithContext(ctx).Where("billing_account_id = ? AND id > ?", accountID, page.after).Order("id").Limit(page.limit + 1).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("read journal for account %s: %w", accountID, err)
	}
	return records, nil
}

func (database *gormManagedTenantDatabase) journalRequest(ctx context.Context, accountID, requestID string) (managedJournalRequestRecord, error) {
	var record managedJournalRequestRecord
	err := database.database.WithContext(ctx).Where("billing_account_id = ? AND id = ?", accountID, requestID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, errUsageJournalNotFound
	}
	if err != nil {
		return record, fmt.Errorf("read journal request %s: %w", requestID, err)
	}
	return record, nil
}

func writeUsageJournalError(ctx *gin.Context, err error) {
	status, code := http.StatusInternalServerError, llmproxycontract.ErrorCodeUsageJournalUnavailable
	switch {
	case errors.Is(err, errUsageJournalUnavailable):
		status, code = http.StatusInternalServerError, llmproxycontract.ErrorCodeUsageJournalUnavailable
	case errors.Is(err, errUsageJournalInvalid):
		status, code = http.StatusBadRequest, errUsageJournalInvalid.Error()
	case errors.Is(err, errUsageJournalNotFound):
		status, code = http.StatusNotFound, errUsageJournalNotFound.Error()
	case errors.Is(err, errUsageJournalConflict):
		status, code = http.StatusConflict, errUsageJournalConflict.Error()
	}
	ctx.JSON(status, gin.H{"error": gin.H{"code": code}})
}

func (service *managementService) ownedBillingAccount(ctx *gin.Context) (managedBillingAccountRecord, bool) {
	record, err := service.store.database.billingAccount(ctx.Request.Context(), managementPrincipalFromContext(ctx).userID)
	if err == nil && record.ID != ctx.Param("billing_account_id") {
		err = errBillingAccountNotFound
	}
	if err != nil {
		writeBillingAccountError(ctx, err)
		return record, false
	}
	return record, true
}

func (service *managementService) listJournalRequestsHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		page, err := newHostedResourcePage(ctx.Request.URL.RawQuery, journalRequestIDPrefix)
		if err != nil {
			writeUsageJournalError(ctx, errUsageJournalInvalid)
			return
		}
		records, err := service.store.database.journalRequests(ctx.Request.Context(), account.ID, page)
		if err != nil {
			writeUsageJournalError(ctx, err)
			return
		}
		response := managementJournalRequestsResponse{Requests: []managementJournalRequestResponse{}}
		if len(records) > page.limit {
			records = records[:page.limit]
			response.NextCursor = records[len(records)-1].ID
		}
		for _, record := range records {
			response.Requests = append(response.Requests, journalRequestResponse(record))
		}
		ctx.JSON(http.StatusOK, response)
	}
}

func (service *managementService) getJournalRequestHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		record, err := service.store.database.journalRequest(ctx.Request.Context(), account.ID, ctx.Param("request_id"))
		if err != nil {
			writeUsageJournalError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, journalRequestResponse(record))
	}
}
