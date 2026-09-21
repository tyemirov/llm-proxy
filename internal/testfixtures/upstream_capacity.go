package testfixtures

import (
	"testing"

	"github.com/tyemirov/llm-proxy/internal/proxy"
)

// UpstreamCapacity declares the per-origin limits used by a test configuration.
// WithUpstreamCapacity binds these limits to that test's exact upstream servers.
func UpstreamCapacity(active, queued int) proxy.UpstreamCapacityConfiguration {
	global := proxy.UpstreamCapacityLimit{Active: max(2, active), Admitted: 65536}
	principal := proxy.UpstreamCapacityLimit{Active: active, Admitted: active + queued}
	return proxy.UpstreamCapacityConfiguration{
		Global: global, Tenant: principal, Account: principal,
		Media: global, Status: global, Transfer: global, InteractiveReserve: 1,
	}
}

// WithUpstreamCapacity supplies an explicit allocation for each test upstream.
func WithUpstreamCapacity(t testing.TB, configuration proxy.Configuration) proxy.Configuration {
	t.Helper()
	if configuration.UpstreamCapacity.Global.Active == 0 {
		configuration.UpstreamCapacity = UpstreamCapacity(4, 100)
	}
	if len(configuration.UpstreamCapacity.Origins) > 0 {
		return configuration
	}
	origins, err := configuration.ConfiguredUpstreamOrigins()
	if err != nil {
		t.Fatalf("test upstream origins: %v", err)
	}
	limit := configuration.UpstreamCapacity.Tenant
	for _, origin := range origins {
		configuration.UpstreamCapacity.Origins = append(configuration.UpstreamCapacity.Origins, proxy.UpstreamOriginCapacity{Origin: origin, Active: limit.Active, Queued: limit.Admitted - limit.Active})
	}
	for _, provider := range configuration.ProviderCatalog.Schema().Providers {
		hasConnectionEndpoint := false
		for _, transport := range provider.Transports {
			if transport.Endpoint.Protocol == proxy.CatalogEndpointProtocolHTTP && transport.Endpoint.SettingField != "" {
				hasConnectionEndpoint = true
			}
		}
		if provider.ID != proxy.ProviderNameDashScope || !hasConnectionEndpoint {
			continue
		}
		for _, origin := range []string{"https://managed-router-fixture.ap-southeast-1.maas.aliyuncs.com", "https://managed-router-test.ap-southeast-1.maas.aliyuncs.com"} {
			configuration.UpstreamCapacity.Origins = append(configuration.UpstreamCapacity.Origins, proxy.UpstreamOriginCapacity{Origin: origin, Provider: provider.ID, Active: limit.Active, Queued: limit.Admitted - limit.Active})
		}
	}
	return configuration
}
