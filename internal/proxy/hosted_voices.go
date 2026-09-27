package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"gorm.io/gorm"
)

type hostedVoiceReadContextKey struct{}
type hostedVoiceRead struct {
	request managedJournalRequestRecord
	owner   string
	service *mediaOperationService
}

// Voice metadata uses current speech authority without admitting paid work.
// The same credential resolver serves discovery and accepted media operations.
func (service *mediaOperationService) voiceReadContext(ctx context.Context, requestTenant tenant, provider string) (context.Context, error) {
	grantID := requestTenant.providerSettings[providerID(provider)].hostedGrantID
	if grantID == "" {
		return ctx, nil
	}
	if service.hostedAdmission == nil {
		return ctx, errHostedAuthorityDenied
	}
	var grant managedHostedGrantRecord
	err := service.store.database.WithContext(ctx).Where("id = ? AND tenant_id = ? AND provider = ?", grantID, requestTenant.identifier.string(), provider).First(&grant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ctx, errHostedAuthorityDenied
	}
	if err != nil {
		return ctx, fmt.Errorf("read voice grant: %w", err)
	}
	var offerings []hostedGrantOffering
	if err := json.Unmarshal(grant.Offerings, &offerings); err != nil {
		return ctx, fmt.Errorf("decode voice grant: %w", err)
	}
	request := managedJournalRequestRecord{TenantID: requestTenant.identifier.string(), Provider: provider, CatalogRevision: service.catalog.Revision(), CreatedAt: service.store.now()}
	for _, offering := range offerings {
		for _, operation := range []string{ModelOperationSpeechGeneration, ModelOperationSpeechConversion} {
			if slices.Contains(offering.Operations, operation) {
				request.Model, request.Operation = offering.Model, operation
				break
			}
		}
		if request.Operation != "" {
			break
		}
	}
	if request.Operation == "" {
		return ctx, errHostedAuthorityDenied
	}
	if err := bindJournalAuthority(service.store.database.WithContext(ctx), requestTenant.userID, &request); err != nil {
		return ctx, err
	}
	access := hostedVoiceRead{request: request, owner: requestTenant.userID, service: service}
	ctx = context.WithValue(ctx, hostedVoiceReadContextKey{}, access)
	ctx = context.WithValue(ctx, hostedMediaRequestContextKey{}, request)
	return context.WithValue(ctx, hostedMediaAuthorizationContextKey{}, hostedMediaAuthorize(access.authorize)), nil
}

func (access hostedVoiceRead) authorize(ctx context.Context, role hostedProviderRole) error {
	if role != hostedProviderMetadata {
		return errHostedAuthorityDenied
	}
	current := access.request
	current.CreatedAt = access.service.store.now()
	if err := bindJournalAuthority(access.service.store.database.WithContext(ctx), access.owner, &current); err != nil {
		return err
	}
	if current.GrantID != access.request.GrantID || current.GrantRevision != access.request.GrantRevision || current.PlatformConnectionID != access.request.PlatformConnectionID || current.CredentialVersion != access.request.CredentialVersion {
		return errHostedAuthorityDenied
	}
	return nil
}

func authorizeVoiceRead(ctx context.Context) error {
	if access, hosted := ctx.Value(hostedVoiceReadContextKey{}).(hostedVoiceRead); hosted {
		return access.authorize(ctx, hostedProviderMetadata)
	}
	return nil
}

func voiceCursorAuthority(ctx context.Context, authority string) string {
	if access, hosted := ctx.Value(hostedVoiceReadContextKey{}).(hostedVoiceRead); hosted {
		return authority + ":" + access.request.GrantID + ":" + strconv.FormatUint(access.request.GrantRevision, 10) + ":" + mediaJournalCredentialReference(access.request)
	}
	return authority
}
