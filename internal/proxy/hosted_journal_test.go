package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestHostedJournalAccountIsolationAndEmptyResources(t *testing.T) {
	router := newManagementRouter(t, proxy.Configuration{})
	server := httptest.NewServer(router)
	defer server.Close()
	owner := managementSessionCookie(t, "journal-owner")
	other := managementSessionCookie(t, "journal-other")
	account := hostedResourceID(t, requireHostedHTTP(t, server, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "journal-billing", http.StatusCreated))
	requestManagementAccount(t, router, other)
	path := "/billing-accounts/" + account + "/requests"
	requireHostedHTTP(t, server, nil, http.MethodGet, path, "", "", http.StatusUnauthorized)
	requireHostedHTTP(t, server, other, http.MethodGet, path, "", "", http.StatusNotFound)
	empty := requireHostedHTTP(t, server, owner, http.MethodGet, path, "", "", http.StatusOK)
	if string(empty.body) != `{"requests":[],"next_cursor":""}` {
		t.Fatalf("unexpected empty journal: %s", empty.body)
	}
	if empty.header.Get("Cache-Control") != "no-store" {
		t.Fatal("journal response permits caching")
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?limit=1&limit=2", "?cursor=unknown", "?owner=journal-other"} {
		requireHostedHTTP(t, server, owner, http.MethodGet, path+query, "", "", http.StatusBadRequest)
	}
	requireHostedHTTP(t, server, owner, http.MethodGet, path+"/request-00000000000000000000000000000000", "", "", http.StatusNotFound)
	requireHostedHTTP(t, server, other, http.MethodGet, path+"/request-00000000000000000000000000000000", "", "", http.StatusNotFound)
}

func TestHostedJournalRejectsPartialOrChangedSchema(t *testing.T) {
	for _, scenario := range []struct{ name, mutation string }{
		{"missing delivery table", "DROP TABLE managed_journal_delivery_records"},
		{"missing intent uniqueness", "DROP INDEX idx_journal_tenant_intent"},
		{"missing evidence uniqueness", "DROP INDEX idx_journal_observation_evidence"},
		{"obsolete field", "ALTER TABLE managed_journal_request_records ADD COLUMN legacy_balance REAL"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "journal-schema.db")
			newManagementRouterWithDatabasePath(t, proxy.Configuration{}, path)
			database := openManagedFixtureDatabase(t, path)
			if err := database.Exec(scenario.mutation).Error; err != nil {
				t.Fatal(err)
			}
			before := capabilityDatabaseSnapshot(t, database)
			_, err := buildRouterWithCatalogs(t, managementConfigurationWithDatabasePath(proxy.Configuration{}, path), zap.NewNop().Sugar())
			if err == nil || !strings.Contains(err.Error(), "operation=validate_usage_journal") {
				t.Fatalf("accepted a changed journal schema: %v", err)
			}
			if !reflect.DeepEqual(before, capabilityDatabaseSnapshot(t, database)) {
				t.Fatal("rejected startup changed financial records")
			}
		})
	}
}
