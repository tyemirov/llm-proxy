package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var quoteIdentifierPattern = regexp.MustCompile(`^quote-[a-f0-9]{32}$`)

type financialQuoteIntent struct {
	Kind     string          `json:"kind"`
	Provider string          `json:"provider"`
	Request  json.RawMessage `json:"request"`
}
type financialQuoteInput struct {
	ProcessingDigest string               `json:"processing_digest"`
	Intent           financialQuoteIntent `json:"intent"`
}
type preparedFinancialIntent struct {
	provider, model, operation, catalog, digest string
	executionKind                               journalExecutionKind
	canonical                                   []byte
	reserve                                     journalReservation
	native                                      *chatRequestParameters
	mediaPayload                                *mediaOperationCreatePayload
}
type applicationQuoteRuntime struct {
	database    *gormManagedTenantDatabase
	text        *hostedTextRequests
	providers   *providerRegistry
	coordinator *providerRouter
	media       *mediaOperationService
	assets      *tenantAssetStore
	store       *managedTenantStore
	logger      *zap.SugaredLogger
	now         func() time.Time
	maxBytes    int64
}

func registerApplicationQuoteRoutes(router *gin.Engine, auth tenantAuthenticator, policy requestTimeoutPolicy, runtime *applicationQuoteRuntime) {
	wrap := func(handler gin.HandlerFunc) gin.HandlerFunc {
		return mediaTenantAuthenticatedHandler(auth, runtime.logger, requestTimeoutHandler(policy, runtime.logger, handler))
	}
	cache := func(ctx *gin.Context) {
		ctx.Header(headerCacheControl, cacheControlNoStore)
		ctx.Header("Vary", "Authorization")
	}
	path := llmproxycontract.FinancialQuotesPath
	router.POST(path, cache, wrap(runtime.createHandler()))
	router.GET(path, cache, wrap(runtime.readHandler()))
	router.GET(path+"/:quote_id", cache, wrap(runtime.readHandler()))
	router.PATCH(path+"/:quote_id", cache, wrap(runtime.releaseHandler()))
	router.POST(path+"/:quote_id/executions", cache, wrap(runtime.executeHandler()))
	router.GET(path+"/:quote_id/executions", cache, wrap(runtime.executionReadHandler()))
}
func decodeFinancialQuoteBody(ctx *gin.Context, limit int64, value any) error {
	if ctx.Request.URL.RawQuery != "" || !strings.HasPrefix(ctx.GetHeader("Content-Type"), "application/json") {
		return errQuoteInvalid
	}
	if limit <= 0 {
		limit = 8 << 20
	}
	decoder := json.NewDecoder(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return errQuoteInvalid
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errQuoteInvalid
	}
	return nil
}
func (runtime *applicationQuoteRuntime) prepare(ctx *gin.Context, identity tenant, input financialQuoteInput, key string) (preparedFinancialIntent, error) {
	var prepared preparedFinancialIntent
	if !canonicalSHA256(input.ProcessingDigest) || input.Intent.Provider == "" || input.Intent.Provider != strings.ToLower(strings.TrimSpace(input.Intent.Provider)) {
		return prepared, errQuoteInvalid
	}
	if runtime.database == nil {
		return prepared, errQuoteUnavailable
	}
	if runtime.providers == nil {
		return prepared, errQuoteUnavailable
	}
	if _, known := runtime.providers.definitions[providerID(input.Intent.Provider)]; !known {
		return prepared, errQuoteInvalid
	}
	if settings, allowed := identity.providerSettings[providerID(input.Intent.Provider)]; !allowed || settings.hostedGrantID == "" {
		return prepared, errHostedAuthorityDenied
	}
	prepared.provider = input.Intent.Provider
	switch input.Intent.Kind {
	case "native_v2":
		if runtime.text == nil {
			return prepared, errQuoteUnavailable
		}
		var payload chatV2RequestPayload
		if err := decodeStrictJSON(input.Intent.Request, &payload); err != nil || payload.Model == "" || payload.StructuredOutput == nil {
			return prepared, errQuoteInvalid
		}
		request, err := prepareV2TextRequest(ctx.Request.Context(), payload, input.Intent.Provider, "", []string{key}, requestTimeoutStateFromContext(ctx).budget, textRequestDefaultsForProvider(input.Intent.Provider, identity, runtime.providers), newModelValidator(runtime.providers.forTenant(identity)), identity, runtime.assets)
		if err != nil {
			return prepared, fmt.Errorf("%w: prepare native quote: %w", errQuoteInvalid, err)
		}
		prepared.native = &request
		prepared.model = request.model.identifier.string()
		prepared.operation = ModelOperationText
		prepared.catalog = runtime.text.catalogRevision
		prepared.executionKind = journalExecutionText
		prepared.canonical = hostedTextIntent(request)
		prepared.reserve = func(tx *gorm.DB, record managedJournalRequestRecord) error {
			return runtime.text.authorize(tx, record, hostedCompletionIntent{provider: request.provider, model: request.model.identifier, operation: ModelOperationText, kind: journalExecutionText, canonical: prepared.canonical, endpoint: endpointKindText, webSearch: request.webSearchEnabled})
		}
	case "media_operation":
		if runtime.media == nil || runtime.media.hostedAdmission == nil {
			return prepared, errQuoteUnavailable
		}
		var payload mediaOperationCreatePayload
		if err := decodeStrictJSON(input.Intent.Request, &payload); err != nil || payload.Provider != input.Intent.Provider || (payload.Capability != llmproxycontract.MediaCapabilityImageGenerate && payload.Capability != llmproxycontract.MediaCapabilityImageEdit) || payload.Model == "" {
			return prepared, errQuoteInvalid
		}
		canonicalInput, err := canonicalJSONObject(payload.Input)
		if err != nil {
			return prepared, errQuoteInvalid
		}
		canonicalControls, err := canonicalJSONObject(payload.Controls)
		if err != nil {
			return prepared, errQuoteInvalid
		}
		operation, validated, _, err := runtime.media.prepareNewMediaOperation(ctx.Request.Context(), identity, payload, canonicalInput, canonicalControls, "", sha256Hex(quotedResponseKey(key)), catalogOperationForMediaCapability(payload.Capability))
		if err != nil {
			return prepared, fmt.Errorf("%w: prepare image quote: %w", errQuoteInvalid, err)
		}
		var assets []string
		for _, id := range validated.InputAssetIDs {
			metadata, err := runtime.media.assets.metadata(identity, id)
			if err != nil {
				return prepared, errQuoteInvalid
			}
			assets = append(assets, id+":"+metadata.ContentSHA256)
		}
		prepared.canonical, _ = json.Marshal(struct {
			Input, Controls json.RawMessage
			Assets          []string
			Parent, Binding string
		}{operation.NormalizedInput, operation.NormalizedControls, assets, operation.ParentOperationID, operation.ExecutionBinding})
		prepared.model = payload.Model
		prepared.operation = operation.CatalogOperation
		prepared.catalog = operation.CatalogRevision
		prepared.executionKind = journalExecutionMedia
		prepared.mediaPayload = &payload
		prepared.reserve = func(tx *gorm.DB, record managedJournalRequestRecord) error {
			return runtime.media.hostedAdmission(tx, record, operation)
		}
	default:
		return prepared, errQuoteInvalid
	}
	fingerprint, _ := json.Marshal(struct {
		Kind, Provider, Model, Processing string
		Intent                            json.RawMessage
	}{input.Intent.Kind, prepared.provider, prepared.model, input.ProcessingDigest, prepared.canonical})
	prepared.digest = sha256Hex(string(fingerprint))
	return prepared, nil
}
func (prepared preparedFinancialIntent) close() {
	if prepared.native != nil {
		prepared.native.messages.closeMedia()
	}
}
func (runtime *applicationQuoteRuntime) createHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		key, err := hostedTextKeyValues(ctx.Request.Header.Values(llmproxycontract.HeaderIdempotencyKey))
		if err != nil {
			writeFinancialQuoteError(ctx, errQuoteInvalid)
			return
		}
		var input financialQuoteInput
		if err := decodeFinancialQuoteBody(ctx, runtime.maxBytes, &input); err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		identity := authenticatedTenantFromContext(ctx)
		prepared, err := runtime.prepare(ctx, identity, input, key)
		if err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		defer prepared.close()
		response, created, err := runtime.createQuote(ctx.Request.Context(), identity, key, input, prepared)
		if err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		ctx.Header("Location", llmproxycontract.FinancialQuotesPath+"/"+response.ID)
		ctx.JSON(status, response)
	}
}
func (runtime *applicationQuoteRuntime) owned(ctx *gin.Context) (managedFinancialQuoteRecord, error) {
	if runtime.database == nil {
		return managedFinancialQuoteRecord{}, errQuoteUnavailable
	}
	if ctx.Request.URL.RawQuery != "" {
		return managedFinancialQuoteRecord{}, errQuoteInvalid
	}
	id, key := ctx.Param("quote_id"), ""
	if id != "" {
		if !quoteIdentifierPattern.MatchString(id) {
			return managedFinancialQuoteRecord{}, errQuoteInvalid
		}
	} else {
		var err error
		key, err = hostedTextKeyValues(ctx.Request.Header.Values(llmproxycontract.HeaderIdempotencyKey))
		if err != nil {
			return managedFinancialQuoteRecord{}, errQuoteInvalid
		}
	}
	return readOwnedQuote(runtime.database.database.WithContext(ctx.Request.Context()), authenticatedTenantFromContext(ctx).identifier.string(), id, key)
}
func (runtime *applicationQuoteRuntime) readHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		quote, err := runtime.owned(ctx)
		if err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		response, err := quoteResponse(runtime.database.database.WithContext(ctx.Request.Context()), quote)
		if err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}
func (runtime *applicationQuoteRuntime) releaseHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		quote, err := runtime.owned(ctx)
		if err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		var change struct {
			Revision uint64 `json:"revision"`
			State    string `json:"state"`
		}
		if err := decodeFinancialQuoteBody(ctx, runtime.maxBytes, &change); err != nil || change.Revision == 0 || change.State != quoteReleased {
			writeFinancialQuoteError(ctx, errQuoteInvalid)
			return
		}
		var response financialQuoteResponse
		err = runtime.database.database.WithContext(ctx.Request.Context()).Transaction(func(tx *gorm.DB) error {
			lock := lockManagedReservationRequest(tx, quote.RequestID)
			if lock.Error != nil {
				return lock.Error
			}
			current, err := readOwnedQuote(tx, quote.TenantID, quote.ID, "")
			if err != nil {
				return err
			}
			if current.State != quoteReleased {
				if current.Revision != change.Revision {
					return errQuoteConflict
				}
				if err := releaseQuote(tx, current, quoteReleased, runtime.now().UTC()); err != nil {
					return err
				}
				current.State = quoteReleased
				current.Revision++
				current.UpdatedAt = runtime.now().UTC()
			}
			response, err = quoteResponse(tx, current)
			return err
		})
		if err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}
func setQuoteExecutionHeaders(ctx *gin.Context, quote managedFinancialQuoteRecord) {
	ctx.Header(llmproxycontract.HeaderFinancialQuoteID, quote.ID)
	ctx.Header(llmproxycontract.HeaderFinancialProcessingDigest, quote.ProcessingDigest)
	ctx.Header(llmproxycontract.HeaderFinancialRequestDigest, quote.RequestDigest)
}
func (runtime *applicationQuoteRuntime) executeHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		quote, err := runtime.owned(ctx)
		if err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		if quote.State != quoteReserved && quote.State != quoteConsumed {
			writeFinancialQuoteError(ctx, errQuoteConflict)
			return
		}
		if quote.State == quoteReserved && !quote.ExpiresAt.After(runtime.now().UTC()) {
			writeFinancialQuoteError(ctx, errQuoteExpired)
			return
		}
		var input financialQuoteInput
		if err := decodeFinancialQuoteBody(ctx, runtime.maxBytes, &input); err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		if input.ProcessingDigest != quote.ProcessingDigest || input.Intent.Kind != quote.Kind {
			writeFinancialQuoteError(ctx, errQuoteConflict)
			return
		}
		identity := authenticatedTenantFromContext(ctx)
		prepared, err := runtime.prepare(ctx, identity, input, quote.ID)
		if err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		defer prepared.close()
		if prepared.digest != quote.RequestDigest {
			writeFinancialQuoteError(ctx, errQuoteConflict)
			return
		}
		ctx.Request = ctx.Request.WithContext(context.WithValue(ctx.Request.Context(), quotedExecutionContextKey{}, quotedExecutionIdentity{quote}))
		setQuoteExecutionHeaders(ctx, quote)
		if prepared.native != nil {
			ctx.Request.Header.Set(llmproxycontract.HeaderIdempotencyKey, quote.ID)
			submitChatRequest(ctx, runtime.coordinator, *prepared.native, identity, usageEndpointV2, runtime.store, runtime.logger, nativeCompletionEncoder)
			return
		}
		operation, created, err := runtime.media.create(ctx.Request.Context(), identity, quotedResponseKey(quote.ID), *prepared.mediaPayload)
		if err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		response, err := runtime.media.store.publicResponse(ctx.Request.Context(), identity.identifier.string(), operation.OperationID)
		if err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusAccepted
		}
		ctx.JSON(status, response)
		if created {
			ctx.Writer.Flush()
			runtime.media.enqueue(operation.OperationID)
		}
	}
}
func (runtime *applicationQuoteRuntime) executionReadHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		quote, err := runtime.owned(ctx)
		if err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		setQuoteExecutionHeaders(ctx, quote)
		if quote.State != quoteConsumed {
			writeFinancialQuoteError(ctx, errQuoteConflict)
			return
		}
		var request managedJournalRequestRecord
		if err := runtime.database.database.WithContext(ctx.Request.Context()).First(&request, "id = ? AND tenant_id = ?", quote.RequestID, quote.TenantID).Error; err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		if quote.Kind == "media_operation" {
			if runtime.media == nil {
				writeFinancialQuoteError(ctx, errQuoteUnavailable)
				return
			}
			response, err := runtime.media.store.publicResponse(ctx.Request.Context(), quote.TenantID, request.ExecutionID)
			if err != nil {
				if errors.Is(err, errMediaOperationStore) {
					writeFinancialQuoteError(ctx, err)
				} else {
					writeMediaOperationError(ctx, err)
				}
				return
			}
			ctx.JSON(http.StatusOK, response)
			return
		}
		if runtime.text == nil {
			writeFinancialQuoteError(ctx, errQuoteUnavailable)
			return
		}
		result, err := runtime.text.replay(request, completedStructuredDataRestore)
		var pending *hostedRequestReplay
		if errors.As(err, &pending) {
			writeStructuredRequestRecord(ctx, pending.record, runtime.now().UTC())
			return
		}
		if errors.Is(err, errHostedResultExpired) {
			ctx.JSON(http.StatusGone, gin.H{"error": gin.H{"code": "hosted_result_expired"}})
			return
		}
		if err != nil {
			writeFinancialQuoteError(ctx, err)
			return
		}
		ctx.Data(http.StatusOK, structuredJSONContentType, []byte(result.content.text()))
	}
}
func completedStructuredDataRestore(text string) completionContent {
	return completedStructuredData(text)
}
func writeFinancialQuoteError(ctx *gin.Context, err error) {
	status := financialQuoteErrorStatus(err)
	code := "quote_unavailable"
	for _, known := range []error{errQuoteInvalid, errQuoteNotFound, errQuoteConflict, errQuoteExpired, errHostedAuthorityDenied, errInsufficientFunds, errFinancialAdmissionUnavailable} {
		if errors.Is(err, known) {
			code = known.Error()
			break
		}
	}
	ctx.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": "The quoted hosted request could not be completed."}})
}
