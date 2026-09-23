package proxy

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type managedFundsCreditRecord struct {
	AdjustmentID     string                        `gorm:"primaryKey"`
	Adjustment       managedChargeAdjustmentRecord `gorm:"foreignKey:AdjustmentID;references:ID;constraint:OnDelete:RESTRICT"`
	RequestID        string                        `gorm:"not null;index"`
	Request          managedJournalRequestRecord   `gorm:"foreignKey:RequestID;references:ID;constraint:OnDelete:RESTRICT"`
	BillingAccountID string                        `gorm:"not null;index"`
	BillingAccount   managedBillingAccountRecord   `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	Effect           fundsCreditEffect             `gorm:"embedded"`
	CreatedAt        time.Time                     `gorm:"not null"`
}

// fundsCreditEffect retains the exact remainder transition for a credit.
type fundsCreditEffect struct {
	CreditedCents              int64  `gorm:"not null;check:credited_cents >= 0"`
	RemainderBeforeNumerator   string `gorm:"not null"`
	RemainderBeforeDenominator string `gorm:"not null"`
	RemainderAfterNumerator    string `gorm:"not null"`
	RemainderAfterDenominator  string `gorm:"not null"`
}

// applyCustomerChargeAdjustment owns the account lock, credit authorization,
// and idempotency. This callback shares its transaction with the Ledger effect.
func settleHostedChargeAdjustment(transaction *gorm.DB, adjustment managedChargeAdjustmentRecord) error {
	var charge managedChargeRecord
	if err := transaction.Where("id = ? AND billing_account_id = ?", adjustment.ChargeID, adjustment.BillingAccountID).First(&charge).Error; err != nil {
		return fmt.Errorf("read financial credit charge %s: %w", adjustment.ChargeID, err)
	}
	var reservation managedFundsReservationRecord
	if err := transaction.Where("request_id = ? AND billing_account_id = ?", charge.RequestID, adjustment.BillingAccountID).First(&reservation).Error; err != nil {
		return fmt.Errorf("read financial credit reservation %s: %w", charge.RequestID, err)
	}
	switch reservation.State {
	case fundsReservationHeld, fundsReservationReconciliation:
		// The charge has not affected the balance. Settlement includes this
		// retained credit when it calculates the net customer charge.
		return nil
	case fundsReservationSettled:
	default:
		return fmt.Errorf("%w: credit requires a retained charge reservation", errUsageJournalConflict)
	}
	if err := authorizeSettledFundsCredit(transaction, charge.RequestID, adjustment); err != nil {
		return err
	}
	credit := ExactMoney{Numerator: adjustment.CreditNumerator, Denominator: adjustment.CreditDenominator}
	effect, err := applyHostedFundsCredit(transaction, adjustment.BillingAccountID, charge.RequestID, credit, func(cents int64) error {
		return postHostedChargeCredit(transaction, adjustment, cents)
	})
	if err != nil {
		return err
	}
	receipt := managedFundsCreditRecord{AdjustmentID: adjustment.ID, RequestID: charge.RequestID, BillingAccountID: adjustment.BillingAccountID, Effect: effect, CreatedAt: adjustment.CreatedAt}
	if err := transaction.Omit(clause.Associations).Create(&receipt).Error; err != nil {
		return fmt.Errorf("retain financial credit %s: %w", adjustment.ID, err)
	}
	return nil
}

func applyHostedFundsCredit(transaction *gorm.DB, accountID, requestID string, credit ExactMoney, post func(int64) error) (fundsCreditEffect, error) {
	var financial managedFundsAccountRecord
	if err := transaction.Where("billing_account_id = ?", accountID).First(&financial).Error; err != nil {
		return fundsCreditEffect{}, fmt.Errorf("read financial credit account %s: %w", accountID, err)
	}
	cents, remainder, err := CreditUSDCents(credit, ExactMoney{Numerator: financial.RemainderNumerator, Denominator: financial.RemainderDenominator})
	if err != nil {
		return fundsCreditEffect{}, fmt.Errorf("calculate financial credit for request %s: %w", requestID, err)
	}
	if cents > 0 {
		if err := post(cents); err != nil {
			return fundsCreditEffect{}, err
		}
	}
	effect := fundsCreditEffect{CreditedCents: cents,
		RemainderBeforeNumerator: financial.RemainderNumerator, RemainderBeforeDenominator: financial.RemainderDenominator,
		RemainderAfterNumerator: remainder.Numerator, RemainderAfterDenominator: remainder.Denominator}
	if err := transaction.Model(&financial).Updates(map[string]any{"remainder_numerator": remainder.Numerator, "remainder_denominator": remainder.Denominator}).Error; err != nil {
		return fundsCreditEffect{}, fmt.Errorf("retain credit remainder for request %s: %w", requestID, err)
	}
	amount, err := parseExactMoney(credit)
	if err != nil {
		return fundsCreditEffect{}, err
	}
	if err := adjustHostedTenantUsage(transaction, accountID, requestID, amount.Neg(amount)); err != nil {
		return fundsCreditEffect{}, err
	}
	return effect, nil
}

// Charge records preserve their original amount. A financial decision can
// settle a smaller net amount, which bounds all subsequent request credits.
func authorizeSettledFundsCredit(transaction *gorm.DB, requestID string, proposed managedChargeAdjustmentRecord) error {
	return authorizeHostedFundsCredit(transaction, proposed.BillingAccountID, requestID, ExactMoney{Numerator: proposed.CreditNumerator, Denominator: proposed.CreditDenominator})
}

func authorizeHostedFundsCredit(transaction *gorm.DB, accountID, requestID string, proposed ExactMoney) error {
	var settlement managedFundsSettlementRecord
	if err := transaction.Where("request_id = ? AND billing_account_id = ?", requestID, accountID).First(&settlement).Error; err != nil {
		return fmt.Errorf("read credited settlement %s: %w", requestID, err)
	}
	remaining, err := parseExactMoney(ExactMoney{Numerator: settlement.ChargeNumerator, Denominator: settlement.ChargeDenominator})
	if err != nil {
		return err
	}
	posted := transaction.Model(&managedFundsCreditRecord{}).Select("adjustment_id").Where("request_id = ? AND billing_account_id = ?", requestID, accountID)
	var credits []managedChargeAdjustmentRecord
	if err := transaction.Where("id IN (?)", posted).Find(&credits).Error; err != nil {
		return fmt.Errorf("read settled credits %s: %w", requestID, err)
	}
	amounts := []ExactMoney{proposed}
	for _, credit := range credits {
		amounts = append(amounts, ExactMoney{Numerator: credit.CreditNumerator, Denominator: credit.CreditDenominator})
	}
	var corrections []managedFundsCorrectionRecord
	if err := transaction.Where("request_id = ? AND billing_account_id = ?", requestID, accountID).Find(&corrections).Error; err != nil {
		return fmt.Errorf("read request credits %s: %w", requestID, err)
	}
	for _, correction := range corrections {
		amounts = append(amounts, ExactMoney{Numerator: correction.CreditNumerator, Denominator: correction.CreditDenominator})
	}
	for _, credit := range amounts {
		amount, err := parseExactMoney(credit)
		if err != nil {
			return err
		}
		remaining.Sub(remaining, amount)
	}
	if remaining.Sign() < 0 {
		return fmt.Errorf("%w: credits exceed the settled request charge", errUsageJournalConflict)
	}
	return nil
}

func postHostedChargeCredit(transaction *gorm.DB, adjustment managedChargeAdjustmentRecord, cents int64) error {
	encoded, err := json.Marshal(struct {
		AdjustmentID string `json:"adjustment_id"`
		ChargeID     string `json:"charge_id"`
	}{adjustment.ID, adjustment.ChargeID})
	if err != nil {
		return fmt.Errorf("encode financial credit metadata: %w", err)
	}
	return postHostedFundsCredit(transaction, adjustment.BillingAccountID, "usage-credit:"+adjustment.ID, cents, adjustment.CreatedAt, encoded)
}

func postHostedFundsCredit(transaction *gorm.DB, accountID, eventKey string, cents int64, now time.Time, encoded []byte) error {
	account, err := newHostedLedgerAccount(transaction, accountID, now)
	if err != nil {
		return err
	}
	amount, err := ledger.NewPositiveAmountCents(cents)
	if err != nil {
		return err
	}
	key, err := ledger.NewIdempotencyKey(eventKey)
	if err != nil {
		return err
	}
	metadata, err := ledger.NewMetadataJSON(string(encoded))
	if err != nil {
		return err
	}
	if err := account.service.Grant(transaction.Statement.Context, account.tenant, account.user, account.namespace, amount, key, 0, metadata); err != nil {
		return fmt.Errorf("post shared ledger credit %s: %w", eventKey, err)
	}
	return nil
}
