package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

const (
	testConfigFileName          = "config.yml"
	testProviderCatalogFileName = "providers.yml"
	testDotEnvFileName          = ".env"
)

func TestRootCommandRunsConfiguredProxyFromConfigFile(t *testing.T) {
	originalProcessEnvironment := processEnvironment
	processEnvironment = func() []string { return nil }
	t.Cleanup(func() { processEnvironment = originalProcessEnvironment })
	tempDir := t.TempDir()
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
`)
	writeTestDotEnv(t, tempDir, `
P411_TAUTH_JWT_SIGNING_KEY=tauth-signing-key
P411_MANAGEMENT_DATABASE_PATH=/var/lib/llm-proxy/management.sqlite
P411_MANAGEMENT_PROVIDER_KEY_ENCRYPTION_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
OPENAI_API_KEY=sk-openai-catalog-binding
DASHSCOPE_BASE_URL=https://workspace.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1
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
	if capturedConfiguration.ProviderCatalog == nil || capturedConfiguration.ProviderCatalog.SchemaVersion() != proxy.ProviderCatalogSchemaVersion {
		t.Fatalf("provider catalog=%v", capturedConfiguration.ProviderCatalog)
	}
	if capturedConfiguration.ProviderConnectionValues[proxy.ProviderNameOpenAI][proxy.CatalogCredentialAPIKey] != "sk-openai-catalog-binding" {
		t.Fatalf("openai environment binding was not loaded")
	}
	if capturedConfiguration.ProviderConnectionValues[proxy.ProviderNameDashScope]["base_url"] != "https://workspace.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("dashscope environment binding=%q", capturedConfiguration.ProviderConnectionValues[proxy.ProviderNameDashScope]["base_url"])
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
`)
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
`)
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
	originalProcessEnvironment := processEnvironment
	processEnvironment = func() []string { return nil }
	t.Cleanup(func() { processEnvironment = originalProcessEnvironment })
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
	writeTestProviderCatalog(t, tempDir, canonicalProviderCatalogYAML())
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
DASHSCOPE_BASE_URL=https://workspace.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1
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
	metaOfferings := configuredProviderOfferings(capturedConfiguration.ModelCatalog, proxy.ProviderNameMeta)
	metaDefault, metaDefaultFound := configuredDefaultOffering(capturedConfiguration.ModelCatalog, proxy.ProviderNameMeta, proxy.ModelOperationText)
	if !metaDefaultFound || metaDefault.Model != proxy.ModelNameMuseSpark11 || len(metaOfferings) != 2 ||
		metaOfferings[0].Model != proxy.ModelNameMuseSpark11 || metaOfferings[1].Model != proxy.ModelNameMuseSpark12 {
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
`)
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

func TestRootCommandReturnsPublicCapabilityCatalogProjectionError(t *testing.T) {
	configPath := writeTestConfig(t, t.TempDir(), "server:\n  port: 9191")
	expectedError := errors.New("catalog projection stopped")
	originalConstructor := newPublicCapabilityCatalog
	newPublicCapabilityCatalog = func(proxy.Configuration) (proxy.PublicCapabilityCatalog, error) {
		return proxy.PublicCapabilityCatalog{}, expectedError
	}
	t.Cleanup(func() { newPublicCapabilityCatalog = originalConstructor })
	withServeProxy(t, failingServeProxy(t))
	withServePublicCapabilityAPI(t, func(proxy.PublicCapabilityCatalog, int, string) error {
		t.Fatal("public capability API must not start")
		return nil
	})

	executeError := executeRootCommand(t, "--config", configPath, "--public-capabilities-only")
	if executeError == nil || !strings.Contains(executeError.Error(), expectedError.Error()) {
		t.Fatalf("error=%v want=%v", executeError, expectedError)
	}
}

func TestRootCommandPrintsCatalogDerivedLiveDiscovery(t *testing.T) {
	configPath := writeTestConfig(t, t.TempDir(), completeManagementYAML())
	var output bytes.Buffer
	originalOutput := rootCmd.OutOrStdout()
	rootCmd.SetOut(&output)
	t.Cleanup(func() { rootCmd.SetOut(originalOutput) })
	withServeProxy(t, failingServeProxy(t))

	if executeError := executeRootCommand(t, "--config", configPath, "--provider-catalog-only"); executeError != nil {
		t.Fatalf("ExecuteC error: %v", executeError)
	}
	var discovery providerCatalogDiscovery
	if decodeError := json.Unmarshal(output.Bytes(), &discovery); decodeError != nil {
		t.Fatalf("decode provider discovery: %v", decodeError)
	}
	if discovery.SchemaVersion != proxy.ProviderCatalogSchemaVersion || len(discovery.Providers) != 11 {
		t.Fatalf("provider discovery=%+v", discovery)
	}
	dashScopeFound := false
	for _, provider := range discovery.Providers {
		if provider.ID != proxy.ProviderNameDashScope {
			continue
		}
		dashScopeFound = len(provider.Fields) == 2 && provider.Fields[0].Environment == "DASHSCOPE_API_KEY" && provider.Fields[1].Environment == "DASHSCOPE_BASE_URL"
	}
	if !dashScopeFound {
		t.Fatalf("DashScope discovery=%+v", discovery.Providers)
	}
	for _, privateCatalogFragment := range []string{"default_base_url", "authentication", "upstream_model"} {
		if strings.Contains(output.String(), privateCatalogFragment) {
			t.Fatalf("provider discovery exposed %q", privateCatalogFragment)
		}
	}
}

func TestRootCommandRejectsInvalidCatalogModeSelection(t *testing.T) {
	configPath := writeTestConfig(t, t.TempDir(), completeManagementYAML())
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath, "--public-capabilities-only", "--provider-catalog-only")
	if executeError == nil || !strings.Contains(executeError.Error(), "mutually exclusive") {
		t.Fatalf("error=%v want mutually exclusive catalog modes", executeError)
	}
}

func TestRootCommandRejectsMissingProviderCatalogInDiscoveryMode(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, testConfigFileName)
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath, "--provider-catalog-only")
	if executeError == nil || !strings.Contains(executeError.Error(), "provider_catalog_read_failed") {
		t.Fatalf("error=%v want provider catalog read failure", executeError)
	}
}

func TestRootCommandRejectsMissingRuntimeConfigAfterCatalogLoad(t *testing.T) {
	tempDir := t.TempDir()
	writeTestProviderCatalog(t, tempDir, canonicalProviderCatalogYAML())
	configPath := filepath.Join(tempDir, "missing.yml")
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "config_file_read_failed") {
		t.Fatalf("error=%v want runtime config read failure", executeError)
	}
}

func TestRootCommandRejectsInvalidCatalogEnvironmentBinding(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, completeManagementYAML())
	writeTestDotEnv(t, tempDir, "DASHSCOPE_BASE_URL=ftp://workspace.invalid")
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "provider=dashscope field=base_url") {
		t.Fatalf("error=%v want invalid DashScope catalog binding", executeError)
	}
}

func TestRootCommandRejectsInvalidPublicCapabilityConfig(t *testing.T) {
	testCases := []struct {
		name          string
		prepare       func(*testing.T, string) string
		expectedError string
	}{
		{
			name: "missing config file",
			prepare: func(subTest *testing.T, tempDir string) string {
				writeTestProviderCatalog(subTest, tempDir, canonicalProviderCatalogYAML())
				return filepath.Join(tempDir, "missing.yml")
			},
			expectedError: "missing.yml",
		},
		{
			name: "missing provider catalog",
			prepare: func(subTest *testing.T, tempDir string) string {
				configPath := writeTestConfig(subTest, tempDir, "server:\n  port: 9191")
				if removeError := os.Remove(filepath.Join(tempDir, testProviderCatalogFileName)); removeError != nil {
					subTest.Fatalf("remove provider catalog: %v", removeError)
				}
				return configPath
			},
			expectedError: "provider_catalog_read_failed",
		},
		{
			name: "malformed provider catalog",
			prepare: func(subTest *testing.T, tempDir string) string {
				configPath := writeTestConfig(subTest, tempDir, "server:\n  port: 9191")
				writeTestProviderCatalog(subTest, tempDir, "schema_version: [")
				return configPath
			},
			expectedError: "provider_catalog_invalid",
		},
		{
			name: "malformed config",
			prepare: func(subTest *testing.T, tempDir string) string {
				return writeTestConfig(subTest, tempDir, "server: [")
			},
			expectedError: "did not find expected node content",
		},
		{
			name: "missing server",
			prepare: func(subTest *testing.T, tempDir string) string {
				return writeTestConfig(subTest, tempDir, completeManagementYAML())
			},
			expectedError: "field=server",
		},
		{
			name: "unknown server field",
			prepare: func(subTest *testing.T, tempDir string) string {
				return writeTestConfig(subTest, tempDir, "server:\n  port: 9191\n  future_option: true")
			},
			expectedError: "field=server",
		},
		{
			name: "nonpositive port",
			prepare: func(subTest *testing.T, tempDir string) string {
				return writeTestConfig(subTest, tempDir, "server:\n  port: 0")
			},
			expectedError: "server.port must be positive",
		},
		{
			name: "nonpositive timeout",
			prepare: func(subTest *testing.T, tempDir string) string {
				return writeTestConfig(subTest, tempDir, "server:\n  port: 9191\n  max_request_timeout_seconds: 0")
			},
			expectedError: "server.max_request_timeout_seconds must be positive",
		},
		{
			name: "invalid provider catalog",
			prepare: func(subTest *testing.T, tempDir string) string {
				configPath := writeTestConfig(subTest, tempDir, "server:\n  port: 9191")
				writeTestProviderCatalog(subTest, tempDir, strings.Replace(canonicalProviderCatalogYAML(), "          transport: text", "          transport: missing", 1))
				return configPath
			},
			expectedError: "transport=missing",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			configPath := testCase.prepare(subTest, subTest.TempDir())
			withServeProxy(subTest, failingServeProxy(subTest))
			withServePublicCapabilityAPI(subTest, func(proxy.PublicCapabilityCatalog, int, string) error {
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
	if executeError == nil || !strings.Contains(executeError.Error(), "path=providers.yml") {
		t.Fatalf("error=%v want default config path", executeError)
	}
}

func TestRootCommandRejectsUnreadableDotEnv(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeTestConfig(t, tempDir, `
server:
  log_level: info
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
	configPath := writeTestConfig(t, tempDir, completeManagementYAML())
	providerCatalog := strings.Replace(canonicalProviderCatalogYAML(), "      label: OpenAI\n      key_acquisition_url:", "      label: OpenAI\n      reasoning_effort: high\n      key_acquisition_url:", 1)
	writeTestProviderCatalog(t, tempDir, providerCatalog)
	withServeProxy(t, failingServeProxy(t))

	executeError := executeRootCommand(t, "--config", configPath)
	if executeError == nil || !strings.Contains(executeError.Error(), "provider_catalog_invalid") || !strings.Contains(executeError.Error(), "reasoning_effort") {
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

func TestRootCommandRejectsInvalidProviderCatalog(t *testing.T) {
	testCases := []struct {
		name          string
		mutate        func(string) string
		expectedError string
	}{
		{
			name:          "malformed yaml",
			mutate:        func(string) string { return "schema_version: [" },
			expectedError: "provider_catalog_invalid",
		},
		{
			name: "unsupported schema version",
			mutate: func(document string) string {
				return strings.Replace(document, "schema_version: 1", "schema_version: 2", 1)
			},
			expectedError: "field=schema_version value=2",
		},
		{
			name: "unknown field",
			mutate: func(document string) string {
				return strings.Replace(document, "schema_version: 1", "schema_version: 1\nfuture_option: true", 1)
			},
			expectedError: "field future_option not found",
		},
		{
			name: "duplicate environment binding",
			mutate: func(document string) string {
				return strings.Replace(document, "environment: DEEPSEEK_API_KEY", "environment: OPENAI_API_KEY", 1)
			},
			expectedError: "environment=OPENAI_API_KEY",
		},
		{
			name: "dangling transport",
			mutate: func(document string) string {
				return strings.Replace(document, "          transport: text", "          transport: missing", 1)
			},
			expectedError: "transport=missing",
		},
		{
			name: "unknown protocol",
			mutate: func(document string) string {
				return strings.Replace(document, "request_protocol: openai_responses", "request_protocol: future_protocol", 1)
			},
			expectedError: "reason=unsupported_protocol",
		},
		{
			name: "dangling authentication field",
			mutate: func(document string) string {
				return strings.Replace(document, "            field: api_key", "            field: missing", 1)
			},
			expectedError: "field_id=missing",
		},
		{
			name: "invalid media limit",
			mutate: func(document string) string {
				return strings.Replace(document, "              value: 512000000", "              value: -1", 1)
			},
			expectedError: "media_limits[0].value",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(subTest *testing.T) {
			tempDir := subTest.TempDir()
			configPath := writeTestConfig(subTest, tempDir, completeManagementYAML())
			writeTestProviderCatalog(subTest, tempDir, testCase.mutate(canonicalProviderCatalogYAML()))
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
	publicCapabilityConfiguration = publicCapabilityAPIConfiguration{}
	providerCatalogOnlyConfiguration = providerCatalogDiscovery{}
	return executeError
}

func resetConfigFlag(t *testing.T) {
	t.Helper()
	resetStringFlag(t, flagConfig, defaultConfigPath)
	resetBoolFlag(t, flagPublicCapabilitiesOnly, false)
	resetBoolFlag(t, flagProviderCatalogOnly, false)
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
		providerCatalogOnlyConfiguration = providerCatalogDiscovery{}
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
		providerCatalogOnlyConfiguration = providerCatalogDiscovery{}
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
	providerCatalogPath := filepath.Join(tempDir, testProviderCatalogFileName)
	if _, statError := os.Stat(providerCatalogPath); os.IsNotExist(statError) {
		writeTestProviderCatalog(t, tempDir, canonicalProviderCatalogYAML())
	} else if statError != nil {
		t.Fatalf("inspect provider catalog: %v", statError)
	}
	configPath := filepath.Join(tempDir, testConfigFileName)
	if writeError := os.WriteFile(configPath, []byte(strings.TrimSpace(configContent)+"\n"), 0600); writeError != nil {
		t.Fatalf("write config: %v", writeError)
	}
	return configPath
}

func writeTestProviderCatalog(t *testing.T, tempDir string, catalogContent string) {
	t.Helper()
	providerCatalogPath := filepath.Join(tempDir, testProviderCatalogFileName)
	if writeError := os.WriteFile(providerCatalogPath, []byte(strings.TrimSpace(catalogContent)+"\n"), 0600); writeError != nil {
		t.Fatalf("write provider catalog: %v", writeError)
	}
}

func canonicalProviderCatalogYAML() string {
	catalogBytes, readError := os.ReadFile(filepath.Join("..", "..", "configs", testProviderCatalogFileName))
	if readError != nil {
		panic(fmt.Sprintf("read canonical provider catalog: %v", readError))
	}
	return string(catalogBytes)
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
	return completeManagementYAML()
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
