package proxy_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// chatHandlerScenario defines a single test scenario for model validation.
type chatHandlerScenario struct {
	scenarioName       string
	modelIdentifier    string
	expectedStatusCode int
}

func TestRequestLogsExcludeQueryContent(testingInstance *testing.T) {
	const (
		finalResponse                 = `{"status":"completed", "output":[{"type":"message", "role":"assistant", "content":[{"type":"text","text":"ok"}]}]}`
		promptQueryValue              = "prompt-log-sentinel"
		systemPromptQueryValue        = "system-prompt-log-sentinel"
		tenantSecretQueryValue        = "tenant-secret-log-sentinel"
		rejectedProviderKeyQueryValue = "provider-key-log-sentinel"
		invalidWebSearchQueryValue    = "web-search-log-sentinel"
	)

	mockServer := NewSessionMockServer(finalResponse)
	testingInstance.Cleanup(mockServer.Close)
	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(mockServer.URL)
	observedCore, observedLogs := observer.New(zapcore.DebugLevel)
	loggerInstance := zap.New(observedCore)
	testingInstance.Cleanup(func() { _ = loggerInstance.Sync() })
	router, buildError := buildRouterWithCatalogs(testingInstance, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("logging", tenantSecretQueryValue),
		OpenAIKey:             TestAPIKey,
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
		Endpoints:             endpoints,
	}, loggerInstance.Sugar())
	if buildError != nil {
		testingInstance.Fatalf(messageBuildRouterError, buildError)
	}

	requestScenarios := []struct {
		requestPath        string
		expectedStatusCode int
	}{
		{
			requestPath:        fmt.Sprintf("/?prompt=%s&system_prompt=%s&key=%s&api_key=%s", promptQueryValue, systemPromptQueryValue, tenantSecretQueryValue, rejectedProviderKeyQueryValue),
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			requestPath:        fmt.Sprintf("/?prompt=%s&system_prompt=%s&key=%s&web_search=%s", promptQueryValue, systemPromptQueryValue, tenantSecretQueryValue, invalidWebSearchQueryValue),
			expectedStatusCode: http.StatusOK,
		},
	}
	for _, requestScenario := range requestScenarios {
		request := httptest.NewRequest(http.MethodGet, requestScenario.requestPath, nil)
		responseRecorder := httptest.NewRecorder()

		router.ServeHTTP(responseRecorder, request)

		if responseRecorder.Code != requestScenario.expectedStatusCode {
			testingInstance.Fatalf("status=%d want=%d", responseRecorder.Code, requestScenario.expectedStatusCode)
		}
	}
	if observedLogs.FilterField(zap.String(constants.LogFieldPath, "/")).Len() != len(requestScenarios) {
		testingInstance.Fatal("request log does not contain the query-free root path")
	}
	for _, loggedEntry := range observedLogs.All() {
		loggedContent := loggedEntry.Message + fmt.Sprint(loggedEntry.ContextMap())
		for _, sensitiveQueryValue := range []string{promptQueryValue, systemPromptQueryValue, tenantSecretQueryValue, rejectedProviderKeyQueryValue, invalidWebSearchQueryValue} {
			if strings.Contains(loggedContent, sensitiveQueryValue) {
				testingInstance.Fatal("request logs contain query content")
			}
		}
	}
}

// TestChatHandlerValidatesModel verifies model validation and a successful request flow.
func TestChatHandlerValidatesModel(testingInstance *testing.T) {
	const finalResponse = `{"status":"completed", "output":[{"type":"message", "role":"assistant", "content":[{"type":"text","text":"ok"}]}]}`

	testScenarios := []chatHandlerScenario{
		{
			scenarioName:       "unknown model returns bad request",
			modelIdentifier:    "unknown-model",
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			scenarioName:       "known model returns ok",
			modelIdentifier:    proxy.ModelNameGPT4o,
			expectedStatusCode: http.StatusOK,
		},
		{
			scenarioName:       "GPT-5.5 returns ok",
			modelIdentifier:    proxy.ModelNameGPT55,
			expectedStatusCode: http.StatusOK,
		},
		{
			scenarioName:       "GPT-5.5 pro returns ok",
			modelIdentifier:    proxy.ModelNameGPT55Pro,
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, testScenario := range testScenarios {
		testingInstance.Run(testScenario.scenarioName, func(subTestInstance *testing.T) {
			mockServer := NewSessionMockServer(finalResponse)
			defer mockServer.Close()
			router := NewTestRouter(subTestInstance, mockServer.URL)

			requestPath := fmt.Sprintf("/?prompt=%s&model=%s&key=%s", TestPrompt, testScenario.modelIdentifier, TestSecret)
			request := httptest.NewRequest(http.MethodGet, requestPath, nil)
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != testScenario.expectedStatusCode {
				subTestInstance.Fatalf("status=%d want=%d", responseRecorder.Code, testScenario.expectedStatusCode)
			}
		})
	}
}

func TestChatHandlerAcceptsJSONBody(testingInstance *testing.T) {
	const finalResponse = `{"status":"completed", "output":[{"type":"message", "role":"assistant", "content":[{"type":"text","text":"ok"}]}]}`
	const russianPrompt = "\u0431\u043e\u043b\u044c\u0448\u043e\u0439 \u0440\u0443\u0441\u0441\u043a\u0438\u0439 \u0442\u0435\u043a\u0441\u0442"
	const systemPrompt = "optional"

	var capturedPayload map[string]any
	mockServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		if httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/" {
			bodyBytes, readError := io.ReadAll(httpRequest.Body)
			if readError != nil {
				testingInstance.Fatalf("read request body: %v", readError)
			}
			if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
				testingInstance.Fatalf("unmarshal request body: %v", unmarshalError)
			}
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(fmt.Sprintf(`{"id": "%s", "status": "queued"}`, TestJobID)))
			return
		}
		if httpRequest.Method == http.MethodGet && strings.HasSuffix(httpRequest.URL.Path, TestJobID) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(finalResponse))
			return
		}
		http.NotFound(responseWriter, httpRequest)
	}))
	defer mockServer.Close()

	router := NewTestRouter(testingInstance, mockServer.URL)
	requestBody := bytes.NewBufferString(`{"prompt":"` + russianPrompt + `","model":"` + proxy.ModelNameGPT55 + `","web_search":false,"system_prompt":"` + systemPrompt + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret, requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		testingInstance.Fatalf("status=%d want=%d", responseRecorder.Code, http.StatusOK)
	}
	if capturedPayload["model"] != proxy.ModelNameGPT55 {
		testingInstance.Fatalf("model=%v want=%s", capturedPayload["model"], proxy.ModelNameGPT55)
	}
	if capturedPayload["input"] != systemPrompt+"\n\n"+russianPrompt {
		testingInstance.Fatalf("input=%q", capturedPayload["input"])
	}
	if _, found := capturedPayload["tools"]; found {
		testingInstance.Fatalf("tools must be omitted when web_search=false")
	}
}

func TestChatHandlersApplyPublicReasoningEffortOverTenantDefault(testingInstance *testing.T) {
	var capturedPayloads []map[string]any
	mockServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			testingInstance.Fatalf("read request body: %v", readError)
		}
		var payload map[string]any
		if unmarshalError := json.Unmarshal(bodyBytes, &payload); unmarshalError != nil {
			testingInstance.Fatalf("unmarshal request body: %v", unmarshalError)
		}
		capturedPayloads = append(capturedPayloads, payload)
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"id":"public-reasoning","status":"completed","output_text":"reviewed"}`))
	}))
	testingInstance.Cleanup(mockServer.Close)

	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(mockServer.URL)
	defaults := proxy.DefaultTenantDefaults()
	defaults.Model = proxy.ModelNameGPT55
	defaults.ReasoningEffort = "xhigh"
	router, buildError := buildRouterWithCatalogs(testingInstance, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, defaults),
		OpenAIKey:             TestAPIKey,
		LogLevel:              proxy.LogLevelDebug,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
		Endpoints:             endpoints,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		testingInstance.Fatalf(messageBuildRouterError, buildError)
	}

	testCases := []struct {
		name           string
		method         string
		path           string
		body           string
		expectedEffort string
	}{
		{name: "GET query", method: http.MethodGet, path: "/?key=" + TestSecret + "&prompt=review&reasoning_effort=high", expectedEffort: "high"},
		{name: "root JSON", method: http.MethodPost, path: "/?key=" + TestSecret, body: `{"prompt":"review","reasoning_effort":"high"}`, expectedEffort: "high"},
		{name: "v2 JSON", method: http.MethodPost, path: "/v2?key=" + TestSecret, body: `{"messages":[{"role":"user","content":"review"}],"reasoning_effort":"high"}`, expectedEffort: "high"},
		{name: "omitted field", method: http.MethodPost, path: "/v2?key=" + TestSecret, body: `{"messages":[{"role":"user","content":"review"}]}`, expectedEffort: "xhigh"},
	}
	for requestIndex, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
			if testCase.method == http.MethodPost {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "reviewed" {
				subTest.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
			reasoning, hasReasoning := capturedPayloads[requestIndex]["reasoning"].(map[string]any)
			if !hasReasoning || reasoning["effort"] != testCase.expectedEffort {
				subTest.Fatalf("upstream reasoning=%v want=%q", capturedPayloads[requestIndex]["reasoning"], testCase.expectedEffort)
			}
		})
	}
}

func TestChatHandlersRejectInvalidPublicReasoningEffortBeforeUpstreamRequest(testingInstance *testing.T) {
	var upstreamRequests atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		upstreamRequests.Add(1)
		responseWriter.WriteHeader(http.StatusInternalServerError)
	}))
	testingInstance.Cleanup(mockServer.Close)
	router := NewTestRouter(testingInstance, mockServer.URL)

	testCases := []struct {
		name            string
		method          string
		path            string
		body            string
		expectedMessage string
	}{
		{name: "GET query", method: http.MethodGet, path: "/?key=" + TestSecret + "&prompt=review&model=" + proxy.ModelNameGPT41 + "&reasoning_effort=high"},
		{name: "root JSON", method: http.MethodPost, path: "/?key=" + TestSecret, body: `{"prompt":"review","model":"` + proxy.ModelNameGPT41 + `","reasoning_effort":"high"}`},
		{name: "v2 JSON", method: http.MethodPost, path: "/v2?key=" + TestSecret, body: `{"messages":[{"role":"user","content":"review"}],"model":"` + proxy.ModelNameGPT41 + `","reasoning_effort":"high"}`},
		{name: "supported route with unsupported effort", method: http.MethodPost, path: "/v2?key=" + TestSecret, body: `{"messages":[{"role":"user","content":"review"}],"model":"` + proxy.ModelNameGPT55 + `","reasoning_effort":"max"}`},
		{name: "GET blank effort", method: http.MethodGet, path: "/?key=" + TestSecret + "&prompt=review&model=" + proxy.ModelNameGPT55 + "&reasoning_effort="},
		{name: "root JSON blank effort", method: http.MethodPost, path: "/?key=" + TestSecret, body: `{"prompt":"review","model":"` + proxy.ModelNameGPT55 + `","reasoning_effort":""}`},
		{name: "v2 JSON blank effort", method: http.MethodPost, path: "/v2?key=" + TestSecret, body: `{"messages":[{"role":"user","content":"review"}],"model":"` + proxy.ModelNameGPT55 + `","reasoning_effort":""}`},
		{name: "root JSON null effort", method: http.MethodPost, path: "/?key=" + TestSecret, body: `{"prompt":"review","model":"` + proxy.ModelNameGPT41 + `","reasoning_effort":null}`},
		{name: "v2 JSON null effort", method: http.MethodPost, path: "/v2?key=" + TestSecret, body: `{"messages":[{"role":"user","content":"review"}],"model":"` + proxy.ModelNameGPT41 + `","reasoning_effort":null}`},
		{name: "root JSON nonstring effort", method: http.MethodPost, path: "/?key=" + TestSecret, body: `{"prompt":"review","model":"` + proxy.ModelNameGPT41 + `","reasoning_effort":1}`, expectedMessage: "invalid JSON request"},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
			if testCase.method == http.MethodPost {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			expectedMessage := testCase.expectedMessage
			if expectedMessage == "" {
				expectedMessage = "invalid reasoning_effort parameter"
			}
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), expectedMessage) {
				subTest.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
		})
	}
	if upstreamRequests.Load() != 0 {
		testingInstance.Fatalf("upstream requests=%d want=0", upstreamRequests.Load())
	}
}

func TestChatHandlerAcceptsMessagesJSONBodyForOpenAIResponses(testingInstance *testing.T) {
	const finalResponse = `{"status":"completed", "output":[{"type":"message", "role":"assistant", "content":[{"type":"text","text":"ok"}]}]}`

	var capturedPayload map[string]any
	mockServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		if httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/" {
			bodyBytes, readError := io.ReadAll(httpRequest.Body)
			if readError != nil {
				testingInstance.Fatalf("read request body: %v", readError)
			}
			if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
				testingInstance.Fatalf("unmarshal request body: %v", unmarshalError)
			}
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(fmt.Sprintf(`{"id": "%s", "status": "queued"}`, TestJobID)))
			return
		}
		if httpRequest.Method == http.MethodGet && strings.HasSuffix(httpRequest.URL.Path, TestJobID) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(finalResponse))
			return
		}
		http.NotFound(responseWriter, httpRequest)
	}))
	defer mockServer.Close()

	router := NewTestRouter(testingInstance, mockServer.URL)
	requestBody := bytes.NewBufferString(`{"messages":[{"role":"user","content":"Continue.","order":3},{"role":"assistant","content":"Hi.","order":2},{"role":"system","content":"Follow the contract.","order":0},{"role":"user","content":"Hello.","order":1}],"model":"` + proxy.ModelNameGPT55 + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/v2?key="+TestSecret+"&format=application/json", requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		testingInstance.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
	}
	expectedInput := "system:\nFollow the contract.\n\nuser:\nHello.\n\nassistant:\nHi.\n\nuser:\nContinue."
	if capturedPayload["input"] != expectedInput {
		testingInstance.Fatalf("input=%q want=%q", capturedPayload["input"], expectedInput)
	}
	var response struct {
		Request  string `json:"request"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
			Order   *int   `json:"order"`
		} `json:"messages"`
	}
	if decodeError := json.Unmarshal(responseRecorder.Body.Bytes(), &response); decodeError != nil {
		testingInstance.Fatalf("decode response: %v", decodeError)
	}
	if response.Request != expectedInput || len(response.Messages) != 4 || response.Messages[0].Order == nil || *response.Messages[0].Order != 0 {
		testingInstance.Fatalf("response=%+v", response)
	}
}

func TestChatHandlerRejectsIncompleteGPT55JSONBody(testingInstance *testing.T) {
	const incompleteResponseID = "resp_incomplete_gpt55"

	var capturedPayloads []map[string]any
	mockServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		if httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/" {
			bodyBytes, readError := io.ReadAll(httpRequest.Body)
			if readError != nil {
				testingInstance.Fatalf("read request body: %v", readError)
			}
			var capturedPayload map[string]any
			if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
				testingInstance.Fatalf("unmarshal request body: %v", unmarshalError)
			}
			capturedPayloads = append(capturedPayloads, capturedPayload)
			_, _ = responseWriter.Write([]byte(`{"id":"` + incompleteResponseID + `","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial answer"}]}]}`))
			return
		}
		http.NotFound(responseWriter, httpRequest)
	}))
	defer mockServer.Close()

	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(mockServer.URL)
	defaults := proxy.DefaultTenantDefaults()
	defaults.Model = proxy.ModelNameGPT55
	defaults.ReasoningEffort = "high"
	router, buildRouterError := buildRouterWithCatalogs(testingInstance, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, defaults),
		OpenAIKey:             TestAPIKey,
		LogLevel:              proxy.LogLevelDebug,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
		Endpoints:             endpoints,
	}, zap.NewNop().Sugar())
	if buildRouterError != nil {
		testingInstance.Fatalf(messageBuildRouterError, buildRouterError)
	}
	requestBody := bytes.NewBufferString(`{"prompt":"search current model facts","model":"` + proxy.ModelNameGPT55 + `","web_search":true}`)
	request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret, requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadGateway || responseRecorder.Body.String() != proxy.ErrUpstreamIncomplete.Error() {
		testingInstance.Fatalf("status=%d body=%q", responseRecorder.Code, responseRecorder.Body.String())
	}
	if len(capturedPayloads) != 1 {
		testingInstance.Fatalf("payloads=%d want=1", len(capturedPayloads))
	}
	if capturedPayloads[0]["model"] != proxy.ModelNameGPT55 {
		testingInstance.Fatalf("initial model=%v want=%s", capturedPayloads[0]["model"], proxy.ModelNameGPT55)
	}
	if strings.Contains(responseRecorder.Body.String(), "partial answer") {
		testingInstance.Fatalf("partial response leaked: %q", responseRecorder.Body.String())
	}
}

func TestChatHandlerRejectsOversizedJSONBody(testingInstance *testing.T) {
	endpoints := proxy.NewEndpoints()
	logger := zap.NewNop()
	router, buildRouterError := buildRouterWithCatalogs(testingInstance, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:             TestAPIKey,
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
		MaxPromptBytes:        32,
		Endpoints:             endpoints,
	}, logger.Sugar())
	if buildRouterError != nil {
		testingInstance.Fatalf(messageBuildRouterError, buildRouterError)
	}

	requestBody := bytes.NewBufferString(`{"prompt":"this body is intentionally larger than the configured JSON prompt limit"}`)
	request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret, requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusRequestEntityTooLarge {
		testingInstance.Fatalf("status=%d want=%d", responseRecorder.Code, http.StatusRequestEntityTooLarge)
	}
}
