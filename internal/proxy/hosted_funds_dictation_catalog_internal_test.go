package proxy

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostedFundsDictationCatalogFinancialAcceptance(t *testing.T) {
	catalog := internalCanonicalProviderCatalog().ModelCatalog()
	providers := internalManagementProviderRegistry()
	count := 0
	for _, offering := range catalog.Offerings {
		if !slices.Contains(offering.Operations, ModelOperationDictation) {
			continue
		}
		count++
		modes := []string{"tokens"}
		if offering.WireContract == CatalogProtocolMultipartTranscription {
			modes = append(modes, "duration")
		}
		for _, mode := range modes {
			t.Run(offering.Provider+"/"+offering.Model+"/"+mode, func(t *testing.T) {
				database, _, management, _ := newHostedRatingFixture(t)
				payload, providerCost, customerCharge, posted, remainder, hold := hostedDictationFinancialResponse(t, offering.WireContract, mode)
				var calls atomic.Int64
				var missingUsage atomic.Bool
				upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.Method == http.MethodDelete {
						writer.WriteHeader(http.StatusNoContent)
						return
					}
					calls.Add(1)
					transport := providers.definitions[providerID(offering.Provider)].transports[offering.Transport]
					secret := "sk-platform-dictation"
					if offering.Provider == "openai" {
						secret = "sk-platform-pinned"
					}
					if request.Method != http.MethodPost || request.Header.Get(transport.authentication.Header) != transport.authentication.Prefix+secret {
						t.Error("dictation lost platform authority")
					}
					var nativeModel string
					expectedModel := offering.ProviderModel
					if offering.WireContract == CatalogProtocolMultipartTranscription {
						if err := request.ParseMultipartForm(1 << 20); err != nil {
							t.Error(err)
							return
						}
						defer request.MultipartForm.RemoveAll()
						field := transport.protocolParameters.ModelField
						if field == "" {
							expectedModel = ""
							nativeModel = request.FormValue("model")
						} else {
							nativeModel = request.FormValue(field)
						}
					} else if offering.WireContract == CatalogProtocolVertexGenerateContent {
						if !strings.Contains(request.URL.Path, offering.ProviderModel) {
							t.Errorf("Vertex lost native model: %s", request.URL.Path)
						}
						nativeModel = offering.ProviderModel
					} else {
						var input struct {
							Model string `json:"model"`
						}
						if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
							t.Error(err)
							return
						}
						nativeModel = input.Model
					}
					if nativeModel != expectedModel {
						t.Errorf("native model=%s want=%s", nativeModel, expectedModel)
					}
					writer.Header().Set("Content-Type", "application/json")
					if missingUsage.Load() {
						var body map[string]any
						if err := json.Unmarshal([]byte(payload), &body); err != nil {
							t.Error(err)
							return
						}
						delete(body, "usage")
						delete(body, "usageMetadata")
						if err := json.NewEncoder(writer).Encode(body); err != nil {
							t.Error(err)
						}
						return
					}
					fmt.Fprint(writer, payload)
				}))
				t.Cleanup(upstream.Close)
				settings := hostedDictationFinancialSettings(t, offering, mode)
				server := newHostedDictationProviderServer(t, database, upstream.URL, t.TempDir(), offering.Provider, offering.Model, func(dependencies *hostedTextRequestDependencies) {
					dependencies.authorize = settings.authorizeCompletion
				})
				if offering.Provider == "openai" {
					scope, err := json.Marshal([]hostedGrantOffering{{Model: offering.Model, Operations: []string{ModelOperationDictation}}})
					if err != nil {
						t.Fatal(err)
					}
					if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("offerings", scope).Error; err != nil {
						t.Fatal(err)
					}
				}
				audio := []byte("RIFF\x00\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x80\x3e\x00\x00\x00\x7d\x00\x00\x02\x00\x10\x00data\x02\x00\x00\x00\x00\x00")
				binary.LittleEndian.PutUint32(audio[4:], uint32(len(audio)-8))
				paths := []string{dictatePath, transcriptionsPath}
				status := http.StatusPaymentRequired
				if offering.WireContract == CatalogProtocolMetaTranscription {
					status = http.StatusServiceUnavailable
				}
				for _, path := range paths {
					hostedDictationProviderHTTP(t, server, path, "unfunded", string(audio), offering.Provider, offering.Model, status)
				}
				if calls.Load() != 0 {
					t.Fatal("unfunded dictation dispatched")
				}
				seedHostedFunds(t, database, 500)
				if offering.WireContract == CatalogProtocolMetaTranscription {
					for _, path := range paths {
						body := hostedDictationProviderHTTP(t, server, path, "unsupported-meter", string(audio), offering.Provider, offering.Model, status)
						if !strings.Contains(body, errFinancialAdmissionUnavailable.Error()) {
							t.Fatalf("missing financial qualification error: %s", body)
						}
					}
					if calls.Load() != 0 {
						t.Fatal("unsupported meter dispatched")
					}
					assertHostedFundsBalance(t, database, 500, 500)
					return
				}
				for index, path := range paths {
					key := fmt.Sprintf("dictation-%d", index)
					body := hostedDictationProviderHTTP(t, server, path, key, string(audio), offering.Provider, offering.Model, http.StatusOK)
					if !strings.Contains(body, "funded transcript") {
						t.Fatalf("transcript=%s", body)
					}
					for _, replayPath := range paths {
						hostedDictationProviderHTTP(t, server, replayPath, key, string(audio), offering.Provider, offering.Model, http.StatusOK)
					}
				}
				assertHostedFundsBalance(t, database, 500, 500-2*hold)
				for range 2 {
					if err := database.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
						t.Fatal(err)
					}
				}
				assertHostedFundsBalance(t, database, posted, posted)
				assertFundsCreditRemainder(t, database, remainder.Numerator, remainder.Denominator)
				charges := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)["charges"].([]any)
				if len(charges) != 2 || calls.Load() != 2 {
					t.Fatalf("charges=%d calls=%d", len(charges), calls.Load())
				}
				for _, item := range charges {
					charge := item.(map[string]any)
					if charge["state"] != chargeRated || !reflect.DeepEqual(charge["customer_charge"], customerCharge) || !reflect.DeepEqual(charge["rating"].(map[string]any)["provider_cost"], providerCost) {
						t.Fatalf("dictation charge=%v", charge)
					}
				}
				for index := range paths {
					for _, path := range paths {
						hostedDictationProviderHTTP(t, server, path, fmt.Sprintf("dictation-%d", index), string(audio), offering.Provider, offering.Model, http.StatusOK)
					}
				}
				if calls.Load() != 2 {
					t.Fatal("settled replay dispatched")
				}
				missingUsage.Store(true)
				for _, path := range paths {
					hostedDictationProviderHTTP(t, server, path, "missing-usage", string(audio), offering.Provider, offering.Model, http.StatusOK)
				}
				for range 2 {
					if err := database.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
						t.Fatal(err)
					}
				}
				assertHostedFundsBalance(t, database, posted, posted-hold)
				assertFundsCreditRemainder(t, database, remainder.Numerator, remainder.Denominator)
				charges = ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)["charges"].([]any)
				if len(charges) != 3 || calls.Load() != 3 {
					t.Fatalf("missing-usage charges=%d calls=%d", len(charges), calls.Load())
				}
				unresolved := 0
				for _, item := range charges {
					charge := item.(map[string]any)
					if charge["state"] != chargeUsageUnresolved {
						continue
					}
					unresolved++
					if charge["customer_charge"] != nil {
						t.Fatalf("unknown usage became a charge: %v", charge)
					}
					summary := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+charge["request_id"].(string)+"/charge-summary", "", http.StatusOK)
					if summary["state"] != string(requestChargeUnresolved) || summary["provider_cost"] != nil || summary["customer_charge"] != nil {
						t.Fatalf("unknown usage became a total: %v", summary)
					}
				}
				if unresolved != 1 {
					t.Fatalf("unresolved charges=%d", unresolved)
				}
			})
		}
	}
	if count == 0 {
		t.Fatal("dictation catalog is empty")
	}
}

func hostedDictationFinancialSettings(t *testing.T, offering ProviderOffering, mode string) *hostedRuntimeSettings {
	t.Helper()
	catalog := internalCanonicalProviderCatalog().ModelCatalog()
	catalog.Revision = "journal-catalog"
	rates := []CatalogPriceRate{}
	for index, component := range []string{"input_audio", "input_text", "output_text"} {
		rates = append(rates, CatalogPriceRate{Component: component, Currency: "USD", Rate: []CatalogDecimal{"20", "10", "40"}[index], Unit: "USD/1M_tokens", Conditions: CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}})
	}
	if mode == "duration" {
		rates = []CatalogPriceRate{{Component: "input_audio", Currency: "USD", Rate: "0.6", Unit: "USD/minute", Conditions: CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}}}
	}
	for index := range catalog.Prices {
		price := &catalog.Prices[index]
		if price.Provider == offering.Provider && price.Model == offering.Model && price.Operation == ModelOperationDictation {
			*price = CatalogPriceDescriptor{Provider: offering.Provider, Model: offering.Model, Operation: ModelOperationDictation, Available: true, Source: "https://example.com/controlled-dictation-rates", LastVerified: "2026-09-23", Rates: rates}
		}
	}
	for index := range catalog.Offerings {
		route := &catalog.Offerings[index]
		if route.Provider != offering.Provider || route.Model != offering.Model {
			continue
		}
		dimensions := []string{"input_audio_tokens", "input_text_tokens", "output_tokens"}
		if offering.WireContract == CatalogProtocolMultipartTranscription {
			dimensions = append(dimensions, "audio_seconds")
		} else if offering.WireContract == CatalogProtocolGeminiInteractions || offering.WireContract == CatalogProtocolVertexGenerateContent {
			dimensions = append(dimensions, "reasoning_tokens")
		} else {
			dimensions = nil
		}
		for _, dimension := range dimensions {
			route.Limits = slices.DeleteFunc(route.Limits, func(limit CatalogLimit) bool { return limit.ID == dimension })
			value, unit := 1000, "tokens"
			if dimension == "audio_seconds" {
				value, unit = 60, "seconds"
			}
			route.Limits = append(route.Limits, CatalogLimit{ID: dimension, Unit: unit, Value: &value})
		}
	}
	settings, err := newHostedRuntimeSettings(&HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: offering.Provider, Model: offering.Model, Operation: ModelOperationDictation, MaximumAttempts: 1}}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return settings
}

func hostedDictationFinancialResponse(t *testing.T, codec, mode string) (string, any, any, int64, ExactMoney, int64) {
	t.Helper()
	if codec == CatalogProtocolMetaTranscription {
		return `{"transcript":"funded transcript"}`, nil, nil, 500, ExactMoney{Numerator: "0", Denominator: "1"}, 0
	}
	if codec == CatalogProtocolMultipartTranscription {
		if mode == "duration" {
			return `{"text":"funded transcript","usage":{"type":"duration","seconds":1.25}}`, map[string]any{"numerator": "1", "denominator": "80"}, map[string]any{"numerator": "13", "denominator": "800"}, 497, ExactMoney{Numerator: "1", Denominator: "400"}, 78
		}
		return `{"text":"funded transcript","usage":{"type":"tokens","input_tokens":1100,"output_tokens":200,"total_tokens":1300,"input_token_details":{"audio_tokens":1000,"text_tokens":100}}}`, map[string]any{"numerator": "29", "denominator": "1000"}, map[string]any{"numerator": "377", "denominator": "10000"}, 493, ExactMoney{Numerator: "27", Denominator: "5000"}, 10
	}
	var payload string
	switch codec {
	case CatalogProtocolGeminiInteractions:
		payload = `{"id":"financial-audio","status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"funded transcript"}]}],"usage":{"total_input_tokens":1100,"total_output_tokens":200,"total_cached_tokens":0,"total_thought_tokens":50,"total_tool_use_tokens":0,"input_tokens_by_modality":[{"modality":"audio","tokens":1000},{"modality":"text","tokens":100}],"output_tokens_by_modality":[{"modality":"text","tokens":200}]}}`
	case CatalogProtocolVertexGenerateContent:
		payload = `{"candidates":[{"content":{"parts":[{"text":"funded transcript"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1100,"candidatesTokenCount":200,"cachedContentTokenCount":0,"thoughtsTokenCount":50,"toolUsePromptTokenCount":0,"promptTokensDetails":[{"modality":"AUDIO","tokenCount":1000},{"modality":"TEXT","tokenCount":100}],"candidatesTokensDetails":[{"modality":"TEXT","tokenCount":200}]}}`
	default:
		t.Fatalf("unqualified dictation protocol %s", codec)
	}
	return payload, map[string]any{"numerator": "31", "denominator": "1000"}, map[string]any{"numerator": "403", "denominator": "10000"}, 492, ExactMoney{Numerator: "3", Denominator: "5000"}, 15
}
