package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHostedFundsPaymentReversalDuringExecutionRetainsDeficit(t *testing.T) {
	for _, action := range []string{"refund", "chargeback"} {
		t.Run(action, func(t *testing.T) {
			acceptedAt := time.Now().UTC()
			database, _, management, _ := newHostedRatingFixture(t)
			prices := hostedRatingFixtureAdmissionAt(t, 2, acceptedAt)
			service, orders, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			paymentOrderHTTP(t, orders, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "inflight-reversal", `{"offer_code":"five"}`, http.StatusCreated)
			checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
			if err := checkout.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			sendPaymentEventFixture(t, database, completedPaymentFixture(t, processor), "transaction.completed", 1)
			worker := paymentProcessorFixture(t, checkout, database)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodDelete {
					writer.WriteHeader(http.StatusNoContent)
					return
				}
				calls.Add(1)
				adjustment := paymentAdjustmentFixture(processor, "approved", 500, 1)
				processor.mutex.Lock()
				adjustment["action"] = action
				processor.mutex.Unlock()
				sendPaymentEventFixture(t, database, adjustment, "adjustment.created", 2)
				if err := worker.reconcile(t.Context()); err != nil {
					t.Error(err)
					writer.WriteHeader(http.StatusInternalServerError)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(writer, `{"id":"reversed-payment-result","status":"completed","output_text":"funded result","usage":`+startupCompleteUsage+`}`)
			}))
			t.Cleanup(upstream.Close)
			generation := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), func(dependencies *hostedTextRequestDependencies) {
				fundsDependencies(prices)(dependencies)
				dependencies.now = func() time.Time { return acceptedAt }
			})
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			stopped := make(chan error, 1)
			application := &proxyApplication{closeStore: func() error { return nil }, router: management.Config.Handler.(*gin.Engine), database: database, now: time.Now}
			go func() { stopped <- application.serve(ctx, listener) }()
			t.Cleanup(func() {
				cancel()
				if err := <-stopped; err != nil {
					t.Errorf("service stopped: %v", err)
				}
			})
			hostedIdentityHTTP(t, generation, "inflight-reversal", "funded prompt", http.StatusOK)
			var reservation managedFundsReservationRecord
			deadline := time.Now().Add(5 * time.Second)
			for {
				if err := database.database.First(&reservation).Error; err != nil {
					t.Fatal(err)
				}
				if reservation.State == fundsReservationReconciliation {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("deficit did not enter reconciliation: state=%s", reservation.State)
				}
				time.Sleep(10 * time.Millisecond)
			}
			client := &http.Client{Timeout: time.Second}
			response, err := client.Get("http://" + listener.Addr().String() + managementAPIPath + fundsBalanceTestPath)
			if err != nil {
				t.Fatalf("deficit stopped HTTP: %v", err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("balance status=%d", response.StatusCode)
			}
			fixture := fundsStartupFixture{database, management, &calls}
			before := fixture.state(t)
			balance := before["balance"].(map[string]any)
			if balance["posted_cents"] != "0" || balance["reserved_cents"] != "3" || balance["pending_cents"] != "3" || balance["spent_cents"] != "0" {
				t.Fatalf("deficit lost funds evidence: %v", balance)
			}
			summary := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+reservation.RequestID+"/charge-summary", "", http.StatusOK)
			if summary["state"] != "rated" {
				t.Fatalf("deficit lost rated usage: %v", summary)
			}
			var pending, settlements int64
			if err := database.database.Model(&managedJournalDeliveryRecord{}).Where("delivered_at IS NULL").Count(&pending).Error; err != nil || pending != 0 {
				t.Fatalf("delivery not acknowledged: count=%d error=%v", pending, err)
			}
			if err := database.database.Model(&managedFundsSettlementRecord{}).Count(&settlements).Error; err != nil || settlements != 0 {
				t.Fatalf("deficit recorded settlement: count=%d error=%v", settlements, err)
			}
			assertFundsCreditRemainder(t, database, "0", "1")
			var cases []managedJournalCaseRecord
			if err := database.database.Where("request_id = ?", reservation.RequestID).Find(&cases).Error; err != nil || len(cases) != 1 || cases[0].Reason != "settlement_insufficient_funds" {
				t.Fatalf("deficit cases=%+v error=%v", cases, err)
			}
			for range 2 {
				restartFundsApplication(t, database, management, time.Now)
				if !reflect.DeepEqual(before, fixture.state(t)) {
					t.Fatal("restart changed deficit evidence")
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("deficit repeated provider execution: calls=%d", calls.Load())
			}
		})
	}
}
