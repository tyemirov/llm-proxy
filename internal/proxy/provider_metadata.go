package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const providerMetadataMaximumBytes = 1 << 20
const providerMetadataTimeout = 30 * time.Second

var providerOverageAmount = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

func (service *mediaOperationService) providerMetadataHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		identifier := c.Param("provider")
		kind := llmproxycontract.ProviderResourceKind(c.Param("kind"))
		definition, found := service.providers.definitions[providerID(identifier)]
		transportID := ""
		if found && (kind == llmproxycontract.ProviderResourceMetadata || kind == llmproxycontract.ProviderResourceQuotas) {
			for _, resource := range definition.resources {
				if resource.Kind == kind {
					transportID = resource.Transport
				}
			}
		}
		if transportID == "" || c.Request.URL.RawQuery != "" {
			writeProviderResourceError(c, http.StatusNotFound, llmproxycontract.ErrorCodeProviderResourceNotFound)
			return
		}
		requestTenant := authenticatedTenantFromContext(c)
		settings, configured := requestTenant.providerSettings[definition.identifier]
		if !configured || !settings.hasRequiredConnectionFields(definition) {
			writeProviderResourceError(c, http.StatusNotFound, llmproxycontract.ErrorCodeProviderResourceNotFound)
			return
		}
		provider := service.providers.forTenant(requestTenant).definitions[definition.identifier]
		provider, _ = provider.resolvedTransport(transportID)
		ctx, cancel := context.WithTimeout(c.Request.Context(), providerMetadataTimeout)
		defer cancel()
		client := newProviderTransportHTTPDoer(service.httpClient, provider, provider.credentialFor(endpointKindText))
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, provider.textEndpointURL, nil)
		request.Header.Set("Accept", "application/json")
		response, err := client.Do(request)
		if err != nil {
			writeProviderResourceError(c, http.StatusBadGateway, llmproxycontract.ErrorCodeProviderResourceUnavailable)
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, providerMetadataMaximumBytes+1))
		if err != nil || response.StatusCode != http.StatusOK || len(body) > providerMetadataMaximumBytes {
			writeProviderResourceError(c, http.StatusBadGateway, llmproxycontract.ErrorCodeProviderResourceUnavailable)
			return
		}
		if kind == llmproxycontract.ProviderResourceMetadata {
			models, valid := decodeProviderModelMetadata(body)
			if !valid {
				writeProviderResourceError(c, http.StatusBadGateway, llmproxycontract.ErrorCodeProviderResourceUnavailable)
				return
			}
			c.JSON(http.StatusOK, llmproxycontract.ProviderMetadata{Provider: identifier, Models: models})
			return
		}
		subscription, valid := decodeProviderSubscription(body)
		if !valid {
			writeProviderResourceError(c, http.StatusBadGateway, llmproxycontract.ErrorCodeProviderResourceUnavailable)
			return
		}
		c.JSON(http.StatusOK, llmproxycontract.ProviderQuotas{Provider: identifier, Subscription: subscription})
	}
}

func writeProviderResourceError(c *gin.Context, status int, code string) {
	c.JSON(status, mediaOperationErrorEnvelope{Error: mediaOperationErrorResponse{Code: code}})
}

func decodeProviderModelMetadata(body []byte) ([]llmproxycontract.ProviderModelMetadata, bool) {
	var models []llmproxycontract.ProviderModelMetadata
	if json.Unmarshal(body, &models) != nil || models == nil {
		return nil, false
	}
	seen := map[string]bool{}
	for _, model := range models {
		if model.ModelID == "" || model.ModelID != strings.TrimSpace(model.ModelID) || strings.TrimSpace(model.Name) == "" || seen[model.ModelID] {
			return nil, false
		}
		seen[model.ModelID] = true
		for _, limit := range []*int64{model.MaximumTextLengthPerRequest, model.MaxCharactersRequestFreeUser, model.MaxCharactersRequestSubscribedUser} {
			if limit != nil && *limit < 0 {
				return nil, false
			}
		}
	}
	return models, true
}

func decodeProviderSubscription(body []byte) (llmproxycontract.ProviderSubscription, bool) {
	var native struct {
		Tier                 string                            `json:"tier"`
		Status               string                            `json:"status"`
		CharacterCount       *int64                            `json:"character_count"`
		CharacterLimit       *int64                            `json:"character_limit"`
		CreditExtension      json.RawMessage                   `json:"max_credit_limit_extension"`
		CanExtendCreditLimit *bool                             `json:"can_extend_character_limit"`
		CurrentOverage       *llmproxycontract.ProviderOverage `json:"current_overage"`
		HasOpenInvoices      *bool                             `json:"has_open_invoices"`
		Currency             *string                           `json:"currency"`
		NextReset            *int64                            `json:"next_character_count_reset_unix"`
	}
	if json.Unmarshal(body, &native) != nil || strings.TrimSpace(native.Tier) == "" || strings.TrimSpace(native.Status) == "" || native.CharacterCount == nil || *native.CharacterCount < 0 || native.CharacterLimit == nil || *native.CharacterLimit < 0 || native.CanExtendCreditLimit == nil || native.HasOpenInvoices == nil || len(native.CreditExtension) == 0 {
		return llmproxycontract.ProviderSubscription{}, false
	}
	extension := llmproxycontract.ProviderCreditExtension{}
	if bytes.Equal(native.CreditExtension, []byte(`"unlimited"`)) {
		extension.Unlimited = true
	} else {
		var value *int64
		if json.Unmarshal(native.CreditExtension, &value) != nil || value == nil || *value < 0 {
			return llmproxycontract.ProviderSubscription{}, false
		}
		extension.Value = value
	}
	if native.Currency != nil && strings.TrimSpace(*native.Currency) == "" || native.NextReset != nil && *native.NextReset < 0 || native.CurrentOverage != nil && (!providerOverageAmount.MatchString(native.CurrentOverage.Amount) || strings.TrimSpace(native.CurrentOverage.Currency) == "") {
		return llmproxycontract.ProviderSubscription{}, false
	}
	return llmproxycontract.ProviderSubscription{Tier: native.Tier, Status: native.Status, CharacterCount: *native.CharacterCount, CharacterLimit: *native.CharacterLimit, CreditExtension: extension, CanExtendCreditLimit: *native.CanExtendCreditLimit, CurrentOverage: native.CurrentOverage, HasOpenInvoices: *native.HasOpenInvoices, Currency: native.Currency, NextCharacterCountResetUnix: native.NextReset}, true
}
