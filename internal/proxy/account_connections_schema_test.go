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

func TestAccountConnectionFreshSchemaAndRestart(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "current.db")
	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	database := openManagedFixtureDatabase(t, databasePath)
	if database.Migrator().HasTable("managed_schema_migration_records") {
		t.Fatal("fresh database must use a versionless schema without migration records")
	}
	for _, obsolete := range []string{"managed_provider_api_key_records", "managed_provider_connection_records", "managed_tenant_records_f014_legacy", "managed_usage_event_records_f037_legacy"} {
		if database.Migrator().HasTable(obsolete) {
			t.Fatalf("fresh database contains predecessor table %s", obsolete)
		}
	}
	owner := managementSessionCookie(t, "current-schema-owner")
	created := accountConnectionExchange(t, router, owner, http.MethodPost, "/connections", map[string]any{
		"name": "Current connection", "provider": "openai", "fields": map[string]string{"api_key": testManagementOpenAIKey},
	}, http.StatusCreated)
	path := "/connections/" + created["id"].(string)
	before := accountConnectionExchange(t, router, owner, http.MethodGet, path, nil, http.StatusOK)
	// Historical bookkeeping is not authority for the current database shape.
	if err := database.Exec("CREATE TABLE managed_schema_migration_records (version INTEGER PRIMARY KEY, applied_at DATETIME)").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("INSERT INTO managed_schema_migration_records (version) VALUES (999)").Error; err != nil {
		t.Fatal(err)
	}
	var schemaBefore []map[string]any
	if err := database.Raw("SELECT name, sql FROM sqlite_master ORDER BY name").Scan(&schemaBefore).Error; err != nil {
		t.Fatal(err)
	}
	restarted := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	after := accountConnectionExchange(t, restarted, owner, http.MethodGet, path, nil, http.StatusOK)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("restart changed connection: before=%v after=%v", before, after)
	}
	var schemaAfter []map[string]any
	if err := database.Raw("SELECT name, sql FROM sqlite_master ORDER BY name").Scan(&schemaAfter).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schemaBefore, schemaAfter) {
		t.Fatal("restart changed schema records")
	}
}

type currentSchemaFailureDialector struct {
	gorm.Dialector
	stage string
}

func (dialector currentSchemaFailureDialector) Migrator(database *gorm.DB) gorm.Migrator {
	return currentSchemaFailureMigrator{Migrator: dialector.Dialector.Migrator(database), stage: dialector.stage}
}

type currentSchemaFailureMigrator struct {
	gorm.Migrator
	stage string
}

func (migrator currentSchemaFailureMigrator) GetTables() ([]string, error) {
	if migrator.stage == "inspect_schema" {
		return nil, errors.New("injected schema read failure")
	}
	return migrator.Migrator.GetTables()
}

func (migrator currentSchemaFailureMigrator) AutoMigrate(models ...any) error {
	if err := migrator.Migrator.AutoMigrate(models...); err != nil {
		return err
	}
	return errors.New("injected schema creation failure")
}

func TestAccountConnectionFreshSchemaFailureIsAtomic(t *testing.T) {
	for _, stage := range []string{"inspect_schema", "create_current_schema"} {
		t.Run(stage, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "failure.db")
			configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath)
			configuration.Management.DatabaseDialector = currentSchemaFailureDialector{Dialector: sqlite.Open(databasePath), stage: stage}
			_, err := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
			if err == nil || !strings.Contains(err.Error(), "operation="+stage) {
				t.Fatalf("startup error=%v, want operation=%s", err, stage)
			}
			database := openManagedFixtureDatabase(t, databasePath)
			tables, err := database.Migrator().GetTables()
			if err != nil || len(tables) != 0 {
				t.Fatalf("failed startup changed database: tables=%v error=%v", tables, err)
			}
		})
	}
}

func TestAccountConnectionCurrentSchemaRejectsPredecessorTables(t *testing.T) {
	for _, table := range []string{"managed_provider_api_key_records", "managed_provider_connection_records", "managed_tenant_records_f014_legacy"} {
		t.Run(table, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "obsolete.db")
			newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
			database := openManagedFixtureDatabase(t, databasePath)
			if err := database.Exec("CREATE TABLE " + table + " (retained_value TEXT)").Error; err != nil {
				t.Fatal(err)
			}
			if err := database.Exec("INSERT INTO " + table + " VALUES ('retained')").Error; err != nil {
				t.Fatal(err)
			}
			_, err := buildRouterWithCatalogs(t, managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath), zap.NewNop().Sugar())
			if err == nil || !strings.Contains(err.Error(), table) {
				t.Fatalf("startup error=%v, want obsolete table context %s", err, table)
			}
			var value string
			if err := database.Raw("SELECT retained_value FROM " + table).Scan(&value).Error; err != nil || value != "retained" {
				t.Fatalf("rejected startup changed predecessor data: value=%q error=%v", value, err)
			}
		})
	}
}

func TestAccountConnectionStartupRejectsUnrecognizedDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "unrecognized.db")
	database := openManagedFixtureDatabase(t, databasePath)
	if err := database.Exec("CREATE TABLE other_application_data (value TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	_, err := buildRouterWithCatalogs(t, managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath), zap.NewNop().Sugar())
	if err == nil || !strings.Contains(err.Error(), "operation=validate_current_schema") {
		t.Fatalf("startup error=%v, want unrecognized database rejection", err)
	}
	tables, err := database.Migrator().GetTables()
	if err != nil || !reflect.DeepEqual(tables, []string{"other_application_data"}) {
		t.Fatalf("rejected startup changed schema: tables=%v error=%v", tables, err)
	}
}

type predecessorQueryFailureDialector struct {
	gorm.Dialector
	table string
}

func (dialector predecessorQueryFailureDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	return database.Callback().Query().Before("gorm:query").Register("predecessor_query_failure", func(query *gorm.DB) {
		if query.Statement.Table == dialector.table {
			query.AddError(errors.New("injected predecessor query failure"))
		}
	})
}

func TestAccountConnectionPredecessorStartupFailurePreservesData(t *testing.T) {
	for _, table := range []string{"managed_schema_migration_records", "managed_account_connection_records"} {
		t.Run(table, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "predecessor.db")
			router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
			owner := managementSessionCookie(t, "retained-predecessor-owner")
			requestManagementAccount(t, router, owner)
			database := openManagedFixtureDatabase(t, databasePath)
			for _, current := range []string{"managed_connection_creation_records", "managed_connection_field_records", "managed_tenant_connection_records", "managed_account_connection_records"} {
				if err := database.Migrator().DropTable(current); err != nil {
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
			var tenantsBefore []map[string]any
			if err := database.Raw("SELECT * FROM managed_tenant_records").Scan(&tenantsBefore).Error; err != nil {
				t.Fatal(err)
			}
			configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath)
			configuration.Management.DatabaseDialector = predecessorQueryFailureDialector{Dialector: sqlite.Open(databasePath), table: table}
			_, err := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
			if err == nil || !strings.Contains(err.Error(), "injected predecessor query failure") {
				t.Fatalf("startup error=%v, want injected failure", err)
			}
			if !database.Migrator().HasTable("managed_provider_connection_records") || database.Migrator().HasTable("managed_account_connection_records") {
				t.Fatal("failed transfer changed predecessor tables")
			}
			var tenantsAfter []map[string]any
			if err := database.Raw("SELECT * FROM managed_tenant_records").Scan(&tenantsAfter).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tenantsBefore, tenantsAfter) {
				t.Fatal("failed transfer changed retained tenants")
			}
		})
	}
}
