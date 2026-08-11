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

			router, buildRouterError := proxy.BuildRouter(testfixtures.WithModelCatalog(subTestInstance, proxy.Configuration{
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
		ModelCatalog: testfixtures.ModelCatalog(testingInstance),
	})
	if catalogError != nil {
		testingInstance.Fatalf("NewPublicCapabilityCatalog error: %v", catalogError)
	}
	if catalog.Revision == "" || len(catalog.Operations) != 3 || len(catalog.Prices) != len(catalog.Offerings) || catalog.Counts.Providers != 11 || catalog.Counts.ModelPublishers != 11 || catalog.Counts.ExactModels != len(catalog.Models) || catalog.Counts.ProviderOfferings != len(catalog.Offerings) || catalog.MaxPromptBytes != proxy.DefaultMaxPromptBytes || catalog.MaxInputAudioBytes != proxy.DefaultMaxInputAudioBytes {
		testingInstance.Fatalf("catalog summary=%+v", catalog)
	}
	geminiCapabilityFound := false
	openAIDictationCapabilityFound := false
	for _, model := range catalog.Models {
		for _, capability := range model.Capabilities {
			if capability == "background" || capability == "synchronous" {
				testingInstance.Fatalf("public capability catalog exposed execution lifecycle model=%s capability=%s", model.Identifier, capability)
			}
		}
		switch model.Identifier {
		case proxy.ModelNameGemini35Flash:
			geminiCapabilityFound = true
			if !slices.Equal(model.Capabilities, []string{
				proxy.PublicModelCapabilityAudioInput,
				proxy.PublicModelCapabilityImageInput,
				proxy.PublicModelCapabilityText,
			}) {
				testingInstance.Fatalf("Gemini capabilities=%v", model.Capabilities)
			}
		case proxy.DefaultDictationModel:
			if !slices.Equal(model.Capabilities, []string{proxy.PublicModelCapabilityDictation}) {
				testingInstance.Fatalf("OpenAI dictation capability=%+v", model)
			}
		}
	}
	for _, offering := range catalog.Offerings {
		if offering.Provider == proxy.ProviderNameOpenAI && offering.Model == proxy.DefaultDictationModel {
			openAIDictationCapabilityFound = true
		}
	}
	if !geminiCapabilityFound || !openAIDictationCapabilityFound {
		testingInstance.Fatalf("public capability catalog projections missing gemini=%t openai_dictation=%t", geminiCapabilityFound, openAIDictationCapabilityFound)
	}
}

func TestPublicCapabilityCatalogPublishesExactGeminiMediaLimits(testingInstance *testing.T) {
	catalog, catalogError := proxy.NewPublicCapabilityCatalog(proxy.Configuration{
		ModelCatalog: testfixtures.ModelCatalog(testingInstance),
	})
	if catalogError != nil {
		testingInstance.Fatalf("NewPublicCapabilityCatalog error: %v", catalogError)
	}
	expectedLimits := map[string]struct {
		mediaType string
		transport string
		status    string
		value     int64
		unit      string
		scope     string
		source    string
	}{
		proxy.CatalogMediaLimitIDInlineRequestBytes: {
			mediaType: proxy.CatalogMediaLimitTypeAll, transport: proxy.CatalogMediaTransportInline,
			status: proxy.CatalogMediaLimitStatusBounded, value: 20_000_000,
			unit: proxy.CatalogMediaLimitUnitBytes, scope: proxy.CatalogMediaLimitScopeRequestEncodedBytes,
			source: "https://ai.google.dev/gemini-api/docs/file-input-methods",
		},
		proxy.CatalogMediaLimitIDImageCount: {
			mediaType: "image", transport: proxy.CatalogMediaTransportAny,
			status: proxy.CatalogMediaLimitStatusBounded, value: 3_600,
			unit: proxy.CatalogMediaLimitUnitFiles, scope: proxy.CatalogMediaLimitScopeRequest,
			source: "https://ai.google.dev/gemini-api/docs/image-understanding",
		},
		proxy.CatalogMediaLimitIDAudioCount: {
			mediaType: "audio", transport: proxy.CatalogMediaTransportAny,
			status: proxy.CatalogMediaLimitStatusUnknown,
			unit:   proxy.CatalogMediaLimitUnitFiles, scope: proxy.CatalogMediaLimitScopeRequest,
			source: "https://ai.google.dev/gemini-api/docs/audio",
		},
		proxy.CatalogMediaLimitIDImageFileBytes: {
			mediaType: "image", transport: proxy.CatalogMediaTransportFile,
			status: proxy.CatalogMediaLimitStatusBounded, value: 2_000_000_000,
			unit: proxy.CatalogMediaLimitUnitBytes, scope: proxy.CatalogMediaLimitScopeAttachment,
			source: "https://ai.google.dev/gemini-api/docs/files",
		},
		proxy.CatalogMediaLimitIDAudioFileBytes: {
			mediaType: "audio", transport: proxy.CatalogMediaTransportFile,
			status: proxy.CatalogMediaLimitStatusBounded, value: 2_000_000_000,
			unit: proxy.CatalogMediaLimitUnitBytes, scope: proxy.CatalogMediaLimitScopeAttachment,
			source: "https://ai.google.dev/gemini-api/docs/files",
		},
	}

	mediaOfferingCount := 0
	for _, offering := range catalog.Offerings {
		if len(offering.MediaLimits) == 0 {
			continue
		}
		mediaOfferingCount++
		if offering.Provider != proxy.ProviderNameGemini || (offering.Model != proxy.ModelNameGemini35Flash && offering.Model != proxy.ModelNameGemini25Flash) || len(offering.MediaLimits) != len(expectedLimits) {
			testingInstance.Fatalf("unexpected media offering=%+v", offering)
		}
		for _, limit := range offering.MediaLimits {
			expected, exists := expectedLimits[limit.ID]
			if !exists || limit.MediaType != expected.mediaType || limit.Transport != expected.transport || limit.Status != expected.status || limit.Unit != expected.unit || limit.Scope != expected.scope || limit.Source != expected.source || limit.LastVerified != "2026-08-11" {
				testingInstance.Fatalf("media limit=%+v", limit)
			}
			if expected.status == proxy.CatalogMediaLimitStatusBounded && (limit.Value == nil || *limit.Value != expected.value) {
				testingInstance.Fatalf("bounded media limit=%+v", limit)
			}
			if expected.status != proxy.CatalogMediaLimitStatusBounded && limit.Value != nil {
				testingInstance.Fatalf("non-bounded media limit=%+v", limit)
			}
		}
	}
	if mediaOfferingCount != 2 {
		testingInstance.Fatalf("media offerings=%d want=2", mediaOfferingCount)
	}
}

func TestPublicCapabilityRESTResourceProjectsChangedCatalogWithoutPrivateConfig(testingInstance *testing.T) {
	const (
		changedModelID = "kimi-rest-contract"
		privateAPIKey  = "private-provider-key-sentinel"
		privateBaseURL = "https://private-provider-origin.invalid/v1"
		privateSecret  = "private-tenant-secret-sentinel"
	)
	catalogs := testfixtures.ModelCatalog(testingInstance)
	var changedModel proxy.ExactModel
	var changedOffering proxy.ProviderOffering
	for _, offering := range catalogs.Offerings {
		if offering.Provider == proxy.ProviderNameMoonshot {
			changedOffering = offering
			break
		}
	}
	for _, model := range catalogs.Models {
		if model.ID == changedOffering.Model {
			changedModel = model
			break
		}
	}
	changedModel.ID = changedModelID
	changedModel.Version = "rest-contract"
	changedOffering.Model = changedModelID
	changedOffering.ProviderModel = "private-native-model-sentinel"
	changedOffering.DefaultOperations = nil
	catalogs.Models = append(catalogs.Models, changedModel)
	catalogs.Offerings = append(catalogs.Offerings, changedOffering)
	catalogs.Prices = append(catalogs.Prices, proxy.CatalogPriceDescriptor{
		Provider: changedOffering.Provider, Model: changedModelID, Operation: proxy.ModelOperationText,
		Source: "https://platform.moonshot.ai/docs/pricing/chat", LastVerified: "2026-08-10",
		UnavailableReason: "Exact published pricing has not been imported for this provider offering.",
	})
	router, buildError := proxy.BuildRouter(proxy.Configuration{
		Tenants:       proxy.SingleTenantConfigurations("private-tenant", privateSecret),
		OpenAIKey:     privateAPIKey,
		OpenAIBaseURL: privateBaseURL,
		ModelCatalog:  catalogs,
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
	for _, privateValue := range []string{privateAPIKey, privateBaseURL, privateSecret, changedOffering.ProviderModel, "base_url", "provider_model", "default_operations"} {
		if strings.Contains(responseBody, privateValue) {
			testingInstance.Fatalf("public capability response exposed %q: %s", privateValue, responseBody)
		}
	}
	var catalog proxy.PublicCapabilityCatalog
	if decodeError := json.Unmarshal(response.Body.Bytes(), &catalog); decodeError != nil {
		testingInstance.Fatalf("decode public capability response: %v", decodeError)
	}
	if !slices.Equal(catalog.Providers[0].CredentialKinds, []string{proxy.CatalogCredentialAPIKey}) {
		testingInstance.Fatalf("public provider credential kinds=%v", catalog.Providers[0].CredentialKinds)
	}
	changedModelFound := false
	for _, model := range catalog.Models {
		if model.Identifier == changedModelID && slices.Contains(model.ProviderOfferings, proxy.ProviderNameMoonshot+":"+changedModelID) {
			changedModelFound = true
		}
	}
	if !changedModelFound {
		testingInstance.Fatalf("public capability response omitted changed model %q", changedModelID)
	}
}

func TestPublicCapabilityCatalogRejectsNoncanonicalRuntimeRegistries(testingInstance *testing.T) {
	testCases := []struct {
		name          string
		mutate        func(*proxy.ModelCatalog)
		expectedError string
	}{
		{
			name: "noncanonical provider identifier",
			mutate: func(catalogs *proxy.ModelCatalog) {
				catalogs.Providers[0].ID = "OpenAI"
			},
			expectedError: "reason=not_canonical",
		},
		{
			name: "unknown provider identifier",
			mutate: func(catalogs *proxy.ModelCatalog) {
				catalogs.Providers[0].ID = "future"
			},
			expectedError: "reason=unknown",
		},
		{
			name: "incomplete provider registry",
			mutate: func(catalogs *proxy.ModelCatalog) {
				catalogs.Providers = slices.Delete(catalogs.Providers, 9, 10)
			},
			expectedError: "field=catalog.providers provider=meta reason=missing",
		},
		{
			name: "invalid model catalog",
			mutate: func(catalogs *proxy.ModelCatalog) {
				catalogs.Offerings[0].Model = "missing-model"
			},
			expectedError: "reason=dangling_reference",
		},
		{
			name: "missing media limits",
			mutate: func(catalogs *proxy.ModelCatalog) {
				for offeringIndex := range catalogs.Offerings {
					if catalogs.Offerings[offeringIndex].Provider == proxy.ProviderNameGemini && catalogs.Offerings[offeringIndex].Model == proxy.ModelNameGemini25Flash {
						catalogs.Offerings[offeringIndex].MediaLimits = nil
						return
					}
				}
			},
			expectedError: "reason=media_limits_missing",
		},
		{
			name: "bounded media limit without value",
			mutate: func(catalogs *proxy.ModelCatalog) {
				for offeringIndex := range catalogs.Offerings {
					if catalogs.Offerings[offeringIndex].Provider == proxy.ProviderNameGemini && catalogs.Offerings[offeringIndex].Model == proxy.ModelNameGemini25Flash {
						catalogs.Offerings[offeringIndex].MediaLimits[0].Value = nil
						return
					}
				}
			},
			expectedError: ".media_limits[0].value",
		},
	}

	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTestInstance *testing.T) {
			catalogs := testfixtures.ModelCatalog(subTestInstance)
			testCase.mutate(&catalogs)
			_, catalogError := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ModelCatalog: catalogs})
			if catalogError == nil || !strings.Contains(catalogError.Error(), testCase.expectedError) {
				subTestInstance.Fatalf("error=%v want contains %q", catalogError, testCase.expectedError)
			}
		})
	}
}
