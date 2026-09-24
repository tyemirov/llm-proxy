package proxy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestHostedFundsImageGenerationAndEditing(t *testing.T) {
	png := imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024)))
	encoded := base64.StdEncoding.EncodeToString(png)
	for _, operation := range []string{ModelOperationImageGeneration, ModelOperationImageEditing} {
		for _, mode := range []string{"json", "stream", "zero", "missing", "invalid_output", "responses_unknown_meter"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				database, _, management, _ := newHostedRatingFixture(t)
				var calls atomic.Int64
				usage := `{"total_tokens":22,"input_tokens":13,"output_tokens":9,"input_tokens_details":{"text_tokens":10,"image_tokens":3},"output_tokens_details":{"text_tokens":2,"image_tokens":7}}`
				if mode == "zero" {
					usage = `{"total_tokens":0,"input_tokens":0,"output_tokens":0,"input_tokens_details":{"text_tokens":0,"image_tokens":0},"output_tokens_details":{"text_tokens":0,"image_tokens":0}}`
				}
				if mode == "missing" {
					usage = `null`
				}
				output := encoded
				if mode == "invalid_output" {
					output = base64.StdEncoding.EncodeToString([]byte("invalid image"))
				}
				upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					calls.Add(1)
					if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer sk-platform-pinned" {
						t.Error("image lost accepted authority")
					}
					if operation == ModelOperationImageEditing && mode != "responses_unknown_meter" {
						if err := request.ParseMultipartForm(1 << 20); err != nil {
							t.Error(err)
							return
						}
						defer request.MultipartForm.RemoveAll()
						if request.FormValue("model") != "gpt-image-2" {
							t.Error("wrong native image model")
						}
						file, _, err := request.FormFile("image[]")
						if err != nil {
							t.Error(err)
							return
						}
						defer file.Close()
						data, err := io.ReadAll(file)
						if err != nil || !bytes.Equal(data, png) {
							t.Error("editing input changed")
						}
					}
					if mode == "stream" {
						event := "image_generation.completed"
						if operation == ModelOperationImageEditing {
							event = "image_edit.completed"
						}
						writer.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprintf(writer, "event: %s\ndata: {\"type\":%q,\"b64_json\":%q,\"output_format\":\"png\",\"usage\":%s}\n\n", event, event, output, usage)
					} else {
						writer.Header().Set("Content-Type", "application/json")
						if mode == "responses_unknown_meter" {
							fmt.Fprintf(writer, `{"id":"image-response","status":"completed","output":[{"id":"image-result","type":"image_generation_call","status":"completed","result":%q}],"usage":{"input_tokens":13,"output_tokens":9,"total_tokens":22,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`, output)
						} else {
							fmt.Fprintf(writer, `{"data":[{"b64_json":%q}],"usage":%s}`, output, usage)
						}
					}
				}))
				t.Cleanup(upstream.Close)
				settings := hostedImageFinancialSettings(t, operation)
				server, worker := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
				worker.catalog = settings.catalog
				worker.hostedAdmission = settings.mediaAdmission(worker.providers)
				scope, err := json.Marshal([]hostedGrantOffering{{Model: "gpt-image-2", Operations: []string{operation}}})
				if err != nil {
					t.Fatal(err)
				}
				if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", "grant-journal").Update("offerings", scope).Error; err != nil {
					t.Fatal(err)
				}
				capability := llmproxycontract.MediaCapabilityImageGenerate
				input := `{"prompt":"funded image"}`
				if operation == ModelOperationImageEditing {
					capability = llmproxycontract.MediaCapabilityImageEdit
					asset, err := worker.assets.upload(tenant{identifier: tenantID("managed-first")}, "image/png", bytes.NewReader(png))
					if err != nil {
						t.Fatal(err)
					}
					input = fmt.Sprintf(`{"prompt":"funded image","image_asset_ids":[%q]}`, asset.AssetID)
				}
				adapter := worker.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")].(*imageGenerationAdapter)
				offering, err := settings.catalog.ResolveOffering("openai", "gpt-image-2")
				if err != nil {
					t.Fatal(err)
				}
				worker.adapters[mediaOperationAdapterKey(capability, "openai", "gpt-image-2")] = newImageGenerationAdapter(offering, worker.providers.definitions[providerID("openai")], adapter.tenants, worker.store, worker.assets, settings.catalog)
				surface, extra := "images", ""
				if mode == "responses_unknown_meter" {
					surface, extra = "responses", `,"responses_model":"gpt-5"`
				}
				intent := fmt.Sprintf(`{"capability":%q,"provider":"openai","model":"gpt-image-2","input":%s,"controls":{"surface":%q%s,"quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1,"stream":%t}}`, capability, input, surface, extra, mode == "stream")
				if mode == "responses_unknown_meter" {
					seedHostedFunds(t, database, 500)
					for range 2 {
						hostedSpeechHTTP(t, server, "unbounded-responses-image", intent, http.StatusServiceUnavailable)
					}
					assertHostedFundsBalance(t, database, 500, 500)
					if calls.Load() != 0 {
						t.Fatal("unbounded Responses image dispatched")
					}
					charges := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)["charges"].([]any)
					if len(charges) != 0 {
						t.Fatalf("rejected Responses image created charges: %v", charges)
					}
					return
				}
				hostedSpeechHTTP(t, server, "unfunded-image", intent, http.StatusPaymentRequired)
				if calls.Load() != 0 {
					t.Fatal("unfunded image dispatched")
				}
				seedHostedFunds(t, database, 500)
				accepted := hostedSpeechHTTP(t, server, "funded-image", intent, http.StatusAccepted)
				id := accepted["operation_id"].(string)
				assertHostedFundsBalance(t, database, 500, 461)
				worker.runOperation("funded-image", id)
				state := MediaOperationStateSucceeded
				if mode == "invalid_output" {
					state = MediaOperationStateFailed
				}
				if result := hostedMediaWorkerStatus(t, server, id); result["state"] != state {
					t.Fatalf("image state=%v", result)
				}
				for range 2 {
					if err := database.reconcileHostedFunds(t.Context(), time.Now().UTC()); err != nil {
						t.Fatal(err)
					}
					worker.runOperation("duplicate-image", id)
					if replay := hostedSpeechHTTP(t, server, "funded-image", intent, http.StatusOK); replay["operation_id"] != id {
						t.Fatal("image replay identity changed")
					}
				}
				charges := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)["charges"].([]any)
				if len(charges) != 1 || calls.Load() != 1 {
					t.Fatalf("charges=%d calls=%d", len(charges), calls.Load())
				}
				charge := charges[0].(map[string]any)
				requestPath := "/billing-accounts/billing-journal/requests/" + charge["request_id"].(string)
				summary := ratingHTTPExchange(t, management, http.MethodGet, requestPath+"/charge-summary", "", http.StatusOK)
				balance := ratingHTTPExchange(t, management, http.MethodGet, fundsBalanceTestPath, "", http.StatusOK)
				switch mode {
				case "json", "stream":
					assertHostedFundsBalance(t, database, 498, 498)
					assertFundsCreditRemainder(t, database, "1", "1250")
					if charge["state"] != chargeRated || summary["state"] != string(requestChargeRated) || !reflect.DeepEqual(summary["provider_cost"], map[string]any{"numerator": "2", "denominator": "125"}) || !reflect.DeepEqual(summary["customer_charge"], map[string]any{"numerator": "13", "denominator": "625"}) {
						t.Fatalf("image financial totals=%v", summary)
					}
				case "zero":
					assertHostedFundsBalance(t, database, 500, 500)
					assertFundsCreditRemainder(t, database, "0", "1")
					if !reflect.DeepEqual(summary["customer_charge"], map[string]any{"numerator": "0", "denominator": "1"}) {
						t.Fatalf("zero image=%v", summary)
					}
				default:
					assertHostedFundsBalance(t, database, 500, 461)
					if summary["state"] != string(requestChargeUnresolved) || summary["customer_charge"] != nil || balance["pending_cents"] != "39" {
						t.Fatalf("unresolved image summary=%v balance=%v", summary, balance)
					}
					if mode == "invalid_output" {
						if !reflect.DeepEqual(summary["provider_cost"], map[string]any{"numerator": "2", "denominator": "125"}) {
							t.Fatalf("failed output lost provider cost: %v", summary)
						}
					} else if charge["state"] != chargeUsageUnresolved || summary["provider_cost"] != nil {
						t.Fatalf("unmeasured image charged: %v", summary)
					}
				}
			})
		}
	}
}

func hostedImageFinancialSettings(t *testing.T, operation string) *hostedRuntimeSettings {
	t.Helper()
	catalog := internalCanonicalProviderCatalog().ModelCatalog()
	for index := range catalog.Offerings {
		offering := &catalog.Offerings[index]
		if offering.Provider == "openai" && offering.Model == "gpt-image-2" {
			for _, dimension := range []string{"input_text_tokens", "input_image_tokens", "output_text_tokens", "output_image_tokens"} {
				maximum := 100
				offering.Limits = append(offering.Limits, CatalogLimit{ID: dimension, Unit: "tokens", Value: &maximum})
			}
		}
	}
	for index := range catalog.Prices {
		price := &catalog.Prices[index]
		if price.Provider != "openai" || price.Model != "gpt-image-2" || price.Operation != operation {
			continue
		}
		*price = CatalogPriceDescriptor{Provider: price.Provider, Model: price.Model, Operation: operation, Available: true, Source: "https://example.com/controlled-image-prices", LastVerified: "2026-09-23"}
		for index, component := range []string{"input_text", "input_image", "output_text", "output_image"} {
			price.Rates = append(price.Rates, CatalogPriceRate{Component: component, Currency: "USD", Rate: CatalogDecimal(fmt.Sprint(200 << index)), Unit: "USD/1M_tokens", Conditions: CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z", Quality: "low", Resolution: "1024x1024"}})
		}
	}
	settings, err := newHostedRuntimeSettings(&HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: "openai", Model: "gpt-image-2", Operation: operation, MaximumAttempts: 1, Conditions: CatalogPriceConditions{Quality: "low", Resolution: "1024x1024"}}}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return settings
}
