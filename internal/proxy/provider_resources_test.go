package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gopkg.in/yaml.v3"
)

func TestProviderCatalogResourcesWithoutModelOfferings(t *testing.T) {
	for _, providerID := range []string{"dictator", "resource-fixture"} {
		t.Run(providerID, func(t *testing.T) {
			schema := testfixtures.ProviderCatalog(t).Schema()
			for _, provider := range schema.Providers {
				if provider.ID != "dictator" {
					continue
				}
				provider.ID = providerID
				provider.Offerings = nil
				provider.Fields = slices.Clone(provider.Fields)
				provider.Transports = slices.Clone(provider.Transports)
				provider.Transports[0].Endpoint.SettingField = "voice_address"
				provider.Transports[0].Components.Authentication.Field = "voice_token"
				for index := range provider.Fields {
					switch provider.Fields[index].ID {
					case "grpc_address":
						provider.Fields[index].ID = "voice_address"
					case "grpc_auth_token":
						provider.Fields[index].ID = "voice_token"
					}
				}
				schema.Providers = slices.DeleteFunc(schema.Providers, func(item proxy.ProviderCatalogProvider) bool { return item.ID == providerID })
				schema.Providers = append(schema.Providers, provider)
				break
			}
			offered := map[string]bool{}
			for _, provider := range schema.Providers {
				for _, offering := range provider.Offerings {
					offered[offering.Model] = true
				}
			}
			schema.Models = slices.DeleteFunc(schema.Models, func(model proxy.ProviderCatalogModel) bool { return !offered[model.ID] })
			families, publishers := map[string]bool{}, map[string]bool{}
			for _, model := range schema.Models {
				families[model.Family] = true
				publishers[model.Publisher] = true
			}
			schema.Families = slices.DeleteFunc(schema.Families, func(family proxy.ModelFamily) bool { return !families[family.ID] })
			schema.Publishers = slices.DeleteFunc(schema.Publishers, func(publisher proxy.ModelPublisher) bool { return !publishers[publisher.ID] })
			document, err := yaml.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if err := yaml.Unmarshal(document, &raw); err != nil {
				t.Fatal(err)
			}
			for _, item := range raw["providers"].([]any) {
				provider := item.(map[string]any)
				if provider["id"] == providerID {
					provider["resources"] = []any{map[string]any{"kind": "voices", "transport": "speech"}}
				}
			}
			document, err = yaml.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			catalog, err := proxy.ParseProviderCatalog(document)
			if err != nil {
				t.Fatal(err)
			}
			fixture, listener := newDictatorAcceptanceUpstream(t, providerID)
			router := newManagementRouter(t, proxy.Configuration{ProviderCatalog: catalog})
			server := httptest.NewServer(router)
			defer server.Close()
			owner := managementSessionCookie(t, "resource-owner")
			tenantID := managementDefaultTenantTestID(t, router, owner)
			secret := generateManagementTenantSecret(t, router, owner, tenantID)
			read := func(path, key string, status int) map[string]any {
				t.Helper()
				request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, nil)
				if err != nil {
					t.Fatal(err)
				}
				if key != "" {
					request.Header.Set("Authorization", "Bearer "+key)
				}
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != status {
					t.Fatalf("GET %s status=%d body=%s", path, response.StatusCode, body)
				}
				if strings.Contains(string(body), fixture.token) || strings.Contains(string(body), listener.Addr().String()) {
					t.Fatal("private resource data exposed")
				}
				var value map[string]any
				if err := json.Unmarshal(body, &value); err != nil {
					t.Fatal(err)
				}
				return value
			}
			public := read(proxy.PublicCapabilitiesPath, "", http.StatusOK)
			publicProviderFound := false
			for _, item := range public["providers"].([]any) {
				provider := item.(map[string]any)
				if provider["identifier"] == providerID {
					publicProviderFound = true
					resources, _ := provider["resources"].([]any)
					if len(resources) != 1 || resources[0] != "voices" {
						t.Fatalf("public resources=%v", resources)
					}
				}
			}
			if !publicProviderFound {
				t.Fatal("public provider is missing")
			}
			for _, item := range public["offerings"].([]any) {
				if item.(map[string]any)["provider"] == providerID {
					t.Fatal("resource provider advertised a fake model offering")
				}
			}
			assertResources := func(key string, count int) {
				t.Helper()
				capabilities := read(llmproxycontract.MediaCapabilitiesPath, key, http.StatusOK)
				resources, ok := capabilities["resources"].([]any)
				if !ok || len(resources) != count {
					t.Fatalf("tenant resources=%v want=%d", capabilities["resources"], count)
				}
				if count > 0 && (resources[0].(map[string]any)["kind"] != "voices" || resources[0].(map[string]any)["provider"] != providerID) {
					t.Fatalf("resource=%v", resources[0])
				}
			}
			assertResources(secret, 0)
			connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Resource account", "provider": providerID, "fields": map[string]string{"voice_address": listener.Addr().String(), "voice_token": fixture.token, "grpc_tls": "false"}}, http.StatusCreated)
			assignmentPath := "/tenants/" + tenantID + "/connections/" + providerID
			accountConnectionExchange(t, router, owner, http.MethodPut, assignmentPath, map[string]string{"connection_id": connection["id"].(string)}, http.StatusOK)
			assertResources(secret, 1)
			clientConfig, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(clientConfig, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			capabilities, err := client.GetMediaCapabilities(t.Context())
			if err != nil || len(capabilities.Resources) != 1 || capabilities.Resources[0].Provider != providerID {
				t.Fatalf("official client resources=%v error=%v", capabilities.Resources, err)
			}
			voices := read(llmproxycontract.MediaVoicesPath+"?provider="+providerID, secret, http.StatusOK)["voices"].([]any)
			if len(voices) != 1 {
				t.Fatalf("voices=%v", voices)
			}
			voiceID := voices[0].(map[string]any)["voice_id"].(string)
			foreign := managementSessionCookie(t, "foreign-resource-owner")
			foreignID := managementDefaultTenantTestID(t, router, foreign)
			foreignSecret := generateManagementTenantSecret(t, router, foreign, foreignID)
			assertResources(foreignSecret, 0)
			read(llmproxycontract.MediaVoicesPath+"/"+voiceID, foreignSecret, http.StatusNotFound)
			replacement, replacementListener := newDictatorAcceptanceUpstream(t, providerID)
			connectionPath := "/connections/" + connection["id"].(string)
			current := accountConnectionExchange(t, router, owner, http.MethodGet, connectionPath, nil, http.StatusOK)
			accountConnectionExchange(t, router, owner, http.MethodPut, connectionPath, map[string]any{"name": "Resource account", "provider": providerID, "version": current["version"], "fields": map[string]string{"voice_address": replacementListener.Addr().String(), "voice_token": replacement.token, "grpc_tls": "false"}}, http.StatusOK)
			updatedVoices := read(llmproxycontract.MediaVoicesPath+"?provider="+providerID, secret, http.StatusOK)["voices"].([]any)
			if len(updatedVoices) != 1 || updatedVoices[0].(map[string]any)["voice_id"] == voiceID {
				t.Fatal("resource discovery retained the replaced connection")
			}
			assertResources(secret, 1)
			accountConnectionExchange(t, router, owner, http.MethodDelete, assignmentPath, nil, http.StatusNoContent)
			assertResources(secret, 0)
		})
	}
}

func TestProviderCatalogResourcesRejectUnsupportedBindings(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		resources []proxy.ProviderCatalogResource
	}{
		{"unknown kind", []proxy.ProviderCatalogResource{{Kind: "unknown", Transport: "speech"}}},
		{"duplicate kind", []proxy.ProviderCatalogResource{{Kind: "voices", Transport: "speech"}, {Kind: "voices", Transport: "speech"}}},
		{"dangling transport", []proxy.ProviderCatalogResource{{Kind: "voices", Transport: "absent"}}},
		{"history protocol", []proxy.ProviderCatalogResource{{Kind: "history", Transport: "speech"}}},
		{"library protocol", []proxy.ProviderCatalogResource{{Kind: "voice_library", Transport: "speech"}}},
		{"dictionary protocol", []proxy.ProviderCatalogResource{{Kind: "pronunciation_dictionaries", Transport: "speech"}}},
		{"metadata protocol", []proxy.ProviderCatalogResource{{Kind: "metadata", Transport: "speech"}}},
		{"quota protocol", []proxy.ProviderCatalogResource{{Kind: "quotas", Transport: "speech"}}},
		{"element protocol", []proxy.ProviderCatalogResource{{Kind: "elements", Transport: "speech"}}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			schema := testfixtures.ProviderCatalog(t).Schema()
			for index := range schema.Providers {
				if schema.Providers[index].ID == "dictator" {
					schema.Providers[index].Resources = scenario.resources
				}
			}
			if _, err := proxy.NewProviderCatalog(schema); err == nil {
				t.Fatal("invalid resource binding accepted")
			}
		})
	}
}
