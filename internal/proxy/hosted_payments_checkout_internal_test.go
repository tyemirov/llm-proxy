package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/utils/billing"
)

type checkoutProtocolFixture struct {
	server       *httptest.Server
	mutex        sync.Mutex
	transactions []map[string]any
	adjustments  []map[string]any
	creates      atomic.Int64
	portalCalls  atomic.Int64
	portalURL    string
	dropResponse atomic.Bool
	wrongAccount atomic.Bool
	recurring    atomic.Bool
	priceStatus  atomic.Int64
}

func newCheckoutProtocolFixture(t *testing.T) *checkoutProtocolFixture {
	t.Helper()
	fixture := &checkoutProtocolFixture{portalURL: "https://customer-portal.paddle.com/fixture?token=temporary"}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fixture.mutex.Lock()
		defer fixture.mutex.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		if request.Header.Get("Authorization") != "Bearer checkout-fixture-key" {
			t.Error("shared Paddle authentication absent")
		}
		encode := func(value any) {
			if err := json.NewEncoder(writer).Encode(value); err != nil {
				t.Error(err)
			}
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/prices/pri_01hv8x2axb33yr5y238zfwcn5p":
			if status := fixture.priceStatus.Load(); status != 0 {
				writer.WriteHeader(int(status))
				encode(map[string]any{"error": map[string]any{"code": "price_temporarily_unavailable"}})
				return
			}
			var cycle any
			if fixture.recurring.Load() {
				cycle = map[string]any{"interval": "month", "frequency": 1}
			}
			encode(map[string]any{"data": map[string]any{"id": "pri_01hv8x2axb33yr5y238zfwcn5p", "billing_cycle": cycle, "unit_price": map[string]any{"amount": "500", "currency_code": "USD"}}})
		case request.Method == http.MethodPost && request.URL.Path == "/customers/ctm_01hv8x2axb33yr5y238zfwcn5p/portal-sessions":
			fixture.portalCalls.Add(1)
			writer.WriteHeader(http.StatusCreated)
			encode(map[string]any{"data": map[string]any{"urls": map[string]any{"general": map[string]any{"overview": fixture.portalURL}}}})
		case request.Method == http.MethodGet && request.URL.Path == "/adjustments":
			if request.URL.Query().Get("transaction_id") != "txn_00000000000000000000000001" {
				t.Error("unscoped adjustment read")
			}
			encode(map[string]any{"data": fixture.adjustments, "meta": map[string]any{"pagination": map[string]any{"has_more": false}}})
		case request.Method == http.MethodGet && request.URL.Path == "/customers":
			if request.URL.Query().Get("email") == "" {
				t.Error("customer lookup lacks owner email")
			}
			encode(map[string]any{"data": []any{map[string]any{"id": "ctm_01hv8x2axb33yr5y238zfwcn5p"}}, "meta": map[string]any{"pagination": map[string]any{"has_more": false}}})
		case request.Method == http.MethodPost && request.URL.Path == "/transactions":
			var input struct {
				CustomerID string            `json:"customer_id"`
				Metadata   map[string]string `json:"custom_data"`
				Items      []struct {
					PriceID  string `json:"price_id"`
					Quantity int64  `json:"quantity"`
				} `json:"items"`
			}
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Error(err)
				return
			}
			if input.Metadata["funding_order_id"] == "" || input.Metadata["billing_account_id"] != "billing-journal" || input.CustomerID != "ctm_01hv8x2axb33yr5y238zfwcn5p" || len(input.Items) != 1 || input.Items[0].Quantity != 1 {
				t.Errorf("unbound checkout: %+v", input)
			}
			id := fmt.Sprintf("txn_%026d", fixture.creates.Add(1))
			if fixture.wrongAccount.Load() {
				input.Metadata["billing_account_id"] = "billing-other"
			}
			transaction := map[string]any{"id": id, "status": "ready", "currency_code": "USD", "collection_mode": "automatic", "customer_id": input.CustomerID, "custom_data": input.Metadata, "items": []any{map[string]any{"quantity": 1, "price": map[string]any{"id": input.Items[0].PriceID, "unit_price": map[string]any{"amount": "500", "currency_code": "USD"}}}}}
			if fixture.recurring.Load() {
				transaction["items"].([]any)[0].(map[string]any)["price"].(map[string]any)["billing_cycle"] = map[string]any{"interval": "month", "frequency": 1}
			}
			fixture.transactions = append(fixture.transactions, transaction)
			if fixture.dropResponse.Load() {
				connection, _, err := writer.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				if err := connection.Close(); err != nil {
					t.Error(err)
				}
				return
			}
			encode(map[string]any{"data": map[string]any{"id": id}})
		case request.Method == http.MethodGet && request.URL.Path == "/transactions":
			if request.URL.Query().Get("customer_id") != "ctm_01hv8x2axb33yr5y238zfwcn5p" {
				t.Error("unscoped checkout recovery")
			}
			encode(map[string]any{"data": fixture.transactions, "meta": map[string]any{"pagination": map[string]any{"has_more": false}}})
		case request.Method == http.MethodGet:
			for _, transaction := range fixture.transactions {
				if request.URL.Path == "/transactions/"+transaction["id"].(string) {
					encode(map[string]any{"data": transaction})
					return
				}
			}
			writer.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected processor request: %s %s", request.Method, request.URL)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func checkoutWorkerFixture(t *testing.T, database *gormManagedTenantDatabase, catalog *fundingCatalog, processor *checkoutProtocolFixture, now time.Time) *paddleCheckoutDelivery {
	t.Helper()
	client, err := billing.NewPaddleCommerceClient("sandbox", "checkout-fixture-key", processor.server.URL, processor.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	worker := newPaddleCheckoutDelivery(database, catalog, client)
	worker.now = func() time.Time { return now }
	return worker
}

func TestHostedPaymentsCheckoutBindsOneTransactionAcrossWorkers(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	created := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "checkout-one", `{"offer_code":"five"}`, http.StatusCreated)
	path := paymentOrdersTestPath + "/" + created["id"].(string) + "/checkout"
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusNotFound)
	workers := []*paddleCheckoutDelivery{checkoutWorkerFixture(t, database, service.funding, processor, time.Now()), checkoutWorkerFixture(t, openJournalTransactionInstance(t, database), service.funding, processor, time.Now())}
	var group sync.WaitGroup
	for _, worker := range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := worker.reconcile(t.Context()); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	checkout := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
	if checkout["provider"] != "paddle" || checkout["environment"] != "sandbox" || checkout["transaction_id"] != "txn_00000000000000000000000001" || processor.creates.Load() != 1 {
		t.Fatalf("checkout=%v creates=%d", checkout, processor.creates.Load())
	}
	paymentOrderHTTP(t, server, cookie("other"), http.MethodGet, path, "", "", http.StatusNotFound)
	if err := workers[1].reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if processor.creates.Load() != 1 {
		t.Fatal("checkout replayed")
	}
	var customers []managedPaymentCustomerRecord
	if err := database.database.Find(&customers).Error; err != nil || len(customers) != 1 || customers[0].BillingAccountID != "billing-journal" {
		t.Fatalf("customer links=%v error=%v", customers, err)
	}
	order := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+created["id"].(string), "", "", http.StatusOK)
	if order["state"] != "pending" {
		t.Fatalf("order=%v", order)
	}
	assertHostedFundsBalance(t, database, 0, 0)
}

func TestHostedPaymentsCheckoutRecoversLostResponseWithoutAnotherPOST(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	processor.dropResponse.Store(true)
	created := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "lost-checkout", `{"offer_code":"five"}`, http.StatusCreated)
	now := time.Now()
	worker := checkoutWorkerFixture(t, database, service.funding, processor, now)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	path := paymentOrdersTestPath + "/" + created["id"].(string) + "/checkout"
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusNotFound)
	restarted := checkoutWorkerFixture(t, openJournalTransactionInstance(t, database), service.funding, processor, now.Add(2*time.Minute))
	if err := restarted.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
	if processor.creates.Load() != 1 {
		t.Fatalf("uncertain POST repeated: %d", processor.creates.Load())
	}
	assertHostedFundsBalance(t, database, 0, 0)
}

func TestHostedPaymentsCheckoutRejectsForeignProcessorEvidence(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	processor.wrongAccount.Store(true)
	created := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "foreign-checkout", `{"offer_code":"five"}`, http.StatusCreated)
	worker := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+created["id"].(string)+"/checkout", "", "", http.StatusNotFound)
	var delivery managedPaymentDeliveryRecord
	if err := database.database.First(&delivery).Error; err != nil || delivery.State != "reconciliation_required" {
		t.Fatalf("delivery=%+v error=%v", delivery, err)
	}
	assertHostedFundsBalance(t, database, 0, 0)
}

func TestHostedPaymentsCheckoutRejectsRecurringPriceBeforeDispatch(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	processor.recurring.Store(true)
	created := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "recurring-price", `{"offer_code":"five"}`, http.StatusCreated)
	worker := checkoutWorkerFixture(t, database, service.funding, processor, time.Now())
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+created["id"].(string)+"/checkout", "", "", http.StatusNotFound)
	if processor.creates.Load() != 0 {
		t.Fatal("recurring funding price created a processor transaction")
	}
	assertHostedFundsBalance(t, database, 0, 0)
}

func TestHostedPaymentsCheckoutRetriesPriceReadWithoutTransactionIntent(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	processor.priceStatus.Store(http.StatusServiceUnavailable)
	created := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "unavailable-price", `{"offer_code":"five"}`, http.StatusCreated)
	now := time.Now()
	worker := checkoutWorkerFixture(t, database, service.funding, processor, now)
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	checkoutPath := paymentOrdersTestPath + "/" + created["id"].(string) + "/checkout"
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, checkoutPath, "", "", http.StatusNotFound)
	var delivery managedPaymentDeliveryRecord
	if err := database.database.First(&delivery).Error; err != nil || delivery.State != paymentDeliveryPending || delivery.TransactionDispatchedAt != nil || processor.creates.Load() != 0 {
		t.Fatalf("price failure retained a transaction intent: state=%s dispatched=%v creates=%d error=%v", delivery.State, delivery.TransactionDispatchedAt, processor.creates.Load(), err)
	}
	processor.priceStatus.Store(0)
	worker.now = func() time.Time { return now.Add(2 * time.Minute) }
	if err := worker.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, checkoutPath, "", "", http.StatusOK)
	if processor.creates.Load() != 1 {
		t.Fatalf("recovery created %d transactions", processor.creates.Load())
	}
	assertHostedFundsBalance(t, database, 0, 0)
}

func TestHostedPaymentsCheckoutRecoversAfterResponsePersistenceFailure(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	created := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "checkout-write-failure", `{"offer_code":"five"}`, http.StatusCreated)
	if err := database.database.Exec("CREATE TRIGGER reject_checkout_receipt BEFORE INSERT ON managed_payment_checkout_records BEGIN SELECT RAISE(ABORT, 'controlled_checkout_receipt_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	worker := checkoutWorkerFixture(t, database, service.funding, processor, now)
	if err := worker.reconcile(t.Context()); err == nil {
		t.Fatal("lost checkout receipt write failure")
	}
	path := paymentOrdersTestPath + "/" + created["id"].(string) + "/checkout"
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusNotFound)
	var delivery managedPaymentDeliveryRecord
	if err := database.database.First(&delivery).Error; err != nil || delivery.State != paymentDeliveryDispatching || delivery.TransactionDispatchedAt == nil {
		t.Fatalf("dispatch evidence lost: %+v error=%v", delivery, err)
	}
	if err := database.database.Exec("DROP TRIGGER reject_checkout_receipt").Error; err != nil {
		t.Fatal(err)
	}
	restarted := checkoutWorkerFixture(t, openJournalTransactionInstance(t, database), service.funding, processor, now.Add(2*time.Minute))
	if err := restarted.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
	if processor.creates.Load() != 1 {
		t.Fatalf("recovery repeated payment creation: %d", processor.creates.Load())
	}
	assertHostedFundsBalance(t, database, 0, 0)
}

func TestHostedPaymentsCheckoutRetainsAmbiguousTransactions(t *testing.T) {
	for _, evidence := range []string{"missing", "duplicate"} {
		t.Run(evidence, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			processor := newCheckoutProtocolFixture(t)
			processor.dropResponse.Store(true)
			created := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "ambiguous-checkout", `{"offer_code":"five"}`, http.StatusCreated)
			now := time.Now()
			worker := checkoutWorkerFixture(t, database, service.funding, processor, now)
			if err := worker.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			processor.mutex.Lock()
			if evidence == "missing" {
				processor.transactions = nil
			} else {
				duplicate := make(map[string]any, len(processor.transactions[0]))
				for key, value := range processor.transactions[0] {
					duplicate[key] = value
				}
				duplicate["id"] = "txn_00000000000000000000000002"
				processor.transactions = append(processor.transactions, duplicate)
			}
			processor.mutex.Unlock()
			restarted := checkoutWorkerFixture(t, openJournalTransactionInstance(t, database), service.funding, processor, now.Add(2*time.Minute))
			if err := restarted.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+created["id"].(string)+"/checkout", "", "", http.StatusNotFound)
			var delivery managedPaymentDeliveryRecord
			if err := database.database.First(&delivery).Error; err != nil || delivery.State != paymentDeliveryReconciliation || delivery.Reason != "transaction_unresolved" {
				t.Fatalf("uncertainty lost: %+v error=%v", delivery, err)
			}
			if processor.creates.Load() != 1 {
				t.Fatalf("uncertain creation repeated: %d", processor.creates.Load())
			}
		})
	}
}
