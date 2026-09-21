package proxy

import (
	"fmt"

	"gorm.io/gorm"
)

const (
	managedTranscriptionProviderColumn = "default_transcription_provider"
	managedTranscriptionModelColumn    = "default_transcription_model"
	managedSpeechProviderColumn        = "default_speech_provider"
	managedSpeechModelColumn           = "default_speech_model"
	predecessorDictationProviderColumn = "default_dictation_provider"
	predecessorDictationModelColumn    = "default_dictation_model"
)

// migrateManagedCapabilityDefaults transfers the retained pre-capability shape once.
// The caller owns the transaction, including subsequent connection and route validation.
// Remove this bridge after retained databases have transfer receipts (I271).
func migrateManagedCapabilityDefaults(database *gorm.DB) error {
	migrator := database.Migrator()
	types, err := migrator.ColumnTypes(managedTenantTable)
	if err != nil {
		return fmt.Errorf("%w: operation=inspect_capability_defaults: %w", errManagedTenantSchemaMigration, err)
	}
	columns := make(map[string]bool, len(types))
	for _, column := range types {
		columns[column.Name()] = true
	}
	// The separate ownership transfer reads its own predecessor record shape.
	if !columns["owner_user_id"] || (!columns[predecessorDictationProviderColumn] && !columns[predecessorDictationModelColumn]) {
		return nil
	}
	transfers := []struct{ source, target string }{
		{predecessorDictationProviderColumn, managedTranscriptionProviderColumn},
		{predecessorDictationModelColumn, managedTranscriptionModelColumn},
		{"", managedSpeechProviderColumn},
		{"", managedSpeechModelColumn},
	}
	for _, transfer := range transfers {
		if columns[transfer.target] || (transfer.source != "" && !columns[transfer.source]) {
			return fmt.Errorf("%w: operation=transfer_capability_defaults ambiguous_default_column=%s", errManagedTenantSchemaMigration, transfer.target)
		}
	}
	for _, transfer := range transfers {
		if transfer.source != "" {
			err = migrator.RenameColumn(managedTenantTable, transfer.source, transfer.target)
		} else {
			err = migrator.AddColumn(&managedTenantRecord{}, transfer.target)
		}
		if err != nil {
			return fmt.Errorf("%w: operation=transfer_capability_defaults column=%s: %w", errManagedTenantSchemaMigration, transfer.target, err)
		}
	}
	// UpdateColumns leaves the retained tenant timestamps unchanged.
	if err := database.Model(&managedTenantRecord{}).Where("1 = 1").UpdateColumns(map[string]any{
		managedSpeechProviderColumn: "", managedSpeechModelColumn: "",
	}).Error; err != nil {
		return fmt.Errorf("%w: operation=initialize_speech_defaults: %w", errManagedTenantSchemaMigration, err)
	}
	return nil
}
