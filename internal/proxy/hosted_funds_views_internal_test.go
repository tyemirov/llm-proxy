package proxy

import (
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/MarkoPoloResearchLab/ledger/pkg/gormstore"
	"gorm.io/gorm"
)

func TestHostedFundsHistoryPaginatesWithoutLosingSameTimeEntries(t *testing.T) {
	_, management, _ := newFundsCreditFixture(t)
	for _, collection := range []struct {
		path, key string
		count     int
	}{
		{"/reservations", "reservations", 3}, {"/ledger-entries", "entries", 8},
	} {
		seen := map[string]bool{}
		cursor := ""
		for {
			path := "/billing-accounts/billing-journal" + collection.path + "?limit=1"
			if cursor != "" {
				path += "&cursor=" + url.QueryEscape(cursor)
			}
			response := ratingHTTPExchange(t, management, http.MethodGet, path, "", http.StatusOK)
			for _, item := range response[collection.key].([]any) {
				record := item.(map[string]any)
				id := record["id"].(string)
				if seen[id] || record["currency"] != "USD" {
					t.Fatalf("invalid page entry=%v", record)
				}
				if _, present := record["metadata"]; present {
					t.Fatal("private Ledger metadata exposed")
				}
				if _, present := record["idempotency_key"]; present {
					t.Fatal("private idempotency key exposed")
				}
				seen[id] = true
			}
			cursor = response["next_cursor"].(string)
			if cursor == "" {
				break
			}
		}
		if len(seen) != collection.count {
			t.Fatalf("%s count=%d want=%d", collection.path, len(seen), collection.count)
		}
	}
}

const fundsBalanceTestPath = "/billing-accounts/billing-journal/balance"

func TestHostedFundsBalanceReadDoesNotCreateFinancialRecords(t *testing.T) {
	database, _, management, _ := newHostedRatingFixture(t)
	for range 2 {
		response := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
		for _, field := range []string{"posted_cents", "available_cents", "reserved_cents", "spent_cents", "pending_cents"} {
			if response[field] != "0" {
				t.Fatalf("%s=%v", field, response[field])
			}
		}
		if response["currency"] != "USD" || response["state"] != "active" {
			t.Fatalf("balance=%v", response)
		}
	}
	for _, model := range []any{&gormstore.LedgerAccount{}, &managedFundsAccountRecord{}} {
		var count int64
		if err := database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("GET created financial records: %T count=%d error=%v", model, count, err)
		}
	}
}

func TestHostedFundsBalanceReadPreservesLargeCentAmounts(t *testing.T) {
	database, _, management, _ := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 9007199254740993)
	response := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
	if response["posted_cents"] != "9007199254740993" || response["available_cents"] != "9007199254740993" {
		t.Fatalf("balance lost integer precision: %v", response)
	}
}

func TestHostedFundsBalanceReadPreservesSpentHistoryAfterCredit(t *testing.T) {
	database, management, charges := newFundsCreditFixture(t)
	response := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
	if response["spent_cents"] != "1" || response["posted_cents"] != "4" {
		t.Fatalf("spent balance=%v", response)
	}
	if err := applyFundsFixtureCredit(t, database, fundsCreditCommand(t, charges[0], "balance-refund")); err != nil {
		t.Fatal(err)
	}
	response = ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
	if response["spent_cents"] != "1" || response["posted_cents"] != "5" {
		t.Fatalf("credit rewrote spent history: %v", response)
	}
}

func TestHostedFundsBalanceReadFailsWithoutPartialFinancialResponse(t *testing.T) {
	database, _, management, _ := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	callback := "test:balance_read_failure"
	if err := database.database.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "ledger_accounts" {
			tx.AddError(errors.New("controlled_balance_read_failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.database.Callback().Query().Remove(callback); err != nil {
			t.Error(err)
		}
	})
	response := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusInternalServerError)
	if len(response) != 1 || response["error"].(map[string]any)["code"] != "billing_account_store_failed" {
		t.Fatalf("partial financial response=%v", response)
	}
}

func TestHostedFundsBalanceReadTracksHoldSettlementAndCredit(t *testing.T) {
	database, _, management, prices := newHostedRatingFixture(t)
	seedHostedFunds(t, database, 5)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	server := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(prices))
	hostedIdentityHTTP(t, server, "balance-request", "funded prompt", http.StatusOK)
	response := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
	if response["posted_cents"] != "5" || response["available_cents"] != "2" || response["reserved_cents"] != "3" || response["pending_cents"] != "0" {
		t.Fatalf("held balance=%v", response)
	}
	if err := deliverFundsFixture(t, database); err != nil {
		t.Fatal(err)
	}
	response = ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
	if response["available_cents"] != "5" || response["reserved_cents"] != "0" || !reflect.DeepEqual(response["unsettled_fraction"], map[string]any{"numerator": "91", "denominator": "25000"}) {
		t.Fatalf("settled balance=%v", response)
	}
	var charge managedChargeRecord
	if err := database.database.First(&charge).Error; err != nil {
		t.Fatal(err)
	}
	if err := applyFundsFixtureCredit(t, database, fundsCreditCommand(t, charge, "balance-credit")); err != nil {
		t.Fatal(err)
	}
	response = ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
	if response["available_cents"] != "5" || !reflect.DeepEqual(response["unsettled_fraction"], map[string]any{"numerator": "0", "denominator": "1"}) {
		t.Fatalf("credited balance=%v", response)
	}
}
