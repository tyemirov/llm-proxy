package proxy_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestHostedAssignmentConflictsPreserveMixedProviderChoices(t *testing.T) {
	fixture := newAuthorityRecoveryFixture(t)
	owner := managementSessionCookie(t, "authority-owner")
	var intent struct {
		TenantID string `json:"tenant_id"`
	}
	if err := json.Unmarshal([]byte(fixture.grantIntent), &intent); err != nil {
		t.Fatal(err)
	}
	collection := "/tenants/" + intent.TenantID + "/connections"
	hosted := fmt.Sprintf(`{"kind":"hosted_access_grant","resource_id":%q}`, fixture.grant)
	requireHostedHTTP(t, fixture.server, owner, http.MethodPut, collection+"/openai", hosted, "", http.StatusOK)
	connection := hostedResourceID(t, requireHostedHTTP(t, fixture.server, owner, http.MethodPost, "/connections", `{"name":"Customer Gemini","provider":"gemini","version":0,"fields":{"api_key":"sk-user-gemini"}}`, "mixed-provider", http.StatusCreated))
	owned := fmt.Sprintf(`{"kind":"account_connection","resource_id":%q}`, connection)
	requireHostedHTTP(t, fixture.server, owner, http.MethodPut, collection+"/gemini", owned, "", http.StatusOK)
	before := requireHostedHTTP(t, fixture.server, owner, http.MethodGet, collection, "", "", http.StatusOK)
	var listed struct {
		Assignments []struct {
			Provider   string `json:"provider"`
			Kind       string `json:"kind"`
			ResourceID string `json:"resource_id"`
		} `json:"assignments"`
	}
	if err := json.Unmarshal(before.body, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Assignments) != 2 || listed.Assignments[0].Provider != "gemini" || listed.Assignments[0].Kind != "account_connection" || listed.Assignments[0].ResourceID != connection || listed.Assignments[1].Provider != "openai" || listed.Assignments[1].Kind != "hosted_access_grant" || listed.Assignments[1].ResourceID != fixture.grant {
		t.Fatalf("mixed provider assignments=%s", before.body)
	}
	alternative := hostedResourceID(t, requireHostedHTTP(t, fixture.server, fixture.operator, http.MethodPost, authorityGrantsPath, fixture.grantIntent, "alternative-grant", http.StatusCreated))
	for range 2 {
		requireHostedHTTP(t, fixture.server, owner, http.MethodPut, collection+"/openai", fmt.Sprintf(`{"kind":"hosted_access_grant","resource_id":%q}`, alternative), "", http.StatusConflict)
	}
	requireHostedHTTP(t, fixture.server, owner, http.MethodGet, "/tenants/%20/connections", "", "", http.StatusNotFound)
	restarted := httptest.NewServer(newManagementRouterWithDatabasePath(t, fixture.configuration, fixture.databasePath))
	t.Cleanup(restarted.Close)
	for _, server := range []*httptest.Server{fixture.server, restarted} {
		requireHostedHTTP(t, server, owner, http.MethodPut, collection+"/openai", hosted, "", http.StatusOK)
		after := requireHostedHTTP(t, server, owner, http.MethodGet, collection, "", "", http.StatusOK)
		if !reflect.DeepEqual(before.body, after.body) {
			t.Fatalf("conflict or restart changed explicit provider choices: before=%s after=%s", before.body, after.body)
		}
	}
}
