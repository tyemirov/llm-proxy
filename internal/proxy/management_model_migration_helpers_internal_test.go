package proxy

var (
	retiredQwenCloudModelIdentifier       = internalManagedModelMigrationSource(managedQwenCloudRetirementVersion, retiredQwenCloudProviderIdentifier, ModelOperationText)
	managedMiniMaxNativeModel             = internalManagedModelMigrationSource(managedModelIdentitySchemaVersion, ProviderNameMiniMax, ModelOperationText)
	managedSiliconFlowDeepSeekNativeModel = internalManagedModelMigrationSource(managedModelIdentitySchemaVersion, ProviderNameSiliconFlow, ModelOperationText)
	managedSenseVoiceNativeModel          = internalManagedModelMigrationSource(managedModelIdentitySchemaVersion, ProviderNameSiliconFlow, ModelOperationDictation)
)

func internalManagedModelMigrationSource(schemaVersion int, provider string, operation string) string {
	migrations := internalManagementProviderRegistry().modelMigrationsFor(schemaVersion, provider, operation)
	if len(migrations) != 1 {
		panic("expected exactly one managed model migration")
	}
	return migrations[0].source
}
