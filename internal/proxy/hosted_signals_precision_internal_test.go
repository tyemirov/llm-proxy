package proxy

import (
	"encoding/json"
	"math"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
)

func TestHostedSignalsPreserveAggregateBeyondInt64(t *testing.T) {
	fixture := newAccountCreationFixture(t)
	for _, owner := range []string{"owner", "another-owner"} {
		request, err := http.NewRequest(http.MethodPost, fixture.server.URL+managementAPIPath+managementBillingAccountsPath, strings.NewReader(`{"currency":"USD"}`))
		if err != nil {
			t.Fatal(err)
		}
		request.AddCookie(fixture.cookie(owner))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(managementIdempotencyHeader, "precision-account")
		response, err := fixture.server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var account managementBillingAccountResponse
		err = json.NewDecoder(response.Body).Decode(&account)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusCreated {
			t.Fatalf("precision account status=%d error=%v", response.StatusCode, err)
		}
		funds, err := newHostedLedgerAccount(fixture.database.database, account.ID, ratingTestAcceptanceTime())
		if err != nil {
			t.Fatal(err)
		}
		amount, err := ledger.NewPositiveAmountCents(math.MaxInt64)
		if err != nil {
			t.Fatal(err)
		}
		key, err := ledger.NewIdempotencyKey("precision-funding")
		if err != nil {
			t.Fatal(err)
		}
		metadata, err := ledger.NewMetadataJSON(`{"source":"precision-fixture"}`)
		if err != nil {
			t.Fatal(err)
		}
		if err := funds.service.Grant(t.Context(), funds.tenant, funds.user, funds.namespace, amount, key, 0, metadata); err != nil {
			t.Fatal(err)
		}
		balance := paymentOrderHTTP(t, fixture.server, fixture.cookie(owner), http.MethodGet, managementBillingAccountsPath+"/"+account.ID+"/balance", "", "", http.StatusOK)
		if balance["posted_cents"] != "9223372036854775807" || balance["available_cents"] != "9223372036854775807" || balance["reserved_cents"] != "0" {
			t.Fatalf("account balance lost precision: %v", balance)
		}
	}
	before := fixture.inventory(t)
	original := readHostedSignalsFixture(t, fixture.database)
	if original.PostedCents != "18446744073709551614" || original.ReservedCents != "0" || original.UnsettledFraction != (ExactMoney{Numerator: "0", Denominator: "1"}) {
		t.Fatalf("aggregate balance lost precision: %+v", original)
	}
	repeated := readHostedSignalsFixture(t, fixture.database)
	repeated.ObservedAt = original.ObservedAt
	if !reflect.DeepEqual(original, repeated) || !reflect.DeepEqual(before, fixture.inventory(t)) {
		t.Fatal("aggregate read changed the report or financial records")
	}
}
