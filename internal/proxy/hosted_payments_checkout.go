package proxy

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/utils/billing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	managementPaymentCheckoutPath = managementFundingOrderPath + "/checkout"
	paymentDeliveryDispatching    = "dispatching"
	paymentDeliveryDelivered      = "delivered"
	paymentDeliveryReconciliation = "reconciliation_required"
	paymentCheckoutLease          = time.Minute
	paymentCheckoutRetry          = 30 * time.Second
	paymentMetadataOrder          = "funding_order_id"
	paymentMetadataAccount        = "billing_account_id"
)

type managedPaymentCustomerRecord struct {
	ID                 string                      `gorm:"primaryKey"`
	BillingAccountID   string                      `gorm:"not null;uniqueIndex:payment_customer_account,priority:1"`
	BillingAccount     managedBillingAccountRecord `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	Environment        string                      `gorm:"not null;uniqueIndex:payment_customer_account,priority:2;uniqueIndex:payment_customer_identity,priority:1"`
	ProcessorAccountID string                      `gorm:"not null;uniqueIndex:payment_customer_account,priority:3;uniqueIndex:payment_customer_identity,priority:2"`
	CustomerID         string                      `gorm:"not null;uniqueIndex:payment_customer_identity,priority:3"`
	CreatedAt          time.Time                   `gorm:"not null"`
}

type managedPaymentCheckoutRecord struct {
	OrderID            string                    `gorm:"primaryKey"`
	Order              managedFundingOrderRecord `gorm:"foreignKey:OrderID;references:ID;constraint:OnDelete:RESTRICT"`
	Environment        string                    `gorm:"not null;uniqueIndex:payment_checkout_transaction,priority:1"`
	ProcessorAccountID string                    `gorm:"not null;uniqueIndex:payment_checkout_transaction,priority:2"`
	TransactionID      string                    `gorm:"not null;uniqueIndex:payment_checkout_transaction,priority:3"`
	CustomerID         string                    `gorm:"not null"`
	CreatedAt          time.Time                 `gorm:"not null"`
}

type paddleCheckoutDelivery struct {
	database *gormManagedTenantDatabase
	catalog  *fundingCatalog
	client   billing.PaddleCommerceClient
	now      func() time.Time
}

func newPaddleCheckoutDelivery(database *gormManagedTenantDatabase, catalog *fundingCatalog, client billing.PaddleCommerceClient) (*paddleCheckoutDelivery, error) {
	if database == nil || catalog == nil || client == nil {
		return nil, fmt.Errorf("configure Paddle checkout delivery: missing dependency")
	}
	return &paddleCheckoutDelivery{database: database, catalog: catalog, client: client, now: time.Now}, nil
}

type paymentCheckoutJob struct {
	order    managedFundingOrderRecord
	delivery managedPaymentDeliveryRecord
}

func (worker *paddleCheckoutDelivery) reconcile(ctx context.Context) error {
	now := worker.now().UTC()
	orderScope := worker.database.database.Model(&managedFundingOrderRecord{}).Select("id").Where("environment = ? AND processor_account_id = ? AND supplier_id = ?", worker.catalog.environment, worker.catalog.processorAccountID, worker.catalog.supplierID)
	var candidates []managedPaymentDeliveryRecord
	eligible := "((state IN ? AND retry_at <= ?) OR (state = ? AND lease_expires_at <= ?))"
	states := []string{paymentDeliveryPending, paymentDeliveryReconciliation}
	if err := worker.database.database.WithContext(ctx).Where("order_id IN (?)", orderScope).Where(eligible, states, now, paymentDeliveryDispatching, now).Order("created_at, order_id").Limit(100).Find(&candidates).Error; err != nil {
		return fmt.Errorf("find pending checkouts: %w", err)
	}
	for _, candidate := range candidates {
		token, err := newHostedResourceID("checkout-worker-", rand.Reader)
		if err != nil {
			return err
		}
		claimed := worker.database.database.WithContext(ctx).Model(&managedPaymentDeliveryRecord{}).Where("order_id = ?", candidate.OrderID).Where(eligible, states, now, paymentDeliveryDispatching, now).Updates(map[string]any{"state": paymentDeliveryDispatching, "owner_token": token, "lease_expires_at": now.Add(paymentCheckoutLease)})
		if claimed.Error != nil {
			return fmt.Errorf("claim checkout %s: %w", candidate.OrderID, claimed.Error)
		}
		if claimed.RowsAffected == 0 {
			continue
		}
		candidate.OwnerToken = token
		var order managedFundingOrderRecord
		if err := worker.database.database.WithContext(ctx).Where("id = ?", candidate.OrderID).First(&order).Error; err != nil {
			return fmt.Errorf("read claimed order %s: %w", candidate.OrderID, err)
		}
		attempt, cancel := context.WithTimeout(ctx, paymentCheckoutRetry)
		err = worker.deliver(attempt, paymentCheckoutJob{order: order, delivery: candidate})
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func paymentCustomerKey(order managedFundingOrderRecord) string {
	return "payment-customer-" + sha256Hex(order.Environment + "\x00" + order.ProcessorAccountID + "\x00" + order.BillingAccountID)[:32]
}

func (worker *paddleCheckoutDelivery) customer(ctx context.Context, order managedFundingOrderRecord) (string, error) {
	var record managedPaymentCustomerRecord
	key := paymentCustomerKey(order)
	err := worker.database.database.WithContext(ctx).Where("id = ?", key).First(&record).Error
	if err == nil {
		return record.CustomerID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("read processor customer: %w", err)
	}
	var owner managedUserRecord
	if err := worker.database.database.WithContext(ctx).Joins("JOIN managed_billing_account_records AS account ON account.owner_user_id = managed_user_records.user_id").Where("account.id = ?", order.BillingAccountID).First(&owner).Error; err != nil {
		return "", fmt.Errorf("read funding owner: %w", err)
	}
	customerID, err := worker.client.ResolveCustomerID(ctx, owner.UserEmail)
	if err != nil {
		return "", fmt.Errorf("resolve funding customer: %w", err)
	}
	if !strings.HasPrefix(customerID, "ctm_") || !paddleEntityIDPattern.MatchString(customerID) {
		return "", fmt.Errorf("invalid funding customer identity")
	}
	proposed := managedPaymentCustomerRecord{ID: key, BillingAccountID: order.BillingAccountID, Environment: order.Environment, ProcessorAccountID: order.ProcessorAccountID, CustomerID: customerID, CreatedAt: worker.now().UTC()}
	err = worker.database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit(clause.Associations).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&proposed).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", key).First(&record).Error; err != nil {
			return err
		}
		if record.CustomerID != customerID {
			return errFundingConflict
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("bind processor customer: %w", err)
	}
	return record.CustomerID, nil
}

func checkoutMetadata(order managedFundingOrderRecord) map[string]string {
	return map[string]string{paymentMetadataOrder: order.ID, paymentMetadataAccount: order.BillingAccountID, "environment": order.Environment, "processor_account_id": order.ProcessorAccountID, "supplier_id": order.SupplierID}
}

func (worker *paddleCheckoutDelivery) deliver(ctx context.Context, job paymentCheckoutJob) error {
	price, err := worker.client.GetPrice(ctx, job.order.PriceID)
	if err != nil {
		return worker.retainOutcome(ctx, job, paymentDeliveryPending, "price_unavailable")
	}
	if price.ID != job.order.PriceID || price.PriceCents != job.order.FundingCents || price.BillingCycle.Interval != "" || price.BillingCycle.Frequency != 0 {
		return worker.retainOutcome(ctx, job, paymentDeliveryReconciliation, "price_mismatch")
	}
	if job.delivery.TransactionDispatchedAt != nil {
		return worker.recover(ctx, job)
	}
	customerID, err := worker.customer(ctx, job.order)
	if err != nil {
		return worker.retainOutcome(ctx, job, paymentDeliveryPending, "customer_unavailable")
	}
	now := worker.now().UTC()
	result := worker.database.database.WithContext(ctx).Model(&managedPaymentDeliveryRecord{}).Where("order_id = ? AND owner_token = ? AND state = ?", job.order.ID, job.delivery.OwnerToken, paymentDeliveryDispatching).Updates(map[string]any{"customer_id": customerID, "transaction_dispatched_at": now})
	if result.Error != nil {
		return fmt.Errorf("retain checkout dispatch intent: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errFundingConflict
	}
	job.delivery.CustomerID = customerID
	transactionID, err := worker.client.CreateTransaction(ctx, billing.PaddleTransactionInput{CustomerID: customerID, PriceID: job.order.PriceID, Metadata: checkoutMetadata(job.order)})
	if err != nil {
		return worker.retainOutcome(ctx, job, paymentDeliveryReconciliation, "creation_outcome_unknown")
	}
	transaction, err := worker.client.GetTransaction(ctx, transactionID)
	if err != nil {
		return worker.retainOutcome(ctx, job, paymentDeliveryReconciliation, "transaction_unavailable")
	}
	if transaction.ID != transactionID || !checkoutMatchesOrder(transaction, job) {
		return worker.retainOutcome(ctx, job, paymentDeliveryReconciliation, "transaction_mismatch")
	}
	return worker.complete(ctx, job, transaction.ID)
}

func checkoutMatchesOrder(transaction billing.PaddleTransactionCompletedWebhookData, job paymentCheckoutJob) bool {
	if !strings.HasPrefix(transaction.ID, "txn_") || !paddleEntityIDPattern.MatchString(transaction.ID) || transaction.CustomerID != job.delivery.CustomerID || transaction.CurrencyCode != job.order.Currency || transaction.CollectionMode != "automatic" || transaction.SubscriptionID != "" || len(transaction.Items) != 1 {
		return false
	}
	for key, value := range checkoutMetadata(job.order) {
		if transaction.CustomData[key] != value {
			return false
		}
	}
	item := transaction.Items[0]
	return item.Quantity == 1 && item.Price.ID == job.order.PriceID && item.Price.UnitPrice != nil && item.Price.UnitPrice.CurrencyCode == job.order.Currency && item.Price.UnitPrice.Amount == strconv.FormatInt(job.order.FundingCents, 10)
}

func (worker *paddleCheckoutDelivery) recover(ctx context.Context, job paymentCheckoutJob) error {
	transactions, err := worker.client.ListCustomerTransactions(ctx, job.delivery.CustomerID)
	if err != nil {
		return worker.retainOutcome(ctx, job, paymentDeliveryReconciliation, "transaction_unavailable")
	}
	var matches []billing.PaddleTransactionCompletedWebhookData
	for _, transaction := range transactions {
		if transaction.CustomData[paymentMetadataOrder] == job.order.ID {
			matches = append(matches, transaction)
		}
	}
	if len(matches) != 1 || !checkoutMatchesOrder(matches[0], job) {
		return worker.retainOutcome(ctx, job, paymentDeliveryReconciliation, "transaction_unresolved")
	}
	return worker.complete(ctx, job, matches[0].ID)
}

func (worker *paddleCheckoutDelivery) retainOutcome(ctx context.Context, job paymentCheckoutJob, state, reason string) error {
	// The caller can be canceled after Paddle accepted a POST. Persist that
	// uncertainty with a bounded independent context before returning.
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), journalPersistenceTimeout)
	defer cancel()
	result := worker.database.database.WithContext(persist).Model(&managedPaymentDeliveryRecord{}).Where("order_id = ? AND owner_token = ? AND state = ?", job.order.ID, job.delivery.OwnerToken, paymentDeliveryDispatching).Updates(map[string]any{"state": state, "reason": reason, "retry_at": worker.now().UTC().Add(paymentCheckoutRetry)})
	if result.Error != nil {
		return fmt.Errorf("retain checkout outcome %s: %w", job.order.ID, result.Error)
	}
	if result.RowsAffected != 1 {
		return errFundingConflict
	}
	return nil
}

func (worker *paddleCheckoutDelivery) complete(ctx context.Context, job paymentCheckoutJob, transactionID string) error {
	return worker.database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&managedPaymentDeliveryRecord{}).Where("order_id = ? AND owner_token = ? AND state = ?", job.order.ID, job.delivery.OwnerToken, paymentDeliveryDispatching).Updates(map[string]any{"state": paymentDeliveryDelivered, "reason": ""})
		if result.Error != nil {
			return fmt.Errorf("complete checkout delivery: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return errFundingConflict
		}
		record := managedPaymentCheckoutRecord{OrderID: job.order.ID, Environment: job.order.Environment, ProcessorAccountID: job.order.ProcessorAccountID, TransactionID: transactionID, CustomerID: job.delivery.CustomerID, CreatedAt: worker.now().UTC()}
		if err := tx.Omit(clause.Associations).Create(&record).Error; err != nil {
			return fmt.Errorf("retain checkout transaction: %w", err)
		}
		return tx.Model(&managedFundingOrderRecord{}).Where("id = ? AND state = ?", job.order.ID, fundingOrderCreated).Update("state", "pending").Error
	})
}

type managementPaymentCheckoutResponse struct {
	Provider      string `json:"provider"`
	Environment   string `json:"environment"`
	TransactionID string `json:"transaction_id"`
}

func (database *gormManagedTenantDatabase) paymentCheckout(ctx context.Context, accountID, orderID string) (managementPaymentCheckoutResponse, error) {
	var record managedPaymentCheckoutRecord
	err := database.database.WithContext(ctx).Joins("JOIN managed_funding_order_records AS funding ON funding.id = managed_payment_checkout_records.order_id").Where("funding.billing_account_id = ? AND funding.id = ?", accountID, orderID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return managementPaymentCheckoutResponse{}, errFundingNotFound
	}
	if err != nil {
		return managementPaymentCheckoutResponse{}, fmt.Errorf("read checkout %s: %w", orderID, err)
	}
	return managementPaymentCheckoutResponse{Provider: "paddle", Environment: record.Environment, TransactionID: record.TransactionID}, nil
}

func (service *managementService) paymentCheckoutHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		if ctx.Request.URL.RawQuery != "" {
			writeFundingError(ctx, errFundingInvalid)
			return
		}
		response, err := service.store.database.paymentCheckout(ctx.Request.Context(), account.ID, ctx.Param("order_id"))
		if err != nil {
			writeFundingError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}
