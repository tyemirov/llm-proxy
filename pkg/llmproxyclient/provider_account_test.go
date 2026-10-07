package llmproxyclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestProviderServicesAccountBillingObservations(t *testing.T) {
	for _, scenario := range []struct {
		name, body string
		valid      bool
	}{
		{"wallet", `{"provider":"heygen","billing_type":"wallet","wallet":{"currency":"usd","remaining_balance":12.75,"auto_reload":null},"subscription":null,"usage_based":null}`, true},
		{"credits", `{"provider":"heygen","billing_type":"wallet","wallet":{"currency":"credits","remaining_balance":null,"auto_reload":{"enabled":true,"amount_usd":20,"threshold_usd":5}},"subscription":null,"usage_based":null}`, true},
		{"subscription", `{"provider":"heygen","billing_type":"subscription","wallet":null,"subscription":{"plan":"enterprise","credits":{"premium_credits":{"remaining":100,"resets_at":null},"add_on_credits":null}},"usage_based":null}`, true},
		{"metered", `{"provider":"heygen","billing_type":"usage_based","wallet":null,"subscription":null,"usage_based":{"included_credits":100,"remaining_credits":5,"spending_cap_usd":null,"spending_current_usd":25}}`, true},
		{"unknown billing", `{"provider":"heygen","billing_type":"future"}`, false},
		{"missing wallet", `{"provider":"heygen","billing_type":"wallet","wallet":null}`, false},
		{"wrong currency", `{"provider":"heygen","billing_type":"wallet","wallet":{"currency":"EUR"}}`, false},
		{"wrong provider", `{"provider":"other","billing_type":"wallet","wallet":{"currency":"usd"}}`, false},
		{"empty provider", `{"provider":"","billing_type":"wallet","wallet":{"currency":"usd"}}`, false},
		{"missing subscription", `{"provider":"heygen","billing_type":"subscription","subscription":null}`, false},
		{"empty plan", `{"provider":"heygen","billing_type":"subscription","subscription":{"plan":""}}`, false},
		{"missing metered", `{"provider":"heygen","billing_type":"usage_based","usage_based":null}`, false},
		{"competing branches", `{"provider":"heygen","billing_type":"wallet","wallet":{"currency":"usd"},"usage_based":{}}`, false},
		{"missing credit pools", `{"provider":"heygen","billing_type":"subscription","wallet":null,"subscription":{"plan":"enterprise"},"usage_based":null}`, false},
		{"null credit pools", `{"provider":"heygen","billing_type":"subscription","wallet":null,"subscription":{"plan":"enterprise","credits":null},"usage_based":null}`, false},
		{"malformed", `{`, false},
		{"unknown field", `{"provider":"heygen","billing_type":"wallet","wallet":{"currency":"usd"},"secret":"must reject"}`, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/model/v1/provider-resources/heygen/account" || r.Header.Get("Authorization") != "Bearer tenant" {
					t.Error("invalid account resource request")
				}
				_, _ = io.WriteString(w, scenario.body)
			}))
			defer server.Close()
			config, _ := NewConfig(ConfigInput{BaseURL: server.URL, Secret: "tenant"})
			client, _ := NewClient(config, server.Client())
			account, err := client.GetProviderAccount(context.Background(), "heygen")
			if (err == nil) != scenario.valid || scenario.valid && account.Provider != "heygen" {
				t.Fatalf("account=%+v error=%v", account, err)
			}
			if _, err := client.GetProviderAccount(context.Background(), "invalid/provider"); !errors.Is(err, ErrInvalidClientRequest) {
				t.Fatalf("invalid provider error=%v", err)
			}
		})
	}
}

func TestProviderServicesAccountRejectsInvalidPublicUnion(t *testing.T) {
	for _, account := range []llmproxycontract.ProviderAccount{
		{},
		{Provider: "heygen", BillingType: "future"},
		{Provider: "heygen", BillingType: "wallet"},
		{Provider: "heygen", BillingType: "wallet", Wallet: &llmproxycontract.ProviderWallet{Currency: "EUR"}},
		{Provider: "heygen", BillingType: "subscription"},
		{Provider: "heygen", BillingType: "subscription", Subscription: &llmproxycontract.ProviderCreditPlan{}},
		{Provider: "heygen", BillingType: "usage_based"},
	} {
		if err := llmproxycontract.ValidateProviderAccount(account); err == nil {
			t.Fatalf("invalid account accepted: %+v", account)
		}
	}
}
