package integration_test

import (
	"context"
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
	// requestTimeoutSeconds is the proxy request timeout used for queue saturation.
	requestTimeoutSeconds = 1
	// singleWorkerCount specifies the number of workers used in this test.
	singleWorkerCount = 1
	// singleQueueSlot specifies the number of queued requests used in this test.
	singleQueueSlot = 1
	// queueSaturationRequestCount is the number of concurrent requests needed to fill one worker, one queue slot, and one rejected request.
	queueSaturationRequestCount = singleWorkerCount + singleQueueSlot + 1
	// queueAssertionTimeout bounds synchronization failures without driving the tested behavior.
	queueAssertionTimeout = 2 * time.Second
	// queueGatewayTimeout bounds queue-full requests through the inbound request context.
	queueGatewayTimeout = 25 * time.Millisecond
	// queueFullCountFormat reports the number of queue-full responses.
	queueFullCountFormat = "queue_full=%d"
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
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"output_text":"` + integrationOKBody + `"}`)), Header: make(http.Header)}, nil
			}
		default:
			testingInstance.Fatalf(unexpectedRequestFormat, httpRequest.URL.String())
			return nil, nil
		}
	})}
}

// TestIntegrationHighLoadQueue verifies queue saturation handling.
func TestIntegrationHighLoadQueue(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	endpoints := proxy.NewEndpoints()
	upstreamRequestStarted := make(chan struct{})
	releaseResponses := make(chan struct{})
	client := makeBlockingHTTPClient(testingInstance, endpoints, upstreamRequestStarted, releaseResponses)
	configureProxy(testingInstance, client, endpoints)
	router, buildRouterError := proxy.BuildRouter(proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("integration", serviceSecretValue),
		OpenAIKey:             openAIKeyValue,
		LogLevel:              logLevelDebug,
		WorkerCount:           singleWorkerCount,
		QueueSize:             singleQueueSlot,
		RequestTimeoutSeconds: requestTimeoutSeconds,
		Endpoints:             endpoints,
	}, newLogger(testingInstance))
	if buildRouterError != nil {
		testingInstance.Fatalf(buildRouterFailedFormat, buildRouterError)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)
	queueServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		requestContext, cancelRequest := context.WithTimeout(httpRequest.Context(), queueGatewayTimeout)
		defer cancelRequest()
		router.ServeHTTP(responseWriter, httpRequest.WithContext(requestContext))
	}))
	testingInstance.Cleanup(queueServer.Close)
	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	requestURL.RawQuery = queryValues.Encode()
	queueRequestURL, _ := url.Parse(queueServer.URL)
	queueQueryValues := queueRequestURL.Query()
	queueQueryValues.Set(promptQueryParameter, promptValue)
	queueQueryValues.Set(keyQueryParameter, serviceSecretValue)
	queueRequestURL.RawQuery = queueQueryValues.Encode()

	statuses := make(chan int, queueSaturationRequestCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(queueSaturationRequestCount)
	go func() {
		defer waitGroup.Done()
		statuses <- performQueueRequest(requestURL.String())
	}()
	<-upstreamRequestStarted
	for requestIndex := 1; requestIndex < queueSaturationRequestCount; requestIndex++ {
		go func() {
			defer waitGroup.Done()
			statuses <- performQueueRequest(queueRequestURL.String())
		}()
	}
	queueFullCount := waitForQueueFullResponse(testingInstance, statuses)
	close(releaseResponses)
	waitGroup.Wait()
	close(statuses)

	for status := range statuses {
		if status == http.StatusServiceUnavailable {
			queueFullCount++
		}
	}
	if queueFullCount != 1 {
		testingInstance.Fatalf(queueFullCountFormat, queueFullCount)
	}
}

func performQueueRequest(requestURL string) int {
	httpResponse, requestError := http.Get(requestURL)
	if requestError != nil {
		return 0
	}
	defer httpResponse.Body.Close()
	return httpResponse.StatusCode
}

func waitForQueueFullResponse(testingInstance *testing.T, statuses <-chan int) int {
	testingInstance.Helper()
	for receivedStatusCount := 0; receivedStatusCount < queueSaturationRequestCount-1; receivedStatusCount++ {
		select {
		case status := <-statuses:
			if status == http.StatusServiceUnavailable {
				return 1
			}
		case <-time.After(queueAssertionTimeout):
			testingInstance.Fatal("queue-full response was not observed")
		}
	}
	testingInstance.Fatal("queue-full response was not observed")
	return 0
}
