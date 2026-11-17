package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"time"

	"gorm.io/gorm"
)

type qualificationBudgetRegistry struct {
	ID           string `gorm:"primaryKey"`
	Policy       string `gorm:"not null"`
	Closed       bool
	FrozenDigest string
}
type qualificationBudgetState struct {
	ID        string `gorm:"primaryKey"`
	Policy    string `gorm:"not null"`
	Frozen    string `gorm:"not null"`
	Remaining int64
	Suspended bool
	Closed    bool
}
type qualificationBudgetAttempt struct {
	ID              uint64 `gorm:"primaryKey"`
	LogicalID       string
	RequestID       string
	Provider        string
	Model           string
	ConnectionID    string
	InputTokens     *uint64
	OutputTokens    *uint64
	CatalogRevision string
	Reserved        int64
	Cost            int64
	State           string
	ResourceID      string
	Active          bool
	CreatedAt       time.Time
}
type qualificationFrozenOffering struct {
	PriceRegion   string
	Source        string
	LastVerified  string
	Provider      string
	Model         string
	ProviderModel string
	Codec         string
	InputCeiling  uint64
	OutputCeiling uint64
	MaxOutput     int
	Rates         []CatalogPriceRate
	Revision      string
}
type qualificationBudget struct {
	mutex        sync.Mutex
	database     *gorm.DB
	policy       QualificationBudgetConfiguration
	frozen       []qualificationFrozenOffering
	now          func() time.Time
	suspended    bool
	verification map[string]bool
	lockFile     *os.File
}

func newQualificationBudget(configuration *QualificationBudgetConfiguration, database *gormManagedTenantDatabase, catalog ModelCatalog) (*qualificationBudget, error) {
	if database == nil {
		if configuration != nil {
			return nil, fmt.Errorf("%w: managed database required", errQualificationBudget)
		}
		return nil, nil
	}
	if configuration == nil {
		if database.database.Migrator().HasTable(&qualificationBudgetRegistry{}) {
			var registries []qualificationBudgetRegistry
			if err := database.database.Find(&registries).Error; err != nil {
				return nil, err
			}
			for _, registry := range registries {
				if !registry.Closed {
					return nil, fmt.Errorf("%w: initialized policy cannot be removed", errQualificationBudget)
				}
				if err := validateClosedQualificationRegistry(registry); err != nil {
					return nil, err
				}
			}
		}
		return nil, nil
	}
	policy := *configuration
	policy.Offerings = slices.Clone(configuration.Offerings)
	policy.VerificationConnections = slices.Clone(configuration.VerificationConnections)
	tenant, err := database.tenantByTenantID(context.Background(), policy.TenantID)
	if err != nil {
		return nil, fmt.Errorf("%w: policy tenant unavailable", errQualificationBudget)
	}
	if policy.OwnerUserID != "" && policy.OwnerUserID != tenant.OwnerUserID {
		return nil, fmt.Errorf("%w: policy owner differs", errQualificationBudget)
	}
	policy.OwnerUserID = tenant.OwnerUserID
	if err := database.database.AutoMigrate(&qualificationBudgetRegistry{}); err != nil {
		return nil, err
	}
	var registry qualificationBudgetRegistry
	err = database.database.First(&registry, "id = ?", policy.BudgetID).Error
	initialized := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	encoded := qualificationPolicyJSON(policy)
	if initialized && (registry.Closed || registry.Policy != encoded) {
		return nil, fmt.Errorf("%w: initialized policy is immutable", errQualificationBudget)
	}
	var prepared []qualificationFrozenOffering
	if !initialized {
		prepared, err = freezeQualificationOfferings(policy, catalog)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(policy.ReceiptDatabasePath); !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: unregistered journal path exists or unavailable", errQualificationBudget)
		}
		// Publish the durable initialization marker before creating the receipt store.
		frozenDocument, _ := json.Marshal(prepared)
		if err := database.database.Transaction(func(tx *gorm.DB) error {
			if err := lockQualificationBudgetRegistry(tx).Error; err != nil {
				return err
			}
			var active int64
			if err := tx.Model(&qualificationBudgetRegistry{}).Where("closed = ?", false).Count(&active).Error; err != nil {
				return err
			}
			if active != 0 {
				return fmt.Errorf("%w: another initialized budget remains active", errQualificationBudget)
			}
			return tx.Create(&qualificationBudgetRegistry{ID: policy.BudgetID, Policy: encoded, FrozenDigest: sha256Hex(string(frozenDocument))}).Error
		}); err != nil {
			return nil, err
		}
	} else if _, err := os.Stat(policy.ReceiptDatabasePath); err != nil {
		return nil, fmt.Errorf("%w: initialized journal missing", errQualificationBudget)
	}
	lockFile, err := os.Create(policy.ReceiptDatabasePath + ".lock")
	if err != nil {
		return nil, fmt.Errorf("%w: journal lock unavailable", errQualificationBudget)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("%w: journal already owned by an active process", errQualificationBudget)
	}
	lockTransferred := false
	defer func() {
		if !lockTransferred {
			_ = lockFile.Close()
		}
	}()
	receiptDB, err := openQualificationSQLite(policy.ReceiptDatabasePath)
	if err != nil {
		return nil, fmt.Errorf("%w: journal open", errQualificationBudget)
	}
	sqlDB, _ := receiptDB.DB()
	sqlDB.SetMaxOpenConns(1)
	fail := func(err error) (*qualificationBudget, error) { _ = sqlDB.Close(); return nil, err }
	if initialized {
		if err := validateQualificationJournalIntegrity(receiptDB); err != nil {
			return fail(err)
		}
	} else if err := receiptDB.AutoMigrate(&qualificationBudgetState{}, &qualificationBudgetAttempt{}); err != nil {
		return fail(err)
	}
	budget := &qualificationBudget{database: receiptDB, policy: policy, now: time.Now, lockFile: lockFile}
	budget.verification = map[string]bool{}
	for _, connection := range configuration.VerificationConnections {
		budget.verification[connection.Provider+"/"+connection.nameKey] = true
	}
	if initialized {
		var state qualificationBudgetState
		if err := receiptDB.First(&state, "id = ?", policy.BudgetID).Error; err != nil || state.Policy != encoded || state.Closed {
			return fail(fmt.Errorf("%w: journal identity", errQualificationBudget))
		}
		if sha256Hex(state.Frozen) != registry.FrozenDigest {
			return fail(fmt.Errorf("%w: frozen catalog integrity", errQualificationBudget))
		}
		if err := json.Unmarshal([]byte(state.Frozen), &budget.frozen); err != nil {
			return fail(fmt.Errorf("%w: frozen catalog", errQualificationBudget))
		}
		if err := validateQualificationAccounting(receiptDB, policy, state); err != nil {
			return fail(err)
		}
		var pending int64
		if err := receiptDB.Model(&qualificationBudgetAttempt{}).Where("state NOT IN ?", []string{"settled", "conservative"}).Count(&pending).Error; err != nil {
			return fail(err)
		}
		if pending > 0 {
			if err := receiptDB.Model(&qualificationBudgetAttempt{}).Where("state NOT IN ?", []string{"settled", "conservative"}).Update("active", false).Error; err != nil {
				return fail(err)
			}
			if err := receiptDB.Model(&state).Update("suspended", true).Error; err != nil {
				return fail(err)
			}
		}
	} else {
		budget.frozen = prepared
		frozen, _ := json.Marshal(budget.frozen)
		if err := receiptDB.Create(&qualificationBudgetState{ID: policy.BudgetID, Policy: encoded, Frozen: string(frozen), Remaining: policy.RemainingMicroUSD}).Error; err != nil {
			return fail(err)
		}
	}
	lockTransferred = true
	return budget, nil
}

func freezeQualificationOfferings(policy QualificationBudgetConfiguration, catalog ModelCatalog) ([]qualificationFrozenOffering, error) {
	result := []qualificationFrozenOffering{}
	for _, allowed := range policy.Offerings {
		var frozen qualificationFrozenOffering
		found := false
		for _, offering := range catalog.Offerings {
			if offering.Provider != allowed.Provider || offering.Model != allowed.Model {
				continue
			}
			found = true
			frozen = qualificationFrozenOffering{Provider: allowed.Provider, Model: allowed.Model, PriceRegion: allowed.PriceRegion, ProviderModel: offering.ProviderModel, Codec: offering.WireContract, MaxOutput: allowed.MaxOutputTokens, OutputCeiling: uint64(offering.OutputTokenLimit), Revision: catalog.Revision}
			// Wire contracts and codec identities coincide for the supported shared text protocols.
			for _, limit := range offering.Limits {
				if (limit.ID == "input_tokens" || limit.ID == "context_tokens") && limit.Unit == "tokens" && !limit.AccountDependent && limit.Value != nil && *limit.Value > 0 && (frozen.InputCeiling == 0 || uint64(*limit.Value) < frozen.InputCeiling) {
					frozen.InputCeiling = uint64(*limit.Value)
				}
			}
		}
		if !found || frozen.InputCeiling == 0 || frozen.OutputCeiling == 0 || uint64(allowed.MaxOutputTokens) > frozen.OutputCeiling {
			return nil, fmt.Errorf("%w: offering or token ceilings unavailable", errQualificationBudget)
		}
		for _, price := range catalog.Prices {
			if price.Provider == allowed.Provider && price.Model == allowed.Model && price.Operation == ModelOperationText && price.Available {
				if price.MinimumCharge != nil {
					return nil, fmt.Errorf("%w: minimum charges are outside token-only qualification", errQualificationBudget)
				}
				frozen.Rates = slices.Clone(price.Rates)
				frozen.Source = price.Source
				frozen.LastVerified = price.LastVerified
			}
		}
		if _, err := frozen.estimate(frozen.InputCeiling, uint64(frozen.MaxOutput), true, time.Now()); err != nil {
			return nil, err
		}
		result = append(result, frozen)
	}
	return result, nil
}

func (offering qualificationFrozenOffering) estimate(input, output uint64, worst bool, at time.Time) (int64, error) {
	var inputs, outputs []CatalogPriceRate
	var selectedConditions *CatalogPriceConditions
	for _, rate := range offering.Rates {
		if rate.Conditions.CacheClass != "" {
			continue
		}
		if rate.Unit != "USD/1M_tokens" || rate.Conditions.InputTokens.UnresolvedReason != "" {
			return 0, fmt.Errorf("%w: unsupported list price", errQualificationBudget)
		}
		conditions := rate.Conditions
		categorical := categoricalPriceConditions(conditions)
		if conditions.ServiceTier != "" && conditions.ServiceTier != "standard" || conditions.BillingMode != "standard" && conditions.BillingMode != "pay_as_you_go_list" || categorical != (CatalogPriceConditions{ServiceTier: conditions.ServiceTier, BillingMode: conditions.BillingMode, Region: offering.PriceRegion}) {
			return 0, fmt.Errorf("%w: unsupported price conditions", errQualificationBudget)
		}
		if !catalogPriceActiveAt(conditions, at) {
			continue
		}
		if selectedConditions != nil && *selectedConditions != categorical {
			return 0, fmt.Errorf("%w: ambiguous list price conditions", errQualificationBudget)
		}
		selectedConditions = &categorical
		if !worst && (input < conditions.InputTokens.Minimum || conditions.InputTokens.MaximumExclusive != 0 && input >= conditions.InputTokens.MaximumExclusive) {
			continue
		}
		switch rate.Component {
		case "input_tokens", "input_text":
			if len(inputs) > 0 && inputs[0].Component != rate.Component {
				return 0, fmt.Errorf("%w: ambiguous input price dimensions", errQualificationBudget)
			}
			inputs = append(inputs, rate)
		case "output_tokens", "output_text":
			if len(outputs) > 0 && outputs[0].Component != rate.Component {
				return 0, fmt.Errorf("%w: ambiguous output price dimensions", errQualificationBudget)
			}
			outputs = append(outputs, rate)
		default:
			return 0, fmt.Errorf("%w: unsupported billable component", errQualificationBudget)
		}
	}
	if len(inputs) == 0 || len(outputs) == 0 {
		return 0, fmt.Errorf("%w: exact list rates unavailable", errQualificationBudget)
	}
	inputRate, outputRate := inputs[0], outputs[0]
	if worst {
		measurements := map[string]journalQuantity{"input_tokens": {Dimension: "input_tokens", Unit: "token", Value: strconv.FormatUint(input, 10)}}
		var err error
		inputRate, err = maximumCatalogRate(catalogSnapshotComponent{rates: inputs}, measurements)
		if err != nil {
			return 0, errors.Join(errQualificationBudget, err)
		}
		outputRate, err = maximumCatalogRate(catalogSnapshotComponent{rates: outputs}, measurements)
		if err != nil {
			return 0, errors.Join(errQualificationBudget, err)
		}
	}
	// USD per million tokens times tokens is microUSD. Round each component upward.
	total := new(big.Int)
	for _, component := range []struct {
		rate   CatalogDecimal
		tokens uint64
	}{{inputRate.Rate, input}, {outputRate.Rate, output}} {
		amount := new(big.Rat).Mul(ratingRational(string(component.rate)), new(big.Rat).SetInt(new(big.Int).SetUint64(component.tokens)))
		rounded := new(big.Int).Quo(amount.Num(), amount.Denom())
		if new(big.Int).Mod(amount.Num(), amount.Denom()).Sign() != 0 {
			rounded.Add(rounded, big.NewInt(1))
		}
		total.Add(total, rounded)
	}
	if !total.IsInt64() {
		return 0, errQualificationBudget
	}
	return total.Int64(), nil
}

func (budget *qualificationBudget) reserve(logical *qualificationExecution, offering qualificationFrozenOffering, connection string) (qualificationBudgetAttempt, error) {
	budget.mutex.Lock()
	defer budget.mutex.Unlock()
	var attempt qualificationBudgetAttempt
	err := budget.database.Transaction(func(tx *gorm.DB) error {
		// Obtain the SQLite write lock before reading available funds.
		if err := lockQualificationBudgetState(tx, budget.policy.BudgetID).Error; err != nil {
			return err
		}
		var state qualificationBudgetState
		if err := tx.First(&state, "id = ?", budget.policy.BudgetID).Error; err != nil {
			return err
		}
		if budget.suspended && !state.Suspended {
			var unresolved int64
			if err := tx.Model(&qualificationBudgetAttempt{}).Where("state NOT IN ?", []string{"settled", "conservative"}).Count(&unresolved).Error; err != nil {
				return err
			}
			if unresolved != 0 {
				return errQualificationBudget
			}
			budget.suspended = false
		}
		acceptedAt := budget.now().UTC()
		expiry, _ := time.Parse(time.RFC3339, budget.policy.ExpiresAt)
		if state.Closed || state.Suspended || !acceptedAt.Before(expiry) {
			return fmt.Errorf("%w: suspended or expired", errQualificationBudget)
		}
		var count int64
		if err := tx.Model(&qualificationBudgetAttempt{}).Where("logical_id = ?", logical.id).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(budget.policy.MaxAttemptsPerRequest) {
			return fmt.Errorf("%w: attempt limit", errQualificationBudget)
		}
		maximum, err := offering.estimate(offering.InputCeiling, uint64(offering.MaxOutput), true, acceptedAt)
		if err != nil {
			return err
		}
		if state.Remaining < maximum {
			return fmt.Errorf("%w: cap exhausted", errQualificationBudget)
		}
		attempt = qualificationBudgetAttempt{LogicalID: logical.id, RequestID: logical.requestID, Provider: offering.Provider, Model: offering.Model, ConnectionID: connection, CatalogRevision: offering.Revision, Reserved: maximum, State: "dispatched", Active: true, CreatedAt: acceptedAt}
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}
		return tx.Model(&state).Update("remaining", state.Remaining-maximum).Error
	})
	return attempt, err
}

// releaseNotDispatched settles a proven unsubmitted generation at zero cost.
func (budget *qualificationBudget) releaseNotDispatched(attempt qualificationBudgetAttempt) error {
	return budget.database.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&qualificationBudgetAttempt{}).Where("id = ? AND state = ? AND resource_id = ?", attempt.ID, "dispatched", "").Updates(map[string]any{"state": "settled", "active": false, "cost": 0})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: attempt cannot be released before dispatch", errQualificationBudget)
		}
		return tx.Model(&qualificationBudgetState{}).Where("id = ?", budget.policy.BudgetID).UpdateColumn("remaining", gorm.Expr("remaining + ?", attempt.Reserved)).Error
	})
}

func (budget *qualificationBudget) suspend() error {
	budget.mutex.Lock()
	defer budget.mutex.Unlock()
	budget.suspended = true
	return budget.database.Model(&qualificationBudgetState{}).Where("id = ?", budget.policy.BudgetID).Update("suspended", true).Error
}
func (budget *qualificationBudget) settle(attempt qualificationBudgetAttempt, offering qualificationFrozenOffering, evidence journalUsageEvidenceInput) error {
	var input, output, reasoning uint64
	reasoningKnown := false
	knownInput, knownOutput := false, false
	for _, quantity := range evidence.Quantities {
		switch quantity.Dimension {
		case "input_tokens", "output_tokens":
			if quantity.UnknownReason != "" {
				return fmt.Errorf("%w: incomplete usage", errQualificationBudget)
			}
			value, err := strconv.ParseUint(quantity.Value, 10, 64)
			if err != nil {
				return err
			}
			if quantity.Dimension == "input_tokens" {
				input = value
				knownInput = true
			} else {
				output = value
				knownOutput = true
			}
		case "reasoning_tokens":
			if (offering.Codec == CatalogProtocolGeminiInteractions || offering.Codec == CatalogProtocolVertexGenerateContent) && quantity.UnknownReason == "" {
				value, err := strconv.ParseUint(quantity.Value, 10, 64)
				if err != nil {
					return err
				}
				reasoning = value
				reasoningKnown = true
			}
		}
	}
	if !knownInput || !knownOutput {
		return fmt.Errorf("%w: missing usage", errQualificationBudget)
	}
	if offering.Codec == CatalogProtocolGeminiInteractions || offering.Codec == CatalogProtocolVertexGenerateContent {
		if !reasoningKnown || reasoning > uint64(offering.MaxOutput) || output > uint64(offering.MaxOutput)-reasoning {
			return fmt.Errorf("%w: missing or excessive separate reasoning usage", errQualificationBudget)
		}
		output += reasoning
	}
	if offering.Codec == CatalogProtocolAnthropicMessages {
		for _, quantity := range evidence.Quantities {
			if quantity.Dimension == "cache_read_tokens" || quantity.Dimension == "cache_write_tokens" {
				if quantity.UnknownReason != "" {
					return fmt.Errorf("%w: missing cache usage", errQualificationBudget)
				}
				value, err := strconv.ParseUint(quantity.Value, 10, 64)
				if err != nil {
					return err
				}
				if value > offering.InputCeiling || input > offering.InputCeiling-value {
					return fmt.Errorf("%w: excessive input usage", errQualificationBudget)
				}
				input += value
			}
		}
	}
	if input > offering.InputCeiling || output > uint64(offering.MaxOutput) {
		return fmt.Errorf("%w: usage exceeds enforced bounds", errQualificationBudget)
	}
	cost, err := offering.estimate(input, output, false, attempt.CreatedAt)
	if err != nil {
		return err
	}
	// Enforced quantities and the maximum active rates at this attempt's
	// single acceptance timestamp make cost no greater than its reservation.
	return budget.database.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&qualificationBudgetAttempt{}).Where("id = ? AND state IN ?", attempt.ID, []string{"dispatched", "pending"}).Updates(map[string]any{"state": "settled", "active": false, "cost": cost, "input_tokens": input, "output_tokens": output})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: attempt already settled", errQualificationBudget)
		}
		return tx.Model(&qualificationBudgetState{}).Where("id = ?", budget.policy.BudgetID).UpdateColumn("remaining", gorm.Expr("remaining + ?", attempt.Reserved-cost)).Error
	})
}

func validateQualificationAccounting(database *gorm.DB, policy QualificationBudgetConfiguration, state qualificationBudgetState) error {
	if state.Remaining < 0 || state.Remaining > policy.RemainingMicroUSD {
		return fmt.Errorf("%w: invalid remaining allowance", errQualificationBudget)
	}
	var attempts []qualificationBudgetAttempt
	if err := database.Find(&attempts).Error; err != nil {
		return err
	}
	total := big.NewInt(state.Remaining)
	for _, attempt := range attempts {
		if attempt.Reserved < 0 || attempt.Cost < 0 || attempt.Cost > attempt.Reserved || attempt.LogicalID == "" || attempt.RequestID == "" || attempt.ConnectionID == "" {
			return fmt.Errorf("%w: invalid attempt receipt", errQualificationBudget)
		}
		switch attempt.State {
		case "settled", "conservative":
			total.Add(total, big.NewInt(attempt.Cost))
		case "dispatched", "pending":
			total.Add(total, big.NewInt(attempt.Reserved))
		default:
			return fmt.Errorf("%w: invalid attempt state", errQualificationBudget)
		}
	}
	if total.Cmp(big.NewInt(policy.RemainingMicroUSD)) != 0 {
		return fmt.Errorf("%w: allowance conservation failed", errQualificationBudget)
	}
	return nil
}

func validateClosedQualificationRegistry(registry qualificationBudgetRegistry) error {
	var policy QualificationBudgetConfiguration
	if err := json.Unmarshal([]byte(registry.Policy), &policy); err != nil {
		return errQualificationBudget
	}
	if _, err := os.Stat(policy.ReceiptDatabasePath); err != nil {
		return fmt.Errorf("%w: closed receipts missing", errQualificationBudget)
	}
	database, err := openQualificationSQLite(policy.ReceiptDatabasePath)
	if err != nil {
		return err
	}
	sqlDB, _ := database.DB()
	defer sqlDB.Close()
	var state qualificationBudgetState
	if err := database.First(&state, "id = ?", policy.BudgetID).Error; err != nil || !state.Closed || state.Policy != registry.Policy || sha256Hex(state.Frozen) != registry.FrozenDigest {
		return fmt.Errorf("%w: closed receipt integrity", errQualificationBudget)
	}
	return validateQualificationAccounting(database, policy, state)
}

func (budget *qualificationBudget) close() error {
	database, _ := budget.database.DB()
	return errors.Join(database.Close(), budget.lockFile.Close())
}
