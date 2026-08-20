package proxy

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var errInternalTestDatabase = errors.New("database failed")
var errInternalTestRead = errors.New("read failed")

const testManagedProviderKeyEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func TestResponseConstructionHonorsRequestDeadline(t *testing.T) {
	managedTenants := newManagedTenantStoreWithDatabase(newFakeManagedTenantDatabase())
	testCases := []struct {
		name     string
		complete func(*gin.Context)
	}{
		{
			name: "text",
			complete: func(ginContext *gin.Context) {
				completeChatRequest(
					ginContext,
					chatRequestParameters{},
					textGenerationResult{text: "late response"},
					tenant{},
					usageEndpointText,
					managedTenants,
					zap.NewNop().Sugar(),
					time.Now(),
				)
			},
		},
		{
			name: "dictation",
			complete: func(ginContext *gin.Context) {
				completeDictationRequest(
					ginContext,
					"late transcription",
					tenant{},
					providerDefinition{},
					modelID(""),
					managedTenants,
					zap.NewNop().Sugar(),
					time.Now(),
				)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			requestContext, cancelRequest := context.WithCancelCause(context.Background())
			cancelRequest(errRequestTimeoutBudgetExpired)
			responseRecorder := httptest.NewRecorder()
			ginContext, _ := gin.CreateTestContext(responseRecorder)
			ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(requestContext)
			timeoutState := &requestTimeoutState{
				budget:              newRequestTimeoutBudget(7),
				outcome:             requestOutcomeValidation,
				managedUsageOutcome: managedUsageOutcomeInvalidRequest,
			}
			ginContext.Set(contextKeyRequestTimeoutState, timeoutState)

			testCase.complete(ginContext)

			if responseRecorder.Code != http.StatusGatewayTimeout {
				subTest.Fatalf("status=%d want=%d", responseRecorder.Code, http.StatusGatewayTimeout)
			}
			if responseRecorder.Body.String() != `{"error":{"code":"request_timeout","request_timeout_seconds":7}}` {
				subTest.Fatalf("body=%q", responseRecorder.Body.String())
			}
			if timeoutState.outcome != requestOutcomeProxyTimeout {
				subTest.Fatalf("outcome=%q want=%q", timeoutState.outcome, requestOutcomeProxyTimeout)
			}
			if timeoutState.managedUsageOutcome != managedUsageOutcomeRequestTimeout {
				subTest.Fatalf("managed outcome=%q want=%q", timeoutState.managedUsageOutcome, managedUsageOutcomeRequestTimeout)
			}
		})
	}
}

func TestRequestFailureOutcomeClassifiesProxyOverload(t *testing.T) {
	if outcome := requestFailureOutcome(errQueueFull); outcome != requestOutcomeProxyOverload {
		t.Fatalf("queue outcome=%q want=%q", outcome, requestOutcomeProxyOverload)
	}
	if outcome := requestFailureOutcome(errInternalTestDatabase); outcome != requestOutcomeProviderFailure {
		t.Fatalf("provider outcome=%q want=%q", outcome, requestOutcomeProviderFailure)
	}
	for _, testCase := range []struct {
		requestError error
		expected     managedUsageOutcomeCode
	}{
		{requestError: ErrProviderRateLimited, expected: managedUsageOutcomeRateLimited},
		{requestError: ErrProviderNotConfigured, expected: managedUsageOutcomeServiceUnavailable},
		{requestError: errQueueFull, expected: managedUsageOutcomeServiceUnavailable},
		{requestError: context.DeadlineExceeded, expected: managedUsageOutcomeRequestTimeout},
		{requestError: context.Canceled, expected: managedUsageOutcomeRequestTimeout},
		{requestError: errInternalTestDatabase, expected: managedUsageOutcomeUpstreamError},
	} {
		if outcome := managedRequestFailureOutcome(testCase.requestError); outcome != testCase.expected {
			t.Fatalf("request error=%v outcome=%q want=%q", testCase.requestError, outcome, testCase.expected)
		}
	}
}

func TestManagedUsageSummaryBucketsAndOrdering(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 30, 0, 0, time.UTC)
	for _, invalidInterval := range []string{"", "30", "1h", "ALL"} {
		if _, intervalError := newUsageInterval(invalidInterval); !errors.Is(intervalError, errManagedUsageIntervalInvalid) {
			t.Fatalf("interval=%q error=%v", invalidInterval, intervalError)
		}
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != errManagedUsageIntervalInvalid {
				t.Fatalf("invalid constructed interval panic=%v", recovered)
			}
		}()
		_, _, _, _ = usageInterval("invalid").finiteWindow()
	}()
	for _, intervalExpectation := range []struct {
		identifier  string
		bucketUnit  usageBucketUnit
		bucketCount int
		duration    time.Duration
	}{
		{identifier: "30d", bucketUnit: usageBucketUnitDay, bucketCount: 30, duration: 30 * 24 * time.Hour},
		{identifier: "7d", bucketUnit: usageBucketUnitDay, bucketCount: 7, duration: 7 * 24 * time.Hour},
		{identifier: "1d", bucketUnit: usageBucketUnitHour, bucketCount: 24, duration: 24 * time.Hour},
	} {
		interval, intervalError := newUsageInterval(intervalExpectation.identifier)
		if intervalError != nil {
			t.Fatalf("interval=%s error=%v", intervalExpectation.identifier, intervalError)
		}
		periodStart := now.Add(-intervalExpectation.duration)
		summary := summarizeManagedUsage([]managedUsageEventRecord{
			{TenantID: "tenant", ProviderID: "excluded-before", ModelID: "old-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: periodStart.Add(-time.Nanosecond)},
			{TenantID: "tenant", ProviderID: "included-start", ModelID: "start-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: periodStart},
			{TenantID: "tenant", ProviderID: "included-end", ModelID: "end-model", Endpoint: usageEndpointText, StatusCode: http.StatusBadGateway, Success: false, CreatedAt: now},
			{TenantID: "tenant", ProviderID: "excluded-after", ModelID: "future-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: now.Add(time.Nanosecond)},
		}, interval, now)
		if summary.interval != interval || summary.bucketUnit != intervalExpectation.bucketUnit || len(summary.buckets) != intervalExpectation.bucketCount || summary.totals.requests != 2 || summary.totals.successfulRequests != 1 || summary.totals.failedRequests != 1 {
			t.Fatalf("interval=%s summary=%+v buckets=%d", intervalExpectation.identifier, summary, len(summary.buckets))
		}
		if !summary.buckets[0].start.Equal(periodStart) || summary.buckets[0].aggregate.requests != 1 || summary.buckets[len(summary.buckets)-1].aggregate.requests != 1 {
			t.Fatalf("interval=%s first=%+v last=%+v", intervalExpectation.identifier, summary.buckets[0], summary.buckets[len(summary.buckets)-1])
		}
	}

	allInterval, allIntervalError := newUsageInterval("all")
	if allIntervalError != nil {
		t.Fatalf("all interval error=%v", allIntervalError)
	}
	emptyAllSummary := summarizeManagedUsage(nil, allInterval, now)
	if emptyAllSummary.interval != allInterval || emptyAllSummary.bucketUnit != usageBucketUnitDay || len(emptyAllSummary.buckets) != 0 {
		t.Fatalf("empty all summary=%+v", emptyAllSummary)
	}
	earliest := time.Date(2026, 4, 20, 23, 59, 0, 0, time.FixedZone("fixture", -7*60*60))
	allSummary := summarizeManagedUsage([]managedUsageEventRecord{
		{TenantID: "tenant", ProviderID: "earliest", ModelID: "earliest-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: earliest},
		{TenantID: "tenant", ProviderID: "current", ModelID: "current-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: now},
		{TenantID: "tenant", ProviderID: "future", ModelID: "future-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: now.Add(time.Nanosecond)},
	}, allInterval, now)
	expectedAllStart := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	expectedAllBucketCount := int(now.Sub(expectedAllStart).Hours()/24) + 1
	if allSummary.totals.requests != 2 || len(allSummary.buckets) != expectedAllBucketCount || !allSummary.buckets[0].start.Equal(expectedAllStart) || allSummary.providers[0].providerIdentifier != "current" {
		t.Fatalf("all summary=%+v buckets=%d providers=%+v", allSummary.totals, len(allSummary.buckets), allSummary.providers)
	}

	adminPeriodStart := usagePeriodStart(now)
	adminSummary := summarizeManagedAdminUsage([]managedUsageEventRecord{
		{ProviderID: "excluded-before", ModelID: "old-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: adminPeriodStart.Add(-time.Nanosecond)},
		{ProviderID: "included", ModelID: "current-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: now},
		{ProviderID: "excluded-after", ModelID: "future-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: now.Add(time.Nanosecond)},
	}, now)
	if adminSummary.periodDays != managedUsageSummaryDays || adminSummary.totals.requests != 1 || adminSummary.daily[len(adminSummary.daily)-1].aggregate.requests != 1 {
		t.Fatalf("admin summary=%+v daily=%+v", adminSummary.totals, adminSummary.daily)
	}

	providers := usageProviderBucketList(map[string]managedUsageAggregate{
		"beta":  {requests: 1},
		"alpha": {requests: 1},
		"gamma": {requests: 2},
	})
	if len(providers) != 3 || providers[0].providerIdentifier != "gamma" || providers[1].providerIdentifier != "alpha" || providers[2].providerIdentifier != "beta" {
		t.Fatalf("providers=%+v", providers)
	}

	models := usageModelBucketList(map[string]managedUsageModelBucket{
		"beta/model": {
			providerIdentifier: "beta",
			modelIdentifier:    "model",
			aggregate:          managedUsageAggregate{requests: 1},
		},
		"alpha/zeta": {
			providerIdentifier: "alpha",
			modelIdentifier:    "zeta",
			aggregate:          managedUsageAggregate{requests: 1},
		},
		"alpha/alpha": {
			providerIdentifier: "alpha",
			modelIdentifier:    "alpha",
			aggregate:          managedUsageAggregate{requests: 1},
		},
		"gamma/model": {
			providerIdentifier: "gamma",
			modelIdentifier:    "model",
			aggregate:          managedUsageAggregate{requests: 2},
		},
	})
	if len(models) != 4 ||
		models[0].providerIdentifier != "gamma" ||
		models[1].providerIdentifier != "alpha" ||
		models[1].modelIdentifier != "alpha" ||
		models[2].providerIdentifier != "alpha" ||
		models[2].modelIdentifier != "zeta" ||
		models[3].providerIdentifier != "beta" {
		t.Fatalf("models=%+v", models)
	}
}

func TestTextRequestDefaultsForProviderInternalEdges(t *testing.T) {
	providers := newProviderRegistry(Configuration{
		ModelCatalog: internalTestModelCatalog(
			internalTestOffering(ProviderNameOpenAI, ModelNameGPT41, []string{ModelOperationText}, []string{ModelOperationText}),
			internalTestOffering(ProviderNameOpenAI, ModelNameGPT55, []string{ModelOperationText}, nil),
			internalTestOffering(ProviderNameDeepSeek, ModelNameDeepSeekV4Flash, []string{ModelOperationText}, []string{ModelOperationText}),
		),
	})
	requestTenant := tenant{
		defaults: newTenantDefaults(TenantDefaults{
			Provider:     ProviderNameOpenAI,
			Model:        ModelNameGPT41,
			SystemPrompt: "tenant system",
		}),
	}
	explicitDefaults := textRequestDefaultsForProvider(ProviderNameDeepSeek, requestTenant, providers)
	if explicitDefaults.model != ModelNameDeepSeekV4Flash || explicitDefaults.systemPrompt != "tenant system" {
		t.Fatalf("explicit defaults=%+v", explicitDefaults)
	}

	managedNoSettingsDefaults := textRequestDefaultsForProvider(ProviderNameOpenAI, requestTenant, providers)
	if managedNoSettingsDefaults.model != ModelNameGPT41 || managedNoSettingsDefaults.systemPrompt != "tenant system" {
		t.Fatalf("managed no-settings defaults=%+v", managedNoSettingsDefaults)
	}
	managedUnknownProviderDefaults := textRequestDefaultsForProvider("unknown-provider", requestTenant, providers)
	if managedUnknownProviderDefaults.model != "" || managedUnknownProviderDefaults.systemPrompt != "tenant system" {
		t.Fatalf("managed unknown-provider defaults=%+v", managedUnknownProviderDefaults)
	}

	requestTenant.providerSettings = map[providerID]managedProviderSettings{
		newProviderID(ProviderNameOpenAI): {
			textModel:    ModelNameGPT55,
			systemPrompt: "saved system",
		},
	}
	managedSavedOmittedDefaults := textRequestDefaultsForProvider("", requestTenant, providers)
	if managedSavedOmittedDefaults.model != ModelNameGPT41 || managedSavedOmittedDefaults.systemPrompt != "tenant system" {
		t.Fatalf("managed saved omitted defaults=%+v", managedSavedOmittedDefaults)
	}
	managedSavedExplicitDefaults := textRequestDefaultsForProvider(ProviderNameOpenAI, requestTenant, providers)
	if managedSavedExplicitDefaults.model != ModelNameGPT55 || managedSavedExplicitDefaults.systemPrompt != "saved system" {
		t.Fatalf("managed saved explicit defaults=%+v", managedSavedExplicitDefaults)
	}
}

func TestProviderKeyRejectionInternalEdges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	ginContext.Request.Body = failingReadCloser{}
	ginContext.Set(contextKeyRequestTimeoutState, &requestTimeoutState{})
	if _, ok := readJSONProxyBody(ginContext); ok || response.Code != http.StatusBadRequest {
		t.Fatalf("readJSONProxyBody ok=%v status=%d", ok, response.Code)
	}

	formResponse := httptest.NewRecorder()
	formContext, _ := gin.CreateTestContext(formResponse)
	formContext.Request = httptest.NewRequest(http.MethodPost, "/dictate", nil)
	if rejectClientProviderCredentialsFromForm(formContext) {
		t.Fatalf("nil multipart form must not be rejected")
	}
	if forbiddenClientProviderCredentialParameter(" ") {
		t.Fatalf("blank provider credential parameter must not be forbidden")
	}
}

func TestProviderSummaryInternalEdges(t *testing.T) {
	textModels := sortedTextModelSummaries(map[string]textModelDefinition{
		"alias-a": {identifier: newModelID("same-text-model")},
		"alias-b": {identifier: newModelID("same-text-model")},
		"other":   {identifier: newModelID("other-text-model")},
	})
	if !reflect.DeepEqual(textModels, []textModelSummary{{identifier: "other-text-model"}, {identifier: "same-text-model"}}) {
		t.Fatalf("text models=%v", textModels)
	}
	dictationModels := sortedDictationModels(map[string]dictationModelDefinition{
		"alias-a": {identifier: newModelID("same-dictation-model")},
		"alias-b": {identifier: newModelID("same-dictation-model")},
		"other":   {identifier: newModelID("other-dictation-model")},
	})
	if !reflect.DeepEqual(dictationModels, []string{"other-dictation-model", "same-dictation-model"}) {
		t.Fatalf("dictation models=%v", dictationModels)
	}
}

func TestManagementConfigurationInternalEdges(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, decodeError := decodeManagedProviderKey(shortKey); decodeError == nil || !strings.Contains(decodeError.Error(), "invalid_length") {
		t.Fatalf("short key decode error=%v want invalid_length", decodeError)
	}
	for _, rawEmail := range []string{" ", "Admin <admin@example.com>"} {
		if _, emailError := normalizeManagementEmail(rawEmail); !errors.Is(emailError, ErrInvalidManagementConfiguration) {
			t.Fatalf("admin email error=%v want %v", emailError, ErrInvalidManagementConfiguration)
		}
	}
	invalidQueueConfiguration := ManagementConfiguration{
		PublicOrigin:             "https://llm-proxy.example",
		UIDescription:            "LLM Proxy",
		UIOrigins:                []string{"https://llm-proxy.example"},
		TAuthURL:                 "https://tauth.example",
		TAuthTenantID:            "llm-proxy",
		GoogleClientID:           "google-client",
		LoginPath:                "/auth/google",
		LogoutPath:               "/auth/logout",
		NoncePath:                "/auth/nonce",
		SessionPath:              "/auth/session",
		DatabasePath:             "management.sqlite",
		UsageQueueSize:           -1,
		ProviderKeyEncryptionKey: testManagedProviderKeyEncryptionKey,
		ManagementAPIOrigin:      "https://llm-proxy-api.example",
		ProxyOrigin:              "https://llm-proxy-api.example",
	}
	if validationError := validateManagementConfiguration(invalidQueueConfiguration); !errors.Is(validationError, ErrInvalidManagementConfiguration) || !strings.Contains(validationError.Error(), "management.usage_queue_size") {
		t.Fatalf("usage queue validation error=%v", validationError)
	}
}

type failingReadCloser struct{}

func (failingReadCloser) Read([]byte) (int, error) {
	return 0, errInternalTestRead
}

func (failingReadCloser) Close() error {
	return nil
}
