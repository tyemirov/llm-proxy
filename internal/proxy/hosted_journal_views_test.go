package proxy_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func TestHostedJournalEvidenceCollectionsAreOwnedBoundedAndPrivate(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "journal-views.db")
	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	server := httptest.NewServer(router)
	defer server.Close()
	owner, other := managementSessionCookie(t, "journal-view-owner"), managementSessionCookie(t, "journal-view-other")
	operator := managementSessionCookieWithEmail(t, "journal-view-operator", testManagementAdminEmail)
	tenant := requestManagementAccount(t, router, owner).Tenants[0].ID
	requestManagementAccount(t, router, other)
	account := hostedResourceID(t, requireHostedHTTP(t, server, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "billing", http.StatusCreated))
	foreignAccount := hostedResourceID(t, requireHostedHTTP(t, server, other, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "other-billing", http.StatusCreated))
	connection := hostedResourceID(t, requireHostedHTTP(t, server, operator, http.MethodPost, "/platform-connections", `{"name":"Journal provider","provider":"openai","fields":{"api_key":"sk-user-openai"}}`, "platform", http.StatusCreated))
	catalogResponse, err := server.Client().Get(server.URL + "/api/public/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Revision string `json:"revision"`
	}
	if err := json.NewDecoder(catalogResponse.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	catalogResponse.Body.Close()
	grant := hostedResourceID(t, requireHostedHTTP(t, server, operator, http.MethodPost, "/hosted-access-grants", fmt.Sprintf(`{"billing_account_id":%q,"tenant_id":%q,"platform_connection_id":%q,"catalog_revision":%q,"offerings":[{"model":"gpt-4.1","operations":["text"]}],"reason":"Journal view acceptance"}`, account, tenant, connection, catalog.Revision), "grant", http.StatusCreated))
	database := openManagedFixtureDatabase(t, databasePath)
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	requestID := "request-" + strings.Repeat("1", 32)
	if err := database.Exec(`INSERT INTO managed_journal_request_records (id,billing_account_id,tenant_id,key_digest,intent_digest,grant_id,grant_revision,platform_connection_id,credential_version,provider,model,operation,catalog_revision,execution_kind,execution_id,state,usage_state,owner_token,claim_expires_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, requestID, account, tenant, "private-key-digest", "private-intent-digest", grant, 1, connection, 1, "openai", "gpt-4.1", "text", catalog.Revision, "text_request", "retained-execution", "completed", "unknown", "private-worker", now, now, now).Error; err != nil {
		t.Fatal(err)
	}
	for number := 1; number <= 2; number++ {
		attemptID := fmt.Sprintf("attempt-%032d", number)
		observationID := fmt.Sprintf("observation-%032d", number)
		caseID := fmt.Sprintf("case-%032d", number)
		if err := database.Exec(`INSERT INTO managed_journal_attempt_records (id,request_id,number,state,provider_request_id,dispatch_at,observed_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, attemptID, requestID, number, "observed", "private-native-request", now, now, now, now).Error; err != nil {
			t.Fatal(err)
		}
		quantities := `[{"dimension":"input_tokens","unit":"token","value":"9007199254740993"},{"dimension":"output_tokens","unit":"token","value":"0"},{"dimension":"cache_read_tokens","unit":"token","unknown_reason":"not_reported","included_in":"input_tokens"}]`
		if err := database.Exec(`INSERT INTO managed_journal_observation_records (id,attempt_id,evidence_digest,adapter_revision,quantities,source_fields,completeness,outcome,observed_at,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, observationID, attemptID, "private-evidence-digest", "private-adapter-revision", []byte(quantities), []byte(`[{"path":"private_source_field","value":"3"}]`), "unknown", "continue", now, now).Error; err != nil {
			t.Fatal(err)
		}
		reason := "usage_unknown"
		var resolved any
		if number == 2 {
			reason = "execution_outcome_unknown"
			resolved = now
		}
		if err := database.Exec(`INSERT INTO managed_journal_case_records (id,request_id,reason,resolved_at,resolution,created_at) VALUES (?,?,?,?,?,?)`, caseID, requestID, reason, resolved, "private-resolution-notes", now).Error; err != nil {
			t.Fatal(err)
		}
	}
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	base := "/billing-accounts/" + account + "/requests/" + requestID
	snapshot := func() map[string][]map[string]any {
		result := map[string][]map[string]any{}
		for _, table := range []string{"managed_journal_request_records", "managed_journal_attempt_records", "managed_journal_observation_records", "managed_journal_case_records", "managed_journal_delivery_records"} {
			var rows []map[string]any
			if err := database.Table(table).Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			result[table] = rows
		}
		return result
	}
	before := snapshot()
	for _, collection := range []struct{ path, key, prefix string }{
		{"attempts", "attempts", "attempt-"}, {"observations", "observations", "observation-"}, {"reconciliation-cases", "cases", "case-"},
	} {
		t.Run(collection.path, func(t *testing.T) {
			first := requireHostedHTTP(t, server, owner, http.MethodGet, base+"/"+collection.path+"?limit=1", "", "", http.StatusOK)
			if first.header.Get("Cache-Control") != "no-store" {
				t.Fatal("journal evidence permits caching")
			}
			template := "/api/management/billing-accounts/{billing_account_id}/requests/{request_id}/" + collection.path
			if err := contract.ValidateResponse(template, http.MethodGet, http.StatusOK, first.header, first.body); err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{"private-", "private_source_field", connection, "provider_request_id", "source_fields", "resolution"} {
				if strings.Contains(string(first.body), private) {
					t.Fatalf("journal exposes private evidence: %s", first.body)
				}
			}
			var page map[string]any
			if err := json.Unmarshal(first.body, &page); err != nil {
				t.Fatal(err)
			}
			rows := page[collection.key].([]any)
			if len(rows) != 1 || page["next_cursor"] != collection.prefix+strings.Repeat("0", 31)+"1" {
				t.Fatalf("first page=%v", page)
			}
			next := requireHostedHTTP(t, server, owner, http.MethodGet, base+"/"+collection.path+"?limit=1&cursor="+page["next_cursor"].(string), "", "", http.StatusOK)
			if err := contract.ValidateResponse(template, http.MethodGet, http.StatusOK, next.header, next.body); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(next.body, &page); err != nil {
				t.Fatal(err)
			}
			if len(page[collection.key].([]any)) != 1 || page["next_cursor"] != "" {
				t.Fatalf("last page=%v", page)
			}
			if collection.key == "observations" && (!strings.Contains(string(first.body), `"value":"9007199254740993"`) || !strings.Contains(string(first.body), `"value":"0"`) || !strings.Contains(string(first.body), `"unknown_reason":"not_reported"`)) {
				t.Fatalf("measurement semantics changed: %s", first.body)
			}
			if collection.key == "cases" && (!strings.Contains(string(first.body), `"state":"open"`) || !strings.Contains(string(next.body), `"state":"resolved"`) || strings.Contains(string(next.body), "private-resolution")) {
				t.Fatalf("case states changed: %s %s", first.body, next.body)
			}
			for _, query := range []string{"?limit=0", "?limit=101", "?limit=1&limit=2", "?cursor=wrong", "?owner=other"} {
				requireHostedHTTP(t, server, owner, http.MethodGet, base+"/"+collection.path+query, "", "", http.StatusBadRequest)
			}
			requireHostedHTTP(t, server, nil, http.MethodGet, base+"/"+collection.path, "", "", http.StatusUnauthorized)
			requireHostedHTTP(t, server, other, http.MethodGet, base+"/"+collection.path, "", "", http.StatusNotFound)
			requireHostedHTTP(t, server, other, http.MethodGet, strings.Replace(base, account, foreignAccount, 1)+"/"+collection.path, "", "", http.StatusNotFound)
			requireHostedHTTP(t, server, owner, http.MethodGet, strings.Replace(base, requestID, "request-"+strings.Repeat("f", 32), 1)+"/"+collection.path, "", "", http.StatusNotFound)
		})
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("journal GET changed retained evidence")
	}
	if err := database.Exec("DELETE FROM managed_journal_case_records WHERE id = ?", "case-"+strings.Repeat("0", 31)+"2").Error; err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"usage_unknown", "dispatch_outcome_unknown", "execution_outcome_unknown", "execution_result_unknown"} {
		if err := database.Exec("UPDATE managed_journal_case_records SET reason = ?", reason).Error; err != nil {
			t.Fatal(err)
		}
		page := requireHostedHTTP(t, server, owner, http.MethodGet, base+"/reconciliation-cases", "", "", http.StatusOK)
		if err := contract.ValidateResponse("/api/management/billing-accounts/{billing_account_id}/requests/{request_id}/reconciliation-cases", http.MethodGet, http.StatusOK, page.header, page.body); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(page.body), `"reason":"`+reason+`"`) {
			t.Fatalf("missing case reason: %s", page.body)
		}
	}
	for _, quantities := range []string{`[]`, `not-json`, `[{"dimension":"input_tokens","unit":"token","value":"NaN"}]`} {
		if err := database.Exec("UPDATE managed_journal_observation_records SET quantities = ?, completeness = 'complete'", []byte(quantities)).Error; err != nil {
			t.Fatal(err)
		}
		failure := requireHostedHTTP(t, server, owner, http.MethodGet, base+"/observations", "", "", http.StatusInternalServerError)
		if strings.Contains(string(failure.body), quantities) {
			t.Fatal("invalid stored evidence reached the caller")
		}
	}
	if err := database.Exec("UPDATE managed_journal_case_records SET reason = ? WHERE id = ?", "private-unrecognized-reason", "case-"+strings.Repeat("0", 31)+"1").Error; err != nil {
		t.Fatal(err)
	}
	requireHostedHTTP(t, server, owner, http.MethodGet, base+"/reconciliation-cases", "", "", http.StatusInternalServerError)
}
