package proxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/utils/billing"
	"gorm.io/gorm"
)

func pendingPaymentStateFixture(t *testing.T) checkoutRecoveryFixture {
	t.Helper()
	fixture := newCheckoutRecoveryFixture(t)
	if err := fixture.worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture checkoutRecoveryFixture) withRuntime(t *testing.T, now time.Time, check func(checkoutRecoveryFixture)) {
	t.Helper()
	application := fixture.application(t, openJournalTransactionInstance(t, fixture.database))
	application.payments.processor.now = func() time.Time { return now }
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- application.serve(ctx, listener) }()
	defer func() {
		cancel()
		select {
		case err := <-stopped:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("payment recovery runtime did not stop")
		}
	}()
	fixture.server = &httptest.Server{URL: "http://" + listener.Addr().String()}
	check(fixture)
}

func TestHostedPaymentsStateStorageFailuresPreservePendingEvent(t *testing.T) {
	for _, scenario := range []struct{ name, statement, table string }{
		{"account-lock", "BEFORE UPDATE OF id ON managed_billing_account_records", ""},
		{"state-observation", "BEFORE INSERT ON managed_payment_state_observation_records", ""},
		{"order-state", "BEFORE UPDATE OF state ON managed_funding_order_records", ""},
		{"inbox-state", "BEFORE UPDATE OF state ON managed_payment_inbox_records", ""},
		{"observation-read", "", "managed_payment_state_observation_records"},
		{"receipt-read", "", "managed_payment_receipt_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := pendingPaymentStateFixture(t)
			canceled := paymentStateFixture(t, fixture.processor, paymentTransactionStatusCanceled, "2026-09-23T12:01:00Z")
			sendPaymentEventFixture(t, fixture.database, canceled, "transaction.canceled", 1)
			before := fixture.funds(t)
			admissions := retainedFundingAdmissions(t, fixture.database)
			events := retainedPaymentInbox(t, fixture.database)
			var failures atomic.Int64
			const callback = "test:payment_state_read_failure"
			if scenario.table != "" {
				if err := fixture.database.database.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
					if !tx.DryRun && tx.Statement.Table == scenario.table {
						failures.Add(1)
						tx.AddError(errors.New("controlled_payment_state_failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			} else if err := fixture.database.database.Exec("CREATE TRIGGER reject_state_recovery " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_payment_state_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			fixture.failStartup(t, "controlled_payment_state_failure")
			if scenario.table != "" {
				if err := fixture.database.database.Callback().Query().Remove(callback); err != nil {
					t.Fatal(err)
				}
				if failures.Load() != 1 {
					t.Fatalf("state read failure count=%d", failures.Load())
				}
			} else if err := fixture.database.database.Exec("DROP TRIGGER reject_state_recovery").Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, fixture.funds(t)) || !reflect.DeepEqual(events, retainedPaymentInbox(t, fixture.database)) || !reflect.DeepEqual(admissions, retainedFundingAdmissions(t, fixture.database)) {
				t.Fatal("failed state processing changed pending evidence or financial resources")
			}
			var count int64
			if err := fixture.database.database.Model(&managedPaymentStateObservationRecord{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("partial state observation count=%d error=%v", count, err)
			}
			for range 2 {
				fixture.withRuntime(t, time.Now(), func(live checkoutRecoveryFixture) {
					order := paymentOrderHTTP(t, live.server, live.cookie, http.MethodGet, live.path, "", "", http.StatusOK)
					if order["state"] != fundingOrderFailed || !reflect.DeepEqual(before, live.funds(t)) || fixture.processor.creates.Load() != 1 {
						t.Fatalf("cancellation recovery changed funds or repeated checkout: %v", order)
					}
					paymentOrderHTTP(t, live.server, live.cookie, http.MethodGet, live.path+"/receipt", "", "", http.StatusNotFound)
				})
			}
			if err := fixture.database.database.Model(&managedPaymentStateObservationRecord{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("cancellation replay observation count=%d error=%v", count, err)
			}
			after := retainedPaymentInbox(t, fixture.database)
			if len(after) != 1 || after[0].State != paymentInboxApplied || after[0].Payload != events[0].Payload {
				t.Fatalf("cancellation did not preserve and apply its event: %+v", after)
			}
		})
	}
}

func TestHostedPaymentsStateUnverifiedEvidenceRetainsReconciliation(t *testing.T) {
	for _, scenario := range []struct{ name, reason string }{
		{"malformed-event", "transaction_event_invalid"},
		{"processor-outage", "transaction_unavailable"},
		{"incomplete-completion", "transaction_mismatch"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := pendingPaymentStateFixture(t)
			event := paymentStateFixture(t, fixture.processor, paymentTransactionStatusCanceled, "2026-09-23T12:01:00Z")
			before := fixture.funds(t)
			var unavailable atomic.Bool
			var failedReads atomic.Int64
			switch scenario.name {
			case "malformed-event":
				event["status"] = []any{"canceled"}
			case "processor-outage":
				unavailable.Store(true)
				processor := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if unavailable.Load() && strings.HasPrefix(request.URL.Path, "/transactions/") {
						failedReads.Add(1)
						writer.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					fixture.processor.server.Config.Handler.ServeHTTP(writer, request)
				}))
				t.Cleanup(processor.Close)
				client, err := billing.NewPaddleCommerceClient("sandbox", "checkout-fixture-key", processor.URL, processor.Client())
				if err != nil {
					t.Fatal(err)
				}
				fixture.worker.client = client
			case "incomplete-completion":
				paymentStateFixture(t, fixture.processor, paymentTransactionStatusCompleted, "2026-09-23T12:02:00Z")
			}
			sendPaymentEventFixture(t, fixture.database, event, "transaction.canceled", 1)
			fixture.withRuntime(t, fixture.now.Add(10*time.Minute), func(live checkoutRecoveryFixture) {
				order := paymentOrderHTTP(t, live.server, live.cookie, http.MethodGet, live.path, "", "", http.StatusOK)
				if order["state"] != fundingOrderPending || !reflect.DeepEqual(before, live.funds(t)) {
					t.Fatalf("unverified evidence changed funds or order: %v", order)
				}
			})
			retained := retainedPaymentInbox(t, fixture.database)
			if len(retained) != 1 || retained[0].State != paymentInboxReconciliation || retained[0].Reason != scenario.reason {
				t.Fatalf("unverified evidence lacks reconciliation reason: %+v", retained)
			}
			if scenario.name == "processor-outage" && failedReads.Load() == 0 {
				t.Fatal("processor outage was not exercised")
			}
			unavailable.Store(false)
			canceled := paymentStateFixture(t, fixture.processor, paymentTransactionStatusCanceled, "2026-09-23T12:03:00Z")
			sendPaymentEventFixture(t, fixture.database, canceled, "transaction.canceled", 2)
			fixture.withRuntime(t, fixture.now.Add(20*time.Minute), func(live checkoutRecoveryFixture) {
				order := paymentOrderHTTP(t, live.server, live.cookie, http.MethodGet, live.path, "", "", http.StatusOK)
				if order["state"] != fundingOrderFailed || !reflect.DeepEqual(before, live.funds(t)) || fixture.processor.creates.Load() != 1 {
					t.Fatalf("restored evidence did not apply cancellation without financial effects: %v", order)
				}
			})
			after := retainedPaymentInbox(t, fixture.database)
			for _, record := range after {
				want := paymentInboxApplied
				if scenario.name == "malformed-event" && record.ID == retained[0].ID {
					want = paymentInboxReconciliation
				}
				if record.State != want {
					t.Fatalf("recovered state event=%s want=%s", record.State, want)
				}
			}
		})
	}
}

func TestHostedPaymentsStateCancellationCannotReverseVerifiedFunding(t *testing.T) {
	funded := newPaymentAuditFixture(t)
	before := paymentAdjustmentResources(t, funded)
	fixture := checkoutRecoveryFixture{
		database: funded.database, server: funded.server, cookie: funded.cookie("owner"),
		processor: funded.processor, worker: funded.checkout,
		path: paymentOrdersTestPath + "/" + funded.orderID, now: time.Now(),
	}
	var original []managedPaymentStateObservationRecord
	if err := fixture.database.database.Order("id").Find(&original).Error; err != nil {
		t.Fatal(err)
	}
	canceled := paymentStateFixture(t, fixture.processor, paymentTransactionStatusCanceled, "2026-09-23T12:01:00Z")
	sendPaymentEventFixture(t, fixture.database, canceled, "transaction.canceled", 2)
	fixture.withRuntime(t, fixture.now.Add(10*time.Minute), func(live checkoutRecoveryFixture) {
		funded.server = live.server
		if !reflect.DeepEqual(before, paymentAdjustmentResources(t, funded)) {
			t.Fatal("lifecycle cancellation reversed verified funding")
		}
	})
	var retained []managedPaymentStateObservationRecord
	if err := fixture.database.database.Order("id").Find(&retained).Error; err != nil || !reflect.DeepEqual(original, retained) {
		t.Fatalf("conflicting state committed a new observation: error=%v", err)
	}
	events := retainedPaymentInbox(t, fixture.database)
	var conflict managedPaymentInboxRecord
	for _, event := range events {
		if event.EventType == "transaction.canceled" {
			conflict = event
		}
	}
	if conflict.State != paymentInboxReconciliation || conflict.Reason != "transaction_state_conflict" {
		t.Fatalf("cancellation conflict was not retained: %+v", conflict)
	}
	paymentStateFixture(t, fixture.processor, paymentTransactionStatusCompleted, "2026-09-23T12:02:00Z")
	for iteration := range 2 {
		fixture.withRuntime(t, fixture.now.Add(time.Duration(20+iteration*10)*time.Minute), func(live checkoutRecoveryFixture) {
			funded.server = live.server
			if !reflect.DeepEqual(before, paymentAdjustmentResources(t, funded)) || fixture.processor.creates.Load() != 1 {
				t.Fatal("state reconciliation changed funding or repeated checkout")
			}
		})
	}
	for _, event := range retainedPaymentInbox(t, fixture.database) {
		if event.State != paymentInboxApplied {
			t.Fatalf("restored completed evidence did not resolve state event: %+v", event)
		}
	}
}
