package proxy

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const paymentInboxTestSecret = "controlled-paddle-notification-secret"

func paymentInboxTestServer(t *testing.T, database *gormManagedTenantDatabase, environment, account string) *httptest.Server {
	t.Helper()
	return paymentInboxServerWithSecret(t, database, environment, account, paymentInboxTestSecret)
}

func paymentInboxServerWithSecret(t *testing.T, database *gormManagedTenantDatabase, environment, account, secret string) *httptest.Server {
	t.Helper()
	inbox, err := newPaddlePaymentInbox(database, environment, account, secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	registerPaddlePaymentRoutes(router, inbox)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server
}

func paymentInboxSignature(body, secret string, at time.Time) string {
	stamp := fmt.Sprint(at.Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(stamp + ":" + body))
	return "ts=" + stamp + ";h1=" + hex.EncodeToString(mac.Sum(nil))
}

func paymentInboxHTTP(t *testing.T, server *httptest.Server, body, signature string, status int) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/payments/paddle/events", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Paddle-Signature", signature)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("inbox status=%d want=%d body=%s", response.StatusCode, status, payload)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("payment event response permits caching")
	}
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateResponse(paddlePaymentEventsPath, http.MethodPost, status, response.Header, payload); err != nil {
		t.Fatal(err)
	}
}

func TestHostedPaymentsInboxDeduplicatesAcrossInstances(t *testing.T) {
	database, _, _, _ := newHostedRatingFixture(t)
	servers := []*httptest.Server{
		paymentInboxTestServer(t, database, "sandbox", "paddle-fixture-account"),
		paymentInboxTestServer(t, openJournalTransactionInstance(t, database), "sandbox", "paddle-fixture-account"),
	}
	body := paymentInboxFixtureEvent()
	var workers sync.WaitGroup
	for index := range 6 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			paymentInboxHTTP(t, servers[index%len(servers)], body, paymentInboxSignature(body, paymentInboxTestSecret, time.Now()), http.StatusOK)
		}()
	}
	workers.Wait()
	// Different delivery metadata and JSON layout describe the same event.
	reordered := `{"data":{"currency_code":"USD","status":"completed","id":"txn_01hv8x2axb33yr5y238zfwcn5p"},"notification_id":"ntf_01hv8x2axb33yr5y238zfwcn5q","occurred_at":"2026-09-23T12:00:00+00:00","event_type":"transaction.completed","event_id":"evt_01hv8x2axb33yr5y238zfwcn5p"}`
	paymentInboxHTTP(t, servers[0], reordered, paymentInboxSignature(reordered, paymentInboxTestSecret, time.Now()), http.StatusOK)
	var count int64
	if err := database.database.Model(&managedPaymentInboxRecord{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("duplicate events=%d error=%v", count, err)
	}
}

func TestHostedPaymentsInboxIsolatesProcessorIdentities(t *testing.T) {
	database, _, _, _ := newHostedRatingFixture(t)
	body := paymentInboxFixtureEvent()
	for _, identity := range []struct{ environment, account, secret string }{
		{"sandbox", "account-one", "sandbox-account-one-secret"},
		{"production", "account-one", "production-account-one-secret"},
		{"sandbox", "account-two", "sandbox-account-two-secret"},
	} {
		server := paymentInboxServerWithSecret(t, database, identity.environment, identity.account, identity.secret)
		paymentInboxHTTP(t, server, body, paymentInboxSignature(body, paymentInboxTestSecret, time.Now()), http.StatusUnauthorized)
		paymentInboxHTTP(t, server, body, paymentInboxSignature(body, identity.secret, time.Now()), http.StatusOK)
	}
	var count int64
	if err := database.database.Model(&managedPaymentInboxRecord{}).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("processor identities collapsed: count=%d error=%v", count, err)
	}
}

func TestHostedPaymentsInboxRejectsAcknowledgmentAfterDatabaseFailure(t *testing.T) {
	database, _, _, _ := newHostedRatingFixture(t)
	var diagnostic bytes.Buffer
	database.database = database.database.Session(&gorm.Session{Logger: logger.New(log.New(&diagnostic, "", 0), logger.Config{LogLevel: logger.Warn})})
	server := paymentInboxTestServer(t, database, "sandbox", "paddle-fixture-account")
	if err := database.database.Exec("CREATE TRIGGER reject_payment_event BEFORE INSERT ON managed_payment_inbox_records BEGIN SELECT RAISE(ABORT, 'controlled_payment_inbox_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	body := paymentInboxFixtureEvent()
	body = strings.Replace(body, `"currency_code":"USD"`, `"currency_code":"USD","customer_email":"private-payment@example.invalid"`, 1)
	paymentInboxHTTP(t, server, body, paymentInboxSignature(body, paymentInboxTestSecret, time.Now()), http.StatusServiceUnavailable)
	if strings.Contains(diagnostic.String(), "private-payment@example.invalid") {
		t.Fatal("payment inbox SQL diagnostic exposed the private event body")
	}
	if !strings.Contains(diagnostic.String(), "controlled_payment_inbox_failure") {
		t.Fatal("payment inbox lost the database failure diagnostic")
	}
	var count int64
	if err := database.database.Model(&managedPaymentInboxRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed inbox write retained event: count=%d error=%v", count, err)
	}
	if err := database.database.Exec("DROP TRIGGER reject_payment_event").Error; err != nil {
		t.Fatal(err)
	}
	restarted := paymentInboxTestServer(t, openJournalTransactionInstance(t, database), "sandbox", "paddle-fixture-account")
	paymentInboxHTTP(t, restarted, body, paymentInboxSignature(body, paymentInboxTestSecret, time.Now()), http.StatusOK)
}

func paymentInboxFixtureEvent() string {
	return `{"event_id":"evt_01hv8x2axb33yr5y238zfwcn5p","event_type":"transaction.completed","occurred_at":"2026-09-23T12:00:00Z","notification_id":"ntf_01hv8x2axb33yr5y238zfwcn5p","data":{"id":"txn_01hv8x2axb33yr5y238zfwcn5p","status":"completed","currency_code":"USD"}}`
}

func TestHostedPaymentsInboxPersistsVerifiedEvents(t *testing.T) {
	database, _, _, _ := newHostedRatingFixture(t)
	server := paymentInboxTestServer(t, database, "sandbox", "paddle-fixture-account")
	body := paymentInboxFixtureEvent()
	for range 2 {
		paymentInboxHTTP(t, server, body, paymentInboxSignature(body, paymentInboxTestSecret, time.Now()), http.StatusOK)
	}
	var rows []managedPaymentInboxRecord
	if err := database.database.Find(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatalf("retained inbox=%v error=%v", rows, err)
	}
	if rows[0].State != "pending" || rows[0].Environment != "sandbox" || rows[0].ProcessorAccountID != "paddle-fixture-account" {
		t.Fatalf("retained event=%+v", rows[0])
	}
	// Receipt of a signed event is not yet evidence that its funding order matches.
	assertHostedFundsBalance(t, database, 0, 0)
	reopened := openJournalTransactionInstance(t, database)
	restarted := paymentInboxTestServer(t, reopened, "sandbox", "paddle-fixture-account")
	paymentInboxHTTP(t, restarted, body, paymentInboxSignature(body, paymentInboxTestSecret, time.Now()), http.StatusOK)
	changed := strings.Replace(body, `"completed"`, `"paid"`, 1)
	paymentInboxHTTP(t, restarted, changed, paymentInboxSignature(changed, paymentInboxTestSecret, time.Now()), http.StatusConflict)
}

func TestHostedPaymentsInboxVerifiesBeforeParsing(t *testing.T) {
	database, _, _, _ := newHostedRatingFixture(t)
	server := paymentInboxTestServer(t, database, "sandbox", "paddle-fixture-account")
	body := paymentInboxFixtureEvent()
	for _, scenario := range []struct {
		body, signature string
		status          int
	}{
		{"{", "", http.StatusUnauthorized},
		{"{", paymentInboxSignature("{", paymentInboxTestSecret, time.Now()), http.StatusBadRequest},
		{body, paymentInboxSignature(body, "another-environment-secret", time.Now()), http.StatusUnauthorized},
		{body, paymentInboxSignature(body, paymentInboxTestSecret, time.Now().Add(-time.Hour)), http.StatusUnauthorized},
		{body + " ", paymentInboxSignature(body, paymentInboxTestSecret, time.Now()), http.StatusUnauthorized},
		{"{}", paymentInboxSignature("{}", paymentInboxTestSecret, time.Now()), http.StatusBadRequest},
		{strings.Repeat("x", paymentWebhookMaximumBytes+1), "", http.StatusRequestEntityTooLarge},
	} {
		paymentInboxHTTP(t, server, scenario.body, scenario.signature, scenario.status)
	}
	var count int64
	if err := database.database.Model(&managedPaymentInboxRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected events persisted: count=%d error=%v", count, err)
	}
}
