package proxy

import (
	"errors"
	"net/http"
	"reflect"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func TestHostedRequestSummaryIncludesAuditedRequestCredits(t *testing.T) {
	database, server, cookie, reservation := newFundsResolutionFixtureFor(t, `{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`, journalRequestCompleted)
	requestPath := "/billing-accounts/billing-journal/requests/" + reservation.RequestID
	summaryPath := requestPath + "/charge-summary"
	original := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, summaryPath, "", http.StatusOK)
	if original["state"] != "rated" || !reflect.DeepEqual(original["customer_charge"], map[string]any{"numerator": "91", "denominator": "25000"}) {
		t.Fatalf("original funded summary=%v", original)
	}
	creditPath := requestPath + "/funds-credits/summary-partial"
	body := `{"credit":{"numerator":"1","denominator":"1000"},"reason":"approved_correction","evidence_reference":"summary-review"}`
	receipt := fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, creditPath, body, http.StatusOK)
	expected := make(map[string]any, len(original))
	for key, value := range original {
		expected[key] = value
	}
	expected["customer_credits"] = map[string]any{"numerator": "1", "denominator": "1000"}
	expected["net_customer_charge"] = map[string]any{"numerator": "33", "denominator": "12500"}
	if current := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, summaryPath, "", http.StatusOK); !reflect.DeepEqual(current, expected) {
		t.Fatalf("audited credit missing from request summary: got=%v want=%v", current, expected)
	}
	restarted, restartedCookie := newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, database))
	for range 2 {
		if replayed := fundsResolutionHTTP(t, restarted, restartedCookie("operator"), http.MethodPut, creditPath, body, http.StatusOK); !reflect.DeepEqual(replayed, receipt) {
			t.Fatalf("credit replay changed receipt: %v", replayed)
		}
		if current := fundsResolutionHTTP(t, restarted, restartedCookie("owner"), http.MethodGet, summaryPath, "", http.StatusOK); !reflect.DeepEqual(current, expected) {
			t.Fatalf("restarted summary changed: %v", current)
		}
	}
	full := `{"credit":{"numerator":"33","denominator":"12500"},"reason":"approved_correction","evidence_reference":"summary-full-review"}`
	fundsResolutionHTTP(t, restarted, restartedCookie("operator"), http.MethodPut, requestPath+"/funds-credits/summary-rest", full, http.StatusOK)
	expected["customer_credits"] = original["customer_charge"]
	expected["net_customer_charge"] = map[string]any{"numerator": "0", "denominator": "1"}
	if current := fundsResolutionHTTP(t, restarted, restartedCookie("owner"), http.MethodGet, summaryPath, "", http.StatusOK); !reflect.DeepEqual(current, expected) {
		t.Fatalf("full credit summary=%v want=%v", current, expected)
	}
	fundsResolutionHTTP(t, restarted, restartedCookie("other"), http.MethodGet, summaryPath, "", http.StatusNotFound)
}

func TestHostedRequestSummaryRejectsInvalidAuditedCredits(t *testing.T) {
	database, server, cookie, reservation := newFundsResolutionFixtureFor(t, `{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`, journalRequestCompleted)
	requestPath := "/billing-accounts/billing-journal/requests/" + reservation.RequestID
	summaryPath := requestPath + "/charge-summary"
	creditPath := requestPath + "/funds-credits/summary-evidence"
	body := `{"credit":{"numerator":"1","denominator":"1000"},"reason":"approved_correction","evidence_reference":"summary-review"}`
	receipt := fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, creditPath, body, http.StatusOK)
	original := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, summaryPath, "", http.StatusOK)
	var record managedFundsCorrectionRecord
	if err := database.database.First(&record).Error; err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct{ column, corrupt, restored string }{
		{"credit_numerator", "2", record.CreditNumerator},
		{"remainder_after_denominator", "0", record.Effect.RemainderAfterDenominator},
		{"reason", "invalid reason", record.Reason},
	} {
		t.Run(scenario.column, func(t *testing.T) {
			if err := database.database.Model(&record).UpdateColumn(scenario.column, scenario.corrupt).Error; err != nil {
				t.Fatal(err)
			}
			failed := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, summaryPath, "", http.StatusInternalServerError)
			if !reflect.DeepEqual(failed, map[string]any{"error": map[string]any{"code": "usage_journal_unavailable"}}) {
				t.Fatalf("partial or private credit evidence exposed: %v", failed)
			}
			if err := database.database.Model(&record).UpdateColumn(scenario.column, scenario.restored).Error; err != nil {
				t.Fatal(err)
			}
		})
	}
	// This receipt is internally consistent but exceeds the original charge:
	// 0.00364 - 0.004 + 0.01 = 0.00964, with one credited cent.
	if err := database.database.Model(&managedFundsCorrectionRecord{}).Where("id = ?", record.ID).Updates(map[string]any{
		"credit_numerator": "4", "credited_cents": int64(1), "remainder_after_numerator": "241", "remainder_after_denominator": "25000",
	}).Error; err != nil {
		t.Fatal(err)
	}
	fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, summaryPath, "", http.StatusInternalServerError)
	if err := database.database.Model(&managedFundsCorrectionRecord{}).Where("id = ?", record.ID).Updates(map[string]any{
		"credit_numerator": record.CreditNumerator, "credited_cents": record.Effect.CreditedCents,
		"remainder_after_numerator": record.Effect.RemainderAfterNumerator, "remainder_after_denominator": record.Effect.RemainderAfterDenominator,
	}).Error; err != nil {
		t.Fatal(err)
	}
	callback := database.database.Callback().Query()
	if err := callback.Before("gorm:query").Register("test:summary_credit_read", func(tx *gorm.DB) {
		if tx.Statement.Table == "managed_funds_correction_records" {
			tx.AddError(errors.New("controlled_summary_credit_read_failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, summaryPath, "", http.StatusInternalServerError)
	if err := callback.Remove("test:summary_credit_read"); err != nil {
		t.Fatal(err)
	}
	if restored := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, summaryPath, "", http.StatusOK); !reflect.DeepEqual(restored, original) {
		t.Fatalf("restored summary changed: %v", restored)
	}
	if current := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, creditPath, "", http.StatusOK); !reflect.DeepEqual(current, receipt) {
		t.Fatalf("summary read changed credit receipt: %v", current)
	}
}

func TestHostedRequestSummaryPreservesUnresolvedStateAfterCredit(t *testing.T) {
	fixture := newFundsCorrectionFixture(t)
	path := "/billing-accounts/billing-journal/requests/" + fixture.reservation.RequestID + "/charge-summary"
	original := fundsResolutionHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, path, "", http.StatusOK)
	if original["state"] != "unresolved" || original["customer_charge"] != nil || original["customer_credits"] != nil || original["net_customer_charge"] != nil {
		t.Fatalf("unexpected unresolved summary: %v", original)
	}
	fundsResolutionHTTP(t, fixture.server, fixture.cookie("operator"), http.MethodPut, fixture.creditPath, fixture.creditBody, http.StatusOK)
	if current := fundsResolutionHTTP(t, fixture.server, fixture.cookie("owner"), http.MethodGet, path, "", http.StatusOK); !reflect.DeepEqual(current, original) {
		t.Fatalf("request credit invented resolved usage: %v", current)
	}
}

func TestHostedRequestSummaryUsesOneAuditedCreditSnapshot(t *testing.T) {
	database, server, cookie, reservation := newFundsResolutionFixtureFor(t, `{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`, journalRequestCompleted)
	requestPath := "/billing-accounts/billing-journal/requests/" + reservation.RequestID
	summaryPath := requestPath + "/charge-summary"
	original := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, summaryPath, "", http.StatusOK)
	second, secondCookie := newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, database))
	var once sync.Once
	callback := database.database.Callback().Query()
	if err := callback.After("gorm:query").Register("test:summary_concurrent_request_credit", func(tx *gorm.DB) {
		if tx.Statement.Table == "managed_journal_request_records" {
			once.Do(func() {
				body := `{"credit":{"numerator":"1","denominator":"1000"},"reason":"approved_correction","evidence_reference":"snapshot-review"}`
				fundsResolutionHTTP(t, second, secondCookie("operator"), http.MethodPut, requestPath+"/funds-credits/snapshot", body, http.StatusOK)
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := callback.Remove("test:summary_concurrent_request_credit"); err != nil {
			t.Error(err)
		}
	})
	if current := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, summaryPath, "", http.StatusOK); !reflect.DeepEqual(current, original) {
		t.Fatalf("summary mixed credit snapshots: %v", current)
	}
	original["customer_credits"] = map[string]any{"numerator": "1", "denominator": "1000"}
	original["net_customer_charge"] = map[string]any{"numerator": "33", "denominator": "12500"}
	if current := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, summaryPath, "", http.StatusOK); !reflect.DeepEqual(current, original) {
		t.Fatalf("next summary omitted committed credit: %v", current)
	}
}
