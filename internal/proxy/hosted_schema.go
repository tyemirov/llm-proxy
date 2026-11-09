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

func validateHostedTable(database *gorm.DB, model any) error {
	statement := &gorm.Statement{DB: database}
	// Each caller supplies a fixed repository model. Parse describes that Go
	// type; the database shape is checked through the migrator below.
	_ = statement.Parse(model)
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
