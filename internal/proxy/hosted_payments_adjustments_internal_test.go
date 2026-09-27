package proxy

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
)

func paymentAdjustmentFixture(processor *checkoutProtocolFixture, status string, principal int, revision int) map[string]any {
	processor.mutex.Lock()
	defer processor.mutex.Unlock()
	updated := fmt.Sprintf("2026-09-23T12:%02d:00Z", revision)
	adjustment := map[string]any{
		"id": "adj_00000000000000000000000001", "action": "refund", "type": "partial", "status": status,
		"transaction_id": processor.transactions[0]["id"], "customer_id": "ctm_01hv8x2axb33yr5y238zfwcn5p", "currency_code": "USD",
		"created_at": "2026-09-23T12:01:00Z", "updated_at": updated,
		"items":  []any{map[string]any{"id": "adjitm_fixture", "item_id": "txnitm_01hv8x2axb33yr5y238zfwcn5p", "type": "partial", "amount": fmt.Sprint(principal), "totals": map[string]any{"subtotal": fmt.Sprint(principal), "tax": fmt.Sprint(principal / 10), "total": fmt.Sprint(principal + principal/10)}}},
		"totals": map[string]any{"subtotal": fmt.Sprint(principal), "tax": fmt.Sprint(principal / 10), "total": fmt.Sprint(principal + principal/10), "fee": "0", "retained_fee": "0", "earnings": fmt.Sprint(principal), "currency_code": "USD"},
	}
	remaining := 500
	if status == "approved" {
		remaining -= principal
	}
	processor.transactions[0]["updated_at"] = updated
	processor.transactions[0]["details"].(map[string]any)["adjusted_totals"] = map[string]any{"subtotal": fmt.Sprint(remaining), "tax": fmt.Sprint(remaining / 10), "total": fmt.Sprint(remaining + remaining/10), "grand_total": fmt.Sprint(remaining + remaining/10), "grand_total_tax": fmt.Sprint(remaining / 10), "fee": "30", "retained_fee": "0", "earnings": fmt.Sprint(remaining - 30), "currency_code": "USD"}
	processor.adjustments = []map[string]any{adjustment}
	return adjustment
}

func TestHostedPaymentsAdjustmentDeficitSuspendsAdmissionAndRecovers(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "refund-deficit", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	transaction := completedPaymentFixture(t, processor)
	sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
	worker := paymentProcessorFixture(t, checkout, database)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	account, err := newHostedLedgerAccount(database.database, "billing-journal", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	amount, err := ledger.NewPositiveAmountCents(400)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ledger.NewIdempotencyKey("earlier-consumption")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := ledger.NewMetadataJSON(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := account.service.Spend(t.Context(), account.tenant, account.user, account.namespace, amount, key, metadata); err != nil {
		t.Fatal(err)
	}
	adjustment := paymentAdjustmentFixture(processor, "approved", 500, 1)
	sendPaymentEventFixture(t, database, adjustment, "adjustment.created", 2)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	balance := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
	if balance["posted_cents"] != "-400" || balance["state"] != "suspended" {
		t.Fatalf("deficit is not suspended: %v", balance)
	}
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	proxy := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	hostedIdentityHTTP(t, proxy, "deficit-rejected", "funded prompt", http.StatusForbidden)
	if calls.Load() != 0 {
		t.Fatalf("deficit dispatched %d requests", calls.Load())
	}
	// Processor reversal restores the original funding once, without erasing usage.
	adjustment = paymentAdjustmentFixture(processor, "reversed", 500, 2)
	sendPaymentEventFixture(t, database, adjustment, "adjustment.updated", 3)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	balance = paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
	if balance["posted_cents"] != "100" || balance["state"] != "active" {
		t.Fatalf("reversal did not clear financial restriction: %v", balance)
	}
}

func TestHostedPaymentsAdjustmentHoldsAndCompensation(t *testing.T) {
	for _, final := range []string{"approved", "rejected"} {
		t.Run(final, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "refund", `{"offer_code":"five"}`, http.StatusCreated)
			checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
			if err := checkout.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			transaction := completedPaymentFixture(t, processor)
			sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
			worker := paymentProcessorFixture(t, checkout, database)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			pending := paymentAdjustmentFixture(processor, "pending_approval", 200, 1)
			sendPaymentEventFixture(t, database, pending, "adjustment.created", 2)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			balance := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
			if balance["posted_cents"] != "500" || balance["available_cents"] != "300" {
				t.Fatalf("pending refund has no protected funds: %v", balance)
			}
			adjustment := paymentAdjustmentFixture(processor, final, 200, 2)
			sendPaymentEventFixture(t, database, adjustment, "adjustment.updated", 3)
			worker = paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database))
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			// A stale, separately delivered creation event must use current processor state.
			sendPaymentEventFixture(t, database, pending, "adjustment.created", 4)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			want, state := "500", "paid"
			if final == "approved" {
				want, state = "300", "partially_refunded"
			}
			balance = paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
			if balance["posted_cents"] != want || balance["available_cents"] != want {
				t.Fatalf("refund balance=%v want=%s", balance, want)
			}
			got := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string), "", "", http.StatusOK)
			if got["state"] != state {
				t.Fatalf("refund order=%v want=%s", got, state)
			}
			// An older provider snapshot must not stop processing or roll back
			// the verified financial state. Retain it for reconciliation.
			stale := paymentAdjustmentFixture(processor, "pending_approval", 200, 1)
			sendPaymentEventFixture(t, database, stale, "adjustment.updated", 5)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			balance = paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
			if balance["posted_cents"] != want || balance["available_cents"] != want {
				t.Fatalf("stale provider evidence changed balance: %v", balance)
			}
			var retained managedPaymentInboxRecord
			if err := database.database.Where("event_id = ?", fmt.Sprintf("evt_%026d", 5)).First(&retained).Error; err != nil || retained.State != paymentInboxReconciliation {
				t.Fatalf("stale evidence was not retained: %+v error=%v", retained, err)
			}
		})
	}
}

func TestHostedPaymentsAdjustmentBeforeFundingNeverMakesRefundedFundsAvailable(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "already-refunded", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	transaction := completedPaymentFixture(t, processor)
	sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
	paymentAdjustmentFixture(processor, "approved", 500, 1)
	if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	balance := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
	if balance["posted_cents"] != "0" || balance["available_cents"] != "0" {
		t.Fatalf("refunded funds became available: %v", balance)
	}
}

func TestHostedPaymentsAdjustmentRollbackConcurrencyAndChargebackReplay(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "chargeback", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	transaction := completedPaymentFixture(t, processor)
	sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
	worker := paymentProcessorFixture(t, checkout, database)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	warning := paymentAdjustmentFixture(processor, "approved", 500, 1)
	warning["action"] = "chargeback_warning"
	sendPaymentEventFixture(t, database, warning, "adjustment.created", 2)
	if err := database.database.Exec("CREATE TRIGGER reject_adjustment BEFORE INSERT ON managed_payment_adjustment_revision_records BEGIN SELECT RAISE(ABORT, 'controlled_adjustment_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := worker.reconcile(t.Context()); err == nil {
		t.Fatal("adjustment write failure was ignored")
	}
	assertHostedFundsBalance(t, database, 500, 500)
	if err := database.database.Exec("DROP TRIGGER reject_adjustment").Error; err != nil {
		t.Fatal(err)
	}
	workers := []*paddlePaymentProcessor{worker, paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database))}
	var group sync.WaitGroup
	for _, current := range workers {
		group.Go(func() {
			if err := current.reconcile(t.Context()); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	assertHostedFundsBalance(t, database, 0, 0)
	// A warning followed by a chargeback must not deduct the original principal twice.
	chargeback := paymentAdjustmentFixture(processor, "approved", 500, 2)
	chargeback["id"], chargeback["action"] = "adj_00000000000000000000000002", "chargeback"
	processor.adjustments = append(processor.adjustments, warning)
	sendPaymentEventFixture(t, database, chargeback, "adjustment.created", 3)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 0, 0)
	// The original records become reversed and separate reverse records arrive.
	reversal := paymentAdjustmentFixture(processor, "approved", 500, 3)
	reversal["id"], reversal["action"] = "adj_00000000000000000000000003", "chargeback_reverse"
	for _, original := range []map[string]any{warning, chargeback} {
		original["status"], original["updated_at"] = "reversed", "2026-09-23T12:03:00Z"
	}
	processor.adjustments = []map[string]any{warning, chargeback, reversal}
	processor.transactions[0]["details"].(map[string]any)["adjusted_totals"] = map[string]any{"subtotal": "500", "tax": "50", "total": "550", "grand_total": "550", "grand_total_tax": "50", "retained_fee": "30", "fee": "30", "earnings": "470", "currency_code": "USD"}
	for sequence := 4; sequence <= 5; sequence++ {
		sendPaymentEventFixture(t, database, reversal, "adjustment.created", sequence)
	}
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 500, 500)
}
