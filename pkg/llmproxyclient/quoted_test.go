package llmproxyclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestQuotedCreationUsesExplicitFrozenNativeIntent(t *testing.T) {
	digest := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != "POST" || request.URL.Path != "/model/v1/financial-quotes" || request.Header.Get("Authorization") != "Bearer tenant-key" || request.Header.Get("Idempotency-Key") != "quote-key" {
			t.Errorf("unexpected quote request %s %s", request.Method, request.URL)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["processing_digest"] != digest {
			t.Error("lost processing identity")
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"id":"quote-11111111111111111111111111111111","revision":1,"state":"reserved","execution_kind":"native_v2","processing_digest":"` + digest + `","request_digest":"` + strings.Repeat("b", 64) + `","request_id":"request-22222222222222222222222222222222","reservation_id":"request-22222222222222222222222222222222","pricing_revision":"prices-v1","price_snapshot_digest":"` + strings.Repeat("c", 64) + `","currency":"USD","maximum_cents":"25","expires_at":"2026-11-09T20:00:00Z","created_at":"2026-11-09T19:00:00Z","updated_at":"2026-11-09T19:00:00Z","execution_id":""}`))
	}))
	defer server.Close()
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "tenant-key"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Model: "exact-model", Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Fictional document facts"}}, StructuredOutput: &llmproxyclient.StructuredOutputInput{JSONSchema: json.RawMessage(`{"type":"object"}`)}, IdempotencyKey: "unused-native-header-key"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := llmproxyclient.NewNativeFinancialQuoteInput(digest, "openai", request)
	if err != nil {
		t.Fatal(err)
	}
	quote, err := client.CreateFinancialQuote(context.Background(), "quote-key", input)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := quote.Snapshot()
	if err != nil || snapshot.MaximumCents != "25" || snapshot.State != "reserved" {
		t.Fatalf("quote %+v %v", snapshot, err)
	}
}

func quotedNativeInput(t *testing.T) llmproxyclient.FinancialQuoteInput {
	t.Helper()
	request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Model: "exact-model", Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Fictional facts"}}, StructuredOutput: &llmproxyclient.StructuredOutputInput{JSONSchema: []byte(`{"type":"object"}`)}, IdempotencyKey: "constructor-key"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := llmproxyclient.NewNativeFinancialQuoteInput(strings.Repeat("a", 64), "openai", request)
	if err != nil {
		t.Fatal(err)
	}
	return input
}
func quotedPayload() map[string]any {
	return map[string]any{"id": "quote-" + strings.Repeat("1", 32), "revision": 1, "state": "reserved", "execution_kind": "native_v2", "processing_digest": strings.Repeat("a", 64), "request_digest": strings.Repeat("b", 64), "request_id": "request-" + strings.Repeat("2", 32), "reservation_id": "request-" + strings.Repeat("2", 32), "pricing_revision": "prices-v1", "price_snapshot_digest": strings.Repeat("c", 64), "currency": "USD", "maximum_cents": "25", "expires_at": "2026-11-09T20:00:00Z", "created_at": "2026-11-09T19:00:00Z", "updated_at": "2026-11-09T19:00:00Z", "execution_id": ""}
}
func quotedHeaders(writer http.ResponseWriter) {
	writer.Header().Set("X-LLM-Proxy-Quote-ID", "quote-"+strings.Repeat("1", 32))
	writer.Header().Set("X-LLM-Proxy-Processing-Digest", strings.Repeat("a", 64))
	writer.Header().Set("X-LLM-Proxy-Request-Digest", strings.Repeat("b", 64))
}
func TestQuotedLostCreateLookupReleaseAndPayloadIndependentRead(t *testing.T) {
	calls := []string{}
	payload := quotedPayload()
	client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
		calls = append(calls, request.Method+" "+request.URL.Path)
		if request.Header.Get("Authorization") != "Bearer application-key" || request.URL.RawQuery != "" {
			t.Error("invalid resource authentication")
		}
		if request.URL.Path == "/model/v1/financial-quotes" && request.Header.Get("Idempotency-Key") != "original-key" {
			t.Error("lost quote creation key")
		}
		if request.Method == "PATCH" {
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body["revision"] != float64(payload["revision"].(int)) || body["state"] != "released" {
				t.Error("missing release CAS")
			}
			payload["state"] = "released"
			payload["revision"] = 2
		}
		_ = json.NewEncoder(writer).Encode(payload)
	})
	quote, err := client.CreateFinancialQuote(context.Background(), "original-key", quotedNativeInput(t))
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := client.GetFinancialQuote(context.Background(), "original-key")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := recovered.Snapshot()
	byID, err := client.GetFinancialQuoteByID(context.Background(), snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	released, err := client.ReleaseFinancialQuote(context.Background(), byID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ = released.Snapshot()
	if snapshot.State != "released" || len(calls) != 4 {
		t.Fatalf("release/lookup %+v %v", snapshot, calls)
	}
	before, _ := quote.Snapshot()
	if before.State != "reserved" {
		t.Fatal("release mutated original immutable snapshot")
	}
	again, err := client.ReleaseFinancialQuote(context.Background(), released)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ = again.Snapshot()
	if snapshot.State != "released" {
		t.Fatal("idempotent release lost state")
	}
}
func TestQuotedExecutionVerifiesHeadersAndKeepsPendingDistinct(t *testing.T) {
	calls := 0
	pending := true
	client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if !strings.HasSuffix(request.URL.Path, "/executions") {
			_ = json.NewEncoder(writer).Encode(quotedPayload())
			return
		}
		if request.Header.Get("Idempotency-Key") != "" {
			t.Error("execution added a second idempotency identity")
		}
		quotedHeaders(writer)
		if pending {
			writer.WriteHeader(http.StatusAccepted)
			_, _ = writer.Write([]byte(`{"state":"dispatched","proxy_request_id":"execution-one","started_at":"2026-11-09T19:00:00Z","updated_at":"2026-11-09T19:00:01Z","elapsed_seconds":1}`))
			return
		}
		_, _ = writer.Write([]byte(`{"answer":"supported fictional claim"}`))
	})
	input := quotedNativeInput(t)
	quote, err := client.CreateFinancialQuote(context.Background(), "quote-one", input)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := client.CreateQuotedExecution(context.Background(), quote, input)
	if err != nil {
		t.Fatal(err)
	}
	native, ok := execution.NativeResult()
	if !ok || native.State != "dispatched" || !errors.Is(execution.PendingError(), llmproxyclient.ErrStructuredRequestPending) {
		t.Fatalf("pending state lost %+v", execution)
	}
	pending = false
	execution, err = client.GetQuotedExecution(context.Background(), quote)
	if err != nil {
		t.Fatal(err)
	}
	native, ok = execution.NativeResult()
	if !ok || native.State != "succeeded" || native.Output != `{"answer":"supported fictional claim"}` || execution.PendingError() != nil || calls != 3 {
		t.Fatalf("result %+v calls=%d", native, calls)
	}
}
func TestQuotedRejectsInvalidResponseAndAddressedOwnership(t *testing.T) {
	cases := map[string]func(map[string]any){"wrong-id": func(p map[string]any) { p["id"] = "quote-" + strings.Repeat("9", 32) }, "zero-revision": func(p map[string]any) { p["revision"] = 0 }, "wrong-reservation": func(p map[string]any) { p["reservation_id"] = "request-" + strings.Repeat("3", 32) }, "unknown-state": func(p map[string]any) { p["state"] = "unknown" }, "unknown-kind": func(p map[string]any) { p["execution_kind"] = "text" }, "non-string-money": func(p map[string]any) { p["maximum_cents"] = 25 }, "fractional-money": func(p map[string]any) { p["maximum_cents"] = "1.5" }, "leading-zero-money": func(p map[string]any) { p["maximum_cents"] = "01" }, "overflow": func(p map[string]any) { p["maximum_cents"] = "9223372036854775808" }, "wrong-currency": func(p map[string]any) { p["currency"] = "EUR" }, "consumed-without-id": func(p map[string]any) { p["state"] = "consumed" }, "reserved-execution": func(p map[string]any) { p["execution_id"] = "unexpected" }, "missing-field": func(p map[string]any) { delete(p, "execution_id") }, "null-field": func(p map[string]any) { p["execution_id"] = nil }, "unknown-field": func(p map[string]any) { p["receipt"] = map[string]any{} }, "timestamp": func(p map[string]any) { p["updated_at"] = "not-a-time" }, "expired-before-expiry": func(p map[string]any) { p["state"] = "expired" }}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			payload := quotedPayload()
			mutate(payload)
			calls := 0
			client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
				calls++
				_ = json.NewEncoder(writer).Encode(payload)
			})
			if _, err := client.GetFinancialQuoteByID(context.Background(), "quote-"+strings.Repeat("1", 32)); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
				t.Fatalf("invalid quote accepted: %v", err)
			}
			if calls != 1 {
				t.Fatal("hidden retry")
			}
		})
	}
	for _, header := range []string{"X-LLM-Proxy-Quote-ID", "X-LLM-Proxy-Processing-Digest", "X-LLM-Proxy-Request-Digest"} {
		t.Run(header, func(t *testing.T) {
			client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
				if !strings.HasSuffix(request.URL.Path, "/executions") {
					_ = json.NewEncoder(writer).Encode(quotedPayload())
					return
				}
				quotedHeaders(writer)
				writer.Header().Set(header, "wrong")
				_, _ = writer.Write([]byte(`{"answer":"untrusted"}`))
			})
			quote, err := client.GetFinancialQuote(context.Background(), "quote-key")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.GetQuotedExecution(context.Background(), quote); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
				t.Fatal("untrusted execution identity accepted", err)
			}
		})
	}
}
func TestQuotedReadStatesAndErrorsNeverFabricateRelease(t *testing.T) {
	for _, state := range []string{"reserved", "consumed", "expired", "released"} {
		t.Run(state, func(t *testing.T) {
			payload := quotedPayload()
			payload["state"] = state
			if state == "consumed" {
				payload["execution_id"] = "execution-one"
			}
			if state == "expired" {
				payload["updated_at"] = "2026-11-09T20:01:00Z"
			}
			client := financialClient(t, func(writer http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(writer).Encode(payload) })
			quote, err := client.GetFinancialQuote(context.Background(), "original-key")
			if err != nil {
				t.Fatal(err)
			}
			snapshot, _ := quote.Snapshot()
			if snapshot.State != state {
				t.Fatal("read translated reservation state")
			}
		})
	}
	calls := 0
	client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if request.Method == "GET" && request.URL.Path == "/model/v1/financial-quotes" {
			_ = json.NewEncoder(writer).Encode(quotedPayload())
			return
		}
		writer.WriteHeader(http.StatusConflict)
		_, _ = writer.Write([]byte(`{"error":{"code":"quote_conflict"}}`))
	})
	quote, err := client.GetFinancialQuote(context.Background(), "original-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.ReleaseFinancialQuote(context.Background(), quote); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
		t.Fatal("release conflict lost")
	}
	var failure *llmproxyclient.HTTPFailure
	if !errors.As(err, &failure) || failure.StatusCode() != 409 || failure.ProxyErrorCode() != "quote_conflict" {
		t.Fatalf("unstable quote error %v", err)
	}
	snapshot, _ := quote.Snapshot()
	if snapshot.State != "reserved" || calls != 2 {
		t.Fatal("error fabricated release or retried")
	}
}
func TestQuotedInputCopiesAndInvalidZeroAreRejected(t *testing.T) {
	input := quotedNativeInput(t)
	first, _ := input.Payload()
	first[0] = '!'
	second, _ := input.Payload()
	if second[0] != '{' {
		t.Fatal("input retained caller mutable bytes")
	}
	client := financialClient(t, func(http.ResponseWriter, *http.Request) { t.Fatal("invalid input reached transport") })
	if _, err := client.CreateFinancialQuote(context.Background(), "bad key", input); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
		t.Fatal(err)
	}
	if _, err := client.CreateFinancialQuote(context.Background(), "good", llmproxyclient.FinancialQuoteInput{}); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
		t.Fatal(err)
	}
	if _, err := client.GetQuotedExecution(context.Background(), llmproxyclient.FinancialQuote{}); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
		t.Fatal(err)
	}
	media, err := llmproxyclient.NewMediaFinancialQuoteInput(strings.Repeat("a", 64), llmproxyclient.MediaOperationInput{Capability: "video.generate", Provider: "openai", Model: "exact-model", Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{}`)})
	if err == nil {
		t.Fatal("unsupported quoted media accepted", media)
	}
}

func TestQuotedImageExecutionAndReadUseOneBoundReservation(t *testing.T) {
	quoteValue := quotedPayload()
	quoteValue["execution_kind"] = "media_operation"
	operationID := "mop_" + strings.Repeat("4", 32)
	operation := map[string]any{"operation_id": operationID, "capability": "image.generate", "provider": "openai", "model": "exact-image", "catalog_revision": "catalog-v1", "state": "queued", "cancellation_state": "not_requested", "outputs": []any{}, "cost": map[string]any{"available": false, "reason": "not_settled"}, "accepted_at": "2026-11-09T19:00:00Z", "updated_at": "2026-11-09T19:00:00Z", "deadline_at": "2026-11-09T20:00:00Z"}
	calls := 0
	client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if !strings.HasSuffix(request.URL.Path, "/executions") {
			if request.Method == "GET" {
				quoteValue["state"] = "consumed"
				quoteValue["execution_id"] = operationID
			}
			_ = json.NewEncoder(writer).Encode(quoteValue)
			return
		}
		quotedHeaders(writer)
		if request.Method == "POST" {
			writer.WriteHeader(http.StatusAccepted)
		}
		_ = json.NewEncoder(writer).Encode(operation)
	})
	input, err := llmproxyclient.NewMediaFinancialQuoteInput(strings.Repeat("a", 64), llmproxyclient.MediaOperationInput{Capability: "image.generate", Provider: "openai", Model: "exact-image", Input: json.RawMessage(`{"prompt":"Fictional diagram"}`), Controls: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	quote, err := client.CreateFinancialQuote(context.Background(), "image-quote", input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.CreateQuotedExecution(context.Background(), quote, input)
	if err != nil {
		t.Fatal(err)
	}
	media, ok := result.MediaResult()
	if !ok || media.OperationID != operationID || media.State != "queued" {
		t.Fatal(media)
	}
	restored, err := client.GetFinancialQuoteByID(context.Background(), "quote-"+strings.Repeat("1", 32))
	if err != nil {
		t.Fatal(err)
	}
	result, err = client.GetQuotedExecution(context.Background(), restored)
	if err != nil {
		t.Fatal(err)
	}
	media, ok = result.MediaResult()
	if !ok || media.OperationID != operationID || calls != 4 {
		t.Fatalf("media read %+v calls=%d", media, calls)
	}
	operation["operation_id"] = "mop_" + strings.Repeat("5", 32)
	if _, err = client.GetQuotedExecution(context.Background(), restored); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
		t.Fatal("media read accepted different execution ID", err)
	}
}
func TestQuotedChangedIntentAndZeroClientDoNotReachTransport(t *testing.T) {
	client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/executions") {
			t.Error("changed intent reached execution")
		}
		_ = json.NewEncoder(writer).Encode(quotedPayload())
	})
	quote, err := client.CreateFinancialQuote(context.Background(), "original-key", quotedNativeInput(t))
	if err != nil {
		t.Fatal(err)
	}
	request, err := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{Model: "other-model", Messages: []llmproxyclient.MessageInput{{Role: "user", Content: "Fictional facts"}}, StructuredOutput: &llmproxyclient.StructuredOutputInput{JSONSchema: []byte(`{"type":"object"}`)}, IdempotencyKey: "constructor-key"})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := llmproxyclient.NewNativeFinancialQuoteInput(strings.Repeat("a", 64), "openai", request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.CreateQuotedExecution(context.Background(), quote, changed); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
		t.Fatal("changed payload accepted", err)
	}
	if _, err = (llmproxyclient.Client{}).GetFinancialQuote(context.Background(), "key"); !errors.Is(err, llmproxyclient.ErrInvalidClientConfig) {
		t.Fatal("zero client accepted", err)
	}
}

func TestQuotedCreationRejectsOtherProcessingAndTerminalReadDoesNotRelease(t *testing.T) {
	payload := quotedPayload()
	calls := 0
	client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if strings.HasSuffix(request.URL.Path, "/executions") {
			writer.WriteHeader(http.StatusGone)
			_, _ = writer.Write([]byte(`{"error":{"code":"hosted_result_expired"}}`))
			return
		}
		if request.Method == "POST" {
			payload["processing_digest"] = strings.Repeat("d", 64)
		}
		_ = json.NewEncoder(writer).Encode(payload)
	})
	if _, err := client.CreateFinancialQuote(context.Background(), "key", quotedNativeInput(t)); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
		t.Fatal("other processing digest accepted", err)
	}
	payload["processing_digest"] = strings.Repeat("a", 64)
	payload["state"] = "consumed"
	payload["execution_id"] = "execution-one"
	quote, err := client.GetFinancialQuote(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.GetQuotedExecution(context.Background(), quote); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
		t.Fatal("expired execution did not retain error", err)
	}
	var failure *llmproxyclient.HTTPFailure
	if !errors.As(err, &failure) || failure.StatusCode() != 410 || failure.ProxyErrorCode() != "hosted_result_expired" {
		t.Fatal(err)
	}
	snapshot, _ := quote.Snapshot()
	if snapshot.State != "consumed" || calls != 3 {
		t.Fatal("payload expiry fabricated release or retried")
	}
}
func TestQuotedReleaseRejectsSameRevisionTransition(t *testing.T) {
	payload := quotedPayload()
	client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == "PATCH" {
			payload["state"] = "released"
		}
		_ = json.NewEncoder(writer).Encode(payload)
	})
	quote, err := client.GetFinancialQuote(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.ReleaseFinancialQuote(context.Background(), quote); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
		t.Fatal("release accepted state change without CAS revision", err)
	}
}
