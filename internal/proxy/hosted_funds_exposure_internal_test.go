package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestHostedFundsExposureReportsExcessProviderCost(t *testing.T) {
	database, server, cookie, reservation := newFundsResolutionFixtureFor(t, `{"input_tokens":20000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`, journalRequestCompleted)
	path := "/billing-accounts/billing-journal/reservations/" + reservation.RequestID
	response := fundsResolutionHTTP(t, server, cookie("operator"), http.MethodGet, path, "", http.StatusOK)
	expectedCost := map[string]any{"numerator": "51", "denominator": "1250"}
	expectedExposure := map[string]any{"numerator": "37", "denominator": "2500"}
	if response["provider_cost_complete"] != true || !reflect.DeepEqual(response["known_provider_cost"], expectedCost) || !reflect.DeepEqual(response["known_platform_exposure"], expectedExposure) {
		t.Fatalf("missing provider exposure: %v", response)
	}
	assertHostedFundsBalance(t, database, 5, 2)
	body := fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":"0","denominator":"1"},"reason":"approved_waiver","evidence_reference":"excess-cost-review"}`, reservation.Revision)
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, "/billing-accounts/billing-journal/requests/"+reservation.RequestID+"/funds-resolution", body, http.StatusOK)
	after := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, path, "", http.StatusOK)
	if !reflect.DeepEqual(after["known_provider_cost"], expectedCost) || !reflect.DeepEqual(after["known_platform_exposure"], expectedExposure) {
		t.Fatalf("waiver changed provider exposure: %v", after)
	}
	assertHostedFundsBalance(t, database, 5, 5)
	var cases int64
	if err := database.database.Model(&managedJournalCaseRecord{}).Where("request_id = ? AND reason = ?", reservation.RequestID, "platform_exposure").Count(&cases).Error; err != nil || cases != 1 {
		t.Fatalf("exposure cases=%d error=%v", cases, err)
	}
}

func TestHostedFundsExposureFailureRetainsHoldAndPendingEvidence(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"excess-cost-result","status":"completed","output_text":"funded result","usage":{"input_tokens":20000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)
	generation := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	hostedIdentityHTTP(t, generation, "exposure-write-failure", "funded prompt", http.StatusOK)
	if err := database.database.Exec("CREATE TRIGGER reject_exposure BEFORE INSERT ON managed_funds_exposure_records BEGIN SELECT RAISE(ABORT, 'controlled_exposure_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := deliverFundsFixture(t, database); err == nil {
		t.Fatal("failed exposure write acknowledged usage")
	}
	assertHostedFundsBalance(t, database, 5, 2)
	var count int64
	if err := database.database.Model(&managedChargeRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial financial delivery: charges=%d error=%v", count, err)
	}
	if err := database.database.Exec("DROP TRIGGER reject_exposure").Error; err != nil {
		t.Fatal(err)
	}
	restarted := openJournalTransactionInstance(t, database)
	for range 2 {
		if err := restarted.reconcileHostedFunds(t.Context(), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	assertHostedFundsBalance(t, restarted, 5, 2)
	var records []managedFundsExposureRecord
	if err := restarted.database.Find(&records).Error; err != nil || len(records) != 1 {
		t.Fatalf("replayed exposure records=%v error=%v", records, err)
	}
	if records[0].ExcessNumerator != "37" || records[0].ExcessDenominator != "2500" || !records[0].ProviderCostComplete {
		t.Fatalf("retained exposure=%+v", records[0])
	}
	if err := restarted.database.Model(&managedJournalCaseRecord{}).Where("reason = ?", journalCasePlatformExposure).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("replayed exposure cases=%d error=%v", count, err)
	}
}

func TestHostedFundsExposureDoesNotConvertUnknownCostToZero(t *testing.T) {
	_, server, cookie, reservation := newFundsResolutionFixture(t)
	response := fundsResolutionHTTP(t, server, cookie("owner"), http.MethodGet, "/billing-accounts/billing-journal/reservations/"+reservation.RequestID, "", http.StatusOK)
	if response["provider_cost_complete"] != false || !reflect.DeepEqual(response["known_provider_cost"], map[string]any{"numerator": "0", "denominator": "1"}) {
		t.Fatalf("unknown cost not explicit: %v", response)
	}
}
