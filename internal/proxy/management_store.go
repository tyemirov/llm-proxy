package proxy

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"golang.org/x/sync/semaphore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	generatedTenantSecretBytes          = 32
	generatedTenantSecretAttempts       = 16
	generatedTenantSecretPrefix         = "llmp_"
	generatedTenantIdentifierBytes      = 16
	generatedTenantIdentifierAttempts   = 16
	managedTenantIdentifierPrefix       = "managed-"
	managedTenantNameMaximumCharacters  = 80
	managedTenantIDMaximumCharacters    = 128
	managedProviderKeyCiphertextPrefix  = "llmpk1:"
	maskedSecretPrefixLength            = 3
	maskedSecretSuffixLength            = 4
	managedUsageSummaryDays             = 30
	managedUsageReadBatchSize           = 256
	managedTenantStoreLockCapacity      = int64(1 << 30)
	managedTenantOwnershipSchemaVersion = 1
	managedTenantSchemaVersion          = 2

	managedUserTable                = "managed_user_records"
	managedTenantTable              = "managed_tenant_records"
	managedProviderKeyTable         = "managed_provider_api_key_records"
	managedUsageEventTable          = "managed_usage_event_records"
	managedSchemaMigrationTable     = "managed_schema_migration_records"
	legacyTenantMigrationTable      = "managed_tenant_records_f014_legacy"
	legacyProviderKeyMigrationTable = "managed_provider_api_key_records_f014_legacy"
	legacyUsageEventMigrationTable  = "managed_usage_event_records_f014_legacy"
	legacyTenantSecretDigestIndex   = "idx_managed_tenant_records_secret_digest"
	legacyUsageCreatedAtIndex       = "idx_managed_usage_created_at"
	migrationTenantSecretIndex      = "idx_f014_legacy_tenant_secret_digest"
	migrationUsageCreatedAtIndex    = "idx_f014_legacy_usage_created_at"
	obsoleteStaticMigrationTable    = "managed_static_config_migration_records"
	obsoleteRoutingMigrationTable   = "managed_routing_defaults_migration_records"
	legacyStaticTenantUserIDPrefix  = "static-config:"
	managedUsageIDColumn            = "id"
	managedUsageTenantIDColumn      = "tenant_id"
	managedUsageCreatedAtColumn     = "created_at"
	managedUsageSuccessColumn       = "success"
	managedUsageOutcomeCodeColumn   = "outcome_code"
	managedSchemaVersionColumn      = "version"
	managedUsageFailurePageIndex    = "idx_managed_usage_failure_page"
)

var (
	errManagedTenantStoreOpen       = errors.New("managed_tenant_store_open_failed")
	errManagedTenantStorePersist    = errors.New("managed_tenant_store_persist_failed")
	errManagedProviderKeyInvalid    = errors.New("managed_provider_key_invalid")
	errManagedProviderKeyEncryption = errors.New("managed_provider_key_encryption_failed")
	errManagedProviderKeyDecryption = errors.New("managed_provider_key_decryption_failed")
	errManagedProviderKeyNotFound   = errors.New("managed_provider_key_not_found")
	errManagedSecretGeneration      = errors.New("managed_secret_generation_failed")
	errManagedSecretCollision       = errors.New("managed_secret_collision")
	errManagedTenantIDGeneration    = errors.New("managed_tenant_id_generation_failed")
	errManagedTenantIDCollision     = errors.New("managed_tenant_id_collision")
	errManagedTenantIDInvalid       = errors.New("managed_tenant_id_invalid")
	errManagedTenantNameInvalid     = errors.New("managed_tenant_name_invalid")
	errManagedTenantNameConflict    = errors.New("managed_tenant_name_conflict")
	errManagedTenantNotFound        = errors.New("managed_tenant_not_found")
	errManagedFinalTenantDeletion   = errors.New("managed_final_tenant_deletion")
	errManagedTenantSchemaMigration = errors.New("managed_tenant_schema_migration_failed")
)

type managedTenantStore struct {
	mutex             *managedTenantStoreMutex
	database          managedTenantDatabase
	providerKeyCipher managedProviderKeyCipher
	routingDefaults   *providerRegistry
	randomReader      io.Reader
	now               func() time.Time
}

type managedTenantStoreMutex struct {
	weighted *semaphore.Weighted
}

func newManagedTenantStoreMutex() *managedTenantStoreMutex {
	return &managedTenantStoreMutex{weighted: semaphore.NewWeighted(managedTenantStoreLockCapacity)}
}

func (mutex *managedTenantStoreMutex) Lock() {
	_ = mutex.weighted.Acquire(context.Background(), managedTenantStoreLockCapacity)
}

func (mutex *managedTenantStoreMutex) LockContext(requestContext context.Context) error {
	return mutex.weighted.Acquire(requestContext, managedTenantStoreLockCapacity)
}

func (mutex *managedTenantStoreMutex) Unlock() {
	mutex.weighted.Release(managedTenantStoreLockCapacity)
}

func (mutex *managedTenantStoreMutex) RLock() {
	_ = mutex.weighted.Acquire(context.Background(), 1)
}

func (mutex *managedTenantStoreMutex) RUnlock() {
	mutex.weighted.Release(1)
}

type managedTenantIdentifier string

func newManagedTenantIdentifier(value string) (managedTenantIdentifier, error) {
	identifier := strings.TrimSpace(value)
	if identifier == constants.EmptyString || utf8.RuneCountInString(identifier) > managedTenantIDMaximumCharacters {
		return "", fmt.Errorf("%w: value=%q", errManagedTenantIDInvalid, value)
	}
	for _, character := range identifier {
		if unicode.IsControl(character) || unicode.IsSpace(character) || character == '/' {
			return "", fmt.Errorf("%w: value=%q", errManagedTenantIDInvalid, value)
		}
	}
	return managedTenantIdentifier(identifier), nil
}

func (identifier managedTenantIdentifier) string() string {
	return string(identifier)
}

type managedTenantName struct {
	display string
	key     string
}

func newManagedTenantName(value string) (managedTenantName, error) {
	displayName := strings.TrimSpace(value)
	characterCount := utf8.RuneCountInString(displayName)
	if characterCount == 0 || characterCount > managedTenantNameMaximumCharacters {
		return managedTenantName{}, fmt.Errorf("%w: length=%d", errManagedTenantNameInvalid, characterCount)
	}
	for _, character := range displayName {
		if unicode.IsControl(character) || unicode.In(character, unicode.Cf, unicode.Zl, unicode.Zp) {
			return managedTenantName{}, fmt.Errorf("%w: character=%U", errManagedTenantNameInvalid, character)
		}
	}
	return managedTenantName{display: displayName, key: strings.ToLower(displayName)}, nil
}

type managedUsageEventVisitor func(managedUsageEventRecord)

type managedTenantDatabase interface {
	userByID(userID string) (managedUserRecord, error)
	users() ([]managedUserRecord, error)
	saveUser(record managedUserRecord) error
	createUserAndTenant(user managedUserRecord, tenant managedTenantRecord) error
	tenantByOwnerAndID(ownerUserID string, tenantID string) (managedTenantRecord, error)
	tenantByTenantID(requestContext context.Context, tenantID string) (managedTenantRecord, error)
	tenantBySecretDigest(secretDigest string) (managedTenantRecord, error)
	tenantIDExists(tenantID string) (bool, error)
	tenantNameExists(ownerUserID string, nameKey string, excludedTenantID string) (bool, error)
	createTenant(record managedTenantRecord) error
	saveTenant(record managedTenantRecord) error
	deleteTenant(ownerUserID string, tenantID string) error
	providerKeys() ([]managedProviderAPIKeyRecord, error)
	saveProviderKey(ownerUserID string, record managedProviderAPIKeyRecord, updatedAt time.Time) error
	deleteProviderKey(ownerUserID string, tenantID string, providerID string, updatedAt time.Time) error
	createUsageEvent(requestContext context.Context, record managedUsageEventRecord) error
	earliestUsageEventByTenantIDsThrough(tenantIDs []string, periodEnd time.Time) (time.Time, error)
	streamUsageEventsByTenantIDsBetween(tenantIDs []string, periodStart time.Time, periodEnd time.Time, visit managedUsageEventVisitor) error
	streamUsageEventsByTenantIDsThrough(tenantIDs []string, periodEnd time.Time, visit managedUsageEventVisitor) error
	usageEventsSince(periodStart time.Time) ([]managedUsageEventRecord, error)
	usageFailuresByOwnerAndTenant(ownerUserID string, tenantID string, query managedUsageFailureRecordQuery) ([]managedUsageFailureRecord, uint, error)
	usageFailuresByTenantIDs(tenantIDs []string, query managedUsageFailureRecordQuery) ([]managedUsageFailureRecord, uint, error)
}

type gormManagedTenantDatabase struct {
	database *gorm.DB
}

type managedUserRecord struct {
	UserID          string `gorm:"primaryKey"`
	UserEmail       string
	UserDisplayName string
	UserAvatarURL   string
	Tenants         []managedTenantRecord `gorm:"foreignKey:OwnerUserID;references:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type managedTenantRecord struct {
	TenantID                 string  `gorm:"primaryKey"`
	OwnerUserID              string  `gorm:"not null;index:idx_managed_tenant_owner_created,priority:1;uniqueIndex:idx_managed_tenant_owner_name,priority:1"`
	Name                     string  `gorm:"not null"`
	NameKey                  string  `gorm:"not null;uniqueIndex:idx_managed_tenant_owner_name,priority:2"`
	SecretDigest             *string `gorm:"uniqueIndex"`
	DefaultProvider          string
	DefaultModel             string
	DefaultDictationProvider string
	DefaultDictationModel    string
	DefaultSystemPrompt      string
	DefaultReasoningEffort   string
	ProviderAPIKeys          []managedProviderAPIKeyRecord `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	UsageEvents              []managedUsageEventRecord     `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	CreatedAt                time.Time                     `gorm:"index:idx_managed_tenant_owner_created,priority:2"`
	UpdatedAt                time.Time
}

type managedProviderAPIKeyRecord struct {
	TenantID        string `gorm:"primaryKey"`
	ProviderID      string `gorm:"primaryKey"`
	EncryptedAPIKey string
	TextModel       string
	SystemPrompt    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type managedUsageEventRecord struct {
	ID                  uint   `gorm:"primaryKey;index:idx_managed_usage_failure_page,priority:4,sort:desc"`
	TenantID            string `gorm:"not null;index:idx_managed_usage_tenant_created,priority:1;index:idx_managed_usage_failure_page,priority:1"`
	Endpoint            string
	ProviderID          string
	ModelID             string
	StatusCode          int
	Success             bool                    `gorm:"index:idx_managed_usage_failure_page,priority:2"`
	OutcomeCode         managedUsageOutcomeCode `gorm:"not null"`
	LatencyMilliseconds int64
	RequestTokens       int
	ResponseTokens      int
	TotalTokens         int
	CreatedAt           time.Time `gorm:"index:idx_managed_usage_tenant_created,priority:2;index:idx_managed_usage_created_at;index:idx_managed_usage_failure_page,priority:3,sort:desc"`
}

type managedUsageEventSchemaOneRecord struct {
	ID                  uint   `gorm:"primaryKey"`
	TenantID            string `gorm:"not null"`
	Endpoint            string
	ProviderID          string
	ModelID             string
	StatusCode          int
	Success             bool
	LatencyMilliseconds int64
	RequestTokens       int
	ResponseTokens      int
	TotalTokens         int
	CreatedAt           time.Time
}

func (managedUsageEventSchemaOneRecord) TableName() string {
	return managedUsageEventTable
}

type managedUsageOutcomeMigrationRecord struct {
	OutcomeCode *managedUsageOutcomeCode `gorm:"column:outcome_code"`
}

func (managedUsageOutcomeMigrationRecord) TableName() string {
	return managedUsageEventTable
}

type managedSchemaMigrationRecord struct {
	Version   int `gorm:"primaryKey"`
	AppliedAt time.Time
}

type managedAccountSnapshot struct {
	userID          string
	userEmail       string
	userDisplayName string
	userAvatarURL   string
	tenants         []managedTenantSummary
}

type managedTenantSummary struct {
	tenantID  string
	name      string
	hasSecret bool
	createdAt time.Time
	updatedAt time.Time
}

type managedTenantSnapshot struct {
	ownerUserID      string
	tenantID         string
	tenantName       string
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
	tenants         []managedAdminTenantSnapshot
}

type managedAdminTenantSnapshot struct {
	tenantID  string
	name      string
	hasSecret bool
	createdAt time.Time
	updatedAt time.Time
	usage     managedAdminUsageSummary
}

type managedProviderKeyCipher struct {
	aeadCipher cipher.AEAD
}

func newManagedTenantStore(configuration ManagementConfiguration, providers *providerRegistry) (*managedTenantStore, error) {
	providerKeyCipher, cipherError := newManagedProviderKeyCipher(configuration.ProviderKeyEncryptionKey)
	if cipherError != nil {
		return nil, fmt.Errorf("%w: field=management.provider_key_encryption_key: %v", errManagedTenantStoreOpen, cipherError)
	}
	if _, defaultsError := validatePersistedManagedRoutingDefaults(providers, defaultManagedRoutingDefaults().value()); defaultsError != nil {
		return nil, fmt.Errorf("%w: default: %w", errManagedRoutingDefaultsMigration, defaultsError)
	}
	database, databaseError := newGORMManagedTenantDatabase(configuration, providerKeyCipher, providers)
	if databaseError != nil {
		return nil, databaseError
	}
	store := newManagedTenantStoreWithDatabaseAndCipher(database, providerKeyCipher)
	store.routingDefaults = providers
	return store, nil
}

func newManagedTenantStoreWithDatabase(database managedTenantDatabase) *managedTenantStore {
	return newManagedTenantStoreWithDatabaseAndCipher(database, internalManagedProviderKeyCipher())
}

func newManagedTenantStoreWithDatabaseAndCipher(database managedTenantDatabase, providerKeyCipher managedProviderKeyCipher) *managedTenantStore {
	return &managedTenantStore{
		mutex:             newManagedTenantStoreMutex(),
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

func (providerKeyCipher managedProviderKeyCipher) encrypt(randomReader io.Reader, tenantID string, providerIdentifier string, rawAPIKey string) (string, error) {
	apiKey := strings.TrimSpace(rawAPIKey)
	if apiKey == constants.EmptyString {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s", errManagedProviderKeyInvalid, providerIdentifier)
	}
	nonce := make([]byte, providerKeyCipher.aeadCipher.NonceSize())
	if _, readError := io.ReadFull(randomReader, nonce); readError != nil {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s: %v", errManagedProviderKeyEncryption, providerIdentifier, readError)
	}
	sealedAPIKey := providerKeyCipher.aeadCipher.Seal(nil, nonce, []byte(apiKey), managedProviderKeyAssociatedData(tenantID, providerIdentifier))
	sealedPayload := append(nonce, sealedAPIKey...)
	return managedProviderKeyCiphertextPrefix + base64.StdEncoding.EncodeToString(sealedPayload), nil
}

func (providerKeyCipher managedProviderKeyCipher) decrypt(record managedProviderAPIKeyRecord) (string, error) {
	return providerKeyCipher.decryptValue(record.EncryptedAPIKey, record.TenantID, record.ProviderID)
}

func (providerKeyCipher managedProviderKeyCipher) decryptValue(encryptedValue string, ownershipID string, providerIdentifier string) (string, error) {
	encryptedAPIKey := strings.TrimSpace(encryptedValue)
	if encryptedAPIKey == constants.EmptyString || !strings.HasPrefix(encryptedAPIKey, managedProviderKeyCiphertextPrefix) {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s", errManagedProviderKeyDecryption, providerIdentifier)
	}
	encodedPayload := strings.TrimPrefix(encryptedAPIKey, managedProviderKeyCiphertextPrefix)
	sealedPayload, decodeError := base64.StdEncoding.DecodeString(encodedPayload)
	if decodeError != nil {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s: %v", errManagedProviderKeyDecryption, providerIdentifier, decodeError)
	}
	if len(sealedPayload) <= providerKeyCipher.aeadCipher.NonceSize() {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s", errManagedProviderKeyDecryption, providerIdentifier)
	}
	nonce := sealedPayload[:providerKeyCipher.aeadCipher.NonceSize()]
	ciphertext := sealedPayload[providerKeyCipher.aeadCipher.NonceSize():]
	apiKeyBytes, decryptError := providerKeyCipher.aeadCipher.Open(nil, nonce, ciphertext, managedProviderKeyAssociatedData(ownershipID, providerIdentifier))
	if decryptError != nil {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s: %v", errManagedProviderKeyDecryption, providerIdentifier, decryptError)
	}
	return strings.TrimSpace(string(apiKeyBytes)), nil
}

func managedProviderKeyAssociatedData(tenantID string, providerIdentifier string) []byte {
	return []byte(strings.TrimSpace(tenantID) + "\x00" + strings.TrimSpace(providerIdentifier))
}

func newGORMManagedTenantDatabase(configuration ManagementConfiguration, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) (*gormManagedTenantDatabase, error) {
	database, openError := gorm.Open(managementDatabaseDialector(configuration), &gorm.Config{TranslateError: true})
	if openError != nil {
		return nil, fmt.Errorf("%w: %v", errManagedTenantStoreOpen, openError)
	}
	if initializeError := initializeManagedTenantSchema(database, providerKeyCipher, providers); initializeError != nil {
		return nil, fmt.Errorf("%w: %w", errManagedTenantStoreOpen, initializeError)
	}
	return &gormManagedTenantDatabase{database: database}, nil
}

func managementDatabaseDialector(configuration ManagementConfiguration) gorm.Dialector {
	if configuration.DatabaseDialector != nil {
		return configuration.DatabaseDialector
	}
	return sqlite.Open(configuration.DatabasePath)
}

func migrateCurrentManagedSchema(database *gorm.DB) error {
	return database.AutoMigrate(
		&managedUserRecord{},
		&managedTenantRecord{},
		&managedProviderAPIKeyRecord{},
		&managedUsageEventRecord{},
		&managedSchemaMigrationRecord{},
	)
}

func initializeManagedTenantSchema(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	migrator := database.Migrator()
	if !migrator.HasTable(managedTenantTable) {
		return database.Transaction(func(transaction *gorm.DB) error {
			if migrationError := migrateCurrentManagedSchema(transaction); migrationError != nil {
				return fmt.Errorf("%w: operation=create_current_schema: %v", errManagedTenantSchemaMigration, migrationError)
			}
			return transaction.Create(&managedSchemaMigrationRecord{Version: managedTenantSchemaVersion, AppliedAt: time.Now().UTC()}).Error
		})
	}
	if managedTableHasColumn(migrator, managedTenantTable, "user_id") {
		return migrateLegacyManagedTenantSchema(database, providerKeyCipher, providers)
	}
	if !managedTableHasColumn(migrator, managedTenantTable, "owner_user_id") || !migrator.HasTable(managedUserTable) {
		return fmt.Errorf("%w: operation=validate_current_schema table=%s", errManagedTenantSchemaMigration, managedTenantTable)
	}
	var migration managedSchemaMigrationRecord
	if queryError := database.Order(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: managedSchemaVersionColumn}, Desc: true}}}).First(&migration).Error; queryError != nil {
		return fmt.Errorf("%w: operation=read_version table=%s: %v", errManagedTenantSchemaMigration, managedSchemaMigrationTable, queryError)
	}
	switch migration.Version {
	case managedTenantOwnershipSchemaVersion:
		return migrateManagedUsageOutcomeSchema(database)
	case managedTenantSchemaVersion:
		if !managedTableHasColumn(migrator, managedUsageEventTable, managedUsageOutcomeCodeColumn) ||
			!migrator.HasIndex(&managedUsageEventRecord{}, managedUsageFailurePageIndex) {
			return fmt.Errorf("%w: operation=validate_current_schema table=%s", errManagedTenantSchemaMigration, managedUsageEventTable)
		}
		return nil
	default:
		return fmt.Errorf("%w: operation=validate_version version=%d expected=%d", errManagedTenantSchemaMigration, migration.Version, managedTenantSchemaVersion)
	}
}

type managedUsageOutcomeBackfill struct {
	recordID    uint
	outcomeCode managedUsageOutcomeCode
}

func migrateManagedUsageOutcomeSchema(database *gorm.DB) error {
	var schemaOneRecords []managedUsageEventSchemaOneRecord
	if queryError := database.
		Select("id", "success", "status_code").
		Order("id").
		Find(&schemaOneRecords).
		Error; queryError != nil {
		return fmt.Errorf("%w: operation=preflight table=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, queryError)
	}
	backfills := make([]managedUsageOutcomeBackfill, 0, len(schemaOneRecords))
	for _, record := range schemaOneRecords {
		outcomeCode, outcomeError := historicalManagedUsageOutcome(record.Success, record.StatusCode)
		if outcomeError != nil {
			return fmt.Errorf("%w: operation=preflight table=%s id=%d: %v", errManagedTenantSchemaMigration, managedUsageEventTable, record.ID, outcomeError)
		}
		backfills = append(backfills, managedUsageOutcomeBackfill{recordID: record.ID, outcomeCode: outcomeCode})
	}
	return database.Transaction(func(transaction *gorm.DB) error {
		migrator := transaction.Migrator()
		if addError := migrator.AddColumn(&managedUsageOutcomeMigrationRecord{}, "OutcomeCode"); addError != nil {
			return fmt.Errorf("%w: operation=add_column table=%s column=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, managedUsageOutcomeCodeColumn, addError)
		}
		for _, backfill := range backfills {
			result := transaction.Table(managedUsageEventTable).
				Where("id = ?", backfill.recordID).
				Update(managedUsageOutcomeCodeColumn, backfill.outcomeCode)
			if result.Error != nil || result.RowsAffected != 1 {
				return fmt.Errorf(
					"%w: operation=backfill table=%s id=%d rows=%d: %v",
					errManagedTenantSchemaMigration,
					managedUsageEventTable,
					backfill.recordID,
					result.RowsAffected,
					result.Error,
				)
			}
		}
		if alterError := migrator.AlterColumn(&managedUsageEventRecord{}, "OutcomeCode"); alterError != nil {
			return fmt.Errorf("%w: operation=require_column table=%s column=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, managedUsageOutcomeCodeColumn, alterError)
		}
		if indexError := migrator.CreateIndex(&managedUsageEventRecord{}, managedUsageFailurePageIndex); indexError != nil {
			return fmt.Errorf("%w: operation=create_index table=%s index=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, managedUsageFailurePageIndex, indexError)
		}
		var migratedRecords []managedUsageEventRecord
		if queryError := transaction.
			Select("id", managedUsageOutcomeCodeColumn).
			Order("id").
			Find(&migratedRecords).
			Error; queryError != nil {
			return fmt.Errorf("%w: operation=verify table=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, queryError)
		}
		if len(migratedRecords) != len(backfills) {
			return fmt.Errorf("%w: operation=verify table=%s count=%d expected=%d", errManagedTenantSchemaMigration, managedUsageEventTable, len(migratedRecords), len(backfills))
		}
		for recordIndex, record := range migratedRecords {
			expected := backfills[recordIndex]
			if record.ID != expected.recordID || record.OutcomeCode != expected.outcomeCode {
				return fmt.Errorf("%w: operation=verify table=%s id=%d", errManagedTenantSchemaMigration, managedUsageEventTable, expected.recordID)
			}
		}
		if createError := transaction.Create(&managedSchemaMigrationRecord{Version: managedTenantSchemaVersion, AppliedAt: time.Now().UTC()}).Error; createError != nil {
			return fmt.Errorf("%w: operation=record_version table=%s: %v", errManagedTenantSchemaMigration, managedSchemaMigrationTable, createError)
		}
		return nil
	})
}

func historicalManagedUsageOutcome(success bool, statusCode int) (managedUsageOutcomeCode, error) {
	if success {
		return managedUsageOutcomeSuccess, nil
	}
	switch statusCode {
	case http.StatusBadRequest:
		return managedUsageOutcomeInvalidRequest, nil
	case http.StatusRequestEntityTooLarge:
		return managedUsageOutcomePayloadTooLarge, nil
	case http.StatusTooManyRequests:
		return managedUsageOutcomeRateLimited, nil
	case http.StatusServiceUnavailable:
		return managedUsageOutcomeServiceUnavailable, nil
	case statusClientClosedRequest, http.StatusGatewayTimeout:
		return managedUsageOutcomeRequestTimeout, nil
	case http.StatusBadGateway:
		return managedUsageOutcomeUpstreamError, nil
	default:
		return "", fmt.Errorf("%w: success=false status_code=%d", errManagedUsageOutcomeInvalid, statusCode)
	}
}

func managedTableHasColumn(migrator gorm.Migrator, tableName string, columnName string) bool {
	columns, columnError := migrator.ColumnTypes(tableName)
	if columnError != nil {
		return false
	}
	for _, column := range columns {
		if strings.EqualFold(column.Name(), columnName) {
			return true
		}
	}
	return false
}

type legacyManagedTenantRecord struct {
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
	DefaultReasoningEffort   string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type legacyManagedProviderAPIKeyRecord struct {
	UserID          string `gorm:"primaryKey"`
	ProviderID      string `gorm:"primaryKey"`
	APIKey          string
	EncryptedAPIKey string
	TextModel       string
	SystemPrompt    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type legacyManagedUsageEventRecord struct {
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

type managedIndexRename struct {
	table  string
	source string
	target string
}

type managedTenantMigrationDataset struct {
	users              []managedUserRecord
	tenants            []managedTenantRecord
	providerKeys       []managedProviderAPIKeyRecord
	usageEvents        []managedUsageEventRecord
	decryptedAPIKeys   map[string]string
	legacyTenants      []legacyManagedTenantRecord
	legacyProviderKeys []legacyManagedProviderAPIKeyRecord
	legacyUsageEvents  []legacyManagedUsageEventRecord
}

func migrateLegacyManagedTenantSchema(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	dataset, preflightError := preflightLegacyManagedTenantSchema(database, providerKeyCipher, providers)
	if preflightError != nil {
		return preflightError
	}
	return database.Transaction(func(transaction *gorm.DB) error {
		migrator := transaction.Migrator()
		for _, rename := range legacyManagedIndexRenames() {
			if renameError := migrator.RenameIndex(rename.table, rename.source, rename.target); renameError != nil {
				return fmt.Errorf(
					"%w: operation=rename_index table=%s index=%s: %v",
					errManagedTenantSchemaMigration,
					rename.table,
					rename.source,
					renameError,
				)
			}
		}
		for _, rename := range []struct {
			source string
			target string
		}{
			{source: managedProviderKeyTable, target: legacyProviderKeyMigrationTable},
			{source: managedUsageEventTable, target: legacyUsageEventMigrationTable},
			{source: managedTenantTable, target: legacyTenantMigrationTable},
		} {
			if renameError := migrator.RenameTable(rename.source, rename.target); renameError != nil {
				return fmt.Errorf("%w: operation=rename table=%s: %v", errManagedTenantSchemaMigration, rename.source, renameError)
			}
		}
		if migrationError := migrateCurrentManagedSchema(transaction); migrationError != nil {
			return fmt.Errorf("%w: operation=create_current_schema: %v", errManagedTenantSchemaMigration, migrationError)
		}
		if len(dataset.users) != 0 {
			if createError := transaction.Create(&dataset.users).Error; createError != nil {
				return fmt.Errorf("%w: operation=create table=%s: %v", errManagedTenantSchemaMigration, managedUserTable, createError)
			}
			if createError := transaction.Create(&dataset.tenants).Error; createError != nil {
				return fmt.Errorf("%w: operation=create table=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, createError)
			}
		}
		if len(dataset.providerKeys) != 0 {
			if createError := transaction.Create(&dataset.providerKeys).Error; createError != nil {
				return fmt.Errorf("%w: operation=create table=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, createError)
			}
		}
		if len(dataset.usageEvents) != 0 {
			if createError := transaction.Create(&dataset.usageEvents).Error; createError != nil {
				return fmt.Errorf("%w: operation=create table=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, createError)
			}
		}
		if verifyError := verifyManagedTenantMigration(transaction, providerKeyCipher, dataset); verifyError != nil {
			return verifyError
		}
		if createError := transaction.Create(&managedSchemaMigrationRecord{Version: managedTenantSchemaVersion, AppliedAt: time.Now().UTC()}).Error; createError != nil {
			return fmt.Errorf("%w: operation=record_version table=%s: %v", errManagedTenantSchemaMigration, managedSchemaMigrationTable, createError)
		}
		for _, tableName := range []string{
			legacyProviderKeyMigrationTable,
			legacyUsageEventMigrationTable,
			legacyTenantMigrationTable,
			obsoleteRoutingMigrationTable,
			obsoleteStaticMigrationTable,
		} {
			if migrator.HasTable(tableName) {
				if dropError := migrator.DropTable(tableName); dropError != nil {
					return fmt.Errorf("%w: operation=drop table=%s: %v", errManagedTenantSchemaMigration, tableName, dropError)
				}
			}
		}
		return nil
	})
}

func legacyManagedIndexRenames() []managedIndexRename {
	return []managedIndexRename{
		{table: managedTenantTable, source: legacyTenantSecretDigestIndex, target: migrationTenantSecretIndex},
		{table: managedUsageEventTable, source: legacyUsageCreatedAtIndex, target: migrationUsageCreatedAtIndex},
	}
}

func preflightLegacyManagedTenantSchema(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) (managedTenantMigrationDataset, error) {
	migrator := database.Migrator()
	for _, tableName := range []string{managedProviderKeyTable, managedUsageEventTable} {
		if !migrator.HasTable(tableName) {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s missing", errManagedTenantSchemaMigration, tableName)
		}
	}
	var legacyTenants []legacyManagedTenantRecord
	if queryError := database.Table(managedTenantTable).Find(&legacyTenants).Error; queryError != nil {
		return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, queryError)
	}
	var legacyProviderKeys []legacyManagedProviderAPIKeyRecord
	if queryError := database.Table(managedProviderKeyTable).Find(&legacyProviderKeys).Error; queryError != nil {
		return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, queryError)
	}
	var legacyUsageEvents []legacyManagedUsageEventRecord
	if queryError := database.Table(managedUsageEventTable).Find(&legacyUsageEvents).Error; queryError != nil {
		return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, queryError)
	}

	dataset := managedTenantMigrationDataset{
		users:              make([]managedUserRecord, 0, len(legacyTenants)),
		tenants:            make([]managedTenantRecord, 0, len(legacyTenants)),
		providerKeys:       make([]managedProviderAPIKeyRecord, 0, len(legacyProviderKeys)),
		usageEvents:        make([]managedUsageEventRecord, 0, len(legacyUsageEvents)),
		decryptedAPIKeys:   make(map[string]string, len(legacyProviderKeys)),
		legacyTenants:      legacyTenants,
		legacyProviderKeys: legacyProviderKeys,
		legacyUsageEvents:  legacyUsageEvents,
	}
	tenantByUserID := make(map[string]legacyManagedTenantRecord, len(legacyTenants))
	seenTenantIDs := make(map[string]struct{}, len(legacyTenants))
	seenSecretDigests := make(map[string]struct{}, len(legacyTenants))
	defaultName, _ := newManagedTenantName("Default")
	for _, legacyTenant := range legacyTenants {
		if strings.TrimSpace(legacyTenant.UserID) == constants.EmptyString || strings.TrimSpace(legacyTenant.TenantID) == constants.EmptyString {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%q tenant=%q", errManagedTenantSchemaMigration, managedTenantTable, legacyTenant.UserID, legacyTenant.TenantID)
		}
		if strings.HasPrefix(legacyTenant.UserID, legacyStaticTenantUserIDPrefix) {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s: complete the F011 ownership claim first", errManagedTenantSchemaMigration, managedTenantTable, legacyTenant.UserID, legacyTenant.TenantID)
		}
		if _, duplicate := tenantByUserID[legacyTenant.UserID]; duplicate {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s duplicate_user=%s", errManagedTenantSchemaMigration, managedTenantTable, legacyTenant.UserID)
		}
		if _, duplicate := seenTenantIDs[legacyTenant.TenantID]; duplicate {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s duplicate_tenant=%s", errManagedTenantSchemaMigration, managedTenantTable, legacyTenant.TenantID)
		}
		if _, identifierError := newManagedTenantIdentifier(legacyTenant.TenantID); identifierError != nil {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, legacyTenant.UserID, legacyTenant.TenantID, identifierError)
		}
		if _, defaultsError := validatePersistedManagedRoutingDefaults(providers, legacyTenant.defaults()); defaultsError != nil {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, legacyTenant.UserID, legacyTenant.TenantID, defaultsError)
		}
		var secretDigest *string
		if legacyTenant.SecretDigest != constants.EmptyString {
			if _, duplicate := seenSecretDigests[legacyTenant.SecretDigest]; duplicate {
				return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s duplicate_secret_digest tenant=%s", errManagedTenantSchemaMigration, managedTenantTable, legacyTenant.TenantID)
			}
			if !validManagedSecretDigest(legacyTenant.SecretDigest) {
				return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s invalid_secret_digest tenant=%s", errManagedTenantSchemaMigration, managedTenantTable, legacyTenant.TenantID)
			}
			digest := legacyTenant.SecretDigest
			secretDigest = &digest
			seenSecretDigests[digest] = struct{}{}
		}
		tenantByUserID[legacyTenant.UserID] = legacyTenant
		seenTenantIDs[legacyTenant.TenantID] = struct{}{}
		dataset.users = append(dataset.users, managedUserRecord{
			UserID:          legacyTenant.UserID,
			UserEmail:       legacyTenant.UserEmail,
			UserDisplayName: legacyTenant.UserDisplayName,
			UserAvatarURL:   legacyTenant.UserAvatarURL,
			CreatedAt:       legacyTenant.CreatedAt,
			UpdatedAt:       legacyTenant.UpdatedAt,
		})
		dataset.tenants = append(dataset.tenants, managedTenantRecord{
			TenantID:                 legacyTenant.TenantID,
			OwnerUserID:              legacyTenant.UserID,
			Name:                     defaultName.display,
			NameKey:                  defaultName.key,
			SecretDigest:             secretDigest,
			DefaultProvider:          legacyTenant.DefaultProvider,
			DefaultModel:             legacyTenant.DefaultModel,
			DefaultDictationProvider: legacyTenant.DefaultDictationProvider,
			DefaultDictationModel:    legacyTenant.DefaultDictationModel,
			DefaultSystemPrompt:      legacyTenant.DefaultSystemPrompt,
			DefaultReasoningEffort:   legacyTenant.DefaultReasoningEffort,
			CreatedAt:                legacyTenant.CreatedAt,
			UpdatedAt:                legacyTenant.UpdatedAt,
		})
	}
	for _, legacyProviderKey := range legacyProviderKeys {
		ownerTenant, ownerExists := tenantByUserID[legacyProviderKey.UserID]
		if !ownerExists {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s orphan_user=%s provider=%s", errManagedTenantSchemaMigration, managedProviderKeyTable, legacyProviderKey.UserID, legacyProviderKey.ProviderID)
		}
		providerIdentifier, providerError := providers.canonicalProviderID(legacyProviderKey.ProviderID)
		if providerError != nil {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, legacyProviderKey.UserID, ownerTenant.TenantID, legacyProviderKey.ProviderID, providerError)
		}
		if _, _, modelError := providers.resolveTextModel(providerIdentifier.string(), legacyProviderKey.TextModel, providerIdentifier.string(), legacyProviderKey.TextModel, false); modelError != nil {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, legacyProviderKey.UserID, ownerTenant.TenantID, legacyProviderKey.ProviderID, modelError)
		}
		if strings.TrimSpace(legacyProviderKey.APIKey) != constants.EmptyString {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s provider=%s plaintext_key", errManagedTenantSchemaMigration, managedProviderKeyTable, legacyProviderKey.UserID, ownerTenant.TenantID, legacyProviderKey.ProviderID)
		}
		apiKey, decryptError := providerKeyCipher.decryptValue(legacyProviderKey.EncryptedAPIKey, legacyProviderKey.UserID, legacyProviderKey.ProviderID)
		if decryptError != nil {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, legacyProviderKey.UserID, ownerTenant.TenantID, legacyProviderKey.ProviderID, decryptError)
		}
		encryptedAPIKey, encryptionError := providerKeyCipher.encrypt(rand.Reader, ownerTenant.TenantID, legacyProviderKey.ProviderID, apiKey)
		if encryptionError != nil {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, legacyProviderKey.UserID, ownerTenant.TenantID, legacyProviderKey.ProviderID, encryptionError)
		}
		key := ownerTenant.TenantID + "\x00" + legacyProviderKey.ProviderID
		dataset.decryptedAPIKeys[key] = apiKey
		dataset.providerKeys = append(dataset.providerKeys, managedProviderAPIKeyRecord{
			TenantID:        ownerTenant.TenantID,
			ProviderID:      legacyProviderKey.ProviderID,
			EncryptedAPIKey: encryptedAPIKey,
			TextModel:       legacyProviderKey.TextModel,
			SystemPrompt:    legacyProviderKey.SystemPrompt,
			CreatedAt:       legacyProviderKey.CreatedAt,
			UpdatedAt:       legacyProviderKey.UpdatedAt,
		})
	}
	for _, legacyUsageEvent := range legacyUsageEvents {
		ownerTenant, ownerExists := tenantByUserID[legacyUsageEvent.UserID]
		if !ownerExists || ownerTenant.TenantID != legacyUsageEvent.TenantID {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s", errManagedTenantSchemaMigration, managedUsageEventTable, legacyUsageEvent.UserID, legacyUsageEvent.TenantID)
		}
		outcomeCode, outcomeError := historicalManagedUsageOutcome(legacyUsageEvent.Success, legacyUsageEvent.StatusCode)
		if outcomeError != nil {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s id=%d: %v", errManagedTenantSchemaMigration, managedUsageEventTable, legacyUsageEvent.ID, outcomeError)
		}
		dataset.usageEvents = append(dataset.usageEvents, managedUsageEventRecord{
			ID:                  legacyUsageEvent.ID,
			TenantID:            legacyUsageEvent.TenantID,
			Endpoint:            legacyUsageEvent.Endpoint,
			ProviderID:          legacyUsageEvent.ProviderID,
			ModelID:             legacyUsageEvent.ModelID,
			StatusCode:          legacyUsageEvent.StatusCode,
			Success:             legacyUsageEvent.Success,
			OutcomeCode:         outcomeCode,
			LatencyMilliseconds: legacyUsageEvent.LatencyMilliseconds,
			RequestTokens:       legacyUsageEvent.RequestTokens,
			ResponseTokens:      legacyUsageEvent.ResponseTokens,
			TotalTokens:         legacyUsageEvent.TotalTokens,
			CreatedAt:           legacyUsageEvent.CreatedAt,
		})
	}
	return dataset, nil
}

func verifyManagedTenantMigration(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, dataset managedTenantMigrationDataset) error {
	countChecks := []struct {
		model    interface{}
		table    string
		expected int
	}{
		{model: &managedUserRecord{}, table: managedUserTable, expected: len(dataset.users)},
		{model: &managedTenantRecord{}, table: managedTenantTable, expected: len(dataset.tenants)},
		{model: &managedProviderAPIKeyRecord{}, table: managedProviderKeyTable, expected: len(dataset.providerKeys)},
		{model: &managedUsageEventRecord{}, table: managedUsageEventTable, expected: len(dataset.usageEvents)},
	}
	for _, check := range countChecks {
		var count int64
		if countError := database.Model(check.model).Count(&count).Error; countError != nil || count != int64(check.expected) {
			return fmt.Errorf("%w: operation=verify table=%s count=%d expected=%d error=%v", errManagedTenantSchemaMigration, check.table, count, check.expected, countError)
		}
	}
	for _, expectedTenant := range dataset.tenants {
		var actualTenant managedTenantRecord
		if queryError := database.Where(&managedTenantRecord{TenantID: expectedTenant.TenantID, OwnerUserID: expectedTenant.OwnerUserID}).First(&actualTenant).Error; queryError != nil {
			return fmt.Errorf("%w: operation=verify table=%s user=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, expectedTenant.OwnerUserID, expectedTenant.TenantID, queryError)
		}
		if actualTenant.Name != expectedTenant.Name || actualTenant.NameKey != expectedTenant.NameKey ||
			managedSecretDigestValue(actualTenant.SecretDigest) != managedSecretDigestValue(expectedTenant.SecretDigest) ||
			actualTenant.defaults() != expectedTenant.defaults() ||
			!actualTenant.CreatedAt.Equal(expectedTenant.CreatedAt) || !actualTenant.UpdatedAt.Equal(expectedTenant.UpdatedAt) {
			return fmt.Errorf("%w: operation=verify table=%s user=%s tenant=%s values", errManagedTenantSchemaMigration, managedTenantTable, expectedTenant.OwnerUserID, expectedTenant.TenantID)
		}
	}
	for _, expectedProviderKey := range dataset.providerKeys {
		var actualProviderKey managedProviderAPIKeyRecord
		if queryError := database.Where(&managedProviderAPIKeyRecord{TenantID: expectedProviderKey.TenantID, ProviderID: expectedProviderKey.ProviderID}).First(&actualProviderKey).Error; queryError != nil {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, expectedProviderKey.TenantID, expectedProviderKey.ProviderID, queryError)
		}
		apiKey, decryptError := providerKeyCipher.decrypt(actualProviderKey)
		expectedAPIKey := dataset.decryptedAPIKeys[expectedProviderKey.TenantID+"\x00"+expectedProviderKey.ProviderID]
		if decryptError != nil || apiKey != expectedAPIKey || actualProviderKey.TextModel != expectedProviderKey.TextModel ||
			actualProviderKey.SystemPrompt != expectedProviderKey.SystemPrompt ||
			!actualProviderKey.CreatedAt.Equal(expectedProviderKey.CreatedAt) || !actualProviderKey.UpdatedAt.Equal(expectedProviderKey.UpdatedAt) {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s provider=%s", errManagedTenantSchemaMigration, managedProviderKeyTable, expectedProviderKey.TenantID, expectedProviderKey.ProviderID)
		}
	}
	var actualUsageEvents []managedUsageEventRecord
	if queryError := database.Order("id").Find(&actualUsageEvents).Error; queryError != nil {
		return fmt.Errorf("%w: operation=verify table=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, queryError)
	}
	expectedUsageEvents := append([]managedUsageEventRecord(nil), dataset.usageEvents...)
	sort.Slice(expectedUsageEvents, func(first int, second int) bool {
		return expectedUsageEvents[first].ID < expectedUsageEvents[second].ID
	})
	for index, expectedUsageEvent := range expectedUsageEvents {
		actualUsageEvent := actualUsageEvents[index]
		if actualUsageEvent != expectedUsageEvent {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s row=%d", errManagedTenantSchemaMigration, managedUsageEventTable, expectedUsageEvent.TenantID, expectedUsageEvent.ID)
		}
	}
	return nil
}

func (record legacyManagedTenantRecord) defaults() TenantDefaults {
	return TenantDefaults{
		Provider:          record.DefaultProvider,
		Model:             record.DefaultModel,
		DictationProvider: record.DefaultDictationProvider,
		DictationModel:    record.DefaultDictationModel,
		SystemPrompt:      record.DefaultSystemPrompt,
		ReasoningEffort:   record.DefaultReasoningEffort,
	}
}

func (database *gormManagedTenantDatabase) userByID(userID string) (managedUserRecord, error) {
	var record managedUserRecord
	queryError := database.database.
		Preload("Tenants", func(query *gorm.DB) *gorm.DB { return query.Order("created_at, tenant_id") }).
		Preload("Tenants.ProviderAPIKeys").
		Where(&managedUserRecord{UserID: userID}).
		First(&record).
		Error
	return record, queryError
}

func (database *gormManagedTenantDatabase) users() ([]managedUserRecord, error) {
	var records []managedUserRecord
	queryError := database.database.
		Preload("Tenants", func(query *gorm.DB) *gorm.DB { return query.Order("created_at, tenant_id") }).
		Order("LOWER(user_email), user_id").
		Find(&records).
		Error
	return records, queryError
}

func (database *gormManagedTenantDatabase) saveUser(record managedUserRecord) error {
	result := database.database.Model(&managedUserRecord{}).
		Where(&managedUserRecord{UserID: record.UserID}).
		Updates(map[string]interface{}{
			"user_email":        record.UserEmail,
			"user_display_name": record.UserDisplayName,
			"user_avatar_url":   record.UserAvatarURL,
			"updated_at":        record.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (database *gormManagedTenantDatabase) createUserAndTenant(user managedUserRecord, tenant managedTenantRecord) error {
	return database.database.Transaction(func(transaction *gorm.DB) error {
		if createError := transaction.Create(&user).Error; createError != nil {
			return createError
		}
		return transaction.Create(&tenant).Error
	})
}

func (database *gormManagedTenantDatabase) tenantByOwnerAndID(ownerUserID string, tenantID string) (managedTenantRecord, error) {
	var record managedTenantRecord
	queryError := database.database.
		Preload("ProviderAPIKeys").
		Where(&managedTenantRecord{OwnerUserID: ownerUserID, TenantID: tenantID}).
		First(&record).
		Error
	return record, queryError
}

func (database *gormManagedTenantDatabase) tenantByTenantID(requestContext context.Context, tenantID string) (managedTenantRecord, error) {
	var record managedTenantRecord
	queryError := database.database.WithContext(requestContext).
		Where(&managedTenantRecord{TenantID: tenantID}).
		First(&record).
		Error
	return record, queryError
}

func (database *gormManagedTenantDatabase) tenantBySecretDigest(secretDigest string) (managedTenantRecord, error) {
	var record managedTenantRecord
	queryError := database.database.
		Preload("ProviderAPIKeys").
		Where("secret_digest = ?", secretDigest).
		First(&record).
		Error
	return record, queryError
}

func (database *gormManagedTenantDatabase) tenantIDExists(tenantID string) (bool, error) {
	var count int64
	queryError := database.database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: tenantID}).Count(&count).Error
	return count != 0, queryError
}

func (database *gormManagedTenantDatabase) tenantNameExists(ownerUserID string, nameKey string, excludedTenantID string) (bool, error) {
	query := database.database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{OwnerUserID: ownerUserID, NameKey: nameKey})
	if excludedTenantID != constants.EmptyString {
		query = query.Not(&managedTenantRecord{TenantID: excludedTenantID})
	}
	var count int64
	queryError := query.Count(&count).Error
	return count != 0, queryError
}

func (database *gormManagedTenantDatabase) createTenant(record managedTenantRecord) error {
	return database.database.Create(&record).Error
}

func (database *gormManagedTenantDatabase) saveTenant(record managedTenantRecord) error {
	result := database.database.Model(&managedTenantRecord{}).
		Where(&managedTenantRecord{OwnerUserID: record.OwnerUserID, TenantID: record.TenantID}).
		Updates(map[string]interface{}{
			"name":                       record.Name,
			"name_key":                   record.NameKey,
			"secret_digest":              record.SecretDigest,
			"default_provider":           record.DefaultProvider,
			"default_model":              record.DefaultModel,
			"default_dictation_provider": record.DefaultDictationProvider,
			"default_dictation_model":    record.DefaultDictationModel,
			"default_system_prompt":      record.DefaultSystemPrompt,
			"default_reasoning_effort":   record.DefaultReasoningEffort,
			"updated_at":                 record.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (database *gormManagedTenantDatabase) deleteTenant(ownerUserID string, tenantID string) error {
	return database.database.Transaction(func(transaction *gorm.DB) error {
		var tenantRecord managedTenantRecord
		if queryError := transaction.Where(&managedTenantRecord{OwnerUserID: ownerUserID, TenantID: tenantID}).First(&tenantRecord).Error; queryError != nil {
			return queryError
		}
		var tenantCount int64
		if countError := transaction.Model(&managedTenantRecord{}).Where(&managedTenantRecord{OwnerUserID: ownerUserID}).Count(&tenantCount).Error; countError != nil {
			return countError
		}
		if tenantCount <= 1 {
			return errManagedFinalTenantDeletion
		}
		if deleteError := transaction.Where(&managedProviderAPIKeyRecord{TenantID: tenantID}).Delete(&managedProviderAPIKeyRecord{}).Error; deleteError != nil {
			return deleteError
		}
		if deleteError := transaction.Where(&managedUsageEventRecord{TenantID: tenantID}).Delete(&managedUsageEventRecord{}).Error; deleteError != nil {
			return deleteError
		}
		result := transaction.Where(&managedTenantRecord{OwnerUserID: ownerUserID, TenantID: tenantID}).Delete(&managedTenantRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (database *gormManagedTenantDatabase) providerKeys() ([]managedProviderAPIKeyRecord, error) {
	var records []managedProviderAPIKeyRecord
	queryError := database.database.Find(&records).Error
	return records, queryError
}

func (database *gormManagedTenantDatabase) saveProviderKey(ownerUserID string, record managedProviderAPIKeyRecord, updatedAt time.Time) error {
	return database.database.Transaction(func(transaction *gorm.DB) error {
		var tenantRecord managedTenantRecord
		if queryError := transaction.Where(&managedTenantRecord{OwnerUserID: ownerUserID, TenantID: record.TenantID}).First(&tenantRecord).Error; queryError != nil {
			return queryError
		}
		if saveError := transaction.Save(&record).Error; saveError != nil {
			return saveError
		}
		return transaction.Model(&managedTenantRecord{}).
			Where(&managedTenantRecord{OwnerUserID: ownerUserID, TenantID: record.TenantID}).
			Update("updated_at", updatedAt).
			Error
	})
}

func (database *gormManagedTenantDatabase) deleteProviderKey(ownerUserID string, tenantID string, providerID string, updatedAt time.Time) error {
	return database.database.Transaction(func(transaction *gorm.DB) error {
		var tenantRecord managedTenantRecord
		if queryError := transaction.Where(&managedTenantRecord{OwnerUserID: ownerUserID, TenantID: tenantID}).First(&tenantRecord).Error; queryError != nil {
			return queryError
		}
		if deleteError := transaction.Where(&managedProviderAPIKeyRecord{TenantID: tenantID, ProviderID: providerID}).Delete(&managedProviderAPIKeyRecord{}).Error; deleteError != nil {
			return deleteError
		}
		return transaction.Model(&managedTenantRecord{}).
			Where(&managedTenantRecord{OwnerUserID: ownerUserID, TenantID: tenantID}).
			Update("updated_at", updatedAt).
			Error
	})
}

func (database *gormManagedTenantDatabase) createUsageEvent(requestContext context.Context, record managedUsageEventRecord) error {
	return database.database.WithContext(requestContext).Create(&record).Error
}

func (database *gormManagedTenantDatabase) earliestUsageEventByTenantIDsThrough(tenantIDs []string, periodEnd time.Time) (time.Time, error) {
	var records []managedUsageEventRecord
	queryResult := database.database.
		Where(clause.IN{Column: clause.Column{Name: managedUsageTenantIDColumn}, Values: stringInterfaceValues(tenantIDs)}).
		Where(clause.Lte{Column: clause.Column{Name: managedUsageCreatedAtColumn}, Value: periodEnd}).
		Order("created_at, id").
		Limit(1).
		Find(&records)
	if len(records) == 0 || queryResult.Error != nil {
		return time.Time{}, queryResult.Error
	}
	return records[0].CreatedAt, nil
}

func (database *gormManagedTenantDatabase) streamUsageEventsByTenantIDsBetween(tenantIDs []string, periodStart time.Time, periodEnd time.Time, visit managedUsageEventVisitor) error {
	query := database.database.
		Where(clause.IN{Column: clause.Column{Name: managedUsageTenantIDColumn}, Values: stringInterfaceValues(tenantIDs)}).
		Where(clause.Gte{Column: clause.Column{Name: managedUsageCreatedAtColumn}, Value: periodStart}).
		Where(clause.Lte{Column: clause.Column{Name: managedUsageCreatedAtColumn}, Value: periodEnd})
	return database.streamUsageEvents(query, visit)
}

func (database *gormManagedTenantDatabase) streamUsageEventsByTenantIDsThrough(tenantIDs []string, periodEnd time.Time, visit managedUsageEventVisitor) error {
	query := database.database.
		Where(clause.IN{Column: clause.Column{Name: managedUsageTenantIDColumn}, Values: stringInterfaceValues(tenantIDs)}).
		Where(clause.Lte{Column: clause.Column{Name: managedUsageCreatedAtColumn}, Value: periodEnd})
	return database.streamUsageEvents(query, visit)
}

func stringInterfaceValues(values []string) []interface{} {
	interfaceValues := make([]interface{}, 0, len(values))
	for _, value := range values {
		interfaceValues = append(interfaceValues, value)
	}
	return interfaceValues
}

func (database *gormManagedTenantDatabase) streamUsageEvents(query *gorm.DB, visit managedUsageEventVisitor) error {
	records := make([]managedUsageEventRecord, 0, managedUsageReadBatchSize)
	return query.FindInBatches(&records, managedUsageReadBatchSize, func(_ *gorm.DB, _ int) error {
		for _, record := range records {
			visit(record)
		}
		return nil
	}).Error
}

func (database *gormManagedTenantDatabase) usageEventsSince(periodStart time.Time) ([]managedUsageEventRecord, error) {
	var records []managedUsageEventRecord
	queryError := database.database.
		Where(clause.Gte{Column: clause.Column{Name: managedUsageCreatedAtColumn}, Value: periodStart}).
		Find(&records).
		Error
	return records, queryError
}

func (database *gormManagedTenantDatabase) usageFailuresByOwnerAndTenant(ownerUserID string, tenantID string, query managedUsageFailureRecordQuery) ([]managedUsageFailureRecord, uint, error) {
	var records []managedUsageFailureRecord
	var resolvedSnapshotID uint
	transactionError := database.database.Transaction(func(transaction *gorm.DB) error {
		var tenantRecord managedTenantRecord
		if tenantError := transaction.
			Select(managedUsageTenantIDColumn, "name").
			Where(&managedTenantRecord{OwnerUserID: ownerUserID, TenantID: tenantID}).
			First(&tenantRecord).
			Error; tenantError != nil {
			return tenantError
		}
		var recordsError error
		records, resolvedSnapshotID, recordsError = usageFailuresByTenantIDsInTransaction(
			transaction,
			[]string{tenantID},
			query,
		)
		if recordsError != nil {
			return recordsError
		}
		for recordIndex := range records {
			records[recordIndex].failure.tenantName = tenantRecord.Name
		}
		return nil
	})
	return records, resolvedSnapshotID, transactionError
}

func (database *gormManagedTenantDatabase) usageFailuresByTenantIDs(tenantIDs []string, query managedUsageFailureRecordQuery) ([]managedUsageFailureRecord, uint, error) {
	var records []managedUsageFailureRecord
	var resolvedSnapshotID uint
	transactionError := database.database.Transaction(func(transaction *gorm.DB) error {
		var recordsError error
		records, resolvedSnapshotID, recordsError = usageFailuresByTenantIDsInTransaction(transaction, tenantIDs, query)
		return recordsError
	})
	return records, resolvedSnapshotID, transactionError
}

func usageFailuresByTenantIDsInTransaction(transaction *gorm.DB, tenantIDs []string, query managedUsageFailureRecordQuery) ([]managedUsageFailureRecord, uint, error) {
	resolvedSnapshotID := uint(0)
	if query.snapshotID != nil {
		resolvedSnapshotID = *query.snapshotID
	} else {
		var snapshotRecords []managedUsageEventRecord
		snapshotError := transaction.
			Select(managedUsageIDColumn).
			Where(clause.IN{Column: clause.Column{Name: managedUsageTenantIDColumn}, Values: stringInterfaceValues(tenantIDs)}).
			Where(clause.Lte{Column: clause.Column{Name: managedUsageCreatedAtColumn}, Value: query.snapshotAt}).
			Order("id DESC").
			Limit(1).
			Find(&snapshotRecords).
			Error
		if snapshotError != nil {
			return nil, 0, snapshotError
		}
		if len(snapshotRecords) != 0 {
			resolvedSnapshotID = snapshotRecords[0].ID
		}
	}

	usageQuery := transaction.
		Where(clause.IN{Column: clause.Column{Name: managedUsageTenantIDColumn}, Values: stringInterfaceValues(tenantIDs)}).
		Where(clause.Eq{Column: clause.Column{Name: managedUsageSuccessColumn}, Value: false}).
		Where(clause.Lte{Column: clause.Column{Name: managedUsageCreatedAtColumn}, Value: query.snapshotAt}).
		Where(clause.Lte{Column: clause.Column{Name: managedUsageIDColumn}, Value: resolvedSnapshotID})
	if query.periodStart != nil {
		usageQuery = usageQuery.Where(clause.Gte{Column: clause.Column{Name: managedUsageCreatedAtColumn}, Value: *query.periodStart})
	}
	if query.position != nil {
		usageQuery = usageQuery.Where(
			"(created_at < ?) OR (created_at = ? AND id < ?)",
			query.position.occurredAt,
			query.position.occurredAt,
			query.position.recordID,
		)
	}
	var usageRecords []managedUsageEventRecord
	if recordsError := usageQuery.
		Order("created_at DESC, id DESC").
		Limit(query.limit).
		Find(&usageRecords).
		Error; recordsError != nil {
		return nil, 0, recordsError
	}
	records := make([]managedUsageFailureRecord, 0, len(usageRecords))
	for _, record := range usageRecords {
		outcomeCode, outcomeError := newManagedUsageOutcomeCode(string(record.OutcomeCode))
		if outcomeError != nil {
			return nil, 0, fmt.Errorf("%w: table=%s id=%d: %v", errManagedTenantStorePersist, managedUsageEventTable, record.ID, outcomeError)
		}
		records = append(records, managedUsageFailureRecord{
			recordID: record.ID,
			failure: managedUsageFailure{
				tenantIdentifier:    record.TenantID,
				occurredAt:          record.CreatedAt.UTC(),
				endpoint:            record.Endpoint,
				providerIdentifier:  record.ProviderID,
				modelIdentifier:     record.ModelID,
				statusCode:          record.StatusCode,
				outcomeCode:         outcomeCode,
				latencyMilliseconds: record.LatencyMilliseconds,
			},
		})
	}
	return records, resolvedSnapshotID, nil
}

func (store *managedTenantStore) account(principal managementPrincipal) (managedAccountSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.ensureUserLocked(principal)
	if recordError != nil {
		return managedAccountSnapshot{}, recordError
	}
	return managedAccountSnapshotFor(record), nil
}

func (store *managedTenantStore) tenantProfile(principal managementPrincipal, tenantIdentifier managedTenantIdentifier) (managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if _, ensureError := store.ensureUserLocked(principal); ensureError != nil {
		return managedTenantSnapshot{}, ensureError
	}
	record, recordError := store.database.tenantByOwnerAndID(principal.userID, tenantIdentifier.string())
	if recordError != nil {
		return managedTenantSnapshot{}, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordError)
	}
	return store.snapshot(record)
}

func (store *managedTenantStore) createTenant(principal managementPrincipal, name managedTenantName) (managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if _, ensureError := store.ensureUserLocked(principal); ensureError != nil {
		return managedTenantSnapshot{}, ensureError
	}
	nameExists, nameQueryError := store.database.tenantNameExists(principal.userID, name.key, constants.EmptyString)
	if nameQueryError != nil {
		return managedTenantSnapshot{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, principal.userID, nameQueryError)
	}
	if nameExists {
		return managedTenantSnapshot{}, errManagedTenantNameConflict
	}
	for attempt := 0; attempt < generatedTenantIdentifierAttempts; attempt++ {
		tenantIdentifier, identifierError := store.newTenantIdentifier()
		if identifierError != nil {
			return managedTenantSnapshot{}, identifierError
		}
		identifierExists, queryError := store.database.tenantIDExists(tenantIdentifier.string())
		if queryError != nil {
			return managedTenantSnapshot{}, fmt.Errorf("%w: tenant_id=%s: %v", errManagedTenantStorePersist, tenantIdentifier, queryError)
		}
		if identifierExists {
			continue
		}
		timestamp := store.now()
		record := newManagedTenantRecord(principal.userID, tenantIdentifier, name, timestamp)
		if createError := store.database.createTenant(record); createError != nil {
			if errors.Is(createError, gorm.ErrDuplicatedKey) {
				continue
			}
			return managedTenantSnapshot{}, fmt.Errorf("%w: user_id=%s tenant_id=%s: %v", errManagedTenantStorePersist, principal.userID, tenantIdentifier, createError)
		}
		return store.snapshot(record)
	}
	return managedTenantSnapshot{}, errManagedTenantIDCollision
}

func (store *managedTenantStore) renameTenant(principal managementPrincipal, tenantIdentifier managedTenantIdentifier, name managedTenantName) (managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.database.tenantByOwnerAndID(principal.userID, tenantIdentifier.string())
	if recordError != nil {
		return managedTenantSnapshot{}, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordError)
	}
	nameExists, nameQueryError := store.database.tenantNameExists(principal.userID, name.key, tenantIdentifier.string())
	if nameQueryError != nil {
		return managedTenantSnapshot{}, fmt.Errorf("%w: user_id=%s tenant_id=%s: %v", errManagedTenantStorePersist, principal.userID, tenantIdentifier, nameQueryError)
	}
	if nameExists {
		return managedTenantSnapshot{}, errManagedTenantNameConflict
	}
	record.Name = name.display
	record.NameKey = name.key
	record.UpdatedAt = store.now()
	if persistError := store.database.saveTenant(record); persistError != nil {
		if errors.Is(persistError, gorm.ErrDuplicatedKey) {
			return managedTenantSnapshot{}, errManagedTenantNameConflict
		}
		return managedTenantSnapshot{}, managedTenantMutationError(principal.userID, tenantIdentifier.string(), persistError)
	}
	return store.snapshot(record)
}

func (store *managedTenantStore) deleteTenant(principal managementPrincipal, tenantIdentifier managedTenantIdentifier) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if persistError := store.database.deleteTenant(principal.userID, tenantIdentifier.string()); persistError != nil {
		if errors.Is(persistError, errManagedFinalTenantDeletion) {
			return persistError
		}
		return managedTenantMutationError(principal.userID, tenantIdentifier.string(), persistError)
	}
	return nil
}

func (store *managedTenantStore) saveProviderKey(principal managementPrincipal, tenantIdentifier managedTenantIdentifier, providerIdentifier providerID, rawAPIKey string, textModel string, systemPrompt string) (managedTenantSnapshot, error) {
	apiKey := strings.TrimSpace(rawAPIKey)
	normalizedTextModel := strings.TrimSpace(textModel)
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.database.tenantByOwnerAndID(principal.userID, tenantIdentifier.string())
	if recordError != nil {
		return managedTenantSnapshot{}, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordError)
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
		encryptedAPIKey, encryptionError = store.providerKeyCipher.encrypt(store.randomReader, record.TenantID, providerIdentifier.string(), apiKey)
		if encryptionError != nil {
			return managedTenantSnapshot{}, encryptionError
		}
	}
	if createdAt.IsZero() {
		createdAt = timestamp
	}
	providerRecord := managedProviderAPIKeyRecord{
		TenantID:        record.TenantID,
		ProviderID:      providerIdentifier.string(),
		EncryptedAPIKey: encryptedAPIKey,
		TextModel:       normalizedTextModel,
		SystemPrompt:    systemPrompt,
		CreatedAt:       createdAt,
		UpdatedAt:       timestamp,
	}
	if persistError := store.database.saveProviderKey(principal.userID, providerRecord, timestamp); persistError != nil {
		return managedTenantSnapshot{}, managedTenantMutationError(principal.userID, tenantIdentifier.string(), persistError)
	}
	return store.snapshotByOwnerAndIDLocked(principal.userID, tenantIdentifier.string())
}

func (store *managedTenantStore) revealProviderKey(principal managementPrincipal, tenantIdentifier managedTenantIdentifier, providerIdentifier providerID) (string, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.database.tenantByOwnerAndID(principal.userID, tenantIdentifier.string())
	if recordError != nil {
		return constants.EmptyString, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordError)
	}
	providerKeyRecord, hasProviderKey := managedProviderKeyRecordForProvider(record.ProviderAPIKeys, providerIdentifier)
	if !hasProviderKey {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s", errManagedProviderKeyNotFound, providerIdentifier.string())
	}
	return store.providerKeyCipher.decrypt(providerKeyRecord)
}

func managedProviderKeyRecordForProvider(providerKeyRecords []managedProviderAPIKeyRecord, providerIdentifier providerID) (managedProviderAPIKeyRecord, bool) {
	for _, providerKeyRecord := range providerKeyRecords {
		if newProviderID(providerKeyRecord.ProviderID) == providerIdentifier {
			return providerKeyRecord, true
		}
	}
	return managedProviderAPIKeyRecord{}, false
}

func (store *managedTenantStore) removeProviderKey(principal managementPrincipal, tenantIdentifier managedTenantIdentifier, providerIdentifier providerID) (managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	timestamp := store.now()
	if persistError := store.database.deleteProviderKey(principal.userID, tenantIdentifier.string(), providerIdentifier.string(), timestamp); persistError != nil {
		return managedTenantSnapshot{}, managedTenantMutationError(principal.userID, tenantIdentifier.string(), persistError)
	}
	return store.snapshotByOwnerAndIDLocked(principal.userID, tenantIdentifier.string())
}

func (store *managedTenantStore) updateDefaults(principal managementPrincipal, tenantIdentifier managedTenantIdentifier, defaults managedRoutingDefaults) (managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.database.tenantByOwnerAndID(principal.userID, tenantIdentifier.string())
	if recordError != nil {
		return managedTenantSnapshot{}, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordError)
	}
	record.applyRoutingDefaults(defaults)
	record.UpdatedAt = store.now()
	if persistError := store.database.saveTenant(record); persistError != nil {
		return managedTenantSnapshot{}, managedTenantMutationError(principal.userID, tenantIdentifier.string(), persistError)
	}
	return store.snapshot(record)
}

func (store *managedTenantStore) generateSecret(principal managementPrincipal, tenantIdentifier managedTenantIdentifier, secretDigestInUse func([sha256.Size]byte) bool) (string, managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.database.tenantByOwnerAndID(principal.userID, tenantIdentifier.string())
	if recordError != nil {
		return constants.EmptyString, managedTenantSnapshot{}, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordError)
	}
	for attempt := 0; attempt < generatedTenantSecretAttempts; attempt++ {
		rawSecret, secretDigest, secretError := store.newTenantSecret()
		if secretError != nil {
			return constants.EmptyString, managedTenantSnapshot{}, secretError
		}
		if secretDigestInUse(secretDigest) || store.containsSecretDigestLocked(secretDigest) {
			continue
		}
		digestValue := hex.EncodeToString(secretDigest[:])
		record.SecretDigest = &digestValue
		record.UpdatedAt = store.now()
		if persistError := store.database.saveTenant(record); persistError != nil {
			if errors.Is(persistError, gorm.ErrDuplicatedKey) {
				continue
			}
			return constants.EmptyString, managedTenantSnapshot{}, managedTenantMutationError(principal.userID, tenantIdentifier.string(), persistError)
		}
		snapshot, snapshotError := store.snapshotByOwnerAndIDLocked(principal.userID, tenantIdentifier.string())
		if snapshotError != nil {
			return constants.EmptyString, managedTenantSnapshot{}, snapshotError
		}
		return rawSecret, snapshot, nil
	}
	return constants.EmptyString, managedTenantSnapshot{}, errManagedSecretCollision
}

func (store *managedTenantStore) revokeSecret(principal managementPrincipal, tenantIdentifier managedTenantIdentifier) (managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.database.tenantByOwnerAndID(principal.userID, tenantIdentifier.string())
	if recordError != nil {
		return managedTenantSnapshot{}, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordError)
	}
	record.SecretDigest = nil
	record.UpdatedAt = store.now()
	if persistError := store.database.saveTenant(record); persistError != nil {
		return managedTenantSnapshot{}, managedTenantMutationError(principal.userID, tenantIdentifier.string(), persistError)
	}
	return store.snapshot(record)
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

func (store *managedTenantStore) ensureUserLocked(principal managementPrincipal) (managedUserRecord, error) {
	record, recordError := store.database.userByID(principal.userID)
	if recordError == nil {
		if len(record.Tenants) == 0 {
			return managedUserRecord{}, fmt.Errorf("%w: user_id=%s no_tenants", errManagedTenantStorePersist, principal.userID)
		}
		record.UserEmail = principal.userEmail
		record.UserDisplayName = principal.userDisplayName
		record.UserAvatarURL = principal.userAvatarURL
		record.UpdatedAt = store.now()
		if persistError := store.database.saveUser(record); persistError != nil {
			return managedUserRecord{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, principal.userID, persistError)
		}
		return record, nil
	}
	if !errors.Is(recordError, gorm.ErrRecordNotFound) {
		return managedUserRecord{}, fmt.Errorf("%w: user_id=%s: %v", errManagedTenantStorePersist, principal.userID, recordError)
	}
	defaultName, _ := newManagedTenantName("Default")
	for attempt := 0; attempt < generatedTenantIdentifierAttempts; attempt++ {
		tenantIdentifier, identifierError := store.newTenantIdentifier()
		if identifierError != nil {
			return managedUserRecord{}, identifierError
		}
		identifierExists, queryError := store.database.tenantIDExists(tenantIdentifier.string())
		if queryError != nil {
			return managedUserRecord{}, fmt.Errorf("%w: tenant_id=%s: %v", errManagedTenantStorePersist, tenantIdentifier, queryError)
		}
		if identifierExists {
			continue
		}
		createdAt := store.now()
		userRecord := managedUserRecord{
			UserID:          principal.userID,
			UserEmail:       principal.userEmail,
			UserDisplayName: principal.userDisplayName,
			UserAvatarURL:   principal.userAvatarURL,
			CreatedAt:       createdAt,
			UpdatedAt:       createdAt,
		}
		tenantRecord := newManagedTenantRecord(principal.userID, tenantIdentifier, defaultName, createdAt)
		if persistError := store.database.createUserAndTenant(userRecord, tenantRecord); persistError != nil {
			if errors.Is(persistError, gorm.ErrDuplicatedKey) {
				continue
			}
			return managedUserRecord{}, fmt.Errorf("%w: user_id=%s tenant_id=%s: %v", errManagedTenantStorePersist, principal.userID, tenantIdentifier, persistError)
		}
		userRecord.Tenants = []managedTenantRecord{tenantRecord}
		return userRecord, nil
	}
	return managedUserRecord{}, errManagedTenantIDCollision
}

func (store *managedTenantStore) snapshotByOwnerAndIDLocked(ownerUserID string, tenantID string) (managedTenantSnapshot, error) {
	record, recordError := store.database.tenantByOwnerAndID(ownerUserID, tenantID)
	if recordError != nil {
		return managedTenantSnapshot{}, managedTenantQueryError(ownerUserID, tenantID, recordError)
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

func (store *managedTenantStore) newTenantIdentifier() (managedTenantIdentifier, error) {
	randomBytes := make([]byte, generatedTenantIdentifierBytes)
	if _, readError := io.ReadFull(store.randomReader, randomBytes); readError != nil {
		return "", fmt.Errorf("%w: %v", errManagedTenantIDGeneration, readError)
	}
	return managedTenantIdentifier(managedTenantIdentifierPrefix + hex.EncodeToString(randomBytes)), nil
}

func newManagedTenantRecord(ownerUserID string, identifier managedTenantIdentifier, name managedTenantName, createdAt time.Time) managedTenantRecord {
	record := managedTenantRecord{
		TenantID:    identifier.string(),
		OwnerUserID: ownerUserID,
		Name:        name.display,
		NameKey:     name.key,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	record.applyRoutingDefaults(defaultManagedRoutingDefaults())
	return record
}

func (record *managedTenantRecord) applyRoutingDefaults(defaults managedRoutingDefaults) {
	validatedDefaults := defaults.value()
	record.DefaultProvider = validatedDefaults.Provider
	record.DefaultModel = validatedDefaults.Model
	record.DefaultDictationProvider = validatedDefaults.DictationProvider
	record.DefaultDictationModel = validatedDefaults.DictationModel
	record.DefaultSystemPrompt = validatedDefaults.SystemPrompt
	record.DefaultReasoningEffort = validatedDefaults.ReasoningEffort
}

func (store *managedTenantStore) snapshot(record managedTenantRecord) (managedTenantSnapshot, error) {
	providerSettings, providerKeyError := store.providerSettingsMap(record.ProviderAPIKeys)
	if providerKeyError != nil {
		return managedTenantSnapshot{}, providerKeyError
	}
	return managedTenantSnapshot{
		ownerUserID:      record.OwnerUserID,
		tenantID:         record.TenantID,
		tenantName:       record.Name,
		hasSecret:        record.SecretDigest != nil,
		providerAPIKeys:  managedProviderAPIKeys(providerSettings),
		providerSettings: providerSettings,
		defaults:         record.defaults(),
		createdAt:        record.CreatedAt,
		updatedAt:        record.UpdatedAt,
	}, nil
}

func managedAccountSnapshotFor(record managedUserRecord) managedAccountSnapshot {
	tenantSummaries := make([]managedTenantSummary, 0, len(record.Tenants))
	for _, tenantRecord := range record.Tenants {
		tenantSummaries = append(tenantSummaries, tenantRecord.summary())
	}
	sort.Slice(tenantSummaries, func(first int, second int) bool {
		if tenantSummaries[first].createdAt.Equal(tenantSummaries[second].createdAt) {
			return tenantSummaries[first].tenantID < tenantSummaries[second].tenantID
		}
		return tenantSummaries[first].createdAt.Before(tenantSummaries[second].createdAt)
	})
	return managedAccountSnapshot{
		userID:          record.UserID,
		userEmail:       record.UserEmail,
		userDisplayName: record.UserDisplayName,
		userAvatarURL:   record.UserAvatarURL,
		tenants:         tenantSummaries,
	}
}

func (record managedTenantRecord) summary() managedTenantSummary {
	return managedTenantSummary{
		tenantID:  record.TenantID,
		name:      record.Name,
		hasSecret: record.SecretDigest != nil,
		createdAt: record.CreatedAt,
		updatedAt: record.UpdatedAt,
	}
}

func (record managedTenantRecord) defaults() TenantDefaults {
	return TenantDefaults{
		Provider:          record.DefaultProvider,
		Model:             record.DefaultModel,
		DictationProvider: record.DefaultDictationProvider,
		DictationModel:    record.DefaultDictationModel,
		SystemPrompt:      record.DefaultSystemPrompt,
		ReasoningEffort:   record.DefaultReasoningEffort,
	}
}

func (store *managedTenantStore) tenant(record managedTenantRecord, secretDigest [sha256.Size]byte) (tenant, error) {
	providerSettings, providerKeyError := store.providerSettingsMap(record.ProviderAPIKeys)
	if providerKeyError != nil {
		return tenant{}, providerKeyError
	}
	defaults := record.defaults()
	if store.routingDefaults != nil {
		validatedDefaults, defaultsError := validatePersistedManagedRoutingDefaults(store.routingDefaults, defaults)
		if defaultsError != nil {
			return tenant{}, managedRoutingDefaultsTenantError(record.TenantID, defaultsError)
		}
		defaults = validatedDefaults.value()
	}
	return tenant{
		identifier:       tenantID(record.TenantID),
		userID:           record.OwnerUserID,
		secretDigest:     secretDigest,
		defaults:         newTenantDefaults(defaults),
		managed:          true,
		providerAPIKeys:  managedProviderAPIKeys(providerSettings),
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
	if record.SecretDigest == nil {
		return [sha256.Size]byte{}, false
	}
	digestBytes, decodeError := hex.DecodeString(*record.SecretDigest)
	if decodeError != nil || len(digestBytes) != sha256.Size {
		return [sha256.Size]byte{}, false
	}
	var secretDigest [sha256.Size]byte
	copy(secretDigest[:], digestBytes)
	return secretDigest, true
}

func validManagedSecretDigest(value string) bool {
	digestBytes, decodeError := hex.DecodeString(value)
	return decodeError == nil && len(digestBytes) == sha256.Size
}

func managedSecretDigestValue(value *string) string {
	if value == nil {
		return constants.EmptyString
	}
	return *value
}

func managedTenantQueryError(ownerUserID string, tenantID string, queryError error) error {
	if errors.Is(queryError, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: tenant_id=%s", errManagedTenantNotFound, tenantID)
	}
	return fmt.Errorf("%w: user_id=%s tenant_id=%s: %v", errManagedTenantStorePersist, ownerUserID, tenantID, queryError)
}

func managedTenantMutationError(ownerUserID string, tenantID string, mutationError error) error {
	if errors.Is(mutationError, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: tenant_id=%s", errManagedTenantNotFound, tenantID)
	}
	return fmt.Errorf("%w: user_id=%s tenant_id=%s: %v", errManagedTenantStorePersist, ownerUserID, tenantID, mutationError)
}

func managedRoutingDefaultsTenantError(tenantIdentifier string, defaultsError error) error {
	return fmt.Errorf("tenant=%s: %w", tenantIdentifier, defaultsError)
}

func maskedAPIKey(rawAPIKey string) string {
	apiKey := strings.TrimSpace(rawAPIKey)
	if len(apiKey) <= maskedSecretPrefixLength+maskedSecretSuffixLength {
		return "saved"
	}
	return apiKey[:maskedSecretPrefixLength] + "..." + apiKey[len(apiKey)-maskedSecretSuffixLength:]
}
