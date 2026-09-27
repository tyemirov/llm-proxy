package proxy

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"gorm.io/gorm"
)

type fundsHoldRecoveryFixture struct {
	fundsAdmissionFixture
	request       managedJournalRequestRecord
	providerCalls int64
	server        *httptest.Server
	owner         *http.Cookie
}

func newFundsHoldRecoveryFixture(t *testing.T, providerCalls int64) fundsHoldRecoveryFixture {
	t.Helper()
	fixture := newFundsAdmissionFixture(t)
	table := "managed_journal_attempt_records"
	if providerCalls == 1 {
		table = "managed_journal_observation_records"
	}
	if err := fixture.database.database.Exec("CREATE TRIGGER interrupt_held_request BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT, 'controlled_held_request_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	request := failHostedTextExecution(t, fixture, providerCalls)
	if err := fixture.database.database.Exec("DROP TRIGGER interrupt_held_request").Error; err != nil {
		t.Fatal(err)
	}
	server, cookie := newFundsManagementHTTPFixture(t, fixture.database)
	return fundsHoldRecoveryFixture{fixture, request, providerCalls, server, cookie("owner")}
}

func (fixture fundsHoldRecoveryFixture) resources(t *testing.T) map[string]any {
	t.Helper()
	state := fixture.state(t)
	path := "/billing-accounts/billing-journal/requests/" + fixture.request.ID
	state["journal"] = accountConnectionHTTPExchange(t, fixture.management, http.MethodGet, path, "", http.StatusOK)
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"/attempts", "/reconciliation-cases"} {
		request, err := http.NewRequest(http.MethodGet, fixture.server.URL+managementAPIPath+path+suffix, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.AddCookie(fixture.owner)
		response, err := fixture.server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("journal read %s status=%d error=%v body=%s", suffix, response.StatusCode, err, payload)
		}
		template := "/api/management/billing-accounts/{billing_account_id}/requests/{request_id}" + suffix
		if err := contract.ValidateResponse(template, http.MethodGet, response.StatusCode, response.Header, payload); err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(payload, &value); err != nil {
			t.Fatal(err)
		}
		state["journal"+suffix] = value
	}
	return state
}

func (fixture fundsHoldRecoveryFixture) assertUnchanged(t *testing.T, before map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(before, fixture.resources(t)) {
		t.Fatal("failed hold recovery changed financial or journal resources")
	}
	assertFundsCreditRemainder(t, fixture.database, "0", "1")
	for _, model := range []any{&managedFundsSettlementRecord{}, &managedJournalObservationRecord{}, &managedJournalDeliveryRecord{}} {
		var count int64
		if err := fixture.database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("partial hold recovery %T: count=%d error=%v", model, count, err)
		}
	}
}

func (fixture fundsHoldRecoveryFixture) recover(t *testing.T, before map[string]any) {
	t.Helper()
	recoverHostedTextExecution(t, fixture.fundsAdmissionFixture, fixture.request, fixture.providerCalls)
	after := fixture.resources(t)
	previousEntries := before["ledger-entries"].(map[string]any)["entries"].([]any)
	entries := after["ledger-entries"].(map[string]any)["entries"].([]any)
	want := len(previousEntries)
	if fixture.providerCalls == 0 {
		want++
	}
	if len(entries) != want {
		t.Fatalf("hold recovery ledger entries=%d want=%d", len(entries), want)
	}
	assertFundsCreditRemainder(t, fixture.database, "0", "1")
	recoverHostedTextExecution(t, fixture.fundsAdmissionFixture, fixture.request, fixture.providerCalls)
	if !reflect.DeepEqual(after, fixture.resources(t)) {
		t.Fatal("repeated hold recovery changed financial or journal resources")
	}
}

func TestHostedFundsHoldRecoveryReadFailuresPreserveReservation(t *testing.T) {
	for _, scenario := range []struct {
		table         string
		read          int64
		providerCalls int64
	}{
		{"managed_funds_reservation_records", 1, 0},
		{"managed_funds_reservation_records", 2, 0},
		{"managed_journal_request_records", 1, 0},
		{"managed_journal_attempt_records", 1, 0},
		{"ledger_accounts", 1, 0},
		{"reservations", 1, 0},
		{"managed_funds_reservation_records", 1, 1},
		{"managed_funds_reservation_records", 2, 1},
		{"managed_journal_request_records", 1, 1},
		{"managed_journal_attempt_records", 1, 1},
	} {
		t.Run(scenario.table+"/"+strconv.FormatInt(scenario.read, 10)+"/calls-"+strconv.FormatInt(scenario.providerCalls, 10), func(t *testing.T) {
			fixture := newFundsHoldRecoveryFixture(t, scenario.providerCalls)
			before := fixture.resources(t)
			callback := fixture.database.database.Callback().Query()
			var reads, failures atomic.Int64
			if err := callback.Before("gorm:query").Register("test:hold_recovery_read", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == scenario.table && reads.Add(1) == scenario.read {
					failures.Add(1)
					tx.AddError(errors.New("controlled_hold_recovery_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			fixture.failStartup(t)
			if err := callback.Remove("test:hold_recovery_read"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 {
				t.Fatalf("hold recovery read failures=%d", failures.Load())
			}
			fixture.assertUnchanged(t, before)
			fixture.recover(t, before)
		})
	}
}

func TestHostedFundsHoldRecoveryWriteFailuresPreserveReservation(t *testing.T) {
	for _, scenario := range []struct {
		name, statement string
		providerCalls   int64
	}{
		{"undispatched-lock", "BEFORE UPDATE ON managed_journal_request_records", 0},
		{"dispatched-lock", "BEFORE UPDATE ON managed_journal_request_records", 1},
		{"ledger-release", "BEFORE UPDATE ON reservations", 0},
		{"reverse-hold", "BEFORE INSERT ON ledger_entries", 0},
		{"released-reservation", "BEFORE UPDATE ON managed_funds_reservation_records", 0},
		{"reconciliation-reservation", "BEFORE UPDATE ON managed_funds_reservation_records", 1},
		{"reconciliation-case", "BEFORE INSERT ON managed_journal_case_records", 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsHoldRecoveryFixture(t, scenario.providerCalls)
			before := fixture.resources(t)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_hold_recovery " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_hold_recovery_write_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			fixture.failStartup(t)
			fixture.assertUnchanged(t, before)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_hold_recovery").Error; err != nil {
				t.Fatal(err)
			}
			fixture.recover(t, before)
		})
	}
}
