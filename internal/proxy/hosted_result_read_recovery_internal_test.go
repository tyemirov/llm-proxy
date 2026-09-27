package proxy

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func (fixture publicationRecoveryFixture) resultState(t *testing.T) map[string]any {
	t.Helper()
	state := fixture.state(t)
	var request managedJournalRequestRecord
	if err := fixture.database.database.Where("id = ?", fixture.request.ID).First(&request).Error; err != nil {
		t.Fatal(err)
	}
	state["request"] = request
	return state
}

func (fixture publicationRecoveryFixture) assertResultUnchanged(t *testing.T, before map[string]any) {
	t.Helper()
	if fixture.calls.Load() != 1 || !reflect.DeepEqual(before, fixture.resultState(t)) {
		t.Fatalf("result read changed journal or financial effects: calls=%d", fixture.calls.Load())
	}
}

func assertPrivateResultAbsent(t *testing.T, body string) {
	t.Helper()
	for _, private := range []string{"funded result", "controlled_", "unrelated-"} {
		if strings.Contains(body, private) {
			t.Fatalf("result error exposed private data: %s", body)
		}
	}
}

func TestHostedResultReadRejectsInvalidRetainedKeyDigest(t *testing.T) {
	fixture := newPublicationRecoveryFixture(t, "receipt")
	fixture.recover(t, "receipt")
	before := fixture.resultState(t)
	queries := fixture.database.database.Callback().Query()
	const callback = "test:invalid_result_digest"
	var failures atomic.Int64
	if err := queries.After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.DryRun || tx.Error != nil || tx.Statement.Table != "managed_journal_request_records" || !strings.Contains(tx.Statement.SQL.String(), "key_digest = ?") {
			return
		}
		tx.Statement.Dest.(*managedJournalRequestRecord).KeyDigest = "../invalid-result-digest"
		failures.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = queries.Remove(callback) })
	for range 2 {
		assertPrivateResultAbsent(t, hostedIdentityStatusHTTP(t, fixture.generation, publicationRecoveryKey, http.StatusInternalServerError))
	}
	if err := queries.Remove(callback); err != nil {
		t.Fatal(err)
	}
	if failures.Load() != 2 {
		t.Fatalf("invalid digest read count=%d", failures.Load())
	}
	fixture.assertResultUnchanged(t, before)
	fixture.recover(t, "receipt")
}

func TestHostedResultStartupRejectsInvalidStoredKeyDigest(t *testing.T) {
	fixture := newPublicationRecoveryFixture(t, "receipt")
	configuration, _ := hostedRecoveryApplicationConfiguration(t, fixture.fundsAdmissionFixture)
	if err := fixture.database.database.Model(&managedJournalRequestRecord{}).Where("id = ?", fixture.request.ID).Update("key_digest", "../invalid-result-digest").Error; err != nil {
		t.Fatal(err)
	}
	before := fixture.resultState(t)
	for range 2 {
		router, err := BuildRouter(configuration, zap.NewNop().Sugar())
		if router != nil || !errors.Is(err, errUsageJournalConflict) {
			t.Fatalf("invalid stored digest admitted startup: router=%v error=%v", router, err)
		}
		fixture.assertResultUnchanged(t, before)
	}
	if err := fixture.database.database.Model(&managedJournalRequestRecord{}).Where("id = ?", fixture.request.ID).Update("key_digest", fixture.request.KeyDigest).Error; err != nil {
		t.Fatal(err)
	}
	fixture.recover(t, "receipt")
}

func TestHostedResultRecoveryRejectsInvalidKeyDigestRead(t *testing.T) {
	fixture := newPublicationRecoveryFixture(t, "receipt")
	configuration, _ := hostedRecoveryApplicationConfiguration(t, fixture.fundsAdmissionFixture)
	before := fixture.resultState(t)
	var failures atomic.Int64
	configuration.Management.DatabaseDialector = hostedResultReadFailureDialector{
		Dialector: fixture.database.database.Dialector, failures: &failures,
		change: func(tx *gorm.DB) {
			tx.Statement.Dest.(*managedJournalRequestRecord).KeyDigest = "../invalid-result-digest"
		},
	}
	for range 2 {
		router, err := BuildRouter(configuration, zap.NewNop().Sugar())
		if router != nil || err == nil || !strings.Contains(err.Error(), "invalid structured request digest") {
			t.Fatalf("invalid result digest read admitted recovery: router=%v error=%v", router, err)
		}
		fixture.assertResultUnchanged(t, before)
	}
	if failures.Load() != 2 {
		t.Fatalf("recovery digest failures=%d", failures.Load())
	}
	fixture.recover(t, "receipt")
}

func TestHostedResultReadRejectsMismatchedSavedIdentity(t *testing.T) {
	for _, checkpoint := range []string{"unpublished", "published"} {
		for _, scenario := range []struct {
			name   string
			change func(*structuredRequestRecord)
		}{
			{"request", func(record *structuredRequestRecord) { record.ProxyRequestID = "unrelated-request" }},
			{"intent", func(record *structuredRequestRecord) { record.IntentSHA256 = sha256Hex("unrelated-intent") }},
			{"tenant", func(record *structuredRequestRecord) { record.TenantSHA256 = sha256Hex("unrelated-tenant") }},
			{"key", func(record *structuredRequestRecord) { record.IdempotencySHA256 = sha256Hex("unrelated-key") }},
			{"provider", func(record *structuredRequestRecord) { record.Provider = "unrelated-provider" }},
			{"model", func(record *structuredRequestRecord) { record.Model = "unrelated-model" }},
		} {
			t.Run(checkpoint+"/"+scenario.name, func(t *testing.T) {
				fixture := newPublicationRecoveryFixture(t, "receipt")
				if checkpoint == "published" {
					fixture.recover(t, "receipt")
				}
				before := fixture.resultState(t)
				original, err := os.ReadFile(fixture.resultPath)
				if err != nil {
					t.Fatal(err)
				}
				var record structuredRequestRecord
				if err := json.Unmarshal(original, &record); err != nil {
					t.Fatal(err)
				}
				scenario.change(&record)
				encoded, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				writeHostedRecoverySignal(t, fixture.resultPath, string(encoded))
				for range 2 {
					body := hostedIdentityStatusHTTP(t, fixture.generation, publicationRecoveryKey, http.StatusInternalServerError)
					if !strings.Contains(body, llmproxycontract.ErrorCodeStructuredRequestStore) {
						t.Fatalf("identity mismatch lost status error code: %s", body)
					}
					assertPrivateResultAbsent(t, body)
					body = hostedIdentityHTTP(t, fixture.generation, publicationRecoveryKey, "funded prompt", http.StatusConflict)
					if !strings.Contains(body, llmproxycontract.ErrorCodeUsageJournalConflict) {
						t.Fatalf("identity mismatch lost replay error code: %s", body)
					}
					assertPrivateResultAbsent(t, body)
					fixture.assertResultUnchanged(t, before)
				}
				retained, err := os.ReadFile(fixture.resultPath)
				if err != nil || string(retained) != string(encoded) {
					t.Fatalf("rejected read changed the saved file: %v", err)
				}
				writeHostedRecoverySignal(t, fixture.resultPath, string(original))
				fixture.recover(t, "receipt")
			})
		}
	}
}

func TestHostedResultReadStorageFailuresKeepFinancialEvidence(t *testing.T) {
	for _, checkpoint := range []string{"unpublished", "published"} {
		t.Run(checkpoint, func(t *testing.T) {
			fixture := newPublicationRecoveryFixture(t, "receipt")
			if checkpoint == "published" {
				fixture.recover(t, "receipt")
			}
			before := fixture.resultState(t)
			var failures atomic.Int64
			fixture.responses.mu.Lock()
			read := fixture.responses.read
			fixture.responses.read = func(string) (structuredRequestRecord, error) {
				failures.Add(1)
				return structuredRequestRecord{}, errors.New("controlled_result_read_failure")
			}
			fixture.responses.mu.Unlock()
			restore := func() {
				fixture.responses.mu.Lock()
				fixture.responses.read = read
				fixture.responses.mu.Unlock()
			}
			t.Cleanup(restore)
			for range 2 {
				body := hostedIdentityStatusHTTP(t, fixture.generation, publicationRecoveryKey, http.StatusInternalServerError)
				if !strings.Contains(body, llmproxycontract.ErrorCodeStructuredRequestStore) {
					t.Fatalf("result read lost status error code: %s", body)
				}
				assertPrivateResultAbsent(t, body)
				assertPrivateResultAbsent(t, hostedIdentityHTTP(t, fixture.generation, publicationRecoveryKey, "funded prompt", http.StatusBadGateway))
				fixture.assertResultUnchanged(t, before)
			}
			if failures.Load() != 4 {
				t.Fatalf("result read failures=%d want=4", failures.Load())
			}
			restore()
			fixture.recover(t, "receipt")
		})
	}
}

func TestHostedResultReadStatusDatabaseFailureKeepsUnpublishedReceipt(t *testing.T) {
	fixture := newPublicationRecoveryFixture(t, "receipt")
	before := fixture.resultState(t)
	var failures atomic.Int64
	const callbackName = "test:hosted_result_status_read"
	callback := fixture.database.database.Callback().Query()
	if err := callback.After("gorm:query").Register(callbackName, func(transaction *gorm.DB) {
		if !transaction.DryRun && transaction.Statement.Table == "managed_journal_request_records" && strings.Contains(transaction.Statement.SQL.String(), "billing_account_id IN") {
			failures.Add(1)
			transaction.AddError(errors.New("controlled_result_status_read_failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		body := hostedIdentityStatusHTTP(t, fixture.generation, publicationRecoveryKey, http.StatusInternalServerError)
		if !strings.Contains(body, llmproxycontract.ErrorCodeStructuredRequestStore) {
			t.Fatalf("status read lost error code: %s", body)
		}
		assertPrivateResultAbsent(t, body)
		fixture.assertResultUnchanged(t, before)
	}
	if err := callback.Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	if failures.Load() != 2 {
		t.Fatalf("status read failures=%d want=2", failures.Load())
	}
	hostedIdentityStatusHTTP(t, fixture.generation, publicationRecoveryKey, http.StatusOK)
	fixture.assertResultUnchanged(t, before)
	fixture.recover(t, "receipt")
}

type hostedResultReadFailureDialector struct {
	gorm.Dialector
	failures *atomic.Int64
	change   func(*gorm.DB)
}

func (dialector hostedResultReadFailureDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	var armed atomic.Bool
	const callbackName = "test:hosted_result_recovery_read"
	if err := database.Callback().Update().After("gorm:update").Register(callbackName, func(transaction *gorm.DB) {
		if !transaction.DryRun && transaction.Statement.Table == "managed_journal_request_records" && strings.Contains(transaction.Statement.SQL.String(), "result_published_at IS NULL") {
			armed.Store(true)
		}
	}); err != nil {
		return err
	}
	return database.Callback().Query().After("gorm:query").Register(callbackName, func(transaction *gorm.DB) {
		if !transaction.DryRun && transaction.Statement.Table == "managed_journal_request_records" && strings.Contains(transaction.Statement.SQL.String(), "WHERE id =") && armed.CompareAndSwap(true, false) {
			dialector.failures.Add(1)
			dialector.change(transaction)
		}
	})
}

func TestHostedResultReadStartupFailureKeepsUnpublishedReceipt(t *testing.T) {
	fixture := newPublicationRecoveryFixture(t, "receipt")
	configuration, _ := hostedRecoveryApplicationConfiguration(t, fixture.fundsAdmissionFixture)
	before := fixture.resultState(t)
	var failures atomic.Int64
	dialector := fixture.database.database.Dialector
	configuration.Management.DatabaseDialector = hostedResultReadFailureDialector{
		Dialector: dialector, failures: &failures,
		change: func(tx *gorm.DB) { tx.AddError(errors.New("controlled_result_recovery_read_failure")) },
	}
	// An occupied port bounds Serve if startup incorrectly reaches the listener.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	configuration.Port = listener.Addr().(*net.TCPAddr).Port
	for range 2 {
		if err := Serve(configuration, zap.NewNop().Sugar()); err == nil || !strings.Contains(err.Error(), "read unpublished request") || !strings.Contains(err.Error(), "controlled_result_recovery_read_failure") {
			t.Fatalf("startup result read error=%v", err)
		}
		fixture.assertResultUnchanged(t, before)
	}
	if failures.Load() != 2 {
		t.Fatalf("startup result read failures=%d want=2", failures.Load())
	}
	configuration.Management.DatabaseDialector = dialector
	replayHostedRecoveryApplication(t, configuration, newManagedTenantStore, publicationRecoveryKey, http.StatusOK)
	fixture.recover(t, "receipt")
}

func TestHostedResultReadExpiryDeleteFailureKeepsSettlement(t *testing.T) {
	fixture := newPublicationRecoveryFixture(t, "receipt")
	fixture.recover(t, "receipt")
	before := fixture.resultState(t)
	original, err := os.ReadFile(fixture.resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var record structuredRequestRecord
	if err := json.Unmarshal(original, &record); err != nil {
		t.Fatal(err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, record.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	fixture.responses.mu.Lock()
	fixture.responses.now = func() time.Time { return updatedAt.Add(fixture.responses.retention + time.Second) }
	fixture.responses.mu.Unlock()
	remove := structuredRequestRemove
	var failures atomic.Int64
	var armed atomic.Bool
	armed.Store(true)
	structuredRequestRemove = func(path string) error {
		if armed.Load() && path == fixture.resultPath {
			failures.Add(1)
			return errors.New("controlled_result_expiry_failure")
		}
		return remove(path)
	}
	t.Cleanup(func() {
		fixture.generation.Close()
		structuredRequestRemove = remove
	})
	for range 2 {
		body := hostedIdentityStatusHTTP(t, fixture.generation, publicationRecoveryKey, http.StatusInternalServerError)
		if !strings.Contains(body, llmproxycontract.ErrorCodeStructuredRequestStore) {
			t.Fatalf("failed expiry lost status error code: %s", body)
		}
		assertPrivateResultAbsent(t, body)
		assertPrivateResultAbsent(t, hostedIdentityHTTP(t, fixture.generation, publicationRecoveryKey, "funded prompt", http.StatusBadGateway))
		fixture.assertResultUnchanged(t, before)
	}
	if failures.Load() != 4 {
		t.Fatalf("expiry delete failures=%d want=4", failures.Load())
	}
	retained, err := os.ReadFile(fixture.resultPath)
	if err != nil || string(retained) != string(original) {
		t.Fatalf("failed expiry changed the saved result: %v", err)
	}
	armed.Store(false)
	for iteration := range 3 {
		if iteration > 0 {
			fixture.generation.Close()
			fixture.generation = newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, fixture.database), fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices))
		}
		body := hostedIdentityStatusHTTP(t, fixture.generation, publicationRecoveryKey, http.StatusGone)
		if !strings.Contains(body, llmproxycontract.ErrorCodeHostedResultExpired) {
			t.Fatalf("expiry lost its error code: %s", body)
		}
		hostedIdentityHTTP(t, fixture.generation, publicationRecoveryKey, "funded prompt", http.StatusGone)
		fixture.assertResultUnchanged(t, before)
	}
}

func TestHostedResultReadAfterReceiptRecoveryPreservesPublishedResult(t *testing.T) {
	fixture := newPublicationRecoveryFixture(t, "receipt")
	before := fixture.state(t)
	var failures atomic.Int64
	callback := fixture.database.database.Callback().Query()
	const name = "test:recovered_result_read"
	if err := callback.After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "managed_journal_request_records" && strings.Contains(tx.Statement.SQL.String(), "billing_account_id = ? AND id = ?") {
			failures.Add(1)
			tx.AddError(errors.New("controlled_recovered_result_read_failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = callback.Remove(name) })
	assertPrivateResultAbsent(t, hostedIdentityHTTP(t, fixture.generation, publicationRecoveryKey, "funded prompt", http.StatusBadGateway))
	if err := callback.Remove(name); err != nil {
		t.Fatal(err)
	}
	if failures.Load() != 1 || !reflect.DeepEqual(before, fixture.state(t)) || fixture.calls.Load() != 1 {
		t.Fatalf("failed result read changed financial effects: failures=%d calls=%d", failures.Load(), fixture.calls.Load())
	}
	var request managedJournalRequestRecord
	if err := fixture.database.database.Where("id = ?", fixture.request.ID).First(&request).Error; err != nil {
		t.Fatal(err)
	}
	if request.ResultPublishedAt == nil || request.State != journalRequestCompleted {
		t.Fatal("failed read discarded the recovered publication receipt")
	}
	fixture.recover(t, "receipt")
}

type hostedResultAuthorityReadFailureDialector struct {
	gorm.Dialector
	failures *atomic.Int64
}

func (dialector hostedResultAuthorityReadFailureDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	return database.Callback().Query().After("gorm:query").Register("test:result_authority_read", func(tx *gorm.DB) {
		if tx.Statement.Table == "managed_journal_request_records" && strings.Contains(tx.Statement.SQL.String(), "execution_kind = ? AND execution_id = ?") {
			dialector.failures.Add(1)
			tx.AddError(errors.New("controlled_result_authority_read_failure"))
		}
	})
}

func TestHostedResultStartupAuthorityFailuresPreservePublishedResult(t *testing.T) {
	for _, scenario := range []string{"read", "identity"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPublicationRecoveryFixture(t, "receipt")
			fixture.recover(t, "receipt")
			before := fixture.resultState(t)
			original, err := os.ReadFile(fixture.resultPath)
			if err != nil {
				t.Fatal(err)
			}
			configuration, _ := hostedRecoveryApplicationConfiguration(t, fixture.fundsAdmissionFixture)
			dialector := fixture.database.database.Dialector
			var failures atomic.Int64
			expected := "read hosted result authority"
			if scenario == "read" {
				configuration.Management.DatabaseDialector = hostedResultAuthorityReadFailureDialector{dialector, &failures}
			} else {
				var record structuredRequestRecord
				if err := json.Unmarshal(original, &record); err != nil {
					t.Fatal(err)
				}
				record.IntentSHA256 = sha256Hex("unrelated-intent")
				encoded, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				writeHostedRecoverySignal(t, fixture.resultPath, string(encoded))
				expected = errUsageJournalConflict.Error()
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			configuration.Port = listener.Addr().(*net.TCPAddr).Port
			for range 2 {
				if err := Serve(configuration, zap.NewNop().Sugar()); err == nil || !strings.Contains(err.Error(), expected) {
					t.Fatalf("startup error=%v want=%s", err, expected)
				}
				fixture.assertResultUnchanged(t, before)
			}
			if scenario == "read" && failures.Load() != 2 {
				t.Fatalf("authority read failures=%d", failures.Load())
			}
			configuration.Management.DatabaseDialector = dialector
			writeHostedRecoverySignal(t, fixture.resultPath, string(original))
			replayHostedRecoveryApplication(t, configuration, newManagedTenantStore, publicationRecoveryKey, http.StatusOK)
			fixture.recover(t, "receipt")
		})
	}
}
