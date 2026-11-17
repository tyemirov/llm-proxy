package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestQualificationBudgetCLIConfiguration(t *testing.T) {
	for _, scenario := range []struct{ name, document, reason string }{
		{"disabled", "null", ""},
		{"empty", "{}", "invalid policy fields"},
		{"unknown", `{"unexpected":true}`, "unexpected"},
		{"valid", `{"budget_id":"local-fixture","tenant_id":"existing-tenant","expires_at":"2026-11-17T08:00:00Z","remaining_micro_usd":4355061,"receipt_database_path":"/tmp/qualification-fixture.sqlite","max_attempts_per_request":3,"offerings":[{"provider":"cerebras","model":"qwen3.8-27b","max_output_tokens":32768}],"verification_connections":[{"provider":"cerebras","name":"fixture connection"}]}`, ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			directory := t.TempDir()
			configuration := "server:\n  qualification_budget: " + scenario.document + "\n" + completeManagementYAML()
			path := writeTestConfig(t, directory, configuration)
			started := false
			withServeProxy(t, func(configuration proxy.Configuration, _ *zap.SugaredLogger) error {
				started = true
				if scenario.name == "valid" && (configuration.QualificationBudget == nil || configuration.QualificationBudget.RemainingMicroUSD != 4355061) {
					t.Fatalf("policy=%+v", configuration.QualificationBudget)
				}
				return nil
			})
			err := executeRootCommand(t, "--config", path)
			if scenario.reason == "" {
				if err != nil || !started {
					t.Fatalf("started=%t error=%v", started, err)
				}
			} else if err == nil || started || !strings.Contains(err.Error(), scenario.reason) {
				t.Fatalf("started=%t error=%v", started, err)
			}
		})
	}
}

func TestQualificationBudgetCLIOperatorArguments(t *testing.T) {
	for _, args := range [][]string{{"inspect"}, {"inspect", "--management-database", "/missing/operator-fixture.sqlite"}, {"invalid", "--management-database", "/missing/operator-fixture.sqlite", "--budget-id", "fixture"}, {"inspect", "--management-database", "/missing/operator-fixture.sqlite", "--budget-id", "fixture"}} {
		command := newQualificationBudgetCommand()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs(args)
		if err := command.Execute(); err == nil {
			t.Fatalf("invalid operator arguments accepted: %v", args)
		}
	}
}

func TestQualificationBudgetCLIOperatorConservativeLifecycle(t *testing.T) {
	root := t.TempDir()
	managementPath := filepath.Join(root, "management.sqlite")
	receiptPath := filepath.Join(root, "receipts.sqlite")
	mainDB, err := sql.Open("sqlite", managementPath)
	if err != nil {
		t.Fatal(err)
	}
	defer mainDB.Close()
	receiptDB, err := sql.Open("sqlite", receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	defer receiptDB.Close()
	policy, _ := json.Marshal(proxy.QualificationBudgetConfiguration{BudgetID: "fixture", TenantID: "existing", ReceiptDatabasePath: receiptPath, RemainingMicroUSD: 1000})
	digest := sha256.Sum256([]byte("[]"))
	for _, statement := range []string{`CREATE TABLE qualification_budget_states (id TEXT PRIMARY KEY,policy TEXT,frozen TEXT,remaining INTEGER,suspended BOOLEAN,closed BOOLEAN)`, `CREATE TABLE qualification_budget_attempts (id INTEGER PRIMARY KEY,logical_id TEXT,request_id TEXT,provider TEXT,model TEXT,connection_id TEXT,reserved INTEGER,cost INTEGER,state TEXT,active BOOLEAN)`} {
		if _, err := receiptDB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mainDB.Exec(`CREATE TABLE qualification_budget_registries (id TEXT PRIMARY KEY,policy TEXT,closed BOOLEAN,frozen_digest TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := mainDB.Exec(`INSERT INTO qualification_budget_registries VALUES (?,?,?,?)`, "fixture", string(policy), false, hex.EncodeToString(digest[:])); err != nil {
		t.Fatal(err)
	}
	if _, err := receiptDB.Exec(`INSERT INTO qualification_budget_states VALUES (?,?,?,?,?,?)`, "fixture", string(policy), "[]", 900, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := receiptDB.Exec(`INSERT INTO qualification_budget_attempts VALUES (1,'logical','origin-fixture','fixture-provider','fixture-model','connection',100,0,'dispatched',false)`); err != nil {
		t.Fatal(err)
	}
	run := func(action string, extra ...string) map[string]any {
		t.Helper()
		command := newQualificationBudgetCommand()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs(append([]string{action, "--management-database", managementPath, "--budget-id", "fixture"}, extra...))
		if err := command.Execute(); err != nil {
			t.Fatalf("%s: %v output=%s", action, err, output.String())
		}
		var value map[string]any
		if err := json.Unmarshal(output.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := run("inspect")
	after := run("reconcile-max", "--attempt-id", "1")
	again := run("reconcile-max", "--attempt-id", "1")
	if before["remaining_micro_usd"] != after["remaining_micro_usd"] || after["remaining_micro_usd"] != again["remaining_micro_usd"] || after["conservative_unknown_maximum_micro_usd"] != float64(100) || after["known_uncached_estimate_micro_usd"] != float64(0) || after["suspended"] != false {
		t.Fatalf("operator reconciliation=%v repeat=%v", after, again)
	}
	for range 2 {
		if run("close")["closed"] != true {
			t.Fatal("close was not idempotent")
		}
	}
}
