package proxy

import (
	"context"
	"net"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

type paymentStartupFixture struct {
	paymentAuditFixture
	configuration Configuration
	before        map[string]any
}

func newPaymentStartupFixture(t *testing.T) paymentStartupFixture {
	t.Helper()
	fixture := newPaymentAuditFixture(t)
	service, _, _ := paymentOrdersFixture(t, fixture.database)
	var source struct{ File string }
	if err := fixture.database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	configuration := withInternalUpstreamCapacity(t, Configuration{
		Management: service.configuration, ProviderCatalog: internalCanonicalProviderCatalog(),
		AssetStorePath: t.TempDir(), Payments: fixture.payments,
	})
	configuration.Management.DatabaseDialector = fixture.database.database.Dialector
	configuration.Management.DatabasePath = source.File
	return paymentStartupFixture{fixture, configuration, paymentAdjustmentResources(t, fixture)}
}

func (fixture paymentStartupFixture) reject(t *testing.T, configuration Configuration, reason string) {
	t.Helper()
	// Occupying the port bounds the public Serve call if construction incorrectly succeeds.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	configuration.Port = listener.Addr().(*net.TCPAddr).Port
	if err := Serve(configuration, zap.NewNop().Sugar()); err == nil || !strings.Contains(err.Error(), reason) {
		t.Fatalf("payment startup error=%v want=%s", err, reason)
	}
}

func (fixture paymentStartupFixture) recover(t *testing.T) {
	t.Helper()
	for range 2 {
		func() {
			application, err := buildProxyApplication(fixture.configuration, zap.NewNop().Sugar(), newManagedTenantStore)
			if err != nil {
				t.Fatal(err)
			}
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
					t.Error("recovered payment service did not stop")
				}
			}()
			live := fixture.paymentAuditFixture
			live.server = &httptest.Server{URL: "http://" + listener.Addr().String()}
			if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, live)) || fixture.processor.creates.Load() != 1 {
				t.Fatal("payment startup recovery changed financial resources")
			}
		}()
	}
}

func TestHostedPaymentsStartupReadFailuresPreserveFundedAccounts(t *testing.T) {
	for _, scenario := range []struct{ table, reason string }{
		{"managed_payment_environment_records", "read payment environment"},
		{"managed_funding_order_records", "check retained payment environment"},
		{"managed_payment_inbox_records", "check retained payment environment"},
	} {
		t.Run(scenario.table, func(t *testing.T) {
			fixture := newPaymentStartupFixture(t)
			configuration := fixture.configuration
			var failures atomic.Int64
			configuration.Management.DatabaseDialector = paymentAuditReadFailureDialector{
				Dialector: configuration.Management.DatabaseDialector, table: scenario.table, failures: &failures,
			}
			fixture.reject(t, configuration, scenario.reason)
			if failures.Load() != 1 || !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
				t.Fatalf("startup read failure changed financial resources: failures=%d", failures.Load())
			}
			fixture.recover(t)
		})
	}
}

func TestHostedPaymentsStartupWriteFailurePreservesEnvironment(t *testing.T) {
	fixture := newPaymentStartupFixture(t)
	if err := fixture.database.database.Exec("CREATE TRIGGER reject_environment_binding BEFORE INSERT ON managed_payment_environment_records BEGIN SELECT RAISE(ABORT, 'controlled_payment_environment_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	fixture.reject(t, fixture.configuration, "bind payment environment")
	if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
		t.Fatal("failed environment binding changed financial resources")
	}
	if err := fixture.database.database.Exec("DROP TRIGGER reject_environment_binding").Error; err != nil {
		t.Fatal(err)
	}
	fixture.recover(t)
}

func TestHostedPaymentsStartupRejectsMixedEnvironments(t *testing.T) {
	for _, scenario := range []struct{ table, reason string }{
		{"managed_payment_environment_records", "payment environment differs from the database binding"},
		{"managed_funding_order_records", "payment environment differs from retained financial records"},
		{"managed_payment_inbox_records", "payment environment differs from retained financial records"},
	} {
		t.Run(scenario.table, func(t *testing.T) {
			fixture := newPaymentStartupFixture(t)
			if err := fixture.database.database.Table(scenario.table).Where("environment = ?", "sandbox").Update("environment", "production").Error; err != nil {
				t.Fatal(err)
			}
			fixture.reject(t, fixture.configuration, scenario.reason)
			var count int64
			if err := fixture.database.database.Table(scenario.table).Where("environment = ?", "production").Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("startup changed the conflicting environment: count=%d error=%v", count, err)
			}
			if err := fixture.database.database.Table(scenario.table).Where("environment = ?", "production").Update("environment", "sandbox").Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
				t.Fatal("mixed environment startup changed financial resources")
			}
			fixture.recover(t)
		})
	}
}

func TestHostedPaymentsStartupRejectsIncompleteFinancialSchema(t *testing.T) {
	for _, scenario := range []struct{ name, corrupt, restore, reason string }{
		{"missing-inbox", "ALTER TABLE managed_payment_inbox_records RENAME TO retained_inbox_records", "ALTER TABLE retained_inbox_records RENAME TO managed_payment_inbox_records", "partial payment records"},
		{"missing-inbox-column", "ALTER TABLE managed_payment_inbox_records RENAME COLUMN content_digest TO retained_digest", "ALTER TABLE managed_payment_inbox_records RENAME COLUMN retained_digest TO content_digest", "validate payment records"},
		{"missing-order-column", "ALTER TABLE managed_funding_order_records RENAME COLUMN price_id TO retained_price", "ALTER TABLE managed_funding_order_records RENAME COLUMN retained_price TO price_id", "validate funding orders"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newPaymentStartupFixture(t)
			if err := fixture.database.database.Exec(scenario.corrupt).Error; err != nil {
				t.Fatal(err)
			}
			fixture.reject(t, fixture.configuration, scenario.reason)
			if err := fixture.database.database.Exec(scenario.restore).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
				t.Fatal("incomplete schema startup changed financial resources")
			}
			fixture.recover(t)
		})
	}
}

func TestHostedPaymentsStartupRejectsInvalidAPIOrigin(t *testing.T) {
	fixture := newPaymentStartupFixture(t)
	for _, scenario := range []struct{ name, origin string }{
		{"invalid-escape", "https://%zz.example"},
		{"missing-host", "https://"},
		{"embedded-credential", "https://user:private-payment@example.invalid"},
		{"query", "https://example.invalid?token=private-payment"},
		{"fragment", "https://example.invalid/#private-payment"},
		{"relative", "/processor"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			configuration := fixture.configuration
			payments := *configuration.Payments
			payments.APIBaseURL = scenario.origin
			configuration.Payments = &payments
			fixture.reject(t, configuration, "configure payments: invalid api_base_url")
			if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) || fixture.processor.creates.Load() != 1 {
				t.Fatal("invalid API origin changed financial resources")
			}
		})
	}
	fixture.recover(t)
}
