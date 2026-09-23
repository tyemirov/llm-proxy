package proxy_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func TestHostedGrantProviderServicesUseExplicitOperationsWithoutModels(t *testing.T) {
	for _, provider := range []string{"elevenlabs", "alignment-fixture"} {
		t.Run(provider, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet || request.URL.Path != "/v1/user/subscription" {
					t.Errorf("unexpected verification: %s %s", request.Method, request.URL.Path)
				}
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, elevenQuotaFixture)
			}))
			t.Cleanup(upstream.Close)
			catalog := elevenResourceCatalog(t, provider, upstream.URL)
			router := newManagementRouter(t, proxy.Configuration{ProviderCatalog: catalog})
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			operator := managementSessionCookieWithEmail(t, "service-operator", testManagementAdminEmail)
			owner := managementSessionCookie(t, "service-owner")
			tenant := requestManagementAccount(t, router, owner).Tenants[0].ID
			account := hostedResourceID(t, requireHostedHTTP(t, server, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "billing", http.StatusCreated))
			platformIntent := fmt.Sprintf(`{"name":"Service account","provider":%q,"fields":{"resource_token":"service-fixture-secret"}}`, provider)
			platform := hostedResourceID(t, requireHostedHTTP(t, server, operator, http.MethodPost, "/platform-connections", platformIntent, "platform", http.StatusCreated))
			base := fmt.Sprintf(`{"billing_account_id":%q,"tenant_id":%q,"platform_connection_id":%q,"catalog_revision":%q,"reason":"Service access","offerings":`, account, tenant, platform, catalog.ModelCatalog().Revision)
			intent := base + `[{"operations":["pronunciation_dictionary_creation","audio_alignment"]}]}`
			created := requireHostedHTTP(t, server, operator, http.MethodPost, "/hosted-access-grants", intent, "services", http.StatusCreated)
			var grant map[string]any
			if err := json.Unmarshal(created.body, &grant); err != nil {
				t.Fatal(err)
			}
			scope := grant["offerings"].([]any)[0].(map[string]any)
			if _, present := scope["model"]; present {
				t.Fatalf("service grant invented model: %s", created.body)
			}
			replayed := requireHostedHTTP(t, server, operator, http.MethodPost, "/hosted-access-grants", base+`[{"operations":["audio_alignment","pronunciation_dictionary_creation"]}]}`, "services", http.StatusCreated)
			if string(replayed.body) != string(created.body) {
				t.Fatal("service grant replay changed")
			}
			requireHostedHTTP(t, server, owner, http.MethodGet, "/hosted-access-grants/"+grant["id"].(string), "", "", http.StatusOK)
			for index, scope := range []string{
				`{"model":"","operations":["audio_alignment"]}`, `{"model":null,"operations":["audio_alignment"]}`,
				`{"model":" ","operations":["audio_alignment"]}`, `{"model":3,"operations":["audio_alignment"]}`,
				`{"operations":["text"]}`, `{"model":"eleven_multilingual_v2","operations":["audio_alignment"]}`,
				`{"operations":["audio_alignment","audio_alignment"]}`, `{"operations":["not-a-service"]}`, `{"operations":[]}`,
			} {
				requireHostedHTTP(t, server, operator, http.MethodPost, "/hosted-access-grants", base+"["+scope+"]}", fmt.Sprintf("invalid-%d", index), http.StatusBadRequest)
			}
			duplicate := strings.Replace(intent, `[{"operations":`, `[{"operations":["audio_alignment"]},{"operations":`, 1)
			requireHostedHTTP(t, server, operator, http.MethodPost, "/hosted-access-grants", duplicate, "duplicate", http.StatusBadRequest)
		})
	}
}
