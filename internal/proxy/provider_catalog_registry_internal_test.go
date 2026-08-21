package proxy

import "testing"

func TestProviderCatalogEndpointOverridesCoverEveryScope(t *testing.T) {
	catalogEndpoint := ProviderCatalogEndpoint{Path: "/messages"}
	if endpoint := (&Endpoints{}).providerTransportEndpoint("provider", "text", catalogEndpoint); endpoint != "" {
		t.Fatalf("empty endpoint override=%q", endpoint)
	}

	providerEndpoints := &Endpoints{}
	providerEndpoints.SetProviderBaseURL(" provider ", " https://provider.example/ ")
	if endpoint := providerEndpoints.providerTransportEndpoint("provider", "text", catalogEndpoint); endpoint != "https://provider.example/messages" {
		t.Fatalf("provider endpoint override=%q", endpoint)
	}

	transportEndpoints := &Endpoints{}
	transportEndpoints.SetProviderTransportBaseURL(" provider ", " text ", " https://transport.example/ ")
	if endpoint := transportEndpoints.providerTransportEndpoint("provider", "text", catalogEndpoint); endpoint != "https://transport.example/messages" {
		t.Fatalf("transport endpoint override=%q", endpoint)
	}

	completeEndpoints := &Endpoints{}
	completeEndpoints.SetProviderTransportURL(" provider ", " text ", " https://complete.example/custom ")
	if endpoint := completeEndpoints.providerTransportEndpoint("provider", "text", catalogEndpoint); endpoint != "https://complete.example/custom" {
		t.Fatalf("complete endpoint override=%q", endpoint)
	}
}

func TestProviderRegistryConsumesValidatedConnectionsAndSettingEndpoints(t *testing.T) {
	catalog := internalCanonicalProviderCatalog()
	configuration, configurationError := NewConfiguration(Configuration{
		ProviderCatalog: catalog,
		ProviderConnectionValues: map[string]map[string]string{
			ProviderNameOpenAI: {CatalogCredentialAPIKey: "static-openai-key"},
		},
		AssetStorePath: t.TempDir(),
		Management:     ManagedRouterTestManagementConfiguration(),
	})
	if configurationError != nil {
		t.Fatalf("validate catalog registry configuration: %v", configurationError)
	}
	registry := newProviderRegistry(configuration)
	if registry.definitions[providerID(ProviderNameOpenAI)].connectionValues[CatalogCredentialAPIKey] != "static-openai-key" {
		t.Fatal("catalog registry omitted validated provider connection")
	}

	schema := catalog.Schema()
	dashScopeIndex := -1
	for providerIndex := range schema.Providers {
		if schema.Providers[providerIndex].ID == ProviderNameDashScope {
			dashScopeIndex = providerIndex
			break
		}
	}
	if dashScopeIndex < 0 {
		t.Fatal("canonical catalog omitted DashScope")
	}
	dashScopeProvider := schema.Providers[dashScopeIndex]
	schema.Providers = append([]ProviderCatalogProvider{dashScopeProvider}, append(schema.Providers[:dashScopeIndex], schema.Providers[dashScopeIndex+1:]...)...)
	settingFirstCatalog, catalogError := NewProviderCatalog(schema)
	if catalogError != nil {
		t.Fatalf("compile setting-first provider catalog: %v", catalogError)
	}
	settingFirstRegistry := newProviderRegistry(Configuration{ProviderCatalog: settingFirstCatalog, Endpoints: NewEndpoints()})
	for _, transport := range settingFirstRegistry.definitions[providerID(ProviderNameDashScope)].transports {
		if transport.endpoint.SettingField != "" && transport.endpointURLOverride != "" {
			t.Fatalf("setting-backed transport received default endpoint override: %+v", transport)
		}
	}
}

func TestProviderDefinitionRejectsUnknownTransport(t *testing.T) {
	definition := providerDefinition{transports: map[string]providerTransportDefinition{
		"text": {identifier: "text", endpoint: ProviderCatalogEndpoint{DefaultBaseURL: "https://provider.example", Path: "/messages"}},
	}}
	if _, found := definition.resolvedTransport("missing"); found {
		t.Fatal("provider definition resolved an unknown transport")
	}
	if resolved, found := definition.resolvedTransport("text"); !found || resolved.textEndpointURL != "https://provider.example/messages" {
		t.Fatalf("resolved provider transport=%+v found=%t", resolved, found)
	}
}
