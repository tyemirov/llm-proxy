package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func verifyRetainedPriceRecovery(t *testing.T, mutate func(*hostedPriceSnapshotDocument), encoding string, publicStatus int) {
	t.Helper()
	fixture := newFundsStartupFixture(t, startupCompleteUsage)
	before := fixture.state(t)
	var retained managedPriceSnapshotRecord
	if err := fixture.database.database.First(&retained).Error; err != nil {
		t.Fatal(err)
	}
	path := "/billing-accounts/billing-journal/price-snapshots/" + retained.ID
	original := ratingHTTPExchange(t, fixture.management, http.MethodGet, path, "", http.StatusOK)
	var document hostedPriceSnapshotDocument
	if err := json.Unmarshal(retained.Document, &document); err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(&document)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	switch encoding {
	case "invalid-json":
		encoded = []byte("{")
	case "unknown-field":
		encoded = append(encoded[:len(encoded)-1], []byte(`,"unexpected":true}`)...)
	case "trailing-value":
		encoded = append(encoded, []byte(" {}")...)
	case "trailing-garbage":
		encoded = append(encoded, []byte(" corrupt")...)
	}
	digest := sha256Hex(string(encoded))
	if encoding == "digest-mismatch" {
		digest = strings.Repeat("0", 64)
	}
	write := func(document []byte, digest string) {
		t.Helper()
		if err := fixture.database.database.Model(&managedPriceSnapshotRecord{}).Where("id = ?", retained.ID).Updates(map[string]any{"document": document, "digest": digest}).Error; err != nil {
			t.Fatal(err)
		}
	}
	write(encoded, digest)
	response := ratingHTTPExchange(t, fixture.management, http.MethodGet, path, "", publicStatus)
	if publicStatus == http.StatusInternalServerError && strings.Contains(fmt.Sprint(response), "retained") {
		t.Fatal("price failure exposed private validation details")
	}
	fixture.failStartup(t)
	fixture.assertPending(t, before)
	write(retained.Document, retained.Digest)
	if restored := ratingHTTPExchange(t, fixture.management, http.MethodGet, path, "", http.StatusOK); !reflect.DeepEqual(original, restored) {
		t.Fatal("recovery changed the accepted price resource")
	}
	fixture.assertSettledOnce(t, before)
}

func TestHostedRatingCorruptSnapshotPreventsSettlement(t *testing.T) {
	for _, encoding := range []string{"digest-mismatch", "invalid-json", "unknown-field", "trailing-value", "trailing-garbage"} {
		t.Run(encoding, func(t *testing.T) {
			verifyRetainedPriceRecovery(t, nil, encoding, http.StatusInternalServerError)
		})
	}
	for _, scenario := range []struct {
		name   string
		mutate func(*hostedPriceSnapshotDocument)
	}{
		{"source", func(document *hostedPriceSnapshotDocument) { document.Source = "http://invalid.example/prices" }},
		{"verified-date", func(document *hostedPriceSnapshotDocument) { document.LastVerified = "invalid" }},
		{"minimum-unit", func(document *hostedPriceSnapshotDocument) {
			document.MinimumCharge = &CatalogMinimumCharge{Currency: "USD", Amount: "1", Unit: "USD/token"}
		}},
		{"revision", func(document *hostedPriceSnapshotDocument) { document.CatalogRevision = "" }},
		{"markup-denominator", func(document *hostedPriceSnapshotDocument) { document.Markup.Denominator = "0" }},
		{"zero-markup", func(document *hostedPriceSnapshotDocument) { document.Markup.Numerator = "0" }},
		{"empty-component", func(document *hostedPriceSnapshotDocument) { document.Components[0].Rates = nil }},
		{"missing-dimension", func(document *hostedPriceSnapshotDocument) { document.Components[0].Dimension = "" }},
		{"duplicate-dimension", func(document *hostedPriceSnapshotDocument) {
			document.Components[1].Dimension = document.Components[0].Dimension
		}},
		{"additional-dimension", func(document *hostedPriceSnapshotDocument) {
			document.Components[0].AdditionalDimension = document.Components[1].Dimension
		}},
		{"missing-effective-date", func(document *hostedPriceSnapshotDocument) {
			document.Components[0].Rates[0].ProviderRate.Conditions.EffectiveFrom = ""
		}},
		{"inactive-price", func(document *hostedPriceSnapshotDocument) {
			document.Components[0].Rates[0].ProviderRate.Conditions.EffectiveFrom = "2020-01-01T00:00:00Z"
			document.Components[0].Rates[0].ProviderRate.Conditions.EffectiveUntil = "2020-02-01T00:00:00Z"
		}},
		{"unresolved-interval", func(document *hostedPriceSnapshotDocument) {
			document.Components[0].Rates[0].ProviderRate.Conditions.InputTokens.UnresolvedReason = "unavailable"
		}},
		{"unknown-unit", func(document *hostedPriceSnapshotDocument) {
			document.Components[0].Rates[0].ProviderRate.Unit = "USD/unknown"
		}},
		{"customer-rate", func(document *hostedPriceSnapshotDocument) {
			document.Components[0].Rates[0].CustomerRate.Numerator = "99"
		}},
		{"excluded-component", func(document *hostedPriceSnapshotDocument) { document.ExcludedComponents = []string{"input_tokens"} }},
		{"zero-dimension", func(document *hostedPriceSnapshotDocument) { document.ZeroDimensions = []string{"input_tokens"} }},
		{"missing-bounds", func(document *hostedPriceSnapshotDocument) { document.Bounds = nil }},
		{"invalid-bound", func(document *hostedPriceSnapshotDocument) { document.Bounds[0].Maximum = "invalid" }},
		{"bound-unit", func(document *hostedPriceSnapshotDocument) { document.Bounds[0].Unit = "seconds" }},
		{"zero-attempts", func(document *hostedPriceSnapshotDocument) { document.Maximum.Attempts = 0 }},
		{"maximum-reserve", func(document *hostedPriceSnapshotDocument) { document.Maximum.ReservedCents++ }},
		{"maximum-charge", func(document *hostedPriceSnapshotDocument) { document.Maximum.CustomerCharge.Numerator = "99" }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			verifyRetainedPriceRecovery(t, scenario.mutate, "", http.StatusInternalServerError)
		})
	}
}

func TestHostedRatingForeignSnapshotCannotSettleAcceptedRequest(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*hostedPriceSnapshotDocument)
	}{
		{"provider", func(document *hostedPriceSnapshotDocument) { document.Provider = "anthropic" }},
		{"model", func(document *hostedPriceSnapshotDocument) { document.Model = "other-model" }},
		{"operation", func(document *hostedPriceSnapshotDocument) { document.Operation = ModelOperationImageGeneration }},
		{"revision", func(document *hostedPriceSnapshotDocument) { document.CatalogRevision = "another-catalog" }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			verifyRetainedPriceRecovery(t, scenario.mutate, "", http.StatusOK)
		})
	}
}
