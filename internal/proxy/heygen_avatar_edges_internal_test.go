package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

type avatarBoundaryDoer func(*http.Request) (*http.Response, error)

func (doer avatarBoundaryDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}
func avatarBoundaryResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

// Inject failures at the adapter's public execution boundary while SQLite and asset I/O stay real.
func TestHeyGenAvatarAdapterBoundaryFailures(t *testing.T) {
	fixture := newMediaOperationInternalFixture(t)
	if err := fixture.database.AutoMigrate(&mediaAvatarRecord{}, &managedConnectionFieldRecord{}); err != nil {
		t.Fatal(err)
	}
	registry := internalManagementProviderRegistry()
	provider := registry.definitions[providerID("heygen")]
	provider.fields = nil
	registry.definitions[providerID("heygen")] = provider
	tenants := &managedTenantStore{routingDefaults: registry}
	connection := managedAccountConnectionRecord{ID: "avatar-connection", ProviderID: "heygen", Version: 1}
	if err := fixture.database.Create(&connection).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.Create(&managedTenantConnectionRecord{TenantID: fixture.tenant.identifier.string(), ProviderID: "heygen", ConnectionID: connection.ID}).Error; err != nil {
		t.Fatal(err)
	}
	catalog, _ := NewCatalogService(internalCanonicalProviderCatalog().ModelCatalog())
	route, _ := catalog.ResolveService("heygen", ModelOperationAvatarCreation)
	creation := newProviderHeyGenAvatarAdapter(route, provider, tenants, fixture.service.store, fixture.service.assets)
	videoRoute, _ := catalog.ResolveService("heygen", ModelOperationAvatarVideoGeneration)
	video := newProviderHeyGenAvatarAdapter(videoRoute, provider, tenants, fixture.service.store, fixture.service.assets)
	image, err := fixture.service.assets.upload(fixture.tenant, "image/png", strings.NewReader("photo"))
	if err != nil {
		t.Fatal(err)
	}
	audio, err := fixture.service.assets.upload(fixture.tenant, "audio/wav", strings.NewReader("audio"))
	if err != nil {
		t.Fatal(err)
	}
	public := llmproxycontract.MediaAvatar{AvatarID: "ava_11111111111111111111111111111111", Name: "Presenter", Provider: "heygen"}
	record := mediaAvatarRecord{AvatarID: public.AvatarID, TenantID: fixture.tenant.identifier.string(), Provider: "heygen", CredentialReference: "avatar-connection:v1", NativeLookID: "retained", Metadata: heygenEncode(public)}
	if err := fixture.database.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected boundary failure")
	upload := avatarBoundaryDoer(func(request *http.Request) (*http.Response, error) {
		mime := "image/png"
		if strings.Contains(request.Header.Get("Content-Type"), "multipart") {
			data, _ := io.ReadAll(request.Body)
			if strings.Contains(string(data), "audio/wav") {
				mime = "audio/wav"
			}
		}
		return avatarBoundaryResponse(`{"data":{"asset_id":"upload","url":"https://example.test/asset","mime_type":"` + mime + `","size_bytes":5}}`), nil
	})
	submission := avatarBoundaryDoer(func(*http.Request) (*http.Response, error) {
		return avatarBoundaryResponse(`{"data":{"avatar_item":{"id":"look"},"video_id":"render"}}`), nil
	})
	status := avatarBoundaryDoer(func(*http.Request) (*http.Response, error) {
		return avatarBoundaryResponse(`{"data":{"id":"look","status":"completed","avatar_type":"photo_avatar"}}`), nil
	})
	base := MediaOperationExecutionRequest{TenantID: fixture.tenant.identifier.string(), Provider: "heygen", CredentialReference: "avatar-connection:v1", ExecutionBinding: creation.binding, OperationID: "mop_22222222222222222222222222222222", Input: heygenEncode(llmproxycontract.AvatarCreationSource{Name: "Presenter", ImageAssetID: image.AssetID}), Controls: heygenEncode(struct{}{}), HTTP: MediaOperationHTTPClients{Submission: submission, Status: status, Transfer: upload}, PersistProviderReceipt: func(MediaOperationProviderReceipt) error { return failure }}
	for _, scenario := range []string{"missing-connection", "submission-network", "receipt-store", "video-lookup-race", "video-upload"} {
		t.Run(scenario, func(t *testing.T) {
			request := base
			adapter := creation
			expected := MediaOperationStateUncertain
			switch scenario {
			case "missing-connection":
				request.CredentialReference = "missing"
				expected = MediaOperationStateFailed
			case "submission-network":
				request.HTTP.Submission = avatarBoundaryDoer(func(*http.Request) (*http.Response, error) { return nil, failure })
			case "video-lookup-race", "video-upload":
				adapter = video
				request.ExecutionBinding = video.binding
				avatarID := public.AvatarID
				if scenario == "video-lookup-race" {
					avatarID = "ava_33333333333333333333333333333333"
					expected = MediaOperationStateFailed
				}
				request.Input = heygenEncode(llmproxycontract.AvatarVideoSource{AvatarID: avatarID, AudioAssetID: audio.AssetID})
				request.HTTP.Transfer = avatarBoundaryDoer(func(*http.Request) (*http.Response, error) { return nil, failure })
			}
			result := adapter.Execute(t.Context(), request)
			if result.State != expected {
				t.Fatalf("result=%+v", result)
			}
		})
	}
	if creation.canRecoverAfterShutdown(creation.binding, "look") != true || creation.canRecoverAfterShutdown("obsolete", "look") {
		t.Fatal("shutdown recovery authority incorrect")
	}
	for _, adapter := range []*providerHeyGenAvatarAdapter{creation, video} {
		request := MediaOperationAdapterRequest{TenantID: fixture.tenant.identifier.string(), Provider: "heygen", CredentialReference: "avatar-connection:v1", Input: []byte(`{}`), Controls: []byte(`{}`)}
		if _, err := adapter.Validate(t.Context(), request); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	for _, scenario := range []string{"status-network", "download-network", "gateway-output-cap", "wrong-output-kind", "wait-cancel", "wait-progress"} {
		t.Run(scenario, func(t *testing.T) {
			request := base
			request.ProviderHandle = "look"
			resolved := provider
			resolved.textEndpointURL = "https://example.test/v3/avatars"
			ctx := t.Context()
			adapter := creation
			switch scenario {
			case "status-network":
				request.HTTP.Status = avatarBoundaryDoer(func(*http.Request) (*http.Response, error) { return nil, failure })
			case "download-network":
				adapter = video
				request.ProviderHandle = "render"
				request.HTTP.Status = avatarBoundaryDoer(func(*http.Request) (*http.Response, error) {
					return avatarBoundaryResponse(`{"data":{"id":"render","status":"completed","video_url":"https://foreign.invalid/video"}}`), nil
				})
			case "gateway-output-cap", "wrong-output-kind":
				adapter = video
				request.ProviderHandle = "render"
				origin := "https://example.test"
				definition := adapter.provider.transports[adapter.route.Transport]
				previous := definition.artifactOrigins
				definition.artifactOrigins = []string{origin}
				adapter.provider.transports[adapter.route.Transport] = definition
				defer func() {
					definition.artifactOrigins = previous
					adapter.provider.transports[adapter.route.Transport] = definition
				}()
				request.HTTP.Status = avatarBoundaryDoer(func(*http.Request) (*http.Response, error) {
					return avatarBoundaryResponse(`{"data":{"id":"render","status":"completed","video_url":"https://example.test/video"}}`), nil
				})
				request.HTTP.Transfer = avatarBoundaryDoer(func(*http.Request) (*http.Response, error) {
					size := 33
					if scenario == "wrong-output-kind" {
						size = 1
					}
					response := avatarBoundaryResponse(strings.Repeat("v", size))
					response.ContentLength = int64(size)
					response.Header.Set("Content-Type", "video/mp4")
					if scenario == "wrong-output-kind" {
						response.Header.Set("Content-Type", "audio/wav")
					}
					return response, nil
				})
			case "wait-cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				request.HTTP.Status = avatarBoundaryDoer(func(*http.Request) (*http.Response, error) {
					cancel()
					return avatarBoundaryResponse(`{"data":{"id":"look","status":"processing"}}`), nil
				})
			case "wait-progress":
				calls := 0
				request.HTTP.Status = avatarBoundaryDoer(func(*http.Request) (*http.Response, error) {
					calls++
					if calls == 1 {
						return avatarBoundaryResponse(`{"data":{"id":"look","status":"processing"}}`), nil
					}
					return avatarBoundaryResponse(`{"data":{"id":"look","status":"failed"}}`), nil
				})
			}
			result := adapter.poll(ctx, request, resolved)
			expected := MediaOperationStateUncertain
			if scenario == "wait-progress" {
				expected = MediaOperationStateFailed
			}
			if result.State != expected {
				t.Fatalf("result=%+v", result)
			}
		})
	}
	for _, scenario := range []string{"path", "limit", "controls"} {
		t.Run(scenario, func(t *testing.T) {
			service := videoRoute
			service.Limits = append([]CatalogLimit(nil), service.Limits...)
			service.Controls = append([]CatalogControl(nil), service.Controls...)
			native := ProviderCatalogTransport{}
			native.Endpoint.Path = "/v3/videos"
			switch scenario {
			case "path":
				native.Endpoint.Path = "/obsolete"
			case "limit":
				service.Limits[0].ID = "obsolete"
			case "controls":
				service.Controls[0].Values = []string{"obsolete"}
			}
			if validateHeyGenAvatarService(service, native, "fixture") == nil {
				t.Fatal("invalid catalog admitted")
			}
		})
	}
}
