package proxy_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestImageGenerationStreamsDurableOrderedPreviews(t *testing.T) {
	fixtures := []imageEditingFixture{
		{catalog: testfixtures.ProviderCatalog(t), provider: "openai", model: "gpt-image-2", path: "/images/edits", header: "Authorization", credential: "Bearer image-provider-secret"},
		imageEditingSecondProvider(t),
	}
	for _, fixture := range fixtures {
		for _, capability := range []string{"image.generate", "image.edit"} {
			t.Run(fixture.provider+"/"+capability, func(t *testing.T) {
				images := streamingImageFixtures(t)
				finish := make(chan struct{})
				var finishOnce sync.Once
				completeProvider := func() { finishOnce.Do(func() { close(finish) }) }
				defer completeProvider()
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					calls.Add(1)
					if request.Method != http.MethodPost || request.Header.Get(fixture.header) != fixture.credential {
						t.Error("incorrect stream transport")
						writer.WriteHeader(http.StatusBadRequest)
						return
					}
					prefix := "image_generation"
					if capability == "image.edit" {
						prefix = "image_edit"
						if err := request.ParseMultipartForm(4 << 20); err != nil {
							t.Error(err)
							return
						}
						defer request.MultipartForm.RemoveAll()
						if request.URL.Path != fixture.path || request.FormValue("stream") != "true" || request.FormValue("partial_images") != "3" || request.FormValue("model") != fixture.model {
							t.Errorf("incorrect edit stream fields=%v", request.MultipartForm.Value)
						}
					} else {
						var payload map[string]any
						if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
							t.Error(err)
							return
						}
						if request.URL.Path != "/images/generations" || payload["stream"] != true || payload["partial_images"] != float64(3) || payload["model"] != fixture.model {
							t.Errorf("incorrect generation stream fields=%v", payload)
						}
					}
					writer.Header().Set("Content-Type", "text/event-stream")
					// Repeat an identical event: one durable preview must remain at this position.
					for _, index := range []int{0, 0, 1} {
						writeImageStreamEvent(writer, prefix+".partial_image", images[index], &index)
					}
					select {
					case <-finish:
						writeImageStreamEvent(writer, prefix+".completed", images[2], nil)
					case <-request.Context().Done():
					}
				}))
				t.Cleanup(upstream.Close)
				client := imageGenerationTestClient(t, upstream, fixture.catalog, fixture.provider)
				input := llmproxyclient.MediaOperationInput{
					Capability: capability, Provider: fixture.provider, Model: "gpt-image-2",
					Input:    json.RawMessage(`{"prompt":"A lighthouse preview"}`),
					Controls: json.RawMessage(`{"surface":"images","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1,"stream":true,"partial_images":3}`),
				}
				if capability == "image.edit" {
					asset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "image/png", Data: images[0]})
					if err != nil {
						t.Fatal(err)
					}
					input.Input, _ = json.Marshal(map[string]any{"prompt": "A lighthouse preview", "image_asset_ids": []string{asset.AssetID}})
				}
				accepted, err := client.CreateMediaOperation(t.Context(), "streaming-image", input)
				if err != nil {
					t.Fatalf("accept progressive image: %v", err)
				}
				var running llmproxyclient.MediaOperation
				var partials []llmproxyclient.MediaOperationPartialOutput
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					running, err = client.GetMediaOperation(t.Context(), accepted.OperationID)
					if err != nil {
						t.Fatal(err)
					}
					partials = running.PartialOutputs
					if len(partials) == 2 {
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
				if running.State != proxy.MediaOperationStateRunning || len(running.Outputs) != 0 || len(partials) != 2 {
					t.Fatalf("previews not published during execution: operation=%+v partials=%+v", running, partials)
				}
				contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
				if err != nil {
					t.Fatal(err)
				}
				publicJSON, _ := json.Marshal(running)
				if err := contract.ValidateResponse("/model/v1/operations/{operation_id}", http.MethodGet, http.StatusOK, http.Header{"Content-Type": {"application/json"}}, publicJSON); err != nil {
					t.Fatalf("progressive response differs from OpenAPI: %v", err)
				}
				for index, partial := range partials {
					if partial.OutputOrdinal != 0 || partial.PartialOrdinal != index || partial.MIMEType != "image/png" || partial.SizeBytes != int64(len(images[index])) {
						t.Fatalf("partial order=%+v", partials)
					}
					asset, err := client.GetAsset(t.Context(), partial.AssetID)
					if err != nil {
						t.Fatal(err)
					}
					data, err := client.DownloadAsset(t.Context(), asset)
					if err != nil || !bytes.Equal(data, images[index]) {
						t.Fatalf("preview bytes differ: %v", err)
					}
					if err := client.DeleteAsset(t.Context(), partial.AssetID); httpFailureStatus(err) != http.StatusConflict {
						t.Fatalf("active preview deletion: %v", err)
					}
				}
				cancelled, err := client.CancelMediaOperation(t.Context(), accepted.OperationID)
				if err != nil || cancelled.CancellationState != proxy.MediaCancellationUnsupported || cancelled.State != proxy.MediaOperationStateRunning {
					t.Fatalf("stream cancellation=%+v error=%v", cancelled, err)
				}
				completeProvider()
				completed := waitForImageGeneration(t, client, accepted)
				if completed.State != proxy.MediaOperationStateSucceeded || len(completed.Outputs) != 1 || len(completed.PartialOutputs) != 2 {
					t.Fatalf("stream result=%+v", completed)
				}
				asset, err := client.GetAsset(t.Context(), completed.Outputs[0].AssetID)
				if err != nil {
					t.Fatal(err)
				}
				data, err := client.DownloadAsset(t.Context(), asset)
				if err != nil || !bytes.Equal(data, images[2]) {
					t.Fatalf("final image differs: %v", err)
				}
				duplicate, err := client.CreateMediaOperation(t.Context(), "streaming-image", input)
				if err != nil || duplicate.OperationID != accepted.OperationID || calls.Load() != 1 {
					t.Fatalf("stream duplicate=%+v error=%v calls=%d", duplicate, err, calls.Load())
				}
			})
		}
	}
}

func streamingImageFixtures(t *testing.T) [][]byte {
	t.Helper()
	images := make([][]byte, 3)
	for index := range images {
		canvas := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
		canvas.SetNRGBA(index, index, color.NRGBA{R: uint8(60 + index), A: 255})
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, canvas); err != nil {
			t.Fatal(err)
		}
		images[index] = encoded.Bytes()
	}
	return images
}

func writeImageStreamEvent(writer http.ResponseWriter, eventType string, imageData []byte, partialIndex *int) {
	event := map[string]any{"type": eventType, "b64_json": base64.StdEncoding.EncodeToString(imageData), "output_format": "png", "size": "1024x1024", "quality": "low", "background": "opaque"}
	if partialIndex != nil {
		event["partial_image_index"] = *partialIndex
	}
	encoded, _ := json.Marshal(event)
	_, _ = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", eventType, encoded)
	writer.(http.Flusher).Flush()
}

func TestImageGenerationStreamingRejectsInvalidResultsAndKeepsUncertainPreviews(t *testing.T) {
	images := streamingImageFixtures(t)
	for _, scenario := range []struct {
		name, state, code string
		partials          int
		write             func(http.ResponseWriter)
	}{
		{"truncated", proxy.MediaOperationStateUncertain, "provider_outcome_unknown", 1, func(writer http.ResponseWriter) {
			index := 0
			writeImageStreamEvent(writer, "image_generation.partial_image", images[0], &index)
		}},
		{"missing index", proxy.MediaOperationStateFailed, "provider_result_invalid", 0, func(writer http.ResponseWriter) {
			writeImageStreamEvent(writer, "image_generation.partial_image", images[0], nil)
		}},
		{"out of order", proxy.MediaOperationStateFailed, "provider_result_invalid", 0, func(writer http.ResponseWriter) {
			index := 1
			writeImageStreamEvent(writer, "image_generation.partial_image", images[0], &index)
		}},
		{"changed duplicate", proxy.MediaOperationStateFailed, "provider_result_invalid", 1, func(writer http.ResponseWriter) {
			index := 0
			writeImageStreamEvent(writer, "image_generation.partial_image", images[0], &index)
			writeImageStreamEvent(writer, "image_generation.partial_image", images[1], &index)
		}},
		{"corrupt preview", proxy.MediaOperationStateFailed, "provider_result_invalid", 0, func(writer http.ResponseWriter) {
			index := 0
			writeImageStreamEvent(writer, "image_generation.partial_image", images[0][:len(images[0])-8], &index)
		}},
		{"corrupt final", proxy.MediaOperationStateFailed, "provider_result_invalid", 1, func(writer http.ResponseWriter) {
			index := 0
			writeImageStreamEvent(writer, "image_generation.partial_image", images[0], &index)
			writeImageStreamEvent(writer, "image_generation.completed", images[1][:len(images[1])-8], nil)
		}},
		{"wrong event family", proxy.MediaOperationStateFailed, "provider_result_invalid", 0, func(writer http.ResponseWriter) {
			writeImageStreamEvent(writer, "image_edit.completed", images[0], nil)
		}},
		{"malformed JSON", proxy.MediaOperationStateFailed, "provider_result_invalid", 0, func(writer http.ResponseWriter) { _, _ = fmt.Fprint(writer, "data: {invalid}\n\n") }},
		{"mismatched event", proxy.MediaOperationStateFailed, "provider_result_invalid", 0, func(writer http.ResponseWriter) {
			_, _ = fmt.Fprint(writer, "event: image_generation.completed\ndata: {\"type\":\"image_generation.partial_image\"}\n\n")
		}},
		{"provider error", proxy.MediaOperationStateFailed, "provider_error", 0, func(writer http.ResponseWriter) {
			_, _ = fmt.Fprint(writer, "data: {\"type\":\"error\",\"message\":\"private provider failure\"}\n\n")
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writer.Header().Set("Content-Type", "text/event-stream")
				scenario.write(writer)
			}))
			defer upstream.Close()
			client := imageGenerationTestClient(t, upstream, testfixtures.ProviderCatalog(t), "openai")
			input := imageGenerationTestIntent()
			input.Stream, input.PartialImages = true, 3
			accepted, err := client.CreateImageGeneration(t.Context(), "stream-failure", input)
			if err != nil {
				t.Fatal(err)
			}
			result := waitForImageGeneration(t, client, accepted)
			if result.State != scenario.state || result.Error == nil || result.Error.Code != scenario.code || len(result.Outputs) != 0 || len(result.PartialOutputs) != scenario.partials {
				t.Fatalf("invalid stream result=%+v", result)
			}
			duplicate, err := client.CreateImageGeneration(t.Context(), "stream-failure", input)
			if err != nil || duplicate.OperationID != accepted.OperationID || calls.Load() != 1 {
				t.Fatalf("failure duplicate=%+v error=%v calls=%d", duplicate, err, calls.Load())
			}
		})
	}
}

func TestImageGenerationStreamingControlsRejectAmbiguousOutputBeforeDispatch(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusBadRequest)
	}))
	defer upstream.Close()
	client := imageGenerationTestClient(t, upstream, testfixtures.ProviderCatalog(t), "openai")
	for index, scenario := range []struct {
		stream          bool
		partials, count int
	}{{false, 1, 1}, {true, -1, 1}, {true, 4, 1}, {true, 3, 2}} {
		input := imageGenerationTestIntent()
		input.Stream, input.PartialImages, input.OutputCount = scenario.stream, scenario.partials, scenario.count
		if _, err := client.CreateImageGeneration(t.Context(), fmt.Sprintf("invalid-stream-%d", index), input); httpFailureStatus(err) != http.StatusBadRequest {
			t.Fatalf("invalid controls=%+v error=%v", scenario, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid stream dispatches=%d", calls.Load())
	}
}

func TestImageGenerationStreamingPreviewIsolationRestartAndRetention(t *testing.T) {
	for _, outcome := range []string{"success", "failure", "worker-loss"} {
		t.Run(outcome, func(t *testing.T) {
			images := streamingImageFixtures(t)
			var calls atomic.Int32
			release := make(chan struct{})
			var once sync.Once
			releaseProvider := func() { once.Do(func() { close(release) }) }
			defer releaseProvider()
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				writer.Header().Set("Content-Type", "text/event-stream")
				index := 0
				writeImageStreamEvent(writer, "image_generation.partial_image", images[0], &index)
				select {
				case <-release:
				case <-request.Context().Done():
					return
				}
				if outcome == "failure" {
					writeImageStreamEvent(writer, "image_generation.completed", []byte("corrupt"), nil)
					return
				}
				index = 1
				writeImageStreamEvent(writer, "image_generation.partial_image", images[1], &index)
				writeImageStreamEvent(writer, "image_generation.completed", images[2], nil)
			}))
			t.Cleanup(upstream.Close)
			endpoints := proxy.NewEndpoints()
			endpoints.SetProviderBaseURL("openai", upstream.URL)
			configuration, err := testfixtures.ProvisionManagedRouter(t, proxy.Configuration{
				ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: t.TempDir(), Endpoints: endpoints,
				Management:                 proxy.ManagementConfiguration{DatabasePath: filepath.Join(t.TempDir(), "preview.sqlite")},
				MediaOperationClaimSeconds: 7200, MediaOperationClaimRenewalSeconds: 3600,
			}, zap.NewNop().Sugar(), testfixtures.ManagedTenant{Secret: "preview-owner", Defaults: proxy.TenantDefaults{Provider: "openai", Model: proxy.ModelNameGPT41}, ProviderKeys: map[string]string{"openai": "preview-provider"}})
			if err != nil {
				t.Fatal(err)
			}
			database, err := gorm.Open(configuration.Management.DatabaseDialector, &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			newClient := func(server *httptest.Server, secret string) llmproxyclient.Client {
				config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
				if err != nil {
					t.Fatal(err)
				}
				client, err := llmproxyclient.NewClient(config, server.Client())
				if err != nil {
					t.Fatal(err)
				}
				return client
			}
			router, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			owner := newClient(server, "preview-owner")
			cookie := imageGenerationOwnerCookie(t, configuration.Management, "foreign-preview-owner")
			account := requestManagementAccount(t, router, cookie)
			foreign := newClient(server, generateManagementTenantSecret(t, router, cookie, account.Tenants[0].ID))
			input := imageGenerationTestIntent()
			input.Stream, input.PartialImages = true, 3
			accepted, err := owner.CreateImageGeneration(t.Context(), "durable-preview", input)
			if err != nil {
				t.Fatal(err)
			}
			var running llmproxyclient.MediaOperation
			for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
				running, err = owner.GetMediaOperation(t.Context(), accepted.OperationID)
				if err != nil {
					t.Fatal(err)
				}
				if len(running.PartialOutputs) == 1 {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if len(running.PartialOutputs) != 1 {
				t.Fatalf("preview not published: %+v", running)
			}
			previewID := running.PartialOutputs[0].AssetID
			asset, err := owner.GetAsset(t.Context(), previewID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := foreign.GetMediaOperation(t.Context(), accepted.OperationID); httpFailureStatus(err) != http.StatusNotFound {
				t.Fatalf("foreign operation: %v", err)
			}
			if _, err := foreign.GetAsset(t.Context(), previewID); httpFailureStatus(err) != http.StatusNotFound {
				t.Fatalf("foreign preview: %v", err)
			}
			if _, err := foreign.DownloadAsset(t.Context(), asset); httpFailureStatus(err) != http.StatusNotFound {
				t.Fatalf("foreign preview bytes: %v", err)
			}
			if err := foreign.DeleteAsset(t.Context(), previewID); httpFailureStatus(err) != http.StatusNotFound {
				t.Fatalf("foreign preview delete: %v", err)
			}
			publicJSON, _ := json.Marshal(running)
			if strings.Contains(string(publicJSON), `"sha256":`) || strings.Contains(string(publicJSON), `"content_sha256":`) {
				t.Fatal("private preview integrity exposed")
			}
			if outcome == "worker-loss" {
				if err := database.Exec("UPDATE media_operation_claim_records SET generation = generation + 1, expires_at = ? WHERE operation_id = ?", time.Now().Add(-time.Second), accepted.OperationID).Error; err != nil {
					t.Fatal(err)
				}
			}
			releaseProvider()
			newServer := func() llmproxyclient.Client {
				restartedRouter, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
				if err != nil {
					t.Fatal(err)
				}
				restarted := httptest.NewServer(restartedRouter)
				t.Cleanup(restarted.Close)
				return newClient(restarted, "preview-owner")
			}
			if outcome == "worker-loss" {
				owner = newServer()
			}
			result := waitForImageGeneration(t, owner, accepted)
			expectedState, expectedPartials := proxy.MediaOperationStateSucceeded, 2
			if outcome == "failure" {
				expectedState, expectedPartials = proxy.MediaOperationStateFailed, 1
			}
			if outcome == "worker-loss" {
				expectedState, expectedPartials = proxy.MediaOperationStateUncertain, 1
			}
			if result.State != expectedState || len(result.PartialOutputs) != expectedPartials || calls.Load() != 1 {
				t.Fatalf("result=%+v calls=%d", result, calls.Load())
			}
			owner = newServer()
			retained, err := owner.GetMediaOperation(t.Context(), accepted.OperationID)
			if err != nil || len(retained.PartialOutputs) != expectedPartials || retained.PartialOutputs[0].AssetID != previewID {
				t.Fatalf("restart previews=%+v error=%v", retained, err)
			}
			data, err := owner.DownloadAsset(t.Context(), asset)
			if err != nil || !bytes.Equal(data, images[0]) {
				t.Fatalf("retained preview bytes: %v", err)
			}
			if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Update("terminal_at", time.Now().Add(-49*time.Hour)).Error; err != nil {
				t.Fatal(err)
			}
			owner = newServer()
			_, err = owner.GetMediaOperation(t.Context(), accepted.OperationID)
			if outcome == "worker-loss" {
				if err != nil {
					t.Fatalf("uncertain operation expired: %v", err)
				}
				if err := owner.DeleteAsset(t.Context(), previewID); httpFailureStatus(err) != http.StatusConflict {
					t.Fatalf("uncertain preview unprotected: %v", err)
				}
			} else {
				if httpFailureStatus(err) != http.StatusGone {
					t.Fatalf("terminal operation expiry: %v", err)
				}
				if err := owner.DeleteAsset(t.Context(), previewID); err != nil {
					t.Fatalf("expired preview reference remains: %v", err)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("restart repeated paid submission: %d", calls.Load())
			}
		})
	}
}
