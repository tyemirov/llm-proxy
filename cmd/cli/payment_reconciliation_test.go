package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func TestHostedPaymentsCLIRecordsAndReplaysReconciliationRun(t *testing.T) {
	directory := t.TempDir()
	configurationText := strings.ReplaceAll(completeManagementYAML(), "/tmp/llm-proxy-test.sqlite", filepath.Join(directory, "audit.sqlite")) + `
payments:
  environment: sandbox
  client_token: test_fixture
  processor_account_id: processor-fixture
  supplier_id: supplier-fixture
  api_key: fixture-key
  webhook_secret: fixture-secret
  offers:
    - code: five
      price_id: pri_01hv8x2axb33yr5y238zfwcn5p
      funding_cents: 500
`
	configPath := writeTestConfig(t, directory, configurationText)
	configuration, err := loadRuntimeConfiguration(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testfixtures.BuildRouter(t, configuration, zap.NewNop().Sugar()); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil) })
	arguments := []string{"reconcile-payments", "--config", configPath, "--run-id", "scheduled-fixture"}
	if err := executeRootCommand(t, arguments...); err != nil {
		t.Fatalf("command failed: %v; %s", err, output.String())
	}
	first := output.String()
	var report proxy.PaymentReconciliationReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.RunID != "scheduled-fixture" || report.State != "completed" || report.TotalOrders != 0 || report.Items == nil {
		t.Fatalf("report=%+v", report)
	}
	output.Reset()
	if err := executeRootCommand(t, arguments...); err != nil {
		t.Fatal(err)
	}
	if output.String() != first {
		t.Fatal("completed CLI replay changed its report")
	}
	for _, scenario := range []struct {
		name, config, runID, message string
	}{
		{"missing configuration", filepath.Join(directory, "absent.yml"), "scheduled-fixture", "absent.yml"},
		{"invalid run identifier", configPath, "invalid run", "invalid run ID"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before := reconciliationDatabaseBytes(t, configuration.Management.DatabasePath)
			output.Reset()
			rootCmd.SetErr(&bytes.Buffer{})
			err := executeRootCommand(t, "reconcile-payments", "--config", scenario.config, "--run-id", scenario.runID)
			if err == nil || !strings.Contains(err.Error(), scenario.message) {
				t.Fatalf("rejected reconciliation: %v, want %q", err, scenario.message)
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
				t.Fatal("failed reconciliation changed completed report replay")
			}
		})
	}
}

func reconciliationDatabaseBytes(t *testing.T, path string) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	for _, suffix := range []string{"", "-wal"} {
		contents, err := os.ReadFile(path + suffix)
		if suffix == "-wal" && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		files[suffix] = contents
	}
	return files
}

func assertReconciliationDatabaseBytes(t *testing.T, path string, before map[string][]byte) {
	t.Helper()
	after := reconciliationDatabaseBytes(t, path)
	if len(before) != len(after) {
		t.Fatal("failed reconciliation changed retained database files")
	}
	for suffix, original := range before {
		if !bytes.Equal(original, after[suffix]) {
			t.Fatalf("failed reconciliation changed database%s", suffix)
		}
	}
}
