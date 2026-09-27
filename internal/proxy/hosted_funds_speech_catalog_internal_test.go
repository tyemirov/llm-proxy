package proxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostedFundsSpeechCatalogSettlesExactProviderUnits(t *testing.T) {
	count := 0
	for _, offering := range internalCanonicalProviderCatalog().ModelCatalog().Offerings {
		if offering.WireContract != CatalogProtocolElevenLabsSpeech && offering.WireContract != CatalogProtocolElevenLabsConversion {
			continue
		}
		count++
		t.Run(offering.Provider+"/"+offering.Model, func(t *testing.T) {
			database, _, management, _ := newHostedRatingFixture(t)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.Method != http.MethodPost || request.Header.Get("xi-api-key") != "hosted-voice-secret" {
					t.Error("speech lost accepted platform authority")
				}
				var model string
				if offering.WireContract == CatalogProtocolElevenLabsConversion {
					if err := request.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						return
					}
					defer request.MultipartForm.RemoveAll()
					model = request.FormValue("model_id")
					file, _, err := request.FormFile("audio")
					if err != nil {
						t.Error(err)
						return
					}
					defer file.Close()
					data, err := io.ReadAll(file)
					if err != nil || string(data) != "1234" {
						t.Error("conversion changed its owned audio input")
					}
				} else {
					var input struct {
						Model string `json:"model_id"`
					}
					if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
						t.Error(err)
						return
					}
					model = input.Model
				}
				if model != offering.ProviderModel {
					t.Errorf("native model=%s want=%s", model, offering.ProviderModel)
				}
				writer.Header().Set("request-id", "funded-catalog-speech")
				writer.Header().Set(speechCharacterCostHeader, "12.5")
				if strings.HasSuffix(request.URL.Path, "/with-timestamps") {
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(writer, `{"audio_base64":%q}`, base64.StdEncoding.EncodeToString([]byte("controlled audio")))
				} else {
					writer.Header().Set("Content-Type", "audio/mpeg")
					fmt.Fprint(writer, "controlled audio")
				}
			}))
			t.Cleanup(upstream.Close)
			operation := ModelOperationSpeechGeneration
			modes := []string{"plain", "timed"}
			if offering.WireContract == CatalogProtocolElevenLabsConversion {
				operation = ModelOperationSpeechConversion
				modes = modes[:1]
			}
			server, worker, intent := newHostedSpeechUsageFixture(t, database, offering.Provider, offering.Model, upstream.URL, hostedSpeechFundsConfiguration(t, offering, operation))
			hostedSpeechHTTP(t, server, "unfunded", intent, http.StatusPaymentRequired)
			if calls.Load() != 0 {
				t.Fatal("unfunded speech reached the provider")
			}
			seedHostedFunds(t, database, 500)
			for index, mode := range modes {
				body := intent
				if mode == "timed" {
					body = strings.Replace(body, `"timestamps":false`, `"timestamps":true`, 1)
				}
				accepted := hostedSpeechHTTP(t, server, mode, body, http.StatusAccepted)
				id := accepted["operation_id"].(string)
				// The fixture ceiling is 100 provider units at USD 0.004 per unit.
				before := int64(500 - index*6)
				assertHostedFundsBalance(t, database, before, before-52)
				if replay := hostedSpeechHTTP(t, server, mode, body, http.StatusOK); replay["operation_id"] != id {
					t.Fatal("queued replay changed identity")
				}
				worker.runOperation("funded-speech", id)
				if result := hostedMediaWorkerStatus(t, server, id); result["state"] != MediaOperationStateSucceeded {
					t.Fatalf("funded speech result=%v", result)
				}
				for range 2 {
					if err := database.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
						t.Fatal(err)
					}
				}
				worker.runOperation("duplicate-speech", id)
				hostedSpeechHTTP(t, server, mode, body, http.StatusOK)
				// Each response costs USD 0.05 and charges USD 0.065. The second
				// settlement consumes the retained half-cent without double rounding.
				total := int64(494)
				numerator, denominator := "1", "200"
				if index == 1 {
					total = 487
					numerator, denominator = "0", "1"
				}
				assertHostedFundsBalance(t, database, total, total)
				assertFundsCreditRemainder(t, database, numerator, denominator)
			}
			charges := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)["charges"].([]any)
			if len(charges) != len(modes) || calls.Load() != int64(len(modes)) {
				t.Fatalf("charges=%d provider calls=%d expected=%d", len(charges), calls.Load(), len(modes))
			}
			for _, value := range charges {
				charge := value.(map[string]any)
				if charge["state"] != chargeRated || !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": "13", "denominator": "200"}) || !reflect.DeepEqual(charge["rating"].(map[string]any)["provider_cost"], map[string]any{"numerator": "1", "denominator": "20"}) {
					t.Fatalf("speech charge=%v", charge)
				}
			}
		})
	}
	if count == 0 {
		t.Fatal("catalog has no speech or conversion offerings")
	}
}

// Supply rates and ceilings at the existing catalog/configuration boundary.
// No reservation, rating, journal, or financial worker is replaced.
func hostedSpeechFundsConfiguration(t *testing.T, offering ProviderOffering, operation string) func(*mediaOperationService) {
	t.Helper()
	catalog := internalCanonicalProviderCatalog().ModelCatalog()
	for index := range catalog.Offerings {
		route := &catalog.Offerings[index]
		if route.Provider == offering.Provider && route.Model == offering.Model {
			maximum := 100
			route.Limits = append(route.Limits, CatalogLimit{ID: "character_cost", Unit: "provider_units", Value: &maximum})
		}
	}
	for index := range catalog.Prices {
		price := &catalog.Prices[index]
		if price.Provider == offering.Provider && price.Model == offering.Model && price.Operation == operation {
			*price = CatalogPriceDescriptor{Provider: price.Provider, Model: price.Model, Operation: operation, Available: true, Source: "https://example.com/controlled-speech-rates", LastVerified: "2026-09-23", Rates: []CatalogPriceRate{{Component: "character_cost", Currency: "USD", Rate: "0.004", Unit: "USD/provider_unit", Conditions: CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}}}}
		}
	}
	settings, err := newHostedRuntimeSettings(&HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: offering.Provider, Model: offering.Model, Operation: operation, MaximumAttempts: 1}}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return func(service *mediaOperationService) {
		service.catalog = settings.catalog
		service.hostedAdmission = settings.mediaAdmission(service.providers)
	}
}
