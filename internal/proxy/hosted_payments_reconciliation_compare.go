package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/tyemirov/utils/billing"
	"gorm.io/gorm"
)

type paymentReconciliationEvidence struct {
	Order             managedFundingOrderRecord                      `json:"order"`
	Checkout          managedPaymentCheckoutRecord                   `json:"checkout"`
	Receipt           *managedPaymentReceiptRecord                   `json:"receipt"`
	Projection        *managedPaymentAdjustmentRecord                `json:"projection"`
	Transaction       *billing.PaddleTransactionCompletedWebhookData `json:"transaction"`
	Adjustments       []billing.PaddleAdjustment                     `json:"adjustments"`
	Entries           []gormstore.LedgerEntry                        `json:"entries"`
	Hold              *gormstore.Reservation                         `json:"hold"`
	LatestObservation *managedPaymentStateObservationRecord          `json:"latest_observation"`
}

func (report *PaymentReconciliationItem) difference(category, code, expected, observed string) {
	report.Differences = append(report.Differences, FinancialReconciliationDifference{category, code, expected, observed})
}

func optionalPaymentRecord(tx *gorm.DB, result any, query string, values ...any) (bool, error) {
	err := tx.Where(query, values...).First(result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read payment audit record: %w", err)
	}
	return true, nil
}

func comparePaymentEvidence(tx *gorm.DB, order managedFundingOrderRecord, checkout managedPaymentCheckoutRecord, transaction *billing.PaddleTransactionCompletedWebhookData, adjustments []billing.PaddleAdjustment, now time.Time) (PaymentReconciliationItem, string, error) {
	report := PaymentReconciliationItem{OrderID: order.ID, BillingAccountID: order.BillingAccountID, ObservedAt: now, Differences: []FinancialReconciliationDifference{}}
	evidence := paymentReconciliationEvidence{Order: order, Checkout: checkout, Transaction: transaction, Adjustments: adjustments, Entries: []gormstore.LedgerEntry{}}
	var receipt managedPaymentReceiptRecord
	found, err := optionalPaymentRecord(tx, &receipt, "order_id = ?", order.ID)
	if err != nil {
		return report, "", err
	}
	if found {
		evidence.Receipt = &receipt
	}
	var projection managedPaymentAdjustmentRecord
	found, err = optionalPaymentRecord(tx, &projection, "order_id = ?", order.ID)
	if err != nil {
		return report, "", err
	}
	if found {
		evidence.Projection = &projection
	}
	var latest managedPaymentStateObservationRecord
	found, err = optionalPaymentRecord(tx.Order("processor_updated_at DESC, id DESC"), &latest, "order_id = ?", order.ID)
	if err != nil {
		return report, "", err
	}
	if found {
		evidence.LatestObservation = &latest
	}
	if err := readPaymentLedgerEvidence(tx, &evidence); err != nil {
		return report, "", err
	}
	comparePaymentProcessor(&report, evidence)
	comparePaymentLedger(&report, evidence)
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return report, "", fmt.Errorf("encode reconciliation evidence: %w", err)
	}
	return report, string(encoded), nil
}

func readPaymentLedgerEvidence(tx *gorm.DB, evidence *paymentReconciliationEvidence) error {
	var account gormstore.LedgerAccount
	found, err := optionalPaymentRecord(tx, &account, "tenant_id = ? AND user_id = ? AND ledger_id = ?", hostedLedgerTenant, evidence.Order.BillingAccountID, evidence.Order.Currency)
	if err != nil || !found {
		return err
	}
	var entries []gormstore.LedgerEntry
	if err := tx.Where("account_id = ?", account.AccountID).Order("created_at, entry_id").Find(&entries).Error; err != nil {
		return fmt.Errorf("read reconciliation Ledger entries: %w", err)
	}
	fundingKey := "payment-funding:" + evidence.Order.ID
	adjustmentPrefix := "payment-adjustment:" + evidence.Order.ID + ":"
	for _, entry := range entries {
		var metadata map[string]json.RawMessage
		if err := json.Unmarshal(entry.Metadata, &metadata); err != nil {
			return fmt.Errorf("decode Ledger metadata for reconciliation: %w", err)
		}
		var orderID string
		if raw, exists := metadata["funding_order_id"]; exists {
			if err := json.Unmarshal(raw, &orderID); err != nil {
				return fmt.Errorf("decode Ledger funding order: %w", err)
			}
		}
		if entry.IdempotencyKey == fundingKey || strings.HasPrefix(entry.IdempotencyKey, adjustmentPrefix) || orderID == evidence.Order.ID {
			evidence.Entries = append(evidence.Entries, entry)
		}
	}
	if evidence.Projection != nil && evidence.Projection.HoldID != "" {
		var hold gormstore.Reservation
		found, err := optionalPaymentRecord(tx, &hold, "account_id = ? AND reservation_id = ?", account.AccountID, evidence.Projection.HoldID)
		if err != nil {
			return err
		}
		if found {
			evidence.Hold = &hold
		}
	}
	return nil
}

func comparePaymentProcessor(report *PaymentReconciliationItem, evidence paymentReconciliationEvidence) {
	transaction := evidence.Transaction
	if transaction == nil {
		report.difference("timing", "checkout_missing", "retained processor checkout", "absent")
		return
	}
	order, checkout := evidence.Order, evidence.Checkout
	if transaction.CurrencyCode != order.Currency {
		report.difference("currency", "payment_currency", order.Currency, transaction.CurrencyCode)
	}
	if transaction.DiscountID != nil {
		report.difference("discount", "payment_discount", "none", "present")
	}
	updated, err := validatePaymentState(*transaction, order, checkout)
	if err != nil {
		report.difference("identity", "transaction_mismatch", "owned transaction and current state", "invalid")
		return
	}
	if evidence.LatestObservation != nil {
		encoded, _ := json.Marshal(transaction)
		if updated.Before(evidence.LatestObservation.ProcessorUpdatedAt) || (updated.Equal(evidence.LatestObservation.ProcessorUpdatedAt) && sha256Hex(string(encoded)) != evidence.LatestObservation.EvidenceDigest) {
			report.difference("timing", "processor_snapshot_conflict", "current consistent processor snapshot", "older or conflicting")
			return
		}
	}
	if transaction.Status != paymentTransactionStatusCompleted {
		expected := fundingOrderPending
		if transaction.Status == paymentTransactionStatusCanceled {
			expected = fundingOrderFailed
		}
		if order.State != expected {
			report.difference("timing", "order_state", expected, order.State)
		}
		if evidence.Receipt != nil {
			report.difference("timing", "receipt_without_completion", paymentTransactionStatusCompleted, transaction.Status)
		}
		return
	}
	if evidence.Receipt == nil {
		report.difference("timing", "receipt_missing", "verified receipt", "absent")
	}
	if transaction.Details.Totals != nil {
		if transaction.Details.Totals.Discount != "0" {
			report.difference("discount", "payment_discount_amount", "0", transaction.Details.Totals.Discount)
		}
		if evidence.Receipt != nil && !sameOptionalPaymentAmount(evidence.Receipt.FeeCents, transaction.Details.Totals.Fee) {
			report.difference("fee", "processor_fee", optionalPaymentAmount(evidence.Receipt.FeeCents), optionalPaymentAmount(transaction.Details.Totals.Fee))
		}
	}
	payment, err := completedPaymentEvidence(*transaction, order, checkout)
	if err != nil {
		report.difference("amount", "completed_evidence_invalid", "verified completed payment", "invalid")
		return
	}
	if evidence.Receipt != nil && evidence.Receipt.EvidenceDigest != payment.digest {
		report.difference("amount", "receipt_evidence_changed", evidence.Receipt.EvidenceDigest, payment.digest)
	}
	verified, err := verifyPaymentAdjustments(payment, order, evidence.Adjustments)
	if err != nil {
		report.difference("adjustment", "adjustment_evidence_invalid", "consistent adjustment totals", "invalid")
		return
	}
	expectedState := fundingOrderPaid
	if verified.disputed {
		expectedState = "disputed"
	} else if verified.reversed == order.FundingCents {
		expectedState = "refunded"
	} else if verified.reversed > 0 {
		expectedState = "partially_refunded"
	}
	if order.State != expectedState {
		report.difference("timing", "order_state", expectedState, order.State)
	}
	if evidence.Projection == nil {
		report.difference("adjustment", "adjustment_projection_missing", "retained adjustment state", "absent")
		return
	}
	if evidence.Projection.ReversedCents != verified.reversed {
		report.difference("adjustment", "reversed_credit", strconv.FormatInt(verified.reversed, 10), strconv.FormatInt(evidence.Projection.ReversedCents, 10))
	}
	if evidence.Projection.PendingCents != verified.pending {
		report.difference("adjustment", "pending_refund", strconv.FormatInt(verified.pending, 10), strconv.FormatInt(evidence.Projection.PendingCents, 10))
	}
}

func optionalPaymentAmount(value *string) string {
	if value == nil {
		return "unknown"
	}
	return *value
}
func sameOptionalPaymentAmount(first, second *string) bool {
	return optionalPaymentAmount(first) == optionalPaymentAmount(second)
}

func comparePaymentLedger(report *PaymentReconciliationItem, evidence paymentReconciliationEvidence) {
	expectedCredit, expectedReversed := int64(0), int64(0)
	if evidence.Receipt != nil {
		expectedCredit = evidence.Receipt.CreditCents
		if expectedCredit != evidence.Order.FundingCents {
			report.difference("amount", "receipt_credit", strconv.FormatInt(evidence.Order.FundingCents, 10), strconv.FormatInt(expectedCredit, 10))
		}
		if evidence.Receipt.Currency != evidence.Order.Currency {
			report.difference("currency", "receipt_currency", evidence.Order.Currency, evidence.Receipt.Currency)
		}
		if evidence.Receipt.BillingAccountID != evidence.Order.BillingAccountID || evidence.Receipt.Environment != evidence.Order.Environment || evidence.Receipt.ProcessorAccountID != evidence.Order.ProcessorAccountID || evidence.Receipt.TransactionID != evidence.Checkout.TransactionID || evidence.Receipt.CustomerID != evidence.Checkout.CustomerID || evidence.Receipt.LedgerKey != "payment-funding:"+evidence.Order.ID {
			report.difference("identity", "receipt_identity", "owned funding receipt", "mismatch")
		}
	}
	if evidence.Projection != nil {
		expectedReversed = evidence.Projection.ReversedCents
	}
	credited, reversed := new(big.Int), new(big.Int)
	fundingEntries := 0
	for _, entry := range evidence.Entries {
		if entry.Type != string(ledger.EntryGrant) && entry.Type != string(ledger.EntrySpend) {
			continue
		}
		if strings.HasPrefix(entry.IdempotencyKey, "payment-adjustment:"+evidence.Order.ID+":") {
			reversed.Sub(reversed, big.NewInt(entry.AmountCents))
		} else {
			credited.Add(credited, big.NewInt(entry.AmountCents))
			fundingEntries++
		}
	}
	if fundingEntries > 1 {
		report.difference("duplicate_effect", "multiple_funding_entries", "1", strconv.Itoa(fundingEntries))
	}
	if credited.Cmp(big.NewInt(expectedCredit)) != 0 {
		report.difference("ledger", "funding_credit", strconv.FormatInt(expectedCredit, 10), credited.String())
	}
	if reversed.Cmp(big.NewInt(expectedReversed)) != 0 {
		report.difference("ledger", "funding_reversal", strconv.FormatInt(expectedReversed, 10), reversed.String())
	}
	if evidence.Projection != nil {
		held := int64(0)
		if evidence.Hold != nil && evidence.Hold.Status == string(ledger.ReservationStatusActive) {
			held = evidence.Hold.AmountCents
		}
		if held != evidence.Projection.HeldCents {
			report.difference("ledger", "refund_hold", strconv.FormatInt(evidence.Projection.HeldCents, 10), strconv.FormatInt(held, 10))
		}
		if evidence.Projection.HeldCents < evidence.Projection.PendingCents {
			report.difference("adjustment", "refund_hold_shortfall", strconv.FormatInt(evidence.Projection.PendingCents, 10), strconv.FormatInt(evidence.Projection.HeldCents, 10))
		}
	}
}
