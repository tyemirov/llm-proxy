package proxy

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	generatedTenantSecretBytes              = 32
	generatedTenantSecretAttempts           = 16
	generatedTenantSecretPrefix             = "llmp_"
	generatedTenantIdentifierBytes          = 16
	generatedTenantIdentifierAttempts       = 16
	managedTenantIdentifierPrefix           = "managed-"
	managedTenantNameMaximumCharacters      = 80
	managedTenantIDMaximumCharacters        = 128
	managedProviderKeyCiphertextPrefix      = "llmpk1:"
	maskedSecretPrefixLength                = 3
	maskedSecretSuffixLength                = 4
	managedUsageSummaryDays                 = 30
	managedUsageReadBatchSize               = 256
	managedTenantOwnershipSchemaVersion     = 1
	managedUsageOutcomeSchemaVersion        = 2
	managedKeyedRoutingSchemaVersion        = 3
	managedQwenCloudRetirementVersion       = 4
	managedModelIdentitySchemaVersion       = 5
	managedXAIProviderSchemaVersion         = 6
	managedDashScopeSettingsSchemaVersion   = 7
	managedZAIProviderSchemaVersion         = 8
	managedProviderConnectionsSchemaVersion = 9
	managedTenantSchemaVersion              = managedProviderConnectionsSchemaVersion
	managedSQLiteRuntimeQuery               = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	retiredQwenCloudProviderIdentifier      = "qwencloud"
	retiredQwenCloudModelIdentifier         = "qwen3.8-max-preview"
	retiredGrokProviderIdentifier           = "grok"
	retiredZhipuProviderIdentifier          = "zhipu"
	managedMiniMaxNativeModel               = "MiniMax-M2.7"
	managedSiliconFlowDeepSeekNativeModel   = "deepseek-ai/DeepSeek-R1"
	managedSenseVoiceNativeModel            = "FunAudioLLM/SenseVoiceSmall"

	managedUserTable                = "managed_user_records"
	managedTenantTable              = "managed_tenant_records"
	managedProviderKeyTable         = "managed_provider_api_key_records"
	managedProviderConnectionTable  = "managed_provider_connection_records"
	managedProviderProfileTable     = "managed_provider_profile_records"
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
	managedProviderBaseURLColumn    = "base_url"
	managedSchemaVersionColumn      = "version"
	managedUsageFailurePageIndex    = "idx_managed_usage_failure_page"
	dashScopeWorkspaceHostSuffix    = ".ap-southeast-1.maas.aliyuncs.com"
	dashScopeCompatibleModePath     = "/compatible-mode/v1"
)

var (
	errManagedTenantStoreOpen        = errors.New("managed_tenant_store_open_failed")
	errManagedTenantStorePersist     = errors.New("managed_tenant_store_persist_failed")
	errManagedProviderKeyInvalid     = errors.New("managed_provider_key_invalid")
	errManagedProviderBaseURLInvalid = errors.New("managed_provider_base_url_invalid")
	errManagedProviderKeyEncryption  = errors.New("managed_provider_key_encryption_failed")
	errManagedProviderKeyDecryption  = errors.New("managed_provider_key_decryption_failed")
	errManagedProviderKeyNotFound    = errors.New("managed_provider_key_not_found")
	errManagedProviderKeyConflict    = errors.New("managed_provider_key_conflict")
	errManagedSecretGeneration       = errors.New("managed_secret_generation_failed")
	errManagedSecretCollision        = errors.New("managed_secret_collision")
	errManagedTenantIDGeneration     = errors.New("managed_tenant_id_generation_failed")
	errManagedTenantIDCollision      = errors.New("managed_tenant_id_collision")
	errManagedTenantIDInvalid        = errors.New("managed_tenant_id_invalid")
	errManagedTenantNameInvalid      = errors.New("managed_tenant_name_invalid")
	errManagedTenantNameConflict     = errors.New("managed_tenant_name_conflict")
	errManagedTenantNotFound         = errors.New("managed_tenant_not_found")
	errManagedFinalTenantDeletion    = errors.New("managed_final_tenant_deletion")
	errManagedTenantSchemaMigration  = errors.New("managed_tenant_schema_migration_failed")
)

type managedTenantStore struct {
	mutex             *managedTenantStoreMutex
	database          managedTenantDatabase
	providerKeyCipher managedProviderKeyCipher
	routingDefaults   *providerRegistry
	randomReader      io.Reader
	now               func() time.Time
	usageWriter       *managedUsageWriter
}

type managedTenantStoreMutex struct {
	mutations     sync.Mutex
	databaseWrite chan struct{}
}

func newManagedTenantStoreMutex() *managedTenantStoreMutex {
	return &managedTenantStoreMutex{databaseWrite: make(chan struct{}, 1)}
}

func (mutex *managedTenantStoreMutex) Lock() {
	_ = mutex.LockContext(context.Background())
}

func (mutex *managedTenantStoreMutex) LockContext(requestContext context.Context) error {
	if lockError := mutex.DatabaseWriteLockContext(requestContext); lockError != nil {
		return lockError
	}
	mutex.mutations.Lock()
	if contextError := requestContext.Err(); contextError != nil {
		mutex.mutations.Unlock()
		mutex.DatabaseWriteUnlock()
		return contextError
	}
	return nil
}

func (mutex *managedTenantStoreMutex) Unlock() {
	mutex.mutations.Unlock()
	mutex.DatabaseWriteUnlock()
}

func (mutex *managedTenantStoreMutex) DatabaseWriteLockContext(requestContext context.Context) error {
	select {
	case mutex.databaseWrite <- struct{}{}:
		return nil
	case <-requestContext.Done():
		return requestContext.Err()
	}
}

func (mutex *managedTenantStoreMutex) DatabaseWriteUnlock() {
	<-mutex.databaseWrite
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
	tenantBySecretDigest(requestContext context.Context, secretDigest string) (managedTenantRecord, error)
	tenantIDExists(tenantID string) (bool, error)
	tenantNameExists(ownerUserID string, nameKey string, excludedTenantID string) (bool, error)
	createTenant(record managedTenantRecord) error
	saveTenant(record managedTenantRecord) error
	deleteTenant(ownerUserID string, tenantID string) error
	saveProviderConnections(requestContext context.Context, ownerUserID string, records []managedProviderConnectionRecord, profile managedProviderProfileRecord, defaults managedRoutingDefaults, updatedAt time.Time) error
	deleteProviderConnections(ownerUserID string, tenantID string, providerID string, defaults managedRoutingDefaults, updatedAt time.Time) error
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
	// ProviderAPIKeys is populated only by bounded predecessor-schema migrations.
	ProviderAPIKeys     []managedProviderAPIKeyRecord     `gorm:"-"`
	ProviderConnections []managedProviderConnectionRecord `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ProviderProfiles    []managedProviderProfileRecord    `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	UsageEvents         []managedUsageEventRecord         `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	CreatedAt           time.Time                         `gorm:"index:idx_managed_tenant_owner_created,priority:2"`
	UpdatedAt           time.Time
}

type managedProviderAPIKeyRecord struct {
	TenantID        string `gorm:"primaryKey"`
	ProviderID      string `gorm:"primaryKey"`
	EncryptedAPIKey string
	BaseURL         string
	TextModel       string
	SystemPrompt    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type managedProviderConnectionRecord struct {
	TenantID   string `gorm:"primaryKey"`
	ProviderID string `gorm:"primaryKey"`
	FieldID    string `gorm:"primaryKey"`
	Value      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type managedProviderProfileRecord struct {
	TenantID     string `gorm:"primaryKey"`
	ProviderID   string `gorm:"primaryKey"`
	TextModel    string
	SystemPrompt string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func managedProviderBaseURL(providerIdentifier providerID, rawBaseURL string) (string, error) {
	baseURL := strings.TrimSpace(rawBaseURL)
	if providerIdentifier.string() != ProviderNameDashScope {
		if baseURL != constants.EmptyString {
			return constants.EmptyString, fmt.Errorf("%w: provider=%s", errManagedProviderBaseURLInvalid, providerIdentifier.string())
		}
		return constants.EmptyString, nil
	}
	parsedURL, parseError := url.Parse(baseURL)
	if parseError != nil || parsedURL.Scheme != "https" || parsedURL.User != nil || parsedURL.Port() != constants.EmptyString || parsedURL.RawQuery != constants.EmptyString || parsedURL.Fragment != constants.EmptyString {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s", errManagedProviderBaseURLInvalid, providerIdentifier.string())
	}
	hostname := parsedURL.Hostname()
	workspaceID := strings.TrimSuffix(hostname, dashScopeWorkspaceHostSuffix)
	canonicalURL := "https://" + workspaceID + dashScopeWorkspaceHostSuffix + dashScopeCompatibleModePath
	if !strings.HasSuffix(hostname, dashScopeWorkspaceHostSuffix) || !validDashScopeWorkspaceID(workspaceID) || parsedURL.EscapedPath() != dashScopeCompatibleModePath || baseURL != canonicalURL {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s", errManagedProviderBaseURLInvalid, providerIdentifier.string())
	}
	return canonicalURL, nil
}

func validDashScopeWorkspaceID(workspaceID string) bool {
	if workspaceID == constants.EmptyString || workspaceID[0] == '-' || workspaceID[len(workspaceID)-1] == '-' {
		return false
	}
	for _, character := range workspaceID {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
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
	database, databaseError := newGORMManagedTenantDatabase(configuration, providerKeyCipher, providers)
	if databaseError != nil {
		return nil, databaseError
	}
	store := newManagedTenantStoreWithDatabaseAndCipherAndUsageQueue(database, providerKeyCipher, configuration.UsageQueueSize)
	store.routingDefaults = providers
	return store, nil
}

func newManagedTenantStoreWithDatabase(database managedTenantDatabase) *managedTenantStore {
	return newManagedTenantStoreWithDatabaseAndCipher(database, internalManagedProviderKeyCipher())
}

func newManagedTenantStoreWithDatabaseAndCipher(database managedTenantDatabase, providerKeyCipher managedProviderKeyCipher) *managedTenantStore {
	return newManagedTenantStoreWithDatabaseAndCipherAndUsageQueue(database, providerKeyCipher, DefaultManagementUsageQueueSize)
}

func newManagedTenantStoreWithDatabaseAndCipherAndUsageQueue(database managedTenantDatabase, providerKeyCipher managedProviderKeyCipher, usageQueueSize int) *managedTenantStore {
	store := &managedTenantStore{
		mutex:             newManagedTenantStoreMutex(),
		database:          database,
		providerKeyCipher: providerKeyCipher,
		randomReader:      rand.Reader,
		now:               func() time.Time { return time.Now().UTC() },
	}
	store.usageWriter = newManagedUsageWriter(store, usageQueueSize)
	return store
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

func (providerKeyCipher managedProviderKeyCipher) encryptConnection(randomReader io.Reader, tenantID string, providerIdentifier string, fieldIdentifier string, rawValue string) (string, error) {
	value := strings.TrimSpace(rawValue)
	if value == constants.EmptyString {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s field=%s", errManagedProviderKeyInvalid, providerIdentifier, fieldIdentifier)
	}
	nonce := make([]byte, providerKeyCipher.aeadCipher.NonceSize())
	if _, readError := io.ReadFull(randomReader, nonce); readError != nil {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s field=%s: %v", errManagedProviderKeyEncryption, providerIdentifier, fieldIdentifier, readError)
	}
	sealedValue := providerKeyCipher.aeadCipher.Seal(nil, nonce, []byte(value), managedProviderConnectionAssociatedData(tenantID, providerIdentifier, fieldIdentifier))
	return managedProviderKeyCiphertextPrefix + base64.StdEncoding.EncodeToString(append(nonce, sealedValue...)), nil
}

func (providerKeyCipher managedProviderKeyCipher) decryptConnection(record managedProviderConnectionRecord) (string, error) {
	encryptedValue := strings.TrimSpace(record.Value)
	if encryptedValue == constants.EmptyString || !strings.HasPrefix(encryptedValue, managedProviderKeyCiphertextPrefix) {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s field=%s", errManagedProviderKeyDecryption, record.ProviderID, record.FieldID)
	}
	sealedPayload, decodeError := base64.StdEncoding.DecodeString(strings.TrimPrefix(encryptedValue, managedProviderKeyCiphertextPrefix))
	if decodeError != nil || len(sealedPayload) <= providerKeyCipher.aeadCipher.NonceSize() {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s field=%s", errManagedProviderKeyDecryption, record.ProviderID, record.FieldID)
	}
	nonce := sealedPayload[:providerKeyCipher.aeadCipher.NonceSize()]
	ciphertext := sealedPayload[providerKeyCipher.aeadCipher.NonceSize():]
	value, decryptError := providerKeyCipher.aeadCipher.Open(nil, nonce, ciphertext, managedProviderConnectionAssociatedData(record.TenantID, record.ProviderID, record.FieldID))
	if decryptError != nil {
		return constants.EmptyString, fmt.Errorf("%w: provider=%s field=%s: %v", errManagedProviderKeyDecryption, record.ProviderID, record.FieldID, decryptError)
	}
	return strings.TrimSpace(string(value)), nil
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

func managedProviderConnectionAssociatedData(tenantID string, providerIdentifier string, fieldIdentifier string) []byte {
	return []byte(strings.TrimSpace(tenantID) + "\x00" + strings.TrimSpace(providerIdentifier) + "\x00" + strings.TrimSpace(fieldIdentifier))
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
	return sqlite.Open(configuration.DatabasePath + managedSQLiteRuntimeQuery)
}

func migrateCurrentManagedSchema(database *gorm.DB) error {
	return database.AutoMigrate(
		&managedUserRecord{},
		&managedTenantRecord{},
		&managedProviderAPIKeyRecord{},
		&managedProviderConnectionRecord{},
		&managedProviderProfileRecord{},
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
			if dropError := transaction.Migrator().DropTable(&managedProviderAPIKeyRecord{}); dropError != nil {
				return fmt.Errorf("%w: operation=drop_predecessor table=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, dropError)
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
	requiresQwenCloudRetirement := false
	requiresModelIdentityMigration := false
	switch migration.Version {
	case managedTenantOwnershipSchemaVersion:
		if migrationError := migrateManagedUsageOutcomeSchema(database); migrationError != nil {
			return migrationError
		}
		if migrationError := migrateManagedKeyedRoutingDefaults(database, providerKeyCipher, providers); migrationError != nil {
			return migrationError
		}
		requiresQwenCloudRetirement = true
		requiresModelIdentityMigration = true
	case managedUsageOutcomeSchemaVersion:
		if !managedTableHasColumn(migrator, managedUsageEventTable, managedUsageOutcomeCodeColumn) ||
			!migrator.HasIndex(&managedUsageEventRecord{}, managedUsageFailurePageIndex) {
			return fmt.Errorf("%w: operation=validate_current_schema table=%s", errManagedTenantSchemaMigration, managedUsageEventTable)
		}
		if migrationError := migrateManagedKeyedRoutingDefaults(database, providerKeyCipher, providers); migrationError != nil {
			return migrationError
		}
		requiresQwenCloudRetirement = true
		requiresModelIdentityMigration = true
	case managedKeyedRoutingSchemaVersion:
		if !managedTableHasColumn(migrator, managedUsageEventTable, managedUsageOutcomeCodeColumn) ||
			!migrator.HasIndex(&managedUsageEventRecord{}, managedUsageFailurePageIndex) {
			return fmt.Errorf("%w: operation=validate_current_schema table=%s", errManagedTenantSchemaMigration, managedUsageEventTable)
		}
		requiresQwenCloudRetirement = true
		requiresModelIdentityMigration = true
	case managedQwenCloudRetirementVersion:
		if !managedTableHasColumn(migrator, managedUsageEventTable, managedUsageOutcomeCodeColumn) ||
			!migrator.HasIndex(&managedUsageEventRecord{}, managedUsageFailurePageIndex) {
			return fmt.Errorf("%w: operation=validate_current_schema table=%s", errManagedTenantSchemaMigration, managedUsageEventTable)
		}
		requiresModelIdentityMigration = true
	case managedModelIdentitySchemaVersion:
		if !managedTableHasColumn(migrator, managedUsageEventTable, managedUsageOutcomeCodeColumn) ||
			!migrator.HasIndex(&managedUsageEventRecord{}, managedUsageFailurePageIndex) {
			return fmt.Errorf("%w: operation=validate_current_schema table=%s", errManagedTenantSchemaMigration, managedUsageEventTable)
		}
	case managedXAIProviderSchemaVersion:
		if !managedTableHasColumn(migrator, managedUsageEventTable, managedUsageOutcomeCodeColumn) ||
			!migrator.HasIndex(&managedUsageEventRecord{}, managedUsageFailurePageIndex) {
			return fmt.Errorf("%w: operation=validate_current_schema table=%s", errManagedTenantSchemaMigration, managedUsageEventTable)
		}
		if migrationError := migrateManagedDashScopeSettings(database, providerKeyCipher, providers); migrationError != nil {
			return migrationError
		}
		if migrationError := migrateManagedZAIProvider(database, providerKeyCipher, providers); migrationError != nil {
			return migrationError
		}
		return migrateManagedProviderConnections(database, providerKeyCipher, providers)
	case managedDashScopeSettingsSchemaVersion:
		if !managedTableHasColumn(migrator, managedUsageEventTable, managedUsageOutcomeCodeColumn) ||
			!migrator.HasIndex(&managedUsageEventRecord{}, managedUsageFailurePageIndex) ||
			!managedTableHasColumn(migrator, managedProviderKeyTable, managedProviderBaseURLColumn) {
			return fmt.Errorf("%w: operation=validate_current_schema table=%s", errManagedTenantSchemaMigration, managedProviderKeyTable)
		}
		if migrationError := migrateManagedZAIProvider(database, providerKeyCipher, providers); migrationError != nil {
			return migrationError
		}
		return migrateManagedProviderConnections(database, providerKeyCipher, providers)
	case managedZAIProviderSchemaVersion:
		if !managedTableHasColumn(migrator, managedUsageEventTable, managedUsageOutcomeCodeColumn) ||
			!migrator.HasIndex(&managedUsageEventRecord{}, managedUsageFailurePageIndex) ||
			!managedTableHasColumn(migrator, managedProviderKeyTable, managedProviderBaseURLColumn) {
			return fmt.Errorf("%w: operation=validate_current_schema table=%s", errManagedTenantSchemaMigration, managedProviderKeyTable)
		}
		if validationError := validateManagedKeyedRoutingDefaults(database, providerKeyCipher, providers); validationError != nil {
			return validationError
		}
		return migrateManagedProviderConnections(database, providerKeyCipher, providers)
	case managedProviderConnectionsSchemaVersion:
		if migrator.HasTable(managedProviderKeyTable) || !migrator.HasTable(managedProviderConnectionTable) || !migrator.HasTable(managedProviderProfileTable) {
			return fmt.Errorf("%w: operation=validate_current_schema table=%s", errManagedTenantSchemaMigration, managedProviderConnectionTable)
		}
		return validateManagedConnectionRoutingDefaults(database, providerKeyCipher, providers)
	default:
		return fmt.Errorf("%w: operation=validate_version version=%d expected=%d", errManagedTenantSchemaMigration, migration.Version, managedTenantSchemaVersion)
	}
	if requiresQwenCloudRetirement {
		if migrationError := migrateManagedQwenCloudRetirement(database, providerKeyCipher, providers); migrationError != nil {
			return migrationError
		}
	}
	if requiresModelIdentityMigration {
		if migrationError := migrateManagedModelIdentity(database, providerKeyCipher, providers); migrationError != nil {
			return migrationError
		}
	}
	if migrationError := migrateManagedXAIProvider(database, providerKeyCipher, providers); migrationError != nil {
		return migrationError
	}
	if migrationError := migrateManagedDashScopeSettings(database, providerKeyCipher, providers); migrationError != nil {
		return migrationError
	}
	if migrationError := migrateManagedZAIProvider(database, providerKeyCipher, providers); migrationError != nil {
		return migrationError
	}
	return migrateManagedProviderConnections(database, providerKeyCipher, providers)
}

func migrateManagedProviderConnections(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	return database.Transaction(func(transaction *gorm.DB) error {
		if migrationError := transaction.AutoMigrate(&managedProviderConnectionRecord{}, &managedProviderProfileRecord{}); migrationError != nil {
			return fmt.Errorf("%w: operation=create table=%s: %v", errManagedTenantSchemaMigration, managedProviderConnectionTable, migrationError)
		}
		if migrationError := migrateManagedProviderConnectionData(transaction, providerKeyCipher, providers); migrationError != nil {
			return migrationError
		}
		return transaction.Create(&managedSchemaMigrationRecord{Version: managedProviderConnectionsSchemaVersion, AppliedAt: time.Now().UTC()}).Error
	})
}

func migrateManagedProviderConnectionData(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	return migrateManagedProviderConnectionDataWithReader(database, providerKeyCipher, providers, rand.Reader)
}

func migrateManagedProviderConnectionDataWithReader(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry, randomReader io.Reader) error {
	var predecessorRecords []managedProviderAPIKeyRecord
	if queryError := database.Order("tenant_id, provider_id").Find(&predecessorRecords).Error; queryError != nil {
		return fmt.Errorf("%w: operation=read table=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, queryError)
	}
	connections := make([]managedProviderConnectionRecord, 0, len(predecessorRecords)*2)
	profiles := make([]managedProviderProfileRecord, 0, len(predecessorRecords))
	expectedValues := map[string]string{}
	expectedProfiles := map[string]managedProviderProfileRecord{}
	for _, predecessor := range predecessorRecords {
		providerIdentifier := newProviderID(predecessor.ProviderID)
		definition, found := providers.definitions[providerIdentifier]
		if !found || predecessor.ProviderID != providerIdentifier.string() {
			return fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s reason=unknown", errManagedTenantSchemaMigration, managedProviderKeyTable, predecessor.TenantID, predecessor.ProviderID)
		}
		credentialFields := make([]ProviderCatalogField, 0, 1)
		for _, field := range definition.fields {
			if field.Kind == CatalogProviderFieldKindCredential {
				credentialFields = append(credentialFields, field)
			}
		}
		if len(credentialFields) != 1 {
			return fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s credential_count=%d", errManagedTenantSchemaMigration, managedProviderKeyTable, predecessor.TenantID, predecessor.ProviderID, len(credentialFields))
		}
		apiKey, decryptError := providerKeyCipher.decrypt(predecessor)
		if decryptError != nil {
			return fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, predecessor.TenantID, predecessor.ProviderID, decryptError)
		}
		if _, valueError := validatedProviderFieldValue(credentialFields[0], apiKey); valueError != nil {
			return fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s field=%s", errManagedTenantSchemaMigration, managedProviderKeyTable, predecessor.TenantID, predecessor.ProviderID, credentialFields[0].ID)
		}
		encryptedValue, encryptionError := providerKeyCipher.encryptConnection(randomReader, predecessor.TenantID, predecessor.ProviderID, credentialFields[0].ID, apiKey)
		if encryptionError != nil {
			return encryptionError
		}
		credentialRecord := managedProviderConnectionRecord{
			TenantID: predecessor.TenantID, ProviderID: predecessor.ProviderID, FieldID: credentialFields[0].ID,
			Value: encryptedValue, CreatedAt: predecessor.CreatedAt, UpdatedAt: predecessor.UpdatedAt,
		}
		connections = append(connections, credentialRecord)
		expectedValues[managedProviderConnectionIdentity(credentialRecord)] = apiKey
		for _, field := range definition.fields {
			if field.Kind != CatalogProviderFieldKindSetting {
				continue
			}
			if field.ID != "base_url" {
				return fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s field=%s reason=unmapped_predecessor_column", errManagedTenantSchemaMigration, managedProviderKeyTable, predecessor.TenantID, predecessor.ProviderID, field.ID)
			}
			value, valueError := validatedProviderFieldValue(field, predecessor.BaseURL)
			if valueError != nil {
				return fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s field=%s", errManagedTenantSchemaMigration, managedProviderKeyTable, predecessor.TenantID, predecessor.ProviderID, field.ID)
			}
			settingRecord := managedProviderConnectionRecord{
				TenantID: predecessor.TenantID, ProviderID: predecessor.ProviderID, FieldID: field.ID,
				Value: value, CreatedAt: predecessor.CreatedAt, UpdatedAt: predecessor.UpdatedAt,
			}
			connections = append(connections, settingRecord)
			expectedValues[managedProviderConnectionIdentity(settingRecord)] = value
		}
		if _, _, modelError := providers.resolveTextModel(predecessor.ProviderID, predecessor.TextModel, predecessor.ProviderID, predecessor.TextModel, false); modelError != nil {
			return fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s model=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, predecessor.TenantID, predecessor.ProviderID, predecessor.TextModel, modelError)
		}
		profile := managedProviderProfileRecord{
			TenantID: predecessor.TenantID, ProviderID: predecessor.ProviderID,
			TextModel: strings.TrimSpace(predecessor.TextModel), SystemPrompt: predecessor.SystemPrompt,
			CreatedAt: predecessor.CreatedAt, UpdatedAt: predecessor.UpdatedAt,
		}
		profiles = append(profiles, profile)
		expectedProfiles[managedProviderProfileIdentity(profile)] = profile
	}
	if len(connections) != 0 {
		if createError := database.Create(&connections).Error; createError != nil {
			return fmt.Errorf("%w: operation=create table=%s: %v", errManagedTenantSchemaMigration, managedProviderConnectionTable, createError)
		}
	}
	if len(profiles) != 0 {
		if createError := database.Create(&profiles).Error; createError != nil {
			return fmt.Errorf("%w: operation=create table=%s: %v", errManagedTenantSchemaMigration, managedProviderProfileTable, createError)
		}
	}
	var actualConnections []managedProviderConnectionRecord
	if queryError := database.Order("tenant_id, provider_id, field_id").Find(&actualConnections).Error; queryError != nil || len(actualConnections) != len(connections) {
		return fmt.Errorf("%w: operation=verify table=%s count=%d expected=%d: %v", errManagedTenantSchemaMigration, managedProviderConnectionTable, len(actualConnections), len(connections), queryError)
	}
	for _, record := range actualConnections {
		definition := providers.definitions[providerID(record.ProviderID)].fields[record.FieldID]
		value := record.Value
		if definition.Secret {
			var decryptError error
			value, decryptError = providerKeyCipher.decryptConnection(record)
			if decryptError != nil {
				return decryptError
			}
		}
		if value != expectedValues[managedProviderConnectionIdentity(record)] {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s provider=%s field=%s", errManagedTenantSchemaMigration, managedProviderConnectionTable, record.TenantID, record.ProviderID, record.FieldID)
		}
	}
	var actualProfiles []managedProviderProfileRecord
	if queryError := database.Order("tenant_id, provider_id").Find(&actualProfiles).Error; queryError != nil || len(actualProfiles) != len(profiles) {
		return fmt.Errorf("%w: operation=verify table=%s count=%d expected=%d: %v", errManagedTenantSchemaMigration, managedProviderProfileTable, len(actualProfiles), len(profiles), queryError)
	}
	for _, profile := range actualProfiles {
		expected, found := expectedProfiles[managedProviderProfileIdentity(profile)]
		if !found || profile.TextModel != expected.TextModel || profile.SystemPrompt != expected.SystemPrompt || !profile.CreatedAt.Equal(expected.CreatedAt) || !profile.UpdatedAt.Equal(expected.UpdatedAt) {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s provider=%s", errManagedTenantSchemaMigration, managedProviderProfileTable, profile.TenantID, profile.ProviderID)
		}
	}
	if dropError := database.Migrator().DropTable(&managedProviderAPIKeyRecord{}); dropError != nil {
		return fmt.Errorf("%w: operation=drop_predecessor table=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, dropError)
	}
	return nil
}

func managedProviderConnectionIdentity(record managedProviderConnectionRecord) string {
	return record.TenantID + "\x00" + record.ProviderID + "\x00" + record.FieldID
}

func managedProviderProfileIdentity(record managedProviderProfileRecord) string {
	return record.TenantID + "\x00" + record.ProviderID
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
		if createError := transaction.Create(&managedSchemaMigrationRecord{Version: managedUsageOutcomeSchemaVersion, AppliedAt: time.Now().UTC()}).Error; createError != nil {
			return fmt.Errorf("%w: operation=record_version table=%s: %v", errManagedTenantSchemaMigration, managedSchemaMigrationTable, createError)
		}
		return nil
	})
}

type managedRoutingDefaultsBackfill struct {
	tenantID  string
	defaults  managedRoutingDefaults
	updatedAt time.Time
}

func migrateManagedKeyedRoutingDefaults(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	records, queryError := managedTenantRecordsForRoutingValidation(database)
	if queryError != nil {
		return queryError
	}
	backfills := make([]managedRoutingDefaultsBackfill, 0, len(records))
	for _, record := range records {
		providerSettings, providerSettingsError := managedProviderSettingsFromPredecessorRecords(providerKeyCipher, record.ProviderAPIKeys)
		if providerSettingsError != nil {
			return fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, record.OwnerUserID, record.TenantID, providerSettingsError)
		}
		projectedDefaults, _ := canonicalManagedPredecessorDefaults(record.defaults())
		currentDefaults, defaultsError := validateCanonicalManagedRoutingDefaults(providers, projectedDefaults)
		if defaultsError != nil {
			return fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, defaultsError)
		}
		reconciledDefaults, reconciliationError := reconcileManagedRoutingDefaults(providers, providerSettings, currentDefaults)
		if reconciliationError != nil {
			return fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, reconciliationError)
		}
		backfills = append(backfills, managedRoutingDefaultsBackfill{
			tenantID:  record.TenantID,
			defaults:  reconciledDefaults,
			updatedAt: record.UpdatedAt,
		})
	}
	return database.Transaction(func(transaction *gorm.DB) error {
		for _, backfill := range backfills {
			result := transaction.Model(&managedTenantRecord{}).
				Where(&managedTenantRecord{TenantID: backfill.tenantID}).
				Updates(managedRoutingDefaultsDatabaseUpdates(backfill.defaults, backfill.updatedAt))
			if result.Error != nil || result.RowsAffected != 1 {
				return fmt.Errorf(
					"%w: operation=backfill table=%s tenant=%s rows=%d: %v",
					errManagedTenantSchemaMigration,
					managedTenantTable,
					backfill.tenantID,
					result.RowsAffected,
					result.Error,
				)
			}
		}
		if validationError := validateManagedPendingRouteIdentities(transaction, providerKeyCipher, providers); validationError != nil {
			return validationError
		}
		if createError := transaction.Create(&managedSchemaMigrationRecord{Version: managedKeyedRoutingSchemaVersion, AppliedAt: time.Now().UTC()}).Error; createError != nil {
			return fmt.Errorf("%w: operation=record_version table=%s: %v", errManagedTenantSchemaMigration, managedSchemaMigrationTable, createError)
		}
		return nil
	})
}

type managedQwenCloudRetirementBackfill struct {
	tenantID  string
	defaults  managedRoutingDefaults
	updatedAt time.Time
}

type managedQwenCloudRetirementDataset struct {
	backfills               []managedQwenCloudRetirementBackfill
	retiredProviderKeyCount int64
	historicalUsage         []managedUsageEventRecord
}

func migrateManagedQwenCloudRetirement(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	dataset, preflightError := preflightManagedQwenCloudRetirement(database, providerKeyCipher, providers)
	if preflightError != nil {
		return preflightError
	}
	return database.Transaction(func(transaction *gorm.DB) error {
		deleteResult := transaction.
			Where(&managedProviderAPIKeyRecord{ProviderID: retiredQwenCloudProviderIdentifier}).
			Delete(&managedProviderAPIKeyRecord{})
		if deleteResult.Error != nil || deleteResult.RowsAffected != dataset.retiredProviderKeyCount {
			return fmt.Errorf(
				"%w: operation=delete_retired_provider table=%s provider=%s rows=%d expected=%d: %v",
				errManagedTenantSchemaMigration,
				managedProviderKeyTable,
				retiredQwenCloudProviderIdentifier,
				deleteResult.RowsAffected,
				dataset.retiredProviderKeyCount,
				deleteResult.Error,
			)
		}
		for _, backfill := range dataset.backfills {
			result := transaction.Model(&managedTenantRecord{}).
				Where(&managedTenantRecord{TenantID: backfill.tenantID}).
				Updates(managedRoutingDefaultsDatabaseUpdates(backfill.defaults, backfill.updatedAt))
			if result.Error != nil || result.RowsAffected != 1 {
				return fmt.Errorf(
					"%w: operation=backfill table=%s tenant=%s rows=%d: %v",
					errManagedTenantSchemaMigration,
					managedTenantTable,
					backfill.tenantID,
					result.RowsAffected,
					result.Error,
				)
			}
		}
		if verifyError := verifyManagedQwenCloudRetirement(transaction, providerKeyCipher, providers, dataset); verifyError != nil {
			return verifyError
		}
		if createError := transaction.Create(&managedSchemaMigrationRecord{Version: managedQwenCloudRetirementVersion, AppliedAt: time.Now().UTC()}).Error; createError != nil {
			return fmt.Errorf("%w: operation=record_version table=%s: %v", errManagedTenantSchemaMigration, managedSchemaMigrationTable, createError)
		}
		return nil
	})
}

func preflightManagedQwenCloudRetirement(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) (managedQwenCloudRetirementDataset, error) {
	records, queryError := managedTenantRecordsForRoutingValidation(database)
	if queryError != nil {
		return managedQwenCloudRetirementDataset{}, queryError
	}
	dataset := managedQwenCloudRetirementDataset{
		backfills: make([]managedQwenCloudRetirementBackfill, 0, len(records)),
	}
	for _, record := range records {
		remainingProviderKeys := make([]managedProviderAPIKeyRecord, 0, len(record.ProviderAPIKeys))
		for _, providerKeyRecord := range record.ProviderAPIKeys {
			if providerKeyRecord.ProviderID == retiredQwenCloudProviderIdentifier {
				dataset.retiredProviderKeyCount++
				continue
			}
			remainingProviderKeys = append(remainingProviderKeys, providerKeyRecord)
		}
		providerSettings, providerSettingsError := managedProviderSettingsFromPredecessorRecords(providerKeyCipher, remainingProviderKeys)
		if providerSettingsError != nil {
			return managedQwenCloudRetirementDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, record.OwnerUserID, record.TenantID, providerSettingsError)
		}
		currentDefaults, defaultsError := validateManagedQwenCloudRetirementDefaults(providers, record.defaults())
		if defaultsError != nil {
			return managedQwenCloudRetirementDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, defaultsError)
		}
		if strings.TrimSpace(record.DefaultProvider) != retiredQwenCloudProviderIdentifier {
			if _, validationError := validatePersistedManagedRoutingDefaults(providers, providerSettings, currentDefaults.value()); validationError != nil {
				return managedQwenCloudRetirementDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, validationError)
			}
			continue
		}
		reconciledDefaults, reconciliationError := reconcileManagedRoutingDefaults(providers, providerSettings, currentDefaults)
		if reconciliationError != nil {
			return managedQwenCloudRetirementDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, reconciliationError)
		}
		dataset.backfills = append(dataset.backfills, managedQwenCloudRetirementBackfill{
			tenantID:  record.TenantID,
			defaults:  reconciledDefaults,
			updatedAt: record.UpdatedAt,
		})
	}
	if usageQueryError := database.
		Where(&managedUsageEventRecord{ProviderID: retiredQwenCloudProviderIdentifier}).
		Order("id").
		Find(&dataset.historicalUsage).
		Error; usageQueryError != nil {
		return managedQwenCloudRetirementDataset{}, fmt.Errorf("%w: operation=preflight table=%s provider=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, retiredQwenCloudProviderIdentifier, usageQueryError)
	}
	return dataset, nil
}

func validateManagedQwenCloudRetirementDefaults(providers *providerRegistry, rawDefaults TenantDefaults) (managedRoutingDefaults, error) {
	if strings.TrimSpace(rawDefaults.Provider) != retiredQwenCloudProviderIdentifier {
		projectedDefaults, _ := canonicalManagedPredecessorDefaults(rawDefaults)
		return validateCanonicalManagedRoutingDefaults(providers, projectedDefaults)
	}
	if rawDefaults.Provider != retiredQwenCloudProviderIdentifier || strings.TrimSpace(rawDefaults.Model) != retiredQwenCloudModelIdentifier || rawDefaults.Model != retiredQwenCloudModelIdentifier {
		return managedRoutingDefaults{}, managedRoutingDefaultsCanonicalError(endpointKindText, rawDefaults.Provider, rawDefaults.Model)
	}
	currentWithoutRetiredTextRoute := rawDefaults
	currentWithoutRetiredTextRoute.Provider = constants.EmptyString
	currentWithoutRetiredTextRoute.Model = constants.EmptyString
	currentWithoutRetiredTextRoute.ReasoningEffort = constants.EmptyString
	projectedDefaults, _ := canonicalManagedPredecessorDefaults(currentWithoutRetiredTextRoute)
	validatedCurrent, validationError := validateCanonicalManagedRoutingDefaults(providers, projectedDefaults)
	if validationError != nil {
		return managedRoutingDefaults{}, validationError
	}
	validatedDefaults := validatedCurrent.value()
	validatedDefaults.Provider = retiredQwenCloudProviderIdentifier
	validatedDefaults.Model = retiredQwenCloudModelIdentifier
	validatedDefaults.ReasoningEffort = rawDefaults.ReasoningEffort
	return managedRoutingDefaults{tenantDefaults: validatedDefaults}, nil
}

func verifyManagedQwenCloudRetirement(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry, dataset managedQwenCloudRetirementDataset) error {
	var retiredProviderKeyCount int64
	if countError := database.Model(&managedProviderAPIKeyRecord{}).
		Where(&managedProviderAPIKeyRecord{ProviderID: retiredQwenCloudProviderIdentifier}).
		Count(&retiredProviderKeyCount).
		Error; countError != nil || retiredProviderKeyCount != 0 {
		return fmt.Errorf("%w: operation=verify table=%s provider=%s count=%d: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, retiredQwenCloudProviderIdentifier, retiredProviderKeyCount, countError)
	}
	for _, backfill := range dataset.backfills {
		var record managedTenantRecord
		if queryError := database.Where(&managedTenantRecord{TenantID: backfill.tenantID}).First(&record).Error; queryError != nil {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, backfill.tenantID, queryError)
		}
		if record.defaults() != backfill.defaults.value() || !record.UpdatedAt.Equal(backfill.updatedAt) {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s values", errManagedTenantSchemaMigration, managedTenantTable, backfill.tenantID)
		}
	}
	if validationError := validateManagedPendingRouteIdentities(database, providerKeyCipher, providers); validationError != nil {
		return validationError
	}
	var historicalUsage []managedUsageEventRecord
	if queryError := database.
		Where(&managedUsageEventRecord{ProviderID: retiredQwenCloudProviderIdentifier}).
		Order("id").
		Find(&historicalUsage).
		Error; queryError != nil {
		return fmt.Errorf("%w: operation=verify table=%s provider=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, retiredQwenCloudProviderIdentifier, queryError)
	}
	if len(historicalUsage) != len(dataset.historicalUsage) {
		return fmt.Errorf("%w: operation=verify table=%s provider=%s count=%d expected=%d", errManagedTenantSchemaMigration, managedUsageEventTable, retiredQwenCloudProviderIdentifier, len(historicalUsage), len(dataset.historicalUsage))
	}
	for usageIndex, expectedUsage := range dataset.historicalUsage {
		if historicalUsage[usageIndex] != expectedUsage {
			return fmt.Errorf("%w: operation=verify table=%s provider=%s id=%d", errManagedTenantSchemaMigration, managedUsageEventTable, retiredQwenCloudProviderIdentifier, expectedUsage.ID)
		}
	}
	return nil
}

type managedModelIdentityProviderKeyBackfill struct {
	tenantID string
	provider string
	model    string
}

type managedModelIdentityTenantBackfill struct {
	tenantID  string
	defaults  managedRoutingDefaults
	updatedAt time.Time
}

type managedModelIdentityDataset struct {
	providerKeys    []managedModelIdentityProviderKeyBackfill
	tenants         []managedModelIdentityTenantBackfill
	historicalUsage []managedUsageEventRecord
}

func migrateManagedModelIdentity(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	dataset, preflightError := preflightManagedModelIdentity(database, providerKeyCipher, providers)
	if preflightError != nil {
		return preflightError
	}
	return database.Transaction(func(transaction *gorm.DB) error {
		for _, backfill := range dataset.providerKeys {
			result := transaction.Model(&managedProviderAPIKeyRecord{}).
				Where(&managedProviderAPIKeyRecord{TenantID: backfill.tenantID, ProviderID: backfill.provider}).
				UpdateColumn("text_model", backfill.model)
			if result.Error != nil || result.RowsAffected != 1 {
				return fmt.Errorf("%w: operation=backfill table=%s tenant=%s provider=%s rows=%d: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, backfill.tenantID, backfill.provider, result.RowsAffected, result.Error)
			}
		}
		for _, backfill := range dataset.tenants {
			result := transaction.Model(&managedTenantRecord{}).
				Where(&managedTenantRecord{TenantID: backfill.tenantID}).
				Updates(managedRoutingDefaultsDatabaseUpdates(backfill.defaults, backfill.updatedAt))
			if result.Error != nil || result.RowsAffected != 1 {
				return fmt.Errorf("%w: operation=backfill table=%s tenant=%s rows=%d: %v", errManagedTenantSchemaMigration, managedTenantTable, backfill.tenantID, result.RowsAffected, result.Error)
			}
		}
		if validationError := validateManagedPendingRouteIdentities(transaction, providerKeyCipher, providers); validationError != nil {
			return validationError
		}
		migratedHistoricalUsage, usageError := managedModelIdentityHistoricalUsage(transaction)
		if usageError != nil {
			return usageError
		}
		if !slices.Equal(migratedHistoricalUsage, dataset.historicalUsage) {
			return fmt.Errorf("%w: operation=verify table=%s historical_usage_changed", errManagedTenantSchemaMigration, managedUsageEventTable)
		}
		if createError := transaction.Create(&managedSchemaMigrationRecord{Version: managedModelIdentitySchemaVersion, AppliedAt: time.Now().UTC()}).Error; createError != nil {
			return fmt.Errorf("%w: operation=record_version table=%s: %v", errManagedTenantSchemaMigration, managedSchemaMigrationTable, createError)
		}
		return nil
	})
}

func preflightManagedModelIdentity(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) (managedModelIdentityDataset, error) {
	records, queryError := managedTenantRecordsForRoutingValidation(database)
	if queryError != nil {
		return managedModelIdentityDataset{}, queryError
	}
	dataset := managedModelIdentityDataset{
		providerKeys: make([]managedModelIdentityProviderKeyBackfill, 0),
		tenants:      make([]managedModelIdentityTenantBackfill, 0),
	}
	for recordIndex := range records {
		record := &records[recordIndex]
		for providerKeyIndex := range record.ProviderAPIKeys {
			providerKey := &record.ProviderAPIKeys[providerKeyIndex]
			canonicalModel, changed := canonicalManagedTextModel(providerKey.ProviderID, providerKey.TextModel)
			if changed {
				providerKey.TextModel = canonicalModel
				dataset.providerKeys = append(dataset.providerKeys, managedModelIdentityProviderKeyBackfill{
					tenantID: record.TenantID, provider: providerKey.ProviderID, model: canonicalModel,
				})
			}
		}
		providerSettings, providerSettingsError := managedProviderSettingsFromPredecessorRecords(providerKeyCipher, record.ProviderAPIKeys)
		if providerSettingsError != nil {
			return managedModelIdentityDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, record.OwnerUserID, record.TenantID, providerSettingsError)
		}
		modelIdentityDefaults, defaultsChanged := canonicalManagedTenantDefaults(record.defaults())
		projectedDefaults, _ := canonicalManagedPredecessorDefaults(modelIdentityDefaults)
		_, defaultsError := validatePersistedManagedRoutingDefaults(providers, providerSettings, projectedDefaults)
		if defaultsError != nil {
			return managedModelIdentityDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, defaultsError)
		}
		if defaultsChanged {
			dataset.tenants = append(dataset.tenants, managedModelIdentityTenantBackfill{
				tenantID: record.TenantID, defaults: managedRoutingDefaults{tenantDefaults: modelIdentityDefaults}, updatedAt: record.UpdatedAt,
			})
		}
	}
	historicalUsage, usageError := managedModelIdentityHistoricalUsage(database)
	if usageError != nil {
		return managedModelIdentityDataset{}, usageError
	}
	dataset.historicalUsage = historicalUsage
	return dataset, nil
}

func canonicalManagedTextModel(provider string, model string) (string, bool) {
	switch {
	case provider == ProviderNameMiniMax && model == managedMiniMaxNativeModel:
		return ModelNameMiniMaxM27, true
	case provider == ProviderNameSiliconFlow && model == managedSiliconFlowDeepSeekNativeModel:
		return ModelNameSiliconFlowDeepSeek, true
	default:
		return model, false
	}
}

func canonicalManagedTenantDefaults(defaults TenantDefaults) (TenantDefaults, bool) {
	changed := false
	if canonicalModel, modelChanged := canonicalManagedTextModel(defaults.Provider, defaults.Model); modelChanged {
		defaults.Model = canonicalModel
		changed = true
	}
	if defaults.DictationProvider == ProviderNameSiliconFlow && defaults.DictationModel == managedSenseVoiceNativeModel {
		defaults.DictationModel = "sensevoice-small"
		changed = true
	}
	return defaults, changed
}

func canonicalManagedPredecessorDefaults(defaults TenantDefaults) (TenantDefaults, bool) {
	modelIdentityDefaults, modelIdentityChanged := canonicalManagedTenantDefaults(defaults)
	xaiDefaults, xaiProviderChanged := canonicalManagedXAIProviderDefaults(modelIdentityDefaults)
	currentDefaults, zaiProviderChanged := canonicalManagedZAIProviderDefaults(xaiDefaults)
	return currentDefaults, modelIdentityChanged || xaiProviderChanged || zaiProviderChanged
}

func managedModelIdentityHistoricalUsage(database *gorm.DB) ([]managedUsageEventRecord, error) {
	var historicalUsage []managedUsageEventRecord
	queryError := database.
		Where("(provider_id = ? AND model_id = ?) OR (provider_id = ? AND model_id IN ?)", ProviderNameMiniMax, managedMiniMaxNativeModel, ProviderNameSiliconFlow, []string{managedSiliconFlowDeepSeekNativeModel, managedSenseVoiceNativeModel}).
		Order("id").
		Find(&historicalUsage).
		Error
	if queryError != nil {
		return nil, fmt.Errorf("%w: operation=read table=%s historical_model_identity: %v", errManagedTenantSchemaMigration, managedUsageEventTable, queryError)
	}
	return historicalUsage, nil
}

type managedXAIProviderKeyBackfill struct {
	record managedProviderAPIKeyRecord
	apiKey string
}

type managedXAITenantBackfill struct {
	tenantID  string
	defaults  managedRoutingDefaults
	updatedAt time.Time
}

type managedXAIProviderDataset struct {
	providerKeys    []managedXAIProviderKeyBackfill
	tenants         []managedXAITenantBackfill
	historicalUsage []managedUsageEventRecord
}

func migrateManagedXAIProvider(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	dataset, preflightError := preflightManagedXAIProvider(database, providerKeyCipher, providers)
	if preflightError != nil {
		return preflightError
	}
	return database.Transaction(func(transaction *gorm.DB) error {
		for _, backfill := range dataset.providerKeys {
			result := transaction.Model(&managedProviderAPIKeyRecord{}).
				Where(&managedProviderAPIKeyRecord{TenantID: backfill.record.TenantID, ProviderID: retiredGrokProviderIdentifier}).
				UpdateColumns(map[string]any{
					"provider_id":       backfill.record.ProviderID,
					"encrypted_api_key": backfill.record.EncryptedAPIKey,
				})
			if result.Error != nil || result.RowsAffected != 1 {
				return fmt.Errorf("%w: operation=backfill table=%s tenant=%s provider=%s rows=%d: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, backfill.record.TenantID, retiredGrokProviderIdentifier, result.RowsAffected, result.Error)
			}
		}
		for _, backfill := range dataset.tenants {
			result := transaction.Model(&managedTenantRecord{}).
				Where(&managedTenantRecord{TenantID: backfill.tenantID}).
				Updates(managedRoutingDefaultsDatabaseUpdates(backfill.defaults, backfill.updatedAt))
			if result.Error != nil || result.RowsAffected != 1 {
				return fmt.Errorf("%w: operation=backfill table=%s tenant=%s rows=%d: %v", errManagedTenantSchemaMigration, managedTenantTable, backfill.tenantID, result.RowsAffected, result.Error)
			}
		}
		if verifyError := verifyManagedXAIProviderMigration(transaction, providerKeyCipher, providers, dataset); verifyError != nil {
			return verifyError
		}
		if createError := transaction.Create(&managedSchemaMigrationRecord{Version: managedXAIProviderSchemaVersion, AppliedAt: time.Now().UTC()}).Error; createError != nil {
			return fmt.Errorf("%w: operation=record_version table=%s: %v", errManagedTenantSchemaMigration, managedSchemaMigrationTable, createError)
		}
		return nil
	})
}

func preflightManagedXAIProvider(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) (managedXAIProviderDataset, error) {
	return preflightManagedXAIProviderWithReader(database, providerKeyCipher, providers, rand.Reader)
}

func preflightManagedXAIProviderWithReader(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry, randomReader io.Reader) (managedXAIProviderDataset, error) {
	records, queryError := managedTenantRecordsForRoutingValidation(database)
	if queryError != nil {
		return managedXAIProviderDataset{}, queryError
	}
	dataset := managedXAIProviderDataset{
		providerKeys: make([]managedXAIProviderKeyBackfill, 0),
		tenants:      make([]managedXAITenantBackfill, 0),
	}
	for recordIndex := range records {
		record := &records[recordIndex]
		hasRetiredProvider := false
		hasCanonicalProvider := false
		for _, providerKey := range record.ProviderAPIKeys {
			hasRetiredProvider = hasRetiredProvider || providerKey.ProviderID == retiredGrokProviderIdentifier
			hasCanonicalProvider = hasCanonicalProvider || providerKey.ProviderID == ProviderNameXAI
		}
		if hasRetiredProvider && hasCanonicalProvider {
			return managedXAIProviderDataset{}, fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider_conflict=%s:%s", errManagedTenantSchemaMigration, managedProviderKeyTable, record.TenantID, retiredGrokProviderIdentifier, ProviderNameXAI)
		}
		for providerKeyIndex := range record.ProviderAPIKeys {
			providerKey := &record.ProviderAPIKeys[providerKeyIndex]
			if providerKey.ProviderID != retiredGrokProviderIdentifier {
				continue
			}
			apiKey, decryptError := providerKeyCipher.decrypt(*providerKey)
			if decryptError != nil {
				return managedXAIProviderDataset{}, fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, record.TenantID, retiredGrokProviderIdentifier, decryptError)
			}
			encryptedAPIKey, encryptionError := providerKeyCipher.encrypt(randomReader, record.TenantID, ProviderNameXAI, apiKey)
			if encryptionError != nil {
				return managedXAIProviderDataset{}, fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, record.TenantID, retiredGrokProviderIdentifier, encryptionError)
			}
			providerKey.ProviderID = ProviderNameXAI
			providerKey.EncryptedAPIKey = encryptedAPIKey
			dataset.providerKeys = append(dataset.providerKeys, managedXAIProviderKeyBackfill{record: *providerKey, apiKey: apiKey})
		}
		providerSettings, providerSettingsError := managedProviderSettingsFromPredecessorRecords(providerKeyCipher, record.ProviderAPIKeys)
		if providerSettingsError != nil {
			return managedXAIProviderDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, record.OwnerUserID, record.TenantID, providerSettingsError)
		}
		defaults, defaultsChanged := canonicalManagedXAIProviderDefaults(record.defaults())
		projectedDefaults, _ := canonicalManagedZAIProviderDefaults(defaults)
		_, defaultsError := validatePersistedManagedRoutingDefaults(providers, providerSettings, projectedDefaults)
		if defaultsError != nil {
			return managedXAIProviderDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, defaultsError)
		}
		if defaultsChanged {
			dataset.tenants = append(dataset.tenants, managedXAITenantBackfill{
				tenantID: record.TenantID, defaults: managedRoutingDefaults{tenantDefaults: defaults}, updatedAt: record.UpdatedAt,
			})
		}
	}
	historicalUsage, usageError := managedXAIHistoricalUsage(database)
	if usageError != nil {
		return managedXAIProviderDataset{}, usageError
	}
	dataset.historicalUsage = historicalUsage
	return dataset, nil
}

func canonicalManagedXAIProviderDefaults(defaults TenantDefaults) (TenantDefaults, bool) {
	changed := false
	if defaults.Provider == retiredGrokProviderIdentifier {
		defaults.Provider = ProviderNameXAI
		changed = true
	}
	if defaults.DictationProvider == retiredGrokProviderIdentifier {
		defaults.DictationProvider = ProviderNameXAI
		changed = true
	}
	return defaults, changed
}

func verifyManagedXAIProviderMigration(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry, dataset managedXAIProviderDataset) error {
	var retiredProviderKeyCount int64
	if countError := database.Model(&managedProviderAPIKeyRecord{}).
		Where(&managedProviderAPIKeyRecord{ProviderID: retiredGrokProviderIdentifier}).
		Count(&retiredProviderKeyCount).
		Error; countError != nil || retiredProviderKeyCount != 0 {
		return fmt.Errorf("%w: operation=verify table=%s provider=%s count=%d: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, retiredGrokProviderIdentifier, retiredProviderKeyCount, countError)
	}
	for _, expected := range dataset.providerKeys {
		var actual managedProviderAPIKeyRecord
		if queryError := database.Where(&managedProviderAPIKeyRecord{TenantID: expected.record.TenantID, ProviderID: ProviderNameXAI}).First(&actual).Error; queryError != nil {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, expected.record.TenantID, ProviderNameXAI, queryError)
		}
		apiKey, decryptError := providerKeyCipher.decrypt(actual)
		if decryptError != nil || apiKey != expected.apiKey || actual.TextModel != expected.record.TextModel || actual.SystemPrompt != expected.record.SystemPrompt || !actual.CreatedAt.Equal(expected.record.CreatedAt) || !actual.UpdatedAt.Equal(expected.record.UpdatedAt) {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s provider=%s", errManagedTenantSchemaMigration, managedProviderKeyTable, expected.record.TenantID, ProviderNameXAI)
		}
	}
	for _, expected := range dataset.tenants {
		var actual managedTenantRecord
		if queryError := database.Where(&managedTenantRecord{TenantID: expected.tenantID}).First(&actual).Error; queryError != nil {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, expected.tenantID, queryError)
		}
		if actual.defaults() != expected.defaults.value() || !actual.UpdatedAt.Equal(expected.updatedAt) {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s values", errManagedTenantSchemaMigration, managedTenantTable, expected.tenantID)
		}
	}
	if validationError := validateManagedPendingRouteIdentities(database, providerKeyCipher, providers); validationError != nil {
		return validationError
	}
	historicalUsage, usageError := managedXAIHistoricalUsage(database)
	if usageError != nil {
		return usageError
	}
	if !slices.Equal(historicalUsage, dataset.historicalUsage) {
		return fmt.Errorf("%w: operation=verify table=%s historical_usage_changed provider=%s", errManagedTenantSchemaMigration, managedUsageEventTable, retiredGrokProviderIdentifier)
	}
	return nil
}

func managedXAIHistoricalUsage(database *gorm.DB) ([]managedUsageEventRecord, error) {
	var historicalUsage []managedUsageEventRecord
	if queryError := database.
		Where(&managedUsageEventRecord{ProviderID: retiredGrokProviderIdentifier}).
		Order("id").
		Find(&historicalUsage).
		Error; queryError != nil {
		return nil, fmt.Errorf("%w: operation=read table=%s provider=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, retiredGrokProviderIdentifier, queryError)
	}
	return historicalUsage, nil
}

type managedDashScopeSettingsMigrationDataset struct {
	backfills               []managedRoutingDefaultsBackfill
	removedProviderKeyCount int64
	historicalUsage         []managedUsageEventRecord
}

func migrateManagedDashScopeSettings(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	dataset, preflightError := preflightManagedDashScopeSettings(database, providerKeyCipher, providers)
	if preflightError != nil {
		return preflightError
	}
	return database.Transaction(func(transaction *gorm.DB) error {
		if !managedTableHasColumn(transaction.Migrator(), managedProviderKeyTable, managedProviderBaseURLColumn) {
			if addError := transaction.Migrator().AddColumn(&managedProviderAPIKeyRecord{}, "BaseURL"); addError != nil {
				return fmt.Errorf("%w: operation=add_column table=%s column=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, managedProviderBaseURLColumn, addError)
			}
		}
		deleteResult := transaction.
			Where(&managedProviderAPIKeyRecord{ProviderID: ProviderNameDashScope}).
			Delete(&managedProviderAPIKeyRecord{})
		if deleteResult.Error != nil || deleteResult.RowsAffected != dataset.removedProviderKeyCount {
			return fmt.Errorf("%w: operation=delete_incomplete_provider table=%s provider=%s rows=%d expected=%d: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, ProviderNameDashScope, deleteResult.RowsAffected, dataset.removedProviderKeyCount, deleteResult.Error)
		}
		for _, backfill := range dataset.backfills {
			result := transaction.Model(&managedTenantRecord{}).
				Where(&managedTenantRecord{TenantID: backfill.tenantID}).
				Updates(managedRoutingDefaultsDatabaseUpdates(backfill.defaults, backfill.updatedAt))
			if result.Error != nil || result.RowsAffected != 1 {
				return fmt.Errorf("%w: operation=backfill table=%s tenant=%s rows=%d: %v", errManagedTenantSchemaMigration, managedTenantTable, backfill.tenantID, result.RowsAffected, result.Error)
			}
		}
		if verifyError := verifyManagedDashScopeSettingsMigration(transaction, providerKeyCipher, providers, dataset); verifyError != nil {
			return verifyError
		}
		if createError := transaction.Create(&managedSchemaMigrationRecord{Version: managedDashScopeSettingsSchemaVersion, AppliedAt: time.Now().UTC()}).Error; createError != nil {
			return fmt.Errorf("%w: operation=record_version table=%s: %v", errManagedTenantSchemaMigration, managedSchemaMigrationTable, createError)
		}
		return nil
	})
}

func preflightManagedDashScopeSettings(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) (managedDashScopeSettingsMigrationDataset, error) {
	records, queryError := managedTenantRecordsForRoutingValidation(database)
	if queryError != nil {
		return managedDashScopeSettingsMigrationDataset{}, queryError
	}
	dataset := managedDashScopeSettingsMigrationDataset{backfills: make([]managedRoutingDefaultsBackfill, 0, len(records))}
	for _, record := range records {
		remainingProviderKeys := make([]managedProviderAPIKeyRecord, 0, len(record.ProviderAPIKeys))
		for _, providerKeyRecord := range record.ProviderAPIKeys {
			if providerKeyRecord.ProviderID == ProviderNameDashScope {
				dataset.removedProviderKeyCount++
				continue
			}
			remainingProviderKeys = append(remainingProviderKeys, providerKeyRecord)
		}
		providerSettings, providerSettingsError := managedProviderSettingsFromPredecessorRecords(providerKeyCipher, remainingProviderKeys)
		if providerSettingsError != nil {
			return managedDashScopeSettingsMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, record.OwnerUserID, record.TenantID, providerSettingsError)
		}
		projectedDefaults, _ := canonicalManagedZAIProviderDefaults(record.defaults())
		currentDefaults, defaultsError := validateCanonicalManagedRoutingDefaults(providers, projectedDefaults)
		if defaultsError != nil {
			return managedDashScopeSettingsMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, defaultsError)
		}
		if strings.TrimSpace(record.DefaultProvider) != ProviderNameDashScope {
			if _, validationError := validatePersistedManagedRoutingDefaults(providers, providerSettings, currentDefaults.value()); validationError != nil {
				return managedDashScopeSettingsMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, validationError)
			}
			continue
		}
		reconciledDefaults, reconciliationError := reconcileManagedRoutingDefaults(providers, providerSettings, currentDefaults)
		if reconciliationError != nil {
			return managedDashScopeSettingsMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, reconciliationError)
		}
		dataset.backfills = append(dataset.backfills, managedRoutingDefaultsBackfill{tenantID: record.TenantID, defaults: reconciledDefaults, updatedAt: record.UpdatedAt})
	}
	historicalUsage, usageError := managedDashScopeHistoricalUsage(database)
	if usageError != nil {
		return managedDashScopeSettingsMigrationDataset{}, usageError
	}
	dataset.historicalUsage = historicalUsage
	return dataset, nil
}

func verifyManagedDashScopeSettingsMigration(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry, dataset managedDashScopeSettingsMigrationDataset) error {
	if !managedTableHasColumn(database.Migrator(), managedProviderKeyTable, managedProviderBaseURLColumn) {
		return fmt.Errorf("%w: operation=verify table=%s column=%s", errManagedTenantSchemaMigration, managedProviderKeyTable, managedProviderBaseURLColumn)
	}
	var providerKeyCount int64
	if countError := database.Model(&managedProviderAPIKeyRecord{}).Where(&managedProviderAPIKeyRecord{ProviderID: ProviderNameDashScope}).Count(&providerKeyCount).Error; countError != nil || providerKeyCount != 0 {
		return fmt.Errorf("%w: operation=verify table=%s provider=%s count=%d: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, ProviderNameDashScope, providerKeyCount, countError)
	}
	for _, backfill := range dataset.backfills {
		var record managedTenantRecord
		if queryError := database.Where(&managedTenantRecord{TenantID: backfill.tenantID}).First(&record).Error; queryError != nil {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, backfill.tenantID, queryError)
		}
		if record.defaults() != backfill.defaults.value() || !record.UpdatedAt.Equal(backfill.updatedAt) {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s values", errManagedTenantSchemaMigration, managedTenantTable, backfill.tenantID)
		}
	}
	if validationError := validateManagedPendingRouteIdentities(database, providerKeyCipher, providers); validationError != nil {
		return validationError
	}
	historicalUsage, usageError := managedDashScopeHistoricalUsage(database)
	if usageError != nil {
		return usageError
	}
	if !slices.Equal(historicalUsage, dataset.historicalUsage) {
		return fmt.Errorf("%w: operation=verify table=%s historical_usage_changed provider=%s", errManagedTenantSchemaMigration, managedUsageEventTable, ProviderNameDashScope)
	}
	return nil
}

func managedDashScopeHistoricalUsage(database *gorm.DB) ([]managedUsageEventRecord, error) {
	var historicalUsage []managedUsageEventRecord
	if queryError := database.Where(&managedUsageEventRecord{ProviderID: ProviderNameDashScope}).Order("id").Find(&historicalUsage).Error; queryError != nil {
		return nil, fmt.Errorf("%w: operation=read table=%s provider=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, ProviderNameDashScope, queryError)
	}
	return historicalUsage, nil
}

type managedZAIProviderKeyBackfill struct {
	record managedProviderAPIKeyRecord
	apiKey string
}

type managedZAITenantBackfill struct {
	tenantID  string
	defaults  managedRoutingDefaults
	updatedAt time.Time
}

type managedZAIProviderDataset struct {
	providerKeys    []managedZAIProviderKeyBackfill
	tenants         []managedZAITenantBackfill
	historicalUsage []managedUsageEventRecord
}

func migrateManagedZAIProvider(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	dataset, preflightError := preflightManagedZAIProvider(database, providerKeyCipher, providers)
	if preflightError != nil {
		return preflightError
	}
	return database.Transaction(func(transaction *gorm.DB) error {
		for _, backfill := range dataset.providerKeys {
			result := transaction.Model(&managedProviderAPIKeyRecord{}).
				Where(&managedProviderAPIKeyRecord{TenantID: backfill.record.TenantID, ProviderID: retiredZhipuProviderIdentifier}).
				UpdateColumns(map[string]any{
					"provider_id":       backfill.record.ProviderID,
					"encrypted_api_key": backfill.record.EncryptedAPIKey,
				})
			if result.Error != nil || result.RowsAffected != 1 {
				return fmt.Errorf("%w: operation=backfill table=%s tenant=%s provider=%s rows=%d: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, backfill.record.TenantID, retiredZhipuProviderIdentifier, result.RowsAffected, result.Error)
			}
		}
		for _, backfill := range dataset.tenants {
			result := transaction.Model(&managedTenantRecord{}).
				Where(&managedTenantRecord{TenantID: backfill.tenantID}).
				Updates(managedRoutingDefaultsDatabaseUpdates(backfill.defaults, backfill.updatedAt))
			if result.Error != nil || result.RowsAffected != 1 {
				return fmt.Errorf("%w: operation=backfill table=%s tenant=%s rows=%d: %v", errManagedTenantSchemaMigration, managedTenantTable, backfill.tenantID, result.RowsAffected, result.Error)
			}
		}
		if verifyError := verifyManagedZAIProviderMigration(transaction, providerKeyCipher, providers, dataset); verifyError != nil {
			return verifyError
		}
		if createError := transaction.Create(&managedSchemaMigrationRecord{Version: managedZAIProviderSchemaVersion, AppliedAt: time.Now().UTC()}).Error; createError != nil {
			return fmt.Errorf("%w: operation=record_version table=%s: %v", errManagedTenantSchemaMigration, managedSchemaMigrationTable, createError)
		}
		return nil
	})
}

func preflightManagedZAIProvider(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) (managedZAIProviderDataset, error) {
	return preflightManagedZAIProviderWithReader(database, providerKeyCipher, providers, rand.Reader)
}

func preflightManagedZAIProviderWithReader(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry, randomReader io.Reader) (managedZAIProviderDataset, error) {
	records, queryError := managedTenantRecordsForRoutingValidation(database)
	if queryError != nil {
		return managedZAIProviderDataset{}, queryError
	}
	dataset := managedZAIProviderDataset{
		providerKeys: make([]managedZAIProviderKeyBackfill, 0),
		tenants:      make([]managedZAITenantBackfill, 0),
	}
	for recordIndex := range records {
		record := &records[recordIndex]
		hasRetiredProvider := false
		hasCanonicalProvider := false
		for _, providerKey := range record.ProviderAPIKeys {
			normalizedProvider := newProviderID(providerKey.ProviderID).string()
			if (normalizedProvider == retiredZhipuProviderIdentifier || normalizedProvider == ProviderNameZAI) && providerKey.ProviderID != normalizedProvider {
				return managedZAIProviderDataset{}, fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s reason=not_canonical", errManagedTenantSchemaMigration, managedProviderKeyTable, record.TenantID, providerKey.ProviderID)
			}
			hasRetiredProvider = hasRetiredProvider || providerKey.ProviderID == retiredZhipuProviderIdentifier
			hasCanonicalProvider = hasCanonicalProvider || providerKey.ProviderID == ProviderNameZAI
		}
		if hasRetiredProvider && hasCanonicalProvider {
			return managedZAIProviderDataset{}, fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider_conflict=%s:%s", errManagedTenantSchemaMigration, managedProviderKeyTable, record.TenantID, retiredZhipuProviderIdentifier, ProviderNameZAI)
		}
		for providerKeyIndex := range record.ProviderAPIKeys {
			providerKey := &record.ProviderAPIKeys[providerKeyIndex]
			if providerKey.ProviderID != retiredZhipuProviderIdentifier {
				continue
			}
			if strings.TrimSpace(providerKey.BaseURL) != constants.EmptyString {
				return managedZAIProviderDataset{}, fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s field=base_url reason=not_canonical", errManagedTenantSchemaMigration, managedProviderKeyTable, record.TenantID, retiredZhipuProviderIdentifier)
			}
			apiKey, decryptError := providerKeyCipher.decrypt(*providerKey)
			if decryptError != nil {
				return managedZAIProviderDataset{}, fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, record.TenantID, retiredZhipuProviderIdentifier, decryptError)
			}
			encryptedAPIKey, encryptionError := providerKeyCipher.encrypt(randomReader, record.TenantID, ProviderNameZAI, apiKey)
			if encryptionError != nil {
				return managedZAIProviderDataset{}, fmt.Errorf("%w: operation=preflight table=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, record.TenantID, retiredZhipuProviderIdentifier, encryptionError)
			}
			providerKey.ProviderID = ProviderNameZAI
			providerKey.EncryptedAPIKey = encryptedAPIKey
			dataset.providerKeys = append(dataset.providerKeys, managedZAIProviderKeyBackfill{record: *providerKey, apiKey: apiKey})
		}
		providerSettings, providerSettingsError := managedProviderSettingsFromRecords(providerKeyCipher, record.ProviderAPIKeys)
		if providerSettingsError != nil {
			return managedZAIProviderDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, record.OwnerUserID, record.TenantID, providerSettingsError)
		}
		defaults, defaultsChanged := canonicalManagedZAIProviderDefaults(record.defaults())
		validatedDefaults, defaultsError := validatePersistedManagedRoutingDefaults(providers, providerSettings, defaults)
		if defaultsError != nil {
			return managedZAIProviderDataset{}, fmt.Errorf("%w: operation=preflight table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, defaultsError)
		}
		if defaultsChanged {
			dataset.tenants = append(dataset.tenants, managedZAITenantBackfill{
				tenantID: record.TenantID, defaults: validatedDefaults, updatedAt: record.UpdatedAt,
			})
		}
	}
	historicalUsage, usageError := managedZAIHistoricalUsage(database)
	if usageError != nil {
		return managedZAIProviderDataset{}, usageError
	}
	dataset.historicalUsage = historicalUsage
	return dataset, nil
}

func canonicalManagedZAIProviderDefaults(defaults TenantDefaults) (TenantDefaults, bool) {
	changed := false
	if defaults.Provider == retiredZhipuProviderIdentifier {
		defaults.Provider = ProviderNameZAI
		changed = true
	}
	if defaults.DictationProvider == retiredZhipuProviderIdentifier {
		defaults.DictationProvider = ProviderNameZAI
		changed = true
	}
	return defaults, changed
}

func verifyManagedZAIProviderMigration(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry, dataset managedZAIProviderDataset) error {
	var retiredProviderKeyCount int64
	if countError := database.Model(&managedProviderAPIKeyRecord{}).
		Where(&managedProviderAPIKeyRecord{ProviderID: retiredZhipuProviderIdentifier}).
		Count(&retiredProviderKeyCount).
		Error; countError != nil || retiredProviderKeyCount != 0 {
		return fmt.Errorf("%w: operation=verify table=%s provider=%s count=%d: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, retiredZhipuProviderIdentifier, retiredProviderKeyCount, countError)
	}
	for _, expected := range dataset.providerKeys {
		var actual managedProviderAPIKeyRecord
		if queryError := database.Where(&managedProviderAPIKeyRecord{TenantID: expected.record.TenantID, ProviderID: ProviderNameZAI}).First(&actual).Error; queryError != nil {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, expected.record.TenantID, ProviderNameZAI, queryError)
		}
		apiKey, decryptError := providerKeyCipher.decrypt(actual)
		if decryptError != nil || apiKey != expected.apiKey || actual.BaseURL != expected.record.BaseURL || actual.TextModel != expected.record.TextModel || actual.SystemPrompt != expected.record.SystemPrompt || !actual.CreatedAt.Equal(expected.record.CreatedAt) || !actual.UpdatedAt.Equal(expected.record.UpdatedAt) {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s provider=%s", errManagedTenantSchemaMigration, managedProviderKeyTable, expected.record.TenantID, ProviderNameZAI)
		}
	}
	for _, expected := range dataset.tenants {
		var actual managedTenantRecord
		if queryError := database.Where(&managedTenantRecord{TenantID: expected.tenantID}).First(&actual).Error; queryError != nil {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, expected.tenantID, queryError)
		}
		if actual.defaults() != expected.defaults.value() || !actual.UpdatedAt.Equal(expected.updatedAt) {
			return fmt.Errorf("%w: operation=verify table=%s tenant=%s values", errManagedTenantSchemaMigration, managedTenantTable, expected.tenantID)
		}
	}
	if validationError := validateManagedKeyedRoutingDefaults(database, providerKeyCipher, providers); validationError != nil {
		return validationError
	}
	historicalUsage, usageError := managedZAIHistoricalUsage(database)
	if usageError != nil {
		return usageError
	}
	if !slices.Equal(historicalUsage, dataset.historicalUsage) {
		return fmt.Errorf("%w: operation=verify table=%s historical_usage_changed provider=%s", errManagedTenantSchemaMigration, managedUsageEventTable, retiredZhipuProviderIdentifier)
	}
	return nil
}

func managedZAIHistoricalUsage(database *gorm.DB) ([]managedUsageEventRecord, error) {
	var historicalUsage []managedUsageEventRecord
	if queryError := database.
		Where(&managedUsageEventRecord{ProviderID: retiredZhipuProviderIdentifier}).
		Order("id").
		Find(&historicalUsage).
		Error; queryError != nil {
		return nil, fmt.Errorf("%w: operation=read table=%s provider=%s: %v", errManagedTenantSchemaMigration, managedUsageEventTable, retiredZhipuProviderIdentifier, queryError)
	}
	return historicalUsage, nil
}

func validateManagedKeyedRoutingDefaults(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	return validateManagedRoutingDefaults(
		database,
		providerKeyCipher,
		providers,
		managedProviderSettingsFromRecords,
		func(defaults TenantDefaults) TenantDefaults { return defaults },
	)
}

func validateManagedPendingRouteIdentities(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	return validateManagedRoutingDefaults(
		database,
		providerKeyCipher,
		providers,
		managedProviderSettingsFromPredecessorRecords,
		func(defaults TenantDefaults) TenantDefaults {
			projectedDefaults, _ := canonicalManagedPredecessorDefaults(defaults)
			return projectedDefaults
		},
	)
}

func validateManagedConnectionRoutingDefaults(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	var records []managedTenantRecord
	if queryError := database.
		Preload("ProviderConnections").
		Preload("ProviderProfiles").
		Order("tenant_id").
		Find(&records).
		Error; queryError != nil {
		return fmt.Errorf("%w: operation=read table=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, queryError)
	}
	for _, record := range records {
		providerSettings, settingsError := managedProviderSettingsFromConnectionRecords(providerKeyCipher, providers, record.ProviderConnections, record.ProviderProfiles)
		if settingsError != nil {
			return fmt.Errorf("%w: operation=validate table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedProviderConnectionTable, record.OwnerUserID, record.TenantID, settingsError)
		}
		if _, defaultsError := validatePersistedManagedRoutingDefaults(providers, providerSettings, record.defaults()); defaultsError != nil {
			return fmt.Errorf("%w: operation=validate table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, defaultsError)
		}
	}
	return nil
}

type managedProviderSettingsProjection func(managedProviderKeyCipher, []managedProviderAPIKeyRecord) (map[providerID]managedProviderSettings, error)

type managedRoutingDefaultsProjection func(TenantDefaults) TenantDefaults

func validateManagedRoutingDefaults(database *gorm.DB, providerKeyCipher managedProviderKeyCipher, providers *providerRegistry, providerSettingsProjection managedProviderSettingsProjection, defaultsProjection managedRoutingDefaultsProjection) error {
	records, queryError := managedTenantRecordsForRoutingValidation(database)
	if queryError != nil {
		return queryError
	}
	for _, record := range records {
		providerSettings, providerSettingsError := providerSettingsProjection(providerKeyCipher, record.ProviderAPIKeys)
		if providerSettingsError != nil {
			return fmt.Errorf("%w: operation=validate table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, record.OwnerUserID, record.TenantID, providerSettingsError)
		}
		if _, defaultsError := validatePersistedManagedRoutingDefaults(providers, providerSettings, defaultsProjection(record.defaults())); defaultsError != nil {
			return fmt.Errorf("%w: operation=validate table=%s owner=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, record.OwnerUserID, record.TenantID, defaultsError)
		}
	}
	return nil
}

func managedTenantRecordsForRoutingValidation(database *gorm.DB) ([]managedTenantRecord, error) {
	var records []managedTenantRecord
	if queryError := database.Order("tenant_id").Find(&records).Error; queryError != nil {
		return nil, fmt.Errorf("%w: operation=read table=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, queryError)
	}
	for recordIndex := range records {
		if queryError := database.
			Where(&managedProviderAPIKeyRecord{TenantID: records[recordIndex].TenantID}).
			Order("provider_id").
			Find(&records[recordIndex].ProviderAPIKeys).
			Error; queryError != nil {
			return nil, fmt.Errorf("%w: operation=read table=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, records[recordIndex].TenantID, queryError)
		}
	}
	return records, nil
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
		if migrationError := migrateManagedProviderConnectionData(transaction, providerKeyCipher, providers); migrationError != nil {
			return migrationError
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
	routingDefaultsByTenant := make(map[string]managedRoutingDefaults, len(legacyTenants))
	routingProvidersByTenant := make(map[string][]managedRoutingProvider, len(legacyTenants))
	tenantByUserID := make(map[string]legacyManagedTenantRecord, len(legacyTenants))
	seenCanonicalProviderKeys := make(map[string]struct{}, len(legacyProviderKeys))
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
		canonicalDefaults, _ := canonicalManagedPredecessorDefaults(legacyTenant.defaults())
		validatedDefaults, defaultsError := validateCanonicalManagedRoutingDefaults(providers, canonicalDefaults)
		if defaultsError != nil {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s: %v", errManagedTenantSchemaMigration, managedTenantTable, legacyTenant.UserID, legacyTenant.TenantID, defaultsError)
		}
		routingDefaultsByTenant[legacyTenant.TenantID] = validatedDefaults
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
			DefaultProvider:          canonicalDefaults.Provider,
			DefaultModel:             canonicalDefaults.Model,
			DefaultDictationProvider: canonicalDefaults.DictationProvider,
			DefaultDictationModel:    canonicalDefaults.DictationModel,
			DefaultSystemPrompt:      canonicalDefaults.SystemPrompt,
			DefaultReasoningEffort:   canonicalDefaults.ReasoningEffort,
			CreatedAt:                legacyTenant.CreatedAt,
			UpdatedAt:                legacyTenant.UpdatedAt,
		})
	}
	for _, legacyProviderKey := range legacyProviderKeys {
		ownerTenant, ownerExists := tenantByUserID[legacyProviderKey.UserID]
		if !ownerExists {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s orphan_user=%s provider=%s", errManagedTenantSchemaMigration, managedProviderKeyTable, legacyProviderKey.UserID, legacyProviderKey.ProviderID)
		}
		providerIdentifier := legacyProviderKey.ProviderID
		if providerIdentifier == retiredGrokProviderIdentifier {
			providerIdentifier = ProviderNameXAI
		}
		if providerIdentifier == retiredZhipuProviderIdentifier {
			providerIdentifier = ProviderNameZAI
		}
		if providerIdentifier == ProviderNameDashScope {
			continue
		}
		canonicalProviderKey := ownerTenant.TenantID + "\x00" + providerIdentifier
		if _, duplicate := seenCanonicalProviderKeys[canonicalProviderKey]; duplicate {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s provider_conflict=%s", errManagedTenantSchemaMigration, managedProviderKeyTable, legacyProviderKey.UserID, ownerTenant.TenantID, providerIdentifier)
		}
		seenCanonicalProviderKeys[canonicalProviderKey] = struct{}{}
		textModel, _ := canonicalManagedTextModel(providerIdentifier, legacyProviderKey.TextModel)
		providerDefinition, textModelDefinition, modelError := providers.resolveTextModel(
			providerIdentifier,
			textModel,
			constants.EmptyString,
			constants.EmptyString,
			false,
		)
		if modelError != nil {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, legacyProviderKey.UserID, ownerTenant.TenantID, legacyProviderKey.ProviderID, modelError)
		}
		canonicalProviderIdentifier := providerDefinition.identifier.string()
		canonicalTextModel := textModelDefinition.string()
		if providerIdentifier != canonicalProviderIdentifier || textModel != canonicalTextModel {
			return managedTenantMigrationDataset{}, fmt.Errorf(
				"%w: operation=preflight table=%s user=%s tenant=%s provider=%s model=%s reason=not_canonical",
				errManagedTenantSchemaMigration,
				managedProviderKeyTable,
				legacyProviderKey.UserID,
				ownerTenant.TenantID,
				legacyProviderKey.ProviderID,
				legacyProviderKey.TextModel,
			)
		}
		if strings.TrimSpace(legacyProviderKey.APIKey) != constants.EmptyString {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s provider=%s plaintext_key", errManagedTenantSchemaMigration, managedProviderKeyTable, legacyProviderKey.UserID, ownerTenant.TenantID, legacyProviderKey.ProviderID)
		}
		apiKey, decryptError := providerKeyCipher.decryptValue(legacyProviderKey.EncryptedAPIKey, legacyProviderKey.UserID, legacyProviderKey.ProviderID)
		if decryptError != nil {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, legacyProviderKey.UserID, ownerTenant.TenantID, legacyProviderKey.ProviderID, decryptError)
		}
		encryptedAPIKey, encryptionError := providerKeyCipher.encrypt(rand.Reader, ownerTenant.TenantID, canonicalProviderIdentifier, apiKey)
		if encryptionError != nil {
			return managedTenantMigrationDataset{}, fmt.Errorf("%w: operation=preflight table=%s user=%s tenant=%s provider=%s: %v", errManagedTenantSchemaMigration, managedProviderKeyTable, legacyProviderKey.UserID, ownerTenant.TenantID, legacyProviderKey.ProviderID, encryptionError)
		}
		key := ownerTenant.TenantID + "\x00" + canonicalProviderIdentifier
		dataset.decryptedAPIKeys[key] = apiKey
		dataset.providerKeys = append(dataset.providerKeys, managedProviderAPIKeyRecord{
			TenantID:        ownerTenant.TenantID,
			ProviderID:      canonicalProviderIdentifier,
			EncryptedAPIKey: encryptedAPIKey,
			TextModel:       canonicalTextModel,
			SystemPrompt:    legacyProviderKey.SystemPrompt,
			CreatedAt:       legacyProviderKey.CreatedAt,
			UpdatedAt:       legacyProviderKey.UpdatedAt,
		})
		routingProvidersByTenant[ownerTenant.TenantID] = append(
			routingProvidersByTenant[ownerTenant.TenantID],
			managedRoutingProvider{
				definition: providerDefinition,
				textModel:  textModelDefinition,
			},
		)
	}
	for tenantIndex := range dataset.tenants {
		tenantRecord := &dataset.tenants[tenantIndex]
		reconciledDefaults := reconcileManagedRoutingDefaultsWithProviders(
			routingDefaultsByTenant[tenantRecord.TenantID],
			routingProvidersByTenant[tenantRecord.TenantID],
		)
		tenantRecord.applyRoutingDefaults(reconciledDefaults)
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
		Preload("Tenants.ProviderConnections").
		Preload("Tenants.ProviderProfiles").
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
		Preload("ProviderConnections").
		Preload("ProviderProfiles").
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

func (database *gormManagedTenantDatabase) tenantBySecretDigest(requestContext context.Context, secretDigest string) (managedTenantRecord, error) {
	var record managedTenantRecord
	transactionError := database.database.WithContext(requestContext).Transaction(
		func(transaction *gorm.DB) error {
			return transaction.
				Preload("ProviderConnections").
				Preload("ProviderProfiles").
				Where("secret_digest = ?", secretDigest).
				First(&record).
				Error
		},
		&sql.TxOptions{ReadOnly: true},
	)
	return record, transactionError
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
		if deleteError := transaction.Where(&managedProviderConnectionRecord{TenantID: tenantID}).Delete(&managedProviderConnectionRecord{}).Error; deleteError != nil {
			return deleteError
		}
		if deleteError := transaction.Where(&managedProviderProfileRecord{TenantID: tenantID}).Delete(&managedProviderProfileRecord{}).Error; deleteError != nil {
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

func (database *gormManagedTenantDatabase) saveProviderConnections(requestContext context.Context, ownerUserID string, records []managedProviderConnectionRecord, profile managedProviderProfileRecord, defaults managedRoutingDefaults, updatedAt time.Time) error {
	return database.database.WithContext(requestContext).Transaction(func(transaction *gorm.DB) error {
		var tenantRecord managedTenantRecord
		if queryError := transaction.Where(&managedTenantRecord{OwnerUserID: ownerUserID, TenantID: profile.TenantID}).First(&tenantRecord).Error; queryError != nil {
			return queryError
		}
		if deleteError := transaction.Where(&managedProviderConnectionRecord{TenantID: profile.TenantID, ProviderID: profile.ProviderID}).Delete(&managedProviderConnectionRecord{}).Error; deleteError != nil {
			return deleteError
		}
		if len(records) != 0 {
			if createError := transaction.Create(&records).Error; createError != nil {
				return createError
			}
		}
		if saveError := transaction.Save(&profile).Error; saveError != nil {
			return saveError
		}
		return transaction.Model(&managedTenantRecord{}).
			Where(&managedTenantRecord{OwnerUserID: ownerUserID, TenantID: profile.TenantID}).
			Updates(managedRoutingDefaultsDatabaseUpdates(defaults, updatedAt)).
			Error
	})
}

func (database *gormManagedTenantDatabase) deleteProviderConnections(ownerUserID string, tenantID string, providerID string, defaults managedRoutingDefaults, updatedAt time.Time) error {
	return database.database.Transaction(func(transaction *gorm.DB) error {
		var tenantRecord managedTenantRecord
		if queryError := transaction.Where(&managedTenantRecord{OwnerUserID: ownerUserID, TenantID: tenantID}).First(&tenantRecord).Error; queryError != nil {
			return queryError
		}
		if deleteError := transaction.Where(&managedProviderConnectionRecord{TenantID: tenantID, ProviderID: providerID}).Delete(&managedProviderConnectionRecord{}).Error; deleteError != nil {
			return deleteError
		}
		if deleteError := transaction.Where(&managedProviderProfileRecord{TenantID: tenantID, ProviderID: providerID}).Delete(&managedProviderProfileRecord{}).Error; deleteError != nil {
			return deleteError
		}
		return transaction.Model(&managedTenantRecord{}).
			Where(&managedTenantRecord{OwnerUserID: ownerUserID, TenantID: tenantID}).
			Updates(managedRoutingDefaultsDatabaseUpdates(defaults, updatedAt)).
			Error
	})
}

func managedRoutingDefaultsDatabaseUpdates(defaults managedRoutingDefaults, updatedAt time.Time) map[string]interface{} {
	values := defaults.value()
	return map[string]interface{}{
		"default_provider":           values.Provider,
		"default_model":              values.Model,
		"default_dictation_provider": values.DictationProvider,
		"default_dictation_model":    values.DictationModel,
		"default_system_prompt":      values.SystemPrompt,
		"default_reasoning_effort":   values.ReasoningEffort,
		"updated_at":                 updatedAt,
	}
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

func (store *managedTenantStore) saveProviderConnections(requestContext context.Context, principal managementPrincipal, tenantIdentifier managedTenantIdentifier, providerIdentifier providerID, rawFields map[string]string, textModel string, systemPrompt string, verifiedVersions map[string]managedProviderConnectionVersion) (managedTenantSnapshot, error) {
	normalizedTextModel := strings.TrimSpace(textModel)
	if lockError := store.mutex.LockContext(requestContext); lockError != nil {
		return managedTenantSnapshot{}, managedTenantMutationError(principal.userID, tenantIdentifier.string(), lockError)
	}
	defer store.mutex.Unlock()
	record, recordError := store.database.tenantByOwnerAndID(principal.userID, tenantIdentifier.string())
	if recordError != nil {
		return managedTenantSnapshot{}, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordError)
	}
	definition := store.routingDefaults.definitions[providerIdentifier]
	providerSettings, providerSettingsError := store.providerSettingsMap(record.ProviderConnections, record.ProviderProfiles)
	if providerSettingsError != nil {
		return managedTenantSnapshot{}, providerSettingsError
	}
	existingSettings, configured := providerSettings[providerIdentifier]
	connectionValues, valuesError := validatedManagedProviderConnectionValues(definition, rawFields, existingSettings, configured)
	if valuesError != nil {
		return managedTenantSnapshot{}, valuesError
	}
	for fieldIdentifier, verifiedVersion := range verifiedVersions {
		if !configured || existingSettings.connectionVersion(fieldIdentifier) != verifiedVersion {
			return managedTenantSnapshot{}, fmt.Errorf("%w: provider=%s field=%s", errManagedProviderKeyConflict, providerIdentifier.string(), fieldIdentifier)
		}
	}
	providerTextModelChanged := configured && existingSettings.textModel != normalizedTextModel
	timestamp := store.now()
	existingRecords := managedProviderConnectionRecordsForProvider(record.ProviderConnections, providerIdentifier)
	connectionRecords := make([]managedProviderConnectionRecord, 0, len(connectionValues))
	connectionVersions := make(map[string]managedProviderConnectionVersion, len(connectionValues))
	for fieldIdentifier, value := range connectionValues {
		field := definition.fields[fieldIdentifier]
		if value == *field.Default && !field.Secret {
			continue
		}
		storedValue := value
		createdAt := timestamp
		if existingRecord, exists := existingRecords[fieldIdentifier]; exists {
			createdAt = existingRecord.CreatedAt
			if field.Secret && rawFields[fieldIdentifier] == constants.EmptyString {
				storedValue = existingRecord.Value
			}
		}
		if field.Secret && storedValue == value {
			encryptedValue, encryptionError := store.providerKeyCipher.encryptConnection(store.randomReader, record.TenantID, providerIdentifier.string(), fieldIdentifier, value)
			if encryptionError != nil {
				return managedTenantSnapshot{}, encryptionError
			}
			storedValue = encryptedValue
		}
		connectionRecord := managedProviderConnectionRecord{
			TenantID: record.TenantID, ProviderID: providerIdentifier.string(), FieldID: fieldIdentifier,
			Value: storedValue, CreatedAt: createdAt, UpdatedAt: timestamp,
		}
		connectionRecords = append(connectionRecords, connectionRecord)
		if field.Secret {
			connectionVersions[fieldIdentifier] = managedProviderConnectionVersionForRecord(connectionRecord)
		}
	}
	sort.Slice(connectionRecords, func(first int, second int) bool {
		return connectionRecords[first].FieldID < connectionRecords[second].FieldID
	})
	profileCreatedAt := timestamp
	if existingProfile, exists := managedProviderProfileRecordForProvider(record.ProviderProfiles, providerIdentifier); exists {
		profileCreatedAt = existingProfile.CreatedAt
	}
	profile := managedProviderProfileRecord{
		TenantID: record.TenantID, ProviderID: providerIdentifier.string(), TextModel: normalizedTextModel,
		SystemPrompt: systemPrompt, CreatedAt: profileCreatedAt, UpdatedAt: timestamp,
	}
	providerSettings[providerIdentifier] = managedProviderSettings{
		connectionValues: connectionValues, connectionVersions: connectionVersions,
		configuredFields: map[string]bool{},
		textModel:        normalizedTextModel, systemPrompt: systemPrompt,
	}
	for _, connectionRecord := range connectionRecords {
		providerSettings[providerIdentifier].configuredFields[connectionRecord.FieldID] = true
	}
	currentDefaults, defaultsError := validateCanonicalManagedRoutingDefaults(store.routingDefaults, record.defaults())
	if defaultsError != nil {
		return managedTenantSnapshot{}, managedRoutingDefaultsTenantError(record.TenantID, defaultsError)
	}
	routingProviders := managedRoutingProvidersFromValidatedSettings(store.routingDefaults, providerSettings)
	reconciledDefaults := reconcileManagedRoutingDefaultsWithProviders(currentDefaults, routingProviders)
	if providerTextModelChanged {
		reconciledDefaults = reconcileManagedRoutingDefaultsAfterProviderTextModelChange(reconciledDefaults, routingProviders, providerIdentifier)
	}
	if persistError := store.database.saveProviderConnections(requestContext, principal.userID, connectionRecords, profile, reconciledDefaults, timestamp); persistError != nil {
		return managedTenantSnapshot{}, managedTenantMutationError(principal.userID, tenantIdentifier.string(), persistError)
	}
	return store.snapshotByOwnerAndIDLocked(principal.userID, tenantIdentifier.string())
}

func validatedManagedProviderConnectionValues(definition providerDefinition, rawFields map[string]string, existing managedProviderSettings, configured bool) (map[string]string, error) {
	if len(rawFields) != len(definition.fields) {
		return nil, fmt.Errorf("%w: provider=%s field_set", errManagedProviderKeyInvalid, definition.identifier.string())
	}
	values := make(map[string]string, len(definition.fields))
	for fieldIdentifier, rawValue := range rawFields {
		field, known := definition.fields[fieldIdentifier]
		if !known {
			return nil, fmt.Errorf("%w: provider=%s field=%s", errManagedProviderKeyInvalid, definition.identifier.string(), fieldIdentifier)
		}
		value := rawValue
		if field.Secret && value == constants.EmptyString && configured {
			value = existing.connectionValue(fieldIdentifier)
		}
		if value == constants.EmptyString {
			value = *field.Default
		}
		if value == constants.EmptyString {
			if field.Required {
				return nil, fmt.Errorf("%w: provider=%s field=%s", errManagedProviderKeyInvalid, definition.identifier.string(), fieldIdentifier)
			}
			values[fieldIdentifier] = value
			continue
		}
		validatedValue, valueError := validatedProviderFieldValue(field, value)
		if valueError != nil {
			return nil, fmt.Errorf("%w: provider=%s field=%s", errManagedProviderKeyInvalid, definition.identifier.string(), fieldIdentifier)
		}
		values[fieldIdentifier] = validatedValue
	}
	return values, nil
}

func (store *managedTenantStore) revealProviderConnectionField(principal managementPrincipal, tenantIdentifier managedTenantIdentifier, providerIdentifier providerID, fieldIdentifier string) (string, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.database.tenantByOwnerAndID(principal.userID, tenantIdentifier.string())
	if recordError != nil {
		return constants.EmptyString, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordError)
	}
	for _, connectionRecord := range record.ProviderConnections {
		if connectionRecord.ProviderID == providerIdentifier.string() && connectionRecord.FieldID == fieldIdentifier {
			return store.providerKeyCipher.decryptConnection(connectionRecord)
		}
	}
	return constants.EmptyString, fmt.Errorf("%w: provider=%s field=%s", errManagedProviderKeyNotFound, providerIdentifier.string(), fieldIdentifier)
}

func managedProviderConnectionRecordsForProvider(connectionRecords []managedProviderConnectionRecord, providerIdentifier providerID) map[string]managedProviderConnectionRecord {
	providerRecords := map[string]managedProviderConnectionRecord{}
	for _, record := range connectionRecords {
		if record.ProviderID == providerIdentifier.string() {
			providerRecords[record.FieldID] = record
		}
	}
	return providerRecords
}

func managedProviderProfileRecordForProvider(profileRecords []managedProviderProfileRecord, providerIdentifier providerID) (managedProviderProfileRecord, bool) {
	for _, record := range profileRecords {
		if record.ProviderID == providerIdentifier.string() {
			return record, true
		}
	}
	return managedProviderProfileRecord{}, false
}

func (store *managedTenantStore) removeProviderConnections(principal managementPrincipal, tenantIdentifier managedTenantIdentifier, providerIdentifier providerID) (managedTenantSnapshot, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, recordError := store.database.tenantByOwnerAndID(principal.userID, tenantIdentifier.string())
	if recordError != nil {
		return managedTenantSnapshot{}, managedTenantQueryError(principal.userID, tenantIdentifier.string(), recordError)
	}
	providerSettings, providerSettingsError := store.providerSettingsMap(record.ProviderConnections, record.ProviderProfiles)
	if providerSettingsError != nil {
		return managedTenantSnapshot{}, providerSettingsError
	}
	delete(providerSettings, providerIdentifier)
	currentDefaults, defaultsError := validateCanonicalManagedRoutingDefaults(store.routingDefaults, record.defaults())
	if defaultsError != nil {
		return managedTenantSnapshot{}, managedRoutingDefaultsTenantError(record.TenantID, defaultsError)
	}
	reconciledDefaults := reconcileManagedRoutingDefaultsWithProviders(
		currentDefaults,
		managedRoutingProvidersFromValidatedSettings(store.routingDefaults, providerSettings),
	)
	timestamp := store.now()
	if persistError := store.database.deleteProviderConnections(principal.userID, tenantIdentifier.string(), providerIdentifier.string(), reconciledDefaults, timestamp); persistError != nil {
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

func (store *managedTenantStore) generateSecret(principal managementPrincipal, tenantIdentifier managedTenantIdentifier) (string, managedTenantSnapshot, error) {
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
		if store.containsSecretDigestLocked(secretDigest) {
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

func (store *managedTenantStore) authenticate(requestContext context.Context, rawSecret string) (tenant, bool) {
	presentedSecret := strings.TrimSpace(rawSecret)
	if presentedSecret == constants.EmptyString {
		return tenant{}, false
	}
	presentedDigest := sha256.Sum256([]byte(presentedSecret))
	presentedDigestString := hex.EncodeToString(presentedDigest[:])
	record, recordError := store.database.tenantBySecretDigest(requestContext, presentedDigestString)
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
	_, recordError := store.database.tenantBySecretDigest(context.Background(), secretDigestString)
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
	providerSettings, providerKeyError := store.providerSettingsMap(record.ProviderConnections, record.ProviderProfiles)
	if providerKeyError != nil {
		return managedTenantSnapshot{}, providerKeyError
	}
	return managedTenantSnapshot{
		ownerUserID:      record.OwnerUserID,
		tenantID:         record.TenantID,
		tenantName:       record.Name,
		hasSecret:        record.SecretDigest != nil,
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
	providerSettings, providerKeyError := store.providerSettingsMap(record.ProviderConnections, record.ProviderProfiles)
	if providerKeyError != nil {
		return tenant{}, providerKeyError
	}
	defaults := record.defaults()
	if store.routingDefaults != nil {
		validatedDefaults, defaultsError := validatePersistedManagedRoutingDefaults(store.routingDefaults, providerSettings, defaults)
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
		providerSettings: providerSettings,
	}, nil
}

func (store *managedTenantStore) providerSettingsMap(connectionRecords []managedProviderConnectionRecord, profileRecords []managedProviderProfileRecord) (map[providerID]managedProviderSettings, error) {
	return managedProviderSettingsFromConnectionRecords(store.providerKeyCipher, store.routingDefaults, connectionRecords, profileRecords)
}

func managedProviderSettingsFromConnectionRecords(providerKeyCipher managedProviderKeyCipher, providers *providerRegistry, connectionRecords []managedProviderConnectionRecord, profileRecords []managedProviderProfileRecord) (map[providerID]managedProviderSettings, error) {
	if providers == nil {
		if len(connectionRecords) == 0 && len(profileRecords) == 0 {
			return map[providerID]managedProviderSettings{}, nil
		}
		return nil, fmt.Errorf("%w: provider_registry_missing", errManagedTenantStorePersist)
	}
	profiles := make(map[providerID]managedProviderProfileRecord, len(profileRecords))
	for _, profile := range profileRecords {
		providerIdentifier := newProviderID(profile.ProviderID)
		if providerIdentifier.string() == constants.EmptyString || profile.ProviderID != providerIdentifier.string() {
			return nil, fmt.Errorf("%w: provider=%s", errManagedProviderKeyInvalid, profile.ProviderID)
		}
		if _, found := providers.definitions[providerIdentifier]; !found {
			return nil, fmt.Errorf("%w: provider=%s", errManagedProviderKeyInvalid, profile.ProviderID)
		}
		if _, duplicate := profiles[providerIdentifier]; duplicate {
			return nil, fmt.Errorf("%w: provider=%s profile=duplicate", errManagedProviderKeyInvalid, profile.ProviderID)
		}
		profiles[providerIdentifier] = profile
	}
	settingsByProvider := make(map[providerID]managedProviderSettings, len(profiles))
	for providerIdentifier, profile := range profiles {
		definition, _, modelError := providers.resolveTextModel(providerIdentifier.string(), profile.TextModel, providerIdentifier.string(), profile.TextModel, false)
		if modelError != nil {
			return nil, fmt.Errorf("%w: provider=%s model=%s", errManagedProviderKeyInvalid, providerIdentifier.string(), profile.TextModel)
		}
		values := make(map[string]string, len(definition.fields))
		for fieldIdentifier, field := range definition.fields {
			values[fieldIdentifier] = *field.Default
		}
		settingsByProvider[providerIdentifier] = managedProviderSettings{
			connectionValues:   values,
			connectionVersions: map[string]managedProviderConnectionVersion{},
			configuredFields:   map[string]bool{},
			textModel:          strings.TrimSpace(profile.TextModel),
			systemPrompt:       profile.SystemPrompt,
		}
	}
	for _, record := range connectionRecords {
		providerIdentifier := newProviderID(record.ProviderID)
		settings, hasProfile := settingsByProvider[providerIdentifier]
		definition, knownProvider := providers.definitions[providerIdentifier]
		field, knownField := definition.fields[record.FieldID]
		if record.ProviderID != providerIdentifier.string() || !knownProvider || !knownField || !hasProfile {
			return nil, fmt.Errorf("%w: provider=%s field=%s", errManagedProviderKeyInvalid, record.ProviderID, record.FieldID)
		}
		value := record.Value
		if field.Secret {
			var decryptError error
			value, decryptError = providerKeyCipher.decryptConnection(record)
			if decryptError != nil {
				return nil, decryptError
			}
			settings.connectionVersions[record.FieldID] = managedProviderConnectionVersionForRecord(record)
		}
		validatedValue, valueError := validatedProviderFieldValue(field, value)
		if valueError != nil {
			return nil, fmt.Errorf("%w: provider=%s field=%s", errManagedProviderKeyInvalid, record.ProviderID, record.FieldID)
		}
		settings.connectionValues[record.FieldID] = validatedValue
		settings.configuredFields[record.FieldID] = true
		settingsByProvider[providerIdentifier] = settings
	}
	for providerIdentifier, settings := range settingsByProvider {
		if !settings.hasRequiredConnectionFields(providers.definitions[providerIdentifier]) {
			return nil, fmt.Errorf("%w: provider=%s required_field_missing", errManagedProviderKeyInvalid, providerIdentifier.string())
		}
	}
	return settingsByProvider, nil
}

func managedProviderSettingsFromRecords(providerKeyCipher managedProviderKeyCipher, providerKeyRecords []managedProviderAPIKeyRecord) (map[providerID]managedProviderSettings, error) {
	return managedProviderSettingsFromRecordsForSchema(providerKeyCipher, providerKeyRecords, true)
}

func managedProviderSettingsFromRecordsForSchema(providerKeyCipher managedProviderKeyCipher, providerKeyRecords []managedProviderAPIKeyRecord, requireBaseURL bool) (map[providerID]managedProviderSettings, error) {
	providerSettings := make(map[providerID]managedProviderSettings, len(providerKeyRecords))
	for _, providerKeyRecord := range providerKeyRecords {
		providerIdentifier := newProviderID(providerKeyRecord.ProviderID)
		if providerIdentifier.string() == constants.EmptyString {
			continue
		}
		apiKey, decryptError := providerKeyCipher.decrypt(providerKeyRecord)
		if decryptError != nil {
			return nil, decryptError
		}
		if apiKey != constants.EmptyString {
			baseURL := constants.EmptyString
			if requireBaseURL {
				var baseURLError error
				baseURL, baseURLError = managedProviderBaseURL(providerIdentifier, providerKeyRecord.BaseURL)
				if baseURLError != nil {
					return nil, baseURLError
				}
			}
			providerSettings[providerIdentifier] = managedProviderSettings{
				connectionValues: map[string]string{
					CatalogCredentialAPIKey: apiKey,
					"base_url":              baseURL,
				},
				connectionVersions: map[string]managedProviderConnectionVersion{
					CatalogCredentialAPIKey: managedProviderConnectionVersionForCiphertext(providerKeyRecord.EncryptedAPIKey),
				},
				configuredFields: map[string]bool{CatalogCredentialAPIKey: true},
				textModel:        strings.TrimSpace(providerKeyRecord.TextModel),
				systemPrompt:     providerKeyRecord.SystemPrompt,
			}
		}
	}
	return providerSettings, nil
}

func managedProviderConnectionVersionForRecord(record managedProviderConnectionRecord) managedProviderConnectionVersion {
	return managedProviderConnectionVersionForCiphertext(record.Value)
}

func managedProviderConnectionVersionForCiphertext(encryptedValue string) managedProviderConnectionVersion {
	return sha256.Sum256([]byte(encryptedValue))
}

func managedProviderSettingsFromPredecessorRecords(providerKeyCipher managedProviderKeyCipher, providerKeyRecords []managedProviderAPIKeyRecord) (map[providerID]managedProviderSettings, error) {
	predecessorSettings, settingsError := managedProviderSettingsFromRecordsForSchema(providerKeyCipher, providerKeyRecords, false)
	if settingsError != nil {
		return nil, settingsError
	}
	providerSettings := make(map[providerID]managedProviderSettings, len(predecessorSettings))
	for predecessorProviderIdentifier, settings := range predecessorSettings {
		providerIdentifier := predecessorProviderIdentifier.string()
		if providerIdentifier == retiredGrokProviderIdentifier {
			providerIdentifier = ProviderNameXAI
		}
		if providerIdentifier == retiredZhipuProviderIdentifier {
			providerIdentifier = ProviderNameZAI
		}
		textModel, _ := canonicalManagedTextModel(providerIdentifier, settings.textModel)
		canonicalProviderIdentifier := newProviderID(providerIdentifier)
		if _, duplicate := providerSettings[canonicalProviderIdentifier]; duplicate {
			return nil, fmt.Errorf("%w: provider_conflict=%s", errManagedRoutingDefaultsInvalid, canonicalProviderIdentifier.string())
		}
		settings.textModel = strings.TrimSpace(textModel)
		providerSettings[canonicalProviderIdentifier] = managedProviderSettings{
			connectionValues:   cloneStringMap(settings.connectionValues),
			connectionVersions: map[string]managedProviderConnectionVersion{},
			configuredFields:   map[string]bool{CatalogCredentialAPIKey: true},
			textModel:          settings.textModel,
			systemPrompt:       settings.systemPrompt,
		}
	}
	return providerSettings, nil
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
