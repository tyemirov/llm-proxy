package proxy

import (
	"context"
	"encoding/json"
	"github.com/tyemirov/utils/billing"
	"net/http"
	"testing"
	"time"
)

func paymentStateFixture(t *testing.T, processor *checkoutProtocolFixture, status, updated string) map[string]any {
	t.Helper()
	processor.mutex.Lock()
	defer processor.mutex.Unlock()
	transaction := processor.transactions[0]
	transaction["status"], transaction["updated_at"] = status, updated
	encoded, err := json.Marshal(transaction)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestHostedPaymentsCanceledStateUsesCurrentProcessorEvidence(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "canceled-state", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	canceled := paymentStateFixture(t, processor, "canceled", "2026-09-23T12:01:00Z")
	sendPaymentEventFixture(t, database, canceled, "transaction.canceled", 1)
	worker := paymentProcessorFixture(t, checkout, database)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	read := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
	if read["state"] != "failed" {
		t.Fatalf("verified cancellation left order=%v", read)
	}
	assertHostedFundsBalance(t, database, 0, 0)
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string)+"/receipt", "", "", http.StatusNotFound)
	// A later-delivered notification must use the current canceled processor state.
	old := paymentStateFixture(t, processor, "ready", "2026-09-23T12:00:00Z")
	paymentStateFixture(t, processor, "canceled", "2026-09-23T12:01:00Z")
	sendPaymentEventFixture(t, database, old, "transaction.ready", 2)
	if err := paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database)).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	read = paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
	if read["state"] != "failed" {
		t.Fatalf("reordered notification changed canceled order=%v", read)
	}
	assertHostedFundsBalance(t, database, 0, 0)
}

func TestHostedPaymentsDeclinedAttemptRemainsRetryableUntilVerifiedCompletion(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "declined-state", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	declined := paymentStateFixture(t, processor, "ready", "2026-09-23T12:00:00Z")
	sendPaymentEventFixture(t, database, declined, "transaction.payment_failed", 1)
	worker := paymentProcessorFixture(t, checkout, database)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	read := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
	if read["state"] != "pending" {
		t.Fatalf("declined attempt cannot resume: %v", read)
	}
	assertHostedFundsBalance(t, database, 0, 0)
	transaction := completedPaymentFixture(t, processor)
	sendPaymentEventFixture(t, database, transaction, "transaction.updated", 2)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 0, 0)
	var retained managedPaymentInboxRecord
	if err := database.database.Where("event_id = ?", "evt_00000000000000000000000002").First(&retained).Error; err != nil {
		t.Fatal(err)
	}
	if retained.State != paymentInboxReconciliation || retained.Reason != "completion_event_required" {
		t.Fatalf("missing completion event was concealed: %+v", retained)
	}
	sendPaymentEventFixture(t, database, transaction, "transaction.completed", 3)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 500, 500)
	sendPaymentEventFixture(t, database, declined, "transaction.payment_failed", 4)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	read = paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
	if read["state"] != "paid" {
		t.Fatalf("old failure regressed credit: %v", read)
	}
	assertHostedFundsBalance(t, database, 500, 500)
}

func TestHostedPaymentsStateRejectsUnverifiedOrUnknownEvidence(t *testing.T) {
	for _, scenario := range []string{"event-owner", "processor-owner", "processor-environment", "timestamp", "stale-api", "unknown-event", "unknown-state"} {
		t.Run(scenario, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, scenario, `{"offer_code":"five"}`, http.StatusCreated)
			checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
			if err := checkout.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			canceled := paymentStateFixture(t, processor, "canceled", "2026-09-23T12:01:00Z")
			eventType := "transaction.canceled"
			processor.mutex.Lock()
			switch scenario {
			case "event-owner":
				canceled["custom_data"].(map[string]any)["billing_account_id"] = "another-account"
			case "processor-owner":
				processor.transactions[0]["customer_id"] = "ctm_00000000000000000000000002"
			case "processor-environment":
				processor.transactions[0]["custom_data"].(map[string]string)["environment"] = "production"
			case "timestamp":
				processor.transactions[0]["updated_at"] = "unknown"
			case "stale-api":
				processor.transactions[0]["updated_at"], processor.transactions[0]["status"] = "2026-09-23T12:00:00Z", "ready"
			case "unknown-event":
				eventType = "transaction.future"
			case "unknown-state":
				processor.transactions[0]["status"] = "future"
			}
			processor.mutex.Unlock()
			sendPaymentEventFixture(t, database, canceled, eventType, 1)
			if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			read := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
			if read["state"] != "pending" {
				t.Fatalf("invalid evidence changed order: %v", read)
			}
			assertHostedFundsBalance(t, database, 0, 0)
			var event managedPaymentInboxRecord
			if err := database.database.First(&event).Error; err != nil {
				t.Fatal(err)
			}
			if event.State != paymentInboxReconciliation {
				t.Fatalf("invalid state event was acknowledged as applied: %s", event.State)
			}
			var count int64
			if err := database.database.Model(&managedPaymentStateObservationRecord{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("invalid processor observation retained: count=%d error=%v", count, err)
			}
		})
	}
}

func TestHostedPaymentsStateWritesRollbackTogetherAndRecover(t *testing.T) {
	for _, boundary := range []string{"BEFORE INSERT ON managed_payment_state_observation_records", "BEFORE UPDATE OF state ON managed_funding_order_records", "BEFORE UPDATE OF state ON managed_payment_inbox_records"} {
		t.Run(boundary, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "state-rollback", `{"offer_code":"five"}`, http.StatusCreated)
			checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
			if err := checkout.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			canceled := paymentStateFixture(t, processor, "canceled", "2026-09-23T12:01:00Z")
			sendPaymentEventFixture(t, database, canceled, "transaction.canceled", 1)
			if err := database.database.Exec("CREATE TRIGGER reject_payment_state " + boundary + " BEGIN SELECT RAISE(ABORT, 'controlled_state_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err == nil {
				t.Fatal("state write failure ignored")
			}
			read := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
			if read["state"] != "pending" {
				t.Fatalf("order committed without evidence: %v", read)
			}
			var count int64
			if err := database.database.Model(&managedPaymentStateObservationRecord{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("partial state observation count=%d error=%v", count, err)
			}
			var event managedPaymentInboxRecord
			if err := database.database.First(&event).Error; err != nil || event.State != paymentInboxPending {
				t.Fatalf("partial inbox state=%s error=%v", event.State, err)
			}
			if err := database.database.Exec("DROP TRIGGER reject_payment_state").Error; err != nil {
				t.Fatal(err)
			}
			if err := paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database)).reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			read = paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
			if read["state"] != "failed" {
				t.Fatalf("restart did not apply cancellation: %v", read)
			}
			assertHostedFundsBalance(t, database, 0, 0)
		})
	}
}

type delayedPaymentStateRead struct {
	paddleTransactionReader
	read    chan struct{}
	release chan struct{}
}

func (client delayedPaymentStateRead) GetTransaction(ctx context.Context, id string) (billing.PaddleTransactionCompletedWebhookData, error) {
	transaction, err := client.paddleTransactionReader.GetTransaction(ctx, id)
	close(client.read)
	select {
	case <-client.release:
		return transaction, err
	case <-ctx.Done():
		return billing.PaddleTransactionCompletedWebhookData{}, ctx.Err()
	}
}

func TestHostedPaymentsConcurrentOlderSnapshotCannotReopenCanceledOrder(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "state-concurrency", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	ready := paymentStateFixture(t, processor, "ready", "2026-09-23T12:00:00Z")
	sendPaymentEventFixture(t, database, ready, "transaction.ready", 1)
	delayed := delayedPaymentStateRead{paddleTransactionReader: checkout.client, read: make(chan struct{}), release: make(chan struct{}, 1)}
	t.Cleanup(func() { close(delayed.release) })
	first := paymentProcessorFixture(t, checkout, database)
	first.client = delayed
	done := make(chan error, 1)
	go func() { done <- first.reconcile(t.Context()) }()
	select {
	case <-delayed.read:
	case <-time.After(5 * time.Second):
		t.Fatal("first processor read did not arrive")
	}
	canceled := paymentStateFixture(t, processor, "canceled", "2026-09-23T12:01:00Z")
	sendPaymentEventFixture(t, database, canceled, "transaction.canceled", 2)
	if err := paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database)).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	delayed.release <- struct{}{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("older state worker did not stop")
	}
	read := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
	if read["state"] != "failed" {
		t.Fatalf("older worker reopened canceled payment: %v", read)
	}
	var observations []managedPaymentStateObservationRecord
	if err := database.database.Find(&observations).Error; err != nil {
		t.Fatal(err)
	}
	if len(observations) != 2 {
		t.Fatalf("observations=%d", len(observations))
	}
	for _, observation := range observations {
		if observation.ProcessorStatus != "canceled" {
			t.Fatal("older processor state committed")
		}
	}
	assertHostedFundsBalance(t, database, 0, 0)
}
