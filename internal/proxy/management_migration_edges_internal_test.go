package proxy

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestManagedTenantSQLiteMigrationPreflightRejectsMalformedOwnershipData(t *testing.T) {
	cipher := internalManagedProviderKeyCipher()
	providers := internalManagementProviderRegistry()
	now := time.Date(2026, 7, 25, 17, 0, 0, 0, time.UTC)

	type fixture struct {
		database *gorm.DB
		tenant   legacyManagedTenantRecord
	}
	newFixture := func(subTest *testing.T) fixture {
		database := openLegacyManagedTenantDatabase(subTest, filepath.Join(subTest.TempDir(), "preflight.db"))
		tenantRecord := legacyManagedTenantRecord{
			UserID: "owner", UserEmail: "owner@example.com", TenantID: "managed-default", CreatedAt: now, UpdatedAt: now,
		}
		tenantRecord.applyDefaults(defaultManagedRoutingDefaults())
		if createError := database.Table(managedTenantTable).Create(&tenantRecord).Error; createError != nil {
			subTest.Fatalf("seed legacy tenant: %v", createError)
		}
		return fixture{database: database, tenant: tenantRecord}
	}
	addProvider := func(subTest *testing.T, testFixture fixture, mutate func(*legacyManagedProviderAPIKeyRecord)) {
		encryptedKey, encryptionError := cipher.encrypt(
			bytes.NewReader(bytes.Repeat([]byte{1}, cipher.aeadCipher.NonceSize())),
			testFixture.tenant.UserID,
			ProviderNameOpenAI,
			"sk-key",
		)
		if encryptionError != nil {
			subTest.Fatalf("encrypt provider fixture: %v", encryptionError)
		}
		record := legacyManagedProviderAPIKeyRecord{
			UserID: testFixture.tenant.UserID, ProviderID: ProviderNameOpenAI, EncryptedAPIKey: encryptedKey,
			TextModel: ModelNameGPT41, CreatedAt: now, UpdatedAt: now,
		}
		if mutate != nil {
			mutate(&record)
		}
		if createError := testFixture.database.Table(managedProviderKeyTable).Create(&record).Error; createError != nil {
			subTest.Fatalf("seed provider fixture: %v", createError)
		}
	}

	testCases := []struct {
		name      string
		configure func(*testing.T, fixture)
		want      string
	}{
		{
			name: "missing provider table",
			configure: func(subTest *testing.T, testFixture fixture) {
				if dropError := testFixture.database.Migrator().DropTable(managedProviderKeyTable); dropError != nil {
					subTest.Fatalf("drop provider table: %v", dropError)
				}
			},
			want: "table=" + managedProviderKeyTable + " missing",
		},
		{
			name: "missing usage table",
			configure: func(subTest *testing.T, testFixture fixture) {
				if dropError := testFixture.database.Migrator().DropTable(managedUsageEventTable); dropError != nil {
					subTest.Fatalf("drop usage table: %v", dropError)
				}
			},
			want: "table=" + managedUsageEventTable + " missing",
		},
		{
			name: "tenant query error",
			configure: func(subTest *testing.T, testFixture fixture) {
				registerManagedGORMError(subTest, testFixture.database, "preflight_tenant_query", "query", managedTenantTable, errInternalTestDatabase)
			},
			want: errInternalTestDatabase.Error(),
		},
		{
			name: "provider query error",
			configure: func(subTest *testing.T, testFixture fixture) {
				registerManagedGORMError(subTest, testFixture.database, "preflight_provider_query", "query", managedProviderKeyTable, errInternalTestDatabase)
			},
			want: errInternalTestDatabase.Error(),
		},
		{
			name: "usage query error",
			configure: func(subTest *testing.T, testFixture fixture) {
				registerManagedGORMError(subTest, testFixture.database, "preflight_usage_query", "query", managedUsageEventTable, errInternalTestDatabase)
			},
			want: errInternalTestDatabase.Error(),
		},
		{
			name: "missing owner",
			configure: func(subTest *testing.T, testFixture fixture) {
				if updateError := testFixture.database.Table(managedTenantTable).Where("user_id = ?", testFixture.tenant.UserID).Update("user_id", "").Error; updateError != nil {
					subTest.Fatalf("blank owner: %v", updateError)
				}
			},
			want: "user=\"\"",
		},
		{
			name: "missing tenant",
			configure: func(subTest *testing.T, testFixture fixture) {
				if updateError := testFixture.database.Table(managedTenantTable).Where("user_id = ?", testFixture.tenant.UserID).Update("tenant_id", "").Error; updateError != nil {
					subTest.Fatalf("blank tenant: %v", updateError)
				}
			},
			want: "tenant=\"\"",
		},
		{
			name: "invalid tenant identifier",
			configure: func(subTest *testing.T, testFixture fixture) {
				if updateError := testFixture.database.Table(managedTenantTable).Where("user_id = ?", testFixture.tenant.UserID).Update("tenant_id", "invalid/id").Error; updateError != nil {
					subTest.Fatalf("write invalid tenant: %v", updateError)
				}
			},
			want: errManagedTenantIDInvalid.Error(),
		},
		{
			name: "invalid defaults",
			configure: func(subTest *testing.T, testFixture fixture) {
				if updateError := testFixture.database.Table(managedTenantTable).Where("user_id = ?", testFixture.tenant.UserID).Update("default_provider", "missing").Error; updateError != nil {
					subTest.Fatalf("write invalid defaults: %v", updateError)
				}
			},
			want: errManagedRoutingDefaultsInvalid.Error(),
		},
		{
			name: "duplicate secret",
			configure: func(subTest *testing.T, testFixture fixture) {
				digest := sha256.Sum256([]byte("secret"))
				digestText := hex.EncodeToString(digest[:])
				if updateError := testFixture.database.Table(managedTenantTable).Where("user_id = ?", testFixture.tenant.UserID).Update("secret_digest", digestText).Error; updateError != nil {
					subTest.Fatalf("write first secret: %v", updateError)
				}
				second := testFixture.tenant
				second.UserID = "other"
				second.TenantID = "managed-other"
				second.SecretDigest = digestText
				if createError := testFixture.database.Table(managedTenantTable).Create(&second).Error; createError != nil {
					subTest.Fatalf("seed duplicate secret: %v", createError)
				}
			},
			want: "duplicate_secret_digest",
		},
		{
			name: "invalid secret",
			configure: func(subTest *testing.T, testFixture fixture) {
				if updateError := testFixture.database.Table(managedTenantTable).Where("user_id = ?", testFixture.tenant.UserID).Update("secret_digest", "invalid").Error; updateError != nil {
					subTest.Fatalf("write invalid secret: %v", updateError)
				}
			},
			want: "invalid_secret_digest",
		},
		{
			name: "orphan provider",
			configure: func(subTest *testing.T, testFixture fixture) {
				addProvider(subTest, testFixture, func(record *legacyManagedProviderAPIKeyRecord) {
					record.UserID = "missing-owner"
				})
			},
			want: "orphan_user=missing-owner",
		},
		{
			name: "unknown provider",
			configure: func(subTest *testing.T, testFixture fixture) {
				addProvider(subTest, testFixture, func(record *legacyManagedProviderAPIKeyRecord) {
					record.ProviderID = "missing"
				})
			},
			want: "provider=missing",
		},
		{
			name: "unknown model",
			configure: func(subTest *testing.T, testFixture fixture) {
				addProvider(subTest, testFixture, func(record *legacyManagedProviderAPIKeyRecord) {
					record.TextModel = "missing-model"
				})
			},
			want: "missing-model",
		},
		{
			name: "plaintext provider key",
			configure: func(subTest *testing.T, testFixture fixture) {
				addProvider(subTest, testFixture, func(record *legacyManagedProviderAPIKeyRecord) {
					record.APIKey = "sk-plaintext"
				})
			},
			want: "plaintext_key",
		},
		{
			name: "provider re-encryption",
			configure: func(subTest *testing.T, testFixture fixture) {
				addProvider(subTest, testFixture, nil)
				originalRandomReader := cryptorand.Reader
				cryptorand.Reader = strings.NewReader("")
				subTest.Cleanup(func() { cryptorand.Reader = originalRandomReader })
			},
			want: errManagedProviderKeyEncryption.Error(),
		},
		{
			name: "orphan usage owner",
			configure: func(subTest *testing.T, testFixture fixture) {
				if createError := testFixture.database.Table(managedUsageEventTable).Create(&legacyManagedUsageEventRecord{
					UserID: "missing-owner", TenantID: testFixture.tenant.TenantID, CreatedAt: now,
				}).Error; createError != nil {
					subTest.Fatalf("seed orphan usage: %v", createError)
				}
			},
			want: "table=" + managedUsageEventTable,
		},
		{
			name: "usage tenant mismatch",
			configure: func(subTest *testing.T, testFixture fixture) {
				if createError := testFixture.database.Table(managedUsageEventTable).Create(&legacyManagedUsageEventRecord{
					UserID: testFixture.tenant.UserID, TenantID: "managed-other", CreatedAt: now,
				}).Error; createError != nil {
					subTest.Fatalf("seed mismatched usage: %v", createError)
				}
			},
			want: "tenant=managed-other",
		},
		{
			name: "unsupported usage status",
			configure: func(subTest *testing.T, testFixture fixture) {
				if createError := testFixture.database.Table(managedUsageEventTable).Create(&legacyManagedUsageEventRecord{
					UserID: testFixture.tenant.UserID, TenantID: testFixture.tenant.TenantID,
					StatusCode: http.StatusTeapot, CreatedAt: now,
				}).Error; createError != nil {
					subTest.Fatalf("seed unsupported usage: %v", createError)
				}
			},
			want: "status_code=418",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			testFixture := newFixture(subTest)
			testCase.configure(subTest, testFixture)
			_, preflightError := preflightLegacyManagedTenantSchema(testFixture.database, cipher, providers)
			if preflightError == nil || !strings.Contains(preflightError.Error(), testCase.want) {
				subTest.Fatalf("preflight error=%v want fragment=%q", preflightError, testCase.want)
			}
		})
	}

	t.Run("duplicate tenant", func(subTest *testing.T) {
		database := openUnconstrainedLegacyTenantDatabase(subTest, filepath.Join(subTest.TempDir(), "duplicate-tenant.db"))
		for _, ownerUserID := range []string{"first-owner", "second-owner"} {
			record := legacyManagedTenantRecord{
				UserID: ownerUserID, TenantID: "managed-default", CreatedAt: now, UpdatedAt: now,
			}
			record.applyDefaults(defaultManagedRoutingDefaults())
			if createError := database.Table(managedTenantTable).Create(&record).Error; createError != nil {
				subTest.Fatalf("seed duplicate tenant: %v", createError)
			}
		}
		_, preflightError := preflightLegacyManagedTenantSchema(database, cipher, providers)
		if preflightError == nil || !strings.Contains(preflightError.Error(), "duplicate_tenant=managed-default") {
			subTest.Fatalf("duplicate tenant error=%v", preflightError)
		}
	})

	t.Run("duplicate owner", func(subTest *testing.T) {
		database := openUnconstrainedLegacyTenantDatabase(subTest, filepath.Join(subTest.TempDir(), "duplicate-owner.db"))
		for _, tenantIDValue := range []string{"managed-first", "managed-second"} {
			record := legacyManagedTenantRecord{
				UserID: "duplicate-owner", TenantID: tenantIDValue, CreatedAt: now, UpdatedAt: now,
			}
			record.applyDefaults(defaultManagedRoutingDefaults())
			if createError := database.Table(managedTenantTable).Create(&record).Error; createError != nil {
				subTest.Fatalf("seed duplicate owner: %v", createError)
			}
		}
		_, preflightError := preflightLegacyManagedTenantSchema(database, cipher, providers)
		if preflightError == nil || !strings.Contains(preflightError.Error(), "duplicate_user=duplicate-owner") {
			subTest.Fatalf("duplicate owner error=%v", preflightError)
		}
	})
}

func TestVerifyManagedTenantMigrationRejectsMismatches(t *testing.T) {
	cipher := internalManagedProviderKeyCipher()
	now := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
	baseRecords := func() (managedUserRecord, managedTenantRecord) {
		user := managedUserRecord{UserID: "owner", UserEmail: "owner@example.com", CreatedAt: now, UpdatedAt: now}
		tenantRecord := fakeTenantRecord("owner", "managed-default", "Default", now)
		return user, tenantRecord
	}
	newDatabase := func(subTest *testing.T) *gorm.DB {
		database, openError := gorm.Open(sqlite.Open(filepath.Join(subTest.TempDir(), "verify.db")), &gorm.Config{})
		if openError != nil {
			subTest.Fatalf("open verify database: %v", openError)
		}
		if migrationError := migrateCurrentManagedSchema(database); migrationError != nil {
			subTest.Fatalf("create verify schema: %v", migrationError)
		}
		return database
	}

	t.Run("count mismatch", func(subTest *testing.T) {
		database := newDatabase(subTest)
		user, _ := baseRecords()
		dataset := managedTenantMigrationDataset{users: []managedUserRecord{user}}
		if verifyError := verifyManagedTenantMigration(database, cipher, dataset); !errors.Is(verifyError, errManagedTenantSchemaMigration) {
			subTest.Fatalf("count mismatch error=%v", verifyError)
		}
	})
	t.Run("tenant query", func(subTest *testing.T) {
		database := newDatabase(subTest)
		user, tenantRecord := baseRecords()
		if createError := database.Create(&user).Error; createError != nil {
			subTest.Fatalf("create user: %v", createError)
		}
		if createError := database.Create(&tenantRecord).Error; createError != nil {
			subTest.Fatalf("create tenant: %v", createError)
		}
		expectedTenant := tenantRecord
		expectedTenant.OwnerUserID = "other"
		dataset := managedTenantMigrationDataset{users: []managedUserRecord{user}, tenants: []managedTenantRecord{expectedTenant}}
		if verifyError := verifyManagedTenantMigration(database, cipher, dataset); verifyError == nil || !strings.Contains(verifyError.Error(), "table="+managedTenantTable) {
			subTest.Fatalf("tenant query error=%v", verifyError)
		}
	})
	t.Run("tenant values", func(subTest *testing.T) {
		database := newDatabase(subTest)
		user, tenantRecord := baseRecords()
		if createError := database.Create(&user).Error; createError != nil {
			subTest.Fatalf("create user: %v", createError)
		}
		if createError := database.Create(&tenantRecord).Error; createError != nil {
			subTest.Fatalf("create tenant: %v", createError)
		}
		expectedTenant := tenantRecord
		expectedTenant.Name = "Different"
		dataset := managedTenantMigrationDataset{users: []managedUserRecord{user}, tenants: []managedTenantRecord{expectedTenant}}
		if verifyError := verifyManagedTenantMigration(database, cipher, dataset); verifyError == nil || !strings.Contains(verifyError.Error(), "values") {
			subTest.Fatalf("tenant values error=%v", verifyError)
		}
	})
	t.Run("provider query", func(subTest *testing.T) {
		database := newDatabase(subTest)
		user, tenantRecord := baseRecords()
		if createError := database.Create(&user).Error; createError != nil {
			subTest.Fatalf("create user: %v", createError)
		}
		if createError := database.Create(&tenantRecord).Error; createError != nil {
			subTest.Fatalf("create tenant: %v", createError)
		}
		actualProvider := managedProviderAPIKeyRecord{TenantID: tenantRecord.TenantID, ProviderID: ProviderNameOpenAI, EncryptedAPIKey: "cipher", CreatedAt: now, UpdatedAt: now}
		if createError := database.Create(&actualProvider).Error; createError != nil {
			subTest.Fatalf("create provider: %v", createError)
		}
		expectedProvider := actualProvider
		expectedProvider.ProviderID = ProviderNameDeepSeek
		dataset := managedTenantMigrationDataset{
			users: []managedUserRecord{user}, tenants: []managedTenantRecord{tenantRecord},
			providerKeys: []managedProviderAPIKeyRecord{expectedProvider},
		}
		if verifyError := verifyManagedTenantMigration(database, cipher, dataset); verifyError == nil || !strings.Contains(verifyError.Error(), "provider="+ProviderNameDeepSeek) {
			subTest.Fatalf("provider query error=%v", verifyError)
		}
	})
	t.Run("provider values", func(subTest *testing.T) {
		database := newDatabase(subTest)
		user, tenantRecord := baseRecords()
		if createError := database.Create(&user).Error; createError != nil {
			subTest.Fatalf("create user: %v", createError)
		}
		if createError := database.Create(&tenantRecord).Error; createError != nil {
			subTest.Fatalf("create tenant: %v", createError)
		}
		encryptedKey, encryptionError := cipher.encrypt(bytes.NewReader(bytes.Repeat([]byte{1}, cipher.aeadCipher.NonceSize())), tenantRecord.TenantID, ProviderNameOpenAI, "sk-actual")
		if encryptionError != nil {
			subTest.Fatalf("encrypt provider: %v", encryptionError)
		}
		actualProvider := managedProviderAPIKeyRecord{
			TenantID: tenantRecord.TenantID, ProviderID: ProviderNameOpenAI, EncryptedAPIKey: encryptedKey, TextModel: ModelNameGPT41, CreatedAt: now, UpdatedAt: now,
		}
		if createError := database.Create(&actualProvider).Error; createError != nil {
			subTest.Fatalf("create provider: %v", createError)
		}
		dataset := managedTenantMigrationDataset{
			users: []managedUserRecord{user}, tenants: []managedTenantRecord{tenantRecord},
			providerKeys:     []managedProviderAPIKeyRecord{actualProvider},
			decryptedAPIKeys: map[string]string{tenantRecord.TenantID + "\x00" + ProviderNameOpenAI: "sk-expected"},
		}
		if verifyError := verifyManagedTenantMigration(database, cipher, dataset); verifyError == nil || !strings.Contains(verifyError.Error(), "table="+managedProviderKeyTable) {
			subTest.Fatalf("provider values error=%v", verifyError)
		}
	})
	t.Run("usage query", func(subTest *testing.T) {
		database := newDatabase(subTest)
		user, tenantRecord := baseRecords()
		if createError := database.Create(&user).Error; createError != nil {
			subTest.Fatalf("create user: %v", createError)
		}
		if createError := database.Create(&tenantRecord).Error; createError != nil {
			subTest.Fatalf("create tenant: %v", createError)
		}
		usageRecord := managedUsageEventRecord{ID: 1, TenantID: tenantRecord.TenantID, CreatedAt: now}
		if createError := database.Create(&usageRecord).Error; createError != nil {
			subTest.Fatalf("create usage: %v", createError)
		}
		queryCount := 0
		if callbackError := database.Callback().Query().Before("gorm:query").Register("verify_usage_query", func(callbackDatabase *gorm.DB) {
			if callbackDatabase.Statement.Table == managedUsageEventTable {
				queryCount++
				if queryCount == 2 {
					callbackDatabase.AddError(errInternalTestDatabase)
				}
			}
		}); callbackError != nil {
			subTest.Fatalf("register usage query callback: %v", callbackError)
		}
		dataset := managedTenantMigrationDataset{
			users: []managedUserRecord{user}, tenants: []managedTenantRecord{tenantRecord}, usageEvents: []managedUsageEventRecord{usageRecord},
		}
		if verifyError := verifyManagedTenantMigration(database, cipher, dataset); verifyError == nil || !strings.Contains(verifyError.Error(), errInternalTestDatabase.Error()) {
			subTest.Fatalf("usage query error=%v", verifyError)
		}
	})
	t.Run("usage values", func(subTest *testing.T) {
		database := newDatabase(subTest)
		user, tenantRecord := baseRecords()
		if createError := database.Create(&user).Error; createError != nil {
			subTest.Fatalf("create user: %v", createError)
		}
		if createError := database.Create(&tenantRecord).Error; createError != nil {
			subTest.Fatalf("create tenant: %v", createError)
		}
		actualUsage := managedUsageEventRecord{ID: 1, TenantID: tenantRecord.TenantID, ProviderID: ProviderNameOpenAI, CreatedAt: now}
		if createError := database.Create(&actualUsage).Error; createError != nil {
			subTest.Fatalf("create usage: %v", createError)
		}
		expectedUsage := actualUsage
		expectedUsage.ProviderID = ProviderNameDeepSeek
		dataset := managedTenantMigrationDataset{
			users: []managedUserRecord{user}, tenants: []managedTenantRecord{tenantRecord}, usageEvents: []managedUsageEventRecord{expectedUsage},
		}
		if verifyError := verifyManagedTenantMigration(database, cipher, dataset); verifyError == nil || !strings.Contains(verifyError.Error(), "row=1") {
			subTest.Fatalf("usage values error=%v", verifyError)
		}
	})
}

func TestMigrateLegacyManagedTenantSchemaRollsBackTransactionalFailures(t *testing.T) {
	cipher := internalManagedProviderKeyCipher()
	providers := internalManagementProviderRegistry()
	now := time.Date(2026, 7, 25, 19, 0, 0, 0, time.UTC)
	seedFixture := func(subTest *testing.T) string {
		databasePath := filepath.Join(subTest.TempDir(), "transaction.db")
		database := openLegacyManagedTenantDatabase(subTest, databasePath)
		tenantRecord := legacyManagedTenantRecord{
			UserID: "owner", UserEmail: "owner@example.com", TenantID: "managed-default", CreatedAt: now, UpdatedAt: now,
		}
		tenantRecord.applyDefaults(defaultManagedRoutingDefaults())
		if createError := database.Table(managedTenantTable).Create(&tenantRecord).Error; createError != nil {
			subTest.Fatalf("seed tenant: %v", createError)
		}
		encryptedKey, encryptionError := cipher.encrypt(
			bytes.NewReader(bytes.Repeat([]byte{1}, cipher.aeadCipher.NonceSize())),
			tenantRecord.UserID,
			ProviderNameOpenAI,
			"sk-key",
		)
		if encryptionError != nil {
			subTest.Fatalf("encrypt provider: %v", encryptionError)
		}
		if createError := database.Table(managedProviderKeyTable).Create(&legacyManagedProviderAPIKeyRecord{
			UserID: tenantRecord.UserID, ProviderID: ProviderNameOpenAI, EncryptedAPIKey: encryptedKey,
			TextModel: ModelNameGPT41, CreatedAt: now, UpdatedAt: now,
		}).Error; createError != nil {
			subTest.Fatalf("seed provider: %v", createError)
		}
		if createError := database.Table(managedUsageEventTable).Create(&legacyManagedUsageEventRecord{
			ID: 1, UserID: tenantRecord.UserID, TenantID: tenantRecord.TenantID,
			StatusCode: http.StatusOK, Success: true, CreatedAt: now,
		}).Error; createError != nil {
			subTest.Fatalf("seed usage: %v", createError)
		}
		sqlDatabase, sqlError := database.DB()
		if sqlError != nil {
			subTest.Fatalf("legacy SQL database: %v", sqlError)
		}
		if closeError := sqlDatabase.Close(); closeError != nil {
			subTest.Fatalf("close legacy database: %v", closeError)
		}
		return databasePath
	}
	openDatabase := func(subTest *testing.T, dialector gorm.Dialector) *gorm.DB {
		database, openError := gorm.Open(dialector, &gorm.Config{})
		if openError != nil {
			subTest.Fatalf("open transactional fixture: %v", openError)
		}
		return database
	}
	testCases := []struct {
		name      string
		open      func(*testing.T, string) *gorm.DB
		configure func(*testing.T, *gorm.DB)
	}{
		{
			name: "rename index",
			open: func(subTest *testing.T, databasePath string) *gorm.DB {
				return openDatabase(subTest, failingManagedIndexRenameDialector{Dialector: sqlite.Open(databasePath)})
			},
		},
		{
			name: "rename table",
			open: func(subTest *testing.T, databasePath string) *gorm.DB {
				return openDatabase(subTest, failingManagedRenameDialector{Dialector: sqlite.Open(databasePath)})
			},
		},
		{
			name: "create current schema",
			open: func(subTest *testing.T, databasePath string) *gorm.DB {
				return openDatabase(subTest, failingManagedAutoMigrateDialector{Dialector: sqlite.Open(databasePath)})
			},
		},
		{
			name: "create users",
			configure: func(subTest *testing.T, database *gorm.DB) {
				registerManagedGORMError(subTest, database, "migrate_create_users", "create", managedUserTable, errInternalTestDatabase)
			},
		},
		{
			name: "create tenants",
			configure: func(subTest *testing.T, database *gorm.DB) {
				registerManagedGORMError(subTest, database, "migrate_create_tenants", "create", managedTenantTable, errInternalTestDatabase)
			},
		},
		{
			name: "create provider keys",
			configure: func(subTest *testing.T, database *gorm.DB) {
				registerManagedGORMError(subTest, database, "migrate_create_providers", "create", managedProviderKeyTable, errInternalTestDatabase)
			},
		},
		{
			name: "create usage",
			configure: func(subTest *testing.T, database *gorm.DB) {
				registerManagedGORMError(subTest, database, "migrate_create_usage", "create", managedUsageEventTable, errInternalTestDatabase)
			},
		},
		{
			name: "verify",
			configure: func(subTest *testing.T, database *gorm.DB) {
				registerManagedGORMError(subTest, database, "migrate_verify", "query", managedUserTable, errInternalTestDatabase)
			},
		},
		{
			name: "record version",
			configure: func(subTest *testing.T, database *gorm.DB) {
				registerManagedGORMError(subTest, database, "migrate_record_version", "create", managedSchemaMigrationTable, errInternalTestDatabase)
			},
		},
		{
			name: "drop legacy table",
			open: func(subTest *testing.T, databasePath string) *gorm.DB {
				return openDatabase(subTest, failingManagedDropDialector{Dialector: sqlite.Open(databasePath)})
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			databasePath := seedFixture(subTest)
			var database *gorm.DB
			if testCase.open != nil {
				database = testCase.open(subTest, databasePath)
			} else {
				database = openDatabase(subTest, sqlite.Open(databasePath))
			}
			if testCase.configure != nil {
				testCase.configure(subTest, database)
			}
			migrationError := migrateLegacyManagedTenantSchema(database, cipher, providers)
			if migrationError == nil {
				subTest.Fatal("transactional migration unexpectedly succeeded")
			}
			rollbackDatabase := openDatabase(subTest, sqlite.Open(databasePath))
			if !managedTableHasColumn(rollbackDatabase.Migrator(), managedTenantTable, "user_id") ||
				rollbackDatabase.Migrator().HasTable(managedUserTable) {
				subTest.Fatalf("failed migration mutated schema: %v", migrationError)
			}
			for _, rename := range legacyManagedIndexRenames() {
				if !rollbackDatabase.Migrator().HasIndex(rename.table, rename.source) {
					subTest.Fatalf("failed migration removed legacy index %s: %v", rename.source, migrationError)
				}
			}
		})
	}
}

func openUnconstrainedLegacyTenantDatabase(t *testing.T, databasePath string) *gorm.DB {
	t.Helper()
	database, openError := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if openError != nil {
		t.Fatalf("open unconstrained legacy database: %v", openError)
	}
	createTenantTable := `CREATE TABLE ` + managedTenantTable + ` (
		user_id TEXT,
		user_email TEXT,
		user_display_name TEXT,
		user_avatar_url TEXT,
		tenant_id TEXT,
		secret_digest TEXT,
		default_provider TEXT,
		default_model TEXT,
		default_dictation_provider TEXT,
		default_dictation_model TEXT,
		default_system_prompt TEXT,
		default_reasoning_effort TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`
	if createError := database.Exec(createTenantTable).Error; createError != nil {
		t.Fatalf("create unconstrained tenant table: %v", createError)
	}
	for _, migration := range []struct {
		table string
		model interface{}
	}{
		{table: managedProviderKeyTable, model: &legacyManagedProviderAPIKeyRecord{}},
		{table: managedUsageEventTable, model: &legacyManagedUsageEventRecord{}},
	} {
		if migrationError := database.Table(migration.table).AutoMigrate(migration.model); migrationError != nil {
			t.Fatalf("create legacy table %s: %v", migration.table, migrationError)
		}
	}
	return database
}

type failingManagedRenameDialector struct {
	gorm.Dialector
}

type failingManagedIndexRenameDialector struct {
	gorm.Dialector
}

func (dialector failingManagedIndexRenameDialector) Migrator(database *gorm.DB) gorm.Migrator {
	return failingManagedIndexRenameMigrator{Migrator: dialector.Dialector.Migrator(database)}
}

type failingManagedIndexRenameMigrator struct {
	gorm.Migrator
}

func (failingManagedIndexRenameMigrator) RenameIndex(interface{}, string, string) error {
	return errInternalTestDatabase
}

func (dialector failingManagedRenameDialector) Migrator(database *gorm.DB) gorm.Migrator {
	return failingManagedRenameMigrator{Migrator: dialector.Dialector.Migrator(database)}
}

type failingManagedRenameMigrator struct {
	gorm.Migrator
}

func (migrator failingManagedRenameMigrator) RenameTable(interface{}, interface{}) error {
	return errInternalTestDatabase
}

type failingManagedDropDialector struct {
	gorm.Dialector
}

func (dialector failingManagedDropDialector) Migrator(database *gorm.DB) gorm.Migrator {
	return failingManagedDropMigrator{Migrator: dialector.Dialector.Migrator(database)}
}

type failingManagedDropMigrator struct {
	gorm.Migrator
}

func (migrator failingManagedDropMigrator) DropTable(...interface{}) error {
	return errInternalTestDatabase
}
