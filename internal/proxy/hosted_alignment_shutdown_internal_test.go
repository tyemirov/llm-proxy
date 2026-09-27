package proxy

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestHostedAlignmentShutdownStorageFailuresPreserveReceipt(t *testing.T) {
	for _, boundary := range []string{"claim-lock", "receipt-read", "claim-release", "journal-binding"} {
		t.Run(boundary, func(t *testing.T) {
			testHostedAlignmentFinancial(t, "shutdown-"+boundary, "")
		})
	}
}

type alignmentShutdownReadFailureDialector struct {
	*sqlite.Dialector
	armed *atomic.Bool
}

func (dialector alignmentShutdownReadFailureDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	return database.Callback().Query().Before("gorm:query").Register("test:alignment_shutdown_read", func(tx *gorm.DB) {
		if tx.Statement.Table == "media_operation_records" && dialector.armed.CompareAndSwap(true, false) {
			tx.AddError(errors.New("controlled_alignment_shutdown_read_failure"))
		}
	})
}

func rejectAlignmentShutdownWrite(t *testing.T, database *gormManagedTenantDatabase, scenario string) string {
	t.Helper()
	var statement, reason string
	switch scenario {
	case "shutdown-claim-lock":
		statement = "BEFORE UPDATE OF generation ON media_operation_claim_records"
		reason = "lock media claim"
	case "shutdown-claim-release":
		statement = "BEFORE UPDATE OF expires_at ON media_operation_claim_records"
		reason = "release shutdown claim"
	case "shutdown-journal-binding":
		statement = "BEFORE UPDATE OF claim_expires_at ON managed_journal_request_records"
		reason = "bind media claim"
	case "shutdown-receipt-read":
		return "read shutdown receipt"
	default:
		t.Fatalf("unknown shutdown boundary: %s", scenario)
	}
	if err := database.database.Exec("CREATE TRIGGER reject_alignment_shutdown " + statement + " BEGIN SELECT RAISE(ABORT, 'controlled_alignment_shutdown_write_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	return reason
}

type alignmentShutdownState struct {
	operation mediaOperationRecord
	claim     mediaOperationClaimRecord
	journal   managedJournalRequestRecord
}

func readAlignmentShutdownState(t *testing.T, database *gormManagedTenantDatabase, operationID string) alignmentShutdownState {
	t.Helper()
	var state alignmentShutdownState
	if err := database.database.First(&state.operation, "operation_id = ?", operationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.database.First(&state.claim, "operation_id = ?", operationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.database.First(&state.journal, "execution_kind = ? AND execution_id = ?", journalExecutionMedia, operationID).Error; err != nil {
		t.Fatal(err)
	}
	return state
}
