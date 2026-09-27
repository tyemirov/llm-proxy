package proxy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gopkg.in/yaml.v3"
)

const elevenModelsFixture = `[{"model_id":"eleven_v3","name":"Eleven v3","can_do_text_to_speech":true,"can_do_voice_conversion":false,"maximum_text_length_per_request":5000,"max_characters_request_free_user":2500,"max_characters_request_subscribed_user":5000}]`
const elevenQuotaFixture = `{"tier":"creator","status":"active","character_count":100,"character_limit":10000,"max_credit_limit_extension":"unlimited","can_extend_character_limit":true,"has_open_invoices":false,"currency":"usd","next_character_count_reset_unix":1790000000,"current_overage":{"amount":"1.25","currency":"usd"}}`

func elevenResourceCatalog(t *testing.T, identifier, origin string) *proxy.ProviderCatalog {
	t.Helper()
	schema := testfixtures.ProviderCatalog(t).Schema()
	found := 0
	for index := range schema.Providers {
		provider := &schema.Providers[index]
		if provider.ID != "elevenlabs" {
			continue
		}
		found++
		provider.ID = identifier
		for i := range provider.Fields {
			provider.Fields[i].ID = "resource_token"
		}
		for i := range provider.Transports {
			provider.Transports[i].Endpoint.DefaultBaseURL = origin
			if provider.Transports[i].Components.RequestCodec.ID == proxy.CatalogProtocolElevenLabsVoices {
				provider.Transports[i].ArtifactOrigins = []string{origin}
			}
			provider.Transports[i].Components.Authentication.Field = "resource_token"
		}
	}
	if found != 1 {
		t.Fatalf("catalog must contain one ElevenLabs provider, found %d", found)
	}
	document, err := yaml.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := proxy.ParseProviderCatalog(document)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestElevenLabsAccountResourcesShareOneProviderConnection(t *testing.T) {
	for _, identifier := range []string{"elevenlabs", "speech-resource-fixture"} {
		t.Run(identifier, func(t *testing.T) {
			var reads atomic.Int32
			var token atomic.Value
			token.Store("account-resource-secret")
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("xi-api-key") != token.Load().(string) {
					t.Errorf("invalid resource request method or credential")
					w.WriteHeader(401)
					return
				}
				reads.Add(1)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/models":
					_, _ = io.WriteString(w, elevenModelsFixture)
				case "/v1/user/subscription":
					_, _ = io.WriteString(w, elevenQuotaFixture)
				default:
					t.Errorf("unexpected resource path %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			catalog := elevenResourceCatalog(t, identifier, upstream.URL)
			router := newManagementRouter(t, proxy.Configuration{ProviderCatalog: catalog})
			server := httptest.NewServer(router)
			defer server.Close()
			owner := managementSessionCookie(t, "eleven-resource-owner")
			tenantID := managementDefaultTenantTestID(t, router, owner)
			secret := generateManagementTenantSecret(t, router, owner, tenantID)
			contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
			if err != nil {
				t.Fatal(err)
			}
			read := func(kind, key string, status int) map[string]any {
				t.Helper()
				request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/model/v1/provider-resources/"+identifier+"/"+kind, nil)
				request.Header.Set("Authorization", "Bearer "+key)
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil || response.StatusCode != status {
					t.Fatalf("resource status=%d body=%s error=%v", response.StatusCode, body, err)
				}
				if err := contract.ValidateResponse("/model/v1/provider-resources/{provider}/{kind}", http.MethodGet, response.StatusCode, response.Header, body); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(body), token.Load().(string)) || strings.Contains(string(body), upstream.URL) {
					t.Fatal("resource exposed credentials or endpoint")
				}
				var value map[string]any
				if err := json.Unmarshal(body, &value); err != nil {
					t.Fatal(err)
				}
				return value
			}
			read("metadata", secret, 404)
			if reads.Load() != 0 {
				t.Fatal("unassigned resource reached provider")
			}
			connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Eleven account", "provider": identifier, "fields": map[string]string{"resource_token": token.Load().(string)}}, http.StatusCreated)
			assignment := "/tenants/" + tenantID + "/connections/" + identifier
			accountConnectionExchange(t, router, owner, http.MethodPut, assignment, map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
			metadata := read("metadata", secret, 200)
			models := metadata["models"].([]any)
			if len(models) != 1 || models[0].(map[string]any)["model_id"] != "eleven_v3" {
				t.Fatalf("models=%v", models)
			}
			quota := read("quotas", secret, 200)["subscription"].(map[string]any)
			if quota["character_count"] != float64(100) || quota["credit_extension"].(map[string]any)["unlimited"] != true {
				t.Fatalf("quota=%v", quota)
			}
			foreign := managementSessionCookie(t, "foreign-eleven-owner")
			foreignID := managementDefaultTenantTestID(t, router, foreign)
			foreignKey := generateManagementTenantSecret(t, router, foreign, foreignID)
			before := reads.Load()
			read("quotas", foreignKey, 404)
			read("voices", secret, 404)
			if reads.Load() != before {
				t.Fatal("unavailable resource reached provider")
			}
			configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(configuration, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			if metadata, err := client.GetProviderMetadata(t.Context(), identifier); err != nil || len(metadata.Models) != 1 {
				t.Fatalf("metadata=%+v error=%v", metadata, err)
			}
			if quota, err := client.GetProviderQuotas(t.Context(), identifier); err != nil || !quota.Subscription.CreditExtension.Unlimited {
				t.Fatalf("quota=%+v error=%v", quota, err)
			}
			current := accountConnectionExchange(t, router, owner, http.MethodGet, "/connections/"+connection["id"].(string), nil, http.StatusOK)
			token.Store("replacement-resource-secret")
			accountConnectionExchange(t, router, owner, http.MethodPut, "/connections/"+connection["id"].(string), map[string]any{"name": "Eleven account", "provider": identifier, "version": current["version"], "fields": map[string]string{"resource_token": token.Load().(string)}}, http.StatusOK)
			read("metadata", secret, 200)
			accountConnectionExchange(t, router, owner, http.MethodDelete, assignment, nil, http.StatusNoContent)
			before = reads.Load()
			read("metadata", secret, 404)
			if reads.Load() != before {
				t.Fatal("detached resource reached provider")
			}
		})
	}
}

func TestElevenLabsAccountResourcesRejectInvalidProviderResponses(t *testing.T) {
	type result struct {
		kind, body, mode string
		status           int
	}
	var selected atomic.Value
	selected.Store(result{body: elevenQuotaFixture, status: 200})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := selected.Load().(result)
		if value.mode == "disconnect" {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = connection.Close()
			return
		}
		if value.mode == "truncated" {
			w.Header().Set("Content-Length", "999")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(value.status)
		_, _ = io.WriteString(w, value.body)
	}))
	defer upstream.Close()
	router := newManagementRouter(t, proxy.Configuration{ProviderCatalog: elevenResourceCatalog(t, "elevenlabs", upstream.URL)})
	server := httptest.NewServer(router)
	defer server.Close()
	owner := managementSessionCookie(t, "eleven-negative-owner")
	tenantID := managementDefaultTenantTestID(t, router, owner)
	key := generateManagementTenantSecret(t, router, owner, tenantID)
	connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Account resources", "provider": "elevenlabs", "fields": map[string]string{"resource_token": "resource-secret"}}, http.StatusCreated)
	accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenantID+"/connections/elevenlabs", map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
	cases := []result{
		{kind: "metadata", body: "null"}, {kind: "metadata", body: "{}"}, {kind: "metadata", body: `[{"model_id":"","name":"Missing"}]`},
		{kind: "metadata", body: `[{"model_id":"m","name":""}]`}, {kind: "metadata", body: `[{"model_id":"m","name":"Model","maximum_text_length_per_request":-1}]`},
		{kind: "metadata", body: `[{"model_id":"m","name":"Model"},{"model_id":"m","name":"Model"}]`},
		{kind: "quotas", body: "{}"}, {kind: "quotas", body: "{"},
		{kind: "quotas", body: strings.Replace(elevenQuotaFixture, `"unlimited"`, `-1`, 1)},
		{kind: "quotas", body: strings.Replace(elevenQuotaFixture, `"unlimited"`, `null`, 1)},
		{kind: "quotas", body: strings.Replace(elevenQuotaFixture, `"unlimited"`, `"1000"`, 1)},
		{kind: "quotas", body: strings.Replace(elevenQuotaFixture, `"unlimited"`, `1.5`, 1)},
		{kind: "quotas", body: strings.Replace(elevenQuotaFixture, `"1.25"`, `"not money"`, 1)},
		{kind: "quotas", body: strings.Replace(elevenQuotaFixture, `1790000000`, `-1`, 1)},
		{kind: "metadata", body: strings.Repeat(" ", (1<<20)+1)},
		{kind: "metadata", body: `{"private":"credential-secret"}`, status: 401},
		{kind: "metadata", body: `[]`, mode: "truncated"}, {kind: "metadata", mode: "disconnect"},
	}
	for index, value := range cases {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			if value.status == 0 {
				value.status = 200
			}
			selected.Store(value)
			request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/model/v1/provider-resources/elevenlabs/"+value.kind, nil)
			request.Header.Set("Authorization", "Bearer "+key)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != 502 || string(body) != `{"error":{"code":"provider_resource_unavailable"}}` {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
		})
	}
	selected.Store(result{kind: "quotas", body: strings.Replace(elevenQuotaFixture, `"unlimited"`, `0`, 1), status: 200})
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: key})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	quota, err := client.GetProviderQuotas(t.Context(), "elevenlabs")
	if err != nil || quota.Subscription.CreditExtension.Unlimited || quota.Subscription.CreditExtension.Value == nil || *quota.Subscription.CreditExtension.Value != 0 {
		t.Fatalf("quota=%+v error=%v", quota, err)
	}
}

func TestElevenLabsAccountResourcesShareAccountAdmission(t *testing.T) {
	var armed atomic.Bool
	started := make(chan string, 3)
	release := make(chan struct{})
	var once sync.Once
	releaseAll := func() { once.Do(func() { close(release) }) }
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if armed.Load() {
			started <- r.Header.Get("xi-api-key") + r.URL.Path
			if r.Header.Get("xi-api-key") == "shared-resource" {
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
			}
		}
		if r.URL.Path == "/v1/models" {
			_, _ = io.WriteString(w, elevenModelsFixture)
		} else {
			_, _ = io.WriteString(w, elevenQuotaFixture)
		}
	}))
	defer upstream.Close()
	defer releaseAll()
	capacity := testfixtures.UpstreamCapacity(2, 8)
	capacity.Account.Active = 1
	core, logs := observer.New(zap.InfoLevel)
	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{ProviderCatalog: elevenResourceCatalog(t, "elevenlabs", upstream.URL), UpstreamCapacity: capacity}, filepath.Join(t.TempDir(), "management.sqlite"))
	router, err := buildRouterWithCatalogs(t, configuration, zap.New(core).Sugar())
	if err != nil {
		t.Fatal(err)
	}
	owner := managementSessionCookie(t, "resource-admission-owner")
	tenants := []string{managementDefaultTenantTestID(t, router, owner), createManagementTenant(t, router, owner, "Shared").Tenant.ID, createManagementTenant(t, router, owner, "Independent").Tenant.ID}
	connections := map[string]string{}
	for _, name := range []string{"shared-resource", "independent-resource"} {
		result := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": name, "provider": "elevenlabs", "fields": map[string]string{"resource_token": name}}, http.StatusCreated)
		connections[name] = result["id"].(string)
	}
	keys := make([]string, 3)
	for i, tenant := range tenants {
		name := "shared-resource"
		if i == 2 {
			name = "independent-resource"
		}
		accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenant+"/connections/elevenlabs", map[string]string{"kind": "account_connection", "resource_id": connections[name]}, http.StatusOK)
		keys[i] = generateManagementTenantSecret(t, router, owner, tenant)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	defer releaseAll()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	results := make(chan error, 3)
	send := func(index int, kind string) {
		go func() {
			request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+llmproxycontract.ProviderResourcesPath+"/elevenlabs/"+kind, nil)
			request.Header.Set("Authorization", "Bearer "+keys[index])
			response, err := server.Client().Do(request)
			if err == nil {
				_, err = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				if response.StatusCode != 200 {
					err = fmt.Errorf("resource status=%d", response.StatusCode)
				}
			}
			results <- err
		}()
	}
	receive := func(want string) {
		t.Helper()
		select {
		case got := <-started:
			if got != want {
				t.Fatalf("native start=%s want=%s", got, want)
			}
		case <-ctx.Done():
			t.Fatal("resource did not start")
		}
	}
	logs.TakeAll()
	armed.Store(true)
	send(0, "metadata")
	receive("shared-resource/v1/models")
	send(1, "quotas")
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for logs.FilterMessage("upstream HTTP admission").FilterField(zap.String("decision", "capacity_wait")).Len() == 0 {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("shared resource did not queue")
		}
	}
	send(2, "quotas")
	receive("independent-resource/v1/user/subscription")
	if err := <-results; err != nil {
		t.Fatal(err)
	}
	releaseAll()
	receive("shared-resource/v1/user/subscription")
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}
