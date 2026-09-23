package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	managementBillingAccountsPath = "/billing-accounts"
	managementBillingAccountPath  = managementBillingAccountsPath + "/:billing_account_id"
	billingAccountIDPrefix        = "billing-"
	hostedResourceIDBytes         = 16
)

var (
	errBillingAccountInvalid  = errors.New("billing_account_invalid")
	errBillingAccountConflict = errors.New("billing_account_conflict")
	errBillingAccountNotFound = errors.New("billing_account_not_found")
)

// Billing accounts retain their ownership independently of tenant lifetimes.
type managedBillingAccountRecord struct {
	ID                string            `gorm:"primaryKey"`
	OwnerUserID       string            `gorm:"not null;uniqueIndex"`
	Owner             managedUserRecord `gorm:"foreignKey:OwnerUserID;references:UserID;constraint:OnDelete:RESTRICT"`
	Currency          string            `gorm:"not null;check:currency = 'USD'"`
	CreationKeyDigest string            `gorm:"not null"`
	CreatedAt         time.Time         `gorm:"not null"`
}

type managementBillingAccountRequest struct {
	Currency string `json:"currency"`
}

type managementBillingAccountResponse struct {
	ID        string `json:"id"`
	Currency  string `json:"currency"`
	CreatedAt string `json:"created_at"`
}

type managementBillingAccountsResponse struct {
	BillingAccounts []managementBillingAccountResponse `json:"billing_accounts"`
}

func newHostedResourceID(prefix string, entropy io.Reader) (string, error) {
	identifier := make([]byte, hostedResourceIDBytes)
	if _, err := io.ReadFull(entropy, identifier); err != nil {
		return "", fmt.Errorf("create %s identifier: %w", prefix, err)
	}
	return prefix + hex.EncodeToString(identifier), nil
}

func hostedCreationKey(ctx *gin.Context) (string, error) {
	keys := ctx.Request.Header.Values(managementIdempotencyHeader)
	if len(keys) != 1 || !validIdempotencyKey(keys[0]) {
		return "", errBillingAccountInvalid
	}
	digest := sha256.Sum256([]byte(keys[0]))
	return hex.EncodeToString(digest[:]), nil
}

func billingAccountResponse(record managedBillingAccountRecord) managementBillingAccountResponse {
	return managementBillingAccountResponse{ID: record.ID, Currency: record.Currency, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano)}
}

func (database *gormManagedTenantDatabase) billingAccount(ctx context.Context, owner string) (managedBillingAccountRecord, error) {
	var record managedBillingAccountRecord
	err := database.database.WithContext(ctx).Where("owner_user_id = ?", owner).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, errBillingAccountNotFound
	}
	if err != nil {
		return record, fmt.Errorf("read billing account: %w", err)
	}
	return record, nil
}

func (database *gormManagedTenantDatabase) createBillingAccount(ctx context.Context, proposed managedBillingAccountRecord) (managedBillingAccountRecord, error) {
	var record managedBillingAccountRecord
	err := database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		// The unique owner constraint arbitrates admission across service instances.
		if err := transaction.Omit("Owner").Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "owner_user_id"}}, DoNothing: true,
		}).Create(&proposed).Error; err != nil {
			return fmt.Errorf("create billing account: %w", err)
		}
		if err := transaction.Where("owner_user_id = ?", proposed.OwnerUserID).First(&record).Error; err != nil {
			return fmt.Errorf("read created billing account: %w", err)
		}
		if record.CreationKeyDigest != proposed.CreationKeyDigest || record.Currency != proposed.Currency {
			return errBillingAccountConflict
		}
		return nil
	})
	return record, err
}

func (store *managedTenantStore) createBillingAccount(ctx context.Context, principal managementPrincipal, key string) (managedBillingAccountRecord, error) {
	if err := store.mutex.LockContext(ctx); err != nil {
		return managedBillingAccountRecord{}, err
	}
	defer store.mutex.Unlock()
	if _, err := store.ensureUserLocked(principal); err != nil {
		return managedBillingAccountRecord{}, err
	}
	record, err := store.database.billingAccount(ctx, principal.userID)
	if err == nil {
		if record.CreationKeyDigest != key {
			return managedBillingAccountRecord{}, errBillingAccountConflict
		}
		return record, nil
	}
	if !errors.Is(err, errBillingAccountNotFound) {
		return managedBillingAccountRecord{}, err
	}
	identifier, err := newHostedResourceID(billingAccountIDPrefix, store.randomReader)
	if err != nil {
		return managedBillingAccountRecord{}, err
	}
	return store.database.createBillingAccount(ctx, managedBillingAccountRecord{
		ID: identifier, OwnerUserID: principal.userID, Currency: CatalogCurrencyUSD,
		CreationKeyDigest: key, CreatedAt: store.now().UTC(),
	})
}

func writeBillingAccountError(ctx *gin.Context, err error) {
	status, code := http.StatusInternalServerError, "billing_account_store_failed"
	switch {
	case errors.Is(err, errBillingAccountInvalid):
		status, code = http.StatusBadRequest, errBillingAccountInvalid.Error()
	case errors.Is(err, errBillingAccountConflict):
		status, code = http.StatusConflict, errBillingAccountConflict.Error()
	case errors.Is(err, errBillingAccountNotFound):
		status, code = http.StatusNotFound, errBillingAccountNotFound.Error()
	}
	ctx.JSON(status, gin.H{"error": gin.H{"code": code}})
}

func (service *managementService) createBillingAccountHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var request managementBillingAccountRequest
		if err := decodeManagementJSON(ctx, &request); err != nil || request.Currency != CatalogCurrencyUSD {
			writeBillingAccountError(ctx, errBillingAccountInvalid)
			return
		}
		key, err := hostedCreationKey(ctx)
		if err != nil {
			writeBillingAccountError(ctx, err)
			return
		}
		record, err := service.store.createBillingAccount(ctx.Request.Context(), managementPrincipalFromContext(ctx), key)
		if err != nil {
			writeBillingAccountError(ctx, err)
			return
		}
		ctx.Header("Location", managementAPIPath+managementBillingAccountsPath+"/"+record.ID)
		ctx.JSON(http.StatusCreated, billingAccountResponse(record))
	}
}

func (service *managementService) listBillingAccountsHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		record, err := service.store.database.billingAccount(ctx.Request.Context(), managementPrincipalFromContext(ctx).userID)
		response := managementBillingAccountsResponse{BillingAccounts: []managementBillingAccountResponse{}}
		if errors.Is(err, errBillingAccountNotFound) {
			ctx.JSON(http.StatusOK, response)
			return
		}
		if err != nil {
			writeBillingAccountError(ctx, err)
			return
		}
		response.BillingAccounts = append(response.BillingAccounts, billingAccountResponse(record))
		ctx.JSON(http.StatusOK, response)
	}
}

func (service *managementService) getBillingAccountHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		record, err := service.store.database.billingAccount(ctx.Request.Context(), managementPrincipalFromContext(ctx).userID)
		if err != nil {
			writeBillingAccountError(ctx, err)
			return
		}
		if record.ID != ctx.Param("billing_account_id") {
			writeBillingAccountError(ctx, errBillingAccountNotFound)
			return
		}
		ctx.JSON(http.StatusOK, billingAccountResponse(record))
	}
}
