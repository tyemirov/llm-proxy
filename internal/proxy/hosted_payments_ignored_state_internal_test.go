package proxy

import (
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestHostedPaymentsIgnoredStateWritesPreserveFundsAndRecover(t *testing.T) {
	for _, transition := range []struct {
		name, eventType, finalState, conflictReason string
	}{
		{"cancellation", "transaction.canceled", fundingOrderFailed, "transaction_state_conflict"},
		{"completion", paymentTransactionCompleted, fundingOrderPaid, "financial_state_conflict"},
	} {
		for _, boundary := range []struct{ name, statement string }{
			{"account-lock", "BEFORE UPDATE OF id ON managed_billing_account_records"},
			{"order-state", "BEFORE UPDATE OF state ON managed_funding_order_records"},
		} {
			t.Run(transition.name+"/"+boundary.name, func(t *testing.T) {
				fixture := pendingPaymentStateFixture(t)
				var event map[string]any
				if transition.eventType == paymentTransactionCompleted {
					event = completedPaymentFixture(t, fixture.processor)
				} else {
					event = paymentStateFixture(t, fixture.processor, paymentTransactionStatusCanceled, "2026-09-23T12:01:00Z")
				}
				sendPaymentEventFixture(t, fixture.database, event, transition.eventType, 1)
				before := fixture.funds(t)
				order := paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, fixture.path, "", "", http.StatusOK)
				events := retainedPaymentInbox(t, fixture.database)
				if err := fixture.database.database.Exec("CREATE TRIGGER ignore_payment_state " + boundary.statement + " BEGIN SELECT RAISE(IGNORE); END").Error; err != nil {
					t.Fatal(err)
				}
				for range 2 {
					if boundary.name == "account-lock" {
						fixture.failStartup(t, errFundingNotFound.Error())
					} else {
						fixture.withRuntime(t, fixture.now.Add(10*time.Minute), func(live checkoutRecoveryFixture) {
							current := paymentOrderHTTP(t, live.server, live.cookie, http.MethodGet, live.path, "", "", http.StatusOK)
							if !reflect.DeepEqual(order, current) || !reflect.DeepEqual(before, live.funds(t)) {
								t.Fatal("ignored order update committed a partial financial transition")
							}
						})
					}
				}
				if !reflect.DeepEqual(before, fixture.funds(t)) || fixture.processor.creates.Load() != 1 {
					t.Fatal("ignored payment write changed funds or repeated checkout")
				}
				current := paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, fixture.path, "", "", http.StatusOK)
				if !reflect.DeepEqual(order, current) {
					t.Fatal("ignored payment write changed the pending order")
				}
				paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, fixture.path+"/receipt", "", "", http.StatusNotFound)
				for _, table := range []string{"ledger_accounts", "ledger_entries", "managed_payment_receipt_records", "managed_payment_state_observation_records", "managed_payment_adjustment_records", "managed_payment_adjustment_revision_records"} {
					var count int64
					if err := fixture.database.database.Table(table).Count(&count).Error; err != nil || count != 0 {
						t.Fatalf("ignored payment write retained partial records: table=%s count=%d error=%v", table, count, err)
					}
				}
				retained := retainedPaymentInbox(t, fixture.database)
				if boundary.name == "account-lock" {
					if !reflect.DeepEqual(events, retained) {
						t.Fatal("ignored account lock changed the pending event")
					}
				} else if len(retained) != 1 || retained[0].ID != events[0].ID || retained[0].Payload != events[0].Payload || retained[0].State != paymentInboxReconciliation || retained[0].Reason != transition.conflictReason {
					t.Fatalf("ignored order update lost reconciliation evidence: %+v", retained)
				}
				if err := fixture.database.database.Exec("DROP TRIGGER ignore_payment_state").Error; err != nil {
					t.Fatal(err)
				}
				var recovered map[string]any
				for iteration := range 2 {
					if iteration == 1 {
						sendPaymentEventFixture(t, fixture.database, event, transition.eventType, 2)
					}
					fixture.withRuntime(t, fixture.now.Add(time.Duration(20+iteration*10)*time.Minute), func(live checkoutRecoveryFixture) {
						resources := live.funds(t)
						current := paymentOrderHTTP(t, live.server, live.cookie, http.MethodGet, live.path, "", "", http.StatusOK)
						resources[live.path] = current
						if current["state"] != transition.finalState || fixture.processor.creates.Load() != 1 {
							t.Fatalf("payment recovery did not apply one transition: %v", current)
						}
						if transition.eventType == paymentTransactionCompleted {
							resources[live.path+"/receipt"] = paymentOrderHTTP(t, live.server, live.cookie, http.MethodGet, live.path+"/receipt", "", "", http.StatusOK)
							balance := resources[fundsBalanceTestPath].(map[string]any)
							history := resources["/billing-accounts/billing-journal/ledger-entries?limit=100"].(map[string]any)
							if balance["posted_cents"] != "500" || balance["available_cents"] != "500" || len(history["entries"].([]any)) != 1 {
								t.Fatalf("payment recovery did not credit exactly once: %v", resources)
							}
						} else {
							paymentOrderHTTP(t, live.server, live.cookie, http.MethodGet, live.path+"/receipt", "", "", http.StatusNotFound)
							if !reflect.DeepEqual(before, live.funds(t)) {
								t.Fatal("cancellation recovery changed funds")
							}
						}
						if iteration == 0 {
							recovered = resources
						} else if !reflect.DeepEqual(recovered, resources) {
							t.Fatal("replayed payment event changed the receipt or financial effects")
						}
					})
				}
				for _, record := range retainedPaymentInbox(t, fixture.database) {
					if record.State != paymentInboxApplied || record.Payload != events[0].Payload && record.ID == events[0].ID {
						t.Fatalf("payment recovery did not preserve and apply the retained event: %+v", record)
					}
				}
			})
		}
	}
}
