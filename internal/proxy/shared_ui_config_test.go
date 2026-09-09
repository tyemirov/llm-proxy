package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"gopkg.in/yaml.v3"
)

func TestManagementCurrentSharedUIConfigHTTP(t *testing.T) {
	apiServer := httptest.NewServer(newManagementRouter(t, proxy.Configuration{}))
	defer apiServer.Close()
	staticServer := httptest.NewServer(http.FileServer(http.Dir("../../site")))
	defer staticServer.Close()

	for _, scenario := range []struct {
		name, origin, authOrigin, tenant, clientID string
	}{
		{"API", apiServer.URL, "http://localhost:8443", "llm-proxy-test", "google-client-id"},
		{"static", staticServer.URL, "https://tauth-api.mprlab.com", "llm-proxy", "925457785190-3frk7j3bsr3ucidtkcohrp2sl07e0paa.apps.googleusercontent.com"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, scenario.origin+proxy.ManagementConfigUIPath, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Origin", "http://localhost:8080")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("config status=%d", response.StatusCode)
			}
			if scenario.name == "API" && response.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("API config cache policy=%q", response.Header.Get("Cache-Control"))
			}
			var config struct {
				Environments []struct {
					Auth map[string]any `yaml:"auth"`
				} `yaml:"environments"`
			}
			if err := yaml.NewDecoder(response.Body).Decode(&config); err != nil {
				t.Fatal(err)
			}
			if len(config.Environments) != 1 {
				t.Fatalf("environment count=%d", len(config.Environments))
			}
			expected := map[string]any{
				"tauthUrl": scenario.authOrigin, "tenantId": scenario.tenant,
				"sessionPath": "/auth/session", "logoutPath": "/auth/logout",
				"providers": map[string]any{
					"google": map[string]any{"enabled": true, "clientId": scenario.clientID, "loginPath": "/auth/google", "noncePath": "/auth/nonce"},
					"apple":  map[string]any{"enabled": false}, "password": map[string]any{"enabled": false},
				},
			}
			if !reflect.DeepEqual(config.Environments[0].Auth, expected) {
				t.Fatalf("provider map mismatch: got=%#v want=%#v", config.Environments[0].Auth, expected)
			}
		})
	}
}
