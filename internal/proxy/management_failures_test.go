package proxy_test

import (
	"bytes"
	"context"
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
	"time"

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

type managementAccountUsageFailuresTestResponse struct {
	Interval   string                                  `json:"interval"`
	Failures   []managementAccountUsageFailureTestItem `json:"failures"`
	NextCursor string                                  `json:"next_cursor"`
}

type managementAccountUsageFailureTestItem struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	managementUsageFailureTestItem
}

func TestOpenAIResponsesCompleteTruncatedOutputAndRecordOneSuccessAtPublicV2Boundary(testingInstance *testing.T) {
	const (
		incompletePrompt = "b080-incomplete"
		completedPrompt  = "b080-completed"
		partialText      = "provider-truncated partial text"
		completionText   = " and recovered suffix"
		completedText    = "complete response text"
	)
	upstreamRequestCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		upstreamRequestCount++
		requestBody, readError := io.ReadAll(request.Body)
		if readError != nil {
			testingInstance.Errorf("read upstream request: %v", readError)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		switch {
		case bytes.Contains(requestBody, []byte(incompletePrompt)) && bytes.Contains(requestBody, []byte("Return only the missing suffix")):
			_, _ = responseWriter.Write([]byte(`{"id":"b080-recovered-response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + completionText + `"}]}],"usage":{"input_tokens":5,"output_tokens":7,"total_tokens":12}}`))
		case bytes.Contains(requestBody, []byte(incompletePrompt)):
			_, _ = responseWriter.Write([]byte(`{"id":"b080-incomplete-response","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + partialText + `"}]}],"usage":{"input_tokens":1599,"output_tokens":2048,"total_tokens":3647}}`))
		case bytes.Contains(requestBody, []byte(completedPrompt)):
			_, _ = responseWriter.Write([]byte(`{"id":"b080-completed-response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + completedText + `"}]}],"usage":{"input_tokens":7,"output_tokens":11,"total_tokens":18}}`))
		default:
			http.Error(responseWriter, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer upstreamServer.Close()

	databasePath := filepath.Join(testingInstance.TempDir(), "openai-terminal-events.db")
	router := newManagementRouterWithDatabasePath(testingInstance, proxy.Configuration{OpenAIBaseURL: upstreamServer.URL}, databasePath)
	ownerCookie := managementSessionCookie(testingInstance, "openai-terminal-owner")
	tenantIdentifier := managementDefaultTenantTestID(testingInstance, router, ownerCookie)
	saveManagementProviderKey(testingInstance, router, ownerCookie, tenantIdentifier, testManagementOpenAIKey, proxy.ModelNameGPT41, "")
	secret := generateManagementTenantSecret(testingInstance, router, ownerCookie, tenantIdentifier)

	requestV2 := func(prompt string, maxTokens int) *httptest.ResponseRecorder {
		testingInstance.Helper()
		requestBody, marshalError := json.Marshal(map[string]any{
			"messages":   []map[string]string{{"role": "user", "content": prompt}},
			"max_tokens": maxTokens,
		})
		if marshalError != nil {
			testingInstance.Fatalf("marshal v2 request: %v", marshalError)
		}
		request := httptest.NewRequest(http.MethodPost, "/v2?key="+url.QueryEscape(secret), bytes.NewReader(requestBody))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	incompleteResponse := requestV2(incompletePrompt, 2048)
	if incompleteResponse.Code != http.StatusOK || incompleteResponse.Body.String() != partialText+completionText {
		testingInstance.Fatalf("incomplete status=%d body=%q", incompleteResponse.Code, incompleteResponse.Body.String())
	}
	if incompleteResponse.Header().Get(testHeaderLLMProxyRequestTokens) != "1604" ||
		incompleteResponse.Header().Get(testHeaderLLMProxyResponseTokens) != "2055" ||
		incompleteResponse.Header().Get(testHeaderLLMProxyTotalTokens) != "3659" {
		testingInstance.Fatalf("recovered token headers=%v", incompleteResponse.Header())
	}

	completedResponse := requestV2(completedPrompt, 2048)
	if completedResponse.Code != http.StatusOK || completedResponse.Body.String() != completedText {
		testingInstance.Fatalf("completed status=%d body=%q", completedResponse.Code, completedResponse.Body.String())
	}
	if completedResponse.Header().Get(testHeaderLLMProxyRequestTokens) != "7" ||
		completedResponse.Header().Get(testHeaderLLMProxyResponseTokens) != "11" ||
		completedResponse.Header().Get(testHeaderLLMProxyTotalTokens) != "18" {
		testingInstance.Fatalf("completed token headers=%v", completedResponse.Header())
	}
	if upstreamRequestCount != 3 {
		testingInstance.Fatalf("upstream requests=%d want=3", upstreamRequestCount)
	}

	type persistedOpenAIUsageEvent struct {
		Endpoint       string `gorm:"column:endpoint"`
		Provider       string `gorm:"column:provider_id"`
		Model          string `gorm:"column:model_id"`
		StatusCode     int    `gorm:"column:status_code"`
		Success        bool   `gorm:"column:success"`
		OutcomeCode    string `gorm:"column:outcome_code"`
		RequestTokens  int    `gorm:"column:request_tokens"`
		ResponseTokens int    `gorm:"column:response_tokens"`
		TotalTokens    int    `gorm:"column:total_tokens"`
	}
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		testingInstance.Fatalf("open usage database: %v", openError)
	}
	var usageEvents []persistedOpenAIUsageEvent
	if queryError := database.
		Table("managed_usage_event_records").
		Select("endpoint", "provider_id", "model_id", "status_code", "success", "outcome_code", "request_tokens", "response_tokens", "total_tokens").
		Order("id").
		Find(&usageEvents).
		Error; queryError != nil {
		testingInstance.Fatalf("load OpenAI usage events: %v", queryError)
	}
	expectedUsageEvents := []persistedOpenAIUsageEvent{
		{Endpoint: "v2", Provider: proxy.ProviderNameOpenAI, Model: proxy.ModelNameGPT41, StatusCode: http.StatusOK, Success: true, OutcomeCode: "success", RequestTokens: 1604, ResponseTokens: 2055, TotalTokens: 3659},
		{Endpoint: "v2", Provider: proxy.ProviderNameOpenAI, Model: proxy.ModelNameGPT41, StatusCode: http.StatusOK, Success: true, OutcomeCode: "success", RequestTokens: 7, ResponseTokens: 11, TotalTokens: 18},
	}
	if !reflect.DeepEqual(usageEvents, expectedUsageEvents) {
		testingInstance.Fatalf("usage events=%+v want=%+v", usageEvents, expectedUsageEvents)
	}
}

func TestOpenAIPolledIncompleteUsesLatestSnapshotThenCompletesAtPublicV2Boundary(testingInstance *testing.T) {
	const (
		responseIdentifier = "b080-polled-usage"
		partialText        = "polled provider-truncated partial text"
	)
	upstreamRequestCount := 0
	postRequestCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		upstreamRequestCount++
		responseWriter.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost:
			postRequestCount++
			if postRequestCount == 2 {
				_, _ = responseWriter.Write([]byte(`{"id":"b085-polled-complete","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":" recovered suffix"}]}],"usage":{"input_tokens":5,"output_tokens":7,"total_tokens":12}}`))
				return
			}
			_, _ = responseWriter.Write([]byte(`{"id":"` + responseIdentifier + `","status":"queued","usage":{"input_tokens":11,"output_tokens":13,"total_tokens":24}}`))
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/"+responseIdentifier):
			_, _ = responseWriter.Write([]byte(`{"id":"` + responseIdentifier + `","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + partialText + `"}]}],"usage":{"input_tokens":17,"output_tokens":19,"total_tokens":36}}`))
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	defer upstreamServer.Close()

	databasePath := filepath.Join(testingInstance.TempDir(), "openai-polled-usage.db")
	router := newManagementRouterWithDatabasePath(testingInstance, proxy.Configuration{OpenAIBaseURL: upstreamServer.URL}, databasePath)
	ownerCookie := managementSessionCookie(testingInstance, "openai-polled-usage-owner")
	tenantIdentifier := managementDefaultTenantTestID(testingInstance, router, ownerCookie)
	saveManagementProviderKey(testingInstance, router, ownerCookie, tenantIdentifier, testManagementOpenAIKey, proxy.ModelNameGPT41, "")
	secret := generateManagementTenantSecret(testingInstance, router, ownerCookie, tenantIdentifier)

	requestBody := bytes.NewBufferString(`{"messages":[{"role":"user","content":"poll this response"}],"max_tokens":2048}`)
	request := httptest.NewRequest(http.MethodPost, "/v2?key="+url.QueryEscape(secret), requestBody)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != partialText+" recovered suffix" {
		testingInstance.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Header().Get(testHeaderLLMProxyRequestTokens) != "22" ||
		response.Header().Get(testHeaderLLMProxyResponseTokens) != "26" ||
		response.Header().Get(testHeaderLLMProxyTotalTokens) != "48" {
		testingInstance.Fatalf("token headers=%v", response.Header())
	}
	if upstreamRequestCount != 3 {
		testingInstance.Fatalf("upstream requests=%d want=3", upstreamRequestCount)
	}

	type persistedUsageEvent struct {
		StatusCode     int    `gorm:"column:status_code"`
		Success        bool   `gorm:"column:success"`
		OutcomeCode    string `gorm:"column:outcome_code"`
		RequestTokens  int    `gorm:"column:request_tokens"`
		ResponseTokens int    `gorm:"column:response_tokens"`
		TotalTokens    int    `gorm:"column:total_tokens"`
	}
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		testingInstance.Fatalf("open usage database: %v", openError)
	}
	var usageEvents []persistedUsageEvent
	if queryError := database.
		Table("managed_usage_event_records").
		Select("status_code", "success", "outcome_code", "request_tokens", "response_tokens", "total_tokens").
		Order("id").
		Find(&usageEvents).
		Error; queryError != nil {
		testingInstance.Fatalf("load usage events: %v", queryError)
	}
	expectedUsageEvents := []persistedUsageEvent{{
		StatusCode:     http.StatusOK,
		Success:        true,
		OutcomeCode:    "success",
		RequestTokens:  22,
		ResponseTokens: 26,
		TotalTokens:    48,
	}}
	if !reflect.DeepEqual(usageEvents, expectedUsageEvents) {
		testingInstance.Fatalf("usage events=%+v want=%+v", usageEvents, expectedUsageEvents)
	}
}

func TestProviderCompletionSignalsRecoverPartialTextAndAggregateUsageAtPublicV2Boundary(testingInstance *testing.T) {
	const (
		chatPartialText      = "chat completion partial text"
		geminiPartialText    = "gemini partial text"
		anthropicPartialText = "anthropic partial text"
		completionText       = " recovered suffix"
	)
	upstreamRequestCounts := map[string]int{}
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/chat/completions":
			upstreamRequestCounts["chat"]++
			if upstreamRequestCounts["chat"] == 2 {
				_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"` + completionText + `"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`))
				return
			}
			_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"` + chatPartialText + `"},"finish_reason":"length"}],"usage":{"prompt_tokens":31,"completion_tokens":47,"total_tokens":78}}`))
		case strings.HasSuffix(request.URL.Path, ":generateContent"):
			upstreamRequestCounts["gemini"]++
			if upstreamRequestCounts["gemini"] == 2 {
				_, _ = responseWriter.Write([]byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"` + completionText + `"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":7,"totalTokenCount":12}}`))
				return
			}
			_, _ = responseWriter.Write([]byte(`{"candidates":[{"finishReason":"MAX_TOKENS","content":{"parts":[{"text":"` + geminiPartialText + `"}]}}],"usageMetadata":{"promptTokenCount":41,"candidatesTokenCount":53,"totalTokenCount":94}}`))
		case request.URL.Path == "/v1/messages":
			upstreamRequestCounts["anthropic"]++
			if upstreamRequestCounts["anthropic"] == 2 {
				_, _ = responseWriter.Write([]byte(`{"content":[{"type":"text","text":"` + completionText + `"}],"stop_reason":"end_turn","usage":{"input_tokens":7,"output_tokens":9}}`))
				return
			}
			_, _ = responseWriter.Write([]byte(`{"content":[{"type":"text","text":"` + anthropicPartialText + `"}],"stop_reason":"max_tokens","usage":{"input_tokens":59,"output_tokens":61}}`))
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	defer upstreamServer.Close()

	databasePath := filepath.Join(testingInstance.TempDir(), "provider-completion-events.db")
	router := newManagementRouterWithDatabasePath(testingInstance, proxy.Configuration{
		DeepSeekBaseURL:  upstreamServer.URL,
		GeminiBaseURL:    upstreamServer.URL,
		AnthropicBaseURL: upstreamServer.URL,
	}, databasePath)
	ownerCookie := managementSessionCookie(testingInstance, "provider-completion-owner")
	tenantIdentifier := managementDefaultTenantTestID(testingInstance, router, ownerCookie)
	saveProviderKey := func(providerIdentifier string, apiKey string, modelIdentifier string) {
		testingInstance.Helper()
		request := authenticatedJSONRequest(
			http.MethodPut,
			managementTenantTestPath(tenantIdentifier, "/provider-keys/"+providerIdentifier),
			managementProviderKeyRequestBody(testingInstance, apiKey, modelIdentifier, ""),
			ownerCookie,
		)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			testingInstance.Fatalf("save provider=%s status=%d body=%q", providerIdentifier, response.Code, response.Body.String())
		}
	}
	saveProviderKey(proxy.ProviderNameDeepSeek, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash)
	saveProviderKey(proxy.ProviderNameGemini, "sk-user-gemini", proxy.ModelNameGemini25Flash)
	saveProviderKey(proxy.ProviderNameAnthropic, "sk-user-anthropic", proxy.ModelNameClaudeSonnet46)
	secret := generateManagementTenantSecret(testingInstance, router, ownerCookie, tenantIdentifier)

	testCases := []struct {
		provider    string
		model       string
		partialText string
	}{
		{provider: proxy.ProviderNameDeepSeek, model: proxy.ModelNameDeepSeekV4Flash, partialText: chatPartialText},
		{provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, partialText: geminiPartialText},
		{provider: proxy.ProviderNameAnthropic, model: proxy.ModelNameClaudeSonnet46, partialText: anthropicPartialText},
	}
	for _, testCase := range testCases {
		requestBody, marshalError := json.Marshal(map[string]any{
			"messages": []map[string]string{{"role": "user", "content": "complete this response"}},
			"model":    testCase.model,
		})
		if marshalError != nil {
			testingInstance.Fatalf("marshal provider=%s request: %v", testCase.provider, marshalError)
		}
		requestPath := "/v2?key=" + url.QueryEscape(secret) + "&provider=" + url.QueryEscape(testCase.provider)
		request := httptest.NewRequest(http.MethodPost, requestPath, bytes.NewReader(requestBody))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.String() != testCase.partialText+completionText {
			testingInstance.Fatalf("provider=%s status=%d body=%q", testCase.provider, response.Code, response.Body.String())
		}
	}

	type persistedProviderUsageEvent struct {
		Endpoint       string `gorm:"column:endpoint"`
		Provider       string `gorm:"column:provider_id"`
		Model          string `gorm:"column:model_id"`
		StatusCode     int    `gorm:"column:status_code"`
		Success        bool   `gorm:"column:success"`
		OutcomeCode    string `gorm:"column:outcome_code"`
		RequestTokens  int    `gorm:"column:request_tokens"`
		ResponseTokens int    `gorm:"column:response_tokens"`
		TotalTokens    int    `gorm:"column:total_tokens"`
	}
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		testingInstance.Fatalf("open usage database: %v", openError)
	}
	var usageEvents []persistedProviderUsageEvent
	if queryError := database.
		Table("managed_usage_event_records").
		Select("endpoint", "provider_id", "model_id", "status_code", "success", "outcome_code", "request_tokens", "response_tokens", "total_tokens").
		Order("id").
		Find(&usageEvents).
		Error; queryError != nil {
		testingInstance.Fatalf("load provider usage events: %v", queryError)
	}
	expectedUsageEvents := []persistedProviderUsageEvent{
		{Endpoint: "v2", Provider: proxy.ProviderNameDeepSeek, Model: proxy.ModelNameDeepSeekV4Flash, StatusCode: http.StatusOK, Success: true, OutcomeCode: "success", RequestTokens: 36, ResponseTokens: 54, TotalTokens: 90},
		{Endpoint: "v2", Provider: proxy.ProviderNameGemini, Model: proxy.ModelNameGemini25Flash, StatusCode: http.StatusOK, Success: true, OutcomeCode: "success", RequestTokens: 46, ResponseTokens: 60, TotalTokens: 106},
		{Endpoint: "v2", Provider: proxy.ProviderNameAnthropic, Model: proxy.ModelNameClaudeSonnet46, StatusCode: http.StatusOK, Success: true, OutcomeCode: "success", RequestTokens: 66, ResponseTokens: 70, TotalTokens: 136},
	}
	if !reflect.DeepEqual(usageEvents, expectedUsageEvents) {
		testingInstance.Fatalf("usage events=%+v want=%+v", usageEvents, expectedUsageEvents)
	}
}

func TestCompletionCoordinatorDeadlineRecordsAccumulatedUsageAsOneTimeout(testingInstance *testing.T) {
	upstreamRequestCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		upstreamRequestCount++
		if upstreamRequestCount == 1 {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"partial text"},"finish_reason":"length"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`))
			return
		}
		select {
		case <-request.Context().Done():
		case <-time.After(750 * time.Millisecond):
		}
	}))
	defer upstreamServer.Close()

	databasePath := filepath.Join(testingInstance.TempDir(), "completion-timeout.db")
	router := newManagementRouterWithDatabasePath(testingInstance, proxy.Configuration{DeepSeekBaseURL: upstreamServer.URL}, databasePath)
	ownerCookie := managementSessionCookie(testingInstance, "completion-timeout-owner")
	tenantIdentifier := managementDefaultTenantTestID(testingInstance, router, ownerCookie)
	saveProviderRequest := authenticatedJSONRequest(
		http.MethodPut,
		managementTenantTestPath(tenantIdentifier, "/provider-keys/"+proxy.ProviderNameDeepSeek),
		managementProviderKeyRequestBody(testingInstance, testManagementDeepSeekKey, proxy.ModelNameDeepSeekV4Flash, ""),
		ownerCookie,
	)
	saveProviderResponse := httptest.NewRecorder()
	router.ServeHTTP(saveProviderResponse, saveProviderRequest)
	if saveProviderResponse.Code != http.StatusOK {
		testingInstance.Fatalf("save DeepSeek provider status=%d body=%q", saveProviderResponse.Code, saveProviderResponse.Body.String())
	}
	secret := generateManagementTenantSecret(testingInstance, router, ownerCookie, tenantIdentifier)

	request := httptest.NewRequest(
		http.MethodPost,
		"/v2?key="+url.QueryEscape(secret)+"&provider="+proxy.ProviderNameDeepSeek,
		bytes.NewBufferString(`{"messages":[{"role":"user","content":"complete before the deadline"}]}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(llmproxycontract.HeaderRequestTimeoutSeconds, "1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusGatewayTimeout || !strings.Contains(response.Body.String(), `"code":"request_timeout"`) {
		testingInstance.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "partial text") {
		testingInstance.Fatalf("timeout leaked partial text: %q", response.Body.String())
	}
	if upstreamRequestCount != 2 {
		testingInstance.Fatalf("upstream requests=%d want=2", upstreamRequestCount)
	}

	type persistedTimeoutUsageEvent struct {
		StatusCode     int    `gorm:"column:status_code"`
		Success        bool   `gorm:"column:success"`
		OutcomeCode    string `gorm:"column:outcome_code"`
		RequestTokens  int    `gorm:"column:request_tokens"`
		ResponseTokens int    `gorm:"column:response_tokens"`
		TotalTokens    int    `gorm:"column:total_tokens"`
	}
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		testingInstance.Fatalf("open usage database: %v", openError)
	}
	var usageEvents []persistedTimeoutUsageEvent
	if queryError := database.
		Table("managed_usage_event_records").
		Select("status_code", "success", "outcome_code", "request_tokens", "response_tokens", "total_tokens").
		Order("id").
		Find(&usageEvents).
		Error; queryError != nil {
		testingInstance.Fatalf("load timeout usage events: %v", queryError)
	}
	expectedUsageEvents := []persistedTimeoutUsageEvent{{
		StatusCode:     http.StatusGatewayTimeout,
		Success:        false,
		OutcomeCode:    "request_timeout",
		RequestTokens:  3,
		ResponseTokens: 5,
		TotalTokens:    8,
	}}
	if !reflect.DeepEqual(usageEvents, expectedUsageEvents) {
		testingInstance.Fatalf("usage events=%+v want=%+v", usageEvents, expectedUsageEvents)
	}
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

func TestManagementAccountUsageFailuresAggregateOwnedTenantsAndBindCursorsToScope(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	ownerCookie := managementSessionCookie(t, "account-failure-owner")
	otherOwnerCookie := managementSessionCookie(t, "account-failure-other-owner")
	account := requestManagementAccount(t, router, ownerCookie)
	firstTenantID := account.Tenants[0].ID
	firstTenantName := account.Tenants[0].Name
	secondTenant := createManagementTenant(t, router, ownerCookie, "Second")
	secondTenantID := secondTenant.Tenant.ID
	otherAccount := requestManagementAccount(t, router, otherOwnerCookie)
	otherTenantID := otherAccount.Tenants[0].ID

	firstSecret := generateManagementTenantSecret(t, router, ownerCookie, firstTenantID)
	secondSecret := generateManagementTenantSecret(t, router, ownerCookie, secondTenantID)
	otherSecret := generateManagementTenantSecret(t, router, otherOwnerCookie, otherTenantID)
	recordInvalidRequest := func(secret string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/?key="+url.QueryEscape(secret)+"&prompt=invalid&max_tokens=0", nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid request status=%d body=%q", response.Code, response.Body.String())
		}
	}
	recordInvalidRequest(firstSecret)
	recordInvalidRequest(secondSecret)
	recordInvalidRequest(firstSecret)
	recordInvalidRequest(otherSecret)

	firstPage := requestManagementAccountUsageFailures(t, router, ownerCookie, "30d", 1, "")
	if firstPage.Interval != "30d" || len(firstPage.Failures) != 1 || firstPage.NextCursor == "" {
		t.Fatalf("first account page=%+v", firstPage)
	}
	if failure := firstPage.Failures[0]; failure.TenantID != firstTenantID || failure.TenantName != firstTenantName || failure.OutcomeCode != "invalid_request" {
		t.Fatalf("first account failure=%+v", failure)
	}
	secondPage := requestManagementAccountUsageFailures(t, router, ownerCookie, "30d", 1, firstPage.NextCursor)
	if len(secondPage.Failures) != 1 || secondPage.NextCursor == "" {
		t.Fatalf("second account page=%+v", secondPage)
	}
	if failure := secondPage.Failures[0]; failure.TenantID != secondTenantID || failure.TenantName != "Second" || failure.OutcomeCode != "invalid_request" {
		t.Fatalf("second account failure=%+v", failure)
	}
	thirdPage := requestManagementAccountUsageFailures(t, router, ownerCookie, "30d", 1, secondPage.NextCursor)
	if len(thirdPage.Failures) != 1 || thirdPage.NextCursor != "" || thirdPage.Failures[0].TenantID != firstTenantID {
		t.Fatalf("third account page=%+v", thirdPage)
	}
	for _, page := range []managementAccountUsageFailuresTestResponse{firstPage, secondPage, thirdPage} {
		for _, failure := range page.Failures {
			if failure.TenantID == otherTenantID {
				t.Fatalf("account failures leaked other owner's tenant: %+v", failure)
			}
		}
	}

	firstTenantPath := managementTenantTestPath(firstTenantID, "")
	tenantPage := requestManagementUsageFailures(t, router, ownerCookie, firstTenantPath, "30d", 1, "")
	if len(tenantPage.Failures) != 1 || tenantPage.NextCursor == "" {
		t.Fatalf("tenant page=%+v", tenantPage)
	}
	for _, crossScopePath := range []string{
		firstTenantPath + "/usage/failures?interval=30d&limit=1&cursor=" + url.QueryEscape(firstPage.NextCursor),
		"/api/management/usage/failures?interval=30d&limit=1&cursor=" + url.QueryEscape(tenantPage.NextCursor),
	} {
		request := httptest.NewRequest(http.MethodGet, crossScopePath, nil)
		request.AddCookie(ownerCookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("cross-scope cursor path=%q status=%d body=%q", crossScopePath, response.Code, response.Body.String())
		}
	}

	invalidQueryRequest := httptest.NewRequest(http.MethodGet, "/api/management/usage/failures?interval=30d&unknown=value", nil)
	invalidQueryRequest.AddCookie(ownerCookie)
	invalidQueryResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidQueryResponse, invalidQueryRequest)
	if invalidQueryResponse.Code != http.StatusBadRequest {
		t.Fatalf("account invalid query status=%d body=%q", invalidQueryResponse.Code, invalidQueryResponse.Body.String())
	}
	unauthorizedRequest := httptest.NewRequest(http.MethodGet, "/api/management/usage/failures?interval=30d", nil)
	unauthorizedResponse := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedResponse, unauthorizedRequest)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("account unauthorized status=%d body=%q", unauthorizedResponse.Code, unauthorizedResponse.Body.String())
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
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"success"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
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

	callerContext, cancelCaller := context.WithCancel(context.Background())
	cancelCaller()
	canceledRequest := httptest.NewRequest(
		http.MethodGet,
		"/?key="+secretQuery+"&prompt=caller-canceled",
		nil,
	).WithContext(callerContext)
	canceledResponse := httptest.NewRecorder()
	router.ServeHTTP(canceledResponse, canceledRequest)
	if canceledResponse.Code != 499 {
		t.Fatalf("canceled status=%d body=%q", canceledResponse.Code, canceledResponse.Body.String())
	}
	canceledPage := requestManagementUsageFailures(t, router, ownerCookie, tenantPath, "30d", 1, "")
	if len(canceledPage.Failures) != 1 ||
		canceledPage.Failures[0].StatusCode != 499 ||
		canceledPage.Failures[0].OutcomeCode != "request_timeout" {
		t.Fatalf("canceled page=%+v", canceledPage)
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
		"request_timeout",
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

func requestManagementAccountUsageFailures(t *testing.T, router http.Handler, sessionCookie *http.Cookie, interval string, limit int, cursor string) managementAccountUsageFailuresTestResponse {
	t.Helper()
	query := url.Values{"interval": []string{interval}, "limit": []string{strconv.Itoa(limit)}}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/management/usage/failures?"+query.Encode(), nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("account failures status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("account failures cache-control=%q want=no-store", response.Header().Get("Cache-Control"))
	}
	var rawPayload struct {
		Failures []map[string]json.RawMessage `json:"failures"`
	}
	if decodeError := json.Unmarshal(response.Body.Bytes(), &rawPayload); decodeError != nil {
		t.Fatalf("decode raw account failures: %v", decodeError)
	}
	for _, rawFailure := range rawPayload.Failures {
		actualFields := make([]string, 0, len(rawFailure))
		for fieldName := range rawFailure {
			actualFields = append(actualFields, fieldName)
		}
		sort.Strings(actualFields)
		expectedFields := []string{"endpoint", "latency_ms", "model", "occurred_at", "outcome_code", "provider", "status_code", "tenant_id", "tenant_name"}
		if !reflect.DeepEqual(actualFields, expectedFields) {
			t.Fatalf("account failure fields=%v want=%v", actualFields, expectedFields)
		}
	}
	var payload managementAccountUsageFailuresTestResponse
	if decodeError := json.Unmarshal(response.Body.Bytes(), &payload); decodeError != nil {
		t.Fatalf("decode account failures: %v", decodeError)
	}
	return payload
}
