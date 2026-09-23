package proxy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type managedChargeAdjustmentRecord struct {
	ID                string                      `gorm:"primaryKey"`
	BillingAccountID  string                      `gorm:"not null;index"`
	BillingAccount    managedBillingAccountRecord `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	ChargeID          string                      `gorm:"not null;index"`
	Charge            managedChargeRecord         `gorm:"foreignKey:ChargeID;references:ID;constraint:OnDelete:RESTRICT"`
	CreditNumerator   string                      `gorm:"not null"`
	CreditDenominator string                      `gorm:"not null"`
	Reason            string                      `gorm:"not null"`
	CreatedAt         time.Time                   `gorm:"not null"`
}

type customerChargeAdjustment struct{ record managedChargeAdjustmentRecord }
type chargeAdjustmentSettlement func(*gorm.DB, managedChargeAdjustmentRecord) error

func newCustomerChargeAdjustment(accountID, chargeID, eventKey, reason string, credit ExactMoney, now time.Time) (customerChargeAdjustment, error) {
	amount, err := parseExactMoney(credit)
	if err != nil || amount.Sign() <= 0 || accountID == "" || len(accountID) > 128 || strings.TrimSpace(accountID) != accountID || !strings.HasPrefix(chargeID, chargeIDPrefix) || !validIdempotencyKey(eventKey) || !journalDimensionPattern.MatchString(reason) || len(reason) > 64 || now.IsZero() {
		return customerChargeAdjustment{}, fmt.Errorf("%w: invalid customer charge adjustment", ErrCatalogRatingInvalid)
	}
	canonical := ratingMoney(amount)
	return customerChargeAdjustment{record: managedChargeAdjustmentRecord{
		ID: "adjustment-" + sha256Hex(accountID + "\x00" + eventKey)[:32], BillingAccountID: accountID, ChargeID: chargeID,
		CreditNumerator: canonical.Numerator, CreditDenominator: canonical.Denominator, Reason: reason, CreatedAt: now.UTC(),
	}}, nil
}

// A trusted financial command supplies settlement. Customers cannot create
// credits through the read-only charge resources.
func (database *gormManagedTenantDatabase) applyCustomerChargeAdjustment(ctx context.Context, command customerChargeAdjustment, settle chargeAdjustmentSettlement) error {
	return database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		proposed := command.record
		lock := transaction.Model(&managedBillingAccountRecord{}).Where("id = ?", proposed.BillingAccountID).UpdateColumn("id", gorm.Expr("id"))
		if lock.Error != nil {
			return fmt.Errorf("lock credit account: %w", lock.Error)
		}
		if lock.RowsAffected != 1 {
			return errUsageJournalNotFound
		}
		var charge managedChargeRecord
		if err := transaction.Where("id = ? AND billing_account_id = ?", proposed.ChargeID, proposed.BillingAccountID).First(&charge).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errUsageJournalNotFound
			}
			return fmt.Errorf("read adjustment charge: %w", err)
		}
		var existing managedChargeAdjustmentRecord
		err := transaction.Where("id = ?", proposed.ID).First(&existing).Error
		if err == nil {
			if existing.ChargeID != proposed.ChargeID || existing.BillingAccountID != proposed.BillingAccountID || existing.CreditNumerator != proposed.CreditNumerator || existing.CreditDenominator != proposed.CreditDenominator || existing.Reason != proposed.Reason {
				return errUsageJournalConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read retained adjustment: %w", err)
		}
		if charge.State != chargeRated {
			return errUsageJournalConflict
		}
		rating, err := decodeRetainedCharge(charge)
		if err != nil {
			return err
		}
		adjustments := []managedChargeAdjustmentRecord{}
		if err := transaction.Where("charge_id = ? AND billing_account_id = ?", charge.ID, charge.BillingAccountID).Find(&adjustments).Error; err != nil {
			return fmt.Errorf("read charge credits: %w", err)
		}
		adjustments = append(adjustments, proposed)
		if _, err := netCustomerCharge(rating.CustomerCharge, adjustments); err != nil {
			return fmt.Errorf("%w: %w", errUsageJournalConflict, err)
		}
		if err := transaction.Omit(clause.Associations).Create(&proposed).Error; err != nil {
			return fmt.Errorf("retain customer credit: %w", err)
		}
		if err := settle(transaction, proposed); err != nil {
			return fmt.Errorf("settle customer credit %s: %w", proposed.ID, err)
		}
		return nil
	})
}

func netCustomerCharge(original ExactMoney, adjustments []managedChargeAdjustmentRecord) (ExactMoney, error) {
	net, err := parseExactMoney(original)
	if err != nil {
		return ExactMoney{}, err
	}
	for _, adjustment := range adjustments {
		credit, err := parseExactMoney(ExactMoney{Numerator: adjustment.CreditNumerator, Denominator: adjustment.CreditDenominator})
		if err != nil || credit.Sign() <= 0 || !journalDimensionPattern.MatchString(adjustment.Reason) || adjustment.CreatedAt.IsZero() {
			return ExactMoney{}, fmt.Errorf("invalid retained customer credit")
		}
		net.Sub(net, credit)
	}
	if net.Sign() < 0 {
		return ExactMoney{}, fmt.Errorf("customer credits exceed original charge")
	}
	return ratingMoney(net), nil
}

func (database *gormManagedTenantDatabase) billingChargeAdjustments(ctx context.Context, accountID string, ids []string) ([]managedChargeAdjustmentRecord, error) {
	adjustments := []managedChargeAdjustmentRecord{}
	if err := database.database.WithContext(ctx).Where("billing_account_id = ? AND charge_id IN ?", accountID, ids).Order("id").Find(&adjustments).Error; err != nil {
		return nil, fmt.Errorf("read customer charge adjustments: %w", err)
	}
	return adjustments, nil
}

func (service *managementService) chargeResponses(ctx context.Context, accountID string, records []managedChargeRecord) ([]managementChargeResponse, error) {
	responses := make([]managementChargeResponse, 0, len(records))
	if len(records) == 0 {
		return responses, nil
	}
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	adjustments, err := service.store.database.billingChargeAdjustments(ctx, accountID, ids)
	if err != nil {
		return nil, err
	}
	byCharge := map[string][]managedChargeAdjustmentRecord{}
	for _, adjustment := range adjustments {
		byCharge[adjustment.ChargeID] = append(byCharge[adjustment.ChargeID], adjustment)
	}
	for _, record := range records {
		response, err := chargeResponse(record, byCharge[record.ID])
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}
