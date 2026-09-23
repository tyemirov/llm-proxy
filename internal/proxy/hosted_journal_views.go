package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	managementJournalAttemptsPath     = managementJournalRequestPath + "/attempts"
	managementJournalObservationsPath = managementJournalRequestPath + "/observations"
	managementJournalCasesPath        = managementJournalRequestPath + "/reconciliation-cases"
)

type managementJournalAttemptResponse struct {
	ID           string              `json:"id"`
	Number       uint64              `json:"number"`
	State        journalAttemptState `json:"state"`
	DispatchedAt string              `json:"dispatched_at,omitempty"`
	ObservedAt   string              `json:"observed_at,omitempty"`
	CreatedAt    string              `json:"created_at"`
	UpdatedAt    string              `json:"updated_at"`
}
type managementJournalObservationResponse struct {
	ID           string                    `json:"id"`
	AttemptID    string                    `json:"attempt_id"`
	Quantities   []journalQuantity         `json:"quantities"`
	Completeness journalUsageState         `json:"completeness"`
	Outcome      journalObservationOutcome `json:"outcome"`
	FailureCode  string                    `json:"failure_code,omitempty"`
	ObservedAt   string                    `json:"observed_at"`
	CreatedAt    string                    `json:"created_at"`
}
type managementJournalCaseResponse struct {
	ID         string `json:"id"`
	Reason     string `json:"reason"`
	State      string `json:"state"`
	ResolvedAt string `json:"resolved_at,omitempty"`
	CreatedAt  string `json:"created_at"`
}

func (database *gormManagedTenantDatabase) journalAttempts(ctx context.Context, requestID string, page managedConnectionPage) ([]managedJournalAttemptRecord, error) {
	records := []managedJournalAttemptRecord{}
	if err := database.database.WithContext(ctx).Where("request_id = ? AND id > ?", requestID, page.after).Order("id").Limit(page.limit + 1).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("read attempts for request %s: %w", requestID, err)
	}
	return records, nil
}
func (database *gormManagedTenantDatabase) journalObservations(ctx context.Context, requestID string, page managedConnectionPage) ([]managedJournalObservationRecord, error) {
	records := []managedJournalObservationRecord{}
	attempts := database.database.Model(&managedJournalAttemptRecord{}).Select("id").Where("request_id = ?", requestID)
	if err := database.database.WithContext(ctx).Where("attempt_id IN (?) AND id > ?", attempts, page.after).Order("id").Limit(page.limit + 1).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("read observations for request %s: %w", requestID, err)
	}
	return records, nil
}
func (database *gormManagedTenantDatabase) journalCases(ctx context.Context, requestID string, page managedConnectionPage) ([]managedJournalCaseRecord, error) {
	records := []managedJournalCaseRecord{}
	if err := database.database.WithContext(ctx).Where("request_id = ? AND id > ?", requestID, page.after).Order("id").Limit(page.limit + 1).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("read reconciliation cases for request %s: %w", requestID, err)
	}
	return records, nil
}

func journalOptionalTimestamp(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

// Every child collection inherits account and request ownership before reading
// its bounded page. Only an explicit public projection can leave this boundary.
func journalCollectionHandler[T, R any](service *managementService, key, prefix string, read func(context.Context, string, managedConnectionPage) ([]T, error), project func(T) (R, string, error)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		request, err := service.store.database.journalRequest(ctx.Request.Context(), account.ID, ctx.Param("request_id"))
		if err != nil {
			writeUsageJournalError(ctx, err)
			return
		}
		page, err := newHostedResourcePage(ctx.Request.URL.RawQuery, prefix)
		if err != nil {
			writeUsageJournalError(ctx, errUsageJournalInvalid)
			return
		}
		records, err := read(ctx.Request.Context(), request.ID, page)
		if err != nil {
			writeUsageJournalError(ctx, err)
			return
		}
		more := len(records) > page.limit
		if more {
			records = records[:page.limit]
		}
		public := make([]R, 0, len(records))
		cursor := ""
		for _, record := range records {
			value, id, err := project(record)
			if err != nil {
				writeUsageJournalError(ctx, fmt.Errorf("%w: %w", errUsageJournalUnavailable, err))
				return
			}
			public = append(public, value)
			if more {
				cursor = id
			}
		}
		ctx.JSON(http.StatusOK, gin.H{key: public, "next_cursor": cursor})
	}
}

func (service *managementService) listJournalAttemptsHandler() gin.HandlerFunc {
	return journalCollectionHandler(service, "attempts", journalAttemptIDPrefix, service.store.database.journalAttempts, func(record managedJournalAttemptRecord) (managementJournalAttemptResponse, string, error) {
		return managementJournalAttemptResponse{ID: record.ID, Number: record.Number, State: record.State, DispatchedAt: journalOptionalTimestamp(record.DispatchAt), ObservedAt: journalOptionalTimestamp(record.ObservedAt), CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: record.UpdatedAt.UTC().Format(time.RFC3339Nano)}, record.ID, nil
	})
}
func (service *managementService) listJournalObservationsHandler() gin.HandlerFunc {
	return journalCollectionHandler(service, "observations", journalObservationIDPrefix, service.store.database.journalObservations, func(record managedJournalObservationRecord) (managementJournalObservationResponse, string, error) {
		var quantities []journalQuantity
		if err := json.Unmarshal(record.Quantities, &quantities); err != nil {
			return managementJournalObservationResponse{}, "", fmt.Errorf("read quantities for observation %s: %w", record.ID, err)
		}
		completeness, err := normalizeJournalQuantities(quantities)
		if err != nil {
			return managementJournalObservationResponse{}, "", fmt.Errorf("read retained quantities for observation %s: %w", record.ID, err)
		}
		if len(quantities) == 0 || completeness != record.Completeness {
			return managementJournalObservationResponse{}, "", fmt.Errorf("invalid retained quantities for observation %s", record.ID)
		}
		return managementJournalObservationResponse{ID: record.ID, AttemptID: record.AttemptID, Quantities: quantities, Completeness: record.Completeness, Outcome: record.Outcome, FailureCode: record.FailureCode, ObservedAt: record.ObservedAt.UTC().Format(time.RFC3339Nano), CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano)}, record.ID, nil
	})
}
func (service *managementService) listJournalCasesHandler() gin.HandlerFunc {
	return journalCollectionHandler(service, "cases", journalCaseIDPrefix, service.store.database.journalCases, func(record managedJournalCaseRecord) (managementJournalCaseResponse, string, error) {
		switch record.Reason {
		case journalCaseUsageUnknown, journalCaseDispatchUnknown, journalCaseExecutionUnknown, journalCaseResultUnknown:
		default:
			return managementJournalCaseResponse{}, "", fmt.Errorf("invalid retained reason for journal case %s", record.ID)
		}
		state := "open"
		if record.ResolvedAt != nil {
			state = "resolved"
		}
		return managementJournalCaseResponse{ID: record.ID, Reason: record.Reason, State: state, ResolvedAt: journalOptionalTimestamp(record.ResolvedAt), CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano)}, record.ID, nil
	})
}
