package proxy_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestDashScopeResponsesIncompleteDiagnostics(t *testing.T) {
	for _, reason := range []string{"content_filter", "provider_capacity", ""} {
		t.Run(reason, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if calls == 1 {
					fmt.Fprintf(w, `{"status":"incomplete","incomplete_details":{"reason":%q},"output":[],"usage":{"input_tokens":2,"output_tokens":7,"total_tokens":9,"output_tokens_details":{"reasoning_tokens":5}}}`, reason)
					return
				}
				io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"incorrect continuation"}]}]}`)
			}))
			defer upstream.Close()
			core, observed := observer.New(zap.InfoLevel)
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"dashscope": upstream.URL}, nil)}, zap.New(core).Sugar())
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&provider=dashscope&prompt=test", nil))
			if response.Code != http.StatusBadGateway || calls != 1 {
				t.Fatalf("incomplete reason=%q status=%d provider calls=%d", reason, response.Code, calls)
			}
			events := observed.FilterMessage("proxy provider progress").All()
			if len(events) != 1 {
				t.Fatalf("progress events=%d", len(events))
			}
			fields := events[0].ContextMap()
			if fields["completion_signal"] != "failure" || fields["reasoning_tokens"] != int64(5) {
				t.Fatalf("progress diagnostics=%v", fields)
			}
			if reason == "" {
				if _, exists := fields["incomplete_reason"]; exists {
					t.Fatalf("invented reason: %v", fields)
				}
			} else if fields["incomplete_reason"] != reason {
				t.Fatalf("reason lost: %v", fields)
			}
			if strings.Contains(response.Body.String(), reason) && reason != "" {
				t.Fatalf("internal reason leaked: %s", response.Body.String())
			}
		})
	}
}

func TestDashScopeResponsesReasoningDiagnostics(t *testing.T) {
	for _, fixture := range []struct {
		name, details string
		reasoning     *int
		status        int
	}{
		{"missing", "", nil, 200},
		{"reasoning only", `,"output_tokens_details":{"reasoning_tokens":3}`, diagnosticInt(3), 200},
		{"zero", `,"output_tokens_details":{"reasoning_tokens":0}`, diagnosticInt(0), 200},
		{"positive", `,"output_tokens_details":{"reasoning_tokens":5}`, diagnosticInt(5), 200},
		{"negative", `,"output_tokens_details":{"reasoning_tokens":-1}`, nil, 502},
		{"larger than output", `,"output_tokens_details":{"reasoning_tokens":8}`, nil, 502},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				usage := `"input_tokens":2,"output_tokens":7,"total_tokens":9` + fixture.details
				if fixture.name == "reasoning only" {
					usage = strings.TrimPrefix(fixture.details, ",")
				}
				fmt.Fprintf(w, `{"status":"completed","output":[{"type":"reasoning","content":[{"type":"reasoning_text","text":"PRIVATE_REASONING"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{%s}}`, usage)
			}))
			defer upstream.Close()
			core, events := observer.New(zap.InfoLevel)
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"dashscope": upstream.URL}, nil)}, zap.New(core).Sugar())
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+TestSecret+"&provider=dashscope&prompt=test", nil))
			if response.Code != fixture.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			progress := events.FilterMessage("proxy provider progress").All()
			if len(progress) != 1 {
				t.Fatalf("progress events=%d", len(progress))
			}
			fields := progress[0].ContextMap()
			actual, present := fields["reasoning_tokens"]
			if fixture.reasoning == nil && present {
				t.Fatalf("unknown reasoning became known: %v", fields)
			}
			if fixture.reasoning != nil && (!present || actual != int64(*fixture.reasoning)) {
				t.Fatalf("reasoning=%v expected=%v", actual, *fixture.reasoning)
			}
			for _, entry := range events.All() {
				data, _ := json.Marshal(entry.ContextMap())
				if strings.Contains(string(data), "PRIVATE_REASONING") {
					t.Fatal("reasoning text leaked in logs")
				}
			}
			expectedTotal := "9"
			if fixture.name == "reasoning only" {
				expectedTotal = ""
			}
			if response.Code == 200 && response.Header().Get("X-LLM-Proxy-Total-Tokens") != expectedTotal {
				t.Fatalf("reasoning double counted: %v", response.Header())
			}
		})
	}
}

func TestDashScopeResponsesStructuredIncompleteDiagnostics(t *testing.T) {
	for _, reason := range []string{"max_output_tokens", "content_filter", "provider_capacity", "", "PRIVATE unsafe text", strings.Repeat("x", 129)} {
		t.Run(reason, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"status":"incomplete","incomplete_details":{"reason":%q},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"decision\":\"pass\"}"}]}],"usage":{"input_tokens":2,"output_tokens":7,"total_tokens":9,"output_tokens_details":{"reasoning_tokens":5}}}`, reason)
			}))
			defer upstream.Close()
			core, events := observer.New(zap.InfoLevel)
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"dashscope": upstream.URL}, nil), AssetStorePath: t.TempDir(), AssetRetentionSeconds: 3600}, zap.New(core).Sugar())
			if err != nil {
				t.Fatal(err)
			}
			result := performStructuredRequestForProvider(t, router, dashScopeStructuredBody(t, dashScopeFlashStructuredModel, nil), "diagnostics:incomplete", "dashscope")
			if result.Code != http.StatusBadGateway || calls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", result.Code, calls, result.Body.String())
			}
			progress := events.FilterMessage("proxy provider progress").All()
			if len(progress) != 1 {
				t.Fatalf("progress events=%d", len(progress))
			}
			fields := progress[0].ContextMap()
			expected := "failure"
			if reason == "max_output_tokens" {
				expected = "output_limit"
			}
			if fields["completion_signal"] != expected {
				t.Fatalf("signal=%v", fields)
			}
			valid := reason != "" && !strings.Contains(reason, " ") && len(reason) <= 128
			if valid && fields["incomplete_reason"] != reason {
				t.Fatalf("reason lost during structured failure: %v", fields)
			}
			if !valid {
				if _, present := fields["incomplete_reason"]; present {
					t.Fatalf("unsafe/absent reason retained: %v", fields)
				}
			}
			if fields["reasoning_tokens"] != int64(5) {
				t.Fatalf("reasoning count lost: %v", fields)
			}
		})
	}
}

func TestDashScopeResponsesDiagnosticPersistence(t *testing.T) {
	for _, fixture := range []struct {
		name, status, reason string
		counts               []*int
		expected             *int
	}{
		{"positive", "completed", "", []*int{diagnosticInt(5)}, diagnosticInt(5)},
		{"zero", "completed", "", []*int{diagnosticInt(0)}, diagnosticInt(0)},
		{"absent", "completed", "", []*int{nil}, nil},
		{"filter", "incomplete", "content_filter", []*int{diagnosticInt(5)}, diagnosticInt(5)},
		{"structured filter", "incomplete", "content_filter", []*int{diagnosticInt(5)}, diagnosticInt(5)},
		{"structured limit", "incomplete", "max_output_tokens", []*int{diagnosticInt(5)}, diagnosticInt(5)},
		{"limit", "incomplete", "max_output_tokens", []*int{diagnosticInt(5), diagnosticInt(2)}, diagnosticInt(7)},
		{"multiple continuations", "incomplete", "max_output_tokens", []*int{diagnosticInt(5), diagnosticInt(2), diagnosticInt(0)}, diagnosticInt(7)},
		{"partial first", "incomplete", "max_output_tokens", []*int{diagnosticInt(5), nil}, nil},
		{"partial last", "incomplete", "max_output_tokens", []*int{nil, diagnosticInt(2)}, nil},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				index := calls
				calls++
				if index >= len(fixture.counts) {
					t.Error("unexpected continuation")
					w.WriteHeader(500)
					return
				}
				status, reason := fixture.status, fixture.reason
				if len(fixture.counts) > 1 && index == len(fixture.counts)-1 {
					status, reason = "completed", ""
				}
				details := ""
				if fixture.counts[index] != nil {
					details = fmt.Sprintf(`,"output_tokens_details":{"reasoning_tokens":%d}`, *fixture.counts[index])
				}
				fmt.Fprintf(w, `{"status":%q,"incomplete_details":{"reason":%q},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":2,"output_tokens":7,"total_tokens":9%s}}`, status, reason, details)
			}))
			defer upstream.Close()
			target, _ := url.Parse(upstream.URL)
			previous := proxy.HTTPClient
			proxy.HTTPClient = coverageHTTPDoer(func(request *http.Request) (*http.Response, error) {
				rewritten := request.Clone(request.Context())
				rewritten.URL.Scheme = target.Scheme
				rewritten.URL.Host = target.Host
				rewritten.Host = ""
				return previous.Do(rewritten)
			})
			defer func() { proxy.HTTPClient = previous }()
			configuration := proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{"dashscope": upstream.URL}, nil)}
			databasePath := filepath.Join(t.TempDir(), "diagnostics.sqlite")
			router := newTokenPresenceRouter(t, configuration, databasePath)
			cookie := managementSessionCookie(t, "diagnostics-owner")
			tenantID := requestManagementAccount(t, router, cookie).Tenants[0].ID
			saved := putManagementProviderKeyWithBaseURL(t, router, cookie, tenantID, "dashscope", "diagnostics-key", testDashScopeWorkspaceURL, proxy.ModelNameDashScopeQwenPlus, "", context.Background())
			if saved.Code != 200 {
				t.Fatalf("save status=%d body=%s", saved.Code, saved.Body.String())
			}
			secret := generateManagementTenantSecret(t, router, cookie, tenantID)
			response := httptest.NewRecorder()
			if strings.HasPrefix(fixture.name, "structured") {
				request := httptest.NewRequest(http.MethodPost, "/v2?key="+secret+"&provider=dashscope", strings.NewReader(dashScopeStructuredBody(t, dashScopeFlashStructuredModel, nil)))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Idempotency-Key", "diagnostics:incomplete")
				router.ServeHTTP(response, request)
			} else {
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+secret+"&provider=dashscope&prompt=test", nil))
			}
			expectedStatus := 200
			if fixture.status == "incomplete" && len(fixture.counts) == 1 {
				expectedStatus = 502
			}
			if response.Code != expectedStatus || calls != len(fixture.counts) {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
			}
			if err := router.Close(); err != nil {
				t.Fatal(err)
			}
			verify := func() {
				t.Helper()
				database, err := sql.Open("sqlite", databasePath)
				if err != nil {
					t.Fatal(err)
				}
				defer database.Close()
				var reasoning sql.NullInt64
				var reason sql.NullString
				var total int
				if err := database.QueryRow("SELECT reasoning_tokens,incomplete_reason,total_tokens FROM managed_usage_event_records WHERE tenant_id=?", tenantID).Scan(&reasoning, &reason, &total); err != nil {
					t.Fatal(err)
				}
				if reasoning.Valid != (fixture.expected != nil) || (fixture.expected != nil && reasoning.Int64 != int64(*fixture.expected)) {
					t.Fatalf("reasoning=%v expected=%v", reasoning, fixture.expected)
				}
				expectedReason := fixture.reason
				if len(fixture.counts) > 1 {
					expectedReason = ""
				}
				if reason.Valid != (expectedReason != "") || (reason.Valid && reason.String != expectedReason) {
					t.Fatalf("terminal reason=%v expected=%q", reason, expectedReason)
				}
				if total != 9*len(fixture.counts) {
					t.Fatalf("reasoning changed totals: %d", total)
				}
			}
			verify()
			reopened := newTokenPresenceRouter(t, configuration, databasePath)
			if err := reopened.Close(); err != nil {
				t.Fatal(err)
			}
			verify()
			if fixture.name == "positive" {
				database, err := sql.Open("sqlite", databasePath)
				if err != nil {
					t.Fatal(err)
				}
				for _, column := range []string{"reasoning_tokens", "incomplete_reason"} {
					if _, err := database.Exec("ALTER TABLE managed_usage_event_records DROP COLUMN " + column); err != nil {
						t.Fatal(err)
					}
				}
				if err := database.Close(); err != nil {
					t.Fatal(err)
				}
				for startup := 0; startup < 2; startup++ {
					migrated := newTokenPresenceRouter(t, configuration, databasePath)
					if err := migrated.Close(); err != nil {
						t.Fatal(err)
					}
					database, err := sql.Open("sqlite", databasePath)
					if err != nil {
						t.Fatal(err)
					}
					var count, reason sql.NullString
					var total int
					err = database.QueryRow("SELECT reasoning_tokens,incomplete_reason,total_tokens FROM managed_usage_event_records WHERE tenant_id=?", tenantID).Scan(&count, &reason, &total)
					database.Close()
					if err != nil {
						t.Fatal(err)
					}
					if count.Valid || reason.Valid || total != 9 {
						t.Fatalf("upgrade invented historical diagnostics: count=%v reason=%v total=%d", count, reason, total)
					}
				}
			}
		})
	}
}

func diagnosticInt(value int) *int { return &value }
