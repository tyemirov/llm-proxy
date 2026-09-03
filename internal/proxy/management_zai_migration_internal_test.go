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

type managedZAIProviderMigrationFixture struct {
	database          *gorm.DB
	providerKeyCipher managedProviderKeyCipher
	providers         *providerRegistry
	tenant            managedTenantRecord
	providerKey       managedProviderAPIKeyRecord
	usage             managedUsageEventRecord
}

type managedZAIProviderFailingReader struct{}

func (managedZAIProviderFailingReader) Read([]byte) (int, error) {
	return 0, errInternalTestDatabase
}

func newManagedZAIProviderMigrationFixture(t *testing.T) managedZAIProviderMigrationFixture {
	t.Helper()
	database, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "zai-provider-stage.db")), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open Z.AI provider fixture: %v", openError)
	}
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create Z.AI provider schema: %v", migrationError)
	}
	now := time.Date(2026, 8, 10, 23, 45, 0, 0, time.UTC)
	user := managedUserRecord{UserID: "zai-provider-owner", CreatedAt: now, UpdatedAt: now}
	if createError := database.Create(&user).Error; createError != nil {
		t.Fatalf("seed Z.AI provider user: %v", createError)
	}
	tenantRecord := fakeTenantRecord(user.UserID, "zai-provider-tenant", "Default", now)
	tenantRecord.DefaultProvider = retiredZhipuProviderIdentifier
	tenantRecord.DefaultModel = ModelNameZAIGLM
	tenantRecord.DefaultDictationProvider = retiredZhipuProviderIdentifier
	tenantRecord.DefaultDictationModel = "glm-asr-2512"
	tenantRecord.DefaultSystemPrompt = "preserve tenant prompt"
	if createError := database.Create(&tenantRecord).Error; createError != nil {
		t.Fatalf("seed Z.AI provider tenant: %v", createError)
	}
	providerKeyCipher := internalManagedProviderKeyCipher()
	encryptedKey, encryptionError := providerKeyCipher.encrypt(
		strings.NewReader(strings.Repeat("x", providerKeyCipher.aeadCipher.NonceSize())),
		tenantRecord.TenantID,
		retiredZhipuProviderIdentifier,
		"sk-zhipu",
	)
	if encryptionError != nil {
		t.Fatalf("encrypt Z.AI provider key: %v", encryptionError)
	}
	providerKey := managedProviderAPIKeyRecord{
		TenantID: tenantRecord.TenantID, ProviderID: retiredZhipuProviderIdentifier,
		EncryptedAPIKey: encryptedKey, TextModel: ModelNameZAIGLM,
		SystemPrompt: "preserve provider prompt", CreatedAt: now, UpdatedAt: now,
	}
	if createError := database.Create(&providerKey).Error; createError != nil {
		t.Fatalf("seed Z.AI provider key: %v", createError)
	}
	usage := managedUsageEventRecord{
		ID: 101, TenantID: tenantRecord.TenantID, Endpoint: usageEndpointText,
		ProviderID: retiredZhipuProviderIdentifier, ModelID: ModelNameZAIGLM,
		StatusCode: http.StatusOK, Success: true, OutcomeCode: managedUsageOutcomeSuccess, CreatedAt: now,
	}
	if createError := database.Create(&usage).Error; createError != nil {
		t.Fatalf("seed Z.AI provider usage: %v", createError)
	}
	return managedZAIProviderMigrationFixture{
		database: database, providerKeyCipher: providerKeyCipher,
		providers: internalManagementProviderRegistry(), tenant: tenantRecord,
		providerKey: providerKey, usage: usage,
	}
}

func (fixture managedZAIProviderMigrationFixture) addProvider(t *testing.T, providerIdentifier string, textModel string, encryptedAPIKey string) {
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

func applyManagedZAIProviderDataset(t *testing.T, fixture managedZAIProviderMigrationFixture, dataset managedZAIProviderDataset) {
	t.Helper()
	for _, backfill := range dataset.providerKeys {
		if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).
			Where(&managedProviderAPIKeyRecord{TenantID: backfill.record.TenantID, ProviderID: retiredZhipuProviderIdentifier}).
			UpdateColumns(map[string]any{"provider_id": backfill.record.ProviderID, "encrypted_api_key": backfill.record.EncryptedAPIKey}).Error; updateError != nil {
			t.Fatalf("apply Z.AI provider key: %v", updateError)
		}
	}
	for _, backfill := range dataset.tenants {
		if updateError := fixture.database.Model(&managedTenantRecord{}).
			Where(&managedTenantRecord{TenantID: backfill.tenantID}).
			Updates(managedRoutingDefaultsDatabaseUpdates(backfill.defaults, backfill.updatedAt)).Error; updateError != nil {
			t.Fatalf("apply Z.AI tenant defaults: %v", updateError)
		}
	}
}

func TestManagedZAIProviderMigrationCanonicalizesCurrentRoutesAndPreservesUsage(t *testing.T) {
	fixture := newManagedZAIProviderMigrationFixture(t)
	if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedDashScopeSettingsSchemaVersion, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
		t.Fatalf("seed schema version: %v", createError)
	}
	if migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers); migrationError != nil {
		t.Fatalf("migrate Z.AI provider: %v", migrationError)
	}
	if fixture.database.Migrator().HasTable(managedProviderKeyTable) {
		t.Fatal("predecessor provider table remains after Z.AI connection migration")
	}
	var connection managedProviderConnectionRecord
	if queryError := fixture.database.Where(&managedProviderConnectionRecord{TenantID: fixture.tenant.TenantID, ProviderID: ProviderNameZAI, FieldID: CatalogCredentialAPIKey}).First(&connection).Error; queryError != nil {
		t.Fatalf("load Z.AI provider connection: %v", queryError)
	}
	var profile managedProviderProfileRecord
	if queryError := fixture.database.Where(&managedProviderProfileRecord{TenantID: fixture.tenant.TenantID, ProviderID: ProviderNameZAI}).First(&profile).Error; queryError != nil {
		t.Fatalf("load Z.AI provider profile: %v", queryError)
	}
	apiKey, decryptError := fixture.providerKeyCipher.decryptConnection(connection)
	if decryptError != nil || apiKey != "sk-zhipu" || profile.TextModel != fixture.providerKey.TextModel || profile.SystemPrompt != fixture.providerKey.SystemPrompt || !connection.CreatedAt.Equal(fixture.providerKey.CreatedAt) || !connection.UpdatedAt.Equal(fixture.providerKey.UpdatedAt) || !profile.CreatedAt.Equal(fixture.providerKey.CreatedAt) || !profile.UpdatedAt.Equal(fixture.providerKey.UpdatedAt) {
		t.Fatalf("migrated connection=%+v profile=%+v api_key=%q error=%v", connection, profile, apiKey, decryptError)
	}
	if _, oldDecryptError := fixture.providerKeyCipher.decryptValue(connection.Value, connection.TenantID, retiredZhipuProviderIdentifier); oldDecryptError == nil {
		t.Fatal("migrated provider key still decrypts with retired associated data")
	}
	var tenantRecord managedTenantRecord
	if queryError := fixture.database.Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).First(&tenantRecord).Error; queryError != nil {
		t.Fatalf("load Z.AI tenant: %v", queryError)
	}
	expectedDefaults := TenantDefaults{
		Provider: ProviderNameZAI, Model: ModelNameZAIGLM,
		DictationProvider: ProviderNameZAI, DictationModel: "glm-asr-2512",
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
		t.Fatalf("reopen Z.AI provider schema: %v", validationError)
	}
}

func TestManagedZAIProviderMigrationRejectsStageFailures(t *testing.T) {
	assertMigrationError := func(t *testing.T, migrationError error, want string) {
		t.Helper()
		if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), want) {
			t.Fatalf("migration error=%v want=%q", migrationError, want)
		}
	}
	t.Run("encryption", func(t *testing.T) {
		fixture := newManagedZAIProviderMigrationFixture(t)
		_, migrationError := preflightManagedZAIProviderWithReader(fixture.database, fixture.providerKeyCipher, fixture.providers, managedZAIProviderFailingReader{})
		assertMigrationError(t, migrationError, "operation=preflight")
	})
	for _, testCase := range []struct {
		name      string
		want      string
		configure func(*testing.T, managedZAIProviderMigrationFixture)
	}{
		{
			name: "read tenants", want: "operation=read",
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "zai_provider_read", "query", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "provider conflict", want: "provider_conflict=zhipu:zai",
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				fixture.addProvider(t, ProviderNameZAI, ModelNameZAIGLM, "")
			},
		},
		{
			name: "noncanonical provider", want: "reason=not_canonical",
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).Where(&managedProviderAPIKeyRecord{TenantID: fixture.tenant.TenantID}).Update("provider_id", "ZHIPU").Error; updateError != nil {
					t.Fatalf("seed noncanonical provider: %v", updateError)
				}
			},
		},
		{
			name: "retired base URL", want: "field=base_url reason=not_canonical",
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).Where(&managedProviderAPIKeyRecord{TenantID: fixture.tenant.TenantID}).Update("base_url", "https://open.bigmodel.cn/api/paas/v4").Error; updateError != nil {
					t.Fatalf("seed retired base URL: %v", updateError)
				}
			},
		},
		{
			name: "decrypt retired provider", want: "operation=preflight",
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).Where(&managedProviderAPIKeyRecord{TenantID: fixture.tenant.TenantID, ProviderID: retiredZhipuProviderIdentifier}).Update("encrypted_api_key", "invalid").Error; updateError != nil {
					t.Fatalf("seed invalid retired provider: %v", updateError)
				}
			},
		},
		{
			name: "decrypt retained provider", want: "operation=preflight",
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				fixture.addProvider(t, ProviderNameOpenAI, ModelNameGPT41, "invalid")
			},
		},
		{
			name: "invalid defaults", want: "operation=preflight table=" + managedTenantTable,
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				if updateError := fixture.database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).Update("default_model", "missing-model").Error; updateError != nil {
					t.Fatalf("seed invalid defaults: %v", updateError)
				}
			},
		},
		{
			name: "read historical usage", want: "operation=read table=" + managedUsageEventTable,
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "zai_provider_usage_read", "query", managedUsageEventTable, errInternalTestDatabase)
			},
		},
		{
			name: "backfill provider key", want: "operation=backfill table=" + managedProviderKeyTable,
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "zai_provider_key_backfill", "update", managedProviderKeyTable, errInternalTestDatabase)
			},
		},
		{
			name: "backfill tenant", want: "operation=backfill table=" + managedTenantTable,
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "zai_provider_tenant_backfill", "update", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "verify", want: "operation=verify",
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				if callbackError := fixture.database.Callback().Query().Before("gorm:query").Register("zai_provider_verify", func(callbackDatabase *gorm.DB) {
					if callbackDatabase.Statement.Table == managedProviderKeyTable {
						if _, isCount := callbackDatabase.Statement.Dest.(*int64); isCount {
							callbackDatabase.AddError(errInternalTestDatabase)
						}
					}
				}); callbackError != nil {
					t.Fatalf("register Z.AI verification failure: %v", callbackError)
				}
			},
		},
		{
			name: "record version", want: "operation=record_version",
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "zai_provider_record_version", "create", managedSchemaMigrationTable, errInternalTestDatabase)
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newManagedZAIProviderMigrationFixture(t)
			testCase.configure(t, fixture)
			assertMigrationError(t, migrateManagedZAIProvider(fixture.database, fixture.providerKeyCipher, fixture.providers), testCase.want)
		})
	}

	t.Run("schema seven validation", func(t *testing.T) {
		fixture := newManagedZAIProviderMigrationFixture(t)
		if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedDashScopeSettingsSchemaVersion, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
			t.Fatalf("seed schema version: %v", createError)
		}
		if dropError := fixture.database.Migrator().DropIndex(&managedUsageEventRecord{}, managedUsageFailurePageIndex); dropError != nil {
			t.Fatalf("drop usage failure index: %v", dropError)
		}
		assertMigrationError(t, initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers), "operation=validate_current_schema")
	})
}

func TestManagedZAIProviderVerificationRejectsDrift(t *testing.T) {
	assertVerificationError := func(t *testing.T, verificationError error, want string) {
		t.Helper()
		if !errors.Is(verificationError, errManagedTenantSchemaMigration) || !strings.Contains(verificationError.Error(), want) {
			t.Fatalf("verification error=%v want=%q", verificationError, want)
		}
	}
	verifiedFixture := func(t *testing.T) (managedZAIProviderMigrationFixture, managedZAIProviderDataset) {
		t.Helper()
		fixture := newManagedZAIProviderMigrationFixture(t)
		dataset, preflightError := preflightManagedZAIProvider(fixture.database, fixture.providerKeyCipher, fixture.providers)
		if preflightError != nil {
			t.Fatalf("preflight Z.AI verification fixture: %v", preflightError)
		}
		applyManagedZAIProviderDataset(t, fixture, dataset)
		return fixture, dataset
	}

	t.Run("retired provider remains", func(t *testing.T) {
		fixture := newManagedZAIProviderMigrationFixture(t)
		assertVerificationError(t, verifyManagedZAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, managedZAIProviderDataset{}), "operation=verify")
	})
	t.Run("provider query", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		if callbackError := fixture.database.Callback().Query().Before("gorm:query").Register("zai_verify_provider_query", func(callbackDatabase *gorm.DB) {
			if callbackDatabase.Statement.Table == managedProviderKeyTable {
				if _, isRecord := callbackDatabase.Statement.Dest.(*managedProviderAPIKeyRecord); isRecord {
					callbackDatabase.AddError(errInternalTestDatabase)
				}
			}
		}); callbackError != nil {
			t.Fatalf("register provider query failure: %v", callbackError)
		}
		assertVerificationError(t, verifyManagedZAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=verify")
	})
	t.Run("provider values", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		dataset.providerKeys[0].apiKey = "changed"
		assertVerificationError(t, verifyManagedZAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=verify")
	})
	t.Run("tenant query", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		registerManagedGORMError(t, fixture.database, "zai_verify_tenant_query", "query", managedTenantTable, errInternalTestDatabase)
		assertVerificationError(t, verifyManagedZAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=verify")
	})
	t.Run("tenant values", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		dataset.tenants[0].updatedAt = dataset.tenants[0].updatedAt.Add(time.Second)
		assertVerificationError(t, verifyManagedZAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "values")
	})
	t.Run("current routing validation", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		fixture.addProvider(t, ProviderNameOpenAI, ModelNameGPT41, "invalid")
		assertVerificationError(t, verifyManagedZAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=validate")
	})
	t.Run("usage query", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		registerManagedGORMError(t, fixture.database, "zai_verify_usage_query", "query", managedUsageEventTable, errInternalTestDatabase)
		assertVerificationError(t, verifyManagedZAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "operation=read")
	})
	t.Run("usage values", func(t *testing.T) {
		fixture, dataset := verifiedFixture(t)
		dataset.historicalUsage = nil
		assertVerificationError(t, verifyManagedZAIProviderMigration(fixture.database, fixture.providerKeyCipher, fixture.providers, dataset), "historical_usage_changed")
	})
}

func TestManagedZAICurrentSchemaRejectsRetiredRoutingValues(t *testing.T) {
	canonicalFixture := func(t *testing.T) managedZAIProviderMigrationFixture {
		t.Helper()
		fixture := newManagedZAIProviderMigrationFixture(t)
		dataset, preflightError := preflightManagedZAIProvider(fixture.database, fixture.providerKeyCipher, fixture.providers)
		if preflightError != nil {
			t.Fatalf("preflight canonical fixture: %v", preflightError)
		}
		applyManagedZAIProviderDataset(t, fixture, dataset)
		if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedZAIProviderSchemaVersion, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
			t.Fatalf("seed current schema version: %v", createError)
		}
		return fixture
	}
	for _, testCase := range []struct {
		name      string
		configure func(*testing.T, managedZAIProviderMigrationFixture)
		want      string
	}{
		{
			name: "retired provider key",
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).Where(&managedProviderAPIKeyRecord{TenantID: fixture.tenant.TenantID}).UpdateColumns(map[string]any{
					"provider_id": retiredZhipuProviderIdentifier, "encrypted_api_key": fixture.providerKey.EncryptedAPIKey,
				}).Error; updateError != nil {
					t.Fatalf("restore retired provider key: %v", updateError)
				}
			},
			want: "unknown provider: zhipu",
		},
		{
			name: "retired text default",
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				if updateError := fixture.database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).Update("default_provider", retiredZhipuProviderIdentifier).Error; updateError != nil {
					t.Fatalf("restore retired text default: %v", updateError)
				}
			},
			want: "unknown provider: zhipu",
		},
		{
			name: "retired dictation default",
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				if updateError := fixture.database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).Update("default_dictation_provider", retiredZhipuProviderIdentifier).Error; updateError != nil {
					t.Fatalf("restore retired dictation default: %v", updateError)
				}
			},
			want: "unknown provider: zhipu",
		},
		{
			name: "retired glm alias",
			configure: func(t *testing.T, fixture managedZAIProviderMigrationFixture) {
				if updateError := fixture.database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: fixture.tenant.TenantID}).Update("default_provider", "glm").Error; updateError != nil {
					t.Fatalf("seed retired GLM alias: %v", updateError)
				}
			},
			want: "unknown provider: glm",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := canonicalFixture(t)
			testCase.configure(t, fixture)
			validationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers)
			if validationError == nil || !strings.Contains(validationError.Error(), testCase.want) {
				t.Fatalf("validation error=%v want=%q", validationError, testCase.want)
			}
		})
	}
}

func TestManagedZAIInitializationPropagatesDashScopeFailures(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		version int
	}{
		{name: "schema five chain", version: managedModelIdentitySchemaVersion},
		{name: "schema six direct", version: managedXAIProviderSchemaVersion},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newManagedZAIProviderMigrationFixture(t)
			if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: testCase.version, AppliedAt: fixture.tenant.CreatedAt}).Error; createError != nil {
				t.Fatalf("seed predecessor schema version: %v", createError)
			}
			if callbackError := fixture.database.Callback().Create().Before("gorm:create").Register("zai_predecessor_dashscope_failure", func(callbackDatabase *gorm.DB) {
				record, isMigration := callbackDatabase.Statement.Dest.(*managedSchemaMigrationRecord)
				if isMigration && record.Version == managedDashScopeSettingsSchemaVersion {
					callbackDatabase.AddError(errInternalTestDatabase)
				}
			}); callbackError != nil {
				t.Fatalf("register DashScope migration failure: %v", callbackError)
			}
			migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers)
			if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), "operation=record_version") {
				t.Fatalf("migration error=%v", migrationError)
			}
		})
	}
}
