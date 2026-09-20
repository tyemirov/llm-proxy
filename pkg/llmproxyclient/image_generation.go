package llmproxyclient

import (
	"context"
	"encoding/json"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

// ImageGenerationInput selects one explicit image-generation intent.
// Read the tenant's media capabilities for the selected route's accepted values.
// OutputCompression must be set for JPEG and WebP and omitted for PNG.
// Stream supports one output and at most three progressive previews.
type ImageGenerationInput struct {
	Surface             string `json:"surface"`
	ResponsesModel      string `json:"responses_model,omitempty"`
	PreviousOperationID string `json:"previous_operation_id,omitempty"`
	Provider            string `json:"provider"`
	Model               string `json:"model"`
	Prompt              string `json:"prompt"`
	Quality             string `json:"quality"`
	Size                string `json:"size"`
	Background          string `json:"background"`
	OutputFormat        string `json:"output_format"`
	OutputCompression   *int   `json:"output_compression,omitempty"`
	OutputCount         int    `json:"output_count"`
	Stream              bool   `json:"stream,omitempty"`
	PartialImages       int    `json:"partial_images,omitempty"`
}

// CreateImageGeneration accepts one image operation without waiting or retrying.
// Reuse idempotencyKey for the same intent to recover its durable operation.
func (client Client) CreateImageGeneration(ctx context.Context, idempotencyKey string, input ImageGenerationInput) (MediaOperation, error) {
	return client.CreateMediaOperation(ctx, idempotencyKey, imageOperationInput(input))
}

// ImageEditingInput selects ordered tenant images and an optional PNG mask.
// The mask applies to the first image and must have the same dimensions.
type ImageEditingInput struct {
	ImageGenerationInput
	ImageAssetIDs []string `json:"image_asset_ids"`
	MaskAssetID   string   `json:"mask_asset_id,omitempty"`
}

// CreateImageEditing accepts an image edit without waiting or retrying.
func (client Client) CreateImageEditing(ctx context.Context, idempotencyKey string, input ImageEditingInput) (MediaOperation, error) {
	request := imageOperationInput(input.ImageGenerationInput)
	request.Capability = llmproxycontract.MediaCapabilityImageEdit
	request.Input, _ = json.Marshal(struct {
		Prompt              string   `json:"prompt"`
		ImageAssetIDs       []string `json:"image_asset_ids,omitempty"`
		MaskAssetID         string   `json:"mask_asset_id,omitempty"`
		PreviousOperationID string   `json:"previous_operation_id,omitempty"`
	}{input.Prompt, input.ImageAssetIDs, input.MaskAssetID, input.PreviousOperationID})
	return client.CreateMediaOperation(ctx, idempotencyKey, request)
}

func imageOperationInput(input ImageGenerationInput) MediaOperationInput {
	prompt, _ := json.Marshal(struct {
		Prompt              string `json:"prompt"`
		PreviousOperationID string `json:"previous_operation_id,omitempty"`
	}{input.Prompt, input.PreviousOperationID})
	controls, _ := json.Marshal(struct {
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
	}{input.Surface, input.ResponsesModel, input.Quality, input.Size, input.Background, input.OutputFormat, input.OutputCompression, input.OutputCount, input.Stream, input.PartialImages})
	return MediaOperationInput{
		Capability: llmproxycontract.MediaCapabilityImageGenerate, Provider: input.Provider, Model: input.Model,
		Input: prompt, Controls: controls,
	}
}
