package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

type managementUsageFailuresTestResponse struct {
	Interval   string                           `json:"interval"`
	Failures   []managementUsageFailureTestItem `json:"failures"`
	NextCursor string                           `json:"next_cursor"`
}

type managementUsageFailureTestItem struct {
	OccurredAt  string `json:"occurred_at"`
	Endpoint    string `json:"endpoint"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	StatusCode  int    `json:"status_code"`
	OutcomeCode string `json:"outcome_code"`
	LatencyMS   int64  `json:"latency_ms"`
}

func TestManagementUsageFailuresRejectsInvalidQueriesAndEnforcesTenantOwnership(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	ownerCookie := managementSessionCookie(t, "failure-owner")
	otherOwnerCookie := managementSessionCookie(t, "failure-other-owner")
	ownerTenantPath := managementDefaultTenantTestPath(t, router, ownerCookie, "")
	otherTenantPath := managementDefaultTenantTestPath(t, router, otherOwnerCookie, "")
	failuresPath := ownerTenantPath + "/usage/failures"

	for _, invalidQuery := range []string{
		"",
		"?interval=unknown",
		"?interval=1d&interval=7d",
		"?interval=30d&unknown=value",
		"?interval=30d&limit=",
		"?interval=30d&limit=0",
		"?interval=30d&limit=01",
		"?interval=30d&limit=101",
		"?interval=30d&limit=1&limit=2",
		"?interval=30d&cursor=",
		"?interval=30d&cursor=not-a-cursor",
		"?interval=30d&cursor=one&cursor=two",
	} {
		request := httptest.NewRequest(http.MethodGet, failuresPath+invalidQuery, nil)
		request.AddCookie(ownerCookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("query=%q status=%d body=%q", invalidQuery, response.Code, response.Body.String())
		}
	}

	unauthorizedRequest := httptest.NewRequest(http.MethodGet, failuresPath+"?interval=30d", nil)
	unauthorizedResponse := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedResponse, unauthorizedRequest)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d body=%q", unauthorizedResponse.Code, unauthorizedResponse.Body.String())
	}

	for _, inaccessiblePath := range []string{
		ownerTenantPath + "-missing/usage/failures?interval=30d",
		otherTenantPath + "/usage/failures?interval=30d",
	} {
		request := httptest.NewRequest(http.MethodGet, inaccessiblePath, nil)
		request.AddCookie(ownerCookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("path=%q status=%d body=%q", inaccessiblePath, response.Code, response.Body.String())
		}
	}

	validRequest := httptest.NewRequest(http.MethodGet, failuresPath+"?interval=30d&limit=1", nil)
	validRequest.AddCookie(ownerCookie)
	validResponse := httptest.NewRecorder()
	router.ServeHTTP(validResponse, validRequest)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("valid status=%d body=%q", validResponse.Code, validResponse.Body.String())
	}
	var payload managementUsageFailuresTestResponse
	if decodeError := json.Unmarshal(validResponse.Body.Bytes(), &payload); decodeError != nil {
		t.Fatalf("decode failures response: %v", decodeError)
	}
	if payload.Interval != "30d" || len(payload.Failures) != 0 || payload.NextCursor != "" {
		t.Fatalf("empty failures payload=%+v", payload)
	}
	var rawPayload map[string]json.RawMessage
	if decodeError := json.Unmarshal(validResponse.Body.Bytes(), &rawPayload); decodeError != nil {
		t.Fatalf("decode failures fields: %v", decodeError)
	}
	actualFields := make([]string, 0, len(rawPayload))
	for fieldName := range rawPayload {
		actualFields = append(actualFields, fieldName)
	}
	sort.Strings(actualFields)
	if !reflect.DeepEqual(actualFields, []string{"failures", "interval"}) {
		t.Fatalf("empty response fields=%v", actualFields)
	}
	for _, supportedInterval := range []string{"all", "7d", "1d"} {
		supportedPayload := requestManagementUsageFailures(t, router, ownerCookie, ownerTenantPath, supportedInterval, 25, "")
		if supportedPayload.Interval != supportedInterval || len(supportedPayload.Failures) != 0 {
			t.Fatalf("interval=%s payload=%+v", supportedInterval, supportedPayload)
		}
	}
}

func TestManagementUsageFailuresExposeSafeCanonicalRowsWithStableSnapshotPagination(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestBody, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Errorf("read upstream request: %v", readError)
			return
		}
		switch {
		case bytes.Contains(requestBody, []byte("rate-limited")):
			http.Error(responseWriter, `{"private":"rate-limit-provider-body"}`, http.StatusTooManyRequests)
			return
		case bytes.Contains(requestBody, []byte("upstream-error")):
			http.Error(responseWriter, `{"private":"upstream-provider-body"}`, http.StatusInternalServerError)
			return
		case bytes.Contains(requestBody, []byte("request-timeout")):
			<-request.Context().Done()
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"success"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstreamServer.Close()

	databasePath := filepath.Join(t.TempDir(), "failure-events.db")
	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{
		DeepSeekBaseURL: upstreamServer.URL,
		MaxPromptBytes:  64,
	}, databasePath)
	ownerCookie := managementSessionCookie(t, "failure-pagination-owner")
	tenantPath := managementDefaultTenantTestPath(t, router, ownerCookie, "")

	saveKeyRequest := authenticatedJSONRequest(
		http.MethodPut,
		tenantPath+"/provider-keys/deepseek",
		managementProviderKeyRequestBody(t, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""),
		ownerCookie,
	)
	saveKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveKeyResponse, saveKeyRequest)
	if saveKeyResponse.Code != http.StatusOK {
		t.Fatalf("save key status=%d body=%q", saveKeyResponse.Code, saveKeyResponse.Body.String())
	}
	saveDictationKeyRequest := authenticatedJSONRequest(
		http.MethodPut,
		tenantPath+"/provider-keys/openai",
		managementProviderKeyRequestBody(t, testManagementOpenAIKey, proxy.ModelNameGPT41, ""),
		ownerCookie,
	)
	saveDictationKeyResponse := httptest.NewRecorder()
	router.ServeHTTP(saveDictationKeyResponse, saveDictationKeyRequest)
	if saveDictationKeyResponse.Code != http.StatusOK {
		t.Fatalf("save dictation key status=%d body=%q", saveDictationKeyResponse.Code, saveDictationKeyResponse.Body.String())
	}
	defaultsRequest := authenticatedJSONRequest(
		http.MethodPut,
		tenantPath+"/defaults",
		managementDefaultsRequestBody(t, proxy.ProviderNameDeepSeek, proxy.ModelNameDeepSeekV4Flash, proxy.ProviderNameOpenAI, proxy.DefaultDictationModel, ""),
		ownerCookie,
	)
	defaultsResponse := httptest.NewRecorder()
	router.ServeHTTP(defaultsResponse, defaultsRequest)
	if defaultsResponse.Code != http.StatusOK {
		t.Fatalf("defaults status=%d body=%q", defaultsResponse.Code, defaultsResponse.Body.String())
	}
	secretRequest := authenticatedJSONRequest(http.MethodPost, tenantPath+"/secrets", `{}`, ownerCookie)
	secretResponse := httptest.NewRecorder()
	router.ServeHTTP(secretResponse, secretRequest)
	if secretResponse.Code != http.StatusOK {
		t.Fatalf("secret status=%d body=%q", secretResponse.Code, secretResponse.Body.String())
	}
	var secretPayload struct {
		Secret string `json:"secret"`
	}
	if decodeError := json.Unmarshal(secretResponse.Body.Bytes(), &secretPayload); decodeError != nil {
		t.Fatalf("decode secret: %v", decodeError)
	}
	secretQuery := url.QueryEscape(secretPayload.Secret)

	requests := []struct {
		request    *http.Request
		wantStatus int
	}{
		{
			request:    httptest.NewRequest(http.MethodGet, "/?key="+secretQuery+"&prompt=invalid&max_tokens=0", nil),
			wantStatus: http.StatusBadRequest,
		},
		{
			request: httptest.NewRequest(
				http.MethodPost,
				"/v2?key="+secretQuery,
				bytes.NewBufferString(`{"messages":[{"role":"user","content":"payload that exceeds the configured request boundary"}]}`),
			),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			request:    httptest.NewRequest(http.MethodGet, "/?key="+secretQuery+"&prompt=rate-limited", nil),
			wantStatus: http.StatusTooManyRequests,
		},
		{
			request:    httptest.NewRequest(http.MethodGet, "/?key="+secretQuery+"&prompt=upstream-error", nil),
			wantStatus: http.StatusBadGateway,
		},
		{
			request: httptest.NewRequest(
				http.MethodGet,
				"/?key="+secretQuery+"&prompt=unavailable&provider="+proxy.ProviderNameMeta+"&model="+proxy.ModelNameMuseSpark11,
				nil,
			),
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			request:    httptest.NewRequest(http.MethodGet, "/?key="+secretQuery+"&prompt=request-timeout", nil),
			wantStatus: http.StatusGatewayTimeout,
		},
	}
	requests[1].request.Header.Set("Content-Type", "application/json")
	requests[5].request.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "1")
	for requestIndex, requestCase := range requests {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, requestCase.request)
		if response.Code != requestCase.wantStatus {
			t.Fatalf("request=%d status=%d want=%d body=%q", requestIndex, response.Code, requestCase.wantStatus, response.Body.String())
		}
	}

	firstPage := requestManagementUsageFailures(t, router, ownerCookie, tenantPath, "30d", 3, "")
	if firstPage.NextCursor == "" || len(firstPage.Failures) != 3 {
		t.Fatalf("first page=%+v", firstPage)
	}
	if got := []string{firstPage.Failures[0].OutcomeCode, firstPage.Failures[1].OutcomeCode, firstPage.Failures[2].OutcomeCode}; !reflect.DeepEqual(got, []string{"request_timeout", "service_unavailable", "upstream_error"}) {
		t.Fatalf("first outcomes=%v", got)
	}
	if firstPage.Failures[0].Endpoint != "text" ||
		firstPage.Failures[0].Provider != proxy.ProviderNameDeepSeek ||
		firstPage.Failures[0].Model != proxy.ModelNameDeepSeekV4Flash ||
		firstPage.Failures[0].StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("first failure=%+v", firstPage.Failures[0])
	}
	for _, invalidCursorRequest := range []*http.Request{
		httptest.NewRequest(http.MethodGet, tenantPath+"/usage/failures?interval=7d&cursor="+url.QueryEscape(firstPage.NextCursor), nil),
		httptest.NewRequest(http.MethodGet, tenantPath+"/usage/failures?interval=30d&cursor="+url.QueryEscape(firstPage.NextCursor+"a"), nil),
	} {
		invalidCursorRequest.AddCookie(ownerCookie)
		invalidCursorResponse := httptest.NewRecorder()
		router.ServeHTTP(invalidCursorResponse, invalidCursorRequest)
		if invalidCursorResponse.Code != http.StatusBadRequest {
			t.Fatalf("invalid cursor status=%d body=%q", invalidCursorResponse.Code, invalidCursorResponse.Body.String())
		}
	}

	newFailureRequest := httptest.NewRequest(http.MethodGet, "/?key="+secretQuery+"&prompt=new&max_tokens=0", nil)
	newFailureResponse := httptest.NewRecorder()
	router.ServeHTTP(newFailureResponse, newFailureRequest)
	if newFailureResponse.Code != http.StatusBadRequest {
		t.Fatalf("new failure status=%d body=%q", newFailureResponse.Code, newFailureResponse.Body.String())
	}

	secondPage := requestManagementUsageFailures(t, router, ownerCookie, tenantPath, "30d", 3, firstPage.NextCursor)
	if secondPage.NextCursor != "" || len(secondPage.Failures) != 3 {
		t.Fatalf("second page=%+v", secondPage)
	}
	if got := []string{secondPage.Failures[0].OutcomeCode, secondPage.Failures[1].OutcomeCode, secondPage.Failures[2].OutcomeCode}; !reflect.DeepEqual(got, []string{"rate_limited", "payload_too_large", "invalid_request"}) {
		t.Fatalf("second outcomes=%v", got)
	}
	for _, page := range []managementUsageFailuresTestResponse{firstPage, secondPage} {
		for _, failure := range page.Failures {
			if strings.Contains(failure.Provider, "private") || strings.Contains(failure.Model, "private") {
				t.Fatalf("failure leaked provider response: %+v", failure)
			}
		}
	}

	refreshedPage := requestManagementUsageFailures(t, router, ownerCookie, tenantPath, "30d", 1, "")
	if len(refreshedPage.Failures) != 1 || refreshedPage.Failures[0].OutcomeCode != "invalid_request" {
		t.Fatalf("refreshed page=%+v", refreshedPage)
	}

	successRequest := httptest.NewRequest(http.MethodGet, "/?key="+secretQuery+"&prompt=success", nil)
	successResponse := httptest.NewRecorder()
	router.ServeHTTP(successResponse, successRequest)
	if successResponse.Code != http.StatusOK || strings.TrimSpace(successResponse.Body.String()) != "success" {
		t.Fatalf("success status=%d body=%q", successResponse.Code, successResponse.Body.String())
	}

	type persistedUsageOutcome struct {
		StatusCode  int    `gorm:"column:status_code"`
		Success     bool   `gorm:"column:success"`
		OutcomeCode string `gorm:"column:outcome_code"`
	}
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open usage database: %v", openError)
	}
	var persistedOutcomes []persistedUsageOutcome
	if queryError := database.
		Table("managed_usage_event_records").
		Select("status_code", "success", "outcome_code").
		Order("id").
		Find(&persistedOutcomes).
		Error; queryError != nil {
		t.Fatalf("load persisted outcomes: %v", queryError)
	}
	actualOutcomes := make([]string, 0, len(persistedOutcomes))
	for _, persistedOutcome := range persistedOutcomes {
		actualOutcomes = append(actualOutcomes, persistedOutcome.OutcomeCode)
		if persistedOutcome.Success != (persistedOutcome.OutcomeCode == "success") {
			t.Fatalf("persisted outcome=%+v", persistedOutcome)
		}
	}
	expectedOutcomes := []string{
		"invalid_request",
		"payload_too_large",
		"rate_limited",
		"upstream_error",
		"service_unavailable",
		"request_timeout",
		"invalid_request",
		"success",
	}
	if !reflect.DeepEqual(actualOutcomes, expectedOutcomes) {
		t.Fatalf("persisted outcomes=%v want=%v", actualOutcomes, expectedOutcomes)
	}

	var tableColumns []struct {
		Name string `gorm:"column:name"`
	}
	if queryError := database.Raw("PRAGMA table_info(managed_usage_event_records)").Scan(&tableColumns).Error; queryError != nil {
		t.Fatalf("read usage columns: %v", queryError)
	}
	for _, tableColumn := range tableColumns {
		switch tableColumn.Name {
		case "error", "error_message", "prompt", "response", "provider_body", "raw_body":
			t.Fatalf("usage table retained prohibited column %q", tableColumn.Name)
		}
	}
}

func requestManagementUsageFailures(t *testing.T, router http.Handler, sessionCookie *http.Cookie, tenantPath string, interval string, limit int, cursor string) managementUsageFailuresTestResponse {
	t.Helper()
	query := url.Values{"interval": []string{interval}, "limit": []string{strconv.Itoa(limit)}}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	request := httptest.NewRequest(http.MethodGet, tenantPath+"/usage/failures?"+query.Encode(), nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("failures status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("failures cache-control=%q want=no-store", response.Header().Get("Cache-Control"))
	}
	var rawPayload struct {
		Failures []map[string]json.RawMessage `json:"failures"`
	}
	if decodeError := json.Unmarshal(response.Body.Bytes(), &rawPayload); decodeError != nil {
		t.Fatalf("decode raw failures: %v", decodeError)
	}
	for _, rawFailure := range rawPayload.Failures {
		actualFields := make([]string, 0, len(rawFailure))
		for fieldName := range rawFailure {
			actualFields = append(actualFields, fieldName)
		}
		sort.Strings(actualFields)
		expectedFields := []string{"endpoint", "latency_ms", "model", "occurred_at", "outcome_code", "provider", "status_code"}
		if !reflect.DeepEqual(actualFields, expectedFields) {
			t.Fatalf("failure fields=%v want=%v", actualFields, expectedFields)
		}
	}
	var payload managementUsageFailuresTestResponse
	if decodeError := json.Unmarshal(response.Body.Bytes(), &payload); decodeError != nil {
		t.Fatalf("decode failures: %v", decodeError)
	}
	return payload
}
