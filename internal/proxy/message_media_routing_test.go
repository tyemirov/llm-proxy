package proxy_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

func TestV2RoutesExactOrderedImageAndAudioAttachmentsThroughGemini(testingInstance *testing.T) {
	var capturedPayload map[string]any
	requestCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		assertGeminiInteractionHeaders(testingInstance, httpRequest, testGeminiKey)
		if httpRequest.Method != http.MethodPost || httpRequest.URL.Path != testGeminiInteractionsPath {
			testingInstance.Fatalf("upstream request=%s %s", httpRequest.Method, httpRequest.URL.Path)
		}
		requestCount++
		capturedPayload = decodeGeminiInteractionRequest(testingInstance, httpRequest)
		if capturedPayload["background"] != false || capturedPayload["store"] != false {
			testingInstance.Fatalf("Gemini media execution=%v", capturedPayload)
		}
		writeGeminiInteractionSnapshot(testingInstance, responseWriter, "", "completed", "media accepted", nil)
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(testingInstance, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameGemini, proxy.ProviderNameXAI),
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
	if !inputOK || len(input) != 1 || requestCount != 1 {
		testingInstance.Fatalf("input=%v requests=%d", capturedPayload["input"], requestCount)
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

func TestV2RoutesExactOrderedImagesThroughProviderAdapters(testingInstance *testing.T) {
	firstImage := []byte("first-image")
	secondImage := []byte("second-image")
	for _, testCase := range []struct {
		name       string
		provider   string
		model      string
		path       string
		assertBody func(*testing.T, map[string]any)
	}{
		{
			name: "OpenAI Responses", provider: proxy.ProviderNameOpenAI, model: proxy.ModelNameGPT4oMini, path: "/responses",
			assertBody: func(subTest *testing.T, payload map[string]any) {
				assertOpenAIImageInput(subTest, payload, "auto", firstImage, secondImage)
				if payload["background"] != true || payload["store"] != true {
					subTest.Fatalf("OpenAI persistence fields=%v", payload)
				}
			},
		},
		{
			name: "Anthropic Messages", provider: proxy.ProviderNameAnthropic, model: "claude-fable-5", path: "/v1/messages",
			assertBody: func(subTest *testing.T, payload map[string]any) {
				messages := payload["messages"].([]any)
				content := messages[0].(map[string]any)["content"].([]any)
				if len(content) != 3 {
					subTest.Fatalf("Anthropic content=%v", content)
				}
				assertAnthropicImageBlock(subTest, content[0], "image/png", firstImage)
				assertAnthropicImageBlock(subTest, content[1], "image/jpeg", secondImage)
				if textBlock := content[2].(map[string]any); textBlock["type"] != "text" || textBlock["text"] != "Inspect in exact order." {
					subTest.Fatalf("Anthropic text=%v", textBlock)
				}
			},
		},
		{
			name: "xAI Responses", provider: proxy.ProviderNameXAI, model: proxy.ModelNameGrok45, path: "/responses",
			assertBody: func(subTest *testing.T, payload map[string]any) {
				assertOpenAIImageInput(subTest, payload, "high", firstImage, secondImage)
				if payload["store"] != false {
					subTest.Fatalf("xAI store=%v", payload["store"])
				}
				if _, found := payload["background"]; found {
					subTest.Fatalf("xAI payload includes background=%v", payload)
				}
			},
		},
		{
			name: "Moonshot Kimi K2.6", provider: proxy.ProviderNameMoonshot, model: proxy.ModelNameMoonshotKimiK26, path: "/chat/completions",
			assertBody: func(subTest *testing.T, payload map[string]any) {
				assertChatCompletionImageInput(subTest, payload, firstImage, secondImage)
			},
		},
		{
			name: "Moonshot Kimi K3", provider: proxy.ProviderNameMoonshot, model: proxy.ModelNameMoonshotKimiK3, path: "/chat/completions",
			assertBody: func(subTest *testing.T, payload map[string]any) {
				assertChatCompletionImageInput(subTest, payload, firstImage, secondImage)
			},
		},
		{
			name: "Moonshot Kimi K2.7 Code", provider: proxy.ProviderNameMoonshot, model: proxy.ModelNameMoonshotKimiK27Code, path: "/chat/completions",
			assertBody: func(subTest *testing.T, payload map[string]any) {
				assertChatCompletionImageInput(subTest, payload, firstImage, secondImage)
			},
		},
		{
			name: "Moonshot Kimi K2.7 Code Highspeed", provider: proxy.ProviderNameMoonshot, model: proxy.ModelNameMoonshotKimiK27CodeHighSpeed, path: "/chat/completions",
			assertBody: func(subTest *testing.T, payload map[string]any) {
				assertChatCompletionImageInput(subTest, payload, firstImage, secondImage)
			},
		},
	} {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			var capturedPayload map[string]any
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
				if httpRequest.Method != http.MethodPost || httpRequest.URL.Path != testCase.path {
					subTest.Fatalf("upstream request=%s %s", httpRequest.Method, httpRequest.URL.Path)
				}
				if decodeError := json.NewDecoder(httpRequest.Body).Decode(&capturedPayload); decodeError != nil {
					subTest.Fatalf("decode upstream request: %v", decodeError)
				}
				responseWriter.Header().Set("Content-Type", "application/json")
				if testCase.provider == proxy.ProviderNameAnthropic {
					_, _ = responseWriter.Write([]byte(`{"content":[{"type":"text","text":"media accepted"}],"stop_reason":"end_turn"}`))
					return
				}
				if testCase.provider == proxy.ProviderNameMoonshot {
					_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"media accepted"},"finish_reason":"stop"}]}`))
					return
				}
				_, _ = responseWriter.Write([]byte(`{"id":"media-input","status":"completed","output_text":"media accepted"}`))
			}))
			defer upstreamServer.Close()

			router, buildError := buildRouterWithCatalogs(subTest, proxy.Configuration{
				Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameOpenAI, proxy.ProviderNameAnthropic, proxy.ProviderNameMoonshot, proxy.ProviderNameXAI),
				LogLevel:              proxy.LogLevelInfo,
				WorkerCount:           1,
				QueueSize:             1,
				RequestTimeoutSeconds: TestTimeout,
			}, zap.NewNop().Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			requestBody := mediaV2RequestBody(subTest, testCase.model, "Inspect in exact order.", []map[string]any{
				messageMediaPayload("image", "image/png", firstImage),
				messageMediaPayload("image", "image/jpeg", secondImage),
			})
			request := httptest.NewRequest(http.MethodPost, "/v2?key="+TestSecret+"&provider="+testCase.provider+"&format=application/json", strings.NewReader(requestBody))
			request.Header.Set("Content-Type", "application/json")
			responseRecorder := httptest.NewRecorder()
			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusOK {
				subTest.Fatalf("status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
			}
			testCase.assertBody(subTest, capturedPayload)
		})
	}
}

func TestV2AppliesNewProviderMediaLimitsAtTheBoundary(testingInstance *testing.T) {
	for _, testCase := range []struct {
		name                string
		provider            string
		model               string
		limitID             string
		limitValue          int64
		boundaryAttachments []map[string]any
		aboveAttachments    []map[string]any
	}{
		{
			name:                "OpenAI image count",
			provider:            proxy.ProviderNameOpenAI,
			model:               proxy.ModelNameGPT4oMini,
			limitID:             proxy.CatalogMediaLimitIDImageCount,
			limitValue:          1,
			boundaryAttachments: []map[string]any{messageMediaPayload("image", "image/png", []byte("x"))},
			aboveAttachments:    []map[string]any{messageMediaPayload("image", "image/png", []byte("x")), messageMediaPayload("image", "image/png", []byte("y"))},
		},
		{
			name:                "Anthropic encoded image bytes",
			provider:            proxy.ProviderNameAnthropic,
			model:               "claude-fable-5",
			limitID:             proxy.CatalogMediaLimitIDImageInlineBytes,
			limitValue:          4,
			boundaryAttachments: []map[string]any{messageMediaPayload("image", "image/png", []byte("x"))},
			aboveAttachments:    []map[string]any{messageMediaPayload("image", "image/png", []byte("xxxx"))},
		},
		{
			name:                "xAI raw image bytes",
			provider:            proxy.ProviderNameXAI,
			model:               proxy.ModelNameGrok45,
			limitID:             proxy.CatalogMediaLimitIDImageInlineBytes,
			limitValue:          1,
			boundaryAttachments: []map[string]any{messageMediaPayload("image", "image/png", []byte("x"))},
			aboveAttachments:    []map[string]any{messageMediaPayload("image", "image/png", []byte("xx"))},
		},
		{
			name:                "Moonshot raw image bytes",
			provider:            proxy.ProviderNameMoonshot,
			model:               proxy.ModelNameMoonshotKimiK3,
			limitID:             proxy.CatalogMediaLimitIDImageInlineBytes,
			limitValue:          1,
			boundaryAttachments: []map[string]any{messageMediaPayload("image", "image/png", []byte("x"))},
			aboveAttachments:    []map[string]any{messageMediaPayload("image", "image/png", []byte("xx"))},
		},
	} {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			upstreamCalls := 0
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
				upstreamCalls++
				responseWriter.Header().Set("Content-Type", "application/json")
				if testCase.provider == proxy.ProviderNameAnthropic {
					_, _ = responseWriter.Write([]byte(`{"content":[{"type":"text","text":"accepted"}],"stop_reason":"end_turn"}`))
					return
				}
				if testCase.provider == proxy.ProviderNameMoonshot {
					_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"content":"accepted"},"finish_reason":"stop"}]}`))
					return
				}
				_, _ = responseWriter.Write([]byte(`{"id":"media-limit","status":"completed","output_text":"accepted"}`))
			}))
			defer upstreamServer.Close()

			catalog := testfixtures.ModelCatalog(subTest)
			setProviderMediaLimit(subTest, &catalog, testCase.provider, testCase.model, testCase.limitID, testCase.limitValue)
			router, buildError := buildRouterWithCatalogs(subTest, proxy.Configuration{
				Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameOpenAI, proxy.ProviderNameAnthropic, proxy.ProviderNameMoonshot, proxy.ProviderNameXAI),
				ModelCatalog:          catalog,
				LogLevel:              proxy.LogLevelInfo,
				WorkerCount:           1,
				QueueSize:             1,
				RequestTimeoutSeconds: TestTimeout,
			}, zap.NewNop().Sugar())
			if buildError != nil {
				subTest.Fatalf(messageBuildRouterError, buildError)
			}

			perform := func(attachments []map[string]any) *httptest.ResponseRecorder {
				request := httptest.NewRequest(
					http.MethodPost,
					"/v2?key="+TestSecret+"&provider="+testCase.provider+"&format=text/plain",
					strings.NewReader(mediaV2RequestBody(subTest, testCase.model, "inspect", attachments)),
				)
				request.Header.Set("Content-Type", "application/json")
				responseRecorder := httptest.NewRecorder()
				router.ServeHTTP(responseRecorder, request)
				return responseRecorder
			}

			boundaryResponse := perform(testCase.boundaryAttachments)
			if boundaryResponse.Code != http.StatusOK || upstreamCalls != 1 {
				subTest.Fatalf("boundary status=%d calls=%d body=%s", boundaryResponse.Code, upstreamCalls, boundaryResponse.Body.String())
			}
			aboveResponse := perform(testCase.aboveAttachments)
			if aboveResponse.Code != http.StatusRequestEntityTooLarge || upstreamCalls != 1 || !strings.Contains(aboveResponse.Body.String(), llmproxycontract.ErrorCodeProviderMediaLimitExceeded) {
				subTest.Fatalf("above status=%d calls=%d body=%s", aboveResponse.Code, upstreamCalls, aboveResponse.Body.String())
			}
		})
	}
}

func assertChatCompletionImageInput(testingInstance *testing.T, payload map[string]any, firstImage []byte, secondImage []byte) {
	testingInstance.Helper()
	messages := payload["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	if len(content) != 3 {
		testingInstance.Fatalf("Chat Completions content=%v", content)
	}
	for index, expected := range []struct {
		mimeType string
		data     []byte
	}{{"image/png", firstImage}, {"image/jpeg", secondImage}} {
		block := content[index].(map[string]any)
		imageURL := block["image_url"].(map[string]any)["url"]
		expectedURL := "data:" + expected.mimeType + ";base64," + base64.StdEncoding.EncodeToString(expected.data)
		if block["type"] != "image_url" || imageURL != expectedURL {
			testingInstance.Fatalf("Chat Completions image[%d]=%v want=%s", index, block, expectedURL)
		}
	}
	if textBlock := content[2].(map[string]any); textBlock["type"] != "text" || textBlock["text"] != "Inspect in exact order." {
		testingInstance.Fatalf("Chat Completions text=%v", textBlock)
	}
}

func setProviderMediaLimit(testingInstance *testing.T, catalog *proxy.ModelCatalog, provider string, model string, limitID string, value int64) {
	testingInstance.Helper()
	offeringIndex := catalogOfferingIndex(*catalog, provider, model)
	if offeringIndex < 0 {
		testingInstance.Fatalf("missing offering provider=%s model=%s", provider, model)
	}
	for limitIndex := range catalog.Offerings[offeringIndex].MediaLimits {
		limit := &catalog.Offerings[offeringIndex].MediaLimits[limitIndex]
		if limit.ID == limitID {
			limit.Status = proxy.CatalogMediaLimitStatusBounded
			limit.Value = &value
			return
		}
	}
	testingInstance.Fatalf("missing media limit provider=%s model=%s limit=%s", provider, model, limitID)
}

func assertOpenAIImageInput(testingInstance *testing.T, payload map[string]any, detail string, firstImage []byte, secondImage []byte) {
	testingInstance.Helper()
	input := payload["input"].([]any)
	content := input[0].(map[string]any)["content"].([]any)
	if len(content) != 3 {
		testingInstance.Fatalf("Responses content=%v", content)
	}
	for index, expected := range []struct {
		mimeType string
		data     []byte
	}{{"image/png", firstImage}, {"image/jpeg", secondImage}} {
		imageBlock := content[index].(map[string]any)
		expectedURL := "data:" + expected.mimeType + ";base64," + base64.StdEncoding.EncodeToString(expected.data)
		if imageBlock["type"] != "input_image" || imageBlock["image_url"] != expectedURL || imageBlock["detail"] != detail {
			testingInstance.Fatalf("Responses image[%d]=%v", index, imageBlock)
		}
	}
	if textBlock := content[2].(map[string]any); textBlock["type"] != "input_text" || textBlock["text"] != "Inspect in exact order." {
		testingInstance.Fatalf("Responses text=%v", textBlock)
	}
}

func assertAnthropicImageBlock(testingInstance *testing.T, rawBlock any, mimeType string, data []byte) {
	testingInstance.Helper()
	block := rawBlock.(map[string]any)
	source := block["source"].(map[string]any)
	if block["type"] != "image" || source["type"] != "base64" || source["media_type"] != mimeType || source["data"] != base64.StdEncoding.EncodeToString(data) {
		testingInstance.Fatalf("Anthropic image=%v", block)
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
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameDeepSeek, proxy.ProviderNameGemini, proxy.ProviderNameMoonshot, proxy.ProviderNameXAI),
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
		{name: "null attachments", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini35Flash, role: "user", attachments: nil},
		{name: "empty attachments", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini35Flash, role: "user", attachments: []any{}},
		{name: "non-array attachments", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini35Flash, role: "user", attachments: "image"},
		{name: "unknown attachment field", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini35Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "image/png", "data": validImage["data"], "detail": "high"}}},
		{name: "obsolete hash field", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini35Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "image/png", "data": validImage["data"], "sha256": strings.Repeat("0", 64)}}},
		{name: "unsupported type", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini35Flash, role: "user", attachments: []any{map[string]any{"type": "video", "mime_type": "video/mp4", "data": validImage["data"]}}},
		{name: "unsupported image MIME", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini35Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "audio/mpeg", "data": validImage["data"]}}},
		{name: "unsupported audio MIME", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini35Flash, role: "user", attachments: []any{map[string]any{"type": "audio", "mime_type": "image/png", "data": validAudio["data"]}}},
		{name: "empty data", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini35Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "image/png", "data": ""}}},
		{name: "empty decoded data", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini35Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "image/png", "data": "\n"}}},
		{name: "noncanonical base64", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini35Flash, role: "user", attachments: []any{map[string]any{"type": "image", "mime_type": "image/png", "data": validImage["data"].(string) + "\n"}}},
		{name: "system attachment", provider: proxy.ProviderNameGemini, model: proxy.ModelNameGemini35Flash, role: "system", attachments: []any{validImage}},
		{name: "text-only model", provider: proxy.ProviderNameXAI, model: proxy.ModelNameGrok43, role: "user", attachments: []any{validImage}},
		{name: "provider MIME", provider: proxy.ProviderNameXAI, model: proxy.ModelNameGrok45, role: "user", attachments: []any{messageMediaPayload("image", "image/webp", []byte("webp"))}},
		{name: "Moonshot unsupported audio", provider: proxy.ProviderNameMoonshot, model: proxy.ModelNameMoonshotKimiK3, role: "user", attachments: []any{validAudio}},
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

func TestCompatibilityMessagesRejectMediaAndV2IgnoresCompatibilityBodyLimit(testingInstance *testing.T) {
	upstreamCalls := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		upstreamCalls++
		http.Error(responseWriter, "unexpected upstream call", http.StatusInternalServerError)
	}))
	defer upstreamServer.Close()

	router, buildError := buildRouterWithCatalogs(testingInstance, proxy.Configuration{
		Endpoints:             providerEndpoints(upstreamServer.URL, proxy.ProviderNameGemini),
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

	oversizedBody := mediaV2RequestBody(testingInstance, proxy.ModelNameGemini35Flash, strings.Repeat("x", 600), []map[string]any{validImage})
	oversizedRequest := httptest.NewRequest(
		http.MethodPost,
		"/v2?key="+TestSecret+"&provider="+proxy.ProviderNameGemini,
		strings.NewReader(oversizedBody),
	)
	oversizedRequest.Header.Set("Content-Type", "application/json")
	oversizedResponse := httptest.NewRecorder()
	router.ServeHTTP(oversizedResponse, oversizedRequest)
	if oversizedResponse.Code != http.StatusBadGateway {
		testingInstance.Fatalf("oversized status=%d body=%s", oversizedResponse.Code, oversizedResponse.Body.String())
	}
	if upstreamCalls != 1 {
		testingInstance.Fatalf("upstream calls=%d want=1", upstreamCalls)
	}
}

func TestModelCatalogRejectsInvalidMediaInputDeclarations(testingInstance *testing.T) {
	for _, testCase := range []struct {
		name      string
		configure func(proxy.ModelCatalog)
	}{
		{
			name: "unknown media input",
			configure: func(catalogs proxy.ModelCatalog) {
				setModelMediaInputs(catalogs, proxy.ProviderNameGemini, proxy.ModelNameGemini35Flash, []string{"video"})
			},
		},
		{
			name: "noncanonical media input",
			configure: func(catalogs proxy.ModelCatalog) {
				setModelMediaInputs(catalogs, proxy.ProviderNameGemini, proxy.ModelNameGemini35Flash, []string{" image"})
			},
		},
		{
			name: "duplicate media input",
			configure: func(catalogs proxy.ModelCatalog) {
				setModelMediaInputs(catalogs, proxy.ProviderNameGemini, proxy.ModelNameGemini35Flash, []string{"image", "image"})
			},
		},
		{
			name: "unsupported provider adapter",
			configure: func(catalogs proxy.ModelCatalog) {
				setModelMediaInputs(catalogs, proxy.ProviderNameDeepSeek, proxy.ModelNameDeepSeekV4Flash, []string{"image"})
			},
		},
		{
			name: "unsupported xAI Chat Completions adapter",
			configure: func(catalogs proxy.ModelCatalog) {
				setModelMediaInputs(catalogs, proxy.ProviderNameXAI, proxy.ModelNameGrok43, []string{"image"})
			},
		},
		{
			name: "unsupported Chat Completions audio input",
			configure: func(catalogs proxy.ModelCatalog) {
				setModelMediaInputs(catalogs, proxy.ProviderNameMoonshot, proxy.ModelNameMoonshotKimiK3, []string{"audio"})
			},
		},
		{
			name: "unsupported endpoint",
			configure: func(catalogs proxy.ModelCatalog) {
				offeringIndex := catalogOfferingIndex(catalogs, proxy.ProviderNameOpenAI, proxy.DefaultDictationModel)
				catalogs.Offerings[offeringIndex].MediaInputs = []string{"image"}
			},
		},
	} {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			catalogs := testfixtures.ModelCatalog(subTest)
			testCase.configure(catalogs)
			_, buildError := buildRouterWithCatalogs(subTest, proxy.Configuration{
				ModelCatalog: catalogs,
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
	return map[string]any{
		"type":      mediaType,
		"mime_type": mimeType,
		"data":      base64.StdEncoding.EncodeToString(data),
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

func setModelMediaInputs(catalogs proxy.ModelCatalog, providerName string, modelName string, mediaInputs []string) {
	modelIndex := catalogExactModelIndex(catalogs, modelName)
	offeringIndex := catalogOfferingIndex(catalogs, providerName, modelName)
	catalogs.Models[modelIndex].MediaInputs = mediaInputs
	catalogs.Offerings[offeringIndex].MediaInputs = mediaInputs
}
