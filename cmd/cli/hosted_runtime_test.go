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

func TestHostedRuntimeCLIRejectsInvalidScopesBeforeServiceStartup(t *testing.T) {
	const offering = `    - provider: openai
      model: gpt-4.1
      operation: text
      maximum_attempts: 1
      conditions: {}
`
	const prefix = "\nhosted:\n  offerings:\n"
	for _, scenario := range []struct{ name, hosted, reason string }{
		{"empty-list", "\nhosted:\n  offerings: []\n", "explicit offering scopes are required"},
		{"missing-list", "\nhosted: {}\n", "explicit offering scopes are required"},
		{"duplicate", prefix + offering + offering, "duplicate offering scope"},
		{"duplicate-with-other-conditions", prefix + offering + strings.Replace(offering, "conditions: {}", "conditions: {service_tier: priority}", 1), "duplicate offering scope"},
		{"unknown-service", prefix + strings.Replace(offering, "model: gpt-4.1", "model: ''", 1), "configure hosted service scope"},
		{"cache-condition", prefix + strings.Replace(offering, "conditions: {}", "conditions: {cache_class: read}", 1), "categorical service conditions"},
		{"token-range", prefix + strings.Replace(offering, "conditions: {}", "conditions: {input_tokens: {maximum_exclusive: '1000'}}", 1), "categorical service conditions"},
		{"invalid-region", prefix + strings.Replace(offering, "conditions: {}", "conditions: {region: 'US West'}", 1), "invalid price region"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			directory := t.TempDir()
			databasePath := filepath.Join(directory, "hosted.sqlite")
			configuration := strings.ReplaceAll(completeManagementYAML(), "/tmp/llm-proxy-test.sqlite", databasePath) + scenario.hosted
			configPath := writeTestConfig(t, directory, configuration)
			started := false
			withServeProxy(t, func(proxy.Configuration, *zap.SugaredLogger) error {
				started = true
				return nil
			})
			err := executeRootCommand(t, "--config", configPath)
			if started || err == nil || !strings.Contains(err.Error(), scenario.reason) {
				t.Fatalf("invalid hosted scope reached startup: started=%t error=%v want=%s", started, err, scenario.reason)
			}
			if _, err := os.Stat(databasePath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid hosted scope touched its database: %v", err)
			}
		})
	}
}

func TestHostedRuntimeConfigurationSelectsExactOfferingScopes(t *testing.T) {
	directory := t.TempDir()
	configurationText := strings.ReplaceAll(completeManagementYAML(), "/tmp/llm-proxy-test.sqlite", filepath.Join(directory, "hosted.sqlite")) + `
hosted:
  offerings:
    - provider: openai
      model: gpt-4.1
      operation: text
      maximum_attempts: 1
      conditions: {}
`
	configuration, err := loadRuntimeConfiguration(writeTestConfig(t, directory, configurationText))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar()); err != nil {
		t.Fatal(err)
	}
	for _, replacement := range []struct{ from, to string }{
		{"maximum_attempts: 1", "maximum_attempts: 0"},
		{"maximum_attempts: 1", "maximum_attempts: 1.5"},
		{"maximum_attempts: 1", "maximum_attempts: -1"},
		{"maximum_attempts: 1", "maximum_attempts: 4294967296"},
		{"maximum_attempts: 1", "maximum_attempts: '1'"},
		{"provider: openai", "provider: unknown"},
		{"provider: openai", "provider: ' openai '"},
		{"operation: text", "operation: unknown"},
		{"model: gpt-4.1", "model: unknown"},
		{"model: gpt-4.1", "model: ' gpt-4.1 '"},
		{"conditions: {}", "conditions: {service_tier: unknown}"},
		{"conditions: {}", "conditions: {effective_from: '2026-01-01T00:00:00Z'}"},
	} {
		_, err := loadRuntimeConfiguration(writeTestConfig(t, directory, strings.Replace(configurationText, replacement.from, replacement.to, 1)))
		if err == nil {
			t.Fatalf("invalid hosted scope accepted: %s", replacement.to)
		}
	}
}

func TestHostedRuntimeCLIOmittedScopesKeepHostedDisabled(t *testing.T) {
	directory := t.TempDir()
	configuration := strings.ReplaceAll(completeManagementYAML(), "/tmp/llm-proxy-test.sqlite", filepath.Join(directory, "hosted.sqlite"))
	configPath := writeTestConfig(t, directory, configuration)
	started := false
	withServeProxy(t, func(configuration proxy.Configuration, _ *zap.SugaredLogger) error {
		started = true
		if configuration.Hosted != nil {
			t.Fatal("omitted hosted configuration enabled hosted execution")
		}
		return nil
	})
	if err := executeRootCommand(t, "--config", configPath); err != nil || !started {
		t.Fatalf("omitted hosted configuration prevented startup: started=%t error=%v", started, err)
	}
}
