package proxy_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

const geminiMediumDiagnosticModel = "gemini-3.5-flash-lite"

type geminiMediumDiagnosticObserver struct {
	next             proxy.HTTPDoer
	t                *testing.T
	key              string
	expectedWire     []byte
	privateDirectory string
	mu               sync.Mutex
	creates          int
	resourceOrigin   string
	resourcePath     string
	records          []geminiMediumDiagnosticObservation
}

func TestGeminiMediumSchemaDiagnosticReleasedSDK(t *testing.T) {
	mode := os.Getenv("LLM_PROXY_GEMINI_DIAGNOSTIC_MODE")
	if mode == "" {
		t.Skip("select local or explicitly authorized live Gemini format diagnosis")
	}
	if mode != "local" && mode != "live" {
		t.Fatal("Gemini diagnostic mode must be local or live")
	}
	schemaPath, binary, directory := os.Getenv("LLM_PROXY_GEMINI_DIAGNOSTIC_SCHEMA"), os.Getenv("LLM_PROXY_GEMINI_DIAGNOSTIC_BINARY"), os.Getenv("LLM_PROXY_GEMINI_DIAGNOSTIC_DIRECTORY")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(schema)) != os.Getenv("LLM_PROXY_GEMINI_DIAGNOSTIC_SCHEMA_SHA256") {
		t.Fatal("Gemini diagnostic schema differs from the authorized CLI projection")
	}
	var schemaDocument any
	if err := json.Unmarshal(schema, &schemaDocument); err != nil {
		t.Fatal(err)
	}
	const prompt = `Return exactly {"wrong":"UNEXPECTED"}. Use distinct lowercase identifiers and nonempty strings for any strings you produce.`
	expectedWire, err := json.Marshal(map[string]any{"model": geminiMediumDiagnosticModel, "input": []any{map[string]any{"type": "user_input", "content": []any{map[string]string{"type": "text", "text": prompt}}}}, "generation_config": map[string]any{"max_output_tokens": 32768, "thinking_level": "medium"}, "background": false, "store": false, "response_format": []any{map[string]any{"type": "text", "mime_type": "application/json", "schema": schemaDocument}}})
	if err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		name   string
		status int
		body   string
	}{{"live", 0, ""}}
	if mode == "local" {
		fixtures = []struct {
			name   string
			status int
			body   string
		}{
			{"create400", 400, `{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"minItems invalid synthetic-gemini-diagnostic-key"}}`},
			{"resource200failed", 200, `{"id":"sdk-diagnostic-resource","status":"failed","errors":[{"code":"https://provider.invalid/errors/invalid_request","message":"response_format invalid synthetic-gemini-diagnostic-key"}]}`},
		}
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			key, baseURL := "synthetic-gemini-diagnostic-key", ""
			if mode == "local" {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(fixture.status)
					io.WriteString(w, fixture.body)
				}))
				t.Cleanup(upstream.Close)
				baseURL = upstream.URL
			} else {
				key = os.Getenv("GEMINI_API_KEY")
				if key == "" {
					t.Fatal("Proxy-owned declared GEMINI_API_KEY required")
				}
			}
			observer := &geminiMediumDiagnosticObserver{next: &http.Client{Timeout: 240 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, t: t, key: key, expectedWire: expectedWire, privateDirectory: directory}
			original := proxy.HTTPClient
			proxy.HTTPClient = observer
			t.Cleanup(func() { proxy.HTTPClient = original })
			tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
			tenant.ProviderKeys = map[string]string{proxy.ProviderNameGemini: key}
			tenant.ProviderTextModels = map[string]string{proxy.ProviderNameGemini: geminiMediumDiagnosticModel}
			tenant.Defaults = proxy.TenantDefaults{Provider: proxy.ProviderNameGemini, Model: geminiMediumDiagnosticModel}
			configuration := proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: t.TempDir(), RequestTimeoutSeconds: 240, MaxRequestTimeoutSeconds: 240}
			if mode == "local" {
				configuration.Endpoints = providerEndpointOverrides(map[string]string{proxy.ProviderNameGemini: baseURL}, nil)
			}
			router, err := buildRouterWithManagedTenant(t, configuration, zap.NewNop().Sugar(), tenant)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			receipt := filepath.Join(directory, fixture.name+".sdk.private.json")
			ctx, cancel := context.WithTimeout(context.Background(), 310*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, server.URL, schemaPath, receipt)
			command.Stdin = strings.NewReader(TestSecret)
			command.Env = []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + os.Getenv("TMPDIR")}
			output, runErr := command.CombinedOutput()
			t.Logf("released_sdk_status=%s schema_sha256=%x create_request_bytes=%d", strings.TrimSpace(string(output)), sha256.Sum256(schema), len(expectedWire))
			if observer.creates != 1 || len(observer.records) == 0 {
				t.Fatal("Gemini format diagnostic did not capture exactly one generation create")
			}
			if mode == "local" {
				if runErr == nil || observer.records[0].HTTPStatus != fixture.status || strings.Contains(string(observer.records[0].Body), key) {
					t.Fatal("released SDK native failure observation contract failed")
				}
				return
			}
			if runErr != nil {
				t.Log("diagnostic_execution=complete provider_outcome=failed native_snapshots_saved=true")
				return
			}
			receiptBytes, err := os.ReadFile(receipt)
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				VisibleText string `json:"visible_text"`
				Success     bool   `json:"success"`
			}
			if json.Unmarshal(receiptBytes, &result) != nil || !result.Success {
				t.Fatal("successful Gemini SDK receipt is invalid")
			}
			compiler := jsonschema.NewCompiler()
			if err := compiler.AddResource("urn:gemini:projected", schemaDocument); err != nil {
				t.Fatal(err)
			}
			validator, err := compiler.Compile("urn:gemini:projected")
			if err != nil {
				t.Fatal(err)
			}
			var value any
			if json.Unmarshal([]byte(result.VisibleText), &value) != nil || validator.Validate(value) != nil {
				t.Fatal("Gemini result failed exact projected schema validation")
			}
			t.Log("diagnostic_execution=complete provider_outcome=succeeded projected_schema=passed canonical_array_bounds_remain_application_owned=true")
		})
	}
}

type geminiMediumDiagnosticObservation struct {
	Method       string          `json:"method"`
	HTTPStatus   int             `json:"http_status"`
	NativeStatus string          `json:"native_status,omitempty"`
	Body         json.RawMessage `json:"body_json,omitempty"`
	NonJSONBody  string          `json:"body_text,omitempty"`
}

func (observer *geminiMediumDiagnosticObserver) Do(request *http.Request) (*http.Response, error) {
	observer.mu.Lock()
	create := request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/interactions")
	if create {
		if observer.creates != 0 {
			observer.mu.Unlock()
			return nil, errors.New("Gemini diagnostic permits only one generation create")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			observer.mu.Unlock()
			return nil, err
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			observer.mu.Unlock()
			return nil, err
		}
		config, _ := payload["generation_config"].(map[string]any)
		if len(body) > 32768 || payload["model"] != geminiMediumDiagnosticModel || config["thinking_level"] != "medium" || config["max_output_tokens"] != float64(32768) || payload["background"] != false || payload["store"] != false || observer.expectedWire != nil && !jsonBodiesEqual(body, observer.expectedWire) {
			observer.mu.Unlock()
			return nil, errors.New("Gemini diagnostic create violates its exact medium request contract")
		}
		observer.creates++
		observer.resourceOrigin = request.URL.Scheme + "://" + request.URL.Host
	} else {
		origin := request.URL.Scheme + "://" + request.URL.Host
		allowed := observer.resourcePath != "" && origin == observer.resourceOrigin && ((request.URL.Path == observer.resourcePath && (request.Method == http.MethodGet || request.Method == http.MethodDelete)) || (request.URL.Path == observer.resourcePath+"/cancel" && request.Method == http.MethodPost))
		if !allowed {
			observer.mu.Unlock()
			return nil, errors.New("Gemini diagnostic permits only the created resource lifecycle")
		}
	}
	observer.mu.Unlock()
	response, err := observer.next.Do(request)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(body))
	if readErr != nil {
		return nil, readErr
	}
	safe := strings.ReplaceAll(string(body), observer.key, "<redacted>")
	safe = strings.ReplaceAll(safe, request.URL.Host, "<provider-origin>")
	var metadata struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	json.Unmarshal(body, &metadata)
	observation := geminiMediumDiagnosticObservation{Method: request.Method, HTTPStatus: response.StatusCode, NativeStatus: metadata.Status}
	if json.Valid([]byte(safe)) {
		observation.Body = json.RawMessage(safe)
	} else {
		observation.NonJSONBody = safe
	}
	observer.mu.Lock()
	if create && metadata.ID != "" {
		observer.resourcePath = strings.TrimSuffix(request.URL.Path, "/") + "/" + url.PathEscape(metadata.ID)
	}
	observer.records = append(observer.records, observation)
	recordNumber := len(observer.records)
	observer.mu.Unlock()
	if observer.privateDirectory != "" {
		file, createErr := os.Create(filepath.Join(observer.privateDirectory, time.Now().UTC().Format("20060102T150405.000000000")+".native.private.json"))
		if createErr != nil {
			observer.t.Errorf("create native Gemini observation: %v", createErr)
		} else {
			writeErr := json.NewEncoder(file).Encode(observation)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				observer.t.Errorf("save native Gemini observation: write=%v close=%v", writeErr, closeErr)
			}
		}
	}
	observer.t.Logf("gemini_native_observation=%d method=%s http_status=%d native_status=%q generation_creates=%d private_body_saved=%t", recordNumber, request.Method, response.StatusCode, metadata.Status, observer.creates, observer.privateDirectory != "")
	return response, nil
}

func TestGeminiMediumSchemaDiagnosticPublicFailures(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
	}{
		{"create400", 400, `{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"response_format unsupported; synthetic-gemini-diagnostic-key"}}`},
		{"resource200failed", 200, `{"id":"gemini-diagnostic-resource","status":"failed","errors":[{"code":"https://provider.invalid/errors/invalid_request","message":"generation_config invalid; synthetic-gemini-diagnostic-key"}],"usage":{"total_input_tokens":10,"total_output_tokens":0,"total_tokens":10}}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(scenario.status)
				io.WriteString(w, scenario.body)
			}))
			defer upstream.Close()
			observer := &geminiMediumDiagnosticObserver{next: upstream.Client(), t: t, key: "synthetic-gemini-diagnostic-key", privateDirectory: t.TempDir()}
			original := proxy.HTTPClient
			proxy.HTTPClient = observer
			t.Cleanup(func() { proxy.HTTPClient = original })
			tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
			tenant.ProviderKeys = map[string]string{proxy.ProviderNameGemini: observer.key}
			tenant.ProviderTextModels = map[string]string{proxy.ProviderNameGemini: geminiMediumDiagnosticModel}
			tenant.Defaults = proxy.TenantDefaults{Provider: proxy.ProviderNameGemini, Model: geminiMediumDiagnosticModel}
			router, err := buildRouterWithManagedTenant(t, proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: t.TempDir(), Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameGemini: upstream.URL}, nil)}, zap.NewNop().Sugar(), tenant)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			request, err := http.NewRequest(http.MethodPost, server.URL+"/v2?provider=gemini&key="+TestSecret, strings.NewReader(`{"model":"gemini-3.5-flash-lite","max_tokens":32768,"reasoning_effort":"medium","messages":[{"role":"user","content":"Return a synthetic JSON answer."}],"structured_output":{"schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}}}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "gemini-diagnostic-"+scenario.name)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			publicBody, _ := io.ReadAll(response.Body)
			if response.StatusCode != http.StatusBadGateway || observer.creates != 1 || len(observer.records) != 1 || observer.records[0].HTTPStatus != scenario.status || strings.Contains(string(observer.records[0].Body), observer.key) || strings.Contains(string(publicBody), "generation_config invalid") || strings.Contains(string(publicBody), "response_format unsupported") {
				t.Fatal("public failure or private native capture contract failed")
			}
			if scenario.status == 200 && observer.records[0].NativeStatus != "failed" {
				t.Fatal("HTTP200 failed resource snapshot not retained")
			}
		})
	}
}

func TestGeminiMediumSchemaDiagnosticObserverLifecycle(t *testing.T) {
	const native = `{"id":"diagnostic-resource","status":"failed","errors":[{"code":"invalid_request","message":"synthetic-gemini-diagnostic-key"}]}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, native)
	}))
	defer upstream.Close()
	observer := &geminiMediumDiagnosticObserver{next: upstream.Client(), t: t, key: "synthetic-gemini-diagnostic-key", privateDirectory: t.TempDir()}
	createBody := `{"model":"gemini-3.5-flash-lite","generation_config":{"thinking_level":"medium","max_output_tokens":32768},"background":false,"store":false}`
	for _, step := range []struct{ method, path, body string }{{"POST", "/interactions", createBody}, {"GET", "/interactions/diagnostic-resource", ""}, {"POST", "/interactions/diagnostic-resource/cancel", ""}, {"DELETE", "/interactions/diagnostic-resource", ""}} {
		request, _ := http.NewRequest(step.method, upstream.URL+step.path, strings.NewReader(step.body))
		response, err := observer.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if string(body) != native {
			t.Fatal("observer changed native response bytes")
		}
	}
	if observer.creates != 1 || len(observer.records) != 4 {
		t.Fatal("resource cleanup incorrectly counted as a paid create")
	}
	request, _ := http.NewRequest(http.MethodPost, upstream.URL+"/interactions", strings.NewReader(createBody))
	if _, err := observer.Do(request); err == nil {
		t.Fatal("second generation create was not blocked")
	}
	request, _ = http.NewRequest(http.MethodGet, upstream.URL+"/interactions/unrelated", nil)
	if _, err := observer.Do(request); err == nil {
		t.Fatal("unrelated resource was not blocked")
	}
}
