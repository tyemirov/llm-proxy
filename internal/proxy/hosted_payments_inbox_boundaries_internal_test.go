package proxy

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func paymentInboxBoundaryResponse(t *testing.T, request *http.Request, response *http.Response, status int) {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != status || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("inbox response status=%d want=%d body=%s error=%v", response.StatusCode, status, body, err)
	}
	if strings.Contains(string(body), "controlled_") || strings.Contains(string(body), "private-payment") {
		t.Fatal("inbox response exposed private failure details")
	}
	validateHostedIdentityResponse(t, request, response, body)
}

func retainedPaymentInbox(t *testing.T, database *gormManagedTenantDatabase) []managedPaymentInboxRecord {
	t.Helper()
	var records []managedPaymentInboxRecord
	if err := database.database.Order("id").Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	return records
}

func TestHostedPaymentsInboxRejectsMalformedHTTPWithoutFinancialEffects(t *testing.T) {
	fixture := newPaymentAuditFixture(t)
	server := paymentInboxTestServer(t, fixture.database, "sandbox", "processor-fixture")
	before := paymentAdjustmentResources(t, fixture)
	retained := retainedPaymentInbox(t, fixture.database)
	data := `{"id":"txn_01hv8x2axb33yr5y238zfwcn5p"}`
	for _, scenario := range []struct {
		name, contentType, encoding, query, data, occurredAt string
		signatures, status                                   int
	}{
		{name: "missing-content-type", data: data, status: http.StatusUnsupportedMediaType},
		{name: "wrong-content-type", contentType: "text/plain", data: data, status: http.StatusUnsupportedMediaType},
		{name: "malformed-content-type", contentType: "application/json; charset", data: data, status: http.StatusUnsupportedMediaType},
		{name: "query", contentType: "application/json", query: "account=other", data: data, status: http.StatusBadRequest},
		{name: "compressed-body", contentType: "application/json", encoding: "gzip", data: data, status: http.StatusBadRequest},
		{name: "duplicate-signature", contentType: "application/json", data: data, signatures: 2, status: http.StatusUnauthorized},
		{name: "null-data", contentType: "application/json", data: "null", status: http.StatusBadRequest},
		{name: "array-data", contentType: "application/json", data: "[]", status: http.StatusBadRequest},
		{name: "string-data", contentType: "application/json", data: `"private-payment"`, status: http.StatusBadRequest},
		{name: "number-data", contentType: "application/json", data: "123", status: http.StatusBadRequest},
		{name: "boolean-data", contentType: "application/json", data: "true", status: http.StatusBadRequest},
		{name: "missing-entity", contentType: "application/json", data: `{}`, status: http.StatusBadRequest},
		{name: "null-entity", contentType: "application/json", data: `{"id":null}`, status: http.StatusBadRequest},
		{name: "numeric-entity", contentType: "application/json", data: `{"id":123}`, status: http.StatusBadRequest},
		{name: "invalid-entity", contentType: "application/json", data: `{"id":"private-payment"}`, status: http.StatusBadRequest},
		{name: "unrepresentable-utc-time", contentType: "application/json", data: data, occurredAt: "0000-01-01T00:00:00+01:00", status: http.StatusBadRequest},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal([]byte(paymentInboxFixtureEvent()), &envelope); err != nil {
				t.Fatal(err)
			}
			envelope["data"] = json.RawMessage(scenario.data)
			if scenario.occurredAt != "" {
				envelope["occurred_at"] = json.RawMessage(fmt.Sprintf("%q", scenario.occurredAt))
			}
			body, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			request, err := http.NewRequest(http.MethodPost, server.URL+paddlePaymentEventsPath, strings.NewReader(string(body)))
			if err != nil {
				t.Fatal(err)
			}
			request.URL.RawQuery = scenario.query
			request.Header.Set("Content-Type", scenario.contentType)
			request.Header.Set("Content-Encoding", scenario.encoding)
			signature := paymentInboxSignature(string(body), paymentInboxTestSecret, time.Now())
			request.Header.Set("Paddle-Signature", signature)
			if scenario.signatures == 2 {
				request.Header.Add("Paddle-Signature", signature)
			}
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			paymentInboxBoundaryResponse(t, request, response, scenario.status)
			if !reflect.DeepEqual(retained, retainedPaymentInbox(t, fixture.database)) || !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) || fixture.processor.creates.Load() != 1 {
				t.Fatal("rejected event changed inbox records or financial resources")
			}
		})
	}
}

func TestHostedPaymentsInboxRejectsIncompleteBodyWithoutAcknowledgment(t *testing.T) {
	fixture := newPaymentAuditFixture(t)
	server := paymentInboxTestServer(t, fixture.database, "sandbox", "processor-fixture")
	before := paymentAdjustmentResources(t, fixture)
	retained := retainedPaymentInbox(t, fixture.database)
	address, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("tcp", address.Host, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	body := paymentInboxFixtureEvent()
	if _, err := fmt.Fprintf(connection, "POST %s HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nPaddle-Signature: %s\r\nConnection: close\r\n\r\n%s", paddlePaymentEventsPath, address.Host, len(body)+1, paymentInboxSignature(body, paymentInboxTestSecret, time.Now()), body); err != nil {
		t.Fatal(err)
	}
	if err := connection.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	request := &http.Request{Method: http.MethodPost, URL: &url.URL{Path: paddlePaymentEventsPath}}
	response, err := http.ReadResponse(bufio.NewReader(connection), request)
	if err != nil {
		t.Fatal(err)
	}
	paymentInboxBoundaryResponse(t, request, response, http.StatusBadRequest)
	if !reflect.DeepEqual(retained, retainedPaymentInbox(t, fixture.database)) || !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) {
		t.Fatal("incomplete body changed the event or financial resources")
	}
}

func TestHostedPaymentsInboxReplayReadFailurePreservesAppliedCredit(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
	}{
		{"identical", http.StatusOK},
		{"conflicting", http.StatusConflict},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newPaymentAuditFixture(t)
			server := paymentInboxTestServer(t, fixture.database, "sandbox", "processor-fixture")
			before := paymentAdjustmentResources(t, fixture)
			retained := retainedPaymentInbox(t, fixture.database)
			body := retained[0].Payload
			if scenario.name == "conflicting" {
				body = strings.Replace(body, `"status":"completed"`, `"status":"paid"`, 1)
			}
			var failures atomic.Int64
			callback := fixture.database.database.Callback().Query()
			if err := callback.Before("gorm:query").Register("test:payment_inbox_replay", func(tx *gorm.DB) {
				if !tx.DryRun && tx.Statement.Table == "managed_payment_inbox_records" {
					failures.Add(1)
					tx.AddError(errors.New("controlled_payment_inbox_replay_failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			paymentInboxHTTP(t, server, body, paymentInboxSignature(body, paymentInboxTestSecret, time.Now()), http.StatusServiceUnavailable)
			if err := callback.Remove("test:payment_inbox_replay"); err != nil {
				t.Fatal(err)
			}
			if failures.Load() != 1 || !reflect.DeepEqual(retained, retainedPaymentInbox(t, fixture.database)) || !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) {
				t.Fatal("failed replay changed the retained event or financial resources")
			}
			for range 2 {
				database := openJournalTransactionInstance(t, fixture.database)
				restarted := paymentInboxTestServer(t, database, "sandbox", "processor-fixture")
				paymentInboxHTTP(t, restarted, body, paymentInboxSignature(body, paymentInboxTestSecret, time.Now()), scenario.status)
				if err := paymentProcessorFixture(t, fixture.checkout, database).reconcile(t.Context()); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(retained, retainedPaymentInbox(t, fixture.database)) || !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) || fixture.processor.creates.Load() != 1 {
					t.Fatal("restarted replay repeated payment effects")
				}
			}
		})
	}
}

func TestHostedPaymentsInboxPreservesExactLargeNumbersAcrossReplay(t *testing.T) {
	fixture := newPaymentAuditFixture(t)
	server := paymentInboxTestServer(t, fixture.database, "sandbox", "processor-fixture")
	before := paymentAdjustmentResources(t, fixture)
	body := strings.Replace(paymentInboxFixtureEvent(), `"currency_code":"USD"`, `"counter":9007199254740992,"currency_code":"USD"`, 1)
	paymentInboxHTTP(t, server, body, paymentInboxSignature(body, paymentInboxTestSecret, time.Now()), http.StatusOK)
	retained := retainedPaymentInbox(t, fixture.database)
	if len(retained) != 2 {
		t.Fatalf("retained events=%d", len(retained))
	}
	reordered := strings.Replace(body, `"counter":9007199254740992,"currency_code":"USD"`, `"currency_code":"USD","counter":9007199254740992`, 1)
	changed := strings.Replace(reordered, "9007199254740992", "9007199254740993", 1)
	for range 2 {
		restarted := paymentInboxTestServer(t, openJournalTransactionInstance(t, fixture.database), "sandbox", "processor-fixture")
		paymentInboxHTTP(t, restarted, reordered, paymentInboxSignature(reordered, paymentInboxTestSecret, time.Now()), http.StatusOK)
		paymentInboxHTTP(t, restarted, changed, paymentInboxSignature(changed, paymentInboxTestSecret, time.Now()), http.StatusConflict)
		if !reflect.DeepEqual(retained, retainedPaymentInbox(t, fixture.database)) || !reflect.DeepEqual(before, paymentAdjustmentResources(t, fixture)) || fixture.processor.creates.Load() != 1 {
			t.Fatal("numeric replay changed retained events or financial resources")
		}
	}
}
