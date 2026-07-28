package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const malformedJSONPayload = "invalid"

type integrationProviderErrorEnvelope struct {
	Error struct {
		Code           string  `json:"code"`
		Provider       string  `json:"provider"`
		UpstreamStatus *int    `json:"upstream_status"`
		Retryable      bool    `json:"retryable"`
		RequestID      string  `json:"request_id"`
		RetryAfter     *string `json:"retry_after"`
	} `json:"error"`
}

// newMalformedOpenAIServer returns a stub OpenAI server emitting invalid JSON for the responses endpoint.
func newMalformedOpenAIServer(testingInstance *testing.T) *httptest.Server {
	testingInstance.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch httpRequest.URL.Path {
		case integrationModelsPath:
			responseWriter.Header().Set("Content-Type", contentTypeJSON)
			_, _ = io.WriteString(responseWriter, integrationModelListBody)
		case integrationResponsesPath:
			responseWriter.Header().Set("Content-Type", contentTypeJSON)
			_, _ = io.WriteString(responseWriter, malformedJSONPayload)
		default:
			http.NotFound(responseWriter, httpRequest)
		}
	}))
	return server
}

// TestOpenAIMalformedJSON verifies that the proxy returns a 502 error when the upstream responds with invalid JSON.
func TestOpenAIMalformedJSON(testingInstance *testing.T) {
	openAIServer := newMalformedOpenAIServer(testingInstance)
	testingInstance.Cleanup(openAIServer.Close)
	applicationServer := newIntegrationServer(testingInstance, openAIServer)
	requestURL, _ := url.Parse(applicationServer.URL)
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	queryValues.Set(keyQueryParameter, integrationServiceSecret)
	requestURL.RawQuery = queryValues.Encode()
	httpResponse, requestError := http.Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf(requestErrorFormat, requestError)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusBadGateway {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		testingInstance.Fatalf(unexpectedStatusFormat, httpResponse.StatusCode, string(responseBody))
	}
	responseBytes, _ := io.ReadAll(httpResponse.Body)
	var errorEnvelope integrationProviderErrorEnvelope
	if decodeError := json.Unmarshal(responseBytes, &errorEnvelope); decodeError != nil {
		testingInstance.Fatalf("decode provider error: %v body=%s", decodeError, responseBytes)
	}
	requestID := httpResponse.Header.Get(llmproxycontract.HeaderRequestID)
	if errorEnvelope.Error.Code != llmproxycontract.ErrorCodeProviderError ||
		errorEnvelope.Error.Provider != proxy.ProviderNameOpenAI ||
		errorEnvelope.Error.UpstreamStatus != nil ||
		errorEnvelope.Error.Retryable ||
		errorEnvelope.Error.RequestID == "" ||
		errorEnvelope.Error.RequestID != requestID ||
		errorEnvelope.Error.RetryAfter != nil {
		testingInstance.Fatalf("provider error=%+v request_id_header=%q", errorEnvelope.Error, requestID)
	}
	if strings.Contains(string(responseBytes), malformedJSONPayload) {
		testingInstance.Fatalf("provider error exposed upstream body: %s", responseBytes)
	}
}
