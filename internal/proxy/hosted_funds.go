package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	hostedLedgerTenant             = "llm-proxy"
	fundsAccountActive             = "active"
	fundsReservationHeld           = "held"
	fundsReservationSettled        = "settled"
	fundsReservationReleased       = "released"
	fundsReservationReconciliation = "reconciliation_required"
)

var (
	errInsufficientFunds             = errors.New(llmproxycontract.ErrorCodeInsufficientFunds)
	errFinancialAdmissionUnavailable = errors.New(llmproxycontract.ErrorCodeFinancialAdmissionUnavailable)
	errFinancialAccountSuspended     = fmt.Errorf("%w: financial account is suspended", errHostedAuthorityDenied)
)

type managedFundsAccountRecord struct {
	BillingAccountID     string                      `gorm:"primaryKey"`
	BillingAccount       managedBillingAccountRecord `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	State                string                      `gorm:"not null;check:state IN ('active','suspended','reconciliation_required')"`
	RemainderNumerator   string                      `gorm:"not null"`
	RemainderDenominator string                      `gorm:"not null"`
	CreatedAt            time.Time                   `gorm:"not null"`
}

type managedFundsReservationRecord struct {
	RequestID        string                      `gorm:"primaryKey"`
	Request          managedJournalRequestRecord `gorm:"foreignKey:RequestID;references:ID;constraint:OnDelete:RESTRICT"`
	BillingAccountID string                      `gorm:"not null;index"`
	BillingAccount   managedBillingAccountRecord `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	MaximumCents     int64                       `gorm:"not null;check:maximum_cents >= 0"`
	Currency         string                      `gorm:"not null;check:currency = 'USD'"`
	State            string                      `gorm:"not null;check:state IN ('held','settled','released','reconciliation_required')"`
	Revision         uint64                      `gorm:"not null;check:revision > 0"`
	CreatedAt        time.Time                   `gorm:"not null"`
	UpdatedAt        time.Time                   `gorm:"not null"`
}

func initializeHostedFundsSchema(database *gorm.DB) error {
	models := []any{&gormstore.LedgerAccount{}, &gormstore.LedgerEntry{}, &gormstore.Reservation{}, &managedFundsAccountRecord{}, &managedFundsReservationRecord{}, &managedFundsSettlementRecord{}, &managedFundsCreditRecord{}, &managedFundsTenantRecord{}, &managedFundsResolutionRecord{}, &managedFundsExposureRecord{}, &managedFundsCorrectionRecord{}}
	present := 0
	for _, model := range models {
		if database.Migrator().HasTable(model) {
			present++
		}
	}
	if present == 0 {
		if err := database.AutoMigrate(models...); err != nil {
			return fmt.Errorf("%w: create hosted funds schema: %w", errManagedTenantSchemaMigration, err)
		}
		return nil
	}
	if present != len(models) {
		return fmt.Errorf("%w: partial hosted funds schema", errManagedTenantSchemaMigration)
	}
	for _, model := range models {
		if err := validateHostedTable(database, model); err != nil {
			return fmt.Errorf("%w: validate hosted funds schema: %w", errManagedTenantSchemaMigration, err)
		}
	}
	return nil
}

type hostedLedgerAccount struct {
	service   *ledger.Service
	tenant    ledger.TenantID
	user      ledger.UserID
	namespace ledger.LedgerID
}

func newHostedLedgerAccount(transaction *gorm.DB, accountID string, now time.Time) (hostedLedgerAccount, error) {
	tenant, err := ledger.NewTenantID(hostedLedgerTenant)
	if err != nil {
		return hostedLedgerAccount{}, err
	}
	user, err := ledger.NewUserID(accountID)
	if err != nil {
		return hostedLedgerAccount{}, fmt.Errorf("construct ledger account %s: %w", accountID, err)
	}
	namespace, err := ledger.NewLedgerID(CatalogCurrencyUSD)
	if err != nil {
		return hostedLedgerAccount{}, err
	}
	service, err := ledger.NewService(gormstore.New(transaction), func() int64 { return now.Unix() })
	if err != nil {
		return hostedLedgerAccount{}, fmt.Errorf("construct ledger service for account %s: %w", accountID, err)
	}
	return hostedLedgerAccount{service: service, tenant: tenant, user: user, namespace: namespace}, nil
}

// The existing journal transaction owns the accepted price and Ledger hold.
// Later attempts check the retained reservation without reserving it again.
func newHostedFundsAdmission(prices journalReservation) journalReservation {
	return func(transaction *gorm.DB, request managedJournalRequestRecord) error {
		if err := prices(transaction, request); err != nil {
			if errors.Is(err, errUsageJournalConflict) {
				return err
			}
			return fmt.Errorf("%w: retain request price: %w", errFinancialAdmissionUnavailable, err)
		}
		if err := reserveHostedFunds(transaction, request); err != nil {
			if errors.Is(err, errInsufficientFunds) || errors.Is(err, errHostedAuthorityDenied) {
				return err
			}
			return fmt.Errorf("%w: reserve request %s: %w", errFinancialAdmissionUnavailable, request.ID, err)
		}
		return nil
	}
}

func reserveHostedFunds(transaction *gorm.DB, request managedJournalRequestRecord) error {
	// This writer lock serializes all tenants of the billing account before
	// reading its financial state or invoking shared Ledger operations.
	lock := transaction.Model(&managedBillingAccountRecord{}).Where("id = ?", request.BillingAccountID).UpdateColumn("id", gorm.Expr("id"))
	if lock.Error != nil {
		return fmt.Errorf("lock billing account %s: %w", request.BillingAccountID, lock.Error)
	}
	if lock.RowsAffected != 1 {
		return errHostedAuthorityDenied
	}
	financial := managedFundsAccountRecord{BillingAccountID: request.BillingAccountID, State: fundsAccountActive, RemainderNumerator: "0", RemainderDenominator: "1", CreatedAt: request.CreatedAt}
	if err := transaction.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&financial).Error; err != nil {
		return fmt.Errorf("create financial account %s: %w", request.BillingAccountID, err)
	}
	if err := transaction.Where("billing_account_id = ?", request.BillingAccountID).First(&financial).Error; err != nil {
		return fmt.Errorf("read financial account %s: %w", request.BillingAccountID, err)
	}
	if financial.State != fundsAccountActive {
		return errFinancialAccountSuspended
	}
	var retained managedPriceSnapshotRecord
	if err := transaction.Where("request_id = ? AND billing_account_id = ?", request.ID, request.BillingAccountID).First(&retained).Error; err != nil {
		return fmt.Errorf("read reserved price for request %s: %w", request.ID, err)
	}
	_, document, err := restoreHostedPriceSnapshot(retained)
	if err != nil {
		return err
	}
	var existing managedFundsReservationRecord
	err = transaction.Where("request_id = ?", request.ID).First(&existing).Error
	if err == nil {
		if existing.BillingAccountID != request.BillingAccountID || existing.MaximumCents != document.Maximum.ReservedCents || existing.State != fundsReservationHeld {
			return fmt.Errorf("request %s has no matching active funds reservation", request.ID)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("read reservation for request %s: %w", request.ID, err)
	}
	maximum := document.Maximum.ReservedCents
	if err := reserveHostedTenantFunds(transaction, request, maximum); err != nil {
		return err
	}
	if maximum > 0 {
		account, err := newHostedLedgerAccount(transaction, request.BillingAccountID, request.CreatedAt)
		if err != nil {
			return err
		}
		amount, err := ledger.NewPositiveAmountCents(maximum)
		if err != nil {
			return err
		}
		reservationID, err := ledger.NewReservationID(request.ID)
		if err != nil {
			return err
		}
		key, err := ledger.NewIdempotencyKey("reserve:" + request.ID)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(struct {
			RequestID string `json:"request_id"`
		}{request.ID})
		if err != nil {
			return err
		}
		metadata, err := ledger.NewMetadataJSON(string(encoded))
		if err != nil {
			return err
		}
		// A request timeout cannot release funds after an uncertain dispatch.
		if err := account.service.Reserve(transaction.Statement.Context, account.tenant, account.user, account.namespace, amount, reservationID, key, 0, metadata); err != nil {
			if errors.Is(err, ledger.ErrInsufficientFunds) {
				return errInsufficientFunds
			}
			return fmt.Errorf("reserve shared ledger funds: %w", err)
		}
	}
	record := managedFundsReservationRecord{RequestID: request.ID, BillingAccountID: request.BillingAccountID, Currency: CatalogCurrencyUSD, MaximumCents: maximum, State: fundsReservationHeld, Revision: 1, CreatedAt: request.CreatedAt, UpdatedAt: request.CreatedAt}
	if err := transaction.Omit(clause.Associations).Create(&record).Error; err != nil {
		return fmt.Errorf("record reservation for request %s: %w", request.ID, err)
	}
	return nil
}
