package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestStructuredOutputFailuresExposeOnlyBoundedPrivateClassification(t *testing.T) {
	const property = "private-property-do-not-log"
	const definition = "private-definition-do-not-log"
	const privateValue = "private-output-do-not-log"
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{property}, "properties": map[string]any{property: map[string]any{"$ref": "#/$defs/" + definition}}, "$defs": map[string]any{definition: map[string]any{"type": "string", "enum": []string{"allowed"}}}}
	for _, test := range []struct {
		name, output string
		codes        []string
	}{
		{"not-json", privateValue, []string{"structured_output.not_json"}},
		{"multiple-json", `{"` + property + `":"allowed"} {"` + privateValue + `":true}`, []string{"structured_output.multiple_json_values"}},
		{"schema", `{"` + property + `":"` + privateValue + `"}`, []string{"structured_output.schema_violation", "json_schema.enum"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"id": "diagnostic-response", "status": "completed", "output_text": test.output})
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()
			core, logs := observer.New(zap.DebugLevel)
			endpoints := proxy.NewEndpoints()
			endpoints.SetResponsesURL(upstream.URL)
			tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
			tenant.ID = "structured-diagnostics"
			tenant.Defaults.Model = proxy.ModelNameGPT55
			assetRoot := t.TempDir()
			router, err := buildRouterWithManagedTenant(t, proxy.Configuration{UpstreamCapacity: testfixtures.UpstreamCapacity(1, 2), RequestTimeoutSeconds: TestTimeout, Endpoints: endpoints, AssetStorePath: assetRoot, AssetRetentionSeconds: 3600}, zap.New(core).Sugar(), tenant)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			payload, _ := json.Marshal(map[string]any{"model": proxy.ModelNameGPT55, "messages": []map[string]string{{"role": "user", "content": "private-prompt-do-not-log"}}, "structured_output": map[string]any{"schema": schema}})
			request, err := http.NewRequest(http.MethodPost, server.URL+"/v2?key="+TestSecret, bytes.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "diagnostic:"+test.name)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusBadGateway || calls.Load() != 1 {
				t.Fatalf("status=%d calls=%d", response.StatusCode, calls.Load())
			}
			var envelope publicProviderErrorEnvelope
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Error.Code != llmproxycontract.ErrorCodeProviderError || envelope.Error.Retryable || envelope.Error.UpstreamStatus != nil {
				t.Fatalf("public failure changed: %s", body)
			}
			for i := 0; i < 2; i++ {
				observation, _ := http.NewRequest(http.MethodGet, server.URL+"/v2/requests?key="+TestSecret, nil)
				observation.Header.Set(llmproxycontract.HeaderIdempotencyKey, "diagnostic:"+test.name)
				observed, err := server.Client().Do(observation)
				if err != nil {
					t.Fatal(err)
				}
				observedBody, _ := io.ReadAll(observed.Body)
				observed.Body.Close()
				if observed.StatusCode != http.StatusBadGateway || observed.Header.Get(llmproxycontract.HeaderStructuredRequestState) != "failed" || calls.Load() != 1 {
					t.Fatalf("retained observation status=%d calls=%d", observed.StatusCode, calls.Load())
				}
				body = append(body, observedBody...)
			}
			entries := logs.FilterMessage("provider failure").All()
			if len(entries) != 1 {
				t.Fatalf("failure entries=%d", len(entries))
			}
			fields := entries[0].ContextMap()
			actualCodes, _ := json.Marshal(fields["provider_error_codes"])
			expectedCodes, _ := json.Marshal(test.codes)
			if !bytes.Equal(actualCodes, expectedCodes) {
				t.Fatalf("classification=%v want=%v", fields["provider_error_codes"], test.codes)
			}
			if fields["request_id"] != response.Header.Get(llmproxycontract.HeaderRequestID) || fields["retryable"] != false {
				t.Fatal("safe correlation metadata changed")
			}
			records, err := filepath.Glob(filepath.Join(assetRoot, "structured-requests", "*", "*.json"))
			if err != nil || len(records) != 1 {
				t.Fatalf("retained records=%v err=%v", records, err)
			}
			recordBytes, err := os.ReadFile(records[0])
			if err != nil {
				t.Fatal(err)
			}
			var record map[string]any
			if err := json.Unmarshal(recordBytes, &record); err != nil {
				t.Fatal(err)
			}
			if record["state"] != "failed" || record["result"] != nil || record["failure_code"] != llmproxycontract.ErrorCodeProviderError {
				t.Fatal("invalid output became an accepted result")
			}
			evidence := append(body, recordBytes...)
			for _, entry := range logs.All() {
				encoded, _ := json.Marshal(entry.ContextMap())
				evidence = append(evidence, []byte(entry.Message)...)
				evidence = append(evidence, encoded...)
			}
			for _, marker := range []string{property, definition, privateValue, "private-prompt-do-not-log"} {
				if bytes.Contains(evidence, []byte(marker)) {
					t.Fatalf("private marker leaked: %s", marker)
				}
			}
			if strings.Contains(string(body), "structured_output.") || strings.Contains(string(body), "json_schema.") {
				t.Fatal("internal classification escaped into public response")
			}
		})
	}
}
