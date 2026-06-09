package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const (
	testDeepSeekKey    = "sk-deepseek"
	testSiliconFlowKey = "sk-siliconflow"
	testZhipuKey       = "sk-zhipu"
	testGeminiKey      = "sk-gemini"
	testAnthropicKey   = "sk-ant"
	testGrokKey        = "sk-xai"
)

func TestProviderRoutingUsesConfiguredOpenAIURLsForTextAndDictation(t *testing.T) {
	var capturedPaths []string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		capturedPaths = append(capturedPaths, request.URL.Path)
		if authorizationHeader := request.Header.Get("Authorization"); authorizationHeader != "Bearer "+TestAPIKey {
			t.Fatalf("authorization=%q want=%q", authorizationHeader, "Bearer "+TestAPIKey)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/text-api/responses":
			_, _ = responseWriter.Write([]byte(`{"id":"response-id","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"text","text":"openai text ok"}]}]}`))
		case "/dictation-api/transcriptions":
			_, _ = responseWriter.Write([]byte(`{"text":"openai dictation ok"}`))
		default:
			t.Fatalf("unexpected upstream path=%s", request.URL.Path)
		}
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		OpenAIBaseURL:              upstreamServer.URL + "/text-api",
		OpenAITranscriptionsURL:    upstreamServer.URL + "/dictation-api/transcriptions",
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	textRequest := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello", nil)
	textResponse := httptest.NewRecorder()
	router.ServeHTTP(textResponse, textRequest)
	if textResponse.Code != http.StatusOK || strings.TrimSpace(textResponse.Body.String()) != "openai text ok" {
		t.Fatalf("text status=%d body=%q", textResponse.Code, textResponse.Body.String())
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	filePart, createError := writer.CreateFormFile("audio", "recording.webm")
	if createError != nil {
		t.Fatalf("CreateFormFile error: %v", createError)
	}
	if _, writeError := filePart.Write([]byte("audio")); writeError != nil {
		t.Fatalf("write audio: %v", writeError)
	}
	if closeError := writer.Close(); closeError != nil {
		t.Fatalf("close multipart writer: %v", closeError)
	}
	dictationRequest := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret, body)
	dictationRequest.Header.Set("Content-Type", writer.FormDataContentType())
	dictationResponse := httptest.NewRecorder()
	router.ServeHTTP(dictationResponse, dictationRequest)
	if dictationResponse.Code != http.StatusOK || !strings.Contains(dictationResponse.Body.String(), "openai dictation ok") {
		t.Fatalf("dictation status=%d body=%q", dictationResponse.Code, dictationResponse.Body.String())
	}

	if len(capturedPaths) != 2 || capturedPaths[0] != "/text-api/responses" || capturedPaths[1] != "/dictation-api/transcriptions" {
		t.Fatalf("capturedPaths=%v", capturedPaths)
	}
}

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
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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

func TestProviderRoutingUsesConfiguredTextModelCatalog(t *testing.T) {
	const configuredDeepSeekModel = "deepseek-configured-latest"

	baseConfiguration, configurationError := newConfigurationWithCatalogs(t, proxy.Configuration{
		Tenants:   proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey: TestAPIKey,
	})
	if configurationError != nil {
		t.Fatalf("NewConfiguration error: %v", configurationError)
	}
	configuredCatalogs := baseConfiguration.ProviderModels
	deepSeekCatalog := configuredCatalogs[proxy.ProviderNameDeepSeek]
	deepSeekCatalog.Text.Models = append(deepSeekCatalog.Text.Models, proxy.ModelConfiguration{ID: configuredDeepSeekModel})
	configuredCatalogs[proxy.ProviderNameDeepSeek] = deepSeekCatalog

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
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"configured model ok"}}]}`))
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		DeepSeekKey:                testDeepSeekKey,
		DeepSeekBaseURL:            upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
		ProviderModels:             configuredCatalogs,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	queryParameters := url.Values{}
	queryParameters.Set("key", TestSecret)
	queryParameters.Set("prompt", TestPrompt)
	queryParameters.Set("provider", proxy.ProviderNameDeepSeek)
	queryParameters.Set("model", configuredDeepSeekModel)
	request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
	}
	if capturedPayload["model"] != configuredDeepSeekModel {
		t.Fatalf("model=%v want=%s", capturedPayload["model"], configuredDeepSeekModel)
	}
}

func TestProviderRoutingRejectsMissingConfiguredProviderCatalog(t *testing.T) {
	baseConfiguration, configurationError := newConfigurationWithCatalogs(t, proxy.Configuration{
		Tenants:   proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey: TestAPIKey,
	})
	if configurationError != nil {
		t.Fatalf("NewConfiguration error: %v", configurationError)
	}
	configuredCatalogs := baseConfiguration.ProviderModels
	delete(configuredCatalogs, proxy.ProviderNameDeepSeek)

	_, configurationError = newConfigurationWithCatalogs(t, proxy.Configuration{
		Tenants:        proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:      TestAPIKey,
		ProviderModels: configuredCatalogs,
	})
	if configurationError == nil || !strings.Contains(configurationError.Error(), "invalid_model_catalog: provider=deepseek field=providers.deepseek.text") {
		t.Fatalf("error=%v want missing deepseek catalog", configurationError)
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
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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

func TestProviderRoutingSupportsMessagesJSONPostForOpenAICompatibleChat(t *testing.T) {
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
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"chat messages ok"}}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`))
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants: proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{
			Provider:          proxy.ProviderNameOpenAI,
			Model:             proxy.DefaultModel,
			DictationProvider: proxy.ProviderNameOpenAI,
			DictationModel:    proxy.DefaultDictationModel,
			SystemPrompt:      "Tenant system.",
		}),
		OpenAIKey:                  TestAPIKey,
		DeepSeekKey:                testDeepSeekKey,
		DeepSeekBaseURL:            upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	requestBody := bytes.NewBufferString(`{"messages":[{"role":"user","content":"Continue.","order":3},{"role":"user","content":"Hello","order":1},{"role":"assistant","content":"Hi.","order":2}],"model":"` + proxy.ModelNameDeepSeekV4Flash + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/v2?key="+TestSecret+"&provider="+proxy.ProviderNameDeepSeek+"&format=application/json", requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	rawMessages, ok := capturedPayload["messages"].([]any)
	if !ok || len(rawMessages) != 4 {
		t.Fatalf("messages=%v", capturedPayload["messages"])
	}
	firstMessage, ok := rawMessages[0].(map[string]any)
	if !ok || firstMessage["role"] != "system" || firstMessage["content"] != "Tenant system." {
		t.Fatalf("firstMessage=%v", rawMessages[0])
	}
	secondMessage, ok := rawMessages[1].(map[string]any)
	if !ok || secondMessage["role"] != "user" || secondMessage["content"] != "Hello" {
		t.Fatalf("secondMessage=%v", rawMessages[1])
	}
	thirdMessage, ok := rawMessages[2].(map[string]any)
	if !ok || thirdMessage["role"] != "assistant" || thirdMessage["content"] != "Hi." {
		t.Fatalf("thirdMessage=%v", rawMessages[2])
	}
	var response struct {
		Object   string `json:"object"`
		Model    string `json:"model"`
		Request  string `json:"request"`
		Response string `json:"response"`
		Choices  []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
			Order   *int   `json:"order"`
		} `json:"messages"`
	}
	if decodeError := json.Unmarshal(responseRecorder.Body.Bytes(), &response); decodeError != nil {
		t.Fatalf("decode response: %v", decodeError)
	}
	if response.Object != "chat.completion" || response.Model != proxy.ModelNameDeepSeekV4Flash || response.Response != "chat messages ok" {
		t.Fatalf("response=%+v", response)
	}
	if response.Choices[0].Message.Role != "assistant" || response.Choices[0].Message.Content != "chat messages ok" {
		t.Fatalf("choices=%+v", response.Choices)
	}
	expectedRequestDisplay := "user:\nHello\n\nassistant:\nHi.\n\nuser:\nContinue."
	if response.Request != expectedRequestDisplay || len(response.Messages) != 3 || response.Messages[0].Order == nil || *response.Messages[0].Order != 1 {
		t.Fatalf("messages=%+v request=%q", response.Messages, response.Request)
	}
	for _, responseMessage := range response.Messages {
		if responseMessage.Content == "Tenant system." {
			t.Fatalf("response leaked tenant system prompt: %+v", response.Messages)
		}
	}
}

func TestProviderRoutingSurfacesChatCompletionTokenUsage(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"chat usage ok"}}],"usage":{"prompt_tokens":11,"completion_tokens":4}}`))
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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
		if request.URL.Path != "/models/"+proxy.ModelNameGemini25Flash+":generateContent" {
			t.Fatalf("path=%s want=%s", request.URL.Path, "/models/"+proxy.ModelNameGemini25Flash+":generateContent")
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
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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

func TestProviderRoutingSelectsDefaultsByTenantSecret(t *testing.T) {
	const openAITenantSecret = "openai-tenant-secret"
	const geminiTenantSecret = "gemini-tenant-secret"

	var openAIModels []string
	var openAIInputs []string
	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && request.URL.Path == "/" {
			bodyBytes, readError := io.ReadAll(request.Body)
			if readError != nil {
				t.Fatalf("read OpenAI body: %v", readError)
			}
			var payload map[string]any
			if unmarshalError := json.Unmarshal(bodyBytes, &payload); unmarshalError != nil {
				t.Fatalf("unmarshal OpenAI body: %v", unmarshalError)
			}
			openAIModels = append(openAIModels, payload["model"].(string))
			openAIInputs = append(openAIInputs, payload["input"].(string))
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"id":"resp_tenant_default","status":"queued"}`))
			return
		}
		if request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "resp_tenant_default") {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"text","text":"openai tenant ok"}]}]}`))
			return
		}
		http.NotFound(responseWriter, request)
	}))
	defer openAIServer.Close()

	var geminiPath string
	var geminiPayload map[string]any
	geminiServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		geminiPath = request.URL.Path
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read Gemini body: %v", readError)
		}
		if unmarshalError := json.Unmarshal(bodyBytes, &geminiPayload); unmarshalError != nil {
			t.Fatalf("unmarshal Gemini body: %v", unmarshalError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"gemini tenant ok"}]}}]}`))
	}))
	defer geminiServer.Close()

	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(openAIServer.URL)
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants: []proxy.TenantConfiguration{
			{
				ID:     "openai",
				Secret: openAITenantSecret,
				Defaults: proxy.TenantDefaults{
					Provider:          proxy.ProviderNameOpenAI,
					Model:             proxy.ModelNameGPT41,
					DictationProvider: proxy.ProviderNameOpenAI,
					DictationModel:    proxy.DefaultDictationModel,
					SystemPrompt:      "openai tenant system",
				},
			},
			{
				ID:     "gemini",
				Secret: geminiTenantSecret,
				Defaults: proxy.TenantDefaults{
					Provider:          proxy.ProviderNameGemini,
					Model:             proxy.ModelNameGemini35Flash,
					DictationProvider: proxy.ProviderNameOpenAI,
					DictationModel:    proxy.DefaultDictationModel,
					SystemPrompt:      "gemini tenant system",
				},
			},
		},
		OpenAIKey:                  TestAPIKey,
		GeminiKey:                  testGeminiKey,
		GeminiBaseURL:              geminiServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  3,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
		Endpoints:                  endpoints,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	geminiQuery := url.Values{}
	geminiQuery.Set("key", geminiTenantSecret)
	geminiQuery.Set("prompt", "hello gemini default")
	geminiResponse := httptest.NewRecorder()
	router.ServeHTTP(geminiResponse, httptest.NewRequest(http.MethodGet, "/?"+geminiQuery.Encode(), nil))
	if geminiResponse.Code != http.StatusOK {
		t.Fatalf("gemini status=%d body=%s", geminiResponse.Code, geminiResponse.Body.String())
	}
	if strings.TrimSpace(geminiResponse.Body.String()) != "gemini tenant ok" {
		t.Fatalf("gemini body=%q", geminiResponse.Body.String())
	}
	if geminiPath != "/models/"+proxy.ModelNameGemini35Flash+":generateContent" {
		t.Fatalf("gemini path=%s", geminiPath)
	}
	if systemInstructionText(geminiPayload["systemInstruction"]) != "gemini tenant system" {
		t.Fatalf("gemini systemInstruction=%v", geminiPayload["systemInstruction"])
	}

	openAIQuery := url.Values{}
	openAIQuery.Set("key", openAITenantSecret)
	openAIQuery.Set("prompt", "hello openai default")
	openAIResponse := httptest.NewRecorder()
	router.ServeHTTP(openAIResponse, httptest.NewRequest(http.MethodGet, "/?"+openAIQuery.Encode(), nil))
	if openAIResponse.Code != http.StatusOK {
		t.Fatalf("openai status=%d body=%s", openAIResponse.Code, openAIResponse.Body.String())
	}

	overrideQuery := url.Values{}
	overrideQuery.Set("key", geminiTenantSecret)
	overrideQuery.Set("prompt", "hello override")
	overrideQuery.Set("provider", proxy.ProviderNameOpenAI)
	overrideQuery.Set("model", proxy.ModelNameGPT41)
	overrideResponse := httptest.NewRecorder()
	router.ServeHTTP(overrideResponse, httptest.NewRequest(http.MethodGet, "/?"+overrideQuery.Encode(), nil))
	if overrideResponse.Code != http.StatusOK {
		t.Fatalf("override status=%d body=%s", overrideResponse.Code, overrideResponse.Body.String())
	}

	if len(openAIModels) != 2 {
		t.Fatalf("openAIModels=%v want two OpenAI calls", openAIModels)
	}
	if openAIModels[0] != proxy.ModelNameGPT41 || openAIModels[1] != proxy.ModelNameGPT41 {
		t.Fatalf("openAIModels=%v", openAIModels)
	}
	if openAIInputs[0] != "openai tenant system\n\nhello openai default" {
		t.Fatalf("openAI default input=%q", openAIInputs[0])
	}
	if openAIInputs[1] != "gemini tenant system\n\nhello override" {
		t.Fatalf("override input=%q", openAIInputs[1])
	}
}

func systemInstructionText(rawSystemInstruction any) string {
	systemInstruction, ok := rawSystemInstruction.(map[string]any)
	if !ok {
		return ""
	}
	return geminiContentText(systemInstruction)
}

func geminiContentText(content map[string]any) string {
	parts, ok := content["parts"].([]any)
	if !ok || len(parts) == 0 {
		return ""
	}
	firstPart, ok := parts[0].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := firstPart["text"].(string)
	return text
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
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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

func TestProviderRoutingSupportsMessagesJSONPostForGemini(t *testing.T) {
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
		_, _ = responseWriter.Write([]byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"gemini messages ok"}]}}]}`))
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		GeminiKey:                  testGeminiKey,
		GeminiBaseURL:              upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	requestBody := bytes.NewBufferString(`{"messages":[{"role":"user","content":"Continue.","order":4},{"role":"assistant","content":"Hi.","order":3},{"role":"system","content":"Gemini system","order":1},{"role":"user","content":"Hello","order":2}],"model":"` + proxy.ModelNameGemini25Flash + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/v2?key="+TestSecret+"&provider="+proxy.ProviderNameGemini, requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if systemInstructionText(capturedPayload["systemInstruction"]) != "Gemini system" {
		t.Fatalf("systemInstruction=%v", capturedPayload["systemInstruction"])
	}
	contents, ok := capturedPayload["contents"].([]any)
	if !ok || len(contents) != 3 {
		t.Fatalf("contents=%v", capturedPayload["contents"])
	}
	firstContent, ok := contents[0].(map[string]any)
	if !ok || firstContent["role"] != "user" || geminiContentText(firstContent) != "Hello" {
		t.Fatalf("firstContent=%v", contents[0])
	}
	secondContent, ok := contents[1].(map[string]any)
	if !ok || secondContent["role"] != "model" || geminiContentText(secondContent) != "Hi." {
		t.Fatalf("secondContent=%v", contents[1])
	}
}

func TestProviderRoutingSupportsAnthropicMessages(t *testing.T) {
	var capturedPayload map[string]any
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method=%s want=%s", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/v1/messages" {
			t.Fatalf("path=%s want=%s", request.URL.Path, "/v1/messages")
		}
		if apiKeyHeader := request.Header.Get("x-api-key"); apiKeyHeader != testAnthropicKey {
			t.Fatalf("x-api-key=%q want=%q", apiKeyHeader, testAnthropicKey)
		}
		if versionHeader := request.Header.Get("anthropic-version"); versionHeader != "2023-06-01" {
			t.Fatalf("anthropic-version=%q want=%q", versionHeader, "2023-06-01")
		}
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
			t.Fatalf("unmarshal body: %v", unmarshalError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"content":[{"type":"text","text":"claude ok"}],"usage":{"input_tokens":17,"output_tokens":6}}`))
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		AnthropicKey:               testAnthropicKey,
		AnthropicBaseURL:           upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	requestBody := bytes.NewBufferString(`{"messages":[{"role":"user","content":"Continue.","order":4},{"role":"assistant","content":"Hi.","order":3},{"role":"system","content":"Anthropic system","order":1},{"role":"user","content":"Hello","order":2}],"model":"` + proxy.ModelNameClaudeSonnet46 + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/v2?key="+TestSecret+"&provider="+proxy.ProviderNameAnthropic+"&format=application/json", requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if capturedPayload["model"] != proxy.ModelNameClaudeSonnet46 {
		t.Fatalf("model=%v want=%s", capturedPayload["model"], proxy.ModelNameClaudeSonnet46)
	}
	if capturedPayload["max_tokens"] != float64(64000) {
		t.Fatalf("max_tokens=%v payload=%v", capturedPayload["max_tokens"], capturedPayload)
	}
	if capturedPayload["system"] != "Anthropic system" {
		t.Fatalf("system=%v", capturedPayload["system"])
	}
	rawMessages, ok := capturedPayload["messages"].([]any)
	if !ok || len(rawMessages) != 3 {
		t.Fatalf("messages=%v", capturedPayload["messages"])
	}
	firstMessage, ok := rawMessages[0].(map[string]any)
	if !ok || firstMessage["role"] != "user" || firstMessage["content"] != "Hello" {
		t.Fatalf("firstMessage=%v", rawMessages[0])
	}
	secondMessage, ok := rawMessages[1].(map[string]any)
	if !ok || secondMessage["role"] != "assistant" || secondMessage["content"] != "Hi." {
		t.Fatalf("secondMessage=%v", rawMessages[1])
	}
	if responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens) != "17" {
		t.Fatalf("request tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens))
	}
	if responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens) != "6" {
		t.Fatalf("response tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens))
	}
	if responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens) != "23" {
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
		t.Fatalf("decode response: %v", decodeError)
	}
	if response.Response != "claude ok" || response.Usage.RequestTokens != 17 || response.Usage.ResponseTokens != 6 || response.Usage.TotalTokens != 23 {
		t.Fatalf("response=%+v", response)
	}
}

func TestProviderRoutingAnthropicDefaultMaxTokensByModel(t *testing.T) {
	testCases := []struct {
		name              string
		modelIdentifier   string
		expectedMaxTokens float64
	}{
		{name: "opus 4.8", modelIdentifier: proxy.ModelNameClaudeOpus48, expectedMaxTokens: 128000},
		{name: "opus 4.1", modelIdentifier: proxy.ModelNameClaudeOpus41Alias, expectedMaxTokens: 32000},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			var capturedPayload map[string]any
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				bodyBytes, readError := io.ReadAll(request.Body)
				if readError != nil {
					subTest.Fatalf("read body: %v", readError)
				}
				if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
					subTest.Fatalf("unmarshal body: %v", unmarshalError)
				}
				responseWriter.Header().Set("Content-Type", "application/json")
				_, _ = responseWriter.Write([]byte(`{"content":[{"type":"text","text":"claude default max ok"}]}`))
			}))
			defer upstreamServer.Close()

			router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
				Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
				OpenAIKey:                  TestAPIKey,
				AnthropicKey:               testAnthropicKey,
				AnthropicBaseURL:           upstreamServer.URL,
				LogLevel:                   proxy.LogLevelInfo,
				WorkerCount:                1,
				QueueSize:                  1,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			}, zap.NewNop().Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			queryParameters := url.Values{}
			queryParameters.Set("key", TestSecret)
			queryParameters.Set("prompt", TestPrompt)
			queryParameters.Set("provider", proxy.ProviderNameAnthropic)
			queryParameters.Set("model", testCase.modelIdentifier)
			request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusOK {
				subTest.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
			}
			if capturedPayload["max_tokens"] != testCase.expectedMaxTokens {
				subTest.Fatalf("max_tokens=%v want=%v payload=%v", capturedPayload["max_tokens"], testCase.expectedMaxTokens, capturedPayload)
			}
		})
	}
}

func TestProviderRoutingTranslatesMaxTokensForAnthropicMessages(t *testing.T) {
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
		_, _ = responseWriter.Write([]byte(`{"content":[{"type":"text","text":"claude cap ok"}]}`))
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		AnthropicKey:               testAnthropicKey,
		AnthropicBaseURL:           upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	requestBody := bytes.NewBufferString(`{"prompt":"hello claude","model":"` + proxy.ModelNameClaudeSonnet46 + `","max_tokens":444}`)
	request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret+"&provider="+proxy.ProviderNameAnthropic, requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if capturedPayload["max_tokens"] != float64(444) {
		t.Fatalf("max_tokens=%v payload=%v", capturedPayload["max_tokens"], capturedPayload)
	}
}

func TestProviderRoutingSupportsGrokChatCompletions(t *testing.T) {
	var capturedPayload map[string]any
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			t.Fatalf("path=%s want=%s", request.URL.Path, "/chat/completions")
		}
		if authorizationHeader := request.Header.Get("Authorization"); authorizationHeader != "Bearer "+testGrokKey {
			t.Fatalf("authorization=%q want=%q", authorizationHeader, "Bearer "+testGrokKey)
		}
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
			t.Fatalf("unmarshal body: %v", unmarshalError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"grok ok"}}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}}`))
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		GrokKey:                    testGrokKey,
		GrokBaseURL:                upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	queryParameters := url.Values{}
	queryParameters.Set("key", TestSecret)
	queryParameters.Set("prompt", TestPrompt)
	queryParameters.Set("provider", "xai")
	queryParameters.Set("model", proxy.ModelNameGrokCodeFast)
	queryParameters.Set("format", "application/json")
	request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if capturedPayload["model"] != proxy.ModelNameGrokCodeFast {
		t.Fatalf("model=%v want=%s", capturedPayload["model"], proxy.ModelNameGrokCodeFast)
	}
	if responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens) != "13" {
		t.Fatalf("total tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens))
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
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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

func TestProviderRoutingRejectsAnthropicMaxTokensAboveModelLimit(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		t.Fatal("upstream must not be called for max_tokens above Anthropic model limit")
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		AnthropicKey:               testAnthropicKey,
		AnthropicBaseURL:           upstreamServer.URL,
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	queryParameters := url.Values{}
	queryParameters.Set("key", TestSecret)
	queryParameters.Set("prompt", TestPrompt)
	queryParameters.Set("provider", proxy.ProviderNameAnthropic)
	queryParameters.Set("model", proxy.ModelNameClaudeSonnet46)
	queryParameters.Set("max_tokens", "64001")
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
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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

func TestProviderRoutingRejectsAnthropicAndGrokUnsupportedCapabilities(t *testing.T) {
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		AnthropicKey:               testAnthropicKey,
		GrokKey:                    testGrokKey,
		AnthropicBaseURL:           "https://anthropic.invalid",
		GrokBaseURL:                "https://grok.invalid",
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	testCases := []struct {
		name   string
		target string
		method string
	}{
		{name: "anthropic web search", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=anthropic&web_search=1"},
		{name: "grok web search", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=grok&web_search=1"},
		{name: "anthropic dictation", method: http.MethodPost, target: "/dictate?key=" + TestSecret + "&provider=anthropic"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			var request *http.Request
			if testCase.method == http.MethodPost {
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
			if responseRecorder.Code != http.StatusBadRequest {
				subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusBadRequest, responseRecorder.Body.String())
			}
		})
	}
}

func TestProviderRoutingRejectsGeminiMissingCredential(t *testing.T) {
	logger := zap.NewNop()
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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
	if !strings.Contains(responseRecorder.Body.String(), "provider not configured: provider=gemini endpoint=text") {
		t.Fatalf("body=%q want provider not configured detail", responseRecorder.Body.String())
	}
}

func TestProviderRoutingRejectsAnthropicAndGrokMissingCredentials(t *testing.T) {
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		AnthropicBaseURL:           "https://anthropic.invalid",
		GrokBaseURL:                "https://grok.invalid",
		LogLevel:                   proxy.LogLevelInfo,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	testCases := []struct {
		name     string
		provider string
		model    string
	}{
		{name: "anthropic", provider: proxy.ProviderNameAnthropic, model: proxy.ModelNameClaudeSonnet46},
		{name: "grok", provider: proxy.ProviderNameGrok, model: proxy.ModelNameGrok43},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider="+testCase.provider+"&model="+url.QueryEscape(testCase.model), nil)
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusServiceUnavailable {
				subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusServiceUnavailable, responseRecorder.Body.String())
			}
		})
	}
}

func TestProviderRoutingRejectsMissingGeminiDefaultCredential(t *testing.T) {
	logger := zap.NewNop()
	_, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameGemini, Model: proxy.ModelNameGemini35Flash, DictationProvider: proxy.ProviderNameOpenAI, DictationModel: proxy.DefaultDictationModel}),
		OpenAIKey:                  TestAPIKey,
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

func TestProviderRoutingRejectsMissingAnthropicAndGrokDefaultCredentials(t *testing.T) {
	testCases := []struct {
		name          string
		defaults      proxy.TenantDefaults
		expectedError string
	}{
		{
			name:          "anthropic",
			defaults:      proxy.TenantDefaults{Provider: proxy.ProviderNameAnthropic, Model: proxy.ModelNameClaudeSonnet46, DictationProvider: proxy.ProviderNameOpenAI, DictationModel: proxy.DefaultDictationModel},
			expectedError: "provider not configured: provider=anthropic",
		},
		{
			name:          "grok",
			defaults:      proxy.TenantDefaults{Provider: proxy.ProviderNameGrok, Model: proxy.ModelNameGrok43, DictationProvider: proxy.ProviderNameOpenAI, DictationModel: proxy.DefaultDictationModel},
			expectedError: "provider not configured: provider=grok",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			_, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
				Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, testCase.defaults),
				OpenAIKey:                  TestAPIKey,
				LogLevel:                   proxy.LogLevelInfo,
				WorkerCount:                1,
				QueueSize:                  1,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			}, zap.NewNop().Sugar())
			if buildError == nil || !strings.Contains(buildError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want %q", buildError, testCase.expectedError)
			}
		})
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
			router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
				Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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
		router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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
		router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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
		requestContext, cancelRequest := context.WithTimeout(request.Context(), coverageShortRequestTimeout)
		defer cancelRequest()
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request.WithContext(requestContext))
		if responseRecorder.Code != http.StatusGatewayTimeout {
			subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusGatewayTimeout, responseRecorder.Body.String())
		}
	})
}

func TestProviderRoutingMapsAnthropicProviderErrors(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		body       string
		wantStatus int
	}{
		{name: "rate limited", statusCode: http.StatusTooManyRequests, body: `{}`, wantStatus: http.StatusTooManyRequests},
		{name: "provider api failure", statusCode: http.StatusInternalServerError, body: `{}`, wantStatus: http.StatusBadGateway},
		{name: "malformed json", statusCode: http.StatusOK, body: `{`, wantStatus: http.StatusBadGateway},
		{name: "negative usage", statusCode: http.StatusOK, body: `{"content":[{"type":"text","text":"bad usage"}],"usage":{"input_tokens":1,"output_tokens":-1}}`, wantStatus: http.StatusBadGateway},
		{name: "missing text", statusCode: http.StatusOK, body: `{"content":[{"type":"tool_use","text":"not visible"}]}`, wantStatus: http.StatusBadGateway},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				responseWriter.Header().Set("Content-Type", "application/json")
				responseWriter.WriteHeader(testCase.statusCode)
				_, _ = responseWriter.Write([]byte(testCase.body))
			}))
			defer upstreamServer.Close()

			router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
				Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
				OpenAIKey:                  TestAPIKey,
				AnthropicKey:               testAnthropicKey,
				AnthropicBaseURL:           upstreamServer.URL,
				LogLevel:                   proxy.LogLevelInfo,
				WorkerCount:                1,
				QueueSize:                  1,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			}, zap.NewNop().Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=anthropic", nil)
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != testCase.wantStatus {
				subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, testCase.wantStatus, responseRecorder.Body.String())
			}
		})
	}
}

func TestProviderRoutingMapsAnthropicTransportErrors(t *testing.T) {
	t.Run("invalid request URL", func(subTest *testing.T) {
		router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			AnthropicKey:               testAnthropicKey,
			AnthropicBaseURL:           "http://[::1",
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      TestTimeout,
			UpstreamPollTimeoutSeconds: TestTimeout,
		}, zap.NewNop().Sugar())
		if buildError != nil {
			subTest.Fatalf(messageBuildRouterError, buildError)
		}

		request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=anthropic", nil)
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

		router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
			Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:                  TestAPIKey,
			AnthropicKey:               testAnthropicKey,
			AnthropicBaseURL:           "https://anthropic.invalid",
			LogLevel:                   proxy.LogLevelInfo,
			WorkerCount:                1,
			QueueSize:                  1,
			RequestTimeoutSeconds:      1,
			UpstreamPollTimeoutSeconds: TestTimeout,
		}, zap.NewNop().Sugar())
		if buildError != nil {
			subTest.Fatalf(messageBuildRouterError, buildError)
		}

		request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=anthropic", nil)
		requestContext, cancelRequest := context.WithTimeout(request.Context(), coverageShortRequestTimeout)
		defer cancelRequest()
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request.WithContext(requestContext))
		if responseRecorder.Code != http.StatusGatewayTimeout {
			subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusGatewayTimeout, responseRecorder.Body.String())
		}
	})
}

func TestProviderRoutingRejectsUnsupportedWebSearch(t *testing.T) {
	logger := zap.NewNop()
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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
	if !strings.Contains(responseRecorder.Body.String(), "provider not configured: provider=deepseek endpoint=text") {
		t.Fatalf("body=%q want provider not configured detail", responseRecorder.Body.String())
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
				Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.DefaultModel, DictationProvider: proxy.ProviderNameSiliconFlow}),
				OpenAIKey:                  TestAPIKey,
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
				Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.DefaultModel, DictationProvider: proxy.ProviderNameDeepSeek}),
				OpenAIKey:                  TestAPIKey,
				DeepSeekKey:                testDeepSeekKey,
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
				Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.DefaultModel, DictationProvider: proxy.ProviderNameGemini}),
				OpenAIKey:                  TestAPIKey,
				GeminiKey:                  testGeminiKey,
				LogLevel:                   proxy.LogLevelInfo,
				WorkerCount:                1,
				QueueSize:                  1,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			},
			expectedError: "unsupported provider endpoint: provider=gemini endpoint=dictation",
		},
		{
			name: "unsupported_anthropic_dictation",
			configuration: proxy.Configuration{
				Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.DefaultModel, DictationProvider: proxy.ProviderNameAnthropic}),
				OpenAIKey:                  TestAPIKey,
				AnthropicKey:               testAnthropicKey,
				LogLevel:                   proxy.LogLevelInfo,
				WorkerCount:                1,
				QueueSize:                  1,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			},
			expectedError: "unsupported provider endpoint: provider=anthropic endpoint=dictation",
		},
		{
			name: "missing_zhipu_credential",
			configuration: proxy.Configuration{
				Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.DefaultModel, DictationProvider: proxy.ProviderNameZhipu}),
				OpenAIKey:                  TestAPIKey,
				LogLevel:                   proxy.LogLevelInfo,
				WorkerCount:                1,
				QueueSize:                  1,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			},
			expectedError: "provider not configured: provider=zhipu endpoint=dictation",
		},
		{
			name: "missing_grok_credential",
			configuration: proxy.Configuration{
				Tenants:                    proxy.SingleTenantConfigurationsWithDefaults("test", TestSecret, proxy.TenantDefaults{Provider: proxy.ProviderNameOpenAI, Model: proxy.DefaultModel, DictationProvider: proxy.ProviderNameGrok}),
				OpenAIKey:                  TestAPIKey,
				LogLevel:                   proxy.LogLevelInfo,
				WorkerCount:                1,
				QueueSize:                  1,
				RequestTimeoutSeconds:      TestTimeout,
				UpstreamPollTimeoutSeconds: TestTimeout,
			},
			expectedError: "provider not configured: provider=grok endpoint=dictation",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			logger := zap.NewNop()
			_, buildError := buildRouterWithCatalogs(t, testCase.configuration, logger.Sugar())
			if buildError == nil || !strings.Contains(buildError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want contains %q", buildError, testCase.expectedError)
			}
		})
	}
}

func TestProviderRoutingRejectsConflictingJSONModelParameters(t *testing.T) {
	logger := zap.NewNop()
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
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
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                      proxy.SingleTenantConfigurations("test", TestSecret),
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

func TestProviderRoutingSupportsZhipuAndGrokDictation(t *testing.T) {
	testCases := []struct {
		name             string
		providerName     string
		apiKey           string
		expectedModel    string
		expectModelField bool
		expectedResponse string
		configuration    func(string) proxy.Configuration
	}{
		{
			name:             "zhipu",
			providerName:     proxy.ProviderNameZhipu,
			apiKey:           testZhipuKey,
			expectedModel:    "glm-asr-2512",
			expectModelField: true,
			expectedResponse: "zhipu dictation ok",
			configuration: func(transcriptionsURL string) proxy.Configuration {
				return proxy.Configuration{
					Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
					OpenAIKey:                  TestAPIKey,
					ZhipuKey:                   testZhipuKey,
					ZhipuTranscriptionsURL:     transcriptionsURL,
					LogLevel:                   proxy.LogLevelInfo,
					WorkerCount:                1,
					QueueSize:                  1,
					RequestTimeoutSeconds:      TestTimeout,
					UpstreamPollTimeoutSeconds: TestTimeout,
					MaxInputAudioBytes:         1024 * 1024,
				}
			},
		},
		{
			name:             "grok",
			providerName:     proxy.ProviderNameGrok,
			apiKey:           testGrokKey,
			expectedModel:    "",
			expectModelField: false,
			expectedResponse: "grok dictation ok",
			configuration: func(transcriptionsURL string) proxy.Configuration {
				return proxy.Configuration{
					Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
					OpenAIKey:                  TestAPIKey,
					GrokKey:                    testGrokKey,
					GrokTranscriptionsURL:      transcriptionsURL,
					LogLevel:                   proxy.LogLevelInfo,
					WorkerCount:                1,
					QueueSize:                  1,
					RequestTimeoutSeconds:      TestTimeout,
					UpstreamPollTimeoutSeconds: TestTimeout,
					MaxInputAudioBytes:         1024 * 1024,
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost {
					subTest.Fatalf("method=%s want=%s", request.Method, http.MethodPost)
				}
				if authorizationHeader := request.Header.Get("Authorization"); authorizationHeader != "Bearer "+testCase.apiKey {
					subTest.Fatalf("authorization=%q want=%q", authorizationHeader, "Bearer "+testCase.apiKey)
				}
				if parseError := request.ParseMultipartForm(1024 * 1024); parseError != nil {
					subTest.Fatalf("ParseMultipartForm error: %v", parseError)
				}
				modelValues, hasModelField := request.MultipartForm.Value["model"]
				if hasModelField != testCase.expectModelField {
					subTest.Fatalf("model field present=%t want=%t", hasModelField, testCase.expectModelField)
				}
				if testCase.expectModelField && (len(modelValues) != 1 || modelValues[0] != testCase.expectedModel) {
					subTest.Fatalf("model values=%v want=[%s]", modelValues, testCase.expectedModel)
				}
				if _, _, fileError := request.FormFile("file"); fileError != nil {
					subTest.Fatalf("FormFile(file) error: %v", fileError)
				}
				responseWriter.Header().Set("Content-Type", "application/json")
				_, _ = responseWriter.Write([]byte(`{"text":"` + testCase.expectedResponse + `"}`))
			}))
			defer upstreamServer.Close()

			router, buildError := buildRouterWithCatalogs(t, testCase.configuration(upstreamServer.URL), zap.NewNop().Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			filePart, createError := writer.CreateFormFile("audio", "recording.webm")
			if createError != nil {
				subTest.Fatalf("CreateFormFile error: %v", createError)
			}
			if _, copyError := io.Copy(filePart, strings.NewReader(testAudioPayload)); copyError != nil {
				subTest.Fatalf("Copy error: %v", copyError)
			}
			if closeError := writer.Close(); closeError != nil {
				subTest.Fatalf("Close writer error: %v", closeError)
			}
			request := httptest.NewRequest(http.MethodPost, "/dictate?key="+TestSecret+"&provider="+testCase.providerName, body)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusOK {
				subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
			}
			if responseText := decodeTextResponse(subTest, responseRecorder.Body.Bytes()); responseText != testCase.expectedResponse {
				subTest.Fatalf("text=%q want=%q", responseText, testCase.expectedResponse)
			}
		})
	}
}
