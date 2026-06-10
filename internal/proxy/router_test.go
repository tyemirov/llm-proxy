package proxy_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

// chatHandlerScenario defines a single test scenario for model validation.
type chatHandlerScenario struct {
	scenarioName       string
	modelIdentifier    string
	expectedStatusCode int
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

func TestChatHandlerContinuesIncompleteGPT55JSONBody(testingInstance *testing.T) {
	const finalResponse = `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"continued ok"}]}]}`
	const incompleteResponseID = "resp_incomplete_gpt55"
	const continuedResponseID = "resp_continued_gpt55"

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
			if capturedPayload["previous_response_id"] == nil {
				_, _ = responseWriter.Write([]byte(`{"id":"` + incompleteResponseID + `","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"reasoning","summary":[]},{"type":"web_search_call","status":"incomplete"}]}`))
				return
			}
			_, _ = responseWriter.Write([]byte(`{"id":"` + continuedResponseID + `","status":"queued"}`))
			return
		}
		if httpRequest.Method == http.MethodGet && strings.HasSuffix(httpRequest.URL.Path, continuedResponseID) {
			_, _ = responseWriter.Write([]byte(finalResponse))
			return
		}
		http.NotFound(responseWriter, httpRequest)
	}))
	defer mockServer.Close()

	router := NewTestRouter(testingInstance, mockServer.URL)
	requestBody := bytes.NewBufferString(`{"prompt":"search current model facts","model":"` + proxy.ModelNameGPT55 + `","web_search":true}`)
	request := httptest.NewRequest(http.MethodPost, "/?key="+TestSecret, requestBody)
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()

	router.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		testingInstance.Fatalf("status=%d body=%q", responseRecorder.Code, responseRecorder.Body.String())
	}
	if responseRecorder.Body.String() != "continued ok" {
		testingInstance.Fatalf("body=%q want continued ok", responseRecorder.Body.String())
	}
	if len(capturedPayloads) != 2 {
		testingInstance.Fatalf("payloads=%d want=2", len(capturedPayloads))
	}
	if capturedPayloads[0]["model"] != proxy.ModelNameGPT55 {
		testingInstance.Fatalf("initial model=%v want=%s", capturedPayloads[0]["model"], proxy.ModelNameGPT55)
	}
	if capturedPayloads[1]["model"] != proxy.ModelNameGPT55 {
		testingInstance.Fatalf("continued model=%v want=%s", capturedPayloads[1]["model"], proxy.ModelNameGPT55)
	}
	if capturedPayloads[1]["previous_response_id"] != incompleteResponseID {
		testingInstance.Fatalf("previous_response_id=%v want=%s", capturedPayloads[1]["previous_response_id"], incompleteResponseID)
	}
	if _, found := capturedPayloads[1]["tools"]; !found {
		testingInstance.Fatalf("continued tools missing: %v", capturedPayloads[1])
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
