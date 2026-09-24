package proxy

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHostedMediaRecoveryAuthorizesBeforeWorkersStart(t *testing.T) {
	fixture := newFundedMediaRecoveryFixture(t)
	configuration, management := fixture.restartConfiguration(t, &HostedConfiguration{Offerings: []HostedOfferingConfiguration{{
		Provider: "openai", Model: "gpt-image-2", Operation: ModelOperationImageGeneration, MaximumAttempts: 1,
		Conditions: CatalogPriceConditions{Quality: "low", Resolution: "1024x1024"},
	}}})
	// Observe queued work while a later startup dependency is still initializing.
	originalLstat := structuredRequestLstat
	t.Cleanup(func() { structuredRequestLstat = originalLstat })
	observed := false
	structuredRequestLstat = func(path string) (os.FileInfo, error) {
		if path == filepath.Join(configuration.AssetStorePath, structuredRequestDirectoryName) {
			observed = true
			current := hostedMediaWorkerStatus(t, fixture.server, fixture.operationID)
			if current["state"] != MediaOperationStateQueued || fixture.calls.Load() != 0 {
				t.Fatalf("media started before runtime initialization: state=%v calls=%d", current, fixture.calls.Load())
			}
		}
		return originalLstat(path)
	}
	application, err := buildRouterWithStoreForTest(t, configuration, zap.NewNop().Sugar(), func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
		return management, nil
	})
	structuredRequestLstat = originalLstat
	if err != nil {
		t.Fatal(err)
	}
	if !observed {
		t.Fatal("startup did not reach the controlled storage dependency")
	}
	server := httptest.NewServer(application)
	t.Cleanup(server.Close)
	server.Client().Transport = hostedMCPBearerTransport{token: hostedIdentityFixtureKey}
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	var current map[string]any
	for {
		current = hostedMediaWorkerStatus(t, server, fixture.operationID)
		if current["state"] != MediaOperationStateQueued && current["state"] != MediaOperationStateRunning {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("authorized media did not recover after startup: %v", current)
		case <-tick.C:
		}
	}
	if current["state"] != MediaOperationStateSucceeded || fixture.calls.Load() != 1 || len(current["outputs"].([]any)) != 1 {
		t.Fatalf("authorized queued media did not recover: state=%v calls=%d", current, fixture.calls.Load())
	}
	if err := fixture.database.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, fixture.database, 498, 498)
	before := fixture.state(t)
	for range 2 {
		replay := hostedMediaAdmissionHTTP(t, server, mediaRecoveryKey, mediaRecoveryPrompt, http.StatusOK)
		fixture.worker.runOperation("replayed-media-worker", fixture.operationID)
		if err := fixture.database.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		if replay["operation_id"] != fixture.operationID || fixture.calls.Load() != 1 || !reflect.DeepEqual(before, fixture.state(t)) {
			t.Fatal("recovered media repeated provider work or financial effects")
		}
	}
}

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
			configuration, management := fixture.restartConfiguration(t, scenario.hosted)
			application, err := buildRouterWithStoreForTest(t, configuration, zap.NewNop().Sugar(), func(ManagementConfiguration, *providerRegistry) (*managedTenantStore, error) {
				return management, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(application)
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
			if err := fixture.database.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			assertHostedFundsBalance(t, fixture.database, 500, 500)
			// Restoring the original scope must preserve the terminal result and released funds.
			fixture.recover(t, MediaOperationStateFailed)
		})
	}
}

func (fixture fundedMediaRecoveryFixture) restartConfiguration(t *testing.T, hosted *HostedConfiguration) (Configuration, *managedTenantStore) {
	t.Helper()
	database := openJournalTransactionInstance(t, fixture.database)
	root := t.TempDir()
	_, management, _ := newHostedIdentityHTTPHandler(t, database, fixture.upstreamURL, root)
	var source struct{ File string }
	if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	management.configuration.DatabasePath = source.File
	catalog := internalCanonicalProviderCatalog()
	catalog.modelCatalog = hostedImageFinancialCatalog(ModelOperationImageGeneration)
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
		Management: management.configuration, ProviderCatalog: catalog, AssetStorePath: root, Hosted: hosted,
	})
	return configuration, management.store
}
