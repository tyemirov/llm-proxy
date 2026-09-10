package proxy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const managementIdempotencyHeader = "Idempotency-Key"

// A successful creation receipt belongs to the account. It remains after the
// connection is deleted so that a delayed retry cannot recreate the resource.
type managedConnectionCreationRecord struct {
	OwnerUserID  string            `gorm:"primaryKey"`
	KeyDigest    string            `gorm:"primaryKey"`
	Owner        managedUserRecord `gorm:"foreignKey:OwnerUserID;references:UserID;constraint:OnDelete:CASCADE"`
	RequestMAC   string            `gorm:"not null"`
	ConnectionID string            `gorm:"not null"`
	Response     []byte            `gorm:"not null"`
	CreatedAt    time.Time
}

func (service *managementService) connectionCreationIntent(ctx *gin.Context, request managementConnectionRequest) (managedConnectionCreationRecord, error) {
	keys := ctx.Request.Header.Values(managementIdempotencyHeader)
	if len(keys) != 1 || !validIdempotencyKey(keys[0]) {
		return managedConnectionCreationRecord{}, errManagedConnectionInvalid
	}
	keyDigest := sha256.Sum256([]byte(keys[0]))
	requestJSON, _ := json.Marshal(request)
	mac := hmac.New(sha256.New, []byte(service.configuration.ProviderKeyEncryptionKey))
	_, _ = mac.Write(requestJSON)
	return managedConnectionCreationRecord{
		OwnerUserID: managementPrincipalFromContext(ctx).userID,
		KeyDigest:   hex.EncodeToString(keyDigest[:]),
		RequestMAC:  hex.EncodeToString(mac.Sum(nil)),
	}, nil
}

func (database *gormManagedTenantDatabase) connectionCreation(ctx context.Context, owner, keyDigest string) (managedConnectionCreationRecord, error) {
	var record managedConnectionCreationRecord
	err := database.database.WithContext(ctx).Where("owner_user_id = ? AND key_digest = ?", owner, keyDigest).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, errManagedConnectionNotFound
	}
	return record, err
}

func (database *gormManagedTenantDatabase) createAccountConnection(ctx context.Context, connection managedAccountConnectionRecord, intent managedConnectionCreationRecord) (managedConnectionCreationRecord, error) {
	var receipt managedConnectionCreationRecord
	err := database.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		queryError := tx.Where("owner_user_id = ? AND key_digest = ?", intent.OwnerUserID, intent.KeyDigest).First(&receipt).Error
		if queryError == nil {
			if receipt.RequestMAC != intent.RequestMAC {
				return errManagedConnectionConflict
			}
			return nil
		}
		if !errors.Is(queryError, gorm.ErrRecordNotFound) {
			return queryError
		}
		if err := tx.Omit("Owner", "Assignments").Create(&connection).Error; err != nil {
			return err
		}
		if err := tx.Omit("Owner").Create(&intent).Error; err != nil {
			return err
		}
		receipt = intent
		return nil
	})
	return receipt, err
}

func writeConnectionCreation(ctx *gin.Context, receipt managedConnectionCreationRecord) {
	ctx.Header("Location", managementAPIPath+managementConnectionsPath+"/"+receipt.ConnectionID)
	ctx.Header(headerCacheControl, cacheControlNoStore)
	ctx.Data(http.StatusCreated, "application/json; charset=utf-8", receipt.Response)
}
