package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tyemirov/utils/billing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	fundingOrderPending              = "pending"
	fundingOrderFailed               = "failed"
	paymentTransactionStatusCanceled = "canceled"
)

// Observations retain the processor response used for a state decision.
// They commit with the order, event, and any funding effect.
type managedPaymentStateObservationRecord struct {
	ID                 string                    `gorm:"primaryKey"`
	OrderID            string                    `gorm:"not null;index:payment_state_order,priority:1"`
	Order              managedFundingOrderRecord `gorm:"foreignKey:OrderID;references:ID;constraint:OnDelete:RESTRICT"`
	InboxID            string                    `gorm:"not null;index"`
	Inbox              managedPaymentInboxRecord `gorm:"foreignKey:InboxID;references:ID;constraint:OnDelete:RESTRICT"`
	ProcessorUpdatedAt time.Time                 `gorm:"not null;index:payment_state_order,priority:2"`
	ProcessorStatus    string                    `gorm:"not null;check:processor_status IN ('draft','ready','billed','paid','completed','past_due','canceled')"`
	Evidence           string                    `gorm:"not null"`
	EvidenceDigest     string                    `gorm:"not null"`
	CreatedAt          time.Time                 `gorm:"not null"`
}

func paymentStateEvent(eventType string) bool {
	switch eventType {
	case "transaction.created", "transaction.ready", "transaction.billed", "transaction.paid", "transaction.past_due", "transaction.payment_failed", "transaction.canceled", "transaction.updated", "transaction.revised":
		return true
	default:
		return false
	}
}

func validatePaymentState(transaction billing.PaddleTransactionCompletedWebhookData, order managedFundingOrderRecord, checkout managedPaymentCheckoutRecord) (time.Time, error) {
	job := paymentCheckoutJob{order: order, delivery: managedPaymentDeliveryRecord{CustomerID: checkout.CustomerID}}
	if transaction.ID != checkout.TransactionID || !checkoutMatchesOrder(transaction, job) {
		return time.Time{}, errFundingConflict
	}
	switch transaction.Status {
	case "draft", "ready", "billed", "paid", paymentTransactionStatusCompleted, "past_due", paymentTransactionStatusCanceled:
	default:
		return time.Time{}, errFundingInvalid
	}
	updated, err := time.Parse(time.RFC3339Nano, transaction.UpdatedAt)
	if err != nil || updated.IsZero() {
		return time.Time{}, errFundingInvalid
	}
	return updated.UTC(), nil
}

func paymentStateObservation(event managedPaymentInboxRecord, order managedFundingOrderRecord, transaction billing.PaddleTransactionCompletedWebhookData, updated, now time.Time) (managedPaymentStateObservationRecord, error) {
	encoded, err := json.Marshal(transaction)
	if err != nil {
		return managedPaymentStateObservationRecord{}, fmt.Errorf("encode processor state: %w", err)
	}
	digest := sha256Hex(string(encoded))
	return managedPaymentStateObservationRecord{ID: "payment-state-" + sha256Hex(event.ID + "\x00" + digest)[:32], OrderID: order.ID, InboxID: event.ID, ProcessorUpdatedAt: updated, ProcessorStatus: transaction.Status, Evidence: string(encoded), EvidenceDigest: digest, CreatedAt: now}, nil
}

// The caller holds the financial account writer lock.
func retainPaymentStateObservation(tx *gorm.DB, observation managedPaymentStateObservationRecord) error {
	var latest managedPaymentStateObservationRecord
	err := tx.Where("order_id = ?", observation.OrderID).Order("processor_updated_at DESC, id DESC").First(&latest).Error
	if err == nil {
		if observation.ProcessorUpdatedAt.Before(latest.ProcessorUpdatedAt) || (observation.ProcessorUpdatedAt.Equal(latest.ProcessorUpdatedAt) && observation.EvidenceDigest != latest.EvidenceDigest) {
			return errFundingConflict
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("read retained processor state: %w", err)
	}
	if err := tx.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&observation).Error; err != nil {
		return fmt.Errorf("retain processor state: %w", err)
	}
	return nil
}

func (worker *paddlePaymentProcessor) processState(ctx context.Context, event managedPaymentInboxRecord, order managedFundingOrderRecord, checkout managedPaymentCheckoutRecord) error {
	var envelope struct {
		Data billing.PaddleTransactionCompletedWebhookData `json:"data"`
	}
	if err := json.Unmarshal([]byte(event.Payload), &envelope); err != nil {
		return worker.deferEvent(ctx, event, "transaction_event_invalid")
	}
	eventUpdated, err := validatePaymentState(envelope.Data, order, checkout)
	if err != nil {
		return worker.deferEvent(ctx, event, "transaction_event_mismatch")
	}
	attempt, cancel := context.WithTimeout(ctx, paymentCheckoutRetry)
	current, err := worker.client.GetTransaction(attempt, checkout.TransactionID)
	cancel()
	if err != nil {
		return worker.deferEvent(ctx, event, "transaction_unavailable")
	}
	updated, err := validatePaymentState(current, order, checkout)
	if err != nil {
		return worker.deferEvent(ctx, event, "transaction_mismatch")
	}
	if updated.Before(eventUpdated) {
		return worker.deferEvent(ctx, event, "transaction_snapshot_stale")
	}
	completedDigest := ""
	if current.Status == paymentTransactionStatusCompleted {
		evidence, err := completedPaymentEvidence(current, order, checkout)
		if err != nil {
			return worker.deferEvent(ctx, event, "transaction_mismatch")
		}
		completedDigest = evidence.digest
	}
	now := worker.now().UTC()
	observation, err := paymentStateObservation(event, order, current, updated, now)
	if err != nil {
		return err
	}
	privateQuery := worker.database.database.Session(&gorm.Session{Logger: paymentInboxLogger{worker.database.database.Logger}})
	err = privateQuery.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lock := tx.Model(&managedBillingAccountRecord{}).Where("id = ?", order.BillingAccountID).UpdateColumn("id", gorm.Expr("id"))
		if lock.Error != nil {
			return fmt.Errorf("lock payment state account: %w", lock.Error)
		}
		if lock.RowsAffected != 1 {
			return errFundingNotFound
		}
		if err := retainPaymentStateObservation(tx, observation); err != nil {
			return err
		}
		var receipt managedPaymentReceiptRecord
		receiptError := tx.Where("order_id = ?", order.ID).First(&receipt).Error
		state, reason := paymentInboxApplied, "no_funding_effect"
		if receiptError == nil {
			// A receipt is never reversed by a lifecycle notification. Adjustments own reversals.
			if current.Status != paymentTransactionStatusCompleted || receipt.EvidenceDigest != completedDigest {
				return errFundingConflict
			}
		} else if errors.Is(receiptError, gorm.ErrRecordNotFound) {
			nextState := fundingOrderPending
			if current.Status == paymentTransactionStatusCanceled {
				nextState = fundingOrderFailed
			}
			if current.Status == paymentTransactionStatusCompleted {
				state, reason = paymentInboxReconciliation, "completion_event_required"
			}
			result := tx.Model(&managedFundingOrderRecord{}).Where("id = ? AND state IN ?", order.ID, []string{fundingOrderCreated, fundingOrderPending, fundingOrderFailed}).Update("state", nextState)
			if result.Error != nil {
				return fmt.Errorf("update verified payment state: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return errFundingConflict
			}
		} else {
			return fmt.Errorf("read payment state receipt: %w", receiptError)
		}
		if err := tx.Model(&managedPaymentInboxRecord{}).Where("id = ?", event.ID).Updates(map[string]any{"state": state, "reason": reason, "retry_at": now.Add(paymentCheckoutRetry)}).Error; err != nil {
			return fmt.Errorf("complete payment state event: %w", err)
		}
		return nil
	})
	if errors.Is(err, errFundingConflict) {
		return worker.deferEvent(ctx, event, "transaction_state_conflict")
	}
	return err
}
