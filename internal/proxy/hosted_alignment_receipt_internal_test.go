package proxy

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const (
	alignmentReceiptClaimLock   = "receipt-claim-lock"
	alignmentReceiptAttemptRead = "receipt-attempt-read"
	alignmentReceiptIgnored     = "receipt-ignored"
)

func TestHostedAlignmentReceiptFailuresPreserveFundsWithoutResubmission(t *testing.T) {
	for _, boundary := range []string{alignmentReceiptClaimLock, alignmentReceiptAttemptRead, alignmentReceiptIgnored} {
		t.Run(boundary, func(t *testing.T) {
			testHostedAlignmentFinancial(t, boundary, "")
		})
	}
}

type alignmentReceiptFailureDialector struct {
	*sqlite.Dialector
	boundary string
	armed    *atomic.Bool
	failures *atomic.Int64
}

func (dialector alignmentReceiptFailureDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	fail := func(tx *gorm.DB) {
		if dialector.armed.CompareAndSwap(true, false) {
			dialector.failures.Add(1)
			tx.AddError(errors.New("controlled_alignment_receipt_failure"))
		}
	}
	if dialector.boundary == alignmentReceiptClaimLock {
		return database.Callback().Update().Before("gorm:update").Register("test:alignment_receipt_lock", func(tx *gorm.DB) {
			if tx.Statement.Table == "media_operation_claim_records" {
				fail(tx)
			}
		})
	}
	return database.Callback().Query().Before("gorm:query").Register("test:alignment_receipt_attempt", func(tx *gorm.DB) {
		if tx.Statement.Table == "managed_journal_attempt_records" {
			fail(tx)
		}
	})
}

func assertNoAlignmentProviderReceipt(t *testing.T, database *gormManagedTenantDatabase, operationID string) {
	t.Helper()
	var operation mediaOperationRecord
	if err := database.database.First(&operation, "operation_id = ?", operationID).Error; err != nil {
		t.Fatal(err)
	}
	var attempts []managedJournalAttemptRecord
	if err := database.database.Where("request_id IN (?)", database.database.Model(&managedJournalRequestRecord{}).Select("id").Where("execution_kind = ? AND execution_id = ?", journalExecutionMedia, operationID)).Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if operation.ProviderHandle != "" || len(attempts) != 1 || attempts[0].ProviderRequestID != "" {
		t.Fatal("failed receipt transaction retained a partial provider identity")
	}
}
