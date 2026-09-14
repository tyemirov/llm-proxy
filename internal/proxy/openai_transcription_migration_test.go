package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestOpenAITranscriptionRetirementOwnershipMigration(t *testing.T) {
	for _, model := range []string{"gpt-4o-mini-transcribe", "gpt-4o-transcribe"} {
		t.Run(model, func(t *testing.T) {
			database := openLegacyManagedTenantDatabase(t, filepath.Join(t.TempDir(), "ownership.db"))
			now := time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC)
			tenant := legacyManagedTenantRecord{UserID: "transcription-owner", TenantID: "managed-transcription", DefaultProvider: ProviderNameOpenAI, DefaultModel: "gpt-4.1", DefaultDictationProvider: ProviderNameOpenAI, DefaultDictationModel: model, DefaultSystemPrompt: "preserve prompt", CreatedAt: now, UpdatedAt: now}
			cipher := internalManagedProviderKeyCipher()
			encrypted, err := cipher.encrypt(bytes.NewReader(bytes.Repeat([]byte{1}, cipher.aeadCipher.NonceSize())), tenant.UserID, ProviderNameOpenAI, "transcription-test-key")
			if err != nil {
				t.Fatal(err)
			}
			key := legacyManagedProviderAPIKeyRecord{UserID: tenant.UserID, ProviderID: ProviderNameOpenAI, EncryptedAPIKey: encrypted, TextModel: "gpt-4.1", CreatedAt: now, UpdatedAt: now}
			event := legacyManagedUsageEventRecord{ID: 1, UserID: tenant.UserID, TenantID: tenant.TenantID, Endpoint: usageEndpointDictation, ProviderID: ProviderNameOpenAI, ModelID: model, StatusCode: 200, Success: true, TotalTokens: 9, CreatedAt: now}
			for _, record := range []struct {
				table string
				value any
			}{{managedTenantTable, &tenant}, {managedProviderKeyTable, &key}, {managedUsageEventTable, &event}} {
				if err := database.Table(record.table).Create(record.value).Error; err != nil {
					t.Fatal(err)
				}
			}
			management := managedRouterTestManagementConfiguration()
			management.DatabaseDialector = database.Dialector
			configuration := Configuration{ProviderCatalog: internalCanonicalProviderCatalog(), Management: management, AssetStorePath: t.TempDir()}
			router, err := BuildRouter(configuration, zap.NewNop().Sugar())
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, PublicCapabilitiesPath, nil))
			if response.Code != 200 {
				t.Fatalf("public discovery status=%d", response.Code)
			}
			database, err = gorm.Open(database.Dialector, &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			var current managedTenantRecord
			if err := database.First(&current, "tenant_id = ?", tenant.TenantID).Error; err != nil {
				t.Fatal(err)
			}
			if current.DefaultDictationModel != "gpt-transcribe" || current.DefaultModel != tenant.DefaultModel || current.DefaultSystemPrompt != tenant.DefaultSystemPrompt || !current.UpdatedAt.Equal(now) {
				t.Fatalf("ownership migration dictation=%s text=%s prompt=%q updated=%s expected=%s", current.DefaultDictationModel, current.DefaultModel, current.DefaultSystemPrompt, current.UpdatedAt, now)
			}
			var historical managedUsageEventRecord
			if err := database.First(&historical, 1).Error; err != nil {
				t.Fatal(err)
			}
			if historical.ModelID != model || historical.ProviderID != ProviderNameOpenAI || historical.TotalTokens != 9 {
				t.Fatal("ownership migration changed historical usage")
			}
			if _, err := BuildRouter(configuration, zap.NewNop().Sugar()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOpenAITranscriptionRetirementStartup(t *testing.T) {
	for _, scenario := range []string{"success", "tenant update failure", "version record failure"} {
		t.Run(scenario, func(t *testing.T) {
			database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "transcription.db")), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err = migrateCurrentManagedSchema(database); err != nil {
				t.Fatal(err)
			}
			if err = database.Migrator().DropTable(&managedProviderAPIKeyRecord{}); err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC)
			if err = database.Create(&managedUserRecord{UserID: "transcription-owner", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
				t.Fatal(err)
			}
			if err = database.Create(&managedSchemaMigrationRecord{Version: 15, AppliedAt: now}).Error; err != nil {
				t.Fatal(err)
			}
			var tenants []managedTenantRecord
			var profiles []managedProviderProfileRecord
			var connections []managedProviderConnectionRecord
			var usage []managedUsageEventRecord
			cipher := internalManagedProviderKeyCipher()
			for index, model := range []string{"gpt-4o-mini-transcribe", "gpt-4o-transcribe", "gpt-transcribe"} {
				tenant := fakeTenantRecord("transcription-owner", fmt.Sprintf("transcription-%d", index), fmt.Sprintf("Transcription %d", index), now)
				digest := sha256.Sum256([]byte(fmt.Sprintf("transcription-secret-%d", index)))
				secret := hex.EncodeToString(digest[:])
				tenant.SecretDigest = &secret
				tenant.DefaultProvider, tenant.DefaultModel = ProviderNameOpenAI, "gpt-4.1"
				tenant.DefaultDictationProvider, tenant.DefaultDictationModel = ProviderNameOpenAI, model
				tenant.DefaultSystemPrompt = "preserve tenant prompt"
				profile := managedProviderProfileRecord{TenantID: tenant.TenantID, ProviderID: ProviderNameOpenAI, TextModel: "gpt-4.1", SystemPrompt: "preserve profile prompt", CreatedAt: now, UpdatedAt: now}
				encrypted, err := cipher.encryptConnection(strings.NewReader(strings.Repeat("n", cipher.aeadCipher.NonceSize())), tenant.TenantID, ProviderNameOpenAI, CatalogCredentialAPIKey, "transcription-test-key")
				if err != nil {
					t.Fatal(err)
				}
				connection := managedProviderConnectionRecord{TenantID: tenant.TenantID, ProviderID: ProviderNameOpenAI, FieldID: CatalogCredentialAPIKey, Value: encrypted, CreatedAt: now, UpdatedAt: now}
				event := managedUsageEventRecord{ID: uint(index + 1), TenantID: tenant.TenantID, Endpoint: usageEndpointDictation, ProviderID: ProviderNameOpenAI, ModelID: model, StatusCode: 200, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess, TotalTokens: 9, CreatedAt: now}
				for _, record := range []any{&tenant, &profile, &connection, &event} {
					if err = database.Create(record).Error; err != nil {
						t.Fatal(err)
					}
				}
				tenants = append(tenants, tenant)
				profiles = append(profiles, profile)
				connections = append(connections, connection)
				usage = append(usage, event)
			}
			if scenario == "tenant update failure" {
				err = database.Exec(`CREATE TRIGGER reject_transcription BEFORE UPDATE ON managed_tenant_records BEGIN SELECT RAISE(ABORT, 'transcription update rejected'); END`).Error
			}
			if scenario == "version record failure" {
				err = database.Exec(`CREATE TRIGGER reject_transcription BEFORE INSERT ON managed_schema_migration_records WHEN NEW.version=16 BEGIN SELECT RAISE(ABORT, 'transcription version rejected'); END`).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseMultipartForm(1024); err != nil {
					t.Error(err)
				}
				if r.FormValue("model") != "gpt-transcribe" || r.Header.Get("Authorization") != "Bearer transcription-test-key" {
					t.Error("migrated request has the wrong model or credential")
				}
				_, _ = io.WriteString(w, `{"text":"Migrated transcript."}`)
			}))
			defer upstream.Close()
			endpoints := NewEndpoints()
			endpoints.SetProviderBaseURL(ProviderNameOpenAI, upstream.URL)
			management := managedRouterTestManagementConfiguration()
			management.DatabaseDialector = database.Dialector
			configuration := Configuration{Endpoints: endpoints, ProviderCatalog: internalCanonicalProviderCatalog(), Management: management, AssetStorePath: t.TempDir()}
			router, err := BuildRouter(configuration, zap.NewNop().Sugar())
			if scenario != "success" {
				if err == nil || !strings.Contains(err.Error(), "rejected") {
					t.Fatalf("rollback error=%v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = BuildRouter(configuration, zap.NewNop().Sugar()); err != nil {
					t.Fatal(err)
				}
			}
			for index, expected := range tenants {
				if scenario == "success" {
					expected.DefaultDictationModel = "gpt-transcribe"
				}
				var actual managedTenantRecord
				if err := database.First(&actual, "tenant_id = ?", expected.TenantID).Error; err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(actual, expected) {
					t.Fatalf("tenant %d changed beyond the expected dictation migration", index)
				}
				var profile managedProviderProfileRecord
				var connection managedProviderConnectionRecord
				var event managedUsageEventRecord
				if err := database.First(&profile, "tenant_id = ?", expected.TenantID).Error; err != nil {
					t.Fatal(err)
				}
				if scenario == "success" {
					assertMigratedConnectionField(t, database, cipher, connections[index])
				} else {
					if err := database.First(&connection, "tenant_id = ?", expected.TenantID).Error; err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(connection, connections[index]) {
						t.Fatal("failed migration changed the predecessor credential")
					}
				}
				if err := database.First(&event, index+1).Error; err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(profile, profiles[index]) || !reflect.DeepEqual(event, usage[index]) {
					t.Fatal("migration changed a profile, credential, or historical usage record")
				}
				if scenario == "success" {
					var body bytes.Buffer
					form := multipart.NewWriter(&body)
					part, _ := form.CreateFormFile("audio", "speech.wav")
					_, _ = part.Write([]byte("fixture audio"))
					_ = form.Close()
					request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/dictate?key=transcription-secret-%d", index), &body)
					request.Header.Set("Content-Type", form.FormDataContentType())
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					if response.Code != 200 || !strings.Contains(response.Body.String(), "Migrated transcript.") {
						t.Fatalf("migrated HTTP status=%d body=%s", response.Code, response.Body)
					}
					deadline := time.Now().Add(5 * time.Second)
					for {
						var count int64
						if err := database.Model(&managedUsageEventRecord{}).Count(&count).Error; err != nil {
							t.Fatal(err)
						}
						if count == int64(len(usage)+index+1) {
							break
						}
						if time.Now().After(deadline) {
							t.Fatalf("migrated request usage count=%d", count)
						}
						time.Sleep(time.Millisecond)
					}
				}
			}
		})
	}
}
