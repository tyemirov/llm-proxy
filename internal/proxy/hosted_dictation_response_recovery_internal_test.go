package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostedDictationMalformedResponsesRetainFundsWithoutResubmission(t *testing.T) {
	for _, scenario := range []struct{ name, body string }{
		{"truncated", `{"text":"private transcript","usage":`},
		{"array", `[]`},
		{"raw-text", `private transcript`},
		{"string", `"private transcript"`},
		{"null", `null`},
		{"number", `123`},
		{"transcript-alias", `{"transcript":"private transcript"}`},
		{"output-text-alias", `{"output_text":"private transcript"}`},
		{"wrong-text-type", `{"text":123}`},
		{"empty-text", `{"text":"  "}`},
		{"missing-text", `{"languages":[]}`},
		{"trailing-value", `{"text":"private transcript","usage":{"type":"duration","seconds":1}} {}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, management, _ := newHostedRatingFixture(t)
			grantHostedDictation(t, database)
			catalog, err := NewCatalogService(internalCanonicalProviderCatalog().ModelCatalog())
			if err != nil {
				t.Fatal(err)
			}
			offering, err := catalog.ResolveOffering("openai", "gpt-transcribe")
			if err != nil {
				t.Fatal(err)
			}
			settings := hostedDictationFinancialSettings(t, offering, "duration")
			configure := func(dependencies *hostedTextRequestDependencies) {
				dependencies.authorize = settings.authorizeCompletion
			}
			var calls atomic.Int64
			var response atomic.Value
			response.Store(scenario.body)
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.Header.Get("Authorization") != "Bearer sk-platform-pinned" {
					t.Error("dictation lost its pinned platform credential")
				}
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, response.Load().(string))
			}))
			t.Cleanup(upstream.Close)
			root := t.TempDir()
			server := newHostedDictationServer(t, database, upstream.URL, root, configure)
			seedHostedFunds(t, database, 500)
			body := hostedDictationHTTP(t, server, dictatePath, "malformed-audio", "private audio", http.StatusBadGateway)
			if strings.Contains(body, "private transcript") {
				t.Fatalf("malformed response published transcript content: %s", body)
			}
			assertHostedFundsBalance(t, database, 500, 422)
			server.Close()
			response.Store(`{"text":"restored transcript","usage":{"type":"duration","seconds":1}}`)
			financial := fundsStartupFixture{database: database, management: management}
			var previous map[string]any
			for iteration := range 2 {
				restarted := openJournalTransactionInstance(t, database)
				live := newHostedDictationServer(t, restarted, upstream.URL, root, configure)
				for _, path := range []string{dictatePath, transcriptionsPath} {
					hostedDictationHTTP(t, live, path, "malformed-audio", "private audio", http.StatusBadGateway)
				}
				hostedIdentityStatusHTTP(t, live, "malformed-audio", http.StatusBadGateway)
				if err := restarted.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
				live.Close()
				assertHostedFundsBalance(t, restarted, 500, 422)
				state := financial.state(t)
				charges := state["charges"].(map[string]any)["charges"].([]any)
				if len(charges) != 1 || calls.Load() != 1 {
					t.Fatalf("malformed response repeated provider work or lost usage: charges=%v calls=%d", charges, calls.Load())
				}
				charge := charges[0].(map[string]any)
				summary := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+charge["request_id"].(string)+"/charge-summary", "", http.StatusOK)
				if charge["state"] != chargeUsageUnresolved || charge["customer_charge"] != nil || summary["state"] != string(requestChargeUnresolved) || summary["provider_cost"] != nil || summary["customer_charge"] != nil {
					t.Fatalf("malformed native JSON became a known cost or customer charge: charge=%v summary=%v", charge, summary)
				}
				if iteration > 0 && !reflect.DeepEqual(previous, state) {
					t.Fatal("restarted dictation changed uncertain financial resources")
				}
				previous = state
			}
		})
	}
}
