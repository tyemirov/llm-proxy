package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/tyemirov/utils/billing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	paymentInboxApplied               = "applied"
	paymentInboxReconciliation        = "reconciliation_required"
	paymentTransactionCompleted       = "transaction.completed"
	paymentTransactionStatusCompleted = "completed"
	fundingOrderPaid                  = "paid"
)

var paymentMinorAmountPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
var paymentCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

type managedPaymentReceiptRecord struct {
	OrderID             string                    `gorm:"primaryKey"`
	Order               managedFundingOrderRecord `gorm:"foreignKey:OrderID;references:ID;constraint:OnDelete:RESTRICT"`
	BillingAccountID    string                    `gorm:"not null;index"`
	Environment         string                    `gorm:"not null;uniqueIndex:payment_receipt_transaction,priority:1"`
	ProcessorAccountID  string                    `gorm:"not null;uniqueIndex:payment_receipt_transaction,priority:2"`
	TransactionID       string                    `gorm:"not null;uniqueIndex:payment_receipt_transaction,priority:3"`
	CustomerID          string                    `gorm:"not null"`
	InboxID             string                    `gorm:"not null"`
	Inbox               managedPaymentInboxRecord `gorm:"foreignKey:InboxID;references:ID;constraint:OnDelete:RESTRICT"`
	LedgerKey           string                    `gorm:"not null;uniqueIndex"`
	Currency            string                    `gorm:"not null"`
	CreditCents         int64                     `gorm:"not null;check:credit_cents >= 500"`
	GrossCents          string                    `gorm:"not null"`
	TaxCents            string                    `gorm:"not null"`
	FeeCents            *string
	EarningsCents       *string
	PayoutCurrency      string `gorm:"not null"`
	PayoutEarningsCents *string
	InvoiceNumber       *string
	FinancialEvidence   string    `gorm:"not null"`
	EvidenceDigest      string    `gorm:"not null"`
	CompletedAt         time.Time `gorm:"not null"`
	CreatedAt           time.Time `gorm:"not null"`
}

type paddleTransactionReader interface {
	GetTransaction(context.Context, string) (billing.PaddleTransactionCompletedWebhookData, error)
}

type paddlePaymentProcessor struct {
	database *gormManagedTenantDatabase
	catalog  *fundingCatalog
	client   paddleTransactionReader
	now      func() time.Time
}

func newPaddlePaymentProcessor(database *gormManagedTenantDatabase, catalog *fundingCatalog, client paddleTransactionReader) (*paddlePaymentProcessor, error) {
	if database == nil || catalog == nil || client == nil {
		return nil, fmt.Errorf("configure Paddle payment processor: missing dependency")
	}
	return &paddlePaymentProcessor{database: database, catalog: catalog, client: client, now: time.Now}, nil
}

func (worker *paddlePaymentProcessor) reconcile(ctx context.Context) error {
	var events []managedPaymentInboxRecord
	query := worker.database.database.WithContext(ctx).Where("environment = ? AND processor_account_id = ? AND state IN ? AND retry_at <= ?", worker.catalog.environment, worker.catalog.processorAccountID, []string{paymentInboxPending, paymentInboxReconciliation}, worker.now().UTC())
	if err := query.Order("retry_at, received_at, id").Limit(100).Find(&events).Error; err != nil {
		return fmt.Errorf("find payment events: %w", err)
	}
	for _, event := range events {
		if err := worker.process(ctx, event); err != nil {
			return fmt.Errorf("process payment event %s: %w", event.EventID, err)
		}
	}
	return nil
}

func (worker *paddlePaymentProcessor) deferEvent(ctx context.Context, event managedPaymentInboxRecord, reason string) error {
	result := worker.database.database.WithContext(ctx).Model(&managedPaymentInboxRecord{}).Where("id = ? AND state IN ?", event.ID, []string{paymentInboxPending, paymentInboxReconciliation}).Updates(map[string]any{"state": paymentInboxReconciliation, "reason": reason, "retry_at": worker.now().UTC().Add(paymentCheckoutRetry)})
	if result.Error != nil {
		return fmt.Errorf("retain payment verification outcome: %w", result.Error)
	}
	return nil
}

func (worker *paddlePaymentProcessor) process(ctx context.Context, event managedPaymentInboxRecord) error {
	if !strings.HasPrefix(event.EventType, "transaction.") {
		return worker.deferEvent(ctx, event, "event_processing_required")
	}
	var checkout managedPaymentCheckoutRecord
	err := worker.database.database.WithContext(ctx).Where("environment = ? AND processor_account_id = ? AND transaction_id = ?", event.Environment, event.ProcessorAccountID, event.EntityID).First(&checkout).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return worker.deferEvent(ctx, event, "checkout_unresolved")
	}
	if err != nil {
		return fmt.Errorf("read payment checkout: %w", err)
	}
	var order managedFundingOrderRecord
	if err := worker.database.database.WithContext(ctx).Where("id = ?", checkout.OrderID).First(&order).Error; err != nil {
		return fmt.Errorf("read payment order: %w", err)
	}
	if order.SupplierID != worker.catalog.supplierID {
		return worker.deferEvent(ctx, event, "supplier_mismatch")
	}
	if event.EventType != paymentTransactionCompleted {
		// Informational events cannot grant funds or move a verified paid order
		// backwards. Completion and adjustments own their financial effects.
		return worker.database.database.WithContext(ctx).Model(&managedPaymentInboxRecord{}).Where("id = ? AND state IN ?", event.ID, []string{paymentInboxPending, paymentInboxReconciliation}).Updates(map[string]any{"state": paymentInboxApplied, "reason": "no_funding_effect"}).Error
	}
	var envelope struct {
		Data billing.PaddleTransactionCompletedWebhookData `json:"data"`
	}
	if err := json.Unmarshal([]byte(event.Payload), &envelope); err != nil {
		return worker.deferEvent(ctx, event, "transaction_event_invalid")
	}
	eventEvidence, err := completedPaymentEvidence(envelope.Data, order, checkout)
	if err != nil {
		return worker.deferEvent(ctx, event, "transaction_event_mismatch")
	}
	attempt, cancel := context.WithTimeout(ctx, paymentCheckoutRetry)
	transaction, err := worker.client.GetTransaction(attempt, checkout.TransactionID)
	cancel()
	if err != nil {
		return worker.deferEvent(ctx, event, "transaction_unavailable")
	}
	verified, err := completedPaymentEvidence(transaction, order, checkout)
	if err != nil {
		return worker.deferEvent(ctx, event, "transaction_mismatch")
	}
	if eventEvidence.digest != verified.digest {
		return worker.deferEvent(ctx, event, "transaction_evidence_changed")
	}
	return worker.credit(ctx, event, order, checkout, verified)
}

type verifiedCompletedPayment struct {
	transaction billing.PaddleTransactionCompletedWebhookData
	completedAt time.Time
	encoded     string
	digest      string
}

func completedPaymentEvidence(transaction billing.PaddleTransactionCompletedWebhookData, order managedFundingOrderRecord, checkout managedPaymentCheckoutRecord) (verifiedCompletedPayment, error) {
	job := paymentCheckoutJob{order: order, delivery: managedPaymentDeliveryRecord{CustomerID: checkout.CustomerID}}
	if transaction.ID != checkout.TransactionID || transaction.Status != paymentTransactionStatusCompleted || !checkoutMatchesOrder(transaction, job) || transaction.DiscountID != nil {
		return verifiedCompletedPayment{}, errFundingConflict
	}
	completedAt, err := time.Parse(time.RFC3339Nano, transaction.CompletedAt)
	if err != nil || completedAt.IsZero() {
		return verifiedCompletedPayment{}, errFundingInvalid
	}
	totals := transaction.Details.Totals
	if !validPaymentTotals(totals) || totals.CurrencyCode != order.Currency || totals.Discount != "0" || totals.Credit != "0" || totals.CreditToBalance != "0" || totals.Balance != "0" || totals.GrandTotal != totals.Total || totals.GrandTotalTax != totals.Tax {
		return verifiedCompletedPayment{}, errFundingConflict
	}
	total, _ := new(big.Int).SetString(totals.Total, 10)
	subtotal, _ := new(big.Int).SetString(totals.Subtotal, 10)
	tax, _ := new(big.Int).SetString(totals.Tax, 10)
	expected := big.NewInt(order.FundingCents)
	if new(big.Int).Add(subtotal, tax).Cmp(total) != 0 || (total.Cmp(expected) != 0 && subtotal.Cmp(expected) != 0) {
		return verifiedCompletedPayment{}, errFundingConflict
	}
	if len(transaction.Details.LineItems) != 1 {
		return verifiedCompletedPayment{}, errFundingConflict
	}
	line := transaction.Details.LineItems[0]
	if line.PriceID != order.PriceID || line.Quantity != 1 || line.Totals == nil || line.Totals.Subtotal != totals.Subtotal || line.Totals.Discount != totals.Discount || line.Totals.Tax != totals.Tax || line.Totals.Total != totals.Total {
		return verifiedCompletedPayment{}, errFundingConflict
	}
	captured := new(big.Int)
	seen := make(map[string]bool)
	for _, payment := range transaction.Payments {
		if payment.PaymentAttemptID == "" || seen[payment.PaymentAttemptID] || !paymentMinorAmountPattern.MatchString(payment.Amount) {
			return verifiedCompletedPayment{}, errFundingInvalid
		}
		seen[payment.PaymentAttemptID] = true
		if payment.Status != "captured" {
			continue
		}
		if payment.CapturedAt == nil || payment.ErrorCode != nil {
			return verifiedCompletedPayment{}, errFundingInvalid
		}
		if at, err := time.Parse(time.RFC3339Nano, *payment.CapturedAt); err != nil || at.IsZero() {
			return verifiedCompletedPayment{}, errFundingInvalid
		}
		amount, _ := new(big.Int).SetString(payment.Amount, 10)
		captured.Add(captured, amount)
	}
	if captured.Cmp(total) != 0 {
		return verifiedCompletedPayment{}, errFundingConflict
	}
	if transaction.Details.PayoutTotals != nil && !validPaymentTotals(transaction.Details.PayoutTotals) {
		return verifiedCompletedPayment{}, errFundingInvalid
	}
	// Keep monetary evidence separate from mutable customer, address, checkout,
	// and transaction timestamps. All monetary strings retain exact precision.
	evidence := struct {
		CompletedAt   time.Time                        `json:"completed_at"`
		Totals        *billing.PaddleTransactionTotals `json:"totals"`
		PayoutTotals  *billing.PaddleTransactionTotals `json:"payout_totals"`
		InvoiceNumber *string                          `json:"invoice_number"`
	}{completedAt.UTC(), totals, transaction.Details.PayoutTotals, transaction.InvoiceNumber}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return verifiedCompletedPayment{}, fmt.Errorf("encode payment evidence: %w", err)
	}
	return verifiedCompletedPayment{transaction: transaction, completedAt: completedAt.UTC(), encoded: string(encoded), digest: sha256Hex(string(encoded))}, nil
}

func validPaymentTotals(totals *billing.PaddleTransactionTotals) bool {
	if totals == nil || !paymentCurrencyPattern.MatchString(totals.CurrencyCode) {
		return false
	}
	for _, amount := range []string{totals.Subtotal, totals.Discount, totals.Tax, totals.Total, totals.Credit, totals.CreditToBalance, totals.Balance, totals.GrandTotal, totals.GrandTotalTax} {
		if !paymentMinorAmountPattern.MatchString(amount) {
			return false
		}
	}
	for _, amount := range []*string{totals.Fee, totals.Earnings} {
		if amount != nil && !paymentMinorAmountPattern.MatchString(*amount) {
			return false
		}
	}
	return true
}

func (worker *paddlePaymentProcessor) credit(ctx context.Context, event managedPaymentInboxRecord, order managedFundingOrderRecord, checkout managedPaymentCheckoutRecord, evidence verifiedCompletedPayment) error {
	now := worker.now().UTC()
	totals := evidence.transaction.Details.Totals
	receipt := managedPaymentReceiptRecord{OrderID: order.ID, BillingAccountID: order.BillingAccountID, Environment: order.Environment, ProcessorAccountID: order.ProcessorAccountID, TransactionID: checkout.TransactionID, CustomerID: checkout.CustomerID, InboxID: event.ID, LedgerKey: "payment-funding:" + order.ID, Currency: order.Currency, CreditCents: order.FundingCents, GrossCents: totals.Total, TaxCents: totals.Tax, FeeCents: totals.Fee, EarningsCents: totals.Earnings, InvoiceNumber: evidence.transaction.InvoiceNumber, FinancialEvidence: evidence.encoded, EvidenceDigest: evidence.digest, CompletedAt: evidence.completedAt, CreatedAt: now}
	if payout := evidence.transaction.Details.PayoutTotals; payout != nil {
		receipt.PayoutCurrency, receipt.PayoutEarningsCents = payout.CurrencyCode, payout.Earnings
	}
	privateQuery := worker.database.database.Session(&gorm.Session{Logger: paymentInboxLogger{worker.database.database.Logger}})
	return privateQuery.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lock := tx.Model(&managedBillingAccountRecord{}).Where("id = ?", order.BillingAccountID).UpdateColumn("id", gorm.Expr("id"))
		if lock.Error != nil {
			return fmt.Errorf("lock payment account: %w", lock.Error)
		}
		if lock.RowsAffected != 1 {
			return errFundingNotFound
		}
		var retained managedPaymentReceiptRecord
		err := tx.Where("order_id = ?", order.ID).First(&retained).Error
		if err == nil {
			if retained.TransactionID != receipt.TransactionID || retained.EvidenceDigest != receipt.EvidenceDigest {
				return errFundingConflict
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			metadata, err := json.Marshal(map[string]string{"funding_order_id": order.ID, "transaction_id": checkout.TransactionID, "environment": order.Environment, "processor_account_id": order.ProcessorAccountID})
			if err != nil {
				return fmt.Errorf("encode funding credit: %w", err)
			}
			if err := postHostedFundsCredit(tx, order.BillingAccountID, receipt.LedgerKey, order.FundingCents, now, metadata); err != nil {
				return err
			}
			if err := tx.Omit(clause.Associations).Create(&receipt).Error; err != nil {
				return fmt.Errorf("retain payment receipt: %w", err)
			}
			result := tx.Model(&managedFundingOrderRecord{}).Where("id = ? AND state IN ?", order.ID, []string{fundingOrderCreated, "pending", "failed"}).Update("state", fundingOrderPaid)
			if result.Error != nil {
				return fmt.Errorf("complete funding order: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return errFundingConflict
			}
		} else {
			return fmt.Errorf("read funding receipt: %w", err)
		}
		if err := tx.Model(&managedPaymentInboxRecord{}).Where("id = ?", event.ID).Updates(map[string]any{"state": paymentInboxApplied, "reason": ""}).Error; err != nil {
			return fmt.Errorf("complete payment event: %w", err)
		}
		return nil
	})
}
