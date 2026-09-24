package proxy

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

const publicationRecoveryKey = "publication-recovery"

type publicationRecoveryFixture struct {
	fundsAdmissionFixture
	now        *atomic.Int64
	request    managedJournalRequestRecord
	resultPath string
}

func TestHostedResultPublicationReplaysToolCallsWithoutText(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, `{"id":"funded-tool-result","status":"completed","output":[{"type":"function_call","call_id":"call_read","name":"read","arguments":"{}"}],"usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	root := t.TempDir()
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, root, fundsDependencies(prices))
	var original map[string]any
	for iteration := range 3 {
		if iteration == 2 {
			server.Close()
			server = newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, fundsDependencies(prices))
		}
		request, err := http.NewRequest(http.MethodPost, server.URL+responsesPath, strings.NewReader(`{"model":"openai/gpt-4.1","input":"read","tools":[{"type":"function","name":"read","parameters":{"type":"object","properties":{},"additionalProperties":false},"strict":true}]}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "funded-tool-replay")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("tool replay status=%d body=%s error=%v", response.StatusCode, body, err)
		}
		validateHostedIdentityResponse(t, request, response, body)
		var result map[string]any
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		if iteration == 0 {
			original = result
			output := result["output"].([]any)
			if len(output) != 1 || output[0].(map[string]any)["call_id"] != "call_read" || output[0].(map[string]any)["arguments"] != "{}" {
				t.Fatalf("unexpected function result: %s", body)
			}
		} else if !reflect.DeepEqual(original, result) {
			t.Fatalf("tool replay changed: %s", body)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("tool replay dispatched work: %d", calls.Load())
	}
	assertHostedFundsBalance(t, database, 5, 5)
	assertFundsCreditRemainder(t, database, "91", "25000")
}

func newPublicationRecoveryFixture(t *testing.T, failure string) publicationRecoveryFixture {
	t.Helper()
	fixture := newFundsAdmissionFixture(t)
	fixture.generation.Close()
	now := &atomic.Int64{}
	now.Store(ratingTestAcceptanceTime().UnixNano())
	var responses *structuredRequestStore
	fixture.generation = newHostedIdentityHTTPServer(t, fixture.database, fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices), func(dependencies *hostedTextRequestDependencies) {
		dependencies.now = func() time.Time { return time.Unix(0, now.Load()) }
		responses = dependencies.responses
		if failure == "file" {
			publish := responses.publish
			responses.publish = func(path string, record structuredRequestRecord) error {
				if record.State == structuredRequestStateSucceeded {
					return errors.New("controlled_result_publication_failure")
				}
				return publish(path, record)
			}
		}
	})
	if failure == "receipt" {
		if err := fixture.database.database.Exec("CREATE TRIGGER reject_publication_receipt BEFORE UPDATE OF result_published_at ON managed_journal_request_records BEGIN SELECT RAISE(ABORT, 'controlled_result_receipt_failure'); END").Error; err != nil {
			t.Fatal(err)
		}
	}
	body := hostedIdentityHTTP(t, fixture.generation, publicationRecoveryKey, "funded prompt", http.StatusBadGateway)
	if strings.Contains(body, "funded result") || strings.Contains(body, "controlled_") {
		t.Fatalf("publication failure exposed a result or private details: %s", body)
	}
	if failure == "receipt" {
		if err := fixture.database.database.Exec("DROP TRIGGER reject_publication_receipt").Error; err != nil {
			t.Fatal(err)
		}
	}
	var request managedJournalRequestRecord
	if err := fixture.database.database.Where("key_digest = ?", sha256Hex(publicationRecoveryKey)).First(&request).Error; err != nil {
		t.Fatal(err)
	}
	if request.State != journalRequestCompleted || request.ResultPublishedAt != nil || fixture.calls.Load() != 1 {
		t.Fatalf("invalid publication checkpoint: state=%s published=%v calls=%d", request.State, request.ResultPublishedAt, fixture.calls.Load())
	}
	path, err := responses.recordPath(sha256Hex(request.TenantID), request.KeyDigest, false)
	if err != nil {
		t.Fatal(err)
	}
	now.Store(request.ClaimExpiresAt.Add(time.Second).UnixNano())
	assertHostedFundsBalance(t, fixture.database, 5, 2)
	return publicationRecoveryFixture{fixture, now, request, path}
}

func (fixture publicationRecoveryFixture) assertRetained(t *testing.T, before map[string]any) {
	t.Helper()
	fixture.assertPending(t, before)
	var request managedJournalRequestRecord
	if err := fixture.database.database.Where("id = ?", fixture.request.ID).First(&request).Error; err != nil {
		t.Fatal(err)
	}
	if request.State != journalRequestCompleted || request.ResultPublishedAt != nil || fixture.calls.Load() != 1 {
		t.Fatal("failed publication recovery changed the journal checkpoint or dispatched work")
	}
}

func (fixture publicationRecoveryFixture) recover(t *testing.T, result string) {
	t.Helper()
	status, available, reservationState := http.StatusOK, int64(5), fundsReservationSettled
	if result == "file" {
		status, available, reservationState = http.StatusConflict, 2, fundsReservationReconciliation
	}
	var original map[string]any
	for iteration := range 2 {
		restarted := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, fixture.database), fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices), func(dependencies *hostedTextRequestDependencies) {
			dependencies.now = func() time.Time { return time.Unix(0, fixture.now.Load()) }
		})
		body := hostedIdentityHTTP(t, restarted, publicationRecoveryKey, "funded prompt", status)
		if status == http.StatusOK && body != "funded result" {
			t.Fatalf("recovered result=%q", body)
		}
		assertHostedFundsBalance(t, fixture.database, 5, available)
		if result == "receipt" {
			assertFundsCreditRemainder(t, fixture.database, "91", "25000")
		} else {
			assertFundsCreditRemainder(t, fixture.database, "0", "1")
		}
		reservation := ratingHTTPExchange(t, fixture.management, http.MethodGet, "/billing-accounts/billing-journal/reservations/"+fixture.request.ID, "", http.StatusOK)
		if reservation["state"] != string(reservationState) || fixture.calls.Load() != 1 {
			t.Fatalf("recovered publication state=%v calls=%d", reservation, fixture.calls.Load())
		}
		current := fixture.state(t)
		if iteration == 0 {
			original = current
		} else if !reflect.DeepEqual(original, current) {
			t.Fatal("publication restart repeated financial effects")
		}
		restarted.Close()
	}
}

func TestHostedResultPublicationRecoveryWriteFailuresPreserveFunds(t *testing.T) {
	for _, scenario := range []struct{ name, result, statement string }{
		{"publication-lock", "receipt", "BEFORE UPDATE OF state ON managed_journal_request_records WHEN OLD.state = 'completed' AND NEW.state = 'completed'"},
		{"publication-receipt", "receipt", "BEFORE UPDATE OF result_published_at ON managed_journal_request_records"},
		{"uncertain-result", "file", "BEFORE UPDATE OF state ON managed_journal_request_records WHEN NEW.state = 'uncertain'"},
		{"reconciliation-case", "file", "BEFORE INSERT ON managed_journal_case_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newPublicationRecoveryFixture(t, scenario.result)
			before := fixture.state(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_result_recovery " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_result_recovery_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			hostedIdentityHTTP(t, fixture.generation, publicationRecoveryKey, "funded prompt", http.StatusBadGateway)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_result_recovery").Error; err != nil {
				t.Fatal(err)
			}
			fixture.assertRetained(t, before)
			fixture.recover(t, scenario.result)
		})
	}
}

func TestHostedResultPublicationRecoveryReadFailurePreservesFunds(t *testing.T) {
	for _, result := range []string{"receipt", "file"} {
		t.Run(result, func(t *testing.T) {
			fixture := newPublicationRecoveryFixture(t, result)
			before := fixture.state(t)
			var reads, failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:result_recovery", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == "managed_journal_request_records" && reads.Add(1) == 2 {
					failures.Add(1)
					tx.AddError(errors.New("controlled_result_recovery_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			hostedIdentityHTTP(t, fixture.generation, publicationRecoveryKey, "funded prompt", http.StatusBadGateway)
			if err := callback.Remove("test:result_recovery"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 {
				t.Fatalf("recovery read failures=%d", failures.Load())
			}
			fixture.assertRetained(t, before)
			fixture.recover(t, result)
		})
	}
}

func TestHostedResultPublicationRejectsInvalidRetainedCompletion(t *testing.T) {
	for _, checkpoint := range []string{"unpublished", "published"} {
		t.Run(checkpoint, func(t *testing.T) {
			for _, scenario := range []struct{ name, result string }{
				{"unknown-field", `{"text":"funded result","unknown":true}`},
				{"array", `[]`},
				{"null", `null`},
				{"missing-text", `{}`},
				{"null-text", `{"text":null}`},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					fixture := newPublicationRecoveryFixture(t, "receipt")
					if checkpoint == "published" {
						fixture.recover(t, "receipt")
					}
					before := fixture.state(t)
					encoded, err := os.ReadFile(fixture.resultPath)
					if err != nil {
						t.Fatal(err)
					}
					var record structuredRequestRecord
					if err := json.Unmarshal(encoded, &record); err != nil {
						t.Fatal(err)
					}
					record.Result = json.RawMessage(scenario.result)
					data, err := json.Marshal(record)
					if err != nil {
						t.Fatal(err)
					}
					writeHostedRecoverySignal(t, fixture.resultPath, string(data))
					hostedIdentityStatusHTTP(t, fixture.generation, publicationRecoveryKey, http.StatusInternalServerError)
					hostedIdentityHTTP(t, fixture.generation, publicationRecoveryKey, "funded prompt", http.StatusBadGateway)
					if checkpoint == "unpublished" {
						fixture.assertRetained(t, before)
					} else if !reflect.DeepEqual(before, fixture.state(t)) || fixture.calls.Load() != 1 {
						t.Fatal("invalid published result changed funds or dispatched work")
					}
					writeHostedRecoverySignal(t, fixture.resultPath, string(encoded))
					fixture.recover(t, "receipt")
				})
			}
		})
	}
}
