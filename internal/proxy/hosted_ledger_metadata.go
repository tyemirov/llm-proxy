package proxy

import (
	"encoding/json"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
)

type hostedLedgerRequestMetadata struct {
	RequestID string `json:"request_id"`
}

type hostedLedgerChargeCreditMetadata struct {
	AdjustmentID string `json:"adjustment_id"`
	ChargeID     string `json:"charge_id"`
}

type hostedLedgerCorrectionMetadata struct {
	RequestID string `json:"request_id"`
	CreditID  string `json:"credit_id"`
}

type hostedLedgerMetadataFields interface {
	hostedLedgerRequestMetadata | hostedLedgerChargeCreditMetadata | hostedLedgerCorrectionMetadata | map[string]string
}

func newHostedLedgerMetadata[Fields hostedLedgerMetadataFields](fields Fields) ledger.MetadataJSON {
	// The closed field types contain only strings. Marshal always produces
	// valid JSON, which satisfies the shared Ledger metadata constructor.
	encoded, _ := json.Marshal(fields)
	metadata, _ := ledger.NewMetadataJSON(string(encoded))
	return metadata
}
