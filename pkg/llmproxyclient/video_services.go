package llmproxyclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

// VideoLipSyncInput selects an existing-video lip-sync operation.
type VideoLipSyncInput struct {
	Provider string
	Source   llmproxycontract.VideoLipSyncSource
	Controls llmproxycontract.VideoLipSyncControls
}

// CreateVideoLipSync accepts a durable video lip-sync intent without a native API call in the consumer.
func (client Client) CreateVideoLipSync(ctx context.Context, idempotencyKey string, input VideoLipSyncInput) (MediaOperation, error) {
	return client.createVideoService(ctx, idempotencyKey, llmproxycontract.MediaCapabilityVideoLipSync, input.Provider, input.Source, input.Controls)
}

// VideoTranslationInput selects ordered translation languages for tenant-owned media.
type VideoTranslationInput struct {
	Provider string
	Source   llmproxycontract.VideoTranslationSource
	Controls llmproxycontract.VideoTranslationControls
}

// CreateVideoTranslation accepts a durable translation intent for all requested languages.
func (client Client) CreateVideoTranslation(ctx context.Context, idempotencyKey string, input VideoTranslationInput) (MediaOperation, error) {
	return client.createVideoService(ctx, idempotencyKey, llmproxycontract.MediaCapabilityVideoTranslate, input.Provider, input.Source, input.Controls)
}

// AvatarCreationInput selects a reusable photo avatar with tenant-owned input.
type AvatarCreationInput struct {
	Provider string
	Source   llmproxycontract.AvatarCreationSource
}

// CreateAvatar accepts a durable avatar creation intent.
func (client Client) CreateAvatar(ctx context.Context, idempotencyKey string, input AvatarCreationInput) (MediaOperation, error) {
	return client.createVideoService(ctx, idempotencyKey, llmproxycontract.MediaCapabilityAvatarCreate, input.Provider, input.Source, struct{}{})
}

// AvatarVideoInput selects an audio-driven video from a gateway-owned avatar.
type AvatarVideoInput struct {
	Provider string
	Source   llmproxycontract.AvatarVideoSource
	Controls llmproxycontract.AvatarVideoControls
}

// CreateAvatarVideo accepts a durable avatar rendering intent.
func (client Client) CreateAvatarVideo(ctx context.Context, idempotencyKey string, input AvatarVideoInput) (MediaOperation, error) {
	return client.createVideoService(ctx, idempotencyKey, llmproxycontract.MediaCapabilityAvatarVideoGenerate, input.Provider, input.Source, input.Controls)
}

func (client Client) createVideoService(ctx context.Context, key, capability, provider string, source, controls any) (MediaOperation, error) {
	inputJSON, _ := json.Marshal(source)
	controlsJSON, err := json.Marshal(controls)
	if err != nil {
		return MediaOperation{}, fmt.Errorf("%w: encode video controls: %v", ErrInvalidClientRequest, err)
	}
	return client.CreateMediaOperation(ctx, key, MediaOperationInput{Capability: capability, Provider: provider, Input: inputJSON, Controls: controlsJSON})
}
