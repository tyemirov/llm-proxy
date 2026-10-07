package proxy_test

import (
	"database/sql"
	"encoding/json"
	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func TestManagementTokenMeasurementPresence(t *testing.T) {
	fixtures := []struct {
		name, usage             string
		known, partial, unknown int
		tokens                  int
		second                  string
		poll                    bool
	}{
		{"absent", "", 0, 0, 1, 0, "", false},
		{"empty", `,"usage":{}`, 0, 0, 1, 0, "", false},
		{"zero", `,"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}`, 1, 0, 0, 0, "", false},
		{"measured", `,"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}`, 1, 0, 0, 13, "", false},
		{"input only", `,"usage":{"input_tokens":10}`, 0, 0, 1, 0, "", false},
		{"total only zero", `,"usage":{"total_tokens":0}`, 1, 0, 0, 0, "", false},
		{"derived total", `,"usage":{"input_tokens":10,"output_tokens":3}`, 1, 0, 0, 13, "", false},
		{"both missing continuation", "", 0, 0, 1, 0, "absent", false},
		{"known then missing continuation", `,"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}`, 0, 1, 0, 13, "absent", false},
		{"missing then known continuation", "", 0, 1, 0, 13, `,"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}`, false},
		{"poll measured then missing", `,"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}`, 0, 1, 0, 13, "absent", true},
		{"poll repeated measured", `,"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}`, 1, 0, 0, 13, `,"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}`, true},
		{"poll explicit zero replaces measured", `,"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}`, 1, 0, 0, 0, `,"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}`, true},
		{"poll missing then measured", "", 1, 0, 0, 13, `,"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}`, true},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			upstreamAttempts := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				upstreamAttempts++
				usage := fixture.usage
				status := `"completed"`
				if fixture.poll && upstreamAttempts == 1 {
					status = `"queued"`
				}
				if !fixture.poll && fixture.second != "" && upstreamAttempts == 1 {
					status = `"incomplete","incomplete_details":{"reason":"max_output_tokens"}`
				}
				if fixture.second != "" && upstreamAttempts > 1 {
					usage = fixture.second
					if usage == "absent" {
						usage = ""
					}
				}
				io.WriteString(w, `{"id":"presence","status":`+status+`,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}]`+usage+`}`)
			}))
			defer upstream.Close()
			databasePath := filepath.Join(t.TempDir(), "usage.sqlite")
			configuration := proxy.Configuration{Endpoints: providerEndpoints(upstream.URL, proxy.ProviderNameOpenAI)}
			router := newTokenPresenceRouter(t, configuration, databasePath)
			cookie := managementSessionCookie(t, "presence-owner")
			tenantID := requestManagementAccount(t, router, cookie).Tenants[0].ID
			saveManagementProviderKey(t, router, cookie, tenantID, "dummy-provider-key", proxy.ModelNameGPT41, "")
			secret := generateManagementTenantSecret(t, router, cookie, tenantID)
			server := httptest.NewServer(router)
			request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/responses", strings.NewReader(`{"model":"openai/gpt-4.1","input":"text","stream":false}`))
			request.Header.Set("Authorization", "Bearer "+secret)
			request.Header.Set("Content-Type", "application/json")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			json.NewDecoder(response.Body).Decode(&body)
			response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatalf("completion status=%d body=%v", response.StatusCode, body)
			}
			if fixture.name == "input only" || fixture.name == "total only zero" {
				if body["usage"] != nil {
					t.Fatalf("incomplete completion usage must remain unknown: %v", body["usage"])
				}
			}
			path := "/api/management/tenants/" + tenantID + "/usage?interval=all"
			read := func(handler http.Handler, target string) map[string]any {
				t.Helper()
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, authenticatedJSONRequest(http.MethodGet, target, "", cookie))
				if recorder.Code != 200 {
					t.Fatalf("usage %d %s", recorder.Code, recorder.Body.String())
				}
				var value map[string]any
				json.Unmarshal(recorder.Body.Bytes(), &value)
				return value
			}
			deadline := time.Now().Add(5 * time.Second)
			var summary map[string]any
			for {
				summary = read(router, path)
				if summary["totals"].(map[string]any)["requests"] == float64(1) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("usage commit did not complete")
				}
				time.Sleep(time.Millisecond)
			}
			verify := func(summary map[string]any) {
				t.Helper()
				totals := summary["totals"].(map[string]any)
				coverage, ok := totals["token_coverage"].(map[string]any)
				if !ok {
					t.Fatalf("missing measurement coverage: %v", totals)
				}
				assertCoverage := func(data map[string]any) {
					t.Helper()
					for _, metric := range []string{"request_tokens", "response_tokens", "total_tokens"} {
						known, partial, unknown := fixture.known, fixture.partial, fixture.unknown
						if fixture.name == "input only" && metric == "request_tokens" {
							known, unknown = 1, 0
						}
						if fixture.name == "total only zero" && metric != "total_tokens" {
							known, unknown = 0, 1
						}
						quantity := data[metric].(map[string]any)
						for field, want := range map[string]int{"measured_requests": known, "partial_requests": partial, "unknown_requests": unknown, "historical_requests": 0} {
							if quantity[field] != float64(want) {
								t.Errorf("%s.%s=%v want %d", metric, field, quantity[field], want)
							}
						}
					}
				}
				assertCoverage(coverage)
				if totals["total_tokens"] != float64(fixture.tokens) {
					t.Errorf("tokens=%v want %d", totals["total_tokens"], fixture.tokens)
				}
				for _, collection := range []string{"buckets", "providers", "models"} {
					rows := summary[collection].([]any)
					found := false
					for _, row := range rows {
						data := row.(map[string]any)["data"].(map[string]any)
						if data["requests"] == float64(1) {
							found = true
							groupCoverage, ok := data["token_coverage"].(map[string]any)
							if !ok {
								t.Fatalf("%s coverage absent", collection)
							}
							assertCoverage(groupCoverage)
						}
					}
					if !found {
						t.Errorf("%s executed event absent", collection)
					}
				}
			}
			verify(summary)
			verify(read(router, "/api/management/usage?interval=all"))
			denial := httptest.NewRecorder()
			router.ServeHTTP(denial, authenticatedJSONRequest(http.MethodGet, path, "", managementSessionCookie(t, "another-owner")))
			if denial.Code != 404 {
				t.Fatalf("tenant authorization status=%d", denial.Code)
			}
			server.Close()
			if err := router.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := newTokenPresenceRouter(t, configuration, databasePath)
			verify(read(reopened, path))
			if fixture.name == "measured" || fixture.name == "zero" || fixture.name == "absent" {
				if err := reopened.Close(); err != nil {
					t.Fatal(err)
				}
				database, err := sql.Open("sqlite", databasePath)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = database.Exec("ALTER TABLE managed_usage_event_records DROP COLUMN measurement_evidence"); err != nil {
					t.Fatal(err)
				}
				if err = database.Close(); err != nil {
					t.Fatal(err)
				}
				for startup := 0; startup < 2; startup++ {
					migrated := newTokenPresenceRouter(t, configuration, databasePath)
					totals := read(migrated, path)["totals"].(map[string]any)
					if totals["total_tokens"] != float64(fixture.tokens) {
						t.Fatalf("migration changed quantities: %v", totals)
					}
					for _, metric := range []string{"request_tokens", "response_tokens", "total_tokens"} {
						coverage := totals["token_coverage"].(map[string]any)[metric].(map[string]any)
						if coverage["historical_requests"] != float64(1) || coverage["measured_requests"] != float64(0) {
							t.Fatalf("historical evidence invented: %v", coverage)
						}
					}
					if err := migrated.Close(); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func newTokenPresenceRouter(t *testing.T, configuration proxy.Configuration, path string) *proxy.Router {
	t.Helper()
	configuration = managementConfigurationWithDatabasePath(configuration, path)
	if configuration.AssetStorePath == "" {
		configuration.AssetStorePath = t.TempDir()
	}
	configured, err := configurationWithCatalogs(t, configuration)
	if err != nil {
		t.Fatal(err)
	}
	previous := proxy.HTTPClient
	proxy.HTTPClient = managementProviderKeyVerificationHTTPDoer{next: previous}
	defer func() { proxy.HTTPClient = previous }()
	router, err := proxy.BuildRouter(configured, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := router.Close(); err != nil {
			t.Error(err)
		}
	})
	return router
}

func TestTokenPresenceCompatibilityJSON(t *testing.T) {
	for _, fixture := range []struct {
		name, usage string
		measured    bool
		total       int
	}{
		{"absent", "", false, 0},
		{"empty", `,"usage":{}`, false, 0},
		{"input only", `,"usage":{"input_tokens":10}`, false, 0},
		{"zero", `,"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}`, true, 0},
		{"measured", `,"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}`, true, 13},
		{"tool measured", `,"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}`, true, 13},
		{"tool partial", `,"usage":{"input_tokens":10}`, false, 0},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasPrefix(fixture.name, "tool") {
					io.WriteString(w, `{"id":"presence","status":"completed","output":[{"type":"function_call","call_id":"call_read","name":"read","arguments":"{}"}]`+fixture.usage+`}`)
					return
				}
				io.WriteString(w, `{"id":"presence","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}]`+fixture.usage+`}`)
			}))
			defer upstream.Close()
			router := newTokenPresenceRouter(t, proxy.Configuration{Endpoints: providerEndpoints(upstream.URL, proxy.ProviderNameOpenAI)}, filepath.Join(t.TempDir(), "usage.sqlite"))
			owner := managementSessionCookie(t, "compatibility-owner")
			tenantID := requestManagementAccount(t, router, owner).Tenants[0].ID
			saveManagementProviderKey(t, router, owner, tenantID, "dummy-provider-key", proxy.ModelNameGPT41, "")
			secret := generateManagementTenantSecret(t, router, owner, tenantID)
			server := httptest.NewServer(router)
			defer server.Close()
			response, err := server.Client().Post(server.URL+"/v2?key="+secret+"&provider=openai&format=application/json", "application/json", strings.NewReader(`{"model":"gpt-4.1","messages":[{"role":"user","content":"text"}],"tools":[{"name":"read","parameters":{"type":"object"}}]}`))
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			json.NewDecoder(response.Body).Decode(&body)
			response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatalf("status=%d body=%v", response.StatusCode, body)
			}
			usage, present := body["usage"]
			if (present && usage != nil) != fixture.measured {
				t.Fatalf("usage presence=%v want=%v body=%v", present, fixture.measured, body)
			}
			if fixture.measured {
				counts := usage.(map[string]any)
				if len(counts) != 3 || counts["total_tokens"] != float64(fixture.total) {
					t.Fatalf("public usage=%v", counts)
				}
			} else if response.Header.Get("X-LLM-Proxy-Total-Tokens") != "" {
				t.Fatalf("unknown usage has measured header: %v", response.Header)
			}
		})
	}
}

func TestTokenPresenceVertexNegativeOutput(t *testing.T) {
	server, _ := vertexHTTPFixture(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"answer"}]},"finishReason":"STOP"}],"usageMetadata":{"candidatesTokenCount":-1,"thoughtsTokenCount":5}}`)
	})
	response, err := http.Get(server.URL + "/?key=" + TestSecret + "&provider=gemini&prompt=test")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("negative native component accepted: status=%d", response.StatusCode)
	}
}
