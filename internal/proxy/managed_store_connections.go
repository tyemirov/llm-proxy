package proxy

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
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

func initializeWithHostedQuoteBoundary(database *gorm.DB, initialize func(*gorm.DB) error) error {
	expression, err := quoteJournalExpression(database)
	if err != nil {
		return err
	}
	predecessor := false
	switch canonicalJournalExpression(expression) {
	case "", canonicalJournalExpression(currentJournalQuoteExpression):
	case canonicalJournalExpression(priorJournalQuoteExpression):
		predecessor = true
	default:
		return fmt.Errorf("%w: unsupported journal predecessor expression", errManagedTenantSchemaMigration)
	}
	if !predecessor {
		return database.Transaction(initialize)
	}
	return database.Connection(func(connection *gorm.DB) (result error) {
		connection = connection.Session(&gorm.Session{NewDB: true})
		cleanup := func() error {
			ctx, cancel := context.WithTimeout(context.Background(), journalPersistenceTimeout)
			defer cancel()
			clean := connection.WithContext(ctx)
			restoreError := clean.Exec("PRAGMA foreign_keys = ON").Error
			var enabled int
			if restoreError == nil {
				restoreError = clean.Raw("PRAGMA foreign_keys").Scan(&enabled).Error
			}
			if restoreError == nil && enabled != 1 {
				restoreError = fmt.Errorf("foreign key enforcement was not restored")
			}
			if restoreError != nil {
				if pinned, ok := connection.Statement.ConnPool.(*sql.Conn); ok {
					_ = pinned.Raw(func(any) error { return driver.ErrBadConn })
				}
				pool, poolError := database.DB()
				if poolError == nil {
					poolError = pool.Close()
				}
				return errors.Join(fmt.Errorf("restore quote migration foreign keys: %w", restoreError), poolError)
			}
			return nil
		}
		defer func() { result = errors.Join(result, cleanup()) }()
		var enabled int
		if err := connection.Raw("PRAGMA foreign_keys").Scan(&enabled).Error; err != nil {
			return err
		}
		if enabled != 1 {
			return fmt.Errorf("%w: initial foreign key enforcement disabled", errManagedTenantSchemaMigration)
		}
		if err := connection.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
			return err
		}
		if err := connection.Raw("PRAGMA foreign_keys").Scan(&enabled).Error; err != nil {
			return err
		}
		if enabled != 0 {
			return fmt.Errorf("%w: could not suspend foreign keys on pinned connection", errManagedTenantSchemaMigration)
		}
		return connection.Transaction(func(transaction *gorm.DB) error {
			transaction = transaction.Session(&gorm.Session{NewDB: true})
			// Acquire SQLite's writer before rechecking the named predecessor. The
			// false predicate avoids executing any retained row's update triggers.
			if err := transaction.Exec("UPDATE managed_journal_request_records SET state = state WHERE 0").Error; err != nil {
				return fmt.Errorf("lock quoted schema transition: %w", err)
			}
			if _, err := quoteSchemaPredecessor(transaction); err != nil {
				return err
			}
			if err := initialize(transaction); err != nil {
				return err
			}
			return requireNoForeignKeyViolations(transaction)
		})
	})
}
