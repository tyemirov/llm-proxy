package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestHostedFailedCompletionReplayContract(t *testing.T) {
	for _, kind := range []string{"text", "dictation"} {
		t.Run(kind, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				writer.WriteHeader(http.StatusBadRequest)
			}))
			t.Cleanup(upstream.Close)
			if kind == "dictation" {
				grantHostedDictation(t, database)
				server := newHostedDictationServer(t, database, upstream.URL, t.TempDir())
				hostedDictationHTTP(t, server, dictatePath, "failed-replay", "private audio", http.StatusBadGateway)
				for _, path := range []string{dictatePath, transcriptionsPath} {
					body := hostedDictationHTTP(t, server, path, "failed-replay", "private audio", http.StatusBadGateway)
					if !strings.Contains(body, llmproxycontract.ErrorCodeStructuredRequestFailed) {
						t.Fatalf("dictation failed replay=%s", body)
					}
				}
			} else {
				server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir())
				hostedIdentityHTTP(t, server, "failed-replay", "private prompt", http.StatusBadGateway)
				for _, scenario := range []struct{ path, body string }{
					{"/?provider=openai&model=gpt-4.1", `{"prompt":"private prompt"}`},
					{v2Path + "?provider=openai&model=gpt-4.1", `{"messages":[{"role":"user","content":"private prompt"}]}`},
					{chatCompletionsPath, `{"model":"openai/gpt-4.1","messages":[{"role":"user","content":"private prompt"}]}`},
					{responsesPath, `{"model":"openai/gpt-4.1","input":"private prompt"}`},
				} {
					request, err := http.NewRequest(http.MethodPost, server.URL+scenario.path, strings.NewReader(scenario.body))
					if err != nil {
						t.Fatal(err)
					}
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set(llmproxycontract.HeaderIdempotencyKey, "failed-replay")
					response, err := server.Client().Do(request)
					if err != nil {
						t.Fatal(err)
					}
					body, err := io.ReadAll(response.Body)
					response.Body.Close()
					if err != nil || response.StatusCode != http.StatusBadGateway || !strings.Contains(string(body), llmproxycontract.ErrorCodeStructuredRequestFailed) {
						t.Fatalf("failed replay status=%d body=%s error=%v", response.StatusCode, body, err)
					}
					validateHostedIdentityResponse(t, request, response, body)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("failed replay repeated provider work: %d", calls.Load())
			}
		})
	}
}
