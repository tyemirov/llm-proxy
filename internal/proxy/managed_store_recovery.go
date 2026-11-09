//go:build pilot_presubmission_recovery

package proxy

import (
	"fmt"
	"net/url"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type managedRecoveryAccess string

const (
	managedRecoveryReadOnly  managedRecoveryAccess = "ro"
	managedRecoveryReadWrite managedRecoveryAccess = "rw"
)

func openManagedRecoveryDatabase(databasePath string, access managedRecoveryAccess) (*gorm.DB, error) {
	location := url.URL{Scheme: "file", Path: databasePath}
	query := url.Values{"mode": []string{string(access)}, "_pragma": []string{"busy_timeout(5000)"}}
	if access == managedRecoveryReadOnly {
		query.Add("_pragma", "query_only(1)")
	}
	location.RawQuery = query.Encode()
	database, err := gorm.Open(sqlite.Open(location.String()), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open recovery database: %w", err)
	}
	return database, nil
}

func lockManagedRecoveryOperation(transaction *gorm.DB, operationID, tenantID string) *gorm.DB {
	return lockManagedRows(transaction.Model(&mediaOperationRecord{}).Where("operation_id = ? AND tenant_id = ?", operationID, tenantID), "operation_id")
}

func lockManagedRecoveryUsage(transaction *gorm.DB, usageID uint, tenantID string) *gorm.DB {
	return lockManagedRows(transaction.Model(&managedUsageEventRecord{}).Where("id = ? AND tenant_id = ?", usageID, tenantID), "id")
}

func initializeManagedRecoveryAudit(transaction *gorm.DB) error {
	if err := transaction.Exec(`CREATE TABLE IF NOT EXISTS media_presubmission_recovery_records (operation_id TEXT PRIMARY KEY, plan TEXT NOT NULL, applied_operation TEXT NOT NULL, apply_receipt TEXT NOT NULL, applied_at DATETIME NOT NULL, final_operation TEXT, final_usage TEXT, final_receipt TEXT, finalized_at DATETIME)`).Error; err != nil {
		return fmt.Errorf("create private recovery audit: %w", err)
	}
	return nil
}
