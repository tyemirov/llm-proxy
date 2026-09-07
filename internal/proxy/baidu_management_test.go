package proxy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestBaiduManagementVerification(t *testing.T) {
	for _, scenario := range []struct {
		name, body string
		status     int
	}{
		{"safe", baiduResponse("verified", "stop", "0"), 200},
		{"low risk", baiduResponse("verified", "stop", "1"), 200},
		{"omitted flag", baiduResponse("verified", "stop", ""), 200},
		{"token limited", baiduResponse("verified", "length", "0"), 200},
		{"banned", baiduResponse("private Baidu content", "stop", "2"), 503},
		{"hidden", baiduResponse("private Baidu content", "stop", "3"), 503},
		{"remove", baiduResponse("private Baidu content", "stop", "4"), 503},
		{"unknown flag", baiduResponse("private Baidu content", "stop", "99"), 503},
		{"malformed flag", baiduResponse("private Baidu content", "stop", `"0"`), 503},
		{"null flag", baiduResponse("private Baidu content", "stop", "null"), 503},
		{"blocked continuation", baiduResponse("private Baidu content", "length", "2"), 503},
		{"unsupported finish", baiduResponse("private Baidu content", "tool_calls", "0"), 503},
		{"malformed body", `{"choices":`, 503},
		{"empty choice", `{"choices":[{}]}`, 503},
		{"empty content", baiduResponse("", "stop", "0"), 503},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			const key = "candidate-baidu-opaque-key"
			observedURL := ""
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+key || r.Header.Get("appid") != "" || payload["model"] != "deepseek-v4-pro" || payload["max_tokens"] != float64(16) {
					t.Errorf("invalid verification request path=%s payload=%v", r.URL.Path, payload)
				}
				if !reflect.DeepEqual(payload["messages"], []any{map[string]any{"role": "user", "content": testProviderKeyVerificationPrompt}}) {
					t.Errorf("verification messages=%v", payload["messages"])
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, scenario.body)
			}))
			defer upstream.Close()
			previous := proxy.HTTPClient
			proxy.HTTPClient = coverageHTTPDoer(func(r *http.Request) (*http.Response, error) {
				observedURL = r.URL.String()
				rewritten := r.Clone(r.Context())
				rewritten.URL.Scheme = "http"
				rewritten.URL.Host = strings.TrimPrefix(upstream.URL, "http://")
				return previous.Do(rewritten)
			})
			defer func() { proxy.HTTPClient = previous }()
			core, logs := observer.New(zap.DebugLevel)
			databasePath := t.TempDir() + "/managed.db"
			router := newOperationalProviderKeyVerificationRouter(t, proxy.Configuration{}, zap.New(core).Sugar(), databasePath, TestTimeout)
			cookie := managementSessionCookie(t, "baidu-"+strings.ReplaceAll(scenario.name, " ", "-"))
			tenantID := managementDefaultTenantTestID(t, router, cookie)
			before := requestProviderKeyVerificationProfile(t, router, cookie, tenantID)
			response := putManagementProviderKeyWithBaseURL(t, router, cookie, tenantID, "baidu", key, "https://api.baiduqianfan.ai/v1", "deepseek-v4-pro", "saved system prompt", context.Background())
			if response.Code != scenario.status || calls != 1 || observedURL != "https://api.baiduqianfan.ai/v1/chat/completions" {
				t.Fatalf("status=%d calls=%d URL=%s body=%s", response.Code, calls, observedURL, response.Body)
			}
			if strings.Contains(response.Body.String(), key) || strings.Contains(response.Body.String(), "private Baidu content") {
				t.Fatal("credential or provider content leaked")
			}
			after := requestProviderKeyVerificationProfile(t, router, cookie, tenantID)
			provider := verificationProfileProvider(t, after, "baidu")
			if scenario.status == 200 {
				if !provider.Configured || provider.MaskedKey == "" || provider.TextModel != "deepseek-v4-pro" || provider.BaseURL != "https://api.baiduqianfan.ai/v1" || after.Tenant.Defaults.Provider != "baidu" || after.Tenant.Defaults.Model != "deepseek-v4-pro" {
					t.Fatalf("saved profile=%+v provider=%+v", after.Tenant.Defaults, provider)
				}
				database := openManagedFixtureDatabase(t, databasePath)
				row, err := loadManagedProviderKeyFixture(database, tenantID, "baidu")
				if err != nil {
					t.Fatal(err)
				}
				if row.EncryptedAPIKey == "" || strings.Contains(row.EncryptedAPIKey, key) {
					t.Fatal("provider key was not encrypted")
				}
			} else if !reflect.DeepEqual(before, after) || provider.Configured {
				t.Fatal("rejected verification changed tenant state")
			}
			if usage := requestManagementUsage(t, router, cookie, "all"); usage.Totals.Requests != 0 {
				t.Fatal("verification counted as generation")
			}
			assertProviderKeyVerificationLogsAreSafe(t, logs, key, "private Baidu content", scenario.body)
		})
	}
}

func TestBaiduUnconfiguredTenant(t *testing.T) {
	tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	delete(tenant.ProviderKeys, "baidu")
	router, err := buildRouterWithManagedTenant(t, proxy.Configuration{}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/?key="+TestSecret+"&provider=baidu&prompt=test", nil))
	if response.Code != 409 || strings.TrimSpace(response.Body.String()) != "provider_not_configured" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}
