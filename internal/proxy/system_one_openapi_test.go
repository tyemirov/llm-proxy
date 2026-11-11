package proxy_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

func TestSystemOneOpenAPIRealExchange(t *testing.T) {
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, systemOneResponse)
	}))
	defer upstream.Close()
	tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenant.ProviderKeys[proxy.ProviderNameTypeSafe] = "openapi-provider-key"
	router, err := buildRouterWithManagedTenant(t, proxy.Configuration{Endpoints: providerEndpoints(upstream.URL, proxy.ProviderNameTypeSafe)}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, key := range []string{TestSecret, "invalid-tenant-key"} {
		body := []byte(systemOneRequest)
		request, err := http.NewRequest(http.MethodPost, server.URL+llmproxycontract.SystemOnePath, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+key)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("TypeSafe-Danger-Zone-Images", "true")
		if err := contract.ValidateRequest(llmproxycontract.SystemOnePath, request.Method, request, body); err != nil {
			t.Fatal(err)
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		result, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := contract.ValidateResponse(llmproxycontract.SystemOnePath, request.Method, response.StatusCode, response.Header, result); err != nil {
			t.Fatal(err)
		}
		if key == TestSecret && response.StatusCode != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.StatusCode, result)
		}
		if key != TestSecret && response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", response.StatusCode, result)
		}
	}
}
