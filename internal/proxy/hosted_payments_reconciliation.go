package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/tyemirov/utils/billing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const (
	paymentReconciliationPending   = "pending"
	paymentReconciliationCompleted = "completed"
)

type managedPaymentReconciliationRunRecord struct {
	ID                 string    `gorm:"primaryKey"`
	Environment        string    `gorm:"not null"`
	ProcessorAccountID string    `gorm:"not null"`
	SupplierID         string    `gorm:"not null"`
	State              string    `gorm:"not null;check:state IN ('pending','completed')"`
	TotalOrders        int64     `gorm:"not null"`
	CompletedOrders    int64     `gorm:"not null"`
	CreatedAt          time.Time `gorm:"not null"`
	CompletedAt        *time.Time
}

type managedPaymentReconciliationItemRecord struct {
	RunID          string                                `gorm:"primaryKey"`
	Run            managedPaymentReconciliationRunRecord `gorm:"foreignKey:RunID;references:ID;constraint:OnDelete:RESTRICT"`
	OrderID        string                                `gorm:"primaryKey"`
	Order          managedFundingOrderRecord             `gorm:"foreignKey:OrderID;references:ID;constraint:OnDelete:RESTRICT"`
	State          string                                `gorm:"not null;check:state IN ('pending','completed')"`
	Result         string                                `gorm:"not null"`
	Evidence       string                                `gorm:"not null"`
	EvidenceDigest string                                `gorm:"not null"`
	ObservedAt     *time.Time
}

// PaymentReconciliationItem exposes a private operator report without raw processor data.
type PaymentReconciliationItem struct {
	OrderID          string                              `json:"order_id"`
	BillingAccountID string                              `json:"billing_account_id"`
	ObservedAt       time.Time                           `json:"observed_at"`
	EvidenceDigest   string                              `json:"evidence_digest"`
	Differences      []FinancialReconciliationDifference `json:"differences"`
}

// PaymentReconciliationReport is an immutable completed run or its durable checkpoint.
type PaymentReconciliationReport struct {
	RunID              string                      `json:"run_id"`
	Environment        string                      `json:"environment"`
	ProcessorAccountID string                      `json:"processor_account_id"`
	SupplierID         string                      `json:"supplier_id"`
	State              string                      `json:"state"`
	TotalOrders        int64                       `json:"total_orders"`
	CompletedOrders    int64                       `json:"completed_orders"`
	CreatedAt          time.Time                   `json:"created_at"`
	CompletedAt        *time.Time                  `json:"completed_at"`
	Items              []PaymentReconciliationItem `json:"items"`
}

// ReconcilePayments records processor comparisons without creating monetary effects.
// Reusing a run ID resumes its fixed order set or returns its completed report.
func ReconcilePayments(ctx context.Context, management ManagementConfiguration, configuration *PaymentConfiguration, runID string) (PaymentReconciliationReport, error) {
	if !validIdempotencyKey(runID) {
		return PaymentReconciliationReport{}, fmt.Errorf("reconcile payments: invalid run ID")
	}
	settings, err := newPaymentSettings(configuration)
	if err != nil {
		return PaymentReconciliationReport{}, err
	}
	if settings == nil {
		return PaymentReconciliationReport{}, fmt.Errorf("reconcile payments: payments configuration is required")
	}
	if management.DatabaseDialector == nil && management.DatabasePath == "" {
		return PaymentReconciliationReport{}, fmt.Errorf("reconcile payments: management database is required")
	}
	database, err := gorm.Open(managementDatabaseDialector(management), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return PaymentReconciliationReport{}, fmt.Errorf("open payment reconciliation database: %w", err)
	}
	connection, err := database.DB()
	if err != nil {
		return PaymentReconciliationReport{}, err
	}
	defer connection.Close()
	// Reconciliation only consumes the current initialized service schema.
	for _, model := range []any{&managedPaymentReconciliationRunRecord{}, &managedPaymentReconciliationItemRecord{}, &managedPaymentEnvironmentRecord{}} {
		if err := validateHostedTable(database, model); err != nil {
			return PaymentReconciliationReport{}, fmt.Errorf("validate payment reconciliation schema: %w", err)
		}
	}
	var binding managedPaymentEnvironmentRecord
	if err := database.WithContext(ctx).First(&binding, 1).Error; err != nil {
		return PaymentReconciliationReport{}, fmt.Errorf("read payment environment: %w", err)
	}
	if binding.Environment != settings.catalog.environment {
		return PaymentReconciliationReport{}, fmt.Errorf("payment environment differs from the database binding")
	}
	client, err := billing.NewPaddleCommerceClient(settings.catalog.environment, settings.apiKey, settings.apiBaseURL, &http.Client{Timeout: paymentCheckoutRetry})
	if err != nil {
		return PaymentReconciliationReport{}, fmt.Errorf("configure payment reconciliation client: %w", err)
	}
	auditor := paymentReconciler{database: database, catalog: settings.catalog, client: client, now: time.Now}
	if err := auditor.prepare(ctx, runID); err != nil {
		return PaymentReconciliationReport{}, err
	}
	if err := auditor.resume(ctx, runID); err != nil {
		return PaymentReconciliationReport{}, err
	}
	return auditor.report(ctx, runID)
}

type paymentReconciler struct {
	database *gorm.DB
	catalog  *fundingCatalog
	client   paddleTransactionReader
	now      func() time.Time
}

func (auditor paymentReconciler) prepare(ctx context.Context, runID string) error {
	return auditor.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		run := managedPaymentReconciliationRunRecord{ID: runID, Environment: auditor.catalog.environment, ProcessorAccountID: auditor.catalog.processorAccountID, SupplierID: auditor.catalog.supplierID, State: paymentReconciliationPending, CreatedAt: auditor.now().UTC()}
		inserted := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&run)
		if inserted.Error != nil {
			return fmt.Errorf("retain payment reconciliation run: %w", inserted.Error)
		}
		if inserted.RowsAffected == 0 {
			if err := tx.First(&run, "id = ?", runID).Error; err != nil {
				return err
			}
			if run.Environment != auditor.catalog.environment || run.ProcessorAccountID != auditor.catalog.processorAccountID || run.SupplierID != auditor.catalog.supplierID {
				return fmt.Errorf("payment reconciliation run identity conflict")
			}
			return nil
		}
		result := tx.Exec("INSERT INTO managed_payment_reconciliation_item_records (run_id, order_id, state, result, evidence, evidence_digest) SELECT ?, id, ?, '', '', '' FROM managed_funding_order_records WHERE environment = ? AND processor_account_id = ? AND supplier_id = ?", run.ID, paymentReconciliationPending, run.Environment, run.ProcessorAccountID, run.SupplierID)
		if result.Error != nil {
			return fmt.Errorf("retain reconciliation order set: %w", result.Error)
		}
		return tx.Model(&managedPaymentReconciliationRunRecord{}).Where("id = ?", run.ID).Update("total_orders", result.RowsAffected).Error
	})
}

func (auditor paymentReconciler) resume(ctx context.Context, runID string) error {
	for {
		var item managedPaymentReconciliationItemRecord
		err := auditor.database.WithContext(ctx).Where("run_id = ? AND state = ?", runID, paymentReconciliationPending).Order("order_id").First(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			break
		}
		if err != nil {
			return fmt.Errorf("read reconciliation checkpoint: %w", err)
		}
		var order managedFundingOrderRecord
		if err := auditor.database.WithContext(ctx).First(&order, "id = ?", item.OrderID).Error; err != nil {
			return err
		}
		var checkout managedPaymentCheckoutRecord
		err = auditor.database.WithContext(ctx).First(&checkout, "order_id = ?", order.ID).Error
		var transaction *billing.PaddleTransactionCompletedWebhookData
		var adjustments []billing.PaddleAdjustment
		if err == nil {
			attempt, cancel := context.WithTimeout(ctx, paymentCheckoutRetry)
			current, readError := auditor.client.GetTransaction(attempt, checkout.TransactionID)
			if readError == nil && current.Status == paymentTransactionStatusCompleted {
				adjustments, readError = auditor.client.ListTransactionAdjustments(attempt, checkout.TransactionID)
			}
			cancel()
			if readError != nil {
				return fmt.Errorf("read processor evidence for order %s: processor unavailable; retry run %s", order.ID, runID)
			}
			transaction = &current
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read reconciliation checkout: %w", err)
		}
		if err := auditor.record(ctx, item, order, checkout, transaction, adjustments); err != nil {
			return err
		}
	}
	now := auditor.now().UTC()
	return auditor.database.WithContext(ctx).Model(&managedPaymentReconciliationRunRecord{}).Where("id = ? AND state = ? AND completed_orders = total_orders", runID, paymentReconciliationPending).Updates(map[string]any{"state": paymentReconciliationCompleted, "completed_at": now}).Error
}

func (auditor paymentReconciler) record(ctx context.Context, item managedPaymentReconciliationItemRecord, order managedFundingOrderRecord, checkout managedPaymentCheckoutRecord, transaction *billing.PaddleTransactionCompletedWebhookData, adjustments []billing.PaddleAdjustment) error {
	return auditor.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Both audit workers and payment workers use this account writer boundary.
		lock := tx.Model(&managedBillingAccountRecord{}).Where("id = ?", order.BillingAccountID).UpdateColumn("id", gorm.Expr("id"))
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return errFundingNotFound
		}
		if err := tx.First(&item, "run_id = ? AND order_id = ?", item.RunID, order.ID).Error; err != nil {
			return err
		}
		if item.State == paymentReconciliationCompleted {
			return nil
		}
		// Re-read mutable local state after the processor read and account lock.
		if err := tx.First(&order, "id = ?", order.ID).Error; err != nil {
			return err
		}
		report, evidence, err := comparePaymentEvidence(tx, order, checkout, transaction, adjustments, auditor.now().UTC())
		if err != nil {
			return err
		}
		report.EvidenceDigest = sha256Hex(evidence)
		encoded, err := json.Marshal(report)
		if err != nil {
			return err
		}
		if err := tx.Model(&managedPaymentReconciliationItemRecord{}).Where("run_id = ? AND order_id = ?", item.RunID, item.OrderID).Updates(map[string]any{"state": paymentReconciliationCompleted, "result": string(encoded), "evidence": evidence, "evidence_digest": report.EvidenceDigest, "observed_at": report.ObservedAt}).Error; err != nil {
			return fmt.Errorf("retain reconciliation item: %w", err)
		}
		if err := tx.Model(&managedPaymentReconciliationRunRecord{}).Where("id = ? AND state = ?", item.RunID, paymentReconciliationPending).Update("completed_orders", gorm.Expr("completed_orders + 1")).Error; err != nil {
			return fmt.Errorf("retain reconciliation checkpoint: %w", err)
		}
		return nil
	})
}

func (auditor paymentReconciler) report(ctx context.Context, runID string) (PaymentReconciliationReport, error) {
	var run managedPaymentReconciliationRunRecord
	if err := auditor.database.WithContext(ctx).First(&run, "id = ?", runID).Error; err != nil {
		return PaymentReconciliationReport{}, err
	}
	report := PaymentReconciliationReport{RunID: run.ID, Environment: run.Environment, ProcessorAccountID: run.ProcessorAccountID, SupplierID: run.SupplierID, State: run.State, TotalOrders: run.TotalOrders, CompletedOrders: run.CompletedOrders, CreatedAt: run.CreatedAt, CompletedAt: run.CompletedAt, Items: []PaymentReconciliationItem{}}
	var items []managedPaymentReconciliationItemRecord
	if err := auditor.database.WithContext(ctx).Where("run_id = ? AND state = ?", runID, paymentReconciliationCompleted).Order("order_id").Find(&items).Error; err != nil {
		return report, err
	}
	for _, item := range items {
		var result PaymentReconciliationItem
		if err := json.Unmarshal([]byte(item.Result), &result); err != nil {
			return report, fmt.Errorf("decode retained reconciliation item: %w", err)
		}
		report.Items = append(report.Items, result)
	}
	return report, nil
}
