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
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"gorm.io/gorm"
)

type financialReadResource struct {
	name, path, template, failureCode string
}

type financialReadFixture struct {
	fundsStartupFixture
	server    *httptest.Server
	cookie    func(string) *http.Cookie
	contract  *openapitest.Contract
	resources map[string]financialReadResource
}

func newFinancialReadFixture(t *testing.T) financialReadFixture {
	t.Helper()
	startup := newFundsStartupFixture(t, startupCompleteUsage)
	startup.restart(t)
	var charge managedChargeRecord
	if err := startup.database.database.First(&charge).Error; err != nil {
		t.Fatal(err)
	}
	server, cookie := newFundsManagementHTTPFixture(t, startup.database)
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	resources := map[string]financialReadResource{}
	for _, resource := range []financialReadResource{
		{"balance", "/balance", "/balance", "billing_account_store_failed"},
		{"entries", "/ledger-entries", "/ledger-entries", "billing_account_store_failed"},
		{"reservations", "/reservations", "/reservations", "billing_account_store_failed"},
		{"reservation", "/reservations/" + charge.RequestID, "/reservations/{request_id}", "billing_account_store_failed"},
		{"charges", "/charges", "/charges", "usage_journal_unavailable"},
		{"charge", "/charges/" + charge.ID, "/charges/{charge_id}", "usage_journal_unavailable"},
		{"price", "/price-snapshots/" + charge.PriceSnapshotID, "/price-snapshots/{price_snapshot_id}", "usage_journal_unavailable"},
		{"summary", "/requests/" + charge.RequestID + "/charge-summary", "/requests/{request_id}/charge-summary", "usage_journal_unavailable"},
		{"tenant", "/tenant-limits/managed-first", "/tenant-limits/{tenant_id}", "billing_account_store_failed"},
	} {
		resource.path = "/billing-accounts/billing-journal" + resource.path
		resource.template = "/api/management/billing-accounts/{billing_account_id}" + resource.template
		resources[resource.name] = resource
	}
	return financialReadFixture{startup, server, cookie, contract, resources}
}

func (fixture financialReadFixture) read(t *testing.T, resource financialReadResource, cookie *http.Cookie, status int) map[string]any {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, fixture.server.URL+managementAPIPath+resource.path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := fixture.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != status || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("financial read %s status=%d want=%d error=%v body=%s", resource.path, response.StatusCode, status, err, body)
	}
	if err := fixture.contract.ValidateResponse(resource.template, http.MethodGet, status, response.Header, body); err != nil {
		t.Fatal(err)
	}
	if status == http.StatusUnauthorized && len(body) == 0 {
		return nil
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if status == http.StatusInternalServerError && !reflect.DeepEqual(value, map[string]any{"error": map[string]any{"code": resource.failureCode}}) {
		t.Fatalf("financial failure contains partial data or private details: %s", body)
	}
	return value
}

func (fixture financialReadFixture) snapshot(t *testing.T) map[string]any {
	t.Helper()
	result := map[string]any{}
	for name, resource := range fixture.resources {
		result[name] = fixture.read(t, resource, fixture.cookie("owner"), http.StatusOK)
	}
	return result
}

func (fixture financialReadFixture) assertUnchanged(t *testing.T, before map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(before, fixture.snapshot(t)) {
		t.Fatal("financial read changed a retained resource")
	}
	assertFundsCreditRemainder(t, fixture.database, "3", "1000")
	if fixture.calls.Load() != 1 {
		t.Fatalf("financial read dispatched provider work: calls=%d", fixture.calls.Load())
	}
}

func TestHostedFinancialReadsRejectStorageFailuresWithoutPartialData(t *testing.T) {
	fixture := newFinancialReadFixture(t)
	before := fixture.snapshot(t)
	for _, scenario := range []struct {
		resource, table, operation string
		read                       int64
	}{
		{"balance", "managed_billing_account_records", "query", 1},
		{"balance", "managed_funds_account_records", "query", 1},
		{"balance", "ledger_accounts", "query", 1},
		{"balance", "ledger_accounts", "query", 2},
		{"balance", "managed_payment_adjustment_records", "query", 1},
		{"balance", "ledger_entries", "row", 1},
		{"balance", "ledger_entries", "row", 2},
		{"balance", "reservations", "row", 1},
		{"balance", "reservations", "row", 2},
		{"balance", "managed_funds_settlement_records", "row", 1},
		{"balance", "managed_funds_reservation_records", "row", 1},
		{"reservations", "managed_funds_reservation_records", "query", 1},
		{"entries", "ledger_entries", "query", 1},
		{"reservation", "managed_funds_reservation_records", "query", 1},
		{"reservation", "managed_price_snapshot_records", "query", 1},
		{"reservation", "managed_journal_request_records", "query", 1},
		{"reservation", "managed_journal_attempt_records", "query", 1},
		{"reservation", "managed_charge_records", "query", 1},
		{"reservation", "managed_charge_adjustment_records", "query", 1},
		{"charges", "managed_charge_records", "query", 1},
		{"charges", "managed_charge_adjustment_records", "query", 1},
		{"charge", "managed_charge_records", "query", 1},
		{"charge", "managed_charge_adjustment_records", "query", 1},
		{"price", "managed_price_snapshot_records", "query", 1},
		{"summary", "managed_journal_request_records", "query", 1},
		{"summary", "managed_journal_attempt_records", "query", 1},
		{"summary", "managed_charge_records", "query", 1},
		{"summary", "managed_charge_adjustment_records", "query", 1},
		{"tenant", "managed_tenant_records", "query", 1},
		{"tenant", "managed_funds_tenant_records", "query", 1},
		{"tenant", "managed_funds_reservation_records", "row", 1},
	} {
		t.Run(scenario.resource+"/"+scenario.table+"/"+strconv.FormatInt(scenario.read, 10), func(t *testing.T) {
			callback := fixture.database.database.Callback().Query()
			if scenario.operation == "row" {
				callback = fixture.database.database.Callback().Row()
			}
			var reads, failures atomic.Int64
			if err := callback.Before("gorm:"+scenario.operation).Register("test:financial_read", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == scenario.table && reads.Add(1) == scenario.read {
					failures.Add(1)
					tx.AddError(errors.New("controlled_financial_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := callback.Remove("test:financial_read"); err != nil {
					t.Error(err)
				}
			})
			fixture.read(t, fixture.resources[scenario.resource], fixture.cookie("owner"), http.StatusInternalServerError)
			if failures.Load() != 1 {
				t.Fatalf("financial read failures=%d", failures.Load())
			}
		})
		fixture.assertUnchanged(t, before)
	}
}

func TestHostedFinancialReadsRejectInvalidQueriesWithoutWrites(t *testing.T) {
	fixture := newFinancialReadFixture(t)
	before := fixture.snapshot(t)
	for _, scenario := range []struct{ resource, query string }{
		{"balance", "?unexpected=1"},
		{"reservation", "?unexpected=1"},
		{"tenant", "?unexpected=1"},
		{"reservations", "?limit=0"},
		{"reservations", "?cursor=invalid"},
		{"reservations", "?unexpected=1"},
		{"entries", "?limit=0"},
		{"entries", "?cursor=%20"},
		{"entries", "?unexpected=1"},
		{"charges", "?limit=0"},
		{"charges", "?cursor=invalid"},
		{"charges", "?unexpected=1"},
	} {
		t.Run(scenario.resource+scenario.query, func(t *testing.T) {
			resource := fixture.resources[scenario.resource]
			resource.path += scenario.query
			fixture.read(t, resource, fixture.cookie("owner"), http.StatusBadRequest)
			fixture.assertUnchanged(t, before)
		})
	}
}

func TestHostedFinancialReadsRejectMissingOrForeignResources(t *testing.T) {
	fixture := newFinancialReadFixture(t)
	before := fixture.snapshot(t)
	for _, name := range []string{"reservation", "charge", "price"} {
		t.Run("missing/"+name, func(t *testing.T) {
			resource := fixture.resources[name]
			index := strings.LastIndex(resource.path, "-")
			resource.path = resource.path[:index+1] + strings.Repeat("0", 32)
			fixture.read(t, resource, fixture.cookie("owner"), http.StatusNotFound)
			fixture.assertUnchanged(t, before)
		})
	}
	for name, resource := range fixture.resources {
		t.Run("foreign/"+name, func(t *testing.T) {
			fixture.read(t, resource, fixture.cookie("other"), http.StatusNotFound)
			fixture.assertUnchanged(t, before)
		})
		t.Run("anonymous/"+name, func(t *testing.T) {
			fixture.read(t, resource, nil, http.StatusUnauthorized)
			fixture.assertUnchanged(t, before)
		})
	}
}

func TestHostedFinancialReadsRejectCorruptChargesWithoutChangingFunds(t *testing.T) {
	fixture := newFinancialReadFixture(t)
	before := fixture.snapshot(t)
	var original managedChargeRecord
	if err := fixture.database.database.First(&original).Error; err != nil {
		t.Fatal(err)
	}
	write := func(t *testing.T, encoded []byte, digest string) {
		t.Helper()
		result := fixture.database.database.Model(&managedChargeRecord{}).Where("id = ?", original.ID).Updates(map[string]any{"rating": encoded, "rating_digest": digest})
		if result.Error != nil || result.RowsAffected != 1 {
			t.Fatalf("retained charge update rows=%d error=%v", result.RowsAffected, result.Error)
		}
	}
	for _, scenario := range []struct {
		name   string
		mutate func(*CatalogRatedUsage)
	}{
		{"digest", nil},
		{"invalid-json", nil},
		{"unknown-field", nil},
		{"trailing-value", nil},
		{"state", func(rating *CatalogRatedUsage) { rating.State = "invalid" }},
		{"provider-cost", func(rating *CatalogRatedUsage) { rating.ProviderCost.Denominator = "0" }},
		{"customer-charge", func(rating *CatalogRatedUsage) { rating.CustomerCharge.Numerator = "-1" }},
		{"minimum-adjustment", func(rating *CatalogRatedUsage) { rating.MinimumAdjustment.Denominator = "0" }},
		{"unresolved-state", func(rating *CatalogRatedUsage) {
			rating.State = CatalogRatingUnresolved
			rating.UnresolvedDimensions = []string{"output_tokens"}
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var rating CatalogRatedUsage
			if err := json.Unmarshal(original.Rating, &rating); err != nil {
				t.Fatal(err)
			}
			if scenario.mutate != nil {
				scenario.mutate(&rating)
			}
			encoded, err := json.Marshal(rating)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario.name {
			case "invalid-json":
				encoded = []byte("{")
			case "unknown-field":
				encoded = append(encoded[:len(encoded)-1], []byte(`,"unexpected":true}`)...)
			case "trailing-value":
				encoded = append(encoded, []byte(" {}")...)
			}
			digest := sha256Hex(string(encoded))
			if scenario.name == "digest" {
				digest = strings.Repeat("0", 64)
			}
			write(t, encoded, digest)
			t.Cleanup(func() { write(t, original.Rating, original.RatingDigest) })
			for _, name := range []string{"charges", "charge", "summary", "reservation"} {
				fixture.read(t, fixture.resources[name], fixture.cookie("owner"), http.StatusInternalServerError)
			}
			for _, name := range []string{"balance", "entries", "tenant"} {
				if !reflect.DeepEqual(before[name], fixture.read(t, fixture.resources[name], fixture.cookie("owner"), http.StatusOK)) {
					t.Fatalf("corrupt charge changed %s", name)
				}
			}
		})
		fixture.assertUnchanged(t, before)
	}
}

func TestHostedFinancialReadsRejectCorruptReservationPrice(t *testing.T) {
	fixture := newFinancialReadFixture(t)
	before := fixture.snapshot(t)
	var original managedPriceSnapshotRecord
	if err := fixture.database.database.First(&original).Error; err != nil {
		t.Fatal(err)
	}
	write := func(document []byte, digest string) {
		t.Helper()
		if err := fixture.database.database.Model(&managedPriceSnapshotRecord{}).Where("id = ?", original.ID).Updates(map[string]any{"document": document, "digest": digest}).Error; err != nil {
			t.Fatal(err)
		}
	}
	write([]byte("{"), sha256Hex("{"))
	for _, name := range []string{"reservation", "price"} {
		fixture.read(t, fixture.resources[name], fixture.cookie("owner"), http.StatusInternalServerError)
	}
	write(original.Document, original.Digest)
	fixture.assertUnchanged(t, before)
}

func TestHostedFinancialReadsPreserveNewBillingAccount(t *testing.T) {
	fixture := newFinancialReadFixture(t)
	before := fixture.snapshot(t)
	owner := fixture.cookie("read-account-owner")
	request, err := http.NewRequest(http.MethodPost, fixture.server.URL+managementAPIPath+managementBillingAccountsPath, strings.NewReader(`{"currency":"USD"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(owner)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(managementIdempotencyHeader, "financial-read-account")
	response, err := fixture.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusCreated {
		t.Fatalf("new financial account status=%d error=%v body=%s", response.StatusCode, err, encoded)
	}
	if err := fixture.contract.ValidateResponse("/api/management/billing-accounts", http.MethodPost, http.StatusCreated, response.Header, encoded); err != nil {
		t.Fatal(err)
	}
	var account map[string]any
	if err := json.Unmarshal(encoded, &account); err != nil {
		t.Fatal(err)
	}
	path := "/billing-accounts/" + account["id"].(string)
	collection := financialReadResource{"accounts", "/billing-accounts", "/api/management/billing-accounts", "billing_account_store_failed"}
	detail := financialReadResource{"account", path, "/api/management/billing-accounts/{billing_account_id}", "billing_account_store_failed"}
	for _, resource := range []financialReadResource{collection, detail} {
		t.Run(resource.name, func(t *testing.T) {
			original := fixture.read(t, resource, owner, http.StatusOK)
			if resource.name == "accounts" {
				if !reflect.DeepEqual(original["billing_accounts"], []any{account}) {
					t.Fatalf("account collection=%v", original)
				}
			} else if !reflect.DeepEqual(original, account) {
				t.Fatalf("account details=%v", original)
			}
			callback := fixture.database.database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:account_read", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == "managed_billing_account_records" {
					tx.AddError(errors.New("controlled_account_read_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := callback.Remove("test:account_read"); err != nil {
					t.Error(err)
				}
				if !reflect.DeepEqual(original, fixture.read(t, resource, owner, http.StatusOK)) {
					t.Error("account read recovery changed identity")
				}
			})
			fixture.read(t, resource, owner, http.StatusInternalServerError)
		})
	}
	foreign := detail
	foreign.path = "/billing-accounts/billing-journal"
	fixture.read(t, foreign, owner, http.StatusNotFound)
	balance := fixture.resources["balance"]
	balance.path = path + "/balance"
	value := fixture.read(t, balance, owner, http.StatusOK)
	if value["posted_cents"] != "0" || value["available_cents"] != "0" || value["reserved_cents"] != "0" {
		t.Fatalf("new account received funds from another owner: %v", value)
	}
	for _, scenario := range []struct{ table, column, value string }{
		{"ledger_accounts", "user_id", "read-account-owner"},
		{"managed_funds_account_records", "billing_account_id", account["id"].(string)},
	} {
		var count int64
		if err := fixture.database.database.Table(scenario.table).Where(scenario.column+" = ?", scenario.value).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("financial read created records in %s: count=%d error=%v", scenario.table, count, err)
		}
	}
	fixture.assertUnchanged(t, before)
}
