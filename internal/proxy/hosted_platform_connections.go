package proxy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	managementPlatformConnectionsPath = "/platform-connections"
	managementPlatformConnectionPath  = managementPlatformConnectionsPath + "/:platform_connection_id"
	platformConnectionIDPrefix        = "platform-"
	hostedCreationPlatformConnection  = "platform_connection"
)

var (
	errHostedAccessInvalid  = errors.New("hosted_access_invalid")
	errHostedAccessConflict = errors.New("hosted_access_conflict")
	errHostedAccessNotFound = errors.New("hosted_access_not_found")
)

type managedPlatformConnectionRecord struct {
	ID        string `gorm:"primaryKey"`
	Provider  string `gorm:"not null"`
	Name      string `gorm:"not null"`
	Version   uint64 `gorm:"not null;check:version > 0"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type managedPlatformCredentialRecord struct {
	ConnectionID string                          `gorm:"primaryKey"`
	Version      uint64                          `gorm:"primaryKey;check:version > 0"`
	Connection   managedPlatformConnectionRecord `gorm:"foreignKey:ConnectionID;references:ID;constraint:OnDelete:RESTRICT"`
	Fields       []byte                          `gorm:"not null"`
	QualifiedAt  time.Time                       `gorm:"not null"`
	CreatedAt    time.Time
}

// Receipts retain safe creation responses independently of later resource changes.
type managedHostedCreationRecord struct {
	ActorUserID string `gorm:"primaryKey"`
	Kind        string `gorm:"primaryKey"`
	KeyDigest   string `gorm:"primaryKey"`
	RequestMAC  string `gorm:"not null"`
	ResourceID  string `gorm:"not null"`
	Response    []byte `gorm:"not null"`
	CreatedAt   time.Time
}

type managementPlatformConnectionRequest struct {
	Name     string            `json:"name"`
	Provider string            `json:"provider"`
	Fields   map[string]string `json:"fields"`
	Version  uint64            `json:"version"`
}

type managementPlatformConnectionResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	Version   uint64 `json:"version"`
	Qualified bool   `json:"qualified"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type managementPlatformConnectionsResponse struct {
	Connections []managementPlatformConnectionResponse `json:"platform_connections"`
	NextCursor  string                                 `json:"next_cursor"`
}

func newHostedResourcePage(rawQuery, prefix string) (managedConnectionPage, error) {
	return newHostedPage(rawQuery, func(cursor string) bool {
		encoded, found := strings.CutPrefix(cursor, prefix)
		identifier, err := hex.DecodeString(encoded)
		return found && err == nil && len(identifier) == hostedResourceIDBytes && encoded == strings.ToLower(encoded)
	})
}

func newHostedPage(rawQuery string, validCursor func(string) bool) (managedConnectionPage, error) {
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return managedConnectionPage{}, errHostedAccessInvalid
	}
	page := managedConnectionPage{limit: managementConnectionPageDefault}
	for name, values := range query {
		if len(values) != 1 {
			return managedConnectionPage{}, errHostedAccessInvalid
		}
		switch name {
		case "limit":
			page.limit, err = strconv.Atoi(values[0])
			if err != nil || page.limit < 1 || page.limit > managementConnectionPageMaximum {
				return managedConnectionPage{}, errHostedAccessInvalid
			}
		case "cursor":
			if !validCursor(values[0]) {
				return managedConnectionPage{}, errHostedAccessInvalid
			}
			page.after = values[0]
		default:
			return managedConnectionPage{}, errHostedAccessInvalid
		}
	}
	return page, nil
}

func platformCredentialReference(id string, version uint64) string {
	return fmt.Sprintf("%s:v%d", id, version)
}

func platformConnectionResponse(record managedPlatformConnectionRecord) managementPlatformConnectionResponse {
	return managementPlatformConnectionResponse{
		ID: record.ID, Name: record.Name, Provider: record.Provider, Version: record.Version, Qualified: true,
		CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (service *managementService) hostedCreationIntent(ctx *gin.Context, kind string, request any) (managedHostedCreationRecord, error) {
	key, err := hostedCreationKey(ctx)
	if err != nil {
		return managedHostedCreationRecord{}, errHostedAccessInvalid
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return managedHostedCreationRecord{}, fmt.Errorf("encode %s creation intent: %w", kind, err)
	}
	mac := hmac.New(sha256.New, []byte(service.configuration.ProviderKeyEncryptionKey))
	_, _ = mac.Write(payload)
	return managedHostedCreationRecord{
		ActorUserID: managementPrincipalFromContext(ctx).userID, Kind: kind, KeyDigest: key,
		RequestMAC: hex.EncodeToString(mac.Sum(nil)), CreatedAt: service.store.now().UTC(),
	}, nil
}

func (database *gormManagedTenantDatabase) hostedCreation(ctx context.Context, intent managedHostedCreationRecord) (managedHostedCreationRecord, error) {
	var record managedHostedCreationRecord
	err := database.database.WithContext(ctx).Where("actor_user_id = ? AND kind = ? AND key_digest = ?", intent.ActorUserID, intent.Kind, intent.KeyDigest).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, errHostedAccessNotFound
	}
	if err != nil {
		return record, fmt.Errorf("read hosted creation receipt: %w", err)
	}
	if record.RequestMAC != intent.RequestMAC {
		return record, errHostedAccessConflict
	}
	return record, nil
}

func (database *gormManagedTenantDatabase) platformConnection(ctx context.Context, id string) (managedPlatformConnectionRecord, error) {
	var record managedPlatformConnectionRecord
	err := database.database.WithContext(ctx).Where("id = ?", id).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, errHostedAccessNotFound
	}
	if err != nil {
		return record, fmt.Errorf("read platform connection %s: %w", id, err)
	}
	return record, nil
}

func (database *gormManagedTenantDatabase) platformConnections(ctx context.Context, page managedConnectionPage) ([]managedPlatformConnectionRecord, error) {
	records := []managedPlatformConnectionRecord{}
	err := database.database.WithContext(ctx).Where("id > ?", page.after).Order("id").Limit(page.limit + 1).Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("list platform connections: %w", err)
	}
	return records, nil
}

func insertHostedCreation(transaction *gorm.DB, intent managedHostedCreationRecord) (managedHostedCreationRecord, bool, error) {
	result := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&intent)
	if result.Error != nil {
		return managedHostedCreationRecord{}, false, fmt.Errorf("create hosted receipt: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return intent, true, nil
	}
	var existing managedHostedCreationRecord
	if err := transaction.Where("actor_user_id = ? AND kind = ? AND key_digest = ?", intent.ActorUserID, intent.Kind, intent.KeyDigest).First(&existing).Error; err != nil {
		return existing, false, fmt.Errorf("read concurrent hosted receipt: %w", err)
	}
	if existing.RequestMAC != intent.RequestMAC {
		return existing, false, errHostedAccessConflict
	}
	return existing, false, nil
}

func (database *gormManagedTenantDatabase) createPlatformConnection(ctx context.Context, record managedPlatformConnectionRecord, credential managedPlatformCredentialRecord, intent managedHostedCreationRecord) (managedHostedCreationRecord, error) {
	var receipt managedHostedCreationRecord
	err := database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var created bool
		var err error
		receipt, created, err = insertHostedCreation(transaction, intent)
		if err != nil || !created {
			return err
		}
		if err := transaction.Create(&record).Error; err != nil {
			return fmt.Errorf("create platform connection: %w", err)
		}
		if err := transaction.Omit("Connection").Create(&credential).Error; err != nil {
			return fmt.Errorf("create platform credential: %w", err)
		}
		return nil
	})
	return receipt, err
}

func (database *gormManagedTenantDatabase) rotatePlatformConnection(ctx context.Context, record managedPlatformConnectionRecord, credential managedPlatformCredentialRecord, expected uint64) error {
	return database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		result := transaction.Model(&managedPlatformConnectionRecord{}).Where("id = ? AND version = ? AND provider = ?", record.ID, expected, record.Provider).
			Updates(map[string]any{"name": record.Name, "version": record.Version, "updated_at": record.UpdatedAt})
		if result.Error != nil {
			return fmt.Errorf("rotate platform connection: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return errHostedAccessConflict
		}
		if err := transaction.Omit("Connection").Create(&credential).Error; err != nil {
			return fmt.Errorf("retain platform credential version: %w", err)
		}
		return nil
	})
}

func requireHostedOperator(ctx *gin.Context) bool {
	if !managementPrincipalFromContext(ctx).isAdmin {
		ctx.AbortWithStatus(http.StatusForbidden)
		return false
	}
	return true
}

func writeHostedAccessError(ctx *gin.Context, err error) {
	status, code := http.StatusInternalServerError, "hosted_access_store_failed"
	switch {
	case errors.Is(err, errHostedAccessInvalid):
		status, code = http.StatusBadRequest, errHostedAccessInvalid.Error()
	case errors.Is(err, errHostedAccessConflict):
		status, code = http.StatusConflict, errHostedAccessConflict.Error()
	case errors.Is(err, errHostedAccessNotFound):
		status, code = http.StatusNotFound, errHostedAccessNotFound.Error()
	}
	ctx.JSON(status, gin.H{"error": gin.H{"code": code}})
}

func writeHostedCreation(ctx *gin.Context, collection string, receipt managedHostedCreationRecord) {
	ctx.Header("Location", managementAPIPath+collection+"/"+receipt.ResourceID)
	ctx.Data(http.StatusCreated, "application/json; charset=utf-8", receipt.Response)
}

func (service *managementService) qualifiedPlatformCredential(ctx *gin.Context, record managedPlatformConnectionRecord, values map[string]string) (managedPlatformCredentialRecord, error) {
	provider := service.providers.definitions[providerID(record.Provider)]
	provider.upstreamScope = upstreamRequestScope{tenant: upstreamManagementTenant + ":" + managementPrincipalFromContext(ctx).userID, account: record.ID, class: upstreamInteractive}
	model, available := provider.textModels[provider.verification.Model]
	if provider.verification.Model != "" && !available {
		return managedPlatformCredentialRecord{}, errProviderKeyVerificationUnavailable
	}
	provider.connectionValues = cloneStringMap(values)
	provider, transportAvailable := provider.resolvedTransport(provider.verification.Transport)
	if !transportAvailable {
		return managedPlatformCredentialRecord{}, errProviderKeyVerificationUnavailable
	}
	if err := service.keyVerifier.verify(ctx.Request.Context(), provider, model, values[provider.activeTransport.authentication.Field]); err != nil {
		return managedPlatformCredentialRecord{}, err
	}
	fields := cloneStringMap(values)
	for fieldID, value := range fields {
		if provider.fields[fieldID].Secret && value != "" {
			encrypted, err := service.store.providerKeyCipher.encryptConnection(service.store.randomReader, platformCredentialReference(record.ID, record.Version), record.Provider, fieldID, value)
			if err != nil {
				return managedPlatformCredentialRecord{}, err
			}
			fields[fieldID] = encrypted
		}
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return managedPlatformCredentialRecord{}, fmt.Errorf("encode platform credential: %w", err)
	}
	return managedPlatformCredentialRecord{ConnectionID: record.ID, Version: record.Version, Fields: payload, QualifiedAt: service.store.now().UTC(), CreatedAt: record.UpdatedAt}, nil
}

func (service *managementService) platformConnectionRequest(ctx *gin.Context) (managementPlatformConnectionRequest, error) {
	var request managementPlatformConnectionRequest
	if err := decodeManagementJSON(ctx, &request); err != nil {
		return request, errHostedAccessInvalid
	}
	name, err := newManagedTenantName(request.Name)
	if err != nil {
		return request, errHostedAccessInvalid
	}
	provider, err := service.providers.canonicalProviderID(request.Provider)
	if err != nil {
		return request, errHostedAccessInvalid
	}
	values, err := validatedManagedProviderConnectionValues(service.providers.definitions[provider], request.Fields, managedProviderSettings{}, false)
	if err != nil {
		return request, errHostedAccessInvalid
	}
	request.Name, request.Provider, request.Fields = name.display, provider.string(), values
	return request, nil
}

func (service *managementService) createPlatformConnectionHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireHostedOperator(ctx) {
			return
		}
		request, err := service.platformConnectionRequest(ctx)
		if err != nil || request.Version != 0 {
			writeHostedAccessError(ctx, errHostedAccessInvalid)
			return
		}
		intent, err := service.hostedCreationIntent(ctx, hostedCreationPlatformConnection, request)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		receipt, err := service.store.database.hostedCreation(ctx.Request.Context(), intent)
		if err == nil {
			writeHostedCreation(ctx, managementPlatformConnectionsPath, receipt)
			return
		}
		if !errors.Is(err, errHostedAccessNotFound) {
			writeHostedAccessError(ctx, err)
			return
		}
		id, err := newHostedResourceID(platformConnectionIDPrefix, service.store.randomReader)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		now := service.store.now().UTC()
		record := managedPlatformConnectionRecord{ID: id, Provider: request.Provider, Name: request.Name, Version: 1, CreatedAt: now, UpdatedAt: now}
		credential, err := service.qualifiedPlatformCredential(ctx, record, request.Fields)
		if err != nil {
			writeProviderKeyVerificationError(ctx, err)
			return
		}
		intent.ResourceID = id
		intent.Response, err = json.Marshal(platformConnectionResponse(record))
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		receipt, err = service.store.database.createPlatformConnection(ctx.Request.Context(), record, credential, intent)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		writeHostedCreation(ctx, managementPlatformConnectionsPath, receipt)
	}
}

func (service *managementService) rotatePlatformConnectionHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireHostedOperator(ctx) {
			return
		}
		request, err := service.platformConnectionRequest(ctx)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		record, err := service.store.database.platformConnection(ctx.Request.Context(), ctx.Param("platform_connection_id"))
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		if request.Version == 0 || request.Version == math.MaxUint64 || record.Version != request.Version || record.Provider != request.Provider {
			writeHostedAccessError(ctx, errHostedAccessConflict)
			return
		}
		record.Name, record.Version, record.UpdatedAt = request.Name, request.Version+1, service.store.now().UTC()
		credential, err := service.qualifiedPlatformCredential(ctx, record, request.Fields)
		if err != nil {
			writeProviderKeyVerificationError(ctx, err)
			return
		}
		if err := service.store.database.rotatePlatformConnection(ctx.Request.Context(), record, credential, request.Version); err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, platformConnectionResponse(record))
	}
}

func (service *managementService) getPlatformConnectionHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireHostedOperator(ctx) {
			return
		}
		record, err := service.store.database.platformConnection(ctx.Request.Context(), ctx.Param("platform_connection_id"))
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, platformConnectionResponse(record))
	}
}

func (service *managementService) listPlatformConnectionsHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireHostedOperator(ctx) {
			return
		}
		page, err := newHostedResourcePage(ctx.Request.URL.RawQuery, platformConnectionIDPrefix)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		records, err := service.store.database.platformConnections(ctx.Request.Context(), page)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		response := managementPlatformConnectionsResponse{Connections: []managementPlatformConnectionResponse{}}
		if len(records) > page.limit {
			records = records[:page.limit]
			response.NextCursor = records[len(records)-1].ID
		}
		for _, record := range records {
			response.Connections = append(response.Connections, platformConnectionResponse(record))
		}
		ctx.JSON(http.StatusOK, response)
	}
}
