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
		"--gemini_api_key", "sk-gemini",
		"--gemini_base_url", "https://gemini.example",
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
	if capturedConfiguration.GeminiKey != "sk-gemini" {
		t.Fatalf("geminiKey=%q", capturedConfiguration.GeminiKey)
	}
	if capturedConfiguration.GeminiBaseURL != "https://gemini.example" {
		t.Fatalf("geminiBaseURL=%q", capturedConfiguration.GeminiBaseURL)
	}
}

func TestRemovedGlobalMaxOutputTokensConfigSurface(t *testing.T) {
	if rootCmd.Flags().Lookup("max_output_tokens") != nil {
		t.Fatal("max_output_tokens flag must not be registered")
	}
	for _, binding := range environmentBindings() {
		if binding.key == "max_output_tokens" {
			t.Fatalf("max_output_tokens env binding must not be registered: %+v", binding)
		}
		for _, environmentVariable := range binding.environmentVariables {
			if environmentVariable == "LLM_PROXY_MAX_OUTPUT_TOKENS" {
				t.Fatalf("LLM_PROXY_MAX_OUTPUT_TOKENS env binding must not be registered: %+v", binding)
			}
		}
	}
}
