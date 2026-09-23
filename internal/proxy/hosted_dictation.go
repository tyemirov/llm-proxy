package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

func (service *hostedTextRequests) executeDictation(ctx context.Context, router *providerRouter, request dictationRequestParameters, identity hostedTextIdentity, logger *zap.SugaredLogger) (completionResult, error) {
	audio, err := io.ReadAll(request.audioReader)
	if err != nil {
		return completionResult{}, fmt.Errorf("read hosted dictation input: %w", err)
	}
	if len(audio) == 0 {
		return completionResult{}, fmt.Errorf("%w: empty audio", ErrInvalidAudioInput)
	}
	digest := sha256.Sum256(audio)
	canonical, err := json.Marshal(struct {
		SHA256    string `json:"sha256"`
		Size      int    `json:"size"`
		Extension string `json:"extension"`
	}{hex.EncodeToString(digest[:]), len(audio), strings.ToLower(filepath.Ext(request.fileName))})
	if err != nil {
		return completionResult{}, fmt.Errorf("encode hosted dictation intent: %w", err)
	}
	intent := hostedCompletionIntent{provider: request.provider, model: request.model, operation: ModelOperationDictation,
		kind: journalExecutionDictation, canonical: canonical, endpoint: endpointKindDictation}
	return service.executeCompletion(ctx, intent, identity, func(ctx context.Context, provider providerDefinition) (completionResult, error) {
		request.provider, request.audioReader = provider, bytes.NewReader(audio)
		text, err := router.transcribeAudio(ctx, request, logger)
		return completionResult{content: completedText(text)}, err
	}, func(text string) completionContent { return completedText(text) })
}
