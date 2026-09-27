package proxy

import (
	"errors"
	"reflect"
	"strings"
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
		{"missing-rating-table", "partial hosted rating schema", []string{"ALTER TABLE managed_charge_adjustment_records RENAME TO retained_charge_adjustment_records"}, []string{"ALTER TABLE retained_charge_adjustment_records RENAME TO managed_charge_adjustment_records"}},
		{"missing-rating-column", "validate hosted rating schema", []string{"ALTER TABLE managed_price_snapshot_records RENAME COLUMN digest TO retained_digest"}, []string{"ALTER TABLE managed_price_snapshot_records RENAME COLUMN retained_digest TO digest"}},
		{"missing-funding-order-table", "partial funding order schema", []string{"ALTER TABLE managed_payment_checkout_records RENAME TO retained_payment_checkout_records"}, []string{"ALTER TABLE retained_payment_checkout_records RENAME TO managed_payment_checkout_records"}},
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
	columns, err := migrator.Migrator.ColumnTypes(value)
	if err != nil {
		return nil, err
	}
	if migrator.rejects(value, "primary_key") {
		for index, column := range columns {
			if column.Name() == "id" {
				columns[index] = financialColumnWithoutPrimaryKey{column}
			}
		}
	}
	if migrator.rejects(value, "nullable") {
		for index, column := range columns {
			if column.Name() == "currency" {
				columns[index] = financialNullableColumn{column}
			}
		}
	}
	return columns, nil
}

type financialColumnMetadata interface{ gorm.ColumnType }

type financialColumnWithoutPrimaryKey struct{ financialColumnMetadata }

func (financialColumnWithoutPrimaryKey) PrimaryKey() (bool, bool) { return false, true }

type financialNullableColumn struct{ financialColumnMetadata }

func (financialNullableColumn) Nullable() (bool, bool) { return true, true }

func (migrator financialSchemaFailureMigrator) HasConstraint(value any, name string) bool {
	if migrator.rejects(value, "constraint:"+name) {
		return false
	}
	return migrator.Migrator.HasConstraint(value, name)
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
		{"managed_billing_account_records", "primary_key", "column=id reason=primary_key"},
		{"managed_billing_account_records", "nullable", "column=currency reason=nullable"},
		{"managed_billing_account_records", "constraint:chk_managed_billing_account_records_currency", "constraint=chk_managed_billing_account_records_currency reason=missing"},
		{"managed_billing_account_records", "constraint:fk_managed_billing_account_records_owner", "constraint=fk_managed_billing_account_records_owner reason=missing"},
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

type hostedInvariantReadFailureDialector struct {
	gorm.Dialector
	queryFragment string
	failures      *atomic.Int64
}

func (dialector hostedInvariantReadFailureDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	return database.Callback().Row().Before("gorm:row").Register("test:hosted_invariant_read", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), dialector.queryFragment) {
			dialector.failures.Add(1)
			tx.AddError(errors.New("controlled_hosted_invariant_read_failure"))
		}
	})
}

func TestHostedFundsStartupInvariantReadFailuresPreserveFundedAccounts(t *testing.T) {
	for _, scenario := range []struct{ invariant, queryFragment string }{
		{"foreign_keys", "SELECT COUNT(*) FROM pragma_foreign_key_check"},
		{"platform_versions", "FROM managed_platform_connection_records AS connection"},
		{"grant_ownership", "account.owner_user_id != tenant.owner_user_id"},
		{"grant_audit", "grant_record.revision !="},
		{"hosted_assignments", "FROM managed_hosted_tenant_assignment_records AS assignment"},
	} {
		t.Run(scenario.invariant, func(t *testing.T) {
			fixture := newPaymentStartupFixture(t)
			configuration := fixture.configuration
			var failures atomic.Int64
			configuration.Management.DatabaseDialector = hostedInvariantReadFailureDialector{
				Dialector: configuration.Management.DatabaseDialector, queryFragment: scenario.queryFragment, failures: &failures,
			}
			before := financialSchemaSnapshot(t, fixture.database.database)
			for attempt := int64(1); attempt <= 2; attempt++ {
				fixture.reject(t, configuration, "operation=validate_hosted_records invariant="+scenario.invariant+": controlled_hosted_invariant_read_failure")
				if failures.Load() != attempt {
					t.Fatalf("invariant read failures=%d want=%d", failures.Load(), attempt)
				}
				if !reflect.DeepEqual(before, financialSchemaSnapshot(t, fixture.database.database)) || !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
					t.Fatal("failed invariant read changed schema or financial resources")
				}
			}
			fixture.recover(t)
		})
	}
}

func TestHostedFundsStartupAssignmentConstraintReadFailurePreservesFundedAccount(t *testing.T) {
	fixture := newPaymentStartupFixture(t)
	configuration := fixture.configuration
	var failures atomic.Int64
	configuration.Management.DatabaseDialector = hostedInvariantReadFailureDialector{
		Dialector:     configuration.Management.DatabaseDialector,
		queryFragment: "SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name =",
		failures:      &failures,
	}
	before := financialSchemaSnapshot(t, fixture.database.database)
	for attempt := int64(1); attempt <= 2; attempt++ {
		fixture.reject(t, configuration, "read assignment constraint")
		if failures.Load() != attempt {
			t.Fatalf("constraint read failures=%d want=%d", failures.Load(), attempt)
		}
		if !reflect.DeepEqual(before, financialSchemaSnapshot(t, fixture.database.database)) || !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
			t.Fatal("failed constraint read changed schema or financial resources")
		}
	}
	fixture.recover(t)
}

type routingStartupReadFailureDialector struct {
	gorm.Dialector
	table    string
	failures *atomic.Int64
}

func (dialector routingStartupReadFailureDialector) Initialize(database *gorm.DB) error {
	if err := dialector.Dialector.Initialize(database); err != nil {
		return err
	}
	var routingValidation atomic.Bool
	return database.Callback().Query().Before("gorm:query").Register("test:routing_startup_read", func(tx *gorm.DB) {
		if tx.DryRun {
			return
		}
		if _, exists := tx.Statement.Preloads["ConnectionAssignments.Connection.Fields"]; exists {
			routingValidation.Store(true)
		}
		if routingValidation.Load() && tx.Statement.Table == dialector.table {
			dialector.failures.Add(1)
			tx.AddError(errors.New("controlled_routing_startup_read_failure"))
		}
	})
}

func TestHostedFundsStartupRoutingReadFailuresPreserveFundedAccounts(t *testing.T) {
	for _, scenario := range []struct{ table, reason string }{
		{"managed_tenant_records", "read tenants for routing validation"},
		{"managed_tenant_connection_records", "read tenants for routing validation"},
		{"managed_provider_profile_records", "read tenants for routing validation"},
		{"managed_hosted_tenant_assignment_records", "read hosted assignments for tenant"},
		{"managed_hosted_grant_records", "read hosted assignments for tenant"},
	} {
		t.Run(scenario.table, func(t *testing.T) {
			fixture := newPaymentStartupFixture(t)
			configuration := fixture.configuration
			var failures atomic.Int64
			configuration.Management.DatabaseDialector = routingStartupReadFailureDialector{
				Dialector: configuration.Management.DatabaseDialector, table: scenario.table, failures: &failures,
			}
			before := financialSchemaSnapshot(t, fixture.database.database)
			for attempt := int64(1); attempt <= 2; attempt++ {
				fixture.reject(t, configuration, scenario.reason)
				if failures.Load() != attempt {
					t.Fatalf("routing read failures=%d want=%d", failures.Load(), attempt)
				}
				if !reflect.DeepEqual(before, financialSchemaSnapshot(t, fixture.database.database)) || !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
					t.Fatal("failed routing read changed schema or financial resources")
				}
			}
			fixture.recover(t)
		})
	}
}

func TestHostedFundsStartupRejectsMalformedGrantScopeWithoutFinancialChanges(t *testing.T) {
	fixture := newPaymentStartupFixture(t)
	var retained managedHostedGrantRecord
	if err := fixture.database.database.Where("id = ?", hostedJournalFixtureGrantID).First(&retained).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", retained.ID).UpdateColumn("offerings", []byte(`{"invalid":`)).Error; err != nil {
		t.Fatal(err)
	}
	before := financialSchemaSnapshot(t, fixture.database.database)
	for range 2 {
		fixture.reject(t, fixture.configuration, "read grant scope for tenant managed-first provider openai")
		if !reflect.DeepEqual(before, financialSchemaSnapshot(t, fixture.database.database)) || !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) {
			t.Fatal("malformed grant scope changed schema or financial resources")
		}
	}
	if err := fixture.database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", retained.ID).UpdateColumn("offerings", retained.Offerings).Error; err != nil {
		t.Fatal(err)
	}
	fixture.recover(t)
}
