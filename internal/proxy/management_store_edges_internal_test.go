package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"gorm.io/gorm"
)

type blockingUsageManagedTenantDatabase struct {
	managedTenantDatabase
	usageStarted chan struct{}
	usageRelease chan struct{}
}

func (database *blockingUsageManagedTenantDatabase) createUsageEvent(requestContext context.Context, record managedUsageEventRecord) error {
	close(database.usageStarted)
	select {
	case <-database.usageRelease:
		return database.managedTenantDatabase.createUsageEvent(requestContext, record)
	case <-requestContext.Done():
		return requestContext.Err()
	}
}

func TestManagedTenantAuthenticationDoesNotWaitForUsagePersistence(t *testing.T) {
	const (
		rawSecret = "llmp_parallel_authentication"
		ownerID   = "parallel-owner"
		tenantID  = "parallel-tenant"
	)
	secretDigest := sha256.Sum256([]byte(rawSecret))
	secretDigestText := hex.EncodeToString(secretDigest[:])
	baseDatabase := newFakeManagedTenantDatabase()
	record := fakeTenantRecord(ownerID, tenantID, "Parallel", time.Now().UTC())
	record.SecretDigest = &secretDigestText
	baseDatabase.tenantsByID[tenantID] = record
	blockingDatabase := &blockingUsageManagedTenantDatabase{
		managedTenantDatabase: baseDatabase,
		usageStarted:          make(chan struct{}),
		usageRelease:          make(chan struct{}),
	}
	store := newManagedTenantStoreWithDatabase(blockingDatabase)
	usageContext, cancelUsage := context.WithCancel(context.Background())
	defer cancelUsage()
	usageDone := make(chan error, 1)
	go func() {
		usageDone <- store.recordUsage(usageContext, tenant{
			identifier: tenantID,
			userID:     ownerID,
			managed:    true,
		}, managedUsageEvent{outcomeCode: managedUsageOutcomeSuccess})
	}()

	select {
	case <-blockingDatabase.usageStarted:
	case <-time.After(time.Second):
		cancelUsage()
		t.Fatal("usage persistence did not start")
	}

	authenticationDone := make(chan bool, 1)
	go func() {
		_, authenticated := store.authenticate(context.Background(), rawSecret)
		authenticationDone <- authenticated
	}()
	select {
	case authenticated := <-authenticationDone:
		if !authenticated {
			cancelUsage()
			t.Fatal("managed tenant did not authenticate")
		}
	case <-time.After(time.Second):
		cancelUsage()
		<-usageDone
		t.Fatal("managed authentication waited for unrelated usage persistence")
	}

	close(blockingDatabase.usageRelease)
	if usageError := <-usageDone; usageError != nil {
		t.Fatalf("usage persistence error=%v", usageError)
	}
}

func TestManagedTenantStoreCipherAndSnapshotEdges(t *testing.T) {
	providers := internalManagementProviderRegistry()
	if _, storeError := newManagedTenantStore(ManagementConfiguration{
		ProviderKeyEncryptionKey: "invalid",
	}, providers); !errors.Is(storeError, errManagedTenantStoreOpen) {
		t.Fatalf("invalid encryption key error=%v", storeError)
	}
	invalidDefaultsProviders := newProviderRegistry(Configuration{
		ProviderModels: ProviderModelCatalogs{
			ProviderNameDeepSeek: {
				Text: ModelEndpointCatalog{
					DefaultModel: ModelNameDeepSeekV4Flash,
					Models:       []ModelConfiguration{{ID: ModelNameDeepSeekV4Flash}},
				},
			},
		},
	})
	if _, storeError := newManagedTenantStore(ManagementConfiguration{
		ProviderKeyEncryptionKey: testManagedProviderKeyEncryptionKey,
		DatabaseDialector:        sqlite.Open(":memory:"),
	}, invalidDefaultsProviders); storeError != nil {
		t.Fatalf("text-only provider catalog store error=%v", storeError)
	}

	cipher := internalManagedProviderKeyCipher()
	if _, encryptionError := cipher.encrypt(bytes.NewReader(nil), "tenant", ProviderNameOpenAI, "sk-key"); !errors.Is(encryptionError, errManagedProviderKeyEncryption) {
		t.Fatalf("short random reader error=%v", encryptionError)
	}
	if _, encryptionError := cipher.encrypt(bytes.NewReader(nil), "tenant", ProviderNameOpenAI, " "); !errors.Is(encryptionError, errManagedProviderKeyInvalid) {
		t.Fatalf("blank provider key error=%v", encryptionError)
	}
	encrypted, encryptionError := cipher.encrypt(bytes.NewReader(bytes.Repeat([]byte{1}, 64)), "tenant", ProviderNameOpenAI, " sk-key ")
	if encryptionError != nil {
		t.Fatalf("encrypt provider key: %v", encryptionError)
	}
	validRecord := managedProviderAPIKeyRecord{
		TenantID:        "tenant",
		ProviderID:      ProviderNameOpenAI,
		EncryptedAPIKey: encrypted,
		TextModel:       " " + ModelNameGPT41 + " ",
		SystemPrompt:    "system",
	}
	if decrypted, decryptError := cipher.decrypt(validRecord); decryptError != nil || decrypted != "sk-key" {
		t.Fatalf("decrypted=%q error=%v", decrypted, decryptError)
	}
	for _, invalidRecord := range []managedProviderAPIKeyRecord{
		{TenantID: "tenant", ProviderID: ProviderNameOpenAI},
		{TenantID: "tenant", ProviderID: ProviderNameOpenAI, EncryptedAPIKey: "plaintext"},
		{TenantID: "tenant", ProviderID: ProviderNameOpenAI, EncryptedAPIKey: managedProviderKeyCiphertextPrefix + "%"},
		{TenantID: "tenant", ProviderID: ProviderNameOpenAI, EncryptedAPIKey: managedProviderKeyCiphertextPrefix + "AQID"},
		{TenantID: "other", ProviderID: ProviderNameOpenAI, EncryptedAPIKey: encrypted},
	} {
		if _, decryptError := cipher.decrypt(invalidRecord); !errors.Is(decryptError, errManagedProviderKeyDecryption) {
			t.Fatalf("record=%+v decrypt error=%v", invalidRecord, decryptError)
		}
	}

	blankNonce := bytes.Repeat([]byte{2}, cipher.aeadCipher.NonceSize())
	blankCiphertext := cipher.aeadCipher.Seal(nil, blankNonce, []byte(" "), managedProviderKeyAssociatedData("tenant", ProviderNameOpenAI))
	blankRecord := managedProviderAPIKeyRecord{
		TenantID:        "tenant",
		ProviderID:      ProviderNameOpenAI,
		EncryptedAPIKey: managedProviderKeyCiphertextPrefix + base64.StdEncoding.EncodeToString(append(blankNonce, blankCiphertext...)),
	}
	store := newManagedTenantStoreWithDatabase(newFakeManagedTenantDatabase())
	settings, settingsError := store.providerSettingsMap([]managedProviderAPIKeyRecord{
		{TenantID: "tenant", ProviderID: "", EncryptedAPIKey: "ignored"},
		blankRecord,
		validRecord,
	})
	if settingsError != nil || settings[newProviderID(ProviderNameOpenAI)].apiKey != "sk-key" || settings[newProviderID(ProviderNameOpenAI)].textModel != ModelNameGPT41 {
		t.Fatalf("settings=%+v error=%v", settings, settingsError)
	}
	if _, settingsError := store.providerSettingsMap([]managedProviderAPIKeyRecord{{
		TenantID: "tenant", ProviderID: ProviderNameOpenAI, EncryptedAPIKey: "invalid",
	}}); !errors.Is(settingsError, errManagedProviderKeyDecryption) {
		t.Fatalf("settings decrypt error=%v", settingsError)
	}

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	brokenRecord := fakeTenantRecord("owner", "tenant", "Default", now)
	brokenRecord.ProviderAPIKeys = []managedProviderAPIKeyRecord{{
		TenantID: "tenant", ProviderID: ProviderNameOpenAI, EncryptedAPIKey: "invalid",
	}}
	if _, snapshotError := store.snapshot(brokenRecord); !errors.Is(snapshotError, errManagedProviderKeyDecryption) {
		t.Fatalf("snapshot error=%v", snapshotError)
	}
	if _, tenantError := store.tenant(brokenRecord, sha256.Sum256([]byte("secret"))); !errors.Is(tenantError, errManagedProviderKeyDecryption) {
		t.Fatalf("tenant provider error=%v", tenantError)
	}
	invalidDefaultsRecord := fakeTenantRecord("owner", "tenant", "Default", now)
	invalidDefaultsRecord.DefaultProvider = "missing"
	store.routingDefaults = providers
	if _, tenantError := store.tenant(invalidDefaultsRecord, sha256.Sum256([]byte("secret"))); !errors.Is(tenantError, errManagedRoutingDefaultsInvalid) {
		t.Fatalf("tenant routing error=%v", tenantError)
	}

	if _, valid := managedRecordSecretDigest(managedTenantRecord{}); valid {
		t.Fatal("nil digest reported valid")
	}
	invalidDigest := "invalid"
	if _, valid := managedRecordSecretDigest(managedTenantRecord{SecretDigest: &invalidDigest}); valid {
		t.Fatal("invalid digest reported valid")
	}
	validDigest := sha256.Sum256([]byte("secret"))
	validDigestText := hex.EncodeToString(validDigest[:])
	if parsed, valid := managedRecordSecretDigest(managedTenantRecord{SecretDigest: &validDigestText}); !valid || parsed != validDigest {
		t.Fatalf("parsed digest=%x valid=%t", parsed, valid)
	}
	if managedSecretDigestValue(nil) != constants.EmptyString || managedSecretDigestValue(&validDigestText) != validDigestText {
		t.Fatal("secret digest value mismatch")
	}
	assertManagedError(t, managedTenantQueryError("owner", "tenant", gorm.ErrRecordNotFound), errManagedTenantNotFound)
	assertManagedError(t, managedTenantQueryError("owner", "tenant", errInternalTestDatabase), errManagedTenantStorePersist)
	assertManagedError(t, managedTenantMutationError("owner", "tenant", gorm.ErrRecordNotFound), errManagedTenantNotFound)
	assertManagedError(t, managedTenantMutationError("owner", "tenant", errInternalTestDatabase), errManagedTenantStorePersist)
	if !errors.Is(managedRoutingDefaultsTenantError("tenant", errManagedRoutingDefaultsInvalid), errManagedRoutingDefaultsInvalid) {
		t.Fatal("routing defaults tenant error lost cause")
	}
}

func TestManagedTenantStoreAccountAndTenantEdges(t *testing.T) {
	now := time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)
	principal := managementPrincipal{
		userID:          "tauth-user",
		userEmail:       "owner@example.com",
		userDisplayName: "Owner",
		userAvatarURL:   "https://example.com/avatar.png",
	}

	t.Run("bootstrap and account ordering", func(subTest *testing.T) {
		database := newFakeManagedTenantDatabase()
		store := newManagedTenantStoreWithDatabase(database)
		store.now = func() time.Time { return now }
		store.randomReader = bytes.NewReader(bytes.Repeat([]byte{1}, generatedTenantIdentifierBytes))
		account, accountError := store.account(principal)
		if accountError != nil || len(account.tenants) != 1 || account.tenants[0].name != "Default" {
			subTest.Fatalf("account=%+v error=%v", account, accountError)
		}
		firstID := account.tenants[0].tenantID
		second := fakeTenantRecord(principal.userID, "managed-b", "Beta", now)
		database.tenantsByID[second.TenantID] = second
		first := database.tenantsByID[firstID]
		first.CreatedAt = now
		database.tenantsByID[firstID] = first
		account, accountError = store.account(managementPrincipal{
			userID: principal.userID, userEmail: "updated@example.com", userDisplayName: "Updated",
		})
		if accountError != nil || len(account.tenants) != 2 || account.tenants[0].tenantID >= account.tenants[1].tenantID || account.userEmail != "updated@example.com" {
			subTest.Fatalf("ordered account=%+v error=%v", account, accountError)
		}
	})

	t.Run("account failures", func(subTest *testing.T) {
		for _, testCase := range []struct {
			name      string
			configure func(*fakeManagedTenantDatabase, *managedTenantStore)
			expected  error
		}{
			{
				name: "query",
				configure: func(database *fakeManagedTenantDatabase, _ *managedTenantStore) {
					database.userByIDErrors = []error{errInternalTestDatabase}
				},
				expected: errManagedTenantStorePersist,
			},
			{
				name: "existing user without tenant",
				configure: func(database *fakeManagedTenantDatabase, _ *managedTenantStore) {
					database.usersByID[principal.userID] = managedUserRecord{UserID: principal.userID}
				},
				expected: errManagedTenantStorePersist,
			},
			{
				name: "save user",
				configure: func(database *fakeManagedTenantDatabase, _ *managedTenantStore) {
					fakeUserWithTenant(database, principal, "managed-default", "Default", now)
					database.saveUserError = errInternalTestDatabase
				},
				expected: errManagedTenantStorePersist,
			},
			{
				name: "identifier random",
				configure: func(_ *fakeManagedTenantDatabase, store *managedTenantStore) {
					store.randomReader = strings.NewReader("")
				},
				expected: errManagedTenantIDGeneration,
			},
			{
				name: "identifier query",
				configure: func(database *fakeManagedTenantDatabase, store *managedTenantStore) {
					store.randomReader = bytes.NewReader(bytes.Repeat([]byte{1}, generatedTenantIdentifierBytes))
					database.tenantIDExistsErrors = []error{errInternalTestDatabase}
				},
				expected: errManagedTenantStorePersist,
			},
			{
				name: "identifier collision",
				configure: func(database *fakeManagedTenantDatabase, store *managedTenantStore) {
					store.randomReader = bytes.NewReader(bytes.Repeat([]byte{1}, generatedTenantIdentifierBytes*generatedTenantIdentifierAttempts))
					database.tenantIDExistsResults = repeatFakeBool(true, generatedTenantIdentifierAttempts)
				},
				expected: errManagedTenantIDCollision,
			},
			{
				name: "create duplicate collision",
				configure: func(database *fakeManagedTenantDatabase, store *managedTenantStore) {
					store.randomReader = bytes.NewReader(bytes.Repeat([]byte{1}, generatedTenantIdentifierBytes*generatedTenantIdentifierAttempts))
					database.tenantIDExistsResults = repeatFakeBool(false, generatedTenantIdentifierAttempts)
					database.createUserAndTenantErrors = repeatFakeError(gorm.ErrDuplicatedKey, generatedTenantIdentifierAttempts)
				},
				expected: errManagedTenantIDCollision,
			},
			{
				name: "create persistence",
				configure: func(database *fakeManagedTenantDatabase, store *managedTenantStore) {
					store.randomReader = bytes.NewReader(bytes.Repeat([]byte{1}, generatedTenantIdentifierBytes))
					database.createUserAndTenantErrors = []error{errInternalTestDatabase}
				},
				expected: errManagedTenantStorePersist,
			},
		} {
			subTest.Run(testCase.name, func(caseTest *testing.T) {
				database := newFakeManagedTenantDatabase()
				store := newManagedTenantStoreWithDatabase(database)
				testCase.configure(database, store)
				_, accountError := store.account(principal)
				assertManagedError(caseTest, accountError, testCase.expected)
			})
		}
	})

	t.Run("tenant profile failures", func(subTest *testing.T) {
		database := newFakeManagedTenantDatabase()
		store := newManagedTenantStoreWithDatabase(database)
		database.userByIDErrors = []error{errInternalTestDatabase}
		if _, profileError := store.tenantProfile(principal, "managed-default"); !errors.Is(profileError, errManagedTenantStorePersist) {
			subTest.Fatalf("ensure profile error=%v", profileError)
		}
		database = newFakeManagedTenantDatabase()
		identifier := fakeUserWithTenant(database, principal, "managed-default", "Default", now)
		database.tenantByOwnerAndIDErrors = []error{errInternalTestDatabase}
		if _, profileError := newManagedTenantStoreWithDatabase(database).tenantProfile(principal, identifier); !errors.Is(profileError, errManagedTenantStorePersist) {
			subTest.Fatalf("query profile error=%v", profileError)
		}
	})

	t.Run("create tenant failures and collision retry", func(subTest *testing.T) {
		name, _ := newManagedTenantName("Project")
		for _, testCase := range []struct {
			name      string
			configure func(*fakeManagedTenantDatabase, *managedTenantStore)
			expected  error
		}{
			{
				name: "ensure",
				configure: func(database *fakeManagedTenantDatabase, _ *managedTenantStore) {
					database.userByIDErrors = []error{errInternalTestDatabase}
				},
				expected: errManagedTenantStorePersist,
			},
			{
				name: "name query",
				configure: func(database *fakeManagedTenantDatabase, _ *managedTenantStore) {
					fakeUserWithTenant(database, principal, "managed-default", "Default", now)
					database.tenantNameExistsErrors = []error{errInternalTestDatabase}
				},
				expected: errManagedTenantStorePersist,
			},
			{
				name: "name conflict",
				configure: func(database *fakeManagedTenantDatabase, _ *managedTenantStore) {
					fakeUserWithTenant(database, principal, "managed-default", "Default", now)
					database.tenantNameExistsResults = []bool{true}
				},
				expected: errManagedTenantNameConflict,
			},
			{
				name: "identifier random",
				configure: func(database *fakeManagedTenantDatabase, store *managedTenantStore) {
					fakeUserWithTenant(database, principal, "managed-default", "Default", now)
					store.randomReader = strings.NewReader("")
				},
				expected: errManagedTenantIDGeneration,
			},
			{
				name: "identifier query",
				configure: func(database *fakeManagedTenantDatabase, store *managedTenantStore) {
					fakeUserWithTenant(database, principal, "managed-default", "Default", now)
					store.randomReader = bytes.NewReader(bytes.Repeat([]byte{2}, generatedTenantIdentifierBytes))
					database.tenantIDExistsErrors = []error{errInternalTestDatabase}
				},
				expected: errManagedTenantStorePersist,
			},
			{
				name: "identifier collision",
				configure: func(database *fakeManagedTenantDatabase, store *managedTenantStore) {
					fakeUserWithTenant(database, principal, "managed-default", "Default", now)
					store.randomReader = bytes.NewReader(bytes.Repeat([]byte{2}, generatedTenantIdentifierBytes*generatedTenantIdentifierAttempts))
					database.tenantIDExistsResults = repeatFakeBool(true, generatedTenantIdentifierAttempts)
				},
				expected: errManagedTenantIDCollision,
			},
			{
				name: "create duplicate collision",
				configure: func(database *fakeManagedTenantDatabase, store *managedTenantStore) {
					fakeUserWithTenant(database, principal, "managed-default", "Default", now)
					store.randomReader = bytes.NewReader(bytes.Repeat([]byte{2}, generatedTenantIdentifierBytes*generatedTenantIdentifierAttempts))
					database.tenantIDExistsResults = repeatFakeBool(false, generatedTenantIdentifierAttempts)
					database.createTenantErrors = repeatFakeError(gorm.ErrDuplicatedKey, generatedTenantIdentifierAttempts)
				},
				expected: errManagedTenantIDCollision,
			},
			{
				name: "create persistence",
				configure: func(database *fakeManagedTenantDatabase, store *managedTenantStore) {
					fakeUserWithTenant(database, principal, "managed-default", "Default", now)
					store.randomReader = bytes.NewReader(bytes.Repeat([]byte{2}, generatedTenantIdentifierBytes))
					database.createTenantErrors = []error{errInternalTestDatabase}
				},
				expected: errManagedTenantStorePersist,
			},
		} {
			subTest.Run(testCase.name, func(caseTest *testing.T) {
				database := newFakeManagedTenantDatabase()
				store := newManagedTenantStoreWithDatabase(database)
				testCase.configure(database, store)
				_, createError := store.createTenant(principal, name)
				assertManagedError(caseTest, createError, testCase.expected)
			})
		}
		database := newFakeManagedTenantDatabase()
		fakeUserWithTenant(database, principal, "managed-default", "Default", now)
		store := newManagedTenantStoreWithDatabase(database)
		store.randomReader = bytes.NewReader(append(bytes.Repeat([]byte{3}, generatedTenantIdentifierBytes), bytes.Repeat([]byte{4}, generatedTenantIdentifierBytes)...))
		database.tenantIDExistsResults = []bool{true, false}
		database.createTenantErrors = []error{nil}
		snapshot, createError := store.createTenant(principal, name)
		if createError != nil || snapshot.tenantName != "Project" {
			subTest.Fatalf("retried snapshot=%+v error=%v", snapshot, createError)
		}
	})

	t.Run("rename and delete failures", func(subTest *testing.T) {
		name, _ := newManagedTenantName("Renamed")
		database := newFakeManagedTenantDatabase()
		identifier := fakeUserWithTenant(database, principal, "managed-default", "Default", now)
		store := newManagedTenantStoreWithDatabase(database)
		database.tenantByOwnerAndIDErrors = []error{errInternalTestDatabase}
		if _, renameError := store.renameTenant(principal, identifier, name); !errors.Is(renameError, errManagedTenantStorePersist) {
			subTest.Fatalf("rename query error=%v", renameError)
		}
		database.tenantNameExistsErrors = []error{errInternalTestDatabase}
		if _, renameError := store.renameTenant(principal, identifier, name); !errors.Is(renameError, errManagedTenantStorePersist) {
			subTest.Fatalf("rename name query error=%v", renameError)
		}
		database.tenantNameExistsResults = []bool{true}
		if _, renameError := store.renameTenant(principal, identifier, name); !errors.Is(renameError, errManagedTenantNameConflict) {
			subTest.Fatalf("rename conflict error=%v", renameError)
		}
		database.saveTenantErrors = []error{gorm.ErrDuplicatedKey}
		if _, renameError := store.renameTenant(principal, identifier, name); !errors.Is(renameError, errManagedTenantNameConflict) {
			subTest.Fatalf("rename duplicate error=%v", renameError)
		}
		database.saveTenantErrors = []error{errInternalTestDatabase}
		if _, renameError := store.renameTenant(principal, identifier, name); !errors.Is(renameError, errManagedTenantStorePersist) {
			subTest.Fatalf("rename persistence error=%v", renameError)
		}
		if renamed, renameError := store.renameTenant(principal, identifier, name); renameError != nil || renamed.tenantName != "Renamed" {
			subTest.Fatalf("renamed=%+v error=%v", renamed, renameError)
		}

		if deleteError := store.deleteTenant(principal, identifier); !errors.Is(deleteError, errManagedFinalTenantDeletion) {
			subTest.Fatalf("final delete error=%v", deleteError)
		}
		database.tenantsByID["managed-second"] = fakeTenantRecord(principal.userID, "managed-second", "Second", now)
		database.deleteTenantErrors = []error{errInternalTestDatabase}
		if deleteError := store.deleteTenant(principal, identifier); !errors.Is(deleteError, errManagedTenantStorePersist) {
			subTest.Fatalf("delete persistence error=%v", deleteError)
		}
		database.deleteTenantErrors = []error{gorm.ErrRecordNotFound}
		if deleteError := store.deleteTenant(principal, identifier); !errors.Is(deleteError, errManagedTenantNotFound) {
			subTest.Fatalf("delete missing error=%v", deleteError)
		}
		if deleteError := store.deleteTenant(principal, identifier); deleteError != nil {
			subTest.Fatalf("delete error=%v", deleteError)
		}
	})
}

func TestManagedTenantStoreProviderSecretUsageAndAdminEdges(t *testing.T) {
	now := time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)
	principal := managementPrincipal{userID: "tauth-user", userEmail: "owner@example.com"}
	database := newFakeManagedTenantDatabase()
	identifier := fakeUserWithTenant(database, principal, "managed-default", "Default", now)
	store := newManagedTenantStoreWithDatabase(database)
	store.routingDefaults = internalManagementProviderRegistry()
	store.now = func() time.Time { return now }

	if _, saveError := store.saveProviderKey(principal, "missing", newProviderID(ProviderNameOpenAI), "sk-key", ModelNameGPT41, ""); !errors.Is(saveError, errManagedTenantNotFound) {
		t.Fatalf("provider tenant error=%v", saveError)
	}
	if _, saveError := store.saveProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI), " ", ModelNameGPT41, ""); !errors.Is(saveError, errManagedProviderKeyInvalid) {
		t.Fatalf("blank provider error=%v", saveError)
	}
	store.randomReader = strings.NewReader("")
	if _, saveError := store.saveProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI), "sk-key", ModelNameGPT41, ""); !errors.Is(saveError, errManagedProviderKeyEncryption) {
		t.Fatalf("provider encryption error=%v", saveError)
	}
	store.randomReader = bytes.NewReader(bytes.Repeat([]byte{1}, 256))
	database.saveProviderKeyErrors = []error{errInternalTestDatabase}
	if _, saveError := store.saveProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI), "sk-key", ModelNameGPT41, ""); !errors.Is(saveError, errManagedTenantStorePersist) {
		t.Fatalf("provider persistence error=%v", saveError)
	}
	snapshot, saveError := store.saveProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI), "sk-key", ModelNameGPT41, "provider system")
	if saveError != nil || snapshot.providerSettings[newProviderID(ProviderNameOpenAI)].apiKey != "sk-key" {
		t.Fatalf("provider snapshot=%+v error=%v", snapshot, saveError)
	}
	originalCiphertext := database.tenantsByID[identifier.string()].ProviderAPIKeys[0].EncryptedAPIKey
	snapshot, saveError = store.saveProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI), "", ModelNameGPT55, "updated system")
	if saveError != nil || snapshot.providerSettings[newProviderID(ProviderNameOpenAI)].textModel != ModelNameGPT55 || database.tenantsByID[identifier.string()].ProviderAPIKeys[0].EncryptedAPIKey != originalCiphertext {
		t.Fatalf("updated snapshot=%+v error=%v", snapshot, saveError)
	}
	database.tenantByOwnerAndIDErrors = []error{nil, errInternalTestDatabase}
	if _, saveError := store.saveProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI), "", ModelNameGPT41, ""); !errors.Is(saveError, errManagedTenantStorePersist) {
		t.Fatalf("provider reload error=%v", saveError)
	}

	if _, revealError := store.revealProviderKey(principal, "missing", newProviderID(ProviderNameOpenAI)); !errors.Is(revealError, errManagedTenantNotFound) {
		t.Fatalf("reveal tenant error=%v", revealError)
	}
	if _, revealError := store.revealProviderKey(principal, identifier, newProviderID(ProviderNameDeepSeek)); !errors.Is(revealError, errManagedProviderKeyNotFound) {
		t.Fatalf("reveal missing key error=%v", revealError)
	}
	record := database.tenantsByID[identifier.string()]
	record.ProviderAPIKeys[0].EncryptedAPIKey = "invalid"
	database.tenantsByID[identifier.string()] = record
	if _, revealError := store.revealProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI)); !errors.Is(revealError, errManagedProviderKeyDecryption) {
		t.Fatalf("reveal decryption error=%v", revealError)
	}
	record.ProviderAPIKeys[0].EncryptedAPIKey = originalCiphertext
	database.tenantsByID[identifier.string()] = record

	database.deleteProviderKeyErrors = []error{errInternalTestDatabase}
	if _, removeError := store.removeProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI)); !errors.Is(removeError, errManagedTenantStorePersist) {
		t.Fatalf("remove persistence error=%v", removeError)
	}
	database.tenantByOwnerAndIDErrors = []error{errInternalTestDatabase}
	if _, removeError := store.removeProviderKey(principal, identifier, newProviderID(ProviderNameDeepSeek)); !errors.Is(removeError, errManagedTenantStorePersist) {
		t.Fatalf("remove reload error=%v", removeError)
	}
	if _, removeError := store.removeProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI)); removeError != nil {
		t.Fatalf("remove provider error=%v", removeError)
	}

	defaults, defaultsError := newManagedRoutingDefaults(internalManagementProviderRegistry(), DefaultTenantDefaults())
	if defaultsError != nil {
		t.Fatalf("defaults fixture: %v", defaultsError)
	}
	database.tenantByOwnerAndIDErrors = []error{errInternalTestDatabase}
	if _, updateError := store.updateDefaults(principal, identifier, defaults); !errors.Is(updateError, errManagedTenantStorePersist) {
		t.Fatalf("defaults query error=%v", updateError)
	}
	database.saveTenantErrors = []error{errInternalTestDatabase}
	if _, updateError := store.updateDefaults(principal, identifier, defaults); !errors.Is(updateError, errManagedTenantStorePersist) {
		t.Fatalf("defaults persistence error=%v", updateError)
	}

	database.tenantByOwnerAndIDErrors = []error{errInternalTestDatabase}
	if _, _, generationError := store.generateSecret(principal, identifier, func([sha256.Size]byte) bool { return false }); !errors.Is(generationError, errManagedTenantStorePersist) {
		t.Fatalf("generate query error=%v", generationError)
	}
	store.randomReader = strings.NewReader("")
	if _, _, generationError := store.generateSecret(principal, identifier, func([sha256.Size]byte) bool { return false }); !errors.Is(generationError, errManagedSecretGeneration) {
		t.Fatalf("generate random error=%v", generationError)
	}
	store.randomReader = bytes.NewReader(bytes.Repeat([]byte{2}, generatedTenantSecretBytes*generatedTenantSecretAttempts))
	if _, _, generationError := store.generateSecret(principal, identifier, func([sha256.Size]byte) bool { return true }); !errors.Is(generationError, errManagedSecretCollision) {
		t.Fatalf("static collision error=%v", generationError)
	}
	store.randomReader = bytes.NewReader(bytes.Repeat([]byte{3}, generatedTenantSecretBytes*generatedTenantSecretAttempts))
	firstRawSecret := generatedTenantSecretPrefix + hex.EncodeToString(bytes.Repeat([]byte{3}, generatedTenantSecretBytes))
	firstDigest := sha256.Sum256([]byte(firstRawSecret))
	firstDigestText := hex.EncodeToString(firstDigest[:])
	otherRecord := fakeTenantRecord("other", "managed-other", "Other", now)
	otherRecord.SecretDigest = &firstDigestText
	database.tenantsByID[otherRecord.TenantID] = otherRecord
	if _, _, generationError := store.generateSecret(principal, identifier, func([sha256.Size]byte) bool { return false }); !errors.Is(generationError, errManagedSecretCollision) {
		t.Fatalf("database collision error=%v", generationError)
	}
	delete(database.tenantsByID, otherRecord.TenantID)
	store.randomReader = bytes.NewReader(bytes.Repeat([]byte{4}, generatedTenantSecretBytes*generatedTenantSecretAttempts))
	database.saveTenantErrors = repeatFakeError(gorm.ErrDuplicatedKey, generatedTenantSecretAttempts)
	if _, _, generationError := store.generateSecret(principal, identifier, func([sha256.Size]byte) bool { return false }); !errors.Is(generationError, errManagedSecretCollision) {
		t.Fatalf("save collision error=%v", generationError)
	}
	store.randomReader = bytes.NewReader(bytes.Repeat([]byte{5}, generatedTenantSecretBytes))
	database.saveTenantErrors = []error{errInternalTestDatabase}
	if _, _, generationError := store.generateSecret(principal, identifier, func([sha256.Size]byte) bool { return false }); !errors.Is(generationError, errManagedTenantStorePersist) {
		t.Fatalf("generate persistence error=%v", generationError)
	}
	store.randomReader = bytes.NewReader(bytes.Repeat([]byte{6}, generatedTenantSecretBytes))
	database.tenantByOwnerAndIDErrors = []error{nil, errInternalTestDatabase}
	if _, _, generationError := store.generateSecret(principal, identifier, func([sha256.Size]byte) bool { return false }); !errors.Is(generationError, errManagedTenantStorePersist) {
		t.Fatalf("generate reload error=%v", generationError)
	}
	store.randomReader = bytes.NewReader(bytes.Repeat([]byte{7}, generatedTenantSecretBytes))
	rawSecret, secretSnapshot, generationError := store.generateSecret(principal, identifier, func([sha256.Size]byte) bool { return false })
	if generationError != nil || rawSecret == "" || !secretSnapshot.hasSecret {
		t.Fatalf("generated secret=%q snapshot=%+v error=%v", rawSecret, secretSnapshot, generationError)
	}
	if authenticatedTenant, authenticated := store.authenticate(context.Background(), rawSecret); !authenticated || authenticatedTenant.identifier.string() != identifier.string() {
		t.Fatalf("authenticated=%t tenant=%+v", authenticated, authenticatedTenant)
	}
	if _, authenticated := store.authenticate(context.Background(), " "); authenticated {
		t.Fatal("blank secret authenticated")
	}
	if _, authenticated := store.authenticate(context.Background(), "missing"); authenticated {
		t.Fatal("missing secret authenticated")
	}
	record = database.tenantsByID[identifier.string()]
	invalidDigest := "invalid"
	record.SecretDigest = &invalidDigest
	database.tenantsByID[identifier.string()] = record
	database.tenantBySecretDigestErrors = []error{nil}
	if _, authenticated := store.authenticate(context.Background(), rawSecret); authenticated {
		t.Fatal("invalid stored digest authenticated")
	}
	digest := sha256.Sum256([]byte(rawSecret))
	wrongDigest := sha256.Sum256([]byte("wrong"))
	wrongDigestText := hex.EncodeToString(wrongDigest[:])
	record.SecretDigest = &wrongDigestText
	database.tenantsByID[identifier.string()] = record
	database.tenantBySecretDigestRecord = &record
	if _, authenticated := store.authenticate(context.Background(), rawSecret); authenticated {
		t.Fatal("mismatched digest authenticated")
	}
	database.tenantBySecretDigestRecord = nil
	digestText := hex.EncodeToString(digest[:])
	record.SecretDigest = &digestText
	record.ProviderAPIKeys = []managedProviderAPIKeyRecord{{
		TenantID: identifier.string(), ProviderID: ProviderNameOpenAI, EncryptedAPIKey: "invalid",
	}}
	database.tenantsByID[identifier.string()] = record
	if _, authenticated := store.authenticate(context.Background(), rawSecret); authenticated {
		t.Fatal("tenant with invalid provider key authenticated")
	}
	record.ProviderAPIKeys = nil
	database.tenantsByID[identifier.string()] = record

	unmanagedTenant := tenant{}
	if usageError := store.recordUsage(context.Background(), unmanagedTenant, managedUsageEvent{}); usageError != nil {
		t.Fatalf("unmanaged usage error=%v", usageError)
	}
	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()
	managedTenant := tenant{identifier: tenantID(identifier.string()), userID: principal.userID, managed: true}
	if usageError := store.recordUsage(cancelledContext, managedTenant, managedUsageEvent{outcomeCode: managedUsageOutcomeSuccess}); !errors.Is(usageError, context.Canceled) || !errors.Is(usageError, errManagedTenantStorePersist) {
		t.Fatalf("cancelled usage error=%v", usageError)
	}
	database.createUsageEventError = errInternalTestDatabase
	if usageError := store.recordUsage(context.Background(), managedTenant, managedUsageEvent{outcomeCode: managedUsageOutcomeSuccess}); !errors.Is(usageError, errManagedTenantStorePersist) {
		t.Fatalf("persist usage error=%v", usageError)
	}
	database.createUsageEventError = nil
	if usageError := store.recordUsage(context.Background(), managedTenant, managedUsageEvent{
		endpoint: usageEndpointText, providerIdentifier: ProviderNameOpenAI, modelIdentifier: ModelNameGPT41,
		statusCode: http.StatusOK, outcomeCode: managedUsageOutcomeSuccess, latencyMilliseconds: 12, usage: &tokenUsage{RequestTokens: 2, ResponseTokens: 3, TotalTokens: 5},
	}); usageError != nil {
		t.Fatalf("record usage error=%v", usageError)
	}

	database.userByIDErrors = []error{errInternalTestDatabase}
	if _, summaryError := store.usageSummary(principal, identifier, usageIntervalThirtyDay); !errors.Is(summaryError, errManagedTenantStorePersist) {
		t.Fatalf("usage account error=%v", summaryError)
	}
	database.tenantByOwnerAndIDErrors = []error{errInternalTestDatabase}
	if _, summaryError := store.usageSummary(principal, identifier, usageIntervalThirtyDay); !errors.Is(summaryError, errManagedTenantStorePersist) {
		t.Fatalf("usage tenant error=%v", summaryError)
	}
	database.earliestUsageEventError = errInternalTestDatabase
	if _, summaryError := store.usageSummary(principal, identifier, usageIntervalAll); !errors.Is(summaryError, errManagedTenantStorePersist) {
		t.Fatalf("usage earliest error=%v", summaryError)
	}
	database.earliestUsageEventError = nil
	database.streamUsageEventsError = errInternalTestDatabase
	if _, summaryError := store.usageSummary(principal, identifier, usageIntervalThirtyDay); !errors.Is(summaryError, errManagedTenantStorePersist) {
		t.Fatalf("finite stream error=%v", summaryError)
	}
	if _, summaryError := store.usageSummary(principal, identifier, usageIntervalAll); !errors.Is(summaryError, errManagedTenantStorePersist) {
		t.Fatalf("all stream error=%v", summaryError)
	}
	database.streamUsageEventsError = nil
	if summary, summaryError := store.usageSummary(principal, identifier, usageIntervalThirtyDay); summaryError != nil || summary.totals.requests != 1 {
		t.Fatalf("usage summary=%+v error=%v", summary, summaryError)
	}
	database.userByIDErrors = []error{errInternalTestDatabase}
	if _, summaryError := store.accountUsageSummary(principal, usageIntervalThirtyDay); !errors.Is(summaryError, errManagedTenantStorePersist) {
		t.Fatalf("account usage summary error=%v", summaryError)
	}
	accountFailureQuery := managedUsageFailureQuery{
		interval: usageIntervalThirtyDay,
		scope:    managedUsageAllTenantsScope,
		limit:    managedUsageFailureDefaultLimit,
	}
	database.userByIDErrors = []error{errInternalTestDatabase}
	if _, failuresError := store.accountUsageFailures(principal, accountFailureQuery); !errors.Is(failuresError, errManagedTenantStorePersist) {
		t.Fatalf("account usage failures account error=%v", failuresError)
	}
	database.usageFailuresError = errInternalTestDatabase
	if _, failuresError := store.accountUsageFailures(principal, accountFailureQuery); !errors.Is(failuresError, errManagedTenantStorePersist) {
		t.Fatalf("account usage failures error=%v", failuresError)
	}
	database.usageFailuresError = nil

	database.usersError = errInternalTestDatabase
	if _, adminError := store.adminUsersSummary(); !errors.Is(adminError, errManagedTenantStorePersist) {
		t.Fatalf("admin users error=%v", adminError)
	}
	database.usersError = nil
	database.usageEventsSinceError = errInternalTestDatabase
	if _, adminError := store.adminUsersSummary(); !errors.Is(adminError, errManagedTenantStorePersist) {
		t.Fatalf("admin usage error=%v", adminError)
	}
	database.usageEventsSinceError = nil
	secondPrincipal := managementPrincipal{userID: "tauth-second", userEmail: principal.userEmail}
	fakeUserWithTenant(database, secondPrincipal, "managed-second", "Second", now)
	adminSnapshots, adminError := store.adminUsersSummary()
	if adminError != nil || len(adminSnapshots) != 2 || adminSnapshots[0].userID != "tauth-second" {
		t.Fatalf("admin snapshots=%+v error=%v", adminSnapshots, adminError)
	}
}

func TestManagedTenantStoreProviderRoutingReconciliationErrors(t *testing.T) {
	now := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	principal := managementPrincipal{userID: "routing-owner", userEmail: "routing@example.com"}
	database := newFakeManagedTenantDatabase()
	identifier := fakeUserWithTenant(database, principal, "routing-default", "Default", now)
	store := newManagedTenantStoreWithDatabase(database)
	store.routingDefaults = internalManagementProviderRegistry()
	store.randomReader = bytes.NewReader(bytes.Repeat([]byte{4}, 512))
	store.now = func() time.Time { return now }

	record := database.tenantsByID[identifier.string()]
	record.ProviderAPIKeys = []managedProviderAPIKeyRecord{{
		TenantID:        record.TenantID,
		ProviderID:      ProviderNameOpenAI,
		EncryptedAPIKey: "invalid",
		TextModel:       ModelNameGPT41,
	}}
	database.tenantsByID[identifier.string()] = record
	if _, saveError := store.saveProviderKey(principal, identifier, newProviderID(ProviderNameDeepSeek), "sk-deepseek", ModelNameDeepSeekV4Flash, ""); !errors.Is(saveError, errManagedProviderKeyDecryption) {
		t.Fatalf("save provider decryption error=%v", saveError)
	}
	if _, removeError := store.removeProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI)); !errors.Is(removeError, errManagedProviderKeyDecryption) {
		t.Fatalf("remove provider decryption error=%v", removeError)
	}

	record.ProviderAPIKeys = nil
	record.DefaultProvider = "missing"
	record.DefaultModel = ""
	database.tenantsByID[identifier.string()] = record
	if _, saveError := store.saveProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI), "sk-openai", ModelNameGPT41, ""); !errors.Is(saveError, errManagedRoutingDefaultsInvalid) {
		t.Fatalf("save invalid defaults error=%v", saveError)
	}
	if _, removeError := store.removeProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI)); !errors.Is(removeError, errManagedRoutingDefaultsInvalid) {
		t.Fatalf("remove invalid defaults error=%v", removeError)
	}

	record.DefaultProvider = ""
	encryptedUnknownKey, encryptionError := store.providerKeyCipher.encrypt(
		bytes.NewReader(bytes.Repeat([]byte{5}, store.providerKeyCipher.aeadCipher.NonceSize())),
		record.TenantID,
		"missing",
		"sk-missing",
	)
	if encryptionError != nil {
		t.Fatalf("encrypt unknown provider: %v", encryptionError)
	}
	record.ProviderAPIKeys = []managedProviderAPIKeyRecord{{
		TenantID:        record.TenantID,
		ProviderID:      "missing",
		EncryptedAPIKey: encryptedUnknownKey,
		TextModel:       "missing-model",
	}}
	database.tenantsByID[identifier.string()] = record
	if _, saveError := store.saveProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI), "sk-openai", ModelNameGPT41, ""); !errors.Is(saveError, errManagedRoutingDefaultsInvalid) {
		t.Fatalf("save reconciliation error=%v", saveError)
	}
	if _, removeError := store.removeProviderKey(principal, identifier, newProviderID(ProviderNameOpenAI)); !errors.Is(removeError, errManagedRoutingDefaultsInvalid) {
		t.Fatalf("remove reconciliation error=%v", removeError)
	}
}

func repeatFakeBool(value bool, count int) []bool {
	values := make([]bool, count)
	for index := range values {
		values[index] = value
	}
	return values
}

func repeatFakeError(value error, count int) []error {
	values := make([]error, count)
	for index := range values {
		values[index] = value
	}
	return values
}
