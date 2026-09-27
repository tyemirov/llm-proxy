package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHostedFundsMCPRejectsUnfundedWorkAndReusesSettlement(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server, clientFor := newHostedMCPIdentityServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	client := clientFor("owner")
	input := map[string]any{"tenant_id": "managed-first", "idempotency_key": "funds-mcp", "provider": "openai", "model": "gpt-4.1", "messages": []map[string]string{{"role": "user", "content": "funded prompt"}}}
	if denied := hostedMCPCall(t, client, input, true); denied["code"] != errInsufficientFunds.Error() || calls.Load() != 0 {
		t.Fatalf("unfunded MCP=%v calls=%d", denied, calls.Load())
	}
	seedHostedFunds(t, database, 5)
	if result := hostedMCPCall(t, client, input, false); result["text"] != "funded result" {
		t.Fatalf("funded MCP=%v", result)
	}
	assertHostedFundsBalance(t, database, 5, 2)
	if err := deliverFundsFixture(t, database); err != nil {
		t.Fatal(err)
	}
	hostedIdentityHTTP(t, server, "funds-mcp", "funded prompt", http.StatusOK)
	assertHostedFundsBalance(t, database, 5, 5)
	if calls.Load() != 1 {
		t.Fatalf("settled MCP replay dispatched again: %d", calls.Load())
	}
}

func TestHostedFundsDictationUsesOneHoldAcrossProtocols(t *testing.T) {
	database, _, _, _ := newHostedRatingFixture(t)
	grantHostedDictation(t, database)
	catalog := internalTestModelCatalog(internalTestOffering("openai", "gpt-transcribe", []string{ModelOperationDictation}, []string{ModelOperationDictation}))
	catalog.Revision = "journal-catalog"
	seconds := 60
	catalog.Offerings[0].Limits = []CatalogLimit{{ID: "audio_seconds", Unit: "seconds", Value: &seconds}}
	conditions := CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}
	catalog.Prices[0] = CatalogPriceDescriptor{Provider: "openai", Model: "gpt-transcribe", Operation: ModelOperationDictation, Available: true, Source: "https://example.com/prices", LastVerified: "2026-09-22", Rates: []CatalogPriceRate{{Component: "input_audio", Currency: "USD", Rate: "0.0045", Unit: "USD/minute", Conditions: conditions}}}
	prices, err := NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	reserve, err := newHostedMediaPriceAdmission(prices, managedJournalRequestRecord{Provider: "openai", Model: "gpt-transcribe", Operation: ModelOperationDictation, CreatedAt: ratingTestAcceptanceTime()}, CatalogProtocolMultipartTranscription, categoricalPriceConditions(conditions), 1)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"text":"funded transcript","usage":{"type":"duration","seconds":1.25}}`)
	}))
	t.Cleanup(upstream.Close)
	server := newHostedDictationServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(reserve))
	for _, path := range []string{dictatePath, transcriptionsPath} {
		body := hostedDictationHTTP(t, server, path, "funds-dictation", "private audio", http.StatusPaymentRequired)
		if !strings.Contains(body, errInsufficientFunds.Error()) {
			t.Fatalf("missing funds error: %s", body)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("unfunded dictation dispatched %d times", calls.Load())
	}
	seedHostedFunds(t, database, 5)
	for _, path := range []string{dictatePath, transcriptionsPath} {
		hostedDictationHTTP(t, server, path, "funds-dictation", "private audio", http.StatusOK)
	}
	assertHostedFundsBalance(t, database, 5, 4)
	if err := deliverFundsFixture(t, database); err != nil {
		t.Fatal(err)
	}
	hostedDictationHTTP(t, server, dictatePath, "funds-dictation", "private audio", http.StatusOK)
	assertHostedFundsBalance(t, database, 5, 5)
	if calls.Load() != 1 {
		t.Fatalf("settled dictation replay dispatched again: %d", calls.Load())
	}
}
