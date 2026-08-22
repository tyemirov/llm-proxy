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

type managedGemini3OnlyMigrationFixture struct {
	database          *gorm.DB
	providerKeyCipher managedProviderKeyCipher
	providers         *providerRegistry
	profiles          []managedProviderProfileRecord
	tenants           []managedTenantRecord
	usage             []managedUsageEventRecord
	targetModel       string
}

func newManagedGemini3OnlyMigrationFixture(t *testing.T) managedGemini3OnlyMigrationFixture {
	t.Helper()
	database, openError := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "gemini-3-only.db")), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open Gemini 3-only fixture: %v", openError)
	}
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create Gemini 3-only schema: %v", migrationError)
	}
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
	migrations := providers.modelMigrationsFor(managedGemini3OnlySchemaVersion, ProviderNameGemini, ModelOperationText)
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
	fixture := managedGemini3OnlyMigrationFixture{
		database: database, providerKeyCipher: providerKeyCipher,
		providers: providers, targetModel: targetModel,
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
			StatusCode: http.StatusOK, Success: true, OutcomeCode: managedUsageOutcomeSuccess,
			TotalTokens: index + 1, CreatedAt: timestamp,
		}
		if createError := database.Create(&connection).Error; createError != nil {
			t.Fatalf("seed Gemini connection model=%s: %v", retiredModel, createError)
		}
		if createError := database.Create(&profile).Error; createError != nil {
			t.Fatalf("seed Gemini profile model=%s: %v", retiredModel, createError)
		}
		if createError := database.Create(&usage).Error; createError != nil {
			t.Fatalf("seed Gemini usage model=%s: %v", retiredModel, createError)
		}
		fixture.tenants = append(fixture.tenants, tenant)
		fixture.profiles = append(fixture.profiles, profile)
		fixture.usage = append(fixture.usage, usage)
	}
	if createError := database.Create(&managedSchemaMigrationRecord{Version: managedProviderConnectionsSchemaVersion, AppliedAt: now}).Error; createError != nil {
		t.Fatalf("seed Gemini schema version: %v", createError)
	}
	return fixture
}

func (fixture managedGemini3OnlyMigrationFixture) assertUnchanged(t *testing.T) {
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
	if countError := fixture.database.Model(&managedSchemaMigrationRecord{}).Where(&managedSchemaMigrationRecord{Version: managedGemini3OnlySchemaVersion}).Count(&currentVersionCount).Error; countError != nil || currentVersionCount != 0 {
		t.Fatalf("Gemini schema version count=%d error=%v", currentVersionCount, countError)
	}
}

func TestManagedGemini3OnlyMigrationMovesRoutingStateAndPreservesUsage(t *testing.T) {
	fixture := newManagedGemini3OnlyMigrationFixture(t)
	if migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers); migrationError != nil {
		t.Fatalf("migrate Gemini 3-only schema: %v", migrationError)
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
	if queryError := fixture.database.Where(&managedUsageEventRecord{ProviderID: ProviderNameGemini}).Order("id").Find(&historicalUsage).Error; queryError != nil || len(historicalUsage) != len(fixture.usage) {
		t.Fatalf("historical Gemini usage=%+v error=%v", historicalUsage, queryError)
	}
	for index := range historicalUsage {
		if historicalUsage[index].ModelID != fixture.usage[index].ModelID {
			t.Fatalf("historical Gemini usage changed=%+v", historicalUsage)
		}
	}
	var latest managedSchemaMigrationRecord
	if queryError := fixture.database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedGemini3OnlySchemaVersion {
		t.Fatalf("latest Gemini schema version=%+v error=%v", latest, queryError)
	}
	if validationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers); validationError != nil {
		t.Fatalf("reopen Gemini 3-only schema: %v", validationError)
	}
}

func TestManagedModelSelectionMigrationUsesCatalogTarget(t *testing.T) {
	fixture := newManagedGemini3OnlyMigrationFixture(t)
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

func TestManagedGemini3OnlyMigrationRejectsIncompletePredecessorSchema(t *testing.T) {
	fixture := newManagedGemini3OnlyMigrationFixture(t)
	if dropError := fixture.database.Migrator().DropTable(&managedProviderProfileRecord{}); dropError != nil {
		t.Fatalf("drop predecessor profile table: %v", dropError)
	}
	migrationError := initializeManagedTenantSchema(fixture.database, fixture.providerKeyCipher, fixture.providers)
	if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), "operation=validate_current_schema table="+managedProviderConnectionTable) {
		t.Fatalf("Gemini predecessor schema error=%v", migrationError)
	}
}

func TestManagedGemini3OnlyMigrationRollsBackStageFailures(t *testing.T) {
	testCases := []struct {
		name      string
		want      string
		configure func(*testing.T, managedGemini3OnlyMigrationFixture)
	}{
		{
			name: "profile backfill", want: "operation=backfill table=" + managedProviderProfileTable,
			configure: func(t *testing.T, fixture managedGemini3OnlyMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "gemini_profile_backfill", "update", managedProviderProfileTable, errInternalTestDatabase)
			},
		},
		{
			name: "tenant backfill", want: "operation=backfill table=" + managedTenantTable,
			configure: func(t *testing.T, fixture managedGemini3OnlyMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "gemini_tenant_backfill", "update", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "validation", want: "operation=validate table=" + managedProviderConnectionTable,
			configure: func(t *testing.T, fixture managedGemini3OnlyMigrationFixture) {
				if updateError := fixture.database.Model(&managedProviderConnectionRecord{}).Where(&managedProviderConnectionRecord{TenantID: fixture.tenants[0].TenantID, ProviderID: ProviderNameGemini, FieldID: CatalogCredentialAPIKey}).UpdateColumn("value", "invalid").Error; updateError != nil {
					t.Fatalf("corrupt Gemini connection: %v", updateError)
				}
			},
		},
		{
			name: "record version", want: "operation=record_version table=" + managedSchemaMigrationTable,
			configure: func(t *testing.T, fixture managedGemini3OnlyMigrationFixture) {
				registerManagedGORMError(t, fixture.database, "gemini_record_version", "create", managedSchemaMigrationTable, errInternalTestDatabase)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newManagedGemini3OnlyMigrationFixture(t)
			testCase.configure(t, fixture)
			migrationError := migrateManagedModelSelections(fixture.database, fixture.providerKeyCipher, fixture.providers, managedGemini3OnlySchemaVersion)
			if !errors.Is(migrationError, errManagedTenantSchemaMigration) || !strings.Contains(migrationError.Error(), testCase.want) {
				t.Fatalf("Gemini migration error=%v want=%q", migrationError, testCase.want)
			}
			fixture.assertUnchanged(t)
		})
	}
}
