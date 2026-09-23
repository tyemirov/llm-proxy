package proxy

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/tyemirov/llm-proxy/internal/openapitest"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func newHostedRatingFixture(t *testing.T) (*gormManagedTenantDatabase, func(string) journalAdmissionIntent, *httptest.Server, journalReservation) {
	t.Helper()
	return newHostedRatingFixtureForAttempts(t, 2)
}

func newHostedRatingFixtureForAttempts(t *testing.T, attempts uint32) (*gormManagedTenantDatabase, func(string) journalAdmissionIntent, *httptest.Server, journalReservation) {
	t.Helper()
	database, intent, _, server := newJournalTransactionHTTPFixture(t)
	service := newInternalManagementService(t, newFakeManagedTenantDatabase(), internalManagementProviderRegistry())
	service.store.database = database
	router := server.Config.Handler.(*gin.Engine)
	router.GET(managementAPIPath+managementChargesPath, service.listChargesHandler())
	router.GET(managementAPIPath+managementChargePath, service.getChargeHandler())
	router.GET(managementAPIPath+managementPriceSnapshotPath, service.getPriceSnapshotHandler())
	router.GET(managementAPIPath+managementRequestChargeSummaryPath, service.getRequestChargeSummaryHandler())
	catalog := internalTestModelCatalog(internalTestOffering("openai", "gpt-4.1", []string{"text"}, []string{"text"}))
	catalog.Revision = "journal-catalog"
	conditions := CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z", EffectiveUntil: "2026-10-01T00:00:00Z"}
	catalog.Prices[0] = CatalogPriceDescriptor{Provider: "openai", Model: "gpt-4.1", Operation: "text", Available: true, Source: "https://example.com/prices", LastVerified: "2026-09-22", Rates: []CatalogPriceRate{
		{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: conditions},
		{Component: "output_tokens", Currency: "USD", Rate: "8", Unit: "USD/1M_tokens", Conditions: conditions},
	}}
	prices, err := NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := prices.NewRatingSnapshot("openai", "gpt-4.1", "text", ratingTestAcceptanceTime(), []CatalogRateBinding{{Dimension: "input_tokens", Component: "input_tokens", Conditions: categoricalPriceConditions(conditions)}, {Dimension: "output_tokens", Component: "output_tokens", Conditions: categoricalPriceConditions(conditions)}})
	if err != nil {
		t.Fatal(err)
	}
	admission, err := newHostedPriceAdmission(snapshot, []CatalogUsageBound{{Dimension: "input_tokens", Unit: "token", Maximum: "1000"}, {Dimension: "output_tokens", Unit: "token", Maximum: "1000"}}, attempts)
	if err != nil {
		t.Fatal(err)
	}
	return database, intent, server, admission
}

func observeRatedFixture(t *testing.T, database *gormManagedTenantDatabase, intent journalAdmissionIntent, reserve journalReservation, outcome journalObservationOutcome, quantities []journalQuantity) (managedJournalRequestRecord, managedJournalObservationRecord) {
	t.Helper()
	request, err := database.admitJournalRequest(t.Context(), intent, reserve)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := newJournalWorkerClaim(request.ID, request.OwnerToken, request.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-"+sha256Hex(request.ID)[:32], reserve)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.dispatchJournalAttempt(t.Context(), claim, attempt.ID); err != nil {
		t.Fatal(err)
	}
	input := journalUsageEvidenceInput{AttemptID: attempt.ID, AdapterRevision: "test-native-meter", ProviderRequestID: "private-provider-request", Quantities: quantities, Outcome: outcome, ObservedAt: claim.now}
	if outcome == journalOutcomeFail {
		input.FailureCode = "provider_failed"
	}
	evidence, err := newJournalUsageEvidence(input, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := database.observeJournalAttempt(t.Context(), claim, evidence)
	if err != nil {
		t.Fatal(err)
	}
	return request, observation
}

func TestHostedRatingPersistsAcceptedPricesAndIdempotentCharges(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	request, observation := observeRatedFixture(t, database, intent("priced-request"), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}})
	// The rating worker starts from retained prices, without the admission catalog.
	original, err := database.database.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := openJournalTransactionInstance(t, database)
	database.database = restarted.database
	delivery := newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })
	for range 2 {
		if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, delivery); err != nil {
			t.Fatal(err)
		}
	}
	collection := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges?limit=1", "", http.StatusOK)
	charges := collection["charges"].([]any)
	if len(charges) != 1 || collection["next_cursor"] != "" {
		t.Fatalf("duplicated charge: %v", collection)
	}
	charge := charges[0].(map[string]any)
	if charge["request_id"] != request.ID || charge["observation_id"] != observation.ID || charge["state"] != "rated" {
		t.Fatalf("charge attribution: %v", charge)
	}
	rating := charge["rating"].(map[string]any)
	if !reflect.DeepEqual(rating["provider_cost"], map[string]any{"numerator": "7", "denominator": "2500"}) || !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": "91", "denominator": "25000"}) {
		t.Fatalf("incorrect charge: %v", charge)
	}
	path := "/billing-accounts/billing-journal/charges/" + charge["id"].(string)
	detail := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK)
	if !reflect.DeepEqual(detail, charge) {
		t.Fatalf("charge detail differs: %v", detail)
	}
	retained := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/price-snapshots/"+charge["price_snapshot_id"].(string), "", http.StatusOK)
	snapshot := retained["snapshot"].(map[string]any)
	if snapshot["catalog_revision"] != "journal-catalog" || retained["request_id"] != request.ID {
		t.Fatalf("snapshot attribution: %v", retained)
	}
	if strings.Contains(string(mustRatingJSON(t, charge)), "private-") {
		t.Fatal("charge leaked provider identity")
	}
	ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-foreign/charges/"+charge["id"].(string), "", http.StatusNotFound)
	ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges?limit=0", "", http.StatusBadRequest)
	ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges/charge-missing", "", http.StatusNotFound)
}

func TestHostedRatingRetainsSeparateReasoningInOutputCharge(t *testing.T) {
	database, intent, server, _ := newHostedRatingFixture(t)
	catalog := internalTestModelCatalog(internalTestOffering("openai", "gpt-4.1", []string{"text"}, []string{"text"}))
	catalog.Revision = "journal-catalog"
	conditions := CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}
	catalog.Prices[0] = CatalogPriceDescriptor{Provider: "openai", Model: "gpt-4.1", Operation: "text", Available: true, Source: "https://example.com/prices", LastVerified: "2026-09-22", Rates: []CatalogPriceRate{
		{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: conditions},
		{Component: "output_tokens", Currency: "USD", Rate: "8", Unit: "USD/1M_tokens", Conditions: conditions},
	}}
	prices, err := NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := prices.NewRatingSnapshot("openai", "gpt-4.1", "text", ratingTestAcceptanceTime(), []CatalogRateBinding{{Dimension: "input_tokens", Component: "input_tokens"}, {Dimension: "output_tokens", AdditionalDimension: "reasoning_tokens", Component: "output_tokens"}})
	if err != nil {
		t.Fatal(err)
	}
	reserve, err := newHostedPriceAdmission(snapshot, []CatalogUsageBound{{Dimension: "input_tokens", Unit: "token", Maximum: "1000"}, {Dimension: "output_tokens", Unit: "token", Maximum: "1000"}, {Dimension: "reasoning_tokens", Unit: "token", Maximum: "1000"}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	_, observation := observeRatedFixture(t, database, intent("separate-reasoning"), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1000"}, {Dimension: "output_tokens", Unit: "token", Value: "100"}, {Dimension: "reasoning_tokens", Unit: "token", Value: "40"}})
	original, err := database.database.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	database.database = openJournalTransactionInstance(t, database).database
	if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.ObservedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })); err != nil {
		t.Fatal(err)
	}
	collection := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
	charge := collection["charges"].([]any)[0].(map[string]any)
	if charge["state"] != chargeRated || !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": "507", "denominator": "125000"}) {
		t.Fatalf("separate reasoning was not charged: %v", charge)
	}
	lines := charge["rating"].(map[string]any)["lines"].([]any)
	if lines[1].(map[string]any)["quantity"] != "140" {
		t.Fatalf("incorrect output quantity: %v", lines)
	}
	retained := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/price-snapshots/"+charge["price_snapshot_id"].(string), "", http.StatusOK)
	component := retained["snapshot"].(map[string]any)["components"].([]any)[1].(map[string]any)
	if component["additional_dimension"] != "reasoning_tokens" {
		t.Fatalf("compound quantity contract was not retained: %v", component)
	}
	var retainedObservation managedJournalObservationRecord
	if err := database.database.Where("id = ?", observation.ID).First(&retainedObservation).Error; err != nil {
		t.Fatal(err)
	}
	if string(retainedObservation.Quantities) != string(observation.Quantities) {
		t.Fatal("compound rating changed the native journal measurements")
	}
}

func TestHostedRatingRetainsTierSchedulesAcrossRestart(t *testing.T) {
	for _, test := range []struct {
		input, inputRate, outputRate string
		charge                       ExactMoney
	}{
		{"199999", "2", "8", ExactMoney{Numerator: "2605187", Denominator: "5000000"}},
		{"200000", "4", "6", ExactMoney{Numerator: "52039", Denominator: "50000"}},
	} {
		t.Run(test.input, func(t *testing.T) {
			database, intent, server, _ := newHostedRatingFixture(t)
			catalog := internalTestModelCatalog(internalTestOffering("openai", "gpt-4.1", []string{"text"}, []string{"text"}))
			catalog.Revision = "journal-catalog"
			low := CatalogPriceConditions{InputTokens: CatalogTokenRange{MaximumExclusive: 200000}, EffectiveFrom: "2026-09-01T00:00:00Z", EffectiveUntil: "2026-10-01T00:00:00Z"}
			high := low
			high.InputTokens = CatalogTokenRange{Minimum: 200000}
			catalog.Prices[0] = CatalogPriceDescriptor{Provider: "openai", Model: "gpt-4.1", Operation: "text", Available: true, Source: "https://example.com/prices", LastVerified: "2026-09-22", Rates: []CatalogPriceRate{
				{Component: "input_tokens", Currency: "USD", Rate: "2", Unit: "USD/1M_tokens", Conditions: low},
				{Component: "input_tokens", Currency: "USD", Rate: "4", Unit: "USD/1M_tokens", Conditions: high},
				{Component: "output_tokens", Currency: "USD", Rate: "8", Unit: "USD/1M_tokens", Conditions: low},
				{Component: "output_tokens", Currency: "USD", Rate: "6", Unit: "USD/1M_tokens", Conditions: high},
			}}
			prices, err := NewCatalogService(catalog)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := prices.NewRatingSnapshot("openai", "gpt-4.1", "text", ratingTestAcceptanceTime(), []CatalogRateBinding{{Dimension: "input_tokens", Component: "input_tokens"}, {Dimension: "output_tokens", Component: "output_tokens"}})
			if err != nil {
				t.Fatal(err)
			}
			reserve, err := newHostedPriceAdmission(snapshot, []CatalogUsageBound{{Dimension: "input_tokens", Unit: "token", Maximum: "300000"}, {Dimension: "output_tokens", Unit: "token", Maximum: "1000"}}, 2)
			if err != nil {
				t.Fatal(err)
			}
			_, observation := observeRatedFixture(t, database, intent("tiered-request"), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: test.input}, {Dimension: "output_tokens", Unit: "token", Value: "100"}})
			original, err := database.database.DB()
			if err != nil {
				t.Fatal(err)
			}
			if err := original.Close(); err != nil {
				t.Fatal(err)
			}
			database.database = openJournalTransactionInstance(t, database).database
			// Delivery after price expiry must use the retained acceptance schedule.
			if err := database.deliverJournalObservation(t.Context(), observation.ID, ratingTestAcceptanceTime().AddDate(0, 1, 0), newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })); err != nil {
				t.Fatal(err)
			}
			collection := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
			charge := collection["charges"].([]any)[0].(map[string]any)
			if charge["state"] != chargeRated || !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": test.charge.Numerator, "denominator": test.charge.Denominator}) {
				t.Fatalf("incorrect tier charge: %v", charge)
			}
			lines := charge["rating"].(map[string]any)["lines"].([]any)
			if len(lines) != 2 || lines[0].(map[string]any)["provider_rate"] != test.inputRate || lines[1].(map[string]any)["provider_rate"] != test.outputRate {
				t.Fatalf("incorrect selected tiers: %v", lines)
			}
			retained := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/price-snapshots/"+charge["price_snapshot_id"].(string), "", http.StatusOK)
			document := retained["snapshot"].(map[string]any)
			components := document["components"].([]any)
			if len(components) != 2 || len(components[0].(map[string]any)["rates"].([]any)) != 2 || len(components[1].(map[string]any)["rates"].([]any)) != 2 {
				t.Fatalf("incomplete retained schedule: %v", document)
			}
			maximum := document["maximum"].(map[string]any)
			if maximum["reserved_cents"] != "315" {
				t.Fatalf("tier maximum lost after restart: %v", maximum)
			}
		})
	}
}

func mustRatingJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestHostedRatingDestinationFailureRollsBackChargeAndDelivery(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	_, observation := observeRatedFixture(t, database, intent("rolled-back-charge"), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "0"}, {Dimension: "output_tokens", Unit: "token", Value: "1"}})
	failure := errors.New("controlled settlement failure")
	err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return failure }))
	if !errors.Is(err, failure) {
		t.Fatalf("missing settlement failure: %v", err)
	}
	collection := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
	if len(collection["charges"].([]any)) != 0 {
		t.Fatalf("partial charge committed: %v", collection)
	}
	pending, err := database.pendingJournalDeliveries(t.Context(), 10)
	if err != nil || len(pending) != 1 || pending[0].ID != observation.ID {
		t.Fatalf("lost pending evidence: %+v %v", pending, err)
	}
}

func TestHostedRatingUnresolvedChargesKeepProviderCostsAndPreventSettlement(t *testing.T) {
	for _, test := range []struct {
		name    string
		outcome journalObservationOutcome
		input   journalQuantity
		state   string
	}{
		{"unknown", journalOutcomeComplete, journalQuantity{Dimension: "input_tokens", Unit: "token", UnknownReason: journalQuantityNotReported}, chargeUsageUnresolved},
		{"failure policy", journalOutcomeFail, journalQuantity{Dimension: "input_tokens", Unit: "token", Value: "10"}, chargePolicyUnresolved},
		{"maximum", journalOutcomeComplete, journalQuantity{Dimension: "input_tokens", Unit: "token", Value: "1001"}, chargeLimitUnresolved},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, intent, server, reserve := newHostedRatingFixture(t)
			_, observation := observeRatedFixture(t, database, intent("unresolved"), reserve, test.outcome, []journalQuantity{test.input, {Dimension: "output_tokens", Unit: "token", Value: "1"}})
			if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return errors.New("unresolved charge reached settlement") })); err != nil {
				t.Fatal(err)
			}
			collection := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
			charge := collection["charges"].([]any)[0].(map[string]any)
			if charge["state"] != test.state || charge["customer_charge"] != nil {
				t.Fatalf("unresolved evidence settled: %v", charge)
			}
			rating := charge["rating"].(map[string]any)
			if test.state != chargeUsageUnresolved && rating["provider_cost"] == nil {
				t.Fatalf("provider cost was discarded: %v", charge)
			}
			if test.state == chargeUsageUnresolved && rating["provider_cost"] != nil {
				t.Fatalf("unknown usage became a cost: %v", charge)
			}
		})
	}
}

func ratingHTTPExchange(t *testing.T, server *httptest.Server, method, path, body string, status int) map[string]any {
	t.Helper()
	request, err := http.NewRequest(method, server.URL+managementAPIPath+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("%s status=%d expected=%d body=%s", path, response.StatusCode, status, payload)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("financial response permits caching")
	}
	template := "/api/management/billing-accounts/{billing_account_id}/charges"
	if strings.HasSuffix(path, "/charge-summary") {
		template = "/api/management/billing-accounts/{billing_account_id}/requests/{request_id}/charge-summary"
	} else if strings.Contains(path, "/price-snapshots/") {
		template = "/api/management/billing-accounts/{billing_account_id}/price-snapshots/{price_snapshot_id}"
	} else if strings.Contains(path, "/charges/") {
		template += "/{charge_id}"
	}
	contract, err := openapitest.Load(filepath.Join("..", "..", openapitest.CanonicalDocumentPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateResponse(template, method, status, response.Header, payload); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestHostedRatingRejectsChangesToAcceptedPrices(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	request, err := database.admitJournalRequest(t.Context(), intent("immutable-price"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	var retained managedPriceSnapshotRecord
	if err := database.database.Where("request_id = ?", request.ID).First(&retained).Error; err != nil {
		t.Fatal(err)
	}
	path := "/billing-accounts/billing-journal/price-snapshots/" + retained.ID
	before := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK)
	changed, document, err := restoreHostedPriceSnapshot(retained)
	if err != nil {
		t.Fatal(err)
	}
	changed.markup = big.NewRat(2, 1)
	changed.components[0].rates[0].Rate = "99"
	replacement, err := newHostedPriceAdmission(changed, document.Bounds, document.Maximum.Attempts)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.database.Transaction(func(tx *gorm.DB) error { return replacement(tx, request) }); !errors.Is(err, errUsageJournalConflict) {
		t.Fatalf("accepted prices were replaced: %v", err)
	}
	after := ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusOK)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("historical rates changed")
	}
	if err := database.database.Model(&managedPriceSnapshotRecord{}).Where("id = ?", retained.ID).UpdateColumn("document", []byte(`{"modified":true}`)).Error; err != nil {
		t.Fatal(err)
	}
	ratingHTTPExchange(t, server, http.MethodGet, path, "", http.StatusInternalServerError)
}

func TestHostedRatingChargePagesAndReadIntegrity(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	delivery := newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })
	for _, key := range []string{"first-charge", "second-charge"} {
		_, observation := observeRatedFixture(t, database, intent(key), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "0"}, {Dimension: "output_tokens", Unit: "token", Value: "1"}})
		if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, delivery); err != nil {
			t.Fatal(err)
		}
	}
	base := "/billing-accounts/billing-journal/charges"
	first := ratingHTTPExchange(t, server, http.MethodGet, base+"?limit=1", "", http.StatusOK)
	cursor := first["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("missing next cursor")
	}
	second := ratingHTTPExchange(t, server, http.MethodGet, base+"?limit=1&cursor="+cursor, "", http.StatusOK)
	if second["next_cursor"] != "" || len(second["charges"].([]any)) != 1 {
		t.Fatalf("incorrect final page: %v", second)
	}
	firstID := first["charges"].([]any)[0].(map[string]any)["id"].(string)
	secondID := second["charges"].([]any)[0].(map[string]any)["id"].(string)
	if firstID == secondID {
		t.Fatal("page repeated a charge")
	}
	if err := database.database.Model(&managedChargeRecord{}).Where("id = ?", firstID).UpdateColumn("rating", []byte(`{}`)).Error; err != nil {
		t.Fatal(err)
	}
	ratingHTTPExchange(t, server, http.MethodGet, base+"/"+firstID, "", http.StatusInternalServerError)
	ratingHTTPExchange(t, server, http.MethodGet, base, "", http.StatusInternalServerError)
}

func TestHostedRatingAttemptLimitRejectsWorkBeforeDispatch(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	request, first := observeRatedFixture(t, database, intent("bounded-attempts"), reserve, journalOutcomeContinue, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1"}, {Dimension: "output_tokens", Unit: "token", Value: "1"}})
	claim, err := newJournalWorkerClaim(request.ID, request.OwnerToken, request.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-"+strings.Repeat("2", 32), reserve)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.dispatchJournalAttempt(t.Context(), claim, second.ID); err != nil {
		t.Fatal(err)
	}
	evidence, err := newJournalUsageEvidence(journalUsageEvidenceInput{AttemptID: second.ID, AdapterRevision: "test-native-meter", Quantities: []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1"}, {Dimension: "output_tokens", Unit: "token", Value: "1"}}, Outcome: journalOutcomeContinue, ObservedAt: claim.now}, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := database.observeJournalAttempt(t.Context(), claim, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.prepareJournalAttempt(t.Context(), claim, "attempt-"+strings.Repeat("3", 32), reserve); !errors.Is(err, errUsageJournalConflict) {
		t.Fatalf("work beyond accepted attempt limit was authorized: %v", err)
	}
	delivery := newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })
	for _, observation := range []managedJournalObservationRecord{first, observed} {
		if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, delivery); err != nil {
			t.Fatal(err)
		}
	}
	collection := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
	if len(collection["charges"].([]any)) != 2 {
		t.Fatalf("attempt charges lost: %v", collection)
	}
}

func TestHostedRatingSnapshotWriteFailureRollsBackAdmission(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	if err := database.database.Exec("CREATE TRIGGER reject_price_snapshot BEFORE INSERT ON managed_price_snapshot_records BEGIN SELECT RAISE(ABORT, 'controlled_snapshot_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := database.admitJournalRequest(t.Context(), intent("snapshot-retry"), reserve); err == nil {
		t.Fatal("snapshot write failure was ignored")
	}
	requests := accountConnectionHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/requests", "", http.StatusOK)
	if len(requests["requests"].([]any)) != 0 {
		t.Fatal("request admitted without accepted prices")
	}
	if err := database.database.Exec("DROP TRIGGER reject_price_snapshot").Error; err != nil {
		t.Fatal(err)
	}
	request, err := database.admitJournalRequest(t.Context(), intent("snapshot-retry"), reserve)
	if err != nil {
		t.Fatal(err)
	}
	snapshotID := priceSnapshotIDPrefix + sha256Hex(request.ID)[:32]
	ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/price-snapshots/"+snapshotID, "", http.StatusOK)
}

func TestHostedRatingChargeWriteFailureKeepsUsagePending(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	_, observation := observeRatedFixture(t, database, intent("charge-write-retry"), reserve, journalOutcomeComplete, []journalQuantity{{Dimension: "input_tokens", Unit: "token", Value: "1"}, {Dimension: "output_tokens", Unit: "token", Value: "1"}})
	if err := database.database.Exec("CREATE TRIGGER reject_charge BEFORE INSERT ON managed_charge_records BEGIN SELECT RAISE(ABORT, 'controlled_charge_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	delivery := newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { return nil })
	if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, delivery); err == nil {
		t.Fatal("charge write failure was ignored")
	}
	pending, err := database.pendingJournalDeliveries(t.Context(), 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("evidence lost after charge failure: %+v %v", pending, err)
	}
	collection := ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
	if len(collection["charges"].([]any)) != 0 {
		t.Fatal("partial charge committed")
	}
	if err := database.database.Exec("DROP TRIGGER reject_charge").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.deliverJournalObservation(t.Context(), observation.ID, observation.CreatedAt, delivery); err != nil {
		t.Fatal(err)
	}
	collection = ratingHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
	if len(collection["charges"].([]any)) != 1 {
		t.Fatal("replayed charge was not retained")
	}
}

func TestHostedRatingExpiredPricesCannotAdmitRequests(t *testing.T) {
	database, intent, server, reserve := newHostedRatingFixture(t)
	proposal := intent("expired-price")
	proposal.record.CreatedAt = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	proposal.record.UpdatedAt = proposal.record.CreatedAt
	proposal.record.ClaimExpiresAt = proposal.record.CreatedAt.Add(time.Minute)
	if _, err := database.admitJournalRequest(t.Context(), proposal, reserve); !errors.Is(err, ErrCatalogRatingUnavailable) {
		t.Fatalf("expired prices admitted work: %v", err)
	}
	requests := accountConnectionHTTPExchange(t, server, http.MethodGet, "/billing-accounts/billing-journal/requests", "", http.StatusOK)
	if len(requests["requests"].([]any)) != 0 {
		t.Fatal("expired request remained admitted")
	}
}

func ratingTestAcceptanceTime() time.Time { return time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC) }
