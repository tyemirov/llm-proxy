package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func assignedConnectionFieldForTest(t *testing.T, database *gorm.DB, tenantID, providerID, fieldID string) managedConnectionFieldRecord {
	t.Helper()
	var assignment managedTenantConnectionRecord
	if err := database.Where("tenant_id = ? AND provider_id = ?", tenantID, providerID).First(&assignment).Error; err != nil {
		t.Fatal(err)
	}
	var field managedConnectionFieldRecord
	if err := database.Where("connection_id = ? AND field_id = ?", assignment.ConnectionID, fieldID).First(&field).Error; err != nil {
		t.Fatal(err)
	}
	return field
}

func assertMigratedConnectionField(t *testing.T, database *gorm.DB, cipher managedProviderKeyCipher, original managedProviderConnectionRecord) {
	t.Helper()
	field := assignedConnectionFieldForTest(t, database, original.TenantID, original.ProviderID, original.FieldID)
	before, err := cipher.decryptConnection(original)
	if err != nil {
		t.Fatal(err)
	}
	after, err := cipher.decryptConnectionValue(field.ConnectionID, original.ProviderID, original.FieldID, field.Value)
	if err != nil || before != after || !field.CreatedAt.Equal(original.CreatedAt) || !field.UpdatedAt.Equal(original.UpdatedAt) || field.Value == original.Value {
		t.Fatalf("migration failed to preserve and rebind the provider credential: %v", err)
	}
}

func TestAccountConnectionMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migration.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cipher := internalManagedProviderKeyCipher()
	providers := internalManagementProviderRegistry()
	if err := initializeManagedTenantSchemaRecords(db, cipher, providers); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)
	user := managedUserRecord{UserID: "migration-owner", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	secrets := map[string]string{}
	usageBefore := []managedUsageEventRecord{}
	for index, id := range []string{"home", "work"} {
		tenant := fakeTenantRecord(user.UserID, id, id, now)
		digest := sha256.Sum256([]byte("tenant-access-" + id))
		secret := hex.EncodeToString(digest[:])
		tenant.SecretDigest = &secret
		secrets[id] = secret
		tenant.DefaultProvider = ProviderNameOpenAI
		tenant.DefaultModel = ModelNameGPT41
		tenant.DefaultSystemPrompt = "Tenant prompt"
		if err := db.Create(&tenant).Error; err != nil {
			t.Fatal(err)
		}
		profile := managedProviderProfileRecord{TenantID: id, ProviderID: ProviderNameOpenAI, TextModel: ModelNameGPT41, SystemPrompt: "Provider prompt", CreatedAt: now, UpdatedAt: now}
		if err := db.Create(&profile).Error; err != nil {
			t.Fatal(err)
		}
		value, err := cipher.encryptConnection(strings.NewReader(strings.Repeat("n", 64)), id, ProviderNameOpenAI, "api_key", "same-key")
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&managedProviderConnectionRecord{TenantID: id, ProviderID: ProviderNameOpenAI, FieldID: "api_key", Value: value, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
		usage := managedUsageEventRecord{ID: uint(index + 1), TenantID: id, Endpoint: usageEndpointText, ProviderID: ProviderNameOpenAI, ModelID: ModelNameGPT41, StatusCode: http.StatusOK, Disposition: managedUsageDispositionSucceeded, OutcomeCode: managedUsageOutcomeSuccess, LatencyMilliseconds: 75, RequestTokens: 100, ResponseTokens: 30, TotalTokens: 130, CreatedAt: now}
		if err := db.Create(&usage).Error; err != nil {
			t.Fatal(err)
		}
		usageBefore = append(usageBefore, usage)
	}
	if err := initializeManagedTenantSchema(db, cipher, providers); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable(managedProviderConnectionTable) {
		t.Fatal("predecessor credential table remains")
	}
	database := &gormManagedTenantDatabase{database: db}
	connections, err := database.accountConnections(context.Background(), user.UserID, managedConnectionPage{limit: managementConnectionPageMaximum})
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 2 {
		t.Fatalf("connections=%d want two independent copies", len(connections))
	}
	store := newManagedTenantStoreWithDatabaseAndCipher(database, cipher)
	store.routingDefaults = providers
	for _, connection := range connections {
		if len(connection.Assignments) != 1 {
			t.Fatal("migration changed assignments")
		}
		settings, err := store.accountConnectionSettings(connection)
		if err != nil {
			t.Fatal(err)
		}
		if settings.connectionValue("api_key") != "same-key" {
			t.Fatal("credential changed")
		}
		tenant, err := database.tenantByOwnerAndID(user.UserID, connection.Assignments[0].TenantID)
		if err != nil {
			t.Fatal(err)
		}
		if tenant.DefaultModel != ModelNameGPT41 || tenant.DefaultSystemPrompt != "Tenant prompt" || tenant.ProviderProfiles[0].SystemPrompt != "Provider prompt" {
			t.Fatal("tenant defaults or prompts changed")
		}
		if tenant.SecretDigest == nil || *tenant.SecretDigest != secrets[tenant.TenantID] || !tenant.CreatedAt.Equal(now) || !tenant.UpdatedAt.Equal(now) || !connection.CreatedAt.Equal(now) || !connection.UpdatedAt.Equal(now) {
			t.Fatal("migration changed tenant access or configuration timestamps")
		}
		resolved, err := database.tenantBySecretDigest(context.Background(), secrets[tenant.TenantID])
		if err != nil || resolved.TenantID != tenant.TenantID {
			t.Fatalf("tenant access no longer resolves: %v", err)
		}
	}
	var usageAfter []managedUsageEventRecord
	if err := db.Order("id").Find(&usageAfter).Error; err != nil {
		t.Fatal(err)
	}
	if len(usageAfter) != len(usageBefore) {
		t.Fatal("migration changed usage count")
	}
	for index := range usageBefore {
		before, after := usageBefore[index], usageAfter[index]
		if !before.CreatedAt.Equal(after.CreatedAt) {
			t.Fatal("migration changed usage timestamp")
		}
		after.CreatedAt = before.CreatedAt
		if before != after {
			t.Fatalf("migration changed usage: before=%+v after=%+v", before, after)
		}
	}
	if err := initializeManagedTenantSchema(db, cipher, providers); err != nil {
		t.Fatal(err)
	}
	again, err := database.accountConnections(context.Background(), user.UserID, managedConnectionPage{limit: managementConnectionPageMaximum})
	if err != nil || len(again) != 2 || again[0].ID != connections[0].ID {
		t.Fatal("restart duplicated migrated connections")
	}
}
