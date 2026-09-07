package proxy

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestModelActivationMigratesStoredSelectionsAtStartup(t *testing.T) {
	for _, hasMigration := range []bool{true, false} {
		name := "explicit migration"
		if !hasMigration {
			name = "missing migration"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newManagedGeminiModelSelectionMigrationFixture(t, managedGeminiRouteRetirementVersion, "activation-migration.db")
			schema := internalCanonicalProviderCatalog().Schema()
			var templateModel ProviderCatalogModel
			for _, model := range schema.Models {
				if model.ID == fixture.targetModel {
					templateModel = model
				}
			}
			providerIndex := slices.IndexFunc(schema.Providers, func(provider ProviderCatalogProvider) bool { return provider.ID == ProviderNameGemini })
			provider := &schema.Providers[providerIndex]
			templateOffering := provider.Offerings[0]
			for _, profile := range fixture.profiles {
				model := templateModel
				model.ID, model.Version, model.Enabled = profile.TextModel, profile.TextModel, ModelDisabled
				index := slices.IndexFunc(schema.Models, func(candidate ProviderCatalogModel) bool { return candidate.ID == model.ID })
				if index < 0 {
					schema.Models = append(schema.Models, model)
				} else {
					schema.Models[index] = model
				}
				offering := templateOffering
				offering.Model, offering.UpstreamModel, offering.DefaultOperations = model.ID, model.ID, nil
				provider.Offerings = append(provider.Offerings, offering)
			}
			if !hasMigration {
				schema.ModelMigrations = slices.DeleteFunc(schema.ModelMigrations, func(migration ProviderCatalogModelMigration) bool {
					return migration.ManagedSchemaVersion == managedGeminiRouteRetirementVersion
				})
			}
			catalog, err := NewProviderCatalog(schema)
			if err != nil {
				t.Fatal(err)
			}
			management := managedRouterTestManagementConfiguration()
			management.DatabaseDialector = fixture.database.Dialector
			router, err := BuildRouter(Configuration{ProviderCatalog: catalog, Management: management, AssetStorePath: t.TempDir()}, zap.NewNop().Sugar())
			if !hasMigration {
				if err == nil || !strings.Contains(err.Error(), "read_model_migrations") {
					t.Fatalf("startup must reject absent migration: %v", err)
				}
				fixture.assertUnchanged(t)
				return
			}
			if err != nil {
				t.Fatalf("startup migration: %v", err)
			}
			for _, previous := range fixture.profiles {
				var profile managedProviderProfileRecord
				if err := fixture.database.Where("tenant_id = ? AND provider_id = ?", previous.TenantID, previous.ProviderID).First(&profile).Error; err != nil {
					t.Fatal(err)
				}
				var tenant managedTenantRecord
				if err := fixture.database.Where("tenant_id = ?", previous.TenantID).First(&tenant).Error; err != nil {
					t.Fatal(err)
				}
				if profile.TextModel != fixture.targetModel || tenant.DefaultModel != fixture.targetModel {
					t.Fatalf("stored disabled selection remained: profile=%s default=%s", profile.TextModel, tenant.DefaultModel)
				}
			}
			request := httptest.NewRequest(http.MethodGet, PublicCapabilitiesPath, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("discovery status=%d", response.Code)
			}
			for _, previous := range fixture.profiles {
				if strings.Contains(response.Body.String(), `"`+previous.TextModel+`"`) {
					t.Fatalf("public discovery retained disabled model %s", previous.TextModel)
				}
			}
		})
	}
}
