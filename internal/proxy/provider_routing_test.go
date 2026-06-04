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
