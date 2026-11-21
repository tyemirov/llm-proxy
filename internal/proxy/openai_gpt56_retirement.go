package proxy

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const managedOpenAIGPT56CutoverVersion = 18

type openAICutoverSelection struct {
	tenantID string
	source   string
	target   string
}

type openAICutoverPlan struct {
	tenants  []openAICutoverSelection
	profiles []openAICutoverSelection
	record   bool
}

func openAICutoverTarget(providers *providerRegistry, provider, model string) (string, bool) {
	return providers.modelMigrationTarget(managedOpenAIGPT56CutoverVersion, provider, ModelOperationText, model)
}

// This projection is confined to predecessor-schema validation. Current request
// routing never resolves a retired selection through a replacement model.
func projectOpenAIPredecessorDefaults(providers *providerRegistry, defaults TenantDefaults) TenantDefaults {
	if target, changed := openAICutoverTarget(providers, defaults.Provider, defaults.Model); changed {
		defaults.Model = target
	}
	return defaults
}

func projectOpenAIPredecessorProfiles(providers *providerRegistry, profiles []managedProviderProfileRecord) []managedProviderProfileRecord {
	result := append([]managedProviderProfileRecord(nil), profiles...)
	for index := range result {
		if target, changed := openAICutoverTarget(providers, result[index].ProviderID, result[index].TextModel); changed {
			result[index].TextModel = target
		}
	}
	return result
}

// Earlier one-off migrations validate against the final catalog without
// mutating an OpenAI selection ahead of this cutover's model-only update.
func validateOpenAIPredecessorRoutingDefaults(providers *providerRegistry, settings map[providerID]managedProviderSettings, defaults TenantDefaults) (managedRoutingDefaults, error) {
	projected := make(map[providerID]managedProviderSettings, len(settings))
	for provider, value := range settings {
		if target, changed := openAICutoverTarget(providers, provider.string(), value.textModel); changed {
			value.textModel = target
		}
		projected[provider] = value
	}
	return validatePersistedManagedRoutingDefaults(providers, projected, projectOpenAIPredecessorDefaults(providers, defaults))
}

// preflightOpenAICutover reads selected model identities and inherited efforts
// before any schema/default write. The outer startup transaction owns the plan.
func preflightOpenAICutover(database *gorm.DB, providers *providerRegistry) (openAICutoverPlan, error) {
	plan := openAICutoverPlan{}
	migrations := providers.modelMigrations[managedOpenAIGPT56CutoverVersion]
	if len(migrations) == 0 {
		return plan, nil
	}
	for _, migration := range migrations {
		if migration.provider != ProviderNameOpenAI || migration.operation != ModelOperationText || migration.targetReasoningEffort != "" || !migration.preserveSourceUsage {
			return plan, fmt.Errorf("%w: operation=preflight_openai_cutover invalid_preservation_policy source_model=%s", errManagedTenantSchemaMigration, migration.source)
		}
	}
	if !database.Migrator().HasTable(&managedTenantRecord{}) {
		return plan, nil
	}
	var tenants []managedTenantRecord
	legacyTenantsByUser := map[string]legacyManagedTenantRecord{}
	tenantColumns, columnError := database.Migrator().ColumnTypes(managedTenantTable)
	if columnError != nil {
		return plan, fmt.Errorf("%w: operation=preflight_openai_cutover inspect_columns table=%s: %w", errManagedTenantSchemaMigration, managedTenantTable, columnError)
	}
	legacyOwnership := false
	for _, column := range tenantColumns {
		if column.Name() == "user_id" {
			legacyOwnership = true
		}
	}
	if legacyOwnership {
		var legacyTenants []legacyManagedTenantRecord
		if err := database.Table(managedTenantTable).Find(&legacyTenants).Error; err != nil {
			return plan, fmt.Errorf("%w: operation=preflight_openai_cutover tenants: %w", errManagedTenantSchemaMigration, err)
		}
		for _, legacy := range legacyTenants {
			legacyTenantsByUser[legacy.UserID] = legacy
			tenants = append(tenants, managedTenantRecord{TenantID: legacy.TenantID, OwnerUserID: legacy.UserID, DefaultProvider: legacy.DefaultProvider, DefaultModel: legacy.DefaultModel, DefaultReasoningEffort: legacy.DefaultReasoningEffort})
		}
	} else if err := database.Find(&tenants).Error; err != nil {
		return plan, fmt.Errorf("%w: operation=preflight_openai_cutover tenants: %w", errManagedTenantSchemaMigration, err)
	}
	byID := make(map[string]managedTenantRecord, len(tenants))
	validate := func(tenant managedTenantRecord, source, target, scope string) error {
		definition, model, err := providers.resolveTextModel(ProviderNameOpenAI, target, ProviderNameOpenAI, target, false)
		if err == nil {
			err = validateReasoningEffortForResolvedTextRoute(definition, model, tenant.DefaultReasoningEffort)
		}
		if err != nil {
			return fmt.Errorf("%w: operation=preflight_openai_cutover tenant=%s provider=openai scope=%s source_model=%s target_model=%s effort=%q reason=provider_reasoning_decision_required: %w", errManagedTenantSchemaMigration, tenant.TenantID, scope, source, target, tenant.DefaultReasoningEffort, err)
		}
		return nil
	}
	for _, tenant := range tenants {
		byID[tenant.TenantID] = tenant
		if target, changed := openAICutoverTarget(providers, tenant.DefaultProvider, tenant.DefaultModel); changed {
			if err := validate(tenant, tenant.DefaultModel, target, "default"); err != nil {
				return plan, err
			}
			plan.tenants = append(plan.tenants, openAICutoverSelection{tenant.TenantID, tenant.DefaultModel, target})
		}
	}
	var profiles []managedProviderProfileRecord
	if database.Migrator().HasTable(&managedProviderProfileRecord{}) {
		if err := database.Where(&managedProviderProfileRecord{ProviderID: ProviderNameOpenAI}).Find(&profiles).Error; err != nil {
			return plan, fmt.Errorf("%w: operation=preflight_openai_cutover profiles: %w", errManagedTenantSchemaMigration, err)
		}
	}
	if legacyOwnership && database.Migrator().HasTable(&managedProviderAPIKeyRecord{}) {
		var predecessor []legacyManagedProviderAPIKeyRecord
		if err := database.Table(managedProviderKeyTable).Where(&legacyManagedProviderAPIKeyRecord{ProviderID: ProviderNameOpenAI}).Find(&predecessor).Error; err != nil {
			return plan, fmt.Errorf("%w: operation=preflight_openai_cutover predecessor profiles: %w", errManagedTenantSchemaMigration, err)
		}
		for _, record := range predecessor {
			owner, exists := legacyTenantsByUser[record.UserID]
			if !exists {
				return plan, fmt.Errorf("%w: operation=preflight_openai_cutover orphan_user=%s provider=%s", errManagedTenantSchemaMigration, record.UserID, record.ProviderID)
			}
			profiles = append(profiles, managedProviderProfileRecord{TenantID: owner.TenantID, ProviderID: record.ProviderID, TextModel: record.TextModel})
		}
	} else if database.Migrator().HasTable(&managedProviderAPIKeyRecord{}) {
		var predecessor []managedProviderAPIKeyRecord
		if err := database.Where(&managedProviderAPIKeyRecord{ProviderID: ProviderNameOpenAI}).Find(&predecessor).Error; err != nil {
			return plan, fmt.Errorf("%w: operation=preflight_openai_cutover predecessor profiles: %w", errManagedTenantSchemaMigration, err)
		}
		for _, record := range predecessor {
			profiles = append(profiles, managedProviderProfileRecord{TenantID: record.TenantID, ProviderID: record.ProviderID, TextModel: record.TextModel})
		}
	}
	for _, profile := range profiles {
		if target, changed := openAICutoverTarget(providers, profile.ProviderID, profile.TextModel); changed {
			tenant, exists := byID[profile.TenantID]
			if !exists {
				return plan, fmt.Errorf("%w: operation=preflight_openai_cutover orphan_tenant=%s", errManagedTenantSchemaMigration, profile.TenantID)
			}
			if err := validate(tenant, profile.TextModel, target, "provider_profile"); err != nil {
				return plan, err
			}
			plan.profiles = append(plan.profiles, openAICutoverSelection{profile.TenantID, profile.TextModel, target})
		}
	}
	affected := len(plan.tenants)+len(plan.profiles) > 0
	if database.Migrator().HasTable(&managedSchemaMigrationRecord{}) {
		var marker managedSchemaMigrationRecord
		err := database.Where(&managedSchemaMigrationRecord{Version: managedOpenAIGPT56CutoverVersion}).First(&marker).Error
		if err == nil {
			if affected {
				return plan, fmt.Errorf("%w: operation=preflight_openai_cutover reason=retired_selection_after_cutover", errManagedTenantSchemaMigration)
			}
			return plan, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return plan, fmt.Errorf("%w: operation=preflight_openai_cutover marker: %w", errManagedTenantSchemaMigration, err)
		}
		var predecessorCount int64
		if err := database.Model(&managedSchemaMigrationRecord{}).Where(&managedSchemaMigrationRecord{Version: managedAccountConnectionsSchemaVersion}).Count(&predecessorCount).Error; err != nil {
			return plan, err
		}
		plan.record = predecessorCount > 0 || affected
	} else {
		plan.record = affected
	}
	return plan, nil
}

// applyOpenAICutover changes only model columns. Timestamp, effort, key, prompt,
// assignment and historical usage records retain their exact previous values.
func applyOpenAICutover(database *gorm.DB, plan openAICutoverPlan) error {
	for _, selection := range plan.profiles {
		var profile managedProviderProfileRecord
		if err := database.Where(&managedProviderProfileRecord{TenantID: selection.tenantID, ProviderID: ProviderNameOpenAI}).First(&profile).Error; err != nil {
			return fmt.Errorf("%w: operation=apply_openai_cutover tenant=%s scope=profile: %w", errManagedTenantSchemaMigration, selection.tenantID, err)
		}
		if profile.TextModel == selection.target {
			continue
		}
		result := database.Model(&managedProviderProfileRecord{}).Where(&managedProviderProfileRecord{TenantID: selection.tenantID, ProviderID: ProviderNameOpenAI, TextModel: selection.source}).UpdateColumn("text_model", selection.target)
		if result.Error != nil || result.RowsAffected != 1 {
			return fmt.Errorf("%w: operation=apply_openai_cutover tenant=%s source_model=%s scope=profile rows=%d: %v", errManagedTenantSchemaMigration, selection.tenantID, selection.source, result.RowsAffected, result.Error)
		}
	}
	for _, selection := range plan.tenants {
		var tenant managedTenantRecord
		if err := database.Where(&managedTenantRecord{TenantID: selection.tenantID}).First(&tenant).Error; err != nil {
			return fmt.Errorf("%w: operation=apply_openai_cutover tenant=%s scope=default: %w", errManagedTenantSchemaMigration, selection.tenantID, err)
		}
		if tenant.DefaultModel == selection.target {
			continue
		}
		result := database.Model(&managedTenantRecord{}).Where(&managedTenantRecord{TenantID: selection.tenantID, DefaultProvider: ProviderNameOpenAI, DefaultModel: selection.source}).UpdateColumn("default_model", selection.target)
		if result.Error != nil || result.RowsAffected != 1 {
			return fmt.Errorf("%w: operation=apply_openai_cutover tenant=%s source_model=%s scope=default rows=%d: %v", errManagedTenantSchemaMigration, selection.tenantID, selection.source, result.RowsAffected, result.Error)
		}
	}
	if !plan.record {
		return nil
	}
	if err := database.AutoMigrate(&managedSchemaMigrationRecord{}); err != nil {
		return fmt.Errorf("%w: operation=apply_openai_cutover marker schema: %w", errManagedTenantSchemaMigration, err)
	}
	if err := database.Clauses(clause.OnConflict{DoNothing: true}).Create(&managedSchemaMigrationRecord{Version: managedOpenAIGPT56CutoverVersion, AppliedAt: time.Now().UTC()}).Error; err != nil {
		return fmt.Errorf("%w: operation=apply_openai_cutover marker: %w", errManagedTenantSchemaMigration, err)
	}
	return nil
}
