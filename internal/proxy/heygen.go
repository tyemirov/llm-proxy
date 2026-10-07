package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
)

const (
	CatalogProtocolHeyGenLipSync           = "heygen_v3_lipsync"
	CatalogProtocolHeyGenTranslation       = "heygen_v3_translation"
	CatalogProtocolHeyGenAccount           = "heygen_v3_account"
	heygenMaximumInputBytes          int64 = 32 << 20
	heygenAssetsPath                       = "/v3/assets"
	heygenInputVideoBytes                  = "input_video_bytes"
	heygenInputAudioBytes                  = "input_audio_bytes"
	heygenOutputBytes                      = "output_bytes"
)

type heygenOutputKind string

const (
	heygenOutputVideo heygenOutputKind = "video"
	heygenOutputAudio heygenOutputKind = "audio"
)

type heygenNativeAsset struct {
	Type    string `json:"type"`
	AssetID string `json:"asset_id"`
}

func heygenUploadAsset(ctx context.Context, request MediaOperationExecutionRequest, provider providerDefinition, assets *tenantAssetStore, assetID string) (string, error) {
	owner := tenant{identifier: tenantID(request.TenantID)}
	metadata, err := assets.metadata(owner, assetID)
	if err != nil {
		return "", err
	}
	asset, err := assets.resolve(owner, assetID, metadata.MIMEType)
	if err != nil {
		return "", err
	}
	defer asset.Close()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	headers := textproto.MIMEHeader{}
	headers.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": assetID}))
	headers.Set("Content-Type", metadata.MIMEType)
	_, _ = form.CreatePart(headers)
	headerBytes := body.Len()
	_ = form.Close()
	endpoint, _ := url.Parse(provider.textEndpointURL)
	endpoint.Path = heygenAssetsPath
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	reader := io.MultiReader(bytes.NewReader(body.Bytes()[:headerBytes]), asset.file, bytes.NewReader(body.Bytes()[headerBytes:]))
	native, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), reader)
	native.ContentLength = int64(body.Len()) + metadata.SizeBytes
	native.Header.Set("Content-Type", form.FormDataContentType())
	authorizeQueueRequest(native, provider)
	response, err := request.HTTP.Transfer.Do(native)
	if err != nil {
		return "", err
	}
	var receipt struct {
		Data struct {
			AssetID   string `json:"asset_id"`
			MIMEType  string `json:"mime_type"`
			SizeBytes int64  `json:"size_bytes"`
			URL       string `json:"url"`
		} `json:"data"`
	}
	if err := readQueueJSON(response, &receipt); err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated || !queueRequestID.MatchString(receipt.Data.AssetID) || receipt.Data.MIMEType != metadata.MIMEType || receipt.Data.SizeBytes != metadata.SizeBytes || receipt.Data.URL == "" {
		return "", errMediaOperationInvalid
	}
	return receipt.Data.AssetID, nil
}

func heygenDownloadOutput(ctx context.Context, client HTTPDoer, origins []string, endpoint string, maximumBytes int64, kind heygenOutputKind) (MediaOperationOutput, error) {
	target, err := url.Parse(endpoint)
	if err != nil || target.User != nil || target.Fragment != "" || !slices.Contains(origins, upstreamRequestOrigin(target)) {
		return MediaOperationOutput{}, errMediaOperationInvalid
	}
	native, _ := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	response, err := client.Do(native)
	if err != nil {
		return MediaOperationOutput{}, err
	}
	defer response.Body.Close()
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || response.StatusCode != http.StatusOK || !slices.Contains([]string{"video/mp4", "video/webm", "audio/mpeg", "audio/mp4", "audio/wav"}, mediaType) || !strings.HasPrefix(mediaType, string(kind)+"/") {
		return MediaOperationOutput{}, errMediaOperationInvalid
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumBytes+1))
	if err != nil {
		return MediaOperationOutput{}, err
	}
	if len(data) == 0 || int64(len(data)) > maximumBytes || response.ContentLength >= 0 && response.ContentLength != int64(len(data)) {
		return MediaOperationOutput{}, errMediaOperationInvalid
	}
	return MediaOperationOutput{MIMEType: mediaType, Data: data}, nil
}

func validateHeyGenService(service ProviderCatalogService, transport ProviderCatalogTransport, field string) error {
	if len(service.Limits) != 3 {
		return fmt.Errorf("%w: field=%s reason=heygen_limits", ErrInvalidModelCatalog, field)
	}
	seen := map[string]bool{}
	for _, limit := range service.Limits {
		if limit.Value == nil || limit.AccountDependent || limit.Unit != "bytes" || *limit.Value <= 0 || seen[limit.ID] {
			return fmt.Errorf("%w: field=%s reason=heygen_limits", ErrInvalidModelCatalog, field)
		}
		seen[limit.ID] = true
		switch limit.ID {
		case heygenInputVideoBytes, heygenInputAudioBytes:
			if int64(*limit.Value) > heygenMaximumInputBytes {
				return fmt.Errorf("%w: field=%s reason=heygen_input_limit", ErrInvalidModelCatalog, field)
			}
		case heygenOutputBytes:
		default:
			return fmt.Errorf("%w: field=%s reason=heygen_limits", ErrInvalidModelCatalog, field)
		}
	}
	allowed := map[string]string{"mode": CatalogControlEnum, "disable_music_track": CatalogControlBoolean, "enable_dynamic_duration": CatalogControlBoolean, "enable_speech_enhancement": CatalogControlBoolean, "enable_watermark": CatalogControlBoolean, "start_time": CatalogControlNumber, "end_time": CatalogControlNumber, "keep_the_same_format": CatalogControlBoolean, "fps_mode": CatalogControlEnum}
	expectedPath := "/v3/lipsyncs"
	if service.Operation == ModelOperationVideoTranslation {
		expectedPath = "/v3/video-translations"
		allowed["translate_audio_only"] = CatalogControlBoolean
		allowed["speaker_num"] = CatalogControlInteger
	} else {
		allowed["keep_the_same_format"] = CatalogControlBoolean
		allowed["fps_mode"] = CatalogControlEnum
	}
	if transport.Endpoint.Path != expectedPath || len(service.Controls) != len(allowed) {
		return fmt.Errorf("%w: field=%s reason=heygen_service_composition", ErrInvalidModelCatalog, field)
	}
	for _, control := range service.Controls {
		if control.AccountDependent || allowed[control.ID] != control.Kind {
			return fmt.Errorf("%w: field=%s reason=heygen_controls", ErrInvalidModelCatalog, field)
		}
		if (control.ID == "start_time" || control.ID == "end_time") && (control.Minimum == nil || control.Maximum == nil || *control.Minimum != 0 || *control.Maximum != 36000) || control.ID == "speaker_num" && (control.Minimum == nil || control.Maximum == nil || *control.Minimum != 0 || *control.Maximum != 100) {
			return fmt.Errorf("%w: field=%s reason=heygen_control_bounds", ErrInvalidModelCatalog, field)
		}
		if control.ID == "mode" && !slices.Equal(control.Values, []string{"speed", "precision"}) || control.ID == "fps_mode" && !slices.Equal(control.Values, []string{"vfr", "cfr", "passthrough"}) {
			return fmt.Errorf("%w: field=%s reason=heygen_control_values", ErrInvalidModelCatalog, field)
		}
	}
	return nil
}

func heygenAssetAllowed(assets *tenantAssetStore, tenantIDValue, assetID, prefix string, maximum int64) bool {
	metadata, err := assets.metadata(tenant{identifier: tenantID(tenantIDValue)}, assetID)
	return err == nil && strings.HasPrefix(metadata.MIMEType, prefix) && metadata.SizeBytes <= maximum
}

func heygenEncode(value any) json.RawMessage { data, _ := json.Marshal(value); return data }
