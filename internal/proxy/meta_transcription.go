package proxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"go.uber.org/zap"
)

// This gateway bound includes the multipart envelope. Activation also requires
// provider confirmation of the documented 32 MB unit.
const metaTranscriptionRequestBytes = 32_000_000

func (client *OpenAIClient) transcribeMetaAudio(ctx context.Context, endpoint, model string, audio io.Reader, logger *zap.SugaredLogger) (string, error) {
	data, readError := io.ReadAll(io.LimitReader(audio, metaTranscriptionRequestBytes+1))
	if readError != nil {
		return "", fmt.Errorf("read Meta transcription audio: %w", readError)
	}
	if len(data) > metaTranscriptionRequestBytes {
		return "", fmt.Errorf("%w: Meta transcription request exceeds 32000000 bytes", ErrInvalidAudioInput)
	}
	if validationError := validateMetaTranscriptionWAV(data); validationError != nil {
		return "", validationError
	}
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	settings, _ := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="request"`},
		"Content-Type":        {mimeApplicationJSON},
	})
	_ = json.NewEncoder(settings).Encode(struct {
		Model         string `json:"model"`
		AudioEncoding string `json:"audioEncoding"`
		Mode          string `json:"mode"`
	}{Model: model, AudioEncoding: "WAV", Mode: "PUSH_TO_TALK"})
	part, _ := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="audio"; filename="audio.wav"`},
		"Content-Type":        {"audio/wav"},
	})
	_, _ = part.Write(data)
	_ = writer.Close()
	if payload.Len() > metaTranscriptionRequestBytes {
		return "", fmt.Errorf("%w: Meta transcription multipart request exceeds 32000000 bytes", ErrInvalidAudioInput)
	}
	request, buildError := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &payload)
	if buildError != nil {
		return "", fmt.Errorf("build Meta transcription request: %w", buildError)
	}
	request.Header.Set(headerContentType, writer.FormDataContentType())
	request.Header.Set(headerAccept, mimeApplicationJSON)
	status, body, headers, _, requestError := client.performTranscriptionsRequest(request, logger)
	if status == http.StatusRequestEntityTooLarge {
		return "", newProviderMediaLimitHTTPError(status, headers)
	}
	if responseError := providerResponseError(status, headers, requestError); responseError != nil {
		return "", responseError
	}
	var response struct {
		Transcript string `json:"transcript"`
	}
	if err := json.Unmarshal(body, &response); err != nil || strings.TrimSpace(response.Transcript) == "" {
		return "", fmt.Errorf("%w: invalid Meta transcription response", ErrProviderAPI)
	}
	return strings.TrimSpace(response.Transcript), nil
}

func validateMetaTranscriptionWAV(data []byte) error {
	invalid := fmt.Errorf("%w: Meta requires a nonempty mono 16-bit PCM WAV at 16000 or 24000 Hz, at most 600 seconds", ErrInvalidAudioInput)
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" || uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data)) {
		return invalid
	}
	var sampleRate, audioBytes uint32
	var hasFormat, hasAudio bool
	for offset := 12; offset < len(data); {
		if len(data)-offset < 8 {
			return invalid
		}
		size := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		end := uint64(offset) + 8 + size
		paddedEnd := end + size%2
		if paddedEnd > uint64(len(data)) {
			return invalid
		}
		chunk := data[offset+8 : end]
		switch string(data[offset : offset+4]) {
		case "fmt ":
			if hasFormat || size < 16 {
				return invalid
			}
			hasFormat = true
			sampleRate = binary.LittleEndian.Uint32(chunk[4:8])
			if binary.LittleEndian.Uint16(chunk[:2]) != 1 || binary.LittleEndian.Uint16(chunk[2:4]) != 1 ||
				(sampleRate != 16000 && sampleRate != 24000) || binary.LittleEndian.Uint32(chunk[8:12]) != sampleRate*2 ||
				binary.LittleEndian.Uint16(chunk[12:14]) != 2 || binary.LittleEndian.Uint16(chunk[14:16]) != 16 {
				return invalid
			}
		case "data":
			if hasAudio || size == 0 || size%2 != 0 {
				return invalid
			}
			hasAudio = true
			audioBytes = uint32(size)
		}
		offset = int(paddedEnd)
	}
	if !hasFormat || !hasAudio || audioBytes > sampleRate*2*600 {
		return invalid
	}
	return nil
}
