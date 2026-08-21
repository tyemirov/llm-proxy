package main

import (
	"encoding/json"
	"io"
)

type providerCatalogDiscovery struct {
	SchemaVersion int                                `json:"schema_version"`
	Providers     []providerCatalogDiscoveryProvider `json:"providers"`
}

type providerCatalogDiscoveryProvider struct {
	ID     string                          `json:"id"`
	Fields []providerCatalogDiscoveryField `json:"fields"`
}

type providerCatalogDiscoveryField struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Required    bool   `json:"required"`
	Environment string `json:"environment"`
}

func loadProviderCatalogDiscovery(rawConfigPath string) (providerCatalogDiscovery, error) {
	catalog, catalogError := loadProviderCatalog(normalizedConfigPath(rawConfigPath))
	if catalogError != nil {
		return providerCatalogDiscovery{}, catalogError
	}
	discovery := providerCatalogDiscovery{SchemaVersion: catalog.SchemaVersion()}
	for _, provider := range catalog.Schema().Providers {
		providerDiscovery := providerCatalogDiscoveryProvider{ID: provider.ID}
		for _, field := range provider.Fields {
			providerDiscovery.Fields = append(providerDiscovery.Fields, providerCatalogDiscoveryField{
				ID: field.ID, Kind: field.Kind, Required: field.Required, Environment: field.Environment,
			})
		}
		discovery.Providers = append(discovery.Providers, providerDiscovery)
	}
	return discovery, nil
}

func writeProviderCatalogDiscovery(output io.Writer, discovery providerCatalogDiscovery) error {
	return json.NewEncoder(output).Encode(discovery)
}
