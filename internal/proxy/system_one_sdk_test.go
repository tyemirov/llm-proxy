package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestSystemOneOfficialTypeScriptSDKConsumer(t *testing.T) {
	var calls atomic.Int32
	var retries atomic.Int32
	cancelStarted := make(chan struct{})
	cancelFinished := make(chan struct{})
	timeoutFinished := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/cancel-started" {
			select {
			case <-cancelStarted:
				writer.WriteHeader(http.StatusNoContent)
			case <-request.Context().Done():
			}
			return
		}
		call := calls.Add(1)
		if request.URL.Path != "/v1/systemone" || request.Header.Get("Authorization") != "Bearer sdk-provider-secret" {
			t.Errorf("SDK request route or provider credential changed")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if request.Header.Get("Content-Encoding") == "zstd" {
			decoder, decodeErr := zstd.NewReader(bytes.NewReader(body))
			if decodeErr != nil {
				t.Error(decodeErr)
				return
			}
			body, err = io.ReadAll(decoder)
			decoder.Close()
			if err != nil {
				t.Error(err)
				return
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-TypeSafe-Request-ID", "typesafe-sdk-request")
		if call > 2 {
			var document struct {
				State struct {
					Scenario string `json:"scenario"`
				} `json:"state"`
			}
			if err := json.Unmarshal(body, &document); err != nil {
				t.Error(err)
				return
			}
			switch document.State.Scenario {
			case "error":
				writer.Header().Set("Retry-After", "0")
				writer.WriteHeader(http.StatusTooManyRequests)
				io.WriteString(writer, `{"detail":{"message":"sdk-provider-secret rejected this fixture"}}`)
			case "retry":
				if retries.Add(1) == 1 {
					writer.Header().Set("Retry-After", "0")
					writer.WriteHeader(http.StatusTooManyRequests)
					io.WriteString(writer, `{"detail":"Retry this fixture"}`)
				} else {
					io.WriteString(writer, `{"model":"jev-1.14.0","answers":{"check":{"type":"noul","noul":1}},"usage":{"input_tokens":1,"output_tokens":1}}`)
				}
			case "cancel":
				close(cancelStarted)
				<-request.Context().Done()
				close(cancelFinished)
			case "timeout":
				<-request.Context().Done()
				close(timeoutFinished)
			default:
				t.Errorf("unknown SDK scenario=%s", document.State.Scenario)
			}
			return
		}
		if call == 1 {
			if request.Header.Get("Content-Encoding") != "zstd" {
				t.Error("SDK compression was not exercised")
			}
			var document map[string]json.RawMessage
			if err := json.Unmarshal(body, &document); err != nil {
				t.Error(err)
			}
			if string(document["model"]) != `"jev-latest"` {
				t.Errorf("SDK model=%s", document["model"])
			}
			io.WriteString(writer, `{"model":"jev-1.14.0","answers":{"routing":{"type":"choice","choice":"a","confidence":0.9,"probabilities":{"a":0.9,"b":0.1}},"severity":{"type":"score","score":0.25,"confidence":0.9,"legend":{"0":"Low","1":"High"},"probabilities":{"0":0.75,"1":0.25}},"accepted":{"type":"noul","noul":0.8}},"usage":{"input_tokens":400,"output_tokens":20}}`)
			return
		}
		mediaType, parameters, parseErr := mime.ParseMediaType(request.Header.Get("Content-Type"))
		if parseErr != nil || mediaType != "multipart/form-data" || request.Header.Get("TypeSafe-Danger-Zone-Images") != "true" {
			t.Errorf("SDK multipart or opt-in changed: %s", request.Header.Get("Content-Type"))
		}
		parts := multipart.NewReader(bytes.NewReader(body), parameters["boundary"])
		first, partErr := parts.NextPart()
		if partErr != nil || first.FormName() != "request" {
			t.Error("SDK JSON part is absent")
			return
		}
		asset, partErr := parts.NextPart()
		if partErr != nil || asset.FormName() == "" {
			t.Error("SDK image part is absent")
			return
		}
		io.WriteString(writer, `{"model":"jev-1.14.0","answers":{"region":{"type":"box","box":{"top":0,"bottom":1,"left":0,"right":1},"confidence":null}},"usage":{"input_tokens":null,"output_tokens":null}}`)
	}))
	defer upstream.Close()
	tenant := proxy.StandardManagedTenantTestConfiguration(TestSecret)
	tenant.ProviderKeys[proxy.ProviderNameTypeSafe] = "sdk-provider-secret"
	router, err := buildRouterWithManagedTenant(t, proxy.Configuration{Endpoints: providerEndpoints(upstream.URL, proxy.ProviderNameTypeSafe)}, zap.NewNop().Sugar(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	command := exec.Command("node", "--experimental-strip-types", filepath.Join("..", "..", "scripts", "system_one_sdk_acceptance.ts"), server.URL, TestSecret, upstream.URL)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("official TypeScript SDK failed: %v: %s", err, output)
	}
	if !strings.Contains(string(output), `"requests":7`) || calls.Load() != 7 || retries.Load() != 2 {
		t.Fatalf("SDK acceptance output=%s calls=%d", output, calls.Load())
	}
	for _, finished := range []chan struct{}{cancelFinished, timeoutFinished} {
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Fatal("SDK cancellation did not reach provider")
		}
	}
}
