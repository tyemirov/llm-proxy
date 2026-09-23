package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestHostedFundsAdmissionOfficialClientsKeepFundingErrors(t *testing.T) {
	for _, scenario := range []struct {
		name, code string
		status     int
	}{
		{"empty", llmproxycontract.ErrorCodeInsufficientFunds, http.StatusPaymentRequired},
		{"database-failure", llmproxycontract.ErrorCodeFinancialAdmissionUnavailable, http.StatusServiceUnavailable},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, _, prices := newHostedRatingFixture(t)
			if scenario.name == "database-failure" {
				seedHostedFunds(t, database, 5)
				if err := database.database.Exec("CREATE TRIGGER reject_client_funds BEFORE INSERT ON managed_funds_reservation_records BEGIN SELECT RAISE(ABORT, 'controlled_funds_failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			var calls atomic.Int64
			upstream := fundsUpstream(t, &calls)
			server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
			for _, protocol := range []llmproxyclient.Protocol{llmproxyclient.ProtocolNative, llmproxyclient.ProtocolOpenAIResponses} {
				t.Run(string(protocol), func(t *testing.T) {
					client, input := hostedIdentityGoClient(t, server.URL, protocol)
					_, err := client.PostMessages(t.Context(), input)
					var failure *llmproxyclient.HTTPFailure
					if !errors.As(err, &failure) || failure.StatusCode() != scenario.status || failure.ProxyErrorCode() != scenario.code {
						t.Fatalf("client lost funding error: %v", err)
					}
				})
			}
			t.Run("python", func(t *testing.T) {
				script := `import sys
from llm_proxy_client import Client, ClientConfig, ClientMessage, ClientMessagesRequest, LLMProxyHTTPError
client = Client(ClientConfig(base_url=sys.argv[1], secret=sys.argv[2], provider="openai"))
request = ClientMessagesRequest(messages=(ClientMessage(role="user", content="client prompt"),), model="gpt-4.1", idempotency_key="client-pending")
try:
    client.post_messages(request)
except LLMProxyHTTPError as error:
    assert error.status_code == int(sys.argv[3]) and error.proxy_error_code == sys.argv[4], repr(error)
else:
    raise AssertionError("request without funds did not fail")
`
				if output, err := hostedClientPythonCommand(t, script, server.URL, hostedIdentityFixtureKey, fmt.Sprint(scenario.status), scenario.code).CombinedOutput(); err != nil {
					t.Fatalf("Python client lost funding error: %s error=%v", output, err)
				}
			})
			if calls.Load() != 0 {
				t.Fatalf("client with no funds dispatched %d requests", calls.Load())
			}
		})
	}
}

func TestHostedFundsAdmissionRejectsBeforeDispatchAndReservesOnce(t *testing.T) {
	database, _, _, priceAdmission := newHostedRatingFixture(t)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(priceAdmission))
	body := hostedIdentityHTTP(t, server, "no-funds", "funded prompt", http.StatusPaymentRequired)
	if !strings.Contains(body, `"code":"insufficient_funds"`) || calls.Load() != 0 {
		t.Fatalf("unfunded request dispatched: calls=%d body=%s", calls.Load(), body)
	}
	for _, table := range []string{"managed_journal_request_records", "managed_price_snapshot_records", "managed_funds_reservation_records"} {
		var count int64
		if err := database.database.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("failed admission retained %s: count=%d error=%v", table, count, err)
		}
	}
	// A five-cent fixture balance represents funds left after earlier usage.
	seedHostedFunds(t, database, 5)
	for range 2 {
		if result := hostedIdentityHTTP(t, server, "funded", "funded prompt", http.StatusOK); result != "funded result" {
			t.Fatalf("funded result=%q", result)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("retry dispatched %d times", calls.Load())
	}
	assertHostedFundsBalance(t, database, 5, 2)
	hostedIdentityHTTP(t, server, "next-request", "funded prompt", http.StatusPaymentRequired)
	assertHostedFundsBalance(t, database, 5, 2)
	if calls.Load() != 1 {
		t.Fatalf("overdrawn request dispatched: %d", calls.Load())
	}
}

func TestHostedFundsAdmissionSerializesServiceInstances(t *testing.T) {
	database, _, _, priceAdmission := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	first := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(priceAdmission))
	second := newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, t.TempDir(), fundsDependencies(priceAdmission))
	start := make(chan struct{})
	var group sync.WaitGroup
	statuses := make(chan int, 2)
	for index, server := range []*httptest.Server{first, second} {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			request, err := http.NewRequest(http.MethodPost, server.URL+"/?provider=openai&model=gpt-4.1", strings.NewReader(`{"prompt":"funded prompt"}`))
			if err != nil {
				t.Error(err)
				return
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(llmproxycontract.HeaderIdempotencyKey, fmt.Sprintf("instance-%d", index))
			response, err := server.Client().Do(request)
			if err != nil {
				t.Error(err)
				return
			}
			defer response.Body.Close()
			statuses <- response.StatusCode
		}()
	}
	close(start)
	group.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[http.StatusOK] != 1 || counts[http.StatusPaymentRequired] != 1 || calls.Load() != 1 {
		t.Fatalf("concurrent admission: statuses=%v calls=%d", counts, calls.Load())
	}
	assertHostedFundsBalance(t, database, 5, 2)
}

func TestHostedFundsAdmissionFailureRollsBackLedgerHold(t *testing.T) {
	database, _, _, priceAdmission := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	if err := database.database.Exec("CREATE TRIGGER reject_funds_reservation BEFORE INSERT ON managed_funds_reservation_records BEGIN SELECT RAISE(ABORT, 'controlled_reservation_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(priceAdmission))
	body := hostedIdentityHTTP(t, server, "failed-reservation", "funded prompt", http.StatusServiceUnavailable)
	if !strings.Contains(body, `"code":"financial_admission_unavailable"`) || calls.Load() != 0 {
		t.Fatalf("failed reservation dispatched: calls=%d body=%s", calls.Load(), body)
	}
	assertHostedFundsBalance(t, database, 5, 5)
}

func fundsDependencies(prices journalReservation) func(*hostedTextRequestDependencies) {
	return func(dependencies *hostedTextRequestDependencies) {
		dependencies.now = ratingTestAcceptanceTime
		dependencies.authorize = fixedHostedCompletionAdmission(newHostedFundsAdmission(prices))
	}
}

func fundsUpstream(t *testing.T, calls *atomic.Int64) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"funded-provider-result","status":"completed","output_text":"funded result","usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(server.Close)
	return server
}

func seedHostedFunds(t *testing.T, database *gormManagedTenantDatabase, cents int64) {
	t.Helper()
	account, err := newHostedLedgerAccount(database.database, "billing-journal", ratingTestAcceptanceTime())
	if err != nil {
		t.Fatal(err)
	}
	amount, err := ledger.NewPositiveAmountCents(cents)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ledger.NewIdempotencyKey("controlled-funding")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := ledger.NewMetadataJSON(`{"source":"controlled-fixture"}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := account.service.Grant(t.Context(), account.tenant, account.user, account.namespace, amount, key, 0, metadata); err != nil {
		t.Fatal(err)
	}
}

func assertHostedFundsBalance(t *testing.T, database *gormManagedTenantDatabase, total, available int64) {
	t.Helper()
	account, err := newHostedLedgerAccount(database.database, "billing-journal", ratingTestAcceptanceTime().Add(365*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	balance, err := account.service.Balance(t.Context(), account.tenant, account.user, account.namespace)
	if err != nil || balance.TotalCents.Int64() != total || balance.AvailableCents.Int64() != available {
		encoded, _ := json.Marshal(balance)
		t.Fatalf("balance=%s want total=%d available=%d error=%v", encoded, total, available, err)
	}
}
