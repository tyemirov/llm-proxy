package proxy_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const cerebrasPaidNativeOutputSchema = `{"type":"object","properties":{"ok":{"type":"string","enum":["READY"]}},"required":["ok"],"additionalProperties":false}`

func TestCerebrasPortableNativeOutputSourceBudget(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"ok\":\"READY\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`)
	}))
	defer upstream.Close()
	server := cerebrasHTTPServer(t, upstream.URL, "cerebras")
	body := `{"model":"qwen3.8-27b","messages":[{"role":"user","content":"Return READY JSON."}],"max_tokens":32768,"reasoning_effort":"medium","structured_output":{"schema":` + cerebrasPaidNativeOutputSchema + `}}`
	status, _, _ := cerebrasPost(t, server, "/v2?provider=cerebras", body, "cerebras-portable-native-output-source")
	if status != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("portable source output cap status=%d native_calls=%d", status, calls.Load())
	}
	response, err := server.Client().Get(server.URL + proxy.PublicCapabilitiesPath)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var public proxy.PublicCapabilityCatalog
	if err := json.NewDecoder(response.Body).Decode(&public); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, offering := range public.Offerings {
		if offering.Provider != "cerebras" || offering.Model != cerebrasQualifiedModel {
			continue
		}
		found = true
		outputOK, contextOK := false, false
		for _, control := range offering.Controls {
			if control.ID == "max_tokens" && control.Maximum != nil && *control.Maximum == 32768 {
				outputOK = true
			}
		}
		for _, limit := range offering.Limits {
			if limit.ID == "context_tokens" && limit.Value != nil && *limit.Value == 65536 {
				contextOK = true
			}
		}
		if !outputOK || !contextOK {
			t.Fatal("public Cerebras offering does not expose portable output 32768 and context 65536")
		}
	}
	if !found {
		t.Fatal("exact Cerebras offering absent from public discovery")
	}
}

type cerebrasPaidNativeOutputObserver struct {
	next     proxy.HTTPDoer
	key      string
	path     string
	calls    atomic.Int64
	status   int
	workflow *qwen27BWorkflowAllowance
}

func (observer *cerebrasPaidNativeOutputObserver) Do(request *http.Request) (*http.Response, error) {
	if observer.workflow == nil && observer.calls.Add(1) != 1 {
		return nil, errors.New("Cerebras paid-native-output qualification permits one native generation")
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	var payload struct {
		Model      string           `json:"model"`
		Maximum    int              `json:"max_completion_tokens"`
		OldMaximum json.RawMessage  `json:"max_tokens"`
		Effort     string           `json:"reasoning_effort"`
		Messages   []map[string]any `json:"messages"`
		Format     struct {
			Type   string `json:"type"`
			Schema struct {
				Name   string          `json:"name"`
				Strict bool            `json:"strict"`
				Schema json.RawMessage `json:"schema"`
			} `json:"json_schema"`
		} `json:"response_format"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return nil, errors.New("decode Cerebras request")
	}
	contractOK := len(body) <= 4096 && payload.Maximum == 40960 && payload.Effort == "medium" && payload.Format.Type == "json_schema" && payload.Format.Schema.Strict && jsonBodiesEqual(payload.Format.Schema.Schema, []byte(cerebrasPaidNativeOutputSchema)) && len(payload.Messages) == 1 && payload.Messages[0]["role"] == "user" && payload.Messages[0]["content"] == "Return a JSON object with ok set to READY."
	if observer.workflow != nil {
		contractOK = len(body) <= 4194304 && slices.Contains(observer.workflow.AllowedOutputs, payload.Maximum) && payload.Effort == observer.workflow.Effort && (payload.Format.Type == "" || payload.Format.Type == "json_schema" && payload.Format.Schema.Strict)
	}
	if !contractOK || request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/chat/completions") || payload.Model != "qwen-3.8-27b" || len(payload.OldMaximum) != 0 {
		return nil, errors.New("Cerebras paid-native-output native request differs from authorized contract")
	}
	attempt := 0
	if observer.workflow != nil {
		attempt, err = observer.workflow.reserve(payload.Maximum, body)
		if err != nil {
			return nil, err
		}
		observer.calls.Add(1)
	}
	response, err := observer.next.Do(request)
	if err != nil {
		if observer.workflow != nil {
			return nil, errors.Join(err, observer.workflow.unknown("transport_error"))
		}
		return nil, err
	}
	data, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(data))
	observer.status = response.StatusCode
	safe := strings.ReplaceAll(string(data), observer.key, "<redacted>")
	safe = strings.ReplaceAll(safe, request.URL.Host, "<provider-origin>")
	capture := observer.path
	if observer.workflow != nil {
		capture = fmt.Sprintf("%s.attempt-%02d.native.private.json", capture, attempt)
	}
	file, err := os.Create(capture)
	if err != nil {
		if observer.workflow != nil {
			return nil, errors.Join(err, observer.workflow.unknown("native_capture_failed"))
		}
		return nil, err
	}
	writeErr := json.NewEncoder(file).Encode(map[string]any{"native_http_status": response.StatusCode, "native_generations": observer.calls.Load(), "request_bytes": len(body), "model": payload.Model, "reasoning_effort": payload.Effort, "max_completion_tokens": payload.Maximum, "body": safe, "body_read_failed": readErr != nil})
	closeErr := file.Close()
	if err := errors.Join(readErr, writeErr, closeErr); err != nil {
		if observer.workflow != nil {
			return nil, errors.Join(err, observer.workflow.unknown("native_body_capture_incomplete"))
		}
		return nil, err
	}
	if observer.workflow != nil {
		if err := observer.workflow.observe(response.StatusCode, data, capture); err != nil {
			return nil, err
		}
	}
	return response, nil
}

func TestCerebrasPaidNativeOutputReleasedSDKQualification(t *testing.T) {
	mode := os.Getenv("LLM_PROXY_CEREBRAS_NativeOutput_MODE")
	if mode == "" {
		t.Skip("select local or explicitly authorized one-generation paid-native-output qualification")
	}
	if mode != "local" && mode != "live" {
		t.Fatal("invalid qualification mode")
	}
	catalogPath, requestPath, binary, directory := os.Getenv("LLM_PROXY_CEREBRAS_NativeOutput_CATALOG"), os.Getenv("LLM_PROXY_CEREBRAS_NativeOutput_REQUEST"), os.Getenv("LLM_PROXY_CEREBRAS_NativeOutput_BINARY"), os.Getenv("LLM_PROXY_CEREBRAS_NativeOutput_DIRECTORY")
	catalogBytes, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := proxy.ParseProviderCatalog(catalogBytes)
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
			{"accepted200", 200, `{"choices":[{"message":{"content":"{\"ok\":\"READY\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10,"completion_tokens_details":{"reasoning_tokens":1}}}`},
			{"rejected400", 400, `{"error":{"code":"invalid_request","message":"max_completion_tokens must be less than or equal to 40960; synthetic-cerebras-64k-key"}}`},
		}
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			key, secret := "synthetic-cerebras-64k-key", TestSecret
			var next proxy.HTTPDoer
			endpoint := ""
			if mode == "local" {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(fixture.status)
					io.WriteString(w, fixture.body)
				}))
				t.Cleanup(upstream.Close)
				next = upstream.Client()
				endpoint = upstream.URL
			} else {
				key = os.Getenv("CEREBRAS_API_KEY")
				if key == "" {
					t.Fatal("declared Proxy-owned CEREBRAS_API_KEY required")
				}
				raw := make([]byte, 32)
				if _, err := rand.Read(raw); err != nil {
					t.Fatal(err)
				}
				secret = fmt.Sprintf("%x", raw)
				client := &http.Client{Timeout: 1800 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
				next = client
				t.Cleanup(client.CloseIdleConnections)
			}
			capture := filepath.Join(directory, fixture.name+".native.private.json")
			observer := &cerebrasPaidNativeOutputObserver{next: next, key: key, path: capture}
			original := proxy.HTTPClient
			proxy.HTTPClient = observer
			t.Cleanup(func() { proxy.HTTPClient = original })
			tenant := proxy.StandardManagedTenantTestConfiguration(secret)
			tenant.ProviderKeys = map[string]string{"cerebras": key}
			tenant.ProviderTextModels = map[string]string{"cerebras": cerebrasQualifiedModel}
			tenant.Defaults = proxy.TenantDefaults{Provider: "cerebras", Model: cerebrasQualifiedModel}
			configuration := proxy.Configuration{ProviderCatalog: catalog, AssetStorePath: t.TempDir(), RequestTimeoutSeconds: 1800, MaxRequestTimeoutSeconds: 1800}
			if mode == "local" {
				configuration.Endpoints = providerEndpointOverrides(map[string]string{"cerebras": endpoint}, nil)
			}
			router, err := buildRouterWithManagedTenant(t, configuration, zap.NewNop().Sugar(), tenant)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 1900*time.Second)
			defer cancel()
			receipt := filepath.Join(directory, fixture.name+".sdk.private.json")
			command := exec.CommandContext(ctx, binary, "cerebras", server.URL, requestPath, receipt, fmt.Sprintf("cerebras-paid-native-output-%d", time.Now().UnixNano()))
			command.Env = []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + os.Getenv("TMPDIR"), "TELLTALE_EVALUATION_PROXY_SECRET=" + secret}
			output, runErr := command.CombinedOutput()
			if observer.calls.Load() != 1 {
				t.Fatalf("one native generation required, observed %d", observer.calls.Load())
			}
			nativeBytes, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(nativeBytes), key) {
				t.Fatal("private native capture exposed provider key")
			}
			t.Logf("released_sdk_status=%s native_http_status=%d native_generations=1 requested_max_completion_tokens=40960 effort=medium", strings.TrimSpace(string(output)), observer.status)
			if mode == "local" && ((fixture.status == 200 && runErr != nil) || (fixture.status == 400 && runErr == nil) || observer.status != fixture.status) {
				t.Fatal("local native acceptance/rejection observation failed")
			}
			if mode == "live" && runErr != nil {
				t.Fatalf("provider rejected requested40960 qualification; native response preserved privately: %v", runErr)
			}
			if observer.status == 200 && runErr == nil {
				receiptBytes, err := os.ReadFile(receipt)
				if err != nil {
					t.Fatal(err)
				}
				var result struct {
					Text    string `json:"visible_text"`
					Success bool   `json:"success"`
				}
				if json.Unmarshal(receiptBytes, &result) != nil || !result.Success || !jsonBodiesEqual([]byte(result.Text), []byte(`{"ok":"READY"}`)) {
					t.Fatal("accepted provider result failed tiny strict schema")
				}
			}
		})
	}
}
