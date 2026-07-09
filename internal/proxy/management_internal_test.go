package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

var errInternalTestDatabase = errors.New("database failed")
var errInternalTestRead = errors.New("read failed")

const testManagedProviderKeyEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func TestManagedTenantStoreInternalEdges(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	principal := managementPrincipal{userID: "tauth-internal-user"}

	inMemoryDatabase := newFakeManagedTenantDatabase()
	inMemoryStore := newManagedTenantStoreWithDatabase(inMemoryDatabase)
	inMemoryStore.now = func() time.Time { return fixedTime }
	snapshot, profileError := inMemoryStore.profile(principal)
	if profileError != nil || snapshot.userID != principal.userID {
		t.Fatalf("profile snapshot=%+v error=%v", snapshot, profileError)
	}
	keySnapshot, saveKeyError := inMemoryStore.saveProviderKey(principal, newProviderID("openai"), "sk-openai", ModelNameGPT41, "provider system")
	if saveKeyError != nil {
		t.Fatalf("save provider key error=%v", saveKeyError)
	}
	if keySnapshot.providerAPIKeys[newProviderID("openai")] != "sk-openai" {
		t.Fatalf("snapshot provider keys=%+v", keySnapshot.providerAPIKeys)
	}
	if keySnapshot.providerSettings[newProviderID("openai")].textModel != ModelNameGPT41 || keySnapshot.providerSettings[newProviderID("openai")].systemPrompt != "provider system" {
		t.Fatalf("snapshot provider settings=%+v", keySnapshot.providerSettings)
	}
	keyRecord := inMemoryDatabase.records[principal.userID].ProviderAPIKeys[0]
	if keyRecord.APIKey != "" || !strings.HasPrefix(keyRecord.EncryptedAPIKey, managedProviderKeyCiphertextPrefix) || strings.Contains(keyRecord.EncryptedAPIKey, "sk-openai") {
		t.Fatalf("provider key record=%+v", keyRecord)
	}
	updatedKeySnapshot, updateKeyError := inMemoryStore.saveProviderKey(principal, newProviderID("openai"), "", ModelNameGPT41, "updated provider system")
	if updateKeyError != nil {
		t.Fatalf("update provider settings error=%v", updateKeyError)
	}
	if updatedKeySnapshot.providerSettings[newProviderID("openai")].systemPrompt != "updated provider system" {
		t.Fatalf("updated provider settings=%+v", updatedKeySnapshot.providerSettings)
	}
	updatedKeyRecord := inMemoryDatabase.records[principal.userID].ProviderAPIKeys[0]
	if updatedKeyRecord.EncryptedAPIKey != keyRecord.EncryptedAPIKey {
		t.Fatalf("updated provider key re-encrypted or changed: before=%s after=%s", keyRecord.EncryptedAPIKey, updatedKeyRecord.EncryptedAPIKey)
	}
	for _, invalidRecord := range []managedProviderAPIKeyRecord{
		{UserID: principal.userID, ProviderID: "openai"},
		{UserID: principal.userID, ProviderID: "openai", EncryptedAPIKey: "plaintext"},
		{UserID: principal.userID, ProviderID: "openai", EncryptedAPIKey: managedProviderKeyCiphertextPrefix + "%"},
		{UserID: principal.userID, ProviderID: "openai", EncryptedAPIKey: managedProviderKeyCiphertextPrefix + "AQID"},
		{UserID: principal.userID, ProviderID: "deepseek", EncryptedAPIKey: keyRecord.EncryptedAPIKey},
	} {
		if _, decryptError := inMemoryStore.providerKeyCipher.decrypt(invalidRecord); !errors.Is(decryptError, errManagedProviderKeyDecryption) {
			t.Fatalf("decrypt error=%v want %v", decryptError, errManagedProviderKeyDecryption)
		}
	}
	emptyProviderMap, emptyProviderMapError := inMemoryStore.providerAPIKeyMap([]managedProviderAPIKeyRecord{
		{UserID: principal.userID, ProviderID: "", EncryptedAPIKey: "ignored"},
		internalBlankManagedProviderKeyRecord(t, principal.userID, "openai", fixedTime),
	})
	if emptyProviderMapError != nil || len(emptyProviderMap) != 0 {
		t.Fatalf("empty provider map=%+v error=%v", emptyProviderMap, emptyProviderMapError)
	}
	brokenProviderRecord := internalManagedTenantRecord("broken-provider-user", "", fixedTime)
	brokenProviderRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{{UserID: "broken-provider-user", ProviderID: "openai", EncryptedAPIKey: "bad"}}
	if _, brokenSnapshotError := inMemoryStore.snapshot(brokenProviderRecord); !errors.Is(brokenSnapshotError, errManagedProviderKeyDecryption) {
		t.Fatalf("broken snapshot error=%v want %v", brokenSnapshotError, errManagedProviderKeyDecryption)
	}
	brokenSecretDigest := sha256.Sum256([]byte("broken-secret"))
	brokenProviderRecord.SecretDigest = hex.EncodeToString(brokenSecretDigest[:])
	if _, brokenTenantError := inMemoryStore.tenant(brokenProviderRecord, brokenSecretDigest); !errors.Is(brokenTenantError, errManagedProviderKeyDecryption) {
		t.Fatalf("broken tenant error=%v want %v", brokenTenantError, errManagedProviderKeyDecryption)
	}
	if _, providerMapError := inMemoryStore.providerAPIKeyMap([]managedProviderAPIKeyRecord{{UserID: "broken-provider-user", ProviderID: "openai", EncryptedAPIKey: "bad"}}); !errors.Is(providerMapError, errManagedProviderKeyDecryption) {
		t.Fatalf("provider map error=%v want %v", providerMapError, errManagedProviderKeyDecryption)
	}
	if _, authenticated := inMemoryStore.authenticate(" "); authenticated {
		t.Fatalf("blank generated secret authenticated")
	}

	readErrorStore := newManagedTenantStoreWithDatabase(newFakeManagedTenantDatabase())
	readErrorStore.randomReader = strings.NewReader("")
	_, _, generationError := readErrorStore.generateSecret(principal, func([sha256.Size]byte) bool { return false })
	if !errors.Is(generationError, errManagedSecretGeneration) {
		t.Fatalf("generation error=%v want %v", generationError, errManagedSecretGeneration)
	}
	if _, encryptionError := readErrorStore.saveProviderKey(principal, newProviderID("openai"), "sk-openai", ModelNameGPT41, ""); !errors.Is(encryptionError, errManagedProviderKeyEncryption) {
		t.Fatalf("provider key encryption error=%v want %v", encryptionError, errManagedProviderKeyEncryption)
	}
	if _, invalidProviderKeyError := readErrorStore.saveProviderKey(principal, newProviderID("openai"), " ", ModelNameGPT41, ""); !errors.Is(invalidProviderKeyError, errManagedProviderKeyInvalid) {
		t.Fatalf("provider key invalid error=%v want %v", invalidProviderKeyError, errManagedProviderKeyInvalid)
	}
	if _, invalidProviderKeyError := internalManagedProviderKeyCipher().encrypt(randReaderForProviderKeyTests(), principal.userID, "openai", " "); !errors.Is(invalidProviderKeyError, errManagedProviderKeyInvalid) {
		t.Fatalf("provider key direct invalid error=%v want %v", invalidProviderKeyError, errManagedProviderKeyInvalid)
	}

	collisionDatabase := newFakeManagedTenantDatabase()
	collisionStore := newManagedTenantStoreWithDatabase(collisionDatabase)
	collisionStore.now = func() time.Time { return fixedTime }
	zeroSecretBytes := make([]byte, generatedTenantSecretBytes)
	zeroRawSecret := generatedTenantSecretPrefix + hex.EncodeToString(zeroSecretBytes)
	zeroSecretDigest := sha256.Sum256([]byte(zeroRawSecret))
	collisionDatabase.records["existing"] = internalManagedTenantRecord("existing", hex.EncodeToString(zeroSecretDigest[:]), fixedTime)
	collisionStore.randomReader = bytes.NewReader(bytes.Repeat(zeroSecretBytes, generatedTenantSecretAttempts))
	_, _, collisionError := collisionStore.generateSecret(principal, func([sha256.Size]byte) bool { return false })
	if !errors.Is(collisionError, errManagedSecretCollision) {
		t.Fatalf("collision error=%v want %v", collisionError, errManagedSecretCollision)
	}

	legacyDatabase := newFakeManagedTenantDatabase()
	legacyRecord := internalManagedTenantRecord("legacy-user", "", fixedTime)
	legacyRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{{UserID: "legacy-user", ProviderID: "openai", APIKey: "sk-legacy", CreatedAt: fixedTime, UpdatedAt: fixedTime}}
	legacyDatabase.records[legacyRecord.UserID] = legacyRecord
	legacyStore := newManagedTenantStoreWithDatabase(legacyDatabase)
	legacyStore.now = func() time.Time { return fixedTime }
	if migrationError := legacyStore.migratePlaintextProviderKeys(); migrationError != nil {
		t.Fatalf("provider key migration error=%v", migrationError)
	}
	migratedKeyRecord := legacyDatabase.records[legacyRecord.UserID].ProviderAPIKeys[0]
	if migratedKeyRecord.APIKey != "" || !strings.HasPrefix(migratedKeyRecord.EncryptedAPIKey, managedProviderKeyCiphertextPrefix) {
		t.Fatalf("migrated provider key record=%+v", migratedKeyRecord)
	}
	legacySnapshot, legacySnapshotError := legacyStore.profile(managementPrincipal{userID: legacyRecord.UserID})
	if legacySnapshotError != nil || legacySnapshot.providerAPIKeys[newProviderID("openai")] != "sk-legacy" {
		t.Fatalf("legacy snapshot=%+v error=%v", legacySnapshot, legacySnapshotError)
	}
	if migrationError := legacyStore.migrateProviderTextSettings(internalManagementProviderRegistry()); migrationError != nil {
		t.Fatalf("provider text settings migration error=%v", migrationError)
	}
	migratedSettingsRecord := legacyDatabase.records[legacyRecord.UserID].ProviderAPIKeys[0]
	if migratedSettingsRecord.TextModel != ModelNameGPT41 {
		t.Fatalf("migrated text model=%q", migratedSettingsRecord.TextModel)
	}
	migrationSkipDatabase := newFakeManagedTenantDatabase()
	migrationSkipRecord := internalManagedTenantRecord("migration-skip-user", "", fixedTime)
	migrationSkipRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{
		{UserID: migrationSkipRecord.UserID, ProviderID: "openai", EncryptedAPIKey: migratedSettingsRecord.EncryptedAPIKey, TextModel: ModelNameGPT55},
		{UserID: migrationSkipRecord.UserID, ProviderID: "unknown", EncryptedAPIKey: migratedSettingsRecord.EncryptedAPIKey},
	}
	migrationSkipDatabase.records[migrationSkipRecord.UserID] = migrationSkipRecord
	if migrationError := newManagedTenantStoreWithDatabase(migrationSkipDatabase).migrateProviderTextSettings(internalManagementProviderRegistry()); migrationError != nil {
		t.Fatalf("provider text settings skip migration error=%v", migrationError)
	}
	skippedProviderKeys := migrationSkipDatabase.records[migrationSkipRecord.UserID].ProviderAPIKeys
	if skippedProviderKeys[0].TextModel != ModelNameGPT55 || skippedProviderKeys[1].TextModel != "" {
		t.Fatalf("skipped provider key records=%+v", skippedProviderKeys)
	}

	migrationQueryDatabase := newFakeManagedTenantDatabase()
	migrationQueryDatabase.userQueryErrors = []error{errInternalTestDatabase}
	if migrationError := newManagedTenantStoreWithDatabase(migrationQueryDatabase).migratePlaintextProviderKeys(); !errors.Is(migrationError, errManagedTenantStorePersist) {
		t.Fatalf("provider key migration query error=%v want %v", migrationError, errManagedTenantStorePersist)
	}
	providerSettingsQueryDatabase := newFakeManagedTenantDatabase()
	providerSettingsQueryDatabase.userQueryErrors = []error{errInternalTestDatabase}
	if migrationError := newManagedTenantStoreWithDatabase(providerSettingsQueryDatabase).migrateProviderTextSettings(internalManagementProviderRegistry()); !errors.Is(migrationError, errManagedTenantStorePersist) {
		t.Fatalf("provider settings migration query error=%v want %v", migrationError, errManagedTenantStorePersist)
	}

	migrationSaveDatabase := newFakeManagedTenantDatabase()
	migrationSaveRecord := internalManagedTenantRecord("migration-save-user", "", fixedTime)
	migrationSaveRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{{UserID: migrationSaveRecord.UserID, ProviderID: "openai", APIKey: "sk-save"}}
	migrationSaveDatabase.records[migrationSaveRecord.UserID] = migrationSaveRecord
	migrationSaveDatabase.saveProviderKeyError = errInternalTestDatabase
	if migrationError := newManagedTenantStoreWithDatabase(migrationSaveDatabase).migratePlaintextProviderKeys(); !errors.Is(migrationError, errManagedTenantStorePersist) {
		t.Fatalf("provider key migration save error=%v want %v", migrationError, errManagedTenantStorePersist)
	}
	if migrationError := newManagedTenantStoreWithDatabase(migrationSaveDatabase).migrateProviderTextSettings(internalManagementProviderRegistry()); !errors.Is(migrationError, errManagedTenantStorePersist) {
		t.Fatalf("provider settings migration save error=%v want %v", migrationError, errManagedTenantStorePersist)
	}

	migrationEncryptionDatabase := newFakeManagedTenantDatabase()
	migrationEncryptionRecord := internalManagedTenantRecord("migration-encryption-user", "", fixedTime)
	migrationEncryptionRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{{UserID: migrationEncryptionRecord.UserID, ProviderID: "openai", APIKey: "sk-encryption"}}
	migrationEncryptionDatabase.records[migrationEncryptionRecord.UserID] = migrationEncryptionRecord
	migrationEncryptionStore := newManagedTenantStoreWithDatabase(migrationEncryptionDatabase)
	migrationEncryptionStore.randomReader = strings.NewReader("")
	if migrationError := migrationEncryptionStore.migratePlaintextProviderKeys(); !errors.Is(migrationError, errManagedProviderKeyEncryption) {
		t.Fatalf("provider key migration encryption error=%v want %v", migrationError, errManagedProviderKeyEncryption)
	}

	authProviderKeyDatabase := newFakeManagedTenantDatabase()
	authProviderKeyRecord := internalManagedTenantRecord("auth-provider-key-user", hex.EncodeToString(brokenSecretDigest[:]), fixedTime)
	authProviderKeyRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{{UserID: authProviderKeyRecord.UserID, ProviderID: "openai", EncryptedAPIKey: "bad"}}
	authProviderKeyDatabase.secretQueryRecord = &authProviderKeyRecord
	if _, authenticated := newManagedTenantStoreWithDatabase(authProviderKeyDatabase).authenticate("broken-secret"); authenticated {
		t.Fatalf("broken provider key authenticated")
	}
}

func TestManagedTenantStoreDatabaseErrorEdges(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	principal := managementPrincipal{userID: "tauth-database-error-user"}

	queryErrorDatabase := newFakeManagedTenantDatabase()
	queryErrorDatabase.userQueryErrors = []error{errInternalTestDatabase}
	queryErrorStore := newManagedTenantStoreWithDatabase(queryErrorDatabase)
	if _, profileError := queryErrorStore.profile(principal); !errors.Is(profileError, errManagedTenantStorePersist) {
		t.Fatalf("profile query error=%v want %v", profileError, errManagedTenantStorePersist)
	}

	createErrorDatabase := newFakeManagedTenantDatabase()
	createErrorDatabase.createError = errInternalTestDatabase
	createErrorStore := newManagedTenantStoreWithDatabase(createErrorDatabase)
	if _, profileError := createErrorStore.profile(principal); !errors.Is(profileError, errManagedTenantStorePersist) {
		t.Fatalf("profile create error=%v want %v", profileError, errManagedTenantStorePersist)
	}

	saveTenantErrorDatabase := newFakeManagedTenantDatabase()
	saveTenantErrorDatabase.records[principal.userID] = internalManagedTenantRecord(principal.userID, "", fixedTime)
	saveTenantErrorDatabase.saveTenantError = errInternalTestDatabase
	saveTenantErrorStore := newManagedTenantStoreWithDatabase(saveTenantErrorDatabase)
	if _, profileError := saveTenantErrorStore.profile(principal); !errors.Is(profileError, errManagedTenantStorePersist) {
		t.Fatalf("profile save error=%v want %v", profileError, errManagedTenantStorePersist)
	}

	saveProviderKeyErrorDatabase := newFakeManagedTenantDatabase()
	saveProviderKeyErrorDatabase.records[principal.userID] = internalManagedTenantRecord(principal.userID, "", fixedTime)
	saveProviderKeyErrorDatabase.saveProviderKeyError = errInternalTestDatabase
	saveProviderKeyErrorStore := newManagedTenantStoreWithDatabase(saveProviderKeyErrorDatabase)
	if _, saveError := saveProviderKeyErrorStore.saveProviderKey(principal, newProviderID("openai"), "sk-openai", ModelNameGPT41, ""); !errors.Is(saveError, errManagedTenantStorePersist) {
		t.Fatalf("provider key save error=%v want %v", saveError, errManagedTenantStorePersist)
	}

	saveProviderKeyTenantErrorDatabase := newFakeManagedTenantDatabase()
	saveProviderKeyTenantErrorDatabase.records[principal.userID] = internalManagedTenantRecord(principal.userID, "", fixedTime)
	saveProviderKeyTenantErrorDatabase.saveTenantErrors = []error{nil, errInternalTestDatabase}
	saveProviderKeyTenantErrorStore := newManagedTenantStoreWithDatabase(saveProviderKeyTenantErrorDatabase)
	if _, saveError := saveProviderKeyTenantErrorStore.saveProviderKey(principal, newProviderID("openai"), "sk-openai", ModelNameGPT41, ""); !errors.Is(saveError, errManagedTenantStorePersist) {
		t.Fatalf("provider key tenant save error=%v want %v", saveError, errManagedTenantStorePersist)
	}

	saveProviderKeyRecordErrorDatabase := newFakeManagedTenantDatabase()
	saveProviderKeyRecordErrorDatabase.userQueryErrors = []error{errInternalTestDatabase}
	saveProviderKeyRecordErrorStore := newManagedTenantStoreWithDatabase(saveProviderKeyRecordErrorDatabase)
	if _, saveError := saveProviderKeyRecordErrorStore.saveProviderKey(principal, newProviderID("openai"), "sk-openai", ModelNameGPT41, ""); !errors.Is(saveError, errManagedTenantStorePersist) {
		t.Fatalf("provider key record error=%v want %v", saveError, errManagedTenantStorePersist)
	}

	deleteProviderKeyErrorDatabase := newFakeManagedTenantDatabase()
	deleteRecord := internalManagedTenantRecord(principal.userID, "", fixedTime)
	deleteRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{internalManagedProviderKeyRecord(t, principal.userID, "openai", "sk-openai", fixedTime)}
	deleteProviderKeyErrorDatabase.records[principal.userID] = deleteRecord
	deleteProviderKeyErrorDatabase.saveTenantErrors = []error{nil}
	deleteProviderKeyErrorDatabase.deleteProviderKeyError = errInternalTestDatabase
	deleteProviderKeyErrorStore := newManagedTenantStoreWithDatabase(deleteProviderKeyErrorDatabase)
	if _, deleteError := deleteProviderKeyErrorStore.removeProviderKey(principal, newProviderID("openai")); !errors.Is(deleteError, errManagedTenantStorePersist) {
		t.Fatalf("provider key delete error=%v want %v", deleteError, errManagedTenantStorePersist)
	}

	deleteProviderKeyTenantErrorDatabase := newFakeManagedTenantDatabase()
	deleteProviderKeyTenantErrorDatabase.records[principal.userID] = deleteRecord
	deleteProviderKeyTenantErrorDatabase.saveTenantErrors = []error{nil, errInternalTestDatabase}
	deleteProviderKeyTenantErrorStore := newManagedTenantStoreWithDatabase(deleteProviderKeyTenantErrorDatabase)
	if _, deleteError := deleteProviderKeyTenantErrorStore.removeProviderKey(principal, newProviderID("openai")); !errors.Is(deleteError, errManagedTenantStorePersist) {
		t.Fatalf("provider key delete tenant save error=%v want %v", deleteError, errManagedTenantStorePersist)
	}

	deleteProviderKeyRecordErrorDatabase := newFakeManagedTenantDatabase()
	deleteProviderKeyRecordErrorDatabase.userQueryErrors = []error{errInternalTestDatabase}
	deleteProviderKeyRecordErrorStore := newManagedTenantStoreWithDatabase(deleteProviderKeyRecordErrorDatabase)
	if _, deleteError := deleteProviderKeyRecordErrorStore.removeProviderKey(principal, newProviderID("openai")); !errors.Is(deleteError, errManagedTenantStorePersist) {
		t.Fatalf("provider key delete record error=%v want %v", deleteError, errManagedTenantStorePersist)
	}

	updateDefaultsErrorDatabase := newFakeManagedTenantDatabase()
	updateDefaultsErrorDatabase.records[principal.userID] = internalManagedTenantRecord(principal.userID, "", fixedTime)
	updateDefaultsErrorDatabase.saveTenantErrors = []error{nil, errInternalTestDatabase}
	updateDefaultsErrorStore := newManagedTenantStoreWithDatabase(updateDefaultsErrorDatabase)
	if _, updateError := updateDefaultsErrorStore.updateDefaults(principal, DefaultTenantDefaults()); !errors.Is(updateError, errManagedTenantStorePersist) {
		t.Fatalf("defaults update error=%v want %v", updateError, errManagedTenantStorePersist)
	}

	updateDefaultsRecordErrorDatabase := newFakeManagedTenantDatabase()
	updateDefaultsRecordErrorDatabase.userQueryErrors = []error{errInternalTestDatabase}
	updateDefaultsRecordErrorStore := newManagedTenantStoreWithDatabase(updateDefaultsRecordErrorDatabase)
	if _, updateError := updateDefaultsRecordErrorStore.updateDefaults(principal, DefaultTenantDefaults()); !errors.Is(updateError, errManagedTenantStorePersist) {
		t.Fatalf("defaults update record error=%v want %v", updateError, errManagedTenantStorePersist)
	}

	generateSecretSaveErrorDatabase := newFakeManagedTenantDatabase()
	generateSecretSaveErrorDatabase.saveTenantError = errInternalTestDatabase
	generateSecretSaveErrorStore := newManagedTenantStoreWithDatabase(generateSecretSaveErrorDatabase)
	if _, _, generateError := generateSecretSaveErrorStore.generateSecret(principal, func([sha256.Size]byte) bool { return false }); !errors.Is(generateError, errManagedTenantStorePersist) {
		t.Fatalf("generate secret save error=%v want %v", generateError, errManagedTenantStorePersist)
	}

	generateSecretSnapshotErrorDatabase := newFakeManagedTenantDatabase()
	generateSecretSnapshotErrorDatabase.userQueryErrors = []error{gorm.ErrRecordNotFound, errInternalTestDatabase}
	generateSecretSnapshotErrorStore := newManagedTenantStoreWithDatabase(generateSecretSnapshotErrorDatabase)
	if _, _, generateError := generateSecretSnapshotErrorStore.generateSecret(principal, func([sha256.Size]byte) bool { return false }); !errors.Is(generateError, errManagedTenantStorePersist) {
		t.Fatalf("generate secret snapshot error=%v want %v", generateError, errManagedTenantStorePersist)
	}

	generateSecretRecordErrorDatabase := newFakeManagedTenantDatabase()
	generateSecretRecordErrorDatabase.userQueryErrors = []error{errInternalTestDatabase}
	generateSecretRecordErrorStore := newManagedTenantStoreWithDatabase(generateSecretRecordErrorDatabase)
	if _, _, generateError := generateSecretRecordErrorStore.generateSecret(principal, func([sha256.Size]byte) bool { return false }); !errors.Is(generateError, errManagedTenantStorePersist) {
		t.Fatalf("generate secret record error=%v want %v", generateError, errManagedTenantStorePersist)
	}

	revokeSecretErrorDatabase := newFakeManagedTenantDatabase()
	revokeSecretDigest := sha256.Sum256([]byte("secret"))
	revokeSecretErrorDatabase.records[principal.userID] = internalManagedTenantRecord(principal.userID, hex.EncodeToString(revokeSecretDigest[:]), fixedTime)
	revokeSecretErrorDatabase.saveTenantErrors = []error{nil, errInternalTestDatabase}
	revokeSecretErrorStore := newManagedTenantStoreWithDatabase(revokeSecretErrorDatabase)
	if _, revokeError := revokeSecretErrorStore.revokeSecret(principal); !errors.Is(revokeError, errManagedTenantStorePersist) {
		t.Fatalf("revoke secret save error=%v want %v", revokeError, errManagedTenantStorePersist)
	}

	revokeSecretRecordErrorDatabase := newFakeManagedTenantDatabase()
	revokeSecretRecordErrorDatabase.userQueryErrors = []error{errInternalTestDatabase}
	revokeSecretRecordErrorStore := newManagedTenantStoreWithDatabase(revokeSecretRecordErrorDatabase)
	if _, revokeError := revokeSecretRecordErrorStore.revokeSecret(principal); !errors.Is(revokeError, errManagedTenantStorePersist) {
		t.Fatalf("revoke secret record error=%v want %v", revokeError, errManagedTenantStorePersist)
	}

	authenticateErrorDatabase := newFakeManagedTenantDatabase()
	authenticateErrorDatabase.secretQueryErrors = []error{errInternalTestDatabase}
	authenticateErrorStore := newManagedTenantStoreWithDatabase(authenticateErrorDatabase)
	if _, authenticated := authenticateErrorStore.authenticate("llmp_secret"); authenticated {
		t.Fatalf("secret query error authenticated")
	}

	invalidDigestRecord := internalManagedTenantRecord(principal.userID, "not-hex", fixedTime)
	invalidDigestDatabase := newFakeManagedTenantDatabase()
	invalidDigestDatabase.secretQueryRecord = &invalidDigestRecord
	invalidDigestStore := newManagedTenantStoreWithDatabase(invalidDigestDatabase)
	if _, authenticated := invalidDigestStore.authenticate("llmp_secret"); authenticated {
		t.Fatalf("invalid digest authenticated")
	}

	if _, digestValid := managedRecordSecretDigest(managedTenantRecord{}); digestValid {
		t.Fatalf("empty digest must be invalid")
	}
	if _, digestValid := managedRecordSecretDigest(managedTenantRecord{SecretDigest: "abc"}); digestValid {
		t.Fatalf("short digest must be invalid")
	}
}

func TestManagedTenantStoreUsageEdges(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	principal := managementPrincipal{userID: "tauth-usage-error-user"}
	managedTenant := tenant{identifier: tenantID("managed-usage"), userID: principal.userID, managed: true}

	noOpDatabase := newFakeManagedTenantDatabase()
	noOpStore := newManagedTenantStoreWithDatabase(noOpDatabase)
	if recordError := noOpStore.recordUsage(tenant{identifier: tenantID("static-usage")}, managedUsageEvent{statusCode: http.StatusOK}); recordError != nil {
		t.Fatalf("unmanaged record usage error=%v", recordError)
	}
	if recordError := noOpStore.recordUsage(tenant{identifier: tenantID("missing-user"), managed: true}, managedUsageEvent{statusCode: http.StatusOK}); recordError != nil {
		t.Fatalf("missing user record usage error=%v", recordError)
	}
	if len(noOpDatabase.usageEvents) != 0 {
		t.Fatalf("no-op usage events=%+v", noOpDatabase.usageEvents)
	}

	createErrorDatabase := newFakeManagedTenantDatabase()
	createErrorDatabase.createUsageEventError = errInternalTestDatabase
	createErrorStore := newManagedTenantStoreWithDatabase(createErrorDatabase)
	createErrorStore.now = func() time.Time { return fixedTime }
	recordError := createErrorStore.recordUsage(managedTenant, managedUsageEvent{
		endpoint:            usageEndpointText,
		providerIdentifier:  ProviderNameOpenAI,
		modelIdentifier:     ModelNameGPT41,
		statusCode:          http.StatusOK,
		latencyMilliseconds: 17,
		usage:               &tokenUsage{RequestTokens: 1, ResponseTokens: 2, TotalTokens: 3},
	})
	if !errors.Is(recordError, errManagedTenantStorePersist) {
		t.Fatalf("record usage error=%v want %v", recordError, errManagedTenantStorePersist)
	}

	queryRecordErrorDatabase := newFakeManagedTenantDatabase()
	queryRecordErrorDatabase.userQueryErrors = []error{errInternalTestDatabase}
	queryRecordErrorStore := newManagedTenantStoreWithDatabase(queryRecordErrorDatabase)
	if _, summaryError := queryRecordErrorStore.usageSummary(principal); !errors.Is(summaryError, errManagedTenantStorePersist) {
		t.Fatalf("usage summary record error=%v want %v", summaryError, errManagedTenantStorePersist)
	}

	queryUsageErrorDatabase := newFakeManagedTenantDatabase()
	queryUsageErrorDatabase.records[principal.userID] = internalManagedTenantRecord(principal.userID, "", fixedTime)
	queryUsageErrorDatabase.usageEventsQueryError = errInternalTestDatabase
	queryUsageErrorStore := newManagedTenantStoreWithDatabase(queryUsageErrorDatabase)
	if _, summaryError := queryUsageErrorStore.usageSummary(principal); !errors.Is(summaryError, errManagedTenantStorePersist) {
		t.Fatalf("usage summary usage error=%v want %v", summaryError, errManagedTenantStorePersist)
	}

	boundedUsageDatabase := newFakeManagedTenantDatabase()
	boundedUsageDatabase.records[principal.userID] = internalManagedTenantRecord(principal.userID, "", fixedTime)
	boundedUsageDatabase.usageEvents = []managedUsageEventRecord{
		{UserID: principal.userID, ProviderID: "old", ModelID: "old-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: fixedTime.AddDate(0, 0, -managedUsageSummaryDays)},
		{UserID: principal.userID, ProviderID: "current", ModelID: "current-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: fixedTime},
	}
	boundedUsageStore := newManagedTenantStoreWithDatabase(boundedUsageDatabase)
	boundedUsageStore.now = func() time.Time { return fixedTime }
	boundedSummary, boundedSummaryError := boundedUsageStore.usageSummary(principal)
	if boundedSummaryError != nil {
		t.Fatalf("bounded usage summary error=%v", boundedSummaryError)
	}
	if boundedSummary.totals.requests != 1 || !boundedUsageDatabase.usageEventsQueryPeriodStart.Equal(usagePeriodStart(fixedTime)) {
		t.Fatalf("bounded summary=%+v period_start=%s", boundedSummary.totals, boundedUsageDatabase.usageEventsQueryPeriodStart)
	}

	observedCore, observedLogs := observer.New(zapcore.WarnLevel)
	recordManagedUsage(createErrorStore, zap.New(observedCore).Sugar(), managedTenant, usageEndpointText, ProviderNameOpenAI, ModelNameGPT41, http.StatusOK, nil, fixedTime.Add(-time.Second))
	if observedLogs.Len() != 1 || observedLogs.All()[0].Message != logEventUsageRecordFailed {
		t.Fatalf("usage record logs=%+v", observedLogs.All())
	}

	service := newInternalManagementService(queryUsageErrorDatabase)
	status := executeInternalManagementHandler(service.usageHandler(), http.MethodGet, "/api/management/usage", "", nil, principal)
	if status != http.StatusInternalServerError {
		t.Fatalf("usage handler status=%d want=%d", status, http.StatusInternalServerError)
	}

	adminTenantQueryErrorDatabase := newFakeManagedTenantDatabase()
	adminTenantQueryErrorDatabase.userQueryErrors = []error{errInternalTestDatabase}
	if _, adminError := newManagedTenantStoreWithDatabase(adminTenantQueryErrorDatabase).adminUsersSummary(); !errors.Is(adminError, errManagedTenantStorePersist) {
		t.Fatalf("admin tenant query error=%v want %v", adminError, errManagedTenantStorePersist)
	}

	adminUsageQueryErrorDatabase := newFakeManagedTenantDatabase()
	adminUsageQueryErrorDatabase.records[principal.userID] = internalManagedTenantRecord(principal.userID, "", fixedTime)
	adminUsageQueryErrorDatabase.usageEventsQueryError = errInternalTestDatabase
	if _, adminError := newManagedTenantStoreWithDatabase(adminUsageQueryErrorDatabase).adminUsersSummary(); !errors.Is(adminError, errManagedTenantStorePersist) {
		t.Fatalf("admin usage query error=%v want %v", adminError, errManagedTenantStorePersist)
	}

	adminOrderingDatabase := newFakeManagedTenantDatabase()
	firstAdminRecord := internalManagedTenantRecord("admin-user-b", "", fixedTime)
	firstAdminRecord.UserEmail = "same@example.com"
	secondAdminRecord := internalManagedTenantRecord("admin-user-a", "", fixedTime)
	secondAdminRecord.UserEmail = "same@example.com"
	adminOrderingDatabase.records[firstAdminRecord.UserID] = firstAdminRecord
	adminOrderingDatabase.records[secondAdminRecord.UserID] = secondAdminRecord
	adminSnapshots, adminSnapshotsError := newManagedTenantStoreWithDatabase(adminOrderingDatabase).adminUsersSummary()
	if adminSnapshotsError != nil || len(adminSnapshots) != 2 || adminSnapshots[0].userID != "admin-user-a" {
		t.Fatalf("admin snapshots=%+v error=%v", adminSnapshots, adminSnapshotsError)
	}
}

func TestManagedUsageSummaryBucketsAndOrdering(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	summary := summarizeManagedUsage([]managedUsageEventRecord{
		{UserID: "user", ProviderID: "old", ModelID: "old-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: now.AddDate(0, 0, -managedUsageSummaryDays)},
		{UserID: "user", ProviderID: "current", ModelID: "current-model", Endpoint: usageEndpointText, StatusCode: http.StatusOK, Success: true, CreatedAt: now},
	}, now)
	if summary.totals.requests != 1 || summary.providers[0].providerIdentifier != "current" {
		t.Fatalf("summary totals=%+v providers=%+v", summary.totals, summary.providers)
	}

	providers := usageProviderBucketList(map[string]managedUsageAggregate{
		"beta":  {requests: 1},
		"alpha": {requests: 1},
		"gamma": {requests: 2},
	})
	if len(providers) != 3 || providers[0].providerIdentifier != "gamma" || providers[1].providerIdentifier != "alpha" || providers[2].providerIdentifier != "beta" {
		t.Fatalf("providers=%+v", providers)
	}

	models := usageModelBucketList(map[string]managedUsageModelBucket{
		"beta/model": {
			providerIdentifier: "beta",
			modelIdentifier:    "model",
			aggregate:          managedUsageAggregate{requests: 1},
		},
		"alpha/zeta": {
			providerIdentifier: "alpha",
			modelIdentifier:    "zeta",
			aggregate:          managedUsageAggregate{requests: 1},
		},
		"alpha/alpha": {
			providerIdentifier: "alpha",
			modelIdentifier:    "alpha",
			aggregate:          managedUsageAggregate{requests: 1},
		},
		"gamma/model": {
			providerIdentifier: "gamma",
			modelIdentifier:    "model",
			aggregate:          managedUsageAggregate{requests: 2},
		},
	})
	if len(models) != 4 ||
		models[0].providerIdentifier != "gamma" ||
		models[1].providerIdentifier != "alpha" ||
		models[1].modelIdentifier != "alpha" ||
		models[2].providerIdentifier != "alpha" ||
		models[2].modelIdentifier != "zeta" ||
		models[3].providerIdentifier != "beta" {
		t.Fatalf("models=%+v", models)
	}
}

func TestManagedTenantStoreStaticConfigMigrationEdges(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	legacyTenantSecretDigest := sha256.Sum256([]byte("legacy-secret"))
	legacyTenant := tenant{
		identifier:   tenantID("legacy"),
		secretDigest: legacyTenantSecretDigest,
		defaults:     newTenantDefaults(TenantDefaults{Provider: ProviderNameDeepSeek, Model: ModelNameDeepSeekV4Flash, DictationProvider: ProviderNameOpenAI, DictationModel: DefaultDictationModel}),
	}
	configuration := Configuration{
		OpenAIKey:   "sk-openai",
		DeepSeekKey: "sk-deepseek",
		tenants: tenantRegistry{
			tenants: []tenant{legacyTenant},
		},
	}

	database := newFakeManagedTenantDatabase()
	store := newManagedTenantStoreWithDatabase(database)
	store.now = func() time.Time { return fixedTime }
	if migrationError := store.migrateStaticConfiguration(configuration); migrationError != nil {
		t.Fatalf("migrate static configuration: %v", migrationError)
	}
	if _, exists := database.migrations[staticConfigMigrationID]; !exists {
		t.Fatalf("migration marker was not saved")
	}
	record, exists := database.records[staticConfigTenantUserID(legacyTenant.identifier)]
	if !exists {
		t.Fatalf("legacy tenant was not imported")
	}
	if record.TenantID != "legacy" || record.SecretDigest != hex.EncodeToString(legacyTenantSecretDigest[:]) {
		t.Fatalf("legacy record=%+v", record)
	}
	providerAPIKeys, providerKeyError := store.providerAPIKeyMap(record.ProviderAPIKeys)
	if providerKeyError != nil {
		t.Fatalf("provider key map error=%v", providerKeyError)
	}
	if providerAPIKeys[newProviderID(ProviderNameOpenAI)] != "sk-openai" || providerAPIKeys[newProviderID(ProviderNameDeepSeek)] != "sk-deepseek" {
		t.Fatalf("provider keys=%v", providerAPIKeys)
	}

	database.records[staticConfigTenantUserID(legacyTenant.identifier)] = internalManagedTenantRecord(staticConfigTenantUserID(legacyTenant.identifier), "", fixedTime)
	if migrationError := store.migrateStaticConfiguration(Configuration{OpenAIKey: "sk-stale"}); migrationError != nil {
		t.Fatalf("repeat migration: %v", migrationError)
	}
	if repeatRecord := database.records[staticConfigTenantUserID(legacyTenant.identifier)]; len(repeatRecord.ProviderAPIKeys) != 0 {
		t.Fatalf("migration reran after marker: %+v", repeatRecord.ProviderAPIKeys)
	}

	migrationQueryErrorDatabase := newFakeManagedTenantDatabase()
	migrationQueryErrorDatabase.migrationQueryErrors = []error{errInternalTestDatabase}
	migrationQueryErrorStore := newManagedTenantStoreWithDatabase(migrationQueryErrorDatabase)
	if migrationError := migrationQueryErrorStore.migrateStaticConfiguration(configuration); !errors.Is(migrationError, errManagedTenantStorePersist) {
		t.Fatalf("migration query error=%v want %v", migrationError, errManagedTenantStorePersist)
	}

	migrationTenantSaveErrorDatabase := newFakeManagedTenantDatabase()
	migrationTenantSaveErrorDatabase.saveTenantError = errInternalTestDatabase
	migrationTenantSaveErrorStore := newManagedTenantStoreWithDatabase(migrationTenantSaveErrorDatabase)
	if migrationError := migrationTenantSaveErrorStore.migrateStaticConfiguration(configuration); !errors.Is(migrationError, errManagedTenantStorePersist) {
		t.Fatalf("migration tenant save error=%v want %v", migrationError, errManagedTenantStorePersist)
	}

	migrationProviderKeyErrorDatabase := newFakeManagedTenantDatabase()
	migrationProviderKeyErrorDatabase.saveProviderKeyError = errInternalTestDatabase
	migrationProviderKeyErrorStore := newManagedTenantStoreWithDatabase(migrationProviderKeyErrorDatabase)
	if migrationError := migrationProviderKeyErrorStore.migrateStaticConfiguration(configuration); !errors.Is(migrationError, errManagedTenantStorePersist) {
		t.Fatalf("migration provider key error=%v want %v", migrationError, errManagedTenantStorePersist)
	}

	migrationEncryptionErrorStore := newManagedTenantStoreWithDatabase(newFakeManagedTenantDatabase())
	migrationEncryptionErrorStore.randomReader = strings.NewReader("")
	if migrationError := migrationEncryptionErrorStore.migrateStaticConfiguration(configuration); !errors.Is(migrationError, errManagedProviderKeyEncryption) {
		t.Fatalf("migration provider key encryption error=%v want %v", migrationError, errManagedProviderKeyEncryption)
	}

	migrationMarkerErrorDatabase := newFakeManagedTenantDatabase()
	migrationMarkerErrorDatabase.createMigrationError = errInternalTestDatabase
	migrationMarkerErrorStore := newManagedTenantStoreWithDatabase(migrationMarkerErrorDatabase)
	if migrationError := migrationMarkerErrorStore.migrateStaticConfiguration(configuration); !errors.Is(migrationError, errManagedTenantStorePersist) {
		t.Fatalf("migration marker error=%v want %v", migrationError, errManagedTenantStorePersist)
	}
}

func TestManagedTenantGORMDatabaseOpenError(t *testing.T) {
	_, storeError := newGORMManagedTenantDatabase(ManagementConfiguration{DatabaseDialect: ManagementDatabaseDialectPostgres, DatabaseDSN: "postgres://%"})
	if !errors.Is(storeError, errManagedTenantStoreOpen) {
		t.Fatalf("gorm store error=%v want %v", storeError, errManagedTenantStoreOpen)
	}
	_, managedStoreError := newManagedTenantStore(ManagementConfiguration{
		DatabaseDialect:          ManagementDatabaseDialectPostgres,
		DatabaseDSN:              "postgres://%",
		ProviderKeyEncryptionKey: testManagedProviderKeyEncryptionKey,
	})
	if !errors.Is(managedStoreError, errManagedTenantStoreOpen) {
		t.Fatalf("managed store error=%v want %v", managedStoreError, errManagedTenantStoreOpen)
	}

	_, missingDialectError := newGORMManagedTenantDatabase(ManagementConfiguration{DatabaseDSN: "postgres://%"})
	if !errors.Is(missingDialectError, errManagedTenantStoreOpen) {
		t.Fatalf("gorm missing dialect error=%v want %v", missingDialectError, errManagedTenantStoreOpen)
	}

	_, dialectError := newGORMManagedTenantDatabase(ManagementConfiguration{DatabaseDialect: "mysql", DatabaseDSN: "mysql://example"})
	if !errors.Is(dialectError, errManagedTenantStoreOpen) {
		t.Fatalf("gorm dialect error=%v want %v", dialectError, errManagedTenantStoreOpen)
	}

	_, migrationError := newGORMManagedTenantDatabase(ManagementConfiguration{
		DatabaseDialect: ManagementDatabaseDialectSQLite,
		DatabaseDSN:     "sqlite-test-management",
		DatabaseDialector: failingAutoMigrateDialector{
			Dialector: sqlite.Open(":memory:"),
		},
	})
	if !errors.Is(migrationError, errManagedTenantStoreOpen) {
		t.Fatalf("gorm migration error=%v want %v", migrationError, errManagedTenantStoreOpen)
	}

	_, invalidKeyStoreError := newManagedTenantStore(ManagementConfiguration{
		DatabaseDialect:          ManagementDatabaseDialectSQLite,
		DatabaseDSN:              filepath.Join(t.TempDir(), "invalid-key.db"),
		ProviderKeyEncryptionKey: "not-base64",
	})
	if !errors.Is(invalidKeyStoreError, errManagedTenantStoreOpen) {
		t.Fatalf("invalid key store error=%v want %v", invalidKeyStoreError, errManagedTenantStoreOpen)
	}

	readonlyDatabasePath := filepath.Join(t.TempDir(), "readonly-migration.db")
	readonlySeedDatabase, readonlySeedError := newGORMManagedTenantDatabase(ManagementConfiguration{
		DatabaseDialect: ManagementDatabaseDialectSQLite,
		DatabaseDSN:     readonlyDatabasePath,
	})
	if readonlySeedError != nil {
		t.Fatalf("readonly seed database error=%v", readonlySeedError)
	}
	readonlyTenantRecord := internalManagedTenantRecord("readonly-migration-user", "", time.Now().UTC())
	if createError := readonlySeedDatabase.createTenant(readonlyTenantRecord); createError != nil {
		t.Fatalf("readonly seed create tenant error=%v", createError)
	}
	if saveError := readonlySeedDatabase.saveProviderKey(managedProviderAPIKeyRecord{
		UserID:     readonlyTenantRecord.UserID,
		ProviderID: "openai",
		APIKey:     "sk-readonly",
	}); saveError != nil {
		t.Fatalf("readonly seed provider key error=%v", saveError)
	}
	_, readonlyStoreError := newManagedTenantStore(ManagementConfiguration{
		DatabaseDialect:          ManagementDatabaseDialectSQLite,
		DatabaseDSN:              "file:" + readonlyDatabasePath + "?mode=ro",
		ProviderKeyEncryptionKey: testManagedProviderKeyEncryptionKey,
	})
	if !errors.Is(readonlyStoreError, errManagedTenantStorePersist) {
		t.Fatalf("readonly migration store error=%v want %v", readonlyStoreError, errManagedTenantStorePersist)
	}
}

func TestManagedTenantGORMDatabaseEncryptsProviderKeysAtRest(t *testing.T) {
	store, storeError := newManagedTenantStore(ManagementConfiguration{
		DatabaseDialect:          ManagementDatabaseDialectSQLite,
		DatabaseDSN:              filepath.Join(t.TempDir(), "managed-tenants.db"),
		ProviderKeyEncryptionKey: testManagedProviderKeyEncryptionKey,
	})
	if storeError != nil {
		t.Fatalf("new managed tenant store: %v", storeError)
	}
	principal := managementPrincipal{userID: "tauth-gorm-encryption-user"}
	if _, saveError := store.saveProviderKey(principal, newProviderID("openai"), "sk-openai", ModelNameGPT41, "provider system"); saveError != nil {
		t.Fatalf("save provider key: %v", saveError)
	}
	record, recordError := store.database.tenantByUserID(principal.userID)
	if recordError != nil {
		t.Fatalf("load record: %v", recordError)
	}
	if len(record.ProviderAPIKeys) != 1 {
		t.Fatalf("provider key records=%+v", record.ProviderAPIKeys)
	}
	providerKeyRecord := record.ProviderAPIKeys[0]
	if providerKeyRecord.APIKey != "" || !strings.HasPrefix(providerKeyRecord.EncryptedAPIKey, managedProviderKeyCiphertextPrefix) || strings.Contains(providerKeyRecord.EncryptedAPIKey, "sk-openai") {
		t.Fatalf("provider key record=%+v", providerKeyRecord)
	}
	if providerKeyRecord.TextModel != ModelNameGPT41 || providerKeyRecord.SystemPrompt != "provider system" {
		t.Fatalf("provider key settings=%+v", providerKeyRecord)
	}
	snapshot, snapshotError := store.profile(principal)
	if snapshotError != nil || snapshot.providerAPIKeys[newProviderID("openai")] != "sk-openai" {
		t.Fatalf("snapshot=%+v error=%v", snapshot, snapshotError)
	}
}

func TestManagedTenantGORMDatabaseMigratesUsageIndexes(t *testing.T) {
	database, databaseError := newGORMManagedTenantDatabase(ManagementConfiguration{
		DatabaseDialect: ManagementDatabaseDialectSQLite,
		DatabaseDSN:     filepath.Join(t.TempDir(), "managed-usage-indexes.db"),
	})
	if databaseError != nil {
		t.Fatalf("new gorm database: %v", databaseError)
	}
	for _, indexName := range []string{"idx_managed_usage_user_created", "idx_managed_usage_created_at"} {
		if !database.database.Migrator().HasIndex(&managedUsageEventRecord{}, indexName) {
			t.Fatalf("missing usage index %s", indexName)
		}
	}
}

func TestManagementHandlerStoreErrorEdges(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	principal := managementPrincipal{userID: "tauth-handler-error-user"}

	profileErrorDatabase := newFakeManagedTenantDatabase()
	profileErrorDatabase.userQueryErrors = []error{errInternalTestDatabase}
	profileErrorService := newInternalManagementService(profileErrorDatabase)
	if responseCode := executeInternalManagementHandler(profileErrorService.profileHandler(), http.MethodGet, "/api/management/profile", "", nil, principal); responseCode != http.StatusInternalServerError {
		t.Fatalf("profile error status=%d want=%d", responseCode, http.StatusInternalServerError)
	}

	removeErrorDatabase := newFakeManagedTenantDatabase()
	removeRecord := internalManagedTenantRecord(principal.userID, "", fixedTime)
	removeRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{internalManagedProviderKeyRecord(t, principal.userID, "openai", "sk-openai", fixedTime)}
	removeErrorDatabase.records[principal.userID] = removeRecord
	removeErrorDatabase.saveTenantErrors = []error{nil}
	removeErrorDatabase.deleteProviderKeyError = errInternalTestDatabase
	removeErrorService := newInternalManagementService(removeErrorDatabase)
	if responseCode := executeInternalManagementHandler(removeErrorService.removeProviderKeyHandler(), http.MethodDelete, "/api/management/provider-keys/openai", "", gin.Params{{Key: "provider", Value: "openai"}}, principal); responseCode != http.StatusInternalServerError {
		t.Fatalf("remove error status=%d want=%d", responseCode, http.StatusInternalServerError)
	}

	adminErrorDatabase := newFakeManagedTenantDatabase()
	adminErrorDatabase.userQueryErrors = []error{errInternalTestDatabase}
	adminErrorService := newInternalManagementService(adminErrorDatabase)
	adminPrincipal := managementPrincipal{userID: "tauth-admin-error-user", isAdmin: true}
	if responseCode := executeInternalManagementHandler(adminErrorService.adminUsersHandler(), http.MethodGet, "/api/management/admin/users", "", nil, adminPrincipal); responseCode != http.StatusInternalServerError {
		t.Fatalf("admin error status=%d want=%d", responseCode, http.StatusInternalServerError)
	}

	defaultsProfileErrorDatabase := newFakeManagedTenantDatabase()
	defaultsProfileErrorDatabase.userQueryErrors = []error{errInternalTestDatabase}
	defaultsProfileErrorService := newInternalManagementService(defaultsProfileErrorDatabase)
	defaultsBody := `{"provider":"openai","model":"` + ModelNameGPT41 + `","dictation_provider":"openai","dictation_model":"` + DefaultDictationModel + `","system_prompt":""}`
	if responseCode := executeInternalManagementHandler(defaultsProfileErrorService.updateDefaultsHandler(), http.MethodPut, "/api/management/defaults", defaultsBody, nil, principal); responseCode != http.StatusInternalServerError {
		t.Fatalf("defaults profile error status=%d want=%d", responseCode, http.StatusInternalServerError)
	}

	defaultsStoreErrorDatabase := newFakeManagedTenantDatabase()
	defaultsRecord := internalManagedTenantRecord(principal.userID, "", fixedTime)
	defaultsRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{internalManagedProviderKeyRecord(t, principal.userID, "openai", "sk-openai", fixedTime)}
	defaultsStoreErrorDatabase.records[principal.userID] = defaultsRecord
	defaultsStoreErrorDatabase.saveTenantErrors = []error{nil, nil, errInternalTestDatabase}
	defaultsStoreErrorService := newInternalManagementService(defaultsStoreErrorDatabase)
	if responseCode := executeInternalManagementHandler(defaultsStoreErrorService.updateDefaultsHandler(), http.MethodPut, "/api/management/defaults", defaultsBody, nil, principal); responseCode != http.StatusInternalServerError {
		t.Fatalf("defaults store error status=%d want=%d", responseCode, http.StatusInternalServerError)
	}

	generateErrorDatabase := newFakeManagedTenantDatabase()
	generateErrorService := newInternalManagementService(generateErrorDatabase)
	generateErrorService.store.randomReader = strings.NewReader("")
	if responseCode := executeInternalManagementHandler(generateErrorService.generateSecretHandler(), http.MethodPost, "/api/management/secrets", `{}`, nil, principal); responseCode != http.StatusInternalServerError {
		t.Fatalf("generate error status=%d want=%d", responseCode, http.StatusInternalServerError)
	}

	revokeErrorDatabase := newFakeManagedTenantDatabase()
	revokeErrorDatabase.records[principal.userID] = internalManagedTenantRecord(principal.userID, "", fixedTime)
	revokeErrorDatabase.saveTenantErrors = []error{nil, errInternalTestDatabase}
	revokeErrorService := newInternalManagementService(revokeErrorDatabase)
	if responseCode := executeInternalManagementHandler(revokeErrorService.revokeSecretHandler(), http.MethodDelete, "/api/management/secrets", "", nil, principal); responseCode != http.StatusInternalServerError {
		t.Fatalf("revoke error status=%d want=%d", responseCode, http.StatusInternalServerError)
	}
}

func TestBuildRouterReturnsStaticConfigMigrationError(t *testing.T) {
	failingDatabase := newFakeManagedTenantDatabase()
	failingDatabase.migrationQueryErrors = []error{errInternalTestDatabase}
	_, buildError := buildRouter(internalManagementRouterConfiguration(), zap.NewNop().Sugar(), func(ManagementConfiguration) (*managedTenantStore, error) {
		return newManagedTenantStoreWithDatabase(failingDatabase), nil
	})
	if !errors.Is(buildError, errManagedTenantStorePersist) {
		t.Fatalf("BuildRouter error=%v want %v", buildError, errManagedTenantStorePersist)
	}
}

func TestBuildRouterReturnsProviderTextSettingsMigrationError(t *testing.T) {
	failingDatabase := newFakeManagedTenantDatabase()
	failingDatabase.userQueryErrors = []error{errInternalTestDatabase}
	_, buildError := buildRouter(internalManagementRouterConfiguration(), zap.NewNop().Sugar(), func(ManagementConfiguration) (*managedTenantStore, error) {
		return newManagedTenantStoreWithDatabase(failingDatabase), nil
	})
	if !errors.Is(buildError, errManagedTenantStorePersist) {
		t.Fatalf("BuildRouter error=%v want %v", buildError, errManagedTenantStorePersist)
	}
}

func TestTextRequestDefaultsForProviderInternalEdges(t *testing.T) {
	providers := newProviderRegistry(Configuration{
		OpenAIKey:      "sk-openai",
		DeepSeekKey:    "sk-deepseek",
		ProviderModels: internalProviderModelCatalogs(),
	})
	staticTenant := tenant{
		defaults: newTenantDefaults(TenantDefaults{
			Provider:     ProviderNameOpenAI,
			Model:        ModelNameGPT41,
			SystemPrompt: "tenant system",
		}),
	}
	staticExplicitDefaults := textRequestDefaultsForProvider(ProviderNameDeepSeek, staticTenant, providers)
	if staticExplicitDefaults.model != "" || staticExplicitDefaults.systemPrompt != "tenant system" {
		t.Fatalf("static explicit defaults=%+v", staticExplicitDefaults)
	}

	managedTenant := staticTenant
	managedTenant.managed = true
	managedNoSettingsDefaults := textRequestDefaultsForProvider(ProviderNameOpenAI, managedTenant, providers)
	if managedNoSettingsDefaults.model != "" || managedNoSettingsDefaults.systemPrompt != "tenant system" {
		t.Fatalf("managed no-settings defaults=%+v", managedNoSettingsDefaults)
	}
	managedUnknownProviderDefaults := textRequestDefaultsForProvider("unknown-provider", managedTenant, providers)
	if managedUnknownProviderDefaults.model != "" || managedUnknownProviderDefaults.systemPrompt != "tenant system" {
		t.Fatalf("managed unknown-provider defaults=%+v", managedUnknownProviderDefaults)
	}

	managedTenant.providerSettings = map[providerID]managedProviderSettings{
		newProviderID(ProviderNameOpenAI): {
			textModel:    ModelNameGPT55,
			systemPrompt: "saved system",
		},
	}
	managedSavedOmittedDefaults := textRequestDefaultsForProvider("", managedTenant, providers)
	if managedSavedOmittedDefaults.model != ModelNameGPT41 || managedSavedOmittedDefaults.systemPrompt != "tenant system" {
		t.Fatalf("managed saved omitted defaults=%+v", managedSavedOmittedDefaults)
	}
	managedSavedExplicitDefaults := textRequestDefaultsForProvider(ProviderNameOpenAI, managedTenant, providers)
	if managedSavedExplicitDefaults.model != ModelNameGPT55 || managedSavedExplicitDefaults.systemPrompt != "saved system" {
		t.Fatalf("managed saved explicit defaults=%+v", managedSavedExplicitDefaults)
	}
}

func TestProviderKeyRejectionInternalEdges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	ginContext.Request.Body = failingReadCloser{}
	if _, ok := readJSONProxyBody(ginContext); ok || response.Code != http.StatusBadRequest {
		t.Fatalf("readJSONProxyBody ok=%v status=%d", ok, response.Code)
	}

	formResponse := httptest.NewRecorder()
	formContext, _ := gin.CreateTestContext(formResponse)
	formContext.Request = httptest.NewRequest(http.MethodPost, "/dictate", nil)
	if rejectClientProviderCredentialsFromForm(formContext) {
		t.Fatalf("nil multipart form must not be rejected")
	}
	if forbiddenClientProviderCredentialParameter(" ") {
		t.Fatalf("blank provider credential parameter must not be forbidden")
	}
}

func TestProviderSummaryInternalEdges(t *testing.T) {
	textModels := sortedTextModels(map[string]textModelDefinition{
		"alias-a": {identifier: newModelID("same-text-model")},
		"alias-b": {identifier: newModelID("same-text-model")},
		"other":   {identifier: newModelID("other-text-model")},
	})
	if !reflect.DeepEqual(textModels, []string{"other-text-model", "same-text-model"}) {
		t.Fatalf("text models=%v", textModels)
	}
	dictationModels := sortedDictationModels(map[string]modelID{
		"alias-a": newModelID("same-dictation-model"),
		"alias-b": newModelID("same-dictation-model"),
		"other":   newModelID("other-dictation-model"),
	})
	if !reflect.DeepEqual(dictationModels, []string{"other-dictation-model", "same-dictation-model"}) {
		t.Fatalf("dictation models=%v", dictationModels)
	}
	if label := providerLabel(newProviderID("custom-provider")); label != "custom-provider" {
		t.Fatalf("provider label=%q", label)
	}
}

func TestManagementConfigurationInternalEdges(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, decodeError := decodeManagedProviderKey(shortKey); decodeError == nil || !strings.Contains(decodeError.Error(), "invalid_length") {
		t.Fatalf("short key decode error=%v want invalid_length", decodeError)
	}
	for _, rawEmail := range []string{" ", "Admin <admin@example.com>"} {
		if _, emailError := normalizeManagementAdminEmail(rawEmail); !errors.Is(emailError, ErrInvalidManagementConfiguration) {
			t.Fatalf("admin email error=%v want %v", emailError, ErrInvalidManagementConfiguration)
		}
	}
}

func TestTenantRegistryContainsSecretDigestEdges(t *testing.T) {
	secretDigest := sha256.Sum256([]byte("service-secret"))
	serviceTenantID, tenantIDError := newTenantID("service")
	if tenantIDError != nil {
		t.Fatalf("new tenant id: %v", tenantIDError)
	}
	registry := tenantRegistry{
		tenants: []tenant{
			{
				identifier:   serviceTenantID,
				secretDigest: secretDigest,
				defaults:     newTenantDefaults(DefaultTenantDefaults()),
			},
		},
	}
	if !registry.containsSecretDigest(secretDigest) {
		t.Fatalf("registry did not find known secret digest")
	}
	if registry.containsSecretDigest(sha256.Sum256([]byte("other-secret"))) {
		t.Fatalf("registry matched unknown secret digest")
	}
}

func newFakeManagedTenantDatabase() *fakeManagedTenantDatabase {
	return &fakeManagedTenantDatabase{
		records:    map[string]managedTenantRecord{},
		migrations: map[string]managedStaticConfigMigrationRecord{},
	}
}

type fakeManagedTenantDatabase struct {
	records                     map[string]managedTenantRecord
	usageEvents                 []managedUsageEventRecord
	migrations                  map[string]managedStaticConfigMigrationRecord
	userQueryErrors             []error
	secretQueryErrors           []error
	migrationQueryErrors        []error
	secretQueryRecord           *managedTenantRecord
	createError                 error
	saveTenantError             error
	saveTenantErrors            []error
	saveProviderKeyError        error
	deleteProviderKeyError      error
	createUsageEventError       error
	usageEventsQueryError       error
	usageEventsQueryPeriodStart time.Time
	createMigrationError        error
}

func (database *fakeManagedTenantDatabase) tenantByUserID(userID string) (managedTenantRecord, error) {
	if queryError, hasQueryError := database.popUserQueryError(); hasQueryError {
		return managedTenantRecord{}, queryError
	}
	record, foundRecord := database.records[userID]
	if !foundRecord {
		return managedTenantRecord{}, gorm.ErrRecordNotFound
	}
	return cloneManagedTenantRecord(record), nil
}

func (database *fakeManagedTenantDatabase) tenantBySecretDigest(secretDigest string) (managedTenantRecord, error) {
	if queryError, hasQueryError := database.popSecretQueryError(); hasQueryError {
		return managedTenantRecord{}, queryError
	}
	if database.secretQueryRecord != nil {
		return cloneManagedTenantRecord(*database.secretQueryRecord), nil
	}
	for _, record := range database.records {
		if record.SecretDigest == secretDigest {
			return cloneManagedTenantRecord(record), nil
		}
	}
	return managedTenantRecord{}, gorm.ErrRecordNotFound
}

func (database *fakeManagedTenantDatabase) tenants() ([]managedTenantRecord, error) {
	if queryError, hasQueryError := database.popUserQueryError(); hasQueryError {
		return nil, queryError
	}
	records := make([]managedTenantRecord, 0, len(database.records))
	for _, record := range database.records {
		records = append(records, cloneManagedTenantRecord(record))
	}
	return records, nil
}

func (database *fakeManagedTenantDatabase) providerKeys() ([]managedProviderAPIKeyRecord, error) {
	if queryError, hasQueryError := database.popUserQueryError(); hasQueryError {
		return nil, queryError
	}
	records := []managedProviderAPIKeyRecord{}
	for _, tenantRecord := range database.records {
		records = append(records, tenantRecord.ProviderAPIKeys...)
	}
	return records, nil
}

func (database *fakeManagedTenantDatabase) createTenant(record managedTenantRecord) error {
	if database.createError != nil {
		return database.createError
	}
	database.records[record.UserID] = cloneManagedTenantRecord(record)
	return nil
}

func (database *fakeManagedTenantDatabase) saveTenant(record managedTenantRecord) error {
	if saveError, hasSaveError := database.popSaveTenantError(); hasSaveError && saveError != nil {
		return saveError
	}
	if database.saveTenantError != nil {
		return database.saveTenantError
	}
	existingRecord, foundExistingRecord := database.records[record.UserID]
	if foundExistingRecord {
		record.ProviderAPIKeys = existingRecord.ProviderAPIKeys
	}
	database.records[record.UserID] = cloneManagedTenantRecord(record)
	return nil
}

func (database *fakeManagedTenantDatabase) saveProviderKey(record managedProviderAPIKeyRecord) error {
	if database.saveProviderKeyError != nil {
		return database.saveProviderKeyError
	}
	tenantRecord := cloneManagedTenantRecord(database.records[record.UserID])
	updatedProviderKeys := make([]managedProviderAPIKeyRecord, 0, len(tenantRecord.ProviderAPIKeys)+1)
	replacedProviderKey := false
	for _, existingRecord := range tenantRecord.ProviderAPIKeys {
		if existingRecord.ProviderID == record.ProviderID {
			updatedProviderKeys = append(updatedProviderKeys, record)
			replacedProviderKey = true
		} else {
			updatedProviderKeys = append(updatedProviderKeys, existingRecord)
		}
	}
	if !replacedProviderKey {
		updatedProviderKeys = append(updatedProviderKeys, record)
	}
	tenantRecord.ProviderAPIKeys = updatedProviderKeys
	database.records[record.UserID] = cloneManagedTenantRecord(tenantRecord)
	return nil
}

func (database *fakeManagedTenantDatabase) deleteProviderKey(record managedProviderAPIKeyRecord) error {
	if database.deleteProviderKeyError != nil {
		return database.deleteProviderKeyError
	}
	tenantRecord := cloneManagedTenantRecord(database.records[record.UserID])
	updatedProviderKeys := make([]managedProviderAPIKeyRecord, 0, len(tenantRecord.ProviderAPIKeys))
	for _, existingRecord := range tenantRecord.ProviderAPIKeys {
		if existingRecord.ProviderID != record.ProviderID {
			updatedProviderKeys = append(updatedProviderKeys, existingRecord)
		}
	}
	tenantRecord.ProviderAPIKeys = updatedProviderKeys
	database.records[record.UserID] = cloneManagedTenantRecord(tenantRecord)
	return nil
}

func (database *fakeManagedTenantDatabase) createUsageEvent(record managedUsageEventRecord) error {
	if database.createUsageEventError != nil {
		return database.createUsageEventError
	}
	record.ID = uint(len(database.usageEvents) + 1)
	database.usageEvents = append(database.usageEvents, record)
	return nil
}

func (database *fakeManagedTenantDatabase) usageEventsByUserIDSince(userID string, periodStart time.Time) ([]managedUsageEventRecord, error) {
	if database.usageEventsQueryError != nil {
		return nil, database.usageEventsQueryError
	}
	database.usageEventsQueryPeriodStart = periodStart
	records := make([]managedUsageEventRecord, 0, len(database.usageEvents))
	for _, record := range database.usageEvents {
		if record.UserID == userID && !record.CreatedAt.Before(periodStart) {
			records = append(records, record)
		}
	}
	return records, nil
}

func (database *fakeManagedTenantDatabase) usageEventsSince(periodStart time.Time) ([]managedUsageEventRecord, error) {
	if database.usageEventsQueryError != nil {
		return nil, database.usageEventsQueryError
	}
	database.usageEventsQueryPeriodStart = periodStart
	records := make([]managedUsageEventRecord, 0, len(database.usageEvents))
	for _, record := range database.usageEvents {
		if !record.CreatedAt.Before(periodStart) {
			records = append(records, record)
		}
	}
	return records, nil
}

func (database *fakeManagedTenantDatabase) staticConfigMigrationByID(identifier string) (managedStaticConfigMigrationRecord, error) {
	if queryError, hasQueryError := database.popMigrationQueryError(); hasQueryError {
		return managedStaticConfigMigrationRecord{}, queryError
	}
	record, foundRecord := database.migrations[identifier]
	if !foundRecord {
		return managedStaticConfigMigrationRecord{}, gorm.ErrRecordNotFound
	}
	return record, nil
}

func (database *fakeManagedTenantDatabase) createStaticConfigMigration(record managedStaticConfigMigrationRecord) error {
	if database.createMigrationError != nil {
		return database.createMigrationError
	}
	database.migrations[record.ID] = record
	return nil
}

func (database *fakeManagedTenantDatabase) popUserQueryError() (error, bool) {
	if len(database.userQueryErrors) == 0 {
		return nil, false
	}
	queryError := database.userQueryErrors[0]
	database.userQueryErrors = database.userQueryErrors[1:]
	return queryError, true
}

func (database *fakeManagedTenantDatabase) popSecretQueryError() (error, bool) {
	if len(database.secretQueryErrors) == 0 {
		return nil, false
	}
	queryError := database.secretQueryErrors[0]
	database.secretQueryErrors = database.secretQueryErrors[1:]
	return queryError, true
}

func (database *fakeManagedTenantDatabase) popMigrationQueryError() (error, bool) {
	if len(database.migrationQueryErrors) == 0 {
		return nil, false
	}
	queryError := database.migrationQueryErrors[0]
	database.migrationQueryErrors = database.migrationQueryErrors[1:]
	return queryError, true
}

func (database *fakeManagedTenantDatabase) popSaveTenantError() (error, bool) {
	if len(database.saveTenantErrors) == 0 {
		return nil, false
	}
	saveError := database.saveTenantErrors[0]
	database.saveTenantErrors = database.saveTenantErrors[1:]
	return saveError, true
}

func cloneManagedTenantRecord(record managedTenantRecord) managedTenantRecord {
	record.ProviderAPIKeys = append([]managedProviderAPIKeyRecord(nil), record.ProviderAPIKeys...)
	return record
}

func newInternalManagementService(database *fakeManagedTenantDatabase) *managementService {
	store := newManagedTenantStoreWithDatabase(database)
	return newManagementService(
		ManagementConfiguration{
			PublicOrigin:             "http://localhost:8080",
			UIDescription:            "LLM Proxy",
			UIOrigins:                []string{"http://localhost:8080"},
			TAuthURL:                 "http://localhost:8443",
			TAuthTenantID:            "llm-proxy-test",
			GoogleClientID:           "google-client-id",
			LoginPath:                "/auth/google",
			LogoutPath:               "/auth/logout",
			NoncePath:                "/auth/nonce",
			JWTSigningKey:            "management-signing-key",
			JWTIssuer:                DefaultManagementJWTIssuer,
			SessionCookieName:        "llm_proxy_test_session",
			ProviderKeyEncryptionKey: testManagedProviderKeyEncryptionKey,
			ManagementAPIOrigin:      "http://localhost:8080",
			ProxyOrigin:              "http://localhost:8080",
		},
		store,
		internalManagementProviderRegistry(),
		newTenantAuthenticator(tenantRegistry{}, store),
		zap.NewNop().Sugar(),
	)
}

func internalManagementProviderRegistry() *providerRegistry {
	return newProviderRegistry(Configuration{
		OpenAIKey:               "sk-config-openai",
		OpenAITranscriptionsURL: "https://openai.example/transcriptions",
		ProviderModels: ProviderModelCatalogs{
			ProviderNameOpenAI: {
				Text: ModelEndpointCatalog{
					DefaultModel: ModelNameGPT41,
					Models:       []ModelConfiguration{{ID: ModelNameGPT41}},
				},
				Dictation: ModelEndpointCatalog{
					DefaultModel: DefaultDictationModel,
					Models:       []ModelConfiguration{{ID: DefaultDictationModel}},
				},
			},
		},
	})
}

func internalManagementRouterConfiguration() Configuration {
	return Configuration{
		Management: ManagementConfiguration{
			Enabled:                  true,
			PublicOrigin:             "http://localhost:8080",
			UIDescription:            "LLM Proxy",
			UIOrigins:                []string{"http://localhost:8080"},
			TAuthURL:                 "http://localhost:8443",
			TAuthTenantID:            "llm-proxy-test",
			GoogleClientID:           "google-client-id",
			LoginPath:                "/auth/google",
			LogoutPath:               "/auth/logout",
			NoncePath:                "/auth/nonce",
			JWTSigningKey:            "management-signing-key",
			JWTIssuer:                DefaultManagementJWTIssuer,
			SessionCookieName:        "llm_proxy_test_session",
			DatabaseDialect:          ManagementDatabaseDialectSQLite,
			DatabaseDSN:              "sqlite-test-management",
			ProviderKeyEncryptionKey: testManagedProviderKeyEncryptionKey,
			ManagementAPIOrigin:      "http://localhost:8080",
			ProxyOrigin:              "http://localhost:8080",
		},
		ProviderModels: internalProviderModelCatalogs(),
		LogLevel:       LogLevelInfo,
		WorkerCount:    1,
		QueueSize:      1,
	}
}

func internalProviderModelCatalogs() ProviderModelCatalogs {
	textCatalog := func(modelIdentifier string) ModelEndpointCatalog {
		return ModelEndpointCatalog{
			DefaultModel: modelIdentifier,
			Models:       []ModelConfiguration{{ID: modelIdentifier}},
		}
	}
	openAITextCatalog := ModelEndpointCatalog{
		DefaultModel: ModelNameGPT41,
		Models: []ModelConfiguration{{
			ID:             ModelNameGPT41,
			RequestProfile: string(requestProfileOpenAIResponsesTemperatureTools),
		}},
	}
	openAIDictationCatalog := textCatalog(DefaultDictationModel)
	anthropicTextCatalog := ModelEndpointCatalog{
		DefaultModel: ModelNameClaudeSonnet46,
		Models: []ModelConfiguration{{
			ID:               ModelNameClaudeSonnet46,
			OutputTokenLimit: 64000,
		}},
	}
	return ProviderModelCatalogs{
		ProviderNameOpenAI:      {Text: openAITextCatalog, Dictation: openAIDictationCatalog},
		ProviderNameDeepSeek:    {Text: textCatalog(ModelNameDeepSeekV4Flash)},
		ProviderNameDashScope:   {Text: textCatalog(ModelNameDashScopeQwenPlus)},
		ProviderNameMoonshot:    {Text: textCatalog(ModelNameMoonshotKimi)},
		ProviderNameSiliconFlow: {Text: textCatalog(ModelNameSiliconFlowDeepSeek), Dictation: textCatalog("FunAudioLLM/SenseVoiceSmall")},
		ProviderNameZhipu:       {Text: textCatalog(ModelNameZhipuGLM), Dictation: textCatalog("glm-asr-2512")},
		ProviderNameGemini:      {Text: textCatalog(ModelNameGemini25Flash)},
		ProviderNameAnthropic:   {Text: anthropicTextCatalog},
		ProviderNameMeta:        {Text: textCatalog(ModelNameMuseSpark11)},
		ProviderNameGrok:        {Text: textCatalog(ModelNameGrok43), Dictation: textCatalog("xai-stt")},
	}
}

func executeInternalManagementHandler(handler gin.HandlerFunc, method string, path string, body string, params gin.Params, principal managementPrincipal) int {
	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	ginContext.Params = params
	ginContext.Set(contextKeyManagementPrincipal, principal)
	handler(ginContext)
	return response.Code
}

func internalManagedTenantRecord(userID string, secretDigest string, now time.Time) managedTenantRecord {
	record := managedTenantRecord{
		UserID:       userID,
		TenantID:     managedTenantID(userID),
		SecretDigest: secretDigest,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	record.applyDefaults(DefaultTenantDefaults())
	return record
}

func internalManagedProviderKeyRecord(t *testing.T, userID string, providerIdentifier string, apiKey string, now time.Time) managedProviderAPIKeyRecord {
	t.Helper()
	encryptedAPIKey, encryptionError := internalManagedProviderKeyCipher().encrypt(randReaderForProviderKeyTests(), userID, providerIdentifier, apiKey)
	if encryptionError != nil {
		t.Fatalf("encrypt provider key: %v", encryptionError)
	}
	return managedProviderAPIKeyRecord{
		UserID:          userID,
		ProviderID:      providerIdentifier,
		EncryptedAPIKey: encryptedAPIKey,
		TextModel:       ModelNameGPT41,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func internalBlankManagedProviderKeyRecord(t *testing.T, userID string, providerIdentifier string, now time.Time) managedProviderAPIKeyRecord {
	t.Helper()
	providerKeyCipher := internalManagedProviderKeyCipher()
	nonce := bytes.Repeat([]byte{2}, providerKeyCipher.aeadCipher.NonceSize())
	sealedAPIKey := providerKeyCipher.aeadCipher.Seal(nil, nonce, []byte(" "), managedProviderKeyAssociatedData(userID, providerIdentifier))
	return managedProviderAPIKeyRecord{
		UserID:          userID,
		ProviderID:      providerIdentifier,
		EncryptedAPIKey: managedProviderKeyCiphertextPrefix + base64.StdEncoding.EncodeToString(append(nonce, sealedAPIKey...)),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func randReaderForProviderKeyTests() io.Reader {
	return bytes.NewReader(bytes.Repeat([]byte{1}, 64))
}

type failingReadCloser struct{}

type failingAutoMigrateDialector struct {
	gorm.Dialector
}

func (dialector failingAutoMigrateDialector) Migrator(database *gorm.DB) gorm.Migrator {
	return failingAutoMigrateMigrator{Migrator: dialector.Dialector.Migrator(database)}
}

type failingAutoMigrateMigrator struct {
	gorm.Migrator
}

func (migrator failingAutoMigrateMigrator) AutoMigrate(...interface{}) error {
	return errInternalTestDatabase
}

func (failingReadCloser) Read([]byte) (int, error) {
	return 0, errInternalTestRead
}

func (failingReadCloser) Close() error {
	return nil
}
