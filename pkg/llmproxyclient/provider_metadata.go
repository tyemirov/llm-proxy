package llmproxyclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

var providerOverageAmountPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

// GetProviderMetadata reads upstream model observations without adding catalog routes.
func (client Client) GetProviderMetadata(ctx context.Context, provider string) (llmproxycontract.ProviderMetadata, error) {
	var result llmproxycontract.ProviderMetadata
	body, err := client.providerResource(ctx, provider, llmproxycontract.ProviderResourceMetadata)
	if err != nil {
		return result, err
	}
	fields := exactProviderResourceObject(body, 2)
	if fields == nil || decodeExactJSON(body, &result) != nil || result.Provider != provider || result.Models == nil {
		return llmproxycontract.ProviderMetadata{}, fmt.Errorf("%w: invalid provider metadata", ErrClientHTTPFailure)
	}
	var rawModels []json.RawMessage
	_ = json.Unmarshal(fields["models"], &rawModels)
	seen := map[string]bool{}
	for index, model := range result.Models {
		if exactProviderResourceObject(rawModels[index], 7, "can_do_text_to_speech", "can_do_voice_conversion", "maximum_text_length_per_request", "max_characters_request_free_user", "max_characters_request_subscribed_user") == nil || model.ModelID == "" || model.ModelID != strings.TrimSpace(model.ModelID) || strings.TrimSpace(model.Name) == "" || seen[model.ModelID] {
			return llmproxycontract.ProviderMetadata{}, fmt.Errorf("%w: invalid provider model", ErrClientHTTPFailure)
		}
		seen[model.ModelID] = true
		for _, limit := range []*int64{model.MaximumTextLengthPerRequest, model.MaxCharactersRequestFreeUser, model.MaxCharactersRequestSubscribedUser} {
			if limit != nil && *limit < 0 {
				return llmproxycontract.ProviderMetadata{}, fmt.Errorf("%w: invalid provider model limit", ErrClientHTTPFailure)
			}
		}
	}
	return result, nil
}

// GetProviderQuotas reads subscription observations independently of published prices.
func (client Client) GetProviderQuotas(ctx context.Context, provider string) (llmproxycontract.ProviderQuotas, error) {
	var result llmproxycontract.ProviderQuotas
	body, err := client.providerResource(ctx, provider, llmproxycontract.ProviderResourceQuotas)
	if err != nil {
		return result, err
	}
	fields := exactProviderResourceObject(body, 2)
	if fields == nil || decodeExactJSON(body, &result) != nil || result.Provider != provider || strings.TrimSpace(result.Subscription.Tier) == "" || strings.TrimSpace(result.Subscription.Status) == "" || result.Subscription.CharacterCount < 0 || result.Subscription.CharacterLimit < 0 {
		return llmproxycontract.ProviderQuotas{}, fmt.Errorf("%w: invalid provider quotas", ErrClientHTTPFailure)
	}
	subscription := exactProviderResourceObject(fields["subscription"], 10, "current_overage", "currency", "next_character_count_reset_unix")
	if subscription == nil || exactProviderResourceObject(subscription["credit_extension"], 2, "value") == nil {
		return llmproxycontract.ProviderQuotas{}, fmt.Errorf("%w: invalid provider subscription shape", ErrClientHTTPFailure)
	}
	extension := result.Subscription.CreditExtension
	if extension.Unlimited && extension.Value != nil || !extension.Unlimited && (extension.Value == nil || *extension.Value < 0) {
		return llmproxycontract.ProviderQuotas{}, fmt.Errorf("%w: invalid provider credit extension", ErrClientHTTPFailure)
	}
	current := result.Subscription
	if current.Currency != nil && strings.TrimSpace(*current.Currency) == "" || current.NextCharacterCountResetUnix != nil && *current.NextCharacterCountResetUnix < 0 || current.CurrentOverage != nil && (exactProviderResourceObject(subscription["current_overage"], 2) == nil || !providerOverageAmountPattern.MatchString(current.CurrentOverage.Amount) || strings.TrimSpace(current.CurrentOverage.Currency) == "") {
		return llmproxycontract.ProviderQuotas{}, fmt.Errorf("%w: invalid provider subscription observation", ErrClientHTTPFailure)
	}
	return result, nil
}

func exactProviderResourceObject(body []byte, count int, nullable ...string) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || len(fields) != count {
		return nil
	}
	for key, value := range fields {
		if string(value) == "null" && !slices.Contains(nullable, key) {
			return nil
		}
	}
	return fields
}

func (client Client) providerResource(ctx context.Context, provider string, kind llmproxycontract.ProviderResourceKind) ([]byte, error) {
	if !diagnosticProviderPattern.MatchString(provider) {
		return nil, fmt.Errorf("%w: invalid resource provider", ErrInvalidClientRequest)
	}
	requestURL := client.config.mediaResourceURL(llmproxycontract.ProviderResourcesPath + "/" + provider + "/" + string(kind))
	request := (&http.Request{Method: http.MethodGet, URL: &requestURL, Header: http.Header{}}).WithContext(ctx)
	request.Header.Set(headerAccept, "application/json")
	request.Header.Set("Authorization", "Bearer "+client.config.secret)
	return client.doMediaResource(request, http.StatusOK)
}
