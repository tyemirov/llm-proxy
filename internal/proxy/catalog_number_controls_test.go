package proxy_test

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"gopkg.in/yaml.v3"
)

func numberControlCatalogDocument(t *testing.T, control map[string]any) []byte {
	t.Helper()
	document, err := yaml.Marshal(catalogWithTestProvider(t).Schema())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(document, &raw); err != nil {
		t.Fatal(err)
	}
	for _, value := range raw["providers"].([]any) {
		provider := value.(map[string]any)
		if provider["id"] == testCatalogProviderID {
			provider["offerings"].([]any)[0].(map[string]any)["controls"] = []any{control}
		}
	}
	document, err = yaml.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func TestProviderCatalogNumberControlsReachPublicHTTP(t *testing.T) {
	for _, bounds := range [][2]float64{{0.7, 1.2}, {-0.5, 0.5}, {0, 0}} {
		catalog, err := proxy.ParseProviderCatalog(numberControlCatalogDocument(t, map[string]any{"id": "speed", "kind": "number", "minimum": bounds[0], "maximum": bounds[1]}))
		if err != nil {
			t.Fatal(err)
		}
		public, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: catalog})
		if err != nil {
			t.Fatal(err)
		}
		snapshot := catalog.ModelCatalog()
		service, err := proxy.NewCatalogService(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		for index := range snapshot.Offerings {
			if snapshot.Offerings[index].Provider == testCatalogProviderID {
				*snapshot.Offerings[index].Controls[0].Minimum = 99
			}
		}
		selected, err := service.ResolveOffering(testCatalogProviderID, testCatalogModelID)
		if err != nil || *selected.Controls[0].Minimum != bounds[0] {
			t.Fatalf("mutable catalog bounds: %v", err)
		}
		*selected.Controls[0].Minimum = 99
		selected, err = service.ResolveOffering(testCatalogProviderID, testCatalogModelID)
		if err != nil || *selected.Controls[0].Minimum != bounds[0] {
			t.Fatalf("mutable resolved bounds: %v", err)
		}
		server := httptest.NewServer(proxy.BuildPublicCapabilityRouter(public, "error"))
		t.Cleanup(server.Close)
		response, err := server.Client().Get(server.URL + proxy.PublicCapabilitiesPath)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("status=%d error=%v", response.StatusCode, err)
		}
		contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
		if err != nil {
			t.Fatal(err)
		}
		if err := contract.ValidateResponse(proxy.PublicCapabilitiesPath, http.MethodGet, response.StatusCode, response.Header, body); err != nil {
			t.Fatal(err)
		}
		var result struct {
			Offerings []struct {
				Provider string `json:"provider"`
				Controls []struct {
					Kind    string  `json:"kind"`
					Minimum float64 `json:"minimum"`
					Maximum float64 `json:"maximum"`
				} `json:"controls"`
			} `json:"offerings"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, offering := range result.Offerings {
			if offering.Provider != testCatalogProviderID {
				continue
			}
			found = true
			if len(offering.Controls) != 1 || offering.Controls[0].Kind != "number" || offering.Controls[0].Minimum != bounds[0] || offering.Controls[0].Maximum != bounds[1] {
				t.Fatalf("controls=%+v", offering.Controls)
			}
		}
		if !found {
			t.Fatal("catalog omitted provider")
		}
	}
}

func TestProviderCatalogNumberControlsReachTenantClient(t *testing.T) {
	schema := testfixtures.ProviderCatalog(t).Schema()
	minimum, maximum := 0.7, 1.2
	for providerIndex := range schema.Providers {
		provider := &schema.Providers[providerIndex]
		if provider.ID != "dictator" {
			continue
		}
		for index := range provider.Offerings {
			if provider.Offerings[index].Model == "qwen3-tts" {
				provider.Offerings[index].Controls = append(provider.Offerings[index].Controls, proxy.CatalogControl{ID: "test-decimal", Kind: "number", Minimum: &minimum, Maximum: &maximum})
			}
		}
	}
	document, err := yaml.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := proxy.ParseProviderCatalog(document)
	if err != nil {
		t.Fatal(err)
	}
	fixture, listener := newDictatorAcceptanceUpstream(t, "dictator")
	router := newManagementRouter(t, proxy.Configuration{ProviderCatalog: catalog})
	server := httptest.NewServer(router)
	defer server.Close()
	owner := managementSessionCookie(t, "number-controls-owner")
	tenantID := managementDefaultTenantTestID(t, router, owner)
	secret := generateManagementTenantSecret(t, router, owner, tenantID)
	connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Number controls", "provider": "dictator", "fields": map[string]string{"grpc_address": listener.Addr().String(), "grpc_auth_token": fixture.token, "grpc_tls": "false"}}, http.StatusCreated)
	accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenantID+"/connections/dictator", map[string]string{"connection_id": connection["id"].(string)}, http.StatusOK)
	configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(configuration, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := client.GetMediaCapabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, route := range capabilities.Routes {
		if route.Provider != "dictator" || route.Model != "qwen3-tts" {
			continue
		}
		for _, data := range route.Controls {
			var control proxy.CatalogControl
			if err := json.Unmarshal(data, &control); err != nil {
				t.Fatal(err)
			}
			if control.ID == "test-decimal" {
				found = true
				if control.Kind != "number" || *control.Minimum != minimum || *control.Maximum != maximum {
					t.Fatalf("client bounds=%s", data)
				}
			}
		}
	}
	if !found {
		t.Fatal("tenant discovery omitted numeric control")
	}
}

func TestProviderCatalogNumberControlsRejectInvalidBounds(t *testing.T) {
	for name, control := range map[string]map[string]any{
		"missing minimum":            {"kind": "number", "maximum": 1.2},
		"missing maximum":            {"kind": "number", "minimum": 0.7},
		"inverted":                   {"kind": "number", "minimum": 1.2, "maximum": 0.7},
		"not a number":               {"kind": "number", "minimum": math.NaN(), "maximum": 1.2},
		"infinite minimum":           {"kind": "number", "minimum": math.Inf(-1), "maximum": 1.2},
		"infinite maximum":           {"kind": "number", "minimum": 0.7, "maximum": math.Inf(1)},
		"enum values":                {"kind": "number", "minimum": 0.7, "maximum": 1.2, "values": []string{"auto"}},
		"fractional integer":         {"kind": "integer", "minimum": 0.7, "maximum": 1.2},
		"negative integer":           {"kind": "integer", "minimum": -1, "maximum": 2},
		"fractional integer maximum": {"kind": "integer", "minimum": 0, "maximum": 1.2},
	} {
		t.Run(name, func(t *testing.T) {
			control["id"] = "speed"
			if _, err := proxy.ParseProviderCatalog(numberControlCatalogDocument(t, control)); err == nil {
				t.Fatal("catalog accepted invalid numeric control")
			}
		})
	}
}
