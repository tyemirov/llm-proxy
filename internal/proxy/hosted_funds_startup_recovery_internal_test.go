package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type fundsStartupFixture struct {
	database   *gormManagedTenantDatabase
	management *httptest.Server
	calls      *atomic.Int64
}

func newFundsStartupFixture(t *testing.T, usage string) fundsStartupFixture {
	t.Helper()
	database, _, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	calls := &atomic.Int64{}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"startup-provider-result","status":"completed","output_text":"funded result","usage":`+usage+`}`)
	}))
	t.Cleanup(upstream.Close)
	generation := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	hostedIdentityHTTP(t, generation, "startup-settlement", "funded prompt", http.StatusOK)
	return fundsStartupFixture{database, management, calls}
}

const startupCompleteUsage = `{"input_tokens":1000,"output_tokens":1000,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`

func (fixture fundsStartupFixture) state(t *testing.T) map[string]any {
	t.Helper()
	state := map[string]any{}
	for _, resource := range []string{"balance", "ledger-entries", "charges", "reservations"} {
		state[resource] = ratingHTTPExchange(t, fixture.management, http.MethodGet, "/billing-accounts/billing-journal/"+resource, "", http.StatusOK)
	}
	return state
}

func (fixture fundsStartupFixture) failStartup(t *testing.T) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	application := &proxyApplication{router: fixture.management.Config.Handler.(*gin.Engine), database: fixture.database, now: ratingTestAcceptanceTime}
	stopped := make(chan error, 1)
	go func() { stopped <- application.serve(ctx, listener) }()
	select {
	case err := <-stopped:
		if err == nil || !strings.Contains(err.Error(), "initialize funds reconciliation") {
			t.Fatalf("startup failure not reported: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("failed startup retained a live worker")
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err == nil {
		connection.Close()
		t.Fatal("failed startup retained its listener")
	}
}

func (fixture fundsStartupFixture) assertPending(t *testing.T, before map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(before, fixture.state(t)) {
		t.Fatal("failed startup changed financial resources")
	}
	assertFundsCreditRemainder(t, fixture.database, "0", "1")
	var pending int64
	if err := fixture.database.database.Model(&managedJournalDeliveryRecord{}).Where("delivered_at IS NULL").Count(&pending).Error; err != nil || pending != 1 {
		t.Fatalf("delivery lost: pending=%d error=%v", pending, err)
	}
	var settled int64
	if err := fixture.database.database.Model(&managedFundsSettlementRecord{}).Count(&settled).Error; err != nil || settled != 0 {
		t.Fatalf("partial settlement: count=%d error=%v", settled, err)
	}
}

func (fixture fundsStartupFixture) restart(t *testing.T) {
	t.Helper()
	database := openJournalTransactionInstance(t, fixture.database)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	application := &proxyApplication{router: fixture.management.Config.Handler.(*gin.Engine), database: database, now: ratingTestAcceptanceTime}
	stopped := make(chan error, 1)
	go func() { stopped <- application.serve(ctx, listener) }()
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + managementAPIPath + fundsBalanceTestPath)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("recovered service status=%d", response.StatusCode)
	}
	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("recovered service failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("recovered service did not stop")
	}
	var pending int64
	if err := fixture.database.database.Model(&managedJournalDeliveryRecord{}).Where("delivered_at IS NULL").Count(&pending).Error; err != nil || pending != 0 {
		t.Fatalf("delivery not acknowledged: pending=%d error=%v", pending, err)
	}
	if fixture.calls.Load() != 1 {
		t.Fatalf("recovery repeated provider work: calls=%d", fixture.calls.Load())
	}
}

func (fixture fundsStartupFixture) assertSettledOnce(t *testing.T, before map[string]any) {
	t.Helper()
	fixture.restart(t)
	assertHostedFundsBalance(t, fixture.database, 4, 4)
	assertFundsCreditRemainder(t, fixture.database, "3", "1000")
	after := fixture.state(t)
	previous := before["ledger-entries"].(map[string]any)["entries"].([]any)
	entries := after["ledger-entries"].(map[string]any)["entries"].([]any)
	if len(entries) != len(previous)+2 {
		t.Fatalf("settlement needs one release and one debit: before=%v after=%v", previous, entries)
	}
	fixture.restart(t)
	if !reflect.DeepEqual(after, fixture.state(t)) {
		t.Fatal("restart repeated financial effects")
	}
}

func TestHostedFundsStartupWriteFailuresPreserveDeliveryAndFinancialState(t *testing.T) {
	for _, scenario := range []struct{ name, statement string }{
		{"delivery-lock", "BEFORE UPDATE OF observation_id ON managed_journal_delivery_records"},
		{"charge", "BEFORE INSERT ON managed_charge_records"},
		{"account-lock", "BEFORE UPDATE ON managed_billing_account_records"},
		{"hold-release", "BEFORE UPDATE ON reservations"},
		{"debit", "BEFORE INSERT ON ledger_entries WHEN NEW.amount_cents = -1"},
		{"settlement", "BEFORE INSERT ON managed_funds_settlement_records"},
		{"remainder", "BEFORE UPDATE ON managed_funds_account_records"},
		{"tenant-usage", "BEFORE UPDATE ON managed_funds_tenant_records"},
		{"reservation", "BEFORE UPDATE ON managed_funds_reservation_records"},
		{"delivery-acknowledgment", "BEFORE UPDATE OF delivered_at ON managed_journal_delivery_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsStartupFixture(t, startupCompleteUsage)
			before := fixture.state(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_startup_funds " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_startup_funds_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			fixture.failStartup(t)
			fixture.assertPending(t, before)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_startup_funds").Error; err != nil {
				t.Fatal(err)
			}
			fixture.assertSettledOnce(t, before)
		})
	}
}

func TestHostedFundsStartupReadFailuresPreserveDeliveryAndFinancialState(t *testing.T) {
	for _, table := range []string{"managed_journal_observation_records", "managed_journal_delivery_records", "managed_journal_attempt_records", "managed_journal_request_records", "managed_price_snapshot_records", "managed_charge_records", "managed_charge_adjustment_records", "managed_funds_reservation_records", "managed_funds_account_records", "managed_funds_tenant_records", "ledger_accounts", "reservations"} {
		t.Run(table, func(t *testing.T) {
			fixture := newFundsStartupFixture(t, startupCompleteUsage)
			before := fixture.state(t)
			callback := fixture.database.database.Callback().Query()
			var failures atomic.Int64
			if err := callback.Before("gorm:query").Register("test:startup_funds_read", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == table {
					failures.Add(1)
					tx.AddError(errors.New("controlled_startup_funds_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			fixture.failStartup(t)
			if err := callback.Remove("test:startup_funds_read"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() == 0 {
				t.Fatal("read failure was not exercised")
			}
			fixture.assertPending(t, before)
			fixture.assertSettledOnce(t, before)
		})
	}
}

func TestHostedFundsStartupUnresolvedUsageRetainsHoldAfterWriteRecovery(t *testing.T) {
	for _, statement := range []string{"BEFORE INSERT ON managed_journal_case_records", "BEFORE UPDATE ON managed_funds_reservation_records", "BEFORE UPDATE OF delivered_at ON managed_journal_delivery_records"} {
		t.Run(statement, func(t *testing.T) {
			fixture := newFundsStartupFixture(t, `{}`)
			before := fixture.state(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_unresolved_startup " + statement + " BEGIN SELECT RAISE(ABORT, 'controlled_unresolved_startup_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			fixture.failStartup(t)
			fixture.assertPending(t, before)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_unresolved_startup").Error; err != nil {
				t.Fatal(err)
			}
			fixture.restart(t)
			assertHostedFundsBalance(t, fixture.database, 5, 2)
			assertFundsCreditRemainder(t, fixture.database, "0", "1")
			after := fixture.state(t)
			if !reflect.DeepEqual(before["ledger-entries"], after["ledger-entries"]) {
				t.Fatal("unresolved usage moved funds")
			}
			var reservation managedFundsReservationRecord
			if err := fixture.database.database.First(&reservation).Error; err != nil || reservation.State != fundsReservationReconciliation {
				t.Fatalf("unresolved hold not retained: state=%s error=%v", reservation.State, err)
			}
			fixture.restart(t)
			if !reflect.DeepEqual(after, fixture.state(t)) {
				t.Fatal("unresolved recovery repeated financial effects")
			}
		})
	}
}
