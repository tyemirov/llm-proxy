package proxy

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

type delayedAccountingFixture struct {
	database   *gormManagedTenantDatabase
	management *httptest.Server
	operator   *httptest.Server
	cookie     func(string) *http.Cookie
	requestID  string
	last       managedJournalObservationRecord
	body       string
}

func newDelayedAccountingFixture(t *testing.T, quantities []journalQuantity, amount ExactMoney) delayedAccountingFixture {
	t.Helper()
	database, intent, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	reserve := newHostedFundsAdmission(prices)
	excess := []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "20000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}}
	request, first := observeRatedFixture(t, database, intent("delayed-accounting"), reserve, journalOutcomeContinue, excess)
	claim, err := newJournalWorkerClaim(request.ID, request.OwnerToken, request.CreatedAt.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-22222222222222222222222222222222", reserve)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.dispatchJournalAttempt(t.Context(), claim, attempt.ID); err != nil {
		t.Fatal(err)
	}
	evidence, err := newJournalUsageEvidence(journalUsageEvidenceInput{AttemptID: attempt.ID, AdapterRevision: "test-native-meter", Quantities: quantities, Outcome: journalOutcomeComplete, ObservedAt: claim.now}, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	last, err := database.observeJournalAttempt(t.Context(), claim, evidence)
	if err != nil {
		t.Fatal(err)
	}
	publication := &hostedTextRequests{hostedTextRequestDependencies: hostedTextRequestDependencies{database: database, now: func() time.Time { return claim.now }}}
	if err := publication.publishResponse(t.Context(), request, func(managedJournalRequestRecord) error { return nil }); err != nil {
		t.Fatal(err)
	}
	// Both observations are valid. Stop the second accounting transaction only,
	// after the first has committed its unresolved hold.
	trigger := fmt.Sprintf("CREATE TRIGGER defer_final_accounting BEFORE INSERT ON managed_charge_records WHEN NEW.observation_id = '%s' BEGIN SELECT RAISE(ABORT, 'controlled_accounting_delay'); END", last.ID)
	if err := database.database.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}
	failFundsApplication(t, database, management)
	var firstDelivery managedJournalDeliveryRecord
	if err := database.database.First(&firstDelivery, "observation_id = ?", first.ID).Error; err != nil || firstDelivery.DeliveredAt == nil {
		t.Fatalf("first accounting delivery did not commit: error=%v", err)
	}
	var reservation managedFundsReservationRecord
	if err := database.database.First(&reservation).Error; err != nil || reservation.State != fundsReservationReconciliation {
		t.Fatalf("missing unresolved reservation: state=%s error=%v", reservation.State, err)
	}
	operator, cookie := newFundsManagementHTTPFixture(t, database)
	body := fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":%q,"denominator":%q},"reason":"approved_exception","evidence_reference":"before-final-accounting"}`, reservation.Revision, amount.Numerator, amount.Denominator)
	fixture := delayedAccountingFixture{database, management, operator, cookie, request.ID, last, body}
	fundsResolutionHTTP(t, operator, cookie("operator"), http.MethodPut, fixture.decisionPath(), body, http.StatusOK)
	if err := database.database.Exec("DROP TRIGGER defer_final_accounting").Error; err != nil {
		t.Fatal(err)
	}
	fixture.assertPending(t)
	return fixture
}

func (fixture delayedAccountingFixture) decisionPath() string {
	return "/billing-accounts/billing-journal/requests/" + fixture.requestID + "/funds-resolution"
}

func (fixture delayedAccountingFixture) financialState(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{
		"balance":  ratingHTTPExchange(t, fixture.management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK),
		"entries":  ratingHTTPExchange(t, fixture.management, http.MethodGet, "/billing-accounts/billing-journal/ledger-entries", "", http.StatusOK),
		"decision": fundsResolutionHTTP(t, fixture.operator, fixture.cookie("owner"), http.MethodGet, fixture.decisionPath(), "", http.StatusOK),
		"tenant":   fundsResolutionHTTP(t, fixture.operator, fixture.cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/tenant-limits/managed-first", "", http.StatusOK),
	}
}

func (fixture delayedAccountingFixture) charges(t *testing.T) map[string]any {
	t.Helper()
	return ratingHTTPExchange(t, fixture.management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
}

func (fixture delayedAccountingFixture) reservation(t *testing.T) map[string]any {
	t.Helper()
	return fundsResolutionHTTP(t, fixture.operator, fixture.cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/reservations/"+fixture.requestID, "", http.StatusOK)
}

func (fixture delayedAccountingFixture) assertPending(t *testing.T) {
	t.Helper()
	var delivery managedJournalDeliveryRecord
	if err := fixture.database.database.First(&delivery, "observation_id = ?", fixture.last.ID).Error; err != nil || delivery.DeliveredAt != nil {
		t.Fatalf("delayed accounting lost its pending delivery: error=%v", err)
	}
	if charges := fixture.charges(t)["charges"].([]any); len(charges) != 1 {
		t.Fatalf("partial delayed accounting: charges=%v", charges)
	}
	fixture.assertExposure(t, ExactMoney{"51", "1250"}, ExactMoney{"37", "2500"}, false)
}

func (fixture delayedAccountingFixture) assertExposure(t *testing.T, cost, exposure ExactMoney, complete bool) {
	t.Helper()
	var records []managedFundsExposureRecord
	if err := fixture.database.database.Where("request_id = ?", fixture.requestID).Find(&records).Error; err != nil || len(records) != 1 {
		t.Fatalf("retained exposure count=%d error=%v", len(records), err)
	}
	record := records[0]
	if record.KnownCostNumerator != cost.Numerator || record.KnownCostDenominator != cost.Denominator || record.ExcessNumerator != exposure.Numerator || record.ExcessDenominator != exposure.Denominator || record.ProviderCostComplete != complete {
		t.Fatalf("retained exposure differs from accepted evidence: %+v", record)
	}
	var cases int64
	if err := fixture.database.database.Model(&managedJournalCaseRecord{}).Where("request_id = ? AND reason = ?", fixture.requestID, journalCasePlatformExposure).Count(&cases).Error; err != nil || cases != 1 {
		t.Fatalf("retained exposure cases=%d error=%v", cases, err)
	}
}

func (fixture delayedAccountingFixture) recover(t *testing.T, before map[string]any, cost, exposure ExactMoney, chargeState string) {
	t.Helper()
	var previousCharges, previousReservation map[string]any
	for iteration := 0; iteration < 2; iteration++ {
		restartFundsApplication(t, fixture.database, fixture.management)
		if !reflect.DeepEqual(before, fixture.financialState(t)) {
			t.Fatal("late accounting changed the operator decision or customer funds")
		}
		if result := fundsResolutionHTTP(t, fixture.operator, fixture.cookie("operator"), http.MethodPut, fixture.decisionPath(), fixture.body, http.StatusOK); !reflect.DeepEqual(before["decision"], result) {
			t.Fatal("late accounting changed decision replay")
		}
		charges, reservation := fixture.charges(t), fixture.reservation(t)
		if iteration == 1 && (!reflect.DeepEqual(previousCharges, charges) || !reflect.DeepEqual(previousReservation, reservation)) {
			t.Fatal("restart repeated late financial evidence")
		}
		previousCharges, previousReservation = charges, reservation
		if reservation["state"] != string(fundsReservationSettled) || reservation["provider_cost_complete"] != true {
			t.Fatalf("late accounting did not complete costs or reopened settlement: %v", reservation)
		}
		fixture.assertExposure(t, cost, exposure, true)
		for field, expected := range map[string]ExactMoney{"known_provider_cost": cost, "known_platform_exposure": exposure} {
			if !reflect.DeepEqual(reservation[field], map[string]any{"numerator": expected.Numerator, "denominator": expected.Denominator}) {
				t.Fatalf("late %s=%v want=%+v", field, reservation[field], expected)
			}
		}
		items := charges["charges"].([]any)
		if len(items) != 2 {
			t.Fatalf("late charge count=%d", len(items))
		}
		found := false
		for _, item := range items {
			charge := item.(map[string]any)
			if charge["observation_id"] == fixture.last.ID {
				found = charge["state"] == chargeState
			}
		}
		if !found {
			t.Fatalf("missing late charge in state %s: %v", chargeState, charges)
		}
		var pending int64
		if err := fixture.database.database.Model(&managedJournalDeliveryRecord{}).Where("delivered_at IS NULL").Count(&pending).Error; err != nil || pending != 0 {
			t.Fatalf("late delivery not acknowledged: count=%d error=%v", pending, err)
		}
	}
}

func TestHostedFundsDelayedAccountingPreservesOperatorDecision(t *testing.T) {
	for _, scenario := range []struct {
		name, input, output    string
		charge, cost, exposure ExactMoney
		state                  string
	}{
		{"waiver", "1000", "1000", ExactMoney{"0", "1"}, ExactMoney{"127", "2500"}, ExactMoney{"31", "1250"}, chargeRated},
		{"partial-charge", "1000", "1000", ExactMoney{"3", "200"}, ExactMoney{"127", "2500"}, ExactMoney{"31", "1250"}, chargeRated},
		{"waiver-with-exposure", "20000", "100", ExactMoney{"0", "1"}, ExactMoney{"51", "625"}, ExactMoney{"139", "2500"}, chargeLimitUnresolved},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newDelayedAccountingFixture(t, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: scenario.input}, {Dimension: "output_tokens", Unit: "token", Value: scenario.output}}, scenario.charge)
			fixture.recover(t, fixture.financialState(t), scenario.cost, scenario.exposure, scenario.state)
		})
	}
}

func TestHostedFundsDelayedAccountingWriteFailuresPreserveDecision(t *testing.T) {
	for _, scenario := range []struct{ name, statement string }{
		{"charge", "BEFORE INSERT ON managed_charge_records"},
		{"exposure", "BEFORE INSERT ON managed_funds_exposure_records"},
		{"exposure-case", "BEFORE INSERT ON managed_journal_case_records WHEN NEW.reason = 'platform_exposure'"},
		{"acknowledgment", "BEFORE UPDATE OF delivered_at ON managed_journal_delivery_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newDelayedAccountingFixture(t, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "20000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}}, ExactMoney{"0", "1"})
			before, charges, reservation := fixture.financialState(t), fixture.charges(t), fixture.reservation(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_delayed_accounting " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_late_accounting_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			failFundsApplication(t, fixture.database, fixture.management)
			fixture.assertPending(t)
			if !reflect.DeepEqual(before, fixture.financialState(t)) || !reflect.DeepEqual(charges, fixture.charges(t)) || !reflect.DeepEqual(reservation, fixture.reservation(t)) {
				t.Fatal("failed late accounting changed financial resources")
			}
			if err := fixture.database.database.Exec("DROP TRIGGER reject_delayed_accounting").Error; err != nil {
				t.Fatal(err)
			}
			fixture.recover(t, before, ExactMoney{"51", "625"}, ExactMoney{"139", "2500"}, chargeLimitUnresolved)
		})
	}
}
