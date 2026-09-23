package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/tyemirov/utils/billing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PaymentConfiguration selects one processor environment. Nil disables payments.
type PaymentConfiguration struct {
	Environment        string                      `mapstructure:"environment"`
	ClientToken        string                      `mapstructure:"client_token"`
	ProcessorAccountID string                      `mapstructure:"processor_account_id"`
	SupplierID         string                      `mapstructure:"supplier_id"`
	APIKey             string                      `mapstructure:"api_key"`
	APIBaseURL         string                      `mapstructure:"api_base_url"`
	WebhookSecret      string                      `mapstructure:"webhook_secret"`
	Offers             []PaymentOfferConfiguration `mapstructure:"offers"`
}

// PaymentOfferConfiguration binds a server offer to an immutable funding amount.
type PaymentOfferConfiguration struct {
	Code         string `mapstructure:"code"`
	PriceID      string `mapstructure:"price_id"`
	FundingCents int64  `mapstructure:"funding_cents"`
}

type paymentSettings struct {
	catalog       *fundingCatalog
	apiKey        string
	apiBaseURL    string
	webhookSecret string
	clientToken   string
}

var paymentClientTokenPattern = regexp.MustCompile(`^(test|live)_[a-zA-Z0-9]+$`)

func newPaymentSettings(input *PaymentConfiguration) (*paymentSettings, error) {
	if input == nil {
		return nil, nil
	}
	prefix := "live_"
	if input.Environment == paymentEnvironmentSandbox {
		prefix = "test_"
	}
	if !paymentClientTokenPattern.MatchString(input.ClientToken) || !strings.HasPrefix(input.ClientToken, prefix) {
		return nil, fmt.Errorf("configure payments: client_token must match the payment environment")
	}
	if strings.TrimSpace(input.APIKey) == "" || strings.TrimSpace(input.WebhookSecret) == "" {
		return nil, fmt.Errorf("configure payments: api_key and webhook_secret are required")
	}
	if input.APIBaseURL != "" {
		parsed, err := url.Parse(input.APIBaseURL)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("configure payments: invalid api_base_url")
		}
		loopback := parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost" || parsed.Hostname() == "::1"
		if parsed.Scheme != "https" && !(input.Environment == paymentEnvironmentSandbox && parsed.Scheme == "http" && loopback) {
			return nil, fmt.Errorf("configure payments: api_base_url requires HTTPS except for a sandbox loopback protocol")
		}
	}
	offers := make([]fundingOfferInput, len(input.Offers))
	for index, offer := range input.Offers {
		offers[index] = fundingOfferInput(offer)
	}
	catalog, err := newFundingCatalog(input.Environment, input.ProcessorAccountID, input.SupplierID, offers)
	if err != nil {
		return nil, err
	}
	return &paymentSettings{catalog: catalog, apiKey: input.APIKey, apiBaseURL: input.APIBaseURL, webhookSecret: input.WebhookSecret, clientToken: input.ClientToken}, nil
}

type paddlePaymentRuntime struct {
	checkout  *paddleCheckoutDelivery
	processor *paddlePaymentProcessor
	inbox     *paddlePaymentInbox
	client    billing.PaddleCommerceClient
	catalog   *fundingCatalog
}

func newPaddlePaymentRuntime(settings *paymentSettings, database *gormManagedTenantDatabase) (*paddlePaymentRuntime, error) {
	if settings == nil {
		return nil, nil
	}
	client, err := billing.NewPaddleCommerceClient(settings.catalog.environment, settings.apiKey, settings.apiBaseURL, &http.Client{Timeout: paymentCheckoutRetry})
	if err != nil {
		return nil, fmt.Errorf("configure shared Paddle client: %w", err)
	}
	checkout, err := newPaddleCheckoutDelivery(database, settings.catalog, client)
	if err != nil {
		return nil, err
	}
	processor, err := newPaddlePaymentProcessor(database, settings.catalog, client)
	if err != nil {
		return nil, err
	}
	inbox, err := newPaddlePaymentInbox(database, settings.catalog.environment, settings.catalog.processorAccountID, settings.webhookSecret, 5*time.Minute)
	if err != nil {
		return nil, err
	}
	if err := bindPaymentEnvironment(database.database, settings.catalog.environment); err != nil {
		return nil, err
	}
	return &paddlePaymentRuntime{checkout, processor, inbox, client, settings.catalog}, nil
}

type managedPaymentEnvironmentRecord struct {
	ID          int    `gorm:"primaryKey;check:id = 1"`
	Environment string `gorm:"not null;check:environment IN ('sandbox','production')"`
}

func bindPaymentEnvironment(database *gorm.DB, environment string) error {
	return database.Transaction(func(tx *gorm.DB) error {
		binding := managedPaymentEnvironmentRecord{ID: 1, Environment: environment}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&binding).Error; err != nil {
			return fmt.Errorf("bind payment environment: %w", err)
		}
		if err := tx.Where("id = ?", 1).First(&binding).Error; err != nil {
			return fmt.Errorf("read payment environment: %w", err)
		}
		if binding.Environment != environment {
			return fmt.Errorf("payment environment differs from the database binding")
		}
		for _, model := range []any{&managedFundingOrderRecord{}, &managedPaymentInboxRecord{}} {
			var count int64
			if err := tx.Model(model).Where("environment <> ?", environment).Count(&count).Error; err != nil {
				return fmt.Errorf("check retained payment environment: %w", err)
			}
			if count != 0 {
				return fmt.Errorf("payment environment differs from retained financial records")
			}
		}
		return nil
	})
}

func (runtime *paddlePaymentRuntime) reconcile(ctx context.Context) error {
	if err := runtime.checkout.reconcile(ctx); err != nil {
		return fmt.Errorf("deliver payment checkout: %w", err)
	}
	if err := runtime.processor.reconcile(ctx); err != nil {
		return fmt.Errorf("process payment evidence: %w", err)
	}
	return nil
}
