package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
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
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"gopkg.in/yaml.v3"
)

func heygenTestCatalog(t *testing.T, provider, origin string) *proxy.ProviderCatalog {
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
		if entry["id"] != "heygen" {
			continue
		}
		found = true
		entry["id"] = provider
		for _, value := range entry["transports"].([]any) {
			transport := value.(map[string]any)
			transport["endpoint"].(map[string]any)["default_base_url"] = origin
			if _, ok := transport["artifact_origins"]; ok {
				transport["artifact_origins"] = []string{origin}
			}
			if provider != "heygen" {
				transport["components"].(map[string]any)["authentication"] = map[string]any{"kind": "bearer", "field": "fixture_token", "header": "Authorization", "prefix": "Bearer "}
			}
		}
		if provider != "heygen" {
			entry["fields"].([]any)[0].(map[string]any)["id"] = "fixture_token"
		}
	}
	if !found {
		t.Fatal("canonical catalog omits HeyGen v3 model-free services")
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

func TestHeyGenV3ServicesMaterializeOrderedResults(t *testing.T) {
	for _, provider := range []string{"heygen", "heygen-fixture"} {
		t.Run(provider, func(t *testing.T) {
			var submits, uploads atomic.Int32
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/files/") {
					if r.Header.Get("X-Api-Key") != "" || r.Header.Get("Authorization") != "" {
						t.Error("artifact leaked credentials")
					}
					w.Header().Set("Content-Type", "video/mp4")
					_, _ = io.WriteString(w, "video:"+r.URL.Path)
					return
				}
				if provider == "heygen" && r.Header.Get("X-Api-Key") != "heygen-secret" || provider != "heygen" && r.Header.Get("Authorization") != "Bearer heygen-secret" {
					t.Error("incorrect provider authentication")
					w.WriteHeader(401)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /v3/users/me":
					_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
				case "POST /v3/assets":
					uploads.Add(1)
					if err := r.ParseMultipartForm(2 << 20); err != nil {
						t.Error(err)
						return
					}
					defer r.MultipartForm.RemoveAll()
					file, header, err := r.FormFile("file")
					if err != nil {
						t.Error(err)
						return
					}
					defer file.Close()
					data, _ := io.ReadAll(file)
					if len(data) == 0 {
						t.Error("empty upload")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"asset_id": fmt.Sprintf("native-asset-%d", uploads.Load()), "url": upstream.URL + "/private", "mime_type": header.Header.Get("Content-Type"), "size_bytes": len(data)}})
				case "POST /v3/lipsyncs", "POST /v3/video-translations":
					submits.Add(1)
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["model"] != nil || body["video"].(map[string]any)["type"] != "asset_id" {
						t.Errorf("invalid native body=%v", body)
					}
					if r.URL.Path == "/v3/lipsyncs" {
						_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"lipsync_id": "lip-1"}})
					} else {
						_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"video_translation_ids": []string{"trans-fr", "trans-de"}}})
					}
				case "GET /v3/lipsyncs/lip-1":
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "lip-1", "status": "completed", "video_url": upstream.URL + "/files/lip.mp4"}})
				case "GET /v3/video-translations/trans-fr", "GET /v3/video-translations/trans-de":
					language := "French"
					if strings.HasSuffix(r.URL.Path, "trans-de") {
						language = "German"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": strings.TrimPrefix(r.URL.Path, "/v3/video-translations/"), "status": "completed", "output_language": language, "video_url": upstream.URL + "/files/" + language + ".mp4"}})
				default:
					t.Errorf("unexpected upstream %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			configuration := proxy.Configuration{ProviderCatalog: heygenTestCatalog(t, provider, upstream.URL), AssetStorePath: t.TempDir(), UpstreamCapacity: testfixtures.UpstreamCapacity(4, 100)}
			databasePath := filepath.Join(t.TempDir(), "usage.sqlite")
			router := newTokenPresenceRouter(t, configuration, databasePath)
			server := httptest.NewServer(router)
			defer server.Close()
			owner := managementSessionCookie(t, "heygen-owner")
			tenant := managementDefaultTenantTestID(t, router, owner)
			secret := generateManagementTenantSecret(t, router, owner, tenant)
			field := "api_key"
			if provider != "heygen" {
				field = "fixture_token"
			}
			connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Video account", "provider": provider, "fields": map[string]string{field: "heygen-secret"}}, http.StatusCreated)
			accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenant+"/connections/"+provider, map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
			cfg, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
			if err != nil {
				t.Fatal(err)
			}
			client, err := llmproxyclient.NewClient(cfg, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			video, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "video/mp4", Data: []byte("input-video")})
			if err != nil {
				t.Fatal(err)
			}
			audio, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: metaTranscriptionWAV(16000, 1)})
			if err != nil {
				t.Fatal(err)
			}
			account, err := client.GetProviderAccount(t.Context(), provider)
			if err != nil || account.BillingType != "wallet" || account.Wallet == nil || account.Wallet.Currency != "usd" || account.Wallet.RemainingBalance == nil || *account.Wallet.RemainingBalance != 20 {
				t.Fatalf("account=%+v error=%v", account, err)
			}
			cases := []struct {
				capability, input, controls string
				outputs                     []string
			}{
				{"video.lipsync", fmt.Sprintf(`{"video_asset_id":%q,"audio_asset_id":%q}`, video.AssetID, audio.AssetID), `{"mode":"speed"}`, []string{"video:/files/lip.mp4"}},
				{"video.translate", fmt.Sprintf(`{"video_asset_id":%q}`, video.AssetID), `{"mode":"precision","output_languages":["French","German"]}`, []string{"video:/files/French.mp4", "video:/files/German.mp4"}},
			}
			for index, tc := range cases {
				input := llmproxyclient.MediaOperationInput{Capability: tc.capability, Provider: provider, Input: json.RawMessage(tc.input), Controls: json.RawMessage(tc.controls)}
				key := fmt.Sprintf("heygen-%d", index)
				accepted, err := client.CreateMediaOperation(t.Context(), key, input)
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				completed, err := client.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
				cancel()
				if err != nil || completed.State != proxy.MediaOperationStateSucceeded || len(completed.Outputs) != len(tc.outputs) {
					t.Fatalf("result=%+v err=%v", completed, err)
				}
				for ordinal, out := range completed.Outputs {
					asset, err := client.GetAsset(t.Context(), out.AssetID)
					if err != nil {
						t.Fatal(err)
					}
					data, err := client.DownloadAsset(t.Context(), asset)
					if err != nil || string(data) != tc.outputs[ordinal] {
						t.Fatalf("artifact=%q error=%v", data, err)
					}
				}
				repeated, err := client.CreateMediaOperation(t.Context(), key, input)
				if err != nil || repeated.OperationID != accepted.OperationID {
					t.Fatalf("duplicate=%+v err=%v", repeated, err)
				}
				payload, _ := json.Marshal(completed)
				if bytes.Contains(payload, []byte("native-")) || bytes.Contains(payload, []byte(upstream.URL)) {
					t.Error("private provider evidence leaked")
				}
			}
			if submits.Load() != 2 || uploads.Load() != 3 {
				t.Fatalf("submits=%d uploads=%d", submits.Load(), uploads.Load())
			}
			capabilities, err := client.GetMediaCapabilities(t.Context())
			if err != nil || len(capabilities.Services) != 4 {
				t.Fatalf("services=%+v err=%v", capabilities.Services, err)
			}
			usage := requestManagementUsage(t, router, owner, "30d")
			if len(usage.Providers) != 1 || usage.Providers[0].Data.Requests != 2 || len(usage.Models) != 0 {
				t.Fatalf("usage=%+v", usage)
			}
			verify := func(handler http.Handler) {
				t.Helper()
				read := func(path string, cookie *http.Cookie) map[string]any {
					t.Helper()
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, authenticatedJSONRequest(http.MethodGet, path, "", cookie))
					if response.Code != http.StatusOK {
						t.Fatalf("usage status=%d body=%s", response.Code, response.Body.String())
					}
					var summary map[string]any
					if err := json.Unmarshal(response.Body.Bytes(), &summary); err != nil {
						t.Fatal(err)
					}
					return summary
				}
				assertCoverage := func(data map[string]any) {
					t.Helper()
					coverage := data["token_coverage"].(map[string]any)
					for _, metric := range []string{"request_tokens", "response_tokens", "total_tokens"} {
						quantity := coverage[metric].(map[string]any)
						for field, want := range map[string]float64{"unknown_requests": data["requests"].(float64), "historical_requests": 0, "measured_requests": 0, "partial_requests": 0} {
							if quantity[field] != want {
								t.Errorf("%s.%s=%v want %v", metric, field, quantity[field], want)
							}
						}
					}
				}
				assertTotals := func(data map[string]any) {
					t.Helper()
					if data["requests"] != float64(2) {
						t.Fatalf("retained requests=%v want 2", data["requests"])
					}
					for _, metric := range []string{"request_tokens", "response_tokens", "total_tokens"} {
						if data[metric] != float64(0) {
							t.Errorf("media %s=%v want 0", metric, data[metric])
						}
					}
					assertCoverage(data)
				}
				for _, path := range []string{"/api/management/usage?interval=all", "/api/management/tenants/" + tenant + "/usage?interval=all"} {
					summary := read(path, owner)
					assertTotals(summary["totals"].(map[string]any))
					for _, collection := range []string{"providers", "buckets"} {
						rows := summary[collection].([]any)
						if len(rows) == 0 {
							t.Fatalf("%s usage absent", collection)
						}
						for _, value := range rows {
							assertCoverage(value.(map[string]any)["data"].(map[string]any))
						}
					}
				}
				admin := managementSessionCookieWithEmail(t, "media-evidence-admin", testManagementAdminEmail)
				users := read("/api/management/admin/users", admin)["users"].([]any)
				found := false
				for _, user := range users {
					for _, row := range user.(map[string]any)["tenants"].([]any) {
						item := row.(map[string]any)
						if item["id"] == tenant {
							found = true
							assertTotals(item["usage"].(map[string]any)["totals"].(map[string]any))
							executedDays := 0
							for _, day := range item["usage"].(map[string]any)["daily"].([]any) {
								data := day.(map[string]any)["data"].(map[string]any)
								assertCoverage(data)
								if data["requests"].(float64) > 0 {
									executedDays++
								}
							}
							if executedDays == 0 {
								t.Fatal("administrator executed day absent")
							}
						}
					}
				}
				if !found {
					t.Fatal("administrator tenant usage absent")
				}
			}
			verify(router)
			server.Close()
			if err := router.Close(); err != nil {
				t.Fatal(err)
			}
			verify(newTokenPresenceRouter(t, configuration, databasePath))
		})
	}
}

func heygenDurableClient(t *testing.T, upstream *httptest.Server, changes ...func(*proxy.Configuration)) (llmproxyclient.Client, *gorm.DB, func() llmproxyclient.Client) {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "heygen.sqlite")
	configuration := proxy.Configuration{ProviderCatalog: heygenTestCatalog(t, "heygen", upstream.URL), AssetStorePath: t.TempDir(), UpstreamCapacity: testfixtures.UpstreamCapacity(4, 100), MediaOperationWorkers: 1, MediaOperationClaimSeconds: 7200, MediaOperationClaimRenewalSeconds: 3600}
	for _, change := range changes {
		change(&configuration)
	}
	router := newManagementRouterWithDatabasePath(t, configuration, databasePath)
	owner := managementSessionCookie(t, "heygen-durable-owner")
	tenant := managementDefaultTenantTestID(t, router, owner)
	secret := generateManagementTenantSecret(t, router, owner, tenant)
	connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{"name": "Recovery account", "provider": "heygen", "fields": map[string]string{"api_key": "heygen-secret"}}, http.StatusCreated)
	accountConnectionExchange(t, router, owner, http.MethodPut, "/tenants/"+tenant+"/connections/heygen", map[string]string{"kind": "account_connection", "resource_id": connection["id"].(string)}, http.StatusOK)
	makeClient := func(handler http.Handler) llmproxyclient.Client {
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		cfg, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: secret})
		if err != nil {
			t.Fatal(err)
		}
		client, err := llmproxyclient.NewClient(cfg, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	database, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return makeClient(router), database, func() llmproxyclient.Client {
		return makeClient(newManagementRouterWithDatabasePath(t, configuration, databasePath))
	}
}

func heygenLipInput(t *testing.T, client llmproxyclient.Client) llmproxyclient.MediaOperationInput {
	t.Helper()
	video, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "video/mp4", Data: []byte("video")})
	if err != nil {
		t.Fatal(err)
	}
	audio, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: metaTranscriptionWAV(16000, 1)})
	if err != nil {
		t.Fatal(err)
	}
	return llmproxyclient.MediaOperationInput{Provider: "heygen", Capability: "video.lipsync", Input: heygenTestJSON(map[string]string{"video_asset_id": video.AssetID, "audio_asset_id": audio.AssetID}), Controls: json.RawMessage(`{"mode":"speed"}`)}
}
func heygenTestJSON(value any) json.RawMessage { data, _ := json.Marshal(value); return data }

func heygenFixtureUpload(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		t.Error(err)
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		t.Error(err)
		return
	}
	defer file.Close()
	data, _ := io.ReadAll(file)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"asset_id": "uploaded-asset", "url": "https://api.heygen.com/asset", "mime_type": header.Header.Get("Content-Type"), "size_bytes": len(data)}})
}

func TestHeyGenValidationRejectsBeforeProviderDispatch(t *testing.T) {
	var dispatches atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/users/me" {
			_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
			return
		}
		dispatches.Add(1)
		w.WriteHeader(500)
	}))
	defer upstream.Close()
	client, _, _ := heygenDurableClient(t, upstream)
	valid := heygenLipInput(t, client)
	cases := []llmproxyclient.MediaOperationInput{valid, valid, valid, valid, valid}
	cases[0].Controls = json.RawMessage(`{"mode":"obsolete"}`)
	cases[1].Controls = json.RawMessage(`{"mode":"speed","enable_caption":true}`)
	cases[2].Input = json.RawMessage(`{"video_asset_id":"ast_foreign","audio_asset_id":"ast_foreign"}`)
	cases[3].Model = "invented"
	cases[4].Controls = json.RawMessage(`{"mode":"speed","start_time":5,"end_time":2}`)
	for index, input := range cases {
		if _, err := client.CreateMediaOperation(t.Context(), fmt.Sprintf("invalid-%d", index), input); httpFailureStatus(err) != http.StatusBadRequest && !(index == 3 && httpFailureStatus(err) == http.StatusUnprocessableEntity) {
			t.Fatalf("invalid %d error=%v", index, err)
		}
	}
	if dispatches.Load() != 0 {
		t.Fatalf("invalid inputs dispatched=%d", dispatches.Load())
	}
}

func TestHeyGenBoundariesPreserveSanitizedUncertainty(t *testing.T) {
	for _, scenario := range []string{"rejected", "lost-submit", "unknown-state", "wrong-id", "failed", "foreign-artifact", "redirect-artifact"} {
		t.Run(scenario, func(t *testing.T) {
			var submissions, foreignCalls atomic.Int32
			foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreignCalls.Add(1); w.WriteHeader(500) }))
			defer foreign.Close()
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET /v3/users/me":
					_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
				case "GET /files/redirect.mp4":
					http.Redirect(w, r, foreign.URL+"/video.mp4", http.StatusTemporaryRedirect)
				case "POST /v3/assets":
					heygenFixtureUpload(t, w, r)
				case "POST /v3/lipsyncs":
					submissions.Add(1)
					if scenario == "rejected" {
						w.WriteHeader(422)
						_, _ = io.WriteString(w, `{"message":"private secret"}`)
						return
					}
					if scenario == "lost-submit" {
						conn, _, _ := w.(http.Hijacker).Hijack()
						_ = conn.Close()
						return
					}
					_, _ = io.WriteString(w, `{"data":{"lipsync_id":"accepted-job"}}`)
				default:
					status, id := "completed", "accepted-job"
					if scenario == "unknown-state" {
						status = "obsolete"
					}
					if scenario == "wrong-id" {
						id = "other-job"
					}
					if scenario == "failed" {
						status = "failed"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id, "status": status, "failure_message": "private secret", "video_url": func() string {
						if scenario == "redirect-artifact" {
							return upstream.URL + "/files/redirect.mp4"
						}
						return "https://foreign.invalid/video.mp4"
					}()}})
				}
			}))
			defer upstream.Close()
			client, _, _ := heygenDurableClient(t, upstream)
			input := heygenLipInput(t, client)
			accepted, err := client.CreateMediaOperation(t.Context(), "boundary", input)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			result, err := client.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
			expected := proxy.MediaOperationStateUncertain
			if scenario == "rejected" || scenario == "failed" {
				expected = proxy.MediaOperationStateFailed
			}
			if err != nil || result.State != expected || submissions.Load() != 1 {
				t.Fatalf("result=%+v error=%v submissions=%d", result, err, submissions.Load())
			}
			if foreignCalls.Load() != 0 {
				t.Fatal("untrusted artifact origin was contacted")
			}
			data, _ := json.Marshal(result)
			if bytes.Contains(data, []byte("private secret")) {
				t.Fatal("native error leaked")
			}
			duplicate, err := client.CreateMediaOperation(t.Context(), "boundary", input)
			if err != nil || duplicate.OperationID != accepted.OperationID || submissions.Load() != 1 {
				t.Fatalf("duplicate=%+v err=%v", duplicate, err)
			}
		})
	}
}

func TestHeyGenReceiptRecoveryNeverReplaysSubmission(t *testing.T) {
	for _, change := range []string{"none", "missing-handle", "changed-binding", "revoked-connection"} {
		t.Run(change, func(t *testing.T) {
			var submissions, polls atomic.Int32
			started, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET /v3/users/me":
					_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
				case "POST /v3/assets":
					heygenFixtureUpload(t, w, r)
				case "POST /v3/lipsyncs":
					submissions.Add(1)
					_, _ = io.WriteString(w, `{"data":{"lipsync_id":"retained-job"}}`)
				case "GET /v3/lipsyncs/retained-job":
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
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "retained-job", "status": "completed", "video_url": upstream.URL + "/files/recovered.mp4"}})
				case "GET /files/recovered.mp4":
					w.Header().Set("Content-Type", "video/mp4")
					_, _ = io.WriteString(w, "recovered-video")
				default:
					t.Errorf("unexpected %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			t.Cleanup(upstream.Close)
			client, database, restart := heygenDurableClient(t, upstream)
			input := heygenLipInput(t, client)
			accepted, err := client.CreateMediaOperation(t.Context(), "recover", input)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("not polled")
			}
			cancelled, err := client.CancelMediaOperation(t.Context(), accepted.OperationID)
			if err != nil || cancelled.State != proxy.MediaOperationStateRunning || cancelled.CancellationState != proxy.MediaCancellationUnsupported {
				t.Fatalf("cancellation=%+v error=%v", cancelled, err)
			}
			var receipt struct{ ProviderHandle string }
			if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Take(&receipt).Error; err != nil || !strings.Contains(receipt.ProviderHandle, "retained-job") {
				t.Fatalf("receipt=%+v error=%v", receipt, err)
			}
			if err := database.Exec("UPDATE media_operation_claim_records SET generation = generation + 1, expires_at = ? WHERE operation_id = ?", time.Now().Add(-time.Second), accepted.OperationID).Error; err != nil {
				t.Fatal(err)
			}
			switch change {
			case "missing-handle":
				if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Update("provider_handle", "").Error; err != nil {
					t.Fatal(err)
				}
			case "changed-binding":
				if err := database.Table("media_operation_records").Where("operation_id = ?", accepted.OperationID).Update("execution_binding", "obsolete").Error; err != nil {
					t.Fatal(err)
				}
			case "revoked-connection":
				if err := database.Exec("DELETE FROM managed_tenant_connection_records WHERE provider_id = ?", "heygen").Error; err != nil {
					t.Fatal(err)
				}
			}
			recovered := restart()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			result, err := recovered.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
			expected := proxy.MediaOperationStateUncertain
			if change == "none" {
				expected = proxy.MediaOperationStateSucceeded
			}
			if err != nil || result.State != expected || submissions.Load() != 1 {
				t.Fatalf("result=%+v error=%v submissions=%d", result, err, submissions.Load())
			}
		})
	}
}

func TestHeyGenCatalogRejectsUnsupportedCompositions(t *testing.T) {
	catalog := heygenTestCatalog(t, "heygen", "http://127.0.0.1:8765")
	for name, mutate := range map[string]func(*proxy.ProviderCatalogProvider){
		"missing artifact origins": func(p *proxy.ProviderCatalogProvider) { p.Transports[1].ArtifactOrigins = nil },
		"wrong lifecycle":          func(p *proxy.ProviderCatalogProvider) { p.Transports[1].Components.Execution.ID = "read_only" },
		"codec mismatch": func(p *proxy.ProviderCatalogProvider) {
			p.Transports[1].Components.ResponseCodec.ID = "heygen_v3_translation"
		},
		"wrong endpoint":   func(p *proxy.ProviderCatalogProvider) { p.Transports[1].Endpoint.Path = "/v2/video_translate" },
		"obsolete control": func(p *proxy.ProviderCatalogProvider) { p.Services[2].Controls[0].ID = "enable_caption" },
		"lower control bound": func(p *proxy.ProviderCatalogProvider) {
			value := float64(10)
			for index := range p.Services[2].Controls {
				if p.Services[2].Controls[index].ID == "start_time" {
					p.Services[2].Controls[index].Maximum = &value
				}
			}
		},
		"excessive upload": func(p *proxy.ProviderCatalogProvider) { value := 33554433; p.Services[2].Limits[0].Value = &value },
	} {
		t.Run(name, func(t *testing.T) {
			schema := catalog.Schema()
			for index := range schema.Providers {
				if schema.Providers[index].ID == "heygen" {
					mutate(&schema.Providers[index])
				}
			}
			if _, err := proxy.NewProviderCatalog(schema); err == nil {
				t.Fatal("unsupported catalog accepted")
			}
		})
	}
}

func TestHeyGenTranslationKeepsCompletedLanguageOnLaterFailure(t *testing.T) {
	var submits atomic.Int32
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v3/users/me":
			_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
		case "POST /v3/assets":
			heygenFixtureUpload(t, w, r)
		case "POST /v3/video-translations":
			submits.Add(1)
			_, _ = io.WriteString(w, `{"data":{"video_translation_ids":["completed-fr","failed-de"]}}`)
		case "GET /v3/video-translations/completed-fr":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "completed-fr", "status": "completed", "output_language": "French", "video_url": upstream.URL + "/files/fr.mp4"}})
		case "GET /v3/video-translations/failed-de":
			_, _ = io.WriteString(w, `{"data":{"id":"failed-de","status":"failed","failure_message":"sensitive upstream text"}}`)
		case "GET /files/fr.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = io.WriteString(w, "French-video")
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	client, _, _ := heygenDurableClient(t, upstream)
	lip := heygenLipInput(t, client)
	var source map[string]string
	_ = json.Unmarshal(lip.Input, &source)
	input := llmproxyclient.MediaOperationInput{Provider: "heygen", Capability: "video.translate", Input: heygenTestJSON(map[string]string{"video_asset_id": source["video_asset_id"]}), Controls: json.RawMessage(`{"mode":"precision","output_languages":["French","German"]}`)}
	accepted, err := client.CreateMediaOperation(t.Context(), "partial-translation", input)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	result, err := client.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
	if err != nil || result.State != proxy.MediaOperationStateFailed || len(result.PartialOutputs) != 1 || result.PartialOutputs[0].OutputOrdinal != 0 || submits.Load() != 1 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	asset, err := client.GetAsset(t.Context(), result.PartialOutputs[0].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := client.DownloadAsset(t.Context(), asset)
	if err != nil || string(data) != "French-video" {
		t.Fatalf("partial=%q error=%v", data, err)
	}
}

func TestHeyGenNativeBoundaryFailures(t *testing.T) {
	for _, scenario := range []string{"video-upload-lost", "audio-upload-lost", "upload-malformed", "upload-invalid-metadata", "submit-malformed", "submit-empty-handle", "submit-server-error", "poll-lost", "poll-malformed", "artifact-lost", "artifact-truncated", "artifact-empty", "artifact-wrong-mime", "artifact-wrong-kind", "artifact-oversized", "receipt-fenced", "pending-running", "deadline"} {
		t.Run(scenario, func(t *testing.T) {
			var submits, uploads, polls atomic.Int32
			var upstream *httptest.Server
			var database *gorm.DB
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v3/users/me":
					_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
				case r.URL.Path == "/v3/assets":
					number := uploads.Add(1)
					if scenario == "video-upload-lost" || scenario == "audio-upload-lost" && number == 2 {
						connection, _, _ := w.(http.Hijacker).Hijack()
						_ = connection.Close()
						return
					}
					if scenario == "upload-malformed" {
						_, _ = io.WriteString(w, `{`)
						return
					}
					if scenario == "upload-invalid-metadata" {
						_, _ = io.WriteString(w, `{"data":{"asset_id":"invalid/id","mime_type":"video/mp4","size_bytes":1,"url":"asset"}}`)
						return
					}
					heygenFixtureUpload(t, w, r)
				case r.Method == http.MethodPost:
					submits.Add(1)
					if scenario == "submit-malformed" {
						_, _ = io.WriteString(w, `{`)
						return
					}
					if scenario == "submit-empty-handle" {
						_, _ = io.WriteString(w, `{"data":{"lipsync_id":""}}`)
						return
					}
					if scenario == "submit-server-error" {
						w.WriteHeader(503)
						return
					}
					if scenario == "receipt-fenced" {
						if err := database.Exec("UPDATE media_operation_claim_records SET generation = generation + 1, expires_at = ?", time.Now().Add(-time.Second)).Error; err != nil {
							t.Error(err)
						}
					}
					_, _ = io.WriteString(w, `{"data":{"lipsync_id":"boundary-job"}}`)
				case strings.HasPrefix(r.URL.Path, "/files/"):
					if scenario == "artifact-lost" {
						connection, _, _ := w.(http.Hijacker).Hijack()
						_ = connection.Close()
						return
					}
					w.Header().Set("Content-Type", "video/mp4")
					if scenario == "artifact-truncated" {
						w.Header().Set("Content-Length", "100")
						_, _ = io.WriteString(w, "tiny")
						return
					}
					if scenario == "artifact-empty" {
						return
					}
					if scenario == "artifact-wrong-kind" {
						w.Header().Set("Content-Type", "audio/wav")
					}
					if scenario == "artifact-wrong-mime" {
						w.Header().Set("Content-Type", "text/plain")
					}
					_, _ = io.WriteString(w, "bounded-output")
				default:
					number := polls.Add(1)
					if scenario == "poll-lost" {
						connection, _, _ := w.(http.Hijacker).Hijack()
						_ = connection.Close()
						return
					}
					if scenario == "poll-malformed" {
						_, _ = io.WriteString(w, `{`)
						return
					}
					status := "completed"
					if scenario == "pending-running" && number < 3 {
						status = "pending"
						if number == 2 {
							status = "running"
						}
					}
					if scenario == "deadline" {
						status = "pending"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "boundary-job", "status": status, "video_url": upstream.URL + "/files/video.mp4"}})
				}
			}))
			defer upstream.Close()
			client, db, restart := heygenDurableClient(t, upstream, func(c *proxy.Configuration) {
				if scenario == "deadline" {
					c.MediaOperationLifetimeSeconds = 1
				}
				if scenario == "artifact-oversized" {
					schema := c.ProviderCatalog.Schema()
					for i := range schema.Providers {
						if schema.Providers[i].ID == "heygen" {
							for j := range schema.Providers[i].Services {
								if schema.Providers[i].Services[j].Operation == proxy.ModelOperationVideoLipSync {
									value := 1
									schema.Providers[i].Services[j].Limits[2].Value = &value
								}
							}
						}
					}
					catalog, err := proxy.NewProviderCatalog(schema)
					if err != nil {
						t.Fatal(err)
					}
					c.ProviderCatalog = catalog
				}
			})
			database = db
			input := heygenLipInput(t, client)
			accepted, err := client.CreateMediaOperation(t.Context(), "native-failure", input)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "receipt-fenced" {
				for submits.Load() == 0 {
					time.Sleep(time.Millisecond)
				}
				client = restart()
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			result, err := client.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
			expected := proxy.MediaOperationStateUncertain
			if strings.Contains(scenario, "upload") {
				expected = proxy.MediaOperationStateFailed
			}
			if scenario == "pending-running" {
				expected = proxy.MediaOperationStateSucceeded
			}
			if err != nil || result.State != expected {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			if submits.Load() > 1 {
				t.Fatal("paid create replayed")
			}
		})
	}
}

func TestHeyGenTranslationRejectsInvalidInputsAtBoundary(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/users/me" {
			t.Error("invalid input dispatched")
		}
		_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
	}))
	defer upstream.Close()
	client, _, _ := heygenDurableClient(t, upstream)
	lip := heygenLipInput(t, client)
	var source map[string]string
	_ = json.Unmarshal(lip.Input, &source)
	for index, controls := range []string{`{}`, `{"mode":"speed","output_languages":[]}`, `{"mode":"speed","output_languages":["French","French"]}`, `{"mode":"speed","output_languages":[" French"]}`, `{"mode":"speed","output_languages":["French"],"speaker_num":-1}`, `{"mode":"speed","output_languages":["French"],"fps_mode":"vfr"}`, `{"mode":"speed","output_languages":["French"],"start_time":-1}`, `{"mode":"speed","output_languages":["French"],"enable_caption":true}`} {
		input := llmproxyclient.MediaOperationInput{Provider: "heygen", Capability: "video.translate", Input: heygenTestJSON(map[string]string{"video_asset_id": source["video_asset_id"]}), Controls: json.RawMessage(controls)}
		if _, err := client.CreateMediaOperation(t.Context(), fmt.Sprintf("bad-translation-%d", index), input); httpFailureStatus(err) != 400 {
			t.Fatalf("invalid translation %d=%v", index, err)
		}
	}
	lip.Input = heygenTestJSON(map[string]string{"video_asset_id": source["video_asset_id"], "audio_asset_id": source["video_asset_id"]})
	if _, err := client.CreateMediaOperation(t.Context(), "wrong-audio-kind", lip); httpFailureStatus(err) != 400 {
		t.Fatalf("wrong audio error=%v", err)
	}
}
func TestHeyGenTranslationKeepsLaterSuccessAfterFirstFailure(t *testing.T) {
	var submits atomic.Int32
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v3/users/me":
			_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
		case "POST /v3/assets":
			heygenFixtureUpload(t, w, r)
		case "POST /v3/video-translations":
			submits.Add(1)
			_, _ = io.WriteString(w, `{"data":{"video_translation_ids":["failed-de","completed-fr"]}}`)
		case "GET /v3/video-translations/completed-fr":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "completed-fr", "status": "completed", "output_language": "French", "video_url": upstream.URL + "/files/fr.mp4"}})
		case "GET /v3/video-translations/failed-de":
			_, _ = io.WriteString(w, `{"data":{"id":"failed-de","status":"failed","failure_message":"sensitive upstream text"}}`)
		case "GET /files/fr.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = io.WriteString(w, "French-video")
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	client, _, _ := heygenDurableClient(t, upstream)
	lip := heygenLipInput(t, client)
	var source map[string]string
	_ = json.Unmarshal(lip.Input, &source)
	input := llmproxyclient.MediaOperationInput{Provider: "heygen", Capability: "video.translate", Input: heygenTestJSON(map[string]string{"video_asset_id": source["video_asset_id"]}), Controls: json.RawMessage(`{"mode":"precision","output_languages":["German","French"]}`)}
	accepted, err := client.CreateMediaOperation(t.Context(), "partial-translation", input)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	result, err := client.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
	if err != nil || result.State != proxy.MediaOperationStateFailed || len(result.PartialOutputs) != 1 || result.PartialOutputs[0].OutputOrdinal != 1 || submits.Load() != 1 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	asset, err := client.GetAsset(t.Context(), result.PartialOutputs[0].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := client.DownloadAsset(t.Context(), asset)
	if err != nil || string(data) != "French-video" {
		t.Fatalf("partial=%q error=%v", data, err)
	}
}

func TestHeyGenTranslationRecoversMixedChildrenAndPreservesEverySuccess(t *testing.T) {
	var submissions atomic.Int32
	var recovered atomic.Bool
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3/users/me":
			_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
		case r.URL.Path == "/v3/assets":
			heygenFixtureUpload(t, w, r)
		case r.Method == http.MethodPost:
			submissions.Add(1)
			_, _ = io.WriteString(w, `{"data":{"video_translation_ids":["failed-de","pending-fr","completed-es"]}}`)
		case strings.HasPrefix(r.URL.Path, "/files/"):
			if recovered.Load() && strings.Contains(r.URL.Path, "Spanish") {
				t.Error("durable completed child was downloaded from expired upstream URL")
				w.WriteHeader(410)
				return
			}
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = io.WriteString(w, "video:"+r.URL.Path)
		default:
			id := strings.TrimPrefix(r.URL.Path, "/v3/video-translations/")
			if recovered.Load() && (id == "completed-es" || id == "failed-de") {
				t.Error("durable terminal child was polled again")
				w.WriteHeader(410)
				return
			}
			status, language := "completed", "Spanish"
			if id == "failed-de" {
				status, language = "failed", "German"
			}
			if id == "pending-fr" {
				language = "French"
				if !recovered.Load() {
					status = "running"
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id, "status": status, "output_language": language, "video_url": upstream.URL + "/files/" + language + ".mp4"}})
		}
	}))
	defer upstream.Close()
	client, database, restart := heygenDurableClient(t, upstream)
	lip := heygenLipInput(t, client)
	var source map[string]string
	_ = json.Unmarshal(lip.Input, &source)
	input := llmproxyclient.MediaOperationInput{Provider: "heygen", Capability: "video.translate", Input: heygenTestJSON(map[string]string{"video_asset_id": source["video_asset_id"]}), Controls: json.RawMessage(`{"mode":"precision","output_languages":["German","French","Spanish"]}`)}
	accepted, err := client.CreateMediaOperation(t.Context(), "mixed-recovery", input)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	for {
		current, err := client.GetMediaOperation(ctx, accepted.OperationID)
		if err != nil {
			t.Fatal(err)
		}
		if len(current.PartialOutputs) == 1 {
			if current.State != proxy.MediaOperationStateRunning || current.PartialOutputs[0].OutputOrdinal != 2 {
				t.Fatalf("premature terminal state=%+v", current)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("completed sibling was not retained while earlier sibling remained running")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if err := database.Exec("UPDATE media_operation_claim_records SET generation = generation + 1, expires_at = ? WHERE operation_id = ?", time.Now().Add(-time.Second), accepted.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	recovered.Store(true)
	resumed := restart()
	result, err := resumed.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
	if err != nil || result.State != proxy.MediaOperationStateFailed || len(result.PartialOutputs) != 2 || submissions.Load() != 1 {
		t.Fatalf("recovered=%+v error=%v submissions=%d", result, err, submissions.Load())
	}
	for index, partial := range result.PartialOutputs {
		if partial.OutputOrdinal != index+1 {
			t.Fatalf("ordinal=%d", partial.OutputOrdinal)
		}
		asset, err := resumed.GetAsset(ctx, partial.AssetID)
		if err != nil {
			t.Fatal(err)
		}
		data, err := resumed.DownloadAsset(ctx, asset)
		expected := "video:/files/French.mp4"
		if index == 1 {
			expected = "video:/files/Spanish.mp4"
		}
		if err != nil || string(data) != expected {
			t.Fatalf("partial=%q error=%v", data, err)
		}
	}
	var usageCount int64
	if err := database.Table("media_operation_usage_delivery_records").Where("operation_id = ?", accepted.OperationID).Count(&usageCount).Error; err != nil || usageCount != 1 {
		t.Fatalf("usage=%d error=%v", usageCount, err)
	}
}
func TestHeyGenAudioTranslationRecoversMixedChildrenAndPreservesEverySuccess(t *testing.T) {
	var submissions atomic.Int32
	var recovered atomic.Bool
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3/users/me":
			_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
		case r.URL.Path == "/v3/assets":
			heygenFixtureUpload(t, w, r)
		case r.Method == http.MethodPost:
			submissions.Add(1)
			_, _ = io.WriteString(w, `{"data":{"video_translation_ids":["failed-de","pending-fr","completed-es"]}}`)
		case strings.HasPrefix(r.URL.Path, "/files/"):
			if recovered.Load() && strings.Contains(r.URL.Path, "Spanish") {
				t.Error("durable completed child was downloaded from expired upstream URL")
				w.WriteHeader(410)
				return
			}
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = io.WriteString(w, "video:"+r.URL.Path)
		default:
			id := strings.TrimPrefix(r.URL.Path, "/v3/video-translations/")
			if recovered.Load() && (id == "completed-es" || id == "failed-de") {
				t.Error("durable terminal child was polled again")
				w.WriteHeader(410)
				return
			}
			status, language := "completed", "Spanish"
			if id == "failed-de" {
				status, language = "failed", "German"
			}
			if id == "pending-fr" {
				language = "French"
				if !recovered.Load() {
					status = "running"
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id, "status": status, "output_language": language, "audio_url": upstream.URL + "/files/" + language + ".mp4"}})
		}
	}))
	defer upstream.Close()
	client, database, restart := heygenDurableClient(t, upstream)
	lip := heygenLipInput(t, client)
	var source map[string]string
	_ = json.Unmarshal(lip.Input, &source)
	input := llmproxyclient.MediaOperationInput{Provider: "heygen", Capability: "video.translate", Input: heygenTestJSON(map[string]string{"video_asset_id": source["video_asset_id"]}), Controls: json.RawMessage(`{"mode":"precision","output_languages":["German","French","Spanish"],"translate_audio_only":true}`)}
	accepted, err := client.CreateMediaOperation(t.Context(), "mixed-recovery", input)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	for {
		current, err := client.GetMediaOperation(ctx, accepted.OperationID)
		if err != nil {
			t.Fatal(err)
		}
		if len(current.PartialOutputs) == 1 {
			if current.State != proxy.MediaOperationStateRunning || current.PartialOutputs[0].OutputOrdinal != 2 {
				t.Fatalf("premature terminal state=%+v", current)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("completed sibling was not retained while earlier sibling remained running")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if err := database.Exec("UPDATE media_operation_claim_records SET generation = generation + 1, expires_at = ? WHERE operation_id = ?", time.Now().Add(-time.Second), accepted.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	recovered.Store(true)
	resumed := restart()
	result, err := resumed.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
	if err != nil || result.State != proxy.MediaOperationStateFailed || len(result.PartialOutputs) != 2 || submissions.Load() != 1 {
		t.Fatalf("recovered=%+v error=%v submissions=%d", result, err, submissions.Load())
	}
	for index, partial := range result.PartialOutputs {
		if partial.OutputOrdinal != index+1 {
			t.Fatalf("ordinal=%d", partial.OutputOrdinal)
		}
		asset, err := resumed.GetAsset(ctx, partial.AssetID)
		if err != nil {
			t.Fatal(err)
		}
		data, err := resumed.DownloadAsset(ctx, asset)
		expected := "video:/files/French.mp4"
		if index == 1 {
			expected = "video:/files/Spanish.mp4"
		}
		if err != nil || string(data) != expected {
			t.Fatalf("partial=%q error=%v", data, err)
		}
	}
	var usageCount int64
	if err := database.Table("media_operation_usage_delivery_records").Where("operation_id = ?", accepted.OperationID).Count(&usageCount).Error; err != nil || usageCount != 1 {
		t.Fatalf("usage=%d error=%v", usageCount, err)
	}
}

func TestHeyGenTranslationRejectsInvalidProviderBatchReceipts(t *testing.T) {
	for _, scenario := range []string{"missing-id", "duplicate-id", "wrong-language"} {
		t.Run(scenario, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v3/users/me":
					_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
				case r.URL.Path == "/v3/assets":
					heygenFixtureUpload(t, w, r)
				case r.Method == http.MethodPost:
					ids := []string{"first", "second"}
					if scenario == "missing-id" {
						ids = []string{"first"}
					}
					if scenario == "duplicate-id" {
						ids = []string{"first", "first"}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"video_translation_ids": ids}})
				default:
					_, _ = io.WriteString(w, `{"data":{"id":"first","status":"completed","output_language":"Spanish","video_url":"https://foreign.invalid/video"}}`)
				}
			}))
			defer upstream.Close()
			client, _, _ := heygenDurableClient(t, upstream)
			lip := heygenLipInput(t, client)
			var source map[string]string
			_ = json.Unmarshal(lip.Input, &source)
			input := llmproxyclient.MediaOperationInput{Provider: "heygen", Capability: "video.translate", Input: heygenTestJSON(map[string]string{"video_asset_id": source["video_asset_id"]}), Controls: json.RawMessage(`{"mode":"precision","output_languages":["French","German"]}`)}
			accepted, err := client.CreateMediaOperation(t.Context(), scenario, input)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			result, err := client.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
			if err != nil || result.State != proxy.MediaOperationStateUncertain {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestHeyGenTranslationUnicodeCharacterBounds(t *testing.T) {
	title := strings.Repeat("🎬", 300)
	inputLanguage := strings.Repeat("日", 40)
	outputLanguage := strings.Repeat("語", 40)
	var submissions atomic.Int32
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v3/users/me":
			_, _ = io.WriteString(w, `{"data":{"billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":20}}}`)
		case "POST /v3/assets":
			heygenFixtureUpload(t, w, r)
		case "POST /v3/video-translations":
			submissions.Add(1)
			var body struct {
				Title           string   `json:"title"`
				InputLanguage   string   `json:"input_language"`
				OutputLanguages []string `json:"output_languages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Title != title || body.InputLanguage != inputLanguage || len(body.OutputLanguages) != 1 || body.OutputLanguages[0] != outputLanguage {
				t.Errorf("Unicode input changed: %+v", body)
			}
			_, _ = io.WriteString(w, `{"data":{"video_translation_ids":["unicode-job"]}}`)
		case "GET /v3/video-translations/unicode-job":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "unicode-job", "status": "completed", "output_language": outputLanguage, "video_url": upstream.URL + "/files/unicode.mp4"}})
		case "GET /files/unicode.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = io.WriteString(w, "Unicode-video")
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	client, _, _ := heygenDurableClient(t, upstream)
	lip := heygenLipInput(t, client)
	var source map[string]string
	_ = json.Unmarshal(lip.Input, &source)
	controls := map[string]any{"mode": "precision", "title": title, "input_language": inputLanguage, "output_languages": []string{outputLanguage}}
	input := llmproxyclient.MediaOperationInput{Provider: "heygen", Capability: "video.translate", Input: heygenTestJSON(map[string]string{"video_asset_id": source["video_asset_id"]}), Controls: heygenTestJSON(controls)}
	accepted, err := client.CreateMediaOperation(t.Context(), "Unicode-accepted", input)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	result, err := client.WaitMediaOperation(ctx, accepted.OperationID, 5*time.Millisecond)
	if err != nil || result.State != proxy.MediaOperationStateSucceeded {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	for _, field := range []string{"title", "input_language", "output_languages"} {
		t.Run(field, func(t *testing.T) {
			invalid := map[string]any{"mode": "precision", "title": title, "input_language": inputLanguage, "output_languages": []string{outputLanguage}}
			switch field {
			case "title":
				invalid[field] = strings.Repeat("🎬", 1001)
			case "input_language":
				invalid[field] = strings.Repeat("日", 101)
			case "output_languages":
				invalid[field] = []string{strings.Repeat("語", 101)}
			}
			input.Controls = heygenTestJSON(invalid)
			if _, err := client.CreateMediaOperation(t.Context(), "Unicode-limit-"+field, input); httpFailureStatus(err) != 400 {
				t.Fatalf("over-limit Unicode %s=%v", field, err)
			}
		})
	}
	if submissions.Load() != 1 {
		t.Fatalf("Unicode rejected input dispatched: %d", submissions.Load())
	}
}
