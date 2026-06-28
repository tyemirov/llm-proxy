package proxy

import (
	"crypto/rand"
	"crypto/sha256"
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
)

const (
	generatedTenantSecretBytes       = 32
	generatedTenantSecretAttempts    = 16
	generatedTenantSecretPrefix      = "llmp_"
	staticConfigMigrationID          = "static-config-v1"
	staticConfigTenantUserIDPrefix   = "static-config:"
	managedTenantIdentifierPrefix    = "managed-"
	managedTenantIdentifierHashBytes = 12
	maskedSecretPrefixLength         = 3
	maskedSecretSuffixLength         = 4
)

var (
	errManagedTenantStoreOpen    = errors.New("managed_tenant_store_open_failed")
	errManagedTenantStorePersist = errors.New("managed_tenant_store_persist_failed")
	errManagedProviderKeyInvalid = errors.New("managed_provider_key_invalid")
	errManagedSecretGeneration   = errors.New("managed_secret_generation_failed")
	errManagedSecretCollision    = errors.New("managed_secret_collision")
)

type managedTenantStore struct {
	mutex        sync.RWMutex
	database     managedTenantDatabase
	randomReader io.Reader
	now          func() time.Time
}

type managedTenantDatabase interface {
	tenantByUserID(userID string) (managedTenantRecord, error)
	tenantBySecretDigest(secretDigest string) (managedTenantRecord, error)
	createTenant(record managedTenantRecord) error
	saveTenant(record managedTenantRecord) error
	saveProviderKey(record managedProviderAPIKeyRecord) error
	deleteProviderKey(record managedProviderAPIKeyRecord) error
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
	UserID     string `gorm:"primaryKey"`
	ProviderID string `gorm:"primaryKey"`
	APIKey     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
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

func newManagedTenantStore(configuration ManagementConfiguration) (*managedTenantStore, error) {
	database, databaseError := newGORMManagedTenantDatabase(configuration)
	if databaseError != nil {
		return nil, databaseError
	}
	return newManagedTenantStoreWithDatabase(database), nil
}

func newManagedTenantStoreWithDatabase(database managedTenantDatabase) *managedTenantStore {
	return &managedTenantStore{
		database:     database,
		randomReader: rand.Reader,
		now:          func() time.Time { return time.Now().UTC() },
	}
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
	if migrateError := database.AutoMigrate(&managedTenantRecord{}, &managedProviderAPIKeyRecord{}, &managedStaticConfigMigrationRecord{}); migrateError != nil {
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
			if persistError := store.database.saveProviderKey(managedProviderAPIKeyRecord{
				UserID:     record.UserID,
				ProviderID: providerIdentifier.string(),
				APIKey:     apiKey,
				CreatedAt:  timestamp,
				UpdatedAt:  timestamp,
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
	return record.snapshot(), nil
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
	if persistError := store.database.saveProviderKey(managedProviderAPIKeyRecord{
		UserID:     record.UserID,
		ProviderID: providerIdentifier.string(),
		APIKey:     apiKey,
		CreatedAt:  timestamp,
		UpdatedAt:  timestamp,
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
	return record.tenant(recordDigest), true
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
	return record.snapshot(), nil
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

func (record managedTenantRecord) snapshot() managedTenantSnapshot {
	return managedTenantSnapshot{
		userID:          record.UserID,
		userEmail:       record.UserEmail,
		userDisplayName: record.UserDisplayName,
		userAvatarURL:   record.UserAvatarURL,
		tenantID:        record.TenantID,
		hasSecret:       record.SecretDigest != constants.EmptyString,
		providerAPIKeys: record.providerAPIKeyMap(),
		defaults:        record.defaults(),
		createdAt:       record.CreatedAt,
		updatedAt:       record.UpdatedAt,
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

func (record managedTenantRecord) tenant(secretDigest [sha256.Size]byte) tenant {
	return tenant{
		identifier:      tenantID(record.TenantID),
		secretDigest:    secretDigest,
		defaults:        newTenantDefaults(record.defaults()),
		managed:         true,
		providerAPIKeys: record.providerAPIKeyMap(),
	}
}

func (record managedTenantRecord) providerAPIKeyMap() map[providerID]string {
	providerAPIKeys := make(map[providerID]string, len(record.ProviderAPIKeys))
	for _, providerKeyRecord := range record.ProviderAPIKeys {
		providerIdentifier := newProviderID(providerKeyRecord.ProviderID)
		apiKey := strings.TrimSpace(providerKeyRecord.APIKey)
		if providerIdentifier.string() != constants.EmptyString && apiKey != constants.EmptyString {
			providerAPIKeys[providerIdentifier] = apiKey
		}
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
