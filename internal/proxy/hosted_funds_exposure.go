package proxy

import (
	"fmt"
	"math/big"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const journalCasePlatformExposure = "platform_exposure"

type managedFundsExposureRecord struct {
	RequestID            string                      `gorm:"primaryKey"`
	Request              managedJournalRequestRecord `gorm:"belongsTo:Request;foreignKey:RequestID;references:ID;constraint:OnDelete:RESTRICT"`
	BillingAccountID     string                      `gorm:"not null;index"`
	KnownCostNumerator   string                      `gorm:"not null"`
	KnownCostDenominator string                      `gorm:"not null"`
	ExcessNumerator      string                      `gorm:"not null"`
	ExcessDenominator    string                      `gorm:"not null"`
	ProviderCostComplete bool                        `gorm:"not null"`
	CreatedAt            time.Time                   `gorm:"not null"`
	UpdatedAt            time.Time                   `gorm:"not null"`
}

type hostedFundsExposure struct {
	ResolutionChargeLimit ExactMoney `json:"resolution_charge_limit"`
	KnownProviderCost     ExactMoney `json:"known_provider_cost"`
	KnownPlatformExposure ExactMoney `json:"known_platform_exposure"`
	ProviderCostComplete  bool       `json:"provider_cost_complete"`
}

func readHostedFundsExposure(tx *gorm.DB, accountID, requestID string, maximum ExactMoney) (hostedFundsExposure, error) {
	reader := &gormManagedTenantDatabase{database: tx}
	summary, err := reader.billingRequestChargeSummary(tx.Statement.Context, accountID, requestID)
	if err != nil {
		return hostedFundsExposure{}, err
	}
	known, err := parseExactMoney(summary.knownProviderCost)
	if err != nil {
		return hostedFundsExposure{}, err
	}
	authorized, err := parseExactMoney(maximum)
	if err != nil {
		return hostedFundsExposure{}, err
	}
	excess := new(big.Rat).Sub(known, authorized)
	if excess.Sign() < 0 {
		excess.SetInt64(0)
	}
	if summary.providerCostComplete {
		quoted, err := parseExactMoney(summary.knownCustomerQuote)
		if err != nil {
			return hostedFundsExposure{}, err
		}
		if quoted.Cmp(authorized) < 0 {
			authorized.Set(quoted)
		}
	}
	credits, err := parseExactMoney(summary.retainedCustomerCredits)
	if err != nil {
		return hostedFundsExposure{}, err
	}
	authorized.Sub(authorized, credits)
	if authorized.Sign() < 0 {
		authorized.SetInt64(0)
	}
	return hostedFundsExposure{KnownProviderCost: summary.knownProviderCost, KnownPlatformExposure: ratingMoney(excess), ProviderCostComplete: summary.providerCostComplete, ResolutionChargeLimit: ratingMoney(authorized)}, nil
}

// Retained charges remain the source of costs. This projection and case share
// the observation delivery transaction, including late evidence after a waiver.
func recordHostedFundsExposure(tx *gorm.DB, charge managedChargeRecord, now time.Time) error {
	var retained managedPriceSnapshotRecord
	if err := tx.Where("request_id = ? AND billing_account_id = ?", charge.RequestID, charge.BillingAccountID).First(&retained).Error; err != nil {
		return fmt.Errorf("read exposure authorization %s: %w", charge.RequestID, err)
	}
	_, document, err := restoreHostedPriceSnapshot(retained)
	if err != nil {
		return err
	}
	exposure, err := readHostedFundsExposure(tx, charge.BillingAccountID, charge.RequestID, document.Maximum.CustomerCharge)
	if err != nil {
		return err
	}
	if exposure.KnownPlatformExposure.Numerator == "0" {
		return nil
	}
	record := managedFundsExposureRecord{RequestID: charge.RequestID, BillingAccountID: charge.BillingAccountID, KnownCostNumerator: exposure.KnownProviderCost.Numerator, KnownCostDenominator: exposure.KnownProviderCost.Denominator, ExcessNumerator: exposure.KnownPlatformExposure.Numerator, ExcessDenominator: exposure.KnownPlatformExposure.Denominator, ProviderCostComplete: exposure.ProviderCostComplete, CreatedAt: now, UpdatedAt: now}
	updates := []string{"known_cost_numerator", "known_cost_denominator", "excess_numerator", "excess_denominator", "provider_cost_complete", "updated_at"}
	if err := tx.Omit(clause.Associations).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "request_id"}}, DoUpdates: clause.AssignmentColumns(updates)}).Create(&record).Error; err != nil {
		return fmt.Errorf("retain platform exposure %s: %w", charge.RequestID, err)
	}
	evidence := managedJournalCaseRecord{ID: journalCaseIDPrefix + sha256Hex(charge.RequestID + ":" + journalCasePlatformExposure)[:32], RequestID: charge.RequestID, Reason: journalCasePlatformExposure, CreatedAt: now}
	if err := tx.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&evidence).Error; err != nil {
		return fmt.Errorf("retain platform exposure case %s: %w", charge.RequestID, err)
	}
	return nil
}
