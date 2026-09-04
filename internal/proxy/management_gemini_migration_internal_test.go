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

type managedGeminiModelSelectionMigrationFixture struct {
	database          *gorm.DB
	providerKeyCipher managedProviderKeyCipher
	providers         *providerRegistry
	profiles          []managedProviderProfileRecord
	tenants           []managedTenantRecord
	usage             []managedUsageEventRecord
	targetModel       string
	schemaVersion     int
}

func newManagedGeminiModelSelectionMigrationFixture(t *testing.T, schemaVersion int, databaseName string) managedGeminiModelSelectionMigrationFixture {
	t.Helper()
	database, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), databaseName)), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open Gemini 3-only fixture: %v", openError)
	}
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create Gemini 3-only schema: %v", migrationError)
	}
	useManagedUsageSchemaTwelve(t, database)
	if dropError := database.Migrator().DropTable(&managedProviderAPIKeyRecord{}); dropError != nil {
		t.Fatalf("drop predecessor provider table: %v", dropError)
	}
	now := time.Date(2026, 8, 22, 18, 0, 0, 0, time.UTC)
	user := managedUserRecord{UserID: "gemini-3-only-owner", CreatedAt: now, UpdatedAt: now}
	if createError := database.Create(&user).Error; createError != nil {
		t.Fatalf("seed Gemini 3-only user: %v", createError)
	}
	providerKeyCipher := internalManagedProviderKeyCipher()
	providers := internalManagementProviderRegistry()
	migrations := providers.modelMigrationsFor(schemaVersion, ProviderNameGemini, ModelOperationText)
	retiredModels := make([]string, 0, len(migrations))
	targetModel := ""
	for _, migration := range migrations {
		retiredModels = append(retiredModels, migration.source)
		if targetModel == "" {
			targetModel = migration.target
		} else if migration.target != targetModel {
			t.Fatalf("Gemini migrations have different targets")
		}
	}
	if len(retiredModels) == 0 || targetModel == "" {
		t.Fatal("Gemini model migrations are missing")
	}
	fixture := managedGeminiModelSelectionMigrationFixture{
		database: database, providerKeyCipher: providerKeyCipher,
		providers: providers, targetModel: targetModel, schemaVersion: schemaVersion,
		profiles: make([]managedProviderProfileRecord, 0, len(retiredModels)),
		tenants:  make([]managedTenantRecord, 0, len(retiredModels)),
		usage:    make([]managedUsageEventRecord, 0, len(retiredModels)),
	}
	for index, retiredModel := range retiredModels {
		timestamp := now.Add(time.Duration(index) * time.Minute)
		tenant := fakeTenantRecord(user.UserID, "gemini-3-only-tenant-"+string(rune('a'+index)), "Gemini tenant "+string(rune('A'+index)), timestamp)
		tenant.DefaultProvider = ProviderNameGemini
		tenant.DefaultModel = retiredModel
		if createError := database.Create(&tenant).Error; createError != nil {
			t.Fatalf("seed Gemini tenant model=%s: %v", retiredModel, createError)
		}
		encryptedKey, encryptionError := providerKeyCipher.encryptConnection(
			strings.NewReader(strings.Repeat(string(rune('a'+index)), providerKeyCipher.aeadCipher.NonceSize())),
			tenant.TenantID,
			ProviderNameGemini,
			CatalogCredentialAPIKey,
			"gemini-key-"+retiredModel,
		)
		if encryptionError != nil {
			t.Fatalf("encrypt Gemini connection model=%s: %v", retiredModel, encryptionError)
		}
		connection := managedProviderConnectionRecord{
			TenantID: tenant.TenantID, ProviderID: ProviderNameGemini, FieldID: CatalogCredentialAPIKey,
			Value: encryptedKey, CreatedAt: timestamp, UpdatedAt: timestamp,
		}
		profile := managedProviderProfileRecord{
			TenantID: tenant.TenantID, ProviderID: ProviderNameGemini, TextModel: retiredModel,
			SystemPrompt: "preserve Gemini prompt", CreatedAt: timestamp, UpdatedAt: timestamp,
		}
		usage := managedUsageEventRecord{
			ID: uint(index + 1), TenantID: tenant.TenantID, Endpoint: usageEndpointText,
			ProviderID: ProviderNameGemini, ModelID: retiredModel,
			StatusCode: http.StatusOK, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess,
			TotalTokens: index + 1, CreatedAt: timestamp,
		}
		if createError := database.Create(&connection).Error; createError != nil {
			t.Fatalf("seed Gemini connection model=%s: %v", retiredModel, createError)
		}
		if createError := database.Create(&profile).Error; createError != nil {
			t.Fatalf("seed Gemini profile model=%s: %v", retiredModel, createError)
		}
		createManagedUsageSchemaTwelve(t, database, usage)
		fixture.tenants = append(fixture.tenants, tenant)
		fixture.profiles = append(fixture.profiles, profile)
		fixture.usage = append(fixture.usage, usage)
	}
	if createError := database.Create(&managedSchemaMigrationRecord{Version: schemaVersion - 1, AppliedAt: now}).Error; createError != nil {
		t.Fatalf("seed Gemini schema version: %v", createError)
	}
	return fixture
}

func (fixture managedGeminiModelSelectionMigrationFixture) assertUnchanged(t *testing.T) {
	t.Helper()
	for index, expectedProfile := range fixture.profiles {
		var profile managedProviderProfileRecord
		if queryError := fixture.database.Where(&managedProviderProfileRecord{TenantID: expectedProfile.TenantID, ProviderID: ProviderNameGemini}).First(&profile).Error; queryError != nil {
			t.Fatalf("load Gemini profile after rollback: %v", queryError)
		}
		var tenant managedTenantRecord
		if queryError := fixture.database.Where(&managedTenantRecord{TenantID: expectedProfile.TenantID}).First(&tenant).Error; queryError != nil {
			t.Fatalf("load Gemini tenant after rollback: %v", queryError)
		}
		if profile.TextModel != expectedProfile.TextModel || !profile.UpdatedAt.Equal(expectedProfile.UpdatedAt) || tenant.DefaultModel != fixture.tenants[index].DefaultModel || !tenant.UpdatedAt.Equal(fixture.tenants[index].UpdatedAt) {
			t.Fatalf("Gemini migration was not atomic profile=%+v tenant=%+v", profile, tenant)
		}
	}
	var currentVersionCount int64
	if countError := fixture.database.Model(&managedSchemaMigrationRecord{}).Where(&managedSchemaMigrationRecord{Version: fixture.schemaVersion}).Count(&currentVersionCount).Error; countError != nil || currentVersionCount != 0 {
		t.Fatalf("Gemini schema version count=%d error=%v", currentVersionCount, countError)
	}
}

func (fixture managedGeminiModelSelectionMigrationFixture) setStartingSchemaVersion(t *testing.T, schemaVersion int) {
	t.Helper()
	updateResult := fixture.database.Model(&managedSchemaMigrationRecord{}).
		Where(&managedSchemaMigrationRecord{Version: fixture.schemaVersion - 1}).
		UpdateColumn(managedSchemaVersionColumn, schemaVersion)
	if updateResult.Error != nil || updateResult.RowsAffected != 1 {
		t.Fatalf("set Gemini starting schema version=%d rows=%d error=%v", schemaVersion, updateResult.RowsAffected, updateResult.Error)
	}
}

func TestManagedGeminiModelSelectionMigrationsMoveRoutingStateAndPreserveUsage(t *testing.T) {
	testCases := []struct {
		name                   string
		migrationSchemaVersion int
		startingSchemaVersion  int
	}{
		{name: "Gemini 2.5 retirement", migrationSchemaVersion: managedGemini3OnlySchemaVersion, startingSchemaVersion: managedProviderConnectionsSchemaVersion},
		{name: "Gemini 3.1 route retirement", migrationSchemaVersion: managedGeminiRouteRetirementVersion, startingSchemaVersion: managedGemini3OnlySchemaVersion},
		{name: "Gemini 3.1 route retirement from schema 9", migrationSchemaVersion: managedGeminiRouteRetirementVersion, startingSchemaVersion: managedProviderConnectionsSchemaVersion},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newManagedGeminiModelSelectionMigrationFixture(t, testCase.migrationSchemaVersion, "gemini-model-selection.db")
			fixture.setStartingSchemaVersion(t, testCase.startingSchemaVersion)
			if migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers); migrationError != nil {
				t.Fatalf("migrate Gemini model selection schema: %v", migrationError)
			}
			for index, previousProfile := range fixture.profiles {
				var profile managedProviderProfileRecord
				if queryError := fixture.database.Where(&managedProviderProfileRecord{TenantID: previousProfile.TenantID, ProviderID: ProviderNameGemini}).First(&profile).Error; queryError != nil {
					t.Fatalf("load migrated Gemini profile: %v", queryError)
				}
				var tenant managedTenantRecord
				if queryError := fixture.database.Where(&managedTenantRecord{TenantID: previousProfile.TenantID}).First(&tenant).Error; queryError != nil {
					t.Fatalf("load migrated Gemini tenant: %v", queryError)
				}
				if profile.TextModel != fixture.targetModel || !profile.UpdatedAt.Equal(previousProfile.UpdatedAt) || tenant.DefaultModel != fixture.targetModel || !tenant.UpdatedAt.Equal(fixture.tenants[index].UpdatedAt) {
					t.Fatalf("migrated Gemini profile=%+v tenant=%+v", profile, tenant)
				}
			}
			var historicalUsage []managedUsageEventRecord
			if queryError := fixture.database.Order("id").Find(&historicalUsage).Error; queryError != nil || len(historicalUsage) != len(fixture.usage) {
				t.Fatalf("historical Gemini usage=%+v error=%v", historicalUsage, queryError)
			}
			for index := range historicalUsage {
				if historicalUsage[index] != managedUsageRecordWithoutRoute(fixture.usage[index]) {
					t.Fatalf("historical Gemini usage changed=%+v", historicalUsage)
				}
			}
			var latest managedSchemaMigrationRecord
			if queryError := fixture.database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedTenantSchemaVersion {
				t.Fatalf("latest Gemini schema version=%+v error=%v", latest, queryError)
			}
			if validationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers); validationError != nil {
				t.Fatalf("reopen Gemini model selection schema: %v", validationError)
			}
		})
	}
}

func TestManagedGeminiModelSelectionMigrationsStartFromSchemaEight(t *testing.T) {
	providers := internalManagementProviderRegistry()
	var migrations []managedModelMigration
	for _, schemaVersion := range managedModelSelectionSchemaVersions {
		migrations = append(migrations, providers.modelMigrationsFor(schemaVersion, ProviderNameGemini, ModelOperationText)...)
	}
	for _, migration := range migrations {
		t.Run(migration.source, func(t *testing.T) {
			fixture := newManagedProviderConnectionsMigrationFixture(t, ProviderNameGemini, migration.source, "")
			var originalTenant managedTenantRecord
			if queryError := fixture.database.Where(&managedTenantRecord{TenantID: fixture.predecessor.TenantID}).First(&originalTenant).Error; queryError != nil {
				t.Fatalf("load schema-eight Gemini tenant: %v", queryError)
			}
			historicalUsage := managedUsageEventRecord{
				ID: 1, TenantID: fixture.predecessor.TenantID, Endpoint: usageEndpointText,
				ProviderID: ProviderNameGemini, ModelID: migration.source,
				StatusCode: http.StatusOK, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess,
				RequestTokens: 1, ResponseTokens: 2, TotalTokens: 3, CreatedAt: fixture.predecessor.CreatedAt,
			}
			createManagedUsageSchemaTwelve(t, fixture.database, historicalUsage)
			if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedZAIProviderSchemaVersion, AppliedAt: fixture.predecessor.CreatedAt}).Error; createError != nil {
				t.Fatalf("seed schema-eight Gemini version: %v", createError)
			}

			if migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers); migrationError != nil {
				t.Fatalf("migrate schema-eight Gemini model=%s: %v", migration.source, migrationError)
			}
			if fixture.database.Migrator().HasTable(managedProviderKeyTable) {
				t.Fatal("schema-eight Gemini provider table was retained")
			}

			var connection managedProviderConnectionRecord
			if queryError := fixture.database.Where(&managedProviderConnectionRecord{
				TenantID: fixture.predecessor.TenantID, ProviderID: ProviderNameGemini, FieldID: CatalogCredentialAPIKey,
			}).First(&connection).Error; queryError != nil {
				t.Fatalf("load migrated Gemini connection: %v", queryError)
			}
			apiKey, decryptError := fixture.providerKeyCipher.decryptConnection(connection)
			if decryptError != nil || apiKey != "sk-provider" || !connection.CreatedAt.Equal(fixture.predecessor.CreatedAt) || !connection.UpdatedAt.Equal(fixture.predecessor.UpdatedAt) {
				t.Fatalf("migrated Gemini connection=%+v key=%q error=%v", connection, apiKey, decryptError)
			}

			var profile managedProviderProfileRecord
			if queryError := fixture.database.Where(&managedProviderProfileRecord{TenantID: fixture.predecessor.TenantID, ProviderID: ProviderNameGemini}).First(&profile).Error; queryError != nil {
				t.Fatalf("load migrated Gemini profile: %v", queryError)
			}
			if profile.TextModel != migration.target || profile.SystemPrompt != fixture.predecessor.SystemPrompt || !profile.CreatedAt.Equal(fixture.predecessor.CreatedAt) || !profile.UpdatedAt.Equal(fixture.predecessor.UpdatedAt) {
				t.Fatalf("migrated Gemini profile=%+v", profile)
			}

			var tenant managedTenantRecord
			if queryError := fixture.database.Where(&managedTenantRecord{TenantID: fixture.predecessor.TenantID}).First(&tenant).Error; queryError != nil {
				t.Fatalf("load migrated Gemini tenant: %v", queryError)
			}
			if tenant.DefaultProvider != ProviderNameGemini || tenant.DefaultModel != migration.target || !tenant.UpdatedAt.Equal(originalTenant.UpdatedAt) {
				t.Fatalf("migrated Gemini tenant=%+v", tenant)
			}

			var migratedUsage managedUsageEventRecord
			if queryError := fixture.database.First(&migratedUsage, historicalUsage.ID).Error; queryError != nil || migratedUsage != managedUsageRecordWithoutRoute(historicalUsage) {
				t.Fatalf("migrated Gemini usage=%+v error=%v", migratedUsage, queryError)
			}
			var latest managedSchemaMigrationRecord
			if queryError := fixture.database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedTenantSchemaVersion {
				t.Fatalf("latest Gemini schema version=%+v error=%v", latest, queryError)
			}
		})
	}
}

func TestManagedGeminiPredecessorProjectionRollsBackStageFailures(t *testing.T) {
	migrations := internalManagementProviderRegistry().modelMigrationsFor(managedGemini3OnlySchemaVersion, ProviderNameGemini, ModelOperationText)
	if len(migrations) == 0 {
		t.Fatal("Gemini model migrations are missing")
	}
	testCases := []struct {
		name      string
		want      string
		configure func(*testing.T, managedProviderConnectionsMigrationFixture)
	}{
		{
			name: "model migrations", want: "operation=read_model_migrations",
			configure: func(_ *testing.T, fixture managedProviderConnectionsMigrationFixture) {
				delete(fixture.providers.modelMigrations, managedGemini3OnlySchemaVersion)
			},
		},
		{
			name: "provider model", want: "operation=backfill table=" + managedProviderKeyTable,
			configure: func(t *testing.T, fixture managedProviderConnectionsMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "gemini_predecessor_provider_backfill", "update", managedProviderKeyTable, errInternalTestDatabase)
			},
		},
		{
			name: "tenant model", want: "operation=backfill table=" + managedTenantTable,
			configure: func(t *testing.T, fixture managedProviderConnectionsMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "gemini_predecessor_tenant_backfill", "update", managedTenantTable, errInternalTestDatabase)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newManagedProviderConnectionsMigrationFixture(t, ProviderNameGemini, migrations[0].source, "")
			if createError := fixture.database.Create(&managedSchemaMigrationRecord{Version: managedZAIProviderSchemaVersion, AppliedAt: fixture.predecessor.CreatedAt}).Error; createError != nil {
				t.Fatalf("seed schema-eight Gemini version: %v", createError)
			}
			testCase.configure(t, fixture)

			migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers)
			if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), testCase.want) {
				t.Fatalf("Gemini predecessor migration error=%v want=%q", migrationError, testCase.want)
			}
			var predecessor managedProviderAPIKeyRecord
			if queryError := fixture.database.Where(&managedProviderAPIKeyRecord{TenantID: fixture.predecessor.TenantID, ProviderID: ProviderNameGemini}).First(&predecessor).Error; queryError != nil || predecessor.TextModel != migrations[0].source {
				t.Fatalf("Gemini predecessor after rollback=%+v error=%v", predecessor, queryError)
			}
			var tenant managedTenantRecord
			if queryError := fixture.database.Where(&managedTenantRecord{TenantID: fixture.predecessor.TenantID}).First(&tenant).Error; queryError != nil || tenant.DefaultModel != migrations[0].source {
				t.Fatalf("Gemini tenant after rollback=%+v error=%v", tenant, queryError)
			}
			var latest managedSchemaMigrationRecord
			if queryError := fixture.database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedZAIProviderSchemaVersion {
				t.Fatalf("Gemini schema version after rollback=%+v error=%v", latest, queryError)
			}
		})
	}
}

func TestManagedModelSelectionMigrationUsesCatalogTarget(t *testing.T) {
	fixture := newManagedGeminiModelSelectionMigrationFixture(t, managedGemini3OnlySchemaVersion, "gemini-3-only-catalog.db")
	schema := internalCanonicalProviderCatalog().Schema()
	alternateTarget := ""
	for _, provider := range schema.Providers {
		if provider.ID != ProviderNameGemini {
			continue
		}
		for _, offering := range provider.Offerings {
			if offering.Model != fixture.targetModel && slices.Contains(offering.Operations, ModelOperationText) {
				alternateTarget = offering.Model
				break
			}
		}
	}
	if alternateTarget == "" {
		t.Fatal("alternate Gemini text offering is missing")
	}
	for migrationIndex := range schema.ModelMigrations {
		migration := &schema.ModelMigrations[migrationIndex]
		if migration.ManagedSchemaVersion == managedGemini3OnlySchemaVersion {
			migration.TargetModel = alternateTarget
		}
	}
	catalog, catalogError := NewProviderCatalog(schema)
	if catalogError != nil {
		t.Fatalf("compile changed model migration policy: %v", catalogError)
	}
	fixture.providers = newProviderRegistry(Configuration{ProviderCatalog: catalog})
	if migrationError := migrateManagedModelSelections(fixture.database, fixture.providerKeyCipher, fixture.providers, managedGemini3OnlySchemaVersion); migrationError != nil {
		t.Fatalf("apply changed model migration policy: %v", migrationError)
	}
	for _, previousProfile := range fixture.profiles {
		var profile managedProviderProfileRecord
		if queryError := fixture.database.Where(&managedProviderProfileRecord{TenantID: previousProfile.TenantID, ProviderID: ProviderNameGemini}).First(&profile).Error; queryError != nil {
			t.Fatalf("load catalog-migrated profile: %v", queryError)
		}
		var tenant managedTenantRecord
		if queryError := fixture.database.Where(&managedTenantRecord{TenantID: previousProfile.TenantID}).First(&tenant).Error; queryError != nil {
			t.Fatalf("load catalog-migrated tenant: %v", queryError)
		}
		if profile.TextModel != alternateTarget || tenant.DefaultModel != alternateTarget {
			t.Fatalf("catalog target=%s profile=%s tenant=%s", alternateTarget, profile.TextModel, tenant.DefaultModel)
		}
	}
}

func TestManagedGeminiModelSelectionMigrationsRejectIncompletePredecessorSchemas(t *testing.T) {
	testCases := []struct {
		name          string
		schemaVersion int
	}{
		{name: "Gemini 2.5 retirement", schemaVersion: managedGemini3OnlySchemaVersion},
		{name: "Gemini 3.1 route retirement", schemaVersion: managedGeminiRouteRetirementVersion},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newManagedGeminiModelSelectionMigrationFixture(t, testCase.schemaVersion, "gemini-model-selection-incomplete.db")
			if dropError := fixture.database.Migrator().DropTable(&managedProviderProfileRecord{}); dropError != nil {
				t.Fatalf("drop predecessor profile table: %v", dropError)
			}
			migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers)
			if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), "operation=validate_current_schema table="+managedProviderConnectionTable) {
				t.Fatalf("Gemini predecessor schema error=%v", migrationError)
			}
		})
	}
}

func TestManagedGeminiRouteRetirementRejectsMissingMigrationPolicy(t *testing.T) {
	fixture := newManagedGeminiModelSelectionMigrationFixture(t, managedGeminiRouteRetirementVersion, "gemini-route-retirement-missing-policy.db")
	fixture.setStartingSchemaVersion(t, managedProviderConnectionsSchemaVersion)
	delete(fixture.providers.modelMigrations, managedGeminiRouteRetirementVersion)
	migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers)
	if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), "operation=read_model_migrations") {
		t.Fatalf("Gemini route-retirement migration error=%v", migrationError)
	}
	fixture.assertUnchanged(t)
	var predecessorVersionCount int64
	if countError := fixture.database.Model(&managedSchemaMigrationRecord{}).Where(&managedSchemaMigrationRecord{Version: managedGemini3OnlySchemaVersion}).Count(&predecessorVersionCount).Error; countError != nil || predecessorVersionCount != 0 {
		t.Fatalf("Gemini predecessor schema version count=%d error=%v", predecessorVersionCount, countError)
	}
}

func TestManagedGemini3OnlyMigrationRollsBackStageFailures(t *testing.T) {
	testCases := []struct {
		name      string
		want      string
		configure func(*testing.T, managedGeminiModelSelectionMigrationFixture)
	}{
		{
			name: "profile backfill", want: "operation=backfill table=" + managedProviderProfileTable,
			configure: func(t *testing.T, fixture managedGeminiModelSelectionMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "gemini_profile_backfill", "update", managedProviderProfileTable, errInternalTestDatabase)
			},
		},
		{
			name: "tenant backfill", want: "operation=backfill table=" + managedTenantTable,
			configure: func(t *testing.T, fixture managedGeminiModelSelectionMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "gemini_tenant_backfill", "update", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "validation", want: "operation=validate table=" + managedProviderConnectionTable,
			configure: func(t *testing.T, fixture managedGeminiModelSelectionMigrationFixture) {
				if updateError := fixture.database.Model(&managedProviderConnectionRecord{}).Where(&managedProviderConnectionRecord{TenantID: fixture.tenants[0].TenantID, ProviderID: ProviderNameGemini, FieldID: CatalogCredentialAPIKey}).UpdateColumn("value", "invalid").Error; updateError != nil {
					t.Fatalf("corrupt Gemini connection: %v", updateError)
				}
			},
		},
		{
			name: "record version", want: "operation=record_version table=" + managedSchemaMigrationTable,
			configure: func(t *testing.T, fixture managedGeminiModelSelectionMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "gemini_record_version", "create", managedSchemaMigrationTable, errInternalTestDatabase)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newManagedGeminiModelSelectionMigrationFixture(t, managedGemini3OnlySchemaVersion, "gemini-3-only-rollback.db")
			testCase.configure(t, fixture)
			migrationError := migrateManagedModelSelections(fixture.database, fixture.providerKeyCipher, fixture.providers, managedGemini3OnlySchemaVersion)
			if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), testCase.want) {
				t.Fatalf("Gemini migration error=%v want=%q", migrationError, testCase.want)
			}
			fixture.assertUnchanged(t)
		})
	}
}
