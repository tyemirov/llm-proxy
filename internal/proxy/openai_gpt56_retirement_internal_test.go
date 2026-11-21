package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var openAIRetirementMappings = []struct{ source, target, effort string }{
	{"gpt-4o-mini", "gpt-6-luna", "none"}, {"gpt-5-mini", "gpt-6-luna", "high"},
	{"gpt-4o", "gpt-6.2-sol", "low"}, {"gpt-4.1", "gpt-6.2-sol", ""},
	{"gpt-5", "gpt-6.2-sol", "medium"}, {"gpt-5.5", "gpt-6.2-sol", "xhigh"},
	{"gpt-5.5-pro", "gpt-6.2-astra", "max"},
}

func openAIRetirementDatabase(t *testing.T, predecessor bool) (*gorm.DB, []managedTenantRecord, []managedProviderProfileRecord, []managedUsageEventRecord) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "cutover.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if predecessor {
		createPredecessorAccountConnectionFixtureSchema(t, database)
	} else {
		if err := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry()); err != nil {
			t.Fatal(err)
		}
		if err := database.AutoMigrate(&managedSchemaMigrationRecord{}); err != nil {
			t.Fatal(err)
		}
		if err := database.Create(&managedSchemaMigrationRecord{Version: 17, AppliedAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	owner := managedUserRecord{UserID: "cutover-owner", CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	tenants := []managedTenantRecord{}
	profiles := []managedProviderProfileRecord{}
	usage := []managedUsageEventRecord{}
	cipher := internalManagedProviderKeyCipher()
	for index, mapping := range openAIRetirementMappings {
		tenant := fakeTenantRecord(owner.UserID, fmt.Sprintf("cutover-%d", index), fmt.Sprintf("Cutover %d", index), now)
		tenant.DefaultProvider = ProviderNameOpenAI
		tenant.DefaultModel = mapping.source
		tenant.DefaultReasoningEffort = mapping.effort
		tenant.DefaultSystemPrompt = "Keep exact prompt"
		if err := database.Create(&tenant).Error; err != nil {
			t.Fatal(err)
		}
		profile := managedProviderProfileRecord{TenantID: tenant.TenantID, ProviderID: ProviderNameOpenAI, TextModel: mapping.source, SystemPrompt: "Keep profile prompt", CreatedAt: now, UpdatedAt: now}
		if err := database.Create(&profile).Error; err != nil {
			t.Fatal(err)
		}
		if predecessor {
			encrypted, err := cipher.encryptConnection(strings.NewReader(strings.Repeat("n", 64)), tenant.TenantID, ProviderNameOpenAI, "api_key", "cutover-key")
			if err != nil {
				t.Fatal(err)
			}
			if err := database.Create(&managedProviderConnectionRecord{TenantID: tenant.TenantID, ProviderID: ProviderNameOpenAI, FieldID: "api_key", Value: encrypted, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
				t.Fatal(err)
			}
		} else {
			id := fmt.Sprintf("cutover-connection-%d", index)
			encrypted, err := cipher.encryptConnection(strings.NewReader(strings.Repeat("n", 64)), id, ProviderNameOpenAI, "api_key", "cutover-key")
			if err != nil {
				t.Fatal(err)
			}
			connection := managedAccountConnectionRecord{ID: id, OwnerUserID: owner.UserID, ProviderID: ProviderNameOpenAI, Name: tenant.Name, Version: 1, Fields: []managedConnectionFieldRecord{{ConnectionID: id, FieldID: "api_key", Value: encrypted, CreatedAt: now, UpdatedAt: now}}, CreatedAt: now, UpdatedAt: now}
			if err := database.Omit("Owner", "Assignments").Create(&connection).Error; err != nil {
				t.Fatal(err)
			}
			assignment := managedTenantConnectionRecord{TenantID: tenant.TenantID, ProviderID: ProviderNameOpenAI, ConnectionID: id, CreatedAt: now}
			if err := database.Omit("Connection").Create(&assignment).Error; err != nil {
				t.Fatal(err)
			}
		}
		event := managedUsageEventRecord{ID: uint(index + 1), TenantID: tenant.TenantID, Endpoint: usageEndpointV2, ProviderID: ProviderNameOpenAI, ModelID: mapping.source, StatusCode: 200, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess, TotalTokens: 12, CreatedAt: now}
		if err := database.Create(&event).Error; err != nil {
			t.Fatal(err)
		}
		tenants = append(tenants, tenant)
		profiles = append(profiles, profile)
		usage = append(usage, event)
	}
	return database, tenants, profiles, usage
}

func TestOpenAIGPT56RetirementStartup(t *testing.T) {
	for _, predecessor := range []bool{false, true} {
		t.Run(fmt.Sprintf("predecessor-%t", predecessor), func(t *testing.T) {
			database, beforeTenants, beforeProfiles, beforeUsage := openAIRetirementDatabase(t, predecessor)
			for attempt := 0; attempt < 2; attempt++ {
				if err := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry()); err != nil {
					t.Fatal(err)
				}
				for index, mapping := range openAIRetirementMappings {
					var tenant managedTenantRecord
					var profile managedProviderProfileRecord
					if err := database.Where(&managedTenantRecord{TenantID: beforeTenants[index].TenantID}).First(&tenant).Error; err != nil {
						t.Fatal(err)
					}
					if err := database.Where(&managedProviderProfileRecord{TenantID: tenant.TenantID, ProviderID: ProviderNameOpenAI}).First(&profile).Error; err != nil {
						t.Fatal(err)
					}
					wantTenant := beforeTenants[index]
					wantTenant.DefaultModel = mapping.target
					wantProfile := beforeProfiles[index]
					wantProfile.TextModel = mapping.target
					if !reflect.DeepEqual(tenant, wantTenant) || !reflect.DeepEqual(profile, wantProfile) {
						t.Fatalf("model-only migration changed retained state: index=%d tenant=%+v profile=%+v", index, tenant, profile)
					}
				}
				var afterUsage []managedUsageEventRecord
				if err := database.Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}}).Find(&afterUsage).Error; err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(beforeUsage, afterUsage) {
					t.Fatal("historical usage changed")
				}
				var marker managedSchemaMigrationRecord
				if err := database.Where(&managedSchemaMigrationRecord{Version: 18}).First(&marker).Error; err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestOpenAIGPT56RetirementPreflightAndRollback(t *testing.T) {
	for _, mode := range []string{"unsupported effort", "incompatible profile effort", "update failure", "marker failure"} {
		t.Run(mode, func(t *testing.T) {
			database, beforeTenants, beforeProfiles, _ := openAIRetirementDatabase(t, false)
			if mode == "unsupported effort" {
				beforeTenants[3].DefaultReasoningEffort = "none"
				if err := database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: beforeTenants[3].TenantID}).UpdateColumn("default_reasoning_effort", "none").Error; err != nil {
					t.Fatal(err)
				}
			}
			if mode == "incompatible profile effort" {
				beforeProfiles[0].TextModel = "gpt-4.1"
				if err := database.Model(&managedProviderProfileRecord{}).Where(&managedProviderProfileRecord{TenantID: beforeProfiles[0].TenantID, ProviderID: ProviderNameOpenAI}).UpdateColumn("text_model", "gpt-4.1").Error; err != nil {
					t.Fatal(err)
				}
			}
			writes := 0
			name := "cutover-write-observer"
			if err := database.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
				writes++
				if mode == "update failure" && tx.Statement.Table == managedProviderProfileTable {
					tx.AddError(errors.New("injected model update failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer database.Callback().Update().Remove(name)
			if mode == "marker failure" {
				if err := database.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
					if value, ok := tx.Statement.Dest.(*managedSchemaMigrationRecord); ok && value.Version == 18 {
						tx.AddError(errors.New("injected cutover marker failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				defer database.Callback().Create().Remove(name)
			}
			err := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
			if err == nil {
				t.Fatal("cutover unexpectedly succeeded")
			}
			if mode == "incompatible profile effort" && (!strings.Contains(err.Error(), "cutover-0") || !strings.Contains(err.Error(), "scope=provider_profile") || writes != 0) {
				t.Fatalf("profile preflight writes=%d error=%v", writes, err)
			}
			if mode == "unsupported effort" && (!strings.Contains(err.Error(), "cutover-3") || !strings.Contains(err.Error(), "gpt-4.1") || writes != 0) {
				t.Fatalf("preflight failed after writes or without source identity: writes=%d error=%v", writes, err)
			}
			for index, before := range beforeTenants {
				var tenant managedTenantRecord
				var profile managedProviderProfileRecord
				if err := database.Where(&managedTenantRecord{TenantID: before.TenantID}).First(&tenant).Error; err != nil {
					t.Fatal(err)
				}
				if err := database.Where(&managedProviderProfileRecord{TenantID: before.TenantID, ProviderID: ProviderNameOpenAI}).First(&profile).Error; err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(tenant, before) || !reflect.DeepEqual(profile, beforeProfiles[index]) {
					t.Fatal("failed transaction changed selected model or timestamps")
				}
			}
		})
	}
}

func TestOpenAIGPT56RetirementAcceptedRequestReconciliation(t *testing.T) {
	for _, initialState := range []string{structuredRequestStateDispatched, structuredRequestStateUncertain} {
		t.Run(initialState, func(t *testing.T) {
			root := t.TempDir()
			requestTenant := managedTenantForInternalTest("old-request-tenant", "old-request-secret")
			store, err := newStructuredRequestStore(root, 3600, nil)
			if err != nil {
				t.Fatal(err)
			}
			key := "retained-old-request"
			intent := structuredRequestIntent(ProviderNameOpenAI, "gpt-4.1", []byte(`{"model":"gpt-4.1","messages":[{"role":"user","content":"original"}]}`))
			accepted, created, err := store.begin(requestTenant, key, intent, ProviderNameOpenAI, "gpt-4.1", "original-provider-request")
			if err != nil || !created {
				t.Fatalf("accepted=%+v error=%v", accepted, err)
			}
			if claimed, err := store.claimDispatch(requestTenant, key, intent); err != nil || !claimed {
				t.Fatalf("claim=%t error=%v", claimed, err)
			}
			if initialState == structuredRequestStateUncertain {
				if err := store.uncertain(requestTenant, key, intent); err != nil {
					t.Fatal(err)
				}
			}
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
			t.Cleanup(upstream.Close)
			endpoints := NewEndpoints()
			endpoints.SetResponsesURL(upstream.URL)
			configuration := StandardManagedTenantTestConfiguration("old-request-secret")
			configuration.ID = "old-request-tenant"
			router, err := BuildRouterWithManagedTenantForTest(t, Configuration{AssetStorePath: root, AssetRetentionSeconds: 3600, Endpoints: endpoints}, zap.NewNop().Sugar(), configuration)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			readback := func(want int) string {
				request, _ := http.NewRequest(http.MethodGet, server.URL+"/v2/requests?key=old-request-secret", nil)
				request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if response.StatusCode != want || calls.Load() != 0 {
					t.Fatalf("readback status=%d calls=%d body=%s", response.StatusCode, calls.Load(), body)
				}
				return string(body)
			}
			if body := readback(http.StatusConflict); !strings.Contains(body, "structured_request_outcome_unknown") {
				t.Fatalf("body=%s", body)
			}
			retained, err := store.lookup(requestTenant, key)
			if err != nil || retained.Model != "gpt-4.1" || retained.IntentSHA256 != intent || retained.ProxyRequestID != accepted.ProxyRequestID {
				t.Fatalf("original binding changed: %+v error=%v", retained, err)
			}
			// Inject the response evidence for the already accepted original execution.
			if err := store.succeed(requestTenant, key, intent, `{"original_result":true}`); err != nil {
				t.Fatal(err)
			}
			if body := readback(http.StatusOK); !jsonEqual([]byte(body), []byte(`{"original_result":true}`)) {
				t.Fatalf("result changed: %s", body)
			}
			retained, err = store.lookup(requestTenant, key)
			if err != nil || retained.Model != "gpt-4.1" || retained.IntentSHA256 != intent {
				t.Fatal("reconciliation substituted a new route")
			}
		})
	}
}

// The current tenant table references the user table's user_id column. Column
// inspection must distinguish that reference from a tenant-owned user_id field.
func TestOpenAIGPT56RetirementPreNineTenantBindings(t *testing.T) {
	for _, version := range []int{managedXAIProviderSchemaVersion, managedZAIProviderSchemaVersion} {
		t.Run(fmt.Sprintf("version-%d", version), func(t *testing.T) {
			for _, effort := range []string{"high", "none"} {
				t.Run(effort, func(t *testing.T) {
					fixture := newOpenAIProviderConnectionsMigrationFixture(t)
					database := fixture.database
					if err := database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: fixture.predecessor.TenantID}).UpdateColumns(map[string]any{"default_model": "gpt-5.6-terra", "default_reasoning_effort": effort}).Error; err != nil {
						t.Fatal(err)
					}
					second := fakeTenantRecord("provider-connections-owner", "second-current-tenant", "Second", fixture.predecessor.CreatedAt)
					second.DefaultProvider, second.DefaultModel, second.DefaultReasoningEffort = ProviderNameOpenAI, "gpt-6-luna", "none"
					if err := database.Create(&second).Error; err != nil {
						t.Fatal(err)
					}
					encrypted, err := fixture.providerKeyCipher.encrypt(strings.NewReader(strings.Repeat("s", 64)), second.TenantID, ProviderNameOpenAI, "second-provider-key")
					if err != nil {
						t.Fatal(err)
					}
					key := managedProviderAPIKeyRecord{TenantID: second.TenantID, ProviderID: ProviderNameOpenAI, TextModel: "gpt-5-mini", EncryptedAPIKey: encrypted, CreatedAt: second.CreatedAt, UpdatedAt: second.UpdatedAt}
					if err := database.Create(&key).Error; err != nil {
						t.Fatal(err)
					}
					if err := database.Create(&managedSchemaMigrationRecord{Version: version, AppliedAt: second.CreatedAt}).Error; err != nil {
						t.Fatal(err)
					}
					var before []managedTenantRecord
					if err := database.Order(clause.OrderByColumn{Column: clause.Column{Name: "tenant_id"}}).Find(&before).Error; err != nil {
						t.Fatal(err)
					}
					err = initializeManagedTenantSchema(database, fixture.providerKeyCipher, fixture.providers)
					if effort == "none" {
						if err == nil || !strings.Contains(err.Error(), "tenant="+fixture.predecessor.TenantID) || !strings.Contains(err.Error(), "scope=provider_profile") || !strings.Contains(err.Error(), "source_model=gpt-4.1") {
							t.Fatalf("incompatible profile must identify its own tenant: %v", err)
						}
						var after []managedTenantRecord
						if err := database.Order(clause.OrderByColumn{Column: clause.Column{Name: "tenant_id"}}).Find(&after).Error; err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(before, after) {
							t.Fatal("incompatible inherited effort changed tenant state")
						}
						var retainedKeys []managedProviderAPIKeyRecord
						if err := database.Find(&retainedKeys).Error; err != nil || len(retainedKeys) != 2 {
							t.Fatalf("predecessor keys changed: count=%d error=%v", len(retainedKeys), err)
						}
						return
					}
					if err != nil {
						t.Fatalf("independent valid tenant selections rejected: %v", err)
					}
					var after []managedTenantRecord
					if err := database.Order(clause.OrderByColumn{Column: clause.Column{Name: "tenant_id"}}).Find(&after).Error; err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(before, after) {
						t.Fatal("profile-only cutover changed retained tenant defaults, efforts or timestamps")
					}
					for tenantID, target := range map[string]string{fixture.predecessor.TenantID: "gpt-6.2-sol", second.TenantID: "gpt-6-luna"} {
						var profile managedProviderProfileRecord
						if err := database.Where(&managedProviderProfileRecord{TenantID: tenantID, ProviderID: ProviderNameOpenAI}).First(&profile).Error; err != nil || profile.TextModel != target {
							t.Fatalf("tenant=%s profile=%+v error=%v", tenantID, profile, err)
						}
					}
				})
			}
		})
	}
}

func openAIRetirementState(t *testing.T, database *gorm.DB) string {
	t.Helper()
	state := map[string]any{}
	tables, err := database.Migrator().GetTables()
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(tables)
	state["tables"] = tables
	for _, record := range []struct {
		table string
		rows  any
		order []string
	}{
		{managedTenantTable, &[]managedTenantRecord{}, []string{"tenant_id"}},
		{managedProviderProfileTable, &[]managedProviderProfileRecord{}, []string{"tenant_id", "provider_id"}},
		{managedProviderKeyTable, &[]managedProviderAPIKeyRecord{}, []string{"tenant_id", "provider_id"}},
		{managedUsageEventTable, &[]managedUsageEventRecord{}, []string{"id"}},
		{managedSchemaMigrationTable, &[]managedSchemaMigrationRecord{}, []string{"version"}},
		{"managed_account_connection_records", &[]managedAccountConnectionRecord{}, []string{"id"}},
		{"managed_connection_field_records", &[]managedConnectionFieldRecord{}, []string{"connection_id", "field_id"}},
		{"managed_tenant_connection_records", &[]managedTenantConnectionRecord{}, []string{"tenant_id", "provider_id"}},
	} {
		if !database.Migrator().HasTable(record.table) {
			continue
		}
		if record.table == managedTenantTable || record.table == managedProviderKeyTable {
			fields, err := database.Migrator().ColumnTypes(record.table)
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range fields {
				if field.Name() == "user_id" {
					if record.table == managedTenantTable {
						record.rows, record.order = &[]legacyManagedTenantRecord{}, []string{"user_id"}
					} else {
						record.rows, record.order = &[]legacyManagedProviderAPIKeyRecord{}, []string{"user_id", "provider_id"}
					}
				}
			}
		}
		columns := []clause.OrderByColumn{}
		for _, column := range record.order {
			columns = append(columns, clause.OrderByColumn{Column: clause.Column{Name: column}})
		}
		if err := database.Table(record.table).Order(clause.OrderBy{Columns: columns}).Find(record.rows).Error; err != nil {
			t.Fatal(err)
		}
		state[record.table] = record.rows
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func openAIRetirementObserveWrites(t *testing.T, database *gorm.DB) *int {
	t.Helper()
	writes := new(int)
	observe := func(*gorm.DB) { *writes++ }
	for _, callback := range []struct {
		register func(string, func(*gorm.DB)) error
		remove   func(string) error
	}{
		{database.Callback().Create().Before("gorm:create").Register, database.Callback().Create().Remove},
		{database.Callback().Update().Before("gorm:update").Register, database.Callback().Update().Remove},
		{database.Callback().Delete().Before("gorm:delete").Register, database.Callback().Delete().Remove},
	} {
		if err := callback.register("cutover-observe-writes", observe); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { callback.remove("cutover-observe-writes") })
	}
	return writes
}

type openAIRetirementFailureDialector struct {
	gorm.Dialector
	stage string
}
type openAIRetirementFailureMigrator struct {
	gorm.Migrator
	stage string
}

func (dialector openAIRetirementFailureDialector) Migrator(database *gorm.DB) gorm.Migrator {
	return openAIRetirementFailureMigrator{dialector.Dialector.Migrator(database), dialector.stage}
}
func (migrator openAIRetirementFailureMigrator) ColumnTypes(model any) ([]gorm.ColumnType, error) {
	if migrator.stage == "inspect_columns" {
		return nil, errInternalTestDatabase
	}
	return migrator.Migrator.ColumnTypes(model)
}
func (migrator openAIRetirementFailureMigrator) AutoMigrate(models ...any) error {
	for _, model := range models {
		if _, marker := model.(*managedSchemaMigrationRecord); marker && migrator.stage == "marker_schema" {
			return errInternalTestDatabase
		}
	}
	return migrator.Migrator.AutoMigrate(models...)
}

func TestOpenAIGPT56RetirementMetadataFailuresAreReadOnly(t *testing.T) {
	for _, failure := range []struct {
		name, table string
		occurrence  int
	}{
		{"tenant rows", managedTenantTable, 1}, {"legacy tenant rows", managedTenantTable, 1}, {"provider profiles", managedProviderProfileTable, 1},
		{"predecessor keys", managedProviderKeyTable, 1}, {"legacy predecessor keys", managedProviderKeyTable, 1}, {"cutover marker", managedSchemaMigrationTable, 1},
		{"predecessor marker count", managedSchemaMigrationTable, 2}, {"inspect_columns", "", 0},
	} {
		t.Run(failure.name, func(t *testing.T) {
			database, _, _, _ := openAIRetirementDatabase(t, false)
			if strings.HasPrefix(failure.name, "legacy ") {
				database, _ = openAIRetirementLegacySource(t)
			} else if failure.table == managedProviderKeyTable {
				fixture := newOpenAIProviderConnectionsMigrationFixture(t)
				database = fixture.database
			}
			before := openAIRetirementState(t, database)
			writes := openAIRetirementObserveWrites(t, database)
			original := database.Dialector
			if failure.name == "inspect_columns" {
				database.Config.Dialector = openAIRetirementFailureDialector{original, failure.name}
			}
			queries := 0
			if err := database.Callback().Query().Before("gorm:query").Register("cutover-metadata-failure", func(tx *gorm.DB) {
				if tx.Statement.Table == failure.table {
					queries++
					if queries == failure.occurrence {
						tx.AddError(errInternalTestDatabase)
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			err := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
			database.Callback().Query().Remove("cutover-metadata-failure")
			database.Config.Dialector = original
			if !errors.Is(err, errInternalTestDatabase) || *writes != 0 {
				t.Fatalf("metadata failure writes=%d error=%v", *writes, err)
			}
			if failure.name == "inspect_columns" && !strings.Contains(err.Error(), "inspect_columns") {
				t.Fatalf("inspection failure lacks context: %v", err)
			}
			if after := openAIRetirementState(t, database); after != before {
				t.Fatal("metadata failure changed persisted state or schema")
			}
		})
	}
}

func TestOpenAIGPT56RetirementCompletedMarkerRejectsReintroducedSelections(t *testing.T) {
	for _, scope := range []string{"default", "profile"} {
		t.Run(scope, func(t *testing.T) {
			database, tenants, profiles, _ := openAIRetirementDatabase(t, false)
			if err := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry()); err != nil {
				t.Fatal(err)
			}
			if scope == "default" {
				if err := database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: tenants[3].TenantID}).UpdateColumn("default_model", "gpt-4.1").Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := database.Model(&managedProviderProfileRecord{}).Where(&managedProviderProfileRecord{TenantID: profiles[0].TenantID, ProviderID: ProviderNameOpenAI}).UpdateColumn("text_model", "gpt-4o-mini").Error; err != nil {
					t.Fatal(err)
				}
			}
			before := openAIRetirementState(t, database)
			writes := openAIRetirementObserveWrites(t, database)
			err := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
			if err == nil || !strings.Contains(err.Error(), "retired_selection_after_cutover") || *writes != 0 {
				t.Fatalf("retired state writes=%d error=%v", *writes, err)
			}
			if after := openAIRetirementState(t, database); after != before {
				t.Fatal("reintroduced selection was rewritten")
			}
		})
	}
}

func TestOpenAIGPT56RetirementLateFailuresRollbackCompleteState(t *testing.T) {
	for _, failure := range []string{"selected profile read", "selected default read", "profile update", "default update", "marker_schema", "marker create"} {
		t.Run(failure, func(t *testing.T) {
			database, _, _, _ := openAIRetirementDatabase(t, false)
			before := openAIRetirementState(t, database)
			original := database.Dialector
			if failure == "marker_schema" {
				database.Config.Dialector = openAIRetirementFailureDialector{original, failure}
			}
			profileReads, tenantReads := 0, 0
			if err := database.Callback().Query().Before("gorm:query").Register("cutover-late-read", func(tx *gorm.DB) {
				if tx.Statement.Table == managedProviderProfileTable {
					profileReads++
					if failure == "selected profile read" && profileReads == 2 {
						tx.AddError(errInternalTestDatabase)
					}
				}
				if tx.Statement.Table == managedTenantTable {
					tenantReads++
					if failure == "selected default read" && tenantReads == 2 {
						tx.AddError(errInternalTestDatabase)
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			if err := database.Callback().Update().Before("gorm:update").Register("cutover-late-update", func(tx *gorm.DB) {
				if (failure == "profile update" && tx.Statement.Table == managedProviderProfileTable) || (failure == "default update" && tx.Statement.Table == managedTenantTable) {
					tx.AddError(errInternalTestDatabase)
				}
			}); err != nil {
				t.Fatal(err)
			}
			if err := database.Callback().Create().Before("gorm:create").Register("cutover-late-marker", func(tx *gorm.DB) {
				if marker, ok := tx.Statement.Dest.(*managedSchemaMigrationRecord); ok && failure == "marker create" && marker.Version == 18 {
					tx.AddError(errInternalTestDatabase)
				}
			}); err != nil {
				t.Fatal(err)
			}
			err := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
			database.Callback().Query().Remove("cutover-late-read")
			database.Callback().Update().Remove("cutover-late-update")
			database.Callback().Create().Remove("cutover-late-marker")
			database.Config.Dialector = original
			if !errors.Is(err, errManagedTenantSchemaMigration) || !strings.Contains(err.Error(), errInternalTestDatabase.Error()) {
				t.Fatalf("late persistence failure: %v", err)
			}
			if after := openAIRetirementState(t, database); after != before {
				t.Fatal("late failure changed models, efforts, timestamps, credentials, prompts, assignments, usage or markers")
			}
		})
	}
}

func openAIRetirementLegacySource(t *testing.T) (*gorm.DB, legacyManagedTenantRecord) {
	t.Helper()
	database := openLegacyManagedTenantDatabase(t, filepath.Join(t.TempDir(), "legacy-cutover.db"))
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	tenant := legacyManagedTenantRecord{UserID: "legacy-cutover-owner", TenantID: "legacy-cutover-tenant", DefaultProvider: ProviderNameOpenAI, DefaultModel: "gpt-4.1", DefaultReasoningEffort: "high", CreatedAt: now, UpdatedAt: now}
	if err := database.Table(managedTenantTable).Create(&tenant).Error; err != nil {
		t.Fatal(err)
	}
	encrypted, err := internalManagedProviderKeyCipher().encrypt(strings.NewReader(strings.Repeat("x", 64)), tenant.UserID, ProviderNameOpenAI, "legacy-provider-key")
	if err != nil {
		t.Fatal(err)
	}
	key := legacyManagedProviderAPIKeyRecord{UserID: tenant.UserID, ProviderID: ProviderNameOpenAI, TextModel: "gpt-4.1", EncryptedAPIKey: encrypted, CreatedAt: now, UpdatedAt: now}
	if err := database.Table(managedProviderKeyTable).Create(&key).Error; err != nil {
		t.Fatal(err)
	}
	return database, tenant
}

func TestOpenAIGPT56RetirementLegacyNormalizationPreservesMappedSelections(t *testing.T) {
	database, before := openAIRetirementLegacySource(t)
	if err := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry()); err != nil {
		t.Fatal(err)
	}
	var tenant managedTenantRecord
	var profile managedProviderProfileRecord
	if err := database.Where(&managedTenantRecord{TenantID: before.TenantID}).First(&tenant).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Where(&managedProviderProfileRecord{TenantID: before.TenantID, ProviderID: ProviderNameOpenAI}).First(&profile).Error; err != nil {
		t.Fatal(err)
	}
	if tenant.DefaultModel != "gpt-6.2-sol" || profile.TextModel != "gpt-6.2-sol" || tenant.DefaultReasoningEffort != before.DefaultReasoningEffort || tenant.CreatedAt != before.CreatedAt || tenant.UpdatedAt != before.UpdatedAt || profile.CreatedAt != before.CreatedAt || profile.UpdatedAt != before.UpdatedAt {
		t.Fatal("normalization/cutover changed retained effort or timestamps")
	}
}

func TestOpenAIGPT56RetirementRejectsOrphanSelections(t *testing.T) {
	for _, scope := range []string{"profile", "legacy key"} {
		t.Run(scope, func(t *testing.T) {
			var database *gorm.DB
			if scope == "legacy key" {
				database, _ = openAIRetirementLegacySource(t)
				if err := database.Table(managedProviderKeyTable).Where(&legacyManagedProviderAPIKeyRecord{ProviderID: ProviderNameOpenAI}).UpdateColumn("user_id", "absent-owner").Error; err != nil {
					t.Fatal(err)
				}
			} else {
				database, _, _, _ = openAIRetirementDatabase(t, false)
				if err := database.Migrator().DropConstraint(&managedProviderProfileRecord{}, "fk_managed_tenant_records_provider_profiles"); err != nil {
					t.Fatal(err)
				}
				if err := database.Create(&managedProviderProfileRecord{TenantID: "absent-tenant", ProviderID: ProviderNameOpenAI, TextModel: "gpt-4.1"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			writes := openAIRetirementObserveWrites(t, database)
			err := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
			if err == nil || !strings.Contains(err.Error(), "orphan_") || *writes != 0 {
				t.Fatalf("orphan rejection writes=%d error=%v", *writes, err)
			}
		})
	}
}

func TestOpenAIGPT56RetirementRejectsNonPreservingPolicy(t *testing.T) {
	for _, change := range []string{"effort", "usage"} {
		t.Run(change, func(t *testing.T) {
			schema := internalCanonicalProviderCatalog().Schema()
			for index := range schema.ModelMigrations {
				if schema.ModelMigrations[index].ManagedSchemaVersion == 18 {
					if change == "effort" {
						schema.ModelMigrations[index].TargetReasoningEffort = "medium"
					} else {
						schema.ModelMigrations[index].PreserveSourceUsage = false
					}
					break
				}
			}
			catalog, err := NewProviderCatalog(schema)
			if err != nil {
				t.Fatalf("fixture must pass generic catalog boundary: %v", err)
			}
			database, _, _, _ := openAIRetirementDatabase(t, false)
			before := openAIRetirementState(t, database)
			writes := openAIRetirementObserveWrites(t, database)
			err = initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), newProviderRegistry(Configuration{ProviderCatalog: catalog}))
			if err == nil || !strings.Contains(err.Error(), "invalid_preservation_policy") || *writes != 0 {
				t.Fatalf("nonpreserving cutover writes=%d error=%v", *writes, err)
			}
			if after := openAIRetirementState(t, database); after != before {
				t.Fatal("nonpreserving policy changed persisted state")
			}
		})
	}
}
