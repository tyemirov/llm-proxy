package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	managementPaymentReceiptPath        = managementFundingOrderPath + "/receipt"
	managementPaymentPortalSessionsPath = managementBillingAccountPath + "/payment-portal-sessions"
	paymentPortalRequestMaximumBytes    = 1024
)

type paddlePortalClient interface {
	CreateCustomerPortalURL(context.Context, string) (string, error)
}

type managementPaymentReceiptResponse struct {
	FundingOrderID     string  `json:"funding_order_id"`
	Environment        string  `json:"environment"`
	Currency           string  `json:"currency"`
	State              string  `json:"state"`
	CreditCents        string  `json:"credit_cents"`
	GrossCents         string  `json:"gross_cents"`
	TaxCents           string  `json:"tax_cents"`
	AdjustedGrossCents string  `json:"adjusted_gross_cents"`
	AdjustedTaxCents   string  `json:"adjusted_tax_cents"`
	ReversedCents      string  `json:"reversed_cents"`
	PendingRefundCents string  `json:"pending_refund_cents"`
	InvoiceNumber      *string `json:"invoice_number"`
	PaidAt             string  `json:"paid_at"`
}

func (database *gormManagedTenantDatabase) paymentReceipt(ctx context.Context, accountID, orderID string) (managementPaymentReceiptResponse, error) {
	var response managementPaymentReceiptResponse
	err := database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var receipt managedPaymentReceiptRecord
		if err := tx.Where("order_id = ? AND billing_account_id = ?", orderID, accountID).First(&receipt).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errFundingNotFound
			}
			return err
		}
		var order managedFundingOrderRecord
		if err := tx.Where("id = ? AND billing_account_id = ?", orderID, accountID).First(&order).Error; err != nil {
			return err
		}
		var adjustment managedPaymentAdjustmentRecord
		if err := tx.Where("order_id = ? AND billing_account_id = ?", orderID, accountID).First(&adjustment).Error; err != nil {
			return fmt.Errorf("read receipt adjustment: %w", err)
		}
		var evidence paymentAdjustmentEvidence
		if err := json.Unmarshal([]byte(adjustment.Evidence), &evidence); err != nil {
			return fmt.Errorf("decode retained receipt adjustment: %w", err)
		}
		if evidence.Totals == nil {
			return fmt.Errorf("missing retained receipt adjustment totals for %s", orderID)
		}
		response = managementPaymentReceiptResponse{FundingOrderID: receipt.OrderID, Environment: receipt.Environment, Currency: receipt.Currency, State: order.State, CreditCents: strconv.FormatInt(receipt.CreditCents, 10), GrossCents: receipt.GrossCents, TaxCents: receipt.TaxCents, ReversedCents: strconv.FormatInt(adjustment.ReversedCents, 10), PendingRefundCents: strconv.FormatInt(adjustment.PendingCents, 10), InvoiceNumber: receipt.InvoiceNumber, PaidAt: receipt.CompletedAt.UTC().Format(time.RFC3339Nano)}
		response.AdjustedGrossCents, response.AdjustedTaxCents = evidence.Totals.Total, evidence.Totals.Tax
		return nil
	})
	if errors.Is(err, errFundingNotFound) {
		return response, errFundingNotFound
	}
	if err != nil {
		return response, fmt.Errorf("read payment receipt for %s: %w", orderID, err)
	}
	return response, nil
}

func (service *managementService) paymentReceiptHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		if ctx.Request.URL.RawQuery != "" {
			writeFundingError(ctx, errFundingInvalid)
			return
		}
		receipt, err := service.store.database.paymentReceipt(ctx.Request.Context(), account.ID, ctx.Param("order_id"))
		if err != nil {
			writeFundingError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, receipt)
	}
}

func (database *gormManagedTenantDatabase) paymentCustomer(ctx context.Context, accountID, environment, processorAccountID string) (string, error) {
	var customer managedPaymentCustomerRecord
	err := database.database.WithContext(ctx).Where("billing_account_id = ? AND environment = ? AND processor_account_id = ?", accountID, environment, processorAccountID).First(&customer).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", errFundingNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read payment customer: %w", err)
	}
	return customer.CustomerID, nil
}

func (service *managementService) createPaymentPortalSessionHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		payload, readError := io.ReadAll(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, paymentPortalRequestMaximumBytes))
		var input map[string]json.RawMessage
		if err := json.Unmarshal(payload, &input); readError != nil || err != nil || input == nil || len(input) != 0 || ctx.Request.URL.RawQuery != "" {
			writeFundingError(ctx, errFundingInvalid)
			return
		}
		if service.funding == nil || service.paymentPortal == nil {
			writeFundingError(ctx, errFundingUnavailable)
			return
		}
		customerID, err := service.store.database.paymentCustomer(ctx.Request.Context(), account.ID, service.funding.environment, service.funding.processorAccountID)
		if err != nil {
			writeFundingError(ctx, err)
			return
		}
		attempt, cancel := context.WithTimeout(ctx.Request.Context(), paymentCheckoutRetry)
		defer cancel()
		portalURL, err := service.paymentPortal.CreateCustomerPortalURL(attempt, customerID)
		if err != nil {
			writeFundingError(ctx, errFundingUnavailable)
			return
		}
		parsed, err := url.Parse(portalURL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
			writeFundingError(ctx, errFundingUnavailable)
			return
		}
		ctx.Header("Location", portalURL)
		ctx.JSON(http.StatusCreated, gin.H{"provider": "paddle", "environment": service.funding.environment, "url": portalURL})
	}
}
