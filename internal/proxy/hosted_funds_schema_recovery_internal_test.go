package proxy

import (
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

func TestHostedFundsStartupRejectsIncompleteStorageWithoutFinancialChanges(t *testing.T) {
	for _, scenario := range []struct {
		name, reason     string
		corrupt, restore []string
	}{
		{"missing-funds-table", "partial hosted funds schema", []string{"ALTER TABLE managed_funds_credit_records RENAME TO retained_funds_credit_records"}, []string{"ALTER TABLE retained_funds_credit_records RENAME TO managed_funds_credit_records"}},
		{"missing-funds-column", "validate hosted funds schema", []string{"ALTER TABLE managed_funds_account_records RENAME COLUMN remainder_numerator TO retained_numerator"}, []string{"ALTER TABLE managed_funds_account_records RENAME COLUMN retained_numerator TO remainder_numerator"}},
		{"extra-ledger-column", "reason=column_count", []string{"ALTER TABLE ledger_entries ADD COLUMN obsolete_amount TEXT"}, []string{"ALTER TABLE ledger_entries DROP COLUMN obsolete_amount"}},
		{"missing-account-index", "reason=missing", []string{"DROP INDEX idx_ledger_accounts_tenant_user_ledger"}, []string{"CREATE UNIQUE INDEX idx_ledger_accounts_tenant_user_ledger ON ledger_accounts(tenant_id,user_id,ledger_id)"}},
		{"nonunique-account-index", "reason=definition", []string{"DROP INDEX idx_ledger_accounts_tenant_user_ledger", "CREATE INDEX idx_ledger_accounts_tenant_user_ledger ON ledger_accounts(tenant_id,user_id,ledger_id)"}, []string{"DROP INDEX idx_ledger_accounts_tenant_user_ledger", "CREATE UNIQUE INDEX idx_ledger_accounts_tenant_user_ledger ON ledger_accounts(tenant_id,user_id,ledger_id)"}},
		{"reordered-account-index", "reason=definition", []string{"DROP INDEX idx_ledger_accounts_tenant_user_ledger", "CREATE UNIQUE INDEX idx_ledger_accounts_tenant_user_ledger ON ledger_accounts(user_id,tenant_id,ledger_id)"}, []string{"DROP INDEX idx_ledger_accounts_tenant_user_ledger", "CREATE UNIQUE INDEX idx_ledger_accounts_tenant_user_ledger ON ledger_accounts(tenant_id,user_id,ledger_id)"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newPaymentStartupFixture(t)
			for _, statement := range scenario.corrupt {
				if err := fixture.database.database.Exec(statement).Error; err != nil {
					t.Fatal(err)
				}
			}
			before := financialSchemaSnapshot(t, fixture.database.database)
			for range 2 {
				fixture.reject(t, fixture.configuration, scenario.reason)
			}
			if !reflect.DeepEqual(before, financialSchemaSnapshot(t, fixture.database.database)) {
				t.Fatal("rejected financial startup modified the retained schema")
			}
			for _, statement := range scenario.restore {
				if err := fixture.database.database.Exec(statement).Error; err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
				t.Fatal("rejected financial startup changed receipts or funds")
			}
			fixture.recover(t)
		})
	}
}

func financialSchemaSnapshot(t *testing.T, database *gorm.DB) []map[string]any {
	t.Helper()
	var schema []map[string]any
	if err := database.Raw("SELECT name, sql FROM sqlite_master ORDER BY name").Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	return schema
}

type financialSchemaFailureDialector struct {
	gorm.Dialector
	table, operation string
	failures         *atomic.Int64
}

func (dialector financialSchemaFailureDialector) Migrator(database *gorm.DB) gorm.Migrator {
	return financialSchemaFailureMigrator{Migrator: dialector.Dialector.Migrator(database), database: database, table: dialector.table, operation: dialector.operation, failures: dialector.failures}
}

type financialSchemaFailureMigrator struct {
	gorm.Migrator
	database         *gorm.DB
	table, operation string
	failures         *atomic.Int64
}

func (migrator financialSchemaFailureMigrator) rejects(value any, operation string) bool {
	statement := &gorm.Statement{DB: migrator.database}
	if statement.Parse(value) != nil || statement.Schema.Table != migrator.table || operation != migrator.operation {
		return false
	}
	migrator.failures.Add(1)
	return true
}

func (migrator financialSchemaFailureMigrator) ColumnTypes(value any) ([]gorm.ColumnType, error) {
	if migrator.rejects(value, "columns") {
		return nil, errors.New("controlled_financial_column_inspection_failure")
	}
	return migrator.Migrator.ColumnTypes(value)
}

func (migrator financialSchemaFailureMigrator) GetIndexes(value any) ([]gorm.Index, error) {
	if migrator.rejects(value, "indexes") {
		return nil, errors.New("controlled_financial_index_inspection_failure")
	}
	return migrator.Migrator.GetIndexes(value)
}

func TestHostedFundsStartupMetadataFailuresPreserveFundedAccounts(t *testing.T) {
	for _, scenario := range []struct{ table, operation, reason string }{
		{"ledger_accounts", "columns", "inspect table=ledger_accounts"},
		{"ledger_accounts", "indexes", "inspect table=ledger_accounts indexes"},
		{"managed_funds_account_records", "columns", "inspect table=managed_funds_account_records"},
		{"managed_funds_reservation_records", "indexes", "inspect table=managed_funds_reservation_records indexes"},
	} {
		t.Run(scenario.table+"/"+scenario.operation, func(t *testing.T) {
			fixture := newPaymentStartupFixture(t)
			configuration := fixture.configuration
			var failures atomic.Int64
			configuration.Management.DatabaseDialector = financialSchemaFailureDialector{
				Dialector: configuration.Management.DatabaseDialector, table: scenario.table, operation: scenario.operation, failures: &failures,
			}
			before := financialSchemaSnapshot(t, fixture.database.database)
			fixture.reject(t, configuration, scenario.reason)
			if failures.Load() != 1 {
				t.Fatalf("metadata failure count=%d want=1", failures.Load())
			}
			if !reflect.DeepEqual(before, financialSchemaSnapshot(t, fixture.database.database)) || !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
				t.Fatal("metadata inspection failure changed the schema or financial resources")
			}
			fixture.recover(t)
		})
	}
}
