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

// hostedFundsBalance retains validated amounts until a transport formats them.
type hostedFundsBalance struct {
	state             string
	postedCents       int64
	reservedCents     int64
	spentCents        int64
	pendingCents      int64
	unsettledFraction *big.Rat
}

func (balance hostedFundsBalance) response() managementFundsBalanceResponse {
	return managementFundsBalanceResponse{
		Currency: CatalogCurrencyUSD, State: balance.state,
		PostedCents:       strconv.FormatInt(balance.postedCents, 10),
		AvailableCents:    new(big.Int).Sub(big.NewInt(balance.postedCents), big.NewInt(balance.reservedCents)).String(),
		ReservedCents:     strconv.FormatInt(balance.reservedCents, 10),
		SpentCents:        strconv.FormatInt(balance.spentCents, 10),
		PendingCents:      strconv.FormatInt(balance.pendingCents, 10),
		UnsettledFraction: ratingMoney(balance.unsettledFraction),
	}
}

func (database *gormManagedTenantDatabase) billingFundsBalance(ctx context.Context, accountID string, now time.Time) (hostedFundsBalance, error) {
	balance := hostedFundsBalance{state: fundsAccountActive, unsettledFraction: new(big.Rat)}
	err := database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var financial managedFundsAccountRecord
		err := tx.Where("billing_account_id = ?", accountID).First(&financial).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			balance.state = financial.State
			fraction, err := parseUSDCentRemainder(ExactMoney{Numerator: financial.RemainderNumerator, Denominator: financial.RemainderDenominator})
			if err != nil {
				return fmt.Errorf("read retained remainder: %w", err)
			}
			balance.unsettledFraction = fraction
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
			balance.postedCents = total.Int64()
			balance.reservedCents = holds.Int64()
		}
		var spent, pending int64
		if err := tx.Model(&managedFundsSettlementRecord{}).Where("billing_account_id = ?", accountID).Select("coalesce(sum(settled_cents), 0)").Scan(&spent).Error; err != nil {
			return err
		}
		if err := tx.Model(&managedFundsReservationRecord{}).Where("billing_account_id = ? AND state = ?", accountID, fundsReservationReconciliation).Select("coalesce(sum(maximum_cents), 0)").Scan(&pending).Error; err != nil {
			return err
		}
		balance.spentCents = spent
		balance.pendingCents = pending
		restricted, err := paymentFundsRestricted(tx, accountID, now)
		if err != nil {
			return err
		}
		if restricted && balance.state == fundsAccountActive {
			balance.state = "suspended"
		}
		return nil
	})
	if err != nil {
		return hostedFundsBalance{}, fmt.Errorf("read funds for billing account %s: %w", accountID, err)
	}
	return balance, nil
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
		balance, err := service.store.database.billingFundsBalance(ctx.Request.Context(), account.ID, service.store.now())
		if err != nil {
			writeBillingAccountError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, balance.response())
	}
}
