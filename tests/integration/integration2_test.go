package integration_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

const webSearchQueryParameter = "web_search"

// TestClientResponseDelivery validates responses with and without web search.
func TestClientResponseDelivery(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct {
		name       string
		webSearch  bool
		expected   string
		checkTools bool
	}{
		{name: "plain", webSearch: false, expected: integrationOKBody},
		{name: "web_search", webSearch: true, expected: integrationSearchBody, checkTools: true},
	}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			endpoints := proxy.NewEndpoints()
			client, captured := makeHTTPClient(subTest, testCase.webSearch, endpoints)
			configureProxy(subTest, client, endpoints)
			router, buildRouterError := buildIntegrationRouter(subTest, proxy.Configuration{
				LogLevel:    logLevelDebug,
				WorkerCount: 1,
				QueueSize:   8,
				Endpoints:   endpoints,
			}, newLogger(subTest))
			if buildRouterError != nil {
				subTest.Fatalf(buildRouterFailedFormat, buildRouterError)
			}
			server := httptest.NewServer(router)
			subTest.Cleanup(server.Close)
			requestURL, _ := url.Parse(server.URL)
			queryValues := requestURL.Query()
			queryValues.Set(promptQueryParameter, promptValue)
			queryValues.Set(keyQueryParameter, serviceSecretValue)
			if testCase.webSearch {
				queryValues.Set(webSearchQueryParameter, "true")
			}
			requestURL.RawQuery = queryValues.Encode()
			httpResponse, requestError := http.Get(requestURL.String())
			if requestError != nil {
				subTest.Fatalf(getFailedFormat, requestError)
			}
			defer httpResponse.Body.Close()
			if httpResponse.StatusCode != http.StatusOK {
				subTest.Fatalf(statusWantFormat, httpResponse.StatusCode, http.StatusOK)
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if string(responseBytes) != testCase.expected {
				subTest.Fatalf(bodyMismatchFormat, string(responseBytes), testCase.expected)
			}
			if testCase.checkTools {
				tools, ok := (*captured)["tools"].([]any)
				if !ok || len(tools) == 0 {
					subTest.Fatalf(toolsMissingFormat, *captured)
				}
				first, _ := tools[0].(map[string]any)
				if first["type"] != "web_search" {
					subTest.Fatalf(toolTypeMismatchFormat, first["type"])
				}
			}
		})
	}
}

// TestIntegrationConfiguration rejects an unregistered client key.
func TestIntegrationConfiguration(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	testingInstance.Run("wrong_key", func(subTest *testing.T) {
		endpoints := proxy.NewEndpoints()
		client, _ := makeHTTPClient(subTest, false, endpoints)
		configureProxy(subTest, client, endpoints)
		config := proxy.Configuration{LogLevel: logLevelDebug, WorkerCount: 1, QueueSize: 4}
		config.Endpoints = endpoints
		router, buildRouterError := buildIntegrationRouter(subTest, config, newLogger(subTest))
		if buildRouterError != nil {
			subTest.Fatalf(buildRouterFailedFormat, buildRouterError)
		}
		server := httptest.NewServer(router)
		subTest.Cleanup(server.Close)
		requestURL, _ := url.Parse(server.URL)
		queryValues := requestURL.Query()
		queryValues.Set(promptQueryParameter, promptValue)
		queryValues.Set(keyQueryParameter, "wrong")
		requestURL.RawQuery = queryValues.Encode()
		httpResponse, requestError := http.Get(requestURL.String())
		if requestError != nil {
			subTest.Fatalf(getFailedFormat, requestError)
		}
		defer httpResponse.Body.Close()
		if httpResponse.StatusCode != http.StatusForbidden {
			var bodyBuffer bytes.Buffer
			_, _ = io.Copy(&bodyBuffer, httpResponse.Body)
			subTest.Fatalf(statusWantBodyFormat, httpResponse.StatusCode, http.StatusForbidden, bodyBuffer.String())
		}
	})
}
