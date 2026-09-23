package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	managementFundsReservationsPath = managementBillingAccountPath + "/reservations"
	managementFundsReservationPath  = managementFundsReservationsPath + "/:request_id"
	managementFundsEntriesPath      = managementBillingAccountPath + "/ledger-entries"
)

type managementFundsReservationResponse struct {
	ID           string `json:"id"`
	Currency     string `json:"currency"`
	MaximumCents string `json:"maximum_cents"`
	State        string `json:"state"`
	Revision     uint64 `json:"revision"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type managementFundsReservationDetailResponse struct {
	managementFundsReservationResponse
	hostedFundsExposure
	AuthorizedMaximum ExactMoney `json:"authorized_maximum"`
}

func fundsReservationResponse(record managedFundsReservationRecord) managementFundsReservationResponse {
	return managementFundsReservationResponse{ID: record.RequestID, Currency: record.Currency, MaximumCents: strconv.FormatInt(record.MaximumCents, 10), State: record.State, Revision: record.Revision, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: record.UpdatedAt.UTC().Format(time.RFC3339Nano)}
}

func (database *gormManagedTenantDatabase) billingFundsReservation(ctx context.Context, accountID, requestID string) (managementFundsReservationDetailResponse, error) {
	var response managementFundsReservationDetailResponse
	err := database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record managedFundsReservationRecord
		if err := tx.Where("request_id = ? AND billing_account_id = ?", requestID, accountID).First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errBillingAccountNotFound
			}
			return err
		}
		var retained managedPriceSnapshotRecord
		if err := tx.Where("request_id = ? AND billing_account_id = ?", requestID, accountID).First(&retained).Error; err != nil {
			return err
		}
		_, document, err := restoreHostedPriceSnapshot(retained)
		if err != nil {
			return err
		}
		exposure, err := readHostedFundsExposure(tx, accountID, requestID, document.Maximum.CustomerCharge)
		if err != nil {
			return err
		}
		response = managementFundsReservationDetailResponse{managementFundsReservationResponse: fundsReservationResponse(record), AuthorizedMaximum: document.Maximum.CustomerCharge, hostedFundsExposure: exposure}
		return nil
	})
	if err != nil {
		return response, fmt.Errorf("read funds reservation %s: %w", requestID, err)
	}
	return response, nil
}

func (service *managementService) getFundsReservationHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !managementPrincipalFromContext(ctx).isAdmin {
			if _, ok := service.ownedBillingAccount(ctx); !ok {
				return
			}
		}
		if ctx.Request.URL.RawQuery != "" {
			writeBillingAccountError(ctx, errBillingAccountInvalid)
			return
		}
		response, err := service.store.database.billingFundsReservation(ctx.Request.Context(), ctx.Param("billing_account_id"), ctx.Param("request_id"))
		if err != nil {
			writeBillingAccountError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}

type managementFundsEntryResponse struct {
	ID              string  `json:"id"`
	Currency        string  `json:"currency"`
	Type            string  `json:"type"`
	AmountCents     string  `json:"amount_cents"`
	ReservationID   *string `json:"reservation_id"`
	RefundOfEntryID *string `json:"refund_of_entry_id"`
	CreatedAt       string  `json:"created_at"`
}

func (database *gormManagedTenantDatabase) billingFundsReservations(ctx context.Context, accountID string, page managedConnectionPage) ([]managedFundsReservationRecord, error) {
	records := []managedFundsReservationRecord{}
	if err := database.database.WithContext(ctx).Where("billing_account_id = ? AND request_id > ?", accountID, page.after).Order("request_id").Limit(page.limit + 1).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("read account reservations: %w", err)
	}
	return records, nil
}

func (database *gormManagedTenantDatabase) billingFundsEntries(ctx context.Context, accountID string, page managedConnectionPage) ([]managementFundsEntryResponse, error) {
	// Project existing shared Ledger records. Never derive a second balance here.
	accounts := database.database.Model(&gormstore.LedgerAccount{}).Select("account_id").Where("tenant_id = ? AND user_id = ? AND ledger_id = ?", hostedLedgerTenant, accountID, CatalogCurrencyUSD)
	rows := []gormstore.LedgerEntry{}
	if err := database.database.WithContext(ctx).Where("account_id IN (?) AND entry_id > ?", accounts, page.after).Order("entry_id").Limit(page.limit + 1).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("read account Ledger entries: %w", err)
	}
	responses := make([]managementFundsEntryResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, managementFundsEntryResponse{ID: row.EntryID, Currency: CatalogCurrencyUSD, Type: row.Type, AmountCents: strconv.FormatInt(row.AmountCents, 10), ReservationID: row.ReservationID, RefundOfEntryID: row.RefundOfEntryID, CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339Nano)})
	}
	return responses, nil
}

func fundsCollectionHandler[T, R any](service *managementService, key string, parsePage func(string) (managedConnectionPage, error), read func(context.Context, string, managedConnectionPage) ([]T, error), project func(T) (R, string)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		page, err := parsePage(ctx.Request.URL.RawQuery)
		if err != nil {
			writeBillingAccountError(ctx, errBillingAccountInvalid)
			return
		}
		records, err := read(ctx.Request.Context(), account.ID, page)
		if err != nil {
			writeBillingAccountError(ctx, err)
			return
		}
		more := len(records) > page.limit
		if more {
			records = records[:page.limit]
		}
		responses := make([]R, 0, len(records))
		cursor := ""
		for _, record := range records {
			response, id := project(record)
			responses = append(responses, response)
			if more {
				cursor = id
			}
		}
		ctx.JSON(http.StatusOK, gin.H{key: responses, "next_cursor": cursor})
	}
}

func (service *managementService) listFundsReservationsHandler() gin.HandlerFunc {
	return fundsCollectionHandler(service, "reservations", func(query string) (managedConnectionPage, error) {
		return newHostedResourcePage(query, journalRequestIDPrefix)
	}, service.store.database.billingFundsReservations, func(record managedFundsReservationRecord) (managementFundsReservationResponse, string) {
		return fundsReservationResponse(record), record.RequestID
	})
}

func (service *managementService) listFundsEntriesHandler() gin.HandlerFunc {
	return fundsCollectionHandler(service, "entries", func(query string) (managedConnectionPage, error) {
		return newHostedPage(query, func(cursor string) bool {
			id, err := ledger.NewEntryID(cursor)
			return err == nil && id.String() == cursor && len(cursor) <= 128
		})
	}, service.store.database.billingFundsEntries, func(record managementFundsEntryResponse) (managementFundsEntryResponse, string) {
		return record, record.ID
	})
}
