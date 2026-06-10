package llm_proxy_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

type roundTripperFunc func(httpRequest *http.Request) (*http.Response, error)

const (
	modelsURL    = "https://mock.local/v1/models"
	responsesURL = "https://mock.local/v1/responses"
)

func (roundTripper roundTripperFunc) RoundTrip(httpRequest *http.Request) (*http.Response, error) {
	return roundTripper(httpRequest)
}

// newRouterWithStubbedOpenAI returns a router that uses a stubbed OpenAI backend.
func newRouterWithStubbedOpenAI(testingInstance *testing.T, modelsBody, responsesBody string, workerCount, queueSize, requestTimeoutSeconds int) *gin.Engine {
	testingInstance.Helper()

	endpoints := proxy.NewEndpoints()
	endpoints.SetModelsURL(modelsURL)
	endpoints.SetResponsesURL(responsesURL)

	originalClient := proxy.HTTPClient
	testingInstance.Cleanup(func() { proxy.HTTPClient = originalClient })

	proxy.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			switch request.URL.String() {
			case endpoints.GetModelsURL():
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(modelsBody)),
					Header:     make(http.Header),
				}, nil
			case endpoints.GetResponsesURL():
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(responsesBody)),
					Header:     make(http.Header),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{}`)),
					Header:     make(http.Header),
				}, nil
			}
		}),
		Timeout: 5 * time.Second,
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	router, buildError := proxy.BuildRouter(testfixtures.WithProviderModelCatalogs(testingInstance, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("test", "sekret"),
		OpenAIKey:             "sk-test",
		LogLevel:              "debug",
		WorkerCount:           workerCount,
		QueueSize:             queueSize,
		RequestTimeoutSeconds: requestTimeoutSeconds,
		Endpoints:             endpoints,
	}), logger.Sugar())
	if buildError != nil {
		testingInstance.Fatalf("BuildRouter error: %v", buildError)
	}
	return router
}

// TestEndpoint_Empty200TreatedAsError ensures that empty successful responses are treated as errors.
func TestEndpoint_Empty200TreatedAsError(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newRouterWithStubbedOpenAI(
		testingInstance,
		`{"data":[{"id":"`+proxy.ModelNameGPT41+`"}]}`,
		`{"output":[]}`,
		1,
		4,
		5,
	)

	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	request, _ := http.NewRequest("GET", server.URL+"/?prompt=test&key=sekret", nil)
	response, requestError := http.DefaultClient.Do(request)
	if requestError != nil {
		testingInstance.Fatalf("request failed: %v", requestError)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusBadGateway {
		testingInstance.Fatalf("status=%d want=%d", response.StatusCode, http.StatusBadGateway)
	}
}

// TestEndpoint_RespectsAcceptHeaderCSV validates CSV responses when the Accept header requests text/csv.
func TestEndpoint_RespectsAcceptHeaderCSV(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newRouterWithStubbedOpenAI(
		testingInstance,
		`{"data":[{"id":"`+proxy.ModelNameGPT41+`"}]}`,
		`{"output_text":"Hello, world!"}`,
		1,
		4,
		5,
	)

	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	request, _ := http.NewRequest("GET", server.URL+"/?prompt=anything&key=sekret", nil)
	request.Header.Set("Accept", "text/csv")
	response, requestError := http.DefaultClient.Do(request)
	if requestError != nil {
		testingInstance.Fatalf("request failed: %v", requestError)
	}
	defer response.Body.Close()

	if contentType := response.Header.Get("Content-Type"); contentType != "text/csv" {
		testingInstance.Fatalf("content-type=%q want=%q", contentType, "text/csv")
	}
	responseBody, _ := io.ReadAll(response.Body)
	if body := string(responseBody); body != "\"Hello, world!\"\n" {
		testingInstance.Fatalf("body=%q want=%q", body, "\"Hello, world!\"\n")
	}
}

// TestEndpoint_ReturnsServiceUnavailableWhenQueueFull confirms that a full upstream HTTP queue results in an HTTP 503 status code.
func TestEndpoint_ReturnsServiceUnavailableWhenQueueFull(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	endpoints := proxy.NewEndpoints()
	endpoints.SetModelsURL(modelsURL)
	endpoints.SetResponsesURL(responsesURL)
	upstreamStarted := make(chan struct{})
	releaseUpstream := make(chan struct{})
	var closeStartedOnce sync.Once
	originalClient := proxy.HTTPClient
	testingInstance.Cleanup(func() { proxy.HTTPClient = originalClient })
	proxy.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			switch request.URL.String() {
			case endpoints.GetModelsURL():
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"` + proxy.ModelNameGPT41 + `"}]}`)),
					Header:     make(http.Header),
				}, nil
			case endpoints.GetResponsesURL():
				closeStartedOnce.Do(func() { close(upstreamStarted) })
				select {
				case <-request.Context().Done():
					return nil, request.Context().Err()
				case <-releaseUpstream:
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"output_text":"queued"}`)),
						Header:     make(http.Header),
					}, nil
				}
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{}`)),
					Header:     make(http.Header),
				}, nil
			}
		}),
		Timeout: 5 * time.Second,
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	router, buildError := proxy.BuildRouter(testfixtures.WithProviderModelCatalogs(testingInstance, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("test", "sekret"),
		OpenAIKey:             "sk-test",
		LogLevel:              "debug",
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: 1,
		Endpoints:             endpoints,
	}), logger.Sugar())
	if buildError != nil {
		testingInstance.Fatalf("BuildRouter error: %v", buildError)
	}

	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	firstRequest, _ := http.NewRequest("GET", server.URL+"/?prompt=first&key=sekret", nil)
	go func() {
		firstResponse, requestError := http.DefaultClient.Do(firstRequest)
		if requestError == nil {
			_ = firstResponse.Body.Close()
		}
	}()
	select {
	case <-upstreamStarted:
	case <-time.After(time.Second):
		testingInstance.Fatal("upstream request did not start")
	}

	statuses := make(chan int, 2)
	for requestIndex := 0; requestIndex < 2; requestIndex++ {
		go func() {
			requestContext, cancelRequest := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancelRequest()
			request, _ := http.NewRequestWithContext(requestContext, "GET", server.URL+"/?prompt=queued&key=sekret", nil)
			response, requestError := http.DefaultClient.Do(request)
			if requestError != nil {
				statuses <- 0
				return
			}
			defer response.Body.Close()
			statuses <- response.StatusCode
		}()
	}

	queueFullObserved := false
	for receivedStatusCount := 0; receivedStatusCount < 2 && !queueFullObserved; receivedStatusCount++ {
		select {
		case status := <-statuses:
			queueFullObserved = status == http.StatusServiceUnavailable
		case <-time.After(time.Second):
			testingInstance.Fatal("queue full response was not observed")
		}
	}
	if !queueFullObserved {
		testingInstance.Fatal("queue full response was not observed")
	}
	close(releaseUpstream)
}

// TestEndpoint_ReturnsGatewayTimeoutWhenWaitingForUpstreamWorker confirms that admitted upstream HTTP waits respect the request timeout.
func TestEndpoint_ReturnsGatewayTimeoutWhenWaitingForUpstreamWorker(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	endpoints := proxy.NewEndpoints()
	endpoints.SetModelsURL(modelsURL)
	endpoints.SetResponsesURL(responsesURL)
	upstreamStarted := make(chan struct{})
	releaseUpstream := make(chan struct{})
	var closeStartedOnce sync.Once
	originalClient := proxy.HTTPClient
	testingInstance.Cleanup(func() { proxy.HTTPClient = originalClient })
	proxy.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			switch request.URL.String() {
			case endpoints.GetModelsURL():
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"` + proxy.ModelNameGPT41 + `"}]}`)),
					Header:     make(http.Header),
				}, nil
			case endpoints.GetResponsesURL():
				closeStartedOnce.Do(func() { close(upstreamStarted) })
				select {
				case <-request.Context().Done():
					return nil, request.Context().Err()
				case <-releaseUpstream:
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"output_text":"queued"}`)),
						Header:     make(http.Header),
					}, nil
				}
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{}`)),
					Header:     make(http.Header),
				}, nil
			}
		}),
		Timeout: 5 * time.Second,
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	router, buildError := proxy.BuildRouter(testfixtures.WithProviderModelCatalogs(testingInstance, proxy.Configuration{
		Tenants:               proxy.SingleTenantConfigurations("test", "sekret"),
		OpenAIKey:             "sk-test",
		LogLevel:              "debug",
		WorkerCount:           1,
		QueueSize:             1,
		RequestTimeoutSeconds: 1,
		Endpoints:             endpoints,
	}), logger.Sugar())
	if buildError != nil {
		testingInstance.Fatalf("BuildRouter error: %v", buildError)
	}

	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)

	firstRequest, _ := http.NewRequest("GET", server.URL+"/?prompt=first&key=sekret", nil)
	go func() {
		firstResponse, requestError := http.DefaultClient.Do(firstRequest)
		if requestError == nil {
			_ = firstResponse.Body.Close()
		}
	}()
	select {
	case <-upstreamStarted:
	case <-time.After(time.Second):
		testingInstance.Fatal("upstream request did not start")
	}

	waitingRequest, _ := http.NewRequest("GET", server.URL+"/?prompt=waiting&key=sekret", nil)
	waitingResponse, waitingRequestError := http.DefaultClient.Do(waitingRequest)
	if waitingRequestError != nil {
		testingInstance.Fatalf("request failed: %v", waitingRequestError)
	}
	defer waitingResponse.Body.Close()
	if waitingResponse.StatusCode != http.StatusGatewayTimeout {
		testingInstance.Fatalf("status=%d want=%d", waitingResponse.StatusCode, http.StatusGatewayTimeout)
	}
	time.Sleep(50 * time.Millisecond)
	close(releaseUpstream)
}
