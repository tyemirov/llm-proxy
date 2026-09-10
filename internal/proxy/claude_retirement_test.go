package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestClaudeRetirementHTTP(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"retired-route","type":"message","role":"assistant","content":[{"type":"text","text":"retired response"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameAnthropic: upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"claude-opus-4-1", "claude-opus-4-1-20250805"} {
		t.Run(model, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&provider=anthropic&model="+model+"&prompt=test", nil))
			if response.Code != http.StatusBadRequest || upstreamCalls != 0 {
				t.Fatalf("retired route status=%d upstream calls=%d", response.Code, upstreamCalls)
			}
		})
	}
	for _, path := range []string{proxy.PublicCapabilitiesPath, "/v1/models"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+TestSecret)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "claude-opus-4-1") || !strings.Contains(response.Body.String(), "claude-opus-5") {
			t.Fatalf("discovery path=%s status=%d retired_present=%t replacement_present=%t", path, response.Code, strings.Contains(response.Body.String(), "claude-opus-4-1"), strings.Contains(response.Body.String(), "claude-opus-5"))
		}
	}
}

func TestClaudeRetirementManagedSelection(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	router := newOperationalProviderKeyVerificationRouter(t, providerKeyVerificationConfiguration(upstream.URL), zap.NewNop().Sugar(), t.TempDir()+"/managed.db", TestTimeout)
	cookie := managementSessionCookie(t, "claude-retirement-selection")
	tenant := managementDefaultTenantTestID(t, router, cookie)
	for _, model := range []string{"claude-opus-4-1", "claude-opus-4-1-20250805"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, authenticatedJSONRequest(http.MethodPut, "/api/management/tenants/"+tenant+"/provider-profiles/anthropic", `{"text_model":"`+model+`","system_prompt":""}`, cookie))
		if response.Code != http.StatusBadRequest || calls != 0 {
			t.Fatalf("retired selection model=%s status=%d calls=%d body=%s", model, response.Code, calls, response.Body.String())
		}
	}
}
