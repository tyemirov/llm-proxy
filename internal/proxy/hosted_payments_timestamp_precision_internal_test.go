package proxy

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestHostedPaymentsWebhookSnapshotPrecision(t *testing.T) {
	for _, scenario := range []struct {
		name, eventType, eventStatus, apiStatus, apiUpdated, expectedState string
		changedFee                                                         bool
	}{
		{"completed", "transaction.completed", "completed", "completed", "2026-09-23T12:00:01.026101Z", "paid", false},
		{"older microsecond", "transaction.completed", "completed", "completed", "2026-09-23T12:00:01.026100999Z", "pending", false},
		{"changed financial evidence", "transaction.completed", "completed", "completed", "2026-09-23T12:00:01.026101Z", "pending", true},
		{"canceled", "transaction.canceled", "canceled", "canceled", "2026-09-23T12:00:01.026101Z", "failed", false},
		{"conflicting lifecycle", "transaction.ready", "ready", "canceled", "2026-09-23T12:00:01.026101Z", "pending", false},
		{"older lifecycle", "transaction.canceled", "canceled", "canceled", "2026-09-23T12:00:01.026100999Z", "pending", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "snapshot-precision", `{"offer_code":"five"}`, http.StatusCreated)
			checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
			if err := checkout.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			transaction := completedPaymentFixture(t, processor)
			processor.mutex.Lock()
			transaction["status"] = scenario.eventStatus
			transaction["updated_at"] = "2026-09-23T12:00:01.026101390Z"
			if scenario.name == "older microsecond" {
				transaction["updated_at"] = "2026-09-23T12:00:01.026101Z"
			}
			encoded, err := json.Marshal(transaction)
			if err != nil {
				t.Fatal(err)
			}
			var event map[string]any
			if err := json.Unmarshal(encoded, &event); err != nil {
				t.Fatal(err)
			}
			transaction["updated_at"] = scenario.apiUpdated
			transaction["status"] = scenario.apiStatus
			if scenario.changedFee {
				transaction["details"].(map[string]any)["totals"].(map[string]any)["fee"] = "31"
			}
			processor.mutex.Unlock()
			sendPaymentEventFixture(t, database, event, scenario.eventType, 1)
			if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			paid := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
			if paid["state"] != scenario.expectedState {
				t.Fatalf("order state=%v; want %s", paid["state"], scenario.expectedState)
			}
			if scenario.expectedState != "paid" {
				assertHostedFundsBalance(t, database, 0, 0)
				return
			}
			// The original signed event replays after storage is reopened.
			sendPaymentEventFixture(t, database, event, scenario.eventType, 1)
			if err := paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database)).reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			assertHostedFundsBalance(t, database, 500, 500)
			history := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/ledger-entries", "", "", http.StatusOK)
			if len(history["entries"].([]any)) != 1 {
				t.Fatalf("replayed credit history=%v", history)
			}
			receipt := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string)+"/receipt", "", "", http.StatusOK)
			if receipt["paid_at"] != "2026-09-23T12:00:00Z" || receipt["credit_cents"] != "500" {
				t.Fatalf("receipt=%v", receipt)
			}
		})
	}
}
