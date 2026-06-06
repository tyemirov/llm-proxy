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

const (
	testConfigFileName = "config.yml"
	testDotEnvFileName = ".env"
)

func TestRootCommandRunsConfiguredProxyFromConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
server:
  port: 18080
  log_level: debug
  workers: 2
  queue_size: 9
  request_timeout_seconds: 7
  upstream_poll_timeout_seconds: 3
  max_prompt_bytes: 1024
  max_input_audio_bytes: 2048
tenants:
  - id: default
    secret: "${P411_SERVICE_SECRET}"
    defaults:
      provider: deepseek
      model: deepseek-v4-flash
      dictation_provider: openai
      dictation_model: gpt-4o-transcribe
      system_prompt: "Be terse."
providers:
  openai:
    api_key: "${P411_OPENAI_KEY}"
  deepseek:
    api_key: "${P411_DEEPSEEK_KEY}"
  gemini:
    api_key: "${P411_GEMINI_KEY}"
    base_url: "https://gemini.example"
`)
	writeTestDotEnv(t, tempDir, `
P411_SERVICE_SECRET=dotenv-secret
P411_OPENAI_KEY=sk-openai
P411_DEEPSEEK_KEY=sk-deepseek
P411_GEMINI_KEY=sk-gemini
`)
	t.Setenv("P411_SERVICE_SECRET", "process-secret")

	var capturedConfiguration proxy.Configuration
	withServeProxy(t, func(configuration proxy.Configuration, structuredLogger *zap.SugaredLogger) error {
		if _, buildError := proxy.BuildRouter(configuration, structuredLogger); buildError != nil {
			t.Fatalf("BuildRouter error: %v", buildError)
		}
		capturedConfiguration = configuration
		return nil
	})

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError != nil {
		t.Fatalf("ExecuteC error: %v", executeError)
	}
	if capturedConfiguration.Tenants[0].Secret != "process-secret" {
		t.Fatalf("tenantSecret=%q", capturedConfiguration.Tenants[0].Secret)
	}
	if capturedConfiguration.OpenAIKey != "sk-openai" {
		t.Fatalf("openAIKey=%q", capturedConfiguration.OpenAIKey)
	}
	if capturedConfiguration.Tenants[0].Defaults.Provider != proxy.ProviderNameDeepSeek {
		t.Fatalf("tenantDefaultProvider=%q", capturedConfiguration.Tenants[0].Defaults.Provider)
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

func TestRootCommandRunsProductionLoggerFromConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
server:
  log_level: info
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: openai
      model: gpt-4.1
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
providers:
  openai:
    api_key: "sk-openai"
`)
	withServeProxy(t, func(configuration proxy.Configuration, structuredLogger *zap.SugaredLogger) error {
		if configuration.LogLevel != proxy.LogLevelInfo {
			t.Fatalf("logLevel=%q", configuration.LogLevel)
		}
		return nil
	})

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError != nil {
		t.Fatalf("ExecuteC error: %v", executeError)
	}
}

func TestRootCommandRejectsRemovedServiceConfigurationFlags(t *testing.T) {
	if rootCmd.Flags().Lookup(flagConfig) == nil {
		t.Fatal("config flag must be registered")
	}
	removedFlags := []string{
		"service_secret",
		"openai_api_key",
		"default_provider",
		"default_model",
		"default_dictation_provider",
		"gemini_api_key",
		"port",
		"log_level",
		"workers",
		"queue_size",
		"request_timeout",
		"upstream_poll_timeout",
		"max_prompt_bytes",
		"dictation_model",
		"max_input_audio_bytes",
	}
	for _, removedFlag := range removedFlags {
		if rootCmd.Flags().Lookup(removedFlag) != nil {
			t.Fatalf("removed service configuration flag is still registered: %s", removedFlag)
		}
	}

	executeError := executeRootCommand(t, "--service_secret", "sekret")
	if executeError == nil || !strings.Contains(executeError.Error(), "unknown flag: --service_secret") {
		t.Fatalf("error=%v want unknown service_secret flag", executeError)
	}
}

func TestRootCommandRejectsMissingConfigPlaceholder(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
server:
  log_level: info
tenants:
  - id: default
    secret: "${P411_MISSING_SERVICE_SECRET}"
    defaults:
      provider: openai
      model: gpt-4.1
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
providers:
  openai:
    api_key: "sk-openai"
`)
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "config_placeholder_missing") {
		t.Fatalf("error=%v want missing placeholder", executeError)
	}
}

func TestRootCommandUsesDefaultConfigPathForBlankConfigFlag(t *testing.T) {
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", "")
	if executeError == nil || !strings.Contains(executeError.Error(), "path=config.yml") {
		t.Fatalf("error=%v want default config path", executeError)
	}
}

func TestRootCommandRejectsUnreadableDotEnv(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
server:
  log_level: info
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: openai
      model: gpt-4.1
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
providers:
  openai:
    api_key: "sk-openai"
`)
	if mkdirError := os.Mkdir(filepath.Join(tempDir, testDotEnvFileName), 0700); mkdirError != nil {
		t.Fatalf("create dotenv directory: %v", mkdirError)
	}
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "config_environment_read_failed") {
		t.Fatalf("error=%v want environment read failure", executeError)
	}
}

func TestRootCommandRejectsInvalidYAML(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
tenants:
  - id: [
`)
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "config_file_parse_failed") {
		t.Fatalf("error=%v want YAML parse failure", executeError)
	}
}

func TestRootCommandRejectsUnknownConfigKeys(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
server:
  unsupported: true
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: openai
      model: gpt-4.1
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
providers:
  openai:
    api_key: "sk-openai"
`)
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "config_file_parse_failed") {
		t.Fatalf("error=%v want config parse failure", executeError)
	}
}

func TestRootCommandRejectsMissingDefaultProviderCredential(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: gemini
      model: gemini-3.5-flash
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
providers:
  openai:
    api_key: "sk-openai"
`)
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "provider not configured: provider=gemini") {
		t.Fatalf("error=%v want missing gemini credential", executeError)
	}
}

func executeRootCommand(t *testing.T, arguments ...string) error {
	t.Helper()
	rootCmd.SetArgs(arguments)
	_, executeError := rootCmd.ExecuteC()
	rootCmd.SetArgs(nil)
	resetConfigFlag(t)
	runtimeConfiguration = proxy.Configuration{}
	return executeError
}

func resetConfigFlag(t *testing.T) {
	t.Helper()
	if flagError := rootCmd.Flags().Set(flagConfig, defaultConfigPath); flagError != nil {
		t.Fatalf("reset config flag: %v", flagError)
	}
}

func withServeProxy(t *testing.T, replacement func(proxy.Configuration, *zap.SugaredLogger) error) {
	t.Helper()
	originalServeProxy := serveProxy
	t.Cleanup(func() {
		serveProxy = originalServeProxy
		rootCmd.SetArgs(nil)
		resetConfigFlag(t)
		runtimeConfiguration = proxy.Configuration{}
	})
	serveProxy = replacement
}

func failingServeProxy(t *testing.T) func(proxy.Configuration, *zap.SugaredLogger) error {
	t.Helper()
	return func(configuration proxy.Configuration, structuredLogger *zap.SugaredLogger) error {
		t.Fatal("serveProxy must not be called")
		return errors.New("unexpected serve")
	}
}

func writeTestConfig(t *testing.T, tempDir string, configContent string) string {
	t.Helper()
	configPath := filepath.Join(tempDir, testConfigFileName)
	if writeError := os.WriteFile(configPath, []byte(strings.TrimSpace(configContent)+"\n"), 0600); writeError != nil {
		t.Fatalf("write config: %v", writeError)
	}
	return configPath
}

func writeTestDotEnv(t *testing.T, tempDir string, dotEnvContent string) {
	t.Helper()
	dotEnvPath := filepath.Join(tempDir, testDotEnvFileName)
	if writeError := os.WriteFile(dotEnvPath, []byte(strings.TrimSpace(dotEnvContent)+"\n"), 0600); writeError != nil {
		t.Fatalf("write dotenv: %v", writeError)
	}
}
