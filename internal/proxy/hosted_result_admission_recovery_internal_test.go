package proxy

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostedResultAdmissionRejectsTerminalFileForAcceptedRequest(t *testing.T) {
	for _, state := range []string{structuredRequestStateSucceeded, structuredRequestStateFailed} {
		t.Run(state, func(t *testing.T) {
			fixture := newJournalInterruptionFixture(t, "accepted")
			path := filepath.Join(fixture.responseRoot, structuredRequestDirectoryName, sha256Hex(fixture.request.TenantID), fixture.request.KeyDigest+".json")
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var record structuredRequestRecord
			if err := json.Unmarshal(original, &record); err != nil {
				t.Fatal(err)
			}
			record.State = state
			record.CompletedAt = fixture.request.CreatedAt.Format(time.RFC3339Nano)
			if state == structuredRequestStateSucceeded {
				record.StatusCode = http.StatusOK
				record.Result = json.RawMessage(`{"text":"unrelated-terminal-result"}`)
			} else {
				record.StatusCode = http.StatusBadGateway
				record.FailureCode = "unrelated-terminal-failure"
			}
			encoded, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			writeHostedRecoverySignal(t, path, string(encoded))
			assertPrivateResultAbsent(t, hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", http.StatusConflict))
			before := fixture.resources(t)
			for range 2 {
				assertPrivateResultAbsent(t, hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", http.StatusBadGateway))
				assertPrivateResultAbsent(t, hostedIdentityStatusHTTP(t, fixture.generation, textExecutionRecoveryKey, http.StatusBadGateway))
				retained, err := os.ReadFile(path)
				if err != nil || string(retained) != string(encoded) || fixture.calls.Load() != 0 || !reflect.DeepEqual(before, fixture.resources(t)) {
					t.Fatalf("terminal file conflict changed the result or financial effects: calls=%d error=%v", fixture.calls.Load(), err)
				}
			}
			writeHostedRecoverySignal(t, path, string(original))
			fixture.generation.Close()
			recoverHostedTextExecution(t, fixture.fundsAdmissionFixture, fixture.request, 0)
			assertHostedFundsBalance(t, fixture.database, 5, 5)
			assertFundsCreditRemainder(t, fixture.database, "0", "1")
		})
	}
}

func TestHostedResultAdmissionStorageFailuresPreventProviderDispatch(t *testing.T) {
	for _, scenario := range []string{"root-directory", "tenant-directory", "read", "initial-publication", "saved-identity"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			fixture.generation.Close()
			var responses *structuredRequestStore
			fixture.generation = newHostedIdentityHTTPServer(t, fixture.database, fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices), func(dependencies *hostedTextRequestDependencies) {
				responses = dependencies.responses
			})
			var failures atomic.Int64
			restore := func() {}
			initialStatus := http.StatusBadGateway
			var retainedPath string
			var retainedBody []byte
			switch scenario {
			case "root-directory":
				saved := fixture.responseRoot + "-saved"
				if err := os.Rename(fixture.responseRoot, saved); err != nil {
					t.Fatal(err)
				}
				writeHostedRecoverySignal(t, fixture.responseRoot, "blocked response root")
				restore = func() {
					if err := os.Remove(fixture.responseRoot); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(saved, fixture.responseRoot); err != nil {
						t.Fatal(err)
					}
				}
			case "tenant-directory":
				if err := os.Rename(t.TempDir(), responses.root); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(responses.root, sha256Hex("managed-first"))
				writeHostedRecoverySignal(t, path, "blocked tenant directory")
				restore = func() {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
			case "saved-identity":
				source := newFundsAdmissionFixture(t)
				hostedIdentityHTTP(t, source.generation, textExecutionRecoveryKey, "funded prompt", http.StatusOK)
				source.generation.Close()
				tenantDigest := sha256Hex("managed-first")
				name := sha256Hex(textExecutionRecoveryKey) + ".json"
				var err error
				retainedBody, err = os.ReadFile(filepath.Join(source.responseRoot, structuredRequestDirectoryName, tenantDigest, name))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(t.TempDir(), responses.root); err != nil {
					t.Fatal(err)
				}
				tenantPath := filepath.Join(responses.root, tenantDigest)
				if err := os.Rename(t.TempDir(), tenantPath); err != nil {
					t.Fatal(err)
				}
				retainedPath = filepath.Join(tenantPath, name)
				writeHostedRecoverySignal(t, retainedPath, string(retainedBody))
				initialStatus = http.StatusConflict
				restore = func() {
					if err := os.Remove(retainedPath); err != nil {
						t.Fatal(err)
					}
				}
			case "read":
				read := responses.read
				responses.read = func(string) (structuredRequestRecord, error) {
					failures.Add(1)
					return structuredRequestRecord{}, errors.New("controlled_initial_result_read_failure")
				}
				restore = func() { responses.read = read }
			case "initial-publication":
				publish := responses.publish
				responses.publish = func(string, structuredRequestRecord) error {
					failures.Add(1)
					return errors.New("controlled_initial_result_publication_failure")
				}
				restore = func() { responses.publish = publish }
			}
			request := failHostedTextExecutionStatus(t, fixture, 0, initialStatus)
			before := fixture.state(t)
			for range 2 {
				assertPrivateResultAbsent(t, hostedIdentityStatusHTTP(t, fixture.generation, textExecutionRecoveryKey, http.StatusBadGateway))
				assertPrivateResultAbsent(t, hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", http.StatusBadGateway))
				if fixture.calls.Load() != 0 || !reflect.DeepEqual(before, fixture.state(t)) {
					t.Fatal("failed initial result storage changed funds or dispatched provider work")
				}
			}
			if (scenario == "read" || scenario == "initial-publication") && failures.Load() != 1 {
				t.Fatalf("result storage failures=%d want=1", failures.Load())
			}
			for _, model := range []any{&managedJournalAttemptRecord{}, &managedJournalObservationRecord{}, &managedChargeRecord{}} {
				var count int64
				if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("failed initial result storage retained paid work: %T count=%d error=%v", model, count, err)
				}
			}
			if retainedPath != "" {
				body, err := os.ReadFile(retainedPath)
				if err != nil || string(body) != string(retainedBody) {
					t.Fatalf("rejected request changed the existing result: %v", err)
				}
			}
			fixture.generation.Close()
			restore()
			recoverHostedTextExecution(t, fixture, request, 0)
			server := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, fixture.database), fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices))
			if body := hostedIdentityHTTP(t, server, "restored-result-store", "funded prompt", http.StatusOK); body != "funded result" {
				t.Fatalf("restored result=%q", body)
			}
			server.Close()
			server = newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, fixture.database), fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices))
			hostedIdentityHTTP(t, server, textExecutionRecoveryKey, "funded prompt", http.StatusBadGateway)
			hostedIdentityHTTP(t, server, "restored-result-store", "funded prompt", http.StatusOK)
			assertHostedFundsBalance(t, fixture.database, 5, 5)
			assertFundsCreditRemainder(t, fixture.database, "91", "25000")
			if fixture.calls.Load() != 1 {
				t.Fatalf("storage recovery repeated provider work: %d", fixture.calls.Load())
			}
		})
	}
}
