package proxy

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestManagedTenantGORMInitializationEdges(t *testing.T) {
	cipher := internalManagedProviderKeyCipher()
	providers := internalManagementProviderRegistry()
	customDialector := sqlite.Open(":memory:")
	resolvedDialector := managementDatabaseDialector(ManagementConfiguration{DatabaseDialector: customDialector})
	if resolvedDialector != customDialector {
		t.Fatalf("custom dialector=%v", resolvedDialector)
	}
	if _, databaseError := newGORMManagedTenantDatabase(ManagementConfiguration{
		DatabasePath: filepath.Join(t.TempDir(), "missing", "management.sqlite"),
	}, cipher, providers); !errors.Is(databaseError, errManagedTenantStoreOpen) {
		t.Fatalf("open error=%v", databaseError)
	}
	if _, databaseError := newGORMManagedTenantDatabase(ManagementConfiguration{
		DatabasePath: "failing-auto-migrate",
		DatabaseDialector: failingManagedAutoMigrateDialector{
			Dialector: sqlite.Open(":memory:"),
		},
	}, cipher, providers); !errors.Is(databaseError, errManagedTenantStoreOpen) || !strings.Contains(databaseError.Error(), errInternalTestDatabase.Error()) {
		t.Fatalf("auto-migrate error=%v", databaseError)
	}

	versionErrorDatabase, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "version-error.db")), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open version-error database: %v", openError)
	}
	if callbackError := versionErrorDatabase.Callback().Create().Before("gorm:create").Register("f014_version_create_error", func(callbackDatabase *gorm.DB) {
		if callbackDatabase.Statement.Table == managedSchemaMigrationTable {
			callbackDatabase.AddError(errInternalTestDatabase)
		}
	}); callbackError != nil {
		t.Fatalf("register version callback: %v", callbackError)
	}
	if initializeError := initializeManagedTenantSchema(versionErrorDatabase, cipher, providers); !errors.Is(initializeError, errInternalTestDatabase) {
		t.Fatalf("version create error=%v", initializeError)
	}

	malformedDatabase, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "malformed.db")), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open malformed database: %v", openError)
	}
	if createError := malformedDatabase.Exec("CREATE TABLE " + managedTenantTable + " (tenant_id TEXT PRIMARY KEY)").Error; createError != nil {
		t.Fatalf("create malformed tenant table: %v", createError)
	}
	if initializeError := initializeManagedTenantSchema(malformedDatabase, cipher, providers); !errors.Is(initializeError, errManagedTenantSchemaMigration) || !strings.Contains(initializeError.Error(), "validate_current_schema") {
		t.Fatalf("malformed schema error=%v", initializeError)
	}

	missingVersionDatabase, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "missing-version.db")), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open missing-version database: %v", openError)
	}
	if migrateError := migrateCurrentManagedSchema(missingVersionDatabase); migrateError != nil {
		t.Fatalf("migrate missing-version schema: %v", migrateError)
	}
	if initializeError := initializeManagedTenantSchema(missingVersionDatabase, cipher, providers); !errors.Is(initializeError, errManagedTenantSchemaMigration) || !strings.Contains(initializeError.Error(), "read_version") {
		t.Fatalf("missing version error=%v", initializeError)
	}

	missingUsageIndexDatabase, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "missing-usage-index.db")), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open missing-index database: %v", openError)
	}
	if migrateError := migrateCurrentManagedSchema(missingUsageIndexDatabase); migrateError != nil {
		t.Fatalf("migrate missing-index schema: %v", migrateError)
	}
	if createError := missingUsageIndexDatabase.Create(&managedSchemaMigrationRecord{
		Version: managedTenantSchemaVersion,
	}).Error; createError != nil {
		t.Fatalf("seed missing-index version: %v", createError)
	}
	if dropError := missingUsageIndexDatabase.Migrator().DropIndex(
		&managedUsageEventRecord{},
		managedUsageFailurePageIndex,
	); dropError != nil {
		t.Fatalf("drop usage index: %v", dropError)
	}
	if initializeError := initializeManagedTenantSchema(missingUsageIndexDatabase, cipher, providers); !errors.Is(initializeError, errManagedTenantSchemaMigration) ||
		!strings.Contains(initializeError.Error(), "validate_current_schema") {
		t.Fatalf("missing usage index error=%v", initializeError)
	}

	schemaTwoMissingUsageIndexDatabase, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "schema-two-missing-usage-index.db")), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open schema-two missing-index database: %v", openError)
	}
	if migrateError := migrateCurrentManagedSchema(schemaTwoMissingUsageIndexDatabase); migrateError != nil {
		t.Fatalf("migrate schema-two missing-index schema: %v", migrateError)
	}
	if createError := schemaTwoMissingUsageIndexDatabase.Create(&managedSchemaMigrationRecord{
		Version: managedUsageOutcomeSchemaVersion,
	}).Error; createError != nil {
		t.Fatalf("seed schema-two missing-index version: %v", createError)
	}
	if dropError := schemaTwoMissingUsageIndexDatabase.Migrator().DropIndex(
		&managedUsageEventRecord{},
		managedUsageFailurePageIndex,
	); dropError != nil {
		t.Fatalf("drop schema-two usage index: %v", dropError)
	}
	if initializeError := initializeManagedTenantSchema(schemaTwoMissingUsageIndexDatabase, cipher, providers); !errors.Is(initializeError, errManagedTenantSchemaMigration) ||
		!strings.Contains(initializeError.Error(), "validate_current_schema") {
		t.Fatalf("schema-two missing usage index error=%v", initializeError)
	}

	if managedTableHasColumn(failingManagedColumnTypesMigrator{Migrator: missingVersionDatabase.Migrator()}, managedTenantTable, "owner_user_id") {
		t.Fatal("column-types error reported a column")
	}
}

func TestManagedTenantGORMFailurePageQueryEdges(t *testing.T) {
	now := time.Date(2026, 7, 25, 16, 30, 0, 0, time.UTC)
	query := managedUsageFailureRecordQuery{
		snapshotAt: now.Add(time.Hour),
		limit:      managedUsageFailureDefaultLimit,
	}
	seedFailure := func(subTest *testing.T, database *gormManagedTenantDatabase, outcomeCode managedUsageOutcomeCode) {
		if createError := database.database.Create(&managedUsageEventRecord{
			TenantID:    "managed-first",
			Endpoint:    usageEndpointV2,
			StatusCode:  http.StatusBadGateway,
			Success:     false,
			OutcomeCode: outcomeCode,
			CreatedAt:   now,
		}).Error; createError != nil {
			subTest.Fatalf("seed failure: %v", createError)
		}
	}

	t.Run("snapshot query", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		seedFailure(subTest, database, managedUsageOutcomeUpstreamError)
		registerManagedGORMError(subTest, database.database, "usage_failure_snapshot_query", "query", managedUsageEventTable, errInternalTestDatabase)
		if _, _, queryError := database.usageFailuresByOwnerAndTenant("owner", "managed-first", query); !errors.Is(queryError, errInternalTestDatabase) {
			subTest.Fatalf("snapshot query error=%v", queryError)
		}
	})

	t.Run("records query", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		seedFailure(subTest, database, managedUsageOutcomeUpstreamError)
		queryCount := 0
		if callbackError := database.database.Callback().Query().Before("gorm:query").Register(
			"usage_failure_records_query",
			func(callbackDatabase *gorm.DB) {
				if callbackDatabase.Statement.Table == managedUsageEventTable {
					queryCount++
					if queryCount == 2 {
						callbackDatabase.AddError(errInternalTestDatabase)
					}
				}
			},
		); callbackError != nil {
			subTest.Fatalf("register records query callback: %v", callbackError)
		}
		if _, _, queryError := database.usageFailuresByOwnerAndTenant("owner", "managed-first", query); !errors.Is(queryError, errInternalTestDatabase) {
			subTest.Fatalf("records query error=%v", queryError)
		}
	})

	t.Run("invalid persisted outcome", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		seedFailure(subTest, database, managedUsageOutcomeCode("provider_error"))
		if _, _, queryError := database.usageFailuresByOwnerAndTenant("owner", "managed-first", query); !errors.Is(queryError, errManagedTenantStorePersist) {
			subTest.Fatalf("invalid outcome query error=%v", queryError)
		}
	})
}

func TestManagedTenantGORMLowLevelMutationEdges(t *testing.T) {
	now := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)

	t.Run("queries and row counts", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		if _, queryError := database.tenantByTenantID(context.Background(), "managed-first"); queryError != nil {
			subTest.Fatalf("tenant by id: %v", queryError)
		}
		if _, queryError := database.tenantByTenantID(context.Background(), "missing"); !errors.Is(queryError, gorm.ErrRecordNotFound) {
			subTest.Fatalf("missing tenant by id error=%v", queryError)
		}
		if providerRecords, providerError := database.providerKeys(); providerError != nil || len(providerRecords) != 0 {
			subTest.Fatalf("provider records=%+v error=%v", providerRecords, providerError)
		}
		if saveError := database.saveUser(managedUserRecord{UserID: "missing", UpdatedAt: now}); !errors.Is(saveError, gorm.ErrRecordNotFound) {
			subTest.Fatalf("missing user save error=%v", saveError)
		}
		missingTenant := fakeTenantRecord("owner", "missing", "Missing", now)
		if saveError := database.saveTenant(missingTenant); !errors.Is(saveError, gorm.ErrRecordNotFound) {
			subTest.Fatalf("missing tenant save error=%v", saveError)
		}
	})

	t.Run("save errors", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		registerManagedGORMError(subTest, database.database, "update_user", "update", managedUserTable, errInternalTestDatabase)
		if saveError := database.saveUser(managedUserRecord{UserID: "owner", UpdatedAt: now}); !errors.Is(saveError, errInternalTestDatabase) {
			subTest.Fatalf("user update error=%v", saveError)
		}
	})
	t.Run("tenant save error", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		registerManagedGORMError(subTest, database.database, "update_tenant", "update", managedTenantTable, errInternalTestDatabase)
		record, queryError := database.tenantByOwnerAndID("owner", "managed-first")
		if queryError != nil {
			subTest.Fatalf("load tenant: %v", queryError)
		}
		if saveError := database.saveTenant(record); !errors.Is(saveError, errInternalTestDatabase) {
			subTest.Fatalf("tenant update error=%v", saveError)
		}
	})
	t.Run("create user and tenant errors", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		existingTenant := fakeTenantRecord("new-owner", "managed-first", "Existing", now)
		if createError := database.createUserAndTenant(managedUserRecord{UserID: "owner"}, existingTenant); createError == nil {
			subTest.Fatal("duplicate user creation succeeded")
		}
		if createError := database.createUserAndTenant(managedUserRecord{UserID: "new-owner"}, existingTenant); createError == nil {
			subTest.Fatal("duplicate tenant creation succeeded")
		}
		var count int64
		if countError := database.database.Model(&managedUserRecord{}).Where(&managedUserRecord{UserID: "new-owner"}).Count(&count).Error; countError != nil || count != 0 {
			subTest.Fatalf("rolled-back user count=%d error=%v", count, countError)
		}
	})

	t.Run("delete count error", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		queryCount := 0
		if callbackError := database.database.Callback().Query().Before("gorm:query").Register("f014_delete_count_error", func(callbackDatabase *gorm.DB) {
			if callbackDatabase.Statement.Table == managedTenantTable {
				queryCount++
				if queryCount == 2 {
					callbackDatabase.AddError(errInternalTestDatabase)
				}
			}
		}); callbackError != nil {
			subTest.Fatalf("register count callback: %v", callbackError)
		}
		if deleteError := database.deleteTenant("owner", "managed-first"); !errors.Is(deleteError, errInternalTestDatabase) {
			subTest.Fatalf("count error=%v", deleteError)
		}
	})
	for _, tableName := range []string{managedProviderKeyTable, managedUsageEventTable, managedTenantTable} {
		t.Run("delete error "+tableName, func(subTest *testing.T) {
			database := newCanonicalGORMFixture(subTest, now)
			registerManagedGORMError(subTest, database.database, "delete_"+tableName, "delete", tableName, errInternalTestDatabase)
			if deleteError := database.deleteTenant("owner", "managed-first"); !errors.Is(deleteError, errInternalTestDatabase) {
				subTest.Fatalf("table=%s delete error=%v", tableName, deleteError)
			}
		})
	}
	t.Run("delete affected rows", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		if callbackError := database.database.Callback().Delete().After("gorm:delete").Register("f014_delete_zero_rows", func(callbackDatabase *gorm.DB) {
			if callbackDatabase.Statement.Table == managedTenantTable {
				callbackDatabase.RowsAffected = 0
			}
		}); callbackError != nil {
			subTest.Fatalf("register rows callback: %v", callbackError)
		}
		if deleteError := database.deleteTenant("owner", "managed-first"); !errors.Is(deleteError, gorm.ErrRecordNotFound) {
			subTest.Fatalf("zero-row delete error=%v", deleteError)
		}
	})

	t.Run("save provider ownership query", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		if saveError := database.saveProviderKey("other", managedProviderAPIKeyRecord{
			TenantID: "managed-first", ProviderID: ProviderNameOpenAI, EncryptedAPIKey: "cipher",
		}, defaultManagedRoutingDefaults(), now); !errors.Is(saveError, gorm.ErrRecordNotFound) {
			subTest.Fatalf("provider ownership error=%v", saveError)
		}
	})
	t.Run("save provider record", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		registerManagedGORMError(subTest, database.database, "create_provider", "create", managedProviderKeyTable, errInternalTestDatabase)
		if saveError := database.saveProviderKey("owner", managedProviderAPIKeyRecord{
			TenantID: "managed-first", ProviderID: ProviderNameOpenAI, EncryptedAPIKey: "cipher",
		}, defaultManagedRoutingDefaults(), now); !errors.Is(saveError, errInternalTestDatabase) {
			subTest.Fatalf("provider record error=%v", saveError)
		}
	})
	t.Run("delete provider ownership query", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		if deleteError := database.deleteProviderKey("other", "managed-first", ProviderNameOpenAI, defaultManagedRoutingDefaults(), now); !errors.Is(deleteError, gorm.ErrRecordNotFound) {
			subTest.Fatalf("provider ownership delete error=%v", deleteError)
		}
	})
	t.Run("delete provider record", func(subTest *testing.T) {
		database := newCanonicalGORMFixture(subTest, now)
		registerManagedGORMError(subTest, database.database, "delete_provider", "delete", managedProviderKeyTable, errInternalTestDatabase)
		if deleteError := database.deleteProviderKey("owner", "managed-first", ProviderNameOpenAI, defaultManagedRoutingDefaults(), now); !errors.Is(deleteError, errInternalTestDatabase) {
			subTest.Fatalf("provider record delete error=%v", deleteError)
		}
	})
}

func newCanonicalGORMFixture(t *testing.T, now time.Time) *gormManagedTenantDatabase {
	t.Helper()
	database, databaseError := newGORMManagedTenantDatabase(ManagementConfiguration{
		DatabasePath: filepath.Join(t.TempDir(), "canonical.db"),
	}, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
	if databaseError != nil {
		t.Fatalf("open canonical database: %v", databaseError)
	}
	user := managedUserRecord{
		UserID: "owner", UserEmail: "owner@example.com", CreatedAt: now, UpdatedAt: now,
	}
	if createError := database.database.Create(&user).Error; createError != nil {
		t.Fatalf("create owner: %v", createError)
	}
	for _, record := range []managedTenantRecord{
		fakeTenantRecord("owner", "managed-first", "First", now),
		fakeTenantRecord("owner", "managed-second", "Second", now.Add(time.Second)),
	} {
		if createError := database.database.Create(&record).Error; createError != nil {
			t.Fatalf("create tenant %s: %v", record.TenantID, createError)
		}
	}
	return database
}

func registerManagedGORMError(t *testing.T, database *gorm.DB, callbackName string, operation string, tableName string, callbackError error) {
	t.Helper()
	callback := func(callbackDatabase *gorm.DB) {
		if callbackDatabase.Statement.Table == tableName {
			callbackDatabase.AddError(callbackError)
		}
	}
	var registrationError error
	switch operation {
	case "create":
		registrationError = database.Callback().Create().Before("gorm:create").Register(callbackName, callback)
	case "update":
		registrationError = database.Callback().Update().Before("gorm:update").Register(callbackName, callback)
	case "delete":
		registrationError = database.Callback().Delete().Before("gorm:delete").Register(callbackName, callback)
	case "query":
		registrationError = database.Callback().Query().Before("gorm:query").Register(callbackName, callback)
	default:
		t.Fatalf("unsupported callback operation %s", operation)
	}
	if registrationError != nil {
		t.Fatalf("register %s callback: %v", callbackName, registrationError)
	}
}

type failingManagedAutoMigrateDialector struct {
	gorm.Dialector
}

func (dialector failingManagedAutoMigrateDialector) Migrator(database *gorm.DB) gorm.Migrator {
	return failingManagedAutoMigrateMigrator{Migrator: dialector.Dialector.Migrator(database)}
}

type failingManagedAutoMigrateMigrator struct {
	gorm.Migrator
}

func (migrator failingManagedAutoMigrateMigrator) AutoMigrate(...interface{}) error {
	return errInternalTestDatabase
}

type failingManagedColumnTypesMigrator struct {
	gorm.Migrator
}

func (migrator failingManagedColumnTypesMigrator) ColumnTypes(interface{}) ([]gorm.ColumnType, error) {
	return nil, errInternalTestDatabase
}
