package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type mediaOperationInternalAdapter struct {
	validateResult MediaOperationValidatedRequest
	validateError  error
	executeResult  MediaOperationExecutionResult
	recoverResult  MediaOperationExecutionResult
	cancelResult   MediaOperationCancellationResult
	validateHook   func()
	executeHook    func(MediaOperationExecutionRequest)
	cancelHook     func()
}

type deploymentMediaOperationAdapter struct {
	*mediaOperationInternalAdapter
	credentialReference string
}

type internalMediaVoiceProvider struct {
	voices []MediaVoiceProviderRecord
	err    error
}

func (provider internalMediaVoiceProvider) DiscoverMediaVoices(context.Context) ([]MediaVoiceProviderRecord, error) {
	return provider.voices, provider.err
}

func (adapter *deploymentMediaOperationAdapter) MediaOperationCredentialReference() string {
	return adapter.credentialReference
}

func (adapter *mediaOperationInternalAdapter) Validate(request MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	if adapter.validateHook != nil {
		adapter.validateHook()
	}
	if adapter.validateResult.Input == nil {
		adapter.validateResult.Input = request.Input
	}
	if adapter.validateResult.Controls == nil {
		adapter.validateResult.Controls = request.Controls
	}
	return adapter.validateResult, adapter.validateError
}

func (adapter *mediaOperationInternalAdapter) Execute(_ context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	if adapter.executeHook != nil {
		adapter.executeHook(request)
	}
	return adapter.executeResult
}

func (adapter *mediaOperationInternalAdapter) Recover(context.Context, MediaOperationExecutionRequest) MediaOperationExecutionResult {
	return adapter.recoverResult
}

func (adapter *mediaOperationInternalAdapter) Cancel(context.Context, MediaOperationExecutionRequest) MediaOperationCancellationResult {
	if adapter.cancelHook != nil {
		adapter.cancelHook()
	}
	return adapter.cancelResult
}

type mediaOperationInternalFixture struct {
	service  *mediaOperationService
	tenant   tenant
	database *gorm.DB
	adapter  *mediaOperationInternalAdapter
	now      time.Time
}

func newMediaOperationInternalFixture(testingInstance *testing.T) mediaOperationInternalFixture {
	testingInstance.Helper()
	database, databaseError := gorm.Open(sqlite.Open(filepath.Join(testingInstance.TempDir(), "media-operations.db")), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if databaseError != nil {
		testingInstance.Fatal(databaseError)
	}
	if migrationError := database.AutoMigrate(&mediaOperationRecord{}, &mediaOperationClaimRecord{}, &mediaOperationAssetReferenceRecord{}, &mediaOperationUsageDeliveryRecord{}, &mediaOperationTombstoneRecord{}, &mediaVoiceRecord{}, &managedUsageEventRecord{}, &managedAccountConnectionRecord{}, &managedTenantConnectionRecord{}); migrationError != nil {
		testingInstance.Fatal(migrationError)
	}
	providerCatalog := internalCanonicalProviderCatalog()
	catalog, catalogError := NewCatalogService(internalTestModelCatalog(ProviderOffering{
		Provider: ProviderNameXAI, Model: "grok-imagine-video-1.5", ProviderModel: "grok-imagine-video-1.5",
		Operations: []string{ModelOperationVideoGeneration}, DefaultOperations: []string{ModelOperationVideoGeneration},
		WireContract: CatalogProtocolXAIVideosGenerations, ExecutionLifecycle: string(textExecutionLifecyclePollableResource),
		Controls: []CatalogControl{{ID: "enabled", Kind: CatalogControlBoolean}}, Limits: []CatalogLimit{{ID: "jobs", Unit: "requests", AccountDependent: true}},
	}))
	if catalogError != nil {
		testingInstance.Fatal(catalogError)
	}
	providers := newProviderRegistry(Configuration{ProviderCatalog: providerCatalog, Endpoints: NewEndpoints()})
	provider := providers.definitions[providerID(ProviderNameXAI)]
	connectionValues := make(map[string]string, len(provider.fields))
	configuredFields := make(map[string]bool, len(provider.fields))
	for fieldID, field := range provider.fields {
		if field.Default != nil && strings.TrimSpace(*field.Default) != "" {
			connectionValues[fieldID] = *field.Default
		} else {
			connectionValues[fieldID] = "internal-credential"
		}
		configuredFields[fieldID] = true
	}
	requestTenant := tenant{identifier: tenantID("media-internal-tenant"), providerSettings: map[providerID]managedProviderSettings{
		providerID(ProviderNameXAI): {connectionValues: connectionValues, configuredFields: configuredFields},
	}}
	connection := managedAccountConnectionRecord{ID: "connection-internal", OwnerUserID: "owner", ProviderID: ProviderNameXAI, Name: "Internal", Version: 3}
	if createError := database.Create(&connection).Error; createError != nil {
		testingInstance.Fatal(createError)
	}
	if createError := database.Create(&managedTenantConnectionRecord{TenantID: requestTenant.identifier.string(), ProviderID: ProviderNameXAI, ConnectionID: connection.ID}).Error; createError != nil {
		testingInstance.Fatal(createError)
	}
	now := time.Date(2026, 9, 10, 18, 0, 0, 0, time.UTC)
	store := &mediaOperationStore{database: database, now: func() time.Time { return now }}
	assets := newTenantAssetStore(testingInstance.TempDir(), 32, 3600)
	assets.now = func() time.Time { return now }
	adapter := &mediaOperationInternalAdapter{
		executeResult: MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: []MediaOperationOutput{{MIMEType: "video/mp4", Data: []byte("video")}}},
		recoverResult: MediaOperationExecutionResult{State: MediaOperationStateUncertain},
		cancelResult:  MediaOperationCancellationResult{State: MediaCancellationUnsupported},
	}
	service := &mediaOperationService{
		store: store, assets: assets, adapters: map[string]MediaOperationAdapter{mediaOperationAdapterKey(llmproxycontract.MediaCapabilityVideoGenerate, ProviderNameXAI, "grok-imagine-video-1.5"): adapter},
		catalog: catalog, providers: providers, queue: make(chan string, 1), claimLifetime: time.Minute, claimRenewal: time.Millisecond,
		lifetime: time.Hour, globalCapacity: 2, tenantCapacity: 2, terminalRetention: time.Hour,
	}
	if _, offeringError := catalog.ResolveOffering(ProviderNameXAI, "grok-imagine-video-1.5"); offeringError != nil {
		testingInstance.Fatalf("media fixture offering: %v", offeringError)
	}
	resolvedProvider, providerError := providers.forTenant(requestTenant).resolveProvider(ProviderNameXAI, "")
	if providerError != nil || !requestTenant.providerSettings[providerID(ProviderNameXAI)].hasRequiredConnectionFields(resolvedProvider) {
		testingInstance.Fatalf("media fixture provider=%+v error=%v settings=%+v", resolvedProvider, providerError, requestTenant.providerSettings[providerID(ProviderNameXAI)])
	}
	if _, credentialError := store.credentialReference(context.Background(), requestTenant.identifier.string(), providerID(ProviderNameXAI)); credentialError != nil {
		testingInstance.Fatalf("media fixture credential: %v", credentialError)
	}
	return mediaOperationInternalFixture{service: service, tenant: requestTenant, database: database, adapter: adapter, now: now}
}

func (fixture mediaOperationInternalFixture) payload() mediaOperationCreatePayload {
	return mediaOperationCreatePayload{Capability: llmproxycontract.MediaCapabilityVideoGenerate, Provider: ProviderNameXAI, Model: "grok-imagine-video-1.5", Input: json.RawMessage(`{"prompt":"internal"}`), Controls: json.RawMessage(`{}`)}
}

func (fixture mediaOperationInternalFixture) record(state string, providerState string) mediaOperationRecord {
	fixtureRecord := mediaOperationRecord{
		OperationID: newMediaOperationIdentifier(), TenantID: fixture.tenant.identifier.string(), IdempotencyKeyDigest: mediaSHA256Hex([]byte(newMediaOperationIdentifier())), IntentDigest: mediaSHA256Hex([]byte("intent")),
		Capability: llmproxycontract.MediaCapabilityVideoGenerate, CatalogOperation: ModelOperationVideoGeneration, Provider: ProviderNameXAI, Model: "grok-imagine-video-1.5", CatalogRevision: fixture.service.catalog.Revision(), CredentialReference: "connection-internal:v3",
		NormalizedInput: []byte(`{}`), NormalizedControls: []byte(`{}`), PublicState: state, ProviderExecutionState: providerState, CancellationState: MediaCancellationNotRequested,
		AcceptedAt: fixture.now.Add(-time.Minute), UpdatedAt: fixture.now.Add(-time.Minute), DeadlineAt: fixture.now.Add(time.Hour),
	}
	if createError := fixture.database.Create(&fixtureRecord).Error; createError != nil {
		panic(createError)
	}
	return fixtureRecord
}

func TestMediaOperationHelperContracts(testingInstance *testing.T) {
	if (mediaOperationExpiredError{}).Error() != errMediaOperationExpired.Error() {
		testingInstance.Fatal("expired error contract")
	}
	if catalogOperationForMediaCapability("future") != "" || mediaCapabilityForCatalogOperation("future") != "" {
		testingInstance.Fatal("unknown operation mapped")
	}
	if canonical, canonicalError := canonicalJSONObject(nil); canonicalError != nil || string(canonical) != "{}" {
		testingInstance.Fatalf("canonical=%s error=%v", canonical, canonicalError)
	}
	for _, rawValue := range []json.RawMessage{json.RawMessage(`[]`), json.RawMessage(`{} {}`), json.RawMessage(`null`)} {
		if _, canonicalError := canonicalJSONObject(rawValue); !errors.Is(canonicalError, errMediaOperationInvalid) {
			testingInstance.Fatalf("raw=%s error=%v", rawValue, canonicalError)
		}
	}
	fixture := newMediaOperationInternalFixture(testingInstance)
	providers := fixture.service.providers
	if !managedUsageRouteIsCurrent(managedUsageEventRecord{Endpoint: usageEndpointMedia, ProviderID: ProviderNameXAI, ModelID: "grok-imagine-video-1.5"}, providers) || managedUsageRouteIsCurrent(managedUsageEventRecord{Endpoint: usageEndpointMedia, ProviderID: ProviderNameXAI, ModelID: "missing"}, providers) {
		testingInstance.Fatal("media usage route validation mismatch")
	}
	for _, outputs := range [][]MediaOperationOutput{
		nil,
		{{MIMEType: "text/plain", Data: []byte("x")}},
		{{MIMEType: "video/mp4"}},
		{{MIMEType: "video/mp4", Data: bytes.Repeat([]byte("x"), 33)}},
	} {
		if fixture.service.validOutputs(outputs, nil) {
			testingInstance.Fatalf("invalid outputs accepted=%+v", outputs)
		}
	}
	if !fixture.service.validOutputs([]MediaOperationOutput{{MIMEType: " VIDEO/MP4 ", Data: []byte("x")}}, nil) {
		testingInstance.Fatal("valid output rejected")
	}
	if callerSafeMediaOperationError(" provider_rate_limited ") != "provider_rate_limited" || callerSafeMediaOperationError("native private failure") != "" {
		testingInstance.Fatal("caller-safe error mapping mismatch")
	}

	for operationError, expectedStatus := range map[error]int{
		errMediaOperationInvalid: http.StatusBadRequest, errMediaOperationUnavailable: http.StatusUnprocessableEntity,
		errMediaOperationNotFound: http.StatusNotFound, errMediaOperationIntentConflict: http.StatusConflict,
		errMediaOperationCapacity: http.StatusTooManyRequests, errMediaOperationStore: http.StatusInternalServerError,
		mediaOperationExpiredError{operationID: "mop_0123456789abcdef0123456789abcdef", state: MediaOperationStateSucceeded}: http.StatusGone,
	} {
		response := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(response)
		writeMediaOperationError(ginContext, operationError)
		if response.Code != expectedStatus {
			testingInstance.Fatalf("error=%v status=%d", operationError, response.Code)
		}
	}
}

func TestMediaOperationCapabilitiesFilterAndOrderRoutes(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	fixture.service.catalog.catalog.offerings = map[string]ProviderOffering{
		"xai:z":     {Provider: ProviderNameXAI, Model: "z", Operations: []string{ModelOperationVideoGeneration}, Controls: []CatalogControl{}, Limits: []CatalogLimit{}},
		"xai:a":     {Provider: ProviderNameXAI, Model: "a", Operations: []string{ModelOperationVideoGeneration}, Controls: []CatalogControl{}, Limits: []CatalogLimit{}},
		"missing:x": {Provider: "missing", Model: "x", Operations: []string{ModelOperationVideoGeneration}, Controls: []CatalogControl{}, Limits: []CatalogLimit{}},
	}
	fixture.service.adapters = map[string]MediaOperationAdapter{
		mediaOperationAdapterKey(llmproxycontract.MediaCapabilityVideoGenerate, ProviderNameXAI, "z"): fixture.adapter,
		mediaOperationAdapterKey(llmproxycontract.MediaCapabilityVideoGenerate, ProviderNameXAI, "a"): fixture.adapter,
		mediaOperationAdapterKey(llmproxycontract.MediaCapabilityVideoGenerate, "missing", "x"):       fixture.adapter,
	}
	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ginContext.Set(contextKeyTenant, fixture.tenant)
	fixture.service.capabilitiesHandler()(ginContext)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"model":"a"`) || strings.Index(response.Body.String(), `"model":"a"`) > strings.Index(response.Body.String(), `"model":"z"`) || strings.Contains(response.Body.String(), `"provider":"missing"`) {
		testingInstance.Fatalf("capabilities=%s", response.Body.String())
	}
}

func TestMediaOperationQueueDeduplicatesAndRetriesCapacity(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	fixture.service.enqueue("first")
	fixture.service.enqueue("first")
	if queued := <-fixture.service.queue; queued != "first" {
		testingInstance.Fatalf("queued=%q", queued)
	}
	fixture.service.queued.Delete("first")
	fixture.service.queue <- "blocking"
	fixture.service.enqueue("dropped")
	if _, retained := fixture.service.queued.Load("dropped"); retained {
		testingInstance.Fatal("full queue retained dropped marker")
	}
}

func failNthMediaGORMOperation(testingInstance *testing.T, database *gorm.DB, operation string, table string, occurrence int) {
	testingInstance.Helper()
	seen := 0
	callback := func(callbackDatabase *gorm.DB) {
		if callbackDatabase.Statement.Table != table {
			return
		}
		seen++
		if seen == occurrence {
			callbackDatabase.AddError(errInternalTestDatabase)
		}
	}
	name := "media-failure-" + operation + "-" + table
	var registrationError error
	switch operation {
	case "query":
		registrationError = database.Callback().Query().Before("gorm:query").Register(name, callback)
	case "create":
		registrationError = database.Callback().Create().Before("gorm:create").Register(name, callback)
	case "update":
		registrationError = database.Callback().Update().Before("gorm:update").Register(name, callback)
	case "delete":
		registrationError = database.Callback().Delete().Before("gorm:delete").Register(name, callback)
	default:
		testingInstance.Fatalf("unsupported operation %q", operation)
	}
	if registrationError != nil {
		testingInstance.Fatal(registrationError)
	}
}

func TestMediaOperationCreationPersistenceFailures(testingInstance *testing.T) {
	testCases := []struct {
		name       string
		operation  string
		table      string
		occurrence int
	}{
		{name: "accepted lookup", operation: "query", table: "media_operation_records", occurrence: 1},
		{name: "tombstone lookup", operation: "query", table: "media_operation_tombstone_records", occurrence: 1},
		{name: "connection lookup", operation: "query", table: "managed_tenant_connection_records", occurrence: 1},
		{name: "transaction lookup", operation: "query", table: "media_operation_records", occurrence: 2},
		{name: "global capacity", operation: "query", table: "media_operation_records", occurrence: 3},
		{name: "tenant capacity", operation: "query", table: "media_operation_records", occurrence: 4},
		{name: "operation create", operation: "create", table: "media_operation_records", occurrence: 1},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(testingInstance *testing.T) {
			fixture := newMediaOperationInternalFixture(testingInstance)
			failNthMediaGORMOperation(testingInstance, fixture.database, testCase.operation, testCase.table, testCase.occurrence)
			if _, _, createError := fixture.service.create(context.Background(), fixture.tenant, "failure", fixture.payload()); !errors.Is(createError, errMediaOperationStore) {
				testingInstance.Fatalf("error=%v", createError)
			}
		})
	}
	testingInstance.Run("asset reference", func(testingInstance *testing.T) {
		fixture := newMediaOperationInternalFixture(testingInstance)
		asset, uploadError := fixture.service.assets.upload(fixture.tenant, "image/png", strings.NewReader("input"))
		if uploadError != nil {
			testingInstance.Fatal(uploadError)
		}
		fixture.adapter.validateResult.InputAssetIDs = []string{asset.AssetID}
		failNthMediaGORMOperation(testingInstance, fixture.database, "create", "media_operation_asset_reference_records", 1)
		if _, _, createError := fixture.service.create(context.Background(), fixture.tenant, "reference", fixture.payload()); !errors.Is(createError, errMediaOperationStore) {
			testingInstance.Fatal(createError)
		}
	})
	testingInstance.Run("provider", func(testingInstance *testing.T) {
		fixture := newMediaOperationInternalFixture(testingInstance)
		fixture.service.providers = &providerRegistry{definitions: map[providerID]providerDefinition{}}
		if _, _, createError := fixture.service.create(context.Background(), fixture.tenant, "provider", fixture.payload()); !errors.Is(createError, errMediaOperationUnavailable) {
			testingInstance.Fatal(createError)
		}
	})
}

func TestMediaOperationCreationTransactionConvergesConcurrentAcceptance(testingInstance *testing.T) {
	for _, conflict := range []bool{false, true} {
		testingInstance.Run(map[bool]string{false: "same", true: "changed"}[conflict], func(testingInstance *testing.T) {
			fixture := newMediaOperationInternalFixture(testingInstance)
			payload := fixture.payload()
			canonicalInput, _ := canonicalJSONObject(payload.Input)
			canonicalControls, _ := canonicalJSONObject(payload.Controls)
			intentBytes, _ := json.Marshal(mediaOperationCreatePayload{Capability: payload.Capability, Provider: payload.Provider, Model: payload.Model, Input: canonicalInput, Controls: canonicalControls})
			intentDigest := mediaSHA256Hex(intentBytes)
			if conflict {
				intentDigest = "changed"
			}
			fixture.adapter.validateHook = func() {
				record := mediaOperationRecord{
					OperationID: newMediaOperationIdentifier(), TenantID: fixture.tenant.identifier.string(), IdempotencyKeyDigest: mediaSHA256Hex([]byte("race")), IntentDigest: intentDigest,
					Capability: payload.Capability, CatalogOperation: ModelOperationVideoGeneration, Provider: payload.Provider, Model: payload.Model, CatalogRevision: fixture.service.catalog.Revision(), CredentialReference: "connection-internal:v3",
					NormalizedInput: canonicalInput, NormalizedControls: canonicalControls, PublicState: MediaOperationStateQueued, ProviderExecutionState: MediaProviderExecutionNotDispatched, CancellationState: MediaCancellationNotRequested,
					AcceptedAt: fixture.now, UpdatedAt: fixture.now, DeadlineAt: fixture.now.Add(time.Hour),
				}
				if createError := fixture.database.Create(&record).Error; createError != nil {
					testingInstance.Fatal(createError)
				}
			}
			operation, created, createError := fixture.service.create(context.Background(), fixture.tenant, "race", payload)
			if conflict {
				if !errors.Is(createError, errMediaOperationIntentConflict) {
					testingInstance.Fatal(createError)
				}
			} else if createError != nil || created || operation.OperationID == "" {
				testingInstance.Fatalf("operation=%+v created=%t error=%v", operation, created, createError)
			}
		})
	}
}

func TestMediaOperationServiceRejectsInvalidAdapterCatalog(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	managedTenants := &managedTenantStore{database: &gormManagedTenantDatabase{database: fixture.database}}
	configuration := Configuration{
		ModelCatalog: ModelCatalog{}, MediaOperationAdapters: map[string]MediaOperationAdapter{"video.generate|xai|model": fixture.adapter},
		MediaOperationWorkers: 1, MediaOperationCapacity: 1, TenantMediaOperationCapacity: 1,
		MediaOperationLifetimeSeconds: 60, MediaOperationClaimSeconds: 10, MediaOperationClaimRenewalSeconds: 1, AssetRetentionSeconds: 60,
	}
	if service, serviceError := newMediaOperationService(configuration, managedTenants, fixture.service.assets, fixture.service.providers); service != nil || serviceError == nil {
		testingInstance.Fatalf("service=%v error=%v", service, serviceError)
	}
	configuration.validated = true
	configuration.Endpoints = NewEndpoints()
	configuration.ProviderCatalog = internalCanonicalProviderCatalog()
	configuration.WorkerCount = 1
	configuration.QueueSize = 1
	configuration.AssetStorePath = testingInstance.TempDir()
	if router, routerError := buildRouter(configuration, zap.NewNop().Sugar(), func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
		return managedTenants, nil
	}); router != nil || routerError == nil {
		testingInstance.Fatalf("router=%v error=%v", router, routerError)
	}
}

func TestMediaOperationCreateHandlerRejectsHeadersAndResponseStoreFailure(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	ginContext.Set(contextKeyTenant, fixture.tenant)
	fixture.service.createHandler()(ginContext)
	if response.Code != http.StatusBadRequest {
		testingInstance.Fatalf("missing key status=%d", response.Code)
	}

	fixture = newMediaOperationInternalFixture(testingInstance)
	failNthMediaGORMOperation(testingInstance, fixture.database, "query", "media_operation_asset_reference_records", 1)
	response = httptest.NewRecorder()
	ginContext, _ = gin.CreateTestContext(response)
	requestBody, _ := json.Marshal(fixture.payload())
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(requestBody))
	ginContext.Request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "response-failure")
	ginContext.Set(contextKeyTenant, fixture.tenant)
	fixture.service.createHandler()(ginContext)
	if response.Code != http.StatusInternalServerError {
		testingInstance.Fatalf("response failure status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMediaOperationCancellationPersistenceFailures(testingInstance *testing.T) {
	runningRecord := func(fixture mediaOperationInternalFixture) mediaOperationRecord {
		record := fixture.record(MediaOperationStateRunning, MediaProviderExecutionDispatched)
		record.CancellationState = MediaCancellationRequested
		if updateError := fixture.database.Save(&record).Error; updateError != nil {
			testingInstance.Fatal(updateError)
		}
		return record
	}
	testCases := []struct {
		name     string
		prepare  func(mediaOperationInternalFixture, mediaOperationRecord)
		expected error
	}{
		{name: "initial save", prepare: func(fixture mediaOperationInternalFixture, record mediaOperationRecord) {
			record.CancellationState = MediaCancellationNotRequested
			if updateError := fixture.database.Save(&record).Error; updateError != nil {
				testingInstance.Fatal(updateError)
			}
			failNthMediaGORMOperation(testingInstance, fixture.database, "update", "media_operation_records", 1)
		}, expected: errMediaOperationStore},
		{name: "post-request read", prepare: func(fixture mediaOperationInternalFixture, _ mediaOperationRecord) {
			failNthMediaGORMOperation(testingInstance, fixture.database, "query", "media_operation_records", 2)
		}, expected: errMediaOperationStore},
		{name: "adapter unavailable", prepare: func(fixture mediaOperationInternalFixture, _ mediaOperationRecord) {
			fixture.service.adapters = map[string]MediaOperationAdapter{}
		}, expected: errMediaOperationUnavailable},
		{name: "confirmation read", prepare: func(fixture mediaOperationInternalFixture, _ mediaOperationRecord) {
			fixture.adapter.cancelResult.State = MediaCancellationConfirmed
			failNthMediaGORMOperation(testingInstance, fixture.database, "query", "media_operation_records", 3)
		}, expected: errMediaOperationStore},
		{name: "confirmation persist", prepare: func(fixture mediaOperationInternalFixture, _ mediaOperationRecord) {
			fixture.adapter.cancelResult.State = MediaCancellationConfirmed
			failNthMediaGORMOperation(testingInstance, fixture.database, "update", "media_operation_records", 1)
		}, expected: errMediaOperationStore},
		{name: "unsupported persist", prepare: func(fixture mediaOperationInternalFixture, _ mediaOperationRecord) {
			failNthMediaGORMOperation(testingInstance, fixture.database, "update", "media_operation_records", 1)
		}, expected: errMediaOperationStore},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(testingInstance *testing.T) {
			fixture := newMediaOperationInternalFixture(testingInstance)
			record := runningRecord(fixture)
			testCase.prepare(fixture, record)
			if _, cancelError := fixture.service.cancel(context.Background(), fixture.tenant, record.OperationID); !errors.Is(cancelError, testCase.expected) {
				testingInstance.Fatalf("error=%v want=%v", cancelError, testCase.expected)
			}
		})
	}
	testingInstance.Run("terminal race", func(testingInstance *testing.T) {
		fixture := newMediaOperationInternalFixture(testingInstance)
		record := runningRecord(fixture)
		fixture.adapter.cancelResult.State = MediaCancellationConfirmed
		fixture.adapter.cancelHook = func() {
			if updateError := fixture.database.Model(&mediaOperationRecord{}).Where("operation_id = ?", record.OperationID).Update("public_state", MediaOperationStateSucceeded).Error; updateError != nil {
				testingInstance.Fatal(updateError)
			}
		}
		response, cancelError := fixture.service.cancel(context.Background(), fixture.tenant, record.OperationID)
		if cancelError != nil || response.State != MediaOperationStateSucceeded {
			testingInstance.Fatalf("response=%+v error=%v", response, cancelError)
		}
	})
}

func TestMediaOperationCancellationFinalizerFailures(testingInstance *testing.T) {
	testCases := []struct {
		name       string
		operation  string
		table      string
		occurrence int
	}{
		{name: "record", operation: "update", table: "media_operation_records", occurrence: 1},
		{name: "references", operation: "update", table: "media_operation_asset_reference_records", occurrence: 1},
		{name: "usage", operation: "query", table: "media_operation_usage_delivery_records", occurrence: 1},
		{name: "claim", operation: "delete", table: "media_operation_claim_records", occurrence: 1},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(testingInstance *testing.T) {
			fixture := newMediaOperationInternalFixture(testingInstance)
			record := fixture.record(MediaOperationStateQueued, MediaProviderExecutionNotDispatched)
			record.PublicState = MediaOperationStateCancelled
			record.CancellationState = MediaCancellationConfirmed
			record.TerminalAt = &fixture.now
			failNthMediaGORMOperation(testingInstance, fixture.database, testCase.operation, testCase.table, testCase.occurrence)
			finalizeError := fixture.database.Transaction(func(transaction *gorm.DB) error {
				return finalizeMediaOperationCancellation(transaction, record, fixture.now)
			})
			if finalizeError == nil {
				testingInstance.Fatal("expected finalizer failure")
			}
		})
	}
}

func TestMediaOperationClaimFailureContracts(testingInstance *testing.T) {
	testingInstance.Run("missing", func(testingInstance *testing.T) {
		fixture := newMediaOperationInternalFixture(testingInstance)
		if _, _, _, claimError := fixture.service.claim("worker", "mop_00000000000000000000000000000000"); claimError == nil {
			testingInstance.Fatal("expected missing claim failure")
		}
	})
	testingInstance.Run("terminal", func(testingInstance *testing.T) {
		fixture := newMediaOperationInternalFixture(testingInstance)
		record := fixture.record(MediaOperationStateSucceeded, MediaProviderExecutionSucceeded)
		if _, _, _, claimError := fixture.service.claim("worker", record.OperationID); !errors.Is(claimError, gorm.ErrRecordNotFound) {
			testingInstance.Fatal(claimError)
		}
	})
	for _, testCase := range []struct{ name, operation, table string }{
		{name: "claim read", operation: "query", table: "media_operation_claim_records"},
		{name: "claim create", operation: "create", table: "media_operation_claim_records"},
		{name: "record update", operation: "update", table: "media_operation_records"},
	} {
		testingInstance.Run(testCase.name, func(testingInstance *testing.T) {
			fixture := newMediaOperationInternalFixture(testingInstance)
			record := fixture.record(MediaOperationStateQueued, MediaProviderExecutionNotDispatched)
			failNthMediaGORMOperation(testingInstance, fixture.database, testCase.operation, testCase.table, 1)
			if _, _, _, claimError := fixture.service.claim("worker", record.OperationID); claimError == nil {
				testingInstance.Fatal("expected claim failure")
			}
		})
	}
}

func TestMediaOperationRunOperationFailureContracts(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	record := fixture.record(MediaOperationStateQueued, MediaProviderExecutionNotDispatched)
	fixture.service.adapters = map[string]MediaOperationAdapter{}
	fixture.service.runOperation("worker", record.OperationID)
	response, responseError := fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), record.OperationID)
	if responseError != nil || response.State != MediaOperationStateFailed || response.Error == nil || response.Error.Code != errMediaOperationUnavailable.Error() {
		testingInstance.Fatalf("response=%+v error=%v", response, responseError)
	}

	fixture = newMediaOperationInternalFixture(testingInstance)
	record = fixture.record(MediaOperationStateRunning, MediaProviderExecutionNotDispatched)
	failNthMediaGORMOperation(testingInstance, fixture.database, "update", "media_operation_records", 1)
	fixture.service.runOperation("worker", record.OperationID)
	response, responseError = fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), record.OperationID)
	if responseError != nil || response.State != MediaOperationStateRunning {
		testingInstance.Fatalf("response=%+v error=%v", response, responseError)
	}
}

func TestMediaOperationPersistsProviderHandleBeforeAdapterCompletion(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	fixture.service.claimRenewal = time.Hour
	record := fixture.record(MediaOperationStateQueued, MediaProviderExecutionNotDispatched)
	record.DeadlineAt = time.Now().UTC().Add(time.Hour)
	if updateError := fixture.database.Model(&record).Update("deadline_at", record.DeadlineAt).Error; updateError != nil {
		testingInstance.Fatal(updateError)
	}
	var hookError error
	fixture.adapter.executeHook = func(request MediaOperationExecutionRequest) {
		if request.TenantID != fixture.tenant.identifier.string() || request.PersistProviderHandle == nil {
			hookError = fmt.Errorf("execution request=%+v", request)
			return
		}
		if persistError := request.PersistProviderHandle(`{"version":1,"job_id":"native-job"}`); persistError != nil {
			hookError = fmt.Errorf("persist provider handle: %w", persistError)
			return
		}
		var persisted mediaOperationRecord
		if queryError := fixture.database.First(&persisted, "operation_id = ?", request.OperationID).Error; queryError != nil || !strings.Contains(persisted.ProviderHandle, "native-job") {
			hookError = fmt.Errorf("persisted handle=%q error=%v", persisted.ProviderHandle, queryError)
		}
	}
	fixture.adapter.executeResult = MediaOperationExecutionResult{State: MediaOperationStateFailed, ProviderHandle: `{"version":1,"job_id":"native-job"}`, ErrorCode: "provider_error"}
	fixture.service.runOperation("worker", record.OperationID)
	if hookError != nil {
		testingInstance.Fatal(hookError)
	}
	var completed mediaOperationRecord
	if queryError := fixture.database.First(&completed, "operation_id = ?", record.OperationID).Error; queryError != nil || !strings.Contains(completed.ProviderHandle, "native-job") {
		testingInstance.Fatalf("completed handle=%q error=%v", completed.ProviderHandle, queryError)
	}
}

func TestDictatorWorkerIsolationPreservesAdmittedCloudExecution(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	fixture.service.claimRenewal = time.Hour
	fixture.service.dictatorQueue = make(chan string, 2)
	cloud := fixture.record(MediaOperationStateQueued, MediaProviderExecutionNotDispatched)
	cloud.DeadlineAt = time.Now().UTC().Add(time.Hour)
	if saveError := fixture.database.Save(&cloud).Error; saveError != nil {
		testingInstance.Fatal(saveError)
	}
	dictator := fixture.record(MediaOperationStateQueued, MediaProviderExecutionNotDispatched)
	dictator.Provider = ProviderNameDictator
	dictator.Model = ModelNameDictatorSpeechV1
	dictator.Capability = llmproxycontract.MediaCapabilityAudioTranscribe
	dictator.CatalogOperation = ModelOperationAudioTranscription
	dictator.DeadlineAt = time.Now().UTC().Add(time.Hour)
	if saveError := fixture.database.Save(&dictator).Error; saveError != nil {
		testingInstance.Fatal(saveError)
	}
	dictatorStarted := make(chan struct{})
	releaseDictator := make(chan struct{})
	dictatorAdapter := &deploymentMediaOperationAdapter{mediaOperationInternalAdapter: &mediaOperationInternalAdapter{
		executeHook:   func(MediaOperationExecutionRequest) { close(dictatorStarted); <-releaseDictator },
		executeResult: MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"},
	}, credentialReference: "deployment:dictator:test"}
	fixture.service.adapters[mediaOperationAdapterKey(dictator.Capability, dictator.Provider, dictator.Model)] = dictatorAdapter
	cloudCompleted := make(chan struct{})
	fixture.adapter.executeHook = func(MediaOperationExecutionRequest) { close(cloudCompleted) }
	go fixture.service.runWorker("cloud-worker", fixture.service.queue)
	go fixture.service.runWorker("dictator-worker", fixture.service.dictatorQueue)
	defer close(fixture.service.queue)
	defer close(fixture.service.dictatorQueue)
	fixture.service.enqueue(dictator.OperationID)
	select {
	case <-dictatorStarted:
	case <-time.After(time.Second):
		testingInstance.Fatal("Dictator worker did not start")
	}
	fixture.service.enqueue(cloud.OperationID)
	select {
	case <-cloudCompleted:
	case <-time.After(time.Second):
		testingInstance.Fatal("admitted cloud operation was blocked by Dictator")
	}
	close(releaseDictator)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		response, responseError := fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), cloud.OperationID)
		if responseError == nil && response.State == MediaOperationStateSucceeded {
			return
		}
		time.Sleep(time.Millisecond)
	}
	testingInstance.Fatal("cloud operation did not complete")
}

func TestMediaOperationFinishPersistenceFailures(testingInstance *testing.T) {
	testCases := []struct {
		name       string
		operation  string
		table      string
		occurrence int
	}{
		{name: "record read", operation: "query", table: "media_operation_records", occurrence: 1},
		{name: "output reference", operation: "create", table: "media_operation_asset_reference_records", occurrence: 1},
		{name: "record save", operation: "update", table: "media_operation_records", occurrence: 1},
		{name: "input release", operation: "update", table: "media_operation_asset_reference_records", occurrence: 1},
		{name: "usage", operation: "query", table: "media_operation_usage_delivery_records", occurrence: 1},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(testingInstance *testing.T) {
			fixture := newMediaOperationInternalFixture(testingInstance)
			record := fixture.record(MediaOperationStateRunning, MediaProviderExecutionDispatched)
			if createError := fixture.database.Create(&mediaOperationClaimRecord{OperationID: record.OperationID, WorkerID: "worker", Generation: 1, ExpiresAt: fixture.now.Add(time.Minute)}).Error; createError != nil {
				testingInstance.Fatal(createError)
			}
			failNthMediaGORMOperation(testingInstance, fixture.database, testCase.operation, testCase.table, testCase.occurrence)
			fixture.service.finish(record.OperationID, 1, MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: []MediaOperationOutput{{MIMEType: "video/mp4", Data: []byte("output")}}})
		})
	}
	testingInstance.Run("asset publication", func(testingInstance *testing.T) {
		fixture := newMediaOperationInternalFixture(testingInstance)
		record := fixture.record(MediaOperationStateRunning, MediaProviderExecutionDispatched)
		if createError := fixture.database.Create(&mediaOperationClaimRecord{OperationID: record.OperationID, WorkerID: "worker", Generation: 1, ExpiresAt: fixture.now.Add(time.Minute)}).Error; createError != nil {
			testingInstance.Fatal(createError)
		}
		fixture.service.assets.cleanupError = errAssetEdge
		fixture.service.finish(record.OperationID, 1, MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: []MediaOperationOutput{{MIMEType: "video/mp4", Data: []byte("output")}}})
		response, responseError := fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), record.OperationID)
		if responseError != nil || response.State != MediaOperationStateFailed || response.Error == nil || response.Error.Code != "asset_publication_failed" {
			testingInstance.Fatalf("response=%+v error=%v", response, responseError)
		}
	})
}

func TestMediaOperationUsageAndRetentionPersistenceFailures(testingInstance *testing.T) {
	testingInstance.Run("resume query", func(testingInstance *testing.T) {
		fixture := newMediaOperationInternalFixture(testingInstance)
		failNthMediaGORMOperation(testingInstance, fixture.database, "query", "media_operation_records", 1)
		fixture.service.resumeOutstanding()
	})
	for _, testCase := range []struct{ name, operation, table string }{
		{name: "pending query", operation: "query", table: "media_operation_records"},
		{name: "retention query", operation: "query", table: "media_operation_records"},
	} {
		testingInstance.Run(testCase.name, func(testingInstance *testing.T) {
			fixture := newMediaOperationInternalFixture(testingInstance)
			failNthMediaGORMOperation(testingInstance, fixture.database, testCase.operation, testCase.table, 1)
			if testCase.name == "pending query" {
				fixture.service.deliverPendingUsage()
			} else {
				fixture.service.expireTerminalData()
			}
		})
	}
	for _, testCase := range []struct{ name, operation, table string }{
		{name: "tombstone", operation: "create", table: "media_operation_tombstone_records"},
		{name: "references", operation: "delete", table: "media_operation_asset_reference_records"},
		{name: "claims", operation: "delete", table: "media_operation_claim_records"},
		{name: "operation", operation: "delete", table: "media_operation_records"},
	} {
		testingInstance.Run("retention "+testCase.name, func(testingInstance *testing.T) {
			fixture := newMediaOperationInternalFixture(testingInstance)
			record := fixture.record(MediaOperationStateSucceeded, MediaProviderExecutionSucceeded)
			terminalAt := fixture.now.Add(-2 * time.Hour)
			if updateError := fixture.database.Model(&mediaOperationRecord{}).Where("operation_id = ?", record.OperationID).Update("terminal_at", terminalAt).Error; updateError != nil {
				testingInstance.Fatal(updateError)
			}
			failNthMediaGORMOperation(testingInstance, fixture.database, testCase.operation, testCase.table, 1)
			fixture.service.expireTerminalData()
		})
	}
	for _, testCase := range []struct{ name, operation, table string }{
		{name: "count", operation: "query", table: "media_operation_usage_delivery_records"},
		{name: "event", operation: "create", table: "managed_usage_event_records"},
		{name: "delivery", operation: "create", table: "media_operation_usage_delivery_records"},
	} {
		testingInstance.Run("usage "+testCase.name, func(testingInstance *testing.T) {
			fixture := newMediaOperationInternalFixture(testingInstance)
			record := fixture.record(MediaOperationStateSucceeded, MediaProviderExecutionSucceeded)
			failNthMediaGORMOperation(testingInstance, fixture.database, testCase.operation, testCase.table, 1)
			if usageError := deliverMediaOperationUsage(fixture.database, record, fixture.now); usageError == nil {
				testingInstance.Fatal("expected usage failure")
			}
		})
	}
	fixture := newMediaOperationInternalFixture(testingInstance)
	record := fixture.record(MediaOperationStateSucceeded, MediaProviderExecutionSucceeded)
	if usageError := deliverMediaOperationUsage(fixture.database, record, fixture.now); usageError != nil {
		testingInstance.Fatal(usageError)
	}
	if usageError := deliverMediaOperationUsage(fixture.database, record, fixture.now); usageError != nil {
		testingInstance.Fatal(usageError)
	}
}

func TestMediaOperationCredentialReferenceRejectsInvalidConnectionVersion(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	if updateError := fixture.database.Model(&managedAccountConnectionRecord{}).Where("id = ?", "connection-internal").Update("version", 0).Error; updateError != nil {
		testingInstance.Fatal(updateError)
	}
	if _, referenceError := fixture.service.store.credentialReference(context.Background(), fixture.tenant.identifier.string(), providerID(ProviderNameXAI)); !errors.Is(referenceError, errMediaOperationStore) {
		testingInstance.Fatal(referenceError)
	}
}

func TestMediaOperationStoreConstructionFailures(testingInstance *testing.T) {
	store, storeError := newMediaOperationStore(&managedTenantStore{})
	if store != nil || storeError != nil {
		testingInstance.Fatalf("store=%v error=%v", store, storeError)
	}
	database, databaseError := gorm.Open(failingManagedAutoMigrateDialector{Dialector: sqlite.Open(filepath.Join(testingInstance.TempDir(), "migration.db"))}, &gorm.Config{})
	if databaseError != nil {
		testingInstance.Fatal(databaseError)
	}
	store, storeError = newMediaOperationStore(&managedTenantStore{database: &gormManagedTenantDatabase{database: database}})
	if store != nil || !errors.Is(storeError, errMediaOperationStore) {
		testingInstance.Fatalf("store=%v error=%v", store, storeError)
	}
}

func TestMediaOperationCreationRejectsInvalidAndUnavailableIntent(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	invalidPayloads := []mediaOperationCreatePayload{
		{},
		{Capability: llmproxycontract.MediaCapabilityVideoGenerate, Provider: ProviderNameXAI, Model: "grok-imagine-video-1.5", Input: json.RawMessage(`[]`), Controls: json.RawMessage(`{}`)},
	}
	for _, payload := range invalidPayloads {
		if _, _, createError := fixture.service.create(context.Background(), fixture.tenant, "invalid", payload); !errors.Is(createError, errMediaOperationInvalid) {
			testingInstance.Fatalf("payload=%+v error=%v", payload, createError)
		}
	}
	unavailable := fixture.payload()
	unavailable.Model = "missing"
	if _, _, createError := fixture.service.create(context.Background(), fixture.tenant, "missing-model", unavailable); !errors.Is(createError, errMediaOperationUnavailable) {
		testingInstance.Fatal(createError)
	}
	missingSettings := fixture.tenant
	missingSettings.providerSettings = map[providerID]managedProviderSettings{}
	if _, _, createError := fixture.service.create(context.Background(), missingSettings, "missing-settings", fixture.payload()); !errors.Is(createError, errMediaOperationUnavailable) {
		testingInstance.Fatal(createError)
	}
	fixture.service.adapters = map[string]MediaOperationAdapter{}
	if _, _, createError := fixture.service.create(context.Background(), fixture.tenant, "missing-adapter", fixture.payload()); !errors.Is(createError, errMediaOperationUnavailable) {
		testingInstance.Fatal(createError)
	}
	fixture.service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityVideoGenerate, ProviderNameXAI, "grok-imagine-video-1.5")] = fixture.adapter
	fixture.adapter.validateError = io.ErrUnexpectedEOF
	if _, _, createError := fixture.service.create(context.Background(), fixture.tenant, "invalid-adapter", fixture.payload()); !errors.Is(createError, errMediaOperationInvalid) {
		testingInstance.Fatal(createError)
	}
	fixture.adapter.validateError = nil
	fixture.adapter.validateResult.Input = json.RawMessage(`[]`)
	if _, _, createError := fixture.service.create(context.Background(), fixture.tenant, "invalid-normalized", fixture.payload()); !errors.Is(createError, errMediaOperationInvalid) {
		testingInstance.Fatal(createError)
	}
}

func TestMediaOperationCreationEnforcesCapacityAssetsAndCredentialVersion(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	fixture.service.globalCapacity = 0
	if _, _, createError := fixture.service.create(context.Background(), fixture.tenant, "capacity", fixture.payload()); !errors.Is(createError, errMediaOperationCapacity) {
		testingInstance.Fatal(createError)
	}
	fixture.service.globalCapacity = 2
	fixture.adapter.validateResult.InputAssetIDs = []string{"ast_00000000000000000000000000000000"}
	if _, _, createError := fixture.service.create(context.Background(), fixture.tenant, "missing-asset", fixture.payload()); !errors.Is(createError, errMediaOperationInvalid) {
		testingInstance.Fatal(createError)
	}
	fixture.adapter.validateResult.InputAssetIDs = nil
	if deleteError := fixture.database.Where("tenant_id = ? AND provider_id = ?", fixture.tenant.identifier.string(), ProviderNameXAI).Delete(&managedTenantConnectionRecord{}).Error; deleteError != nil {
		testingInstance.Fatal(deleteError)
	}
	if _, _, createError := fixture.service.create(context.Background(), fixture.tenant, "credential", fixture.payload()); !errors.Is(createError, errMediaOperationUnavailable) {
		testingInstance.Fatal(createError)
	}
}

func TestMediaOperationCancellationFinalizesQueuedAndRunningWork(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	queued := fixture.record(MediaOperationStateQueued, MediaProviderExecutionNotDispatched)
	if createError := fixture.database.Create(&mediaOperationAssetReferenceRecord{OperationID: queued.OperationID, Role: "input", Ordinal: 0, TenantID: queued.TenantID, AssetID: "ast_0123456789abcdef0123456789abcdef", MIMEType: "image/png", SizeBytes: 1, Active: true, CreatedAt: fixture.now}).Error; createError != nil {
		testingInstance.Fatal(createError)
	}
	cancelled, cancelError := fixture.service.cancel(context.Background(), fixture.tenant, queued.OperationID)
	if cancelError != nil || cancelled.State != MediaOperationStateCancelled {
		testingInstance.Fatalf("cancelled=%+v error=%v", cancelled, cancelError)
	}
	var activeCount int64
	if countError := fixture.database.Model(&mediaOperationAssetReferenceRecord{}).Where("operation_id = ? AND active = ?", queued.OperationID, true).Count(&activeCount).Error; countError != nil || activeCount != 0 {
		testingInstance.Fatalf("active=%d error=%v", activeCount, countError)
	}

	running := fixture.record(MediaOperationStateRunning, MediaProviderExecutionDispatched)
	if createError := fixture.database.Create(&mediaOperationClaimRecord{OperationID: running.OperationID, WorkerID: "worker", Generation: 1, ExpiresAt: fixture.now.Add(time.Minute)}).Error; createError != nil {
		testingInstance.Fatal(createError)
	}
	fixture.adapter.cancelResult.State = MediaCancellationConfirmed
	cancelled, cancelError = fixture.service.cancel(context.Background(), fixture.tenant, running.OperationID)
	if cancelError != nil || cancelled.State != MediaOperationStateCancelled || cancelled.CancellationState != MediaCancellationConfirmed {
		testingInstance.Fatalf("cancelled=%+v error=%v", cancelled, cancelError)
	}
	if _, cancelError := fixture.service.cancel(context.Background(), fixture.tenant, "invalid"); !errors.Is(cancelError, errMediaOperationNotFound) {
		testingInstance.Fatal(cancelError)
	}
	if _, cancelError := fixture.service.cancel(context.Background(), fixture.tenant, "mop_00000000000000000000000000000000"); !errors.Is(cancelError, errMediaOperationNotFound) {
		testingInstance.Fatal(cancelError)
	}
}

func TestMediaOperationClaimExecutionAndTerminalEdges(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	queued := fixture.record(MediaOperationStateQueued, MediaProviderExecutionNotDispatched)
	record, generation, recoverOperation, claimError := fixture.service.claim("worker", queued.OperationID)
	if claimError != nil || record.PublicState != MediaOperationStateRunning || generation != 1 || recoverOperation {
		testingInstance.Fatalf("record=%+v generation=%d recover=%t error=%v", record, generation, recoverOperation, claimError)
	}
	if _, _, _, claimError := fixture.service.claim("other", queued.OperationID); !errors.Is(claimError, gorm.ErrRecordNotFound) {
		testingInstance.Fatal(claimError)
	}
	if dispatchError := fixture.service.markDispatched(queued.OperationID, generation+1, queued.OperationID); !errors.Is(dispatchError, errMediaOperationStore) {
		testingInstance.Fatal(dispatchError)
	}
	if dispatchError := fixture.service.markDispatched(queued.OperationID, generation, queued.OperationID); dispatchError != nil {
		testingInstance.Fatal(dispatchError)
	}
	fixture.service.finish(queued.OperationID, generation, MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "native private error"})
	response, responseError := fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), queued.OperationID)
	if responseError != nil || response.State != MediaOperationStateFailed || response.Error == nil || response.Error.Code != "provider_error" {
		testingInstance.Fatalf("response=%+v error=%v", response, responseError)
	}

	uncertain := fixture.record(MediaOperationStateRunning, MediaProviderExecutionDispatched)
	if createError := fixture.database.Create(&mediaOperationClaimRecord{OperationID: uncertain.OperationID, WorkerID: "worker", Generation: 1, ExpiresAt: fixture.now.Add(time.Minute)}).Error; createError != nil {
		testingInstance.Fatal(createError)
	}
	fixture.service.finish(uncertain.OperationID, 1, MediaOperationExecutionResult{State: "future", ErrorCode: "native private error"})
	response, responseError = fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), uncertain.OperationID)
	if responseError != nil || response.State != MediaOperationStateUncertain || response.Error == nil || response.Error.Code != "provider_outcome_unknown" {
		testingInstance.Fatalf("response=%+v error=%v", response, responseError)
	}

	deadlineContext, cancelDeadline := context.WithCancel(context.Background())
	cancelDeadline()
	deadlineResult := fixture.service.runWithClaimRenewal(deadlineContext, "missing", 1, func() MediaOperationExecutionResult { select {} })
	if deadlineResult.ErrorCode != "operation_deadline_exceeded" {
		testingInstance.Fatalf("deadline result=%+v", deadlineResult)
	}
	claimContext, cancelClaim := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelClaim()
	claimResult := fixture.service.runWithClaimRenewal(claimContext, "missing", 1, func() MediaOperationExecutionResult { select {} })
	if claimResult.ErrorCode != "worker_claim_lost" {
		testingInstance.Fatalf("claim result=%+v", claimResult)
	}
}

func TestMediaOperationPublicResponseAndRetentionEdges(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	if _, responseError := fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), "invalid"); !errors.Is(responseError, errMediaOperationNotFound) {
		testingInstance.Fatal(responseError)
	}
	if _, responseError := fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), "mop_00000000000000000000000000000000"); !errors.Is(responseError, errMediaOperationNotFound) {
		testingInstance.Fatal(responseError)
	}
	tombstone := mediaOperationTombstoneRecord{TenantID: fixture.tenant.identifier.string(), IdempotencyKeyDigest: "key", IntentDigest: "intent", OperationID: "mop_11111111111111111111111111111111", TerminalState: MediaOperationStateSucceeded, CreatedAt: fixture.now}
	if createError := fixture.database.Create(&tombstone).Error; createError != nil {
		testingInstance.Fatal(createError)
	}
	if _, responseError := fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), tombstone.OperationID); responseError == nil {
		testingInstance.Fatal("expected expired response")
	} else {
		var expired mediaOperationExpiredError
		if !errors.As(responseError, &expired) {
			testingInstance.Fatal(responseError)
		}
	}

	terminal := fixture.record(MediaOperationStateSucceeded, MediaProviderExecutionSucceeded)
	terminalAt := fixture.now.Add(-2 * time.Hour)
	if updateError := fixture.database.Model(&mediaOperationRecord{}).Where("operation_id = ?", terminal.OperationID).Updates(map[string]any{"terminal_at": terminalAt, "updated_at": terminalAt}).Error; updateError != nil {
		testingInstance.Fatal(updateError)
	}
	fixture.service.expireTerminalData()
	if _, responseError := fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), terminal.OperationID); responseError == nil {
		testingInstance.Fatal("expected retained tombstone")
	}

	undelivered := fixture.record(MediaOperationStateFailed, MediaProviderExecutionFailed)
	terminalAt = fixture.now
	if updateError := fixture.database.Model(&mediaOperationRecord{}).Where("operation_id = ?", undelivered.OperationID).Update("terminal_at", terminalAt).Error; updateError != nil {
		testingInstance.Fatal(updateError)
	}
	fixture.service.deliverPendingUsage()
	fixture.service.deliverPendingUsage()
	var deliveryCount int64
	if countError := fixture.database.Model(&mediaOperationUsageDeliveryRecord{}).Where("operation_id = ?", undelivered.OperationID).Count(&deliveryCount).Error; countError != nil || deliveryCount != 1 {
		testingInstance.Fatalf("deliveries=%d error=%v", deliveryCount, countError)
	}
}

func TestMediaOperationHandlersReturnResourceErrors(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	for _, handler := range []gin.HandlerFunc{fixture.service.statusHandler(), fixture.service.cancellationHandler()} {
		response := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(response)
		ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		ginContext.Set(contextKeyTenant, fixture.tenant)
		ginContext.Params = gin.Params{{Key: "operation_id", Value: "invalid"}}
		handler(ginContext)
		if response.Code != http.StatusNotFound {
			testingInstance.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{} {}`))
	ginContext.Set(contextKeyTenant, fixture.tenant)
	ginContext.Request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "valid")
	fixture.service.createHandler()(ginContext)
	if response.Code != http.StatusBadRequest {
		testingInstance.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDictatorMediaCapabilityContract(testingInstance *testing.T) {
	expectedOperations := map[string]string{
		llmproxycontract.MediaCapabilityAudioTranscribe:     ModelOperationAudioTranscription,
		llmproxycontract.MediaCapabilityAudioDiarize:        ModelOperationAudioDiarization,
		llmproxycontract.MediaCapabilityAudioAlign:          ModelOperationAudioAlignment,
		llmproxycontract.MediaCapabilitySubtitlesCreate:     ModelOperationSubtitleCreation,
		llmproxycontract.MediaCapabilityAudioSpeechGenerate: ModelOperationSpeechGeneration,
		llmproxycontract.MediaCapabilityAudioVoiceExtract:   ModelOperationVoiceExtraction,
	}
	for capability, operation := range expectedOperations {
		if catalogOperationForMediaCapability(capability) != operation || mediaCapabilityForCatalogOperation(operation) != capability {
			testingInstance.Fatalf("capability=%s operation=%s", capability, operation)
		}
	}
	for _, mimeType := range []string{"application/json", "application/x-subrip", "audio/wav"} {
		if !supportedTenantAssetMIME(mimeType) {
			testingInstance.Fatalf("unsupported Dictator result MIME type=%s", mimeType)
		}
	}
}

func TestDictatorRegistrationQueueAndTerminalEdges(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	validValues := map[string]map[string]string{ProviderNameDictator: {"grpc_address": "dictator.internal:50051", "grpc_auth_token": "token", "grpc_tls": "true"}}
	configuration := Configuration{ProviderConnectionValues: validValues, dictatorProtocol: &controlledDictatorProtocol{}}
	fixture.service.adapters = nil
	fixture.service.voiceProviders = nil
	if registrationError := fixture.service.registerDictatorAdapter(configuration); registrationError != nil || len(fixture.service.adapters) != 6 || fixture.service.voiceProviders[ProviderNameDictator] == nil {
		testingInstance.Fatalf("registration error=%v adapters=%d voices=%v", registrationError, len(fixture.service.adapters), fixture.service.voiceProviders)
	}
	if registrationError := fixture.service.registerDictatorAdapter(Configuration{dictatorProtocol: &controlledDictatorProtocol{}}); !errors.Is(registrationError, errMediaOperationUnavailable) {
		testingInstance.Fatalf("invalid connection registration error=%v", registrationError)
	}
	invalidService := &mediaOperationService{store: fixture.service.store}
	if registrationError := invalidService.registerDictatorAdapter(configuration); registrationError == nil {
		testingInstance.Fatal("invalid adapter dependencies accepted")
	}

	managedTenants := &managedTenantStore{database: &gormManagedTenantDatabase{database: fixture.database}}
	serviceConfiguration := Configuration{
		ProviderConnectionValues: validValues, dictatorProtocol: &controlledDictatorProtocol{},
		MediaOperationCapacity: 1, TenantMediaOperationCapacity: 1,
		MediaOperationLifetimeSeconds: 60, MediaOperationClaimSeconds: 10, MediaOperationClaimRenewalSeconds: 1, AssetRetentionSeconds: 60,
	}
	service, serviceError := newMediaOperationService(serviceConfiguration, managedTenants, fixture.service.assets, fixture.service.providers)
	if serviceError != nil || service == nil {
		testingInstance.Fatalf("service=%v error=%v", service, serviceError)
	}
	serviceConfiguration.ProviderConnectionValues = nil
	if invalid, serviceError := newMediaOperationService(serviceConfiguration, managedTenants, fixture.service.assets, fixture.service.providers); invalid != nil || !errors.Is(serviceError, errMediaOperationUnavailable) {
		testingInstance.Fatalf("invalid service=%v error=%v", invalid, serviceError)
	}

	fixture.service.dictatorQueue = make(chan string, 1)
	fixture.service.enqueue("mop_00000000000000000000000000000000")
	if _, queued := fixture.service.queued.Load("mop_00000000000000000000000000000000"); queued {
		testingInstance.Fatal("missing operation retained in queue set")
	}
	if persistError := fixture.service.persistProviderHandle("missing", 1, " "); !errors.Is(persistError, errMediaOperationStore) {
		testingInstance.Fatalf("empty provider handle error=%v", persistError)
	}
	if persistError := fixture.service.persistProviderHandle("missing", 1, "native"); !errors.Is(persistError, errMediaOperationStore) {
		testingInstance.Fatalf("missing provider handle error=%v", persistError)
	}

	finish := func(record mediaOperationRecord, result MediaOperationExecutionResult) mediaOperationResponse {
		if claimError := fixture.database.Create(&mediaOperationClaimRecord{OperationID: record.OperationID, WorkerID: "worker", Generation: 1, ExpiresAt: fixture.now.Add(time.Minute)}).Error; claimError != nil {
			testingInstance.Fatal(claimError)
		}
		fixture.service.finish(record.OperationID, 1, result)
		response, responseError := fixture.service.store.publicResponse(context.Background(), fixture.tenant.identifier.string(), record.OperationID)
		if responseError != nil {
			testingInstance.Fatal(responseError)
		}
		return response
	}
	cancelled := finish(fixture.record(MediaOperationStateRunning, MediaProviderExecutionDispatched), MediaOperationExecutionResult{State: MediaOperationStateCancelled})
	if cancelled.State != MediaOperationStateCancelled || cancelled.CancellationState != MediaCancellationConfirmed {
		testingInstance.Fatalf("cancelled=%+v", cancelled)
	}
	validVoice := MediaVoiceProviderRecord{Provider: ProviderNameXAI, Model: "model", Mode: MediaVoiceModeExtracted, Language: "en", DisplayName: "voice", SampleRates: []int{24000}, DefaultSampleRate: 24000, ProviderVoiceReference: "native"}
	wrongCapability := finish(fixture.record(MediaOperationStateRunning, MediaProviderExecutionDispatched), MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Voice: &validVoice, Outputs: []MediaOperationOutput{{MIMEType: "application/json", Data: []byte(`{}`)}}})
	if wrongCapability.State != MediaOperationStateFailed || wrongCapability.Error == nil || wrongCapability.Error.Code != "provider_result_invalid" {
		testingInstance.Fatalf("wrong capability=%+v", wrongCapability)
	}
	extractionRecord := fixture.record(MediaOperationStateRunning, MediaProviderExecutionDispatched)
	extractionRecord.Capability = llmproxycontract.MediaCapabilityAudioVoiceExtract
	extractionRecord.CatalogOperation = ModelOperationVoiceExtraction
	if saveError := fixture.database.Save(&extractionRecord).Error; saveError != nil {
		testingInstance.Fatal(saveError)
	}
	invalidVoice := validVoice
	invalidVoice.Provider = ProviderNameDictator
	invalidExtraction := finish(extractionRecord, MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Voice: &invalidVoice, Outputs: []MediaOperationOutput{{MIMEType: "application/json", Data: []byte(`{}`)}}})
	if invalidExtraction.State != MediaOperationStateFailed || invalidExtraction.Error == nil || invalidExtraction.Error.Code != "provider_result_invalid" {
		testingInstance.Fatalf("invalid extraction=%+v", invalidExtraction)
	}
}

func TestMediaOperationDeploymentCredentialDoesNotRequireTenantConnection(testingInstance *testing.T) {
	fixture := newMediaOperationInternalFixture(testingInstance)
	fixture.tenant.providerSettings = map[providerID]managedProviderSettings{}
	adapter := &deploymentMediaOperationAdapter{mediaOperationInternalAdapter: fixture.adapter, credentialReference: "deployment:dictator:sha256-reference"}
	fixture.service.adapters = map[string]MediaOperationAdapter{
		mediaOperationAdapterKey(llmproxycontract.MediaCapabilityVideoGenerate, ProviderNameXAI, "grok-imagine-video-1.5"): adapter,
	}

	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ginContext.Set(contextKeyTenant, fixture.tenant)
	fixture.service.capabilitiesHandler()(ginContext)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"provider":"xai"`) {
		testingInstance.Fatalf("capabilities=%s", response.Body.String())
	}

	record, created, createError := fixture.service.create(context.Background(), fixture.tenant, "deployment-credential", mediaOperationCreatePayload{
		Capability: llmproxycontract.MediaCapabilityVideoGenerate,
		Provider:   ProviderNameXAI,
		Model:      "grok-imagine-video-1.5",
		Input:      json.RawMessage(`{"prompt":"generate"}`),
		Controls:   json.RawMessage(`{}`),
	})
	if createError != nil || !created || record.CredentialReference != adapter.credentialReference {
		testingInstance.Fatalf("created=%t credential=%q error=%v", created, record.CredentialReference, createError)
	}
	emptyCredential := &deploymentMediaOperationAdapter{mediaOperationInternalAdapter: fixture.adapter}
	provider := fixture.service.providers.definitions[providerID(ProviderNameXAI)]
	if fixture.service.mediaOperationCredentialAvailable(fixture.tenant, provider, emptyCredential) {
		testingInstance.Fatal("empty deployment credential accepted")
	}
	if _, credentialError := fixture.service.mediaOperationCredentialReference(context.Background(), fixture.tenant, provider, emptyCredential); !errors.Is(credentialError, errMediaOperationUnavailable) {
		testingInstance.Fatalf("credential error=%v", credentialError)
	}
}

func TestMediaVoiceBoundaryFailuresAndPrivateStore(testingInstance *testing.T) {
	validVoice := MediaVoiceProviderRecord{
		Provider: ProviderNameXAI, Model: "private-model", Mode: MediaVoiceModePreset,
		Language: "en-US", DisplayName: "Narrator", Default: true,
		SampleRates: []int{48000, 24000}, DefaultSampleRate: 24000, ProviderVoiceReference: "native-voice",
	}
	fixture := newMediaOperationInternalFixture(testingInstance)
	fixture.service.voiceProviders = map[string]MediaVoiceProvider{ProviderNameXAI: internalMediaVoiceProvider{voices: []MediaVoiceProviderRecord{validVoice}}}

	request := func(path string) (*httptest.ResponseRecorder, *gin.Context) {
		response := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(response)
		ginContext.Request = httptest.NewRequest(http.MethodGet, path, nil)
		ginContext.Set(contextKeyTenant, fixture.tenant)
		return response, ginContext
	}

	response, ginContext := request("/?provider=XAI")
	fixture.service.mediaVoiceCollectionHandler()(ginContext)
	if response.Code != http.StatusBadRequest {
		testingInstance.Fatalf("invalid provider status=%d", response.Code)
	}
	fixture.service.voiceProviders[ProviderNameXAI] = internalMediaVoiceProvider{err: io.ErrUnexpectedEOF}
	response, ginContext = request("/?provider=xai")
	fixture.service.mediaVoiceCollectionHandler()(ginContext)
	if response.Code != http.StatusBadGateway {
		testingInstance.Fatalf("provider failure status=%d", response.Code)
	}
	fixture.service.voiceProviders[ProviderNameXAI] = internalMediaVoiceProvider{voices: []MediaVoiceProviderRecord{{Provider: ProviderNameXAI}}}
	response, ginContext = request("/?provider=xai")
	fixture.service.mediaVoiceCollectionHandler()(ginContext)
	if response.Code != http.StatusBadGateway {
		testingInstance.Fatalf("invalid discovery status=%d", response.Code)
	}

	if persistError := fixture.service.store.persistMediaVoices(context.Background(), fixture.tenant.identifier.string(), ProviderNameXAI, []MediaVoiceProviderRecord{validVoice}); persistError != nil {
		testingInstance.Fatal(persistError)
	}
	voices, listError := fixture.service.store.listMediaVoices(context.Background(), fixture.tenant.identifier.string(), ProviderNameXAI)
	if listError != nil || len(voices) != 1 || voices[0].SampleRates[0] != 24000 {
		testingInstance.Fatalf("voices=%+v error=%v", voices, listError)
	}
	voice, voiceError := fixture.service.store.mediaVoice(context.Background(), fixture.tenant.identifier.string(), voices[0].VoiceID)
	if voiceError != nil || voice.VoiceID != voices[0].VoiceID {
		testingInstance.Fatalf("voice=%+v error=%v", voice, voiceError)
	}
	validVoice.DisplayName = "Updated narrator"
	if persistError := fixture.service.store.persistMediaVoices(context.Background(), fixture.tenant.identifier.string(), ProviderNameXAI, []MediaVoiceProviderRecord{validVoice}); persistError != nil {
		testingInstance.Fatal(persistError)
	}
	updated, updateError := fixture.service.store.mediaVoice(context.Background(), fixture.tenant.identifier.string(), voice.VoiceID)
	if updateError != nil || updated.DisplayName != "Updated narrator" {
		testingInstance.Fatalf("updated=%+v error=%v", updated, updateError)
	}
	if _, invalidIDError := fixture.service.store.mediaVoice(context.Background(), fixture.tenant.identifier.string(), "invalid"); invalidIDError == nil {
		testingInstance.Fatal("invalid voice identifier accepted")
	}
	if _, missingError := fixture.service.store.mediaVoice(context.Background(), fixture.tenant.identifier.string(), "voi_00000000000000000000000000000000"); missingError == nil || missingError.Error() != llmproxycontract.ErrorCodeMediaVoiceNotFound {
		testingInstance.Fatalf("missing voice error=%v", missingError)
	}

	invalidRecords := []MediaVoiceProviderRecord{
		{Provider: ProviderNameXAI},
		{Provider: ProviderNameXAI, Model: "model", Mode: MediaVoiceModePreset, Language: "en", DisplayName: "voice", SampleRates: []int{24000, 24000}, DefaultSampleRate: 24000, ProviderVoiceReference: "native"},
		{Provider: ProviderNameXAI, Model: "model", Mode: MediaVoiceModePreset, Language: "en", DisplayName: "voice", SampleRates: []int{24000}, DefaultSampleRate: 48000, ProviderVoiceReference: "native"},
	}
	for _, invalid := range invalidRecords {
		if _, recordError := newMediaVoiceRecord(fixture.tenant.identifier.string(), ProviderNameXAI, invalid, fixture.now); recordError == nil {
			testingInstance.Fatalf("invalid voice accepted=%+v", invalid)
		}
	}

	corrupt := mediaVoiceRecord{VoiceID: "voi_11111111111111111111111111111111", TenantID: fixture.tenant.identifier.string(), Provider: ProviderNameXAI, ProviderVoiceReference: "corrupt", Model: "private", Mode: MediaVoiceModePreset, Language: "en", DisplayName: "corrupt", SampleRates: []byte(`null`), DefaultSampleRate: 24000}
	if createError := fixture.database.Create(&corrupt).Error; createError != nil {
		testingInstance.Fatal(createError)
	}
	if _, corruptError := fixture.service.store.mediaVoice(context.Background(), fixture.tenant.identifier.string(), corrupt.VoiceID); corruptError == nil {
		testingInstance.Fatal("corrupt voice accepted")
	}
	if _, corruptListError := fixture.service.store.listMediaVoices(context.Background(), fixture.tenant.identifier.string(), ProviderNameXAI); corruptListError == nil {
		testingInstance.Fatal("corrupt voice list accepted")
	}

	response, ginContext = request("/")
	ginContext.Params = gin.Params{{Key: "voice_id", Value: "invalid"}}
	fixture.service.mediaVoiceHandler()(ginContext)
	if response.Code != http.StatusNotFound {
		testingInstance.Fatalf("missing voice handler status=%d", response.Code)
	}

	for voiceError, expectedStatus := range map[error]int{
		errors.New(llmproxycontract.ErrorCodeMediaVoiceInvalid):  http.StatusBadRequest,
		errors.New(llmproxycontract.ErrorCodeMediaVoiceNotFound): http.StatusNotFound,
		errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider): http.StatusBadGateway,
		errors.New(llmproxycontract.ErrorCodeMediaVoiceStore):    http.StatusInternalServerError,
		errors.New("private provider detail"):                    http.StatusInternalServerError,
	} {
		response := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(response)
		writeMediaVoiceError(ginContext, voiceError)
		if response.Code != expectedStatus || strings.Contains(response.Body.String(), "private provider detail") {
			testingInstance.Fatalf("error=%v status=%d body=%s", voiceError, response.Code, response.Body.String())
		}
	}

	brokenFixture := newMediaOperationInternalFixture(testingInstance)
	if dropError := brokenFixture.database.Migrator().DropTable(&mediaVoiceRecord{}); dropError != nil {
		testingInstance.Fatal(dropError)
	}
	brokenFixture.service.voiceProviders = map[string]MediaVoiceProvider{ProviderNameXAI: internalMediaVoiceProvider{voices: []MediaVoiceProviderRecord{validVoice}}}
	response = httptest.NewRecorder()
	ginContext, _ = gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/?provider=xai", nil)
	ginContext.Set(contextKeyTenant, brokenFixture.tenant)
	brokenFixture.service.mediaVoiceCollectionHandler()(ginContext)
	if response.Code != http.StatusInternalServerError {
		testingInstance.Fatalf("persist failure status=%d", response.Code)
	}
	if _, listError := brokenFixture.service.store.listMediaVoices(context.Background(), brokenFixture.tenant.identifier.string(), ProviderNameXAI); listError == nil {
		testingInstance.Fatal("broken voice list succeeded")
	}
	if _, readError := brokenFixture.service.store.mediaVoice(context.Background(), brokenFixture.tenant.identifier.string(), "voi_22222222222222222222222222222222"); readError == nil || readError.Error() != llmproxycontract.ErrorCodeMediaVoiceStore {
		testingInstance.Fatalf("broken voice read error=%v", readError)
	}
	response = httptest.NewRecorder()
	ginContext, _ = gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ginContext.Params = gin.Params{{Key: "voice_id", Value: "voi_22222222222222222222222222222222"}}
	ginContext.Set(contextKeyTenant, brokenFixture.tenant)
	brokenFixture.service.mediaVoiceHandler()(ginContext)
	if response.Code != http.StatusInternalServerError {
		testingInstance.Fatalf("broken voice handler status=%d", response.Code)
	}

	listFailureFixture := newMediaOperationInternalFixture(testingInstance)
	listFailureFixture.service.voiceProviders = map[string]MediaVoiceProvider{ProviderNameXAI: internalMediaVoiceProvider{}}
	registerManagedGORMError(testingInstance, listFailureFixture.database, "media_voice_list_failure", "query", "media_voice_records", errInternalTestDatabase)
	response = httptest.NewRecorder()
	ginContext, _ = gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/?provider=xai", nil)
	ginContext.Set(contextKeyTenant, listFailureFixture.tenant)
	listFailureFixture.service.mediaVoiceCollectionHandler()(ginContext)
	if response.Code != http.StatusInternalServerError {
		testingInstance.Fatalf("list failure status=%d", response.Code)
	}
}

func TestMediaVoicePrivateStoreFailures(testingInstance *testing.T) {
	validVoice := MediaVoiceProviderRecord{Provider: ProviderNameDictator, Model: "model", Mode: MediaVoiceModePreset, Language: "en", DisplayName: "voice", SampleRates: []int{24000}, DefaultSampleRate: 24000, ProviderVoiceReference: "native"}
	for _, testCase := range []struct {
		name      string
		operation string
	}{
		{name: "create", operation: "create"},
		{name: "read after upsert", operation: "query"},
	} {
		testingInstance.Run(testCase.name, func(testingInstance *testing.T) {
			fixture := newMediaOperationInternalFixture(testingInstance)
			failNthMediaGORMOperation(testingInstance, fixture.database, testCase.operation, "media_voice_records", 1)
			if persistError := fixture.service.store.persistMediaVoices(context.Background(), fixture.tenant.identifier.string(), ProviderNameDictator, []MediaVoiceProviderRecord{validVoice}); persistError == nil || persistError.Error() != llmproxycontract.ErrorCodeMediaVoiceStore {
				testingInstance.Fatalf("persist error=%v", persistError)
			}
		})
	}
	fixture := newMediaOperationInternalFixture(testingInstance)
	if _, voiceError := fixture.service.store.providerMediaVoice(context.Background(), fixture.tenant.identifier.string(), "invalid"); voiceError == nil || voiceError.Error() != llmproxycontract.ErrorCodeMediaVoiceNotFound {
		testingInstance.Fatalf("invalid identifier error=%v", voiceError)
	}
	if _, voiceError := fixture.service.store.providerMediaVoice(context.Background(), fixture.tenant.identifier.string(), "voi_00000000000000000000000000000000"); voiceError == nil || voiceError.Error() != llmproxycontract.ErrorCodeMediaVoiceNotFound {
		testingInstance.Fatalf("missing voice error=%v", voiceError)
	}
	fixture = newMediaOperationInternalFixture(testingInstance)
	failNthMediaGORMOperation(testingInstance, fixture.database, "query", "media_voice_records", 1)
	if _, voiceError := fixture.service.store.providerMediaVoice(context.Background(), fixture.tenant.identifier.string(), "voi_00000000000000000000000000000000"); voiceError == nil || voiceError.Error() != llmproxycontract.ErrorCodeMediaVoiceStore {
		testingInstance.Fatalf("store error=%v", voiceError)
	}
}
