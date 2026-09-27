package proxy

import (
	"errors"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
)

// These inputs retain the shared Ledger domain types at the financial boundary.
// Each constructor returns all input errors before a caller invokes the service.
type hostedLedgerAmountInput struct {
	amount ledger.PositiveAmountCents
	key    ledger.IdempotencyKey
}

func newHostedLedgerAmountInput(cents int64, eventKey string) (hostedLedgerAmountInput, error) {
	amount, amountErr := ledger.NewPositiveAmountCents(cents)
	key, keyErr := ledger.NewIdempotencyKey(eventKey)
	return hostedLedgerAmountInput{amount: amount, key: key}, errors.Join(amountErr, keyErr)
}

type hostedLedgerReservationInput struct {
	hostedLedgerAmountInput
	reservation ledger.ReservationID
}

func newHostedLedgerReservationInput(cents int64, reservationID, eventKey string) (hostedLedgerReservationInput, error) {
	amount, amountErr := newHostedLedgerAmountInput(cents, eventKey)
	reservation, reservationErr := ledger.NewReservationID(reservationID)
	return hostedLedgerReservationInput{hostedLedgerAmountInput: amount, reservation: reservation}, errors.Join(amountErr, reservationErr)
}

type hostedLedgerReleaseInput struct {
	reservation ledger.ReservationID
	key         ledger.IdempotencyKey
}

func newHostedLedgerReleaseInput(reservationID, eventKey string) (hostedLedgerReleaseInput, error) {
	reservation, reservationErr := ledger.NewReservationID(reservationID)
	key, keyErr := ledger.NewIdempotencyKey(eventKey)
	return hostedLedgerReleaseInput{reservation: reservation, key: key}, errors.Join(reservationErr, keyErr)
}
