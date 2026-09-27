package proxy

import (
	"errors"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHostedPaymentsCheckoutIgnoredWritesStopStartupAndRecover(t *testing.T) {
	for _, scenario := range []struct {
		name, condition string
		creates         int64
	}{
		{"dispatch-intent", "NEW.transaction_dispatched_at IS NOT NULL", 0},
		{"uncertain-outcome", "NEW.state = 'reconciliation_required'", 1},
		{"delivery-completion", "NEW.state = 'delivered'", 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newCheckoutRecoveryFixture(t)
			before := fixture.funds(t)
			if scenario.name == "uncertain-outcome" {
				fixture.processor.dropResponse.Store(true)
			}
			if err := fixture.database.database.Exec("CREATE TRIGGER ignore_checkout_write BEFORE UPDATE ON managed_payment_delivery_records WHEN " + scenario.condition + " BEGIN SELECT RAISE(IGNORE); END").Error; err != nil {
				t.Fatal(err)
			}
			fixture.rejectPublicStartup(t)
			fixture.assertUnavailable(t, before, scenario.creates)
			if err := fixture.database.database.Exec("DROP TRIGGER ignore_checkout_write").Error; err != nil {
				t.Fatal(err)
			}
			fixture.processor.dropResponse.Store(false)
			fixture.recover(t, before)
		})
	}
}

func (fixture checkoutRecoveryFixture) rejectPublicStartup(t *testing.T) {
	t.Helper()
	service, _, _ := paymentOrdersFixture(t, fixture.database)
	management, payments := paymentReconciliationConfiguration(t, fixture.database, fixture.processor)
	var source struct{ File string }
	if err := fixture.database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	configuration := withInternalUpstreamCapacity(t, Configuration{
		Management: service.configuration, ProviderCatalog: internalCanonicalProviderCatalog(),
		AssetStorePath: t.TempDir(), Payments: payments,
	})
	configuration.Management.DatabaseDialector = management.DatabaseDialector
	configuration.Management.DatabasePath = source.File
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	configuration.Port = listener.Addr().(*net.TCPAddr).Port
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	// Keep a second signal handler until Serve exits, including on failure.
	observed := make(chan os.Signal, 1)
	signal.Notify(observed, syscall.SIGTERM)
	defer signal.Stop(observed)
	stopped := make(chan error, 1)
	go func() { stopped <- Serve(configuration, zap.NewNop().Sugar()) }()
	select {
	case err := <-stopped:
		if !errors.Is(err, errFundingConflict) || !strings.Contains(err.Error(), "initialize payment reconciliation") {
			t.Fatalf("ignored checkout write did not stop public startup: %v", err)
		}
	case <-time.After(5 * time.Second):
		process, err := os.FindProcess(os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		if err := process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("public Serve did not stop after termination")
		}
		t.Fatal("ignored checkout write did not stop startup")
	}
	if connection, err := net.DialTimeout("tcp", address, time.Second); err == nil {
		connection.Close()
		t.Fatal("failed checkout startup retained its listener")
	}
}
