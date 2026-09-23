package proxy

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	chargeIDPrefix         = "charge-"
	chargeRated            = "rated"
	chargeUsageUnresolved  = "usage_unresolved"
	chargePolicyUnresolved = "policy_unresolved"
	chargeLimitUnresolved  = "limit_unresolved"
)

type managedChargeRecord struct {
	ID               string                          `gorm:"primaryKey"`
	BillingAccountID string                          `gorm:"not null;index"`
	BillingAccount   managedBillingAccountRecord     `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	RequestID        string                          `gorm:"not null;index"`
	Request          managedJournalRequestRecord     `gorm:"foreignKey:RequestID;references:ID;constraint:OnDelete:RESTRICT"`
	AttemptID        string                          `gorm:"not null;uniqueIndex"`
	Attempt          managedJournalAttemptRecord     `gorm:"foreignKey:AttemptID;references:ID;constraint:OnDelete:RESTRICT"`
	ObservationID    string                          `gorm:"not null;uniqueIndex"`
	Observation      managedJournalObservationRecord `gorm:"foreignKey:ObservationID;references:ID;constraint:OnDelete:RESTRICT"`
	PriceSnapshotID  string                          `gorm:"not null;index"`
	PriceSnapshot    managedPriceSnapshotRecord      `gorm:"foreignKey:PriceSnapshotID;references:ID;constraint:OnDelete:RESTRICT"`
	State            string                          `gorm:"not null;check:state IN ('rated','usage_unresolved','policy_unresolved','limit_unresolved')"`
	Rating           []byte                          `gorm:"not null"`
	RatingDigest     string                          `gorm:"not null"`
	CreatedAt        time.Time                       `gorm:"not null"`
}

func initializeHostedRatingSchema(database *gorm.DB) error {
	models := []any{&managedPriceSnapshotRecord{}, &managedChargeRecord{}, &managedChargeAdjustmentRecord{}}
	present := 0
	for _, model := range models {
		if database.Migrator().HasTable(model) {
			present++
		}
	}
	if present == 0 {
		if err := database.AutoMigrate(models...); err != nil {
			return fmt.Errorf("%w: create hosted rating schema: %w", errManagedTenantSchemaMigration, err)
		}
		return nil
	}
	if present != len(models) {
		return fmt.Errorf("%w: partial hosted rating schema", errManagedTenantSchemaMigration)
	}
	for _, model := range models {
		if err := validateHostedTable(database, model); err != nil {
			return fmt.Errorf("%w: validate hosted rating schema: %w", errManagedTenantSchemaMigration, err)
		}
	}
	return nil
}

type journalChargeSettlement func(*gorm.DB, managedChargeRecord) error

// Rating, settlement, and the delivery acknowledgment share the journal transaction.
func newJournalRatingDelivery(settle journalChargeSettlement) journalAccountingDelivery {
	return func(transaction *gorm.DB, observation managedJournalObservationRecord) error {
		charge, err := rateJournalObservation(transaction, observation)
		if err != nil {
			return err
		}
		if charge.State != chargeRated {
			return nil
		}
		return settle(transaction, charge)
	}
}

func rateJournalObservation(transaction *gorm.DB, observation managedJournalObservationRecord) (managedChargeRecord, error) {
	var attempt managedJournalAttemptRecord
	if err := transaction.Where("id = ?", observation.AttemptID).First(&attempt).Error; err != nil {
		return managedChargeRecord{}, fmt.Errorf("read rated attempt: %w", err)
	}
	var request managedJournalRequestRecord
	if err := transaction.Where("id = ?", attempt.RequestID).First(&request).Error; err != nil {
		return managedChargeRecord{}, fmt.Errorf("read rated request: %w", err)
	}
	var retained managedPriceSnapshotRecord
	if err := transaction.Where("request_id = ? AND billing_account_id = ?", request.ID, request.BillingAccountID).First(&retained).Error; err != nil {
		return managedChargeRecord{}, fmt.Errorf("read accepted price snapshot: %w", err)
	}
	snapshot, document, err := restoreHostedPriceSnapshot(retained)
	if err != nil {
		return managedChargeRecord{}, err
	}
	if document.Provider != request.Provider || document.Model != request.Model || document.Operation != request.Operation || document.CatalogRevision != request.CatalogRevision {
		return managedChargeRecord{}, fmt.Errorf("retained price route differs from request")
	}
	var quantities []journalQuantity
	if err := decodeRatingJSON(observation.Quantities, &quantities); err != nil {
		return managedChargeRecord{}, err
	}
	result, err := snapshot.Rate(quantities)
	if err != nil {
		return managedChargeRecord{}, fmt.Errorf("rate observation %s: %w", observation.ID, err)
	}
	state := chargeRated
	switch {
	case result.State == CatalogRatingUnresolved:
		state = chargeUsageUnresolved
	case observation.Outcome == journalOutcomeFail:
		state = chargePolicyUnresolved
	case exceedsHostedPriceBound(document, attempt.Number, quantities):
		state = chargeLimitUnresolved
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return managedChargeRecord{}, fmt.Errorf("encode rated observation: %w", err)
	}
	charge := managedChargeRecord{ID: chargeIDPrefix + sha256Hex(observation.ID)[:32], BillingAccountID: request.BillingAccountID, RequestID: request.ID, AttemptID: attempt.ID, ObservationID: observation.ID, PriceSnapshotID: retained.ID, State: state, Rating: encoded, RatingDigest: sha256Hex(string(encoded)), CreatedAt: observation.ObservedAt}
	if err := transaction.Omit(clause.Associations).Create(&charge).Error; err != nil {
		return managedChargeRecord{}, fmt.Errorf("retain charge for observation %s: %w", observation.ID, err)
	}
	return charge, nil
}

func exceedsHostedPriceBound(document hostedPriceSnapshotDocument, attempt uint64, quantities []journalQuantity) bool {
	if attempt > uint64(document.Maximum.Attempts) {
		return true
	}
	measured := make(map[string]journalQuantity, len(quantities))
	for _, quantity := range quantities {
		measured[quantity.Dimension] = quantity
	}
	for _, bound := range document.Bounds {
		quantity, found := measured[bound.Dimension]
		if !found || quantity.UnknownReason != "" || ratingRational(quantity.Value).Cmp(ratingRational(bound.Maximum)) > 0 {
			return true
		}
	}
	return false
}

func decodeRetainedCharge(record managedChargeRecord) (CatalogRatedUsage, error) {
	var result CatalogRatedUsage
	if record.RatingDigest != sha256Hex(string(record.Rating)) {
		return result, fmt.Errorf("invalid retained charge digest")
	}
	if err := decodeRatingJSON(record.Rating, &result); err != nil {
		return result, err
	}
	switch result.State {
	case CatalogRatingResolved:
		for _, amount := range []ExactMoney{result.ProviderCost, result.CustomerCharge, result.MinimumAdjustment} {
			if _, err := parseExactMoney(amount); err != nil {
				return result, err
			}
		}
	case CatalogRatingUnresolved:
		if record.State != chargeUsageUnresolved || len(result.UnresolvedDimensions) == 0 {
			return result, fmt.Errorf("invalid unresolved charge")
		}
	default:
		return result, fmt.Errorf("invalid retained rating state")
	}
	return result, nil
}
