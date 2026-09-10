package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const managedAccountConnectionsSchemaVersion = 17

var (
	errManagedConnectionNotFound = errors.New("managed_connection_not_found")
	errManagedConnectionConflict = errors.New("managed_connection_conflict")
	errManagedConnectionAssigned = errors.New("managed_connection_assigned")
	errManagedConnectionInvalid  = errors.New("managed_connection_invalid")
)

// managedAccountConnectionRecord owns credentials independently of tenant assignments.
type managedAccountConnectionRecord struct {
	ID          string                          `gorm:"primaryKey"`
	OwnerUserID string                          `gorm:"not null;index"`
	Owner       managedUserRecord               `gorm:"foreignKey:OwnerUserID;references:UserID;constraint:OnDelete:CASCADE"`
	ProviderID  string                          `gorm:"not null"`
	Name        string                          `gorm:"not null"`
	Version     uint64                          `gorm:"not null"`
	Fields      []managedConnectionFieldRecord  `gorm:"foreignKey:ConnectionID;references:ID;constraint:OnDelete:CASCADE"`
	Assignments []managedTenantConnectionRecord `gorm:"foreignKey:ConnectionID;references:ID;constraint:OnDelete:RESTRICT"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type managedConnectionFieldRecord struct {
	ConnectionID string `gorm:"primaryKey"`
	FieldID      string `gorm:"primaryKey"`
	Value        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type managedTenantConnectionRecord struct {
	TenantID     string                         `gorm:"primaryKey"`
	ProviderID   string                         `gorm:"primaryKey"`
	ConnectionID string                         `gorm:"not null;index"`
	Connection   managedAccountConnectionRecord `gorm:"foreignKey:ConnectionID;references:ID;constraint:OnDelete:RESTRICT"`
	CreatedAt    time.Time
}

func newManagedConnectionID(reader io.Reader) (string, error) {
	bytes := make([]byte, 16)
	if _, err := io.ReadFull(reader, bytes); err != nil {
		return "", fmt.Errorf("connection identifier: %w", err)
	}
	return "connection-" + hex.EncodeToString(bytes), nil
}

func migrateAccountConnections(database *gorm.DB, keyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	if err := database.AutoMigrate(&managedAccountConnectionRecord{}, &managedConnectionFieldRecord{}, &managedTenantConnectionRecord{}, &managedConnectionCreationRecord{}); err != nil {
		return fmt.Errorf("create account connection schema: %w", err)
	}
	return migrateAccountConnectionRecords(database, keyCipher, providers, rand.Reader)
}

func migrateAccountConnectionRecords(database *gorm.DB, keyCipher managedProviderKeyCipher, providers *providerRegistry, entropy io.Reader) error {
	var tenants []managedTenantRecord
	if err := database.Preload("ProviderConnections").Preload("ProviderProfiles").Order("tenant_id").Find(&tenants).Error; err != nil {
		return fmt.Errorf("read predecessor connections: %w", err)
	}
	for _, tenant := range tenants {
		fieldsByProvider := map[string][]managedProviderConnectionRecord{}
		for _, field := range tenant.ProviderConnections {
			fieldsByProvider[field.ProviderID] = append(fieldsByProvider[field.ProviderID], field)
		}
		for _, profile := range tenant.ProviderProfiles {
			fields := fieldsByProvider[profile.ProviderID]
			id, err := newManagedConnectionID(entropy)
			if err != nil {
				return err
			}
			connection := managedAccountConnectionRecord{ID: id, OwnerUserID: tenant.OwnerUserID, ProviderID: profile.ProviderID, Name: tenant.Name, Version: 1, CreatedAt: profile.CreatedAt, UpdatedAt: profile.UpdatedAt}
			for _, field := range fields {
				value := field.Value
				if providers.definitions[providerID(profile.ProviderID)].fields[field.FieldID].Secret {
					value, err = keyCipher.decryptConnection(field)
					if err != nil {
						return err
					}
					value, err = keyCipher.encryptConnection(entropy, id, profile.ProviderID, field.FieldID, value)
					if err != nil {
						return err
					}
				}
				connection.Fields = append(connection.Fields, managedConnectionFieldRecord{ConnectionID: id, FieldID: field.FieldID, Value: value, CreatedAt: field.CreatedAt, UpdatedAt: field.UpdatedAt})
			}
			if err := database.Omit("Owner", "Assignments").Create(&connection).Error; err != nil {
				return fmt.Errorf("migrate tenant connection: %w", err)
			}
			assignment := managedTenantConnectionRecord{TenantID: tenant.TenantID, ProviderID: profile.ProviderID, ConnectionID: id, CreatedAt: profile.CreatedAt}
			if err := database.Omit("Connection").Create(&assignment).Error; err != nil {
				return fmt.Errorf("migrate tenant assignment: %w", err)
			}
		}
	}
	if err := database.Migrator().DropTable(&managedProviderConnectionRecord{}); err != nil {
		return fmt.Errorf("remove predecessor connection fields: %w", err)
	}
	return nil
}

func (database *gormManagedTenantDatabase) accountConnections(ctx context.Context, owner string, page managedConnectionPage) ([]managedAccountConnectionRecord, error) {
	records := []managedAccountConnectionRecord{}
	err := database.database.WithContext(ctx).Preload("Fields").Preload("Assignments").Where("owner_user_id = ? AND id > ?", owner, page.after).Order("id").Limit(page.limit + 1).Find(&records).Error
	return records, err
}

func (database *gormManagedTenantDatabase) accountConnection(ctx context.Context, owner, id string) (managedAccountConnectionRecord, error) {
	var record managedAccountConnectionRecord
	err := database.database.WithContext(ctx).Preload("Fields").Preload("Assignments").Where("owner_user_id = ? AND id = ?", owner, id).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, errManagedConnectionNotFound
	}
	return record, err
}

func (database *gormManagedTenantDatabase) saveAccountConnection(ctx context.Context, record managedAccountConnectionRecord, expected uint64) error {
	return database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&managedAccountConnectionRecord{}).Where("id = ? AND owner_user_id = ? AND version = ?", record.ID, record.OwnerUserID, expected).Updates(map[string]any{"name": record.Name, "version": record.Version, "updated_at": record.UpdatedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errManagedConnectionConflict
		}
		if err := tx.Where("connection_id = ?", record.ID).Delete(&managedConnectionFieldRecord{}).Error; err != nil {
			return err
		}
		if len(record.Fields) > 0 {
			return tx.Create(&record.Fields).Error
		}
		return nil
	})
}

func (database *gormManagedTenantDatabase) deleteAccountConnection(ctx context.Context, owner, id string) error {
	return database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record managedAccountConnectionRecord
		if err := tx.Where("owner_user_id = ? AND id = ?", owner, id).First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errManagedConnectionNotFound
			}
			return err
		}
		var count int64
		if err := tx.Model(&managedTenantConnectionRecord{}).Where("connection_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errManagedConnectionAssigned
		}
		if err := tx.Where("connection_id = ?", id).Delete(&managedConnectionFieldRecord{}).Error; err != nil {
			return err
		}
		return tx.Delete(&record).Error
	})
}

func (database *gormManagedTenantDatabase) assignAccountConnection(ctx context.Context, owner, tenantID, providerID, connectionID, textModel string, now time.Time) error {
	return database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant managedTenantRecord
		if err := tx.Where("owner_user_id = ? AND tenant_id = ?", owner, tenantID).First(&tenant).Error; err != nil {
			return managedTenantQueryError(owner, tenantID, err)
		}
		var connection managedAccountConnectionRecord
		if err := tx.Where("owner_user_id = ? AND id = ? AND provider_id = ?", owner, connectionID, providerID).First(&connection).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errManagedConnectionNotFound
			}
			return err
		}
		var existing managedTenantConnectionRecord
		err := tx.Where("tenant_id = ? AND provider_id = ?", tenantID, providerID).First(&existing).Error
		if err == nil {
			if existing.ConnectionID == connectionID {
				return nil
			}
			return errManagedConnectionConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		assignment := managedTenantConnectionRecord{TenantID: tenantID, ProviderID: providerID, ConnectionID: connectionID, CreatedAt: now}
		if err := tx.Omit("Connection").Create(&assignment).Error; err != nil {
			return err
		}
		profile := managedProviderProfileRecord{TenantID: tenantID, ProviderID: providerID, TextModel: textModel, CreatedAt: now, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&profile).Error; err != nil {
			return err
		}
		return advanceManagedConnectionVersion(tx, connectionID, now)
	})
}

func advanceManagedConnectionVersion(tx *gorm.DB, connectionID string, now time.Time) error {
	return tx.Model(&managedAccountConnectionRecord{}).Where("id = ?", connectionID).
		Updates(map[string]any{"version": gorm.Expr("version + 1"), "updated_at": now}).Error
}

func (database *gormManagedTenantDatabase) detachAccountConnection(ctx context.Context, owner, tenantID, providerID string, clearDefaults bool, now time.Time) error {
	return database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant managedTenantRecord
		if err := tx.Where("owner_user_id = ? AND tenant_id = ?", owner, tenantID).First(&tenant).Error; err != nil {
			return managedTenantQueryError(owner, tenantID, err)
		}
		var assignment managedTenantConnectionRecord
		if err := tx.Where("tenant_id = ? AND provider_id = ?", tenantID, providerID).First(&assignment).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		affected := tenant.DefaultProvider == providerID || tenant.DefaultDictationProvider == providerID
		if affected && !clearDefaults {
			return errManagedConnectionConflict
		}
		if tenant.DefaultProvider == providerID {
			tenant.DefaultProvider = ""
			tenant.DefaultModel = ""
			tenant.DefaultReasoningEffort = ""
		}
		if tenant.DefaultDictationProvider == providerID {
			tenant.DefaultDictationProvider = ""
			tenant.DefaultDictationModel = ""
		}
		if err := tx.Where("tenant_id = ? AND provider_id = ?", tenantID, providerID).Delete(&managedTenantConnectionRecord{}).Error; err != nil {
			return err
		}
		if err := advanceManagedConnectionVersion(tx, assignment.ConnectionID, now); err != nil {
			return err
		}
		return tx.Model(&managedTenantRecord{}).Where("tenant_id = ?", tenantID).Updates(map[string]any{"default_provider": tenant.DefaultProvider, "default_model": tenant.DefaultModel, "default_reasoning_effort": tenant.DefaultReasoningEffort, "default_dictation_provider": tenant.DefaultDictationProvider, "default_dictation_model": tenant.DefaultDictationModel, "updated_at": now}).Error
	})
}

func (store *managedTenantStore) accountConnectionSettings(record managedAccountConnectionRecord) (managedProviderSettings, error) {
	definition, exists := store.routingDefaults.definitions[providerID(record.ProviderID)]
	if !exists {
		return managedProviderSettings{}, errManagedConnectionInvalid
	}
	settings := managedProviderSettings{connectionValues: map[string]string{}, configuredFields: map[string]bool{}, textModel: definition.defaultTextModel.string()}
	for id, field := range definition.fields {
		settings.connectionValues[id] = *field.Default
	}
	for _, field := range record.Fields {
		definitionField, known := definition.fields[field.FieldID]
		if !known {
			return managedProviderSettings{}, errManagedConnectionInvalid
		}
		value := field.Value
		if definitionField.Secret {
			var err error
			value, err = store.providerKeyCipher.decryptConnectionValue(record.ID, record.ProviderID, field.FieldID, value)
			if err != nil {
				return managedProviderSettings{}, err
			}
		}
		value, err := validatedProviderFieldValue(definitionField, value)
		if err != nil {
			return managedProviderSettings{}, err
		}
		settings.connectionValues[field.FieldID] = value
		settings.configuredFields[field.FieldID] = true
	}
	return settings, nil
}

func (store *managedTenantStore) assignedProviderSettings(record managedTenantRecord) (map[providerID]managedProviderSettings, error) {
	settings := map[providerID]managedProviderSettings{}
	for _, profile := range record.ProviderProfiles {
		settings[providerID(profile.ProviderID)] = managedProviderSettings{textModel: profile.TextModel, systemPrompt: profile.SystemPrompt}
	}
	for _, assignment := range record.ConnectionAssignments {
		value, err := store.accountConnectionSettings(assignment.Connection)
		if err != nil {
			return nil, err
		}
		profile, exists := managedProviderProfileRecordForProvider(record.ProviderProfiles, providerID(assignment.ProviderID))
		if !exists {
			return nil, fmt.Errorf("tenant=%s provider=%s profile missing: %w", record.TenantID, assignment.ProviderID, errManagedConnectionInvalid)
		}
		value.textModel = profile.TextModel
		value.systemPrompt = profile.SystemPrompt
		settings[providerID(assignment.ProviderID)] = value
	}
	return settings, nil
}

func (database *gormManagedTenantDatabase) saveTenantProviderProfile(ctx context.Context, owner string, profile managedProviderProfileRecord) error {
	return database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant managedTenantRecord
		if err := tx.Where("owner_user_id = ? AND tenant_id = ?", owner, profile.TenantID).First(&tenant).Error; err != nil {
			return managedTenantQueryError(owner, profile.TenantID, err)
		}
		var count int64
		if err := tx.Model(&managedTenantConnectionRecord{}).Where("tenant_id = ? AND provider_id = ?", profile.TenantID, profile.ProviderID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return errManagedConnectionNotFound
		}
		result := tx.Model(&managedProviderProfileRecord{}).Where("tenant_id = ? AND provider_id = ?", profile.TenantID, profile.ProviderID).Updates(map[string]any{"text_model": profile.TextModel, "system_prompt": profile.SystemPrompt, "updated_at": profile.UpdatedAt})
		return result.Error
	})
}

func validateAccountConnectionSchema(database *gorm.DB, keyCipher managedProviderKeyCipher, providers *providerRegistry) error {
	for _, model := range []any{&managedAccountConnectionRecord{}, &managedConnectionFieldRecord{}, &managedTenantConnectionRecord{}, &managedConnectionCreationRecord{}} {
		if !database.Migrator().HasTable(model) {
			return fmt.Errorf("%w: account connection table missing", errManagedTenantSchemaMigration)
		}
	}
	if database.Migrator().HasTable(managedProviderConnectionTable) {
		return fmt.Errorf("%w: predecessor connection table remains", errManagedTenantSchemaMigration)
	}
	if err := validateManagedUsageDispositionSchema(database); err != nil {
		return err
	}
	var connections []managedAccountConnectionRecord
	if err := database.Preload("Fields").Preload("Owner").Find(&connections).Error; err != nil {
		return fmt.Errorf("%w: operation=validate account connections: %w", errManagedTenantSchemaMigration, err)
	}
	store := managedTenantStore{providerKeyCipher: keyCipher, routingDefaults: providers}
	settingsByConnection := make(map[string]managedProviderSettings, len(connections))
	connectionsByID := make(map[string]managedAccountConnectionRecord, len(connections))
	for _, connection := range connections {
		name, nameError := newManagedTenantName(connection.Name)
		if nameError != nil || name.display != connection.Name || connection.Version == 0 || connection.Owner.UserID != connection.OwnerUserID || connection.OwnerUserID == "" {
			return fmt.Errorf("%w: operation=validate connection=%s: %w", errManagedTenantSchemaMigration, connection.ID, errManagedConnectionInvalid)
		}
		settings, err := store.accountConnectionSettings(connection)
		if err != nil {
			return fmt.Errorf("%w: operation=validate connection=%s: %w", errManagedTenantSchemaMigration, connection.ID, err)
		}
		settingsByConnection[connection.ID] = settings
		connectionsByID[connection.ID] = connection
	}
	var tenants []managedTenantRecord
	if err := database.Preload("ConnectionAssignments").Preload("ProviderProfiles").Find(&tenants).Error; err != nil {
		return err
	}
	for _, tenant := range tenants {
		settings := make(map[providerID]managedProviderSettings, len(tenant.ConnectionAssignments))
		for _, assignment := range tenant.ConnectionAssignments {
			connection, exists := connectionsByID[assignment.ConnectionID]
			profile, profileExists := managedProviderProfileRecordForProvider(tenant.ProviderProfiles, providerID(assignment.ProviderID))
			if !exists || !profileExists || connection.OwnerUserID != tenant.OwnerUserID || connection.ProviderID != assignment.ProviderID {
				return fmt.Errorf("%w: operation=validate tenant=%s connection=%s invalid assignment", errManagedTenantSchemaMigration, tenant.TenantID, assignment.ConnectionID)
			}
			value := settingsByConnection[assignment.ConnectionID]
			value.textModel = profile.TextModel
			value.systemPrompt = profile.SystemPrompt
			settings[providerID(assignment.ProviderID)] = value
		}
		for _, profile := range tenant.ProviderProfiles {
			if _, _, err := providers.resolveTextModel(profile.ProviderID, profile.TextModel, profile.ProviderID, profile.TextModel, false); err != nil {
				return fmt.Errorf("%w: operation=validate table=%s owner=%s tenant=%s: %w", errManagedTenantSchemaMigration, managedProviderProfileTable, tenant.OwnerUserID, tenant.TenantID, err)
			}
		}
		if _, err := validatePersistedManagedRoutingDefaults(providers, settings, tenant.defaults()); err != nil {
			return fmt.Errorf("%w: operation=validate table=%s owner=%s tenant=%s: %w", errManagedTenantSchemaMigration, managedTenantTable, tenant.OwnerUserID, tenant.TenantID, err)
		}
	}
	return nil
}
