package proxy

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

func readFinancialDifferenceSignals(tx *gorm.DB, report *HostedFinancialSignals) error {
	var items []managedPaymentReconciliationItemRecord
	if err := tx.Where("state = ?", paymentReconciliationCompleted).Order("observed_at DESC, run_id DESC").Find(&items).Error; err != nil {
		return err
	}
	seenOrders := map[string]bool{}
	for _, item := range items {
		if seenOrders[item.OrderID] {
			continue
		}
		seenOrders[item.OrderID] = true
		var result PaymentReconciliationItem
		if err := decodeRatingJSON([]byte(item.Result), &result); err != nil {
			return fmt.Errorf("read payment comparison %s: %w", item.RunID, err)
		}
		if item.ObservedAt == nil || result.ObservedAt.IsZero() || !result.ObservedAt.Equal(*item.ObservedAt) || result.OrderID != item.OrderID || result.EvidenceDigest != item.EvidenceDigest || sha256Hex(item.Evidence) != item.EvidenceDigest {
			return fmt.Errorf("payment comparison %s has inconsistent evidence", item.RunID)
		}
		observeFinancialComparison(&report.LatestPaymentComparisons, result.ObservedAt)
		for _, difference := range result.Differences {
			report.PaymentDifferences[difference.Code]++
		}
	}
	var runs []managedProviderReconciliationRunRecord
	if err := tx.Order("created_at DESC, id DESC").Find(&runs).Error; err != nil {
		return err
	}
	type scope struct {
		provider, connection, account, model, operation, currency, start, end string
		version                                                               uint64
	}
	seenScopes := map[scope]bool{}
	for _, run := range runs {
		var result ProviderCostReconciliationReport
		if err := decodeRatingJSON([]byte(run.Report), &result); err != nil {
			return fmt.Errorf("read provider comparison %s: %w", run.ID, err)
		}
		if result.RunID != run.ID || result.CreatedAt.IsZero() || !result.CreatedAt.Equal(run.CreatedAt) || result.LocalEvidenceDigest != run.LocalDigest || sha256Hex(run.LocalEvidence) != run.LocalDigest {
			return fmt.Errorf("provider comparison %s has inconsistent evidence", run.ID)
		}
		key := scope{result.Provider, result.PlatformConnectionID, result.ProviderAccountReference, result.Model, result.Operation, result.ImportedCurrency, result.PeriodStart.String(), result.PeriodEnd.String(), result.CredentialVersion}
		if seenScopes[key] {
			continue
		}
		seenScopes[key] = true
		observeFinancialComparison(&report.LatestProviderComparisons, result.CreatedAt)
		for _, difference := range result.Differences {
			report.ProviderDifferences[difference.Code]++
		}
	}
	return nil
}

func observeFinancialComparison(signal *HostedFinancialQueueSignal, observedAt time.Time) {
	signal.Count++
	if signal.OldestAt == nil || observedAt.Before(*signal.OldestAt) {
		signal.OldestAt = &observedAt
	}
}
