package proxy

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestManagedUsageFailureDomainRejectsMalformedValues(t *testing.T) {
	if _, outcomeError := newManagedUsageOutcomeCode("provider_error"); outcomeError == nil {
		t.Fatal("unknown outcome code accepted")
	}
	for _, limit := range []string{"", "1x"} {
		if _, queryError := newManagedUsageFailureQuery(url.Values{
			usageFailureIntervalQuery: {string(usageIntervalThirtyDay)},
			usageFailureLimitQuery:    {limit},
		}); queryError == nil {
			t.Fatalf("limit %q accepted", limit)
		}
	}

	encodeCursorPayload := func(payload string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(payload))
	}
	if _, cursorError := newManagedUsageFailureCursor("%", usageIntervalThirtyDay); cursorError == nil {
		t.Fatal("invalid base64 cursor accepted")
	}
	invalidSnapshot := encodeCursorPayload(`{"v":1,"i":"30d","s":"invalid","x":2,"p":"2026-07-25T12:00:00Z","n":1}`)
	if _, cursorError := newManagedUsageFailureCursor(invalidSnapshot, usageIntervalThirtyDay); cursorError == nil {
		t.Fatal("invalid snapshot timestamp accepted")
	}
	positionAfterSnapshot := encodeCursorPayload(`{"v":1,"i":"30d","s":"2026-07-25T12:00:00Z","x":2,"p":"2026-07-25T12:00:01Z","n":1}`)
	if _, cursorError := newManagedUsageFailureCursor(positionAfterSnapshot, usageIntervalThirtyDay); cursorError == nil {
		t.Fatal("position after snapshot accepted")
	}
	noncanonical := encodeCursorPayload(" " + `{"v":1,"i":"30d","s":"2026-07-25T12:00:00Z","x":2,"p":"2026-07-25T11:59:59Z","n":1}`)
	if _, cursorError := newManagedUsageFailureCursor(noncanonical, usageIntervalThirtyDay); cursorError == nil {
		t.Fatal("noncanonical cursor accepted")
	}

	database := newFakeManagedTenantDatabase()
	store := newManagedTenantStoreWithDatabase(database)
	managedTenant := tenant{identifier: tenantID("managed-default"), userID: "owner", managed: true}
	if usageError := store.recordUsage(context.Background(), managedTenant, managedUsageEvent{
		outcomeCode: managedUsageOutcomeCode("provider_error"),
	}); usageError == nil {
		t.Fatal("invalid persisted outcome accepted")
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
		managed:    true,
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
		ProviderNameOpenAI,
		ModelNameGPT41,
		http.StatusOK,
		nil,
		time.Now().Add(-time.Second),
	)
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
		ProviderNameOpenAI,
		ModelNameGPT41,
		http.StatusOK,
		nil,
		time.Now().Add(-time.Second),
	)
	if cancelledLogs.Len() != 0 || !cancelledRecorder.Flushed {
		t.Fatalf("cancelled logs=%+v flushed=%t", cancelledLogs.All(), cancelledRecorder.Flushed)
	}
}
