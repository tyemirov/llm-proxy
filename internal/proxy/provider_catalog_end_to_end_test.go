package proxy_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
)

const (
	testCatalogProviderID          = "catalog-test"
	testCatalogProviderAlias       = "catalog-test-alias"
	testCatalogCredentialField     = "access_token"
	testCatalogSettingField        = "gateway_url"
	testCatalogCredentialEnv       = "CATALOG_TEST_TOKEN"
	testCatalogSettingEnv          = "CATALOG_TEST_URL"
	testCatalogModelID             = "catalog-test-model"
	testCatalogUpstreamModelID     = "private-catalog-test-model"
	testCatalogTransportID         = "text"
	testCatalogProviderCredential  = "catalog-test-secret-value"
	testCatalogProviderResponse    = "catalog route complete"
	testCatalogProviderSystem      = "catalog system prompt"
	testCatalogProviderPublisherID = "catalog-test-publisher"
	testCatalogProviderFamilyID    = "catalog-test-family"
)

func TestProviderCatalogDeclaresProviderSpecificResourceVisibilityPolicies(testingInstance *testing.T) {
	expectedPolicies := map[string]proxy.ProviderCatalogResourceVisibility{
		proxy.ProviderNameOpenAI: {
			RetryIntervalMilliseconds: 2000,
			RetryLimit:                1,
			RetryStatusCodes:          []int{http.StatusForbidden, http.StatusNotFound},
		},
		proxy.ProviderNameGemini: {
			RetryIntervalMilliseconds: 5000,
			RetryLimit:                6,
			RetryStatusCodes:          []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound},
		},
	}
	observedPolicies := map[string]proxy.ProviderCatalogResourceVisibility{}
	for _, provider := range testfixtures.ProviderCatalog(testingInstance).Schema().Providers {
		if _, expected := expectedPolicies[provider.ID]; !expected {
			continue
		}
		for _, transport := range provider.Transports {
			if transport.ResourceVisibility.RetryLimit != 0 {
				observedPolicies[provider.ID] = transport.ResourceVisibility
			}
		}
	}
	for providerIdentifier, expectedPolicy := range expectedPolicies {
		observedPolicy, found := observedPolicies[providerIdentifier]
		if !found ||
			observedPolicy.RetryIntervalMilliseconds != expectedPolicy.RetryIntervalMilliseconds ||
			observedPolicy.RetryLimit != expectedPolicy.RetryLimit ||
			!slices.Equal(observedPolicy.RetryStatusCodes, expectedPolicy.RetryStatusCodes) {
			testingInstance.Fatalf("provider=%s visibility=%+v want=%+v", providerIdentifier, observedPolicy, expectedPolicy)
		}
	}
}

func TestProviderCatalogDeclaresGeminiInteractionsWithoutReplayContinuation(testingInstance *testing.T) {
	for _, provider := range testfixtures.ProviderCatalog(testingInstance).Schema().Providers {
		if provider.ID != proxy.ProviderNameGemini {
			continue
		}
		for _, transport := range provider.Transports {
			if transport.RequestProtocol != proxy.CatalogProtocolGeminiInteractions {
				continue
			}
			if len(transport.ProtocolParameters.ContinuationRules) != 0 {
				testingInstance.Fatalf("Gemini Interactions continuation rules=%v want=[]", transport.ProtocolParameters.ContinuationRules)
			}
			return
		}
	}
	testingInstance.Fatal("Gemini Interactions transport is missing")
}

func TestCatalogDefinedProviderFlowsThroughEveryGenericConsumer(testingInstance *testing.T) {
	var requestMutex sync.Mutex
	routedModels := []string{}
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer "+testCatalogProviderCredential {
			http.Error(responseWriter, "missing catalog authentication", http.StatusUnauthorized)
			return
		}
		var payload struct {
			Model string `json:"model"`
		}
		if decodeError := json.NewDecoder(request.Body).Decode(&payload); decodeError != nil {
			http.Error(responseWriter, "invalid request", http.StatusBadRequest)
			return
		}
		requestMutex.Lock()
		routedModels = append(routedModels, payload.Model)
		requestMutex.Unlock()
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(responseWriter, `{"choices":[{"message":{"content":"`+testCatalogProviderResponse+`"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`)
	}))
	defer upstreamServer.Close()

	providerCatalog := catalogWithTestProvider(testingInstance)
	environmentBindings, bindingError := providerCatalog.ResolveEnvironmentBindings(map[string]string{
		testCatalogCredentialEnv: testCatalogProviderCredential,
		testCatalogSettingEnv:    upstreamServer.URL,
	})
	if bindingError != nil {
		testingInstance.Fatalf("resolve catalog test environment: %v", bindingError)
	}
	if !mapsEqual(environmentBindings[testCatalogProviderID], map[string]string{
		testCatalogCredentialField: testCatalogProviderCredential,
		testCatalogSettingField:    upstreamServer.URL,
	}) {
		testingInstance.Fatalf("catalog environment bindings=%v", environmentBindings[testCatalogProviderID])
	}
	databasePath := filepath.Join(testingInstance.TempDir(), "managed-tenants.db")
	configuration := proxy.Configuration{ProviderCatalog: providerCatalog}
	router := newManagementRouterWithDatabasePath(testingInstance, configuration, databasePath)
	sessionCookie := managementSessionCookie(testingInstance, "tauth-catalog-provider-owner")
	tenantPath := managementDefaultTenantTestPath(testingInstance, router, sessionCookie, "")

	profileRequest := httptest.NewRequest(http.MethodGet, tenantPath, nil)
	profileRequest.AddCookie(sessionCookie)
	profileResponse := httptest.NewRecorder()
	router.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusOK {
		testingInstance.Fatalf("catalog provider profile status=%d body=%s", profileResponse.Code, profileResponse.Body.String())
	}
	assertTestCatalogManagementSchema(testingInstance, profileResponse.Body.Bytes(), false, upstreamServer.URL)
	for _, privateBinding := range []string{testCatalogCredentialEnv, testCatalogSettingEnv, testCatalogUpstreamModelID, "Authorization", "Bearer "} {
		if strings.Contains(profileResponse.Body.String(), privateBinding) {
			testingInstance.Fatalf("management schema exposed private catalog binding %q: %s", privateBinding, profileResponse.Body.String())
		}
	}

	connectionBody, marshalError := json.Marshal(map[string]any{
		"fields": map[string]string{
			testCatalogCredentialField: testCatalogProviderCredential,
			testCatalogSettingField:    upstreamServer.URL,
		},
		"text_model":    testCatalogModelID,
		"system_prompt": testCatalogProviderSystem,
	})
	if marshalError != nil {
		testingInstance.Fatalf("marshal catalog provider connection: %v", marshalError)
	}
	connectionRequest := authenticatedJSONRequest(http.MethodPut, tenantPath+"/provider-connections/"+testCatalogProviderAlias, string(connectionBody), sessionCookie)
	connectionResponse := httptest.NewRecorder()
	router.ServeHTTP(connectionResponse, connectionRequest)
	if connectionResponse.Code != http.StatusOK {
		testingInstance.Fatalf("save catalog provider connection status=%d body=%s", connectionResponse.Code, connectionResponse.Body.String())
	}
	if strings.Contains(connectionResponse.Body.String(), testCatalogProviderCredential) {
		testingInstance.Fatalf("save response exposed catalog credential: %s", connectionResponse.Body.String())
	}
	assertTestCatalogManagementSchema(testingInstance, connectionResponse.Body.Bytes(), true, upstreamServer.URL)

	tenantID := managementDefaultTenantTestID(testingInstance, router, sessionCookie)
	fixtureDatabase := openManagedFixtureDatabase(testingInstance, databasePath)
	var credentialRecord managedProviderConnectionFixture
	credentialQuery := fixtureDatabase.Where(
		"tenant_id = ? AND provider_id = ? AND field_id = ?",
		tenantID,
		testCatalogProviderID,
		testCatalogCredentialField,
	).First(&credentialRecord)
	if credentialQuery.Error != nil {
		testingInstance.Fatalf("load catalog credential record: %v", credentialQuery.Error)
	}
	if credentialRecord.Value == testCatalogProviderCredential || strings.Contains(credentialRecord.Value, testCatalogProviderCredential) {
		testingInstance.Fatalf("catalog credential was not encrypted: %q", credentialRecord.Value)
	}
	var settingRecord managedProviderConnectionFixture
	settingQuery := fixtureDatabase.Where(
		"tenant_id = ? AND provider_id = ? AND field_id = ?",
		tenantID,
		testCatalogProviderID,
		testCatalogSettingField,
	).First(&settingRecord)
	if settingQuery.Error != nil {
		testingInstance.Fatalf("load catalog setting record: %v", settingQuery.Error)
	}
	if settingRecord.Value != upstreamServer.URL {
		testingInstance.Fatalf("catalog setting value=%q want=%q", settingRecord.Value, upstreamServer.URL)
	}
	fixtureSQLDatabase, fixtureDatabaseError := fixtureDatabase.DB()
	if fixtureDatabaseError != nil {
		testingInstance.Fatalf("resolve catalog fixture database: %v", fixtureDatabaseError)
	}
	if closeError := fixtureSQLDatabase.Close(); closeError != nil {
		testingInstance.Fatalf("close catalog fixture database: %v", closeError)
	}

	revealRequest := authenticatedProviderKeyRevealRequest(
		http.MethodPost,
		tenantPath+"/provider-connections/"+testCatalogProviderID+"/fields/"+testCatalogCredentialField+"/reveal",
		sessionCookie,
		"http://localhost:8080",
	)
	revealResponse := httptest.NewRecorder()
	router.ServeHTTP(revealResponse, revealRequest)
	if revealResponse.Code != http.StatusOK {
		testingInstance.Fatalf("reveal catalog credential status=%d body=%s", revealResponse.Code, revealResponse.Body.String())
	}
	var revealPayload map[string]string
	if decodeError := json.Unmarshal(revealResponse.Body.Bytes(), &revealPayload); decodeError != nil {
		testingInstance.Fatalf("decode catalog credential reveal: %v", decodeError)
	}
	if revealPayload["field_id"] != testCatalogCredentialField || revealPayload["value"] != testCatalogProviderCredential {
		testingInstance.Fatalf("catalog credential reveal=%v", revealPayload)
	}

	secretRequest := authenticatedJSONRequest(http.MethodPost, tenantPath+"/secrets", `{}`, sessionCookie)
	secretResponse := httptest.NewRecorder()
	router.ServeHTTP(secretResponse, secretRequest)
	if secretResponse.Code != http.StatusOK {
		testingInstance.Fatalf("generate catalog tenant secret status=%d body=%s", secretResponse.Code, secretResponse.Body.String())
	}
	var secretPayload struct {
		Secret string `json:"secret"`
	}
	if decodeError := json.Unmarshal(secretResponse.Body.Bytes(), &secretPayload); decodeError != nil {
		testingInstance.Fatalf("decode catalog tenant secret: %v", decodeError)
	}

	assertTestCatalogRoute(testingInstance, router, secretPayload.Secret)
	reloadedRouter := newManagementRouterWithDatabasePath(testingInstance, configuration, databasePath)
	assertTestCatalogRoute(testingInstance, reloadedRouter, secretPayload.Secret)
	requestMutex.Lock()
	observedModels := append([]string(nil), routedModels...)
	requestMutex.Unlock()
	if !slices.Equal(observedModels, []string{testCatalogUpstreamModelID, testCatalogUpstreamModelID}) {
		testingInstance.Fatalf("catalog upstream models=%v", observedModels)
	}

	capabilitiesRequest := httptest.NewRequest(http.MethodGet, proxy.PublicCapabilitiesPath, nil)
	capabilitiesResponse := httptest.NewRecorder()
	reloadedRouter.ServeHTTP(capabilitiesResponse, capabilitiesRequest)
	if capabilitiesResponse.Code != http.StatusOK {
		testingInstance.Fatalf("catalog capabilities status=%d body=%s", capabilitiesResponse.Code, capabilitiesResponse.Body.String())
	}
	capabilitiesBody := capabilitiesResponse.Body.String()
	for _, expectedPublicValue := range []string{testCatalogProviderID, testCatalogModelID} {
		if !strings.Contains(capabilitiesBody, expectedPublicValue) {
			testingInstance.Fatalf("public capabilities omitted %q: %s", expectedPublicValue, capabilitiesBody)
		}
	}
	for _, privateValue := range []string{
		testCatalogCredentialField,
		testCatalogSettingField,
		testCatalogCredentialEnv,
		testCatalogSettingEnv,
		testCatalogProviderCredential,
		upstreamServer.URL,
		testCatalogUpstreamModelID,
		"Authorization",
		"Bearer ",
	} {
		if strings.Contains(capabilitiesBody, privateValue) {
			testingInstance.Fatalf("public capabilities exposed private catalog value %q: %s", privateValue, capabilitiesBody)
		}
	}

	deleteRequest := authenticatedJSONRequest(http.MethodDelete, tenantPath+"/provider-connections/"+testCatalogProviderID, `{}`, sessionCookie)
	deleteResponse := httptest.NewRecorder()
	reloadedRouter.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		testingInstance.Fatalf("delete catalog credential status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	var deletedProfile struct {
		Providers []struct {
			ID           string `json:"id"`
			Configured   bool   `json:"configured"`
			TextModel    string `json:"text_model"`
			SystemPrompt string `json:"system_prompt"`
			Fields       []struct {
				ID         string  `json:"id"`
				Configured bool    `json:"configured"`
				Value      *string `json:"value"`
			} `json:"fields"`
		} `json:"providers"`
	}
	if decodeError := json.Unmarshal(deleteResponse.Body.Bytes(), &deletedProfile); decodeError != nil {
		testingInstance.Fatalf("decode credential-deleted profile: %v", decodeError)
	}
	deletedProvider := deletedProfile.Providers[len(deletedProfile.Providers)-1]
	if deletedProvider.ID != testCatalogProviderID || deletedProvider.Configured || deletedProvider.TextModel != testCatalogModelID || deletedProvider.SystemPrompt != testCatalogProviderSystem {
		testingInstance.Fatalf("credential-deleted provider profile=%+v", deletedProvider)
	}
	if deletedProvider.Fields[0].Configured || deletedProvider.Fields[1].Value == nil || *deletedProvider.Fields[1].Value != upstreamServer.URL {
		testingInstance.Fatalf("credential deletion changed provider fields=%+v", deletedProvider.Fields)
	}
}

func TestProviderCatalogRejectsStructuralAndAdapterContractViolations(testingInstance *testing.T) {
	testCases := []struct {
		name          string
		mutate        func(*proxy.ProviderCatalogSchema)
		expectedError string
	}{
		{
			name: "duplicate provider identifier",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				schema.Providers[1].ID = schema.Providers[0].ID
			},
			expectedError: "duplicate_identifier=",
		},
		{
			name: "provider alias collision",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				schema.Providers[1].Aliases = append(schema.Providers[1].Aliases, schema.Providers[0].ID)
			},
			expectedError: "alias_collision=",
		},
		{
			name: "duplicate field identifier",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				schema.Providers[2].Fields[1].ID = schema.Providers[2].Fields[0].ID
			},
			expectedError: "duplicate_identifier=api_key",
		},
		{
			name: "missing exact model reference",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				schema.Providers[0].Offerings[0].Model = "missing-exact-model"
			},
			expectedError: "reason=dangling_reference",
		},
		{
			name: "unsupported lifecycle",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				schema.Providers[0].Transports[0].Lifecycle = "future_lifecycle"
			},
			expectedError: "lifecycle=future_lifecycle",
		},
		{
			name: "adapter parameter mismatch",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				schema.Providers[0].Transports[0].ProtocolParameters.ModelField = "future_model_field"
			},
			expectedError: "reason=adapter_contract_mismatch",
		},
		{
			name: "negative output token limit",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				schema.Providers[0].Offerings[0].OutputTokenLimit = -1
			},
			expectedError: "output_token_limit",
		},
		{
			name: "missing operation default",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				provider, offering := catalogDefaultOffering(schema, proxy.ModelOperationText)
				provider.Offerings[offering].DefaultOperations = removeCatalogValue(provider.Offerings[offering].DefaultOperations, proxy.ModelOperationText)
			},
			expectedError: "default_count=0",
		},
		{
			name: "duplicate operation default",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				provider, _ := catalogDefaultOffering(schema, proxy.ModelOperationText)
				for offeringIndex := range provider.Offerings {
					offering := &provider.Offerings[offeringIndex]
					if slices.Contains(offering.Operations, proxy.ModelOperationText) && !slices.Contains(offering.DefaultOperations, proxy.ModelOperationText) {
						offering.DefaultOperations = append(offering.DefaultOperations, proxy.ModelOperationText)
						return
					}
				}
				testingInstance.Fatal("canonical provider has no second text offering")
			},
			expectedError: "default_count=2",
		},
		{
			name: "missing offering price",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				schema.Providers[0].Offerings[0].Prices = nil
			},
			expectedError: ".prices",
		},
		{
			name: "invalid available price",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				price := &schema.Providers[0].Offerings[0].Prices[0]
				price.Available = true
				price.Rates = nil
				price.UnavailableReason = ""
			},
			expectedError: "reason=incomplete_available_price",
		},
		{
			name: "invalid control bounds",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				control := firstCatalogControl(testingInstance, schema)
				minimum := 2
				maximum := 1
				control.Kind = proxy.CatalogControlInteger
				control.Values = nil
				control.Minimum = &minimum
				control.Maximum = &maximum
			},
			expectedError: ".controls",
		},
		{
			name: "invalid limit bounds",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				limit := firstCatalogLimit(testingInstance, schema)
				zero := 0
				limit.AccountDependent = false
				limit.Value = &zero
			},
			expectedError: ".limits",
		},
		{
			name: "invalid exact model media",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				schema.Models[0].MediaInputs = []string{proxy.CatalogArtifactVideo}
			},
			expectedError: "media_input=video",
		},
		{
			name: "invalid request profile",
			mutate: func(schema *proxy.ProviderCatalogSchema) {
				schema.Providers[0].Offerings[0].RequestProfile = "future-profile"
			},
			expectedError: ".request_profile",
		},
	}

	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subTest *testing.T) {
			schema := testfixtures.ProviderCatalog(subTest).Schema()
			testCase.mutate(&schema)
			_, catalogError := proxy.NewProviderCatalog(schema)
			if catalogError == nil || !errors.Is(catalogError, proxy.ErrInvalidModelCatalog) || !strings.Contains(catalogError.Error(), testCase.expectedError) {
				subTest.Fatalf("catalog error=%v want invalid catalog containing %q", catalogError, testCase.expectedError)
			}
		})
	}
}

func TestProviderCatalogSnapshotsAreImmutable(testingInstance *testing.T) {
	catalog := testfixtures.ProviderCatalog(testingInstance)
	originalSchema := catalog.Schema()
	originalModelCatalog := catalog.ModelCatalog()

	mutatedSchema := catalog.Schema()
	mutatedSchema.Providers[0].ID = "mutated-provider"
	*mutatedSchema.Providers[0].Fields[0].Default = "mutated-secret"
	mutatedSchema.Providers[0].Offerings[0].Prices[0].Source = "https://mutated.example/pricing"
	mutatedModelCatalog := catalog.ModelCatalog()
	mutatedModelCatalog.Providers[0].ID = "mutated-provider"
	mutatedModelCatalog.Offerings[0].ProviderModel = "mutated-upstream-model"
	mutatedModelCatalog.Prices[0].Source = "https://mutated.example/pricing"

	currentSchema := catalog.Schema()
	currentModelCatalog := catalog.ModelCatalog()
	if currentSchema.Providers[0].ID != originalSchema.Providers[0].ID ||
		*currentSchema.Providers[0].Fields[0].Default != *originalSchema.Providers[0].Fields[0].Default ||
		currentSchema.Providers[0].Offerings[0].Prices[0].Source != originalSchema.Providers[0].Offerings[0].Prices[0].Source {
		testingInstance.Fatalf("schema snapshot mutated catalog state")
	}
	if currentModelCatalog.Providers[0].ID != originalModelCatalog.Providers[0].ID ||
		currentModelCatalog.Offerings[0].ProviderModel != originalModelCatalog.Offerings[0].ProviderModel ||
		currentModelCatalog.Prices[0].Source != originalModelCatalog.Prices[0].Source {
		testingInstance.Fatalf("runtime snapshot mutated catalog state")
	}
}

func catalogWithTestProvider(testingInstance *testing.T) *proxy.ProviderCatalog {
	testingInstance.Helper()
	schema := testfixtures.ProviderCatalog(testingInstance).Schema()
	emptyValue := ""
	schema.Publishers = append(schema.Publishers, proxy.ModelPublisher{
		ID: testCatalogProviderPublisherID, Label: "Catalog Test Publisher",
	})
	schema.Families = append(schema.Families, proxy.ModelFamily{
		ID: testCatalogProviderFamilyID, Publisher: testCatalogProviderPublisherID,
		Label: "Catalog Test Family", WeightAccess: proxy.ModelWeightAccessProprietary,
	})
	schema.Models = append(schema.Models, proxy.ExactModel{
		ID: testCatalogModelID, Publisher: testCatalogProviderPublisherID,
		Family: testCatalogProviderFamilyID, Version: "catalog-test-v1",
		Operations: []string{proxy.ModelOperationText}, MediaInputs: []string{},
	})
	schema.Providers = append(schema.Providers, proxy.ProviderCatalogProvider{
		ID: testCatalogProviderID, Label: "Catalog Test", KeyAcquisitionURL: "https://provider.example/keys", Aliases: []string{testCatalogProviderAlias},
		Fields: []proxy.ProviderCatalogField{
			{
				ID: testCatalogCredentialField, Label: "Access token",
				Kind: proxy.CatalogProviderFieldKindCredential, Type: proxy.CatalogProviderFieldTypeOpaque,
				Required: true, Default: &emptyValue, Secret: true,
				Validation:  proxy.ProviderCatalogFieldValidation{MinimumLength: 1},
				Environment: testCatalogCredentialEnv,
			},
			{
				ID: testCatalogSettingField, Label: "Gateway URL",
				Kind: proxy.CatalogProviderFieldKindSetting, Type: proxy.CatalogProviderFieldTypeURL,
				Required: true, Default: &emptyValue, Secret: false,
				Validation:  proxy.ProviderCatalogFieldValidation{AllowedSchemes: []string{"https", "http"}},
				Environment: testCatalogSettingEnv,
			},
		},
		Transports: []proxy.ProviderCatalogTransport{{
			ID: testCatalogTransportID,
			Endpoint: proxy.ProviderCatalogEndpoint{
				Method: proxy.CatalogEndpointMethodPost, SettingField: testCatalogSettingField, Path: "/chat/completions",
			},
			Authentication: proxy.ProviderCatalogAuthentication{
				Kind: proxy.CatalogAuthenticationBearer, Field: testCatalogCredentialField,
				Header: "Authorization", Prefix: "Bearer ",
			},
			RequestProtocol:  proxy.CatalogProtocolOpenAIChatCompletions,
			ResponseProtocol: proxy.CatalogProtocolOpenAIChatCompletions,
			UsageMapping:     proxy.CatalogProtocolOpenAIChatCompletions,
			Lifecycle:        "synchronous_completion",
			ProtocolParameters: proxy.ProviderCatalogProtocolParameters{
				ModelField: "model", TokenField: "max_tokens", MediaExecutionLifecycle: "synchronous_completion",
				OutputFields: []string{"choices[].message.content"},
				FinishRules: proxy.ProviderCatalogFinishRules{
					Complete: []string{"stop"}, Continue: []string{"length"},
				},
				ContinuationRules: []string{"append_visible_assistant_output", "request_missing_suffix"},
				ErrorRules:        []string{"content_filter", "tool_calls", "unknown_finish_reason"},
				UsageFields: proxy.ProviderCatalogUsageFields{
					Input: "usage.prompt_tokens", Output: "usage.completion_tokens", Total: "usage.total_tokens",
				},
			},
		}},
		Offerings: []proxy.ProviderCatalogOffering{{
			Model: testCatalogModelID, UpstreamModel: testCatalogUpstreamModelID,
			Transport:  testCatalogTransportID,
			Operations: []string{proxy.ModelOperationText}, DefaultOperations: []string{proxy.ModelOperationText},
			OutputTokenLimit: 4096,
			Prices: []proxy.ProviderCatalogPrice{{
				Operation: proxy.ModelOperationText, Available: false,
				Source: "https://catalog-test.example/pricing", LastVerified: "2026-08-20",
				UnavailableReason: "Published pricing is not available for the test provider.",
			}},
		}},
	})
	catalog, catalogError := proxy.NewProviderCatalog(schema)
	if catalogError != nil {
		testingInstance.Fatalf("compile catalog test provider: %v", catalogError)
	}
	return catalog
}

func assertTestCatalogManagementSchema(testingInstance *testing.T, responseBody []byte, configured bool, expectedGatewayURL string) {
	testingInstance.Helper()
	var profile struct {
		Providers []struct {
			ID                string   `json:"id"`
			KeyAcquisitionURL string   `json:"key_acquisition_url"`
			Capabilities      []string `json:"capabilities"`
			Configured        bool     `json:"configured"`
			TextModel         string   `json:"text_model"`
			Fields            []struct {
				ID          string  `json:"id"`
				Kind        string  `json:"kind"`
				Type        string  `json:"type"`
				Required    bool    `json:"required"`
				Secret      bool    `json:"secret"`
				Configured  bool    `json:"configured"`
				Value       *string `json:"value"`
				MaskedValue string  `json:"masked_value"`
				Validation  struct {
					MinimumLength  int      `json:"minimum_length"`
					AllowedSchemes []string `json:"allowed_schemes"`
				} `json:"validation"`
			} `json:"fields"`
		} `json:"providers"`
	}
	if decodeError := json.Unmarshal(responseBody, &profile); decodeError != nil {
		testingInstance.Fatalf("decode catalog management schema: %v", decodeError)
	}
	if len(profile.Providers) == 0 || profile.Providers[len(profile.Providers)-1].ID != testCatalogProviderID {
		testingInstance.Fatalf("management provider order=%v", profile.Providers)
	}
	for _, provider := range profile.Providers {
		if provider.ID != testCatalogProviderID {
			continue
		}
		if provider.KeyAcquisitionURL != "https://provider.example/keys" || !slices.Equal(provider.Capabilities, []string{proxy.ModelOperationText}) || provider.Configured != configured || provider.TextModel != testCatalogModelID || len(provider.Fields) != 2 {
			testingInstance.Fatalf("catalog management provider=%+v", provider)
		}
		credentialField := provider.Fields[0]
		settingField := provider.Fields[1]
		if credentialField.ID != testCatalogCredentialField || credentialField.Kind != proxy.CatalogProviderFieldKindCredential || credentialField.Type != proxy.CatalogProviderFieldTypeOpaque || !credentialField.Required || !credentialField.Secret || credentialField.Validation.MinimumLength != 1 {
			testingInstance.Fatalf("catalog credential field=%+v", credentialField)
		}
		if settingField.ID != testCatalogSettingField || settingField.Kind != proxy.CatalogProviderFieldKindSetting || settingField.Type != proxy.CatalogProviderFieldTypeURL || !settingField.Required || settingField.Secret || !slices.Equal(settingField.Validation.AllowedSchemes, []string{"https", "http"}) {
			testingInstance.Fatalf("catalog setting field=%+v", settingField)
		}
		if credentialField.Value != nil || settingField.Value == nil {
			testingInstance.Fatalf("catalog field value presence=%+v", provider.Fields)
		}
		if configured {
			if !credentialField.Configured || credentialField.MaskedValue == "" || !settingField.Configured || *settingField.Value != expectedGatewayURL {
				testingInstance.Fatalf("configured catalog fields=%+v", provider.Fields)
			}
		} else if credentialField.Configured || credentialField.MaskedValue != "" || settingField.Configured || *settingField.Value != "" {
			testingInstance.Fatalf("unconfigured catalog fields=%+v", provider.Fields)
		}
		return
	}
	testingInstance.Fatalf("management schema omitted test provider: %s", responseBody)
}

func assertTestCatalogRoute(testingInstance *testing.T, router http.Handler, tenantSecret string) {
	testingInstance.Helper()
	request := httptest.NewRequest(
		http.MethodGet,
		"/?key="+url.QueryEscape(tenantSecret)+"&provider="+testCatalogProviderAlias+"&model="+testCatalogModelID+"&prompt=hello",
		nil,
	)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != testCatalogProviderResponse {
		testingInstance.Fatalf("catalog route status=%d body=%s", response.Code, response.Body.String())
	}
}

func catalogDefaultOffering(schema *proxy.ProviderCatalogSchema, operation string) (*proxy.ProviderCatalogProvider, int) {
	for providerIndex := range schema.Providers {
		provider := &schema.Providers[providerIndex]
		for offeringIndex, offering := range provider.Offerings {
			if slices.Contains(offering.DefaultOperations, operation) {
				return provider, offeringIndex
			}
		}
	}
	panic("canonical catalog has no default offering for " + operation)
}

func removeCatalogValue(values []string, removed string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != removed {
			result = append(result, value)
		}
	}
	return result
}

func firstCatalogControl(testingInstance *testing.T, schema *proxy.ProviderCatalogSchema) *proxy.CatalogControl {
	testingInstance.Helper()
	for providerIndex := range schema.Providers {
		for offeringIndex := range schema.Providers[providerIndex].Offerings {
			offering := &schema.Providers[providerIndex].Offerings[offeringIndex]
			if len(offering.Controls) != 0 {
				return &offering.Controls[0]
			}
		}
	}
	testingInstance.Fatal("canonical catalog has no control")
	return nil
}

func firstCatalogLimit(testingInstance *testing.T, schema *proxy.ProviderCatalogSchema) *proxy.CatalogLimit {
	testingInstance.Helper()
	for providerIndex := range schema.Providers {
		for offeringIndex := range schema.Providers[providerIndex].Offerings {
			offering := &schema.Providers[providerIndex].Offerings[offeringIndex]
			if len(offering.Limits) != 0 {
				return &offering.Limits[0]
			}
		}
	}
	testingInstance.Fatal("canonical catalog has no limit")
	return nil
}

func mapsEqual(actual map[string]string, expected map[string]string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for key, expectedValue := range expected {
		if actual[key] != expectedValue {
			return false
		}
	}
	return true
}
