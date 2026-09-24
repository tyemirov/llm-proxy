package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/tyemirov/utils/billing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The projection and its immutable revisions commit with the Ledger effects.
// Processor fees and taxes are evidence, never additional customer debits.
type managedPaymentAdjustmentRecord struct {
	OrderID          string                    `gorm:"primaryKey"`
	Order            managedFundingOrderRecord `gorm:"foreignKey:OrderID;references:ID;constraint:OnDelete:RESTRICT"`
	BillingAccountID string                    `gorm:"not null;index"`
	Revision         uint64                    `gorm:"not null"`
	ReversedCents    int64                     `gorm:"not null;check:reversed_cents >= 0"`
	PendingCents     int64                     `gorm:"not null;check:pending_cents >= 0"`
	HeldCents        int64                     `gorm:"not null;check:held_cents >= 0"`
	HoldID           string                    `gorm:"not null"`
	Evidence         string                    `gorm:"not null"`
	EvidenceDigest   string                    `gorm:"not null"`
	UpdatedAt        time.Time                 `gorm:"not null"`
}

type managedPaymentAdjustmentRevisionRecord struct {
	OrderID        string    `gorm:"primaryKey"`
	Revision       uint64    `gorm:"primaryKey"`
	Evidence       string    `gorm:"not null"`
	EvidenceDigest string    `gorm:"not null"`
	ReversedCents  int64     `gorm:"not null"`
	PendingCents   int64     `gorm:"not null"`
	HeldCents      int64     `gorm:"not null"`
	CreatedAt      time.Time `gorm:"not null"`
}

type paymentAdjustmentEvidence struct {
	TransactionUpdatedAt time.Time                                `json:"transaction_updated_at"`
	Totals               *billing.PaddleAdjustedTransactionTotals `json:"adjusted_totals"`
	Adjustments          []billing.PaddleAdjustment               `json:"adjustments"`
	ReversedExact        string                                   `json:"reversed_exact_cents"`
	PendingExact         string                                   `json:"pending_exact_cents"`
}

type verifiedPaymentAdjustments struct {
	evidence paymentAdjustmentEvidence
	encoded  string
	digest   string
	reversed int64
	pending  int64
	disputed bool
}

func (worker *paddlePaymentProcessor) readAdjustments(ctx context.Context, payment verifiedCompletedPayment, order managedFundingOrderRecord, checkout managedPaymentCheckoutRecord) (verifiedPaymentAdjustments, error) {
	attempt, cancel := context.WithTimeout(ctx, paymentCheckoutRetry)
	defer cancel()
	adjustments, err := worker.client.ListTransactionAdjustments(attempt, checkout.TransactionID)
	if err != nil {
		return verifiedPaymentAdjustments{}, err
	}
	return verifyPaymentAdjustments(payment, order, adjustments)
}

func verifyPaymentAdjustments(payment verifiedCompletedPayment, order managedFundingOrderRecord, adjustments []billing.PaddleAdjustment) (verifiedPaymentAdjustments, error) {
	transaction := payment.transaction
	updated := payment.updatedAt

	totals := transaction.Details.AdjustedTotals
	if totals == nil || totals.CurrencyCode != order.Currency {
		return verifiedPaymentAdjustments{}, errFundingInvalid
	}
	for _, value := range []string{totals.Subtotal, totals.Tax, totals.Total, totals.GrandTotal, totals.GrandTotalTax, totals.RetainedFee} {
		if !paymentMinorAmountPattern.MatchString(value) {
			return verifiedPaymentAdjustments{}, errFundingInvalid
		}
	}
	if !paymentAmountSum(totals.Subtotal, totals.Tax, totals.Total) || totals.GrandTotal != totals.Total || totals.GrandTotalTax != totals.Tax {
		return verifiedPaymentAdjustments{}, errFundingConflict
	}
	original, _ := new(big.Int).SetString(transaction.Details.Totals.Subtotal, 10)
	remaining, _ := new(big.Int).SetString(totals.Subtotal, 10)
	if original.Sign() <= 0 || remaining.Cmp(original) > 0 {
		return verifiedPaymentAdjustments{}, errFundingConflict
	}
	approved, pending := new(big.Int), new(big.Int)
	disputed := false
	seen := make(map[string]bool, len(adjustments))
	sort.Slice(adjustments, func(left, right int) bool { return adjustments[left].ID < adjustments[right].ID })
	for _, adjustment := range adjustments {
		if adjustment.ID == "" || seen[adjustment.ID] || adjustment.TransactionID != transaction.ID || adjustment.CustomerID != transaction.CustomerID || adjustment.SubscriptionID != nil || adjustment.CurrencyCode != order.Currency || adjustment.Totals.CurrencyCode != order.Currency || len(adjustment.Items) != 1 {
			return verifiedPaymentAdjustments{}, errFundingConflict
		}
		seen[adjustment.ID] = true
		createdAt, createdErr := time.Parse(time.RFC3339Nano, adjustment.CreatedAt)
		updatedAt, updatedErr := time.Parse(time.RFC3339Nano, adjustment.UpdatedAt)
		if createdErr != nil || updatedErr != nil || createdAt.Before(payment.completedAt) || updatedAt.Before(createdAt) {
			return verifiedPaymentAdjustments{}, errFundingInvalid
		}
		item := adjustment.Items[0]
		if item.ItemID != transaction.Details.LineItems[0].ID || item.Totals.Subtotal != adjustment.Totals.Subtotal || item.Totals.Tax != adjustment.Totals.Tax || item.Totals.Total != adjustment.Totals.Total {
			return verifiedPaymentAdjustments{}, errFundingConflict
		}
		for _, value := range []string{adjustment.Totals.Subtotal, adjustment.Totals.Tax, adjustment.Totals.Total, adjustment.Totals.Fee, adjustment.Totals.RetainedFee} {
			if !paymentMinorAmountPattern.MatchString(value) {
				return verifiedPaymentAdjustments{}, errFundingInvalid
			}
		}
		if !paymentAmountSum(adjustment.Totals.Subtotal, adjustment.Totals.Tax, adjustment.Totals.Total) {
			return verifiedPaymentAdjustments{}, errFundingConflict
		}
		amount, _ := new(big.Int).SetString(adjustment.Totals.Subtotal, 10)
		forward := true
		switch adjustment.Action {
		case "refund", "credit", "chargeback", "chargeback_warning":
		case "chargeback_reverse", "chargeback_warning_reverse", "credit_reverse":
			// The original adjustment becomes reversed. Its replacement record
			// is retained as evidence, but cannot restore the credit twice.
			forward = false
		default:
			return verifiedPaymentAdjustments{}, errFundingInvalid
		}
		switch adjustment.Status {
		case "pending_approval":
			if !forward {
				return verifiedPaymentAdjustments{}, errFundingInvalid
			}
			pending.Add(pending, amount)
		case "approved":
			if forward {
				approved.Add(approved, amount)
				disputed = disputed || strings.HasPrefix(adjustment.Action, "chargeback")
			}
		case "rejected", "reversed":
		default:
			return verifiedPaymentAdjustments{}, errFundingInvalid
		}
	}
	// Warnings and a subsequent chargeback can describe the same principal.
	// Current adjusted totals must corroborate the bounded cumulative amount.
	if approved.Cmp(original) > 0 {
		approved.Set(original)
	}
	reversed := new(big.Int).Sub(original, remaining)
	if reversed.Cmp(approved) != 0 {
		return verifiedPaymentAdjustments{}, errFundingConflict
	}
	if pending.Cmp(remaining) > 0 {
		pending.Set(remaining)
	}
	reversedExact := new(big.Rat).SetFrac(new(big.Int).Mul(reversed, big.NewInt(order.FundingCents)), original)
	pendingExact := new(big.Rat).SetFrac(new(big.Int).Mul(pending, big.NewInt(order.FundingCents)), original)
	// Floor the cumulative debit; round the pending hold up. Retain both exact
	// ratios so rounding never compounds across partial refunds.
	debit := new(big.Int).Quo(reversedExact.Num(), reversedExact.Denom()).Int64()
	hold, remainder := new(big.Int), new(big.Int)
	hold.QuoRem(pendingExact.Num(), pendingExact.Denom(), remainder)
	if remainder.Sign() > 0 {
		hold.Add(hold, big.NewInt(1))
	}
	evidence := paymentAdjustmentEvidence{updated.UTC(), totals, adjustments, reversedExact.RatString(), pendingExact.RatString()}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return verifiedPaymentAdjustments{}, fmt.Errorf("encode payment adjustments: %w", err)
	}
	return verifiedPaymentAdjustments{evidence, string(encoded), sha256Hex(string(encoded)), debit, hold.Int64(), disputed}, nil
}

func paymentAmountSum(left, right, total string) bool {
	first, firstOK := new(big.Int).SetString(left, 10)
	second, secondOK := new(big.Int).SetString(right, 10)
	want, totalOK := new(big.Int).SetString(total, 10)
	return firstOK && secondOK && totalOK && new(big.Int).Add(first, second).Cmp(want) == 0
}

func (worker *paddlePaymentProcessor) processAdjustment(ctx context.Context, event managedPaymentInboxRecord, order managedFundingOrderRecord, checkout managedPaymentCheckoutRecord) error {
	var receipt managedPaymentReceiptRecord
	err := worker.database.database.WithContext(ctx).Where("order_id = ?", order.ID).First(&receipt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return worker.deferEvent(ctx, event, "funding_receipt_unresolved")
	}
	if err != nil {
		return fmt.Errorf("read adjusted payment receipt: %w", err)
	}
	attempt, cancel := context.WithTimeout(ctx, paymentCheckoutRetry)
	transaction, err := worker.client.GetTransaction(attempt, checkout.TransactionID)
	cancel()
	if err != nil {
		return worker.deferEvent(ctx, event, "transaction_unavailable")
	}
	payment, err := completedPaymentEvidence(transaction, order, checkout)
	if err != nil || payment.digest != receipt.EvidenceDigest {
		return worker.deferEvent(ctx, event, "transaction_evidence_changed")
	}
	adjustments, err := worker.readAdjustments(ctx, payment, order, checkout)
	if err != nil {
		return worker.deferEvent(ctx, event, "adjustment_evidence_unavailable")
	}
	found := false
	for _, adjustment := range adjustments.evidence.Adjustments {
		found = found || adjustment.ID == event.EntityID
	}
	if !found {
		return worker.deferEvent(ctx, event, "adjustment_unresolved")
	}
	return worker.credit(ctx, event, order, checkout, payment, adjustments)
}

func applyPaymentAdjustments(tx *gorm.DB, order managedFundingOrderRecord, verified verifiedPaymentAdjustments, now time.Time) error {
	tx = tx.Session(&gorm.Session{Logger: paymentInboxLogger{tx.Logger}})
	var previous managedPaymentAdjustmentRecord
	err := tx.Where("order_id = ?", order.ID).First(&previous).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("read payment adjustment projection: %w", err)
	}
	if err == nil {
		if previous.EvidenceDigest == verified.digest && previous.HeldCents == verified.pending {
			return nil
		}
		var evidence paymentAdjustmentEvidence
		if err := json.Unmarshal([]byte(previous.Evidence), &evidence); err != nil {
			return fmt.Errorf("decode retained adjustment evidence: %w", err)
		}
		if !paymentAdjustmentRecordsFollow(evidence.Adjustments, verified.evidence.Adjustments) {
			return errFundingConflict
		}
	}
	next := managedPaymentAdjustmentRecord{OrderID: order.ID, BillingAccountID: order.BillingAccountID, Revision: previous.Revision + 1, ReversedCents: verified.reversed, PendingCents: verified.pending, Evidence: verified.encoded, EvidenceDigest: verified.digest, UpdatedAt: now}
	key := fmt.Sprintf("payment-adjustment:%s:%d", order.ID, next.Revision)
	metadata, err := json.Marshal(map[string]string{"funding_order_id": order.ID, "evidence_digest": verified.digest})
	if err != nil {
		return err
	}
	account, err := newHostedLedgerAccount(tx, order.BillingAccountID, now)
	if err != nil {
		return err
	}
	ledgerMetadata, err := ledger.NewMetadataJSON(string(metadata))
	if err != nil {
		return err
	}
	if previous.HeldCents > 0 {
		reservation, err := ledger.NewReservationID(previous.HoldID)
		if err != nil {
			return err
		}
		releaseKey, err := ledger.NewIdempotencyKey(key + ":release")
		if err != nil {
			return err
		}
		if err := account.service.Release(tx.Statement.Context, account.tenant, account.user, account.namespace, reservation, releaseKey, ledgerMetadata); err != nil {
			return fmt.Errorf("release payment hold: %w", err)
		}
	}
	delta := previous.ReversedCents - verified.reversed
	if delta > 0 {
		if err := postHostedFundsCredit(tx, order.BillingAccountID, key, delta, now, metadata); err != nil {
			return err
		}
	} else if delta < 0 {
		// Mandatory processor reversals can exceed the remaining balance.
		// The shared Ledger store supports signed entries; admission will reject
		// a deficit instead of preventing the external reversal from being recorded.
		store := gormstore.New(tx)
		accountID, err := store.GetOrCreateAccountID(tx.Statement.Context, account.tenant, account.user, account.namespace)
		if err != nil {
			return err
		}
		amount, err := ledger.NewEntryAmountCents(delta)
		if err != nil {
			return err
		}
		entryKey, err := ledger.NewIdempotencyKey(key)
		if err != nil {
			return err
		}
		entry, err := ledger.NewEntryInput(accountID, ledger.EntrySpend, amount, nil, nil, entryKey, 0, ledgerMetadata, now.Unix())
		if err != nil {
			return err
		}
		if _, err := store.InsertEntry(tx.Statement.Context, entry); err != nil {
			return fmt.Errorf("post payment reversal: %w", err)
		}
	}
	balance, err := account.service.Balance(tx.Statement.Context, account.tenant, account.user, account.namespace)
	if err != nil {
		return err
	}
	next.HeldCents = min(verified.pending, max(int64(0), balance.AvailableCents.Int64()))
	if next.HeldCents > 0 {
		next.HoldID = key + ":hold"
		reservation, err := ledger.NewReservationID(next.HoldID)
		if err != nil {
			return err
		}
		holdKey, err := ledger.NewIdempotencyKey(next.HoldID)
		if err != nil {
			return err
		}
		amount, err := ledger.NewPositiveAmountCents(next.HeldCents)
		if err != nil {
			return err
		}
		if err := account.service.Reserve(tx.Statement.Context, account.tenant, account.user, account.namespace, amount, reservation, holdKey, 0, ledgerMetadata); err != nil {
			return fmt.Errorf("reserve payment hold: %w", err)
		}
	}
	state := fundingOrderPaid
	if verified.reversed > 0 {
		state = "partially_refunded"
	}
	if verified.reversed == order.FundingCents {
		state = "refunded"
	}
	if verified.disputed {
		state = "disputed"
	}
	if err := tx.Model(&managedFundingOrderRecord{}).Where("id = ?", order.ID).Update("state", state).Error; err != nil {
		return fmt.Errorf("record adjusted order: %w", err)
	}
	if err := tx.Omit(clause.Associations).Save(&next).Error; err != nil {
		return fmt.Errorf("retain payment adjustment projection: %w", err)
	}
	revision := managedPaymentAdjustmentRevisionRecord{order.ID, next.Revision, next.Evidence, next.EvidenceDigest, next.ReversedCents, next.PendingCents, next.HeldCents, now}
	if err := tx.Create(&revision).Error; err != nil {
		return fmt.Errorf("retain payment adjustment revision: %w", err)
	}
	return nil
}

// New processor evidence passes retainPaymentStateObservation in the same
// transaction. Hold refresh reuses its saved evidence without a processor read.
// This boundary checks the separate adjustment history.
func paymentAdjustmentRecordsFollow(previous, next []billing.PaddleAdjustment) bool {
	current := make(map[string]billing.PaddleAdjustment, len(next))
	for _, adjustment := range next {
		current[adjustment.ID] = adjustment
	}
	for _, earlier := range previous {
		later, exists := current[earlier.ID]
		if !exists {
			return false
		}
		earlierAt, _ := time.Parse(time.RFC3339Nano, earlier.UpdatedAt)
		laterAt, _ := time.Parse(time.RFC3339Nano, later.UpdatedAt)
		if laterAt.Before(earlierAt) {
			return false
		}
		if laterAt.Equal(earlierAt) {
			before, _ := json.Marshal(earlier)
			after, _ := json.Marshal(later)
			if string(before) != string(after) {
				return false
			}
		}
	}
	return true
}

// Payment restrictions are derived separately from operator account state, so a
// recovered payment cannot clear an unrelated operator suspension.
func paymentFundsRestricted(tx *gorm.DB, accountID string, now time.Time) (bool, error) {
	var count int64
	if err := tx.Model(&managedPaymentAdjustmentRecord{}).Where("billing_account_id = ? AND pending_cents > held_cents", accountID).Count(&count).Error; err != nil {
		return false, fmt.Errorf("read payment hold deficit: %w", err)
	}
	if count > 0 {
		return true, nil
	}
	var account gormstore.LedgerAccount
	err := tx.Where("tenant_id = ? AND user_id = ? AND ledger_id = ?", hostedLedgerTenant, accountID, CatalogCurrencyUSD).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	identifier, err := ledger.NewAccountID(account.AccountID)
	if err != nil {
		return false, err
	}
	store := gormstore.New(tx)
	total, err := store.SumTotal(tx.Statement.Context, identifier, now.Unix())
	if err != nil {
		return false, err
	}
	holds, err := store.SumActiveHolds(tx.Statement.Context, identifier, now.Unix())
	if err != nil {
		return false, err
	}
	return total.Int64() < holds.Int64(), nil
}

// Run under the account writer lock whenever new funds or released usage holds
// can cover a previously unreserved refund amount.
func refreshPaymentHolds(tx *gorm.DB, accountID string, now time.Time) error {
	var projections []managedPaymentAdjustmentRecord
	if err := tx.Where("billing_account_id = ? AND pending_cents > held_cents", accountID).Order("order_id").Find(&projections).Error; err != nil {
		return fmt.Errorf("find payment hold deficits: %w", err)
	}
	for _, projection := range projections {
		var order managedFundingOrderRecord
		if err := tx.Where("id = ?", projection.OrderID).First(&order).Error; err != nil {
			return err
		}
		var evidence paymentAdjustmentEvidence
		if err := json.Unmarshal([]byte(projection.Evidence), &evidence); err != nil {
			return err
		}
		verified := verifiedPaymentAdjustments{evidence: evidence, encoded: projection.Evidence, digest: projection.EvidenceDigest, reversed: projection.ReversedCents, pending: projection.PendingCents, disputed: order.State == "disputed"}
		if err := applyPaymentAdjustments(tx, order, verified, now); err != nil {
			return err
		}
	}
	return nil
}
