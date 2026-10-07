package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const (
	// CatalogProtocolHeyGenAvatarV3 creates photo avatars through the native v3 API.
	CatalogProtocolHeyGenAvatarV3 = "heygen_avatar_v3"
	// CatalogProtocolHeyGenAvatarVideoV3 renders retained avatars through the native v3 API.
	CatalogProtocolHeyGenAvatarVideoV3 = "heygen_avatar_video_v3"
	heygenAvatarImageBytes             = "input_image_bytes"
)

var mediaAvatarIdentifierPattern = regexp.MustCompile(`^ava_[0-9a-f]{32}$`)

type mediaAvatarRecord struct {
	AvatarID            string `gorm:"primaryKey"`
	TenantID            string `gorm:"not null;index"`
	Provider            string `gorm:"not null"`
	CredentialReference string `gorm:"not null"`
	NativeLookID        string `gorm:"not null"`
	Metadata            []byte `gorm:"not null"`
}

type providerHeyGenAvatarAdapter struct {
	route    ProviderCatalogService
	provider providerDefinition
	tenants  *managedTenantStore
	store    *mediaOperationStore
	assets   *tenantAssetStore
	binding  string
	limits   map[string]int64
}

func newProviderHeyGenAvatarAdapter(route ProviderCatalogService, provider providerDefinition, tenants *managedTenantStore, store *mediaOperationStore, assets *tenantAssetStore) *providerHeyGenAvatarAdapter {
	codec := CatalogProtocolHeyGenAvatarV3
	if route.Operation == ModelOperationAvatarVideoGeneration {
		codec = CatalogProtocolHeyGenAvatarVideoV3
	}
	adapter := &providerHeyGenAvatarAdapter{route: route, provider: provider, tenants: tenants, store: store, assets: assets, binding: providerServiceBinding(route, provider, codec), limits: map[string]int64{}}
	for _, limit := range route.Limits {
		adapter.limits[limit.ID] = int64(*limit.Value)
	}
	return adapter
}

func (adapter *providerHeyGenAvatarAdapter) Validate(ctx context.Context, request MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	if adapter.route.Operation == ModelOperationAvatarCreation {
		var input llmproxycontract.AvatarCreationSource
		var controls struct{}
		if !utf8.Valid(request.Input) || !utf8.Valid(request.Controls) || !decodeImageRequestObject(request.Input, &input) || !decodeImageRequestObject(request.Controls, &controls) || strings.TrimSpace(input.Name) == "" || !heygenAssetAllowed(adapter.assets, request.TenantID, input.ImageAssetID, "image/", adapter.limits[heygenAvatarImageBytes]) {
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
		input.Name = strings.TrimSpace(input.Name)
		return MediaOperationValidatedRequest{ExecutionBinding: adapter.binding, Input: heygenEncode(input), Controls: json.RawMessage(`{}`), InputAssetIDs: []string{input.ImageAssetID}}, nil
	}
	var input llmproxycontract.AvatarVideoSource
	var controls llmproxycontract.AvatarVideoControls
	if !utf8.Valid(request.Input) || !utf8.Valid(request.Controls) || !decodeImageRequestObject(request.Input, &input) || !decodeImageRequestObject(request.Controls, &controls) || controls.Engine != "avatar_iv" || !slices.Contains([]string{"16:9", "9:16", "1:1"}, controls.AspectRatio) || !slices.Contains([]string{"720p", "1080p"}, controls.Resolution) || controls.Expressiveness != "" && !slices.Contains([]string{"low", "medium", "high"}, controls.Expressiveness) || !heygenAssetAllowed(adapter.assets, request.TenantID, input.AudioAssetID, "audio/", adapter.limits[heygenInputAudioBytes]) {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	if _, err := adapter.avatar(ctx, request.TenantID, request.Provider, request.CredentialReference, input.AvatarID); err != nil {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	return MediaOperationValidatedRequest{ExecutionBinding: adapter.binding, Input: heygenEncode(input), Controls: heygenEncode(controls), InputAssetIDs: []string{input.AudioAssetID}}, nil
}

func (adapter *providerHeyGenAvatarAdapter) avatar(ctx context.Context, tenantIDValue, provider, credential, avatarID string) (mediaAvatarRecord, error) {
	if !mediaAvatarIdentifierPattern.MatchString(avatarID) {
		return mediaAvatarRecord{}, errMediaOperationInvalid
	}
	var record mediaAvatarRecord
	err := adapter.store.database.WithContext(ctx).Where("avatar_id = ? AND tenant_id = ? AND provider = ? AND credential_reference = ?", avatarID, tenantIDValue, provider, credential).First(&record).Error
	return record, err
}

func (adapter *providerHeyGenAvatarAdapter) Execute(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	provider, err := resolveMediaProvider(ctx, request, adapter.provider.identifier.string(), adapter.route.Transport, adapter.provider, adapter.tenants, adapter.store)
	if err != nil || request.ExecutionBinding != adapter.binding {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	var body any
	if adapter.route.Operation == ModelOperationAvatarCreation {
		var input llmproxycontract.AvatarCreationSource
		_ = json.Unmarshal(request.Input, &input)
		assetID, err := heygenUploadAsset(ctx, request, provider, adapter.assets, input.ImageAssetID)
		if err != nil {
			return imageGenerationUncertain()
		}
		body = struct {
			Type string            `json:"type"`
			Name string            `json:"name"`
			File heygenNativeAsset `json:"file"`
		}{"photo", input.Name, heygenNativeAsset{Type: "asset_id", AssetID: assetID}}
	} else {
		var input llmproxycontract.AvatarVideoSource
		var controls llmproxycontract.AvatarVideoControls
		_ = json.Unmarshal(request.Input, &input)
		_ = json.Unmarshal(request.Controls, &controls)
		avatar, err := adapter.avatar(ctx, request.TenantID, request.Provider, request.CredentialReference, input.AvatarID)
		if err != nil {
			return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
		}
		assetID, err := heygenUploadAsset(ctx, request, provider, adapter.assets, input.AudioAssetID)
		if err != nil {
			return imageGenerationUncertain()
		}
		body = struct {
			Type         string `json:"type"`
			AvatarID     string `json:"avatar_id"`
			AudioAssetID string `json:"audio_asset_id"`
			Engine       struct {
				Type string `json:"type"`
			} `json:"engine"`
			AspectRatio    string `json:"aspect_ratio"`
			Resolution     string `json:"resolution"`
			Title          string `json:"title,omitempty"`
			MotionPrompt   string `json:"motion_prompt,omitempty"`
			Expressiveness string `json:"expressiveness,omitempty"`
		}{Type: "avatar", AvatarID: avatar.NativeLookID, AudioAssetID: assetID, AspectRatio: controls.AspectRatio, Resolution: controls.Resolution, Title: controls.Title, MotionPrompt: controls.MotionPrompt, Expressiveness: controls.Expressiveness, Engine: struct {
			Type string `json:"type"`
		}{Type: "avatar_iv"}}
	}
	native, _ := http.NewRequestWithContext(ctx, http.MethodPost, provider.textEndpointURL, bytes.NewReader(heygenEncode(body)))
	native.Header.Set("Content-Type", "application/json")
	authorizeQueueRequest(native, provider)
	response, err := request.HTTP.Submission.Do(native)
	if err != nil {
		return imageSubmissionFailure(err)
	}
	var receipt struct {
		Data struct {
			VideoID    string `json:"video_id"`
			AvatarItem struct {
				ID string `json:"id"`
			} `json:"avatar_item"`
		} `json:"data"`
	}
	status := response.StatusCode
	decodeError := readQueueJSON(response, &receipt)
	if status >= 400 && status < 500 && status != 408 {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
	}
	if decodeError != nil || status != 200 && status != 201 {
		return imageGenerationUncertain()
	}
	handle := receipt.Data.VideoID
	if adapter.route.Operation == ModelOperationAvatarCreation {
		handle = receipt.Data.AvatarItem.ID
	}
	if !queueRequestID.MatchString(handle) {
		return imageGenerationUncertain()
	}
	if err := request.PersistProviderReceipt(MediaOperationProviderReceipt{Handle: handle, RequestID: response.Header.Get("request-id")}); err != nil {
		return imageGenerationUncertain()
	}
	request.ProviderHandle = handle
	return adapter.poll(ctx, request, provider)
}

func (adapter *providerHeyGenAvatarAdapter) Recover(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	provider, err := resolveMediaProvider(ctx, request, adapter.provider.identifier.string(), adapter.route.Transport, adapter.provider, adapter.tenants, adapter.store)
	if err != nil || request.ExecutionBinding != adapter.binding || !queueRequestID.MatchString(request.ProviderHandle) {
		return imageGenerationUncertain()
	}
	return adapter.poll(ctx, request, provider)
}

func (adapter *providerHeyGenAvatarAdapter) poll(ctx context.Context, request MediaOperationExecutionRequest, provider providerDefinition) MediaOperationExecutionResult {
	endpoint, _ := url.Parse(provider.textEndpointURL)
	endpoint.Path = "/v3/videos/" + request.ProviderHandle
	if adapter.route.Operation == ModelOperationAvatarCreation {
		endpoint.Path = "/v3/avatars/looks/" + request.ProviderHandle
	}
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	for {
		native, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		authorizeQueueRequest(native, provider)
		response, err := request.HTTP.Status.Do(native)
		if err != nil {
			return imageGenerationUncertain()
		}
		var result struct {
			Data struct {
				ID         string `json:"id"`
				Status     string `json:"status"`
				AvatarType string `json:"avatar_type"`
				VideoURL   string `json:"video_url"`
			} `json:"data"`
		}
		status := response.StatusCode
		if readQueueJSON(response, &result) != nil || status != 200 || result.Data.ID != request.ProviderHandle {
			return imageGenerationUncertain()
		}
		switch result.Data.Status {
		case "completed":
			if adapter.route.Operation == ModelOperationAvatarCreation {
				if result.Data.AvatarType != "photo_avatar" {
					return imageGenerationUncertain()
				}
				var input llmproxycontract.AvatarCreationSource
				_ = json.Unmarshal(request.Input, &input)
				public := llmproxycontract.MediaAvatar{AvatarID: "ava_" + strings.TrimPrefix(request.OperationID, "mop_"), Provider: request.Provider, Name: input.Name}
				metadata := heygenEncode(public)
				return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: []MediaOperationOutput{{MIMEType: "application/json", Data: metadata}}, avatar: &mediaAvatarRecord{AvatarID: public.AvatarID, TenantID: request.TenantID, Provider: request.Provider, CredentialReference: request.CredentialReference, NativeLookID: request.ProviderHandle, Metadata: metadata}}
			}
			output, err := heygenDownloadOutput(ctx, request.HTTP.Transfer, adapter.provider.transports[adapter.route.Transport].artifactOrigins, result.Data.VideoURL, min(adapter.limits[heygenOutputBytes], adapter.assets.maxAssetBytes), heygenOutputVideo)
			if err != nil {
				return imageGenerationUncertain()
			}
			return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: []MediaOperationOutput{output}}
		case "failed":
			return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
		case "processing", "pending", "pending_consent":
		default:
			return imageGenerationUncertain()
		}
		timer := time.NewTimer(queueImagePollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return imageGenerationUncertain()
		case <-timer.C:
		}
	}
}

func (*providerHeyGenAvatarAdapter) Cancel(context.Context, MediaOperationExecutionRequest) MediaOperationCancellationResult {
	return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
}
func (adapter *providerHeyGenAvatarAdapter) canRecoverAfterShutdown(binding, handle string) bool {
	return binding == adapter.binding && queueRequestID.MatchString(handle)
}

func validateHeyGenAvatarService(service ProviderCatalogService, transport ProviderCatalogTransport, field string) error {
	expectedPath := "/v3/avatars"
	expectedLimits := []string{heygenAvatarImageBytes, heygenOutputBytes}
	expectedControls := map[string][]string{}
	if service.Operation == ModelOperationAvatarVideoGeneration {
		expectedPath = "/v3/videos"
		expectedLimits = []string{heygenInputAudioBytes, heygenOutputBytes}
		expectedControls = map[string][]string{"engine": {"avatar_iv"}, "aspect_ratio": {"16:9", "9:16", "1:1"}, "resolution": {"720p", "1080p"}, "expressiveness": {"low", "medium", "high"}}
	}
	if transport.Endpoint.Path != expectedPath || len(service.Limits) != len(expectedLimits) || len(service.Controls) != len(expectedControls) {
		return fmt.Errorf("%w: field=%s reason=heygen_avatar_composition", ErrInvalidModelCatalog, field)
	}
	seen := map[string]bool{}
	for _, limit := range service.Limits {
		if !slices.Contains(expectedLimits, limit.ID) || seen[limit.ID] || limit.Value == nil || *limit.Value <= 0 || limit.Unit != "bytes" || limit.AccountDependent || limit.ID != heygenOutputBytes && int64(*limit.Value) > heygenMaximumInputBytes {
			return fmt.Errorf("%w: field=%s reason=heygen_avatar_limits", ErrInvalidModelCatalog, field)
		}
		seen[limit.ID] = true
	}
	for _, control := range service.Controls {
		values, ok := expectedControls[control.ID]
		if !ok || control.Kind != CatalogControlEnum || control.AccountDependent || !slices.Equal(values, control.Values) {
			return fmt.Errorf("%w: field=%s reason=heygen_avatar_controls", ErrInvalidModelCatalog, field)
		}
		delete(expectedControls, control.ID)
	}
	return nil
}
