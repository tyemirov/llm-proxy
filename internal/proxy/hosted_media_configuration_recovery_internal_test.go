package proxy

import (
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHostedMediaRecoveryRejectsRemovedRuntimeAuthorization(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		hosted *HostedConfiguration
	}{
		{name: "disabled"},
		{name: "scope-removed", hosted: &HostedConfiguration{Offerings: []HostedOfferingConfiguration{{
			Provider: "openai", Model: "gpt-image-2", Operation: ModelOperationImageEditing, MaximumAttempts: 1,
			Conditions: CatalogPriceConditions{Quality: "low", Resolution: "1024x1024"},
		}}}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newFundedMediaRecoveryFixture(t)
			database := openJournalTransactionInstance(t, fixture.database)
			root := t.TempDir()
			_, management, _ := newHostedIdentityHTTPHandler(t, database, fixture.upstreamURL, root)
			var source struct{ File string }
			if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
				t.Fatal(err)
			}
			management.configuration.DatabasePath = source.File
			catalog := internalCanonicalProviderCatalog()
			for _, schema := range []*ProviderCatalogSchema{&catalog.schema, &catalog.runtimeSchema} {
				for index := range schema.Providers {
					if schema.Providers[index].ID != "openai" {
						continue
					}
					for transport := range schema.Providers[index].Transports {
						if schema.Providers[index].Transports[transport].Endpoint.Protocol == CatalogEndpointProtocolHTTP {
							schema.Providers[index].Transports[transport].Endpoint.DefaultBaseURL = fixture.upstreamURL
						}
					}
				}
			}
			configuration := withInternalUpstreamCapacity(t, Configuration{
				Management: management.configuration, ProviderCatalog: catalog, AssetStorePath: root, Hosted: scenario.hosted,
			})
			application, err := buildProxyApplication(configuration, zap.NewNop().Sugar(), func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
				return management.store, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(application.router)
			t.Cleanup(server.Close)
			server.Client().Transport = hostedMCPBearerTransport{token: hostedIdentityFixtureKey}
			deadline := time.After(5 * time.Second)
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			for {
				current := hostedMediaWorkerStatus(t, server, fixture.operationID)
				if current["state"] == MediaOperationStateFailed {
					if fixture.calls.Load() != 0 || len(current["outputs"].([]any)) != 0 {
						t.Fatalf("removed authorization dispatched or published output: state=%v calls=%d", current, fixture.calls.Load())
					}
					break
				}
				select {
				case <-deadline:
					t.Fatalf("removed authorization did not reject queued media: %v", current)
				case <-tick.C:
				}
			}
			if err := database.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			assertHostedFundsBalance(t, database, 500, 500)
			// Restoring the original scope must preserve the terminal result and released funds.
			fixture.recover(t, MediaOperationStateFailed)
		})
	}
}
