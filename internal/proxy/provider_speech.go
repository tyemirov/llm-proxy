package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// CatalogProtocolElevenLabsConversion selects the native voice conversion protocol.
const CatalogProtocolElevenLabsConversion = "elevenlabs_conversion"

const (
	speechControlInputFormat  = "input_format"
	speechControlOutputFormat = "output_format"
	speechControlStability    = "stability"
	speechControlSimilarity   = "similarity_boost"
	speechControlStyle        = "style"
	speechControlSpeakerBoost = "use_speaker_boost"
	speechControlSeed         = "seed"
	speechControlNoise        = "remove_background_noise"
	speechRawInputFormat      = "pcm_s16le_16"
)

type providerSpeechInput struct {
	VoiceID      string `json:"voice_id"`
	AudioAssetID string `json:"audio_asset_id"`
}

type providerVoiceSettings struct {
	Stability       *float64 `json:"stability,omitempty"`
	SimilarityBoost *float64 `json:"similarity_boost,omitempty"`
	Style           *float64 `json:"style,omitempty"`
	UseSpeakerBoost *bool    `json:"use_speaker_boost,omitempty"`
}

type providerConversionControls struct {
	providerVoiceSettings
	InputFormat           string  `json:"input_format"`
	OutputFormat          string  `json:"output_format"`
	Seed                  *uint64 `json:"seed,omitempty"`
	RemoveBackgroundNoise *bool   `json:"remove_background_noise,omitempty"`
}

type providerSpeechReceipt struct {
	RequestID     string `json:"request_id"`
	HistoryItemID string `json:"history_item_id"`
}

type providerSpeechAdapter struct {
	offering ProviderOffering
	provider providerDefinition
	tenants  *managedTenantStore
	store    *mediaOperationStore
	assets   *tenantAssetStore
	controls map[string]CatalogControl
	binding  string
	revision string
}

type providerConversionAdapter struct{ *providerSpeechAdapter }

func newProviderConversionAdapter(offering ProviderOffering, provider providerDefinition, tenants *managedTenantStore, store *mediaOperationStore, assets *tenantAssetStore, revision string) *providerConversionAdapter {
	return &providerConversionAdapter{newProviderSpeechAdapter(offering, provider, tenants, store, assets, revision, ModelOperationSpeechConversion, CatalogProtocolElevenLabsConversion)}
}

func newProviderSpeechAdapter(offering ProviderOffering, provider providerDefinition, tenants *managedTenantStore, store *mediaOperationStore, assets *tenantAssetStore, revision, operation, codec string) *providerSpeechAdapter {
	controls := map[string]CatalogControl{}
	for _, control := range offering.Controls {
		controls[control.ID] = control
	}
	route := ProviderCatalogService{Operation: operation, Transport: offering.Transport, Controls: offering.Controls, Limits: offering.Limits}
	binding := providerServiceBinding(route, provider, codec) + ":" + offering.ProviderModel + ":" + provider.transports[offering.Transport].protocolParameters.SpeechRate
	return &providerSpeechAdapter{offering: offering, provider: provider, tenants: tenants, store: store, assets: assets, controls: controls, binding: binding, revision: revision}
}

func (adapter *providerConversionAdapter) Validate(ctx context.Context, request MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	var input providerSpeechInput
	var controls providerConversionControls
	if !decodeImageRequestObject(request.Input, &input) || !decodeImageRequestObject(request.Controls, &controls) || !slices.Contains(adapter.controls[speechControlOutputFormat].Values, controls.OutputFormat) || !slices.Contains(adapter.controls[speechControlInputFormat].Values, controls.InputFormat) {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	for id, value := range map[string]*float64{speechControlStability: controls.Stability, speechControlSimilarity: controls.SimilarityBoost, speechControlStyle: controls.Style} {
		control := adapter.controls[id]
		if value != nil && (*value < *control.Minimum || *value > *control.Maximum) {
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
	}
	if controls.Seed != nil && float64(*controls.Seed) > *adapter.controls[speechControlSeed].Maximum {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	if _, err := adapter.nativeVoice(ctx, request.TenantID, request.CredentialReference, input.VoiceID); err != nil {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	metadata, err := adapter.assets.metadata(tenant{identifier: tenantID(request.TenantID)}, input.AudioAssetID)
	if err != nil || metadata.SizeBytes > adapter.assets.maxAssetBytes {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	if controls.InputFormat == speechRawInputFormat {
		if metadata.MIMEType != "application/octet-stream" || metadata.SizeBytes%2 != 0 {
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
	} else if !slices.Contains([]string{"audio/mpeg", "audio/wav", "audio/m4a", "audio/flac", "audio/ogg"}, metadata.MIMEType) {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	inputJSON, _ := json.Marshal(input)
	controlsJSON, _ := json.Marshal(controls)
	return MediaOperationValidatedRequest{Input: inputJSON, Controls: controlsJSON, InputAssetIDs: []string{input.AudioAssetID}, ExecutionBinding: adapter.binding}, nil
}

func (adapter *providerSpeechAdapter) nativeVoice(ctx context.Context, tenantID, credential, voiceID string) (string, error) {
	voice, err := adapter.store.providerMediaVoice(ctx, tenantID, voiceID)
	if err != nil || voice.Provider != adapter.offering.Provider || voice.Authority != credential+":"+adapter.revision {
		return "", errMediaOperationInvalid
	}
	var reference elevenLabsVoiceReference
	if json.Unmarshal([]byte(voice.ProviderVoiceReference), &reference) != nil || reference.Authority != voice.Authority || reference.VoiceID == "" || reference.VoiceID == "." || reference.VoiceID == ".." {
		return "", errMediaOperationInvalid
	}
	return reference.VoiceID, nil
}

func (adapter *providerConversionAdapter) Execute(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	provider, err := resolveMediaProvider(ctx, request, adapter.offering.Provider, adapter.offering.Transport, adapter.provider, adapter.tenants, adapter.store)
	if err != nil || request.ExecutionBinding != adapter.binding {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	var input providerSpeechInput
	var controls providerConversionControls
	_ = json.Unmarshal(request.Input, &input)
	_ = json.Unmarshal(request.Controls, &controls)
	voice, err := adapter.nativeVoice(ctx, request.TenantID, request.CredentialReference, input.VoiceID)
	if err != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationInvalid.Error()}
	}
	owner := tenant{identifier: tenantID(request.TenantID)}
	metadata, err := adapter.assets.metadata(owner, input.AudioAssetID)
	if err != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationInvalid.Error()}
	}
	asset, err := adapter.assets.resolve(owner, input.AudioAssetID, metadata.MIMEType)
	if err != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationInvalid.Error()}
	}
	defer asset.Close()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("model_id", adapter.offering.ProviderModel)
	_ = form.WriteField("file_format", controls.InputFormat)
	settings, _ := json.Marshal(controls.providerVoiceSettings)
	if string(settings) != "{}" {
		_ = form.WriteField("voice_settings", string(settings))
	}
	if controls.Seed != nil {
		_ = form.WriteField(speechControlSeed, strconv.FormatUint(*controls.Seed, 10))
	}
	if controls.RemoveBackgroundNoise != nil {
		_ = form.WriteField(speechControlNoise, strconv.FormatBool(*controls.RemoveBackgroundNoise))
	}
	headers := textproto.MIMEHeader{}
	headers.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "audio", "filename": input.AudioAssetID}))
	headers.Set("Content-Type", metadata.MIMEType)
	_, _ = form.CreatePart(headers)
	headerBytes := body.Len()
	_ = form.Close()
	reader := io.MultiReader(bytes.NewReader(body.Bytes()[:headerBytes]), asset.file, bytes.NewReader(body.Bytes()[headerBytes:]))
	target, _ := url.Parse(provider.textEndpointURL)
	target.RawPath = strings.TrimRight(target.EscapedPath(), "/") + "/" + url.PathEscape(voice)
	target.Path, _ = url.PathUnescape(target.RawPath)
	query := target.Query()
	query.Set(speechControlOutputFormat, controls.OutputFormat)
	target.RawQuery = query.Encode()
	native, _ := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), reader)
	native.ContentLength = int64(body.Len()) + metadata.SizeBytes
	native.Header.Set("Content-Type", form.FormDataContentType())
	authorizeQueueRequest(native, provider)
	response, err := request.HTTP.Submission.Do(native)
	if err != nil {
		return imageSubmissionFailure(err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != http.StatusRequestTimeout {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
	}
	if response.StatusCode != http.StatusOK {
		return imageGenerationUncertain()
	}
	receipt, _ := json.Marshal(providerSpeechReceipt{RequestID: response.Header.Get("request-id"), HistoryItemID: response.Header.Get("history-item-id")})
	if err := request.PersistProviderHandle(string(receipt)); err != nil {
		return imageGenerationUncertain()
	}
	contentType, _, contentTypeError := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if contentTypeError != nil || (!strings.HasPrefix(contentType, "audio/") && contentType != "application/octet-stream") {
		return imageGenerationUncertain()
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, adapter.assets.maxAssetBytes+1))
	if err != nil || len(data) == 0 || int64(len(data)) > adapter.assets.maxAssetBytes {
		return imageGenerationUncertain()
	}
	description := providerSpeechFormats[controls.OutputFormat]
	if description.RawAudio != nil && description.RawAudio.Encoding == "s16le" && len(data)%2 != 0 {
		return imageGenerationUncertain()
	}
	description.OutputFormat = controls.OutputFormat
	encoded, _ := json.Marshal(description)
	return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: []MediaOperationOutput{{MIMEType: description.MIMEType, Data: data}, {MIMEType: "application/json", Data: encoded}}}
}

func (*providerConversionAdapter) Recover(context.Context, MediaOperationExecutionRequest) MediaOperationExecutionResult {
	return imageGenerationUncertain()
}

func (*providerConversionAdapter) Cancel(context.Context, MediaOperationExecutionRequest) MediaOperationCancellationResult {
	return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
}
