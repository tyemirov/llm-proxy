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

func financialPayload() map[string]any {
	return map[string]any{
		"request_id": "request-" + strings.Repeat("a", 32), "reservation_id": "request-" + strings.Repeat("a", 32), "execution_id": "execution-one", "execution_kind": "text_request", "request_digest": strings.Repeat("b", 64), "pricing_revision": "prices-one", "price_snapshot_digest": strings.Repeat("c", 64), "currency": "USD", "maximum_cents": "3", "reservation_state": "settled", "reservation_revision": 1, "execution_state": "completed", "state": "settled", "receipt": map[string]any{"id": "request-" + strings.Repeat("a", 32), "revision": 1, "exact_customer_charge": map[string]any{"numerator": "12345678901234567890123456789", "denominator": "100000000000000000000000000000"}, "posted_cents": "0", "settled_at": "2026-11-09T12:00:00Z"}, "updated_at": "2026-11-09T12:00:00Z",
	}
}
func financialClient(t *testing.T, handler http.HandlerFunc) llmproxyclient.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: "application-key", Provider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return client
}
func TestFinancialRequestSDKProtocol(t *testing.T) {
	for _, state := range []string{"settled", "pending", "uncertain", "definitely_undispatched_released"} {
		t.Run(state, func(t *testing.T) {
			payload := financialPayload()
			payload["state"] = state
			if state != "settled" {
				payload["receipt"] = nil
				payload["reservation_state"] = "held"
				if state == "uncertain" {
					payload["reservation_state"] = "reconciliation_required"
				}
				if state == "definitely_undispatched_released" {
					payload["reservation_state"] = "released"
					payload["execution_state"] = "failed"
				}
			}
			client := financialClient(t, func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet || request.URL.RawQuery != "" || request.Header.Get("Authorization") != "Bearer application-key" {
					t.Errorf("invalid financial transport")
				}
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(payload)
			})
			status, err := client.GetFinancialRequest(context.Background(), "idempotency-one")
			if err != nil {
				t.Fatal(err)
			}
			if status.State != state {
				t.Fatal(status)
			}
			same, err := client.GetFinancialRequestByID(context.Background(), status.RequestID)
			if err != nil || same.State != state {
				t.Fatal(same, err)
			}
			if status.Receipt != nil && status.Receipt.ExactCustomerCharge.Numerator != "12345678901234567890123456789" {
				t.Fatal("SDK rounded financial evidence")
			}
		})
	}
}
func TestFinancialRequestSDKRejectsInvalidEvidence(t *testing.T) {
	cases := map[string]func(map[string]any){
		"identity": func(value map[string]any) { value["request_id"] = "wrong" }, "reservation": func(value map[string]any) { value["reservation_id"] = "other" }, "execution": func(value map[string]any) { value["execution_id"] = "" }, "price": func(value map[string]any) { value["pricing_revision"] = "" }, "currency": func(value map[string]any) { value["currency"] = "EUR" }, "revision": func(value map[string]any) { value["reservation_revision"] = 0 }, "digest": func(value map[string]any) { value["request_digest"] = "wrong" }, "snapshot": func(value map[string]any) { value["price_snapshot_digest"] = "wrong" }, "maximum": func(value map[string]any) { value["maximum_cents"] = "-1" }, "updated": func(value map[string]any) { value["updated_at"] = "not-time" }, "execution kind": func(value map[string]any) { value["execution_kind"] = "other" }, "execution state": func(value map[string]any) { value["execution_state"] = "other" }, "state": func(value map[string]any) { value["state"] = "other" }, "reservation state": func(value map[string]any) { value["reservation_state"] = "other" }, "missing receipt": func(value map[string]any) { value["receipt"] = nil }, "unsettled receipt": func(value map[string]any) { value["state"] = "pending" }, "wrong receipt": func(value map[string]any) { value["receipt"].(map[string]any)["id"] = "other" }, "wrong receipt revision": func(value map[string]any) { value["receipt"].(map[string]any)["revision"] = 2 }, "invalid posting": func(value map[string]any) { value["receipt"].(map[string]any)["posted_cents"] = "-1" }, "invalid numerator": func(value map[string]any) {
			value["receipt"].(map[string]any)["exact_customer_charge"].(map[string]any)["numerator"] = "-1"
		}, "invalid denominator": func(value map[string]any) {
			value["receipt"].(map[string]any)["exact_customer_charge"].(map[string]any)["denominator"] = "bad"
		}, "zero denominator": func(value map[string]any) {
			value["receipt"].(map[string]any)["exact_customer_charge"].(map[string]any)["denominator"] = "0"
		}, "settled time": func(value map[string]any) { value["receipt"].(map[string]any)["settled_at"] = "bad" }, "unknown field": func(value map[string]any) { value["unexpected"] = true },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			payload := financialPayload()
			change(payload)
			client := financialClient(t, func(writer http.ResponseWriter, _ *http.Request) { json.NewEncoder(writer).Encode(payload) })
			if _, err := client.GetFinancialRequest(context.Background(), "one"); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
				t.Fatalf("invalid financial evidence accepted: %v", err)
			}
		})
	}
	client := financialClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "unavailable", http.StatusServiceUnavailable)
	})
	if _, err := client.GetFinancialRequest(context.Background(), ""); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
		t.Fatal(err)
	}
	if _, err := client.GetFinancialRequestByID(context.Background(), "../other"); !errors.Is(err, llmproxyclient.ErrInvalidClientRequest) {
		t.Fatal(err)
	}
	if _, err := client.GetFinancialRequest(context.Background(), "valid"); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
		t.Fatal(err)
	}
	if _, err := client.GetFinancialRequestByID(context.Background(), "request-"+strings.Repeat("a", 32)); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
		t.Fatal(err)
	}
}

func TestFinancialRequestSDKRejectsOtherAddressedReceipt(t *testing.T) {
	client := financialClient(t, func(writer http.ResponseWriter, _ *http.Request) { json.NewEncoder(writer).Encode(financialPayload()) })
	if _, err := client.GetFinancialRequestByID(context.Background(), "request-"+strings.Repeat("d", 32)); !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
		t.Fatalf("another request's receipt accepted: %v", err)
	}
}

func TestFinancialRequestSDKRetainsAndValidatesQuoteOrigin(t *testing.T) {
	for _, test := range []struct {
		name      string
		reference interface{}
		valid     bool
	}{{"quoted-origin", map[string]interface{}{"id": "quote-" + strings.Repeat("1", 32), "processing_digest": strings.Repeat("d", 64)}, true}, {"malformed-origin", []interface{}{}, false}, {"unknown-origin-field", map[string]interface{}{"id": "quote-" + strings.Repeat("1", 32), "processing_digest": strings.Repeat("d", 64), "other": "unexpected"}, false}, {"invalid-origin-id", map[string]interface{}{"id": "another", "processing_digest": strings.Repeat("d", 64)}, false}, {"invalid-processing-digest", map[string]interface{}{"id": "quote-" + strings.Repeat("1", 32), "processing_digest": "wrong"}, false}, {"null-origin", nil, false}} {
		t.Run(test.name, func(t *testing.T) {
			payload := financialPayload()
			payload["quote"] = test.reference
			client := financialClient(t, func(writer http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(writer).Encode(payload) })
			result, err := client.GetFinancialRequestByID(context.Background(), payload["request_id"].(string))
			if !test.valid {
				if !errors.Is(err, llmproxyclient.ErrClientHTTPFailure) {
					t.Fatal("invalid financial quote origin accepted", err)
				}
				return
			}
			if err != nil || result.Quote == nil || result.Quote.ID != "quote-"+strings.Repeat("1", 32) || result.Quote.ProcessingDigest != strings.Repeat("d", 64) || result.Receipt == nil || result.Receipt.PostedCents != "0" {
				t.Fatal("financial quote origin or exact receipt lost", result, err)
			}
		})
	}
}
