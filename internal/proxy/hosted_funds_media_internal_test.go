package proxy

import (
	"fmt"
	"net/http"
	"testing"

	"gorm.io/gorm"
)

func TestHostedFundsMediaAdmissionUsesFinancialErrors(t *testing.T) {
	for _, scenario := range []struct {
		name, code string
		status     int
	}{
		{"empty", "insufficient_funds", http.StatusPaymentRequired},
		{"database-failure", "financial_admission_unavailable", http.StatusServiceUnavailable},
		{"suspended", "hosted_authority_denied", http.StatusForbidden},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, _, _ := newHostedRatingFixture(t)
			server, service := newHostedMediaAdmissionHTTPServer(t, database)
			service.store.now = ratingTestAcceptanceTime
			rates := []CatalogPriceRate{}
			for _, component := range []string{"input_text", "input_image", "output_text", "output_image"} {
				rates = append(rates, CatalogPriceRate{Component: component, Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}})
			}
			prices := hostedMediaPriceCatalog(t, "openai", "gpt-image-2", ModelOperationImageGeneration, rates, func(offering *ProviderOffering) {
				for _, dimension := range []string{"input_text_tokens", "input_image_tokens", "output_text_tokens", "output_image_tokens"} {
					maximum := 100
					offering.Limits = append(offering.Limits, CatalogLimit{ID: dimension, Unit: "tokens", Value: &maximum})
				}
			})
			service.hostedAdmission = fixedHostedMediaAdmission(newHostedFundsAdmission(func(transaction *gorm.DB, request managedJournalRequestRecord) error {
				admission, err := newHostedMediaPriceAdmission(prices, request, CatalogProtocolOpenAIImages, CatalogPriceConditions{}, 1)
				if err != nil {
					return err
				}
				return admission(transaction, request)
			}))
			if scenario.name != "empty" {
				seedHostedFunds(t, database, 5)
			}
			if scenario.name == "database-failure" {
				if err := database.database.Exec("CREATE TRIGGER reject_media_funds BEFORE INSERT ON managed_funds_reservation_records BEGIN SELECT RAISE(ABORT, 'controlled_funds_failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario.name == "suspended" {
				if err := database.database.Create(&managedFundsAccountRecord{BillingAccountID: "billing-journal", State: "suspended", RemainderNumerator: "0", RemainderDenominator: "1", CreatedAt: ratingTestAcceptanceTime()}).Error; err != nil {
					t.Fatal(err)
				}
			}
			response := hostedMediaAdmissionHTTP(t, server, "media-funds", "funded image prompt", scenario.status)
			if response["error"].(map[string]any)["code"] != scenario.code {
				t.Fatalf("financial error=%v", response)
			}
			for _, model := range []any{&managedJournalRequestRecord{}, &managedFundsReservationRecord{}, &mediaOperationRecord{}} {
				var count int64
				if err := database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("rejected %T count=%d error=%v", model, count, err)
				}
			}
			if len(service.queue) != 0 {
				t.Fatal("unfunded media entered the worker queue")
			}
			if scenario.name == "empty" {
				seedHostedFunds(t, database, 5)
			}
			if scenario.name == "database-failure" {
				if err := database.database.Exec("DROP TRIGGER reject_media_funds").Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario.name == "suspended" {
				if err := database.database.Model(&managedFundsAccountRecord{}).Where("billing_account_id = ?", "billing-journal").Update("state", fundsAccountActive).Error; err != nil {
					t.Fatal(err)
				}
			}
			first := hostedMediaAdmissionHTTP(t, server, "media-funds", "funded image prompt", http.StatusAccepted)
			replay := hostedMediaAdmissionHTTP(t, server, "media-funds", "funded image prompt", http.StatusOK)
			if fmt.Sprint(first["operation_id"]) != fmt.Sprint(replay["operation_id"]) {
				t.Fatalf("media replay changed identity: %v %v", first, replay)
			}
			assertHostedFundsBalance(t, database, 5, 4)
		})
	}
}
