package proxy

import (
	"context"
	"fmt"
	"net/url"
	"sort"
)

// ConfiguredUpstreamOrigins returns the exact HTTP origins of configured provider
// transports. Unset tenant URL fields have no origin until a connection supplies
// their value. This projection supplies no capacity defaults.
func (configuration Configuration) ConfiguredUpstreamOrigins() ([]string, error) {
	if configuration.ProviderCatalog == nil {
		return nil, fmt.Errorf("%w: field=provider_catalog", ErrInvalidUpstreamCapacity)
	}
	registry := newProviderRegistry(configuration)
	origins := map[string]struct{}{}
	for _, definition := range registry.definitions {
		for identifier, transport := range definition.transports {
			if transport.endpoint.Protocol != CatalogEndpointProtocolHTTP {
				continue
			}
			if transport.endpoint.SettingField != "" && definition.connectionValues[transport.endpoint.SettingField] == "" && transport.endpointURLOverride == "" {
				continue
			}
			resolved, _ := definition.resolvedTransport(identifier)
			origin, err := configuredEndpointOrigin(resolved.textEndpointURL)
			if err != nil {
				return nil, fmt.Errorf("%w: provider=%s transport=%s", err, definition.identifier, identifier)
			}
			origins[origin] = struct{}{}
		}
	}
	if configuration.Endpoints != nil && configuration.Endpoints.GetModelsURL() != "" {
		origin, err := configuredEndpointOrigin(configuration.Endpoints.GetModelsURL())
		if err != nil {
			return nil, fmt.Errorf("%w: endpoint=models", err)
		}
		origins[origin] = struct{}{}
	}
	result := make([]string, 0, len(origins))
	for origin := range origins {
		result = append(result, origin)
	}
	sort.Strings(result)
	return result, nil
}

func configuredEndpointOrigin(raw string) (string, error) {
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.User != nil {
		return "", ErrInvalidUpstreamCapacity
	}
	origin, err := normalizedUpstreamOrigin(upstreamRequestOrigin(endpoint))
	if err != nil {
		return "", ErrInvalidUpstreamCapacity
	}
	return origin, nil
}

func validateUpstreamCapacityOrigins(configuration Configuration, raw UpstreamCapacityConfiguration, capacity upstreamCapacity) error {
	configured, err := configuration.ConfiguredUpstreamOrigins()
	if err != nil {
		return err
	}
	required := make(map[string]struct{}, len(configured))
	for _, origin := range configured {
		required[origin] = struct{}{}
		if _, declared := capacity.origins[origin]; !declared {
			return fmt.Errorf("%w: origin=%s reason=missing", ErrInvalidUpstreamCapacity, origin)
		}
	}
	connectionProviders := map[string]bool{}
	for _, provider := range configuration.ProviderCatalog.runtimeSchema.Providers {
		for _, transport := range provider.Transports {
			if transport.Endpoint.Protocol == CatalogEndpointProtocolHTTP && transport.Endpoint.SettingField != "" {
				connectionProviders[provider.ID] = true
			}
		}
	}
	for _, rule := range raw.Origins {
		origin, _ := normalizedUpstreamOrigin(rule.Origin)
		if rule.Provider != "" && !connectionProviders[rule.Provider] {
			return fmt.Errorf("%w: origin=%s field=provider reason=unknown_connection_origin_provider", ErrInvalidUpstreamCapacity, origin)
		}
		if _, configured := required[origin]; !configured && !connectionProviders[rule.Provider] {
			return fmt.Errorf("%w: origin=%s reason=unknown", ErrInvalidUpstreamCapacity, origin)
		}
	}
	return nil
}

func validateSavedUpstreamCapacityOrigins(ctx context.Context, store *managedTenantStore, providers *providerRegistry, capacity upstreamCapacity) error {
	err := store.database.streamAccountConnections(ctx, func(record managedAccountConnectionRecord) error {
		// Store initialization validates the connection schema and field values.
		// Origin projection needs only non-secret settings, not decrypted credentials.
		definition := providers.definitions[providerID(record.ProviderID)]
		definition.connectionValues = make(map[string]string, len(definition.fields))
		for id, field := range definition.fields {
			if !field.Secret {
				definition.connectionValues[id] = *field.Default
			}
		}
		for _, field := range record.Fields {
			if !definition.fields[field.FieldID].Secret {
				definition.connectionValues[field.FieldID] = field.Value
			}
		}
		for identifier, transport := range definition.transports {
			if transport.endpoint.Protocol != CatalogEndpointProtocolHTTP {
				continue
			}
			resolved, _ := definition.resolvedTransport(identifier)
			endpoint, _ := url.Parse(resolved.textEndpointURL)
			origin := upstreamRequestOrigin(endpoint)
			if _, declared := capacity.origins[origin]; !declared {
				return fmt.Errorf("%w: connection=%s origin=%s reason=missing", ErrInvalidUpstreamCapacity, record.ID, origin)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("validate saved upstream origins: %w", err)
	}
	return nil
}
