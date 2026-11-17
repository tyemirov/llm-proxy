package proxy

import (
	"errors"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
)

func TestDashScopeResponsesDiagnosticMigrationRollback(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migration.sqlite")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := database.Exec("CREATE TABLE managed_usage_event_records (id INTEGER PRIMARY KEY, request_tokens INTEGER,response_tokens INTEGER,total_tokens INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("INSERT INTO managed_usage_event_records VALUES (1,10,3,13)").Error; err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("startup interrupted")
	err = database.Transaction(func(tx *gorm.DB) error {
		if err := migrateManagedProviderDiagnostics(tx); err != nil {
			return err
		}
		return interrupted
	})
	if !errors.Is(err, interrupted) {
		t.Fatal(err)
	}
	for _, field := range []string{"ReasoningTokens", "IncompleteReason"} {
		if database.Migrator().HasColumn(&managedUsageEventRecord{}, field) {
			t.Fatal("failed transaction retained partial diagnostic migration")
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := database.Transaction(migrateManagedProviderDiagnostics); err != nil {
			t.Fatal(err)
		}
	}
	var record managedUsageEventRecord
	if err := database.First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if record.ReasoningTokens != nil || record.IncompleteReason != nil || record.RequestTokens != 10 || record.ResponseTokens != 3 || record.TotalTokens != 13 {
		t.Fatalf("historical measurements changed: %+v", record)
	}
}

func TestDashScopeResponsesDiagnosticStartupMigrationFailure(t *testing.T) {
	cipher, providers := internalManagedProviderKeyCipher(), internalManagementProviderRegistry()
	database, err := newGORMManagedTenantDatabase(ManagementConfiguration{DatabasePath: filepath.Join(t.TempDir(), "startup.sqlite")}, cipher, providers)
	if err != nil {
		t.Fatal(err)
	}
	defer database.close()
	for _, column := range []string{"reasoning_tokens", "incomplete_reason"} {
		if err := database.database.Exec("ALTER TABLE managed_usage_event_records DROP COLUMN " + column).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := database.database.Exec("PRAGMA query_only = ON").Error; err != nil {
		t.Fatal(err)
	}
	err = initializeManagedTenantSchema(database.database, cipher, providers)
	if !errors.Is(err, errManagedTenantSchemaMigration) {
		t.Fatalf("startup accepted failed diagnostic migration: %v", err)
	}
	for _, field := range []string{"ReasoningTokens", "IncompleteReason"} {
		if database.database.Migrator().HasColumn(&managedUsageEventRecord{}, field) {
			t.Fatal("failed startup retained diagnostic column")
		}
	}
	if err := database.database.Exec("PRAGMA query_only = OFF").Error; err != nil {
		t.Fatal(err)
	}
	if err := initializeManagedTenantSchema(database.database, cipher, providers); err != nil {
		t.Fatal(err)
	}
}
