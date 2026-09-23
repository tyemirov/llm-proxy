package proxy

import (
	"fmt"
	"slices"

	"gorm.io/gorm"
)

func initializeHostedSchema(database *gorm.DB) error {
	models := []any{&managedBillingAccountRecord{}, &managedPlatformConnectionRecord{}, &managedPlatformCredentialRecord{},
		&managedHostedCreationRecord{}, &managedHostedGrantRecord{}, &managedHostedGrantRevisionRecord{}, &managedHostedTenantAssignmentRecord{}}
	migrator := database.Migrator()
	present := 0
	for _, model := range models {
		if migrator.HasTable(model) {
			present++
		}
	}
	if present == 0 {
		if err := database.AutoMigrate(models...); err != nil {
			return fmt.Errorf("%w: operation=create_hosted_schema: %w", errManagedTenantSchemaMigration, err)
		}
		if err := createManagedAssignmentConstraints(database); err != nil {
			return fmt.Errorf("%w: operation=create_hosted_schema: %w", errManagedTenantSchemaMigration, err)
		}
		return validateHostedRecords(database)
	}
	if present != len(models) {
		return fmt.Errorf("%w: operation=validate_hosted_schema reason=partial_schema", errManagedTenantSchemaMigration)
	}
	for _, model := range models {
		if err := validateHostedTable(database, model); err != nil {
			return fmt.Errorf("%w: operation=validate_hosted_schema: %w", errManagedTenantSchemaMigration, err)
		}
	}
	if err := validateManagedAssignmentConstraints(database); err != nil {
		return fmt.Errorf("%w: operation=validate_hosted_schema: %w", errManagedTenantSchemaMigration, err)
	}
	return validateHostedRecords(database)
}

func validateHostedRecords(database *gorm.DB) error {
	for _, invariant := range []struct{ name, query string }{
		{"foreign_keys", `SELECT COUNT(*) FROM pragma_foreign_key_check`},
		{"platform_versions", `SELECT COUNT(*) FROM managed_platform_connection_records AS connection
			WHERE connection.version != (SELECT COUNT(*) FROM managed_platform_credential_records WHERE connection_id = connection.id)
			OR connection.version != (SELECT COALESCE(MAX(version),0) FROM managed_platform_credential_records WHERE connection_id = connection.id)`},
		{"grant_ownership", `SELECT COUNT(*) FROM managed_hosted_grant_records AS grant_record
			JOIN managed_billing_account_records AS account ON account.id = grant_record.billing_account_id
			JOIN managed_tenant_records AS tenant ON tenant.tenant_id = grant_record.tenant_id
			JOIN managed_platform_connection_records AS connection ON connection.id = grant_record.platform_connection_id
			WHERE account.owner_user_id != tenant.owner_user_id OR connection.provider != grant_record.provider`},
		{"grant_audit", `SELECT COUNT(*) FROM managed_hosted_grant_records AS grant_record
			WHERE grant_record.revision != (SELECT COUNT(*) FROM managed_hosted_grant_revision_records WHERE grant_id = grant_record.id)
			OR grant_record.revision != (SELECT COALESCE(MAX(revision),0) FROM managed_hosted_grant_revision_records WHERE grant_id = grant_record.id)
			OR NOT EXISTS (SELECT 1 FROM managed_hosted_grant_revision_records WHERE grant_id = grant_record.id AND revision = grant_record.revision AND state = grant_record.state)`},
		{"hosted_assignments", `SELECT COUNT(*) FROM managed_hosted_tenant_assignment_records AS assignment
			JOIN managed_hosted_grant_records AS grant_record ON grant_record.id = assignment.grant_id
			WHERE assignment.tenant_id != grant_record.tenant_id OR assignment.provider_id != grant_record.provider
			OR EXISTS (SELECT 1 FROM managed_tenant_connection_records WHERE tenant_id = assignment.tenant_id AND provider_id = assignment.provider_id)
			OR NOT EXISTS (SELECT 1 FROM managed_provider_profile_records WHERE tenant_id = assignment.tenant_id AND provider_id = assignment.provider_id)`},
	} {
		var invalid int64
		if err := database.Raw(invariant.query).Scan(&invalid).Error; err != nil {
			return fmt.Errorf("%w: operation=validate_hosted_records invariant=%s: %w", errManagedTenantSchemaMigration, invariant.name, err)
		}
		if invalid != 0 {
			return fmt.Errorf("%w: operation=validate_hosted_records invariant=%s invalid=%d", errManagedTenantSchemaMigration, invariant.name, invalid)
		}
	}
	return nil
}

func validateHostedTable(database *gorm.DB, model any) error {
	statement := &gorm.Statement{DB: database}
	if err := statement.Parse(model); err != nil {
		return fmt.Errorf("read hosted schema definition: %w", err)
	}
	migrator := database.Migrator()
	columns, err := migrator.ColumnTypes(model)
	if err != nil {
		return fmt.Errorf("inspect table=%s: %w", statement.Schema.Table, err)
	}
	if len(columns) != len(statement.Schema.DBNames) {
		return fmt.Errorf("table=%s reason=column_count", statement.Schema.Table)
	}
	for _, column := range columns {
		field, exists := statement.Schema.FieldsByDBName[column.Name()]
		if !exists {
			return fmt.Errorf("table=%s column=%s reason=unknown", statement.Schema.Table, column.Name())
		}
		primary, primaryKnown := column.PrimaryKey()
		if !primaryKnown || primary != field.PrimaryKey {
			return fmt.Errorf("table=%s column=%s reason=primary_key", statement.Schema.Table, column.Name())
		}
		nullable, nullableKnown := column.Nullable()
		if field.NotNull && (!nullableKnown || nullable) {
			return fmt.Errorf("table=%s column=%s reason=nullable", statement.Schema.Table, column.Name())
		}
	}
	for name := range statement.Schema.ParseCheckConstraints() {
		if !migrator.HasConstraint(model, name) {
			return fmt.Errorf("table=%s constraint=%s reason=missing", statement.Schema.Table, name)
		}
	}
	for _, relation := range statement.Schema.Relationships.Relations {
		constraint := relation.ParseConstraint()
		if constraint != nil && constraint.Schema == statement.Schema && !migrator.HasConstraint(model, constraint.Name) {
			return fmt.Errorf("table=%s constraint=%s reason=missing", statement.Schema.Table, constraint.Name)
		}
	}
	expectedIndexes := statement.Schema.ParseIndexes()
	if len(expectedIndexes) == 0 {
		return nil
	}
	indexes, err := migrator.GetIndexes(model)
	if err != nil {
		return fmt.Errorf("inspect table=%s indexes: %w", statement.Schema.Table, err)
	}
	actualIndexes := make(map[string]gorm.Index, len(indexes))
	for _, index := range indexes {
		actualIndexes[index.Name()] = index
	}
	for _, expected := range expectedIndexes {
		actual, exists := actualIndexes[expected.Name]
		if !exists {
			return fmt.Errorf("table=%s index=%s reason=missing", statement.Schema.Table, expected.Name)
		}
		unique, known := actual.Unique()
		expectedColumns := make([]string, len(expected.Fields))
		for position, field := range expected.Fields {
			expectedColumns[position] = field.DBName
		}
		if !known || unique != (expected.Class == "UNIQUE") || !slices.Equal(actual.Columns(), expectedColumns) {
			return fmt.Errorf("table=%s index=%s reason=definition", statement.Schema.Table, expected.Name)
		}
	}
	return nil
}
