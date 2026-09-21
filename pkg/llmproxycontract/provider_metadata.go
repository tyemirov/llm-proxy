package llmproxycontract

const (
	ProviderResourcesPath                = "/model/v1/provider-resources"
	ErrorCodeProviderResourceNotFound    = "provider_resource_not_found"
	ErrorCodeProviderResourceUnavailable = "provider_resource_unavailable"
)

// ProviderModelMetadata is an upstream observation, not an executable catalog offering.
type ProviderModelMetadata struct {
	ModelID                            string `json:"model_id"`
	Name                               string `json:"name"`
	CanDoTextToSpeech                  *bool  `json:"can_do_text_to_speech"`
	CanDoVoiceConversion               *bool  `json:"can_do_voice_conversion"`
	MaximumTextLengthPerRequest        *int64 `json:"maximum_text_length_per_request"`
	MaxCharactersRequestFreeUser       *int64 `json:"max_characters_request_free_user"`
	MaxCharactersRequestSubscribedUser *int64 `json:"max_characters_request_subscribed_user"`
}

// ProviderMetadata contains account-visible model observations.
type ProviderMetadata struct {
	Provider string                  `json:"provider"`
	Models   []ProviderModelMetadata `json:"models"`
}

// ProviderCreditExtension is either unlimited or a nonnegative credit count.
type ProviderCreditExtension struct {
	Unlimited bool   `json:"unlimited"`
	Value     *int64 `json:"value"`
}

// ProviderOverage preserves the provider's decimal monetary observation.
type ProviderOverage struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// ProviderSubscription describes observed account quota, separately from catalog prices.
type ProviderSubscription struct {
	Tier                        string                  `json:"tier"`
	Status                      string                  `json:"status"`
	CharacterCount              int64                   `json:"character_count"`
	CharacterLimit              int64                   `json:"character_limit"`
	CreditExtension             ProviderCreditExtension `json:"credit_extension"`
	CanExtendCreditLimit        bool                    `json:"can_extend_credit_limit"`
	CurrentOverage              *ProviderOverage        `json:"current_overage"`
	HasOpenInvoices             bool                    `json:"has_open_invoices"`
	Currency                    *string                 `json:"currency"`
	NextCharacterCountResetUnix *int64                  `json:"next_character_count_reset_unix"`
}

// ProviderQuotas contains one provider's current subscription observation.
type ProviderQuotas struct {
	Provider     string               `json:"provider"`
	Subscription ProviderSubscription `json:"subscription"`
}
