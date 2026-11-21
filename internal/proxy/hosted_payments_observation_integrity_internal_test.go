package proxy

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestHostedPaymentsCorruptObservationDefersEventWithoutStallingInbox(t *testing.T) {
	for _, scenario := range []struct {
		name           string
		eventType      string
		recoveredCents string
	}{
		{"invalid-digest", "transaction.updated", "500"},
		{"invalid-json", "adjustment.updated", "300"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			before := paymentAdjustmentResources(t, fixture)
			var observation managedPaymentStateObservationRecord
			if err := fixture.database.database.First(&observation, "order_id = ?", fixture.orderID).Error; err != nil {
				t.Fatal(err)
			}
			corruptEvidence, corruptDigest := observation.Evidence, "incorrect-digest"
			if scenario.name == "invalid-json" {
				corruptEvidence = "{"
				corruptDigest = sha256Hex(corruptEvidence)
			}
			row := fixture.database.database.Model(&managedPaymentStateObservationRecord{}).Where("id = ?", observation.ID)
			if err := row.Updates(map[string]any{"evidence": corruptEvidence, "evidence_digest": corruptDigest}).Error; err != nil {
				t.Fatal(err)
			}

			otherOwner := fixture.cookie("other-payment-owner")
			account := paymentOrderHTTP(t, fixture.server, otherOwner, http.MethodPost, managementBillingAccountsPath, "other-payment-account", `{"currency":"USD"}`, http.StatusCreated)
			accountID := account["id"].(string)
			accountPath := managementBillingAccountsPath + "/" + accountID
			ordersPath := accountPath + "/funding-orders"
			order := paymentOrderHTTP(t, fixture.server, otherOwner, http.MethodPost, ordersPath, "other-payment-order", `{"offer_code":"five"}`, http.StatusCreated)
			fixture.processor.mutex.Lock()
			fixture.processor.customers["other-payment-owner@example.com"] = checkoutProtocolCustomer{accountID: accountID, customerID: "ctm_00000000000000000000000002"}
			fixture.processor.mutex.Unlock()
			checkout := checkoutWorkerFixture(t, fixture.database, fixture.checkout.catalog, fixture.processor, time.Now())
			if err := checkout.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			completed := completedPaymentFixtureAt(t, fixture.processor, 1)
			var affected map[string]any
			if scenario.eventType == "adjustment.updated" {
				affected = paymentAdjustmentFixture(fixture.processor, "approved", 200, 2)
				fixture.processor.mutex.Lock()
				fixture.processor.transactions[0]["updated_at"] = observation.ProcessorUpdatedAt.Format(time.RFC3339Nano)
				fixture.processor.mutex.Unlock()
			} else {
				affected = fixture.processor.transactions[0]
			}
			sendPaymentEventFixture(t, fixture.database, affected, scenario.eventType, 2)
			sendPaymentEventFixture(t, fixture.database, completed, paymentTransactionCompleted, 3)

			now := time.Now().UTC()
			worker := paymentProcessorFixture(t, checkout, fixture.database)
			worker.now = func() time.Time { return now }
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatalf("corrupt observation stopped the batch: %v", err)
			}
			loadAffected := func() managedPaymentInboxRecord {
				t.Helper()
				var event managedPaymentInboxRecord
				if err := fixture.database.database.First(&event, "event_id = ?", fmt.Sprintf("evt_%026d", 2)).Error; err != nil {
					t.Fatal(err)
				}
				return event
			}
			event := loadAffected()
			if event.State != paymentInboxReconciliation || event.Reason != "transaction_observation_integrity" || !event.RetryAt.Equal(now.Add(paymentCheckoutRetry)) {
				t.Fatalf("unresolved integrity event state=%s reason=%s retry_at=%s", event.State, event.Reason, event.RetryAt)
			}
			if !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) {
				t.Fatal("corrupt observation changed the affected account's financial resources")
			}
			otherBalance := paymentOrderHTTP(t, fixture.server, otherOwner, http.MethodGet, accountPath+"/balance", "", "", http.StatusOK)
			if otherBalance["available_cents"] != "500" {
				t.Fatalf("later unrelated completion failed: %v", otherBalance)
			}
			receiptPath := ordersPath + "/" + order["id"].(string) + "/receipt"
			otherReceipt := paymentOrderHTTP(t, fixture.server, otherOwner, http.MethodGet, receiptPath, "", "", http.StatusOK)

			if err := row.Updates(map[string]any{"evidence": observation.Evidence, "evidence_digest": observation.EvidenceDigest}).Error; err != nil {
				t.Fatal(err)
			}
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(event, loadAffected()) {
				t.Fatal("event retried before its scheduled time")
			}
			now = event.RetryAt
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			if retried := loadAffected(); retried.State != paymentInboxApplied {
				t.Fatalf("repaired event did not apply: state=%s reason=%s", retried.State, retried.Reason)
			}
			if balance := fixture.balance(t); balance["available_cents"] != scenario.recoveredCents {
				t.Fatalf("repaired event had unexpected effects: %v", balance)
			}
			if !reflect.DeepEqual(otherReceipt, paymentOrderHTTP(t, fixture.server, otherOwner, http.MethodGet, receiptPath, "", "", http.StatusOK)) || !reflect.DeepEqual(otherBalance, paymentOrderHTTP(t, fixture.server, otherOwner, http.MethodGet, accountPath+"/balance", "", "", http.StatusOK)) {
				t.Fatal("retry changed the other account's receipt or balance")
			}
		})
	}
}
