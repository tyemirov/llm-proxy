package proxy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

func TestHostedFundsResponsesImageAdvertisedCombinedPrice(t *testing.T) {
	png := imageBoundaryPNG(t, image.NewNRGBA(image.Rect(0, 0, 1024, 1024)))
	for _, operation := range []string{ModelOperationImageGeneration, ModelOperationImageEditing} {
		for _, mode := range []string{"complete", "poll", "stream", "zero", "missing_tool", "missing_model", "excess_tool", "invalid_output"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				database, _, management, _ := newHostedRatingFixture(t)
				usage := `{"total_tokens":1100,"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":200},"output_tokens_details":{"reasoning_tokens":20}}`
				toolUsage := `{"image_gen":{"total_tokens":214,"input_tokens":18,"output_tokens":196,"input_tokens_details":{"text_tokens":18,"image_tokens":0},"output_tokens_details":{"text_tokens":0,"image_tokens":196}}}`
				output := base64.StdEncoding.EncodeToString(png)
				expectedState, expectedCharge := MediaOperationStateSucceeded, chargeRated
				available := int64(500)
				remainder := ExactMoney{Numerator: "6513", Denominator: "1000000"}
				cost := ExactMoney{Numerator: "501", Denominator: "100000"}
				switch mode {
				case "zero":
					usage = `{"total_tokens":0,"input_tokens":0,"output_tokens":0,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`
					toolUsage = `{"image_gen":{"total_tokens":0,"input_tokens":0,"output_tokens":0,"input_tokens_details":{"text_tokens":0,"image_tokens":0},"output_tokens_details":{"text_tokens":0,"image_tokens":0}}}`
					remainder, cost = ExactMoney{Numerator: "0", Denominator: "1"}, ExactMoney{Numerator: "0", Denominator: "1"}
				case "missing_tool":
					toolUsage = `null`
					expectedCharge = chargeUsageUnresolved
				case "missing_model":
					usage = `null`
					expectedCharge = chargeUsageUnresolved
				case "excess_tool":
					toolUsage = `{"image_gen":{"total_tokens":2019,"input_tokens":18,"output_tokens":2001,"input_tokens_details":{"text_tokens":18,"image_tokens":0},"output_tokens_details":{"text_tokens":0,"image_tokens":2001}}}`
					expectedCharge = chargeLimitUnresolved
				case "invalid_output":
					output = base64.StdEncoding.EncodeToString([]byte("invalid image"))
					expectedState = MediaOperationStateFailed
				}
				quote := remainder
				if expectedCharge != chargeRated || expectedState == MediaOperationStateFailed {
					available = 494
					remainder = ExactMoney{Numerator: "0", Denominator: "1"}
				}
				var calls, polls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodDelete {
						w.WriteHeader(http.StatusNoContent)
						return
					}
					terminal := fmt.Sprintf(`{"id":"resp_advertised","status":"completed","output":[{"id":"image_advertised","type":"image_generation_call","status":"completed","result":%q}],"usage":%s,"tool_usage":%s}`, output, usage, toolUsage)
					if r.Method == http.MethodGet {
						polls.Add(1)
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprint(w, terminal)
						return
					}
					calls.Add(1)
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
						return
					}
					if payload["model"] != "gpt-5" || payload["max_tool_calls"] != float64(1) {
						t.Errorf("unbounded or wrong response model: %v", payload)
					}
					w.Header().Set("Content-Type", "application/json")
					if mode == "poll" {
						fmt.Fprint(w, `{"id":"resp_advertised","status":"queued"}`)
					} else if mode == "stream" {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "event: response.created\ndata: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_advertised\",\"status\":\"queued\"}}\n\n")
						fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":1,\"response\":%s}\n\n", terminal)
					} else {
						fmt.Fprint(w, terminal)
					}
				}))
				t.Cleanup(upstream.Close)
				settings := hostedResponsesAdvertisedSettings(t, operation)
				server, worker := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
				worker.catalog = settings.catalog
				worker.hostedAdmission = settings.mediaAdmission(worker.providers)
				grant, err := json.Marshal([]hostedGrantOffering{{Model: "gpt-image-2", Operations: []string{operation}}})
				if err != nil {
					t.Fatal(err)
				}
				if err := database.database.Model(&managedHostedGrantRecord{}).Where("id = ?", hostedJournalFixtureGrantID).Update("offerings", grant).Error; err != nil {
					t.Fatal(err)
				}
				capability := llmproxycontract.MediaCapabilityImageGenerate
				input := `{"prompt":"advertised image"}`
				if operation == ModelOperationImageEditing {
					capability = llmproxycontract.MediaCapabilityImageEdit
					asset, err := worker.assets.upload(tenant{identifier: tenantID("managed-first")}, "image/png", bytes.NewReader(png))
					if err != nil {
						t.Fatal(err)
					}
					input = fmt.Sprintf(`{"prompt":"advertised image","image_asset_ids":[%q]}`, asset.AssetID)
				}
				adapter := worker.adapters[mediaOperationAdapterKey(llmproxycontract.MediaCapabilityImageGenerate, "openai", "gpt-image-2")].(*imageGenerationAdapter)
				offering, err := settings.catalog.ResolveOffering("openai", "gpt-image-2")
				if err != nil {
					t.Fatal(err)
				}
				worker.adapters[mediaOperationAdapterKey(capability, "openai", "gpt-image-2")] = newImageGenerationAdapter(offering, worker.providers.definitions[providerID("openai")], adapter.tenants, worker.store, worker.assets, settings.catalog)
				intent := fmt.Sprintf(`{"capability":%q,"provider":"openai","model":"gpt-image-2","input":%s,"controls":{"surface":"responses","responses_model":"gpt-5","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1}}`, capability, input)
				if mode == "stream" {
					intent = strings.Replace(intent, `"output_count":1`, `"output_count":1,"stream":true`, 1)
				}
				hostedSpeechHTTP(t, server, "unfunded-advertised", intent, http.StatusPaymentRequired)
				if calls.Load() != 0 {
					t.Fatal("unfunded image dispatched")
				}
				seedHostedFunds(t, database, 500)
				accepted := hostedSpeechHTTP(t, server, "advertised-image", intent, http.StatusAccepted)
				id := accepted["operation_id"].(string)
				assertHostedFundsBalance(t, database, 500, 494)
				if operation == ModelOperationImageGeneration && mode == "complete" {
					assertHostedResponsesPriceOriginRecovery(t, database, management)
				}
				worker.runOperation("advertised-worker", id)
				if result := hostedMediaWorkerStatus(t, server, id); result["state"] != expectedState {
					t.Fatalf("result=%v", result)
				}
				for range 2 {
					restarted := openJournalTransactionInstance(t, database)
					if err := restarted.reconcileHostedFunds(t.Context(), time.Now()); err != nil {
						t.Fatal(err)
					}
					worker.runOperation("advertised-replay", id)
					if replay := hostedSpeechHTTP(t, server, "advertised-image", intent, http.StatusOK); replay["operation_id"] != id {
						t.Fatal("replay identity changed")
					}
					assertHostedFundsBalance(t, database, 500, available)
					assertFundsCreditRemainder(t, database, remainder.Numerator, remainder.Denominator)
				}
				charges := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/charges", "", http.StatusOK)["charges"].([]any)
				if mode == "poll" && polls.Load() != 1 {
					t.Fatalf("polls=%d", polls.Load())
				}
				if len(charges) != 1 || calls.Load() != 1 {
					t.Fatalf("charges=%v calls=%d", charges, calls.Load())
				}
				charge := charges[0].(map[string]any)
				rating := charge["rating"].(map[string]any)
				if charge["state"] != expectedCharge {
					t.Fatalf("combined charge state=%v", charge)
				}
				if expectedCharge == chargeRated {
					if !reflect.DeepEqual(rating["provider_cost"], map[string]any{"numerator": cost.Numerator, "denominator": cost.Denominator}) || !reflect.DeepEqual(charge["customer_charge"], map[string]any{"numerator": quote.Numerator, "denominator": quote.Denominator}) {
						t.Fatalf("combined advertised rating=%v", charge)
					}
				} else if charge["customer_charge"] != nil {
					t.Fatalf("unresolved work has a customer charge: %v", charge)
				}

				summary := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/requests/"+charge["request_id"].(string)+"/charge-summary", "", http.StatusOK)
				if expectedCharge != chargeRated || expectedState == MediaOperationStateFailed {
					if summary["state"] != string(requestChargeUnresolved) || summary["customer_charge"] != nil {
						t.Fatalf("unresolved request settled: %v", summary)
					}
				}
				price := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/price-snapshots/"+charge["price_snapshot_id"].(string), "", http.StatusOK)
				snapshot := price["snapshot"].(map[string]any)
				components := snapshot["components"].([]any)
				if len(components) != 6 {
					t.Fatalf("combined components=%v", components)
				}
				models := map[string]int{}
				for _, value := range components {
					origin := value.(map[string]any)["origin"].(map[string]any)
					models[origin["model"].(string)]++
					if origin["provider"] != "openai" || origin["source"] == "" || origin["last_verified"] == "" {
						t.Fatalf("lost source: %v", origin)
					}
				}
				if models["gpt-image-2"] != 3 || models["gpt-5"] != 3 {
					t.Fatalf("price owners=%v", models)
				}
				var observation managedJournalObservationRecord
				if err := database.database.First(&observation).Error; err != nil {
					t.Fatal(err)
				}
				var quantities []journalQuantity
				if err := json.Unmarshal(observation.Quantities, &quantities); err != nil {
					t.Fatal(err)
				}
				unknown := 0
				for _, q := range quantities {
					if q.Dimension == "cache_read_text_tokens" || q.Dimension == "cache_read_image_tokens" {
						if q.UnknownReason != journalQuantityUnsupported || q.Value != "" {
							t.Fatalf("fabricated cache count: %+v", q)
						}
						unknown++
					}
				}
				if unknown != 2 {
					t.Fatal("image cache evidence lost")
				}

				if operation == ModelOperationImageGeneration && mode == "complete" {
					root := worker.assets.root
					server.Close()
					changed := hostedResponsesAdvertisedCatalog(operation)
					for i := range changed.Prices {
						if changed.Prices[i].Provider == "openai" && (changed.Prices[i].Model == "gpt-5" || changed.Prices[i].Model == "gpt-image-2") {
							for j := range changed.Prices[i].Rates {
								changed.Prices[i].Rates[j].Rate = "99"
							}
						}
					}
					current := hostedResponsesSettingsForCatalog(t, operation, changed)
					for range 2 {
						reopened := openJournalTransactionInstance(t, database)
						if err := reopened.reconcileHostedFunds(t.Context(), time.Now()); err != nil {
							t.Fatal(err)
						}
						recovered, recoveredWorker := newHostedMediaAdmissionHTTPServer(t, reopened, hostedMediaWorkerProvider(upstream.URL), func(service *mediaOperationService) {
							service.assets = newTenantAssetStore(root, 4096, 60)
						})
						recoveredWorker.catalog = current.catalog
						recoveredWorker.hostedAdmission = current.mediaAdmission(recoveredWorker.providers)
						if replay := hostedSpeechHTTP(t, recovered, "advertised-image", intent, http.StatusOK); replay["operation_id"] != id {
							t.Fatal("restart changed request identity")
						}
						recoveredWorker.runOperation("restarted-advertised", id)
						if result := hostedMediaWorkerStatus(t, recovered, id); result["state"] != MediaOperationStateSucceeded {
							t.Fatalf("restart lost result: %v", result)
						}
						assertHostedFundsBalance(t, reopened, 500, 500)
						assertFundsCreditRemainder(t, reopened, "6513", "1000000")
						if calls.Load() != 1 {
							t.Fatal("restart submitted provider work again")
						}
						retained := ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/price-snapshots/"+charge["price_snapshot_id"].(string), "", http.StatusOK)
						if !reflect.DeepEqual(price, retained) {
							t.Fatal("current catalog changed accepted price")
						}
						recovered.Close()
					}
				}
			})
		}
	}
}

func hostedResponsesAdvertisedCatalog(operation string) ModelCatalog {
	catalog := hostedImageFinancialCatalog(operation)
	for i := range catalog.Offerings {
		offering := &catalog.Offerings[i]
		if offering.Provider != "openai" {
			continue
		}
		if offering.Model == "gpt-image-2" {
			for j := range offering.Limits {
				if offering.Limits[j].Unit == "tokens" {
					v := 1000
					offering.Limits[j].Value = &v
				}
			}
		}
		if offering.Model == "gpt-5" {
			v := 1000
			offering.OutputTokenLimit = 1000
			offering.Limits = []CatalogLimit{{ID: "context_tokens", Value: &v, Unit: "tokens"}}
		}
	}
	imageConditions := CatalogPriceConditions{Quality: "low", Resolution: "1024x1024", EffectiveFrom: "2026-09-01T00:00:00Z"}
	textConditions := CatalogPriceConditions{EffectiveFrom: "2026-09-01T00:00:00Z"}
	cacheConditions := textConditions
	cacheConditions.CacheClass = "read"
	for i := range catalog.Prices {
		price := &catalog.Prices[i]
		if price.Provider != "openai" {
			continue
		}
		if price.Model == "gpt-image-2" && price.Operation == operation {
			price.Rates = []CatalogPriceRate{
				{Component: "input_text", Currency: "USD", Rate: "2.5", Unit: "USD/1M_tokens", Conditions: imageConditions},
				{Component: "input_image", Currency: "USD", Rate: "4", Unit: "USD/1M_tokens", Conditions: imageConditions},
				{Component: "output_image", Currency: "USD", Rate: "15", Unit: "USD/1M_tokens", Conditions: imageConditions},
			}
		}
		if price.Model == "gpt-5" && price.Operation == ModelOperationText {
			price.Available = true
			price.UnavailableReason = ""
			price.Rates = []CatalogPriceRate{
				{Component: "input_tokens", Currency: "USD", Rate: "1.25", Unit: "USD/1M_tokens", Conditions: textConditions},
				{Component: "cache_read", Currency: "USD", Rate: "0.125", Unit: "USD/1M_tokens", Conditions: cacheConditions},
				{Component: "output_tokens", Currency: "USD", Rate: "10", Unit: "USD/1M_tokens", Conditions: textConditions},
			}
		}
	}
	return catalog
}

func hostedResponsesAdvertisedSettings(t *testing.T, operation string) *hostedRuntimeSettings {
	t.Helper()
	return hostedResponsesSettingsForCatalog(t, operation, hostedResponsesAdvertisedCatalog(operation))
}

func hostedResponsesSettingsForCatalog(t *testing.T, operation string, catalog ModelCatalog) *hostedRuntimeSettings {
	t.Helper()
	imageConditions := CatalogPriceConditions{Quality: "low", Resolution: "1024x1024"}
	settings, err := newHostedRuntimeSettings(&HostedConfiguration{Offerings: []HostedOfferingConfiguration{{Provider: "openai", Model: "gpt-image-2", Operation: operation, MaximumAttempts: 1, Conditions: categoricalPriceConditions(imageConditions)}}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return settings
}

func TestHostedFundsResponsesImageRejectsIncompletePrice(t *testing.T) {
	for _, scenario := range []string{"image_price", "response_price", "image_bound", "response_input", "response_output", "image_minimum", "response_minimum", "image_tier", "response_tier"} {
		t.Run(scenario, func(t *testing.T) {
			database, _, management, _ := newHostedRatingFixture(t)
			seedHostedFunds(t, database, 500)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(upstream.Close)
			server, worker := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL))
			catalog := hostedResponsesAdvertisedCatalog(ModelOperationImageGeneration)
			for i := range catalog.Prices {
				price := &catalog.Prices[i]
				if price.Provider != "openai" {
					continue
				}
				target := ""
				if price.Model == "gpt-image-2" && price.Operation == ModelOperationImageGeneration {
					target = "image"
				}
				if price.Model == "gpt-5" && price.Operation == ModelOperationText {
					target = "response"
				}
				if target == "" {
					continue
				}
				switch scenario {
				case target + "_price":
					price.Available, price.Rates = false, nil
					price.UnavailableReason = "Controlled price qualification is incomplete."
				case target + "_minimum":
					price.MinimumCharge = &CatalogMinimumCharge{Currency: "USD", Amount: "1", Unit: "USD/request"}
				case target + "_tier":
					for j := range price.Rates {
						price.Rates[j].Conditions.InputTokens.MaximumExclusive = 10000
					}
				}
			}
			for i := range catalog.Offerings {
				offering := &catalog.Offerings[i]
				if offering.Provider != "openai" {
					continue
				}
				if offering.Model == "gpt-5" {
					if scenario == "response_input" {
						offering.Limits = nil
					}
					if scenario == "response_output" {
						offering.OutputTokenLimit = 0
					}
				}
				if offering.Model == "gpt-image-2" && scenario == "image_bound" {
					for j, limit := range offering.Limits {
						if limit.ID == "output_image_tokens" {
							offering.Limits = append(offering.Limits[:j], offering.Limits[j+1:]...)
							break
						}
					}
				}
			}
			settings := hostedResponsesSettingsForCatalog(t, ModelOperationImageGeneration, catalog)
			worker.catalog, worker.hostedAdmission = settings.catalog, settings.mediaAdmission(worker.providers)
			intent := `{"capability":"image.generate","provider":"openai","model":"gpt-image-2","input":{"prompt":"advertised image"},"controls":{"surface":"responses","responses_model":"gpt-5","quality":"low","size":"1024x1024","background":"opaque","output_format":"png","output_count":1}}`
			financial := fundsStartupFixture{database: database, management: management, calls: &calls}
			before := financial.state(t)
			for range 2 {
				hostedSpeechHTTP(t, server, "repair-advertised", intent, http.StatusServiceUnavailable)
				if !reflect.DeepEqual(before, financial.state(t)) || calls.Load() != 0 || len(worker.queue) != 0 {
					t.Fatal("incomplete price changed financial state or dispatched work")
				}
				for _, model := range []any{&managedJournalRequestRecord{}, &managedPriceSnapshotRecord{}, &managedFundsReservationRecord{}, &mediaOperationRecord{}} {
					var count int64
					if err := database.database.Model(model).Count(&count).Error; err != nil || count != 0 {
						t.Fatalf("rejected admission retained %T: count=%d error=%v", model, count, err)
					}
				}
			}
			settings = hostedResponsesAdvertisedSettings(t, ModelOperationImageGeneration)
			worker.catalog, worker.hostedAdmission = settings.catalog, settings.mediaAdmission(worker.providers)
			accepted := hostedSpeechHTTP(t, server, "repair-advertised", intent, http.StatusAccepted)
			replay := hostedSpeechHTTP(t, server, "repair-advertised", intent, http.StatusOK)
			if accepted["operation_id"] != replay["operation_id"] || len(worker.queue) != 1 || calls.Load() != 0 {
				t.Fatal("price repair duplicated work")
			}
			assertHostedFundsBalance(t, database, 500, 494)
			assertFundsCreditRemainder(t, database, "0", "1")
		})
	}
}

func assertHostedResponsesPriceOriginRecovery(t *testing.T, database *gormManagedTenantDatabase, management *httptest.Server) {
	t.Helper()
	var original managedPriceSnapshotRecord
	if err := database.database.First(&original).Error; err != nil {
		t.Fatal(err)
	}
	financial := fundsStartupFixture{database: database, management: management, calls: &atomic.Int64{}}
	before := financial.state(t)
	for _, scenario := range []string{"provider", "empty_model", "long_model", "spaced_model", "operation", "source", "verified"} {
		var document hostedPriceSnapshotDocument
		if err := json.Unmarshal(original.Document, &document); err != nil {
			t.Fatal(err)
		}
		origin := document.Components[0].Origin
		switch scenario {
		case "provider":
			origin.Provider = "different"
		case "empty_model":
			origin.Model = ""
		case "long_model":
			origin.Model = strings.Repeat("a", 129)
		case "spaced_model":
			origin.Model = " gpt-image-2"
		case "operation":
			origin.Operation = "speech"
		case "source":
			origin.Source = "not-a-url"
		case "verified":
			origin.LastVerified = "not-a-date"
		}
		encoded, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if err := database.database.Model(&managedPriceSnapshotRecord{}).Where("id = ?", original.ID).Updates(map[string]any{"document": encoded, "digest": sha256Hex(string(encoded))}).Error; err != nil {
			t.Fatal(err)
		}
		ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/price-snapshots/"+original.ID, "", http.StatusInternalServerError)
		if !reflect.DeepEqual(before, financial.state(t)) {
			t.Fatal("invalid price origin changed funds")
		}
	}
	if err := database.database.Model(&managedPriceSnapshotRecord{}).Where("id = ?", original.ID).Updates(map[string]any{"document": original.Document, "digest": original.Digest}).Error; err != nil {
		t.Fatal(err)
	}
	ratingHTTPExchange(t, management, http.MethodGet, "/billing-accounts/billing-journal/price-snapshots/"+original.ID, "", http.StatusOK)
}
