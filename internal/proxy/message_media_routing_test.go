package proxy_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
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
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		if httpRequest.Method != http.MethodPost ||
			httpRequest.URL.Path != "/models/"+proxy.ModelNameGemini25Flash+":generateContent" {
			testingInstance.Fatalf("upstream request=%s %s", httpRequest.Method, httpRequest.URL.Path)
		}
		bodyBytes, readError := io.ReadAll(httpRequest.Body)
		if readError != nil {
			testingInstance.Fatalf("read upstream body: %v", readError)
		}
		if decodeError := json.Unmarshal(bodyBytes, &capturedPayload); decodeError != nil {
			testingInstance.Fatalf("decode upstream body: %v", decodeError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"media accepted"}]}}]}`))
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
	requestBody := mediaV2RequestBody(testingInstance, proxy.ModelNameGemini25Flash, "Inspect in exact order.", []map[string]any{
		messageMediaPayload("image", "image/png", firstImage),
		messageMediaPayload("image", "image/jpeg", secondImage),
		messageMediaPayload("audio", "audio/mp4", audio),
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
	contents, contentsOK := capturedPayload["contents"].([]any)
	if !contentsOK || len(contents) != 1 {
		testingInstance.Fatalf("contents=%v", capturedPayload["contents"])
	}
	userContent, contentOK := contents[0].(map[string]any)
	if !contentOK || userContent["role"] != "user" {
		testingInstance.Fatalf("content=%v", contents[0])
	}
	parts, partsOK := userContent["parts"].([]any)
	if !partsOK || len(parts) != 4 {
		testingInstance.Fatalf("parts=%v", userContent["parts"])
	}
	assertGeminiTextPart(testingInstance, parts[0], "Inspect in exact order.")
	assertGeminiInlineDataPart(testingInstance, parts[1], "image/png", firstImage)
	assertGeminiInlineDataPart(testingInstance, parts[2], "image/jpeg", secondImage)
	assertGeminiInlineDataPart(testingInstance, parts[3], "audio/mp4", audio)

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

func assertGeminiTextPart(testingInstance *testing.T, rawPart any, expectedText string) {
	testingInstance.Helper()
	part, partOK := rawPart.(map[string]any)
	if !partOK || part["text"] != expectedText || part["inlineData"] != nil {
		testingInstance.Fatalf("text part=%v", rawPart)
	}
}

func assertGeminiInlineDataPart(testingInstance *testing.T, rawPart any, expectedMIMEType string, expectedData []byte) {
	testingInstance.Helper()
	part, partOK := rawPart.(map[string]any)
	if !partOK || part["text"] != nil {
		testingInstance.Fatalf("inline part=%v", rawPart)
	}
	inlineData, inlineDataOK := part["inlineData"].(map[string]any)
	if !inlineDataOK ||
		inlineData["mimeType"] != expectedMIMEType ||
		inlineData["data"] != base64.StdEncoding.EncodeToString(expectedData) {
		testingInstance.Fatalf("inline data=%v", part["inlineData"])
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
