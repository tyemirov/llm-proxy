package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

type elevenLabsVoiceProvider struct {
	provider  providerDefinition
	transport string
	tenants   *managedTenantStore
	store     *mediaOperationStore
	client    HTTPDoer
	revision  string
}

type elevenLabsVoiceReference struct {
	Authority string `json:"authority"`
	VoiceID   string `json:"voice_id"`
}

func (adapter *elevenLabsVoiceProvider) DiscoverMediaVoices(ctx context.Context, tenant string, query MediaVoiceQuery) (MediaVoiceDiscovery, error) {
	access, hosted := ctx.Value(hostedVoiceReadContextKey{}).(hostedVoiceRead)
	if hosted {
		if (query.VoiceType != "" && query.VoiceType != "default") || (query.Category != "" && query.Category != "premade") {
			return MediaVoiceDiscovery{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceInvalid)
		}
		query.VoiceType, query.Category = "default", "premade"
	}
	credential, err := adapter.store.credentialReference(ctx, tenant, adapter.provider.identifier)
	if err != nil {
		return MediaVoiceDiscovery{}, err
	}
	provider, err := resolveMediaProvider(ctx, MediaOperationExecutionRequest{TenantID: tenant, CredentialReference: credential}, adapter.provider.identifier.string(), adapter.transport, adapter.provider, adapter.tenants, adapter.store)
	if err != nil {
		return MediaVoiceDiscovery{}, err
	}
	account, _, _ := strings.Cut(credential, ":v")
	provider.upstreamScope = upstreamRequestScope{tenant: tenant, account: account, class: upstreamInteractive}
	target, _ := url.Parse(provider.textEndpointURL)
	values := target.Query()
	values.Set("page_size", strconv.Itoa(query.PageSize))
	voiceType := query.VoiceType
	if voiceType == "account" {
		voiceType = "workspace"
	}
	for key, value := range map[string]string{"search": query.Search, "sort": query.Sort, "sort_direction": query.SortDirection, "voice_type": voiceType, "category": query.Category, "next_page_token": query.PageToken} {
		if value != "" {
			values.Set(key, value)
		}
	}
	if query.IncludeTotalCount != nil {
		values.Set("include_total_count", strconv.FormatBool(*query.IncludeTotalCount))
	}
	target.RawQuery = values.Encode()
	ctx, cancel := context.WithTimeout(ctx, providerMetadataTimeout)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	request.Header.Set("Accept", "application/json")
	client := adapter.client
	if hosted {
		client = &hostedMediaHTTPDoer{next: client, authorize: access.authorize, role: hostedProviderMetadata}
	}
	response, err := newProviderTransportHTTPDoer(client, provider, provider.credentialFor(endpointKindText)).Do(request)
	if err != nil {
		return MediaVoiceDiscovery{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, providerMetadataMaximumBytes+1))
	if err != nil || response.StatusCode != http.StatusOK || len(body) > providerMetadataMaximumBytes {
		return MediaVoiceDiscovery{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider)
	}
	discovery, err := decodeElevenLabsVoices(body, provider.identifier.string(), credential+":"+adapter.revision, provider.activeTransport.artifactOrigins)
	if err != nil {
		return MediaVoiceDiscovery{}, err
	}
	if hosted {
		for _, voice := range discovery.Voices {
			if voice.Metadata.Category == nil || *voice.Metadata.Category != "premade" {
				return MediaVoiceDiscovery{}, errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider)
			}
		}
	}
	return discovery, nil
}

func decodeElevenLabsVoices(body []byte, provider, authority string, origins []string) (MediaVoiceDiscovery, error) {
	type nativeLanguage struct {
		Language   string  `json:"language"`
		ModelID    string  `json:"model_id"`
		Accent     *string `json:"accent"`
		Locale     *string `json:"locale"`
		PreviewURL *string `json:"preview_url"`
	}
	type nativeVoice struct {
		VoiceID                 string            `json:"voice_id"`
		Name                    string            `json:"name"`
		Category                *string           `json:"category"`
		Description             *string           `json:"description"`
		Labels                  map[string]string `json:"labels"`
		HighQualityBaseModelIDs []string          `json:"high_quality_base_model_ids"`
		VerifiedLanguages       []nativeLanguage  `json:"verified_languages"`
		PreviewURL              *string           `json:"preview_url"`
	}
	var native struct {
		Voices        []nativeVoice `json:"voices"`
		HasMore       *bool         `json:"has_more"`
		TotalCount    *int          `json:"total_count"`
		NextPageToken *string       `json:"next_page_token"`
	}
	invalid := errors.New(llmproxycontract.ErrorCodeMediaVoiceProvider)
	if json.Unmarshal(body, &native) != nil || native.Voices == nil || native.HasMore == nil || native.TotalCount != nil && *native.TotalCount < 0 {
		return MediaVoiceDiscovery{}, invalid
	}
	token := ""
	if native.NextPageToken != nil {
		token = *native.NextPageToken
	}
	if *native.HasMore != (strings.TrimSpace(token) != "") || len(token) > 4096 {
		return MediaVoiceDiscovery{}, invalid
	}
	result := MediaVoiceDiscovery{Authority: authority, NativePage: true, TotalCount: native.TotalCount, NextPageToken: token, Voices: make([]MediaVoiceProviderRecord, 0, len(native.Voices))}
	seen := map[string]bool{}
	for _, voice := range native.Voices {
		if strings.TrimSpace(voice.VoiceID) == "" || voice.VoiceID != strings.TrimSpace(voice.VoiceID) || strings.TrimSpace(voice.Name) == "" || seen[voice.VoiceID] {
			return MediaVoiceDiscovery{}, invalid
		}
		seen[voice.VoiceID] = true
		metadata := llmproxycontract.MediaVoiceMetadata{Description: voice.Description, Category: voice.Category, Labels: voice.Labels, HighQualityBaseModelIDs: voice.HighQualityBaseModelIDs, VerifiedLanguages: []llmproxycontract.MediaVoiceLanguage{}}
		previews := []string{""}
		if voice.PreviewURL != nil {
			previews[0] = *voice.PreviewURL
		}
		for _, language := range voice.VerifiedLanguages {
			if strings.TrimSpace(language.Language) == "" || strings.TrimSpace(language.ModelID) == "" {
				return MediaVoiceDiscovery{}, invalid
			}
			metadata.VerifiedLanguages = append(metadata.VerifiedLanguages, llmproxycontract.MediaVoiceLanguage{Language: language.Language, ModelID: language.ModelID, Accent: language.Accent, Locale: language.Locale})
			preview := ""
			if language.PreviewURL != nil {
				preview = *language.PreviewURL
			}
			previews = append(previews, preview)
		}
		for _, preview := range previews {
			if preview != "" && !validMediaVoicePreviewURL(preview, origins) {
				return MediaVoiceDiscovery{}, invalid
			}
		}
		for _, model := range metadata.HighQualityBaseModelIDs {
			if strings.TrimSpace(model) == "" {
				return MediaVoiceDiscovery{}, invalid
			}
		}
		reference, _ := json.Marshal(elevenLabsVoiceReference{Authority: authority, VoiceID: voice.VoiceID})
		result.Voices = append(result.Voices, MediaVoiceProviderRecord{Authority: authority, Provider: provider, Mode: MediaVoiceModePreset, DisplayName: voice.Name, Metadata: metadata, PreviewURLs: previews, ProviderVoiceReference: string(reference)})
	}
	return result, nil
}

func validMediaVoicePreviewURL(raw string, origins []string) bool {
	target, err := url.Parse(raw)
	return err == nil && target.User == nil && target.Fragment == "" && slices.Contains(origins, upstreamRequestOrigin(target))
}

func (adapter *elevenLabsVoiceProvider) MediaVoiceAuthority(ctx context.Context, tenant string) (string, error) {
	credential, err := adapter.store.credentialReference(ctx, tenant, adapter.provider.identifier)
	return credential + ":" + adapter.revision, err
}
