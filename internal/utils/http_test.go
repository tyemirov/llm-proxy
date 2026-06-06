package utils_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/utils"
)

const (
	httpMethodGet      = "GET"
	requestURLExample  = "http://example.com"
	headerNameExample  = "X-Test-Header"
	headerValueExample = "header-value"
	invalidRequestURL  = "://bad-url"
	bodyContent        = "body"
)

type buildHTTPRequestTestDefinition struct {
	testName            string
	method              string
	requestURL          string
	headers             map[string]string
	expectError         bool
	expectedHeaderValue string
}

// TestBuildHTTPRequestWithHeaders_ConstructsRequests verifies that BuildHTTPRequestWithHeaders creates requests and applies headers.
func TestBuildHTTPRequestWithHeaders_ConstructsRequests(testingInstance *testing.T) {
	testCases := []buildHTTPRequestTestDefinition{
		{
			testName:            "valid request",
			method:              httpMethodGet,
			requestURL:          requestURLExample,
			headers:             map[string]string{headerNameExample: headerValueExample},
			expectError:         false,
			expectedHeaderValue: headerValueExample,
		},
		{
			testName:    "invalid url",
			method:      httpMethodGet,
			requestURL:  invalidRequestURL,
			headers:     map[string]string{},
			expectError: true,
		},
	}
	for _, currentTestCase := range testCases {
		testingInstance.Run(currentTestCase.testName, func(nestedTestingInstance *testing.T) {
			httpRequest, buildRequestError := utils.BuildHTTPRequestWithHeaders(currentTestCase.method, currentTestCase.requestURL, bytes.NewBufferString(bodyContent), currentTestCase.headers)
			if currentTestCase.expectError {
				if buildRequestError == nil {
					nestedTestingInstance.Fatalf("expected error but got none")
				}
				return
			}
			if buildRequestError != nil {
				nestedTestingInstance.Fatalf("unexpected error: %v", buildRequestError)
			}
			headerValue := httpRequest.Header.Get(headerNameExample)
			if headerValue != currentTestCase.expectedHeaderValue {
				nestedTestingInstance.Fatalf("header value=%s expected=%s", headerValue, currentTestCase.expectedHeaderValue)
			}
		})
	}
}

func TestPerformHTTPRequest_ReturnsBodyResetError(testingInstance *testing.T) {
	httpRequest, buildRequestError := http.NewRequest(httpMethodGet, requestURLExample, nil)
	if buildRequestError != nil {
		testingInstance.Fatalf("build request: %v", buildRequestError)
	}
	requestContext, cancelRequest := context.WithTimeout(httpRequest.Context(), time.Millisecond)
	defer cancelRequest()
	httpRequest = httpRequest.WithContext(requestContext)
	httpRequest.GetBody = func() (io.ReadCloser, error) {
		return nil, context.Canceled
	}
	_, _, _, performError := utils.PerformHTTPRequest(func(*http.Request) (*http.Response, error) {
		testingInstance.Fatalf("executeRequest should not run")
		return nil, nil
	}, httpRequest, nil, "transport error")
	if performError == nil {
		testingInstance.Fatalf("performError=nil want non-nil")
	}
}
