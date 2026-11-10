package llmproxyclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
)

func TestQuotedInvalidConstructionAndHandlesStayOutsideHTTP(t *testing.T) {
	client := financialClient(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid SDK value reached HTTP") })
	if _, err := (llmproxyclient.FinancialQuoteInput{}).Payload(); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
		t.Fatal("zero input exposed payload", err)
	}
	if _, err := (llmproxyclient.FinancialQuote{}).Snapshot(); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
		t.Fatal("zero quote exposed snapshot", err)
	}
	if _, ok := (llmproxyclient.QuotedExecution{}).NativeResult(); ok {
		t.Fatal("zero execution exposed native result")
	}
	if _, ok := (llmproxyclient.QuotedExecution{}).MediaResult(); ok {
		t.Fatal("zero execution exposed media result")
	}
	for _, test := range []struct {
		name string
		call func() error
	}{
		{"lookup-key", func() error { _, err := client.GetFinancialQuote(context.Background(), "bad key"); return err }},
		{"lookup-id", func() error { _, err := client.GetFinancialQuoteByID(context.Background(), "../quote"); return err }},
		{"release-zero", func() error {
			_, err := client.ReleaseFinancialQuote(context.Background(), llmproxyclient.FinancialQuote{})
			return err
		}},
		{"native-zero", func() error {
			_, err := llmproxyclient.NewNativeFinancialQuoteInput(strings.Repeat("a", 64), "openai", llmproxyclient.MessagesRequest{})
			return err
		}},
		{"digest", func() error {
			_, err := llmproxyclient.NewMediaFinancialQuoteInput("wrong", llmproxyclient.MediaOperationInput{Capability: "image.generate", Provider: "openai", Model: "exact-image", Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{}`)})
			return err
		}},
		{"provider", func() error {
			_, err := llmproxyclient.NewMediaFinancialQuoteInput(strings.Repeat("a", 64), llmproxyclient.MediaOperationInput{Capability: "image.generate", Provider: "wrong provider", Model: "exact-image", Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{}`)})
			return err
		}},
		{"model", func() error {
			_, err := llmproxyclient.NewMediaFinancialQuoteInput(strings.Repeat("a", 64), llmproxyclient.MediaOperationInput{Capability: "image.generate", Provider: "openai", Model: "", Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{}`)})
			return err
		}},
		{"malformed-media", func() error {
			_, err := llmproxyclient.NewMediaFinancialQuoteInput(strings.Repeat("a", 64), llmproxyclient.MediaOperationInput{Capability: "image.generate", Provider: "openai", Model: "exact-image", Input: json.RawMessage(`{"prompt":`), Controls: json.RawMessage(`{}`)})
			return err
		}},
		{"non-object-media", func() error {
			_, err := llmproxyclient.NewMediaFinancialQuoteInput(strings.Repeat("a", 64), llmproxyclient.MediaOperationInput{Capability: "image.generate", Provider: "openai", Model: "exact-image", Input: json.RawMessage(`[]`), Controls: json.RawMessage(`{}`)})
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
				t.Fatal("invalid request or value accepted", err)
			}
		})
	}
}

func TestQuotedExecutionRoundTripAccessorsIdentifyTheAddressedResource(t *testing.T) {
	id := "quote-" + strings.Repeat("3", 32)
	client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/executions") {
			if request.URL.Path != "/model/v1/financial-quotes/"+id+"/executions" {
				t.Error("read addressed another quote")
			}
			quotedHeaders(writer)
			writer.Header().Set("X-LLM-Proxy-Quote-ID", id)
			_, _ = writer.Write([]byte(`{"answer":"the addressed result"}`))
			return
		}
		payload := quotedPayload()
		payload["id"] = id
		_ = json.NewEncoder(writer).Encode(payload)
	})
	quote, err := client.GetFinancialQuoteByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.GetQuotedExecution(context.Background(), quote)
	if err != nil {
		t.Fatal(err)
	}
	native, ok := result.NativeResult()
	if result.QuoteID() != id || !ok || native.Output != `{"answer":"the addressed result"}` || result.PendingError() != nil {
		t.Fatal("execution accessor lost addressed result")
	}
	if _, ok := result.MediaResult(); ok {
		t.Fatal("native execution exposed media variant")
	}
}

type quotedFailingTransport struct{}

func (quotedFailingTransport) Do(*http.Request) (*http.Response, error) {
	return nil, io.ErrUnexpectedEOF
}
func TestQuotedFailuresNeverConfirmExecutionOrRelease(t *testing.T) {
	validClient := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(quotedPayload())
	})
	quote, err := validClient.GetFinancialQuote(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	for _, client := range []llmproxyclient.Client{{}, func() llmproxyclient.Client {
		config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: "https://proxy.example.invalid", Secret: "application-key"})
		if err != nil {
			t.Fatal(err)
		}
		client, err := llmproxyclient.NewClient(config, quotedFailingTransport{})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}()} {
		if _, err := client.GetQuotedExecution(context.Background(), quote); err == nil {
			t.Fatal("unavailable transport returned an execution")
		}
	}
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{{"http-failure", 503, `{"error":{"code":"quote_unavailable"}}`}, {"malformed-json", 200, `{`}, {"not-object", 200, `[]`}} {
		t.Run(test.name, func(t *testing.T) {
			client := financialClient(t, func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			})
			if _, err := client.GetFinancialQuoteByID(context.Background(), "quote-"+strings.Repeat("1", 32)); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
				t.Fatal("invalid resource returned a quote", err)
			}
		})
	}
}
func TestQuotedNativeExecutionRejectsInvalidPendingIdentityAndJSON(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{{"invalid-pending", 202, `{"state":"unknown"}`}, {"another-execution", 202, `{"state":"dispatched","proxy_request_id":"other-execution","started_at":"2026-11-09T19:00:00Z","updated_at":"2026-11-09T19:00:01Z","elapsed_seconds":1}`}, {"invalid-completed-json", 200, `{`}} {
		t.Run(test.name, func(t *testing.T) {
			client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
				if strings.HasSuffix(request.URL.Path, "/executions") {
					quotedHeaders(writer)
					writer.WriteHeader(test.status)
					_, _ = writer.Write([]byte(test.body))
					return
				}
				payload := quotedPayload()
				payload["state"] = "consumed"
				payload["execution_id"] = "original-execution"
				_ = json.NewEncoder(writer).Encode(payload)
			})
			quote, err := client.GetFinancialQuote(context.Background(), "key")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.GetQuotedExecution(context.Background(), quote); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
				t.Fatal("invalid native execution accepted", err)
			}
		})
	}
}

func quotedMediaPayload() map[string]interface{} {
	return map[string]interface{}{"operation_id": "mop_" + strings.Repeat("4", 32), "capability": "image.edit", "provider": "openai", "model": "exact-image", "catalog_revision": "catalog-v1", "state": "failed", "cancellation_state": "not_requested", "outputs": []interface{}{map[string]interface{}{"asset_id": "ast_" + strings.Repeat("a", 32), "mime_type": "image/png", "size_bytes": 10, "ordinal": 0}}, "partial_outputs": []interface{}{map[string]interface{}{"asset_id": "ast_" + strings.Repeat("b", 32), "mime_type": "image/png", "size_bytes": 5, "output_ordinal": 0, "partial_ordinal": 0}}, "error": map[string]interface{}{"code": "provider_error"}, "cost": map[string]interface{}{"available": false, "reason": "provider_error"}, "accepted_at": "2026-11-09T19:00:00Z", "updated_at": "2026-11-09T19:00:00Z", "deadline_at": "2026-11-09T20:00:00Z"}
}
func TestQuotedDetachedMediaIntentReplayAndVariantCopies(t *testing.T) {
	quoteID := "quote-" + strings.Repeat("4", 32)
	var original map[string]interface{}
	client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !strings.HasSuffix(request.URL.Path, "/executions") {
			original = body
			intent := body["intent"].(map[string]interface{})
			typed := intent["request"].(map[string]interface{})
			if intent["kind"] != "media_operation" || intent["provider"] != "openai" || typed["capability"] != "image.edit" || typed["model"] != "exact-image" || typed["input"].(map[string]interface{})["prompt"] != "Fictional image edit" {
				t.Error("typed quoted request body changed")
			}
			payload := quotedPayload()
			payload["id"] = quoteID
			payload["execution_kind"] = "media_operation"
			_ = json.NewEncoder(writer).Encode(payload)
			return
		}
		if request.URL.Path != "/model/v1/financial-quotes/"+quoteID+"/executions" || !reflect.DeepEqual(body, original) {
			t.Error("execution changed frozen body or addressed resource")
		}
		quotedHeaders(writer)
		writer.Header().Set("X-LLM-Proxy-Quote-ID", quoteID)
		_ = json.NewEncoder(writer).Encode(quotedMediaPayload())
	})
	raw := json.RawMessage(`{"prompt":"Fictional image edit"}`)
	controls := json.RawMessage(`{}`)
	input, err := llmproxyclient.NewMediaFinancialQuoteInput(strings.Repeat("a", 64), llmproxyclient.MediaOperationInput{Capability: "image.edit", Provider: "openai", Model: "exact-image", Input: raw, Controls: controls})
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = '!'
	controls[0] = '!'
	copyPayload, err := input.Payload()
	if err != nil {
		t.Fatal(err)
	}
	copyPayload[0] = '!'
	quote, err := client.CreateFinancialQuote(context.Background(), "image-edit-key", input)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := quote.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.ID = "quote-" + strings.Repeat("9", 32)
	result, err := client.CreateQuotedExecution(context.Background(), quote, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.QuoteID() != quoteID || result.PendingError() != nil {
		t.Fatal("media variant lost quote identity")
	}
	if _, ok := result.NativeResult(); ok {
		t.Fatal("media execution exposed native variant")
	}
	media, ok := result.MediaResult()
	if !ok || media.Error == nil || len(media.Outputs) != 1 || len(media.PartialOutputs) != 1 {
		t.Fatal("media result omitted retained error or outputs")
	}
	media.Error.Code = "mutated"
	media.Outputs[0].AssetID = "mutated"
	media.PartialOutputs[0].AssetID = "mutated"
	again, ok := result.MediaResult()
	if !ok || again.Error.Code != "provider_error" || again.Outputs[0].AssetID != "ast_"+strings.Repeat("a", 32) || again.PartialOutputs[0].AssetID != "ast_"+strings.Repeat("b", 32) {
		t.Fatal("media accessor retained caller mutable storage")
	}
}
func TestQuotedMediaRejectsWrongResponseShapeModelAndFrozenRoute(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(map[string]interface{})
	}{{"native-variant", func(value map[string]interface{}) {
		for key := range value {
			delete(value, key)
		}
		value["answer"] = "wrong protocol"
	}}, {"model-type", func(value map[string]interface{}) { value["model"] = 42 }}, {"model-empty", func(value map[string]interface{}) { value["model"] = "" }}, {"route-model", func(value map[string]interface{}) { value["model"] = "another-image" }}, {"route-provider", func(value map[string]interface{}) { value["provider"] = "another" }}, {"route-capability", func(value map[string]interface{}) { value["capability"] = "image.generate" }}} {
		t.Run(test.name, func(t *testing.T) {
			client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
				if !strings.HasSuffix(request.URL.Path, "/executions") {
					payload := quotedPayload()
					payload["execution_kind"] = "media_operation"
					_ = json.NewEncoder(writer).Encode(payload)
					return
				}
				quotedHeaders(writer)
				payload := quotedMediaPayload()
				test.change(payload)
				_ = json.NewEncoder(writer).Encode(payload)
			})
			input, err := llmproxyclient.NewMediaFinancialQuoteInput(strings.Repeat("a", 64), llmproxyclient.MediaOperationInput{Capability: "image.edit", Provider: "openai", Model: "exact-image", Input: json.RawMessage(`{}`), Controls: json.RawMessage(`{}`)})
			if err != nil {
				t.Fatal(err)
			}
			quote, err := client.CreateFinancialQuote(context.Background(), "key", input)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.CreateQuotedExecution(context.Background(), quote, input); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
				t.Fatal("untrusted media result accepted", err)
			}
		})
	}
}
func TestQuotedCreationFailureReturnsNoUsableReservation(t *testing.T) {
	client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != "POST" {
			t.Error("creation retried as another operation")
		}
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"error":{"code":"quote_unavailable"}}`))
	})
	quote, err := client.CreateFinancialQuote(context.Background(), "key", quotedNativeInput(t))
	if !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
		t.Fatal(err)
	}
	if _, err = quote.Snapshot(); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
		t.Fatal("failed creation exposed usable quote")
	}
}
