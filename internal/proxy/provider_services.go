package proxy

import "fmt"

// ProviderCatalogService binds a model-free operation to the provider's transport.
// Transport is private; public projections contain operation controls, limits and prices.
type ProviderCatalogService struct {
	Operation string               `yaml:"operation" json:"operation"`
	Transport string               `yaml:"transport" json:"-"`
	Controls  []CatalogControl     `yaml:"controls" json:"controls"`
	Limits    []CatalogLimit       `yaml:"limits" json:"limits"`
	Price     ProviderCatalogPrice `yaml:"price" json:"price"`
}

func validateProviderCatalogServices(services []ProviderCatalogService, transports map[string]ProviderCatalogTransport, field string) error {
	for index, service := range services {
		transport, found := transports[service.Transport]
		if !found {
			return fmt.Errorf("%w: field=%s[%d].transport reason=dangling_reference", ErrInvalidModelCatalog, field, index)
		}
		expectedOperation, supported := map[string]string{CatalogProtocolHeyGenAvatarV3: ModelOperationAvatarCreation, CatalogProtocolHeyGenAvatarVideoV3: ModelOperationAvatarVideoGeneration, CatalogProtocolHeyGenLipSync: ModelOperationVideoLipSync, CatalogProtocolHeyGenTranslation: ModelOperationVideoTranslation, CatalogProtocolElevenLabsAlignment: ModelOperationAudioAlignment, CatalogProtocolElevenLabsDictionary: ModelOperationPronunciationDictionaryCreation}[transport.Components.RequestCodec.ID]
		if !supported || service.Operation != expectedOperation {
			return fmt.Errorf("%w: field=%s[%d] reason=unsupported_service_composition", ErrInvalidModelCatalog, field, index)
		}
		if expectedOperation == ModelOperationAvatarCreation || expectedOperation == ModelOperationAvatarVideoGeneration {
			if err := validateHeyGenAvatarService(service, transport, field); err != nil {
				return err
			}
			continue
		}
		if expectedOperation == ModelOperationVideoLipSync || expectedOperation == ModelOperationVideoTranslation {
			if err := validateHeyGenService(service, transport, field); err != nil {
				return err
			}
			continue
		}
		if expectedOperation == ModelOperationPronunciationDictionaryCreation {
			if len(service.Controls) != 0 || len(service.Limits) != 0 {
				return fmt.Errorf("%w: field=%s[%d] reason=invalid_dictionary_service", ErrInvalidModelCatalog, field, index)
			}
			continue
		}
		if len(service.Controls) != 0 || len(service.Limits) != 2 {
			return fmt.Errorf("%w: field=%s[%d] reason=invalid_alignment_limits", ErrInvalidModelCatalog, field, index)
		}
		for _, limit := range service.Limits {
			valid := limit.Value != nil && !limit.AccountDependent
			if valid {
				switch limit.ID {
				case alignmentInputLimit:
					valid = limit.Unit == "bytes" && *limit.Value > 0 && *limit.Value <= 1_000_000_000
				case alignmentDurationDimension:
					valid = limit.Unit == "seconds" && *limit.Value == alignmentMaximumInputSeconds
				default:
					valid = false
				}
			}
			if !valid {
				return fmt.Errorf("%w: field=%s[%d] reason=invalid_alignment_limits", ErrInvalidModelCatalog, field, index)
			}
		}
	}
	return nil
}

func validateCatalogServices(providers []CatalogProvider, operations map[string]ModelOperationKind) error {
	for _, provider := range providers {
		seen := map[string]bool{}
		for index, service := range provider.Services {
			field := fmt.Sprintf("catalog.providers.%s.services[%d]", provider.ID, index)
			if _, found := operations[service.Operation]; !found || seen[service.Operation] || service.Price.Operation != service.Operation {
				return fmt.Errorf("%w: field=%s reason=invalid_service_operation", ErrInvalidModelCatalog, field)
			}
			seen[service.Operation] = true
			if _, err := canonicalCatalogIdentifier(service.Transport, field+".transport"); err != nil {
				return err
			}
			if err := validateCatalogControls(service.Controls, field+".controls"); err != nil {
				return err
			}
			if err := validateCatalogLimits(service.Limits, field+".limits"); err != nil {
				return err
			}
			if err := validateCatalogPriceValues(servicePriceDescriptor(service.Price), field+".price"); err != nil {
				return err
			}
		}
	}
	return nil
}

func servicePriceDescriptor(price ProviderCatalogPrice) CatalogPriceDescriptor {
	return CatalogPriceDescriptor{Operation: price.Operation, Available: price.Available, Rates: price.Rates, MinimumCharge: price.MinimumCharge, Source: price.Source, LastVerified: price.LastVerified, UnavailableReason: price.UnavailableReason}
}

func cloneProviderServices(services []ProviderCatalogService) []ProviderCatalogService {
	result := make([]ProviderCatalogService, len(services))
	for index, service := range services {
		route := cloneProviderOffering(ProviderOffering{Controls: service.Controls, Limits: service.Limits})
		result[index] = service
		result[index].Controls = route.Controls
		for controlIndex := range result[index].Controls {
			if result[index].Controls[controlIndex].Values == nil {
				result[index].Controls[controlIndex].Values = []string{}
			}
		}
		result[index].Limits = route.Limits
		result[index].Price.Rates = append([]CatalogPriceRate{}, service.Price.Rates...)
		if service.Price.MinimumCharge != nil {
			minimum := *service.Price.MinimumCharge
			result[index].Price.MinimumCharge = &minimum
		}
	}
	return result
}

// ResolveService resolves one explicit provider service without model selection.
func (service CatalogService) ResolveService(provider, operation string) (ProviderCatalogService, error) {
	for _, route := range service.catalog.providers[provider].Services {
		if route.Operation == operation {
			return cloneProviderServices([]ProviderCatalogService{route})[0], nil
		}
	}
	return ProviderCatalogService{}, fmt.Errorf("%w: provider=%s operation=%s reason=unknown_service", ErrInvalidModelCatalog, provider, operation)
}
