package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestPublicHealthChecksDatastoreWithoutUsage(t *testing.T) {
	management := ManagementConfiguration{
		PublicOrigin: "http://localhost", UIOrigins: []string{"http://localhost"},
		TAuthTenantID: "health-test", JWTSigningKey: "health-signing-key",
		JWTIssuer: DefaultManagementJWTIssuer, SessionCookieName: "health-session",
		DatabasePath:             filepath.Join(t.TempDir(), "management.sqlite"),
		ProviderKeyEncryptionKey: testManagedProviderKeyEncryptionKey, UsageQueueSize: 1,
	}
	validator, err := newManagementSessionValidator(management)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := newRequestTimeoutPolicy(5, 5)
	if err != nil {
		t.Fatal(err)
	}
	models := internalManagedUsageWriterProviderModels()
	config := Configuration{
		Management: management, WorkerCount: 1, QueueSize: 1, MaxPromptBytes: 1024,
		Endpoints: NewEndpoints(), ProviderCatalog: internalTestProviderCatalog(models),
		ModelCatalog: models, LogLevel: LogLevelInfo, AssetStorePath: t.TempDir(),
		upstreamRateLimits:         upstreamRateLimits{rules: map[string]upstreamRateLimitRule{}},
		managementSessionValidator: validator, requestTimeoutPolicy: policy, validated: true,
	}
	core, logs := observer.New(zap.InfoLevel)
	var store *managedTenantStore
	router, err := buildRouter(config, zap.New(core).Sugar(), func(configuration ManagementConfiguration, providers *providerRegistry) (*managedTenantStore, error) {
		var openError error
		store, openError = newManagedTenantStore(configuration, providers)
		return store, openError
	})
	if err != nil {
		t.Fatal(err)
	}
	listener := httptest.NewServer(router)
	defer listener.Close()
	logs.TakeAll()
	check := func(status int, body string) {
		response, requestError := listener.Client().Get(listener.URL + healthPath)
		if requestError != nil {
			t.Fatal(requestError)
		}
		defer response.Body.Close()
		payload, readError := io.ReadAll(response.Body)
		if readError != nil {
			t.Fatal(readError)
		}
		if response.StatusCode != status || response.Header.Get("Cache-Control") != "no-store" || strings.TrimSpace(string(payload)) != body {
			t.Fatalf("health response=%d headers=%v body=%s", response.StatusCode, response.Header, payload)
		}
	}
	check(http.StatusOK, `{"status":"ok"}`)
	if logs.Len() != 0 {
		t.Fatalf("successful probe logs: %+v", logs.All())
	}
	database := store.database.(*gormManagedTenantDatabase).database
	var count int64
	if err := database.Model(&managedUsageEventRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("probe created usage: count=%d error=%v", count, err)
	}
	connection, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	check(http.StatusServiceUnavailable, `{"status":"unavailable"}`)
	if logs.FilterMessage("health probe failed").Len() != 1 {
		t.Fatal("missing failed probe evidence")
	}
}
