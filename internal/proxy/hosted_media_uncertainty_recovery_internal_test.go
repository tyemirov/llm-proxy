package proxy

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHostedMediaUncertaintyWriteFailuresRetainFundsAndRecoverOnce(t *testing.T) {
	for _, scenario := range []struct{ name, statement string }{
		{"attempt", "BEFORE UPDATE ON managed_journal_attempt_records WHEN NEW.state = 'uncertain'"},
		{"request", "BEFORE UPDATE ON managed_journal_request_records WHEN NEW.state = 'uncertain'"},
		{"reconciliation", "BEFORE INSERT ON managed_journal_case_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundedMediaRecoveryFixtureWithResponse(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusServiceUnavailable)
			}))
			before := fixture.state(t)
			core, logs := observer.New(zap.ErrorLevel)
			fixture.worker.logger = zap.New(core).Sugar()
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_media_uncertainty " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_media_uncertainty_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			fixture.worker.runOperation("uncertainty-failure-worker", fixture.operationID)
			fixture.assertFailure(t, MediaOperationStateRunning, 1)
			if !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatal("failed uncertainty persistence changed financial resources")
			}
			requests := accountConnectionHTTPExchange(t, fixture.management, http.MethodGet, "/billing-accounts/billing-journal/requests", "", http.StatusOK)["requests"].([]any)
			if len(requests) != 1 || requests[0].(map[string]any)["state"] != string(journalRequestExecuting) {
				t.Fatalf("failed uncertainty published a terminal journal request: %v", requests)
			}
			requestPath := "/billing-accounts/billing-journal/requests/" + requests[0].(map[string]any)["id"].(string)
			reports := logs.FilterMessage("media operation worker failed").All()
			if len(reports) != 1 || reports[0].ContextMap()["operation_id"] != fixture.operationID || !strings.Contains(fmt.Sprint(reports[0].ContextMap()["error"]), "controlled_media_uncertainty_failure") {
				t.Fatalf("uncertainty failure reports=%v", reports)
			}
			for _, model := range []any{&managedJournalCaseRecord{}, &managedJournalObservationRecord{}, &managedJournalDeliveryRecord{}, &mediaOperationUsageDeliveryRecord{}} {
				var count int64
				if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("failed uncertainty retained partial %T records=%d error=%v", model, count, err)
				}
			}
			if err := fixture.database.database.Exec("DROP TRIGGER reject_media_uncertainty").Error; err != nil {
				t.Fatal(err)
			}
			fixture.recover(t, MediaOperationStateRunning)
			request := accountConnectionHTTPExchange(t, fixture.management, http.MethodGet, requestPath, "", http.StatusOK)
			cases := accountConnectionHTTPExchange(t, fixture.management, http.MethodGet, requestPath+"/reconciliation-cases", "", http.StatusOK)["cases"].([]any)
			if request["state"] != string(journalRequestUncertain) || request["usage_state"] != string(journalUsageUnknown) || len(cases) != 1 || cases[0].(map[string]any)["reason"] != journalCaseDispatchUnknown {
				t.Fatalf("recovery lost uncertainty or repeated reconciliation: request=%v cases=%v", request, cases)
			}
		})
	}
}

func TestHostedMediaIgnoredJournalClaimPreventsDispatchAndRecoversOnce(t *testing.T) {
	fixture := newFundedMediaRecoveryFixture(t)
	before := fixture.state(t)
	operation := hostedMediaWorkerStatus(t, fixture.server, fixture.operationID)
	if err := fixture.database.database.Exec(`CREATE TRIGGER ignore_media_journal_claim BEFORE UPDATE OF owner_token ON managed_journal_request_records
		BEGIN SELECT RAISE(IGNORE); END`).Error; err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 2; iteration++ {
		fixture.worker.runOperation("ignored-claim-worker", fixture.operationID)
		fixture.assertFailure(t, MediaOperationStateQueued, 0)
		if !reflect.DeepEqual(operation, hostedMediaWorkerStatus(t, fixture.server, fixture.operationID)) || !reflect.DeepEqual(before, fixture.state(t)) {
			t.Fatal("ignored journal claim changed the accepted operation or financial resources")
		}
		for _, model := range []any{&mediaOperationClaimRecord{}, &managedJournalAttemptRecord{}} {
			var count int64
			if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("ignored journal claim retained partial %T records=%d error=%v", model, count, err)
			}
		}
	}
	if err := fixture.database.database.Exec("DROP TRIGGER ignore_media_journal_claim").Error; err != nil {
		t.Fatal(err)
	}
	fixture.recover(t, MediaOperationStateQueued)
}
