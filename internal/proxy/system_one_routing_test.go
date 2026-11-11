package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func TestSystemOneConfiguredProtocolRouting(t *testing.T) {
	for _, scenario := range []struct {
		name, model       string
		direct, qualified bool
		status            int
		selected          string
	}{
		{"protocol clone accepts upstream alias", "jev-latest", false, true, 200, "qualified-decision"},
		{"protocol clone accepts preview unchanged", "jev-preview", false, true, 200, "qualified-decision"},
		{"ambiguous alias rejects before dispatch", "jev-latest", true, true, 400, ""},
		{"exact direct model takes precedence", "jev-1.14.0", true, true, 200, "typesafe"},
		{"exact qualified model takes precedence", "qualified-jev", true, true, 200, "qualified-decision"},
		{"exact upstream name takes precedence", "qualified-upstream-jev", true, true, 200, "qualified-decision"},
		{"unconfigured clone ignored", "jev-latest", true, false, 200, "typesafe"},
		{"all unconfigured reject", "jev-latest", false, false, 400, ""},
		{"exact unconfigured route rejects", "qualified-jev", true, false, 400, ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			calls := map[string]int{}
			upstream := func(provider string) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls[provider]++
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					var request struct {
						Model string `json:"model"`
					}
					if err := json.Unmarshal(body, &request); err != nil {
						t.Error(err)
					}
					if request.Model != scenario.model {
						t.Errorf("model=%s want=%s", request.Model, scenario.model)
					}
					if r.Header.Get("Authorization") != "Bearer "+provider+"-secret" {
						t.Error("credential crossed provider boundary")
					}
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"model":"jev-1.14.0","answers":{"ok":{"type":"noul","noul":0.7}},"usage":{"input_tokens":1,"output_tokens":1}}`)
				}))
			}
			direct := upstream("typesafe")
			defer direct.Close()
			qualified := upstream("qualified-decision")
			defer qualified.Close()
			schema := testfixtures.ProviderCatalog(t).Schema()
			var clone proxy.ProviderCatalogProvider
			for i := range schema.Providers {
				if schema.Providers[i].ID == proxy.ProviderNameTypeSafe {
					clone = schema.Providers[i]
					for j := range schema.Providers[i].Transports {
						schema.Providers[i].Transports[j].Endpoint.DefaultBaseURL = direct.URL
					}
				}
			}
			clone.ID = "qualified-decision"
			clone.Label = "Qualified decision"
			clone.APIServiceLabel = "Qualified decision API"
			clone.Transports = append([]proxy.ProviderCatalogTransport(nil), clone.Transports...)
			for i := range clone.Transports {
				clone.Transports[i].Endpoint.DefaultBaseURL = qualified.URL
			}
			clone.Offerings = append([]proxy.ProviderCatalogOffering(nil), clone.Offerings...)
			clone.Offerings[0].Model = "qualified-jev"
			clone.Offerings[0].UpstreamModel = "qualified-upstream-jev"
			schema.Providers = append(schema.Providers, clone)
			for _, model := range schema.Models {
				if model.ID == proxy.ModelNameJev1140 {
					model.ID = "qualified-jev"
					model.Version = "qualified-jev"
					schema.Models = append(schema.Models, model)
					break
				}
			}
			catalog, err := proxy.NewProviderCatalog(schema)
			if err != nil {
				t.Fatal(err)
			}
			tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
			if scenario.direct {
				tenant.ProviderKeys["typesafe"] = "typesafe-secret"
			}
			if scenario.qualified {
				tenant.ProviderKeys["qualified-decision"] = "qualified-decision-secret"
			}
			router, err := buildRouterWithManagedTenant(t, proxy.Configuration{ProviderCatalog: catalog}, zap.NewNop().Sugar(), tenant)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/v1/systemone", strings.NewReader(`{"model":"`+scenario.model+`","state":"sample","questions":{"ok":{"type":"noul"}}}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+TestSecret)
			request.Header.Set("Content-Type", "application/json")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != scenario.status {
				t.Fatalf("status=%d want=%d body=%s", response.StatusCode, scenario.status, body)
			}
			for _, provider := range []string{"typesafe", "qualified-decision"} {
				expected := 0
				if provider == scenario.selected {
					expected = 1
				}
				if calls[provider] != expected {
					t.Errorf("%s dispatch=%d want=%d", provider, calls[provider], expected)
				}
			}
		})
	}
}
