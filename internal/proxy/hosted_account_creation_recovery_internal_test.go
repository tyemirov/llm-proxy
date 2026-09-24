package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"gorm.io/gorm"
)

type accountCreationFixture struct {
	service  *managementService
	database *gormManagedTenantDatabase
	server   *httptest.Server
	cookie   func(string) *http.Cookie
	contract *openapitest.Contract
}

func newAccountCreationFixture(t *testing.T) accountCreationFixture {
	t.Helper()
	database := newCanonicalGORMFixture(t, ratingTestAcceptanceTime())
	service := newInternalManagementService(t, newFakeManagedTenantDatabase(), internalManagementProviderRegistry())
	service.store.database = database
	server, cookie := fundsManagementServiceHTTPFixture(t, service)
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	return accountCreationFixture{service, database, server, cookie, contract}
}

func (fixture accountCreationFixture) exchange(t *testing.T, method, path, key string, status int) map[string]any {
	t.Helper()
	request, err := http.NewRequest(method, fixture.server.URL+managementAPIPath+path, strings.NewReader(`{"currency":"USD"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(fixture.cookie("owner"))
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set(managementIdempotencyHeader, key)
	}
	response, err := fixture.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != status || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("%s %s status=%d want=%d error=%v body=%s", method, path, response.StatusCode, status, err, body)
	}
	if err := fixture.contract.ValidateResponse(managementAPIPath+managementBillingAccountsPath, method, status, response.Header, body); err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if status == http.StatusCreated && response.Header.Get("Location") != managementAPIPath+managementBillingAccountsPath+"/"+value["id"].(string) {
		t.Fatalf("account creation Location=%q", response.Header.Get("Location"))
	}
	if status == http.StatusInternalServerError && !reflect.DeepEqual(value, map[string]any{"error": map[string]any{"code": "billing_account_store_failed"}}) {
		t.Fatalf("account creation exposed private or partial data: %s", body)
	}
	return value
}

func (fixture accountCreationFixture) inventory(t *testing.T) map[string]int64 {
	t.Helper()
	counts := map[string]int64{}
	for _, table := range []string{"managed_billing_account_records", "managed_funds_account_records", "ledger_accounts", "ledger_entries"} {
		var count int64
		if err := fixture.database.database.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		counts[table] = count
	}
	return counts
}

func (fixture accountCreationFixture) recover(t *testing.T, before map[string]int64) {
	t.Helper()
	fixture.server, fixture.cookie = newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, fixture.database))
	created := fixture.exchange(t, http.MethodPost, managementBillingAccountsPath, "recover-account", http.StatusCreated)
	for range 2 {
		replayed := fixture.exchange(t, http.MethodPost, managementBillingAccountsPath, "recover-account", http.StatusCreated)
		if !reflect.DeepEqual(created, replayed) {
			t.Fatal("account recovery changed identity")
		}
	}
	collection := fixture.exchange(t, http.MethodGet, managementBillingAccountsPath, "", http.StatusOK)
	if !reflect.DeepEqual(collection["billing_accounts"], []any{created}) {
		t.Fatalf("account recovery collection=%v", collection)
	}
	before["managed_billing_account_records"]++
	if !reflect.DeepEqual(before, fixture.inventory(t)) {
		t.Fatal("account recovery created duplicate accounts or financial effects")
	}
}

func TestHostedAccountCreationFailuresRecoverOneUnfundedAccount(t *testing.T) {
	for _, scenario := range []struct {
		name, operation, table string
		read                   int64
	}{
		{"user-read", "query", managedUserTable, 1},
		{"user-update", "update", managedUserTable, 0},
		{"account-read", "query", "managed_billing_account_records", 1},
		{"created-account-read", "query", "managed_billing_account_records", 2},
		{"account-write", "create", "managed_billing_account_records", 0},
		{"account-entropy", "entropy", "", 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newAccountCreationFixture(t)
			before := fixture.inventory(t)
			var reads, failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			switch scenario.operation {
			case "update":
				callback = fixture.database.database.Callback().Update()
			case "create":
				callback = fixture.database.database.Callback().Create()
			}
			original := fixture.service.store.randomReader
			if scenario.operation == "entropy" {
				fixture.service.store.randomReader = strings.NewReader("")
			} else if err := callback.Before("gorm:"+scenario.operation).Register("test:account_creation", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == scenario.table && (scenario.read == 0 || reads.Add(1) == scenario.read) {
					failures.Add(1)
					tx.AddError(errors.New("controlled_account_creation_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			fixture.exchange(t, http.MethodPost, managementBillingAccountsPath, "recover-account", http.StatusInternalServerError)
			if scenario.operation == "entropy" {
				fixture.service.store.randomReader = original
			} else {
				if err := callback.Remove("test:account_creation"); err != nil {
					t.Fatal(err)
				}
				if failures.Load() != 1 {
					t.Fatalf("creation failures=%d", failures.Load())
				}
			}
			collection := fixture.exchange(t, http.MethodGet, managementBillingAccountsPath, "", http.StatusOK)
			if !reflect.DeepEqual(before, fixture.inventory(t)) || !reflect.DeepEqual(collection["billing_accounts"], []any{}) {
				t.Fatal("failed account creation retained partial accounts or financial effects")
			}
			fixture.recover(t, before)
		})
	}
}

func TestHostedAccountCreationArbitratesAcrossInstances(t *testing.T) {
	for _, sameKey := range []bool{true, false} {
		name := "different-intents"
		if sameKey {
			name = "same-intent"
		}
		t.Run(name, func(t *testing.T) {
			first := newAccountCreationFixture(t)
			second := first
			second.database = openJournalTransactionInstance(t, first.database)
			second.server, second.cookie = newFundsManagementHTTPFixture(t, second.database)
			before := first.inventory(t)
			ready := make(chan struct{}, 2)
			releases := []chan struct{}{make(chan struct{}), make(chan struct{})}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			for index, database := range []*gormManagedTenantDatabase{first.database, second.database} {
				release := releases[index]
				callback := database.database.Callback().Query()
				if err := callback.After("gorm:query").Register("test:account_race", func(tx *gorm.DB) {
					if tx.Statement.Table == "managed_billing_account_records" && errors.Is(tx.Error, gorm.ErrRecordNotFound) {
						ready <- struct{}{}
						select {
						case <-release:
						case <-ctx.Done():
							tx.AddError(ctx.Err())
						}
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := callback.Remove("test:account_race"); err != nil {
						t.Error(err)
					}
				})
			}
			results := make(chan map[string]any, 2)
			go func() {
				results <- first.exchange(t, http.MethodPost, managementBillingAccountsPath, "recover-account", http.StatusCreated)
			}()
			// Both requests must first observe absence. Release the first writer
			// separately so the second transaction sees its committed account.
			select {
			case <-ready:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			key, status := "different-account", http.StatusConflict
			if sameKey {
				key, status = "recover-account", http.StatusCreated
			}
			go func() { results <- second.exchange(t, http.MethodPost, managementBillingAccountsPath, key, status) }()
			select {
			case <-ready:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			values := []map[string]any{}
			for _, release := range releases {
				close(release)
				select {
				case value := <-results:
					values = append(values, value)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			if sameKey && !reflect.DeepEqual(values[0], values[1]) {
				t.Fatal("service instances returned different accounts for the same intent")
			}
			before["managed_billing_account_records"]++
			if !reflect.DeepEqual(before, first.inventory(t)) {
				t.Fatal("competing instances created duplicate accounts or financial effects")
			}
		})
	}
}

func TestHostedAccountCreationCancelledWaitPreservesInventory(t *testing.T) {
	fixture := newAccountCreationFixture(t)
	before := fixture.inventory(t)
	handler := fixture.server.Config.Handler
	cancelled := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithCancel(request.Context())
		cancel()
		handler.ServeHTTP(writer, request.WithContext(ctx))
	}))
	t.Cleanup(cancelled.Close)
	fixture.server = cancelled
	func() {
		fixture.service.store.mutex.Lock()
		defer fixture.service.store.mutex.Unlock()
		fixture.exchange(t, http.MethodPost, managementBillingAccountsPath, "recover-account", http.StatusInternalServerError)
	}()
	if !reflect.DeepEqual(before, fixture.inventory(t)) {
		t.Fatal("cancelled account creation changed account inventory or financial effects")
	}
	fixture.recover(t, before)
}
