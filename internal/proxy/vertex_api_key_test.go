package proxy_test

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func TestGeminiCurrentModelsVertexAPIKeyConnection(t *testing.T) {
	var calls atomic.Int32
	var expectedKey atomic.Value
	expectedKey.Store("vertex-customer-key")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if (r.URL.Path != "/publishers/google/models/gemini-3.8-flash:generateContent" && r.URL.Path != "/publishers/google/models/gemini-3.5-flash:generateContent") || r.Method != http.MethodPost {
			t.Errorf("Vertex method=%s path=%s", r.Method, r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") == "invalid-replacement" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Header.Get("x-goog-api-key") != expectedKey.Load().(string) || r.Header.Get("Authorization") != "" || r.Header.Get("x-goog-user-project") != "" {
			t.Error("Vertex API-key authorization contract")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, vertexSuccess)
	}))
	defer upstream.Close()
	config := proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t), Endpoints: providerEndpointOverrides(map[string]string{"vertex": upstream.URL}, nil)}
	router := newOperationalProviderKeyVerificationRouter(t, config, zap.NewNop().Sugar(), t.TempDir()+"/managed.db", TestTimeout)
	owner := managementSessionCookie(t, "vertex-api-key-owner")
	tenant := managementDefaultTenantTestID(t, router, owner)
	server := httptest.NewServer(router)
	defer server.Close()
	send := func(method, path, body string, cookie *http.Cookie) (int, string) {
		t.Helper()
		request, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		if method == http.MethodPost && path == "/api/management/connections" {
			request.Header.Set("Idempotency-Key", rand.Text())
		}
		request.AddCookie(cookie)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, string(data)
	}
	payload := `{"name":"Vertex","provider":"vertex","fields":{"api_key":"vertex-customer-key"}}`
	status, body := send(http.MethodPost, "/api/management/connections", payload, owner)
	if status != http.StatusCreated || calls.Load() != 1 {
		t.Fatalf("API-key connection status=%d calls=%d body=%s", status, calls.Load(), body)
	}
	if strings.Contains(body, "vertex-customer-key") {
		t.Fatal("provider key exposed in profile")
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	path := "/api/management/connections/" + created.ID
	assignmentPath := "/api/management/tenants/" + tenant + "/connections/vertex"
	status, body = send(http.MethodPut, assignmentPath, `{"connection_id":"`+created.ID+`"}`, owner)
	if status != http.StatusOK {
		t.Fatalf("assign connection status=%d body=%s", status, body)
	}
	update := func(key string) (int, string) {
		t.Helper()
		status, body := send(http.MethodGet, path, "", owner)
		if status != http.StatusOK {
			t.Fatalf("read connection status=%d body=%s", status, body)
		}
		var current map[string]any
		if err := json.Unmarshal([]byte(body), &current); err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(map[string]any{"name": "Vertex", "provider": "vertex", "version": current["version"], "fields": map[string]string{"api_key": key}})
		if err != nil {
			t.Fatal(err)
		}
		return send(http.MethodPut, path, string(payload), owner)
	}

	other := managementSessionCookie(t, "vertex-api-key-other")
	otherTenant := managementDefaultTenantTestID(t, router, other)
	profile := requestProviderKeyVerificationProfile(t, router, other, otherTenant)
	if verificationProfileProvider(t, profile, "vertex").Configured {
		t.Fatal("connection crossed tenants")
	}
	status, _ = send(http.MethodDelete, path, "", other)
	if status != http.StatusNoContent || calls.Load() != 1 {
		t.Fatalf("cross-tenant disconnect status=%d", status)
	}
	status, _ = send(http.MethodPut, path, `{"name":"Vertex","provider":"vertex","version":2,"fields":{"credential_profile":"old-profile"}}`, owner)
	if status != http.StatusBadRequest || calls.Load() != 1 {
		t.Fatalf("obsolete profile status=%d", status)
	}
	ownerSecret := generateManagementTenantSecret(t, router, owner, tenant)
	otherSecret := generateManagementTenantSecret(t, router, other, otherTenant)
	generate := func(secret string, cookie *http.Cookie, want int) {
		t.Helper()
		status, body := send(http.MethodPost, "/v2?provider=vertex&key="+secret, `{"model":"gemini-3.8-flash","messages":[{"role":"user","content":"test"}]}`, cookie)
		if status != want {
			t.Fatalf("Vertex generation status=%d want=%d body=%s", status, want, body)
		}
	}
	generate(ownerSecret, owner, http.StatusOK)
	status, _ = update("invalid-replacement")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid replacement status=%d", status)
	}
	generate(ownerSecret, owner, http.StatusOK)
	expectedKey.Store("vertex-second-key")
	status, body = send(http.MethodPost, "/api/management/connections", strings.Replace(payload, "vertex-customer-key", "vertex-second-key", 1), other)
	if status != http.StatusCreated {
		t.Fatalf("second connection status=%d", status)
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	status, body = send(http.MethodPut, "/api/management/tenants/"+otherTenant+"/connections/vertex", `{"connection_id":"`+created.ID+`"}`, other)
	if status != http.StatusOK {
		t.Fatalf("assign second connection status=%d body=%s", status, body)
	}

	generate(otherSecret, other, http.StatusOK)
	expectedKey.Store("vertex-replacement-key")
	status, _ = update("vertex-replacement-key")
	if status != http.StatusOK {
		t.Fatalf("valid replacement status=%d", status)
	}
	generate(ownerSecret, owner, http.StatusOK)
	expectedKey.Store("vertex-second-key")
	generate(otherSecret, other, http.StatusOK)
	status, _ = send(http.MethodDelete, assignmentPath, "", owner)
	if status != http.StatusNoContent {
		t.Fatalf("disconnect status=%d", status)
	}
	profile = requestProviderKeyVerificationProfile(t, router, owner, tenant)
	if verificationProfileProvider(t, profile, "vertex").Configured {
		t.Fatal("disconnected Vertex remains configured")
	}
	generate(ownerSecret, owner, http.StatusConflict)
	generate(otherSecret, other, http.StatusOK)
}
