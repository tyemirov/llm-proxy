package proxy

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedMediaCredentialReadFailuresPreventProviderCalls(t *testing.T) {
	for _, scenario := range []struct{ name, fields string }{
		{"read", ""},
		{"malformed", "{"},
		{"unknown-field", `{"unknown":"controlled"}`},
		{"ciphertext", `{"api_key":"invalid-ciphertext"}`},
		{"missing-fields", `{}`},
		{"null-fields", `null`},
		{"foreign-connection", ""},
		{"foreign-version", ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundedMediaRecoveryFixture(t)
			fields := []byte(scenario.fields)
			if scenario.name == "foreign-connection" || scenario.name == "foreign-version" {
				reference := platformCredentialReference("platform-foreign", 1)
				if scenario.name == "foreign-version" {
					reference = platformCredentialReference("platform-journal", 2)
				}
				ciphertext, err := internalManagedProviderKeyCipher().encryptConnection(rand.Reader, reference, "openai", CatalogCredentialAPIKey, "sk-platform-pinned")
				if err != nil {
					t.Fatal(err)
				}
				fields, err = json.Marshal(map[string]string{CatalogCredentialAPIKey: ciphertext})
				if err != nil {
					t.Fatal(err)
				}
			}
			before := fixture.state(t)
			var dispatched atomic.Bool
			var failures atomic.Int64
			updates := fixture.database.database.Callback().Update()
			queries := fixture.database.database.Callback().Query()
			const dispatchCallback = "test:credential_media_dispatch"
			const credentialCallback = "test:credential_media_read"
			if err := updates.After("gorm:update").Register(dispatchCallback, func(tx *gorm.DB) {
				if tx.Error == nil && tx.Statement.Table == "media_operation_records" {
					if values, ok := tx.Statement.Dest.(map[string]any); ok && values["provider_execution_state"] == MediaProviderExecutionDispatched {
						dispatched.Store(true)
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = updates.Remove(dispatchCallback) })
			if err := queries.After("gorm:query").Register(credentialCallback, func(tx *gorm.DB) {
				if tx.Error != nil || tx.DryRun || tx.Statement.Table != "managed_platform_credential_records" || !dispatched.CompareAndSwap(true, false) {
					return
				}
				failures.Add(1)
				if scenario.name == "read" {
					tx.AddError(errors.New("controlled_media_credential_read_failure"))
					return
				}
				tx.Statement.Dest.(*managedPlatformCredentialRecord).Fields = fields
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = queries.Remove(credentialCallback) })
			fixture.worker.runOperation("credential-read-worker", fixture.operationID)
			if err := queries.Remove(credentialCallback); err != nil {
				t.Fatal(err)
			}
			if err := updates.Remove(dispatchCallback); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 {
				t.Fatalf("credential read failures=%d want=1", failures.Load())
			}
			fixture.assertFailure(t, MediaOperationStateFailed, 0)
			if !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatal("credential rejection changed financial resources")
			}
			var previous map[string]any
			for iteration := 0; iteration < 2; iteration++ {
				database := openJournalTransactionInstance(t, fixture.database)
				server, worker := newFundedMediaRecoveryWorker(t, database, fixture.upstreamURL, fixture.worker.assets)
				worker.runOperation("credential-recovery-worker", fixture.operationID)
				if err := database.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
				replay := hostedMediaAdmissionHTTP(t, server, mediaRecoveryKey, mediaRecoveryPrompt, http.StatusOK)
				if replay["operation_id"] != fixture.operationID || replay["state"] != MediaOperationStateFailed || fixture.calls.Load() != 0 {
					t.Fatalf("credential failure replay repeated work: replay=%v calls=%d", replay, fixture.calls.Load())
				}
				assertHostedFundsBalance(t, database, 500, 461)
				current := fixture.state(t)
				if iteration > 0 && !reflect.DeepEqual(previous, current) {
					t.Fatal("credential failure recovery repeated financial effects")
				}
				previous = current
				server.Close()
			}
		})
	}
}

func TestHostedTextIncompleteCredentialsPreventProviderCalls(t *testing.T) {
	for _, fields := range []string{`{}`, `null`} {
		t.Run(fields, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			var failures atomic.Int64
			queries := fixture.database.database.Callback().Query()
			const callback = "test:text_incomplete_credential"
			if err := queries.After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Error == nil && !tx.DryRun && tx.Statement.Table == "managed_platform_credential_records" && hostedTextExecutionFromContext(tx.Statement.Context) != nil {
					failures.Add(1)
					tx.Statement.Dest.(*managedPlatformCredentialRecord).Fields = []byte(fields)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = queries.Remove(callback) })
			hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", http.StatusForbidden)
			if fixture.calls.Load() != 0 {
				t.Fatal("incomplete text credentials reached the provider")
			}
			assertHostedFundsBalance(t, fixture.database, 5, 2)
			var request managedJournalRequestRecord
			if err := fixture.database.database.Where("key_digest = ?", sha256Hex(textExecutionRecoveryKey)).First(&request).Error; err != nil {
				t.Fatal(err)
			}
			if err := queries.Remove(callback); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 {
				t.Fatalf("incomplete text credential reads=%d want=1", failures.Load())
			}
			recoverHostedTextExecution(t, fixture, request, 0)
		})
	}
}
