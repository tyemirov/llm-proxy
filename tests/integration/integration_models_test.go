package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

// TestIntegrationModelSpecSuppression verifies that certain fields are suppressed for mini models.
func TestIntegrationModelSpecSuppression(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct {
		name  string
		model string
	}{{name: "gpt_5_mini", model: proxy.ModelNameGPT5Mini}}
	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			endpoints := proxy.NewEndpoints()
			client, captured := makeHTTPClient(subTest, true, endpoints)
			configureProxy(subTest, client, endpoints)
			router, buildRouterError := proxy.BuildRouter(integrationConfiguration(subTest, proxy.Configuration{
				Tenants:     proxy.SingleTenantConfigurations("integration", serviceSecretValue),
				OpenAIKey:   openAIKeyValue,
				LogLevel:    logLevelDebug,
				WorkerCount: 1,
				QueueSize:   8,
				Endpoints:   endpoints,
			}), newLogger(subTest))
			if buildRouterError != nil {
				subTest.Fatalf(buildRouterFailedFormat, buildRouterError)
			}
			server := httptest.NewServer(router)
			subTest.Cleanup(server.Close)
			requestURL, _ := url.Parse(server.URL)
			queryValues := requestURL.Query()
			queryValues.Set(promptQueryParameter, promptValue)
			queryValues.Set(keyQueryParameter, serviceSecretValue)
			queryValues.Set(adaptiveModelQueryParameter, testCase.model)
			requestURL.RawQuery = queryValues.Encode()
			httpResponse, requestError := http.Get(requestURL.String())
			if requestError != nil {
				subTest.Fatalf(getFailedFormat, requestError)
			}
			defer httpResponse.Body.Close()
			_, _ = io.ReadAll(httpResponse.Body)
			if httpResponse.StatusCode != http.StatusOK {
				subTest.Fatalf("status=%d want=%d", httpResponse.StatusCode, http.StatusOK)
			}
			payload := *captured
			if _, ok := payload["temperature"]; ok {
				subTest.Fatalf(temperatureOmittedFormat, testCase.model, payload["temperature"])
			}
			if _, ok := payload["tools"]; ok {
				subTest.Fatalf(toolsOmittedFormat, testCase.model, payload["tools"])
			}
			if _, hasInput := payload["input"]; !hasInput {
				subTest.Fatalf("input must be present for responses API")
			}
			if _, hasMessages := payload["messages"]; hasMessages {
				subTest.Fatalf("messages must not be present for responses API payload")
			}
			time.Sleep(10 * time.Millisecond)
		})
	}
}

// TestIntegrationModelCatalogRejectsUnsupportedWebSearch verifies configured model capability validation.
func TestIntegrationModelCatalogRejectsUnsupportedWebSearch(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	router, buildRouterError := proxy.BuildRouter(integrationConfiguration(testingInstance, proxy.Configuration{
		Tenants:     proxy.SingleTenantConfigurations("integration", serviceSecretValue),
		OpenAIKey:   openAIKeyValue,
		LogLevel:    logLevelDebug,
		WorkerCount: 1,
		QueueSize:   8,
	}), newLogger(testingInstance))
	if buildRouterError != nil {
		testingInstance.Fatalf(buildRouterFailedFormat, buildRouterError)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)
	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	queryValues.Set(webSearchQueryParameter, "1")
	queryValues.Set(adaptiveModelQueryParameter, proxy.ModelNameGPT5Mini)
	requestURL.RawQuery = queryValues.Encode()
	httpResponse, requestError := http.Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf(getFailedFormat, requestError)
	}
	defer httpResponse.Body.Close()
	responseBody, _ := io.ReadAll(httpResponse.Body)
	if httpResponse.StatusCode != http.StatusBadRequest {
		testingInstance.Fatalf("status=%d body=%q", httpResponse.StatusCode, string(responseBody))
	}
	if !strings.Contains(string(responseBody), "unsupported provider capability") {
		testingInstance.Fatalf("body=%q want unsupported provider capability", string(responseBody))
	}
}

// TestIntegrationGPT5TemperatureSuppression verifies that temperature is omitted and tools retained for GPT-5.
func TestIntegrationGPT5TemperatureSuppression(testingInstance *testing.T) {
	gin.SetMode(gin.TestMode)
	endpoints := proxy.NewEndpoints()
	client, captured := makeHTTPClient(testingInstance, true, endpoints)
	configureProxy(testingInstance, client, endpoints)
	router, buildRouterError := proxy.BuildRouter(integrationConfiguration(testingInstance, proxy.Configuration{
		Tenants:     proxy.SingleTenantConfigurations("integration", serviceSecretValue),
		OpenAIKey:   openAIKeyValue,
		LogLevel:    logLevelDebug,
		WorkerCount: 1,
		QueueSize:   8,
		Endpoints:   endpoints,
	}), newLogger(testingInstance))
	if buildRouterError != nil {
		testingInstance.Fatalf(buildRouterFailedFormat, buildRouterError)
	}
	server := httptest.NewServer(router)
	testingInstance.Cleanup(server.Close)
	requestURL, _ := url.Parse(server.URL)
	queryValues := requestURL.Query()
	queryValues.Set(promptQueryParameter, promptValue)
	queryValues.Set(keyQueryParameter, serviceSecretValue)
	queryValues.Set(webSearchQueryParameter, "1")
	queryValues.Set(adaptiveModelQueryParameter, proxy.ModelNameGPT5)
	requestURL.RawQuery = queryValues.Encode()
	httpResponse, requestError := http.Get(requestURL.String())
	if requestError != nil {
		testingInstance.Fatalf(getFailedFormat, requestError)
	}
	defer httpResponse.Body.Close()
	_, _ = io.ReadAll(httpResponse.Body)
	payload := *captured
	if _, ok := payload["temperature"]; ok {
		testingInstance.Fatalf(temperatureOmittedFormat, proxy.ModelNameGPT5, payload["temperature"])
	}
	if _, ok := payload["tools"]; !ok {
		testingInstance.Fatal(toolsMissingMessage)
	}
	time.Sleep(10 * time.Millisecond)
}
