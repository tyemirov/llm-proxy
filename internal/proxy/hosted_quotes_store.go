package proxy

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errQuoteInvalid = errors.New(llmproxycontract.ErrorCodeFinancialQuoteInvalid)
var errQuoteNotFound = errors.New(llmproxycontract.ErrorCodeFinancialQuoteNotFound)
var errQuoteConflict = errors.New(llmproxycontract.ErrorCodeFinancialQuoteConflict)
var errQuoteExpired = errors.New(llmproxycontract.ErrorCodeFinancialQuoteExpired)
var errQuoteUnavailable = errors.New(llmproxycontract.ErrorCodeFinancialQuoteUnavailable)

func quotedResponseKey(id string) string { return "\x00quoted-execution\x00" + id }

type financialQuoteResponse struct {
	ExecutionKind       string `json:"execution_kind"`
	ID                  string `json:"id"`
	Revision            uint64 `json:"revision"`
	State               string `json:"state"`
	ProcessingDigest    string `json:"processing_digest"`
	RequestDigest       string `json:"request_digest"`
	RequestID           string `json:"request_id"`
	ReservationID       string `json:"reservation_id"`
	PricingRevision     string `json:"pricing_revision"`
	PriceSnapshotDigest string `json:"price_snapshot_digest"`
	Currency            string `json:"currency"`
	MaximumCents        string `json:"maximum_cents"`
	ExpiresAt           string `json:"expires_at"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
	ExecutionID         string `json:"execution_id"`
}

func quoteResponse(tx *gorm.DB, quote managedFinancialQuoteRecord) (financialQuoteResponse, error) {
	var request managedJournalRequestRecord
	var reservation managedFundsReservationRecord
	var price managedPriceSnapshotRecord
	if err := tx.First(&request, "id = ? AND tenant_id = ?", quote.RequestID, quote.TenantID).Error; err != nil {
		return financialQuoteResponse{}, fmt.Errorf("read quote journal: %w", err)
	}
	if err := tx.First(&reservation, "request_id = ?", quote.RequestID).Error; err != nil {
		return financialQuoteResponse{}, fmt.Errorf("read quote hold: %w", err)
	}
	if err := tx.First(&price, "request_id = ?", quote.RequestID).Error; err != nil {
		return financialQuoteResponse{}, fmt.Errorf("read quote price: %w", err)
	}
	executionID := ""
	if quote.State == quoteConsumed {
		executionID = request.ExecutionID
	}
	return financialQuoteResponse{ExecutionKind: quote.Kind, ID: quote.ID, Revision: quote.Revision, State: quote.State, ProcessingDigest: quote.ProcessingDigest, RequestDigest: quote.RequestDigest, RequestID: quote.RequestID, ReservationID: reservation.RequestID, PricingRevision: request.CatalogRevision, PriceSnapshotDigest: price.Digest, Currency: reservation.Currency, MaximumCents: strconv.FormatInt(reservation.MaximumCents, 10), ExpiresAt: quote.ExpiresAt.UTC().Format(time.RFC3339Nano), CreatedAt: quote.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: quote.UpdatedAt.UTC().Format(time.RFC3339Nano), ExecutionID: executionID}, nil
}
func readOwnedQuote(tx *gorm.DB, tenantID, id, key string) (managedFinancialQuoteRecord, error) {
	var quote managedFinancialQuoteRecord
	query := tx.Where("tenant_id = ?", tenantID)
	if id != "" {
		query = query.Where("id = ?", id)
	} else {
		query = query.Where("key_digest = ?", sha256Hex(key))
	}
	if err := query.First(&quote).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return quote, errQuoteNotFound
		}
		return quote, fmt.Errorf("read financial quote: %w", err)
	}
	return quote, nil
}
func (runtime *applicationQuoteRuntime) createQuote(ctx context.Context, identity tenant, key string, input financialQuoteInput, prepared preparedFinancialIntent) (financialQuoteResponse, bool, error) {
	var response financialQuoteResponse
	created := false
	now := runtime.now().UTC()
	err := runtime.database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockProviderAssignmentTenant(tx, identity.userID, identity.identifier.string()); err != nil {
			return err
		}
		prior, err := readOwnedQuote(tx, identity.identifier.string(), "", key)
		if err == nil {
			if prior.ProcessingDigest != input.ProcessingDigest || prior.RequestDigest != prepared.digest {
				return errQuoteConflict
			}
			response, err = quoteResponse(tx, prior)
			return err
		}
		if !errors.Is(err, errQuoteNotFound) {
			return err
		}
		id, err := newHostedResourceID(quoteIDPrefix, rand.Reader)
		if err != nil {
			return err
		}
		proposal, err := newJournalAdmissionIntent(journalAdmissionInput{OwnerUserID: identity.userID, TenantID: identity.identifier.string(), Provider: prepared.provider, Model: prepared.model, Operation: prepared.operation, CatalogRevision: prepared.catalog, IdempotencyKey: id, CanonicalIntent: prepared.canonical, ExecutionKind: prepared.executionKind, ExecutionID: id, OwnerToken: id, Now: now, ClaimExpiresAt: now.Add(quoteLifetime)}, rand.Reader)
		if err != nil {
			return err
		}
		request := proposal.record
		request.State = journalRequestQuoteReserved
		request.KeyDigest = sha256Hex(quotedResponseKey(id))
		request.IntentDigest = prepared.digest
		if err := bindJournalAuthority(tx, identity.userID, &request); err != nil {
			return err
		}
		if err := tx.Omit(clause.Associations).Create(&request).Error; err != nil {
			return fmt.Errorf("create quoted journal: %w", err)
		}
		if err := prepared.reserve(tx, request); err != nil {
			return err
		}
		quote := managedFinancialQuoteRecord{ID: id, TenantID: identity.identifier.string(), KeyDigest: sha256Hex(key), RequestID: request.ID, Kind: input.Intent.Kind, ProcessingDigest: input.ProcessingDigest, RequestDigest: prepared.digest, State: quoteReserved, Revision: 1, ExpiresAt: now.Add(quoteLifetime), CreatedAt: now, UpdatedAt: now}
		if err := tx.Omit(clause.Associations).Create(&quote).Error; err != nil {
			return fmt.Errorf("retain financial quote: %w", err)
		}
		response, err = quoteResponse(tx, quote)
		created = true
		return err
	})
	return response, created, err
}
func releaseQuote(tx *gorm.DB, quote managedFinancialQuoteRecord, state string, now time.Time) error {
	if quote.State != quoteReserved {
		return errQuoteConflict
	}
	lock := lockManagedReservationRequest(tx, quote.RequestID)
	if lock.Error != nil {
		return fmt.Errorf("lock quote release: %w", lock.Error)
	}
	var request managedJournalRequestRecord
	var reservation managedFundsReservationRecord
	if err := tx.First(&request, "id = ?", quote.RequestID).Error; err != nil {
		return err
	}
	if request.State != journalRequestQuoteReserved {
		return errQuoteConflict
	}
	if err := tx.First(&reservation, "request_id = ?", quote.RequestID).Error; err != nil {
		return err
	}
	if reservation.State != fundsReservationHeld {
		return errQuoteConflict
	}
	var attempts int64
	if err := tx.Model(&managedJournalAttemptRecord{}).Where("request_id = ?", quote.RequestID).Count(&attempts).Error; err != nil {
		return err
	}
	if attempts != 0 {
		return errQuoteConflict
	}
	if err := postHostedFundsSettlement(tx, reservation, 0, now); err != nil {
		return err
	}
	if err := updateHostedFundsReservation(tx, reservation, fundsReservationReleased, now); err != nil {
		return err
	}
	if err := tx.Model(&request).Updates(map[string]any{"state": journalRequestFailed, "failure_code": "quote_" + state, "updated_at": now}).Error; err != nil {
		return err
	}
	result := tx.Model(&managedFinancialQuoteRecord{}).Where("id = ? AND state = ? AND revision = ?", quote.ID, quoteReserved, quote.Revision).Updates(map[string]any{"state": state, "revision": quote.Revision + 1, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errQuoteConflict
	}
	return nil
}
func (database *gormManagedTenantDatabase) expireFinancialQuotes(ctx context.Context, now time.Time) error {
	for {
		var quotes []managedFinancialQuoteRecord
		if err := database.database.WithContext(ctx).Where("state = ? AND expires_at <= ?", quoteReserved, now).Order("id").Limit(fundsRecoveryBatchSize).Find(&quotes).Error; err != nil {
			return fmt.Errorf("read expired financial quotes: %w", err)
		}
		for _, quote := range quotes {
			err := database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				lock := lockManagedReservationRequest(tx, quote.RequestID)
				if lock.Error != nil {
					return lock.Error
				}
				current, err := readOwnedQuote(tx, quote.TenantID, quote.ID, "")
				if err != nil {
					return err
				}
				if current.State != quoteReserved {
					return nil
				}
				return releaseQuote(tx, current, quoteExpired, now)
			})
			if err != nil {
				return fmt.Errorf("expire financial quote: %w", err)
			}
		}
		if len(quotes) < fundsRecoveryBatchSize {
			return nil
		}
	}
}

type quotedExecutionContextKey struct{}
type quotedExecutionIdentity struct{ quote managedFinancialQuoteRecord }

func (database *gormManagedTenantDatabase) consumeFinancialQuote(tx *gorm.DB, identity quotedExecutionIdentity, proposal managedJournalRequestRecord, now time.Time, authorize journalReservation) (managedJournalRequestRecord, error) {
	lock := lockManagedReservationRequest(tx, identity.quote.RequestID)
	if lock.Error != nil {
		return managedJournalRequestRecord{}, lock.Error
	}
	quote, err := readOwnedQuote(tx, identity.quote.TenantID, identity.quote.ID, "")
	if err != nil {
		return managedJournalRequestRecord{}, err
	}
	if quote.ProcessingDigest != identity.quote.ProcessingDigest || quote.RequestDigest != identity.quote.RequestDigest {
		return managedJournalRequestRecord{}, errQuoteConflict
	}
	var request managedJournalRequestRecord
	if err := tx.First(&request, "id = ? AND tenant_id = ?", quote.RequestID, quote.TenantID).Error; err != nil {
		return request, err
	}
	if quote.State == quoteConsumed {
		return request, nil
	}
	if quote.State != quoteReserved {
		return request, errQuoteConflict
	}
	if !quote.ExpiresAt.After(now) {
		return request, errQuoteExpired
	}
	if err := requireJournalAuthority(tx, request); err != nil {
		return request, err
	}
	if err := authorize(tx, request); err != nil {
		return request, err
	}
	request.State = journalRequestAccepted
	request.OwnerToken = proposal.OwnerToken
	request.ExecutionID = proposal.ExecutionID
	request.ClaimExpiresAt = proposal.ClaimExpiresAt
	request.UpdatedAt = now
	if err := tx.Model(&managedJournalRequestRecord{}).Where("id = ? AND state = ?", request.ID, journalRequestQuoteReserved).Updates(map[string]any{"state": request.State, "owner_token": request.OwnerToken, "execution_id": request.ExecutionID, "claim_expires_at": request.ClaimExpiresAt, "updated_at": now}).Error; err != nil {
		return request, err
	}
	result := tx.Model(&managedFinancialQuoteRecord{}).Where("id = ? AND state = ? AND revision = ?", quote.ID, quoteReserved, quote.Revision).Updates(map[string]any{"state": quoteConsumed, "revision": quote.Revision + 1, "updated_at": now})
	if result.Error != nil {
		return request, result.Error
	}
	if result.RowsAffected != 1 {
		return request, errQuoteConflict
	}
	return request, nil
}
func financialQuoteErrorStatus(err error) int {
	switch {
	case errors.Is(err, errQuoteInvalid):
		return http.StatusBadRequest
	case errors.Is(err, errQuoteNotFound):
		return http.StatusNotFound
	case errors.Is(err, errQuoteConflict), errors.Is(err, errQuoteExpired):
		return http.StatusConflict
	case errors.Is(err, errHostedAuthorityDenied):
		return http.StatusForbidden
	case errors.Is(err, errInsufficientFunds):
		return http.StatusPaymentRequired
	default:
		return http.StatusServiceUnavailable
	}
}
