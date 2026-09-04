package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

const (
	testDeepSeekKey    = "sk-deepseek"
	testSiliconFlowKey = "sk-siliconflow"
	testZAIKey         = "sk-zai"
	testGeminiKey      = "sk-gemini"
	testAnthropicKey   = "sk-ant"
	testMetaKey        = "sk-meta"
	testXAIKey         = "sk-xai"
)

func openAIResponsesReasoningEffortCapability() *proxy.ReasoningEffortCapability {
	return &proxy.ReasoningEffortCapability{
		Adapter: "openai_responses",
		Efforts: []string{"minimal", "low", "medium", "high"},
	}
}

func moonshotChatReasoningEffortCapability() *proxy.ReasoningEffortCapability {
	return &proxy.ReasoningEffortCapability{
		Adapter: "moonshot_chat_completions",
		Efforts: []string{"low", "high", "max"},
	}
}

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
		Endpoints: providerEndpointOverrides(
			map[string]string{proxy.ProviderNameOpenAI: upstreamServer.URL + "/text-api"},
			map[string]map[string]string{proxy.ProviderNameOpenAI: {"dictation": upstreamServer.URL + "/dictation-api/transcriptions"}},
		),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
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

func TestProviderRoutingEnumeratesConfiguredTextRouteCapabilities(t *testing.T) {
	providerOrder := []string{
		proxy.ProviderNameOpenAI,
		proxy.ProviderNameDeepSeek,
		proxy.ProviderNameDashScope,
		proxy.ProviderNameMoonshot,
		proxy.ProviderNameMiniMax,
		proxy.ProviderNameSiliconFlow,
		proxy.ProviderNameZAI,
		proxy.ProviderNameGemini,
		proxy.ProviderNameAnthropic,
		proxy.ProviderNameMeta,
		proxy.ProviderNameXAI,
	}
	expectedCapabilities := map[string]struct {
		wireContract       string
		executionLifecycle string
	}{
		proxy.ProviderNameOpenAI:      {wireContract: "openai_responses", executionLifecycle: "pollable_resource"},
		proxy.ProviderNameDeepSeek:    {wireContract: "openai_chat_completions", executionLifecycle: "synchronous_completion"},
		proxy.ProviderNameDashScope:   {wireContract: "openai_chat_completions", executionLifecycle: "synchronous_completion"},
		proxy.ProviderNameMoonshot:    {wireContract: "openai_chat_completions", executionLifecycle: "synchronous_completion"},
		proxy.ProviderNameMiniMax:     {wireContract: "openai_chat_completions", executionLifecycle: "synchronous_completion"},
		proxy.ProviderNameSiliconFlow: {wireContract: "openai_chat_completions", executionLifecycle: "synchronous_completion"},
		proxy.ProviderNameZAI:         {wireContract: "openai_chat_completions", executionLifecycle: "synchronous_completion"},
		proxy.ProviderNameAnthropic:   {wireContract: "anthropic_messages", executionLifecycle: "synchronous_completion"},
		proxy.ProviderNameMeta:        {wireContract: "openai_chat_completions", executionLifecycle: "synchronous_completion"},
		proxy.ProviderNameXAI:         {wireContract: "openai_chat_completions", executionLifecycle: "synchronous_completion"},
	}
	catalogs := testfixtures.ModelCatalog(t)
	type providerRoute struct {
		provider       string
		canonicalModel string
	}
	providerByNativeModel := map[string]providerRoute{}
	for _, providerName := range providerOrder {
		for _, offering := range catalogOfferingsForProvider(catalogs, providerName, proxy.ModelOperationText) {
			if existingRoute, duplicate := providerByNativeModel[offering.ProviderModel]; duplicate {
				t.Fatalf("native model=%s is shared by providers=%s,%s", offering.ProviderModel, existingRoute.provider, providerName)
			}
			providerByNativeModel[offering.ProviderModel] = providerRoute{provider: providerName, canonicalModel: offering.Model}
		}
	}

	pollModel := catalogOfferingsForProvider(catalogs, proxy.ProviderNameOpenAI, proxy.ModelOperationText)[0].Model
	observedPaths := map[string][]string{}
	geminiModelsByInteraction := map[string]string{}
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/responses/capability-poll" {
			routeKey := proxy.ProviderNameOpenAI + "/" + pollModel
			observedPaths[routeKey] = append(observedPaths[routeKey], request.Method+" "+request.URL.Path)
			_, _ = responseWriter.Write([]byte(`{"id":"capability-poll","status":"completed","output_text":"route ok"}`))
			return
		}
		if request.Method == http.MethodDelete && strings.HasPrefix(request.URL.Path, testGeminiInteractionsPath+"/") {
			interactionIdentifier := strings.TrimPrefix(request.URL.Path, testGeminiInteractionsPath+"/")
			modelIdentifier, knownInteraction := geminiModelsByInteraction[interactionIdentifier]
			if !knownInteraction {
				t.Fatalf("unrecognized Gemini interaction=%q", interactionIdentifier)
			}
			routeKey := proxy.ProviderNameGemini + "/" + modelIdentifier
			observedPaths[routeKey] = append(observedPaths[routeKey], request.Method+" "+request.URL.Path)
			writeGeminiInteractionDeleted(t, responseWriter)
			return
		}

		var payload struct {
			Model      string `json:"model"`
			Background bool   `json:"background"`
			Store      bool   `json:"store"`
		}
		_ = json.NewDecoder(request.Body).Decode(&payload)
		nativeModelIdentifier := payload.Model
		route, configured := providerByNativeModel[nativeModelIdentifier]
		if !configured {
			t.Fatalf("unrecognized upstream model=%q path=%s", nativeModelIdentifier, request.URL.Path)
		}
		providerName := route.provider
		modelIdentifier := route.canonicalModel
		routeKey := providerName + "/" + modelIdentifier
		observedPaths[routeKey] = append(observedPaths[routeKey], request.Method+" "+request.URL.Path)

		switch request.URL.Path {
		case "/responses":
			if modelIdentifier == pollModel {
				_, _ = responseWriter.Write([]byte(`{"id":"capability-poll","status":"queued"}`))
				return
			}
			_, _ = responseWriter.Write([]byte(`{"id":"capability-complete","status":"completed","output_text":"route ok"}`))
		case "/chat/completions":
			_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"route ok"},"finish_reason":"stop"}]}`))
		case "/v1/messages":
			_, _ = responseWriter.Write([]byte(`{"id":"capability-message","type":"message","role":"assistant","content":[{"type":"text","text":"route ok"}],"stop_reason":"end_turn"}`))
		case testGeminiInteractionsPath:
			if !payload.Background || !payload.Store {
				t.Fatalf("Gemini model=%s payload=%v", modelIdentifier, payload)
			}
			interactionIdentifier := "capability-gemini-" + modelIdentifier
			geminiModelsByInteraction[interactionIdentifier] = modelIdentifier
			writeGeminiInteractionSnapshot(t, responseWriter, interactionIdentifier, "completed", "route ok", nil)
		default:
			t.Fatalf("unexpected upstream path=%s", request.URL.Path)
		}
	}))
	t.Cleanup(upstreamServer.Close)

	endpoints := proxy.NewEndpoints()
	for _, provider := range []string{
		proxy.ProviderNameOpenAI,
		proxy.ProviderNameDeepSeek,
		proxy.ProviderNameDashScope,
		proxy.ProviderNameMoonshot,
		proxy.ProviderNameMiniMax,
		proxy.ProviderNameSiliconFlow,
		proxy.ProviderNameZAI,
		proxy.ProviderNameGemini,
		proxy.ProviderNameAnthropic,
		proxy.ProviderNameMeta,
		proxy.ProviderNameXAI,
	} {
		endpoints.SetProviderBaseURL(provider, upstreamServer.URL)
	}
	for _, provider := range []string{proxy.ProviderNameOpenAI, proxy.ProviderNameSiliconFlow, proxy.ProviderNameZAI, proxy.ProviderNameXAI} {
		endpoints.SetProviderTransportURL(provider, "dictation", upstreamServer.URL+"/audio/transcriptions")
	}
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
		Endpoints:             endpoints,
		ModelCatalog:          catalogs,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	for _, providerName := range providerOrder {
		for _, offering := range catalogOfferingsForProvider(catalogs, providerName, proxy.ModelOperationText) {
			expected := expectedCapabilities[providerName]
			if providerName == proxy.ProviderNameGemini {
				expected.wireContract = "gemini_interactions"
				expected.executionLifecycle = "pollable_resource"
			}
			if providerName == proxy.ProviderNameXAI && offering.Model == proxy.ModelNameGrok45 {
				expected.wireContract = "openai_responses"
				expected.executionLifecycle = "synchronous_completion"
			}
			if offering.WireContract != expected.wireContract || offering.ExecutionLifecycle != expected.executionLifecycle {
				t.Fatalf("provider=%s model=%s capabilities=%s/%s want=%s/%s", providerName, offering.Model, offering.WireContract, offering.ExecutionLifecycle, expected.wireContract, expected.executionLifecycle)
			}
			queryParameters := url.Values{}
			queryParameters.Set("key", TestSecret)
			queryParameters.Set("prompt", "enumerate route capabilities")
			queryParameters.Set("provider", providerName)
			queryParameters.Set("model", offering.Model)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil))
			if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "route ok" {
				t.Fatalf("provider=%s model=%s status=%d body=%q", providerName, offering.Model, response.Code, response.Body.String())
			}
			routeKey := providerName + "/" + offering.Model
			expectedRequestCount := 1
			if offering.Model == pollModel {
				expectedRequestCount = 2
			}
			if providerName == proxy.ProviderNameGemini && offering.ExecutionLifecycle == "pollable_resource" {
				expectedRequestCount = 2
			}
			if len(observedPaths[routeKey]) != expectedRequestCount {
				t.Fatalf("provider=%s model=%s upstream=%v want requests=%d", providerName, offering.Model, observedPaths[routeKey], expectedRequestCount)
			}
		}
	}
}

func TestProviderRoutingSupportsDeepSeekChatCompletions(t *testing.T) {
	var capturedPayload map[string]any
	requestCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestCount++
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
		if requestCount == 1 {
			_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"deepseek partial "},"finish_reason":"length"}]}`))
			return
		}
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"deepseek ok"},"finish_reason":"stop"}]}`))
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameDeepSeek),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
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
	if strings.TrimSpace(responseRecorder.Body.String()) != "deepseek partial deepseek ok" {
		t.Fatalf("body=%q want=%q", responseRecorder.Body.String(), "deepseek partial deepseek ok")
	}
	if capturedPayload["model"] != proxy.ModelNameDeepSeekV4Flash {
		t.Fatalf("model=%v want=%s", capturedPayload["model"], proxy.ModelNameDeepSeekV4Flash)
	}
	if _, exists := capturedPayload["max_tokens"]; exists {
		t.Fatalf("max_tokens must be omitted by default: %v", capturedPayload)
	}
	messages, messagesOK := capturedPayload["messages"].([]any)
	if !messagesOK || len(messages) != 3 {
		t.Fatalf("continuation messages=%v", capturedPayload["messages"])
	}
}

func TestProviderRoutingExpandsConfiguredContinuationTokenBudget(t *testing.T) {
	testCases := []struct {
		name              string
		initialMaxTokens  string
		expectedMaxTokens float64
	}{
		{name: "caps caller budget at catalog limit", initialMaxTokens: "40000", expectedMaxTokens: 65536},
		{name: "uses catalog limit when caller omits budget", expectedMaxTokens: 65536},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			requestCount := 0
			var continuationPayload map[string]any
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				requestCount++
				if request.Method != http.MethodPost || request.URL.Path != "/chat/completions" {
					subTest.Fatalf("request=%s %s", request.Method, request.URL.Path)
				}
				requestBytes, readError := io.ReadAll(request.Body)
				if readError != nil {
					subTest.Fatalf("read body: %v", readError)
				}
				if requestCount == 1 {
					responseWriter.Header().Set("Content-Type", "application/json")
					_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":""},"finish_reason":"length"}]}`))
					return
				}
				if unmarshalError := json.Unmarshal(requestBytes, &continuationPayload); unmarshalError != nil {
					subTest.Fatalf("unmarshal continuation body: %v", unmarshalError)
				}
				responseWriter.Header().Set("Content-Type", "application/json")
				_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"continued"},"finish_reason":"stop"}]}`))
			}))
			defer upstreamServer.Close()

			router, buildError := buildRouterWithCatalogs(subTest, proxy.Configuration{
				Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameDashScope),
				LogLevel:              proxy.LogLevelInfo,
				WorkerCount:           1,
				QueueSize:             1,
				RequestTimeoutSeconds: TestTimeout,
			}, zap.NewNop().Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			queryParameters := url.Values{}
			queryParameters.Set("key", TestSecret)
			queryParameters.Set("prompt", TestPrompt)
			queryParameters.Set("provider", proxy.ProviderNameDashScope)
			queryParameters.Set("model", proxy.ModelNameDashScopeQwen37Max)
			if testCase.initialMaxTokens != "" {
				queryParameters.Set("max_tokens", testCase.initialMaxTokens)
			}
			responseRecorder := httptest.NewRecorder()
			router.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil))

			if responseRecorder.Code != http.StatusOK || strings.TrimSpace(responseRecorder.Body.String()) != "continued" {
				subTest.Fatalf("status=%d body=%q", responseRecorder.Code, responseRecorder.Body.String())
			}
			if requestCount != 2 {
				subTest.Fatalf("upstream requests=%d want=2", requestCount)
			}
			if continuationPayload["max_tokens"] != testCase.expectedMaxTokens {
				subTest.Fatalf("continuation max_tokens=%v want=%v", continuationPayload["max_tokens"], testCase.expectedMaxTokens)
			}
		})
	}
}

func TestProviderRoutingSupportsCurrentOpenAICompatibleCatalogModels(t *testing.T) {
	testCases := []struct {
		name                string
		provider            string
		model               string
		providerModel       string
		tokenParameterField string
		expectedAPIKey      string
		forbiddenFields     []string
	}{
		{name: "DashScope Qwen Plus", provider: proxy.ProviderNameDashScope, model: proxy.ModelNameDashScopeQwenPlus, tokenParameterField: "max_tokens"},
		{name: "DashScope Qwen 3.7 Max", provider: proxy.ProviderNameDashScope, model: proxy.ModelNameDashScopeQwen37Max, tokenParameterField: "max_tokens", expectedAPIKey: "sk-dashscope"},
		{name: "DashScope Qwen 3.7 Plus", provider: proxy.ProviderNameDashScope, model: proxy.ModelNameDashScopeQwen37Plus, tokenParameterField: "max_tokens", expectedAPIKey: "sk-dashscope"},
		{name: "DashScope Qwen 3.6 Flash", provider: proxy.ProviderNameDashScope, model: proxy.ModelNameDashScopeQwen36Flash, tokenParameterField: "max_tokens", expectedAPIKey: "sk-dashscope"},
		{name: "Moonshot Kimi K2.6", provider: proxy.ProviderNameMoonshot, model: proxy.ModelNameMoonshotKimiK26, tokenParameterField: "max_completion_tokens", forbiddenFields: []string{"thinking", "reasoning_effort"}},
		{name: "Moonshot Kimi K3", provider: proxy.ProviderNameMoonshot, model: proxy.ModelNameMoonshotKimiK3, tokenParameterField: "max_completion_tokens", forbiddenFields: []string{"temperature", "top_p", "n", "presence_penalty", "frequency_penalty", "thinking", "reasoning_effort"}},
		{name: "Moonshot Kimi K2.7 Code", provider: proxy.ProviderNameMoonshot, model: proxy.ModelNameMoonshotKimiK27Code, tokenParameterField: "max_completion_tokens", forbiddenFields: []string{"temperature", "top_p", "n", "presence_penalty", "frequency_penalty", "thinking", "reasoning_effort"}},
		{name: "Moonshot Kimi K2.7 Code Highspeed", provider: proxy.ProviderNameMoonshot, model: proxy.ModelNameMoonshotKimiK27CodeHighSpeed, tokenParameterField: "max_completion_tokens", forbiddenFields: []string{"thinking", "reasoning_effort"}},
		{name: "MiniMax M2.7", provider: proxy.ProviderNameMiniMax, model: proxy.ModelNameMiniMaxM27, providerModel: "MiniMax-M2.7", tokenParameterField: "max_completion_tokens", expectedAPIKey: "sk-minimax", forbiddenFields: []string{"max_tokens"}},
		{name: "MiniMax M2.7 Highspeed", provider: proxy.ProviderNameMiniMax, model: proxy.ModelNameMiniMaxM27HighSpeed, providerModel: "MiniMax-M2.7-highspeed", tokenParameterField: "max_completion_tokens", expectedAPIKey: "sk-minimax", forbiddenFields: []string{"max_tokens"}},
		{name: "MiniMax M2.5", provider: proxy.ProviderNameMiniMax, model: proxy.ModelNameMiniMaxM25, providerModel: "MiniMax-M2.5", tokenParameterField: "max_completion_tokens", expectedAPIKey: "sk-minimax", forbiddenFields: []string{"max_tokens"}},
		{name: "MiniMax M2.5 Highspeed", provider: proxy.ProviderNameMiniMax, model: proxy.ModelNameMiniMaxM25HighSpeed, providerModel: "MiniMax-M2.5-highspeed", tokenParameterField: "max_completion_tokens", expectedAPIKey: "sk-minimax", forbiddenFields: []string{"max_tokens"}},
		{name: "MiniMax M2.1", provider: proxy.ProviderNameMiniMax, model: proxy.ModelNameMiniMaxM21, providerModel: "MiniMax-M2.1", tokenParameterField: "max_completion_tokens", expectedAPIKey: "sk-minimax", forbiddenFields: []string{"max_tokens"}},
		{name: "MiniMax M2.1 Highspeed", provider: proxy.ProviderNameMiniMax, model: proxy.ModelNameMiniMaxM21HighSpeed, providerModel: "MiniMax-M2.1-highspeed", tokenParameterField: "max_completion_tokens", expectedAPIKey: "sk-minimax", forbiddenFields: []string{"max_tokens"}},
		{name: "MiniMax M2", provider: proxy.ProviderNameMiniMax, model: proxy.ModelNameMiniMaxM2, providerModel: "MiniMax-M2", tokenParameterField: "max_completion_tokens", expectedAPIKey: "sk-minimax", forbiddenFields: []string{"max_tokens"}},
		{name: "SiliconFlow DeepSeek R1", provider: proxy.ProviderNameSiliconFlow, model: proxy.ModelNameSiliconFlowDeepSeek, providerModel: "deepseek-ai/DeepSeek-R1", tokenParameterField: "max_tokens", expectedAPIKey: testSiliconFlowKey},
		{name: "ZAI GLM 5.2", provider: proxy.ProviderNameZAI, model: "glm-5.2", tokenParameterField: "max_tokens", forbiddenFields: []string{"thinking", "reasoning_effort"}},
		{name: "Grok 4.20 reasoning", provider: proxy.ProviderNameXAI, model: "grok-4.20-0309-reasoning", tokenParameterField: "max_tokens"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			var capturedPayload map[string]any
			requestCount := 0
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				requestCount++
				if request.Method != http.MethodPost {
					subTest.Fatalf("method=%s want=%s", request.Method, http.MethodPost)
				}
				if request.URL.Path != "/chat/completions" {
					subTest.Fatalf("path=%s want=%s", request.URL.Path, "/chat/completions")
				}
				if testCase.expectedAPIKey != "" && request.Header.Get("Authorization") != "Bearer "+testCase.expectedAPIKey {
					subTest.Fatalf("authorization=%q want=%q", request.Header.Get("Authorization"), "Bearer "+testCase.expectedAPIKey)
				}
				bodyBytes, readError := io.ReadAll(request.Body)
				if readError != nil {
					subTest.Fatalf("read body: %v", readError)
				}
				if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
					subTest.Fatalf("unmarshal body: %v", unmarshalError)
				}
				responseWriter.Header().Set("Content-Type", "application/json")
				if requestCount == 1 {
					_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"catalog partial "},"finish_reason":"length"}]}`))
					return
				}
				_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"current compatible model ok"},"finish_reason":"stop"}]}`))
			}))
			subTest.Cleanup(upstreamServer.Close)

			router, buildError := buildRouterWithCatalogs(subTest, proxy.Configuration{
				Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameDashScope, proxy.ProviderNameMoonshot, proxy.ProviderNameMiniMax, proxy.ProviderNameSiliconFlow, proxy.ProviderNameZAI, proxy.ProviderNameXAI),
				LogLevel:              proxy.LogLevelInfo,
				WorkerCount:           1,
				QueueSize:             1,
				RequestTimeoutSeconds: TestTimeout,
			}, zap.NewNop().Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			queryParameters := url.Values{}
			queryParameters.Set("key", TestSecret)
			queryParameters.Set("prompt", TestPrompt)
			queryParameters.Set("provider", testCase.provider)
			queryParameters.Set("model", testCase.model)
			queryParameters.Set("max_tokens", "321")
			request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
			responseRecorder := httptest.NewRecorder()
			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusOK {
				subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
			}
			if responseRecorder.Body.String() != "catalog partial current compatible model ok" {
				subTest.Fatalf("body=%q", responseRecorder.Body.String())
			}
			if requestCount != 2 {
				subTest.Fatalf("upstream requests=%d want=2", requestCount)
			}
			expectedProviderModel := testCase.providerModel
			if expectedProviderModel == "" {
				expectedProviderModel = testCase.model
			}
			if capturedPayload["model"] != expectedProviderModel {
				subTest.Fatalf("model=%v want=%s", capturedPayload["model"], expectedProviderModel)
			}
			if capturedPayload[testCase.tokenParameterField] != float64(321) {
				subTest.Fatalf("%s=%v payload=%v", testCase.tokenParameterField, capturedPayload[testCase.tokenParameterField], capturedPayload)
			}
			for _, forbiddenField := range testCase.forbiddenFields {
				if _, present := capturedPayload[forbiddenField]; present {
					subTest.Fatalf("payload unexpectedly contained %s: %v", forbiddenField, capturedPayload)
				}
			}
			messages, messagesOK := capturedPayload["messages"].([]any)
			if !messagesOK || len(messages) != 3 {
				subTest.Fatalf("continuation messages=%v", capturedPayload["messages"])
			}
		})
	}
}

func TestProviderRoutingMapsKimiK3ReasoningEffortWithoutExposingReasoningContent(t *testing.T) {
	capturedPayloads := make([]map[string]any, 0, 4)
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
			t.Fatalf("decode upstream request: %v", decodeError)
		}
		capturedPayloads = append(capturedPayloads, payload)
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"reasoning_content":"private reasoning","content":"visible answer"},"finish_reason":"stop"}]}`))
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameMoonshot),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	for _, effort := range []string{"low", "high", "max", ""} {
		queryParameters := url.Values{
			"key":      {TestSecret},
			"prompt":   {TestPrompt},
			"provider": {proxy.ProviderNameMoonshot},
			"model":    {proxy.ModelNameMoonshotKimiK3},
		}
		if effort != "" {
			queryParameters.Set("reasoning_effort", effort)
		}
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil))
		if responseRecorder.Code != http.StatusOK || responseRecorder.Body.String() != "visible answer" {
			t.Fatalf("effort=%q status=%d body=%q", effort, responseRecorder.Code, responseRecorder.Body.String())
		}
		payload := capturedPayloads[len(capturedPayloads)-1]
		mappedEffort, present := payload["reasoning_effort"]
		if effort == "" && present {
			t.Fatalf("omitted reasoning_effort was serialized: %v", payload)
		}
		if effort != "" && mappedEffort != effort {
			t.Fatalf("reasoning_effort=%v want=%s payload=%v", mappedEffort, effort, payload)
		}
		if _, present := payload["thinking"]; present {
			t.Fatalf("Kimi payload includes thinking=%v", payload)
		}
	}

	requestCount := len(capturedPayloads)
	invalidRequest := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=review&provider=moonshot&model=kimi-k3&reasoning_effort=medium", nil)
	invalidResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidResponse, invalidRequest)
	if invalidResponse.Code != http.StatusBadRequest || len(capturedPayloads) != requestCount || !strings.Contains(invalidResponse.Body.String(), "invalid reasoning_effort parameter") {
		t.Fatalf("unsupported effort status=%d upstream=%d body=%s", invalidResponse.Code, len(capturedPayloads)-requestCount, invalidResponse.Body.String())
	}
}

func TestProviderRoutingPreservesKimiReasoningDuringOutputContinuation(t *testing.T) {
	testCases := []struct {
		name  string
		model string
	}{
		{name: "Kimi K3", model: proxy.ModelNameMoonshotKimiK3},
		{name: "Kimi K2.7 Code", model: proxy.ModelNameMoonshotKimiK27Code},
		{name: "Kimi K2.7 Code Highspeed", model: proxy.ModelNameMoonshotKimiK27CodeHighSpeed},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			capturedPayloads := make([]map[string]any, 0, 2)
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				var payload map[string]any
				if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
					subTest.Fatalf("decode upstream request: %v", decodeError)
				}
				capturedPayloads = append(capturedPayloads, payload)
				responseWriter.Header().Set("Content-Type", "application/json")
				if len(capturedPayloads) == 1 {
					_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"reasoning_content":"private first reasoning","content":"visible partial "},"finish_reason":"length"}]}`))
					return
				}
				_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"reasoning_content":"private second reasoning","content":"visible suffix"},"finish_reason":"stop"}]}`))
			}))
			subTest.Cleanup(upstreamServer.Close)

			router, buildError := buildRouterWithCatalogs(subTest, proxy.Configuration{
				Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameMoonshot),
				LogLevel:              proxy.LogLevelInfo,
				WorkerCount:           1,
				QueueSize:             1,
				RequestTimeoutSeconds: TestTimeout,
			}, zap.NewNop().Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			queryParameters := url.Values{
				"key":      {TestSecret},
				"prompt":   {TestPrompt},
				"provider": {proxy.ProviderNameMoonshot},
				"model":    {testCase.model},
			}
			responseRecorder := httptest.NewRecorder()
			router.ServeHTTP(responseRecorder, httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil))

			if responseRecorder.Code != http.StatusOK || responseRecorder.Body.String() != "visible partial visible suffix" {
				subTest.Fatalf("status=%d body=%q", responseRecorder.Code, responseRecorder.Body.String())
			}
			if len(capturedPayloads) != 2 {
				subTest.Fatalf("upstream requests=%d want=2", len(capturedPayloads))
			}
			continuationMessages, messagesOK := capturedPayloads[1]["messages"].([]any)
			if !messagesOK || len(continuationMessages) != 3 {
				subTest.Fatalf("continuation messages=%v", capturedPayloads[1]["messages"])
			}
			assistantMessage, assistantOK := continuationMessages[1].(map[string]any)
			instructionMessage, instructionOK := continuationMessages[2].(map[string]any)
			if !assistantOK || assistantMessage["role"] != "assistant" || assistantMessage["content"] != "visible partial " || assistantMessage["reasoning_content"] != "private first reasoning" {
				subTest.Fatalf("continuation assistant message=%v", continuationMessages[1])
			}
			if !instructionOK || instructionMessage["role"] != "user" || instructionMessage["content"] != "Continue exactly where the previous response stopped. Return only the missing suffix without repeating any completed text." {
				subTest.Fatalf("continuation instruction=%v", continuationMessages[2])
			}
		})
	}
}

func TestProviderRoutingRejectsGLM52MaxTokensAboveModelLimit(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		t.Fatal("upstream must not be called for max_tokens above the GLM-5.2 limit")
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameZAI),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	queryParameters := url.Values{}
	queryParameters.Set("key", TestSecret)
	queryParameters.Set("prompt", TestPrompt)
	queryParameters.Set("provider", proxy.ProviderNameZAI)
	queryParameters.Set("model", "glm-5.2")
	queryParameters.Set("max_tokens", "131073")
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

func TestProviderRoutingRejectsCurrentQwenMaxTokensAboveModelLimit(t *testing.T) {
	for _, model := range []string{
		proxy.ModelNameDashScopeQwen37Max,
		proxy.ModelNameDashScopeQwen37Plus,
		proxy.ModelNameDashScopeQwen36Flash,
	} {
		t.Run(model, func(subTest *testing.T) {
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				subTest.Fatal("upstream must not be called for max_tokens above the Qwen model limit")
			}))
			defer upstreamServer.Close()

			router, buildError := buildRouterWithCatalogs(subTest, proxy.Configuration{
				Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameDashScope),
				LogLevel:              proxy.LogLevelInfo,
				WorkerCount:           1,
				QueueSize:             1,
				RequestTimeoutSeconds: TestTimeout,
			}, zap.NewNop().Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			queryParameters := url.Values{}
			queryParameters.Set("key", TestSecret)
			queryParameters.Set("prompt", TestPrompt)
			queryParameters.Set("provider", proxy.ProviderNameDashScope)
			queryParameters.Set("model", model)
			queryParameters.Set("max_tokens", "65537")
			request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
			responseRecorder := httptest.NewRecorder()
			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusBadRequest || !strings.Contains(responseRecorder.Body.String(), "invalid max_tokens parameter") {
				subTest.Fatalf("status=%d body=%q", responseRecorder.Code, responseRecorder.Body.String())
			}
		})
	}
}

func TestProviderRoutingRejectsCurrentMiniMaxMaxTokensAboveModelLimit(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		t.Fatal("upstream must not be called for max_tokens above the MiniMax M2 limit")
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameMiniMax),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	for _, model := range []string{
		proxy.ModelNameMiniMaxM27,
		proxy.ModelNameMiniMaxM27HighSpeed,
		proxy.ModelNameMiniMaxM25,
		proxy.ModelNameMiniMaxM25HighSpeed,
		proxy.ModelNameMiniMaxM21,
		proxy.ModelNameMiniMaxM21HighSpeed,
		proxy.ModelNameMiniMaxM2,
	} {
		queryParameters := url.Values{}
		queryParameters.Set("key", TestSecret)
		queryParameters.Set("prompt", TestPrompt)
		queryParameters.Set("provider", proxy.ProviderNameMiniMax)
		queryParameters.Set("model", model)
		queryParameters.Set("max_tokens", "204801")
		request := httptest.NewRequest(http.MethodGet, "/?"+queryParameters.Encode(), nil)
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request)

		if responseRecorder.Code != http.StatusBadRequest || !strings.Contains(responseRecorder.Body.String(), "invalid max_tokens parameter") {
			t.Fatalf("model=%s status=%d body=%q", model, responseRecorder.Code, responseRecorder.Body.String())
		}
	}
}

func TestProviderRoutingSupportsMetaMuseSparkAcrossPublicTextEndpoints(t *testing.T) {
	type capturedMetaRequest struct {
		payload map[string]any
	}

	var capturedRequests []capturedMetaRequest
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method=%s want=%s", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/chat/completions" {
			t.Fatalf("path=%s want=%s", request.URL.Path, "/chat/completions")
		}
		if authorizationHeader := request.Header.Get("Authorization"); authorizationHeader != "Bearer "+testMetaKey {
			t.Fatalf("authorization=%q want=%q", authorizationHeader, "Bearer "+testMetaKey)
		}
		if contentType := request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
			t.Fatalf("content-type=%q want application/json", contentType)
		}
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		var payload map[string]any
		if unmarshalError := json.Unmarshal(bodyBytes, &payload); unmarshalError != nil {
			t.Fatalf("unmarshal body: %v", unmarshalError)
		}
		capturedRequests = append(capturedRequests, capturedMetaRequest{payload: payload})
		responseWriter.Header().Set("Content-Type", "application/json")
		if len(capturedRequests)%2 == 1 {
			_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"meta partial "},"finish_reason":"length"}]}`))
			return
		}
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"meta ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameMeta),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	testCases := []struct {
		name           string
		method         string
		target         string
		body           string
		expectedPrompt string
		expectedModel  string
	}{
		{
			name:           "default get",
			method:         http.MethodGet,
			target:         "/?key=" + TestSecret + "&prompt=meta+get&provider=" + proxy.ProviderNameMeta + "&max_tokens=321&format=application/json",
			expectedPrompt: "meta get",
			expectedModel:  proxy.ModelNameMuseSpark11,
		},
		{
			name:           "default compatibility post",
			method:         http.MethodPost,
			target:         "/?key=" + TestSecret + "&provider=" + proxy.ProviderNameMeta + "&format=application/json",
			body:           `{"prompt":"meta post","max_tokens":321}`,
			expectedPrompt: "meta post",
			expectedModel:  proxy.ModelNameMuseSpark11,
		},
		{
			name:           "default v2 post",
			method:         http.MethodPost,
			target:         "/v2?key=" + TestSecret + "&provider=" + proxy.ProviderNameMeta + "&format=application/json",
			body:           `{"messages":[{"role":"user","content":"meta v2"}],"max_tokens":321}`,
			expectedPrompt: "meta v2",
			expectedModel:  proxy.ModelNameMuseSpark11,
		},
		{
			name:           "Muse Spark 1.2 get",
			method:         http.MethodGet,
			target:         "/?key=" + TestSecret + "&prompt=meta+1.2+get&provider=" + proxy.ProviderNameMeta + "&model=" + proxy.ModelNameMuseSpark12 + "&max_tokens=321&format=application/json",
			expectedPrompt: "meta 1.2 get",
			expectedModel:  proxy.ModelNameMuseSpark12,
		},
		{
			name:           "Muse Spark 1.2 compatibility post",
			method:         http.MethodPost,
			target:         "/?key=" + TestSecret + "&provider=" + proxy.ProviderNameMeta + "&format=application/json",
			body:           `{"prompt":"meta 1.2 post","model":"muse-spark-1.2","max_tokens":321}`,
			expectedPrompt: "meta 1.2 post",
			expectedModel:  proxy.ModelNameMuseSpark12,
		},
		{
			name:           "Muse Spark 1.2 v2 post",
			method:         http.MethodPost,
			target:         "/v2?key=" + TestSecret + "&provider=" + proxy.ProviderNameMeta + "&format=application/json",
			body:           `{"messages":[{"role":"user","content":"meta 1.2 v2"}],"model":"muse-spark-1.2","max_tokens":321}`,
			expectedPrompt: "meta 1.2 v2",
			expectedModel:  proxy.ModelNameMuseSpark12,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			request := httptest.NewRequest(testCase.method, testCase.target, strings.NewReader(testCase.body))
			if testCase.method == http.MethodPost {
				request.Header.Set("Content-Type", "application/json")
			}
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusOK {
				subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
			}
			if responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens) != "11" {
				subTest.Fatalf("request tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyRequestTokens))
			}
			if responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens) != "7" {
				subTest.Fatalf("response tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyResponseTokens))
			}
			if responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens) != "18" {
				subTest.Fatalf("total tokens header=%q", responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens))
			}
			var response struct {
				Model    string `json:"model"`
				Response string `json:"response"`
				Usage    struct {
					RequestTokens  int `json:"request_tokens"`
					ResponseTokens int `json:"response_tokens"`
					TotalTokens    int `json:"total_tokens"`
				} `json:"usage"`
			}
			if decodeError := json.Unmarshal(responseRecorder.Body.Bytes(), &response); decodeError != nil {
				subTest.Fatalf("decode response: %v", decodeError)
			}
			if response.Model != testCase.expectedModel || response.Response != "meta partial meta ok" {
				subTest.Fatalf("response=%+v", response)
			}
			if response.Usage.RequestTokens != 11 || response.Usage.ResponseTokens != 7 || response.Usage.TotalTokens != 18 {
				subTest.Fatalf("usage=%+v", response.Usage)
			}
		})
	}

	if len(capturedRequests) != len(testCases)*2 {
		t.Fatalf("captured requests=%d want=%d", len(capturedRequests), len(testCases)*2)
	}
	for testCaseIndex, testCase := range testCases {
		initialRequest := capturedRequests[testCaseIndex*2]
		continuationRequest := capturedRequests[testCaseIndex*2+1]
		for requestIndex, capturedRequest := range []capturedMetaRequest{initialRequest, continuationRequest} {
			if capturedRequest.payload["model"] != testCase.expectedModel {
				t.Fatalf("request %d.%d model=%v want=%s", testCaseIndex, requestIndex, capturedRequest.payload["model"], testCase.expectedModel)
			}
			if capturedRequest.payload["max_completion_tokens"] != float64(321) {
				t.Fatalf("request %d.%d max_completion_tokens=%v", testCaseIndex, requestIndex, capturedRequest.payload["max_completion_tokens"])
			}
			if _, deprecatedFieldPresent := capturedRequest.payload["max_tokens"]; deprecatedFieldPresent {
				t.Fatalf("request %d.%d must not use deprecated Meta max_tokens: %v", testCaseIndex, requestIndex, capturedRequest.payload)
			}
		}
		initialMessages, initialMessagesOK := initialRequest.payload["messages"].([]any)
		if !initialMessagesOK || len(initialMessages) != 1 {
			t.Fatalf("request %d initial messages=%v", testCaseIndex, initialRequest.payload["messages"])
		}
		initialMessage, initialMessageOK := initialMessages[0].(map[string]any)
		if !initialMessageOK || initialMessage["role"] != "user" || initialMessage["content"] != testCase.expectedPrompt {
			t.Fatalf("request %d initial message=%v", testCaseIndex, initialMessages[0])
		}
		continuationMessages, continuationMessagesOK := continuationRequest.payload["messages"].([]any)
		if !continuationMessagesOK || len(continuationMessages) != 3 {
			t.Fatalf("request %d continuation messages=%v", testCaseIndex, continuationRequest.payload["messages"])
		}
		assistantMessage, assistantMessageOK := continuationMessages[1].(map[string]any)
		instructionMessage, instructionMessageOK := continuationMessages[2].(map[string]any)
		instructionContent, instructionContentOK := instructionMessage["content"].(string)
		if !assistantMessageOK || assistantMessage["role"] != "assistant" || assistantMessage["content"] != "meta partial " ||
			!instructionMessageOK || instructionMessage["role"] != "user" || !instructionContentOK || !strings.Contains(instructionContent, "missing suffix") {
			t.Fatalf("request %d continuation transcript=%v", testCaseIndex, continuationMessages)
		}
	}
}

func TestProviderRoutingIncreasesMetaContinuationBudgetAfterNoProgress(t *testing.T) {
	testCases := []struct {
		name            string
		initialBudget   int
		expectedBudgets []int
	}{
		{name: "doubles the caller budget", initialBudget: 512, expectedBudgets: []int{512, 1024}},
		{name: "does not overflow the platform integer", initialBudget: math.MaxInt, expectedBudgets: []int{math.MaxInt, math.MaxInt}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			var capturedBudgets []int
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.URL.Path != "/chat/completions" {
					subTest.Fatalf("request=%s %s", request.Method, request.URL.Path)
				}
				var payload struct {
					MaxCompletionTokens int `json:"max_completion_tokens"`
				}
				if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
					subTest.Fatalf("decode request: %v", decodeError)
				}
				capturedBudgets = append(capturedBudgets, payload.MaxCompletionTokens)
				responseWriter.Header().Set("Content-Type", "application/json")
				if len(capturedBudgets) == 1 {
					_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":""},"finish_reason":"length"}]}`))
					return
				}
				_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"meta completed"},"finish_reason":"stop"}]}`))
			}))
			defer upstreamServer.Close()

			router, buildError := buildRouterWithCatalogs(subTest, proxy.Configuration{
				Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameMeta),
				LogLevel:              proxy.LogLevelInfo,
				WorkerCount:           1,
				QueueSize:             1,
				RequestTimeoutSeconds: TestTimeout,
			}, zap.NewNop().Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			request := httptest.NewRequest(
				http.MethodPost,
				"/v2?key="+TestSecret+"&provider="+proxy.ProviderNameMeta,
				strings.NewReader(fmt.Sprintf(`{"messages":[{"role":"user","content":"complete every record"}],"model":"%s","max_tokens":%d}`, proxy.ModelNameMuseSpark11, testCase.initialBudget)),
			)
			request.Header.Set("Content-Type", "application/json")
			responseRecorder := httptest.NewRecorder()
			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusOK || strings.TrimSpace(responseRecorder.Body.String()) != "meta completed" {
				subTest.Fatalf("status=%d body=%q", responseRecorder.Code, responseRecorder.Body.String())
			}
			if !reflect.DeepEqual(capturedBudgets, testCase.expectedBudgets) {
				subTest.Fatalf("max_completion_tokens sequence=%v want=%v", capturedBudgets, testCase.expectedBudgets)
			}
		})
	}
}

func TestProviderRoutingUsesConfiguredTextModelCatalog(t *testing.T) {
	const (
		configuredDeepSeekModel = "deepseek-configured-latest"
		configuredNativeModel   = "deepseek-native-route"
	)

	baseConfiguration, configurationError := newConfigurationWithCatalogs(t, proxy.Configuration{})
	if configurationError != nil {
		t.Fatalf("NewConfiguration error: %v", configurationError)
	}
	configuredCatalogs := baseConfiguration.ModelCatalog
	deepSeekOfferings := catalogOfferingsForProvider(configuredCatalogs, proxy.ProviderNameDeepSeek, proxy.ModelOperationText)
	configuredOffering := deepSeekOfferings[0]
	configuredModel := configuredCatalogs.Models[catalogExactModelIndex(configuredCatalogs, configuredOffering.Model)]
	configuredModel.ID = configuredDeepSeekModel
	configuredModel.Version = "configured-latest"
	configuredOffering.Model = configuredDeepSeekModel
	configuredOffering.ProviderModel = configuredNativeModel
	configuredOffering.DefaultOperations = nil
	configuredCatalogs.Models = append(configuredCatalogs.Models, configuredModel)
	configuredCatalogs.Offerings = append(configuredCatalogs.Offerings, configuredOffering)
	configuredCatalogs.Prices = append(configuredCatalogs.Prices, proxy.CatalogPriceDescriptor{
		Provider: configuredOffering.Provider, Model: configuredDeepSeekModel, Operation: proxy.ModelOperationText,
		Source: "https://api-docs.deepseek.com/quick_start/pricing", LastVerified: "2026-08-10",
		UnavailableReason: "Exact published pricing has not been imported for this provider offering.",
	})

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
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"configured model ok"},"finish_reason":"stop"}]}`))
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameDeepSeek),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
		ModelCatalog:          configuredCatalogs,
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
	if capturedPayload["model"] != configuredNativeModel {
		t.Fatalf("model=%v want=%s", capturedPayload["model"], configuredNativeModel)
	}
}

func TestProviderRoutingAppliesModelSpecificReasoningEffortCapability(t *testing.T) {
	catalogs := testfixtures.ModelCatalog(t)
	var capturedPayload map[string]any
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/" {
			t.Fatalf("upstream method=%s path=%s", request.Method, request.URL.Path)
		}
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
			t.Fatalf("unmarshal body: %v", unmarshalError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"id":"model-reasoning-effort","status":"completed","output_text":"model capability ok"}`))
	}))
	defer upstreamServer.Close()

	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(upstreamServer.URL)
	defaults := proxy.DefaultTenantDefaults()
	defaults.Model = proxy.ModelNameGPT5
	defaults.ReasoningEffort = "high"
	tenantConfiguration := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenantConfiguration.Defaults = defaults
	router, buildError := buildRouterWithManagedTenant(t, proxy.Configuration{
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
		Endpoints:             endpoints,
		ModelCatalog:          catalogs,
	}, zap.NewNop().Sugar(), tenantConfiguration)
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=model-capability", nil))
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "model capability ok" {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	reasoning, hasReasoning := capturedPayload["reasoning"].(map[string]any)
	if !hasReasoning || reasoning["effort"] != "high" {
		t.Fatalf("reasoning=%v want high", capturedPayload["reasoning"])
	}
}

func TestProviderRoutingRejectsModelReasoningEffortCapabilityOnUnsupportedRoute(t *testing.T) {
	catalogs := testfixtures.ModelCatalog(t)
	catalogs.Offerings[catalogOfferingIndex(catalogs, proxy.ProviderNameOpenAI, proxy.ModelNameGPT4oMini)].ReasoningEffort = openAIResponsesReasoningEffortCapability()

	_, configurationError := newConfigurationWithCatalogs(t, proxy.Configuration{
		ModelCatalog: catalogs,
	})
	if configurationError == nil || !strings.Contains(configurationError.Error(), "invalid_model_catalog") || !strings.Contains(configurationError.Error(), "adapter=openai_responses") {
		t.Fatalf("configuration error=%v want unsupported model capability", configurationError)
	}
}

func TestProviderRoutingRejectsInvalidReasoningEffortCatalogCapabilities(t *testing.T) {
	testCases := []struct {
		name      string
		configure func(proxy.ModelCatalog)
	}{
		{
			name: "model capability has no adapter",
			configure: func(catalogs proxy.ModelCatalog) {
				capability := openAIResponsesReasoningEffortCapability()
				capability.Adapter = ""
				catalogs.Offerings[catalogOfferingIndex(catalogs, proxy.ProviderNameOpenAI, proxy.ModelNameGPT5)].ReasoningEffort = capability
			},
		},
		{
			name: "model capability has unknown adapter",
			configure: func(catalogs proxy.ModelCatalog) {
				capability := openAIResponsesReasoningEffortCapability()
				capability.Adapter = "unsupported_adapter"
				catalogs.Offerings[catalogOfferingIndex(catalogs, proxy.ProviderNameOpenAI, proxy.ModelNameGPT5)].ReasoningEffort = capability
			},
		},
		{
			name: "model capability has an empty option list",
			configure: func(catalogs proxy.ModelCatalog) {
				capability := openAIResponsesReasoningEffortCapability()
				capability.Efforts = []string{}
				catalogs.Offerings[catalogOfferingIndex(catalogs, proxy.ProviderNameOpenAI, proxy.ModelNameGPT5)].ReasoningEffort = capability
			},
		},
		{
			name: "model capability has duplicate options",
			configure: func(catalogs proxy.ModelCatalog) {
				capability := openAIResponsesReasoningEffortCapability()
				capability.Efforts = []string{"minimal", "low", "low"}
				catalogs.Offerings[catalogOfferingIndex(catalogs, proxy.ProviderNameOpenAI, proxy.ModelNameGPT5)].ReasoningEffort = capability
			},
		},
		{
			name: "model capability has unsupported effort",
			configure: func(catalogs proxy.ModelCatalog) {
				capability := openAIResponsesReasoningEffortCapability()
				capability.Efforts = []string{"warp"}
				catalogs.Offerings[catalogOfferingIndex(catalogs, proxy.ProviderNameOpenAI, proxy.ModelNameGPT5)].ReasoningEffort = capability
			},
		},
		{
			name: "dictation capability is forbidden",
			configure: func(catalogs proxy.ModelCatalog) {
				catalogs.Offerings[catalogOfferingIndex(catalogs, proxy.ProviderNameOpenAI, proxy.DefaultDictationModel)].ReasoningEffort = openAIResponsesReasoningEffortCapability()
			},
		},
		{
			name: "Moonshot adapter is exact to Kimi K3",
			configure: func(catalogs proxy.ModelCatalog) {
				catalogs.Offerings[catalogOfferingIndex(catalogs, proxy.ProviderNameMoonshot, proxy.ModelNameMoonshotKimiK3)].ReasoningEffort = nil
				catalogs.Offerings[catalogOfferingIndex(catalogs, proxy.ProviderNameMoonshot, proxy.ModelNameMoonshotKimiK26)].ReasoningEffort = moonshotChatReasoningEffortCapability()
			},
		},
		{
			name: "Moonshot adapter is provider exact",
			configure: func(catalogs proxy.ModelCatalog) {
				catalogs.Offerings[catalogOfferingIndex(catalogs, proxy.ProviderNameDeepSeek, proxy.ModelNameDeepSeekV4Flash)].ReasoningEffort = moonshotChatReasoningEffortCapability()
			},
		},
		{
			name: "Moonshot adapter rejects unsupported effort",
			configure: func(catalogs proxy.ModelCatalog) {
				capability := moonshotChatReasoningEffortCapability()
				capability.Efforts = []string{"medium"}
				catalogs.Offerings[catalogOfferingIndex(catalogs, proxy.ProviderNameMoonshot, proxy.ModelNameMoonshotKimiK3)].ReasoningEffort = capability
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			catalogs := testfixtures.ModelCatalog(subTest)
			testCase.configure(catalogs)
			_, configurationError := newConfigurationWithCatalogs(subTest, proxy.Configuration{
				ModelCatalog: catalogs,
			})
			if configurationError == nil || !strings.Contains(configurationError.Error(), "invalid_model_catalog") {
				subTest.Fatalf("configuration error=%v want invalid capability", configurationError)
			}
		})
	}
}

func TestProviderCatalogRejectsOfferingWhoseProviderMetadataIsMissing(t *testing.T) {
	baseConfiguration, configurationError := newConfigurationWithCatalogs(t, proxy.Configuration{})
	if configurationError != nil {
		t.Fatalf("NewConfiguration error: %v", configurationError)
	}
	configuredCatalogs := baseConfiguration.ModelCatalog
	for index, provider := range configuredCatalogs.Providers {
		if provider.ID == proxy.ProviderNameDeepSeek {
			configuredCatalogs.Providers = slices.Delete(configuredCatalogs.Providers, index, index+1)
			break
		}
	}

	_, configurationError = newConfigurationWithCatalogs(t, proxy.Configuration{
		ModelCatalog: configuredCatalogs,
	})
	if configurationError == nil || !strings.Contains(configurationError.Error(), "field=providers[") || !strings.Contains(configurationError.Error(), ".label") {
		t.Fatalf("error=%v want missing provider metadata", configurationError)
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
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"deepseek cap ok"},"finish_reason":"stop"}]}`))
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameDeepSeek),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
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
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"chat messages ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`))
	}))
	defer upstreamServer.Close()

	tenantConfiguration := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenantConfiguration.ProviderSystemPrompts = map[string]string{proxy.ProviderNameDeepSeek: "Tenant system."}
	router, buildError := buildRouterWithManagedTenant(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameDeepSeek),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar(), tenantConfiguration)
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
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"chat usage ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":4}}`))
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameDeepSeek),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
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

func TestProviderRoutingSupportsGeminiInteractionsWithBackgroundPolling(t *testing.T) {
	const currentGeminiModel = "gemini-3-flash-preview"
	const interactionIdentifier = "gemini-background-poll"

	var capturedPayload map[string]any
	var capturedRequests []string
	pollCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		capturedRequests = append(capturedRequests, request.Method+" "+request.URL.Path)
		assertGeminiInteractionHeaders(t, request, testGeminiKey)
		switch {
		case request.Method == http.MethodPost && request.URL.Path == testGeminiInteractionsPath:
			capturedPayload = decodeGeminiInteractionRequest(t, request)
			writeGeminiInteractionSnapshot(t, responseWriter, interactionIdentifier, "queued", "", &testGeminiInteractionUsage{Input: 11, Total: 11})
		case request.Method == http.MethodGet && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			pollCount++
			if pollCount == 1 {
				http.Error(responseWriter, "resource is not visible", http.StatusForbidden)
				return
			}
			if pollCount == 2 {
				writeGeminiInteractionSnapshot(t, responseWriter, "", "in_progress", "", &testGeminiInteractionUsage{Input: 12, Total: 12})
				return
			}
			writeGeminiInteractionSnapshot(t, responseWriter, "", "completed", "gemini ok", &testGeminiInteractionUsage{Input: 13, Output: 5, Total: 20})
		case request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/"+interactionIdentifier:
			writeGeminiInteractionDeleted(t, responseWriter)
		default:
			t.Fatalf("unexpected Gemini Interactions request=%s %s", request.Method, request.URL.Path)
		}
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameGemini),
		ProviderCatalog:       testfixtures.ProviderCatalogWithResourceVisibilityInterval(t, proxy.ProviderNameGemini, testVisibilityRetryIntervalMS),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	queryParameters := url.Values{}
	queryParameters.Set("key", TestSecret)
	queryParameters.Set("prompt", TestPrompt)
	queryParameters.Set("provider", proxy.ProviderNameGemini)
	queryParameters.Set("model", currentGeminiModel)
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
	if responseRecorder.Header().Get(testHeaderLLMProxyTotalTokens) != "20" {
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
	if response.Response != "gemini ok" || response.Usage.RequestTokens != 13 || response.Usage.ResponseTokens != 5 || response.Usage.TotalTokens != 20 {
		t.Fatalf("response=%+v", response)
	}
	if capturedPayload["model"] != currentGeminiModel || capturedPayload["background"] != true || capturedPayload["store"] != true {
		t.Fatalf("Gemini Interactions create payload=%v", capturedPayload)
	}
	if _, exists := capturedPayload["generation_config"]; exists {
		t.Fatalf("generation_config must be omitted by default: %v", capturedPayload["generation_config"])
	}
	if capturedPayload["system_instruction"] != "system text" {
		t.Fatalf("system_instruction=%v", capturedPayload["system_instruction"])
	}
	input, inputOK := capturedPayload["input"].([]any)
	if !inputOK || len(input) != 1 || geminiInteractionStepText(t, input[0]) != TestPrompt {
		t.Fatalf("input=%v", capturedPayload["input"])
	}
	expectedRequests := []string{
		http.MethodPost + " " + testGeminiInteractionsPath,
		http.MethodGet + " " + testGeminiInteractionsPath + "/" + interactionIdentifier,
		http.MethodGet + " " + testGeminiInteractionsPath + "/" + interactionIdentifier,
		http.MethodGet + " " + testGeminiInteractionsPath + "/" + interactionIdentifier,
		http.MethodDelete + " " + testGeminiInteractionsPath + "/" + interactionIdentifier,
	}
	if !reflect.DeepEqual(capturedRequests, expectedRequests) {
		t.Fatalf("Gemini Interactions requests=%v want=%v", capturedRequests, expectedRequests)
	}
}

func TestProviderRoutingDoesNotReplayGeminiIncompleteResponse(t *testing.T) {
	var capturedPayloads []map[string]any
	createCount := 0
	deleteCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		assertGeminiInteractionHeaders(t, request, testGeminiKey)
		if request.Method == http.MethodDelete && strings.HasPrefix(request.URL.Path, testGeminiInteractionsPath+"/continuation-") {
			deleteCount++
			writeGeminiInteractionDeleted(t, responseWriter)
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != testGeminiInteractionsPath {
			t.Fatalf("unexpected request=%s %s", request.Method, request.URL.Path)
		}
		createCount++
		payload := decodeGeminiInteractionRequest(t, request)
		capturedPayloads = append(capturedPayloads, payload)
		if createCount != 1 {
			t.Fatalf("Gemini Interactions creates=%d want=1", createCount)
		}
		writeGeminiInteractionSnapshot(t, responseWriter, "continuation-1", "incomplete", "partial answer", nil)
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameGemini),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	request := httptest.NewRequest(
		http.MethodGet,
		"/?key="+TestSecret+"&prompt=hello&provider="+proxy.ProviderNameGemini+"&model="+proxy.ModelNameGemini35Flash+"&max_tokens=1000",
		nil,
	)
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, request)
	if responseRecorder.Code != http.StatusBadGateway || strings.Contains(responseRecorder.Body.String(), "partial answer") {
		t.Fatalf("status=%d body=%q", responseRecorder.Code, responseRecorder.Body.String())
	}
	if len(capturedPayloads) != 1 {
		t.Fatalf("payloads=%d want=1", len(capturedPayloads))
	}
	if deleteCount != 1 {
		t.Fatalf("Gemini Interactions deletes=%d want=1", deleteCount)
	}
	generationConfig, generationConfigOK := capturedPayloads[0]["generation_config"].(map[string]any)
	if !generationConfigOK || generationConfig["max_output_tokens"] != float64(1000) {
		t.Fatalf("generation_config=%v", capturedPayloads[0]["generation_config"])
	}
}

func TestProviderRoutingUsesGeminiDefaultModelForJSONPosts(t *testing.T) {
	var capturedModels []string
	deleteCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		assertGeminiInteractionHeaders(t, request, testGeminiKey)
		if request.Method == http.MethodDelete && strings.HasPrefix(request.URL.Path, testGeminiInteractionsPath+"/default-") {
			deleteCount++
			writeGeminiInteractionDeleted(t, responseWriter)
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != testGeminiInteractionsPath {
			t.Fatalf("unexpected request=%s %s", request.Method, request.URL.Path)
		}
		payload := decodeGeminiInteractionRequest(t, request)
		modelIdentifier, _ := payload["model"].(string)
		if payload["background"] != true || payload["store"] != true {
			t.Fatalf("default Gemini payload=%v", payload)
		}
		capturedModels = append(capturedModels, modelIdentifier)
		writeGeminiInteractionSnapshot(t, responseWriter, fmt.Sprintf("default-%d", len(capturedModels)), "completed", "gemini default ok", nil)
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameGemini),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	for _, testCase := range []struct {
		name string
		path string
		body string
	}{
		{
			name: "post prompt",
			path: "/?key=" + TestSecret + "&provider=" + proxy.ProviderNameGemini,
			body: `{"prompt":"hello gemini default","web_search":false}`,
		},
		{
			name: "post v2 messages",
			path: "/v2?key=" + TestSecret + "&provider=" + proxy.ProviderNameGemini,
			body: `{"messages":[{"role":"user","content":"hello gemini default"}]}`,
		},
	} {
		t.Run(testCase.name, func(subTest *testing.T) {
			request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusOK {
				subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
			}
			if strings.TrimSpace(responseRecorder.Body.String()) != "gemini default ok" {
				subTest.Fatalf("body=%q", responseRecorder.Body.String())
			}
		})
	}
	if !reflect.DeepEqual(capturedModels, []string{proxy.ModelNameGemini35Flash, proxy.ModelNameGemini35Flash}) || deleteCount != 2 {
		t.Fatalf("capturedModels=%v deletes=%d", capturedModels, deleteCount)
	}
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
	geminiDeleteCount := 0
	geminiServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		assertGeminiInteractionHeaders(t, request, testGeminiKey)
		if request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/tenant-default" {
			geminiDeleteCount++
			writeGeminiInteractionDeleted(t, responseWriter)
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != testGeminiInteractionsPath {
			t.Fatalf("unexpected Gemini request=%s %s", request.Method, request.URL.Path)
		}
		geminiPath = request.URL.Path
		geminiPayload = decodeGeminiInteractionRequest(t, request)
		writeGeminiInteractionSnapshot(t, responseWriter, "tenant-default", "completed", "gemini tenant ok", nil)
	}))
	defer geminiServer.Close()

	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(openAIServer.URL)
	endpoints.SetProviderBaseURL(proxy.ProviderNameGemini, geminiServer.URL)
	openAITenant := proxy.StandardManagedTenantTestConfiguration(openAITenantSecret)
	openAITenant.ID = "openai"
	openAITenant.Defaults.SystemPrompt = "openai tenant system"
	geminiTenant := proxy.StandardManagedTenantTestConfiguration(geminiTenantSecret)
	geminiTenant.ID = "gemini"
	geminiTenant.Defaults = proxy.TenantDefaults{
		Provider:          proxy.ProviderNameGemini,
		Model:             proxy.ModelNameGemini35Flash,
		DictationProvider: proxy.ProviderNameOpenAI,
		DictationModel:    proxy.DefaultDictationModel,
		SystemPrompt:      "gemini tenant system",
	}
	geminiTenant.ProviderSystemPrompts = map[string]string{proxy.ProviderNameOpenAI: "gemini tenant openai system"}
	router, buildError := proxy.BuildRouterWithManagedTenantsForTest(t, withModelCatalog(t, proxy.Configuration{
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             3,
		RequestTimeoutSeconds: TestTimeout,
		Endpoints:             endpoints,
	}), zap.NewNop().Sugar(), []proxy.ManagedTenantTestConfiguration{openAITenant, geminiTenant})
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
	if geminiPath != testGeminiInteractionsPath || geminiPayload["model"] != proxy.ModelNameGemini35Flash || geminiDeleteCount != 1 {
		t.Fatalf("gemini path=%s", geminiPath)
	}
	if geminiPayload["system_instruction"] != "gemini tenant system" {
		t.Fatalf("gemini system_instruction=%v", geminiPayload["system_instruction"])
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
	if openAIInputs[1] != "gemini tenant openai system\n\nhello override" {
		t.Fatalf("override input=%q", openAIInputs[1])
	}
}

func TestProviderRoutingSupportsGeminiJSONPost(t *testing.T) {
	var capturedPath string
	var capturedPayload map[string]any
	deleteCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		assertGeminiInteractionHeaders(t, request, testGeminiKey)
		if request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/json-post" {
			deleteCount++
			writeGeminiInteractionDeleted(t, responseWriter)
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != testGeminiInteractionsPath {
			t.Fatalf("unexpected request=%s %s", request.Method, request.URL.Path)
		}
		capturedPath = request.URL.Path
		capturedPayload = decodeGeminiInteractionRequest(t, request)
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"id":"json-post","status":"completed","steps":[{"type":"user_input","content":[{"type":"text","text":"ignored echoed input"}]},{"type":"model_output","content":[{"type":"thought","text":"gemini internal thought"},{"type":"text","text":"gemini json ok"}]}]}`))
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameGemini),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	requestBody := bytes.NewBufferString(`{"prompt":"hello json","model":"` + proxy.ModelNameGemini35Flash + `","system_prompt":"system json","max_tokens":222}`)
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
	if capturedPath != testGeminiInteractionsPath || capturedPayload["model"] != proxy.ModelNameGemini35Flash || deleteCount != 1 || capturedPayload["background"] != true || capturedPayload["store"] != true {
		t.Fatalf("path=%s payload=%v deletes=%d", capturedPath, capturedPayload, deleteCount)
	}
	if capturedPayload["system_instruction"] != "system json" {
		t.Fatalf("system_instruction=%v", capturedPayload["system_instruction"])
	}
	input, inputOK := capturedPayload["input"].([]any)
	if !inputOK || len(input) != 1 || geminiInteractionStepText(t, input[0]) != "hello json" {
		t.Fatalf("input=%v", capturedPayload["input"])
	}
	if generationConfig, ok := capturedPayload["generation_config"].(map[string]any); !ok || generationConfig["max_output_tokens"] != float64(222) {
		t.Fatalf("generation_config=%v", capturedPayload["generation_config"])
	}
}

func TestProviderRoutingSupportsMessagesJSONPostForGemini(t *testing.T) {
	var capturedPayload map[string]any
	deleteCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		assertGeminiInteractionHeaders(t, request, testGeminiKey)
		if request.Method == http.MethodDelete && request.URL.Path == testGeminiInteractionsPath+"/messages-post" {
			deleteCount++
			writeGeminiInteractionDeleted(t, responseWriter)
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != testGeminiInteractionsPath {
			t.Fatalf("unexpected request=%s %s", request.Method, request.URL.Path)
		}
		capturedPayload = decodeGeminiInteractionRequest(t, request)
		writeGeminiInteractionSnapshot(t, responseWriter, "messages-post", "completed", "gemini messages ok", nil)
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameGemini),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	requestBody := bytes.NewBufferString(`{"messages":[{"role":"user","content":"Continue.","order":4},{"role":"system","content":"Gemini system","order":1},{"role":"user","content":"Hello","order":2}],"model":"` + proxy.ModelNameGemini35Flash + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/v2?key="+TestSecret+"&provider="+proxy.ProviderNameGemini, requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if capturedPayload["system_instruction"] != "Gemini system" || deleteCount != 1 || capturedPayload["background"] != true || capturedPayload["store"] != true {
		t.Fatalf("system_instruction=%v deletes=%d", capturedPayload["system_instruction"], deleteCount)
	}
	input, ok := capturedPayload["input"].([]any)
	if !ok || len(input) != 2 {
		t.Fatalf("input=%v", capturedPayload["input"])
	}
	firstStep, ok := input[0].(map[string]any)
	if !ok || firstStep["type"] != testGeminiInteractionUserStep || geminiInteractionStepText(t, firstStep) != "Hello" {
		t.Fatalf("firstStep=%v", input[0])
	}
	secondStep, ok := input[1].(map[string]any)
	if !ok || secondStep["type"] != testGeminiInteractionUserStep || geminiInteractionStepText(t, secondStep) != "Continue." {
		t.Fatalf("secondStep=%v", input[1])
	}
}

func TestProviderRoutingRejectsAssistantHistoryForAllGeminiRoutes(t *testing.T) {
	upstreamRequests := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		upstreamRequests++
		assertGeminiInteractionHeaders(t, request, testGeminiKey)
		if request.Method == http.MethodDelete {
			writeGeminiInteractionDeleted(t, responseWriter)
			return
		}
		_ = decodeGeminiInteractionRequest(t, request)
		writeGeminiInteractionSnapshot(t, responseWriter, "older-prefill", "completed", "continued", nil)
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameGemini),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}
	models := []string{
		proxy.ModelNameGemini35Flash,
		"gemini-3-flash-preview",
	}
	for _, model := range models {
		body := `{"messages":[{"role":"user","content":"Review."},{"role":"assistant","content":"Draft."},{"role":"user","content":"Continue."}],"model":"` + model + `"}`
		request := httptest.NewRequest(http.MethodPost, "/v2?key="+TestSecret+"&provider=gemini", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid messages parameter") {
			t.Fatalf("model=%s status=%d body=%q", model, response.Code, response.Body.String())
		}
	}
	legacyRequest := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret+"&provider=gemini", strings.NewReader(`{"messages":[{"role":"user","content":"Review."},{"role":"assistant","content":"Draft."}],"model":"`+proxy.ModelNameGemini35Flash+`"}`))
	legacyRequest.Header.Set("Content-Type", "application/json")
	legacyResponse := httptest.NewRecorder()
	router.ServeHTTP(legacyResponse, legacyRequest)
	if legacyResponse.Code != http.StatusBadRequest || !strings.Contains(legacyResponse.Body.String(), "invalid messages parameter") {
		t.Fatalf("legacy status=%d body=%q", legacyResponse.Code, legacyResponse.Body.String())
	}
	if upstreamRequests != 0 {
		t.Fatalf("Gemini assistant history reached upstream=%d", upstreamRequests)
	}
	userRequest := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret+"&provider=gemini", strings.NewReader(`{"messages":[{"role":"user","content":"Review."}],"model":"`+proxy.ModelNameGemini35Flash+`"}`))
	userRequest.Header.Set("Content-Type", "application/json")
	userResponse := httptest.NewRecorder()
	router.ServeHTTP(userResponse, userRequest)
	if userResponse.Code != http.StatusOK || strings.TrimSpace(userResponse.Body.String()) != "continued" || upstreamRequests != 2 {
		t.Fatalf("new route status=%d body=%q upstream=%d", userResponse.Code, userResponse.Body.String(), upstreamRequests)
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
		_, _ = responseWriter.Write([]byte(`{"content":[{"type":"text","text":"claude ok"}],"stop_reason":"end_turn","usage":{"input_tokens":17,"output_tokens":6}}`))
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameAnthropic),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
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
		{name: "Fable 5", modelIdentifier: "claude-fable-5", expectedMaxTokens: 128000},
		{name: "Sonnet 5", modelIdentifier: "claude-sonnet-5", expectedMaxTokens: 128000},
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
				_, _ = responseWriter.Write([]byte(`{"content":[{"type":"text","text":"claude default max ok"}],"stop_reason":"stop_sequence"}`))
			}))
			defer upstreamServer.Close()

			router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
				Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameAnthropic),
				LogLevel:              proxy.LogLevelInfo,
				WorkerCount:           1,
				QueueSize:             1,
				RequestTimeoutSeconds: TestTimeout,
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
		_, _ = responseWriter.Write([]byte(`{"content":[{"type":"text","text":"claude cap ok"}],"stop_reason":"end_turn"}`))
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameAnthropic),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
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
		if authorizationHeader := request.Header.Get("Authorization"); authorizationHeader != "Bearer "+testXAIKey {
			t.Fatalf("authorization=%q want=%q", authorizationHeader, "Bearer "+testXAIKey)
		}
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read body: %v", readError)
		}
		if unmarshalError := json.Unmarshal(bodyBytes, &capturedPayload); unmarshalError != nil {
			t.Fatalf("unmarshal body: %v", unmarshalError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"grok ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}}`))
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameXAI),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
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

func TestProviderRoutingRejectsGeminiJSONPostMaxTokensAboveModelLimit(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		t.Fatal("upstream must not be called for max_tokens above Gemini model limit")
	}))
	defer upstreamServer.Close()

	logger := zap.NewNop()
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameGemini),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
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
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameGemini),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
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
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameAnthropic),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
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
		Endpoints:             providerEndpoints("https://gemini.invalid", proxy.ProviderNameGemini),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
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
		{name: "retired Gemini 2.5 Flash", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=gemini&model=gemini-2.5-flash", expectedCode: http.StatusBadRequest},
		{name: "retired Gemini 2.5 Flash-Lite", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=gemini&model=gemini-2.5-flash-lite", expectedCode: http.StatusBadRequest},
		{name: "retired Gemini 2.5 Pro", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=gemini&model=gemini-2.5-pro", expectedCode: http.StatusBadRequest},
		{name: "retired Gemini 3.1 Flash-Lite", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=gemini&model=gemini-3.1-flash-lite", expectedCode: http.StatusBadRequest},
		{name: "retired Gemini 3.1 Pro Preview", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=gemini&model=gemini-3.1-pro-preview", expectedCode: http.StatusBadRequest},
		{name: "unsupported web search", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=gemini&web_search=true", expectedCode: http.StatusBadRequest},
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

func TestProviderRoutingRejectsAnthropicMetaAndGrokUnsupportedCapabilities(t *testing.T) {
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints: providerEndpointOverrides(map[string]string{
			proxy.ProviderNameAnthropic: "https://anthropic.invalid",
			proxy.ProviderNameMeta:      "https://meta.invalid",
			proxy.ProviderNameXAI:       "https://xai.invalid",
		}, nil),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	testCases := []struct {
		name   string
		target string
		method string
	}{
		{name: "anthropic web search", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=anthropic&web_search=true"},
		{name: "meta web search", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=meta&web_search=true"},
		{name: "xai web search", method: http.MethodGet, target: "/?key=" + TestSecret + "&prompt=hello&provider=xai&web_search=true"},
		{name: "anthropic dictation", method: http.MethodPost, target: "/dictate?key=" + TestSecret + "&provider=anthropic"},
		{name: "meta dictation", method: http.MethodPost, target: "/dictate?key=" + TestSecret + "&provider=meta"},
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
	tenantConfiguration := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	delete(tenantConfiguration.ProviderKeys, proxy.ProviderNameGemini)
	router, buildError := buildRouterWithManagedTenant(t, proxy.Configuration{
		Endpoints:             providerEndpoints("https://gemini.invalid", proxy.ProviderNameGemini),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, logger.Sugar(), tenantConfiguration)
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=gemini", nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusConflict {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusConflict, responseRecorder.Body.String())
	}
	if strings.TrimSpace(responseRecorder.Body.String()) != "provider_not_configured" {
		t.Fatalf("body=%q want provider_not_configured", responseRecorder.Body.String())
	}
}

func TestProviderRoutingRejectsAnthropicMetaAndGrokMissingCredentials(t *testing.T) {
	tenantConfiguration := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	delete(tenantConfiguration.ProviderKeys, proxy.ProviderNameAnthropic)
	delete(tenantConfiguration.ProviderKeys, proxy.ProviderNameMeta)
	delete(tenantConfiguration.ProviderKeys, proxy.ProviderNameXAI)
	router, buildError := buildRouterWithManagedTenant(t, proxy.Configuration{
		Endpoints: providerEndpointOverrides(map[string]string{
			proxy.ProviderNameAnthropic: "https://anthropic.invalid",
			proxy.ProviderNameMeta:      "https://meta.invalid",
			proxy.ProviderNameXAI:       "https://xai.invalid",
		}, nil),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar(), tenantConfiguration)
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	testCases := []struct {
		name     string
		provider string
		model    string
	}{
		{name: "anthropic", provider: proxy.ProviderNameAnthropic, model: proxy.ModelNameClaudeSonnet46},
		{name: "meta", provider: proxy.ProviderNameMeta, model: proxy.ModelNameMuseSpark11},
		{name: "xai", provider: proxy.ProviderNameXAI, model: proxy.ModelNameGrok43},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider="+testCase.provider+"&model="+url.QueryEscape(testCase.model), nil)
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusConflict {
				subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusConflict, responseRecorder.Body.String())
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
		{name: "missing id", statusCode: http.StatusOK, body: `{"status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"orphaned text"}]}]}`, wantStatus: http.StatusBadGateway},
		{name: "negative usage", statusCode: http.StatusOK, body: `{"id":"error-interaction","status":"queued","usage":{"total_input_tokens":1,"total_output_tokens":-1}}`, wantStatus: http.StatusBadGateway},
		{name: "missing status", statusCode: http.StatusOK, body: `{"id":"error-interaction","steps":[{"type":"model_output","content":[{"type":"text","text":"unfinished text"}]}]}`, wantStatus: http.StatusBadGateway},
		{name: "failed status", statusCode: http.StatusOK, body: `{"id":"error-interaction","status":"failed"}`, wantStatus: http.StatusBadGateway},
		{name: "missing text", statusCode: http.StatusOK, body: `{"id":"error-interaction","status":"completed","steps":[{"type":"model_output","content":[{"type":"thought","text":"hidden"}]}]}`, wantStatus: http.StatusBadGateway},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				responseWriter.Header().Set("Content-Type", "application/json")
				if testCase.name == "negative usage" && request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/cancel") {
					http.Error(responseWriter, "private cleanup failure", http.StatusInternalServerError)
					return
				}
				if request.Method == http.MethodDelete {
					writeGeminiInteractionDeleted(subTest, responseWriter)
					return
				}
				responseWriter.WriteHeader(testCase.statusCode)
				_, _ = responseWriter.Write([]byte(testCase.body))
			}))
			defer upstreamServer.Close()

			logger := zap.NewNop()
			router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
				Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameGemini),
				LogLevel:              proxy.LogLevelInfo,
				WorkerCount:           1,
				QueueSize:             1,
				RequestTimeoutSeconds: TestTimeout,
			}, logger.Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=gemini&model="+proxy.ModelNameGemini35Flash, nil)
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
			Endpoints:             providerEndpoints("http://[::1", proxy.ProviderNameGemini),
			LogLevel:              proxy.LogLevelInfo,
			WorkerCount:           1,
			QueueSize:             1,
			RequestTimeoutSeconds: TestTimeout,
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
			Endpoints:             providerEndpoints("https://gemini.invalid", proxy.ProviderNameGemini),
			LogLevel:              proxy.LogLevelInfo,
			WorkerCount:           1,
			QueueSize:             1,
			RequestTimeoutSeconds: 1,
		}, logger.Sugar())
		if buildError != nil {
			subTest.Fatalf(messageBuildRouterError, buildError)
		}

		request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=gemini", nil)
		requestContext, cancelRequest := context.WithTimeout(request.Context(), coverageShortRequestTimeout)
		defer cancelRequest()
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request.WithContext(requestContext))
		if responseRecorder.Code != 499 {
			subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, 499, responseRecorder.Body.String())
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
		{name: "negative usage", statusCode: http.StatusOK, body: `{"content":[{"type":"text","text":"bad usage"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":-1}}`, wantStatus: http.StatusBadGateway},
		{name: "missing stop reason", statusCode: http.StatusOK, body: `{"content":[{"type":"text","text":"unfinished text"}]}`, wantStatus: http.StatusBadGateway},
		{name: "refusal stop reason", statusCode: http.StatusOK, body: `{"content":[{"type":"text","text":"refused text"}],"stop_reason":"refusal"}`, wantStatus: http.StatusBadGateway},
		{name: "paused turn stop reason", statusCode: http.StatusOK, body: `{"content":[{"type":"text","text":"intermediate text"}],"stop_reason":"pause_turn"}`, wantStatus: http.StatusBadGateway},
		{name: "missing text", statusCode: http.StatusOK, body: `{"content":[{"type":"tool_use","text":"not visible"}],"stop_reason":"end_turn"}`, wantStatus: http.StatusBadGateway},
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
				Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameAnthropic),
				LogLevel:              proxy.LogLevelInfo,
				WorkerCount:           1,
				QueueSize:             1,
				RequestTimeoutSeconds: TestTimeout,
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
			Endpoints:             providerEndpoints("http://[::1", proxy.ProviderNameAnthropic),
			LogLevel:              proxy.LogLevelInfo,
			WorkerCount:           1,
			QueueSize:             1,
			RequestTimeoutSeconds: TestTimeout,
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
			Endpoints:             providerEndpoints("https://anthropic.invalid", proxy.ProviderNameAnthropic),
			LogLevel:              proxy.LogLevelInfo,
			WorkerCount:           1,
			QueueSize:             1,
			RequestTimeoutSeconds: 1,
		}, zap.NewNop().Sugar())
		if buildError != nil {
			subTest.Fatalf(messageBuildRouterError, buildError)
		}

		request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=anthropic", nil)
		requestContext, cancelRequest := context.WithTimeout(request.Context(), coverageShortRequestTimeout)
		defer cancelRequest()
		responseRecorder := httptest.NewRecorder()
		router.ServeHTTP(responseRecorder, request.WithContext(requestContext))
		if responseRecorder.Code != 499 {
			subTest.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, 499, responseRecorder.Body.String())
		}
	})
}

func TestProviderRoutingRejectsUnsupportedWebSearch(t *testing.T) {
	logger := zap.NewNop()
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Endpoints:             providerEndpoints("https://deepseek.invalid", proxy.ProviderNameDeepSeek),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, logger.Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=deepseek&model="+proxy.ModelNameDeepSeekV4Flash+"&web_search=true", nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusBadRequest, responseRecorder.Body.String())
	}
}

func TestProviderRoutingRejectsMissingProviderCredential(t *testing.T) {
	logger := zap.NewNop()
	tenantConfiguration := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	delete(tenantConfiguration.ProviderKeys, proxy.ProviderNameDeepSeek)
	router, buildError := buildRouterWithManagedTenant(t, proxy.Configuration{
		Endpoints:             providerEndpoints("https://deepseek.invalid", proxy.ProviderNameDeepSeek),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, logger.Sugar(), tenantConfiguration)
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&provider=deepseek&model="+proxy.ModelNameDeepSeekV4Flash, nil)
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusConflict {
		t.Fatalf("status=%d want=%d body=%s", responseRecorder.Code, http.StatusConflict, responseRecorder.Body.String())
	}
	if strings.TrimSpace(responseRecorder.Body.String()) != "provider_not_configured" {
		t.Fatalf("body=%q want provider_not_configured", responseRecorder.Body.String())
	}
}

func TestProviderRoutingRejectsClientSuppliedMetaCredential(t *testing.T) {
	router := NewTestRouter(t, "https://upstream.invalid")
	for _, credentialField := range []string{"model_api_key", "zai_api_key", "zhipu_api_key", "glm_api_key"} {
		t.Run(credentialField, func(subTest *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&prompt=hello&"+credentialField+"=sk-client", nil)
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusBadRequest || strings.TrimSpace(responseRecorder.Body.String()) != "client provider API keys are not accepted" {
				subTest.Fatalf("field=%s status=%d body=%q", credentialField, responseRecorder.Code, responseRecorder.Body.String())
			}
		})
	}
}

func TestProviderRoutingRejectsConflictingJSONModelParameters(t *testing.T) {
	logger := zap.NewNop()
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
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
		Endpoints: providerEndpointOverrides(nil, map[string]map[string]string{
			proxy.ProviderNameSiliconFlow: {"dictation": upstreamServer.URL},
		}),
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
		MaxInputAudioBytes:    1024 * 1024,
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

func TestProviderRoutingSupportsZAIAndGrokDictation(t *testing.T) {
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
			name:             "zai",
			providerName:     proxy.ProviderNameZAI,
			apiKey:           testZAIKey,
			expectedModel:    "glm-asr-2512",
			expectModelField: true,
			expectedResponse: "zai dictation ok",
			configuration: func(transcriptionsURL string) proxy.Configuration {
				return proxy.Configuration{
					Endpoints: providerEndpointOverrides(nil, map[string]map[string]string{
						proxy.ProviderNameZAI: {"dictation": transcriptionsURL},
					}),
					LogLevel:              proxy.LogLevelInfo,
					WorkerCount:           1,
					QueueSize:             1,
					RequestTimeoutSeconds: TestTimeout,
					MaxInputAudioBytes:    1024 * 1024,
				}
			},
		},
		{
			name:             "xai",
			providerName:     proxy.ProviderNameXAI,
			apiKey:           testXAIKey,
			expectedModel:    "",
			expectModelField: false,
			expectedResponse: "xai dictation ok",
			configuration: func(transcriptionsURL string) proxy.Configuration {
				return proxy.Configuration{
					Endpoints: providerEndpointOverrides(nil, map[string]map[string]string{
						proxy.ProviderNameXAI: {"dictation": transcriptionsURL},
					}),
					LogLevel:              proxy.LogLevelInfo,
					WorkerCount:           1,
					QueueSize:             1,
					RequestTimeoutSeconds: TestTimeout,
					MaxInputAudioBytes:    1024 * 1024,
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
