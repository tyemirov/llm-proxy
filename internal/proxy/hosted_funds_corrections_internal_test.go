package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHostedFundsCorrectionCreditsAuditedSettlement(t *testing.T) {
	database, server, cookie, reservation := newFundsResolutionFixture(t)
	requestPath := "/billing-accounts/billing-journal/requests/" + reservation.RequestID
	path := requestPath + "/funds-credits/review-credit-1"
	body := `{"credit":{"numerator":"7","denominator":"1000"},"reason":"approved_correction","evidence_reference":"review-456"}`
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusConflict)
	decision := fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":"3","denominator":"200"},"reason":"approved_exception","evidence_reference":"review-123"}`, reservation.Revision)
	original := fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, requestPath+"/funds-resolution", decision, http.StatusOK)
	fundsResolutionHTTP(t, server, cookie("owner"), http.MethodPut, path, body, http.StatusForbidden)
	for range 2 {
		credit := fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusOK)
		if credit["credited_cents"] != "1" || credit["request_id"] != reservation.RequestID {
			t.Fatalf("credit=%v", credit)
		}
		if _, exposed := credit["evidence_reference"]; exposed {
			t.Fatal("private correction evidence exposed")
		}
	}
	assertHostedFundsBalance(t, database, 5, 5)
	assertFundsCreditRemainder(t, database, "1", "125")
	fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, path, "", http.StatusOK)
	fundsResolutionHTTP(t, server, cookie("other"), http.MethodGet, path, "", http.StatusNotFound)
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, strings.Replace(body, "review-456", "changed", 1), http.StatusConflict)
	excess := strings.Replace(body, `"7"`, `"9"`, 1)
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, requestPath+"/funds-credits/excess", excess, http.StatusConflict)
	final := strings.Replace(body, `"7"`, `"8"`, 1)
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, requestPath+"/funds-credits/final", final, http.StatusOK)
	assertFundsCreditRemainder(t, database, "0", "1")
	assertHostedFundsBalance(t, database, 5, 5)
	if current := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, requestPath+"/funds-resolution", "", http.StatusOK); !reflect.DeepEqual(current, original) {
		t.Fatalf("correction changed original decision: %v", current)
	}
	var charge managedChargeRecord
	if err := database.database.First(&charge).Error; err != nil || charge.State == chargeRated {
		t.Fatalf("correction invented resolved provider usage: charge=%+v error=%v", charge, err)
	}
}

func TestHostedFundsCorrectionAuditFailureRollsBack(t *testing.T) {
	database, server, cookie, reservation := newFundsResolutionFixture(t)
	requestPath := "/billing-accounts/billing-journal/requests/" + reservation.RequestID
	decision := fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":"3","denominator":"200"},"reason":"approved_exception","evidence_reference":"review-123"}`, reservation.Revision)
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, requestPath+"/funds-resolution", decision, http.StatusOK)
	if err := database.database.Exec("CREATE TRIGGER reject_request_credit BEFORE INSERT ON managed_funds_correction_records BEGIN SELECT RAISE(ABORT, 'controlled_request_credit_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	path := requestPath + "/funds-credits/rollback"
	body := `{"credit":{"numerator":"3","denominator":"200"},"reason":"approved_correction","evidence_reference":"review-rollback"}`
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusInternalServerError)
	fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, path, "", http.StatusNotFound)
	assertHostedFundsBalance(t, database, 4, 4)
	assertFundsCreditRemainder(t, database, "1", "200")
	if err := database.database.Exec("DROP TRIGGER reject_request_credit").Error; err != nil {
		t.Fatal(err)
	}
	reopened := openJournalTransactionInstance(t, database)
	restarted, restartedCookie := newFundsManagementHTTPFixture(t, reopened)
	var workers sync.WaitGroup
	for range 3 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			fundsResolutionHTTP(t, restarted, restartedCookie("operator"), http.MethodPut, path, body, http.StatusOK)
		}()
	}
	workers.Wait()
	assertHostedFundsBalance(t, reopened, 5, 5)
	assertFundsCreditRemainder(t, reopened, "0", "1")
}

func TestHostedFundsCorrectionSharesLimitWithUsageCredits(t *testing.T) {
	for _, order := range []string{"request-first", "usage-first"} {
		t.Run(order, func(t *testing.T) {
			database, server, cookie, reservation := newFundsResolutionFixtureFor(t, `{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`, journalRequestFailed)
			requestPath := "/billing-accounts/billing-journal/requests/" + reservation.RequestID
			decision := fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":"1","denominator":"1000"},"reason":"approved_partial_charge","evidence_reference":"review-partial"}`, reservation.Revision)
			fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, requestPath+"/funds-resolution", decision, http.StatusOK)
			var original managedChargeRecord
			if err := database.database.First(&original).Error; err != nil {
				t.Fatal(err)
			}
			usageCredit, err := newCustomerChargeAdjustment("billing-journal", original.ID, "full-usage-credit", "customer_credit", ExactMoney{Numerator: "1", Denominator: "1000"}, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			body := `{"credit":{"numerator":"1","denominator":"1000"},"reason":"approved_correction","evidence_reference":"review-credit"}`
			path := requestPath + "/funds-credits/full-request-credit"
			if order == "request-first" {
				fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusOK)
				if err := applyFundsFixtureCredit(t, database, usageCredit); !errors.Is(err, errUsageJournalConflict) {
					t.Fatalf("usage credit exceeded shared limit: %v", err)
				}
			} else {
				if err := applyFundsFixtureCredit(t, database, usageCredit); err != nil {
					t.Fatal(err)
				}
				fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusConflict)
			}
			assertHostedFundsBalance(t, database, 5, 5)
			assertFundsCreditRemainder(t, database, "0", "1")
		})
	}
}
