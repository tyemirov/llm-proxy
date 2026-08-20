package tests_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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

			router, buildRouterError := testfixtures.BuildManagedRouter(subTestInstance, proxy.Configuration{
				LogLevel:    logLevel,
				WorkerCount: 1,
				QueueSize:   4,
				Endpoints:   endpoints,
			}, logger.Sugar(), testfixtures.StandardManagedTenant(serviceSecret))
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
	if catalog.Revision == "" || len(catalog.Operations) != 3 || len(catalog.Prices) != len(catalog.Offerings) || catalog.Counts.Providers != 11 || catalog.Counts.ModelPublishers != 11 || catalog.Counts.ModelFamilies != len(catalog.Families) || catalog.Counts.ExactModels != len(catalog.Models) || catalog.Counts.ProviderOfferings != len(catalog.Offerings) || catalog.MaxPromptBytes != proxy.DefaultMaxPromptBytes || catalog.MaxInputAudioBytes != proxy.DefaultMaxInputAudioBytes {
		testingInstance.Fatalf("catalog summary=%+v", catalog)
	}
	weightAccessFound := map[string]bool{}
	for _, family := range catalog.Families {
		if family.WeightAccess != proxy.ModelWeightAccessProprietary && family.WeightAccess != proxy.ModelWeightAccessOpenWeights {
			testingInstance.Fatalf("family=%s weight_access=%q", family.Identifier, family.WeightAccess)
		}
		weightAccessFound[family.WeightAccess] = true
	}
	if !weightAccessFound[proxy.ModelWeightAccessProprietary] || !weightAccessFound[proxy.ModelWeightAccessOpenWeights] {
		testingInstance.Fatalf("public weight access classifications=%v", weightAccessFound)
	}
	geminiCapabilityFound := false
	openAIDictationCapabilityFound := false
	expectedKimiModels := map[string]struct{}{
		proxy.ModelNameMoonshotKimiK26:              {},
		proxy.ModelNameMoonshotKimiK3:               {},
		proxy.ModelNameMoonshotKimiK27Code:          {},
		proxy.ModelNameMoonshotKimiK27CodeHighSpeed: {},
	}
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
		default:
			if _, expected := expectedKimiModels[model.Identifier]; expected {
				expectedCapabilities := []string{proxy.PublicModelCapabilityImageInput, proxy.PublicModelCapabilityText}
				if model.Identifier == proxy.ModelNameMoonshotKimiK3 {
					expectedCapabilities = []string{proxy.PublicModelCapabilityImageInput, proxy.PublicModelCapabilityReasoning, proxy.PublicModelCapabilityText}
				}
				if !slices.Equal(model.Capabilities, expectedCapabilities) {
					testingInstance.Fatalf("Kimi model=%s capabilities=%v", model.Identifier, model.Capabilities)
				}
				delete(expectedKimiModels, model.Identifier)
			}
		}
	}
	for _, offering := range catalog.Offerings {
		if offering.Provider == proxy.ProviderNameOpenAI && offering.Model == proxy.DefaultDictationModel {
			openAIDictationCapabilityFound = true
		}
		if offering.Provider == proxy.ProviderNameMoonshot && offering.Model == proxy.ModelNameMoonshotKimiK3 {
			if !reflect.DeepEqual(offering.ReasoningEfforts, []string{"low", "high", "max"}) {
				testingInstance.Fatalf("Kimi K3 reasoning efforts=%v", offering.ReasoningEfforts)
			}
		}
	}
	if !geminiCapabilityFound || !openAIDictationCapabilityFound || len(expectedKimiModels) != 0 {
		testingInstance.Fatalf("public capability catalog projections missing gemini=%t openai_dictation=%t", geminiCapabilityFound, openAIDictationCapabilityFound)
	}

	expectedQwenModels := map[string]struct{}{
		proxy.ModelNameDashScopeQwen37Max:   {},
		proxy.ModelNameDashScopeQwen37Plus:  {},
		proxy.ModelNameDashScopeQwen36Flash: {},
	}
	for _, offering := range catalog.Offerings {
		if _, expected := expectedQwenModels[offering.Model]; !expected || offering.Provider != proxy.ProviderNameDashScope {
			continue
		}
		if offering.WireContract != "openai_chat_completions" || offering.ExecutionLifecycle != "synchronous_completion" || offering.OutputTokenLimit != 65536 {
			testingInstance.Fatalf("Qwen offering route=%+v", offering)
		}
		if len(offering.Controls) != 1 || offering.Controls[0].ID != "max_tokens" || offering.Controls[0].Minimum == nil || *offering.Controls[0].Minimum != 1 || offering.Controls[0].Maximum == nil || *offering.Controls[0].Maximum != 65536 {
			testingInstance.Fatalf("Qwen offering controls=%+v", offering.Controls)
		}
		if len(offering.Limits) != 1 || offering.Limits[0].ID != "context_tokens" || offering.Limits[0].Value == nil || *offering.Limits[0].Value != 1000000 || offering.Limits[0].Unit != "tokens" {
			testingInstance.Fatalf("Qwen offering limits=%+v", offering.Limits)
		}
		delete(expectedQwenModels, offering.Model)
	}
	if len(expectedQwenModels) != 0 {
		testingInstance.Fatalf("public capability catalog omitted Qwen models=%v", expectedQwenModels)
	}

	expectedQwenRates := map[string][]float64{
		proxy.ModelNameDashScopeQwen37Max:   {2.5, 7.5},
		proxy.ModelNameDashScopeQwen37Plus:  {0.4, 1.6},
		proxy.ModelNameDashScopeQwen36Flash: {0.25, 1.5},
	}
	for _, price := range catalog.Prices {
		expectedRates, expected := expectedQwenRates[price.Model]
		if !expected || price.Provider != proxy.ProviderNameDashScope {
			continue
		}
		if !price.Available || price.Source != "https://www.alibabacloud.com/help/en/model-studio/model-pricing" || price.LastVerified != "2026-08-13" || len(price.Rates) != 2 || price.Rates[0].Rate != expectedRates[0] || price.Rates[1].Rate != expectedRates[1] {
			testingInstance.Fatalf("Qwen price=%+v", price)
		}
		delete(expectedQwenRates, price.Model)
	}
	if len(expectedQwenRates) != 0 {
		testingInstance.Fatalf("public capability catalog omitted Qwen prices=%v", expectedQwenRates)
	}

	expectedMiniMaxRates := map[string][]float64{
		proxy.ModelNameMiniMaxM27:          {0.3, 1.2, 0.06, 0.375},
		proxy.ModelNameMiniMaxM27HighSpeed: {0.6, 2.4, 0.06, 0.375},
		proxy.ModelNameMiniMaxM25:          {0.3, 1.2, 0.03, 0.375},
		proxy.ModelNameMiniMaxM25HighSpeed: {0.6, 2.4, 0.03, 0.375},
		proxy.ModelNameMiniMaxM21:          {0.3, 1.2, 0.03, 0.375},
		proxy.ModelNameMiniMaxM21HighSpeed: {0.6, 2.4, 0.03, 0.375},
		proxy.ModelNameMiniMaxM2:           {0.3, 1.2, 0.03, 0.375},
	}
	validatedMiniMaxOfferings := 0
	for _, offering := range catalog.Offerings {
		if _, expected := expectedMiniMaxRates[offering.Model]; !expected || offering.Provider != proxy.ProviderNameMiniMax {
			continue
		}
		if offering.WireContract != "openai_chat_completions" || offering.ExecutionLifecycle != "synchronous_completion" || offering.OutputTokenLimit != 204800 {
			testingInstance.Fatalf("MiniMax offering route=%+v", offering)
		}
		if len(offering.Controls) != 1 || offering.Controls[0].ID != "max_tokens" || offering.Controls[0].Minimum == nil || *offering.Controls[0].Minimum != 1 || offering.Controls[0].Maximum == nil || *offering.Controls[0].Maximum != 204800 {
			testingInstance.Fatalf("MiniMax offering controls=%+v", offering.Controls)
		}
		if len(offering.Limits) != 1 || offering.Limits[0].ID != "context_tokens" || offering.Limits[0].Value == nil || *offering.Limits[0].Value != 204800 || offering.Limits[0].Unit != "tokens" {
			testingInstance.Fatalf("MiniMax offering limits=%+v", offering.Limits)
		}
		validatedMiniMaxOfferings++
	}
	if validatedMiniMaxOfferings != len(expectedMiniMaxRates) {
		testingInstance.Fatalf("public capability catalog MiniMax offerings=%d want=%d", validatedMiniMaxOfferings, len(expectedMiniMaxRates))
	}
	for _, price := range catalog.Prices {
		expectedRates, expected := expectedMiniMaxRates[price.Model]
		if !expected || price.Provider != proxy.ProviderNameMiniMax {
			continue
		}
		if !price.Available || price.Source != "https://platform.minimax.io/docs/guides/pricing-paygo" || price.LastVerified != "2026-08-13" || len(price.Rates) != len(expectedRates) {
			testingInstance.Fatalf("MiniMax price=%+v", price)
		}
		for rateIndex, expectedRate := range expectedRates {
			if price.Rates[rateIndex].Rate != expectedRate || price.Rates[rateIndex].Conditions.BillingMode != "pay_as_you_go_standard" {
				testingInstance.Fatalf("MiniMax price=%+v", price)
			}
		}
		delete(expectedMiniMaxRates, price.Model)
	}
	if len(expectedMiniMaxRates) != 0 {
		testingInstance.Fatalf("public capability catalog omitted MiniMax prices=%v", expectedMiniMaxRates)
	}
}

func TestPublicCapabilityCatalogPublishesExactProviderMediaLimits(testingInstance *testing.T) {
	catalog, catalogError := proxy.NewPublicCapabilityCatalog(proxy.Configuration{
		ModelCatalog: testfixtures.ModelCatalog(testingInstance),
	})
	if catalogError != nil {
		testingInstance.Fatalf("NewPublicCapabilityCatalog error: %v", catalogError)
	}
	expectedOfferingCounts := map[string]int{
		proxy.ProviderNameOpenAI:    11,
		proxy.ProviderNameGemini:    7,
		proxy.ProviderNameAnthropic: 10,
		proxy.ProviderNameMoonshot:  4,
		proxy.ProviderNameXAI:       1,
	}
	observedOfferingCounts := map[string]int{}
	for _, offering := range catalog.Offerings {
		if len(offering.MediaLimits) == 0 {
			continue
		}
		observedOfferingCounts[offering.Provider]++
		expectedLimitCount := 3
		if offering.Provider == proxy.ProviderNameGemini {
			expectedLimitCount = 5
		}
		if _, expectedProvider := expectedOfferingCounts[offering.Provider]; !expectedProvider || len(offering.MediaLimits) != expectedLimitCount {
			testingInstance.Fatalf("unexpected media offering=%+v", offering)
		}
		observedLimitIDs := map[string]proxy.CatalogMediaLimit{}
		for _, limit := range offering.MediaLimits {
			expectedVerificationDate := "2026-08-11"
			if offering.Provider == proxy.ProviderNameMoonshot {
				expectedVerificationDate = "2026-08-13"
			}
			if limit.LastVerified != expectedVerificationDate {
				testingInstance.Fatalf("media limit=%+v", limit)
			}
			observedLimitIDs[limit.ID] = limit
		}
		for _, requiredLimitID := range []string{proxy.CatalogMediaLimitIDInlineRequestBytes, proxy.CatalogMediaLimitIDImageCount} {
			if _, found := observedLimitIDs[requiredLimitID]; !found {
				testingInstance.Fatalf("provider=%s model=%s missing media limit=%s", offering.Provider, offering.Model, requiredLimitID)
			}
		}
		if offering.Provider == proxy.ProviderNameGemini {
			for _, requiredLimitID := range []string{proxy.CatalogMediaLimitIDAudioCount, proxy.CatalogMediaLimitIDImageFileBytes, proxy.CatalogMediaLimitIDAudioFileBytes} {
				if _, found := observedLimitIDs[requiredLimitID]; !found {
					testingInstance.Fatalf("provider=%s model=%s missing media limit=%s", offering.Provider, offering.Model, requiredLimitID)
				}
			}
		} else if _, found := observedLimitIDs[proxy.CatalogMediaLimitIDImageInlineBytes]; !found {
			testingInstance.Fatalf("provider=%s model=%s missing inline image limit", offering.Provider, offering.Model)
		}
		if offering.Provider == proxy.ProviderNameMoonshot {
			requestLimit := observedLimitIDs[proxy.CatalogMediaLimitIDInlineRequestBytes]
			imageCount := observedLimitIDs[proxy.CatalogMediaLimitIDImageCount]
			imageBytes := observedLimitIDs[proxy.CatalogMediaLimitIDImageInlineBytes]
			if requestLimit.Status != proxy.CatalogMediaLimitStatusBounded || requestLimit.Value == nil || *requestLimit.Value != 100000000 || imageCount.Status != proxy.CatalogMediaLimitStatusUnbounded || imageCount.Value != nil || imageBytes.Status != proxy.CatalogMediaLimitStatusUnknown || imageBytes.Value != nil {
				testingInstance.Fatalf("Moonshot media limits=%+v", offering.MediaLimits)
			}
		}
	}
	if !reflect.DeepEqual(observedOfferingCounts, expectedOfferingCounts) {
		testingInstance.Fatalf("media offering counts=%v want=%v", observedOfferingCounts, expectedOfferingCounts)
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
	tenant := testfixtures.StandardManagedTenant(privateSecret)
	tenant.ProviderKeys[proxy.ProviderNameOpenAI] = privateAPIKey
	router, buildError := testfixtures.BuildManagedRouter(testingInstance, proxy.Configuration{
		OpenAIBaseURL: privateBaseURL,
		ModelCatalog:  catalogs,
	}, zap.NewNop().Sugar(), tenant)
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
