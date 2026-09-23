package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func TestHostedRatingManagementRoutesRequireAccountOwnership(t *testing.T) {
	router := newManagementRouterWithDatabasePath(t, proxy.Configuration{}, filepath.Join(t.TempDir(), "rating-access.db"))
	server := httptest.NewServer(router)
	defer server.Close()
	owner, other := managementSessionCookie(t, "rating-owner"), managementSessionCookie(t, "rating-other")
	requestManagementAccount(t, router, owner)
	requestManagementAccount(t, router, other)
	account := hostedResourceID(t, requireHostedHTTP(t, server, owner, http.MethodPost, "/billing-accounts", `{"currency":"USD"}`, "billing", http.StatusCreated))
	path := "/billing-accounts/" + account + "/charges"
	response := requireHostedHTTP(t, server, owner, http.MethodGet, path, "", "", http.StatusOK)
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateResponse("/api/management/billing-accounts/{billing_account_id}/charges", http.MethodGet, http.StatusOK, response.header, response.body); err != nil {
		t.Fatal(err)
	}
	for _, tail := range []string{"/charges", "/charges/charge-unknown", "/price-snapshots/price-unknown"} {
		requireHostedHTTP(t, server, other, http.MethodGet, "/billing-accounts/"+account+tail, "", "", http.StatusNotFound)
		requireHostedHTTP(t, server, nil, http.MethodGet, "/billing-accounts/"+account+tail, "", "", http.StatusUnauthorized)
	}
}
