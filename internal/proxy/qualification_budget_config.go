package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

var errQualificationBudget = errors.New("qualification_budget_denied")

// QualificationBudgetConfiguration defines an operator-owned, uncached list-cost estimate cap.
// It contains resource identities and limits, never provider credentials.
type QualificationBudgetConfiguration struct {
	BudgetID                string                          `json:"budget_id" mapstructure:"budget_id"`
	TenantID                string                          `json:"tenant_id" mapstructure:"tenant_id"`
	OwnerUserID             string                          `json:"owner_user_id" mapstructure:"owner_user_id"`
	ExpiresAt               string                          `json:"expires_at" mapstructure:"expires_at"`
	RemainingMicroUSD       int64                           `json:"remaining_micro_usd" mapstructure:"remaining_micro_usd"`
	ReceiptDatabasePath     string                          `json:"receipt_database_path" mapstructure:"receipt_database_path"`
	MaxAttemptsPerRequest   int                             `json:"max_attempts_per_request" mapstructure:"max_attempts_per_request"`
	Offerings               []QualificationBudgetOffering   `json:"offerings" mapstructure:"offerings"`
	VerificationConnections []QualificationBudgetConnection `json:"verification_connections" mapstructure:"verification_connections"`
}

// QualificationBudgetOffering authorizes one exact text route and enforced output bound.
type QualificationBudgetOffering struct {
	PriceRegion     string `json:"price_region" mapstructure:"price_region"`
	Provider        string `json:"provider" mapstructure:"provider"`
	Model           string `json:"model" mapstructure:"model"`
	MaxOutputTokens int    `json:"max_output_tokens" mapstructure:"max_output_tokens"`
}

// QualificationBudgetConnection identifies a connection before it has a tenant assignment.
type QualificationBudgetConnection struct {
	nameKey  string
	Provider string `json:"provider" mapstructure:"provider"`
	Name     string `json:"name" mapstructure:"name"`
}

func validateQualificationBudget(configuration *QualificationBudgetConfiguration, managementPath string) error {
	if configuration == nil {
		return nil
	}
	if configuration.BudgetID == "" || configuration.BudgetID != strings.TrimSpace(configuration.BudgetID) || configuration.TenantID == "" || configuration.RemainingMicroUSD <= 0 || configuration.MaxAttemptsPerRequest < 1 || configuration.MaxAttemptsPerRequest > 32 || len(configuration.Offerings) == 0 {
		return fmt.Errorf("%w: invalid policy fields", errQualificationBudget)
	}
	expiry, err := time.Parse(time.RFC3339, configuration.ExpiresAt)
	if err != nil || expiry.Format(time.RFC3339) != configuration.ExpiresAt || !strings.HasSuffix(configuration.ExpiresAt, "Z") {
		return fmt.Errorf("%w: invalid expiry", errQualificationBudget)
	}
	path := configuration.ReceiptDatabasePath
	if strings.ContainsAny(path, "?#\x00") || !filepath.IsAbs(path) || filepath.Clean(path) != path || path == managementPath || filepath.Dir(path) != filepath.Dir(managementPath) {
		return fmt.Errorf("%w: receipt database must share the management volume directory", errQualificationBudget)
	}
	seen := map[string]bool{}
	for _, offering := range configuration.Offerings {
		key := offering.Provider + "/" + offering.Model
		if offering.Provider == "" || offering.Model == "" || offering.MaxOutputTokens < 1 || offering.MaxOutputTokens > 32768 || seen[key] {
			return fmt.Errorf("%w: invalid offering", errQualificationBudget)
		}
		if offering.PriceRegion != "" && !catalogPriceRegionPattern.MatchString(offering.PriceRegion) {
			return fmt.Errorf("%w: invalid offering price region", errQualificationBudget)
		}
		seen[key] = true
	}
	seen = map[string]bool{}
	for index := range configuration.VerificationConnections {
		connection := &configuration.VerificationConnections[index]
		name, err := newManagedTenantName(connection.Name)
		key := connection.Provider + "/" + name.key
		if err != nil || name.display != connection.Name || connection.Provider == "" || seen[key] {
			return fmt.Errorf("%w: invalid verification connection", errQualificationBudget)
		}
		seen[key] = true
		connection.nameKey = name.key
	}
	return nil
}

func qualificationPolicyJSON(configuration QualificationBudgetConfiguration) string {
	encoded, _ := json.Marshal(configuration)
	return string(encoded)
}
