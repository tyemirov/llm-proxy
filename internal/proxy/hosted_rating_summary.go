package proxy

import (
	"context"
	"fmt"
	"math/big"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const managementRequestChargeSummaryPath = managementJournalRequestPath + "/charge-summary"

type requestChargeSummaryState string

const (
	requestChargePending    requestChargeSummaryState = "pending"
	requestChargeUnresolved requestChargeSummaryState = "unresolved"
	requestChargeRated      requestChargeSummaryState = "rated"
)

type requestChargeSummary struct {
	knownProviderCost       ExactMoney
	knownCustomerQuote      ExactMoney
	retainedCustomerCredits ExactMoney
	providerCostComplete    bool
	RequestID               string                    `json:"request_id"`
	State                   requestChargeSummaryState `json:"state"`
	AttemptCount            int64                     `json:"attempt_count"`
	ChargeCount             int64                     `json:"charge_count"`
	ProviderCost            *ExactMoney               `json:"provider_cost"`
	CustomerCharge          *ExactMoney               `json:"customer_charge"`
	CustomerCredits         *ExactMoney               `json:"customer_credits"`
	NetCustomerCharge       *ExactMoney               `json:"net_customer_charge"`
}

// One SQLite read transaction gives the request, attempts, charges, and credits
// one snapshot. Charge pages limit the number of attempts held in memory.
func (database *gormManagedTenantDatabase) billingRequestChargeSummary(ctx context.Context, accountID, requestID string) (requestChargeSummary, error) {
	var summary requestChargeSummary
	err := database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		reader := &gormManagedTenantDatabase{database: transaction}
		request, err := reader.journalRequest(ctx, accountID, requestID)
		if err != nil {
			return err
		}
		summary.RequestID = request.ID
		if err := transaction.Model(&managedJournalAttemptRecord{}).Where("request_id = ?", request.ID).Count(&summary.AttemptCount).Error; err != nil {
			return fmt.Errorf("count attempts for request %s: %w", request.ID, err)
		}
		provider, customer, net, quoted := new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat)
		providerComplete, customerComplete := true, true
		const batchSize = 100
		cursor := ""
		for {
			var records []managedChargeRecord
			if err := transaction.Where("billing_account_id = ? AND request_id = ? AND id > ?", accountID, request.ID, cursor).Order("id").Limit(batchSize).Find(&records).Error; err != nil {
				return fmt.Errorf("read charges for request %s: %w", request.ID, err)
			}
			ids := make([]string, 0, len(records))
			for _, record := range records {
				ids = append(ids, record.ID)
			}
			adjustments, err := reader.billingChargeAdjustments(ctx, accountID, ids)
			if err != nil {
				return err
			}
			byCharge := map[string][]managedChargeAdjustmentRecord{}
			for _, adjustment := range adjustments {
				byCharge[adjustment.ChargeID] = append(byCharge[adjustment.ChargeID], adjustment)
			}
			for _, record := range records {
				charge, err := chargeResponse(record, byCharge[record.ID])
				if err != nil {
					return fmt.Errorf("read charge %s for request %s: %w", record.ID, request.ID, err)
				}
				for _, amount := range []struct {
					total *big.Rat
					value *ExactMoney
				}{{provider, charge.Rating.ProviderCost}, {customer, charge.CustomerCharge}, {net, charge.NetCustomerCharge}, {quoted, charge.Rating.CustomerCharge}} {
					if amount.value == nil {
						continue
					}
					value, err := parseExactMoney(*amount.value)
					if err != nil {
						return fmt.Errorf("read amount for charge %s: %w", record.ID, err)
					}
					amount.total.Add(amount.total, value)
				}
				providerComplete = providerComplete && charge.Rating.ProviderCost != nil
				customerComplete = customerComplete && charge.CustomerCharge != nil
				summary.ChargeCount++
				cursor = record.ID
			}
			if len(records) < batchSize {
				break
			}
		}
		if summary.ChargeCount > summary.AttemptCount {
			return fmt.Errorf("request %s has more charges than attempts", request.ID)
		}
		summary.knownProviderCost = ratingMoney(provider)
		summary.knownCustomerQuote = ratingMoney(quoted)
		summary.retainedCustomerCredits = ratingMoney(new(big.Rat).Sub(customer, net))
		summary.providerCostComplete = request.State != journalRequestAccepted && request.State != journalRequestExecuting && summary.ChargeCount == summary.AttemptCount && providerComplete
		switch request.State {
		case journalRequestAccepted, journalRequestExecuting:
			summary.State = requestChargePending
		case journalRequestFailed, journalRequestUncertain:
			summary.State = requestChargeUnresolved
		case journalRequestCompleted:
			switch {
			case request.ResultPublishedAt == nil || summary.ChargeCount < summary.AttemptCount:
				summary.State = requestChargePending
			case !customerComplete:
				summary.State = requestChargeUnresolved
			default:
				summary.State = requestChargeRated
			}
		default:
			return fmt.Errorf("request %s has an invalid journal state", request.ID)
		}
		if summary.State != requestChargePending && summary.ChargeCount == summary.AttemptCount && providerComplete {
			cost := ratingMoney(provider)
			summary.ProviderCost = &cost
		}
		if summary.State == requestChargeRated {
			gross, credits, adjusted := ratingMoney(customer), ratingMoney(new(big.Rat).Sub(customer, net)), ratingMoney(net)
			summary.CustomerCharge, summary.CustomerCredits, summary.NetCustomerCharge = &gross, &credits, &adjusted
		}
		return nil
	})
	if err != nil {
		return requestChargeSummary{}, fmt.Errorf("read charge summary for request %s: %w", requestID, err)
	}
	return summary, nil
}

func (service *managementService) getRequestChargeSummaryHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		summary, err := service.store.database.billingRequestChargeSummary(ctx.Request.Context(), account.ID, ctx.Param("request_id"))
		if err != nil {
			writeUsageJournalError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, summary)
	}
}
