package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
)

type failedGeminiAudioReader struct{ cause error }

func (reader failedGeminiAudioReader) Read([]byte) (int, error) { return 0, reader.cause }

func TestGeminiTranscriptionAudioReadFailure(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	cause := errors.New("audio storage read failed")
	text, err := newGeminiInteractionsClient(http.DefaultClient).transcribeAudio(context.Background(), "test-key", upstream.URL, "gemini-3.5-transcribe", "speech.wav", failedGeminiAudioReader{cause: cause}, zap.NewNop().Sugar())
	if text != "" || !errors.Is(err, cause) || calls.Load() != 0 {
		t.Fatalf("read failure: text=%q error=%v calls=%d", text, err, calls.Load())
	}
}
