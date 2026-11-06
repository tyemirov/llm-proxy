//go:build pilot_presubmission_recovery

package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type pilotRecoveryFixture struct {
	config   Configuration
	db       *gorm.DB
	evidence pilotRecoveryEvidence
}

func newPilotRecoveryFixture(t *testing.T) pilotRecoveryFixture {
	t.Helper()
	directory := t.TempDir()
	source, err := os.ReadFile("../../configs/providers.yml")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := ParseProviderCatalog(source)
	if err != nil {
		t.Fatal(err)
	}
	config := Configuration{ProviderCatalog: catalog, ModelCatalog: catalog.ModelCatalog(), AssetStorePath: filepath.Join(directory, "assets"), MaxAssetBytes: 33554432, AssetRetentionSeconds: 86400}
	config.Management.DatabasePath = filepath.Join(directory, "management.sqlite")
	db, err := gorm.Open(sqlite.Open(config.Management.DatabasePath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	for _, model := range []any{&mediaOperationRecord{}, &mediaOperationClaimRecord{}, &mediaOperationPartialReferenceRecord{}, &mediaOperationAssetReferenceRecord{}, &mediaOperationUsageDeliveryRecord{}, &managedUsageEventRecord{}, &managedJournalRequestRecord{}, &managedJournalAttemptRecord{}, &managedFundsReservationRecord{}, &mediaAvatarRecord{}, &managedAccountConnectionRecord{}, &managedTenantConnectionRecord{}} {
		if err := db.AutoMigrate(model); err != nil {
			t.Fatal(err)
		}
	}
	at, err := time.Parse(time.RFC3339Nano, pilotRecoveryDelivery)
	if err != nil {
		t.Fatal(err)
	}
	no, yes, count := false, true, 0
	evidence := pilotRecoveryEvidence{RecoveryID: "pilot-wav-receipt-2026-11-04", TenantID: pilotRecoveryTenant, OperationID: pilotRecoveryOperation, UsageEventID: pilotRecoveryUsageID, DeliveryAt: at, Reason: "heygen_wav_receipt_mime_mismatch", SourceReference: "fixture:complete-provider-history", NativeSubmissionReached: &no, ProviderHistoryComplete: &yes, MatchingNativeVideos: &count}
	avatarID, assetID := "ava_11111111111111111111111111111111", "ast_22222222222222222222222222222222"
	credential := "connection-recovery:v1"
	service, err := NewCatalogService(config.ModelCatalog)
	if err != nil {
		t.Fatal(err)
	}
	route, err := service.ResolveService("heygen", ModelOperationAvatarVideoGeneration)
	if err != nil {
		t.Fatal(err)
	}
	registry := newProviderRegistry(config)
	operation := mediaOperationRecord{OperationID: pilotRecoveryOperation, TenantID: pilotRecoveryTenant, IdempotencyKeyDigest: "original-key", IntentDigest: "original-intent", Capability: "avatar.video.generate", CatalogOperation: ModelOperationAvatarVideoGeneration, Provider: "heygen", CatalogRevision: "original-accepted-revision", CredentialReference: credential, NormalizedInput: []byte(`{"avatar_id":"` + avatarID + `","audio_asset_id":"` + assetID + `"}`), NormalizedControls: []byte(`{"engine":"avatar_iv","resolution":"1080p","aspect_ratio":"9:16"}`), PublicState: MediaOperationStateUncertain, ProviderExecutionState: MediaProviderExecutionUncertain, CancellationState: MediaCancellationNotRequested, DispatchToken: pilotRecoveryOperation, PublicErrorCode: "upstream_error", AcceptedAt: at.Add(-time.Minute), UpdatedAt: at, DeadlineAt: at.Add(time.Minute), TerminalAt: &at, ExecutionBinding: providerServiceBinding(route, registry.definitions[providerID("heygen")], CatalogProtocolHeyGenAvatarVideoV3)}
	payload := []byte("RIFF fixture original bytes")
	store := newTenantAssetStore(config.AssetStorePath, config.MaxAssetBytes, config.AssetRetentionSeconds)
	if err := os.MkdirAll(config.AssetStorePath, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := tenantAssetMetadata{Version: assetMetadataVersion, AssetID: assetID, TenantID: pilotRecoveryTenant, MIMEType: "audio/wav", SizeBytes: int64(len(payload)), ContentSHA256: mediaSHA256Hex(payload), State: assetStateAvailable, CreatedAt: at, ExpiresAt: at.Add(24 * time.Hour)}
	if err := store.writeMetadata(metadata); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.dataPath(assetID), payload, 0644); err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{&operation, &managedAccountConnectionRecord{ID: "connection-recovery", OwnerUserID: "owner", ProviderID: "heygen", Name: "fixture", Version: 1}, &managedTenantConnectionRecord{TenantID: pilotRecoveryTenant, ProviderID: "heygen", ConnectionID: "connection-recovery"}, &mediaAvatarRecord{AvatarID: avatarID, TenantID: pilotRecoveryTenant, Provider: "heygen", CredentialReference: credential, NativeLookID: "original-native-avatar", Metadata: []byte(`{}`)}, &mediaOperationAssetReferenceRecord{OperationID: pilotRecoveryOperation, TenantID: pilotRecoveryTenant, Role: "input", Ordinal: 0, AssetID: assetID, MIMEType: "audio/wav", SizeBytes: int64(len(payload)), Active: true, CreatedAt: at}, &mediaOperationUsageDeliveryRecord{OperationID: pilotRecoveryOperation, TenantID: pilotRecoveryTenant, CreatedAt: at, DeliveredAt: &at}, &managedUsageEventRecord{ID: pilotRecoveryUsageID, TenantID: pilotRecoveryTenant, Endpoint: usageEndpointMedia, ProviderID: "heygen", StatusCode: 502, Disposition: managedUsageDispositionFailed, OutcomeCode: managedUsageOutcomeUpstreamError, CreatedAt: at}} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return pilotRecoveryFixture{config, db, evidence}
}
func (f pilotRecoveryFixture) call(phase string, input any) (json.RawMessage, error) {
	data, _ := json.Marshal(input)
	return RunMediaPresubmissionRecovery(context.Background(), f.config, phase, bytes.NewReader(data))
}
func (f pilotRecoveryFixture) prepare(t *testing.T) pilotRecoveryPlan {
	t.Helper()
	data, err := f.call("prepare", f.evidence)
	if err != nil {
		t.Fatal(err)
	}
	var plan pilotRecoveryPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	return plan
}
func (f pilotRecoveryFixture) terminal(t *testing.T, state string) {
	t.Helper()
	at := time.Now().UTC()
	if err := f.db.Model(&mediaOperationRecord{}).Where("operation_id = ?", pilotRecoveryOperation).Updates(map[string]any{"public_state": state, "provider_execution_state": "completed", "terminal_at": at, "updated_at": at, "provider_handle": "original-native-video"}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestPilotRecoverySQLiteLifecycle(t *testing.T) {
	f := newPilotRecoveryFixture(t)
	plan := f.prepare(t)
	if f.db.Migrator().HasTable(&mediaPresubmissionRecoveryRecord{}) {
		t.Fatal("prepare created audit schema")
	}
	if _, err := f.call("preview", plan); err != nil {
		t.Fatal(err)
	}
	if f.db.Migrator().HasTable(&mediaPresubmissionRecoveryRecord{}) {
		t.Fatal("preview created audit schema")
	}
	if plan.Snapshot.Operation.CatalogRevision == plan.Snapshot.CurrentCatalogRevision {
		t.Fatal("fixture must preserve older accepted catalog")
	}
	first, err := f.call("apply", plan)
	if err != nil {
		t.Fatal(err)
	}
	var operation mediaOperationRecord
	f.db.First(&operation, "operation_id = ?", pilotRecoveryOperation)
	if operation.PublicState != MediaOperationStateQueued || operation.ProviderExecutionState != MediaProviderExecutionNotDispatched || !operation.AcceptedAt.Equal(plan.Snapshot.Operation.AcceptedAt) || !operation.DeadlineAt.After(time.Now()) || operation.DeadlineAt.After(time.Now().Add(901*time.Second)) {
		t.Fatalf("invalid reopened operation: %+v", operation)
	}
	if _, err := f.call("finalize", plan); err == nil {
		t.Fatal("finalized queued operation")
	}
	f.terminal(t, MediaOperationStateSucceeded)
	repeat, err := f.call("apply", plan)
	if err != nil || !bytes.Equal(first, repeat) {
		t.Fatalf("apply repeat %s %v", repeat, err)
	}
	final, err := f.call("finalize", plan)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.call("finalize", plan)
	if err != nil || !bytes.Equal(final, again) {
		t.Fatalf("finalize repeat %s %v", again, err)
	}
	if _, err := f.call("preview", plan); err != nil {
		t.Fatal(err)
	}
	var usage managedUsageEventRecord
	f.db.First(&usage, pilotRecoveryUsageID)
	if usage.StatusCode != 200 || usage.Disposition != managedUsageDispositionSucceeded || !usage.CreatedAt.Equal(f.evidence.DeliveryAt) {
		t.Fatalf("usage %+v", usage)
	}
	var count int64
	f.db.Model(&managedUsageEventRecord{}).Count(&count)
	if count != 1 {
		t.Fatalf("usage duplicated: %d", count)
	}
	var delivery mediaOperationUsageDeliveryRecord
	f.db.First(&delivery, "operation_id = ?", pilotRecoveryOperation)
	if !pilotRecoveryEqual(delivery, plan.Snapshot.Delivery) {
		t.Fatal("delivery marker changed")
	}
	var audit mediaPresubmissionRecoveryRecord
	f.db.First(&audit)
	if audit.Plan != pilotRecoveryJSON(plan) || audit.FinalizedAt == nil || audit.AppliedOperation == "" || audit.FinalUsage == "" {
		t.Fatal("missing immutable audit")
	}
}

func TestPilotRecoverySQLiteRejectsChangedAuthority(t *testing.T) {
	cases := map[string]func(pilotRecoveryFixture){
		"handle": func(f pilotRecoveryFixture) {
			f.db.Model(&mediaOperationRecord{}).Where("operation_id = ?", pilotRecoveryOperation).Update("provider_handle", "already-submitted")
		},
		"claim": func(f pilotRecoveryFixture) {
			f.db.Create(&mediaOperationClaimRecord{OperationID: pilotRecoveryOperation, WorkerID: "active", Generation: 1, ExpiresAt: time.Now().Add(time.Hour)})
		},
		"cancel": func(f pilotRecoveryFixture) {
			f.db.Model(&mediaOperationRecord{}).Where("operation_id = ?", pilotRecoveryOperation).Update("cancellation_state", "requested")
		},
		"input": func(f pilotRecoveryFixture) {
			f.db.Model(&mediaOperationAssetReferenceRecord{}).Where("operation_id = ?", pilotRecoveryOperation).Update("active", false)
		},
		"credential": func(f pilotRecoveryFixture) {
			f.db.Model(&managedAccountConnectionRecord{}).Where("id = ?", "connection-recovery").Update("version", 2)
		},
		"usage": func(f pilotRecoveryFixture) {
			f.db.Model(&managedUsageEventRecord{}).Where("id = ?", pilotRecoveryUsageID).Update("latency_milliseconds", 55)
		},
		"duplicate usage": func(f pilotRecoveryFixture) {
			f.db.Create(&managedUsageEventRecord{ID: 11238, TenantID: pilotRecoveryTenant, Endpoint: usageEndpointMedia, ProviderID: "heygen", CreatedAt: f.evidence.DeliveryAt, Disposition: managedUsageDispositionFailed, OutcomeCode: managedUsageOutcomeUpstreamError})
		},
		"duplicate delivery": func(f pilotRecoveryFixture) {
			at := f.evidence.DeliveryAt
			f.db.Create(&mediaOperationUsageDeliveryRecord{OperationID: "mop_other", TenantID: pilotRecoveryTenant, CreatedAt: at, DeliveredAt: &at})
		},
		"bytes": func(f pilotRecoveryFixture) {
			os.WriteFile(filepath.Join(f.config.AssetStorePath, "ast_22222222222222222222222222222222.data"), []byte("changed"), 0644)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newPilotRecoveryFixture(t)
			plan := f.prepare(t)
			mutate(f)
			if _, err := f.call("apply", plan); err == nil {
				t.Fatal("accepted changed authority")
			}
			if f.db.Migrator().HasTable(&mediaPresubmissionRecoveryRecord{}) {
				t.Fatal("failed transaction retained audit schema")
			}
		})
	}
}
func TestPilotRecoverySQLiteConcurrentApply(t *testing.T) {
	f := newPilotRecoveryFixture(t)
	plan := f.prepare(t)
	var wait sync.WaitGroup
	results := make(chan string, 2)
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := f.call("apply", plan)
			if err != nil {
				results <- err.Error()
				return
			}
			results <- string(result)
		}()
	}
	wait.Wait()
	close(results)
	first := ""
	for result := range results {
		if first == "" {
			first = result
		}
		if result != first {
			t.Fatalf("concurrent receipts differ: %s / %s", first, result)
		}
	}
	var count int64
	f.db.Model(&mediaPresubmissionRecoveryRecord{}).Count(&count)
	if count != 1 {
		t.Fatalf("audit count %d", count)
	}
}
func TestPilotRecoverySQLiteFinalizationGuards(t *testing.T) {
	for _, state := range []string{MediaOperationStateFailed, MediaOperationStateCancelled} {
		t.Run(state, func(t *testing.T) {
			f := newPilotRecoveryFixture(t)
			plan := f.prepare(t)
			if _, err := f.call("apply", plan); err != nil {
				t.Fatal(err)
			}
			f.terminal(t, state)
			if _, err := f.call("finalize", plan); err != nil {
				t.Fatal(err)
			}
			var usage managedUsageEventRecord
			f.db.First(&usage, pilotRecoveryUsageID)
			expected := 502
			if state == MediaOperationStateCancelled {
				expected = 499
			}
			if usage.StatusCode != expected {
				t.Fatal(usage.StatusCode)
			}
		})
	}
	t.Run("changed usage", func(t *testing.T) {
		f := newPilotRecoveryFixture(t)
		plan := f.prepare(t)
		f.call("apply", plan)
		f.terminal(t, MediaOperationStateSucceeded)
		f.db.Model(&managedUsageEventRecord{}).Where("id = ?", pilotRecoveryUsageID).Update("request_tokens", 1)
		if _, err := f.call("finalize", plan); err == nil {
			t.Fatal("changed usage finalized")
		}
	})
	t.Run("audit failure rollback", func(t *testing.T) {
		f := newPilotRecoveryFixture(t)
		plan := f.prepare(t)
		if err := f.db.AutoMigrate(&mediaPresubmissionRecoveryRecord{}); err != nil {
			t.Fatal(err)
		}
		if err := f.db.Exec(`CREATE TRIGGER reject_recovery_audit BEFORE INSERT ON media_presubmission_recovery_records BEGIN SELECT RAISE(ABORT,'fixture audit unavailable'); END`).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.call("apply", plan); err == nil {
			t.Fatal("audit failure accepted")
		}
		var operation mediaOperationRecord
		f.db.First(&operation, "operation_id = ?", pilotRecoveryOperation)
		if operation.PublicState != MediaOperationStateUncertain {
			t.Fatal("operation changed without audit")
		}
	})
}

func TestPilotRecoveryRealWorkerNativeHTTP(t *testing.T) {
	f := newPilotRecoveryFixture(t)
	if err := f.db.AutoMigrate(&managedConnectionFieldRecord{}); err != nil {
		t.Fatal(err)
	}
	var uploads, renders, polls atomic.Int32
	var origin string
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v3/assets":
			uploads.Add(1)
			form, err := r.MultipartReader()
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			part, err := form.NextPart()
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			data, err := io.ReadAll(part)
			if err != nil || part.Header.Get("Content-Type") != "audio/wav" || string(data) != "RIFF fixture original bytes" {
				t.Errorf("changed original upload: %q %v", data, err)
			}
			fmt.Fprintf(w, `{"data":{"asset_id":"original-audio-upload","url":"https://example.test/audio","mime_type":"audio/x-wav","size_bytes":%d}}`, len(data))
		case r.Method == http.MethodPost && r.URL.Path == "/v3/videos":
			renders.Add(1)
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["avatar_id"] != "original-native-avatar" || body["audio_asset_id"] != "original-audio-upload" {
				t.Errorf("wrong native intent: %+v", body)
			}
			io.WriteString(w, `{"data":{"video_id":"first-native-video"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v3/videos/first-native-video":
			state := "waiting"
			if polls.Add(1) > 1 {
				state = "completed"
			}
			fmt.Fprintf(w, `{"data":{"id":"first-native-video","status":%q,"video_url":%q}}`, state, origin+"/output.mp4")
		case r.URL.Path == "/output.mp4":
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" {
				t.Error("credential forwarded to output")
			}
			w.Header().Set("Content-Type", "video/mp4")
			io.WriteString(w, "fixture-video")
		default:
			t.Errorf("unexpected native route %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer native.Close()
	origin = native.URL
	plan := f.prepare(t)
	if _, err := f.call("apply", plan); err != nil {
		t.Fatal(err)
	}
	registry := newProviderRegistry(f.config)
	provider := registry.definitions[providerID("heygen")]
	provider.fields = nil
	registry.definitions[providerID("heygen")] = provider
	tenants := &managedTenantStore{routingDefaults: registry}
	store := &mediaOperationStore{database: f.db, now: func() time.Time { return time.Now().UTC() }}
	assets := newTenantAssetStore(f.config.AssetStorePath, f.config.MaxAssetBytes, 86400)
	// The saved fixture input is historical; keep it available while the renewed operation runs.
	assets.now = func() time.Time { return f.evidence.DeliveryAt.Add(time.Minute) }
	catalog, err := NewCatalogService(f.config.ModelCatalog)
	if err != nil {
		t.Fatal(err)
	}
	route, err := catalog.ResolveService("heygen", ModelOperationAvatarVideoGeneration)
	if err != nil {
		t.Fatal(err)
	}
	active := provider.transports[route.Transport]
	active.artifactOrigins = []string{origin}
	provider.transports[route.Transport] = active
	adapter := newProviderHeyGenAvatarAdapter(route, provider, tenants, store, assets)
	target, _ := url.Parse(origin)
	doer := avatarBoundaryDoer(func(r *http.Request) (*http.Response, error) {
		copy := r.Clone(r.Context())
		u := *r.URL
		u.Scheme = target.Scheme
		u.Host = target.Host
		copy.URL = &u
		copy.RequestURI = ""
		return native.Client().Do(copy)
	})
	service := &mediaOperationService{workerContext: t.Context(), logger: zap.NewNop().Sugar(), httpClient: doer, store: store, assets: assets, adapters: map[string]MediaOperationAdapter{mediaOperationAdapterKey("avatar.video.generate", "heygen", ""): adapter}, catalog: catalog, providers: registry, claimLifetime: time.Minute, claimRenewal: time.Second}
	if err := service.executeOperation("recovery-worker", pilotRecoveryOperation); err != nil {
		t.Fatal(err)
	}
	var operation mediaOperationRecord
	f.db.First(&operation, "operation_id = ?", pilotRecoveryOperation)
	if operation.PublicState != MediaOperationStateSucceeded {
		t.Fatalf("worker state %+v", operation)
	}
	if operation.IdempotencyKeyDigest != plan.Snapshot.Operation.IdempotencyKeyDigest || operation.IntentDigest != plan.Snapshot.Operation.IntentDigest || !operation.AcceptedAt.Equal(plan.Snapshot.Operation.AcceptedAt) || operation.CatalogRevision != plan.Snapshot.Operation.CatalogRevision {
		t.Fatal("worker changed original intent")
	}
	if uploads.Load() != 1 || renders.Load() != 1 || polls.Load() != 2 {
		t.Fatalf("native counts uploads=%d renders=%d polls=%d", uploads.Load(), renders.Load(), polls.Load())
	}
	if _, err := f.call("finalize", plan); err != nil {
		t.Fatal(err)
	}
	if _, err := f.call("apply", plan); err != nil {
		t.Fatal(err)
	}
	if err := service.executeOperation("repeat-worker", pilotRecoveryOperation); err != nil {
		t.Fatal(err)
	}
	if uploads.Load() != 1 || renders.Load() != 1 {
		t.Fatal("repeat recovery submitted again")
	}
	var usageCount int64
	f.db.Model(&managedUsageEventRecord{}).Count(&usageCount)
	if usageCount != 1 {
		t.Fatalf("duplicate usage events %d", usageCount)
	}
}

func TestPilotRecoveryRealCLI(t *testing.T) {
	f := newPilotRecoveryFixture(t)
	directory := t.TempDir()
	binary := filepath.Join(directory, "recovery")
	build := exec.CommandContext(t.Context(), "go", "build", "-tags", "pilot_presubmission_recovery", "-o", binary, "../../cmd/cli")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual tagged CLI: %s %v", out, err)
	}
	source, err := os.ReadFile("../../configs/config.yml")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(source, &document); err != nil {
		t.Fatal(err)
	}
	document["management"] = map[string]any{"public_origin": "https://proxy.example", "ui_description": "Fixture", "ui_origins": []string{"https://proxy.example"}, "admin_emails": []string{}, "tauth_url": "https://tauth.example", "tauth_tenant_id": "fixture", "google_client_id": "fixture", "login_path": "/auth/login", "logout_path": "/auth/logout", "nonce_path": "/auth/nonce", "session_path": "/auth/session", "jwt_signing_key": "fixture-key", "jwt_issuer": "fixture", "session_cookie_name": "fixture", "database_path": f.config.Management.DatabasePath, "provider_key_encryption_key": "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", "management_api_origin": "https://proxy.example", "proxy_origin": "https://proxy.example"}
	document["server"].(map[string]any)["asset_store_path"] = f.config.AssetStorePath
	config, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yml")
	if err := os.WriteFile(configPath, config, 0644); err != nil {
		t.Fatal(err)
	}
	providers, err := os.ReadFile("../../configs/providers.yml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "providers.yml"), providers, 0644); err != nil {
		t.Fatal(err)
	}
	run := func(phase string, input any) ([]byte, error) {
		data, _ := json.Marshal(input)
		inputPath := filepath.Join(directory, phase+".json")
		if err := os.WriteFile(inputPath, data, 0644); err != nil {
			t.Fatal(err)
		}
		command := exec.CommandContext(t.Context(), binary, "recover-media-presubmission", phase, "--config", configPath, "--input", inputPath)
		return command.CombinedOutput()
	}
	data, err := run("prepare", f.evidence)
	if err != nil {
		t.Fatalf("CLI prepare: %s %v", data, err)
	}
	var plan pilotRecoveryPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatalf("CLI JSON: %s %v", data, err)
	}
	if f.db.Migrator().HasTable(&mediaPresubmissionRecoveryRecord{}) {
		t.Fatal("CLI prepare mutated schema")
	}
	if data, err := run("preview", plan); err != nil || !strings.Contains(string(data), `"eligible":true`) {
		t.Fatalf("CLI preview: %s %v", data, err)
	}
	applied, err := run("apply", plan)
	if err != nil {
		t.Fatalf("CLI apply: %s %v", applied, err)
	}
	if data, err := run("finalize", plan); err == nil || !strings.Contains(string(data), "no concrete terminal outcome") {
		t.Fatalf("CLI finalized queued: %s %v", data, err)
	}
	f.terminal(t, MediaOperationStateSucceeded)
	if data, err := run("apply", plan); err != nil || !bytes.Equal(data, applied) {
		t.Fatalf("CLI repeat apply: %s %v", data, err)
	}
	finalized, err := run("finalize", plan)
	if err != nil {
		t.Fatalf("CLI finalize: %s %v", finalized, err)
	}
	if data, err := run("finalize", plan); err != nil || !bytes.Equal(data, finalized) {
		t.Fatalf("CLI repeat finalize: %s %v", data, err)
	}
	wrong := plan
	wrong.Evidence.UsageEventID++
	if data, err := run("apply", wrong); err == nil {
		t.Fatalf("CLI wrong event accepted: %s", data)
	}
}

func TestPilotRecoveryPublicInputAdmission(t *testing.T) {
	f := newPilotRecoveryFixture(t)
	cases := map[string]func(*pilotRecoveryEvidence){
		"tenant": func(e *pilotRecoveryEvidence) { e.TenantID = "other" }, "operation": func(e *pilotRecoveryEvidence) { e.OperationID = "mop_other" }, "event": func(e *pilotRecoveryEvidence) { e.UsageEventID++ }, "timestamp": func(e *pilotRecoveryEvidence) { e.DeliveryAt = e.DeliveryAt.Add(time.Nanosecond) }, "reason": func(e *pilotRecoveryEvidence) { e.Reason = "unknown" }, "reference": func(e *pilotRecoveryEvidence) { e.SourceReference = "" }, "recovery id": func(e *pilotRecoveryEvidence) { e.RecoveryID = "" }, "submission absent": func(e *pilotRecoveryEvidence) { e.NativeSubmissionReached = nil }, "submission reached": func(e *pilotRecoveryEvidence) { yes := true; e.NativeSubmissionReached = &yes }, "history absent": func(e *pilotRecoveryEvidence) { e.ProviderHistoryComplete = nil }, "history incomplete": func(e *pilotRecoveryEvidence) { no := false; e.ProviderHistoryComplete = &no }, "matches absent": func(e *pilotRecoveryEvidence) { e.MatchingNativeVideos = nil }, "matches present": func(e *pilotRecoveryEvidence) { one := 1; e.MatchingNativeVideos = &one },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			evidence := f.evidence
			change(&evidence)
			if _, err := f.call("prepare", evidence); err == nil {
				t.Fatal("invalid exact evidence admitted")
			}
		})
	}
	for _, input := range []string{`{`, `{"unknown":true}`, pilotRecoveryJSON(f.evidence) + ` {}`} {
		if _, err := RunMediaPresubmissionRecovery(t.Context(), f.config, "prepare", strings.NewReader(input)); err == nil {
			t.Fatalf("invalid JSON admitted: %s", input)
		}
	}
	if _, err := RunMediaPresubmissionRecovery(t.Context(), f.config, "prepare", nil); err == nil {
		t.Fatal("missing input admitted")
	}
	if _, err := f.call("unknown", f.evidence); err == nil {
		t.Fatal("unknown phase admitted")
	}
	plan := f.prepare(t)
	for _, seconds := range []int{0, 901} {
		invalid := plan
		invalid.RenewalSeconds = seconds
		if _, err := f.call("apply", invalid); err == nil {
			t.Fatal("unbounded renewal admitted")
		}
	}
	invalid := plan
	invalid.Schema = "other"
	if _, err := f.call("apply", invalid); err == nil {
		t.Fatal("unknown plan admitted")
	}
	if _, err := f.call("finalize", plan); err == nil {
		t.Fatal("unapplied plan finalized")
	}
	missing := f.config
	missing.Management.DatabasePath = ""
	if _, err := RunMediaPresubmissionRecovery(t.Context(), missing, "prepare", strings.NewReader(pilotRecoveryJSON(f.evidence))); err == nil {
		t.Fatal("missing database admitted")
	}
	missing.Management.DatabasePath = filepath.Join(t.TempDir(), "absent.sqlite")
	if _, err := RunMediaPresubmissionRecovery(t.Context(), missing, "prepare", strings.NewReader(pilotRecoveryJSON(f.evidence))); err == nil {
		t.Fatal("absent database created")
	}
	if _, err := os.Stat(missing.Management.DatabasePath); !os.IsNotExist(err) {
		t.Fatal("prepare created database")
	}
}
func TestPilotRecoveryFinalizationImmutableAudit(t *testing.T) {
	for _, column := range []string{"deadline_at", "idempotency_key_digest", "intent_digest", "catalog_revision", "credential_reference"} {
		t.Run(column, func(t *testing.T) {
			f := newPilotRecoveryFixture(t)
			plan := f.prepare(t)
			if _, err := f.call("apply", plan); err != nil {
				t.Fatal(err)
			}
			f.terminal(t, MediaOperationStateSucceeded)
			var value any = "changed"
			if column == "deadline_at" {
				value = time.Now().Add(time.Hour)
			}
			if err := f.db.Model(&mediaOperationRecord{}).Where("operation_id = ?", pilotRecoveryOperation).Update(column, value).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := f.call("finalize", plan); err == nil {
				t.Fatal("changed terminal authority finalized")
			}
		})
	}
	t.Run("different plan", func(t *testing.T) {
		f := newPilotRecoveryFixture(t)
		plan := f.prepare(t)
		if _, err := f.call("apply", plan); err != nil {
			t.Fatal(err)
		}
		plan.Evidence.RecoveryID = "different-recovery"
		if _, err := f.call("apply", plan); err == nil {
			t.Fatal("different plan reused audit")
		}
		if _, err := f.call("preview", plan); err == nil {
			t.Fatal("different plan preview reused audit")
		}
	})
	t.Run("final audit failure", func(t *testing.T) {
		f := newPilotRecoveryFixture(t)
		plan := f.prepare(t)
		if _, err := f.call("apply", plan); err != nil {
			t.Fatal(err)
		}
		f.terminal(t, MediaOperationStateSucceeded)
		if err := f.db.Exec(`CREATE TRIGGER reject_recovery_final BEFORE UPDATE ON media_presubmission_recovery_records BEGIN SELECT RAISE(ABORT,'fixture audit unavailable'); END`).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.call("finalize", plan); err == nil {
			t.Fatal("final audit failure accepted")
		}
		var usage managedUsageEventRecord
		f.db.First(&usage, pilotRecoveryUsageID)
		if !pilotRecoveryEqual(usage, plan.Snapshot.Usage) {
			t.Fatal("usage changed without durable final audit")
		}
	})
}
