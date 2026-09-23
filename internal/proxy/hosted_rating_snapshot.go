package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const priceSnapshotIDPrefix = "price-"

type hostedSnapshotRate struct {
	ProviderRate CatalogPriceRate `json:"provider_rate"`
	CustomerRate ExactMoney       `json:"customer_rate"`
}

type hostedSnapshotComponent struct {
	Dimension           string               `json:"dimension"`
	AdditionalDimension string               `json:"additional_dimension"`
	Rates               []hostedSnapshotRate `json:"rates"`
}

type hostedPriceSnapshotDocument struct {
	CatalogRevision    string                    `json:"catalog_revision"`
	Provider           string                    `json:"provider"`
	Model              string                    `json:"model"`
	Operation          string                    `json:"operation"`
	Source             string                    `json:"source"`
	LastVerified       string                    `json:"last_verified"`
	Markup             ExactMoney                `json:"markup"`
	Components         []hostedSnapshotComponent `json:"components"`
	ExcludedComponents []string                  `json:"excluded_components"`
	ZeroDimensions     []string                  `json:"zero_dimensions"`
	MinimumCharge      *CatalogMinimumCharge     `json:"minimum_charge"`
	Bounds             []CatalogUsageBound       `json:"bounds"`
	Maximum            CatalogAuthorizedMaximum  `json:"maximum"`
}

type managedPriceSnapshotRecord struct {
	ID               string                      `gorm:"primaryKey"`
	RequestID        string                      `gorm:"not null;uniqueIndex"`
	Request          managedJournalRequestRecord `gorm:"foreignKey:RequestID;references:ID;constraint:OnDelete:RESTRICT"`
	BillingAccountID string                      `gorm:"not null;index"`
	BillingAccount   managedBillingAccountRecord `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	Document         []byte                      `gorm:"not null"`
	Digest           string                      `gorm:"not null"`
	CreatedAt        time.Time                   `gorm:"not null"`
}

func newHostedPriceAdmission(snapshot *CatalogRatingSnapshot, bounds []CatalogUsageBound, attempts uint32) (journalReservation, error) {
	maximum, err := snapshot.MaximumCharge(bounds, attempts)
	if err != nil {
		return nil, err
	}
	document := hostedPriceSnapshotDocument{
		CatalogRevision: snapshot.revision, Provider: snapshot.descriptor.Provider, Model: snapshot.descriptor.Model, Operation: snapshot.descriptor.Operation,
		Source: snapshot.descriptor.Source, LastVerified: snapshot.descriptor.LastVerified, Markup: ratingMoney(snapshot.markup),
		MinimumCharge: snapshot.descriptor.MinimumCharge, Bounds: slices.Clone(bounds), Maximum: maximum, Components: make([]hostedSnapshotComponent, 0, len(snapshot.components)),
		ExcludedComponents: append([]string{}, snapshot.excludedComponents...), ZeroDimensions: append([]string{}, snapshot.zeroDimensions...),
	}
	for index := range document.Bounds {
		document.Bounds[index].Maximum = normalizeJournalDecimal(document.Bounds[index].Maximum)
	}
	for _, component := range snapshot.components {
		retained := hostedSnapshotComponent{Dimension: component.binding.Dimension, AdditionalDimension: component.binding.AdditionalDimension, Rates: make([]hostedSnapshotRate, 0, len(component.rates))}
		for _, rate := range component.rates {
			retained.Rates = append(retained.Rates, hostedSnapshotRate{ProviderRate: rate, CustomerRate: ratingMoney(snapshot.customerAmount(ratingRational(string(rate.Rate))))})
		}
		document.Components = append(document.Components, retained)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode accepted price snapshot: %w", err)
	}
	digest := sha256Hex(string(encoded))
	return func(transaction *gorm.DB, request managedJournalRequestRecord) error {
		for _, component := range document.Components {
			for _, entry := range component.Rates {
				conditions := entry.ProviderRate.Conditions
				if conditions.EffectiveFrom == "" || conditions.InputTokens.UnresolvedReason != "" || !catalogPriceActiveAt(conditions, request.CreatedAt) {
					return fmt.Errorf("%w: accepted price interval is unavailable", ErrCatalogRatingUnavailable)
				}
			}
		}
		if !snapshot.acceptedAt.Equal(request.CreatedAt) {
			return fmt.Errorf("%w: price acceptance time differs", errUsageJournalConflict)
		}
		if request.CatalogRevision != document.CatalogRevision || request.Provider != document.Provider || request.Model != document.Model || request.Operation != document.Operation {
			return fmt.Errorf("%w: price snapshot route differs", errUsageJournalConflict)
		}
		var attemptCount int64
		if err := transaction.Model(&managedJournalAttemptRecord{}).Where("request_id = ?", request.ID).Count(&attemptCount).Error; err != nil {
			return fmt.Errorf("read accepted attempt count: %w", err)
		}
		if attemptCount > int64(document.Maximum.Attempts) {
			return fmt.Errorf("%w: accepted attempt limit exceeded", errUsageJournalConflict)
		}
		record := managedPriceSnapshotRecord{ID: priceSnapshotIDPrefix + sha256Hex(request.ID)[:32], RequestID: request.ID, BillingAccountID: request.BillingAccountID, Document: encoded, Digest: digest, CreatedAt: request.CreatedAt}
		result := transaction.Omit(clause.Associations).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "request_id"}}, DoNothing: true}).Create(&record)
		if result.Error != nil {
			return fmt.Errorf("retain price snapshot for request %s: %w", request.ID, result.Error)
		}
		if result.RowsAffected == 1 {
			return nil
		}
		var accepted managedPriceSnapshotRecord
		if err := transaction.Where("request_id = ?", request.ID).First(&accepted).Error; err != nil {
			return fmt.Errorf("read accepted price for request %s: %w", request.ID, err)
		}
		if accepted.Digest != digest || !bytes.Equal(accepted.Document, encoded) {
			return fmt.Errorf("%w: immutable price snapshot differs", errUsageJournalConflict)
		}
		return nil
	}, nil
}

func decodeRatingJSON(encoded []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode retained rating document: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("invalid trailing rating document")
	}
	return nil
}

// Historical prices are validated directly, without consulting a current catalog.
func restoreHostedPriceSnapshot(record managedPriceSnapshotRecord) (*CatalogRatingSnapshot, hostedPriceSnapshotDocument, error) {
	var document hostedPriceSnapshotDocument
	if record.Digest != sha256Hex(string(record.Document)) {
		return nil, document, fmt.Errorf("invalid retained price digest")
	}
	if err := decodeRatingJSON(record.Document, &document); err != nil {
		return nil, document, err
	}
	descriptor := CatalogPriceDescriptor{Provider: document.Provider, Model: document.Model, Operation: document.Operation, Available: true, Source: document.Source, LastVerified: document.LastVerified, MinimumCharge: document.MinimumCharge}
	for _, component := range document.Components {
		for _, entry := range component.Rates {
			descriptor.Rates = append(descriptor.Rates, entry.ProviderRate)
		}
	}
	if err := validateCatalogPriceValues(descriptor, "retained_price"); err != nil {
		return nil, document, err
	}
	if document.MinimumCharge != nil && document.MinimumCharge.Unit != "USD/request" {
		return nil, document, fmt.Errorf("invalid retained minimum charge unit")
	}
	if err := validateCatalogRevision(document.CatalogRevision); err != nil {
		return nil, document, err
	}
	markup, err := parseExactMoney(document.Markup)
	if err != nil || markup.Sign() <= 0 {
		return nil, document, fmt.Errorf("invalid retained price markup")
	}
	snapshot := &CatalogRatingSnapshot{revision: document.CatalogRevision, descriptor: descriptor, markup: markup, acceptedAt: record.CreatedAt, excludedComponents: slices.Clone(document.ExcludedComponents), zeroDimensions: slices.Clone(document.ZeroDimensions)}
	dimensions, components := map[string]bool{}, map[string]bool{}
	for _, component := range document.Components {
		if len(component.Rates) == 0 {
			return nil, document, fmt.Errorf("invalid retained price component")
		}
		first := component.Rates[0].ProviderRate
		binding := CatalogRateBinding{Dimension: component.Dimension, AdditionalDimension: component.AdditionalDimension, Component: first.Component, Conditions: categoricalPriceConditions(first.Conditions)}
		if err := claimRatingDimensions(binding, dimensions); err != nil {
			return nil, document, err
		}
		restored := catalogSnapshotComponent{binding: binding}
		for _, entry := range component.Rates {
			rate := entry.ProviderRate
			conditions := rate.Conditions
			if conditions.EffectiveFrom == "" || conditions.InputTokens.UnresolvedReason != "" || !catalogPriceActiveAt(conditions, record.CreatedAt) {
				return nil, document, fmt.Errorf("invalid retained effective price interval")
			}
			unit, known := catalogRatingUnits[rate.Unit]
			if !known || rate.Unit != first.Unit || rate.Component != first.Component || categoricalPriceConditions(conditions) != binding.Conditions {
				return nil, document, fmt.Errorf("invalid retained price schedule")
			}
			expected := ratingMoney(snapshot.customerAmount(ratingRational(string(rate.Rate))))
			if entry.CustomerRate != expected {
				return nil, document, fmt.Errorf("invalid retained customer rate")
			}
			restored.unit = unit
			restored.rates = append(restored.rates, rate)
		}
		snapshot.components = append(snapshot.components, restored)
		components[first.Component] = true
	}
	if err := validateRatingRules(document.ExcludedComponents, document.ZeroDimensions, dimensions, components); err != nil {
		return nil, document, err
	}
	maximum, err := snapshot.MaximumCharge(document.Bounds, document.Maximum.Attempts)
	if err != nil {
		return nil, document, fmt.Errorf("validate retained authorization maximum: %w", err)
	}
	if maximum != document.Maximum {
		return nil, document, fmt.Errorf("invalid retained authorization maximum")
	}
	return snapshot, document, nil
}
