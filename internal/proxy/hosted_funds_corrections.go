package proxy

import (
	"context"
	"encoding/json"
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

const managementFundsCreditPath = managementJournalRequestPath + "/funds-credits/:credit_id"

type managedFundsCorrectionRecord struct {
	ID                string                      `gorm:"primaryKey"`
	RequestID         string                      `gorm:"not null;index"`
	Request           managedJournalRequestRecord `gorm:"foreignKey:RequestID;references:ID;constraint:OnDelete:RESTRICT"`
	BillingAccountID  string                      `gorm:"not null;index"`
	BillingAccount    managedBillingAccountRecord `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	CreditID          string                      `gorm:"not null"`
	ActorUserID       string                      `gorm:"not null"`
	CreditNumerator   string                      `gorm:"not null"`
	CreditDenominator string                      `gorm:"not null"`
	Reason            string                      `gorm:"not null"`
	EvidenceReference string                      `gorm:"not null"`
	Effect            fundsCreditEffect           `gorm:"embedded"`
	CreatedAt         time.Time                   `gorm:"not null"`
}

type fundsCorrectionCommand struct{ record managedFundsCorrectionRecord }

type managementFundsCreditResponse struct {
	ID            string     `json:"id"`
	RequestID     string     `json:"request_id"`
	Currency      string     `json:"currency"`
	Credit        ExactMoney `json:"credit"`
	CreditedCents string     `json:"credited_cents"`
	Reason        string     `json:"reason"`
	CreatedAt     string     `json:"created_at"`
}

func fundsCorrectionID(accountID, requestID, creditID string) string {
	return "correction-" + sha256Hex(accountID + "\x00" + requestID + "\x00" + creditID)[:32]
}

func decodeFundsCorrection(ctx *gin.Context, now time.Time) (fundsCorrectionCommand, error) {
	var input struct {
		Credit            ExactMoney `json:"credit"`
		Reason            string     `json:"reason"`
		EvidenceReference string     `json:"evidence_reference"`
	}
	if err := decodeManagementJSON(ctx, &input); err != nil {
		return fundsCorrectionCommand{}, errBillingAccountInvalid
	}
	amount, err := parseExactMoney(input.Credit)
	if err != nil || amount.Sign() <= 0 || !journalDimensionPattern.MatchString(input.Reason) || len(input.EvidenceReference) > 256 || strings.TrimSpace(input.EvidenceReference) == "" || strings.TrimSpace(input.EvidenceReference) != input.EvidenceReference || strings.ContainsAny(input.EvidenceReference, "\r\n\x00") {
		return fundsCorrectionCommand{}, errBillingAccountInvalid
	}
	credit := ratingMoney(amount)
	accountID, requestID, creditID := ctx.Param("billing_account_id"), ctx.Param("request_id"), ctx.Param("credit_id")
	return fundsCorrectionCommand{record: managedFundsCorrectionRecord{ID: fundsCorrectionID(accountID, requestID, creditID), RequestID: requestID, BillingAccountID: accountID, CreditID: creditID, ActorUserID: managementPrincipalFromContext(ctx).userID, CreditNumerator: credit.Numerator, CreditDenominator: credit.Denominator, Reason: input.Reason, EvidenceReference: input.EvidenceReference, CreatedAt: now}}, nil
}

func (database *gormManagedTenantDatabase) fundsCorrection(ctx context.Context, accountID, requestID, creditID string, command *fundsCorrectionCommand) (managementFundsCreditResponse, error) {
	var record managedFundsCorrectionRecord
	err := database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if command != nil {
			if err := applyFundsCorrection(tx, command.record); err != nil {
				return err
			}
		}
		err := tx.Where("id = ?", fundsCorrectionID(accountID, requestID, creditID)).First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errBillingAccountNotFound
		}
		return err
	})
	if err != nil {
		return managementFundsCreditResponse{}, fmt.Errorf("financial credit for request %s: %w", requestID, err)
	}
	return managementFundsCreditResponse{ID: record.CreditID, RequestID: record.RequestID, Currency: CatalogCurrencyUSD, Credit: ExactMoney{Numerator: record.CreditNumerator, Denominator: record.CreditDenominator}, CreditedCents: strconv.FormatInt(record.Effect.CreditedCents, 10), Reason: record.Reason, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano)}, nil
}

func applyFundsCorrection(tx *gorm.DB, proposed managedFundsCorrectionRecord) error {
	lock := tx.Model(&managedBillingAccountRecord{}).Where("id = ?", proposed.BillingAccountID).UpdateColumn("id", gorm.Expr("id"))
	if lock.Error != nil {
		return lock.Error
	}
	if lock.RowsAffected != 1 {
		return errBillingAccountNotFound
	}
	var existing managedFundsCorrectionRecord
	err := tx.Where("id = ?", proposed.ID).First(&existing).Error
	if err == nil {
		if existing.CreditNumerator != proposed.CreditNumerator || existing.CreditDenominator != proposed.CreditDenominator || existing.Reason != proposed.Reason || existing.EvidenceReference != proposed.EvidenceReference {
			return errBillingAccountConflict
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var reservation managedFundsReservationRecord
	if err := tx.Where("request_id = ? AND billing_account_id = ?", proposed.RequestID, proposed.BillingAccountID).First(&reservation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errBillingAccountNotFound
		}
		return err
	}
	if reservation.State != fundsReservationSettled {
		return errBillingAccountConflict
	}
	credit := ExactMoney{Numerator: proposed.CreditNumerator, Denominator: proposed.CreditDenominator}
	if err := authorizeHostedFundsCredit(tx, proposed.BillingAccountID, proposed.RequestID, credit); err != nil {
		if errors.Is(err, errUsageJournalConflict) {
			return errBillingAccountConflict
		}
		return err
	}
	metadata, err := json.Marshal(struct {
		RequestID string `json:"request_id"`
		CreditID  string `json:"credit_id"`
	}{proposed.RequestID, proposed.CreditID})
	if err != nil {
		return err
	}
	effect, err := applyHostedFundsCredit(tx, proposed.BillingAccountID, proposed.RequestID, credit, func(cents int64) error {
		return postHostedFundsCredit(tx, proposed.BillingAccountID, proposed.ID, cents, proposed.CreatedAt, metadata)
	})
	if err != nil {
		return err
	}
	proposed.Effect = effect
	if err := tx.Omit(clause.Associations).Create(&proposed).Error; err != nil {
		return fmt.Errorf("retain audited request credit: %w", err)
	}
	return nil
}

func (service *managementService) fundsCorrectionHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var command *fundsCorrectionCommand
		if ctx.Request.Method == http.MethodPut {
			if !requireHostedOperator(ctx) {
				return
			}
			value, err := decodeFundsCorrection(ctx, service.store.now().UTC())
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
		if ctx.Request.URL.RawQuery != "" || !validIdempotencyKey(ctx.Param("credit_id")) {
			writeBillingAccountError(ctx, errBillingAccountInvalid)
			return
		}
		value, err := service.store.database.fundsCorrection(ctx.Request.Context(), ctx.Param("billing_account_id"), ctx.Param("request_id"), ctx.Param("credit_id"), command)
		if err != nil {
			writeBillingAccountError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, value)
	}
}
