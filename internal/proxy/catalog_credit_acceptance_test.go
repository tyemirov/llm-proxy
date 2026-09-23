package proxy_test

import (
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
)

func TestCatalogRatingCreditsReverseExactCentSettlement(t *testing.T) {
	zero := proxy.ExactMoney{Numerator: "0", Denominator: "1"}
	charges := []proxy.ExactMoney{zero, {Numerator: "1", Denominator: "1000000"}, {Numerator: "13", Denominator: "4000"}, {Numerator: "1", Denominator: "3"}, {Numerator: "9223372036854775807", Denominator: "100"}}
	for _, carry := range []proxy.ExactMoney{zero, {Numerator: "91", Denominator: "25000"}, {Numerator: "999", Denominator: "100000"}} {
		for _, charge := range charges {
			debited, remainder, err := proxy.SettleUSDCents(charge, carry)
			if err != nil {
				t.Fatal(err)
			}
			credited, restored, err := proxy.CreditUSDCents(charge, remainder)
			if err != nil || credited != debited || restored != carry {
				t.Fatalf("charge=%+v carry=%+v debit=%d credit=%d restored=%+v error=%v", charge, carry, debited, credited, restored, err)
			}
		}
	}
	carry := zero
	charge := proxy.ExactMoney{Numerator: "13", Denominator: "4000"}
	var debited, credited int64
	for range 400 {
		cents, next, err := proxy.SettleUSDCents(charge, carry)
		if err != nil {
			t.Fatal(err)
		}
		debited, carry = debited+cents, next
	}
	for range 400 {
		cents, next, err := proxy.CreditUSDCents(charge, carry)
		if err != nil {
			t.Fatal(err)
		}
		credited, carry = credited+cents, next
	}
	if debited != 130 || credited != 130 || carry != zero {
		t.Fatalf("fractional reversal lost money: debit=%d credit=%d carry=%+v", debited, credited, carry)
	}
}

func TestCatalogRatingCreditsRejectInvalidAmounts(t *testing.T) {
	zero := proxy.ExactMoney{Numerator: "0", Denominator: "1"}
	for _, value := range []proxy.ExactMoney{{Numerator: "-1", Denominator: "1"}, {Numerator: "1", Denominator: "0"}, {Numerator: "1.2", Denominator: "1"}, {Numerator: "01", Denominator: "1"}, {Numerator: "9223372036854775808", Denominator: "100"}, {}} {
		if _, _, err := proxy.CreditUSDCents(value, zero); err == nil {
			t.Fatalf("invalid credit accepted: %+v", value)
		}
	}
	for _, carry := range []proxy.ExactMoney{{Numerator: "1", Denominator: "100"}, {Numerator: "-1", Denominator: "1000"}, {}} {
		if _, _, err := proxy.CreditUSDCents(zero, carry); err == nil {
			t.Fatalf("invalid remainder accepted: %+v", carry)
		}
	}
}
