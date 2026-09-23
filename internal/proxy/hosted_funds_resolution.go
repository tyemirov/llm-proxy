package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const managementFundsResolutionPath = managementJournalRequestPath + "/funds-resolution"

type managedFundsResolutionRecord struct {
	RequestID           string                      `gorm:"primaryKey"`
	Request             managedJournalRequestRecord `gorm:"belongsTo:Request;foreignKey:RequestID;references:ID;constraint:OnDelete:RESTRICT"`
	BillingAccountID    string                      `gorm:"not null;index"`
	ActorUserID         string                      `gorm:"not null"`
	ReservationRevision uint64                      `gorm:"not null"`
	ChargeNumerator     string                      `gorm:"not null"`
	ChargeDenominator   string                      `gorm:"not null"`
	Reason              string                      `gorm:"not null"`
	EvidenceReference   string                      `gorm:"not null"`
	CreatedAt           time.Time                   `gorm:"not null"`
}

type fundsResolutionCommand struct{ record managedFundsResolutionRecord }
type managementFundsResolutionResponse struct {
	RequestID      string     `json:"request_id"`
	Currency       string     `json:"currency"`
	CustomerCharge ExactMoney `json:"customer_charge"`
	SettledCents   string     `json:"settled_cents"`
	Reason         string     `json:"reason"`
	CreatedAt      string     `json:"created_at"`
}

func decodeFundsResolution(ctx *gin.Context, now time.Time) (fundsResolutionCommand, error) {
	var input struct {
		Revision          uint64     `json:"revision"`
		CustomerCharge    ExactMoney `json:"customer_charge"`
		Reason            string     `json:"reason"`
		EvidenceReference string     `json:"evidence_reference"`
	}
	if err := decodeManagementJSON(ctx, &input); err != nil {
		return fundsResolutionCommand{}, errBillingAccountInvalid
	}
	amount, err := parseExactMoney(input.CustomerCharge)
	if err != nil || input.Revision == 0 || !journalDimensionPattern.MatchString(input.Reason) || len(input.EvidenceReference) > 256 || strings.TrimSpace(input.EvidenceReference) == "" || strings.TrimSpace(input.EvidenceReference) != input.EvidenceReference || strings.ContainsAny(input.EvidenceReference, "\r\n\x00") {
		return fundsResolutionCommand{}, errBillingAccountInvalid
	}
	exact := ratingMoney(amount)
	return fundsResolutionCommand{record: managedFundsResolutionRecord{RequestID: ctx.Param("request_id"), BillingAccountID: ctx.Param("billing_account_id"), ActorUserID: managementPrincipalFromContext(ctx).userID, ReservationRevision: input.Revision, ChargeNumerator: exact.Numerator, ChargeDenominator: exact.Denominator, Reason: input.Reason, EvidenceReference: input.EvidenceReference, CreatedAt: now}}, nil
}

func readFundsResolution(tx *gorm.DB, accountID, requestID string) (managementFundsResolutionResponse, error) {
	var record managedFundsResolutionRecord
	if err := tx.Where("request_id = ? AND billing_account_id = ?", requestID, accountID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return managementFundsResolutionResponse{}, errBillingAccountNotFound
		}
		return managementFundsResolutionResponse{}, err
	}
	var settlement managedFundsSettlementRecord
	if err := tx.Where("request_id = ? AND billing_account_id = ?", requestID, accountID).First(&settlement).Error; err != nil {
		return managementFundsResolutionResponse{}, err
	}
	return managementFundsResolutionResponse{RequestID: record.RequestID, Currency: CatalogCurrencyUSD, CustomerCharge: ExactMoney{Numerator: record.ChargeNumerator, Denominator: record.ChargeDenominator}, SettledCents: strconv.FormatInt(settlement.SettledCents, 10), Reason: record.Reason, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano)}, nil
}

func (database *gormManagedTenantDatabase) fundsResolution(ctx context.Context, accountID, requestID string, command *fundsResolutionCommand) (managementFundsResolutionResponse, error) {
	var response managementFundsResolutionResponse
	err := database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if command != nil {
			if err := applyFundsResolution(tx, command.record); err != nil {
				return err
			}
		}
		var err error
		response, err = readFundsResolution(tx, accountID, requestID)
		return err
	})
	if err != nil {
		return response, fmt.Errorf("financial resolution for request %s: %w", requestID, err)
	}
	return response, nil
}

func applyFundsResolution(tx *gorm.DB, decision managedFundsResolutionRecord) error {
	lock := tx.Model(&managedJournalRequestRecord{}).Where("id = ? AND billing_account_id = ?", decision.RequestID, decision.BillingAccountID).UpdateColumn("state", gorm.Expr("state"))
	if lock.Error != nil {
		return lock.Error
	}
	if lock.RowsAffected != 1 {
		return errBillingAccountNotFound
	}
	if err := tx.Model(&managedBillingAccountRecord{}).Where("id = ?", decision.BillingAccountID).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
		return err
	}
	var existing managedFundsResolutionRecord
	err := tx.Where("request_id = ?", decision.RequestID).First(&existing).Error
	if err == nil {
		if existing.ReservationRevision != decision.ReservationRevision || existing.ChargeNumerator != decision.ChargeNumerator || existing.ChargeDenominator != decision.ChargeDenominator || existing.Reason != decision.Reason || existing.EvidenceReference != decision.EvidenceReference {
			return errBillingAccountConflict
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var request managedJournalRequestRecord
	if err := tx.Where("id = ?", decision.RequestID).First(&request).Error; err != nil {
		return err
	}
	if request.State != journalRequestCompleted && request.State != journalRequestFailed && request.State != journalRequestUncertain {
		return errBillingAccountConflict
	}
	var reservation managedFundsReservationRecord
	if err := tx.Where("request_id = ? AND billing_account_id = ?", decision.RequestID, decision.BillingAccountID).First(&reservation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errBillingAccountNotFound
		}
		return err
	}
	if reservation.State != fundsReservationReconciliation || reservation.Revision != decision.ReservationRevision {
		return errBillingAccountConflict
	}
	var retained managedPriceSnapshotRecord
	if err := tx.Where("request_id = ?", decision.RequestID).First(&retained).Error; err != nil {
		return err
	}
	_, document, err := restoreHostedPriceSnapshot(retained)
	if err != nil {
		return err
	}
	exposure, err := readHostedFundsExposure(tx, decision.BillingAccountID, decision.RequestID, document.Maximum.CustomerCharge)
	if err != nil {
		return err
	}
	maximum, err := parseExactMoney(exposure.ResolutionChargeLimit)
	if err != nil {
		return err
	}
	amount, err := parseExactMoney(ExactMoney{Numerator: decision.ChargeNumerator, Denominator: decision.ChargeDenominator})
	if err != nil {
		return err
	}
	if amount.Cmp(maximum) > 0 {
		return errBillingAccountConflict
	}
	// The decision is the final net amount. Retain prior credits as included so
	// later credits cannot apply the same financial reduction twice.
	var creditIDs []string
	charges := tx.Model(&managedChargeRecord{}).Select("id").Where("request_id = ?", decision.RequestID)
	if err := tx.Model(&managedChargeAdjustmentRecord{}).Where("charge_id IN (?)", charges).Order("id").Pluck("id", &creditIDs).Error; err != nil {
		return err
	}
	if err := commitHostedFundsSettlement(tx, reservation, amount, creditIDs, decision.CreatedAt); err != nil {
		return err
	}
	if err := tx.Omit(clause.Associations).Create(&decision).Error; err != nil {
		return fmt.Errorf("retain financial decision: %w", err)
	}
	return nil
}

func (service *managementService) fundsResolutionHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var command *fundsResolutionCommand
		if ctx.Request.Method == http.MethodPut {
			if !requireHostedOperator(ctx) {
				return
			}
			value, err := decodeFundsResolution(ctx, service.store.now().UTC())
			if err != nil {
				writeBillingAccountError(ctx, err)
				return
			}
			command = &value
		} else if !managementPrincipalFromContext(ctx).isAdmin {
			if _, ok := service.ownedBillingAccount(ctx); !ok {
				return
			}
		}
		if ctx.Request.URL.RawQuery != "" {
			writeBillingAccountError(ctx, errBillingAccountInvalid)
			return
		}
		value, err := service.store.database.fundsResolution(ctx.Request.Context(), ctx.Param("billing_account_id"), ctx.Param("request_id"), command)
		if err != nil {
			writeBillingAccountError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, value)
	}
}
