package proxy

import (
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type managedUsageEventSchemaOneFixtureRecord struct {
	ID                  uint   `gorm:"primaryKey"`
	TenantID            string `gorm:"not null;index:idx_managed_usage_tenant_created,priority:1"`
	Endpoint            string
	ProviderID          string
	ModelID             string
	StatusCode          int
	Success             bool
	LatencyMilliseconds int64
	RequestTokens       int
	ResponseTokens      int
	TotalTokens         int
	CreatedAt           time.Time `gorm:"index:idx_managed_usage_tenant_created,priority:2;index:idx_managed_usage_created_at"`
}

func (managedUsageEventSchemaOneFixtureRecord) TableName() string {
	return managedUsageEventTable
}

func TestManagedUsageOutcomeSQLiteMigrationBackfillsCanonicalCodesAndRejectsUnsupportedRows(t *testing.T) {
	t.Run("backfill", func(subTest *testing.T) {
		databasePath := filepath.Join(subTest.TempDir(), "usage-outcomes.db")
		database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
		if openError != nil {
			subTest.Fatalf("open SQLite fixture: %v", openError)
		}
		seedManagedUsageSchemaOne(subTest, database)
		assertManagedUsageOutcomeMigration(subTest, database)
		if reopenError := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry()); reopenError != nil {
			subTest.Fatalf("reopen migrated schema: %v", reopenError)
		}
	})

	t.Run("unsupported status rollback", func(subTest *testing.T) {
		database, openError := gorm.Open(sqlite.Open(filepath.Join(subTest.TempDir(), "unsupported.db")), &gorm.Config{})
		if openError != nil {
			subTest.Fatalf("open SQLite fixture: %v", openError)
		}
		seedManagedUsageSchemaOne(subTest, database)
		if updateError := database.Table(managedUsageEventTable).
			Where("id = ?", 2).
			Update("status_code", http.StatusTeapot).
			Error; updateError != nil {
			subTest.Fatalf("seed unsupported status: %v", updateError)
		}
		migrationError := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
		if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), "status_code=418") {
			subTest.Fatalf("migration error=%v", migrationError)
		}
		if managedTableHasColumn(database.Migrator(), managedUsageEventTable, "outcome_code") {
			subTest.Fatal("failed preflight added outcome_code")
		}
		var latest managedSchemaMigrationRecord
		if queryError := database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedTenantOwnershipSchemaVersion {
			subTest.Fatalf("latest version=%+v error=%v", latest, queryError)
		}
	})
}

func TestManagedUsageOutcomeSQLiteMigrationRollsBackStageFailures(t *testing.T) {
	type migrationFailureCase struct {
		name      string
		stage     string
		want      string
		configure func(*testing.T, *gorm.DB)
	}
	testCases := []migrationFailureCase{
		{
			name: "preflight query",
			want: "operation=preflight",
			configure: func(subTest *testing.T, database *gorm.DB) {
				registerManagedGORMError(subTest, database, "usage_outcome_preflight_query", "query", managedUsageEventTable, errInternalTestDatabase)
			},
		},
		{name: "add column", stage: "add_column", want: "operation=add_column"},
		{
			name: "backfill",
			want: "operation=backfill",
			configure: func(subTest *testing.T, database *gorm.DB) {
				registerManagedGORMError(subTest, database, "usage_outcome_backfill", "update", managedUsageEventTable, errInternalTestDatabase)
			},
		},
		{name: "require column", stage: "alter_column", want: "operation=require_column"},
		{
			name: "create index",
			want: "operation=create_index",
			configure: func(subTest *testing.T, database *gorm.DB) {
				if createError := database.Exec("CREATE TABLE managed_usage_index_collision (id INTEGER)").Error; createError != nil {
					subTest.Fatalf("create index-collision table: %v", createError)
				}
				if createError := database.Exec(
					"CREATE INDEX " + managedUsageFailurePageIndex + " ON managed_usage_index_collision (id)",
				).Error; createError != nil {
					subTest.Fatalf("create colliding index: %v", createError)
				}
			},
		},
		{
			name: "verify query",
			want: "operation=verify",
			configure: func(subTest *testing.T, database *gorm.DB) {
				queryCount := 0
				if callbackError := database.Callback().Query().Before("gorm:query").Register(
					"usage_outcome_verify_query",
					func(callbackDatabase *gorm.DB) {
						if callbackDatabase.Statement.Table == managedUsageEventTable {
							queryCount++
							if queryCount == 2 {
								callbackDatabase.AddError(errInternalTestDatabase)
							}
						}
					},
				); callbackError != nil {
					subTest.Fatalf("register verify query callback: %v", callbackError)
				}
			},
		},
		{
			name: "verify count",
			want: "operation=verify",
			configure: func(subTest *testing.T, database *gorm.DB) {
				if callbackError := database.Callback().Query().After("gorm:after_query").Register(
					"usage_outcome_verify_count",
					func(callbackDatabase *gorm.DB) {
						records, correctType := callbackDatabase.Statement.Dest.(*[]managedUsageEventRecord)
						if correctType && len(*records) > 0 {
							*records = (*records)[:len(*records)-1]
						}
					},
				); callbackError != nil {
					subTest.Fatalf("register verify count callback: %v", callbackError)
				}
			},
		},
		{
			name: "verify values",
			want: "operation=verify",
			configure: func(subTest *testing.T, database *gorm.DB) {
				if callbackError := database.Callback().Query().After("gorm:after_query").Register(
					"usage_outcome_verify_values",
					func(callbackDatabase *gorm.DB) {
						records, correctType := callbackDatabase.Statement.Dest.(*[]managedUsageEventRecord)
						if correctType && len(*records) > 0 {
							(*records)[0].OutcomeCode = managedUsageOutcomeUpstreamError
						}
					},
				); callbackError != nil {
					subTest.Fatalf("register verify values callback: %v", callbackError)
				}
			},
		},
		{
			name: "record version",
			want: "operation=record_version",
			configure: func(subTest *testing.T, database *gorm.DB) {
				registerManagedGORMError(subTest, database, "usage_outcome_record_version", "create", managedSchemaMigrationTable, errInternalTestDatabase)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			databasePath := filepath.Join(subTest.TempDir(), "usage-outcome-stage.db")
			activeStage := ""
			var dialector gorm.Dialector = sqlite.Open(databasePath)
			if testCase.stage == "add_column" || testCase.stage == "alter_column" {
				dialector = failingManagedUsageMigrationDialector{
					Dialector: dialector,
					stage:     &activeStage,
				}
			}
			database, openError := gorm.Open(dialector, &gorm.Config{})
			if openError != nil {
				subTest.Fatalf("open stage fixture: %v", openError)
			}
			seedManagedUsageSchemaOne(subTest, database)
			activeStage = testCase.stage
			if testCase.configure != nil {
				testCase.configure(subTest, database)
			}
			migrationError := migrateManagedUsageOutcomeSchema(database)
			if !errors.Is(migrationError, errManagedTenantSchemaMigration) ||
				!strings.Contains(migrationError.Error(), testCase.want) {
				subTest.Fatalf("migration error=%v want=%q", migrationError, testCase.want)
			}
			if managedTableHasColumn(database.Migrator(), managedUsageEventTable, managedUsageOutcomeCodeColumn) {
				subTest.Fatal("failed migration retained outcome column")
			}
			var latest managedSchemaMigrationRecord
			if queryError := database.Order("version DESC").First(&latest).Error; queryError != nil ||
				latest.Version != managedTenantOwnershipSchemaVersion {
				subTest.Fatalf("latest version=%+v error=%v", latest, queryError)
			}
		})
	}
}

type failingManagedUsageMigrationDialector struct {
	gorm.Dialector
	stage *string
}

func (dialector failingManagedUsageMigrationDialector) Migrator(database *gorm.DB) gorm.Migrator {
	return failingManagedUsageMigrationMigrator{
		Migrator: dialector.Dialector.Migrator(database),
		stage:    dialector.stage,
	}
}

type failingManagedUsageMigrationMigrator struct {
	gorm.Migrator
	stage *string
}

func (migrator failingManagedUsageMigrationMigrator) AddColumn(value interface{}, field string) error {
	if *migrator.stage == "add_column" {
		return errInternalTestDatabase
	}
	return migrator.Migrator.AddColumn(value, field)
}

func (migrator failingManagedUsageMigrationMigrator) AlterColumn(value interface{}, field string) error {
	if *migrator.stage == "alter_column" {
		return errInternalTestDatabase
	}
	return migrator.Migrator.AlterColumn(value, field)
}

func (migrator failingManagedUsageMigrationMigrator) CreateIndex(value interface{}, name string) error {
	if *migrator.stage == "create_index" {
		return errInternalTestDatabase
	}
	return migrator.Migrator.CreateIndex(value, name)
}

func seedManagedUsageSchemaOne(t *testing.T, database *gorm.DB) {
	t.Helper()
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create supporting schema: %v", migrationError)
	}
	if dropError := database.Migrator().DropTable(managedUsageEventTable); dropError != nil {
		t.Fatalf("drop current usage table: %v", dropError)
	}
	if migrationError := database.AutoMigrate(&managedUsageEventSchemaOneFixtureRecord{}); migrationError != nil {
		t.Fatalf("create schema-one usage table: %v", migrationError)
	}
	now := time.Date(2026, 7, 25, 18, 30, 0, 0, time.UTC)
	user := managedUserRecord{UserID: "usage-migration-owner", UserEmail: "owner@example.com", CreatedAt: now, UpdatedAt: now}
	tenantRecord := fakeTenantRecord(user.UserID, "usage-migration-tenant", "Usage Migration", now)
	if createError := database.Create(&user).Error; createError != nil {
		t.Fatalf("seed user: %v", createError)
	}
	if createError := database.Create(&tenantRecord).Error; createError != nil {
		t.Fatalf("seed tenant: %v", createError)
	}
	records := []managedUsageEventSchemaOneFixtureRecord{
		{ID: 1, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, TotalTokens: 11, CreatedAt: now},
		{ID: 2, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointText, StatusCode: http.StatusBadRequest, TotalTokens: 2, CreatedAt: now.Add(time.Second)},
		{ID: 3, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointV2, StatusCode: http.StatusRequestEntityTooLarge, TotalTokens: 3, CreatedAt: now.Add(2 * time.Second)},
		{ID: 4, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointText, StatusCode: http.StatusTooManyRequests, TotalTokens: 4, CreatedAt: now.Add(3 * time.Second)},
		{ID: 5, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointText, StatusCode: http.StatusServiceUnavailable, TotalTokens: 5, CreatedAt: now.Add(4 * time.Second)},
		{ID: 6, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointText, StatusCode: http.StatusGatewayTimeout, TotalTokens: 6, CreatedAt: now.Add(5 * time.Second)},
		{ID: 7, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointDictation, StatusCode: http.StatusBadGateway, TotalTokens: 7, CreatedAt: now.Add(6 * time.Second)},
		{ID: 8, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointText, StatusCode: statusClientClosedRequest, TotalTokens: 8, CreatedAt: now.Add(7 * time.Second)},
	}
	if createError := database.Create(&records).Error; createError != nil {
		t.Fatalf("seed usage: %v", createError)
	}
	if createError := database.Create(&managedSchemaMigrationRecord{Version: managedTenantOwnershipSchemaVersion, AppliedAt: now}).Error; createError != nil {
		t.Fatalf("seed schema version: %v", createError)
	}
}

func assertManagedUsageOutcomeMigration(t *testing.T, database *gorm.DB) {
	t.Helper()
	type usageTotals struct {
		Requests    int
		TotalTokens int
	}
	readTotals := func() usageTotals {
		var totals usageTotals
		if queryError := database.Table(managedUsageEventTable).
			Select("COUNT(*) AS requests, COALESCE(SUM(total_tokens), 0) AS total_tokens").
			Scan(&totals).
			Error; queryError != nil {
			t.Fatalf("read usage totals: %v", queryError)
		}
		return totals
	}
	before := readTotals()
	if migrationError := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry()); migrationError != nil {
		t.Fatalf("migrate usage outcomes: %v", migrationError)
	}
	after := readTotals()
	if before != after || after.Requests != 8 || after.TotalTokens != 46 {
		t.Fatalf("usage totals before=%+v after=%+v", before, after)
	}
	var records []managedUsageEventRecord
	if queryError := database.Order("id").Find(&records).Error; queryError != nil {
		t.Fatalf("load migrated usage: %v", queryError)
	}
	actualOutcomes := make([]managedUsageOutcomeCode, 0, len(records))
	for _, record := range records {
		actualOutcomes = append(actualOutcomes, record.OutcomeCode)
	}
	expectedOutcomes := []managedUsageOutcomeCode{
		managedUsageOutcomeSuccess,
		managedUsageOutcomeInvalidRequest,
		managedUsageOutcomePayloadTooLarge,
		managedUsageOutcomeRateLimited,
		managedUsageOutcomeServiceUnavailable,
		managedUsageOutcomeRequestTimeout,
		managedUsageOutcomeUpstreamError,
		managedUsageOutcomeRequestTimeout,
	}
	if !reflect.DeepEqual(actualOutcomes, expectedOutcomes) {
		t.Fatalf("outcomes=%v want=%v", actualOutcomes, expectedOutcomes)
	}
	if !database.Migrator().HasIndex(&managedUsageEventRecord{}, managedUsageFailurePageIndex) {
		t.Fatalf("missing index %s", managedUsageFailurePageIndex)
	}
	if insertError := database.Exec(
		"INSERT INTO "+managedUsageEventTable+" (tenant_id, success, outcome_code, created_at) VALUES (?, ?, NULL, ?)",
		"usage-migration-tenant",
		false,
		time.Now().UTC(),
	).Error; insertError == nil {
		t.Fatal("outcome_code accepted NULL")
	}
	var latest managedSchemaMigrationRecord
	if queryError := database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedTenantSchemaVersion {
		t.Fatalf("latest version=%+v error=%v", latest, queryError)
	}
}
