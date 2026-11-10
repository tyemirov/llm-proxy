package proxy

import (
	"context"
	"database/sql"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedQuoteMigrationIOFailuresRollbackAndRestart(t *testing.T) {
	cases := []struct {
		name, operation, fragment string
		nth                       int
		cleanupFails              bool
	}{
		{"initial schema read", "row", "SELECT sql FROM sqlite_master", 1, false},
		{"initial FK read", "row", "PRAGMA foreign_keys", 1, false},
		{"disable FK", "raw", "PRAGMA foreign_keys = OFF", 1, false},
		{"disabled FK read", "row", "PRAGMA foreign_keys", 2, false},
		{"writer lock", "raw", "UPDATE managed_journal_request_records SET state", 1, false},
		{"locked predecessor read", "row", "SELECT sql FROM sqlite_master", 2, false},
		{"migration predecessor read", "row", "SELECT sql FROM sqlite_master", 3, false},
		{"migration FK read", "row", "PRAGMA foreign_keys", 3, false},
		{"schema objects read", "row", "SELECT type,name,sql FROM sqlite_master", 1, false},
		{"constraint rebuild", "raw", "DROP TABLE", 1, false},
		{"current constraint rebuild", "raw", "DROP TABLE", 2, false},
		{"retained index restore", "raw", "CREATE INDEX", 1, false},
		{"restored objects read", "row", "SELECT type,name,sql FROM sqlite_master", 2, false},
		{"current schema read", "row", "SELECT sql FROM sqlite_master", 4, false},
		{"reference check", "row", "PRAGMA foreign_key_check", 1, false},
		{"restoration read", "row", "PRAGMA foreign_keys", 4, true},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			makePreviousQuoteSchema(t, fixture.database)
			sentinel := errors.New("controlled migration boundary failure")
			hits := 0
			name := "quoted:migration-io"
			fault := func(tx *gorm.DB) {
				if strings.Contains(tx.Statement.SQL.String(), scenario.fragment) {
					hits++
					if hits == scenario.nth {
						tx.AddError(sentinel)
					}
				}
			}
			callbacks := fixture.database.database.Callback()
			if scenario.operation == "row" {
				if err := callbacks.Row().Before("gorm:row").Register(name, fault); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := callbacks.Raw().Before("gorm:raw").Register(name, fault); err != nil {
					t.Fatal(err)
				}
			}
			err := initializeWithHostedQuoteBoundary(fixture.database.database, initializeHostedFinancialQuoteSchema)
			if scenario.operation == "row" {
				callbacks.Row().Remove(name)
			} else {
				callbacks.Raw().Remove(name)
			}
			if hits < scenario.nth || !errors.Is(err, sentinel) {
				t.Fatalf("hits=%d error=%v", hits, err)
			}
			pool, poolErr := fixture.database.database.DB()
			if poolErr != nil {
				t.Fatal(poolErr)
			}
			if scenario.cleanupFails {
				if pool.Ping() == nil {
					t.Fatal("unverified restore left pool usable")
				}
				return
			}
			var enabled int
			if err := fixture.database.database.Raw("PRAGMA foreign_keys").Scan(&enabled).Error; err != nil || enabled != 1 {
				t.Fatalf("FK=%d err=%v", enabled, err)
			}
			predecessor, err := quoteSchemaPredecessor(fixture.database.database)
			if err != nil || !predecessor {
				t.Fatalf("failed startup changed schema: %v", err)
			}
			if err := initializeWithHostedQuoteBoundary(fixture.database.database, initializeHostedFinancialQuoteSchema); err != nil {
				t.Fatalf("restart failed: %v", err)
			}
		})
	}
}

func TestHostedQuoteMigrationRejectsSchemaDrift(t *testing.T) {
	for _, scenario := range []string{"missing named check", "duplicate named check", "duplicate malformed check", "nested unsupported expression", "predecessor with quote table", "predecessor missing index", "current missing index", "quote missing index"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			db := fixture.database.database
			if !strings.HasPrefix(scenario, "current") && !strings.HasPrefix(scenario, "quote") {
				makePreviousQuoteSchema(t, fixture.database)
			}
			switch scenario {
			case "predecessor with quote table":
				if err := db.AutoMigrate(&managedFinancialQuoteRecord{}); err != nil {
					t.Fatal(err)
				}
			case "predecessor missing index", "current missing index":
				if err := db.Migrator().DropIndex(&managedJournalRequestRecord{}, "idx_journal_tenant_intent"); err != nil {
					t.Fatal(err)
				}
			case "quote missing index":
				if err := db.Migrator().DropIndex(&managedFinancialQuoteRecord{}, "idx_quote_tenant_key"); err != nil {
					t.Fatal(err)
				}
			default:
				var definition string
				if err := db.Raw("SELECT sql FROM sqlite_master WHERE name = 'managed_journal_request_records'").Scan(&definition).Error; err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "missing named check":
					definition = strings.ReplaceAll(definition, journalQuoteConstraint, "different_constraint")
				case "duplicate named check":
					definition += " CONSTRAINT " + journalQuoteConstraint + " CHECK(" + priorJournalQuoteExpression + ")"
				case "duplicate malformed check":
					definition += " CONSTRAINT " + journalQuoteConstraint + " CHECK(state = 'accepted')"
				case "nested unsupported expression":
					definition = strings.Replace(definition, priorJournalQuoteExpression, "state IN (('accepted'),'executing')", 1)
				}
				name := "quote:imported-schema"
				if err := db.Callback().Row().Before("gorm:row").Register(name, func(tx *gorm.DB) {
					if strings.Contains(tx.Statement.SQL.String(), "SELECT sql FROM sqlite_master") {
						tx.Statement.SQL.Reset()
						tx.Statement.SQL.WriteString("SELECT ?")
						tx.Statement.Vars = []any{definition}
					}
				}); err != nil {
					t.Fatal(err)
				}
				defer db.Callback().Row().Remove(name)
			}
			entered := false
			writer := false
			db.Callback().Raw().Before("gorm:raw").Register("quote:unexpected-writer", func(tx *gorm.DB) {
				if strings.HasPrefix(tx.Statement.SQL.String(), "UPDATE managed_journal_request_records SET state = state WHERE 0") {
					writer = true
				}
			})
			defer db.Callback().Raw().Remove("quote:unexpected-writer")
			err := initializeWithHostedQuoteBoundary(db, func(tx *gorm.DB) error { entered = true; return initializeHostedFinancialQuoteSchema(tx) })
			if err == nil {
				t.Fatal("schema drift permitted startup")
			}
			if (strings.Contains(scenario, "named check") || scenario == "duplicate malformed check" || scenario == "nested unsupported expression") && (entered || writer) {
				t.Fatal("malformed named check reached schema mutation transaction")
			}
		})
	}
}

func TestHostedQuoteMigrationReadbackEnforcement(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		nth, reported int
		closed        bool
	}{{"initial enforcement disabled", 1, 0, false}, {"disable readback enabled", 2, 1, false}, {"migration enforcement enabled", 3, 1, false}, {"restore readback disabled", 4, 0, true}} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			makePreviousQuoteSchema(t, fixture.database)
			db := fixture.database.database
			hits := 0
			name := "quote:pragma-readback"
			db.Callback().Row().Before("gorm:row").Register(name, func(tx *gorm.DB) {
				if tx.Statement.SQL.String() == "PRAGMA foreign_keys" {
					hits++
					if hits == scenario.nth {
						tx.Statement.SQL.Reset()
						tx.Statement.SQL.WriteString("SELECT ?")
						tx.Statement.Vars = []any{scenario.reported}
					}
				}
			})
			err := initializeWithHostedQuoteBoundary(db, initializeHostedFinancialQuoteSchema)
			db.Callback().Row().Remove(name)
			if err == nil || hits < scenario.nth {
				t.Fatalf("readback failure ignored hits=%d err=%v", hits, err)
			}
			pool, poolErr := db.DB()
			if poolErr != nil {
				t.Fatal(poolErr)
			}
			if scenario.closed {
				if pool.Ping() == nil {
					t.Fatal("unsafe readback returned usable pool")
				}
				return
			}
			var enabled int
			if err := db.Raw("PRAGMA foreign_keys").Scan(&enabled).Error; err != nil || enabled != 1 {
				t.Fatalf("FK=%d err=%v", enabled, err)
			}
			if err := initializeWithHostedQuoteBoundary(db, initializeHostedFinancialQuoteSchema); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHostedQuoteMigrationChecksEveryForeignKeyRow(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	makePreviousQuoteSchema(t, fixture.database)
	db := fixture.database.database
	err := initializeWithHostedQuoteBoundary(db, func(tx *gorm.DB) error {
		if err := initializeHostedFinancialQuoteSchema(tx); err != nil {
			return err
		}
		if err := tx.Exec("CREATE TABLE quote_reference_probe (parent_id TEXT REFERENCES managed_journal_request_records(id))").Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO quote_reference_probe(parent_id) VALUES ('missing-request')").Error
	})
	if !errors.Is(err, errManagedTenantSchemaMigration) {
		t.Fatalf("orphan admitted: %v", err)
	}
	if db.Migrator().HasTable("quote_reference_probe") {
		t.Fatal("failed migration retained orphan table")
	}
	if err := initializeWithHostedQuoteBoundary(db, initializeHostedFinancialQuoteSchema); err != nil {
		t.Fatal(err)
	}
}

func TestHostedQuoteMigrationRejectsLostAndExtraSchemaObjects(t *testing.T) {
	for _, scenario := range []string{"extra restored object", "final expression read", "final expression drift", "quote table creation"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newFundsAdmissionFixture(t)
			makePreviousQuoteSchema(t, fixture.database)
			db := fixture.database.database
			name := "quote:schema-final"
			hits := 0
			injected := false
			if scenario == "final expression read" || scenario == "final expression drift" {
				db.Callback().Row().Before("gorm:row").Register(name, func(tx *gorm.DB) {
					if tx.Statement.SQL.String() == "SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?" {
						hits++
						if hits == 4 {
							if scenario == "final expression read" {
								tx.AddError(errors.New("controlled final schema read failure"))
							} else {
								var definition string
								tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT sql FROM sqlite_master WHERE name = 'managed_journal_request_records'").Scan(&definition)
								definition = strings.Replace(definition, currentJournalQuoteExpression, priorJournalQuoteExpression, 1)
								tx.Statement.SQL.Reset()
								tx.Statement.SQL.WriteString("SELECT ?")
								tx.Statement.Vars = []any{definition}
							}
						}
					}
				})
				defer db.Callback().Row().Remove(name)
			} else if scenario == "quote table creation" {
				db.Callback().Raw().Before("gorm:raw").Register(name, func(tx *gorm.DB) {
					if strings.HasPrefix(tx.Statement.SQL.String(), "CREATE TABLE `managed_financial_quote_records`") {
						injected = true
						tx.AddError(errors.New("controlled quote table creation failure"))
					}
				})
				defer db.Callback().Raw().Remove(name)
			} else {
				db.Callback().Raw().After("gorm:raw").Register(name, func(tx *gorm.DB) {
					if !injected && strings.HasPrefix(tx.Statement.SQL.String(), "CREATE INDEX") {
						injected = true
						if err := tx.Session(&gorm.Session{NewDB: true}).Exec("CREATE INDEX quoted_unexpected_schema_object ON managed_journal_request_records(state)").Error; err != nil {
							tx.AddError(err)
						}
					}
				})
				defer db.Callback().Raw().Remove(name)
			}
			err := initializeWithHostedQuoteBoundary(db, initializeHostedFinancialQuoteSchema)
			db.Callback().Row().Remove(name)
			db.Callback().Raw().Remove(name)
			if err == nil {
				t.Fatalf("schema anomaly admitted hits=%d injected=%v", hits, injected)
			}
			if err := initializeWithHostedQuoteBoundary(db, initializeHostedFinancialQuoteSchema); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHostedQuoteMigrationReferenceIterationCancellation(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	makePreviousQuoteSchema(t, fixture.database)
	db := fixture.database.database
	var cancel context.CancelFunc
	db.Callback().Row().Before("gorm:row").Register("quote:reference-context", func(tx *gorm.DB) {
		if tx.Statement.SQL.String() == "PRAGMA foreign_key_check" {
			tx.Statement.Context, cancel = context.WithCancel(tx.Statement.Context)
		}
	})
	db.Callback().Row().After("gorm:row").Register("quote:reference-cancel", func(tx *gorm.DB) {
		if tx.Statement.SQL.String() == "PRAGMA foreign_key_check" && tx.Error == nil {
			cancel()
			rows, ok := tx.Statement.Dest.(*sql.Rows)
			if !ok {
				t.Error("reference check did not return SQL rows")
				return
			}
			deadline := time.Now().Add(time.Second)
			for rows.Err() == nil && time.Now().Before(deadline) {
				runtime.Gosched()
			}
			if rows.Err() == nil {
				t.Error("cancelled reference iterator did not terminate")
			}
		}
	})
	err := initializeWithHostedQuoteBoundary(db, initializeHostedFinancialQuoteSchema)
	db.Callback().Row().Remove("quote:reference-context")
	db.Callback().Row().Remove("quote:reference-cancel")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled reference iterator accepted: %v", err)
	}
	if err := initializeWithHostedQuoteBoundary(db, initializeHostedFinancialQuoteSchema); err != nil {
		t.Fatal(err)
	}
}

func TestHostedQuoteMigrationRejectsMissingJournal(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	db := fixture.database.database
	if err := db.Migrator().DropTable(&managedJournalRequestRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := initializeWithHostedQuoteBoundary(db, initializeHostedFinancialQuoteSchema); !errors.Is(err, errManagedTenantSchemaMigration) {
		t.Fatalf("missing journal admitted: %v", err)
	}
}

func TestHostedQuoteMigrationRechecksExactPredecessorAfterWriterLock(t *testing.T) {
	fixture := newFundsAdmissionFixture(t)
	makePreviousQuoteSchema(t, fixture.database)
	db := fixture.database.database
	hits := 0
	name := "quote:predecessor-drift"
	db.Callback().Row().Before("gorm:row").Register(name, func(tx *gorm.DB) {
		if tx.Statement.SQL.String() == "SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?" {
			hits++
			if hits == 2 {
				definition := "CREATE TABLE journal (state TEXT, CONSTRAINT " + journalQuoteConstraint + " CHECK(state IN ('unrecognized')))"
				tx.Statement.SQL.Reset()
				tx.Statement.SQL.WriteString("SELECT ?")
				tx.Statement.Vars = []any{definition}
			}
		}
	})
	err := initializeWithHostedQuoteBoundary(db, initializeHostedFinancialQuoteSchema)
	db.Callback().Row().Remove(name)
	if !errors.Is(err, errManagedTenantSchemaMigration) || hits < 2 {
		t.Fatalf("drift admitted hits=%d err=%v", hits, err)
	}
	if err := initializeWithHostedQuoteBoundary(db, initializeHostedFinancialQuoteSchema); err != nil {
		t.Fatal(err)
	}
}
