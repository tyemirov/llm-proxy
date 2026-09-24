package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/testfixtures"
	"go.uber.org/zap"
)

func hostedSignalsDurableBytes(t *testing.T, path string) map[string][sha256.Size]byte {
	t.Helper()
	result := map[string][sha256.Size]byte{}
	for _, suffix := range []string{"", "-wal"} {
		data, err := os.ReadFile(path + suffix)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		result[suffix] = sha256.Sum256(data)
	}
	return result
}

func TestHostedSignalsCLIFailuresPublishNoPartialReport(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "signals.sqlite")
	configPath := writeTestConfig(t, directory, strings.ReplaceAll(completeManagementYAML(), "/tmp/llm-proxy-test.sqlite", path))
	configuration, err := loadRuntimeConfiguration(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var output, diagnostics bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&diagnostics)
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil) })
	fail := func(t *testing.T, config string) {
		t.Helper()
		before := hostedSignalsDurableBytes(t, path)
		output.Reset()
		diagnostics.Reset()
		if err := executeRootCommand(t, "hosted-financial-signals", "--config", config); err == nil {
			t.Fatalf("invalid financial source produced success: %s", output.String())
		}
		for _, field := range []string{`"observed_at"`, `"posted_cents"`, `"payment_differences"`} {
			if strings.Contains(output.String(), field) {
				t.Fatalf("failed signals command published a partial report: %s", output.String())
			}
		}
		if !reflect.DeepEqual(before, hostedSignalsDurableBytes(t, path)) {
			t.Fatal("failed signals command changed durable database bytes")
		}
	}
	t.Run("missing-configuration", func(t *testing.T) { fail(t, filepath.Join(directory, "missing.yml")) })
	t.Run("missing-database", func(t *testing.T) {
		fail(t, configPath)
		if len(hostedSignalsDurableBytes(t, path)) != 0 {
			t.Fatal("signals command created the missing database")
		}
	})
	if _, err := testfixtures.BuildRouter(t, configuration, zap.NewNop().Sugar()); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	read := func() map[string]any {
		t.Helper()
		output.Reset()
		diagnostics.Reset()
		before := hostedSignalsDurableBytes(t, path)
		if err := executeRootCommand(t, "hosted-financial-signals", "--config", configPath); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, hostedSignalsDurableBytes(t, path)) {
			t.Fatal("signals command changed durable database bytes")
		}
		var report map[string]any
		if err := json.Unmarshal(output.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		delete(report, "observed_at")
		return report
	}
	original := read()
	for _, table := range []string{
		"managed_journal_attempt_records",
		"managed_journal_delivery_records",
		"managed_journal_request_records",
		"managed_funds_reservation_records",
		"managed_journal_case_records",
		"managed_payment_reconciliation_run_records",
		"managed_payment_reconciliation_item_records",
		"managed_provider_reconciliation_run_records",
		"managed_billing_account_records",
		"managed_funds_exposure_records",
	} {
		t.Run(table, func(t *testing.T) {
			if _, err := database.Exec("ALTER TABLE " + table + " RENAME TO unavailable_signals_table"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := database.Exec("ALTER TABLE unavailable_signals_table RENAME TO " + table); err != nil {
					t.Error(err)
				}
			})
			fail(t, configPath)
		})
		if !reflect.DeepEqual(original, read()) {
			t.Fatal("restored financial tables changed the report")
		}
	}
}
