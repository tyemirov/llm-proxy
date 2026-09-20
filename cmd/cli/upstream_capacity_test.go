package main

import (
	"gopkg.in/yaml.v3"
	"os"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestRootCommandRejectsObsoleteOrMissingUpstreamCapacity(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		server   string
		expected string
	}{
		{name: "missing capacity", server: "server:\n  port: 18080\n", expected: "invalid_upstream_capacity"},
		{name: "obsolete worker count", server: "server:\n  workers: 4\n", expected: "workers"},
		{name: "obsolete queue size", server: "server:\n  queue_size: 32\n", expected: "queue_size"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			configPath := writeRawTestConfig(t, t.TempDir(), testCase.server+completeLiteralRuntimeYAML())
			started := false
			withServeProxy(t, func(proxy.Configuration, *zap.SugaredLogger) error {
				started = true
				return nil
			})
			err := executeRootCommand(t, "--config", configPath)
			if started || err == nil || !strings.Contains(err.Error(), testCase.expected) {
				t.Fatalf("invalid upstream capacity reached startup: started=%t error=%v", started, err)
			}
		})
	}
}

func TestRootCommandRejectsInvalidUpstreamCapacityContract(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		change   func(map[string]any)
		expected string
	}{
		{"missing required origin", func(capacity map[string]any) {
			origins := capacity["origins"].([]any)
			capacity["origins"] = origins[1:]
		}, "reason=missing"},
		{"unknown origin", func(capacity map[string]any) {
			capacity["origins"] = append(capacity["origins"].([]any), map[string]any{"origin": "https://unknown.invalid", "active": 1, "queued": 1})
		}, "reason=unknown"},
		{"duplicate origin", func(capacity map[string]any) {
			origins := capacity["origins"].([]any)
			capacity["origins"] = append(origins, origins[0])
		}, "reason=duplicate"},
		{"unknown provider", func(capacity map[string]any) { capacity["origins"].([]any)[0].(map[string]any)["provider"] = "unknown" }, "unknown_connection_origin_provider"},
		{"unknown capacity field", func(capacity map[string]any) { capacity["global"].(map[string]any)["burst"] = 1 }, "burst"},
		{"contradictory global limit", func(capacity map[string]any) { capacity["global"].(map[string]any)["admitted"] = 1 }, "positive_ordered_limits_required"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			directory := t.TempDir()
			configPath := writeTestConfig(t, directory, "server:\n  port: 18080\n"+completeLiteralRuntimeYAML())
			content, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err = yaml.Unmarshal(content, &document); err != nil {
				t.Fatal(err)
			}
			scenario.change(document["server"].(map[string]any)["upstream_capacity"].(map[string]any))
			changed, err := yaml.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			configPath = writeRawTestConfig(t, directory, string(changed))
			started := false
			withServeProxy(t, func(proxy.Configuration, *zap.SugaredLogger) error { started = true; return nil })
			err = executeRootCommand(t, "--config", configPath)
			if started || err == nil || !strings.Contains(err.Error(), scenario.expected) {
				t.Fatalf("invalid capacity startup: started=%t error=%v", started, err)
			}
		})
	}
}
