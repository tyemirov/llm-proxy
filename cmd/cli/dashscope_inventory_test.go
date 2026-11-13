package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type inventoryLoopbackTransport struct {
	origin *url.URL
	base   http.RoundTripper
}

func TestDashScopeInventoryCommandRejectsIncompleteProviderEvidence(t *testing.T) {
	valid := inventoryPage(1, 1, []json.RawMessage{json.RawMessage(`{"model":"qwen"}`)})
	change := func(field string, value any) string {
		var document map[string]any
		_ = json.Unmarshal([]byte(valid), &document)
		output := document["output"].(map[string]any)
		output[field] = value
		encoded, _ := json.Marshal(document)
		return string(encoded)
	}
	for _, scenario := range []struct{ name, document, expected string }{
		{"malformed JSON", `{"secret":"inventory-private-fixture"`, "invalid JSON"},
		{"extra JSON", valid + `{}`, "trailing response data"},
		{"trailing garbage", valid + `secret`, "trailing response data"},
		{"provider failure", strings.Replace(valid, `"success":true`, `"success":false`, 1), "provider reported failure"},
		{"missing success", strings.Replace(valid, `"success":true`, `"unused":true`, 1), "provider reported failure"},
		{"error code", strings.Replace(valid, `"code":null`, `"code":"inventory-private-fixture"`, 1), "provider reported failure"},
		{"missing output", strings.Replace(valid, `"output":`, `"unused":`, 1), "missing page metadata"},
		{"missing request ID", strings.Replace(valid, `"request_id":`, `"unused":`, 1), "missing page metadata"},
		{"missing total", change("total", nil), "missing page metadata"},
		{"missing page number", change("page_no", nil), "missing page metadata"},
		{"missing page size", change("page_size", nil), "missing page metadata"},
		{"missing models", change("models", nil), "missing page metadata"},
		{"negative total", change("total", -1), "invalid page metadata"},
		{"excessive total", change("total", 20001), "invalid page metadata"},
		{"wrong page", change("page_no", 2), "invalid page metadata"},
		{"wrong page size", change("page_size", 19), "invalid page metadata"},
		{"short page", change("total", 2), "incomplete model count"},
		{"premature empty page", change("models", []any{}), "incomplete model count"},
		{"missing identity", change("models", []any{map[string]any{"secret": "inventory-private-fixture"}}), "invalid model identity"},
		{"invalid model", change("models", []any{"inventory-private-fixture"}), "invalid model identity"},
		{"duplicate identity", inventoryPage(1, 2, []json.RawMessage{json.RawMessage(`{"model":"same"}`), json.RawMessage(`{"model":"same"}`)}), "duplicate model identity"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			run := inventoryCLI(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, scenario.document) })
			output, diagnostics, err := run(context.Background())
			if err == nil || !strings.Contains(err.Error(), scenario.expected) {
				t.Fatalf("error=%v want %s", err, scenario.expected)
			}
			if output != "" || strings.Contains(diagnostics+err.Error(), "inventory-private-fixture") {
				t.Fatalf("unsafe failed export output=%q diagnostics=%s error=%v", output, diagnostics, err)
			}
		})
	}
}

func TestDashScopeInventoryCommandRejectsChangedTotalAndSecondCollectionFailure(t *testing.T) {
	for _, scenario := range []string{"changed total", "second collection"} {
		t.Run(scenario, func(t *testing.T) {
			run := inventoryCLI(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("supports") == "deploy" {
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprint(w, "inventory-private-fixture")
					return
				}
				page, total, count := 1, 21, 20
				if scenario == "second collection" {
					total, count = 0, 0
				}
				if r.URL.Query().Get("page_no") == "2" {
					page, total, count = 2, 22, 2
				}
				models := []json.RawMessage{}
				for index := 0; index < count; index++ {
					models = append(models, json.RawMessage(fmt.Sprintf(`{"model":"qwen-%d-%d"}`, page, index)))
				}
				fmt.Fprint(w, inventoryPage(page, total, models))
			})
			output, _, err := run(context.Background())
			if err == nil || output != "" {
				t.Fatalf("failed collection published output=%q error=%v", output, err)
			}
			if scenario == "changed total" && !strings.Contains(err.Error(), "total changed") {
				t.Fatal(err)
			}
			if scenario == "second collection" && !strings.Contains(err.Error(), "deploy inventory page 1: HTTP 401") {
				t.Fatal(err)
			}
		})
	}
}

func TestDashScopeInventoryCommandHandlesEmptySingaporeInventory(t *testing.T) {
	run := inventoryCLI(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, inventoryPage(1, 0, []json.RawMessage{})) })
	t.Setenv("DASHSCOPE_BASE_URL", "https://workspace.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1")
	originalClock := dashScopeInventoryNow
	dashScopeInventoryNow = func() time.Time { return time.Date(2026, 11, 13, 12, 30, 0, 0, time.FixedZone("fixture", 3600)) }
	t.Cleanup(func() { dashScopeInventoryNow = originalClock })
	output, _, err := run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `"access_region":"ap-southeast-1"`) || !strings.Contains(output, `"retrieved_at":"2026-11-13T11:30:00Z"`) || strings.Count(output, `"models":[]`) != 2 {
		t.Fatalf("export=%s", output)
	}
}

func TestDashScopeInventoryCommandRejectsUnsafeInputsBeforeHTTP(t *testing.T) {
	for _, region := range []string{"ap-southeast-1", "us-east-1"} {
		t.Run("reserved trial endpoint "+region, func(t *testing.T) {
			run := inventoryCLI(t, func(w http.ResponseWriter, r *http.Request) { t.Error("trial endpoint received workspace credential") })
			t.Setenv("DASHSCOPE_BASE_URL", "https://trial."+region+".maas.aliyuncs.com/compatible-mode/v1")
			output, diagnostics, err := run(context.Background())
			if err == nil || !strings.Contains(err.Error(), "DASHSCOPE_BASE_URL") || output != "" || strings.Contains(diagnostics+err.Error(), "inventory-private-fixture") {
				t.Fatalf("output=%q diagnostics=%s error=%v", output, diagnostics, err)
			}
		})
	}
	for _, input := range []string{"", "http://workspace.us-east-1.maas.aliyuncs.com/compatible-mode/v1", "https://user:inventory-private-fixture@workspace.us-east-1.maas.aliyuncs.com/compatible-mode/v1", "https://workspace.us-east-1.maas.aliyuncs.com:443/compatible-mode/v1", "https://workspace.us-east-1.maas.aliyuncs.com/compatible-mode/v1?secret=inventory-private-fixture", "https://workspace.us-east-1.maas.aliyuncs.com/compatible-mode/v1#fragment", "https://workspace.us-east-1.maas.aliyuncs.com/compatible-mode/v1#", "https://workspace.us-east-1.maas.aliyuncs.com/compatible-mode/v1?", "https://workspace.us-east-1.maas.aliyuncs.com/compatible-mode/v1/", "https://workspace.us-east-1.maas.aliyuncs.com/compatible-mode/%76%31", "https://workspace.us-east-1.maas.aliyuncs.com.evil.test/compatible-mode/v1", "https://workspace.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", ":invalid", "https://%zz/compatible-mode/v1"} {
		t.Run(input, func(t *testing.T) {
			run := inventoryCLI(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unsafe configuration dispatched HTTP") })
			t.Setenv("DASHSCOPE_BASE_URL", input)
			output, diagnostics, err := run(context.Background())
			if err == nil || !strings.Contains(err.Error(), "DASHSCOPE_BASE_URL") || output != "" || strings.Contains(diagnostics+err.Error(), "inventory-private-fixture") {
				t.Fatalf("output=%q diagnostics=%s error=%v", output, diagnostics, err)
			}
		})
	}
	t.Run("missing key", func(t *testing.T) {
		run := inventoryCLI(t, func(w http.ResponseWriter, r *http.Request) { t.Error("missing key dispatched HTTP") })
		t.Setenv("DASHSCOPE_API_KEY", "")
		output, _, err := run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "DASHSCOPE_API_KEY") || output != "" {
			t.Fatalf("output=%q error=%v", output, err)
		}
	})
}

func TestDashScopeInventoryCommandRejectsRedirectsAndCancellation(t *testing.T) {
	for _, target := range []string{"https://workspace.us-east-1.maas.aliyuncs.com/api/v1/other", "https://external.invalid/collect"} {
		t.Run(target, func(t *testing.T) {
			requests := 0
			run := inventoryCLI(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				http.Redirect(w, r, target, http.StatusFound)
			})
			output, _, err := run(context.Background())
			if err == nil || !strings.Contains(err.Error(), "HTTP 302") || requests != 1 || output != "" {
				t.Fatalf("requests=%d output=%q error=%v", requests, output, err)
			}
		})
	}
	t.Run("cancellation", func(t *testing.T) {
		run := inventoryCLI(t, func(w http.ResponseWriter, r *http.Request) { t.Error("cancelled command dispatched HTTP") })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		output, _, err := run(ctx)
		if err == nil || !strings.Contains(err.Error(), "transport failed") || output != "" {
			t.Fatalf("output=%q error=%v", output, err)
		}
	})
}

type inventoryRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip inventoryRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

type inventoryReadFailure struct{}

func (inventoryReadFailure) Read([]byte) (int, error) {
	return 0, errors.New("inventory-private-fixture")
}
func (inventoryReadFailure) Close() error { return nil }

type inventoryWriteFailure struct{}

func (inventoryWriteFailure) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestDashScopeInventoryCommandHandlesBoundaryFailures(t *testing.T) {
	for _, scenario := range []string{"transport", "read", "oversize page", "oversize export", "writer"} {
		t.Run(scenario, func(t *testing.T) {
			run := inventoryCLI(t, func(w http.ResponseWriter, r *http.Request) {
				if scenario == "oversize page" {
					fmt.Fprint(w, strings.Repeat("x", (2<<20)+1))
					return
				}
				if scenario == "oversize export" {
					page := 1
					fmt.Sscan(r.URL.Query().Get("page_no"), &page)
					models := []json.RawMessage{}
					for index := 0; index < 20; index++ {
						models = append(models, json.RawMessage(fmt.Sprintf(`{"model":"qwen-%d-%d","metadata":"%s"}`, page, index, strings.Repeat("x", 100000))))
					}
					fmt.Fprint(w, inventoryPage(page, 340, models))
					return
				}
				fmt.Fprint(w, inventoryPage(1, 0, []json.RawMessage{}))
			})
			if scenario == "transport" {
				http.DefaultTransport = inventoryRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("inventory-private-fixture") })
			}
			if scenario == "read" {
				http.DefaultTransport = inventoryRoundTripFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: inventoryReadFailure{}, Header: http.Header{}}, nil
				})
			}
			if scenario == "writer" {
				rootCmd.SetOut(inventoryWriteFailure{})
				rootCmd.SetErr(&bytes.Buffer{})
				defer func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil) }()
				err := executeRootCommand(t, "inventory-dashscope")
				if err == nil || !strings.Contains(err.Error(), "write DashScope inventory export") {
					t.Fatalf("error=%v", err)
				}
				return
			}
			output, diagnostics, err := run(context.Background())
			if err == nil || output != "" || strings.Contains(err.Error()+diagnostics, "inventory-private-fixture") {
				t.Fatalf("output=%q diagnostics=%s error=%v", output, diagnostics, err)
			}
		})
	}
}

func (transport inventoryLoopbackTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.URL.Scheme, copy.URL.Host = transport.origin.Scheme, transport.origin.Host
	return transport.base.RoundTrip(copy)
}

func inventoryCLI(t *testing.T, handler http.HandlerFunc) func(context.Context) (string, string, error) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	origin, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	originalTransport := http.DefaultTransport
	http.DefaultTransport = inventoryLoopbackTransport{origin: origin, base: originalTransport}
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	t.Setenv("DASHSCOPE_API_KEY", "inventory-private-fixture")
	t.Setenv("DASHSCOPE_BASE_URL", "https://workspace.us-east-1.maas.aliyuncs.com/compatible-mode/v1")
	return func(ctx context.Context) (string, string, error) {
		var output, diagnostics bytes.Buffer
		rootCmd.SetOut(&output)
		rootCmd.SetErr(&diagnostics)
		rootCmd.SetContext(ctx)
		command, _, findError := rootCmd.Find([]string{"inventory-dashscope"})
		if findError != nil {
			t.Fatal(findError)
		}
		command.SetContext(ctx)
		defer func() {
			rootCmd.SetOut(nil)
			rootCmd.SetErr(nil)
			rootCmd.SetContext(context.Background())
			command.SetContext(context.Background())
		}()
		err := executeRootCommand(t, "inventory-dashscope")
		return output.String(), diagnostics.String(), err
	}
}

func inventoryPage(page, total int, models []json.RawMessage) string {
	encoded, _ := json.Marshal(map[string]any{"success": true, "code": nil, "request_id": fmt.Sprintf("request-%d", page), "output": map[string]any{"page_no": page, "page_size": 20, "total": total, "models": models}})
	return string(encoded)
}

func TestDashScopeInventoryCommandCollectsEveryPageAndUseCase(t *testing.T) {
	requests := 0
	run := inventoryCLI(t, func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/models" || request.Header.Get("Authorization") != "Bearer inventory-private-fixture" {
			t.Errorf("unexpected request boundary: %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if len(query) != 3 || query.Get("page_size") != "20" {
			t.Errorf("query=%v", query)
		}
		page := 1
		if query.Get("page_no") == "2" {
			page = 2
		}
		models := []json.RawMessage{}
		total := 21
		if query.Get("supports") == "deploy" {
			total = 1
		}
		for index := (page - 1) * 20; index < min(page*20, total); index++ {
			models = append(models, json.RawMessage(fmt.Sprintf(`{"model":"qwen-%d","prices":[{"price":"0.030"}],"future_metadata":{"flag":true}}`, index)))
		}
		fmt.Fprint(writer, inventoryPage(page, total, models))
	})
	output, _, err := run(context.Background())
	if err != nil {
		t.Fatalf("inventory command: %v", err)
	}
	if requests != 3 {
		t.Fatalf("requests=%d want 3", requests)
	}
	var report struct {
		SourceHost            string `json:"source_host"`
		AccessRegion          string `json:"access_region"`
		WorkspaceServiceScope string `json:"workspace_service_scope"`
		Collections           []struct {
			Supports string            `json:"supports"`
			Total    int               `json:"total"`
			Models   []json.RawMessage `json:"models"`
		} `json:"collections"`
	}
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("report: %v output=%s", err, output)
	}
	if report.SourceHost != "workspace.us-east-1.maas.aliyuncs.com" || report.AccessRegion != "us-east-1" || report.WorkspaceServiceScope != "unknown" || len(report.Collections) != 2 || len(report.Collections[0].Models) != 21 || len(report.Collections[1].Models) != 1 {
		t.Fatalf("incomplete report: %+v", report)
	}
	if !strings.Contains(output, `"0.030"`) || !strings.Contains(output, `"future_metadata"`) {
		t.Fatalf("metadata changed: %s", output)
	}
}
