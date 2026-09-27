package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	_ "golang.org/x/image/webp"
)

type imageGenerationInput struct {
	Prompt              string   `json:"prompt"`
	PreviousOperationID string   `json:"previous_operation_id,omitempty"`
	ImageAssetIDs       []string `json:"image_asset_ids,omitempty"`
	MaskAssetID         string   `json:"mask_asset_id,omitempty"`
}

type imageGenerationControls struct {
	Surface           string `json:"surface"`
	ResponsesModel    string `json:"responses_model,omitempty"`
	Quality           string `json:"quality"`
	Size              string `json:"size"`
	Background        string `json:"background"`
	OutputFormat      string `json:"output_format"`
	OutputCompression *int   `json:"output_compression,omitempty"`
	OutputCount       int    `json:"output_count"`
	Stream            bool   `json:"stream,omitempty"`
	PartialImages     int    `json:"partial_images,omitempty"`
}

// imageGenerationAdapter implements the catalog-selected synchronous Images API.
// Recovery never repeats a paid submission without a provider retrieval handle.
type imageGenerationAdapter struct {
	offering           ProviderOffering
	provider           providerDefinition
	tenants            *managedTenantStore
	store              *mediaOperationStore
	assets             *tenantAssetStore
	controls           map[string]CatalogControl
	limits             map[string]int
	maximumOutputBytes int64
	responsesModels    map[string]string
}

func newImageGenerationAdapter(offering ProviderOffering, provider providerDefinition, tenants *managedTenantStore, store *mediaOperationStore, assets *tenantAssetStore, catalog CatalogService) *imageGenerationAdapter {
	adapter := &imageGenerationAdapter{offering: offering, provider: provider, tenants: tenants, store: store, assets: assets, controls: map[string]CatalogControl{}, limits: map[string]int{}, responsesModels: map[string]string{}}
	for _, control := range offering.Controls {
		adapter.controls[control.ID] = control
	}
	for _, limit := range offering.Limits {
		adapter.limits[limit.ID] = *limit.Value
	}
	adapter.maximumOutputBytes = min(int64(adapter.limits[imageLimitOutput]), assets.maxAssetBytes)
	for _, model := range adapter.controls[imageControlResponsesModel].Values {
		textOffering, _ := catalog.ResolveOffering(offering.Provider, model)
		adapter.responsesModels[model] = textOffering.ProviderModel
	}
	return adapter
}

func (adapter *imageGenerationAdapter) Validate(ctx context.Context, request MediaOperationAdapterRequest) (MediaOperationValidatedRequest, error) {
	var input imageGenerationInput
	var controls imageGenerationControls
	if !decodeImageRequestObject(request.Input, &input) || !decodeImageRequestObject(request.Controls, &controls) || strings.TrimSpace(input.Prompt) == "" || !utf8.ValidString(input.Prompt) || utf8.RuneCountInString(input.Prompt) > adapter.limits[imageLimitPrompt] {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	if !slices.Contains(adapter.controls[imageControlSurface].Values, controls.Surface) {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	if controls.Surface == "responses" {
		if err := adapter.validateResponsesInput(ctx, request, input, controls); err != nil {
			return MediaOperationValidatedRequest{}, err
		}
	} else if controls.ResponsesModel != "" || input.PreviousOperationID != "" {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	if !slices.Contains(adapter.controls[imageControlQuality].Values, controls.Quality) || !slices.Contains(adapter.controls[imageControlBackground].Values, controls.Background) || !slices.Contains(adapter.controls[imageControlFormat].Values, controls.OutputFormat) || !adapter.controls[imageControlSize].ImageSize.acceptsSize(controls.Size) {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	count := adapter.controls[imageControlCount]
	if controls.OutputCount < int(*count.Minimum) || controls.OutputCount > int(*count.Maximum) || (controls.Background == "transparent" && controls.OutputFormat == "jpeg") {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	partials := adapter.controls[imageControlPartials]
	if controls.PartialImages < int(*partials.Minimum) || controls.PartialImages > int(*partials.Maximum) || (!controls.Stream && controls.PartialImages != 0) || (controls.Stream && controls.OutputCount > adapter.limits[imageLimitStreamOutputs]) {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	if controls.OutputFormat == "png" {
		if controls.OutputCompression != nil {
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
	} else {
		compression := adapter.controls[imageControlCompression]
		if controls.OutputCompression == nil || *controls.OutputCompression < int(*compression.Minimum) || *controls.OutputCompression > int(*compression.Maximum) {
			return MediaOperationValidatedRequest{}, errMediaOperationInvalid
		}
	}
	var inputAssetIDs []string
	if request.Capability == llmproxycontract.MediaCapabilityImageEdit {
		if len(input.ImageAssetIDs) == 0 && controls.Surface == "responses" && input.PreviousOperationID != "" {
			inputAssetIDs = nil
		} else {
			var err error
			inputAssetIDs, err = adapter.validateEditingInputs(request.TenantID, input)
			if err != nil {
				return MediaOperationValidatedRequest{}, err
			}
		}
	} else if len(input.ImageAssetIDs) != 0 || input.MaskAssetID != "" {
		return MediaOperationValidatedRequest{}, errMediaOperationInvalid
	}
	inputJSON, _ := json.Marshal(input)
	controlsJSON, _ := json.Marshal(controls)
	return MediaOperationValidatedRequest{Input: inputJSON, Controls: controlsJSON, InputAssetIDs: inputAssetIDs, ParentOperationID: input.PreviousOperationID, ExecutionBinding: adapter.responsesExecutionBinding(controls.ResponsesModel)}, nil
}

func decodeImageRequestObject(document []byte, destination any) bool {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return false
	}
	return decoder.Decode(new(any)) == io.EOF
}

func (adapter *imageGenerationAdapter) Execute(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	var controls imageGenerationControls
	_ = json.Unmarshal(request.Controls, &controls)
	if controls.Surface == "responses" {
		return adapter.executeResponses(ctx, request, controls)
	}
	transportID := adapter.offering.Transport
	if request.Capability == llmproxycontract.MediaCapabilityImageEdit {
		transportID = adapter.offering.ImageRoutes.Editing
	}
	provider, err := adapter.resolveImageProvider(ctx, request, transportID)
	if err != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	return adapter.executeImages(ctx, request, provider, controls)
}

func (adapter *imageGenerationAdapter) resolveImageProvider(ctx context.Context, request MediaOperationExecutionRequest, transportID string) (providerDefinition, error) {
	return resolveMediaProvider(ctx, request, adapter.offering.Provider, transportID, adapter.provider, adapter.tenants, adapter.store)
}

func resolveMediaProvider(ctx context.Context, request MediaOperationExecutionRequest, providerID, transportID string, definition providerDefinition, tenants *managedTenantStore, store *mediaOperationStore) (providerDefinition, error) {
	settings, _, err := mediaConnectionSettings(ctx, request.TenantID, providerID, request.CredentialReference, tenants, store)
	if err != nil {
		return providerDefinition{}, err
	}
	provider := definition
	provider.connectionValues = cloneStringMap(provider.connectionValues)
	for field, value := range settings.connectionValues {
		provider.connectionValues[field] = value
	}
	provider, _ = provider.resolvedTransport(transportID)
	return provider, nil
}

func (adapter *imageGenerationAdapter) executeImages(ctx context.Context, request MediaOperationExecutionRequest, provider providerDefinition, controls imageGenerationControls) MediaOperationExecutionResult {
	var input imageGenerationInput
	_ = json.Unmarshal(request.Input, &input)
	var partialImages *int
	if controls.Stream {
		partialImages = &controls.PartialImages
	}
	payload, _ := json.Marshal(struct {
		Model             string `json:"model"`
		Prompt            string `json:"prompt"`
		Quality           string `json:"quality"`
		Size              string `json:"size"`
		Background        string `json:"background"`
		OutputFormat      string `json:"output_format"`
		OutputCompression *int   `json:"output_compression,omitempty"`
		Count             int    `json:"n"`
		Stream            bool   `json:"stream,omitempty"`
		PartialImages     *int   `json:"partial_images,omitempty"`
	}{adapter.offering.ProviderModel, input.Prompt, controls.Quality, controls.Size, controls.Background, controls.OutputFormat, controls.OutputCompression, controls.OutputCount, controls.Stream, partialImages})
	requestURL, _ := url.Parse(provider.textEndpointURL)
	providerRequest := (&http.Request{Method: provider.activeTransport.endpoint.Method, URL: requestURL, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: int64(len(payload))}).WithContext(ctx)
	providerRequest.Header.Set("Content-Type", "application/json")
	if request.Capability == llmproxycontract.MediaCapabilityImageEdit {
		body, contentType := adapter.editingBody(request.TenantID, input, controls)
		defer body.Close()
		providerRequest.Body = body
		providerRequest.ContentLength = -1
		providerRequest.Header.Set("Content-Type", contentType)
	}
	response, err := newProviderTransportHTTPDoer(request.HTTP.Submission, provider, provider.textAPIKey).Do(providerRequest)
	if err != nil {
		return imageSubmissionFailure(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_rate_limited"}
	}
	if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != http.StatusRequestTimeout {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_error"}
	}
	if response.StatusCode != http.StatusOK {
		return imageGenerationUncertain()
	}
	if controls.Stream {
		return adapter.decodeStream(response.Body, controls, request)
	}
	return adapter.decodeOutputs(response.Body, controls, request)
}

func (adapter *imageGenerationAdapter) decodeOutputs(body io.Reader, controls imageGenerationControls, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	maximumBodyBytes := ((adapter.maximumOutputBytes+2)/3*4+1024)*int64(controls.OutputCount) + 65536
	encoded, err := io.ReadAll(io.LimitReader(body, maximumBodyBytes+1))
	if err != nil {
		return imageGenerationUncertain()
	}
	if int64(len(encoded)) > maximumBodyBytes {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_result_invalid"}
	}
	if err := request.recordImageUsage(encoded); err != nil {
		return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: llmproxycontract.ErrorCodeUsageJournalUnavailable}
	}
	var result struct {
		Data []struct {
			Base64 string `json:"b64_json"`
		} `json:"data"`
	}
	if json.Unmarshal(encoded, &result) != nil || len(result.Data) != controls.OutputCount {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_result_invalid"}
	}
	outputs := make([]MediaOperationOutput, 0, controls.OutputCount)
	for _, output := range result.Data {
		decoded, decodeError := adapter.decodeImage(output.Base64, controls)
		if decodeError != nil {
			return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: "provider_result_invalid"}
		}
		outputs = append(outputs, decoded)
	}
	return MediaOperationExecutionResult{State: MediaOperationStateSucceeded, Outputs: outputs}
}

func (adapter *imageGenerationAdapter) decodeImage(encoded string, controls imageGenerationControls) (MediaOperationOutput, error) {
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || int64(len(data)) > adapter.maximumOutputBytes {
		return MediaOperationOutput{}, errMediaOperationInvalid
	}
	configuration, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != controls.OutputFormat || !adapter.controls[imageControlSize].ImageSize.acceptsDimensions(configuration.Width, configuration.Height) || (controls.Size != "auto" && controls.Size != strconv.Itoa(configuration.Width)+"x"+strconv.Itoa(configuration.Height)) {
		return MediaOperationOutput{}, errMediaOperationInvalid
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return MediaOperationOutput{}, errMediaOperationInvalid
	}
	return MediaOperationOutput{MIMEType: "image/" + format, Data: data}, nil
}

func imageSubmissionFailure(err error) MediaOperationExecutionResult {
	if errors.Is(err, errUpstreamNotDispatched) {
		return MediaOperationExecutionResult{State: MediaOperationStateFailed, ErrorCode: errMediaOperationUnavailable.Error()}
	}
	return imageGenerationUncertain()
}

func imageGenerationUncertain() MediaOperationExecutionResult {
	return MediaOperationExecutionResult{State: MediaOperationStateUncertain, ErrorCode: "provider_outcome_unknown"}
}

func (adapter *imageGenerationAdapter) Recover(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationExecutionResult {
	var controls imageGenerationControls
	_ = json.Unmarshal(request.Controls, &controls)
	if controls.Surface != "responses" || !imageResponseHandlePattern.MatchString(request.ProviderHandle) || request.ExecutionBinding != adapter.responsesExecutionBinding(controls.ResponsesModel) {
		return imageGenerationUncertain()
	}
	provider, err := adapter.resolveImageProvider(ctx, request, adapter.offering.ImageRoutes.Responses)
	if err != nil {
		return imageGenerationUncertain()
	}
	return adapter.pollImageResponse(ctx, request, provider, request.ProviderHandle, controls)
}

func (adapter *imageGenerationAdapter) Cancel(ctx context.Context, request MediaOperationExecutionRequest) MediaOperationCancellationResult {
	var controls imageGenerationControls
	_ = json.Unmarshal(request.Controls, &controls)
	if controls.Surface != "responses" || !imageResponseHandlePattern.MatchString(request.ProviderHandle) || request.ExecutionBinding != adapter.responsesExecutionBinding(controls.ResponsesModel) {
		return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
	}
	provider, err := adapter.resolveImageProvider(ctx, request, adapter.offering.ImageRoutes.Responses)
	if err != nil {
		return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
	}
	snapshot, err := adapter.fetchImageResponse(ctx, request.HTTP.Status, provider, http.MethodPost, request.ProviderHandle, "/cancel")
	if err == nil && snapshot.Status == "cancelled" {
		return MediaOperationCancellationResult{State: MediaCancellationConfirmed}
	}
	return MediaOperationCancellationResult{State: MediaCancellationUnsupported}
}
