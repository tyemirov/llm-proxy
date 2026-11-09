package proxy

import (
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const managedSQLiteRuntimeQuery = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"

func managementDatabaseDialector(configuration ManagementConfiguration) gorm.Dialector {
	if configuration.DatabaseDialector != nil {
		return configuration.DatabaseDialector
	}
	return sqlite.Open(configuration.DatabasePath + managedSQLiteRuntimeQuery)
}

func openManagedFinancialSignalsDatabase(databasePath string) (*gorm.DB, error) {
	if databasePath == "" {
		return nil, fmt.Errorf("financial signals require the managed database path")
	}
	absolute, err := filepath.Abs(databasePath)
	if err != nil {
		return nil, fmt.Errorf("resolve financial database path: %w", err)
	}
	location := url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=ro"}
	database, err := gorm.Open(sqlite.Open(location.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open financial signals database: %w", err)
	}
	return database, nil
}
