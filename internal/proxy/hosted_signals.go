package proxy

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"net/url"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// HostedFinancialQueueSignal describes retained work without choosing an alert threshold.
type HostedFinancialQueueSignal struct {
	Count    int64      `json:"count"`
	OldestAt *time.Time `json:"oldest_at"`
}

// HostedFinancialSignals is a read-only snapshot of the existing financial records.
// Monetary totals use exact strings; no account or provider credentials are included.
type HostedFinancialSignals struct {
	ObservedAt                 time.Time                  `json:"observed_at"`
	Currency                   string                     `json:"currency"`
	UnresolvedAttempts         HostedFinancialQueueSignal `json:"unresolved_attempts"`
	ActiveAttempts             HostedFinancialQueueSignal `json:"active_attempts"`
	PendingUsageDeliveries     HostedFinancialQueueSignal `json:"pending_usage_deliveries"`
	UnsettledCompletedRequests HostedFinancialQueueSignal `json:"unsettled_completed_requests"`
	OpenReconciliationCases    HostedFinancialQueueSignal `json:"open_reconciliation_cases"`
	PendingPaymentComparisons  HostedFinancialQueueSignal `json:"pending_payment_comparisons"`
	LatestPaymentComparisons   HostedFinancialQueueSignal `json:"latest_payment_comparisons"`
	LatestProviderComparisons  HostedFinancialQueueSignal `json:"latest_provider_comparisons"`
	PaymentDifferences         map[string]int64           `json:"payment_differences"`
	ProviderDifferences        map[string]int64           `json:"provider_differences"`
	PostedCents                string                     `json:"posted_cents"`
	ReservedCents              string                     `json:"reserved_cents"`
	ReconciliationHeldCents    string                     `json:"reconciliation_held_cents"`
	UnsettledFraction          ExactMoney                 `json:"unsettled_fraction_usd"`
	KnownPlatformExposure      ExactMoney                 `json:"known_platform_exposure_usd"`
	IncompleteProviderCosts    int64                      `json:"incomplete_provider_costs"`
}

// ReadHostedFinancialSignals reads one consistent SQLite snapshot without migration,
// external calls, account creation, or changes to financial records.
func ReadHostedFinancialSignals(ctx context.Context, databasePath string) (HostedFinancialSignals, error) {
	if databasePath == "" {
		return HostedFinancialSignals{}, fmt.Errorf("financial signals require the managed database path")
	}
	absolute, err := filepath.Abs(databasePath)
	if err != nil {
		return HostedFinancialSignals{}, fmt.Errorf("resolve financial database path: %w", err)
	}
	location := url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=ro"}
	database, err := gorm.Open(sqlite.Open(location.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return HostedFinancialSignals{}, fmt.Errorf("open financial signals database: %w", err)
	}
	connection, err := database.DB()
	if err != nil {
		return HostedFinancialSignals{}, err
	}
	defer connection.Close()
	report := HostedFinancialSignals{ObservedAt: time.Now().UTC(), Currency: CatalogCurrencyUSD, PaymentDifferences: map[string]int64{}, ProviderDifferences: map[string]int64{}}
	err = database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, query := range []struct {
			target    *HostedFinancialQueueSignal
			model     any
			where     string
			arguments []any
			timestamp string
		}{
			{&report.UnresolvedAttempts, &managedJournalAttemptRecord{}, "state = ?", []any{journalAttemptUncertain}, "created_at"},
			{&report.ActiveAttempts, &managedJournalAttemptRecord{}, "state IN ?", []any{[]journalAttemptState{journalAttemptPrepared, journalAttemptDispatched}}, "created_at"},
			{&report.PendingUsageDeliveries, &managedJournalDeliveryRecord{}, "delivered_at IS NULL", nil, "created_at"},
			{&report.UnsettledCompletedRequests, &managedJournalRequestRecord{}, "state = ? AND id IN (SELECT request_id FROM managed_funds_reservation_records WHERE state = ?)", []any{journalRequestCompleted, fundsReservationHeld}, "updated_at"},
			{&report.OpenReconciliationCases, &managedJournalCaseRecord{}, "resolved_at IS NULL", nil, "created_at"},
			{&report.PendingPaymentComparisons, &managedPaymentReconciliationRunRecord{}, "state = ?", []any{paymentReconciliationPending}, "created_at"},
		} {
			if err := readFinancialQueueSignal(tx.Model(query.model).Where(query.where, query.arguments...), query.target, query.timestamp); err != nil {
				return err
			}
		}
		if err := readFinancialDifferenceSignals(tx, &report); err != nil {
			return err
		}
		return readFinancialExposureSignals(tx, &report)
	}, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return HostedFinancialSignals{}, fmt.Errorf("read financial signals: %w", err)
	}
	return report, nil
}

func readFinancialQueueSignal(query *gorm.DB, target *HostedFinancialQueueSignal, timestamp string) error {
	if err := query.Count(&target.Count).Error; err != nil {
		return fmt.Errorf("count financial work: %w", err)
	}
	if target.Count == 0 {
		return nil
	}
	var oldest struct{ CreatedAt time.Time }
	if err := query.Select(timestamp + " AS created_at").Order(timestamp + " ASC").Take(&oldest).Error; err != nil {
		return fmt.Errorf("read oldest financial work: %w", err)
	}
	target.OldestAt = &oldest.CreatedAt
	return nil
}

func readFinancialExposureSignals(tx *gorm.DB, report *HostedFinancialSignals) error {
	var accounts []managedBillingAccountRecord
	if err := tx.Select("id").Find(&accounts).Error; err != nil {
		return err
	}
	posted, reserved, pending := new(big.Int), new(big.Int), new(big.Int)
	remainder, excess := new(big.Rat), new(big.Rat)
	reader := &gormManagedTenantDatabase{database: tx}
	for _, account := range accounts {
		balance, err := reader.billingFundsBalance(tx.Statement.Context, account.ID, report.ObservedAt)
		if err != nil {
			return err
		}
		posted.Add(posted, big.NewInt(balance.postedCents))
		reserved.Add(reserved, big.NewInt(balance.reservedCents))
		pending.Add(pending, big.NewInt(balance.pendingCents))
		remainder.Add(remainder, balance.unsettledFraction)
	}
	var exposures []managedFundsExposureRecord
	if err := tx.Find(&exposures).Error; err != nil {
		return err
	}
	for _, exposure := range exposures {
		value, err := parseExactMoney(ExactMoney{Numerator: exposure.ExcessNumerator, Denominator: exposure.ExcessDenominator})
		if err != nil {
			return fmt.Errorf("read retained exposure: %w", err)
		}
		excess.Add(excess, value)
		if !exposure.ProviderCostComplete {
			report.IncompleteProviderCosts++
		}
	}
	report.PostedCents, report.ReservedCents, report.ReconciliationHeldCents = posted.String(), reserved.String(), pending.String()
	report.UnsettledFraction, report.KnownPlatformExposure = ratingMoney(remainder), ratingMoney(excess)
	return nil
}
