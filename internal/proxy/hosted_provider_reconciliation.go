package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const providerCostSourceMaximumBytes = 16 << 20

type providerCostEvidenceInput struct {
	SourceReference          string     `json:"source_reference"`
	SourceSHA256             string     `json:"source_sha256"`
	Provider                 string     `json:"provider"`
	PlatformConnectionID     string     `json:"platform_connection_id"`
	CredentialVersion        uint64     `json:"credential_version"`
	ProviderAccountReference string     `json:"provider_account_reference"`
	PeriodStart              time.Time  `json:"period_start"`
	PeriodEnd                time.Time  `json:"period_end"`
	ReportedAt               time.Time  `json:"reported_at"`
	Model                    string     `json:"model,omitempty"`
	Operation                string     `json:"operation,omitempty"`
	Currency                 string     `json:"currency"`
	UsageAmount              ExactMoney `json:"usage_amount"`
	Discount                 ExactMoney `json:"discount"`
	Fees                     ExactMoney `json:"fees"`
	AttemptCount             *int64     `json:"attempt_count,omitempty"`
}

type managedProviderCostEvidenceRecord struct {
	ID        string    `gorm:"primaryKey"`
	Input     string    `gorm:"not null"`
	Source    []byte    `gorm:"not null"`
	Digest    string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
}

type managedProviderReconciliationRunRecord struct {
	ID            string                            `gorm:"primaryKey"`
	EvidenceID    string                            `gorm:"not null;index"`
	Evidence      managedProviderCostEvidenceRecord `gorm:"foreignKey:EvidenceID;references:ID;constraint:OnDelete:RESTRICT"`
	LocalEvidence string                            `gorm:"not null"`
	LocalDigest   string                            `gorm:"not null"`
	Report        string                            `gorm:"not null"`
	CreatedAt     time.Time                         `gorm:"not null"`
}

// ProviderCostReconciliationReport compares an operator-supplied source scope with local rated dispatches.
// KnownProviderCost is in USD; imported amounts retain their declared currency.
type ProviderCostReconciliationReport struct {
	RunID                    string                              `json:"run_id"`
	EvidenceID               string                              `json:"evidence_id"`
	SourceSHA256             string                              `json:"source_sha256"`
	LocalEvidenceDigest      string                              `json:"local_evidence_digest"`
	Provider                 string                              `json:"provider"`
	PlatformConnectionID     string                              `json:"platform_connection_id"`
	CredentialVersion        uint64                              `json:"credential_version"`
	ProviderAccountReference string                              `json:"provider_account_reference"`
	PeriodStart              time.Time                           `json:"period_start"`
	PeriodEnd                time.Time                           `json:"period_end"`
	ReportedAt               time.Time                           `json:"reported_at"`
	Model                    string                              `json:"model,omitempty"`
	Operation                string                              `json:"operation,omitempty"`
	AttemptCount             int64                               `json:"attempt_count"`
	IncompleteAttempts       int64                               `json:"incomplete_attempts"`
	KnownProviderCost        ExactMoney                          `json:"known_provider_cost_usd"`
	ImportedCurrency         string                              `json:"imported_currency"`
	ImportedUsageAmount      ExactMoney                          `json:"imported_usage_amount"`
	ImportedDiscount         ExactMoney                          `json:"imported_discount"`
	ImportedFees             ExactMoney                          `json:"imported_fees"`
	Differences              []FinancialReconciliationDifference `json:"differences"`
	CreatedAt                time.Time                           `json:"created_at"`
}

func decodeProviderCostEvidence(normalized, source io.Reader) (providerCostEvidenceInput, managedProviderCostEvidenceRecord, error) {
	var input providerCostEvidenceInput
	if normalized == nil || source == nil {
		return input, managedProviderCostEvidenceRecord{}, fmt.Errorf("provider evidence and source readers are required")
	}
	raw, err := io.ReadAll(io.LimitReader(normalized, paymentWebhookMaximumBytes+1))
	if err != nil || len(raw) > paymentWebhookMaximumBytes {
		return input, managedProviderCostEvidenceRecord{}, fmt.Errorf("read normalized provider evidence: invalid or excessive input")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, managedProviderCostEvidenceRecord{}, fmt.Errorf("decode normalized provider evidence: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return input, managedProviderCostEvidenceRecord{}, fmt.Errorf("normalized provider evidence requires one JSON object")
	}
	if !validIdempotencyKey(input.SourceReference) || !validIdempotencyKey(input.ProviderAccountReference) || !validIdempotencyKey(input.PlatformConnectionID) || input.Provider == "" || input.CredentialVersion == 0 || input.PeriodStart.IsZero() || !input.PeriodEnd.After(input.PeriodStart) || input.ReportedAt.IsZero() || !paymentCurrencyPattern.MatchString(input.Currency) || (input.AttemptCount != nil && *input.AttemptCount < 0) {
		return input, managedProviderCostEvidenceRecord{}, fmt.Errorf("invalid provider evidence scope")
	}
	for _, value := range []string{input.Model, input.Operation} {
		if len(value) > 256 || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
			return input, managedProviderCostEvidenceRecord{}, fmt.Errorf("invalid provider evidence route")
		}
	}
	for _, field := range []*ExactMoney{&input.UsageAmount, &input.Discount, &input.Fees} {
		amount, err := parseExactMoney(*field)
		if err != nil {
			return input, managedProviderCostEvidenceRecord{}, err
		}
		*field = ratingMoney(amount)
	}
	input.PeriodStart, input.PeriodEnd, input.ReportedAt = input.PeriodStart.UTC(), input.PeriodEnd.UTC(), input.ReportedAt.UTC()
	sourceBytes, err := io.ReadAll(io.LimitReader(source, providerCostSourceMaximumBytes+1))
	if err != nil || len(sourceBytes) == 0 || len(sourceBytes) > providerCostSourceMaximumBytes {
		return input, managedProviderCostEvidenceRecord{}, fmt.Errorf("read provider source: empty, unreadable, or excessive input")
	}
	if input.SourceSHA256 != sha256Hex(string(sourceBytes)) {
		return input, managedProviderCostEvidenceRecord{}, fmt.Errorf("provider source digest mismatch")
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return input, managedProviderCostEvidenceRecord{}, err
	}
	identity := []string{input.SourceReference, input.Provider, input.ProviderAccountReference, input.PlatformConnectionID, strconv.FormatUint(input.CredentialVersion, 10), input.PeriodStart.Format(time.RFC3339Nano), input.PeriodEnd.Format(time.RFC3339Nano), input.Model, input.Operation}
	record := managedProviderCostEvidenceRecord{ID: "provider-evidence-" + sha256Hex(strings.Join(identity, "\x00")), Input: string(encoded), Source: sourceBytes, Digest: sha256Hex(string(encoded) + "\x00" + string(sourceBytes)), CreatedAt: time.Now().UTC()}
	return input, record, nil
}

// ReconcileProviderCosts imports retained source bytes and compares one exact invoice or usage-export scope.
// It never changes ratings, usage observations, customer charges, or Ledger entries.
func ReconcileProviderCosts(ctx context.Context, configuration ManagementConfiguration, runID string, normalized, source io.Reader) (ProviderCostReconciliationReport, error) {
	var report ProviderCostReconciliationReport
	if !validIdempotencyKey(runID) {
		return report, fmt.Errorf("invalid provider reconciliation run ID")
	}
	input, evidence, err := decodeProviderCostEvidence(normalized, source)
	if err != nil {
		return report, err
	}
	if configuration.DatabaseDialector == nil && configuration.DatabasePath == "" {
		return report, fmt.Errorf("provider reconciliation requires the management database")
	}
	database, err := gorm.Open(managementDatabaseDialector(configuration), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return report, fmt.Errorf("open provider reconciliation database: %w", err)
	}
	connection, err := database.DB()
	if err != nil {
		return report, err
	}
	defer connection.Close()
	for _, model := range []any{&managedProviderCostEvidenceRecord{}, &managedProviderReconciliationRunRecord{}} {
		if err := validateHostedTable(database, model); err != nil {
			return report, err
		}
	}
	err = database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lock := tx.Model(&managedPlatformConnectionRecord{}).Where("id = ?", input.PlatformConnectionID).UpdateColumn("id", gorm.Expr("id"))
		if lock.Error != nil {
			return fmt.Errorf("lock provider reconciliation scope: %w", lock.Error)
		}
		if lock.RowsAffected != 1 {
			return fmt.Errorf("provider evidence platform connection is absent")
		}
		var credential managedPlatformCredentialRecord
		if err := tx.Select("connection_id", "version").Where("connection_id = ? AND version = ?", input.PlatformConnectionID, input.CredentialVersion).First(&credential).Error; err != nil {
			return fmt.Errorf("provider evidence credential binding: %w", err)
		}
		var platform managedPlatformConnectionRecord
		if err := tx.First(&platform, "id = ?", input.PlatformConnectionID).Error; err != nil {
			return err
		}
		if platform.Provider != input.Provider {
			return fmt.Errorf("provider evidence does not match platform connection")
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&evidence).Error; err != nil {
			return fmt.Errorf("retain provider cost source: %w", err)
		}
		var retained managedProviderCostEvidenceRecord
		if err := tx.First(&retained, "id = ?", evidence.ID).Error; err != nil {
			return err
		}
		if retained.Digest != evidence.Digest || sha256Hex(retained.Input+"\x00"+string(retained.Source)) != retained.Digest {
			return fmt.Errorf("provider cost source identity conflict")
		}
		var run managedProviderReconciliationRunRecord
		err := tx.First(&run, "id = ?", runID).Error
		if err == nil {
			if run.EvidenceID != evidence.ID {
				return fmt.Errorf("provider reconciliation run identity conflict")
			}
			if sha256Hex(run.LocalEvidence) != run.LocalDigest {
				return fmt.Errorf("provider reconciliation local evidence changed")
			}
			return json.Unmarshal([]byte(run.Report), &report)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var local string
		report, local, err = compareProviderCosts(tx, runID, input, evidence)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(report)
		if err != nil {
			return err
		}
		run = managedProviderReconciliationRunRecord{ID: runID, EvidenceID: evidence.ID, LocalEvidence: local, LocalDigest: report.LocalEvidenceDigest, Report: string(encoded), CreatedAt: report.CreatedAt}
		if err := tx.Omit(clause.Associations).Create(&run).Error; err != nil {
			return fmt.Errorf("retain provider reconciliation report: %w", err)
		}
		return nil
	})
	return report, err
}

type providerCostLocalEvidence struct {
	Attempt managedJournalAttemptRecord `json:"attempt"`
	Request managedJournalRequestRecord `json:"request"`
	Charge  *managedChargeRecord        `json:"charge"`
}

func compareProviderCosts(tx *gorm.DB, runID string, input providerCostEvidenceInput, source managedProviderCostEvidenceRecord) (ProviderCostReconciliationReport, string, error) {
	report := ProviderCostReconciliationReport{RunID: runID, EvidenceID: source.ID, SourceSHA256: input.SourceSHA256, Provider: input.Provider, PlatformConnectionID: input.PlatformConnectionID, CredentialVersion: input.CredentialVersion, ProviderAccountReference: input.ProviderAccountReference, PeriodStart: input.PeriodStart, PeriodEnd: input.PeriodEnd, ReportedAt: input.ReportedAt, Model: input.Model, Operation: input.Operation, ImportedCurrency: input.Currency, ImportedUsageAmount: input.UsageAmount, ImportedDiscount: input.Discount, ImportedFees: input.Fees, Differences: []FinancialReconciliationDifference{}, CreatedAt: time.Now().UTC()}
	query := tx.Table("managed_journal_attempt_records AS attempts").Select("attempts.*").Joins("JOIN managed_journal_request_records AS requests ON requests.id = attempts.request_id").Where("requests.platform_connection_id = ? AND requests.credential_version = ? AND requests.provider = ? AND attempts.dispatch_at >= ? AND attempts.dispatch_at < ?", input.PlatformConnectionID, input.CredentialVersion, input.Provider, input.PeriodStart, input.PeriodEnd)
	if input.Model != "" {
		query = query.Where("requests.model = ?", input.Model)
	}
	if input.Operation != "" {
		query = query.Where("requests.operation = ?", input.Operation)
	}
	var attempts []managedJournalAttemptRecord
	if err := query.Order("attempts.id").Find(&attempts).Error; err != nil {
		return report, "", fmt.Errorf("read provider reconciliation attempts: %w", err)
	}
	local := make([]providerCostLocalEvidence, 0, len(attempts))
	total := new(big.Rat)
	seen := map[string]bool{}
	difference := func(category, code, expected, observed string) {
		report.Differences = append(report.Differences, FinancialReconciliationDifference{category, code, expected, observed})
	}
	for _, attempt := range attempts {
		item := providerCostLocalEvidence{Attempt: attempt}
		if err := tx.First(&item.Request, "id = ?", attempt.RequestID).Error; err != nil {
			return report, "", err
		}
		report.AttemptCount++
		if attempt.ProviderRequestID != "" {
			if seen[attempt.ProviderRequestID] {
				difference("duplicate_effect", "provider_request_reused", "one dispatched attempt", attempt.ID)
			}
			seen[attempt.ProviderRequestID] = true
		}
		var charge managedChargeRecord
		found, err := optionalPaymentRecord(tx, &charge, "attempt_id = ?", attempt.ID)
		if err != nil {
			return report, "", err
		}
		if found {
			item.Charge = &charge
			response, err := chargeResponse(charge, nil)
			if err != nil {
				return report, "", err
			}
			if response.Rating.ProviderCost != nil {
				amount, err := parseExactMoney(*response.Rating.ProviderCost)
				if err != nil {
					return report, "", err
				}
				total.Add(total, amount)
			} else {
				report.IncompleteAttempts++
			}
		} else {
			report.IncompleteAttempts++
		}
		local = append(local, item)
	}
	report.KnownProviderCost = ratingMoney(total)
	if report.IncompleteAttempts > 0 {
		difference("missing_usage", "unrated_provider_attempts", "0", strconv.FormatInt(report.IncompleteAttempts, 10))
	}
	if input.ReportedAt.Before(input.PeriodEnd) {
		difference("timing", "source_period_incomplete", input.PeriodEnd.Format(time.RFC3339Nano), input.ReportedAt.Format(time.RFC3339Nano))
	}
	if input.AttemptCount != nil && *input.AttemptCount != report.AttemptCount {
		difference("missing_usage", "attempt_count", strconv.FormatInt(*input.AttemptCount, 10), strconv.FormatInt(report.AttemptCount, 10))
	}
	if input.Currency != CatalogCurrencyUSD {
		difference("currency", "provider_invoice_currency", CatalogCurrencyUSD, input.Currency)
	} else if report.IncompleteAttempts == 0 {
		expected, _ := parseExactMoney(input.UsageAmount)
		if total.Cmp(expected) != 0 {
			difference("amount", "provider_usage_cost", expected.RatString(), total.RatString())
		}
	}
	discount, _ := parseExactMoney(input.Discount)
	if discount.Sign() != 0 {
		difference("discount", "provider_discount", "separate from accepted usage rates", discount.RatString())
	}
	fees, _ := parseExactMoney(input.Fees)
	if fees.Sign() != 0 {
		difference("fee", "provider_fees", "separate from accepted usage rates", fees.RatString())
	}
	encoded, err := json.Marshal(local)
	if err != nil {
		return report, "", err
	}
	report.LocalEvidenceDigest = sha256Hex(string(encoded))
	return report, string(encoded), nil
}
