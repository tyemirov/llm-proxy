package proxy

import (
	"net/http"
	"reflect"
	"testing"
)

func TestHostedFundsTenantLimitWriteFailuresPreserveBudget(t *testing.T) {
	for _, scenario := range []struct{ name, statement, body string }{
		{"account-lock", "BEFORE UPDATE OF id ON managed_billing_account_records", `{"limit_cents":"5","revision":0}`},
		{"insert", "BEFORE INSERT ON managed_funds_tenant_records", `{"limit_cents":"5","revision":0}`},
		{"update", "BEFORE UPDATE OF limit_cents ON managed_funds_tenant_records", `{"limit_cents":"4","revision":1}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			if scenario.name == "update" {
				ratingHTTPExchange(t, fixture.management, http.MethodPut, tenantFundsLimitTestPath, `{"limit_cents":"5","revision":0}`, http.StatusOK)
			}
			before := fixture.state(t)
			limit := ratingHTTPExchange(t, fixture.management, http.MethodGet, tenantFundsLimitTestPath, "", http.StatusOK)
			if err := fixture.database.database.Exec("CREATE TRIGGER reject_tenant_limit " + scenario.statement + " BEGIN SELECT RAISE(ABORT, 'controlled_tenant_limit_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			ratingHTTPExchange(t, fixture.management, http.MethodPut, tenantFundsLimitTestPath, scenario.body, http.StatusInternalServerError)
			if err := fixture.database.database.Exec("DROP TRIGGER reject_tenant_limit").Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(limit, ratingHTTPExchange(t, fixture.management, http.MethodGet, tenantFundsLimitTestPath, "", http.StatusOK)) || !reflect.DeepEqual(before, fixture.state(t)) || fixture.calls.Load() != 0 {
				t.Fatal("failed tenant budget write changed limits, funds, or provider calls")
			}
			accepted := ratingHTTPExchange(t, fixture.management, http.MethodPut, tenantFundsLimitTestPath, scenario.body, http.StatusOK)
			if replay := ratingHTTPExchange(t, fixture.management, http.MethodPut, tenantFundsLimitTestPath, scenario.body, http.StatusOK); !reflect.DeepEqual(accepted, replay) {
				t.Fatal("tenant budget replay changed its revision")
			}
			fixture.recoverAdmission(t)
			current := ratingHTTPExchange(t, fixture.management, http.MethodGet, tenantFundsLimitTestPath, "", http.StatusOK)
			if current["revision"] != accepted["revision"] || current["reserved_cents"] != "0" || !reflect.DeepEqual(current["spent"], map[string]any{"numerator": "91", "denominator": "25000"}) {
				t.Fatalf("tenant budget recovery lost exact accounting: %v", current)
			}
		})
	}
}

func TestHostedFundsTenantAdmissionRejectsCorruptUsage(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	ratingHTTPExchange(t, fixture.management, http.MethodPut, tenantFundsLimitTestPath, `{"limit_cents":"5","revision":0}`, http.StatusOK)
	before := fixture.state(t)
	if err := fixture.database.database.Model(&managedFundsTenantRecord{}).Where("tenant_id = ?", "managed-first").UpdateColumn("spent_denominator", "0").Error; err != nil {
		t.Fatal(err)
	}
	fixture.reject(t)
	if err := fixture.database.database.Model(&managedFundsTenantRecord{}).Where("tenant_id = ?", "managed-first").UpdateColumn("spent_denominator", "1").Error; err != nil {
		t.Fatal(err)
	}
	fixture.assertRolledBack(t, before)
	fixture.recoverAdmission(t)
}

func TestHostedFundsTenantCreditRejectsInconsistentUsage(t *testing.T) {
	fixture := newFundsCorrectionFixture(t)
	before := fixture.resources(t)
	var tenant managedFundsTenantRecord
	if err := fixture.database.database.Where("tenant_id = ?", "managed-first").First(&tenant).Error; err != nil {
		t.Fatal(err)
	}
	original := tenant.SpentNumerator
	if err := fixture.database.database.Model(&tenant).UpdateColumn("spent_numerator", "0").Error; err != nil {
		t.Fatal(err)
	}
	fixture.reject(t, http.StatusInternalServerError)
	if err := fixture.database.database.Model(&tenant).UpdateColumn("spent_numerator", original).Error; err != nil {
		t.Fatal(err)
	}
	fixture.assertUnchanged(t, before)
	fixture.recoverCredit(t, before)
}
