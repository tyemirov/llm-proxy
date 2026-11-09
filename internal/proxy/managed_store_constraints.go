package proxy

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type managedAssignmentTrigger struct{ name, sql string }

func managedAssignmentTriggers() []managedAssignmentTrigger {
	triggers := make([]managedAssignmentTrigger, 0, 4)
	for _, operation := range []string{"INSERT", "UPDATE"} {
		for _, kind := range []managedAssignmentKind{assignmentAccountConnection, assignmentHostedGrant} {
			target, opposite := "managed_tenant_connection_records", "managed_hosted_tenant_assignment_records"
			if kind == assignmentHostedGrant {
				target, opposite = opposite, target
			}
			name := "guard_" + string(kind) + "_" + strings.ToLower(operation)
			condition := fmt.Sprintf("EXISTS (SELECT 1 FROM %s WHERE tenant_id = NEW.tenant_id AND provider_id = NEW.provider_id)", opposite)
			if kind == assignmentHostedGrant {
				condition += " OR NOT EXISTS (SELECT 1 FROM managed_hosted_grant_records AS grants JOIN managed_billing_account_records AS accounts ON accounts.id = grants.billing_account_id JOIN managed_tenant_records AS tenants ON tenants.tenant_id = grants.tenant_id AND tenants.owner_user_id = accounts.owner_user_id WHERE grants.id = NEW.grant_id AND grants.tenant_id = NEW.tenant_id AND grants.provider = NEW.provider_id)"
			}
			sql := fmt.Sprintf("CREATE TRIGGER %s BEFORE %s ON %s WHEN %s BEGIN SELECT RAISE(ABORT, 'managed_connection_conflict'); END", name, operation, target, condition)
			triggers = append(triggers, managedAssignmentTrigger{name: name, sql: sql})
		}
	}
	return triggers
}

func createManagedAssignmentConstraints(database *gorm.DB) error {
	for _, trigger := range managedAssignmentTriggers() {
		if err := database.Exec(trigger.sql).Error; err != nil {
			return fmt.Errorf("create assignment constraint %s: %w", trigger.name, err)
		}
	}
	return nil
}

func validateManagedAssignmentConstraints(database *gorm.DB) error {
	for _, trigger := range managedAssignmentTriggers() {
		var definition string
		if err := database.Raw("SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = ?", trigger.name).Scan(&definition).Error; err != nil {
			return fmt.Errorf("read assignment constraint %s: %w", trigger.name, err)
		}
		if definition != trigger.sql {
			return fmt.Errorf("assignment constraint %s is missing or changed", trigger.name)
		}
	}
	return nil
}

func validateHostedRecords(database *gorm.DB) error {
	for _, invariant := range []struct{ name, query string }{
		{"foreign_keys", `SELECT COUNT(*) FROM pragma_foreign_key_check`},
		{"platform_versions", `SELECT COUNT(*) FROM managed_platform_connection_records AS connection
			WHERE connection.version != (SELECT COUNT(*) FROM managed_platform_credential_records WHERE connection_id = connection.id)
			OR connection.version != (SELECT COALESCE(MAX(version),0) FROM managed_platform_credential_records WHERE connection_id = connection.id)`},
		{"grant_ownership", `SELECT COUNT(*) FROM managed_hosted_grant_records AS grant_record
			JOIN managed_billing_account_records AS account ON account.id = grant_record.billing_account_id
			JOIN managed_tenant_records AS tenant ON tenant.tenant_id = grant_record.tenant_id
			JOIN managed_platform_connection_records AS connection ON connection.id = grant_record.platform_connection_id
			WHERE account.owner_user_id != tenant.owner_user_id OR connection.provider != grant_record.provider`},
		{"grant_audit", `SELECT COUNT(*) FROM managed_hosted_grant_records AS grant_record
			WHERE grant_record.revision != (SELECT COUNT(*) FROM managed_hosted_grant_revision_records WHERE grant_id = grant_record.id)
			OR grant_record.revision != (SELECT COALESCE(MAX(revision),0) FROM managed_hosted_grant_revision_records WHERE grant_id = grant_record.id)
			OR NOT EXISTS (SELECT 1 FROM managed_hosted_grant_revision_records WHERE grant_id = grant_record.id AND revision = grant_record.revision AND state = grant_record.state)`},
		{"hosted_assignments", `SELECT COUNT(*) FROM managed_hosted_tenant_assignment_records AS assignment
			JOIN managed_hosted_grant_records AS grant_record ON grant_record.id = assignment.grant_id
			WHERE assignment.tenant_id != grant_record.tenant_id OR assignment.provider_id != grant_record.provider
			OR EXISTS (SELECT 1 FROM managed_tenant_connection_records WHERE tenant_id = assignment.tenant_id AND provider_id = assignment.provider_id)
			OR NOT EXISTS (SELECT 1 FROM managed_provider_profile_records WHERE tenant_id = assignment.tenant_id AND provider_id = assignment.provider_id)`},
	} {
		var invalid int64
		if err := database.Raw(invariant.query).Scan(&invalid).Error; err != nil {
			return fmt.Errorf("%w: operation=validate_hosted_records invariant=%s: %w", errManagedTenantSchemaMigration, invariant.name, err)
		}
		if invalid != 0 {
			return fmt.Errorf("%w: operation=validate_hosted_records invariant=%s invalid=%d", errManagedTenantSchemaMigration, invariant.name, invalid)
		}
	}
	return nil
}
