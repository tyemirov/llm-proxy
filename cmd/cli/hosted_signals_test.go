package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func TestHostedSignalsCLIReadsInitializedDatabase(t *testing.T) {
	directory := t.TempDir()
	configPath := writeTestConfig(t, directory, strings.ReplaceAll(completeManagementYAML(), "/tmp/llm-proxy-test.sqlite", filepath.Join(directory, "signals.sqlite")))
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
	if err := executeRootCommand(t, "hosted-financial-signals", "--config", configPath); err != nil {
		t.Fatalf("signals command: %v output=%s", err, output.String())
	}
	var report map[string]any
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report["currency"] != "USD" || report["observed_at"] == nil || report["posted_cents"] != "0" || report["reserved_cents"] != "0" {
		t.Fatalf("empty financial report: %v", report)
	}
	for _, name := range []string{"unresolved_attempts", "pending_usage_deliveries", "unsettled_completed_requests", "open_reconciliation_cases", "pending_payment_comparisons"} {
		value, ok := report[name].(map[string]any)
		if !ok || value["count"] != float64(0) || value["oldest_at"] != nil {
			t.Fatalf("empty signal %s: %v", name, report[name])
		}
	}
}
