package proxy

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var applicationFinancialContract = flag.String("application-financial-contract", "../../docs/openapi.yaml", "Canonical contract file for financial response checks")

func TestHostedFundsApplicationFinancialRead(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	responseRoot := t.TempDir()
	router, service, _ := newHostedIdentityHTTPHandler(t, database, upstream.URL, responseRoot, fundsDependencies(prices))
	registerApplicationFinancialRoutes(router, newTenantAuthenticator(service.store), zap.NewNop().Sugar(), database)
	registerClientMethodErrors(router)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	// Existing hosted transport is used only for dispatch, never financial reads.
	server.Client().Transport = hostedIdentityTransport{next: http.DefaultTransport}
	read := func(path, key, secret string, want int) map[string]any {
		t.Helper()
		request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if secret != "" {
			request.Header.Set("Authorization", "Bearer "+secret)
		}
		if key != "" {
			request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Fatalf("status=%d want=%d body=%s", response.StatusCode, want, body)
		}
		if want == http.StatusOK {
			contract, err := openapitest.Load(*applicationFinancialContract)
			if err != nil {
				t.Fatal(err)
			}
			if err := contract.ValidateResponse(func() string {
				if strings.HasPrefix(request.URL.Path, llmproxycontract.FinancialRequestsPath+"/") {
					return llmproxycontract.FinancialRequestsPath + "/{request_id}"
				}
				return request.URL.Path
			}(), request.Method, response.StatusCode, response.Header, body); err != nil {
				t.Fatal(err)
			}
		}
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("financial response is cacheable")
		}
		var value map[string]any
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	path := llmproxycontract.FinancialRequestsPath
	read(path, "missing", hostedIdentityFixtureKey, http.StatusNotFound)
	if calls.Load() != 0 {
		t.Fatal("financial read dispatched provider work")
	}
	seedHostedFunds(t, database, 5)
	hostedIdentityHTTP(t, server, "app-receipt", "funded prompt", http.StatusOK)
	pending := read(path, "app-receipt", hostedIdentityFixtureKey, http.StatusOK)
	if pending["state"] != "pending" || pending["receipt"] != nil {
		t.Fatalf("pending=%v", pending)
	}
	if err := deliverFundsFixture(t, database); err != nil {
		t.Fatal(err)
	}
	settled := read(path, "app-receipt", hostedIdentityFixtureKey, http.StatusOK)
	if settled["state"] != "settled" {
		t.Fatalf("settled=%v", settled)
	}
	receipt := settled["receipt"].(map[string]any)
	if receipt["exact_customer_charge"] == nil || receipt["posted_cents"] == nil || receipt["settled_at"] == nil {
		t.Fatalf("receipt=%v", receipt)
	}
	encoded, _ := json.Marshal(settled)
	for _, forbidden := range []string{"sk-platform", "private prompt", "billing-journal", "owner_token", "remainder_before"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("private field escaped: %s", forbidden)
		}
	}
	config, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: server.URL, Secret: hostedIdentityFixtureKey, Provider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(config, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	sdkStatus, err := client.GetFinancialRequest(t.Context(), "app-receipt")
	if err != nil {
		t.Fatal(err)
	}
	if sdkStatus.RequestID != settled["request_id"] || sdkStatus.Receipt == nil {
		t.Fatalf("SDK status=%v", sdkStatus)
	}
	sdkReceipt, err := client.GetFinancialRequestByID(t.Context(), sdkStatus.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sdkStatus, sdkReceipt) {
		t.Fatal("SDK retained receipt mismatch")
	}
	_, missingError := client.GetFinancialRequest(t.Context(), "missing-receipt")
	var sdkFailure *llmproxyclient.HTTPFailure
	if !errors.As(missingError, &sdkFailure) || sdkFailure.ProxyErrorCode() != "financial_request_not_found" {
		t.Fatalf("SDK lost financial error identity: %v", missingError)
	}
	script := `import sys
from llm_proxy_client import Client, ClientConfig, LLMProxyHTTPError
client=Client(ClientConfig(base_url=sys.argv[1],secret=sys.argv[2],provider="openai"))
status=client.get_financial_request("app-receipt")
assert status.state=="settled" and status.receipt is not None
assert client.get_financial_request_by_id(status.request_id)==status
try:
    client.get_financial_request("missing-receipt")
except LLMProxyHTTPError as error:
    assert error.proxy_error_code=="financial_request_not_found"
else:
    raise AssertionError("missing receipt succeeded")
`
	if output, err := hostedClientPythonCommand(t, script, server.URL, hostedIdentityFixtureKey).CombinedOutput(); err != nil {
		t.Fatalf("Python financial SDK: %s error=%v", output, err)
	}
	// Purge all caller execution payload files. Financial evidence remains in hosted storage.
	if err := os.RemoveAll(responseRoot); err != nil {
		t.Fatal(err)
	}
	byID := read(path+"/"+settled["request_id"].(string), "", hostedIdentityFixtureKey, http.StatusOK)
	got, _ := json.Marshal(byID["receipt"])
	want, _ := json.Marshal(receipt)
	if string(got) != string(want) {
		t.Fatal("retained immutable receipt changed after payload purge")
	}
	if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", hostedJournalFixtureGrantID).Update("state", hostedGrantRevoked).Error; err != nil {
		t.Fatal(err)
	}
	revoked := read(path+"/"+settled["request_id"].(string), "", hostedIdentityFixtureKey, http.StatusOK)
	if !reflect.DeepEqual(revoked["receipt"], receipt) {
		t.Fatal("grant revocation hid historical financial evidence")
	}
	// Another authenticated tenant shares no authority over the addressed journal request.
	secondSecret := "second-application-secret"
	digest := sha256Hex(secondSecret)
	second := managedTenantRecord{TenantID: "receipt-other", OwnerUserID: "owner", Name: "Receipt other", NameKey: "receipt-other", SecretDigest: &digest}
	if err := database.database.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	read(path+"/"+settled["request_id"].(string), "", secondSecret, http.StatusNotFound)
	read(path, "app-receipt", secondSecret, http.StatusNotFound)
	read(path, "", hostedIdentityFixtureKey, http.StatusBadRequest)
	read(path+"?unexpected=1", "app-receipt", hostedIdentityFixtureKey, http.StatusBadRequest)
	for _, scenario := range []struct {
		method, secret string
		status         int
	}{{http.MethodGet, "", http.StatusForbidden}, {http.MethodPost, hostedIdentityFixtureKey, http.StatusMethodNotAllowed}} {
		request, err := http.NewRequest(scenario.method, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if scenario.secret != "" {
			request.Header.Set("Authorization", "Bearer "+scenario.secret)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != scenario.status || response.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("auth/method response=%d", response.StatusCode)
		}
		if scenario.status == http.StatusMethodNotAllowed && response.Header.Get("Allow") != http.MethodGet {
			t.Fatal("missing method declaration")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("financial reads repeated provider execution")
	}
	// The same journal projection stays tenant-key authenticated; owner cookies are unnecessary.

}

func TestHostedFundsApplicationFinancialRecoveryStates(t *testing.T) {
	for _, calls := range []int64{0, 1} {
		t.Run(strconv.FormatInt(calls, 10), func(t *testing.T) {
			fixture := newFundsHoldRecoveryFixture(t, calls)
			recoverHostedTextExecution(t, fixture.fundsAdmissionFixture, fixture.request, calls)
			router, service, _ := newHostedIdentityHTTPHandler(t, fixture.database, fixture.upstreamURL, fixture.responseRoot, fundsDependencies(fixture.prices))
			registerApplicationFinancialRoutes(router, newTenantAuthenticator(service.store), zap.NewNop().Sugar(), fixture.database)
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			before := fixture.state(t)
			request, err := http.NewRequest(http.MethodGet, server.URL+llmproxycontract.FinancialRequestsPath+"/"+fixture.request.ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
			var result map[string]any
			if err := json.Unmarshal(body, &result); err != nil {
				t.Fatal(err)
			}
			want := "uncertain"
			if calls == 0 {
				want = "definitely_undispatched_released"
			}
			if result["state"] != want || result["receipt"] != nil {
				t.Fatalf("status=%v", result)
			}
			if !reflect.DeepEqual(before, fixture.state(t)) {
				t.Fatal("financial status read changed funds or reconciled work")
			}
		})
	}
}

func financialReadServer(t *testing.T, database, readDatabase *gormManagedTenantDatabase) *httptest.Server {
	t.Helper()
	service := newInternalManagementService(t, newFakeManagedTenantDatabase(), internalManagementProviderRegistry())
	service.store.database = database
	router := gin.New()
	router.Use(requestIdentifierHandler())
	registerApplicationFinancialRoutes(router, newTenantAuthenticator(service.store), zap.NewNop().Sugar(), readDatabase)
	registerClientMethodErrors(router)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server
}
func applicationFinancialHTTP(t *testing.T, server *httptest.Server, path, key string, want int) map[string]any {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, server.URL+llmproxycontract.FinancialRequestsPath+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+hostedIdentityFixtureKey)
	if key != "" {
		request.Header.Set(llmproxycontract.HeaderIdempotencyKey, key)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestHostedFundsApplicationFinancialReadFailures(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	hostedIdentityHTTP(t, fixture.generation, "financial-errors", "funded prompt", http.StatusOK)
	if err := deliverFundsFixture(t, fixture.database); err != nil {
		t.Fatal(err)
	}
	server := financialReadServer(t, fixture.database, fixture.database)
	applicationFinancialHTTP(t, server, "/invalid-id", "", http.StatusBadRequest)
	missing := financialReadServer(t, fixture.database, nil)
	applicationFinancialHTTP(t, missing, "", "financial-errors", http.StatusServiceUnavailable)
	for _, table := range []string{"managed_journal_request_records", "managed_funds_reservation_records", "managed_price_snapshot_records", "managed_funds_settlement_records"} {
		t.Run(table, func(t *testing.T) {
			callback := "financial:error"
			if err := fixture.database.database.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					tx.AddError(errors.New("controlled financial read failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			result := applicationFinancialHTTP(t, server, "", "financial-errors", http.StatusServiceUnavailable)
			if err := fixture.database.database.Callback().Query().Remove(callback); err != nil {
				t.Fatal(err)
			}
			if result["receipt"] != nil || result["maximum_cents"] != nil {
				t.Fatal("partial financial evidence escaped")
			}
		})
	}
	for _, state := range []string{"released", "invalid"} {
		t.Run(state, func(t *testing.T) {
			callback := "financial:invalid"
			if err := fixture.database.database.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if value, ok := tx.Statement.Dest.(*managedFundsReservationRecord); ok {
					value.State = state
				}
			}); err != nil {
				t.Fatal(err)
			}
			applicationFinancialHTTP(t, server, "", "financial-errors", http.StatusServiceUnavailable)
			if err := fixture.database.database.Callback().Query().Remove(callback); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestHostedFundsApplicationFinancialReleasedReadFailureAndHeldUncertainty(t *testing.T) {
	fixture := newFundsHoldRecoveryFixture(t, 0)
	recoverHostedTextExecution(t, fixture.fundsAdmissionFixture, fixture.request, 0)
	server := financialReadServer(t, fixture.database, fixture.database)
	callback := "financial:dispatch"
	if err := fixture.database.database.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "managed_journal_attempt_records" {
			tx.AddError(errors.New("controlled dispatch read failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	applicationFinancialHTTP(t, server, "/"+fixture.request.ID, "", http.StatusServiceUnavailable)
	if err := fixture.database.database.Callback().Query().Remove(callback); err != nil {
		t.Fatal(err)
	}
	uncertain := newFundsHoldRecoveryFixture(t, 1)
	if err := uncertain.database.recoverJournalDispatches(t.Context(), journalExecutionText, uncertain.request.ClaimExpiresAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	uncertainServer := financialReadServer(t, uncertain.database, uncertain.database)
	result := applicationFinancialHTTP(t, uncertainServer, "/"+uncertain.request.ID, "", http.StatusOK)
	if result["state"] != "uncertain" || result["reservation_state"] != "held" {
		t.Fatalf("unreconciled dispatch=%v", result)
	}
}
