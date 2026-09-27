package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestRootCommandPaymentConfigurationPresence(t *testing.T) {
	for _, scenario := range []struct {
		name, payments, reason string
	}{
		{name: "omitted"},
		{name: "empty", payments: "\npayments: {}\n", reason: "client_token must match the payment environment"},
		{name: "incomplete", payments: "\npayments:\n  environment: sandbox\n", reason: "client_token must match the payment environment"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			directory := t.TempDir()
			databasePath := filepath.Join(directory, "payments.sqlite")
			configuration := strings.ReplaceAll(completeManagementYAML(), "/tmp/llm-proxy-test.sqlite", databasePath) + scenario.payments
			configPath := writeTestConfig(t, directory, configuration)
			started := false
			withServeProxy(t, func(configuration proxy.Configuration, _ *zap.SugaredLogger) error {
				started = true
				if scenario.reason == "" && configuration.Payments != nil {
					t.Fatal("omitted payment configuration enabled payments")
				}
				return nil
			})
			err := executeRootCommand(t, "--config", configPath)
			if scenario.reason == "" {
				if err != nil || !started {
					t.Fatalf("omitted payment configuration prevented startup: started=%t error=%v", started, err)
				}
			} else if started || err == nil || !strings.Contains(err.Error(), scenario.reason) {
				t.Fatalf("invalid payment configuration reached startup: started=%t error=%v want=%s", started, err, scenario.reason)
			}
			if _, err := os.Stat(databasePath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("configuration parsing touched its database: %v", err)
			}
		})
	}
}
