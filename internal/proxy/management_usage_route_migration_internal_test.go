package proxy

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func managedUsageRecordWithoutRoute(record managedUsageEventRecord) managedUsageEventRecord {
	record.ProviderID = ""
	record.ModelID = ""
	return record
}

func newManagedResolvedUsageRouteMigrationFixture(t *testing.T) (*gorm.DB, managedProviderKeyCipher, *providerRegistry, []managedUsageEventRecord) {
	t.Helper()
	database, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "resolved-usage-route.db")), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open resolved usage route fixture: %v", openError)
	}
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create resolved usage route fixture: %v", migrationError)
	}
	if dropError := database.Migrator().DropTable(&managedProviderAPIKeyRecord{}); dropError != nil {
		t.Fatalf("drop predecessor provider table: %v", dropError)
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	user := managedUserRecord{UserID: "resolved-route-owner", CreatedAt: now, UpdatedAt: now}
	if createError := database.Create(&user).Error; createError != nil {
		t.Fatalf("seed resolved usage route user: %v", createError)
	}
	tenant := fakeTenantRecord(user.UserID, "resolved-route-tenant", "Default", now)
	if createError := database.Create(&tenant).Error; createError != nil {
		t.Fatalf("seed resolved usage route tenant: %v", createError)
	}
	records := []managedUsageEventRecord{
		{ID: 1, TenantID: tenant.TenantID, Endpoint: usageEndpointText, ProviderID: ProviderNameOpenAI, ModelID: ModelNameGPT41, StatusCode: http.StatusOK, Success: true, OutcomeCode: managedUsageOutcomeSuccess, LatencyMilliseconds: 11, RequestTokens: 2, ResponseTokens: 3, TotalTokens: 5, CreatedAt: now},
		{ID: 2, TenantID: tenant.TenantID, Endpoint: usageEndpointV2, ProviderID: ProviderNameDeepSeek, ModelID: ModelNameDeepSeekV4Flash, StatusCode: http.StatusBadGateway, Success: false, OutcomeCode: managedUsageOutcomeUpstreamError, LatencyMilliseconds: 17, CreatedAt: now.Add(time.Minute)},
		{ID: 3, TenantID: tenant.TenantID, Endpoint: usageEndpointDictation, ProviderID: ProviderNameOpenAI, ModelID: DefaultDictationModel, StatusCode: http.StatusOK, Success: true, OutcomeCode: managedUsageOutcomeSuccess, LatencyMilliseconds: 23, CreatedAt: now.Add(2 * time.Minute)},
		{ID: 4, TenantID: tenant.TenantID, Endpoint: usageEndpointText, StatusCode: http.StatusBadRequest, Success: false, OutcomeCode: managedUsageOutcomeInvalidRequest, LatencyMilliseconds: 29, CreatedAt: now.Add(3 * time.Minute)},
		{ID: 5, TenantID: tenant.TenantID, Endpoint: usageEndpointText, ProviderID: "__credential_validation__", StatusCode: http.StatusBadRequest, Success: false, OutcomeCode: managedUsageOutcomeInvalidRequest, LatencyMilliseconds: 31, CreatedAt: now.Add(4 * time.Minute)},
		{ID: 6, TenantID: tenant.TenantID, Endpoint: usageEndpointText, ModelID: ModelNameGPT41, StatusCode: http.StatusBadRequest, Success: false, OutcomeCode: managedUsageOutcomeInvalidRequest, LatencyMilliseconds: 37, CreatedAt: now.Add(5 * time.Minute)},
		{ID: 7, TenantID: tenant.TenantID, Endpoint: usageEndpointText, ProviderID: strings.ToUpper(ProviderNameOpenAI), ModelID: ModelNameGPT41, StatusCode: http.StatusBadRequest, Success: false, OutcomeCode: managedUsageOutcomeInvalidRequest, LatencyMilliseconds: 41, CreatedAt: now.Add(6 * time.Minute)},
		{ID: 8, TenantID: tenant.TenantID, Endpoint: usageEndpointText, ProviderID: ProviderNameOpenAI, ModelID: ModelNameDeepSeekV4Flash, StatusCode: http.StatusBadRequest, Success: false, OutcomeCode: managedUsageOutcomeInvalidRequest, LatencyMilliseconds: 43, CreatedAt: now.Add(7 * time.Minute)},
		{ID: 9, TenantID: tenant.TenantID, Endpoint: "unknown", ProviderID: ProviderNameOpenAI, ModelID: ModelNameGPT41, StatusCode: http.StatusBadRequest, Success: false, OutcomeCode: managedUsageOutcomeInvalidRequest, LatencyMilliseconds: 47, CreatedAt: now.Add(8 * time.Minute)},
		{ID: 10, TenantID: tenant.TenantID, Endpoint: usageEndpointDictation, ProviderID: ProviderNameOpenAI, ModelID: ModelNameGPT41, StatusCode: http.StatusBadRequest, Success: false, OutcomeCode: managedUsageOutcomeInvalidRequest, LatencyMilliseconds: 53, CreatedAt: now.Add(9 * time.Minute)},
	}
	if createError := database.Create(&records).Error; createError != nil {
		t.Fatalf("seed resolved usage route records: %v", createError)
	}
	if createError := database.Create(&managedSchemaMigrationRecord{Version: managedGeminiRouteRetirementVersion, AppliedAt: now}).Error; createError != nil {
		t.Fatalf("seed resolved usage route schema version: %v", createError)
	}
	return database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry(), records
}

func TestManagedResolvedUsageRouteMigrationClearsOnlyInvalidDimensions(t *testing.T) {
	database, providerKeyCipher, providers, records := newManagedResolvedUsageRouteMigrationFixture(t)
	if migrationError := initializeManagedTenantSchema(database, providerKeyCipher, providers); migrationError != nil {
		t.Fatalf("migrate resolved usage routes: %v", migrationError)
	}
	var migrated []managedUsageEventRecord
	if queryError := database.Order(managedUsageIDColumn).Find(&migrated).Error; queryError != nil {
		t.Fatalf("load migrated resolved usage routes: %v", queryError)
	}
	expected := append([]managedUsageEventRecord(nil), records...)
	for index := 4; index < len(expected); index++ {
		expected[index] = managedUsageRecordWithoutRoute(expected[index])
	}
	if len(migrated) != len(expected) {
		t.Fatalf("migrated record count=%d want=%d", len(migrated), len(expected))
	}
	for index := range expected {
		if migrated[index] != expected[index] {
			t.Fatalf("migrated record index=%d got=%+v want=%+v", index, migrated[index], expected[index])
		}
	}
	var latest managedSchemaMigrationRecord
	if queryError := database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedResolvedUsageRouteSchemaVersion {
		t.Fatalf("latest resolved usage route version=%+v error=%v", latest, queryError)
	}
	if reopenError := initializeManagedTenantSchema(database, providerKeyCipher, providers); reopenError != nil {
		t.Fatalf("reopen resolved usage route schema: %v", reopenError)
	}
}

func TestManagedResolvedUsageRouteMigrationRollsBackStageFailures(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		operation string
		table     string
		want      string
	}{
		{name: "read", operation: "query", table: managedUsageEventTable, want: "operation=read"},
		{name: "clear", operation: "update", table: managedUsageEventTable, want: "operation=clear_route"},
		{name: "record version", operation: "create", table: managedSchemaMigrationTable, want: "operation=record_version"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			database, _, providers, records := newManagedResolvedUsageRouteMigrationFixture(t)
			registerManagedGORMError(t, database, "resolved_usage_route_"+strings.ReplaceAll(testCase.name, " ", "_"), testCase.operation, testCase.table, errInternalTestDatabase)
			migrationError := migrateManagedResolvedUsageRoutes(database, providers)
			if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), testCase.want) {
				t.Fatalf("migration error=%v want=%s", migrationError, testCase.want)
			}
			var invalid managedUsageEventRecord
			if queryError := database.Raw("SELECT * FROM "+managedUsageEventTable+" WHERE id = ?", records[4].ID).Scan(&invalid).Error; queryError != nil || invalid != records[4] {
				t.Fatalf("rolled back invalid route=%+v error=%v", invalid, queryError)
			}
			var versionCount int64
			if countError := database.Raw("SELECT COUNT(*) FROM "+managedSchemaMigrationTable+" WHERE version = ?", managedResolvedUsageRouteSchemaVersion).Scan(&versionCount).Error; countError != nil || versionCount != 0 {
				t.Fatalf("resolved route version count=%d error=%v", versionCount, countError)
			}
		})
	}
}

func TestManagedResolvedUsageRouteCurrentSchemaRejectsInvalidDimensions(t *testing.T) {
	database, providerKeyCipher, providers, _ := newManagedResolvedUsageRouteMigrationFixture(t)
	if migrationError := migrateManagedResolvedUsageRoutes(database, providers); migrationError != nil {
		t.Fatalf("migrate resolved usage routes: %v", migrationError)
	}
	if updateError := database.Model(&managedUsageEventRecord{}).Where("id = ?", 4).Updates(map[string]any{
		managedUsageProviderIDColumn: "__credential_validation__",
		managedUsageModelIDColumn:    "",
	}).Error; updateError != nil {
		t.Fatalf("corrupt resolved usage route: %v", updateError)
	}
	validationError := initializeManagedTenantSchema(database, providerKeyCipher, providers)
	if !errors.Is(validationError, errManagedTenantSchemaMigration) || !strings.Contains(validationError.Error(), "operation=validate_routes") {
		t.Fatalf("current resolved route validation error=%v", validationError)
	}
}

func TestManagedResolvedUsageRouteSchemaElevenRejectsInvalidStoreShapes(t *testing.T) {
	t.Run("missing connection table", func(t *testing.T) {
		database, providerKeyCipher, providers, _ := newManagedResolvedUsageRouteMigrationFixture(t)
		if dropError := database.Migrator().DropTable(&managedProviderConnectionRecord{}); dropError != nil {
			t.Fatalf("drop provider connection table: %v", dropError)
		}
		migrationError := initializeManagedTenantSchema(database, providerKeyCipher, providers)
		if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), "operation=validate_current_schema") {
			t.Fatalf("missing connection table error=%v", migrationError)
		}
	})

	t.Run("invalid connection defaults", func(t *testing.T) {
		database, providerKeyCipher, providers, _ := newManagedResolvedUsageRouteMigrationFixture(t)
		registerManagedGORMError(t, database, "resolved_usage_route_connection_validation", "query", managedTenantTable, errInternalTestDatabase)
		migrationError := initializeManagedTenantSchema(database, providerKeyCipher, providers)
		if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), "operation=read") {
			t.Fatalf("connection validation error=%v", migrationError)
		}
	})
}

func TestManagedResolvedUsageRouteValidationPropagatesReadFailure(t *testing.T) {
	database, _, providers, _ := newManagedResolvedUsageRouteMigrationFixture(t)
	registerManagedGORMError(t, database, "resolved_usage_route_validation_read", "query", managedUsageEventTable, errInternalTestDatabase)
	validationError := validateManagedResolvedUsageRoutes(database, providers)
	if !errors.Is(validationError, errManagedTenantSchemaMigration) || !strings.Contains(validationError.Error(), "operation=validate_routes") {
		t.Fatalf("route validation read error=%v", validationError)
	}
}

func TestManagedResolvedUsageRouteValidationReadsDistinctDimensions(t *testing.T) {
	database, _, providers, records := newManagedResolvedUsageRouteMigrationFixture(t)
	if migrationError := migrateManagedResolvedUsageRoutes(database, providers); migrationError != nil {
		t.Fatalf("migrate resolved usage routes: %v", migrationError)
	}
	repeatedRecords := make([]managedUsageEventRecord, 512)
	for index := range repeatedRecords {
		repeatedRecords[index] = records[0]
		repeatedRecords[index].ID = uint(len(records) + index + 1)
		repeatedRecords[index].CreatedAt = records[0].CreatedAt.Add(time.Duration(index+1) * time.Second)
	}
	if createError := database.Create(&repeatedRecords).Error; createError != nil {
		t.Fatalf("seed repeated resolved routes: %v", createError)
	}

	distinctRouteCount := int64(-1)
	callbackName := "resolved_usage_route_distinct_count"
	callbackError := database.Callback().Query().After("gorm:after_query").Register(callbackName, func(callbackDatabase *gorm.DB) {
		if callbackDatabase.Statement.Table == managedUsageEventTable && callbackDatabase.Statement.Distinct {
			distinctRouteCount = callbackDatabase.RowsAffected
		}
	})
	if callbackError != nil {
		t.Fatalf("register distinct route callback: %v", callbackError)
	}
	t.Cleanup(func() { _ = database.Callback().Query().Remove(callbackName) })

	if validationError := validateManagedResolvedUsageRoutes(database, providers); validationError != nil {
		t.Fatalf("validate distinct resolved routes: %v", validationError)
	}
	if distinctRouteCount != 6 {
		t.Fatalf("distinct route count=%d want=6", distinctRouteCount)
	}
}
