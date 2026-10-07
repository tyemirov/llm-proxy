package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

type heygenOperationHandle struct {
	IDs            []string `json:"ids"`
	FailedOrdinals []int    `json:"failed_ordinals"`
}
type heygenAdapter struct {
	route    ProviderCatalogService
	provider providerDefinition
	tenants  *managedTenantStore
	store    *mediaOperationStore
	assets   *tenantAssetStore
	binding  string
	limits   map[string]int64
}

func newHeyGenAdapter(route ProviderCatalogService, provider providerDefinition, tenants *managedTenantStore, store *mediaOperationStore, assets *tenantAssetStore) *heygenAdapter {
	limits := map[string]int64{}
	for _, limit := range route.Limits {
		limits[limit.ID] = int64(*limit.Value)
	}
	return &heygenAdapter{route: route, provider: provider, tenants: tenants, store: store, assets: assets, binding: providerServiceBinding(route, provider, "heygen_v3:1"), limits: limits}
}

func validHeyGenProcessingControls(controls llmproxycontract.VideoProcessingControls) bool {
	if !slices.Contains([]string{"speed", "precision"}, controls.Mode) || utf8.RuneCountInString(controls.Title) > 1000 || controls.FPSMode != "" && !slices.Contains([]string{"vfr", "cfr", "passthrough"}, controls.FPSMode) {
		return false
	}
	for _, value := range []*float64{controls.StartTime, controls.EndTime} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 36000) {
			return false
		}
	}
	return controls.EndTime == nil || controls.StartTime == nil || *controls.EndTime > *controls.StartTime
}

func (adapter *heygenAdapter) Validate(_ context.Context, request MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	var source llmproxycontract.VideoTranslationSource
	var controls llmproxycontract.VideoTranslationControls
	if adapter.route.Operation == ModelOperationVideoLipSync {
		var lipSource llmproxycontract.VideoLipSyncSource
		var lipControls llmproxycontract.VideoLipSyncControls
		if !decodeImageRequestObject(request.Input, &lipSource) || !decodeImageRequestObject(request.Controls, &lipControls) || lipSource.AudioAssetID == "" {
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
		source = llmproxycontract.VideoTranslationSource(lipSource)
		controls.VideoProcessingControls = lipControls.VideoProcessingControls
	} else {
		if !decodeImageRequestObject(request.Input, &source) || !decodeImageRequestObject(request.Controls, &controls) || len(controls.OutputLanguages) == 0 || len(controls.OutputLanguages) > 20 {
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
		seen := map[string]bool{}
		for _, language := range controls.OutputLanguages {
			if strings.TrimSpace(language) != language || language == "" || utf8.RuneCountInString(language) > 100 || seen[language] {
				return MediaOperationValidatedRequest{}, errMediaOperationInvalid
			}
			seen[language] = true
		}
		if controls.InputLanguage != strings.TrimSpace(controls.InputLanguage) || utf8.RuneCountInString(controls.InputLanguage) > 100 || controls.SpeakerNum != nil && (*controls.SpeakerNum < 0 || *controls.SpeakerNum > 100) || controls.FPSMode != "" && source.AudioAssetID == "" {
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
	}
	if !validHeyGenProcessingControls(controls.VideoProcessingControls) || !heygenAssetAllowed(adapter.assets, request.TenantID, source.VideoAssetID, "video/", adapter.limits[heygenInputVideoBytes]) {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	ids := []string{source.VideoAssetID}
	if source.AudioAssetID != "" {
		if !heygenAssetAllowed(adapter.assets, request.TenantID, source.AudioAssetID, "audio/", adapter.limits[heygenInputAudioBytes]) {
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
		ids = append(ids, source.AudioAssetID)
	}
	encodedControls := heygenEncode(controls)
	if adapter.route.Operation == ModelOperationVideoLipSync {
		encodedControls = heygenEncode(llmproxycontract.VideoLipSyncControls{VideoProcessingControls: controls.VideoProcessingControls})
	}
	return MediaOperationValidatedRequest{Input: heygenEncode(source), Controls: encodedControls, InputAssetIDs: ids, ExecutionBinding: adapter.binding}, nil
}

func (adapter *heygenAdapter) resolve(ctx context.Context, request MediaOperationExecutionRequest) (providerDefinition, error) {
	return resolveMediaProvider(ctx, request, adapter.provider.identifier.string(), adapter.route.Transport, adapter.provider, adapter.tenants, adapter.store)
}

func (adapter *heygenAdapter) Execute(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	provider, err := adapter.resolve(ctx, request)
	if err != nil || request.ExecutionBinding != adapter.binding {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	var source llmproxycontract.VideoTranslationSource
	_ = json.Unmarshal(request.Input, &source)
	videoID, err := heygenUploadAsset(ctx, request, provider, adapter.assets, source.VideoAssetID)
	if err != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
	}
	var audio *heygenNativeAsset
	if source.AudioAssetID != "" {
		audioID, err := heygenUploadAsset(ctx, request, provider, adapter.assets, source.AudioAssetID)
		if err != nil {
			return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
		}
		audio = &heygenNativeAsset{Type: "asset_id", AssetID: audioID}
	}
	var body []byte
	count := 1
	if adapter.route.Operation == ModelOperationVideoLipSync {
		var controls llmproxycontract.VideoLipSyncControls
		_ = json.Unmarshal(request.Controls, &controls)
		body = heygenEncode(struct {
			Video heygenNativeAsset  `json:"video"`
			Audio *heygenNativeAsset `json:"audio"`
			llmproxycontract.VideoLipSyncControls
		}{heygenNativeAsset{Type: "asset_id", AssetID: videoID}, audio, controls})
	} else {
		var controls llmproxycontract.VideoTranslationControls
		_ = json.Unmarshal(request.Controls, &controls)
		count = len(controls.OutputLanguages)
		body = heygenEncode(struct {
			Video heygenNativeAsset  `json:"video"`
			Audio *heygenNativeAsset `json:"audio,omitempty"`
			llmproxycontract.VideoTranslationControls
		}{heygenNativeAsset{Type: "asset_id", AssetID: videoID}, audio, controls})
	}
	native, _ := http.NewRequestWithContext(ctx, http.MethodPost, provider.textEndpointURL, bytes.NewReader(body))
	native.Header.Set("Content-Type", "application/json")
	authorizeQueueRequest(native, provider)
	response, err := request.HTTP.Submission.Do(native)
	if err != nil {
		return imageSubmissionFailure(err)
	}
	var accepted struct {
		Data struct {
			LipSyncID      string   `json:"lipsync_id"`
			TranslationIDs []string `json:"video_translation_ids"`
		} `json:"data"`
	}
	decodeErr := readQueueJSON(response, &accepted)
	if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != http.StatusRequestTimeout {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
	}
	if decodeErr != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return imageGenerationUncertain()
	}
	handle := heygenOperationHandle{IDs: accepted.Data.TranslationIDs, FailedOrdinals: []int{}}
	if adapter.route.Operation == ModelOperationVideoLipSync {
		handle.IDs = []string{accepted.Data.LipSyncID}
	}
	if !validHeyGenHandle(handle, count) {
		return imageGenerationUncertain()
	}
	request.ProviderHandle = string(heygenEncode(handle))
	if err := request.PersistProviderReceipt(MediaOperationProviderReceipt{Handle: request.ProviderHandle, RequestID: handle.IDs[0]}); err != nil {
		return imageGenerationUncertain()
	}
	return adapter.poll(ctx, request, provider, handle)
}

func validHeyGenHandle(handle heygenOperationHandle, count int) bool {
	if len(handle.IDs) != count || handle.FailedOrdinals == nil {
		return false
	}
	failed := map[int]bool{}
	for _, ordinal := range handle.FailedOrdinals {
		if ordinal < 0 || ordinal >= count || failed[ordinal] {
			return false
		}
		failed[ordinal] = true
	}
	seen := map[string]bool{}
	for _, id := range handle.IDs {
		if !queueRequestID.MatchString(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func (adapter *heygenAdapter) retained(ctx context.Context, request MediaOperationExecutionRequest) (providerDefinition, heygenOperationHandle, bool) {
	provider, err := adapter.resolve(ctx, request)
	var handle heygenOperationHandle
	count := 1
	if adapter.route.Operation == ModelOperationVideoTranslation {
		var controls llmproxycontract.VideoTranslationControls
		_ = json.Unmarshal(request.Controls, &controls)
		count = len(controls.OutputLanguages)
	}
	valid := err == nil && request.ExecutionBinding == adapter.binding && decodeImageRequestObject([]byte(request.ProviderHandle), &handle) && validHeyGenHandle(handle, count)
	return provider, handle, valid
}
func (adapter *heygenAdapter) Recover(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	provider, handle, valid := adapter.retained(ctx, request)
	if !valid {
		return imageGenerationUncertain()
	}
	return adapter.poll(ctx, request, provider, handle)
}
func (adapter *heygenAdapter) canRecoverAfterShutdown(binding, handle string) bool {
	var receipt heygenOperationHandle
	return binding == adapter.binding && decodeImageRequestObject([]byte(handle), &receipt) && len(receipt.IDs) > 0 && validHeyGenHandle(receipt, len(receipt.IDs))
}
func (*heygenAdapter) Cancel(context.Context, MediaOperationExecutionRequest) MediaOperationCancellationResult {
	return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
}

func (adapter *heygenAdapter) poll(ctx context.Context, request MediaOperationExecutionRequest, provider providerDefinition, handle heygenOperationHandle) MediaOperationExecutionResult {
	outputs := make([]MediaOperationOutput, len(handle.IDs))
	terminal := make([]bool, len(handle.IDs))
	var controls llmproxycontract.VideoTranslationControls
	_ = json.Unmarshal(request.Controls, &controls)
	failureObserved := len(handle.FailedOrdinals) > 0
	for _, ordinal := range handle.FailedOrdinals {
		terminal[ordinal] = true
	}
	var partials []mediaOperationPartialReferenceRecord
	if err := adapter.store.database.WithContext(ctx).Where("operation_id = ? AND tenant_id = ?", request.OperationID, request.TenantID).Find(&partials).Error; err != nil {
		return imageGenerationUncertain()
	}
	for _, partial := range partials {
		if partial.OutputOrdinal < 0 || partial.OutputOrdinal >= len(outputs) || partial.PartialOrdinal != 0 {
			return imageGenerationUncertain()
		}
		kind := heygenOutputVideo
		if controls.TranslateAudioOnly != nil && *controls.TranslateAudioOnly {
			kind = heygenOutputAudio
		}
		if !strings.HasPrefix(partial.MIMEType, string(kind)+"/") || partial.SizeBytes > min(adapter.limits[heygenOutputBytes], adapter.assets.maxAssetBytes) {
			return imageGenerationUncertain()
		}
		asset, err := adapter.assets.resolve(tenant{identifier: tenantID(request.TenantID)}, partial.AssetID, partial.MIMEType)
		if err != nil {
			return imageGenerationUncertain()
		}
		data, err := io.ReadAll(io.LimitReader(asset.file, partial.SizeBytes+1))
		closeError := asset.Close()
		if err != nil || closeError != nil || int64(len(data)) != partial.SizeBytes {
			return imageGenerationUncertain()
		}
		outputs[partial.OutputOrdinal] = MediaOperationOutput{MIMEType: partial.MIMEType, Data: data}
		terminal[partial.OutputOrdinal] = true
	}
	for {
		pending, uncertain := false, false
		for ordinal, id := range handle.IDs {
			if terminal[ordinal] {
				continue
			}
			native, _ := http.NewRequestWithContext(ctx, http.MethodGet, provider.textEndpointURL+"/"+id, nil)
			authorizeQueueRequest(native, provider)
			response, err := request.HTTP.Status.Do(native)
			if err != nil {
				uncertain = true
				continue
			}
			var observation struct {
				Data struct {
					ID             string `json:"id"`
					Status         string `json:"status"`
					VideoURL       string `json:"video_url"`
					AudioURL       string `json:"audio_url"`
					OutputLanguage string `json:"output_language"`
				} `json:"data"`
			}
			if readQueueJSON(response, &observation) != nil || response.StatusCode != http.StatusOK || observation.Data.ID != id {
				uncertain = true
				continue
			}
			switch observation.Data.Status {
			case "failed":
				handle.FailedOrdinals = append(handle.FailedOrdinals, ordinal)
				request.ProviderHandle = string(heygenEncode(handle))
				if err := request.PersistProviderReceipt(MediaOperationProviderReceipt{Handle: request.ProviderHandle, RequestID: handle.IDs[0]}); err != nil {
					return imageGenerationUncertain()
				}
				terminal[ordinal] = true
				failureObserved = true
			case "completed":
				if adapter.route.Operation == ModelOperationVideoTranslation && observation.Data.OutputLanguage != controls.OutputLanguages[ordinal] {
					uncertain = true
					continue
				}
				kind := heygenOutputVideo
				endpoint := observation.Data.VideoURL
				if controls.TranslateAudioOnly != nil && *controls.TranslateAudioOnly {
					endpoint = observation.Data.AudioURL
					kind = heygenOutputAudio
				}
				output, err := heygenDownloadOutput(ctx, request.HTTP.Transfer, provider.activeTransport.artifactOrigins, endpoint, min(adapter.limits[heygenOutputBytes], adapter.assets.maxAssetBytes), kind)
				if err != nil {
					uncertain = true
					continue
				}
				if adapter.route.Operation == ModelOperationVideoTranslation && len(handle.IDs) > 1 {
					if err := request.PublishPartial(MediaOperationPartialOutput{MediaOperationOutput: output, OutputOrdinal: ordinal, PartialOrdinal: 0}); err != nil {
						return imageGenerationUncertain()
					}
				}
				outputs[ordinal] = output
				terminal[ordinal] = true
			case "pending", "running":
				pending = true
			default:
				uncertain = true
			}
		}
		if uncertain {
			return imageGenerationUncertain()
		}
		if !pending {
			if failureObserved {
				return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
			}
			return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, ProviderHandle: request.ProviderHandle, Outputs: outputs}
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
