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

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

func TestHeyGenAvatarCreationAndRendering(t *testing.T) {
	for _, provider := range []string{"heygen", "heygen-fixture"} {
		t.Run(provider, func(t *testing.T) {
			var submissions atomic.Int32
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/render.mp4" {
					if r.Header.Get("X-Api-Key") != "" || r.Header.Get("Authorization") != "" {
						t.Error("artifact credential leak")
					}
					w.Header().Set("Content-Type", "video/mp4")
					_, _ = io.WriteString(w, "avatar-video")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /v3/users/me":
					_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
				case "POST /v3/assets":
					if err := r.ParseMultipartForm(2 << 20); err != nil {
						t.Error(err)
					}
					defer r.MultipartForm.RemoveAll()
					file, header, err := r.FormFile("file")
					if err != nil {
						t.Error(err)
						return
					}
					data, _ := io.ReadAll(file)
					file.Close()
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"asset_id": "native-upload", "mime_type": header.Header.Get("Content-Type"), "size_bytes": len(data), "url": upstream.URL + "/asset"}})
				case "POST /v3/avatars":
					submissions.Add(1)
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["type"] != "photo" || body["name"] != "Presenter" || body["file"].(map[string]any)["asset_id"] != "native-upload" {
						t.Errorf("avatar body=%v", body)
					}
					_, _ = io.WriteString(w, `{"data":{"avatar_item":{"id":"native-look","name":"Presenter","avatar_type":"photo_avatar","status":"processing"}}}`)
				case "GET /v3/avatars/looks/native-look":
					_, _ = io.WriteString(w, `{"data":{"id":"native-look","name":"Presenter","avatar_type":"photo_avatar","status":"completed"}}`)
				case "POST /v3/videos":
					submissions.Add(1)
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["avatar_id"] != "native-look" || body["audio_asset_id"] != "native-upload" || body["type"] != "avatar" || body["engine"].(map[string]any)["type"] != "avatar_iv" || body["motion_prompt"] != "Wave" {
						t.Errorf("video body=%v", body)
					}
					_, _ = io.WriteString(w, `{"data":{"video_id":"native-render","status":"waiting"}}`)
				case "GET /v3/videos/native-render":
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "native-render", "status": "completed", "video_url": upstream.URL + "/render.mp4"}})
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			configuration := proxy.Configuration{UpstreamCapacity: testfixtures.UpstreamCapacity(4, 100), ProviderCatalog: heygenTestCatalog(t, provider, upstream.URL), AssetStorePath: t.TempDir()}
			databasePath := filepath.Join(t.TempDir(), "avatars.db")
			router := newManagementRouterWithDatabasePath(t, configuration, databasePath)
			server := httptest.NewServer(router)
			defer server.Close()
			owner := managementSessionCookie(t, "avatar-owner")
			tenant := managementDefaultTenantTestID(t, router, owner)
			secret := generateManagementTenantSecret(t, router, owner, tenant)
			field := "api_key"
			if provider != "heygen" {
				field = "fixture_token"
			}
			connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Avatar account", "provider": provider, "fields": map[string]string{field: "heygen-secret"}}, http.StatusCreated)
			accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenant+"/connections/"+provider, map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
			cfg, _ := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
			client, _ := llmproxyclient.NewClient(cfg, server.Client())
			image, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "image/png", Data: []byte("photo")})
			if err != nil {
				t.Fatal(err)
			}
			wait := func(op llmproxyclient.MediaOperation) llmproxyclient.MediaOperation {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				result, err := client.WaitMediaOperation(ctx, op.OperationID, 5*time.Millisecond)
				if err != nil || result.State != "succeeded" {
					t.Fatalf("operation=%+v err=%v", result, err)
				}
				return result
			}
			input := llmproxyclient.MediaOperationInput{Capability: "avatar.create", Provider: provider, Input: json.RawMessage(fmt.Sprintf(`{"name":"Presenter","image_asset_id":%q}`, image.AssetID)), Controls: json.RawMessage(`{}`)}
			created, err := client.CreateMediaOperation(t.Context(), "avatar-create", input)
			if err != nil {
				t.Fatal(err)
			}
			created = wait(created)
			metadata, err := client.GetAsset(t.Context(), created.Outputs[0].AssetID)
			if err != nil {
				t.Fatal(err)
			}
			data, err := client.DownloadAsset(t.Context(), metadata)
			if err != nil {
				t.Fatal(err)
			}
			var avatar llmproxycontract.MediaAvatar
			if err := json.Unmarshal(data, &avatar); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(avatar.AvatarID, "ava_") {
				t.Fatalf("avatar=%+v", avatar)
			}
			repeated, err := client.CreateMediaOperation(t.Context(), "avatar-create", input)
			if err != nil || repeated.OperationID != created.OperationID {
				t.Fatalf("duplicate=%+v err=%v", repeated, err)
			}
			database, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := database.Table("media_operation_records").Where("operation_id = ?", created.OperationID).Update("terminal_at", time.Now().Add(-72*time.Hour)).Error; err != nil {
				t.Fatal(err)
			}
			server.Close()
			router = newManagementRouterWithDatabasePath(t, configuration, databasePath)
			server = httptest.NewServer(router)
			defer server.Close()
			cfg, _ = llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
			client, _ = llmproxyclient.NewClient(cfg, server.Client())
			if _, err := client.GetMediaOperation(t.Context(), created.OperationID); err == nil {
				t.Fatal("expired creation operation remained available")
			}
			audio, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: metaTranscriptionWAV(16000, 1)})
			if err != nil {
				t.Fatal(err)
			}
			controls := json.RawMessage(`{"engine":"avatar_iv","aspect_ratio":"16:9","resolution":"720p","motion_prompt":"Wave","expressiveness":"medium"}`)
			render := llmproxyclient.MediaOperationInput{Capability: "avatar.video.generate", Provider: provider, Input: json.RawMessage(fmt.Sprintf(`{"avatar_id":%q,"audio_asset_id":%q}`, avatar.AvatarID, audio.AssetID)), Controls: controls}
			accepted, err := client.CreateMediaOperation(t.Context(), "avatar-render", render)
			if err != nil {
				t.Fatal(err)
			}
			completed := wait(accepted)
			metadata, err = client.GetAsset(t.Context(), completed.Outputs[0].AssetID)
			if err != nil {
				t.Fatal(err)
			}
			data, err = client.DownloadAsset(t.Context(), metadata)
			if err != nil || string(data) != "avatar-video" {
				t.Fatalf("data=%q err=%v", data, err)
			}
			foreignTenant := createManagementTenant(t, router, owner, "Foreign avatar tenant").Tenant.ID
			accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+foreignTenant+"/connections/"+provider, map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
			foreignSecret := generateManagementTenantSecret(t, router, owner, foreignTenant)
			foreignCfg, _ := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: foreignSecret})
			foreignClient, _ := llmproxyclient.NewClient(foreignCfg, server.Client())
			foreignAudio, err := foreignClient.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: metaTranscriptionWAV(16000, 1)})
			if err != nil {
				t.Fatal(err)
			}
			foreignRender := render
			foreignRender.Input = json.RawMessage(fmt.Sprintf(`{"avatar_id":%q,"audio_asset_id":%q}`, avatar.AvatarID, foreignAudio.AssetID))
			if _, err := foreignClient.CreateMediaOperation(t.Context(), "foreign-avatar", foreignRender); err == nil {
				t.Fatal("foreign avatar accepted")
			}
			replacement := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Replacement", "provider": provider, "fields": map[string]string{field: "replacement-key"}}, http.StatusCreated)
			accountConnectionExchange(t, router, owner, http.MethodDelete, "/tenants/"+tenant+"/connections/"+provider, nil, http.StatusNoContent)
			accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenant+"/connections/"+provider, map[string]string{"kind": "account_connection", "resource_id": replacement["id"].(string)}, http.StatusOK)
			if _, err := client.CreateMediaOperation(t.Context(), "invalid-credential", render); err == nil {
				t.Fatal("avatar accepted under changed credential")
			}
			render.Input = json.RawMessage(fmt.Sprintf(`{"avatar_id":"native-look","audio_asset_id":%q}`, audio.AssetID))
			if _, err := client.CreateMediaOperation(t.Context(), "invalid-native", render); err == nil {
				t.Fatal("native id accepted")
			}
			if submissions.Load() != 2 {
				t.Fatalf("submissions=%d", submissions.Load())
			}
		})
	}
}

func TestHeyGenAvatarProviderFailures(t *testing.T) {
	for _, scenario := range []string{"upload-rejected", "submit-rejected", "submission-malformed", "missing-look", "status-error", "status-malformed", "wrong-look", "wrong-type", "failed", "unknown-state"} {
		t.Run(scenario, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /v3/users/me":
					_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
				case "POST /v3/assets":
					if scenario == "upload-rejected" {
						w.WriteHeader(400)
						return
					}
					heygenFixtureUpload(t, w, r)
				case "POST /v3/avatars":
					if scenario == "submit-rejected" {
						w.WriteHeader(400)
						return
					}
					if scenario == "submission-malformed" {
						_, _ = io.WriteString(w, `{`)
						return
					}
					if scenario == "missing-look" {
						_, _ = io.WriteString(w, `{"data":{}}`)
						return
					}
					_, _ = io.WriteString(w, `{"data":{"avatar_item":{"id":"accepted-look"}}}`)
				case "GET /v3/avatars/looks/accepted-look":
					if scenario == "status-error" {
						w.WriteHeader(503)
						return
					}
					if scenario == "status-malformed" {
						_, _ = io.WriteString(w, `{`)
						return
					}
					id, status, kind := "accepted-look", "completed", "photo_avatar"
					if scenario == "wrong-look" {
						id = "other"
					}
					if scenario == "wrong-type" {
						kind = "digital_twin"
					}
					if scenario == "failed" {
						status = "failed"
					}
					if scenario == "unknown-state" {
						status = "obsolete"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id, "status": status, "avatar_type": kind}})
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			t.Cleanup(upstream.Close)
			client, _, _ := heygenDurableClient(t, upstream)
			image, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "image/png", Data: []byte("image")})
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := client.CreateAvatar(t.Context(), "fault", llmproxyclient.AvatarCreationInput{Provider: "heygen", Source: llmproxycontract.AvatarCreationSource{Name: "Presenter", ImageAssetID: image.AssetID}})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			result, err := client.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
			expected := "uncertain"
			if scenario == "submit-rejected" || scenario == "failed" {
				expected = "failed"
			}
			if err != nil || result.State != expected {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestHeyGenAvatarReceiptRecoveryAndCancellation(t *testing.T) {
	for _, change := range []string{"none", "missing-handle", "changed-binding"} {
		t.Run(change, func(t *testing.T) {
			var submissions, polls atomic.Int32
			started, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET /v3/users/me":
					_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
				case "POST /v3/assets":
					heygenFixtureUpload(t, w, r)
				case "POST /v3/avatars":
					submissions.Add(1)
					_, _ = io.WriteString(w, `{"data":{"avatar_item":{"id":"retained-look"}}}`)
				case "GET /v3/avatars/looks/retained-look":
					if polls.Add(1) == 1 {
						close(started)
						select {
						case <-release:
						case <-r.Context().Done():
							return
						}
						w.WriteHeader(503)
						return
					}
					_, _ = io.WriteString(w, `{"data":{"id":"retained-look","status":"completed","avatar_type":"photo_avatar"}}`)
				default:
					t.Errorf("unexpected %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			t.Cleanup(upstream.Close)
			client, database, restart := heygenDurableClient(t, upstream)
			image, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "image/png", Data: []byte("photo")})
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := client.CreateAvatar(t.Context(), "recovery", llmproxyclient.AvatarCreationInput{Provider: "heygen", Source: llmproxycontract.AvatarCreationSource{Name: "Presenter", ImageAssetID: image.AssetID}})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("avatar not polled")
			}
			cancelled, err := client.CancelMediaOperation(t.Context(), accepted.OperationID)
			if err != nil || cancelled.CancellationState != "unsupported" {
				t.Fatalf("cancel=%+v err=%v", cancelled, err)
			}
			if err := database.Exec("UPDATE media_operation_claim_records SET generation = generation + 1, expires_at = ? WHERE operation_id = ?", time.Now().Add(-time.Second), accepted.OperationID).Error; err != nil {
				t.Fatal(err)
			}
			if change != "none" {
				column, value := "provider_handle", ""
				if change == "changed-binding" {
					column, value = "execution_binding", "obsolete"
				}
				if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Update(column, value).Error; err != nil {
					t.Fatal(err)
				}
			}
			client = restart()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			result, err := client.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
			expected := "succeeded"
			if change != "none" {
				expected = "uncertain"
			}
			if err != nil || result.State != expected || submissions.Load() != 1 {
				t.Fatalf("recovered=%+v err=%v submissions=%d", result, err, submissions.Load())
			}
		})
	}
}

func TestHeyGenAvatarFinalCommitFailureKeepsAcceptedIntent(t *testing.T) {
	var submissions atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /v3/users/me":
			_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
		case "POST /v3/assets":
			heygenFixtureUpload(t, w, r)
		case "POST /v3/avatars":
			submissions.Add(1)
			_, _ = io.WriteString(w, `{"data":{"avatar_item":{"id":"persisted-provider-look"}}}`)
		case "GET /v3/avatars/looks/persisted-provider-look":
			_, _ = io.WriteString(w, `{"data":{"id":"persisted-provider-look","status":"completed","avatar_type":"photo_avatar"}}`)
		default:
			t.Errorf("unexpected native request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)
	client, database, _ := heygenDurableClient(t, upstream)
	if err := database.Exec(`CREATE TRIGGER reject_avatar_commit BEFORE INSERT ON media_avatar_records BEGIN SELECT RAISE(ABORT, 'injected avatar persistence failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	image, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "image/png", Data: []byte("photo")})
	if err != nil {
		t.Fatal(err)
	}
	input := llmproxyclient.AvatarCreationInput{Provider: "heygen", Source: llmproxycontract.AvatarCreationSource{Name: "Presenter", ImageAssetID: image.AssetID}}
	accepted, err := client.CreateAvatar(t.Context(), "avatar-final-commit-fault", input)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	result, err := client.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
	if err != nil || result.State != proxy.MediaOperationStateUncertain || len(result.Outputs) != 0 || result.Error == nil || result.Error.Code != "provider_result_invalid" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	var retained int64
	if err := database.Table("media_avatar_records").Count(&retained).Error; err != nil || retained != 0 {
		t.Fatalf("retained avatars=%d err=%v", retained, err)
	}
	if err := database.Exec("DROP TRIGGER reject_avatar_commit").Error; err != nil {
		t.Fatal(err)
	}
	repeated, err := client.CreateAvatar(t.Context(), "avatar-final-commit-fault", input)
	if err != nil || repeated.OperationID != accepted.OperationID || repeated.State != proxy.MediaOperationStateUncertain || len(repeated.Outputs) != 0 || submissions.Load() != 1 {
		t.Fatalf("duplicate=%+v err=%v submissions=%d", repeated, err, submissions.Load())
	}
}

func TestHeyGenAvatarCatalogRejectsIncompleteServiceComposition(t *testing.T) {
	for _, operation := range []string{proxy.ModelOperationAvatarCreation, proxy.ModelOperationAvatarVideoGeneration} {
		t.Run(operation, func(t *testing.T) {
			data, err := os.ReadFile("../../configs/providers.yml")
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := yaml.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			changed := false
			for _, raw := range document["providers"].([]any) {
				provider := raw.(map[string]any)
				if provider["id"] != "heygen" {
					continue
				}
				for _, rawService := range provider["services"].([]any) {
					service := rawService.(map[string]any)
					if service["operation"] != operation {
						continue
					}
					service["limits"] = service["limits"].([]any)[:1]
					changed = true
				}
			}
			if !changed {
				t.Fatal("canonical catalog omits avatar service")
			}
			invalid, err := yaml.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := proxy.ParseProviderCatalog(invalid); err == nil || !strings.Contains(err.Error(), "reason=heygen_avatar_composition") {
				t.Fatalf("incomplete avatar service admitted or wrong validation error: %v", err)
			}
		})
	}
}
