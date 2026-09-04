package proxy

import (
	"testing"

	"gorm.io/gorm"
)

func managedUsageSchemaTwelveRecord(record managedUsageEventRecord) managedUsageEventSchemaTwelveRecord {
	return managedUsageEventSchemaTwelveRecord{
		ID: record.ID, TenantID: record.TenantID, Endpoint: record.Endpoint,
		ProviderID: record.ProviderID, ModelID: record.ModelID, StatusCode: record.StatusCode,
		Success: record.Disposition == managedUsageDispositionSucceeded, OutcomeCode: record.OutcomeCode,
		LatencyMilliseconds: record.LatencyMilliseconds,
		RequestTokens:       record.RequestTokens, ResponseTokens: record.ResponseTokens,
		TotalTokens: record.TotalTokens, CreatedAt: record.CreatedAt,
	}
}

func useManagedUsageSchemaTwelve(t *testing.T, database *gorm.DB) {
	t.Helper()
	if dropError := database.Migrator().DropTable(&managedUsageEventRecord{}); dropError != nil {
		t.Fatalf("drop current usage table: %v", dropError)
	}
	if migrationError := database.AutoMigrate(&managedUsageEventSchemaTwelveRecord{}); migrationError != nil {
		t.Fatalf("create schema-twelve usage table: %v", migrationError)
	}
}

func createManagedUsageSchemaTwelve(t *testing.T, database *gorm.DB, records ...managedUsageEventRecord) {
	t.Helper()
	schemaTwelveRecords := make([]managedUsageEventSchemaTwelveRecord, 0, len(records))
	for _, record := range records {
		schemaTwelveRecords = append(schemaTwelveRecords, managedUsageSchemaTwelveRecord(record))
	}
	if len(schemaTwelveRecords) != 0 {
		if createError := database.Create(&schemaTwelveRecords).Error; createError != nil {
			t.Fatalf("seed schema-twelve usage: %v", createError)
		}
	}
}

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
