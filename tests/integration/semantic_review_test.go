package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

const (
	semanticReviewMinimumPromptBytes  = 31 * 1024
	semanticReviewRequiredOutputLimit = 8192
	semanticReviewAcceptedResponse    = `{"decisions":[{"target":"stress","accepted":true}]}`
	semanticReviewPromptPathEnv       = "LLM_PROXY_SEMANTIC_REVIEW_PROMPT_PATH"
)

type semanticReviewCapture struct {
	mutex              sync.Mutex
	initialPromptBytes int
	initialModel       string
	initialOutputLimit int
	initialBackground  bool
	initialStore       bool
	requestCount       int
	pollCount          int
}

func (capture *semanticReviewCapture) record(payload map[string]any) {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	capture.requestCount++
	if _, hasPreviousResponse := payload["previous_response_id"]; hasPreviousResponse {
		return
	}
	capture.initialModel, _ = payload["model"].(string)
	promptText, _ := payload["input"].(string)
	capture.initialPromptBytes = len([]byte(promptText))
	outputLimit, _ := payload["max_output_tokens"].(float64)
	capture.initialOutputLimit = int(outputLimit)
	capture.initialBackground, _ = payload["background"].(bool)
	capture.initialStore, _ = payload["store"].(bool)
}

func (capture *semanticReviewCapture) recordPoll() {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	capture.pollCount++
}

func (capture *semanticReviewCapture) snapshot() semanticReviewCapture {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	return semanticReviewCapture{
		initialPromptBytes: capture.initialPromptBytes,
		initialModel:       capture.initialModel,
		initialOutputLimit: capture.initialOutputLimit,
		initialBackground:  capture.initialBackground,
		initialStore:       capture.initialStore,
		requestCount:       capture.requestCount,
		pollCount:          capture.pollCount,
	}
}

func largeSemanticReviewPrompt(testingInstance *testing.T) string {
	testingInstance.Helper()
	promptPath := strings.TrimSpace(os.Getenv(semanticReviewPromptPathEnv))
	if promptPath != "" {
		promptBytes, readError := os.ReadFile(promptPath)
		if readError != nil {
			testingInstance.Fatalf("read semantic review prompt from %s: %v", promptPath, readError)
		}
		return string(promptBytes)
	}

	var promptBuilder strings.Builder
	promptBuilder.WriteString("Return decisions JSON for semantic stress review. Preserve source bytes exactly and only adjust stress coverage.\n")
	for promptBuilder.Len() < semanticReviewMinimumPromptBytes {
		promptBuilder.WriteString("source_token=belaya_utochka_review_target stress_target=required keep_boundary_whitespace=true\n")
	}
	return promptBuilder.String()
}

func newSemanticReviewOpenAIServer(testingInstance *testing.T, capture *semanticReviewCapture) *httptest.Server {
	testingInstance.Helper()
	return httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		responseWriter.Header().Set("Content-Type", contentTypeJSON)
		switch {
		case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == integrationResponsesPath:
			requestBytes, readError := io.ReadAll(httpRequest.Body)
			if readError != nil {
				testingInstance.Fatalf("read upstream request: %v", readError)
			}
			var payload map[string]any
			if unmarshalError := json.Unmarshal(requestBytes, &payload); unmarshalError != nil {
				testingInstance.Fatalf("unmarshal upstream request: %v", unmarshalError)
			}
			capture.record(payload)
			outputLimit, _ := payload["max_output_tokens"].(float64)
			if int(outputLimit) < semanticReviewRequiredOutputLimit {
				responseIdentifier := "semantic_review_initial"
				if _, hasPreviousResponse := payload["previous_response_id"]; hasPreviousResponse {
					responseIdentifier = "semantic_review_continued"
				}
				_, _ = responseWriter.Write([]byte(`{"id":"` + responseIdentifier + `","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`))
				return
			}
			encodedResponseText, marshalError := json.Marshal(semanticReviewAcceptedResponse)
			if marshalError != nil {
				testingInstance.Fatalf("marshal upstream response text: %v", marshalError)
			}
			_, _ = responseWriter.Write([]byte(`{"id":"semantic_review_complete","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":` + string(encodedResponseText) + `}]}]}`))
		case httpRequest.Method == http.MethodGet && strings.HasPrefix(httpRequest.URL.Path, integrationResponsesPath+"/"):
			_, _ = responseWriter.Write([]byte(`{"id":"semantic_review_continued","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`))
		default:
			http.NotFound(responseWriter, httpRequest)
		}
	}))
}

func TestIntegrationLargeSemanticReviewPostUsesRequestMaxTokens(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &semanticReviewCapture{}
	openAIServer := newSemanticReviewOpenAIServer(testingInstance, capture)
	testingInstance.Cleanup(openAIServer.Close)

	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(openAIServer.URL + integrationResponsesPath)
	originalClient := proxy.HTTPClient
	proxy.HTTPClient = openAIServer.Client()
	testingInstance.Cleanup(func() { proxy.HTTPClient = originalClient })
	router, buildError := proxy.BuildRouter(integrationConfiguration(testingInstance, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("integration", serviceSecretValue),
		OpenAIKey:                  openAIKeyValue,
		LogLevel:                   logLevelDebug,
		WorkerCount:                1,
		QueueSize:                  8,
		RequestTimeoutSeconds:      requestTimeoutSecondsDefault,
		UpstreamPollTimeoutSeconds: requestTimeoutSecondsDefault,
		Endpoints:                  endpoints,
	}), newLogger(testingInstance))
	if buildError != nil {
		testingInstance.Fatalf(buildRouterFailedFormat, buildError)
	}
	applicationServer := httptest.NewServer(router)
	testingInstance.Cleanup(applicationServer.Close)

	requestURL, _ := url.Parse(applicationServer.URL)
	queryValues := requestURL.Query()
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	requestURL.RawQuery = queryValues.Encode()
	requestPayload := map[string]any{
		"prompt":     largeSemanticReviewPrompt(testingInstance),
		"model":      proxy.ModelNameGPT55Pro,
		"web_search": false,
		"max_tokens": semanticReviewRequiredOutputLimit,
	}
	requestBytes, marshalError := json.Marshal(requestPayload)
	if marshalError != nil {
		testingInstance.Fatalf("marshal request: %v", marshalError)
	}
	httpResponse, requestError := http.Post(requestURL.String(), contentTypeJSON, bytes.NewReader(requestBytes))
	if requestError != nil {
		testingInstance.Fatalf(requestErrorFormat, requestError)
	}
	defer httpResponse.Body.Close()
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	captured := capture.snapshot()

	if httpResponse.StatusCode != http.StatusOK {
		testingInstance.Fatalf(
			"status=%d body=%s initial_model=%s initial_prompt_bytes=%d initial_max_output_tokens=%d upstream_requests=%d",
			httpResponse.StatusCode,
			string(responseBytes),
			captured.initialModel,
			captured.initialPromptBytes,
			captured.initialOutputLimit,
			captured.requestCount,
		)
	}
	if string(responseBytes) != semanticReviewAcceptedResponse {
		testingInstance.Fatalf(bodyMismatchFormat, string(responseBytes), semanticReviewAcceptedResponse)
	}
	if captured.initialModel != proxy.ModelNameGPT55Pro {
		testingInstance.Fatalf("model=%s want=%s", captured.initialModel, proxy.ModelNameGPT55Pro)
	}
	if captured.initialPromptBytes < semanticReviewMinimumPromptBytes {
		testingInstance.Fatalf("prompt_bytes=%d want>=%d", captured.initialPromptBytes, semanticReviewMinimumPromptBytes)
	}
	if captured.initialOutputLimit < semanticReviewRequiredOutputLimit {
		testingInstance.Fatalf("max_output_tokens=%d want>=%d", captured.initialOutputLimit, semanticReviewRequiredOutputLimit)
	}
	if !captured.initialBackground || !captured.initialStore {
		testingInstance.Fatalf("background=%v store=%v want true", captured.initialBackground, captured.initialStore)
	}
}

func TestIntegrationLargeSemanticReviewPostPollsBackgroundOpenAIResponse(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &semanticReviewCapture{}
	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		responseWriter.Header().Set("Content-Type", contentTypeJSON)
		switch {
		case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == integrationResponsesPath:
			requestBytes, readError := io.ReadAll(httpRequest.Body)
			if readError != nil {
				testingInstance.Fatalf("read upstream request: %v", readError)
			}
			var payload map[string]any
			if unmarshalError := json.Unmarshal(requestBytes, &payload); unmarshalError != nil {
				testingInstance.Fatalf("unmarshal upstream request: %v", unmarshalError)
			}
			capture.record(payload)
			_, _ = responseWriter.Write([]byte(`{"id":"semantic_review_background","status":"queued","background":true}`))
		case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == integrationResponsesPath+"/semantic_review_background":
			capture.recordPoll()
			encodedResponseText, marshalError := json.Marshal(semanticReviewAcceptedResponse)
			if marshalError != nil {
				testingInstance.Fatalf("marshal response text: %v", marshalError)
			}
			_, _ = responseWriter.Write([]byte(`{"id":"semantic_review_background","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":` + string(encodedResponseText) + `}]}]}`))
		default:
			http.NotFound(responseWriter, httpRequest)
		}
	}))
	testingInstance.Cleanup(openAIServer.Close)

	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(openAIServer.URL + integrationResponsesPath)
	originalClient := proxy.HTTPClient
	proxy.HTTPClient = openAIServer.Client()
	testingInstance.Cleanup(func() { proxy.HTTPClient = originalClient })
	router, buildError := proxy.BuildRouter(integrationConfiguration(testingInstance, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("integration", serviceSecretValue),
		OpenAIKey:                  openAIKeyValue,
		LogLevel:                   logLevelDebug,
		WorkerCount:                1,
		QueueSize:                  8,
		RequestTimeoutSeconds:      3,
		UpstreamPollTimeoutSeconds: 3,
		Endpoints:                  endpoints,
	}), newLogger(testingInstance))
	if buildError != nil {
		testingInstance.Fatalf(buildRouterFailedFormat, buildError)
	}
	applicationServer := httptest.NewServer(router)
	testingInstance.Cleanup(applicationServer.Close)

	requestURL, _ := url.Parse(applicationServer.URL)
	queryValues := requestURL.Query()
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	requestURL.RawQuery = queryValues.Encode()
	requestPayload := map[string]any{
		"prompt":     largeSemanticReviewPrompt(testingInstance),
		"model":      proxy.ModelNameGPT55Pro,
		"web_search": false,
		"max_tokens": semanticReviewRequiredOutputLimit,
	}
	requestBytes, marshalError := json.Marshal(requestPayload)
	if marshalError != nil {
		testingInstance.Fatalf("marshal request: %v", marshalError)
	}
	httpResponse, requestError := http.Post(requestURL.String(), contentTypeJSON, bytes.NewReader(requestBytes))
	if requestError != nil {
		testingInstance.Fatalf(requestErrorFormat, requestError)
	}
	defer httpResponse.Body.Close()
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	captured := capture.snapshot()

	if httpResponse.StatusCode != http.StatusOK {
		testingInstance.Fatalf("status=%d body=%s poll_count=%d", httpResponse.StatusCode, string(responseBytes), captured.pollCount)
	}
	if string(responseBytes) != semanticReviewAcceptedResponse {
		testingInstance.Fatalf(bodyMismatchFormat, string(responseBytes), semanticReviewAcceptedResponse)
	}
	if !captured.initialBackground || !captured.initialStore {
		testingInstance.Fatalf("background=%v store=%v want true", captured.initialBackground, captured.initialStore)
	}
	if captured.pollCount == 0 {
		testingInstance.Fatal("background response was not polled")
	}
}

func TestIntegrationLargeSemanticReviewPostReturnsResumeTokenAndStoredResponseCompletes(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &semanticReviewCapture{}
	var completionMutex sync.Mutex
	completeStoredResponse := false
	openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		responseWriter.Header().Set("Content-Type", contentTypeJSON)
		switch {
		case httpRequest.Method == http.MethodPost && httpRequest.URL.Path == integrationResponsesPath:
			requestBytes, readError := io.ReadAll(httpRequest.Body)
			if readError != nil {
				testingInstance.Fatalf("read upstream request: %v", readError)
			}
			var payload map[string]any
			if unmarshalError := json.Unmarshal(requestBytes, &payload); unmarshalError != nil {
				testingInstance.Fatalf("unmarshal upstream request: %v", unmarshalError)
			}
			capture.record(payload)
			_, _ = responseWriter.Write([]byte(`{"id":"semantic_review_background","status":"queued","background":true}`))
		case httpRequest.Method == http.MethodGet && httpRequest.URL.Path == integrationResponsesPath+"/semantic_review_background":
			capture.recordPoll()
			completionMutex.Lock()
			shouldComplete := completeStoredResponse
			completionMutex.Unlock()
			if !shouldComplete {
				_, _ = responseWriter.Write([]byte(`{"id":"semantic_review_background","status":"in_progress"}`))
				return
			}
			encodedResponseText, marshalError := json.Marshal(semanticReviewAcceptedResponse)
			if marshalError != nil {
				testingInstance.Fatalf("marshal response text: %v", marshalError)
			}
			_, _ = responseWriter.Write([]byte(`{"id":"semantic_review_background","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":` + string(encodedResponseText) + `}]}]}`))
		default:
			http.NotFound(responseWriter, httpRequest)
		}
	}))
	testingInstance.Cleanup(openAIServer.Close)

	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(openAIServer.URL + integrationResponsesPath)
	originalClient := proxy.HTTPClient
	proxy.HTTPClient = openAIServer.Client()
	testingInstance.Cleanup(func() { proxy.HTTPClient = originalClient })
	router, buildError := proxy.BuildRouter(integrationConfiguration(testingInstance, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("integration", serviceSecretValue),
		OpenAIKey:                  openAIKeyValue,
		LogLevel:                   logLevelDebug,
		WorkerCount:                1,
		QueueSize:                  8,
		RequestTimeoutSeconds:      3,
		UpstreamPollTimeoutSeconds: 1,
		Endpoints:                  endpoints,
	}), newLogger(testingInstance))
	if buildError != nil {
		testingInstance.Fatalf(buildRouterFailedFormat, buildError)
	}
	applicationServer := httptest.NewServer(router)
	testingInstance.Cleanup(applicationServer.Close)

	requestURL, _ := url.Parse(applicationServer.URL)
	queryValues := requestURL.Query()
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	requestURL.RawQuery = queryValues.Encode()
	requestPayload := map[string]any{
		"prompt":     largeSemanticReviewPrompt(testingInstance),
		"model":      proxy.ModelNameGPT55Pro,
		"web_search": false,
		"max_tokens": semanticReviewRequiredOutputLimit,
	}
	requestBytes, marshalError := json.Marshal(requestPayload)
	if marshalError != nil {
		testingInstance.Fatalf("marshal request: %v", marshalError)
	}
	httpResponse, requestError := http.Post(requestURL.String(), contentTypeJSON, bytes.NewReader(requestBytes))
	if requestError != nil {
		testingInstance.Fatalf(requestErrorFormat, requestError)
	}
	defer httpResponse.Body.Close()
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	if httpResponse.StatusCode != http.StatusGatewayTimeout {
		testingInstance.Fatalf("status=%d want=%d body=%s", httpResponse.StatusCode, http.StatusGatewayTimeout, string(responseBytes))
	}
	responseIdentifier := httpResponse.Header.Get("X-LLM-Proxy-Upstream-Response-ID")
	if responseIdentifier != "semantic_review_background" {
		testingInstance.Fatalf("resume response id=%q want semantic_review_background", responseIdentifier)
	}
	if resumeProvider := httpResponse.Header.Get("X-LLM-Proxy-Resume-Provider"); resumeProvider != proxy.ProviderNameOpenAI {
		testingInstance.Fatalf("resume provider=%q want %s", resumeProvider, proxy.ProviderNameOpenAI)
	}

	completionMutex.Lock()
	completeStoredResponse = true
	completionMutex.Unlock()
	resumeURL, _ := url.Parse(applicationServer.URL + "/responses/" + responseIdentifier)
	resumeQueryValues := resumeURL.Query()
	resumeQueryValues.Set(keyQueryParameter, serviceSecretValue)
	resumeQueryValues.Set("provider", proxy.ProviderNameOpenAI)
	resumeQueryValues.Set("format", "text/plain")
	resumeURL.RawQuery = resumeQueryValues.Encode()
	resumeResponse, resumeError := http.Get(resumeURL.String())
	if resumeError != nil {
		testingInstance.Fatalf(requestErrorFormat, resumeError)
	}
	defer resumeResponse.Body.Close()
	resumeBytes, _ := io.ReadAll(resumeResponse.Body)
	if resumeResponse.StatusCode != http.StatusOK {
		testingInstance.Fatalf("resume status=%d body=%s", resumeResponse.StatusCode, string(resumeBytes))
	}
	if string(resumeBytes) != semanticReviewAcceptedResponse {
		testingInstance.Fatalf(bodyMismatchFormat, string(resumeBytes), semanticReviewAcceptedResponse)
	}
}
