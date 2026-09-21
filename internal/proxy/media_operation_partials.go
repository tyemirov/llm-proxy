package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type mediaOperationPartialReferenceRecord struct {
	OperationID    string    `gorm:"primaryKey"`
	OutputOrdinal  int       `gorm:"primaryKey;autoIncrement:false"`
	PartialOrdinal int       `gorm:"primaryKey;autoIncrement:false"`
	TenantID       string    `gorm:"not null;index:idx_media_partial_asset,priority:1"`
	AssetID        string    `gorm:"not null;index:idx_media_partial_asset,priority:2"`
	MIMEType       string    `gorm:"not null"`
	SizeBytes      int64     `gorm:"not null"`
	ContentSHA256  string    `gorm:"not null"`
	CreatedAt      time.Time `gorm:"not null"`
}

type mediaOperationPartialResponse struct {
	AssetID        string `json:"asset_id"`
	MIMEType       string `json:"mime_type"`
	SizeBytes      int64  `json:"size_bytes"`
	OutputOrdinal  int    `json:"output_ordinal"`
	PartialOrdinal int    `json:"partial_ordinal"`
}

func (store *mediaOperationStore) partialResponses(ctx context.Context, operationID string) ([]mediaOperationPartialResponse, error) {
	var records []mediaOperationPartialReferenceRecord
	err := store.database.WithContext(ctx).Where("operation_id = ?", operationID).Order("output_ordinal ASC, partial_ordinal ASC").Find(&records).Error
	if err != nil {
		return nil, err
	}
	outputs := make([]mediaOperationPartialResponse, 0, len(records))
	for _, record := range records {
		outputs = append(outputs, mediaOperationPartialResponse{AssetID: record.AssetID, MIMEType: record.MIMEType, SizeBytes: record.SizeBytes, OutputOrdinal: record.OutputOrdinal, PartialOrdinal: record.PartialOrdinal})
	}
	return outputs, nil
}

func (service *mediaOperationService) publishPartial(operationID string, generation uint64, output MediaOperationPartialOutput) error {
	if output.OutputOrdinal < 0 || output.PartialOrdinal < 0 || !service.validOutputs([]MediaOperationOutput{output.MediaOperationOutput}, nil) {
		return errMediaOperationStore
	}
	digest := sha256.Sum256(output.Data)
	contentDigest := hex.EncodeToString(digest[:])
	service.assets.referenceMutex.Lock()
	defer service.assets.referenceMutex.Unlock()
	return service.store.database.Transaction(func(transaction *gorm.DB) error {
		now := service.store.now()
		var claim mediaOperationClaimRecord
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&claim, "operation_id = ? AND generation = ? AND expires_at > ?", operationID, generation, now).Error; err != nil {
			return err
		}
		var operation mediaOperationRecord
		if err := transaction.First(&operation, "operation_id = ? AND public_state = ? AND provider_execution_state = ?", operationID, MediaOperationStateRunning, MediaProviderExecutionDispatched).Error; err != nil {
			return err
		}
		var existing mediaOperationPartialReferenceRecord
		err := transaction.First(&existing, "operation_id = ? AND output_ordinal = ? AND partial_ordinal = ?", operationID, output.OutputOrdinal, output.PartialOrdinal).Error
		if err == nil {
			if existing.ContentSHA256 != contentDigest || existing.MIMEType != output.MIMEType {
				return errMediaOperationStore
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		asset, err := service.assets.upload(tenant{identifier: tenantID(operation.TenantID)}, output.MIMEType, bytes.NewReader(output.Data))
		if err != nil {
			return err
		}
		reference := mediaOperationPartialReferenceRecord{
			OperationID: operationID, OutputOrdinal: output.OutputOrdinal, PartialOrdinal: output.PartialOrdinal,
			TenantID: operation.TenantID, AssetID: asset.AssetID, MIMEType: asset.MIMEType, SizeBytes: asset.SizeBytes, ContentSHA256: contentDigest, CreatedAt: now,
		}
		if err := transaction.Create(&reference).Error; err != nil {
			return err
		}
		return transaction.Model(&operation).Update("updated_at", now).Error
	})
}
