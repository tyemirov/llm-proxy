package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"github.com/tyemirov/tauth/pkg/sessionvalidator"
)

func newFundsResolutionFixture(t *testing.T) (*gormManagedTenantDatabase, *httptest.Server, func(string) *http.Cookie, managedFundsReservationRecord) {
	return newFundsResolutionFixtureFor(t, `{}`, journalRequestCompleted)
}

func newFundsResolutionFixtureFor(t *testing.T, usage string, state journalRequestState) (*gormManagedTenantDatabase, *httptest.Server, func(string) *http.Cookie, managedFundsReservationRecord) {
	t.Helper()
	database, _, _, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"unmetered-result","status":"completed","output_text":"funded result","usage":`+usage+`}`)
	}))
	t.Cleanup(upstream.Close)
	generation := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	hostedIdentityHTTP(t, generation, "unmetered-resolution", "funded prompt", http.StatusOK)
	// Seed the retained request outcome independently of the provider's known usage.
	if err := database.database.Model(&managedJournalRequestRecord{}).Where("billing_account_id = ?", "billing-journal").Update("state", state).Error; err != nil {
		t.Fatal(err)
	}
	if err := deliverFundsFixture(t, database); err != nil {
		t.Fatal(err)
	}
	if err := database.reconcileHostedFunds(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	var reservation managedFundsReservationRecord
	if err := database.database.First(&reservation).Error; err != nil {
		t.Fatal(err)
	}
	server, cookie := newFundsManagementHTTPFixture(t, database)
	return database, server, cookie, reservation
}

func newFundsManagementHTTPFixture(t *testing.T, database *gormManagedTenantDatabase) (*httptest.Server, func(string) *http.Cookie) {
	t.Helper()
	service := newInternalManagementService(t, newFakeManagedTenantDatabase(), internalManagementProviderRegistry())
	service.store.database = database
	service.sessionValidator.adminEmails = map[string]struct{}{"operator@example.com": {}}
	router := gin.New()
	service.registerRoutes(router)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	cookie := func(user string) *http.Cookie {
		claims := sessionvalidator.Claims{TenantID: service.configuration.TAuthTenantID, UserID: user, UserEmail: user + "@example.com", RegisteredClaims: jwt.RegisteredClaims{Issuer: service.configuration.JWTIssuer, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims).SignedString([]byte(service.configuration.JWTSigningKey))
		if err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: service.configuration.SessionCookieName, Value: token}
	}
	return server, cookie
}

func TestHostedFundsResolutionCapsLaterCreditsAtSettledAmount(t *testing.T) {
	database, server, cookie, reservation := newFundsResolutionFixtureFor(t, `{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`, journalRequestFailed)
	var original managedChargeRecord
	if err := database.database.First(&original).Error; err != nil {
		t.Fatal(err)
	}
	path := "/billing-accounts/billing-journal/requests/" + reservation.RequestID + "/funds-resolution"
	body := fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":"1","denominator":"1000"},"reason":"approved_partial_charge","evidence_reference":"review-partial"}`, reservation.Revision)
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusOK)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	generation := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(hostedRatingFixtureAdmission(t, 2)))
	hostedIdentityHTTP(t, generation, "independent-usage", "funded prompt", http.StatusOK)
	if err := deliverFundsFixture(t, database); err != nil {
		t.Fatal(err)
	}
	excessive := fundsCreditCommand(t, original, "credit-after-decision")
	if err := applyFundsFixtureCredit(t, database, excessive); !errors.Is(err, errUsageJournalConflict) {
		t.Fatalf("credit exceeded resolved charge: %v", err)
	}
	allowed, err := newCustomerChargeAdjustment("billing-journal", original.ID, "bounded-credit", "customer_credit", ExactMoney{Numerator: "1", Denominator: "1000"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := applyFundsFixtureCredit(t, database, allowed); err != nil {
		t.Fatal(err)
	}
	assertFundsCreditRemainder(t, database, "91", "25000")
}

func TestHostedFundsResolutionPreservesKnownPricesAndPriorCredits(t *testing.T) {
	for _, scenario := range []struct{ name, excessNumerator, excessDenominator, allowedDenominator string }{
		{"known-price", "3", "200", "25000"},
		{"prior-credit", "91", "25000", "50000"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, server, cookie, reservation := newFundsResolutionFixtureFor(t, `{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`, journalRequestFailed)
			if scenario.name == "prior-credit" {
				var charge managedChargeRecord
				if err := database.database.First(&charge).Error; err != nil {
					t.Fatal(err)
				}
				credit, err := newCustomerChargeAdjustment("billing-journal", charge.ID, "prior-decision-credit", "customer_credit", ExactMoney{Numerator: "91", Denominator: "50000"}, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				if err := applyFundsFixtureCredit(t, database, credit); err != nil {
					t.Fatal(err)
				}
			}
			path := "/billing-accounts/billing-journal/requests/" + reservation.RequestID + "/funds-resolution"
			body := func(numerator, denominator string) string {
				return fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":%q,"denominator":%q},"reason":"approved_charge","evidence_reference":"known-price-review"}`, reservation.Revision, numerator, denominator)
			}
			fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body(scenario.excessNumerator, scenario.excessDenominator), http.StatusConflict)
			fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body("91", scenario.allowedDenominator), http.StatusOK)
			assertFundsCreditRemainder(t, database, "91", scenario.allowedDenominator)
		})
	}
}

func fundsResolutionHTTP(t *testing.T, server *httptest.Server, cookie *http.Cookie, method, path, body string, status int) map[string]any {
	t.Helper()
	request, err := http.NewRequest(method, server.URL+managementAPIPath+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	request.Header.Set("Content-Type", "application/json")
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
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.StatusCode, status, payload)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("resolution response permits caching")
	}
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	template := "/api/management/billing-accounts/{billing_account_id}/requests/{request_id}/funds-resolution"
	if strings.Contains(path, "/reservations/") {
		template = "/api/management/billing-accounts/{billing_account_id}/reservations/{request_id}"
	}
	if strings.Contains(path, "/funds-credits/") {
		template = "/api/management/billing-accounts/{billing_account_id}/requests/{request_id}/funds-credits/{credit_id}"
	}
	if err := contract.ValidateResponse(template, method, status, response.Header, payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) == 0 {
		return nil
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestHostedFundsResolutionSettlesUncertainHoldOnce(t *testing.T) {
	database, server, cookie, reservation := newFundsResolutionFixture(t)
	reservationPath := "/billing-accounts/billing-journal/reservations/" + reservation.RequestID
	detail := fundsResolutionHTTP(t, server, cookie("operator"), http.MethodGet, reservationPath, "", http.StatusOK)
	if detail["revision"] != float64(reservation.Revision) || detail["authorized_maximum"].(map[string]any)["numerator"] != "13" || detail["authorized_maximum"].(map[string]any)["denominator"] != "500" {
		t.Fatalf("operator reservation=%v", detail)
	}
	fundsResolutionHTTP(t, server, cookie("other"), http.MethodGet, reservationPath, "", http.StatusNotFound)
	path := "/billing-accounts/billing-journal/requests/" + reservation.RequestID + "/funds-resolution"
	body := fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":"3","denominator":"200"},"reason":"approved_exception","evidence_reference":"review-123"}`, reservation.Revision)
	for range 2 {
		result := fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusOK)
		if result["settled_cents"] != "1" || result["reason"] != "approved_exception" {
			t.Fatalf("resolution=%v", result)
		}
	}
	assertHostedFundsBalance(t, database, 4, 4)
	result := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, path, "", http.StatusOK)
	if result["customer_charge"].(map[string]any)["numerator"] != "3" {
		t.Fatalf("resolution lost exact amount: %v", result)
	}
	if _, present := result["actor_user_id"]; present {
		t.Fatal("customer received private operator identity")
	}
	if _, present := result["evidence_reference"]; present {
		t.Fatal("customer received private review reference")
	}
	var financial managedFundsAccountRecord
	if err := database.database.First(&financial).Error; err != nil {
		t.Fatal(err)
	}
	if financial.RemainderNumerator != "1" || financial.RemainderDenominator != "200" {
		t.Fatalf("resolution remainder=%+v", financial)
	}
	var charge managedChargeRecord
	if err := database.database.First(&charge).Error; err != nil {
		t.Fatal(err)
	}
	if charge.State != chargeUsageUnresolved {
		t.Fatalf("operator decision changed provider evidence: %+v", charge)
	}
	if err := database.reconcileHostedFunds(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 4, 4)
}

func TestHostedFundsResolutionAuthorityBoundsAndConflicts(t *testing.T) {
	database, server, cookie, reservation := newFundsResolutionFixture(t)
	path := "/billing-accounts/billing-journal/requests/" + reservation.RequestID + "/funds-resolution"
	body := fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":"0","denominator":"1"},"reason":"approved_waiver","evidence_reference":"review-waiver"}`, reservation.Revision)
	fundsResolutionHTTP(t, server, nil, http.MethodPut, path, body, http.StatusUnauthorized)
	fundsResolutionHTTP(t, server, cookie("owner"), http.MethodPut, path, body, http.StatusForbidden)
	fundsResolutionHTTP(t, server, cookie("other"), http.MethodGet, path, "", http.StatusNotFound)
	for _, invalid := range []string{`{}`, strings.Replace(body, `"numerator":"0"`, `"numerator":"-1"`, 1), strings.Replace(body, `"denominator":"1"`, `"denominator":"0"`, 1), strings.Replace(body, `"review-waiver"`, `""`, 1)} {
		fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, invalid, http.StatusBadRequest)
	}
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, strings.Replace(body, `"revision":2`, `"revision":1`, 1), http.StatusConflict)
	for _, state := range []journalRequestState{journalRequestAccepted, journalRequestExecuting} {
		if err := database.database.Model(&managedJournalRequestRecord{}).Where("id = ?", reservation.RequestID).Update("state", state).Error; err != nil {
			t.Fatal(err)
		}
		fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusConflict)
	}
	if err := database.database.Model(&managedJournalRequestRecord{}).Where("id = ?", reservation.RequestID).Update("state", journalRequestCompleted).Error; err != nil {
		t.Fatal(err)
	}
	// Three cents fits the rounded hold, but exceeds the exact authorized 2.6 cents.
	excessive := strings.Replace(strings.Replace(body, `"numerator":"0"`, `"numerator":"3"`, 1), `"denominator":"1"`, `"denominator":"100"`, 1)
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, excessive, http.StatusConflict)
	assertHostedFundsBalance(t, database, 5, 2)
	operator := cookie("operator")
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			fundsResolutionHTTP(t, server, operator, http.MethodPut, path, body, http.StatusOK)
		}()
	}
	group.Wait()
	assertHostedFundsBalance(t, database, 5, 5)
	fundsResolutionHTTP(t, server, operator, http.MethodPut, path, strings.Replace(body, "approved_waiver", "changed_reason", 1), http.StatusConflict)
	var count int64
	if err := database.database.Model(&managedFundsSettlementRecord{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("settlement count=%d error=%v", count, err)
	}
	if err := database.database.Model(&managedFundsResolutionRecord{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("resolution count=%d error=%v", count, err)
	}
	var decision managedFundsResolutionRecord
	if err := database.database.First(&decision).Error; err != nil {
		t.Fatal(err)
	}
	if decision.ActorUserID != "operator" || decision.EvidenceReference != "review-waiver" {
		t.Fatalf("missing audit: %+v", decision)
	}
}

func TestHostedFundsResolutionAuditFailureRollsBackSettlement(t *testing.T) {
	database, server, cookie, reservation := newFundsResolutionFixture(t)
	path := "/billing-accounts/billing-journal/requests/" + reservation.RequestID + "/funds-resolution"
	body := fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":"3","denominator":"200"},"reason":"approved_exception","evidence_reference":"review-rollback"}`, reservation.Revision)
	if err := database.database.Exec("CREATE TRIGGER reject_financial_decision BEFORE INSERT ON managed_funds_resolution_records BEGIN SELECT RAISE(ABORT, 'controlled_decision_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusInternalServerError)
	assertHostedFundsBalance(t, database, 5, 2)
	assertFundsCreditRemainder(t, database, "0", "1")
	if err := database.database.Exec("DROP TRIGGER reject_financial_decision").Error; err != nil {
		t.Fatal(err)
	}
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusOK)
	assertHostedFundsBalance(t, database, 4, 4)
	reopened := openJournalTransactionInstance(t, database)
	if err := reopened.reconcileHostedFunds(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, reopened, 4, 4)
}
