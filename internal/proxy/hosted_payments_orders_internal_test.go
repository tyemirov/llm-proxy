package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const paymentOrdersTestPath = "/billing-accounts/billing-journal/funding-orders"

func paymentOrdersFixture(t *testing.T, database *gormManagedTenantDatabase) (*managementService, *httptest.Server, func(string) *http.Cookie) {
	t.Helper()
	service := newInternalManagementService(t, newFakeManagedTenantDatabase(), internalManagementProviderRegistry())
	service.store.database = database
	catalog, err := newFundingCatalog("sandbox", "processor-fixture", "supplier-fixture", []fundingOfferInput{
		{Code: "five", PriceID: "pri_01hv8x2axb33yr5y238zfwcn5p", FundingCents: 500},
		{Code: "ten", PriceID: "pri_01hv8x2axb33yr5y238zfwcn5q", FundingCents: 1000},
	})
	if err != nil {
		t.Fatal(err)
	}
	service.funding = catalog
	server, cookie := fundsManagementServiceHTTPFixture(t, service)
	return service, server, cookie
}

func paymentOrderHTTP(t *testing.T, server *httptest.Server, cookie *http.Cookie, method, path, key, body string, status int) map[string]any {
	t.Helper()
	request, err := http.NewRequest(method, server.URL+managementAPIPath+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if key != "" {
		request.Header.Set(managementIdempotencyHeader, key)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Origin", "http://localhost:8080")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.StatusCode, status, payload)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("payment resource permits caching")
	}
	template := "/api/management/billing-accounts/{billing_account_id}/funding-orders"
	resourcePath := strings.SplitN(path, "?", 2)[0]
	if strings.HasSuffix(resourcePath, "/balance") {
		template = "/api/management/billing-accounts/{billing_account_id}/balance"
	} else if strings.HasSuffix(resourcePath, "/ledger-entries") {
		template = "/api/management/billing-accounts/{billing_account_id}/ledger-entries"
	} else if strings.HasSuffix(resourcePath, "/funding-offers") {
		template = "/api/management/billing-accounts/{billing_account_id}/funding-offers"
	} else if strings.HasSuffix(resourcePath, "/payment-portal-sessions") {
		template = "/api/management/billing-accounts/{billing_account_id}/payment-portal-sessions"
	} else if strings.Contains(resourcePath, "/funding-orders/") {
		template += "/{order_id}"
		if strings.HasSuffix(resourcePath, "/checkout") {
			template += "/checkout"
		} else if strings.HasSuffix(resourcePath, "/receipt") {
			template += "/receipt"
		}
	}
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateResponse(template, method, status, response.Header, payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) == 0 {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	if status == http.StatusCreated && strings.HasSuffix(resourcePath, "/payment-portal-sessions") {
		if response.Header.Get("Location") != result["url"] {
			t.Fatalf("portal location=%s", response.Header.Get("Location"))
		}
	} else if status == http.StatusCreated && response.Header.Get("Location") != managementAPIPath+paymentOrdersTestPath+"/"+result["id"].(string) {
		t.Fatalf("location=%s", response.Header.Get("Location"))
	}
	return result
}

func TestHostedPaymentsOrdersRetainServerOfferAndOneDeliveryAcrossRestart(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	offers := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/funding-offers", "", "", http.StatusOK)
	if len(offers["offers"].([]any)) != 2 {
		t.Fatalf("offers=%v", offers)
	}
	body := `{"offer_code":"five"}`
	created := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "order-one", body, http.StatusCreated)
	if created["funding_cents"] != "500" || created["currency"] != "USD" || created["state"] != "created" || created["offer_code"] != "five" {
		t.Fatalf("order=%v", created)
	}
	for _, field := range []string{"price_id", "processor_account_id", "supplier_id", "idempotency_key", "creation_key_digest"} {
		if _, exists := created[field]; exists {
			t.Fatalf("private field %s exposed", field)
		}
	}
	service.funding.offers = map[string]fundingOfferInput{}
	replay := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "order-one", body, http.StatusCreated)
	if !reflect.DeepEqual(created, replay) {
		t.Fatalf("replay changed order: %v", replay)
	}
	_, restarted, restartedCookie := paymentOrdersFixture(t, openJournalTransactionInstance(t, database))
	read := paymentOrderHTTP(t, restarted, restartedCookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+created["id"].(string), "", "", http.StatusOK)
	if !reflect.DeepEqual(created, read) {
		t.Fatalf("restart changed order: %v", read)
	}
	paymentOrderHTTP(t, restarted, restartedCookie("owner"), http.MethodPost, paymentOrdersTestPath, "order-one", `{"offer_code":"ten"}`, http.StatusConflict)
	var order managedFundingOrderRecord
	if err := database.database.First(&order).Error; err != nil {
		t.Fatal(err)
	}
	if order.PriceID != "pri_01hv8x2axb33yr5y238zfwcn5p" || order.FundingCents != 500 || order.SupplierID != "supplier-fixture" || order.Environment != "sandbox" {
		t.Fatalf("lost snapshot: %+v", order)
	}
	var deliveries []managedPaymentDeliveryRecord
	if err := database.database.Find(&deliveries).Error; err != nil || len(deliveries) != 1 || deliveries[0].OrderID != order.ID || deliveries[0].State != "pending" {
		t.Fatalf("delivery=%v error=%v", deliveries, err)
	}
	assertHostedFundsBalance(t, database, 0, 0)
}

func TestHostedPaymentsOrdersRejectUnownedAndClientFinancialValues(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	_, server, cookie := paymentOrdersFixture(t, database)
	for _, body := range []string{`{}`, `{"offer_code":"unknown"}`, `{"offer_code":"five","funding_cents":"1"}`, `{"offer_code":"five","currency":"EUR"}`, `{"offer_code":"five","return_url":"https://attacker.example"}`} {
		paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "invalid-order", body, http.StatusBadRequest)
	}
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "", `{"offer_code":"five"}`, http.StatusBadRequest)
	paymentOrderHTTP(t, server, cookie("other"), http.MethodPost, paymentOrdersTestPath, "other-order", `{"offer_code":"five"}`, http.StatusNotFound)
	paymentOrderHTTP(t, server, nil, http.MethodPost, paymentOrdersTestPath, "no-session", `{"offer_code":"five"}`, http.StatusUnauthorized)
	created := paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "owned-order", `{"offer_code":"five"}`, http.StatusCreated)
	paymentOrderHTTP(t, server, cookie("other"), http.MethodGet, paymentOrdersTestPath+"/"+created["id"].(string), "", "", http.StatusNotFound)
}

func TestHostedPaymentsOrdersCommitDeliveryAtomically(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	_, server, cookie := paymentOrdersFixture(t, database)
	if err := database.database.Exec("CREATE TRIGGER reject_checkout_delivery BEFORE INSERT ON managed_payment_delivery_records BEGIN SELECT RAISE(ABORT, 'controlled_delivery_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "retry-order", `{"offer_code":"five"}`, http.StatusServiceUnavailable)
	var count int64
	if err := database.database.Model(&managedFundingOrderRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial order count=%d error=%v", count, err)
	}
	if err := database.database.Exec("DROP TRIGGER reject_checkout_delivery").Error; err != nil {
		t.Fatal(err)
	}
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "retry-order", `{"offer_code":"five"}`, http.StatusCreated)
}

func TestHostedPaymentsOrdersConcurrentCreationHasOneEffect(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	_, first, cookie := paymentOrdersFixture(t, database)
	_, second, _ := paymentOrdersFixture(t, openJournalTransactionInstance(t, database))
	var group sync.WaitGroup
	results := make([]map[string]any, 6)
	for index := range results {
		group.Add(1)
		go func() {
			defer group.Done()
			results[index] = paymentOrderHTTP(t, []*httptest.Server{first, second}[index%2], cookie("owner"), http.MethodPost, paymentOrdersTestPath, "concurrent-order", `{"offer_code":"five"}`, http.StatusCreated)
		}()
	}
	group.Wait()
	for _, result := range results {
		if !reflect.DeepEqual(result, results[0]) {
			t.Fatalf("different orders: %v", results)
		}
	}
	for _, model := range []any{&managedFundingOrderRecord{}, &managedPaymentDeliveryRecord{}} {
		var count int64
		if err := database.database.Model(model).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("%T count=%d error=%v", model, count, err)
		}
	}
}

func TestHostedPaymentsOrdersHistoryAndDisabledFunding(t *testing.T) {
	database, _, _ := newJournalTransactionFixture(t)
	service, server, cookie := paymentOrdersFixture(t, database)
	for _, key := range []string{"history-first", "history-second", "history-third"} {
		paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, key, `{"offer_code":"five"}`, http.StatusCreated)
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		path := paymentOrdersTestPath + "?limit=1"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		page := paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, path, "", "", http.StatusOK)
		for _, value := range page["orders"].([]any) {
			id := value.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("duplicate order %s", id)
			}
			seen[id] = true
		}
		cursor = page["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	if len(seen) != 3 {
		t.Fatalf("history count=%d", len(seen))
	}
	paymentOrderHTTP(t, server, cookie("other"), http.MethodGet, paymentOrdersTestPath, "", "", http.StatusNotFound)
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"?limit=101", "", "", http.StatusBadRequest)
	service.funding = nil
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/funding-offers", "", "", http.StatusServiceUnavailable)
	paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "disabled-order", `{"offer_code":"five"}`, http.StatusServiceUnavailable)
	for id := range seen {
		paymentOrderHTTP(t, server, cookie("owner"), http.MethodGet, paymentOrdersTestPath+"/"+id, "", "", http.StatusOK)
	}
}

func TestHostedPaymentsCatalogRejectsBelowMinimumAndAmbiguousOffers(t *testing.T) {
	for _, offers := range [][]fundingOfferInput{
		nil,
		{{Code: "small", PriceID: "pri_01hv8x2axb33yr5y238zfwcn5p", FundingCents: 499}},
		{{Code: "five", PriceID: "pri_01hv8x2axb33yr5y238zfwcn5p", FundingCents: 500}, {Code: "five", PriceID: "pri_01hv8x2axb33yr5y238zfwcn5q", FundingCents: 1000}},
	} {
		if _, err := newFundingCatalog("sandbox", "processor-fixture", "supplier-fixture", offers); err == nil {
			t.Fatal("invalid funding catalog accepted")
		}
	}
}

func TestHostedPaymentsOrdersDoNotReplayAcrossProcessorBoundaries(t *testing.T) {
	for _, boundary := range []string{"environment", "processor", "supplier"} {
		t.Run(boundary, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service, server, cookie := paymentOrdersFixture(t, database)
			paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "scoped-order", `{"offer_code":"five"}`, http.StatusCreated)
			switch boundary {
			case "environment":
				service.funding.environment = paymentEnvironmentProduction
			case "processor":
				service.funding.processorAccountID = "processor-other"
			case "supplier":
				service.funding.supplierID = "supplier-other"
			}
			paymentOrderHTTP(t, server, cookie("owner"), http.MethodPost, paymentOrdersTestPath, "scoped-order", `{"offer_code":"five"}`, http.StatusConflict)
		})
	}
}
