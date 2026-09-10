package proxy

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	managementConnectionsPath       = "/connections"
	managementConnectionPath        = managementConnectionsPath + "/:connection_id"
	managementTenantConnectionPath  = managementConnectionsPath + "/:provider"
	managementConnectionPageDefault = 50
	managementConnectionPageMaximum = 100
)

type managementConnectionRequest struct {
	Name     string            `json:"name"`
	Provider string            `json:"provider"`
	Fields   map[string]string `json:"fields"`
	Version  uint64            `json:"version"`
}

type managementConnectionResponse struct {
	ID        string                            `json:"id"`
	Name      string                            `json:"name"`
	Provider  string                            `json:"provider"`
	Version   uint64                            `json:"version"`
	Fields    []managementProviderFieldResponse `json:"fields"`
	TenantIDs []string                          `json:"tenant_ids"`
	CreatedAt string                            `json:"created_at"`
	UpdatedAt string                            `json:"updated_at"`
}

type managementConnectionsResponse struct {
	Connections []managementConnectionResponse `json:"connections"`
	Providers   []managementProviderResponse   `json:"providers"`
	NextCursor  string                         `json:"next_cursor"`
}

type managedConnectionPage struct {
	after string
	limit int
}

func newManagedConnectionPage(rawQuery string) (managedConnectionPage, error) {
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return managedConnectionPage{}, errManagedConnectionInvalid
	}
	page := managedConnectionPage{limit: managementConnectionPageDefault}
	for name, values := range query {
		if len(values) != 1 {
			return managedConnectionPage{}, errManagedConnectionInvalid
		}
		switch name {
		case "limit":
			limit, err := strconv.Atoi(values[0])
			if err != nil || limit < 1 || limit > managementConnectionPageMaximum {
				return managedConnectionPage{}, errManagedConnectionInvalid
			}
			page.limit = limit
		case "cursor":
			encoded, valid := strings.CutPrefix(values[0], "connection-")
			decoded, err := hex.DecodeString(encoded)
			if !valid || err != nil || len(decoded) != 16 || encoded != strings.ToLower(encoded) {
				return managedConnectionPage{}, errManagedConnectionInvalid
			}
			page.after = values[0]
		default:
			return managedConnectionPage{}, errManagedConnectionInvalid
		}
	}
	return page, nil
}

func (service *managementService) connectionResponse(record managedAccountConnectionRecord) (managementConnectionResponse, error) {
	settings, err := service.store.accountConnectionSettings(record)
	if err != nil {
		return managementConnectionResponse{}, err
	}
	return service.connectionResponseWithSettings(record, settings), nil
}

func (service *managementService) connectionResponseWithSettings(record managedAccountConnectionRecord, settings managedProviderSettings) managementConnectionResponse {
	fields := []managementProviderFieldResponse{}
	for _, provider := range service.providerResponses(map[providerID]managedProviderSettings{providerID(record.ProviderID): settings}) {
		if provider.ID == record.ProviderID {
			fields = provider.Fields
			break
		}
	}
	tenants := make([]string, 0, len(record.Assignments))
	for _, assignment := range record.Assignments {
		tenants = append(tenants, assignment.TenantID)
	}
	sort.Strings(tenants)
	return managementConnectionResponse{ID: record.ID, Name: record.Name, Provider: record.ProviderID, Version: record.Version, Fields: fields, TenantIDs: tenants, CreatedAt: record.CreatedAt.Format(time.RFC3339), UpdatedAt: record.UpdatedAt.Format(time.RFC3339)}
}

func writeConnectionError(ctx *gin.Context, err error) {
	status, code := http.StatusInternalServerError, "managed_connection_store_failed"
	switch {
	case errors.Is(err, errManagedConnectionNotFound), errors.Is(err, errManagedTenantNotFound):
		status = http.StatusNotFound
		code = "managed_connection_not_found"
	case errors.Is(err, errManagedConnectionConflict):
		status = http.StatusConflict
		code = errManagedConnectionConflict.Error()
	case errors.Is(err, errManagedConnectionAssigned):
		status = http.StatusConflict
		code = errManagedConnectionAssigned.Error()
	case errors.Is(err, errManagedConnectionInvalid), errors.Is(err, errManagedProviderKeyInvalid), errors.Is(err, errManagedTenantNameInvalid):
		status = http.StatusBadRequest
		code = errManagedConnectionInvalid.Error()
	}
	ctx.JSON(status, gin.H{"error": gin.H{"code": code}})
}

func (service *managementService) listConnectionsHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		page, err := newManagedConnectionPage(ctx.Request.URL.RawQuery)
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		principal := managementPrincipalFromContext(ctx)
		if _, err := service.store.account(principal); err != nil {
			writeConnectionError(ctx, err)
			return
		}
		records, err := service.store.database.accountConnections(ctx.Request.Context(), principal.userID, page)
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		response := managementConnectionsResponse{Connections: make([]managementConnectionResponse, 0, len(records)), Providers: service.providerResponses(map[providerID]managedProviderSettings{})}
		if len(records) > page.limit {
			records = records[:page.limit]
			response.NextCursor = records[len(records)-1].ID
		}
		for _, record := range records {
			value, err := service.connectionResponse(record)
			if err != nil {
				writeConnectionError(ctx, err)
				return
			}
			response.Connections = append(response.Connections, value)
		}
		ctx.Header(headerCacheControl, cacheControlNoStore)
		ctx.JSON(http.StatusOK, response)
	}
}

func (service *managementService) getConnectionHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		record, err := service.store.database.accountConnection(ctx.Request.Context(), managementPrincipalFromContext(ctx).userID, ctx.Param("connection_id"))
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		response, err := service.connectionResponse(record)
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		ctx.Header(headerCacheControl, cacheControlNoStore)
		ctx.JSON(http.StatusOK, response)
	}
}

func (service *managementService) saveConnectionHandler(create bool) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var request managementConnectionRequest
		if err := decodeManagementJSON(ctx, &request); err != nil {
			writeConnectionError(ctx, errManagedConnectionInvalid)
			return
		}
		name, err := newManagedTenantName(request.Name)
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		providerID, err := service.providers.canonicalProviderID(request.Provider)
		if err != nil {
			writeConnectionError(ctx, errManagedConnectionInvalid)
			return
		}
		principal := managementPrincipalFromContext(ctx)
		if _, err := service.store.account(principal); err != nil {
			writeConnectionError(ctx, err)
			return
		}
		now := service.store.now()
		record := managedAccountConnectionRecord{OwnerUserID: principal.userID, ProviderID: providerID.string(), CreatedAt: now}
		existingSettings := managedProviderSettings{}
		if create {
			if request.Version != 0 {
				writeConnectionError(ctx, errManagedConnectionInvalid)
				return
			}
		} else {
			record, err = service.store.database.accountConnection(ctx.Request.Context(), principal.userID, ctx.Param("connection_id"))
			if err == nil && (record.ProviderID != providerID.string() || request.Version != record.Version) {
				err = errManagedConnectionConflict
			}
			if err == nil {
				existingSettings, err = service.store.accountConnectionSettings(record)
			}
		}
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		definition := service.providers.definitions[providerID]
		values, err := validatedManagedProviderConnectionValues(definition, request.Fields, existingSettings, !create)
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		var creation managedConnectionCreationRecord
		if create {
			creation, err = service.connectionCreationIntent(ctx, managementConnectionRequest{Name: name.display, Provider: providerID.string(), Fields: values})
			if err != nil {
				writeConnectionError(ctx, err)
				return
			}
			receipt, lookupError := service.store.database.connectionCreation(ctx.Request.Context(), principal.userID, creation.KeyDigest)
			if lookupError == nil {
				if receipt.RequestMAC != creation.RequestMAC {
					writeConnectionError(ctx, errManagedConnectionConflict)
					return
				}
				writeConnectionCreation(ctx, receipt)
				return
			}
			if !errors.Is(lookupError, errManagedConnectionNotFound) {
				writeConnectionError(ctx, lookupError)
				return
			}
			record.ID, err = newManagedConnectionID(service.store.randomReader)
			if err != nil {
				writeConnectionError(ctx, err)
				return
			}
		}
		verify := create
		for field, value := range values {
			verify = verify || existingSettings.connectionValue(field) != value
		}
		if verify {
			provider := definition
			model := definition.textModels[definition.defaultTextModel.string()]
			provider.connectionValues = cloneStringMap(values)
			provider, _ = provider.resolvedTransport(model.transportIdentifier)
			if err := service.keyVerifier.verify(ctx.Request.Context(), provider, model, values[provider.activeTransport.authentication.Field]); err != nil {
				writeProviderKeyVerificationError(ctx, err)
				return
			}
		}
		record.Name = name.display
		record.Version = request.Version + 1
		record.UpdatedAt = now
		record.Fields = make([]managedConnectionFieldRecord, 0, len(values))
		for _, fieldID := range definition.fieldOrder {
			value := values[fieldID]
			field := definition.fields[fieldID]
			if value == "" {
				continue
			}
			if field.Secret {
				value, err = service.store.providerKeyCipher.encryptConnection(service.store.randomReader, record.ID, record.ProviderID, fieldID, value)
				if err != nil {
					writeConnectionError(ctx, err)
					return
				}
			}
			record.Fields = append(record.Fields, managedConnectionFieldRecord{ConnectionID: record.ID, FieldID: fieldID, Value: value, CreatedAt: now, UpdatedAt: now})
		}
		settings := managedProviderSettings{connectionValues: values, configuredFields: map[string]bool{}, textModel: definition.defaultTextModel.string()}
		for _, field := range record.Fields {
			settings.configuredFields[field.FieldID] = true
		}
		response := service.connectionResponseWithSettings(record, settings)
		if create {
			creation.ConnectionID = record.ID
			creation.CreatedAt = now
			creation.Response, _ = json.Marshal(response)
		}
		if err := service.store.mutex.LockContext(ctx.Request.Context()); err != nil {
			writeConnectionError(ctx, err)
			return
		}
		if create {
			creation, err = service.store.database.createAccountConnection(ctx.Request.Context(), record, creation)
		} else {
			err = service.store.database.saveAccountConnection(ctx.Request.Context(), record, request.Version)
		}
		service.store.mutex.Unlock()
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		if create {
			writeConnectionCreation(ctx, creation)
			return
		}
		ctx.Header(headerCacheControl, cacheControlNoStore)
		ctx.JSON(http.StatusOK, response)
	}
}

func (service *managementService) deleteConnectionHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if err := service.store.mutex.LockContext(ctx.Request.Context()); err != nil {
			writeConnectionError(ctx, err)
			return
		}
		err := service.store.database.deleteAccountConnection(ctx.Request.Context(), managementPrincipalFromContext(ctx).userID, ctx.Param("connection_id"))
		service.store.mutex.Unlock()
		if err != nil && !errors.Is(err, errManagedConnectionNotFound) {
			writeConnectionError(ctx, err)
			return
		}
		ctx.Status(http.StatusNoContent)
	}
}

func (service *managementService) assignConnectionHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		tenantID, valid := managementTenantIdentifierFromContext(ctx)
		if !valid {
			return
		}
		providerID, err := service.providers.canonicalProviderID(ctx.Param("provider"))
		if err != nil {
			writeConnectionError(ctx, errManagedConnectionInvalid)
			return
		}
		var request struct {
			ConnectionID string `json:"connection_id"`
		}
		if err := decodeManagementJSON(ctx, &request); err != nil || request.ConnectionID == "" {
			writeConnectionError(ctx, errManagedConnectionInvalid)
			return
		}
		principal := managementPrincipalFromContext(ctx)
		if err := service.store.mutex.LockContext(ctx.Request.Context()); err != nil {
			writeConnectionError(ctx, err)
			return
		}
		err = service.store.database.assignAccountConnection(ctx.Request.Context(), principal.userID, tenantID.string(), providerID.string(), request.ConnectionID, service.providers.definitions[providerID].defaultTextModel.string(), service.store.now())
		service.store.mutex.Unlock()
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		snapshot, err := service.store.tenantProfile(principal, tenantID)
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		service.writeTenantProfileResponse(ctx, snapshot, http.StatusOK)
	}
}

func (service *managementService) detachConnectionHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		tenantID, valid := managementTenantIdentifierFromContext(ctx)
		if !valid {
			return
		}
		providerID, err := service.providers.canonicalProviderID(ctx.Param("provider"))
		if err != nil {
			writeConnectionError(ctx, errManagedConnectionInvalid)
			return
		}
		clearDefaults := false
		if value, present := ctx.GetQuery("clear_defaults"); present {
			if value != "true" && value != "false" {
				writeConnectionError(ctx, errManagedConnectionInvalid)
				return
			}
			clearDefaults, _ = strconv.ParseBool(value)
		}
		if err := service.store.mutex.LockContext(ctx.Request.Context()); err != nil {
			writeConnectionError(ctx, err)
			return
		}
		err = service.store.database.detachAccountConnection(ctx.Request.Context(), managementPrincipalFromContext(ctx).userID, tenantID.string(), providerID.string(), clearDefaults, service.store.now())
		service.store.mutex.Unlock()
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		ctx.Status(http.StatusNoContent)
	}
}

func (service *managementService) saveTenantProviderProfileHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		tenantID, valid := managementTenantIdentifierFromContext(ctx)
		if !valid {
			return
		}
		providerID, err := service.providers.canonicalProviderID(ctx.Param("provider"))
		if err != nil {
			writeConnectionError(ctx, errManagedConnectionInvalid)
			return
		}
		var request struct {
			TextModel    string `json:"text_model"`
			SystemPrompt string `json:"system_prompt"`
		}
		if err := decodeManagementJSON(ctx, &request); err != nil || request.TextModel == "" {
			writeConnectionError(ctx, errManagedConnectionInvalid)
			return
		}
		if _, _, err := service.providers.resolveTextModel(providerID.string(), request.TextModel, providerID.string(), request.TextModel, false); err != nil {
			writeConnectionError(ctx, errManagedConnectionInvalid)
			return
		}
		principal := managementPrincipalFromContext(ctx)
		if err := service.store.mutex.LockContext(ctx.Request.Context()); err != nil {
			writeConnectionError(ctx, err)
			return
		}
		err = service.store.database.saveTenantProviderProfile(ctx.Request.Context(), principal.userID, managedProviderProfileRecord{TenantID: tenantID.string(), ProviderID: providerID.string(), TextModel: request.TextModel, SystemPrompt: request.SystemPrompt, UpdatedAt: service.store.now()})
		service.store.mutex.Unlock()
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		snapshot, err := service.store.tenantProfile(principal, tenantID)
		if err != nil {
			writeConnectionError(ctx, err)
			return
		}
		service.writeTenantProfileResponse(ctx, snapshot, http.StatusOK)
	}
}
