package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/utils/billing"
	"gorm.io/gorm"
)

type checkoutRecoveryFixture struct {
	database  *gormManagedTenantDatabase
	server    *httptest.Server
	cookie    *http.Cookie
	processor *checkoutProtocolFixture
	worker    *paddleCheckoutDelivery
	order     map[string]any
	path      string
	now       time.Time
}

func newCheckoutRecoveryFixture(t *testing.T) checkoutRecoveryFixture {
	t.Helper()
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "recover-checkout", `{"offer_code":"five"}`, http.StatusCreated)
	now := time.Now()
	worker := checkoutWorkerFixture(t, database, service.funding, processor, now)
	return checkoutRecoveryFixture{database, server, cookie("owner"), processor, worker, order, paymentOrdersTestPath + "/" + order["id"].(string), now}
}

func (fixture checkoutRecoveryFixture) application(t *testing.T, database *gormManagedTenantDatabase) *proxyApplication {
	t.Helper()
	checkout := newPaddleCheckoutDelivery(database, fixture.worker.catalog, fixture.worker.client)
	return &proxyApplication{
		router: fixture.server.Config.Handler.(*gin.Engine), database: database, now: time.Now,
		payments: &paddlePaymentRuntime{checkout: checkout, processor: paymentProcessorFixture(t, checkout, database)},
	}
}

func (fixture checkoutRecoveryFixture) failStartup(t *testing.T, reason string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err = fixture.application(t, fixture.database).serve(ctx, listener)
	if err == nil || !strings.Contains(err.Error(), "initialize payment reconciliation") || !strings.Contains(err.Error(), reason) {
		t.Fatalf("payment failure did not stop startup: %v", err)
	}
	if connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
		connection.Close()
		t.Fatal("payment failure retained the HTTP listener")
	}
}

func (fixture checkoutRecoveryFixture) funds(t *testing.T) map[string]any {
	t.Helper()
	result := map[string]any{}
	for _, path := range []string{fundsBalanceTestPath, "/billing-accounts/billing-journal/ledger-entries?limit=100"} {
		result[path] = paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, path, "", "", http.StatusOK)
	}
	return result
}

func (fixture checkoutRecoveryFixture) assertUnavailable(t *testing.T, before map[string]any, creates int64) {
	t.Helper()
	paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, fixture.path+"/checkout", "", "", http.StatusNotFound)
	order := paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, fixture.path, "", "", http.StatusOK)
	if !reflect.DeepEqual(order, fixture.order) || !reflect.DeepEqual(before, fixture.funds(t)) || fixture.processor.creates.Load() != creates {
		t.Fatalf("failed checkout changed public state: order=%v creates=%d", order, fixture.processor.creates.Load())
	}
}

func (fixture checkoutRecoveryFixture) recover(t *testing.T, before map[string]any) {
	t.Helper()
	worker := checkoutWorkerFixture(t, openJournalTransactionInstance(t, fixture.database), fixture.worker.catalog, fixture.processor, fixture.now.Add(2*time.Minute))
	for range 2 {
		if err := worker.reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		checkout := paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, fixture.path+"/checkout", "", "", http.StatusOK)
		if checkout["transaction_id"] != "txn_00000000000000000000000001" || fixture.processor.creates.Load() != 1 || !reflect.DeepEqual(before, fixture.funds(t)) {
			t.Fatalf("checkout recovery repeated effects: checkout=%v creates=%d", checkout, fixture.processor.creates.Load())
		}
	}
	order := paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, fixture.path, "", "", http.StatusOK)
	if order["state"] != "pending" || order["id"] != fixture.order["id"] {
		t.Fatalf("recovered order=%v", order)
	}
	completed := completedPaymentFixture(t, fixture.processor)
	var funded map[string]any
	for sequence := 1; sequence <= 2; sequence++ {
		sendPaymentEventFixture(t, fixture.database, completed, "transaction.completed", sequence)
		if err := paymentProcessorFixture(t, worker, worker.database).reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		balance := paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
		if balance["posted_cents"] != "500" || balance["available_cents"] != "500" {
			t.Fatalf("recovered payment balance=%v", balance)
		}
		current := fixture.funds(t)
		if sequence == 1 {
			funded = current
		} else if !reflect.DeepEqual(funded, current) {
			t.Fatal("replayed completion changed Ledger history")
		}
	}
}

func TestHostedPaymentsCheckoutWriteFailuresRecoverWithoutDuplicateCreation(t *testing.T) {
	for _, scenario := range []struct {
		name, statement string
		creates         int64
		wantError       bool
	}{
		{"claim", "BEFORE UPDATE ON managed_payment_delivery_records WHEN NEW.owner_token != OLD.owner_token", 0, true},
		{"customer", "BEFORE INSERT ON managed_payment_customer_records", 0, false},
		{"dispatch-intent", "BEFORE UPDATE ON managed_payment_delivery_records WHEN NEW.transaction_dispatched_at IS NOT NULL", 0, true},
		{"delivery-completion", "BEFORE UPDATE ON managed_payment_delivery_records WHEN NEW.state = 'delivered'", 1, true},
		{"order-completion", "BEFORE UPDATE ON managed_funding_order_records WHEN NEW.state = 'pending'", 1, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newCheckoutRecoveryFixture(t)
			before := fixture.funds(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_checkout_write " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_checkout_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			err := fixture.worker.reconcile(t.Context())
			if (err != nil) != scenario.wantError {
				t.Fatalf("checkout error=%v wantError=%v", err, scenario.wantError)
			}
			fixture.assertUnavailable(t, before, scenario.creates)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_checkout_write").Error; err != nil {
				t.Fatal(err)
			}
			fixture.recover(t, before)
		})
	}
}

func TestHostedPaymentsCheckoutReadFailuresPreserveOrder(t *testing.T) {
	for _, scenario := range []struct {
		table     string
		read      int64
		wantError bool
	}{
		{"managed_payment_delivery_records", 1, true},
		{"managed_funding_order_records", 1, true},
		{"managed_payment_customer_records", 1, false},
		{"managed_payment_customer_records", 2, false},
		{"managed_user_records", 1, false},
	} {
		t.Run(scenario.table+"/"+strconv.FormatInt(scenario.read, 10), func(t *testing.T) {
			fixture := newCheckoutRecoveryFixture(t)
			before := fixture.funds(t)
			var reads, failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:checkout_read", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == scenario.table && reads.Add(1) == scenario.read {
					failures.Add(1)
					tx.AddError(errors.New("controlled_checkout_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			err := fixture.worker.reconcile(t.Context())
			if removeError := callback.Remove("test:checkout_read"); removeError != nil {
				t.Fatal(removeError)
			}
			if (err != nil) != scenario.wantError || failures.Load() != 1 {
				t.Fatalf("checkout error=%v wantError=%v failures=%d", err, scenario.wantError, failures.Load())
			}
			fixture.assertUnavailable(t, before, 0)
			fixture.recover(t, before)
		})
	}
}

func TestHostedPaymentsCheckoutCustomerCreationSurvivesLostAcknowledgment(t *testing.T) {
	for _, failure := range []string{"processor-response", "local-binding"} {
		t.Run(failure, func(t *testing.T) {
			fixture := newCheckoutRecoveryFixture(t)
			before := fixture.funds(t)
			var customerCreates atomic.Int64
			var ownerEmail atomic.Value
			original := fixture.processor.server
			processor := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/customers" {
					original.Config.Handler.ServeHTTP(writer, request)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				if request.Method == http.MethodGet {
					if customerCreates.Load() != 0 {
						if request.URL.Query().Get("email") != ownerEmail.Load().(string) {
							t.Error("customer recovery changed owner email")
						}
						original.Config.Handler.ServeHTTP(writer, request)
					} else if err := json.NewEncoder(writer).Encode(map[string]any{"data": []any{}}); err != nil {
						t.Error(err)
					}
					return
				}
				var input struct {
					Email string `json:"email"`
				}
				if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&input) != nil || input.Email == "" {
					t.Error("invalid processor customer creation")
					writer.WriteHeader(http.StatusBadRequest)
					return
				}
				ownerEmail.Store(input.Email)
				customerCreates.Add(1)
				if failure == "processor-response" {
					connection, _, err := writer.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					if err := connection.Close(); err != nil {
						t.Error(err)
					}
					return
				}
				if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": "ctm_01hv8x2axb33yr5y238zfwcn5p"}}); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(processor.Close)
			fixture.processor.server = processor
			fixture.worker = checkoutWorkerFixture(t, fixture.database, fixture.worker.catalog, fixture.processor, fixture.now)
			if failure == "local-binding" {
				if err := fixture.database.database.Exec("CREATE TRIGGER reject_new_customer BEFORE INSERT ON managed_payment_customer_records BEGIN SELECT RAISE(ABORT, 'controlled_new_customer_failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := fixture.worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			fixture.assertUnavailable(t, before, 0)
			if failure == "local-binding" {
				if err := fixture.database.database.Exec("DROP TRIGGER reject_new_customer").Error; err != nil {
					t.Fatal(err)
				}
			}
			fixture.recover(t, before)
			if customerCreates.Load() != 1 {
				t.Fatalf("customer creation repeated: %d", customerCreates.Load())
			}
		})
	}
}

func TestHostedPaymentsCheckoutProcessorFailuresRetainRetryIdentity(t *testing.T) {
	for _, scenario := range []struct {
		name, reason string
		creates      int64
	}{
		{"customer-unavailable", "customer_unavailable", 0},
		{"customer-invalid", "customer_unavailable", 0},
		{"transaction-read", "transaction_unavailable", 1},
		{"transaction-list", "transaction_unavailable", 1},
		{"retry-checkpoint", "", 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newCheckoutRecoveryFixture(t)
			before := fixture.funds(t)
			var failures atomic.Int64
			fault := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				customer := request.URL.Path == "/customers" && strings.HasPrefix(scenario.name, "customer-")
				transaction := strings.HasPrefix(request.URL.Path, "/transactions/") && scenario.name == "transaction-read"
				list := request.URL.Path == "/transactions" && request.Method == http.MethodGet && scenario.name == "transaction-list"
				if customer || transaction || list {
					failures.Add(1)
					writer.Header().Set("Content-Type", "application/json")
					if scenario.name == "customer-invalid" {
						if err := json.NewEncoder(writer).Encode(map[string]any{"data": []any{map[string]any{"id": "ctm_invalid"}}}); err != nil {
							t.Error(err)
						}
					} else {
						writer.WriteHeader(http.StatusServiceUnavailable)
					}
					return
				}
				fixture.processor.server.Config.Handler.ServeHTTP(writer, request)
			}))
			t.Cleanup(fault.Close)
			client, err := billing.NewPaddleCommerceClient("sandbox", "checkout-fixture-key", fault.URL, fault.Client())
			if err != nil {
				t.Fatal(err)
			}
			fixture.worker.client = client
			if scenario.name == "transaction-list" || scenario.name == "retry-checkpoint" {
				fixture.processor.dropResponse.Store(true)
			}
			if scenario.name == "retry-checkpoint" {
				if err := fixture.database.database.Exec("CREATE TRIGGER reject_checkout_retry BEFORE UPDATE ON managed_payment_delivery_records WHEN NEW.state = 'reconciliation_required' BEGIN SELECT RAISE(ABORT, 'controlled_checkout_retry_failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			err = fixture.worker.reconcile(t.Context())
			if (err != nil) != (scenario.name == "retry-checkpoint") {
				t.Fatalf("checkout failure=%v", err)
			}
			if scenario.name == "transaction-list" {
				fixture.worker.now = func() time.Time { return fixture.now.Add(paymentCheckoutRetry) }
				if err := fixture.worker.reconcile(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.name == "retry-checkpoint" {
				if err := fixture.database.database.Exec("DROP TRIGGER reject_checkout_retry").Error; err != nil {
					t.Fatal(err)
				}
			} else if failures.Load() == 0 {
				t.Fatalf("processor failures=%d", failures.Load())
			}
			fixture.assertUnavailable(t, before, scenario.creates)
			var delivery managedPaymentDeliveryRecord
			if err := fixture.database.database.Where("order_id = ?", fixture.order["id"]).First(&delivery).Error; err != nil || delivery.Reason != scenario.reason || (delivery.TransactionDispatchedAt != nil) != (scenario.creates == 1) {
				t.Fatalf("retry evidence=%+v error=%v", delivery, err)
			}
			fixture.recover(t, before)
		})
	}
}
