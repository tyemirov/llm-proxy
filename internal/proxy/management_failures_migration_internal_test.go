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
	"gorm.io/gorm/clause"
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

func TestManagedKeyedRoutingDefaultsMigrationRejectsStageFailures(t *testing.T) {
	providerKeyCipher := internalManagedProviderKeyCipher()
	providers := internalManagementProviderRegistry()
	now := time.Date(2026, 7, 26, 19, 0, 0, 0, time.UTC)
	type fixture struct {
		database *gorm.DB
		tenant   managedTenantRecord
	}
	newFixture := func(subTest *testing.T) fixture {
		database, openError := gorm.Open(sqlite.Open(filepath.Join(subTest.TempDir(), "keyed-defaults-stage.db")), &gorm.Config{})
		if openError != nil {
			subTest.Fatalf("open keyed-defaults fixture: %v", openError)
		}
		if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
			subTest.Fatalf("create keyed-defaults schema: %v", migrationError)
		}
		userRecord := managedUserRecord{UserID: "keyed-defaults-owner", CreatedAt: now, UpdatedAt: now}
		if createError := database.Create(&userRecord).Error; createError != nil {
			subTest.Fatalf("seed keyed-defaults user: %v", createError)
		}
		tenantRecord := fakeTenantRecord(userRecord.UserID, "keyed-defaults-tenant", "Default", now)
		if createError := database.Create(&tenantRecord).Error; createError != nil {
			subTest.Fatalf("seed keyed-defaults tenant: %v", createError)
		}
		return fixture{database: database, tenant: tenantRecord}
	}
	addProvider := func(subTest *testing.T, testFixture fixture, providerIdentifier string, textModel string, encryptedAPIKey string) {
		if encryptedAPIKey == "" {
			var encryptionError error
			encryptedAPIKey, encryptionError = providerKeyCipher.encrypt(
				strings.NewReader(strings.Repeat("a", providerKeyCipher.aeadCipher.NonceSize())),
				testFixture.tenant.TenantID,
				providerIdentifier,
				"sk-key",
			)
			if encryptionError != nil {
				subTest.Fatalf("encrypt keyed-defaults provider: %v", encryptionError)
			}
		}
		if createError := testFixture.database.Create(&managedProviderAPIKeyRecord{
			TenantID:        testFixture.tenant.TenantID,
			ProviderID:      providerIdentifier,
			EncryptedAPIKey: encryptedAPIKey,
			TextModel:       textModel,
			CreatedAt:       now,
			UpdatedAt:       now,
		}).Error; createError != nil {
			subTest.Fatalf("seed keyed-defaults provider: %v", createError)
		}
	}
	assertMigrationError := func(subTest *testing.T, migrationError error, want string) {
		subTest.Helper()
		if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), want) {
			subTest.Fatalf("migration error=%v want=%q", migrationError, want)
		}
	}

	t.Run("read tenants", func(subTest *testing.T) {
		testFixture := newFixture(subTest)
		registerManagedGORMError(subTest, testFixture.database, "keyed_defaults_read", "query", managedTenantTable, errInternalTestDatabase)
		assertMigrationError(subTest, migrateManagedKeyedRoutingDefaults(testFixture.database, providerKeyCipher, providers), "operation=read")
	})
	t.Run("decrypt provider key", func(subTest *testing.T) {
		testFixture := newFixture(subTest)
		addProvider(subTest, testFixture, ProviderNameOpenAI, ModelNameGPT41, "invalid")
		assertMigrationError(subTest, migrateManagedKeyedRoutingDefaults(testFixture.database, providerKeyCipher, providers), "operation=preflight")
	})
	t.Run("invalid defaults", func(subTest *testing.T) {
		testFixture := newFixture(subTest)
		if updateError := testFixture.database.Model(&managedTenantRecord{}).
			Where(&managedTenantRecord{TenantID: testFixture.tenant.TenantID}).
			Update("default_provider", "missing").
			Error; updateError != nil {
			subTest.Fatalf("write invalid defaults: %v", updateError)
		}
		assertMigrationError(subTest, migrateManagedKeyedRoutingDefaults(testFixture.database, providerKeyCipher, providers), "operation=preflight")
	})
	t.Run("invalid provider settings", func(subTest *testing.T) {
		testFixture := newFixture(subTest)
		addProvider(subTest, testFixture, "missing", "missing-model", "")
		assertMigrationError(subTest, migrateManagedKeyedRoutingDefaults(testFixture.database, providerKeyCipher, providers), "operation=preflight")
	})
	t.Run("backfill", func(subTest *testing.T) {
		testFixture := newFixture(subTest)
		registerManagedGORMError(subTest, testFixture.database, "keyed_defaults_backfill", "update", managedTenantTable, errInternalTestDatabase)
		assertMigrationError(subTest, migrateManagedKeyedRoutingDefaults(testFixture.database, providerKeyCipher, providers), "operation=backfill")
	})
	t.Run("validation", func(subTest *testing.T) {
		testFixture := newFixture(subTest)
		queryCount := 0
		if callbackError := testFixture.database.Callback().Query().Before("gorm:query").Register(
			"keyed_defaults_validation",
			func(callbackDatabase *gorm.DB) {
				if callbackDatabase.Statement.Table == managedTenantTable {
					queryCount++
					if queryCount == 2 {
						callbackDatabase.AddError(errInternalTestDatabase)
					}
				}
			},
		); callbackError != nil {
			subTest.Fatalf("register keyed-defaults validation callback: %v", callbackError)
		}
		assertMigrationError(subTest, migrateManagedKeyedRoutingDefaults(testFixture.database, providerKeyCipher, providers), "operation=read")
	})
	t.Run("record version", func(subTest *testing.T) {
		testFixture := newFixture(subTest)
		registerManagedGORMError(subTest, testFixture.database, "keyed_defaults_record_version", "create", managedSchemaMigrationTable, errInternalTestDatabase)
		assertMigrationError(subTest, migrateManagedKeyedRoutingDefaults(testFixture.database, providerKeyCipher, providers), "operation=record_version")
	})
	t.Run("validate provider decryption", func(subTest *testing.T) {
		testFixture := newFixture(subTest)
		addProvider(subTest, testFixture, ProviderNameOpenAI, ModelNameGPT41, "invalid")
		assertMigrationError(subTest, validateManagedKeyedRoutingDefaults(testFixture.database, providerKeyCipher, providers), "operation=validate")
	})
}

type managedModelIdentityMigrationFixture struct {
	database          *gorm.DB
	providerKeyCipher managedProviderKeyCipher
	providers         *providerRegistry
	tenant            managedTenantRecord
	usage             managedUsageEventRecord
}

func newManagedModelIdentityMigrationFixture(t *testing.T) managedModelIdentityMigrationFixture {
	t.Helper()
	database, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "model-identity-stage.db")), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open model identity fixture: %v", openError)
	}
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create model identity schema: %v", migrationError)
	}
	now := time.Date(2026, 8, 10, 23, 30, 0, 0, time.UTC)
	user := managedUserRecord{UserID: "model-identity-owner", CreatedAt: now, UpdatedAt: now}
	if createError := database.Create(&user).Error; createError != nil {
		t.Fatalf("seed model identity user: %v", createError)
	}
	tenantRecord := fakeTenantRecord(user.UserID, "model-identity-tenant", "Default", now)
	tenantRecord.DefaultProvider = ProviderNameMiniMax
	tenantRecord.DefaultModel = managedMiniMaxNativeModel
	tenantRecord.DefaultDictationProvider = ProviderNameSiliconFlow
	tenantRecord.DefaultDictationModel = managedSenseVoiceNativeModel
	if createError := database.Create(&tenantRecord).Error; createError != nil {
		t.Fatalf("seed model identity tenant: %v", createError)
	}
	providerKeyCipher := internalManagedProviderKeyCipher()
	encryptProviderKey := func(providerIdentifier string, rawAPIKey string, nonceByte string) string {
		t.Helper()
		encryptedKey, encryptionError := providerKeyCipher.encrypt(
			strings.NewReader(strings.Repeat(nonceByte, providerKeyCipher.aeadCipher.NonceSize())),
			tenantRecord.TenantID,
			providerIdentifier,
			rawAPIKey,
		)
		if encryptionError != nil {
			t.Fatalf("encrypt model identity provider=%s: %v", providerIdentifier, encryptionError)
		}
		return encryptedKey
	}
	providerKeys := []managedProviderAPIKeyRecord{
		{TenantID: tenantRecord.TenantID, ProviderID: ProviderNameMiniMax, EncryptedAPIKey: encryptProviderKey(ProviderNameMiniMax, "sk-minimax", "m"), TextModel: managedMiniMaxNativeModel, CreatedAt: now, UpdatedAt: now},
		{TenantID: tenantRecord.TenantID, ProviderID: ProviderNameSiliconFlow, EncryptedAPIKey: encryptProviderKey(ProviderNameSiliconFlow, "sk-siliconflow", "s"), TextModel: managedSiliconFlowDeepSeekNativeModel, CreatedAt: now, UpdatedAt: now},
	}
	if createError := database.Create(&providerKeys).Error; createError != nil {
		t.Fatalf("seed model identity provider keys: %v", createError)
	}
	usage := managedUsageEventRecord{
		ID: 91, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointText,
		ProviderID: ProviderNameMiniMax, ModelID: managedMiniMaxNativeModel,
		StatusCode: http.StatusOK, Success: true, OutcomeCode: managedUsageOutcomeSuccess, CreatedAt: now,
	}
	if createError := database.Create(&usage).Error; createError != nil {
		t.Fatalf("seed model identity usage: %v", createError)
	}
	return managedModelIdentityMigrationFixture{
		database: database, providerKeyCipher: providerKeyCipher,
		providers: internalManagementProviderRegistry(), tenant: tenantRecord, usage: usage,
	}
}

func TestManagedModelIdentityMigrationRejectsStageFailures(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		want      string
		configure func(*testing.T, managedModelIdentityMigrationFixture)
	}{
		{
			name: "read tenants", want: "operation=read",
			configure: func(t *testing.T, fixture managedModelIdentityMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "model_identity_read", "query", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "decrypt provider", want: "operation=preflight",
			configure: func(t *testing.T, fixture managedModelIdentityMigrationFixture) {
				if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).
					Where(&managedProviderAPIKeyRecord{TenantID: fixture.tenant.TenantID, ProviderID: ProviderNameMiniMax}).
					Update("encrypted_api_key", "invalid").Error; updateError != nil {
					t.Fatalf("seed invalid model identity provider: %v", updateError)
				}
			},
		},
		{
			name: "invalid defaults", want: "operation=preflight",
			configure: func(t *testing.T, fixture managedModelIdentityMigrationFixture) {
				if updateError := fixture.database.Model(&managedTenantRecord{}).
					Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).
					Update("default_model", "missing-model").Error; updateError != nil {
					t.Fatalf("seed invalid model identity defaults: %v", updateError)
				}
			},
		},
		{
			name: "read historical usage", want: "historical_model_identity",
			configure: func(t *testing.T, fixture managedModelIdentityMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "model_identity_usage_read", "query", managedUsageEventTable, errInternalTestDatabase)
			},
		},
		{
			name: "backfill provider key", want: "operation=backfill table=" + managedProviderKeyTable,
			configure: func(t *testing.T, fixture managedModelIdentityMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "model_identity_provider_backfill", "update", managedProviderKeyTable, errInternalTestDatabase)
			},
		},
		{
			name: "backfill tenant", want: "operation=backfill table=" + managedTenantTable,
			configure: func(t *testing.T, fixture managedModelIdentityMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "model_identity_tenant_backfill", "update", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "validate migrated routes", want: "operation=read",
			configure: func(t *testing.T, fixture managedModelIdentityMigrationFixture) {
				queryCount := 0
				if callbackError := fixture.database.Callback().Query().Before("gorm:query").Register("model_identity_validate", func(callbackDatabase *gorm.DB) {
					if callbackDatabase.Statement.Table == managedTenantTable {
						queryCount++
						if queryCount == 2 {
							callbackDatabase.AddError(errInternalTestDatabase)
						}
					}
				}); callbackError != nil {
					t.Fatalf("register model identity validation failure: %v", callbackError)
				}
			},
		},
		{
			name: "verify historical usage read", want: "historical_model_identity",
			configure: func(t *testing.T, fixture managedModelIdentityMigrationFixture) {
				queryCount := 0
				if callbackError := fixture.database.Callback().Query().Before("gorm:query").Register("model_identity_usage_verify", func(callbackDatabase *gorm.DB) {
					if callbackDatabase.Statement.Table == managedUsageEventTable {
						queryCount++
						if queryCount == 2 {
							callbackDatabase.AddError(errInternalTestDatabase)
						}
					}
				}); callbackError != nil {
					t.Fatalf("register model identity usage verification failure: %v", callbackError)
				}
			},
		},
		{
			name: "verify historical usage drift", want: "historical_usage_changed",
			configure: func(t *testing.T, fixture managedModelIdentityMigrationFixture) {
				queryCount := 0
				if callbackError := fixture.database.Callback().Query().Before("gorm:query").Register("model_identity_usage_drift", func(callbackDatabase *gorm.DB) {
					if callbackDatabase.Statement.Table == managedUsageEventTable {
						queryCount++
						if queryCount == 2 {
							callbackDatabase.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "1 = 0"}}})
						}
					}
				}); callbackError != nil {
					t.Fatalf("register model identity usage drift: %v", callbackError)
				}
			},
		},
		{
			name: "record version", want: "operation=record_version",
			configure: func(t *testing.T, fixture managedModelIdentityMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "model_identity_record_version", "create", managedSchemaMigrationTable, errInternalTestDatabase)
			},
		},
	} {
		t.Run(testCase.name, func(subTest *testing.T) {
			fixture := newManagedModelIdentityMigrationFixture(subTest)
			testCase.configure(subTest, fixture)
			migrationError := migrateManagedModelIdentity(fixture.database, fixture.providerKeyCipher, fixture.providers)
			if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), testCase.want) {
				subTest.Fatalf("migration error=%v want=%q", migrationError, testCase.want)
			}
		})
	}
}

type managedQwenCloudRetirementFixture struct {
	database          *gorm.DB
	providerKeyCipher managedProviderKeyCipher
	providers         *providerRegistry
	tenant            managedTenantRecord
	usage             managedUsageEventRecord
}

func newManagedQwenCloudRetirementFixture(t *testing.T) managedQwenCloudRetirementFixture {
	t.Helper()
	database, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "qwen-retirement-stage.db")), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open qwen retirement fixture: %v", openError)
	}
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create qwen retirement schema: %v", migrationError)
	}
	now := time.Date(2026, 8, 10, 22, 0, 0, 0, time.UTC)
	user := managedUserRecord{UserID: "qwen-retirement-owner", CreatedAt: now, UpdatedAt: now}
	if createError := database.Create(&user).Error; createError != nil {
		t.Fatalf("seed qwen retirement user: %v", createError)
	}
	tenantRecord := fakeTenantRecord(user.UserID, "qwen-retirement-tenant", "Default", now)
	tenantRecord.DefaultProvider = retiredQwenCloudProviderIdentifier
	tenantRecord.DefaultModel = retiredQwenCloudModelIdentifier
	tenantRecord.DefaultReasoningEffort = "high"
	if createError := database.Create(&tenantRecord).Error; createError != nil {
		t.Fatalf("seed qwen retirement tenant: %v", createError)
	}
	providerKeyCipher := internalManagedProviderKeyCipher()
	encryptedKey, encryptionError := providerKeyCipher.encrypt(
		strings.NewReader(strings.Repeat("q", providerKeyCipher.aeadCipher.NonceSize())),
		tenantRecord.TenantID,
		retiredQwenCloudProviderIdentifier,
		"sk-retired",
	)
	if encryptionError != nil {
		t.Fatalf("encrypt qwen retirement key: %v", encryptionError)
	}
	if createError := database.Create(&managedProviderAPIKeyRecord{
		TenantID: tenantRecord.TenantID, ProviderID: retiredQwenCloudProviderIdentifier,
		EncryptedAPIKey: encryptedKey, TextModel: retiredQwenCloudModelIdentifier,
		SystemPrompt: "retired prompt", CreatedAt: now, UpdatedAt: now,
	}).Error; createError != nil {
		t.Fatalf("seed qwen retirement key: %v", createError)
	}
	usage := managedUsageEventRecord{
		ID: 71, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointText,
		ProviderID: retiredQwenCloudProviderIdentifier, ModelID: retiredQwenCloudModelIdentifier,
		StatusCode: http.StatusOK, Success: true, OutcomeCode: managedUsageOutcomeSuccess, CreatedAt: now,
	}
	if createError := database.Create(&usage).Error; createError != nil {
		t.Fatalf("seed qwen retirement usage: %v", createError)
	}
	return managedQwenCloudRetirementFixture{
		database: database, providerKeyCipher: providerKeyCipher,
		providers: internalManagementProviderRegistry(), tenant: tenantRecord, usage: usage,
	}
}

func (fixture managedQwenCloudRetirementFixture) addProvider(t *testing.T, providerIdentifier string, textModel string, encryptedAPIKey string) {
	t.Helper()
	if encryptedAPIKey == "" {
		var encryptionError error
		encryptedAPIKey, encryptionError = fixture.providerKeyCipher.encrypt(
			strings.NewReader(strings.Repeat("r", fixture.providerKeyCipher.aeadCipher.NonceSize())),
			fixture.tenant.TenantID,
			providerIdentifier,
			"sk-current",
		)
		if encryptionError != nil {
			t.Fatalf("encrypt qwen retirement provider: %v", encryptionError)
		}
	}
	if createError := fixture.database.Create(&managedProviderAPIKeyRecord{
		TenantID: fixture.tenant.TenantID, ProviderID: providerIdentifier,
		EncryptedAPIKey: encryptedAPIKey, TextModel: textModel,
		CreatedAt: fixture.tenant.CreatedAt, UpdatedAt: fixture.tenant.UpdatedAt,
	}).Error; createError != nil {
		t.Fatalf("seed qwen retirement provider: %v", createError)
	}
}

func TestManagedQwenCloudRetirementMigrationRejectsStageFailures(t *testing.T) {
	assertMigrationError := func(t *testing.T, migrationError error, want string) {
		t.Helper()
		if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), want) {
			t.Fatalf("migration error=%v want=%q", migrationError, want)
		}
	}
	for _, testCase := range []struct {
		name      string
		want      string
		configure func(*testing.T, managedQwenCloudRetirementFixture)
	}{
		{
			name: "read tenants", want: "operation=read",
			configure: func(t *testing.T, fixture managedQwenCloudRetirementFixture) {
				registerManagedGORMError(t, fixture.database, "qwen_retirement_read", "query", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "decrypt remaining provider", want: "operation=preflight",
			configure: func(t *testing.T, fixture managedQwenCloudRetirementFixture) {
				fixture.addProvider(t, ProviderNameDeepSeek, ModelNameDeepSeekV4Flash, "invalid")
			},
		},
		{
			name: "invalid retired defaults", want: "operation=preflight",
			configure: func(t *testing.T, fixture managedQwenCloudRetirementFixture) {
				if updateError := fixture.database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).Update("default_model", "wrong-model").Error; updateError != nil {
					t.Fatalf("seed invalid retired defaults: %v", updateError)
				}
			},
		},
		{
			name: "invalid retained defaults", want: "operation=preflight",
			configure: func(t *testing.T, fixture managedQwenCloudRetirementFixture) {
				if updateError := fixture.database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).Update("default_dictation_provider", "missing").Error; updateError != nil {
					t.Fatalf("seed invalid retained defaults: %v", updateError)
				}
			},
		},
		{
			name: "invalid remaining provider", want: "operation=preflight",
			configure: func(t *testing.T, fixture managedQwenCloudRetirementFixture) {
				fixture.addProvider(t, "missing", "missing-model", "")
			},
		},
		{
			name: "read historical usage", want: "operation=preflight",
			configure: func(t *testing.T, fixture managedQwenCloudRetirementFixture) {
				registerManagedGORMError(t, fixture.database, "qwen_retirement_usage_read", "query", managedUsageEventTable, errInternalTestDatabase)
			},
		},
		{
			name: "delete retired provider", want: "operation=delete_retired_provider",
			configure: func(t *testing.T, fixture managedQwenCloudRetirementFixture) {
				registerManagedGORMError(t, fixture.database, "qwen_retirement_delete", "delete", managedProviderKeyTable, errInternalTestDatabase)
			},
		},
		{
			name: "backfill", want: "operation=backfill",
			configure: func(t *testing.T, fixture managedQwenCloudRetirementFixture) {
				registerManagedGORMError(t, fixture.database, "qwen_retirement_backfill", "update", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "verify", want: "operation=verify",
			configure: func(t *testing.T, fixture managedQwenCloudRetirementFixture) {
				if callbackError := fixture.database.Callback().Query().Before("gorm:query").Register("qwen_retirement_verify", func(callbackDatabase *gorm.DB) {
					if callbackDatabase.Statement.Table == managedProviderKeyTable {
						if _, isCount := callbackDatabase.Statement.Dest.(*int64); isCount {
							callbackDatabase.AddError(errInternalTestDatabase)
						}
					}
				}); callbackError != nil {
					t.Fatalf("register qwen verification failure: %v", callbackError)
				}
			},
		},
		{
			name: "record version", want: "operation=record_version",
			configure: func(t *testing.T, fixture managedQwenCloudRetirementFixture) {
				registerManagedGORMError(t, fixture.database, "qwen_retirement_record_version", "create", managedSchemaMigrationTable, errInternalTestDatabase)
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newManagedQwenCloudRetirementFixture(t)
			testCase.configure(t, fixture)
			assertMigrationError(t, migrateManagedQwenCloudRetirement(fixture.database, fixture.providerKeyCipher, fixture.providers), testCase.want)
		})
	}
}

func TestManagedTenantSchemaStopsBeforeModelIdentityWhenQwenRetirementFails(t *testing.T) {
	fixture := newManagedQwenCloudRetirementFixture(t)
	if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedKeyedRoutingSchemaVersion, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
		t.Fatalf("seed schema version: %v", createError)
	}
	if updateError := fixture.database.Model(&managedTenantRecord{}).
		Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).
		Update("default_model", "wrong-model").Error; updateError != nil {
		t.Fatalf("seed invalid retired defaults: %v", updateError)
	}
	initializeError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers)
	if !errors.Is(initializeError, errManagedTenantSchemaMigration) || !strings.Contains(initializeError.Error(), "operation=preflight") {
		t.Fatalf("initialize error=%v", initializeError)
	}
}

func TestManagedQwenCloudRetirementVerificationRejectsDrift(t *testing.T) {
	assertVerificationError := func(t *testing.T, verificationError error, want string) {
		t.Helper()
		if !errors.Is(verificationError, errManagedTenantSchemaMigration) || !strings.Contains(verificationError.Error(), want) {
			t.Fatalf("verification error=%v want=%q", verificationError, want)
		}
	}
	verifiedFixture := func(t *testing.T) (managedQwenCloudRetirementFixture, managedQwenCloudRetirementDataset) {
		t.Helper()
		fixture := newManagedQwenCloudRetirementFixture(t)
		dataset, preflightError := preflightManagedQwenCloudRetirement(fixture.database, fixture.providerKeyCipher, fixture.providers)
		if preflightError != nil {
			t.Fatalf("preflight verification fixture: %v", preflightError)
		}
		if deleteError := fixture.database.Where(&managedProviderAPIKeyRecord{ProviderID: retiredQwenCloudProviderIdentifier}).Delete(&managedProviderAPIKeyRecord{}).Error; deleteError != nil {
			t.Fatalf("delete retired verification fixture key: %v", deleteError)
		}
		for _, backfill := range dataset.backfills {
			if updateError := fixture.database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: backfill.tenantID}).Updates(managedRoutingDefaultsDatabaseUpdates(backfill.defaults, backfill.updatedAt)).Error; updateError != nil {
				t.Fatalf("backfill verification fixture: %v", updateError)
			}
		}
		return fixture, dataset
	}

	t.Run("retired provider remains", func(t *testing.T) {
		fixture := newManagedQwenCloudRetirementFixture(t)
		assertVerificationError(t, verifyManagedQwenCloudRetirement(fixture.database, fixture.providerKeyCipher, fixture.providers, managedQwenCloudRetirementDataset{}), "operation=verify")
	})
	t.Run("tenant query", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		registerManagedGORMError(t, fixture.database, "qwen_verify_tenant_query", "query", managedTenantTable, errInternalTestDatabase)
		assertVerificationError(t, verifyManagedQwenCloudRetirement(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=verify")
	})
	t.Run("tenant values", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		dataset.backfills[0].updatedAt = dataset.backfills[0].updatedAt.Add(time.Second)
		assertVerificationError(t, verifyManagedQwenCloudRetirement(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "values")
	})
	t.Run("current routing validation", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		fixture.addProvider(t, ProviderNameDeepSeek, ModelNameDeepSeekV4Flash, "invalid")
		assertVerificationError(t, verifyManagedQwenCloudRetirement(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=validate")
	})
	t.Run("usage query", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		registerManagedGORMError(t, fixture.database, "qwen_verify_usage_query", "query", managedUsageEventTable, errInternalTestDatabase)
		assertVerificationError(t, verifyManagedQwenCloudRetirement(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=verify")
	})
	t.Run("usage count", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		dataset.historicalUsage = nil
		assertVerificationError(t, verifyManagedQwenCloudRetirement(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "expected=0")
	})
	t.Run("usage values", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		dataset.historicalUsage[0].ModelID = "changed"
		assertVerificationError(t, verifyManagedQwenCloudRetirement(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "id=71")
	})
}

func TestManagedTenantInitializationPropagatesPreRetirementMigrationFailures(t *testing.T) {
	t.Run("schema one keyed routing failure", func(t *testing.T) {
		database, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "schema-one-keyed-failure.db")), &gorm.Config{})
		if openError != nil {
			t.Fatalf("open schema-one fixture: %v", openError)
		}
		seedManagedUsageSchemaOne(t, database)
		registerManagedGORMError(t, database, "schema_one_keyed_failure", "query", managedTenantTable, errInternalTestDatabase)
		initializeError := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
		if !errors.Is(initializeError, errManagedTenantSchemaMigration) || !strings.Contains(initializeError.Error(), "operation=read") {
			t.Fatalf("schema-one initialize error=%v", initializeError)
		}
	})

	t.Run("schema two keyed routing failure", func(t *testing.T) {
		database, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "schema-two-keyed-failure.db")), &gorm.Config{})
		if openError != nil {
			t.Fatalf("open schema-two fixture: %v", openError)
		}
		if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
			t.Fatalf("create schema-two fixture: %v", migrationError)
		}
		now := time.Date(2026, 8, 10, 23, 0, 0, 0, time.UTC)
		user := managedUserRecord{UserID: "schema-two-owner", CreatedAt: now, UpdatedAt: now}
		if createError := database.Create(&user).Error; createError != nil {
			t.Fatalf("seed schema-two user: %v", createError)
		}
		tenantRecord := fakeTenantRecord(user.UserID, "schema-two-tenant", "Default", now)
		tenantRecord.DefaultProvider = "missing"
		tenantRecord.DefaultModel = "missing-model"
		if createError := database.Create(&tenantRecord).Error; createError != nil {
			t.Fatalf("seed schema-two tenant: %v", createError)
		}
		if createError := database.Create(&managedSchemaMigrationRecord{Version: managedUsageOutcomeSchemaVersion, AppliedAt: now}).Error; createError != nil {
			t.Fatalf("seed schema-two version: %v", createError)
		}
		initializeError := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
		if !errors.Is(initializeError, errManagedTenantSchemaMigration) || !strings.Contains(initializeError.Error(), "operation=preflight") {
			t.Fatalf("schema-two initialize error=%v", initializeError)
		}
	})
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
