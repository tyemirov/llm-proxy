package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestHostedMediaAdmissionRetainsOperationIdentity(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	server, _ := newHostedMediaAdmissionHTTPServer(t, database)
	first := hostedMediaAdmissionHTTP(t, server, "media-repeat", "private image prompt", http.StatusAccepted)
	second := hostedMediaAdmissionHTTP(t, server, "media-repeat", "private image prompt", http.StatusOK)
	if first["operation_id"] != second["operation_id"] || first["state"] != MediaOperationStateQueued {
		t.Fatalf("media replay changed execution: first=%v second=%v", first, second)
	}
	hostedMediaAdmissionHTTP(t, server, "media-repeat", "changed image prompt", http.StatusConflict)
	entries := read("")["requests"].([]any)
	if len(entries) != 1 {
		t.Fatalf("journal requests=%v", entries)
	}
	record := entries[0].(map[string]any)
	if record["execution_id"] != first["operation_id"] || record["execution_kind"] != string(journalExecutionMedia) || record["state"] != string(journalRequestAccepted) {
		t.Fatalf("media journal attribution=%v", record)
	}
}

func TestHostedMediaAdmissionRollsBackReservationAndOperation(t *testing.T) {
	for _, failure := range []string{"reservation", "operation"} {
		t.Run(failure, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			server, service := newHostedMediaAdmissionHTTPServer(t, database)
			if err := database.database.Exec("CREATE TABLE media_reservation_probe (request_id TEXT PRIMARY KEY)").Error; err != nil {
				t.Fatal(err)
			}
			reject := true
			service.hostedAdmission = func(transaction *gorm.DB, request managedJournalRequestRecord) error {
				if err := transaction.Exec("INSERT INTO media_reservation_probe (request_id) VALUES (?)", request.ID).Error; err != nil {
					return err
				}
				if reject && failure == "reservation" {
					return errors.New("controlled reservation failure")
				}
				return nil
			}
			if failure == "operation" {
				if err := database.database.Exec("CREATE TRIGGER reject_media_operation BEFORE INSERT ON media_operation_records BEGIN SELECT RAISE(ABORT, 'controlled operation failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			hostedMediaAdmissionHTTP(t, server, "rollback", "private image prompt", http.StatusInternalServerError)
			for _, table := range []string{"media_reservation_probe", "media_operation_records"} {
				var count int64
				if err := database.database.Table(table).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("partial admission in %s: count=%d error=%v", table, count, err)
				}
			}
			if entries := read("")["requests"].([]any); len(entries) != 0 {
				t.Fatalf("failed admission retained journal: %v", entries)
			}
			reject = false
			if failure == "operation" {
				if err := database.database.Exec("DROP TRIGGER reject_media_operation").Error; err != nil {
					t.Fatal(err)
				}
			}
			hostedMediaAdmissionHTTP(t, server, "rollback", "private image prompt", http.StatusAccepted)
			var count int64
			if err := database.database.Table("media_reservation_probe").Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("reservation count=%d error=%v", count, err)
			}
		})
	}
}

func TestHostedMediaAdmissionConcurrentInstancesReserveOnce(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	first, firstService := newHostedMediaAdmissionHTTPServer(t, database)
	second, secondService := newHostedMediaAdmissionHTTPServer(t, openJournalTransactionInstance(t, database))
	var reservations atomic.Int64
	reserve := func(*gorm.DB, managedJournalRequestRecord) error { reservations.Add(1); return nil }
	firstService.hostedAdmission, secondService.hostedAdmission = reserve, reserve
	var group sync.WaitGroup
	results := make([]map[string]any, 6)
	for index := range results {
		group.Add(1)
		go func() {
			defer group.Done()
			results[index] = hostedMediaAdmissionHTTP(t, []*httptest.Server{first, second}[index%2], "concurrent-media", "private image prompt", 0)
		}()
	}
	group.Wait()
	for _, result := range results {
		if result["operation_id"] != results[0]["operation_id"] {
			t.Fatalf("duplicate operation identities: %v", results)
		}
	}
	if entries := read("")["requests"].([]any); len(entries) != 1 || reservations.Load() != 1 {
		t.Fatalf("concurrent requests=%v reservations=%d", entries, reservations.Load())
	}
}

func TestHostedMediaAdmissionRetainsAcceptedAuthority(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	server, service := newHostedMediaAdmissionHTTPServer(t, database)
	var reservations int
	service.hostedAdmission = func(*gorm.DB, managedJournalRequestRecord) error { reservations++; return nil }
	first := hostedMediaAdmissionHTTP(t, server, "retained", "private image prompt", http.StatusAccepted)
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("state", hostedGrantRevoked).Error; err != nil {
		t.Fatal(err)
	}
	second := hostedMediaAdmissionHTTP(t, server, "retained", "private image prompt", http.StatusOK)
	hostedMediaAdmissionHTTP(t, server, "denied", "private image prompt", http.StatusUnprocessableEntity)
	if first["operation_id"] != second["operation_id"] || reservations != 1 || len(read("")["requests"].([]any)) != 1 {
		t.Fatal("revocation changed an accepted identity or reserved more funds")
	}
	var operation mediaOperationRecord
	if err := database.database.First(&operation, "operation_id = ?", first["operation_id"]).Error; err != nil || operation.CredentialReference != "platform-journal:v1" {
		t.Fatalf("accepted media authority=%q error=%v", operation.CredentialReference, err)
	}
}

func TestHostedMediaAdmissionRejectsTextKeyReuse(t *testing.T) {
	database, intent, read := newJournalTransactionFixture(t)
	if _, err := database.admitJournalRequest(t.Context(), intent("shared-key"), func(*gorm.DB, managedJournalRequestRecord) error { return nil }); err != nil {
		t.Fatal(err)
	}
	server, service := newHostedMediaAdmissionHTTPServer(t, database)
	service.hostedAdmission = func(*gorm.DB, managedJournalRequestRecord) error { t.Error("conflict reserved funds"); return nil }
	hostedMediaAdmissionHTTP(t, server, "shared-key", "private image prompt", http.StatusConflict)
	if entries := read("")["requests"].([]any); len(entries) != 1 || entries[0].(map[string]any)["execution_kind"] != string(journalExecutionText) {
		t.Fatalf("media changed the text receipt: %v", entries)
	}
}

func TestHostedMediaAdmissionTextStartupPreservesMediaRecovery(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	server, _ := newHostedMediaAdmissionHTTPServer(t, database)
	result := hostedMediaAdmissionHTTP(t, server, "media-recovery", "private image prompt", http.StatusAccepted)
	var record managedJournalRequestRecord
	if err := database.database.First(&record, "execution_id = ?", result["operation_id"]).Error; err != nil {
		t.Fatal(err)
	}
	claim, err := newJournalWorkerClaim(record.ID, record.OwnerToken, record.CreatedAt.Add(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-media-recovery", func(*gorm.DB, managedJournalRequestRecord) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := database.dispatchJournalAttempt(t.Context(), claim, attempt.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.database.Model(&managedJournalRequestRecord{}).Where("id = ?", record.ID).Update("claim_expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	newHostedIdentityHTTPServer(t, database, "http://127.0.0.1:1", t.TempDir())
	if current := read("/" + record.ID); current["state"] != string(journalRequestExecuting) || current["usage_state"] != string(journalUsagePending) {
		t.Fatalf("text startup changed media recovery evidence: %v", current)
	}
}

func TestHostedMediaAdmissionRechecksAuthorityAfterValidation(t *testing.T) {
	for _, change := range []string{"revocation", "rotation"} {
		t.Run(change, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			server, service := newHostedMediaAdmissionHTTPServer(t, database)
			key := mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")
			service.adapters[key] = hostedMediaValidationHook{MediaOperationAdapter: service.adapters[key], after: func() error {
				if change == "revocation" {
					return database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("state", hostedGrantRevoked).Error
				}
				return database.database.Transaction(func(transaction *gorm.DB) error {
					if err := transaction.Create(&managedPlatformCredentialRecord{ConnectionID: "platform-journal", Version: 2, Fields: []byte(`{}`), QualifiedAt: time.Now(), CreatedAt: time.Now()}).Error; err != nil {
						return err
					}
					return transaction.Model(&managedPlatformConnectionRecord{}).Where("id = ?", "platform-journal").Update("version", 2).Error
				})
			}}
			service.hostedAdmission = func(*gorm.DB, managedJournalRequestRecord) error {
				t.Error("obsolete authority reserved funds")
				return nil
			}
			hostedMediaAdmissionHTTP(t, server, "authority-change", "private image prompt", http.StatusUnprocessableEntity)
			if entries := read("")["requests"].([]any); len(entries) != 0 {
				t.Fatalf("obsolete authority created a journal request: %v", entries)
			}
		})
	}
}

func TestHostedMediaAdmissionRequiresFundsOwner(t *testing.T) {
	database, _, read := newJournalTransactionFixture(t)
	server, service := newHostedMediaAdmissionHTTPServer(t, database)
	service.hostedAdmission = nil
	hostedMediaAdmissionHTTP(t, server, "disabled", "private image prompt", http.StatusUnprocessableEntity)
	if entries := read("")["requests"].([]any); len(entries) != 0 || len(service.queue) != 0 {
		t.Fatalf("unfunded admission created work: requests=%v queued=%d", entries, len(service.queue))
	}
}

type hostedMediaValidationHook struct {
	MediaOperationAdapter
	after func() error
}

func (adapter hostedMediaValidationHook) Validate(ctx context.Context, request MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	validated, err := adapter.MediaOperationAdapter.Validate(ctx, request)
	if err != nil {
		return validated, err
	}
	return validated, adapter.after()
}

func newHostedMediaAdmissionHTTPServer(t *testing.T, database *gormManagedTenantDatabase, configure ...func(*mediaOperationService)) (*httptest.Server, *mediaOperationService) {
	t.Helper()
	router, management, _ := newHostedIdentityHTTPHandler(t, database, "http://127.0.0.1:1", t.TempDir())
	catalog, err := NewCatalogService(internalCanonicalProviderCatalog().ModelCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Updates(map[string]any{
		"catalog_revision": catalog.Revision(), "offerings": []byte(`[{"model":"gpt-image-2","operations":["image_generation"]}]`),
	}).Error; err != nil {
		t.Fatal(err)
	}
	store, err := newMediaOperationStore(management.store)
	if err != nil {
		t.Fatal(err)
	}
	service := &mediaOperationService{
		logger:          zap.NewNop().Sugar(),
		hostedAdmission: func(*gorm.DB, managedJournalRequestRecord) error { return nil },
		httpClient:      http.DefaultClient,
		store:           store, assets: newTenantAssetStore(t.TempDir(), 4096, 60), providers: internalManagementProviderRegistry(),
		catalog: catalog, adapters: map[string]MediaOperationAdapter{},
		queue: make(chan string, 10), lifetime: time.Hour, claimLifetime: time.Minute, claimRenewal: 20 * time.Second,
		globalCapacity: 10, tenantCapacity: 10, terminalRetention: time.Hour,
	}
	for _, apply := range configure {
		apply(service)
	}
	offering, err := catalog.ResolveOffering("openai", "gpt-image-2")
	if err != nil {
		t.Fatal(err)
	}
	service.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")] = newImageGenerationAdapter(offering, service.providers.definitions[providerID("openai")], management.store, store, service.assets, catalog)
	auth := newTenantAuthenticator(management.store)
	router.POST(llmproxycontract.MediaOperationsPath, tenantAuthenticatedHandler(auth, zap.NewNop().Sugar(), service.createHandler()))
	router.GET(llmproxycontract.MediaOperationsPath+"/:operation_id", tenantAuthenticatedHandler(auth, zap.NewNop().Sugar(), service.statusHandler()))
	router.PUT(llmproxycontract.MediaOperationsPath+"/:operation_id/cancellation", tenantAuthenticatedHandler(auth, zap.NewNop().Sugar(), service.cancellationHandler()))
	server := httptest.NewServer(router)
	server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
	t.Cleanup(server.Close)
	return server, service
}

func hostedMediaAdmissionHTTP(t *testing.T, server *httptest.Server, key, prompt string, want int, selectedControls ...json.RawMessage) map[string]any {
	t.Helper()
	controls := json.RawMessage(`{"surface":"images","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1}`)
	if len(selectedControls) != 0 {
		controls = selectedControls[0]
	}
	body, err := json.Marshal(mediaOperationCreatePayload{Capability: llmproxycontract.MediaCapabilityImageGenerate, Provider: "openai", Model: "gpt-image-2", Input: mustHostedMediaInput(t, prompt), Controls: controls})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+llmproxycontract.MediaOperationsPath, strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	encoded, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if (want != 0 && response.StatusCode != want) || (want == 0 && response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted) {
		t.Fatalf("media admission status=%d want=%d body=%s", response.StatusCode, want, encoded)
	}
	validateHostedIdentityResponse(t, request, response, encoded)
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func mustHostedMediaInput(t *testing.T, prompt string) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		t.Fatal(err)
	}
	return body
}
