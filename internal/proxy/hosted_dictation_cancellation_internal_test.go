package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

type hostedDictationCancellationReader struct {
	*bytes.Reader
	cancel context.CancelFunc
}

func (reader hostedDictationCancellationReader) Read(buffer []byte) (int, error) {
	count, err := reader.Reader.Read(buffer)
	reader.cancel()
	return count, err
}

func TestHostedDictationCancellationBeforeAdmissionPreservesRequestKey(t *testing.T) {
	for _, path := range []string{dictatePath, transcriptionsPath} {
		t.Run(path, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			grantHostedDictation(t, database)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, `{"text":"recovered transcript","usage":{"type":"duration","seconds":1}}`)
			}))
			t.Cleanup(upstream.Close)
			root := t.TempDir()
			server := newHostedDictationServer(t, database, upstream.URL, root)
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			field, target := formFieldAudio, path+"?provider=openai&model=gpt-transcribe&key="+hostedIdentityFixtureKey
			if path == transcriptionsPath {
				field, target = "file", path
				if err := form.WriteField("model", "openai/gpt-transcribe"); err != nil {
					t.Fatal(err)
				}
			}
			file, err := form.CreateFormFile(field, "audio.wav")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(file, "private audio"); err != nil {
				t.Fatal(err)
			}
			if err := form.Close(); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				ctx, cancel := context.WithCancel(t.Context())
				request := httptest.NewRequest(http.MethodPost, target, hostedDictationCancellationReader{bytes.NewReader(body.Bytes()), cancel}).WithContext(ctx)
				request.Header.Set("Content-Type", form.FormDataContentType())
				request.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
				request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "cancelled-audio")
				response := httptest.NewRecorder()
				server.Config.Handler.ServeHTTP(response, request)
				cancel()
				if response.Code != statusClientClosedRequest || response.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("cancelled audio status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
				}
				if strings.Contains(response.Body.String(), "private audio") || calls.Load() != 0 || len(read("")["requests"].([]any)) != 0 {
					t.Fatal("cancelled audio exposed input or admitted provider work")
				}
			}
			hostedDictationHTTP(t, server, path, "cancelled-audio", "private audio", http.StatusOK)
			server.Close()
			restarted := newHostedDictationServer(t, openJournalTransactionInstance(t, database), upstream.URL, root)
			hostedDictationHTTP(t, restarted, path, "cancelled-audio", "private audio", http.StatusOK)
			if calls.Load() != 1 || len(read("")["requests"].([]any)) != 1 {
				t.Fatal("recovered audio request repeated provider work")
			}
		})
	}
}
