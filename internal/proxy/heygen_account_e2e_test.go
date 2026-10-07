package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestHeyGenAccountRejectsInvalidProviderObservations(t *testing.T) {
	for _, body := range []string{`{`, `{"data":{"billing_type":"future"}}`, `{"data":{"billing_type":"subscription","subscription":{"plan":"enterprise"}}}`, `{"data":{"billing_type":"wallet","wallet":{"currency":"EUR"}}}`} {
		t.Run(body, func(t *testing.T) {
			var invalid atomic.Bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v3/users/me" || r.Header.Get("X-Api-Key") != "heygen-secret" {
					t.Error("invalid native account request")
				}
				w.Header().Set("Content-Type", "application/json")
				if invalid.Load() {
					_, _ = io.WriteString(w, body)
				} else {
					_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd"}}}`)
				}
			}))
			defer upstream.Close()
			router := newManagementRouter(t, proxy.Configuration{ProviderCatalog: heygenTestCatalog(t, "heygen", upstream.URL), AssetStorePath: t.TempDir()})
			server := httptest.NewServer(router)
			defer server.Close()
			owner := managementSessionCookie(t, "heygen-account-owner")
			tenant := managementDefaultTenantTestID(t, router, owner)
			secret := generateManagementTenantSecret(t, router, owner, tenant)
			connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Video account", "provider": "heygen", "fields": map[string]string{"api_key": "heygen-secret"}}, http.StatusCreated)
			accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenant+"/connections/heygen", map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
			invalid.Store(true)
			cfg, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(cfg, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			if account, err := client.GetProviderAccount(t.Context(), "heygen"); err == nil {
				t.Fatalf("invalid native account published: %+v", account)
			}
		})
	}
}
