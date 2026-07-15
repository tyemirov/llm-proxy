package proxy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	generatedTenantSecretBytes         = 32
	generatedTenantSecretAttempts      = 16
	generatedTenantSecretPrefix        = "llmp_"
	legacyStaticTenantUserIDPrefix     = "static-config:"
	managedTenantIdentifierPrefix      = "managed-"
	managedTenantIdentifierHashBytes   = 12
	managedTenantUserEmailColumn       = "user_email"
	managedTenantUserIDColumn          = "user_id"
	managedTenantUserDisplayNameColumn = "user_display_name"
	managedTenantUserAvatarURLColumn   = "user_avatar_url"
	managedRecordUpdatedAtColumn       = "updated_at"
	managedUsageCreatedAtColumn        = "created_at"
	obsoleteStaticMigrationTable       = "managed_static_config_migration_records"
	managedProviderKeyCiphertextPrefix = "llmpk1:"
	maskedSecretPrefixLength           = 3
	maskedSecretSuffixLength           = 4
	managedUsageSummaryDays            = 30
)

var (
	errManagedTenantStoreOpen       = errors.New("managed_tenant_store_open_failed")
	errManagedTenantStorePersist    = errors.New("managed_tenant_store_persist_failed")
	errManagedProviderKeyInvalid    = errors.New("managed_provider_key_invalid")
	errManagedProviderKeyEncryption = errors.New("managed_provider_key_encryption_failed")
	errManagedProviderKeyDecryption = errors.New("managed_provider_key_decryption_failed")
	errManagedSecretGeneration      = errors.New("managed_secret_generation_failed")
	errManagedSecretCollision       = errors.New("managed_secret_collision")
	errManagedLegacyTokenMigration  = errors.New("managed_legacy_token_migration_failed")
	errManagedLegacyTokenConflict   = errors.New("managed_legacy_token_migration_conflict")
	errManagedLegacyTokenUnowned    = errors.New("managed_legacy_token_unowned")
)

type managedTenantStore struct {
	mutex             sync.RWMutex
	database          managedTenantDatabase
	providerKeyCipher managedProviderKeyCipher
	legacyMigration   managedLegacyTokenMigration
	randomReader      io.Reader
	now               func() time.Time
}

type managedTenantDatabase interface {
	tenantByUserID(userID string) (managedTenantRecord, error)
	tenantByTenantID(tenantID string) (managedTenantRecord, error)
	tenantBySecretDigest(secretDigest string) (managedTenantRecord, error)
	tenants() ([]managedTenantRecord, error)
	providerKeys() ([]managedProviderAPIKeyRecord, error)
	createTenant(record managedTenantRecord) error
	saveTenant(record managedTenantRecord) error
	saveProviderKey(record managedProviderAPIKeyRecord) error
	deleteProviderKey(record managedProviderAPIKeyRecord) error
	createUsageEvent(record managedUsageEventRecord) error
	usageEventsByUserIDSince(userID string, periodStart time.Time) ([]managedUsageEventRecord, error)
	usageEventsSince(periodStart time.Time) ([]managedUsageEventRecord, error)
	claimLegacyTenant(claim managedLegacyTenantClaim) error
}

type gormManagedTenantDatabase struct {
	database *gorm.DB
}

type managedTenantRecord struct {
	UserID                   string `gorm:"primaryKey"`
	UserEmail                string
	UserDisplayName          string
	UserAvatarURL            string
	TenantID                 string `gorm:"uniqueIndex"`
	SecretDigest             string `gorm:"index"`
	DefaultProvider          string
	DefaultModel             string
	DefaultDictationProvider string
	DefaultDictationModel    string
	DefaultSystemPrompt      string
	ProviderAPIKeys          []managedProviderAPIKeyRecord `gorm:"foreignKey:UserID;references:UserID;constraint:OnDelete:CASCADE"`
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type managedProviderAPIKeyRecord struct {
	UserID          string `gorm:"primaryKey"`
	ProviderID      string `gorm:"primaryKey"`
	APIKey          string `gorm:"column:api_key"`
	EncryptedAPIKey string
	TextModel       string
	SystemPrompt    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type managedUsageEventRecord struct {
	ID                  uint   `gorm:"primaryKey"`
	UserID              string `gorm:"index:idx_managed_usage_user_created"`
	TenantID            string
	Endpoint            string
	ProviderID          string
	ModelID             string
	StatusCode          int
	Success             bool
	LatencyMilliseconds int64
	RequestTokens       int
	ResponseTokens      int
	TotalTokens         int
	CreatedAt           time.Time `gorm:"index:idx_managed_usage_user_created;index:idx_managed_usage_created_at"`
}

type managedTenantSnapshot struct {
	userID           string
	userEmail        string
	userDisplayName  string
	userAvatarURL    string
	tenantID         string
	hasSecret        bool
	providerAPIKeys  map[providerID]string
	providerSettings map[providerID]managedProviderSettings
	defaults         TenantDefaults
	createdAt        time.Time
	updatedAt        time.Time
}

type managedAdminUserSnapshot struct {
	userID          string
	userEmail       string
	userDisplayName string
	userAvatarURL   string
	tenantID        string
	hasSecret       bool
	createdAt       time.Time
	updatedAt       time.Time
	usage           managedUsageSummary
}

type managedProviderKeyCipher struct {
	aeadCipher cipher.AEAD
}

type managedLegacyTokenMigration struct {
	tenantIdentifier tenantID
	legacyUserID     string
	ownerEmail       string
}

type managedLegacyTenantClaim struct {
	sourceUserID         string
	targetUserID         string
	tenantID             string
	targetEmail          string
	targetDisplayName    string
	targetAvatarURL      string
	updatedAt            time.Time
	reencryptProviderKey func(managedProviderAPIKeyRecord) (managedProviderAPIKeyRecord, error)
}

func newManagedTenantStore(configuration ManagementConfiguration) (*managedTenantStore, error) {
	database, databaseError := newGORMManagedTenantDatabase(configuration)
	if databaseError != nil {
		return nil, databaseError
	}
	providerKeyCipher, cipherError := newManagedProviderKeyCipher(configuration.ProviderKeyEncryptionKey)
	if cipherError != nil {
		return nil, fmt.Errorf("%w: field=management.provider_key_encryption_key: %v", errManagedTenantStoreOpen, cipherError)
	}
	legacyMigration, migrationError := newManagedLegacyTokenMigration(configuration.LegacyTokenMigration)
	if migrationError != nil {
		return nil, fmt.Errorf("%w: %v", errManagedTenantStoreOpen, migrationError)
	}
	store := newManagedTenantStoreWithDatabaseAndCipher(database, providerKeyCipher)
	store.legacyMigration = legacyMigration
	if migrationError := store.migratePlaintextProviderKeys(); migrationError != nil {
		return nil, migrationError
	}
	if migrationStateError := store.validateLegacyTokenMigrationState(); migrationStateError != nil {
		return nil, migrationStateError
	}
	return store, nil
}

func newManagedLegacyTokenMigration(configuration LegacyTokenMigrationConfiguration) (managedLegacyTokenMigration, error) {
	tenantIdentifier := strings.TrimSpace(configuration.TenantID)
	ownerEmail := strings.TrimSpace(configuration.OwnerEmail)
	if tenantIdentifier == constants.EmptyString && ownerEmail == constants.EmptyString {
		return managedLegacyTokenMigration{}, nil
	}
	if validationError := validateLegacyTokenMigrationConfiguration(configuration); validationError != nil {
		return managedLegacyTokenMigration{}, validationError
	}
	validatedTenantIdentifier := tenantID(tenantIdentifier)
	validatedOwnerEmail := strings.ToLower(ownerEmail)
	return managedLegacyTokenMigration{
		tenantIdentifier: validatedTenantIdentifier,
		legacyUserID:     legacyStaticTenantUserID(validatedTenantIdentifier),
		ownerEmail:       validatedOwnerEmail,
	}, nil
}

func (migration managedLegacyTokenMigration) configured() bool {
	return migration.legacyUserID != constants.EmptyString
}

func newManagedTenantStoreWithDatabase(database managedTenantDatabase) *managedTenantStore {
	return newManagedTenantStoreWithDatabaseAndCipher(database, internalManagedProviderKeyCipher())
}

func newManagedTenantStoreWithDatabaseAndCipher(database managedTenantDatabase, providerKeyCipher managedProviderKeyCipher) *managedTenantStore {
	return &managedTenantStore{
		database:          database,
		providerKeyCipher: providerKeyCipher,
		randomReader:      rand.Reader,
		now:               func() time.Time { return time.Now().UTC() },
	}
}

func newManagedProviderKeyCipher(rawEncryptionKey string) (managedProviderKeyCipher, error) {
	encryptionKey, decodeError := decodeManagedProviderKey(rawEncryptionKey)
	if decodeError != nil {
		return managedProviderKeyCipher{}, decodeError
	}
	blockCipher, _ := aes.NewCipher(encryptionKey[:])
	aeadCipher, _ := cipher.NewGCM(blockCipher)
	return managedProviderKeyCipher{aeadCipher: aeadCipher}, nil
}

func internalManagedProviderKeyCipher() managedProviderKeyCipher {
	providerKeyCipher, _ := newManagedProviderKeyCipher("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	return providerKeyCipher
}

func (providerKeyCipher managedProviderKeyCipher) encrypt(randomReader io.Reader, userID string, providerIdentifier string, rawAPIKey string) (string, error) {
	apiKey := strings.TrimSpace(rawAPIKey)
	if apiKey == constants.EmptyString {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s", errManagedProviderKeyInvalid, providerIdentifier)
	}
	nonce := make([]byte, providerKeyCipher.aeadCipher.NonceSize())
	if _, readError := io.ReadFull(randomReader, nonce); readError != nil {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s: %v", errManagedProviderKeyEncryption, providerIdentifier, readError)
	}
	sealedAPIKey := providerKeyCipher.aeadCipher.Seal(nil, nonce, []byte(apiKey), managedProviderKeyAssociatedData(userID, providerIdentifier))
	sealedPayload := append(nonce, sealedAPIKey...)
	return managedProviderKeyCiphertextPrefix + base64.StdEncoding.EncodeToString(sealedPayload), nil
}

func (providerKeyCipher managedProviderKeyCipher) decrypt(record managedProviderAPIKeyRecord) (string, error) {
	encryptedAPIKey := strings.TrimSpace(record.EncryptedAPIKey)
	if encryptedAPIKey == constants.EmptyString {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s", errManagedProviderKeyDecryption, record.ProviderID)
	}
	if !strings.HasPrefix(encryptedAPIKey, managedProviderKeyCiphertextPrefix) {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s", errManagedProviderKeyDecryption, record.ProviderID)
	}
	encodedPayload := strings.TrimPrefix(encryptedAPIKey, managedProviderKeyCiphertextPrefix)
	sealedPayload, decodeError := base64.StdEncoding.DecodeString(encodedPayload)
	if decodeError != nil {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s: %v", errManagedProviderKeyDecryption, record.ProviderID, decodeError)
	}
	if len(sealedPayload) <= providerKeyCipher.aeadCipher.NonceSize() {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s", errManagedProviderKeyDecryption, record.ProviderID)
	}
	nonce := sealedPayload[:providerKeyCipher.aeadCipher.NonceSize()]
	ciphertext := sealedPayload[providerKeyCipher.aeadCipher.NonceSize():]
	apiKeyBytes, decryptError := providerKeyCipher.aeadCipher.Open(nil, nonce, ciphertext, managedProviderKeyAssociatedData(record.UserID, record.ProviderID))
	if decryptError != nil {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s: %v", errManagedProviderKeyDecryption, record.ProviderID, decryptError)
	}
	return strings.TrimSpace(string(apiKeyBytes)), nil
}

func managedProviderKeyAssociatedData(userID string, providerIdentifier string) []byte {
	return []byte(strings.TrimSpace(userID) + "\x00" + strings.TrimSpace(providerIdentifier))
}

func newGORMManagedTenantDatabase(configuration ManagementConfiguration) (*gormManagedTenantDatabase, error) {
	dialector, dialectorError := managementDatabaseDialector(configuration)
	if dialectorError != nil {
		return nil, dialectorError
	}
	database, openError := gorm.Open(dialector, &gorm.Config{})
	if openError != nil {
		return nil, fmt.Errorf("%w: %v", errManagedTenantStoreOpen, openError)
	}
	if migrateError := database.AutoMigrate(&managedTenantRecord{}, &managedProviderAPIKeyRecord{}, &managedUsageEventRecord{}); migrateError != nil {
		return nil, fmt.Errorf("%w: migrate: %v", errManagedTenantStoreOpen, migrateError)
	}
	if cleanupError := removeObsoleteStaticMigrationTable(database); cleanupError != nil {
		return nil, fmt.Errorf("%w: remove_obsolete_static_migration_table: %w", errManagedTenantStoreOpen, cleanupError)
	}
	return &gormManagedTenantDatabase{database: database}, nil
}

func removeObsoleteStaticMigrationTable(database *gorm.DB) error {
	migrator := database.Migrator()
	if !migrator.HasTable(obsoleteStaticMigrationTable) {
		return nil
	}
	return migrator.DropTable(obsoleteStaticMigrationTable)
}

func managementDatabaseDialector(configuration ManagementConfiguration) (gorm.Dialector, error) {
	if configuration.DatabaseDialector != nil {
		return configuration.DatabaseDialector, nil
	}
	databaseDialect := strings.ToLower(strings.TrimSpace(configuration.DatabaseDialect))
	dialectors := map[string]func(string) gorm.Dialector{
		ManagementDatabaseDialectPostgres: postgres.Open,
		ManagementDatabaseDialectSQLite:   sqlite.Open,
	}
	dialectorFactory, supportedDialect := dialectors[databaseDialect]
	if !supportedDialect {
		return nil, fmt.Errorf("%w: field=management.database_dialect value=%s", errManagedTenantStoreOpen, databaseDialect)
	}
	return dialectorFactory(configuration.DatabaseDSN), nil
}

func (database *gormManagedTenantDatabase) tenantByUserID(userID string) (managedTenantRecord, error) {
	var record managedTenantRecord
	queryError := database.database.
		Preload("ProviderAPIKeys").
		Where(&managedTenantRecord{UserID: userID}).
		First(&record).
		Error
	return record, queryError
}

func (database *gormManagedTenantDatabase) tenantByTenantID(tenantID string) (managedTenantRecord, error) {
	var record managedTenantRecord
	queryError := database.database.
		Where(&managedTenantRecord{TenantID: tenantID}).
		First(&record).
		Error
	return record, queryError
}

func (database *gormManagedTenantDatabase) tenantBySecretDigest(secretDigest string) (managedTenantRecord, error) {
	var record managedTenantRecord
	queryError := database.database.
		Preload("ProviderAPIKeys").
		Where(&managedTenantRecord{SecretDigest: secretDigest}).
		First(&record).
		Error
	return record, queryError
}

func (database *gormManagedTenantDatabase) tenants() ([]managedTenantRecord, error) {
	var records []managedTenantRecord
	queryError := database.database.
		Order(clause.OrderBy{Columns: []clause.OrderByColumn{
			{Column: clause.Column{Name: managedTenantUserEmailColumn}},
			{Column: clause.Column{Name: managedTenantUserIDColumn}},
		}}).
		Find(&records).
		Error
	return records, queryError
}

func (database *gormManagedTenantDatabase) providerKeys() ([]managedProviderAPIKeyRecord, error) {
	var records []managedProviderAPIKeyRecord
	queryError := database.database.
		Find(&records).
		Error
	return records, queryError
}

func (database *gormManagedTenantDatabase) createTenant(record managedTenantRecord) error {
	return database.database.Create(&record).Error
}

func (database *gormManagedTenantDatabase) saveTenant(record managedTenantRecord) error {
	return database.database.Omit("ProviderAPIKeys").Save(&record).Error
}

func (database *gormManagedTenantDatabase) saveProviderKey(record managedProviderAPIKeyRecord) error {
	return database.database.Save(&record).Error
}

func (database *gormManagedTenantDatabase) deleteProviderKey(record managedProviderAPIKeyRecord) error {
	return database.database.Where(&record).Delete(&managedProviderAPIKeyRecord{}).Error
}

func (database *gormManagedTenantDatabase) createUsageEvent(record managedUsageEventRecord) error {
	return database.database.Create(&record).Error
}

func (database *gormManagedTenantDatabase) usageEventsByUserIDSince(userID string, periodStart time.Time) ([]managedUsageEventRecord, error) {
	var records []managedUsageEventRecord
	queryError := database.database.
		Where(&managedUsageEventRecord{UserID: userID}).
		Where(clause.Gte{Column: clause.Column{Name: managedUsageCreatedAtColumn}, Value: periodStart}).
		Find(&records).
		Error
	return records, queryError
}

func (database *gormManagedTenantDatabase) usageEventsSince(periodStart time.Time) ([]managedUsageEventRecord, error) {
	var records []managedUsageEventRecord
	queryError := database.database.
		Where(clause.Gte{Column: clause.Column{Name: managedUsageCreatedAtColumn}, Value: periodStart}).
		Find(&records).
		Error
	return records, queryError
}

func (database *gormManagedTenantDatabase) claimLegacyTenant(claim managedLegacyTenantClaim) error {
	return database.database.Transaction(func(transaction *gorm.DB) error {
		var sourceRecord managedTenantRecord
		sourceError := transaction.
			Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Where(&managedTenantRecord{UserID: claim.sourceUserID}).
			First(&sourceRecord).
			Error

		var targetRecord managedTenantRecord
		targetError := transaction.
			Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Where(&managedTenantRecord{UserID: claim.targetUserID}).
			First(&targetRecord).
			Error

		if errors.Is(sourceError, gorm.ErrRecordNotFound) {
			if targetError == nil && targetRecord.TenantID == claim.tenantID {
				return nil
			}
			if targetError != nil && !errors.Is(targetError, gorm.ErrRecordNotFound) {
				return targetError
			}
			return fmt.Errorf("%w: source_missing tenant=%s", errManagedLegacyTokenConflict, claim.tenantID)
		}
		if sourceError != nil {
			return sourceError
		}
		if sourceRecord.TenantID != claim.tenantID {
			return fmt.Errorf("%w: source_tenant=%s expected=%s", errManagedLegacyTokenConflict, sourceRecord.TenantID, claim.tenantID)
		}
		if targetError != nil && !errors.Is(targetError, gorm.ErrRecordNotFound) {
			return targetError
		}

		var targetProviderCount int64
		if countError := transaction.Model(&managedProviderAPIKeyRecord{}).
			Where(&managedProviderAPIKeyRecord{UserID: claim.targetUserID}).
			Count(&targetProviderCount).
			Error; countError != nil {
			return countError
		}
		var targetUsageCount int64
		if countError := transaction.Model(&managedUsageEventRecord{}).
			Where(&managedUsageEventRecord{UserID: claim.targetUserID}).
			Count(&targetUsageCount).
			Error; countError != nil {
			return countError
		}
		if targetProviderCount != 0 || targetUsageCount != 0 {
			return fmt.Errorf("%w: destination_children user_id=%s", errManagedLegacyTokenConflict, claim.targetUserID)
		}
		if targetError == nil {
			if targetRecord.SecretDigest != constants.EmptyString {
				return fmt.Errorf("%w: destination_secret user_id=%s", errManagedLegacyTokenConflict, claim.targetUserID)
			}
			deleteTargetResult := transaction.
				Where(&managedTenantRecord{UserID: claim.targetUserID, TenantID: targetRecord.TenantID}).
				Delete(&managedTenantRecord{})
			if deleteTargetResult.Error != nil {
				return deleteTargetResult.Error
			}
			if deleteTargetResult.RowsAffected != 1 {
				return fmt.Errorf("%w: destination_delete_count=%d", errManagedLegacyTokenMigration, deleteTargetResult.RowsAffected)
			}
		}

		var sourceProviderRecords []managedProviderAPIKeyRecord
		if providerQueryError := transaction.
			Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Where(&managedProviderAPIKeyRecord{UserID: claim.sourceUserID}).
			Find(&sourceProviderRecords).
			Error; providerQueryError != nil {
			return providerQueryError
		}
		var sourceUsageRecords []managedUsageEventRecord
		if usageQueryError := transaction.
			Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Where(&managedUsageEventRecord{UserID: claim.sourceUserID}).
			Find(&sourceUsageRecords).
			Error; usageQueryError != nil {
			return usageQueryError
		}
		for _, usageRecord := range sourceUsageRecords {
			if usageRecord.TenantID != claim.tenantID {
				return fmt.Errorf("%w: usage_tenant=%s expected=%s", errManagedLegacyTokenConflict, usageRecord.TenantID, claim.tenantID)
			}
		}

		targetProviderRecords := make([]managedProviderAPIKeyRecord, 0, len(sourceProviderRecords))
		for _, sourceProviderRecord := range sourceProviderRecords {
			targetProviderRecord, reencryptError := claim.reencryptProviderKey(sourceProviderRecord)
			if reencryptError != nil {
				return reencryptError
			}
			targetProviderRecords = append(targetProviderRecords, targetProviderRecord)
		}

		if len(sourceProviderRecords) != 0 {
			deleteProviderKeysResult := transaction.
				Where(&managedProviderAPIKeyRecord{UserID: claim.sourceUserID}).
				Delete(&managedProviderAPIKeyRecord{})
			if deleteProviderKeysResult.Error != nil {
				return deleteProviderKeysResult.Error
			}
			if deleteProviderKeysResult.RowsAffected != int64(len(sourceProviderRecords)) {
				return fmt.Errorf("%w: provider_delete_count=%d expected=%d", errManagedLegacyTokenMigration, deleteProviderKeysResult.RowsAffected, len(sourceProviderRecords))
			}
		}

		updateTenantResult := transaction.
			Model(&managedTenantRecord{}).
			Where(&managedTenantRecord{UserID: claim.sourceUserID, TenantID: claim.tenantID}).
			Updates(map[string]interface{}{
				managedTenantUserIDColumn:          claim.targetUserID,
				managedTenantUserEmailColumn:       claim.targetEmail,
				managedTenantUserDisplayNameColumn: claim.targetDisplayName,
				managedTenantUserAvatarURLColumn:   claim.targetAvatarURL,
				managedRecordUpdatedAtColumn:       claim.updatedAt,
			})
		if updateTenantResult.Error != nil {
			return updateTenantResult.Error
		}
		if updateTenantResult.RowsAffected != 1 {
			return fmt.Errorf("%w: tenant_update_count=%d", errManagedLegacyTokenMigration, updateTenantResult.RowsAffected)
		}

		if len(targetProviderRecords) != 0 {
			createProviderKeysResult := transaction.Create(&targetProviderRecords)
			if createProviderKeysResult.Error != nil {
				return createProviderKeysResult.Error
			}
			if createProviderKeysResult.RowsAffected != int64(len(targetProviderRecords)) {
				return fmt.Errorf("%w: provider_create_count=%d expected=%d", errManagedLegacyTokenMigration, createProviderKeysResult.RowsAffected, len(targetProviderRecords))
			}
		}

		if len(sourceUsageRecords) != 0 {
			updateUsageResult := transaction.
				Model(&managedUsageEventRecord{}).
				Where(&managedUsageEventRecord{UserID: claim.sourceUserID}).
				Update(managedTenantUserIDColumn, claim.targetUserID)
			if updateUsageResult.Error != nil {
				return updateUsageResult.Error
			}
			if updateUsageResult.RowsAffected != int64(len(sourceUsageRecords)) {
				return fmt.Errorf("%w: usage_update_count=%d expected=%d", errManagedLegacyTokenMigration, updateUsageResult.RowsAffected, len(sourceUsageRecords))
			}
		}

		return verifyLegacyTenantClaim(transaction, claim, len(targetProviderRecords), len(sourceUsageRecords))
	})
}

func verifyLegacyTenantClaim(transaction *gorm.DB, claim managedLegacyTenantClaim, expectedProviderCount int, expectedUsageCount int) error {
	checks := []struct {
		model         interface{}
		userID        string
		expectedCount int64
	}{
		{model: &managedTenantRecord{}, userID: claim.sourceUserID, expectedCount: 0},
		{model: &managedProviderAPIKeyRecord{}, userID: claim.sourceUserID, expectedCount: 0},
		{model: &managedUsageEventRecord{}, userID: claim.sourceUserID, expectedCount: 0},
		{model: &managedTenantRecord{}, userID: claim.targetUserID, expectedCount: 1},
		{model: &managedProviderAPIKeyRecord{}, userID: claim.targetUserID, expectedCount: int64(expectedProviderCount)},
		{model: &managedUsageEventRecord{}, userID: claim.targetUserID, expectedCount: int64(expectedUsageCount)},
	}
	for _, check := range checks {
		var recordCount int64
		if countError := transaction.Model(check.model).
			Where(map[string]interface{}{managedTenantUserIDColumn: check.userID}).
			Count(&recordCount).
			Error; countError != nil {
			return countError
		}
		if recordCount != check.expectedCount {
			return fmt.Errorf("%w: user_id=%s count=%d expected=%d", errManagedLegacyTokenMigration, check.userID, recordCount, check.expectedCount)
		}
	}
	return nil
}

func (store *managedTenantStore) migratePlaintextProviderKeys() error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	providerKeyRecords, queryError := store.database.providerKeys()
	if queryError != nil {
		return fmt.Errorf("%w: provider_keys: %v", errManagedTenantStorePersist, queryError)
	}
	timestamp := store.now()
	for _, providerKeyRecord := range providerKeyRecords {
		plaintextAPIKey := strings.TrimSpace(providerKeyRecord.APIKey)
		if plaintextAPIKey == constants.EmptyString {
			continue
		}
		if strings.TrimSpace(providerKeyRecord.EncryptedAPIKey) == constants.EmptyString {
			encryptedAPIKey, encryptionError := store.providerKeyCipher.encrypt(store.randomReader, providerKeyRecord.UserID, providerKeyRecord.ProviderID, plaintextAPIKey)
			if encryptionError != nil {
				return encryptionError
			}
			providerKeyRecord.EncryptedAPIKey = encryptedAPIKey
		}
		providerKeyRecord.APIKey = constants.EmptyString
		providerKeyRecord.UpdatedAt = timestamp
		if persistError := store.database.saveProviderKey(providerKeyRecord); persistError != nil {
			return fmt.Errorf("%w: provider=%s: %v", errManagedTenantStorePersist, providerKeyRecord.ProviderID, persistError)
		}
	}
	return nil
}

func (store *managedTenantStore) migrateProviderTextSettings(providers *providerRegistry) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	providerKeyRecords, queryError := store.database.providerKeys()
	if queryError != nil {
		return fmt.Errorf("%w: provider_keys: %v", errManagedTenantStorePersist, queryError)
	}
	defaultModels := providerDefaultTextModels(providers)
	timestamp := store.now()
	for _, providerKeyRecord := range providerKeyRecords {
		if strings.TrimSpace(providerKeyRecord.TextModel) != constants.EmptyString {
			continue
		}
		textModel, knownProvider := defaultModels[newProviderID(providerKeyRecord.ProviderID)]
		if !knownProvider {
			continue
		}
		providerKeyRecord.TextModel = textModel
		providerKeyRecord.UpdatedAt = timestamp
		if persistError := store.database.saveProviderKey(providerKeyRecord); persistError != nil {
			return fmt.Errorf("%w: provider=%s: %v", errManagedTenantStorePersist, providerKeyRecord.ProviderID, persistError)
		}
	}
	return nil
}

func providerDefaultTextModels(providers *providerRegistry) map[providerID]string {
	summaries := providers.providerSummaries()
	defaultModels := make(map[providerID]string, len(summaries))
	for _, summary := range summaries {
		defaultModels[newProviderID(summary.identifier)] = summary.textDefaultModel
	}
	return defaultModels
}

func (store *managedTenantStore) validateLegacyTokenMigrationState() error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	tenantRecords, queryError := store.database.tenants()
	if queryError != nil {
		return fmt.Errorf("%w: legacy_tenants: %v", errManagedTenantStorePersist, queryError)
	}
	legacyTenantRecords := make([]managedTenantRecord, 0, 1)
	for _, tenantRecord := range tenantRecords {
		if strings.HasPrefix(tenantRecord.UserID, legacyStaticTenantUserIDPrefix) {
			legacyTenantRecords = append(legacyTenantRecords, tenantRecord)
		}
	}
	if len(legacyTenantRecords) == 0 {
		return nil
	}
	if !store.legacyMigration.configured() {
		return fmt.Errorf("%w: legacy_owner_config_missing", errManagedLegacyTokenMigration)
	}
	if len(legacyTenantRecords) != 1 {
		return fmt.Errorf("%w: legacy_tenant_count=%d", errManagedLegacyTokenMigration, len(legacyTenantRecords))
	}
	legacyTenantRecord := legacyTenantRecords[0]
	if legacyTenantRecord.UserID != store.legacyMigration.legacyUserID || legacyTenantRecord.TenantID != store.legacyMigration.tenantIdentifier.string() {
		return fmt.Errorf("%w: legacy_tenant=%s expected=%s", errManagedLegacyTokenMigration, legacyTenantRecord.TenantID, store.legacyMigration.tenantIdentifier.string())
	}
	return nil
}

func (store *managedTenantStore) claimLegacyToken(principal managementPrincipal) error {
	if !store.legacyMigration.configured() || principal.userEmail != store.legacyMigration.ownerEmail {
		return nil
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if strings.HasPrefix(principal.userID, legacyStaticTenantUserIDPrefix) {
		return fmt.Errorf("%w: invalid_target_user_id", errManagedLegacyTokenConflict)
	}
	claim := managedLegacyTenantClaim{
		sourceUserID:      store.legacyMigration.legacyUserID,
		targetUserID:      principal.userID,
		tenantID:          store.legacyMigration.tenantIdentifier.string(),
		targetEmail:       principal.userEmail,
		targetDisplayName: principal.userDisplayName,
		targetAvatarURL:   principal.userAvatarURL,
		updatedAt:         store.now(),
	}
	claim.reencryptProviderKey = func(sourceRecord managedProviderAPIKeyRecord) (managedProviderAPIKeyRecord, error) {
		apiKey, decryptError := store.providerKeyCipher.decrypt(sourceRecord)
		if decryptError != nil {
			return managedProviderAPIKeyRecord{}, decryptError
		}
		encryptedAPIKey, encryptionError := store.providerKeyCipher.encrypt(store.randomReader, claim.targetUserID, sourceRecord.ProviderID, apiKey)
		if encryptionError != nil {
			return managedProviderAPIKeyRecord{}, encryptionError
		}
		targetRecord := sourceRecord
		targetRecord.UserID = claim.targetUserID
		targetRecord.APIKey = constants.EmptyString
		targetRecord.EncryptedAPIKey = encryptedAPIKey
		targetRecord.UpdatedAt = claim.updatedAt
		return targetRecord, nil
	}
	if migrationError := store.database.claimLegacyTenant(claim); migrationError != nil {
		return fmt.Errorf("%w: tenant=%s: %w", errManagedLegacyTokenMigration, claim.tenantID, migrationError)
	}
	return nil
}

func (store *managedTenantStore) profile(principal managementPrincipal) (managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.ensureRecordLocked(principal)
	if recordError != nil {
		return managedTenantSnapshot{}, recordError
	}
	return store.snapshot(record)
}

func (store *managedTenantStore) saveProviderKey(principal managementPrincipal, providerIdentifier providerID, rawAPIKey string, textModel string, systemPrompt string) (managedTenantSnapshot, error) {
	apiKey := strings.TrimSpace(rawAPIKey)
	normalizedTextModel := strings.TrimSpace(textModel)
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.ensureRecordLocked(principal)
	if recordError != nil {
		return managedTenantSnapshot{}, recordError
	}
	existingProviderKeyRecord, hasExistingProviderKey := managedProviderKeyRecordForProvider(record.ProviderAPIKeys, providerIdentifier)
	if apiKey == constants.EmptyString && !hasExistingProviderKey {
		return managedTenantSnapshot{}, fmt.Errorf("%w: provider=%s", errManagedProviderKeyInvalid, providerIdentifier.string())
	}
	timestamp := store.now()
	encryptedAPIKey := existingProviderKeyRecord.EncryptedAPIKey
	createdAt := existingProviderKeyRecord.CreatedAt
	if apiKey != constants.EmptyString {
		var encryptionError error
		encryptedAPIKey, encryptionError = store.providerKeyCipher.encrypt(store.randomReader, record.UserID, providerIdentifier.string(), apiKey)
		if encryptionError != nil {
			return managedTenantSnapshot{}, encryptionError
		}
	}
	if createdAt.IsZero() {
		createdAt = timestamp
	}
	if persistError := store.database.saveProviderKey(managedProviderAPIKeyRecord{
		UserID:          record.UserID,
		ProviderID:      providerIdentifier.string(),
		EncryptedAPIKey: encryptedAPIKey,
		TextModel:       normalizedTextModel,
		SystemPrompt:    systemPrompt,
		CreatedAt:       createdAt,
		UpdatedAt:       timestamp,
	}); persistError != nil {
		return managedTenantSnapshot{}, fmt.Errorf("%w: provider=%s: %v", errManagedTenantStorePersist, providerIdentifier.string(), persistError)
	}
	record.UpdatedAt = timestamp
	if persistError := store.database.saveTenant(record); persistError != nil {
		return managedTenantSnapshot{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, record.UserID, persistError)
	}
	return store.snapshotByUserIDLocked(record.UserID)
}

func managedProviderKeyRecordForProvider(providerKeyRecords []managedProviderAPIKeyRecord, providerIdentifier providerID) (managedProviderAPIKeyRecord, bool) {
	for _, providerKeyRecord := range providerKeyRecords {
		if newProviderID(providerKeyRecord.ProviderID) == providerIdentifier {
			return providerKeyRecord, true
		}
	}
	return managedProviderAPIKeyRecord{}, false
}

func (store *managedTenantStore) removeProviderKey(principal managementPrincipal, providerIdentifier providerID) (managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.ensureRecordLocked(principal)
	if recordError != nil {
		return managedTenantSnapshot{}, recordError
	}
	if persistError := store.database.deleteProviderKey(managedProviderAPIKeyRecord{UserID: record.UserID, ProviderID: providerIdentifier.string()}); persistError != nil {
		return managedTenantSnapshot{}, fmt.Errorf("%w: provider=%s: %v", errManagedTenantStorePersist, providerIdentifier.string(), persistError)
	}
	record.UpdatedAt = store.now()
	if persistError := store.database.saveTenant(record); persistError != nil {
		return managedTenantSnapshot{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, record.UserID, persistError)
	}
	return store.snapshotByUserIDLocked(record.UserID)
}

func (store *managedTenantStore) updateDefaults(principal managementPrincipal, defaults TenantDefaults) (managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.ensureRecordLocked(principal)
	if recordError != nil {
		return managedTenantSnapshot{}, recordError
	}
	record.applyDefaults(defaults)
	record.UpdatedAt = store.now()
	if persistError := store.database.saveTenant(record); persistError != nil {
		return managedTenantSnapshot{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, record.UserID, persistError)
	}
	return store.snapshotByUserIDLocked(record.UserID)
}

func (store *managedTenantStore) generateSecret(principal managementPrincipal, secretDigestInUse func([sha256.Size]byte) bool) (string, managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.ensureRecordLocked(principal)
	if recordError != nil {
		return constants.EmptyString, managedTenantSnapshot{}, recordError
	}
	for attempt := 0; attempt < generatedTenantSecretAttempts; attempt++ {
		rawSecret, secretDigest, secretError := store.newTenantSecret()
		if secretError != nil {
			return constants.EmptyString, managedTenantSnapshot{}, secretError
		}
		if secretDigestInUse(secretDigest) || store.containsSecretDigestLocked(secretDigest) {
			continue
		}
		record.SecretDigest = hex.EncodeToString(secretDigest[:])
		record.UpdatedAt = store.now()
		if persistError := store.database.saveTenant(record); persistError != nil {
			return constants.EmptyString, managedTenantSnapshot{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, record.UserID, persistError)
		}
		snapshot, snapshotError := store.snapshotByUserIDLocked(record.UserID)
		if snapshotError != nil {
			return constants.EmptyString, managedTenantSnapshot{}, snapshotError
		}
		return rawSecret, snapshot, nil
	}
	return constants.EmptyString, managedTenantSnapshot{}, errManagedSecretCollision
}

func (store *managedTenantStore) revokeSecret(principal managementPrincipal) (managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.ensureRecordLocked(principal)
	if recordError != nil {
		return managedTenantSnapshot{}, recordError
	}
	record.SecretDigest = constants.EmptyString
	record.UpdatedAt = store.now()
	if persistError := store.database.saveTenant(record); persistError != nil {
		return managedTenantSnapshot{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, record.UserID, persistError)
	}
	return store.snapshotByUserIDLocked(record.UserID)
}

func (store *managedTenantStore) authenticate(rawSecret string) (tenant, bool) {
	presentedSecret := strings.TrimSpace(rawSecret)
	if presentedSecret == constants.EmptyString {
		return tenant{}, false
	}
	presentedDigest := sha256.Sum256([]byte(presentedSecret))
	presentedDigestString := hex.EncodeToString(presentedDigest[:])
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	record, recordError := store.database.tenantBySecretDigest(presentedDigestString)
	if recordError != nil {
		return tenant{}, false
	}
	if strings.HasPrefix(record.UserID, legacyStaticTenantUserIDPrefix) {
		return tenant{}, false
	}
	recordDigest, digestValid := managedRecordSecretDigest(record)
	if !digestValid || !constantTimeDigestEquals(recordDigest, presentedDigest) {
		return tenant{}, false
	}
	authenticatedTenant, tenantError := store.tenant(record, recordDigest)
	if tenantError != nil {
		return tenant{}, false
	}
	return authenticatedTenant, true
}

func (store *managedTenantStore) containsSecretDigestLocked(secretDigest [sha256.Size]byte) bool {
	secretDigestString := hex.EncodeToString(secretDigest[:])
	_, recordError := store.database.tenantBySecretDigest(secretDigestString)
	return recordError == nil
}

func (store *managedTenantStore) ensureRecordLocked(principal managementPrincipal) (managedTenantRecord, error) {
	record, recordError := store.database.tenantByUserID(principal.userID)
	if recordError == nil {
		record.UserEmail = principal.userEmail
		record.UserDisplayName = principal.userDisplayName
		record.UserAvatarURL = principal.userAvatarURL
		record.UpdatedAt = store.now()
		if persistError := store.database.saveTenant(record); persistError != nil {
			return managedTenantRecord{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, record.UserID, persistError)
		}
		return record, nil
	}
	if !errors.Is(recordError, gorm.ErrRecordNotFound) {
		return managedTenantRecord{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, principal.userID, recordError)
	}
	createdAt := store.now()
	record = newManagedTenantRecord(principal, createdAt)
	if persistError := store.database.createTenant(record); persistError != nil {
		return managedTenantRecord{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, principal.userID, persistError)
	}
	return record, nil
}

func (store *managedTenantStore) snapshotByUserIDLocked(userID string) (managedTenantSnapshot, error) {
	record, recordError := store.database.tenantByUserID(userID)
	if recordError != nil {
		return managedTenantSnapshot{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, userID, recordError)
	}
	return store.snapshot(record)
}

func (store *managedTenantStore) newTenantSecret() (string, [sha256.Size]byte, error) {
	randomBytes := make([]byte, generatedTenantSecretBytes)
	if _, readError := io.ReadFull(store.randomReader, randomBytes); readError != nil {
		return constants.EmptyString, [sha256.Size]byte{}, fmt.Errorf("%w: %v", errManagedSecretGeneration, readError)
	}
	rawSecret := generatedTenantSecretPrefix + hex.EncodeToString(randomBytes)
	return rawSecret, sha256.Sum256([]byte(rawSecret)), nil
}

func newManagedTenantRecord(principal managementPrincipal, createdAt time.Time) managedTenantRecord {
	record := managedTenantRecord{
		UserID:          principal.userID,
		UserEmail:       principal.userEmail,
		UserDisplayName: principal.userDisplayName,
		UserAvatarURL:   principal.userAvatarURL,
		TenantID:        managedTenantID(principal.userID),
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	record.applyDefaults(DefaultTenantDefaults())
	return record
}

func legacyStaticTenantUserID(identifier tenantID) string {
	return legacyStaticTenantUserIDPrefix + identifier.string()
}

func (record *managedTenantRecord) applyDefaults(defaults TenantDefaults) {
	normalizedDefaults := newTenantDefaults(defaults)
	record.DefaultProvider = normalizedDefaults.provider
	record.DefaultModel = normalizedDefaults.model
	record.DefaultDictationProvider = normalizedDefaults.dictationProvider
	record.DefaultDictationModel = normalizedDefaults.dictationModel
	record.DefaultSystemPrompt = normalizedDefaults.systemPrompt
}

func (store *managedTenantStore) snapshot(record managedTenantRecord) (managedTenantSnapshot, error) {
	providerSettings, providerKeyError := store.providerSettingsMap(record.ProviderAPIKeys)
	if providerKeyError != nil {
		return managedTenantSnapshot{}, providerKeyError
	}
	providerAPIKeys := managedProviderAPIKeys(providerSettings)
	return managedTenantSnapshot{
		userID:           record.UserID,
		userEmail:        record.UserEmail,
		userDisplayName:  record.UserDisplayName,
		userAvatarURL:    record.UserAvatarURL,
		tenantID:         record.TenantID,
		hasSecret:        record.SecretDigest != constants.EmptyString,
		providerAPIKeys:  providerAPIKeys,
		providerSettings: providerSettings,
		defaults:         record.defaults(),
		createdAt:        record.CreatedAt,
		updatedAt:        record.UpdatedAt,
	}, nil
}

func (record managedTenantRecord) adminSnapshot(usageSummary managedUsageSummary) managedAdminUserSnapshot {
	return managedAdminUserSnapshot{
		userID:          record.UserID,
		userEmail:       record.UserEmail,
		userDisplayName: record.UserDisplayName,
		userAvatarURL:   record.UserAvatarURL,
		tenantID:        record.TenantID,
		hasSecret:       record.SecretDigest != constants.EmptyString,
		createdAt:       record.CreatedAt,
		updatedAt:       record.UpdatedAt,
		usage:           usageSummary,
	}
}

func (record managedTenantRecord) defaults() TenantDefaults {
	return TenantDefaults{
		Provider:          record.DefaultProvider,
		Model:             record.DefaultModel,
		DictationProvider: record.DefaultDictationProvider,
		DictationModel:    record.DefaultDictationModel,
		SystemPrompt:      record.DefaultSystemPrompt,
	}
}

func (store *managedTenantStore) tenant(record managedTenantRecord, secretDigest [sha256.Size]byte) (tenant, error) {
	providerSettings, providerKeyError := store.providerSettingsMap(record.ProviderAPIKeys)
	if providerKeyError != nil {
		return tenant{}, providerKeyError
	}
	providerAPIKeys := managedProviderAPIKeys(providerSettings)
	return tenant{
		identifier:       tenantID(record.TenantID),
		userID:           record.UserID,
		secretDigest:     secretDigest,
		defaults:         newTenantDefaults(record.defaults()),
		managed:          true,
		providerAPIKeys:  providerAPIKeys,
		providerSettings: providerSettings,
	}, nil
}

func (store *managedTenantStore) providerAPIKeyMap(providerKeyRecords []managedProviderAPIKeyRecord) (map[providerID]string, error) {
	providerSettings, providerKeyError := store.providerSettingsMap(providerKeyRecords)
	if providerKeyError != nil {
		return nil, providerKeyError
	}
	return managedProviderAPIKeys(providerSettings), nil
}

func (store *managedTenantStore) providerSettingsMap(providerKeyRecords []managedProviderAPIKeyRecord) (map[providerID]managedProviderSettings, error) {
	providerSettings := make(map[providerID]managedProviderSettings, len(providerKeyRecords))
	for _, providerKeyRecord := range providerKeyRecords {
		providerIdentifier := newProviderID(providerKeyRecord.ProviderID)
		if providerIdentifier.string() == constants.EmptyString {
			continue
		}
		apiKey, decryptError := store.providerKeyCipher.decrypt(providerKeyRecord)
		if decryptError != nil {
			return nil, decryptError
		}
		if apiKey != constants.EmptyString {
			providerSettings[providerIdentifier] = managedProviderSettings{
				apiKey:       apiKey,
				textModel:    strings.TrimSpace(providerKeyRecord.TextModel),
				systemPrompt: providerKeyRecord.SystemPrompt,
			}
		}
	}
	return providerSettings, nil
}

func managedProviderAPIKeys(providerSettings map[providerID]managedProviderSettings) map[providerID]string {
	providerAPIKeys := make(map[providerID]string, len(providerSettings))
	for providerIdentifier, providerSetting := range providerSettings {
		providerAPIKeys[providerIdentifier] = providerSetting.apiKey
	}
	return providerAPIKeys
}

func managedRecordSecretDigest(record managedTenantRecord) ([sha256.Size]byte, bool) {
	if record.SecretDigest == constants.EmptyString {
		return [sha256.Size]byte{}, false
	}
	digestBytes, decodeError := hex.DecodeString(record.SecretDigest)
	if decodeError != nil || len(digestBytes) != sha256.Size {
		return [sha256.Size]byte{}, false
	}
	var secretDigest [sha256.Size]byte
	copy(secretDigest[:], digestBytes)
	return secretDigest, true
}

func managedTenantID(userID string) string {
	userDigest := sha256.Sum256([]byte(userID))
	return managedTenantIdentifierPrefix + hex.EncodeToString(userDigest[:])[:managedTenantIdentifierHashBytes]
}

func maskedAPIKey(rawAPIKey string) string {
	apiKey := strings.TrimSpace(rawAPIKey)
	if len(apiKey) <= maskedSecretPrefixLength+maskedSecretSuffixLength {
		return "saved"
	}
	return apiKey[:maskedSecretPrefixLength] + "..." + apiKey[len(apiKey)-maskedSecretSuffixLength:]
}
