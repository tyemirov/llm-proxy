package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	managementHostedGrantsPath         = "/hosted-access-grants"
	managementHostedGrantPath          = managementHostedGrantsPath + "/:grant_id"
	managementHostedGrantRevisionsPath = managementHostedGrantPath + "/revisions"
	hostedGrantIDPrefix                = "grant-"
	hostedCreationGrant                = "hosted_access_grant"
	hostedGrantReasonMaximum           = 500
)

var errManagedTenantHostedHistory = errors.New("managed_tenant_hosted_history")

type hostedGrantState string

const (
	hostedGrantActive    hostedGrantState = "active"
	hostedGrantSuspended hostedGrantState = "suspended"
	hostedGrantRevoked   hostedGrantState = "revoked"
)

type hostedGrantOffering struct {
	Model      string   `json:"model"`
	Operations []string `json:"operations"`
}

type managedHostedGrantRecord struct {
	ID                   string                          `gorm:"primaryKey"`
	BillingAccountID     string                          `gorm:"not null;index"`
	BillingAccount       managedBillingAccountRecord     `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	TenantID             string                          `gorm:"not null;index"`
	Tenant               managedTenantRecord             `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnDelete:RESTRICT"`
	PlatformConnectionID string                          `gorm:"not null"`
	PlatformConnection   managedPlatformConnectionRecord `gorm:"foreignKey:PlatformConnectionID;references:ID;constraint:OnDelete:RESTRICT"`
	Provider             string                          `gorm:"not null"`
	CatalogRevision      string                          `gorm:"not null"`
	Offerings            []byte                          `gorm:"not null"`
	State                hostedGrantState                `gorm:"not null;check:state IN ('active','suspended','revoked')"`
	Revision             uint64                          `gorm:"not null;check:revision > 0"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type managedHostedGrantRevisionRecord struct {
	GrantID     string                   `gorm:"primaryKey"`
	Revision    uint64                   `gorm:"primaryKey;check:revision > 0"`
	Grant       managedHostedGrantRecord `gorm:"foreignKey:GrantID;references:ID;constraint:OnDelete:RESTRICT"`
	State       hostedGrantState         `gorm:"not null;check:state IN ('active','suspended','revoked')"`
	ActorUserID string                   `gorm:"not null"`
	Reason      string                   `gorm:"not null"`
	CreatedAt   time.Time
}

type managementHostedGrantRequest struct {
	BillingAccountID     string                `json:"billing_account_id"`
	TenantID             string                `json:"tenant_id"`
	PlatformConnectionID string                `json:"platform_connection_id"`
	CatalogRevision      string                `json:"catalog_revision"`
	Offerings            []hostedGrantOffering `json:"offerings"`
	Reason               string                `json:"reason"`
}

type managementHostedGrantChange struct {
	State    hostedGrantState `json:"state"`
	Revision uint64           `json:"revision"`
	Reason   string           `json:"reason"`
}

type managementHostedGrantResponse struct {
	ID                   string                `json:"id"`
	BillingAccountID     string                `json:"billing_account_id"`
	TenantID             string                `json:"tenant_id"`
	PlatformConnectionID string                `json:"platform_connection_id,omitempty"`
	Provider             string                `json:"provider"`
	CatalogRevision      string                `json:"catalog_revision"`
	Offerings            []hostedGrantOffering `json:"offerings"`
	State                hostedGrantState      `json:"state"`
	Revision             uint64                `json:"revision"`
	CreatedAt            string                `json:"created_at"`
	UpdatedAt            string                `json:"updated_at"`
}

type managementHostedGrantsResponse struct {
	Grants     []managementHostedGrantResponse `json:"hosted_access_grants"`
	NextCursor string                          `json:"next_cursor"`
}

type managementHostedGrantRevisionResponse struct {
	Revision    uint64           `json:"revision"`
	State       hostedGrantState `json:"state"`
	ActorUserID string           `json:"actor_user_id"`
	Reason      string           `json:"reason"`
	CreatedAt   string           `json:"created_at"`
}

type managementHostedGrantRevisionsResponse struct {
	Revisions  []managementHostedGrantRevisionResponse `json:"revisions"`
	NextCursor string                                  `json:"next_cursor"`
}

func validHostedGrantReason(reason string) bool {
	return reason != "" && reason == strings.TrimSpace(reason) && utf8.RuneCountInString(reason) <= hostedGrantReasonMaximum
}

func validateHostedGrantChange(change managementHostedGrantChange) error {
	if change.Revision == 0 || change.Revision >= math.MaxInt64 || !validHostedGrantReason(change.Reason) {
		return errHostedAccessInvalid
	}
	switch change.State {
	case hostedGrantActive, hostedGrantSuspended, hostedGrantRevoked:
		return nil
	default:
		return errHostedAccessInvalid
	}
}

func normalizeHostedGrantOfferings(request *managementHostedGrantRequest) error {
	if len(request.Offerings) == 0 {
		return errHostedAccessInvalid
	}
	seen := make(map[string]bool, len(request.Offerings))
	for index := range request.Offerings {
		offering := &request.Offerings[index]
		if offering.Model == "" || seen[offering.Model] || len(offering.Operations) == 0 {
			return errHostedAccessInvalid
		}
		seen[offering.Model] = true
		slices.Sort(offering.Operations)
		for operationIndex, operation := range offering.Operations {
			if operation == "" || (operationIndex > 0 && offering.Operations[operationIndex-1] == operation) {
				return errHostedAccessInvalid
			}
		}
	}
	slices.SortFunc(request.Offerings, func(left, right hostedGrantOffering) int { return strings.Compare(left.Model, right.Model) })
	return nil
}

func (service *managementService) validateHostedGrantOfferings(request managementHostedGrantRequest, provider string) error {
	catalog := service.providers.catalog.modelCatalog
	if request.CatalogRevision != catalog.Revision {
		return errHostedAccessConflict
	}
	available := make(map[string][]string)
	for _, offering := range catalog.Offerings {
		if offering.Provider == provider {
			available[offering.Model] = offering.Operations
		}
	}
	for _, offering := range request.Offerings {
		for _, operation := range offering.Operations {
			if !slices.Contains(available[offering.Model], operation) {
				return errHostedAccessInvalid
			}
		}
	}
	return nil
}

func hostedGrantResponse(record managedHostedGrantRecord, principal managementPrincipal) (managementHostedGrantResponse, error) {
	var offerings []hostedGrantOffering
	if err := json.Unmarshal(record.Offerings, &offerings); err != nil {
		return managementHostedGrantResponse{}, fmt.Errorf("read grant %s offerings: %w", record.ID, err)
	}
	response := managementHostedGrantResponse{
		ID: record.ID, BillingAccountID: record.BillingAccountID, TenantID: record.TenantID,
		Provider: record.Provider, CatalogRevision: record.CatalogRevision, Offerings: offerings,
		State: record.State, Revision: record.Revision,
		CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if principal.isAdmin {
		response.PlatformConnectionID = record.PlatformConnectionID
	}
	return response, nil
}

func hostedGrantOwnerQuery(database *gorm.DB, principal managementPrincipal) *gorm.DB {
	query := database.Model(&managedHostedGrantRecord{})
	if !principal.isAdmin {
		query = query.Where("billing_account_id IN (?)", database.Model(&managedBillingAccountRecord{}).Select("id").Where("owner_user_id = ?", principal.userID))
	}
	return query
}

func (database *gormManagedTenantDatabase) hostedGrant(ctx context.Context, principal managementPrincipal, id string) (managedHostedGrantRecord, error) {
	var record managedHostedGrantRecord
	err := hostedGrantOwnerQuery(database.database.WithContext(ctx), principal).Where("id = ?", id).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, errHostedAccessNotFound
	}
	if err != nil {
		return record, fmt.Errorf("read hosted access grant %s: %w", id, err)
	}
	return record, nil
}

func (database *gormManagedTenantDatabase) hostedGrants(ctx context.Context, principal managementPrincipal, page managedConnectionPage) ([]managedHostedGrantRecord, error) {
	records := []managedHostedGrantRecord{}
	err := hostedGrantOwnerQuery(database.database.WithContext(ctx), principal).Where("id > ?", page.after).Order("id").Limit(page.limit + 1).Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("list hosted access grants for actor %s: %w", principal.userID, err)
	}
	return records, nil
}

func (database *gormManagedTenantDatabase) createHostedGrant(ctx context.Context, record managedHostedGrantRecord, audit managedHostedGrantRevisionRecord, intent managedHostedCreationRecord) (managedHostedCreationRecord, error) {
	var receipt managedHostedCreationRecord
	err := database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var created bool
		var err error
		receipt, created, err = insertHostedCreation(transaction, intent)
		if err != nil || !created {
			return err
		}
		var tenant managedTenantRecord
		err = transaction.Where("tenant_id = ? AND owner_user_id IN (?)", record.TenantID,
			transaction.Model(&managedBillingAccountRecord{}).Select("owner_user_id").Where("id = ?", record.BillingAccountID)).First(&tenant).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errHostedAccessNotFound
		}
		if err != nil {
			return fmt.Errorf("read grant tenant and billing owner: %w", err)
		}
		if err := transaction.Omit("BillingAccount", "Tenant", "PlatformConnection").Create(&record).Error; err != nil {
			return fmt.Errorf("create hosted access grant: %w", err)
		}
		if err := transaction.Omit("Grant").Create(&audit).Error; err != nil {
			return fmt.Errorf("create hosted access grant audit: %w", err)
		}
		return nil
	})
	return receipt, err
}

func (database *gormManagedTenantDatabase) changeHostedGrant(ctx context.Context, id string, change managementHostedGrantChange, actor string, now time.Time) (managedHostedGrantRecord, error) {
	var record managedHostedGrantRecord
	err := database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		result := transaction.Model(&managedHostedGrantRecord{}).Where("id = ? AND revision = ? AND state != ? AND state != ?", id, change.Revision, hostedGrantRevoked, change.State).
			Updates(map[string]any{"state": change.State, "revision": change.Revision + 1, "updated_at": now})
		if result.Error != nil {
			return fmt.Errorf("change hosted access grant: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return errHostedAccessConflict
		}
		audit := managedHostedGrantRevisionRecord{GrantID: id, Revision: change.Revision + 1, State: change.State, ActorUserID: actor, Reason: change.Reason, CreatedAt: now}
		if err := transaction.Omit("Grant").Create(&audit).Error; err != nil {
			return fmt.Errorf("record hosted access grant transition: %w", err)
		}
		if err := transaction.Where("id = ?", id).First(&record).Error; err != nil {
			return fmt.Errorf("read changed hosted access grant: %w", err)
		}
		return nil
	})
	return record, err
}

func (database *gormManagedTenantDatabase) hostedGrantRevisions(ctx context.Context, id string, page managedConnectionPage) ([]managedHostedGrantRevisionRecord, error) {
	records := []managedHostedGrantRevisionRecord{}
	err := database.database.WithContext(ctx).Where("grant_id = ? AND revision > ?", id, page.after).Order("revision").Limit(page.limit + 1).Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("list hosted access grant %s revisions: %w", id, err)
	}
	return records, nil
}

func (service *managementService) createHostedGrantHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireHostedOperator(ctx) {
			return
		}
		var request managementHostedGrantRequest
		if err := decodeManagementJSON(ctx, &request); err != nil || !validHostedGrantReason(request.Reason) || request.BillingAccountID == "" || request.TenantID == "" {
			writeHostedAccessError(ctx, errHostedAccessInvalid)
			return
		}
		if err := normalizeHostedGrantOfferings(&request); err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		intent, err := service.hostedCreationIntent(ctx, hostedCreationGrant, request)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		receipt, err := service.store.database.hostedCreation(ctx.Request.Context(), intent)
		if err == nil {
			writeHostedCreation(ctx, managementHostedGrantsPath, receipt)
			return
		}
		if !errors.Is(err, errHostedAccessNotFound) {
			writeHostedAccessError(ctx, err)
			return
		}
		connection, err := service.store.database.platformConnection(ctx.Request.Context(), request.PlatformConnectionID)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		if err := service.validateHostedGrantOfferings(request, connection.Provider); err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		id, err := newHostedResourceID(hostedGrantIDPrefix, service.store.randomReader)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		offerings, err := json.Marshal(request.Offerings)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		now := service.store.now().UTC()
		record := managedHostedGrantRecord{ID: id, BillingAccountID: request.BillingAccountID, TenantID: request.TenantID,
			PlatformConnectionID: connection.ID, Provider: connection.Provider, CatalogRevision: request.CatalogRevision,
			Offerings: offerings, State: hostedGrantActive, Revision: 1, CreatedAt: now, UpdatedAt: now}
		principal := managementPrincipalFromContext(ctx)
		response, err := hostedGrantResponse(record, principal)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		intent.ResourceID = id
		intent.Response, err = json.Marshal(response)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		audit := managedHostedGrantRevisionRecord{GrantID: id, Revision: 1, State: hostedGrantActive, ActorUserID: principal.userID, Reason: request.Reason, CreatedAt: now}
		receipt, err = service.store.database.createHostedGrant(ctx.Request.Context(), record, audit, intent)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		writeHostedCreation(ctx, managementHostedGrantsPath, receipt)
	}
}

func (service *managementService) getHostedGrantHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		principal := managementPrincipalFromContext(ctx)
		record, err := service.store.database.hostedGrant(ctx.Request.Context(), principal, ctx.Param("grant_id"))
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		response, err := hostedGrantResponse(record, principal)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}

func (service *managementService) listHostedGrantsHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		page, err := newHostedResourcePage(ctx.Request.URL.RawQuery, hostedGrantIDPrefix)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		principal := managementPrincipalFromContext(ctx)
		records, err := service.store.database.hostedGrants(ctx.Request.Context(), principal, page)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		response := managementHostedGrantsResponse{Grants: []managementHostedGrantResponse{}}
		if len(records) > page.limit {
			records = records[:page.limit]
			response.NextCursor = records[len(records)-1].ID
		}
		for _, record := range records {
			grant, err := hostedGrantResponse(record, principal)
			if err != nil {
				writeHostedAccessError(ctx, err)
				return
			}
			response.Grants = append(response.Grants, grant)
		}
		ctx.JSON(http.StatusOK, response)
	}
}

func (service *managementService) changeHostedGrantHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireHostedOperator(ctx) {
			return
		}
		var change managementHostedGrantChange
		if err := decodeManagementJSON(ctx, &change); err != nil {
			writeHostedAccessError(ctx, errHostedAccessInvalid)
			return
		}
		if err := validateHostedGrantChange(change); err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		principal := managementPrincipalFromContext(ctx)
		id := ctx.Param("grant_id")
		if _, err := service.store.database.hostedGrant(ctx.Request.Context(), principal, id); err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		record, err := service.store.database.changeHostedGrant(ctx.Request.Context(), id, change, principal.userID, service.store.now().UTC())
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		response, err := hostedGrantResponse(record, principal)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}

func newHostedRevisionPage(rawQuery string) (managedConnectionPage, error) {
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return managedConnectionPage{}, errHostedAccessInvalid
	}
	page := managedConnectionPage{limit: managementConnectionPageDefault, after: "0"}
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
			revision, err := strconv.ParseUint(values[0], 10, 63)
			if err != nil || revision == 0 || strconv.FormatUint(revision, 10) != values[0] {
				return managedConnectionPage{}, errHostedAccessInvalid
			}
			page.after = values[0]
		default:
			return managedConnectionPage{}, errHostedAccessInvalid
		}
	}
	return page, nil
}

func (service *managementService) listHostedGrantRevisionsHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireHostedOperator(ctx) {
			return
		}
		page, err := newHostedRevisionPage(ctx.Request.URL.RawQuery)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		id := ctx.Param("grant_id")
		if _, err := service.store.database.hostedGrant(ctx.Request.Context(), managementPrincipalFromContext(ctx), id); err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		records, err := service.store.database.hostedGrantRevisions(ctx.Request.Context(), id, page)
		if err != nil {
			writeHostedAccessError(ctx, err)
			return
		}
		response := managementHostedGrantRevisionsResponse{Revisions: []managementHostedGrantRevisionResponse{}}
		if len(records) > page.limit {
			records = records[:page.limit]
			response.NextCursor = strconv.FormatUint(records[len(records)-1].Revision, 10)
		}
		for _, record := range records {
			response.Revisions = append(response.Revisions, managementHostedGrantRevisionResponse{Revision: record.Revision, State: record.State,
				ActorUserID: record.ActorUserID, Reason: record.Reason, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano)})
		}
		ctx.JSON(http.StatusOK, response)
	}
}
