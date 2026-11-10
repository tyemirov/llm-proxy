package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var errApplicationFinancialNotFound = errors.New(llmproxycontract.ErrorCodeFinancialRequestNotFound)

type applicationSettlementReceipt struct {
	ID                  string     `json:"id"`
	Revision            uint64     `json:"revision"`
	ExactCustomerCharge ExactMoney `json:"exact_customer_charge"`
	PostedCents         string     `json:"posted_cents"`
	SettledAt           string     `json:"settled_at"`
}
type applicationFinancialQuoteReference struct {
	ID               string `json:"id"`
	ProcessingDigest string `json:"processing_digest"`
}
type applicationFinancialRequest struct {
	Quote               *applicationFinancialQuoteReference `json:"quote,omitempty"`
	RequestID           string                              `json:"request_id"`
	ReservationID       string                              `json:"reservation_id"`
	ExecutionID         string                              `json:"execution_id"`
	ExecutionKind       journalExecutionKind                `json:"execution_kind"`
	RequestDigest       string                              `json:"request_digest"`
	PricingRevision     string                              `json:"pricing_revision"`
	PriceSnapshotDigest string                              `json:"price_snapshot_digest"`
	Currency            string                              `json:"currency"`
	MaximumCents        string                              `json:"maximum_cents"`
	ReservationState    string                              `json:"reservation_state"`
	ReservationRevision uint64                              `json:"reservation_revision"`
	ExecutionState      journalRequestState                 `json:"execution_state"`
	State               string                              `json:"state"`
	Receipt             *applicationSettlementReceipt       `json:"receipt"`
	UpdatedAt           string                              `json:"updated_at"`
}

// Financial reads use tenant authority, not management account ownership.
// They never invoke admission, delivery, reconciliation, or provider execution.
func registerApplicationFinancialRoutes(router *gin.Engine, auth tenantAuthenticator, logger *zap.SugaredLogger, database *gormManagedTenantDatabase) {
	for _, path := range []string{llmproxycontract.FinancialRequestsPath, llmproxycontract.FinancialRequestsPath + "/:request_id"} {
		noStore := func(ctx *gin.Context) {
			ctx.Header(headerCacheControl, cacheControlNoStore)
			ctx.Header("Vary", "Authorization")
		}
		router.GET(path, noStore, mediaTenantAuthenticatedHandler(auth, logger, applicationFinancialRequestHandler(database)))
	}
}
func applicationFinancialRequestHandler(database *gormManagedTenantDatabase) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if ctx.Request.URL.RawQuery != "" || ctx.Request.ContentLength != 0 {
			applicationFinancialError(ctx, http.StatusBadRequest, llmproxycontract.ErrorCodeFinancialRequestInvalid)
			return
		}
		requestID := ctx.Param("request_id")
		keys := ctx.Request.Header.Values(llmproxycontract.HeaderIdempotencyKey)
		if requestID != "" {
			if !strings.HasPrefix(requestID, journalRequestIDPrefix) || len(requestID) > 200 || len(keys) != 0 {
				applicationFinancialError(ctx, http.StatusBadRequest, llmproxycontract.ErrorCodeFinancialRequestInvalid)
				return
			}
		} else if len(keys) != 1 || !validIdempotencyKey(keys[0]) {
			applicationFinancialError(ctx, http.StatusBadRequest, llmproxycontract.ErrorCodeFinancialRequestInvalid)
			return
		}
		if database == nil {
			applicationFinancialError(ctx, http.StatusServiceUnavailable, llmproxycontract.ErrorCodeFinancialStatusUnavailable)
			return
		}
		identity := authenticatedTenantFromContext(ctx)
		var response applicationFinancialRequest
		err := database.database.WithContext(ctx.Request.Context()).Transaction(func(transaction *gorm.DB) error {
			query := transaction.Where("tenant_id = ?", identity.identifier.string())
			if requestID != "" {
				query = query.Where("id = ?", requestID)
			} else {
				query = query.Where("key_digest = ?", sha256Hex(keys[0]))
			}
			var request managedJournalRequestRecord
			if err := query.First(&request).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errApplicationFinancialNotFound
				}
				return err
			}
			var reservation managedFundsReservationRecord
			if err := transaction.Where("request_id = ? AND billing_account_id = ?", request.ID, request.BillingAccountID).First(&reservation).Error; err != nil {
				return fmt.Errorf("read admitted reservation: %w", err)
			}
			var price managedPriceSnapshotRecord
			if err := transaction.Where("request_id = ? AND billing_account_id = ?", request.ID, request.BillingAccountID).First(&price).Error; err != nil {
				return fmt.Errorf("read retained price: %w", err)
			}
			response = applicationFinancialRequest{RequestID: request.ID, ReservationID: reservation.RequestID, ExecutionID: request.ExecutionID, ExecutionKind: request.ExecutionKind, RequestDigest: request.IntentDigest, PricingRevision: request.CatalogRevision, PriceSnapshotDigest: price.Digest, Currency: reservation.Currency, MaximumCents: strconv.FormatInt(reservation.MaximumCents, 10), ReservationState: reservation.State, ReservationRevision: reservation.Revision, ExecutionState: request.State, State: "pending", UpdatedAt: reservation.UpdatedAt.UTC().Format(time.RFC3339Nano)}
			var quote managedFinancialQuoteRecord
			quoteError := transaction.First(&quote, "request_id = ?", request.ID).Error
			if quoteError == nil {
				if quote.TenantID != request.TenantID || quote.RequestDigest != request.IntentDigest {
					return fmt.Errorf("financial quote identity differs")
				}
				response.Quote = &applicationFinancialQuoteReference{ID: quote.ID, ProcessingDigest: quote.ProcessingDigest}
				if quote.State != quoteConsumed {
					response.ExecutionID = ""
				}
			} else if !errors.Is(quoteError, gorm.ErrRecordNotFound) {
				return fmt.Errorf("read financial quote identity: %w", quoteError)
			}

			switch reservation.State {
			case fundsReservationSettled:
				var settlement managedFundsSettlementRecord
				if err := transaction.Where("request_id = ? AND billing_account_id = ?", request.ID, request.BillingAccountID).First(&settlement).Error; err != nil {
					return fmt.Errorf("read immutable settlement: %w", err)
				}
				response.State = "settled"
				response.Receipt = &applicationSettlementReceipt{ID: settlement.RequestID, Revision: 1, ExactCustomerCharge: ExactMoney{Numerator: settlement.ChargeNumerator, Denominator: settlement.ChargeDenominator}, PostedCents: strconv.FormatInt(settlement.SettledCents, 10), SettledAt: settlement.CreatedAt.UTC().Format(time.RFC3339Nano)}
			case fundsReservationReleased:
				var dispatched int64
				if err := transaction.Model(&managedJournalAttemptRecord{}).Where("request_id = ? AND (state <> ? OR dispatch_at IS NOT NULL)", request.ID, journalAttemptPrepared).Count(&dispatched).Error; err != nil {
					return fmt.Errorf("read retained dispatch evidence: %w", err)
				}
				if dispatched != 0 || request.State != journalRequestFailed {
					return fmt.Errorf("released reservation lacks final non-dispatch evidence")
				}
				response.State = "definitely_undispatched_released"
			case fundsReservationReconciliation:
				response.State = "uncertain"
			case fundsReservationHeld:
				if request.State == journalRequestUncertain || request.UsageState == journalUsageUnknown {
					response.State = "uncertain"
				}
			default:
				return fmt.Errorf("unknown reservation state %q", reservation.State)
			}
			return nil
		})
		if errors.Is(err, errApplicationFinancialNotFound) {
			applicationFinancialError(ctx, http.StatusNotFound, llmproxycontract.ErrorCodeFinancialRequestNotFound)
			return
		}
		if err != nil {
			applicationFinancialError(ctx, http.StatusServiceUnavailable, llmproxycontract.ErrorCodeFinancialStatusUnavailable)
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}
func applicationFinancialError(ctx *gin.Context, status int, code string) {
	ctx.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": map[string]string{llmproxycontract.ErrorCodeFinancialRequestInvalid: "The financial request identity is invalid.", llmproxycontract.ErrorCodeFinancialRequestNotFound: "The financial request was not found.", llmproxycontract.ErrorCodeFinancialStatusUnavailable: "The hosted financial status is unavailable."}[code]}})
}
