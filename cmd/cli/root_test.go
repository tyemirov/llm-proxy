package main

import (
	"errors"
	"fmt"
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
	providerValues := defaultProviderYAMLValues()
	providerValues.OpenAIAPIKey = "${P411_OPENAI_KEY}"
	providerValues.OpenAIBaseURL = "https://openai.example/v1"
	providerValues.OpenAITranscriptionsURL = "https://openai.example/v1/audio/transcriptions"
	providerValues.DeepSeekAPIKey = "${P411_DEEPSEEK_KEY}"
	providerValues.DeepSeekBaseURL = "https://deepseek.example"
	providerValues.DashScopeAPIKey = "${P411_DASHSCOPE_KEY}"
	providerValues.DashScopeBaseURL = "https://dashscope.example"
	providerValues.MoonshotAPIKey = "${P411_MOONSHOT_KEY}"
	providerValues.MoonshotBaseURL = "https://moonshot.example"
	providerValues.SiliconFlowAPIKey = "${P411_SILICONFLOW_KEY}"
	providerValues.SiliconFlowBaseURL = "https://siliconflow.example"
	providerValues.SiliconFlowTranscriptionsURL = "https://siliconflow.example/audio/transcriptions"
	providerValues.ZhipuAPIKey = "${P411_ZHIPU_KEY}"
	providerValues.ZhipuBaseURL = "https://zhipu.example"
	providerValues.ZhipuTranscriptionsURL = "https://zhipu.example/audio/transcriptions"
	providerValues.GeminiAPIKey = "${P411_GEMINI_KEY}"
	providerValues.GeminiBaseURL = "https://gemini.example"
	providerValues.AnthropicAPIKey = "${P411_ANTHROPIC_KEY}"
	providerValues.AnthropicBaseURL = "https://anthropic.example"
	providerValues.GrokAPIKey = "${P411_GROK_KEY}"
	providerValues.GrokBaseURL = "https://grok.example"
	providerValues.GrokTranscriptionsURL = "https://grok.example/stt"
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
`+completeProvidersYAML(providerValues))
	writeTestDotEnv(t, tempDir, `
P411_SERVICE_SECRET=dotenv-secret
P411_OPENAI_KEY=sk-openai
P411_DEEPSEEK_KEY=sk-deepseek
P411_DASHSCOPE_KEY=sk-dashscope
P411_MOONSHOT_KEY=sk-moonshot
P411_SILICONFLOW_KEY=sk-siliconflow
P411_ZHIPU_KEY=sk-zhipu
P411_GEMINI_KEY=sk-gemini
P411_ANTHROPIC_KEY=sk-ant
P411_GROK_KEY=sk-xai
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
	if capturedConfiguration.OpenAIBaseURL != "https://openai.example/v1" {
		t.Fatalf("openAIBaseURL=%q", capturedConfiguration.OpenAIBaseURL)
	}
	if capturedConfiguration.OpenAITranscriptionsURL != "https://openai.example/v1/audio/transcriptions" {
		t.Fatalf("openAITranscriptionsURL=%q", capturedConfiguration.OpenAITranscriptionsURL)
	}
	if capturedConfiguration.Tenants[0].Defaults.Provider != proxy.ProviderNameDeepSeek {
		t.Fatalf("tenantDefaultProvider=%q", capturedConfiguration.Tenants[0].Defaults.Provider)
	}
	if capturedConfiguration.DeepSeekBaseURL != "https://deepseek.example" {
		t.Fatalf("deepSeekBaseURL=%q", capturedConfiguration.DeepSeekBaseURL)
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
	if capturedConfiguration.AnthropicKey != "sk-ant" {
		t.Fatalf("anthropicKey=%q", capturedConfiguration.AnthropicKey)
	}
	if capturedConfiguration.AnthropicBaseURL != "https://anthropic.example" {
		t.Fatalf("anthropicBaseURL=%q", capturedConfiguration.AnthropicBaseURL)
	}
	if capturedConfiguration.ZhipuTranscriptionsURL != "https://zhipu.example/audio/transcriptions" {
		t.Fatalf("zhipuTranscriptionsURL=%q", capturedConfiguration.ZhipuTranscriptionsURL)
	}
	if capturedConfiguration.GrokKey != "sk-xai" {
		t.Fatalf("grokKey=%q", capturedConfiguration.GrokKey)
	}
	if capturedConfiguration.GrokBaseURL != "https://grok.example" {
		t.Fatalf("grokBaseURL=%q", capturedConfiguration.GrokBaseURL)
	}
	if capturedConfiguration.GrokTranscriptionsURL != "https://grok.example/stt" {
		t.Fatalf("grokTranscriptionsURL=%q", capturedConfiguration.GrokTranscriptionsURL)
	}
	if capturedConfiguration.ProviderModels[proxy.ProviderNameDeepSeek].Text.DefaultModel != "deepseek-v4-flash" {
		t.Fatalf("deepseek default model=%q", capturedConfiguration.ProviderModels[proxy.ProviderNameDeepSeek].Text.DefaultModel)
	}
	openAIModels := capturedConfiguration.ProviderModels[proxy.ProviderNameOpenAI].Text.Models
	if len(openAIModels) < 3 || openAIModels[2].ID != "gpt-4.1" || openAIModels[2].RequestProfile != "openai_responses_temperature_tools" || !openAIModels[2].WebSearch {
		t.Fatalf("openai model catalog=%+v", openAIModels)
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
`+completeLiteralProvidersYAML())
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

func TestRootCommandUsesDefaultTenantProvidersFromConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
tenants:
  - id: default
    secret: "sekret"
`+completeLiteralProvidersYAML())
	withServeProxy(t, func(configuration proxy.Configuration, structuredLogger *zap.SugaredLogger) error {
		if _, buildError := proxy.BuildRouter(configuration, structuredLogger); buildError != nil {
			t.Fatalf("BuildRouter error: %v", buildError)
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

func TestRootCommandRejectsMissingTenantSecretPlaceholder(t *testing.T) {
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
`+completeLiteralProvidersYAML())
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "config_placeholder_missing: names=P411_MISSING_SERVICE_SECRET") {
		t.Fatalf("error=%v want missing tenant secret placeholder", executeError)
	}
}

func TestRootCommandAllowsMissingNonDefaultProviderKey(t *testing.T) {
	tempDir := t.TempDir()
	providerValues := defaultProviderYAMLValues()
	providerValues.GeminiAPIKey = "${P411_MISSING_GEMINI_KEY}"
	configPath := writeTestConfig(t, tempDir, `
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: openai
      model: gpt-4.1
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
`+completeProvidersYAML(providerValues))

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
	if capturedConfiguration.GeminiKey != "" {
		t.Fatalf("geminiKey=%q want empty disabled non-default provider", capturedConfiguration.GeminiKey)
	}
}

func TestRootCommandRejectsPartialMissingProviderKeyPlaceholder(t *testing.T) {
	tempDir := t.TempDir()
	providerValues := defaultProviderYAMLValues()
	providerValues.GeminiAPIKey = "sk-${P411_MISSING_GEMINI_SUFFIX}"
	configPath := writeTestConfig(t, tempDir, `
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: openai
      model: gpt-4.1
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
`+completeProvidersYAML(providerValues))
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "config_placeholder_missing: names=P411_MISSING_GEMINI_SUFFIX") {
		t.Fatalf("error=%v want missing partial provider key placeholder", executeError)
	}
}

func TestRootCommandRejectsMissingDefaultDictationProviderKey(t *testing.T) {
	tempDir := t.TempDir()
	providerValues := defaultProviderYAMLValues()
	providerValues.SiliconFlowAPIKey = "${P411_MISSING_SILICONFLOW_KEY}"
	configPath := writeTestConfig(t, tempDir, `
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: openai
      model: gpt-4.1
      dictation_provider: siliconflow
      dictation_model: FunAudioLLM/SenseVoiceSmall
`+completeProvidersYAML(providerValues))
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "provider_api_key_required: provider=siliconflow field=providers.siliconflow.api_key") {
		t.Fatalf("error=%v want missing default dictation provider key", executeError)
	}
}

func TestRootCommandRejectsMissingDefaultTextProviderKeys(t *testing.T) {
	testCases := []struct {
		name          string
		provider      string
		model         string
		missingKey    func(*providerYAMLValues)
		expectedError string
	}{
		{
			name:     "dashscope alias",
			provider: providerAliasQwen,
			model:    proxy.ModelNameDashScopeQwenPlus,
			missingKey: func(values *providerYAMLValues) {
				values.DashScopeAPIKey = "${P411_MISSING_DASHSCOPE_KEY}"
			},
			expectedError: "provider_api_key_required: provider=dashscope field=providers.dashscope.api_key",
		},
		{
			name:     "moonshot alias",
			provider: providerAliasKimi,
			model:    proxy.ModelNameMoonshotKimi,
			missingKey: func(values *providerYAMLValues) {
				values.MoonshotAPIKey = "${P411_MISSING_MOONSHOT_KEY}"
			},
			expectedError: "provider_api_key_required: provider=moonshot field=providers.moonshot.api_key",
		},
		{
			name:     "zhipu alias",
			provider: providerAliasGLM,
			model:    proxy.ModelNameZhipuGLM,
			missingKey: func(values *providerYAMLValues) {
				values.ZhipuAPIKey = "${P411_MISSING_ZHIPU_KEY}"
			},
			expectedError: "provider_api_key_required: provider=zhipu field=providers.zhipu.api_key",
		},
		{
			name:     "anthropic alias",
			provider: providerAliasClaude,
			model:    proxy.ModelNameClaudeSonnet46,
			missingKey: func(values *providerYAMLValues) {
				values.AnthropicAPIKey = "${P411_MISSING_ANTHROPIC_KEY}"
			},
			expectedError: "provider_api_key_required: provider=anthropic field=providers.anthropic.api_key",
		},
		{
			name:     "grok alias",
			provider: providerAliasXAI,
			model:    proxy.ModelNameGrok43,
			missingKey: func(values *providerYAMLValues) {
				values.GrokAPIKey = "${P411_MISSING_XAI_KEY}"
			},
			expectedError: "provider_api_key_required: provider=grok field=providers.grok.api_key",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			tempDir := subTest.TempDir()
			providerValues := defaultProviderYAMLValues()
			testCase.missingKey(&providerValues)
			configPath := writeTestConfig(subTest, tempDir, `
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: `+testCase.provider+`
      model: `+testCase.model+`
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
`+completeProvidersYAML(providerValues))
			withServeProxy(subTest, failingServeProxy(subTest))

			executeError := executeRootCommand(subTest, "--config", configPath)
			if executeError == nil || !strings.Contains(executeError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want %q", executeError, testCase.expectedError)
			}
		})
	}
}

func TestRootCommandRejectsUnsupportedDefaultDictationProviderAfterCredentialValidation(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: openai
      model: gpt-4.1
      dictation_provider: deepseek
      dictation_model: deepseek-v4-flash
`+completeLiteralProvidersYAML())
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "unsupported provider endpoint: provider=deepseek endpoint=dictation") {
		t.Fatalf("error=%v want unsupported default dictation provider", executeError)
	}
}

func TestRootCommandRejectsUnknownDefaultTextProviderAfterCredentialValidation(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: unknown
      model: gpt-4.1
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
`+completeLiteralProvidersYAML())
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "unknown provider: unknown") {
		t.Fatalf("error=%v want unknown default text provider", executeError)
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

func TestRootCommandRejectsIncompleteStaticProviderConfig(t *testing.T) {
	testCases := []struct {
		name          string
		providersYAML string
		expectedError string
	}{
		{
			name: "missing default provider api key",
			providersYAML: `
providers:
  openai:
    api_key: "sk-openai"
    base_url: "https://api.openai.com/v1"
    transcriptions_url: "https://api.openai.com/v1/audio/transcriptions"
  deepseek:
    api_key: "sk-deepseek"
    base_url: "https://api.deepseek.com"
  dashscope:
    api_key: "sk-dashscope"
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
  moonshot:
    api_key: "sk-moonshot"
    base_url: "https://api.moonshot.ai/v1"
  siliconflow:
    api_key: "sk-siliconflow"
    base_url: "https://api.siliconflow.com/v1"
    transcriptions_url: "https://api.siliconflow.com/v1/audio/transcriptions"
  zhipu:
    api_key: "sk-zhipu"
    base_url: "https://open.bigmodel.cn/api/paas/v4"
    transcriptions_url: "https://api.z.ai/api/paas/v4/audio/transcriptions"
  gemini:
    api_key: ""
    base_url: "https://generativelanguage.googleapis.com/v1"
  anthropic:
    api_key: "sk-anthropic"
    base_url: "https://api.anthropic.com"
  grok:
    api_key: "sk-grok"
    base_url: "https://api.x.ai/v1"
    transcriptions_url: "https://api.x.ai/v1/stt"
`,
			expectedError: "provider_api_key_required: provider=gemini field=providers.gemini.api_key",
		},
		{
			name: "missing provider base url",
			providersYAML: `
providers:
  openai:
    api_key: "sk-openai"
    base_url: "https://api.openai.com/v1"
    transcriptions_url: "https://api.openai.com/v1/audio/transcriptions"
  deepseek:
    api_key: "sk-deepseek"
    base_url: "https://api.deepseek.com"
  dashscope:
    api_key: "sk-dashscope"
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
  moonshot:
    api_key: "sk-moonshot"
    base_url: "https://api.moonshot.ai/v1"
  siliconflow:
    api_key: "sk-siliconflow"
    base_url: "https://api.siliconflow.com/v1"
    transcriptions_url: "https://api.siliconflow.com/v1/audio/transcriptions"
  zhipu:
    api_key: "sk-zhipu"
    base_url: "https://open.bigmodel.cn/api/paas/v4"
  gemini:
    api_key: "sk-gemini"
    base_url: "https://generativelanguage.googleapis.com/v1"
  anthropic:
    api_key: "sk-anthropic"
    base_url: "https://api.anthropic.com"
  grok:
    api_key: "sk-grok"
    base_url: ""
    transcriptions_url: "https://api.x.ai/v1/stt"
`,
			expectedError: "provider_base_url_required: provider=grok field=providers.grok.base_url",
		},
		{
			name: "missing openai transcriptions url",
			providersYAML: `
providers:
  openai:
    api_key: "sk-openai"
    base_url: "https://api.openai.com/v1"
    transcriptions_url: ""
  deepseek:
    api_key: "sk-deepseek"
    base_url: "https://api.deepseek.com"
  dashscope:
    api_key: "sk-dashscope"
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
  moonshot:
    api_key: "sk-moonshot"
    base_url: "https://api.moonshot.ai/v1"
  siliconflow:
    api_key: "sk-siliconflow"
    base_url: "https://api.siliconflow.com/v1"
    transcriptions_url: "https://api.siliconflow.com/v1/audio/transcriptions"
  zhipu:
    api_key: "sk-zhipu"
    base_url: "https://open.bigmodel.cn/api/paas/v4"
    transcriptions_url: "https://api.z.ai/api/paas/v4/audio/transcriptions"
  gemini:
    api_key: "sk-gemini"
    base_url: "https://generativelanguage.googleapis.com/v1"
  anthropic:
    api_key: "sk-anthropic"
    base_url: "https://api.anthropic.com"
  grok:
    api_key: "sk-grok"
    base_url: "https://api.x.ai/v1"
    transcriptions_url: "https://api.x.ai/v1/stt"
`,
			expectedError: "provider_transcriptions_url_required: provider=openai field=providers.openai.transcriptions_url",
		},
		{
			name: "missing siliconflow transcriptions url",
			providersYAML: `
providers:
  openai:
    api_key: "sk-openai"
    base_url: "https://api.openai.com/v1"
    transcriptions_url: "https://api.openai.com/v1/audio/transcriptions"
  deepseek:
    api_key: "sk-deepseek"
    base_url: "https://api.deepseek.com"
  dashscope:
    api_key: "sk-dashscope"
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
  moonshot:
    api_key: "sk-moonshot"
    base_url: "https://api.moonshot.ai/v1"
  siliconflow:
    api_key: "sk-siliconflow"
    base_url: "https://api.siliconflow.com/v1"
    transcriptions_url: ""
  zhipu:
    api_key: "sk-zhipu"
    base_url: "https://open.bigmodel.cn/api/paas/v4"
    transcriptions_url: "https://api.z.ai/api/paas/v4/audio/transcriptions"
  gemini:
    api_key: "sk-gemini"
    base_url: "https://generativelanguage.googleapis.com/v1"
  anthropic:
    api_key: "sk-anthropic"
    base_url: "https://api.anthropic.com"
  grok:
    api_key: "sk-grok"
    base_url: "https://api.x.ai/v1"
    transcriptions_url: "https://api.x.ai/v1/stt"
`,
			expectedError: "provider_transcriptions_url_required: provider=siliconflow field=providers.siliconflow.transcriptions_url",
		},
		{
			name: "missing zhipu transcriptions url",
			providersYAML: `
providers:
  openai:
    api_key: "sk-openai"
    base_url: "https://api.openai.com/v1"
    transcriptions_url: "https://api.openai.com/v1/audio/transcriptions"
  deepseek:
    api_key: "sk-deepseek"
    base_url: "https://api.deepseek.com"
  dashscope:
    api_key: "sk-dashscope"
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
  moonshot:
    api_key: "sk-moonshot"
    base_url: "https://api.moonshot.ai/v1"
  siliconflow:
    api_key: "sk-siliconflow"
    base_url: "https://api.siliconflow.com/v1"
    transcriptions_url: "https://api.siliconflow.com/v1/audio/transcriptions"
  zhipu:
    api_key: "sk-zhipu"
    base_url: "https://open.bigmodel.cn/api/paas/v4"
    transcriptions_url: ""
  gemini:
    api_key: "sk-gemini"
    base_url: "https://generativelanguage.googleapis.com/v1"
  anthropic:
    api_key: "sk-anthropic"
    base_url: "https://api.anthropic.com"
  grok:
    api_key: "sk-grok"
    base_url: "https://api.x.ai/v1"
    transcriptions_url: "https://api.x.ai/v1/stt"
`,
			expectedError: "provider_transcriptions_url_required: provider=zhipu field=providers.zhipu.transcriptions_url",
		},
		{
			name: "missing grok transcriptions url",
			providersYAML: `
providers:
  openai:
    api_key: "sk-openai"
    base_url: "https://api.openai.com/v1"
    transcriptions_url: "https://api.openai.com/v1/audio/transcriptions"
  deepseek:
    api_key: "sk-deepseek"
    base_url: "https://api.deepseek.com"
  dashscope:
    api_key: "sk-dashscope"
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
  moonshot:
    api_key: "sk-moonshot"
    base_url: "https://api.moonshot.ai/v1"
  siliconflow:
    api_key: "sk-siliconflow"
    base_url: "https://api.siliconflow.com/v1"
    transcriptions_url: "https://api.siliconflow.com/v1/audio/transcriptions"
  zhipu:
    api_key: "sk-zhipu"
    base_url: "https://open.bigmodel.cn/api/paas/v4"
    transcriptions_url: "https://api.z.ai/api/paas/v4/audio/transcriptions"
  gemini:
    api_key: "sk-gemini"
    base_url: "https://generativelanguage.googleapis.com/v1"
  anthropic:
    api_key: "sk-anthropic"
    base_url: "https://api.anthropic.com"
  grok:
    api_key: "sk-grok"
    base_url: "https://api.x.ai/v1"
    transcriptions_url: ""
`,
			expectedError: "provider_transcriptions_url_required: provider=grok field=providers.grok.transcriptions_url",
		},
		{
			name:          "missing provider text models",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "models:\n        - id: \"qwen-plus\"", "models: []", 1),
			expectedError: "invalid_model_catalog: provider=dashscope endpoint=text field=providers.dashscope.text.models",
		},
		{
			name:          "blank provider text default model",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "default_model: \"gpt-4.1\"", "default_model: \"\"", 1),
			expectedError: "invalid_model_catalog: provider=openai endpoint=text field=providers.openai.text.default_model",
		},
		{
			name:          "blank provider dictation default model",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "dictation:\n      default_model: \"gpt-4o-mini-transcribe\"", "dictation:\n      default_model: \"\"", 1),
			expectedError: "invalid_model_catalog: provider=openai endpoint=dictation field=providers.openai.dictation.default_model",
		},
		{
			name:          "blank provider model id",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "- id: \"gpt-4o-mini\"", "- id: \"\"", 1),
			expectedError: "invalid_model_catalog: provider=openai endpoint=text field=providers.openai.text.models[0].id",
		},
		{
			name:          "duplicate provider model id",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "- id: \"gpt-4o\"", "- id: \"gpt-4o-mini\"", 1),
			expectedError: "invalid_model_catalog: provider=openai endpoint=text duplicate_model=gpt-4o-mini",
		},
		{
			name:          "default provider model missing from catalog",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "default_model: \"gpt-4.1\"", "default_model: \"gpt-not-configured\"", 1),
			expectedError: "invalid_model_catalog: provider=openai endpoint=text default_model=gpt-not-configured",
		},
		{
			name:          "negative provider output token limit",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "output_token_limit: 65536", "output_token_limit: -1", 1),
			expectedError: "invalid_model_catalog: provider=gemini endpoint=text field=providers.gemini.text.models[0].output_token_limit",
		},
		{
			name:          "anthropic output token limit required",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "id: \"claude-sonnet-4-6\"\n          output_token_limit: 64000", "id: \"claude-sonnet-4-6\"\n          output_token_limit: 0", 1),
			expectedError: "invalid_model_catalog: provider=anthropic endpoint=text field=providers.anthropic.text.models[1].output_token_limit",
		},
		{
			name:          "blank openai request profile",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "request_profile: \"openai_responses_temperature\"", "request_profile: \"\"", 1),
			expectedError: "invalid_model_catalog: provider=openai endpoint=text",
		},
		{
			name:          "invalid openai request profile",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "request_profile: \"openai_responses_temperature\"", "request_profile: \"future_profile\"", 1),
			expectedError: "invalid_model_catalog: provider=openai endpoint=text profile=future_profile",
		},
		{
			name:          "non openai request profile",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "id: \"deepseek-v4-flash\"", "id: \"deepseek-v4-flash\"\n          request_profile: \"openai_responses_base\"", 1),
			expectedError: "invalid_model_catalog: provider=deepseek endpoint=text profile=openai_responses_base",
		},
		{
			name:          "non openai web search",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "id: \"deepseek-v4-flash\"", "id: \"deepseek-v4-flash\"\n          web_search: true", 1),
			expectedError: "invalid_model_catalog: provider=deepseek endpoint=text field=providers.deepseek.text.models[0].web_search",
		},
		{
			name:          "dictation web search",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "id: \"gpt-4o-mini-transcribe\"", "id: \"gpt-4o-mini-transcribe\"\n          web_search: true", 1),
			expectedError: "invalid_model_catalog: provider=openai endpoint=dictation field=providers.openai.dictation.models[0].web_search",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			tempDir := subTest.TempDir()
			configPath := writeTestConfig(subTest, tempDir, `
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: gemini
      model: gemini-3.5-flash
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
`+testCase.providersYAML)
			withServeProxy(subTest, failingServeProxy(subTest))

			executeError := executeRootCommand(subTest, "--config", configPath)
			if executeError == nil || !strings.Contains(executeError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want contains %q", executeError, testCase.expectedError)
			}
		})
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

func completeLiteralProvidersYAML() string {
	return completeProvidersYAML(defaultProviderYAMLValues())
}

type providerYAMLValues struct {
	OpenAIAPIKey                 string
	OpenAIBaseURL                string
	OpenAITranscriptionsURL      string
	DeepSeekAPIKey               string
	DeepSeekBaseURL              string
	DashScopeAPIKey              string
	DashScopeBaseURL             string
	MoonshotAPIKey               string
	MoonshotBaseURL              string
	SiliconFlowAPIKey            string
	SiliconFlowBaseURL           string
	SiliconFlowTranscriptionsURL string
	ZhipuAPIKey                  string
	ZhipuBaseURL                 string
	ZhipuTranscriptionsURL       string
	GeminiAPIKey                 string
	GeminiBaseURL                string
	AnthropicAPIKey              string
	AnthropicBaseURL             string
	GrokAPIKey                   string
	GrokBaseURL                  string
	GrokTranscriptionsURL        string
}

func defaultProviderYAMLValues() providerYAMLValues {
	return providerYAMLValues{
		OpenAIAPIKey:                 "sk-openai",
		OpenAIBaseURL:                "https://api.openai.com/v1",
		OpenAITranscriptionsURL:      "https://api.openai.com/v1/audio/transcriptions",
		DeepSeekAPIKey:               "sk-deepseek",
		DeepSeekBaseURL:              "https://api.deepseek.com",
		DashScopeAPIKey:              "sk-dashscope",
		DashScopeBaseURL:             "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		MoonshotAPIKey:               "sk-moonshot",
		MoonshotBaseURL:              "https://api.moonshot.ai/v1",
		SiliconFlowAPIKey:            "sk-siliconflow",
		SiliconFlowBaseURL:           "https://api.siliconflow.com/v1",
		SiliconFlowTranscriptionsURL: "https://api.siliconflow.com/v1/audio/transcriptions",
		ZhipuAPIKey:                  "sk-zhipu",
		ZhipuBaseURL:                 "https://open.bigmodel.cn/api/paas/v4",
		ZhipuTranscriptionsURL:       "https://api.z.ai/api/paas/v4/audio/transcriptions",
		GeminiAPIKey:                 "sk-gemini",
		GeminiBaseURL:                "https://generativelanguage.googleapis.com/v1",
		AnthropicAPIKey:              "sk-anthropic",
		AnthropicBaseURL:             "https://api.anthropic.com",
		GrokAPIKey:                   "sk-grok",
		GrokBaseURL:                  "https://api.x.ai/v1",
		GrokTranscriptionsURL:        "https://api.x.ai/v1/stt",
	}
}

func completeProvidersYAML(values providerYAMLValues) string {
	return fmt.Sprintf(`
providers:
  openai:
    api_key: "%s"
    base_url: "%s"
    transcriptions_url: "%s"
    text:
      default_model: "gpt-4.1"
      models:
        - id: "gpt-4o-mini"
          request_profile: "openai_responses_temperature"
        - id: "gpt-4o"
          request_profile: "openai_responses_temperature_tools"
          web_search: true
        - id: "gpt-4.1"
          request_profile: "openai_responses_temperature_tools"
          web_search: true
        - id: "gpt-5-mini"
          request_profile: "openai_responses_base"
        - id: "gpt-5"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
        - id: "gpt-5.5"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
        - id: "gpt-5.5-pro"
          request_profile: "openai_responses_reasoning_tools"
          web_search: true
    dictation:
      default_model: "gpt-4o-mini-transcribe"
      models:
        - id: "gpt-4o-mini-transcribe"
        - id: "gpt-4o-transcribe"
  deepseek:
    api_key: "%s"
    base_url: "%s"
    text:
      default_model: "deepseek-v4-flash"
      models:
        - id: "deepseek-v4-flash"
        - id: "deepseek-v4-pro"
        - id: "deepseek-chat"
        - id: "deepseek-reasoner"
  dashscope:
    api_key: "%s"
    base_url: "%s"
    text:
      default_model: "qwen-plus"
      models:
        - id: "qwen-plus"
  moonshot:
    api_key: "%s"
    base_url: "%s"
    text:
      default_model: "kimi-k2-0905-preview"
      models:
        - id: "kimi-k2-0905-preview"
  siliconflow:
    api_key: "%s"
    base_url: "%s"
    transcriptions_url: "%s"
    text:
      default_model: "deepseek-ai/DeepSeek-R1"
      models:
        - id: "deepseek-ai/DeepSeek-R1"
    dictation:
      default_model: "FunAudioLLM/SenseVoiceSmall"
      models:
        - id: "FunAudioLLM/SenseVoiceSmall"
  zhipu:
    api_key: "%s"
    base_url: "%s"
    transcriptions_url: "%s"
    text:
      default_model: "glm-5.1"
      models:
        - id: "glm-5.1"
    dictation:
      default_model: "glm-asr-2512"
      models:
        - id: "glm-asr-2512"
  gemini:
    api_key: "%s"
    base_url: "%s"
    text:
      default_model: "gemini-2.5-flash"
      models:
        - id: "gemini-3.5-flash"
          output_token_limit: 65536
        - id: "gemini-3.1-flash-lite"
          output_token_limit: 65536
        - id: "gemini-2.5-flash"
          output_token_limit: 65536
        - id: "gemini-2.5-flash-lite"
          output_token_limit: 65536
        - id: "gemini-2.5-pro"
          output_token_limit: 65536
  anthropic:
    api_key: "%s"
    base_url: "%s"
    text:
      default_model: "claude-sonnet-4-6"
      models:
        - id: "claude-opus-4-8"
          output_token_limit: 128000
        - id: "claude-sonnet-4-6"
          output_token_limit: 64000
        - id: "claude-haiku-4-5-20251001"
          output_token_limit: 64000
        - id: "claude-haiku-4-5"
          output_token_limit: 64000
        - id: "claude-sonnet-4-5-20250929"
          output_token_limit: 64000
        - id: "claude-sonnet-4-5"
          output_token_limit: 64000
        - id: "claude-opus-4-1-20250805"
          output_token_limit: 32000
        - id: "claude-opus-4-1"
          output_token_limit: 32000
  grok:
    api_key: "%s"
    base_url: "%s"
    transcriptions_url: "%s"
    text:
      default_model: "grok-4.3"
      models:
        - id: "grok-4.3"
        - id: "grok-4.3-latest"
        - id: "grok-latest"
        - id: "grok-build-0.1"
        - id: "grok-code-fast"
        - id: "grok-code-fast-1"
        - id: "grok-code-fast-1-0825"
    dictation:
      default_model: "xai-stt"
      models:
        - id: "xai-stt"
`,
		values.OpenAIAPIKey,
		values.OpenAIBaseURL,
		values.OpenAITranscriptionsURL,
		values.DeepSeekAPIKey,
		values.DeepSeekBaseURL,
		values.DashScopeAPIKey,
		values.DashScopeBaseURL,
		values.MoonshotAPIKey,
		values.MoonshotBaseURL,
		values.SiliconFlowAPIKey,
		values.SiliconFlowBaseURL,
		values.SiliconFlowTranscriptionsURL,
		values.ZhipuAPIKey,
		values.ZhipuBaseURL,
		values.ZhipuTranscriptionsURL,
		values.GeminiAPIKey,
		values.GeminiBaseURL,
		values.AnthropicAPIKey,
		values.AnthropicBaseURL,
		values.GrokAPIKey,
		values.GrokBaseURL,
		values.GrokTranscriptionsURL,
	)
}
