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
	resolutionChargeLimit *big.Rat
	ResolutionChargeLimit ExactMoney `json:"resolution_charge_limit"`
	KnownProviderCost     ExactMoney `json:"known_provider_cost"`
	KnownPlatformExposure ExactMoney `json:"known_platform_exposure"`
	ProviderCostComplete  bool       `json:"provider_cost_complete"`
}

func readHostedFundsExposure(tx *gorm.DB, accountID, requestID string, maximum *big.Rat) (hostedFundsExposure, error) {
	reader := &gormManagedTenantDatabase{database: tx}
	summary, err := reader.billingUsageChargeSummary(tx.Statement.Context, accountID, requestID)
	if err != nil {
		return hostedFundsExposure{}, err
	}
	authorized := new(big.Rat).Set(maximum)
	excess := new(big.Rat).Sub(summary.knownProviderCost, authorized)
	if excess.Sign() < 0 {
		excess.SetInt64(0)
	}
	if summary.providerCostComplete {
		if summary.knownCustomerQuote.Cmp(authorized) < 0 {
			authorized.Set(summary.knownCustomerQuote)
		}
	}
	authorized.Sub(authorized, summary.retainedCustomerCredits)
	if authorized.Sign() < 0 {
		authorized.SetInt64(0)
	}
	return hostedFundsExposure{resolutionChargeLimit: authorized, KnownProviderCost: ratingMoney(summary.knownProviderCost), KnownPlatformExposure: ratingMoney(excess), ProviderCostComplete: summary.providerCostComplete, ResolutionChargeLimit: ratingMoney(authorized)}, nil
}

// Retained charges remain the source of costs. This projection and case share
// the observation delivery transaction, including late evidence after a waiver.
func recordHostedFundsExposure(tx *gorm.DB, charge ratedJournalObservation, now time.Time) error {
	exposure, err := readHostedFundsExposure(tx, charge.BillingAccountID, charge.RequestID, charge.authorizedMaximum)
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
