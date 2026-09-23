package proxy

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
)

func deliverFundsFixture(t *testing.T, database *gormManagedTenantDatabase) error {
	t.Helper()
	observations, err := database.pendingJournalDeliveries(t.Context(), 100)
	if err != nil {
		return err
	}
	for _, observation := range observations {
		for range 2 {
			if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.ObservedAt, deliverHostedFunds); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestHostedFundsSettlementCarriesExactRemainderAcrossRestart(t *testing.T) {
	database, _, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	for index := range 3 {
		hostedIdentityHTTP(t, server, fmt.Sprintf("settlement-%d", index), "funded prompt", http.StatusOK)
		worker := openJournalTransactionInstance(t, database)
		if err := deliverFundsFixture(t, worker); err != nil {
			t.Fatal(err)
		}
		want := int64(5)
		if index == 2 {
			want = 4
		}
		assertHostedFundsBalance(t, database, want, want)
	}
	var financial managedFundsAccountRecord
	if err := database.database.First(&financial).Error; err != nil {
		t.Fatal(err)
	}
	// Three charges of USD 0.00364 settle one cent and retain USD 0.00092.
	if financial.RemainderNumerator != "23" || financial.RemainderDenominator != "25000" {
		t.Fatalf("lost fractional charges: %+v", financial)
	}
	var reservations []managedFundsReservationRecord
	if err := database.database.Find(&reservations).Error; err != nil || len(reservations) != 3 {
		t.Fatalf("reservations=%v error=%v", reservations, err)
	}
	for _, reservation := range reservations {
		if reservation.State != "settled" || reservation.Revision != 2 {
			t.Fatalf("reservation not settled once: %+v", reservation)
		}
		response := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+reservation.RequestID+"/charge-summary", "", http.StatusOK)
		if response["state"] != "rated" {
			t.Fatalf("missing settled charge: %v", response)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("settlement dispatched extra provider work: %d", calls.Load())
	}
	account, err := newHostedLedgerAccount(database.database, "billing-journal", ratingTestAcceptanceTime())
	if err != nil {
		t.Fatal(err)
	}
	entries, err := account.service.ListEntries(t.Context(), account.tenant, account.user, account.namespace, ratingTestAcceptanceTime().Unix()+1, 100, ledger.ListEntriesFilter{})
	if err != nil || len(entries) != 8 {
		t.Fatalf("settlement effects repeated: entries=%v error=%v", entries, err)
	}
	var holdDelta, totalDelta int64
	for _, entry := range entries {
		totalDelta += entry.AmountCents().Int64()
		if entry.Type() == ledger.EntryHold || entry.Type() == ledger.EntryReverseHold {
			holdDelta += entry.AmountCents().Int64()
		}
	}
	if holdDelta != 0 || totalDelta != 4 {
		t.Fatalf("ledger conservation failed: holds=%d total=%d", holdDelta, totalDelta)
	}
}

func TestHostedFundsSettlementRetainsUnresolvedUsage(t *testing.T) {
	for _, scenario := range []struct{ name, usage, reason string }{
		{"missing", `{}`, chargeUsageUnresolved},
		{"above-bound", `{"input_tokens":1001,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`, chargeLimitUnresolved},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, management, prices := newHostedRatingFixture(t)
			seedHostedFunds(t, database, 5)
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodDelete {
					writer.WriteHeader(http.StatusNoContent)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(writer, `{"id":"unresolved-result","status":"completed","output_text":"funded result","usage":`+scenario.usage+`}`)
			}))
			t.Cleanup(upstream.Close)
			server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
			hostedIdentityHTTP(t, server, "unresolved-settlement", "funded prompt", http.StatusOK)
			if err := deliverFundsFixture(t, database); err != nil {
				t.Fatal(err)
			}
			assertHostedFundsBalance(t, database, 5, 2)
			balance := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
			if balance["pending_cents"] != "3" || balance["reserved_cents"] != "3" || balance["available_cents"] != "2" {
				t.Fatalf("reconciliation funds counted incorrectly: %v", balance)
			}
			var reservation managedFundsReservationRecord
			if err := database.database.First(&reservation).Error; err != nil {
				t.Fatal(err)
			}
			if reservation.State != fundsReservationReconciliation || reservation.Revision != 2 {
				t.Fatalf("uncertain hold=%+v", reservation)
			}
			cases := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+reservation.RequestID+"/reconciliation-cases", "", http.StatusOK)
			if len(cases["cases"].([]any)) == 0 {
				t.Fatalf("missing financial review case: %v", cases)
			}
		})
	}
}

func TestHostedFundsSettlementSerializesAccountRemainder(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 10)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	for index := range 3 {
		hostedIdentityHTTP(t, server, fmt.Sprintf("concurrent-settlement-%d", index), "funded prompt", http.StatusOK)
	}
	assertHostedFundsBalance(t, database, 10, 1)
	observations, err := database.pendingJournalDeliveries(t.Context(), 100)
	if err != nil || len(observations) != 3 {
		t.Fatalf("pending=%v error=%v", observations, err)
	}
	start := make(chan struct{})
	var group sync.WaitGroup
	for _, observation := range observations {
		worker := openJournalTransactionInstance(t, database)
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			if err := worker.deliverJournalObservation(t.Context(), observation.ID, observation.ObservedAt, deliverHostedFunds); err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	group.Wait()
	assertHostedFundsBalance(t, database, 9, 9)
	var financial managedFundsAccountRecord
	if err := database.database.First(&financial).Error; err != nil {
		t.Fatal(err)
	}
	if financial.RemainderNumerator != "23" || financial.RemainderDenominator != "25000" {
		t.Fatalf("concurrent remainder=%+v", financial)
	}
}

func TestHostedFundsSettlementWaitsForEveryAttempt(t *testing.T) {
	database, intent, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	reserve := newHostedFundsAdmission(prices)
	quantities := []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "1000"}}
	request, first := observeRatedFixture(t, database, intent("multi-attempt-funds"), reserve, journalOutcomeContinue, quantities)
	claim, err := newJournalWorkerClaim(request.ID, request.OwnerToken, request.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-22222222222222222222222222222222", reserve)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.dispatchJournalAttempt(t.Context(), claim, second.ID); err != nil {
		t.Fatal(err)
	}
	evidence, err := newJournalUsageEvidence(journalUsageEvidenceInput{AttemptID: second.ID, AdapterRevision: "test-native-meter", Quantities: quantities, Outcome: journalOutcomeComplete, ObservedAt: claim.now}, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	last, err := database.observeJournalAttempt(t.Context(), claim, evidence)
	if err != nil {
		t.Fatal(err)
	}
	publishRatingSummaryResult(t, database, request)
	// The final attempt arrives before the earlier usage delivery.
	if err := database.deliverJournalObservation(t.Context(), last.ID, last.ObservedAt, deliverHostedFunds); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 5, 2)
	for range 2 {
		if err := database.deliverJournalObservation(t.Context(), first.ID, first.ObservedAt, deliverHostedFunds); err != nil {
			t.Fatal(err)
		}
	}
	assertHostedFundsBalance(t, database, 3, 3)
	publishRatingSummaryResult(t, database, request)
	summary := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+request.ID+"/charge-summary", "", http.StatusOK)
	if summary["attempt_count"] != float64(2) || summary["charge_count"] != float64(2) || summary["state"] != "rated" {
		t.Fatalf("incomplete aggregate: %v", summary)
	}
}

func TestHostedFundsSettlementFailureRetainsHoldAndPendingDelivery(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	hostedIdentityHTTP(t, server, "settlement-failure", "funded prompt", http.StatusOK)
	if err := database.database.Exec("CREATE TRIGGER reject_funds_settlement BEFORE UPDATE ON managed_funds_reservation_records WHEN NEW.state = 'settled' BEGIN SELECT RAISE(ABORT, 'controlled_settlement_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := deliverFundsFixture(t, database); err == nil {
		t.Fatal("settlement failure was ignored")
	}
	assertHostedFundsBalance(t, database, 5, 2)
	var charges int64
	if err := database.database.Model(&managedChargeRecord{}).Count(&charges).Error; err != nil || charges != 0 {
		t.Fatalf("rolled back charges=%d error=%v", charges, err)
	}
	if err := database.database.Exec("DROP TRIGGER reject_funds_settlement").Error; err != nil {
		t.Fatal(err)
	}
	if err := deliverFundsFixture(t, openJournalTransactionInstance(t, database)); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 5, 5)
}
