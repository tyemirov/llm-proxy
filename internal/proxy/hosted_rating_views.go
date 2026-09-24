package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	managementChargesPath       = managementBillingAccountPath + "/charges"
	managementChargePath        = managementChargesPath + "/:charge_id"
	managementPriceSnapshotPath = managementBillingAccountPath + "/price-snapshots/:price_snapshot_id"
)

type managementChargeResponse struct {
	ID                  string                               `json:"id"`
	RequestID           string                               `json:"request_id"`
	AttemptID           string                               `json:"attempt_id"`
	ObservationID       string                               `json:"observation_id"`
	PriceSnapshotID     string                               `json:"price_snapshot_id"`
	State               string                               `json:"state"`
	Rating              managementRatingResponse             `json:"rating"`
	CustomerCharge      *ExactMoney                          `json:"customer_charge"`
	NetCustomerCharge   *ExactMoney                          `json:"net_customer_charge"`
	CustomerAdjustments []managementChargeAdjustmentResponse `json:"customer_adjustments"`
	CreatedAt           string                               `json:"created_at"`
}

type managementChargeAdjustmentResponse struct {
	ID        string     `json:"id"`
	Credit    ExactMoney `json:"credit"`
	Reason    string     `json:"reason"`
	CreatedAt string     `json:"created_at"`
}

type managementRatingResponse struct {
	State                string             `json:"state"`
	Lines                []CatalogRatedLine `json:"lines"`
	ProviderCost         *ExactMoney        `json:"provider_cost"`
	CustomerCharge       *ExactMoney        `json:"customer_charge"`
	MinimumAdjustment    *ExactMoney        `json:"minimum_adjustment"`
	UnresolvedDimensions []string           `json:"unresolved_dimensions"`
}

type managementPriceSnapshotResponse struct {
	ID        string                      `json:"id"`
	RequestID string                      `json:"request_id"`
	Snapshot  hostedPriceSnapshotDocument `json:"snapshot"`
	CreatedAt string                      `json:"created_at"`
}

func (database *gormManagedTenantDatabase) billingCharges(ctx context.Context, accountID string, page managedConnectionPage) ([]managedChargeRecord, error) {
	records := []managedChargeRecord{}
	if err := database.database.WithContext(ctx).Where("billing_account_id = ? AND id > ?", accountID, page.after).Order("id").Limit(page.limit + 1).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("read account charges: %w", err)
	}
	return records, nil
}

func (database *gormManagedTenantDatabase) billingCharge(ctx context.Context, accountID, id string) (managedChargeRecord, error) {
	var record managedChargeRecord
	err := database.database.WithContext(ctx).Where("billing_account_id = ? AND id = ?", accountID, id).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, errUsageJournalNotFound
	}
	if err != nil {
		return record, fmt.Errorf("read account charge: %w", err)
	}
	return record, nil
}

func (database *gormManagedTenantDatabase) billingPriceSnapshot(ctx context.Context, accountID, id string) (managedPriceSnapshotRecord, error) {
	var record managedPriceSnapshotRecord
	err := database.database.WithContext(ctx).Where("billing_account_id = ? AND id = ?", accountID, id).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, errUsageJournalNotFound
	}
	if err != nil {
		return record, fmt.Errorf("read account price snapshot: %w", err)
	}
	return record, nil
}

func chargeResponse(record managedChargeRecord, adjustments []managedChargeAdjustmentRecord) (managementChargeResponse, error) {
	rating, err := decodeRetainedCharge(record)
	if err != nil {
		return managementChargeResponse{}, err
	}
	response := managementChargeResponse{ID: record.ID, RequestID: record.RequestID, AttemptID: record.AttemptID, ObservationID: record.ObservationID, PriceSnapshotID: record.PriceSnapshotID, State: record.State, Rating: managementRatingResponse{State: rating.State, Lines: rating.Lines, UnresolvedDimensions: rating.UnresolvedDimensions}, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano)}
	if rating.State == CatalogRatingResolved {
		response.Rating.ProviderCost = &rating.ProviderCost
		response.Rating.CustomerCharge = &rating.CustomerCharge
		response.Rating.MinimumAdjustment = &rating.MinimumAdjustment
	}
	if record.State == chargeRated {
		response.CustomerCharge = &rating.CustomerCharge
		net, err := netCustomerCharge(rating.CustomerCharge, adjustments)
		if err != nil {
			return managementChargeResponse{}, err
		}
		amount := ratingMoney(net)
		response.NetCustomerCharge = &amount
	} else if len(adjustments) != 0 {
		return managementChargeResponse{}, fmt.Errorf("unresolved charge has customer credits")
	}
	response.CustomerAdjustments = make([]managementChargeAdjustmentResponse, 0, len(adjustments))
	for _, adjustment := range adjustments {
		response.CustomerAdjustments = append(response.CustomerAdjustments, managementChargeAdjustmentResponse{ID: adjustment.ID, Credit: ExactMoney{Numerator: adjustment.CreditNumerator, Denominator: adjustment.CreditDenominator}, Reason: adjustment.Reason, CreatedAt: adjustment.CreatedAt.UTC().Format(time.RFC3339Nano)})
	}
	return response, nil
}

func (service *managementService) listChargesHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		page, err := newHostedResourcePage(ctx.Request.URL.RawQuery, chargeIDPrefix)
		if err != nil {
			writeUsageJournalError(ctx, errUsageJournalInvalid)
			return
		}
		records, err := service.store.database.billingCharges(ctx.Request.Context(), account.ID, page)
		if err != nil {
			writeUsageJournalError(ctx, err)
			return
		}
		cursor := ""
		if len(records) > page.limit {
			records = records[:page.limit]
			cursor = records[len(records)-1].ID
		}
		charges, err := service.chargeResponses(ctx.Request.Context(), account.ID, records)
		if err != nil {
			writeUsageJournalError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"charges": charges, "next_cursor": cursor})
	}
}

func (service *managementService) getChargeHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		record, err := service.store.database.billingCharge(ctx.Request.Context(), account.ID, ctx.Param("charge_id"))
		if err != nil {
			writeUsageJournalError(ctx, err)
			return
		}
		responses, err := service.chargeResponses(ctx.Request.Context(), account.ID, []managedChargeRecord{record})
		if err != nil {
			writeUsageJournalError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, responses[0])
	}
}

func (service *managementService) getPriceSnapshotHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		record, err := service.store.database.billingPriceSnapshot(ctx.Request.Context(), account.ID, ctx.Param("price_snapshot_id"))
		if err != nil {
			writeUsageJournalError(ctx, err)
			return
		}
		_, document, err := restoreHostedPriceSnapshot(record)
		if err != nil {
			writeUsageJournalError(ctx, fmt.Errorf("%w: restore accepted price %s: %w", errUsageJournalUnavailable, record.ID, err))
			return
		}
		ctx.JSON(http.StatusOK, managementPriceSnapshotResponse{ID: record.ID, RequestID: record.RequestID, Snapshot: document, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano)})
	}
}
