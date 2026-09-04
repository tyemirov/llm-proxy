package proxy

import (
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
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
					"CREATE INDEX " + managedUsageLegacyFailurePageIndex + " ON managed_usage_index_collision (id)",
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
	useManagedUsageSchemaTwelve(t, database)
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
		StatusCode: http.StatusOK, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess, CreatedAt: now,
	}
	createManagedUsageSchemaTwelve(t, database, usage)
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
							callbackDatabase.Statement.Clauses["WHERE"] = clause.Clause{Name: "WHERE", Expression: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "1 = 0"}}}}
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

func TestManagedTenantInitializationPropagatesModelIdentityFailure(t *testing.T) {
	fixture := newManagedModelIdentityMigrationFixture(t)
	if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedQwenCloudRetirementVersion, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
		t.Fatalf("seed schema version: %v", createError)
	}
	if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).
		Where(&managedProviderAPIKeyRecord{TenantID: fixture.tenant.TenantID, ProviderID: ProviderNameMiniMax}).
		Update("encrypted_api_key", "invalid").Error; updateError != nil {
		t.Fatalf("seed invalid model identity provider: %v", updateError)
	}
	initializeError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers)
	if !errors.Is(initializeError, errManagedTenantSchemaMigration) || !strings.Contains(initializeError.Error(), "operation=preflight") {
		t.Fatalf("initialize error=%v", initializeError)
	}
}

func TestManagedModelMigrationPolicyIsRequiredByEachDatabaseMigration(t *testing.T) {
	fixture := newManagedModelIdentityMigrationFixture(t)

	for _, schemaVersion := range managedModelSelectionSchemaVersions {
		missingSelectionPolicy := internalManagementProviderRegistry()
		delete(missingSelectionPolicy.modelMigrations, schemaVersion)
		selectionError := migrateManagedModelSelections(fixture.database, fixture.providerKeyCipher, missingSelectionPolicy, schemaVersion)
		if !errors.Is(selectionError, errManagedTenantSchemaMigration) || !strings.Contains(selectionError.Error(), "operation=read_model_migrations") {
			t.Fatalf("missing selection policy version=%d error=%v", schemaVersion, selectionError)
		}
	}

	retirementSelectionPolicy := internalManagementProviderRegistry()
	retirementSelectionPolicy.modelMigrations[managedGemini3OnlySchemaVersion] = retirementSelectionPolicy.modelMigrations[managedQwenCloudRetirementVersion]
	selectionError := migrateManagedModelSelections(fixture.database, fixture.providerKeyCipher, retirementSelectionPolicy, managedGemini3OnlySchemaVersion)
	if !errors.Is(selectionError, errManagedTenantSchemaMigration) || !strings.Contains(selectionError.Error(), "operation=read_model_migrations") {
		t.Fatalf("retirement selection policy error=%v", selectionError)
	}

	missingRetirementPolicy := internalManagementProviderRegistry()
	delete(missingRetirementPolicy.modelMigrations, managedQwenCloudRetirementVersion)
	if _, retirementError := preflightManagedQwenCloudRetirement(fixture.database, fixture.providerKeyCipher, missingRetirementPolicy); !errors.Is(retirementError, errManagedTenantSchemaMigration) || !strings.Contains(retirementError.Error(), "operation=read_model_migrations") {
		t.Fatalf("missing retirement policy error=%v", retirementError)
	}

	missingIdentityPolicy := internalManagementProviderRegistry()
	delete(missingIdentityPolicy.modelMigrations, managedModelIdentitySchemaVersion)
	if _, usageError := managedModelIdentityHistoricalUsage(fixture.database, missingIdentityPolicy); !errors.Is(usageError, errManagedTenantSchemaMigration) || !strings.Contains(usageError.Error(), "operation=read_model_migrations") {
		t.Fatalf("missing identity policy error=%v", usageError)
	}
}

type managedXAIProviderMigrationFixture struct {
	database          *gorm.DB
	providerKeyCipher managedProviderKeyCipher
	providers         *providerRegistry
	tenant            managedTenantRecord
	providerKey       managedProviderAPIKeyRecord
	usage             managedUsageEventRecord
}

type managedXAIProviderFailingReader struct{}

func (managedXAIProviderFailingReader) Read([]byte) (int, error) {
	return 0, errInternalTestDatabase
}

func newManagedXAIProviderMigrationFixture(t *testing.T) managedXAIProviderMigrationFixture {
	t.Helper()
	database, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "xai-provider-stage.db")), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open xAI provider fixture: %v", openError)
	}
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create xAI provider schema: %v", migrationError)
	}
	useManagedUsageSchemaTwelve(t, database)
	now := time.Date(2026, 8, 10, 23, 45, 0, 0, time.UTC)
	user := managedUserRecord{UserID: "xai-provider-owner", CreatedAt: now, UpdatedAt: now}
	if createError := database.Create(&user).Error; createError != nil {
		t.Fatalf("seed xAI provider user: %v", createError)
	}
	tenantRecord := fakeTenantRecord(user.UserID, "xai-provider-tenant", "Default", now)
	tenantRecord.DefaultProvider = retiredGrokProviderIdentifier
	tenantRecord.DefaultModel = ModelNameGrok43
	tenantRecord.DefaultDictationProvider = retiredGrokProviderIdentifier
	tenantRecord.DefaultDictationModel = "xai-stt"
	tenantRecord.DefaultSystemPrompt = "preserve tenant prompt"
	if createError := database.Create(&tenantRecord).Error; createError != nil {
		t.Fatalf("seed xAI provider tenant: %v", createError)
	}
	providerKeyCipher := internalManagedProviderKeyCipher()
	encryptedKey, encryptionError := providerKeyCipher.encrypt(
		strings.NewReader(strings.Repeat("x", providerKeyCipher.aeadCipher.NonceSize())),
		tenantRecord.TenantID,
		retiredGrokProviderIdentifier,
		"sk-grok",
	)
	if encryptionError != nil {
		t.Fatalf("encrypt xAI provider key: %v", encryptionError)
	}
	providerKey := managedProviderAPIKeyRecord{
		TenantID: tenantRecord.TenantID, ProviderID: retiredGrokProviderIdentifier,
		EncryptedAPIKey: encryptedKey, TextModel: ModelNameGrok43,
		SystemPrompt: "preserve provider prompt", CreatedAt: now, UpdatedAt: now,
	}
	if createError := database.Create(&providerKey).Error; createError != nil {
		t.Fatalf("seed xAI provider key: %v", createError)
	}
	usage := managedUsageEventRecord{
		ID: 101, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointText,
		ProviderID: retiredGrokProviderIdentifier, ModelID: ModelNameGrok43,
		StatusCode: http.StatusOK, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess, CreatedAt: now,
	}
	createManagedUsageSchemaTwelve(t, database, usage)
	return managedXAIProviderMigrationFixture{
		database: database, providerKeyCipher: providerKeyCipher,
		providers: internalManagementProviderRegistry(), tenant: tenantRecord,
		providerKey: providerKey, usage: usage,
	}
}

func (fixture managedXAIProviderMigrationFixture) addProvider(t *testing.T, providerIdentifier string, textModel string, encryptedAPIKey string) {
	t.Helper()
	if encryptedAPIKey == "" {
		var encryptionError error
		encryptedAPIKey, encryptionError = fixture.providerKeyCipher.encrypt(
			strings.NewReader(strings.Repeat("p", fixture.providerKeyCipher.aeadCipher.NonceSize())),
			fixture.tenant.TenantID,
			providerIdentifier,
			"sk-provider",
		)
		if encryptionError != nil {
			t.Fatalf("encrypt provider=%s: %v", providerIdentifier, encryptionError)
		}
	}
	if createError := fixture.database.Create(&managedProviderAPIKeyRecord{
		TenantID: fixture.tenant.TenantID, ProviderID: providerIdentifier,
		EncryptedAPIKey: encryptedAPIKey, TextModel: textModel,
		CreatedAt: fixture.tenant.CreatedAt, UpdatedAt: fixture.tenant.UpdatedAt,
	}).Error; createError != nil {
		t.Fatalf("seed provider=%s: %v", providerIdentifier, createError)
	}
}

func applyManagedXAIProviderDataset(t *testing.T, fixture managedXAIProviderMigrationFixture, dataset managedXAIProviderDataset) {
	t.Helper()
	for _, backfill := range dataset.providerKeys {
		if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).
			Where(&managedProviderAPIKeyRecord{TenantID: backfill.record.TenantID, ProviderID: retiredGrokProviderIdentifier}).
			UpdateColumns(map[string]any{"provider_id": backfill.record.ProviderID, "encrypted_api_key": backfill.record.EncryptedAPIKey}).Error; updateError != nil {
			t.Fatalf("apply xAI provider key: %v", updateError)
		}
	}
	for _, backfill := range dataset.tenants {
		if updateError := fixture.database.Model(&managedTenantRecord{}).
			Where(&managedTenantRecord{TenantID: backfill.tenantID}).
			Updates(managedRoutingDefaultsDatabaseUpdates(backfill.defaults, backfill.updatedAt)).Error; updateError != nil {
			t.Fatalf("apply xAI tenant defaults: %v", updateError)
		}
	}
}

func TestManagedXAIProviderMigrationCanonicalizesCurrentRoutesAndPreservesUsage(t *testing.T) {
	fixture := newManagedXAIProviderMigrationFixture(t)
	if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedModelIdentitySchemaVersion, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
		t.Fatalf("seed schema version: %v", createError)
	}
	if migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers); migrationError != nil {
		t.Fatalf("migrate xAI provider: %v", migrationError)
	}
	if fixture.database.Migrator().HasTable(managedProviderKeyTable) {
		t.Fatal("predecessor provider table remains after xAI connection migration")
	}
	var connection managedProviderConnectionRecord
	if queryError := fixture.database.Where(&managedProviderConnectionRecord{TenantID: fixture.tenant.TenantID, ProviderID: ProviderNameXAI, FieldID: CatalogCredentialAPIKey}).First(&connection).Error; queryError != nil {
		t.Fatalf("load xAI provider connection: %v", queryError)
	}
	var profile managedProviderProfileRecord
	if queryError := fixture.database.Where(&managedProviderProfileRecord{TenantID: fixture.tenant.TenantID, ProviderID: ProviderNameXAI}).First(&profile).Error; queryError != nil {
		t.Fatalf("load xAI provider profile: %v", queryError)
	}
	apiKey, decryptError := fixture.providerKeyCipher.decryptConnection(connection)
	if decryptError != nil || apiKey != "sk-grok" || profile.TextModel != fixture.providerKey.TextModel || profile.SystemPrompt != fixture.providerKey.SystemPrompt || !connection.CreatedAt.Equal(fixture.providerKey.CreatedAt) || !connection.UpdatedAt.Equal(fixture.providerKey.UpdatedAt) || !profile.CreatedAt.Equal(fixture.providerKey.CreatedAt) || !profile.UpdatedAt.Equal(fixture.providerKey.UpdatedAt) {
		t.Fatalf("migrated connection=%+v profile=%+v api_key=%q error=%v", connection, profile, apiKey, decryptError)
	}
	var tenantRecord managedTenantRecord
	if queryError := fixture.database.Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).First(&tenantRecord).Error; queryError != nil {
		t.Fatalf("load xAI tenant: %v", queryError)
	}
	expectedDefaults := TenantDefaults{
		Provider: ProviderNameXAI, Model: ModelNameGrok43,
		DictationProvider: ProviderNameXAI, DictationModel: "xai-stt",
		SystemPrompt: "preserve tenant prompt",
	}
	if tenantRecord.defaults() != expectedDefaults || !tenantRecord.UpdatedAt.Equal(fixture.tenant.UpdatedAt) {
		t.Fatalf("migrated defaults=%+v updated_at=%s", tenantRecord.defaults(), tenantRecord.UpdatedAt)
	}
	var historicalUsage managedUsageEventRecord
	if queryError := fixture.database.First(&historicalUsage, fixture.usage.ID).Error; queryError != nil || historicalUsage != managedUsageRecordWithoutRoute(fixture.usage) {
		t.Fatalf("historical usage=%+v error=%v", historicalUsage, queryError)
	}
	var latest managedSchemaMigrationRecord
	if queryError := fixture.database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedTenantSchemaVersion {
		t.Fatalf("latest version=%+v error=%v", latest, queryError)
	}
	if validationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers); validationError != nil {
		t.Fatalf("reopen xAI provider schema: %v", validationError)
	}
}

func TestManagedTenantRouteMigrationsComposeConfirmedPredecessorIdentities(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		version int
	}{
		{name: "schema two", version: managedUsageOutcomeSchemaVersion},
		{name: "schema three", version: managedKeyedRoutingSchemaVersion},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newManagedXAIProviderMigrationFixture(t)
			fixture.addProvider(t, ProviderNameMiniMax, managedMiniMaxNativeModel, "")
			fixture.addProvider(t, ProviderNameSiliconFlow, managedSiliconFlowDeepSeekNativeModel, "")
			historicalUsage := []managedUsageEventRecord{
				{ID: 102, TenantID: fixture.tenant.TenantID, Endpoint: usageEndpointText, ProviderID: ProviderNameMiniMax, ModelID: managedMiniMaxNativeModel, StatusCode: http.StatusOK, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess, CreatedAt: fixture.tenant.CreatedAt.Add(time.Minute)},
				{ID: 103, TenantID: fixture.tenant.TenantID, Endpoint: usageEndpointDictation, ProviderID: ProviderNameSiliconFlow, ModelID: managedSenseVoiceNativeModel, StatusCode: http.StatusOK, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess, CreatedAt: fixture.tenant.CreatedAt.Add(2 * time.Minute)},
			}
			createManagedUsageSchemaTwelve(t, fixture.database, historicalUsage...)
			if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: testCase.version, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
				t.Fatalf("seed predecessor schema version: %v", createError)
			}

			if migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers); migrationError != nil {
				t.Fatalf("migrate predecessor routes: %v", migrationError)
			}
			var tenantRecord managedTenantRecord
			if queryError := fixture.database.Preload("ProviderConnections").Preload("ProviderProfiles").Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).First(&tenantRecord).Error; queryError != nil {
				t.Fatalf("load migrated predecessor tenant: %v", queryError)
			}
			expectedDefaults := TenantDefaults{
				Provider: ProviderNameXAI, Model: ModelNameGrok43,
				DictationProvider: ProviderNameXAI, DictationModel: "xai-stt",
				SystemPrompt: "preserve tenant prompt",
			}
			if tenantRecord.defaults() != expectedDefaults || len(tenantRecord.ProviderConnections) != 3 || len(tenantRecord.ProviderProfiles) != 3 {
				t.Fatalf("migrated predecessor tenant=%+v connections=%+v profiles=%+v", tenantRecord, tenantRecord.ProviderConnections, tenantRecord.ProviderProfiles)
			}
			modelsByProvider := map[string]string{}
			for _, profile := range tenantRecord.ProviderProfiles {
				modelsByProvider[profile.ProviderID] = profile.TextModel
			}
			if modelsByProvider[ProviderNameXAI] != ModelNameGrok43 || modelsByProvider[ProviderNameMiniMax] != ModelNameMiniMaxM27 || modelsByProvider[ProviderNameSiliconFlow] != ModelNameSiliconFlowDeepSeek {
				t.Fatalf("migrated predecessor provider models=%v", modelsByProvider)
			}
			var retainedUsage []managedUsageEventRecord
			expectedUsage := []managedUsageEventRecord{managedUsageRecordWithoutRoute(historicalUsage[0]), managedUsageRecordWithoutRoute(historicalUsage[1])}
			if queryError := fixture.database.Where("id IN ?", []uint{102, 103}).Order("id").Find(&retainedUsage).Error; queryError != nil || !slices.Equal(retainedUsage, expectedUsage) {
				t.Fatalf("retained predecessor usage=%+v error=%v", retainedUsage, queryError)
			}
			var latest managedSchemaMigrationRecord
			if queryError := fixture.database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedTenantSchemaVersion {
				t.Fatalf("latest predecessor version=%+v error=%v", latest, queryError)
			}
		})
	}
}

func TestManagedXAIProviderMigrationRejectsStageFailures(t *testing.T) {
	assertMigrationError := func(t *testing.T, migrationError error, want string) {
		t.Helper()
		if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), want) {
			t.Fatalf("migration error=%v want=%q", migrationError, want)
		}
	}
	t.Run("encryption", func(t *testing.T) {
		fixture := newManagedXAIProviderMigrationFixture(t)
		_, migrationError := preflightManagedXAIProviderWithReader(fixture.database, fixture.providerKeyCipher, fixture.providers, managedXAIProviderFailingReader{})
		assertMigrationError(t, migrationError, "operation=preflight")
	})
	for _, testCase := range []struct {
		name      string
		want      string
		configure func(*testing.T, managedXAIProviderMigrationFixture)
	}{
		{
			name: "read tenants", want: "operation=read",
			configure: func(t *testing.T, fixture managedXAIProviderMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "xai_provider_read", "query", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "provider conflict", want: "provider_conflict=grok:xai",
			configure: func(t *testing.T, fixture managedXAIProviderMigrationFixture) {
				fixture.addProvider(t, ProviderNameXAI, ModelNameGrok43, "")
			},
		},
		{
			name: "decrypt retired provider", want: "operation=preflight",
			configure: func(t *testing.T, fixture managedXAIProviderMigrationFixture) {
				if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).Where(&managedProviderAPIKeyRecord{TenantID: fixture.tenant.TenantID, ProviderID: retiredGrokProviderIdentifier}).Update("encrypted_api_key", "invalid").Error; updateError != nil {
					t.Fatalf("seed invalid retired provider: %v", updateError)
				}
			},
		},
		{
			name: "decrypt retained provider", want: "operation=preflight",
			configure: func(t *testing.T, fixture managedXAIProviderMigrationFixture) {
				fixture.addProvider(t, ProviderNameOpenAI, ModelNameGPT41, "invalid")
			},
		},
		{
			name: "invalid defaults", want: "operation=preflight table=" + managedTenantTable,
			configure: func(t *testing.T, fixture managedXAIProviderMigrationFixture) {
				if updateError := fixture.database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).Update("default_model", "missing-model").Error; updateError != nil {
					t.Fatalf("seed invalid defaults: %v", updateError)
				}
			},
		},
		{
			name: "read historical usage", want: "operation=read table=" + managedUsageEventTable,
			configure: func(t *testing.T, fixture managedXAIProviderMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "xai_provider_usage_read", "query", managedUsageEventTable, errInternalTestDatabase)
			},
		},
		{
			name: "backfill provider key", want: "operation=backfill table=" + managedProviderKeyTable,
			configure: func(t *testing.T, fixture managedXAIProviderMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "xai_provider_key_backfill", "update", managedProviderKeyTable, errInternalTestDatabase)
			},
		},
		{
			name: "backfill tenant", want: "operation=backfill table=" + managedTenantTable,
			configure: func(t *testing.T, fixture managedXAIProviderMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "xai_provider_tenant_backfill", "update", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "verify", want: "operation=verify",
			configure: func(t *testing.T, fixture managedXAIProviderMigrationFixture) {
				if callbackError := fixture.database.Callback().Query().Before("gorm:query").Register("xai_provider_verify", func(callbackDatabase *gorm.DB) {
					if callbackDatabase.Statement.Table == managedProviderKeyTable {
						if _, isCount := callbackDatabase.Statement.Dest.(*int64); isCount {
							callbackDatabase.AddError(errInternalTestDatabase)
						}
					}
				}); callbackError != nil {
					t.Fatalf("register xAI verification failure: %v", callbackError)
				}
			},
		},
		{
			name: "record version", want: "operation=record_version",
			configure: func(t *testing.T, fixture managedXAIProviderMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "xai_provider_record_version", "create", managedSchemaMigrationTable, errInternalTestDatabase)
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newManagedXAIProviderMigrationFixture(t)
			testCase.configure(t, fixture)
			assertMigrationError(t, migrateManagedXAIProvider(fixture.database, fixture.providerKeyCipher, fixture.providers), testCase.want)
		})
	}

	t.Run("schema five validation", func(t *testing.T) {
		fixture := newManagedXAIProviderMigrationFixture(t)
		if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedModelIdentitySchemaVersion, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
			t.Fatalf("seed schema version: %v", createError)
		}
		if dropError := fixture.database.Migrator().DropIndex(&managedUsageEventSchemaTwelveRecord{}, managedUsageLegacyFailurePageIndex); dropError != nil {
			t.Fatalf("drop usage failure index: %v", dropError)
		}
		assertMigrationError(t, initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers), "operation=validate_current_schema")
	})
}

func TestManagedXAIProviderVerificationRejectsDrift(t *testing.T) {
	assertVerificationError := func(t *testing.T, verificationError error, want string) {
		t.Helper()
		if !errors.Is(verificationError, errManagedTenantSchemaMigration) || !strings.Contains(verificationError.Error(), want) {
			t.Fatalf("verification error=%v want=%q", verificationError, want)
		}
	}
	verifiedFixture := func(t *testing.T) (managedXAIProviderMigrationFixture, managedXAIProviderDataset) {
		t.Helper()
		fixture := newManagedXAIProviderMigrationFixture(t)
		dataset, preflightError := preflightManagedXAIProvider(fixture.database, fixture.providerKeyCipher, fixture.providers)
		if preflightError != nil {
			t.Fatalf("preflight xAI verification fixture: %v", preflightError)
		}
		applyManagedXAIProviderDataset(t, fixture, dataset)
		return fixture, dataset
	}

	t.Run("retired provider remains", func(t *testing.T) {
		fixture := newManagedXAIProviderMigrationFixture(t)
		assertVerificationError(t, verifyManagedXAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, managedXAIProviderDataset{}), "operation=verify")
	})
	t.Run("provider query", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		if callbackError := fixture.database.Callback().Query().Before("gorm:query").Register("xai_verify_provider_query", func(callbackDatabase *gorm.DB) {
			if callbackDatabase.Statement.Table == managedProviderKeyTable {
				if _, isRecord := callbackDatabase.Statement.Dest.(*managedProviderAPIKeyRecord); isRecord {
					callbackDatabase.AddError(errInternalTestDatabase)
				}
			}
		}); callbackError != nil {
			t.Fatalf("register provider query failure: %v", callbackError)
		}
		assertVerificationError(t, verifyManagedXAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=verify")
	})
	t.Run("provider values", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		dataset.providerKeys[0].apiKey = "changed"
		assertVerificationError(t, verifyManagedXAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=verify")
	})
	t.Run("tenant query", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		registerManagedGORMError(t, fixture.database, "xai_verify_tenant_query", "query", managedTenantTable, errInternalTestDatabase)
		assertVerificationError(t, verifyManagedXAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=verify")
	})
	t.Run("tenant values", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		dataset.tenants[0].updatedAt = dataset.tenants[0].updatedAt.Add(time.Second)
		assertVerificationError(t, verifyManagedXAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "values")
	})
	t.Run("current routing validation", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		fixture.addProvider(t, ProviderNameOpenAI, ModelNameGPT41, "invalid")
		assertVerificationError(t, verifyManagedXAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=validate")
	})
	t.Run("usage query", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		registerManagedGORMError(t, fixture.database, "xai_verify_usage_query", "query", managedUsageEventTable, errInternalTestDatabase)
		assertVerificationError(t, verifyManagedXAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=read")
	})
	t.Run("usage values", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		dataset.historicalUsage = nil
		assertVerificationError(t, verifyManagedXAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "historical_usage_changed")
	})
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
	useManagedUsageSchemaTwelve(t, database)
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
		StatusCode: http.StatusOK, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess, CreatedAt: now,
	}
	createManagedUsageSchemaTwelve(t, database, usage)
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
			name: "nonretired defaults without key", want: "operation=preflight",
			configure: func(t *testing.T, fixture managedQwenCloudRetirementFixture) {
				if updateError := fixture.database.Model(&managedTenantRecord{}).
					Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).
					Updates(map[string]any{
						"default_provider":         ProviderNameXAI,
						"default_model":            ModelNameGrok43,
						"default_reasoning_effort": "",
					}).Error; updateError != nil {
					t.Fatalf("seed nonretired defaults without key: %v", updateError)
				}
			},
		},
		{
			name: "predecessor provider conflict", want: "provider_conflict=" + ProviderNameXAI,
			configure: func(t *testing.T, fixture managedQwenCloudRetirementFixture) {
				fixture.addProvider(t, retiredGrokProviderIdentifier, ModelNameGrok43, "")
				fixture.addProvider(t, ProviderNameXAI, ModelNameGrok43, "")
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
		useManagedUsageSchemaTwelve(t, database)
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

func TestManagedUsageDispositionMigrationRejectsInvalidHistoryAndStageFailures(t *testing.T) {
	type migrationCase struct {
		name      string
		stage     string
		want      string
		record    managedUsageEventSchemaTwelveRecord
		configure func(*testing.T, *gorm.DB)
	}
	validRecord := managedUsageEventSchemaTwelveRecord{
		ID: 1, Endpoint: usageEndpointText, ProviderID: ProviderNameOpenAI, ModelID: ModelNameGPT41,
		StatusCode: http.StatusOK, Success: true, OutcomeCode: managedUsageOutcomeSuccess,
		CreatedAt: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC),
	}
	testCases := []migrationCase{
		{
			name: "read dispositions", want: "operation=read_dispositions", record: validRecord,
			configure: func(t *testing.T, database *gorm.DB) {
				registerManagedGORMError(t, database, "usage_disposition_read", "query", managedUsageEventTable, errInternalTestDatabase)
			},
		},
		{name: "unknown outcome", want: "operation=map_disposition", record: managedUsageEventSchemaTwelveRecord{ID: 1, OutcomeCode: "unknown", CreatedAt: validRecord.CreatedAt}},
		{name: "successful non-success outcome", want: "success=true", record: managedUsageEventSchemaTwelveRecord{ID: 1, Success: true, OutcomeCode: managedUsageOutcomeInvalidRequest, CreatedAt: validRecord.CreatedAt}},
		{name: "failed success outcome", want: "success=false", record: managedUsageEventSchemaTwelveRecord{ID: 1, OutcomeCode: managedUsageOutcomeSuccess, CreatedAt: validRecord.CreatedAt}},
		{name: "rename predecessor", stage: "rename_disposition_predecessor", want: "operation=rename_predecessor", record: validRecord},
		{name: "drop predecessor index", stage: "drop_disposition_predecessor_index", want: "operation=drop_predecessor_index", record: validRecord},
		{name: "create current table", stage: "create_disposition_table", want: "operation=create_current", record: validRecord},
		{name: "create current constraint", stage: "create_disposition_constraint", want: "operation=create_constraint", record: validRecord},
		{name: "create current index", stage: "create_disposition_index", want: "operation=create_current_index", record: validRecord},
		{
			name: "copy rows", want: "operation=copy_rows", record: validRecord,
			configure: func(t *testing.T, database *gorm.DB) {
				registerManagedGORMError(t, database, "usage_disposition_copy", "create", managedUsageEventTable, errInternalTestDatabase)
			},
		},
		{name: "drop predecessor", stage: "drop_disposition_predecessor", want: "operation=drop_predecessor", record: validRecord},
		{
			name: "record version", want: "operation=record_version", record: validRecord,
			configure: func(t *testing.T, database *gorm.DB) {
				registerManagedGORMError(t, database, "usage_disposition_version", "create", managedSchemaMigrationTable, errInternalTestDatabase)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			activeStage := ""
			database := newManagedUsageDispositionSchemaTwelveFixture(t, &activeStage, testCase.record)
			if testCase.configure != nil {
				testCase.configure(t, database)
			}
			activeStage = testCase.stage
			migrationError := migrateManagedUsageDispositionSchema(database)
			if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), testCase.want) {
				t.Fatalf("migration error=%v want=%q", migrationError, testCase.want)
			}
			if !managedTableHasColumn(database.Migrator(), managedUsageEventTable, "success") {
				t.Fatal("failed migration did not restore schema twelve")
			}
		})
	}

	t.Run("existing current index", func(t *testing.T) {
		activeStage := ""
		database := newManagedUsageDispositionSchemaTwelveFixture(t, &activeStage, validRecord)
		activeStage = "existing_disposition_index"
		if migrationError := migrateManagedUsageDispositionSchema(database); migrationError != nil {
			t.Fatalf("migrate with existing index: %v", migrationError)
		}
	})

	t.Run("large history", func(t *testing.T) {
		const recordCount = 2600
		records := make([]managedUsageEventSchemaTwelveRecord, 0, recordCount)
		for recordIndex := 1; recordIndex <= recordCount; recordIndex++ {
			record := validRecord
			record.ID = uint(recordIndex)
			record.CreatedAt = validRecord.CreatedAt.Add(time.Duration(recordIndex) * time.Second)
			records = append(records, record)
		}
		database := newManagedUsageDispositionSchemaTwelveDatabase(
			t,
			sqlite.Open(filepath.Join(t.TempDir(), "usage-disposition-large-history.db")),
			records...,
		)
		if migrationError := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry()); migrationError != nil {
			t.Fatalf("initialize large history: %v", migrationError)
		}
		var migratedCount int64
		if countError := database.Model(&managedUsageEventRecord{}).Count(&migratedCount).Error; countError != nil || migratedCount != recordCount {
			t.Fatalf("migrated count=%d error=%v", migratedCount, countError)
		}
		var lastRecord managedUsageEventRecord
		if queryError := database.Last(&lastRecord).Error; queryError != nil {
			t.Fatalf("read last migrated record: %v", queryError)
		}
		if lastRecord.ID != recordCount || lastRecord.Disposition != managedUsageDispositionSucceeded || lastRecord.OutcomeCode != managedUsageOutcomeSuccess {
			t.Fatalf("last migrated record=%+v", lastRecord)
		}
	})

	t.Run("invalid request server error becomes proxy failure", func(t *testing.T) {
		activeStage := ""
		record := validRecord
		record.Success = false
		record.StatusCode = http.StatusInternalServerError
		record.OutcomeCode = managedUsageOutcomeInvalidRequest
		database := newManagedUsageDispositionSchemaTwelveFixture(t, &activeStage, record)
		if migrationError := migrateManagedUsageDispositionSchema(database); migrationError != nil {
			t.Fatalf("migrate proxy failure: %v", migrationError)
		}
		var migrated managedUsageEventRecord
		if queryError := database.First(&migrated, record.ID).Error; queryError != nil {
			t.Fatalf("read migrated proxy failure: %v", queryError)
		}
		if migrated.Disposition != managedUsageDispositionFailed || migrated.OutcomeCode != managedUsageOutcomeProxyError {
			t.Fatalf("migrated proxy failure=%+v", migrated)
		}
	})
}

func TestManagedUsageDispositionSchemaValidationRejectsInvalidStructuresAndRows(t *testing.T) {
	now := time.Date(2026, 9, 3, 13, 0, 0, 0, time.UTC)
	newCurrentDatabase := func(t *testing.T) *gorm.DB {
		database, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "usage-disposition-current.db")), &gorm.Config{})
		if openError != nil {
			t.Fatalf("open current fixture: %v", openError)
		}
		if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
			t.Fatalf("create current fixture: %v", migrationError)
		}
		return database
	}
	insertInvalidRecord := func(t *testing.T, database *gorm.DB, disposition string, outcome string) {
		if insertError := database.Exec(
			"INSERT INTO "+managedUsageEventTable+" (tenant_id, disposition, outcome_code, created_at) VALUES (?, ?, ?, ?)",
			"validation-tenant", disposition, outcome, now,
		).Error; insertError != nil {
			t.Fatalf("insert invalid usage record: %v", insertError)
		}
	}

	t.Run("missing disposition column", func(t *testing.T) {
		activeStage := ""
		database := newManagedUsageDispositionSchemaTwelveFixture(t, &activeStage, managedUsageEventSchemaTwelveRecord{})
		if validationError := validateManagedUsageDispositionSchema(database); !errors.Is(validationError, errManagedTenantSchemaMigration) || !strings.Contains(validationError.Error(), "missing_column") {
			t.Fatalf("validation error=%v", validationError)
		}
	})
	t.Run("obsolete success column", func(t *testing.T) {
		activeStage := ""
		database := newManagedUsageDispositionSchemaTwelveFixture(t, &activeStage, managedUsageEventSchemaTwelveRecord{})
		if alterError := database.Exec("ALTER TABLE " + managedUsageEventTable + " ADD COLUMN disposition TEXT NOT NULL DEFAULT 'succeeded'").Error; alterError != nil {
			t.Fatalf("add disposition column: %v", alterError)
		}
		if validationError := validateManagedUsageDispositionSchema(database); !errors.Is(validationError, errManagedTenantSchemaMigration) || !strings.Contains(validationError.Error(), "obsolete_column=success") {
			t.Fatalf("validation error=%v", validationError)
		}
	})
	t.Run("missing disposition index", func(t *testing.T) {
		database := newCurrentDatabase(t)
		if dropError := database.Migrator().DropIndex(&managedUsageEventRecord{}, managedUsageDispositionPageIndex); dropError != nil {
			t.Fatalf("drop disposition index: %v", dropError)
		}
		if validationError := validateManagedUsageDispositionSchema(database); !errors.Is(validationError, errManagedTenantSchemaMigration) || !strings.Contains(validationError.Error(), "missing_index") {
			t.Fatalf("validation error=%v", validationError)
		}
	})
	t.Run("predecessor table remains", func(t *testing.T) {
		database := newCurrentDatabase(t)
		if createError := database.Exec("CREATE TABLE " + usageDispositionMigrationTable + " (id INTEGER PRIMARY KEY)").Error; createError != nil {
			t.Fatalf("create predecessor table: %v", createError)
		}
		if validationError := validateManagedUsageDispositionSchema(database); !errors.Is(validationError, errManagedTenantSchemaMigration) || !strings.Contains(validationError.Error(), "predecessor_table") {
			t.Fatalf("validation error=%v", validationError)
		}
	})
	t.Run("record query", func(t *testing.T) {
		database := newCurrentDatabase(t)
		registerManagedGORMError(t, database, "usage_disposition_validate_query", "query", managedUsageEventTable, errInternalTestDatabase)
		if validationError := validateManagedUsageDispositionSchema(database); !errors.Is(validationError, errManagedTenantSchemaMigration) || !strings.Contains(validationError.Error(), "operation=validate_dispositions") {
			t.Fatalf("validation error=%v", validationError)
		}
	})
	t.Run("invalid disposition", func(t *testing.T) {
		database := newCurrentDatabase(t)
		insertInvalidRecord(t, database, "unknown", string(managedUsageOutcomeSuccess))
		if validationError := validateManagedUsageDispositionSchema(database); !errors.Is(validationError, errManagedTenantSchemaMigration) || !strings.Contains(validationError.Error(), "disposition=\"unknown\"") {
			t.Fatalf("validation error=%v", validationError)
		}
	})
	t.Run("invalid outcome", func(t *testing.T) {
		database := newCurrentDatabase(t)
		insertInvalidRecord(t, database, string(managedUsageDispositionFailed), "unknown")
		if validationError := validateManagedUsageDispositionSchema(database); !errors.Is(validationError, errManagedTenantSchemaMigration) || !strings.Contains(validationError.Error(), "outcome_code=\"unknown\"") {
			t.Fatalf("validation error=%v", validationError)
		}
	})
	t.Run("mismatched outcome", func(t *testing.T) {
		database := newCurrentDatabase(t)
		insertInvalidRecord(t, database, string(managedUsageDispositionFailed), string(managedUsageOutcomeSuccess))
		if validationError := validateManagedUsageDispositionSchema(database); !errors.Is(validationError, errManagedTenantSchemaMigration) || !strings.Contains(validationError.Error(), "outcome_code=\"success\"") {
			t.Fatalf("validation error=%v", validationError)
		}
	})
	t.Run("repeated valid pairs", func(t *testing.T) {
		database := newCurrentDatabase(t)
		records := make([]managedUsageEventRecord, 0, 2600)
		for recordIndex := 1; recordIndex <= 2600; recordIndex++ {
			records = append(records, managedUsageEventRecord{
				ID:          uint(recordIndex),
				TenantID:    "validation-tenant",
				Disposition: managedUsageDispositionFailed,
				OutcomeCode: managedUsageOutcomeProxyError,
				CreatedAt:   now.Add(time.Duration(recordIndex) * time.Second),
			})
		}
		if createError := database.CreateInBatches(&records, managedUsageMigrationBatchSize).Error; createError != nil {
			t.Fatalf("insert repeated usage pairs: %v", createError)
		}
		if validationError := validateManagedUsageDispositionSchema(database); validationError != nil {
			t.Fatalf("validate repeated usage pairs: %v", validationError)
		}
	})
}

func TestManagedUsageDispositionInitializationValidatesVersionTwelvePrerequisites(t *testing.T) {
	newFixture := func(t *testing.T) *gorm.DB {
		activeStage := ""
		return newManagedUsageDispositionSchemaTwelveFixture(t, &activeStage, managedUsageEventSchemaTwelveRecord{})
	}
	t.Run("migrates valid schema", func(t *testing.T) {
		database := newFixture(t)
		if initializeError := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry()); initializeError != nil {
			t.Fatalf("initialize schema twelve: %v", initializeError)
		}
	})
	t.Run("provider tables", func(t *testing.T) {
		database := newFixture(t)
		if dropError := database.Migrator().DropTable(&managedProviderProfileRecord{}); dropError != nil {
			t.Fatalf("drop provider profiles: %v", dropError)
		}
		if initializeError := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry()); !errors.Is(initializeError, errManagedTenantSchemaMigration) || !strings.Contains(initializeError.Error(), "validate_current_schema") {
			t.Fatalf("initialize error=%v", initializeError)
		}
	})
	t.Run("routing defaults", func(t *testing.T) {
		database := newFixture(t)
		registerManagedGORMError(t, database, "usage_disposition_connection_validation", "query", managedTenantTable, errInternalTestDatabase)
		if initializeError := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry()); !errors.Is(initializeError, errManagedTenantSchemaMigration) || !strings.Contains(initializeError.Error(), "operation=read") {
			t.Fatalf("initialize error=%v", initializeError)
		}
	})
	t.Run("resolved routes", func(t *testing.T) {
		database := newFixture(t)
		registerManagedGORMError(t, database, "usage_disposition_route_validation", "query", managedUsageEventTable, errInternalTestDatabase)
		if initializeError := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry()); !errors.Is(initializeError, errManagedTenantSchemaMigration) || !strings.Contains(initializeError.Error(), "operation=validate_routes") {
			t.Fatalf("initialize error=%v", initializeError)
		}
	})
}

func newManagedUsageDispositionSchemaTwelveFixture(t *testing.T, activeStage *string, records ...managedUsageEventSchemaTwelveRecord) *gorm.DB {
	t.Helper()
	constraintCreated := false
	dialector := failingManagedUsageMigrationDialector{
		Dialector:         sqlite.Open(filepath.Join(t.TempDir(), "usage-disposition-schema-twelve.db")),
		stage:             activeStage,
		constraintCreated: &constraintCreated,
	}
	database := newManagedUsageDispositionSchemaTwelveDatabase(t, dialector, records...)
	constraintCreated = false
	return database
}

func newManagedUsageDispositionSchemaTwelveDatabase(t *testing.T, dialector gorm.Dialector, records ...managedUsageEventSchemaTwelveRecord) *gorm.DB {
	t.Helper()
	database, openError := gorm.Open(dialector, &gorm.Config{})
	if openError != nil {
		t.Fatalf("open schema-twelve fixture: %v", openError)
	}
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create supporting schema: %v", migrationError)
	}
	if dropError := database.Migrator().DropTable(&managedProviderAPIKeyRecord{}); dropError != nil {
		t.Fatalf("drop predecessor provider table: %v", dropError)
	}
	useManagedUsageSchemaTwelve(t, database)
	now := time.Date(2026, 9, 3, 11, 0, 0, 0, time.UTC)
	user := managedUserRecord{UserID: "usage-disposition-owner", CreatedAt: now, UpdatedAt: now}
	tenantRecord := fakeTenantRecord(user.UserID, "usage-disposition-tenant", "Disposition", now)
	if createError := database.Create(&user).Error; createError != nil {
		t.Fatalf("seed user: %v", createError)
	}
	if createError := database.Create(&tenantRecord).Error; createError != nil {
		t.Fatalf("seed tenant: %v", createError)
	}
	seedRecords := make([]managedUsageEventSchemaTwelveRecord, 0, len(records))
	for _, record := range records {
		if record.ID == 0 {
			continue
		}
		record.TenantID = tenantRecord.TenantID
		seedRecords = append(seedRecords, record)
	}
	if len(seedRecords) != 0 {
		if createError := database.CreateInBatches(&seedRecords, managedUsageMigrationBatchSize).Error; createError != nil {
			t.Fatalf("seed usage records: %v", createError)
		}
	}
	if createError := database.Create(&managedSchemaMigrationRecord{Version: managedResolvedUsageRouteSchemaVersion, AppliedAt: now}).Error; createError != nil {
		t.Fatalf("seed schema version: %v", createError)
	}
	return database
}

type failingManagedUsageMigrationDialector struct {
	gorm.Dialector
	stage             *string
	constraintCreated *bool
}

func (dialector failingManagedUsageMigrationDialector) Migrator(database *gorm.DB) gorm.Migrator {
	sqliteMigrator, validMigrator := dialector.Dialector.Migrator(database).(sqlite.Migrator)
	if !validMigrator {
		panic("expected SQLite migrator")
	}
	return failingManagedUsageMigrationMigrator{
		Migrator:          sqliteMigrator,
		stage:             dialector.stage,
		constraintCreated: dialector.constraintCreated,
	}
}

type failingManagedUsageMigrationMigrator struct {
	sqlite.Migrator
	stage             *string
	constraintCreated *bool
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
	if *migrator.stage == "create_disposition_index" && *migrator.constraintCreated && name == managedUsageDispositionPageIndex {
		return errInternalTestDatabase
	}
	return migrator.Migrator.CreateIndex(value, name)
}

func (migrator failingManagedUsageMigrationMigrator) RenameTable(oldName interface{}, newName interface{}) error {
	if *migrator.stage == "rename_disposition_predecessor" {
		return errInternalTestDatabase
	}
	return migrator.Migrator.RenameTable(oldName, newName)
}

func (migrator failingManagedUsageMigrationMigrator) DropIndex(value interface{}, name string) error {
	if *migrator.stage == "drop_disposition_predecessor_index" {
		return errInternalTestDatabase
	}
	return migrator.Migrator.DropIndex(value, name)
}

func (migrator failingManagedUsageMigrationMigrator) CreateTable(values ...interface{}) error {
	if *migrator.stage == "create_disposition_table" {
		return errInternalTestDatabase
	}
	return migrator.Migrator.CreateTable(values...)
}

func (migrator failingManagedUsageMigrationMigrator) CreateConstraint(value interface{}, name string) error {
	if *migrator.stage == "create_disposition_constraint" {
		return errInternalTestDatabase
	}
	for _, indexName := range []string{managedUsageTenantCreatedIndex, legacyUsageCreatedAtIndex, managedUsageDispositionPageIndex} {
		if migrator.Migrator.HasIndex(&managedUsageEventRecord{}, indexName) {
			if dropError := migrator.Migrator.DropIndex(&managedUsageEventRecord{}, indexName); dropError != nil {
				return dropError
			}
		}
	}
	*migrator.constraintCreated = true
	return nil
}

func (migrator failingManagedUsageMigrationMigrator) HasIndex(value interface{}, name string) bool {
	if *migrator.stage == "existing_disposition_index" && name == managedUsageDispositionPageIndex {
		return true
	}
	return migrator.Migrator.HasIndex(value, name)
}

func (migrator failingManagedUsageMigrationMigrator) DropTable(values ...interface{}) error {
	if *migrator.stage == "drop_disposition_predecessor" {
		return errInternalTestDatabase
	}
	return migrator.Migrator.DropTable(values...)
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
	actualDispositions := make([]managedUsageDisposition, 0, len(records))
	for _, record := range records {
		actualOutcomes = append(actualOutcomes, record.OutcomeCode)
		actualDispositions = append(actualDispositions, record.Disposition)
	}
	expectedOutcomes := []managedUsageOutcomeCode{
		managedUsageOutcomeSuccess,
		managedUsageOutcomeInvalidRequest,
		managedUsageOutcomePayloadTooLarge,
		managedUsageOutcomeRateLimited,
		managedUsageOutcomeProviderNotConfigured,
		managedUsageOutcomeRequestTimeout,
		managedUsageOutcomeUpstreamError,
		managedUsageOutcomeRequestTimeout,
	}
	if !reflect.DeepEqual(actualOutcomes, expectedOutcomes) {
		t.Fatalf("outcomes=%v want=%v", actualOutcomes, expectedOutcomes)
	}
	expectedDispositions := []managedUsageDisposition{
		managedUsageDispositionSucceeded,
		managedUsageDispositionRejected,
		managedUsageDispositionRejected,
		managedUsageDispositionFailed,
		managedUsageDispositionRejected,
		managedUsageDispositionFailed,
		managedUsageDispositionFailed,
		managedUsageDispositionFailed,
	}
	if !reflect.DeepEqual(actualDispositions, expectedDispositions) {
		t.Fatalf("dispositions=%v want=%v", actualDispositions, expectedDispositions)
	}
	for _, indexName := range []string{managedUsageTenantCreatedIndex, legacyUsageCreatedAtIndex, managedUsageDispositionPageIndex} {
		if !database.Migrator().HasIndex(&managedUsageEventRecord{}, indexName) {
			t.Fatalf("missing index %s", indexName)
		}
	}
	if database.Migrator().HasIndex(managedUsageEventTable, managedUsageLegacyFailurePageIndex) {
		t.Fatalf("obsolete index remains: %s", managedUsageLegacyFailurePageIndex)
	}
	type usageForeignKey struct {
		Table    string
		From     string
		To       string
		OnUpdate string `gorm:"column:on_update"`
		OnDelete string `gorm:"column:on_delete"`
	}
	var foreignKeys []usageForeignKey
	if queryError := database.Raw("PRAGMA foreign_key_list(" + managedUsageEventTable + ")").Scan(&foreignKeys).Error; queryError != nil {
		t.Fatalf("read usage foreign keys: %v", queryError)
	}
	expectedForeignKeys := []usageForeignKey{{Table: managedTenantTable, From: "tenant_id", To: "tenant_id", OnUpdate: "CASCADE", OnDelete: "CASCADE"}}
	if !reflect.DeepEqual(foreignKeys, expectedForeignKeys) {
		t.Fatalf("usage foreign keys=%+v want=%+v", foreignKeys, expectedForeignKeys)
	}
	if managedTableHasColumn(database.Migrator(), managedUsageEventTable, "success") {
		t.Fatal("current usage schema retained success")
	}
	if insertError := database.Exec(
		"INSERT INTO "+managedUsageEventTable+" (tenant_id, disposition, outcome_code, created_at) VALUES (?, ?, NULL, ?)",
		"usage-migration-tenant",
		managedUsageDispositionFailed,
		time.Now().UTC(),
	).Error; insertError == nil {
		t.Fatal("outcome_code accepted NULL")
	}
	var latest managedSchemaMigrationRecord
	if queryError := database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedTenantSchemaVersion {
		t.Fatalf("latest version=%+v error=%v", latest, queryError)
	}
}
