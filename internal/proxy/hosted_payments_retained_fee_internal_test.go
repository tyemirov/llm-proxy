package proxy

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHostedPaymentsAdjustedRetainedFeeEvidence(t *testing.T) {
	for _, scenario := range []struct {
		field, fee string
		accepted   bool
	}{
		{"retained_fee", "-31", true}, {"retained_fee", "0", true}, {"retained_fee", "31", true},
		{"retained_fee", "-0", false}, {"retained_fee", "-01", false}, {"retained_fee", "0.31", false}, {"retained_fee", "-0.31", false}, {"retained_fee", "", false}, {"retained_fee", "+31", false},
		{"subtotal", "-300", false}, {"adjustment_retained_fee", "-31", false},
	} {
		t.Run(scenario.field+"="+scenario.fee, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "retained-fee", `{"offer_code":"five"}`, http.StatusCreated)
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
			adjustment := paymentAdjustmentFixture(processor, "approved", 200, 2)
			processor.mutex.Lock()
			adjusted := transaction["details"].(map[string]any)["adjusted_totals"].(map[string]any)
			adjusted["retained_fee"] = "-31"
			adjustment["totals"].(map[string]any)["retained_fee"] = "31"
			if scenario.field == "adjustment_retained_fee" {
				adjustment["totals"].(map[string]any)["retained_fee"] = scenario.fee
			} else {
				adjusted[scenario.field] = scenario.fee
			}
			processor.mutex.Unlock()
			sendPaymentEventFixture(t, database, adjustment, "adjustment.updated", 3)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			want, reversed, state := "500", "0", "paid"
			reserved, pendingCents := "200", "200"
			if scenario.accepted {
				want, reversed, state = "300", "200", "partially_refunded"
				reserved, pendingCents = "0", "0"
				// Replayed processor evidence cannot repeat the customer reversal.
				sendPaymentEventFixture(t, database, adjustment, "adjustment.updated", 3)
				if err := paymentProcessorFixture(t, checkout, openJournalTransactionInstance(t, database)).reconcile(t.Context()); err != nil {
					t.Fatal(err)
				}
				var retained managedPaymentAdjustmentRecord
				if err := database.database.Where("order_id = ?", order["id"]).First(&retained).Error; err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(retained.Evidence, `"retained_fee":"`+scenario.fee+`"`) {
					t.Fatalf("lost signed retained fee: %s", retained.Evidence)
				}
			}
			balance := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
			if balance["posted_cents"] != want || balance["available_cents"] != "300" || balance["reserved_cents"] != reserved {
				t.Fatalf("retained fee changed customer principal: %v; want %s", balance, want)
			}
			receipt := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string)+"/receipt", "", "", http.StatusOK)
			if receipt["state"] != state || receipt["credit_cents"] != "500" || receipt["reversed_cents"] != reversed || receipt["pending_refund_cents"] != pendingCents {
				t.Fatalf("receipt=%v", receipt)
			}
		})
	}
}
