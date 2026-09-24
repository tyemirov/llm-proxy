package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestHostedPaymentsRuntimeProcessesFundingThroughNormalService(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, _, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	var source struct{ File string }
	if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	configuration := withInternalUpstreamCapacity(t, Configuration{Management: service.configuration, ProviderCatalog: internalCanonicalProviderCatalog(), AssetStorePath: t.TempDir(), Payments: &PaymentConfiguration{
		Environment: "sandbox", ClientToken: "test_browserfixture", ProcessorAccountID: "processor-fixture", SupplierID: "supplier-fixture", APIKey: "checkout-fixture-key", APIBaseURL: processor.server.URL, WebhookSecret: paymentInboxTestSecret,
		Offers: []PaymentOfferConfiguration{{Code: "five", PriceID: "pri_01hv8x2axb33yr5y238zfwcn5p", FundingCents: 500}},
	}})
	configuration.Management.DatabasePath = source.File
	application, err := buildProxyApplicationForTest(t, configuration, zap.NewNop().Sugar(), newManagedTenantStore)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- application.serve(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-stopped:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("payment runtime did not stop")
		}
	})
	server := &httptest.Server{URL: "http://" + listener.Addr().String()}
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "runtime-funding", `{"offer_code":"five"}`, http.StatusCreated)
	public := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/funding-offers", "", "", http.StatusOK)
	if public["client_token"] != "test_browserfixture" || public["environment"] != "sandbox" {
		t.Fatalf("runtime checkout configuration=%v", public)
	}
	for _, private := range []string{"api_key", "webhook_secret", "processor_account_id", "supplier_id"} {
		if _, exposed := public[private]; exposed {
			t.Fatalf("private payment field exposed: %s", private)
		}
	}
	checkoutPath := paymentOrdersTestPath + "/" + order["id"].(string) + "/checkout"
	waitPaymentRuntime(t, func() bool { return processor.creates.Load() == 1 })
	waitPaymentRuntime(t, func() bool {
		var count int64
		if err := database.database.Model(&managedPaymentCheckoutRecord{}).Where("order_id = ?", order["id"]).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		return count == 1
	})
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, checkoutPath, "", "", http.StatusOK)
	transaction := completedPaymentFixture(t, processor)
	payload, err := json.Marshal(map[string]any{"event_id": fmt.Sprintf("evt_%026d", 1), "event_type": "transaction.completed", "occurred_at": "2026-09-23T12:00:00Z", "data": transaction})
	if err != nil {
		t.Fatal(err)
	}
	paymentInboxHTTP(t, server, string(payload), paymentInboxSignature(string(payload), paymentInboxTestSecret, time.Now()), http.StatusOK)
	waitPaymentRuntime(t, func() bool {
		balance := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
		return balance["available_cents"] == "500"
	})
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+order["id"].(string)+"/receipt", "", "", http.StatusOK)
	if processor.creates.Load() != 1 {
		t.Fatal("runtime repeated checkout creation")
	}
	// A production runtime cannot admit accounts funded by this sandbox database.
	production := *configuration.Payments
	production.Environment, production.APIBaseURL = "production", ""
	production.ClientToken = "live_browserfixture"
	configuration.Payments = &production
	if _, err := buildProxyApplicationForTest(t, configuration, zap.NewNop().Sugar(), newManagedTenantStore); err == nil || !strings.Contains(err.Error(), "payment environment") {
		t.Fatalf("sandbox database accepted production runtime: %v", err)
	}
}

func waitPaymentRuntime(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for !ready() {
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("payment runtime did not reach the expected state")
		}
	}
}

func TestHostedPaymentsRuntimeFinancialFailureStopsHTTPAndRecovers(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "runtime-recovery", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	transaction := completedPaymentFixture(t, processor)
	worker := paymentProcessorFixture(t, checkout, database)
	application := &proxyApplication{router: server.Config.Handler.(*gin.Engine), database: database, now: time.Now, payments: &paddlePaymentRuntime{checkout: checkout, processor: worker}}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- application.serve(ctx, listener) }()
	live := &httptest.Server{URL: "http://" + listener.Addr().String()}
	paymentOrderHTTP(t, live, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
	if err := database.database.Exec("CREATE TRIGGER reject_runtime_payment BEFORE INSERT ON managed_payment_receipt_records BEGIN SELECT RAISE(ABORT, 'controlled_runtime_payment_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	sendPaymentEventFixture(t, database, transaction, "transaction.completed", 1)
	select {
	case err := <-stopped:
		if err == nil || !strings.Contains(err.Error(), "controlled_runtime_payment_failure") {
			t.Fatalf("runtime financial failure=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("payment failure left admission running")
	}
	if connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
		connection.Close()
		t.Fatal("payment failure retained HTTP listener")
	}
	assertHostedFundsBalance(t, database, 0, 0)
	if err := database.database.Exec("DROP TRIGGER reject_runtime_payment").Error; err != nil {
		t.Fatal(err)
	}
	restarted, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { stopped <- application.serve(ctx, restarted) }()
	live.URL = "http://" + restarted.Addr().String()
	balance := paymentOrderHTTP(t, live, cookie("owner"), http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
	if balance["posted_cents"] != "500" {
		t.Fatalf("startup recovery did not credit funds: %v", balance)
	}
	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("payment runtime did not stop")
	}
	if processor.creates.Load() != 1 {
		t.Fatal("recovery repeated checkout")
	}
}

func TestHostedPaymentsRuntimeRejectsInvalidConfigurationBeforeOpeningDatabase(t *testing.T) {
	for _, scenario := range []string{"secret", "client-token", "client-environment", "client-whitespace", "environment", "minimum", "price", "plaintext-remote"} {
		t.Run(scenario, func(t *testing.T) {
			input := &PaymentConfiguration{Environment: "sandbox", ClientToken: "test_browserfixture", ProcessorAccountID: "processor", SupplierID: "supplier", APIKey: "fixture-key", WebhookSecret: "fixture-secret", Offers: []PaymentOfferConfiguration{{Code: "five", PriceID: "pri_01hv8x2axb33yr5y238zfwcn5p", FundingCents: 500}}}
			switch scenario {
			case "client-token":
				input.ClientToken = ""
			case "client-environment":
				input.ClientToken = "live_browserfixture"
			case "client-whitespace":
				input.ClientToken = "test_browser fixture"
			case "secret":
				input.WebhookSecret = ""
			case "environment":
				input.Environment = "unknown"
			case "minimum":
				input.Offers[0].FundingCents = 499
			case "price":
				input.Offers[0].PriceID = "client-choice"
			case "plaintext-remote":
				input.APIBaseURL = "http://processor.example"
			}
			configuration := withInternalUpstreamCapacity(t, Configuration{Management: managedRouterTestManagementConfiguration(), ProviderCatalog: internalCanonicalProviderCatalog(), AssetStorePath: t.TempDir(), Payments: input})
			_, err := buildProxyApplicationForTest(t, configuration, zap.NewNop().Sugar(), func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
				t.Fatal("invalid payment configuration opened the database")
				return nil, nil
			})
			if err == nil {
				t.Fatal("invalid payment configuration was accepted")
			}
		})
	}
}
