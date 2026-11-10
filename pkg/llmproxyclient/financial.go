package llmproxyclient

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

var financialDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var financialIntegerPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
var financialRequestIDPattern = regexp.MustCompile(`^request-[a-f0-9]{32}$`)

// FinancialExactMoney retains a rational amount in the response currency.
type FinancialExactMoney struct {
	Numerator   string `json:"numerator"`
	Denominator string `json:"denominator"`
}

// FinancialSettlementReceipt is the immutable original settlement, before later corrections.
// PostedCents includes account fractional accounting; it is not rounded request cost.
type FinancialSettlementReceipt struct {
	ID                  string              `json:"id"`
	Revision            uint64              `json:"revision"`
	ExactCustomerCharge FinancialExactMoney `json:"exact_customer_charge"`
	PostedCents         string              `json:"posted_cents"`
	SettledAt           string              `json:"settled_at"`
}

// FinancialRequest is a tenant-owned hosted financial snapshot without execution payloads.
type FinancialRequest struct {
	RequestID           string                      `json:"request_id"`
	ReservationID       string                      `json:"reservation_id"`
	ExecutionID         string                      `json:"execution_id"`
	ExecutionKind       string                      `json:"execution_kind"`
	RequestDigest       string                      `json:"request_digest"`
	PricingRevision     string                      `json:"pricing_revision"`
	PriceSnapshotDigest string                      `json:"price_snapshot_digest"`
	Currency            string                      `json:"currency"`
	MaximumCents        string                      `json:"maximum_cents"`
	ReservationState    string                      `json:"reservation_state"`
	ReservationRevision uint64                      `json:"reservation_revision"`
	ExecutionState      string                      `json:"execution_state"`
	State               string                      `json:"state"`
	Receipt             *FinancialSettlementReceipt `json:"receipt"`
	UpdatedAt           string                      `json:"updated_at"`
}

// GetFinancialRequest reads an existing hosted intent without dispatch or reconciliation.
func (client Client) GetFinancialRequest(ctx context.Context, key string) (FinancialRequest, error) {
	if !validClientIdempotencyKey(key) {
		return FinancialRequest{}, fmt.Errorf("%w: invalid financial idempotency key", ErrInvalidClientRequest)
	}
	return client.readFinancialRequest(ctx, llmproxycontract.FinancialRequestsPath, key)
}

// GetFinancialRequestByID reads retained evidence after execution payload deletion.
func (client Client) GetFinancialRequestByID(ctx context.Context, id string) (FinancialRequest, error) {
	if !financialRequestIDPattern.MatchString(id) {
		return FinancialRequest{}, fmt.Errorf("%w: invalid financial request ID", ErrInvalidClientRequest)
	}
	result, err := client.readFinancialRequest(ctx, llmproxycontract.FinancialRequestsPath+"/"+id, "")
	if err != nil {
		return FinancialRequest{}, err
	}
	if result.RequestID != id {
		return FinancialRequest{}, fmt.Errorf("%w: financial response belongs to another request", ErrClientHTTPFailure)
	}
	return result, nil
}
func (client Client) readFinancialRequest(ctx context.Context, path, key string) (FinancialRequest, error) {
	requestURL := client.config.mediaResourceURL(path)
	request := (&http.Request{Method: http.MethodGet, URL: &requestURL, Header: http.Header{}}).WithContext(ctx)
	request.Header.Set(headerAccept, "application/json")
	request.Header.Set("Authorization", "Bearer "+client.config.secret)
	if key != "" {
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	}
	body, err := client.doMediaResource(request, http.StatusOK)
	if err != nil {
		return FinancialRequest{}, err
	}
	var result FinancialRequest
	if err := decodeExactJSON(body, &result); err != nil || !validFinancialRequest(result) {
		return FinancialRequest{}, fmt.Errorf("%w: invalid financial response", ErrClientHTTPFailure)
	}
	return result, nil
}
func validFinancialRequest(value FinancialRequest) bool {
	if !financialRequestIDPattern.MatchString(value.RequestID) || value.ReservationID != value.RequestID || value.ExecutionID == "" || value.PricingRevision == "" || value.Currency != "USD" || value.ReservationRevision == 0 || !financialDigestPattern.MatchString(value.RequestDigest) || !financialDigestPattern.MatchString(value.PriceSnapshotDigest) || !financialIntegerPattern.MatchString(value.MaximumCents) {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, value.UpdatedAt); err != nil {
		return false
	}
	if !financialEnum(value.ExecutionKind, "text_request", "media_operation", "dictation_request") || !financialEnum(value.ExecutionState, "accepted", "executing", "completed", "failed", "uncertain") {
		return false
	}
	if !financialEnum(value.State, "pending", "uncertain", "settled", "definitely_undispatched_released") || !financialEnum(value.ReservationState, "held", "settled", "released", "reconciliation_required") {
		return false
	}
	if value.State != "settled" {
		return value.Receipt == nil && (value.State == "pending" && value.ReservationState == "held" || value.State == "uncertain" && (value.ReservationState == "held" || value.ReservationState == "reconciliation_required") || value.State == "definitely_undispatched_released" && value.ReservationState == "released" && value.ExecutionState == "failed")
	}
	receipt := value.Receipt
	if receipt == nil || value.ReservationState != "settled" || receipt.ID != value.RequestID || receipt.Revision != 1 || !financialIntegerPattern.MatchString(receipt.PostedCents) || !financialIntegerPattern.MatchString(receipt.ExactCustomerCharge.Numerator) || !financialIntegerPattern.MatchString(receipt.ExactCustomerCharge.Denominator) {
		return false
	}
	denominator, ok := new(big.Int).SetString(receipt.ExactCustomerCharge.Denominator, 10)
	if !ok || denominator.Sign() <= 0 {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, receipt.SettledAt)
	return err == nil
}
func financialEnum(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}
