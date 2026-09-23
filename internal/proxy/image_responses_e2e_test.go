package proxy_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestImageGenerationResponsesUsesPrivateHandlesAndGatewayOperationChains(t *testing.T) {
	images := streamingImageFixtures(t)
	var creates, polls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("Authorization") != "Bearer image-provider-secret" {
			t.Error("missing provider authentication")
		}
		if request.Method == http.MethodPost && request.URL.Path == "/responses" {
			call := creates.Add(1)
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Error(err)
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			if payload["model"] != "gpt-5" || payload["background"] != true || payload["store"] != true || payload["max_tool_calls"] != float64(1) {
				t.Errorf("Responses controls=%v", payload)
			}
			tool := payload["tools"].([]any)[0].(map[string]any)
			action := "generate"
			if call == 2 {
				action = "edit"
			}
			expected := map[string]any{"type": "image_generation", "model": "gpt-image-2", "action": action, "quality": "low", "size": "1024x1024", "background": "opaque", "output_format": "png"}
			if !reflect.DeepEqual(tool, expected) {
				t.Errorf("Responses tool=%v want=%v", tool, expected)
			}
			if call == 1 && payload["previous_response_id"] != nil {
				t.Error("unexpected initial chain")
			}
			if call == 2 && payload["previous_response_id"] != "resp_private_first" {
				t.Errorf("private chain translation=%v", payload["previous_response_id"])
			}
			id := "resp_private_first"
			if call == 2 {
				id = "resp_private_second"
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"id": id, "status": "queued", "output": []any{}})
			return
		}
		if request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/responses/resp_private_") {
			polls.Add(1)
			id := strings.TrimPrefix(request.URL.Path, "/responses/")
			index := 0
			if id == "resp_private_second" {
				index = 1
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"id": id, "status": "completed", "output": []any{map[string]any{"id": "ig_private", "type": "image_generation_call", "status": "completed", "result": base64.StdEncoding.EncodeToString(images[index])}}})
			return
		}
		t.Errorf("unexpected Responses request: %s %s", request.Method, request.URL.Path)
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	client := imageGenerationTestClient(t, upstream, testfixtures.ProviderCatalog(t), "openai")
	input := llmproxyclient.MediaOperationInput{
		Capability: "image.generate", Provider: "openai", Model: "gpt-image-2", Input: json.RawMessage(`{"prompt":"A lighthouse"}`),
		Controls: json.RawMessage(`{"surface":"responses","responses_model":"gpt-5","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1}`),
	}
	first, err := client.CreateMediaOperation(t.Context(), "responses-image-first", input)
	if err != nil {
		t.Fatalf("accept Responses generation: %v", err)
	}
	first = waitForImageGeneration(t, client, first)
	if first.State != proxy.MediaOperationStateSucceeded || len(first.Outputs) != 1 {
		t.Fatalf("first response=%+v", first)
	}
	followUp := input
	followUp.Capability = "image.edit"
	followUp.Input, _ = json.Marshal(map[string]any{"prompt": "Make it a sunset", "previous_operation_id": first.OperationID})
	second, err := client.CreateMediaOperation(t.Context(), "responses-image-second", followUp)
	if err != nil {
		t.Fatalf("accept gateway chain: %v", err)
	}
	second = waitForImageGeneration(t, client, second)
	if second.State != proxy.MediaOperationStateSucceeded || len(second.Outputs) != 1 || second.PreviousOperationID != first.OperationID {
		t.Fatalf("second response=%+v", second)
	}
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	for index, result := range []llmproxyclient.MediaOperation{first, second} {
		encoded, _ := json.Marshal(result)
		if err := contract.ValidateResponse("/model/v1/operations/{operation_id}", http.MethodGet, http.StatusOK, http.Header{"Content-Type": {"application/json"}}, encoded); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(encoded, []byte("resp_private")) || bytes.Contains(encoded, []byte("ig_private")) {
			t.Fatalf("native identifier exposed: %s", encoded)
		}
		asset, err := client.GetAsset(t.Context(), result.Outputs[0].AssetID)
		if err != nil {
			t.Fatal(err)
		}
		data, err := client.DownloadAsset(t.Context(), asset)
		if err != nil || !bytes.Equal(data, images[index]) {
			t.Fatalf("Responses image differs: %v", err)
		}
	}
	duplicate, err := client.CreateMediaOperation(t.Context(), "responses-image-second", followUp)
	if err != nil || duplicate.OperationID != second.OperationID || creates.Load() != 2 || polls.Load() != 2 {
		t.Fatalf("duplicate=%+v error=%v creates=%d polls=%d", duplicate, err, creates.Load(), polls.Load())
	}
}

func TestImageGenerationResponsesRecoversAcceptedJobWithoutResubmission(t *testing.T) {
	images := streamingImageFixtures(t)
	var creates, polls atomic.Int32
	firstPoll, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	releaseProvider := func() { once.Do(func() { close(release) }) }
	defer releaseProvider()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/responses" {
			creates.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "resp_recovery_private", "status": "queued"})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/responses/resp_recovery_private" {
			if polls.Add(1) == 1 {
				close(firstPoll)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			writeCompletedImageResponse(w, "resp_recovery_private", images[0])
			return
		}
		t.Errorf("unexpected provider request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(upstream.Close)
	owner, database, restart, _ := imageResponsesDurableClient(t, upstream)
	input := imageGenerationTestIntent()
	input.Surface, input.ResponsesModel = "responses", "gpt-5"
	accepted, err := owner.CreateImageGeneration(t.Context(), "recover-responses-image", input)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstPoll:
	case <-time.After(3 * time.Second):
		t.Fatal("accepted response was not polled")
	}
	if err := database.Exec("UPDATE media_operation_claim_records SET generation = generation + 1, expires_at = ? WHERE operation_id = ?", time.Now().Add(-time.Second), accepted.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	recovered := restart()
	result := waitForImageGeneration(t, recovered, accepted)
	releaseProvider()
	if result.State != proxy.MediaOperationStateSucceeded || len(result.Outputs) != 1 || creates.Load() != 1 || polls.Load() != 2 {
		t.Fatalf("recovered=%+v creates=%d polls=%d", result, creates.Load(), polls.Load())
	}
}

func TestImageGenerationResponsesConfirmsBackgroundCancellation(t *testing.T) {
	var creates, cancels, polls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		status := "in_progress"
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/responses":
			creates.Add(1)
		case r.Method == http.MethodPost && r.URL.Path == "/responses/resp_cancel_private/cancel":
			cancels.Add(1)
			status = "cancelled"
		case r.Method == http.MethodGet && r.URL.Path == "/responses/resp_cancel_private":
			polls.Add(1)
			if cancels.Load() > 0 {
				status = "cancelled"
			}
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "resp_cancel_private", "status": status})
	}))
	defer upstream.Close()
	owner := imageGenerationTestClient(t, upstream, testfixtures.ProviderCatalog(t), "openai")
	input := imageGenerationTestIntent()
	input.Surface, input.ResponsesModel = "responses", "gpt-5"
	accepted, err := owner.CreateImageGeneration(t.Context(), "cancel-responses-image", input)
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(3 * time.Second); polls.Load() == 0 && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	if polls.Load() == 0 {
		t.Fatal("response was not accepted")
	}
	cancelled, err := owner.CancelMediaOperation(t.Context(), accepted.OperationID)
	if err != nil || cancelled.State != proxy.MediaOperationStateCancelled || cancelled.CancellationState != proxy.MediaCancellationConfirmed || creates.Load() != 1 || cancels.Load() != 1 {
		t.Fatalf("cancelled=%+v error=%v creates=%d cancels=%d", cancelled, err, creates.Load(), cancels.Load())
	}
}

func writeCompletedImageResponse(w http.ResponseWriter, id string, data []byte) {
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "status": "completed", "output": []any{map[string]any{"id": "ig_private", "type": "image_generation_call", "status": "completed", "result": base64.StdEncoding.EncodeToString(data)}}})
}

func imageResponsesDurableClient(t *testing.T, upstream *httptest.Server) (llmproxyclient.Client, *gorm.DB, func(...func(*proxy.Configuration)) llmproxyclient.Client, llmproxyclient.Client) {
	t.Helper()
	endpoints := proxy.NewEndpoints()
	endpoints.SetProviderBaseURL("openai", upstream.URL)
	configuration, err := testfixtures.ProvisionManagedRouter(t, proxy.Configuration{ProviderCatalog: testfixtures.ProviderCatalog(t), AssetStorePath: t.TempDir(), Endpoints: endpoints, Management: proxy.ManagementConfiguration{DatabasePath: filepath.Join(t.TempDir(), "responses.sqlite")}, MediaOperationClaimSeconds: 7200, MediaOperationClaimRenewalSeconds: 3600}, zap.NewNop().Sugar(), testfixtures.ManagedTenant{Secret: "responses-owner", Defaults: proxy.TenantDefaults{Provider: "openai", Model: proxy.ModelNameGPT41}, ProviderKeys: map[string]string{"openai": "responses-provider"}})
	if err != nil {
		t.Fatal(err)
	}
	database, err := gorm.Open(configuration.Management.DatabaseDialector, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var foreignClient llmproxyclient.Client
	var foreignSecret string
	restart := func(changes ...func(*proxy.Configuration)) llmproxyclient.Client {
		for _, change := range changes {
			change(&configuration)
		}
		router, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar())
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(router)
		t.Cleanup(server.Close)
		config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "responses-owner"})
		if err != nil {
			t.Fatal(err)
		}
		client, err := llmproxyclient.NewClient(config, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		if foreignSecret == "" {
			cookie := imageGenerationOwnerCookie(t, configuration.Management, "managed-router-fixture-user")
			otherTenant := createManagementTenant(t, router, cookie, "Responses chain isolation").Tenant.ID
			var assignment struct{ ConnectionID string }
			if err := database.Table("managed_tenant_connection_records").Where("provider_id = ?", "openai").First(&assignment).Error; err != nil {
				t.Fatal(err)
			}
			accountConnectionExchange(t, router, cookie, http.MethodPut, "/tenants/"+otherTenant+"/connections/openai", map[string]string{"kind": "account_connection", "resource_id": assignment.ConnectionID}, http.StatusOK)
			foreignSecret = generateManagementTenantSecret(t, router, cookie, otherTenant)
		}
		foreignConfig, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: foreignSecret})
		if err != nil {
			t.Fatal(err)
		}
		foreignClient, err = llmproxyclient.NewClient(foreignConfig, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	owner := restart()
	return owner, database, restart, foreignClient
}

func TestImageGenerationResponsesStreamsVerifiedPreviewsAndRetrievesInterruptedResponse(t *testing.T) {
	for _, outcome := range []string{"completed", "interrupted"} {
		t.Run(outcome, func(t *testing.T) {
			images := streamingImageFixtures(t)
			var creates, polls atomic.Int32
			release := make(chan struct{})
			var once sync.Once
			releaseProvider := func() { once.Do(func() { close(release) }) }
			defer releaseProvider()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/responses/resp_stream_private" {
					polls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					writeCompletedImageResponse(w, "resp_stream_private", images[2])
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/responses" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				creates.Add(1)
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					return
				}
				tool := payload["tools"].([]any)[0].(map[string]any)
				if payload["stream"] != true || tool["partial_images"] != float64(3) {
					t.Errorf("stream controls=%v", payload)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				writeResponsesImageEvent(w, map[string]any{"type": "response.created", "sequence_number": 0, "response": map[string]any{"id": "resp_stream_private", "status": "in_progress"}})
				writeResponsesImageEvent(w, map[string]any{"type": "response.output_item.added", "sequence_number": 1, "output_index": 1, "item": map[string]any{"id": "ig_stream_private", "type": "image_generation_call", "status": "in_progress"}})
				writeResponsesImageEvent(w, map[string]any{"type": "response.image_generation_call.partial_image", "sequence_number": 2, "output_index": 1, "item_id": "ig_stream_private", "partial_image_index": 0, "partial_image_b64": base64.StdEncoding.EncodeToString(images[0])})
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				if outcome == "interrupted" {
					return
				}
				writeResponsesImageEvent(w, map[string]any{"type": "response.image_generation_call.partial_image", "sequence_number": 3, "output_index": 1, "item_id": "ig_stream_private", "partial_image_index": 0, "partial_image_b64": base64.StdEncoding.EncodeToString(images[0])})
				writeResponsesImageEvent(w, map[string]any{"type": "response.image_generation_call.partial_image", "sequence_number": 4, "output_index": 1, "item_id": "ig_stream_private", "partial_image_index": 1, "partial_image_b64": base64.StdEncoding.EncodeToString(images[1])})
				writeResponsesImageEvent(w, map[string]any{"type": "response.completed", "sequence_number": 5, "response": map[string]any{"id": "resp_stream_private", "status": "completed", "output": []any{map[string]any{"id": "reason_private", "type": "reasoning"}, map[string]any{"id": "ig_stream_private", "type": "image_generation_call", "status": "completed", "result": base64.StdEncoding.EncodeToString(images[2])}}}})
			}))
			t.Cleanup(upstream.Close)
			owner := imageGenerationTestClient(t, upstream, testfixtures.ProviderCatalog(t), "openai")
			input := imageGenerationTestIntent()
			input.Surface, input.ResponsesModel = "responses", "gpt-5"
			input.Stream, input.PartialImages = true, 3
			accepted, err := owner.CreateImageGeneration(t.Context(), "stream-responses-image", input)
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
			if running.State != proxy.MediaOperationStateRunning || len(running.PartialOutputs) != 1 || running.PartialOutputs[0].OutputOrdinal != 0 {
				t.Fatalf("live Responses preview=%+v", running)
			}
			previewID := running.PartialOutputs[0].AssetID
			releaseProvider()
			result := waitForImageGeneration(t, owner, accepted)
			expectedPreviews, expectedPolls := 2, int32(0)
			if outcome == "interrupted" {
				expectedPreviews, expectedPolls = 1, 1
			}
			if result.State != proxy.MediaOperationStateSucceeded || len(result.PartialOutputs) != expectedPreviews || result.PartialOutputs[0].AssetID != previewID || creates.Load() != 1 || polls.Load() != expectedPolls {
				t.Fatalf("streamed=%+v creates=%d polls=%d", result, creates.Load(), polls.Load())
			}
			asset, err := owner.GetAsset(t.Context(), result.Outputs[0].AssetID)
			if err != nil {
				t.Fatal(err)
			}
			data, err := owner.DownloadAsset(t.Context(), asset)
			if err != nil || !bytes.Equal(data, images[2]) {
				t.Fatalf("final bytes differ: %v", err)
			}
		})
	}
}

func writeResponsesImageEvent(w http.ResponseWriter, event map[string]any) {
	encoded, _ := json.Marshal(event)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], encoded)
	w.(http.Flusher).Flush()
}

func TestImageGenerationResponsesRejectsParentFromChangedRoute(t *testing.T) {
	images := streamingImageFixtures(t)
	var creates, foreignCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		creates.Add(1)
		writeCompletedImageResponse(w, "resp_original_route", images[0])
	}))
	defer upstream.Close()
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignCalls.Add(1)
		writeCompletedImageResponse(w, "resp_foreign_route", images[1])
	}))
	defer foreign.Close()
	owner, _, restart, _ := imageResponsesDurableClient(t, upstream)
	input := imageGenerationTestIntent()
	input.Surface, input.ResponsesModel = "responses", "gpt-5"
	first, err := owner.CreateImageGeneration(t.Context(), "route-bound-parent", input)
	if err != nil {
		t.Fatal(err)
	}
	first = waitForImageGeneration(t, owner, first)
	if first.State != proxy.MediaOperationStateSucceeded {
		t.Fatalf("parent=%+v", first)
	}
	changed := restart(func(config *proxy.Configuration) {
		config.Endpoints = proxy.NewEndpoints()
		config.Endpoints.SetProviderBaseURL("openai", foreign.URL)
		config.UpstreamCapacity.Origins = nil
		*config = testfixtures.WithUpstreamCapacity(t, *config)
	})
	input.PreviousOperationID = first.OperationID
	_, err = changed.CreateImageEditing(t.Context(), "changed-route-chain", llmproxyclient.ImageEditingInput{ImageGenerationInput: input})
	if httpFailureStatus(err) != http.StatusBadRequest || creates.Load() != 1 || foreignCalls.Load() != 0 {
		t.Fatalf("changed-route parent accepted: error=%v original=%d foreign=%d", err, creates.Load(), foreignCalls.Load())
	}
}

func TestImageGenerationResponsesSecondProviderUsesCatalogValuesAndOrderedAssets(t *testing.T) {
	fixture := imageEditingSecondProvider(t)
	images := streamingImageFixtures(t)
	for _, capability := range []string{"image.generate", "image.edit"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", capability, stream), func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Method != http.MethodPost || r.URL.Path != "/compose" || r.Header.Get("X-Edit-Token") != fixture.credential || r.Header.Get("Authorization") != "" {
						t.Errorf("catalog transport mismatch: %s %s %v", r.Method, r.URL.Path, r.Header)
					}
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
						return
					}
					tool := payload["tools"].([]any)[0].(map[string]any)
					action := "generate"
					if capability == "image.edit" {
						action = "edit"
					}
					if payload["model"] != "fixture-text-model" || payload["background"] != true || payload["store"] != true || tool["model"] != fixture.model || tool["action"] != action {
						t.Errorf("catalog model/action mismatch: %v", payload)
					}
					content := payload["input"].([]any)[0].(map[string]any)["content"].([]any)
					expected := []any{map[string]any{"type": "input_text", "text": "A small lighthouse beside the sea"}}
					if capability == "image.edit" {
						for _, data := range images[:2] {
							expected = append(expected, map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)})
						}
					}
					if !reflect.DeepEqual(content, expected) {
						t.Errorf("ordered image input differs: content count=%d", len(content))
					}
					if stream {
						if payload["stream"] != true || tool["partial_images"] != float64(2) {
							t.Error("stream controls differ")
						}
						w.Header().Set("Content-Type", "text/event-stream")
						writeResponsesImageEvent(w, map[string]any{"type": "response.created", "sequence_number": 0, "response": map[string]any{"id": "fixture_response", "status": "in_progress"}})
						writeResponsesImageEvent(w, map[string]any{"type": "response.image_generation_call.partial_image", "sequence_number": 1, "output_index": 0, "item_id": "fixture_item", "partial_image_index": 0, "partial_image_b64": base64.StdEncoding.EncodeToString(images[0])})
						writeResponsesImageEvent(w, map[string]any{"type": "response.completed", "sequence_number": 2, "response": map[string]any{"id": "fixture_response", "status": "completed", "output": []any{map[string]any{"id": "fixture_item", "type": "image_generation_call", "status": "completed", "result": base64.StdEncoding.EncodeToString(images[2])}}}})
					} else {
						w.Header().Set("Content-Type", "application/json")
						writeCompletedImageResponse(w, "fixture_response", images[2])
					}
				}))
				defer upstream.Close()
				client := imageGenerationTestClient(t, upstream, fixture.catalog, fixture.provider)
				input := imageGenerationTestIntent()
				input.Provider = fixture.provider
				input.Surface, input.ResponsesModel = "responses", "gpt-5"
				input.Stream = stream
				if stream {
					input.PartialImages = 2
				}
				var accepted llmproxyclient.MediaOperation
				var err error
				if capability == "image.edit" {
					var assets []string
					for _, data := range images[:2] {
						asset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{Data: data, MIMEType: "image/png"})
						if err != nil {
							t.Fatal(err)
						}
						assets = append(assets, asset.AssetID)
					}
					accepted, err = client.CreateImageEditing(t.Context(), "second-provider-responses", llmproxyclient.ImageEditingInput{ImageGenerationInput: input, ImageAssetIDs: assets})
				} else {
					accepted, err = client.CreateImageGeneration(t.Context(), "second-provider-responses", input)
				}
				if err != nil {
					t.Fatal(err)
				}
				result := waitForImageGeneration(t, client, accepted)
				if result.State != proxy.MediaOperationStateSucceeded || calls.Load() != 1 || len(result.Outputs) != 1 || (stream && len(result.PartialOutputs) != 1) {
					t.Fatalf("result=%+v calls=%d", result, calls.Load())
				}
			})
		}
	}
}

func TestImageGenerationResponsesRejectsForeignAndInvalidChainInputsBeforeDispatch(t *testing.T) {
	images := streamingImageFixtures(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeCompletedImageResponse(w, "resp_parent_private", images[0])
	}))
	defer upstream.Close()
	owner, database, _, foreign := imageResponsesDurableClient(t, upstream)
	input := imageGenerationTestIntent()
	input.Surface, input.ResponsesModel = "responses", "gpt-5"
	parent, err := owner.CreateImageGeneration(t.Context(), "valid-chain-parent", input)
	if err != nil {
		t.Fatal(err)
	}
	parent = waitForImageGeneration(t, owner, parent)
	if parent.State != proxy.MediaOperationStateSucceeded {
		t.Fatalf("parent=%+v", parent)
	}
	input.PreviousOperationID = parent.OperationID
	if _, err := foreign.GetMediaOperation(t.Context(), parent.OperationID); httpFailureStatus(err) != http.StatusNotFound {
		t.Fatalf("foreign read=%v", err)
	}
	if _, err := foreign.CreateImageEditing(t.Context(), "foreign-chain", llmproxyclient.ImageEditingInput{ImageGenerationInput: input}); httpFailureStatus(err) != http.StatusBadRequest {
		t.Fatalf("foreign chain=%v", err)
	}
	for name, change := range map[string]func(*llmproxyclient.ImageEditingInput){
		"native parent": func(v *llmproxyclient.ImageEditingInput) { v.PreviousOperationID = "resp_native" },
		"unknown parent": func(v *llmproxyclient.ImageEditingInput) {
			v.PreviousOperationID = "mop_0123456789abcdef0123456789abcdef"
		},
		"missing text model":        func(v *llmproxyclient.ImageEditingInput) { v.ResponsesModel = "" },
		"unknown text model":        func(v *llmproxyclient.ImageEditingInput) { v.ResponsesModel = "not-in-catalog" },
		"multiple outputs":          func(v *llmproxyclient.ImageEditingInput) { v.OutputCount = 2 },
		"images surface chain":      func(v *llmproxyclient.ImageEditingInput) { v.Surface = "images"; v.ResponsesModel = "" },
		"mask":                      func(v *llmproxyclient.ImageEditingInput) { v.MaskAssetID = "ast_0123456789abcdef0123456789abcdef" },
		"missing parent and images": func(v *llmproxyclient.ImageEditingInput) { v.PreviousOperationID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := llmproxyclient.ImageEditingInput{ImageGenerationInput: input}
			change(&candidate)
			if _, err := owner.CreateImageEditing(t.Context(), "invalid-chain-"+strings.ReplaceAll(name, " ", "-"), candidate); httpFailureStatus(err) != http.StatusBadRequest {
				t.Fatalf("invalid input=%v", err)
			}
		})
	}
	for name, changes := range map[string]map[string]any{
		"failed parent":      {"public_state": "failed"},
		"changed connection": {"credential_reference": "unrelated:v9"},
		"wrong text model":   {"normalized_controls": []byte(`{"surface":"responses","responses_model":"other-model"}`)},
		"images parent":      {"normalized_controls": []byte(`{"surface":"images"}`)},
		"invalid handle":     {"provider_handle": "../private"},
	} {
		t.Run(name, func(t *testing.T) {
			var saved struct {
				PublicState         string
				CredentialReference string
				NormalizedControls  []byte
				ProviderHandle      string
			}
			if err := database.Table("media_operation_records").Select("public_state", "credential_reference", "normalized_controls", "provider_handle").Where("operation_id = ?", parent.OperationID).Take(&saved).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.Table("media_operation_records").Where("operation_id = ?", parent.OperationID).Updates(changes).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := owner.CreateImageEditing(t.Context(), "invalid-parent-"+strings.ReplaceAll(name, " ", "-"), llmproxyclient.ImageEditingInput{ImageGenerationInput: input}); httpFailureStatus(err) != http.StatusBadRequest {
				t.Fatalf("invalid parent=%v", err)
			}
			restore := map[string]any{"public_state": saved.PublicState, "credential_reference": saved.CredentialReference, "normalized_controls": saved.NormalizedControls, "provider_handle": saved.ProviderHandle}
			if err := database.Table("media_operation_records").Where("operation_id = ?", parent.OperationID).Updates(restore).Error; err != nil {
				t.Fatal(err)
			}
		})
	}
	if calls.Load() != 1 {
		t.Fatalf("invalid chain dispatched %d times", calls.Load())
	}
}

func TestImageGenerationResponsesRetainsParentWhileChildOutcomeIsOutstanding(t *testing.T) {
	for _, outcome := range []string{"completed", "uncertain"} {
		t.Run(outcome, func(t *testing.T) {
			images := streamingImageFixtures(t)
			var calls atomic.Int32
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			releaseProvider := func() { once.Do(func() { close(release) }) }
			defer releaseProvider()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					writeCompletedImageResponse(w, "resp_retained_parent", images[0])
					return
				}
				close(started)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				if outcome == "uncertain" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				writeCompletedImageResponse(w, "resp_retained_child", images[1])
			}))
			t.Cleanup(upstream.Close)
			owner, database, restart, _ := imageResponsesDurableClient(t, upstream)
			input := imageGenerationTestIntent()
			input.Surface, input.ResponsesModel = "responses", "gpt-5"
			parent, err := owner.CreateImageGeneration(t.Context(), "retained-parent", input)
			if err != nil {
				t.Fatal(err)
			}
			parent = waitForImageGeneration(t, owner, parent)
			input.PreviousOperationID = parent.OperationID
			child, err := owner.CreateImageEditing(t.Context(), "retained-child", llmproxyclient.ImageEditingInput{ImageGenerationInput: input})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("child did not dispatch")
			}
			if err := database.Table("media_operation_records").Where("operation_id = ?", parent.OperationID).Update("terminal_at", time.Now().Add(-49*time.Hour)).Error; err != nil {
				t.Fatal(err)
			}
			restarted := restart()
			if _, err := restarted.GetMediaOperation(t.Context(), parent.OperationID); err != nil {
				t.Fatalf("parent expired while child running: %v", err)
			}
			releaseProvider()
			result := waitForImageGeneration(t, restarted, child)
			expected := proxy.MediaOperationStateSucceeded
			if outcome == "uncertain" {
				expected = proxy.MediaOperationStateUncertain
			}
			if result.State != expected {
				t.Fatalf("child=%+v", result)
			}
			restarted = restart()
			_, err = restarted.GetMediaOperation(t.Context(), parent.OperationID)
			if outcome == "uncertain" {
				if err != nil {
					t.Fatalf("uncertain child's parent expired: %v", err)
				}
			} else if httpFailureStatus(err) != http.StatusGone {
				t.Fatalf("completed child's expired parent remains: %v", err)
			}
			if calls.Load() != 2 {
				t.Fatalf("repeated submission count=%d", calls.Load())
			}
		})
	}
}

func TestImageGenerationResponsesRejectsInvalidProviderStreams(t *testing.T) {
	images := streamingImageFixtures(t)
	for _, name := range []string{"missing created", "missing sequence", "wrong event name", "wrong family", "invalid JSON", "wrong partial index", "missing item", "changed duplicate", "changed item", "wrong format", "corrupt preview", "wrong terminal response", "wrong terminal item", "corrupt terminal", "provider failure", "lost stream"} {
		t.Run(name, func(t *testing.T) {
			var creates, polls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					polls.Add(1)
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				creates.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				created := map[string]any{"type": "response.created", "sequence_number": 0, "response": map[string]any{"id": "resp_invalid_test", "status": "in_progress"}}
				partial := map[string]any{"type": "response.image_generation_call.partial_image", "sequence_number": 1, "output_index": 0, "item_id": "ig_invalid_test", "partial_image_index": 0, "partial_image_b64": base64.StdEncoding.EncodeToString(images[0])}
				item := map[string]any{"id": "ig_invalid_test", "type": "image_generation_call", "status": "completed", "result": base64.StdEncoding.EncodeToString(images[2])}
				terminal := map[string]any{"id": "resp_invalid_test", "status": "completed", "output": []any{item}}
				if name != "missing created" {
					writeResponsesImageEvent(w, created)
				}
				switch name {
				case "missing sequence":
					delete(partial, "sequence_number")
				case "wrong event name":
					encoded, _ := json.Marshal(partial)
					_, _ = fmt.Fprintf(w, "event: response.wrong\ndata: %s\n\n", encoded)
					return
				case "wrong family":
					partial["type"] = "image_generation.partial_image"
				case "invalid JSON":
					_, _ = fmt.Fprint(w, "data: invalid\n\n")
					return
				case "wrong partial index":
					partial["partial_image_index"] = 2
				case "missing item":
					delete(partial, "item_id")
				case "wrong format":
					partial["output_format"] = "jpeg"
				case "corrupt preview":
					partial["partial_image_b64"] = base64.StdEncoding.EncodeToString([]byte("invalid"))
				}
				writeResponsesImageEvent(w, partial)
				switch name {
				case "changed duplicate":
					partial["sequence_number"] = 2
					partial["partial_image_b64"] = base64.StdEncoding.EncodeToString(images[1])
					writeResponsesImageEvent(w, partial)
				case "changed item":
					partial["sequence_number"] = 2
					partial["item_id"] = "other_item"
					writeResponsesImageEvent(w, partial)
				case "wrong terminal response":
					terminal["id"] = "other_response"
				case "wrong terminal item":
					item["id"] = "other_item"
				case "corrupt terminal":
					item["result"] = base64.StdEncoding.EncodeToString([]byte("invalid"))
				case "provider failure":
					terminal["status"] = "failed"
					writeResponsesImageEvent(w, map[string]any{"type": "response.failed", "sequence_number": 3, "response": terminal})
					return
				case "lost stream":
					return
				}
				writeResponsesImageEvent(w, map[string]any{"type": "response.completed", "sequence_number": 3, "response": terminal})
			}))
			defer upstream.Close()
			client := imageGenerationTestClient(t, upstream, testfixtures.ProviderCatalog(t), "openai")
			input := imageGenerationTestIntent()
			input.Surface, input.ResponsesModel = "responses", "gpt-5"
			input.Stream, input.PartialImages = true, 2
			accepted, err := client.CreateImageGeneration(t.Context(), "invalid-response-stream", input)
			if err != nil {
				t.Fatal(err)
			}
			result := waitForImageGeneration(t, client, accepted)
			expectedState, expectedCode, expectedPolls := proxy.MediaOperationStateFailed, "provider_result_invalid", int32(0)
			if name == "provider failure" {
				expectedCode = "provider_error"
			}
			if name == "lost stream" {
				expectedState, expectedCode, expectedPolls = proxy.MediaOperationStateUncertain, "provider_outcome_unknown", 1
			}
			if result.State != expectedState || result.Error == nil || result.Error.Code != expectedCode || creates.Load() != 1 || polls.Load() != expectedPolls {
				t.Fatalf("result=%+v error=%+v creates=%d polls=%d", result, result.Error, creates.Load(), polls.Load())
			}
		})
	}
}

func TestImageGenerationResponsesClassifiesProviderOutcomesWithoutResubmission(t *testing.T) {
	images := streamingImageFixtures(t)
	for _, scenario := range []struct {
		name   string
		status int
		body   string
		state  string
		code   string
	}{
		{"rate limited", 429, `{}`, "failed", "provider_rate_limited"},
		{"request rejected", 400, `{}`, "failed", "provider_error"},
		{"timeout", 408, `{}`, "uncertain", "provider_outcome_unknown"},
		{"server failure", 503, `{}`, "uncertain", "provider_outcome_unknown"},
		{"invalid JSON", 200, `invalid`, "failed", "provider_result_invalid"},
		{"invalid handle", 200, `{"id":"../native","status":"completed"}`, "failed", "provider_result_invalid"},
		{"invalid status", 200, `{"id":"resp_private","status":"unknown"}`, "failed", "provider_result_invalid"},
		{"failed", 200, `{"id":"resp_private","status":"failed"}`, "failed", "provider_error"},
		{"incomplete", 200, `{"id":"resp_private","status":"incomplete"}`, "failed", "provider_error"},
		{"no image", 200, `{"id":"resp_private","status":"completed","output":[]}`, "failed", "provider_result_invalid"},
		{"corrupt image", 200, `{"id":"resp_private","status":"completed","output":[{"id":"ig_private","type":"image_generation_call","status":"completed","result":"aW52YWxpZA=="}]}`, "failed", "provider_result_invalid"},
		{"image not completed", 200, `{"id":"resp_private","status":"completed","output":[{"id":"ig_private","type":"image_generation_call","status":"in_progress","result":"` + base64.StdEncoding.EncodeToString(images[0]) + `"}]}`, "failed", "provider_result_invalid"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(scenario.status)
				_, _ = fmt.Fprint(w, scenario.body)
			}))
			defer upstream.Close()
			client := imageGenerationTestClient(t, upstream, testfixtures.ProviderCatalog(t), "openai")
			input := imageGenerationTestIntent()
			input.Surface, input.ResponsesModel = "responses", "gpt-5"
			accepted, err := client.CreateImageGeneration(t.Context(), "provider-response-outcome", input)
			if err != nil {
				t.Fatal(err)
			}
			result := waitForImageGeneration(t, client, accepted)
			if result.State != scenario.state || result.Error == nil || result.Error.Code != scenario.code || calls.Load() != 1 {
				t.Fatalf("result=%+v error=%+v calls=%d", result, result.Error, calls.Load())
			}
		})
	}
}
