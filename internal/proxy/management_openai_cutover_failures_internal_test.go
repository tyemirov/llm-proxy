package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Include predecessor-only fields and physical columns so startup failure must
// roll back schema transfer as well as current routing and credential records.
func managedCutoverFailureState(t *testing.T, database *gorm.DB) string {
	t.Helper()
	state := struct {
		Current                  string
		Columns                  map[string][]string
		PredecessorConnections   []managedProviderConnectionRecord
		PredecessorUsageOutcomes []managedUsageEventSchemaOneFixtureRecord
	}{Current: openAIRetirementState(t, database), Columns: map[string][]string{}}
	tables, err := database.Migrator().GetTables()
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		columns, err := database.Migrator().ColumnTypes(table)
		if err != nil {
			t.Fatal(err)
		}
		for _, column := range columns {
			state.Columns[table] = append(state.Columns[table], column.Name())
		}
		sort.Strings(state.Columns[table])
	}
	if database.Migrator().HasTable(&managedProviderConnectionRecord{}) {
		if err := database.Order(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "tenant_id"}}, {Column: clause.Column{Name: "provider_id"}}, {Column: clause.Column{Name: "field_id"}}}}).Find(&state.PredecessorConnections).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}}).Find(&state.PredecessorUsageOutcomes).Error; err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func managedQueryOrdersBy(database *gorm.DB, column string, descending bool) bool {
	orderClause := clause.OrderBy{}
	order, ok := database.Statement.Clauses[orderClause.Name()].Expression.(clause.OrderBy)
	if !ok {
		return false
	}
	for _, item := range order.Columns {
		if item.Column.Name == column && item.Desc == descending {
			return true
		}
	}
	return false
}

func TestManagedOpenAICutoverLatestPredecessorMarkerFailurePreservesState(t *testing.T) {
	database, _, _, _ := openAIRetirementDatabase(t, true)
	before := managedCutoverFailureState(t, database)
	fault := fmt.Errorf("latest predecessor marker unavailable: %w", errInternalTestDatabase)
	fired := 0
	if err := database.Callback().Query().Before("gorm:query").Register("cutover-latest-marker", func(tx *gorm.DB) {
		if tx.Statement.Table == managedSchemaMigrationTable && managedQueryOrdersBy(tx, managedSchemaVersionColumn, true) {
			fired++
			tx.AddError(fault)
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
	database.Callback().Query().Remove("cutover-latest-marker")
	if fired != 1 || !errors.Is(err, fault) {
		t.Fatalf("latest predecessor marker hook=%d error=%v", fired, err)
	}
	if after := managedCutoverFailureState(t, database); after != before {
		t.Fatal("marker read failure changed predecessor schema or records")
	}
}

func TestManagedOpenAICutoverApplyAfterPredecessorMigrationRollsBack(t *testing.T) {
	database, _, _, _ := openAIRetirementDatabase(t, true)
	before := managedCutoverFailureState(t, database)
	fault := errors.New("cutover update failed after connection transfer")
	fired, transferCompleted := 0, false
	if err := database.Callback().Update().Before("gorm:update").Register("cutover-after-transfer", func(tx *gorm.DB) {
		if tx.Statement.Table != managedProviderProfileTable {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]any)
		if !ok || updates["text_model"] != "gpt-6-luna" {
			return
		}
		fired++
		transferCompleted = tx.Migrator().HasTable("managed_account_connection_records") && !tx.Migrator().HasTable(managedProviderConnectionTable)
		tx.AddError(fault)
	}); err != nil {
		t.Fatal(err)
	}
	err := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
	database.Callback().Update().Remove("cutover-after-transfer")
	if fired != 1 || !transferCompleted || !errors.Is(err, errManagedTenantSchemaMigration) || !strings.Contains(err.Error(), "operation=apply_openai_cutover") || !strings.Contains(err.Error(), fault.Error()) {
		t.Fatalf("apply hook=%d transferred=%t error=%v", fired, transferCompleted, err)
	}
	if after := managedCutoverFailureState(t, database); after != before {
		t.Fatal("late cutover failure changed predecessor credentials, assignments, models, timestamps, usage or schema")
	}
}

func TestManagedOpenAICutoverSchemaOneKeyedReadFailureRollsBackOutcomeTransfer(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "schema-one-late-read.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	seedManagedUsageSchemaOne(t, database)
	before := managedCutoverFailureState(t, database)
	fault := errors.New("keyed route normalization tenant read failed")
	fired, outcomeTransferred := 0, false
	if err := database.Callback().Query().Before("gorm:query").Register("cutover-schema-one-keyed", func(tx *gorm.DB) {
		if tx.Statement.Table == managedTenantTable && managedQueryOrdersBy(tx, "tenant_id", false) {
			fired++
			outcomeTransferred = managedTableHasColumn(tx.Migrator(), managedUsageEventTable, managedUsageOutcomeCodeColumn)
			tx.AddError(fault)
		}
	}); err != nil {
		t.Fatal(err)
	}
	err = initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
	database.Callback().Query().Remove("cutover-schema-one-keyed")
	if fired != 1 || !outcomeTransferred || !errors.Is(err, errManagedTenantSchemaMigration) || !strings.Contains(err.Error(), "operation=read table="+managedTenantTable) || !strings.Contains(err.Error(), fault.Error()) {
		t.Fatalf("keyed read hook=%d outcome_transferred=%t error=%v", fired, outcomeTransferred, err)
	}
	if after := managedCutoverFailureState(t, database); after != before {
		t.Fatal("keyed route failure retained partial outcome schema or usage changes")
	}
}

func TestManagedOpenAICutoverPredecessorRoutingValidationFailuresRollback(t *testing.T) {
	for _, version := range []int{managedGeminiRouteRetirementVersion, managedResolvedUsageRouteSchemaVersion} {
		t.Run(fmt.Sprintf("version-%d", version), func(t *testing.T) {
			var database *gorm.DB
			if version == managedGeminiRouteRetirementVersion {
				database, _, _, _ = newManagedResolvedUsageRouteMigrationFixture(t)
			} else {
				stage := ""
				database = newManagedUsageDispositionSchemaTwelveFixture(t, &stage, managedUsageEventSchemaTwelveRecord{})
			}
			before := managedCutoverFailureState(t, database)
			fault := errors.New("predecessor connection routing validation unavailable")
			fired := 0
			if err := database.Callback().Query().Before("gorm:query").Register("cutover-predecessor-validation", func(tx *gorm.DB) {
				_, connections := tx.Statement.Preloads["ProviderConnections"]
				_, profiles := tx.Statement.Preloads["ProviderProfiles"]
				if tx.Statement.Table == managedTenantTable && connections && profiles {
					fired++
					tx.AddError(fault)
				}
			}); err != nil {
				t.Fatal(err)
			}
			err := initializeManagedTenantSchema(database, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
			database.Callback().Query().Remove("cutover-predecessor-validation")
			if fired != 1 || !errors.Is(err, errManagedTenantSchemaMigration) || !strings.Contains(err.Error(), "operation=read table="+managedTenantTable) || !strings.Contains(err.Error(), fault.Error()) {
				t.Fatalf("version=%d validation hook=%d error=%v", version, fired, err)
			}
			if after := managedCutoverFailureState(t, database); after != before {
				t.Fatal("predecessor validation failure changed model selections, timestamps, usage, markers or schema")
			}
		})
	}
}
