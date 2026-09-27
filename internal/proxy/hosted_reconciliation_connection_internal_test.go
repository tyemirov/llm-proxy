package proxy

import (
	"database/sql"
	"errors"
	"testing"

	"gorm.io/gorm"
)

var errReconciliationConnectionAccess = errors.New("controlled_reconciliation_connection_access_failure")

type reconciliationConnectionFailureDialector struct {
	gorm.Dialector
	test *testing.T
}

func (dialector reconciliationConnectionFailureDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	connection, err := database.DB()
	if err != nil {
		return err
	}
	dialector.test.Cleanup(func() {
		if err := connection.Close(); err != nil {
			dialector.test.Error(err)
		}
	})
	database.ConnPool = reconciliationConnectionFailurePool{database.ConnPool}
	return nil
}

type reconciliationConnectionFailurePool struct{ gorm.ConnPool }

func (reconciliationConnectionFailurePool) GetDBConn() (*sql.DB, error) {
	return nil, errReconciliationConnectionAccess
}
