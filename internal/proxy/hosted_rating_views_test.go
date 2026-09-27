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
	balance := requireHostedHTTP(t, server, owner, http.MethodGet, "/billing-accounts/"+account+"/balance", "", "", http.StatusOK)
	if err := contract.ValidateResponse("/api/management/billing-accounts/{billing_account_id}/balance", http.MethodGet, http.StatusOK, balance.header, balance.body); err != nil {
		t.Fatal(err)
	}
	for _, tail := range []string{"/balance", "/reservations", "/ledger-entries", "/charges", "/charges/charge-unknown", "/price-snapshots/price-unknown"} {
		requireHostedHTTP(t, server, other, http.MethodGet, "/billing-accounts/"+account+tail, "", "", http.StatusNotFound)
		requireHostedHTTP(t, server, nil, http.MethodGet, "/billing-accounts/"+account+tail, "", "", http.StatusUnauthorized)
	}
	requireHostedHTTP(t, server, owner, http.MethodGet, "/billing-accounts/"+account+"/balance?currency=EUR", "", "", http.StatusBadRequest)
	for _, tail := range []string{"/reservations", "/ledger-entries"} {
		page := requireHostedHTTP(t, server, owner, http.MethodGet, "/billing-accounts/"+account+tail, "", "", http.StatusOK)
		if err := contract.ValidateResponse("/api/management/billing-accounts/{billing_account_id}"+tail, http.MethodGet, http.StatusOK, page.header, page.body); err != nil {
			t.Fatal(err)
		}
		for _, query := range []string{"?limit=0", "?limit=1001", "?limit=1&limit=2", "?cursor=", "?unknown=value"} {
			requireHostedHTTP(t, server, owner, http.MethodGet, "/billing-accounts/"+account+tail+query, "", "", http.StatusBadRequest)
		}
	}
}
