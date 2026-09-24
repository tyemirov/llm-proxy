package proxy

import (
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type managedFundsSettlementRecord struct {
	RequestID                  string                        `gorm:"primaryKey"`
	Reservation                managedFundsReservationRecord `gorm:"belongsTo:Reservation;foreignKey:RequestID;references:RequestID;constraint:OnDelete:RESTRICT"`
	BillingAccountID           string                        `gorm:"not null;index"`
	BillingAccount             managedBillingAccountRecord   `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	ChargeNumerator            string                        `gorm:"not null"`
	ChargeDenominator          string                        `gorm:"not null"`
	AdjustmentIDs              []byte                        `gorm:"not null"`
	SettledCents               int64                         `gorm:"not null;check:settled_cents >= 0"`
	RemainderBeforeNumerator   string                        `gorm:"not null"`
	RemainderBeforeDenominator string                        `gorm:"not null"`
	RemainderAfterNumerator    string                        `gorm:"not null"`
	RemainderAfterDenominator  string                        `gorm:"not null"`
	CreatedAt                  time.Time                     `gorm:"not null"`
}

// The journal owns this transaction and its delivery acknowledgment. No funds
// are released until every attempt has a final, bounded customer charge.
func deliverHostedFunds(transaction *gorm.DB, observation managedJournalObservationRecord) error {
	charge, err := rateJournalObservation(transaction, observation)
	if err != nil {
		return err
	}
	if err := recordHostedFundsExposure(transaction, charge, observation.ObservedAt); err != nil {
		return err
	}
	return settleHostedFunds(transaction, charge.RequestID, observation.ObservedAt)
}

func settleHostedFunds(transaction *gorm.DB, requestID string, now time.Time) error {
	var request managedJournalRequestRecord
	if err := transaction.Where("id = ?", requestID).First(&request).Error; err != nil {
		return fmt.Errorf("read settlement request %s: %w", requestID, err)
	}
	lock := transaction.Model(&managedBillingAccountRecord{}).Where("id = ?", request.BillingAccountID).UpdateColumn("id", gorm.Expr("id"))
	if lock.Error != nil {
		return fmt.Errorf("lock settlement account %s: %w", request.BillingAccountID, lock.Error)
	}
	var reservation managedFundsReservationRecord
	if err := transaction.Where("request_id = ? AND billing_account_id = ?", request.ID, request.BillingAccountID).First(&reservation).Error; err != nil {
		return fmt.Errorf("read settlement reservation %s: %w", request.ID, err)
	}
	if reservation.State == fundsReservationSettled || reservation.State == fundsReservationReconciliation {
		return nil
	}
	if reservation.State != fundsReservationHeld {
		return fmt.Errorf("request %s has no held reservation", request.ID)
	}
	var charges []managedChargeRecord
	if err := transaction.Where("request_id = ? AND billing_account_id = ?", request.ID, request.BillingAccountID).Order("id").Find(&charges).Error; err != nil {
		return fmt.Errorf("read settlement charges %s: %w", request.ID, err)
	}
	for _, charge := range charges {
		if charge.State != chargeRated {
			return retainHostedFundsForReconciliation(transaction, reservation, charge.State, now)
		}
	}
	if request.State != journalRequestCompleted || request.ResultPublishedAt == nil {
		return nil
	}
	var attempts int64
	if err := transaction.Model(&managedJournalAttemptRecord{}).Where("request_id = ?", request.ID).Count(&attempts).Error; err != nil {
		return fmt.Errorf("count settlement attempts %s: %w", request.ID, err)
	}
	if int64(len(charges)) != attempts || attempts == 0 {
		return nil
	}
	total := new(big.Rat)
	creditIDs := []string{}
	reader := &gormManagedTenantDatabase{database: transaction}
	for _, charge := range charges {
		rating, err := decodeRetainedCharge(charge)
		if err != nil {
			return err
		}
		credits, err := reader.billingChargeAdjustments(transaction.Statement.Context, request.BillingAccountID, []string{charge.ID})
		if err != nil {
			return err
		}
		for _, credit := range credits {
			creditIDs = append(creditIDs, credit.ID)
		}
		net, err := netCustomerCharge(rating.CustomerCharge, credits)
		if err != nil {
			return err
		}
		total.Add(total, net)
	}
	return commitHostedFundsSettlement(transaction, reservation, total, creditIDs, now)
}

func commitHostedFundsSettlement(transaction *gorm.DB, reservation managedFundsReservationRecord, total *big.Rat, creditIDs []string, now time.Time) error {
	var financial managedFundsAccountRecord
	if err := transaction.Where("billing_account_id = ?", reservation.BillingAccountID).First(&financial).Error; err != nil {
		return fmt.Errorf("read settlement account %s: %w", reservation.BillingAccountID, err)
	}
	exact := ratingMoney(total)
	cents, remainder, err := SettleUSDCents(exact, ExactMoney{Numerator: financial.RemainderNumerator, Denominator: financial.RemainderDenominator})
	if err != nil {
		return fmt.Errorf("calculate settlement %s: %w", reservation.RequestID, err)
	}
	if cents > reservation.MaximumCents {
		return retainHostedFundsForReconciliation(transaction, reservation, chargeLimitUnresolved, now)
	}
	if err := postHostedFundsSettlement(transaction, reservation, cents, now); err != nil {
		return err
	}
	encodedCreditIDs, err := json.Marshal(creditIDs)
	if err != nil {
		return fmt.Errorf("encode settlement credits %s: %w", reservation.RequestID, err)
	}
	settlement := managedFundsSettlementRecord{RequestID: reservation.RequestID, BillingAccountID: reservation.BillingAccountID,
		ChargeNumerator: exact.Numerator, ChargeDenominator: exact.Denominator, AdjustmentIDs: encodedCreditIDs, SettledCents: cents,
		RemainderBeforeNumerator: financial.RemainderNumerator, RemainderBeforeDenominator: financial.RemainderDenominator,
		RemainderAfterNumerator: remainder.Numerator, RemainderAfterDenominator: remainder.Denominator, CreatedAt: now}
	if err := transaction.Omit(clause.Associations).Create(&settlement).Error; err != nil {
		return fmt.Errorf("retain settlement %s: %w", reservation.RequestID, err)
	}
	if err := transaction.Model(&financial).Updates(map[string]any{"remainder_numerator": remainder.Numerator, "remainder_denominator": remainder.Denominator}).Error; err != nil {
		return fmt.Errorf("retain settlement remainder %s: %w", reservation.RequestID, err)
	}
	if err := adjustHostedTenantUsage(transaction, reservation.BillingAccountID, reservation.RequestID, total); err != nil {
		return err
	}
	return updateHostedFundsReservation(transaction, reservation, fundsReservationSettled, now)
}

func postHostedFundsSettlement(transaction *gorm.DB, reservation managedFundsReservationRecord, cents int64, now time.Time) error {
	account, err := newHostedLedgerAccount(transaction, reservation.BillingAccountID, now)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(struct {
		RequestID string `json:"request_id"`
	}{reservation.RequestID})
	if err != nil {
		return fmt.Errorf("encode settlement identity: %w", err)
	}
	metadata, err := ledger.NewMetadataJSON(string(encoded))
	if err != nil {
		return err
	}
	if reservation.MaximumCents > 0 {
		identifier, err := ledger.NewReservationID(reservation.RequestID)
		if err != nil {
			return err
		}
		key, err := ledger.NewIdempotencyKey("settle-release:" + reservation.RequestID)
		if err != nil {
			return err
		}
		if err := account.service.Release(transaction.Statement.Context, account.tenant, account.user, account.namespace, identifier, key, metadata); err != nil {
			return fmt.Errorf("release settled reservation %s: %w", reservation.RequestID, err)
		}
	}
	if cents > 0 {
		amount, err := ledger.NewPositiveAmountCents(cents)
		if err != nil {
			return err
		}
		key, err := ledger.NewIdempotencyKey("settle-charge:" + reservation.RequestID)
		if err != nil {
			return err
		}
		if err := account.service.Spend(transaction.Statement.Context, account.tenant, account.user, account.namespace, amount, key, metadata); err != nil {
			return fmt.Errorf("post settled charge %s: %w", reservation.RequestID, err)
		}
	}
	return nil
}

func updateHostedFundsReservation(transaction *gorm.DB, reservation managedFundsReservationRecord, state string, now time.Time) error {
	result := transaction.Model(&managedFundsReservationRecord{}).Where("request_id = ? AND revision = ?", reservation.RequestID, reservation.Revision).
		Updates(map[string]any{"state": state, "revision": reservation.Revision + 1, "updated_at": now})
	if result.Error != nil {
		return fmt.Errorf("update funds reservation %s: %w", reservation.RequestID, result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("update funds reservation %s: %w", reservation.RequestID, errUsageJournalConflict)
	}
	return nil
}

func retainHostedFundsForReconciliation(transaction *gorm.DB, reservation managedFundsReservationRecord, reason string, now time.Time) error {
	record := managedJournalCaseRecord{ID: journalCaseIDPrefix + sha256Hex(reservation.RequestID + ":" + reason)[:32], RequestID: reservation.RequestID, Reason: reason, CreatedAt: now}
	if err := transaction.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
		return fmt.Errorf("retain financial case for request %s: %w", reservation.RequestID, err)
	}
	return updateHostedFundsReservation(transaction, reservation, fundsReservationReconciliation, now)
}
