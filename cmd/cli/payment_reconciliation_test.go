package main

import (
	"bytes"
	"encoding/json"
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
}
