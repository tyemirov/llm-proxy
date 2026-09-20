package proxy

import (
	"fmt"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

// ProviderCatalogResource binds a typed account resource to a provider transport.
type ProviderCatalogResource struct {
	Kind      llmproxycontract.ProviderResourceKind `yaml:"kind"`
	Transport string                                `yaml:"transport"`
}

func validateProviderCatalogResources(resources []ProviderCatalogResource, transports map[string]ProviderCatalogTransport, field string) error {
	kinds := map[llmproxycontract.ProviderResourceKind]bool{}
	for index, resource := range resources {
		resourceField := fmt.Sprintf("%s[%d]", field, index)
		if !llmproxycontract.ValidProviderResourceKind(resource.Kind) {
			return fmt.Errorf("%w: field=%s.kind reason=unknown_resource_kind", ErrInvalidModelCatalog, resourceField)
		}
		if kinds[resource.Kind] {
			return fmt.Errorf("%w: field=%s.kind reason=duplicate_resource_kind", ErrInvalidModelCatalog, resourceField)
		}
		kinds[resource.Kind] = true
		transport, found := transports[resource.Transport]
		if !found {
			return fmt.Errorf("%w: field=%s.transport reason=dangling_reference", ErrInvalidModelCatalog, resourceField)
		}
		codec := transport.Components.RequestCodec.ID
		supported := resource.Kind == llmproxycontract.ProviderResourceVoices && (codec == CatalogProtocolDictatorSpeechV1 || codec == CatalogProtocolElevenLabsVoices) || resource.Kind == llmproxycontract.ProviderResourceMetadata && codec == CatalogProtocolElevenLabsModels || resource.Kind == llmproxycontract.ProviderResourceQuotas && codec == CatalogProtocolElevenLabsSubscription
		if !supported {
			return fmt.Errorf("%w: field=%s reason=unsupported_resource_composition", ErrInvalidModelCatalog, resourceField)
		}
	}
	return nil
}

func providerResourceKinds(resources []ProviderCatalogResource) []llmproxycontract.ProviderResourceKind {
	kinds := make([]llmproxycontract.ProviderResourceKind, 0, len(resources))
	for _, resource := range resources {
		kinds = append(kinds, resource.Kind)
	}
	return kinds
}
