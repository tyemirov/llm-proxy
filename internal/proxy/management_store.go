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
	staticConfigMigrationID            = "static-config-v1"
	staticConfigTenantUserIDPrefix     = "static-config:"
	managedTenantIdentifierPrefix      = "managed-"
	managedTenantIdentifierHashBytes   = 12
	managedTenantUserEmailColumn       = "user_email"
	managedTenantUserIDColumn          = "user_id"
	managedUsageCreatedAtColumn        = "created_at"
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
)

type managedTenantStore struct {
	mutex             sync.RWMutex
	database          managedTenantDatabase
	providerKeyCipher managedProviderKeyCipher
	randomReader      io.Reader
	now               func() time.Time
}

type managedTenantDatabase interface {
	tenantByUserID(userID string) (managedTenantRecord, error)
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
	staticConfigMigrationByID(identifier string) (managedStaticConfigMigrationRecord, error)
	createStaticConfigMigration(record managedStaticConfigMigrationRecord) error
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

type managedStaticConfigMigrationRecord struct {
	ID        string `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type managedTenantSnapshot struct {
	userID          string
	userEmail       string
	userDisplayName string
	userAvatarURL   string
	tenantID        string
	hasSecret       bool
	providerAPIKeys map[providerID]string
	defaults        TenantDefaults
	createdAt       time.Time
	updatedAt       time.Time
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

func newManagedTenantStore(configuration ManagementConfiguration) (*managedTenantStore, error) {
	database, databaseError := newGORMManagedTenantDatabase(configuration)
	if databaseError != nil {
		return nil, databaseError
	}
	providerKeyCipher, cipherError := newManagedProviderKeyCipher(configuration.ProviderKeyEncryptionKey)
	if cipherError != nil {
		return nil, fmt.Errorf("%w: field=management.provider_key_encryption_key: %v", errManagedTenantStoreOpen, cipherError)
	}
	store := newManagedTenantStoreWithDatabaseAndCipher(database, providerKeyCipher)
	if migrationError := store.migratePlaintextProviderKeys(); migrationError != nil {
		return nil, migrationError
	}
	return store, nil
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
	if migrateError := database.AutoMigrate(&managedTenantRecord{}, &managedProviderAPIKeyRecord{}, &managedUsageEventRecord{}, &managedStaticConfigMigrationRecord{}); migrateError != nil {
		return nil, fmt.Errorf("%w: migrate: %v", errManagedTenantStoreOpen, migrateError)
	}
	return &gormManagedTenantDatabase{database: database}, nil
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

func (database *gormManagedTenantDatabase) staticConfigMigrationByID(identifier string) (managedStaticConfigMigrationRecord, error) {
	var record managedStaticConfigMigrationRecord
	queryError := database.database.
		Where(&managedStaticConfigMigrationRecord{ID: identifier}).
		First(&record).
		Error
	return record, queryError
}

func (database *gormManagedTenantDatabase) createStaticConfigMigration(record managedStaticConfigMigrationRecord) error {
	return database.database.Create(&record).Error
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

func (store *managedTenantStore) migrateStaticConfiguration(configuration Configuration) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if _, migrationError := store.database.staticConfigMigrationByID(staticConfigMigrationID); migrationError == nil {
		return nil
	} else if !errors.Is(migrationError, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: migration=%s: %v", errManagedTenantStorePersist, staticConfigMigrationID, migrationError)
	}

	timestamp := store.now()
	providerAPIKeys := configuredProviderAPIKeys(configuration)
	for _, legacyTenant := range configuration.tenants.tenants {
		record := newMigratedStaticTenantRecord(legacyTenant, timestamp)
		if persistError := store.database.saveTenant(record); persistError != nil {
			return fmt.Errorf("%w: tenant=%s: %v", errManagedTenantStorePersist, record.TenantID, persistError)
		}
		for providerIdentifier, apiKey := range providerAPIKeys {
			encryptedAPIKey, encryptionError := store.providerKeyCipher.encrypt(store.randomReader, record.UserID, providerIdentifier.string(), apiKey)
			if encryptionError != nil {
				return encryptionError
			}
			if persistError := store.database.saveProviderKey(managedProviderAPIKeyRecord{
				UserID:          record.UserID,
				ProviderID:      providerIdentifier.string(),
				EncryptedAPIKey: encryptedAPIKey,
				CreatedAt:       timestamp,
				UpdatedAt:       timestamp,
			}); persistError != nil {
				return fmt.Errorf("%w: tenant=%s provider=%s: %v", errManagedTenantStorePersist, record.TenantID, providerIdentifier.string(), persistError)
			}
		}
	}
	if persistError := store.database.createStaticConfigMigration(managedStaticConfigMigrationRecord{
		ID:        staticConfigMigrationID,
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
	}); persistError != nil {
		return fmt.Errorf("%w: migration=%s: %v", errManagedTenantStorePersist, staticConfigMigrationID, persistError)
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

func (store *managedTenantStore) saveProviderKey(principal managementPrincipal, providerIdentifier providerID, rawAPIKey string) (managedTenantSnapshot, error) {
	apiKey := strings.TrimSpace(rawAPIKey)
	if apiKey == constants.EmptyString {
		return managedTenantSnapshot{}, fmt.Errorf("%w: provider=%s", errManagedProviderKeyInvalid, providerIdentifier.string())
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.ensureRecordLocked(principal)
	if recordError != nil {
		return managedTenantSnapshot{}, recordError
	}
	timestamp := store.now()
	encryptedAPIKey, encryptionError := store.providerKeyCipher.encrypt(store.randomReader, record.UserID, providerIdentifier.string(), apiKey)
	if encryptionError != nil {
		return managedTenantSnapshot{}, encryptionError
	}
	if persistError := store.database.saveProviderKey(managedProviderAPIKeyRecord{
		UserID:          record.UserID,
		ProviderID:      providerIdentifier.string(),
		EncryptedAPIKey: encryptedAPIKey,
		CreatedAt:       timestamp,
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

func newMigratedStaticTenantRecord(staticTenant tenant, createdAt time.Time) managedTenantRecord {
	record := managedTenantRecord{
		UserID:       staticConfigTenantUserID(staticTenant.identifier),
		TenantID:     staticTenant.identifier.string(),
		SecretDigest: hex.EncodeToString(staticTenant.secretDigest[:]),
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	}
	record.applyDefaults(TenantDefaults{
		Provider:          staticTenant.defaults.provider,
		Model:             staticTenant.defaults.model,
		DictationProvider: staticTenant.defaults.dictationProvider,
		DictationModel:    staticTenant.defaults.dictationModel,
		SystemPrompt:      staticTenant.defaults.systemPrompt,
	})
	return record
}

func staticConfigTenantUserID(identifier tenantID) string {
	return staticConfigTenantUserIDPrefix + identifier.string()
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
	providerAPIKeys, providerKeyError := store.providerAPIKeyMap(record.ProviderAPIKeys)
	if providerKeyError != nil {
		return managedTenantSnapshot{}, providerKeyError
	}
	return managedTenantSnapshot{
		userID:          record.UserID,
		userEmail:       record.UserEmail,
		userDisplayName: record.UserDisplayName,
		userAvatarURL:   record.UserAvatarURL,
		tenantID:        record.TenantID,
		hasSecret:       record.SecretDigest != constants.EmptyString,
		providerAPIKeys: providerAPIKeys,
		defaults:        record.defaults(),
		createdAt:       record.CreatedAt,
		updatedAt:       record.UpdatedAt,
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
	providerAPIKeys, providerKeyError := store.providerAPIKeyMap(record.ProviderAPIKeys)
	if providerKeyError != nil {
		return tenant{}, providerKeyError
	}
	return tenant{
		identifier:      tenantID(record.TenantID),
		userID:          record.UserID,
		secretDigest:    secretDigest,
		defaults:        newTenantDefaults(record.defaults()),
		managed:         true,
		providerAPIKeys: providerAPIKeys,
	}, nil
}

func (store *managedTenantStore) providerAPIKeyMap(providerKeyRecords []managedProviderAPIKeyRecord) (map[providerID]string, error) {
	providerAPIKeys := make(map[providerID]string, len(providerKeyRecords))
	for _, providerKeyRecord := range providerKeyRecords {
		providerIdentifier := newProviderID(providerKeyRecord.ProviderID)
		if providerIdentifier.string() == constants.EmptyString {
			continue
		}
		apiKey, decryptError := store.providerKeyCipher.decrypt(providerKeyRecord)
		if decryptError != nil {
			return nil, decryptError
		}
		if providerIdentifier.string() != constants.EmptyString && apiKey != constants.EmptyString {
			providerAPIKeys[providerIdentifier] = apiKey
		}
	}
	return providerAPIKeys, nil
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
