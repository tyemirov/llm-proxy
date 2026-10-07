package proxy

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

// Exercise dependency failure after validation through the public execution interface.
func TestHeyGenExecutionBoundaryFaults(t *testing.T) {
	for _, scenario := range []string{"changed-binding", "missing-video-metadata", "missing-video-bytes", "partial-store", "partial-ordinal", "partial-kind", "partial-size", "partial-asset", "partial-read", "partial-publish", "failed-receipt-store", "failed-ordinal", "shutdown-receipt"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newMediaOperationInternalFixture(t)
			if err := fixture.database.AutoMigrate(&managedConnectionFieldRecord{}, &mediaOperationPartialReferenceRecord{}); err != nil {
				t.Fatal(err)
			}
			registry := internalManagementProviderRegistry()
			provider := registry.definitions[providerID("heygen")]
			provider.fields = nil
			registry.definitions[providerID("heygen")] = provider
			tenants := &managedTenantStore{routingDefaults: registry}
			connection := managedAccountConnectionRecord{ID: "gateway-connection", ProviderID: "heygen", Version: 1}
			if err := fixture.database.Create(&connection).Error; err != nil {
				t.Fatal(err)
			}
			if err := fixture.database.Create(&managedTenantConnectionRecord{TenantID: fixture.tenant.identifier.string(), ProviderID: "heygen", ConnectionID: connection.ID}).Error; err != nil {
				t.Fatal(err)
			}
			catalog, _ := NewCatalogService(internalCanonicalProviderCatalog().ModelCatalog())
			operation := ModelOperationVideoLipSync
			if strings.HasPrefix(scenario, "partial") || strings.HasPrefix(scenario, "failed-") {
				operation = ModelOperationVideoTranslation
			}
			route, _ := catalog.ResolveService("heygen", operation)
			adapter := newHeyGenAdapter(route, provider, tenants, fixture.service.store, fixture.service.assets)
			video, err := fixture.service.assets.upload(fixture.tenant, "video/mp4", strings.NewReader("video"))
			if err != nil {
				t.Fatal(err)
			}
			audio, err := fixture.service.assets.upload(fixture.tenant, "audio/wav", strings.NewReader("audio"))
			if err != nil {
				t.Fatal(err)
			}
			input := heygenEncode(llmproxycontract.VideoLipSyncSource{VideoAssetID: video.AssetID, AudioAssetID: audio.AssetID})
			controls := heygenEncode(llmproxycontract.VideoLipSyncControls{VideoProcessingControls: llmproxycontract.VideoProcessingControls{Mode: "speed"}})
			if operation == ModelOperationVideoTranslation {
				input = heygenEncode(llmproxycontract.VideoTranslationSource{VideoAssetID: video.AssetID})
				controls = heygenEncode(llmproxycontract.VideoTranslationControls{VideoProcessingControls: llmproxycontract.VideoProcessingControls{Mode: "speed"}, OutputLanguages: []string{"French", "German"}})
			}
			validated, err := adapter.Validate(t.Context(), MediaOperationAdapterRequest{TenantID: fixture.tenant.identifier.string(), Input: input, Controls: controls})
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("injected persistence failure")
			upload := avatarBoundaryDoer(func(request *http.Request) (*http.Response, error) {
				mime := "video/mp4"
				body, _ := io.ReadAll(request.Body)
				if strings.Contains(string(body), "audio/wav") {
					mime = "audio/wav"
				}
				return avatarBoundaryResponse(`{"data":{"asset_id":"uploaded","url":"https://fixture.test/asset","mime_type":"` + mime + `","size_bytes":5}}`), nil
			})
			request := MediaOperationExecutionRequest{TenantID: fixture.tenant.identifier.string(), Provider: "heygen", CredentialReference: "gateway-connection:v1", ExecutionBinding: validated.ExecutionBinding, OperationID: "mop_22222222222222222222222222222222", Input: validated.Input, Controls: validated.Controls, PersistProviderReceipt: func(MediaOperationProviderReceipt) error { return nil }, PublishPartial: func(MediaOperationPartialOutput) error { return failure }}
			request.HTTP = MediaOperationHTTPClients{Transfer: upload, Submission: avatarBoundaryDoer(func(*http.Request) (*http.Response, error) {
				return avatarBoundaryResponse(`{"data":{"lipsync_id":"job","video_translation_ids":["fr","de"]}}`), nil
			}), Status: avatarBoundaryDoer(func(native *http.Request) (*http.Response, error) {
				id := "fr"
				language := "French"
				if strings.HasSuffix(native.URL.Path, "de") {
					id, language = "de", "German"
				}
				return avatarBoundaryResponse(`{"data":{"id":"` + id + `","status":"completed","output_language":"` + language + `","video_url":"https://fixture.test/video"}}`), nil
			})}
			expected := MediaOperationStateUncertain
			switch scenario {
			case "changed-binding":
				request.ExecutionBinding = "obsolete"
				expected = MediaOperationStateFailed
			case "missing-video-metadata", "missing-video-bytes":
				suffix := ".json"
				if scenario == "missing-video-bytes" {
					suffix = ".data"
				}
				if err := os.Remove(filepath.Join(fixture.service.assets.root, video.AssetID+suffix)); err != nil {
					t.Fatal(err)
				}
				expected = MediaOperationStateFailed
			case "partial-store":
				if err := fixture.database.Migrator().DropTable(&mediaOperationPartialReferenceRecord{}); err != nil {
					t.Fatal(err)
				}
			case "partial-ordinal", "partial-kind", "partial-size", "partial-asset", "partial-read":
				output, err := fixture.service.assets.upload(fixture.tenant, "video/mp4", strings.NewReader("output"))
				if err != nil {
					t.Fatal(err)
				}
				partial := mediaOperationPartialReferenceRecord{OperationID: request.OperationID, TenantID: request.TenantID, OutputOrdinal: 0, PartialOrdinal: 0, AssetID: output.AssetID, MIMEType: "video/mp4", SizeBytes: 6}
				switch scenario {
				case "partial-ordinal":
					partial.OutputOrdinal = 3
				case "partial-kind":
					partial.MIMEType = "audio/wav"
				case "partial-size":
					partial.SizeBytes = fixture.service.assets.maxAssetBytes + 1
				case "partial-asset":
					if err := os.Remove(filepath.Join(fixture.service.assets.root, output.AssetID+".data")); err != nil {
						t.Fatal(err)
					}
				case "partial-read":
					partial.SizeBytes = 5
				}
				if err := fixture.database.Create(&partial).Error; err != nil {
					t.Fatal(err)
				}
			case "failed-receipt-store":
				request.PersistProviderReceipt = func(MediaOperationProviderReceipt) error { return failure }
				request.HTTP.Status = avatarBoundaryDoer(func(*http.Request) (*http.Response, error) {
					return avatarBoundaryResponse(`{"data":{"id":"fr","status":"failed"}}`), nil
				})
			case "failed-ordinal":
				request.ProviderHandle = string(heygenEncode(heygenOperationHandle{IDs: []string{"fr", "de"}, FailedOrdinals: []int{3}}))
			case "partial-publish":
				transport := adapter.provider.transports[route.Transport]
				transport.artifactOrigins = []string{"https://fixture.test"}
				adapter.provider.transports[route.Transport] = transport
				request.HTTP.Transfer = avatarBoundaryDoer(func(native *http.Request) (*http.Response, error) {
					if native.Method == http.MethodPost {
						return upload.Do(native)
					}
					response := avatarBoundaryResponse("output")
					response.ContentLength = 6
					response.Header.Set("Content-Type", "video/mp4")
					return response, nil
				})
			case "shutdown-receipt":
				handle := string(heygenEncode(heygenOperationHandle{FailedOrdinals: []int{}, IDs: []string{"job"}}))
				if !adapter.canRecoverAfterShutdown(adapter.binding, handle) || adapter.canRecoverAfterShutdown("obsolete", handle) {
					t.Fatal("shutdown recovery authority incorrect")
				}
				return
			}
			var result MediaOperationExecutionResult
			if scenario == "failed-ordinal" {
				result = adapter.Recover(t.Context(), request)
			} else if scenario == "failed-receipt-store" {
				request.ProviderHandle = string(heygenEncode(heygenOperationHandle{IDs: []string{"fr", "de"}, FailedOrdinals: []int{}}))
				result = adapter.Recover(t.Context(), request)
			} else if strings.HasPrefix(scenario, "partial") && scenario != "partial-publish" {
				request.ProviderHandle = string(heygenEncode(heygenOperationHandle{FailedOrdinals: []int{}, IDs: []string{"fr", "de"}}))
				result = adapter.Recover(t.Context(), request)
			} else {
				result = adapter.Execute(t.Context(), request)
			}
			if result.State != expected {
				t.Fatalf("result=%+v expected=%s", result, expected)
			}
		})
	}
}

func TestHeyGenServiceSchemaBoundaryFaults(t *testing.T) {
	catalog, _ := NewCatalogService(internalCanonicalProviderCatalog().ModelCatalog())
	route, _ := catalog.ResolveService("heygen", ModelOperationVideoLipSync)
	transport := internalManagementProviderRegistry().definitions[providerID("heygen")].transports[route.Transport]
	for _, scenario := range []string{"limit-count", "limit-unit", "limit-id", "control-bounds", "control-values"} {
		t.Run(scenario, func(t *testing.T) {
			service := route
			service.Limits = append([]CatalogLimit(nil), route.Limits...)
			service.Controls = append([]CatalogControl(nil), route.Controls...)
			switch scenario {
			case "limit-count":
				service.Limits = nil
			case "limit-unit":
				service.Limits[0].Unit = "seconds"
			case "limit-id":
				service.Limits[0].ID = "obsolete"
			case "control-values":
				service.Controls[0].Values = []string{"obsolete"}
			case "control-bounds":
				for i := range service.Controls {
					if service.Controls[i].ID == "start_time" {
						maximum := float64(1)
						service.Controls[i].Maximum = &maximum
					}
				}
			}
			native := ProviderCatalogTransport{Endpoint: transport.endpoint}
			if err := validateHeyGenService(service, native, "fixture"); err == nil {
				t.Fatal("incompatible service accepted")
			}
		})
	}
}
