package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func TestHostedPaymentsCLIReconcilesProviderEvidence(t *testing.T) {
	directory := t.TempDir()
	configPath := writeTestConfig(t, directory, strings.ReplaceAll(completeManagementYAML(), "/tmp/llm-proxy-test.sqlite", filepath.Join(directory, "provider-audit.sqlite")))
	configuration, err := loadRuntimeConfiguration(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testfixtures.BuildRouter(t, configuration, zap.NewNop().Sugar()); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", configuration.Management.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec("INSERT INTO managed_platform_connection_records (id,provider,name,version) VALUES ('provider-cli','openai','CLI fixture',1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO managed_platform_credential_records (connection_id,version,fields,qualified_at) VALUES ('provider-cli',1,'{}','2026-09-22 12:00:00')"); err != nil {
		t.Fatal(err)
	}
	source := "period,cost\n2026-09-22,0\n"
	evidence := fmt.Sprintf(`{"source_reference":"export-cli","source_sha256":"%x","provider":"openai","platform_connection_id":"provider-cli","credential_version":1,"provider_account_reference":"account-cli","period_start":"2026-09-22T00:00:00Z","period_end":"2026-09-23T00:00:00Z","reported_at":"2026-09-23T01:00:00Z","currency":"USD","usage_amount":{"numerator":"0","denominator":"1"},"discount":{"numerator":"0","denominator":"1"},"fees":{"numerator":"0","denominator":"1"},"attempt_count":0}`, sha256.Sum256([]byte(source)))
	write := func(name, content string) string {
		file, err := os.Create(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString(content); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		return file.Name()
	}
	evidencePath, sourcePath := write("evidence.json", evidence), write("invoice.csv", source)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil) })
	arguments := []string{"reconcile-provider-costs", "--config", configPath, "--run-id", "provider-cli-run", "--evidence", evidencePath, "--source", sourcePath}
	if err := executeRootCommand(t, arguments...); err != nil {
		t.Fatalf("command: %v output=%s", err, output.String())
	}
	first := output.String()
	var report proxy.ProviderCostReconciliationReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.RunID != "provider-cli-run" || report.AttemptCount != 0 || len(report.Differences) != 0 {
		t.Fatalf("report=%+v", report)
	}
	output.Reset()
	if err := executeRootCommand(t, arguments...); err != nil {
		t.Fatal(err)
	}
	if output.String() != first {
		t.Fatal("provider CLI replay changed the report")
	}
	changedSourcePath := write("changed-invoice.csv", "period,cost\n2026-09-22,1\n")
	for _, scenario := range []struct {
		name, config, evidence, source, runID, message string
	}{
		{"missing configuration", filepath.Join(directory, "absent.yml"), evidencePath, sourcePath, "provider-cli-run", "absent.yml"},
		{"missing normalized evidence", configPath, filepath.Join(directory, "absent.json"), sourcePath, "provider-cli-run", "open normalized provider evidence"},
		{"missing original source", configPath, evidencePath, filepath.Join(directory, "absent.csv"), "provider-cli-run", "open original provider source"},
		{"changed original source", configPath, evidencePath, changedSourcePath, "provider-cli-run", "provider source digest mismatch"},
		{"invalid run identifier", configPath, evidencePath, sourcePath, "invalid run", "invalid provider reconciliation run ID"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before := reconciliationDatabaseBytes(t, configuration.Management.DatabasePath)
			output.Reset()
			rootCmd.SetErr(&bytes.Buffer{})
			err := executeRootCommand(t, "reconcile-provider-costs", "--config", scenario.config, "--run-id", scenario.runID, "--evidence", scenario.evidence, "--source", scenario.source)
			if err == nil || !strings.Contains(err.Error(), scenario.message) {
				t.Fatalf("rejected provider reconciliation: %v, want %q", err, scenario.message)
			}
			if json.Valid(output.Bytes()) || strings.Contains(output.String(), `"run_id"`) {
				t.Fatalf("failed reconciliation published a report: %s", output.String())
			}
			assertReconciliationDatabaseBytes(t, configuration.Management.DatabasePath, before)
			output.Reset()
			if err := executeRootCommand(t, arguments...); err != nil {
				t.Fatal(err)
			}
			if output.String() != first {
				t.Fatal("failed reconciliation changed completed provider report replay")
			}
		})
	}
}
