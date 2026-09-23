package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	managementFundingOffersPath       = managementBillingAccountPath + "/funding-offers"
	managementFundingOrdersPath       = managementBillingAccountPath + "/funding-orders"
	managementFundingOrderPath        = managementFundingOrdersPath + "/:order_id"
	fundingOrderIDPrefix              = "funding-"
	fundingOrderCreated               = "created"
	paymentDeliveryPending            = "pending"
	fundingMinimumCents         int64 = 500
)

var (
	errFundingInvalid     = errors.New("funding_invalid")
	errFundingConflict    = errors.New("funding_conflict")
	errFundingNotFound    = errors.New("funding_not_found")
	errFundingUnavailable = errors.New("funding_unavailable")
)

type fundingOfferInput struct {
	Code         string
	PriceID      string
	FundingCents int64
}

type fundingCatalog struct {
	environment        string
	processorAccountID string
	supplierID         string
	offers             map[string]fundingOfferInput
}

func newFundingCatalog(environment, processorAccountID, supplierID string, offers []fundingOfferInput) (*fundingCatalog, error) {
	if (environment != paymentEnvironmentSandbox && environment != paymentEnvironmentProduction) || !validIdempotencyKey(processorAccountID) || !validIdempotencyKey(supplierID) || len(offers) == 0 {
		return nil, fmt.Errorf("configure funding catalog: invalid processor identity or offers")
	}
	catalog := &fundingCatalog{environment: environment, processorAccountID: processorAccountID, supplierID: supplierID, offers: make(map[string]fundingOfferInput, len(offers))}
	for _, offer := range offers {
		_, duplicate := catalog.offers[offer.Code]
		if duplicate || !journalDimensionPattern.MatchString(offer.Code) || len(offer.Code) > 64 || len(offer.PriceID) != 30 || offer.PriceID[:4] != "pri_" || !paddleEntityIDPattern.MatchString(offer.PriceID) || offer.FundingCents < fundingMinimumCents {
			return nil, fmt.Errorf("configure funding offer %q: invalid code, price, or minimum amount", offer.Code)
		}
		catalog.offers[offer.Code] = offer
	}
	return catalog, nil
}

type managedFundingOrderRecord struct {
	ID                 string                      `gorm:"primaryKey"`
	BillingAccountID   string                      `gorm:"not null;index;uniqueIndex:funding_order_creation,priority:1"`
	BillingAccount     managedBillingAccountRecord `gorm:"foreignKey:BillingAccountID;references:ID;constraint:OnDelete:RESTRICT"`
	CreationKeyDigest  string                      `gorm:"not null;uniqueIndex:funding_order_creation,priority:2"`
	OfferCode          string                      `gorm:"not null"`
	PriceID            string                      `gorm:"not null"`
	FundingCents       int64                       `gorm:"not null;check:funding_cents >= 500"`
	Currency           string                      `gorm:"not null;check:currency = 'USD'"`
	Environment        string                      `gorm:"not null;check:environment IN ('sandbox','production')"`
	ProcessorAccountID string                      `gorm:"not null"`
	SupplierID         string                      `gorm:"not null"`
	State              string                      `gorm:"not null;check:state IN ('created','pending','paid','failed','partially_refunded','refunded','disputed')"`
	CreatedAt          time.Time                   `gorm:"not null"`
}

type managedPaymentDeliveryRecord struct {
	OrderID   string                    `gorm:"primaryKey"`
	Order     managedFundingOrderRecord `gorm:"foreignKey:OrderID;references:ID;constraint:OnDelete:RESTRICT"`
	State     string                    `gorm:"not null;index;check:state IN ('pending','dispatching','delivered','reconciliation_required')"`
	CreatedAt time.Time                 `gorm:"not null"`
}

func initializeFundingOrdersSchema(database *gorm.DB) error {
	models := []any{&managedFundingOrderRecord{}, &managedPaymentDeliveryRecord{}}
	present := 0
	for _, model := range models {
		if database.Migrator().HasTable(model) {
			present++
		}
	}
	if present == 0 {
		if err := database.AutoMigrate(models...); err != nil {
			return fmt.Errorf("%w: create funding orders: %w", errManagedTenantSchemaMigration, err)
		}
		return nil
	}
	if present != len(models) {
		return fmt.Errorf("%w: partial funding order schema", errManagedTenantSchemaMigration)
	}
	for _, model := range models {
		if err := validateHostedTable(database, model); err != nil {
			return fmt.Errorf("%w: validate funding orders: %w", errManagedTenantSchemaMigration, err)
		}
	}
	return nil
}

type managementFundingOfferResponse struct {
	Code         string `json:"code"`
	FundingCents string `json:"funding_cents"`
	Currency     string `json:"currency"`
}

type managementFundingOrderResponse struct {
	ID           string `json:"id"`
	OfferCode    string `json:"offer_code"`
	FundingCents string `json:"funding_cents"`
	Currency     string `json:"currency"`
	Environment  string `json:"environment"`
	State        string `json:"state"`
	CreatedAt    string `json:"created_at"`
}

func fundingOrderResponse(record managedFundingOrderRecord) managementFundingOrderResponse {
	return managementFundingOrderResponse{ID: record.ID, OfferCode: record.OfferCode, FundingCents: strconv.FormatInt(record.FundingCents, 10), Currency: record.Currency, Environment: record.Environment, State: record.State, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano)}
}

func (database *gormManagedTenantDatabase) createFundingOrder(ctx context.Context, catalog *fundingCatalog, accountID, offerCode, key string, now time.Time) (managedFundingOrderRecord, error) {
	identifier := fundingOrderIDPrefix + sha256Hex(accountID + "\x00" + key)[:32]
	var record managedFundingOrderRecord
	privateQuery := database.database.Session(&gorm.Session{Logger: paymentInboxLogger{database.database.Logger}})
	err := privateQuery.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Acquire the write lock before inspecting idempotency or current offers.
		if err := tx.Model(&managedBillingAccountRecord{}).Where("id = ?", accountID).UpdateColumn("id", accountID).Error; err != nil {
			return err
		}
		err := tx.Where("id = ? AND billing_account_id = ?", identifier, accountID).First(&record).Error
		if err == nil {
			if record.OfferCode != offerCode || record.Environment != catalog.environment || record.ProcessorAccountID != catalog.processorAccountID || record.SupplierID != catalog.supplierID {
				return errFundingConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		offer, exists := catalog.offers[offerCode]
		if !exists {
			return errFundingInvalid
		}
		record = managedFundingOrderRecord{ID: identifier, BillingAccountID: accountID, CreationKeyDigest: key, OfferCode: offer.Code, PriceID: offer.PriceID, FundingCents: offer.FundingCents, Currency: CatalogCurrencyUSD, Environment: catalog.environment, ProcessorAccountID: catalog.processorAccountID, SupplierID: catalog.supplierID, State: fundingOrderCreated, CreatedAt: now}
		if err := tx.Omit(clause.Associations).Create(&record).Error; err != nil {
			return err
		}
		return tx.Omit(clause.Associations).Create(&managedPaymentDeliveryRecord{OrderID: identifier, State: paymentDeliveryPending, CreatedAt: now}).Error
	})
	if err != nil {
		return managedFundingOrderRecord{}, fmt.Errorf("create funding order %s: %w", identifier, err)
	}
	return record, nil
}

func (database *gormManagedTenantDatabase) fundingOrder(ctx context.Context, accountID, orderID string) (managedFundingOrderRecord, error) {
	var record managedFundingOrderRecord
	err := database.database.WithContext(ctx).Where("id = ? AND billing_account_id = ?", orderID, accountID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, errFundingNotFound
	}
	if err != nil {
		return record, fmt.Errorf("read funding order %s: %w", orderID, err)
	}
	return record, nil
}

func (database *gormManagedTenantDatabase) fundingOrders(ctx context.Context, accountID string, page managedConnectionPage) ([]managedFundingOrderRecord, error) {
	var records []managedFundingOrderRecord
	query := database.database.WithContext(ctx).Where("billing_account_id = ?", accountID)
	if page.after != "" {
		query = query.Where("id > ?", page.after)
	}
	if err := query.Order("id ASC").Limit(page.limit + 1).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list funding orders for %s: %w", accountID, err)
	}
	return records, nil
}

func writeFundingError(ctx *gin.Context, err error) {
	status, code := http.StatusServiceUnavailable, errFundingUnavailable.Error()
	switch {
	case errors.Is(err, errFundingInvalid):
		status, code = http.StatusBadRequest, errFundingInvalid.Error()
	case errors.Is(err, errFundingConflict):
		status, code = http.StatusConflict, errFundingConflict.Error()
	case errors.Is(err, errFundingNotFound):
		status, code = http.StatusNotFound, errFundingNotFound.Error()
	}
	ctx.JSON(status, gin.H{"error": gin.H{"code": code}})
}

func (service *managementService) fundingOffersHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if _, ok := service.ownedBillingAccount(ctx); !ok {
			return
		}
		if ctx.Request.URL.RawQuery != "" {
			writeFundingError(ctx, errFundingInvalid)
			return
		}
		if service.funding == nil {
			writeFundingError(ctx, errFundingUnavailable)
			return
		}
		offers := make([]managementFundingOfferResponse, 0, len(service.funding.offers))
		for _, offer := range service.funding.offers {
			offers = append(offers, managementFundingOfferResponse{Code: offer.Code, FundingCents: strconv.FormatInt(offer.FundingCents, 10), Currency: CatalogCurrencyUSD})
		}
		sort.Slice(offers, func(first, second int) bool { return offers[first].Code < offers[second].Code })
		ctx.JSON(http.StatusOK, gin.H{"offers": offers})
	}
}

func (service *managementService) createFundingOrderHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		var input struct {
			OfferCode string `json:"offer_code"`
		}
		if err := decodeManagementJSON(ctx, &input); err != nil || ctx.Request.URL.RawQuery != "" || !journalDimensionPattern.MatchString(input.OfferCode) || len(input.OfferCode) > 64 {
			writeFundingError(ctx, errFundingInvalid)
			return
		}
		key, err := hostedCreationKey(ctx)
		if err != nil {
			writeFundingError(ctx, errFundingInvalid)
			return
		}
		if service.funding == nil {
			writeFundingError(ctx, errFundingUnavailable)
			return
		}
		record, err := service.store.database.createFundingOrder(ctx.Request.Context(), service.funding, account.ID, input.OfferCode, key, service.store.now().UTC())
		if err != nil {
			writeFundingError(ctx, err)
			return
		}
		ctx.Header("Location", managementAPIPath+managementBillingAccountsPath+"/"+account.ID+"/funding-orders/"+record.ID)
		ctx.JSON(http.StatusCreated, fundingOrderResponse(record))
	}
}

func (service *managementService) getFundingOrderHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		if ctx.Request.URL.RawQuery != "" {
			writeFundingError(ctx, errFundingInvalid)
			return
		}
		record, err := service.store.database.fundingOrder(ctx.Request.Context(), account.ID, ctx.Param("order_id"))
		if err != nil {
			writeFundingError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, fundingOrderResponse(record))
	}
}

func (service *managementService) listFundingOrdersHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		account, ok := service.ownedBillingAccount(ctx)
		if !ok {
			return
		}
		page, err := newHostedResourcePage(ctx.Request.URL.RawQuery, fundingOrderIDPrefix)
		if err != nil {
			writeFundingError(ctx, errFundingInvalid)
			return
		}
		records, err := service.store.database.fundingOrders(ctx.Request.Context(), account.ID, page)
		if err != nil {
			writeFundingError(ctx, err)
			return
		}
		cursor := ""
		if len(records) > page.limit {
			records = records[:page.limit]
			cursor = records[len(records)-1].ID
		}
		orders := make([]managementFundingOrderResponse, 0, len(records))
		for _, record := range records {
			orders = append(orders, fundingOrderResponse(record))
		}
		ctx.JSON(http.StatusOK, gin.H{"orders": orders, "next_cursor": cursor})
	}
}
