package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

const activationDisabledTextModel = "gpt-4o-mini"

func TestModelActivationCatalogServiceRequiresRuntimeDefault(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		catalog := testfixtures.ProviderCatalog(t).ModelCatalog()
		for index := range catalog.Offerings {
			offering := &catalog.Offerings[index]
			if offering.Provider == "openai" {
				offering.DefaultOperations = nil
				if duplicate {
					offering.DefaultOperations = append([]string(nil), offering.Operations...)
				}
			}
		}
		if _, err := proxy.NewCatalogService(catalog); err == nil || !strings.Contains(err.Error(), "default_count=") {
			t.Fatalf("duplicate=%t error=%v", duplicate, err)
		}
	}
}

func TestModelActivationExcludesUnusedFamiliesFromHTTP(t *testing.T) {
	for _, activation := range []proxy.ModelActivation{proxy.ModelDisabled, proxy.ModelEnabled} {
		schema := testfixtures.ProviderCatalog(t).Schema()
		for index := range schema.Models {
			if schema.Models[index].ID == proxy.ModelNameMiniMaxM3 {
				schema.Models[index].Enabled = activation
			}
		}
		catalog, err := proxy.NewProviderCatalog(schema)
		if err != nil {
			t.Fatal(err)
		}
		if len(catalog.Schema().Families) != len(schema.Families) {
			t.Fatal("private family metadata was removed")
		}
		router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog}, zap.NewNop().Sugar())
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/public/capabilities", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("capabilities status=%d", response.Code)
		}
		var public proxy.PublicCapabilityCatalog
		if err := json.Unmarshal(response.Body.Bytes(), &public); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, family := range public.Families {
			if family.Identifier == proxy.ModelNameMiniMaxM3 {
				found = true
			}
		}
		if found != (activation == proxy.ModelEnabled) {
			t.Fatalf("M3 family discovery=%t activation=%v", found, activation)
		}
	}
}

func modelActivationDocument(t *testing.T) string {
	t.Helper()
	document, err := os.ReadFile("../../configs/providers.yml")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(document), "\nmodels:\n", 2)
	modelParts := strings.SplitN(parts[1], "\nproviders:\n", 2)
	modelParts[0] = strings.ReplaceAll(modelParts[0], "      enabled: false\n", "      enabled: true\n")
	modelParts[1] = strings.Replace(modelParts[1], "          upstream_model: muse-voice-transcribe-1.0\n          transport: dictation\n", "          upstream_model: muse-voice-transcribe-1.0\n          transport: dictation\n          default_operations:\n            - dictation\n", 1)
	return parts[0] + "\nmodels:\n" + modelParts[0] + "\nproviders:\n" + modelParts[1]
}

func TestModelActivationRequiresExplicitBoolean(t *testing.T) {
	document := modelActivationDocument(t)
	if _, err := proxy.ParseProviderCatalog([]byte(document)); err != nil {
		t.Fatalf("explicitly enabled catalog must load: %v", err)
	}
	for _, value := range []string{"", "      enabled: null\n", "      enabled: 1\n", "      enabled: \"true\"\n", "      enabled: yes\n"} {
		t.Run(value, func(t *testing.T) {
			invalid := strings.Replace(document, "      enabled: true\n", value, 1)
			if _, err := proxy.ParseProviderCatalog([]byte(invalid)); err == nil {
				t.Fatal("catalog accepted a missing or non-Boolean activation field")
			}
		})
	}
	disabledDefault := strings.Replace(document, "    - id: gpt-4.1\n      enabled: true", "    - id: gpt-4.1\n      enabled: false", 1)
	if _, err := proxy.ParseProviderCatalog([]byte(disabledDefault)); err == nil || !strings.Contains(err.Error(), "disabled_default") {
		t.Fatalf("disabled provider default error=%v", err)
	}
}

func TestModelActivationValidatesRetainedMetadataAndMigrationTargets(t *testing.T) {
	for _, scenario := range []string{"invalid metadata", "disabled migration target", "missing activation", "invalid activation"} {
		t.Run(scenario, func(t *testing.T) {
			schema := testfixtures.ProviderCatalog(t).Schema()
			for index := range schema.Models {
				if schema.Models[index].ID == activationDisabledTextModel {
					schema.Models[index].Enabled = proxy.ModelDisabled
					if scenario == "missing activation" {
						schema.Models[index].Enabled = 0
					}
					if scenario == "invalid activation" {
						schema.Models[index].Enabled = 99
					}
				}
			}
			if scenario == "invalid metadata" {
				for providerIndex := range schema.Providers {
					for offeringIndex := range schema.Providers[providerIndex].Offerings {
						offering := &schema.Providers[providerIndex].Offerings[offeringIndex]
						if offering.Model == activationDisabledTextModel {
							offering.Prices[0].Source = "invalid-url"
						}
					}
				}
			}
			if scenario == "disabled migration target" {
				schema.ModelMigrations = append(schema.ModelMigrations, proxy.ProviderCatalogModelMigration{
					ManagedSchemaVersion: 11, Provider: proxy.ProviderNameOpenAI, Operation: proxy.ModelOperationText,
					SourceModel: "retired-activation-model", TargetModel: activationDisabledTextModel,
				})
			}
			if _, err := proxy.NewProviderCatalog(schema); err == nil {
				t.Fatal("invalid disabled catalog was accepted")
			}
		})
	}
}

func TestModelActivationExcludesDisabledRoutesFromHTTP(t *testing.T) {
	document := strings.Replace(modelActivationDocument(t), "    - id: "+activationDisabledTextModel+"\n      enabled: true", "    - id: "+activationDisabledTextModel+"\n      enabled: false", 1)
	catalog, err := proxy.ParseProviderCatalog([]byte(document))
	if err != nil {
		t.Fatalf("disabled nondefault catalog must load: %v", err)
	}
	if len(catalog.Schema().Models) != len(catalog.ModelCatalog().Models)+1 {
		t.Fatal("private catalog must retain the disabled model while runtime discovery excludes it")
	}
	for _, offering := range catalog.ModelCatalog().Offerings {
		if offering.Model == activationDisabledTextModel {
			t.Fatal("runtime retained a disabled offering")
		}
	}
	for _, price := range catalog.ModelCatalog().Prices {
		if price.Model == activationDisabledTextModel {
			t.Fatal("runtime retained a disabled price")
		}
	}
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"activation-response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"enabled route works"}]}]}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{
		ProviderCatalog: catalog,
		Endpoints:       providerEndpointOverrides(map[string]string{proxy.ProviderNameOpenAI: upstream.URL}, nil),
	}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, path := range []string{proxy.PublicCapabilitiesPath, "/v1/models"} {
		req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+TestSecret)
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || strings.Contains(string(body), `"`+activationDisabledTextModel+`"`) || strings.Contains(string(body), `openai/`+activationDisabledTextModel+`"`) {
			t.Fatalf("discovery path=%s status=%d body=%s", path, response.StatusCode, body)
		}
	}
	calls.Store(0)
	response, err := server.Client().Get(server.URL + "/?key=" + TestSecret + "&provider=openai&model=" + activationDisabledTextModel + "&prompt=test")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest || calls.Load() != 0 {
		t.Fatalf("disabled route status=%d upstream calls=%d", response.StatusCode, calls.Load())
	}
	response, err = server.Client().Get(server.URL + "/?key=" + TestSecret + "&provider=openai&model=gpt-4.1&prompt=test")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("enabled route status=%d upstream calls=%d", response.StatusCode, calls.Load())
	}
	management := newManagementRouterWithDatabasePath(t, proxy.Configuration{ProviderCatalog: catalog}, filepath.Join(t.TempDir(), "activation.db"))
	cookie := managementSessionCookie(t, "activation-owner")
	path := managementDefaultTenantTestPath(t, management, cookie, "")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	management.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), `"`+activationDisabledTextModel+`"`) {
		t.Fatalf("management exposed disabled model: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
