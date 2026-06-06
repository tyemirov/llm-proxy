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
)

const (
	modelsListBody               = `{"data":[{"id":"` + proxy.ModelNameGPT41 + `"}]}`
	expectedResponseBody         = "SLOW_OK"
	requestTimeoutSecondsDefault = 40
	responseReadyAssertion       = "response returned before upstream release"
	responseReleaseTimeout       = 2 * time.Second
)

// makeControlledResponseHTTPClient returns an HTTP client whose response waits for explicit release.
func makeControlledResponseHTTPClient(testingInstance *testing.T, endpoints *proxy.Endpoints, upstreamRequestStarted chan<- struct{}, releaseResponse <-chan struct{}) *http.Client {
	testingInstance.Helper()
	var closeStartedOnce sync.Once
	return &http.Client{
		Transport: roundTripperFunc(func(httpRequest *http.Request) (*http.Response, error) {
			switch {
			case httpRequest.URL.String() == endpoints.GetModelsURL():
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(modelsListBody)), Header: make(http.Header)}, nil
			case strings.HasPrefix(httpRequest.URL.String(), endpoints.GetModelsURL()+"/"):
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(metadataTemperatureTools)), Header: make(http.Header)}, nil
			case httpRequest.URL.String() == endpoints.GetResponsesURL():
				closeStartedOnce.Do(func() { close(upstreamRequestStarted) })
				select {
				case <-httpRequest.Context().Done():
					return nil, httpRequest.Context().Err()
				case <-releaseResponse:
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"output_text":"` + expectedResponseBody + `"}`)), Header: make(http.Header)}, nil
				}
			default:
				testingInstance.Fatalf(unexpectedRequestFormat, httpRequest.URL.String())
				return nil, nil
			}
		}),
	}
}

// TestIntegrationResponseDeliveredAfterUpstreamRelease verifies pending upstream responses are delivered before the configured request timeout.
func TestIntegrationResponseDeliveredAfterUpstreamRelease(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct{ name string }{{name: "delayed_response"}}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			upstreamRequestStarted := make(chan struct{})
			releaseResponse := make(chan struct{})
			responseReturned := make(chan *http.Response, 1)
			requestErrors := make(chan error, 1)
			endpoints := proxy.NewEndpoints()
			configureProxy(subTest, makeControlledResponseHTTPClient(subTest, endpoints, upstreamRequestStarted, releaseResponse), endpoints)
			router, buildError := proxy.BuildRouter(proxy.Configuration{Tenants: proxy.SingleTenantConfigurations("integration", serviceSecretValue), OpenAIKey: openAIKeyValue, LogLevel: logLevelDebug, WorkerCount: 1, QueueSize: 8, RequestTimeoutSeconds: requestTimeoutSecondsDefault, Endpoints: endpoints}, newLogger(subTest))
			if buildError != nil {
				subTest.Fatalf(buildRouterFailedFormat, buildError)
			}
			server := httptest.NewServer(router)
			subTest.Cleanup(server.Close)
			requestURL, _ := url.Parse(server.URL)
			queryValues := requestURL.Query()
			queryValues.Set(promptQueryParameter, promptValue)
			queryValues.Set(keyQueryParameter, serviceSecretValue)
			requestURL.RawQuery = queryValues.Encode()

			go func() {
				httpResponse, requestError := server.Client().Get(requestURL.String())
				if requestError != nil {
					requestErrors <- requestError
					return
				}
				responseReturned <- httpResponse
			}()

			select {
			case <-upstreamRequestStarted:
			case <-time.After(responseReleaseTimeout):
				subTest.Fatal("upstream response request was not observed")
			}
			select {
			case httpResponse := <-responseReturned:
				if httpResponse != nil {
					httpResponse.Body.Close()
				}
				subTest.Fatal(responseReadyAssertion)
			case requestError := <-requestErrors:
				subTest.Fatalf(getFailedFormat, requestError)
			default:
			}
			close(releaseResponse)

			var httpResponse *http.Response
			select {
			case httpResponse = <-responseReturned:
			case requestError := <-requestErrors:
				subTest.Fatalf(getFailedFormat, requestError)
			case <-time.After(responseReleaseTimeout):
				subTest.Fatal("released upstream response was not delivered")
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != http.StatusOK {
				subTest.Fatalf(statusWantFormat, httpResponse.StatusCode, http.StatusOK)
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if string(responseBytes) != expectedResponseBody {
				subTest.Fatalf(bodyMismatchFormat, string(responseBytes), expectedResponseBody)
			}
		})
	}
}
