package proxy

import (
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHostedRatingImageChargesNativeModalitiesOnce(t *testing.T) {
	for _, scenario := range []struct {
		name, usage, bound, state string
	}{
		{"measured", `{"total_tokens":22,"input_tokens":13,"output_tokens":9,"input_tokens_details":{"text_tokens":10,"image_tokens":3},"output_tokens_details":{"text_tokens":2,"image_tokens":7}}`, "100", chargeRated},
		{"missing modalities", `{"total_tokens":22,"input_tokens":13,"output_tokens":9}`, "100", chargeUsageUnresolved},
		{"exceeded bound", `{"total_tokens":22,"input_tokens":13,"output_tokens":9,"input_tokens_details":{"text_tokens":10,"image_tokens":3},"output_tokens_details":{"text_tokens":2,"image_tokens":7}}`, "6", chargeLimitUnresolved},
		{"missing bound", `{}`, "", ""},
		{"account dependent bound", `{}`, "account", ""},
		{"wrong unit", `{}`, "bytes", ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			database, _, management, _ := newHostedRatingFixture(t)
			encoded := base64.StdEncoding.EncodeToString(imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(writer, `{"data":[{"b64_json":%q}],"usage":%s}`, encoded, scenario.usage)
			}))
			t.Cleanup(upstream.Close)
			server, worker := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
			rates := []CatalogPriceRate{}
			for index, component := range []string{"input_text", "input_image", "output_text", "output_image"} {
				rates = append(rates, CatalogPriceRate{Component: component, Currency: "USD", Rate: CatalogDecimal(fmt.Sprint(2 << index)), Unit: "USD/1M_tokens", Conditions: CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}})
			}
			prices := hostedMediaPriceCatalog(t, "openai", "gpt-image-2", ModelOperationImageGeneration, rates, func(offering *ProviderOffering) {
				for _, dimension := range []string{"input_text_tokens", "input_image_tokens", "output_text_tokens", "output_image_tokens"} {
					maximum, unit, dependent := 100, "tokens", false
					if dimension == "output_image_tokens" {
						switch scenario.bound {
						case "":
							continue
						case "account":
							dependent = true
						case "bytes":
							unit = "bytes"
						default:
							maximum, _ = strconv.Atoi(scenario.bound)
						}
					}
					limit := CatalogLimit{ID: dimension, Unit: unit, Value: &maximum, AccountDependent: dependent}
					if dependent {
						limit.Value = nil
					}
					offering.Limits = append(offering.Limits, limit)
				}
			})
			worker.hostedAdmission = func(transaction *gorm.DB, request managedJournalRequestRecord) error {
				reserve, err := newHostedMediaPriceAdmission(prices, request, CatalogProtocolOpenAIImages, CatalogPriceConditions{}, 1)
				if err != nil {
					return err
				}
				return reserve(transaction, request)
			}
			if scenario.state == "" {
				hostedMediaAdmissionHTTP(t, server, "priced-image", "private image prompt", http.StatusUnprocessableEntity)
				var accepted int64
				if err := database.database.Model(&managedJournalRequestRecord{}).Count(&accepted).Error; err != nil || accepted != 0 || calls.Load() != 0 {
					t.Fatalf("unbounded image dispatched: accepted=%d calls=%d error=%v", accepted, calls.Load(), err)
				}
				return
			}
			accepted := hostedMediaAdmissionHTTP(t, server, "priced-image", "private image prompt", http.StatusAccepted)
			id := accepted["operation_id"].(string)
			worker.runOperation("priced-image-worker", id)
			if result := hostedMediaWorkerStatus(t, server, id); result["state"] != MediaOperationStateSucceeded {
				t.Fatalf("image did not complete: %v", result)
			}
			hostedMediaAdmissionHTTP(t, server, "priced-image", "private image prompt", http.StatusOK)
			worker.runOperation("duplicate-image-worker", id)
			pending, err := database.pendingJournalDeliveries(t.Context(), 10)
			if err != nil || len(pending) != 1 || calls.Load() != 1 {
				t.Fatalf("image observations=%v calls=%d error=%v", pending, calls.Load(), err)
			}
			settlements := 0
			deliver := newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error { settlements++; return nil })
			for range 2 {
				if err := database.deliverJournalObservation(t.Context(), pending[0].ID, pending[0].ObservedAt, deliver); err != nil {
					t.Fatal(err)
				}
			}
			collection := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
			charges := collection["charges"].([]any)
			if len(charges) != 1 {
				t.Fatalf("duplicate image charges: %v", collection)
			}
			charge := charges[0].(map[string]any)
			if charge["state"] != scenario.state {
				t.Fatalf("image charge: %v", charge)
			}
			if scenario.state == chargeRated {
				// (10*2 + 3*4 + 2*8 + 7*16)/1M * 13/10 = 13/62500.
				if settlements != 1 || !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": "13", "denominator": "62500"}) {
					t.Fatalf("image settlement count=%d charge=%v", settlements, charge)
				}
			} else if settlements != 0 || charge["customer_charge"] != nil {
				t.Fatalf("unresolved image settled: count=%d charge=%v", settlements, charge)
			}
		})
	}
}

func TestHostedRatingDictationUsesReportedDurationAndSelectedTimeRate(t *testing.T) {
	for _, test := range []struct{ name, usage, state string }{
		{"duration", `{"type":"duration","seconds":1.25}`, chargeRated},
		{"missing", `{}`, chargeUsageUnresolved},
		{"token meter", `{"type":"tokens","input_tokens":10,"output_tokens":2,"total_tokens":12,"input_token_details":{"audio_tokens":8,"text_tokens":2}}`, chargeUsageUnresolved},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, _, management, _ := newHostedRatingFixture(t)
			grantHostedDictation(t, database)
			catalog := internalTestModelCatalog(internalTestOffering("openai", "gpt-transcribe", []string{ModelOperationDictation}, []string{ModelOperationDictation}))
			catalog.Revision = "journal-catalog"
			audioSeconds := 60
			catalog.Offerings[0].Limits = []CatalogLimit{{ID: "audio_seconds", Unit: "seconds", Value: &audioSeconds}}
			conditions := CatalogPriceConditions{ServiceTier: "standard", EffectiveFrom: "2026-09-01T00:00:00Z"}
			catalog.Prices[0] = CatalogPriceDescriptor{Provider: "openai", Model: "gpt-transcribe", Operation: ModelOperationDictation, Available: true, Source: "https://example.com/prices", LastVerified: "2026-09-22", Rates: []CatalogPriceRate{{Component: "input_audio", Currency: "USD", Rate: "0.0045", Unit: "USD/minute", Conditions: conditions}}}
			prices, err := NewCatalogService(catalog)
			if err != nil {
				t.Fatal(err)
			}
			reserve, err := newHostedMediaPriceAdmission(prices, managedJournalRequestRecord{Provider: "openai", Model: "gpt-transcribe", Operation: ModelOperationDictation, CreatedAt: ratingTestAcceptanceTime()}, CatalogProtocolMultipartTranscription, categoricalPriceConditions(conditions), 1)
			if err != nil {
				t.Fatal(err)
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(writer, `{"text":"priced transcript","usage":%s}`, test.usage)
			}))
			t.Cleanup(upstream.Close)
			server := newHostedDictationServer(t, database, upstream.URL, t.TempDir(), func(dependencies *hostedTextRequestDependencies) {
				dependencies.authorize = reserve
				dependencies.now = func() time.Time { return ratingTestAcceptanceTime() }
			})
			hostedDictationHTTP(t, server, dictatePath, "priced-audio", "private audio", http.StatusOK)
			pending, err := database.pendingJournalDeliveries(t.Context(), 10)
			if err != nil || len(pending) != 1 {
				t.Fatalf("dictation observations=%v error=%v", pending, err)
			}
			if err := database.deliverJournalObservation(t.Context(), pending[0].ID, pending[0].ObservedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error {
				if test.state != chargeRated {
					return errors.New("unmeasured duration reached settlement")
				}
				return nil
			})); err != nil {
				t.Fatal(err)
			}
			collection := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
			charge := collection["charges"].([]any)[0].(map[string]any)
			if charge["state"] != test.state {
				t.Fatalf("dictation charge state: %v", charge)
			}
			if test.state == chargeRated {
				if !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": "39", "denominator": "320000"}) {
					t.Fatalf("duration rate conversion: %v", charge)
				}
			} else if charge["customer_charge"] != nil {
				t.Fatalf("unmeasured duration became a charge: %v", charge)
			}
		})
	}
}

func TestHostedRatingSpeechConvertsOnlyExplicitlyPricedProviderUnits(t *testing.T) {
	for _, reported := range []bool{true, false} {
		t.Run(fmt.Sprint(reported), func(t *testing.T) {
			database, _, management, _ := newHostedRatingFixture(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if reported {
					writer.Header().Set(speechCharacterCostHeader, "2.5")
				}
				writer.Header().Set("Content-Type", "audio/mpeg")
				fmt.Fprint(writer, "controlled audio")
			}))
			t.Cleanup(upstream.Close)
			server, worker, intent := newHostedSpeechUsageFixture(t, database, "elevenlabs", "eleven_v3", upstream.URL)
			conditions := CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}
			prices := hostedMediaPriceCatalog(t, "elevenlabs", "eleven_v3", ModelOperationSpeechGeneration, []CatalogPriceRate{{Component: "character_cost", Currency: "USD", Rate: "0.004", Unit: "USD/provider_unit", Conditions: conditions}}, func(offering *ProviderOffering) {
				maximum := 10
				offering.Limits = append(offering.Limits, CatalogLimit{ID: "character_cost", Unit: "provider_units", Value: &maximum})
			})
			worker.hostedAdmission = func(transaction *gorm.DB, request managedJournalRequestRecord) error {
				reserve, err := newHostedMediaPriceAdmission(prices, request, CatalogProtocolElevenLabsSpeech, categoricalPriceConditions(conditions), 1)
				if err != nil {
					return err
				}
				return reserve(transaction, request)
			}
			accepted := hostedSpeechHTTP(t, server, "priced-speech", intent, http.StatusAccepted)
			id := accepted["operation_id"].(string)
			worker.runOperation("priced-speech-worker", id)
			if result := hostedMediaWorkerStatus(t, server, id); result["state"] != MediaOperationStateSucceeded {
				t.Fatalf("speech did not complete: %v", result)
			}
			pending, err := database.pendingJournalDeliveries(t.Context(), 10)
			if err != nil || len(pending) != 1 {
				t.Fatalf("speech observations=%v error=%v", pending, err)
			}
			if err := database.deliverJournalObservation(t.Context(), pending[0].ID, pending[0].ObservedAt, newJournalRatingDelivery(func(*gorm.DB, managedChargeRecord) error {
				if !reported {
					return errors.New("unreported provider units reached settlement")
				}
				return nil
			})); err != nil {
				t.Fatal(err)
			}
			collection := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)
			charge := collection["charges"].([]any)[0].(map[string]any)
			if reported {
				if charge["state"] != chargeRated || !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": "13", "denominator": "1000"}) {
					t.Fatalf("provider units lost their catalog rate: %v", charge)
				}
			} else if charge["state"] != chargeUsageUnresolved || charge["customer_charge"] != nil {
				t.Fatalf("missing provider cost became a charge: %v", charge)
			}
		})
	}
}

func hostedMediaPriceCatalog(t *testing.T, provider, model, operation string, rates []CatalogPriceRate, configure ...func(*ProviderOffering)) CatalogService {
	t.Helper()
	catalog := internalCanonicalProviderCatalog().ModelCatalog()
	found := false
	for index := range catalog.Prices {
		price := &catalog.Prices[index]
		if price.Provider == provider && price.Model == model && price.Operation == operation {
			*price = CatalogPriceDescriptor{Provider: provider, Model: model, Operation: operation, Available: true, Source: "https://example.com/prices", LastVerified: "2026-09-22", Rates: rates}
			found = true
		}
	}
	if !found {
		t.Fatalf("no catalog offering for %s/%s/%s", provider, model, operation)
	}
	for index := range catalog.Offerings {
		offering := &catalog.Offerings[index]
		if offering.Provider == provider && offering.Model == model {
			for _, apply := range configure {
				apply(offering)
			}
		}
	}
	service, err := NewCatalogService(catalog)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestHostedRatingMediaBindingsUseExactNativeDimensions(t *testing.T) {
	for _, test := range []struct {
		provider, operation, codec, component, rateUnit, dimension, unit, quantity string
		expected                                                                   ExactMoney
	}{
		{"openai", ModelOperationImageGeneration, CatalogProtocolOpenAIImages, "output_image", "USD/1M_tokens", "output_image_tokens", "token", "7", ExactMoney{Numerator: "91", Denominator: "2500000"}},
		{"openai", ModelOperationImageEditing, CatalogProtocolOpenAIImages, "input_image", "USD/1M_tokens", "input_image_tokens", "token", "7", ExactMoney{Numerator: "91", Denominator: "2500000"}},
		{"openai", ModelOperationDictation, CatalogProtocolMultipartTranscription, "input_audio", "USD/minute", "audio_seconds", "second", "1.25", ExactMoney{Numerator: "13", Denominator: "120"}},
		{"openai", ModelOperationDictation, CatalogProtocolMultipartTranscription, "input_audio", "USD/1M_tokens", "input_audio_tokens", "token", "7", ExactMoney{Numerator: "91", Denominator: "2500000"}},
		{"gemini", ModelOperationDictation, CatalogProtocolGeminiInteractions, "input_audio", "USD/1M_tokens", "input_audio_tokens", "token", "7", ExactMoney{Numerator: "91", Denominator: "2500000"}},
		{"elevenlabs", ModelOperationSpeechConversion, CatalogProtocolElevenLabsConversion, "character_cost", "USD/provider_unit", "character_cost", "provider_unit", "2.5", ExactMoney{Numerator: "13", Denominator: "1"}},
		{"fal", ModelOperationImageGeneration, CatalogProtocolFALQueueImages, "billable_units", "USD/provider_unit", "billable_units", "provider_unit", "2.5", ExactMoney{Numerator: "13", Denominator: "1"}},
		{"dictator", ModelOperationSpeechGeneration, CatalogProtocolDictatorSpeechV1, "output_audio", "USD/second", "output_audio_seconds", "second", "1.25", ExactMoney{Numerator: "13", Denominator: "2"}},
	} {
		t.Run(test.provider+"/"+test.operation+"/"+test.rateUnit, func(t *testing.T) {
			var model string
			for _, offering := range internalCanonicalProviderCatalog().ModelCatalog().Offerings {
				if offering.Provider == test.provider && offeringSupportsOperation(offering, test.operation) {
					model = offering.Model
					break
				}
			}
			if model == "" {
				t.Fatal("no existing media offering")
			}
			prices := hostedMediaPriceCatalog(t, test.provider, model, test.operation, []CatalogPriceRate{{Component: test.component, Currency: "USD", Rate: "4", Unit: test.rateUnit}})
			snapshot, err := newHostedMediaRatingSnapshot(prices, test.provider, model, test.operation, test.codec, ratingTestAcceptanceTime(), CatalogPriceConditions{})
			if err != nil {
				t.Fatal(err)
			}
			quantities := []CatalogUsageQuantity{{Dimension: test.dimension, Unit: test.unit, Value: test.quantity}}
			if test.codec == CatalogProtocolGeminiInteractions {
				quantities = append(quantities, CatalogUsageQuantity{Dimension: "tool_input_tokens", Unit: "token", Value: "0"})
			}
			result, err := snapshot.Rate(quantities)
			if err != nil || result.State != CatalogRatingResolved || result.CustomerCharge != test.expected {
				t.Fatalf("media result=%+v error=%v", result, err)
			}
			unknown, err := snapshot.Rate([]CatalogUsageQuantity{{Dimension: test.dimension, Unit: test.unit, UnknownReason: journalQuantityNotReported}})
			if err != nil || unknown.State != CatalogRatingUnresolved {
				t.Fatalf("unknown native quantity settled: %+v %v", unknown, err)
			}
		})
	}
}

func TestHostedRatingMediaRejectsUnqualifiedUnitConversions(t *testing.T) {
	prices := hostedMediaPriceCatalog(t, "elevenlabs", "eleven_v3", ModelOperationSpeechGeneration, []CatalogPriceRate{{Component: "character_cost", Currency: "USD", Rate: "4", Unit: "USD/minute"}})
	if _, err := newHostedMediaRatingSnapshot(prices, "elevenlabs", "eleven_v3", ModelOperationSpeechGeneration, CatalogProtocolElevenLabsSpeech, ratingTestAcceptanceTime(), CatalogPriceConditions{}); !errors.Is(err, ErrCatalogRatingUnavailable) {
		t.Fatalf("provider units were converted to time: %v", err)
	}
	prices = hostedMediaPriceCatalog(t, "openai", "gpt-image-2", ModelOperationImageGeneration, []CatalogPriceRate{{Component: "cache_read_image", Currency: "USD", Rate: "4", Unit: "USD/1M_tokens", Conditions: CatalogPriceConditions{CacheClass: "read"}}})
	if _, err := newHostedMediaRatingSnapshot(prices, "openai", "gpt-image-2", ModelOperationImageGeneration, CatalogProtocolOpenAIImages, ratingTestAcceptanceTime(), CatalogPriceConditions{}); !errors.Is(err, ErrCatalogRatingUnavailable) {
		t.Fatalf("unsupported image cache meter admitted: %v", err)
	}
}
