package proxy_test

import (
	"bytes"
	"encoding/json"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
)

func TestProviderCatalogTypeSafePublicCapabilities(t *testing.T) {
	catalog := testfixtures.ProviderCatalog(t)
	capabilities, err := proxy.NewPublicCapabilityCatalog(proxy.Configuration{ProviderCatalog: catalog})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy.BuildPublicCapabilityRouter(capabilities, "error"))
	defer server.Close()
	response, err := http.Get(server.URL + proxy.PublicCapabilitiesPath)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var actual proxy.PublicCapabilityCatalog
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatal(err)
	}
	for _, offering := range actual.Offerings {
		if offering.Provider != "typesafe" {
			continue
		}
		if offering.Model != "jev-1.14.0" || offering.WireContract != "system_one" || offering.ExecutionLifecycle != "synchronous_completion" || !slices.Equal(offering.Capabilities, []string{"image_input", "typed_decision"}) || !slices.Equal(offering.Domains, []string{"decision", "image"}) {
			t.Fatalf("TypeSafe offering=%+v", offering)
		}
		if !slices.Equal(offering.ImageMIMETypes, []string{"image/png", "image/jpeg"}) || offering.MediaExecutionLifecycle != "synchronous_completion" {
			t.Fatalf("TypeSafe image metadata=%+v", offering)
		}
		if len(offering.Limits) != 2 {
			t.Fatalf("TypeSafe limits=%+v", offering.Limits)
		}
		for _, provider := range catalog.Schema().Providers {
			if provider.ID == "typesafe" {
				if provider.KeyAcquisitionURL != "https://console.typesafe.ai/keys" || provider.Verification.Transport != "models" || provider.Verification.Model != "" {
					t.Fatalf("TypeSafe provider=%+v", provider)
				}
				return
			}
		}
	}
	t.Fatal("TypeSafe typed decision offering is missing")
}

func TestProviderCatalogTypeSafeConnectionVerification(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" || r.ContentLength > 0 {
			t.Errorf("verification=%s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
			return
		}
		if r.Header.Get("Authorization") == "Bearer rejected-secret" {
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("Authorization") != "Bearer first-secret" && r.Header.Get("Authorization") != "Bearer rotated-secret" {
			t.Errorf("unexpected credential header")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"models":[{"name":"jev-1.14.0","description":"Jev","release_date":"2026-11-11"}]}`)
	}))
	defer upstream.Close()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for i := range schema.Providers {
		if schema.Providers[i].ID == "typesafe" {
			for j := range schema.Providers[i].Transports {
				schema.Providers[i].Transports[j].Endpoint.DefaultBaseURL = upstream.URL
			}
		}
	}
	data, err := yaml.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := proxy.ParseProviderCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	configuration := testfixtures.WithUpstreamCapacity(t, proxy.Configuration{ProviderCatalog: catalog})
	router := newOperationalProviderKeyVerificationRouter(t, configuration, zap.NewNop().Sugar(), t.TempDir()+"/managed.db", TestTimeout)
	server := httptest.NewServer(router)
	defer server.Close()
	owner := managementSessionCookie(t, "typesafe-owner")
	foreign := managementSessionCookie(t, "typesafe-foreign")
	exchange := func(cookie *http.Cookie, method, path string, body any, status int) map[string]any {
		t.Helper()
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), method, server.URL+"/api/management"+path, bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if method == http.MethodPost {
			req.Header.Set("Idempotency-Key", "typesafe-create")
		}
		req.AddCookie(cookie)
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		content, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != status {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, path, res.StatusCode, status, content)
		}
		if strings.Contains(string(content), "first-secret") || strings.Contains(string(content), "rotated-secret") {
			t.Fatal("response exposed credential")
		}
		var result map[string]any
		if len(content) > 0 && res.StatusCode < 400 {
			if err := json.Unmarshal(content, &result); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	connection := exchange(owner, http.MethodPost, "/connections", map[string]any{"name": "TypeSafe", "provider": "typesafe", "fields": map[string]string{"api_key": "first-secret"}}, 201)
	path := "/connections/" + connection["id"].(string)
	exchange(foreign, http.MethodGet, path, nil, 404)
	tenant := managementDefaultTenantTestID(t, router, owner)
	exchange(owner, http.MethodPut, "/tenants/"+tenant+"/connections/typesafe", map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, 200)
	connection = exchange(owner, http.MethodGet, path, nil, 200)
	update := map[string]any{"name": "TypeSafe", "provider": "typesafe", "version": connection["version"], "fields": map[string]string{"api_key": "rejected-secret"}}
	exchange(owner, http.MethodPut, path, update, 422)
	unchanged := exchange(owner, http.MethodGet, path, nil, 200)
	if unchanged["version"] != connection["version"] {
		t.Fatal("rejected verification changed connection")
	}
	update["fields"] = map[string]string{"api_key": "rotated-secret"}
	exchange(owner, http.MethodPut, path, update, 200)
	if calls.Load() != 3 {
		t.Fatalf("verification calls=%d want=3", calls.Load())
	}
}

func TestProviderCatalogTypeSafeAuthenticatedDiscovery(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(strconv.FormatBool(configured), func(t *testing.T) {
			tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
			if configured {
				tenant.ProviderKeys[proxy.ProviderNameTypeSafe] = "typesafe-discovery-secret"
			}
			router, err := buildRouterWithManagedTenant(t, proxy.Configuration{}, zap.NewNop().Sugar(), tenant)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/v1/models", nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+TestSecret)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var result struct {
				Data []struct {
					ID      string `json:"id"`
					OwnedBy string `json:"owned_by"`
				} `json:"data"`
			}
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, model := range result.Data {
				if model.ID == proxy.ModelNameJev1140 {
					found = true
					if model.OwnedBy != proxy.ProviderNameTypeSafe {
						t.Fatalf("owner=%s", model.OwnedBy)
					}
				}
				if strings.HasPrefix(model.ID, "typesafe/") {
					t.Fatalf("discovery changed TypeSafe model syntax: %s", model.ID)
				}
			}
			if response.StatusCode != http.StatusOK || found != configured {
				t.Fatalf("status=%d found=%v configured=%v", response.StatusCode, found, configured)
			}
		})
	}
}

func TestProviderCatalogTypeSafeRejectsUnqualifiedProtocol(t *testing.T) {
	for _, invalid := range []string{"protocol", "lifecycle"} {
		t.Run(invalid, func(t *testing.T) {
			catalog := testfixtures.ProviderCatalog(t).ModelCatalog()
			for i := range catalog.Offerings {
				if catalog.Offerings[i].Provider == proxy.ProviderNameTypeSafe {
					if invalid == "protocol" {
						catalog.Offerings[i].WireContract = proxy.CatalogProtocolOpenAIResponses
					} else {
						catalog.Offerings[i].ExecutionLifecycle = "pollable_resource"
					}
				}
			}
			if _, err := proxy.NewCatalogService(catalog); err == nil || !strings.Contains(err.Error(), "unsupported_typed_decision_route") {
				t.Fatalf("unqualified TypeSafe %s error=%v", invalid, err)
			}
		})
	}
}

func TestProviderCatalogTypeSafeModelListVerificationShape(t *testing.T) {
	for _, scenario := range []struct {
		name, body string
		status     int
	}{
		{"missing models", `{}`, 503},
		{"null models", `{"models":null}`, 503},
		{"non array models", `{"models":{}}`, 503},
		{"missing metadata", `{"models":[{"name":"jev-latest"}]}`, 503},
		{"wrong metadata type", `{"models":[{"name":4,"description":"Jev","release_date":"2026-11-11"}]}`, 503},
		{"null metadata", `{"models":[{"name":null,"description":"Jev","release_date":"2026-11-11"}]}`, 503},
		{"empty list", `{"models":[]}`, 201},
		{"metadata", `{"models":[{"name":"jev-latest","description":"Jev","release_date":"2026-11-11"}]}`, 201},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, scenario.body)
			}))
			defer upstream.Close()
			schema := testfixtures.ProviderCatalog(t).Schema()
			for i := range schema.Providers {
				if schema.Providers[i].ID == proxy.ProviderNameTypeSafe {
					for j := range schema.Providers[i].Transports {
						schema.Providers[i].Transports[j].Endpoint.DefaultBaseURL = upstream.URL
					}
				}
			}
			catalog, err := proxy.NewProviderCatalog(schema)
			if err != nil {
				t.Fatal(err)
			}
			router := newOperationalProviderKeyVerificationRouter(t, testfixtures.WithUpstreamCapacity(t, proxy.Configuration{ProviderCatalog: catalog}), zap.NewNop().Sugar(), t.TempDir()+"/management.db", TestTimeout)
			server := httptest.NewServer(router)
			defer server.Close()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/management/connections", strings.NewReader(`{"name":"TypeSafe","provider":"typesafe","fields":{"api_key":"candidate-secret"}}`))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "models-shape")
			req.AddCookie(managementSessionCookie(t, "models-shape-owner"))
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != scenario.status {
				t.Fatalf("status=%d want=%d body=%s", response.StatusCode, scenario.status, body)
			}
		})
	}
}
