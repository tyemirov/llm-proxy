package proxy_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestHostedAuthorityDisabledAcrossPublicInterfaces(t *testing.T) {
	var dispatches atomic.Int64
	catalog := testfixtures.ProviderCatalog(t)
	fixture := newMCPFixtureWithConfig(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		dispatches.Add(1)
		writer.WriteHeader(http.StatusBadGateway)
	}), proxy.Configuration{ProviderCatalog: catalog, AssetStorePath: t.TempDir()})
	server := fixture.server
	operator := managementSessionCookieWithEmail(t, "authority-operator", testManagementAdminEmail)
	owner := managementSessionCookie(t, "authority-owner")
	tenant := requestManagementAccount(t, fixture.router, owner).Tenants[0].ID
	account := hostedResourceID(t, requireHostedHTTP(t, server, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "billing", http.StatusCreated))
	platform := hostedResourceID(t, requireHostedHTTP(t, server, operator, http.MethodPost, "/platform-connections", `{"name":"Hosted","provider":"openai","fields":{"api_key":"sk-user-openai"}}`, "platform", http.StatusCreated))
	offerings := []map[string]any{}
	for _, offering := range catalog.ModelCatalog().Offerings {
		if offering.Provider == "openai" {
			offerings = append(offerings, map[string]any{"model": offering.Model, "operations": offering.Operations})
		}
	}
	intent, err := json.Marshal(map[string]any{"billing_account_id": account, "tenant_id": tenant, "platform_connection_id": platform, "catalog_revision": catalog.ModelCatalog().Revision, "offerings": offerings, "reason": "Cross-interface authority"})
	if err != nil {
		t.Fatal(err)
	}
	grant := hostedResourceID(t, requireHostedHTTP(t, server, operator, http.MethodPost, "/hosted-access-grants", string(intent), "grant", http.StatusCreated))
	requireHostedHTTP(t, server, owner, http.MethodPut, "/tenants/"+tenant+"/connections/openai", fmt.Sprintf(`{"kind":"hosted_access_grant","resource_id":%q}`, grant), "", http.StatusOK)
	secret := generateManagementTenantSecret(t, fixture.router, owner, tenant)
	client := fixture.client(t, "authority-owner")
	for index, state := range []string{"active", "suspended", "revoked"} {
		if index != 0 {
			requireHostedHTTP(t, server, operator, http.MethodPatch, "/hosted-access-grants/"+grant, fmt.Sprintf(`{"revision":%d,"state":%q,"reason":"Authority transition"}`, index, state), "", http.StatusOK)
		}
		t.Run(state, func(t *testing.T) {
			for _, scenario := range []struct {
				path, body string
				status     int
			}{
				{"/?key=" + secret + "&provider=openai&model=gpt-6-astra", `{"prompt":"Hosted request"}`, http.StatusForbidden},
				{"/v2?key=" + secret + "&provider=openai&model=gpt-6-astra", `{"messages":[{"role":"user","content":"Hosted request"}]}`, http.StatusForbidden},
				{"/v1/chat/completions", `{"model":"openai/gpt-6-astra","messages":[{"role":"user","content":"Hosted request"}]}`, http.StatusForbidden},
				{"/v1/responses", `{"model":"openai/gpt-6-astra","input":"Hosted request"}`, http.StatusForbidden},
				{llmproxycontract.MediaOperationsPath, `{"capability":"image.generate","provider":"openai","model":"gpt-image-2","input":{"prompt":"Hosted image"},"controls":{}}`, http.StatusUnprocessableEntity},
			} {
				request, err := http.NewRequest(http.MethodPost, server.URL+scenario.path, strings.NewReader(scenario.body))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				if strings.HasPrefix(scenario.path, "/v1/") || scenario.path == llmproxycontract.MediaOperationsPath {
					request.Header.Set("Authorization", "Bearer "+secret)
				}
				request.Header.Set("Idempotency-Key", "authority-"+state)
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				payload, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != scenario.status {
					t.Fatalf("%s: status=%d want=%d body=%s", scenario.path, response.StatusCode, scenario.status, payload)
				}
			}
			for _, protocol := range []string{"native", "client"} {
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				field, path := "audio", "/dictate?key="+secret+"&provider=openai&model=gpt-transcribe"
				status := http.StatusForbidden
				if protocol == "client" {
					field, path = "file", "/v1/audio/transcriptions"
					if err := writer.WriteField("model", "openai/gpt-transcribe"); err != nil {
						t.Fatal(err)
					}
				}
				file, err := writer.CreateFormFile(field, "recording.wav")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := file.Write([]byte("controlled audio")); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				request, err := http.NewRequest(http.MethodPost, server.URL+path, &body)
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", writer.FormDataContentType())
				request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "dictation-authority-"+state)
				if protocol == "client" {
					request.Header.Set("Authorization", "Bearer "+secret)
				}
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				payload, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != status {
					t.Fatalf("%s transcription status=%d body=%s", protocol, response.StatusCode, payload)
				}
			}
			for _, tool := range []struct {
				name      string
				arguments map[string]any
			}{
				{"llm_proxy.generate_text", map[string]any{"tenant_id": tenant, "provider": "openai", "model": "gpt-6-astra", "messages": []map[string]string{{"role": "user", "content": "Hosted request"}}}},
				{"llm_proxy.create_media_operation", map[string]any{"tenant_id": tenant, "idempotency_key": "mcp-authority-" + state, "capability": "image.generate", "provider": "openai", "model": "gpt-image-2", "input": map[string]any{"prompt": "Hosted image"}, "controls": map[string]any{}}},
			} {
				result := mcpCall(t, client, tool.name, tool.arguments)
				if !result.IsError {
					t.Fatalf("%s authorized unavailable hosted access", tool.name)
				}
			}
			if dispatches.Load() != 0 {
				t.Fatalf("hosted admission dispatched %d upstream requests", dispatches.Load())
			}
		})
	}
}
