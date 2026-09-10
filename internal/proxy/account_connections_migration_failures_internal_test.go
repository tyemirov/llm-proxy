package proxy

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type connectionMigrationFailureDialector struct {
	gorm.Dialector
	stage *string
}

func (dialector connectionMigrationFailureDialector) Migrator(database *gorm.DB) gorm.Migrator {
	return connectionMigrationFailureMigrator{dialector.Dialector.Migrator(database), dialector.stage}
}

type connectionMigrationFailureMigrator struct {
	gorm.Migrator
	stage *string
}

func (migrator connectionMigrationFailureMigrator) AutoMigrate(values ...interface{}) error {
	if *migrator.stage == "schema" {
		return errInternalTestDatabase
	}
	return migrator.Migrator.AutoMigrate(values...)
}
func (migrator connectionMigrationFailureMigrator) DropTable(values ...interface{}) error {
	if *migrator.stage == "drop" {
		return errInternalTestDatabase
	}
	return migrator.Migrator.DropTable(values...)
}

func TestAccountConnectionMigrationFailuresPreservePredecessor(t *testing.T) {
	for _, stage := range []string{"schema", "query", "connection", "assignment", "drop", "decrypt", "identifier", "encryption"} {
		t.Run(stage, func(t *testing.T) {
			failureStage := ""
			database, err := gorm.Open(connectionMigrationFailureDialector{sqlite.Open(filepath.Join(t.TempDir(), "migration.db")), &failureStage}, &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			cipher := internalManagedProviderKeyCipher()
			providers := internalManagementProviderRegistry()
			if err := initializeManagedTenantSchemaRecords(database, cipher, providers); err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
			if err := database.Create(&managedUserRecord{UserID: "owner", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
				t.Fatal(err)
			}
			tenant := fakeTenantRecord("owner", "home", "Home", now)
			if err := database.Create(&tenant).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.Create(&managedProviderProfileRecord{TenantID: "home", ProviderID: ProviderNameOpenAI, TextModel: ModelNameGPT41, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
				t.Fatal(err)
			}
			value, err := cipher.encryptConnection(strings.NewReader(strings.Repeat("x", 64)), "home", ProviderNameOpenAI, "api_key", "original")
			if err != nil {
				t.Fatal(err)
			}
			if stage == "decrypt" {
				value = "invalid"
			}
			if err := database.Create(&managedProviderConnectionRecord{TenantID: "home", ProviderID: ProviderNameOpenAI, FieldID: "api_key", Value: value, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
				t.Fatal(err)
			}
			switch stage {
			case "query":
				registerManagedGORMError(t, database, "migration_failure", "query", managedTenantTable, errInternalTestDatabase)
			case "connection":
				registerManagedGORMError(t, database, "migration_failure", "create", "managed_account_connection_records", errInternalTestDatabase)
			case "assignment":
				registerManagedGORMError(t, database, "migration_failure", "create", "managed_tenant_connection_records", errInternalTestDatabase)
			}
			failureStage = stage
			var entropy io.Reader = strings.NewReader(strings.Repeat("x", 128))
			if stage == "identifier" {
				entropy = strings.NewReader("")
			}
			if stage == "encryption" {
				entropy = strings.NewReader(strings.Repeat("x", 16))
			}
			err = database.Transaction(func(tx *gorm.DB) error {
				if stage == "identifier" || stage == "encryption" || stage == "decrypt" || stage == "query" {
					if err := tx.AutoMigrate(&managedAccountConnectionRecord{}, &managedConnectionFieldRecord{}, &managedTenantConnectionRecord{}, &managedConnectionCreationRecord{}); err != nil {
						return err
					}
					return migrateAccountConnectionRecords(tx, cipher, providers, entropy)
				}
				return initializeManagedTenantSchema(tx.Session(&gorm.Session{DisableNestedTransaction: true}), cipher, providers)
			})
			if err == nil {
				t.Fatal("migration accepted a failed stage")
			}
			expectedError := map[string]string{"schema": "create account connection schema", "query": "read predecessor connections", "connection": "migrate tenant connection", "assignment": "migrate tenant assignment", "drop": "remove predecessor connection fields", "decrypt": errManagedProviderKeyDecryption.Error(), "identifier": "connection identifier", "encryption": errManagedProviderKeyEncryption.Error()}[stage]
			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("unexpected migration failure: %v", err)
			}
			failureStage = ""
			if !database.Migrator().HasTable(managedProviderConnectionTable) {
				t.Fatal("failed migration removed predecessor")
			}
			var fields []managedProviderConnectionRecord
			if err := database.Find(&fields).Error; err != nil {
				t.Fatal(err)
			}
			if len(fields) != 1 || fields[0].Value != value {
				t.Fatal("failed migration changed predecessor credentials")
			}
		})
	}
}

func TestAccountConnectionStartupRejectsInvalidSchemaAndRelations(t *testing.T) {
	for _, scenario := range []string{"missing-table", "predecessor-table", "usage-schema", "version-read", "connections-read", "tenants-read", "missing-profile", "invalid-profile"} {
		t.Run(scenario, func(t *testing.T) {
			service, database, server := newAccountConnectionHTTPFixture(t)
			created := accountConnectionHTTPExchange(t, server, http.MethodPost, managementConnectionsPath, `{"name":"Startup","provider":"openai","fields":{"api_key":"sk-original"}}`, http.StatusCreated)
			accountConnectionHTTPExchange(t, server, http.MethodPut, "/tenants/managed-first/connections/openai", fmt.Sprintf(`{"connection_id":%q}`, created["id"]), http.StatusOK)
			var changeError error
			switch scenario {
			case "missing-table":
				changeError = database.database.Migrator().DropTable(&managedConnectionCreationRecord{})
			case "predecessor-table":
				changeError = database.database.AutoMigrate(&managedProviderConnectionRecord{})
			case "usage-schema":
				changeError = database.database.Migrator().DropTable(&managedUsageEventRecord{})
			case "version-read":
				registerManagedGORMError(t, database.database, "startup_failure", "query", managedSchemaMigrationTable, errInternalTestDatabase)
			case "connections-read":
				registerManagedGORMError(t, database.database, "startup_failure", "query", "managed_account_connection_records", errInternalTestDatabase)
			case "tenants-read":
				registerManagedGORMError(t, database.database, "startup_failure", "query", managedTenantTable, errInternalTestDatabase)
			case "missing-profile":
				changeError = database.database.Where("tenant_id = ?", "managed-first").Delete(&managedProviderProfileRecord{}).Error
			case "invalid-profile":
				changeError = database.database.Model(&managedProviderProfileRecord{}).Where("tenant_id = ?", "managed-first").Update("text_model", "invalid-model").Error
			}
			if changeError != nil {
				t.Fatal(changeError)
			}
			if err := initializeManagedTenantSchema(database.database, service.store.providerKeyCipher, service.providers); err == nil {
				t.Fatal("startup accepted invalid schema or assignment")
			}
		})
	}
}

func TestAccountConnectionMigrationRejectsInvalidPredecessorState(t *testing.T) {
	for _, scenario := range []string{"default-route", "unexpected-version"} {
		database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "predecessor.db")), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		cipher := internalManagedProviderKeyCipher()
		providers := internalManagementProviderRegistry()
		if err := initializeManagedTenantSchemaRecords(database, cipher, providers); err != nil {
			t.Fatal(err)
		}
		if scenario == "unexpected-version" {
			if err := database.Create(&managedSchemaMigrationRecord{Version: 999, AppliedAt: time.Now().UTC()}).Error; err != nil {
				t.Fatal(err)
			}
			if err := initializeManagedTenantSchemaRecords(database, cipher, providers); err == nil {
				t.Fatal("unknown predecessor version accepted")
			}
			continue
		}
		now := time.Now().UTC()
		if err := database.Create(&managedUserRecord{UserID: "owner", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
		tenant := fakeTenantRecord("owner", "home", "Home", now)
		tenant.DefaultProvider = ProviderNameOpenAI
		tenant.DefaultModel = ModelNameGPT41
		if err := database.Create(&tenant).Error; err != nil {
			t.Fatal(err)
		}
		if err := initializeManagedTenantSchema(database, cipher, providers); err == nil {
			t.Fatal("migration accepted a default without provider credentials")
		}
	}
}
