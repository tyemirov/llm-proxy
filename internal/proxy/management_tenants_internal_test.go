package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestManagedTenantDomainTypes(t *testing.T) {
	validName, validNameError := newManagedTenantName("  Project Alpha  ")
	if validNameError != nil || validName.display != "Project Alpha" || validName.key != "project alpha" {
		t.Fatalf("valid name=%+v error=%v", validName, validNameError)
	}
	for _, value := range []string{"", " ", strings.Repeat("a", managedTenantNameMaximumCharacters+1), "hidden\u200bname", "line\nbreak"} {
		if _, nameError := newManagedTenantName(value); !errors.Is(nameError, errManagedTenantNameInvalid) {
			t.Fatalf("name=%q error=%v", value, nameError)
		}
	}
	for _, value := range []string{"", "has space", "has/slash", "line\nbreak", strings.Repeat("a", managedTenantIDMaximumCharacters+1)} {
		if _, identifierError := newManagedTenantIdentifier(value); !errors.Is(identifierError, errManagedTenantIDInvalid) {
			t.Fatalf("identifier=%q error=%v", value, identifierError)
		}
	}
	identifier, identifierError := newManagedTenantIdentifier("preserved-tenant")
	if identifierError != nil || identifier.string() != "preserved-tenant" {
		t.Fatalf("identifier=%q error=%v", identifier, identifierError)
	}
}

func TestManagedTenantSQLiteOwnershipMigrationPreservesAndRebindsData(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy.db")
	providerKeyCipher := internalManagedProviderKeyCipher()
	providers := internalManagementProviderRegistry()
	legacyDatabase := openLegacyManagedTenantDatabase(t, databasePath)
	fixedTime := time.Date(2026, 7, 25, 12, 30, 0, 0, time.UTC)

	firstSecret := "llmp_first"
	secondSecret := "llmp_second"
	firstDigest := sha256.Sum256([]byte(firstSecret))
	secondDigest := sha256.Sum256([]byte(secondSecret))
	firstTenant := legacyManagedTenantRecord{
		UserID:          "first-user",
		UserEmail:       "first@example.com",
		UserDisplayName: "First",
		UserAvatarURL:   "https://example.com/first.png",
		TenantID:        "preserved-first",
		SecretDigest:    hex.EncodeToString(firstDigest[:]),
		CreatedAt:       fixedTime,
		UpdatedAt:       fixedTime.Add(time.Minute),
	}
	firstTenant.applyDefaults(defaultManagedRoutingDefaults())
	secondTenant := legacyManagedTenantRecord{
		UserID:          "second-user",
		UserEmail:       "second@example.com",
		UserDisplayName: "Second",
		TenantID:        "preserved-second",
		SecretDigest:    hex.EncodeToString(secondDigest[:]),
		CreatedAt:       fixedTime.Add(time.Hour),
		UpdatedAt:       fixedTime.Add(2 * time.Hour),
	}
	secondTenant.applyDefaults(defaultManagedRoutingDefaults())
	if createError := legacyDatabase.Table(managedTenantTable).Create(&[]legacyManagedTenantRecord{firstTenant, secondTenant}).Error; createError != nil {
		t.Fatalf("seed legacy tenants: %v", createError)
	}
	firstCiphertext, firstEncryptionError := providerKeyCipher.encrypt(bytes.NewReader(make([]byte, providerKeyCipher.aeadCipher.NonceSize())), firstTenant.UserID, ProviderNameOpenAI, "sk-first")
	if firstEncryptionError != nil {
		t.Fatalf("encrypt first key: %v", firstEncryptionError)
	}
	secondCiphertext, secondEncryptionError := providerKeyCipher.encrypt(bytes.NewReader(bytes.Repeat([]byte{1}, providerKeyCipher.aeadCipher.NonceSize())), secondTenant.UserID, ProviderNameOpenAI, "sk-second")
	if secondEncryptionError != nil {
		t.Fatalf("encrypt second key: %v", secondEncryptionError)
	}
	legacyProviderKeys := []legacyManagedProviderAPIKeyRecord{
		{UserID: firstTenant.UserID, ProviderID: ProviderNameOpenAI, EncryptedAPIKey: firstCiphertext, TextModel: ModelNameGPT41, SystemPrompt: "first system", CreatedAt: fixedTime, UpdatedAt: fixedTime.Add(time.Minute)},
		{UserID: secondTenant.UserID, ProviderID: ProviderNameOpenAI, EncryptedAPIKey: secondCiphertext, TextModel: ModelNameGPT41, SystemPrompt: "second system", CreatedAt: fixedTime.Add(time.Hour), UpdatedAt: fixedTime.Add(2 * time.Hour)},
	}
	if createError := legacyDatabase.Table(managedProviderKeyTable).Create(&legacyProviderKeys).Error; createError != nil {
		t.Fatalf("seed legacy provider keys: %v", createError)
	}
	legacyUsage := []legacyManagedUsageEventRecord{
		{ID: 11, UserID: firstTenant.UserID, TenantID: firstTenant.TenantID, Endpoint: usageEndpointText, ProviderID: ProviderNameOpenAI, ModelID: ModelNameGPT41, StatusCode: http.StatusOK, Success: true, LatencyMilliseconds: 17, RequestTokens: 2, ResponseTokens: 3, TotalTokens: 5, CreatedAt: fixedTime.Add(3 * time.Hour)},
		{ID: 29, UserID: secondTenant.UserID, TenantID: secondTenant.TenantID, Endpoint: usageEndpointText, ProviderID: ProviderNameOpenAI, ModelID: ModelNameGPT41, StatusCode: http.StatusBadGateway, Success: false, LatencyMilliseconds: 31, CreatedAt: fixedTime.Add(4 * time.Hour)},
	}
	if createError := legacyDatabase.Table(managedUsageEventTable).Create(&legacyUsage).Error; createError != nil {
		t.Fatalf("seed legacy usage: %v", createError)
	}

	database, databaseError := newGORMManagedTenantDatabase(
		ManagementConfiguration{DatabaseDialect: ManagementDatabaseDialectSQLite, DatabaseDSN: databasePath, DatabaseDialector: sqlite.Open(databasePath)},
		providerKeyCipher,
		providers,
	)
	if databaseError != nil {
		t.Fatalf("migrate database: %v", databaseError)
	}
	migrator := database.database.Migrator()
	if managedTableHasColumn(migrator, managedTenantTable, "user_id") || !managedTableHasColumn(migrator, managedTenantTable, "owner_user_id") ||
		managedTableHasColumn(migrator, managedProviderKeyTable, "user_id") || managedTableHasColumn(migrator, managedUsageEventTable, "user_id") {
		tenantColumns, _ := migrator.ColumnTypes(managedTenantTable)
		providerColumns, _ := migrator.ColumnTypes(managedProviderKeyTable)
		usageColumns, _ := migrator.ColumnTypes(managedUsageEventTable)
		t.Fatalf("unexpected ownership columns tenant=%v provider=%v usage=%v", managedColumnNames(tenantColumns), managedColumnNames(providerColumns), managedColumnNames(usageColumns))
	}
	for _, legacyTable := range []string{legacyTenantMigrationTable, legacyProviderKeyMigrationTable, legacyUsageEventMigrationTable} {
		if migrator.HasTable(legacyTable) {
			t.Fatalf("legacy table remains: %s", legacyTable)
		}
	}
	firstRecord, firstRecordError := database.tenantByOwnerAndID(firstTenant.UserID, firstTenant.TenantID)
	if firstRecordError != nil {
		t.Fatalf("load migrated first tenant: %v", firstRecordError)
	}
	if firstRecord.Name != "Default" || firstRecord.OwnerUserID != firstTenant.UserID ||
		managedSecretDigestValue(firstRecord.SecretDigest) != firstTenant.SecretDigest ||
		firstRecord.defaults() != firstTenant.defaults() ||
		!firstRecord.CreatedAt.Equal(firstTenant.CreatedAt) || !firstRecord.UpdatedAt.Equal(firstTenant.UpdatedAt) {
		t.Fatalf("migrated first tenant=%+v", firstRecord)
	}
	if len(firstRecord.ProviderAPIKeys) != 1 || firstRecord.ProviderAPIKeys[0].EncryptedAPIKey == firstCiphertext {
		t.Fatalf("migrated provider key=%+v", firstRecord.ProviderAPIKeys)
	}
	firstAPIKey, firstDecryptionError := providerKeyCipher.decrypt(firstRecord.ProviderAPIKeys[0])
	if firstDecryptionError != nil || firstAPIKey != "sk-first" {
		t.Fatalf("migrated API key=%q error=%v", firstAPIKey, firstDecryptionError)
	}
	if _, oldBindingError := providerKeyCipher.decryptValue(firstRecord.ProviderAPIKeys[0].EncryptedAPIKey, firstTenant.UserID, ProviderNameOpenAI); !errors.Is(oldBindingError, errManagedProviderKeyDecryption) {
		t.Fatalf("new ciphertext accepted old user binding: %v", oldBindingError)
	}
	var usageRecords []managedUsageEventRecord
	if queryError := database.database.Order("id").Find(&usageRecords).Error; queryError != nil {
		t.Fatalf("load migrated usage: %v", queryError)
	}
	if len(usageRecords) != 2 || usageRecords[0].ID != 11 || usageRecords[0].TenantID != firstTenant.TenantID ||
		usageRecords[0].TotalTokens != 5 || usageRecords[1].ID != 29 || usageRecords[1].TenantID != secondTenant.TenantID {
		t.Fatalf("migrated usage=%+v", usageRecords)
	}
	store := newManagedTenantStoreWithDatabaseAndCipher(database, providerKeyCipher)
	store.routingDefaults = providers
	if authenticatedTenant, authenticated := store.authenticate(firstSecret); !authenticated || authenticatedTenant.identifier.string() != firstTenant.TenantID {
		t.Fatalf("preserved secret authenticated=%v tenant=%+v", authenticated, authenticatedTenant)
	}
	if initializeError := initializeManagedTenantSchema(database.database, providerKeyCipher, providers); initializeError != nil {
		t.Fatalf("reopen current schema: %v", initializeError)
	}
}

func TestManagedTenantPostgresOwnershipMigrationPreservesRebindsAndRollsBack(t *testing.T) {
	databaseDSN := strings.TrimSpace(os.Getenv("LLM_PROXY_TEST_POSTGRES_DSN"))
	if databaseDSN == "" {
		t.Skip("LLM_PROXY_TEST_POSTGRES_DSN is required for the disposable PostgreSQL migration scenario")
	}
	providerKeyCipher := internalManagedProviderKeyCipher()
	providers := internalManagementProviderRegistry()
	legacyDatabase := openLegacyManagedTenantDatabaseWithDialector(t, postgres.Open(databaseDSN))
	resetManagedTenantTestTables(t, legacyDatabase)
	legacyDatabase = openLegacyManagedTenantDatabaseWithDialector(t, postgres.Open(databaseDSN))
	fixedTime := time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)
	firstDigest := sha256.Sum256([]byte("llmp_postgres_first"))
	secondDigest := sha256.Sum256([]byte("llmp_postgres_second"))
	firstTenant := legacyManagedTenantRecord{
		UserID: "postgres-first-user", UserEmail: "first@example.com", TenantID: "postgres-first",
		SecretDigest: hex.EncodeToString(firstDigest[:]), CreatedAt: fixedTime, UpdatedAt: fixedTime.Add(time.Minute),
	}
	firstTenant.applyDefaults(defaultManagedRoutingDefaults())
	secondTenant := legacyManagedTenantRecord{
		UserID: "postgres-second-user", UserEmail: "second@example.com", TenantID: "postgres-second",
		SecretDigest: hex.EncodeToString(secondDigest[:]), CreatedAt: fixedTime.Add(time.Hour), UpdatedAt: fixedTime.Add(2 * time.Hour),
	}
	secondTenant.applyDefaults(defaultManagedRoutingDefaults())
	if createError := legacyDatabase.Table(managedTenantTable).Create(&[]legacyManagedTenantRecord{firstTenant, secondTenant}).Error; createError != nil {
		t.Fatalf("seed PostgreSQL legacy tenants: %v", createError)
	}
	firstCiphertext, firstEncryptionError := providerKeyCipher.encrypt(
		bytes.NewReader(make([]byte, providerKeyCipher.aeadCipher.NonceSize())),
		firstTenant.UserID,
		ProviderNameOpenAI,
		"sk-postgres-first",
	)
	if firstEncryptionError != nil {
		t.Fatalf("encrypt PostgreSQL first key: %v", firstEncryptionError)
	}
	secondCiphertext, secondEncryptionError := providerKeyCipher.encrypt(
		bytes.NewReader(bytes.Repeat([]byte{1}, providerKeyCipher.aeadCipher.NonceSize())),
		secondTenant.UserID,
		ProviderNameOpenAI,
		"sk-postgres-second",
	)
	if secondEncryptionError != nil {
		t.Fatalf("encrypt PostgreSQL second key: %v", secondEncryptionError)
	}
	if createError := legacyDatabase.Table(managedProviderKeyTable).Create(&[]legacyManagedProviderAPIKeyRecord{
		{UserID: firstTenant.UserID, ProviderID: ProviderNameOpenAI, EncryptedAPIKey: firstCiphertext, TextModel: ModelNameGPT41, CreatedAt: fixedTime, UpdatedAt: fixedTime.Add(time.Minute)},
		{UserID: secondTenant.UserID, ProviderID: ProviderNameOpenAI, EncryptedAPIKey: secondCiphertext, TextModel: ModelNameGPT41, CreatedAt: fixedTime.Add(time.Hour), UpdatedAt: fixedTime.Add(2 * time.Hour)},
	}).Error; createError != nil {
		t.Fatalf("seed PostgreSQL legacy provider keys: %v", createError)
	}
	if createError := legacyDatabase.Table(managedUsageEventTable).Create(&[]legacyManagedUsageEventRecord{
		{ID: 41, UserID: firstTenant.UserID, TenantID: firstTenant.TenantID, Endpoint: usageEndpointText, ProviderID: ProviderNameOpenAI, ModelID: ModelNameGPT41, StatusCode: http.StatusOK, Success: true, TotalTokens: 8, CreatedAt: fixedTime.Add(3 * time.Hour)},
		{ID: 73, UserID: secondTenant.UserID, TenantID: secondTenant.TenantID, Endpoint: usageEndpointDictation, ProviderID: ProviderNameOpenAI, ModelID: DefaultDictationModel, StatusCode: http.StatusBadGateway, CreatedAt: fixedTime.Add(4 * time.Hour)},
	}).Error; createError != nil {
		t.Fatalf("seed PostgreSQL legacy usage: %v", createError)
	}

	database, databaseError := newGORMManagedTenantDatabase(
		ManagementConfiguration{DatabaseDialect: ManagementDatabaseDialectPostgres, DatabaseDSN: databaseDSN},
		providerKeyCipher,
		providers,
	)
	if databaseError != nil {
		t.Fatalf("migrate PostgreSQL database: %v", databaseError)
	}
	if managedTableHasColumn(database.database.Migrator(), managedTenantTable, "user_id") ||
		!managedTableHasColumn(database.database.Migrator(), managedTenantTable, "owner_user_id") ||
		managedTableHasColumn(database.database.Migrator(), managedProviderKeyTable, "user_id") ||
		managedTableHasColumn(database.database.Migrator(), managedUsageEventTable, "user_id") {
		t.Fatalf("PostgreSQL migration retained obsolete ownership columns")
	}
	for _, expectation := range []struct {
		userID   string
		tenantID string
		apiKey   string
	}{
		{userID: firstTenant.UserID, tenantID: firstTenant.TenantID, apiKey: "sk-postgres-first"},
		{userID: secondTenant.UserID, tenantID: secondTenant.TenantID, apiKey: "sk-postgres-second"},
	} {
		record, queryError := database.tenantByOwnerAndID(expectation.userID, expectation.tenantID)
		if queryError != nil {
			t.Fatalf("load PostgreSQL migrated tenant %s: %v", expectation.tenantID, queryError)
		}
		if record.Name != "Default" || len(record.ProviderAPIKeys) != 1 {
			t.Fatalf("PostgreSQL migrated tenant=%+v", record)
		}
		apiKey, decryptionError := providerKeyCipher.decrypt(record.ProviderAPIKeys[0])
		if decryptionError != nil || apiKey != expectation.apiKey {
			t.Fatalf("PostgreSQL tenant=%s key=%q error=%v", expectation.tenantID, apiKey, decryptionError)
		}
	}
	var usageRecords []managedUsageEventRecord
	if queryError := database.database.Order("id").Find(&usageRecords).Error; queryError != nil {
		t.Fatalf("load PostgreSQL migrated usage: %v", queryError)
	}
	if len(usageRecords) != 2 || usageRecords[0].ID != 41 || usageRecords[0].TotalTokens != 8 || usageRecords[1].ID != 73 {
		t.Fatalf("PostgreSQL migrated usage=%+v", usageRecords)
	}
	if initializeError := initializeManagedTenantSchema(database.database, providerKeyCipher, providers); initializeError != nil {
		t.Fatalf("reopen current PostgreSQL schema: %v", initializeError)
	}

	resetManagedTenantTestTables(t, database.database)
	rollbackDatabase := openLegacyManagedTenantDatabaseWithDialector(t, postgres.Open(databaseDSN))
	rollbackTenant := legacyManagedTenantRecord{
		UserID: "postgres-rollback-user", TenantID: "postgres-rollback", CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
	rollbackTenant.applyDefaults(defaultManagedRoutingDefaults())
	if createError := rollbackDatabase.Table(managedTenantTable).Create(&rollbackTenant).Error; createError != nil {
		t.Fatalf("seed PostgreSQL rollback tenant: %v", createError)
	}
	if createError := rollbackDatabase.Table(managedProviderKeyTable).Create(&legacyManagedProviderAPIKeyRecord{
		UserID: rollbackTenant.UserID, ProviderID: ProviderNameOpenAI, EncryptedAPIKey: "corrupt",
		TextModel: ModelNameGPT41, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}).Error; createError != nil {
		t.Fatalf("seed PostgreSQL rollback provider key: %v", createError)
	}
	_, migrationError := newGORMManagedTenantDatabase(
		ManagementConfiguration{DatabaseDialect: ManagementDatabaseDialectPostgres, DatabaseDSN: databaseDSN},
		providerKeyCipher,
		providers,
	)
	if migrationError == nil || !strings.Contains(migrationError.Error(), errManagedProviderKeyDecryption.Error()) {
		t.Fatalf("PostgreSQL rollback migration error=%v", migrationError)
	}
	rollbackMigrator := rollbackDatabase.Migrator()
	if !managedTableHasColumn(rollbackMigrator, managedTenantTable, "user_id") || rollbackMigrator.HasTable(managedUserTable) {
		t.Fatalf("failed PostgreSQL migration mutated schema")
	}
}

func managedColumnNames(columns []gorm.ColumnType) []string {
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.Name())
	}
	return names
}

func TestManagedTenantSQLiteOwnershipMigrationRollsBackInvalidData(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		mutateSeed func(*legacyManagedTenantRecord, *legacyManagedProviderAPIKeyRecord)
		want       string
	}{
		{
			name: "unclaimed static owner",
			mutateSeed: func(tenant *legacyManagedTenantRecord, _ *legacyManagedProviderAPIKeyRecord) {
				tenant.UserID = legacyStaticTenantUserIDPrefix + tenant.TenantID
			},
			want: "complete the F011 ownership claim first",
		},
		{
			name: "corrupt provider ciphertext",
			mutateSeed: func(_ *legacyManagedTenantRecord, providerKey *legacyManagedProviderAPIKeyRecord) {
				providerKey.EncryptedAPIKey = "corrupt"
			},
			want: errManagedProviderKeyDecryption.Error(),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "rollback.db")
			providerKeyCipher := internalManagedProviderKeyCipher()
			legacyDatabase := openLegacyManagedTenantDatabase(t, databasePath)
			fixedTime := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
			tenantRecord := legacyManagedTenantRecord{UserID: "owner", TenantID: "tenant", CreatedAt: fixedTime, UpdatedAt: fixedTime}
			tenantRecord.applyDefaults(defaultManagedRoutingDefaults())
			encryptedKey, encryptionError := providerKeyCipher.encrypt(bytes.NewReader(make([]byte, providerKeyCipher.aeadCipher.NonceSize())), tenantRecord.UserID, ProviderNameOpenAI, "sk-key")
			if encryptionError != nil {
				t.Fatalf("encrypt key: %v", encryptionError)
			}
			providerKeyRecord := legacyManagedProviderAPIKeyRecord{
				UserID: tenantRecord.UserID, ProviderID: ProviderNameOpenAI, EncryptedAPIKey: encryptedKey,
				TextModel: ModelNameGPT41, CreatedAt: fixedTime, UpdatedAt: fixedTime,
			}
			testCase.mutateSeed(&tenantRecord, &providerKeyRecord)
			providerKeyRecord.UserID = tenantRecord.UserID
			if createError := legacyDatabase.Table(managedTenantTable).Create(&tenantRecord).Error; createError != nil {
				t.Fatalf("seed tenant: %v", createError)
			}
			if createError := legacyDatabase.Table(managedProviderKeyTable).Create(&providerKeyRecord).Error; createError != nil {
				t.Fatalf("seed provider key: %v", createError)
			}

			_, migrationError := newGORMManagedTenantDatabase(
				ManagementConfiguration{DatabaseDialect: ManagementDatabaseDialectSQLite, DatabaseDSN: databasePath, DatabaseDialector: sqlite.Open(databasePath)},
				providerKeyCipher,
				internalManagementProviderRegistry(),
			)
			if migrationError == nil || !strings.Contains(migrationError.Error(), testCase.want) {
				t.Fatalf("migration error=%v want fragment=%q", migrationError, testCase.want)
			}
			rollbackDatabase, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
			if openError != nil {
				t.Fatalf("reopen rollback database: %v", openError)
			}
			if !managedTableHasColumn(rollbackDatabase.Migrator(), managedTenantTable, "user_id") || rollbackDatabase.Migrator().HasTable(managedUserTable) {
				t.Fatalf("failed migration mutated schema")
			}
			var tenantCount int64
			if countError := rollbackDatabase.Table(managedTenantTable).Count(&tenantCount).Error; countError != nil || tenantCount != 1 {
				t.Fatalf("legacy tenant count=%d error=%v", tenantCount, countError)
			}
		})
	}
}

func TestManagedTenantSchemaRejectsUnknownVersion(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "unknown-version.db")
	providerKeyCipher := internalManagedProviderKeyCipher()
	providers := internalManagementProviderRegistry()
	database, databaseError := newGORMManagedTenantDatabase(
		ManagementConfiguration{
			DatabaseDialect:   ManagementDatabaseDialectSQLite,
			DatabaseDSN:       databasePath,
			DatabaseDialector: sqlite.Open(databasePath),
		},
		providerKeyCipher,
		providers,
	)
	if databaseError != nil {
		t.Fatalf("create current schema: %v", databaseError)
	}
	if updateError := database.database.Model(&managedSchemaMigrationRecord{}).
		Where(&managedSchemaMigrationRecord{Version: managedTenantSchemaVersion}).
		Update(managedSchemaVersionColumn, managedTenantSchemaVersion+1).Error; updateError != nil {
		t.Fatalf("write unknown schema version: %v", updateError)
	}
	reopenError := initializeManagedTenantSchema(database.database, providerKeyCipher, providers)
	if reopenError == nil || !strings.Contains(reopenError.Error(), "operation=validate_version") {
		t.Fatalf("reopen error=%v want unknown-version rejection", reopenError)
	}
}

func openLegacyManagedTenantDatabase(t *testing.T, databasePath string) *gorm.DB {
	t.Helper()
	return openLegacyManagedTenantDatabaseWithDialector(t, sqlite.Open(databasePath))
}

func openLegacyManagedTenantDatabaseWithDialector(t *testing.T, dialector gorm.Dialector) *gorm.DB {
	t.Helper()
	database, openError := gorm.Open(dialector, &gorm.Config{})
	if openError != nil {
		t.Fatalf("open legacy database: %v", openError)
	}
	for _, migration := range []struct {
		table string
		model interface{}
	}{
		{table: managedTenantTable, model: &legacyManagedTenantRecord{}},
		{table: managedProviderKeyTable, model: &legacyManagedProviderAPIKeyRecord{}},
		{table: managedUsageEventTable, model: &legacyManagedUsageEventRecord{}},
	} {
		if migrationError := database.Table(migration.table).AutoMigrate(migration.model); migrationError != nil {
			t.Fatalf("create legacy table %s: %v", migration.table, migrationError)
		}
	}
	return database
}

func resetManagedTenantTestTables(t *testing.T, database *gorm.DB) {
	t.Helper()
	for _, tableName := range []string{
		managedProviderKeyTable,
		managedUsageEventTable,
		managedTenantTable,
		managedUserTable,
		managedSchemaMigrationTable,
		legacyProviderKeyMigrationTable,
		legacyUsageEventMigrationTable,
		legacyTenantMigrationTable,
		obsoleteRoutingMigrationTable,
		obsoleteStaticMigrationTable,
	} {
		if database.Migrator().HasTable(tableName) {
			if dropError := database.Migrator().DropTable(tableName); dropError != nil {
				t.Fatalf("drop disposable migration table %s: %v", tableName, dropError)
			}
		}
	}
}

func internalManagementProviderRegistry() *providerRegistry {
	return newProviderRegistry(Configuration{
		OpenAIKey:               "sk-config-openai",
		OpenAITranscriptionsURL: "https://openai.example/transcriptions",
		ProviderModels: ProviderModelCatalogs{
			ProviderNameOpenAI: {
				Text: ModelEndpointCatalog{
					DefaultModel: ModelNameGPT41,
					Models: []ModelConfiguration{
						{ID: ModelNameGPT41},
						{ID: ModelNameGPT55},
					},
				},
				Dictation: ModelEndpointCatalog{
					DefaultModel: DefaultDictationModel,
					Models:       []ModelConfiguration{{ID: DefaultDictationModel}},
				},
			},
		},
	})
}

func (record *legacyManagedTenantRecord) applyDefaults(defaults managedRoutingDefaults) {
	validatedDefaults := defaults.value()
	record.DefaultProvider = validatedDefaults.Provider
	record.DefaultModel = validatedDefaults.Model
	record.DefaultDictationProvider = validatedDefaults.DictationProvider
	record.DefaultDictationModel = validatedDefaults.DictationModel
	record.DefaultSystemPrompt = validatedDefaults.SystemPrompt
	record.DefaultReasoningEffort = validatedDefaults.ReasoningEffort
}
