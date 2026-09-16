package main

import (
	"bytes"
	"encoding/json"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMediaCommandWorkflow(t *testing.T) {
	const assetID = "ast_0123456789abcdef0123456789abcdef"
	const operationID = "mop_0123456789abcdef0123456789abcdef"
	const asset = `{"asset_id":"` + assetID + `","mime_type":"audio/wav","size_bytes":3,"state":"available","created_at":"2026-09-15T00:00:00Z","expires_at":"2026-09-16T00:00:00Z"}`
	const operation = `{"operation_id":"` + operationID + `","capability":"audio.voice.extract","provider":"dictator","model":"dictator-speech-v1","catalog_revision":"fixture","state":"succeeded","cancellation_state":"not_requested","outputs":[],"cost":{"available":false,"reason":"unavailable"},"accepted_at":"2026-09-15T00:00:00Z","updated_at":"2026-09-15T00:00:00Z","deadline_at":"2026-09-16T00:00:00Z"}`
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tenant-key" {
			t.Error("missing tenant credential")
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/model/v1/assets":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(asset))
		case "/model/v1/assets/" + assetID:
			w.Write([]byte(asset))
		case "/model/v1/assets/" + assetID + "/content":
			w.Header().Set("Content-Type", "audio/wav")
			w.Write([]byte("wav"))
		case "/model/v1/capabilities":
			w.Write([]byte(`{"catalog_revision":"fixture","routes":[]}`))
		case "/model/v1/voices":
			w.Write([]byte(`{"voices":[]}`))
		default:
			if r.Method == http.MethodPost {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if r.Header.Get("Idempotency-Key") != "speech-intent" || body["controls"].(map[string]any)["duration_seconds"] != 0.5 {
					t.Fatal("submission intent changed")
				}
				w.WriteHeader(http.StatusAccepted)
			}
			w.Write([]byte(operation))
		}
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(file, []byte("wav"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		arguments   []string
		stdin, want string
	}{
		{[]string{"upload", "--file", file, "--mime-type", "audio/wav"}, "", assetID},
		{[]string{"submit", "--idempotency-key", "speech-intent"}, `{"capability":"audio.voice.extract","provider":"dictator","model":"dictator-speech-v1","input":{},"controls":{"duration_seconds":0.5}}`, operationID},
		{[]string{"status", "--operation-id", operationID}, "", operationID},
		{[]string{"wait", "--operation-id", operationID}, "", operationID},
		{[]string{"cancel", "--operation-id", operationID}, "", operationID},
		{[]string{"voices", "--provider", "dictator"}, "", "[]"},
		{[]string{"capabilities"}, "", "catalog_revision"},
		{[]string{"download", "--asset-id", assetID}, "", "wav"},
	} {
		t.Run(scenario.arguments[0], func(t *testing.T) {
			var out, stderr bytes.Buffer
			args := append([]string{"media", "--base-url", server.URL, "--secret", "tenant-key"}, scenario.arguments...)
			if code := run(args, strings.NewReader(scenario.stdin), &out, &stderr, defaultHTTPClientFactory); code != 0 {
				t.Fatalf("exit=%d: %s", code, stderr.String())
			}
			if !strings.Contains(out.String(), scenario.want) {
				t.Fatalf("output=%s", out.String())
			}
		})
	}
	if len(calls) != 9 {
		t.Fatalf("requests=%v", calls)
	}
}

func TestMediaCommandFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	for _, scenario := range []struct {
		name      string
		arguments []string
		stdin     io.Reader
		writer    io.Writer
		factory   httpClientFactory
	}{
		{"timeout", []string{"--timeout", "0s", "status"}, strings.NewReader(""), io.Discard, defaultHTTPClientFactory},
		{"config", []string{"--base-url", ":bad", "status"}, strings.NewReader(""), io.Discard, defaultHTTPClientFactory},
		{"client", []string{"status"}, strings.NewReader(""), io.Discard, func() llmproxyclient.HTTPDoer { return nil }},
		{"input", []string{"submit"}, failingReader{}, io.Discard, defaultHTTPClientFactory},
		{"trailing", []string{"submit"}, strings.NewReader(`{} {}`), io.Discard, defaultHTTPClientFactory},
		{"file", []string{"upload", "--file", filepath.Join(t.TempDir(), "absent")}, strings.NewReader(""), io.Discard, defaultHTTPClientFactory},
		{"download", []string{"download", "--asset-id", "ast_0123456789abcdef0123456789abcdef"}, strings.NewReader(""), io.Discard, defaultHTTPClientFactory},
		{"status", []string{"status", "--operation-id", "mop_fixture"}, strings.NewReader(""), io.Discard, defaultHTTPClientFactory},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var stderr bytes.Buffer
			arguments := append([]string{"media", "--base-url", server.URL, "--secret", "key"}, scenario.arguments...)
			if code := run(arguments, scenario.stdin, scenario.writer, &stderr, scenario.factory); code != 1 || stderr.Len() == 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr.String())
			}
		})
	}
	success := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"catalog_revision":"fixture","routes":[]}`))
	}))
	defer success.Close()
	t.Setenv(envNameBaseURL, success.URL)
	t.Setenv(envNameDefaultTenantKey, "key")
	var stderr bytes.Buffer
	if code := run([]string{"media", "capabilities"}, strings.NewReader(""), failingWriter{}, &stderr, defaultHTTPClientFactory); code != 1 || !strings.Contains(stderr.String(), "write media result") {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
}
