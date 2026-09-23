package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

func loadJournalProvider(ctx context.Context, database *gorm.DB, cipher managedProviderKeyCipher, accepted managedJournalRequestRecord, provider providerDefinition) (providerDefinition, error) {
	var credential managedPlatformCredentialRecord
	if err := database.WithContext(ctx).Where("connection_id = ? AND version = ?", accepted.PlatformConnectionID, accepted.CredentialVersion).First(&credential).Error; err != nil {
		return providerDefinition{}, fmt.Errorf("load credential for request %s: %w", accepted.ID, err)
	}
	var fields map[string]string
	if err := json.Unmarshal(credential.Fields, &fields); err != nil {
		return providerDefinition{}, fmt.Errorf("decode credential for request %s: %w", accepted.ID, err)
	}
	values := make(map[string]string, len(fields))
	for name, value := range fields {
		field, known := provider.fields[name]
		if !known {
			return providerDefinition{}, errHostedAuthorityDenied
		}
		if field.Kind == CatalogProviderFieldKindCredential {
			var err error
			value, err = cipher.decryptConnectionValue(platformCredentialReference(accepted.PlatformConnectionID, accepted.CredentialVersion), accepted.Provider, name, value)
			if err != nil {
				return providerDefinition{}, fmt.Errorf("decrypt credential for request %s: %w", accepted.ID, err)
			}
		}
		values[name] = value
	}
	provider.connectionValues = values
	provider.upstreamScope.account = accepted.PlatformConnectionID
	provider.hostedGrantID = ""
	return provider, nil
}
