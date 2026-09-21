package proxy_test

import (
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const capabilityTenantTable = "managed_tenant_records"

func previousCapabilityDefaults(t *testing.T, database *gorm.DB) {
	t.Helper()
	for _, statement := range []string{
		"ALTER TABLE managed_tenant_records RENAME COLUMN default_transcription_provider TO default_dictation_provider",
		"ALTER TABLE managed_tenant_records RENAME COLUMN default_transcription_model TO default_dictation_model",
		"ALTER TABLE managed_tenant_records DROP COLUMN default_speech_provider",
		"ALTER TABLE managed_tenant_records DROP COLUMN default_speech_model",
	} {
		if err := database.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func capabilityDatabaseSnapshot(t *testing.T, database *gorm.DB) map[string][]map[string]any {
	t.Helper()
	tables, err := database.Migrator().GetTables()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := make(map[string][]map[string]any, len(tables)+1)
	for _, table := range append(tables, "sqlite_master") {
		var records []map[string]any
		if err := database.Table(table).Find(&records).Error; err != nil {
			t.Fatal(err)
		}
		snapshot[table] = records
	}
	return snapshot
}

func TestManagementCapabilityMigrationPreservesDataAndRestart(t *testing.T) {
	for _, populated := range []bool{false, true} {
		name := "empty defaults"
		if populated {
			name = "saved defaults"
		}
		t.Run(name, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "capabilities.sqlite")
			router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
			owner := managementSessionCookie(t, "capability-migration-owner")
			tenantID := managementDefaultTenantTestID(t, router, owner)
			path := "/tenants/" + tenantID
			if populated {
				fixture, listener := newDictatorAcceptanceUpstream(t, proxy.ProviderNameDictator)
				response := putManagementProviderKey(t, router, owner, tenantID, proxy.ProviderNameOpenAI, testManagementOpenAIKey, proxy.ModelNameGPT41, "Provider prompt", t.Context())
				if response.Code != http.StatusOK {
					t.Fatalf("create text connection: status=%d", response.Code)
				}
				connection := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{
					"name": "Transcription", "provider": "dictator",
					"fields": map[string]string{"grpc_address": listener.Addr().String(), "grpc_auth_token": fixture.token, "grpc_tls": "false"},
				}, http.StatusCreated)
				accountConnectionExchange(t, router, owner, http.MethodPut, path+"/connections/dictator", map[string]any{"connection_id": connection["id"]}, http.StatusOK)
				accountConnectionExchange(t, router, owner, http.MethodPut, path+"/defaults", map[string]string{
					"provider": "openai", "model": proxy.ModelNameGPT41,
					"transcription_provider": "dictator", "transcription_model": "whisper-base",
					"system_prompt": "Keep the tenant prompt.", "reasoning_effort": "",
				}, http.StatusOK)
				generateManagementTenantSecret(t, router, owner, tenantID)
			}
			expected := accountConnectionExchange(t, router, owner, http.MethodGet, path, nil, http.StatusOK)
			database := openManagedFixtureDatabase(t, databasePath)
			if err := database.Exec("INSERT INTO managed_usage_event_records (tenant_id, endpoint, provider_id, model_id, status_code, disposition, outcome_code, created_at) VALUES (?, '/', 'openai', 'gpt-4.1', 200, 'succeeded', 'success', CURRENT_TIMESTAMP)", tenantID).Error; err != nil {
				t.Fatal(err)
			}
			before := capabilityDatabaseSnapshot(t, database)
			previousCapabilityDefaults(t, database)
			restarted := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
			after := capabilityDatabaseSnapshot(t, database)
			delete(before, "sqlite_master")
			delete(after, "sqlite_master")
			if !reflect.DeepEqual(before, after) {
				t.Fatal("migration changed retained records")
			}
			actual := accountConnectionExchange(t, restarted, owner, http.MethodGet, path, nil, http.StatusOK)
			if !reflect.DeepEqual(expected, actual) {
				t.Fatal("migration changed the public tenant profile")
			}
			for _, column := range []string{"default_dictation_provider", "default_dictation_model"} {
				if database.Migrator().HasColumn(capabilityTenantTable, column) {
					t.Fatalf("migration retained obsolete column %s", column)
				}
			}
			beforeRestart := capabilityDatabaseSnapshot(t, database)
			restarted = newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
			if !reflect.DeepEqual(beforeRestart, capabilityDatabaseSnapshot(t, database)) {
				t.Fatal("current restart replayed migration or changed records")
			}
			accountConnectionExchange(t, restarted, owner, http.MethodGet, path, nil, http.StatusOK)
		})
	}
}

func TestManagementCapabilityMigrationRejectsAmbiguousSchema(t *testing.T) {
	for _, statement := range []string{
		"ALTER TABLE managed_tenant_records DROP COLUMN default_dictation_model",
		"ALTER TABLE managed_tenant_records ADD COLUMN default_transcription_provider TEXT",
		"ALTER TABLE managed_tenant_records ADD COLUMN default_speech_model TEXT",
		"UPDATE managed_tenant_records SET default_dictation_provider = 'unknown', default_dictation_model = 'unknown'",
	} {
		t.Run(statement, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "rejected.sqlite")
			router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
			managementDefaultTenantTestID(t, router, managementSessionCookie(t, "rejected-migration-owner"))
			database := openManagedFixtureDatabase(t, databasePath)
			previousCapabilityDefaults(t, database)
			if err := database.Exec(statement).Error; err != nil {
				t.Fatal(err)
			}
			before := capabilityDatabaseSnapshot(t, database)
			_, err := buildRouterWithCatalogs(t, managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath), zap.NewNop().Sugar())
			if err == nil {
				t.Fatal("startup accepted an ambiguous schema or invalid saved route")
			}
			if !reflect.DeepEqual(before, capabilityDatabaseSnapshot(t, database)) {
				t.Fatal("rejected migration changed the database")
			}
		})
	}
}

type capabilityMigrationFailureDialector struct {
	gorm.Dialector
	column string
}

func (dialector capabilityMigrationFailureDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	if dialector.column != "initialize_speech_defaults" {
		return nil
	}
	return database.Callback().Update().Before("gorm:update").Register("capability_update_failure", func(query *gorm.DB) {
		query.AddError(errors.New("injected capability migration failure"))
	})
}

func (dialector capabilityMigrationFailureDialector) Migrator(database *gorm.DB) gorm.Migrator {
	return capabilityMigrationFailureMigrator{Migrator: dialector.Dialector.Migrator(database), column: dialector.column}
}

type capabilityMigrationFailureMigrator struct {
	gorm.Migrator
	column string
}

func (migrator capabilityMigrationFailureMigrator) ColumnTypes(model any) ([]gorm.ColumnType, error) {
	if migrator.column == "inspect_capability_defaults" {
		return nil, errors.New("injected capability migration failure")
	}
	return migrator.Migrator.ColumnTypes(model)
}

func (migrator capabilityMigrationFailureMigrator) RenameColumn(model any, oldName, newName string) error {
	if newName == migrator.column {
		return errors.New("injected capability migration failure")
	}
	return migrator.Migrator.RenameColumn(model, oldName, newName)
}

func (migrator capabilityMigrationFailureMigrator) AddColumn(model any, name string) error {
	if name == migrator.column {
		return errors.New("injected capability migration failure")
	}
	return migrator.Migrator.AddColumn(model, name)
}

func TestManagementCapabilityMigrationFailureRollsBack(t *testing.T) {
	for _, column := range []string{"inspect_capability_defaults", "default_transcription_provider", "default_transcription_model", "default_speech_provider", "default_speech_model", "initialize_speech_defaults"} {
		t.Run(column, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "rollback.sqlite")
			router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
			managementDefaultTenantTestID(t, router, managementSessionCookie(t, "rollback-owner"))
			database := openManagedFixtureDatabase(t, databasePath)
			previousCapabilityDefaults(t, database)
			before := capabilityDatabaseSnapshot(t, database)
			configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath)
			configuration.Management.DatabaseDialector = capabilityMigrationFailureDialector{Dialector: sqlite.Open(databasePath), column: column}
			_, err := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
			if err == nil || !strings.Contains(err.Error(), "injected capability migration failure") || !strings.Contains(err.Error(), column) {
				t.Fatalf("migration error=%v, want injected failure with column context", err)
			}
			if !reflect.DeepEqual(before, capabilityDatabaseSnapshot(t, database)) {
				t.Fatal("failed migration changed the database")
			}
		})
	}
}

func TestManagementCapabilityMigrationBeforeAccountConnectionTransfer(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "predecessor.sqlite")
	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	owner := managementSessionCookie(t, "predecessor-capability-owner")
	tenantID := managementDefaultTenantTestID(t, router, owner)
	path := "/tenants/" + tenantID
	expected := accountConnectionExchange(t, router, owner, http.MethodGet, path, nil, http.StatusOK)
	database := openManagedFixtureDatabase(t, databasePath)
	previousCapabilityDefaults(t, database)
	for _, table := range []string{"managed_connection_creation_records", "managed_connection_field_records", "managed_tenant_connection_records", "managed_account_connection_records"} {
		if err := database.Migrator().DropTable(table); err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		"CREATE TABLE managed_provider_connection_records (tenant_id TEXT, provider_id TEXT, field_id TEXT, value TEXT, created_at DATETIME, updated_at DATETIME)",
		"CREATE TABLE managed_schema_migration_records (version INTEGER PRIMARY KEY, applied_at DATETIME)",
		"INSERT INTO managed_schema_migration_records (version) VALUES (16)",
	} {
		if err := database.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	restarted := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	actual := accountConnectionExchange(t, restarted, owner, http.MethodGet, path, nil, http.StatusOK)
	if !reflect.DeepEqual(expected, actual) {
		t.Fatal("predecessor transfer changed the public tenant profile")
	}
	if database.Migrator().HasTable("managed_provider_connection_records") || database.Migrator().HasColumn(capabilityTenantTable, "default_dictation_provider") {
		t.Fatal("predecessor transfer retained obsolete storage")
	}
}
