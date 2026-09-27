package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHostedFundsImageOversizedResponsePreservesUnknownCostAndHold(t *testing.T) {
	database, _, management, _ := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 500)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"data":[],"untrusted_padding":%q}`, strings.Repeat("x", 128<<10))
	}))
	t.Cleanup(upstream.Close)
	server, worker := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
	worker.hostedAdmission = hostedImageFinancialSettings(t, ModelOperationImageGeneration).mediaAdmission(worker.providers)
	accepted := hostedMediaAdmissionHTTP(t, server, "oversized-image", "funded image", http.StatusAccepted)
	id := accepted["operation_id"].(string)
	worker.runOperation("oversized-image-worker", id)
	result := hostedMediaWorkerStatus(t, server, id)
	if result["state"] != MediaOperationStateFailed || result["error"].(map[string]any)["code"] != "provider_result_invalid" || len(result["outputs"].([]any)) != 0 {
		t.Fatalf("oversized image result=%v", result)
	}
	fixture := fundsStartupFixture{database, management, &calls}
	var reconciled map[string]any
	for iteration := range 2 {
		restartFundsApplication(t, database, management, ratingTestAcceptanceTime)
		assertHostedFundsBalance(t, database, 500, 461)
		assertFundsCreditRemainder(t, database, "0", "1")
		if replay := hostedMediaAdmissionHTTP(t, server, "oversized-image", "funded image", http.StatusOK); replay["operation_id"] != id {
			t.Fatal("oversized image replay changed the operation identity")
		}
		worker.runOperation("replayed-oversized-image", id)
		current := fixture.state(t)
		charges := current["charges"].(map[string]any)["charges"].([]any)
		if len(charges) != 1 || calls.Load() != 1 {
			t.Fatalf("oversized image charges=%v calls=%d", charges, calls.Load())
		}
		charge := charges[0].(map[string]any)
		if charge["state"] != chargeUsageUnresolved || charge["customer_charge"] != nil || charge["rating"].(map[string]any)["provider_cost"] != nil {
			t.Fatalf("oversized image acquired an invented cost: %v", charge)
		}
		if iteration > 0 && !reflect.DeepEqual(reconciled, current) {
			t.Fatal("oversized image restart changed financial effects")
		}
		reconciled = current
	}
}
