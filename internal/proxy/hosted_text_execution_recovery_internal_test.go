package proxy

import (
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

const textExecutionRecoveryKey = "execution-failure"

type hostedExecutionEntropyFailure struct {
	armed      *atomic.Bool
	failures   *atomic.Int64
	calls      *atomic.Int64
	afterCalls int64
}

func (reader hostedExecutionEntropyFailure) Read(buffer []byte) (int, error) {
	if reader.armed.Load() && reader.calls.Load() == reader.afterCalls {
		reader.failures.Add(1)
		return 0, errors.New("controlled_execution_entropy_failure")
	}
	return rand.Read(buffer)
}

func TestHostedTextExecutionEntropyFailuresPreserveDispatchEvidence(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		calls int64
	}{{"attempt-identity", 0}, {"observation-identity", 1}} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			var armed atomic.Bool
			var failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			const callbackName = "test:arm_execution_entropy"
			if err := callback.After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == "managed_platform_credential_records" && hostedTextExecutionFromContext(tx.Statement.Context) != nil {
					armed.Store(true)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := callback.Remove(callbackName); err != nil {
					t.Error(err)
				}
			})
			fixture.generation = newHostedIdentityHTTPServer(t, fixture.database, fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices), func(dependencies *hostedTextRequestDependencies) {
				dependencies.entropy = hostedExecutionEntropyFailure{&armed, &failures, fixture.calls, scenario.calls}
			})
			request := failHostedTextExecution(t, fixture, scenario.calls)
			if failures.Load() != 1 {
				t.Fatalf("entropy failures=%d want=1", failures.Load())
			}
			for _, model := range []any{&managedJournalObservationRecord{}, &managedJournalDeliveryRecord{}} {
				var count int64
				if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("partial evidence %T: count=%d error=%v", model, count, err)
				}
			}
			armed.Store(false)
			recoverHostedTextExecution(t, fixture, request, scenario.calls)
		})
	}
}

func TestHostedTextPollingFailuresRetainOneDispatchAndOriginalIdentity(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		polls  int64
	}{
		{"authorization-read", http.StatusBadGateway, 0},
		{"changed-provider-identity", http.StatusConflict, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, management, prices := newHostedRatingFixture(t)
			seedHostedFunds(t, database, 5)
			calls := &atomic.Int64{}
			var polls, failures atomic.Int64
			const providerIdentity = "private-original-poll-identity"
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.Method {
				case http.MethodPost:
					calls.Add(1)
					_, _ = io.WriteString(writer, `{"id":"`+providerIdentity+`","status":"in_progress"}`)
				case http.MethodGet:
					polls.Add(1)
					identity := providerIdentity
					if scenario.name == "changed-provider-identity" {
						identity = "private-changed-poll-identity"
					}
					_, _ = io.WriteString(writer, `{"id":"`+identity+`","status":"completed","output_text":"unrelated result","usage":`+startupCompleteUsage+`}`)
				case http.MethodDelete:
					writer.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected provider method=%s", request.Method)
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			t.Cleanup(upstream.Close)
			root := t.TempDir()
			generation := newHostedIdentityHTTPServer(t, database, upstream.URL, root, fundsDependencies(prices))
			fixture := fundsAdmissionFixture{fundsStartupFixture{database, management, calls}, generation, upstream.URL, root, prices}
			if scenario.name == "authorization-read" {
				const callbackName = "test:poll_authorization_failure"
				if err := database.database.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
					role, _ := tx.Statement.Context.Value(hostedProviderRoleContextKey{}).(hostedProviderRole)
					if !tx.DryRun && tx.Statement.Table == "managed_journal_request_records" && role == hostedProviderObservation {
						failures.Add(1)
						tx.AddError(errors.New("controlled_poll_authorization_failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := database.database.Callback().Query().Remove(callbackName); err != nil {
						t.Error(err)
					}
				})
			}
			body := hostedIdentityHTTP(t, generation, textExecutionRecoveryKey, "funded prompt", scenario.status)
			if strings.Contains(body, "private-") || strings.Contains(body, "controlled_") || strings.Contains(body, "unrelated result") {
				t.Fatalf("polling failure exposed provider data: %s", body)
			}
			if scenario.name == "changed-provider-identity" && !strings.Contains(body, `"code":"usage_journal_conflict"`) {
				t.Fatalf("provider identity conflict lost its error code: %s", body)
			}
			if calls.Load() != 1 || polls.Load() != scenario.polls || (scenario.name == "authorization-read" && failures.Load() != 1) {
				t.Fatalf("polling effects: dispatches=%d polls=%d failures=%d", calls.Load(), polls.Load(), failures.Load())
			}
			var request managedJournalRequestRecord
			if err := database.database.Where("key_digest = ?", sha256Hex(textExecutionRecoveryKey)).First(&request).Error; err != nil {
				t.Fatal(err)
			}
			var attempts []managedJournalAttemptRecord
			if err := database.database.Where("request_id = ?", request.ID).Find(&attempts).Error; err != nil || len(attempts) != 1 || attempts[0].ProviderRequestID != providerIdentity {
				t.Fatalf("original dispatch identity changed: attempts=%+v error=%v", attempts, err)
			}
			for _, model := range []any{&managedJournalObservationRecord{}, &managedJournalDeliveryRecord{}, &managedFundsSettlementRecord{}} {
				var count int64
				if err := database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("partial polling effect %T: count=%d error=%v", model, count, err)
				}
			}
			assertHostedFundsBalance(t, database, 5, 2)
			assertFundsCreditRemainder(t, database, "0", "1")
			recoverHostedTextExecution(t, fixture, request, 1)
			if polls.Load() != scenario.polls {
				t.Fatal("restart repeated provider polling")
			}
		})
	}
}

func TestHostedTextPollingTransportRecoverySettlesOnlyOneDispatch(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		wantPolls int64
	}{{"normal-poll", 1}, {"truncated-poll", 2}} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, management, prices := newHostedRatingFixture(t)
			seedHostedFunds(t, database, 5)
			calls := &atomic.Int64{}
			var polls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.Method {
				case http.MethodPost:
					calls.Add(1)
					_, _ = io.WriteString(writer, `{"id":"private-recovered-poll","status":"in_progress"}`)
				case http.MethodGet:
					if polls.Add(1) == 1 && scenario.name == "truncated-poll" {
						writer.Header().Set("Content-Length", "512")
						_, _ = io.WriteString(writer, `{"id":"private-recovered-poll"`)
						return
					}
					_, _ = io.WriteString(writer, `{"id":"private-recovered-poll","status":"completed","output_text":"funded result","usage":`+startupCompleteUsage+`}`)
				case http.MethodDelete:
					writer.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected provider method=%s", request.Method)
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			t.Cleanup(upstream.Close)
			root := t.TempDir()
			generation := newHostedIdentityHTTPServer(t, database, upstream.URL, root, fundsDependencies(prices))
			if result := hostedIdentityHTTP(t, generation, textExecutionRecoveryKey, "funded prompt", http.StatusOK); result != "funded result" {
				t.Fatalf("polled result=%s", result)
			}
			fixture := fundsStartupFixture{database, management, calls}
			fixture.assertSettledOnce(t, fixture.state(t))
			for range 2 {
				restarted := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, fundsDependencies(prices))
				if result := hostedIdentityHTTP(t, restarted, textExecutionRecoveryKey, "funded prompt", http.StatusOK); result != "funded result" {
					t.Fatalf("replayed result=%s", result)
				}
			}
			if calls.Load() != 1 || polls.Load() != scenario.wantPolls {
				t.Fatalf("polling repeated work: dispatches=%d polls=%d", calls.Load(), polls.Load())
			}
		})
	}
}

func failHostedTextExecution(t *testing.T, fixture fundsAdmissionFixture, calls int64) managedJournalRequestRecord {
	t.Helper()
	return failHostedTextExecutionStatus(t, fixture, calls, http.StatusBadGateway)
}

func failHostedTextExecutionStatus(t *testing.T, fixture fundsAdmissionFixture, calls int64, status int) managedJournalRequestRecord {
	t.Helper()
	body := hostedIdentityHTTP(t, fixture.generation, textExecutionRecoveryKey, "funded prompt", status)
	if strings.Contains(body, "controlled_") || strings.Contains(body, "sk-platform") {
		t.Fatalf("execution failure disclosed private details: %s", body)
	}
	if fixture.calls.Load() != calls {
		t.Fatalf("unexpected dispatch count=%d want=%d", fixture.calls.Load(), calls)
	}
	assertHostedFundsBalance(t, fixture.database, 5, 2)
	var request managedJournalRequestRecord
	if err := fixture.database.database.Where("key_digest = ?", sha256Hex(textExecutionRecoveryKey)).First(&request).Error; err != nil {
		t.Fatal(err)
	}
	return request
}

func recoverHostedTextExecution(t *testing.T, fixture fundsAdmissionFixture, request managedJournalRequestRecord, calls int64) {
	t.Helper()
	available, status, reservationState := int64(2), http.StatusConflict, fundsReservationReconciliation
	if calls == 0 {
		available, status, reservationState = 5, http.StatusBadGateway, fundsReservationReleased
	}
	var recovered map[string]any
	for iteration := 0; iteration < 2; iteration++ {
		restarted := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, fixture.database), fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices), func(dependencies *hostedTextRequestDependencies) {
			dependencies.now = func() time.Time { return request.ClaimExpiresAt.Add(time.Second) }
		})
		hostedIdentityHTTP(t, restarted, textExecutionRecoveryKey, "funded prompt", status)
		assertHostedFundsBalance(t, fixture.database, 5, available)
		current := fixture.state(t)
		if iteration == 0 {
			recovered = current
		} else if !reflect.DeepEqual(recovered, current) {
			t.Fatal("repeated restart changed financial resources")
		}
		if fixture.calls.Load() != calls {
			t.Fatalf("restart repeated provider work: calls=%d want=%d", fixture.calls.Load(), calls)
		}
		restarted.Close()
	}
	reservation := ratingHTTPExchange(t, fixture.management, http.MethodGet, "/billing-accounts/billing-journal/reservations/"+request.ID, "", http.StatusOK)
	if reservation["state"] != string(reservationState) {
		t.Fatalf("recovered reservation=%v want=%s", reservation, reservationState)
	}
	journal := accountConnectionHTTPExchange(t, fixture.management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+request.ID, "", http.StatusOK)
	state := "uncertain"
	if calls == 0 {
		state = "failed"
	}
	if journal["state"] != state {
		t.Fatalf("recovered journal=%v want=%s", journal, state)
	}
}

func TestHostedTextExecutionWriteFailuresPreserveDispatchEvidence(t *testing.T) {
	for _, scenario := range []struct {
		name, statement string
		calls           int64
	}{
		{"attempt", "BEFORE INSERT ON managed_journal_attempt_records", 0},
		{"dispatch", "BEFORE UPDATE ON managed_journal_attempt_records WHEN NEW.state = 'dispatched'", 0},
		{"execution", "BEFORE UPDATE ON managed_journal_request_records WHEN OLD.state = 'accepted' AND NEW.state = 'executing'", 0},
		{"provider-identity", "BEFORE UPDATE ON managed_journal_attempt_records WHEN NEW.provider_request_id != OLD.provider_request_id", 1},
		{"observation", "BEFORE INSERT ON managed_journal_observation_records", 1},
		{"delivery", "BEFORE INSERT ON managed_journal_delivery_records", 1},
		{"observed-attempt", "BEFORE UPDATE ON managed_journal_attempt_records WHEN NEW.state = 'observed'", 1},
		{"observed-request", "BEFORE UPDATE ON managed_journal_request_records WHEN NEW.usage_state = 'complete'", 1},
		{"completion", "BEFORE UPDATE ON managed_journal_request_records WHEN NEW.state = 'completed'", 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_text_execution " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_text_execution_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			request := failHostedTextExecution(t, fixture, scenario.calls)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_text_execution").Error; err != nil {
				t.Fatal(err)
			}
			if scenario.name != "completion" {
				for _, model := range []any{&managedJournalObservationRecord{}, &managedJournalDeliveryRecord{}} {
					var count int64
					if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
						t.Fatalf("partial usage evidence %T: count=%d error=%v", model, count, err)
					}
				}
			}
			recoverHostedTextExecution(t, fixture, request, scenario.calls)
		})
	}
}

func TestHostedTextExecutionReadFailuresPreserveDispatchEvidence(t *testing.T) {
	for _, scenario := range []struct {
		table string
		read  int64
		calls int64
	}{
		{"managed_platform_credential_records", 1, 0},
		{"managed_journal_request_records", 1, 0},
		{"managed_hosted_grant_records", 1, 0},
		{"managed_journal_attempt_records", 1, 0},
		{"managed_journal_attempt_records", 2, 0},
		{"managed_journal_attempt_records", 1, 1},
		{"managed_journal_attempt_records", 2, 1},
		{"managed_journal_request_records", 1, 1},
		{"managed_journal_request_records", 2, 1},
		{"managed_journal_observation_records", 1, 1},
	} {
		t.Run(scenario.table+"/"+strconv.FormatInt(scenario.read, 10)+"/calls-"+strconv.FormatInt(scenario.calls, 10), func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			var reads, failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:text_execution_read", func(tx *gorm.DB) {
				if !tx.DryRun && hostedTextExecutionFromContext(tx.Statement.Context) != nil && fixture.calls.Load() == scenario.calls && tx.Statement.Table == scenario.table && reads.Add(1) == scenario.read {
					failures.Add(1)
					tx.AddError(errors.New("controlled_text_execution_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			request := failHostedTextExecution(t, fixture, scenario.calls)
			if err := callback.Remove("test:text_execution_read"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 {
				t.Fatalf("execution read failures=%d", failures.Load())
			}
			recoverHostedTextExecution(t, fixture, request, scenario.calls)
		})
	}
}

func TestHostedTextExecutionTruncatedResponsePreservesUncertainty(t *testing.T) {
	for _, scenario := range []struct{ name, statement string }{
		{"response-only", ""},
		{"uncertain-attempt", "BEFORE UPDATE ON managed_journal_attempt_records WHEN NEW.state = 'uncertain'"},
		{"reconciliation-case", "BEFORE INSERT ON managed_journal_case_records"},
		{"uncertain-request", "BEFORE UPDATE ON managed_journal_request_records WHEN NEW.state = 'uncertain'"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, management, prices := newHostedRatingFixture(t)
			seedHostedFunds(t, database, 5)
			calls := &atomic.Int64{}
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("Content-Length", "512")
				if _, err := io.WriteString(writer, `{"id":"private-truncated-result"`); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(upstream.Close)
			root := t.TempDir()
			generation := newHostedIdentityHTTPServer(t, database, upstream.URL, root, fundsDependencies(prices))
			fixture := fundsAdmissionFixture{fundsStartupFixture{database, management, calls}, generation, upstream.URL, root, prices}
			if scenario.statement != "" {
				if err := database.database.Exec("CREATE TRIGGER reject_execution_uncertainty " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_execution_uncertainty_failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			request := failHostedTextExecution(t, fixture, 1)
			if scenario.statement != "" {
				if err := database.database.Exec("DROP TRIGGER reject_execution_uncertainty").Error; err != nil {
					t.Fatal(err)
				}
			}
			recoverHostedTextExecution(t, fixture, request, 1)
		})
	}
}
