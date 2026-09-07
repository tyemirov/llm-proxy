package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

const geminiTranscriptionInlineRequestBytes = 20_000_000

type geminiTranscriptionRequest struct {
	Model      string                     `json:"model"`
	Input      []geminiInteractionContent `json:"input"`
	Background bool                       `json:"background"`
	Store      bool                       `json:"store"`
}

var geminiTranscriptionMIMETypes = map[string]string{
	".wav": "audio/wav", ".mp3": "audio/mp3", ".aiff": "audio/aiff", ".aif": "audio/aiff",
	".aac": "audio/aac", ".ogg": "audio/ogg", ".flac": "audio/flac", ".mpeg": "audio/mpeg",
	".m4a": "audio/m4a", ".l16": "audio/l16", ".opus": "audio/opus", ".alaw": "audio/alaw",
	".mulaw": "audio/mulaw", ".webm": "audio/webm",
}

func (client *geminiInteractionsClient) transcribeAudio(ctx context.Context, apiKey, baseURL, model, filename string, audio io.Reader, logger *zap.SugaredLogger) (text string, requestError error) {
	mimeType, supported := geminiTranscriptionMIMETypes[strings.ToLower(filepath.Ext(filename))]
	if !supported {
		return "", fmt.Errorf("%w: unsupported Gemini audio file extension", ErrInvalidAudioInput)
	}
	data, readError := io.ReadAll(audio)
	if readError != nil {
		return "", fmt.Errorf("read Gemini transcription audio: %w", readError)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("%w: empty Gemini audio file", ErrInvalidAudioInput)
	}
	payload := geminiTranscriptionRequest{Model: model, Input: []geminiInteractionContent{{Type: string(messageMediaTypeAudio), MIMEType: mimeType}}}
	// Include the JSON envelope in the inline request limit.
	emptyPayload, _ := json.Marshal(payload)
	if len(emptyPayload)+len(`,"data":""`)+base64.StdEncoding.EncodedLen(len(data)) <= geminiTranscriptionInlineRequestBytes {
		payload.Input[0].Data = base64.StdEncoding.EncodeToString(data)
	} else {
		digest := sha256.Sum256(data)
		attachment := &messageMedia{mediaType: messageMediaTypeAudio, mimeType: mimeType, data: data, sizeBytes: int64(len(data)), contentSHA256: hex.EncodeToString(digest[:])}
		file, uploadError := client.uploadGeminiFile(ctx, apiKey, baseURL, attachment)
		if file.name != "" {
			defer func() {
				if cleanupError := client.releaseGeminiFiles(ctx, apiKey, baseURL, []geminiUploadedFile{file}); cleanupError != nil {
					text = ""
					requestError = errors.Join(requestError, cleanupError)
				}
			}()
		}
		if uploadError != nil {
			return "", uploadError
		}
		payload.Input[0].URI = file.uri
	}
	response, performError := client.performInteractionRequest(ctx, http.MethodPost, geminiInteractionsURL(baseURL), apiKey, payload, logger)
	if performError != nil {
		return "", performError
	}
	snapshot, parseError := newGeminiInteractionSnapshot(response)
	if parseError != nil {
		return "", fmt.Errorf("%w: invalid Gemini transcription response", ErrProviderAPI)
	}
	generation, resolveError := snapshot.resolve()
	if resolveError != nil {
		return "", resolveError
	}
	return generation.text, nil
}
