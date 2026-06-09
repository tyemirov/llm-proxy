package proxy

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

type transcriptionFailingReader struct{}

func (transcriptionFailingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func TestParseTranscriptionText(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		wantText    string
		wantFailure bool
	}{
		{
			name:     "uses text field",
			input:    `{"text":"hello from text"}`,
			wantText: "hello from text",
		},
		{
			name:     "falls back to transcript field",
			input:    `{"transcript":"hello from transcript"}`,
			wantText: "hello from transcript",
		},
		{
			name:     "falls back to output_text field",
			input:    `{"output_text":"hello from output_text"}`,
			wantText: "hello from output_text",
		},
		{
			name:        "returns error for invalid json object payload",
			input:       `{"text":`,
			wantFailure: true,
		},
		{
			name:        "returns error for json without transcript fields",
			input:       `{"status":"ok"}`,
			wantFailure: true,
		},
		{
			name:     "returns plain text payload when response is not json",
			input:    "plain transcription",
			wantText: "plain transcription",
		},
		{
			name:        "returns error for blank payload",
			input:       "   ",
			wantFailure: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			text, parseError := parseTranscriptionText([]byte(testCase.input))
			if testCase.wantFailure {
				if parseError == nil {
					subTest.Fatalf("parseError=nil want non-nil")
				}
				return
			}
			if parseError != nil {
				subTest.Fatalf("unexpected parseError: %v", parseError)
			}
			if text != testCase.wantText {
				subTest.Fatalf("text=%q want=%q", text, testCase.wantText)
			}
		})
	}
}

func TestTranscribeAudioWithURLReturnsReaderError(t *testing.T) {
	client := NewOpenAIClient(http.DefaultClient, NewEndpoints(), time.Second, time.Second)
	_, transcriptionError := client.transcribeAudioWithURL("key", "http://example.test", keyModel, DefaultDictationModel, "audio.webm", transcriptionFailingReader{}, nil)
	if transcriptionError == nil {
		t.Fatalf("transcriptionError=nil want non-nil")
	}
}
