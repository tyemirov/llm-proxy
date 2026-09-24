package proxy

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

const hostedAssignmentRecoveryPath = "/tenants/managed-first/connections/openai"
const hostedAssignmentRecoveryBody = `{"kind":"hosted_access_grant","resource_id":"grant-journal"}`

type hostedAssignmentRecoveryFixture struct {
	database *gormManagedTenantDatabase
	server   *httptest.Server
	owner    *http.Cookie
}

func newHostedAssignmentRecoveryFixture(t *testing.T) hostedAssignmentRecoveryFixture {
	t.Helper()
	database, _, _ := newJournalTransactionFixture(t)
	server, cookie := newFundsManagementHTTPFixture(t, database)
	fixture := hostedAssignmentRecoveryFixture{database, server, cookie("owner")}
	fixture.exchange(t, http.MethodDelete, hostedAssignmentRecoveryPath+"?clear_defaults=true", "", http.StatusNoContent)
	return fixture
}

func (fixture hostedAssignmentRecoveryFixture) exchange(t *testing.T, method, path, body string, status int) map[string]any {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	origin, err := url.Parse(fixture.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{fixture.owner})
	fixture.server.Client().Jar = jar
	return accountConnectionHTTPExchange(t, fixture.server, method, path, body, status)
}

func (fixture hostedAssignmentRecoveryFixture) snapshot(t *testing.T) map[string]any {
	t.Helper()
	resources := map[string]any{}
	for _, path := range []string{"/tenants/managed-first", "/tenants/managed-first/connections", "/hosted-access-grants/grant-journal", fundsBalanceTestPath} {
		resources[path] = fixture.exchange(t, http.MethodGet, path, "", http.StatusOK)
	}
	return resources
}

func (fixture hostedAssignmentRecoveryFixture) recover(t *testing.T) {
	t.Helper()
	restarted, _ := newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, fixture.database))
	fixture.server = restarted
	fixture.exchange(t, http.MethodPut, hostedAssignmentRecoveryPath, hostedAssignmentRecoveryBody, http.StatusOK)
	accepted := fixture.snapshot(t)
	want := map[string]any{"assignments": []any{map[string]any{"provider": "openai", "kind": "hosted_access_grant", "resource_id": "grant-journal"}}}
	if !reflect.DeepEqual(accepted["/tenants/managed-first/connections"], want) {
		t.Fatalf("recovered assignment=%v", accepted)
	}
	for range 2 {
		restarted, _ = newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, fixture.database))
		fixture.server = restarted
		fixture.exchange(t, http.MethodPut, hostedAssignmentRecoveryPath, hostedAssignmentRecoveryBody, http.StatusOK)
		if !reflect.DeepEqual(accepted, fixture.snapshot(t)) {
			t.Fatal("assignment replay changed the grant, tenant profile, or funds")
		}
	}
}

func TestHostedAssignmentStorageFailuresPreserveSelection(t *testing.T) {
	for _, scenario := range []struct{ operation, table string }{
		{"update", managedTenantTable},
		{"query", "managed_hosted_grant_records"},
		{"query", "managed_hosted_tenant_assignment_records"},
		{"query", "managed_tenant_connection_records"},
		{"create", "managed_hosted_tenant_assignment_records"},
		{"create", managedProviderProfileTable},
	} {
		t.Run(scenario.operation+"/"+scenario.table, func(t *testing.T) {
			fixture := newHostedAssignmentRecoveryFixture(t)
			before := fixture.snapshot(t)
			registerManagedGORMError(t, fixture.database.database, "connection-failure", scenario.operation, scenario.table, errInternalTestDatabase)
			for range 2 {
				response := fixture.exchange(t, http.MethodPut, hostedAssignmentRecoveryPath, hostedAssignmentRecoveryBody, http.StatusInternalServerError)
				if !reflect.DeepEqual(response, map[string]any{"error": map[string]any{"code": "managed_connection_store_failed"}}) {
					t.Fatalf("assignment failure exposed private or partial data: %v", response)
				}
			}
			removeAccountConnectionFailure(t, fixture.database.database, scenario.operation)
			if !reflect.DeepEqual(before, fixture.snapshot(t)) {
				t.Fatal("failed assignment changed the grant, tenant profile, or funds")
			}
			fixture.recover(t)
		})
	}
}

func TestHostedAssignmentUnavailableReadsPreserveSelection(t *testing.T) {
	fixture := newHostedAssignmentRecoveryFixture(t)
	fixture.exchange(t, http.MethodPut, hostedAssignmentRecoveryPath, hostedAssignmentRecoveryBody, http.StatusOK)
	before := fixture.snapshot(t)
	for _, table := range []string{"managed_hosted_tenant_assignment_records", "managed_hosted_grant_records"} {
		t.Run(table, func(t *testing.T) {
			registerManagedGORMError(t, fixture.database.database, "connection-failure", "query", table, errInternalTestDatabase)
			fixture.exchange(t, http.MethodGet, "/tenants/managed-first/connections", "", http.StatusInternalServerError)
			removeAccountConnectionFailure(t, fixture.database.database, "query")
			if !reflect.DeepEqual(before, fixture.snapshot(t)) {
				t.Fatal("failed assignment read changed the selected grant")
			}
		})
	}
	fixture.recover(t)
}
