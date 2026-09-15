package llmproxyclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderDiagnosticsClientUsesHTTP(t *testing.T) {
	valid := map[string]any{"provider": "dictator", "scope": "tenant", "operations_total": 6, "queued": 1, "running": 1, "succeeded": 1, "failed": 1, "cancelled": 1, "uncertain": 1}
	body, _ := json.Marshal(valid)
	statusCode := http.StatusOK
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/model/v1/provider-diagnostics/dictator" || request.URL.RawQuery != "" || request.Header.Get("Authorization") != "Bearer tenant-key" || request.Header.Get("Accept") != "application/json" {
			t.Errorf("incorrect diagnostic request contract")
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(statusCode)
		_, _ = writer.Write(body)
	}))
	defer server.Close()
	config, err := NewConfig(ConfigInput{BaseURL: server.URL + "/v2?key=obsolete", Secret: "tenant-key"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.GetProviderDiagnostics(context.Background(), "dictator")
	if err != nil || result.Scope != "tenant" || result.OperationsTotal != 6 || result.Succeeded != 1 {
		t.Fatalf("diagnostics=%+v error=%v", result, err)
	}
	before := requests
	for _, provider := range []string{"", "Dictator", " dictator", "dictator/other", "dictator?key=secret"} {
		if _, err := client.GetProviderDiagnostics(context.Background(), provider); !errors.Is(err, ErrInvalidClientRequest) {
			t.Fatalf("provider %q error=%v", provider, err)
		}
	}
	if requests != before {
		t.Fatal("invalid provider reached HTTP")
	}
	for field, value := range valid {
		delete(valid, field)
		body, _ = json.Marshal(valid)
		if _, err := client.GetProviderDiagnostics(context.Background(), "dictator"); !errors.Is(err, ErrClientHTTPFailure) {
			t.Fatalf("missing %s accepted", field)
		}
		valid[field] = nil
		body, _ = json.Marshal(valid)
		if _, err := client.GetProviderDiagnostics(context.Background(), "dictator"); !errors.Is(err, ErrClientHTTPFailure) {
			t.Fatalf("null %s accepted", field)
		}
		valid[field] = value
	}
	for _, invalid := range []struct {
		field string
		value any
	}{
		{"provider", "other"}, {"scope", "server"}, {"operations_total", -1}, {"queued", -1}, {"running", -1}, {"succeeded", -1}, {"failed", -1}, {"cancelled", -1}, {"uncertain", -1}, {"operations_total", 1.5}, {"operations_total", 7}, {"queued", 7},
	} {
		old := valid[invalid.field]
		valid[invalid.field] = invalid.value
		body, _ = json.Marshal(valid)
		if _, err := client.GetProviderDiagnostics(context.Background(), "dictator"); !errors.Is(err, ErrClientHTTPFailure) {
			t.Fatalf("invalid %s accepted", invalid.field)
		}
		valid[invalid.field] = old
	}
	delete(valid, "scope")
	valid["native_handle"] = "private"
	body, _ = json.Marshal(valid)
	if _, err := client.GetProviderDiagnostics(context.Background(), "dictator"); !errors.Is(err, ErrClientHTTPFailure) {
		t.Fatal("unknown field accepted")
	}
	body = []byte("invalid JSON")
	if _, err := client.GetProviderDiagnostics(context.Background(), "dictator"); !errors.Is(err, ErrClientHTTPFailure) {
		t.Fatal("invalid JSON accepted")
	}
	statusCode = http.StatusForbidden
	body = []byte("unknown client key")
	if _, err := client.GetProviderDiagnostics(context.Background(), "dictator"); !errors.Is(err, ErrClientHTTPFailure) {
		t.Fatal("HTTP failure accepted")
	}
}
