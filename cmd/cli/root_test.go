package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
	providerValues.OpenAIBaseURL = "https://openai.example/v1"
	providerValues.OpenAITranscriptionsURL = "https://openai.example/v1/audio/transcriptions"
	providerValues.DeepSeekBaseURL = "https://deepseek.example"
	providerValues.DashScopeBaseURL = "https://dashscope.example"
	providerValues.MoonshotBaseURL = "https://moonshot.example"
	providerValues.MiniMaxBaseURL = "https://minimax.example"
	providerValues.SiliconFlowBaseURL = "https://siliconflow.example"
	providerValues.SiliconFlowTranscriptionsURL = "https://siliconflow.example/audio/transcriptions"
	providerValues.ZAIBaseURL = "https://zai.example"
	providerValues.ZAITranscriptionsURL = "https://zai.example/audio/transcriptions"
	providerValues.GeminiBaseURL = "https://gemini.example"
	providerValues.AnthropicBaseURL = "https://anthropic.example"
	providerValues.MetaBaseURL = "https://meta.example/v1"
	providerValues.XAIBaseURL = "https://xai.example"
	providerValues.XAITranscriptionsURL = "https://xai.example/stt"
	configPath := writeTestConfig(t, tempDir, `
server:
  port: 18080
  log_level: debug
  workers: 2
  queue_size: 9
  request_timeout_seconds: 7
  max_request_timeout_seconds: 11
  max_prompt_bytes: 1024
  max_input_audio_bytes: 2048
  upstream_rate_limits:
    - origin: "https://openai.example"
      max_requests: 12
      interval: "1m"
management:
  public_origin: "https://llm-proxy.example"
  ui_description: "LLM Proxy"
  ui_origins:
    - "https://llm-proxy.example"
    - "http://127.0.0.1:4179"
  tauth_url: "https://tauth.example"
  tauth_tenant_id: "llm-proxy"
  google_client_id: "google-client-id"
  login_path: "/auth/google"
  logout_path: "/auth/logout"
  nonce_path: "/auth/nonce"
  session_path: "/auth/session"
  jwt_signing_key: "${P411_TAUTH_JWT_SIGNING_KEY}"
  jwt_issuer: "tauth"
  session_cookie_name: "llm_proxy_session"
  database_path: "${P411_MANAGEMENT_DATABASE_PATH}"
  usage_queue_size: 3
  provider_key_encryption_key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
  management_api_origin: "https://llm-proxy-api.example"
  proxy_origin: "https://llm-proxy-api.example"
`+completeProvidersYAML(providerValues))
	writeTestDotEnv(t, tempDir, `
P411_TAUTH_JWT_SIGNING_KEY=tauth-signing-key
P411_MANAGEMENT_DATABASE_PATH=/var/lib/llm-proxy/management.sqlite
P411_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
`)

	var capturedConfiguration proxy.Configuration
	withServeProxy(t, func(configuration proxy.Configuration, structuredLogger *zap.SugaredLogger) error {
		capturedConfiguration = configuration
		return nil
	})

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError != nil {
		t.Fatalf("ExecuteC error: %v", executeError)
	}
	if capturedConfiguration.OpenAIBaseURL != "https://openai.example/v1" {
		t.Fatalf("openAIBaseURL=%q", capturedConfiguration.OpenAIBaseURL)
	}
	if capturedConfiguration.OpenAITranscriptionsURL != "https://openai.example/v1/audio/transcriptions" {
		t.Fatalf("openAITranscriptionsURL=%q", capturedConfiguration.OpenAITranscriptionsURL)
	}
	if capturedConfiguration.DeepSeekBaseURL != "https://deepseek.example" {
		t.Fatalf("deepSeekBaseURL=%q", capturedConfiguration.DeepSeekBaseURL)
	}
	if capturedConfiguration.Port != 18080 {
		t.Fatalf("port=%d", capturedConfiguration.Port)
	}
	if capturedConfiguration.RequestTimeoutSeconds != 7 || capturedConfiguration.MaxRequestTimeoutSeconds != 11 {
		t.Fatalf(
			"request timeout default=%d maximum=%d",
			capturedConfiguration.RequestTimeoutSeconds,
			capturedConfiguration.MaxRequestTimeoutSeconds,
		)
	}
	if len(capturedConfiguration.UpstreamRateLimits) != 1 || capturedConfiguration.UpstreamRateLimits[0].Origin != "https://openai.example" || capturedConfiguration.UpstreamRateLimits[0].MaxRequests != 12 || capturedConfiguration.UpstreamRateLimits[0].Interval != "1m" {
		t.Fatalf("upstreamRateLimits=%+v", capturedConfiguration.UpstreamRateLimits)
	}
	if capturedConfiguration.Management.PublicOrigin != "https://llm-proxy.example" {
		t.Fatalf("management public origin=%q", capturedConfiguration.Management.PublicOrigin)
	}
	if capturedConfiguration.Management.UIDescription != "LLM Proxy" {
		t.Fatalf("management ui description=%q", capturedConfiguration.Management.UIDescription)
	}
	if len(capturedConfiguration.Management.UIOrigins) != 2 || capturedConfiguration.Management.UIOrigins[1] != "http://127.0.0.1:4179" {
		t.Fatalf("management ui origins=%q", capturedConfiguration.Management.UIOrigins)
	}
	if capturedConfiguration.Management.TAuthURL != "https://tauth.example" {
		t.Fatalf("management tauth url=%q", capturedConfiguration.Management.TAuthURL)
	}
	if capturedConfiguration.Management.TAuthTenantID != "llm-proxy" {
		t.Fatalf("management tenant id=%q", capturedConfiguration.Management.TAuthTenantID)
	}
	if capturedConfiguration.Management.GoogleClientID != "google-client-id" {
		t.Fatalf("management google client id=%q", capturedConfiguration.Management.GoogleClientID)
	}
	if capturedConfiguration.Management.LoginPath != "/auth/google" || capturedConfiguration.Management.LogoutPath != "/auth/logout" || capturedConfiguration.Management.NoncePath != "/auth/nonce" || capturedConfiguration.Management.SessionPath != "/auth/session" {
		t.Fatalf("management auth paths=%q %q %q %q", capturedConfiguration.Management.LoginPath, capturedConfiguration.Management.LogoutPath, capturedConfiguration.Management.NoncePath, capturedConfiguration.Management.SessionPath)
	}
	if capturedConfiguration.Management.JWTSigningKey != "tauth-signing-key" {
		t.Fatalf("management signing key=%q", capturedConfiguration.Management.JWTSigningKey)
	}
	if capturedConfiguration.Management.SessionCookieName != "llm_proxy_session" {
		t.Fatalf("management cookie name=%q", capturedConfiguration.Management.SessionCookieName)
	}
	if capturedConfiguration.Management.DatabasePath != "/var/lib/llm-proxy/management.sqlite" {
		t.Fatalf("management database path=%q", capturedConfiguration.Management.DatabasePath)
	}
	if capturedConfiguration.Management.UsageQueueSize != 3 {
		t.Fatalf("management usage queue size=%d", capturedConfiguration.Management.UsageQueueSize)
	}
	if capturedConfiguration.Management.ProviderKeyEncryptionKey != "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" {
		t.Fatalf("management provider key encryption key=%q", capturedConfiguration.Management.ProviderKeyEncryptionKey)
	}
	if capturedConfiguration.Management.ManagementAPIOrigin != "https://llm-proxy-api.example" || capturedConfiguration.Management.ProxyOrigin != "https://llm-proxy-api.example" {
		t.Fatalf("management api origins=%q %q", capturedConfiguration.Management.ManagementAPIOrigin, capturedConfiguration.Management.ProxyOrigin)
	}
	if capturedConfiguration.MiniMaxBaseURL != "https://minimax.example" {
		t.Fatalf("miniMax base URL=%q", capturedConfiguration.MiniMaxBaseURL)
	}
	if capturedConfiguration.GeminiBaseURL != "https://gemini.example" {
		t.Fatalf("geminiBaseURL=%q", capturedConfiguration.GeminiBaseURL)
	}
	if capturedConfiguration.AnthropicBaseURL != "https://anthropic.example" {
		t.Fatalf("anthropicBaseURL=%q", capturedConfiguration.AnthropicBaseURL)
	}
	if capturedConfiguration.MetaBaseURL != "https://meta.example/v1" {
		t.Fatalf("meta base URL=%q", capturedConfiguration.MetaBaseURL)
	}
	if capturedConfiguration.ZAITranscriptionsURL != "https://zai.example/audio/transcriptions" {
		t.Fatalf("zaiTranscriptionsURL=%q", capturedConfiguration.ZAITranscriptionsURL)
	}
	if capturedConfiguration.XAIBaseURL != "https://xai.example" {
		t.Fatalf("grokBaseURL=%q", capturedConfiguration.XAIBaseURL)
	}
	if capturedConfiguration.XAITranscriptionsURL != "https://xai.example/stt" {
		t.Fatalf("grokTranscriptionsURL=%q", capturedConfiguration.XAITranscriptionsURL)
	}
	deepSeekDefault, deepSeekDefaultFound := configuredDefaultOffering(capturedConfiguration.ModelCatalog, proxy.ProviderNameDeepSeek, proxy.ModelOperationText)
	if !deepSeekDefaultFound || deepSeekDefault.Model != "deepseek-v4-flash" {
		t.Fatalf("deepseek default offering=%+v found=%t", deepSeekDefault, deepSeekDefaultFound)
	}
	miniMaxOfferings := configuredProviderOfferings(capturedConfiguration.ModelCatalog, proxy.ProviderNameMiniMax)
	if len(miniMaxOfferings) != 7 || miniMaxOfferings[0].Model != proxy.ModelNameMiniMaxM27 || !slices.Equal(miniMaxOfferings[0].DefaultOperations, []string{proxy.ModelOperationText}) {
		t.Fatalf("miniMax offerings=%+v", miniMaxOfferings)
	}
	for _, offering := range miniMaxOfferings {
		if offering.OutputTokenLimit != 204800 {
			t.Fatalf("miniMax offering=%+v", offering)
		}
	}
	openAIOfferings := configuredProviderOfferings(capturedConfiguration.ModelCatalog, proxy.ProviderNameOpenAI)
	if len(openAIOfferings) < 3 || openAIOfferings[2].Model != "gpt-4.1" || openAIOfferings[2].RequestProfile != "openai_responses_temperature_tools" || !openAIOfferings[2].WebSearch {
		t.Fatalf("openai offerings=%+v", openAIOfferings)
	}
	for _, offering := range capturedConfiguration.ModelCatalog.Offerings {
		if slices.Contains(offering.Operations, proxy.ModelOperationText) && (offering.WireContract == "" || offering.ExecutionLifecycle == "") {
			t.Fatalf("provider=%s model=%s missing route capabilities: %+v", offering.Provider, offering.Model, offering)
		}
	}
}

func TestRootCommandRunsProductionLoggerFromConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
server:
  log_level: info
`+completeLiteralRuntimeYAML())
	withServeProxy(t, func(configuration proxy.Configuration, structuredLogger *zap.SugaredLogger) error {
		if configuration.LogLevel != proxy.LogLevelInfo {
			t.Fatalf("logLevel=%q", configuration.LogLevel)
		}
		if configuration.Management.UsageQueueSize != proxy.DefaultManagementUsageQueueSize {
			t.Fatalf("management usage queue size=%d", configuration.Management.UsageQueueSize)
		}
		return nil
	})

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError != nil {
		t.Fatalf("ExecuteC error: %v", executeError)
	}
}

func TestRootCommandRejectsInvalidRequestTimeoutConfiguration(t *testing.T) {
	testCases := []struct {
		name          string
		serverYAML    string
		expectedError string
	}{
		{
			name:          "zero default",
			serverYAML:    "  request_timeout_seconds: 0\n",
			expectedError: "server.request_timeout_seconds must be positive",
		},
		{
			name:          "null default",
			serverYAML:    "  request_timeout_seconds: null\n",
			expectedError: "server.request_timeout_seconds must be positive",
		},
		{
			name:          "empty default",
			serverYAML:    "  request_timeout_seconds:\n",
			expectedError: "server.request_timeout_seconds must be positive",
		},
		{
			name:          "zero maximum",
			serverYAML:    "  max_request_timeout_seconds: 0\n",
			expectedError: "server.max_request_timeout_seconds must be positive",
		},
		{
			name:          "null maximum",
			serverYAML:    "  max_request_timeout_seconds: null\n",
			expectedError: "server.max_request_timeout_seconds must be positive",
		},
		{
			name:          "empty maximum",
			serverYAML:    "  max_request_timeout_seconds:\n",
			expectedError: "server.max_request_timeout_seconds must be positive",
		},
		{
			name:          "default exceeds maximum",
			serverYAML:    "  request_timeout_seconds: 5\n  max_request_timeout_seconds: 4\n",
			expectedError: "server.request_timeout_seconds exceeds server.max_request_timeout_seconds",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			configPath := writeTestConfig(subTest, subTest.TempDir(), `
server:
`+testCase.serverYAML+completeLiteralRuntimeYAML())
			withServeProxy(subTest, failingServeProxy(subTest))

			executeError := executeRootCommand(subTest, "--config", configPath)
			if executeError == nil || !strings.Contains(executeError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want contains %q", executeError, testCase.expectedError)
			}
		})
	}
}

func TestRootCommandRejectsInvalidManagementUsageQueueSize(t *testing.T) {
	for _, configuredValue := range []string{"0", "-1", "null", ""} {
		t.Run("value="+configuredValue, func(subTest *testing.T) {
			configPath := writeTestConfig(subTest, subTest.TempDir(), `
management:
  usage_queue_size: `+configuredValue+`
`+completeProvidersYAML(defaultProviderYAMLValues()))
			withServeProxy(subTest, failingServeProxy(subTest))

			executeError := executeRootCommand(subTest, "--config", configPath)
			if executeError == nil || !strings.Contains(executeError.Error(), "management.usage_queue_size must be positive") {
				subTest.Fatalf("error=%v want positive usage queue size rejection", executeError)
			}
		})
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

func TestRootCommandRejectsPlaceholderDefaultSyntax(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
server:
  max_request_timeout_seconds: 3600
  max_prompt_bytes: 4194304
  max_input_audio_bytes: 26214400
management:
  public_origin: "https://llm-proxy.example"
  ui_description: "LLM Proxy"
  ui_origins: ["https://llm-proxy.example"]
  tauth_url: "https://tauth.example"
  tauth_tenant_id: "llm-proxy"
  google_client_id: "google-client-id"
  login_path: "/auth/google"
  logout_path: "/auth/logout"
  nonce_path: "/auth/nonce"
  session_path: "/auth/session"
  jwt_signing_key: "signing-key"
  session_cookie_name: "llm_proxy_session"
  database_path: "${P411_MISSING_MANAGEMENT_DATABASE_PATH:-management.sqlite}"
  provider_key_encryption_key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
  management_api_origin: "https://llm-proxy.example"
  proxy_origin: "https://llm-proxy.example"
`+completeProvidersYAML(defaultProviderYAMLValues()))
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "config_placeholder_missing: names=P411_MISSING_MANAGEMENT_DATABASE_PATH:-management.sqlite") {
		t.Fatalf("error=%v want default placeholder syntax rejected", executeError)
	}
}

func TestRootCommandRejectsMissingConfigurationPlaceholder(t *testing.T) {
	originalProcessEnvironment := processEnvironment
	processEnvironment = func() []string { return nil }
	t.Cleanup(func() { processEnvironment = originalProcessEnvironment })

	configPath := writeTestConfig(t, t.TempDir(), `
management:
  database_path: "${P411_MISSING_MANAGEMENT_DATABASE_PATH}"
`)
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "config_placeholder_missing: names=P411_MISSING_MANAGEMENT_DATABASE_PATH") {
		t.Fatalf("error=%v want missing placeholder rejection", executeError)
	}
}

func TestRootCommandLoadsPackagedConfigWithManagementEnvironment(t *testing.T) {
	tempDir := t.TempDir()
	packagedConfigPath := filepath.Join("..", "..", "configs", "config.yml")
	packagedConfig, readError := os.ReadFile(packagedConfigPath)
	if readError != nil {
		t.Fatalf("read packaged config: %v", readError)
	}
	configPath := filepath.Join(tempDir, testConfigFileName)
	if writeError := os.WriteFile(configPath, packagedConfig, 0600); writeError != nil {
		t.Fatalf("write packaged config copy: %v", writeError)
	}
	writeTestDotEnv(t, tempDir, `
LLM_PROXY_MANAGEMENT_PUBLIC_ORIGIN=https://llm-proxy.mprlab.com
LLM_PROXY_MANAGEMENT_UI_DESCRIPTION=LLM Proxy
LLM_PROXY_MANAGEMENT_LOOPBACK_ORIGIN=http://127.0.0.1:4179
LLM_PROXY_MANAGEMENT_LOCALHOST_ORIGIN=http://localhost:4179
LLM_PROXY_MANAGEMENT_ADMIN_EMAILS=["admin@example.invalid","ops@example.invalid"]
LLM_PROXY_MANAGEMENT_TAUTH_URL=https://tauth-api.mprlab.com
LLM_PROXY_MANAGEMENT_TAUTH_TENANT_ID=llm-proxy
LLM_PROXY_MANAGEMENT_GOOGLE_CLIENT_ID=925457785190-3frk7j3bsr3ucidtkcohrp2sl07e0paa.apps.googleusercontent.com
LLM_PROXY_MANAGEMENT_TAUTH_LOGIN_PATH=/auth/google
LLM_PROXY_MANAGEMENT_TAUTH_LOGOUT_PATH=/auth/logout
LLM_PROXY_MANAGEMENT_TAUTH_NONCE_PATH=/auth/nonce
LLM_PROXY_MANAGEMENT_TAUTH_SESSION_PATH=/auth/session
LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY=packaged-tauth-signing-key
LLM_PROXY_MANAGEMENT_JWT_ISSUER=tauth
LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME=app_session_llm_proxy
LLM_PROXY_MANAGEMENT_DATABASE_PATH=llm-proxy-management.sqlite
LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
LLM_PROXY_MANAGEMENT_API_ORIGIN=https://llm-proxy-api.mprlab.com
LLM_PROXY_MANAGEMENT_PROXY_ORIGIN=https://llm-proxy-api.mprlab.com
DASHSCOPE_BASE_URL=https://workspace.example.invalid/compatible-mode/v1
`)

	var capturedConfiguration proxy.Configuration
	withServeProxy(t, func(configuration proxy.Configuration, structuredLogger *zap.SugaredLogger) error {
		capturedConfiguration = configuration
		return nil
	})

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError != nil {
		t.Fatalf("ExecuteC error: %v", executeError)
	}
	if capturedConfiguration.Management.PublicOrigin != "https://llm-proxy.mprlab.com" {
		t.Fatalf("public origin=%q", capturedConfiguration.Management.PublicOrigin)
	}
	if capturedConfiguration.Management.TAuthURL != "https://tauth-api.mprlab.com" {
		t.Fatalf("tauth url=%q", capturedConfiguration.Management.TAuthURL)
	}
	if capturedConfiguration.Management.GoogleClientID != "925457785190-3frk7j3bsr3ucidtkcohrp2sl07e0paa.apps.googleusercontent.com" {
		t.Fatalf("google client id=%q", capturedConfiguration.Management.GoogleClientID)
	}
	if capturedConfiguration.Management.JWTSigningKey != "packaged-tauth-signing-key" {
		t.Fatalf("jwt signing key=%q", capturedConfiguration.Management.JWTSigningKey)
	}
	if capturedConfiguration.Management.DatabasePath != "llm-proxy-management.sqlite" {
		t.Fatalf("database path=%q", capturedConfiguration.Management.DatabasePath)
	}
	if capturedConfiguration.Management.UsageQueueSize != proxy.DefaultManagementUsageQueueSize {
		t.Fatalf("usage queue size=%d", capturedConfiguration.Management.UsageQueueSize)
	}
	if capturedConfiguration.Management.ProviderKeyEncryptionKey != "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" {
		t.Fatalf("provider key encryption key=%q", capturedConfiguration.Management.ProviderKeyEncryptionKey)
	}
	if len(capturedConfiguration.Management.AdminEmails) != 2 ||
		capturedConfiguration.Management.AdminEmails[0] != "admin@example.invalid" ||
		capturedConfiguration.Management.AdminEmails[1] != "ops@example.invalid" {
		t.Fatalf("admin emails=%#v", capturedConfiguration.Management.AdminEmails)
	}
	if capturedConfiguration.Management.ManagementAPIOrigin != "https://llm-proxy-api.mprlab.com" || capturedConfiguration.Management.ProxyOrigin != "https://llm-proxy-api.mprlab.com" {
		t.Fatalf("management api origins=%q %q", capturedConfiguration.Management.ManagementAPIOrigin, capturedConfiguration.Management.ProxyOrigin)
	}
	if capturedConfiguration.MetaBaseURL != "https://api.meta.ai/v1" {
		t.Fatalf("meta base URL=%q", capturedConfiguration.MetaBaseURL)
	}
	metaOfferings := configuredProviderOfferings(capturedConfiguration.ModelCatalog, proxy.ProviderNameMeta)
	metaDefault, metaDefaultFound := configuredDefaultOffering(capturedConfiguration.ModelCatalog, proxy.ProviderNameMeta, proxy.ModelOperationText)
	if !metaDefaultFound || metaDefault.Model != proxy.ModelNameMuseSpark11 || len(metaOfferings) != 1 || metaOfferings[0].Model != proxy.ModelNameMuseSpark11 {
		t.Fatalf("meta offerings=%+v default=%+v", metaOfferings, metaDefault)
	}
}

func TestRootCommandServesSanitizedPublicCapabilityAPI(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
server:
  port: 9191
  log_level: debug
  max_prompt_bytes: 3
  max_input_audio_bytes: 26214400
management:
  google_client_id: "${PUBLIC_CAPABILITY_MODE_IGNORES_PRIVATE_PLACEHOLDERS}"
`+completeProvidersYAML(defaultProviderYAMLValues()))
	withServeProxy(t, failingServeProxy(t))
	var capturedConfiguration publicCapabilityAPIConfiguration
	withServePublicCapabilityAPI(t, func(catalog proxy.PublicCapabilityCatalog, port int, logLevel string) error {
		capturedConfiguration = publicCapabilityAPIConfiguration{Catalog: catalog, Port: port, LogLevel: logLevel}
		return nil
	})

	executeError := executeRootCommand(t, "--config", configPath, "--public-capabilities-only")
	if executeError != nil {
		t.Fatalf("ExecuteC error: %v", executeError)
	}
	if capturedConfiguration.Port != 9191 || capturedConfiguration.LogLevel != proxy.LogLevelDebug {
		t.Fatalf("public API server=%+v", capturedConfiguration)
	}
	if len(capturedConfiguration.Catalog.Providers) != 11 {
		t.Fatalf("provider count=%d want=11", len(capturedConfiguration.Catalog.Providers))
	}
	if capturedConfiguration.Catalog.MaxPromptBytes != 3 || capturedConfiguration.Catalog.MaxInputAudioBytes != 25*1024*1024 {
		t.Fatalf("public limits=%+v", capturedConfiguration.Catalog)
	}
	if capturedConfiguration.Catalog.MaxRequestTimeoutSeconds != proxy.DefaultMaxRequestTimeoutSeconds {
		t.Fatalf("maximum request timeout=%d", capturedConfiguration.Catalog.MaxRequestTimeoutSeconds)
	}
}

func TestRootCommandReturnsPublicCapabilityAPIServeError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
server:
  port: 9191
`+completeLiteralRuntimeYAML())
	expectedError := errors.New("public capability API stopped")
	withServeProxy(t, failingServeProxy(t))
	withServePublicCapabilityAPI(t, func(catalog proxy.PublicCapabilityCatalog, port int, logLevel string) error {
		return expectedError
	})

	executeError := executeRootCommand(t, "--config", configPath, "--public-capabilities-only")
	if !errors.Is(executeError, expectedError) {
		t.Fatalf("error=%v want=%v", executeError, expectedError)
	}
}

func TestRootCommandRejectsInvalidPublicCapabilityConfig(t *testing.T) {
	testCases := []struct {
		name          string
		config        func(*testing.T, string) string
		expectedError string
	}{
		{
			name: "missing file",
			config: func(subTest *testing.T, tempDir string) string {
				return filepath.Join(tempDir, "missing.yml")
			},
			expectedError: "missing.yml",
		},
		{
			name: "malformed yaml",
			config: func(subTest *testing.T, tempDir string) string {
				return writeTestConfig(subTest, tempDir, `server: [`)
			},
			expectedError: "did not find expected node content",
		},
		{
			name: "missing server",
			config: func(subTest *testing.T, tempDir string) string {
				return writeTestConfig(subTest, tempDir, completeLiteralRuntimeYAML())
			},
			expectedError: "field=server",
		},
		{
			name: "missing providers",
			config: func(subTest *testing.T, tempDir string) string {
				return writeTestConfig(subTest, tempDir, `server:
  port: 9191
`+canonicalModelCatalogYAML())
			},
			expectedError: "field=providers",
		},
		{
			name: "missing catalog",
			config: func(subTest *testing.T, tempDir string) string {
				providersYAML, _, catalogFound := strings.Cut(completeLiteralRuntimeYAML(), "\ncatalog:")
				if !catalogFound {
					subTest.Fatal("canonical catalog fixture is missing")
				}
				return writeTestConfig(subTest, tempDir, "server:\n  port: 9191\n"+providersYAML)
			},
			expectedError: "field=catalog",
		},
		{
			name: "unknown server field",
			config: func(subTest *testing.T, tempDir string) string {
				return writeTestConfig(subTest, tempDir, `server:
  port: 9191
  future_option: true
`+completeLiteralRuntimeYAML())
			},
			expectedError: "field=server",
		},
		{
			name: "nonpositive port",
			config: func(subTest *testing.T, tempDir string) string {
				return writeTestConfig(subTest, tempDir, `server:
  port: 0
`+completeLiteralRuntimeYAML())
			},
			expectedError: "server.port must be positive",
		},
		{
			name: "unknown provider field",
			config: func(subTest *testing.T, tempDir string) string {
				providerYAML := strings.Replace(completeLiteralRuntimeYAML(), "\n  deepseek:", "\n    future_option: true\n  deepseek:", 1)
				return writeTestConfig(subTest, tempDir, "server:\n  port: 9191\n"+providerYAML)
			},
			expectedError: "field=providers",
		},
		{
			name: "unknown catalog field",
			config: func(subTest *testing.T, tempDir string) string {
				providerYAML := strings.Replace(completeLiteralRuntimeYAML(), "\n  publishers:", "\n  future_option: true\n  publishers:", 1)
				return writeTestConfig(subTest, tempDir, "server:\n  port: 9191\n"+providerYAML)
			},
			expectedError: "field=catalog",
		},
		{
			name: "retired qwen cloud provider",
			config: func(subTest *testing.T, tempDir string) string {
				providerYAML := strings.Replace(completeLiteralRuntimeYAML(), "\n  moonshot:", `
  qwencloud:
    base_url: "https://retired.example.invalid/v1"
    text:
      default_model: "qwen3.8-max-preview"
      models:
        - id: "qwen3.8-max-preview"
          wire_contract: "openai_chat_completions"
          execution_lifecycle: "synchronous_completion"
  moonshot:`, 1)
				return writeTestConfig(subTest, tempDir, "server:\n  port: 9191\n"+providerYAML)
			},
			expectedError: "field=providers",
		},
		{
			name: "retired zhipu provider",
			config: func(subTest *testing.T, tempDir string) string {
				providerYAML := strings.Replace(completeLiteralRuntimeYAML(), "\n  gemini:", `
  zhipu:
    base_url: "https://open.bigmodel.cn/api/paas/v4"
    transcriptions_url: "https://api.z.ai/api/paas/v4/audio/transcriptions"
  gemini:`, 1)
				return writeTestConfig(subTest, tempDir, "server:\n  port: 9191\n"+providerYAML)
			},
			expectedError: "field=providers",
		},
		{
			name: "nonpositive timeout",
			config: func(subTest *testing.T, tempDir string) string {
				return writeTestConfig(subTest, tempDir, `server:
  port: 9191
  max_request_timeout_seconds: 0
`+completeLiteralRuntimeYAML())
			},
			expectedError: "server.max_request_timeout_seconds must be positive",
		},
		{
			name: "invalid provider catalog",
			config: func(subTest *testing.T, tempDir string) string {
				providerYAML := strings.Replace(completeLiteralRuntimeYAML(), "  - provider: openai\n    model: gpt-4o-mini\n", "  - provider: openai\n    model: missing-model\n", 1)
				return writeTestConfig(subTest, tempDir, `server:
  port: 9191
`+providerYAML)
			},
			expectedError: "invalid_model_catalog",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			tempDir := subTest.TempDir()
			configPath := testCase.config(subTest, tempDir)
			withServeProxy(subTest, failingServeProxy(subTest))
			withServePublicCapabilityAPI(subTest, func(catalog proxy.PublicCapabilityCatalog, port int, logLevel string) error {
				subTest.Fatal("public capability API must not start")
				return errors.New("unexpected public capability API serve")
			})

			executeError := executeRootCommand(subTest, "--config", configPath, "--public-capabilities-only")
			if executeError == nil || !strings.Contains(executeError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want contains %q", executeError, testCase.expectedError)
			}
		})
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
providers:
  openai:
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
	configPath := writeTestConfig(t, tempDir, `server: [`)
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
providers:
  openai:
`)
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "config_file_parse_failed") {
		t.Fatalf("error=%v want config parse failure", executeError)
	}
}

func TestRootCommandRejectsObsoleteTenantConfiguration(t *testing.T) {
	for _, obsoleteYAML := range []string{
		"management:\n  enabled: true\n",
		"tenants:\n  - id: default\n    secret: client-secret\n",
		"providers:\n  openai:\n    api_key: provider-secret\n",
	} {
		t.Run(strings.SplitN(obsoleteYAML, ":", 2)[0], func(subTest *testing.T) {
			configPath := writeTestConfig(subTest, subTest.TempDir(), obsoleteYAML)
			withServeProxy(subTest, failingServeProxy(subTest))

			executeError := executeRootCommand(subTest, "--config", configPath)
			if executeError == nil || !strings.Contains(executeError.Error(), "config_file_parse_failed") {
				subTest.Fatalf("error=%v want obsolete configuration rejection", executeError)
			}
		})
	}
}

func TestRootCommandRejectsStaleProviderLevelReasoningEffortDeclaration(t *testing.T) {
	tempDir := t.TempDir()
	providersYAML := strings.Replace(completeLiteralRuntimeYAML(), `    base_url: "https://api.openai.com/v1"`, `    base_url: "https://api.openai.com/v1"
    reasoning_effort: high`, 1)
	configPath := writeTestConfig(t, tempDir, `
`+providersYAML)
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "config_file_parse_failed") || !strings.Contains(executeError.Error(), "reasoning_effort") {
		t.Fatalf("error=%v want stale provider-level reasoning effort rejection", executeError)
	}
}

func TestRootCommandRejectsInvalidUpstreamRateLimitConfiguration(t *testing.T) {
	testCases := []struct {
		name               string
		rateLimitRulesYAML string
		expectedField      string
	}{
		{
			name: "origin contains path",
			rateLimitRulesYAML: `
    - origin: "https://api.openai.com/v1"
      max_requests: 10
      interval: "1m"`,
			expectedField: "field=origin",
		},
		{
			name: "origin contains user info",
			rateLimitRulesYAML: `
    - origin: "https://credential@api.openai.com"
      max_requests: 10
      interval: "1m"`,
			expectedField: "field=origin",
		},
		{
			name: "maximum is not positive",
			rateLimitRulesYAML: `
    - origin: "https://api.openai.com"
      max_requests: 0
      interval: "1m"`,
			expectedField: "field=max_requests",
		},
		{
			name: "interval is invalid",
			rateLimitRulesYAML: `
    - origin: "https://api.openai.com"
      max_requests: 10
      interval: "not-a-duration"`,
			expectedField: "field=interval",
		},
		{
			name: "interval is not positive",
			rateLimitRulesYAML: `
    - origin: "https://api.openai.com"
      max_requests: 10
      interval: "0s"`,
			expectedField: "field=interval",
		},
		{
			name: "normalized origin is duplicated",
			rateLimitRulesYAML: `
    - origin: "https://api.openai.com"
      max_requests: 10
      interval: "1m"
    - origin: " HTTPS://API.OPENAI.COM "
      max_requests: 20
      interval: "1m"`,
			expectedField: "field=origin duplicate=https://api.openai.com",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			tempDir := subTest.TempDir()
			configPath := writeTestConfig(subTest, tempDir, `
server:
  upstream_rate_limits:`+testCase.rateLimitRulesYAML+`
`+completeLiteralRuntimeYAML())
			withServeProxy(subTest, failingServeProxy(subTest))

			executeError := executeRootCommand(subTest, "--config", configPath)
			if executeError == nil || !strings.Contains(executeError.Error(), "invalid_upstream_rate_limit_configuration") || !strings.Contains(executeError.Error(), testCase.expectedField) {
				subTest.Fatalf("error=%v want invalid upstream rate limit field %s", executeError, testCase.expectedField)
			}
			if strings.Contains(executeError.Error(), "credential@") {
				subTest.Fatalf("error leaked origin user info: %v", executeError)
			}
		})
	}
}

func TestRootCommandRejectsIncompleteProviderConfig(t *testing.T) {
	testCases := []struct {
		name          string
		providersYAML string
		expectedError string
	}{
		{
			name: "missing provider base url",
			providersYAML: `
providers:
  openai:
    base_url: "https://api.openai.com/v1"
    transcriptions_url: "https://api.openai.com/v1/audio/transcriptions"
  deepseek:
    base_url: "https://api.deepseek.com"
  dashscope:
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
  moonshot:
    base_url: "https://api.moonshot.ai/v1"
  siliconflow:
    base_url: "https://api.siliconflow.com/v1"
    transcriptions_url: "https://api.siliconflow.com/v1/audio/transcriptions"
  zai:
    base_url: "https://api.z.ai/api/paas/v4"
  gemini:
    base_url: "https://generativelanguage.googleapis.com/v1beta"
  anthropic:
    base_url: "https://api.anthropic.com"
  meta:
    base_url: "https://api.meta.ai/v1"
  xai:
    base_url: ""
    transcriptions_url: "https://api.x.ai/v1/stt"
`,
			expectedError: "provider_base_url_required: provider=xai field=providers.xai.base_url",
		},
		{
			name: "missing openai transcriptions url",
			providersYAML: `
providers:
  openai:
    base_url: "https://api.openai.com/v1"
    transcriptions_url: ""
  deepseek:
    base_url: "https://api.deepseek.com"
  dashscope:
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
  moonshot:
    base_url: "https://api.moonshot.ai/v1"
  siliconflow:
    base_url: "https://api.siliconflow.com/v1"
    transcriptions_url: "https://api.siliconflow.com/v1/audio/transcriptions"
  zai:
    base_url: "https://api.z.ai/api/paas/v4"
    transcriptions_url: "https://api.z.ai/api/paas/v4/audio/transcriptions"
  gemini:
    base_url: "https://generativelanguage.googleapis.com/v1beta"
  anthropic:
    base_url: "https://api.anthropic.com"
  meta:
    base_url: "https://api.meta.ai/v1"
  xai:
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
    base_url: "https://api.openai.com/v1"
    transcriptions_url: "https://api.openai.com/v1/audio/transcriptions"
  deepseek:
    base_url: "https://api.deepseek.com"
  dashscope:
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
  moonshot:
    base_url: "https://api.moonshot.ai/v1"
  siliconflow:
    base_url: "https://api.siliconflow.com/v1"
    transcriptions_url: ""
  zai:
    base_url: "https://api.z.ai/api/paas/v4"
    transcriptions_url: "https://api.z.ai/api/paas/v4/audio/transcriptions"
  gemini:
    base_url: "https://generativelanguage.googleapis.com/v1beta"
  anthropic:
    base_url: "https://api.anthropic.com"
  meta:
    base_url: "https://api.meta.ai/v1"
  xai:
    base_url: "https://api.x.ai/v1"
    transcriptions_url: "https://api.x.ai/v1/stt"
`,
			expectedError: "provider_transcriptions_url_required: provider=siliconflow field=providers.siliconflow.transcriptions_url",
		},
		{
			name: "missing zai transcriptions url",
			providersYAML: `
providers:
  openai:
    base_url: "https://api.openai.com/v1"
    transcriptions_url: "https://api.openai.com/v1/audio/transcriptions"
  deepseek:
    base_url: "https://api.deepseek.com"
  dashscope:
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
  moonshot:
    base_url: "https://api.moonshot.ai/v1"
  siliconflow:
    base_url: "https://api.siliconflow.com/v1"
    transcriptions_url: "https://api.siliconflow.com/v1/audio/transcriptions"
  zai:
    base_url: "https://api.z.ai/api/paas/v4"
    transcriptions_url: ""
  gemini:
    base_url: "https://generativelanguage.googleapis.com/v1beta"
  anthropic:
    base_url: "https://api.anthropic.com"
  meta:
    base_url: "https://api.meta.ai/v1"
  xai:
    base_url: "https://api.x.ai/v1"
    transcriptions_url: "https://api.x.ai/v1/stt"
`,
			expectedError: "provider_transcriptions_url_required: provider=zai field=providers.zai.transcriptions_url",
		},
		{
			name: "missing xai transcriptions url",
			providersYAML: `
providers:
  openai:
    base_url: "https://api.openai.com/v1"
    transcriptions_url: "https://api.openai.com/v1/audio/transcriptions"
  deepseek:
    base_url: "https://api.deepseek.com"
  dashscope:
    base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
  moonshot:
    base_url: "https://api.moonshot.ai/v1"
  siliconflow:
    base_url: "https://api.siliconflow.com/v1"
    transcriptions_url: "https://api.siliconflow.com/v1/audio/transcriptions"
  zai:
    base_url: "https://api.z.ai/api/paas/v4"
    transcriptions_url: "https://api.z.ai/api/paas/v4/audio/transcriptions"
  gemini:
    base_url: "https://generativelanguage.googleapis.com/v1beta"
  anthropic:
    base_url: "https://api.anthropic.com"
  meta:
    base_url: "https://api.meta.ai/v1"
  xai:
    base_url: "https://api.x.ai/v1"
    transcriptions_url: ""
`,
			expectedError: "provider_transcriptions_url_required: provider=xai field=providers.xai.transcriptions_url",
		},
		{
			name:          "missing provider text default",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameDashScope, proxy.ModelNameDashScopeQwenPlus, func(string) string { return "" }),
			expectedError: "provider=dashscope operation=text default_count=0",
		},
		{
			name:          "missing non-default provider text offering",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameDashScope, proxy.ModelNameDashScopeQwen37Max, func(string) string { return "" }),
			expectedError: "model=qwen3.7-max operation=text reason=missing_provider_offering",
		},
		{
			name:          "empty catalog providers",
			providersYAML: mutateCatalogSectionYAML(completeLiteralRuntimeYAML(), "providers", func(string) string { return "  providers: []" }),
			expectedError: "field=catalog.providers",
		},
		{
			name: "duplicate catalog provider",
			providersYAML: mutateCatalogRecordYAML(completeLiteralRuntimeYAML(), "providers", proxy.ProviderNameDeepSeek, func(block string) string {
				return strings.Replace(block, "  - id: deepseek", "  - id: openai", 1)
			}),
			expectedError: "duplicate_identifier=openai",
		},
		{
			name: "blank catalog provider label",
			providersYAML: mutateCatalogRecordYAML(completeLiteralRuntimeYAML(), "providers", proxy.ProviderNameOpenAI, func(block string) string {
				return strings.Replace(block, "    label: OpenAI", "    label: ''", 1)
			}),
			expectedError: "field=catalog.providers[0].label",
		},
		{
			name:          "empty catalog publishers",
			providersYAML: mutateCatalogSectionYAML(completeLiteralRuntimeYAML(), "publishers", func(string) string { return "  publishers: []" }),
			expectedError: "field=catalog.publishers",
		},
		{
			name: "noncanonical catalog publisher",
			providersYAML: mutateCatalogRecordYAML(completeLiteralRuntimeYAML(), "publishers", proxy.ProviderNameOpenAI, func(block string) string {
				return strings.Replace(block, "  - id: openai", "  - id: OpenAI", 1)
			}),
			expectedError: "field=catalog.publishers[0].id",
		},
		{
			name: "duplicate catalog publisher",
			providersYAML: mutateCatalogRecordYAML(completeLiteralRuntimeYAML(), "publishers", proxy.ProviderNameDeepSeek, func(block string) string {
				return strings.Replace(block, "  - id: deepseek", "  - id: openai", 1)
			}),
			expectedError: "duplicate_identifier=openai",
		},
		{
			name: "blank catalog publisher label",
			providersYAML: mutateCatalogRecordYAML(completeLiteralRuntimeYAML(), "publishers", proxy.ProviderNameOpenAI, func(block string) string {
				return strings.Replace(block, "    label: OpenAI", "    label: ''", 1)
			}),
			expectedError: "field=catalog.publishers[0].label",
		},
		{
			name:          "empty catalog families",
			providersYAML: mutateCatalogSectionYAML(completeLiteralRuntimeYAML(), "families", func(string) string { return "  families: []" }),
			expectedError: "field=catalog.families",
		},
		{
			name: "noncanonical catalog family",
			providersYAML: mutateCatalogRecordYAML(completeLiteralRuntimeYAML(), "families", "gpt-4", func(block string) string {
				return strings.Replace(block, "  - id: gpt-4", "  - id: GPT-4", 1)
			}),
			expectedError: "field=catalog.families[0].id",
		},
		{
			name: "duplicate catalog family",
			providersYAML: mutateCatalogRecordYAML(completeLiteralRuntimeYAML(), "families", "gpt-5", func(block string) string {
				return strings.Replace(block, "  - id: gpt-5", "  - id: gpt-4", 1)
			}),
			expectedError: "duplicate_identifier=gpt-4",
		},
		{
			name: "catalog family dangling publisher",
			providersYAML: mutateCatalogRecordYAML(completeLiteralRuntimeYAML(), "families", "gpt-4", func(block string) string {
				return strings.Replace(block, "    publisher: openai", "    publisher: missing", 1)
			}),
			expectedError: "publisher=missing reason=dangling_reference",
		},
		{
			name: "blank catalog family label",
			providersYAML: mutateCatalogRecordYAML(completeLiteralRuntimeYAML(), "families", "gpt-4", func(block string) string {
				return strings.Replace(block, "    label: GPT-4", "    label: ''", 1)
			}),
			expectedError: "field=catalog.families[0].label",
		},
		{
			name: "unknown catalog family weight access",
			providersYAML: mutateCatalogRecordYAML(completeLiteralRuntimeYAML(), "families", "gpt-4", func(block string) string {
				return strings.Replace(block, "    weight_access: proprietary", "    weight_access: unknown", 1)
			}),
			expectedError: "field=catalog.families[0].weight_access value=unknown",
		},
		{
			name:          "empty exact models",
			providersYAML: mutateCatalogSectionYAML(completeLiteralRuntimeYAML(), "models", func(string) string { return "  models: []" }),
			expectedError: "field=catalog.models",
		},
		{
			name: "exact model dangling publisher",
			providersYAML: mutateExactModelYAML(completeLiteralRuntimeYAML(), proxy.ModelNameGPT4oMini, func(block string) string {
				return strings.Replace(block, "    publisher: openai", "    publisher: missing", 1)
			}),
			expectedError: "publisher=missing reason=dangling_reference",
		},
		{
			name: "exact model family publisher mismatch",
			providersYAML: mutateExactModelYAML(completeLiteralRuntimeYAML(), proxy.ModelNameGPT4oMini, func(block string) string {
				return strings.Replace(block, "    family: gpt-4", "    family: qwen", 1)
			}),
			expectedError: "family=qwen publisher=openai reason=dangling_reference",
		},
		{
			name: "blank exact model version",
			providersYAML: mutateExactModelYAML(completeLiteralRuntimeYAML(), proxy.ModelNameGPT4oMini, func(block string) string {
				return strings.Replace(block, "    version: gpt-4o-mini", "    version: ''", 1)
			}),
			expectedError: "field=catalog.models[0].version",
		},
		{
			name: "duplicate exact model operation",
			providersYAML: mutateExactModelYAML(completeLiteralRuntimeYAML(), proxy.ModelNameGPT4oMini, func(block string) string {
				return strings.Replace(block, "    - text", "    - text\n    - text", 1)
			}),
			expectedError: "duplicate=text",
		},
		{
			name:          "empty provider offerings",
			providersYAML: mutateCatalogSectionYAML(completeLiteralRuntimeYAML(), "offerings", func(string) string { return "  offerings: []" }),
			expectedError: "field=catalog.offerings",
		},
		{
			name: "offering dangling provider",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.ModelNameGPT4oMini, func(block string) string {
				return strings.Replace(block, "  - provider: openai", "  - provider: missing", 1)
			}),
			expectedError: "provider=missing reason=dangling_reference",
		},
		{
			name: "blank provider native model",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.ModelNameGPT4oMini, func(block string) string {
				return strings.Replace(block, "    provider_model: gpt-4o-mini", "    provider_model: ''", 1)
			}),
			expectedError: "field=catalog.offerings[0].provider_model",
		},
		{
			name: "duplicate provider route",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.ModelNameGPT4oMini, func(block string) string {
				return block + block
			}),
			expectedError: "route_conflict=openai:gpt-4o-mini",
		},
		{
			name: "duplicate provider native model",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.ModelNameGPT4o, func(block string) string {
				return strings.Replace(block, "    provider_model: gpt-4o", "    provider_model: gpt-4o-mini", 1)
			}),
			expectedError: "provider_native_model_conflict=gpt-4o-mini",
		},
		{
			name: "invalid provider offering operation",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.ModelNameGPT4oMini, func(block string) string {
				return strings.Replace(block, "    - text", "    - image", 1)
			}),
			expectedError: "operation=image",
		},
		{
			name: "provider operation unsupported by exact model",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.ModelNameGPT4oMini, func(block string) string {
				return strings.Replace(block, "    - text", "    - dictation", 1)
			}),
			expectedError: "operation=dictation reason=unsupported_by_model",
		},
		{
			name: "provider default operation unsupported by offering",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.ModelNameGPT4oMini, func(block string) string {
				return strings.Replace(block, "    wire_contract:", "    default_operations:\n    - dictation\n    wire_contract:", 1)
			}),
			expectedError: "default_operations operation=dictation reason=unsupported_by_offering",
		},
		{
			name: "offering media unsupported by exact model",
			providersYAML: mutateExactModelYAML(completeLiteralRuntimeYAML(), proxy.ModelNameGemini35Flash, func(block string) string {
				return strings.Replace(block, "    - image\n", "", 1)
			}),
			expectedError: "media_input=image reason=unsupported_by_model",
		},
		{
			name: "duplicate provider offering media input",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameGemini, proxy.ModelNameGemini35Flash, func(block string) string {
				return strings.Replace(block, "    - image\n", "    - image\n    - image\n", 1)
			}),
			expectedError: "duplicate=image",
		},
		{
			name:          "missing meta base url",
			providersYAML: strings.Replace(completeLiteralRuntimeYAML(), "base_url: \"https://api.meta.ai/v1\"", "base_url: \"\"", 1),
			expectedError: "provider_base_url_required: provider=meta field=providers.meta.base_url",
		},
		{
			name: "blank provider text default model",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.ModelNameGPT41, func(block string) string {
				return strings.Replace(block, "    default_operations:\n    - text\n", "", 1)
			}),
			expectedError: "provider=openai operation=text default_count=0",
		},
		{
			name: "blank keyed gemini text default model",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameGemini, proxy.ModelNameGemini25Flash, func(block string) string {
				return strings.Replace(block, "    default_operations:\n    - text\n", "", 1)
			}),
			expectedError: "provider=gemini operation=text default_count=0",
		},
		{
			name: "blank provider dictation default model",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.DefaultDictationModel, func(block string) string {
				return strings.Replace(block, "    default_operations:\n    - dictation", "    default_operations:\n    - ''", 1)
			}),
			expectedError: "default_operations",
		},
		{
			name:          "blank provider model id",
			providersYAML: mutateExactModelYAML(completeLiteralRuntimeYAML(), proxy.ModelNameGPT4oMini, func(block string) string { return strings.Replace(block, "  - id: gpt-4o-mini", "  - id: ''", 1) }),
			expectedError: "field=catalog.models[0].id",
		},
		{
			name:          "duplicate provider model id",
			providersYAML: mutateExactModelYAML(completeLiteralRuntimeYAML(), proxy.ModelNameGPT4o, func(block string) string { return strings.Replace(block, "  - id: gpt-4o", "  - id: gpt-4o-mini", 1) }),
			expectedError: "duplicate_identifier=gpt-4o-mini",
		},
		{
			name: "default provider model missing from catalog",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.ModelNameGPT41, func(block string) string {
				return strings.Replace(block, "    model: gpt-4.1", "    model: gpt-not-configured", 1)
			}),
			expectedError: "model=gpt-not-configured reason=dangling_reference",
		},
		{
			name:          "negative provider output token limit",
			providersYAML: strings.Replace(completeLiteralRuntimeYAML(), "output_token_limit: 65536", "output_token_limit: -1", 1),
			expectedError: "output_token_limit",
		},
		{
			name: "anthropic output token limit required",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameAnthropic, proxy.ModelNameClaudeSonnet46, func(block string) string {
				return strings.Replace(block, "    output_token_limit: 64000", "    output_token_limit: 0", 1)
			}),
			expectedError: "output_token_limit provider=anthropic",
		},
		{
			name: "missing text wire contract",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.ModelNameGPT4oMini, func(block string) string {
				return strings.Replace(block, "    wire_contract: openai_responses\n", "", 1)
			}),
			expectedError: "field=catalog.offerings[0].wire_contract",
		},
		{
			name: "missing text execution lifecycle",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.ModelNameGPT4oMini, func(block string) string {
				return strings.Replace(block, "    execution_lifecycle: pollable_resource\n", "", 1)
			}),
			expectedError: "field=catalog.offerings[0].execution_lifecycle",
		},
		{
			name:          "unknown text wire contract",
			providersYAML: strings.Replace(completeLiteralRuntimeYAML(), "wire_contract: openai_responses", "wire_contract: future_responses", 1),
			expectedError: "wire_contract=future_responses",
		},
		{
			name:          "unknown text execution lifecycle",
			providersYAML: strings.Replace(completeLiteralRuntimeYAML(), "execution_lifecycle: pollable_resource", "execution_lifecycle: deferred_once", 1),
			expectedError: "execution_lifecycle=deferred_once",
		},
		{
			name:          "contradictory openai lifecycle",
			providersYAML: strings.Replace(completeLiteralRuntimeYAML(), "execution_lifecycle: pollable_resource", "execution_lifecycle: synchronous_completion", 1),
			expectedError: "provider=openai model=gpt-4o-mini wire_contract=openai_responses execution_lifecycle=synchronous_completion",
		},
		{
			name:          "provider incompatible wire contract",
			providersYAML: strings.Replace(completeLiteralRuntimeYAML(), "wire_contract: openai_chat_completions", "wire_contract: anthropic_messages", 1),
			expectedError: "provider=deepseek",
		},
		{
			name: "dictation wire contract",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.DefaultDictationModel, func(block string) string {
				return strings.Replace(block, "wire_contract: multipart_transcription", "wire_contract: openai_responses", 1)
			}),
			expectedError: "unsupported_dictation_route",
		},
		{
			name: "dictation execution lifecycle",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.DefaultDictationModel, func(block string) string {
				return strings.Replace(block, "execution_lifecycle: synchronous_completion", "execution_lifecycle: pollable_resource", 1)
			}),
			expectedError: "unsupported_dictation_route",
		},
		{
			name:          "blank openai request profile",
			providersYAML: strings.Replace(completeLiteralRuntimeYAML(), "request_profile: openai_responses_temperature", "request_profile: ''", 1),
			expectedError: "field=catalog.offerings[0].request_profile",
		},
		{
			name:          "invalid openai request profile",
			providersYAML: strings.Replace(completeLiteralRuntimeYAML(), "request_profile: openai_responses_temperature", "request_profile: future_profile", 1),
			expectedError: "field=catalog.offerings[0].request_profile",
		},
		{
			name:          "retired openai base request profile",
			providersYAML: strings.Replace(completeLiteralRuntimeYAML(), "request_profile: openai_responses_temperature", "request_profile: openai_responses_base", 1),
			expectedError: "field=catalog.offerings[0].request_profile",
		},
		{
			name: "non openai request profile",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameDeepSeek, proxy.ModelNameDeepSeekV4Flash, func(block string) string {
				return strings.Replace(block, "    wire_contract:", "    request_profile: openai_responses_temperature\n    wire_contract:", 1)
			}),
			expectedError: "provider=deepseek profile=openai_responses_temperature",
		},
		{
			name: "non openai web search",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameDeepSeek, proxy.ModelNameDeepSeekV4Flash, func(block string) string {
				return strings.Replace(block, "    wire_contract:", "    web_search: true\n    wire_contract:", 1)
			}),
			expectedError: "web_search provider=deepseek",
		},
		{
			name: "dictation web search",
			providersYAML: mutateCatalogOfferingYAML(completeLiteralRuntimeYAML(), proxy.ProviderNameOpenAI, proxy.DefaultDictationModel, func(block string) string {
				return strings.Replace(block, "    operations:\n", "    web_search: true\n    operations:\n", 1)
			}),
			expectedError: "text_capabilities_on_dictation_route",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			tempDir := subTest.TempDir()
			providersYAML := withCurrentProviderBaseURLFixtures(testCase.providersYAML)
			if !strings.Contains(providersYAML, "\ncatalog:") {
				providersYAML += canonicalModelCatalogYAML()
			}
			configPath := writeTestConfig(subTest, tempDir, `
`+providersYAML)
			withServeProxy(subTest, failingServeProxy(subTest))

			executeError := executeRootCommand(subTest, "--config", configPath)
			if executeError == nil || !strings.Contains(executeError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want contains %q", executeError, testCase.expectedError)
			}
		})
	}
}

func withCurrentProviderBaseURLFixtures(providersYAML string) string {
	if !strings.Contains(providersYAML, "\nmanagement:") {
		providersYAML = completeManagementYAML() + providersYAML
	}
	if !strings.Contains(providersYAML, "\n  minimax:") {
		providersYAML = strings.Replace(providersYAML, "\n  siliconflow:", `
  minimax:
    base_url: "https://api.minimax.io/v1"
  siliconflow:`, 1)
	}
	return providersYAML
}

func executeRootCommand(t *testing.T, arguments ...string) error {
	t.Helper()
	rootCmd.SetArgs(arguments)
	_, executeError := rootCmd.ExecuteC()
	rootCmd.SetArgs(nil)
	resetConfigFlag(t)
	runtimeConfiguration = proxy.Configuration{}
	publicCapabilityConfiguration = publicCapabilityAPIConfiguration{}
	return executeError
}

func resetConfigFlag(t *testing.T) {
	t.Helper()
	resetStringFlag(t, flagConfig, defaultConfigPath)
	resetBoolFlag(t, flagPublicCapabilitiesOnly, false)
}

func resetStringFlag(t *testing.T, flagName string, flagValue string) {
	t.Helper()
	commandFlags := rootCmd.Flags()
	if flagError := commandFlags.Set(flagName, flagValue); flagError != nil {
		t.Fatalf("reset %s flag: %v", flagName, flagError)
	}
	commandFlags.Lookup(flagName).Changed = false
}

func resetBoolFlag(t *testing.T, flagName string, flagValue bool) {
	t.Helper()
	commandFlags := rootCmd.Flags()
	if flagError := commandFlags.Set(flagName, fmt.Sprintf("%t", flagValue)); flagError != nil {
		t.Fatalf("reset %s flag: %v", flagName, flagError)
	}
	commandFlags.Lookup(flagName).Changed = false
}

func withServeProxy(t *testing.T, replacement func(proxy.Configuration, *zap.SugaredLogger) error) {
	t.Helper()
	originalServeProxy := serveProxy
	t.Cleanup(func() {
		serveProxy = originalServeProxy
		rootCmd.SetArgs(nil)
		resetConfigFlag(t)
		runtimeConfiguration = proxy.Configuration{}
		publicCapabilityConfiguration = publicCapabilityAPIConfiguration{}
	})
	serveProxy = replacement
}

func withServePublicCapabilityAPI(t *testing.T, replacement func(proxy.PublicCapabilityCatalog, int, string) error) {
	t.Helper()
	originalServePublicCapabilityAPI := servePublicCapabilityAPI
	t.Cleanup(func() {
		servePublicCapabilityAPI = originalServePublicCapabilityAPI
		rootCmd.SetArgs(nil)
		resetConfigFlag(t)
		publicCapabilityConfiguration = publicCapabilityAPIConfiguration{}
	})
	servePublicCapabilityAPI = replacement
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

func configuredProviderOfferings(catalog proxy.ModelCatalog, provider string) []proxy.ProviderOffering {
	offerings := []proxy.ProviderOffering{}
	for _, offering := range catalog.Offerings {
		if offering.Provider == provider {
			offerings = append(offerings, offering)
		}
	}
	return offerings
}

func configuredDefaultOffering(catalog proxy.ModelCatalog, provider string, operation string) (proxy.ProviderOffering, bool) {
	for _, offering := range catalog.Offerings {
		if offering.Provider == provider && slices.Contains(offering.DefaultOperations, operation) {
			return offering, true
		}
	}
	return proxy.ProviderOffering{}, false
}

func completeLiteralRuntimeYAML() string {
	return completeManagementYAML() + completeProvidersYAML(defaultProviderYAMLValues())
}

func completeManagementYAML() string {
	return `
management:
  public_origin: "https://llm-proxy.example"
  ui_description: "LLM Proxy"
  ui_origins:
    - "https://llm-proxy.example"
  admin_emails: []
  tauth_url: "https://tauth.example"
  tauth_tenant_id: "llm-proxy"
  google_client_id: "google-client-id"
  login_path: "/auth/google"
  logout_path: "/auth/logout"
  nonce_path: "/auth/nonce"
  session_path: "/auth/session"
  jwt_signing_key: "signing-key"
  jwt_issuer: "tauth"
  session_cookie_name: "llm_proxy_session"
  database_path: "/tmp/llm-proxy-test.sqlite"
  provider_key_encryption_key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
  management_api_origin: "https://llm-proxy-api.example"
  proxy_origin: "https://llm-proxy-api.example"
`
}

type providerYAMLValues struct {
	OpenAIBaseURL                string
	OpenAITranscriptionsURL      string
	DeepSeekBaseURL              string
	DashScopeBaseURL             string
	MoonshotBaseURL              string
	MiniMaxBaseURL               string
	SiliconFlowBaseURL           string
	SiliconFlowTranscriptionsURL string
	ZAIBaseURL                   string
	ZAITranscriptionsURL         string
	GeminiBaseURL                string
	AnthropicBaseURL             string
	MetaBaseURL                  string
	XAIBaseURL                   string
	XAITranscriptionsURL         string
}

func defaultProviderYAMLValues() providerYAMLValues {
	return providerYAMLValues{
		OpenAIBaseURL:                "https://api.openai.com/v1",
		OpenAITranscriptionsURL:      "https://api.openai.com/v1/audio/transcriptions",
		DeepSeekBaseURL:              "https://api.deepseek.com",
		DashScopeBaseURL:             "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		MoonshotBaseURL:              "https://api.moonshot.ai/v1",
		MiniMaxBaseURL:               "https://api.minimax.io/v1",
		SiliconFlowBaseURL:           "https://api.siliconflow.com/v1",
		SiliconFlowTranscriptionsURL: "https://api.siliconflow.com/v1/audio/transcriptions",
		ZAIBaseURL:                   "https://api.z.ai/api/paas/v4",
		ZAITranscriptionsURL:         "https://api.z.ai/api/paas/v4/audio/transcriptions",
		GeminiBaseURL:                "https://generativelanguage.googleapis.com/v1beta",
		AnthropicBaseURL:             "https://api.anthropic.com",
		MetaBaseURL:                  "https://api.meta.ai/v1",
		XAIBaseURL:                   "https://api.x.ai/v1",
		XAITranscriptionsURL:         "https://api.x.ai/v1/stt",
	}
}

func completeProvidersYAML(values providerYAMLValues) string {
	providersYAML := fmt.Sprintf(`
providers:
  openai:
    base_url: "%s"
    transcriptions_url: "%s"
  deepseek:
    base_url: "%s"
  dashscope:
    base_url: "%s"
  moonshot:
    base_url: "%s"
  minimax:
    base_url: "%s"
  siliconflow:
    base_url: "%s"
    transcriptions_url: "%s"
  zai:
    base_url: "%s"
    transcriptions_url: "%s"
  gemini:
    base_url: "%s"
  anthropic:
    base_url: "%s"
  meta:
    base_url: "%s"
  xai:
    base_url: "%s"
    transcriptions_url: "%s"
`,
		values.OpenAIBaseURL,
		values.OpenAITranscriptionsURL,
		values.DeepSeekBaseURL,
		values.DashScopeBaseURL,
		values.MoonshotBaseURL,
		values.MiniMaxBaseURL,
		values.SiliconFlowBaseURL,
		values.SiliconFlowTranscriptionsURL,
		values.ZAIBaseURL,
		values.ZAITranscriptionsURL,
		values.GeminiBaseURL,
		values.AnthropicBaseURL,
		values.MetaBaseURL,
		values.XAIBaseURL,
		values.XAITranscriptionsURL,
	)
	return providersYAML + canonicalModelCatalogYAML()
}

func canonicalModelCatalogYAML() string {
	_, currentFile, _, callerOK := runtime.Caller(0)
	if !callerOK {
		panic("locate CLI test fixture")
	}
	configBytes, readError := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "..", "..", "configs", "config.yml"))
	if readError != nil {
		panic(fmt.Sprintf("read canonical model catalog: %v", readError))
	}
	_, catalogYAML, catalogFound := strings.Cut(string(configBytes), "\ncatalog:\n")
	if !catalogFound {
		panic("canonical model catalog is missing")
	}
	return "\ncatalog:\n" + catalogYAML
}

func mutateCatalogOfferingYAML(configurationYAML string, provider string, model string, mutate func(string) string) string {
	marker := "\n  - provider: " + provider + "\n    model: " + model + "\n"
	start := strings.Index(configurationYAML, marker)
	if start < 0 {
		panic("catalog offering fixture is missing: " + provider + "/" + model)
	}
	end := strings.Index(configurationYAML[start+len(marker):], "\n  - provider:")
	if end < 0 {
		end = len(configurationYAML)
	} else {
		end += start + len(marker)
	}
	return configurationYAML[:start] + mutate(configurationYAML[start:end]) + configurationYAML[end:]
}

func mutateExactModelYAML(configurationYAML string, model string, mutate func(string) string) string {
	marker := "\n  - id: " + model + "\n    publisher:"
	start := strings.Index(configurationYAML, marker)
	if start < 0 {
		panic("exact model fixture is missing: " + model)
	}
	end := strings.Index(configurationYAML[start+len(marker):], "\n  - id:")
	if end < 0 {
		end = strings.Index(configurationYAML[start+len(marker):], "\n  offerings:")
	}
	if end < 0 {
		end = len(configurationYAML)
	} else {
		end += start + len(marker)
	}
	return configurationYAML[:start] + mutate(configurationYAML[start:end]) + configurationYAML[end:]
}

func mutateCatalogSectionYAML(configurationYAML string, section string, mutate func(string) string) string {
	sectionOrder := []string{"providers", "publishers", "families", "models", "offerings"}
	sectionIndex := slices.Index(sectionOrder, section)
	if sectionIndex < 0 {
		panic("unknown catalog section fixture: " + section)
	}
	marker := "\n  " + section + ":"
	start := strings.Index(configurationYAML, marker)
	if start < 0 {
		panic("catalog section fixture is missing: " + section)
	}
	end := len(configurationYAML)
	if sectionIndex+1 < len(sectionOrder) {
		nextMarker := "\n  " + sectionOrder[sectionIndex+1] + ":"
		nextStart := strings.Index(configurationYAML[start+len(marker):], nextMarker)
		if nextStart < 0 {
			panic("next catalog section fixture is missing: " + sectionOrder[sectionIndex+1])
		}
		end = start + len(marker) + nextStart
	}
	return configurationYAML[:start+1] + mutate(configurationYAML[start+1:end]) + configurationYAML[end:]
}

func mutateCatalogRecordYAML(configurationYAML string, section string, identifier string, mutate func(string) string) string {
	return mutateCatalogSectionYAML(configurationYAML, section, func(sectionYAML string) string {
		marker := "\n  - id: " + identifier + "\n"
		start := strings.Index(sectionYAML, marker)
		if start < 0 {
			panic("catalog record fixture is missing: " + section + "/" + identifier)
		}
		end := strings.Index(sectionYAML[start+len(marker):], "\n  - id:")
		if end < 0 {
			end = len(sectionYAML)
		} else {
			end += start + len(marker)
		}
		return sectionYAML[:start] + mutate(sectionYAML[start:end]) + sectionYAML[end:]
	})
}
