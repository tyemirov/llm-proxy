package main

import (
	"os"
	"testing"

	"github.com/temirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestExecuteRunsConfiguredProxyWithInjectedServe(t *testing.T) {
	originalServeProxy := serveProxy
	originalArguments := os.Args
	t.Cleanup(func() {
		serveProxy = originalServeProxy
		os.Args = originalArguments
		rootCmd.SetArgs(nil)
	})

	var capturedConfiguration proxy.Configuration
	serveProxy = func(configuration proxy.Configuration, structuredLogger *zap.SugaredLogger) error {
		capturedConfiguration = configuration
		return nil
	}

	rootCmd.SetArgs([]string{
		"--service_secret", "sekret",
		"--default_provider", proxy.ProviderNameDeepSeek,
		"--deepseek_api_key", "sk-deepseek",
		"--log_level", proxy.LogLevelDebug,
		"--port", "18080",
	})
	Execute()

	if capturedConfiguration.ServiceSecret != "sekret" {
		t.Fatalf("serviceSecret=%q", capturedConfiguration.ServiceSecret)
	}
	if capturedConfiguration.DefaultProvider != proxy.ProviderNameDeepSeek {
		t.Fatalf("defaultProvider=%q", capturedConfiguration.DefaultProvider)
	}
	if capturedConfiguration.Port != 18080 {
		t.Fatalf("port=%d", capturedConfiguration.Port)
	}
}
