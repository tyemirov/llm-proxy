package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
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
	providerValues.OpenAIAPIKey = ""
	providerValues.OpenAIBaseURL = "https://openai.example/v1"
	providerValues.OpenAITranscriptionsURL = "https://openai.example/v1/audio/transcriptions"
	providerValues.DeepSeekAPIKey = ""
	providerValues.DeepSeekBaseURL = "https://deepseek.example"
	providerValues.DashScopeAPIKey = ""
	providerValues.DashScopeBaseURL = "https://dashscope.example"
	providerValues.MoonshotAPIKey = ""
	providerValues.MoonshotBaseURL = "https://moonshot.example"
	providerValues.SiliconFlowAPIKey = ""
	providerValues.SiliconFlowBaseURL = "https://siliconflow.example"
	providerValues.SiliconFlowTranscriptionsURL = "https://siliconflow.example/audio/transcriptions"
	providerValues.ZhipuAPIKey = ""
	providerValues.ZhipuBaseURL = "https://zhipu.example"
	providerValues.ZhipuTranscriptionsURL = "https://zhipu.example/audio/transcriptions"
	providerValues.GeminiAPIKey = ""
	providerValues.GeminiBaseURL = "https://gemini.example"
	providerValues.AnthropicAPIKey = ""
	providerValues.AnthropicBaseURL = "https://anthropic.example"
	providerValues.MetaAPIKey = ""
	providerValues.MetaBaseURL = "https://meta.example/v1"
	providerValues.GrokAPIKey = ""
	providerValues.GrokBaseURL = "https://grok.example"
	providerValues.GrokTranscriptionsURL = "https://grok.example/stt"
	configPath := writeTestConfig(t, tempDir, `
server:
  port: 18080
  log_level: debug
  workers: 2
  queue_size: 9
  request_timeout_seconds: 7
  max_prompt_bytes: 1024
  max_input_audio_bytes: 2048
  upstream_rate_limits:
    - origin: "https://openai.example"
      max_requests: 12
      interval: "1m"
management:
  enabled: true
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
  jwt_signing_key: "${P411_TAUTH_JWT_SIGNING_KEY}"
  jwt_issuer: "tauth"
  session_cookie_name: "llm_proxy_session"
  database_dialect: "${P411_MANAGEMENT_DATABASE_DIALECT}"
  database_dsn: "${P411_MANAGEMENT_DATABASE_DSN}"
  provider_key_encryption_key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
  management_api_origin: "https://llm-proxy-api.example"
  proxy_origin: "https://llm-proxy-api.example"
  legacy_token_migration:
    tenant_id: legacy
    owner_email: "${P411_LEGACY_TOKEN_OWNER_EMAIL}"
`+completeProvidersYAML(providerValues))
	writeTestDotEnv(t, tempDir, `
P411_TAUTH_JWT_SIGNING_KEY=tauth-signing-key
P411_MANAGEMENT_DATABASE_DIALECT=sqlite
P411_MANAGEMENT_DATABASE_DSN=postgres://llm-proxy.example/management
P411_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
P411_LEGACY_TOKEN_OWNER_EMAIL=Legacy.Owner@Example.com
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
	if len(capturedConfiguration.Tenants) != 0 {
		t.Fatalf("management tenants=%+v", capturedConfiguration.Tenants)
	}
	if capturedConfiguration.OpenAIKey != "" {
		t.Fatalf("openAIKey=%q", capturedConfiguration.OpenAIKey)
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
	if len(capturedConfiguration.UpstreamRateLimits) != 1 || capturedConfiguration.UpstreamRateLimits[0].Origin != "https://openai.example" || capturedConfiguration.UpstreamRateLimits[0].MaxRequests != 12 || capturedConfiguration.UpstreamRateLimits[0].Interval != "1m" {
		t.Fatalf("upstreamRateLimits=%+v", capturedConfiguration.UpstreamRateLimits)
	}
	if !capturedConfiguration.Management.Enabled {
		t.Fatalf("management must be enabled")
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
	if capturedConfiguration.Management.LoginPath != "/auth/google" || capturedConfiguration.Management.LogoutPath != "/auth/logout" || capturedConfiguration.Management.NoncePath != "/auth/nonce" {
		t.Fatalf("management auth paths=%q %q %q", capturedConfiguration.Management.LoginPath, capturedConfiguration.Management.LogoutPath, capturedConfiguration.Management.NoncePath)
	}
	if capturedConfiguration.Management.JWTSigningKey != "tauth-signing-key" {
		t.Fatalf("management signing key=%q", capturedConfiguration.Management.JWTSigningKey)
	}
	if capturedConfiguration.Management.SessionCookieName != "llm_proxy_session" {
		t.Fatalf("management cookie name=%q", capturedConfiguration.Management.SessionCookieName)
	}
	if capturedConfiguration.Management.DatabaseDialect != proxy.ManagementDatabaseDialectSQLite {
		t.Fatalf("management database dialect=%q", capturedConfiguration.Management.DatabaseDialect)
	}
	if capturedConfiguration.Management.DatabaseDSN != "postgres://llm-proxy.example/management" {
		t.Fatalf("management database dsn=%q", capturedConfiguration.Management.DatabaseDSN)
	}
	if capturedConfiguration.Management.ProviderKeyEncryptionKey != "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" {
		t.Fatalf("management provider key encryption key=%q", capturedConfiguration.Management.ProviderKeyEncryptionKey)
	}
	if capturedConfiguration.Management.ManagementAPIOrigin != "https://llm-proxy-api.example" || capturedConfiguration.Management.ProxyOrigin != "https://llm-proxy-api.example" {
		t.Fatalf("management api origins=%q %q", capturedConfiguration.Management.ManagementAPIOrigin, capturedConfiguration.Management.ProxyOrigin)
	}
	if capturedConfiguration.Management.LegacyTokenMigration.TenantID != "legacy" || capturedConfiguration.Management.LegacyTokenMigration.OwnerEmail != "legacy.owner@example.com" {
		t.Fatalf("legacy migration=%+v", capturedConfiguration.Management.LegacyTokenMigration)
	}
	if capturedConfiguration.GeminiKey != "" {
		t.Fatalf("geminiKey=%q", capturedConfiguration.GeminiKey)
	}
	if capturedConfiguration.GeminiBaseURL != "https://gemini.example" {
		t.Fatalf("geminiBaseURL=%q", capturedConfiguration.GeminiBaseURL)
	}
	if capturedConfiguration.AnthropicKey != "" {
		t.Fatalf("anthropicKey=%q", capturedConfiguration.AnthropicKey)
	}
	if capturedConfiguration.AnthropicBaseURL != "https://anthropic.example" {
		t.Fatalf("anthropicBaseURL=%q", capturedConfiguration.AnthropicBaseURL)
	}
	if capturedConfiguration.MetaKey != "" || capturedConfiguration.MetaBaseURL != "https://meta.example/v1" {
		t.Fatalf("meta key/base URL=%q %q", capturedConfiguration.MetaKey, capturedConfiguration.MetaBaseURL)
	}
	if capturedConfiguration.ZhipuTranscriptionsURL != "https://zhipu.example/audio/transcriptions" {
		t.Fatalf("zhipuTranscriptionsURL=%q", capturedConfiguration.ZhipuTranscriptionsURL)
	}
	if capturedConfiguration.GrokKey != "" {
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

func TestRootCommandRejectsUnsupportedManagementDatabaseDialect(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
management:
  enabled: true
  public_origin: "https://llm-proxy.example"
  ui_description: "LLM Proxy"
  ui_origins:
    - "https://llm-proxy.example"
  tauth_url: "https://tauth.example"
  tauth_tenant_id: "llm-proxy"
  google_client_id: "google-client-id"
  login_path: "/auth/google"
  logout_path: "/auth/logout"
  nonce_path: "/auth/nonce"
  jwt_signing_key: "tauth-signing-key"
  jwt_issuer: "tauth"
  session_cookie_name: "llm_proxy_session"
  database_dialect: "mysql"
  database_dsn: "mysql://llm-proxy.example/management"
  provider_key_encryption_key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
  management_api_origin: "https://llm-proxy-api.example"
  proxy_origin: "https://llm-proxy-api.example"
`+completeLiteralProvidersYAML())
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "management.database_dialect") {
		t.Fatalf("error=%v want unsupported management database dialect", executeError)
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

func TestRootCommandRejectsPlaceholderDefaultSyntax(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
management:
  enabled: false
  database_dialect: "${P411_MISSING_MANAGEMENT_DATABASE_DIALECT:-sqlite}"
  database_dsn: "${P411_MISSING_MANAGEMENT_DATABASE_DSN:-management.sqlite}"
  provider_key_encryption_key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: openai
      model: gpt-4.1
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
`+completeLiteralProvidersYAML())
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "config_placeholder_missing: names=P411_MISSING_MANAGEMENT_DATABASE_DIALECT:-sqlite,P411_MISSING_MANAGEMENT_DATABASE_DSN:-management.sqlite") {
		t.Fatalf("error=%v want default placeholder syntax rejected", executeError)
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
LLM_PROXY_MANAGEMENT_ENABLED=true
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
LLM_PROXY_MANAGEMENT_JWT_SIGNING_KEY=packaged-tauth-signing-key
LLM_PROXY_MANAGEMENT_JWT_ISSUER=tauth
LLM_PROXY_MANAGEMENT_SESSION_COOKIE_NAME=app_session_llm_proxy
LLM_PROXY_MANAGEMENT_DATABASE_DIALECT=sqlite
LLM_PROXY_MANAGEMENT_DATABASE_DSN=llm-proxy-management.sqlite
LLM_PROXY_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
LLM_PROXY_MANAGEMENT_API_ORIGIN=https://llm-proxy-api.mprlab.com
LLM_PROXY_MANAGEMENT_PROXY_ORIGIN=https://llm-proxy-api.mprlab.com
LLM_PROXY_MANAGEMENT_LEGACY_TOKEN_OWNER_EMAIL=owner@example.invalid
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
	if !capturedConfiguration.Management.Enabled {
		t.Fatalf("packaged config should enable management from environment")
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
	if capturedConfiguration.Management.DatabaseDialect != proxy.ManagementDatabaseDialectSQLite {
		t.Fatalf("database dialect=%q", capturedConfiguration.Management.DatabaseDialect)
	}
	if capturedConfiguration.Management.DatabaseDSN != "llm-proxy-management.sqlite" {
		t.Fatalf("database dsn=%q", capturedConfiguration.Management.DatabaseDSN)
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
	if capturedConfiguration.Management.LegacyTokenMigration.TenantID != "default" || capturedConfiguration.Management.LegacyTokenMigration.OwnerEmail != "owner@example.invalid" {
		t.Fatalf("legacy migration=%+v", capturedConfiguration.Management.LegacyTokenMigration)
	}
	if len(capturedConfiguration.Tenants) != 0 || capturedConfiguration.OpenAIKey != "" || capturedConfiguration.MetaKey != "" {
		t.Fatalf("packaged management static credentials tenants=%d openai=%q meta=%q", len(capturedConfiguration.Tenants), capturedConfiguration.OpenAIKey, capturedConfiguration.MetaKey)
	}
	if capturedConfiguration.MetaBaseURL != "https://api.meta.ai/v1" {
		t.Fatalf("meta key/base URL=%q %q", capturedConfiguration.MetaKey, capturedConfiguration.MetaBaseURL)
	}
	metaCatalog := capturedConfiguration.ProviderModels[proxy.ProviderNameMeta]
	if metaCatalog.Text.DefaultModel != proxy.ModelNameMuseSpark11 || len(metaCatalog.Text.Models) != 1 || metaCatalog.Text.Models[0].ID != proxy.ModelNameMuseSpark11 {
		t.Fatalf("meta model catalog=%+v", metaCatalog)
	}
}

func TestRootCommandRendersStaticSiteWithoutBackendConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "missing-backend-config.yml")
	outputDirectory := filepath.Join(tempDir, "rendered-site")
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath, "--site-source", filepath.Join("..", "..", "site"), "--site-config-url", "https://llm-proxy-api.example/config-ui.yaml", "--render-site-output", outputDirectory)
	if executeError != nil {
		t.Fatalf("ExecuteC error: %v", executeError)
	}

	cnameBytes, readCNAMEError := os.ReadFile(filepath.Join(outputDirectory, "CNAME"))
	if readCNAMEError != nil {
		t.Fatalf("read CNAME: %v", readCNAMEError)
	}
	if string(cnameBytes) != "llm-proxy.mprlab.com\n" {
		t.Fatalf("CNAME=%q", string(cnameBytes))
	}
	if _, statError := os.Stat(filepath.Join(outputDirectory, proxy.ManagementConfigUIFileName)); !os.IsNotExist(statError) {
		t.Fatalf("rendered %s stat error=%v want absent", proxy.ManagementConfigUIFileName, statError)
	}
	if _, statError := os.Stat(filepath.Join(outputDirectory, siteLegacyRuntimeConfig)); !os.IsNotExist(statError) {
		t.Fatalf("rendered %s stat error=%v want absent", siteLegacyRuntimeConfig, statError)
	}
	indexBytes, readIndexError := os.ReadFile(filepath.Join(outputDirectory, "index.html"))
	if readIndexError != nil {
		t.Fatalf("rendered index.html: %v", readIndexError)
	}
	indexHTML := string(indexBytes)
	if !strings.Contains(indexHTML, `data-config-url="https://llm-proxy-api.example/config-ui.yaml"`) ||
		!strings.Contains(indexHTML, "data-mpr-ui-bundle-src=") {
		t.Fatalf("rendered index.html=%s", indexHTML)
	}
	if _, statError := os.Stat(filepath.Join(outputDirectory, "assets", "llm-proxy", "js", "app.js")); statError != nil {
		t.Fatalf("rendered app.js: %v", statError)
	}
}

func TestRootCommandRendersSiteFromDefaultSourceDirectory(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "missing-backend-config.yml")
	outputDirectory := filepath.Join(tempDir, "rendered-site")
	t.Chdir(filepath.Join("..", ".."))
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath, "--site-source", "", "--render-site-output", outputDirectory)
	if executeError != nil {
		t.Fatalf("ExecuteC error: %v", executeError)
	}
	if _, statError := os.Stat(filepath.Join(outputDirectory, "index.html")); statError != nil {
		t.Fatalf("rendered default-source index.html: %v", statError)
	}
}

func TestRootCommandRenderSiteRemovesSourceConfigUI(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "missing-backend-config.yml")
	sourceDirectory := filepath.Join(tempDir, "site-source")
	outputDirectory := filepath.Join(tempDir, "rendered-site")
	writeTestSiteSource(t, sourceDirectory)
	if writeError := os.WriteFile(filepath.Join(sourceDirectory, proxy.ManagementConfigUIFileName), []byte("stale config\n"), 0600); writeError != nil {
		t.Fatalf("write stale config-ui.yaml: %v", writeError)
	}
	if writeError := os.WriteFile(filepath.Join(sourceDirectory, siteLegacyRuntimeConfig), []byte("{}\n"), 0600); writeError != nil {
		t.Fatalf("write stale %s: %v", siteLegacyRuntimeConfig, writeError)
	}
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath, "--site-source", sourceDirectory, "--render-site-output", outputDirectory)
	if executeError != nil {
		t.Fatalf("ExecuteC error: %v", executeError)
	}
	if _, statError := os.Stat(filepath.Join(outputDirectory, proxy.ManagementConfigUIFileName)); !os.IsNotExist(statError) {
		t.Fatalf("rendered stale %s stat error=%v want absent", proxy.ManagementConfigUIFileName, statError)
	}
	if _, statError := os.Stat(filepath.Join(outputDirectory, siteLegacyRuntimeConfig)); !os.IsNotExist(statError) {
		t.Fatalf("rendered stale %s stat error=%v want absent", siteLegacyRuntimeConfig, statError)
	}
}

func TestRootCommandRenderSiteDoesNotLoadBackendConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
management:
  enabled: true
  public_origin: "https://llm-proxy.example"
  ui_description: "LLM Proxy Test"
  ui_origins:
    - "https://llm-proxy.example"
  tauth_url: "https://tauth.example"
  tauth_tenant_id: "llm-proxy"
  google_client_id: "${P411_MISSING_GOOGLE_CLIENT_ID}"
  login_path: "/auth/google"
  logout_path: "/auth/logout"
  nonce_path: "/auth/nonce"
  jwt_signing_key: "tauth-signing-key"
  jwt_issuer: "tauth"
  session_cookie_name: "llm_proxy_session"
  database_dialect: "sqlite"
  database_dsn: "management.sqlite"
  provider_key_encryption_key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
  management_api_origin: "https://llm-proxy-api.example"
  proxy_origin: "https://llm-proxy-api.example"
tenants:
  - id: default
    secret: "sekret"
`+completeLiteralProvidersYAML())
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath, "--site-source", filepath.Join("..", "..", "site"), "--render-site-output", filepath.Join(tempDir, "rendered-site"))
	if executeError != nil {
		t.Fatalf("ExecuteC error: %v", executeError)
	}
}

func TestRootCommandRejectsInvalidSiteRenderInputs(t *testing.T) {
	testCases := []struct {
		name          string
		setup         func(*testing.T, string) (string, string, string)
		expectedError string
	}{
		{
			name: "blank output",
			setup: func(subTest *testing.T, tempDir string) (string, string, string) {
				configPath := filepath.Join(tempDir, "missing-backend-config.yml")
				sourceDirectory := filepath.Join(tempDir, "site-source")
				writeTestSiteSource(subTest, sourceDirectory)
				return configPath, sourceDirectory, ""
			},
			expectedError: "output directory is required",
		},
		{
			name: "missing source",
			setup: func(subTest *testing.T, tempDir string) (string, string, string) {
				configPath := filepath.Join(tempDir, "missing-backend-config.yml")
				return configPath, filepath.Join(tempDir, "missing-site-source"), filepath.Join(tempDir, "rendered-site")
			},
			expectedError: "source=",
		},
		{
			name: "source file",
			setup: func(subTest *testing.T, tempDir string) (string, string, string) {
				configPath := filepath.Join(tempDir, "missing-backend-config.yml")
				sourcePath := filepath.Join(tempDir, "site-source-file")
				if writeError := os.WriteFile(sourcePath, []byte("not a directory"), 0600); writeError != nil {
					subTest.Fatalf("write source file: %v", writeError)
				}
				return configPath, sourcePath, filepath.Join(tempDir, "rendered-site")
			},
			expectedError: "is not a directory",
		},
		{
			name: "existing output",
			setup: func(subTest *testing.T, tempDir string) (string, string, string) {
				configPath := filepath.Join(tempDir, "missing-backend-config.yml")
				sourceDirectory := filepath.Join(tempDir, "site-source")
				outputDirectory := filepath.Join(tempDir, "rendered-site")
				writeTestSiteSource(subTest, sourceDirectory)
				if mkdirError := os.Mkdir(outputDirectory, 0700); mkdirError != nil {
					subTest.Fatalf("create output directory: %v", mkdirError)
				}
				return configPath, sourceDirectory, outputDirectory
			},
			expectedError: "already exists",
		},
		{
			name: "output stat failure",
			setup: func(subTest *testing.T, tempDir string) (string, string, string) {
				configPath := filepath.Join(tempDir, "missing-backend-config.yml")
				sourceDirectory := filepath.Join(tempDir, "site-source")
				outputParentFile := filepath.Join(tempDir, "output-parent-file")
				writeTestSiteSource(subTest, sourceDirectory)
				if writeError := os.WriteFile(outputParentFile, []byte("not a directory"), 0600); writeError != nil {
					subTest.Fatalf("write output parent file: %v", writeError)
				}
				return configPath, sourceDirectory, filepath.Join(outputParentFile, "rendered-site")
			},
			expectedError: "not a directory",
		},
		{
			name: "output inside source",
			setup: func(subTest *testing.T, tempDir string) (string, string, string) {
				configPath := filepath.Join(tempDir, "missing-backend-config.yml")
				sourceDirectory := filepath.Join(tempDir, "site-source")
				writeTestSiteSource(subTest, sourceDirectory)
				return configPath, sourceDirectory, filepath.Join(sourceDirectory, "rendered-site")
			},
			expectedError: "is inside source",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			tempDir := subTest.TempDir()
			configPath, sourceDirectory, outputDirectory := testCase.setup(subTest, tempDir)
			withServeProxy(subTest, failingServeProxy(subTest))

			executeError := executeRootCommand(subTest, "--config", configPath, "--site-source", sourceDirectory, "--render-site-output", outputDirectory)
			if executeError == nil || !strings.Contains(executeError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want contains %q", executeError, testCase.expectedError)
			}
		})
	}
}

func TestRootCommandRejectsSiteRenderInjectedFilesystemFailures(t *testing.T) {
	testCases := []struct {
		name          string
		setup         func(*testing.T, string, string)
		expectedError string
	}{
		{
			name: "source absolute path failure",
			setup: func(subTest *testing.T, sourceDirectory string, outputDirectory string) {
				sitePathAbs = func(rawPath string) (string, error) {
					return "", errors.New("source abs failed")
				}
			},
			expectedError: "source abs failed",
		},
		{
			name: "output absolute path failure",
			setup: func(subTest *testing.T, sourceDirectory string, outputDirectory string) {
				pathCalls := 0
				sitePathAbs = func(rawPath string) (string, error) {
					pathCalls++
					if pathCalls == 2 {
						return "", errors.New("output abs failed")
					}
					return filepath.Abs(rawPath)
				}
			},
			expectedError: "output abs failed",
		},
		{
			name: "relative path failure",
			setup: func(subTest *testing.T, sourceDirectory string, outputDirectory string) {
				sitePathRel = func(basePath string, targetPath string) (string, error) {
					return "", errors.New("relative path failed")
				}
			},
			expectedError: "relative path failed",
		},
		{
			name: "output stat failure",
			setup: func(subTest *testing.T, sourceDirectory string, outputDirectory string) {
				originalSiteStat := siteStat
				siteStat = func(rawPath string) (os.FileInfo, error) {
					if rawPath == sourceDirectory {
						return originalSiteStat(rawPath)
					}
					return nil, errors.New("output stat failed")
				}
			},
			expectedError: "output stat failed",
		},
		{
			name: "copy failure",
			setup: func(subTest *testing.T, sourceDirectory string, outputDirectory string) {
				siteCopyFS = func(outputPath string, sourceFileSystem fs.FS) error {
					return errors.New("copy failed")
				}
			},
			expectedError: "copy failed",
		},
		{
			name: "missing CNAME",
			setup: func(subTest *testing.T, sourceDirectory string, outputDirectory string) {
				if removeError := os.Remove(filepath.Join(sourceDirectory, siteCNAMEFileName)); removeError != nil {
					subTest.Fatalf("remove CNAME: %v", removeError)
				}
			},
			expectedError: "CNAME",
		},
		{
			name: "index read failure",
			setup: func(subTest *testing.T, sourceDirectory string, outputDirectory string) {
				siteReadFile = func(rawPath string) ([]byte, error) {
					return nil, errors.New("read failed")
				}
			},
			expectedError: "read failed",
		},
		{
			name: "index write failure",
			setup: func(subTest *testing.T, sourceDirectory string, outputDirectory string) {
				siteWriteFile = func(rawPath string, content []byte, fileMode os.FileMode) error {
					return errors.New("write failed")
				}
			},
			expectedError: "write failed",
		},
		{
			name: "stale config removal failure",
			setup: func(subTest *testing.T, sourceDirectory string, outputDirectory string) {
				siteRemove = func(rawPath string) error {
					return errors.New("remove failed")
				}
			},
			expectedError: "remove failed",
		},
		{
			name: "stale runtime config removal failure",
			setup: func(subTest *testing.T, sourceDirectory string, outputDirectory string) {
				siteRemove = func(rawPath string) error {
					if filepath.Base(rawPath) == siteLegacyRuntimeConfig {
						return errors.New("runtime remove failed")
					}
					return nil
				}
			},
			expectedError: "runtime remove failed",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			tempDir := subTest.TempDir()
			configPath := filepath.Join(tempDir, "missing-backend-config.yml")
			sourceDirectory := filepath.Join(tempDir, "site-source")
			outputDirectory := filepath.Join(tempDir, "rendered-site")
			writeTestSiteSource(subTest, sourceDirectory)
			withSiteRendererDependencies(subTest)
			testCase.setup(subTest, sourceDirectory, outputDirectory)
			withServeProxy(subTest, failingServeProxy(subTest))

			executeError := executeRootCommand(subTest, "--config", configPath, "--site-source", sourceDirectory, "--render-site-output", outputDirectory)
			if executeError == nil || !strings.Contains(executeError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want contains %q", executeError, testCase.expectedError)
			}
		})
	}
}

func TestRootCommandRejectsSiteRenderIndexWithoutCanonicalConfigURL(t *testing.T) {
	testCases := []struct {
		name          string
		indexHTML     string
		expectedError string
	}{
		{
			name:          "missing config url",
			indexHTML:     `<!doctype html><mpr-header></mpr-header>`,
			expectedError: siteConfigURLSourceAttribute,
		},
		{
			name:          "noncanonical source config url",
			indexHTML:     `<!doctype html><mpr-header data-config-url="https://llm-proxy-api.example/config-ui.yaml"></mpr-header>`,
			expectedError: siteConfigURLSourceAttribute,
		},
		{
			name:          "duplicate config url",
			indexHTML:     `<!doctype html><mpr-header data-config-url="/config-ui.yaml"></mpr-header><mpr-header data-config-url="/config-ui.yaml"></mpr-header>`,
			expectedError: "exactly one",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			tempDir := subTest.TempDir()
			configPath := filepath.Join(tempDir, "missing-backend-config.yml")
			sourceDirectory := filepath.Join(tempDir, "site-source")
			outputDirectory := filepath.Join(tempDir, "rendered-site")
			writeTestSiteSource(subTest, sourceDirectory)
			if writeError := os.WriteFile(filepath.Join(sourceDirectory, "index.html"), []byte(testCase.indexHTML+"\n"), 0600); writeError != nil {
				subTest.Fatalf("write index.html: %v", writeError)
			}
			withServeProxy(subTest, failingServeProxy(subTest))

			executeError := executeRootCommand(subTest, "--config", configPath, "--site-source", sourceDirectory, "--render-site-output", outputDirectory)
			if executeError == nil || !strings.Contains(executeError.Error(), testCase.expectedError) {
				subTest.Fatalf("error=%v want contains %q", executeError, testCase.expectedError)
			}
		})
	}
}

func TestRootCommandRejectsInvalidSiteConfigURL(t *testing.T) {
	testCases := []string{
		"",
		"/other.yaml",
		"config-ui.yaml",
		"//llm-proxy-api.example/config-ui.yaml",
		"http://llm-proxy-api.example/config-ui.yaml",
		"https://llm-proxy-api.example/config-ui.yaml?environment=production",
	}
	for _, rawConfigURL := range testCases {
		t.Run(rawConfigURL, func(subTest *testing.T) {
			tempDir := subTest.TempDir()
			outputDirectory := filepath.Join(tempDir, "rendered-site")
			withServeProxy(subTest, failingServeProxy(subTest))

			executeError := executeRootCommand(subTest, "--site-source", filepath.Join("..", "..", "site"), "--site-config-url", rawConfigURL, "--render-site-output", outputDirectory)
			if executeError == nil || !strings.Contains(executeError.Error(), "site config URL") {
				subTest.Fatalf("error=%v want invalid site config URL", executeError)
			}
		})
	}
}

func TestRootCommandReportsSiteConfigURLParseFailure(t *testing.T) {
	withSiteRendererDependencies(t)
	siteURLParse = func(rawURL string) (*url.URL, error) {
		return nil, errors.New("config URL parse failed")
	}
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--site-config-url", defaultSiteConfigURL, "--render-site-output", t.TempDir())
	if executeError == nil || !strings.Contains(executeError.Error(), "config URL parse failed") {
		t.Fatalf("error=%v want config URL parse failure", executeError)
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
			name:     "deepseek canonical",
			provider: proxy.ProviderNameDeepSeek,
			model:    proxy.ModelNameDeepSeekV4Flash,
			missingKey: func(values *providerYAMLValues) {
				values.DeepSeekAPIKey = "${P411_MISSING_DEEPSEEK_KEY}"
			},
			expectedError: "provider_api_key_required: provider=deepseek field=providers.deepseek.api_key",
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
			name:     "meta canonical",
			provider: proxy.ProviderNameMeta,
			model:    proxy.ModelNameMuseSpark11,
			missingKey: func(values *providerYAMLValues) {
				values.MetaAPIKey = "${P411_MISSING_META_KEY}"
			},
			expectedError: "provider_api_key_required: provider=meta field=providers.meta.api_key",
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
tenants:
  - id: default
    secret: "sekret"
    defaults:
      provider: openai
      model: gpt-4.1
      dictation_provider: openai
      dictation_model: gpt-4o-mini-transcribe
`+completeLiteralProvidersYAML())
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

func TestRootCommandRejectsUnknownTenantDefaultProviderAfterCredentialCheck(t *testing.T) {
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
	if executeError == nil || !strings.Contains(executeError.Error(), "unknown provider") {
		t.Fatalf("error=%v want unknown provider", executeError)
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
  meta:
    api_key: "sk-meta"
    base_url: "https://api.meta.ai/v1"
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
  meta:
    api_key: "sk-meta"
    base_url: "https://api.meta.ai/v1"
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
  meta:
    api_key: "sk-meta"
    base_url: "https://api.meta.ai/v1"
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
  meta:
    api_key: "sk-meta"
    base_url: "https://api.meta.ai/v1"
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
  meta:
    api_key: "sk-meta"
    base_url: "https://api.meta.ai/v1"
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
  meta:
    api_key: "sk-meta"
    base_url: "https://api.meta.ai/v1"
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
			name:          "missing meta base url",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "base_url: \"https://api.meta.ai/v1\"", "base_url: \"\"", 1),
			expectedError: "provider_base_url_required: provider=meta field=providers.meta.base_url",
		},
		{
			name:          "blank provider text default model",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "default_model: \"gpt-4.1\"", "default_model: \"\"", 1),
			expectedError: "invalid_model_catalog: provider=openai endpoint=text field=providers.openai.text.default_model",
		},
		{
			name:          "blank keyed gemini text default model",
			providersYAML: strings.Replace(completeLiteralProvidersYAML(), "default_model: \"gemini-2.5-flash\"", "default_model: \"\"", 1),
			expectedError: "invalid_model_catalog: provider=gemini endpoint=text field=providers.gemini.text.default_model",
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
	resetStringFlag(t, flagConfig, defaultConfigPath)
	resetStringFlag(t, flagRenderSiteOutput, "")
	resetStringFlag(t, flagSiteConfigURL, defaultSiteConfigURL)
	resetStringFlag(t, flagSiteSource, defaultSiteSourceDirectory)
}

func resetStringFlag(t *testing.T, flagName string, flagValue string) {
	t.Helper()
	commandFlags := rootCmd.Flags()
	if flagError := commandFlags.Set(flagName, flagValue); flagError != nil {
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

func withSiteRendererDependencies(t *testing.T) {
	t.Helper()
	originalSiteCopyFS := siteCopyFS
	originalSitePathAbs := sitePathAbs
	originalSitePathRel := sitePathRel
	originalSiteReadFile := siteReadFile
	originalSiteRemove := siteRemove
	originalSiteStat := siteStat
	originalSiteURLParse := siteURLParse
	originalSiteWriteFile := siteWriteFile
	t.Cleanup(func() {
		siteCopyFS = originalSiteCopyFS
		sitePathAbs = originalSitePathAbs
		sitePathRel = originalSitePathRel
		siteReadFile = originalSiteReadFile
		siteRemove = originalSiteRemove
		siteStat = originalSiteStat
		siteURLParse = originalSiteURLParse
		siteWriteFile = originalSiteWriteFile
	})
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

func writeTestSiteSource(t *testing.T, sourceDirectory string) {
	t.Helper()
	if mkdirError := os.MkdirAll(filepath.Join(sourceDirectory, "assets"), 0700); mkdirError != nil {
		t.Fatalf("create test site source: %v", mkdirError)
	}
	if writeError := os.WriteFile(filepath.Join(sourceDirectory, "index.html"), []byte(`<!doctype html>
	<mpr-header data-config-url="/config-ui.yaml"></mpr-header>
`), 0600); writeError != nil {
		t.Fatalf("write index.html: %v", writeError)
	}
	if writeError := os.WriteFile(filepath.Join(sourceDirectory, siteCNAMEFileName), []byte("llm-proxy.mprlab.com\n"), 0600); writeError != nil {
		t.Fatalf("write CNAME: %v", writeError)
	}
	if writeError := os.WriteFile(filepath.Join(sourceDirectory, "assets", "app.js"), []byte("export {};\n"), 0600); writeError != nil {
		t.Fatalf("write app.js: %v", writeError)
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
	MetaAPIKey                   string
	MetaBaseURL                  string
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
		MetaAPIKey:                   "sk-meta",
		MetaBaseURL:                  "https://api.meta.ai/v1",
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
  meta:
    api_key: "%s"
    base_url: "%s"
    text:
      default_model: "muse-spark-1.1"
      models:
        - id: "muse-spark-1.1"
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
		values.MetaAPIKey,
		values.MetaBaseURL,
		values.GrokAPIKey,
		values.GrokBaseURL,
		values.GrokTranscriptionsURL,
	)
}
