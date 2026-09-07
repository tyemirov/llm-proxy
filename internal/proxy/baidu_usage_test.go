package proxy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestBaiduResponsePolicyPreservesManagedUsage(t *testing.T) {
	for _, scenario := range []struct {
		name, body string
		hasUsage   bool
	}{
		{"blocked flag", baiduResponse("private", "stop", "2"), true},
		{"content filter", baiduResponse("private", "content_filter", "0"), true},
		{"blocked length", baiduResponse("private", "length", "2"), true},
		{"second blocked choice", `{"choices":[{"message":{"content":"private"},"finish_reason":"stop","flag":0},{"message":{"content":"private"},"finish_reason":"stop","flag":2}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`, true},
		{"invalid usage", strings.Replace(baiduResponse("private", "stop", "2"), `"prompt_tokens":2`, `"prompt_tokens":-1`, 1), false},
		{"absent usage", `{"choices":[{"message":{"content":"private"},"finish_reason":"stop","flag":2}]}`, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload struct {
					MaxTokens int `json:"max_tokens"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if payload.MaxTokens == 16 {
					io.WriteString(w, baiduResponse("verified", "stop", "0"))
					return
				}
				io.WriteString(w, scenario.body)
			}))
			defer upstream.Close()
			router := newOperationalProviderKeyVerificationRouter(t, proxy.Configuration{Endpoints: providerEndpoints(upstream.URL, "baidu")}, zap.NewNop().Sugar(), t.TempDir()+"/usage.db", TestTimeout)
			owner := managementSessionCookie(t, "baidu-usage-owner")
			tenantID := managementDefaultTenantTestID(t, router, owner)
			saved := putManagementProviderKeyWithBaseURL(t, router, owner, tenantID, "baidu", "sk-baidu", "https://api.baiduqianfan.ai/v1", "ernie-5.0", "", context.Background())
			if saved.Code != http.StatusOK {
				t.Fatalf("save Baidu connection status=%d body=%s", saved.Code, saved.Body)
			}
			key := generateManagementTenantSecret(t, router, owner, tenantID)
			server := httptest.NewServer(router)
			defer server.Close()
			response, err := server.Client().Post(server.URL+"/v2?key="+key+"&provider=baidu", "application/json", strings.NewReader(`{"messages":[{"role":"user","content":"test"}]}`))
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusBadGateway || strings.Contains(string(body), "private") {
				t.Fatalf("policy failure status=%d body=%s", response.StatusCode, body)
			}
			usage := waitForManagementValue(t, func() managementUsageTestResponse {
				return requestManagementUsage(t, router, owner, "all")
			}, func(value managementUsageTestResponse) bool { return value.Totals.Requests == 1 })
			requestTokens, responseTokens, totalTokens := 0, 0, 0
			if scenario.hasUsage {
				requestTokens, responseTokens, totalTokens = 2, 3, 5
			}
			if usage.Totals.Requests != 1 || usage.Totals.FailedRequests != 1 || usage.Totals.SuccessfulRequests != 0 ||
				usage.Totals.RequestTokens != requestTokens || usage.Totals.ResponseTokens != responseTokens || usage.Totals.TotalTokens != totalTokens {
				t.Fatalf("failed request usage=%+v want request=%d response=%d total=%d", usage.Totals, requestTokens, responseTokens, totalTokens)
			}
		})
	}
}
