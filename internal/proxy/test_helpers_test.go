package proxy_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

// Test constants used across the entire test suite for this package.
const (
	TestJobID               = "resp_test_12345"
	messageBuildRouterError = "BuildRouter error: %v"
)

// NewSessionMockServer creates a reusable httptest.Server that correctly
// simulates the multi-step session flow for the Responses API.
func NewSessionMockServer(finalResponseJSON string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		// 1. Handle the initial POST to create the session.
		if httpRequest.Method == http.MethodPost && httpRequest.URL.Path == "/" {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(fmt.Sprintf(`{"id": "%s", "status": "queued"}`, TestJobID)))
			return
		}
		// 2. Handle the subsequent GET to poll the session.
		if httpRequest.Method == http.MethodGet && strings.HasSuffix(httpRequest.URL.Path, TestJobID) {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(finalResponseJSON))
			return
		}
		// 3. Handle a "continue" POST if a test requires it.
		if httpRequest.Method == http.MethodPost && strings.HasSuffix(httpRequest.URL.Path, "/continue") {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"status": "in_progress"}`)) // Acknowledge the continue request
			return
		}
		http.NotFound(responseWriter, httpRequest)
	}))
}

// NewTestRouter creates a pre-configured router for integration tests.
func NewTestRouter(t *testing.T, serverURL string) *gin.Engine {
	t.Helper()
	endpoints := proxy.NewEndpoints()
	endpoints.SetResponsesURL(serverURL)

	logger, _ := zap.NewDevelopment()
	t.Cleanup(func() { _ = logger.Sync() })

	router, err := buildRouterWithCatalogs(t, proxy.Configuration{
		Tenants:                    proxy.SingleTenantConfigurations("test", TestSecret),
		OpenAIKey:                  TestAPIKey,
		LogLevel:                   proxy.LogLevelDebug,
		WorkerCount:                1,
		QueueSize:                  1,
		RequestTimeoutSeconds:      TestTimeout,
		UpstreamPollTimeoutSeconds: TestTimeout,
		Endpoints:                  endpoints,
	}, logger.Sugar())
	if err != nil {
		t.Fatalf("BuildRouter error: %v", err)
	}
	return router
}

func buildRouterWithCatalogs(testingInstance testing.TB, configuration proxy.Configuration, structuredLogger *zap.SugaredLogger) (*gin.Engine, error) {
	testingInstance.Helper()
	return proxy.BuildRouter(withProviderModelCatalogs(testingInstance, configuration), structuredLogger)
}

func newConfigurationWithCatalogs(testingInstance testing.TB, configuration proxy.Configuration) (proxy.Configuration, error) {
	testingInstance.Helper()
	return proxy.NewConfiguration(withProviderModelCatalogs(testingInstance, configuration))
}

func withProviderModelCatalogs(testingInstance testing.TB, configuration proxy.Configuration) proxy.Configuration {
	testingInstance.Helper()
	if len(configuration.ProviderModels) == 0 {
		configuration.ProviderModels = testfixtures.ProviderModelCatalogs(testingInstance)
	}
	return configuration
}
