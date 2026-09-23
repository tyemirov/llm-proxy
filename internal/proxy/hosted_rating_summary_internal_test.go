package proxy

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedRatingRequestSummaryIncludesAttemptsAndCredits(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	quantities := []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}}
	request, first := observeRatedFixture(t, database, intent("request-summary"), reserve, journalOutcomeContinue, quantities)
	path := "/billing-accounts/billing-journal/requests/" + request.ID + "/charge-summary"
	checkPending := func(attempts, charges float64) {
		t.Helper()
		current := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK)
		if current["request_id"] != request.ID || current["state"] != "pending" || current["attempt_count"] != attempts || current["charge_count"] != charges {
			t.Fatalf("pending summary: %v", current)
		}
		for _, field := range []string{"provider_cost", "customer_charge", "customer_credits", "net_customer_charge"} {
			if current[field] != nil {
				t.Fatalf("incomplete request exposes a final %s: %v", field, current)
			}
		}
	}
	checkPending(1, 0)
	delivery := newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })
	if err := database.deliverJournalObservation(t.Context(), first.ID, first.CreatedAt, delivery); err != nil {
		t.Fatal(err)
	}
	checkPending(1, 1)
	claim, err := newJournalWorkerClaim(request.ID, request.OwnerToken, request.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-"+strings.Repeat("2", 32), reserve)
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
	checkPending(2, 1)
	if err := database.deliverJournalObservation(t.Context(), last.ID, last.CreatedAt, delivery); err != nil {
		t.Fatal(err)
	}
	checkPending(2, 2) // Native completion alone does not prove result publication.
	publishRatingSummaryResult(t, database, request)
	completed := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK)
	expected := map[string]any{
		"request_id": request.ID, "state": "rated", "attempt_count": float64(2), "charge_count": float64(2),
		"provider_cost":       map[string]any{"numerator": "7", "denominator": "1250"},
		"customer_charge":     map[string]any{"numerator": "91", "denominator": "12500"},
		"customer_credits":    map[string]any{"numerator": "0", "denominator": "1"},
		"net_customer_charge": map[string]any{"numerator": "91", "denominator": "12500"},
	}
	if !reflect.DeepEqual(completed, expected) {
		t.Fatalf("request totals: got=%v want=%v", completed, expected)
	}
	chargeID := chargeIDPrefix + sha256Hex(first.ID)[:32]
	credit, err := newCustomerChargeAdjustment("billing-journal", chargeID, "request-credit", "customer_credit", ExactMoney{Numerator: "91", Denominator: "50000"}, claim.now)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.applyCustomerChargeAdjustment(t.Context(), credit, func(*gorm.DB, managedChargeAdjustmentRecord) error { return nil }); err != nil {
		t.Fatal(err)
	}
	expected["customer_credits"] = map[string]any{"numerator": "91", "denominator": "50000"}
	expected["net_customer_charge"] = map[string]any{"numerator": "273", "denominator": "50000"}
	if current := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK); !reflect.DeepEqual(current, expected) {
		t.Fatalf("adjusted request totals: %v", current)
	}
	ratingHTTPExchange(t, server, http.MethodGet, strings.Replace(path, "billing-journal", "billing-foreign", 1), "", http.StatusNotFound)
	ratingHTTPExchange(t, server, http.MethodGet, strings.Replace(path, request.ID, "request-"+strings.Repeat("f", 32), 1), "", http.StatusNotFound)
}

func TestHostedRatingRequestSummaryRetainsUnresolvedCosts(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		outcome    journalObservationOutcome
		quantities []journalQuantity
		provider   any
	}{
		{"missing-usage", journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}}, nil},
		{"failed-work", journalOutcomeFail, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}}, map[string]any{"numerator": "7", "denominator": "2500"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, intent, server, reserve := newHostedRatingFixture(t)
			request, observation := observeRatedFixture(t, database, intent(scenario.name), reserve, scenario.outcome, scenario.quantities)
			if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { t.Fatal("unresolved charge settled"); return nil })); err != nil {
				t.Fatal(err)
			}
			if scenario.outcome == journalOutcomeComplete {
				publishRatingSummaryResult(t, database, request)
			}
			current := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/requests/"+request.ID+"/charge-summary", "", http.StatusOK)
			if current["state"] != "unresolved" || current["attempt_count"] != float64(1) || current["charge_count"] != float64(1) || !reflect.DeepEqual(current["provider_cost"], scenario.provider) {
				t.Fatalf("unresolved summary: %v", current)
			}
			for _, field := range []string{"customer_charge", "customer_credits", "net_customer_charge"} {
				if current[field] != nil {
					t.Fatalf("unresolved %s became a final bill: %v", field, current)
				}
			}
		})
	}
}

func publishRatingSummaryResult(t *testing.T, database *gormManagedTenantDatabase, request managedJournalRequestRecord) {
	t.Helper()
	service := &hostedTextRequests{hostedTextRequestDependencies: hostedTextRequestDependencies{database: database, now: func() time.Time { return request.CreatedAt.Add(time.Second) }}}
	if err := service.publishResponse(t.Context(), request, func(managedJournalRequestRecord) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestHostedRatingRequestSummaryIncludesEveryPage(t *testing.T) {
	const attemptCount = 101
	database, intent, server, reserve := newHostedRatingFixtureForAttempts(t, attemptCount)
	request, err := database.admitJournalRequest(t.Context(), intent("summary-pages"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	path := "/billing-accounts/billing-journal/requests/" + request.ID + "/charge-summary"
	empty := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK)
	if empty["state"] != "pending" || empty["attempt_count"] != float64(0) || empty["charge_count"] != float64(0) || empty["net_customer_charge"] != nil {
		t.Fatalf("accepted request became a zero bill: %v", empty)
	}
	claim, err := newJournalWorkerClaim(request.ID, request.OwnerToken, request.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	delivery := newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })
	for index := 0; index < attemptCount; index++ {
		attempt, err := database.prepareJournalAttempt(t.Context(), claim, fmt.Sprintf("attempt-%032x", index+1), reserve)
		if err != nil {
			t.Fatal(err)
		}
		if err := database.dispatchJournalAttempt(t.Context(), claim, attempt.ID); err != nil {
			t.Fatal(err)
		}
		outcome := journalOutcomeContinue
		if index == attemptCount-1 {
			outcome = journalOutcomeComplete
		}
		evidence, err := newJournalUsageEvidence(journalUsageEvidenceInput{AttemptID: attempt.ID, AdapterRevision: "test-native-meter", Quantities: []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1"}, {Dimension: "output_tokens", Unit: "token", Value: "1"}}, Outcome: outcome, ObservedAt: claim.now}, rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		observation, err := database.observeJournalAttempt(t.Context(), claim, evidence)
		if err != nil {
			t.Fatal(err)
		}
		if outcome == journalOutcomeComplete {
			publishRatingSummaryResult(t, database, request)
			pending := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK)
			if pending["state"] != "pending" || pending["charge_count"] != float64(100) || pending["provider_cost"] != nil || pending["net_customer_charge"] != nil {
				t.Fatalf("undelivered charge omitted from total: %v", pending)
			}
		}
		if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, delivery); err != nil {
			t.Fatal(err)
		}
	}
	current := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK)
	if current["state"] != "rated" || current["attempt_count"] != float64(attemptCount) || current["charge_count"] != float64(attemptCount) || !reflect.DeepEqual(current["provider_cost"], map[string]any{"numerator": "101", "denominator": "100000"}) || !reflect.DeepEqual(current["net_customer_charge"], map[string]any{"numerator": "1313", "denominator": "1000000"}) {
		t.Fatalf("paged request totals: %v", current)
	}
}

func TestHostedRatingRequestSummaryUsesOneCreditSnapshot(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	request, observation := observeRatedFixture(t, database, intent("summary-snapshot"), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}})
	if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })); err != nil {
		t.Fatal(err)
	}
	publishRatingSummaryResult(t, database, request)
	second := openJournalTransactionInstance(t, database)
	credit, err := newCustomerChargeAdjustment("billing-journal", chargeIDPrefix+sha256Hex(observation.ID)[:32], "concurrent-summary-credit", "customer_credit", ExactMoney{Numerator: "91", Denominator: "25000"}, observation.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	// Commit a credit through another database connection after the summary's
	// first read. Its remaining reads must retain the original snapshot.
	var once sync.Once
	const hook = "summary_concurrent_credit"
	if err := database.database.Callback().Query().After("gorm:query").Register(hook, func(transaction *gorm.DB) {
		if transaction.Statement.Table == "managed_journal_request_records" {
			once.Do(func() {
				if err := second.applyCustomerChargeAdjustment(t.Context(), credit, func(*gorm.DB, managedChargeAdjustmentRecord) error { return nil }); err != nil {
					transaction.AddError(err)
				}
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.database.Callback().Query().Remove(hook); err != nil {
			t.Error(err)
		}
	})
	path := "/billing-accounts/billing-journal/requests/" + request.ID + "/charge-summary"
	first := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK)
	if !reflect.DeepEqual(first["customer_credits"], map[string]any{"numerator": "0", "denominator": "1"}) || !reflect.DeepEqual(first["net_customer_charge"], map[string]any{"numerator": "91", "denominator": "25000"}) {
		t.Fatalf("summary mixed snapshots: %v", first)
	}
	current := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK)
	if !reflect.DeepEqual(current["customer_credits"], map[string]any{"numerator": "91", "denominator": "25000"}) || !reflect.DeepEqual(current["net_customer_charge"], map[string]any{"numerator": "0", "denominator": "1"}) {
		t.Fatalf("next summary omitted committed credit: %v", current)
	}
}
