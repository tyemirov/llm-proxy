package proxy

import (
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type managedDashScopeSettingsMigrationFixture struct {
	database          *gorm.DB
	providerKeyCipher managedProviderKeyCipher
	providers         *providerRegistry
	tenant            managedTenantRecord
	usage             managedUsageEventRecord
}

func newManagedDashScopeSettingsMigrationFixture(t *testing.T) managedDashScopeSettingsMigrationFixture {
	t.Helper()
	return newManagedDashScopeSettingsMigrationFixtureWithDialector(t, sqlite.Open(filepath.Join(t.TempDir(), "dashscope-settings.db")))
}

func newManagedDashScopeSettingsMigrationFixtureWithDialector(t *testing.T, dialector gorm.Dialector) managedDashScopeSettingsMigrationFixture {
	t.Helper()
	database, openError := gorm.Open(dialector, &gorm.Config{})
	if openError != nil {
		t.Fatalf("open DashScope settings fixture: %v", openError)
	}
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create DashScope settings schema: %v", migrationError)
	}
	now := time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
	user := managedUserRecord{UserID: "dashscope-settings-owner", CreatedAt: now, UpdatedAt: now}
	if createError := database.Create(&user).Error; createError != nil {
		t.Fatalf("seed DashScope settings user: %v", createError)
	}
	tenantRecord := fakeTenantRecord(user.UserID, "dashscope-settings-tenant", "Default", now)
	tenantRecord.DefaultProvider = ProviderNameDashScope
	tenantRecord.DefaultModel = ModelNameDashScopeQwenPlus
	tenantRecord.DefaultSystemPrompt = "preserve DashScope tenant prompt"
	if createError := database.Create(&tenantRecord).Error; createError != nil {
		t.Fatalf("seed DashScope settings tenant: %v", createError)
	}
	providerKeyCipher := internalManagedProviderKeyCipher()
	encryptProviderKey := func(providerIdentifier string, rawAPIKey string, nonceCharacter string) string {
		t.Helper()
		encryptedKey, encryptionError := providerKeyCipher.encrypt(
			strings.NewReader(strings.Repeat(nonceCharacter, providerKeyCipher.aeadCipher.NonceSize())),
			tenantRecord.TenantID,
			providerIdentifier,
			rawAPIKey,
		)
		if encryptionError != nil {
			t.Fatalf("encrypt provider=%s: %v", providerIdentifier, encryptionError)
		}
		return encryptedKey
	}
	providerKeys := []managedProviderAPIKeyRecord{
		{
			TenantID: tenantRecord.TenantID, ProviderID: ProviderNameDashScope,
			EncryptedAPIKey: encryptProviderKey(ProviderNameDashScope, "sk-dashscope", "d"),
			TextModel:       ModelNameDashScopeQwenPlus, SystemPrompt: "incomplete workspace settings",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			TenantID: tenantRecord.TenantID, ProviderID: ProviderNameDeepSeek,
			EncryptedAPIKey: encryptProviderKey(ProviderNameDeepSeek, "sk-deepseek", "e"),
			TextModel:       ModelNameDeepSeekV4Flash, SystemPrompt: "retained settings",
			CreatedAt: now, UpdatedAt: now,
		},
	}
	if createError := database.Create(&providerKeys).Error; createError != nil {
		t.Fatalf("seed DashScope settings provider keys: %v", createError)
	}
	usage := managedUsageEventRecord{
		ID: 111, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointText,
		ProviderID: ProviderNameDashScope, ModelID: ModelNameDashScopeQwenPlus,
		StatusCode: http.StatusOK, Success: true, OutcomeCode: managedUsageOutcomeSuccess,
		TotalTokens: 13, CreatedAt: now.Add(time.Minute),
	}
	if createError := database.Create(&usage).Error; createError != nil {
		t.Fatalf("seed DashScope settings usage: %v", createError)
	}
	return managedDashScopeSettingsMigrationFixture{
		database: database, providerKeyCipher: providerKeyCipher,
		providers: internalManagementProviderRegistry(), tenant: tenantRecord, usage: usage,
	}
}

func (fixture managedDashScopeSettingsMigrationFixture) dropBaseURLColumn(t *testing.T) {
	t.Helper()
	if dropError := fixture.database.Migrator().DropColumn(&managedProviderAPIKeyRecord{}, "BaseURL"); dropError != nil {
		t.Fatalf("drop provider base URL column: %v", dropError)
	}
}

func applyManagedDashScopeSettingsDataset(t *testing.T, fixture managedDashScopeSettingsMigrationFixture, dataset managedDashScopeSettingsMigrationDataset) {
	t.Helper()
	if deleteError := fixture.database.Where(&managedProviderAPIKeyRecord{ProviderID: ProviderNameDashScope}).Delete(&managedProviderAPIKeyRecord{}).Error; deleteError != nil {
		t.Fatalf("delete incomplete DashScope settings: %v", deleteError)
	}
	for _, backfill := range dataset.backfills {
		if updateError := fixture.database.Model(&managedTenantRecord{}).
			Where(&managedTenantRecord{TenantID: backfill.tenantID}).
			Updates(managedRoutingDefaultsDatabaseUpdates(backfill.defaults, backfill.updatedAt)).Error; updateError != nil {
			t.Fatalf("apply DashScope defaults backfill: %v", updateError)
		}
	}
}

func assertManagedDashScopeMigrationError(t *testing.T, migrationError error, want string) {
	t.Helper()
	if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), want) {
		t.Fatalf("migration error=%v want=%q", migrationError, want)
	}
}

func TestManagedDashScopeSettingsMigrationRemovesIncompleteSettingsAndPreservesUsage(t *testing.T) {
	fixture := newManagedDashScopeSettingsMigrationFixture(t)
	fixture.dropBaseURLColumn(t)
	if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedXAIProviderSchemaVersion, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
		t.Fatalf("seed schema version: %v", createError)
	}
	if migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers); migrationError != nil {
		t.Fatalf("migrate DashScope settings: %v", migrationError)
	}
	if fixture.database.Migrator().HasTable(managedProviderKeyTable) {
		t.Fatal("predecessor provider table remains after connection migration")
	}
	var tenantRecord managedTenantRecord
	if queryError := fixture.database.Preload("ProviderConnections").Preload("ProviderProfiles").Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).First(&tenantRecord).Error; queryError != nil {
		t.Fatalf("load migrated DashScope tenant: %v", queryError)
	}
	expectedDefaults := TenantDefaults{
		Provider: ProviderNameDeepSeek, Model: ModelNameDeepSeekV4Flash,
		SystemPrompt: fixture.tenant.DefaultSystemPrompt,
	}
	if tenantRecord.defaults() != expectedDefaults || !tenantRecord.UpdatedAt.Equal(fixture.tenant.UpdatedAt) || len(tenantRecord.ProviderConnections) != 1 || len(tenantRecord.ProviderProfiles) != 1 || tenantRecord.ProviderConnections[0].ProviderID != ProviderNameDeepSeek || tenantRecord.ProviderProfiles[0].ProviderID != ProviderNameDeepSeek {
		t.Fatalf("migrated DashScope tenant=%+v connections=%+v profiles=%+v", tenantRecord, tenantRecord.ProviderConnections, tenantRecord.ProviderProfiles)
	}
	var historicalUsage []managedUsageEventRecord
	if queryError := fixture.database.Where(&managedUsageEventRecord{ProviderID: ProviderNameDashScope}).Order("id").Find(&historicalUsage).Error; queryError != nil || !slices.Equal(historicalUsage, []managedUsageEventRecord{fixture.usage}) {
		t.Fatalf("historical DashScope usage=%+v error=%v", historicalUsage, queryError)
	}
	var latest managedSchemaMigrationRecord
	if queryError := fixture.database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedTenantSchemaVersion {
		t.Fatalf("latest version=%+v error=%v", latest, queryError)
	}
	if validationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers); validationError != nil {
		t.Fatalf("reopen DashScope settings schema: %v", validationError)
	}
}

func TestManagedDashScopeSettingsMigrationRejectsStageFailures(t *testing.T) {
	t.Run("add column", func(t *testing.T) {
		activeStage := ""
		fixture := newManagedDashScopeSettingsMigrationFixtureWithDialector(t, failingManagedUsageMigrationDialector{
			Dialector: sqlite.Open(filepath.Join(t.TempDir(), "dashscope-add-column.db")),
			stage:     &activeStage,
		})
		fixture.dropBaseURLColumn(t)
		activeStage = "add_column"
		assertManagedDashScopeMigrationError(t, migrateManagedDashScopeSettings(fixture.database, fixture.providerKeyCipher, fixture.providers), "operation=add_column")
	})
	for _, testCase := range []struct {
		name      string
		want      string
		configure func(*testing.T, managedDashScopeSettingsMigrationFixture)
	}{
		{
			name: "read tenants", want: "operation=read",
			configure: func(t *testing.T, fixture managedDashScopeSettingsMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "dashscope_settings_read", "query", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "decrypt remaining provider", want: "operation=preflight table=" + managedProviderKeyTable,
			configure: func(t *testing.T, fixture managedDashScopeSettingsMigrationFixture) {
				if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).Where(&managedProviderAPIKeyRecord{TenantID: fixture.tenant.TenantID, ProviderID: ProviderNameDeepSeek}).Update("encrypted_api_key", "invalid").Error; updateError != nil {
					t.Fatalf("seed invalid retained provider: %v", updateError)
				}
			},
		},
		{
			name: "invalid defaults", want: "operation=preflight table=" + managedTenantTable,
			configure: func(t *testing.T, fixture managedDashScopeSettingsMigrationFixture) {
				if updateError := fixture.database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).Update("default_model", "missing-model").Error; updateError != nil {
					t.Fatalf("seed invalid defaults: %v", updateError)
				}
			},
		},
		{
			name: "non-DashScope defaults without key", want: "operation=preflight table=" + managedTenantTable,
			configure: func(t *testing.T, fixture managedDashScopeSettingsMigrationFixture) {
				if updateError := fixture.database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).Updates(map[string]any{"default_provider": ProviderNameOpenAI, "default_model": ModelNameGPT41}).Error; updateError != nil {
					t.Fatalf("seed unkeyed defaults: %v", updateError)
				}
			},
		},
		{
			name: "reconciliation", want: "operation=preflight table=" + managedTenantTable,
			configure: func(t *testing.T, fixture managedDashScopeSettingsMigrationFixture) {
				if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).Where(&managedProviderAPIKeyRecord{TenantID: fixture.tenant.TenantID, ProviderID: ProviderNameDeepSeek}).Update("text_model", "missing-model").Error; updateError != nil {
					t.Fatalf("seed invalid retained route: %v", updateError)
				}
			},
		},
		{
			name: "read historical usage", want: "operation=read table=" + managedUsageEventTable,
			configure: func(t *testing.T, fixture managedDashScopeSettingsMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "dashscope_settings_usage_read", "query", managedUsageEventTable, errInternalTestDatabase)
			},
		},
		{
			name: "delete incomplete provider", want: "operation=delete_incomplete_provider",
			configure: func(t *testing.T, fixture managedDashScopeSettingsMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "dashscope_settings_delete", "delete", managedProviderKeyTable, errInternalTestDatabase)
			},
		},
		{
			name: "backfill", want: "operation=backfill",
			configure: func(t *testing.T, fixture managedDashScopeSettingsMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "dashscope_settings_backfill", "update", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "verify", want: "operation=verify",
			configure: func(t *testing.T, fixture managedDashScopeSettingsMigrationFixture) {
				if callbackError := fixture.database.Callback().Query().Before("gorm:query").Register("dashscope_settings_verify", func(callbackDatabase *gorm.DB) {
					if callbackDatabase.Statement.Table == managedProviderKeyTable {
						if _, isCount := callbackDatabase.Statement.Dest.(*int64); isCount {
							callbackDatabase.AddError(errInternalTestDatabase)
						}
					}
				}); callbackError != nil {
					t.Fatalf("register DashScope verification failure: %v", callbackError)
				}
			},
		},
		{
			name: "record version", want: "operation=record_version",
			configure: func(t *testing.T, fixture managedDashScopeSettingsMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "dashscope_settings_record_version", "create", managedSchemaMigrationTable, errInternalTestDatabase)
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newManagedDashScopeSettingsMigrationFixture(t)
			testCase.configure(t, fixture)
			assertManagedDashScopeMigrationError(t, migrateManagedDashScopeSettings(fixture.database, fixture.providerKeyCipher, fixture.providers), testCase.want)
		})
	}
}

func TestManagedDashScopeSettingsVerificationRejectsDrift(t *testing.T) {
	verifiedFixture := func(t *testing.T) (managedDashScopeSettingsMigrationFixture, managedDashScopeSettingsMigrationDataset) {
		t.Helper()
		fixture := newManagedDashScopeSettingsMigrationFixture(t)
		dataset, preflightError := preflightManagedDashScopeSettings(fixture.database, fixture.providerKeyCipher, fixture.providers)
		if preflightError != nil {
			t.Fatalf("preflight DashScope verification fixture: %v", preflightError)
		}
		applyManagedDashScopeSettingsDataset(t, fixture, dataset)
		return fixture, dataset
	}

	t.Run("missing base URL column", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		fixture.dropBaseURLColumn(t)
		assertManagedDashScopeMigrationError(t, verifyManagedDashScopeSettingsMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "column="+managedProviderBaseURLColumn)
	})
	t.Run("incomplete provider remains", func(t *testing.T) {
		fixture := newManagedDashScopeSettingsMigrationFixture(t)
		assertManagedDashScopeMigrationError(t, verifyManagedDashScopeSettingsMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, managedDashScopeSettingsMigrationDataset{}), "count=1")
	})
	t.Run("tenant query", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		registerManagedGORMError(t, fixture.database, "dashscope_verify_tenant_query", "query", managedTenantTable, errInternalTestDatabase)
		assertManagedDashScopeMigrationError(t, verifyManagedDashScopeSettingsMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=verify")
	})
	t.Run("tenant values", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		dataset.backfills[0].updatedAt = dataset.backfills[0].updatedAt.Add(time.Second)
		assertManagedDashScopeMigrationError(t, verifyManagedDashScopeSettingsMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "values")
	})
	t.Run("current routing validation", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).Where(&managedProviderAPIKeyRecord{TenantID: fixture.tenant.TenantID, ProviderID: ProviderNameDeepSeek}).Update("encrypted_api_key", "invalid").Error; updateError != nil {
			t.Fatalf("corrupt retained provider: %v", updateError)
		}
		assertManagedDashScopeMigrationError(t, verifyManagedDashScopeSettingsMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=validate")
	})
	t.Run("usage query", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		registerManagedGORMError(t, fixture.database, "dashscope_verify_usage_query", "query", managedUsageEventTable, errInternalTestDatabase)
		assertManagedDashScopeMigrationError(t, verifyManagedDashScopeSettingsMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=read")
	})
	t.Run("usage values", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		dataset.historicalUsage = nil
		assertManagedDashScopeMigrationError(t, verifyManagedDashScopeSettingsMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "historical_usage_changed")
	})
}

func TestManagedDashScopeSettingsSchemaValidationRejectsIncompleteCurrentState(t *testing.T) {
	t.Run("schema six usage index", func(t *testing.T) {
		fixture := newManagedDashScopeSettingsMigrationFixture(t)
		if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedXAIProviderSchemaVersion, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
			t.Fatalf("seed schema version: %v", createError)
		}
		if dropError := fixture.database.Migrator().DropIndex(&managedUsageEventRecord{}, managedUsageFailurePageIndex); dropError != nil {
			t.Fatalf("drop usage index: %v", dropError)
		}
		assertManagedDashScopeMigrationError(t, initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers), "operation=validate_current_schema")
	})
	t.Run("schema seven base URL column", func(t *testing.T) {
		fixture := newManagedDashScopeSettingsMigrationFixture(t)
		fixture.dropBaseURLColumn(t)
		if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedTenantSchemaVersion, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
			t.Fatalf("seed schema version: %v", createError)
		}
		assertManagedDashScopeMigrationError(t, initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers), "operation=validate_current_schema")
	})
	t.Run("schema seven incomplete DashScope setting", func(t *testing.T) {
		fixture := newManagedDashScopeSettingsMigrationFixture(t)
		if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedTenantSchemaVersion, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
			t.Fatalf("seed schema version: %v", createError)
		}
		assertManagedDashScopeMigrationError(t, initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers), "operation=validate")
	})
}

func TestManagedTenantInitializationPropagatesXAIProviderFailureBeforeDashScopeMigration(t *testing.T) {
	fixture := newManagedXAIProviderMigrationFixture(t)
	fixture.addProvider(t, ProviderNameXAI, ModelNameGrok43, "")
	if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedModelIdentitySchemaVersion, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
		t.Fatalf("seed schema version: %v", createError)
	}
	assertManagedDashScopeMigrationError(t, initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers), "provider_conflict=grok:xai")
}
