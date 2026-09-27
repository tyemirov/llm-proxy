package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

type hostedHTTPResponse struct {
	status int
	header http.Header
	body   []byte
}

func hostedHTTPExchange(client *http.Client, origin string, cookie *http.Cookie, method, path, body, key string) (hostedHTTPResponse, error) {
	request, err := http.NewRequest(method, origin+"/api/management"+path, strings.NewReader(body))
	if err != nil {
		return hostedHTTPResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := client.Do(request)
	if err != nil {
		return hostedHTTPResponse{}, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	return hostedHTTPResponse{status: response.StatusCode, header: response.Header, body: payload}, err
}

func requireHostedHTTP(t *testing.T, server *httptest.Server, cookie *http.Cookie, method, path, body, key string, expected int) hostedHTTPResponse {
	t.Helper()
	response, err := hostedHTTPExchange(server.Client(), server.URL, cookie, method, path, body, key)
	if err != nil {
		t.Fatal(err)
	}
	if response.status != expected {
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, response.status, expected, response.body)
	}
	if response.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("%s %s permits cached financial resources", method, path)
	}
	return response
}

func TestHostedBillingAccountExplicitCreationIsolationAndRestart(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "hosted.db")
	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath)
	server := httptest.NewServer(router)
	defer server.Close()
	owner := managementSessionCookie(t, "hosted-owner")
	other := managementSessionCookie(t, "hosted-other")
	requestManagementAccount(t, router, owner)
	requestManagementAccount(t, router, other)
	const path = "/billing-accounts"
	const intent = `{"currency":"USD"}`
	requireHostedHTTP(t, server, nil, http.MethodPost, path, intent, "create-account", http.StatusUnauthorized)
	before := requireHostedHTTP(t, server, owner, http.MethodGet, path, "", "", http.StatusOK)
	if strings.TrimSpace(string(before.body)) != `{"billing_accounts":[]}` {
		t.Fatalf("GET must not create a billing account: %s", before.body)
	}
	for _, body := range []string{`{}`, `{"currency":"EUR"}`, `{"currency":"USD","owner_user_id":"hosted-other"}`} {
		requireHostedHTTP(t, server, owner, http.MethodPost, path, body, "invalid-account", http.StatusBadRequest)
	}
	requireHostedHTTP(t, server, owner, http.MethodPost, path, intent, "", http.StatusBadRequest)
	created := requireHostedHTTP(t, server, owner, http.MethodPost, path, intent, "create-account", http.StatusCreated)
	var account struct {
		ID        string `json:"id"`
		Currency  string `json:"currency"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(created.body, &account); err != nil {
		t.Fatal(err)
	}
	if account.ID == "" || account.ID == "hosted-owner" || account.Currency != "USD" || account.CreatedAt == "" {
		t.Fatalf("invalid billing account: %s", created.body)
	}
	if created.header.Get("Location") != "/api/management"+path+"/"+account.ID {
		t.Fatalf("creation location=%q", created.header.Get("Location"))
	}
	if strings.Contains(string(created.body), "key_digest") || strings.Contains(string(created.body), "owner_user_id") {
		t.Fatalf("private account fields leaked: %s", created.body)
	}
	replayed := requireHostedHTTP(t, server, owner, http.MethodPost, path, intent, "create-account", http.StatusCreated)
	if string(replayed.body) != string(created.body) {
		t.Fatalf("creation replay changed: before=%s after=%s", created.body, replayed.body)
	}
	requireHostedHTTP(t, server, owner, http.MethodPost, path, intent, "second-account", http.StatusConflict)
	requireHostedHTTP(t, server, other, http.MethodGet, path+"/"+account.ID, "", "", http.StatusNotFound)
	otherAccount := requireHostedHTTP(t, server, other, http.MethodPost, path, intent, "create-account", http.StatusCreated)
	if string(otherAccount.body) == string(created.body) {
		t.Fatal("idempotency crossed account ownership")
	}
	server.Close()
	restarted := httptest.NewServer(newManagementRouterWithDatabasePath(t, proxy.Configuration{}, databasePath))
	defer restarted.Close()
	retained := requireHostedHTTP(t, restarted, owner, http.MethodGet, path+"/"+account.ID, "", "", http.StatusOK)
	if string(retained.body) != string(created.body) {
		t.Fatalf("restart changed account: before=%s after=%s", created.body, retained.body)
	}
	requireHostedHTTP(t, restarted, owner, http.MethodPost, path, intent, "create-account", http.StatusCreated)
}

func TestHostedBillingAccountConcurrentCreation(t *testing.T) {
	server := httptest.NewServer(newManagementRouter(t, proxy.Configuration{}))
	defer server.Close()
	owner := managementSessionCookie(t, "concurrent-hosted-owner")
	const requests = 8
	responses := make(chan hostedHTTPResponse, requests)
	errors := make(chan error, requests)
	var group sync.WaitGroup
	for index := 0; index < requests; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			response, err := hostedHTTPExchange(server.Client(), server.URL, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "one-account")
			responses <- response
			errors <- err
		}()
	}
	group.Wait()
	close(responses)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first string
	for response := range responses {
		if response.status != http.StatusCreated {
			t.Fatalf("concurrent creation status=%d body=%s", response.status, response.body)
		}
		if first == "" {
			first = string(response.body)
		}
		if string(response.body) != first {
			t.Fatalf("concurrent creation returned different accounts: %s / %s", first, response.body)
		}
	}
}

func TestHostedResourceOpenAPI(t *testing.T) {
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newManagementRouter(t, proxy.Configuration{}))
	defer server.Close()
	owner := managementSessionCookie(t, "hosted-schema-owner")
	admin := managementSessionCookieWithEmail(t, "hosted-schema-operator", testManagementAdminEmail)
	for _, scenario := range []struct {
		cookie *http.Cookie
		method string
		path   string
		body   string
		status int
	}{
		{nil, http.MethodGet, "/billing-accounts", "", http.StatusUnauthorized},
		{owner, http.MethodGet, "/billing-accounts", "", http.StatusOK},
		{owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, http.StatusCreated},
		{owner, http.MethodPost, "/billing-accounts", `{}`, http.StatusBadRequest},
		{owner, http.MethodGet, "/platform-connections", "", http.StatusForbidden},
		{admin, http.MethodGet, "/platform-connections", "", http.StatusOK},
		{admin, http.MethodPost, "/platform-connections", `{"name":"Schema","provider":"openai","fields":{"api_key":"sk-user-openai"}}`, http.StatusCreated},
		{admin, http.MethodGet, "/platform-connections", "", http.StatusOK},
	} {
		response := requireHostedHTTP(t, server, scenario.cookie, scenario.method, scenario.path, scenario.body, "schema-creation", scenario.status)
		if err := contract.ValidateResponse("/api/management"+scenario.path, scenario.method, response.status, response.header, response.body); err != nil {
			t.Fatalf("%s %s response contract: %v", scenario.method, scenario.path, err)
		}
	}
}
