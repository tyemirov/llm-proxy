package proxy

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const managementFundsBalancePath = managementBillingAccountPath + "/balance"

type managementFundsBalanceResponse struct {
	Currency          string     `json:"currency"`
	State             string     `json:"state"`
	PostedCents       string     `json:"posted_cents"`
	AvailableCents    string     `json:"available_cents"`
	ReservedCents     string     `json:"reserved_cents"`
	SpentCents        string     `json:"spent_cents"`
	PendingCents      string     `json:"pending_cents"`
	UnsettledFraction ExactMoney `json:"unsettled_fraction"`
}

func (database *gormManagedTenantDatabase) billingFundsBalance(ctx context.Context, accountID string, now time.Time) (managementFundsBalanceResponse, error) {
	response := managementFundsBalanceResponse{Currency: CatalogCurrencyUSD, State: fundsAccountActive, PostedCents: "0", AvailableCents: "0", ReservedCents: "0", SpentCents: "0", PendingCents: "0", UnsettledFraction: ExactMoney{Numerator: "0", Denominator: "1"}}
	err := database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var financial managedFundsAccountRecord
		err := tx.Where("billing_account_id = ?", accountID).First(&financial).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			response.State = financial.State
			response.UnsettledFraction = ExactMoney{Numerator: financial.RemainderNumerator, Denominator: financial.RemainderDenominator}
		}
		// Read the shared account without the service's create-on-read behavior.
		var account gormstore.LedgerAccount
		err = tx.Where("tenant_id = ? AND user_id = ? AND ledger_id = ?", hostedLedgerTenant, accountID, CatalogCurrencyUSD).First(&account).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			identifier, err := ledger.NewAccountID(account.AccountID)
			if err != nil {
				return err
			}
			store := gormstore.New(tx)
			total, err := store.SumTotal(ctx, identifier, now.Unix())
			if err != nil {
				return err
			}
			holds, err := store.SumActiveHolds(ctx, identifier, now.Unix())
			if err != nil {
				return err
			}
			response.PostedCents = strconv.FormatInt(total.Int64(), 10)
			response.ReservedCents = strconv.FormatInt(holds.Int64(), 10)
			response.AvailableCents = new(big.Int).Sub(big.NewInt(total.Int64()), big.NewInt(holds.Int64())).String()
		}
		var spent, pending int64
		if err := tx.Model(&managedFundsSettlementRecord{}).Where("billing_account_id = ?", accountID).Select("coalesce(sum(settled_cents), 0)").Scan(&spent).Error; err != nil {
			return err
		}
		if err := tx.Model(&managedFundsReservationRecord{}).Where("billing_account_id = ? AND state = ?", accountID, fundsReservationReconciliation).Select("coalesce(sum(maximum_cents), 0)").Scan(&pending).Error; err != nil {
			return err
		}
		response.SpentCents = strconv.FormatInt(spent, 10)
		response.PendingCents = strconv.FormatInt(pending, 10)
		restricted, err := paymentFundsRestricted(tx, accountID, now)
		if err != nil {
			return err
		}
		if restricted && response.State == fundsAccountActive {
			response.State = "suspended"
		}
		return nil
	})
	if err != nil {
		return managementFundsBalanceResponse{}, fmt.Errorf("read funds for billing account %s: %w", accountID, err)
	}
	return response, nil
}

func (service *managementService) getFundsBalanceHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		if ctx.Request.URL.RawQuery != "" {
			writeBillingAccountError(ctx, errBillingAccountInvalid)
			return
		}
		response, err := service.store.database.billingFundsBalance(ctx.Request.Context(), account.ID, service.store.now())
		if err != nil {
			writeBillingAccountError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}
