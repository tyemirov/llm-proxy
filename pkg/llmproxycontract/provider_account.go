package llmproxycontract

import (
	"fmt"
	"strings"
)

// ProviderAccount preserves the selected billing contract without converting USD to credits.
type ProviderAccount struct {
	Provider     string                `json:"provider"`
	BillingType  string                `json:"billing_type"`
	Wallet       *ProviderWallet       `json:"wallet"`
	Subscription *ProviderCreditPlan   `json:"subscription"`
	UsageBased   *ProviderMeteredUsage `json:"usage_based"`
}

// ProviderWallet records a nullable provider balance in its stated currency.
type ProviderWallet struct {
	Currency         string              `json:"currency"`
	RemainingBalance *float64            `json:"remaining_balance"`
	AutoReload       *ProviderAutoReload `json:"auto_reload"`
}

// ProviderAutoReload records the provider's wallet reload settings.
type ProviderAutoReload struct {
	Enabled      bool     `json:"enabled"`
	AmountUSD    *float64 `json:"amount_usd"`
	ThresholdUSD *float64 `json:"threshold_usd"`
}

// ProviderCreditPool preserves nullable credit balances and reset timestamps.
type ProviderCreditPool struct {
	Remaining *int64  `json:"remaining"`
	ResetsAt  *string `json:"resets_at"`
}

// ProviderCreditPlan records the provider's subscription and separate credit pools.
type ProviderCreditPlan struct {
	Plan    string               `json:"plan"`
	Credits *ProviderCreditPools `json:"credits"`
}

// ProviderCreditPools keeps premium and additional credits separate.
type ProviderCreditPools struct {
	PremiumCredits *ProviderCreditPool `json:"premium_credits"`
	AddOnCredits   *ProviderCreditPool `json:"add_on_credits"`
}

// ProviderMeteredUsage records credit and USD observations without estimating prices.
type ProviderMeteredUsage struct {
	IncludedCredits    *float64 `json:"included_credits"`
	RemainingCredits   *float64 `json:"remaining_credits"`
	SpendingCapUSD     *float64 `json:"spending_cap_usd"`
	SpendingCurrentUSD *float64 `json:"spending_current_usd"`
}

// ValidateProviderAccount validates the closed billing union at a transport boundary.
func ValidateProviderAccount(account ProviderAccount) error {
	if strings.TrimSpace(account.Provider) == "" {
		return fmt.Errorf("invalid account provider")
	}
	switch account.BillingType {
	case "wallet":
		if account.Wallet == nil || account.Subscription != nil || account.UsageBased != nil || (account.Wallet.Currency != "usd" && account.Wallet.Currency != "credits") {
			return fmt.Errorf("invalid wallet account")
		}
	case "subscription":
		if account.Subscription == nil || account.Wallet != nil || account.UsageBased != nil || strings.TrimSpace(account.Subscription.Plan) == "" || account.Subscription.Credits == nil {
			return fmt.Errorf("invalid subscription account")
		}
	case "usage_based":
		if account.UsageBased == nil || account.Subscription != nil || account.Wallet != nil {
			return fmt.Errorf("invalid metered account")
		}
	default:
		return fmt.Errorf("unknown account billing type")
	}
	return nil
}
