package llmproxyclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

// FinancialQuoteKind selects one bound execution protocol.
type FinancialQuoteKind string

const (
	FinancialQuoteNative FinancialQuoteKind = "native_v2"
	FinancialQuoteMedia  FinancialQuoteKind = "media_operation"
)

var financialQuoteIDPattern = regexp.MustCompile(`^quote-[a-f0-9]{32}$`)

// FinancialQuoteInput holds a detached, validated request and processing identity.
type FinancialQuoteInput struct {
	payload                     []byte
	processingDigest            string
	kind                        FinancialQuoteKind
	provider, model, capability string
}

func newFinancialQuoteInput(digest string, kind FinancialQuoteKind, provider, model, capability string, request json.RawMessage) (FinancialQuoteInput, error) {
	if !financialDigestPattern.MatchString(digest) || !diagnosticProviderPattern.MatchString(provider) || strings.TrimSpace(model) == "" {
		return FinancialQuoteInput{}, fmt.Errorf("%w: incomplete quoted intent", ErrInvalidClientRequest)
	}
	// Both private callers supply validated JSON objects. These envelope scalars
	// contain only validated ASCII identifiers, so appending them cannot escape JSON.
	prefix := []byte(`{"processing_digest":"` + digest + `","intent":{"kind":"` + string(kind) + `","provider":"` + provider + `","request":`)
	payload := make([]byte, 0, len(prefix)+len(request)+2)
	payload = append(payload, prefix...)
	payload = append(payload, request...)
	payload = append(payload, '}', '}')
	return FinancialQuoteInput{payload: payload, processingDigest: digest, kind: kind, provider: provider, model: model, capability: capability}, nil
}

// NewNativeFinancialQuoteInput freezes an explicit model and structured native request.
// The quote resource supplies execution idempotency and the server work-budget policy.
func NewNativeFinancialQuoteInput(digest, provider string, request MessagesRequest) (FinancialQuoteInput, error) {
	if len(request.messages) == 0 || request.structuredOutput == nil || request.model == "" || request.requestTimeoutSeconds != nil {
		return FinancialQuoteInput{}, fmt.Errorf("%w: quoted native intent requires explicit model and structured output with server work budget", ErrInvalidClientRequest)
	}
	return newFinancialQuoteInput(digest, FinancialQuoteNative, provider, request.model, "", request.payloadBody(request.model))
}

// NewMediaFinancialQuoteInput freezes one supported image-generation or image-edit intent.
func NewMediaFinancialQuoteInput(digest string, request MediaOperationInput) (FinancialQuoteInput, error) {
	if request.Capability != "image.generate" && request.Capability != "image.edit" {
		return FinancialQuoteInput{}, fmt.Errorf("%w: quoted media requires an image intent", ErrInvalidClientRequest)
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return FinancialQuoteInput{}, fmt.Errorf("%w: quoted image intent: %w", ErrInvalidClientRequest, err)
	}
	if !validJSONObject(request.Input) || !validJSONObject(request.Controls) {
		return FinancialQuoteInput{}, fmt.Errorf("%w: quoted media input and controls must be objects", ErrInvalidClientRequest)
	}
	return newFinancialQuoteInput(digest, FinancialQuoteMedia, request.Provider, request.Model, request.Capability, payload)
}

// Payload returns an independent copy of the complete quote request JSON.
func (input FinancialQuoteInput) Payload() ([]byte, error) {
	if len(input.payload) == 0 {
		return nil, fmt.Errorf("%w: invalid quoted input", ErrInvalidClientRequest)
	}
	return append([]byte(nil), input.payload...), nil
}

// FinancialQuoteSnapshot contains retained reservation evidence without the execution payload.
type FinancialQuoteSnapshot struct {
	ID                  string             `json:"id"`
	Revision            uint64             `json:"revision"`
	State               string             `json:"state"`
	ExecutionKind       FinancialQuoteKind `json:"execution_kind"`
	ProcessingDigest    string             `json:"processing_digest"`
	RequestDigest       string             `json:"request_digest"`
	RequestID           string             `json:"request_id"`
	ReservationID       string             `json:"reservation_id"`
	PricingRevision     string             `json:"pricing_revision"`
	PriceSnapshotDigest string             `json:"price_snapshot_digest"`
	Currency            string             `json:"currency"`
	MaximumCents        string             `json:"maximum_cents"`
	ExpiresAt           string             `json:"expires_at"`
	CreatedAt           string             `json:"created_at"`
	UpdatedAt           string             `json:"updated_at"`
	ExecutionID         string             `json:"execution_id"`
}

// FinancialQuote is an immutable validated quote handle. Lookups need no original payload.
type FinancialQuote struct {
	snapshot   FinancialQuoteSnapshot
	boundInput FinancialQuoteInput
	valid      bool
}

// Snapshot returns the retained quote values, detached from the handle.
func (quote FinancialQuote) Snapshot() (FinancialQuoteSnapshot, error) {
	if !quote.valid {
		return FinancialQuoteSnapshot{}, fmt.Errorf("%w: invalid quote handle", ErrInvalidClientRequest)
	}
	return quote.snapshot, nil
}
func validQuoteTime(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	_, offset := parsed.Zone()
	return parsed, err == nil && offset == 0
}
func validFinancialQuote(value FinancialQuoteSnapshot) bool {
	if !financialQuoteIDPattern.MatchString(value.ID) || value.Revision == 0 || !financialEnum(value.State, "reserved", "consumed", "expired", "released") || value.ExecutionKind != FinancialQuoteNative && value.ExecutionKind != FinancialQuoteMedia || !financialDigestPattern.MatchString(value.ProcessingDigest) || !financialDigestPattern.MatchString(value.RequestDigest) || !financialRequestIDPattern.MatchString(value.RequestID) || value.ReservationID != value.RequestID || strings.TrimSpace(value.PricingRevision) == "" || !financialDigestPattern.MatchString(value.PriceSnapshotDigest) || value.Currency != "USD" || !financialIntegerPattern.MatchString(value.MaximumCents) {
		return false
	}
	if _, err := strconv.ParseInt(value.MaximumCents, 10, 64); err != nil {
		return false
	}
	created, createdOK := validQuoteTime(value.CreatedAt)
	updated, updatedOK := validQuoteTime(value.UpdatedAt)
	expires, expiresOK := validQuoteTime(value.ExpiresAt)
	if !createdOK || !updatedOK || !expiresOK || !expires.After(created) || updated.Before(created) {
		return false
	}
	if value.State == "expired" && updated.Before(expires) {
		return false
	}
	if value.State == "consumed" {
		return validClientIdempotencyKey(value.ExecutionID)
	}
	return value.ExecutionID == ""
}
func decodeFinancialQuote(body []byte) (FinancialQuote, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || len(fields) != 16 {
		return FinancialQuote{}, fmt.Errorf("%w: invalid financial quote fields", ErrClientHTTPFailure)
	}
	var snapshot FinancialQuoteSnapshot
	if err := decodeExactJSON(body, &snapshot); err != nil || !validFinancialQuote(snapshot) {
		return FinancialQuote{}, fmt.Errorf("%w: invalid financial quote response", ErrClientHTTPFailure)
	}
	for _, field := range fields {
		if bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
			return FinancialQuote{}, fmt.Errorf("%w: null financial quote field", ErrClientHTTPFailure)
		}
	}
	return FinancialQuote{snapshot: snapshot, valid: true}, nil
}
func (client Client) quotedRequest(ctx context.Context, method, path, key string, body []byte) (*http.Request, error) {
	if client.config.baseURL == nil || client.httpClient == nil {
		return nil, fmt.Errorf("%w: uninitialized client", ErrInvalidClientConfig)
	}
	requestURL := client.config.mediaResourceURL(path)
	request := (&http.Request{Method: method, URL: &requestURL, Header: http.Header{}}).WithContext(ctx)
	request.Header.Set(headerAccept, "application/json")
	request.Header.Set("Authorization", "Bearer "+client.config.secret)
	if body != nil {
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.ContentLength = int64(len(body))
		request.Header.Set(headerContentType, "application/json")
	}
	if key != "" {
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	}
	return request, nil
}

// CreateFinancialQuote reserves a bounded maximum without dispatching execution.
func (client Client) CreateFinancialQuote(ctx context.Context, key string, input FinancialQuoteInput) (FinancialQuote, error) {
	if !validClientIdempotencyKey(key) || len(input.payload) == 0 {
		return FinancialQuote{}, fmt.Errorf("%w: invalid financial quote creation", ErrInvalidClientRequest)
	}
	quote, err := client.quoteResource(ctx, http.MethodPost, llmproxycontract.FinancialQuotesPath, key, input.payload, http.StatusCreated, http.StatusOK)
	if err != nil {
		return FinancialQuote{}, err
	}
	if quote.snapshot.ProcessingDigest != input.processingDigest || quote.snapshot.ExecutionKind != input.kind {
		return FinancialQuote{}, fmt.Errorf("%w: quote belongs to another processing intent", ErrClientHTTPFailure)
	}
	quote.boundInput = input
	return quote, nil
}

// GetFinancialQuote recovers a lost creation response by its original idempotency key.
func (client Client) GetFinancialQuote(ctx context.Context, key string) (FinancialQuote, error) {
	if !validClientIdempotencyKey(key) {
		return FinancialQuote{}, fmt.Errorf("%w: invalid quote key", ErrInvalidClientRequest)
	}
	return client.quoteResource(ctx, http.MethodGet, llmproxycontract.FinancialQuotesPath, key, nil, http.StatusOK)
}

// GetFinancialQuoteByID reads retained quote evidence without original candidate inputs.
func (client Client) GetFinancialQuoteByID(ctx context.Context, id string) (FinancialQuote, error) {
	if !financialQuoteIDPattern.MatchString(id) {
		return FinancialQuote{}, fmt.Errorf("%w: invalid quote ID", ErrInvalidClientRequest)
	}
	quote, err := client.quoteResource(ctx, http.MethodGet, llmproxycontract.FinancialQuotesPath+"/"+id, "", nil, http.StatusOK)
	if err != nil {
		return FinancialQuote{}, err
	}
	if quote.snapshot.ID != id {
		return FinancialQuote{}, fmt.Errorf("%w: quote response belongs to another resource", ErrClientHTTPFailure)
	}
	return quote, nil
}
func (client Client) quoteResource(ctx context.Context, method, path, key string, body []byte, statuses ...int) (FinancialQuote, error) {
	request, err := client.quotedRequest(ctx, method, path, key, body)
	if err != nil {
		return FinancialQuote{}, err
	}
	response, err := client.doMediaResource(request, statuses...)
	if err != nil {
		return FinancialQuote{}, err
	}
	return decodeFinancialQuote(response)
}

// ReleaseFinancialQuote requests release with the observed quote revision.
// A conflict or transport error does not establish release.
func (client Client) ReleaseFinancialQuote(ctx context.Context, quote FinancialQuote) (FinancialQuote, error) {
	if !quote.valid {
		return FinancialQuote{}, fmt.Errorf("%w: invalid quote handle", ErrInvalidClientRequest)
	}
	body := []byte(`{"revision":` + strconv.FormatUint(quote.snapshot.Revision, 10) + `,"state":"released"}`)

	released, err := client.quoteResource(ctx, http.MethodPatch, llmproxycontract.FinancialQuotesPath+"/"+quote.snapshot.ID, "", body, http.StatusOK)
	if err != nil {
		return FinancialQuote{}, err
	}
	before, after := quote.snapshot, released.snapshot
	beforeTime, _ := validQuoteTime(before.UpdatedAt)
	afterTime, _ := validQuoteTime(after.UpdatedAt)
	if afterTime.Before(beforeTime) || before.State != "reserved" && before.State != "released" || before.State == "reserved" && after.Revision <= before.Revision || after.ID != before.ID || after.ProcessingDigest != before.ProcessingDigest || after.RequestDigest != before.RequestDigest || after.RequestID != before.RequestID || after.ExecutionKind != before.ExecutionKind || after.PriceSnapshotDigest != before.PriceSnapshotDigest || after.PricingRevision != before.PricingRevision || after.Currency != before.Currency || after.MaximumCents != before.MaximumCents || after.ExpiresAt != before.ExpiresAt || after.CreatedAt != before.CreatedAt || after.Revision < before.Revision || after.State != "released" {
		return FinancialQuote{}, fmt.Errorf("%w: release response differs from quoted reservation", ErrClientHTTPFailure)
	}
	released.boundInput = quote.boundInput
	return released, nil
}

// QuotedExecution retains the addressed quote identity and one validated protocol result.
type QuotedExecution struct {
	quoteID, processingDigest, requestDigest string
	native                                   *StructuredRequestResult
	media                                    *MediaOperation
	valid                                    bool
}

// QuoteID returns the addressed quote. An invalid zero value returns an empty ID.
func (execution QuotedExecution) QuoteID() string { return execution.quoteID }

// NativeResult returns a copy of the structured result and whether this is a native execution.
func (execution QuotedExecution) NativeResult() (StructuredRequestResult, bool) {
	if !execution.valid || execution.native == nil {
		return StructuredRequestResult{}, false
	}
	return *execution.native, true
}

// MediaResult returns a detached copy of the media operation, if selected.
func (execution QuotedExecution) MediaResult() (MediaOperation, bool) {
	if !execution.valid || execution.media == nil {
		return MediaOperation{}, false
	}
	result := *execution.media
	result.Outputs = make([]MediaOperationOutput, len(execution.media.Outputs))
	copy(result.Outputs, execution.media.Outputs)
	if execution.media.PartialOutputs != nil {
		result.PartialOutputs = make([]MediaOperationPartialOutput, len(execution.media.PartialOutputs))
		copy(result.PartialOutputs, execution.media.PartialOutputs)
	}
	if execution.media.Error != nil {
		failure := *execution.media.Error
		result.Error = &failure
	}

	return result, true
}

// quotedMetadata records status and identity headers. The shared transport owns all I/O and failures.
type quotedMetadata struct {
	transport HTTPDoer
	status    int
	header    http.Header
}

func (metadata *quotedMetadata) Do(request *http.Request) (*http.Response, error) {
	response, err := metadata.transport.Do(request)
	if response != nil {
		metadata.status = response.StatusCode
		metadata.header = response.Header.Clone()
	}
	return response, err
}

// CreateQuotedExecution consumes the existing hold once. The quote ID is its execution key.
func (client Client) CreateQuotedExecution(ctx context.Context, quote FinancialQuote, input FinancialQuoteInput) (QuotedExecution, error) {
	if !quote.valid || len(input.payload) == 0 || quote.snapshot.ProcessingDigest != input.processingDigest || quote.snapshot.ExecutionKind != input.kind || quote.snapshot.State != "reserved" && quote.snapshot.State != "consumed" || len(quote.boundInput.payload) > 0 && !bytes.Equal(quote.boundInput.payload, input.payload) {
		return QuotedExecution{}, fmt.Errorf("%w: execution intent differs from quoted input", ErrInvalidClientRequest)
	}
	return client.quotedExecution(ctx, http.MethodPost, quote, input.payload)
}

// GetQuotedExecution reads the original result. It never creates or retries execution.
func (client Client) GetQuotedExecution(ctx context.Context, quote FinancialQuote) (QuotedExecution, error) {
	if !quote.valid {
		return QuotedExecution{}, fmt.Errorf("%w: invalid quote handle", ErrInvalidClientRequest)
	}
	return client.quotedExecution(ctx, http.MethodGet, quote, nil)
}
func (client Client) quotedExecution(ctx context.Context, method string, quote FinancialQuote, body []byte) (QuotedExecution, error) {
	request, err := client.quotedRequest(ctx, method, llmproxycontract.FinancialQuotesPath+"/"+quote.snapshot.ID+"/executions", "", body)
	if err != nil {
		return QuotedExecution{}, err
	}
	metadata := &quotedMetadata{transport: client.httpClient}
	client.httpClient = metadata
	response, err := client.doMediaResource(request, http.StatusOK, http.StatusAccepted)
	if err != nil {
		return QuotedExecution{}, err
	}
	snapshot := quote.snapshot
	if metadata.header.Get(llmproxycontract.HeaderFinancialQuoteID) != snapshot.ID || metadata.header.Get(llmproxycontract.HeaderFinancialProcessingDigest) != snapshot.ProcessingDigest || metadata.header.Get(llmproxycontract.HeaderFinancialRequestDigest) != snapshot.RequestDigest {
		return QuotedExecution{}, fmt.Errorf("%w: execution response belongs to another quoted intent", ErrClientHTTPFailure)
	}
	result := QuotedExecution{quoteID: snapshot.ID, processingDigest: snapshot.ProcessingDigest, requestDigest: snapshot.RequestDigest, valid: true}
	if snapshot.ExecutionKind == FinancialQuoteNative {
		var native StructuredRequestResult
		if metadata.status == http.StatusAccepted {
			native, err = decodeStructuredRequestPending(response)
			if err != nil {
				return QuotedExecution{}, err
			}
			if snapshot.ExecutionID != "" && native.ProxyRequestID != snapshot.ExecutionID {
				return QuotedExecution{}, fmt.Errorf("%w: native execution identity differs from quote", ErrClientHTTPFailure)
			}
		} else {
			if !json.Valid(response) {
				return QuotedExecution{}, fmt.Errorf("%w: invalid structured quoted result", ErrClientHTTPFailure)
			}
			native = StructuredRequestResult{State: "succeeded", Output: string(response)}
		}
		result.native = &native
		return result, nil
	}
	var wire struct {
		MediaOperation
		Model json.RawMessage `json:"model"`
	}
	if err = decodeExactJSON(response, &wire); err != nil {
		return QuotedExecution{}, fmt.Errorf("%w: invalid quoted media response", ErrClientHTTPFailure)
	}
	operation := wire.MediaOperation
	if len(wire.Model) > 0 && (json.Unmarshal(wire.Model, &operation.Model) != nil || strings.TrimSpace(operation.Model) == "") {
		return QuotedExecution{}, fmt.Errorf("%w: invalid quoted media model", ErrClientHTTPFailure)
	}
	if !validMediaOperation(operation) || !mediaOperationIdentifierPattern.MatchString(operation.OperationID) || !financialEnum(operation.Capability, "image.generate", "image.edit") || !diagnosticProviderPattern.MatchString(operation.Provider) || strings.TrimSpace(operation.Model) == "" || !financialEnum(operation.CancellationState, "not_requested", "requested", "confirmed", "unsupported") || snapshot.ExecutionID != "" && operation.OperationID != snapshot.ExecutionID {
		return QuotedExecution{}, fmt.Errorf("%w: quoted media identity or state invalid", ErrClientHTTPFailure)
	}
	if len(quote.boundInput.payload) > 0 && (operation.Provider != quote.boundInput.provider || operation.Model != quote.boundInput.model || operation.Capability != quote.boundInput.capability) {
		return QuotedExecution{}, fmt.Errorf("%w: quoted media route differs from frozen intent", ErrClientHTTPFailure)
	}
	result.media = &operation
	return result, nil
}

// QuotedExecutionPending reports native accepted state through the existing stable SDK error.
func (execution QuotedExecution) PendingError() error {
	if execution.valid && execution.native != nil && execution.native.State != "succeeded" {
		return &StructuredRequestPendingError{snapshot: *execution.native}
	}
	return nil
}
