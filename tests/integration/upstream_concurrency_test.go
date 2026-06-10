package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

const (
	concurrencyLongPrompt              = "long background request"
	concurrencyShortPrompt             = "short request"
	concurrencyLongResponse            = "LONG_OK"
	concurrencyShortResponse           = "SHORT_OK"
	concurrencyBackgroundResponseID    = "concurrency_background"
	concurrencyShortRequestTimeout     = 450 * time.Millisecond
	concurrencySignalAssertionTimeout  = time.Second
	concurrencyRequestTimeoutSeconds   = 3
	concurrencySingleUpstreamWorker    = 1
	concurrencySingleQueuedHTTPRequest = 1
)

type closeSignalBody struct {
	reader    *strings.Reader
	closeOnce sync.Once
	closed    chan<- struct{}
}

type concurrencyRoundTripper struct {
	mutex             sync.Mutex
	firstPollClosed   chan struct{}
	allowLongComplete <-chan struct{}
	longPollCount     int
}

type concurrencyPostResult struct {
	statusCode int
	requestErr error
}

func (body *closeSignalBody) Read(buffer []byte) (int, error) {
	return body.reader.Read(buffer)
}

func (body *closeSignalBody) Close() error {
	body.closeOnce.Do(func() {
		close(body.closed)
	})
	return nil
}

func (roundTripper *concurrencyRoundTripper) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	switch {
	case httpRequest.Method == http.MethodPost && httpRequest.URL.String() == mockResponsesURL:
		return roundTripper.handleInitialResponse(httpRequest)
	case httpRequest.Method == http.MethodGet && httpRequest.URL.String() == mockResponsesURL+"/"+concurrencyBackgroundResponseID:
		return roundTripper.handleLongPoll()
	default:
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}
}

func (roundTripper *concurrencyRoundTripper) handleInitialResponse(httpRequest *http.Request) (*http.Response, error) {
	requestBytes, readError := io.ReadAll(httpRequest.Body)
	if readError != nil {
		return nil, readError
	}
	var payload map[string]any
	if unmarshalError := json.Unmarshal(requestBytes, &payload); unmarshalError != nil {
		return nil, unmarshalError
	}
	inputText, _ := payload["input"].(string)
	if strings.Contains(inputText, concurrencyLongPrompt) {
		return jsonHTTPResponse(`{"id":"` + concurrencyBackgroundResponseID + `","status":"queued","background":true}`), nil
	}
	if strings.Contains(inputText, concurrencyShortPrompt) {
		return jsonHTTPResponse(`{"id":"concurrency_short","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + concurrencyShortResponse + `"}]}]}`), nil
	}
	return jsonHTTPResponse(`{"id":"concurrency_unknown","status":"completed","output_text":"UNKNOWN"}`), nil
}

func (roundTripper *concurrencyRoundTripper) handleLongPoll() (*http.Response, error) {
	roundTripper.mutex.Lock()
	roundTripper.longPollCount++
	pollCount := roundTripper.longPollCount
	roundTripper.mutex.Unlock()
	if pollCount == 1 {
		responseBody := &closeSignalBody{
			reader: strings.NewReader(`{"id":"` + concurrencyBackgroundResponseID + `","status":"in_progress"}`),
			closed: roundTripper.firstPollClosed,
		}
		return &http.Response{StatusCode: http.StatusOK, Body: responseBody, Header: make(http.Header)}, nil
	}
	select {
	case <-roundTripper.allowLongComplete:
	case <-time.After(concurrencySignalAssertionTimeout):
	}
	return jsonHTTPResponse(`{"id":"` + concurrencyBackgroundResponseID + `","status":"completed","output_text":"` + concurrencyLongResponse + `"}`), nil
}

func jsonHTTPResponse(responseBody string) *http.Response {
	responseHeader := make(http.Header)
	responseHeader.Set("Content-Type", contentTypeJSON)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(responseBody)), Header: responseHeader}
}

func TestIntegrationBackgroundPollSleepDoesNotOccupyUpstreamWorker(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	firstPollClosed := make(chan struct{})
	allowLongComplete := make(chan struct{})
	roundTripper := &concurrencyRoundTripper{
		firstPollClosed:   firstPollClosed,
		allowLongComplete: allowLongComplete,
	}
	endpoints := proxy.NewEndpoints()
	configureProxy(testingInstance, &http.Client{Transport: roundTripper}, endpoints)
	router, buildError := proxy.BuildRouter(integrationConfiguration(testingInstance, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("integration", serviceSecretValue),
		OpenAIKey:             openAIKeyValue,
		LogLevel:              logLevelDebug,
		WorkerCount:           concurrencySingleUpstreamWorker,
		QueueSize:             concurrencySingleQueuedHTTPRequest,
		RequestTimeoutSeconds: concurrencyRequestTimeoutSeconds,
		Endpoints:             endpoints,
	}), newLogger(testingInstance))
	if buildError != nil {
		testingInstance.Fatalf(buildRouterFailedFormat, buildError)
	}
	applicationServer := httptest.NewServer(router)
	testingInstance.Cleanup(applicationServer.Close)

	longDone := make(chan concurrencyPostResult, 1)
	go func() {
		statusCode, requestError := performConcurrencyPost(applicationServer.URL, concurrencyLongPrompt)
		longDone <- concurrencyPostResult{statusCode: statusCode, requestErr: requestError}
	}()

	select {
	case <-firstPollClosed:
	case <-time.After(concurrencySignalAssertionTimeout):
		testingInstance.Fatal("first background poll did not complete")
	}

	shortClient := &http.Client{Timeout: concurrencyShortRequestTimeout}
	shortURL, _ := url.Parse(applicationServer.URL)
	shortQueryValues := shortURL.Query()
	shortQueryValues.Set(promptQueryParameter, concurrencyShortPrompt)
	shortQueryValues.Set(keyQueryParameter, serviceSecretValue)
	shortURL.RawQuery = shortQueryValues.Encode()
	shortResponse, shortRequestError := shortClient.Get(shortURL.String())
	if shortRequestError != nil {
		testingInstance.Fatalf("short request failed before background poll sleep released worker: %v", shortRequestError)
	}
	defer shortResponse.Body.Close()
	shortResponseBytes, _ := io.ReadAll(shortResponse.Body)
	if shortResponse.StatusCode != http.StatusOK {
		testingInstance.Fatalf("short status=%d body=%s", shortResponse.StatusCode, string(shortResponseBytes))
	}
	if string(shortResponseBytes) != concurrencyShortResponse {
		testingInstance.Fatalf(bodyMismatchFormat, string(shortResponseBytes), concurrencyShortResponse)
	}
	select {
	case longResult := <-longDone:
		if longResult.requestErr != nil {
			testingInstance.Fatalf("long request failed before the short request proved concurrent progress: %v", longResult.requestErr)
		}
		testingInstance.Fatalf("long request completed before the short request proved concurrent progress: status=%d", longResult.statusCode)
	default:
	}

	close(allowLongComplete)
	select {
	case longResult := <-longDone:
		if longResult.requestErr != nil {
			testingInstance.Fatalf("long request failed: %v", longResult.requestErr)
		}
		if longResult.statusCode != http.StatusOK {
			testingInstance.Fatalf("long status=%d want=%d", longResult.statusCode, http.StatusOK)
		}
	case <-time.After(concurrencySignalAssertionTimeout):
		testingInstance.Fatal("long request did not complete")
	}
}

func performConcurrencyPost(applicationURL string, prompt string) (int, error) {
	requestURL, _ := url.Parse(applicationURL)
	queryValues := requestURL.Query()
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	requestURL.RawQuery = queryValues.Encode()
	requestPayload := map[string]any{
		"prompt": prompt,
		"model":  proxy.ModelNameGPT41,
	}
	requestBytes, marshalError := json.Marshal(requestPayload)
	if marshalError != nil {
		return 0, marshalError
	}
	httpResponse, requestError := http.Post(requestURL.String(), contentTypeJSON, bytes.NewReader(requestBytes))
	if requestError != nil {
		return 0, requestError
	}
	defer httpResponse.Body.Close()
	return httpResponse.StatusCode, nil
}
