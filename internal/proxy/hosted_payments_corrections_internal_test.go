package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestHostedPaymentsApprovedCorrectionPreservesFundingAndProviderEvidence(t *testing.T) {
	database, _, _, prices := newHostedRatingFixture(t)
	funding, fundingServer, fundingCookie := paymentOrdersFixture(t, database)
	processor := newCheckoutProtocolFixture(t)
	paymentOrderHTTP(t, fundingServer, fundingCookie("owner"), http.MethodPost, paymentOrdersTestPath, "correction-funding", `{"offer_code":"five"}`, http.StatusCreated)
	checkout := checkoutWorkerFixture(t, database, funding.funding, processor, time.Now())
	if err := checkout.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	completed := completedPaymentFixture(t, processor)
	sendPaymentEventFixture(t, database, completed, paymentTransactionCompleted, 1)
	if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 500, 500)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"provider-correction-request","status":"completed","output_text":"metered result","usage":{"input_tokens":1000,"output_tokens":1000,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	defer upstream.Close()
	generation := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	hostedIdentityHTTP(t, generation, "correction-usage", "funded prompt", http.StatusOK)
	if err := deliverFundsFixture(t, database); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 499, 499)
	assertFundsCreditRemainder(t, database, "3", "1000")
	var charge managedChargeRecord
	if err := database.database.First(&charge).Error; err != nil {
		t.Fatal(err)
	}
	var attempt managedJournalAttemptRecord
	if err := database.database.First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	input, _ := providerCostEvidenceFixture()
	periodStart := attempt.DispatchAt.UTC().Truncate(24 * time.Hour)
	input["period_start"], input["period_end"], input["reported_at"] = periodStart.Format(time.RFC3339), periodStart.Add(24*time.Hour).Format(time.RFC3339), periodStart.Add(25*time.Hour).Format(time.RFC3339)
	source := "provider,usage_cost\nopenai,0.009\n"
	input["source_reference"], input["source_sha256"], input["usage_amount"] = "approved-correction-source", sha256Hex(source), ExactMoney{"9", "1000"}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	management, payments := paymentReconciliationConfiguration(t, database, processor)
	report, err := ReconcileProviderCosts(t.Context(), management, "approved-correction-run", strings.NewReader(string(encoded)), strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(report.KnownProviderCost, ExactMoney{"1", "100"}) || len(report.Differences) != 1 || report.Differences[0].Code != "provider_usage_cost" {
		t.Fatalf("review report=%+v", report)
	}
	assertHostedFundsBalance(t, database, 499, 499)
	server, cookie := newFundsManagementHTTPFixture(t, database)
	path := "/billing-accounts/billing-journal/requests/" + charge.RequestID + "/funds-credits/invoice-review"
	reference := "provider-reconciliation:" + report.RunID + ":" + report.LocalEvidenceDigest
	body := fmt.Sprintf(`{"credit":{"numerator":"13","denominator":"1000"},"reason":"approved_invoice_review","evidence_reference":%q}`, reference)
	fundsResolutionHTTP(t, server, cookie("owner"), http.MethodPut, path, body, http.StatusForbidden)
	fundsResolutionHTTP(t, server, cookie("other"), http.MethodPut, path, body, http.StatusForbidden)
	assertHostedFundsBalance(t, database, 499, 499)
	for range 2 {
		credited := fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, body, http.StatusOK)
		if credited["credited_cents"] != "1" {
			t.Fatalf("credit=%v", credited)
		}
	}
	assertHostedFundsBalance(t, database, 500, 500)
	assertFundsCreditRemainder(t, database, "0", "1")
	var correction managedFundsCorrectionRecord
	if err := database.database.First(&correction).Error; err != nil {
		t.Fatal(err)
	}
	if correction.ActorUserID != "operator" || correction.EvidenceReference != reference || correction.Reason != "approved_invoice_review" {
		t.Fatalf("approval audit=%+v", correction)
	}
	var unchanged managedChargeRecord
	if err := database.database.First(&unchanged).Error; err != nil || !reflect.DeepEqual(charge, unchanged) {
		t.Fatalf("original rating changed: %v", err)
	}
	replay, err := ReconcileProviderCosts(t.Context(), management, report.RunID, strings.NewReader(string(encoded)), strings.NewReader(source))
	if err != nil || !reflect.DeepEqual(report, replay) {
		t.Fatalf("original source report changed: %v", err)
	}
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, path, strings.Replace(body, "approved_invoice_review", "different_review", 1), http.StatusConflict)
	fundsResolutionHTTP(t, server, cookie("operator"), http.MethodPut, strings.Replace(path, "invoice-review", "over-credit", 1), body, http.StatusConflict)
	adjustment := paymentAdjustmentFixture(processor, "approved", 200, 1)
	sendPaymentEventFixture(t, database, adjustment, "adjustment.created", 2)
	if err := paymentProcessorFixture(t, checkout, database).reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 300, 300)
	assertFundsCreditRemainder(t, database, "0", "1")
	paymentReport, err := ReconcilePayments(t.Context(), management, payments, "after-approved-correction")
	if err != nil || len(paymentReport.Items) != 1 || len(paymentReport.Items[0].Differences) != 0 {
		t.Fatalf("payment conservation report=%+v error=%v", paymentReport, err)
	}
}
