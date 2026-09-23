package proxy_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func TestHostedSchemaRejectsPartialOrObsoleteShapes(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		mutations []string
	}{
		{"missing table", []string{"DROP TABLE managed_hosted_creation_records"}},
		{"missing column", []string{"ALTER TABLE managed_hosted_creation_records DROP COLUMN request_mac"}},
		{"obsolete column", []string{"ALTER TABLE managed_hosted_creation_records ADD COLUMN legacy_owner TEXT"}},
		{"renamed column", []string{"ALTER TABLE managed_hosted_creation_records RENAME COLUMN request_mac TO legacy_mac"}},
		{"missing index", []string{"DROP INDEX idx_managed_billing_account_records_owner_user_id"}},
		{"nonunique owner", []string{"DROP INDEX idx_managed_billing_account_records_owner_user_id", "CREATE INDEX idx_managed_billing_account_records_owner_user_id ON managed_billing_account_records(owner_user_id)"}},
		{"missing assignment constraint", []string{"DROP TRIGGER guard_hosted_access_grant_insert"}},
		{"changed assignment constraint", []string{"DROP TRIGGER guard_account_connection_update", "CREATE TRIGGER guard_account_connection_update BEFORE UPDATE ON managed_tenant_connection_records BEGIN SELECT 1; END"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hosted-schema.db")
			newManagementRouterWithDatabasePath(t, proxy.Configuration{}, path)
			database := openManagedFixtureDatabase(t, path)
			for _, mutation := range scenario.mutations {
				if err := database.Exec(mutation).Error; err != nil {
					t.Fatal(err)
				}
			}
			var before []map[string]any
			if err := database.Raw("SELECT name, sql FROM sqlite_master ORDER BY name").Scan(&before).Error; err != nil {
				t.Fatal(err)
			}
			_, err := buildRouterWithCatalogs(t, managementConfigurationWithDatabasePath(proxy.Configuration{}, path), zap.NewNop().Sugar())
			if err == nil || !strings.Contains(err.Error(), "operation=validate_hosted_schema") {
				t.Fatalf("startup accepted a noncanonical hosted schema: %v", err)
			}
			var after []map[string]any
			if err := database.Raw("SELECT name, sql FROM sqlite_master ORDER BY name").Scan(&after).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("rejected startup changed the hosted schema")
			}
		})
	}
}

func TestHostedDatabaseEnforcesForeignKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enforced.db")
	configuration := managementConfigurationWithDatabasePath(proxy.Configuration{}, path)
	configuration.Management.DatabaseDialector = nil
	router, err := buildRouterWithCatalogs(t, configuration, zap.NewNop().Sugar())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	database := openManagedFixtureDatabase(t, path)
	// This trigger reads the pragma of the actual HTTP writer connection.
	if err := database.Exec(`CREATE TRIGGER require_foreign_key_enforcement BEFORE INSERT ON managed_billing_account_records
		WHEN (SELECT foreign_keys FROM pragma_foreign_keys) != 1 BEGIN SELECT RAISE(ABORT, 'foreign_keys_disabled'); END`).Error; err != nil {
		t.Fatal(err)
	}
	owner := managementSessionCookie(t, "enforcement-owner")
	requireHostedHTTP(t, server, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "enforced-account", http.StatusCreated)
}

func TestHostedSchemaRejectsBrokenResourceRelations(t *testing.T) {
	for _, scenario := range []struct{ name, mutation string }{
		{"credential version", "DELETE FROM managed_platform_credential_records"},
		{"grant audit", "DELETE FROM managed_hosted_grant_revision_records"},
		{"grant state", "UPDATE managed_hosted_grant_records SET state = 'suspended'"},
		{"grant owner", "UPDATE managed_billing_account_records SET owner_user_id = 'integrity-other'"},
		{"provider profile", "DELETE FROM managed_provider_profile_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "relations.db")
			catalog := testfixtures.ProviderCatalog(t)
			configuration := proxy.Configuration{ProviderCatalog: catalog}
			router := newManagementRouterWithDatabasePath(t, configuration, path)
			server := httptest.NewServer(router)
			defer server.Close()
			owner := managementSessionCookie(t, "integrity-owner")
			operator := managementSessionCookieWithEmail(t, "integrity-operator", testManagementAdminEmail)
			tenant := requestManagementAccount(t, router, owner).Tenants[0].ID
			requestManagementAccount(t, router, managementSessionCookie(t, "integrity-other"))
			account := hostedResourceID(t, requireHostedHTTP(t, server, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "billing", http.StatusCreated))
			platform := hostedResourceID(t, requireHostedHTTP(t, server, operator, http.MethodPost, "/platform-connections", `{"name":"Hosted","provider":"openai","fields":{"api_key":"sk-user-openai"}}`, "platform", http.StatusCreated))
			intent := fmt.Sprintf(`{"billing_account_id":%q,"tenant_id":%q,"platform_connection_id":%q,"catalog_revision":%q,"offerings":[{"model":"gpt-6-astra","operations":["text"]}],"reason":"Integrity acceptance"}`, account, tenant, platform, catalog.ModelCatalog().Revision)
			grant := hostedResourceID(t, requireHostedHTTP(t, server, operator, http.MethodPost, "/hosted-access-grants", intent, "grant", http.StatusCreated))
			requireHostedHTTP(t, server, owner, http.MethodPut, "/tenants/"+tenant+"/connections/openai", fmt.Sprintf(`{"kind":"hosted_access_grant","resource_id":%q}`, grant), "", http.StatusOK)
			server.Close()
			database := openManagedFixtureDatabase(t, path)
			if err := database.Exec(scenario.mutation).Error; err != nil {
				t.Fatal(err)
			}
			before := capabilityDatabaseSnapshot(t, database)
			_, err := buildRouterWithCatalogs(t, managementConfigurationWithDatabasePath(configuration, path), zap.NewNop().Sugar())
			if err == nil || !strings.Contains(err.Error(), "operation=validate_hosted_records") {
				t.Fatalf("startup accepted broken hosted relations: %v", err)
			}
			if !reflect.DeepEqual(before, capabilityDatabaseSnapshot(t, database)) {
				t.Fatal("rejected startup changed retained records")
			}
		})
	}
}
