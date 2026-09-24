package proxy

import (
	"context"
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
	"gorm.io/gorm"
)

type paymentCreditRecoveryFixture struct {
	checkoutRecoveryFixture
	completed map[string]any
	events    []managedPaymentInboxRecord
	before    map[string]any
}

func newPaymentCreditRecoveryFixture(t *testing.T) paymentCreditRecoveryFixture {
	t.Helper()
	fixture := newCheckoutRecoveryFixture(t)
	if err := fixture.worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	completed := completedPaymentFixture(t, fixture.processor)
	sendPaymentEventFixture(t, fixture.database, completed, paymentTransactionCompleted, 1)
	return paymentCreditRecoveryFixture{fixture, completed, retainedPaymentInbox(t, fixture.database), fixture.funds(t)}
}

func (fixture paymentCreditRecoveryFixture) application(t *testing.T, database *gormManagedTenantDatabase) *proxyApplication {
	t.Helper()
	checkout := newPaddleCheckoutDelivery(database, fixture.worker.catalog, fixture.worker.client)
	return &proxyApplication{
		router: fixture.server.Config.Handler.(*gin.Engine), database: database, now: time.Now,
		payments: &paddlePaymentRuntime{checkout: checkout, processor: paymentProcessorFixture(t, checkout, database)},
	}
}

func (fixture paymentCreditRecoveryFixture) failStartup(t *testing.T, reason string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err = fixture.application(t, fixture.database).serve(ctx, listener)
	if err == nil || !strings.Contains(err.Error(), "initialize payment reconciliation") || !strings.Contains(err.Error(), reason) {
		t.Fatalf("credit failure did not stop startup: %v", err)
	}
	if connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
		connection.Close()
		t.Fatal("credit failure retained the HTTP listener")
	}
}

func (fixture paymentCreditRecoveryFixture) assertRolledBack(t *testing.T) {
	t.Helper()
	if !reflect.DeepEqual(fixture.before, fixture.funds(t)) || !reflect.DeepEqual(fixture.events, retainedPaymentInbox(t, fixture.database)) {
		t.Fatal("failed funding changed customer funds or the pending event")
	}
	order := paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, fixture.path, "", "", http.StatusOK)
	if order["state"] != "pending" || fixture.processor.creates.Load() != 1 {
		t.Fatalf("failed funding changed order=%v creates=%d", order, fixture.processor.creates.Load())
	}
	paymentOrderHTTP(t, fixture.server, fixture.cookie, http.MethodGet, fixture.path+"/receipt", "", "", http.StatusNotFound)
	for _, table := range []string{"ledger_accounts", "ledger_entries", "managed_payment_receipt_records", "managed_payment_state_observation_records", "managed_payment_adjustment_records", "managed_payment_adjustment_revision_records"} {
		var count int64
		if err := fixture.database.database.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("partial funding records: table=%s count=%d error=%v", table, count, err)
		}
	}
}

func (fixture paymentCreditRecoveryFixture) recover(t *testing.T) {
	t.Helper()
	var original map[string]any
	for iteration := range 2 {
		if iteration == 1 {
			sendPaymentEventFixture(t, fixture.database, fixture.completed, paymentTransactionCompleted, 2)
		}
		func() {
			database := openJournalTransactionInstance(t, fixture.database)
			application := fixture.application(t, database)
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
					t.Error("recovered credit service did not stop")
				}
			}()
			live := fixture.checkoutRecoveryFixture
			live.server = &httptest.Server{URL: "http://" + listener.Addr().String()}
			current := live.funds(t)
			current[fixture.path] = paymentOrderHTTP(t, live.server, live.cookie, http.MethodGet, fixture.path, "", "", http.StatusOK)
			current[fixture.path+"/receipt"] = paymentOrderHTTP(t, live.server, live.cookie, http.MethodGet, fixture.path+"/receipt", "", "", http.StatusOK)
			balance := current[fundsBalanceTestPath].(map[string]any)
			history := current["/billing-accounts/billing-journal/ledger-entries?limit=100"].(map[string]any)
			if balance["posted_cents"] != "500" || balance["available_cents"] != "500" || len(history["entries"].([]any)) != 1 || fixture.processor.creates.Load() != 1 {
				t.Fatalf("funding recovery did not retain one credit: balance=%v history=%v", balance, history)
			}
			if iteration == 0 {
				original = current
			} else if !reflect.DeepEqual(original, current) {
				t.Fatal("replayed completion changed the receipt or financial effects")
			}
		}()
	}
}

func TestHostedPaymentsCreditWriteFailuresRollBackFinancialTransaction(t *testing.T) {
	for _, scenario := range []struct{ name, statement, reason string }{
		{"account-lock", "BEFORE UPDATE OF id ON managed_billing_account_records", "controlled_credit_write_failure"},
		{"ledger-account", "BEFORE INSERT ON ledger_accounts", "controlled_credit_write_failure"},
		{"ledger-credit", "BEFORE INSERT ON ledger_entries", "store.entry.duplicate"},
		{"adjustment-projection", "BEFORE INSERT ON managed_payment_adjustment_records", "controlled_credit_write_failure"},
		{"adjustment-revision", "BEFORE INSERT ON managed_payment_adjustment_revision_records", "controlled_credit_write_failure"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newPaymentCreditRecoveryFixture(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_credit " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_credit_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			fixture.failStartup(t, scenario.reason)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_credit").Error; err != nil {
				t.Fatal(err)
			}
			fixture.assertRolledBack(t)
			fixture.recover(t)
		})
	}
}

func TestHostedPaymentsCreditReadFailuresRollBackFinancialTransaction(t *testing.T) {
	for _, scenario := range []struct {
		table string
		read  int64
	}{
		{"managed_payment_receipt_records", 1},
		{"managed_payment_state_observation_records", 1},
		{"managed_payment_adjustment_records", 1},
		{"managed_payment_adjustment_records", 2},
		{"ledger_accounts", 1},
	} {
		t.Run(scenario.table+"/"+strconv.FormatInt(scenario.read, 10), func(t *testing.T) {
			fixture := newPaymentCreditRecoveryFixture(t)
			var reads, failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:credit_read", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == scenario.table && reads.Add(1) == scenario.read {
					failures.Add(1)
					tx.AddError(errors.New("controlled_credit_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			fixture.failStartup(t, "controlled_credit_read_failure")
			if err := callback.Remove("test:credit_read"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 {
				t.Fatalf("credit read failures=%d", failures.Load())
			}
			fixture.assertRolledBack(t)
			fixture.recover(t)
		})
	}
}

func TestHostedPaymentsCreditBalanceFailuresRollBackFinancialTransaction(t *testing.T) {
	for _, table := range []string{"ledger_entries", "reservations"} {
		t.Run(table, func(t *testing.T) {
			fixture := newPaymentCreditRecoveryFixture(t)
			var failures atomic.Int64
			callback := fixture.database.database.Callback().Row()
			if err := callback.Before("gorm:row").Register("test:credit_balance", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == table {
					failures.Add(1)
					tx.AddError(errors.New("controlled_credit_balance_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			fixture.failStartup(t, "controlled_credit_balance_failure")
			if err := callback.Remove("test:credit_balance"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() == 0 {
				t.Fatal("credit balance failure was not exercised")
			}
			fixture.assertRolledBack(t)
			fixture.recover(t)
		})
	}
}
