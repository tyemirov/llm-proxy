package proxy_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

type xa1dBudgetFixture struct {
	configuration       proxy.Configuration
	policy              *proxy.QualificationBudgetConfiguration
	secret, otherSecret string
	calls               *atomic.Int64
	upstream            *httptest.Server
	router              *proxy.Router
	provider, model     string
	server              *httptest.Server
}

func newXA1DBudgetFixture(t *testing.T, cap int64, handler http.HandlerFunc) *xa1dBudgetFixture {
	return newXA1DBudgetProviderFixture(t, cap, "cerebras", "qwen3.8-27b", handler)
}
func newXA1DBudgetProviderFixture(t *testing.T, cap int64, provider, model string, handler http.HandlerFunc, options ...func(*proxy.QualificationBudgetConfiguration)) *xa1dBudgetFixture {
	t.Helper()
	root := t.TempDir()
	calls := new(atomic.Int64)
	setup := new(atomic.Bool)
	setup.Store(true)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"qwen-3.8-27b"}]}`)
			return
		}
		if setup.Load() && provider == "gemini" {
			if r.Method == http.MethodDelete {
				io.WriteString(w, `{}`)
				return
			}
			writeGeminiInteractionSnapshot(t, w, "verification", "completed", "OK", nil)
			return
		}
		if setup.Load() && provider == "anthropic" {
			io.WriteString(w, `{"id":"verification","type":"message","role":"assistant","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn"}`)
			return
		}
		if setup.Load() && provider == "openai" {
			io.WriteString(w, `{"id":"verification-fixture","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}]}`)
			return
		}
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(upstream.Close)
	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: filepath.Join(root, "assets"), Endpoints: providerEndpointOverrides(map[string]string{"cerebras": upstream.URL, "openai": upstream.URL, "anthropic": upstream.URL, "gemini": upstream.URL}, nil)}, filepath.Join(root, "management.sqlite"))
	configuration, err := configurationWithCatalogs(t, configuration)
	if err != nil {
		t.Fatal(err)
	}
	router, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	cookie := managementSessionCookie(t, "budget-owner")
	tenant := managementDefaultTenantTestID(t, router, cookie)
	connection := accountConnectionExchange(t, router, cookie, http.MethodPost, "/connections", map[string]any{"name": "qualification connection", "provider": provider, "fields": map[string]string{"api_key": "local-fixture-key"}}, http.StatusCreated)
	id := connection["id"].(string)
	accountConnectionExchange(t, router, cookie, http.MethodPut, "/tenants/"+tenant+"/connections/"+provider, map[string]string{"kind": "account_connection", "resource_id": id}, http.StatusOK)
	secret := generateManagementTenantSecret(t, router, cookie, tenant)
	other := createManagementTenant(t, router, cookie, "other tenant").Tenant.ID
	accountConnectionExchange(t, router, cookie, http.MethodPut, "/tenants/"+other+"/connections/"+provider, map[string]string{"kind": "account_connection", "resource_id": id}, http.StatusOK)
	otherSecret := generateManagementTenantSecret(t, router, cookie, other)
	if err := router.Close(); err != nil {
		t.Fatal(err)
	}
	policy := &proxy.QualificationBudgetConfiguration{BudgetID: "fixture-budget", TenantID: tenant, ExpiresAt: time.Now().Add(time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339), RemainingMicroUSD: cap, ReceiptDatabasePath: filepath.Join(root, "qualification.sqlite"), MaxAttemptsPerRequest: 2, Offerings: []proxy.QualificationBudgetOffering{{Provider: provider, Model: model, MaxOutputTokens: 128}}, VerificationConnections: []proxy.QualificationBudgetConnection{{Provider: provider, Name: "qualification connection"}}}
	for _, option := range options {
		option(policy)
	}
	configuration.QualificationBudget = policy
	setup.Store(false)
	calls.Store(0)
	fixture := &xa1dBudgetFixture{configuration: configuration, policy: policy, secret: secret, otherSecret: otherSecret, calls: calls, upstream: upstream, provider: provider, model: model}
	fixture.start(t)
	t.Cleanup(func() {
		if fixture.router != nil {
			fixture.server.Close()
			_ = fixture.router.Close()
		}
	})
	return fixture
}
func (fixture *xa1dBudgetFixture) start(t *testing.T) {
	t.Helper()
	router, err := proxy.BuildRouter(fixture.configuration, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	fixture.router = router
	fixture.server = httptest.NewServer(router)
}
func (fixture *xa1dBudgetFixture) request(t *testing.T, secret, extra string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	result, err := fixture.server.Client().Get(fixture.server.URL + "/?key=" + secret + "&provider=" + fixture.provider + "&model=" + fixture.model + "&prompt=private-fixture-prompt&max_tokens=128" + extra)
	if err != nil {
		t.Error(err)
		response.Code = 599
		return response
	}
	defer result.Body.Close()
	for name, values := range result.Header {
		for _, value := range values {
			response.Header().Add(name, value)
		}
	}
	response.WriteHeader(result.StatusCode)
	_, _ = io.Copy(response.Body, result.Body)
	return response
}
func (fixture *xa1dBudgetFixture) inspect(t *testing.T) map[string]any {
	t.Helper()
	var output bytes.Buffer
	if err := proxy.OperateQualificationBudget(fixture.configuration.Management.DatabasePath, fixture.policy.BudgetID, "inspect", &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "local-fixture-key") || strings.Contains(output.String(), "private-fixture-prompt") || strings.Contains(output.String(), "private-fixture-answer") {
		t.Fatalf("sensitive receipt: %s", output.String())
	}
	var value map[string]any
	if err := json.Unmarshal(output.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func xa1dBudgetAnswer(w http.ResponseWriter, _ *http.Request) {
	io.WriteString(w, `{"choices":[{"message":{"content":"private-fixture-answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
}

func TestQualificationBudgetXA1DConcurrentCapAndTenantIsolation(t *testing.T) {
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	fixture := newXA1DBudgetFixture(t, 70000, func(w http.ResponseWriter, r *http.Request) { started <- struct{}{}; <-release; xa1dBudgetAnswer(w, r) })
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- fixture.request(t, fixture.secret, "") }()
	<-started
	rejected := fixture.request(t, fixture.secret, "")
	if rejected.Code == http.StatusOK || fixture.calls.Load() != 1 {
		t.Fatalf("cap status=%d calls=%d", rejected.Code, fixture.calls.Load())
	}
	other := make(chan *httptest.ResponseRecorder, 1)
	go func() { other <- fixture.request(t, fixture.otherSecret, "") }()
	close(release)
	if response := <-done; response.Code != http.StatusOK {
		t.Fatalf("selected status=%d body=%s", response.Code, response.Body.String())
	}
	if response := <-other; response.Code != http.StatusOK {
		t.Fatalf("other status=%d body=%s", response.Code, response.Body.String())
	}
	receipt := fixture.inspect(t)
	if receipt["remaining_micro_usd"] != float64(69993) || len(receipt["attempts"].([]any)) != 1 {
		t.Fatalf("receipt=%v", receipt)
	}
}

func TestQualificationBudgetXA1DUncertaintyRestartAndRemoval(t *testing.T) {
	fixture := newXA1DBudgetFixture(t, 1000000, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"unknown usage"},"finish_reason":"stop"}]}`)
	})
	response := fixture.request(t, fixture.secret, "")
	if response.Code == http.StatusOK || fixture.calls.Load() != 1 {
		t.Fatalf("uncertainty status=%d calls=%d", response.Code, fixture.calls.Load())
	}
	receipt := fixture.inspect(t)
	if receipt["suspended"] != true {
		t.Fatalf("receipt=%v", receipt)
	}
	fixture.server.Close()
	_ = fixture.router.Close()
	fixture.router = nil
	fixture.start(t)
	response = fixture.request(t, fixture.secret, "")
	if response.Code == http.StatusOK || fixture.calls.Load() != 1 {
		t.Fatalf("restart escaped suspension: %d %d", response.Code, fixture.calls.Load())
	}
	fixture.server.Close()
	_ = fixture.router.Close()
	fixture.router = nil
	configuration := fixture.configuration
	configuration.QualificationBudget = nil
	if router, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar()); err == nil {
		_ = router.Close()
		t.Fatal("policy removal accepted")
	}
	changed := *fixture.policy
	changed.RemainingMicroUSD++
	configuration = fixture.configuration
	configuration.QualificationBudget = &changed
	if router, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar()); err == nil {
		_ = router.Close()
		t.Fatal("cap reset accepted")
	}
	var output bytes.Buffer
	if err := proxy.OperateQualificationBudget(fixture.configuration.Management.DatabasePath, fixture.policy.BudgetID, "close", &output); err == nil {
		t.Fatal("unresolved journal closed")
	}
}

func TestQualificationBudgetXA1DSettledRestartCloseAndTombstone(t *testing.T) {
	fixture := newXA1DBudgetFixture(t, 1000000, xa1dBudgetAnswer)
	if response := fixture.request(t, fixture.secret, ""); response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	fixture.server.Close()
	_ = fixture.router.Close()
	fixture.router = nil
	fixture.start(t)
	if receipt := fixture.inspect(t); receipt["remaining_micro_usd"] != float64(999993) {
		t.Fatalf("restart receipt=%v", receipt)
	}
	var output bytes.Buffer
	if err := proxy.OperateQualificationBudget(fixture.configuration.Management.DatabasePath, fixture.policy.BudgetID, "close", &output); err != nil {
		t.Fatal(err)
	}
	if response := fixture.request(t, fixture.secret, ""); response.Code == http.StatusOK || fixture.calls.Load() != 1 {
		t.Fatalf("closed policy allowed dispatch: %d %d", response.Code, fixture.calls.Load())
	}
	fixture.server.Close()
	_ = fixture.router.Close()
	fixture.router = nil
	if router, err := proxy.BuildRouter(fixture.configuration, zap.NewNop().Sugar()); err == nil {
		_ = router.Close()
		t.Fatal("closed policy reused")
	}
	configuration := fixture.configuration
	configuration.QualificationBudget = nil
	router, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	_ = router.Close()
	if err := proxy.OperateQualificationBudget(fixture.configuration.Management.DatabasePath, fixture.policy.BudgetID, "close", &output); err != nil {
		t.Fatal(err)
	}
}

func TestQualificationBudgetXA1DOutputAndToolRejection(t *testing.T) {
	fixture := newXA1DBudgetFixture(t, 1000000, xa1dBudgetAnswer)
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+fixture.secret+"&provider=cerebras&model=qwen3.8-27b&prompt=test&max_tokens=129", nil))
	if response.Code == http.StatusOK || fixture.calls.Load() != 0 {
		t.Fatalf("wire output cap status=%d calls=%d", response.Code, fixture.calls.Load())
	}
}

func TestQualificationBudgetXA1DOpenAIPollsAndChargedRetries(t *testing.T) {
	for _, scenario := range []string{"pending", "charged-retry", "missing-usage"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newXA1DBudgetProviderFixture(t, 1000000, "openai", "gpt-6-luna", func(w http.ResponseWriter, r *http.Request) {
				if scenario == "pending" && r.Method == http.MethodPost {
					io.WriteString(w, `{"id":"response-fixture","status":"in_progress"}`)
					return
				}
				if scenario == "missing-usage" {
					io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"unknown"}]}]}`)
					return
				}
				if scenario == "charged-retry" {
					w.WriteHeader(http.StatusInternalServerError)
				}
				io.WriteString(w, `{"id":"response-fixture","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"private-fixture-answer"}]}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`)
			})
			started := time.Now()
			response := fixture.request(t, fixture.secret, "")
			receipt := fixture.inspect(t)
			attempts := receipt["attempts"].([]any)
			for _, raw := range attempts {
				attempt := raw.(map[string]any)
				if attempt["RequestID"] != response.Header().Get("X-LLM-Proxy-Request-ID") || attempt["RequestID"] == "" {
					t.Fatalf("retry/poll originating request attribution missing: %v header=%s", attempt, response.Header().Get("X-LLM-Proxy-Request-ID"))
				}
			}
			switch scenario {
			case "pending":
				if response.Code != http.StatusOK || fixture.calls.Load() != 2 || len(attempts) != 1 || receipt["remaining_micro_usd"] != float64(999997) || receipt["suspended"] != false {
					t.Fatalf("pending status=%d calls=%d receipt=%v body=%s", response.Code, fixture.calls.Load(), receipt, response.Body.String())
				}
			case "charged-retry":
				if response.Code == http.StatusOK || fixture.calls.Load() != 2 || len(attempts) != 2 || receipt["remaining_micro_usd"] != float64(999994) || receipt["suspended"] != false {
					t.Fatalf("retry status=%d calls=%d receipt=%v body=%s", response.Code, fixture.calls.Load(), receipt, response.Body.String())
				}
			case "missing-usage":
				if response.Code == http.StatusOK || fixture.calls.Load() != 1 || receipt["suspended"] != true || time.Since(started) > 2*time.Second {
					t.Fatalf("terminal error retried: %d calls=%d receipt=%v", response.Code, fixture.calls.Load(), receipt)
				}
			}
		})
	}
}

func TestQualificationBudgetXA1DContinuationAttempts(t *testing.T) {
	var attempts atomic.Int64
	fixture := newXA1DBudgetProviderFixture(t, 1000000, "cerebras", "qwen3.8-27b", func(w http.ResponseWriter, r *http.Request) {
		finish := "length"
		if attempts.Add(1) > 1 {
			finish = "stop"
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"suffix "},"finish_reason":"`+finish+`"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
	}, func(policy *proxy.QualificationBudgetConfiguration) { policy.Offerings[0].MaxOutputTokens = 32768 })
	response := fixture.request(t, fixture.secret, "")
	receipt := fixture.inspect(t)
	for _, raw := range receipt["attempts"].([]any) {
		attempt := raw.(map[string]any)
		if attempt["RequestID"] != response.Header().Get("X-LLM-Proxy-Request-ID") || attempt["RequestID"] == "" {
			t.Fatalf("continuation origin attribution: %v", attempt)
		}
	}
	if response.Code != http.StatusOK || len(receipt["attempts"].([]any)) != 2 || receipt["remaining_micro_usd"] != float64(999986) {
		t.Fatalf("continuation status=%d receipt=%v body=%s", response.Code, receipt, response.Body.String())
	}
}

func TestQualificationBudgetXA1DExpiredMissingAndCorruptJournal(t *testing.T) {
	for _, scenario := range []string{"expired", "missing", "corrupt", "wrong-owner", "unpriced"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "expired" {
				fixture := newXA1DBudgetProviderFixture(t, 1000000, "cerebras", "qwen3.8-27b", xa1dBudgetAnswer, func(policy *proxy.QualificationBudgetConfiguration) {
					policy.ExpiresAt = time.Now().Add(-time.Minute).UTC().Truncate(time.Second).Format(time.RFC3339)
				})
				if response := fixture.request(t, fixture.secret, ""); response.Code == http.StatusOK || fixture.calls.Load() != 0 {
					t.Fatalf("expired budget dispatched: status=%d calls=%d", response.Code, fixture.calls.Load())
				}
				return
			}
			fixture := newXA1DBudgetFixture(t, 1000000, xa1dBudgetAnswer)
			fixture.server.Close()
			_ = fixture.router.Close()
			fixture.router = nil
			switch scenario {
			case "missing":
				if err := os.Remove(fixture.policy.ReceiptDatabasePath); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				file, err := os.Create(fixture.policy.ReceiptDatabasePath)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = file.WriteString("corrupt journal fixture")
				_ = file.Close()
			case "wrong-owner":
				changed := *fixture.policy
				changed.OwnerUserID = "different-owner"
				fixture.configuration.QualificationBudget = &changed
			case "unpriced":
				changed := *fixture.policy
				changed.Offerings = []proxy.QualificationBudgetOffering{{Provider: "openai", Model: "gpt-4.1", MaxOutputTokens: 128}}
				fixture.configuration.QualificationBudget = &changed
			}
			if router, err := proxy.BuildRouter(fixture.configuration, zap.NewNop().Sugar()); err == nil {
				_ = router.Close()
				t.Fatal("invalid initialized policy started")
			}
		})
	}
}

func TestQualificationBudgetXA1DTransportUncertaintyAndRedirect(t *testing.T) {
	for _, scenario := range []string{"lost-response", "truncated-response", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newXA1DBudgetFixture(t, 1000000, func(w http.ResponseWriter, r *http.Request) {
				switch scenario {
				case "lost-response":
					connection, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = connection.Close()
				case "truncated-response":
					w.Header().Set("Content-Length", "10000")
					_, _ = io.WriteString(w, `{"usage":`)
				case "redirect":
					w.Header().Set("Location", "/unexpected-generation")
					w.WriteHeader(http.StatusTemporaryRedirect)
				}
			})
			started := time.Now()
			response := fixture.request(t, fixture.secret, "")
			receipt := fixture.inspect(t)
			if response.Code == http.StatusOK || fixture.calls.Load() != 1 || receipt["suspended"] != true || time.Since(started) > 2*time.Second {
				t.Fatalf("uncertain attempt repeated: status=%d calls=%d receipt=%v", response.Code, fixture.calls.Load(), receipt)
			}
		})
	}
}

func TestQualificationBudgetXA1DVerificationOwnerAndConnectionAttribution(t *testing.T) {
	fixture := newXA1DBudgetProviderFixture(t, 1000000, "openai", "gpt-6-luna", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"id":"verification-fixture","status":"completed","usage":{"input_tokens":2,"output_tokens":3}}`)
	}, func(policy *proxy.QualificationBudgetConfiguration) {
		policy.VerificationConnections = append(policy.VerificationConnections, proxy.QualificationBudgetConnection{Provider: "openai", Name: "new qualification connection"})
	})
	cookie := managementSessionCookie(t, "budget-owner")
	// The selected preassignment connection verifies the provider default model.
	// That exact offering is unpriced/unallowed, so admission rejects before dispatch.
	rejected := httptest.NewRecorder()
	fixture.router.ServeHTTP(rejected, authenticatedJSONRequest(http.MethodPost, "/api/management/connections", `{"name":"  NEW Qualification Connection  ","provider":"openai","fields":{"api_key":"new-local-fixture"}}`, cookie))
	if rejected.Code != http.StatusServiceUnavailable {
		t.Fatalf("selected verification status=%d body=%s", rejected.Code, rejected.Body.String())
	}
	if fixture.calls.Load() != 0 {
		t.Fatal("selected verification bypassed budget")
	}
	accountConnectionExchange(t, fixture.router, cookie, http.MethodPost, "/connections", map[string]any{"name": "unrelated connection", "provider": "openai", "fields": map[string]string{"api_key": "other-local-fixture"}}, http.StatusCreated)
	other := managementSessionCookie(t, "another-owner")
	accountConnectionExchange(t, fixture.router, other, http.MethodPost, "/connections", map[string]any{"name": "new qualification connection", "provider": "openai", "fields": map[string]string{"api_key": "other-owner-fixture"}}, http.StatusCreated)
	if fixture.calls.Load() != 2 || len(fixture.inspect(t)["attempts"].([]any)) != 0 {
		t.Fatalf("attribution crossed owner/name boundary: calls=%d", fixture.calls.Load())
	}
}

func TestQualificationBudgetXA1DSuppliedTotalsAndAliases(t *testing.T) {
	for _, provider := range []string{"cerebras", "openai"} {
		for _, scenario := range []string{"absent", "consistent", "contradictory", "malformed", "alias-conflict"} {
			t.Run(provider+"/"+scenario, func(t *testing.T) {
				model := "qwen3.8-27b"
				if provider == "openai" {
					model = "gpt-6-luna"
				}
				fixture := newXA1DBudgetProviderFixture(t, 1000000, provider, model, func(w http.ResponseWriter, _ *http.Request) {
					usage := `"prompt_tokens":0,"completion_tokens":0`
					if provider == "openai" {
						usage = `"input_tokens":0,"output_tokens":0`
					}
					switch scenario {
					case "consistent":
						usage += `,"total_tokens":0`
					case "contradictory":
						usage += `,"total_tokens":9`
					case "malformed":
						usage += `,"total_tokens":"0"`
					case "alias-conflict":
						if provider == "openai" {
							usage += `,"prompt_tokens":1`
						} else {
							usage += `,"input_tokens":1`
						}
					}
					if provider == "openai" {
						io.WriteString(w, `{"id":"answer","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{`+usage+`}}`)
					} else {
						io.WriteString(w, `{"choices":[{"message":{"content":"answer"},"finish_reason":"stop"}],"usage":{`+usage+`}}`)
					}
				})
				response := fixture.request(t, fixture.secret, "")
				receipt := fixture.inspect(t)
				valid := scenario == "absent" || scenario == "consistent"
				if valid {
					if response.Code != http.StatusOK || receipt["remaining_micro_usd"] != float64(1000000) || receipt["suspended"] != false {
						t.Fatalf("valid supplied total status=%d receipt=%v", response.Code, receipt)
					}
				} else if response.Code == http.StatusOK || receipt["suspended"] != true || receipt["remaining_micro_usd"] == float64(1000000) || fixture.calls.Load() != 1 {
					t.Fatalf("contradictory total refunded/retried: status=%d receipt=%v calls=%d", response.Code, receipt, fixture.calls.Load())
				}
			})
		}
	}
}

func TestQualificationBudgetXA1DPendingFinalizer(t *testing.T) {
	for _, scenario := range []string{"cancel-wait", "malformed-pending"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture := newXA1DBudgetProviderFixture(t, 1000000, "openai", "gpt-6-luna", func(w http.ResponseWriter, r *http.Request) {
				if scenario == "malformed-pending" {
					io.WriteString(w, `{"id":"response-fixture","status":"in_progress","output":{}}`)
					return
				}
				io.WriteString(w, `{"id":"response-fixture","status":"in_progress"}`)
				if r.Method == http.MethodPost {
					time.AfterFunc(20*time.Millisecond, cancel)
				}
			})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/?key="+fixture.secret+"&provider=openai&model=gpt-6-luna&prompt=test&max_tokens=128", nil).WithContext(ctx)
			fixture.router.ServeHTTP(response, request)
			receipt := fixture.inspect(t)
			expectedCalls := int64(1)
			if scenario == "cancel-wait" {
				expectedCalls = 2
			}
			if response.Code == http.StatusOK || receipt["suspended"] != true || fixture.calls.Load() != expectedCalls {
				t.Fatalf("pending finalizer: status=%d calls=%d receipt=%v", response.Code, fixture.calls.Load(), receipt)
			}
		})
	}
}

func TestQualificationBudgetXA1DConservativeOperatorReconciliation(t *testing.T) {
	fixture := newXA1DBudgetFixture(t, 1000000, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"unknown"},"finish_reason":"stop"}]}`)
	})
	_ = fixture.request(t, fixture.secret, "")
	before := fixture.inspect(t)
	attempt := before["attempts"].([]any)[0].(map[string]any)
	id := uint64(attempt["ID"].(float64))
	reserved := attempt["Reserved"].(float64)
	var output bytes.Buffer
	for range 2 {
		if err := proxy.ReconcileQualificationBudgetMaximum(fixture.configuration.Management.DatabasePath, fixture.policy.BudgetID, id, &output); err != nil {
			t.Fatal(err)
		}
	}
	after := fixture.inspect(t)
	attempt = after["attempts"].([]any)[0].(map[string]any)
	if before["remaining_micro_usd"] != after["remaining_micro_usd"] || after["suspended"] != false || after["conservative_unknown_maximum_micro_usd"] != reserved || after["known_uncached_estimate_micro_usd"] != float64(0) || attempt["State"] != "conservative" || attempt["InputTokens"] != nil || attempt["OutputTokens"] != nil {
		t.Fatalf("conservative reconciliation invented usage/refund: before=%v after=%v", before, after)
	}
	if err := proxy.OperateQualificationBudget(fixture.configuration.Management.DatabasePath, fixture.policy.BudgetID, "close", &output); err != nil {
		t.Fatal(err)
	}
}

func TestQualificationBudgetXA1DProcessLockIntegrityRecoveryAndNewIdentity(t *testing.T) {
	fixture := newXA1DBudgetFixture(t, 1000000, xa1dBudgetAnswer)
	if router, err := proxy.BuildRouter(fixture.configuration, zap.NewNop().Sugar()); err == nil {
		_ = router.Close()
		t.Fatal("concurrent process ownership accepted")
	}
	fixture.server.Close()
	_ = fixture.router.Close()
	fixture.router = nil
	database, err := sql.Open("sqlite", fixture.policy.ReceiptDatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var frozen string
	if err := database.QueryRow(`SELECT frozen FROM qualification_budget_states WHERE id = ?`, fixture.policy.BudgetID).Scan(&frozen); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE qualification_budget_states SET frozen = '[]'`); err != nil {
		t.Fatal(err)
	}
	if router, err := proxy.BuildRouter(fixture.configuration, zap.NewNop().Sugar()); err == nil {
		_ = router.Close()
		t.Fatal("corrupt frozen catalog accepted")
	}
	if _, err := database.Exec(`UPDATE qualification_budget_states SET frozen = ?`, frozen); err != nil {
		t.Fatal(err)
	}
	fixture.start(t)
	var output bytes.Buffer
	if err := proxy.OperateQualificationBudget(fixture.configuration.Management.DatabasePath, fixture.policy.BudgetID, "close", &output); err != nil {
		t.Fatal(err)
	}
	fixture.server.Close()
	_ = fixture.router.Close()
	fixture.router = nil
	next := *fixture.policy
	next.BudgetID = "future-explicit-budget"
	next.ReceiptDatabasePath = filepath.Join(filepath.Dir(next.ReceiptDatabasePath), "future.sqlite")
	fixture.configuration.QualificationBudget = &next
	fixture.policy = &next
	fixture.start(t)
	if response := fixture.request(t, fixture.secret, ""); response.Code != http.StatusOK {
		t.Fatalf("new explicit budget failed: %d %s", response.Code, response.Body.String())
	}
	if receipt := fixture.inspect(t); receipt["budget_id"] != "future-explicit-budget" || receipt["remaining_micro_usd"] != float64(999993) {
		t.Fatalf("future identity receipt=%v", receipt)
	}
}

func TestQualificationBudgetXA1DReservationPersistenceFailureIsTerminal(t *testing.T) {
	fixture := newXA1DBudgetProviderFixture(t, 1000000, "openai", "gpt-6-luna", func(w http.ResponseWriter, _ *http.Request) { t.Error("provider reached after failed reservation") })
	database, err := sql.Open("sqlite", fixture.policy.ReceiptDatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TRIGGER reject_qualification_reserve BEFORE INSERT ON qualification_budget_attempts BEGIN SELECT RAISE(ABORT,'controlled_reserve_failure'); END`); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	response := fixture.request(t, fixture.secret, "")
	if response.Code == http.StatusOK || fixture.calls.Load() != 0 || time.Since(started) > 2*time.Second {
		t.Fatalf("reservation failure retried: status=%d calls=%d duration=%s", response.Code, fixture.calls.Load(), time.Since(started))
	}
}

func TestQualificationBudgetXA1DGeminiThoughtsAndAnthropicUncachedBasis(t *testing.T) {
	for _, scenario := range []string{"gemini", "gemini-conflicting-total", "gemini-tool-usage", "gemini-missing-thoughts", "anthropic"} {
		t.Run(scenario, func(t *testing.T) {
			provider, model := "gemini", "gemini-3.5-flash-lite"
			if scenario == "anthropic" {
				provider, model = "anthropic", "claude-haiku-5-5"
			}
			fixture := newXA1DBudgetProviderFixture(t, 1000000, provider, model, func(w http.ResponseWriter, _ *http.Request) {
				if provider == "anthropic" {
					io.WriteString(w, `{"id":"answer","type":"message","role":"assistant","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3,"cache_read_input_tokens":3,"cache_creation_input_tokens":4}}`)
					return
				}
				total := 9
				tool := 0
				if scenario == "gemini-conflicting-total" {
					total = 8
				}
				if scenario == "gemini-tool-usage" {
					tool = 1
					total = 10
				}
				payload := map[string]any{"status": "completed", "steps": []any{map[string]any{"type": "model_output", "content": []any{map[string]any{"type": "text", "text": "answer"}}}}, "usage": map[string]any{"total_input_tokens": 2, "total_output_tokens": 3, "total_thought_tokens": 4, "total_tokens": total, "total_tool_use_tokens": tool}}
				if scenario == "gemini-missing-thoughts" {
					delete(payload["usage"].(map[string]any), "total_thought_tokens")
				}
				_ = json.NewEncoder(w).Encode(payload)
			})
			response := fixture.request(t, fixture.secret, "")
			receipt := fixture.inspect(t)
			if scenario == "gemini" || scenario == "anthropic" {
				expected := float64(999981)
				if scenario == "anthropic" {
					expected = 999997
				}
				if response.Code != http.StatusOK || receipt["remaining_micro_usd"] != expected {
					t.Fatalf("uncached basis status=%d receipt=%v body=%s", response.Code, receipt, response.Body.String())
				}
			} else if response.Code == http.StatusOK || receipt["suspended"] != true {
				t.Fatalf("conflicting/unsupported Gemini usage admitted: status=%d receipt=%v", response.Code, receipt)
			}
		})
	}
}

func TestQualificationBudgetXA1DConfiguredRouteAndBillableFeatures(t *testing.T) {
	fixture := newXA1DBudgetProviderFixture(t, 1000000, "openai", "gpt-6-luna", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["max_output_tokens"] != float64(128) {
			t.Errorf("wire output cap missing: %v", payload)
		}
		io.WriteString(w, `{"id":"answer","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":2,"output_tokens":3}}`)
	})
	// The public request omits its limit; the private policy supplies the wire cap.
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+fixture.secret+"&provider=openai&model=gpt-6-luna&prompt=test", nil))
	if response.Code != http.StatusOK || fixture.calls.Load() != 1 {
		t.Fatalf("omitted cap status=%d calls=%d body=%s", response.Code, fixture.calls.Load(), response.Body.String())
	}
	for _, body := range []string{
		`{"model":"gpt-6-luna","max_tokens":128,"messages":[{"role":"user","content":"test"}],"tools":[{"name":"lookup","description":"Lookup data","parameters":{"type":"object","properties":{},"additionalProperties":false}}]}`,
		`{"model":"gpt-6-luna","max_tokens":128,"messages":[{"role":"user","content":"test","attachments":[{"type":"image","mime_type":"image/jpeg","data":"aW1hZ2U="}]}]}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/v2?provider=openai&key="+fixture.secret, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		rejected := httptest.NewRecorder()
		fixture.router.ServeHTTP(rejected, request)
		if rejected.Code == http.StatusOK || fixture.calls.Load() != 1 {
			t.Fatalf("billable feature reached upstream: status=%d calls=%d body=%s", rejected.Code, fixture.calls.Load(), rejected.Body.String())
		}
	}
	// A different enabled model on the same configured connection is outside the exact allowlist.
	rejected := httptest.NewRecorder()
	fixture.router.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/?key="+fixture.secret+"&provider=openai&model=gpt-4.1&prompt=test&max_tokens=128", nil))
	if rejected.Code == http.StatusOK || fixture.calls.Load() != 1 || len(fixture.inspect(t)["attempts"].([]any)) != 1 {
		t.Fatalf("unlisted configured route dispatched: status=%d calls=%d", rejected.Code, fixture.calls.Load())
	}
}

func TestQualificationBudgetXA1DObservationDurabilityFailures(t *testing.T) {
	for _, scenario := range []string{"pending-id-missing", "pending-write-failure", "settlement-write-failure", "poll-redirect"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newXA1DBudgetProviderFixture(t, 1000000, "openai", "gpt-6-luna", func(w http.ResponseWriter, r *http.Request) {
				switch scenario {
				case "pending-id-missing":
					io.WriteString(w, `{"status":"in_progress"}`)
				case "pending-write-failure":
					io.WriteString(w, `{"id":"pending-fixture","status":"in_progress"}`)
				case "settlement-write-failure":
					io.WriteString(w, `{"id":"answer","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":2,"output_tokens":3}}`)
				case "poll-redirect":
					if r.Method == http.MethodPost {
						io.WriteString(w, `{"id":"pending-fixture","status":"in_progress"}`)
					} else {
						w.Header().Set("Location", "/unadmitted-observation")
						w.WriteHeader(http.StatusTemporaryRedirect)
					}
				}
			})
			database, err := sql.Open("sqlite", fixture.policy.ReceiptDatabasePath)
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			switch scenario {
			case "pending-write-failure":
				_, err = database.Exec(`CREATE TRIGGER pending_write_failure BEFORE UPDATE OF resource_id ON qualification_budget_attempts BEGIN SELECT RAISE(ABORT,'controlled_pending_write_failure'); END`)
			case "settlement-write-failure":
				_, err = database.Exec(`CREATE TRIGGER settlement_write_failure BEFORE UPDATE OF state ON qualification_budget_attempts WHEN NEW.state='settled' BEGIN SELECT RAISE(ABORT,'controlled_settlement_write_failure'); END`)
			}
			if err != nil {
				t.Fatal(err)
			}
			response := fixture.request(t, fixture.secret, "")
			receipt := fixture.inspect(t)
			expectedCalls := int64(1)
			if scenario == "poll-redirect" {
				expectedCalls = 2
			}
			if response.Code == http.StatusOK || fixture.calls.Load() != expectedCalls || receipt["suspended"] != true {
				t.Fatalf("observation uncertainty escaped: status=%d calls=%d receipt=%v", response.Code, fixture.calls.Load(), receipt)
			}
		})
	}
}

func TestQualificationBudgetXA1DGeminiOmittedWireBound(t *testing.T) {
	fixture := newXA1DBudgetProviderFixture(t, 1000000, "gemini", "gemini-3.5-flash-lite", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		configuration, _ := payload["generation_config"].(map[string]any)
		if configuration["max_output_tokens"] != float64(128) {
			t.Errorf("Gemini omitted output bound not enforced: %v", payload)
		}
		io.WriteString(w, `{"status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"answer"}]}],"usage":{"total_input_tokens":2,"total_output_tokens":3,"total_thought_tokens":0,"total_tokens":5}}`)
	})
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?key="+fixture.secret+"&provider=gemini&model=gemini-3.5-flash-lite&prompt=test", nil))
	if response.Code != http.StatusOK || fixture.calls.Load() != 1 {
		t.Fatalf("Gemini omitted bound status=%d calls=%d body=%s", response.Code, fixture.calls.Load(), response.Body.String())
	}
}

func TestQualificationBudgetXA1DStructuredOriginAttributionAndReplay(t *testing.T) {
	fixture := newXA1DBudgetProviderFixture(t, 1000000, "openai", "gpt-6-luna", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"id":"answer","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"score\":1}"}]}],"usage":{"input_tokens":2,"output_tokens":3}}`)
	})
	body := `{"model":"gpt-6-luna","max_tokens":128,"messages":[{"role":"user","content":"score"}],"structured_output":{"schema":{"type":"object","properties":{"score":{"type":"integer","enum":[1,2,3,4,5,6,7,8,9,10]}},"required":["score"],"additionalProperties":false}}}`
	firstID := ""
	for index := 0; index < 2; index++ {
		request, err := http.NewRequest(http.MethodPost, fixture.server.URL+"/v2?provider=openai&key="+fixture.secret, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "structured-origin")
		response, err := fixture.server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("structured status=%d body=%s", response.StatusCode, data)
		}
		if index == 0 {
			firstID = response.Header.Get("X-LLM-Proxy-Request-ID")
		}
	}
	receipt := fixture.inspect(t)
	attempts := receipt["attempts"].([]any)
	if fixture.calls.Load() != 1 || len(attempts) != 1 || firstID == "" || attempts[0].(map[string]any)["RequestID"] != firstID {
		t.Fatalf("structured request origin/replay linkage=%v first=%s calls=%d", receipt, firstID, fixture.calls.Load())
	}
}

func TestQualificationBudgetXA1DSuspensionWriteFailureRequiresExplicitReconciliation(t *testing.T) {
	var observed atomic.Int64
	fixture := newXA1DBudgetFixture(t, 1000000, func(w http.ResponseWriter, r *http.Request) {
		if observed.Add(1) == 1 {
			io.WriteString(w, `{"choices":[{"message":{"content":"unknown"},"finish_reason":"stop"}]}`)
		} else {
			xa1dBudgetAnswer(w, r)
		}
	})
	database, err := sql.Open("sqlite", fixture.policy.ReceiptDatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TRIGGER suspend_persistence_failure BEFORE UPDATE OF suspended ON qualification_budget_states WHEN NEW.suspended=true BEGIN SELECT RAISE(ABORT,'controlled_suspend_persistence_failure'); END`); err != nil {
		t.Fatal(err)
	}
	if response := fixture.request(t, fixture.secret, ""); response.Code == http.StatusOK {
		t.Fatal("unknown usage accepted")
	}
	if response := fixture.request(t, fixture.secret, ""); response.Code == http.StatusOK || fixture.calls.Load() != 1 {
		t.Fatal("failed suspension write allowed another dispatch")
	}
	receipt := fixture.inspect(t)
	id := uint64(receipt["attempts"].([]any)[0].(map[string]any)["ID"].(float64))
	if _, err := database.Exec(`DROP TRIGGER suspend_persistence_failure`); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := proxy.ReconcileQualificationBudgetMaximum(fixture.configuration.Management.DatabasePath, fixture.policy.BudgetID, id, &output); err != nil {
		t.Fatal(err)
	}
	if response := fixture.request(t, fixture.secret, ""); response.Code != http.StatusOK || fixture.calls.Load() != 2 {
		t.Fatalf("explicit reconciliation failed to restore execution: status=%d calls=%d body=%s", response.Code, fixture.calls.Load(), response.Body.String())
	}
	after := fixture.inspect(t)
	if after["known_uncached_estimate_micro_usd"] != float64(7) || after["suspended"] != false {
		t.Fatalf("postreconciliation cost=%v", after)
	}
}

func TestQualificationBudgetXA1DRouterRejectsUnsafeHTTPClientAndReleasesJournal(t *testing.T) {
	fixture := newXA1DBudgetFixture(t, 1000000, xa1dBudgetAnswer)
	fixture.server.Close()
	_ = fixture.router.Close()
	fixture.router = nil
	previous := proxy.HTTPClient
	proxy.HTTPClient = managementProviderKeyVerificationHTTPDoer{next: previous}
	router, err := proxy.BuildRouter(fixture.configuration, zap.NewNop().Sugar())
	proxy.HTTPClient = previous
	if err == nil {
		_ = router.Close()
		t.Fatal("unsafe client accepted")
	}
	if !strings.Contains(err.Error(), "standard HTTP client required") {
		t.Fatalf("unexpected startup rejection: %v", err)
	}
	fixture.start(t)
	if response := fixture.request(t, fixture.secret, ""); response.Code != http.StatusOK {
		t.Fatalf("failed startup leaked journal ownership: status=%d body=%s", response.Code, response.Body.String())
	}
}
