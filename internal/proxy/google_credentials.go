package proxy

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
)

const googleCloudScope = "https://www.googleapis.com/auth/cloud-platform"
const googleQuotaProjectHeader = "x-goog-user-project"

// GoogleCredentialProfile is an operator-owned, tenant-bound Google credential reference.
// CredentialsFile must refer to a trusted operator-provisioned Google credential file.
type GoogleCredentialProfile struct {
	ID              string `mapstructure:"id"`
	TenantID        string `mapstructure:"tenant_id"`
	Project         string `mapstructure:"project"`
	Location        string `mapstructure:"location"`
	CredentialsFile string `mapstructure:"credentials_file"`
	CredentialType  string `mapstructure:"credential_type"`
}

type googleCredentialProfile struct {
	tenantID    string
	project     string
	location    string
	credentials *auth.Credentials
}

var googleProjectPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{4,61}[a-z0-9]$`)

func newGoogleCredentialProfiles(inputs []GoogleCredentialProfile) (map[string]googleCredentialProfile, error) {
	profiles := make(map[string]googleCredentialProfile, len(inputs))
	for _, input := range inputs {
		if strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.ID) != input.ID || strings.TrimSpace(input.TenantID) == "" || input.TenantID != strings.TrimSpace(input.TenantID) || !googleProjectPattern.MatchString(input.Project) || input.Location != "global" || !filepath.IsAbs(input.CredentialsFile) {
			return nil, fmt.Errorf("invalid Google credential profile")
		}
		if _, exists := profiles[input.ID]; exists {
			return nil, fmt.Errorf("duplicate Google credential profile: %s", input.ID)
		}
		credentialType := credentials.CredType(input.CredentialType)
		switch credentialType {
		case credentials.ServiceAccount, credentials.ExternalAccount, credentials.ImpersonatedServiceAccount:
		default:
			return nil, fmt.Errorf("invalid Google credential type: profile=%s", input.ID)
		}
		googleCredentials, err := credentials.NewCredentialsFromFile(credentialType, input.CredentialsFile, &credentials.DetectOptions{Scopes: []string{googleCloudScope}})
		if err != nil {
			return nil, fmt.Errorf("load Google credential profile %s: %w", input.ID, err)
		}
		profiles[input.ID] = googleCredentialProfile{tenantID: input.TenantID, project: input.Project, location: input.Location, credentials: googleCredentials}
	}
	return profiles, nil
}

func (provider providerDefinition) googleCredentialProfile(reference string) (googleCredentialProfile, error) {
	profile, found := provider.googleCredentials[reference]
	if !found || profile.tenantID != provider.tenantIdentifier {
		return googleCredentialProfile{}, fmt.Errorf("%w: Google credential profile is unavailable for this tenant", ErrProviderAPI)
	}
	return profile, nil
}
