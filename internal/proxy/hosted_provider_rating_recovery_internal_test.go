package proxy

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestHostedPaymentsProviderReconciliationRejectsCorruptRetainedRatings(t *testing.T) {
	for _, scenario := range []string{"digest", "json", "provider-cost", "customer-charge", "rating-state"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newProviderAuditFixture(t)
			before := fixture.charges(t)
			var original managedChargeRecord
			if err := fixture.database.database.First(&original).Error; err != nil {
				t.Fatal(err)
			}
			var rating CatalogRatedUsage
			if err := json.Unmarshal(original.Rating, &rating); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "provider-cost":
				rating.ProviderCost.Denominator = "0"
			case "customer-charge":
				rating.CustomerCharge.Numerator = "invalid"
			case "rating-state":
				rating.State = CatalogRatingUnresolved
			}
			encoded, err := json.Marshal(rating)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "json" {
				encoded = []byte(`{`)
			}
			digest := sha256Hex(string(encoded))
			if scenario == "digest" {
				digest = sha256Hex("different retained rating")
			}
			if err := fixture.database.database.Model(&managedChargeRecord{}).Where("id = ?", original.ID).Updates(map[string]any{"rating": encoded, "rating_digest": digest}).Error; err != nil {
				t.Fatal(err)
			}
			var corrupted managedChargeRecord
			if err := fixture.database.database.First(&corrupted, "id = ?", original.ID).Error; err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if _, err := fixture.reconcile(t, fixture.configuration); err == nil {
					t.Fatal("corrupt retained rating produced a provider comparison")
				}
				fixture.assertNoImport(t)
				var retained managedChargeRecord
				if err := fixture.database.database.First(&retained, "id = ?", original.ID).Error; err != nil || !reflect.DeepEqual(retained, corrupted) {
					t.Fatalf("failed comparison changed the retained charge: %v", err)
				}
			}
			ratingHTTPExchange(t, fixture.server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusInternalServerError)
			if err := fixture.database.database.Model(&managedChargeRecord{}).Where("id = ?", original.ID).Updates(map[string]any{"rating": original.Rating, "rating_digest": original.RatingDigest}).Error; err != nil {
				t.Fatal(err)
			}
			fixture.assertRecovery(t, before)
		})
	}
}
