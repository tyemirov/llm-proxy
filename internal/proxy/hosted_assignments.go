package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type managedAssignmentKind string

const (
	assignmentAccountConnection managedAssignmentKind = "account_connection"
	assignmentHostedGrant       managedAssignmentKind = "hosted_access_grant"
)

type managedProviderAssignment struct {
	Provider   string                `json:"provider"`
	Kind       managedAssignmentKind `json:"kind"`
	ResourceID string                `json:"resource_id"`
}

type managementAssignmentRequest struct {
	Kind       managedAssignmentKind `json:"kind"`
	ResourceID string                `json:"resource_id"`
}

func (request managementAssignmentRequest) validate() error {
	if request.ResourceID == "" {
		return errManagedConnectionInvalid
	}
	switch request.Kind {
	case assignmentAccountConnection, assignmentHostedGrant:
		return nil
	default:
		return errManagedConnectionInvalid
	}
}

type managedHostedTenantAssignmentRecord struct {
	TenantID   string                   `gorm:"primaryKey"`
	ProviderID string                   `gorm:"primaryKey"`
	GrantID    string                   `gorm:"not null;index"`
	Grant      managedHostedGrantRecord `gorm:"foreignKey:GrantID;references:ID;constraint:OnDelete:RESTRICT"`
	Tenant     managedTenantRecord      `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnDelete:RESTRICT"`
	CreatedAt  time.Time
}

func readHostedTenantAssignments(database *gorm.DB, record *managedTenantRecord) error {
	if err := database.Preload("Grant").Where("tenant_id = ?", record.TenantID).Find(&record.HostedAssignments).Error; err != nil {
		return fmt.Errorf("read hosted assignments for tenant %s: %w", record.TenantID, err)
	}
	return nil
}

func providerAssignments(record managedTenantRecord) []managedProviderAssignment {
	assignments := make([]managedProviderAssignment, 0, len(record.ConnectionAssignments)+len(record.HostedAssignments))
	for _, assignment := range record.ConnectionAssignments {
		assignments = append(assignments, managedProviderAssignment{Provider: assignment.ProviderID, Kind: assignmentAccountConnection, ResourceID: assignment.ConnectionID})
	}
	for _, assignment := range record.HostedAssignments {
		assignments = append(assignments, managedProviderAssignment{Provider: assignment.ProviderID, Kind: assignmentHostedGrant, ResourceID: assignment.GrantID})
	}
	slices.SortFunc(assignments, func(left, right managedProviderAssignment) int { return strings.Compare(left.Provider, right.Provider) })
	return assignments
}

func (database *gormManagedTenantDatabase) assignHostedGrant(ctx context.Context, owner, tenantID, provider, grantID, textModel string, now time.Time) error {
	return database.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if _, err := lockProviderAssignmentTenant(transaction, owner, tenantID); err != nil {
			return err
		}
		var grant managedHostedGrantRecord
		err := transaction.Where("id = ? AND tenant_id = ? AND provider = ? AND billing_account_id IN (?)", grantID, tenantID, provider,
			transaction.Model(&managedBillingAccountRecord{}).Select("id").Where("owner_user_id = ?", owner)).First(&grant).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errManagedConnectionNotFound
		}
		if err != nil {
			return fmt.Errorf("read hosted assignment grant %s: %w", grantID, err)
		}
		var existing managedHostedTenantAssignmentRecord
		err = transaction.Where("tenant_id = ? AND provider_id = ?", tenantID, provider).First(&existing).Error
		if err == nil {
			if existing.GrantID == grantID {
				return nil
			}
			return errManagedConnectionConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read hosted assignment for tenant %s: %w", tenantID, err)
		}
		if grant.State != hostedGrantActive {
			return errManagedConnectionConflict
		}
		var accountAssignments int64
		if err := transaction.Model(&managedTenantConnectionRecord{}).Where("tenant_id = ? AND provider_id = ?", tenantID, provider).Count(&accountAssignments).Error; err != nil {
			return fmt.Errorf("read account assignment for tenant %s: %w", tenantID, err)
		}
		if accountAssignments != 0 {
			return errManagedConnectionConflict
		}
		record := managedHostedTenantAssignmentRecord{TenantID: tenantID, ProviderID: provider, GrantID: grantID, CreatedAt: now}
		if err := transaction.Omit("Grant", "Tenant").Create(&record).Error; err != nil {
			return fmt.Errorf("create hosted assignment for tenant %s: %w", tenantID, err)
		}
		profile := managedProviderProfileRecord{TenantID: tenantID, ProviderID: provider, TextModel: textModel, CreatedAt: now, UpdatedAt: now}
		if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&profile).Error; err != nil {
			return fmt.Errorf("create hosted provider profile for tenant %s: %w", tenantID, err)
		}
		return nil
	})
}

func (service *managementService) listProviderAssignmentsHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		tenantID, valid := managementTenantIdentifierFromContext(ctx)
		if !valid {
			return
		}
		record, err := service.store.database.tenantByOwnerAndID(managementPrincipalFromContext(ctx).userID, tenantID.string())
		if err != nil {
			writeConnectionError(ctx, managedTenantQueryError(managementPrincipalFromContext(ctx).userID, tenantID.string(), err))
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"assignments": providerAssignments(record)})
	}
}
