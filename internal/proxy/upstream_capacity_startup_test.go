package proxy_test

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
)

func TestUpstreamCapacityRejectsUndeclaredSavedConnectionOrigin(t *testing.T) {
	const origin = "https://managed-router-test.ap-southeast-1.maas.aliyuncs.com"
	databasePath := filepath.Join(t.TempDir(), "management.sqlite")
	configuration, err := configurationWithCatalogs(t, managementConfigurationWithDatabasePath(proxy.Configuration{}, databasePath))
	if err != nil {
		t.Fatal(err)
	}
	router := newManagementRouterWithDatabasePath(t, configuration, databasePath)
	owner := managementSessionCookie(t, "capacity-origin-owner")
	tenantID := managementDefaultTenantTestID(t, router, owner)
	response := putManagementProviderKeyWithBaseURL(t, router, owner, tenantID, proxy.ProviderNameDashScope, "test-key", origin+"/compatible-mode/v1", proxy.ModelNameDashScopeQwenPlus, "", context.Background())
	if response.Code != http.StatusOK {
		t.Fatalf("save connection: status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err = proxy.BuildRouter(configuration, zap.NewNop().Sugar()); err != nil {
		t.Fatalf("declared saved origin: %v", err)
	}
	configuration.UpstreamCapacity.Origins = slices.DeleteFunc(configuration.UpstreamCapacity.Origins, func(rule proxy.UpstreamOriginCapacity) bool { return rule.Origin == origin })
	if _, err = proxy.BuildRouter(configuration, zap.NewNop().Sugar()); !errors.Is(err, proxy.ErrInvalidUpstreamCapacity) {
		t.Fatalf("undeclared saved origin startup error=%v", err)
	}
}

func TestUpstreamCapacityConfiguredOriginsRejectMalformedInputs(t *testing.T) {
	if _, err := (proxy.Configuration{}).ConfiguredUpstreamOrigins(); !errors.Is(err, proxy.ErrInvalidUpstreamCapacity) {
		t.Fatalf("missing catalog error=%v", err)
	}
	configuration, err := configurationWithCatalogs(t, managementConfigurationWithDatabasePath(proxy.Configuration{}, filepath.Join(t.TempDir(), "management.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"http://[::1", "https://user:password@provider.example/models", "ftp://provider.example/models"} {
		endpoints := proxy.NewEndpoints()
		endpoints.SetModelsURL(endpoint)
		configuration.Endpoints = endpoints
		if _, err := configuration.ConfiguredUpstreamOrigins(); !errors.Is(err, proxy.ErrInvalidUpstreamCapacity) || !strings.Contains(err.Error(), "endpoint=models") {
			t.Fatalf("invalid models origin error=%v", err)
		}
	}
}
