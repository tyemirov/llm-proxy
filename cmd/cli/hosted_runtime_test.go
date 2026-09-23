package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

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
