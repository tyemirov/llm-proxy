package proxy

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type qualificationFaultFixture struct {
	management     *gormManagedTenantDatabase
	managementPath string
	policy         QualificationBudgetConfiguration
	catalog        ModelCatalog
	budget         *qualificationBudget
}

func newQualificationFaultFixture(t *testing.T) *qualificationFaultFixture {
	t.Helper()
	management := newCanonicalGORMFixture(t, time.Now().UTC())
	var managementPath string
	if err := management.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&managementPath).Error; err != nil {
		t.Fatal(err)
	}
	fixture := &qualificationFaultFixture{management: management, managementPath: managementPath, catalog: internalCanonicalProviderCatalog().ModelCatalog(), policy: QualificationBudgetConfiguration{
		BudgetID: "fault-budget", TenantID: "managed-first", OwnerUserID: "owner", ExpiresAt: time.Now().Add(time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339), RemainingMicroUSD: 1000000, ReceiptDatabasePath: filepath.Join(filepath.Dir(managementPath), "fault-receipts.sqlite"), MaxAttemptsPerRequest: 2, Offerings: []QualificationBudgetOffering{{Provider: "cerebras", Model: "qwen3.8-27b", MaxOutputTokens: 128}},
	}}
	if err := validateQualificationBudget(&fixture.policy, managementPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if fixture.budget != nil {
			if err := fixture.budget.close(); err != nil {
				t.Error(err)
			}
		}
	})
	return fixture
}
func (fixture *qualificationFaultFixture) initialize(t *testing.T) {
	t.Helper()
	budget, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
	if err != nil {
		t.Fatal(err)
	}
	fixture.budget = budget
}
func (fixture *qualificationFaultFixture) stop(t *testing.T) {
	t.Helper()
	if err := fixture.budget.close(); err != nil {
		t.Fatal(err)
	}
	fixture.budget = nil
}
func qualificationFaultJournal(t *testing.T, path string) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	return database
}
func qualificationFaultExec(t *testing.T, database *gorm.DB, statement string, args ...any) {
	t.Helper()
	if err := database.Exec(statement, args...).Error; err != nil {
		t.Fatal(err)
	}
}
func requireQualificationFault(t *testing.T, err error, fragment string) {
	t.Helper()
	if err == nil || fragment != "" && !strings.Contains(err.Error(), fragment) {
		t.Fatalf("expected rejection containing %q, got %v", fragment, err)
	}
}

func TestQualificationBudgetFaultsConfigurationBoundary(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*QualificationBudgetConfiguration, string)
	}{
		{"budget identity", func(policy *QualificationBudgetConfiguration, _ string) { policy.BudgetID = " padded " }},
		{"empty tenant", func(policy *QualificationBudgetConfiguration, _ string) { policy.TenantID = "" }},
		{"zero allowance", func(policy *QualificationBudgetConfiguration, _ string) { policy.RemainingMicroUSD = 0 }},
		{"attempt ceiling", func(policy *QualificationBudgetConfiguration, _ string) { policy.MaxAttemptsPerRequest = 33 }},
		{"expiry encoding", func(policy *QualificationBudgetConfiguration, _ string) { policy.ExpiresAt = "tomorrow" }},
		{"expiry offset", func(policy *QualificationBudgetConfiguration, _ string) {
			policy.ExpiresAt = "2026-12-01T00:00:00+00:00"
		}},
		{"relative journal", func(policy *QualificationBudgetConfiguration, _ string) { policy.ReceiptDatabasePath = "receipts.db" }},
		{"management collision", func(policy *QualificationBudgetConfiguration, path string) { policy.ReceiptDatabasePath = path }},
		{"different volume directory", func(policy *QualificationBudgetConfiguration, path string) {
			policy.ReceiptDatabasePath = filepath.Join(filepath.Dir(path), "other", "receipts.db")
		}},
		{"duplicate offering", func(policy *QualificationBudgetConfiguration, _ string) {
			policy.Offerings = append(policy.Offerings, policy.Offerings[0])
		}},
		{"output ceiling", func(policy *QualificationBudgetConfiguration, _ string) { policy.Offerings[0].MaxOutputTokens = 32769 }},
		{"unnamed verification", func(policy *QualificationBudgetConfiguration, _ string) {
			policy.VerificationConnections = []QualificationBudgetConnection{{Provider: "cerebras"}}
		}},
		{"verification normalization", func(policy *QualificationBudgetConfiguration, _ string) {
			policy.VerificationConnections = []QualificationBudgetConnection{{Provider: "cerebras", Name: " padded "}}
		}},
		{"duplicate verification", func(policy *QualificationBudgetConfiguration, _ string) {
			policy.VerificationConnections = []QualificationBudgetConnection{{Provider: "cerebras", Name: "Account"}, {Provider: "cerebras", Name: "account"}}
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			scenario.change(&fixture.policy, fixture.managementPath)
			requireQualificationFault(t, validateQualificationBudget(&fixture.policy, fixture.managementPath), "qualification_budget_denied")
		})
	}
}

func TestQualificationBudgetFaultsInitialization(t *testing.T) {
	t.Run("managed database required", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		_, err := newQualificationBudget(&fixture.policy, nil, fixture.catalog)
		requireQualificationFault(t, err, "managed database required")
	})
	t.Run("tenant unavailable", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		fixture.policy.TenantID = "missing"
		_, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
		requireQualificationFault(t, err, "tenant unavailable")
	})
	t.Run("owner mismatch", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		fixture.policy.OwnerUserID = "other"
		_, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
		requireQualificationFault(t, err, "owner differs")
	})
	t.Run("unknown offering", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		fixture.policy.Offerings[0].Model = "unqualified"
		_, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
		requireQualificationFault(t, err, "offering or token ceilings unavailable")
	})
	t.Run("unregistered existing receipt", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		qualificationFaultJournal(t, fixture.policy.ReceiptDatabasePath)
		_, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
		requireQualificationFault(t, err, "unregistered journal path")
	})
	t.Run("lock path unavailable", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		if err := os.Mkdir(fixture.policy.ReceiptDatabasePath+".lock", 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
		requireQualificationFault(t, err, "journal lock unavailable")
	})
	t.Run("durable initialization marker failure", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		if err := fixture.management.database.AutoMigrate(&qualificationBudgetRegistry{}); err != nil {
			t.Fatal(err)
		}
		qualificationFaultExec(t, fixture.management.database, `CREATE TRIGGER reject_budget_marker BEFORE INSERT ON qualification_budget_registries BEGIN SELECT RAISE(ABORT,'controlled_marker_failure'); END`)
		_, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
		requireQualificationFault(t, err, "controlled_marker_failure")
		if _, err := os.Stat(fixture.policy.ReceiptDatabasePath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("receipt created without registry authority: %v", err)
		}
	})
	t.Run("another active identity", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		fixture.initialize(t)
		other := fixture.policy
		other.BudgetID = "another-budget"
		other.ReceiptDatabasePath = filepath.Join(filepath.Dir(other.ReceiptDatabasePath), "another.sqlite")
		_, err := newQualificationBudget(&other, fixture.management, fixture.catalog)
		requireQualificationFault(t, err, "another initialized budget remains active")
	})
	t.Run("registry query unavailable", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		registerManagedGORMError(t, fixture.management.database, "qualification_registry_query", "query", "qualification_budget_registries", errors.New("controlled_registry_query"))
		_, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
		requireQualificationFault(t, err, "controlled_registry_query")
	})
}

func TestQualificationBudgetFaultsRestartRejectsCorruption(t *testing.T) {
	for _, scenario := range []struct {
		name, sql, fragment string
		managementSQL       string
	}{
		{name: "missing state", sql: `DELETE FROM qualification_budget_states`, fragment: "journal identity"},
		{name: "closed state", sql: `UPDATE qualification_budget_states SET closed = true`, fragment: "journal identity"},
		{name: "policy mismatch", sql: `UPDATE qualification_budget_states SET policy = '{}'`, fragment: "journal identity"},
		{name: "frozen digest", sql: `UPDATE qualification_budget_states SET frozen = '[]'`, fragment: "frozen catalog integrity"},
		{name: "frozen encoding", sql: `UPDATE qualification_budget_states SET frozen = 'invalid-json'`, managementSQL: `UPDATE qualification_budget_registries SET frozen_digest = ?`, fragment: "frozen catalog"},
		{name: "negative allowance", sql: `UPDATE qualification_budget_states SET remaining = -1`, fragment: "invalid remaining allowance"},
		{name: "inflated allowance", sql: `UPDATE qualification_budget_states SET remaining = 1000001`, fragment: "invalid remaining allowance"},
		{name: "lost attempt table", sql: `DROP TABLE qualification_budget_attempts`, fragment: "no such table"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			fixture.initialize(t)
			fixture.stop(t)
			database := qualificationFaultJournal(t, fixture.policy.ReceiptDatabasePath)
			qualificationFaultExec(t, database, scenario.sql)
			if scenario.managementSQL != "" {
				qualificationFaultExec(t, fixture.management.database, scenario.managementSQL, sha256Hex("invalid-json"))
			}
			_, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
			requireQualificationFault(t, err, scenario.fragment)
		})
	}
}

func TestQualificationBudgetFaultsOperatorBoundaries(t *testing.T) {
	t.Run("unknown action", func(t *testing.T) {
		requireQualificationFault(t, OperateQualificationBudget("unused", "budget", "reset", io.Discard), "operator action")
	})
	t.Run("zero reconciliation identity", func(t *testing.T) {
		requireQualificationFault(t, ReconcileQualificationBudgetMaximum("unused", "budget", 0, io.Discard), "qualification_budget_denied")
	})
	t.Run("missing management store", func(t *testing.T) {
		requireQualificationFault(t, OperateQualificationBudget(filepath.Join(t.TempDir(), "absent.db"), "budget", "inspect", io.Discard), "management database missing")
	})
	t.Run("malformed management store", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.db")
		if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
			t.Fatal(err)
		}
		requireQualificationFault(t, OperateQualificationBudget(path, "budget", "inspect", io.Discard), "")
	})
	for _, scenario := range []struct{ name, mainSQL, journalSQL, fragment string }{
		{name: "registry unavailable", mainSQL: `DELETE FROM qualification_budget_registries`, fragment: "registry unavailable"},
		{name: "registry encoding", mainSQL: `UPDATE qualification_budget_registries SET policy = 'invalid-json'`, fragment: "qualification_budget_denied"},
		{name: "journal identity", journalSQL: `DELETE FROM qualification_budget_states`, fragment: "journal identity"},
		{name: "allowance conservation", journalSQL: `UPDATE qualification_budget_states SET remaining = remaining - 1`, fragment: "allowance conservation failed"},
		{name: "journal frozen encoding", mainSQL: `UPDATE qualification_budget_registries SET frozen_digest = ?`, journalSQL: `UPDATE qualification_budget_states SET frozen = 'invalid-json'`, fragment: "qualification_budget_denied"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			fixture.initialize(t)
			if scenario.mainSQL != "" {
				if strings.Contains(scenario.mainSQL, "?") {
					qualificationFaultExec(t, fixture.management.database, scenario.mainSQL, sha256Hex("invalid-json"))
				} else {
					qualificationFaultExec(t, fixture.management.database, scenario.mainSQL)
				}
			}
			if scenario.journalSQL != "" {
				qualificationFaultExec(t, fixture.budget.database, scenario.journalSQL)
			}
			requireQualificationFault(t, OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "inspect", io.Discard), scenario.fragment)
		})
	}
	t.Run("receipt file absent", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		fixture.initialize(t)
		fixture.stop(t)
		if err := os.Remove(fixture.policy.ReceiptDatabasePath); err != nil {
			t.Fatal(err)
		}
		requireQualificationFault(t, OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "inspect", io.Discard), "initialized journal missing")
	})
	t.Run("receipt file malformed", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		fixture.initialize(t)
		fixture.stop(t)
		if err := os.WriteFile(fixture.policy.ReceiptDatabasePath, []byte("not sqlite"), 0o600); err != nil {
			t.Fatal(err)
		}
		requireQualificationFault(t, OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "inspect", io.Discard), "")
	})
	t.Run("output write failure", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		fixture.initialize(t)
		requireQualificationFault(t, OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "inspect", qualificationFaultWriter{}), "controlled_output_failure")
	})
}

type qualificationFaultWriter struct{}

func (qualificationFaultWriter) Write([]byte) (int, error) {
	return 0, errors.New("controlled_output_failure")
}

func TestQualificationBudgetFaultsAccountingReceipts(t *testing.T) {
	for _, scenario := range []struct{ name, sql, fragment string }{
		{"negative reserve", `UPDATE qualification_budget_attempts SET reserved = -1`, "invalid attempt receipt"},
		{"negative cost", `UPDATE qualification_budget_attempts SET cost = -1`, "invalid attempt receipt"},
		{"cost exceeds reserve", `UPDATE qualification_budget_attempts SET cost = 11`, "invalid attempt receipt"},
		{"missing logical identity", `UPDATE qualification_budget_attempts SET logical_id = ''`, "invalid attempt receipt"},
		{"missing originating request identity", `UPDATE qualification_budget_attempts SET request_id = ''`, "invalid attempt receipt"},
		{"missing connection identity", `UPDATE qualification_budget_attempts SET connection_id = ''`, "invalid attempt receipt"},
		{"unknown state", `UPDATE qualification_budget_attempts SET state = 'unknown'`, "invalid attempt state"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			fixture.initialize(t)
			seedQualificationFaultPending(t, fixture, false)
			qualificationFaultExec(t, fixture.budget.database, scenario.sql)
			requireQualificationFault(t, OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "inspect", io.Discard), scenario.fragment)
		})
	}
}
func seedQualificationFaultPending(t *testing.T, fixture *qualificationFaultFixture, active bool) {
	t.Helper()
	attempt := qualificationBudgetAttempt{ID: 1, LogicalID: "logical", RequestID: "origin-fixture", Provider: "cerebras", Model: "qwen3.8-27b", ConnectionID: "connection", Reserved: 10, State: "pending", Active: active, CreatedAt: time.Now().UTC()}
	if err := fixture.budget.database.Create(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	qualificationFaultExec(t, fixture.budget.database, `UPDATE qualification_budget_states SET remaining = remaining - 10, suspended = true`)
}

func TestQualificationBudgetFaultsOperatorTransactions(t *testing.T) {
	for _, scenario := range []struct {
		name, action, trigger, fragment string
		pending, active                 bool
	}{
		{name: "close unresolved", action: "close", pending: true, fragment: "unresolved reservations"},
		{name: "close journal persistence", action: "close", trigger: `CREATE TRIGGER reject_close BEFORE UPDATE OF closed ON qualification_budget_states BEGIN SELECT RAISE(ABORT,'controlled_close_failure'); END`, fragment: "controlled_close_failure"},
		{name: "reconcile missing attempt", action: "reconcile", fragment: "attempt unavailable"},
		{name: "reconcile active dispatch", action: "reconcile", pending: true, active: true, fragment: "active dispatch cannot be reconciled"},
		{name: "reconcile lock write", action: "reconcile", pending: true, trigger: `CREATE TRIGGER reject_reconcile BEFORE UPDATE ON qualification_budget_states BEGIN SELECT RAISE(ABORT,'controlled_reconcile_failure'); END`, fragment: "controlled_reconcile_failure"},
		{name: "reconcile attempt write", action: "reconcile", pending: true, trigger: `CREATE TRIGGER reject_reconcile BEFORE UPDATE ON qualification_budget_attempts BEGIN SELECT RAISE(ABORT,'controlled_attempt_failure'); END`, fragment: "controlled_attempt_failure"},
		{name: "reconcile suspension write", action: "reconcile", pending: true, trigger: `CREATE TRIGGER reject_reconcile BEFORE UPDATE OF suspended ON qualification_budget_states BEGIN SELECT RAISE(ABORT,'controlled_unsuspend_failure'); END`, fragment: "controlled_unsuspend_failure"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			fixture.initialize(t)
			if scenario.pending {
				seedQualificationFaultPending(t, fixture, scenario.active)
			}
			if scenario.trigger != "" {
				qualificationFaultExec(t, fixture.budget.database, scenario.trigger)
			}
			var err error
			if scenario.action == "reconcile" {
				err = ReconcileQualificationBudgetMaximum(fixture.managementPath, fixture.policy.BudgetID, 1, io.Discard)
			} else {
				err = OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, scenario.action, io.Discard)
			}
			requireQualificationFault(t, err, scenario.fragment)
			var state qualificationBudgetState
			if err := fixture.budget.database.First(&state).Error; err != nil {
				t.Fatal(err)
			}
			expectedRemaining := fixture.policy.RemainingMicroUSD
			if scenario.pending {
				expectedRemaining -= 10
			}
			if state.Closed || state.Remaining != expectedRemaining {
				t.Fatalf("failed operator transaction changed custody: %+v", state)
			}
		})
	}
	t.Run("registry closure failure keeps removal gated", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		fixture.initialize(t)
		qualificationFaultExec(t, fixture.management.database, `CREATE TRIGGER reject_registry_close BEFORE UPDATE ON qualification_budget_registries BEGIN SELECT RAISE(ABORT,'controlled_registry_close'); END`)
		requireQualificationFault(t, OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "close", io.Discard), "controlled_registry_close")
		fixture.stop(t)
		_, err := newQualificationBudget(nil, fixture.management, fixture.catalog)
		requireQualificationFault(t, err, "initialized policy cannot be removed")
	})
}

func TestQualificationBudgetFaultsClosedRegistry(t *testing.T) {
	for _, scenario := range []struct {
		name, sql, fragment string
		missing             bool
	}{
		{name: "closed receipt missing", fragment: "closed receipts missing", missing: true},
		{name: "closed receipt reopened", sql: `UPDATE qualification_budget_states SET closed = false`, fragment: "closed receipt integrity"},
		{name: "closed receipt allowance", sql: `UPDATE qualification_budget_states SET remaining = -1`, fragment: "invalid remaining allowance"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			fixture.initialize(t)
			if err := OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "close", io.Discard); err != nil {
				t.Fatal(err)
			}
			fixture.stop(t)
			if scenario.missing {
				if err := os.Remove(fixture.policy.ReceiptDatabasePath); err != nil {
					t.Fatal(err)
				}
			} else {
				qualificationFaultExec(t, qualificationFaultJournal(t, fixture.policy.ReceiptDatabasePath), scenario.sql)
			}
			_, err := newQualificationBudget(nil, fixture.management, fixture.catalog)
			requireQualificationFault(t, err, scenario.fragment)
		})
	}
	t.Run("invalid tombstone policy", func(t *testing.T) {
		requireQualificationFault(t, validateClosedQualificationRegistry(qualificationBudgetRegistry{Policy: "invalid-json"}), "qualification_budget_denied")
	})
	t.Run("already closed lock reports cleanup error", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		fixture.initialize(t)
		if err := fixture.budget.lockFile.Close(); err != nil {
			t.Fatal(err)
		}
		requireQualificationFault(t, fixture.budget.close(), "file already closed")
		fixture.budget = nil
	})
}

type qualificationFaultHTTPDoer struct{}

func (qualificationFaultHTTPDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected transport use")
}

type qualificationFaultRoundTripper struct{}

func (qualificationFaultRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected transport use")
}
func TestQualificationBudgetFaultsTransportConstruction(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		next     HTTPDoer
		fragment string
	}{
		{"missing admission", http.DefaultClient, "admission transport required"},
		{"nonstandard client", &admissionHTTPDoer{next: qualificationFaultHTTPDoer{}}, "standard HTTP client required"},
		{"nonstandard round tripper", &admissionHTTPDoer{next: &http.Client{Transport: qualificationFaultRoundTripper{}}}, "standard HTTP transport required"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, err := newQualificationSafeHTTPDoer(scenario.next)
			requireQualificationFault(t, err, scenario.fragment)
		})
	}
	// Valid construction must not mutate the caller's HTTP client or transport.
	original := &http.Transport{ForceAttemptHTTP2: true}
	client := &http.Client{Transport: original}
	if _, err := newQualificationSafeHTTPDoer(&admissionHTTPDoer{next: client}); err != nil {
		t.Fatal(err)
	}
	if original.DisableKeepAlives || !original.ForceAttemptHTTP2 || client.CheckRedirect != nil {
		t.Fatal("safe transport construction mutated supplied dependencies")
	}
}

var _ io.Writer = qualificationFaultWriter{}

// Overrides are process-local and only used by nonparallel tests. Every pool
// opened during an override is closed before the effect is restored.
var qualificationFaultOpenMutex sync.Mutex

func installQualificationFaultOpenHook(t *testing.T, target string, configure func(*gorm.DB)) {
	t.Helper()
	qualificationFaultOpenMutex.Lock()
	original := openQualificationSQLite
	var pools []*sql.DB
	openQualificationSQLite = func(path string) (*gorm.DB, error) {
		database, err := original(path)
		if err != nil {
			return nil, err
		}
		pool, err := database.DB()
		if err != nil {
			t.Fatal(err)
		}
		pools = append(pools, pool)
		if path == target {
			configure(database)
		}
		return database, nil
	}
	t.Cleanup(func() {
		for _, pool := range pools {
			if err := pool.Close(); err != nil {
				t.Error(err)
			}
		}
		openQualificationSQLite = original
		qualificationFaultOpenMutex.Unlock()
	})
}
func qualificationFaultQuery(t *testing.T, database *gorm.DB, predicate func(*gorm.DB) bool, message string) {
	t.Helper()
	if err := database.Callback().Query().Before("gorm:query").Register("qualification_controlled_query", func(tx *gorm.DB) {
		if predicate(tx) {
			tx.AddError(errors.New(message))
		}
	}); err != nil {
		t.Fatal(err)
	}
}
func qualificationFaultDDL(t *testing.T, database *gorm.DB, table, message string) {
	t.Helper()
	if err := database.Callback().Raw().Before("gorm:raw").Register("qualification_controlled_ddl", func(tx *gorm.DB) {
		statement := tx.Statement.SQL.String()
		if strings.HasPrefix(statement, "CREATE TABLE") && strings.Contains(statement, table) {
			tx.AddError(errors.New(message))
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestQualificationBudgetFaultsConstructorIOBoundaries(t *testing.T) {
	t.Run("disabled without managed store", func(t *testing.T) {
		budget, err := newQualificationBudget(nil, nil, ModelCatalog{})
		if err != nil || budget != nil {
			t.Fatalf("disabled qualification returned budget=%v error=%v", budget, err)
		}
	})
	t.Run("closed registry enumeration fails", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		if err := fixture.management.database.AutoMigrate(&qualificationBudgetRegistry{}); err != nil {
			t.Fatal(err)
		}
		registerManagedGORMError(t, fixture.management.database, "closed_registry_enumeration", "query", "qualification_budget_registries", errors.New("controlled_registry_enumeration"))
		_, err := newQualificationBudget(nil, fixture.management, fixture.catalog)
		requireQualificationFault(t, err, "controlled_registry_enumeration")
	})
	for _, phase := range []string{"registry migrate", "registry lock write", "registry active count", "receipt migrate", "initial state write"} {
		t.Run(phase, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			switch phase {
			case "registry migrate":
				qualificationFaultDDL(t, fixture.management.database, "qualification_budget_registries", "controlled_registry_migration")
			case "registry lock write":
				registerManagedGORMError(t, fixture.management.database, "qualification_registry_lock", "update", "qualification_budget_registries", errors.New("controlled_registry_lock"))
			case "registry active count":
				qualificationFaultQuery(t, fixture.management.database, func(tx *gorm.DB) bool {
					_, count := tx.Statement.Dest.(*int64)
					return count && tx.Statement.Table == "qualification_budget_registries"
				}, "controlled_active_count")
			case "receipt migrate":
				installQualificationFaultOpenHook(t, fixture.policy.ReceiptDatabasePath, func(database *gorm.DB) {
					qualificationFaultDDL(t, database, "qualification_budget_states", "controlled_receipt_migration")
				})
			case "initial state write":
				installQualificationFaultOpenHook(t, fixture.policy.ReceiptDatabasePath, func(database *gorm.DB) {
					registerManagedGORMError(t, database, "qualification_initial_state", "create", "qualification_budget_states", errors.New("controlled_initial_state"))
				})
			}
			_, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
			requireQualificationFault(t, err, "controlled_")
		})
	}
	for _, phase := range []string{"journal open", "integrity read", "pending count", "active reset", "suspension write"} {
		t.Run("restart "+phase, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			fixture.initialize(t)
			seedQualificationFaultPending(t, fixture, true)
			fixture.stop(t)
			if phase == "journal open" {
				if err := os.Remove(fixture.policy.ReceiptDatabasePath); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(fixture.policy.ReceiptDatabasePath, 0o700); err != nil {
					t.Fatal(err)
				}
			} else {
				installQualificationFaultOpenHook(t, fixture.policy.ReceiptDatabasePath, func(database *gorm.DB) {
					switch phase {
					case "integrity read":
						if err := database.Callback().Row().Before("gorm:row").Register("qualification_integrity_read", func(tx *gorm.DB) {
							if strings.Contains(tx.Statement.SQL.String(), "integrity_check") {
								tx.AddError(errors.New("controlled_integrity_read"))
							}
						}); err != nil {
							t.Fatal(err)
						}
					case "pending count":
						qualificationFaultQuery(t, database, func(tx *gorm.DB) bool {
							_, count := tx.Statement.Dest.(*int64)
							return count && tx.Statement.Table == "qualification_budget_attempts"
						}, "controlled_pending_count")
					case "active reset":
						registerManagedGORMError(t, database, "qualification_active_reset", "update", "qualification_budget_attempts", errors.New("controlled_active_reset"))
					case "suspension write":
						registerManagedGORMError(t, database, "qualification_restart_suspension", "update", "qualification_budget_states", errors.New("controlled_restart_suspension"))
					}
				})
			}
			_, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
			fragment := "controlled_"
			if phase == "journal open" {
				fragment = "journal open"
			}
			if phase == "integrity read" {
				fragment = "journal integrity"
			}
			requireQualificationFault(t, err, fragment)
		})
	}
}

func TestQualificationBudgetFaultsOperatorQueryIO(t *testing.T) {
	for _, phase := range []string{"reconciliation count", "closure count", "terminal receipt enumeration"} {
		t.Run(phase, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			fixture.initialize(t)
			if phase == "reconciliation count" {
				seedQualificationFaultPending(t, fixture, false)
			}
			installQualificationFaultOpenHook(t, fixture.policy.ReceiptDatabasePath, func(database *gorm.DB) {
				enumerations := 0
				qualificationFaultQuery(t, database, func(tx *gorm.DB) bool {
					if tx.Statement.Table != "qualification_budget_attempts" {
						return false
					}
					if _, count := tx.Statement.Dest.(*int64); count {
						return phase != "terminal receipt enumeration"
					}
					if _, list := tx.Statement.Dest.(*[]qualificationBudgetAttempt); list {
						enumerations++
						return phase == "terminal receipt enumeration" && enumerations == 2
					}
					return false
				}, "controlled_operator_query")
			})
			var err error
			switch phase {
			case "reconciliation count":
				err = ReconcileQualificationBudgetMaximum(fixture.managementPath, fixture.policy.BudgetID, 1, io.Discard)
			case "closure count":
				err = OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "close", io.Discard)
			default:
				err = OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "inspect", io.Discard)
			}
			requireQualificationFault(t, err, "controlled_operator_query")
			var state qualificationBudgetState
			if err := fixture.budget.database.First(&state).Error; err != nil {
				t.Fatal(err)
			}
			if state.Closed {
				t.Fatal("failed closure published closed state")
			}
			if phase == "reconciliation count" {
				var attempt qualificationBudgetAttempt
				if err := fixture.budget.database.First(&attempt).Error; err != nil {
					t.Fatal(err)
				}
				if attempt.State != "pending" || attempt.Cost != 0 || !state.Suspended || state.Remaining != fixture.policy.RemainingMicroUSD-10 {
					t.Fatal("failed reconciliation changed retained maximum custody")
				}
			}
		})
	}
	t.Run("other unresolved attempt retains suspension", func(t *testing.T) {
		fixture := newQualificationFaultFixture(t)
		fixture.initialize(t)
		seedQualificationFaultPending(t, fixture, false)
		second := qualificationBudgetAttempt{ID: 2, LogicalID: "second-logical", RequestID: "second-origin-fixture", Provider: "cerebras", Model: "qwen3.8-27b", ConnectionID: "connection", Reserved: 10, State: "pending", CreatedAt: time.Now().UTC()}
		if err := fixture.budget.database.Create(&second).Error; err != nil {
			t.Fatal(err)
		}
		qualificationFaultExec(t, fixture.budget.database, `UPDATE qualification_budget_states SET remaining = remaining - 10`)
		if err := ReconcileQualificationBudgetMaximum(fixture.managementPath, fixture.policy.BudgetID, 1, io.Discard); err != nil {
			t.Fatal(err)
		}
		var state qualificationBudgetState
		if err := fixture.budget.database.First(&state).Error; err != nil {
			t.Fatal(err)
		}
		if !state.Suspended || state.Remaining != fixture.policy.RemainingMicroUSD-20 {
			t.Fatal("reconciling one unknown released another attempt's retained maximum")
		}
	})
}

func TestQualificationBudgetFaultsCorruptClosedJournalOpen(t *testing.T) {
	fixture := newQualificationFaultFixture(t)
	fixture.initialize(t)
	if err := OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "close", io.Discard); err != nil {
		t.Fatal(err)
	}
	fixture.stop(t)
	if err := os.WriteFile(fixture.policy.ReceiptDatabasePath, []byte("corrupt SQLite header"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := newQualificationBudget(nil, fixture.management, fixture.catalog)
	requireQualificationFault(t, err, "")
}

func TestQualificationBudgetFaultsRestartRejectsMissingRequestIdentity(t *testing.T) {
	fixture := newQualificationFaultFixture(t)
	fixture.initialize(t)
	seedQualificationFaultPending(t, fixture, false)
	fixture.stop(t)
	database := qualificationFaultJournal(t, fixture.policy.ReceiptDatabasePath)
	qualificationFaultExec(t, database, `UPDATE qualification_budget_attempts SET request_id = ''`)
	_, err := newQualificationBudget(&fixture.policy, fixture.management, fixture.catalog)
	requireQualificationFault(t, err, "invalid attempt receipt")
}

func TestQualificationBudgetFaultsReserveSQLBoundaries(t *testing.T) {
	for _, phase := range []string{"write lock", "state read", "unresolved count", "logical count", "price before effective date"} {
		t.Run(phase, func(t *testing.T) {
			harness := newQualificationBoundaryHarness(t)
			budget := harness.fixture.budget
			faultEnabled := true
			fragment := "controlled_reserve_query"
			switch phase {
			case "write lock":
				fragment = "controlled_reserve_lock"
				qualificationFaultExec(t, budget.database, `CREATE TRIGGER reserve_lock_failure BEFORE UPDATE OF remaining ON qualification_budget_states BEGIN SELECT RAISE(ABORT, 'controlled_reserve_lock'); END`)
			case "state read":
				qualificationFaultQuery(t, budget.database, func(tx *gorm.DB) bool {
					return faultEnabled && tx.Statement.Table == "qualification_budget_states"
				}, fragment)
			case "unresolved count", "logical count":
				budget.suspended = phase == "unresolved count"
				qualificationFaultQuery(t, budget.database, func(tx *gorm.DB) bool {
					_, count := tx.Statement.Dest.(*int64)
					return faultEnabled && count && tx.Statement.Table == "qualification_budget_attempts"
				}, fragment)
			case "price before effective date":
				var earliest time.Time
				for _, rate := range budget.frozen[0].Rates {
					start, err := time.Parse(time.RFC3339, rate.Conditions.EffectiveFrom)
					if err != nil {
						t.Fatal(err)
					}
					if earliest.IsZero() || start.Before(earliest) {
						earliest = start
					}
				}
				budget.now = func() time.Time { return earliest.Add(-time.Second) }
				fragment = "exact list rates unavailable"
			}
			response, err := harness.doer.Do(harness.request)
			if response != nil {
				_ = response.Body.Close()
			}
			requireQualificationFault(t, err, fragment)
			faultEnabled = false
			if harness.calls.Load() != 0 {
				t.Fatal("failed reservation reached upstream")
			}
			var state qualificationBudgetState
			if err := budget.database.First(&state).Error; err != nil {
				t.Fatal(err)
			}
			var attempts int64
			if err := budget.database.Model(&qualificationBudgetAttempt{}).Count(&attempts).Error; err != nil {
				t.Fatal(err)
			}
			if state.Remaining != harness.fixture.policy.RemainingMicroUSD || attempts != 0 {
				t.Fatalf("failed reservation changed custody: remaining=%d attempts=%d", state.Remaining, attempts)
			}
		})
	}
}

func TestQualificationBudgetFaultsSettlementSQLBoundaries(t *testing.T) {
	for _, phase := range []string{"attempt update", "refund update", "duplicate settlement", "conservative settlement"} {
		t.Run(phase, func(t *testing.T) {
			fixture := newQualificationFaultFixture(t)
			fixture.initialize(t)
			budget := fixture.budget
			offering := budget.frozen[0]
			attempt, err := budget.reserve(&qualificationExecution{id: "settlement-fault", requestID: "origin-fixture"}, offering, "fixture-connection")
			if err != nil {
				t.Fatal(err)
			}
			evidence := journalUsageEvidenceInput{Quantities: []journalQuantity{{Dimension: "input_tokens", Value: "2"}, {Dimension: "output_tokens", Value: "3"}}}
			fragment := "controlled_settlement"
			switch phase {
			case "attempt update":
				qualificationFaultExec(t, budget.database, `CREATE TRIGGER settlement_attempt_failure BEFORE UPDATE ON qualification_budget_attempts BEGIN SELECT RAISE(ABORT, 'controlled_settlement'); END`)
			case "refund update":
				qualificationFaultExec(t, budget.database, `CREATE TRIGGER settlement_refund_failure BEFORE UPDATE OF remaining ON qualification_budget_states BEGIN SELECT RAISE(ABORT, 'controlled_settlement'); END`)
			case "duplicate settlement":
				if err := budget.settle(attempt, offering, evidence); err != nil {
					t.Fatal(err)
				}
				fragment = "attempt already settled"
			case "conservative settlement":
				qualificationFaultExec(t, budget.database, `UPDATE qualification_budget_attempts SET state = 'conservative', cost = reserved, active = false WHERE id = ?`, attempt.ID)
				fragment = "attempt already settled"
			}
			var beforeState qualificationBudgetState
			var beforeAttempt qualificationBudgetAttempt
			if err := budget.database.First(&beforeState).Error; err != nil {
				t.Fatal(err)
			}
			if err := budget.database.First(&beforeAttempt, attempt.ID).Error; err != nil {
				t.Fatal(err)
			}
			requireQualificationFault(t, budget.settle(attempt, offering, evidence), fragment)
			var afterState qualificationBudgetState
			var afterAttempt qualificationBudgetAttempt
			if err := budget.database.First(&afterState).Error; err != nil {
				t.Fatal(err)
			}
			if err := budget.database.First(&afterAttempt, attempt.ID).Error; err != nil {
				t.Fatal(err)
			}
			if afterState.Remaining != beforeState.Remaining || afterAttempt.State != beforeAttempt.State || afterAttempt.Cost != beforeAttempt.Cost || afterAttempt.Active != beforeAttempt.Active || (afterAttempt.InputTokens == nil) != (beforeAttempt.InputTokens == nil) || (afterAttempt.OutputTokens == nil) != (beforeAttempt.OutputTokens == nil) {
				t.Fatalf("rejected settlement changed custody or receipt: before=%+v after=%+v remaining=%d/%d", beforeAttempt, afterAttempt, beforeState.Remaining, afterState.Remaining)
			}
			if err := OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "inspect", io.Discard); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQualificationBudgetInspectSnapshot(t *testing.T) {
	for _, change := range []string{"reservation", "settlement"} {
		for _, phase := range []string{"state-read", "accounting-read"} {
			t.Run(change+"/"+phase, func(t *testing.T) {
				fixture := newQualificationFaultFixture(t)
				fixture.initialize(t)
				budget := fixture.budget
				qualificationFaultExec(t, budget.database, "PRAGMA journal_mode=WAL")
				offering := budget.frozen[0]
				logical := &qualificationExecution{id: "snapshot-request", requestID: "snapshot-origin"}
				var attempt qualificationBudgetAttempt
				expectedRemaining := fixture.policy.RemainingMicroUSD
				expectedAttempts := 0
				if change == "settlement" {
					var err error
					attempt, err = budget.reserve(logical, offering, "snapshot-connection")
					if err != nil {
						t.Fatal(err)
					}
					expectedRemaining -= attempt.Reserved
					expectedAttempts = 1
				}
				committed := false
				installQualificationFaultOpenHook(t, fixture.policy.ReceiptDatabasePath, func(database *gorm.DB) {
					if err := database.Callback().Query().After("gorm:query").Register("commit_between_inspection_reads", func(tx *gorm.DB) {
						target := "qualification_budget_states"
						if phase == "accounting-read" {
							target = "qualification_budget_attempts"
						}
						if committed || tx.Statement.Table != target {
							return
						}
						committed = true
						var err error
						if change == "reservation" {
							attempt, err = budget.reserve(logical, offering, "snapshot-connection")
						} else {
							err = budget.settle(attempt, offering, journalUsageEvidenceInput{Quantities: []journalQuantity{{Dimension: "input_tokens", Value: "2"}, {Dimension: "output_tokens", Value: "3"}}})
						}
						if err != nil {
							t.Fatalf("concurrent commit: %v", err)
						}
					}); err != nil {
						t.Fatal(err)
					}
				})
				var output bytes.Buffer
				if err := OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "inspect", &output); err != nil {
					t.Fatalf("healthy journal rejected: %v", err)
				}
				var receipt struct {
					Remaining int64                        `json:"remaining_micro_usd"`
					Attempts  []qualificationBudgetAttempt `json:"attempts"`
				}
				if err := json.Unmarshal(output.Bytes(), &receipt); err != nil {
					t.Fatal(err)
				}
				if !committed || receipt.Remaining != expectedRemaining || len(receipt.Attempts) != expectedAttempts {
					t.Fatalf("mixed inspection snapshot: %s", output.String())
				}
				if change == "settlement" && receipt.Attempts[0].State != "dispatched" {
					t.Fatalf("new settlement mixed with old balance: %s", output.String())
				}
				output.Reset()
				if err := OperateQualificationBudget(fixture.managementPath, fixture.policy.BudgetID, "inspect", &output); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(output.Bytes(), &receipt); err != nil {
					t.Fatal(err)
				}
				if len(receipt.Attempts) != 1 || change == "reservation" && receipt.Remaining != fixture.policy.RemainingMicroUSD-attempt.Reserved || change == "settlement" && (receipt.Attempts[0].State != "settled" || receipt.Remaining != fixture.policy.RemainingMicroUSD-receipt.Attempts[0].Cost) {
					t.Fatalf("next inspection missed committed work: %s", output.String())
				}
			})
		}
	}
}
