package tests_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
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
	if len(catalog.Providers) != 11 || catalog.MaxPromptBytes != proxy.DefaultMaxPromptBytes || catalog.MaxInputAudioBytes != proxy.DefaultMaxInputAudioBytes {
		testingInstance.Fatalf("catalog summary=%+v", catalog)
	}
	geminiCapabilityFound := false
	openAIDictationCapabilityFound := false
	for _, provider := range catalog.Providers {
		for _, model := range provider.Models {
			for _, capability := range model.Capabilities {
				if capability == "background" || capability == "synchronous" {
					testingInstance.Fatalf("public capability catalog exposed execution lifecycle provider=%s model=%s capability=%s", provider.Identifier, model.Identifier, capability)
				}
			}
			switch {
			case provider.Identifier == proxy.ProviderNameGemini && model.Identifier == proxy.ModelNameGemini35Flash:
				geminiCapabilityFound = true
				if !slices.Equal(model.Capabilities, []string{
					proxy.PublicModelCapabilityText,
					proxy.PublicModelCapabilityAudioInput,
					proxy.PublicModelCapabilityImageInput,
				}) {
					testingInstance.Fatalf("Gemini capabilities=%v", model.Capabilities)
				}
			case provider.Identifier == proxy.ProviderNameOpenAI && model.Identifier == proxy.DefaultDictationModel:
				openAIDictationCapabilityFound = true
				if !slices.Equal(model.Capabilities, []string{proxy.PublicModelCapabilityDictation}) ||
					!slices.Equal(model.DefaultEndpoints, []string{proxy.PublicModelCapabilityDictation}) {
					testingInstance.Fatalf("OpenAI dictation capability=%+v", model)
				}
			}
		}
	}
	if !geminiCapabilityFound || !openAIDictationCapabilityFound {
		testingInstance.Fatalf("public capability catalog projections missing gemini=%t openai_dictation=%t", geminiCapabilityFound, openAIDictationCapabilityFound)
	}
}

func TestPublicCapabilityRESTResourceProjectsChangedCatalogWithoutPrivateConfig(testingInstance *testing.T) {
	const (
		changedModelID = "kimi-rest-contract"
		privateAPIKey  = "private-provider-key-sentinel"
		privateBaseURL = "https://private-provider-origin.invalid/v1"
		privateSecret  = "private-tenant-secret-sentinel"
	)
	catalogs := testfixtures.ProviderModelCatalogs(testingInstance)
	moonshotCatalog := catalogs[proxy.ProviderNameMoonshot]
	changedModel := moonshotCatalog.Text.Models[0]
	changedModel.ID = changedModelID
	moonshotCatalog.Text.Models = append(moonshotCatalog.Text.Models, changedModel)
	catalogs[proxy.ProviderNameMoonshot] = moonshotCatalog
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		Tenants:        proxy.SingleTenantConfigurations("private-tenant", privateSecret),
		OpenAIKey:      privateAPIKey,
		OpenAIBaseURL:  privateBaseURL,
		ProviderModels: catalogs,
	}, zap.NewNop().Sugar())
	if buildError != nil {
		testingInstance.Fatalf("BuildRouter error: %v", buildError)
	}

	request := httptest.NewRequest(http.MethodGet, proxy.PublicCapabilitiesPath, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		testingInstance.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "public, max-age=300" {
		testingInstance.Fatalf("cache control=%q", response.Header().Get("Cache-Control"))
	}
	responseBody := response.Body.String()
	for _, privateValue := range []string{privateAPIKey, privateBaseURL, privateSecret, "api_key", "base_url"} {
		if strings.Contains(responseBody, privateValue) {
			testingInstance.Fatalf("public capability response exposed %q: %s", privateValue, responseBody)
		}
	}
	var catalog proxy.PublicCapabilityCatalog
	if decodeError := json.Unmarshal(response.Body.Bytes(), &catalog); decodeError != nil {
		testingInstance.Fatalf("decode public capability response: %v", decodeError)
	}
	changedModelFound := false
	for _, provider := range catalog.Providers {
		if provider.Identifier != proxy.ProviderNameMoonshot {
			continue
		}
		for _, model := range provider.Models {
			if model.Identifier == changedModelID {
				changedModelFound = true
			}
		}
	}
	if !changedModelFound {
		testingInstance.Fatalf("public capability response omitted changed model %q", changedModelID)
	}
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
			expectedError: "provider=meta field=providers.meta.text",
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
