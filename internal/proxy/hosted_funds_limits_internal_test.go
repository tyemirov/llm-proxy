package proxy

import (
	"fmt"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
)

const tenantFundsLimitTestPath = "/billing-accounts/billing-journal/tenant-limits/managed-first"

func TestHostedFundsTenantLimitCountsReservationsAndExactUsage(t *testing.T) {
	database, _, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 10)
	empty := ratingHTTPExchange(t, management, http.MethodGet, tenantFundsLimitTestPath, "", http.StatusOK)
	if empty["limit_cents"] != nil || empty["revision"] != float64(0) {
		t.Fatalf("unset limit=%v", empty)
	}
	changed := ratingHTTPExchange(t, management, http.MethodPut, tenantFundsLimitTestPath, `{"limit_cents":"3","revision":0}`, http.StatusOK)
	if changed["remaining_cents"] != "3" || changed["revision"] != float64(1) {
		t.Fatalf("changed limit=%v", changed)
	}
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	hostedIdentityHTTP(t, server, "tenant-budget-first", "funded prompt", http.StatusOK)
	hostedIdentityHTTP(t, server, "tenant-budget-second", "funded prompt", http.StatusPaymentRequired)
	assertHostedFundsBalance(t, database, 10, 7)
	if err := deliverFundsFixture(t, database); err != nil {
		t.Fatal(err)
	}
	current := ratingHTTPExchange(t, management, http.MethodGet, tenantFundsLimitTestPath, "", http.StatusOK)
	if current["remaining_cents"] != "2" || current["reserved_cents"] != "0" || !reflect.DeepEqual(current["spent"], map[string]any{"numerator": "91", "denominator": "25000"}) {
		t.Fatalf("fractional usage lost: %v", current)
	}
	hostedIdentityHTTP(t, server, "tenant-budget-second", "funded prompt", http.StatusPaymentRequired)
	var charge managedChargeRecord
	if err := database.database.First(&charge).Error; err != nil {
		t.Fatal(err)
	}
	if err := applyFundsFixtureCredit(t, database, fundsCreditCommand(t, charge, "restore-tenant-allowance")); err != nil {
		t.Fatal(err)
	}
	current = ratingHTTPExchange(t, management, http.MethodGet, tenantFundsLimitTestPath, "", http.StatusOK)
	if current["remaining_cents"] != "3" {
		t.Fatalf("credit did not restore allowance: %v", current)
	}
	hostedIdentityHTTP(t, server, "tenant-budget-second", "funded prompt", http.StatusOK)
	if calls.Load() != 2 {
		t.Fatalf("tenant limit rejected work dispatched: %d", calls.Load())
	}
}

func TestHostedFundsTenantLimitRevisionAndLowering(t *testing.T) {
	database, _, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 10)
	for range 2 {
		ratingHTTPExchange(t, management, http.MethodPut, tenantFundsLimitTestPath, `{"limit_cents":"3","revision":0}`, http.StatusOK)
	}
	ratingHTTPExchange(t, management, http.MethodPut, tenantFundsLimitTestPath, `{"limit_cents":"4","revision":0}`, http.StatusConflict)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	hostedIdentityHTTP(t, server, "lower-budget-accepted", "funded prompt", http.StatusOK)
	ratingHTTPExchange(t, management, http.MethodPut, tenantFundsLimitTestPath, `{"limit_cents":"0","revision":1}`, http.StatusOK)
	hostedIdentityHTTP(t, server, "lower-budget-accepted", "funded prompt", http.StatusOK)
	hostedIdentityHTTP(t, server, "lower-budget-rejected", "funded prompt", http.StatusPaymentRequired)
	assertHostedFundsBalance(t, database, 10, 7)
	ratingHTTPExchange(t, management, http.MethodPut, tenantFundsLimitTestPath, `{"limit_cents":null,"revision":2}`, http.StatusOK)
	hostedIdentityHTTP(t, server, "lower-budget-rejected", "funded prompt", http.StatusOK)
}

func TestHostedFundsTenantLimitValidatesInputAndOwnership(t *testing.T) {
	database, _, management, _ := newHostedRatingFixture(t)
	if err := database.database.Create(&managedUserRecord{UserID: "another-owner", UserEmail: "another@example.com"}).Error; err != nil {
		t.Fatal(err)
	}
	ratingHTTPExchange(t, management, http.MethodGet, tenantFundsLimitTestPath, "", http.StatusOK)
	var count int64
	if err := database.database.Model(&managedFundsTenantRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("GET created tenant financial records: count=%d error=%v", count, err)
	}
	for _, body := range []string{`{}`, `{"limit_cents":null}`, `{"revision":0}`, `{"limit_cents":3,"revision":0}`, `{"limit_cents":"-1","revision":0}`, `{"limit_cents":"01","revision":0}`, `{"limit_cents":"9223372036854775808","revision":0}`, `{"limit_cents":"1","revision":-1}`, `{"limit_cents":"1","revision":0,"unknown":true}`} {
		ratingHTTPExchange(t, management, http.MethodPut, tenantFundsLimitTestPath, body, http.StatusBadRequest)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		body := ""
		if method == http.MethodPut {
			body = `{"limit_cents":"3","revision":0}`
		}
		ratingHTTPExchange(t, management, method, tenantFundsLimitTestPath+"?unexpected=1", body, http.StatusBadRequest)
		ratingHTTPExchange(t, management, method, "/billing-accounts/billing-journal/tenant-limits/missing", body, http.StatusNotFound)
		if err := database.database.Model(&managedTenantRecord{}).Where("tenant_id = ?", "managed-first").Update("owner_user_id", "another-owner").Error; err != nil {
			t.Fatal(err)
		}
		ratingHTTPExchange(t, management, method, tenantFundsLimitTestPath, body, http.StatusNotFound)
		if err := database.database.Model(&managedTenantRecord{}).Where("tenant_id = ?", "managed-first").Update("owner_user_id", "owner").Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestHostedFundsTenantLimitSettlementRollsBackWithProjection(t *testing.T) {
	database, _, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 10)
	ratingHTTPExchange(t, management, http.MethodPut, tenantFundsLimitTestPath, `{"limit_cents":"20","revision":0}`, http.StatusOK)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	for index := range 3 {
		hostedIdentityHTTP(t, server, fmt.Sprintf("tenant-rollback-%d", index), "funded prompt", http.StatusOK)
	}
	if err := database.database.Exec("CREATE TRIGGER reject_tenant_usage BEFORE UPDATE OF spent_numerator ON managed_funds_tenant_records BEGIN SELECT RAISE(ABORT, 'controlled_tenant_usage_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := deliverFundsFixture(t, database); err == nil {
		t.Fatal("failed tenant projection committed settlement")
	}
	assertHostedFundsBalance(t, database, 10, 1)
	current := ratingHTTPExchange(t, management, http.MethodGet, tenantFundsLimitTestPath, "", http.StatusOK)
	if current["reserved_cents"] != "9" || current["spent"].(map[string]any)["numerator"] != "0" {
		t.Fatalf("partial tenant settlement: %v", current)
	}
	if err := database.database.Exec("DROP TRIGGER reject_tenant_usage").Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := deliverFundsFixture(t, database); err != nil {
			t.Fatal(err)
		}
	}
	assertHostedFundsBalance(t, database, 9, 9)
	current = ratingHTTPExchange(t, management, http.MethodGet, tenantFundsLimitTestPath, "", http.StatusOK)
	if current["reserved_cents"] != "0" || current["remaining_cents"] != "18" || !reflect.DeepEqual(current["spent"], map[string]any{"numerator": "273", "denominator": "25000"}) {
		t.Fatalf("recovered tenant settlement: %v", current)
	}
}
