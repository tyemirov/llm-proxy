package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

// These fixtures inject failures at the adapter's database, file, and HTTP
// boundaries. The public HTTP tests exercise the same flows with real servers.
func imageBoundaryFixture(t *testing.T) (mediaOperationInternalFixture, *imageGenerationAdapter, MediaOperationExecutionRequest) {
	t.Helper()
	f := newMediaOperationInternalFixture(t)
	if err := f.database.AutoMigrate(&managedConnectionFieldRecord{}); err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalogService(internalCanonicalProviderCatalog().ModelCatalog())
	if err != nil {
		t.Fatal(err)
	}
	offering, err := catalog.ResolveOffering("openai", "gpt-image-2")
	if err != nil {
		t.Fatal(err)
	}
	store := &managedTenantStore{routingDefaults: f.service.providers, providerKeyCipher: internalManagedProviderKeyCipher()}
	for _, model := range []any{&managedAccountConnectionRecord{}, &managedTenantConnectionRecord{}} {
		if err := f.database.Model(model).Where("provider_id = ?", ProviderNameXAI).Update("provider_id", ProviderNameOpenAI).Error; err != nil {
			t.Fatal(err)
		}
	}
	key, err := store.providerKeyCipher.encryptConnection(rand.Reader, "connection-internal", "openai", "api_key", "boundary-test-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.database.Create(&managedConnectionFieldRecord{ConnectionID: "connection-internal", FieldID: "api_key", Value: key}).Error; err != nil {
		t.Fatal(err)
	}
	f.service.assets.maxAssetBytes = 1 << 20
	adapter := newImageGenerationAdapter(offering, f.service.providers.definitions[providerID("openai")], store, f.service.store, f.service.assets, catalog)
	request := MediaOperationExecutionRequest{TenantID: f.tenant.identifier.string(), CredentialReference: "connection-internal:v3", Capability: llmproxycontract.MediaCapabilityImageGenerate, Provider: "openai", Model: "gpt-image-2", Input: json.RawMessage(`{"prompt":"boundary test"}`), Controls: json.RawMessage(`{"surface":"responses","responses_model":"gpt-5","quality":"low","size":"auto","background":"opaque","output_format":"png","output_count":1}`), ProviderHandle: "resp_boundary", PersistProviderReceipt: func(MediaOperationProviderReceipt) error { return nil }, PublishPartial: func(MediaOperationPartialOutput) error { return nil }}
	request.ExecutionBinding = adapter.responsesExecutionBinding("gpt-5")
	return f, adapter, request
}

func imageBoundaryPNG(t *testing.T, canvas image.Image) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, canvas); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestImageGenerationAdapterEnforcesAcceptedAuthority(t *testing.T) {
	for _, name := range []string{"route changed", "credential removed", "parent removed"} {
		t.Run(name, func(t *testing.T) {
			f, adapter, request := imageBoundaryFixture(t)
			calls := 0
			request.HTTP.Submission = geminiEdgeDoer(func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })
			switch name {
			case "route changed":
				request.ExecutionBinding = "old-route"
			case "credential removed":
				if err := f.database.Where("tenant_id = ?", request.TenantID).Delete(&managedTenantConnectionRecord{}).Error; err != nil {
					t.Fatal(err)
				}
			case "parent removed":
				request.Input = json.RawMessage(`{"prompt":"follow up","previous_operation_id":"mop_0123456789abcdef0123456789abcdef"}`)
			}
			if result := adapter.Execute(t.Context(), request); result.State != MediaOperationStateFailed || calls != 0 {
				t.Fatalf("result=%+v calls=%d", result, calls)
			}
			if name == "credential removed" {
				if result := adapter.Recover(t.Context(), request); result.State != MediaOperationStateUncertain {
					t.Fatalf("recovery=%+v", result)
				}
				if result := adapter.Cancel(t.Context(), request); result.State != MediaCancellationUnsupported {
					t.Fatalf("cancellation=%+v", result)
				}
			}
		})
	}
}

func TestImageGenerationResponsesTransportAndPersistenceFailures(t *testing.T) {
	for _, name := range []string{"submission lost", "response read", "handle persistence", "stream handle persistence", "poll JSON", "poll wrong handle", "cancellation unconfirmed", "cancelled by provider"} {
		t.Run(name, func(t *testing.T) {
			_, adapter, request := imageBoundaryFixture(t)
			calls := 0
			request.HTTP.Submission = geminiEdgeDoer(func(r *http.Request) (*http.Response, error) {
				calls++
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					return nil, err
				}
				if name == "submission lost" {
					return nil, io.ErrUnexpectedEOF
				}
				body := io.Reader(strings.NewReader(`{"id":"resp_boundary","status":"queued"}`))
				if name == "response read" {
					body = geminiErrorReader{}
				}
				if name == "stream handle persistence" {
					body = strings.NewReader("data: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_boundary\",\"status\":\"queued\"}}\n\n")
				}
				if name == "cancelled by provider" {
					body = strings.NewReader(`{"id":"resp_boundary","status":"cancelled"}`)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(body)}, nil
			})
			request.HTTP.Status = geminiEdgeDoer(func(*http.Request) (*http.Response, error) {
				body := `{"id":"resp_boundary","status":"in_progress"}`
				if name == "poll JSON" {
					body = `invalid`
				}
				if name == "poll wrong handle" {
					body = `{"id":"other","status":"cancelled"}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			if strings.Contains(name, "handle persistence") {
				request.PersistProviderReceipt = func(MediaOperationProviderReceipt) error { return errInternalTestDatabase }
			}
			if name == "stream handle persistence" {
				request.Controls = bytes.Replace(request.Controls, []byte(`"output_count":1`), []byte(`"output_count":1,"stream":true`), 1)
			}
			if name == "cancellation unconfirmed" {
				if result := adapter.Cancel(t.Context(), request); result.State != MediaCancellationUnsupported || calls != 0 {
					t.Fatalf("cancel=%+v submissions=%d", result, calls)
				}
				return
			}
			want := MediaOperationStateUncertain
			if strings.HasPrefix(name, "poll ") {
				want = MediaOperationStateFailed
			}
			if name == "cancelled by provider" {
				want = MediaOperationStateCancelled
			}
			if result := adapter.Execute(t.Context(), request); result.State != want || calls != 1 {
				t.Fatalf("result=%+v submissions=%d", result, calls)
			}
		})
	}
}

type imageFailingWriter struct{ remaining int }

func (w *imageFailingWriter) Write(data []byte) (int, error) {
	w.remaining--
	if w.remaining == 0 {
		return 0, io.ErrClosedPipe
	}
	return len(data), nil
}

func TestImageGenerationRequestWritersPropagateInterruptedTransfers(t *testing.T) {
	f, adapter, request := imageBoundaryFixture(t)
	data := imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 64, 64)))
	asset, err := f.service.assets.upload(f.tenant, "image/png", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	input := imageGenerationInput{Prompt: "edit", ImageAssetIDs: []string{asset.AssetID}}
	var controls imageGenerationControls
	if err := json.Unmarshal(request.Controls, &controls); err != nil {
		t.Fatal(err)
	}
	for write := 1; write <= 6; write++ {
		if err := adapter.writeImageResponsesBody(&imageFailingWriter{remaining: write}, request, input, controls, ""); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("Responses write %d: %v", write, err)
		}
	}
	for write := 1; write <= 2; write++ {
		if err := adapter.writeEditingAsset(multipart.NewWriter(&imageFailingWriter{remaining: write}), request.TenantID, asset.AssetID, "image[]"); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("multipart write %d: %v", write, err)
		}
	}
	if err := adapter.writeEditingForm(multipart.NewWriter(&imageFailingWriter{remaining: 1}), request.TenantID, input, controls); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("multipart field: %v", err)
	}
	compression := 75
	controls.OutputCompression = &compression
	var formBytes bytes.Buffer
	form := multipart.NewWriter(&formBytes)
	if err := adapter.writeEditingForm(form, request.TenantID, input, controls); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	parsed, err := multipart.NewReader(bytes.NewReader(formBytes.Bytes()), form.Boundary()).ReadForm(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.RemoveAll()
	if parsed.Value["output_compression"][0] != "75" {
		t.Fatal("compression not transmitted")
	}
	if err := f.service.assets.delete(f.tenant, asset.AssetID); err != nil {
		t.Fatal(err)
	}
	if err := adapter.writeImageResponsesBody(io.Discard, request, input, controls, ""); err == nil {
		t.Fatal("Responses sent a deleted asset")
	}
	if err := adapter.writeEditingForm(multipart.NewWriter(io.Discard), request.TenantID, input, controls); err == nil {
		t.Fatal("Images sent a deleted asset")
	}
}

func TestImageGenerationEditingAssetFileFailuresAndPaletteMasks(t *testing.T) {
	f, adapter, request := imageBoundaryFixture(t)
	data := imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 64, 64)))
	asset, err := f.service.assets.upload(f.tenant, "image/png", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	validation := MediaOperationAdapterRequest{TenantID: request.TenantID, CredentialReference: request.CredentialReference, Capability: llmproxycontract.MediaCapabilityImageEdit, Controls: request.Controls}
	validate := func(images []string, mask string) error {
		validation.Input, _ = json.Marshal(imageGenerationInput{Prompt: "edit", ImageAssetIDs: images, MaskAssetID: mask})
		_, err := adapter.Validate(t.Context(), validation)
		return err
	}
	// Use Images for its mask contract.
	validation.Controls = bytes.Replace(bytes.Replace(request.Controls, []byte(`"surface":"responses"`), []byte(`"surface":"images"`), 1), []byte(`,"responses_model":"gpt-5"`), nil, 1)
	for _, transparent := range []bool{false, true} {
		palette := color.Palette{color.NRGBA{R: 255, A: 255}, color.NRGBA{G: 255, A: 255}}
		if transparent {
			palette[1] = color.NRGBA{G: 255, A: 0}
		}
		mask := image.NewPaletted(image.Rect(0, 0, 64, 64), palette)
		mask.SetColorIndex(1, 1, 1)
		stored, err := f.service.assets.upload(f.tenant, "image/png", bytes.NewReader(imageBoundaryPNG(t, mask)))
		if err != nil {
			t.Fatal(err)
		}
		if err := validate([]string{asset.AssetID}, stored.AssetID); (err == nil) != transparent {
			t.Fatalf("transparent=%v error=%v", transparent, err)
		}
	}
	invalid, err := f.service.assets.upload(f.tenant, "image/png", strings.NewReader("invalid PNG"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validate([]string{invalid.AssetID}, ""); err == nil {
		t.Fatal("invalid header accepted")
	}
	originalSeek := assetSeek
	t.Cleanup(func() { assetSeek = originalSeek })
	assetSeek = func(file *os.File, _ int64, _ int) (int64, error) { _ = file.Close(); return 0, nil }
	if err := validate([]string{asset.AssetID}, ""); err == nil {
		t.Fatal("read from closed file accepted")
	}
	assetSeek = originalSeek
	if err := os.Remove(f.service.assets.dataPath(asset.AssetID)); err != nil {
		t.Fatal(err)
	}
	if err := validate([]string{asset.AssetID}, ""); err == nil {
		t.Fatal("missing bytes accepted")
	}
}

func TestImageGenerationPreviewPublicationStorageFailures(t *testing.T) {
	for _, name := range []string{"invalid output", "terminal operation", "duplicate changed", "reference query", "asset upload", "reference insert"} {
		t.Run(name, func(t *testing.T) {
			f := newMediaOperationInternalFixture(t)
			record := f.record(MediaOperationStateRunning, MediaProviderExecutionDispatched)
			if err := f.database.Create(&mediaOperationClaimRecord{OperationID: record.OperationID, WorkerID: "preview-worker", Generation: 1, ExpiresAt: f.now.Add(time.Hour)}).Error; err != nil {
				t.Fatal(err)
			}
			output := MediaOperationPartialOutput{MediaOperationOutput: MediaOperationOutput{MIMEType: "image/png", Data: []byte("preview")}}
			switch name {
			case "invalid output":
				output.PartialOrdinal = -1
			case "terminal operation":
				if err := f.database.Model(&record).Update("public_state", MediaOperationStateSucceeded).Error; err != nil {
					t.Fatal(err)
				}
			case "duplicate changed":
				if err := f.service.publishPartial(record.OperationID, 1, output); err != nil {
					t.Fatal(err)
				}
				output.Data = []byte("changed")
			case "reference query":
				failNthMediaGORMOperation(t, f.database, "query", "media_operation_partial_reference_records", 1)
			case "reference insert":
				failNthMediaGORMOperation(t, f.database, "create", "media_operation_partial_reference_records", 1)
			case "asset upload":
				f.service.assets.cleanupError = errAssetStore
				f.service.assets.initialized = true
			}
			if err := f.service.publishPartial(record.OperationID, 1, output); err == nil {
				t.Fatal("publication failure was ignored")
			}
			response, err := f.service.store.publicResponse(t.Context(), record.TenantID, record.OperationID)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if name == "duplicate changed" {
				want = 1
			}
			if len(response.PartialOutputs) != want {
				t.Fatalf("uncommitted previews exposed: %+v", response.PartialOutputs)
			}
		})
	}
}

func TestImageGenerationPreviewReadAndParentRetentionFailures(t *testing.T) {
	t.Run("preview response query", func(t *testing.T) {
		f := newMediaOperationInternalFixture(t)
		failNthMediaGORMOperation(t, f.database, "query", "media_operation_partial_reference_records", 1)
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		body, _ := json.Marshal(f.payload())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/media/operations", bytes.NewReader(body))
		ctx.Request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "preview-query-failure")
		ctx.Set(contextKeyTenant, f.tenant)
		f.service.createHandler()(ctx)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
	t.Run("parent removed during acceptance", func(t *testing.T) {
		f := newMediaOperationInternalFixture(t)
		f.adapter.validateResult.ParentOperationID = newMediaOperationIdentifier()
		if _, _, err := f.service.create(context.Background(), f.tenant, "parent-disappeared", f.payload()); !errors.Is(err, errMediaOperationInvalid) {
			t.Fatalf("parent failure=%v", err)
		}
	})
	for _, name := range []string{"parent lookup", "child count", "preview deletion", "already expired"} {
		t.Run(name, func(t *testing.T) {
			f := newMediaOperationInternalFixture(t)
			record := f.record(MediaOperationStateSucceeded, MediaProviderExecutionSucceeded)
			if err := f.database.Model(&record).Update("terminal_at", f.now.Add(-2*time.Hour)).Error; err != nil {
				t.Fatal(err)
			}
			switch name {
			case "parent lookup":
				failNthMediaGORMOperation(t, f.database, "query", "media_operation_records", 2)
			case "child count":
				failNthMediaGORMOperation(t, f.database, "query", "media_operation_records", 3)
			case "preview deletion":
				failNthMediaGORMOperation(t, f.database, "delete", "media_operation_partial_reference_records", 1)
			case "already expired":
				seen := 0
				if err := f.database.Callback().Query().Before("gorm:query").Register("simulate-concurrent-retention", func(db *gorm.DB) {
					if db.Statement.Table == "media_operation_records" {
						seen++
						if seen == 2 {
							db.AddError(gorm.ErrRecordNotFound)
						}
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			f.service.expireTerminalData()
			if _, err := f.service.store.publicResponse(t.Context(), record.TenantID, record.OperationID); err != nil {
				t.Fatalf("failed retention removed operation: %v", err)
			}
		})
	}
}

func TestImageGenerationCompiledCatalogRequiresEditingRoute(t *testing.T) {
	catalog := internalCanonicalProviderCatalog().ModelCatalog()
	for i := range catalog.Offerings {
		if catalog.Offerings[i].Model == "gpt-image-2" {
			catalog.Offerings[i].ImageRoutes.Editing = ""
		}
	}
	if _, err := NewCatalogService(catalog); err == nil || !strings.Contains(err.Error(), "missing_editing_transport") {
		t.Fatalf("catalog error=%v", err)
	}
}

func TestImageGenerationStreamBoundaryLimitsAndEventIdentity(t *testing.T) {
	imageBytes := imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024)))
	encodedImage := base64.StdEncoding.EncodeToString(imageBytes)
	for _, name := range []string{"images format", "images comments", "images long line", "images multiline", "images total", "responses comments", "responses long line", "responses multiline", "responses total", "provider error", "identical sequence", "changed sequence", "invalid created", "pending event", "invalid pending", "invalid item", "progress events", "invalid progress", "preview publication"} {
		t.Run(name, func(t *testing.T) {
			_, adapter, request := imageBoundaryFixture(t)
			var controls imageGenerationControls
			if err := json.Unmarshal(request.Controls, &controls); err != nil {
				t.Fatal(err)
			}
			controls.Stream, controls.PartialImages = true, 2
			if strings.HasPrefix(name, "images ") {
				controls.Surface, controls.ResponsesModel = "images", ""
			}
			var stream strings.Builder
			event := func(value any) { data, _ := json.Marshal(value); fmt.Fprintf(&stream, "data: %s\n\n", data) }
			created := map[string]any{"type": "response.created", "sequence_number": 0, "response": map[string]any{"id": "resp_boundary", "status": "in_progress"}}
			terminal := map[string]any{"type": "response.completed", "sequence_number": 20, "response": map[string]any{"id": "resp_boundary", "status": "completed", "output": []any{map[string]any{"id": "ig_boundary", "type": "image_generation_call", "status": "completed", "result": encodedImage}}}}
			want := MediaOperationStateFailed
			switch {
			case strings.HasSuffix(name, "long line"):
				adapter.maximumOutputBytes = 128
				stream.WriteString("data: " + strings.Repeat("x", 70000) + "\n\n")
			case strings.HasSuffix(name, "multiline"):
				adapter.maximumOutputBytes = 128
				stream.WriteString(strings.Repeat("data: "+strings.Repeat("x", 40000)+"\n", 2) + "\n")
			case strings.HasSuffix(name, "total"):
				adapter.maximumOutputBytes = 128
				stream.WriteString(strings.Repeat(":"+strings.Repeat("x", 512)+"\n", 2000))
			case controls.Surface == "images":
				format := "jpeg"
				if name == "images comments" {
					stream.WriteString(": keep alive\n\nevent: ignored\n\n")
					format = "png"
					want = MediaOperationStateSucceeded
				}
				event(map[string]any{"type": "image_generation.completed", "output_format": format, "b64_json": encodedImage})
			default:
				if name == "responses comments" {
					stream.WriteString(": keep alive\n\nevent: ignored\n\n")
				}
				if name == "invalid created" {
					created["response"] = map[string]any{"id": "resp_boundary", "status": "completed"}
				}
				event(created)
				switch name {
				case "provider error":
					event(map[string]any{"type": "error"})
				case "identical sequence":
					event(created)
					want = MediaOperationStateSucceeded
				case "changed sequence":
					created["response"] = map[string]any{"id": "resp_boundary", "status": "queued"}
					event(created)
				case "pending event", "invalid pending":
					id := "resp_boundary"
					if name == "invalid pending" {
						id = "other"
					} else {
						want = MediaOperationStateSucceeded
					}
					event(map[string]any{"type": "response.in_progress", "sequence_number": 1, "response": map[string]any{"id": id, "status": "in_progress"}})
				case "invalid item":
					event(map[string]any{"type": "response.output_item.added", "sequence_number": 1, "output_index": -1, "item": map[string]any{"id": "ig_boundary", "type": "image_generation_call"}})
				case "progress events", "invalid progress":
					id := "ig_boundary"
					if name == "invalid progress" {
						id = "../invalid"
					} else {
						want = MediaOperationStateSucceeded
					}
					for index, kind := range []string{"response.image_generation_call.in_progress", "response.image_generation_call.generating", "response.image_generation_call.completed", "response.reasoning_text.delta"} {
						event(map[string]any{"type": kind, "sequence_number": index + 1, "item_id": id, "output_index": 0})
					}
				case "preview publication":
					request.PublishPartial = func(MediaOperationPartialOutput) error { return errAssetStore }
					event(map[string]any{"type": "response.image_generation_call.partial_image", "sequence_number": 1, "item_id": "ig_boundary", "output_index": 0, "partial_image_index": 0, "partial_image_b64": encodedImage})
					want = MediaOperationStateUncertain
				case "responses comments":
					want = MediaOperationStateSucceeded
				}
				event(terminal)
			}
			request.Controls, _ = json.Marshal(controls)
			calls := 0
			request.HTTP.Submission = geminiEdgeDoer(func(r *http.Request) (*http.Response, error) {
				calls++
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					return nil, err
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(stream.String()))}, nil
			})
			if result := adapter.Execute(t.Context(), request); result.State != want || calls != 1 {
				t.Fatalf("result=%+v submissions=%d", result, calls)
			}
		})
	}
}
