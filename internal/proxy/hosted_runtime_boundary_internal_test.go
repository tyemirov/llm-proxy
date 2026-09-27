package proxy

import (
	"context"
	"errors"
	"net"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestHostedRuntimeOccupiedPortPreservesFundedAccounts(t *testing.T) {
	fixture := newPaymentStartupFixture(t)
	configuration := fixture.configuration
	dialector := &lifecycleUsageDialector{Dialector: configuration.Management.DatabaseDialector}
	configuration.Management.DatabaseDialector = dialector
	for range 2 {
		fixture.reject(t, configuration, "listen for proxy requests")
		if dialector.connection == nil {
			t.Fatal("occupied port test did not open its database")
		}
		if err := dialector.connection.PingContext(t.Context()); err == nil {
			t.Fatal("listen failure retained the owned database")
		}
		if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
			t.Fatal("listen failure changed funded accounts or receipts")
		}
	}
	fixture.recover(t)
}

type runtimeCloseFailureListener struct {
	net.Listener
	failure error
}

func (listener runtimeCloseFailureListener) Close() error {
	return errors.Join(listener.Listener.Close(), listener.failure)
}

func TestHostedRuntimeListenerFailuresPreserveFundedAccounts(t *testing.T) {
	for _, scenario := range []string{"accept", "shutdown"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPaymentStartupFixture(t)
			application, err := buildProxyApplicationForTest(t, fixture.configuration, zap.NewNop().Sugar(), newManagedTenantStore)
			if err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			var serving net.Listener = listener
			failure := error(net.ErrClosed)
			reason := "serve proxy requests"
			if scenario == "shutdown" {
				failure = errors.New("controlled_runtime_listener_close_failure")
				serving = runtimeCloseFailureListener{Listener: listener, failure: failure}
				reason = "stop proxy requests"
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			stopped := make(chan error, 1)
			go func() { stopped <- application.serve(ctx, serving) }()
			live := fixture.paymentAuditFixture
			live.server = &httptest.Server{URL: "http://" + listener.Addr().String()}
			if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, live)) {
				t.Fatal("runtime startup changed funded accounts or receipts")
			}
			if scenario == "accept" {
				if err := listener.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			select {
			case err := <-stopped:
				if !errors.Is(err, failure) || !strings.Contains(err.Error(), reason) {
					t.Fatalf("runtime listener failure=%v want=%s: %v", err, reason, failure)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("listener failure left the runtime active")
			}
			assertHostedRuntimeBoundaryClosed(t, application, listener)
			if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
				t.Fatal("listener failure changed funded accounts or receipts")
			}
			fixture.recover(t)
		})
	}
}

func TestHostedRuntimeCancellationDuringFinancialReadPreservesFunds(t *testing.T) {
	for _, scenario := range []struct{ name, table string }{
		{"funds", "managed_journal_observation_records"},
		{"checkout", "managed_payment_delivery_records"},
		{"payment-events", "managed_payment_inbox_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newPaymentStartupFixture(t)
			configuration := fixture.configuration
			var armed atomic.Bool
			var interrupted atomic.Int64
			entered := make(chan struct{})
			dialector := &lifecycleUsageDialector{
				Dialector: configuration.Management.DatabaseDialector,
				configure: func(database *gorm.DB) error {
					return database.Callback().Query().Before("gorm:query").Register("test:runtime_read_cancellation", func(tx *gorm.DB) {
						if tx.Statement.Table != scenario.table || !armed.CompareAndSwap(true, false) {
							return
						}
						close(entered)
						<-tx.Statement.Context.Done()
						interrupted.Add(1)
						tx.AddError(tx.Statement.Context.Err())
					})
				},
			}
			configuration.Management.DatabaseDialector = dialector
			application, err := buildProxyApplicationForTest(t, configuration, zap.NewNop().Sugar(), newManagedTenantStore)
			if err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			stopped := make(chan error, 1)
			go func() { stopped <- application.serve(ctx, listener) }()
			live := fixture.paymentAuditFixture
			live.server = &httptest.Server{URL: "http://" + listener.Addr().String()}
			if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, live)) {
				t.Fatal("runtime startup changed funded accounts or receipts")
			}
			armed.Store(true)
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("runtime did not start its financial read")
			}
			cancel()
			select {
			case err := <-stopped:
				if err != nil {
					t.Fatalf("financial read cancellation failed shutdown: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("financial read cancellation left the runtime active")
			}
			if interrupted.Load() != 1 {
				t.Fatalf("interrupted reads=%d want=1", interrupted.Load())
			}
			assertHostedRuntimeBoundaryClosed(t, application, listener)
			if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
				t.Fatal("financial read cancellation changed funded accounts or receipts")
			}
			fixture.recover(t)
		})
	}
}

func assertHostedRuntimeBoundaryClosed(t *testing.T, application *proxyApplication, listener net.Listener) {
	t.Helper()
	connection, err := application.database.(*gormManagedTenantDatabase).database.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.PingContext(t.Context()); err == nil {
		t.Fatal("stopped runtime retained its owned database")
	}
	if connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
		connection.Close()
		t.Fatal("stopped runtime retained its listener")
	}
}
