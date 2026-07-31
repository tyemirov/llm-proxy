package proxy_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

func TestAnalyzerRouteSendsExactOrderedImagesAndStrictSchemaToOpenAI(t *testing.T) {
	var capturedPayload map[string]any
	var upstreamCalls atomic.Int32
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		if request.Method != http.MethodPost {
			t.Fatalf("method=%q", request.Method)
		}
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read upstream body: %v", readError)
		}
		if decodeError := json.Unmarshal(bodyBytes, &capturedPayload); decodeError != nil {
			t.Fatalf("decode upstream body: %v", decodeError)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{
			"id":"analyzer-response",
			"status":"completed",
			"output_text":"{\"outcome\":\"pass\"}",
			"output":[{
				"type":"message",
				"role":"assistant",
				"content":[{"type":"output_text","text":"{\"outcome\":\"pass\"}"}]
			}]
		}`))
	}))
	t.Cleanup(upstreamServer.Close)
	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(upstreamServer.URL)
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:             TestAPIKey,
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
		Endpoints:             endpoints,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	firstImage := []byte("first exact image")
	secondImage := []byte("second exact image")
	requestBody := analyzerRequestBody(t, firstImage, secondImage)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v2/analyze?key="+url.QueryEscape(TestSecret)+"&provider=openai",
		bytes.NewReader(requestBody),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"outcome":"pass"}` {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Header().Get(llmproxycontract.HeaderRequestID) == "" {
		t.Fatal("successful analyzer response is missing proxy request id")
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("upstream calls=%d", upstreamCalls.Load())
	}
	if capturedPayload["model"] != proxy.ModelNameGPT55 ||
		capturedPayload["background"] != true ||
		capturedPayload["store"] != true ||
		capturedPayload["max_output_tokens"] != float64(1200) {
		t.Fatalf("upstream payload=%v", capturedPayload)
	}
	reasoning, reasoningOK := capturedPayload["reasoning"].(map[string]any)
	if !reasoningOK || reasoning["effort"] != "high" {
		t.Fatalf("reasoning=%v", capturedPayload["reasoning"])
	}
	text, textOK := capturedPayload["text"].(map[string]any)
	format, formatOK := text["format"].(map[string]any)
	if !textOK || !formatOK ||
		format["type"] != "json_schema" ||
		format["name"] != "semantic_qa_report" ||
		format["strict"] != true {
		t.Fatalf("text format=%v", capturedPayload["text"])
	}
	input, inputOK := capturedPayload["input"].([]any)
	if !inputOK || len(input) != 2 {
		t.Fatalf("input=%v", capturedPayload["input"])
	}
	userMessage := input[1].(map[string]any)
	content := userMessage["content"].([]any)
	if len(content) != 3 {
		t.Fatalf("content=%v", content)
	}
	assertOpenAIImagePart(t, content[1], "image/png", firstImage, "high")
	assertOpenAIImagePart(t, content[2], "image/jpeg", secondImage, "low")
}

func TestAnalyzerRouteRejectsInvalidDigestAudioAndUnsupportedProviderBeforeUpstream(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		responseWriter.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstreamServer.Close)
	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(upstreamServer.URL)
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:             TestAPIKey,
		DeepSeekKey:           testDeepSeekKey,
		DeepSeekBaseURL:       upstreamServer.URL,
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
		Endpoints:             endpoints,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	validBody := analyzerRequestBody(t, []byte("first"), []byte("second"))
	var decodedBody map[string]any
	if decodeError := json.Unmarshal(validBody, &decodedBody); decodeError != nil {
		t.Fatalf("decode fixture: %v", decodeError)
	}
	messages := decodedBody["messages"].([]any)
	userMessage := messages[1].(map[string]any)
	content := userMessage["content"].([]any)
	firstImage := content[1].(map[string]any)
	firstImage["sha256"] = strings.Repeat("0", 64)
	invalidDigestBody, marshalError := json.Marshal(decodedBody)
	if marshalError != nil {
		t.Fatalf("marshal invalid digest body: %v", marshalError)
	}

	audioDigest := sha256.Sum256([]byte("audio"))
	audioBody := []byte(`{
		"messages":[{
			"role":"user",
			"content":[
				{"type":"text","text":"Inspect."},
				{"type":"audio","mime_type":"audio/mp4","data":"` + base64.StdEncoding.EncodeToString([]byte("audio")) + `","sha256":"` + hex.EncodeToString(audioDigest[:]) + `"}
			]
		}],
		"output_schema":{"name":"report","schema":{"type":"object","additionalProperties":false}},
		"model":"` + proxy.ModelNameGPT55 + `"
	}`)
	invalidAudioDigestBody := []byte(strings.Replace(string(audioBody), hex.EncodeToString(audioDigest[:]), strings.Repeat("0", 64), 1))
	var deepSeekBodyPayload map[string]any
	if decodeError := json.Unmarshal(validBody, &deepSeekBodyPayload); decodeError != nil {
		t.Fatalf("decode DeepSeek body: %v", decodeError)
	}
	deepSeekBodyPayload["model"] = "deepseek-v4-flash"
	deepSeekBody, marshalError := json.Marshal(deepSeekBodyPayload)
	if marshalError != nil {
		t.Fatalf("marshal DeepSeek body: %v", marshalError)
	}

	testCases := []struct {
		name     string
		provider string
		body     []byte
	}{
		{name: "digest mismatch", provider: proxy.ProviderNameOpenAI, body: invalidDigestBody},
		{name: "audio digest mismatch", provider: proxy.ProviderNameOpenAI, body: invalidAudioDigestBody},
		{name: "audio unsupported", provider: proxy.ProviderNameOpenAI, body: audioBody},
		{name: "provider unsupported", provider: proxy.ProviderNameDeepSeek, body: deepSeekBody},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			request := httptest.NewRequest(
				http.MethodPost,
				"/v2/analyze?key="+url.QueryEscape(TestSecret)+"&provider="+url.QueryEscape(testCase.provider),
				bytes.NewReader(testCase.body),
			)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				subTest.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
		})
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("upstream calls=%d want=0", upstreamCalls.Load())
	}
}

func TestAnalyzerRouteHandlesStrictUpstreamLifecycle(t *testing.T) {
	initialResponseCases := []struct {
		name       string
		statusCode int
		body       string
	}{
		{name: "provider HTTP error", statusCode: http.StatusBadRequest, body: `{"error":"bad request"}`},
		{name: "malformed provider JSON", statusCode: http.StatusOK, body: `{`},
		{name: "completed non JSON output", statusCode: http.StatusOK, body: `{"id":"invalid","status":"completed","output_text":"not-json"}`},
		{name: "terminal provider failure", statusCode: http.StatusOK, body: `{"id":"failed","status":"failed"}`},
		{name: "unknown provider status", statusCode: http.StatusOK, body: `{"id":"unknown","status":"mystery"}`},
		{name: "pending response missing identity", statusCode: http.StatusOK, body: `{"status":"queued"}`},
	}
	for _, testCase := range initialResponseCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			router := analyzerRouterWithResponsesHandler(subTest, TestTimeout, func(responseWriter http.ResponseWriter, _ *http.Request) {
				responseWriter.Header().Set("Content-Type", "application/json")
				responseWriter.WriteHeader(testCase.statusCode)
				_, _ = responseWriter.Write([]byte(testCase.body))
			})
			statusCode, _ := performAnalyzerRequest(subTest, router, context.Background())
			if statusCode != http.StatusBadGateway {
				subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
			}
		})
	}

	t.Run("queued response polls through in progress to strict completion", func(subTest *testing.T) {
		var pollCount atomic.Int32
		router := analyzerRouterWithResponsesHandler(subTest, TestTimeout, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			switch {
			case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/":
				_, _ = responseWriter.Write([]byte(`{"id":"poll-success","status":"queued","usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`))
			case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == "/poll-success":
				if pollCount.Add(1) == 1 {
					_, _ = responseWriter.Write([]byte(`{"id":"poll-success","status":"in_progress","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
					return
				}
				_, _ = responseWriter.Write([]byte(`{"id":"poll-success","status":"completed","output_text":"{\"outcome\":\"pass\"}","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`))
			default:
				http.NotFound(responseWriter, httpRequest)
			}
		})
		statusCode, body := performAnalyzerRequest(subTest, router, context.Background())
		if statusCode != http.StatusOK || strings.TrimSpace(body) != `{"outcome":"pass"}` {
			subTest.Fatalf("status=%d body=%q", statusCode, body)
		}
		if pollCount.Load() != 2 {
			subTest.Fatalf("poll count=%d want=2", pollCount.Load())
		}
	})

	t.Run("poll terminal failure", func(subTest *testing.T) {
		router := analyzerRouterWithResponsesHandler(subTest, TestTimeout, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			if httpRequest.Method == http.MethodPost {
				_, _ = responseWriter.Write([]byte(`{"id":"poll-failed","status":"queued"}`))
				return
			}
			_, _ = responseWriter.Write([]byte(`{"id":"poll-failed","status":"incomplete","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
		})
		statusCode, _ := performAnalyzerRequest(subTest, router, context.Background())
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("poll protocol failure", func(subTest *testing.T) {
		router := analyzerRouterWithResponsesHandler(subTest, TestTimeout, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			if httpRequest.Method == http.MethodPost {
				_, _ = responseWriter.Write([]byte(`{"id":"poll-unknown","status":"queued"}`))
				return
			}
			_, _ = responseWriter.Write([]byte(`{"id":"poll-unknown","status":"mystery"}`))
		})
		statusCode, _ := performAnalyzerRequest(subTest, router, context.Background())
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("caller cancellation during poll fetch", func(subTest *testing.T) {
		router := analyzerRouterWithResponsesHandler(subTest, TestTimeout, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			if httpRequest.Method == http.MethodPost {
				_, _ = responseWriter.Write([]byte(`{"id":"poll-blocked","status":"queued"}`))
				return
			}
			<-httpRequest.Context().Done()
		})
		requestContext, cancelRequest := context.WithTimeout(context.Background(), 75*time.Millisecond)
		defer cancelRequest()
		statusCode, _ := performAnalyzerRequest(subTest, router, requestContext)
		if statusCode != 499 {
			subTest.Fatalf("status=%d want=499", statusCode)
		}
	})

	t.Run("caller cancellation during poll interval", func(subTest *testing.T) {
		router := analyzerRouterWithResponsesHandler(subTest, TestTimeout, func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			if httpRequest.Method == http.MethodPost {
				_, _ = responseWriter.Write([]byte(`{"id":"poll-wait","status":"queued"}`))
				return
			}
			_, _ = responseWriter.Write([]byte(`{"id":"poll-wait","status":"in_progress"}`))
		})
		requestContext, cancelRequest := context.WithTimeout(context.Background(), 75*time.Millisecond)
		defer cancelRequest()
		statusCode, _ := performAnalyzerRequest(subTest, router, requestContext)
		if statusCode != 499 {
			subTest.Fatalf("status=%d want=499", statusCode)
		}
	})

	t.Run("invalid provider URL fails request construction", func(subTest *testing.T) {
		endpoints := proxy.NewEndpoints()
		endpoints.SetResponsesURL("http://[::1")
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:               proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:             TestAPIKey,
			LogLevel:              proxy.LogLevelInfo,
			WorkerCount:           1,
			QueueSize:             1,
			RequestTimeoutSeconds: TestTimeout,
			Endpoints:             endpoints,
		})
		statusCode, _ := performAnalyzerRequest(subTest, router, context.Background())
		if statusCode != http.StatusBadGateway {
			subTest.Fatalf("status=%d want=%d", statusCode, http.StatusBadGateway)
		}
	})

	t.Run("caller cancellation after completed provider response suppresses success", func(subTest *testing.T) {
		requestContext, cancelRequest := context.WithCancel(context.Background())
		previousClient := proxy.HTTPClient
		proxy.HTTPClient = coverageHTTPDoer(func(*http.Request) (*http.Response, error) {
			cancelRequest()
			return coverageHTTPResponse(http.StatusOK, `{"id":"completed-cancelled","status":"completed","output_text":"{\"outcome\":\"pass\"}"}`), nil
		})
		subTest.Cleanup(func() {
			proxy.HTTPClient = previousClient
			cancelRequest()
		})
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:               proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:             TestAPIKey,
			LogLevel:              proxy.LogLevelInfo,
			WorkerCount:           1,
			QueueSize:             1,
			RequestTimeoutSeconds: TestTimeout,
		})
		statusCode, _ := performAnalyzerRequest(subTest, router, requestContext)
		if statusCode != 499 {
			subTest.Fatalf("status=%d want=499", statusCode)
		}
	})
}

func TestAnalyzerRouteRejectsClosedContractViolationsBeforeUpstream(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		responseWriter.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstreamServer.Close)
	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(upstreamServer.URL)
	router, buildError := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:             TestAPIKey,
		DeepSeekKey:           testDeepSeekKey,
		DeepSeekBaseURL:       upstreamServer.URL,
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: TestTimeout,
		Endpoints:             endpoints,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		t.Fatalf(messageBuildRouterError, buildError)
	}

	type contractCase struct {
		name       string
		query      url.Values
		rawBody    []byte
		mutateBody func(map[string]any)
	}
	validImage := analyzerBinaryContent("image", "image/png", []byte("image"))
	validImage["detail"] = "auto"
	validAudio := analyzerBinaryContent("audio", "audio/mpeg", []byte("audio"))
	testCases := []contractCase{
		{name: "provider credential query", query: url.Values{"openai_api_key": []string{"forbidden"}}},
		{name: "unknown query", query: url.Values{"extra": []string{"forbidden"}}},
		{name: "repeated provider query", query: url.Values{"provider": []string{proxy.ProviderNameOpenAI, proxy.ProviderNameOpenAI}}},
		{name: "model query", query: url.Values{"model": []string{proxy.ModelNameGPT55}}},
		{name: "provider credential body", mutateBody: func(body map[string]any) { body["openai_api_key"] = "forbidden" }},
		{name: "malformed JSON", rawBody: []byte(`{`)},
		{name: "trailing JSON", rawBody: []byte(string(analyzerTextRequestBody(t)) + `{}`)},
		{name: "unknown body field", mutateBody: func(body map[string]any) { body["extra"] = true }},
		{name: "missing model", mutateBody: func(body map[string]any) { delete(body, "model") }},
		{name: "noncanonical model", mutateBody: func(body map[string]any) { body["model"] = " " + proxy.ModelNameGPT55 }},
		{name: "invalid max tokens", mutateBody: func(body map[string]any) { body["max_tokens"] = 0 }},
		{name: "blank schema name", mutateBody: func(body map[string]any) {
			body["output_schema"].(map[string]any)["name"] = ""
		}},
		{name: "invalid schema name", mutateBody: func(body map[string]any) {
			body["output_schema"].(map[string]any)["name"] = "bad schema"
		}},
		{name: "noncanonical schema name", mutateBody: func(body map[string]any) {
			body["output_schema"].(map[string]any)["name"] = " report"
		}},
		{name: "non object schema", mutateBody: func(body map[string]any) {
			body["output_schema"].(map[string]any)["schema"] = []any{}
		}},
		{name: "missing messages", mutateBody: func(body map[string]any) { body["messages"] = []any{} }},
		{name: "mixed message order", mutateBody: func(body map[string]any) {
			firstMessage := analyzerFirstMessage(body)
			firstMessage["order"] = 1
			body["messages"] = append(body["messages"].([]any), map[string]any{
				"role":    "user",
				"content": []any{map[string]any{"type": "text", "text": "second"}},
			})
		}},
		{name: "negative message order", mutateBody: func(body map[string]any) {
			analyzerFirstMessage(body)["order"] = -1
		}},
		{name: "duplicate message order", mutateBody: func(body map[string]any) {
			analyzerFirstMessage(body)["order"] = 1
			body["messages"] = append(body["messages"].([]any), map[string]any{
				"role":    "user",
				"order":   1,
				"content": []any{map[string]any{"type": "text", "text": "second"}},
			})
		}},
		{name: "unsupported message role", mutateBody: func(body map[string]any) {
			analyzerFirstMessage(body)["role"] = "assistant"
		}},
		{name: "noncanonical message role", mutateBody: func(body map[string]any) {
			analyzerFirstMessage(body)["role"] = "User"
		}},
		{name: "empty message content", mutateBody: func(body map[string]any) {
			analyzerFirstMessage(body)["content"] = []any{}
		}},
		{name: "missing user message", mutateBody: func(body map[string]any) {
			analyzerFirstMessage(body)["role"] = "system"
		}},
		{name: "blank text", mutateBody: func(body map[string]any) {
			analyzerFirstContent(body)["text"] = " "
		}},
		{name: "text binary field", mutateBody: func(body map[string]any) {
			analyzerFirstContent(body)["data"] = "Zm9yYmlkZGVu"
		}},
		{name: "unknown content type", mutateBody: func(body map[string]any) {
			analyzerFirstContent(body)["type"] = "document"
		}},
		{name: "noncanonical content type", mutateBody: func(body map[string]any) {
			analyzerFirstContent(body)["type"] = "Text"
		}},
		{name: "system image", mutateBody: func(body map[string]any) {
			message := analyzerFirstMessage(body)
			message["role"] = "system"
			message["content"] = []any{validImage}
		}},
		{name: "image text field", mutateBody: func(body map[string]any) {
			image := cloneAnalyzerContent(validImage)
			image["text"] = "forbidden"
			analyzerFirstMessage(body)["content"] = []any{image}
		}},
		{name: "unsupported image MIME", mutateBody: func(body map[string]any) {
			image := cloneAnalyzerContent(validImage)
			image["mime_type"] = "image/gif"
			analyzerFirstMessage(body)["content"] = []any{image}
		}},
		{name: "noncanonical image MIME", mutateBody: func(body map[string]any) {
			image := cloneAnalyzerContent(validImage)
			image["mime_type"] = "IMAGE/PNG"
			analyzerFirstMessage(body)["content"] = []any{image}
		}},
		{name: "unsupported image detail", mutateBody: func(body map[string]any) {
			image := cloneAnalyzerContent(validImage)
			image["detail"] = "ultra"
			analyzerFirstMessage(body)["content"] = []any{image}
		}},
		{name: "noncanonical image detail", mutateBody: func(body map[string]any) {
			image := cloneAnalyzerContent(validImage)
			image["detail"] = "High"
			analyzerFirstMessage(body)["content"] = []any{image}
		}},
		{name: "empty image data", mutateBody: func(body map[string]any) {
			image := cloneAnalyzerContent(validImage)
			image["data"] = ""
			analyzerFirstMessage(body)["content"] = []any{image}
		}},
		{name: "invalid image base64", mutateBody: func(body map[string]any) {
			image := cloneAnalyzerContent(validImage)
			image["data"] = "not-base64"
			analyzerFirstMessage(body)["content"] = []any{image}
		}},
		{name: "noncanonical image digest", mutateBody: func(body map[string]any) {
			image := cloneAnalyzerContent(validImage)
			image["sha256"] = strings.ToUpper(image["sha256"].(string))
			analyzerFirstMessage(body)["content"] = []any{image}
		}},
		{name: "padded image digest", mutateBody: func(body map[string]any) {
			image := cloneAnalyzerContent(validImage)
			image["sha256"] = " " + image["sha256"].(string)
			analyzerFirstMessage(body)["content"] = []any{image}
		}},
		{name: "mismatched image digest", mutateBody: func(body map[string]any) {
			image := cloneAnalyzerContent(validImage)
			image["sha256"] = strings.Repeat("0", 64)
			analyzerFirstMessage(body)["content"] = []any{image}
		}},
		{name: "audio unsupported fields", mutateBody: func(body map[string]any) {
			audio := cloneAnalyzerContent(validAudio)
			audio["detail"] = "high"
			analyzerFirstMessage(body)["content"] = []any{audio}
		}},
		{name: "unsupported audio MIME", mutateBody: func(body map[string]any) {
			audio := cloneAnalyzerContent(validAudio)
			audio["mime_type"] = "audio/ogg"
			analyzerFirstMessage(body)["content"] = []any{audio}
		}},
		{name: "unknown provider", query: url.Values{"provider": []string{"unknown"}}},
		{name: "unknown model", mutateBody: func(body map[string]any) { body["model"] = "unknown-model" }},
		{name: "unsupported reasoning effort", mutateBody: func(body map[string]any) { body["reasoning_effort"] = "impossible" }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			queryValues := url.Values{}
			for parameterName, parameterValues := range testCase.query {
				for _, parameterValue := range parameterValues {
					queryValues.Add(parameterName, parameterValue)
				}
			}
			queryValues.Set("key", TestSecret)
			if !queryValues.Has("provider") {
				queryValues.Set("provider", proxy.ProviderNameOpenAI)
			}
			body := testCase.rawBody
			if body == nil {
				decodedBody := analyzerTextRequestMap(t)
				if testCase.mutateBody != nil {
					testCase.mutateBody(decodedBody)
				}
				var marshalError error
				body, marshalError = json.Marshal(decodedBody)
				if marshalError != nil {
					subTest.Fatalf("marshal body: %v", marshalError)
				}
			}
			request := httptest.NewRequest(http.MethodPost, "/v2/analyze?"+queryValues.Encode(), bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				subTest.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
		})
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("upstream calls=%d want=0", upstreamCalls.Load())
	}
}

func TestAnalyzerRouteRejectsOversizedBodyAndModelOutputLimitBeforeUpstream(t *testing.T) {
	t.Run("oversized body", func(subTest *testing.T) {
		router := coverageRouter(subTest, proxy.Configuration{
			Tenants:               proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey:             TestAPIKey,
			LogLevel:              proxy.LogLevelInfo,
			WorkerCount:           1,
			QueueSize:             1,
			RequestTimeoutSeconds: TestTimeout,
			MaxPromptBytes:        8,
		})
		request := httptest.NewRequest(http.MethodPost, "/v2/analyze?key="+url.QueryEscape(TestSecret), bytes.NewReader(analyzerTextRequestBody(subTest)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusRequestEntityTooLarge {
			subTest.Fatalf("status=%d body=%q", response.Code, response.Body.String())
		}
	})

	t.Run("model output limit", func(subTest *testing.T) {
		configuration, configurationError := newConfigurationWithCatalogs(subTest, proxy.Configuration{
			Tenants:   proxy.SingleTenantConfigurations("test", TestSecret),
			OpenAIKey: TestAPIKey,
		})
		if configurationError != nil {
			subTest.Fatalf("configuration: %v", configurationError)
		}
		openAICatalog := configuration.ProviderModels[proxy.ProviderNameOpenAI]
		for modelIndex := range openAICatalog.Text.Models {
			if openAICatalog.Text.Models[modelIndex].ID == proxy.ModelNameGPT55 {
				openAICatalog.Text.Models[modelIndex].OutputTokenLimit = 1
			}
		}
		configuration.ProviderModels[proxy.ProviderNameOpenAI] = openAICatalog
		configuration.WorkerCount = 1
		configuration.QueueSize = 1
		configuration.RequestTimeoutSeconds = TestTimeout
		router, buildError := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
		if buildError != nil {
			subTest.Fatalf(messageBuildRouterError, buildError)
		}
		body := analyzerTextRequestMap(subTest)
		body["max_tokens"] = 2
		bodyBytes, marshalError := json.Marshal(body)
		if marshalError != nil {
			subTest.Fatalf("marshal body: %v", marshalError)
		}
		request := httptest.NewRequest(http.MethodPost, "/v2/analyze?key="+url.QueryEscape(TestSecret), bytes.NewReader(bodyBytes))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			subTest.Fatalf("status=%d body=%q", response.Code, response.Body.String())
		}
	})
}

func analyzerRequestBody(t *testing.T, firstImage []byte, secondImage []byte) []byte {
	t.Helper()
	firstDigest := sha256.Sum256(firstImage)
	secondDigest := sha256.Sum256(secondImage)
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "system",
				"order":   0,
				"content": []any{map[string]any{"type": "text", "text": "Return the report."}},
			},
			map[string]any{
				"role":  "user",
				"order": 1,
				"content": []any{
					map[string]any{"type": "text", "text": "Inspect both frames in order."},
					map[string]any{
						"type":      "image",
						"mime_type": "image/png",
						"data":      base64.StdEncoding.EncodeToString(firstImage),
						"sha256":    hex.EncodeToString(firstDigest[:]),
						"detail":    "high",
					},
					map[string]any{
						"type":      "image",
						"mime_type": "image/jpeg",
						"data":      base64.StdEncoding.EncodeToString(secondImage),
						"sha256":    hex.EncodeToString(secondDigest[:]),
						"detail":    "low",
					},
				},
			},
		},
		"output_schema": map[string]any{
			"name":        "semantic_qa_report",
			"description": "One bounded semantic QA decision.",
			"schema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{"outcome"},
				"properties": map[string]any{
					"outcome": map[string]any{"type": "string", "enum": []any{"pass", "return"}},
				},
			},
		},
		"model":            proxy.ModelNameGPT55,
		"max_tokens":       1200,
		"reasoning_effort": "high",
	}
	bodyBytes, marshalError := json.Marshal(body)
	if marshalError != nil {
		t.Fatalf("marshal body: %v", marshalError)
	}
	return bodyBytes
}

func analyzerTextRequestBody(t *testing.T) []byte {
	t.Helper()
	bodyBytes, marshalError := json.Marshal(analyzerTextRequestMap(t))
	if marshalError != nil {
		t.Fatalf("marshal body: %v", marshalError)
	}
	return bodyBytes
}

func analyzerTextRequestMap(t *testing.T) map[string]any {
	t.Helper()
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": []any{map[string]any{"type": "text", "text": "Inspect the evidence."}},
			},
		},
		"output_schema": map[string]any{
			"name": "report",
			"schema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
			},
		},
		"model": proxy.ModelNameGPT55,
	}
	bodyBytes, marshalError := json.Marshal(body)
	if marshalError != nil {
		t.Fatalf("marshal body: %v", marshalError)
	}
	var copiedBody map[string]any
	if unmarshalError := json.Unmarshal(bodyBytes, &copiedBody); unmarshalError != nil {
		t.Fatalf("copy body: %v", unmarshalError)
	}
	return copiedBody
}

func analyzerFirstMessage(body map[string]any) map[string]any {
	return body["messages"].([]any)[0].(map[string]any)
}

func analyzerFirstContent(body map[string]any) map[string]any {
	return analyzerFirstMessage(body)["content"].([]any)[0].(map[string]any)
}

func analyzerBinaryContent(contentType string, mimeType string, data []byte) map[string]any {
	digest := sha256.Sum256(data)
	return map[string]any{
		"type":      contentType,
		"mime_type": mimeType,
		"data":      base64.StdEncoding.EncodeToString(data),
		"sha256":    hex.EncodeToString(digest[:]),
	}
}

func cloneAnalyzerContent(content map[string]any) map[string]any {
	clonedContent := make(map[string]any, len(content))
	for fieldName, fieldValue := range content {
		clonedContent[fieldName] = fieldValue
	}
	return clonedContent
}

func analyzerRouterWithResponsesHandler(t *testing.T, timeoutSeconds int, handler http.HandlerFunc) http.Handler {
	t.Helper()
	upstreamServer := httptest.NewServer(handler)
	t.Cleanup(upstreamServer.Close)
	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(upstreamServer.URL)
	return coverageRouter(t, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:             TestAPIKey,
		LogLevel:              proxy.LogLevelInfo,
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: timeoutSeconds,
		Endpoints:             endpoints,
	})
}

func performAnalyzerRequest(t *testing.T, router http.Handler, requestContext context.Context) (int, string) {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v2/analyze?key="+url.QueryEscape(TestSecret)+"&provider="+proxy.ProviderNameOpenAI,
		bytes.NewReader(analyzerTextRequestBody(t)),
	).WithContext(requestContext)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response.Code, response.Body.String()
}

func assertOpenAIImagePart(t *testing.T, rawPart any, mimeType string, expectedBytes []byte, detail string) {
	t.Helper()
	part, partOK := rawPart.(map[string]any)
	if !partOK {
		t.Fatalf("part=%v", rawPart)
	}
	expectedURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(expectedBytes)
	if part["type"] != "input_image" || part["image_url"] != expectedURL || part["detail"] != detail {
		t.Fatalf("part=%v", part)
	}
}
