package proxy_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func TestV2RoutesExactOrderedImageAndAudioAttachmentsThroughGemini(testingInstance *testing.T) {
	var capturedPayload map[string]any
	deleteCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		assertGeminiInteractionHeaders(testingInstance, httpRequest, testGeminiKey)
		if httpRequest.Method == http.MethodDelete && httpRequest.URL.Path == testGeminiInteractionsPath+"/media-input" {
			deleteCount++
			writeGeminiInteractionDeleted(testingInstance, responseWriter)
			return
		}
		if httpRequest.Method != http.MethodPost || httpRequest.URL.Path != testGeminiInteractionsPath {
			testingInstance.Fatalf("upstream request=%s %s", httpRequest.Method, httpRequest.URL.Path)
		}
		capturedPayload = decodeGeminiInteractionRequest(testingInstance, httpRequest)
		writeGeminiInteractionSnapshot(testingInstance, responseWriter, "media-input", "completed", "media accepted", nil)
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(testingInstance, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:             TestAPIKey,
		GeminiKey:             testGeminiKey,
		GeminiBaseURL:         upstreamServer.URL,
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		testingInstance.Fatalf(messageBuildRouterError, buildError)
	}

	firstImage := []byte("first-image")
	secondImage := []byte("second-image")
	audio := []byte("voice-track")
	requestBody := mediaV2RequestBody(testingInstance, proxy.ModelNameGemini35Flash, "Inspect in exact order.", []map[string]any{
		messageMediaPayload("image", "image/png", firstImage),
		messageMediaPayload("image", "image/jpeg", secondImage),
		messageMediaPayload("audio", "audio/m4a", audio),
	})
	request := httptest.NewRequest(
		http.MethodPost,
		"/v2?key="+TestSecret+"&provider="+proxy.ProviderNameGemini+"&format=application/json",
		strings.NewReader(requestBody),
	)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		testingInstance.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	input, inputOK := capturedPayload["input"].([]any)
	if !inputOK || len(input) != 1 || deleteCount != 1 {
		testingInstance.Fatalf("input=%v deletes=%d", capturedPayload["input"], deleteCount)
	}
	userStep, stepOK := input[0].(map[string]any)
	if !stepOK || userStep["type"] != testGeminiInteractionUserStep {
		testingInstance.Fatalf("step=%v", input[0])
	}
	content, contentOK := userStep["content"].([]any)
	if !contentOK || len(content) != 4 {
		testingInstance.Fatalf("content=%v", userStep["content"])
	}
	assertGeminiInteractionTextContent(testingInstance, content[0], "Inspect in exact order.")
	assertGeminiInteractionMediaContent(testingInstance, content[1], "image", "image/png", firstImage)
	assertGeminiInteractionMediaContent(testingInstance, content[2], "image", "image/jpeg", secondImage)
	assertGeminiInteractionMediaContent(testingInstance, content[3], "audio", "audio/m4a", audio)

	var responsePayload map[string]any
	if decodeError := json.Unmarshal(responseRecorder.Body.Bytes(), &responsePayload); decodeError != nil {
		testingInstance.Fatalf("decode response: %v", decodeError)
	}
	responseMessages, messagesOK := responsePayload["messages"].([]any)
	if !messagesOK || len(responseMessages) != 1 {
		testingInstance.Fatalf("response messages=%v", responsePayload["messages"])
	}
	responseMessage, responseMessageOK := responseMessages[0].(map[string]any)
	if !responseMessageOK || responseMessage["content"] != "Inspect in exact order." {
		testingInstance.Fatalf("response message=%v", responseMessages[0])
	}
	if _, echoedAttachments := responseMessage["attachments"]; echoedAttachments ||
		strings.Contains(responseRecorder.Body.String(), base64.StdEncoding.EncodeToString(firstImage)) ||
		strings.Contains(responseRecorder.Body.String(), base64.StdEncoding.EncodeToString(audio)) {
		testingInstance.Fatalf("response echoed media=%s", responseRecorder.Body.String())
	}
}

func TestV2RejectsInvalidOrUnsupportedMediaBeforeUpstreamWork(testingInstance *testing.T) {
	upstreamCalls := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		upstreamCalls++
		http.Error(responseWriter, "unexpected upstream call", http.StatusInternalServerError)
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(testingInstance, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:             TestAPIKey,
		DeepSeekKey:           testDeepSeekKey,
		DeepSeekBaseURL:       upstreamServer.URL,
		GeminiKey:             testGeminiKey,
		GeminiBaseURL:         upstreamServer.URL,
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		testingInstance.Fatalf(messageBuildRouterError, buildError)
	}

	validImage := messageMediaPayload("image", "image/png", []byte("image"))
	validAudio := messageMediaPayload("audio", "audio/wav", []byte("audio"))
	for _, testCase := range []struct {
		name        string
		provider    string
		model       string
		role        string
		attachments any
	}{
		{name: "null attachments", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: nil},
		{name: "empty attachments", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: []any{}},
		{name: "non-array attachments", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: "image"},
		{name: "unknown attachment field", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "image/png", "data": validImage["data"], "sha256": validImage["sha256"], "detail": "high"}}},
		{name: "unsupported type", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: []any{map[string]any{"type": "video", "mime_type": "video/mp4", "data": validImage["data"], "sha256": validImage["sha256"]}}},
		{name: "unsupported image MIME", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "audio/mpeg", "data": validImage["data"], "sha256": validImage["sha256"]}}},
		{name: "unsupported audio MIME", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: []any{map[string]any{"type": "audio", "mime_type": "image/png", "data": validAudio["data"], "sha256": validAudio["sha256"]}}},
		{name: "empty data", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "image/png", "data": "", "sha256": validImage["sha256"]}}},
		{name: "empty decoded data", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "image/png", "data": "\n", "sha256": validImage["sha256"]}}},
		{name: "noncanonical base64", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "image/png", "data": validImage["data"].(string) + "\n", "sha256": validImage["sha256"]}}},
		{name: "malformed digest", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "image/png", "data": validImage["data"], "sha256": "xyz"}}},
		{name: "uppercase digest", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "image/png", "data": validImage["data"], "sha256": strings.ToUpper(validImage["sha256"].(string))}}},
		{name: "mismatched digest", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "image/png", "data": validImage["data"], "sha256": messageMediaPayload("image", "image/png", []byte("other"))["sha256"]}}},
		{name: "system attachment", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Flash, role: "system", attachments: []any{validImage}},
		{name: "text-only model", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini25Pro, role: "user", attachments: []any{validImage}},
		{name: "text-only provider", provider: proxy.ProviderNameDeepSeek, model: proxy.ModelNameDeepSeekV4Flash, role: "user", attachments: []any{validImage}},
	} {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			bodyBytes, marshalError := json.Marshal(map[string]any{
				"messages": []map[string]any{{
					"role":        testCase.role,
					"content":     "inspect",
					"attachments": testCase.attachments,
				}},
				"model": testCase.model,
			})
			if marshalError != nil {
				subTest.Fatalf("marshal request: %v", marshalError)
			}
			request := httptest.NewRequest(
				http.MethodPost,
				"/v2?key="+TestSecret+"&provider="+testCase.provider,
				strings.NewReader(string(bodyBytes)),
			)
			request.Header.Set("Content-Type", "application/json")
			responseRecorder := httptest.NewRecorder()

			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusBadRequest {
				subTest.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
			}
		})
	}
	if upstreamCalls != 0 {
		testingInstance.Fatalf("upstream calls=%d want=0", upstreamCalls)
	}
}

func TestCompatibilityMessagesRejectMediaAndV2BoundsEncodedMediaBody(testingInstance *testing.T) {
	upstreamCalls := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		upstreamCalls++
		http.Error(responseWriter, "unexpected upstream call", http.StatusInternalServerError)
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(testingInstance, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:             TestAPIKey,
		GeminiKey:             testGeminiKey,
		GeminiBaseURL:         upstreamServer.URL,
		MaxPromptBytes:        512,
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		testingInstance.Fatalf(messageBuildRouterError, buildError)
	}
	validImage := messageMediaPayload("image", "image/png", []byte("image"))
	compatibilityBodyBytes, _ := json.Marshal(map[string]any{
		"messages": []map[string]any{{"role": "user", "content": "inspect", "attachments": []any{validImage}}},
	})
	compatibilityRequest := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret, strings.NewReader(string(compatibilityBodyBytes)))
	compatibilityRequest.Header.Set("Content-Type", "application/json")
	compatibilityResponse := httptest.NewRecorder()
	router.ServeHTTP(compatibilityResponse, compatibilityRequest)
	if compatibilityResponse.Code != http.StatusBadRequest {
		testingInstance.Fatalf("compatibility status=%d body=%s", compatibilityResponse.Code, compatibilityResponse.Body.String())
	}

	oversizedBody := mediaV2RequestBody(testingInstance, proxy.ModelNameGemini25Flash, strings.Repeat("x", 600), []map[string]any{validImage})
	oversizedRequest := httptest.NewRequest(
		http.MethodPost,
		"/v2?key="+TestSecret+"&provider="+proxy.ProviderNameGemini,
		strings.NewReader(oversizedBody),
	)
	oversizedRequest.Header.Set("Content-Type", "application/json")
	oversizedResponse := httptest.NewRecorder()
	router.ServeHTTP(oversizedResponse, oversizedRequest)
	if oversizedResponse.Code != http.StatusRequestEntityTooLarge {
		testingInstance.Fatalf("oversized status=%d body=%s", oversizedResponse.Code, oversizedResponse.Body.String())
	}
	if upstreamCalls != 0 {
		testingInstance.Fatalf("upstream calls=%d want=0", upstreamCalls)
	}
}

func TestModelCatalogRejectsInvalidMediaInputDeclarations(testingInstance *testing.T) {
	for _, testCase := range []struct {
		name      string
		configure func(proxy.ProviderModelCatalogs)
	}{
		{
			name: "unknown media input",
			configure: func(catalogs proxy.ProviderModelCatalogs) {
				setModelMediaInputs(catalogs, proxy.ProviderNameGemini, proxy.ModelNameGemini25Flash, []string{"video"})
			},
		},
		{
			name: "noncanonical media input",
			configure: func(catalogs proxy.ProviderModelCatalogs) {
				setModelMediaInputs(catalogs, proxy.ProviderNameGemini, proxy.ModelNameGemini25Flash, []string{" image"})
			},
		},
		{
			name: "duplicate media input",
			configure: func(catalogs proxy.ProviderModelCatalogs) {
				setModelMediaInputs(catalogs, proxy.ProviderNameGemini, proxy.ModelNameGemini25Flash, []string{"image", "image"})
			},
		},
		{
			name: "unsupported provider adapter",
			configure: func(catalogs proxy.ProviderModelCatalogs) {
				setModelMediaInputs(catalogs, proxy.ProviderNameDeepSeek, proxy.ModelNameDeepSeekV4Flash, []string{"image"})
			},
		},
		{
			name: "unsupported endpoint",
			configure: func(catalogs proxy.ProviderModelCatalogs) {
				openAICatalog := catalogs[proxy.ProviderNameOpenAI]
				openAICatalog.Dictation.Models[0].MediaInputs = []string{"image"}
				catalogs[proxy.ProviderNameOpenAI] = openAICatalog
			},
		},
	} {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			catalogs := testfixtures.ProviderModelCatalogs(subTest)
			testCase.configure(catalogs)
			_, buildError := proxy.BuildRouter(proxy.Configuration{
				Tenants:        proxy.SingleTenantConfigurations("test", TestSecret),
				OpenAIKey:      TestAPIKey,
				ProviderModels: catalogs,
			}, zap.NewNop().Sugar())
			if buildError == nil || !strings.Contains(buildError.Error(), "invalid_model_catalog") {
				subTest.Fatalf("build error=%v", buildError)
			}
		})
	}
}

func mediaV2RequestBody(testingInstance *testing.T, model string, content string, attachments []map[string]any) string {
	testingInstance.Helper()
	bodyBytes, marshalError := json.Marshal(map[string]any{
		"messages": []map[string]any{{
			"role":        "user",
			"content":     content,
			"attachments": attachments,
		}},
		"model": model,
	})
	if marshalError != nil {
		testingInstance.Fatalf("marshal media request: %v", marshalError)
	}
	return string(bodyBytes)
}

func messageMediaPayload(mediaType string, mimeType string, data []byte) map[string]any {
	digest := sha256.Sum256(data)
	return map[string]any{
		"type":      mediaType,
		"mime_type": mimeType,
		"data":      base64.StdEncoding.EncodeToString(data),
		"sha256":    hex.EncodeToString(digest[:]),
	}
}

func assertGeminiInteractionTextContent(testingInstance *testing.T, rawContent any, expectedText string) {
	testingInstance.Helper()
	content, contentOK := rawContent.(map[string]any)
	if !contentOK || content["type"] != "text" || content["text"] != expectedText || content["data"] != nil || content["mime_type"] != nil {
		testingInstance.Fatalf("text content=%v", rawContent)
	}
}

func assertGeminiInteractionMediaContent(testingInstance *testing.T, rawContent any, expectedType string, expectedMIMEType string, expectedData []byte) {
	testingInstance.Helper()
	content, contentOK := rawContent.(map[string]any)
	if !contentOK ||
		content["type"] != expectedType ||
		content["mime_type"] != expectedMIMEType ||
		content["data"] != base64.StdEncoding.EncodeToString(expectedData) ||
		content["text"] != nil {
		testingInstance.Fatalf("media content=%v", rawContent)
	}
}

func setModelMediaInputs(catalogs proxy.ProviderModelCatalogs, providerName string, modelName string, mediaInputs []string) {
	providerCatalog := catalogs[providerName]
	for modelIndex := range providerCatalog.Text.Models {
		if providerCatalog.Text.Models[modelIndex].ID == modelName {
			providerCatalog.Text.Models[modelIndex].MediaInputs = mediaInputs
		}
	}
	catalogs[providerName] = providerCatalog
}
