package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
)

func TestHostedPaymentsRuntimeProcessesFundingThroughNormalService(t *testing.T) {
	for _, scenario := range []struct {
		environment string
		host        string
		token       string
	}{
		{paymentEnvironmentSandbox, "sandbox-api.paddle.com", "test_browserfixture"},
		{paymentEnvironmentProduction, "api.paddle.com", "live_browserfixture"},
	} {
		t.Run(scenario.environment, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, _, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			fixtureOrigin, err := url.Parse(processor.server.URL)
			if err != nil {
				t.Fatal(err)
			}
			payments := &PaymentConfiguration{
				Environment: scenario.environment, CheckoutEnabled: true, CheckoutURL: "https://checkout.example/app/", ClientToken: scenario.token,
				ProcessorAccountID: "processor-fixture", SupplierID: "supplier-fixture", APIKey: "checkout-fixture-key", WebhookSecret: paymentInboxTestSecret,
				Offers: []PaymentOfferConfiguration{{Code: "five", PriceID: "pri_01hv8x2axb33yr5y238zfwcn5p", FundingCents: 500}},
				transport: paymentRuntimeTestTransport(func(request *http.Request) (*http.Response, error) {
					if request.URL.Scheme != "https" || request.URL.Host != scenario.host {
						t.Errorf("unexpected Paddle origin %s://%s, want https://%s", request.URL.Scheme, request.URL.Host, scenario.host)
						return nil, fmt.Errorf("unexpected Paddle origin")
					}
					forwarded := request.Clone(request.Context())
					forwarded.URL.Scheme = fixtureOrigin.Scheme
					forwarded.URL.Host = fixtureOrigin.Host
					forwarded.Host = fixtureOrigin.Host
					return processor.server.Client().Transport.RoundTrip(forwarded)
				}),
			}
			configuration := paymentRuntimeTestConfiguration(t, database, service.configuration, payments)
			server := startHostedPaymentsRuntime(t, configuration)
			owner := cookie("owner")
			order := paymentOrderHTTP(t, server, owner, http.MethodPost, paymentOrdersTestPath, "runtime-funding", `{"offer_code":"five"}`, http.StatusCreated)
			public := paymentOrderHTTP(t, server, owner, http.MethodGet, "/billing-accounts/billing-journal/funding-offers", "", "", http.StatusOK)
			if public["client_token"] != scenario.token || public["environment"] != scenario.environment {
				t.Fatalf("runtime checkout configuration=%v", public)
			}
			for _, private := range []string{"api_key", "webhook_secret", "processor_account_id", "supplier_id"} {
				if _, exposed := public[private]; exposed {
					t.Fatalf("private payment field exposed: %s", private)
				}
			}
			orderID := order["id"].(string)
			waitPaymentRuntime(t, func() bool {
				var count int64
				if err := database.database.Model(&managedPaymentCheckoutRecord{}).Where(&managedPaymentCheckoutRecord{OrderID: orderID}).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				return count == 1
			})
			checkout := paymentOrderHTTP(t, server, owner, http.MethodGet, paymentOrdersTestPath+"/"+orderID+"/checkout", "", "", http.StatusOK)
			if checkout["environment"] != scenario.environment {
				t.Fatalf("checkout environment=%v, want %s", checkout["environment"], scenario.environment)
			}
			assertHostedFundsBalance(t, database, 0, 0)
			transaction := completedPaymentFixture(t, processor)
			payload, err := json.Marshal(map[string]any{"event_id": fmt.Sprintf("evt_%026d", 1), "event_type": "transaction.completed", "occurred_at": "2026-09-23T12:00:00Z", "data": transaction})
			if err != nil {
				t.Fatal(err)
			}
			signature := paymentInboxSignature(string(payload), paymentInboxTestSecret, time.Now())
			paymentInboxHTTP(t, server, string(payload), signature, http.StatusOK)
			paymentInboxHTTP(t, server, string(payload), signature, http.StatusOK)
			waitPaymentRuntime(t, func() bool {
				balance := paymentOrderHTTP(t, server, owner, http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
				return balance["available_cents"] == "500"
			})
			receipt := paymentOrderHTTP(t, server, owner, http.MethodGet, paymentOrdersTestPath+"/"+orderID+"/receipt", "", "", http.StatusOK)
			if receipt["environment"] != scenario.environment || receipt["credit_cents"] != "500" || processor.creates.Load() != 1 {
				t.Fatalf("verified payment: environment=%v credit=%v creates=%d", receipt["environment"], receipt["credit_cents"], processor.creates.Load())
			}

			other := *payments
			other.Environment, other.ClientToken = paymentEnvironmentProduction, "live_browserfixture"
			if scenario.environment == paymentEnvironmentProduction {
				other.Environment, other.ClientToken = paymentEnvironmentSandbox, "test_browserfixture"
			}
			configuration.Payments = &other
			if _, err := buildProxyApplicationForTest(t, configuration, zap.NewNop().Sugar(), newManagedTenantStore); err == nil || !strings.Contains(err.Error(), "payment environment") {
				t.Fatalf("%s database accepted %s payments: %v", scenario.environment, other.Environment, err)
			}
		})
	}
}

type paymentRuntimeTestTransport func(*http.Request) (*http.Response, error)

func (transport paymentRuntimeTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func paymentRuntimeTestConfiguration(t *testing.T, database *gormManagedTenantDatabase, management ManagementConfiguration, payments *PaymentConfiguration) Configuration {
	t.Helper()
	dialector := database.database.Dialector.(*sqlite.Dialector)
	management.DatabaseDialector = dialector
	management.DatabasePath, _, _ = strings.Cut(dialector.DSN, "?")
	return withInternalUpstreamCapacity(t, Configuration{
		Management: management, ProviderCatalog: internalCanonicalProviderCatalog(), AssetStorePath: t.TempDir(), Payments: payments,
	})
}

func startHostedPaymentsRuntime(t *testing.T, configuration Configuration) *httptest.Server {
	t.Helper()
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
	return &httptest.Server{URL: "http://" + listener.Addr().String()}
}

func TestHostedPaymentsRuntimeKeepsFundingClosedWhileServingWebhooks(t *testing.T) {
	fixture := newPaymentStartupFixture(t)
	fixture.configuration.Payments.CheckoutEnabled = false
	cookie := fixture.cookie("owner")
	queued := paymentOrderHTTP(t, fixture.server, cookie, http.MethodPost, paymentOrdersTestPath, "queued-before-closure", `{"offer_code":"five"}`, http.StatusCreated)
	server := startHostedPaymentsRuntime(t, fixture.configuration)
	paymentOrderHTTP(t, server, cookie, http.MethodGet, "/billing-accounts/billing-journal/funding-offers", "", "", http.StatusServiceUnavailable)
	paymentOrderHTTP(t, server, cookie, http.MethodPost, paymentOrdersTestPath, "closed-checkout", `{"offer_code":"five"}`, http.StatusServiceUnavailable)
	adjustment := paymentAdjustmentFixture(fixture.processor, "approved", 200, 1)
	payload, err := json.Marshal(map[string]any{"event_id": fmt.Sprintf("evt_%026d", 2), "event_type": "adjustment.created", "occurred_at": "2026-09-23T12:00:02Z", "data": adjustment})
	if err != nil {
		t.Fatal(err)
	}
	paymentInboxHTTP(t, server, string(payload), "", http.StatusUnauthorized)
	paymentInboxHTTP(t, server, string(payload), paymentInboxSignature(string(payload), fixture.payments.WebhookSecret, time.Now()), http.StatusOK)
	waitPaymentRuntime(t, func() bool {
		balance := paymentOrderHTTP(t, server, cookie, http.MethodGet, fundsBalanceTestPath, "", "", http.StatusOK)
		return balance["posted_cents"] == "300"
	})
	queuedPath := paymentOrdersTestPath + "/" + queued["id"].(string)
	retained := paymentOrderHTTP(t, server, cookie, http.MethodGet, queuedPath, "", "", http.StatusOK)
	paymentOrderHTTP(t, server, cookie, http.MethodGet, queuedPath+"/checkout", "", "", http.StatusNotFound)
	if retained["state"] != fundingOrderCreated || fixture.processor.creates.Load() != 1 {
		t.Fatalf("closed checkout dispatched queued order: order=%v creates=%d", retained, fixture.processor.creates.Load())
	}
}

func TestHostedPaymentsRuntimeKeepsRequestsOnSelectedOrigin(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		method string
		status int
		reason string
	}{
		{"price-read", http.MethodGet, http.StatusFound, "price_unavailable"},
		{"temporary-create-redirect", http.MethodPost, http.StatusTemporaryRedirect, "creation_outcome_unknown"},
		{"permanent-create-redirect", http.MethodPost, http.StatusPermanentRedirect, "creation_outcome_unknown"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, _, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			var redirected, selected atomic.Int64
			otherOrigin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				redirected.Add(1)
				writer.WriteHeader(http.StatusBadRequest)
			}))
			t.Cleanup(otherOrigin.Close)
			selectedOrigin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				priceRead := request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/prices/")
				transactionCreate := request.Method == http.MethodPost && request.URL.Path == "/transactions"
				if scenario.method == http.MethodGet && priceRead || scenario.method == http.MethodPost && transactionCreate {
					selected.Add(1)
					http.Redirect(writer, request, otherOrigin.URL, scenario.status)
					return
				}
				processor.server.Config.Handler.ServeHTTP(writer, request)
			}))
			t.Cleanup(selectedOrigin.Close)
			configuration := paymentRuntimeTestConfiguration(t, database, service.configuration, &PaymentConfiguration{
				Environment: paymentEnvironmentSandbox, CheckoutEnabled: true, CheckoutURL: "https://checkout.example/app/", ClientToken: "test_browserfixture",
				ProcessorAccountID: "processor-fixture", SupplierID: "supplier-fixture", APIKey: "checkout-fixture-key", APIBaseURL: selectedOrigin.URL,
				WebhookSecret: paymentInboxTestSecret, Offers: []PaymentOfferConfiguration{{Code: "five", PriceID: "pri_01hv8x2axb33yr5y238zfwcn5p", FundingCents: 500}},
			})
			server := startHostedPaymentsRuntime(t, configuration)
			order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "redirected-checkout", `{"offer_code":"five"}`, http.StatusCreated)
			delivery := managedPaymentDeliveryRecord{OrderID: order["id"].(string)}
			waitPaymentRuntime(t, func() bool {
				if err := database.database.First(&delivery).Error; err != nil {
					t.Fatal(err)
				}
				return delivery.Reason == scenario.reason
			})
			if selected.Load() != 1 || redirected.Load() != 0 {
				t.Fatalf("Paddle origin requests: selected=%d redirected=%d, want one selected and zero redirected", selected.Load(), redirected.Load())
			}
			if scenario.method == http.MethodPost {
				if err := database.database.Model(&delivery).UpdateColumn("retry_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
					t.Fatal(err)
				}
				waitPaymentRuntime(t, func() bool {
					if err := database.database.First(&delivery).Error; err != nil {
						t.Fatal(err)
					}
					return delivery.Reason == "transaction_unresolved"
				})
				if selected.Load() != 1 || redirected.Load() != 0 {
					t.Fatalf("checkout recovery repeated creation: selected=%d redirected=%d", selected.Load(), redirected.Load())
				}
			}
			paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+delivery.OrderID+"/checkout", "", "", http.StatusNotFound)
			assertHostedFundsBalance(t, database, 0, 0)
		})
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
	application := &proxyApplication{closeStore: func() error { return nil }, router: server.Config.Handler.(*gin.Engine), database: database, now: time.Now, payments: &paddlePaymentRuntime{checkout: checkout, processor: worker}}
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
	for _, scenario := range []string{"secret", "secret-whitespace", "api-key", "api-key-whitespace", "client-token", "client-environment", "client-whitespace", "environment", "minimum", "price", "plaintext-remote", "sandbox-remote", "production-override", "production-loopback-tls", "local-api-path", "local-api-empty-query", "local-api-empty-fragment", "checkout-missing", "checkout-http", "checkout-relative", "checkout-user", "checkout-query", "checkout-fragment", "checkout-empty-fragment", "checkout-escape"} {
		t.Run(scenario, func(t *testing.T) {
			input := &PaymentConfiguration{Environment: "sandbox", CheckoutURL: "https://checkout.example/app/", ClientToken: "test_browserfixture", ProcessorAccountID: "processor", SupplierID: "supplier", APIKey: "fixture-key", WebhookSecret: "fixture-secret", Offers: []PaymentOfferConfiguration{{Code: "five", PriceID: "pri_01hv8x2axb33yr5y238zfwcn5p", FundingCents: 500}}}
			switch scenario {
			case "checkout-missing":
				input.CheckoutURL = ""
			case "checkout-http":
				input.CheckoutURL = "http://checkout.example/app/"
			case "checkout-relative":
				input.CheckoutURL = "/app/"
			case "checkout-user":
				input.CheckoutURL = "https://user:secret@checkout.example/app/"
			case "checkout-query":
				input.CheckoutURL = "https://checkout.example/app/?funding_order_id=untrusted"
			case "checkout-fragment":
				input.CheckoutURL = "https://checkout.example/app/#fragment"
			case "checkout-empty-fragment":
				input.CheckoutURL = "https://checkout.example/app/#"
			case "checkout-escape":
				input.CheckoutURL = "https://%zz.example/app/"
			case "client-token":
				input.ClientToken = ""
			case "client-environment":
				input.ClientToken = "live_browserfixture"
			case "client-whitespace":
				input.ClientToken = "test_browser fixture"
			case "secret":
				input.WebhookSecret = ""
			case "secret-whitespace":
				input.WebhookSecret = " \t "
			case "api-key":
				input.APIKey = ""
			case "api-key-whitespace":
				input.APIKey = " \t "
			case "environment":
				input.Environment = "unknown"
			case "minimum":
				input.Offers[0].FundingCents = 499
			case "price":
				input.Offers[0].PriceID = "client-choice"
			case "plaintext-remote":
				input.APIBaseURL = "http://processor.example"
			case "sandbox-remote":
				input.APIBaseURL = "https://processor.example"
			case "production-override":
				input.Environment, input.ClientToken = "production", "live_browserfixture"
				input.APIBaseURL = "https://sandbox-api.paddle.com"
			case "production-loopback-tls":
				input.Environment, input.ClientToken = "production", "live_browserfixture"
				input.APIBaseURL = "https://127.0.0.1:443"
			case "local-api-path":
				input.APIBaseURL = "http://127.0.0.1:8080/processor"
			case "local-api-empty-query":
				input.APIBaseURL = "http://127.0.0.1:8080?"
			case "local-api-empty-fragment":
				input.APIBaseURL = "http://127.0.0.1:8080#"
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
