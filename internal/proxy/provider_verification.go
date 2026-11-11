package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
)

func validateProviderCatalogVerification(provider ProviderCatalogProvider, transports map[string]ProviderCatalogTransport, field string) error {
	verification := provider.Verification
	transport, found := transports[verification.Transport]
	if !found {
		return fmt.Errorf("%w: field=%s.transport transport=%s reason=dangling_reference", ErrInvalidModelCatalog, field, verification.Transport)
	}
	codec := transport.Components.RequestCodec.ID
	if codec == CatalogProtocolHeyGenAccount || codec == CatalogProtocolJSONResource || codec == CatalogProtocolSystemOneModels || codec == CatalogProtocolElevenLabsSubscription || codec == CatalogProtocolDictatorSpeechV1 {
		if verification.Model != "" {
			return fmt.Errorf("%w: field=%s.model reason=resource_verification_has_no_model", ErrInvalidModelCatalog, field)
		}
		return nil
	}
	if knownTextWireContract(textWireContract(codec)) {
		for _, offering := range provider.Offerings {
			if offering.Model == verification.Model && offering.Transport == verification.Transport && slices.Contains(offering.Operations, ModelOperationText) {
				return nil
			}
		}
	}
	return fmt.Errorf("%w: field=%s reason=unsupported_verification_route", ErrInvalidModelCatalog, field)
}

func (verifier *operationalProviderKeyVerifier) verifyJSONResource(ctx context.Context, provider providerDefinition, apiKey string) error {
	// Catalog, connection, and upstream admission validation establish this URL.
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, provider.textEndpointURL, nil)
	request.Header.Set("Accept", "application/json")
	client := newProviderTransportHTTPDoer(verifier.httpClient, provider, apiKey)
	response, err := client.Do(request)
	if err != nil {
		return providerKeyVerificationTransportError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return providerKeyVerificationStatusError(response.StatusCode)
	}
	content, err := readProviderKeyVerificationResponse(response.Body)
	if err != nil {
		return err
	}
	if provider.activeTransport.responseCodec == CatalogProtocolSystemOneModels {
		if !validSystemOneModelsResponse(content) {
			return errProviderKeyVerificationUnavailable
		}
		return nil
	}
	var resource map[string]json.RawMessage
	if err := json.Unmarshal(content, &resource); err != nil || resource == nil {
		return errProviderKeyVerificationUnavailable
	}
	return nil
}

func validSystemOneModelsResponse(content []byte) bool {
	var resource struct {
		Models *[]struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			ReleaseDate *string `json:"release_date"`
		} `json:"models"`
	}
	if err := json.Unmarshal(content, &resource); err != nil || resource.Models == nil {
		return false
	}
	for _, model := range *resource.Models {
		if model.Name == nil || model.Description == nil || model.ReleaseDate == nil {
			return false
		}
	}
	return true
}
