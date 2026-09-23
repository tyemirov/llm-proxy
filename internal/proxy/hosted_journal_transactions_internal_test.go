package proxy

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func newJournalTransactionFixture(t *testing.T) (*gormManagedTenantDatabase, func(string) journalAdmissionIntent, func(string) map[string]any) {
	t.Helper()
	service, database, server := newAccountConnectionHTTPFixture(t)
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	for _, record := range []any{
		&managedBillingAccountRecord{ID: "billing-journal", OwnerUserID: "owner", Currency: "USD", CreationKeyDigest: strings.Repeat("a", 64), CreatedAt: now},
		&managedPlatformConnectionRecord{ID: "platform-journal", Provider: "openai", Name: "Journal", Version: 1, CreatedAt: now, UpdatedAt: now},
		&managedPlatformCredentialRecord{ConnectionID: "platform-journal", Version: 1, Fields: []byte(`{}`), QualifiedAt: now, CreatedAt: now},
		&managedHostedGrantRecord{ID: "grant-journal", BillingAccountID: "billing-journal", TenantID: "managed-first", PlatformConnectionID: "platform-journal", Provider: "openai", CatalogRevision: "journal-catalog", Offerings: []byte(`[{"model":"gpt-4.1","operations":["text"]}]`), State: hostedGrantActive, Revision: 1, CreatedAt: now, UpdatedAt: now},
		&managedHostedGrantRevisionRecord{GrantID: "grant-journal", Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Journal fixture", CreatedAt: now},
		&managedHostedTenantAssignmentRecord{TenantID: "managed-first", ProviderID: "openai", GrantID: "grant-journal", CreatedAt: now},
		&managedProviderProfileRecord{TenantID: "managed-first", ProviderID: "openai", TextModel: "gpt-4.1", CreatedAt: now, UpdatedAt: now},
	} {
		if err := database.database.Omit(clause.Associations).Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := server.Config.Handler.(*gin.Engine)
	router.GET(managementAPIPath+managementJournalRequestsPath, service.listJournalRequestsHandler())
	router.GET(managementAPIPath+managementJournalRequestPath, service.getJournalRequestHandler())
	router.GET(managementAPIPath+managementJournalCasesPath, service.listJournalCasesHandler())
	intent := func(key string) journalAdmissionIntent {
		t.Helper()
		value, err := newJournalAdmissionIntent(journalAdmissionInput{OwnerUserID: "owner", TenantID: "managed-first", Provider: "openai", Model: "gpt-4.1", Operation: "text", CatalogRevision: "journal-catalog", IdempotencyKey: key, CanonicalIntent: []byte(`{"messages":[{"role":"user","content":"private prompt"}]}`), ExecutionKind: journalExecutionText, ExecutionID: "execution-" + key, OwnerToken: "worker-" + key, Now: now, ClaimExpiresAt: now.Add(time.Minute)}, rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	read := func(path string) map[string]any {
		t.Helper()
		return accountConnectionHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/requests"+path, "", http.StatusOK)
	}
	return database, intent, read
}

func TestHostedJournalAdmissionPinsAuthorityAndArbitratesRetries(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	second := openJournalTransactionInstance(t, database)
	var reservations atomic.Int64
	reserve := func(_ *gorm.DB, record managedJournalRequestRecord) error {
		if record.State != journalRequestAccepted || record.GrantRevision != 1 || record.CredentialVersion != 1 {
			return fmt.Errorf("incorrect admission attribution")
		}
		reservations.Add(1)
		return nil
	}
	var group sync.WaitGroup
	const count = 6
	records := make([]managedJournalRequestRecord, count)
	results := make([]error, count)
	for index := range count {
		proposed := intent("same-key")
		instance := []*gormManagedTenantDatabase{database, second}[index%2]
		group.Add(1)
		go func() {
			defer group.Done()
			records[index], results[index] = instance.admitJournalRequest(t.Context(), proposed, reserve)
		}()
	}
	group.Wait()
	for index, err := range results {
		if err != nil || records[index].ID != records[0].ID {
			t.Fatalf("duplicate admission %d: record=%+v error=%v", index, records[index], err)
		}
	}
	if reservations.Load() != 1 {
		t.Fatalf("duplicate reservation calls=%d", reservations.Load())
	}
	response := read("/" + records[0].ID)
	if response["execution_id"] != "execution-same-key" || response["state"] != "accepted" {
		t.Fatalf("request=%v", response)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private prompt", "platform-journal", "worker-same-key", "intent_digest", "key_digest"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("private journal field in response: %s", encoded)
		}
	}
	changed := intent("same-key")
	changed.record.IntentDigest = strings.Repeat("c", 64)
	if _, err := database.admitJournalRequest(t.Context(), changed, reserve); !errors.Is(err, errUsageJournalConflict) {
		t.Fatalf("changed intent error=%v", err)
	}
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("state", hostedGrantSuspended).Error; err != nil {
		t.Fatal(err)
	}
	if replay, err := database.admitJournalRequest(t.Context(), intent("same-key"), reserve); err != nil || replay.ID != records[0].ID {
		t.Fatalf("retained admission changed after suspension: %v", err)
	}
	if _, err := database.admitJournalRequest(t.Context(), intent("new-key"), reserve); !errors.Is(err, errHostedAuthorityDenied) {
		t.Fatalf("new suspended admission error=%v", err)
	}
	if reservations.Load() != 1 {
		t.Fatal("suspension or replay reserved more funds")
	}
}

func openJournalTransactionInstance(t *testing.T, original *gormManagedTenantDatabase) *gormManagedTenantDatabase {
	t.Helper()
	instance, err := newGORMManagedTenantDatabase(ManagementConfiguration{DatabaseDialector: original.database.Dialector}, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
	if err != nil {
		t.Fatal(err)
	}
	connection, err := instance.database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Error(err)
		}
	})
	return instance
}

func TestHostedJournalAdmissionRollsBackReservationFailures(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	if err := database.database.Exec("CREATE TABLE controlled_reservation_effects (request_id TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	failure := errors.New("controlled reservation failure")
	_, err := database.admitJournalRequest(t.Context(), intent("rollback"), func(tx *gorm.DB, record managedJournalRequestRecord) error {
		if err := tx.Exec("INSERT INTO controlled_reservation_effects(request_id) VALUES (?)", record.ID).Error; err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("admission error=%v", err)
	}
	var effects int64
	if err := database.database.Table("controlled_reservation_effects").Count(&effects).Error; err != nil {
		t.Fatal(err)
	}
	if effects != 0 || len(read("")["requests"].([]any)) != 0 {
		t.Fatal("failed admission retained a request or reservation effect")
	}
}
