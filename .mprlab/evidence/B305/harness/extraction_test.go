package evaluation_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

const (
	inputText    = "Extract vendor, amount, and currency from this text: Invoice from ACME for USD 42.50."
	expectedText = `{"vendor":"ACME","amount":42.5,"currency":"USD"}`
	schemaText   = `{"type":"object","additionalProperties":false,"required":["vendor","amount","currency"],"properties":{"vendor":{"type":"string"},"amount":{"type":"number"},"currency":{"type":"string","enum":["USD"]}}}`
	tenantKey    = "synthetic-evaluation-tenant-key"
)

type loopbackTransport struct{}

func (loopbackTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	host := request.URL.Hostname()
	address := net.ParseIP(host)
	if host != "localhost" && (address == nil || !address.IsLoopback()) {
		return nil, fmt.Errorf("benchmark rejects non-loopback destination: %s", host)
	}
	return http.DefaultTransport.RoundTrip(request)
}

type fixture struct {
	mu       sync.Mutex
	scenario string
	attempts int
	provider string
	requests []map[string]any
}

func (state *fixture) serve(writer http.ResponseWriter, request *http.Request) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.attempts++
	var payload map[string]any
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writer.WriteHeader(400)
		return
	}
	state.requests = append(state.requests, payload)
	writer.Header().Set("Content-Type", "application/json")
	if state.scenario == "rate_limit" || state.scenario == "provider_failure" {
		status := 429
		if state.scenario == "provider_failure" {
			status = 503
		}
		writer.Header().Set("Retry-After", "1")
		writer.WriteHeader(status)
		errorType := "rate_limit_error"
		if status == 503 {
			errorType = "overloaded_error"
		}
		json.NewEncoder(writer).Encode(map[string]any{"error": map[string]any{"message": "fixture-private-body", "type": errorType}})
		return
	}
	output := expectedText
	if state.scenario == "invalid_output" {
		output = `{"vendor":"ACME","amount":"not-a-number","currency":"USD"}`
	}
	response := map[string]any{}
	if state.provider == "openai" {
		response = map[string]any{"id": "fixture-response", "status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": output}}}}}
	} else {
		response = map[string]any{"id": "fixture-message", "content": []any{map[string]any{"type": "text", "text": output}}, "stop_reason": "end_turn"}
	}
	if state.scenario != "missing_usage" {
		input, output := 10, 3
		if state.scenario == "zero_usage" {
			input, output = 0, 0
		}
		response["usage"] = map[string]int{"input_tokens": input, "output_tokens": output}
		if state.provider == "openai" {
			response["usage"].(map[string]int)["total_tokens"] = input + output
		}
	}
	json.NewEncoder(writer).Encode(response)
}

// responsesPayload is the single existing Responses caller contract.
func responsesPayload(model string, schema any) map[string]any {
	return map[string]any{
		"model": model, "input": inputText, "max_output_tokens": 64, "stream": false,
		"text": map[string]any{"format": map[string]any{"type": "json_schema", "name": "invoice", "strict": true, "schema": schema}},
	}
}

// directPayload adds only the second native provider's required request mapping.
func directPayload(provider, model string, schema any) map[string]any {
	if provider == "openai" {
		return responsesPayload(model, schema)
	}
	return map[string]any{
		"model": model, "messages": []any{map[string]any{"role": "user", "content": inputText}}, "max_tokens": 64, "stream": false,
		"output_config": map[string]any{"format": map[string]any{"type": "json_schema", "schema": schema}},
	}
}

// extractText supports the existing Responses shape and the second native shape.
func extractText(body map[string]any, provider string) string {
	if provider == "anthropic" {
		for _, raw := range body["content"].([]any) {
			item := raw.(map[string]any)
			if item["type"] == "text" {
				return item["text"].(string)
			}
		}
		return ""
	}
	if text, ok := body["output_text"].(string); ok {
		return text
	}
	for _, raw := range body["output"].([]any) {
		item := raw.(map[string]any)
		if item["type"] == "message" {
			for _, rawPart := range item["content"].([]any) {
				part := rawPart.(map[string]any)
				if part["type"] == "output_text" {
					return part["text"].(string)
				}
			}
		}
	}
	return ""
}

func exchange(t *testing.T, handler http.Handler, method, path string, body any, cookie *http.Cookie) map[string]any {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("management %s %s: %d %s", method, path, response.Code, response.Body)
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func cookie(t *testing.T, config proxy.ManagementConfiguration) *http.Cookie {
	t.Helper()
	claims := jwt.MapClaims{"iss": proxy.DefaultManagementJWTIssuer, "tenant_id": config.TAuthTenantID, "user_id": "managed-router-fixture-user", "user_email": "managed-router@example.com", "iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(time.Hour).Unix()}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.JWTSigningKey))
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: config.SessionCookieName, Value: token}
}

type observation struct {
	Alternative     string         `json:"alternative"`
	Route           string         `json:"route"`
	Scenario        string         `json:"scenario"`
	HTTPStatus      int            `json:"http_status"`
	Attempts        int            `json:"upstream_attempts"`
	CallerAccepted  bool           `json:"caller_accepted_extraction"`
	UsagePresent    bool           `json:"wire_usage_present"`
	Usage           any            `json:"wire_usage"`
	Error           any            `json:"error"`
	RetryAfter      string         `json:"retry_after"`
	PersistedDelta  any            `json:"persisted_summary_delta,omitempty"`
	PublicPayload   map[string]any `json:"public_payload"`
	UpstreamPayload map[string]any `json:"upstream_payload,omitempty"`
}

func TestExtractionBenchmark(t *testing.T) {
	var schema any
	if err := json.Unmarshal([]byte(schemaText), &schema); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("urn:extraction", schema); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile("urn:extraction")
	if err != nil {
		t.Fatal(err)
	}
	var expected any
	json.Unmarshal([]byte(expectedText), &expected)
	client := &http.Client{Transport: loopbackTransport{}, Timeout: 5 * time.Second}
	originalHTTP := proxy.HTTPClient
	proxy.HTTPClient = client
	t.Cleanup(func() { proxy.HTTPClient = originalHTTP })
	states := map[string]*fixture{}
	servers := map[string]*httptest.Server{}
	endpoints := proxy.NewEndpoints()
	for _, provider := range []string{"openai", "anthropic"} {
		state := &fixture{provider: provider, scenario: "known_usage"}
		server := httptest.NewServer(http.HandlerFunc(state.serve))
		t.Cleanup(server.Close)
		states[provider], servers[provider] = state, server
		endpoints.SetProviderBaseURL(provider, server.URL)
	}
	managed := testfixtures.ManagedTenant{Secret: tenantKey, Defaults: proxy.DefaultTenantDefaults(), ProviderKeys: map[string]string{"openai": "dummy-openai-key", "anthropic": "dummy-anthropic-key"}}
	config, err := testfixtures.ProvisionManagedRouter(t, proxy.Configuration{Endpoints: endpoints, AssetStorePath: t.TempDir(), RequestTimeoutSeconds: 5, LogLevel: "error"}, zap.NewNop().Sugar(), managed)
	if err != nil {
		t.Fatal(err)
	}
	router, err := proxy.BuildRouter(config, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := router.Close(); err != nil {
			t.Error(err)
		}
	})
	proxyServer := httptest.NewServer(router)
	t.Cleanup(proxyServer.Close)
	session := cookie(t, config.Management)
	account := exchange(t, router, "GET", "/api/management/account", nil, session)
	tenantID := account["tenants"].([]any)[0].(map[string]any)["id"].(string)
	usagePath := "/api/management/tenants/" + tenantID + "/usage?interval=all"
	results := []observation{}
	for _, route := range []struct{ provider, model string }{{"openai", "gpt-4.1"}, {"anthropic", "claude-sonnet-5"}} {
		for _, scenario := range []string{"known_usage", "missing_usage", "zero_usage", "rate_limit", "provider_failure", "invalid_output"} {
			for _, alternative := range []string{"direct", "proxy"} {
				state := states[route.provider]
				state.mu.Lock()
				state.scenario = scenario
				state.attempts = 0
				state.requests = nil
				state.mu.Unlock()
				before := exchange(t, router, "GET", usagePath, nil, session)["totals"].(map[string]any)
				payload := directPayload(route.provider, route.model, schema)
				url := servers[route.provider].URL
				if route.provider == "anthropic" {
					url += "/v1/messages"
				}
				if alternative == "proxy" {
					payload = responsesPayload(route.provider+"/"+route.model, schema)
					url = proxyServer.URL + "/v1/responses"
				}
				data, _ := json.Marshal(payload)
				request, _ := http.NewRequest("POST", url, bytes.NewReader(data))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Authorization", "Bearer dummy-openai-key")
				if alternative == "proxy" {
					request.Header.Set("Authorization", "Bearer "+tenantKey)
				} else if route.provider == "anthropic" {
					request.Header.Del("Authorization")
					request.Header.Set("x-api-key", "dummy-anthropic-key")
					request.Header.Set("anthropic-version", "2023-06-01")
				}
				response, transportError := client.Do(request)
				status := 0
				retryAfter := ""
				bodyBytes := []byte(`{}`)
				if transportError == nil {
					status = response.StatusCode
					retryAfter = response.Header.Get("Retry-After")
					bodyBytes, _ = io.ReadAll(response.Body)
					response.Body.Close()
				}
				var body map[string]any
				json.Unmarshal(bodyBytes, &body)
				accepted := false
				if status == 200 {
					decoderProvider := route.provider
					if alternative == "proxy" {
						decoderProvider = "openai"
					}
					var extracted any
					text := extractText(body, decoderProvider)
					accepted = json.Unmarshal([]byte(text), &extracted) == nil && compiled.Validate(extracted) == nil && reflect.DeepEqual(extracted, expected)
				}
				state.mu.Lock()
				attempts := state.attempts
				upstream := map[string]any{}
				if len(state.requests) > 0 {
					upstream = state.requests[0]
				}
				state.mu.Unlock()
				usage, usageExists := body["usage"]
				obs := observation{Alternative: alternative, Route: route.provider + "/" + route.model, Scenario: scenario, HTTPStatus: status, Attempts: attempts, CallerAccepted: accepted, UsagePresent: usageExists && usage != nil, Usage: usage, Error: body["error"], RetryAfter: retryAfter, PublicPayload: payload, UpstreamPayload: upstream}
				if transportError != nil {
					obs.Error = map[string]any{"class": "caller_transport_error", "timeout": os.IsTimeout(transportError)}
				}
				if alternative == "proxy" {
					var after map[string]any
					deadline := time.Now().Add(5 * time.Second)
					for {
						after = exchange(t, router, "GET", usagePath, nil, session)["totals"].(map[string]any)
						if after["requests"].(float64) == before["requests"].(float64)+1 || time.Now().After(deadline) {
							break
						}
						time.Sleep(10 * time.Millisecond)
					}
					delta := map[string]any{}
					for key, value := range after {
						if old, ok := before[key].(float64); ok {
							delta[key] = value.(float64) - old
						}
					}
					delete(delta, "average_latency_ms")
					coverageDelta := map[string]any{}
					for _, quantity := range []string{"request_tokens", "response_tokens", "total_tokens"} {
						previous := before["token_coverage"].(map[string]any)[quantity].(map[string]any)
						current := after["token_coverage"].(map[string]any)[quantity].(map[string]any)
						counts := map[string]float64{}
						for key, value := range current {
							counts[key] = value.(float64) - previous[key].(float64)
						}
						coverageDelta[quantity] = counts
						if scenario == "known_usage" || scenario == "zero_usage" || scenario == "missing_usage" {
							measured, unknown := float64(1), float64(0)
							if scenario == "missing_usage" {
								measured, unknown = 0, 1
							}
							if counts["measured_requests"] != measured || counts["unknown_requests"] != unknown || counts["partial_requests"] != 0 || counts["historical_requests"] != 0 {
								t.Fatalf("%s %s coverage=%v", scenario, quantity, counts)
							}
						}
					}
					delta["token_coverage"] = coverageDelta

					obs.PersistedDelta = delta
					state.mu.Lock()
					obs.Attempts = state.attempts
					state.mu.Unlock()
				}
				results = append(results, obs)
				t.Logf("%s %s %s status=%d attempts=%d extraction=%t usage=%t", alternative, obs.Route, scenario, obs.HTTPStatus, obs.Attempts, accepted, obs.UsagePresent)
				if scenario == "known_usage" || scenario == "missing_usage" || scenario == "zero_usage" {
					if !accepted {
						t.Errorf("required extraction failed: %+v body=%s", obs, bodyBytes)
					}
				}
				if len(upstream) > 0 && scenario != "rate_limit" && scenario != "provider_failure" {
					var format map[string]any
					if route.provider == "openai" {
						format = upstream["text"].(map[string]any)["format"].(map[string]any)
					} else {
						format = upstream["output_config"].(map[string]any)["format"].(map[string]any)
					}
					if format["type"] != "json_schema" || !reflect.DeepEqual(format["schema"], schema) {
						t.Error("provider schema mapping differs from the required schema")
					}
				}
				if scenario == "invalid_output" && accepted {
					t.Error("caller accepted schema-invalid extraction")
				}
				if alternative == "proxy" && strings.Contains(string(bodyBytes), "fixture-private-body") {
					t.Error("proxy exposed private provider body")
				}
			}
		}
	}
	failureRows := exchange(t, router, "GET", "/api/management/tenants/"+tenantID+"/usage/failures?interval=all", nil, session)
	database, err := sql.Open("sqlite", config.Management.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	columnRows, err := database.Query("PRAGMA table_info(managed_usage_event_records)")
	if err != nil {
		t.Fatal(err)
	}
	columns := []string{}
	for columnRows.Next() {
		var index, notNull, pk int
		var name, dataType string
		var defaultValue any
		if err := columnRows.Scan(&index, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name)
	}
	columnRows.Close()
	recordRows, err := database.Query("SELECT provider_id, model_id, status_code, request_tokens, response_tokens, total_tokens, measurement_evidence FROM managed_usage_event_records ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	records := []map[string]any{}
	for recordRows.Next() {
		var provider, model string
		var status, input, output, total int
		var evidence sql.NullInt64
		if err := recordRows.Scan(&provider, &model, &status, &input, &output, &total, &evidence); err != nil {
			t.Fatal(err)
		}
		records = append(records, map[string]any{"provider": provider, "model": model, "status": status, "input_tokens": input, "output_tokens": output, "total_tokens": total, "measurement_evidence": evidence.Int64, "evidence_present": evidence.Valid})
	}
	recordRows.Close()
	report := map[string]any{"benchmark": "LP-C01-S1-B305", "workflow": "synthetic plain-text structured extraction", "schema": schema, "input": inputText, "expected_output": expected, "observations": results, "failure_rows": failureRows, "database_path_is_temporary": true, "wire_alternatives_executed": []string{"direct", "proxy"}, "litellm_execution": "not run; documented comparison only", "no_latency_or_model_quality_measurements": true, "usage_columns": columns, "persisted_usage_rows": records}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join("results.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
