package proxy

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

func TestHostedFundsRecoveryReleasesKnownUndispatchedFailure(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	root := t.TempDir()
	first := newHostedIdentityHTTPServer(t, database, upstream.URL, root, fundsDependencies(prices), func(dependencies *hostedTextRequestDependencies) {
		dependencies.responses.publish = func(string, structuredRequestRecord) error {
			return errors.New("controlled pre-dispatch response failure")
		}
	})
	hostedIdentityHTTP(t, first, "undispatched", "funded prompt", http.StatusBadGateway)
	assertHostedFundsBalance(t, database, 5, 2)
	first.Close()
	second := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, fundsDependencies(prices))
	assertHostedFundsBalance(t, database, 5, 5)
	hostedIdentityHTTP(t, second, "undispatched", "funded prompt", http.StatusBadGateway)
	if calls.Load() != 0 {
		t.Fatalf("released request dispatched %d times", calls.Load())
	}
	var reservation managedFundsReservationRecord
	if err := database.database.First(&reservation).Error; err != nil {
		t.Fatal(err)
	}
	if reservation.State != "released" || reservation.Revision != 2 {
		t.Fatalf("undispatched hold=%+v", reservation)
	}
}

func TestHostedFundsRecoveryDeliversKnownCompletionOnce(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	root := t.TempDir()
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, root, fundsDependencies(prices))
	hostedIdentityHTTP(t, server, "completed-recovery", "funded prompt", http.StatusOK)
	assertHostedFundsBalance(t, database, 5, 2)
	server.Close()
	for range 2 {
		server = newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, fundsDependencies(prices))
		hostedIdentityHTTP(t, server, "completed-recovery", "funded prompt", http.StatusOK)
		assertHostedFundsBalance(t, database, 5, 5)
		server.Close()
	}
	var financial managedFundsAccountRecord
	if err := database.database.First(&financial).Error; err != nil {
		t.Fatal(err)
	}
	if financial.RemainderNumerator != "91" || financial.RemainderDenominator != "25000" || calls.Load() != 1 {
		t.Fatalf("replayed settlement=%+v calls=%d", financial, calls.Load())
	}
}

func TestHostedFundsRecoveryRetainsUncertainDispatch(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if _, err := io.Copy(io.Discard, request.Body); err != nil {
			t.Error(err)
			return
		}
		connection, _, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		connection.Close()
	}))
	t.Cleanup(upstream.Close)
	root := t.TempDir()
	first := newHostedIdentityHTTPServer(t, database, upstream.URL, root, fundsDependencies(prices))
	hostedIdentityHTTP(t, first, "uncertain-recovery", "funded prompt", http.StatusBadGateway)
	first.Close()
	for range 2 {
		second := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, fundsDependencies(prices), func(dependencies *hostedTextRequestDependencies) {
			dependencies.now = func() time.Time { return ratingTestAcceptanceTime().Add(365 * 24 * time.Hour) }
		})
		hostedIdentityHTTP(t, second, "uncertain-recovery", "funded prompt", http.StatusConflict)
		assertHostedFundsBalance(t, database, 5, 2)
		second.Close()
	}
	var reservation managedFundsReservationRecord
	if err := database.database.First(&reservation).Error; err != nil {
		t.Fatal(err)
	}
	if reservation.State != fundsReservationReconciliation || reservation.Revision != 2 || calls.Load() != 1 {
		t.Fatalf("uncertain hold=%+v calls=%d", reservation, calls.Load())
	}
}

func TestHostedFundsRecoveryFencesExpiredUndispatchedWorker(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	entered, release := make(chan struct{}), make(chan struct{})
	var stopped atomic.Bool
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	if err := database.database.Callback().Query().After("gorm:query").Register("funds_recovery_pre_dispatch", func(transaction *gorm.DB) {
		if transaction.Statement.Table == "managed_platform_credential_records" && hostedTextExecutionFromContext(transaction.Statement.Context) != nil && stopped.CompareAndSwap(false, true) {
			close(entered)
			select {
			case <-release:
			case <-t.Context().Done():
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	first := newHostedIdentityHTTPServer(t, database, upstream.URL, root, fundsDependencies(prices))
	t.Cleanup(unblock)
	request, err := http.NewRequest(http.MethodPost, first.URL+"/?provider=openai&model=gpt-4.1", strings.NewReader(`{"prompt":"funded prompt"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "expired-undispatched")
	finished := make(chan error, 1)
	go func() {
		response, err := first.Client().Do(request)
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil {
				err = readErr
			} else if response.StatusCode != http.StatusConflict {
				err = fmt.Errorf("unfenced worker: status=%d body=%s", response.StatusCode, body)
			}
		}
		finished <- err
	}()
	select {
	case <-entered:
	case err := <-finished:
		t.Fatalf("worker did not reach credential load: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not reach credential load")
	}
	var original managedJournalRequestRecord
	if err := database.database.Where("key_digest = ?", sha256Hex("expired-undispatched")).First(&original).Error; err != nil {
		t.Fatal(err)
	}
	live := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, fundsDependencies(prices))
	assertHostedFundsBalance(t, database, 5, 2)
	hostedIdentityStatusHTTP(t, live, "expired-undispatched", http.StatusAccepted)
	restarted := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, fundsDependencies(prices), func(dependencies *hostedTextRequestDependencies) {
		dependencies.now = func() time.Time { return original.ClaimExpiresAt.Add(time.Second) }
	})
	assertHostedFundsBalance(t, database, 5, 5)
	unblock()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	hostedIdentityHTTP(t, restarted, "expired-undispatched", "funded prompt", http.StatusBadGateway)
	if calls.Load() != 0 {
		t.Fatalf("expired reservation permitted %d provider calls", calls.Load())
	}
	var current managedJournalRequestRecord
	if err := database.database.Where("id = ?", original.ID).First(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.State != journalRequestFailed || current.FailureCode != fundsUndispatchedExpired {
		t.Fatalf("expired journal: state=%s failure=%s", current.State, current.FailureCode)
	}
}

func TestHostedFundsRecoveryWriteFailureRollsBackAndStopsStartup(t *testing.T) {
	for _, scenario := range []string{"release", "settlement"} {
		t.Run(scenario, func(t *testing.T) {
			database, _, _, prices := newHostedRatingFixture(t)
			seedHostedFunds(t, database, 5)
			var calls atomic.Int64
			upstream := fundsUpstream(t, &calls)
			var saved hostedTextRequestDependencies
			root := t.TempDir()
			first := newHostedIdentityHTTPServer(t, database, upstream.URL, root, fundsDependencies(prices), func(dependencies *hostedTextRequestDependencies) {
				if scenario == "release" {
					dependencies.responses.publish = func(string, structuredRequestRecord) error {
						return errors.New("controlled pre-dispatch response failure")
					}
				}
				saved = *dependencies
			})
			status, table, operation := http.StatusOK, "managed_funds_settlement_records", "INSERT"
			if scenario == "release" {
				status, table, operation = http.StatusBadGateway, "managed_funds_reservation_records", "UPDATE"
			}
			hostedIdentityHTTP(t, first, "recovery-failure", "funded prompt", status)
			first.Close()
			statement := "CREATE TRIGGER reject_funds_recovery BEFORE " + operation + " ON " + table + " BEGIN SELECT RAISE(ABORT, 'controlled_recovery_write_failure'); END"
			if err := database.database.Exec(statement).Error; err != nil {
				t.Fatal(err)
			}
			saved.database = openJournalTransactionInstance(t, database)
			if _, err := newHostedTextRequests(t.Context(), saved); err == nil || !strings.Contains(err.Error(), "controlled_recovery_write_failure") {
				t.Fatalf("startup ignored financial recovery failure: %v", err)
			}
			assertHostedFundsBalance(t, database, 5, 2)
			var count int64
			if err := database.database.Model(&managedFundsSettlementRecord{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("partial settlements=%d error=%v", count, err)
			}
			if err := database.database.Exec("DROP TRIGGER reject_funds_recovery").Error; err != nil {
				t.Fatal(err)
			}
			restarted := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root, fundsDependencies(prices))
			hostedIdentityHTTP(t, restarted, "recovery-failure", "funded prompt", status)
			assertHostedFundsBalance(t, database, 5, 5)
			wantCalls := int64(1)
			if scenario == "release" {
				wantCalls = 0
			}
			if calls.Load() != wantCalls {
				t.Fatalf("recovery caused provider work: %d", calls.Load())
			}
		})
	}
}
