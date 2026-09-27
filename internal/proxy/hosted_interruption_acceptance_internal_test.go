package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
)

func TestHostedDispatchedRequestInterruptionRetainsUncertainty(t *testing.T) {
	for _, mode := range []string{"timeout", "disconnect"} {
		t.Run(mode, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			var calls atomic.Int64
			provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if _, err := io.Copy(io.Discard, request.Body); err != nil {
					t.Error(err)
					return
				}
				calls.Add(1)
				started <- struct{}{}
				select {
				case <-request.Context().Done():
				case <-release:
				}
			}))
			t.Cleanup(func() { close(release); provider.Close() })
			_, service, upstream := newHostedIdentityHTTPHandler(t, database, provider.URL, t.TempDir())
			policy, err := newRequestTimeoutPolicy(1, 1)
			if err != nil {
				t.Fatal(err)
			}
			logger := zap.NewNop().Sugar()
			router := gin.New()
			router.Use(requestIdentifierHandler())
			router.POST(rootPath, tenantAuthenticatedHandler(newTenantAuthenticator(service.store), logger, requestTimeoutHandler(policy, logger, chatJSONHandler(upstream, service.store.routingDefaults, 1024, service.store, logger))))
			completed := make(chan struct{}, 3)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				defer func() { completed <- struct{}{} }()
				router.ServeHTTP(writer, request)
			}))
			t.Cleanup(server.Close)
			server.Client().Transport = hostedIdentityTransport{next: server.Client().Transport}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/?provider=openai&model=gpt-4.1", strings.NewReader(`{"prompt":"private prompt"}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(llmproxycontract.HeaderIdempotencyKey, mode)
			type outcome struct {
				status int
				err    error
			}
			result := make(chan outcome, 1)
			go func() {
				response, err := server.Client().Do(request)
				if err != nil {
					result <- outcome{err: err}
					return
				}
				_, readErr := io.Copy(io.Discard, response.Body)
				response.Body.Close()
				result <- outcome{status: response.StatusCode, err: readErr}
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("provider dispatch did not start")
			}
			if mode == "disconnect" {
				cancel()
			}
			select {
			case response := <-result:
				if mode == "disconnect" {
					if !errors.Is(response.err, context.Canceled) {
						t.Fatalf("disconnect result=%+v", response)
					}
				} else if response.status != http.StatusGatewayTimeout || response.err != nil {
					t.Fatalf("timeout result=%+v", response)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("interrupted HTTP request did not return")
			}
			select {
			case <-completed:
			case <-time.After(5 * time.Second):
				t.Fatal("journal finalization did not finish")
			}
			entries := read("")["requests"].([]any)
			if len(entries) != 1 {
				t.Fatalf("interruption requests=%v", entries)
			}
			entry := entries[0].(map[string]any)
			if entry["state"] != string(journalRequestUncertain) || entry["usage_state"] != string(journalUsageUnknown) {
				t.Fatalf("interruption journal=%v", entry)
			}
			hostedIdentityHTTP(t, server, mode, "private prompt", http.StatusConflict)
			if calls.Load() != 1 {
				t.Fatalf("interruption replay repeated work: calls=%d", calls.Load())
			}
			pending, err := database.pendingJournalDeliveries(t.Context(), 100)
			if err != nil || len(pending) != 0 {
				t.Fatalf("unknown dispatch produced settlement evidence: %v error=%v", pending, err)
			}
		})
	}
}
