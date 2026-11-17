package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"gorm.io/gorm"
)

// OperateQualificationBudget inspects or closes an initialized operator journal.
// It opens existing local stores only and never accesses provider credentials.
func OperateQualificationBudget(managementPath, budgetID, action string, output io.Writer) error {
	return operateQualificationBudget(managementPath, budgetID, action, 0, output)
}

// ReconcileQualificationBudgetMaximum permanently consumes an uncertain attempt's
// retained maximum without claiming provider billing or observed token quantities.
func ReconcileQualificationBudgetMaximum(managementPath, budgetID string, attemptID uint64, output io.Writer) error {
	if attemptID == 0 {
		return errQualificationBudget
	}
	return operateQualificationBudget(managementPath, budgetID, "reconcile-max", attemptID, output)
}

func operateQualificationBudget(managementPath, budgetID, action string, attemptID uint64, output io.Writer) error {
	if action != "inspect" && action != "close" && (action != "reconcile-max" || attemptID == 0) {
		return fmt.Errorf("%w: operator action must be inspect or close", errQualificationBudget)
	}
	if _, err := os.Stat(managementPath); err != nil {
		return fmt.Errorf("%w: management database missing", errQualificationBudget)
	}
	mainDB, err := openQualificationSQLite(managementPath)
	if err != nil {
		return err
	}
	mainSQL, _ := mainDB.DB()
	defer mainSQL.Close()
	var registry qualificationBudgetRegistry
	if err := mainDB.First(&registry, "id = ?", budgetID).Error; err != nil {
		return fmt.Errorf("%w: budget registry unavailable", errQualificationBudget)
	}
	var policy QualificationBudgetConfiguration
	if err := json.Unmarshal([]byte(registry.Policy), &policy); err != nil {
		return errQualificationBudget
	}
	if _, err := os.Stat(policy.ReceiptDatabasePath); err != nil {
		return fmt.Errorf("%w: initialized journal missing", errQualificationBudget)
	}
	database, err := openQualificationSQLite(policy.ReceiptDatabasePath)
	if err != nil {
		return err
	}
	sqlDB, _ := database.DB()
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	var document map[string]any
	if err := database.Transaction(func(tx *gorm.DB) error {
		// Mutation actions obtain the write lock before reading the journal.
		// Inspection keeps a read snapshot without excluding concurrent writers.
		if action != "inspect" {
			if err := lockQualificationBudgetState(tx, policy.BudgetID).Error; err != nil {
				return err
			}
		}
		var state qualificationBudgetState
		if err := tx.First(&state, "id = ?", policy.BudgetID).Error; err != nil || state.Policy != registry.Policy || sha256Hex(state.Frozen) != registry.FrozenDigest {
			return fmt.Errorf("%w: journal identity", errQualificationBudget)
		}
		if err := validateQualificationAccounting(tx, policy, state); err != nil {
			return err
		}
		if action == "reconcile-max" {
			var attempt qualificationBudgetAttempt
			if err := tx.First(&attempt, "id = ?", attemptID).Error; err != nil {
				return fmt.Errorf("%w: attempt unavailable", errQualificationBudget)
			}
			if attempt.State != "settled" && attempt.State != "conservative" {
				if attempt.Active {
					return fmt.Errorf("%w: active dispatch cannot be reconciled", errQualificationBudget)
				}
				if err := tx.Model(&attempt).Updates(map[string]any{"state": "conservative", "cost": attempt.Reserved}).Error; err != nil {
					return err
				}
				var unresolved int64
				if err := tx.Model(&qualificationBudgetAttempt{}).Where("state NOT IN ?", []string{"settled", "conservative"}).Count(&unresolved).Error; err != nil {
					return err
				}
				state.Suspended = unresolved != 0
				if err := tx.Model(&qualificationBudgetState{}).Where("id = ?", policy.BudgetID).Update("suspended", state.Suspended).Error; err != nil {
					return err
				}
			}
		}
		if action == "close" {
			var pending int64
			if err := tx.Model(&qualificationBudgetAttempt{}).Where("state NOT IN ?", []string{"settled", "conservative"}).Count(&pending).Error; err != nil {
				return err
			}
			if pending != 0 {
				return fmt.Errorf("%w: unresolved reservations require reconciliation", errQualificationBudget)
			}
			if err := tx.Model(&qualificationBudgetState{}).Where("id = ?", policy.BudgetID).Update("closed", true).Error; err != nil {
				return err
			}
			state.Closed = true
		}
		var attempts []qualificationBudgetAttempt
		if err := tx.Order("id").Find(&attempts).Error; err != nil {
			return err
		}
		var frozen []qualificationFrozenOffering
		if err := json.Unmarshal([]byte(state.Frozen), &frozen); err != nil {
			return errQualificationBudget
		}
		known, conservative := int64(0), int64(0)
		for _, attempt := range attempts {
			if attempt.State == "settled" {
				known += attempt.Cost
			} else if attempt.State == "conservative" {
				conservative += attempt.Cost
			}
		}
		document = map[string]any{"known_uncached_estimate_micro_usd": known, "conservative_unknown_maximum_micro_usd": conservative, "budget_id": policy.BudgetID, "tenant_id": policy.TenantID, "remaining_micro_usd": state.Remaining, "suspended": state.Suspended, "closed": state.Closed, "attempts": attempts, "frozen_offerings": frozen, "cost_basis": "uncached_catalog_list_estimate"}
		return nil
	}); err != nil {
		return err
	}
	if action == "close" {
		// Journal closure precedes registry closure. A crash between them stays gated.
		if err := mainDB.Model(&registry).Update("closed", true).Error; err != nil {
			return err
		}
	}
	return json.NewEncoder(output).Encode(document)
}
