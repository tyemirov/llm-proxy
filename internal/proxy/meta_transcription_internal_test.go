package proxy

import (
	"context"
	"encoding/binary"
	"errors"
	"net/http"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestMetaTranscriptionAudioReadFailure(t *testing.T) {
	cause := errors.New("audio storage read failed")
	text, err := NewOpenAIClient(http.DefaultClient, &Endpoints{}).transcribeMetaAudio(context.Background(), "https://example.invalid", "muse-voice-transcribe-1.0", failedGeminiAudioReader{cause: cause}, zap.NewNop().Sugar())
	if text != "" || !errors.Is(err, cause) {
		t.Fatalf("text=%q error=%v", text, err)
	}
}

func TestMetaTranscriptionInvalidEndpoint(t *testing.T) {
	data := []byte("RIFF\x00\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x80\x3e\x00\x00\x00\x7d\x00\x00\x02\x00\x10\x00data\x02\x00\x00\x00\x00\x00")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	text, err := NewOpenAIClient(http.DefaultClient, &Endpoints{}).transcribeMetaAudio(context.Background(), "://invalid", "muse-voice-transcribe-1.0", strings.NewReader(string(data)), zap.NewNop().Sugar())
	if text != "" || err == nil || !strings.Contains(err.Error(), "build Meta transcription request") {
		t.Fatalf("text=%q error=%v", text, err)
	}
}
