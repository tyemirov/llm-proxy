package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
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
	for _, rename := range legacyManagedIndexRenames() {
		if !legacyDatabase.Migrator().HasIndex(rename.table, rename.source) {
			t.Fatalf("legacy fixture missing index %s", rename.source)
		}
	}
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
	secondTenant.DefaultProvider = retiredGrokProviderIdentifier
	secondTenant.DefaultModel = ModelNameGrok43
	if createError := legacyDatabase.Table(managedTenantTable).Create(&[]legacyManagedTenantRecord{firstTenant, secondTenant}).Error; createError != nil {
		t.Fatalf("seed legacy tenants: %v", createError)
	}
	firstCiphertext, firstEncryptionError := providerKeyCipher.encrypt(bytes.NewReader(make([]byte, providerKeyCipher.aeadCipher.NonceSize())), firstTenant.UserID, ProviderNameOpenAI, "sk-first")
	if firstEncryptionError != nil {
		t.Fatalf("encrypt first key: %v", firstEncryptionError)
	}
	secondCiphertext, secondEncryptionError := providerKeyCipher.encrypt(bytes.NewReader(bytes.Repeat([]byte{1}, providerKeyCipher.aeadCipher.NonceSize())), secondTenant.UserID, retiredGrokProviderIdentifier, "sk-second")
	if secondEncryptionError != nil {
		t.Fatalf("encrypt second key: %v", secondEncryptionError)
	}
	dashScopeCiphertext, dashScopeEncryptionError := providerKeyCipher.encrypt(bytes.NewReader(bytes.Repeat([]byte{2}, providerKeyCipher.aeadCipher.NonceSize())), firstTenant.UserID, ProviderNameDashScope, "sk-dashscope")
	if dashScopeEncryptionError != nil {
		t.Fatalf("encrypt DashScope key: %v", dashScopeEncryptionError)
	}
	legacyProviderKeys := []legacyManagedProviderAPIKeyRecord{
		{UserID: firstTenant.UserID, ProviderID: ProviderNameOpenAI, EncryptedAPIKey: firstCiphertext, TextModel: ModelNameGPT41, SystemPrompt: "first system", CreatedAt: fixedTime, UpdatedAt: fixedTime.Add(time.Minute)},
		{UserID: firstTenant.UserID, ProviderID: ProviderNameDashScope, EncryptedAPIKey: dashScopeCiphertext, TextModel: ModelNameDashScopeQwenPlus, SystemPrompt: "incomplete workspace settings", CreatedAt: fixedTime, UpdatedAt: fixedTime.Add(time.Minute)},
		{UserID: secondTenant.UserID, ProviderID: retiredGrokProviderIdentifier, EncryptedAPIKey: secondCiphertext, TextModel: ModelNameGrok43, SystemPrompt: "second system", CreatedAt: fixedTime.Add(time.Hour), UpdatedAt: fixedTime.Add(2 * time.Hour)},
	}
	if createError := legacyDatabase.Table(managedProviderKeyTable).Create(&legacyProviderKeys).Error; createError != nil {
		t.Fatalf("seed legacy provider keys: %v", createError)
	}
	legacyUsage := []legacyManagedUsageEventRecord{
		{ID: 11, UserID: firstTenant.UserID, TenantID: firstTenant.TenantID, Endpoint: usageEndpointText, ProviderID: ProviderNameOpenAI, ModelID: ModelNameGPT41, StatusCode: http.StatusOK, Success: true, LatencyMilliseconds: 17, RequestTokens: 2, ResponseTokens: 3, TotalTokens: 5, CreatedAt: fixedTime.Add(3 * time.Hour)},
		{ID: 29, UserID: secondTenant.UserID, TenantID: secondTenant.TenantID, Endpoint: usageEndpointText, ProviderID: retiredGrokProviderIdentifier, ModelID: ModelNameGrok43, StatusCode: http.StatusBadGateway, Success: false, LatencyMilliseconds: 31, CreatedAt: fixedTime.Add(4 * time.Hour)},
		{ID: 41, UserID: firstTenant.UserID, TenantID: firstTenant.TenantID, Endpoint: usageEndpointText, ProviderID: ProviderNameOpenAI, ModelID: ModelNameGPT41, StatusCode: statusClientClosedRequest, Success: false, LatencyMilliseconds: 7, CreatedAt: fixedTime.Add(5 * time.Hour)},
	}
	if createError := legacyDatabase.Table(managedUsageEventTable).Create(&legacyUsage).Error; createError != nil {
		t.Fatalf("seed legacy usage: %v", createError)
	}

	database, databaseError := newGORMManagedTenantDatabase(
		ManagementConfiguration{DatabasePath: databasePath, DatabaseDialector: sqlite.Open(databasePath)},
		providerKeyCipher,
		providers,
	)
	if databaseError != nil {
		t.Fatalf("migrate database: %v", databaseError)
	}
	migrator := database.database.Migrator()
	if managedTableHasColumn(migrator, managedTenantTable, "user_id") || !managedTableHasColumn(migrator, managedTenantTable, "owner_user_id") ||
		migrator.HasTable(managedProviderKeyTable) || managedTableHasColumn(migrator, managedUsageEventTable, "user_id") {
		tenantColumns, _ := migrator.ColumnTypes(managedTenantTable)
		providerColumns, _ := migrator.ColumnTypes(managedProviderConnectionTable)
		usageColumns, _ := migrator.ColumnTypes(managedUsageEventTable)
		t.Fatalf("unexpected ownership columns tenant=%v provider=%v usage=%v", managedColumnNames(tenantColumns), managedColumnNames(providerColumns), managedColumnNames(usageColumns))
	}
	for _, rename := range legacyManagedIndexRenames() {
		if !migrator.HasIndex(rename.table, rename.source) {
			t.Fatalf("current schema missing index %s", rename.source)
		}
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
	expectedMigratedDefaults := DefaultTenantDefaults()
	if firstRecord.Name != "Default" || firstRecord.OwnerUserID != firstTenant.UserID ||
		managedSecretDigestValue(firstRecord.SecretDigest) != firstTenant.SecretDigest ||
		firstRecord.defaults() != expectedMigratedDefaults ||
		!firstRecord.CreatedAt.Equal(firstTenant.CreatedAt) || !firstRecord.UpdatedAt.Equal(firstTenant.UpdatedAt) {
		t.Fatalf("migrated first tenant=%+v", firstRecord)
	}
	if len(firstRecord.ProviderConnections) != 1 || len(firstRecord.ProviderProfiles) != 1 || firstRecord.ProviderConnections[0].Value == firstCiphertext {
		t.Fatalf("migrated provider connections=%+v profiles=%+v", firstRecord.ProviderConnections, firstRecord.ProviderProfiles)
	}
	firstAPIKey, firstDecryptionError := providerKeyCipher.decryptConnection(firstRecord.ProviderConnections[0])
	if firstDecryptionError != nil || firstAPIKey != "sk-first" {
		t.Fatalf("migrated API key=%q error=%v", firstAPIKey, firstDecryptionError)
	}
	if _, oldBindingError := providerKeyCipher.decryptValue(firstRecord.ProviderConnections[0].Value, firstTenant.UserID, ProviderNameOpenAI); !errors.Is(oldBindingError, errManagedProviderKeyDecryption) {
		t.Fatalf("new ciphertext accepted old user binding: %v", oldBindingError)
	}
	var usageRecords []managedUsageEventRecord
	if queryError := database.database.Order("id").Find(&usageRecords).Error; queryError != nil {
		t.Fatalf("load migrated usage: %v", queryError)
	}
	if len(usageRecords) != 3 || usageRecords[0].ID != 11 || usageRecords[0].TenantID != firstTenant.TenantID ||
		usageRecords[0].TotalTokens != 5 || usageRecords[1].ID != 29 || usageRecords[1].TenantID != secondTenant.TenantID ||
		usageRecords[2].ID != 41 || usageRecords[2].TenantID != firstTenant.TenantID ||
		usageRecords[2].StatusCode != statusClientClosedRequest ||
		usageRecords[2].OutcomeCode != managedUsageOutcomeRequestTimeout {
		t.Fatalf("migrated usage=%+v", usageRecords)
	}
	store := newManagedTenantStoreWithDatabaseAndCipher(database, providerKeyCipher)
	store.routingDefaults = providers
	if authenticatedTenant, authenticated := store.authenticate(context.Background(), firstSecret); !authenticated || authenticatedTenant.identifier.string() != firstTenant.TenantID {
		t.Fatalf("preserved secret authenticated=%v tenant=%+v", authenticated, authenticatedTenant)
	}
	if initializeError := initializeManagedTenantSchema(database.database, providerKeyCipher, providers); initializeError != nil {
		t.Fatalf("reopen current schema: %v", initializeError)
	}
}

func TestManagedTenantSQLiteOwnershipMigrationCanonicalizesConfirmedRouteIdentities(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy-native-models.db")
	providerKeyCipher := internalManagedProviderKeyCipher()
	providers := internalManagementProviderRegistry()
	legacyDatabase := openLegacyManagedTenantDatabase(t, databasePath)
	fixedTime := time.Date(2026, 8, 11, 7, 0, 0, 0, time.UTC)
	legacyTenant := legacyManagedTenantRecord{
		UserID: "native-model-owner", TenantID: "native-model-tenant",
		DefaultProvider: ProviderNameMiniMax, DefaultModel: managedMiniMaxNativeModel,
		DefaultDictationProvider: ProviderNameSiliconFlow, DefaultDictationModel: managedSenseVoiceNativeModel,
		DefaultSystemPrompt: "preserve native prompt", CreatedAt: fixedTime, UpdatedAt: fixedTime.Add(time.Minute),
	}
	if createError := legacyDatabase.Table(managedTenantTable).Create(&legacyTenant).Error; createError != nil {
		t.Fatalf("seed native legacy tenant: %v", createError)
	}
	encryptLegacyProviderKey := func(providerIdentifier string, rawAPIKey string, nonceByte byte) string {
		t.Helper()
		encryptedKey, encryptionError := providerKeyCipher.encrypt(
			bytes.NewReader(bytes.Repeat([]byte{nonceByte}, providerKeyCipher.aeadCipher.NonceSize())),
			legacyTenant.UserID,
			providerIdentifier,
			rawAPIKey,
		)
		if encryptionError != nil {
			t.Fatalf("encrypt native legacy provider=%s: %v", providerIdentifier, encryptionError)
		}
		return encryptedKey
	}
	legacyProviderKeys := []legacyManagedProviderAPIKeyRecord{
		{UserID: legacyTenant.UserID, ProviderID: ProviderNameMiniMax, EncryptedAPIKey: encryptLegacyProviderKey(ProviderNameMiniMax, "sk-minimax", 1), TextModel: managedMiniMaxNativeModel, CreatedAt: fixedTime, UpdatedAt: fixedTime},
		{UserID: legacyTenant.UserID, ProviderID: ProviderNameSiliconFlow, EncryptedAPIKey: encryptLegacyProviderKey(ProviderNameSiliconFlow, "sk-siliconflow", 2), TextModel: managedSiliconFlowDeepSeekNativeModel, CreatedAt: fixedTime, UpdatedAt: fixedTime},
	}
	if createError := legacyDatabase.Table(managedProviderKeyTable).Create(&legacyProviderKeys).Error; createError != nil {
		t.Fatalf("seed native legacy provider keys: %v", createError)
	}
	legacyUsage := legacyManagedUsageEventRecord{
		ID: 51, UserID: legacyTenant.UserID, TenantID: legacyTenant.TenantID, Endpoint: usageEndpointText,
		ProviderID: ProviderNameMiniMax, ModelID: managedMiniMaxNativeModel,
		StatusCode: http.StatusOK, Success: true, CreatedAt: fixedTime.Add(2 * time.Minute),
	}
	if createError := legacyDatabase.Table(managedUsageEventTable).Create(&legacyUsage).Error; createError != nil {
		t.Fatalf("seed native legacy usage: %v", createError)
	}

	database, databaseError := newGORMManagedTenantDatabase(
		ManagementConfiguration{DatabasePath: databasePath, DatabaseDialector: sqlite.Open(databasePath)},
		providerKeyCipher,
		providers,
	)
	if databaseError != nil {
		t.Fatalf("migrate native legacy database: %v", databaseError)
	}
	migratedTenant, tenantError := database.tenantByOwnerAndID(legacyTenant.UserID, legacyTenant.TenantID)
	if tenantError != nil {
		t.Fatalf("load migrated native legacy tenant: %v", tenantError)
	}
	expectedDefaults := TenantDefaults{
		Provider: ProviderNameMiniMax, Model: ModelNameMiniMaxM27,
		DictationProvider: ProviderNameSiliconFlow, DictationModel: "sensevoice-small",
		SystemPrompt: "preserve native prompt",
	}
	if migratedTenant.defaults() != expectedDefaults || len(migratedTenant.ProviderConnections) != 2 || len(migratedTenant.ProviderProfiles) != 2 || !migratedTenant.UpdatedAt.Equal(legacyTenant.UpdatedAt) {
		t.Fatalf("migrated native legacy tenant=%+v", migratedTenant)
	}
	modelsByProvider := map[string]string{}
	for _, profile := range migratedTenant.ProviderProfiles {
		modelsByProvider[profile.ProviderID] = profile.TextModel
	}
	if modelsByProvider[ProviderNameMiniMax] != ModelNameMiniMaxM27 || modelsByProvider[ProviderNameSiliconFlow] != ModelNameSiliconFlowDeepSeek {
		t.Fatalf("migrated native legacy provider models=%v", modelsByProvider)
	}
	var migratedUsage managedUsageEventRecord
	if queryError := database.database.First(&migratedUsage, legacyUsage.ID).Error; queryError != nil || migratedUsage.ProviderID != legacyUsage.ProviderID || migratedUsage.ModelID != legacyUsage.ModelID {
		t.Fatalf("migrated native legacy usage=%+v error=%v", migratedUsage, queryError)
	}
	if initializeError := initializeManagedTenantSchema(database.database, providerKeyCipher, providers); initializeError != nil {
		t.Fatalf("reopen native legacy schema: %v", initializeError)
	}
}

func TestManagedTenantKeyedRoutingDefaultsMigrationReconcilesExistingTenants(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "keyed-routing-defaults.db")
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open SQLite fixture: %v", openError)
	}
	sqlDatabase, sqlDatabaseError := database.DB()
	if sqlDatabaseError != nil {
		t.Fatalf("resolve SQLite fixture: %v", sqlDatabaseError)
	}
	t.Cleanup(func() {
		if closeError := sqlDatabase.Close(); closeError != nil {
			t.Errorf("close SQLite fixture: %v", closeError)
		}
	})
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create schema two fixture: %v", migrationError)
	}
	providers := internalManagementProviderRegistry()
	providerKeyCipher := internalManagedProviderKeyCipher()
	now := time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC)
	userRecord := managedUserRecord{UserID: "keyed-default-owner", CreatedAt: now, UpdatedAt: now}
	if createError := database.Create(&userRecord).Error; createError != nil {
		t.Fatalf("seed user: %v", createError)
	}
	legacyDefaults, defaultsError := newManagedRoutingDefaults(providers, DefaultTenantDefaults())
	if defaultsError != nil {
		t.Fatalf("legacy defaults: %v", defaultsError)
	}
	tenantRecord := managedTenantRecord{
		TenantID:    "keyed-default-tenant",
		OwnerUserID: userRecord.UserID,
		Name:        "Default",
		NameKey:     "default",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	tenantRecord.applyRoutingDefaults(legacyDefaults)
	if createError := database.Create(&tenantRecord).Error; createError != nil {
		t.Fatalf("seed tenant: %v", createError)
	}
	encryptedKey, encryptionError := providerKeyCipher.encrypt(
		bytes.NewReader(bytes.Repeat([]byte{7}, providerKeyCipher.aeadCipher.NonceSize())),
		tenantRecord.TenantID,
		ProviderNameDeepSeek,
		"sk-deepseek",
	)
	if encryptionError != nil {
		t.Fatalf("encrypt provider key: %v", encryptionError)
	}
	providerKeyRecord := managedProviderAPIKeyRecord{
		TenantID:        tenantRecord.TenantID,
		ProviderID:      ProviderNameDeepSeek,
		EncryptedAPIKey: encryptedKey,
		TextModel:       ModelNameDeepSeekV4Flash,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if createError := database.Create(&providerKeyRecord).Error; createError != nil {
		t.Fatalf("seed provider key: %v", createError)
	}
	if createError := database.Create(&managedSchemaMigrationRecord{Version: managedUsageOutcomeSchemaVersion, AppliedAt: now}).Error; createError != nil {
		t.Fatalf("seed schema version: %v", createError)
	}

	if migrationError := initializeManagedTenantSchema(database, providerKeyCipher, providers); migrationError != nil {
		t.Fatalf("migrate keyed defaults: %v", migrationError)
	}
	var migratedTenant managedTenantRecord
	if queryError := database.Preload("ProviderConnections").Preload("ProviderProfiles").Where(&managedTenantRecord{TenantID: tenantRecord.TenantID}).First(&migratedTenant).Error; queryError != nil {
		t.Fatalf("load migrated tenant: %v", queryError)
	}
	expectedDefaults := TenantDefaults{
		Provider: ProviderNameDeepSeek,
		Model:    ModelNameDeepSeekV4Flash,
	}
	if migratedTenant.defaults() != expectedDefaults || !migratedTenant.UpdatedAt.Equal(now) {
		t.Fatalf("migrated defaults=%+v updated_at=%s", migratedTenant.defaults(), migratedTenant.UpdatedAt)
	}
	var latest managedSchemaMigrationRecord
	if queryError := database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedTenantSchemaVersion {
		t.Fatalf("latest version=%+v error=%v", latest, queryError)
	}
	if validationError := initializeManagedTenantSchema(database, providerKeyCipher, providers); validationError != nil {
		t.Fatalf("reopen keyed defaults schema: %v", validationError)
	}
}

func TestManagedTenantModelIdentityMigrationCanonicalizesCurrentRoutesAndPreservesUsage(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "model-identity.db")
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open SQLite fixture: %v", openError)
	}
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create schema four fixture: %v", migrationError)
	}
	providers := internalManagementProviderRegistry()
	providerKeyCipher := internalManagedProviderKeyCipher()
	now := time.Date(2026, 8, 10, 23, 0, 0, 0, time.UTC)
	users := []managedUserRecord{
		{UserID: "native-model-owner", CreatedAt: now, UpdatedAt: now},
		{UserID: "canonical-model-owner", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
	}
	if createError := database.Create(&users).Error; createError != nil {
		t.Fatalf("seed users: %v", createError)
	}
	nativeTenant := fakeTenantRecord(users[0].UserID, "native-model-tenant", "Default", now)
	nativeTenant.UpdatedAt = now.Add(2 * time.Minute)
	nativeTenant.applyRoutingDefaults(managedRoutingDefaults{tenantDefaults: TenantDefaults{
		Provider: ProviderNameMiniMax, Model: managedMiniMaxNativeModel,
		DictationProvider: ProviderNameSiliconFlow, DictationModel: managedSenseVoiceNativeModel,
		SystemPrompt: "preserve native tenant prompt",
	}})
	canonicalTenant := fakeTenantRecord(users[1].UserID, "canonical-model-tenant", "Default", now.Add(time.Minute))
	canonicalTenant.applyRoutingDefaults(managedRoutingDefaults{tenantDefaults: TenantDefaults{
		Provider: ProviderNameDeepSeek, Model: ModelNameDeepSeekV4Flash,
		SystemPrompt: "preserve canonical tenant prompt",
	}})
	if createError := database.Create(&[]managedTenantRecord{nativeTenant, canonicalTenant}).Error; createError != nil {
		t.Fatalf("seed tenants: %v", createError)
	}
	encryptProviderKey := func(tenantID string, providerIdentifier string, rawAPIKey string, nonceByte byte) string {
		t.Helper()
		encryptedKey, encryptionError := providerKeyCipher.encrypt(
			bytes.NewReader(bytes.Repeat([]byte{nonceByte}, providerKeyCipher.aeadCipher.NonceSize())),
			tenantID,
			providerIdentifier,
			rawAPIKey,
		)
		if encryptionError != nil {
			t.Fatalf("encrypt tenant=%s provider=%s: %v", tenantID, providerIdentifier, encryptionError)
		}
		return encryptedKey
	}
	providerKeys := []managedProviderAPIKeyRecord{
		{TenantID: nativeTenant.TenantID, ProviderID: ProviderNameMiniMax, EncryptedAPIKey: encryptProviderKey(nativeTenant.TenantID, ProviderNameMiniMax, "sk-minimax", 1), TextModel: managedMiniMaxNativeModel, CreatedAt: now, UpdatedAt: now},
		{TenantID: nativeTenant.TenantID, ProviderID: ProviderNameSiliconFlow, EncryptedAPIKey: encryptProviderKey(nativeTenant.TenantID, ProviderNameSiliconFlow, "sk-siliconflow", 2), TextModel: managedSiliconFlowDeepSeekNativeModel, CreatedAt: now, UpdatedAt: now},
		{TenantID: canonicalTenant.TenantID, ProviderID: ProviderNameDeepSeek, EncryptedAPIKey: encryptProviderKey(canonicalTenant.TenantID, ProviderNameDeepSeek, "sk-deepseek", 3), TextModel: ModelNameDeepSeekV4Flash, CreatedAt: now, UpdatedAt: now},
	}
	if createError := database.Create(&providerKeys).Error; createError != nil {
		t.Fatalf("seed provider keys: %v", createError)
	}
	historicalUsage := []managedUsageEventRecord{
		{ID: 81, TenantID: nativeTenant.TenantID, Endpoint: usageEndpointText, ProviderID: ProviderNameMiniMax, ModelID: managedMiniMaxNativeModel, StatusCode: http.StatusOK, Success: true, OutcomeCode: managedUsageOutcomeSuccess, CreatedAt: now.Add(3 * time.Minute)},
		{ID: 82, TenantID: nativeTenant.TenantID, Endpoint: usageEndpointText, ProviderID: ProviderNameSiliconFlow, ModelID: managedSiliconFlowDeepSeekNativeModel, StatusCode: http.StatusOK, Success: true, OutcomeCode: managedUsageOutcomeSuccess, CreatedAt: now.Add(4 * time.Minute)},
		{ID: 83, TenantID: nativeTenant.TenantID, Endpoint: usageEndpointDictation, ProviderID: ProviderNameSiliconFlow, ModelID: managedSenseVoiceNativeModel, StatusCode: http.StatusOK, Success: true, OutcomeCode: managedUsageOutcomeSuccess, CreatedAt: now.Add(5 * time.Minute)},
	}
	if createError := database.Create(&historicalUsage).Error; createError != nil {
		t.Fatalf("seed historical usage: %v", createError)
	}
	if createError := database.Create(&managedSchemaMigrationRecord{Version: managedQwenCloudRetirementVersion, AppliedAt: now}).Error; createError != nil {
		t.Fatalf("seed schema version: %v", createError)
	}

	if migrationError := initializeManagedTenantSchema(database, providerKeyCipher, providers); migrationError != nil {
		t.Fatalf("migrate model identity: %v", migrationError)
	}
	var migratedTenants []managedTenantRecord
	if queryError := database.Preload("ProviderConnections").Preload("ProviderProfiles").Order("tenant_id").Find(&migratedTenants).Error; queryError != nil {
		t.Fatalf("load migrated tenants: %v", queryError)
	}
	if len(migratedTenants) != 2 {
		t.Fatalf("migrated tenants=%+v", migratedTenants)
	}
	migratedCanonicalTenant := migratedTenants[0]
	migratedNativeTenant := migratedTenants[1]
	expectedNativeDefaults := TenantDefaults{
		Provider: ProviderNameMiniMax, Model: ModelNameMiniMaxM27,
		DictationProvider: ProviderNameSiliconFlow, DictationModel: "sensevoice-small",
		SystemPrompt: "preserve native tenant prompt",
	}
	if migratedNativeTenant.defaults() != expectedNativeDefaults || !migratedNativeTenant.UpdatedAt.Equal(nativeTenant.UpdatedAt) || len(migratedNativeTenant.ProviderConnections) != 2 || len(migratedNativeTenant.ProviderProfiles) != 2 {
		t.Fatalf("migrated native tenant=%+v connections=%+v profiles=%+v", migratedNativeTenant, migratedNativeTenant.ProviderConnections, migratedNativeTenant.ProviderProfiles)
	}
	modelsByProvider := map[string]string{}
	for _, profile := range migratedNativeTenant.ProviderProfiles {
		modelsByProvider[profile.ProviderID] = profile.TextModel
	}
	if modelsByProvider[ProviderNameMiniMax] != ModelNameMiniMaxM27 || modelsByProvider[ProviderNameSiliconFlow] != ModelNameSiliconFlowDeepSeek {
		t.Fatalf("migrated provider models=%v", modelsByProvider)
	}
	if migratedCanonicalTenant.defaults() != canonicalTenant.defaults() || len(migratedCanonicalTenant.ProviderConnections) != 1 || len(migratedCanonicalTenant.ProviderProfiles) != 1 || migratedCanonicalTenant.ProviderProfiles[0].TextModel != ModelNameDeepSeekV4Flash {
		t.Fatalf("canonical tenant changed=%+v", migratedCanonicalTenant)
	}
	var migratedUsage []managedUsageEventRecord
	if queryError := database.Order("id").Find(&migratedUsage).Error; queryError != nil || !slices.Equal(migratedUsage, historicalUsage) {
		t.Fatalf("historical usage=%+v error=%v", migratedUsage, queryError)
	}
	var latest managedSchemaMigrationRecord
	if queryError := database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedTenantSchemaVersion {
		t.Fatalf("latest version=%+v error=%v", latest, queryError)
	}
	if validationError := initializeManagedTenantSchema(database, providerKeyCipher, providers); validationError != nil {
		t.Fatalf("reopen canonical model schema: %v", validationError)
	}
}

func TestManagedTenantQwenCloudRetirementMigrationReconcilesCurrentTenants(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "qwen-cloud-retirement.db")
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open SQLite fixture: %v", openError)
	}
	if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
		t.Fatalf("create schema three fixture: %v", migrationError)
	}
	providers := internalManagementProviderRegistry()
	providerKeyCipher := internalManagedProviderKeyCipher()
	now := time.Date(2026, 8, 10, 20, 0, 0, 0, time.UTC)
	qwenOnlySecret := "llmp_qwen_only"
	qwenOnlySecretDigest := sha256.Sum256([]byte(qwenOnlySecret))
	qwenOnlySecretDigestText := hex.EncodeToString(qwenOnlySecretDigest[:])
	users := []managedUserRecord{
		{UserID: "qwen-only-owner", CreatedAt: now, UpdatedAt: now},
		{UserID: "mixed-owner", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
	}
	if createError := database.Create(&users).Error; createError != nil {
		t.Fatalf("seed users: %v", createError)
	}
	qwenOnlyTenant := managedTenantRecord{
		TenantID:     "qwen-only-tenant",
		OwnerUserID:  users[0].UserID,
		Name:         "Default",
		NameKey:      "default",
		SecretDigest: &qwenOnlySecretDigestText,
		CreatedAt:    now,
		UpdatedAt:    now.Add(2 * time.Minute),
	}
	qwenOnlyTenant.applyRoutingDefaults(managedRoutingDefaults{tenantDefaults: TenantDefaults{
		Provider:          retiredQwenCloudProviderIdentifier,
		Model:             retiredQwenCloudModelIdentifier,
		SystemPrompt:      "retain tenant prompt",
		ReasoningEffort:   "high",
		DictationProvider: "",
		DictationModel:    "",
	}})
	mixedTenant := managedTenantRecord{
		TenantID:    "mixed-tenant",
		OwnerUserID: users[1].UserID,
		Name:        "Default",
		NameKey:     "default",
		CreatedAt:   now.Add(time.Minute),
		UpdatedAt:   now.Add(3 * time.Minute),
	}
	mixedTenant.applyRoutingDefaults(managedRoutingDefaults{tenantDefaults: TenantDefaults{
		Provider:        retiredQwenCloudProviderIdentifier,
		Model:           retiredQwenCloudModelIdentifier,
		SystemPrompt:    "retain mixed tenant prompt",
		ReasoningEffort: "high",
	}})
	tenants := []managedTenantRecord{qwenOnlyTenant, mixedTenant}
	if createError := database.Create(&tenants).Error; createError != nil {
		t.Fatalf("seed tenants: %v", createError)
	}
	encryptProviderKey := func(tenantID string, providerIdentifier string, rawAPIKey string, nonceByte byte) string {
		t.Helper()
		encryptedKey, encryptionError := providerKeyCipher.encrypt(
			bytes.NewReader(bytes.Repeat([]byte{nonceByte}, providerKeyCipher.aeadCipher.NonceSize())),
			tenantID,
			providerIdentifier,
			rawAPIKey,
		)
		if encryptionError != nil {
			t.Fatalf("encrypt tenant=%s provider=%s: %v", tenantID, providerIdentifier, encryptionError)
		}
		return encryptedKey
	}
	providerKeys := []managedProviderAPIKeyRecord{
		{TenantID: qwenOnlyTenant.TenantID, ProviderID: retiredQwenCloudProviderIdentifier, EncryptedAPIKey: encryptProviderKey(qwenOnlyTenant.TenantID, retiredQwenCloudProviderIdentifier, "sk-qwen-only", 1), TextModel: retiredQwenCloudModelIdentifier, SystemPrompt: "delete qwen-only provider prompt", CreatedAt: now, UpdatedAt: now},
		{TenantID: mixedTenant.TenantID, ProviderID: retiredQwenCloudProviderIdentifier, EncryptedAPIKey: encryptProviderKey(mixedTenant.TenantID, retiredQwenCloudProviderIdentifier, "sk-qwen-mixed", 2), TextModel: retiredQwenCloudModelIdentifier, SystemPrompt: "delete mixed provider prompt", CreatedAt: now, UpdatedAt: now},
		{TenantID: mixedTenant.TenantID, ProviderID: ProviderNameDeepSeek, EncryptedAPIKey: encryptProviderKey(mixedTenant.TenantID, ProviderNameDeepSeek, "sk-deepseek", 3), TextModel: ModelNameDeepSeekV4Flash, SystemPrompt: "retain deepseek prompt", CreatedAt: now, UpdatedAt: now},
		{TenantID: mixedTenant.TenantID, ProviderID: ProviderNameDashScope, EncryptedAPIKey: encryptProviderKey(mixedTenant.TenantID, ProviderNameDashScope, "sk-dashscope", 4), TextModel: ModelNameDashScopeQwenPlus, SystemPrompt: "retain dashscope prompt", CreatedAt: now, UpdatedAt: now},
	}
	if createError := database.Create(&providerKeys).Error; createError != nil {
		t.Fatalf("seed provider keys: %v", createError)
	}
	historicalUsage := managedUsageEventRecord{
		ID: 41, TenantID: qwenOnlyTenant.TenantID, Endpoint: usageEndpointText,
		ProviderID: retiredQwenCloudProviderIdentifier, ModelID: retiredQwenCloudModelIdentifier,
		StatusCode: http.StatusOK, Success: true, OutcomeCode: managedUsageOutcomeSuccess,
		LatencyMilliseconds: 19, RequestTokens: 2, ResponseTokens: 3, TotalTokens: 5, CreatedAt: now.Add(4 * time.Minute),
	}
	if createError := database.Create(&historicalUsage).Error; createError != nil {
		t.Fatalf("seed historical usage: %v", createError)
	}
	if createError := database.Create(&managedSchemaMigrationRecord{Version: managedKeyedRoutingSchemaVersion, AppliedAt: now}).Error; createError != nil {
		t.Fatalf("seed schema version: %v", createError)
	}

	if migrationError := initializeManagedTenantSchema(database, providerKeyCipher, providers); migrationError != nil {
		t.Fatalf("migrate qwen cloud retirement: %v", migrationError)
	}
	var migratedTenants []managedTenantRecord
	if queryError := database.Preload("ProviderConnections").Preload("ProviderProfiles").Order("tenant_id").Find(&migratedTenants).Error; queryError != nil {
		t.Fatalf("load migrated tenants: %v", queryError)
	}
	if len(migratedTenants) != 2 {
		t.Fatalf("migrated tenants=%+v", migratedTenants)
	}
	migratedMixed := migratedTenants[0]
	migratedQwenOnly := migratedTenants[1]
	expectedMixedDefaults := TenantDefaults{
		Provider: ProviderNameDeepSeek, Model: ModelNameDeepSeekV4Flash,
		SystemPrompt: "retain mixed tenant prompt",
	}
	if migratedMixed.defaults() != expectedMixedDefaults || !migratedMixed.UpdatedAt.Equal(mixedTenant.UpdatedAt) || len(migratedMixed.ProviderConnections) != 1 || len(migratedMixed.ProviderProfiles) != 1 {
		t.Fatalf("migrated mixed tenant=%+v connections=%+v profiles=%+v", migratedMixed, migratedMixed.ProviderConnections, migratedMixed.ProviderProfiles)
	}
	expectedQwenOnlyDefaults := TenantDefaults{SystemPrompt: "retain tenant prompt"}
	if migratedQwenOnly.defaults() != expectedQwenOnlyDefaults || !migratedQwenOnly.UpdatedAt.Equal(qwenOnlyTenant.UpdatedAt) || len(migratedQwenOnly.ProviderConnections) != 0 || len(migratedQwenOnly.ProviderProfiles) != 0 {
		t.Fatalf("migrated qwen-only tenant=%+v connections=%+v profiles=%+v", migratedQwenOnly, migratedQwenOnly.ProviderConnections, migratedQwenOnly.ProviderProfiles)
	}
	var migratedUsage managedUsageEventRecord
	if queryError := database.First(&migratedUsage, historicalUsage.ID).Error; queryError != nil || migratedUsage != historicalUsage {
		t.Fatalf("historical usage=%+v error=%v", migratedUsage, queryError)
	}
	var latest managedSchemaMigrationRecord
	if queryError := database.Order("version DESC").First(&latest).Error; queryError != nil || latest.Version != managedTenantSchemaVersion {
		t.Fatalf("latest version=%+v error=%v", latest, queryError)
	}
	if validationError := initializeManagedTenantSchema(database, providerKeyCipher, providers); validationError != nil {
		t.Fatalf("reopen retired qwen schema: %v", validationError)
	}

	upstreamRequestCount := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamRequestCount++
	}))
	defer upstreamServer.Close()
	managedDatabase := &gormManagedTenantDatabase{database: database}
	store := newManagedTenantStoreWithDatabaseAndCipher(managedDatabase, providerKeyCipher)
	timeoutPolicy, timeoutPolicyError := newRequestTimeoutPolicy(5, 5)
	if timeoutPolicyError != nil {
		t.Fatalf("request timeout policy: %v", timeoutPolicyError)
	}
	endpoints := NewEndpoints()
	endpoints.SetResponsesURL(upstreamServer.URL)
	configuration := Configuration{
		Management:  ManagementConfiguration{},
		WorkerCount: 1, QueueSize: 1, MaxPromptBytes: 1024,
		Endpoints: endpoints, ProviderCatalog: internalTestProviderCatalog(internalManagedUsageWriterProviderModels()), ModelCatalog: internalManagedUsageWriterProviderModels(),
		upstreamRateLimits:   upstreamRateLimits{rules: map[string]upstreamRateLimitRule{}},
		requestTimeoutPolicy: timeoutPolicy, validated: true,
	}
	router, buildError := buildRouter(configuration, zap.NewNop().Sugar(), func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
		store.routingDefaults = newProviderRegistry(configuration)
		return store, nil
	})
	if buildError != nil {
		t.Fatalf("build migrated router: %v", buildError)
	}
	request := httptest.NewRequest(http.MethodGet, "/?key="+qwenOnlySecret+"&provider=openai&model="+ModelNameGPT41+"&prompt=hello", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || upstreamRequestCount != 0 || !strings.Contains(response.Body.String(), ErrProviderNotConfigured.Error()) {
		t.Fatalf("migrated qwen-only route status=%d body=%q upstream_requests=%d", response.Code, response.Body.String(), upstreamRequestCount)
	}
	usageWriteDeadline := time.Now().Add(time.Second)
	for {
		var usageEventCount int64
		if countError := database.Model(&managedUsageEventRecord{}).Count(&usageEventCount).Error; countError != nil {
			t.Fatalf("count migrated route usage: %v", countError)
		}
		if usageEventCount == 2 {
			break
		}
		if usageEventCount > 2 || time.Now().After(usageWriteDeadline) {
			t.Fatalf("migrated route usage count=%d want=2", usageEventCount)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestManagedTenantCurrentSchemaRejectsRetiredQwenCloudShapes(t *testing.T) {
	for _, testCase := range []struct {
		name string
		seed func(*testing.T, *gorm.DB, managedProviderKeyCipher, managedTenantRecord)
	}{
		{
			name: "managed provider settings",
			seed: func(t *testing.T, database *gorm.DB, providerKeyCipher managedProviderKeyCipher, tenantRecord managedTenantRecord) {
				encryptedKey, encryptionError := providerKeyCipher.encryptConnection(bytes.NewReader(bytes.Repeat([]byte{9}, providerKeyCipher.aeadCipher.NonceSize())), tenantRecord.TenantID, retiredQwenCloudProviderIdentifier, CatalogCredentialAPIKey, "sk-retired")
				if encryptionError != nil {
					t.Fatalf("encrypt retired key: %v", encryptionError)
				}
				if createError := database.Create(&managedProviderConnectionRecord{TenantID: tenantRecord.TenantID, ProviderID: retiredQwenCloudProviderIdentifier, FieldID: CatalogCredentialAPIKey, Value: encryptedKey, CreatedAt: tenantRecord.CreatedAt, UpdatedAt: tenantRecord.UpdatedAt}).Error; createError != nil {
					t.Fatalf("seed retired provider settings: %v", createError)
				}
				if createError := database.Create(&managedProviderProfileRecord{TenantID: tenantRecord.TenantID, ProviderID: retiredQwenCloudProviderIdentifier, TextModel: retiredQwenCloudModelIdentifier, SystemPrompt: "retired prompt", CreatedAt: tenantRecord.CreatedAt, UpdatedAt: tenantRecord.UpdatedAt}).Error; createError != nil {
					t.Fatalf("seed retired provider profile: %v", createError)
				}
			},
		},
		{
			name: "routing defaults",
			seed: func(t *testing.T, database *gorm.DB, _ managedProviderKeyCipher, tenantRecord managedTenantRecord) {
				if updateError := database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: tenantRecord.TenantID}).Updates(map[string]interface{}{
					"default_provider": retiredQwenCloudProviderIdentifier,
					"default_model":    retiredQwenCloudModelIdentifier,
				}).Error; updateError != nil {
					t.Fatalf("seed retired defaults: %v", updateError)
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "current-schema.db")
			database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
			if openError != nil {
				t.Fatalf("open current schema fixture: %v", openError)
			}
			providers := internalManagementProviderRegistry()
			providerKeyCipher := internalManagedProviderKeyCipher()
			if initializeError := initializeManagedTenantSchema(database, providerKeyCipher, providers); initializeError != nil {
				t.Fatalf("create current schema: %v", initializeError)
			}
			now := time.Date(2026, 8, 10, 21, 0, 0, 0, time.UTC)
			userRecord := managedUserRecord{UserID: "current-owner", CreatedAt: now, UpdatedAt: now}
			if createError := database.Create(&userRecord).Error; createError != nil {
				t.Fatalf("seed current user: %v", createError)
			}
			tenantRecord := managedTenantRecord{TenantID: "current-tenant", OwnerUserID: userRecord.UserID, Name: "Default", NameKey: "default", CreatedAt: now, UpdatedAt: now}
			tenantRecord.applyRoutingDefaults(defaultManagedRoutingDefaults())
			if createError := database.Create(&tenantRecord).Error; createError != nil {
				t.Fatalf("seed current tenant: %v", createError)
			}
			testCase.seed(t, database, providerKeyCipher, tenantRecord)

			reopenError := initializeManagedTenantSchema(database, providerKeyCipher, providers)
			if !errors.Is(reopenError, errManagedTenantSchemaMigration) || !strings.Contains(reopenError.Error(), "operation=validate") {
				t.Fatalf("reopen error=%v want retired-shape rejection", reopenError)
			}
		})
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
				ManagementConfiguration{DatabasePath: databasePath, DatabaseDialector: sqlite.Open(databasePath)},
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
			DatabasePath:      databasePath,
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

func internalManagementProviderRegistry() *providerRegistry {
	return newProviderRegistry(Configuration{ProviderCatalog: internalCanonicalProviderCatalog()})
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
