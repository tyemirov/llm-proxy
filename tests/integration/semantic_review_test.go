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
	requestCount       int
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
}

func (capture *semanticReviewCapture) snapshot() semanticReviewCapture {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	return semanticReviewCapture{
		initialPromptBytes: capture.initialPromptBytes,
		initialModel:       capture.initialModel,
		initialOutputLimit: capture.initialOutputLimit,
		requestCount:       capture.requestCount,
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
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("integration", serviceSecretValue),
		OpenAIKey:                  openAIKeyValue,
		LogLevel:                   logLevelDebug,
		WorkerCount:                1,
		QueueSize:                  8,
		RequestTimeoutSeconds:      requestTimeoutSecondsDefault,
		UpstreamPollTimeoutSeconds: requestTimeoutSecondsDefault,
		Endpoints:                  endpoints,
	}, newLogger(testingInstance))
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
}
