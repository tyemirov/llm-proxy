package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type blockingManagedUsageDatabase struct {
	managedTenantDatabase
	firstWriteStarted chan struct{}
	releaseFirstWrite chan struct{}
	releaseOnce       sync.Once
	recordsMutex      sync.Mutex
	records           []managedUsageEventRecord
}

type delayedFlushResponseRecorder struct {
	*httptest.ResponseRecorder
	delay time.Duration
}

func (recorder *delayedFlushResponseRecorder) Flush() {
	time.Sleep(recorder.delay)
	recorder.ResponseRecorder.Flush()
}

func newBlockingManagedUsageDatabase(database managedTenantDatabase) *blockingManagedUsageDatabase {
	return &blockingManagedUsageDatabase{
		managedTenantDatabase: database,
		firstWriteStarted:     make(chan struct{}),
		releaseFirstWrite:     make(chan struct{}),
	}
}

func (database *blockingManagedUsageDatabase) createUsageEvent(requestContext context.Context, record managedUsageEventRecord) error {
	database.recordsMutex.Lock()
	writeIndex := len(database.records)
	database.recordsMutex.Unlock()
	if writeIndex == 0 {
		close(database.firstWriteStarted)
		select {
		case <-database.releaseFirstWrite:
		case <-requestContext.Done():
			return requestContext.Err()
		}
	}
	database.recordsMutex.Lock()
	database.records = append(database.records, record)
	database.recordsMutex.Unlock()
	return nil
}

func (database *blockingManagedUsageDatabase) release() {
	database.releaseOnce.Do(func() {
		close(database.releaseFirstWrite)
	})
}

func (database *blockingManagedUsageDatabase) persistedRecords() []managedUsageEventRecord {
	database.recordsMutex.Lock()
	defer database.recordsMutex.Unlock()
	return append([]managedUsageEventRecord(nil), database.records...)
}

func TestManagedUsageFailureDomainRejectsMalformedValues(t *testing.T) {
	if _, outcomeError := newManagedUsageOutcomeCode("provider_error"); outcomeError == nil {
		t.Fatal("unknown outcome code accepted")
	}
	if _, dispositionError := newManagedUsageDisposition("unknown"); dispositionError == nil {
		t.Fatal("unknown disposition accepted")
	}
	if _, queryError := newManagedUsageDetailQuery(url.Values{
		usageDetailIntervalQuery: {string(usageIntervalThirtyDay)},
	}, " ", managedUsageDispositionFailed); queryError == nil {
		t.Fatal("blank failure scope accepted")
	}
	for _, disposition := range []managedUsageDisposition{managedUsageDispositionSucceeded, "unknown"} {
		if _, queryError := newManagedUsageDetailQuery(url.Values{
			usageDetailIntervalQuery: {string(usageIntervalThirtyDay)},
		}, managedUsageAllTenantsScope, disposition); queryError == nil {
			t.Fatalf("detail disposition %q accepted", disposition)
		}
	}
	for _, limit := range []string{"", "1x"} {
		if _, queryError := newManagedUsageDetailQuery(url.Values{
			usageDetailIntervalQuery: {string(usageIntervalThirtyDay)},
			usageDetailLimitQuery:    {limit},
		}, managedUsageAllTenantsScope, managedUsageDispositionFailed); queryError == nil {
			t.Fatalf("limit %q accepted", limit)
		}
	}

	encodeCursorPayload := func(payload string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(payload))
	}
	if _, cursorError := newManagedUsageDetailCursor("%", usageIntervalThirtyDay, managedUsageAllTenantsScope, managedUsageDispositionFailed); cursorError == nil {
		t.Fatal("invalid base64 cursor accepted")
	}
	invalidSnapshot := encodeCursorPayload(`{"v":3,"i":"30d","o":"all-tenants","d":"failed","s":"invalid","x":2,"p":"2026-07-25T12:00:00Z","n":1}`)
	if _, cursorError := newManagedUsageDetailCursor(invalidSnapshot, usageIntervalThirtyDay, managedUsageAllTenantsScope, managedUsageDispositionFailed); cursorError == nil {
		t.Fatal("invalid snapshot timestamp accepted")
	}
	positionAfterSnapshot := encodeCursorPayload(`{"v":3,"i":"30d","o":"all-tenants","d":"failed","s":"2026-07-25T12:00:00Z","x":2,"p":"2026-07-25T12:00:01Z","n":1}`)
	if _, cursorError := newManagedUsageDetailCursor(positionAfterSnapshot, usageIntervalThirtyDay, managedUsageAllTenantsScope, managedUsageDispositionFailed); cursorError == nil {
		t.Fatal("position after snapshot accepted")
	}
	noncanonical := encodeCursorPayload(" " + `{"v":3,"i":"30d","o":"all-tenants","d":"failed","s":"2026-07-25T12:00:00Z","x":2,"p":"2026-07-25T11:59:59Z","n":1}`)
	if _, cursorError := newManagedUsageDetailCursor(noncanonical, usageIntervalThirtyDay, managedUsageAllTenantsScope, managedUsageDispositionFailed); cursorError == nil {
		t.Fatal("noncanonical cursor accepted")
	}
	trailing := encodeCursorPayload(`{"v":3,"i":"30d","o":"all-tenants","d":"failed","s":"2026-07-25T12:00:00Z","x":2,"p":"2026-07-25T11:59:59Z","n":1} true`)
	if _, cursorError := newManagedUsageDetailCursor(trailing, usageIntervalThirtyDay, managedUsageAllTenantsScope, managedUsageDispositionFailed); cursorError == nil {
		t.Fatal("cursor with trailing value accepted")
	}

	database := newFakeManagedTenantDatabase()
	store := newManagedTenantStoreWithDatabase(database)
	managedTenant := tenant{identifier: tenantID("managed-default"), userID: "owner"}
	observedCore, observedLogs := observer.New(zapcore.WarnLevel)
	store.usageWriter.submit(managedTenant, managedUsageEvent{
		outcomeCode: managedUsageOutcomeCode("provider_error"),
	}, zap.New(observedCore).Sugar())
	if observedLogs.Len() != 1 || observedLogs.All()[0].Message != logEventUsageRecordFailed {
		t.Fatalf("invalid queued outcome logs=%+v", observedLogs.All())
	}
}

func TestRecordManagedUsageReportsPersistenceFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database := newFakeManagedTenantDatabase()
	database.createUsageEventError = errInternalTestDatabase
	store := newManagedTenantStoreWithDatabase(database)
	requestTenant := tenant{
		identifier: tenantID("managed-default"),
		userID:     "tauth-owner",
	}
	observedCore, observedLogs := observer.New(zapcore.WarnLevel)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ginContext.Set(contextKeyRequestTimeoutState, &requestTimeoutState{managedUsageOutcome: managedUsageOutcomeSuccess})
	ginContext.Status(http.StatusOK)
	recordManagedUsage(
		store,
		zap.New(observedCore).Sugar(),
		ginContext,
		requestTenant,
		usageEndpointText,
		http.StatusOK,
		nil,
		time.Now().Add(-time.Second),
	)
	waitForObservedLogCount(t, observedLogs, 1)
	if observedLogs.Len() != 1 || observedLogs.All()[0].Message != logEventUsageRecordFailed || !recorder.Flushed {
		t.Fatalf("logs=%+v flushed=%t", observedLogs.All(), recorder.Flushed)
	}

	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()
	cancelledRecorder := httptest.NewRecorder()
	cancelledGinContext, _ := gin.CreateTestContext(cancelledRecorder)
	cancelledGinContext.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(cancelledContext)
	cancelledGinContext.Set(contextKeyRequestTimeoutState, &requestTimeoutState{managedUsageOutcome: managedUsageOutcomeSuccess})
	cancelledGinContext.Status(http.StatusOK)
	cancelledCore, cancelledLogs := observer.New(zapcore.WarnLevel)
	recordManagedUsage(
		store,
		zap.New(cancelledCore).Sugar(),
		cancelledGinContext,
		requestTenant,
		usageEndpointText,
		http.StatusOK,
		nil,
		time.Now().Add(-time.Second),
	)
	waitForObservedLogCount(t, cancelledLogs, 1)
	if cancelledLogs.Len() != 1 || cancelledLogs.All()[0].Message != logEventUsageRecordFailed || !cancelledRecorder.Flushed {
		t.Fatalf("cancelled logs=%+v flushed=%t", cancelledLogs.All(), cancelledRecorder.Flushed)
	}
}

func TestManagedUsageWriterKeepsPublicResponsesIndependentFromPersistence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"id":"response-id","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"text","text":"queue ok"}]}]}`))
	}))
	defer upstreamServer.Close()

	const (
		rawSecret      = "managed-usage-queue-secret"
		tenantIDValue  = "managed-usage-queue"
		ownerUserID    = "managed-usage-owner"
		providerAPIKey = "sk-managed-usage-provider"
	)
	providerKeyCipher := internalManagedProviderKeyCipher()
	encryptedProviderAPIKey, encryptionError := providerKeyCipher.encryptConnection(
		bytes.NewReader(bytes.Repeat([]byte{7}, providerKeyCipher.aeadCipher.NonceSize())),
		tenantIDValue,
		ProviderNameOpenAI,
		CatalogCredentialAPIKey,
		providerAPIKey,
	)
	if encryptionError != nil {
		t.Fatalf("encrypt provider API key: %v", encryptionError)
	}
	secretDigest := sha256.Sum256([]byte(rawSecret))
	secretDigestText := hex.EncodeToString(secretDigest[:])
	timestamp := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	tenantRecord := fakeTenantRecord(ownerUserID, tenantIDValue, "Queue", timestamp)
	tenantRecord.SecretDigest = &secretDigestText
	tenantRecord.DefaultProvider = ProviderNameOpenAI
	tenantRecord.DefaultModel = ModelNameGPT41
	tenantRecord.DefaultDictationProvider = ProviderNameOpenAI
	tenantRecord.DefaultDictationModel = DefaultDictationModel
	tenantRecord.ProviderConnections = []managedProviderConnectionRecord{{
		TenantID: tenantIDValue, ProviderID: ProviderNameOpenAI, FieldID: CatalogCredentialAPIKey,
		Value: encryptedProviderAPIKey, CreatedAt: timestamp, UpdatedAt: timestamp,
	}}
	tenantRecord.ProviderProfiles = []managedProviderProfileRecord{{
		TenantID: tenantIDValue, ProviderID: ProviderNameOpenAI, TextModel: ModelNameGPT41,
		CreatedAt: timestamp, UpdatedAt: timestamp,
	}}
	baseDatabase := newFakeManagedTenantDatabase()
	baseDatabase.tenantsByID[tenantIDValue] = tenantRecord
	database := newBlockingManagedUsageDatabase(baseDatabase)
	t.Cleanup(database.release)
	store := newManagedTenantStoreWithDatabaseAndCipherAndUsageQueue(database, providerKeyCipher, 1)

	managementConfiguration := ManagementConfiguration{
		PublicOrigin:      "http://localhost:8080",
		UIOrigins:         []string{"http://localhost:8080"},
		TAuthTenantID:     "llm-proxy-test",
		JWTSigningKey:     "managed-usage-signing-key",
		JWTIssuer:         DefaultManagementJWTIssuer,
		SessionCookieName: "managed_usage_session",
		UsageQueueSize:    1,
	}
	sessionValidator, sessionValidatorError := newManagementSessionValidator(managementConfiguration)
	if sessionValidatorError != nil {
		t.Fatalf("session validator: %v", sessionValidatorError)
	}
	timeoutPolicy, timeoutPolicyError := newRequestTimeoutPolicy(5, 5)
	if timeoutPolicyError != nil {
		t.Fatalf("request timeout policy: %v", timeoutPolicyError)
	}
	endpoints := NewEndpoints()
	endpoints.SetResponsesURL(upstreamServer.URL)
	configuration := Configuration{
		Management:                 managementConfiguration,
		WorkerCount:                1,
		QueueSize:                  1,
		MaxPromptBytes:             1024,
		Endpoints:                  endpoints,
		ProviderCatalog:            internalTestProviderCatalog(internalManagedUsageWriterProviderModels()),
		ModelCatalog:               internalManagedUsageWriterProviderModels(),
		upstreamRateLimits:         upstreamRateLimits{rules: map[string]upstreamRateLimitRule{}},
		managementSessionValidator: sessionValidator,
		requestTimeoutPolicy:       timeoutPolicy,
		validated:                  true,
	}
	observedCore, observedLogs := observer.New(zapcore.InfoLevel)
	router, buildError := buildRouter(configuration, zap.New(observedCore).Sugar(), func(_ ManagementConfiguration, providers *providerRegistry) (*managedTenantStore, error) {
		store.routingDefaults = providers
		return store, nil
	})
	if buildError != nil {
		t.Fatalf("build router: %v", buildError)
	}

	const flushDelay = 100 * time.Millisecond
	successResponse := &delayedFlushResponseRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		delay:            flushDelay,
	}
	router.ServeHTTP(successResponse, httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(rawSecret)+"&prompt=hello", nil))
	if successResponse.Code != http.StatusOK || strings.TrimSpace(successResponse.Body.String()) != "queue ok" {
		t.Fatalf("success status=%d body=%q", successResponse.Code, successResponse.Body.String())
	}
	requestID := successResponse.Header().Get(llmproxycontract.HeaderRequestID)
	phaseSummaries := observedLogs.FilterMessage(logEventRequestPhaseSummary).All()
	if len(phaseSummaries) != 1 || phaseSummaries[0].ContextMap()[logFieldRequestID] != requestID {
		t.Fatalf("managed request phase summaries=%+v request_id=%q", phaseSummaries, requestID)
	}
	phaseFields := phaseSummaries[0].ContextMap()
	formattingMilliseconds := phaseFields[logFieldResponseFormattingMilliseconds].(int64)
	enqueueMilliseconds := phaseFields[logFieldManagedUsageEnqueueMilliseconds].(int64)
	if formattingMilliseconds < (flushDelay-20*time.Millisecond).Milliseconds() || enqueueMilliseconds >= (flushDelay/2).Milliseconds() {
		t.Fatalf("managed request phase summary=%v", phaseFields)
	}
	select {
	case <-database.firstWriteStarted:
	case <-time.After(time.Second):
		t.Fatal("first usage insert did not start")
	}

	queuedRequest := httptest.NewRequest(http.MethodPost, "/?key="+url.QueryEscape(rawSecret), strings.NewReader("{"))
	queuedRequest.Header.Set("Content-Type", "application/json")
	queuedResponse := httptest.NewRecorder()
	router.ServeHTTP(queuedResponse, queuedRequest)
	if queuedResponse.Code != http.StatusBadRequest {
		t.Fatalf("queued status=%d body=%q", queuedResponse.Code, queuedResponse.Body.String())
	}

	droppedRequest := httptest.NewRequest(http.MethodPost, "/v2?key="+url.QueryEscape(rawSecret), strings.NewReader("{"))
	droppedRequest.Header.Set("Content-Type", "application/json")
	droppedResponse := httptest.NewRecorder()
	router.ServeHTTP(droppedResponse, droppedRequest)
	if droppedResponse.Code != http.StatusBadRequest {
		t.Fatalf("dropped status=%d body=%q", droppedResponse.Code, droppedResponse.Body.String())
	}

	database.release()
	deadline := time.Now().Add(time.Second)
	persistedRecords := database.persistedRecords()
	for len(persistedRecords) != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		persistedRecords = database.persistedRecords()
	}
	if len(persistedRecords) != 2 ||
		persistedRecords[0].Endpoint != usageEndpointText ||
		persistedRecords[0].StatusCode != http.StatusOK ||
		persistedRecords[1].Endpoint != usageEndpointText ||
		persistedRecords[1].StatusCode != http.StatusBadRequest {
		t.Fatalf("persisted records=%+v", persistedRecords)
	}
	droppedLogs := observedLogs.FilterMessage(logEventUsageRecordDropped).All()
	if len(droppedLogs) != 1 {
		t.Fatalf("dropped logs=%+v", droppedLogs)
	}
	logFields := droppedLogs[0].ContextMap()
	if logFields[constants.LogFieldError] != errManagedUsageQueueFull.Error() {
		t.Fatalf("drop error=%v", logFields[constants.LogFieldError])
	}
	serializedLogs := droppedLogs[0].Message
	for fieldName, fieldValue := range logFields {
		serializedLogs += fmt.Sprintf("%s=%v", fieldName, fieldValue)
	}
	for _, forbiddenValue := range []string{rawSecret, providerAPIKey, "hello", "queue ok"} {
		if strings.Contains(serializedLogs, forbiddenValue) {
			t.Fatalf("drop log leaked %q: %s", forbiddenValue, serializedLogs)
		}
	}
}

func waitForObservedLogCount(t *testing.T, observedLogs *observer.ObservedLogs, expectedCount int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for observedLogs.Len() != expectedCount && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if observedLogs.Len() != expectedCount {
		t.Fatalf("observed log count=%d want=%d", observedLogs.Len(), expectedCount)
	}
}

func internalManagedUsageWriterProviderModels() ModelCatalog {
	return internalTestModelCatalog(
		internalTestOffering(ProviderNameOpenAI, ModelNameGPT41, []string{ModelOperationText}, []string{ModelOperationText}),
		internalTestOffering(ProviderNameOpenAI, DefaultDictationModel, []string{ModelOperationDictation}, []string{ModelOperationDictation}),
	)
}
