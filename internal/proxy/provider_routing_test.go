package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const (
	testDeepSeekKey    = "sk-deepseek"
	testSiliconFlowKey = "sk-siliconflow"
	testGeminiKey      = "sk-gemini"
)

func TestProviderRoutingSupportsDeepSeekChatCompletions(t *testing.T) {
	var capturedPayload map[string]any
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method=%s want=%s", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/chat/completions" {
			t.Fatalf("path=%s want=%s", request.URL.Path, "/chat/completions")
		}
		if authorizationHeader := request.Header.Get("Authorization"); authorizationHeader != "Bearer "+testDeepSeekKey {
			t.Fatalf("authorization=%q want=%q", authorizationHeader, "Bearer "+testDeepSeekKey)
		}
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
			t.Fatalf("unmarshal body: %v", unmarshalError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"deepseek ok"}}]}`))
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		DeepSeekKey:                testDeepSeekKey,
		DeepSeekBaseURL:            upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	queryParameters := url.Values{}
	queryParameters.Set("key", TestSecret)
	queryParameters.Set("prompt", TestPrompt)
	queryParameters.Set("provider", proxy.ProviderNameDeepSeek)
	queryParameters.Set("model", proxy.ModelNameDeepSeekV4Flash)
	request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
	}
	if strings.TrimSpace(responseRecorder.Body.String()) != "deepseek ok" {
		t.Fatalf("body=%q want=%q", responseRecorder.Body.String(), "deepseek ok")
	}
	if capturedPayload["model"] != proxy.ModelNameDeepSeekV4Flash {
		t.Fatalf("model=%v want=%s", capturedPayload["model"], proxy.ModelNameDeepSeekV4Flash)
	}
	if _, exists := capturedPayload["max_tokens"]; exists {
		t.Fatalf("max_tokens must be omitted by default: %v", capturedPayload)
	}
}

func TestProviderRoutingTranslatesMaxTokensForOpenAICompatibleChat(t *testing.T) {
	var capturedPayload map[string]any
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
			t.Fatalf("unmarshal body: %v", unmarshalError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"deepseek cap ok"}}]}`))
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		DeepSeekKey:                testDeepSeekKey,
		DeepSeekBaseURL:            upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	queryParameters := url.Values{}
	queryParameters.Set("key", TestSecret)
	queryParameters.Set("prompt", TestPrompt)
	queryParameters.Set("provider", proxy.ProviderNameDeepSeek)
	queryParameters.Set("model", proxy.ModelNameDeepSeekV4Flash)
	queryParameters.Set("max_tokens", "444")
	request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
	}
	if capturedPayload["max_tokens"] != float64(444) {
		t.Fatalf("max_tokens=%v payload=%v", capturedPayload["max_tokens"], capturedPayload)
	}
}

func TestProviderRoutingSurfacesChatCompletionTokenUsage(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"chat usage ok"}}],"usage":{"prompt_tokens":11,"completion_tokens":4}}`))
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		DeepSeekKey:                testDeepSeekKey,
		DeepSeekBaseURL:            upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	queryParameters := url.Values{}
	queryParameters.Set("key", TestSecret)
	queryParameters.Set("prompt", TestPrompt)
	queryParameters.Set("provider", proxy.ProviderNameDeepSeek)
	queryParameters.Set("model", proxy.ModelNameDeepSeekV4Flash)
	queryParameters.Set("format", "application/json")
	request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
	}
	if responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens) != "11" {
		t.Fatalf("request tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens))
	}
	if responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens) != "4" {
		t.Fatalf("response tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens))
	}
	if responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens) != "15" {
		t.Fatalf("total tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens))
	}
	var response struct {
		Response string `json:"response"`
		Usage    struct {
			RequestTokens  int `json:"request_tokens"`
			ResponseTokens int `json:"response_tokens"`
			TotalTokens    int `json:"total_tokens"`
		} `json:"usage"`
	}
	if decodeError := json.Unmarshal(responseRecorder.Body.Bytes(), &response); decodeError != nil {
		t.Fatalf("decode json: %v", decodeError)
	}
	if response.Response != "chat usage ok" || response.Usage.RequestTokens != 11 || response.Usage.ResponseTokens != 4 || response.Usage.TotalTokens != 15 {
		t.Fatalf("response=%+v", response)
	}
}

func TestProviderRoutingSupportsGeminiGenerateContent(t *testing.T) {
	var capturedPayload map[string]any
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method=%s want=%s", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/models/"+proxy.ModelNameGemini35Flash+":generateContent" {
			t.Fatalf("path=%s want=%s", request.URL.Path, "/models/"+proxy.ModelNameGemini35Flash+":generateContent")
		}
		if apiKeyHeader := request.Header.Get("x-goog-api-key"); apiKeyHeader != testGeminiKey {
			t.Fatalf("x-goog-api-key=%q want=%q", apiKeyHeader, testGeminiKey)
		}
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
			t.Fatalf("unmarshal body: %v", unmarshalError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"gemini ok"}]}}],"usageMetadata":{"promptTokenCount":13,"candidatesTokenCount":5}}`))
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		GeminiKey:                  testGeminiKey,
		GeminiBaseURL:              upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	queryParameters := url.Values{}
	queryParameters.Set("key", TestSecret)
	queryParameters.Set("prompt", TestPrompt)
	queryParameters.Set("provider", proxy.ProviderNameGemini)
	queryParameters.Set("system_prompt", "system text")
	queryParameters.Set("format", "application/json")
	request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
	}
	if responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens) != "13" {
		t.Fatalf("request tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens))
	}
	if responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens) != "5" {
		t.Fatalf("response tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens))
	}
	if responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens) != "18" {
		t.Fatalf("total tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens))
	}
	var response struct {
		Response string `json:"response"`
		Usage    struct {
			RequestTokens  int `json:"request_tokens"`
			ResponseTokens int `json:"response_tokens"`
			TotalTokens    int `json:"total_tokens"`
		} `json:"usage"`
	}
	if decodeError := json.Unmarshal(responseRecorder.Body.Bytes(), &response); decodeError != nil {
		t.Fatalf("decode json: %v", decodeError)
	}
	if response.Response != "gemini ok" || response.Usage.RequestTokens != 13 || response.Usage.ResponseTokens != 5 || response.Usage.TotalTokens != 18 {
		t.Fatalf("response=%+v", response)
	}
	if _, exists := capturedPayload["generationConfig"]; exists {
		t.Fatalf("generationConfig must be omitted by default: %v", capturedPayload["generationConfig"])
	}
	if systemInstruction, ok := capturedPayload["systemInstruction"].(map[string]any); !ok || systemInstruction["parts"] == nil {
		t.Fatalf("systemInstruction=%v", capturedPayload["systemInstruction"])
	}
	assertGeminiContentsOmitThought(t, capturedPayload["contents"])
	assertGeminiContentOmitsThought(t, capturedPayload["systemInstruction"], "systemInstruction")
}

func TestProviderRoutingSupportsGeminiJSONPost(t *testing.T) {
	var capturedPath string
	var capturedPayload map[string]any
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		capturedPath = request.URL.Path
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
			t.Fatalf("unmarshal body: %v", unmarshalError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"gemini internal thought","thought":true},{"text":"gemini json ok"}]}}]}`))
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		GeminiKey:                  testGeminiKey,
		GeminiBaseURL:              upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	requestBody := bytes.NewBufferString(`{"prompt":"hello json","model":"` + proxy.ModelNameGemini25Pro + `","system_prompt":"system json","max_tokens":222}`)
	request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret+"&provider="+proxy.ProviderNameGemini, requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
	}
	if strings.TrimSpace(responseRecorder.Body.String()) != "gemini json ok" {
		t.Fatalf("body=%q want=%q", responseRecorder.Body.String(), "gemini json ok")
	}
	if capturedPath != "/models/"+proxy.ModelNameGemini25Pro+":generateContent" {
		t.Fatalf("path=%s want=%s", capturedPath, "/models/"+proxy.ModelNameGemini25Pro+":generateContent")
	}
	if systemInstruction, ok := capturedPayload["systemInstruction"].(map[string]any); !ok || systemInstruction["parts"] == nil {
		t.Fatalf("systemInstruction=%v", capturedPayload["systemInstruction"])
	}
	assertGeminiContentsOmitThought(t, capturedPayload["contents"])
	assertGeminiContentOmitsThought(t, capturedPayload["systemInstruction"], "systemInstruction")
	if generationConfig, ok := capturedPayload["generationConfig"].(map[string]any); !ok || generationConfig["maxOutputTokens"] != float64(222) {
		t.Fatalf("generationConfig=%v", capturedPayload["generationConfig"])
	}
}

func assertGeminiContentsOmitThought(t *testing.T, rawContents any) {
	t.Helper()
	contents, ok := rawContents.([]any)
	if !ok {
		t.Fatalf("contents=%v", rawContents)
	}
	for contentIndex, rawContent := range contents {
		assertGeminiContentOmitsThought(t, rawContent, "contents[%d]", contentIndex)
	}
}

func assertGeminiContentOmitsThought(t *testing.T, rawContent any, labelFormat string, labelArguments ...any) {
	t.Helper()
	content, ok := rawContent.(map[string]any)
	if !ok {
		t.Fatalf(labelFormat+"=%v", append(labelArguments, rawContent)...)
	}
	rawParts := content["parts"]
	parts, ok := rawParts.([]any)
	if !ok {
		t.Fatalf(labelFormat+".parts=%v", append(labelArguments, rawParts)...)
	}
	for partIndex, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			t.Fatalf(labelFormat+".parts[%d]=%v", append(labelArguments, partIndex, rawPart)...)
		}
		if _, exists := part["thought"]; exists {
			t.Fatalf(labelFormat+".parts[%d] must omit thought: %v", append(labelArguments, partIndex, part)...)
		}
	}
}

func TestProviderRoutingRejectsGeminiJSONPostMaxTokensAboveModelLimit(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		t.Fatal("upstream must not be called for max_tokens above Gemini model limit")
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		GeminiKey:                  testGeminiKey,
		GeminiBaseURL:              upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	requestBody := bytes.NewBufferString(`{"prompt":"hello json","model":"` + proxy.ModelNameGemini35Flash + `","max_tokens":262144}`)
	request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret+"&provider="+proxy.ProviderNameGemini, requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusBadRequest, responseRecorder.Body.String())
	}
	if !strings.Contains(responseRecorder.Body.String(), "invalid max_tokens parameter") {
		t.Fatalf("body=%q want invalid max_tokens parameter", responseRecorder.Body.String())
	}
}

func TestProviderRoutingRejectsGeminiQueryMaxTokensAboveModelLimit(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		t.Fatal("upstream must not be called for query max_tokens above Gemini model limit")
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		GeminiKey:                  testGeminiKey,
		GeminiBaseURL:              upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	queryParameters := url.Values{}
	queryParameters.Set("key", TestSecret)
	queryParameters.Set("prompt", TestPrompt)
	queryParameters.Set("provider", proxy.ProviderNameGemini)
	queryParameters.Set("model", proxy.ModelNameGemini35Flash)
	queryParameters.Set("max_tokens", "262144")
	request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusBadRequest, responseRecorder.Body.String())
	}
	if !strings.Contains(responseRecorder.Body.String(), "invalid max_tokens parameter") {
		t.Fatalf("body=%q want invalid max_tokens parameter", responseRecorder.Body.String())
	}
}

func TestProviderRoutingRejectsGeminiUnsupportedAndInvalidRequests(t *testing.T) {
	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		GeminiKey:                  testGeminiKey,
		GeminiBaseURL:              "https://gemini.invalid",
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	testCases := []struct {
		name         string
		method       string
		target       string
		expectedCode int
	}{
		{name: "unknown model", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=gemini&model=unknown", expectedCode: http.StatusBadRequest},
		{name: "unsupported web search", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=gemini&web_search=1", expectedCode: http.StatusBadRequest},
		{name: "unsupported dictation", method: http.MethodPost, target: "/dictate?key=" + TestSecret + "&provider=gemini", expectedCode: http.StatusBadRequest},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			var request *http.Request
			if testCase.target == "/dictate?key="+TestSecret+"&provider=gemini" {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)
				filePart, createError := writer.CreateFormFile("audio", "recording.webm")
				if createError != nil {
					subTest.Fatalf("CreateFormFile error: %v", createError)
				}
				if _, writeError := filePart.Write([]byte(testAudioPayload)); writeError != nil {
					subTest.Fatalf("write audio: %v", writeError)
				}
				if closeError := writer.Close(); closeError != nil {
					subTest.Fatalf("Close writer error: %v", closeError)
				}
				request = httptest.NewRequest(testCase.method, testCase.target, body)
				request.Header.Set("Content-Type", writer.FormDataContentType())
			} else {
				request = httptest.NewRequest(testCase.method, testCase.target, nil)
			}
			responseRecorder := httptest.NewRecorder()
			router.ServeHTTP(responseRecorder, request)
			if responseRecorder.Code != testCase.expectedCode {
				subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, testCase.expectedCode, responseRecorder.Body.String())
			}
		})
	}
}

func TestProviderRoutingRejectsGeminiMissingCredential(t *testing.T) {
	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		GeminiBaseURL:              "https://gemini.invalid",
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=gemini", nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusServiceUnavailable, responseRecorder.Body.String())
	}
}

func TestProviderRoutingRejectsMissingGeminiDefaultCredential(t *testing.T) {
	logger := zap.NewNop()
	_, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		DefaultProvider:            proxy.ProviderNameGemini,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError == nil || !strings.Contains(buildError.Error(), "provider not configured: provider=gemini") {
		t.Fatalf("error=%v want Gemini provider not configured", buildError)
	}
}

func TestProviderRoutingMapsGeminiProviderErrors(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		body       string
		wantStatus int
	}{
		{name: "rate limited", statusCode: http.StatusTooManyRequests, body: `{}`, wantStatus: http.StatusTooManyRequests},
		{name: "provider api failure", statusCode: http.StatusInternalServerError, body: `{}`, wantStatus: http.StatusBadGateway},
		{name: "malformed json", statusCode: http.StatusOK, body: `{`, wantStatus: http.StatusBadGateway},
		{name: "negative usage", statusCode: http.StatusOK, body: `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"bad usage"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":-1}}`, wantStatus: http.StatusBadGateway},
		{name: "missing finish reason", statusCode: http.StatusOK, body: `{"candidates":[{"content":{"parts":[{"text":"unfinished text"}]}}]}`, wantStatus: http.StatusBadGateway},
		{name: "max tokens finish reason", statusCode: http.StatusOK, body: `{"candidates":[{"finishReason":"MAX_TOKENS","content":{"parts":[{"text":"partial text"}]}}]}`, wantStatus: http.StatusBadGateway},
		{name: "missing text", statusCode: http.StatusOK, body: `{"candidates":[{"finishReason":"STOP","content":{"parts":[{}]}}]}`, wantStatus: http.StatusBadGateway},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				responseWriter.Header().Set("Content-Type", "application/json")
				responseWriter.WriteHeader(testCase.statusCode)
				_, _ = responseWriter.Write([]byte(testCase.body))
			}))
			defer upstreamServer.Close()

			logger := zap.NewNop()
			router, buildError := proxy.BuildRouter(proxy.Configuration{
				ServiceSecret:              TestSecret,
				OpenAIKey:                  TestAPIKey,
				GeminiKey:                  testGeminiKey,
				GeminiBaseURL:              upstreamServer.URL,
				LogLevel:                   proxy.LogLevelInfo,
				WorkerCount:                1,
				QueueSize:                  1,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			}, logger.Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=gemini", nil)
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != testCase.wantStatus {
				subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, testCase.wantStatus, responseRecorder.Body.String())
			}
		})
	}
}

func TestProviderRoutingMapsGeminiTransportErrors(t *testing.T) {
	t.Run("invalid request URL", func(subTest *testing.T) {
		logger := zap.NewNop()
		router, buildError := proxy.BuildRouter(proxy.Configuration{
			ServiceSecret:              TestSecret,
			OpenAIKey:                  TestAPIKey,
			GeminiKey:                  testGeminiKey,
			GeminiBaseURL:              "http://[::1",
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      TestTimeout,
			UpstreamPollTimeoutSeconds: TestTimeout,
		}, logger.Sugar())
		if buildError != nil {
			subTest.Fatalf(messageBuildRouterError, buildError)
		}

		request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=gemini", nil)
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusBadGateway, responseRecorder.Body.String())
		}
	})

	t.Run("transport error", func(subTest *testing.T) {
		originalHTTPClient := proxy.HTTPClient
		proxy.HTTPClient = coverageHTTPDoer(func(request *http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		})
		subTest.Cleanup(func() { proxy.HTTPClient = originalHTTPClient })

		logger := zap.NewNop()
		router, buildError := proxy.BuildRouter(proxy.Configuration{
			ServiceSecret:              TestSecret,
			OpenAIKey:                  TestAPIKey,
			GeminiKey:                  testGeminiKey,
			GeminiBaseURL:              "https://gemini.invalid",
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      1,
			UpstreamPollTimeoutSeconds: TestTimeout,
		}, logger.Sugar())
		if buildError != nil {
			subTest.Fatalf(messageBuildRouterError, buildError)
		}

		request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=gemini", nil)
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)
		if responseRecorder.Code != http.StatusGatewayTimeout {
			subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusGatewayTimeout, responseRecorder.Body.String())
		}
	})
}

func TestProviderRoutingRejectsUnsupportedWebSearch(t *testing.T) {
	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		DeepSeekKey:                testDeepSeekKey,
		DeepSeekBaseURL:            "https://deepseek.invalid",
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=deepseek&model="+proxy.ModelNameDeepSeekV4Flash+"&web_search=1", nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusBadRequest, responseRecorder.Body.String())
	}
}

func TestProviderRoutingRejectsMissingProviderCredential(t *testing.T) {
	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		DeepSeekBaseURL:            "https://deepseek.invalid",
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=deepseek&model="+proxy.ModelNameDeepSeekV4Flash, nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusServiceUnavailable, responseRecorder.Body.String())
	}
}

func TestProviderRoutingRejectsInvalidDefaultDictationProvider(t *testing.T) {
	testCases := []struct {
		name          string
		configuration proxy.Configuration
		expectedError string
	}{
		{
			name: "missing_siliconflow_credential",
			configuration: proxy.Configuration{
				ServiceSecret:              TestSecret,
				OpenAIKey:                  TestAPIKey,
				DefaultDictationProvider:   proxy.ProviderNameSiliconFlow,
				LogLevel:                   proxy.LogLevelInfo,
				WorkerCount:                1,
				QueueSize:                  1,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			},
			expectedError: "provider not configured: provider=siliconflow endpoint=dictation",
		},
		{
			name: "unsupported_deepseek_dictation",
			configuration: proxy.Configuration{
				ServiceSecret:              TestSecret,
				OpenAIKey:                  TestAPIKey,
				DeepSeekKey:                testDeepSeekKey,
				DefaultDictationProvider:   proxy.ProviderNameDeepSeek,
				LogLevel:                   proxy.LogLevelInfo,
				WorkerCount:                1,
				QueueSize:                  1,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			},
			expectedError: "unsupported provider endpoint: provider=deepseek endpoint=dictation",
		},
		{
			name: "unsupported_gemini_dictation",
			configuration: proxy.Configuration{
				ServiceSecret:              TestSecret,
				OpenAIKey:                  TestAPIKey,
				GeminiKey:                  testGeminiKey,
				DefaultDictationProvider:   proxy.ProviderNameGemini,
				LogLevel:                   proxy.LogLevelInfo,
				WorkerCount:                1,
				QueueSize:                  1,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			},
			expectedError: "unsupported provider endpoint: provider=gemini endpoint=dictation",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			logger := zap.NewNop()
			_, buildError := proxy.BuildRouter(testCase.configuration, logger.Sugar())
			if buildError == nil || !strings.Contains(buildError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want contains %q", buildError, testCase.expectedError)
			}
		})
	}
}

func TestProviderRoutingRejectsConflictingJSONModelParameters(t *testing.T) {
	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:              TestSecret,
		OpenAIKey:                  TestAPIKey,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	requestBody := bytes.NewBufferString(`{"prompt":"hello","model":"` + proxy.ModelNameGPT4o + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret+"&model="+proxy.ModelNameGPT41, requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusBadRequest, responseRecorder.Body.String())
	}
}

func TestProviderRoutingSupportsSiliconFlowDictation(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method=%s want=%s", request.Method, http.MethodPost)
		}
		if authorizationHeader := request.Header.Get("Authorization"); authorizationHeader != "Bearer "+testSiliconFlowKey {
			t.Fatalf("authorization=%q want=%q", authorizationHeader, "Bearer "+testSiliconFlowKey)
		}
		if parseError := request.ParseMultipartForm(1024 * 1024); parseError != nil {
			t.Fatalf("ParseMultipartForm error: %v", parseError)
		}
		if model := request.FormValue("model"); model != "FunAudioLLM/SenseVoiceSmall" {
			t.Fatalf("model=%q want=%q", model, "FunAudioLLM/SenseVoiceSmall")
		}
		if _, _, fileError := request.FormFile("file"); fileError != nil {
			t.Fatalf("FormFile(file) error: %v", fileError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"text":"siliconflow dictation ok"}`))
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		ServiceSecret:                TestSecret,
		OpenAIKey:                    TestAPIKey,
		SiliconFlowKey:               testSiliconFlowKey,
		SiliconFlowTranscriptionsURL: upstreamServer.URL,
		LogLevel:                     proxy.LogLevelInfo,
		WorkerCount:                  1,
		QueueSize:                    1,
		RequestTimeoutSeconds:        TestTimeout,
		UpstreamPollTimeoutSeconds:   TestTimeout,
		MaxInputAudioBytes:           1024 * 1024,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	filePart, createError := writer.CreateFormFile("audio", "recording.webm")
	if createError != nil {
		t.Fatalf("CreateFormFile error: %v", createError)
	}
	if _, copyError := io.Copy(filePart, strings.NewReader(testAudioPayload)); copyError != nil {
		t.Fatalf("Copy error: %v", copyError)
	}
	if closeError := writer.Close(); closeError != nil {
		t.Fatalf("Close writer error: %v", closeError)
	}
	request := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret+"&provider=siliconflow", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
	}
	if responseText := decodeTextResponse(t, responseRecorder.Body.Bytes()); responseText != "siliconflow dictation ok" {
		t.Fatalf("text=%q want=%q", responseText, "siliconflow dictation ok")
	}
}
