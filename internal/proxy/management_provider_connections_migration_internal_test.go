package proxy

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type managedProviderConnectionsMigrationFixture struct {
	databasePath      string
	database          *gorm.DB
	providerKeyCipher managedProviderKeyCipher
	providers         *providerRegistry
	predecessor       managedProviderAPIKeyRecord
}

type managedProviderConnectionsFailingReader struct{}

func (managedProviderConnectionsFailingReader) Read([]byte) (int, error) {
	return 0, errInternalTestDatabase
}

func newManagedProviderConnectionsMigrationFixture(t *testing.T, providerIdentifier string, textModel string, baseURL string) managedProviderConnectionsMigrationFixture {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "provider-connections.db")
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open provider connections fixture: %v", openError)
	}
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create provider connections schema: %v", migrationError)
	}
	now := time.Date(2026, 8, 20, 19, 0, 0, 0, time.UTC)
	user := managedUserRecord{UserID: "provider-connections-owner", CreatedAt: now, UpdatedAt: now}
	if createError := database.Create(&user).Error; createError != nil {
		t.Fatalf("seed provider connections user: %v", createError)
	}
	tenant := fakeTenantRecord(user.UserID, "provider-connections-tenant", "Default", now)
	tenant.DefaultProvider = providerIdentifier
	tenant.DefaultModel = textModel
	if providerIdentifier == ProviderNameOpenAI {
		tenant.DefaultDictationProvider = ProviderNameOpenAI
		tenant.DefaultDictationModel = DefaultDictationModel
	}
	if createError := database.Create(&tenant).Error; createError != nil {
		t.Fatalf("seed provider connections tenant: %v", createError)
	}
	providerKeyCipher := internalManagedProviderKeyCipher()
	encryptedAPIKey, encryptionError := providerKeyCipher.encrypt(
		strings.NewReader(strings.Repeat("m", providerKeyCipher.aeadCipher.NonceSize())),
		tenant.TenantID,
		providerIdentifier,
		"sk-provider",
	)
	if encryptionError != nil {
		t.Fatalf("encrypt predecessor provider key: %v", encryptionError)
	}
	predecessor := managedProviderAPIKeyRecord{
		TenantID: tenant.TenantID, ProviderID: providerIdentifier,
		EncryptedAPIKey: encryptedAPIKey, BaseURL: baseURL, TextModel: textModel,
		SystemPrompt: "preserve provider prompt", CreatedAt: now, UpdatedAt: now,
	}
	if createError := database.Create(&predecessor).Error; createError != nil {
		t.Fatalf("seed predecessor provider key: %v", createError)
	}
	return managedProviderConnectionsMigrationFixture{
		databasePath: databasePath, database: database, providerKeyCipher: providerKeyCipher,
		providers: internalManagementProviderRegistry(), predecessor: predecessor,
	}
}

func newOpenAIProviderConnectionsMigrationFixture(t *testing.T) managedProviderConnectionsMigrationFixture {
	t.Helper()
	return newManagedProviderConnectionsMigrationFixture(t, ProviderNameOpenAI, ModelNameGPT41, "")
}

func (fixture managedProviderConnectionsMigrationFixture) updatePredecessor(t *testing.T, updates map[string]any) {
	t.Helper()
	if updateError := fixture.database.Model(&managedProviderAPIKeyRecord{}).
		Where(&managedProviderAPIKeyRecord{TenantID: fixture.predecessor.TenantID, ProviderID: fixture.predecessor.ProviderID}).
		UpdateColumns(updates).Error; updateError != nil {
		t.Fatalf("update predecessor provider key: %v", updateError)
	}
}

func TestManagedProviderConnectionsMigrationMovesCanonicalRecords(t *testing.T) {
	fixture := newManagedProviderConnectionsMigrationFixture(
		t,
		ProviderNameDashScope,
		ModelNameDashScopeQwenPlus,
		"https://workspace.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1",
	)
	if migrationError := migrateManagedProviderConnectionDataWithReader(
		fixture.database,
		fixture.providerKeyCipher,
		fixture.providers,
		bytes.NewReader(bytes.Repeat([]byte{7}, 64)),
	); migrationError != nil {
		t.Fatalf("migrate provider connections: %v", migrationError)
	}
	var connectionCount int64
	if countError := fixture.database.Model(&managedProviderConnectionRecord{}).Count(&connectionCount).Error; countError != nil || connectionCount != 2 {
		t.Fatalf("connection count=%d error=%v", connectionCount, countError)
	}
	var profileCount int64
	if countError := fixture.database.Model(&managedProviderProfileRecord{}).Count(&profileCount).Error; countError != nil || profileCount != 1 {
		t.Fatalf("profile count=%d error=%v", profileCount, countError)
	}
	if fixture.database.Migrator().HasTable(managedProviderKeyTable) {
		t.Fatal("predecessor provider table was retained")
	}
}

func TestManagedProviderConnectionsMigrationRejectsPreflightFailures(t *testing.T) {
	testCases := []struct {
		name      string
		configure func(*testing.T, *managedProviderConnectionsMigrationFixture)
		reader    func() interface{ Read([]byte) (int, error) }
		want      string
	}{
		{
			name: "predecessor query",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "provider_connections_predecessor_query", "query", managedProviderKeyTable, errInternalTestDatabase)
			},
			want: "operation=read",
		},
		{
			name: "unknown provider",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				fixture.updatePredecessor(t, map[string]any{"provider_id": "missing"})
			},
			want: "reason=unknown",
		},
		{
			name: "credential count",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				definition := fixture.providers.definitions[providerID(ProviderNameOpenAI)]
				definition.fields["second_key"] = ProviderCatalogField{ID: "second_key", Kind: CatalogProviderFieldKindCredential}
				fixture.providers.definitions[providerID(ProviderNameOpenAI)] = definition
			},
			want: "credential_count=2",
		},
		{
			name: "credential decryption",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				fixture.updatePredecessor(t, map[string]any{"encrypted_api_key": "invalid"})
			},
			want: errManagedProviderKeyDecryption.Error(),
		},
		{
			name: "credential value",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				definition := fixture.providers.definitions[providerID(ProviderNameOpenAI)]
				field := definition.fields[CatalogCredentialAPIKey]
				field.Validation.MinimumLength = 64
				definition.fields[CatalogCredentialAPIKey] = field
				fixture.providers.definitions[providerID(ProviderNameOpenAI)] = definition
			},
			want: "field=api_key",
		},
		{
			name: "credential encryption",
			reader: func() interface{ Read([]byte) (int, error) } {
				return managedProviderConnectionsFailingReader{}
			},
			want: errManagedProviderKeyEncryption.Error(),
		},
		{
			name: "unmapped setting",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				definition := fixture.providers.definitions[providerID(ProviderNameOpenAI)]
				definition.fields["organization"] = ProviderCatalogField{ID: "organization", Kind: CatalogProviderFieldKindSetting}
				fixture.providers.definitions[providerID(ProviderNameOpenAI)] = definition
			},
			want: "reason=unmapped_predecessor_column",
		},
		{
			name: "text model",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				fixture.updatePredecessor(t, map[string]any{"text_model": "missing-model"})
			},
			want: "model=missing-model",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newOpenAIProviderConnectionsMigrationFixture(t)
			if testCase.configure != nil {
				testCase.configure(t, &fixture)
			}
			var randomReader interface{ Read([]byte) (int, error) } = bytes.NewReader(bytes.Repeat([]byte{3}, 64))
			if testCase.reader != nil {
				randomReader = testCase.reader()
			}
			migrationError := migrateManagedProviderConnectionDataWithReader(fixture.database, fixture.providerKeyCipher, fixture.providers, randomReader)
			if migrationError == nil || !strings.Contains(migrationError.Error(), testCase.want) {
				t.Fatalf("migration error=%v want=%q", migrationError, testCase.want)
			}
		})
	}

	t.Run("setting value", func(t *testing.T) {
		fixture := newManagedProviderConnectionsMigrationFixture(t, ProviderNameDashScope, ModelNameDashScopeQwenPlus, "invalid")
		migrationError := migrateManagedProviderConnectionDataWithReader(
			fixture.database,
			fixture.providerKeyCipher,
			fixture.providers,
			bytes.NewReader(bytes.Repeat([]byte{5}, 64)),
		)
		if migrationError == nil || !strings.Contains(migrationError.Error(), "field=base_url") {
			t.Fatalf("migration error=%v want invalid base_url", migrationError)
		}
	})
}

func TestManagedProviderConnectionsMigrationRejectsPersistenceFailures(t *testing.T) {
	testCases := []struct {
		name      string
		configure func(*testing.T, *managedProviderConnectionsMigrationFixture)
		want      string
	}{
		{
			name: "connection create",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "provider_connections_create", "create", managedProviderConnectionTable, errInternalTestDatabase)
			},
			want: "operation=create table=" + managedProviderConnectionTable,
		},
		{
			name: "profile create",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "provider_profiles_create", "create", managedProviderProfileTable, errInternalTestDatabase)
			},
			want: "operation=create table=" + managedProviderProfileTable,
		},
		{
			name: "connection query",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "provider_connections_verify_query", "query", managedProviderConnectionTable, errInternalTestDatabase)
			},
			want: "operation=verify table=" + managedProviderConnectionTable,
		},
		{
			name: "connection count",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				if callbackError := fixture.database.Callback().Query().After("gorm:after_query").Register("provider_connections_verify_count", func(database *gorm.DB) {
					records, correctType := database.Statement.Dest.(*[]managedProviderConnectionRecord)
					if correctType {
						*records = nil
					}
				}); callbackError != nil {
					t.Fatalf("register connection count callback: %v", callbackError)
				}
			},
			want: "operation=verify table=" + managedProviderConnectionTable,
		},
		{
			name: "connection decryption",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				if callbackError := fixture.database.Callback().Query().After("gorm:after_query").Register("provider_connections_verify_decryption", func(database *gorm.DB) {
					records, correctType := database.Statement.Dest.(*[]managedProviderConnectionRecord)
					if correctType && len(*records) != 0 {
						(*records)[0].Value = "invalid"
					}
				}); callbackError != nil {
					t.Fatalf("register connection decryption callback: %v", callbackError)
				}
			},
			want: errManagedProviderKeyDecryption.Error(),
		},
		{
			name: "connection values",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				if callbackError := fixture.database.Callback().Query().After("gorm:after_query").Register("provider_connections_verify_values", func(database *gorm.DB) {
					records, correctType := database.Statement.Dest.(*[]managedProviderConnectionRecord)
					if !correctType || len(*records) == 0 {
						return
					}
					changedValue, encryptionError := fixture.providerKeyCipher.encryptConnection(
						bytes.NewReader(bytes.Repeat([]byte{8}, fixture.providerKeyCipher.aeadCipher.NonceSize())),
						(*records)[0].TenantID,
						(*records)[0].ProviderID,
						(*records)[0].FieldID,
						"sk-changed",
					)
					if encryptionError == nil {
						(*records)[0].Value = changedValue
					}
				}); callbackError != nil {
					t.Fatalf("register connection values callback: %v", callbackError)
				}
			},
			want: "operation=verify table=" + managedProviderConnectionTable,
		},
		{
			name: "profile query",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "provider_profiles_verify_query", "query", managedProviderProfileTable, errInternalTestDatabase)
			},
			want: "operation=verify table=" + managedProviderProfileTable,
		},
		{
			name: "profile count",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				if callbackError := fixture.database.Callback().Query().After("gorm:after_query").Register("provider_profiles_verify_count", func(database *gorm.DB) {
					records, correctType := database.Statement.Dest.(*[]managedProviderProfileRecord)
					if correctType {
						*records = nil
					}
				}); callbackError != nil {
					t.Fatalf("register profile count callback: %v", callbackError)
				}
			},
			want: "operation=verify table=" + managedProviderProfileTable,
		},
		{
			name: "profile values",
			configure: func(t *testing.T, fixture *managedProviderConnectionsMigrationFixture) {
				if callbackError := fixture.database.Callback().Query().After("gorm:after_query").Register("provider_profiles_verify_values", func(database *gorm.DB) {
					records, correctType := database.Statement.Dest.(*[]managedProviderProfileRecord)
					if correctType && len(*records) != 0 {
						(*records)[0].SystemPrompt = "changed"
					}
				}); callbackError != nil {
					t.Fatalf("register profile values callback: %v", callbackError)
				}
			},
			want: "operation=verify table=" + managedProviderProfileTable,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newOpenAIProviderConnectionsMigrationFixture(t)
			testCase.configure(t, &fixture)
			migrationError := migrateManagedProviderConnectionDataWithReader(
				fixture.database,
				fixture.providerKeyCipher,
				fixture.providers,
				bytes.NewReader(bytes.Repeat([]byte{9}, 64)),
			)
			if migrationError == nil || !strings.Contains(migrationError.Error(), testCase.want) {
				t.Fatalf("migration error=%v want=%q", migrationError, testCase.want)
			}
		})
	}

	t.Run("predecessor drop", func(t *testing.T) {
		fixture := newOpenAIProviderConnectionsMigrationFixture(t)
		sqlDatabase, sqlError := fixture.database.DB()
		if sqlError != nil {
			t.Fatalf("provider connections SQL database: %v", sqlError)
		}
		if closeError := sqlDatabase.Close(); closeError != nil {
			t.Fatalf("close provider connections database: %v", closeError)
		}
		database, openError := gorm.Open(failingManagedDropDialector{Dialector: sqlite.Open(fixture.databasePath)}, &gorm.Config{})
		if openError != nil {
			t.Fatalf("reopen provider connections database: %v", openError)
		}
		migrationError := migrateManagedProviderConnectionDataWithReader(
			database,
			fixture.providerKeyCipher,
			fixture.providers,
			bytes.NewReader(bytes.Repeat([]byte{2}, 64)),
		)
		if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), "operation=drop_predecessor") {
			t.Fatalf("migration error=%v want predecessor drop failure", migrationError)
		}
	})
}

func TestManagedProviderConnectionsMigrationWrapperRejectsStageFailures(t *testing.T) {
	t.Run("schema creation", func(t *testing.T) {
		fixture := newOpenAIProviderConnectionsMigrationFixture(t)
		sqlDatabase, sqlError := fixture.database.DB()
		if sqlError != nil {
			t.Fatalf("provider connections SQL database: %v", sqlError)
		}
		if closeError := sqlDatabase.Close(); closeError != nil {
			t.Fatalf("close provider connections database: %v", closeError)
		}
		database, openError := gorm.Open(failingManagedAutoMigrateDialector{Dialector: sqlite.Open(fixture.databasePath)}, &gorm.Config{})
		if openError != nil {
			t.Fatalf("reopen provider connections database: %v", openError)
		}
		migrationError := migrateManagedProviderConnections(database, fixture.providerKeyCipher, fixture.providers)
		if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), "operation=create") {
			t.Fatalf("migration error=%v want schema creation failure", migrationError)
		}
	})

	t.Run("data migration", func(t *testing.T) {
		fixture := newOpenAIProviderConnectionsMigrationFixture(t)
		fixture.updatePredecessor(t, map[string]any{"provider_id": "missing"})
		migrationError := migrateManagedProviderConnections(fixture.database, fixture.providerKeyCipher, fixture.providers)
		if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), "reason=unknown") {
			t.Fatalf("migration error=%v want data migration failure", migrationError)
		}
	})
}

func TestManagedProviderConnectionsSchemaInitializationEdges(t *testing.T) {
	t.Run("fresh schema predecessor drop", func(t *testing.T) {
		database, openError := gorm.Open(failingManagedDropDialector{Dialector: sqlite.Open(filepath.Join(t.TempDir(), "drop.db"))}, &gorm.Config{})
		if openError != nil {
			t.Fatalf("open fresh schema fixture: %v", openError)
		}
		migrationError := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
		if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), "operation=drop_predecessor") {
			t.Fatalf("migration error=%v want predecessor drop failure", migrationError)
		}
	})

	t.Run("version eight predecessor shape", func(t *testing.T) {
		fixture := newOpenAIProviderConnectionsMigrationFixture(t)
		if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedZAIProviderSchemaVersion, AppliedAt: fixture.predecessor.CreatedAt}).Error; createError != nil {
			t.Fatalf("seed version eight: %v", createError)
		}
		if dropError := fixture.database.Migrator().DropColumn(&managedProviderAPIKeyRecord{}, "BaseURL"); dropError != nil {
			t.Fatalf("drop predecessor base URL: %v", dropError)
		}
		migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers)
		if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), "validate_current_schema") {
			t.Fatalf("migration error=%v want predecessor shape failure", migrationError)
		}
	})

	t.Run("version eight transition", func(t *testing.T) {
		fixture := newOpenAIProviderConnectionsMigrationFixture(t)
		if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedZAIProviderSchemaVersion, AppliedAt: fixture.predecessor.CreatedAt}).Error; createError != nil {
			t.Fatalf("seed version eight: %v", createError)
		}
		if migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers); migrationError != nil {
			t.Fatalf("initialize version eight schema: %v", migrationError)
		}
		var latest managedSchemaMigrationRecord
		if queryError := fixture.database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedTenantSchemaVersion {
			t.Fatalf("latest version=%+v error=%v", latest, queryError)
		}
	})
}

func TestManagedProviderConnectionsSchemaInitializationPropagatesZAIMigrationFailures(t *testing.T) {
	for _, schemaVersion := range []int{
		managedModelIdentitySchemaVersion,
		managedXAIProviderSchemaVersion,
		managedDashScopeSettingsSchemaVersion,
	} {
		t.Run(string(rune('0'+schemaVersion)), func(t *testing.T) {
			database, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "zai-dispatch.db")), &gorm.Config{})
			if openError != nil {
				t.Fatalf("open Z.AI dispatch fixture: %v", openError)
			}
			if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
				t.Fatalf("create Z.AI dispatch schema: %v", migrationError)
			}
			if createError := database.Create(&managedSchemaMigrationRecord{Version: schemaVersion, AppliedAt: time.Now().UTC()}).Error; createError != nil {
				t.Fatalf("seed schema version %d: %v", schemaVersion, createError)
			}

			remainingVersionCreations := managedDashScopeSettingsSchemaVersion - schemaVersion
			zaiMigrationActive := remainingVersionCreations == 0
			if callbackError := database.Callback().Create().After("gorm:after_create").Register("activate_zai_dispatch_failure", func(callbackDatabase *gorm.DB) {
				if callbackDatabase.Statement.Table != managedSchemaMigrationTable || zaiMigrationActive {
					return
				}
				remainingVersionCreations--
				zaiMigrationActive = remainingVersionCreations == 0
			}); callbackError != nil {
				t.Fatalf("register Z.AI dispatch activation: %v", callbackError)
			}
			if callbackError := database.Callback().Query().Before("gorm:query").Register("fail_zai_dispatch_query", func(callbackDatabase *gorm.DB) {
				if zaiMigrationActive && callbackDatabase.Statement.Table == managedTenantTable {
					callbackDatabase.AddError(errInternalTestDatabase)
				}
			}); callbackError != nil {
				t.Fatalf("register Z.AI dispatch query failure: %v", callbackError)
			}

			migrationError := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
			if migrationError == nil || !strings.Contains(migrationError.Error(), errInternalTestDatabase.Error()) {
				t.Fatalf("schema version=%d migration error=%v want Z.AI failure", schemaVersion, migrationError)
			}
		})
	}
}

func TestManagedProviderRoutingValidationReportsQueryFailures(t *testing.T) {
	t.Run("current connections", func(t *testing.T) {
		database := newCanonicalGORMFixture(t, time.Date(2026, 8, 20, 20, 0, 0, 0, time.UTC))
		registerManagedGORMError(t, database.database, "current_connection_routing_query", "query", managedTenantTable, errInternalTestDatabase)
		validationError := validateManagedConnectionRoutingDefaults(database.database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
		if !errors.Is(validationError, errManagedTenantSchemaMigration) || !strings.Contains(validationError.Error(), "operation=read") {
			t.Fatalf("validation error=%v want tenant query failure", validationError)
		}
	})

	t.Run("predecessor connections", func(t *testing.T) {
		fixture := newOpenAIProviderConnectionsMigrationFixture(t)
		registerManagedGORMError(t, fixture.database, "predecessor_connection_routing_query", "query", managedProviderKeyTable, errInternalTestDatabase)
		_, validationError := managedTenantRecordsForRoutingValidation(fixture.database)
		if !errors.Is(validationError, errManagedTenantSchemaMigration) || !strings.Contains(validationError.Error(), "tenant="+fixture.predecessor.TenantID) {
			t.Fatalf("validation error=%v want provider query failure", validationError)
		}
	})
}
