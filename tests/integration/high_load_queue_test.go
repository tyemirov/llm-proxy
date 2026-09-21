package integration_test

import (
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
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	// singleWorkerCount specifies the number of workers used in this test.
	singleWorkerCount = 1
	// singleQueueSlot specifies the number of queued requests used in this test.
	singleQueueSlot = 1
	// queueAssertionTimeout bounds synchronization failures without driving the tested behavior.
	queueAssertionTimeout = 2 * time.Second
)

// makeBlockingHTTPClient returns an HTTP client that keeps the first upstream response pending until released.
func makeBlockingHTTPClient(testingInstance *testing.T, endpoints *proxy.Endpoints, upstreamRequestStarted chan<- struct{}, releaseResponses <-chan struct{}) *http.Client {
	testingInstance.Helper()
	var closeStartedOnce sync.Once
	return &http.Client{Transport: roundTripperFunc(func(httpRequest *http.Request) (*http.Response, error) {
		switch {
		case httpRequest.URL.String() == endpoints.GetModelsURL():
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(availableModelsBody)), Header: make(http.Header)}, nil
		case strings.HasPrefix(httpRequest.URL.String(), endpoints.GetModelsURL()+"/"):
			modelIdentifier := strings.TrimPrefix(httpRequest.URL.Path, integrationModelsPath+"/")
			metadata := metadataEmpty
			if modelIdentifier == proxy.ModelNameGPT41 {
				metadata = metadataTemperatureTools
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(metadata)), Header: make(http.Header)}, nil
		case httpRequest.URL.String() == endpoints.GetResponsesURL():
			closeStartedOnce.Do(func() { close(upstreamRequestStarted) })
			select {
			case <-httpRequest.Context().Done():
				return nil, httpRequest.Context().Err()
			case <-releaseResponses:
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":"completed","output_text":"` + integrationOKBody + `"}`)), Header: make(http.Header)}, nil
			}
		default:
			testingInstance.Fatalf(unexpectedRequestFormat, httpRequest.URL.String())
			return nil, nil
		}
	})}
}

// TestIntegrationHighLoadQueue verifies queue saturation through real HTTP requests.
func TestIntegrationHighLoadQueue(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	endpoints := proxy.NewEndpoints()
	upstreamRequestStarted := make(chan struct{})
	queuedRequestObserved := make(chan struct{})
	releaseResponses := make(chan struct{})
	var releaseOnce, queuedOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseResponses) }) }
	defer release()
	observedCore, observedLogs := observer.New(zap.InfoLevel)
	logger := zap.New(observedCore, zap.Hooks(func(entry zapcore.Entry) error {
		if entry.Message == "upstream HTTP admission" && observedLogs.FilterMessage(entry.Message).FilterField(zap.String("decision", "capacity_wait")).Len() > 0 {
			queuedOnce.Do(func() { close(queuedRequestObserved) })
		}
		return nil
	})).Sugar()
	client := makeBlockingHTTPClient(testingInstance, endpoints, upstreamRequestStarted, releaseResponses)
	configureProxy(testingInstance, client, endpoints)
	router, buildRouterError := buildIntegrationRouter(testingInstance, proxy.Configuration{
		LogLevel:         logLevelDebug,
		UpstreamCapacity: testfixtures.UpstreamCapacity(singleWorkerCount, singleQueueSlot),
		Endpoints:        endpoints,
	}, logger)
	if buildRouterError != nil {
		testingInstance.Fatalf(buildRouterFailedFormat, buildRouterError)
	}
	observedLogs.TakeAll()
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)
	testingInstance.Cleanup(release)
	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	requestURL.RawQuery = queryValues.Encode()

	type result struct {
		status int
		body   string
		err    error
	}
	request := func(results chan<- result) {
		response, err := server.Client().Get(requestURL.String())
		if err != nil {
			results <- result{err: err}
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		results <- result{status: response.StatusCode, body: string(body), err: err}
	}
	awaitSignal := func(signal <-chan struct{}, description string) {
		testingInstance.Helper()
		select {
		case <-signal:
		case <-time.After(queueAssertionTimeout):
			testingInstance.Fatalf("did not observe %s", description)
		}
	}
	awaitResult := func(results <-chan result, status int) {
		testingInstance.Helper()
		select {
		case response := <-results:
			if response.err != nil || response.status != status {
				testingInstance.Fatalf("status=%d want=%d body=%s error=%v", response.status, status, response.body, response.err)
			}
			if status == http.StatusOK && response.body != integrationOKBody {
				testingInstance.Fatalf("admitted response=%q want=%q", response.body, integrationOKBody)
			}
		case <-time.After(queueAssertionTimeout):
			testingInstance.Fatalf("did not receive HTTP %d", status)
		}
	}

	admitted := make(chan result, singleWorkerCount+singleQueueSlot)
	go request(admitted)
	awaitSignal(upstreamRequestStarted, "active upstream request")
	go request(admitted)
	awaitSignal(queuedRequestObserved, "queued request")

	excess := make(chan result, 1)
	go request(excess)
	awaitResult(excess, http.StatusServiceUnavailable)
	release()
	for index := 0; index < singleWorkerCount+singleQueueSlot; index++ {
		awaitResult(admitted, http.StatusOK)
	}
	if rejected := observedLogs.FilterMessage("upstream HTTP admission").FilterField(zap.String("decision", "rejected")).Len(); rejected != 1 {
		testingInstance.Fatalf("rejected admissions=%d want=1", rejected)
	}
}
