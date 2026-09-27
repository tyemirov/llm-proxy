package proxy_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
)

func TestHostedPlatformConnectionOperatorBoundaryAndRotation(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "platform.db")
	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	server := httptest.NewServer(router)
	defer server.Close()
	admin := managementSessionCookieWithEmail(t, "hosted-operator", testManagementAdminEmail)
	customer := managementSessionCookie(t, "hosted-customer")
	const collection = "/platform-connections"
	const intent = `{"name":"Platform OpenAI","provider":"openai","fields":{"api_key":"sk-user-openai"}}`
	requireHostedHTTP(t, server, nil, http.MethodPost, collection, intent, "platform-key", http.StatusUnauthorized)
	requireHostedHTTP(t, server, customer, http.MethodPost, collection, intent, "platform-key", http.StatusForbidden)
	requireHostedHTTP(t, server, customer, http.MethodGet, collection, "", "", http.StatusForbidden)
	created := requireHostedHTTP(t, server, admin, http.MethodPost, collection, intent, "platform-key", http.StatusCreated)
	var connection struct {
		ID        string `json:"id"`
		Provider  string `json:"provider"`
		Version   uint64 `json:"version"`
		Qualified bool   `json:"qualified"`
	}
	if err := json.Unmarshal(created.body, &connection); err != nil {
		t.Fatal(err)
	}
	if connection.ID == "" || connection.Version != 1 || !connection.Qualified || connection.Provider != "openai" {
		t.Fatalf("invalid platform connection: %s", created.body)
	}
	if strings.Contains(string(created.body), "sk-user-openai") {
		t.Fatalf("platform response leaks credentials: %s", created.body)
	}
	path := collection + "/" + connection.ID
	requireHostedHTTP(t, server, customer, http.MethodGet, path, "", "", http.StatusForbidden)
	replay := requireHostedHTTP(t, server, admin, http.MethodPost, collection, intent, "platform-key", http.StatusCreated)
	if string(replay.body) != string(created.body) {
		t.Fatal("platform creation replay changed")
	}
	changedIntent := strings.Replace(intent, "Platform OpenAI", "Another connection", 1)
	requireHostedHTTP(t, server, admin, http.MethodPost, collection, changedIntent, "platform-key", http.StatusConflict)
	requireHostedHTTP(t, server, admin, http.MethodPut, path, `{"name":"Rotated","provider":"openai","version":0,"fields":{"api_key":"sk-user-rotated"}}`, "", http.StatusConflict)
	requireHostedHTTP(t, server, customer, http.MethodPut, path, `{"name":"Rotated","provider":"openai","version":1,"fields":{"api_key":"sk-user-rotated"}}`, "", http.StatusForbidden)
	rotated := requireHostedHTTP(t, server, admin, http.MethodPut, path, `{"name":"Rotated","provider":"openai","version":1,"fields":{"api_key":"sk-user-rotated"}}`, "", http.StatusOK)
	if err := json.Unmarshal(rotated.body, &connection); err != nil {
		t.Fatal(err)
	}
	if connection.Version != 2 || !connection.Qualified || strings.Contains(string(rotated.body), "sk-user-rotated") {
		t.Fatalf("invalid rotated connection: %s", rotated.body)
	}
	requireHostedHTTP(t, server, admin, http.MethodPut, path, `{"name":"Stale","provider":"openai","version":1,"fields":{"api_key":"sk-user-old"}}`, "", http.StatusConflict)
	database := openManagedFixtureDatabase(t, databasePath)
	var versions []struct {
		Version uint64
		Fields  []byte
	}
	if err := database.Table("managed_platform_credential_records").Where("connection_id = ?", connection.ID).Order("version").Find(&versions).Error; err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].Version != 1 || versions[1].Version != 2 {
		t.Fatalf("rotation must retain accepted credential versions: %+v", versions)
	}
	for _, version := range versions {
		if strings.Contains(string(version.Fields), "sk-user-") || !strings.Contains(string(version.Fields), "llmpk1:") {
			t.Fatal("platform credentials are not encrypted at rest")
		}
	}
	server.Close()
	restarted := httptest.NewServer(newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath))
	defer restarted.Close()
	after := requireHostedHTTP(t, restarted, admin, http.MethodGet, path, "", "", http.StatusOK)
	if string(after.body) != string(rotated.body) {
		t.Fatal("restart changed the platform connection")
	}
	replayedAfterRotation := requireHostedHTTP(t, restarted, admin, http.MethodPost, collection, intent, "platform-key", http.StatusCreated)
	if string(replayedAfterRotation.body) != string(created.body) {
		t.Fatal("rotation changed the immutable creation receipt")
	}
}

func TestHostedPlatformConnectionDisabledVerificationPreservesCredentials(t *testing.T) {
	fixture := newAuthorityRecoveryFixture(t)
	before, counts := fixture.publicSnapshot(t), fixture.recordCounts(t)
	schema := testfixtures.ProviderCatalog(t).Schema()
	schema.ModelMigrations = nil
	for providerIndex := range schema.Providers {
		provider := &schema.Providers[providerIndex]
		if provider.ID != "openai" {
			continue
		}
		selected := provider.Verification.Model
		for index := range provider.Offerings {
			offering := &provider.Offerings[index]
			if offering.Model == selected {
				offering.Enabled = proxy.ModelDisabled
				offering.DefaultOperations = nil
			}
		}
		for index := range provider.Offerings {
			offering := &provider.Offerings[index]
			if offering.Model != selected && slices.Contains(offering.Operations, proxy.ModelOperationText) {
				offering.DefaultOperations = []string{proxy.ModelOperationText}
				break
			}
		}
	}
	catalog, err := proxy.NewProviderCatalog(schema)
	if err != nil {
		t.Fatal(err)
	}
	disabled := fixture.configuration
	disabled.ProviderCatalog = catalog
	fixture.server.Close()
	fixture.server = httptest.NewServer(newManagementRouterWithDatabasePath(t, disabled, fixture.databasePath))
	t.Cleanup(fixture.server.Close)
	for range 2 {
		for _, operation := range []struct{ method, path, body, key string }{
			{http.MethodPost, authorityConnectionsPath, authorityConnectionBody, "disabled-verification"},
			{http.MethodPut, authorityConnectionsPath + "/" + fixture.connection, authorityRotationBody, ""},
		} {
			response := requireHostedHTTP(t, fixture.server, fixture.operator, operation.method, operation.path, operation.body, operation.key, http.StatusServiceUnavailable)
			if string(response.body) != "provider_key_verification_unavailable" {
				t.Fatalf("disabled verification response=%s", response.body)
			}
		}
		if !reflect.DeepEqual(before, fixture.publicSnapshot(t)) || !reflect.DeepEqual(counts, fixture.recordCounts(t)) {
			t.Fatal("disabled verification changed qualified credentials or creation receipts")
		}
	}
	fixture.server.Close()
	fixture.server = httptest.NewServer(newManagementRouterWithDatabasePath(t, fixture.configuration, fixture.databasePath))
	t.Cleanup(fixture.server.Close)
	created := requireHostedHTTP(t, fixture.server, fixture.operator, http.MethodPost, authorityConnectionsPath, authorityConnectionBody, "disabled-verification", http.StatusCreated)
	for range 2 {
		replay := requireHostedHTTP(t, fixture.server, fixture.operator, http.MethodPost, authorityConnectionsPath, authorityConnectionBody, "disabled-verification", http.StatusCreated)
		if string(replay.body) != string(created.body) {
			t.Fatal("restored verification changed the creation receipt")
		}
	}
	requireHostedHTTP(t, fixture.server, fixture.operator, http.MethodPut, authorityConnectionsPath+"/"+fixture.connection, authorityRotationBody, "", http.StatusOK)
	recovered := fixture.recordCounts(t)
	if recovered["managed_platform_connection_records"] != counts["managed_platform_connection_records"]+1 || recovered["managed_platform_credential_records"] != counts["managed_platform_credential_records"]+2 || recovered["managed_hosted_creation_records"] != counts["managed_hosted_creation_records"]+1 {
		t.Fatalf("restored verification repeated effects: before=%v after=%v", counts, recovered)
	}
}
