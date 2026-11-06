package proxy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gopkg.in/yaml.v3"
)

func heygenCanonicalOriginConfiguration(t *testing.T) proxy.Configuration {
	t.Helper()
	document, err := os.ReadFile("../../configs/providers.yml")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := proxy.ParseProviderCatalog(document)
	if err != nil {
		t.Fatal(err)
	}
	document, err = os.ReadFile("../../configs/config.yml")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Server struct {
			Capacity proxy.UpstreamCapacityConfiguration `yaml:"upstream_capacity"`
		} `yaml:"server"`
	}
	if err = yaml.Unmarshal(document, &source); err != nil {
		t.Fatal(err)
	}
	return proxy.Configuration{ProviderCatalog: catalog, UpstreamCapacity: source.Server.Capacity, AssetStorePath: t.TempDir()}
}

// Both checked-in contracts remain unchanged. Only native HTTP transport is injected.
func TestHeyGenCanonicalAvatarVideoOutputOrigins(t *testing.T) {
	for _, origin := range []string{"https://files2.heygen.ai", "https://resource2.heygen.ai", "https://unknown.example", "https://files2.heygen.ai.example", "http://files2.heygen.ai"} {
		t.Run(origin, func(t *testing.T) {
			allowed := origin == "https://files2.heygen.ai" || origin == "https://resource2.heygen.ai"
			var downloads, videoCreates atomic.Int32
			previous := proxy.HTTPClient
			proxy.HTTPClient = coverageHTTPDoer(func(request *http.Request) (*http.Response, error) {
				respond := func(mime, body string) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{mime}}, ContentLength: int64(len(body)), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
				}
				if request.URL.Host != "api.heygen.com" {
					downloads.Add(1)
					if !allowed || request.Method != http.MethodGet || request.URL.Scheme+"://"+request.URL.Host != origin {
						return nil, fmt.Errorf("unexpected artifact transport")
					}
					if request.Header.Get("X-Api-Key") != "" || request.Header.Get("Authorization") != "" {
						t.Error("artifact request contains provider credentials")
					}
					return respond("video/mp4", "canonical-avatar-output")
				}
				if request.Header.Get("X-Api-Key") != "heygen-test-key" {
					t.Error("native provider credential differs")
				}
				switch request.Method + " " + request.URL.Path {
				case "GET /v3/users/me":
					return respond("application/json", `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
				case "POST /v3/assets":
					parts, err := request.MultipartReader()
					if err != nil {
						return nil, err
					}
					part, err := parts.NextPart()
					if err != nil {
						return nil, err
					}
					data, err := io.ReadAll(part)
					if err != nil {
						return nil, err
					}
					mime := part.Header.Get("Content-Type")
					if mime == "audio/wav" {
						mime = "audio/x-wav"
					}
					body, _ := json.Marshal(map[string]any{"data": map[string]any{"asset_id": "native-upload", "mime_type": mime, "size_bytes": len(data), "url": "https://resource2.heygen.ai/input"}})
					return respond("application/json", string(body))
				case "POST /v3/avatars":
					return respond("application/json", `{"data":{"avatar_item":{"id":"native-look","name":"Presenter","avatar_type":"photo_avatar","status":"processing"}}}`)
				case "GET /v3/avatars/looks/native-look":
					return respond("application/json", `{"data":{"id":"native-look","name":"Presenter","avatar_type":"photo_avatar","status":"completed"}}`)
				case "POST /v3/videos":
					videoCreates.Add(1)
					return respond("application/json", `{"data":{"video_id":"native-render","status":"waiting"}}`)
				case "GET /v3/videos/native-render":
					body, _ := json.Marshal(map[string]any{"data": map[string]any{"id": "native-render", "status": "completed", "video_url": origin + "/video.mp4?signature=fixture-private"}})
					return respond("application/json", string(body))
				default:
					return nil, fmt.Errorf("unexpected native protocol route")
				}
			})
			t.Cleanup(func() { proxy.HTTPClient = previous })
			configuration := heygenCanonicalOriginConfiguration(t)
			router := newManagementRouterWithDatabasePath(t, configuration, filepath.Join(t.TempDir(), "canonical-avatar.sqlite"))
			application := httptest.NewServer(router)
			defer application.Close()
			owner := managementSessionCookie(t, "canonical-avatar-owner")
			tenant := managementDefaultTenantTestID(t, router, owner)
			secret := generateManagementTenantSecret(t, router, owner, tenant)
			connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Canonical avatar account", "provider": "heygen", "fields": map[string]string{"api_key": "heygen-test-key"}}, http.StatusCreated)
			accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenant+"/connections/heygen", map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
			config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: application.URL, Secret: secret})
			if err != nil {
				t.Fatal(err)
			}
			client, _ := llmproxyclient.NewClient(config, application.Client())
			image, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "image/png", Data: []byte("photo")})
			if err != nil {
				t.Fatal(err)
			}
			created, err := client.CreateAvatar(t.Context(), "canonical-avatar", llmproxyclient.AvatarCreationInput{Provider: "heygen", Source: llmproxycontract.AvatarCreationSource{ImageAssetID: image.AssetID, Name: "Presenter"}})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			created, err = client.WaitMediaOperation(ctx, created.OperationID, time.Millisecond)
			if err != nil || created.State != "succeeded" {
				t.Fatalf("avatar creation state=%s err=%v", created.State, err)
			}
			metadata, err := client.GetAsset(t.Context(), created.Outputs[0].AssetID)
			if err != nil {
				t.Fatal(err)
			}
			data, err := client.DownloadAsset(t.Context(), metadata)
			if err != nil {
				t.Fatal(err)
			}
			var avatar llmproxycontract.MediaAvatar
			if err = json.Unmarshal(data, &avatar); err != nil {
				t.Fatal(err)
			}
			audio, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: []byte("audio")})
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := client.CreateAvatarVideo(t.Context(), "canonical-avatar-render", llmproxyclient.AvatarVideoInput{Provider: "heygen", Source: llmproxycontract.AvatarVideoSource{AvatarID: avatar.AvatarID, AudioAssetID: audio.AssetID}, Controls: llmproxycontract.AvatarVideoControls{Engine: "avatar_iv", AspectRatio: "9:16", Resolution: "1080p"}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.WaitMediaOperation(ctx, accepted.OperationID, time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			if allowed {
				if result.State != "succeeded" {
					t.Fatalf("canonical catalog/capacity blocked qualified origin: state=%s downloads=%d", result.State, downloads.Load())
				}
				metadata, err = client.GetAsset(t.Context(), result.Outputs[0].AssetID)
				if err != nil {
					t.Fatal(err)
				}
				data, err = client.DownloadAsset(t.Context(), metadata)
				if err != nil || string(data) != "canonical-avatar-output" {
					t.Fatalf("downloaded output differs: %v", err)
				}
				if downloads.Load() != 1 {
					t.Fatalf("artifact transfers=%d", downloads.Load())
				}
			} else if result.State != "uncertain" || downloads.Load() != 0 {
				t.Fatalf("unapproved origin reached transport: state=%s downloads=%d", result.State, downloads.Load())
			}
			if videoCreates.Load() != 1 {
				t.Fatalf("native video submissions=%d", videoCreates.Load())
			}
		})
	}
}

func TestHeyGenCanonicalOriginAllocationContract(t *testing.T) {
	configuration := heygenCanonicalOriginConfiguration(t)
	old := []string{"https://resource.heygen.ai", "https://files.heygen.ai"}
	current := append(append([]string{}, old...), "https://files2.heygen.ai", "https://resource2.heygen.ai")
	for _, provider := range configuration.ProviderCatalog.Schema().Providers {
		if provider.ID != "heygen" {
			continue
		}
		for _, transport := range provider.Transports {
			expected := []string{}
			switch transport.ID {
			case "avatar_video":
				expected = current
			case "lipsync", "translation":
				expected = old
			}
			if len(transport.ArtifactOrigins) != len(expected) {
				t.Fatalf("origin scope changed for transport %s: %v", transport.ID, transport.ArtifactOrigins)
			}
			for _, origin := range expected {
				found := false
				for _, actual := range transport.ArtifactOrigins {
					found = found || actual == origin
				}
				if !found {
					t.Fatalf("transport %s lacks exact origin %s", transport.ID, origin)
				}
			}
		}
	}
	capacity := configuration.UpstreamCapacity
	if capacity.Global != (proxy.UpstreamCapacityLimit{Active: 32, Admitted: 652}) || capacity.Tenant != (proxy.UpstreamCapacityLimit{Active: 8, Admitted: 32}) || capacity.Account != (proxy.UpstreamCapacityLimit{Active: 8, Admitted: 32}) || capacity.Media != (proxy.UpstreamCapacityLimit{Active: 24, Admitted: 256}) || capacity.Status != (proxy.UpstreamCapacityLimit{Active: 8, Admitted: 128}) || capacity.Transfer != (proxy.UpstreamCapacityLimit{Active: 8, Admitted: 128}) || capacity.InteractiveReserve != 1 {
		t.Fatal("canonical capacity limits differ from the exact allocation contract")
	}
	total := 0
	qualified := 0
	for _, rule := range capacity.Origins {
		total += rule.Active + rule.Queued
		if rule.Origin == "https://files2.heygen.ai" || rule.Origin == "https://resource2.heygen.ai" {
			qualified++
			if rule.Active != 4 || rule.Queued != 24 {
				t.Fatal("qualified origin allocation differs")
			}
		}
	}
	if total != 652 || qualified != 2 {
		t.Fatalf("global allocated=%d qualified origins=%d", total, qualified)
	}
}
