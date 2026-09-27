package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const managementFundsTenantLimitPath = managementBillingAccountPath + "/tenant-limits/:tenant_id"

// Net usage is a settlement projection for tenant budgets, not an account balance.
type managedFundsTenantRecord struct {
	TenantID         string                      `gorm:"primaryKey"`
	BillingAccountID string                      `gorm:"not null;index"`
	BillingAccount   managedBillingAccountRecord `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	LimitCents       *int64                      `gorm:"check:limit_cents IS NULL OR limit_cents >= 0"`
	SpentNumerator   string                      `gorm:"not null"`
	SpentDenominator string                      `gorm:"not null"`
	Revision         uint64                      `gorm:"not null"`
	UpdatedAt        time.Time                   `gorm:"not null"`
}

type managementFundsTenantLimitResponse struct {
	TenantID       string     `json:"tenant_id"`
	Currency       string     `json:"currency"`
	LimitCents     *string    `json:"limit_cents"`
	RemainingCents *string    `json:"remaining_cents"`
	ReservedCents  string     `json:"reserved_cents"`
	Spent          ExactMoney `json:"spent"`
	Revision       uint64     `json:"revision"`
}

type tenantFundsLimitChange struct {
	limit    *int64
	revision uint64
}

func decodeTenantFundsLimit(ctx *gin.Context) (tenantFundsLimitChange, error) {
	var request struct {
		LimitCents json.RawMessage `json:"limit_cents"`
		Revision   *uint64         `json:"revision"`
	}
	if err := decodeManagementJSON(ctx, &request); err != nil || len(request.LimitCents) == 0 || request.Revision == nil {
		return tenantFundsLimitChange{}, errBillingAccountInvalid
	}
	var value *string
	if err := json.Unmarshal(request.LimitCents, &value); err != nil {
		return tenantFundsLimitChange{}, errBillingAccountInvalid
	}
	change := tenantFundsLimitChange{revision: *request.Revision}
	if value != nil {
		cents, err := strconv.ParseInt(*value, 10, 64)
		if err != nil || cents < 0 || strconv.FormatInt(cents, 10) != *value {
			return tenantFundsLimitChange{}, errBillingAccountInvalid
		}
		change.limit = &cents
	}
	return change, nil
}

func tenantFundsRecord(accountID, tenantID string, now time.Time) managedFundsTenantRecord {
	return managedFundsTenantRecord{TenantID: tenantID, BillingAccountID: accountID, SpentNumerator: "0", SpentDenominator: "1", UpdatedAt: now}
}

func tenantReservedCents(tx *gorm.DB, accountID, tenantID string) (int64, error) {
	requests := tx.Model(&managedJournalRequestRecord{}).Select("id").Where("billing_account_id = ? AND tenant_id = ?", accountID, tenantID)
	var held int64
	err := tx.Model(&managedFundsReservationRecord{}).Where("billing_account_id = ? AND request_id IN (?) AND state IN ?", accountID, requests, []string{fundsReservationHeld, fundsReservationReconciliation}).Select("coalesce(sum(maximum_cents), 0)").Scan(&held).Error
	return held, err
}

func tenantRemainingCents(limit int64, spent *big.Rat, held int64) int64 {
	scaled := new(big.Rat).Mul(spent, big.NewRat(usdCentsPerDollar, 1))
	used, remainder := new(big.Int), new(big.Int)
	used.QuoRem(scaled.Num(), scaled.Denom(), remainder)
	if remainder.Sign() != 0 {
		used.Add(used, big.NewInt(1))
	}
	remaining := new(big.Int).Sub(big.NewInt(limit), used)
	remaining.Sub(remaining, big.NewInt(held))
	if remaining.Sign() < 0 {
		return 0
	}
	return remaining.Int64()
}

func reserveHostedTenantFunds(tx *gorm.DB, request managedJournalRequestRecord, maximum int64) error {
	record := tenantFundsRecord(request.BillingAccountID, request.TenantID, request.CreatedAt)
	if err := tx.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
		return fmt.Errorf("initialize tenant funds %s: %w", request.TenantID, err)
	}
	if err := tx.Where("billing_account_id = ? AND tenant_id = ?", request.BillingAccountID, request.TenantID).First(&record).Error; err != nil {
		return fmt.Errorf("read tenant funds %s: %w", request.TenantID, err)
	}
	if record.LimitCents == nil || maximum == 0 {
		return nil
	}
	spent, err := parseExactMoney(ExactMoney{Numerator: record.SpentNumerator, Denominator: record.SpentDenominator})
	if err != nil {
		return err
	}
	held, err := tenantReservedCents(tx, request.BillingAccountID, request.TenantID)
	if err != nil {
		return err
	}
	if maximum > tenantRemainingCents(*record.LimitCents, spent, held) {
		return errInsufficientFunds
	}
	return nil
}

func adjustHostedTenantUsage(tx *gorm.DB, accountID, requestID string, delta *big.Rat) error {
	var request managedJournalRequestRecord
	if err := tx.Where("id = ? AND billing_account_id = ?", requestID, accountID).First(&request).Error; err != nil {
		return err
	}
	var record managedFundsTenantRecord
	if err := tx.Where("billing_account_id = ? AND tenant_id = ?", accountID, request.TenantID).First(&record).Error; err != nil {
		return fmt.Errorf("read tenant usage %s: %w", request.TenantID, err)
	}
	spent, err := parseExactMoney(ExactMoney{Numerator: record.SpentNumerator, Denominator: record.SpentDenominator})
	if err != nil {
		return err
	}
	spent.Add(spent, delta)
	if spent.Sign() < 0 {
		return fmt.Errorf("negative retained usage for tenant %s", request.TenantID)
	}
	exact := ratingMoney(spent)
	if err := tx.Model(&record).Updates(map[string]any{"spent_numerator": exact.Numerator, "spent_denominator": exact.Denominator}).Error; err != nil {
		return fmt.Errorf("retain tenant usage %s: %w", request.TenantID, err)
	}
	return nil
}

func tenantFundsLimitResponse(tx *gorm.DB, record managedFundsTenantRecord) (managementFundsTenantLimitResponse, error) {
	spent, err := parseExactMoney(ExactMoney{Numerator: record.SpentNumerator, Denominator: record.SpentDenominator})
	if err != nil {
		return managementFundsTenantLimitResponse{}, err
	}
	held, err := tenantReservedCents(tx, record.BillingAccountID, record.TenantID)
	if err != nil {
		return managementFundsTenantLimitResponse{}, err
	}
	response := managementFundsTenantLimitResponse{TenantID: record.TenantID, Currency: CatalogCurrencyUSD, ReservedCents: strconv.FormatInt(held, 10), Spent: ratingMoney(spent), Revision: record.Revision}
	if record.LimitCents != nil {
		limit, remaining := strconv.FormatInt(*record.LimitCents, 10), strconv.FormatInt(tenantRemainingCents(*record.LimitCents, spent, held), 10)
		response.LimitCents = &limit
		response.RemainingCents = &remaining
	}
	return response, nil
}

func (database *gormManagedTenantDatabase) tenantFundsLimit(ctx context.Context, account managedBillingAccountRecord, tenantID string, change *tenantFundsLimitChange, now time.Time) (managementFundsTenantLimitResponse, error) {
	var response managementFundsTenantLimitResponse
	err := database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if change != nil {
			if err := tx.Model(&managedBillingAccountRecord{}).Where("id = ?", account.ID).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
				return err
			}
		}
		var tenant managedTenantRecord
		if err := tx.Where("tenant_id = ? AND owner_user_id = ?", tenantID, account.OwnerUserID).First(&tenant).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errBillingAccountNotFound
			}
			return err
		}
		record := tenantFundsRecord(account.ID, tenantID, now)
		err := tx.Where("billing_account_id = ? AND tenant_id = ?", account.ID, tenantID).First(&record).Error
		missing := errors.Is(err, gorm.ErrRecordNotFound)
		if err != nil && !missing {
			return err
		}
		if change != nil {
			same := record.LimitCents == nil && change.limit == nil || record.LimitCents != nil && change.limit != nil && *record.LimitCents == *change.limit
			if record.Revision != change.revision {
				if record.Revision == 0 || record.Revision-1 != change.revision || !same {
					return errBillingAccountConflict
				}
			} else if missing || !same {
				record.LimitCents = change.limit
				record.Revision++
				record.UpdatedAt = now
				if missing {
					err = tx.Omit(clause.Associations).Create(&record).Error
				} else {
					err = tx.Model(&record).Select("limit_cents", "revision", "updated_at").Updates(record).Error
				}
				if err != nil {
					return err
				}
			}
		}
		response, err = tenantFundsLimitResponse(tx, record)
		return err
	})
	if err != nil {
		return response, fmt.Errorf("read or change tenant limit %s: %w", tenantID, err)
	}
	return response, nil
}

func (service *managementService) fundsTenantLimitHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		if ctx.Request.URL.RawQuery != "" {
			writeBillingAccountError(ctx, errBillingAccountInvalid)
			return
		}
		var change *tenantFundsLimitChange
		if ctx.Request.Method == http.MethodPut {
			value, err := decodeTenantFundsLimit(ctx)
			if err != nil {
				writeBillingAccountError(ctx, err)
				return
			}
			change = &value
		}
		response, err := service.store.database.tenantFundsLimit(ctx.Request.Context(), account, ctx.Param("tenant_id"), change, service.store.now().UTC())
		if err != nil {
			writeBillingAccountError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}
