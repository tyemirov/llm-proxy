package proxy_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"gopkg.in/yaml.v3"
)

func TestFALImagesCatalogQueueAndOrderedArtifacts(t *testing.T) {
	for _, provider := range []string{"fal", "queue-fixture"} {
		t.Run(provider, func(t *testing.T) {
			images := streamingImageFixtures(t)
			modelPath, rootPath := "/reve/2.1/text-to-image", "/reve"
			if provider == "queue-fixture" {
				modelPath, rootPath = "/alternate/images", "/alternate"
			}
			var submissions atomic.Int32
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/files/") {
					if r.Header.Get("Authorization") != "" {
						t.Error("provider credential sent to artifact host")
					}
					index := 0
					if r.URL.Path == "/files/1.png" {
						index = 1
					}
					w.Header().Set("Content-Type", "image/png")
					_, _ = w.Write(images[index])
					return
				}
				if r.Header.Get("Authorization") != "Key image-provider-secret" {
					t.Errorf("wrong queue authentication: %s", r.Header.Get("Authorization"))
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /v1/models/pricing":
					_, _ = w.Write([]byte(`{"prices":[]}`))
				case "POST " + modelPath:
					submissions.Add(1)
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					expected := map[string]any{"prompt": "A lighthouse", "aspect_ratio": "1:1", "output_format": "png", "num_images": float64(2)}
					if !reflect.DeepEqual(body, expected) {
						t.Errorf("queue submission=%v", body)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "native-job-1", "status_url": upstream.URL + rootPath + "/requests/native-job-1/status", "response_url": upstream.URL + rootPath + "/requests/native-job-1", "cancel_url": upstream.URL + rootPath + "/requests/native-job-1/cancel"})
				case "GET " + rootPath + "/requests/native-job-1/status":
					_, _ = w.Write([]byte(`{"status":"COMPLETED","request_id":"native-job-1"}`))
				case "GET " + rootPath + "/requests/native-job-1":
					_ = json.NewEncoder(w).Encode(map[string]any{"images": []map[string]any{{"url": upstream.URL + "/files/0.png"}, {"url": upstream.URL + "/files/1.png"}}})
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			catalog := falImageTestCatalog(t, provider, upstream.URL)
			client := imageGenerationTestClient(t, upstream, catalog, provider)
			input := falImageTestInput(provider)
			contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
			if err != nil {
				t.Fatal(err)
			}
			requestBody, _ := json.Marshal(input)
			contractRequest := httptest.NewRequest(http.MethodPost, "/model/v1/operations", bytes.NewReader(requestBody))
			contractRequest.Header.Set("Content-Type", "application/json")
			contractRequest.Header.Set("Idempotency-Key", "queue-image")
			if err := contract.ValidateRequest("/model/v1/operations", http.MethodPost, contractRequest, requestBody); err != nil {
				t.Fatal(err)
			}
			accepted, err := client.CreateAspectRatioImageGeneration(t.Context(), "queue-image", llmproxyclient.AspectRatioImageGenerationInput{Provider: provider, Model: "reve-2.1", Prompt: "A lighthouse", AspectRatio: "1:1", OutputFormat: "png", OutputCount: 2})
			if err != nil {
				t.Fatal(err)
			}
			completed := waitForImageGeneration(t, client, accepted)
			if completed.State != proxy.MediaOperationStateSucceeded || len(completed.Outputs) != 2 {
				t.Fatalf("result=%+v", completed)
			}
			for index, output := range completed.Outputs {
				asset, err := client.GetAsset(t.Context(), output.AssetID)
				if err != nil {
					t.Fatal(err)
				}
				data, err := client.DownloadAsset(t.Context(), asset)
				if err != nil || !bytes.Equal(data, images[index]) {
					t.Fatalf("artifact %d: %v", index, err)
				}
			}
			duplicate, err := client.CreateMediaOperation(t.Context(), "queue-image", input)
			if err != nil || duplicate.OperationID != accepted.OperationID || submissions.Load() != 1 {
				t.Fatalf("duplicate=%+v err=%v submissions=%d", duplicate, err, submissions.Load())
			}
			data, _ := json.Marshal(completed)
			if err := contract.ValidateResponse("/model/v1/operations/{operation_id}", http.MethodGet, http.StatusOK, http.Header{"Content-Type": {"application/json"}}, data); err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte("native-job")) || bytes.Contains(data, []byte(upstream.URL)) {
				t.Fatalf("private handle leaked: %s", data)
			}
		})
	}
}

func falImageTestCatalog(t *testing.T, provider, origin string) *proxy.ProviderCatalog {
	t.Helper()
	data, err := os.ReadFile("../../configs/providers.yml")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, raw := range document["providers"].([]any) {
		entry := raw.(map[string]any)
		if entry["id"] != "fal" {
			continue
		}
		found = true
		entry["id"] = provider
		if provider == "queue-fixture" {
			entry["fields"].([]any)[0].(map[string]any)["id"] = "queue_token"
			entry["offerings"].([]any)[0].(map[string]any)["upstream_model"] = "alternate/images"
		}
		for _, rawTransport := range entry["transports"].([]any) {
			transport := rawTransport.(map[string]any)
			if provider == "queue-fixture" {
				transport["components"].(map[string]any)["authentication"].(map[string]any)["field"] = "queue_token"
			}
			transport["endpoint"].(map[string]any)["default_base_url"] = origin
			if transport["id"] == "image" {
				transport["artifact_origins"] = []string{origin}
			}
		}
	}
	if !found {
		t.Fatal("canonical catalog omits the FAL provider and Reve image offering")
	}
	encoded, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := proxy.ParseProviderCatalog(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func falImageTestInput(provider string) llmproxyclient.MediaOperationInput {
	return llmproxyclient.MediaOperationInput{Capability: "image.generate", Provider: provider, Model: "reve-2.1", Input: json.RawMessage(`{"prompt":"A lighthouse"}`), Controls: json.RawMessage(fmt.Sprintf(`{"aspect_ratio":%q,"output_format":"png","output_count":2}`, "1:1"))}
}

func TestFALImagesRejectsInvalidControlsBeforeSubmission(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models/pricing" {
			_, _ = w.Write([]byte(`{"prices":[]}`))
			return
		}
		calls.Add(1)
		w.WriteHeader(500)
	}))
	defer upstream.Close()
	client := imageGenerationTestClient(t, upstream, falImageTestCatalog(t, "fal", upstream.URL), "fal")
	for index, test := range []struct{ input, controls string }{
		{`{"prompt":" "}`, `{"aspect_ratio":"auto","output_format":"png","output_count":1}`},
		{`{"prompt":"x","image_asset_ids":["ast_private"]}`, `{"aspect_ratio":"auto","output_format":"png","output_count":1}`},
		{`{"prompt":"x"}`, `{"aspect_ratio":"5:1","output_format":"png","output_count":1}`},
		{`{"prompt":"x"}`, `{"aspect_ratio":"auto","output_format":"gif","output_count":1}`},
		{`{"prompt":"x"}`, `{"aspect_ratio":"auto","output_format":"png","output_count":5}`},
		{`{"prompt":"x"}`, `{"aspect_ratio":"auto","output_format":"png","output_count":0}`},
		{`{"prompt":"x"}`, `{"aspect_ratio":"auto","output_format":"png","output_count":1,"quality":"high"}`},
		{`{"prompt":"` + strings.Repeat("x", 4001) + `"}`, `{"aspect_ratio":"auto","output_format":"png","output_count":1}`},
	} {
		input := falImageTestInput("fal")
		input.Input = json.RawMessage(test.input)
		input.Controls = json.RawMessage(test.controls)
		if _, err := client.CreateMediaOperation(t.Context(), fmt.Sprintf("invalid-%d", index), input); httpFailureStatus(err) != http.StatusBadRequest {
			t.Fatalf("input %d: %v", index, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid controls reached provider")
	}
}

func TestFALImagesProviderBoundariesPreserveUncertainty(t *testing.T) {
	images := streamingImageFixtures(t)
	for _, scenario := range []string{"rejected", "lost-submit", "invalid-submit", "missing-handle", "foreign-handle", "relative-handle", "request-mismatch", "unknown-status", "failed", "status-unavailable", "result-unavailable", "result-invalid", "wrong-count", "foreign-artifact", "invalid-artifact-url", "artifact-unavailable", "artifact-invalid", "artifact-redirect", "status-redirect", "queued", "in-progress", "response-suffix", "missing-status-id", "submit-disconnected", "submit-truncated", "submit-oversized", "wrong-prefix", "status-disconnected", "result-disconnected", "artifact-disconnected", "artifact-truncated", "artifact-oversized", "image-truncated"} {
		t.Run(scenario, func(t *testing.T) {
			var submissions, polls, foreign atomic.Int32
			malicious := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreign.Add(1); w.WriteHeader(200) }))
			defer malicious.Close()
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/models/pricing":
					_, _ = w.Write([]byte(`{"prices":[]}`))
				case r.Method == http.MethodPost:
					submissions.Add(1)
					switch scenario {
					case "submit-disconnected":
						connection, _, _ := w.(http.Hijacker).Hijack()
						_ = connection.Close()
						return
					case "submit-truncated":
						w.Header().Set("Content-Length", "100")
						_, _ = w.Write([]byte(`{`))
						return
					case "submit-oversized":
						_, _ = w.Write([]byte(strings.Repeat(" ", (1<<20)+1)))
						return
					case "rejected":
						w.WriteHeader(422)
						return
					case "lost-submit":
						w.WriteHeader(503)
						return
					case "invalid-submit":
						_, _ = w.Write([]byte(`{`))
						return
					}
					id := "private-job"
					status := upstream.URL + "/reve/requests/" + id + "/status"
					result := strings.TrimSuffix(status, "/status")
					if scenario == "missing-handle" {
						id = ""
					}
					if scenario == "foreign-handle" {
						status = malicious.URL + "/reve/requests/private-job/status"
					}
					if scenario == "wrong-prefix" {
						status = upstream.URL + "/other/requests/private-job/status"
					}
					if scenario == "relative-handle" {
						status = "/reve/requests/private-job/status"
					}
					if scenario == "response-suffix" {
						result += "/response"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"request_id": id, "status_url": status, "response_url": result, "cancel_url": upstream.URL + "/reve/requests/private-job/cancel"})
				case strings.HasSuffix(r.URL.Path, "/status"):
					count := polls.Add(1)
					switch scenario {
					case "status-disconnected":
						connection, _, _ := w.(http.Hijacker).Hijack()
						_ = connection.Close()
						return
					case "status-unavailable":
						w.WriteHeader(503)
						return
					case "status-redirect":
						http.Redirect(w, r, malicious.URL, http.StatusTemporaryRedirect)
						return
					case "unknown-status":
						_, _ = w.Write([]byte(`{"status":"ALIEN","request_id":"private-job"}`))
						return
					case "request-mismatch":
						_, _ = w.Write([]byte(`{"status":"COMPLETED","request_id":"someone-else"}`))
						return
					case "failed":
						_, _ = w.Write([]byte(`{"status":"COMPLETED","request_id":"private-job","error":"upstream failure"}`))
						return
					case "queued", "in-progress":
						if count == 1 {
							state := "IN_QUEUE"
							if scenario == "in-progress" {
								state = "IN_PROGRESS"
							}
							_ = json.NewEncoder(w).Encode(map[string]string{"status": state, "request_id": "private-job"})
							return
						}
					}
					if scenario == "missing-status-id" {
						_, _ = w.Write([]byte(`{"status":"COMPLETED"}`))
						return
					}
					_, _ = w.Write([]byte(`{"status":"COMPLETED","request_id":"private-job"}`))
				case strings.HasPrefix(r.URL.Path, "/files/"):
					switch scenario {
					case "artifact-disconnected":
						connection, _, _ := w.(http.Hijacker).Hijack()
						_ = connection.Close()
						return
					case "artifact-truncated":
						w.Header().Set("Content-Length", "100")
						_, _ = w.Write([]byte("abc"))
						return
					case "image-truncated":
						_, _ = w.Write(images[0][:len(images[0])-20])
						return
					case "artifact-unavailable":
						w.WriteHeader(503)
						return
					case "artifact-invalid":
						_, _ = w.Write([]byte("not an image"))
						return
					case "artifact-redirect":
						http.Redirect(w, r, malicious.URL, http.StatusTemporaryRedirect)
						return
					}
					_, _ = w.Write(images[0])
				default:
					switch scenario {
					case "result-disconnected":
						connection, _, _ := w.(http.Hijacker).Hijack()
						_ = connection.Close()
						return
					case "result-unavailable":
						w.WriteHeader(503)
						return
					case "result-invalid":
						_, _ = w.Write([]byte(`{`))
						return
					case "wrong-count":
						_, _ = w.Write([]byte(`{"images":[]}`))
						return
					}
					target := upstream.URL + "/files/image.png"
					if scenario == "foreign-artifact" {
						target = malicious.URL + "/image.png"
					}
					if scenario == "invalid-artifact-url" {
						target = "%"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"images": []map[string]string{{"url": target}, {"url": target}}})
				}
			}))
			defer upstream.Close()
			catalog := falImageTestCatalog(t, "fal", upstream.URL)
			if scenario == "artifact-oversized" {
				schema := catalog.Schema()
				for i := range schema.Providers {
					if schema.Providers[i].ID == "fal" {
						value := 10
						schema.Providers[i].Offerings[0].Limits[1].Value = &value
					}
				}
				var err error
				catalog, err = proxy.NewProviderCatalog(schema)
				if err != nil {
					t.Fatal(err)
				}
			}
			client := imageGenerationTestClient(t, upstream, catalog, "fal")
			input := falImageTestInput("fal")
			accepted, err := client.CreateMediaOperation(t.Context(), "boundary", input)
			if err != nil {
				t.Fatal(err)
			}
			result := waitForImageGeneration(t, client, accepted)
			expected := proxy.MediaOperationStateUncertain
			if scenario == "rejected" || scenario == "failed" {
				expected = proxy.MediaOperationStateFailed
			}
			if scenario == "queued" || scenario == "in-progress" || scenario == "response-suffix" {
				expected = proxy.MediaOperationStateSucceeded
			}
			if result.State != expected {
				t.Fatalf("result=%+v expected=%s", result, expected)
			}
			if foreign.Load() != 0 || submissions.Load() != 1 {
				t.Fatalf("foreign=%d submissions=%d", foreign.Load(), submissions.Load())
			}
		})
	}
}

func TestFALImagesRecoversPersistedQueueWithoutResubmission(t *testing.T) {
	images := streamingImageFixtures(t)
	for _, changed := range []string{"none", "missing-handle", "wrong-binding", "detached"} {
		t.Run(changed, func(t *testing.T) {
			var submissions, polls atomic.Int32
			first, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/models/pricing":
					_, _ = w.Write([]byte(`{"prices":[]}`))
				case r.Method == http.MethodPost:
					submissions.Add(1)
					root := upstream.URL + "/reve/requests/recovery-job"
					_ = json.NewEncoder(w).Encode(map[string]string{"request_id": "recovery-job", "status_url": root + "/status", "response_url": root, "cancel_url": root + "/cancel"})
				case strings.HasSuffix(r.URL.Path, "/status"):
					if polls.Add(1) == 1 {
						close(first)
						select {
						case <-release:
						case <-r.Context().Done():
							return
						}
						w.WriteHeader(503)
						return
					}
					_, _ = w.Write([]byte(`{"status":"COMPLETED","request_id":"recovery-job"}`))
				case strings.HasPrefix(r.URL.Path, "/files/"):
					_, _ = w.Write(images[0])
				default:
					_ = json.NewEncoder(w).Encode(map[string]any{"images": []map[string]string{{"url": upstream.URL + "/files/image.png"}, {"url": upstream.URL + "/files/image.png"}}})
				}
			}))
			t.Cleanup(upstream.Close)
			client, database, restart := falImagesDurableClient(t, upstream)
			accepted, err := client.CreateMediaOperation(t.Context(), "recovery", falImageTestInput("fal"))
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-first:
			case <-time.After(3 * time.Second):
				t.Fatal("no status read after acceptance")
			}
			var retained struct{ ProviderHandle string }
			if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Take(&retained).Error; err != nil || !strings.Contains(retained.ProviderHandle, "recovery-job") {
				t.Fatalf("durable handle=%+v error=%v", retained, err)
			}
			if err := database.Exec("UPDATE media_operation_claim_records SET generation = generation + 1, expires_at = ? WHERE operation_id = ?", time.Now().Add(-time.Second), accepted.OperationID).Error; err != nil {
				t.Fatal(err)
			}
			switch changed {
			case "missing-handle":
				if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Update("provider_handle", "").Error; err != nil {
					t.Fatal(err)
				}
			case "wrong-binding":
				if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Update("execution_binding", "obsolete").Error; err != nil {
					t.Fatal(err)
				}
			case "detached":
				if err := database.Exec("DELETE FROM managed_tenant_connection_records WHERE provider_id = ?", "fal").Error; err != nil {
					t.Fatal(err)
				}
			}
			recovered := restart()
			result := waitForImageGeneration(t, recovered, accepted)
			unblock()
			expected := proxy.MediaOperationStateUncertain
			if changed == "none" {
				expected = proxy.MediaOperationStateSucceeded
			}
			if result.State != expected || submissions.Load() != 1 {
				t.Fatalf("recovery=%+v submissions=%d", result, submissions.Load())
			}
		})
	}
}

func falImagesDurableClient(t *testing.T, upstream *httptest.Server, changes ...func(*proxy.Configuration)) (llmproxyclient.Client, *gorm.DB, func() llmproxyclient.Client) {
	t.Helper()
	endpoints := proxy.NewEndpoints()
	endpoints.SetProviderBaseURL("openai", upstream.URL)
	base := proxy.Configuration{ProviderCatalog: falImageTestCatalog(t, "fal", upstream.URL), AssetStorePath: t.TempDir(), Endpoints: endpoints, Management: proxy.ManagementConfiguration{DatabasePath: filepath.Join(t.TempDir(), "queue.sqlite")}, MediaOperationWorkers: 1, MediaOperationClaimSeconds: 7200, MediaOperationClaimRenewalSeconds: 3600}
	for _, change := range changes {
		change(&base)
	}
	configuration, err := testfixtures.ProvisionManagedRouter(t, base, zap.NewNop().Sugar(), testfixtures.ManagedTenant{Secret: "queue-owner", Defaults: proxy.TenantDefaults{Provider: "openai", Model: proxy.ModelNameGPT41}, ProviderKeys: map[string]string{"openai": "queue-provider", "fal": "queue-provider"}})
	if err != nil {
		t.Fatal(err)
	}
	database, err := gorm.Open(configuration.Management.DatabaseDialector, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	restart := func() llmproxyclient.Client {
		router, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(router)
		t.Cleanup(server.Close)
		config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "queue-owner"})
		if err != nil {
			t.Fatal(err)
		}
		client, err := llmproxyclient.NewClient(config, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	return restart(), database, restart
}

func TestFALImagesCancellationAcknowledgementDoesNotConfirmOutcome(t *testing.T) {
	for _, response := range []string{"accepted", "already-completed", "invalid", "unavailable", "disconnected", "missing-handle"} {
		t.Run(response, func(t *testing.T) {
			var polls, cancels atomic.Int32
			done := make(chan struct{})
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/models/pricing":
					_, _ = w.Write([]byte(`{"prices":[]}`))
				case r.Method == http.MethodPost:
					root := upstream.URL + "/reve/requests/cancel-job"
					_ = json.NewEncoder(w).Encode(map[string]string{"request_id": "cancel-job", "status_url": root + "/status", "response_url": root, "cancel_url": root + "/cancel"})
				case r.Method == http.MethodPut:
					cancels.Add(1)
					switch response {
					case "disconnected":
						connection, _, _ := w.(http.Hijacker).Hijack()
						_ = connection.Close()
						return
					case "accepted":
						w.WriteHeader(202)
						_, _ = w.Write([]byte(`{"status":"CANCELLATION_REQUESTED"}`))
					case "already-completed":
						w.WriteHeader(400)
						_, _ = w.Write([]byte(`{"status":"ALREADY_COMPLETED"}`))
					case "invalid":
						_, _ = w.Write([]byte(`{`))
					case "unavailable":
						w.WriteHeader(503)
					}
				default:
					if polls.Add(1) == 1 {
						close(done)
					}
					_, _ = w.Write([]byte(`{"status":"IN_PROGRESS","request_id":"cancel-job"}`))
				}
			}))
			defer upstream.Close()
			client, database, _ := falImagesDurableClient(t, upstream)
			accepted, err := client.CreateMediaOperation(t.Context(), "cancel", falImageTestInput("fal"))
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("operation not polled")
			}
			if response == "missing-handle" {
				if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Update("provider_handle", "").Error; err != nil {
					t.Fatal(err)
				}
			}
			observed, err := client.CancelMediaOperation(t.Context(), accepted.OperationID)
			expected := proxy.MediaCancellationRequested
			if response == "already-completed" || response == "missing-handle" {
				expected = proxy.MediaCancellationUnsupported
			}
			expectedCalls := int32(1)
			if response == "missing-handle" {
				expectedCalls = 0
			}
			if err != nil || observed.State != proxy.MediaOperationStateRunning || observed.CancellationState != expected || cancels.Load() != expectedCalls {
				t.Fatalf("cancellation=%+v err=%v calls=%d", observed, err, cancels.Load())
			}
			// Fence the test worker before the provider server closes.
			if err := database.Exec("UPDATE media_operation_claim_records SET generation = generation + 1 WHERE operation_id = ?", accepted.OperationID).Error; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFALImagesCatalogRejectsUnsupportedCompositions(t *testing.T) {
	base := falImageTestCatalog(t, "fal", "http://127.0.0.1:8765")
	tests := map[string]func(*proxy.ProviderCatalogProvider){
		"missing origins": func(p *proxy.ProviderCatalogProvider) { p.Transports[1].ArtifactOrigins = nil },
		"duplicate origins": func(p *proxy.ProviderCatalogProvider) {
			p.Transports[1].ArtifactOrigins = []string{"https://fal.media", "https://fal.media"}
		},
		"HTTP artifact": func(p *proxy.ProviderCatalogProvider) { p.Transports[1].ArtifactOrigins = []string{"http://fal.media"} },
		"artifact path": func(p *proxy.ProviderCatalogProvider) {
			p.Transports[1].ArtifactOrigins = []string{"https://fal.media/files"}
		},
		"foreign codec origins": func(p *proxy.ProviderCatalogProvider) {
			p.Transports[0].ArtifactOrigins = []string{"https://fal.media"}
		},
		"model placeholder":     func(p *proxy.ProviderCatalogProvider) { p.Transports[1].Endpoint.Path = "/images" },
		"authentication prefix": func(p *proxy.ProviderCatalogProvider) { p.Transports[0].Components.Authentication.Prefix = "Bearer " },
		"model escape":          func(p *proxy.ProviderCatalogProvider) { p.Offerings[0].UpstreamModel = "reve/../other" },
		"unknown control":       func(p *proxy.ProviderCatalogProvider) { p.Offerings[0].Controls[0].ID = "quality" },
		"invalid ratio":         func(p *proxy.ProviderCatalogProvider) { p.Offerings[0].Controls[0].Values = []string{"9:9"} },
		"unknown limit":         func(p *proxy.ProviderCatalogProvider) { p.Offerings[0].Limits[0].ID = "tokens" },
		"invalid pixel unit":    func(p *proxy.ProviderCatalogProvider) { p.Offerings[0].Limits[2].Unit = "bytes" },
		"missing controls":      func(p *proxy.ProviderCatalogProvider) { p.Offerings[0].Controls = p.Offerings[0].Controls[:2] },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			schema := base.Schema()
			for index := range schema.Providers {
				if schema.Providers[index].ID == "fal" {
					mutate(&schema.Providers[index])
				}
			}
			if _, err := proxy.NewProviderCatalog(schema); err == nil {
				t.Fatal("unsupported catalog accepted")
			}
		})
	}
}

func TestFALImagesDeadlinePreservesAcceptedHandle(t *testing.T) {
	var polls atomic.Int32
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/models/pricing":
			_, _ = w.Write([]byte(`{"prices":[]}`))
		case r.Method == http.MethodPost:
			root := upstream.URL + "/reve/requests/deadline-job"
			_ = json.NewEncoder(w).Encode(map[string]string{"request_id": "deadline-job", "status_url": root + "/status", "response_url": root, "cancel_url": root + "/cancel"})
		default:
			polls.Add(1)
			_, _ = w.Write([]byte(`{"status":"IN_PROGRESS","request_id":"deadline-job"}`))
		}
	}))
	defer upstream.Close()
	client, database, _ := falImagesDurableClient(t, upstream, func(c *proxy.Configuration) { c.MediaOperationLifetimeSeconds = 1 })
	operation, err := client.CreateMediaOperation(t.Context(), "deadline", falImageTestInput("fal"))
	if err != nil {
		t.Fatal(err)
	}
	result := waitForImageGeneration(t, client, operation)
	if result.State != proxy.MediaOperationStateUncertain || polls.Load() == 0 {
		t.Fatalf("deadline=%+v", result)
	}
	var retained struct{ ProviderHandle string }
	if err := database.Table("media_operation_records").Where("operation_id = ?", operation.OperationID).Take(&retained).Error; err != nil || !strings.Contains(retained.ProviderHandle, "deadline-job") {
		t.Fatalf("retained=%+v error=%v", retained, err)
	}
}

func TestFALImagesFencedHandleWriteNeverResubmits(t *testing.T) {
	var database *gorm.DB
	var submissions atomic.Int32
	submitted := make(chan struct{})
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models/pricing" {
			_, _ = w.Write([]byte(`{"prices":[]}`))
			return
		}
		if r.Method != http.MethodPost {
			t.Error("job without persisted handle polled")
			w.WriteHeader(500)
			return
		}
		submissions.Add(1)
		if err := database.Exec("UPDATE media_operation_claim_records SET generation = generation + 1, expires_at = ?", time.Now().Add(-time.Second)).Error; err != nil {
			t.Error(err)
		}
		root := upstream.URL + "/reve/requests/fenced-job"
		_ = json.NewEncoder(w).Encode(map[string]string{"request_id": "fenced-job", "status_url": root + "/status", "response_url": root, "cancel_url": root + "/cancel"})
		close(submitted)
	}))
	defer upstream.Close()
	client, db, restart := falImagesDurableClient(t, upstream)
	database = db
	operation, err := client.CreateMediaOperation(t.Context(), "fenced", falImageTestInput("fal"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-submitted:
	case <-time.After(3 * time.Second):
		t.Fatal("provider not submitted")
	}
	result := waitForImageGeneration(t, restart(), operation)
	if result.State != proxy.MediaOperationStateUncertain || submissions.Load() != 1 {
		t.Fatalf("result=%+v submissions=%d", result, submissions.Load())
	}
}

func TestFALImagesDetachedConnectionRejectsQueuedDispatch(t *testing.T) {
	for _, change := range []string{"detached", "binding-changed"} {
		t.Run(change, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			var submissions atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/models/pricing" {
					_, _ = w.Write([]byte(`{"prices":[]}`))
					return
				}
				if submissions.Add(1) == 1 {
					close(started)
					<-release
				}
				w.WriteHeader(503)
			}))
			t.Cleanup(upstream.Close)
			client, database, _ := falImagesDurableClient(t, upstream)
			first, err := client.CreateMediaOperation(t.Context(), "first", falImageTestInput("fal"))
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("first operation not dispatched")
			}
			second, err := client.CreateMediaOperation(t.Context(), "second", falImageTestInput("fal"))
			if err != nil {
				t.Fatal(err)
			}
			if change == "detached" {
				if err := database.Exec("DELETE FROM managed_tenant_connection_records WHERE provider_id = ?", "fal").Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := database.Table("media_operation_records").Where("operation_id = ?", second.OperationID).Update("execution_binding", "obsolete").Error; err != nil {
					t.Fatal(err)
				}
			}
			unblock()
			_ = waitForImageGeneration(t, client, first)
			result := waitForImageGeneration(t, client, second)
			if result.State != proxy.MediaOperationStateFailed || submissions.Load() != 1 {
				t.Fatalf("detached=%+v submissions=%d", result, submissions.Load())
			}
		})
	}
}
