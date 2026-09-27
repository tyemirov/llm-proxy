package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestHostedTextObservationLockFailuresPreserveFunds(t *testing.T) {
	for _, scenario := range []struct {
		name, action string
		status       int
	}{
		{"rejected", "RAISE(ABORT, 'controlled_observation_lock_failure')", http.StatusBadGateway},
		{"ignored", "RAISE(IGNORE)", http.StatusConflict},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			statement := `CREATE TRIGGER reject_observation_lock
				BEFORE UPDATE ON managed_journal_request_records
				WHEN EXISTS (SELECT 1 FROM managed_journal_attempt_records
					WHERE request_id = OLD.id AND provider_request_id != '' AND state = 'dispatched')
				BEGIN SELECT ` + scenario.action + `; END`
			if err := fixture.database.database.Exec(statement).Error; err != nil {
				t.Fatal(err)
			}
			request := failHostedTextExecutionStatus(t, fixture, 1, scenario.status)
			assertNoHostedObservationEffects(t, fixture)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_observation_lock").Error; err != nil {
				t.Fatal(err)
			}
			recoverHostedTextExecution(t, fixture, request, 1)
		})
	}
}

func TestHostedTextUnknownUsageCaseFailureRollsBackObservation(t *testing.T) {
	database, _, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	calls := &atomic.Int64{}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"private-unmeasured-result","status":"completed","output_text":"funded result","usage":{}}`)
	}))
	t.Cleanup(upstream.Close)
	root := t.TempDir()
	generation := newHostedIdentityHTTPServer(t, database, upstream.URL, root, fundsDependencies(prices))
	fixture := fundsAdmissionFixture{fundsStartupFixture{database, management, calls}, generation, upstream.URL, root, prices}
	if err := database.database.Exec(`CREATE TRIGGER reject_unknown_usage_case
		BEFORE INSERT ON managed_journal_case_records WHEN NEW.reason = 'usage_unknown'
		BEGIN SELECT RAISE(ABORT, 'controlled_unknown_usage_case_failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	request := failHostedTextExecution(t, fixture, 1)
	assertNoHostedObservationEffects(t, fixture)
	if err := database.database.Exec("DROP TRIGGER reject_unknown_usage_case").Error; err != nil {
		t.Fatal(err)
	}
	recoverHostedTextExecution(t, fixture, request, 1)
}

func assertNoHostedObservationEffects(t *testing.T, fixture fundsAdmissionFixture) {
	t.Helper()
	for _, model := range []any{&managedJournalObservationRecord{}, &managedJournalDeliveryRecord{}, &managedChargeRecord{}, &managedFundsSettlementRecord{}} {
		var count int64
		if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("partial observation effect %T: count=%d error=%v", model, count, err)
		}
	}
	assertFundsCreditRemainder(t, fixture.database, "0", "1")
}

func TestHostedFundsStartupIgnoredDeliveryLockPreservesSettlement(t *testing.T) {
	fixture := newFundsStartupFixture(t, startupCompleteUsage)
	before := fixture.state(t)
	if err := fixture.database.database.Exec(`CREATE TRIGGER ignore_startup_delivery_lock
		BEFORE UPDATE OF observation_id ON managed_journal_delivery_records
		BEGIN SELECT RAISE(IGNORE); END`).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		fixture.failStartup(t)
		fixture.assertPending(t, before)
	}
	if err := fixture.database.database.Exec("DROP TRIGGER ignore_startup_delivery_lock").Error; err != nil {
		t.Fatal(err)
	}
	fixture.assertSettledOnce(t, before)
}
