package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

func TestQwenCurrentAlibaba27BActivation(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		reasoning, _ := payload["reasoning"].(map[string]any)
		if r.Method != http.MethodPost || r.URL.Path != "/responses" || payload["model"] != currentQwenSharedModel || reasoning["effort"] != "medium" || payload["text"] != nil {
			t.Errorf("exact Alibaba text transport changed: %v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`)
	}))
	defer upstream.Close()
	router, err := buildRouterWithCatalogs(t, proxy.Configuration{Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameDashScope: upstream.URL}, nil)}, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	response, err := server.Client().Get(server.URL + proxy.PublicCapabilitiesPath)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var catalog proxy.PublicCapabilityCatalog
	if err := json.NewDecoder(response.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, offering := range catalog.Offerings {
		if offering.Provider == proxy.ProviderNameDashScope && offering.Model == currentQwenSharedModel {
			if !slices.Contains(offering.Capabilities, "text") || !slices.Contains(offering.ReasoningEfforts, "medium") {
				t.Fatalf("exact Alibaba offering lacks text or medium effort: %+v", offering)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("exact dashscope/qwen3.8-27b offering is absent from public HTTP discovery")
	}
	text, err := server.Client().Get(server.URL + "/?provider=dashscope&model=" + currentQwenSharedModel + "&reasoning_effort=medium&prompt=test&max_tokens=32&key=" + TestSecret)
	if err != nil {
		t.Fatal(err)
	}
	defer text.Body.Close()
	body, err := io.ReadAll(text.Body)
	if err != nil || text.StatusCode != http.StatusOK || string(body) != "OK" || calls.Load() != 1 || text.Header.Get("X-LLM-Proxy-Total-Tokens") != "5" {
		t.Fatalf("exact text dispatch status=%d calls=%d read_error=%v body=%s", text.StatusCode, calls.Load(), err, body)
	}
}

// Candidate coverage proves the existing transport with an explicitly injected
// policy hypothesis. Actual provider support requires separate live qualification.
func TestQwenCurrentAlibaba27BCandidateStructuredHTTP(t *testing.T) {
	catalog := qwen27BStrictCandidateCatalog(t)
	qwen27BStrictCandidateHTTP(t, catalog)
}

func qwen27BStrictCandidateCatalog(t *testing.T) *proxy.ProviderCatalog {
	t.Helper()
	schema := testfixtures.ProviderCatalog(t).Schema()
	for providerIndex := range schema.Providers {
		provider := &schema.Providers[providerIndex]
		if provider.ID != proxy.ProviderNameDashScope {
			continue
		}
		flashIndex := slices.IndexFunc(provider.Offerings, func(offering proxy.ProviderCatalogOffering) bool {
			return offering.Model == dashScopeFlashStructuredModel
		})
		candidateIndex := slices.IndexFunc(provider.Offerings, func(offering proxy.ProviderCatalogOffering) bool { return offering.Model == currentQwenSharedModel })
		if flashIndex < 0 || candidateIndex < 0 {
			t.Fatal("exact Flash and 27B offerings are required for the candidate policy")
		}
		provider.Offerings[candidateIndex].Enabled = proxy.ModelEnabled
		provider.Offerings[candidateIndex].StructuredOutput = provider.Offerings[flashIndex].StructuredOutput
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func qwen27BStrictCandidateHTTP(t *testing.T, catalog *proxy.ProviderCatalog) {
	t.Helper()
	for _, scenario := range []struct {
		name, status, output string
		wantStatus           int
	}{
		{"valid", "completed", `{"decision":"pass"}`, http.StatusOK},
		{"schema-invalid", "completed", `{"decision":"private-invalid"}`, http.StatusBadGateway},
		{"incomplete", "incomplete", `{"decision":"pass"}`, http.StatusBadGateway},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				text, _ := payload["text"].(map[string]any)
				format, _ := text["format"].(map[string]any)
				reasoning, _ := payload["reasoning"].(map[string]any)
				encodedSchema, _ := json.Marshal(format["schema"])
				if r.Method != http.MethodPost || r.URL.Path != "/responses" || payload["model"] != currentQwenSharedModel || payload["store"] != false || payload["max_output_tokens"] != float64(4096) || reasoning["effort"] != "medium" || format["type"] != "json_schema" || format["strict"] != true || !jsonBodiesEqual(encodedSchema, []byte(structuredDecisionSchema)) {
					t.Errorf("exact 27B strict-schema transport changed: %v", payload)
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"status": scenario.status, "incomplete_details": map[string]string{"reason": "max_output_tokens"}, "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": scenario.output}}}}, "usage": map[string]int{"input_tokens": 9, "output_tokens": 5, "total_tokens": 14}})
			}))
			defer upstream.Close()
			router, err := buildRouterWithCatalogs(t, proxy.Configuration{ProviderCatalog: catalog, AssetStorePath: t.TempDir(), AssetRetentionSeconds: 3600, Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameDashScope: upstream.URL}, nil)}, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			body := strings.Replace(dashScopeStructuredBody(t, currentQwenSharedModel, nil), `"reasoning_effort":"none"`, `"reasoning_effort":"medium"`, 1)
			request, err := http.NewRequest(http.MethodPost, server.URL+"/v2?provider=dashscope&key="+TestSecret, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "qwen27b:candidate:"+scenario.name)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			output, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != scenario.wantStatus || calls.Load() != 1 || strings.Contains(string(output), "private-invalid") {
				t.Fatalf("status=%d calls=%d body=%s", response.StatusCode, calls.Load(), output)
			}
			if scenario.wantStatus == http.StatusOK && (!jsonBodiesEqual(output, []byte(scenario.output)) || response.Header.Get("X-LLM-Proxy-Total-Tokens") != "14") {
				t.Fatalf("valid output or usage changed: body=%s headers=%v", output, response.Header)
			}
		})
	}
}

// TestQwenCurrentAlibaba27BLiveStrictProbe performs one explicitly authorized
// provider request through a real Proxy and an externally built released SDK.
func TestQwenCurrentAlibaba27BLiveStrictProbe(t *testing.T) {
	if os.Getenv("LLM_PROXY_LIVE_QWEN27B_STRICT") != "true" {
		t.Skip("set LLM_PROXY_LIVE_QWEN27B_STRICT=true for the paid one-request probe")
	}
	key, baseURL, binary := os.Getenv("DASHSCOPE_API_KEY"), os.Getenv("DASHSCOPE_BASE_URL"), os.Getenv("LLM_PROXY_QWEN27B_PROBE_BINARY")
	if key == "" || baseURL == "" || binary == "" {
		t.Fatal("declared DashScope key/base URL and released SDK probe binary are required")
	}
	original := proxy.HTTPClient
	observer := &qwen27BProbeHTTPDoer{next: original, t: t, key: key}
	const constWire = `{"input":[{"role":"user","content":"Return exactly {\"decision\":\"PROMPT_WRONG\"}."}],"model":"qwen3.8-27b","store":false,"max_output_tokens":1024,"reasoning":{"effort":"none"},"text":{"format":{"name":"llm_proxy_structured_output","schema":{"type":"object","properties":{"decision":{"type":"string","const":"SCHEMA_OK"}},"required":["decision"],"additionalProperties":false},"strict":true,"type":"json_schema"}}}`
	observer.expectedWire = []byte(constWire)
	if os.Getenv("LLM_PROXY_QWEN27B_PROBE_CONSTRAINT") == "enum" {
		observer.expectedWire = []byte(strings.Replace(constWire, `"const":"SCHEMA_OK"`, `"enum":["SCHEMA_OK"]`, 1))
	}
	proxy.HTTPClient = observer
	t.Cleanup(func() { proxy.HTTPClient = original })
	tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenant.ProviderKeys = map[string]string{proxy.ProviderNameDashScope: key}
	tenant.ProviderTextModels = map[string]string{proxy.ProviderNameDashScope: currentQwenSharedModel}
	tenant.Defaults = proxy.TenantDefaults{Provider: proxy.ProviderNameDashScope, Model: currentQwenSharedModel}
	router, err := buildRouterWithManagedTenant(t, proxy.Configuration{ProviderCatalog: qwen27BStrictCandidateCatalog(t), AssetStorePath: t.TempDir(), AssetRetentionSeconds: 3600, Endpoints: providerEndpointOverrides(map[string]string{proxy.ProviderNameDashScope: baseURL}, nil)}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, server.URL)
	command.Stdin = strings.NewReader(TestSecret)
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + os.Getenv("TMPDIR")}
	output, runError := command.CombinedOutput()
	t.Logf("released_sdk_probe=%s", output)
	if observer.calls.Load() != 1 {
		t.Fatalf("expected exactly one provider request, observed %d", observer.calls.Load())
	}
	if runError != nil {
		t.Fatalf("released SDK strict probe failed: %v", runError)
	}
}

type qwen27BProbeHTTPDoer struct {
	next               proxy.HTTPDoer
	t                  *testing.T
	key                string
	calls              atomic.Int64
	usage              json.RawMessage
	classification     string
	completion         string
	incompleteReason   string
	expectedWire       []byte
	maxTokens          int
	effort             string
	bodyLimit          int
	privateCapturePath string
	workflow           *qwen27BWorkflowAllowance
}

func (observer *qwen27BProbeHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	if observer.workflow == nil && observer.calls.Add(1) != 1 {
		return nil, errors.New("qwen27b probe permits exactly one provider request")
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	text, _ := payload["text"].(map[string]any)
	format, _ := text["format"].(map[string]any)
	reasoning, _ := payload["reasoning"].(map[string]any)
	maxTokens, effort, bodyLimit := observer.maxTokens, observer.effort, observer.bodyLimit
	if maxTokens == 0 {
		maxTokens = 1024
	}
	if effort == "" {
		effort = "none"
	}
	if bodyLimit == 0 {
		bodyLimit = 2048
	}
	structuredOK := format["type"] == "json_schema" && format["strict"] == true
	if observer.workflow != nil {
		requested, ok := payload["max_output_tokens"].(float64)
		if !ok || requested != float64(int(requested)) {
			return nil, errors.New("workflow output cap is invalid")
		}
		maxTokens = int(requested)
		if !slices.Contains(observer.workflow.AllowedOutputs, maxTokens) {
			return nil, errors.New("workflow output cap differs from the frozen budgets")
		}
		structuredOK = payload["text"] == nil || structuredOK
	}
	if len(body) > bodyLimit || request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/responses") || payload["model"] != currentQwenSharedModel || payload["max_output_tokens"] != float64(maxTokens) || reasoning["effort"] != effort || !structuredOK || observer.workflow != nil && (effort != observer.workflow.Effort || payload["store"] != false) {
		return nil, errors.New("qwen27b probe provider request violates the fixed strict-schema contract")
	}
	if observer.expectedWire != nil && !jsonBodiesEqual(body, observer.expectedWire) {
		return nil, errors.New("qwen27b probe differs from the exact authorized wire request")
	}
	attempt := 0
	if observer.workflow != nil {
		attempt, err = observer.workflow.reserve(maxTokens, body)
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
	observer.t.Logf("native_provider_status=%d provider_requests=%d serialized_request_bytes=%d model=%s effort=%s max_output_tokens=%d format=%v strict=%v", response.StatusCode, observer.calls.Load(), len(body), currentQwenSharedModel, effort, maxTokens, format["type"], format["strict"])
	{
		responseBody, readError := io.ReadAll(response.Body)
		response.Body.Close()
		response.Body = io.NopCloser(bytes.NewReader(responseBody))
		if readError != nil {
			if observer.workflow != nil {
				return nil, errors.Join(readError, observer.workflow.unknown("native_body_read_error"))
			}
			return nil, readError
		}
		if observer.workflow != nil {
			safeBody := strings.ReplaceAll(string(responseBody), observer.key, "<redacted>")
			safeBody = strings.ReplaceAll(safeBody, request.URL.Host, "<workspace>")
			capture := fmt.Sprintf("%s.attempt-%02d.raw.private.json", observer.privateCapturePath, attempt)
			if err := qwen27BSavePrivateJSON(capture, map[string]any{"native_status": response.StatusCode, "body": safeBody}); err != nil {
				return nil, errors.Join(err, observer.workflow.unknown("native_capture_failed"))
			}
			if err := observer.workflow.observe(response.StatusCode, responseBody, capture); err != nil {
				return nil, err
			}
		}
		var failure struct {
			Error struct {
				Code    string `json:"code"`
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
			Usage             json.RawMessage `json:"usage"`
			Status            string          `json:"status"`
			IncompleteDetails struct {
				Reason string `json:"reason"`
			} `json:"incomplete_details"`
			Output []struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		}
		if err := json.Unmarshal(responseBody, &failure); err != nil {
			observer.t.Log("native_error_json=unavailable")
		} else {
			observer.usage = append(json.RawMessage(nil), failure.Usage...)
			observer.completion = failure.Status
			observer.incompleteReason = failure.IncompleteDetails.Reason
			sanitize := func(value string) string {
				value = strings.ReplaceAll(value, observer.key, "<redacted>")
				value = strings.ReplaceAll(value, request.URL.Host, "<workspace>")
				if len(value) > 2048 {
					value = value[:2048]
				}
				return value
			}
			var topLevel map[string]json.RawMessage
			json.Unmarshal(responseBody, &topLevel)
			_, errorPresent := topLevel["error"]
			_, usagePresent := topLevel["usage"]
			var counts map[string]any
			usageValid := json.Unmarshal(failure.Usage, &counts) == nil && counts != nil
			safeCounts := map[string]any{}
			for _, name := range []string{"input_tokens", "output_tokens", "total_tokens"} {
				count, known := counts[name].(float64)
				if !known || count < 0 || count != float64(int64(count)) {
					usageValid = false
				} else {
					safeCounts[name] = count
				}
			}
			if usageValid && safeCounts["input_tokens"].(float64)+safeCounts["output_tokens"].(float64) != safeCounts["total_tokens"].(float64) {
				usageValid = false
			}
			encodedCounts, _ := json.Marshal(safeCounts)
			observer.t.Logf("native_error_present=%t native_error_code=%q native_error_type=%q native_error_message=%q native_usage_present=%t native_usage_valid=%t native_usage=%s", errorPresent, sanitize(failure.Error.Code), sanitize(failure.Error.Type), sanitize(failure.Error.Message), usagePresent, usageValid, encodedCounts)
			var visible strings.Builder
			messages, outputTextParts := 0, 0
			shape := make([]map[string]any, 0, len(failure.Output))
			allowlisted := func(value string, values ...string) string {
				if value == "" || slices.Contains(values, value) {
					return value
				}
				return "other"
			}
			for _, item := range failure.Output {
				contentTypes := make([]string, 0, len(item.Content))
				for _, content := range item.Content {
					contentTypes = append(contentTypes, allowlisted(content.Type, "output_text", "refusal", "summary_text"))
				}
				shape = append(shape, map[string]any{"type": allowlisted(item.Type, "message", "reasoning"), "role": allowlisted(item.Role, "assistant", "user", "system", "developer", "tool"), "content_types": contentTypes})
				if item.Type == "message" {
					messages++
					for _, content := range item.Content {
						if content.Type == "output_text" {
							outputTextParts++
							visible.WriteString(content.Text)
						}
					}
				}
			}
			var result map[string]any
			classification := "not_json"
			if json.Unmarshal([]byte(visible.String()), &result) == nil {
				classification = "other_json"
				if len(result) == 1 && result["decision"] == "SCHEMA_OK" {
					classification = "SCHEMA_OK"
				} else if len(result) == 1 && result["decision"] == "PROMPT_WRONG" {
					classification = "PROMPT_WRONG"
				}
			}
			observer.classification = classification
			encodedShape, _ := json.Marshal(shape)
			observer.t.Logf("native_output_shape=%s", encodedShape)
			observer.t.Logf("native_completion_status=%q incomplete_reason=%q output_items=%d message_items=%d output_text_parts=%d visible_bytes=%d visible_result_classification=%s", sanitize(failure.Status), sanitize(failure.IncompleteDetails.Reason), len(failure.Output), messages, outputTextParts, visible.Len(), classification)
			if observer.privateCapturePath != "" {
				safeVisible := strings.ReplaceAll(visible.String(), observer.key, "<redacted>")
				safeVisible = strings.ReplaceAll(safeVisible, request.URL.Host, "<workspace>")
				capture := observer.privateCapturePath
				if observer.workflow != nil {
					capture = fmt.Sprintf("%s.attempt-%02d.visible.private.json", capture, attempt)
				}
				qwen27BWritePrivateJSON(observer.t, capture, map[string]any{"native_status": response.StatusCode, "completion": failure.Status, "incomplete_reason": failure.IncompleteDetails.Reason, "output_shape": shape, "visible_text": safeVisible, "usage": json.RawMessage(observer.usage), "error_code": sanitize(failure.Error.Code), "error_type": sanitize(failure.Error.Type), "error_message": sanitize(failure.Error.Message)})
			}
		}
	}
	return response, nil
}

func TestQwenCurrentAlibaba27BProbeObserver(t *testing.T) {
	for _, scenario := range []struct {
		name                                     string
		httpStatus                               int
		status, decision, reason, classification string
		usage                                    bool
	}{
		{"completed-correct", 200, "completed", "SCHEMA_OK", "", "SCHEMA_OK", true},
		{"completed-wrong", 200, "completed", "PROMPT_WRONG", "", "PROMPT_WRONG", true},
		{"incomplete", 200, "incomplete", "SCHEMA_OK", "max_output_tokens", "SCHEMA_OK", true},
		{"permission-error", 403, "", "", "", "not_json", false},
		{"missing-usage", 200, "completed", "SCHEMA_OK", "", "SCHEMA_OK", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			payload := map[string]any{"status": scenario.status}
			if scenario.decision != "" {
				visible, _ := json.Marshal(map[string]string{"decision": scenario.decision})
				payload["output"] = []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": string(visible)}}}}
			}
			if scenario.reason != "" {
				payload["incomplete_details"] = map[string]string{"reason": scenario.reason}
			}
			if scenario.httpStatus == 403 {
				payload["error"] = map[string]string{"code": "PERMISSION_DENIED", "type": "invalid_request_error", "message": "synthetic-observer-key Access denied by API-Key restrictions."}
			}
			if scenario.usage {
				payload["usage"] = map[string]int{"input_tokens": 30, "output_tokens": 8, "total_tokens": 38}
			}
			responseBody, _ := json.Marshal(payload)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(scenario.httpStatus)
				w.Write(responseBody)
			}))
			defer upstream.Close()
			observer := &qwen27BProbeHTTPDoer{next: upstream.Client(), t: t, key: "synthetic-observer-key"}
			request, err := http.NewRequest(http.MethodPost, upstream.URL+"/responses", strings.NewReader(`{"model":"qwen3.8-27b","max_output_tokens":1024,"reasoning":{"effort":"none"},"text":{"format":{"type":"json_schema","strict":true}}}`))
			if err != nil {
				t.Fatal(err)
			}
			response, err := observer.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || !bytes.Equal(body, responseBody) || response.StatusCode != scenario.httpStatus || observer.classification != scenario.classification || observer.completion != scenario.status || observer.incompleteReason != scenario.reason {
				t.Fatalf("observer did not preserve response or metadata: error=%v status=%d classification=%s completion=%s reason=%s", err, response.StatusCode, observer.classification, observer.completion, observer.incompleteReason)
			}
			if scenario.usage && !jsonBodiesEqual(observer.usage, []byte(`{"input_tokens":30,"output_tokens":8,"total_tokens":38}`)) || !scenario.usage && len(observer.usage) != 0 {
				t.Fatalf("observer changed usage availability: %s", observer.usage)
			}
		})
	}
}
