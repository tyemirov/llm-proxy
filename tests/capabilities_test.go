package tests_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

// Constants used in tests.
const (
	modelIDGPT4o                   = proxy.ModelNameGPT4o
	modelIDGPT4oMini               = proxy.ModelNameGPT4oMini
	modelIDGPT5Mini                = proxy.ModelNameGPT5Mini
	serviceSecret                  = "sekret"
	openAIKey                      = "sk-test"
	logLevel                       = "debug"
	openAIResponsesPath            = "/v1/responses"
	openAIResponseTemplate         = `{"status":"completed","output_text":"%s"}`
	responseTextWithoutTools       = "NO_TOOLS_OK"
	responseTextWithoutTemperature = "TEMPLESS_OK"
)

// TestIntegration_OmitsDisallowedParameters confirms that metadata disallowed fields are removed from requests.
func TestIntegration_OmitsDisallowedParameters(testingInstance *testing.T) {
	testCases := []struct {
		testName         string
		modelIdentifier  string
		additionalQuery  string
		expectedResponse string
		disallowedFields []string
	}{
		{
			testName:         "temperature omitted",
			modelIdentifier:  modelIDGPT5Mini,
			additionalQuery:  "",
			expectedResponse: responseTextWithoutTemperature,
			disallowedFields: []string{"temperature"},
		},
		{
			testName:         "tools omitted",
			modelIdentifier:  modelIDGPT4oMini,
			additionalQuery:  "",
			expectedResponse: responseTextWithoutTools,
			disallowedFields: []string{"tools", "tool_choice"},
		},
	}

	for _, testCase := range testCases {
		currentTestCase := testCase
		testingInstance.Run(currentTestCase.testName, func(subTestInstance *testing.T) {
			var observed any

			openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
				switch {
				case strings.HasSuffix(httpRequest.URL.Path, openAIResponsesPath):
					body, _ := io.ReadAll(httpRequest.Body)
					_ = json.Unmarshal(body, &observed)
					io.WriteString(responseWriter, fmt.Sprintf(openAIResponseTemplate, currentTestCase.expectedResponse))
				default:
					http.NotFound(responseWriter, httpRequest)
				}
			}))
			defer openAIServer.Close()

			endpoints := proxy.NewEndpoints()
			endpoints.SetResponsesURL(openAIServer.URL + openAIResponsesPath)
			originalClient := proxy.HTTPClient
			proxy.HTTPClient = openAIServer.Client()
			subTestInstance.Cleanup(func() { proxy.HTTPClient = originalClient })

			logger, _ := zap.NewDevelopment()
			defer logger.Sync()

			router, buildRouterError := proxy.BuildRouter(testfixtures.WithProviderModelCatalogs(subTestInstance, proxy.Configuration{
				Tenants:     proxy.SingleTenantConfigurations("capabilities", serviceSecret),
				OpenAIKey:   openAIKey,
				LogLevel:    logLevel,
				WorkerCount: 1,
				QueueSize:   4,
				Endpoints:   endpoints,
			}), logger.Sugar())
			if buildRouterError != nil {
				subTestInstance.Fatalf("BuildRouter error: %v", buildRouterError)
			}

			applicationServer := httptest.NewServer(router)
			defer applicationServer.Close()

			httpResponse, requestError := http.Get(applicationServer.URL + "/?prompt=hello&key=" + serviceSecret + "&model=" + currentTestCase.modelIdentifier + currentTestCase.additionalQuery)
			if requestError != nil {
				subTestInstance.Fatalf("request failed: %v", requestError)
			}
			defer httpResponse.Body.Close()

			if httpResponse.StatusCode != http.StatusOK {
				responseBody, _ := io.ReadAll(httpResponse.Body)
				subTestInstance.Fatalf("status=%d body=%s", httpResponse.StatusCode, string(responseBody))
			}
			if payload, ok := observed.(map[string]any); ok {
				for _, fieldName := range currentTestCase.disallowedFields {
					if _, found := payload[fieldName]; found {
						subTestInstance.Fatalf("%s present in payload: %v", fieldName, payload)
					}
				}
			}
			responseBytes, _ := io.ReadAll(httpResponse.Body)
			if strings.TrimSpace(string(responseBytes)) != currentTestCase.expectedResponse {
				subTestInstance.Fatalf("body=%q want %q", string(responseBytes), currentTestCase.expectedResponse)
			}
		})
	}
}

func TestPublicCapabilityCatalogProjectsValidatedRuntimeRegistry(testingInstance *testing.T) {
	catalog, catalogError := proxy.NewPublicCapabilityCatalog(proxy.Configuration{
		ProviderModels: testfixtures.ProviderModelCatalogs(testingInstance),
	})
	if catalogError != nil {
		testingInstance.Fatalf("NewPublicCapabilityCatalog error: %v", catalogError)
	}
	if len(catalog.Providers) != 12 || catalog.MaxPromptBytes != proxy.DefaultMaxPromptBytes || catalog.MaxInputAudioBytes != proxy.DefaultMaxInputAudioBytes {
		testingInstance.Fatalf("catalog summary=%+v", catalog)
	}
	for _, provider := range catalog.Providers {
		if provider.Identifier != proxy.ProviderNameGemini {
			continue
		}
		for _, model := range provider.TextModels {
			if model.Identifier == proxy.ModelNameGemini35Flash {
				if strings.Join(model.MediaInputs, ",") != "audio,image" {
					testingInstance.Fatalf("Gemini media inputs=%v", model.MediaInputs)
				}
				return
			}
		}
	}
	testingInstance.Fatal("Gemini 3.5 Flash public capability missing")
}

func TestPublicCapabilityCatalogRejectsNoncanonicalRuntimeRegistries(testingInstance *testing.T) {
	testCases := []struct {
		name          string
		mutate        func(proxy.ProviderModelCatalogs)
		expectedError string
	}{
		{
			name: "noncanonical provider identifier",
			mutate: func(catalogs proxy.ProviderModelCatalogs) {
				catalogs["OpenAI"] = catalogs[proxy.ProviderNameOpenAI]
			},
			expectedError: "reason=not_canonical",
		},
		{
			name: "unknown provider identifier",
			mutate: func(catalogs proxy.ProviderModelCatalogs) {
				catalogs["future"] = catalogs[proxy.ProviderNameOpenAI]
			},
			expectedError: "reason=unknown",
		},
		{
			name: "incomplete provider registry",
			mutate: func(catalogs proxy.ProviderModelCatalogs) {
				delete(catalogs, proxy.ProviderNameMeta)
			},
			expectedError: "configured_provider_count=11",
		},
		{
			name: "invalid model catalog",
			mutate: func(catalogs proxy.ProviderModelCatalogs) {
				openAICatalog := catalogs[proxy.ProviderNameOpenAI]
				openAICatalog.Text.DefaultModel = "missing-model"
				catalogs[proxy.ProviderNameOpenAI] = openAICatalog
			},
			expectedError: "invalid_model_catalog",
		},
	}

	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTestInstance *testing.T) {
			catalogs := testfixtures.ProviderModelCatalogs(subTestInstance)
			testCase.mutate(catalogs)
			_, catalogError := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderModels: catalogs})
			if catalogError == nil || !strings.Contains(catalogError.Error(), testCase.expectedError) {
				subTestInstance.Fatalf("error=%v want contains %q", catalogError, testCase.expectedError)
			}
		})
	}
}
