package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MediaOperationStateQueued    = llmproxycontract.MediaOperationStateQueued
	MediaOperationStateRunning   = llmproxycontract.MediaOperationStateRunning
	MediaOperationStateSucceeded = llmproxycontract.MediaOperationStateSucceeded
	MediaOperationStateFailed    = llmproxycontract.MediaOperationStateFailed
	MediaOperationStateCancelled = llmproxycontract.MediaOperationStateCancelled
	MediaOperationStateUncertain = llmproxycontract.MediaOperationStateUncertain

	MediaProviderExecutionNotDispatched = "not_dispatched"
	MediaProviderExecutionDispatched    = "dispatched"
	MediaProviderExecutionSucceeded     = "succeeded"
	MediaProviderExecutionFailed        = "failed"
	MediaProviderExecutionUncertain     = "uncertain"

	MediaCancellationNotRequested = llmproxycontract.MediaCancellationNotRequested
	MediaCancellationRequested    = llmproxycontract.MediaCancellationRequested
	MediaCancellationConfirmed    = llmproxycontract.MediaCancellationConfirmed
	MediaCancellationUnsupported  = llmproxycontract.MediaCancellationUnsupported
)

var (
	mediaOperationIdentifierPattern = regexp.MustCompile(`^mop_[0-9a-f]{32}$`)
	mediaIdempotencyKeyPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$`)

	errMediaOperationInvalid        = errors.New(llmproxycontract.ErrorCodeMediaOperationInvalid)
	errMediaOperationUnavailable    = errors.New(llmproxycontract.ErrorCodeMediaOperationUnavailable)
	errMediaOperationNotFound       = errors.New(llmproxycontract.ErrorCodeMediaOperationNotFound)
	errMediaOperationIntentConflict = errors.New(llmproxycontract.ErrorCodeMediaOperationIntentConflict)
	errMediaOperationCapacity       = errors.New(llmproxycontract.ErrorCodeMediaOperationCapacity)
	errMediaOperationStore          = errors.New(llmproxycontract.ErrorCodeMediaOperationStore)
	errMediaOperationExpired        = errors.New(llmproxycontract.ErrorCodeMediaOperationExpired)
)

type mediaOperationExpiredError struct {
	operationID string
	state       string
}

func (failure mediaOperationExpiredError) Error() string { return errMediaOperationExpired.Error() }

// MediaOperationAdapter is one provider-capability execution component. It validates
// provider-specific input before acceptance and owns submission, recovery, and cancellation.
type MediaOperationAdapter interface {
	Validate(MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error)
	Execute(context.Context, MediaOperationExecutionRequest) MediaOperationExecutionResult
	Recover(context.Context, MediaOperationExecutionRequest) MediaOperationExecutionResult
	Cancel(context.Context, MediaOperationExecutionRequest) MediaOperationCancellationResult
}

// MediaOperationAdapterRequest is the caller request after catalog route resolution.
type MediaOperationAdapterRequest struct {
	Capability string
	Provider   string
	Model      string
	Input      json.RawMessage
	Controls   json.RawMessage
}

// MediaOperationValidatedRequest is the canonical provider-specific request retained at acceptance.
type MediaOperationValidatedRequest struct {
	InputAssetIDs []string
	Input         json.RawMessage
	Controls      json.RawMessage
}

// MediaOperationExecutionRequest is the private durable execution contract supplied to an adapter.
type MediaOperationExecutionRequest struct {
	OperationID    string
	DispatchToken  string
	Capability     string
	Provider       string
	Model          string
	Input          json.RawMessage
	Controls       json.RawMessage
	ProviderHandle string
}

// MediaOperationOutput is one ordered provider output to publish as a tenant asset.
type MediaOperationOutput struct {
	MIMEType string
	Data     []byte
}

// MediaOperationExecutionResult is the adapter's terminal or recoverable observation.
type MediaOperationExecutionResult struct {
	State          string
	ProviderHandle string
	ErrorCode      string
	Outputs        []MediaOperationOutput
}

// MediaOperationCancellationResult records only what the provider proves about cancellation.
type MediaOperationCancellationResult struct {
	State string
}

type mediaOperationCreatePayload struct {
	Capability string          `json:"capability"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	Input      json.RawMessage `json:"input"`
	Controls   json.RawMessage `json:"controls"`
}

type mediaOperationRecord struct {
	OperationID            string `gorm:"primaryKey"`
	TenantID               string `gorm:"not null;uniqueIndex:idx_media_operation_tenant_key,priority:1;index:idx_media_operation_active,priority:1"`
	IdempotencyKeyDigest   string `gorm:"not null;uniqueIndex:idx_media_operation_tenant_key,priority:2"`
	IntentDigest           string `gorm:"not null"`
	Capability             string `gorm:"not null"`
	CatalogOperation       string `gorm:"not null"`
	Provider               string `gorm:"not null"`
	Model                  string `gorm:"not null"`
	CatalogRevision        string `gorm:"not null"`
	CredentialReference    string `gorm:"not null"`
	NormalizedInput        []byte `gorm:"not null"`
	NormalizedControls     []byte `gorm:"not null"`
	PublicState            string `gorm:"not null;index:idx_media_operation_active,priority:2"`
	ProviderExecutionState string `gorm:"not null"`
	ProviderHandle         string
	DispatchToken          string
	PublicErrorCode        string
	CancellationState      string    `gorm:"not null"`
	AcceptedAt             time.Time `gorm:"not null"`
	UpdatedAt              time.Time `gorm:"not null"`
	DeadlineAt             time.Time `gorm:"not null"`
	TerminalAt             *time.Time
}

type mediaOperationClaimRecord struct {
	OperationID string    `gorm:"primaryKey"`
	WorkerID    string    `gorm:"not null"`
	Generation  uint64    `gorm:"not null"`
	ExpiresAt   time.Time `gorm:"not null"`
}

type mediaOperationAssetReferenceRecord struct {
	OperationID string    `gorm:"primaryKey"`
	Role        string    `gorm:"primaryKey"`
	Ordinal     int       `gorm:"primaryKey"`
	TenantID    string    `gorm:"not null;index:idx_media_asset_active,priority:1"`
	AssetID     string    `gorm:"not null;index:idx_media_asset_active,priority:2"`
	MIMEType    string    `gorm:"not null"`
	SizeBytes   int64     `gorm:"not null"`
	Active      bool      `gorm:"not null;index:idx_media_asset_active,priority:3"`
	CreatedAt   time.Time `gorm:"not null"`
}

type mediaOperationUsageDeliveryRecord struct {
	OperationID string `gorm:"primaryKey"`
	TenantID    string `gorm:"not null"`
	DeliveredAt *time.Time
	CreatedAt   time.Time `gorm:"not null"`
}

type mediaOperationTombstoneRecord struct {
	TenantID             string    `gorm:"primaryKey"`
	IdempotencyKeyDigest string    `gorm:"primaryKey"`
	IntentDigest         string    `gorm:"not null"`
	OperationID          string    `gorm:"not null"`
	TerminalState        string    `gorm:"not null"`
	CreatedAt            time.Time `gorm:"not null"`
}

type mediaOperationOutputResponse struct {
	AssetID   string `json:"asset_id"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	Ordinal   int    `json:"ordinal"`
}

type mediaOperationCostEvidence struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type mediaOperationResponse struct {
	OperationID       string                         `json:"operation_id"`
	Capability        string                         `json:"capability"`
	Provider          string                         `json:"provider"`
	Model             string                         `json:"model"`
	CatalogRevision   string                         `json:"catalog_revision"`
	State             string                         `json:"state"`
	CancellationState string                         `json:"cancellation_state"`
	Outputs           []mediaOperationOutputResponse `json:"outputs"`
	Error             *mediaOperationErrorResponse   `json:"error,omitempty"`
	Cost              mediaOperationCostEvidence     `json:"cost"`
	AcceptedAt        time.Time                      `json:"accepted_at"`
	UpdatedAt         time.Time                      `json:"updated_at"`
	DeadlineAt        time.Time                      `json:"deadline_at"`
}

type mediaOperationErrorResponse struct {
	Code string `json:"code"`
}

type mediaOperationErrorEnvelope struct {
	Error mediaOperationErrorResponse `json:"error"`
}

type mediaOperationExpiredEnvelope struct {
	OperationID string                      `json:"operation_id"`
	State       string                      `json:"state"`
	Error       mediaOperationErrorResponse `json:"error"`
}

type mediaOperationStore struct {
	database *gorm.DB
	now      func() time.Time
}

type mediaOperationService struct {
	store             *mediaOperationStore
	assets            *tenantAssetStore
	adapters          map[string]MediaOperationAdapter
	catalog           CatalogService
	providers         *providerRegistry
	queue             chan string
	queued            sync.Map
	workerCount       int
	claimLifetime     time.Duration
	claimRenewal      time.Duration
	lifetime          time.Duration
	globalCapacity    int
	tenantCapacity    int
	terminalRetention time.Duration
}

func mediaOperationAdapterKey(capability string, provider string, model string) string {
	return capability + "|" + provider + "|" + model
}

func newMediaOperationStore(managedTenants *managedTenantStore) (*mediaOperationStore, error) {
	database, ok := managedTenants.database.(*gormManagedTenantDatabase)
	if !ok {
		return nil, nil
	}
	if migrationError := database.database.AutoMigrate(
		&mediaOperationRecord{},
		&mediaOperationClaimRecord{},
		&mediaOperationAssetReferenceRecord{},
		&mediaOperationUsageDeliveryRecord{},
		&mediaOperationTombstoneRecord{},
	); migrationError != nil {
		return nil, fmt.Errorf("%w: migrate", errMediaOperationStore)
	}
	return &mediaOperationStore{database: database.database, now: func() time.Time { return time.Now().UTC() }}, nil
}

func newMediaOperationService(configuration Configuration, managedTenants *managedTenantStore, assets *tenantAssetStore, providers *providerRegistry) (*mediaOperationService, error) {
	store, storeError := newMediaOperationStore(managedTenants)
	if storeError != nil || store == nil {
		return nil, storeError
	}
	catalog := CatalogService{catalog: validatedModelCatalog{revision: configuration.ModelCatalog.Revision}}
	if len(configuration.MediaOperationAdapters) != 0 {
		var catalogError error
		catalog, catalogError = NewCatalogService(configuration.ModelCatalog)
		if catalogError != nil {
			return nil, catalogError
		}
	}
	service := &mediaOperationService{
		store: store, assets: assets, adapters: configuration.MediaOperationAdapters,
		catalog: catalog, providers: providers,
		queue:             make(chan string, configuration.MediaOperationCapacity),
		workerCount:       configuration.MediaOperationWorkers,
		claimLifetime:     time.Duration(configuration.MediaOperationClaimSeconds) * time.Second,
		claimRenewal:      time.Duration(configuration.MediaOperationClaimRenewalSeconds) * time.Second,
		lifetime:          time.Duration(configuration.MediaOperationLifetimeSeconds) * time.Second,
		globalCapacity:    configuration.MediaOperationCapacity,
		tenantCapacity:    configuration.TenantMediaOperationCapacity,
		terminalRetention: time.Duration(configuration.AssetRetentionSeconds) * time.Second,
	}
	assets.activeReference = func(tenantID string, assetID string) (bool, error) {
		var count int64
		errorValue := store.database.Model(&mediaOperationAssetReferenceRecord{}).Where("tenant_id = ? AND asset_id = ? AND active = ?", tenantID, assetID, true).Count(&count).Error
		return count != 0, errorValue
	}
	if service.workerCount > 0 {
		for workerIndex := 0; workerIndex < service.workerCount; workerIndex++ {
			go service.runWorker(newMediaOperationIdentifier())
		}
		go service.runMaintenance()
	}
	service.deliverPendingUsage()
	service.expireTerminalData()
	service.resumeOutstanding()
	return service, nil
}

func registerMediaOperationRoutes(router *gin.Engine, authenticator tenantAuthenticator, structuredLogger *zap.SugaredLogger, service *mediaOperationService) {
	router.GET(llmproxycontract.MediaCapabilitiesPath, mediaTenantAuthenticatedHandler(authenticator, structuredLogger, service.capabilitiesHandler()))
	router.POST(llmproxycontract.MediaOperationsPath, mediaTenantAuthenticatedHandler(authenticator, structuredLogger, service.createHandler()))
	router.GET(llmproxycontract.MediaOperationsPath+"/:operation_id", mediaTenantAuthenticatedHandler(authenticator, structuredLogger, service.statusHandler()))
	router.PUT(llmproxycontract.MediaOperationsPath+"/:operation_id/cancellation", mediaTenantAuthenticatedHandler(authenticator, structuredLogger, service.cancellationHandler()))
}

func (service *mediaOperationService) capabilitiesHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestTenant := authenticatedTenantFromContext(ginContext)
		type capabilityRoute struct {
			Capability string           `json:"capability"`
			Provider   string           `json:"provider"`
			Model      string           `json:"model"`
			Controls   []CatalogControl `json:"controls"`
			Limits     []CatalogLimit   `json:"limits"`
		}
		routes := make([]capabilityRoute, 0)
		for _, offering := range service.catalog.catalog.offerings {
			for _, operation := range offering.Operations {
				capability := mediaCapabilityForCatalogOperation(operation)
				if capability == "" || service.adapters[mediaOperationAdapterKey(capability, offering.Provider, offering.Model)] == nil {
					continue
				}
				provider, providerError := service.providers.forTenant(requestTenant).resolveProvider(offering.Provider, "")
				if providerError != nil || !requestTenant.providerSettings[provider.identifier].hasRequiredConnectionFields(provider) {
					continue
				}
				routes = append(routes, capabilityRoute{Capability: capability, Provider: offering.Provider, Model: offering.Model, Controls: offering.Controls, Limits: offering.Limits})
			}
		}
		sort.Slice(routes, func(first, second int) bool {
			return mediaOperationAdapterKey(routes[first].Capability, routes[first].Provider, routes[first].Model) < mediaOperationAdapterKey(routes[second].Capability, routes[second].Provider, routes[second].Model)
		})
		ginContext.JSON(http.StatusOK, gin.H{"catalog_revision": service.catalog.Revision(), "routes": routes})
	}
}

func (service *mediaOperationService) createHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		idempotencyKey := ginContext.GetHeader(llmproxycontract.HeaderIdempotencyKey)
		if !mediaIdempotencyKeyPattern.MatchString(idempotencyKey) {
			writeMediaOperationError(ginContext, errMediaOperationInvalid)
			return
		}
		var payload mediaOperationCreatePayload
		decoder := json.NewDecoder(io.LimitReader(ginContext.Request.Body, DefaultMaxPromptBytes+1))
		decoder.DisallowUnknownFields()
		if decodeError := decoder.Decode(&payload); decodeError != nil || decoder.Decode(&struct{}{}) != io.EOF {
			writeMediaOperationError(ginContext, errMediaOperationInvalid)
			return
		}
		requestTenant := authenticatedTenantFromContext(ginContext)
		operation, created, createError := service.create(ginContext.Request.Context(), requestTenant, idempotencyKey, payload)
		if createError != nil {
			writeMediaOperationError(ginContext, createError)
			return
		}
		response, responseError := service.store.publicResponse(ginContext.Request.Context(), requestTenant.identifier.string(), operation.OperationID)
		if responseError != nil {
			writeMediaOperationError(ginContext, responseError)
			return
		}
		ginContext.Header("Location", llmproxycontract.MediaOperationsPath+"/"+operation.OperationID)
		if !created {
			ginContext.JSON(http.StatusOK, response)
			return
		}
		ginContext.JSON(http.StatusAccepted, response)
		ginContext.Writer.Flush()
		service.enqueue(operation.OperationID)
	}
}

func (service *mediaOperationService) create(requestContext context.Context, requestTenant tenant, idempotencyKey string, payload mediaOperationCreatePayload) (mediaOperationRecord, bool, error) {
	payload.Capability = strings.TrimSpace(payload.Capability)
	payload.Provider = strings.ToLower(strings.TrimSpace(payload.Provider))
	payload.Model = strings.TrimSpace(payload.Model)
	catalogOperation := catalogOperationForMediaCapability(payload.Capability)
	if catalogOperation == "" || payload.Provider == "" || payload.Model == "" {
		return mediaOperationRecord{}, false, errMediaOperationInvalid
	}
	canonicalInput, inputError := canonicalJSONObject(payload.Input)
	canonicalControls, controlsError := canonicalJSONObject(payload.Controls)
	if inputError != nil || controlsError != nil {
		return mediaOperationRecord{}, false, errMediaOperationInvalid
	}
	intentBytes, _ := json.Marshal(mediaOperationCreatePayload{Capability: payload.Capability, Provider: payload.Provider, Model: payload.Model, Input: canonicalInput, Controls: canonicalControls})
	intentDigest := mediaSHA256Hex(intentBytes)
	keyDigest := mediaSHA256Hex([]byte(idempotencyKey))
	var accepted mediaOperationRecord
	lookupError := service.store.database.WithContext(requestContext).Where("tenant_id = ? AND idempotency_key_digest = ?", requestTenant.identifier.string(), keyDigest).First(&accepted).Error
	if lookupError == nil {
		if accepted.IntentDigest != intentDigest {
			return mediaOperationRecord{}, false, errMediaOperationIntentConflict
		}
		return accepted, false, nil
	}
	if !errors.Is(lookupError, gorm.ErrRecordNotFound) {
		return mediaOperationRecord{}, false, errMediaOperationStore
	}
	var tombstone mediaOperationTombstoneRecord
	tombstoneError := service.store.database.WithContext(requestContext).Where("tenant_id = ? AND idempotency_key_digest = ?", requestTenant.identifier.string(), keyDigest).First(&tombstone).Error
	if tombstoneError == nil {
		if tombstone.IntentDigest != intentDigest {
			return mediaOperationRecord{}, false, errMediaOperationIntentConflict
		}
		return mediaOperationRecord{}, false, mediaOperationExpiredError{operationID: tombstone.OperationID, state: tombstone.TerminalState}
	}
	if !errors.Is(tombstoneError, gorm.ErrRecordNotFound) {
		return mediaOperationRecord{}, false, errMediaOperationStore
	}
	offering, offeringError := service.catalog.ResolveOffering(payload.Provider, payload.Model)
	if offeringError != nil || !offeringSupportsOperation(offering, catalogOperation) {
		return mediaOperationRecord{}, false, errMediaOperationUnavailable
	}
	provider, providerError := service.providers.forTenant(requestTenant).resolveProvider(payload.Provider, "")
	if providerError != nil {
		return mediaOperationRecord{}, false, errMediaOperationUnavailable
	}
	settings, configured := requestTenant.providerSettings[provider.identifier]
	if !configured || !settings.hasRequiredConnectionFields(provider) {
		return mediaOperationRecord{}, false, errMediaOperationUnavailable
	}
	adapter := service.adapters[mediaOperationAdapterKey(payload.Capability, payload.Provider, payload.Model)]
	if adapter == nil {
		return mediaOperationRecord{}, false, errMediaOperationUnavailable
	}
	validated, validationError := adapter.Validate(MediaOperationAdapterRequest{Capability: payload.Capability, Provider: payload.Provider, Model: payload.Model, Input: canonicalInput, Controls: canonicalControls})
	if validationError != nil {
		return mediaOperationRecord{}, false, errMediaOperationInvalid
	}
	canonicalInput, inputError = canonicalJSONObject(validated.Input)
	canonicalControls, controlsError = canonicalJSONObject(validated.Controls)
	if inputError != nil || controlsError != nil {
		return mediaOperationRecord{}, false, errMediaOperationInvalid
	}
	now := service.store.now()
	credentialVersionReference, credentialError := service.store.credentialReference(requestContext, requestTenant.identifier.string(), provider.identifier)
	if credentialError != nil {
		return mediaOperationRecord{}, false, credentialError
	}
	record := mediaOperationRecord{
		OperationID: newMediaOperationIdentifier(), TenantID: requestTenant.identifier.string(), IdempotencyKeyDigest: keyDigest,
		IntentDigest: intentDigest, Capability: payload.Capability, CatalogOperation: catalogOperation,
		Provider: payload.Provider, Model: payload.Model, CatalogRevision: service.catalog.Revision(),
		CredentialReference: credentialVersionReference, NormalizedInput: canonicalInput, NormalizedControls: canonicalControls,
		PublicState: MediaOperationStateQueued, ProviderExecutionState: MediaProviderExecutionNotDispatched,
		CancellationState: MediaCancellationNotRequested, AcceptedAt: now, UpdatedAt: now, DeadlineAt: now.Add(service.lifetime),
	}
	created := false
	if len(validated.InputAssetIDs) != 0 {
		service.assets.referenceMutex.Lock()
		defer service.assets.referenceMutex.Unlock()
	}
	transactionError := service.store.database.WithContext(requestContext).Transaction(func(transaction *gorm.DB) error {
		var existing mediaOperationRecord
		lookupError := transaction.Where("tenant_id = ? AND idempotency_key_digest = ?", record.TenantID, keyDigest).First(&existing).Error
		if lookupError == nil {
			if existing.IntentDigest != intentDigest {
				return errMediaOperationIntentConflict
			}
			record = existing
			return nil
		}
		if !errors.Is(lookupError, gorm.ErrRecordNotFound) {
			return errMediaOperationStore
		}
		var globalActive int64
		if countError := transaction.Model(&mediaOperationRecord{}).Where("public_state IN ?", []string{MediaOperationStateQueued, MediaOperationStateRunning}).Count(&globalActive).Error; countError != nil {
			return errMediaOperationStore
		}
		var tenantActive int64
		if countError := transaction.Model(&mediaOperationRecord{}).Where("tenant_id = ? AND public_state IN ?", record.TenantID, []string{MediaOperationStateQueued, MediaOperationStateRunning}).Count(&tenantActive).Error; countError != nil {
			return errMediaOperationStore
		}
		if globalActive >= int64(service.globalCapacity) || tenantActive >= int64(service.tenantCapacity) {
			return errMediaOperationCapacity
		}
		if createError := transaction.Create(&record).Error; createError != nil {
			return errMediaOperationStore
		}
		for index, assetID := range validated.InputAssetIDs {
			metadata, metadataError := service.assets.metadata(requestTenant, assetID)
			if metadataError != nil || metadata.State != assetStateAvailable {
				return errMediaOperationInvalid
			}
			reference := mediaOperationAssetReferenceRecord{OperationID: record.OperationID, Role: "input", Ordinal: index, TenantID: record.TenantID, AssetID: assetID, MIMEType: metadata.MIMEType, SizeBytes: metadata.SizeBytes, Active: true, CreatedAt: now}
			if referenceError := transaction.Create(&reference).Error; referenceError != nil {
				return errMediaOperationStore
			}
		}
		created = true
		return nil
	})
	return record, created, transactionError
}

func (service *mediaOperationService) statusHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestTenant := authenticatedTenantFromContext(ginContext)
		response, responseError := service.store.publicResponse(ginContext.Request.Context(), requestTenant.identifier.string(), ginContext.Param("operation_id"))
		if responseError != nil {
			writeMediaOperationError(ginContext, responseError)
			return
		}
		ginContext.JSON(http.StatusOK, response)
	}
}

func (service *mediaOperationService) cancellationHandler() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestTenant := authenticatedTenantFromContext(ginContext)
		operation, cancellationError := service.cancel(ginContext.Request.Context(), requestTenant, ginContext.Param("operation_id"))
		if cancellationError != nil {
			writeMediaOperationError(ginContext, cancellationError)
			return
		}
		ginContext.JSON(http.StatusOK, operation)
	}
}

func (service *mediaOperationService) cancel(requestContext context.Context, requestTenant tenant, operationID string) (mediaOperationResponse, error) {
	if !mediaOperationIdentifierPattern.MatchString(operationID) {
		return mediaOperationResponse{}, errMediaOperationNotFound
	}
	var adapterRequest MediaOperationExecutionRequest
	needsProviderCancel := false
	service.assets.referenceMutex.Lock()
	transactionError := service.store.database.WithContext(requestContext).Transaction(func(transaction *gorm.DB) error {
		var record mediaOperationRecord
		if lookupError := transaction.Where("operation_id = ? AND tenant_id = ?", operationID, requestTenant.identifier.string()).First(&record).Error; lookupError != nil {
			return errMediaOperationNotFound
		}
		now := service.store.now()
		switch record.PublicState {
		case MediaOperationStateQueued:
			record.PublicState = MediaOperationStateCancelled
			record.CancellationState = MediaCancellationConfirmed
			record.TerminalAt = &now
			record.UpdatedAt = now
			return finalizeMediaOperationCancellation(transaction, record, now)
		case MediaOperationStateRunning:
			if record.CancellationState == MediaCancellationNotRequested {
				record.CancellationState = MediaCancellationRequested
				record.UpdatedAt = now
				if saveError := transaction.Save(&record).Error; saveError != nil {
					return saveError
				}
			}
			needsProviderCancel = true
			adapterRequest = executionRequestFromRecord(record)
		}
		return nil
	})
	service.assets.referenceMutex.Unlock()
	if transactionError != nil {
		if errors.Is(transactionError, errMediaOperationNotFound) {
			return mediaOperationResponse{}, errMediaOperationNotFound
		}
		return mediaOperationResponse{}, errMediaOperationStore
	}
	if needsProviderCancel {
		var record mediaOperationRecord
		if lookupError := service.store.database.WithContext(requestContext).Where("operation_id = ? AND tenant_id = ?", operationID, requestTenant.identifier.string()).First(&record).Error; lookupError != nil {
			return mediaOperationResponse{}, errMediaOperationStore
		}
		adapter := service.adapters[mediaOperationAdapterKey(record.Capability, record.Provider, record.Model)]
		if adapter == nil {
			return mediaOperationResponse{}, errMediaOperationUnavailable
		}
		result := adapter.Cancel(requestContext, adapterRequest)
		if result.State == MediaCancellationConfirmed {
			service.assets.referenceMutex.Lock()
			confirmationError := service.store.database.WithContext(requestContext).Transaction(func(transaction *gorm.DB) error {
				var current mediaOperationRecord
				if lookupError := transaction.Where("operation_id = ? AND tenant_id = ?", operationID, requestTenant.identifier.string()).First(&current).Error; lookupError != nil {
					return lookupError
				}
				if current.PublicState != MediaOperationStateRunning {
					return nil
				}
				now := service.store.now()
				current.CancellationState = MediaCancellationConfirmed
				current.PublicState = MediaOperationStateCancelled
				current.TerminalAt = &now
				current.UpdatedAt = now
				return finalizeMediaOperationCancellation(transaction, current, now)
			})
			service.assets.referenceMutex.Unlock()
			if confirmationError != nil {
				return mediaOperationResponse{}, errMediaOperationStore
			}
		} else {
			now := service.store.now()
			if updateError := service.store.database.WithContext(requestContext).Model(&mediaOperationRecord{}).Where("operation_id = ? AND tenant_id = ? AND public_state = ?", operationID, requestTenant.identifier.string(), MediaOperationStateRunning).Updates(map[string]any{"cancellation_state": MediaCancellationUnsupported, "updated_at": now}).Error; updateError != nil {
				return mediaOperationResponse{}, errMediaOperationStore
			}
		}
	}
	return service.store.publicResponse(requestContext, requestTenant.identifier.string(), operationID)
}

func finalizeMediaOperationCancellation(transaction *gorm.DB, record mediaOperationRecord, now time.Time) error {
	if saveError := transaction.Save(&record).Error; saveError != nil {
		return saveError
	}
	if referenceError := transaction.Model(&mediaOperationAssetReferenceRecord{}).Where("operation_id = ? AND role = ?", record.OperationID, "input").Update("active", false).Error; referenceError != nil {
		return referenceError
	}
	if usageError := deliverMediaOperationUsage(transaction, record, now); usageError != nil {
		return usageError
	}
	return transaction.Where("operation_id = ?", record.OperationID).Delete(&mediaOperationClaimRecord{}).Error
}

func (store *mediaOperationStore) publicResponse(requestContext context.Context, tenantID string, operationID string) (mediaOperationResponse, error) {
	if !mediaOperationIdentifierPattern.MatchString(operationID) {
		return mediaOperationResponse{}, errMediaOperationNotFound
	}
	var record mediaOperationRecord
	if lookupError := store.database.WithContext(requestContext).Where("operation_id = ? AND tenant_id = ?", operationID, tenantID).First(&record).Error; lookupError != nil {
		var tombstone mediaOperationTombstoneRecord
		if tombstoneError := store.database.WithContext(requestContext).Where("operation_id = ? AND tenant_id = ?", operationID, tenantID).First(&tombstone).Error; tombstoneError == nil {
			return mediaOperationResponse{}, mediaOperationExpiredError{operationID: tombstone.OperationID, state: tombstone.TerminalState}
		}
		return mediaOperationResponse{}, errMediaOperationNotFound
	}
	var references []mediaOperationAssetReferenceRecord
	if referenceError := store.database.WithContext(requestContext).Where("operation_id = ? AND role = ?", operationID, "output").Order("ordinal ASC").Find(&references).Error; referenceError != nil {
		return mediaOperationResponse{}, errMediaOperationStore
	}
	outputs := make([]mediaOperationOutputResponse, 0, len(references))
	for _, reference := range references {
		outputs = append(outputs, mediaOperationOutputResponse{AssetID: reference.AssetID, MIMEType: reference.MIMEType, SizeBytes: reference.SizeBytes, Ordinal: reference.Ordinal})
	}
	response := mediaOperationResponse{
		OperationID: record.OperationID, Capability: record.Capability, Provider: record.Provider, Model: record.Model,
		CatalogRevision: record.CatalogRevision, State: record.PublicState, CancellationState: record.CancellationState,
		Outputs: outputs, Cost: mediaOperationCostEvidence{Available: false, Reason: "exact_price_unavailable"},
		AcceptedAt: record.AcceptedAt, UpdatedAt: record.UpdatedAt, DeadlineAt: record.DeadlineAt,
	}
	if record.PublicErrorCode != "" {
		response.Error = &mediaOperationErrorResponse{Code: record.PublicErrorCode}
	}
	return response, nil
}

func (service *mediaOperationService) enqueue(operationID string) {
	if _, loaded := service.queued.LoadOrStore(operationID, struct{}{}); loaded {
		return
	}
	select {
	case service.queue <- operationID:
	default:
		service.queued.Delete(operationID)
	}
}

func (service *mediaOperationService) resumeOutstanding() {
	var records []mediaOperationRecord
	if queryError := service.store.database.Where("public_state IN ? AND NOT EXISTS (SELECT 1 FROM media_operation_claim_records WHERE media_operation_claim_records.operation_id = media_operation_records.operation_id AND expires_at > ?)", []string{MediaOperationStateQueued, MediaOperationStateRunning}, service.store.now()).Find(&records).Error; queryError != nil {
		return
	}
	for _, record := range records {
		service.enqueue(record.OperationID)
	}
}

func (service *mediaOperationService) runMaintenance() {
	ticker := time.NewTicker(service.claimRenewal)
	defer ticker.Stop()
	for range ticker.C {
		service.deliverPendingUsage()
		service.expireTerminalData()
		service.resumeOutstanding()
	}
}

func (service *mediaOperationService) runWorker(workerID string) {
	for operationID := range service.queue {
		service.queued.Delete(operationID)
		service.runOperation(workerID, operationID)
	}
}

func (service *mediaOperationService) runOperation(workerID string, operationID string) {
	record, generation, recoverOperation, claimError := service.claim(workerID, operationID)
	if claimError != nil {
		return
	}
	adapter := service.adapters[mediaOperationAdapterKey(record.Capability, record.Provider, record.Model)]
	if adapter == nil {
		service.finish(operationID, generation, MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()})
		return
	}
	requestContext, cancel := context.WithDeadline(context.Background(), record.DeadlineAt)
	defer cancel()
	request := executionRequestFromRecord(record)
	var result MediaOperationExecutionResult
	if recoverOperation {
		result = service.runWithClaimRenewal(requestContext, operationID, generation, func() MediaOperationExecutionResult {
			return adapter.Recover(requestContext, request)
		})
	} else {
		if dispatchError := service.markDispatched(operationID, generation, record.DispatchToken); dispatchError != nil {
			return
		}
		result = service.runWithClaimRenewal(requestContext, operationID, generation, func() MediaOperationExecutionResult {
			return adapter.Execute(requestContext, request)
		})
	}
	service.finish(operationID, generation, result)
}

func (service *mediaOperationService) runWithClaimRenewal(requestContext context.Context, operationID string, generation uint64, execute func() MediaOperationExecutionResult) MediaOperationExecutionResult {
	resultChannel := make(chan MediaOperationExecutionResult, 1)
	go func() { resultChannel <- execute() }()
	ticker := time.NewTicker(service.claimRenewal)
	defer ticker.Stop()
	for {
		select {
		case result := <-resultChannel:
			return result
		case <-requestContext.Done():
			return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: "operation_deadline_exceeded"}
		case <-ticker.C:
			result := service.store.database.Model(&mediaOperationClaimRecord{}).Where("operation_id = ? AND generation = ?", operationID, generation).Update("expires_at", service.store.now().Add(service.claimLifetime))
			if result.Error != nil || result.RowsAffected != 1 {
				return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: "worker_claim_lost"}
			}
		}
	}
}

func (service *mediaOperationService) claim(workerID string, operationID string) (mediaOperationRecord, uint64, bool, error) {
	var record mediaOperationRecord
	var generation uint64
	recoverOperation := false
	now := service.store.now()
	transactionError := service.store.database.Transaction(func(transaction *gorm.DB) error {
		if lookupError := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "operation_id = ?", operationID).Error; lookupError != nil {
			return lookupError
		}
		if record.PublicState != MediaOperationStateQueued && record.PublicState != MediaOperationStateRunning {
			return gorm.ErrRecordNotFound
		}
		var claim mediaOperationClaimRecord
		claimError := transaction.First(&claim, "operation_id = ?", operationID).Error
		if claimError == nil && claim.ExpiresAt.After(now) {
			return gorm.ErrRecordNotFound
		}
		if claimError != nil && !errors.Is(claimError, gorm.ErrRecordNotFound) {
			return claimError
		}
		generation = claim.Generation + 1
		claim = mediaOperationClaimRecord{OperationID: operationID, WorkerID: workerID, Generation: generation, ExpiresAt: now.Add(service.claimLifetime)}
		if saveError := transaction.Save(&claim).Error; saveError != nil {
			return saveError
		}
		recoverOperation = record.ProviderExecutionState == MediaProviderExecutionDispatched
		if record.PublicState == MediaOperationStateQueued {
			record.PublicState = MediaOperationStateRunning
			record.DispatchToken = operationID
			record.UpdatedAt = now
			if saveError := transaction.Save(&record).Error; saveError != nil {
				return saveError
			}
		}
		return nil
	})
	return record, generation, recoverOperation, transactionError
}

func (service *mediaOperationService) markDispatched(operationID string, generation uint64, dispatchToken string) error {
	now := service.store.now()
	result := service.store.database.Model(&mediaOperationRecord{}).
		Where("operation_id = ? AND public_state = ? AND provider_execution_state = ? AND EXISTS (SELECT 1 FROM media_operation_claim_records WHERE operation_id = ? AND generation = ?)", operationID, MediaOperationStateRunning, MediaProviderExecutionNotDispatched, operationID, generation).
		Updates(map[string]any{"provider_execution_state": MediaProviderExecutionDispatched, "dispatch_token": dispatchToken, "updated_at": now})
	if result.Error != nil || result.RowsAffected != 1 {
		return errMediaOperationStore
	}
	return nil
}

func (service *mediaOperationService) finish(operationID string, generation uint64, result MediaOperationExecutionResult) {
	publicState := result.State
	providerState := MediaProviderExecutionUncertain
	publicError := callerSafeMediaOperationError(result.ErrorCode)
	switch publicState {
	case MediaOperationStateSucceeded:
		providerState = MediaProviderExecutionSucceeded
		publicError = ""
	case MediaOperationStateFailed:
		providerState = MediaProviderExecutionFailed
		if publicError == "" {
			publicError = "provider_error"
		}
	default:
		publicState = MediaOperationStateUncertain
		providerState = MediaProviderExecutionUncertain
		if publicError == "" {
			publicError = "provider_outcome_unknown"
		}
	}
	if publicState == MediaOperationStateSucceeded && !service.validOutputs(result.Outputs) {
		publicState = MediaOperationStateFailed
		providerState = MediaProviderExecutionSucceeded
		publicError = "provider_result_invalid"
	}
	now := service.store.now()
	service.assets.referenceMutex.Lock()
	defer service.assets.referenceMutex.Unlock()
	_ = service.store.database.Transaction(func(transaction *gorm.DB) error {
		var claim mediaOperationClaimRecord
		if claimError := transaction.First(&claim, "operation_id = ? AND generation = ?", operationID, generation).Error; claimError != nil {
			return claimError
		}
		var record mediaOperationRecord
		if recordError := transaction.First(&record, "operation_id = ? AND public_state = ?", operationID, MediaOperationStateRunning).Error; recordError != nil {
			return recordError
		}
		if publicState == MediaOperationStateSucceeded {
			requestTenant := tenant{identifier: tenantID(record.TenantID)}
			for outputIndex, output := range result.Outputs {
				metadata, outputError := service.assets.upload(requestTenant, strings.ToLower(strings.TrimSpace(output.MIMEType)), bytes.NewReader(output.Data))
				if outputError != nil {
					publicState = MediaOperationStateFailed
					providerState = MediaProviderExecutionSucceeded
					publicError = "asset_publication_failed"
					break
				}
				reference := mediaOperationAssetReferenceRecord{OperationID: operationID, Role: "output", Ordinal: outputIndex, TenantID: record.TenantID, AssetID: metadata.AssetID, MIMEType: metadata.MIMEType, SizeBytes: metadata.SizeBytes, Active: true, CreatedAt: now}
				if referenceError := transaction.Create(&reference).Error; referenceError != nil {
					return referenceError
				}
			}
		}
		record.PublicState = publicState
		record.ProviderExecutionState = providerState
		record.ProviderHandle = strings.TrimSpace(result.ProviderHandle)
		record.PublicErrorCode = publicError
		record.TerminalAt = &now
		record.UpdatedAt = now
		if saveError := transaction.Save(&record).Error; saveError != nil {
			return saveError
		}
		if referenceError := transaction.Model(&mediaOperationAssetReferenceRecord{}).Where("operation_id = ? AND role = ?", operationID, "input").Update("active", false).Error; referenceError != nil {
			return referenceError
		}
		if usageError := deliverMediaOperationUsage(transaction, record, now); usageError != nil {
			return usageError
		}
		return transaction.Delete(&claim).Error
	})
}

func (service *mediaOperationService) validOutputs(outputs []MediaOperationOutput) bool {
	if len(outputs) == 0 {
		return false
	}
	for _, output := range outputs {
		if !supportedTenantAssetMIME(strings.ToLower(strings.TrimSpace(output.MIMEType))) || len(output.Data) == 0 || int64(len(output.Data)) > service.assets.maxAssetBytes {
			return false
		}
	}
	return true
}

func (service *mediaOperationService) deliverPendingUsage() {
	var records []mediaOperationRecord
	if queryError := service.store.database.Where("public_state IN ? AND operation_id NOT IN (SELECT operation_id FROM media_operation_usage_delivery_records)", []string{MediaOperationStateSucceeded, MediaOperationStateFailed, MediaOperationStateCancelled, MediaOperationStateUncertain}).Find(&records).Error; queryError != nil {
		return
	}
	for _, record := range records {
		now := service.store.now()
		_ = service.store.database.Transaction(func(transaction *gorm.DB) error {
			return deliverMediaOperationUsage(transaction, record, now)
		})
	}
}

func (service *mediaOperationService) expireTerminalData() {
	cutoff := service.store.now().Add(-service.terminalRetention)
	var records []mediaOperationRecord
	if queryError := service.store.database.Where("terminal_at IS NOT NULL AND terminal_at <= ? AND public_state != ?", cutoff, MediaOperationStateUncertain).Find(&records).Error; queryError != nil {
		return
	}
	for _, record := range records {
		_ = service.store.database.Transaction(func(transaction *gorm.DB) error {
			tombstone := mediaOperationTombstoneRecord{
				TenantID: record.TenantID, IdempotencyKeyDigest: record.IdempotencyKeyDigest, IntentDigest: record.IntentDigest,
				OperationID: record.OperationID, TerminalState: record.PublicState, CreatedAt: service.store.now(),
			}
			if createError := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&tombstone).Error; createError != nil {
				return createError
			}
			if deleteError := transaction.Where("operation_id = ?", record.OperationID).Delete(&mediaOperationAssetReferenceRecord{}).Error; deleteError != nil {
				return deleteError
			}
			if deleteError := transaction.Where("operation_id = ?", record.OperationID).Delete(&mediaOperationClaimRecord{}).Error; deleteError != nil {
				return deleteError
			}
			return transaction.Delete(&record).Error
		})
	}
}

func deliverMediaOperationUsage(transaction *gorm.DB, record mediaOperationRecord, now time.Time) error {
	var existing int64
	if countError := transaction.Model(&mediaOperationUsageDeliveryRecord{}).Where("operation_id = ?", record.OperationID).Count(&existing).Error; countError != nil {
		return countError
	}
	if existing != 0 {
		return nil
	}
	statusCode := http.StatusBadGateway
	disposition := managedUsageDispositionFailed
	outcomeCode := managedUsageOutcomeUpstreamError
	if record.PublicState == MediaOperationStateSucceeded {
		statusCode = http.StatusOK
		disposition = managedUsageDispositionSucceeded
		outcomeCode = managedUsageOutcomeSuccess
	} else if record.PublicState == MediaOperationStateCancelled {
		statusCode = statusClientClosedRequest
		outcomeCode = managedUsageOutcomeRequestTimeout
	}
	usageEvent := managedUsageEventRecord{
		TenantID: record.TenantID, Endpoint: usageEndpointMedia, ProviderID: record.Provider, ModelID: record.Model,
		StatusCode: statusCode, Disposition: disposition, OutcomeCode: outcomeCode,
		LatencyMilliseconds: now.Sub(record.AcceptedAt).Milliseconds(), CreatedAt: now,
	}
	if createError := transaction.Create(&usageEvent).Error; createError != nil {
		return createError
	}
	usage := mediaOperationUsageDeliveryRecord{OperationID: record.OperationID, TenantID: record.TenantID, DeliveredAt: &now, CreatedAt: now}
	return transaction.Create(&usage).Error
}

func callerSafeMediaOperationError(errorCode string) string {
	switch strings.TrimSpace(errorCode) {
	case "provider_error", "provider_rate_limited", "operation_deadline_exceeded", "provider_outcome_unknown", "worker_claim_lost", "asset_publication_failed", "provider_result_invalid", errMediaOperationUnavailable.Error():
		return strings.TrimSpace(errorCode)
	default:
		return ""
	}
}

func executionRequestFromRecord(record mediaOperationRecord) MediaOperationExecutionRequest {
	return MediaOperationExecutionRequest{
		OperationID: record.OperationID, DispatchToken: record.DispatchToken, Capability: record.Capability,
		Provider: record.Provider, Model: record.Model, Input: append(json.RawMessage(nil), record.NormalizedInput...),
		Controls: append(json.RawMessage(nil), record.NormalizedControls...), ProviderHandle: record.ProviderHandle,
	}
}

func catalogOperationForMediaCapability(capability string) string {
	switch capability {
	case llmproxycontract.MediaCapabilityVideoGenerate:
		return ModelOperationVideoGeneration
	default:
		return ""
	}
}

func mediaCapabilityForCatalogOperation(operation string) string {
	switch operation {
	case ModelOperationVideoGeneration:
		return llmproxycontract.MediaCapabilityVideoGenerate
	default:
		return ""
	}
}

func (store *mediaOperationStore) credentialReference(requestContext context.Context, tenantID string, provider providerID) (string, error) {
	var assignment managedTenantConnectionRecord
	lookupError := store.database.WithContext(requestContext).Preload("Connection").Where("tenant_id = ? AND provider_id = ?", tenantID, provider.string()).First(&assignment).Error
	if errors.Is(lookupError, gorm.ErrRecordNotFound) {
		return "", errMediaOperationUnavailable
	}
	if lookupError != nil || assignment.ConnectionID == "" || assignment.Connection.Version == 0 {
		return "", errMediaOperationStore
	}
	return assignment.ConnectionID + ":v" + strconv.FormatUint(assignment.Connection.Version, 10), nil
}

func canonicalJSONObject(rawValue json.RawMessage) (json.RawMessage, error) {
	if len(rawValue) == 0 {
		rawValue = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(rawValue))
	decoder.UseNumber()
	var value map[string]any
	if decodeError := decoder.Decode(&value); decodeError != nil || decoder.Decode(&struct{}{}) != io.EOF || value == nil {
		return nil, errMediaOperationInvalid
	}
	canonical, _ := json.Marshal(value)
	return canonical, nil
}

func mediaSHA256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func newMediaOperationIdentifier() string {
	randomBytes := make([]byte, 16)
	_, _ = rand.Read(randomBytes)
	return "mop_" + hex.EncodeToString(randomBytes)
}

func writeMediaOperationError(ginContext *gin.Context, operationError error) {
	statusCode := http.StatusBadRequest
	code := errMediaOperationInvalid.Error()
	var expired mediaOperationExpiredError
	if errors.As(operationError, &expired) {
		ginContext.JSON(http.StatusGone, mediaOperationExpiredEnvelope{OperationID: expired.operationID, State: expired.state, Error: mediaOperationErrorResponse{Code: errMediaOperationExpired.Error()}})
		return
	}
	switch {
	case errors.Is(operationError, errMediaOperationUnavailable):
		statusCode, code = http.StatusUnprocessableEntity, errMediaOperationUnavailable.Error()
	case errors.Is(operationError, errMediaOperationNotFound):
		statusCode, code = http.StatusNotFound, errMediaOperationNotFound.Error()
	case errors.Is(operationError, errMediaOperationIntentConflict):
		statusCode, code = http.StatusConflict, errMediaOperationIntentConflict.Error()
	case errors.Is(operationError, errMediaOperationCapacity):
		statusCode, code = http.StatusTooManyRequests, errMediaOperationCapacity.Error()
	case errors.Is(operationError, errMediaOperationStore):
		statusCode, code = http.StatusInternalServerError, errMediaOperationStore.Error()
	}
	ginContext.JSON(statusCode, mediaOperationErrorEnvelope{Error: mediaOperationErrorResponse{Code: code}})
}
