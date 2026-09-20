package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"gopkg.in/yaml.v3"
)

func TestAccountConnectionMediaOnlyCatalogVerification(t *testing.T) {
	for _, fixture := range []struct{ provider, credential, header string }{
		{"media-account", "api_key", "X-Media-Key"},
		{"second-media-account", "access_token", "X-Account-Token"},
	} {
		t.Run(fixture.provider, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.Method != http.MethodGet || request.URL.Path != "/account" || request.ContentLength > 0 {
					t.Errorf("verification generated work: %s %s", request.Method, request.URL.Path)
					writer.WriteHeader(http.StatusBadRequest)
					return
				}
				switch request.Header.Get(fixture.header) {
				case "first-secret", "rotated-secret":
					writer.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(writer, `{"subscription":"active"}`)
				default:
					writer.WriteHeader(http.StatusUnauthorized)
				}
			}))
			defer upstream.Close()
			catalog := mediaOnlyVerificationCatalog(t, fixture.provider, fixture.credential, fixture.header, upstream.URL)
			configuration := testfixtures.WithUpstreamCapacity(t, proxy.Configuration{ProviderCatalog: catalog})
			router := newManagementRouter(t, configuration)
			server := httptest.NewServer(router)
			defer server.Close()
			owner := managementSessionCookie(t, "media-connection-owner")
			foreign := managementSessionCookie(t, "media-connection-foreign")
			exchange := func(cookie *http.Cookie, method, path string, payload any, status int) map[string]any {
				t.Helper()
				body, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				request, err := http.NewRequestWithContext(t.Context(), method, server.URL+"/api/management"+path, bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				if method == http.MethodPost {
					request.Header.Set("Idempotency-Key", "media-connection-create")
				}
				request.AddCookie(cookie)
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				content, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != status {
					t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.StatusCode, status, content)
				}
				if strings.Contains(string(content), "first-secret") || strings.Contains(string(content), "rotated-secret") {
					t.Fatal("connection response exposed a credential")
				}
				var value map[string]any
				if len(content) > 0 && response.StatusCode < 400 {
					if err := json.Unmarshal(content, &value); err != nil {
						t.Fatal(err)
					}
				}
				return value
			}
			connection := exchange(owner, http.MethodPost, "/connections", map[string]any{
				"name": "Media account", "provider": fixture.provider, "fields": map[string]string{fixture.credential: "first-secret"},
			}, http.StatusCreated)
			path := "/connections/" + connection["id"].(string)
			exchange(foreign, http.MethodGet, path, nil, http.StatusNotFound)
			tenant := managementDefaultTenantTestID(t, router, owner)
			exchange(owner, http.MethodPut, "/tenants/"+tenant+"/connections/"+fixture.provider, map[string]string{"connection_id": connection["id"].(string)}, http.StatusOK)
			connection = exchange(owner, http.MethodGet, path, nil, http.StatusOK)
			update := map[string]any{"name": "Media account", "provider": fixture.provider, "version": connection["version"], "fields": map[string]string{fixture.credential: "rejected-secret"}}
			exchange(owner, http.MethodPut, path, update, http.StatusUnprocessableEntity)
			unchanged := exchange(owner, http.MethodGet, path, nil, http.StatusOK)
			if unchanged["version"] != connection["version"] {
				t.Fatal("rejected credential changed the connection")
			}
			update["fields"] = map[string]string{fixture.credential: "rotated-secret"}
			exchange(owner, http.MethodPut, path, update, http.StatusOK)
			exchange(owner, http.MethodDelete, "/tenants/"+tenant+"/connections/"+fixture.provider, nil, http.StatusNoContent)
			exchange(owner, http.MethodDelete, path, nil, http.StatusNoContent)
			if calls.Load() != 3 {
				t.Fatalf("verification calls=%d want=3", calls.Load())
			}
			for _, offering := range catalog.ModelCatalog().Offerings {
				if offering.Provider == fixture.provider && slices.Contains(offering.Operations, proxy.ModelOperationText) {
					t.Fatal("media connection requires a fake text model")
				}
			}
		})
	}
}

func mediaOnlyVerificationCatalog(t *testing.T, providerID, credential, header, upstreamURL string) *proxy.ProviderCatalog {
	t.Helper()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for _, provider := range schema.Providers {
		if provider.ID != proxy.ProviderNameXAI {
			continue
		}
		provider.ID, provider.Label, provider.APIServiceLabel = providerID, "Media account", "Media API"
		provider.Aliases = nil
		provider.Fields = slices.Clone(provider.Fields[:1])
		provider.Fields[0].ID, provider.Fields[0].Environment = credential, ""
		provider.Offerings = slices.DeleteFunc(slices.Clone(provider.Offerings), func(offering proxy.ProviderCatalogOffering) bool {
			return !slices.Contains(offering.Operations, "video_generation")
		})
		provider.Transports = slices.DeleteFunc(slices.Clone(provider.Transports), func(transport proxy.ProviderCatalogTransport) bool {
			return transport.ID != provider.Offerings[0].Transport
		})
		provider.Transports[0].Components.Authentication.Field = credential
		provider.Transports = append(provider.Transports, proxy.ProviderCatalogTransport{
			ID: "account", Endpoint: proxy.ProviderCatalogEndpoint{Protocol: "http", Method: http.MethodGet, DefaultBaseURL: upstreamURL, Path: "/account"},
			Components: proxy.ProviderCatalogTransportComponents{
				RequestCodec: proxy.ProviderCatalogCodecReference{ID: "json_resource"}, ResponseCodec: proxy.ProviderCatalogCodecReference{ID: "json_resource"},
				Authentication: proxy.ProviderCatalogAuthentication{Kind: "header", Field: credential, Header: header},
				Execution:      proxy.ProviderCatalogExecutionReference{ID: "read_only"},
			},
		})
		schema.Providers = append(schema.Providers, provider)
		break
	}
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
			provider["verification"] = map[string]any{"transport": "account"}
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
	return catalog
}

func TestProviderCatalogVerificationRejectsInvalidBindings(t *testing.T) {
	catalog := mediaOnlyVerificationCatalog(t, "media-account", "api_key", "X-Media-Key", "https://media.example")
	for _, scenario := range []struct {
		name   string
		mutate func(*proxy.ProviderCatalogSchema)
	}{
		{"previous schema", func(schema *proxy.ProviderCatalogSchema) { schema.SchemaVersion = 4 }},
		{"missing binding", func(schema *proxy.ProviderCatalogSchema) {
			schema.Providers[0].Verification = proxy.ProviderCatalogVerification{}
		}},
		{"unknown transport", func(schema *proxy.ProviderCatalogSchema) { schema.Providers[0].Verification.Transport = "absent" }},
		{"missing text model", func(schema *proxy.ProviderCatalogSchema) { schema.Providers[0].Verification.Model = "" }},
		{"unrelated model", func(schema *proxy.ProviderCatalogSchema) { schema.Providers[0].Verification.Model = "gpt-image-2" }},
		{"media submission", func(schema *proxy.ProviderCatalogSchema) { schema.Providers[0].Verification.Transport = "image" }},
		{"text GET", func(schema *proxy.ProviderCatalogSchema) {
			schema.Providers[0].Transports[0].Endpoint.Method = http.MethodGet
		}},
		{"resource model", func(schema *proxy.ProviderCatalogSchema) {
			schema.Providers[len(schema.Providers)-1].Verification.Model = "gpt-4.1"
		}},
		{"resource POST", func(schema *proxy.ProviderCatalogSchema) {
			schema.Providers[len(schema.Providers)-1].Transports[1].Endpoint.Method = http.MethodPost
		}},
		{"resource generation", func(schema *proxy.ProviderCatalogSchema) {
			schema.Providers[len(schema.Providers)-1].Transports[1].Components.Execution.ID = "synchronous_completion"
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			schema := catalog.Schema()
			scenario.mutate(&schema)
			if _, err := proxy.NewProviderCatalog(schema); err == nil {
				t.Fatal("invalid verification contract accepted")
			}
		})
	}
}

func TestAccountConnectionMediaVerificationRejectsInvalidProviderResponses(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
		want   int
	}{
		{"malformed", 200, "{", http.StatusServiceUnavailable},
		{"null", 200, "null", http.StatusServiceUnavailable},
		{"array", 200, "[]", http.StatusServiceUnavailable},
		{"oversized", 200, strings.Repeat(" ", (1<<20)+1), http.StatusServiceUnavailable},
		{"truncated", -1, "", http.StatusServiceUnavailable},
		{"connection lost", 0, "", http.StatusServiceUnavailable},
		{"rejected", 403, "private upstream error", http.StatusUnprocessableEntity},
		{"rate limited", 429, "private upstream error", http.StatusTooManyRequests},
		{"unavailable", 503, "private upstream error", http.StatusServiceUnavailable},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet {
					t.Errorf("unexpected method=%s", request.Method)
				}
				if scenario.status == 0 {
					connection, _, err := writer.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = connection.Close()
					return
				}
				if scenario.status == -1 {
					writer.Header().Set("Content-Length", "100")
					_, _ = io.WriteString(writer, "{")
					return
				}
				writer.WriteHeader(scenario.status)
				_, _ = io.WriteString(writer, scenario.body)
			}))
			defer upstream.Close()
			catalog := mediaOnlyVerificationCatalog(t, "media-account", "api_key", "X-Media-Key", upstream.URL)
			configuration := testfixtures.WithUpstreamCapacity(t, proxy.Configuration{ProviderCatalog: catalog})
			router := newManagementRouter(t, configuration)
			server := httptest.NewServer(router)
			defer server.Close()
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/management/connections", strings.NewReader(`{"name":"Media","provider":"media-account","fields":{"api_key":"test-secret"}}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "rejected-media-connection")
			owner := managementSessionCookie(t, "media-error-owner")
			request.AddCookie(owner)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != scenario.want {
				t.Fatalf("status=%d want=%d body=%s", response.StatusCode, scenario.want, body)
			}
			if strings.Contains(string(body), "private upstream") || strings.Contains(string(body), "test-secret") {
				t.Fatal("verification error exposed private data")
			}
			inventory := accountConnectionExchange(t, router, owner, http.MethodGet, "/connections", nil, http.StatusOK)
			if len(inventory["connections"].([]any)) != 0 {
				t.Fatal("failed verification created a connection")
			}
		})
	}
}

func TestAccountConnectionDisabledVerificationModelDoesNotSelectAnotherModel(t *testing.T) {
	schema := testfixtures.ProviderCatalog(t).Schema()
	schema.ModelMigrations = nil
	provider := &schema.Providers[0]
	selected := provider.Verification.Model
	for index := range provider.Offerings {
		offering := &provider.Offerings[index]
		if offering.Model == selected {
			offering.Enabled = proxy.ModelDisabled
			offering.DefaultOperations = nil
		}
	}
	for index := range provider.Offerings {
		offering := &provider.Offerings[index]
		if offering.Model != selected && slices.Contains(offering.Operations, proxy.ModelOperationText) {
			offering.DefaultOperations = []string{proxy.ModelOperationText}
			break
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	router := newManagementRouter(t, proxy.Configuration{ProviderCatalog: catalog})
	owner := managementSessionCookie(t, "disabled-verification-owner")
	request := authenticatedJSONRequest(http.MethodPost, "/api/management/connections", `{"name":"Unavailable verification","provider":"openai","fields":{"api_key":"test-secret"}}`, owner)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "provider_key_verification_unavailable" {
		t.Fatalf("disabled verification status=%d body=%s", response.Code, response.Body.String())
	}
}
