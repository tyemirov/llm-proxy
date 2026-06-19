package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var errInternalTestDatabase = errors.New("database failed")
var errInternalTestRead = errors.New("read failed")

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
	if _, authenticated := inMemoryStore.authenticate(" "); authenticated {
		t.Fatalf("blank generated secret authenticated")
	}

	readErrorStore := newManagedTenantStoreWithDatabase(newFakeManagedTenantDatabase())
	readErrorStore.randomReader = strings.NewReader("")
	_, _, generationError := readErrorStore.generateSecret(principal, func([sha256.Size]byte) bool { return false })
	if !errors.Is(generationError, errManagedSecretGeneration) {
		t.Fatalf("generation error=%v want %v", generationError, errManagedSecretGeneration)
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
	if _, saveError := saveProviderKeyErrorStore.saveProviderKey(principal, newProviderID("openai"), "sk-openai"); !errors.Is(saveError, errManagedTenantStorePersist) {
		t.Fatalf("provider key save error=%v want %v", saveError, errManagedTenantStorePersist)
	}

	saveProviderKeyTenantErrorDatabase := newFakeManagedTenantDatabase()
	saveProviderKeyTenantErrorDatabase.records[principal.userID] = internalManagedTenantRecord(principal.userID, "", fixedTime)
	saveProviderKeyTenantErrorDatabase.saveTenantErrors = []error{nil, errInternalTestDatabase}
	saveProviderKeyTenantErrorStore := newManagedTenantStoreWithDatabase(saveProviderKeyTenantErrorDatabase)
	if _, saveError := saveProviderKeyTenantErrorStore.saveProviderKey(principal, newProviderID("openai"), "sk-openai"); !errors.Is(saveError, errManagedTenantStorePersist) {
		t.Fatalf("provider key tenant save error=%v want %v", saveError, errManagedTenantStorePersist)
	}

	saveProviderKeyRecordErrorDatabase := newFakeManagedTenantDatabase()
	saveProviderKeyRecordErrorDatabase.userQueryErrors = []error{errInternalTestDatabase}
	saveProviderKeyRecordErrorStore := newManagedTenantStoreWithDatabase(saveProviderKeyRecordErrorDatabase)
	if _, saveError := saveProviderKeyRecordErrorStore.saveProviderKey(principal, newProviderID("openai"), "sk-openai"); !errors.Is(saveError, errManagedTenantStorePersist) {
		t.Fatalf("provider key record error=%v want %v", saveError, errManagedTenantStorePersist)
	}

	deleteProviderKeyErrorDatabase := newFakeManagedTenantDatabase()
	deleteRecord := internalManagedTenantRecord(principal.userID, "", fixedTime)
	deleteRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{{UserID: principal.userID, ProviderID: "openai", APIKey: "sk-openai"}}
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
	providerAPIKeys := record.providerAPIKeyMap()
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
	removeRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{{UserID: principal.userID, ProviderID: "openai", APIKey: "sk-openai"}}
	removeErrorDatabase.records[principal.userID] = removeRecord
	removeErrorDatabase.saveTenantErrors = []error{nil}
	removeErrorDatabase.deleteProviderKeyError = errInternalTestDatabase
	removeErrorService := newInternalManagementService(removeErrorDatabase)
	if responseCode := executeInternalManagementHandler(removeErrorService.removeProviderKeyHandler(), http.MethodDelete, "/api/management/provider-keys/openai", "", gin.Params{{Key: "provider", Value: "openai"}}, principal); responseCode != http.StatusInternalServerError {
		t.Fatalf("remove error status=%d want=%d", responseCode, http.StatusInternalServerError)
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
	defaultsRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{{UserID: principal.userID, ProviderID: "openai", APIKey: "sk-openai"}}
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
	records                map[string]managedTenantRecord
	migrations             map[string]managedStaticConfigMigrationRecord
	userQueryErrors        []error
	secretQueryErrors      []error
	migrationQueryErrors   []error
	secretQueryRecord      *managedTenantRecord
	createError            error
	saveTenantError        error
	saveTenantErrors       []error
	saveProviderKeyError   error
	deleteProviderKeyError error
	createMigrationError   error
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
			PublicOrigin:      "http://localhost:8080",
			TAuthTenantID:     "llm-proxy-test",
			JWTSigningKey:     "management-signing-key",
			JWTIssuer:         DefaultManagementJWTIssuer,
			SessionCookieName: "llm_proxy_test_session",
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
			Enabled:           true,
			PublicOrigin:      "http://localhost:8080",
			TAuthTenantID:     "llm-proxy-test",
			JWTSigningKey:     "management-signing-key",
			JWTIssuer:         DefaultManagementJWTIssuer,
			SessionCookieName: "llm_proxy_test_session",
			DatabaseDialect:   ManagementDatabaseDialectSQLite,
			DatabaseDSN:       "sqlite-test-management",
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
