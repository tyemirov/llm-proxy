package proxy

import (
	"fmt"
	"math/big"
	"regexp"
)

const (
	ratingMoneyMaximumDigits       = 512
	usdCentsPerDollar        int64 = 100
)

var ratingIntegerPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

func parseExactMoney(value ExactMoney) (*big.Rat, error) {
	if len(value.Numerator) > ratingMoneyMaximumDigits || len(value.Denominator) > ratingMoneyMaximumDigits || !ratingIntegerPattern.MatchString(value.Numerator) || !ratingIntegerPattern.MatchString(value.Denominator) || value.Denominator == "0" {
		return nil, fmt.Errorf("%w: nonnegative rational money required", ErrCatalogRatingInvalid)
	}
	numerator, _ := new(big.Int).SetString(value.Numerator, 10)
	denominator, _ := new(big.Int).SetString(value.Denominator, 10)
	return new(big.Rat).SetFrac(numerator, denominator), nil
}

// SettleUSDCents floors aggregate charges at the ledger's integer-cent boundary.
// The exact remainder must be retained with the account in the same transaction.
func SettleUSDCents(charge, remainder ExactMoney) (int64, ExactMoney, error) {
	amount, err := parseExactMoney(charge)
	if err != nil {
		return 0, ExactMoney{}, err
	}
	carry, err := parseExactMoney(remainder)
	if err != nil {
		return 0, ExactMoney{}, err
	}
	if carry.Cmp(big.NewRat(1, usdCentsPerDollar)) >= 0 {
		return 0, ExactMoney{}, fmt.Errorf("%w: remainder must be less than one cent", ErrCatalogRatingInvalid)
	}
	amount.Add(amount, carry)
	scaled := new(big.Rat).Mul(amount, new(big.Rat).SetInt64(usdCentsPerDollar))
	cents := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	if !cents.IsInt64() {
		return 0, ExactMoney{}, fmt.Errorf("%w: ledger amount exceeds int64 cents", ErrCatalogRatingUnavailable)
	}
	settled := new(big.Rat).SetFrac(cents, big.NewInt(usdCentsPerDollar))
	residual := new(big.Rat).Sub(amount, settled)
	return cents.Int64(), ratingMoney(residual), nil
}

// ReserveUSDCents rounds an authorization maximum upward to integer cents.
// This amount is a reservation limit, not a customer charge.
func ReserveUSDCents(maximum ExactMoney) (int64, error) {
	amount, err := parseExactMoney(maximum)
	if err != nil {
		return 0, err
	}
	amount.Mul(amount, new(big.Rat).SetInt64(usdCentsPerDollar))
	cents, remainder := new(big.Int), new(big.Int)
	cents.QuoRem(amount.Num(), amount.Denom(), remainder)
	if remainder.Sign() != 0 {
		cents.Add(cents, big.NewInt(1))
	}
	if !cents.IsInt64() {
		return 0, fmt.Errorf("%w: reservation exceeds int64 cents", ErrCatalogRatingUnavailable)
	}
	return cents.Int64(), nil
}
