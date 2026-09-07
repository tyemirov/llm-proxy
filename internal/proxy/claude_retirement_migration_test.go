package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestClaudeRetirementStartup(t *testing.T) {
	for _, scenario := range []string{"success", "record failure", "completion record failure", "missing policy", "profile failure", "tenant failure", "conflicting profile reasoning", "profile read failure"} {
		failRecord := scenario != "success"
		t.Run(scenario, func(t *testing.T) {
			database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "retirement.db")), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err = migrateCurrentManagedSchema(database); err != nil {
				t.Fatal(err)
			}
			if err = database.Migrator().DropTable(&managedProviderAPIKeyRecord{}); err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
			if err = database.Create(&managedUserRecord{UserID: "claude-retirement-owner", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
				t.Fatal(err)
			}
			if err = database.Create(&managedSchemaMigrationRecord{Version: 14, AppliedAt: now}).Error; err != nil {
				t.Fatal(err)
			}
			cipher := internalManagedProviderKeyCipher()
			var tenants []managedTenantRecord
			var profiles []managedProviderProfileRecord
			var connections []managedProviderConnectionRecord
			var usage []managedUsageEventRecord
			for index, route := range []struct{ provider, model string }{{ProviderNameAnthropic, "claude-opus-4-1"}, {ProviderNameAnthropic, "claude-opus-4-1-20250805"}, {ProviderNameSiliconFlow, "deepseek-reasoner"}, {ProviderNameAnthropic, "claude-opus-5"}} {
				tenant := fakeTenantRecord("claude-retirement-owner", fmt.Sprintf("retirement-%d", index), fmt.Sprintf("Retirement %d", index), now)
				secret := fmt.Sprintf("claude-retirement-secret-%d", index)
				digest := sha256.Sum256([]byte(secret))
				digestText := hex.EncodeToString(digest[:])
				tenant.SecretDigest = &digestText
				tenant.DefaultProvider, tenant.DefaultModel = route.provider, route.model
				tenant.DefaultSystemPrompt = "preserve default prompt"
				if scenario == "conflicting profile reasoning" && index == 0 {
					tenant.DefaultModel = "claude-opus-5"
					tenant.DefaultReasoningEffort = "max"
				}
				if route.provider == ProviderNameSiliconFlow {
					tenant.DefaultDictationProvider = ProviderNameSiliconFlow
					tenant.DefaultDictationModel = "sensevoice-small"
				}
				if route.model == "claude-opus-5" {
					tenant.DefaultReasoningEffort = "low"
				}
				profile := managedProviderProfileRecord{TenantID: tenant.TenantID, ProviderID: route.provider, TextModel: route.model, SystemPrompt: "preserve profile prompt", CreatedAt: now, UpdatedAt: now}
				encrypted, err := cipher.encryptConnection(strings.NewReader(strings.Repeat("n", cipher.aeadCipher.NonceSize())), tenant.TenantID, route.provider, CatalogCredentialAPIKey, "retirement-key")
				if err != nil {
					t.Fatal(err)
				}
				connection := managedProviderConnectionRecord{TenantID: tenant.TenantID, ProviderID: route.provider, FieldID: CatalogCredentialAPIKey, Value: encrypted, CreatedAt: now, UpdatedAt: now}
				event := managedUsageEventRecord{ID: uint(index + 1), TenantID: tenant.TenantID, Endpoint: usageEndpointText, ProviderID: route.provider, ModelID: route.model, StatusCode: 200, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess, TotalTokens: 9, CreatedAt: now}
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
			if scenario == "record failure" {
				if err = database.Exec(`CREATE TRIGGER reject_retirement BEFORE INSERT ON managed_schema_migration_records WHEN NEW.version = 15 BEGIN SELECT RAISE(ABORT, 'retirement record rejected'); END`).Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "completion record failure" {
				if err = database.Exec(`CREATE TRIGGER reject_retirement_completion BEFORE INSERT ON managed_schema_migration_records WHEN NEW.version = 15 AND EXISTS (SELECT 1 FROM managed_schema_migration_records WHERE version = 15) BEGIN SELECT RAISE(ABORT, 'retirement record rejected'); END`).Error; err != nil {
					t.Fatal(err)
				}
			}
			expectedFailure := "retirement record rejected"
			if scenario == "conflicting profile reasoning" {
				expectedFailure = "provider_reasoning_decision_required"
			}
			if scenario == "profile failure" || scenario == "tenant failure" {
				table := managedProviderProfileTable
				if scenario == "tenant failure" {
					table = managedTenantTable
				}
				if err = database.Exec("CREATE TRIGGER reject_retirement_update BEFORE UPDATE ON " + table + " BEGIN SELECT RAISE(ABORT, 'retirement update rejected'); END").Error; err != nil {
					t.Fatal(err)
				}
				expectedFailure = "retirement update rejected"
			}
			catalog := internalCanonicalProviderCatalog()
			if scenario == "missing policy" {
				schema := catalog.Schema()
				schema.ModelMigrations = slices.DeleteFunc(schema.ModelMigrations, func(migration ProviderCatalogModelMigration) bool { return migration.ManagedSchemaVersion == 15 })
				catalog, err = NewProviderCatalog(schema)
				if err != nil {
					t.Fatal(err)
				}
				expectedFailure = "operation=read_model_migrations version=15"
			}
			management := managedRouterTestManagementConfiguration()
			management.DatabaseDialector = database.Dialector
			payloads := make(chan map[string]any, 4)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				payloads <- payload
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"id":"retirement-message","type":"message","role":"assistant","content":[{"type":"text","text":"migrated result"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`)
			}))
			defer upstream.Close()
			endpoints := NewEndpoints()
			endpoints.SetProviderBaseURL(ProviderNameAnthropic, upstream.URL)
			configuration := Configuration{Endpoints: endpoints, ProviderCatalog: catalog, Management: management, AssetStorePath: t.TempDir()}
			if scenario == "profile read failure" {
				expectedFailure = "operation=preflight_reasoning"
				if err = database.Exec("ALTER TABLE " + managedProviderProfileTable + " RENAME COLUMN text_model TO unavailable_text_model").Error; err != nil {
					t.Fatal(err)
				}
			}
			router, err := BuildRouter(configuration, zap.NewNop().Sugar())
			if scenario == "profile read failure" {
				if restoreError := database.Exec("ALTER TABLE " + managedProviderProfileTable + " RENAME COLUMN unavailable_text_model TO text_model").Error; restoreError != nil {
					t.Fatal(restoreError)
				}
			}
			if failRecord {
				if err == nil || !strings.Contains(err.Error(), expectedFailure) {
					t.Fatalf("rollback error=%v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("startup=%v", err)
				}
				if _, err = BuildRouter(configuration, zap.NewNop().Sugar()); err != nil {
					t.Fatalf("repeated startup=%v", err)
				}
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, PublicCapabilitiesPath, nil))
				if recorder.Code != 200 || strings.Contains(recorder.Body.String(), `"claude-opus-4-1"`) {
					t.Fatalf("discovery status=%d still contains retired route", recorder.Code)
				}
			}
			if !failRecord {
				for index := range 2 {
					response := httptest.NewRecorder()
					router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?key=claude-retirement-secret-%d&prompt=test", index), nil))
					if response.Code != 200 {
						t.Fatalf("migrated default status=%d body=%s", response.Code, response.Body.String())
					}
					payload := <-payloads
					outputConfig, _ := payload["output_config"].(map[string]any)
					if payload["model"] != "claude-opus-5" || outputConfig["effort"] != "high" || payload["thinking"] != nil {
						t.Fatalf("default payload=%v", payload)
					}
				}
				deadline := time.Now().Add(time.Second)
				for {
					var currentUsage []managedUsageEventRecord
					if err := database.Where("id > ?", len(usage)).Find(&currentUsage).Error; err != nil {
						t.Fatalf("read migrated request usage: %v", err)
					}
					if len(currentUsage) == 2 {
						for _, event := range currentUsage {
							if event.ProviderID != ProviderNameAnthropic || event.ModelID != "claude-opus-5" || event.StatusCode != http.StatusOK {
								t.Fatalf("migrated request usage=%+v", event)
							}
						}
						break
					}
					if len(currentUsage) > 2 || time.Now().After(deadline) {
						t.Fatalf("persisted migrated request count=%d want=2", len(currentUsage))
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			var versionCount int64
			if err = database.Model(&managedSchemaMigrationRecord{}).Where("version = ?", 15).Count(&versionCount).Error; err != nil {
				t.Fatal(err)
			}
			expectedCount := int64(1)
			if failRecord {
				expectedCount = 0
			}
			if versionCount != expectedCount {
				t.Fatalf("migration records=%d expected=%d", versionCount, expectedCount)
			}
			for index, original := range tenants {
				var tenant managedTenantRecord
				var profile managedProviderProfileRecord
				var connection managedProviderConnectionRecord
				var event managedUsageEventRecord
				for _, query := range []struct {
					record any
					where  any
				}{{&tenant, &managedTenantRecord{TenantID: original.TenantID}}, {&profile, &managedProviderProfileRecord{TenantID: original.TenantID, ProviderID: original.DefaultProvider}}, {&connection, &managedProviderConnectionRecord{TenantID: original.TenantID, ProviderID: original.DefaultProvider}}, {&event, &managedUsageEventRecord{ID: usage[index].ID}}} {
					if err = database.Where(query.where).First(query.record).Error; err != nil {
						t.Fatal(err)
					}
				}
				expectedProfile := profiles[index]
				if !failRecord && index < 2 {
					original.DefaultModel = "claude-opus-5"
					original.DefaultReasoningEffort = "high"
					expectedProfile.TextModel = "claude-opus-5"
				}
				if !reflect.DeepEqual(tenant, original) || profile != expectedProfile || connection != connections[index] || event != usage[index] {
					t.Fatalf("state mismatch index=%d tenant=%+v expected=%+v profile=%+v usage=%+v", index, tenant, original, profile, event)
				}
			}
		})
	}
}

func TestClaudeRetirementPredecessorStartup(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprint(conflict), func(t *testing.T) {
			fixture := newManagedProviderConnectionsMigrationFixture(t, ProviderNameAnthropic, "claude-opus-4-1", "")
			if err := fixture.database.Create(&managedSchemaMigrationRecord{Version: 8, AppliedAt: time.Now().UTC()}).Error; err != nil {
				t.Fatal(err)
			}
			if conflict {
				if err := fixture.database.Model(&managedTenantRecord{}).Where("tenant_id = ?", fixture.predecessor.TenantID).UpdateColumns(map[string]any{"default_model": "claude-opus-5", "default_reasoning_effort": "max"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			management := managedRouterTestManagementConfiguration()
			management.DatabaseDialector = fixture.database.Dialector
			_, err := BuildRouter(Configuration{ProviderCatalog: internalCanonicalProviderCatalog(), Management: management, AssetStorePath: t.TempDir()}, zap.NewNop().Sugar())
			if conflict {
				if err == nil || !strings.Contains(err.Error(), "provider_reasoning_decision_required") {
					t.Fatalf("predecessor conflict=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var tenant managedTenantRecord
			if err = fixture.database.Where("tenant_id = ?", fixture.predecessor.TenantID).First(&tenant).Error; err != nil {
				t.Fatal(err)
			}
			if tenant.DefaultModel != "claude-opus-5" || tenant.DefaultReasoningEffort != "high" {
				t.Fatalf("predecessor defaults=%+v", tenant)
			}
		})
	}
}
