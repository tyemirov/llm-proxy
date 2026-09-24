package proxy_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	authorityConnectionsPath = "/platform-connections"
	authorityGrantsPath      = "/hosted-access-grants"
	authorityConnectionBody  = `{"name":"Authority provider","provider":"openai","fields":{"api_key":"sk-user-openai"}}`
	authorityRotationBody    = `{"name":"Rotated authority","provider":"openai","version":1,"fields":{"api_key":"sk-user-rotated"}}`
	authoritySuspensionBody  = `{"state":"suspended","revision":1,"reason":"Operator review"}`
)

type authorityRecoveryFixture struct {
	server        *httptest.Server
	database      *gorm.DB
	databasePath  string
	configuration proxy.Configuration
	operator      *http.Cookie
	connection    string
	grant         string
	grantIntent   string
}

func newAuthorityRecoveryFixture(t *testing.T) authorityRecoveryFixture {
	t.Helper()
	catalog := testfixtures.ProviderCatalog(t)
	configuration := proxy.Configuration{ProviderCatalog: catalog}
	databasePath := filepath.Join(t.TempDir(), "authority.db")
	router := newManagementRouterWithDatabasePath(t, configuration, databasePath)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	operator := managementSessionCookieWithEmail(t, "authority-operator", testManagementAdminEmail)
	owner := managementSessionCookie(t, "authority-owner")
	tenant := requestManagementAccount(t, router, owner).Tenants[0].ID
	account := hostedResourceID(t, requireHostedHTTP(t, server, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "billing", http.StatusCreated))
	connection := hostedResourceID(t, requireHostedHTTP(t, server, operator, http.MethodPost, authorityConnectionsPath, authorityConnectionBody, "platform", http.StatusCreated))
	intent := fmt.Sprintf(`{"billing_account_id":%q,"tenant_id":%q,"platform_connection_id":%q,"catalog_revision":%q,"offerings":[{"model":"gpt-6-astra","operations":["text"]}],"reason":"Customer enrollment"}`, account, tenant, connection, catalog.ModelCatalog().Revision)
	grant := hostedResourceID(t, requireHostedHTTP(t, server, operator, http.MethodPost, authorityGrantsPath, intent, "grant", http.StatusCreated))
	return authorityRecoveryFixture{server, openManagedFixtureDatabase(t, databasePath), databasePath, configuration, operator, connection, grant, intent}
}

func (fixture authorityRecoveryFixture) publicSnapshot(t *testing.T) map[string]string {
	t.Helper()
	result := make(map[string]string)
	for _, path := range []string{authorityConnectionsPath, authorityConnectionsPath + "/" + fixture.connection, authorityGrantsPath, authorityGrantsPath + "/" + fixture.grant, authorityGrantsPath + "/" + fixture.grant + "/revisions"} {
		result[path] = string(requireHostedHTTP(t, fixture.server, fixture.operator, http.MethodGet, path, "", "", http.StatusOK).body)
	}
	return result
}

func (fixture authorityRecoveryFixture) recordCounts(t *testing.T) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64)
	for _, table := range []string{"managed_hosted_creation_records", "managed_platform_connection_records", "managed_platform_credential_records", "managed_hosted_grant_records", "managed_hosted_grant_revision_records"} {
		var count int64
		if err := fixture.database.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		counts[table] = count
	}
	return counts
}

func TestHostedAuthorityWritesRollbackAndRecover(t *testing.T) {
	for _, scenario := range []struct{ name, operation, statement string }{
		{"connection-receipt", "create-connection", "BEFORE INSERT ON managed_hosted_creation_records"},
		{"connection-resource", "create-connection", "BEFORE INSERT ON managed_platform_connection_records"},
		{"connection-credential", "create-connection", "BEFORE INSERT ON managed_platform_credential_records"},
		{"grant-receipt", "create-grant", "BEFORE INSERT ON managed_hosted_creation_records"},
		{"grant-resource", "create-grant", "BEFORE INSERT ON managed_hosted_grant_records"},
		{"grant-audit", "create-grant", "BEFORE INSERT ON managed_hosted_grant_revision_records"},
		{"rotation-resource", "rotate", "BEFORE UPDATE ON managed_platform_connection_records"},
		{"rotation-credential", "rotate", "BEFORE INSERT ON managed_platform_credential_records"},
		{"transition-resource", "suspend", "BEFORE UPDATE ON managed_hosted_grant_records"},
		{"transition-audit", "suspend", "BEFORE INSERT ON managed_hosted_grant_revision_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newAuthorityRecoveryFixture(t)
			method, path, body, key, status := http.MethodPost, authorityConnectionsPath, authorityConnectionBody, "recover-creation", http.StatusCreated
			switch scenario.operation {
			case "create-grant":
				path, body = authorityGrantsPath, fixture.grantIntent
			case "rotate":
				method, path, body, key, status = http.MethodPut, authorityConnectionsPath+"/"+fixture.connection, authorityRotationBody, "", http.StatusOK
			case "suspend":
				method, path, body, key, status = http.MethodPatch, authorityGrantsPath+"/"+fixture.grant, authoritySuspensionBody, "", http.StatusOK
			}
			before, counts := fixture.publicSnapshot(t), fixture.recordCounts(t)
			if err := fixture.database.Exec("CREATE TRIGGER reject_authority_write " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_private_authority_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			response := requireHostedHTTP(t, fixture.server, fixture.operator, method, path, body, key, http.StatusInternalServerError)
			if strings.TrimSpace(string(response.body)) != `{"error":{"code":"hosted_access_store_failed"}}` {
				t.Fatalf("write failure exposed partial or private data: %s", response.body)
			}
			if !reflect.DeepEqual(before, fixture.publicSnapshot(t)) || !reflect.DeepEqual(counts, fixture.recordCounts(t)) {
				t.Fatal("failed authority write committed partial resources")
			}
			if err := fixture.database.Exec("DROP TRIGGER reject_authority_write").Error; err != nil {
				t.Fatal(err)
			}
			fixture.server.Close()
			fixture.server = httptest.NewServer(newManagementRouterWithDatabasePath(t, fixture.configuration, fixture.databasePath))
			t.Cleanup(fixture.server.Close)
			accepted := requireHostedHTTP(t, fixture.server, fixture.operator, method, path, body, key, status)
			if strings.Contains(string(accepted.body), "sk-user-") {
				t.Fatal("authority recovery exposed credentials")
			}
			after, recoveredCounts := fixture.publicSnapshot(t), fixture.recordCounts(t)
			if reflect.DeepEqual(before, after) {
				t.Fatal("authority recovery did not apply the requested change")
			}
			if method == http.MethodPost {
				replay := requireHostedHTTP(t, fixture.server, fixture.operator, method, path, body, key, status)
				if string(accepted.body) != string(replay.body) {
					t.Fatal("creation recovery changed the retained response")
				}
			} else {
				requireHostedHTTP(t, fixture.server, fixture.operator, method, path, body, key, http.StatusConflict)
			}
			if !reflect.DeepEqual(after, fixture.publicSnapshot(t)) || !reflect.DeepEqual(recoveredCounts, fixture.recordCounts(t)) {
				t.Fatal("authority replay created another effect")
			}
		})
	}
}

func TestHostedAuthorityUnavailableTablesNeverReturnPartialResources(t *testing.T) {
	fixture := newAuthorityRecoveryFixture(t)
	for _, scenario := range []struct{ name, method, path, body, key, table string }{
		{"connection-list", http.MethodGet, authorityConnectionsPath, "", "", "managed_platform_connection_records"},
		{"connection-read", http.MethodGet, authorityConnectionsPath + "/" + fixture.connection, "", "", "managed_platform_connection_records"},
		{"connection-replay", http.MethodPost, authorityConnectionsPath, authorityConnectionBody, "platform", "managed_hosted_creation_records"},
		{"connection-rotation", http.MethodPut, authorityConnectionsPath + "/" + fixture.connection, authorityRotationBody, "", "managed_platform_connection_records"},
		{"grant-list", http.MethodGet, authorityGrantsPath, "", "", "managed_hosted_grant_records"},
		{"grant-read", http.MethodGet, authorityGrantsPath + "/" + fixture.grant, "", "", "managed_hosted_grant_records"},
		{"grant-replay", http.MethodPost, authorityGrantsPath, fixture.grantIntent, "grant", "managed_hosted_creation_records"},
		{"grant-connection", http.MethodPost, authorityGrantsPath, fixture.grantIntent, "new-grant", "managed_platform_connection_records"},
		{"grant-owner", http.MethodPost, authorityGrantsPath, fixture.grantIntent, "new-grant", "managed_billing_account_records"},
		{"grant-transition", http.MethodPatch, authorityGrantsPath + "/" + fixture.grant, authoritySuspensionBody, "", "managed_hosted_grant_records"},
		{"grant-revisions", http.MethodGet, authorityGrantsPath + "/" + fixture.grant + "/revisions", "", "", "managed_hosted_grant_revision_records"},
		{"grant-revision-owner", http.MethodGet, authorityGrantsPath + "/" + fixture.grant + "/revisions", "", "", "managed_hosted_grant_records"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before, counts := fixture.publicSnapshot(t), fixture.recordCounts(t)
			if err := fixture.database.Exec("ALTER TABLE " + scenario.table + " RENAME TO unavailable_authority_table").Error; err != nil {
				t.Fatal(err)
			}
			response := requireHostedHTTP(t, fixture.server, fixture.operator, scenario.method, scenario.path, scenario.body, scenario.key, http.StatusInternalServerError)
			if err := fixture.database.Exec("ALTER TABLE unavailable_authority_table RENAME TO " + scenario.table).Error; err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(response.body)) != `{"error":{"code":"hosted_access_store_failed"}}` {
				t.Fatalf("unavailable authority read exposed private or partial data: %s", response.body)
			}
			if !reflect.DeepEqual(before, fixture.publicSnapshot(t)) || !reflect.DeepEqual(counts, fixture.recordCounts(t)) {
				t.Fatal("unavailable authority resource changed accepted state")
			}
		})
	}
}

func TestHostedAuthorityRejectsInvalidRequestsWithoutEffects(t *testing.T) {
	fixture := newAuthorityRecoveryFixture(t)
	for _, scenario := range []struct{ name, method, path, body, key string }{
		{"connection-json", http.MethodPost, authorityConnectionsPath, `{`, "invalid"},
		{"connection-name", http.MethodPost, authorityConnectionsPath, strings.Replace(authorityConnectionBody, "Authority provider", " ", 1), "invalid"},
		{"connection-provider", http.MethodPost, authorityConnectionsPath, strings.Replace(authorityConnectionBody, `"openai"`, `"unknown"`, 1), "invalid"},
		{"connection-fields", http.MethodPost, authorityConnectionsPath, strings.Replace(authorityConnectionBody, `"api_key"`, `"unknown_field"`, 1), "invalid"},
		{"connection-version", http.MethodPost, authorityConnectionsPath, authorityRotationBody, "invalid"},
		{"connection-key", http.MethodPost, authorityConnectionsPath, authorityConnectionBody, ""},
		{"rotation-json", http.MethodPut, authorityConnectionsPath + "/" + fixture.connection, `{`, ""},
		{"grant-key", http.MethodPost, authorityGrantsPath, fixture.grantIntent, ""},
		{"grant-offerings", http.MethodPost, authorityGrantsPath, strings.Replace(fixture.grantIntent, `[{"model":"gpt-6-astra","operations":["text"]}]`, `[]`, 1), "invalid"},
		{"grant-offering-schema", http.MethodPost, authorityGrantsPath, strings.Replace(fixture.grantIntent, `"model":`, `"unknown":`, 1), "invalid"},
		{"grant-transition-json", http.MethodPatch, authorityGrantsPath + "/" + fixture.grant, `{`, ""},
		{"grant-transition-state", http.MethodPatch, authorityGrantsPath + "/" + fixture.grant, strings.Replace(authoritySuspensionBody, "suspended", "unknown", 1), ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before, counts := fixture.publicSnapshot(t), fixture.recordCounts(t)
			requireHostedHTTP(t, fixture.server, fixture.operator, scenario.method, scenario.path, scenario.body, scenario.key, http.StatusBadRequest)
			if !reflect.DeepEqual(before, fixture.publicSnapshot(t)) || !reflect.DeepEqual(counts, fixture.recordCounts(t)) {
				t.Fatal("invalid authority request changed accepted state")
			}
		})
	}
	for _, path := range []string{authorityConnectionsPath, authorityGrantsPath, authorityGrantsPath + "/" + fixture.grant + "/revisions"} {
		for _, query := range []string{"limit=%zz", "limit=0", "limit=1&limit=2", "cursor=invalid", "unknown=value"} {
			requireHostedHTTP(t, fixture.server, fixture.operator, http.MethodGet, path+"?"+query, "", "", http.StatusBadRequest)
		}
	}
	for _, request := range []struct{ method, path, body string }{
		{http.MethodGet, authorityConnectionsPath + "/missing", ""},
		{http.MethodPut, authorityConnectionsPath + "/missing", authorityRotationBody},
		{http.MethodPatch, authorityGrantsPath + "/missing", authoritySuspensionBody},
		{http.MethodGet, authorityGrantsPath + "/missing/revisions", ""},
	} {
		requireHostedHTTP(t, fixture.server, fixture.operator, request.method, request.path, request.body, "", http.StatusNotFound)
	}
}

func TestHostedAuthorityConnectionPaginationPreservesEveryResource(t *testing.T) {
	fixture := newAuthorityRecoveryFixture(t)
	for index := range 2 {
		requireHostedHTTP(t, fixture.server, fixture.operator, http.MethodPost, authorityConnectionsPath, authorityConnectionBody, fmt.Sprintf("additional-%d", index), http.StatusCreated)
	}
	seen := make(map[string]bool)
	cursor := ""
	for {
		path := authorityConnectionsPath + "?limit=1"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		response := requireHostedHTTP(t, fixture.server, fixture.operator, http.MethodGet, path, "", "", http.StatusOK)
		var page struct {
			Connections []struct{ ID string } `json:"platform_connections"`
			NextCursor  string                `json:"next_cursor"`
		}
		if err := json.Unmarshal(response.body, &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Connections) != 1 || seen[page.Connections[0].ID] {
			t.Fatalf("invalid connection page: %s", response.body)
		}
		seen[page.Connections[0].ID] = true
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 3 || !seen[fixture.connection] {
		t.Fatalf("pagination lost connections: %v", seen)
	}
}

func TestHostedAuthorityProviderRejectionPreservesQualifiedCredentials(t *testing.T) {
	var reject atomic.Bool
	var calls atomic.Int64
	reject.Store(true)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if reject.Load() {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusUnauthorized)
			if _, err := writer.Write([]byte(`{"private":"rejected credential detail"}`)); err != nil {
				t.Error(err)
			}
			return
		}
		writeProviderKeyVerificationSuccess(writer, verificationTransportOpenAI)
	}))
	defer upstream.Close()
	databasePath := filepath.Join(t.TempDir(), "qualification.db")
	router := newOperationalProviderKeyVerificationRouter(t, providerKeyVerificationConfiguration(upstream.URL), zap.NewNop().Sugar(), databasePath, TestTimeout)
	server := httptest.NewServer(router)
	defer server.Close()
	operator := managementSessionCookieWithEmail(t, "qualification-operator", testManagementAdminEmail)
	before := requireHostedHTTP(t, server, operator, http.MethodGet, authorityConnectionsPath, "", "", http.StatusOK)
	rejected := requireHostedHTTP(t, server, operator, http.MethodPost, authorityConnectionsPath, authorityConnectionBody, "qualification", http.StatusUnprocessableEntity)
	if strings.Contains(string(rejected.body), "private") || strings.Contains(string(rejected.body), "sk-user-") || calls.Load() != 1 {
		t.Fatalf("invalid qualification failure: calls=%d response=%s", calls.Load(), rejected.body)
	}
	after := requireHostedHTTP(t, server, operator, http.MethodGet, authorityConnectionsPath, "", "", http.StatusOK)
	if string(before.body) != string(after.body) {
		t.Fatal("rejected qualification created a platform connection")
	}
	reject.Store(false)
	created := requireHostedHTTP(t, server, operator, http.MethodPost, authorityConnectionsPath, authorityConnectionBody, "qualification", http.StatusCreated)
	connection := hostedResourceID(t, created)
	path := authorityConnectionsPath + "/" + connection
	reject.Store(true)
	rejected = requireHostedHTTP(t, server, operator, http.MethodPut, path, authorityRotationBody, "", http.StatusUnprocessableEntity)
	if strings.Contains(string(rejected.body), "private") || strings.Contains(string(rejected.body), "sk-user-") || calls.Load() != 3 {
		t.Fatalf("invalid rotation failure: calls=%d response=%s", calls.Load(), rejected.body)
	}
	unchanged := requireHostedHTTP(t, server, operator, http.MethodGet, path, "", "", http.StatusOK)
	if string(unchanged.body) != string(created.body) {
		t.Fatal("rejected rotation replaced qualified credentials")
	}
	database := openManagedFixtureDatabase(t, databasePath)
	var count int64
	if err := database.Table("managed_platform_credential_records").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("rejected rotation retained credentials: count=%d error=%v", count, err)
	}
	reject.Store(false)
	requireHostedHTTP(t, server, operator, http.MethodPut, path, authorityRotationBody, "", http.StatusOK)
	if err := database.Table("managed_platform_credential_records").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("verified rotation did not retain one new version: count=%d error=%v", count, err)
	}
	requireHostedHTTP(t, server, operator, http.MethodPut, path, authorityRotationBody, "", http.StatusConflict)
	if calls.Load() != 4 {
		t.Fatalf("stale rotation called the provider: calls=%d", calls.Load())
	}
}

func TestHostedAuthorityConcurrentQualificationCommitsOneIntent(t *testing.T) {
	for _, scenario := range []string{"creation-replay", "creation-conflict", "rotation-conflict"} {
		t.Run(scenario, func(t *testing.T) {
			var synchronize atomic.Bool
			var qualified atomic.Int64
			release := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if synchronize.Load() {
					if qualified.Add(1) == 2 {
						close(release)
					}
					select {
					case <-release:
					case <-request.Context().Done():
						return
					}
				}
				writeProviderKeyVerificationSuccess(writer, verificationTransportOpenAI)
			}))
			defer upstream.Close()
			configuration := providerKeyVerificationConfiguration(upstream.URL)
			configuration.UpstreamCapacity = testfixtures.UpstreamCapacity(2, 2)
			databasePath := filepath.Join(t.TempDir(), "concurrent-authority.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
			first := httptest.NewServer(newOperationalProviderKeyVerificationRouter(t, configuration, zap.NewNop().Sugar(), databasePath, TestTimeout))
			defer first.Close()
			second := httptest.NewServer(newOperationalProviderKeyVerificationRouter(t, configuration, zap.NewNop().Sugar(), databasePath, TestTimeout))
			defer second.Close()
			operator := managementSessionCookieWithEmail(t, "concurrent-authority-operator", testManagementAdminEmail)
			method, path, key := http.MethodPost, authorityConnectionsPath, "concurrent-intent"
			bodies := []string{authorityConnectionBody, authorityConnectionBody}
			if scenario == "creation-conflict" {
				bodies[1] = strings.Replace(authorityConnectionBody, "Authority provider", "Other provider", 1)
			}
			if scenario == "rotation-conflict" {
				connection := hostedResourceID(t, requireHostedHTTP(t, first, operator, http.MethodPost, path, authorityConnectionBody, "original", http.StatusCreated))
				method, path, key = http.MethodPut, authorityConnectionsPath+"/"+connection, ""
				bodies = []string{authorityRotationBody, strings.Replace(authorityRotationBody, "Rotated authority", "Other rotation", 1)}
			}
			synchronize.Store(true)
			responses, failures := make([]hostedHTTPResponse, 2), make([]error, 2)
			var group sync.WaitGroup
			for index, server := range []*httptest.Server{first, second} {
				group.Go(func() {
					responses[index], failures[index] = hostedHTTPExchange(server.Client(), server.URL, operator, method, path, bodies[index], key)
				})
			}
			group.Wait()
			if qualified.Load() != 2 {
				t.Fatalf("requests did not reach concurrent qualification: %d", qualified.Load())
			}
			successes, conflicts := 0, 0
			for index, response := range responses {
				if failures[index] != nil {
					t.Fatal(failures[index])
				}
				switch response.status {
				case http.StatusCreated, http.StatusOK:
					successes++
				case http.StatusConflict:
					conflicts++
				default:
					t.Fatalf("concurrent authority status=%d body=%s", response.status, response.body)
				}
			}
			wantSuccess, wantConflict, wantVersions := 1, 1, int64(1)
			if scenario == "creation-replay" {
				wantSuccess, wantConflict = 2, 0
				if string(responses[0].body) != string(responses[1].body) {
					t.Fatal("concurrent replay returned different connections")
				}
			}
			if scenario == "rotation-conflict" {
				wantVersions = 2
			}
			if successes != wantSuccess || conflicts != wantConflict {
				t.Fatalf("concurrent outcomes: successes=%d conflicts=%d", successes, conflicts)
			}
			database := openManagedFixtureDatabase(t, databasePath)
			for table, want := range map[string]int64{"managed_hosted_creation_records": 1, "managed_platform_connection_records": 1, "managed_platform_credential_records": wantVersions} {
				var count int64
				if err := database.Table(table).Count(&count).Error; err != nil || count != want {
					t.Fatalf("concurrent authority records %s: count=%d want=%d error=%v", table, count, want, err)
				}
			}
			listed := requireHostedHTTP(t, first, operator, http.MethodGet, authorityConnectionsPath, "", "", http.StatusOK)
			if strings.Contains(string(listed.body), "sk-user-") {
				t.Fatal("concurrent qualification exposed credential values")
			}
		})
	}
}

func TestHostedAuthorityUnreadableGrantDoesNotExposePartialLists(t *testing.T) {
	fixture := newAuthorityRecoveryFixture(t)
	before := fixture.publicSnapshot(t)
	var original struct{ Offerings []byte }
	if err := fixture.database.Table("managed_hosted_grant_records").Where("id = ?", fixture.grant).First(&original).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.Table("managed_hosted_grant_records").Where("id = ?", fixture.grant).Update("offerings", []byte(`{"private":"unreadable stored grant"}`)).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{authorityGrantsPath, authorityGrantsPath + "/" + fixture.grant} {
		response := requireHostedHTTP(t, fixture.server, fixture.operator, http.MethodGet, path, "", "", http.StatusInternalServerError)
		if strings.TrimSpace(string(response.body)) != `{"error":{"code":"hosted_access_store_failed"}}` {
			t.Fatalf("unreadable grant exposed partial or private data: %s", response.body)
		}
	}
	if err := fixture.database.Table("managed_hosted_grant_records").Where("id = ?", fixture.grant).Update("offerings", original.Offerings).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, fixture.publicSnapshot(t)) {
		t.Fatal("grant read failure changed accepted authority")
	}
}

func TestHostedAuthorityUnreadableGrantPreservesTransition(t *testing.T) {
	fixture := newAuthorityRecoveryFixture(t)
	before, counts := fixture.publicSnapshot(t), fixture.recordCounts(t)
	var original struct{ Offerings []byte }
	if err := fixture.database.Table("managed_hosted_grant_records").Where("id = ?", fixture.grant).First(&original).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.Table("managed_hosted_grant_records").Where("id = ?", fixture.grant).Update("offerings", []byte(`{"private":"unreadable stored grant"}`)).Error; err != nil {
		t.Fatal(err)
	}
	response := requireHostedHTTP(t, fixture.server, fixture.operator, http.MethodPatch, authorityGrantsPath+"/"+fixture.grant, authoritySuspensionBody, "", http.StatusInternalServerError)
	if strings.TrimSpace(string(response.body)) != `{"error":{"code":"hosted_access_store_failed"}}` {
		t.Fatalf("unreadable grant exposed partial or private data: %s", response.body)
	}
	if err := fixture.database.Table("managed_hosted_grant_records").Where("id = ?", fixture.grant).Update("offerings", original.Offerings).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, fixture.publicSnapshot(t)) || !reflect.DeepEqual(counts, fixture.recordCounts(t)) {
		t.Fatal("failed transition on unreadable scope changed accepted authority or audit history")
	}
	fixture.server.Close()
	fixture.server = httptest.NewServer(newManagementRouterWithDatabasePath(t, fixture.configuration, fixture.databasePath))
	t.Cleanup(fixture.server.Close)
	requireHostedHTTP(t, fixture.server, fixture.operator, http.MethodPatch, authorityGrantsPath+"/"+fixture.grant, authoritySuspensionBody, "", http.StatusOK)
	after, recoveredCounts := fixture.publicSnapshot(t), fixture.recordCounts(t)
	for range 2 {
		fixture.server.Close()
		fixture.server = httptest.NewServer(newManagementRouterWithDatabasePath(t, fixture.configuration, fixture.databasePath))
		t.Cleanup(fixture.server.Close)
		requireHostedHTTP(t, fixture.server, fixture.operator, http.MethodPatch, authorityGrantsPath+"/"+fixture.grant, authoritySuspensionBody, "", http.StatusConflict)
		if !reflect.DeepEqual(after, fixture.publicSnapshot(t)) || !reflect.DeepEqual(recoveredCounts, fixture.recordCounts(t)) {
			t.Fatal("recovered grant transition repeated authority or audit effects")
		}
	}
}
